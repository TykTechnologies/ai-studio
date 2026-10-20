import React, { useCallback, useEffect, useMemo, useState } from "react";
import { Alert, Box, CircularProgress, Grid, Link, Typography } from "@mui/material";
import { Link as RouterLink } from "react-router-dom";
import {
  Chart as ChartJS,
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
import apiClient from "../../utils/apiClient";
import useListQuery from "../../hooks/useListQuery";
import DataTable from "../common/DataTable";
import DateRangePicker from "../common/DateRangePicker";
import Section from "../common/Section";
import { formatTime, apiErrorDetail } from "../../pages/webhookShared";
import MCPAnalyticsEmptyState from "./MCPAnalyticsEmptyState";
import {
  defaultRange,
  intervalFor,
  fetchServerSummary,
  fetchServerTimeseries,
  fetchServerTools,
  fetchServerApps,
  callsChartData,
  latencyChartData,
  timeChartOptions,
  formatCount,
  formatMs,
  formatPercent,
} from "./mcpAnalytics";

ChartJS.register(LinearScale, PointElement, LineElement, Title, ChartTooltip, Legend, TimeScale);

export const StatTile = ({ label, value, testId }) => (
  <Grid item xs={6} md={2}>
    <Typography variant="body2" color="text.secondary">
      {label}
    </Typography>
    <Typography variant="bodyLargeBold" data-testid={testId}>
      {value}
    </Typography>
  </Grid>
);

const toolColumns = [
  { field: "name", headerName: "Name", renderCell: (r) => r.name },
  { field: "primitive_type", headerName: "Type", renderCell: (r) => r.primitiveType },
  { field: "calls", headerName: "Calls", align: "right", renderCell: (r) => formatCount(r.calls) },
  { field: "error_rate", headerName: "Error rate", align: "right", renderCell: (r) => formatPercent(r.errorRate) },
  { field: "avg_ms", headerName: "Avg latency", align: "right", renderCell: (r) => formatMs(r.avgMs) },
  { field: "p95_ms", headerName: "p95 latency", align: "right", renderCell: (r) => formatMs(r.p95Ms) },
  { field: "last_seen_at", headerName: "Last seen", renderCell: (r) => formatTime(r.lastSeenAt) },
];

const appColumns = [
  {
    field: "name",
    headerName: "App",
    renderCell: (r) =>
      r.appId ? (
        <Link component={RouterLink} to={`/admin/apps/${r.appId}`}>
          {r.name}
        </Link>
      ) : (
        r.name
      ),
  },
  { field: "calls", headerName: "Calls", align: "right", renderCell: (r) => formatCount(r.calls) },
  { field: "error_rate", headerName: "Error rate", align: "right", renderCell: (r) => formatPercent(r.errorRate) },
  { field: "last_seen_at", headerName: "Last seen", renderCell: (r) => formatTime(r.lastSeenAt) },
];

// usePagedRows loads one server-paged table and keeps its pagination in step.
const usePagedRows = (fetcher, serverId, range, list) => {
  const [rows, setRows] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const { queryParams, updatePaginationData } = list;

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    fetcher(apiClient, serverId, range, queryParams)
      .then((res) => {
        if (cancelled) return;
        setRows(res.rows);
        updatePaginationData(res.total, res.totalPages);
        setError(null);
      })
      .catch((err) => !cancelled && setError(apiErrorDetail(err, "Failed to load")))
      .finally(() => !cancelled && setLoading(false));
    return () => {
      cancelled = true;
    };
  }, [fetcher, serverId, range, queryParams, updatePaginationData]);

  return { rows, loading, error };
};

/**
 * MCPServerUsage is the Usage tab of an MCP server: calls and errors over
 * time, latency percentiles, a per-tool table and the Apps calling it, from
 * the analytics Tyk Pump sends to AI Studio.
 */
