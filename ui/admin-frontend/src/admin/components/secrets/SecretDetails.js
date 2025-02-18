import React, { useState, useEffect, useCallback } from "react";
import { useParams, useNavigate, Link as RouterLink, NavLink } from "react-router-dom";
import apiClient from "../../utils/apiClient";
import {
  Typography,
  CircularProgress,
  Grid,
  Button,
  IconButton,
  Box,
  Alert,
  Snackbar,
  Link
} from "@mui/material";
import EditIcon from "@mui/icons-material/Edit";
import ArrowBackIcon from "@mui/icons-material/ArrowBack";
import VisibilityIcon from "@mui/icons-material/Visibility";
import VisibilityOffIcon from "@mui/icons-material/VisibilityOff";
import {
  StyledPaper,
  TitleBox,
  ContentBox,
  FieldLabel,
  FieldValue,
  StyledButton,
} from "../../styles/sharedStyles";

const SecretDetails = () => {
  const [secret, setSecret] = useState(null);
  const [loading, setLoading] = useState(true);
  const [showSecret, setShowSecret] = useState(false);
  const [snackbar, setSnackbar] = useState({
    open: false,
    message: "",
    severity: "success",
  });
  const { id } = useParams();
  const navigate = useNavigate();

  const fetchSecretDetails = useCallback(async () => {
    try {
      setLoading(true);
      const response = await apiClient.get(`/secrets/${id}`);
      setSecret(response.data.data);
    } catch (error) {
      console.error("Error fetching secret details", error);
      setSnackbar({
        open: true,
        message: "Failed to fetch secret details",
        severity: "error",
      });
    } finally {
      setLoading(false);
    }
  }, [id]);

  useEffect(() => {
    fetchSecretDetails();
  }, [fetchSecretDetails]);

  const handleCloseSnackbar = (event, reason) => {
    if (reason === "clickaway") {
      return;
    }
    setSnackbar({ ...snackbar, open: false });
  };

  const toggleSecretVisibility = () => {
    setShowSecret(!showSecret);
  };

  const formatSecretValue = (value) => {
    if (!value) return "";
    return showSecret ? value : "•".repeat(12);
  };

  if (loading || !secret) return <CircularProgress />;

  return (
    <>
      <TitleBox top="64px">
        <Typography variant="h5">Secret Details</Typography>
        <Box>
          <Link component={NavLink} to="/admin/secrets">
            <ArrowBackIcon sx={{ mr: 1 }} />
            Back to Secrets
          </Link>
          <StyledButton
            variant="contained"
            startIcon={<EditIcon />}
            onClick={() => navigate(`/admin/secrets/edit/${id}`)}
          >
            Edit Secret
          </StyledButton>
        </Box>
      </TitleBox>
      <ContentBox>
        <Grid container spacing={3}>
          <Grid item xs={12} md={6}>
            <Grid container spacing={2}>
              <Grid item xs={4}>
                <FieldLabel>Variable Name:</FieldLabel>
              </Grid>
              <Grid item xs={8}>
                <FieldValue>{secret.attributes.var_name}</FieldValue>
              </Grid>

              <Grid item xs={4}>
                <FieldLabel>Value:</FieldLabel>
              </Grid>
              <Grid item xs={8}>
                <Box display="flex" alignItems="center">
                  <FieldValue
                    sx={{
                      fontFamily: showSecret ? "inherit" : "monospace",
                      letterSpacing: showSecret ? "inherit" : "0.1em",
                    }}
                  >
                    {formatSecretValue(secret.attributes.value)}
                  </FieldValue>
                  <IconButton
                    onClick={toggleSecretVisibility}
                    size="small"
                    sx={{ ml: 1 }}
                  >
                    {showSecret ? <VisibilityOffIcon /> : <VisibilityIcon />}
                  </IconButton>
                </Box>
              </Grid>
            </Grid>
          </Grid>
        </Grid>

        <Box mt={4}>
          <Alert severity="info">
            <Typography variant="body2">
              This secret can be referenced in configurations using:{" "}
              <code>$SECRET/{secret.attributes.var_name}</code>
            </Typography>
          </Alert>
        </Box>
      </ContentBox>

      <Snackbar
        open={snackbar.open}
        autoHideDuration={6000}
        onClose={handleCloseSnackbar}
        anchorOrigin={{ vertical: "bottom", horizontal: "center" }}
      >
        <Alert
          onClose={handleCloseSnackbar}
          severity={snackbar.severity}
          sx={{ width: "100%" }}
        >
          {snackbar.message}
        </Alert>
      </Snackbar>
    </>
  );
};

export default SecretDetails;
