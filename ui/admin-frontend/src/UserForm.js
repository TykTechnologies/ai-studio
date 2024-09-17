import React, { useState, useEffect } from "react";
import apiClient from "./apiClient";
import {
  TextField,
  Button,
  Box,
  Select,
  MenuItem,
  FormControl,
  InputLabel,
  Alert,
} from "@mui/material";
import { useNavigate, useParams } from "react-router-dom";

const UserForm = () => {
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [groups, setGroups] = useState([]);
  const [selectedGroup, setSelectedGroup] = useState("");
  const [error, setError] = useState("");
  const [successMessage, setSuccessMessage] = useState("");
  const navigate = useNavigate();
  const { id } = useParams(); // Get the user ID from URL if editing

  useEffect(() => {
    const fetchGroups = async () => {
      try {
        const response = await apiClient.get("/groups");
        setGroups(response.data.data || []);
      } catch (error) {
        console.error("Error fetching groups", error);
      }
    };

    fetchGroups();

    // If editing, fetch user details
    if (id) {
      const fetchUser = async () => {
        try {
          const response = await apiClient.get(`/users/${id}`);
          const userData = response.data.data;
          setName(userData.attributes.name);
          setEmail(userData.attributes.email);
          // Don't set password for editing
        } catch (error) {
          console.error("Error fetching user", error);
          setError("Failed to fetch user details");
        }
      };
      fetchUser();
    }
  }, [id]);

  const handleSubmit = async (e) => {
    e.preventDefault();
    setError("");
    setSuccessMessage("");

    const userData = {
      data: {
        type: "User",
        attributes: {
          name,
          email,
          ...(password && { password }), // Only include password if it's set (for new users)
        },
      },
    };

    try {
      let response;
      if (id) {
        response = await apiClient.patch(`/users/${id}`, userData);
        setSuccessMessage("User updated successfully");
      } else {
        response = await apiClient.post("/users", userData);
        setSuccessMessage("User created successfully");
      }

      const userId = response.data.data.id;

      if (selectedGroup) {
        await apiClient.post(`/groups/${selectedGroup}/users`, {
          data: {
            id: userId.toString(),
            type: "users",
          },
        });
        setSuccessMessage((prev) => `${prev} and added to the selected group`);
      }

      setTimeout(() => navigate("/users"), 2000); // Navigate after 2 seconds
    } catch (error) {
      console.error("Error saving user", error);
      setError("Failed to save user. Please try again.");
    }
  };

  return (
    <Box component="form" onSubmit={handleSubmit} sx={{ maxWidth: 500 }}>
      {error && (
        <Alert severity="error" sx={{ mb: 2 }}>
          {error}
        </Alert>
      )}
      {successMessage && (
        <Alert severity="success" sx={{ mb: 2 }}>
          {successMessage}
        </Alert>
      )}
      <TextField
        fullWidth
        margin="normal"
        label="Name"
        value={name}
        onChange={(e) => setName(e.target.value)}
        required
      />
      <TextField
        fullWidth
        margin="normal"
        label="Email"
        type="email"
        value={email}
        onChange={(e) => setEmail(e.target.value)}
        required
      />
      {!id && ( // Only show password field for new users
        <TextField
          fullWidth
          margin="normal"
          label="Password"
          type="password"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          required
        />
      )}
      <FormControl fullWidth margin="normal">
        <InputLabel>Group</InputLabel>
        <Select
          value={selectedGroup}
          onChange={(e) => setSelectedGroup(e.target.value)}
        >
          <MenuItem value="">
            <em>None</em>
          </MenuItem>
          {groups.map((group) => (
            <MenuItem key={group.id} value={group.id}>
              {group.attributes.name}
            </MenuItem>
          ))}
        </Select>
      </FormControl>
      <Button variant="contained" color="primary" type="submit" sx={{ mt: 2 }}>
        {id ? "Update User" : "Add User"}
      </Button>
    </Box>
  );
};

export default UserForm;
