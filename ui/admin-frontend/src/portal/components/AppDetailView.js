import React, { useState, useEffect, useCallback } from "react";
import { Link, useParams, useNavigate } from "react-router-dom";
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
  Accordion,
  AccordionSummary,
  AccordionDetails,
} from "@mui/material";
import DeleteIcon from "@mui/icons-material/Delete";
import VisibilityIcon from "@mui/icons-material/Visibility";
import DescriptionIcon from "@mui/icons-material/Description";
import VisibilityOffIcon from "@mui/icons-material/VisibilityOff";
import ContentCopyIcon from "@mui/icons-material/ContentCopy";
import HelpOutlineIcon from "@mui/icons-material/HelpOutline";
import ExpandMoreIcon from "@mui/icons-material/ExpandMore";
import { getConfig } from "../../config";
import { DangerButton, SecondaryLinkButton } from "../../admin/styles/sharedStyles";
import pubClient from "../../admin/utils/pubClient";
import { Line } from "react-chartjs-2";
import DateRangePicker from "../../admin/components/common/DateRangePicker";

import {
  Chart as ChartJS,
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  BarElement,
  Title,
  Tooltip as ChartTooltip,
  Legend,
  TimeScale,
} from "chart.js";
import "chartjs-adapter-date-fns";

ChartJS.register(
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  BarElement,
  Title,
  ChartTooltip,
  Legend,
  TimeScale
);

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
  const [accessibleDatasources, setAccessibleDatasources] = useState([]);
  const [accessibleTools, setAccessibleTools] = useState([]);
  const [pluginResources, setPluginResources] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [showSecret, setShowSecret] = useState(false);
  const [baseUrl, setBaseUrl] = useState("");
  const [proxyUrl, setProxyUrl] = useState("");
  const [toolDisplayUrl, setToolDisplayUrl] = useState("");
  const [datasourceDisplayUrl, setDatasourceDisplayUrl] = useState("");
  const [tokenUsageAndCostData, setTokenUsageAndCostData] = useState(null);
  const [budgetUsageData, setBudgetUsageData] = useState(null);
  const [appInteractionsData, setAppInteractionsData] = useState(null);
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

  const fetchAnalyticsData = useCallback(async (start, end) => {
    try {
      const [usageResponse, budgetResponse, interactionsResponse] = await Promise.all([
        pubClient.get(`/common/apps/${id}/analytics/usage`, {
          params: { start_date: start, end_date: end },
        }),
        pubClient.get(`/analytics/budget-usage-for-app`, {
          params: { app_id: id },
        }),
        pubClient.get(`/common/apps/${id}/analytics/interactions`, {
          params: { start_date: start, end_date: end },
        }),
      ]);
      setTokenUsageAndCostData(usageResponse.data);
      setBudgetUsageData(budgetResponse.data);
      setAppInteractionsData(interactionsResponse.data);
    } catch (error) {
      console.error("Error fetching usage and budget data", error);
    }
  }, [id]);
  const currentHost = window.location.hostname;

  useEffect(() => {
    const fetchData = async () => {
      try {
        const [appResponse, llmsResponse, datasourcesResponse, toolsResponse] = await Promise.all([
          pubClient.get(`/common/apps/${id}`),
          pubClient.get('/common/accessible-llms'),
          pubClient.get('/common/accessible-datasources'),
          pubClient.get('/common/accessible-tools'),
        ]);

        // Set the base URL for API endpoints
        const config = getConfig();
        const apiHost = config.api_host || window.location.origin;
        // Use proxyURL for proxy endpoints if available, otherwise fall back to apiHost
        const proxyUrlValue = config.proxyURL || `${window.location.protocol}//${window.location.hostname}:9090`;
        // Use separate display URLs for tools and datasources if configured
        const toolDisplayUrlValue = config.toolDisplayURL || proxyUrlValue;
        const datasourceDisplayUrlValue = config.dataSourceDisplayURL || proxyUrlValue;
        setBaseUrl(apiHost);
        setProxyUrl(proxyUrlValue);
        setToolDisplayUrl(toolDisplayUrlValue);
        setDatasourceDisplayUrl(datasourceDisplayUrlValue);

        const app = appResponse.data;
        setApp(app);

        // Filter accessible LLMs that are associated with the app
        const accessibleLLMs = llmsResponse.data;
        const appLLMIds = app.attributes.llm_ids || [];
        const filteredLLMs = accessibleLLMs.filter((llm) => appLLMIds.includes(parseInt(llm.id)));
        setAccessibleLLMs(filteredLLMs);

        // Filter accessible datasources that are associated with the app
        const accessibleDatasources = datasourcesResponse.data;
        const appDatasourceIds = app.attributes.datasource_ids || [];
        const filteredDatasources = accessibleDatasources.filter((ds) => appDatasourceIds.includes(parseInt(ds.id)));
        setAccessibleDatasources(filteredDatasources);

        // Filter accessible tools that are associated with the app
        const accessibleTools = toolsResponse.data;
        const appToolIds = app.attributes.tool_ids || [];
        const filteredTools = accessibleTools.filter((tool) => appToolIds.includes(parseInt(tool.id)));
        setAccessibleTools(filteredTools);

        // Load plugin resource associations
        if (app.attributes.plugin_resources && app.attributes.plugin_resources.length > 0) {
          setPluginResources(app.attributes.plugin_resources);
        } else {
          // Try dedicated endpoint
          try {
            const prResp = await pubClient.get(`/common/apps/${id}/plugin-resources`);
            setPluginResources(prResp.data?.data || []);
          } catch {
            // Plugin resources not available
          }
        }

        // Fetch analytics data
        await fetchAnalyticsData(startDate, endDate);

        setLoading(false);
      } catch (error) {
        console.error("Error:", error);
        setError("Failed to load app details");
        setLoading(false);
      }
    };

    fetchData();
  }, [id, currentHost, startDate, endDate, fetchAnalyticsData]);

  const toggleSecretVisibility = () => {
    setShowSecret(!showSecret);
  };

  const generateVendorEndpointURL = (path, llm) => {
    const v1Suffix = "v1"

    const { name, vendor } = llm.attributes
    const baseUrl = generateEndpointUrl(path, name)

    switch (vendor) {
      case "google_ai":
        return joinUrlParts(baseUrl, v1Suffix)
      default:
        return baseUrl
    }
  }

  const generateEndpointUrl = (path, name) => {
    const slug = generateSlug(name);
    // Use proxyUrl for LLM proxy endpoints
    return `${proxyUrl}${path}${slug}`;
  };

  const generateToolEndpointUrl = (path, name) => {
    const slug = generateSlug(name);
    // Use toolDisplayUrl for tool endpoints
    return `${toolDisplayUrl}${path}${slug}`;
  };

  const generateDatasourceEndpointUrl = (path, name) => {
    const slug = generateSlug(name);
    // Use datasourceDisplayUrl for datasource endpoints
    return `${datasourceDisplayUrl}${path}${slug}`;
  };

  // Helper to join URL parts ensuring proper slash handling
  const joinUrlParts = (...parts) => {
    return parts
      .map((part, index) => {
        if (index === 0) {
          // Remove trailing slash from first part
          return part.replace(/\/+$/, '');
        }
        // Remove leading and trailing slashes from middle parts, only trailing from last
        if (index === parts.length - 1) {
          return part.replace(/^\/+/, '');
        }
        return part.replace(/^\/+/, '').replace(/\/+$/, '');
      })
      .join('/');
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
    (app.attributes.llm_ids || []).includes(Number(llm.id))
  );
  const appDatasources = accessibleDatasources.filter((datasource) =>
    (app.attributes.datasource_ids || []).includes(Number(datasource.id)),
  );

  const appTools = accessibleTools.filter((tool) =>
    (app.attributes.tool_ids || []).includes(Number(tool.id)),
  );

  return (
    <Box sx={{p: 4}}>
      <Box sx={{ mb: 3, display: "flex", justifyContent: "space-between" }}>
        <Box>
          <Typography variant="h4" component="h1">
            {app.attributes.name}
          </Typography>
          <Typography variant="body1" color="textSecondary">
            {app.attributes.description}
          </Typography>
        </Box>
        <Box>
          <SecondaryLinkButton onClick={() => navigate("/portal/apps")}>
            Back to Apps
          </SecondaryLinkButton>
        </Box>
      </Box>
      
      {/* Analytics Charts */}
      <SectionTitle>Token Usage</SectionTitle>
      <Box height={300} mb={4}>
        <Line options={{
          responsive: true,
          maintainAspectRatio: false,
          scales: {
            x: {
              type: "time",
              time: {
                unit: "day",
              },
              title: {
                display: true,
                text: "Date",
              },
              stacked: true,
            },
            y: {
              beginAtZero: true,
              title: {
                display: true,
                text: "Token Usage",
              },
              stacked: true,
            },
          },
          plugins: {
            legend: {
              position: "top",
            },
            title: {
              display: true,
              text: "Token Usage Over Time",
            },
            tooltip: {
              mode: 'index',
            },
          },
        }} data={{
          labels: tokenUsageAndCostData?.labels || [],
          datasets: [
            {
              label: "Prompt Tokens",
              data: tokenUsageAndCostData?.datasets?.[2]?.data || [],
              borderColor: "rgb(53, 162, 235)",
              backgroundColor: "rgba(53, 162, 235, 0.5)",
              fill: true,
            },
            {
              label: "Response Tokens",
              data: tokenUsageAndCostData?.datasets?.[3]?.data || [],
              borderColor: "rgb(75, 192, 192)",
              backgroundColor: "rgba(75, 192, 192, 0.5)",
              fill: true,
            },
            {
              label: "Cache Write Tokens",
              data: tokenUsageAndCostData?.datasets?.[4]?.data || [],
              borderColor: "rgb(255, 159, 64)",
              backgroundColor: "rgba(255, 159, 64, 0.5)",
              fill: true,
            },
            {
              label: "Cache Read Tokens",
              data: tokenUsageAndCostData?.datasets?.[5]?.data || [],
              borderColor: "rgb(153, 102, 255)",
              backgroundColor: "rgba(153, 102, 255, 0.5)",
              fill: true,
            },
          ],
        }} />
      </Box>

      <SectionTitle>Cost</SectionTitle>
      <Box height={300} mb={4}>
        <Line options={{
          responsive: true,
          maintainAspectRatio: false,
          scales: {
            x: {
              type: "time",
              time: {
                unit: "day",
              },
              title: {
                display: true,
                text: "Date",
              },
            },
            y: {
              beginAtZero: true,
              title: {
                display: true,
                text: "Cost ($)",
              },
            },
          },
          plugins: {
            legend: {
              position: "top",
            },
            title: {
              display: true,
              text: "Cost Over Time",
            },
          },
        }} data={{
          labels: tokenUsageAndCostData?.labels || [],
          datasets: [
            {
              label: "Cost",
              data: tokenUsageAndCostData?.datasets?.[1]?.data || [],
              borderColor: "rgb(255, 99, 132)",
              tension: 0.1,
            },
          ],
        }} />
      </Box>

      <SectionTitle>App Interactions</SectionTitle>
      <Box height={300} mb={4}>
        <Line options={{
          responsive: true,
          maintainAspectRatio: false,
          scales: {
            x: {
              type: "time",
              time: {
                unit: "day",
              },
              title: {
                display: true,
                text: "Date",
              },
            },
            y: {
              beginAtZero: true,
              title: {
                display: true,
                text: "Number of Interactions",
              },
            },
          },
          plugins: {
            legend: {
              position: "top",
            },
            title: {
              display: true,
              text: "App Interactions Over Time",
            },
          },
        }} data={{
          labels: appInteractionsData?.labels || [],
          datasets: [
            {
              label: "Interactions",
              data: appInteractionsData?.data || [],
              borderColor: "rgb(255, 206, 86)",
              backgroundColor: "rgba(255, 206, 86, 0.2)",
              tension: 0.1,
            },
          ],
        }} />
      </Box>
      <Box mt={2} mb={4}>
        <DateRangePicker
          startDate={startDate}
          endDate={endDate}
          onStartDateChange={(newDate) => {
            setStartDate(newDate);
            fetchAnalyticsData(newDate, endDate);
          }}
          onEndDateChange={(newDate) => {
            setEndDate(newDate);
            fetchAnalyticsData(startDate, newDate);
          }}
        />
      </Box>

      {/* Main App Details Paper */}
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
            <FieldLabel>Status:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <FieldValue>
              {app.attributes.credential.active ? "Active" : "Inactive"}
            </FieldValue>
          </Grid>
          <Grid item xs={3}>
            <FieldLabel>Data Sources:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <Box display="flex" flexWrap="wrap" gap={1}>
              {appDatasources.length > 0 ? appDatasources.map((ds) => (
                <Chip key={ds.id} label={ds.attributes.name} />
              )) : <Typography variant="body2">No data sources associated.</Typography>}
            </Box>
          </Grid>
          <Grid item xs={3}>
            <FieldLabel>LLMs:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <Box display="flex" flexWrap="wrap" gap={1}>
              {appLLMs.length > 0 ? appLLMs.map((llm) => (
                <Chip key={llm.id} label={llm.attributes.name} />
              )) : <Typography variant="body2">No LLMs associated.</Typography>}
            </Box>
          </Grid>
          <Grid item xs={3}>
            <FieldLabel>Tools:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <Box display="flex" flexWrap="wrap" gap={1}>
              {appTools.length > 0 ? appTools.map((tool) => (
                <Chip key={tool.id} label={tool.attributes.name} />
              )) : <Typography variant="body2">No tools associated.</Typography>}
            </Box>
          </Grid>
          {/* Plugin Resources */}
          {pluginResources.length > 0 && pluginResources.map((pr) => (
            <React.Fragment key={`pr-${pr.plugin_id || ''}-${pr.resource_type_slug || ''}`}>
              <Grid item xs={3}>
                <FieldLabel>{pr.resource_type_name || pr.resource_type_slug || 'Plugin Resources'}:</FieldLabel>
              </Grid>
              <Grid item xs={9}>
                <Box display="flex" flexWrap="wrap" gap={1}>
                  {(pr.instance_ids || []).map((instanceId) => (
                    <Chip key={instanceId} label={instanceId} />
                  ))}
                </Box>
              </Grid>
            </React.Fragment>
          ))}
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
          <DangerButton
            variant="contained"
            color="error"
            startIcon={<DeleteIcon />}
            onClick={handleDeleteClick}
          >
            Delete App
          </DangerButton>
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

              {/* Primary Unified Endpoint Section */}
              <Typography
                variant="subtitle1"
                sx={{
                  fontWeight: "bold",
                  mt: 2,
                  mb: 1,
                }}
              >
                Recommended Endpoint
              </Typography>
              <Typography variant="body2" sx={{ mb: 2 }}>
                Use this unified endpoint that automatically handles both streaming and non-streaming requests based on your request parameters. Works with any vendor SDK or API.
              </Typography>

              <Box sx={{ display: "flex", alignItems: "center", mb: 2 }}>
                <FieldLabel sx={{ minWidth: "100px" }}>UNIFIED:</FieldLabel>
                <Box>
                  <Tooltip title="This endpoint automatically detects streaming vs non-streaming requests and routes them appropriately. Use with your vendor's native SDK or API.">
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
                    {generateVendorEndpointURL("/llm/call/", llm)}
                  </Typography>
                  <IconButton
                    onClick={() =>
                      copyToClipboard(
                        generateVendorEndpointURL("/llm/call/", llm),
                      )
                    }
                    size="small"
                  >
                    <ContentCopyIcon />
                  </IconButton>
                </Box>
              </Box>

              {/* OpenAI Compatible Endpoint Section */}
              <Typography
                variant="subtitle1"
                sx={{
                  fontWeight: "bold",
                  mt: 2,
                  mb: 1,
                }}
              >
                OpenAI-Compatible Endpoint
              </Typography>
              <Typography variant="body2" sx={{ mb: 2 }}>
                Use this endpoint with OpenAI SDKs and tools. It translates OpenAI-format requests to your configured vendor's API.
              </Typography>

              <Box sx={{ display: "flex", alignItems: "center" }}>
                <FieldLabel sx={{ minWidth: "100px" }}>OpenAI API:</FieldLabel>
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
                    {joinUrlParts(generateEndpointUrl("/ai/", llm.attributes.name), "v1")}
                  </Typography>
                  <IconButton
                    onClick={() =>
                      copyToClipboard(
                        joinUrlParts(generateEndpointUrl("/ai/", llm.attributes.name), "v1"),
                      )
                    }
                    size="small"
                  >
                    <ContentCopyIcon />
                  </IconButton>
                </Box>
              </Box>

              {/* Legacy Endpoints - Collapsible */}
              <Accordion
                sx={{
                  mt: 3,
                  boxShadow: 'none',
                  border: '1px solid',
                  borderColor: 'divider',
                  '&:before': { display: 'none' }
                }}
              >
                <AccordionSummary
                  expandIcon={<ExpandMoreIcon />}
                  sx={{
                    bgcolor: 'background.default',
                    '& .MuiAccordionSummary-content': {
                      alignItems: 'center'
                    }
                  }}
                >
                  <Typography variant="subtitle2" color="text.secondary">
                    Legacy Endpoints (Advanced)
                  </Typography>
                </AccordionSummary>
                <AccordionDetails>
                  <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
                    These endpoints require you to manually specify whether your request is streaming or non-streaming. Use the unified endpoint above instead for automatic routing.
                  </Typography>

                  <Box sx={{ display: "flex", flexDirection: "column", gap: 1 }}>
                    <Box sx={{ display: "flex", alignItems: "center" }}>
                      <FieldLabel sx={{ minWidth: "100px" }}>REST API:</FieldLabel>
                      <Box>
                        <Tooltip title="This endpoint proxies directly upstream to the vendor using your settings for non-streaming requests only">
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
                          {generateVendorEndpointURL("/llm/rest/", llm)}
                        </Typography>
                        <IconButton
                          onClick={() =>
                            copyToClipboard(
                              generateVendorEndpointURL("/llm/rest/", llm)
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
                        <Tooltip title="This endpoint proxies directly upstream to the vendor's streaming API for streaming requests only">
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
                          {generateVendorEndpointURL("/llm/stream/", llm)}
                        </Typography>
                        <IconButton
                          onClick={() =>
                            copyToClipboard(
                              generateVendorEndpointURL(
                                "/llm/stream/",
                                llm
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
                </AccordionDetails>
              </Accordion>
            </CardContent>
          </Card>
        ))}
      </Paper>

      <Paper sx={{ p: 3, mt: 3 }}>
        <SectionTitle>Data Source Access Details</SectionTitle>
        {appDatasources.length > 0 ? (
          appDatasources.map((datasource) => (
            <Card key={datasource.id} sx={{ mb: 3 }}>
              <CardContent>
                <Typography variant="h6">{datasource.attributes.name}</Typography>
                <Typography variant="body2" color="text.secondary" mb={2}>
                  {datasource.attributes.short_description || "No description available"}
                </Typography>

                <Typography
                  variant="subtitle1"
                  sx={{
                    fontWeight: "bold",
                    mt: 2,
                    mb: 1,
                  }}
                >
                  Endpoint
                </Typography>
                <Typography variant="body2" sx={{ mb: 2 }}>
                  Use the following URL to search this datasource.
                </Typography>

                <Box sx={{ display: "flex", alignItems: "center" }}>
                  <FieldLabel sx={{ minWidth: "100px" }}>Search API:</FieldLabel>
                  <Box>
                    <Tooltip title="Send a POST request with a JSON body containing 'query' and 'n' fields to search this datasource">
                      <HelpOutlineIcon sx={{ color: "text.secondary", mr: 1 }} />
                    </Tooltip>
                  </Box>
                  <Box sx={{ flexGrow: 1, display: "flex", alignItems: "center" }}>
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
                      {generateDatasourceEndpointUrl("/datasource/", datasource.attributes.name)}
                    </Typography>
                    <IconButton
                      onClick={() =>
                        copyToClipboard(
                          generateDatasourceEndpointUrl("/datasource/", datasource.attributes.name)
                        )
                      }
                      size="small"
                    >
                      <ContentCopyIcon />
                    </IconButton>
                  </Box>
                </Box>

                <Accordion
                  sx={{
                    mt: 3,
                    boxShadow: 'none',
                    border: '1px solid',
                    borderColor: 'divider',
                    '&:before': { display: 'none' }
                  }}
                >
                  <AccordionSummary
                    expandIcon={<ExpandMoreIcon />}
                    sx={{
                      bgcolor: 'background.default',
                      '& .MuiAccordionSummary-content': {
                        alignItems: 'center'
                      }
                    }}
                  >
                    <Typography variant="subtitle2" color="text.secondary">
                      Additional Endpoints
                    </Typography>
                  </AccordionSummary>
                  <AccordionDetails>
                    <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
                      These endpoints provide advanced datasource capabilities including vector search, metadata filtering, and embedding generation.
                    </Typography>

                    <Box sx={{ display: "flex", flexDirection: "column", gap: 1 }}>
                      <Box sx={{ display: "flex", alignItems: "center" }}>
                        <FieldLabel sx={{ minWidth: "140px" }}>Vector Search:</FieldLabel>
                        <Box>
                          <Tooltip title="POST a JSON body with an 'embedding' vector array, optional 'n' (max results) and 'similarity_threshold' to perform similarity search using a pre-computed embedding">
                            <HelpOutlineIcon sx={{ color: "text.secondary", mr: 1 }} />
                          </Tooltip>
                        </Box>
                        <Box sx={{ flexGrow: 1, display: "flex", alignItems: "center" }}>
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
                            {generateDatasourceEndpointUrl("/datasource/", datasource.attributes.name) + "/vector"}
                          </Typography>
                          <IconButton
                            onClick={() =>
                              copyToClipboard(
                                generateDatasourceEndpointUrl("/datasource/", datasource.attributes.name) + "/vector"
                              )
                            }
                            size="small"
                          >
                            <ContentCopyIcon />
                          </IconButton>
                        </Box>
                      </Box>

                      <Box sx={{ display: "flex", alignItems: "center" }}>
                        <FieldLabel sx={{ minWidth: "140px" }}>Metadata Query:</FieldLabel>
                        <Box>
                          <Tooltip title="POST a JSON body with a 'filter' object (key-value pairs), optional 'filter_mode' (AND/OR), 'limit' and 'offset' for paginated metadata-only queries">
                            <HelpOutlineIcon sx={{ color: "text.secondary", mr: 1 }} />
                          </Tooltip>
                        </Box>
                        <Box sx={{ flexGrow: 1, display: "flex", alignItems: "center" }}>
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
                            {generateDatasourceEndpointUrl("/datasource/", datasource.attributes.name) + "/metadata"}
                          </Typography>
                          <IconButton
                            onClick={() =>
                              copyToClipboard(
                                generateDatasourceEndpointUrl("/datasource/", datasource.attributes.name) + "/metadata"
                              )
                            }
                            size="small"
                          >
                            <ContentCopyIcon />
                          </IconButton>
                        </Box>
                      </Box>

                      <Box sx={{ display: "flex", alignItems: "center" }}>
                        <FieldLabel sx={{ minWidth: "140px" }}>Embeddings:</FieldLabel>
                        <Box>
                          <Tooltip title="POST a JSON body with a 'texts' array (max 100 items) to generate embedding vectors without storing them">
                            <HelpOutlineIcon sx={{ color: "text.secondary", mr: 1 }} />
                          </Tooltip>
                        </Box>
                        <Box sx={{ flexGrow: 1, display: "flex", alignItems: "center" }}>
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
                            {generateDatasourceEndpointUrl("/datasource/", datasource.attributes.name) + "/embeddings"}
                          </Typography>
                          <IconButton
                            onClick={() =>
                              copyToClipboard(
                                generateDatasourceEndpointUrl("/datasource/", datasource.attributes.name) + "/embeddings"
                              )
                            }
                            size="small"
                          >
                            <ContentCopyIcon />
                          </IconButton>
                        </Box>
                      </Box>
                    </Box>
                  </AccordionDetails>
                </Accordion>
              </CardContent>
            </Card>
          ))
        ) : (
          <Typography variant="body1">No datasources associated with this app.</Typography>
        )}
      </Paper>

      <Paper sx={{ p: 3, mt: 3 }}>
        <SectionTitle>Tool Access Details</SectionTitle>
        {appTools.length > 0 ? (
          appTools.map((tool) => (
            <Card key={tool.id} sx={{ mb: 3 }}>
              <CardContent>
                <Typography variant="h6">{tool.attributes.name}</Typography>
                <Typography variant="body2" color="text.secondary" mb={2}>
                  {tool.attributes.short_description || "No description available"}
                </Typography>

                <Typography
                  variant="subtitle1"
                  sx={{
                    fontWeight: "bold",
                    mt: 2,
                    mb: 1,
                  }}
                >
                  Endpoint
                </Typography>
                <Typography variant="body2" sx={{ mb: 2 }}>
                  Use the following URL to interact with this tool.
                </Typography>

                <Box sx={{ display: "flex", alignItems: "center" }}>
                  <FieldLabel sx={{ minWidth: "100px" }}>Tool API:</FieldLabel>
                  <Box>
                    <Tooltip title="Use this endpoint to interact with the tool. Refer to the tool's specific documentation for API details.">
                      <HelpOutlineIcon sx={{ color: "text.secondary", mr: 1 }} />
                    </Tooltip>
                  </Box>
                  <Box sx={{ flexGrow: 1, display: "flex", alignItems: "center" }}>
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
                      {generateToolEndpointUrl("/tools/", tool.attributes.name)}
                    </Typography>
                    <IconButton
                      onClick={() =>
                        copyToClipboard(
                          generateToolEndpointUrl("/tools/", tool.attributes.name)
                        )
                      }
                      size="small"
                    >
                      <ContentCopyIcon />
                    </IconButton>
                  </Box>
                </Box>
                
                <Box sx={{ mt: 2, display: "flex", justifyContent: "flex-end" }}>
                  <Button
                    component={Link}
                    to={`/portal/tools/${tool.id}/docs`}
                    variant="outlined"
                    color="primary"
                    size="small"
                    startIcon={<DescriptionIcon />}
                  >
                    View Documentation
                  </Button>
                </Box>
              </CardContent>
            </Card>
          ))
        ) : (
          <Typography variant="body1">No tools associated with this app.</Typography>
        )}
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
          <DangerButton onClick={handleDeleteConfirm} color="error" autoFocus>
            Delete
          </DangerButton>
        </DialogActions>
      </Dialog>
    </Box>
  );
};

export default AppDetailView;
