import React, { useState, useEffect } from "react";
import { useParams, useNavigate, useLocation } from "react-router-dom";
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
  IconButton,
  Card,
  CardContent,
  Tooltip,
} from "@mui/material";

import ArrowBackIcon from "@mui/icons-material/ArrowBack";
import DeleteIcon from "@mui/icons-material/Delete";
import VisibilityIcon from "@mui/icons-material/Visibility";
import VisibilityOffIcon from "@mui/icons-material/VisibilityOff";
import ContentCopyIcon from "@mui/icons-material/ContentCopy";
import HelpOutlineIcon from "@mui/icons-material/HelpOutline";

import pubClient from "../../admin/utils/pubClient";

const SectionTitle = ({ children }) => (
  <Typography variant="h6" gutterBottom sx={{ mt: 3, mb: 2 }}>
    {children}
  </Typography>
);

const FieldLabel = ({ children, sx }) => (
  <Typography variant="subtitle2" color="text.secondary" sx={sx}>
    {children}
  </Typography>
);

const FieldValue = ({ children }) => (
  <Typography variant="body1">{children}</Typography>
);

const AppDetailView = () => {
  const [app, setApp] = useState(null);
  const [accessibleLLMs, setAccessibleLLMs] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [showSecret, setShowSecret] = useState(false);

  const { id } = useParams();
  const navigate = useNavigate();
  const location = useLocation();
  const currentHost = window.location.hostname;

  useEffect(() => {
    const fetchData = async () => {
      try {
        const [appResponse, llmsResponse] = await Promise.all([
          pubClient.get(`/common/apps/${id}`),
          pubClient.get("/common/accessible-llms"),
        ]);
        setApp(appResponse.data);
        setAccessibleLLMs(llmsResponse.data);
        setLoading(false);
      } catch (err) {
        console.error("Error fetching data:", err);
        setError("Failed to load data. Please try again later.");
        setLoading(false);
      }
    };

    fetchData();
  }, [id]);

  const toggleSecretVisibility = () => {
    setShowSecret(!showSecret);
  };

  const copyToClipboard = (text) => {
    navigator.clipboard
      .writeText(text)
      .then(() => {
        console.log("Text copied to clipboard");
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

  const generateSlug = (name) => {
    return name
      .toLowerCase()
      .replace(/[^a-z0-9]+/g, "-")
      .replace(/(^-|-$)/g, "");
  };

  if (loading) return <CircularProgress />;
  if (error) return <Typography color="error">{error}</Typography>;
  if (!app) return <Typography>App not found</Typography>;

  const appLLMs = accessibleLLMs.filter((llm) =>
    app.attributes.llm_ids.includes(Number(llm.id)),
  );

  return (
    <Box>
      <Paper sx={{ p: 3, mb: 3 }}>
        <Box
          display="flex"
          justifyContent="space-between"
          alignItems="center"
          mb={3}
        >
          <Typography variant="h5" sx={{ color: "black" }}>
            App Details
          </Typography>
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
              <IconButton onClick={toggleSecretVisibility} size="small">
                {showSecret ? <VisibilityOffIcon /> : <VisibilityIcon />}
              </IconButton>
              <IconButton
                onClick={() =>
                  copyToClipboard(app.attributes.credential.secret)
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
      </Paper>

      <Paper sx={{ p: 3 }}>
        <SectionTitle>LLM Access Details</SectionTitle>
        {appLLMs.map((llm) => (
          <Card key={llm.id} sx={{ mb: 3 }}>
            <CardContent>
              <Typography variant="h6">{llm.attributes.name}</Typography>
              <Typography variant="body2" color="text.secondary" mb={2}>
                {llm.attributes.short_description}
              </Typography>
              <Box sx={{ display: "flex", flexDirection: "column", gap: 1 }}>
                <Box sx={{ display: "flex", alignItems: "center" }}>
                  <FieldLabel sx={{ minWidth: "100px" }}>REST API:</FieldLabel>
                  <Box>
                    <Tooltip title="This endpoint proxies directly upstream to the vendor using your settings, use the vendor's API or SDK for access">
                      <HelpOutlineIcon
                        sx={{ color: "text.secondary", mr: 1 }}
                      />
                    </Tooltip>
                  </Box>
                  <Box
                    sx={{ flexGrow: 1, display: "flex", alignItems: "center" }}
                  >
                    <Typography
                      variant="body2"
                      component="code"
                      sx={{
                        fontFamily: "monospace",
                        bgcolor: "background.paper",
                        p: 1,
                        borderRadius: 1,
                        flexGrow: 1,
                      }}
                    >
                      {`//${currentHost}:9090/api/llm/rest/${generateSlug(llm.attributes.name)}/`}
                    </Typography>
                    <IconButton
                      onClick={() =>
                        copyToClipboard(
                          `//${currentHost}:9090/api/llm/rest/${generateSlug(llm.attributes.name)}/`,
                        )
                      }
                      size="small"
                    >
                      <ContentCopyIcon />
                    </IconButton>
                  </Box>
                </Box>
                <Box sx={{ display: "flex", alignItems: "center" }}>
                  <FieldLabel sx={{ minWidth: "100px" }}>
                    STREAM API:
                  </FieldLabel>
                  <Box>
                    <Tooltip title="This endpoint proxies directly upstream to the vendor's streaming API using your settings, use the vendor's API or SDK for access, not all vendors support streaming proxy">
                      <HelpOutlineIcon
                        sx={{ color: "text.secondary", mr: 1 }}
                      />
                    </Tooltip>
                  </Box>
                  <Box
                    sx={{ flexGrow: 1, display: "flex", alignItems: "center" }}
                  >
                    <Typography
                      variant="body2"
                      component="code"
                      sx={{
                        fontFamily: "monospace",
                        bgcolor: "background.paper",
                        p: 1,
                        borderRadius: 1,
                        flexGrow: 1,
                      }}
                    >
                      {`//${currentHost}:9090/api/llm/stream/${generateSlug(llm.attributes.name)}/`}
                    </Typography>
                    <IconButton
                      onClick={() =>
                        copyToClipboard(
                          `//${currentHost}:9090/api/llm/stream/${generateSlug(llm.attributes.name)}/`,
                        )
                      }
                      size="small"
                    >
                      <ContentCopyIcon />
                    </IconButton>
                  </Box>
                </Box>
                <Box sx={{ display: "flex", alignItems: "center" }}>
                  <FieldLabel sx={{ minWidth: "100px" }}>
                    UNIFIED API:
                  </FieldLabel>
                  <Box>
                    <Tooltip title="This endpoint exposes an OpenAI-compatible API but translates your requests to the upstream vendor (using the default model defined by the admin)">
                      <HelpOutlineIcon
                        sx={{ color: "text.secondary", mr: 1 }}
                      />
                    </Tooltip>
                  </Box>
                  <Box
                    sx={{ flexGrow: 1, display: "flex", alignItems: "center" }}
                  >
                    <Typography
                      variant="body2"
                      component="code"
                      sx={{
                        fontFamily: "monospace",
                        bgcolor: "background.paper",
                        p: 1,
                        borderRadius: 1,
                        flexGrow: 1,
                      }}
                    >
                      {`//${currentHost}:9090/ai/${generateSlug(llm.attributes.name)}/v1`}
                    </Typography>
                    <IconButton
                      onClick={() =>
                        copyToClipboard(
                          `//${currentHost}:9090/ai/${generateSlug(llm.attributes.name)}/v1/`,
                        )
                      }
                      size="small"
                    >
                      <ContentCopyIcon />
                    </IconButton>
                  </Box>
                </Box>
              </Box>
            </CardContent>
          </Card>
        ))}
      </Paper>

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
    </Box>
  );
};

export default AppDetailView;
