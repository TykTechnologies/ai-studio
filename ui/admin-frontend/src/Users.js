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
  Select,
  MenuItem,
  FormControl,
  InputLabel,
} from "@mui/material";
import { Link } from "react-router-dom";
import DeleteIcon from "@mui/icons-material/Delete";
import AddIcon from "@mui/icons-material/Add";

const Users = () => {
  const [users, setUsers] = useState([]);
  const [groups, setGroups] = useState([]);
  const [selectedGroup, setSelectedGroup] = useState('');
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  useEffect(() => {
    const fetchData = async () => {
      try {
        const [usersResponse, groupsResponse] = await Promise.all([
          apiClient.get("/users"),
          apiClient.get("/groups")
        ]);
        setUsers(usersResponse.data.data || []);
        setGroups(groupsResponse.data.data || []);
        setLoading(false);
      } catch (error) {
        console.error("Error fetching data", error);
        setError("Failed to load data");
        setLoading(false);
      }
    };

    fetchData();
  }, []);

  const handleAddUserToGroup = async (userId) => {
    if (!selectedGroup) {
      alert("Please select a group first");
      return;
    }

    try {
      await apiClient.post(`/groups/${selectedGroup}/users`, {
        data: {
          id: userId.toString(),
          type: "users"
        }
      });
      alert("User added to group successfully");
    } catch (error) {
      console.error("Error adding user to group", error);
      alert("Failed to add user to group");
    }
  };

  const handleDelete = async (id) => {
    try {
      await apiClient.delete(`/users/${id}`);
      setUsers(users.filter((user) => user.id !== id));
    } catch (error) {
      console.error("Error deleting user", error);
      setError("Failed to delete user");
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
          Users
        </Typography>
        <Button
          variant="contained"
          color="primary"
          component={Link}
          to="/users/new"
        >
          Add User
        </Button>
      </Toolbar>
      <Table>
        <TableHead>
          <TableRow>
            <TableCell>ID</TableCell>
            <TableCell>Name</TableCell>
            <TableCell>Email</TableCell>
            <TableCell align="right">Actions</TableCell>
            <TableCell>Add to Group</TableCell>
          </TableRow>
        </TableHead>
        <TableBody>
          {users.length > 0 ? (
            users.map((user) => (
              <TableRow key={user.id}>
                <TableCell>{user.id}</TableCell>
                <TableCell>{user.attributes.name}</TableCell>
                <TableCell>{user.attributes.email}</TableCell>
                <TableCell align="right">
                  <IconButton
                    color="secondary"
                    onClick={() => handleDelete(user.id)}
                  >
                    <DeleteIcon />
                  </IconButton>
                  {/* Add edit button if needed */}
                </TableCell>
                <TableCell>
                  <FormControl fullWidth>
                    <InputLabel>Group</InputLabel>
                    <Select
                      value={selectedGroup}
                      onChange={(e) => setSelectedGroup(e.target.value)}
                    >
                      {groups.map((group) => (
                        <MenuItem key={group.id} value={group.id}>
                          {group.attributes.name}
                        </MenuItem>
                      ))}
                    </Select>
                  </FormControl>
                  <IconButton
                    color="primary"
                    onClick={() => handleAddUserToGroup(user.id)}
                  >
                    <AddIcon />
                  </IconButton>
                </TableCell>
              </TableRow>
            ))
          ) : (
            <TableRow>
              <TableCell colSpan={5}>No users found</TableCell>
            </TableRow>
          )}
        </TableBody>
      </Table>
    </Paper>
  );
};

export default Users;
