import React, { useState, useEffect, useMemo } from "react";
import { useParams, useNavigate } from "react-router-dom";
import apiClient from "../../utils/apiClient";
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
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
} from "@mui/material";
import EditIcon from "@mui/icons-material/Edit";
import ArrowBackIcon from "@mui/icons-material/ArrowBack";
import ContentCopyIcon from "@mui/icons-material/ContentCopy";
import FiberManualRecordIcon from "@mui/icons-material/FiberManualRecord";
import HelpOutlineIcon from "@mui/icons-material/HelpOutline";
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
  const [llm, setLLM] = useState(null);
  const [loading, setLoading] = useState(true);
  const [copySuccess, setCopySuccess] = useState("");
  const [vendorUsageData, setVendorUsageData] = useState(null);
  const [budgetUsageData, setBudgetUsageData] = useState(null);
  const [vendorModelCostData, setVendorModelCostData] = useState([]);
  const [proxyLogs, setProxyLogs] = useState([]);
  const [isTableExpanded, setIsTableExpanded] = useState(false);
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

  useEffect(() => {
    fetchLLMDetails();
  }, [id]);

  useEffect(() => {
    if (llm) {
      fetchVendorUsage();
      fetchProxyLogs();
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
  }, [llm, startDate, endDate, page, pageSize]);

  const fetchVendorModelCost = async () => {
    try {
      const response = await apiClient.get("/analytics/total-cost-per-vendor-and-model", {
        params: {
          start_date: startDate,
          end_date: endDate,
          llm_id: id,
          interaction_type: "ChatInteraction"
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

  const fetchProxyLogs = async () => {
    try {
      const response = await apiClient.get(`/analytics/proxy-logs-for-llm`, {
        params: {
          start_date: startDate,
          end_date: endDate,
          llm_id: id,
          page,
          page_size: pageSize,
        },
      });
      setProxyLogs(response.data.data);
      updatePaginationData(
        response.data.meta.total_count,
        response.data.meta.total_pages,
      );
    } catch (error) {
      console.error("Error fetching proxy logs", error);
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

  const tokenChartOptions = useMemo(() => ({
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
  }), []);

  const costChartOptions = useMemo(() => ({
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
  }), []);

  const tokenChartData = useMemo(() => ({
    labels: vendorUsageData?.labels || [],
    datasets: [
      {
        label: "Prompt Tokens",
        data: vendorUsageData?.datasets[2]?.data || [],
        borderColor: "rgb(53, 162, 235)",
        backgroundColor: "rgba(53, 162, 235, 0.5)",
        fill: true,
      },
      {
        label: "Response Tokens",
        data: vendorUsageData?.datasets[3]?.data || [],
        borderColor: "rgb(75, 192, 192)",
        backgroundColor: "rgba(75, 192, 192, 0.5)",
        fill: true,
      },
      {
        label: "Cache Write Tokens",
        data: vendorUsageData?.datasets[4]?.data || [],
        borderColor: "rgb(255, 159, 64)",
        backgroundColor: "rgba(255, 159, 64, 0.5)",
        fill: true,
      },
      {
        label: "Cache Read Tokens",
        data: vendorUsageData?.datasets[5]?.data || [],
        borderColor: "rgb(153, 102, 255)",
        backgroundColor: "rgba(153, 102, 255, 0.5)",
        fill: true,
      },
    ],
  }), [vendorUsageData]);

  const costChartData = useMemo(() => ({
    labels: vendorUsageData?.labels || [],
    datasets: [
      {
        ...vendorUsageData?.datasets[1] || { data: [] },
        borderColor: "rgb(255, 99, 132)",
        tension: 0.1,
      },
    ],
  }), [vendorUsageData]);

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
          <Typography variant="h6" gutterBottom>
            <Box
              component="span"
              onClick={() => navigate("/admin/model-prices")}
              sx={{
                cursor: 'pointer',
                textDecoration: 'none',
                color: 'inherit',
                '&:hover': {
                  textDecoration: 'underline'
                }
              }}
            >
              Cost per Model
            </Box>
          </Typography>
          {vendorModelCostData.length > 0 ? (
            <>
              <TableContainer>
                <Table>
                  <TableHead>
                    <TableRow>
                      <TableCell>Model</TableCell>
                      <TableCell align="right">Total Cost</TableCell>
                      <TableCell align="right">Request Tokens</TableCell>
                      <TableCell align="right">Response Tokens</TableCell>
                    </TableRow>
                  </TableHead>
                  <TableBody>
                    {vendorModelCostData
                      .slice(0, isTableExpanded ? undefined : 5)
                      .map((row, index) => (
                        <TableRow key={index}>
                          <TableCell>
                            {row.modelPriceId ? (
                              <Box
                                component="span"
                                onClick={() => navigate(`/admin/model-prices/${row.modelPriceId}`)}
                                sx={{
                                  cursor: 'pointer',
                                  textDecoration: 'none',
                                  color: 'inherit',
                                  '&:hover': {
                                    textDecoration: 'underline'
                                  }
                                }}
                              >
                                {row.model}
                              </Box>
                            ) : (
                              row.model
                            )}
                          </TableCell>
                          <TableCell align="right">
                            <div style={{ marginBottom: '4px' }}>${row.totalCost.toFixed(2)}</div>
                            <div style={{ fontSize: '0.85em', color: 'gray' }}>
                              (Prompt: {row.promptCost.toFixed(2)}, CW: {row.cacheWriteCost.toFixed(2)}, CR: {row.cacheReadCost.toFixed(2)}, Resp: {row.responseCost.toFixed(2)})
                            </div>
                          </TableCell>
                          <TableCell align="right">
                            <div style={{ marginBottom: '4px' }}>{(row.promptTokens + row.cacheWriteTokens + row.cacheReadTokens).toLocaleString()}</div>
                            <div style={{ fontSize: '0.85em', color: 'gray' }}>
                              (Prompt: {row.promptTokens.toLocaleString()}, CW: {row.cacheWriteTokens.toLocaleString()}, CR: {row.cacheReadTokens.toLocaleString()})
                            </div>
                          </TableCell>
                          <TableCell align="right">
                            {row.responseTokens.toLocaleString()}
                          </TableCell>
                        </TableRow>
                      ))}
                  </TableBody>
                </Table>
              </TableContainer>
              {vendorModelCostData.length > 5 && (
                <Box mt={2} textAlign="center">
                  <Button onClick={toggleTableExpansion}>
                    {isTableExpanded ? "Collapse" : "Expand"}
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
                No vendor and model cost data available for the selected period.
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
          (for example Anthropic and OopenAI). If you have an LLM provider that
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
              <FieldValue>No model patterns specified</FieldValue>
            )}
            <Typography
              variant="caption"
              color="text.secondary"
              sx={{ display: "block", mt: 1 }}
            >
              These patterns use regex matching to determine which models are
              allowed. For example, "gpt-4.*" allows all GPT-4 models.
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

        <Divider sx={{ my: 3 }} />


        <SectionTitle>Proxy Logs</SectionTitle>
        <StyledPaper>
          <Table>
            <TableHead>
              <TableRow>
                <StyledTableHeaderCell sx={{ verticalAlign: "top" }}>
                  Timestamp
                </StyledTableHeaderCell>
                <StyledTableHeaderCell sx={{ verticalAlign: "top" }}>
                  Vendor
                </StyledTableHeaderCell>
                <StyledTableHeaderCell sx={{ verticalAlign: "top" }}>
                  Response Code
                </StyledTableHeaderCell>
                <StyledTableHeaderCell sx={{ verticalAlign: "top" }}>
                  Request
                </StyledTableHeaderCell>
                <StyledTableHeaderCell sx={{ verticalAlign: "top" }}>
                  Response
                </StyledTableHeaderCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {proxyLogs?.map((log) => (
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
                  <StyledTableCell sx={{ verticalAlign: "top" }}>
                    <pre>
                      <code>
                        <ExpandableMessage
                          message={log.attributes.request_body}
                        />
                      </code>
                    </pre>
                  </StyledTableCell>
                  <StyledTableCell sx={{ verticalAlign: "top" }}>
                    <pre>
                      <code>
                        <ExpandableMessage
                          message={log.attributes.response_body}
                        />
                      </code>
                    </pre>
                  </StyledTableCell>
                </StyledTableRow>
              ))}
            </TableBody>
          </Table>
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
    </>
  );
};

export default LLMDetails;
