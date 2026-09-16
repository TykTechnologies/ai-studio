import React, { useCallback, useEffect, useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";
import { Alert, Box, Chip, CircularProgress, MenuItem, Stack, TextField, Tooltip, Typography } from "@mui/material";
import AddIcon from "@mui/icons-material/Add";
import SyncIcon from "@mui/icons-material/Sync";
import apiClient from "../utils/apiClient";
import Can from "../components/rbac/Can";
import { P } from "../rbac/permissions";
import DataTable from "../components/common/DataTable";
import ActiveStatusDot from "../components/common/ActiveStatusDot";
import EmptyStateWidget from "../components/common/EmptyStateWidget";
import FeedbackSnackbar, { useFeedbackSnackbar } from "../components/common/FeedbackSnackbar";
import useListQuery from "../hooks/useListQuery";
import useConfig from "../hooks/useConfig";
import { TitleBox, ContentBox, PrimaryButton, PrimaryOutlineButton } from "../styles/sharedStyles";
import { apiErrorDetail, formatTime } from "./webhookShared";
import { TykUpsell, TykDisabledNotice } from "./tykShared";

export const AUTH_MODE_LABELS = {
  keyless: "Keyless",
  auth_token: "API key",
  basic: "Basic auth",
  jwt: "JWT",
  oauth_tyk: "OAuth (Tyk)",
  oauth_external: "External OAuth",
  oauth21: "OAuth 2.1",
  mtls: "mTLS",
  hmac: "HMAC",
  custom: "Custom",
  mixed: "Mixed",
};

export const STATE_LABELS = {
  active: "Active",
  inactive: "Inactive on Dashboard",
  missing: "Missing on Dashboard",
  pending_platform: "Awaiting platform team",
};

export const stateColor = (state) => {
  switch (state) {
    case "active":
      return "success";
    case "inactive":
      return "warning";
    case "missing":
      return "error";
    case "pending_platform":
      return "info";
    default:
      return "default";
  }
};

export const DashboardStateChip = ({ state }) => (
  <Chip size="small" label={STATE_LABELS[state] || state} color={stateColor(state)} data-testid={`state-${state}`} />
);

export const KindChip = ({ kind }) => (
  <Chip size="small" variant="outlined" label={kind === "rest_to_mcp" ? "REST API to MCP" : "Remote MCP"} />
);

const INTRO =
  "MCP servers are Tyk Gateway proxies imported from a connected Tyk Dashboard or registered from here. Publish a server to a tool catalog to make it visible in the portal; pin a policy bundle so AI Studio can mint keys for Apps that use it.";

const EMPTY_FILTERS = { connection_id: "", state: "", published: "" };

const MCPServers = () => {
  const navigate = useNavigate();
  const { getDocsLink } = useConfig();
  const { notify, snackbarProps } = useFeedbackSnackbar();
  const [status, setStatus] = useState(null);
  const [connections, setConnections] = useState([]);
  const [servers, setServers] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [filters, setFilters] = useState(EMPTY_FILTERS);

  const { queryParams, updatePaginationData, searchTerm, tableProps, pageSize, handlePageChange } = useListQuery({
    initialPageSize: 25,
  });

  const fetchServers = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const st = await apiClient.get("/tyk-mcp/status");
      setStatus(st.data);
      if (!(st.data?.available && st.data?.enabled)) return;
      // The list endpoint takes `q` for search and reports totals in the body.
      const { search, ...rest } = queryParams;
      const params = { ...rest };
      if (search) params.q = search;
      Object.entries(filters).forEach(([k, v]) => {
        if (v !== "") params[k] = v;
      });
      const [conns, list] = await Promise.all([
        apiClient.get("/tyk-connections"),
        apiClient.get("/mcp-servers", { params }),
      ]);
      setConnections(conns.data || []);
      setServers(list.data?.servers || []);
      const total = list.data?.total || 0;
      updatePaginationData(total, Math.max(1, Math.ceil(total / pageSize)));
    } catch (err) {
      setError(apiErrorDetail(err, "Failed to load MCP servers"));
    } finally {
      setLoading(false);
    }
  }, [queryParams, filters, pageSize, updatePaginationData]);

  useEffect(() => {
    fetchServers();
  }, [fetchServers]);

  const setFilter = (key) => (e) => {
    handlePageChange(1);
    setFilters((f) => ({ ...f, [key]: e.target.value }));
  };

  const activeConnections = useMemo(() => connections.filter((c) => c.status === "active"), [connections]);
  const canRegister = activeConnections.some((c) => c.effective_mode === "full");
  const filtersApplied = Object.values(filters).some((v) => v !== "");

  const syncNow = async () => {
    const connectionId = filters.connection_id || activeConnections[0]?.id;
    if (!connectionId) return;
    setError(null);
    try {
      const res = await apiClient.post(`/tyk-connections/${connectionId}/sync?wait=true`, {});
      const run = res.data;
      notify(`Sync ${run.status}: ${run.proxies_added} added, ${run.proxies_updated} updated, ${run.proxies_missing} missing`);
      await fetchServers();
    } catch (err) {
      notify(apiErrorDetail(err, "Sync failed"), "error");
    }
  };

  const columns = useMemo(
    () => [
      {
        field: "name",
        headerName: "Server",
        renderCell: (s) => (
          <Box>
            <Typography variant="body2" fontWeight={600}>
              {s.name}
            </Typography>
            <Typography variant="caption" color="text.secondary">
              {s.listen_path}
              {s.tyk_api_id ? ` · ${s.tyk_api_id}` : ""}
            </Typography>
          </Box>
        ),
      },
      { field: "connection_name", headerName: "Connection", renderCell: (s) => s.connection_name || s.connection_id },
      { field: "kind", headerName: "Kind", renderCell: (s) => <KindChip kind={s.kind} /> },
      { field: "auth_mode", headerName: "Auth", renderCell: (s) => AUTH_MODE_LABELS[s.auth_mode] || s.auth_mode },
      { field: "dashboard_state", headerName: "Dashboard", renderCell: (s) => <DashboardStateChip state={s.dashboard_state} /> },
      {
        field: "is_active",
        headerName: "Portal",
        renderCell: (s) => (
          <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
            <ActiveStatusDot active={Boolean(s.is_active)} activeLabel="Published" inactiveLabel="Unpublished" showLabel />
            {s.brokerable && (
              <Tooltip title="Keys can be minted for Apps">
                <Chip size="small" label="Brokerable" color="primary" variant="outlined" />
              </Tooltip>
            )}
          </Box>
        ),
      },
      {
        field: "gateway_tags",
        headerName: "Tags",
        renderCell: (s) =>
          (s.gateway_tags?.tags || []).length > 0 ? (
            <Box sx={{ display: "flex", gap: 0.5, flexWrap: "wrap" }}>
              {s.gateway_tags.tags.map((t) => (
                <Chip key={t} size="small" label={t} />
              ))}
            </Box>
          ) : (
            <Typography variant="caption" color="text.secondary">
              all gateways
            </Typography>
          ),
      },
      { field: "last_seen_at", headerName: "Last seen", renderCell: (s) => (s.last_seen_at ? formatTime(s.last_seen_at) : "never") },
    ],
    []
  );

  if (loading && !status) {
    return (
      <Box sx={{ p: 3, display: "flex", justifyContent: "center" }}>
        <CircularProgress />
      </Box>
    );
  }
  if (status && !status.available) return <TykUpsell />;
  if (status && !status.enabled) return <TykDisabledNotice status={status} />;

  const registerButton = (
    <PrimaryButton variant="contained" startIcon={<AddIcon />} onClick={() => navigate("/admin/mcp-servers/register")} data-testid="register-server">
      Register MCP server
    </PrimaryButton>
  );
  const syncButton = (
    <PrimaryOutlineButton variant="contained" startIcon={<SyncIcon />} onClick={syncNow} data-testid="sync-now">
      Sync now
    </PrimaryOutlineButton>
  );

  return (
    <>
      <TitleBox top="64px">
        <Typography variant="headingXLarge">MCP servers</Typography>
        <Stack direction="row" spacing={2}>
          <Can permission={P.TYK_CONNECTIONS_EXECUTE}>{activeConnections.length > 0 && syncButton}</Can>
          <Can permission={P.MCP_SERVERS_EXECUTE}>{canRegister && registerButton}</Can>
        </Stack>
      </TitleBox>
      <Box sx={{ p: 3 }}>
        <Typography variant="bodyLargeDefault" color="text.defaultSubdued">
          {INTRO}
        </Typography>
      </Box>
      <ContentBox>
        {error && (
          <Alert severity="error" sx={{ mb: 2 }} onClose={() => setError(null)} data-testid="page-error">
            {error}
          </Alert>
        )}
        {connections.length === 0 && (
          <Alert severity="info" sx={{ mb: 2 }}>
            No Tyk Dashboard is connected. Connect one under Settings → Tyk Connections to import its MCP proxies.
          </Alert>
        )}

        <Stack direction="row" spacing={2} sx={{ mb: 2, flexWrap: "wrap" }} useFlexGap>
          <TextField select size="small" label="Connection" value={filters.connection_id} onChange={setFilter("connection_id")} sx={{ minWidth: 200 }} inputProps={{ "data-testid": "filter-connection" }}>
            <MenuItem value="">All connections</MenuItem>
            {connections.map((c) => (
              <MenuItem key={c.id} value={String(c.id)}>
                {c.name}
              </MenuItem>
            ))}
          </TextField>
          <TextField select size="small" label="Dashboard state" value={filters.state} onChange={setFilter("state")} sx={{ minWidth: 200 }} inputProps={{ "data-testid": "filter-state" }}>
            <MenuItem value="">Any state</MenuItem>
            {Object.entries(STATE_LABELS).map(([k, v]) => (
              <MenuItem key={k} value={k}>
                {v}
              </MenuItem>
            ))}
          </TextField>
          <TextField select size="small" label="Portal" value={filters.published} onChange={setFilter("published")} sx={{ minWidth: 160 }} inputProps={{ "data-testid": "filter-published" }}>
            <MenuItem value="">Any</MenuItem>
            <MenuItem value="true">Published</MenuItem>
            <MenuItem value="false">Unpublished</MenuItem>
          </TextField>
        </Stack>

        <Can permission={P.MCP_SERVERS_EXECUTE}>
          {(canExecute) => (
            <DataTable
              {...tableProps}
              ariaLabel="MCP servers"
              searchPlaceholder="Search MCP servers by name..."
              columns={columns}
              data={servers}
              loading={loading}
              onRowClick={(s) => navigate(`/admin/mcp-servers/${s.id}`)}
              rowProps={(s) => ({ "data-testid": `server-row-${s.id}` })}
              emptyState={
                !searchTerm && !filtersApplied ? (
                  <EmptyStateWidget
                    title="No MCP servers yet"
                    description="MCP servers arrive from a connected Tyk Dashboard: run a sync to import the proxies it already serves, or register a new one from here on a full-mode connection. Community members can also propose servers through the portal."
                    learnMoreLink={getDocsLink("mcp_servers")}
                    actions={
                      <>
                        {activeConnections.length > 0 && syncButton}
                        {canExecute && canRegister && registerButton}
                      </>
                    }
                  />
                ) : undefined
              }
            />
          )}
        </Can>
      </ContentBox>
      <FeedbackSnackbar {...snackbarProps} />
    </>
  );
};

export default MCPServers;
