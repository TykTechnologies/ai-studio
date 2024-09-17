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
import { useNavigate } from "react-router-dom";

const UserForm = ({ user }) => {
  const [name, setName] = useState(user ? user.attributes.name : "");
  const [email, setEmail] = useState(user ? user.attributes.email : "");
  const [password, setPassword] = useState("");
  const [groups, setGroups] = useState([]);
  const [selectedGroup, setSelectedGroup] = useState('');
  const [error, setError] = useState("");
  const [successMessage, setSuccessMessage] = useState("");
  const navigate = useNavigate();

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
  }, []);

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
          password,
        },
      },
    };

    try {
      let userId;
      if (user) {
        await apiClient.patch(`/users/${user.id}`, userData);
        userId = user.id;
        setSuccessMessage("User updated successfully");
      } else {
        const response = await apiClient.post("/users", userData);
        userId = response.data.id;
        setSuccessMessage("User created successfully");
      }

      if (selectedGroup && userId) {
        await apiClient.post(`/groups/${selectedGroup}/users`, {
          data: {
            id: userId.toString(),
            type: "users"
          }
        });
        setSuccessMessage(prev => `${prev} and added to the selected group`);
      }

      setTimeout(() => navigate("/users"), 2000); // Navigate after 2 seconds
    } catch (error) {
      console.error("Error saving user", error);
      setError("Failed to save user. Please try again.");
    }
  };

  return (
    <Box component="form" onSubmit={handleSubmit} sx={{ maxWidth: 500 }}>
      {error && <Alert severity="error" sx={{ mb: 2 }}>{error}</Alert>}
      {successMessage && <Alert severity="success" sx={{ mb: 2 }}>{successMessage}</Alert>}
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
      {!user && (
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
        {user ? "Update User" : "Add User"}
      </Button>
    </Box>
  );
};

export default UserForm;