const MCPServerUsage = ({ serverId, connectionId }) => {
  const initial = useMemo(() => defaultRange(), []);
  const [startDate, setStartDate] = useState(initial.startDate);
  const [endDate, setEndDate] = useState(initial.endDate);
  const range = useMemo(() => ({ startDate, endDate }), [startDate, endDate]);

  const [summary, setSummary] = useState(null);
  const [points, setPoints] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);

  // Rows come sorted by calls, busiest first; the API has no other order.
  const toolsList = useListQuery();
  const appsList = useListQuery();
  const tools = usePagedRows(fetchServerTools, serverId, range, toolsList);
  const apps = usePagedRows(fetchServerApps, serverId, range, appsList);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const [s, ts] = await Promise.all([
        fetchServerSummary(apiClient, serverId, range),
        fetchServerTimeseries(apiClient, serverId, range),
      ]);
      setSummary(s);
      setPoints(ts);
    } catch (err) {
      setError(apiErrorDetail(err, "Failed to load MCP usage"));
    } finally {
      setLoading(false);
    }
  }, [serverId, range]);

  useEffect(() => {
    load();
  }, [load]);

  const unit = intervalFor(startDate, endDate);
  const empty = summary && summary.calls === 0;

  return (
    <Box data-testid="mcp-server-usage">
      <Box sx={{ display: "flex", justifyContent: "flex-end", mb: 2 }}>
        <DateRangePicker
          startDate={startDate}
          endDate={endDate}
          onStartDateChange={setStartDate}
          onEndDateChange={setEndDate}
          updateMode="immediate"
        />
      </Box>

      {error && (
        <Alert severity="error" sx={{ mb: 2 }} data-testid="mcp-usage-error">
          {error}
        </Alert>
      )}

      {loading && !summary ? (
        <Box sx={{ display: "flex", justifyContent: "center", p: 3 }}>
          <CircularProgress />
        </Box>
      ) : empty ? (
        <MCPAnalyticsEmptyState connectionId={connectionId} />
      ) : summary ? (
        <>
          <Section title="Overview">
            <Grid container spacing={2}>
              <StatTile label="Calls" value={formatCount(summary.calls)} testId="mcp-usage-calls" />
              <StatTile label="Errors" value={formatCount(summary.errors)} testId="mcp-usage-errors" />
              <StatTile label="Error rate" value={formatPercent(summary.errorRate)} testId="mcp-usage-error-rate" />
              <StatTile label="p50 latency" value={formatMs(summary.p50Ms)} testId="mcp-usage-p50" />
              <StatTile label="p95 latency" value={formatMs(summary.p95Ms)} testId="mcp-usage-p95" />
              <StatTile label="Last call" value={summary.lastSeenAt ? formatTime(summary.lastSeenAt) : "Never"} testId="mcp-usage-last-seen" />
            </Grid>
            <Grid container spacing={2} sx={{ mt: 1 }}>
              <Grid item xs={12} md={6}>
                <Box sx={{ height: 300 }}>
                  <Line options={timeChartOptions("Calls and errors", "Calls", unit)} data={callsChartData(points)} />
                </Box>
              </Grid>
              <Grid item xs={12} md={6}>
                <Box sx={{ height: 300 }}>
                  <Line options={timeChartOptions("Latency", "Milliseconds", unit)} data={latencyChartData(points)} />
                </Box>
              </Grid>
            </Grid>
          </Section>

          <Section title="Tools, resources and prompts" description="Errors are HTTP responses of 400 or above and JSON-RPC errors the Gateway detects. An error an upstream MCP server returns inside an HTTP 200 response may not be counted.">
            {tools.error && <Alert severity="error">{tools.error}</Alert>}
            <DataTable
              columns={toolColumns}
              data={tools.rows}
              loading={tools.loading}
              {...toolsList.tableProps}
              searchPlaceholder="Search by name"
              emptyMessage="No calls in this period"
              ariaLabel="MCP tool usage"
            />
          </Section>

          <Section
            title="Top Apps"
            description="Calls are attributed to an App by the Tyk key AI Studio minted for it. OAuth, JWT and keyless servers show as unattributed."
          >
            {apps.error && <Alert severity="error">{apps.error}</Alert>}
            <DataTable
              columns={appColumns}
              data={apps.rows}
              loading={apps.loading}
              pagination={appsList.tableProps.pagination}
              emptyMessage="No calls in this period"
              ariaLabel="Apps calling this MCP server"
            />
          </Section>
        </>
      ) : null}
    </Box>
  );
};

export default MCPServerUsage;
