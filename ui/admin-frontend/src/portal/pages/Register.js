import React, { useState, useEffect } from "react";
import { useNavigate } from "react-router-dom";
import { Link as RouterLink } from "react-router-dom";
import {
  Box,
  Alert,
  Typography,
  FormHelperText,
  FormGroup,
} from "@mui/material";
import CheckCircleOutlineIcon from "@mui/icons-material/CheckCircleOutline";
import CancelOutlinedIcon from "@mui/icons-material/CancelOutlined";
import apiClient from "../../admin/utils/pubClient";
import { getConfig } from "../../config";
import AuthLayout from "./AuthLayout";
import { PrimaryButton } from "../../admin/styles/sharedStyles";
import {
  StyledTextField,
  FormLabel,
  FormLink,
  FormText,
  StyledCheckbox
} from "../styles/authStyles";
import CaptchaWidget from "../components/CaptchaWidget";
import useCaptcha from "../hooks/useCaptcha";


const Register = () => {
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [withPortal, setWithPortal] = useState(false);
  const [withChat, setWithChat] = useState(false);
  const [error, setError] = useState(null);
  const [passwordFocused, setPasswordFocused] = useState(false);
  const [signupMode, setSignupMode] = useState("both");
  const [passwordCriteria, setPasswordCriteria] = useState({
    length: false,
    number: false,
    special: false,
    uppercase: false,
  });
  const navigate = useNavigate();
  const { captchaConfig, setCaptchaToken, getToken } = useCaptcha();

  useEffect(() => {
    const config = getConfig() || {};
    const mode = config.DEFAULT_SIGNUP_MODE || "both";
    setSignupMode(mode);

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
    const meetsAllCriteria =
      password.length >= 8 &&
      /\d/.test(password) &&
      /[!@#$%^&*(),.?":{}|<>_+=-~]/.test(password) &&
      /[A-Z]/.test(password);
    if (!meetsAllCriteria) {
      setError("Please ensure all password criteria are met.");
      return;
    }

    if (signupMode === "both" && !withPortal && !withChat) {
      setError("Please select at least one option (Portal or Chat)");
      return;
    }

    try {
      const token = await getToken();
      const response = await apiClient.post("/auth/register", {
        captcha_token: token || undefined,
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
        <FormHelperText
          key={criterion}
          error={!isMet}
          sx={{
            display: 'flex',
            alignItems: 'center',
            color: isMet ? 'success.main' : 'error.main',
            ml: 0
          }}
        >
          {isMet ? (
            <CheckCircleOutlineIcon color="success" fontSize="small" sx={{ mr: 1 }} />
          ) : (
            <CancelOutlinedIcon color="error" fontSize="small" sx={{ mr: 1 }} />
          )}
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
    <AuthLayout>
      <Typography variant="headingXLarge" component="h1" gutterBottom align="center" color="text.primary">
        Create an account
      </Typography>
      
      {error && (
        <Alert severity="error" sx={{ mb: 2 }}>
          {error}
        </Alert>
      )}
      
      <form onSubmit={handleSubmit}>
        <Box mb={2} mt={2}>
          <FormLabel component="label" htmlFor="name">
            Name
          </FormLabel>
          <StyledTextField
            id="name"
            fullWidth
            value={name}
            onChange={(e) => setName(e.target.value)}
            required
            autoComplete="name"
            autoFocus
            variant="outlined"
          />
        </Box>
        
        <Box mb={2}>
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
            onFocus={() => setPasswordFocused(true)}
            onBlur={() => setPasswordFocused(false)}
            required
            autoComplete="new-password"
            variant="outlined"
          />
          {passwordFocused && renderPasswordCriteria()}
        </Box>
        
        {signupMode === "both" && (
          <FormGroup sx={{ mt: 2, mb: 3 }}>
            <StyledCheckbox
              checked={withPortal}
              onChange={(checked) => setWithPortal(checked)}
              label="Sign up for AI Portal"
            />
            <StyledCheckbox
              checked={withChat}
              onChange={(checked) => setWithChat(checked)}
              label="Sign up for AI Chats"
            />
          </FormGroup>
        )}
        
        {captchaConfig && (
          <CaptchaWidget
            provider={captchaConfig.provider}
            siteKey={captchaConfig.site_key}
            instanceUrl={captchaConfig.instance_url}
            onToken={setCaptchaToken}
          />
        )}

        <Box sx={{ display: 'flex', justifyContent: 'center', width: '100%', mb: 3 }}>
          <PrimaryButton
            type="submit"
            variant="contained"
          >
            Sign up
          </PrimaryButton>
        </Box>
      </form>
      
      <Box sx={{ textAlign: "center" }}>
        <FormText>
          Already a member?
          <FormLink component={RouterLink} to="/login" ml={1}>
            Log in
          </FormLink>
        </FormText>
      </Box>
    </AuthLayout>
  );
};

export default Register;
