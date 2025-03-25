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
} from "@mui/material";
import MoreVertIcon from "@mui/icons-material/MoreVert";
import AddIcon from "@mui/icons-material/Add";
import EmptyStateWidget from "../components/common/EmptyStateWidget";
import {
  StyledPaper,
  TitleBox,
  ContentBox,
  StyledTableCell,
  StyledTableHeaderCell,
  StyledTableRow,
  PrimaryButton,
} from "../styles/sharedStyles";
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
  const [sortField, setSortField] = useState("id");
  const [sortOrder, setSortOrder] = useState("asc");

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
          sort: `${sortOrder === "desc" ? "-" : ""}${sortField}`,
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
  }, [page, pageSize, sortField, sortOrder, updatePaginationData]);

  useEffect(() => {
    fetchApps();
  }, [fetchApps]);

  useEffect(() => {
    fetchUsers();
  }, []);

  const fetchUsers = async () => {
    try {
      // Request all users by setting all=true
      const response = await apiClient.get("/users", {
        params: {
          all: true,
          // Add a large page_size as a fallback in case 'all' is not working
          page_size: 1000
        }
      });
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
    navigate(`/admin/apps/${app.id}`);
  };

  const handleCloseSnackbar = (event, reason) => {
    if (reason === "clickaway") {
      return;
    }
    setSnackbar({ ...snackbar, open: false });
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
    <>
      <>
        <TitleBox top="64px">
          <Typography variant="headingXLarge">Apps</Typography>
          <PrimaryButton
            variant="contained"
            startIcon={<AddIcon />}
            onClick={handleAddApp}
          >
            Add app
          </PrimaryButton>
        </TitleBox>
        <Box sx={{ p: 3 }}>
          <Typography variant="bodyLargeDefault" color="text.defaultSubdued">Apps are used to grant developers direct access to LLMs and data sources in the AI Portal. With active credentials, an app can use the gateway API to work directly with LLMs or access the data source API to search through data. You can create apps for specific developers or set up catalogs so they can request access and customize their setup.</Typography>
        </Box>
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
            <StyledPaper>
              <Table>
                <TableHead>
                  <TableRow>
                    <StyledTableHeaderCell
                      onClick={() => {
                        setSortOrder(sortField === "id" ? (sortOrder === "asc" ? "desc" : "asc") : "asc");
                        setSortField("id");
                      }}
                      sx={{ cursor: 'pointer' }}
                    >
                      ID {sortField === "id" && (sortOrder === "asc" ? "↑" : "↓")}
                    </StyledTableHeaderCell>
                    <StyledTableHeaderCell
                      onClick={() => {
                        setSortOrder(sortField === "name" ? (sortOrder === "asc" ? "desc" : "asc") : "asc");
                        setSortField("name");
                      }}
                      sx={{ cursor: 'pointer' }}
                    >
                      Name {sortField === "name" && (sortOrder === "asc" ? "↑" : "↓")}
                    </StyledTableHeaderCell>
                    <StyledTableHeaderCell
                      onClick={() => {
                        setSortOrder(sortField === "description" ? (sortOrder === "asc" ? "desc" : "asc") : "asc");
                        setSortField("description");
                      }}
                      sx={{ cursor: 'pointer' }}
                    >
                      Description {sortField === "description" && (sortOrder === "asc" ? "↑" : "↓")}
                    </StyledTableHeaderCell>
                    <StyledTableHeaderCell
                      onClick={() => {
                        setSortOrder(sortField === "user_id" ? (sortOrder === "asc" ? "desc" : "asc") : "asc");
                        setSortField("user_id");
                      }}
                      sx={{ cursor: 'pointer' }}
                    >
                      User {sortField === "user_id" && (sortOrder === "asc" ? "↑" : "↓")}
                    </StyledTableHeaderCell>
                    <StyledTableHeaderCell align="right">Actions</StyledTableHeaderCell>
                  </TableRow>
                </TableHead>
                <TableBody>
                  {apps.map((app) => (
                    <StyledTableRow
                      key={app.id}
                      onClick={() => handleAppClick(app)}
                      sx={{ cursor: "pointer" }}
                    >
                      <StyledTableCell>{app.id}</StyledTableCell>
                      <StyledTableCell>{app.attributes.name}</StyledTableCell>
                      <StyledTableCell>{app.attributes.description}</StyledTableCell>
                      <StyledTableCell>
                        {users[app.attributes.user_id] || "Unknown"}
                      </StyledTableCell>
                      <StyledTableCell align="right">
                        <IconButton
                          onClick={(event) => handleMenuOpen(event, app)}
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
        </ContentBox>
      </>

      <Menu
        anchorEl={anchorEl}
        open={Boolean(anchorEl)}
        onClose={handleMenuClose}
      >
        <MenuItem
          onClick={() => navigate(`/admin/apps/edit/${selectedApp?.id}`)}
        >
          Edit app
        </MenuItem>
        <MenuItem onClick={() => handleDelete(selectedApp?.id)}>
          Delete app
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
    </>
  );
};

export default AppList;
