// API calls and mappers for MCP analytics: the calls Tyk Gateways proxy to
// MCP servers, sent to AI Studio by Tyk Pump. The views (server Usage tab,
// App sections, Dashboard panel) render only what these mappers return, so
// the response shapes are handled in one place.

export const MCP_ANALYTICS_BASE = "/analytics/mcp";

export const defaultRange = (days = 30) => ({
  startDate: new Date(Date.now() - days * 24 * 60 * 60 * 1000).toISOString().split("T")[0],
  endDate: new Date().toISOString().split("T")[0],
});

// Hourly buckets for short ranges, daily otherwise, like the LLM usage views.
export const intervalFor = (startDate, endDate) => {
  const days = (new Date(endDate) - new Date(startDate)) / (24 * 60 * 60 * 1000);
  return days <= 2 ? "hour" : "day";
};

const rangeParams = ({ startDate, endDate }) => ({ start_date: startDate, end_date: endDate });

const num = (v) => (typeof v === "number" && Number.isFinite(v) ? v : 0);

// errorRate prefers the server's rate and falls back to errors / calls.
const errorRate = (row) => {
  if (typeof row?.error_rate === "number") return row.error_rate;
  const calls = num(row?.calls);
  return calls ? num(row?.errors) / calls : 0;
};

export const mapSummary = (s) => ({
  calls: num(s?.calls),
  errors: num(s?.errors),
  errorRate: errorRate(s),
  avgMs: num(s?.avg_ms),
  p50Ms: num(s?.p50_ms),
  p95Ms: num(s?.p95_ms),
  lastSeenAt: s?.last_seen_at || null,
});

export const mapTimeseries = (ts) =>
  (ts?.points || []).map((p) => ({
    bucket: p.bucket,
    calls: num(p.calls),
    errors: num(p.errors),
    p50Ms: num(p.p50_ms),
    p95Ms: num(p.p95_ms),
  }));

export const mapToolRow = (r) => ({
  id: `${r.primitive_type || ""}:${r.primitive_name || ""}`,
  primitiveType: r.primitive_type || "",
  name: r.primitive_name || "(none)",
  calls: num(r.calls),
  errors: num(r.errors),
  errorRate: errorRate(r),
  avgMs: num(r.avg_ms),
  p95Ms: num(r.p95_ms),
  lastSeenAt: r.last_seen_at || null,
});

export const mapAppRow = (r) => ({
  id: r.app_id ?? "unattributed",
  appId: r.app_id ?? null,
  name: r.app_id ? r.app_name || `App ${r.app_id}` : "Unattributed",
  calls: num(r.calls),
  errors: num(r.errors),
  errorRate: errorRate(r),
  lastSeenAt: r.last_seen_at || null,
});

export const mapServerRow = (r) => ({
  id: r.mcp_server_id,
  serverId: r.mcp_server_id,
  name: r.server_name || `MCP server ${r.mcp_server_id}`,
  connectionId: r.connection_id ?? null,
  calls: num(r.calls),
  errors: num(r.errors),
  errorRate: errorRate(r),
  p95Ms: num(r.p95_ms),
  lastSeenAt: r.last_seen_at || null,
});

export const mapOverview = (o) => ({
  ...mapSummary(o),
  topTools: (o?.top_tools || []).map((t) => ({
    id: `${t.mcp_server_id}:${t.primitive_name}`,
    serverId: t.mcp_server_id,
    serverName: t.server_name || `MCP server ${t.mcp_server_id}`,
    name: t.primitive_name || "(none)",
    calls: num(t.calls),
    errors: num(t.errors),
    errorRate: errorRate(t),
  })),
});

const pagedRows = (body, mapRow) => {
  const rows = (body?.data || []).map(mapRow);
  return {
    rows,
    total: num(body?.total ?? body?.meta?.total_count ?? rows.length),
    totalPages: num(body?.total_pages ?? body?.meta?.total_pages) || 1,
  };
};

