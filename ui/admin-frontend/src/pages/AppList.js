import React, { useState, useEffect } from "react";
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
  Tooltip,
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

  useEffect(() => {
    fetchApps();
    fetchUsers();
  }, []);

  const fetchApps = async () => {
    try {
      const response = await apiClient.get("/apps");
      setApps(response.data.data || []);
      setLoading(false);
    } catch (error) {
      console.error("Error fetching apps", error);
      setError("Failed to load apps");
      setLoading(false);
    }
  };

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
      setApps(apps.filter((app) => app.id !== id));
      setSnackbar({
        open: true,
        message: "App deleted successfully",
        severity: "success",
      });
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

  const sortedApps = [...apps].sort((a, b) => {
    if (sortConfig.key === null) return 0;
    const aValue =
      sortConfig.key === "user_id"
        ? users[a.attributes.user_id]
        : a.attributes[sortConfig.key];
    const bValue =
      sortConfig.key === "user_id"
        ? users[b.attributes.user_id]
        : b.attributes[sortConfig.key];
    if (aValue < bValue) return sortConfig.direction === "asc" ? -1 : 1;
    if (aValue > bValue) return sortConfig.direction === "asc" ? 1 : -1;
    return 0;
  });

  const handleAddApp = () => {
    navigate("/apps/new");
  };

  if (loading) {
    return <CircularProgress />;
  }

  if (error) {
    return <Alert severity="error">{error}</Alert>;
  }

  return (
    <Box sx={{ p: 0 }}>
      <StyledPaper>
        <TitleBox>
          <Box display="flex" alignItems="center">
            <InfoTooltip title="Apps are requests by users to access LLMs and data sources in the AI Portal. An app with an active credential can access the gateway API to work directly with LLMs, or use the portal data source API to search daata sources." />
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
              description="Apps are requests by users to access LLMs and data sources in the AI Portal. An app with an active credential can access the gateway API to work directly with LLMs, or use the portal data source API to search daata sources. Click the button below to add a new app configuration."
              buttonText="Add App"
              buttonIcon={<AddIcon />}
              onButtonClick={handleAddApp}
            />
          ) : (
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
                {sortedApps.map((app) => (
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
