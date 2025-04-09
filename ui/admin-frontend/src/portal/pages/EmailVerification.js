import React, { useEffect, useState } from "react";
import { useLocation } from "react-router-dom";
import { Box, Typography, Alert, CircularProgress } from "@mui/material";
import apiClient from "../../../admin/utils/apiClient";

const EmailVerification = () => {
  const [status, setStatus] = useState("verifying");
  const location = useLocation();

  useEffect(() => {
    const verifyEmail = async () => {
      const params = new URLSearchParams(location.search);
      const token = params.get("token");

      if (!token) {
        setStatus("error");
        return;
      }

      try {
        const response = await apiClient.get(
          `/auth/verify-email?token=${token}`,
        );
        if (
          response.data.data.attributes.message ===
          "Email verified successfully"
        ) {
          setStatus("success");
        } else {
          setStatus("error");
        }
      } catch (err) {
        setStatus("error");
      }
    };

    verifyEmail();
  }, [location]);

  return (
    <Box sx={{ maxWidth: 400, mx: "auto", textAlign: "center" }}>
      <Typography variant="h4" component="h1" gutterBottom>
        Email Verification
      </Typography>
      {status === "verifying" && (
        <Box sx={{ display: "flex", justifyContent: "center" }}>
          <CircularProgress />
        </Box>
      )}
      {status === "success" && (
        <Alert severity="success">
          Your email has been successfully verified!
        </Alert>
      )}
      {status === "error" && (
        <Alert severity="error">
          Failed to verify your email. Please try again or contact support.
        </Alert>
      )}
    </Box>
  );
};

export default EmailVerification;
