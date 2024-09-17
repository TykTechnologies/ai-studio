// src/Apps.js
import React, { useState, useEffect } from "react";
import apiClient from "../utils/apiClient";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
  Paper,
  Toolbar,
  Typography,
  Button,
  IconButton,
  CircularProgress,
  Alert,
} from "@mui/material";
import { Link } from "react-router-dom";
import DeleteIcon from "@mui/icons-material/Delete";

const Apps = () => {
  const [apps, setApps] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  useEffect(() => {
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

    fetchApps();
  }, []);

  const handleDelete = async (id) => {
    try {
      await apiClient.delete(`/apps/${id}`);
      setApps(apps.filter((app) => app.id !== id));
    } catch (error) {
      console.error("Error deleting app", error);
      setError("Failed to delete app");
    }
  };

  if (loading) {
    return <CircularProgress />;
  }

  if (error) {
    return <Alert severity="error">{error}</Alert>;
  }

  return (
    <Paper sx={{ width: "100%", overflow: "hidden" }}>
      <Toolbar>
        <Typography variant="h6" component="div" sx={{ flexGrow: 1 }}>
          Apps
        </Typography>
        <Button
          variant="contained"
          color="primary"
          component={Link}
          to="/apps/new"
        >
          Add App
        </Button>
      </Toolbar>
      <Table>
        <TableHead>
          <TableRow>
            <TableCell>ID</TableCell>
            <TableCell>Name</TableCell>
            <TableCell>Description</TableCell>
            <TableCell align="right">Actions</TableCell>
          </TableRow>
        </TableHead>
        <TableBody>
          {apps.length > 0 ? (
            apps.map((app) => (
              <TableRow key={app.id}>
                <TableCell>{app.id}</TableCell>
                <TableCell>{app.attributes.name}</TableCell>
                <TableCell>{app.attributes.description}</TableCell>
                <TableCell align="right">
                  <IconButton
                    color="secondary"
                    onClick={() => handleDelete(app.id)}
                  >
                    <DeleteIcon />
                  </IconButton>
                  {/* Add edit button if needed */}
                </TableCell>
              </TableRow>
            ))
          ) : (
            <TableRow>
              <TableCell colSpan={4}>No apps found</TableCell>
            </TableRow>
          )}
        </TableBody>
      </Table>
    </Paper>
  );
};

export default Apps;
