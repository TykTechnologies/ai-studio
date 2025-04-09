import React, { useState, useEffect } from "react";
import { Box, Alert, Typography, useTheme } from "@mui/material";
import { Link as RouterLink } from "react-router-dom";
import pubClient from "../../admin/utils/pubClient";
import AuthLayout from "./AuthLayout";
import { PrimaryButton } from "../../admin/styles/sharedStyles";
import {
  StyledTextField,
  FormLabel,
  FormLink,
  FormText
} from "../styles/authStyles";

const Login = () => {
  const theme = useTheme();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState(null);
  const [ssoEnabled, setSSOEnabled] = useState(false);
  const [ssoProfile, setSSOProfile] = useState(null);

  useEffect(() => {
    const fetchSSOConfig = async () => {
      try {
        const configResponse = await pubClient.get("/auth/config");
        const tibEnabled = configResponse.data.tibEnabled;
        setSSOEnabled(tibEnabled);
        
        if (tibEnabled) {
          const profileResponse = await pubClient.get("/login-sso-profile");
          if (profileResponse.data?.data) {
            setSSOProfile(profileResponse.data.data);
          }
        }
      } catch (err) {
        console.error("Error fetching SSO config:", err);
      }
    };
    
    fetchSSOConfig();
  }, []);

  const handleSubmit = async (e) => {
    e.preventDefault();
    setError(null);
    try {
      const loginResponse = await pubClient.post("/auth/login", {
        data: {
          type: "login",
          attributes: { email, password },
        },
      });

      if (loginResponse.data.message === "Login successful") {
        // Get user entitlements to determine where to redirect
        const userResponse = await pubClient.get("/common/me");
        const { ui_options } = userResponse.data.attributes;

        // Determine which dashboard to show based on permissions
        if (ui_options?.show_portal) {
          window.location.href = "/portal/dashboard";
        } else if (ui_options?.show_chat) {
          window.location.href = "/chat/dashboard";
        } else {
          setError("Your account doesn't have access to any features.");
        }
      }
    } catch (err) {
      console.error("Login error:", err);
      if (err.response) {
        if (err.response.data && err.response.data.error) {
          setError(err.response.data.error);
        } else if (
          err.response.data.errors &&
          err.response.data.errors.length > 0
        ) {
          setError(err.response.data.errors[0].detail);
        } else {
          setError("An unexpected error occurred. Please try again.");
        }
      } else {
        setError("An unexpected error occurred. Please try again.");
      }
    }
  };

  return (
    <AuthLayout>
      <Typography variant="headingXLarge" component="h1" gutterBottom align="center" color="text.primary">
        Log in to your account
      </Typography>
      
      {error && (
        <Alert severity="error" sx={{ mb: 2 }}>
          {error}
        </Alert>
      )}
      
      <form onSubmit={handleSubmit}>
        <Box mb={2} mt={2}>
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
            autoComplete="username"
            autoFocus
            variant="outlined"
          />
        </Box>
        
        <Box mb={3}>
          <FormLabel component="label" htmlFor="password">
            Password
          </FormLabel>
          <StyledTextField
            id="password"
            fullWidth
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            required
            autoComplete="current-password"
            variant="outlined"
          />
        </Box>
        
        <Box sx={{ display: 'flex', justifyContent: 'center', width: '100%', mb: 3 }}>
          <PrimaryButton
            type="submit"
            variant="contained"
          >
            Log in
          </PrimaryButton>
        </Box>
      </form>
      
      <Box sx={{ textAlign: "center" }}>
        <FormText sx={{ mb: 1 }}>
          Don't have an account?
          <FormLink component={RouterLink} to="/register" ml={1}>
            Sign up
          </FormLink>
        </FormText>
        
        <FormLink component={RouterLink} to="/forgot-password">
          Forgot password?
        </FormLink>
      </Box>
      
      {ssoEnabled && ssoProfile && (
        <>
          <Box
            sx={{
              display: 'flex',
              alignItems: 'center',
              mt: 5,
              mb: 3,
              "&::before, &::after": {
                content: '""',
                flex: 1,
                borderBottom: `1px solid ${theme.palette.background.buttonPrimaryOutlineHover}`
              }
            }}
          >
            <Typography
              variant="headingSmall"
              color={theme.palette.custom.white}
              sx={{ mx: 2 }}
            >
              OR
            </Typography>
          </Box>
          
          <Box sx={{ display: 'flex', justifyContent: 'center', width: '100%' }}>
            <PrimaryButton
              component="a"
              href={ssoProfile.attributes.login_url
              }
              variant="contained"
            >
              Log in with SSO
            </PrimaryButton>
          </Box>
        </>
      )}
    </AuthLayout>
  );
};

export default Login;
