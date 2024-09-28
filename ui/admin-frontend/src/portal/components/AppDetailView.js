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
  Dialog,
  DialogActions,
  DialogContent,
  DialogContentText,
  DialogTitle,
} from "@mui/material";

import { IconButton } from "@mui/material";

import ArrowBackIcon from "@mui/icons-material/ArrowBack";
import DeleteIcon from "@mui/icons-material/Delete";

import VisibilityIcon from "@mui/icons-material/Visibility";
import VisibilityOffIcon from "@mui/icons-material/VisibilityOff";
import ContentCopyIcon from "@mui/icons-material/ContentCopy";

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
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);

  const [showSecret, setShowSecret] = useState(false);

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

  const toggleSecretVisibility = () => {
    setShowSecret(!showSecret);
  };

  const copyToClipboard = () => {
    navigator.clipboard
      .writeText(app.attributes.credential.secret)
      .then(() => {
        console.log("Secret copied to clipboard");
      })
      .catch((err) => {
        console.error("Failed to copy text: ", err);
      });
  };

  const handleDeleteClick = () => {
    setDeleteDialogOpen(true);
  };

  const handleDeleteConfirm = async () => {
    try {
      await pubClient.delete(`/common/apps/${id}`);
      setDeleteDialogOpen(false);
      navigate("/portal/apps", { replace: true });
    } catch (err) {
      console.error("Error deleting app:", err);
      setError("Failed to delete app. Please try again later.");
      setDeleteDialogOpen(false);
    }
  };

  const handleDeleteCancel = () => {
    setDeleteDialogOpen(false);
  };

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
          <Box display="flex" alignItems="center">
            <FieldValue>
              {showSecret
                ? app.attributes.credential.secret
                : "••••••••••••••••"}
            </FieldValue>
            <IconButton onClick={() => setShowSecret(!showSecret)} size="small">
              {showSecret ? <VisibilityOffIcon /> : <VisibilityIcon />}
            </IconButton>
            <IconButton
              onClick={() =>
                navigator.clipboard.writeText(app.attributes.credential.secret)
              }
              size="small"
            >
              <ContentCopyIcon />
            </IconButton>
          </Box>
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
          color="error"
          startIcon={<DeleteIcon />}
          onClick={handleDeleteClick}
        >
          Delete App
        </Button>
      </Box>

      <Dialog
        open={deleteDialogOpen}
        onClose={handleDeleteCancel}
        aria-labelledby="alert-dialog-title"
        aria-describedby="alert-dialog-description"
      >
        <DialogTitle id="alert-dialog-title">{"Confirm Deletion"}</DialogTitle>
        <DialogContent>
          <DialogContentText id="alert-dialog-description">
            Are you sure you want to delete the app "{app.attributes.name}"?
            This action cannot be undone.
          </DialogContentText>
        </DialogContent>
        <DialogActions>
          <Button onClick={handleDeleteCancel}>Cancel</Button>
          <Button onClick={handleDeleteConfirm} color="error" autoFocus>
            Delete
          </Button>
        </DialogActions>
      </Dialog>
    </Paper>
  );
};

export default AppDetailView;
