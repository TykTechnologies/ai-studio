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
import EmptyStateWidget from "../components/common/EmptyStateWidget";
import {
  StyledPaper,
  TitleBox,
  ContentBox,
  StyledTableCell,
  StyledTableRow,
  StyledButton,
} from "../styles/sharedStyles";
import InfoTooltip from "../components/common/InfoTooltip";
import PaginationControls from "../components/common/PaginationControls";
import usePagination from "../hooks/usePagination";

const AppList = () => {
  const navigate = useNavigate();
  const [apps, setApps] = useState([]);
  const [users, setUsers] = useState({});
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [anchorEl, setAnchorEl] = useState(null);
  const [selectedApp, setSelectedApp] = useState(null);
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

  const fetchApps = useCallback(async () => {
    try {
      setLoading(true);
      const response = await apiClient.get("/apps", {
        params: {
          page,
          page_size: pageSize,
          sort_by: sortConfig.key,
          sort_direction: sortConfig.direction,
        },
      });
      setApps(response.data.data || []);
      const totalCount = parseInt(response.headers["x-total-count"] || "0", 10);
      const totalPages = parseInt(response.headers["x-total-pages"] || "0", 10);
      updatePaginationData(totalCount, totalPages);
      setError("");
    } catch (error) {
      console.error("Error fetching apps", error);
      setError("Failed to load apps");
    } finally {
      setLoading(false);
    }
  }, [page, pageSize, sortConfig, updatePaginationData]);

  useEffect(() => {
    fetchApps();
  }, [fetchApps]);

  useEffect(() => {
    fetchUsers();
  }, []);

  const fetchUsers = async () => {
    try {
      const response = await apiClient.get("/users");
      const userMap = {};
      response.data.data.forEach((user) => {
        userMap[user.id] = user.attributes.name;
      });
      setUsers(userMap);
    } catch (error) {
      console.error("Error fetching users", error);
    }
  };

  const handleMenuOpen = (event, app) => {
    event.stopPropagation();
    setAnchorEl(event.currentTarget);
    setSelectedApp(app);
  };

  const handleMenuClose = () => {
    setAnchorEl(null);
  };

  const handleDelete = async (id) => {
    try {
      await apiClient.delete(`/apps/${id}`);
      setSnackbar({
        open: true,
        message: "App deleted successfully",
        severity: "success",
      });
      fetchApps();
    } catch (error) {
      console.error("Error deleting app", error);
      setSnackbar({
        open: true,
        message: "Failed to delete app",
        severity: "error",
      });
    }
    handleMenuClose();
  };

  const handleAppClick = (app) => {
    navigate(`/apps/${app.id}`);
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

  const handleAddApp = () => {
    navigate("/admin/apps/new");
  };

  if (loading && apps.length === 0) {
    return <CircularProgress />;
  }

  if (error && apps.length === 0) {
    return <Alert severity="error">{error}</Alert>;
  }

  return (
    <Box sx={{ p: 0 }}>
      <StyledPaper>
        <TitleBox>
          <Box display="flex" alignItems="center">
            <InfoTooltip title="Apps are requests by users to access LLMs and data sources in the AI Portal. An app with an active credential can access the gateway API to work directly with LLMs, or use the portal data source API to search data sources." />
            <Typography variant="h5">Apps</Typography>
          </Box>
          <StyledButton
            variant="contained"
            startIcon={<AddIcon />}
            onClick={handleAddApp}
          >
            Add App
          </StyledButton>
        </TitleBox>
        <ContentBox>
          {apps.length === 0 ? (
            <EmptyStateWidget
              title="No apps configured yet"
              description="Apps are requests by users to access LLMs and data sources in the AI Portal. An app with an active credential can access the gateway API to work directly with LLMs, or use the portal data source API to search data sources. Click the button below to add a new app configuration."
              buttonText="Add App"
              buttonIcon={<AddIcon />}
              onButtonClick={handleAddApp}
            />
          ) : (
            <>
              <Table>
                <TableHead>
                  <TableRow>
                    <StyledTableCell onClick={() => handleSort("name")}>
                      Name
                    </StyledTableCell>
                    <StyledTableCell>Description</StyledTableCell>
                    <StyledTableCell onClick={() => handleSort("user_id")}>
                      User
                    </StyledTableCell>
                    <StyledTableCell align="right">Actions</StyledTableCell>
                  </TableRow>
                </TableHead>
                <TableBody>
                  {apps.map((app) => (
                    <StyledTableRow
                      key={app.id}
                      onClick={() => handleAppClick(app)}
                      sx={{ cursor: "pointer" }}
                    >
                      <TableCell>{app.attributes.name}</TableCell>
                      <TableCell>{app.attributes.description}</TableCell>
                      <TableCell>
                        {users[app.attributes.user_id] || "Unknown"}
                      </TableCell>
                      <TableCell align="right">
                        <IconButton
                          onClick={(event) => handleMenuOpen(event, app)}
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
        <MenuItem onClick={() => navigate(`/apps/edit/${selectedApp?.id}`)}>
          Edit App
        </MenuItem>
        <MenuItem onClick={() => handleDelete(selectedApp?.id)}>
          Delete App
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

export default AppList;
