import React, { useState, useEffect } from "react";
import { useNavigate, useLocation } from "react-router-dom";
import {
  Box,
  TextField,
  Button,
  Typography,
  Alert,
  Snackbar,
  FormHelperText,
} from "@mui/material";
import CheckCircleOutlineIcon from "@mui/icons-material/CheckCircleOutline";
import CancelOutlinedIcon from "@mui/icons-material/CancelOutlined";
import apiClient from "../../admin/utils/pubClient";

const ResetPassword = () => {
  const [password, setPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [token, setToken] = useState("");
  const [error, setError] = useState(null);
  const [successMessage, setSuccessMessage] = useState("");
  const [passwordFocused, setPasswordFocused] = useState(false);
  const [passwordCriteria, setPasswordCriteria] = useState({
    length: false,
    number: false,
    special: false,
    uppercase: false,
  });
  const navigate = useNavigate();
  const location = useLocation();

  useEffect(() => {
    const searchParams = new URLSearchParams(location.search);
    let tokenFromUrl = searchParams.get("token");

    if (!tokenFromUrl) {
      navigate("/forgot-password");
      return;
    }

    console.log("Found reset token:", tokenFromUrl); // Debug log
    setToken(tokenFromUrl);
    // Validate token immediately
    validateToken(tokenFromUrl);
  }, [location, navigate]);

  const validateToken = async (tokenToValidate) => {
    try {
      await apiClient.get(`/auth/validate-reset-token?token=${tokenToValidate}`);
      console.log("Token validated successfully");
    } catch (err) {
      setError(
        err.response?.data?.errors?.[0]?.detail ||
        "Invalid or expired reset token. Please request a new password reset."
      );
    }
  };

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
    if (password !== confirmPassword) {
      setError("Passwords do not match");
      return;
    }
    try {
      const response = await apiClient.post("/auth/reset-password", {
        data: {
          type: "reset-password",
          attributes: { token, password },
        },
      });
      setSuccessMessage(response.data.message);
      setTimeout(() => navigate("/login"), 3000); // Redirect to login after 3 seconds
    } catch (err) {
      console.error("Reset password error:", err);
      if (err.response) {
        switch (err.response.status) {
          case 400:
            if (
              err.response.data.errors &&
              err.response.data.errors.length > 0
            ) {
              setError(err.response.data.errors[0].detail);
            } else {
              setError("Invalid password reset data. Please check your input.");
            }
            break;
          case 401:
            setError(
              "Invalid or expired reset token. Please request a new password reset.",
            );
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

  const renderContent = () => {
    if (error) {
      return (
        <Box sx={{ textAlign: "center" }}>
          <Alert severity="error" sx={{ mb: 2 }}>
            {error}
          </Alert>
          <Button
            variant="contained"
            color="primary"
            onClick={() => navigate("/forgot-password")}
          >
            Request Password Reset
          </Button>
        </Box>
      );
    }

    if (!token) {
      return (
        <Box sx={{ textAlign: "center" }}>
          <Alert severity="warning" sx={{ mb: 2 }}>
            No reset token provided. Please request a password reset.
          </Alert>
          <Button
            variant="contained"
            color="primary"
            onClick={() => navigate("/forgot-password")}
          >
            Request Password Reset
          </Button>
        </Box>
      );
    }

    return (
      <form onSubmit={handleSubmit}>
        <TextField
          fullWidth
          label="New Password"
          type="password"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          onFocus={() => setPasswordFocused(true)}
          onBlur={() => setPasswordFocused(false)}
          margin="normal"
          required
        />
        {passwordFocused && renderPasswordCriteria()}
        <TextField
          fullWidth
          label="Confirm New Password"
          type="password"
          value={confirmPassword}
          onChange={(e) => setConfirmPassword(e.target.value)}
          margin="normal"
          required
          error={password !== confirmPassword && confirmPassword !== ""}
          helperText={
            password !== confirmPassword && confirmPassword !== ""
              ? "Passwords do not match"
              : ""
          }
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
    );
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
          Reset Password
        </Typography>
        {renderContent()}
        <Snackbar
          open={!!successMessage}
          autoHideDuration={3000}
          message={successMessage}
          anchorOrigin={{ vertical: "bottom", horizontal: "center" }}
        />
      </Box>
    </Box>
  );
};

export default ResetPassword;
