import React, { useState, useEffect, useMemo, useCallback } from "react";
import { useParams, useNavigate, useSearchParams } from "react-router-dom";
import apiClient from "../../utils/apiClient";
import {
  Typography,
  CircularProgress,
  Box,
  Link,
  Chip,
  Tooltip,
  Table,
  TableBody,
  TableContainer,
  TableHead,
  TableRow,
  TableSortLabel,
} from "@mui/material";
import ArrowBackIcon from "@mui/icons-material/ArrowBack";
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
import DateRangePicker from "../common/DateRangePicker";
import {
  StyledPaper,
  TitleBox,
  ContentBox,
  StyledTableCell,
  StyledTableHeaderCell,
  StyledTableRow,
  SecondaryLinkButton,
} from "../../styles/sharedStyles";
import { getVendorName } from "../../utils/vendorLogos";
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
} from "../../utils/modelUsage";

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

const APP_COLUMN_TYPES = {
  appName: "string",
  ownerEmail: "string",
  requestCount: "number",
  totalTokens: "number",
  totalCost: "number",
  firstUsed: "date",
  lastUsed: "date",
};

const SectionTitle = ({ children }) => (
  <Typography variant="h6" gutterBottom sx={{ mt: 3, mb: 2 }}>
    {children}
  </Typography>
);

// Names the provider entry this view is scoped to. The vendor is only added
// when it says something the entry name doesn't: "OpenAI Prod (OpenAI)" is
// useful, "Anthropic (Anthropic)" is not.
const providerLabel = (llm) => {
  const entryName = llm.attributes.name || "";
  const vendorName = getVendorName(llm.attributes.vendor) || "";
  const sameName = entryName.trim().toLowerCase() === vendorName.trim().toLowerCase();
  return sameName || !vendorName ? `Provider: ${entryName}` : `Provider: ${entryName} (${vendorName})`;
};

const defaultStart = () =>
  new Date(new Date().getTime() - 30 * 24 * 60 * 60 * 1000).toISOString().split("T")[0];
const defaultEnd = () => new Date().toISOString().split("T")[0];

/**
 * Per-model drill-down from the LLM provider details page: usage of one model
 * over time, and every app that called it in the period with the time each app
 * last did so. Answers "who is using model X?" so the owner can be contacted.
 */
