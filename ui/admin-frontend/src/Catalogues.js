// src/Catalogues.js
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

const Catalogues = () => {
  const [catalogues, setCatalogues] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  useEffect(() => {
    const fetchCatalogues = async () => {
      try {
        const response = await apiClient.get("/catalogues");
        setCatalogues(response.data.data || []);
        setLoading(false);
      } catch (error) {
        console.error("Error fetching catalogues", error);
        setError("Failed to load catalogues");
        setLoading(false);
      }
    };

    fetchCatalogues();
  }, []);

  const handleDelete = async (id) => {
    try {
      await apiClient.delete(`/catalogues/${id}`);
      setCatalogues(catalogues.filter((catalogue) => catalogue.id !== id));
    } catch (error) {
      console.error("Error deleting catalogue", error);
      setError("Failed to delete catalogue");
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
          Catalogues
        </Typography>
        <Button
          variant="contained"
          color="primary"
          component={Link}
          to="/catalogues/new"
        >
          Add Catalogue
        </Button>
      </Toolbar>
      <Table>
        <TableHead>
          <TableRow>
            <TableCell>ID</TableCell>
            <TableCell>Name</TableCell>
            {/* Add more columns if needed */}
            <TableCell align="right">Actions</TableCell>
          </TableRow>
        </TableHead>
        <TableBody>
          {catalogues.length > 0 ? (
            catalogues.map((catalogue) => (
              <TableRow key={catalogue.id}>
                <TableCell>{catalogue.id}</TableCell>
                <TableCell>{catalogue.attributes.name}</TableCell>
                {/* Add more cells if needed */}
                <TableCell align="right">
                  <IconButton
                    color="secondary"
                    onClick={() => handleDelete(catalogue.id)}
                  >
                    <DeleteIcon />
                  </IconButton>
                  {/* Add edit button if needed */}
                </TableCell>
              </TableRow>
            ))
          ) : (
            <TableRow>
              <TableCell colSpan={3}>No catalogues found</TableCell>
            </TableRow>
          )}
        </TableBody>
      </Table>
    </Paper>
  );
};

export default Catalogues;
