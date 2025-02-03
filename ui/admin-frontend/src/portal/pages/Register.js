import React, { useState, useEffect } from "react";
import { useNavigate } from "react-router-dom";
import {
  Box,
  TextField,
  Button,
  Typography,
  Alert,
  FormHelperText,
} from "@mui/material";
import CheckCircleOutlineIcon from "@mui/icons-material/CheckCircleOutline";
import CancelOutlinedIcon from "@mui/icons-material/CancelOutlined";
import apiClient from "../../admin/utils/pubClient";

const Register = () => {
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState(null);
  const [passwordFocused, setPasswordFocused] = useState(false);
  const [passwordCriteria, setPasswordCriteria] = useState({
    length: false,
    number: false,
    special: false,
    uppercase: false,
  });
  const navigate = useNavigate();

  useEffect(() => {
    const checkPasswordCriteria = () => {
      setPasswordCriteria({
        length: password.length >= 8,
        number: /\d/.test(password),
        special: /[!@#$%^&*(),.?":{}|<>]/.test(password),
        uppercase: /[A-Z]/.test(password),
      });
    };
    checkPasswordCriteria();
  }, [password]);

  const handleSubmit = async (e) => {
    e.preventDefault();
    setError(null);
    if (!Object.values(passwordCriteria).every(Boolean)) {
      setError("Please ensure all password criteria are met.");
      return;
    }
    try {
      const response = await apiClient.post("/auth/register", {
        data: {
          type: "register",
          attributes: { name, email, password },
        },
      });
      if (response.data.message === "User registered successfully") {
        navigate("/login");
      }
    } catch (err) {
      console.error("Registration error:", err);
      if (err.response) {
        switch (err.response.status) {
          case 400:
            if (
              err.response.data.errors &&
              err.response.data.errors.length > 0
            ) {
              setError(err.response.data.errors[0].detail);
            } else {
              setError("Invalid registration data. Please check your input.");
            }
            break;
          case 409:
            setError("An account with this email already exists.");
            break;
          default:
            setError("An unexpected error occurred. Please try again.");
        }
      } else {
        setError("An unexpected error occurred. Please try again.");
      }
    }
  };

  const renderPasswordCriteria = () => (
    <Box sx={{ mt: 1 }}>
      {Object.entries(passwordCriteria).map(([criterion, isMet]) => (
        <FormHelperText key={criterion} error={!isMet}>
          {isMet ? (
            <CheckCircleOutlineIcon color="success" fontSize="small" />
          ) : (
            <CancelOutlinedIcon color="error" fontSize="small" />
          )}{" "}
          {criterion === "length"
            ? "At least 8 characters"
            : criterion === "number"
              ? "Contains a number"
              : criterion === "special"
                ? "Contains a special character"
                : "Contains an uppercase letter"}
        </FormHelperText>
      ))}
    </Box>
  );

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
          Register
        </Typography>
        {error && (
          <Alert severity="error" sx={{ mb: 2 }}>
            {error}
          </Alert>
        )}
        <form onSubmit={handleSubmit}>
          <TextField
            fullWidth
            label="Name"
            value={name}
            onChange={(e) => setName(e.target.value)}
            margin="normal"
            required
          />
          <TextField
            fullWidth
            label="Email"
            type="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            margin="normal"
            required
            autoComplete="off"
          />
          <TextField
            fullWidth
            label="Password"
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            onFocus={() => setPasswordFocused(true)}
            onBlur={() => setPasswordFocused(false)}
            margin="normal"
            required
            autoComplete="new-password"
          />
          {passwordFocused && renderPasswordCriteria()}
          <Button
            type="submit"
            variant="contained"
            color="primary"
            fullWidth
            sx={{ mt: 2 }}
          >
            Register
          </Button>
        </form>
      </Box>
    </Box>
  );
};

export default Register;
