import React, { useState, useEffect, useCallback } from "react";
import { useNavigate } from "react-router-dom";
import apiClient from "../utils/apiClient";
import {
  Table,
  TableBody,
  TableHead,
  TableRow,
  Typography,
  IconButton,
  CircularProgress,
  Alert,
  Menu,
  MenuItem,
  Snackbar,
  Box,
  Chip,
} from "@mui/material";
import MoreVertIcon from "@mui/icons-material/MoreVert";
import AddIcon from "@mui/icons-material/Add";
import FiberManualRecordIcon from "@mui/icons-material/FiberManualRecord";
import EmptyStateWidget from "../components/common/EmptyStateWidget";
import {
  TitleBox,
  StyledPaper,
  StyledTableCell,
  StyledTableHeaderCell,
  StyledTableRow,
  PrimaryButton,
} from "../styles/sharedStyles";
import PaginationControls from "../components/common/PaginationControls";
import usePagination from "../hooks/usePagination";

const ModelRouterList = () => {
  const navigate = useNavigate();
  const [routers, setRouters] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [anchorEl, setAnchorEl] = useState(null);
  const [selectedRouter, setSelectedRouter] = useState(null);
  const [snackbar, setSnackbar] = useState({
    open: false,
    message: "",
    severity: "success",
  });
  const [sortConfig, setSortConfig] = useState({ key: null, direction: "asc" });

  const {
    page,
    pageSize,
    totalPages,
    handlePageChange,
    handlePageSizeChange,
    updatePaginationData,
  } = usePagination();

  const fetchRouters = useCallback(async () => {
    try {
      setLoading(true);
      const response = await apiClient.get("/model-routers", {
        params: {
          page,
          page_size: pageSize,
          sort_by: sortConfig.key,
          sort_direction: sortConfig.direction,
        },
      });
      setRouters(response.data.data || []);
      const totalCount = parseInt(response.headers["x-total-count"] || "0", 10);
      const totalPages = parseInt(response.headers["x-total-pages"] || "0", 10);
      updatePaginationData(totalCount, totalPages);
      setError("");
    } catch (error) {
      console.error("Error fetching Model Routers", error);
      if (error.response?.status === 403) {
        setError("Model Routers require Enterprise Edition");
      } else {
        setError("Failed to load Model Routers");
      }
    } finally {
      setLoading(false);
    }
  }, [page, pageSize, sortConfig, updatePaginationData]);

  useEffect(() => {
    fetchRouters();
  }, [fetchRouters]);

  const handleMenuOpen = (event, router) => {
    event.stopPropagation();
    setAnchorEl(event.currentTarget);
    setSelectedRouter(router);
  };

  const handleMenuClose = () => {
    setAnchorEl(null);
  };

  const handleDelete = async (id) => {
    try {
      await apiClient.delete(`/model-routers/${id}`);
      setSnackbar({
        open: true,
        message: "Model Router deleted successfully",
        severity: "success",
      });
      fetchRouters();
    } catch (error) {
      console.error("Error deleting Model Router", error);
      setSnackbar({
        open: true,
        message: "Failed to delete Model Router",
        severity: "error",
      });
    }
    handleMenuClose();
  };

  const handleToggleActive = async (router) => {
    try {
      await apiClient.patch(`/model-routers/${router.id}/toggle`);
      setSnackbar({
        open: true,
        message: `Model Router ${!router.attributes.active ? "activated" : "deactivated"} successfully`,
        severity: "success",
      });
      fetchRouters();
    } catch (error) {
      console.error("Error toggling Model Router active state", error);
      setSnackbar({
        open: true,
        message: "Failed to update Model Router active state",
        severity: "error",
      });
    }
    handleMenuClose();
  };

  const handleRouterClick = (router) => {
    navigate(`/admin/model-routers/${router.id}`);
  };

  const handleCloseSnackbar = (event, reason) => {
    if (reason === "clickaway") {
      return;
    }
    setSnackbar({ ...snackbar, open: false });
  };

  const handleSort = (key) => {
    let direction = "asc";
    if (sortConfig.key === key && sortConfig.direction === "asc") {
      direction = "desc";
    }
    setSortConfig({ key, direction });
  };

  const handleAddRouter = () => {
    navigate("/admin/model-routers/new");
  };

  if (loading && routers.length === 0) {
    return <CircularProgress />;
  }

  if (error && routers.length === 0) {
    return <Alert severity="error">{error}</Alert>;
  }

  return (
    <Box sx={{ p: 0 }}>
      <>
        <TitleBox top="64px">
          <Typography variant="headingXLarge">Model Routers</Typography>
          <PrimaryButton
            variant="contained"
            startIcon={<AddIcon />}
            onClick={handleAddRouter}
          >
            Add Router
          </PrimaryButton>
        </TitleBox>
        <Box sx={{ p: 3 }}>
          <Typography variant="bodyLargeDefault" color="text.defaultSubdued">
            Model Routers enable intelligent request routing to LLM vendors based on model name patterns.
            Configure pools with glob patterns (e.g., "claude-*", "gpt-4*") to route requests to multiple vendors
            with round-robin or weighted selection algorithms.
          </Typography>
        </Box>
        <Box sx={{ p: 3 }}>
          {routers.length === 0 ? (
            <EmptyStateWidget
              title="Create your first Model Router"
              description="Model Routers let you define pools of LLM vendors and route requests based on model name patterns. Great for load balancing and failover."
              buttonText="Add Router"
              buttonIcon={<AddIcon />}
              onButtonClick={handleAddRouter}
            />
          ) : (
            <StyledPaper>
              <Table>
                <TableHead>
                  <TableRow>
                    <StyledTableHeaderCell onClick={() => handleSort("name")}>
                      Name
                    </StyledTableHeaderCell>
                    <StyledTableHeaderCell onClick={() => handleSort("slug")}>
                      Slug
                    </StyledTableHeaderCell>
                    <StyledTableHeaderCell>Description</StyledTableHeaderCell>
                    <StyledTableHeaderCell>Pools</StyledTableHeaderCell>
                    <StyledTableHeaderCell onClick={() => handleSort("active")}>
                      Status
                    </StyledTableHeaderCell>
                    <StyledTableHeaderCell align="right">Actions</StyledTableHeaderCell>
                  </TableRow>
                </TableHead>
                <TableBody>
                  {routers.map((router) => (
                    <StyledTableRow
                      key={router.id}
                      onClick={() => handleRouterClick(router)}
                      sx={{ cursor: "pointer" }}
                    >
                      <StyledTableCell>{router.attributes.name}</StyledTableCell>
                      <StyledTableCell>
                        <Chip
                          label={router.attributes.slug}
                          size="small"
                          variant="outlined"
                        />
                      </StyledTableCell>
                      <StyledTableCell>{router.attributes.description || "-"}</StyledTableCell>
                      <StyledTableCell>
                        {router.attributes.pools?.length || 0} pool(s)
                      </StyledTableCell>
                      <StyledTableCell>
                        <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                          <FiberManualRecordIcon
                            sx={{
                              color: router.attributes.active ? "green" : "red",
                              fontSize: 12,
                            }}
                          />
                          {router.attributes.active ? "Active" : "Inactive"}
                        </Box>
                      </StyledTableCell>
                      <StyledTableCell align="right">
                        <IconButton
                          onClick={(event) => handleMenuOpen(event, router)}
                        >
                          <MoreVertIcon />
                        </IconButton>
                      </StyledTableCell>
                    </StyledTableRow>
                  ))}
                </TableBody>
              </Table>
              <PaginationControls
                page={page}
                pageSize={pageSize}
                totalPages={totalPages}
                onPageChange={handlePageChange}
                onPageSizeChange={handlePageSizeChange}
              />
            </StyledPaper>
          )}
        </Box>
      </>

      <Menu
        anchorEl={anchorEl}
        open={Boolean(anchorEl)}
        onClose={handleMenuClose}
      >
        <MenuItem
          onClick={() => navigate(`/admin/model-routers/edit/${selectedRouter?.id}`)}
        >
          Edit Router
        </MenuItem>
        <MenuItem onClick={() => handleDelete(selectedRouter?.id)}>
          Delete Router
        </MenuItem>
        <MenuItem onClick={() => handleToggleActive(selectedRouter)}>
          {selectedRouter?.attributes.active ? "Deactivate" : "Activate"} Router
        </MenuItem>
      </Menu>

      <Snackbar
        open={snackbar.open}
        autoHideDuration={6000}
        onClose={handleCloseSnackbar}
        anchorOrigin={{ vertical: "bottom", horizontal: "center" }}
      >
        <Alert
          onClose={handleCloseSnackbar}
          severity={snackbar.severity}
          sx={{ width: "100%" }}
        >
          {snackbar.message}
        </Alert>
      </Snackbar>
    </Box>
  );
};

export default ModelRouterList;
