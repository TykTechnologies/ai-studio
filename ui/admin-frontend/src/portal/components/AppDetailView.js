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
import { getConfig } from "../../config"; // Add this import
import { StyledButtonCritical, StyledButtonLink } from "../../admin/styles/sharedStyles";

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
  const [baseUrl, setBaseUrl] = useState("");
  const [tokenUsageAndCostData, setTokenUsageAndCostData] = useState(null);
  const [budgetUsageData, setBudgetUsageData] = useState(null);
  const [startDate, setStartDate] = useState(
    new Date(new Date().getTime() - 30 * 24 * 60 * 60 * 1000)
      .toISOString()
      .split("T")[0],
  );
  const [endDate, setEndDate] = useState(
    new Date().toISOString().split("T")[0],
  );

  const { id } = useParams();
  const navigate = useNavigate();
  const location = useLocation();
  const currentHost = window.location.hostname;

  useEffect(() => {
    const fetchData = async () => {
      try {
        const [appResponse, llmsResponse, usageResponse, budgetResponse] = await Promise.all([
          pubClient.get(`/common/apps/${id}`),
          pubClient.get("/common/accessible-llms"),
          pubClient.get(`/analytics/token-usage-and-cost-for-app`, {
            params: { start_date: startDate, end_date: endDate, app_id: id },
          }),
          pubClient.get(`/analytics/budget-usage-for-app`, {
            params: { app_id: id },
          }),
        ]);

        const config = getConfig();
        setTokenUsageAndCostData(usageResponse.data);
        setBudgetUsageData(budgetResponse.data);
        setBaseUrl(config.proxyURL || `//${currentHost}:9090`);

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

  const generateEndpointUrl = (path, name) => {
    const slug = generateSlug(name);
    return `${baseUrl}${path}${slug}/`;
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
    <Box sx={{p: 4}}>
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
          <StyledButtonLink
            startIcon={<ArrowBackIcon />}
            onClick={() => navigate("/portal/apps")}
          >
            Back to Apps
          </StyledButtonLink>
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
          <Grid item xs={3}>
            <FieldLabel>Monthly Budget:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <Box>
              <FieldValue>
                {app.attributes.monthly_budget ? `$${app.attributes.monthly_budget}` : 'No budget limit'}
              </FieldValue>
              {budgetUsageData?.current_usage != null && budgetUsageData?.start_date && (
                <Typography variant="body2" color="text.secondary" sx={{ mt: 1 }}>
                  Current usage: ${budgetUsageData.current_usage.toFixed(2)} ({budgetUsageData.percentage?.toFixed(1) || 0}%) since {new Date(budgetUsageData.start_date).toLocaleDateString() || 'N/A'}
                </Typography>
              )}
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
          <StyledButtonCritical
            variant="contained"
            color="error"
            startIcon={<DeleteIcon />}
            onClick={handleDeleteClick}
          >
            Delete App
          </StyledButtonCritical>
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

              {/* SDK-Specific Endpoints Section */}
              <Typography
                variant="subtitle1"
                sx={{
                  fontWeight: "bold",
                  mt: 2,
                  mb: 1,
                }}
              >
                SDK-Specific Endpoints
              </Typography>
              <Typography variant="body2" sx={{ mb: 2 }}>
                Use the following URLs in your app, using the appropriate vendor
                SDK or API Specs to interact with the LLMs.
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
                      {generateEndpointUrl("/llm/rest/", llm.attributes.name)}
                    </Typography>
                    <IconButton
                      onClick={() =>
                        copyToClipboard(
                          generateEndpointUrl(
                            "/llm/rest/",
                            llm.attributes.name,
                          ),
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
                      {generateEndpointUrl("/llm/stream/", llm.attributes.name)}
                    </Typography>
                    <IconButton
                      onClick={() =>
                        copyToClipboard(
                          generateEndpointUrl(
                            "/llm/stream/",
                            llm.attributes.name,
                          ),
                        )
                      }
                      size="small"
                    >
                      <ContentCopyIcon />
                    </IconButton>
                  </Box>
                </Box>
              </Box>

              {/* Unified Endpoint Section */}
              <Typography
                variant="subtitle1"
                sx={{
                  fontWeight: "bold",
                  mt: 3,
                  mb: 1,
                }}
              >
                Unified Endpoint
              </Typography>
              <Typography variant="body2" sx={{ mb: 2 }}>
                The Unified Endpoint is an OpenAI-compatible endpoint that
                translates your API calls into vendor-compatible ones to the
                vendor. This endpoint currently does not support Streams.
              </Typography>

              <Box sx={{ display: "flex", alignItems: "center" }}>
                <FieldLabel sx={{ minWidth: "100px" }}>UNIFIED API:</FieldLabel>
                <Box>
                  <Tooltip title="This endpoint exposes an OpenAI-compatible API but translates your requests to the upstream vendor (using the default model defined by the admin)">
                    <HelpOutlineIcon sx={{ color: "text.secondary", mr: 1 }} />
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
                    {`${generateEndpointUrl("/ai/", llm.attributes.name)}v1`}
                  </Typography>
                  <IconButton
                    onClick={() =>
                      copyToClipboard(
                        `${generateEndpointUrl("/ai/", llm.attributes.name)}v1`,
                      )
                    }
                    size="small"
                  >
                    <ContentCopyIcon />
                  </IconButton>
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
          <StyledButtonCritical onClick={handleDeleteConfirm} color="error" autoFocus>
            Delete
          </StyledButtonCritical>
        </DialogActions>
      </Dialog>
    </Box>
  );
};

export default AppDetailView;
