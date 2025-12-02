import React, { useState, useEffect, useCallback, useRef } from "react";
import { useParams, useNavigate, Link as RouterLink } from "react-router-dom";
import { useDebounce } from "use-debounce";
import apiClient, { appToolAPI } from "../../utils/apiClient"; // Import appToolAPI
import { formatBudgetDisplay } from "../../utils/budgetFormatter";
import agentService from "../../services/agentService";
import SearchInput from "../common/SearchInput";
import {
  Alert,
  Typography,
  CircularProgress,
  Box,
  Grid,
  Button,
  Divider,
  Chip,
  Table,
  TableBody,
  TableContainer,
  TableHead,
  TableRow,
  Snackbar,
  List,
  ListItem,
  ListItemText,
} from "@mui/material";
import WarningIcon from "@mui/icons-material/Warning";
import EditIcon from "@mui/icons-material/Edit";
import ArrowBackIcon from "@mui/icons-material/ArrowBack";
import { Line } from "react-chartjs-2";
import {
  Chart as ChartJS,
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  Title,
  Tooltip,
  Legend,
  TimeScale,
} from "chart.js";
import "chartjs-adapter-date-fns";
import {
  PrimaryOutlineButton,
  TitleBox,
  ContentBox,
  FieldLabel,
  FieldValue,
  PrimaryButton,
  StyledPaper,
  StyledTableCell,
  StyledTableHeaderCell,
  StyledTableRow,
  SecondaryLinkButton
} from "../../styles/sharedStyles";
import DateRangePicker from "../common/DateRangePicker";
import PaginationControls from "../common/PaginationControls";
import usePagination from "../../hooks/usePagination";
import ContentCopyIcon from "@mui/icons-material/ContentCopy";
import IconButton from "@mui/material/IconButton";
import DownloadIcon from "@mui/icons-material/Download";
import ExportProxyLogsModal from "../common/ExportProxyLogsModal";
import { useEdition } from "../../context/EditionContext";

ChartJS.register(
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  Title,
  Tooltip,
  Legend,
  TimeScale,
);

const SectionTitle = ({ children }) => (
  <Typography variant="h6" gutterBottom sx={{ mt: 3, mb: 2 }}>
    {children}
  </Typography>
);

const ExpandableMessage = ({ message, isCode = false }) => {
  const [expanded, setExpanded] = useState(false);

  const truncate = (str, n) => {
    return str.length > n ? str.substr(0, n - 1) + "..." : str;
  };

  const formatMessage = (msg) => {
    try {
      const parsed = JSON.parse(msg);
      return JSON.stringify(parsed, null, 2);
    } catch (e) {
      return msg;
    }
  };

  const displayMessage = expanded
    ? formatMessage(message)
    : truncate(message, 150);

  return (
    <Box>
      <Typography
        component={isCode ? "code" : "pre"}
        style={{
          whiteSpace: "pre-wrap",
          wordBreak: "break-word",
          backgroundColor: isCode ? "#f5f5f5" : "transparent",
          padding: isCode ? "8px" : "0",
          borderRadius: isCode ? "4px" : "0",
          fontFamily: isCode ? "monospace" : "inherit",
        }}
      >
        {displayMessage}
      </Typography>
      {message.length > 150 && (
        <Button onClick={() => setExpanded(!expanded)}>
          {expanded ? "Collapse" : "Expand"}
        </Button>
      )}
    </Box>
  );
};

