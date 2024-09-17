// src/Datasources.js
import React, { useState, useEffect } from "react";
import apiClient from "./apiClient";
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

const Datasources = () => {
  const [datasources, setDatasources] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  useEffect(() => {
    const fetchDatasources = async () => {
      try {
        const response = await apiClient.get("/datasources");
        setDatasources(response.data);
        setLoading(false);
      } catch (error) {
        console.error("Error fetching datasources", error);
        setError("Failed to load datasources");
        setLoading(false);
      }
    };

    fetchDatasources();
  }, []);

  const handleDelete = async (id) => {
    try {
      await apiClient.delete(`/datasources/${id}`);
      setDatasources(datasources.filter((ds) => ds.id !== id));
    } catch (error) {
      console.error("Error deleting datasource", error);
      setError("Failed to delete datasource");
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
          Datasources
        </Typography>
        <Button
          variant="contained"
          color="primary"
          component={Link}
          to="/datasources/new"
        >
          Add Datasource
        </Button>
      </Toolbar>
      <Table>
        <TableHead>
          <TableRow>
            <TableCell>ID</TableCell>
            <TableCell>Name</TableCell>
            <TableCell>Type</TableCell>
            {/* Add more columns if needed */}
            <TableCell align="right">Actions</TableCell>
          </TableRow>
        </TableHead>
        <TableBody>
          {datasources.map((ds) => (
            <TableRow key={ds.id}>
              <TableCell>{ds.id}</TableCell>
              <TableCell>{ds.attributes.name}</TableCell>
              <TableCell>{ds.attributes.db_source_type}</TableCell>
              {/* Add more cells if needed */}
              <TableCell align="right">
                <IconButton
                  color="secondary"
                  onClick={() => handleDelete(ds.id)}
                >
                  <DeleteIcon />
                </IconButton>
                {/* Add edit button if needed */}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </Paper>
  );
};

export default Datasources;
