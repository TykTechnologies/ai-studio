// src/Groups.js
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

const Groups = () => {
  const [groups, setGroups] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  useEffect(() => {
    const fetchGroups = async () => {
      try {
        const response = await apiClient.get("/groups");
        setGroups(Array.isArray(response.data) ? response.data : []);
        setLoading(false);
      } catch (error) {
        console.error("Error fetching groups", error);
        setError("Failed to load groups");
        setLoading(false);
      }
    };

    fetchGroups();
  }, []);

  const handleDelete = async (id) => {
    try {
      await apiClient.delete(`/groups/${id}`);
      setGroups(groups.filter((group) => group.id !== id));
    } catch (error) {
      console.error("Error deleting group", error);
      setError("Failed to delete group");
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
          Groups
        </Typography>
        <Button
          variant="contained"
          color="primary"
          component={Link}
          to="/groups/new"
        >
          Add Group
        </Button>
      </Toolbar>
      <Table>
        <TableHead>
          <TableRow>
            <TableCell>ID</TableCell>
            <TableCell>Name</TableCell>
            <TableCell align="right">Actions</TableCell>
          </TableRow>
        </TableHead>
        <TableBody>
          {Array.isArray(groups) && groups.length > 0 ? (
            groups.map((group) => (
              <TableRow key={group.id}>
                <TableCell>{group.id}</TableCell>
                <TableCell>{group.attributes.name}</TableCell>
                <TableCell align="right">
                  {/* Add edit and delete buttons */}
                  <IconButton
                    color="secondary"
                    onClick={() => handleDelete(group.id)}
                  >
                    <DeleteIcon />
                  </IconButton>
                  {/* Add more actions if needed */}
                </TableCell>
              </TableRow>
            ))
          ) : (
            <TableRow>
              <TableCell colSpan={3}>No groups found</TableCell>
            </TableRow>
          )}
        </TableBody>
      </Table>
    </Paper>
  );
};

export default Groups;
