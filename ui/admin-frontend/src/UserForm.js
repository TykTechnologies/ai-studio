// src/UserForm.js
import React, { useState } from "react";
import apiClient from "./apiClient";
import { TextField, Button, Box } from "@mui/material";
import { useNavigate } from "react-router-dom";

const UserForm = ({ user }) => {
  const [name, setName] = useState(user ? user.attributes.name : "");
  const [email, setEmail] = useState(user ? user.attributes.email : "");
  const [password, setPassword] = useState("");
  const navigate = useNavigate();

  const handleSubmit = async (e) => {
    e.preventDefault();

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
      if (user) {
        // Update existing user
        await apiClient.patch(`/users/${user.id}`, userData);
      } else {
        // Create new user
        await apiClient.post("/users", userData);
      }
      navigate("/users");
    } catch (error) {
      console.error("Error saving user", error);
    }
  };

  return (
    <Box component="form" onSubmit={handleSubmit} sx={{ maxWidth: 500 }}>
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
      <Button variant="contained" color="primary" type="submit" sx={{ mt: 2 }}>
        {user ? "Update User" : "Add User"}
      </Button>
    </Box>
  );
};

export default UserForm;
