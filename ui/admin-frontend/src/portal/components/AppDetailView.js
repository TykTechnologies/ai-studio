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
  Alert,
  AlertTitle,
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
  // Base path of the gateway's unified ("Main Ingress") OpenAI-compatible
  // endpoint. Configurable server-side and disable-able, so it comes from
  // /auth/config; "" means the gateway does not serve it and we advertise nothing.
  const [unifiedRouterPath, setUnifiedRouterPath] = useState("");
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
        setUnifiedRouterPath(
          config.unifiedRouterPath === undefined ? "/v1" : config.unifiedRouterPath,
        );

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

  // Unified ("Main Ingress") endpoint helpers. The base path is whatever the
  // gateway reports (default "/v1"), so nothing here hardcodes it.
  const unifiedBaseUrl = () => joinUrlParts(proxyUrl, unifiedRouterPath);

  const unifiedChatCompletionsUrl = () =>
    joinUrlParts(proxyUrl, unifiedRouterPath, "chat/completions");

  // The model string the Main Ingress expects for a given LLM: "<llm-slug>/<model>".
  // Prefer the LLM's default model for the example, then its first allowed model,
  // and fall back to a placeholder when the admin has configured neither.
  const unifiedModelExample = (llm) => {
    const slug = generateSlug(llm.attributes.name);
    const model =
      llm.attributes.default_model ||
      (llm.attributes.allowed_models || [])[0] ||
      "<model>";
    return `${slug}/${model}`;
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
        {/* A portal-created App's credential is minted inactive and an
            administrator has to approve the App before the key works. The
            portal said "submitted for approval" once, on the previous screen,
            and then showed a secret you could copy with nothing indicating it
            was not yet live -- so the state was discovered as a 401. */}
        {!app.attributes.credential.active && (
          <Alert severity="info" sx={{ mb: 2 }}>
            <AlertTitle>Waiting for approval</AlertTitle>
            This credential is not active yet. An administrator has to approve
            this app before the key below will work — requests made with it
            will be rejected with <strong>401 Unauthorized</strong> until then.
          </Alert>
        )}
        <Grid
          container
          spacing={2}
          sx={{
            // Greyed while pending, so the credential does not read as usable.
            opacity: app.attributes.credential.active ? 1 : 0.6,
          }}
        >
          <Grid item xs={3}>
            <FieldLabel>Key ID:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            {/* The API serialises this as key_id (CredentialDetail); keyID
                read as undefined and the field rendered blank. */}
            <FieldValue>{app.attributes.credential.key_id}</FieldValue>
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
            <FieldLabel>Status:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <Chip
              size="small"
              label={
                app.attributes.credential.active
                  ? "Active"
                  : "Pending approval"
              }
              color={app.attributes.credential.active ? "success" : "warning"}
              variant="outlined"
            />
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

        {/* Three shapes of endpoint are on offer and they are easy to confuse, so
            say up-front what each one is for before listing any URLs. */}
        <Typography variant="body2" color="text.secondary" sx={{ mb: 3 }}>
          Every endpoint below is authenticated with this app&apos;s credential and
          enforces the same policies, budgets and filters. They differ in how you
          choose the model:{" "}
          <strong>Main Ingress</strong> is one URL covering every LLM in this app and
          picks the LLM from the request body, while the{" "}
          <strong>per-LLM endpoints</strong> pin one LLM into the URL itself.
        </Typography>

        {/* Main Ingress: the gateway's unified OpenAI-compatible ingress. It is
            app-scoped rather than per-LLM, so it lives above the LLM cards. */}
        {unifiedRouterPath && appLLMs.length > 0 && (
          <Card
            sx={{ mb: 3, border: "1px solid", borderColor: "primary.main" }}
          >
            <CardContent>
              <Box sx={{ display: "flex", alignItems: "center", gap: 1, mb: 1 }}>
                <Typography variant="h6">Main Ingress</Typography>
                <Chip
                  size="small"
                  color="primary"
                  variant="outlined"
                  label="One endpoint, all LLMs"
                />
              </Box>
              <Typography variant="body2" color="text.secondary" mb={2}>
                A single OpenAI-compatible endpoint that fronts every LLM this app
                can access. The LLM is selected per request from the{" "}
                <code>model</code> field, so a client can switch model or vendor
                without changing its base URL or credential.
              </Typography>

              <Typography variant="subtitle2" sx={{ fontWeight: "bold", mb: 0.5 }}>
                Use this when
              </Typography>
              <Box component="ul" sx={{ mt: 0, mb: 2, pl: 3 }}>
                <Typography component="li" variant="body2" color="text.secondary">
                  Your client already speaks the OpenAI Chat Completions API (OpenAI
                  SDKs, LangChain, and most off-the-shelf AI tooling).
                </Typography>
                <Typography component="li" variant="body2" color="text.secondary">
                  You want one base URL for the whole app and want to choose the
                  model at request time, including across vendors.
                </Typography>
                <Typography component="li" variant="body2" color="text.secondary">
                  You are configuring a tool that only lets you set one base URL and
                  one API key.
                </Typography>
              </Box>

              <Box sx={{ display: "flex", alignItems: "center", mb: 1 }}>
                <FieldLabel sx={{ minWidth: "130px" }}>MAIN INGRESS:</FieldLabel>
                <Box>
                  <Tooltip title="OpenAI-compatible ingress for every LLM in this app. The vendor prefix in the model field selects which LLM handles the request.">
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
                    {unifiedChatCompletionsUrl()}
                  </Typography>
                  <IconButton
                    onClick={() => copyToClipboard(unifiedChatCompletionsUrl())}
                    size="small"
                  >
                    <ContentCopyIcon />
                  </IconButton>
                </Box>
              </Box>

              <Box sx={{ display: "flex", alignItems: "center", mb: 2 }}>
                <FieldLabel sx={{ minWidth: "130px" }}>SDK BASE URL:</FieldLabel>
                <Box>
                  <Tooltip title="Set this as base_url / OPENAI_BASE_URL in an OpenAI SDK and use the app secret as the API key. The SDK appends /chat/completions and /models itself.">
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
                    {unifiedBaseUrl()}
                  </Typography>
                  <IconButton
                    onClick={() => copyToClipboard(unifiedBaseUrl())}
                    size="small"
                  >
                    <ContentCopyIcon />
                  </IconButton>
                </Box>
              </Box>

              {/* The two caveats that make this endpoint different from the per-LLM
                  ones. Both are 400-level failures if you get them wrong, so they
                  are called out rather than buried in a tooltip. */}
              <Alert severity="info" sx={{ mb: 2 }}>
                <AlertTitle>Different model configuration</AlertTitle>
                <Box component="ul" sx={{ m: 0, pl: 2.5 }}>
                  <Typography component="li" variant="body2">
                    <strong>Models must be namespaced.</strong> Send{" "}
                    <code>&lt;llm-slug&gt;/&lt;model&gt;</code> rather than a bare model
                    name — the prefix picks the LLM. A bare name is rejected with{" "}
                    <strong>400</strong>. See the prefixes for this app below.
                  </Typography>
                  <Typography component="li" variant="body2">
                    <strong>OpenAI format only.</strong> Requests and responses use the
                    OpenAI Chat Completions schema whatever the upstream vendor is. To
                    use a vendor&apos;s own SDK and native parameters, use that
                    LLM&apos;s vendor-native endpoint below instead.
                  </Typography>
                  <Typography component="li" variant="body2">
                    Streaming and non-streaming are both handled automatically from{" "}
                    <code>stream: true</code>, and{" "}
                    <code>GET {joinUrlParts(unifiedBaseUrl(), "models")}</code> lists
                    every model this app can call, already namespaced.
                  </Typography>
                </Box>
              </Alert>

              <Typography variant="subtitle2" sx={{ fontWeight: "bold", mb: 1 }}>
                Model names for this app
              </Typography>
              <Box sx={{ display: "flex", flexDirection: "column", gap: 1 }}>
                {appLLMs.map((llm) => (
                  <Box
                    key={`unified-${llm.id}`}
                    sx={{ display: "flex", alignItems: "center" }}
                  >
                    <FieldLabel sx={{ minWidth: "200px" }}>
                      {llm.attributes.name}:
                    </FieldLabel>
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
                        {unifiedModelExample(llm)}
                      </Typography>
                      <IconButton
                        onClick={() => copyToClipboard(unifiedModelExample(llm))}
                        size="small"
                      >
                        <ContentCopyIcon />
                      </IconButton>
                    </Box>
                  </Box>
                ))}
              </Box>
              <Typography
                variant="caption"
                color="text.secondary"
                sx={{ display: "block", mt: 1 }}
              >
                Any model the administrator has allowed on an LLM works with that
                LLM&apos;s prefix — the names above are examples using each
                LLM&apos;s default model.
              </Typography>
            </CardContent>
          </Card>
        )}

        {appLLMs.length > 0 && (
          <Typography variant="subtitle1" sx={{ fontWeight: "bold", mb: 2 }}>
            Per-LLM Endpoints
          </Typography>
        )}
        {appLLMs.map((llm) => (
          <Card key={llm.id} sx={{ mb: 3 }}>
            <CardContent>
              <Typography variant="h6">{llm.attributes.name}</Typography>
              <Typography variant="body2" color="text.secondary" mb={2}>
                {llm.attributes.short_description}
              </Typography>

              {/* Vendor-native pass-through. Named for what it does rather than
                  "Unified", which now means the app-wide Main Ingress above. */}
              <Typography
                variant="subtitle1"
                sx={{
                  fontWeight: "bold",
                  mt: 2,
                  mb: 1,
                }}
              >
                Vendor-native Endpoint
              </Typography>
              <Typography variant="body2" sx={{ mb: 2 }}>
                Passes your request through in this vendor&apos;s own API format,
                preserving vendor-specific parameters and response fields. Streaming
                and non-streaming are detected automatically from the request.{" "}
                <strong>Use this when</strong> you are working with the vendor&apos;s
                own SDK, or need a capability the OpenAI schema cannot express.
              </Typography>

              <Box sx={{ display: "flex", alignItems: "center", mb: 2 }}>
                <FieldLabel sx={{ minWidth: "100px" }}>VENDOR API:</FieldLabel>
                <Box>
                  <Tooltip title="Pass-through to the vendor's native API for this LLM. Automatically detects streaming vs non-streaming requests. Use with your vendor's native SDK.">
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
                Speaks the OpenAI Chat Completions API but is pinned to this one LLM:
                the URL names the LLM, so the <code>model</code> field takes a plain
                vendor model name (or is omitted, falling back to the default model
                the administrator configured).{" "}
                <strong>Use this when</strong> a client should only ever reach this
                LLM, or when a tool cannot send a namespaced{" "}
                <code>&lt;llm-slug&gt;/&lt;model&gt;</code> model string as the Main
                Ingress requires.
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

              {/* Anthropic Messages Endpoint (Bedrock only) */}
              {llm.attributes.vendor === "bedrock" && (
                <>
                  <Typography
                    variant="subtitle1"
                    sx={{
                      fontWeight: "bold",
                      mt: 2,
                      mb: 1,
                    }}
                  >
                    Anthropic-Compatible Endpoint
                  </Typography>
                  <Typography variant="body2" sx={{ mb: 2 }}>
                    Use this endpoint with the native Anthropic Messages API. Set it as <code>ANTHROPIC_BASE_URL</code> and pass the app key as <code>ANTHROPIC_AUTH_TOKEN</code> (or <code>ANTHROPIC_API_KEY</code>). Requests are translated to your configured Bedrock model. <strong>Use this when</strong> the client speaks the Anthropic Messages API — Claude Code, for example — rather than the OpenAI schema.
                  </Typography>

                  <Box sx={{ display: "flex", alignItems: "center" }}>
                    <FieldLabel sx={{ minWidth: "100px" }}>Anthropic API:</FieldLabel>
                    <Box>
                      <Tooltip title="Native Anthropic Messages API (appends /v1/messages), translated to Bedrock Converse using the default model defined by the admin. Set this as ANTHROPIC_BASE_URL for Claude Code.">
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
                        {generateEndpointUrl("/anthropic/", llm.attributes.name)}
                      </Typography>
                      <IconButton
                        onClick={() =>
                          copyToClipboard(
                            generateEndpointUrl("/anthropic/", llm.attributes.name),
                          )
                        }
                        size="small"
                      >
                        <ContentCopyIcon />
                      </IconButton>
                    </Box>
                  </Box>
                </>
              )}

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
                    These endpoints require you to manually specify whether your request is streaming or non-streaming. Prefer the vendor-native endpoint above, which routes both automatically; these remain only for existing integrations.
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
                  {/* A Tool is serialised with `description`; only datasources
                      carry `short_description`, so every tool showed the
                      placeholder even when it had a description. */}
                  {tool.attributes.short_description || tool.attributes.description || "No description available"}
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