export const fetchServerSummary = async (client, serverId, range) =>
  mapSummary((await client.get(`${MCP_ANALYTICS_BASE}/servers/${serverId}/summary`, { params: rangeParams(range) })).data);

export const fetchServerTimeseries = async (client, serverId, range) =>
  mapTimeseries(
    (
      await client.get(`${MCP_ANALYTICS_BASE}/servers/${serverId}/timeseries`, {
        params: { ...rangeParams(range), interval: intervalFor(range.startDate, range.endDate) },
      })
    ).data,
  );

// query is useListQuery's queryParams: page, page_size, search, sort.
export const fetchServerTools = async (client, serverId, range, query = { page: 1, page_size: 10 }) =>
  pagedRows(
    (await client.get(`${MCP_ANALYTICS_BASE}/servers/${serverId}/tools`, { params: { ...rangeParams(range), ...query } })).data,
    mapToolRow,
  );

// query is useListQuery's queryParams: page, page_size, search, sort.
export const fetchServerApps = async (client, serverId, range, query = { page: 1, page_size: 10 }) =>
  pagedRows(
    (await client.get(`${MCP_ANALYTICS_BASE}/servers/${serverId}/apps`, { params: { ...rangeParams(range), ...query } })).data,
    mapAppRow,
  );

export const fetchOverview = async (client, range) =>
  mapOverview((await client.get(`${MCP_ANALYTICS_BASE}/overview`, { params: rangeParams(range) })).data);

// App usage: the admin route, or a portal route the caller must own. The
// portal passes its own client and path so it never calls an admin endpoint.
export const fetchAppServers = async (client, path, range) =>
  ((await client.get(path, { params: rangeParams(range) })).data?.servers || []).map(mapServerRow);

export const adminAppPath = (appId) => `${MCP_ANALYTICS_BASE}/apps/${appId}`;
// The portal route answers only for Apps the caller owns.
export const portalAppPath = (appId) => `/common/apps/${appId}/analytics/mcp`;

// Chart.js data for calls and errors over time.
export const callsChartData = (points) => ({
  datasets: [
    {
      label: "Calls",
      data: points.map((p) => ({ x: p.bucket, y: p.calls })),
      borderColor: "rgb(53, 162, 235)",
      backgroundColor: "rgba(53, 162, 235, 0.5)",
      tension: 0.1,
    },
    {
      label: "Errors",
      data: points.map((p) => ({ x: p.bucket, y: p.errors })),
      borderColor: "rgb(255, 99, 132)",
      backgroundColor: "rgba(255, 99, 132, 0.5)",
      tension: 0.1,
    },
  ],
});

export const latencyChartData = (points) => ({
  datasets: [
    {
      label: "p50 (ms)",
      data: points.map((p) => ({ x: p.bucket, y: p.p50Ms })),
      borderColor: "rgb(75, 192, 192)",
      backgroundColor: "rgba(75, 192, 192, 0.5)",
      tension: 0.1,
    },
    {
      label: "p95 (ms)",
      data: points.map((p) => ({ x: p.bucket, y: p.p95Ms })),
      borderColor: "rgb(255, 159, 64)",
      backgroundColor: "rgba(255, 159, 64, 0.5)",
      tension: 0.1,
    },
  ],
});

export const timeChartOptions = (title, yTitle, unit = "day") => ({
  responsive: true,
  maintainAspectRatio: false,
  scales: {
    x: { type: "time", time: { unit }, title: { display: true, text: "Date" } },
    y: { beginAtZero: true, title: { display: true, text: yTitle } },
  },
  plugins: {
    legend: { position: "top" },
    title: { display: true, text: title },
    tooltip: { mode: "index", intersect: false },
  },
});

export const formatPercent = (rate) => `${(num(rate) * 100).toFixed(rate > 0 && rate < 0.001 ? 2 : 1)}%`;
export const formatMs = (ms) => (num(ms) >= 1000 ? `${(ms / 1000).toFixed(2)} s` : `${Math.round(num(ms))} ms`);
export const formatCount = (n) => num(n).toLocaleString();
