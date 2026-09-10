import GovernedMetadataSummary from "../metadata/GovernedMetadataSummary";
import React, { useState, useEffect, useMemo, useCallback, useRef } from "react";
import { useParams, useNavigate } from "react-router-dom";
import { useDebounce } from "use-debounce";
import apiClient from "../../utils/apiClient";
import SearchInput from "../common/SearchInput";
import {
  Typography,
  CircularProgress,
  Box,
  Grid,
  Button,
  IconButton,
  Tooltip,
  Link,
  Divider,
  Table,
  TableBody,
  TableContainer,
  TableHead,
  TableRow,
  TableSortLabel,
} from "@mui/material";
import EditIcon from "@mui/icons-material/Edit";
import PriceChangeIcon from "@mui/icons-material/PriceChange";
import ArrowBackIcon from "@mui/icons-material/ArrowBack";
import ContentCopyIcon from "@mui/icons-material/ContentCopy";
import { CredentialStatusNotice } from "./CredentialStatusIndicator";
import FiberManualRecordIcon from "@mui/icons-material/FiberManualRecord";
import HelpOutlineIcon from "@mui/icons-material/HelpOutline";
import DownloadIcon from "@mui/icons-material/Download";
import ExportProxyLogsModal from "../common/ExportProxyLogsModal";
import { useEdition } from "../../context/EditionContext";
import {
  Chart as ChartJS,
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  Title,
  Tooltip as ChartTooltip,
  Legend,
  TimeScale,
} from "chart.js";
import { Line } from "react-chartjs-2";
import "chartjs-adapter-date-fns";
import DateRangePicker from "../../components/common/DateRangePicker";
import PaginationControls from "../common/PaginationControls";
import usePagination from "../../hooks/usePagination";
import {
  StyledPaper,
  TitleBox,
  ContentBox,
  FieldLabel,
  FieldValue,
  PrimaryButton,
  StyledTableCell,
  StyledTableHeaderCell,
  StyledTableRow,
  SecondaryLinkButton
} from "../../styles/sharedStyles";
import { getVendorName, getVendorLogo } from "../../utils/vendorLogos";
import Chip from "@mui/material/Chip";
import { useTheme } from "@mui/material/styles";
import { formatBudgetDisplay } from "../../utils/budgetFormatter";
import {
  tokenChartOptions,
  costChartOptions,
  buildTokenChartData,
  buildCostChartData,
} from "./usageCharts";
import {
  sortUsageRows,
  nextSortConfig,
  formatUsageTime,
  formatTokens,
  formatCost,
  modelDetailPath,
} from "../../utils/modelUsage";

// Column types drive sort behaviour for the "Models in use" table.
const MODEL_COLUMN_TYPES = {
  model: "string",
  requestCount: "number",
  appCount: "number",
  lastUsed: "date",
  totalCost: "number",
  promptTokens: "number",
  responseTokens: "number",
};

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

ChartJS.register(
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  Title,
  ChartTooltip,
  Legend,
  TimeScale,
);

const SectionTitle = ({ children }) => (
  <Typography variant="h6" gutterBottom sx={{ mt: 3, mb: 2 }}>
    {children}
  </Typography>
);

