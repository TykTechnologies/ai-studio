import React, { useState, useEffect } from "react";
import { useNavigate } from "react-router-dom";
import {
  Box,
  TextField,
  Button,
  Typography,
  Alert,
  FormHelperText,
  FormControlLabel,
  Checkbox,
  FormGroup,
} from "@mui/material";
import CheckCircleOutlineIcon from "@mui/icons-material/CheckCircleOutline";
import CancelOutlinedIcon from "@mui/icons-material/CancelOutlined";
import apiClient from "../../admin/utils/pubClient";
import { getConfig } from "../../config";

const Register = () => {
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [withPortal, setWithPortal] = useState(false);
  const [withChat, setWithChat] = useState(false);
  const [error, setError] = useState(null);
  const [passwordFocused, setPasswordFocused] = useState(false);
  const [signupMode, setSignupMode] = useState("both"); // Default value
  const [passwordCriteria, setPasswordCriteria] = useState({
    length: false,
    number: false,
    special: false,
    uppercase: false,
  });
  const navigate = useNavigate();

  useEffect(() => {
    // Get the signup mode from config
    const config = getConfig();
    const mode = config.DEFAULT_SIGNUP_MODE || "both";
    setSignupMode(mode);

    // Set default values based on mode
    switch (mode) {
      case "portal":
        setWithPortal(true);
        setWithChat(false);
        break;
      case "chat":
        setWithPortal(false);
        setWithChat(true);
        break;
      case "both":
        setWithPortal(true);
        setWithChat(true);
        break;
      default:
        // Use "both" as fallback
        break;
    }
  }, []);

  useEffect(() => {
    const checkPasswordCriteria = () => {
      setPasswordCriteria({
        length: password.length >= 8,
        number: /\d/.test(password),
        special: /[!@#$%^&*(),.?":{}|<>_+=-~]/.test(password),
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

    // Validate that at least one option is selected when mode is "both"
    if (signupMode === "both" && !withPortal && !withChat) {
      setError("Please select at least one option (Portal or Chat)");
      return;
    }

    try {
      const response = await apiClient.post("/auth/register", {
        data: {
          type: "register",
          attributes: {
            name,
            email,
            password,
            with_portal: signupMode === "portal" ? true : withPortal,
            with_chat: signupMode === "chat" ? true : withChat,
          },
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
            autoComplete="off"
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
          />
          {passwordFocused && renderPasswordCriteria()}

          {/* Only show checkboxes if mode is "both" */}
          {signupMode === "both" && (
            <FormGroup sx={{ mt: 2 }}>
              <FormControlLabel
                control={
                  <Checkbox
                    checked={withPortal}
                    onChange={(e) => setWithPortal(e.target.checked)}
                  />
                }
                label="Sign up for AI Developer Portal"
              />
              <FormControlLabel
                control={
                  <Checkbox
                    checked={withChat}
                    onChange={(e) => setWithChat(e.target.checked)}
                  />
                }
                label="Sign up for AI Chat"
              />
            </FormGroup>
          )}

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
