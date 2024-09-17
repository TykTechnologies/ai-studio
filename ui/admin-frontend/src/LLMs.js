// src/LLMs.js
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

const LLMs = () => {
  const [llms, setLLMs] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  useEffect(() => {
    const fetchLLMs = async () => {
      try {
        const response = await apiClient.get("/llms");
        setLLMs(response.data.data || []);
        setLoading(false);
      } catch (error) {
        console.error("Error fetching LLMs", error);
        setError("Failed to load LLMs");
        setLoading(false);
      }
    };

    fetchLLMs();
  }, []);

  const handleDelete = async (id) => {
    try {
      await apiClient.delete(`/llms/${id}`);
      setLLMs(llms.filter((llm) => llm.id !== id));
    } catch (error) {
      console.error("Error deleting LLM", error);
      setError("Failed to delete LLM");
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
          LLMs
        </Typography>
        <Button
          variant="contained"
          color="primary"
          component={Link}
          to="/llms/new"
        >
          Add LLM
        </Button>
      </Toolbar>
      <Table>
        <TableHead>
          <TableRow>
            <TableCell>ID</TableCell>
            <TableCell>Name</TableCell>
            <TableCell>Vendor</TableCell>
            {/* Add more columns if needed */}
            <TableCell align="right">Actions</TableCell>
          </TableRow>
        </TableHead>
        <TableBody>
          {llms.length > 0 ? (
            llms.map((llm) => (
              <TableRow key={llm.id}>
                <TableCell>{llm.id}</TableCell>
                <TableCell>{llm.attributes.name}</TableCell>
                <TableCell>{llm.attributes.vendor}</TableCell>
                {/* Add more cells if needed */}
                <TableCell align="right">
                  <IconButton
                    color="secondary"
                    onClick={() => handleDelete(llm.id)}
                  >
                    <DeleteIcon />
                  </IconButton>
                  {/* Add edit button if needed */}
                </TableCell>
              </TableRow>
            ))
          ) : (
            <TableRow>
              <TableCell colSpan={4}>No LLMs found</TableCell>
            </TableRow>
          )}
        </TableBody>
      </Table>
    </Paper>
  );
};

export default LLMs;
