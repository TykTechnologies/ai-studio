import React, { useState, useEffect, useCallback } from "react";
import { useNavigate } from "react-router-dom";
import apiClient from "../utils/apiClient";
import {
  Table,
  TableBody,
  TableCell,
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
} from "@mui/material";
import MoreVertIcon from "@mui/icons-material/MoreVert";
import AddIcon from "@mui/icons-material/Add";
import FiberManualRecordIcon from "@mui/icons-material/FiberManualRecord";
import EmptyStateWidget from "../components/common/EmptyStateWidget";
import {
  StyledPaper,
  TitleBox,
  ContentBox,
  StyledTableCell,
  StyledTableRow,
  StyledButton,
} from "../styles/sharedStyles";
import { getVendorName, getVendorLogo } from "../utils/vendorLogos";
import InfoTooltip from "../components/common/InfoTooltip";
import PaginationControls from "../components/common/PaginationControls";
import usePagination from "../hooks/usePagination";

const LLMList = () => {
  const navigate = useNavigate();
  const [llms, setLLMs] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [anchorEl, setAnchorEl] = useState(null);
  const [selectedLLM, setSelectedLLM] = useState(null);
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

  const fetchLLMs = useCallback(async () => {
    try {
      setLoading(true);
      const response = await apiClient.get("/llms", {
        params: {
          page,
          page_size: pageSize,
          sort_by: sortConfig.key,
          sort_direction: sortConfig.direction,
        },
      });
      setLLMs(response.data.data || []);
      const totalCount = parseInt(response.headers["x-total-count"] || "0", 10);
      const totalPages = parseInt(response.headers["x-total-pages"] || "0", 10);
      updatePaginationData(totalCount, totalPages);
      setError("");
    } catch (error) {
      console.error("Error fetching LLMs", error);
      setError("Failed to load LLMs");
    } finally {
      setLoading(false);
    }
  }, [page, pageSize, sortConfig, updatePaginationData]);

  useEffect(() => {
    fetchLLMs();
  }, [fetchLLMs]);

  const handleMenuOpen = (event, llm) => {
    event.stopPropagation();
    setAnchorEl(event.currentTarget);
    setSelectedLLM(llm);
  };

  const handleMenuClose = () => {
    setAnchorEl(null);
  };

  const handleDelete = async (id) => {
    try {
      await apiClient.delete(`/llms/${id}`);
      setSnackbar({
        open: true,
        message: "LLM deleted successfully",
        severity: "success",
      });
      fetchLLMs();
    } catch (error) {
      console.error("Error deleting LLM", error);
      setSnackbar({
        open: true,
        message: "Failed to delete LLM",
        severity: "error",
      });
    }
    handleMenuClose();
  };

  const handleToggleActive = async (llm) => {
    try {
      const updatedLLM = {
        ...llm,
        attributes: { ...llm.attributes, active: !llm.attributes.active },
      };
      await apiClient.patch(`/llms/${llm.id}`, { data: updatedLLM });
      setSnackbar({
        open: true,
        message: `LLM ${updatedLLM.attributes.active ? "activated" : "deactivated"} successfully`,
        severity: "success",
      });
      fetchLLMs();
    } catch (error) {
      console.error("Error toggling LLM active state", error);
      setSnackbar({
        open: true,
        message: "Failed to update LLM active state",
        severity: "error",
      });
    }
    handleMenuClose();
  };

  const handleLLMClick = (llm) => {
    navigate(`/admin/llms/${llm.id}`);
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

  const handleAddLLM = () => {
    navigate("/admin/llms/new");
  };

  if (loading && llms.length === 0) {
    return <CircularProgress />;
  }

  if (error && llms.length === 0) {
    return <Alert severity="error">{error}</Alert>;
  }

  return (
    <Box sx={{ p: 0 }}>
      <StyledPaper>
        <TitleBox>
          <Box display="flex" alignItems="center">
            <InfoTooltip title="Large Language Models (LLMs) registered here can be used in chat rooms, and are available to developers in the Portal if set to Active. They must be part of a catalog in order to be usable by a group." />
            <Typography variant="h5">LLMs</Typography>
          </Box>

          <StyledButton
            variant="contained"
            startIcon={<AddIcon />}
            onClick={handleAddLLM}
          >
            Add LLM
          </StyledButton>
        </TitleBox>
        <ContentBox>
          {llms.length === 0 ? (
            <EmptyStateWidget
              title="Want to start working with your favourite LLM?"
              description="Click the button below to add a new LLM configuration to use in your chat room."
              buttonText="Add LLM"
              buttonIcon={<AddIcon />}
              onButtonClick={handleAddLLM}
            />
          ) : (
            <>
              <Table>
                <TableHead>
                  <TableRow>
                    <StyledTableCell onClick={() => handleSort("name")}>
                      Name
                    </StyledTableCell>
                    <StyledTableCell>Short Description</StyledTableCell>
                    <StyledTableCell onClick={() => handleSort("vendor")}>
                      Vendor
                    </StyledTableCell>
                    <StyledTableCell
                      onClick={() => handleSort("privacy_score")}
                    >
                      Privacy Score
                    </StyledTableCell>
                    <StyledTableCell onClick={() => handleSort("active")}>
                      Active
                    </StyledTableCell>
                    <StyledTableCell align="right">Actions</StyledTableCell>
                  </TableRow>
                </TableHead>
                <TableBody>
                  {llms.map((llm) => (
                    <StyledTableRow
                      key={llm.id}
                      onClick={() => handleLLMClick(llm)}
                      sx={{ cursor: "pointer" }}
                    >
                      <TableCell>{llm.attributes.name}</TableCell>
                      <TableCell>{llm.attributes.short_description}</TableCell>
                      <TableCell>
                        <Box sx={{ display: "flex", alignItems: "center" }}>
                          <img
                            src={getVendorLogo(llm.attributes.vendor)}
                            alt={getVendorName(llm.attributes.vendor)}
                            style={{
                              width: 24,
                              height: 24,
                              marginRight: 8,
                              objectFit: "contain",
                            }}
                            onError={(e) => {
                              e.target.onerror = null;
                              e.target.src =
                                process.env.PUBLIC_URL +
                                "/images/placeholder-logo.png";
                            }}
                          />
                          {getVendorName(llm.attributes.vendor)}
                        </Box>
                      </TableCell>
                      <TableCell>{llm.attributes.privacy_score}</TableCell>
                      <TableCell>
                        <FiberManualRecordIcon
                          sx={{
                            color: llm.attributes.active ? "green" : "red",
                          }}
                        />
                      </TableCell>
                      <TableCell align="right">
                        <IconButton
                          onClick={(event) => handleMenuOpen(event, llm)}
                        >
                          <MoreVertIcon />
                        </IconButton>
                      </TableCell>
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
            </>
          )}
        </ContentBox>
      </StyledPaper>

      <Menu
        anchorEl={anchorEl}
        open={Boolean(anchorEl)}
        onClose={handleMenuClose}
      >
        <MenuItem
          onClick={() => navigate(`/admin/llms/edit/${selectedLLM?.id}`)}
        >
          Edit LLM
        </MenuItem>
        <MenuItem onClick={() => handleDelete(selectedLLM?.id)}>
          Delete LLM
        </MenuItem>
        <MenuItem onClick={() => handleToggleActive(selectedLLM)}>
          {selectedLLM?.attributes.active ? "Deactivate" : "Activate"} LLM
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

export default LLMList;
