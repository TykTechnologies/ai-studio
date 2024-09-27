import React, { useState, useEffect } from "react";
import { useParams, useNavigate } from "react-router-dom";
import {
  Typography,
  CircularProgress,
  Box,
  Grid,
  Button,
  Divider,
  Chip,
  Paper,
} from "@mui/material";
import ArrowBackIcon from "@mui/icons-material/ArrowBack";
import pubClient from "../../admin/utils/pubClient";

const SectionTitle = ({ children }) => (
  <Typography variant="h6" gutterBottom sx={{ mt: 3, mb: 2 }}>
    {children}
  </Typography>
);

const FieldLabel = ({ children }) => (
  <Typography variant="subtitle2" color="text.secondary">
    {children}
  </Typography>
);

const FieldValue = ({ children }) => (
  <Typography variant="body1">{children}</Typography>
);

const AppDetailView = () => {
  const [app, setApp] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const { id } = useParams();
  const navigate = useNavigate();

  useEffect(() => {
    const fetchAppDetails = async () => {
      try {
        const response = await pubClient.get(`/common/apps/${id}`);
        setApp(response.data);
        setLoading(false);
      } catch (err) {
        console.error("Error fetching app details:", err);
        setError("Failed to load app details. Please try again later.");
        setLoading(false);
      }
    };

    fetchAppDetails();
  }, [id]);

  if (loading) return <CircularProgress />;
  if (error) return <Typography color="error">{error}</Typography>;
  if (!app) return <Typography>App not found</Typography>;

  return (
    <Paper sx={{ p: 3 }}>
      <Box
        display="flex"
        justifyContent="space-between"
        alignItems="center"
        mb={3}
      >
        <Typography variant="h5">App Details</Typography>
        <Button
          startIcon={<ArrowBackIcon />}
          onClick={() => navigate("/portal/apps")}
        >
          Back to Apps
        </Button>
      </Box>

      <SectionTitle>App Information</SectionTitle>
      <Grid container spacing={2}>
        <Grid item xs={3}>
          <FieldLabel>Name:</FieldLabel>
        </Grid>
        <Grid item xs={9}>
          <FieldValue>{app.attributes.name}</FieldValue>
        </Grid>
        <Grid item xs={3}>
          <FieldLabel>Description:</FieldLabel>
        </Grid>
        <Grid item xs={9}>
          <FieldValue>{app.attributes.description}</FieldValue>
        </Grid>
        <Grid item xs={3}>
          <FieldLabel>Data Sources:</FieldLabel>
        </Grid>
        <Grid item xs={9}>
          <Box display="flex" flexWrap="wrap" gap={1}>
            {app.attributes.datasource_ids.map((id) => (
              <Chip key={id} label={`Data Source ${id}`} />
            ))}
          </Box>
        </Grid>
        <Grid item xs={3}>
          <FieldLabel>LLMs:</FieldLabel>
        </Grid>
        <Grid item xs={9}>
          <Box display="flex" flexWrap="wrap" gap={1}>
            {app.attributes.llm_ids.map((id) => (
              <Chip key={id} label={`LLM ${id}`} />
            ))}
          </Box>
        </Grid>
      </Grid>

      <Divider sx={{ my: 3 }} />

      <SectionTitle>Credential Information</SectionTitle>
      <Grid container spacing={2}>
        <Grid item xs={3}>
          <FieldLabel>Key ID:</FieldLabel>
        </Grid>
        <Grid item xs={9}>
          <FieldValue>{app.attributes.credential.keyID}</FieldValue>
        </Grid>
        <Grid item xs={3}>
          <FieldLabel>Secret:</FieldLabel>
        </Grid>
        <Grid item xs={9}>
          <FieldValue>{app.attributes.credential.secret}</FieldValue>
        </Grid>
        <Grid item xs={3}>
          <FieldLabel>Active:</FieldLabel>
        </Grid>
        <Grid item xs={9}>
          <FieldValue>
            {app.attributes.credential.active ? "Yes" : "No"}
          </FieldValue>
        </Grid>
      </Grid>

      <Box mt={4}>
        <Button
          variant="contained"
          color="primary"
          onClick={() => {
            /* Add logic to navigate to app editing page */
          }}
        >
          Edit App
        </Button>
      </Box>
    </Paper>
  );
};

export default AppDetailView;
