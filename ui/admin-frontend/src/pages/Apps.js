import React, { useState, useEffect } from "react";
import apiClient from "../utils/apiClient";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
  Typography,
  Button,
  IconButton,
  CircularProgress,
  Alert,
  Menu,
  MenuItem,
  Box,
} from "@mui/material";
import { Link, useNavigate } from "react-router-dom";
import MoreVertIcon from "@mui/icons-material/MoreVert";
import {
  StyledPaper,
  TitleBox,
  ContentBox,
  StyledTableCell,
  StyledTableRow,
} from "../styles/sharedStyles";

const Apps = () => {
  const [apps, setApps] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const navigate = useNavigate();
  const [anchorEl, setAnchorEl] = useState(null);
  const [selectedApp, setSelectedApp] = useState(null);

  useEffect(() => {
    fetchApps();
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

  const handleMenuOpen = (event, app) => {
    event.stopPropagation();
    setAnchorEl(event.currentTarget);
    setSelectedApp(app);
  };

  const handleMenuClose = () => {
    setAnchorEl(null);
    setSelectedApp(null);
  };

  const handleDelete = async () => {
    try {
      await apiClient.delete(`/apps/${selectedApp.id}`);
      setApps(apps.filter((app) => app.id !== selectedApp.id));
      handleMenuClose();
    } catch (error) {
      console.error("Error deleting app", error);
      setError("Failed to delete app");
    }
  };

  const handleEdit = () => {
    navigate(`/apps/edit/${selectedApp.id}`);
    handleMenuClose();
  };

  const handleDetails = (app) => {
    navigate(`/apps/${app.id}`);
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
          <Typography variant="h5" color="white" sx={{ fontWeight: "bold" }}>
            Apps
          </Typography>
          <Button
            variant="contained"
            color="secondary"
            component={Link}
            to="/apps/new"
            sx={{ borderRadius: 20 }}
          >
            Add App
          </Button>
        </TitleBox>
        <ContentBox>
          <Table>
            <TableHead>
              <TableRow>
                <StyledTableCell>ID</StyledTableCell>
                <StyledTableCell>Name</StyledTableCell>
                <StyledTableCell>Description</StyledTableCell>
                <StyledTableCell align="right">Actions</StyledTableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {apps.length > 0 ? (
                apps.map((app) => (
                  <StyledTableRow
                    key={app.id}
                    onClick={() => handleDetails(app)}
                    sx={{ cursor: "pointer" }}
                  >
                    <TableCell>{app.id}</TableCell>
                    <TableCell>{app.attributes.name}</TableCell>
                    <TableCell>{app.attributes.description}</TableCell>
                    <TableCell align="right">
                      <IconButton
                        onClick={(event) => handleMenuOpen(event, app)}
                      >
                        <MoreVertIcon />
                      </IconButton>
                    </TableCell>
                  </StyledTableRow>
                ))
              ) : (
                <TableRow>
                  <TableCell colSpan={4}>No apps found</TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        </ContentBox>
      </StyledPaper>
      <Menu
        anchorEl={anchorEl}
        open={Boolean(anchorEl)}
        onClose={handleMenuClose}
      >
        <MenuItem onClick={handleEdit}>Edit</MenuItem>
        <MenuItem onClick={handleDelete}>Delete</MenuItem>
      </Menu>
    </Box>
  );
};

export default Apps;
