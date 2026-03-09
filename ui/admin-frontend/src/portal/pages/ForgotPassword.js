import React, { useState } from "react";
import { Box, Typography, Alert } from "@mui/material";
import { Link as RouterLink } from "react-router-dom";
import apiClient from "../../admin/utils/pubClient";
import AuthLayout from "./AuthLayout";
import { PrimaryButton } from "../../admin/styles/sharedStyles";
import {
  StyledTextField,
  FormLabel,
  FormLink
} from "../styles/authStyles";
import CaptchaWidget from "../components/CaptchaWidget";
import useCaptcha from "../hooks/useCaptcha";

const ForgotPassword = () => {
  const [email, setEmail] = useState("");
  const [error, setError] = useState("");
  const [isSuccess, setIsSuccess] = useState(false);
  const { captchaConfig, setCaptchaToken, getToken } = useCaptcha();

  const handleSubmit = async (e) => {
    e.preventDefault();
    setError("");
    try {
      const token = await getToken();
      await apiClient.post("/auth/forgot-password", {
        captcha_token: token || undefined,
        data: {
          type: "forgot-password",
          attributes: { email },
        },
      });
      setIsSuccess(true);
    } catch (err) {
      setError("Failed to send reset password email. Please try again.");
    }
  };

  if (isSuccess) {
    return (
      <AuthLayout>
        <Typography variant="headingXLarge" component="h1" gutterBottom align="center" color="text.primary">
          Password reset request sent
        </Typography>
        
        <Typography variant="bodyLargeMedium" align="center" color="text.primary" sx={{ mb: 4, mt: 2 }}>
          Success! If we can find an account with that email address, we will send
          you instructions on how to reset your password.
        </Typography>
        
        <Box sx={{ display: 'flex', justifyContent: 'center', width: '100%', mb: 3 }}>
          <PrimaryButton
            component={RouterLink}
            to="/login"
            variant="contained"
          >
            Go to Login
          </PrimaryButton>
        </Box>
      </AuthLayout>
    );
  }

  return (
    <AuthLayout>
      <Typography variant="headingXLarge" component="h1" gutterBottom align="center" color="text.primary">
        Password reset
      </Typography>
      
      <Typography variant="bodyLargeMedium" align="center" color="text.primary" sx={{ m: 2}}>
        We will send you a link you can use to reset your password securely
      </Typography>
      
      {error && (
        <Alert severity="error" sx={{ mb: 2 }}>
          {error}
        </Alert>
      )}
      
      <form onSubmit={handleSubmit}>
        <Box mb={3} mt={2}>
          <FormLabel component="label" htmlFor="email">
            Email address
          </FormLabel>
          <StyledTextField
            id="email"
            fullWidth
            type="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            required
            autoComplete="email"
            autoFocus
            variant="outlined"
          />
        </Box>
        
        {captchaConfig && (
          <CaptchaWidget
            provider={captchaConfig.provider}
            siteKey={captchaConfig.site_key}
            onToken={setCaptchaToken}
          />
        )}

        <Box sx={{ display: 'flex', justifyContent: 'center', width: '100%', mb: 3 }}>
          <PrimaryButton
            type="submit"
            variant="contained"
          >
            Reset password
          </PrimaryButton>
        </Box>
      </form>
      
      <Box sx={{ textAlign: "center" }}>
        <FormLink component={RouterLink} to="/login">
          Return to Login
        </FormLink>
      </Box>
    </AuthLayout>
  );
};

export default ForgotPassword;