const AppDetails = () => {
  const { isEnterprise } = useEdition();
  const [app, setApp] = useState(null);
  const [credential, setCredential] = useState(null);
  const [user, setUser] = useState(null);
  const [llms, setLLMs] = useState([]);
  const [datasources, setDatasources] = useState([]);
  const [tools, setTools] = useState([]); // Added state for tools
  const [agents, setAgents] = useState([]); // Added state for agents
  const [loading, setLoading] = useState(true);
  const [agentsLoading, setAgentsLoading] = useState(false);
  const [tokenUsageAndCostData, setTokenUsageAndCostData] = useState(null);
  const [budgetUsageData, setBudgetUsageData] = useState(null);
  const [appInteractionsData, setAppInteractionsData] = useState(null);
  const [proxyLogs, setProxyLogs] = useState([]);
  const [exportModalOpen, setExportModalOpen] = useState(false);
  const [startDate, setStartDate] = useState(
    new Date(new Date().getTime() - 30 * 24 * 60 * 60 * 1000)
      .toISOString()
      .split("T")[0],
  );
  const [endDate, setEndDate] = useState(
    new Date().toISOString().split("T")[0],
  );
  const [proxyLogSearchTerm, setProxyLogSearchTerm] = useState("");
  const [debouncedProxyLogSearch] = useDebounce(proxyLogSearchTerm, 500);
  const isFirstSearchRender = useRef(true);
  const { id } = useParams();
  const navigate = useNavigate();
  const [snackbar, setSnackbar] = useState({
    open: false,
    message: "",
    severity: "success",
  });

  const {
    page,
    pageSize,
    totalPages,
    handlePageChange,
    handlePageSizeChange,
    updatePaginationData,
  } = usePagination();

  const handleProxyLogSearch = useCallback((value) => {
    setProxyLogSearchTerm(value);
  }, []);

  // Define fetchProxyLogs before the useEffect that uses it
  const fetchProxyLogs = useCallback(async () => {
    try {
      const params = {
        start_date: startDate,
        end_date: endDate,
        app_id: id,
        page,
        page_size: pageSize,
      };

      // Only include search param if 2+ characters entered
      if (debouncedProxyLogSearch && debouncedProxyLogSearch.length >= 2) {
        params.search = debouncedProxyLogSearch;
      }

      const response = await apiClient.get(`/analytics/proxy-logs-for-app`, { params });
      setProxyLogs(response.data.data || []);
      updatePaginationData(
        response.data.meta?.total_count || 0,
        response.data.meta?.total_pages || 0,
      );
    } catch (error) {
      console.error("Error fetching proxy logs", error);
    }
  }, [startDate, endDate, id, page, pageSize, debouncedProxyLogSearch, updatePaginationData]);

  const fetchAppDetails = useCallback(async () => {
    setLoading(true);
    try {
      const response = await apiClient.get(`/apps/${id}`);
      const appData = response.data.data;
      setApp(appData);

      if (appData.attributes.credential_id) {
        fetchCredential(appData.attributes.credential_id);
      }

      fetchUser(appData.attributes.user_id);
      
      // Use the tools from the app response directly if available
      if (appData.attributes.tools && Array.isArray(appData.attributes.tools)) {
        setTools(appData.attributes.tools);
      } else {
        // Fallback to fetching tools separately if not in the main app response
        fetchAppTools(id);
      }
      
      // Assuming LLM and Datasource IDs are still part of the main app response
      // and you have separate functions to fetch their details by IDs.
      // If LLM/Datasource details are now part of AppResponse.Attributes like Tools, adjust accordingly.
      if (appData.attributes.llm_ids && Array.isArray(appData.attributes.llm_ids)) {
         fetchLLMsDetails(appData.attributes.llm_ids);
      } else if (appData.attributes.llms && Array.isArray(appData.attributes.llms)) {
         setLLMs(appData.attributes.llms); // If full LLM objects are now in AppResponse
      }

      if (appData.attributes.datasource_ids && Array.isArray(appData.attributes.datasource_ids)) {
        fetchDatasourcesDetails(appData.attributes.datasource_ids);
      } else if (appData.attributes.datasources && Array.isArray(appData.attributes.datasources)) {
        setDatasources(appData.attributes.datasources); // If full Datasource objects are now in AppResponse
      }


    } catch (error) {
      console.error("Error fetching app details", error);
      setSnackbar({ open: true, message: "Failed to load app details.", severity: "error" });
    } finally {
      setLoading(false);
    }
  }, [id]); // Removed fetchAppTools from here as it's called conditionally

  const fetchAppTools = useCallback(async (appId) => {
    try {
      const response = await appToolAPI.getAppTools(appId);
      setTools(response.data.data || []); // Assuming response.data.data is the array of tools
    } catch (error) {
      console.error("Error fetching app tools", error);
      setSnackbar({ open: true, message: "Failed to load tools for the app.", severity: "error" });
      setTools([]); // Set to empty array on error
    }
  }, []);
  
  // Adjust fetchLLMs and fetchDatasources if they now expect full objects or just IDs
  const fetchLLMsDetails = async (llmIds) => {
    if (!llmIds || llmIds.length === 0) {
        setLLMs([]);
        return;
    }
    try {
      const llmPromises = llmIds.map((llmId) => apiClient.get(`/llms/${llmId}`));
      const llmResponses = await Promise.all(llmPromises);
      setLLMs(llmResponses.map((response) => response.data.data));
    } catch (error) {
      console.error("Error fetching LLMs", error);
      setLLMs([]);
    }
  };

  const fetchDatasourcesDetails = async (datasourceIds) => {
    if (!datasourceIds || datasourceIds.length === 0) {
        setDatasources([]);
        return;
    }
    try {
      const datasourcePromises = datasourceIds.map((dsId) =>
        apiClient.get(`/datasources/${dsId}`),
      );
      const datasourceResponses = await Promise.all(datasourcePromises);
      setDatasources(datasourceResponses.map((response) => response.data.data));
    } catch (error) {
      console.error("Error fetching datasources", error);
      setDatasources([]);
    }
  };

  const fetchAgents = useCallback(async (appId) => {
    setAgentsLoading(true);
    try {
      const result = await agentService.listAgents(1, 100);
      const appAgents = result.data.filter(agent => agent.appId === parseInt(appId));
      setAgents(appAgents);
    } catch (error) {
      console.error("Error fetching agents", error);
      setAgents([]);
    } finally {
      setAgentsLoading(false);
    }
  }, []);


  useEffect(() => {
    fetchAppDetails();
    fetchTokenUsageAndCost();
    fetchAgents(id);
  }, [id, startDate, endDate, fetchAppDetails, fetchAgents]);

  // Separate effect for proxy logs to handle pagination and search independently
  useEffect(() => {
    fetchProxyLogs();
  }, [fetchProxyLogs]);

  // Reset to page 1 when proxy log search term changes (but not on initial render)
  useEffect(() => {
    if (isFirstSearchRender.current) {
      isFirstSearchRender.current = false;
      return;
    }
    handlePageChange(1);
  }, [debouncedProxyLogSearch, handlePageChange]);


  const handleCloseSnackbar = (event, reason) => {
    if (reason === "clickaway") {
      return;
    }
    setSnackbar({ ...snackbar, open: false });
  };

  const handleCopyToClipboard = async (text, fieldName) => {
    try {
      await navigator.clipboard.writeText(text);
      setSnackbar({
        open: true,
        message: `${fieldName} copied to clipboard`,
        severity: "success",
      });
    } catch (err) {
      setSnackbar({
        open: true,
        message: `Failed to copy ${fieldName}`,
        severity: "error",
      });
    }
  };

  const handleApproveApp = async () => {
    try {
      const credentialInput = {
        data: {
          type: "credentials",
          attributes: {
            active: true,
          },
        },
      };

      await apiClient.patch(`/credentials/${credential.id}`, credentialInput);

      setCredential((prevState) => ({
        ...prevState,
        attributes: {
          ...prevState.attributes,
          active: true,
        },
      }));

      setSnackbar({
        open: true,
        message: "App approved successfully",
        severity: "success",
      });
    } catch (error) {
      console.error("Error approving app", error);
      setSnackbar({
        open: true,
        message: "Failed to approve app. Please try again.",
        severity: "error",
      });
    }
  };

  const fetchTokenUsageAndCost = async () => {
    try {
      const [usageResponse, budgetResponse, interactionsResponse] = await Promise.all([
        apiClient.get(`/analytics/usage`, {
          params: { start_date: startDate, end_date: endDate, app_id: id },
        }),
        apiClient.get(`/analytics/budget-usage-for-app`, {
          params: { app_id: id },
        }),
        apiClient.get(`/analytics/app-interactions-over-time`, {
          params: { start_date: startDate, end_date: endDate, app_id: id },
        }),
      ]);
      setTokenUsageAndCostData(usageResponse.data);
      setBudgetUsageData(budgetResponse.data);
      setAppInteractionsData(interactionsResponse.data);
    } catch (error) {
      console.error("Error fetching usage and budget data", error);
    }
  };

  const fetchCredential = async (credentialId) => {
    try {
      const response = await apiClient.get(`/credentials/${credentialId}`);
      setCredential(response.data.data);
    } catch (error) {
      console.error("Error fetching credential", error);
    }
  };

  const fetchUser = async (userId) => {
    try {
      const response = await apiClient.get(`/users/${userId}`);
      setUser(response.data.data);
    } catch (error) {
      console.error("Error fetching user", error);
    }
  };

  if (loading) return <CircularProgress />;
  if (!app) return <Typography>App not found</Typography>;

  const tokenChartOptions = {
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
  };

  const costChartOptions = {
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
  };

  const interactionsChartOptions = {
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
          text: "Interactions Count",
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
  };

  const tokenChartData = {
    labels: tokenUsageAndCostData?.labels || [],
    datasets: [
      {
        label: "Prompt Tokens",
        data: tokenUsageAndCostData?.datasets[2]?.data || [],
        borderColor: "rgb(53, 162, 235)",
        backgroundColor: "rgba(53, 162, 235, 0.5)",
        fill: true,
      },
      {
        label: "Response Tokens",
        data: tokenUsageAndCostData?.datasets[3]?.data || [],
        borderColor: "rgb(75, 192, 192)",
        backgroundColor: "rgba(75, 192, 192, 0.5)",
        fill: true,
      },
      {
        label: "Cache Write Tokens",
        data: tokenUsageAndCostData?.datasets[4]?.data || [],
        borderColor: "rgb(255, 159, 64)",
        backgroundColor: "rgba(255, 159, 64, 0.5)",
        fill: true,
      },
      {
        label: "Cache Read Tokens",
        data: tokenUsageAndCostData?.datasets[5]?.data || [],
        borderColor: "rgb(153, 102, 255)",
        backgroundColor: "rgba(153, 102, 255, 0.5)",
        fill: true,
      },
    ],
  };

  const costChartData = {
    labels: tokenUsageAndCostData?.labels || [],
    datasets: [
      {
        label: "Cost",
        data: tokenUsageAndCostData?.datasets[1]?.data || [],
        borderColor: "rgb(255, 99, 132)",
        tension: 0.1,
      },
    ],
  };

  const interactionsChartData = {
    labels: appInteractionsData?.labels || [],
    datasets: [
      {
        label: "LLM Interactions",
        data: appInteractionsData?.data || [],
        borderColor: "rgb(54, 162, 235)",
        backgroundColor: "rgba(54, 162, 235, 0.2)",
        tension: 0.1,
        fill: true,
      },
    ],
  };

  const handleStartDateChange = (newDate) => {
    setStartDate(newDate);
  };

  const handleEndDateChange = (newDate) => {
    setEndDate(newDate);
  };

  return (
    <>
      <TitleBox top="64px">
        <Typography variant="headingXLarge">App details</Typography>
        <SecondaryLinkButton
          startIcon={<ArrowBackIcon />}
          onClick={() => navigate("/admin/apps")}
          color="inherit"
        >
          Back to apps
        </SecondaryLinkButton>
      </TitleBox>
      <ContentBox>
        <SectionTitle>Token Usage</SectionTitle>
        <Box height={300} mb={4}>
          <Line options={tokenChartOptions} data={tokenChartData} />
        </Box>

        <SectionTitle>Cost</SectionTitle>
        <Box height={300} mb={4}>
          <Line options={costChartOptions} data={costChartData} />
        </Box>

        <SectionTitle>App Interactions</SectionTitle>
        <Box height={300} mb={4}>
          <Line options={interactionsChartOptions} data={interactionsChartData} />
        </Box>
        <Box mt={2} mb={4}>
          <DateRangePicker
            startDate={startDate}
            endDate={endDate}
            onStartDateChange={handleStartDateChange}
            onEndDateChange={handleEndDateChange}
          />
        </Box>

        <Divider sx={{ my: 3 }} />

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
            <FieldLabel>User:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <FieldValue>
              {app.attributes.is_orphaned ? (
                <Box display="flex" alignItems="center" gap={1}>
                  <Chip
                    icon={<WarningIcon />}
                    label="Orphaned App"
                    color="warning"
                    size="small"
                    variant="outlined"
                  />
                  <Typography variant="caption" color="text.secondary">
                    (Original user has been deleted)
                  </Typography>
                </Box>
              ) : user ? (
                user.attributes.name
              ) : (
                "Loading..."
              )}
            </FieldValue>
          </Grid>
          <Grid item xs={3}>
            <FieldLabel>LLMs:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <Box display="flex" flexWrap="wrap" gap={1}>
              {llms.map((llm) => (
                <Chip key={llm.id} label={llm.attributes.name} />
              ))}
            </Box>
          </Grid>
          <Grid item xs={3}>
            <FieldLabel>Datasources:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <Box display="flex" flexWrap="wrap" gap={1}>
              {datasources.map((datasource) => (
                <Chip key={datasource.id} label={datasource.attributes.name} />
              ))}
            </Box>
          </Grid>
          <Grid item xs={3}>
            <FieldLabel>Tools:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <Box display="flex" flexWrap="wrap" gap={1}>
              {tools.length > 0 ? (
                tools.map((tool) => (
                  <Chip key={tool.id || `tool-${Math.random()}`} label={tool.attributes?.name || 'Unnamed tool'} />
                ))
              ) : (
                <Typography variant="body2">No tools associated.</Typography>
              )}
            </Box>
          </Grid>
          <Grid item xs={3}>
            <FieldLabel>Agents:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            {agentsLoading ? (
              <CircularProgress size={20} />
            ) : agents.length > 0 ? (
              <Box display="flex" flexWrap="wrap" gap={1}>
                {agents.map((agent) => (
                  <Chip
                    key={agent.id}
                    label={agent.name}
                    component={RouterLink}
                    to={`/admin/agents/${agent.id}`}
                    clickable
                    color={agent.isActive ? 'primary' : 'default'}
                  />
                ))}
              </Box>
            ) : (
              <Typography variant="body2">No agents using this app.</Typography>
            )}
          </Grid>
          <Grid item xs={3}>
            <FieldLabel>Monthly Budget:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <FieldValue>
              {formatBudgetDisplay({
                monthlyBudget: app.attributes.monthly_budget,
                currentUsage: budgetUsageData?.current_usage,
                percentage: budgetUsageData?.percentage,
                budgetStartDate: app.attributes.budget_start_date || budgetUsageData?.start_date
              })}
            </FieldValue>
          </Grid>
        </Grid>

        {credential && (
          <>
            <Divider sx={{ my: 3 }} />
            <SectionTitle>Credential Information</SectionTitle>
            <Grid container spacing={2}>
              <Grid item xs={3}>
                <FieldLabel>Key ID:</FieldLabel>
              </Grid>
              <Grid item xs={9}>
                <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                  <FieldValue>{credential.attributes.key_id}</FieldValue>
                  <IconButton
                    size="small"
                    onClick={() =>
                      handleCopyToClipboard(
                        credential.attributes.key_id,
                        "Key ID",
                      )
                    }
                    sx={{ ml: 1 }}
                  >
                    <ContentCopyIcon fontSize="small" />
                  </IconButton>
                </Box>
              </Grid>
              <Grid item xs={3}>
                <FieldLabel>Secret:</FieldLabel>
              </Grid>
              <Grid item xs={9}>
                <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                  <FieldValue>********</FieldValue>
                  <IconButton
                    size="small"
                    onClick={() =>
                      handleCopyToClipboard(
                        credential.attributes.secret,
                        "Secret",
                      )
                    }
                    sx={{ ml: 1 }}
                  >
                    <ContentCopyIcon fontSize="small" />
                  </IconButton>
                </Box>
              </Grid>
              <Grid item xs={3}>
                <FieldLabel>Active:</FieldLabel>
              </Grid>
              <Grid item xs={9}>
                <FieldValue>
                  {credential.attributes.active ? "Yes" : "No"}
                </FieldValue>
              </Grid>
              {!credential.attributes.active && (
                <Grid item xs={12}>
                  <Box mt={2}>
                    <PrimaryOutlineButton
                      variant="contained"
                      color="primary"
                      onClick={handleApproveApp}
                    >
                      Approve this App
                    </PrimaryOutlineButton>
                  </Box>
                </Grid>
              )}
            </Grid>
          </>
        )}

        <Divider sx={{ my: 3 }} />

        <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 2 }}>
          <SectionTitle>Proxy Logs</SectionTitle>
          {isEnterprise && (
            <Button
              variant="outlined"
              startIcon={<DownloadIcon />}
              onClick={() => setExportModalOpen(true)}
              size="small"
            >
              Export
            </Button>
          )}
        </Box>
        <Box sx={{ mb: 2, maxWidth: 400 }}>
          <SearchInput
            value={proxyLogSearchTerm}
            onChange={handleProxyLogSearch}
            placeholder="Search request or response..."
          />
        </Box>
        <StyledPaper>
          <TableContainer sx={{ maxWidth: "100%", overflowX: "auto" }}>
            <Table sx={{ tableLayout: "fixed", width: "100%" }}>
              <TableHead>
                <TableRow>
                  <StyledTableHeaderCell sx={{ verticalAlign: "top", width: "15%" }}>
                    Timestamp
                  </StyledTableHeaderCell>
                  <StyledTableHeaderCell sx={{ verticalAlign: "top", width: "10%" }}>
                    Vendor
                  </StyledTableHeaderCell>
                  <StyledTableHeaderCell sx={{ verticalAlign: "top", width: "10%" }}>
                    Response Code
                  </StyledTableHeaderCell>
                  <StyledTableHeaderCell sx={{ verticalAlign: "top", width: "32.5%" }}>
                    Request
                  </StyledTableHeaderCell>
                  <StyledTableHeaderCell sx={{ verticalAlign: "top", width: "32.5%" }}>
                    Response
                  </StyledTableHeaderCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {proxyLogs?.length === 0 && debouncedProxyLogSearch ? (
                  <TableRow>
                    <StyledTableCell colSpan={5} align="center">
                      No proxy logs found matching "{debouncedProxyLogSearch}"
                    </StyledTableCell>
                  </TableRow>
                ) : proxyLogs?.length > 0 ? (
                  proxyLogs.map((log) => (
                    <StyledTableRow key={log.id}>
                      <StyledTableCell sx={{ verticalAlign: "top" }}>
                        {new Date(log.attributes.time_stamp).toLocaleString()}
                      </StyledTableCell>
                      <StyledTableCell sx={{ verticalAlign: "top" }}>
                        {log.attributes.vendor}
                      </StyledTableCell>
                      <StyledTableCell sx={{ verticalAlign: "top" }}>
                        {log.attributes.response_code}
                      </StyledTableCell>
                      <StyledTableCell sx={{ verticalAlign: "top", overflow: "hidden" }}>
                        <Box sx={{ overflow: "auto", maxHeight: 200 }}>
                          <pre style={{ margin: 0, whiteSpace: "pre-wrap", wordBreak: "break-word" }}>
                            <code>
                              <ExpandableMessage
                                message={log.attributes.request_body}
                              />
                            </code>
                          </pre>
                        </Box>
                      </StyledTableCell>
                      <StyledTableCell sx={{ verticalAlign: "top", overflow: "hidden" }}>
                        <Box sx={{ overflow: "auto", maxHeight: 200 }}>
                          <pre style={{ margin: 0, whiteSpace: "pre-wrap", wordBreak: "break-word" }}>
                            <code>
                              <ExpandableMessage
                                message={log.attributes.response_body}
                              />
                            </code>
                          </pre>
                        </Box>
                      </StyledTableCell>
                    </StyledTableRow>
                  ))
                ) : (
                  <TableRow>
                    <StyledTableCell colSpan={5} align="center">
                      No proxy logs available for the selected period.
                    </StyledTableCell>
                  </TableRow>
                )}
              </TableBody>
            </Table>
          </TableContainer>
          <PaginationControls
            page={page}
            pageSize={pageSize}
            totalPages={totalPages}
            onPageChange={handlePageChange}
            onPageSizeChange={handlePageSizeChange}
          />
        </StyledPaper>

        <Box
          mt={4}
          display="flex"
          justifyContent="space-between"
          alignItems="center"
        >
          <PrimaryButton
            variant="contained"
            startIcon={<EditIcon />}
            onClick={() => navigate(`/admin/apps/edit/${id}`)}
          >
            Edit app
          </PrimaryButton>
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

      <ExportProxyLogsModal
        open={exportModalOpen}
        onClose={() => setExportModalOpen(false)}
        sourceType="app"
        sourceId={parseInt(id)}
        initialStartDate={startDate}
        initialEndDate={endDate}
        initialSearch={debouncedProxyLogSearch}
      />
    </>
  );
};

export default AppDetails;
