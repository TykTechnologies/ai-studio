import React, { useState } from "react";
import { Box, TextField, Button, Typography, Alert } from "@mui/material";
import apiClient from "../../admin/utils/pubClient";

const ForgotPassword = () => {
  const [email, setEmail] = useState("");
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");

  const handleSubmit = async (e) => {
    e.preventDefault();
    try {
      const response = await apiClient.post("/forgot-password", {
        data: {
          type: "forgot-password",
          attributes: { email },
        },
      });
      setMessage(response.data.message);
    } catch (err) {
      setError("Failed to send reset password email. Please try again.");
    }
  };

  return (
    <Box sx={{ maxWidth: 400, mx: "auto" }}>
      <Typography variant="h4" component="h1" gutterBottom>
        Forgot Password
      </Typography>
      {error && (
        <Alert severity="error" sx={{ mb: 2 }}>
          {error}
        </Alert>
      )}
      {message && (
        <Alert severity="success" sx={{ mb: 2 }}>
          {message}
        </Alert>
      )}
      <form onSubmit={handleSubmit}>
        <TextField
          fullWidth
          label="Email"
          type="email"
          value={email}
          onChange={(e) => setEmail(e.target.value)}
          margin="normal"
          required
        />
        <Button
          type="submit"
          variant="contained"
          color="primary"
          fullWidth
          sx={{ mt: 2 }}
        >
          Reset Password
        </Button>
      </form>
    </Box>
  );
};

export default ForgotPassword;