const LLMModelDetails = () => {
  const { id } = useParams();
  const [searchParams] = useSearchParams();
  const model = searchParams.get("model") || "";
  const navigate = useNavigate();

  const [llm, setLLM] = useState(null);
  const [loading, setLoading] = useState(true);
  const [usageData, setUsageData] = useState(null);
  const [apps, setApps] = useState([]);
  const [appsLoading, setAppsLoading] = useState(true);
  const [appsError, setAppsError] = useState("");
  const [sort, setSort] = useState({ field: "lastUsed", direction: "desc" });
  const [startDate, setStartDate] = useState(defaultStart);
  const [endDate, setEndDate] = useState(defaultEnd);

  useEffect(() => {
    let cancelled = false;
    apiClient
      .get(`/llms/${id}`)
      .then((response) => {
        if (!cancelled) setLLM(response.data.data);
      })
      .catch((error) => {
        console.error("Error fetching LLM details", error);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [id]);

  const fetchUsage = useCallback(async () => {
    if (!model) return;
    const params = { start_date: startDate, end_date: endDate, llm_id: id, model_name: model };
    try {
      const response = await apiClient.get("/analytics/usage", { params });
      setUsageData(response.data);
    } catch (error) {
      console.error("Error fetching model usage", error);
    }
  }, [id, model, startDate, endDate]);

  const fetchApps = useCallback(async () => {
    if (!model) return;
    const params = { start_date: startDate, end_date: endDate, llm_id: id, model_name: model };
    setAppsLoading(true);
    setAppsError("");
    try {
      const response = await apiClient.get("/analytics/apps-for-model", { params });
      setApps(Array.isArray(response.data) ? response.data : []);
    } catch (error) {
      console.error("Error fetching apps for model", error);
      setAppsError("Could not load the apps for this model. Try again or narrow the date range.");
    } finally {
      setAppsLoading(false);
    }
  }, [id, model, startDate, endDate]);

  useEffect(() => {
    fetchUsage();
    fetchApps();
  }, [fetchUsage, fetchApps]);

  const tokenChartData = useMemo(() => buildTokenChartData(usageData), [usageData]);
  const costChartData = useMemo(() => buildCostChartData(usageData), [usageData]);
  const sortedApps = useMemo(() => sortUsageRows(apps, sort, APP_COLUMN_TYPES), [apps, sort]);

  const totals = useMemo(
    () =>
      apps.reduce(
        (acc, row) => ({
          requests: acc.requests + (Number(row.requestCount) || 0),
          cost: acc.cost + (Number(row.totalCost) || 0),
        }),
        { requests: 0, cost: 0 },
      ),
    [apps],
  );

  const handleSort = (field) => {
    setSort((current) => nextSortConfig(current, field, APP_COLUMN_TYPES));
  };

  const sortHeader = (field, label, align = "left") => (
    <StyledTableHeaderCell align={align} sortDirection={sort.field === field ? sort.direction : false}>
      <TableSortLabel
        active={sort.field === field}
        direction={sort.field === field ? sort.direction : "desc"}
        onClick={() => handleSort(field)}
      >
        {label}
      </TableSortLabel>
    </StyledTableHeaderCell>
  );

  const backToProvider = () => navigate(`/admin/llms/${id}`);

  if (loading) return <CircularProgress />;
  if (!llm) return <Typography>LLM not found</Typography>;
  if (!model) {
    return (
      <>
        <TitleBox top="64px">
          <Typography variant="headingXLarge">Model usage</Typography>
          <SecondaryLinkButton startIcon={<ArrowBackIcon />} onClick={backToProvider} color="inherit">
            Back to provider
          </SecondaryLinkButton>
        </TitleBox>
        <ContentBox>
          <Typography>No model was given. Pick a model from the "Models in use" table on the provider page.</Typography>
        </ContentBox>
      </>
    );
  }

  return (
    <>
      <TitleBox top="64px">
        <Box>
          <Typography variant="headingXLarge" component="h1" sx={{ fontFamily: "monospace" }}>
            {model}
          </Typography>
          <Typography variant="body2" color="text.secondary">
            {providerLabel(llm)}
          </Typography>
        </Box>
        <SecondaryLinkButton startIcon={<ArrowBackIcon />} onClick={backToProvider} color="inherit">
          Back to provider
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
            updateMode="immediate"
          />
        </Box>

        <StyledPaper elevation={3} style={{ padding: "20px", marginTop: "20px", marginBottom: "20px" }}>
          <Box display="flex" justifyContent="space-between" alignItems="flex-start" flexWrap="wrap" gap={1} mb={1}>
            <Box>
              <Typography variant="h6">Apps using this model</Typography>
              <Typography variant="body2" color="text.secondary">
                Every app that called {model} through this provider between {startDate} and {endDate}.
                "Last used" is the most recent call inside that range.
              </Typography>
            </Box>
            {apps.length > 0 && (
              <Box display="flex" gap={1} flexWrap="wrap">
                <Chip size="small" label={`${apps.length} ${apps.length === 1 ? "app" : "apps"}`} />
                <Chip size="small" label={`${formatTokens(totals.requests)} requests`} />
                <Chip size="small" label={formatCost(totals.cost)} />
              </Box>
            )}
          </Box>

          {appsLoading ? (
            <Box py={4} display="flex" justifyContent="center">
              <CircularProgress size={28} />
            </Box>
          ) : appsError ? (
            <Typography color="error" py={2}>
              {appsError}
            </Typography>
          ) : apps.length === 0 ? (
            <Box py={4} textAlign="center">
              <Typography variant="body1" color="text.secondary">
                No app called {model} through this provider in the selected period.
              </Typography>
            </Box>
          ) : (
            <TableContainer sx={{ overflowX: "auto" }}>
              <Table size="small" data-testid="apps-for-model-table">
                <TableHead>
                  <TableRow>
                    {sortHeader("appName", "App")}
                    {sortHeader("ownerEmail", "Owner")}
                    {sortHeader("requestCount", "Requests", "right")}
                    {sortHeader("totalTokens", "Tokens", "right")}
                    {sortHeader("totalCost", "Cost", "right")}
                    {sortHeader("firstUsed", "First used", "right")}
                    {sortHeader("lastUsed", "Last used", "right")}
                  </TableRow>
                </TableHead>
                <TableBody>
                  {sortedApps.map((row) => {
                    const first = formatUsageTime(row.firstUsed);
                    const last = formatUsageTime(row.lastUsed);
                    return (
                      <StyledTableRow key={row.appId}>
                        <StyledTableCell>
                          {row.appDeleted ? (
                            <Tooltip title="This app has been deleted; its usage is kept for the record.">
                              <span style={{ color: "gray" }}>{row.appName}</span>
                            </Tooltip>
                          ) : (
                            <Link
                              component="button"
                              variant="body2"
                              underline="hover"
                              sx={{ textAlign: "left" }}
                              onClick={() => navigate(`/admin/apps/${row.appId}`)}
                            >
                              {row.appName}
                            </Link>
                          )}
                          <div style={{ fontSize: "0.85em", color: "gray" }}>app {row.appId}</div>
                        </StyledTableCell>
                        <StyledTableCell>
                          {row.ownerEmail ? (
                            <Link href={`mailto:${row.ownerEmail}`} underline="hover" variant="body2">
                              {row.ownerEmail}
                            </Link>
                          ) : (
                            <span style={{ color: "gray" }}>Unknown</span>
                          )}
                        </StyledTableCell>
                        <StyledTableCell align="right">{formatTokens(row.requestCount)}</StyledTableCell>
                        <StyledTableCell align="right">{formatTokens(row.totalTokens)}</StyledTableCell>
                        <StyledTableCell align="right">{formatCost(row.totalCost)}</StyledTableCell>
                        <StyledTableCell align="right">
                          <Tooltip title={first.absolute} placement="top">
                            <span>{first.relative}</span>
                          </Tooltip>
                        </StyledTableCell>
                        <StyledTableCell align="right">
                          <Tooltip title={last.absolute} placement="top">
                            <span>{last.relative}</span>
                          </Tooltip>
                          <div style={{ fontSize: "0.85em", color: "gray" }}>{last.absolute}</div>
                        </StyledTableCell>
                      </StyledTableRow>
                    );
                  })}
                </TableBody>
              </Table>
            </TableContainer>
          )}
        </StyledPaper>
      </ContentBox>
    </>
  );
};

export default LLMModelDetails;
