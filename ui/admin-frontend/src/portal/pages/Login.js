import React, { useState } from "react";
import { Box, TextField, Button, Typography, Alert, Link } from "@mui/material";
import { Link as RouterLink } from "react-router-dom";
import pubClient from "../../admin/utils/pubClient";

const Login = () => {
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");

  const handleSubmit = async (e) => {
    e.preventDefault();
    setError("");
    try {
      const response = await pubClient.post("/auth/login", {
        data: {
          type: "login",
          attributes: { email, password },
        },
      });

      if (response.data.message === "Login successful") {
        window.location.reload(); // Force a reload to trigger the /me check in App.js
      }
    } catch (err) {
      console.error("Login error:", err);
      setError("Invalid email or password");
    }
  };

  return (
    <Box
      sx={{
        display: "flex",
        flexDirection: "column",
        alignItems: "center",
        justifyContent: "center",
        minHeight: "100vh",
      }}
    >
      <Box sx={{ maxWidth: 400, width: "100%", p: 3 }}>
        <Typography variant="h4" component="h1" gutterBottom align="center">
          Login
        </Typography>
        {error && (
          <Alert severity="error" sx={{ mb: 2 }}>
            {error}
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
          <TextField
            fullWidth
            label="Password"
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
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
            Login
          </Button>
        </form>
        <Box sx={{ mt: 2, textAlign: "center" }}>
          <Typography variant="body2">
            Don't have an account?{" "}
            <Link component={RouterLink} to="/register">
              Register here
            </Link>
          </Typography>
        </Box>
        <Box sx={{ mt: 1, textAlign: "center" }}>
          <Typography variant="body2">
            <Link component={RouterLink} to="/forgot-password">
              Forgot password?
            </Link>
          </Typography>
        </Box>
      </Box>
    </Box>
  );
};

export default Login;