const LLMDetails = () => {
  const { isEnterprise } = useEdition();
  const [llm, setLLM] = useState(null);
  const [loading, setLoading] = useState(true);
  const [copySuccess, setCopySuccess] = useState("");
  const [vendorUsageData, setVendorUsageData] = useState(null);
  const [budgetUsageData, setBudgetUsageData] = useState(null);
  const [vendorModelCostData, setVendorModelCostData] = useState([]);
  const [modelSort, setModelSort] = useState({ field: "lastUsed", direction: "desc" });
  const [proxyLogs, setProxyLogs] = useState([]);
  const [isTableExpanded, setIsTableExpanded] = useState(false);
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

  const {
    page,
    pageSize,
    totalPages,
    handlePageChange,
    handlePageSizeChange,
    updatePaginationData,
  } = usePagination();

  const apiEndpointPlaceholder = "API Endpoint not set";
  const apiKeyPlaceholder = "API Key not set";
  const theme = useTheme();

  const handleProxyLogSearch = useCallback((value) => {
    setProxyLogSearchTerm(value);
  }, []);

  // Define fetchProxyLogs before the useEffect that uses it
  const fetchProxyLogs = useCallback(async () => {
    try {
      const params = {
        start_date: startDate,
        end_date: endDate,
        llm_id: id,
        page,
        page_size: pageSize,
      };

      // Only include search param if 2+ characters entered
      if (debouncedProxyLogSearch && debouncedProxyLogSearch.length >= 2) {
        params.search = debouncedProxyLogSearch;
      }

      const response = await apiClient.get(`/analytics/proxy-logs-for-llm`, { params });
      setProxyLogs(response.data.data);
      updatePaginationData(
        response.data.meta.total_count,
        response.data.meta.total_pages,
      );
    } catch (error) {
      console.error("Error fetching proxy logs", error);
    }
  }, [startDate, endDate, id, page, pageSize, debouncedProxyLogSearch, updatePaginationData]);

  useEffect(() => {
    fetchLLMDetails();
  }, [id]);

  useEffect(() => {
    if (llm) {
      fetchVendorUsage();
      fetchVendorModelCost();

      // Initialize budget usage with 0 if no monthly budget
      if (!llm.attributes.monthly_budget) {
        setBudgetUsageData({
          current_usage: 0,
          percentage: 0,
          start_date: llm.attributes.budget_start_date || startDate,
        });
      }
    }
  }, [llm, startDate, endDate]);

  // Separate effect for proxy logs to handle pagination and search independently
  useEffect(() => {
    if (llm) {
      fetchProxyLogs();
    }
  }, [llm, fetchProxyLogs]);

  // Reset to page 1 when proxy log search term changes (but not on initial render)
  useEffect(() => {
    if (isFirstSearchRender.current) {
      isFirstSearchRender.current = false;
      return;
    }
    handlePageChange(1);
  }, [debouncedProxyLogSearch, handlePageChange]);

  const fetchVendorModelCost = async () => {
    try {
      const response = await apiClient.get("/analytics/total-cost-per-vendor-and-model", {
        params: {
          start_date: startDate,
          end_date: endDate,
          llm_id: id
        },
      });
      setVendorModelCostData(response.data);
    } catch (error) {
      console.error("Error fetching vendor model cost data", error);
    }
  };

  const toggleTableExpansion = () => {
    setIsTableExpanded(!isTableExpanded);
  };

  const fetchLLMDetails = async () => {
    try {
      const response = await apiClient.get(`/llms/${id}`);
      setLLM(response.data.data);
      setLoading(false);
    } catch (error) {
      console.error("Error fetching LLM details", error);
      setLoading(false);
    }
  };

  const fetchVendorUsage = async () => {
    try {
      const [usageResponse, budgetResponse] = await Promise.all([
        apiClient.get(`/analytics/usage`, {
          params: {
            start_date: startDate,
            end_date: endDate,
            vendor: llm.attributes.vendor,
            llm_id: llm.id,
          },
        }),
        apiClient.get(`/analytics/budget-usage`, {
          params: {
            start_date: startDate,
            end_date: endDate,
            llm_id: llm.id
          },
        })
      ]);

      setVendorUsageData(usageResponse.data);

      // Find the budget usage data for this LLM
      const llmBudgetData = budgetResponse.data.find(item =>
        item.type === "LLM" && item.entity_id === llm.id
      );

      if (llmBudgetData) {
        setBudgetUsageData({
          current_usage: llmBudgetData.currentUsage,
          percentage: llmBudgetData.usagePercent,
          total_cost: llmBudgetData.totalCost,
          start_date: llmBudgetData.budgetStartDate || llm.attributes.budget_start_date || startDate,
        });
      } else if (llm.attributes.monthly_budget) {
        // Fallback to calculating from vendor usage if budget data is not found
        const totalCost = usageResponse.data.cost?.reduce((sum, cost) => sum + cost, 0) || 0;
        setBudgetUsageData({
          current_usage: totalCost,
          percentage: (totalCost / llm.attributes.monthly_budget) * 100,
          total_cost: totalCost,
          start_date: llm.attributes.budget_start_date || startDate,
        });
      }
    } catch (error) {
      console.error("Error fetching usage data", error);
    }
  };

  const copyToClipboard = (text, field) => {
    navigator.clipboard.writeText(text).then(
      () => {
        setCopySuccess(`${field} copied!`);
        setTimeout(() => setCopySuccess(""), 2000);
      },
      (err) => {
        console.error("Could not copy text: ", err);
      },
    );
  };

  const tokenChartData = useMemo(() => buildTokenChartData(vendorUsageData), [vendorUsageData]);
  const costChartData = useMemo(() => buildCostChartData(vendorUsageData), [vendorUsageData]);

  const sortedModelRows = useMemo(
    () => sortUsageRows(vendorModelCostData, modelSort, MODEL_COLUMN_TYPES),
    [vendorModelCostData, modelSort],
  );

  const handleModelSort = (field) => {
    setModelSort((current) => nextSortConfig(current, field, MODEL_COLUMN_TYPES));
  };

  const modelSortLabel = (field, label, align = "left") => (
    <StyledTableHeaderCell align={align} sortDirection={modelSort.field === field ? modelSort.direction : false}>
      <TableSortLabel
        active={modelSort.field === field}
        direction={modelSort.field === field ? modelSort.direction : "desc"}
        onClick={() => handleModelSort(field)}
      >
        {label}
      </TableSortLabel>
    </StyledTableHeaderCell>
  );

  if (loading) return <CircularProgress />;
  if (!llm) return <Typography>LLM not found</Typography>;

  return (
    <>
      <TitleBox top="64px">
        <Typography variant="headingXLarge">LLM provider details</Typography>
        <SecondaryLinkButton
          startIcon={<ArrowBackIcon />}
          onClick={() => navigate("/admin/llms")}
          color="inherit"
        >
          Back to LLMs
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
        <Box mt={2}>
          <DateRangePicker
            startDate={startDate}
            endDate={endDate}
            onStartDateChange={setStartDate}
            onEndDateChange={setEndDate}
            onUpdate={fetchVendorUsage}
            updateMode="immediate"
          />
        </Box>

        <StyledPaper elevation={3} style={{ padding: "20px", marginTop: "20px", marginBottom: "20px" }}>
          <Box display="flex" justifyContent="space-between" alignItems="flex-start" flexWrap="wrap" gap={1} mb={1}>
            <Box>
              <Typography variant="h6">Models in use</Typography>
              <Typography variant="body2" color="text.secondary">
                Every model this provider served in the selected period. Click a model to see which apps are calling it.
              </Typography>
            </Box>
            <Button
              size="small"
              startIcon={<PriceChangeIcon />}
              onClick={() => navigate("/admin/model-prices")}
            >
              Manage model prices
            </Button>
          </Box>
          {vendorModelCostData.length > 0 ? (
            <>
              <TableContainer sx={{ overflowX: "auto" }}>
                <Table size="small" data-testid="models-in-use-table">
                  <TableHead>
                    <TableRow>
                      {modelSortLabel("model", "Model")}
                      {modelSortLabel("requestCount", "Requests", "right")}
                      {modelSortLabel("appCount", "Apps", "right")}
                      {modelSortLabel("lastUsed", "Last used", "right")}
                      {modelSortLabel("totalCost", "Total cost", "right")}
                      {modelSortLabel("promptTokens", "Request tokens", "right")}
                      {modelSortLabel("responseTokens", "Response tokens", "right")}
                    </TableRow>
                  </TableHead>
                  <TableBody>
                    {sortedModelRows
                      .slice(0, isTableExpanded ? undefined : 5)
                      .map((row) => {
                        const lastUsed = formatUsageTime(row.lastUsed);
                        return (
                          <StyledTableRow key={`${row.vendor || ""}:${row.model}`}>
                            <StyledTableCell>
                              <Link
                                component="button"
                                variant="body2"
                                underline="hover"
                                sx={{ fontFamily: "monospace", textAlign: "left" }}
                                onClick={() => navigate(modelDetailPath(id, row.model))}
                              >
                                {row.model}
                              </Link>
                              {row.modelPriceId && (
                                <Tooltip title="Edit model price">
                                  <IconButton
                                    size="small"
                                    aria-label={`Edit price for ${row.model}`}
                                    onClick={() => navigate(`/admin/model-prices/${row.modelPriceId}`)}
                                    sx={{ ml: 0.5 }}
                                  >
                                    <PriceChangeIcon fontSize="inherit" />
                                  </IconButton>
                                </Tooltip>
                              )}
                            </StyledTableCell>
                            <StyledTableCell align="right">{formatTokens(row.requestCount)}</StyledTableCell>
                            <StyledTableCell align="right">{formatTokens(row.appCount)}</StyledTableCell>
                            <StyledTableCell align="right">
                              <Tooltip title={lastUsed.absolute} placement="top">
                                <span>{lastUsed.relative}</span>
                              </Tooltip>
                              <div style={{ fontSize: "0.85em", color: "gray" }}>{lastUsed.absolute}</div>
                            </StyledTableCell>
                            <StyledTableCell align="right">
                              <div style={{ marginBottom: "4px" }}>{formatCost(row.totalCost)}</div>
                              <div style={{ fontSize: "0.85em", color: "gray" }}>
                                (Prompt: {row.promptCost.toFixed(2)}, CW: {row.cacheWriteCost.toFixed(2)}, CR: {row.cacheReadCost.toFixed(2)}, Resp: {row.responseCost.toFixed(2)})
                              </div>
                            </StyledTableCell>
                            <StyledTableCell align="right">
                              <div style={{ marginBottom: "4px" }}>
                                {formatTokens(row.promptTokens + row.cacheWriteTokens + row.cacheReadTokens)}
                              </div>
                              <div style={{ fontSize: "0.85em", color: "gray" }}>
                                (Prompt: {formatTokens(row.promptTokens)}, CW: {formatTokens(row.cacheWriteTokens)}, CR: {formatTokens(row.cacheReadTokens)})
                              </div>
                            </StyledTableCell>
                            <StyledTableCell align="right">{formatTokens(row.responseTokens)}</StyledTableCell>
                          </StyledTableRow>
                        );
                      })}
                  </TableBody>
                </Table>
              </TableContainer>
              {vendorModelCostData.length > 5 && (
                <Box mt={2} textAlign="center">
                  <Button onClick={toggleTableExpansion}>
                    {isTableExpanded ? "Collapse" : `Show all ${vendorModelCostData.length} models`}
                  </Button>
                </Box>
              )}
            </>
          ) : (
            <Box
              display="flex"
              flexDirection="column"
              alignItems="center"
              justifyContent="center"
              height="100%"
              py={4}
            >
              <Typography variant="body1" color="text.secondary">
                No models were called through this provider in the selected period.
              </Typography>
            </Box>
          )}
        </StyledPaper>

        <Divider sx={{ my: 3 }} />

        <SectionTitle>LLM Description</SectionTitle>
        <Grid container spacing={2}>
          <Grid item xs={3}>
            <FieldLabel>Active:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <FieldValue>{llm.attributes.active ? "Yes" : "No"}</FieldValue>
          </Grid>
          <Grid item xs={3}>
            <FieldLabel>Body Logging Disabled:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <FieldValue>{llm.attributes.dont_log_bodies ? "Yes" : "No"}</FieldValue>
          </Grid>
          <Grid item xs={3}>
            <FieldLabel>Short Description:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <FieldValue>{llm.attributes.short_description}</FieldValue>
          </Grid>
          <Grid item xs={3}>
            <FieldLabel>Vendor:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <Box sx={{ display: "flex", alignItems: "center" }}>
              <img
                src={getVendorLogo(llm.attributes.vendor)}
                alt={getVendorName(llm.attributes.vendor)}
                style={{
                  width: 24,
                  height: 24,
                  marginRight: 8,
                  objectFit: "contain",
                }}
              />
              <FieldValue>{getVendorName(llm.attributes.vendor)}</FieldValue>
            </Box>
          </Grid>
          <Grid item xs={3}>
            <FieldLabel>Privacy Level:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <Box sx={{ display: "flex", alignItems: "center" }}>
              <FieldValue>{llm.attributes.privacy_score}</FieldValue>
              <Tooltip
                title="Privacy level is a value between 0 and 100, where 0 is the lowest and 100 is the highest. This determines the privacy level of the LLM for Data Source sharing."
                placement="top"
              >
                <HelpOutlineIcon
                  sx={{ ml: 1, fontSize: 20, color: "text.secondary" }}
                />
              </Tooltip>
            </Box>
          </Grid>
          <Grid item xs={3}>
            <FieldLabel>Monthly Budget:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <FieldValue>
              {formatBudgetDisplay({
                monthlyBudget: llm.attributes.monthly_budget,
                currentUsage: budgetUsageData?.current_usage,
                percentage: budgetUsageData?.percentage,
                budgetStartDate: llm.attributes.budget_start_date || budgetUsageData?.start_date
              })}
            </FieldValue>
          </Grid>
        </Grid>

        <Divider sx={{ my: 3 }} />

        <SectionTitle>Access Details</SectionTitle>
        <Typography variant="body2" color="text.secondary" paragraph>
          Some LLMs do not require an API Key for access, or have a default URL
          (for example Anthropic and OpenAI). If you have an LLM provider that
          is not on the list, but provides an OpenAPI compatible API, you can
          use the compatible vendor setting and override the default URL.
        </Typography>
        <Grid container spacing={2}>
          <Grid item xs={3}>
            <FieldLabel>API Endpoint:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <Box sx={{ display: "flex", alignItems: "center" }}>
              <FieldValue>
                {llm.attributes.api_endpoint || apiEndpointPlaceholder}
              </FieldValue>
              {llm.attributes.api_endpoint && (
                <Tooltip title="Copy to clipboard" placement="top">
                  <IconButton
                    onClick={() =>
                      copyToClipboard(
                        llm.attributes.api_endpoint,
                        "API Endpoint",
                      )
                    }
                  >
                    <ContentCopyIcon />
                  </IconButton>
                </Tooltip>
              )}
            </Box>
          </Grid>
          <Grid item xs={3}>
            <FieldLabel>API Key:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <Box sx={{ display: "flex", alignItems: "center" }}>
              <FieldValue>
                {llm.attributes.api_key ? "*".repeat(20) : apiKeyPlaceholder}
              </FieldValue>
              {llm.attributes.api_key && (
                <Tooltip title="Copy to clipboard" placement="top">
                  <IconButton
                    onClick={() =>
                      copyToClipboard(llm.attributes.api_key, "API Key")
                    }
                  >
                    <ContentCopyIcon />
                  </IconButton>
                </Tooltip>
              )}
            </Box>
            {/* Says whether the key can actually resolve. A provider pointing
                at an empty bootstrap secret used to render as fully healthy. */}
            <Box sx={{ mt: 1 }}>
              <CredentialStatusNotice
                status={llm.attributes.credential_status}
                reference={llm.attributes.credential_ref}
              />
            </Box>
          </Grid>
        </Grid>

        <Divider sx={{ my: 3 }} />

        <SectionTitle>Model Configuration</SectionTitle>
        <Typography variant="body2" color="text.secondary" paragraph>
          The following model patterns are allowed for this LLM. These patterns
          are used to validate model requests through the API Gateway.
        </Typography>
        <Grid container spacing={2}>
          <Grid item xs={3}>
            <FieldLabel>Default Model:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <FieldValue>
              {llm.attributes.default_model || "No default model set"}
            </FieldValue>
          </Grid>
          <Grid item xs={3}>
            <FieldLabel>Allowed Models:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            {llm.attributes.allowed_models &&
              llm.attributes.allowed_models.length > 0 ? (
              <Box sx={{ display: "flex", flexWrap: "wrap", gap: 1 }}>
                {llm.attributes.allowed_models.map((model, index) => (
                  <Chip
                    key={index}
                    label={model}
                    color="primary"
                    variant="outlined"
                    sx={{
                      backgroundColor: theme.palette.background.paper,
                      "& .MuiChip-label": {
                        color: theme.palette.text.primary,
                      },
                    }}
                  />
                ))}
              </Box>
            ) : (
              // An empty list is not a deny-all: it permits everything.
              <FieldValue>All models allowed (no patterns specified)</FieldValue>
            )}
            <Typography
              variant="caption"
              color="text.secondary"
              sx={{ display: "block", mt: 1 }}
            >
              These patterns use regex matching to determine which models are
              allowed, matched anywhere in the model name. For example,
              "gpt-4.*" allows all GPT-4 models — and also matches
              "legacy-gpt-4o". Anchor with ^ and $ to match the whole name.
            </Typography>
          </Grid>
        </Grid>

        <Divider sx={{ my: 3 }} />

        <SectionTitle>Portal Display Information</SectionTitle>
        <Typography variant="body2" color="text.secondary" paragraph>
          The following settings will be used in the Portal UI that your
          end-users / developers will see when browsing for LLMs to use.
        </Typography>
        <Grid container spacing={2}>
          <Grid item xs={3}>
            <FieldLabel>Logo URL:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <Box sx={{ display: "flex", alignItems: "center" }}>
              <img
                src={llm.attributes.logo_url}
                alt="LLM Logo"
                style={{
                  width: 50,
                  height: 50,
                  marginRight: 8,
                  objectFit: "contain",
                }}
              />
              <Box
                component="a"
                href={llm.attributes.logo_url}
                target="_blank"
                rel="noopener noreferrer"
                sx={{
                  maxWidth: "300px",
                  overflow: "hidden",
                  textOverflow: "ellipsis",
                  whiteSpace: "nowrap",
                  textDecoration: "none",
                  color: "inherit",
                  "&:hover": {
                    textDecoration: "underline"
                  }
                }}
              >
                {llm.attributes.logo_url}
              </Box>
            </Box>
          </Grid>
          <Grid item xs={3}>
            <FieldLabel>Loaded into Gateway:</FieldLabel>
          </Grid>
          <Grid item xs={9}>
            <FieldValue>
              <FiberManualRecordIcon
                sx={{
                  color: llm.attributes.active ? "green" : "red",
                  verticalAlign: "middle",
                  marginRight: 1,
                }}
              />
              {llm.attributes.active ? "Active" : "Inactive"}
            </FieldValue>
          </Grid>
        </Grid>

        <GovernedMetadataSummary
          objectType="llm"
          values={llm.governed_metadata}
          status={llm.governed_metadata_status}
        />

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
              ) : (
                proxyLogs?.map((log) => (
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
          <Typography color="success.main">{copySuccess}</Typography>
          <PrimaryButton
            variant="contained"
            startIcon={<EditIcon />}
            onClick={() => navigate(`/admin/llms/edit/${id}`)}
          >
            Edit LLM
          </PrimaryButton>
        </Box>
      </ContentBox>

      <ExportProxyLogsModal
        open={exportModalOpen}
        onClose={() => setExportModalOpen(false)}
        sourceType="llm"
        sourceId={parseInt(id)}
        initialStartDate={startDate}
        initialEndDate={endDate}
        initialSearch={debouncedProxyLogSearch}
      />
    </>
  );
};

export default LLMDetails;
