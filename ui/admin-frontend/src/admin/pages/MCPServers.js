import React, { useCallback, useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import {
  Alert,
  Box,
  Button,
  Chip,
  CircularProgress,
  FormControl,
  InputLabel,
  MenuItem,
  Select,
  Table,
  TableBody,
  TableContainer,
  TableHead,
  TableRow,
  TextField,
  Tooltip,
  Typography,
} from "@mui/material";
import HubIcon from "@mui/icons-material/Hub";
import RefreshIcon from "@mui/icons-material/Refresh";
import SyncIcon from "@mui/icons-material/Sync";
import apiClient from "../utils/apiClient";
import Can from "../components/rbac/Can";
import { P } from "../rbac/permissions";
import {
  TitleBox,
  StyledPaper,
  StyledTableHeaderCell,
  StyledTableCell,
  StyledTableRow,
} from "../styles/sharedStyles";
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

const MCPServers = () => {
  const navigate = useNavigate();
  const [status, setStatus] = useState(null);
  const [connections, setConnections] = useState([]);
  const [servers, setServers] = useState([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [notice, setNotice] = useState(null);
  const [filters, setFilters] = useState({ connection_id: "", state: "", published: "", q: "" });
  const [page, setPage] = useState(1);
  const pageSize = 25;

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const st = await apiClient.get("/tyk-mcp/status");
      setStatus(st.data);
      if (!(st.data?.available && st.data?.enabled)) return;
      const params = { page, page_size: pageSize };
      Object.entries(filters).forEach(([k, v]) => {
        if (v !== "") params[k] = v;
      });
      const [conns, list] = await Promise.all([apiClient.get("/tyk-connections"), apiClient.get("/mcp-servers", { params })]);
      setConnections(conns.data || []);
      setServers(list.data?.servers || []);
      setTotal(list.data?.total || 0);
    } catch (err) {
      setError(apiErrorDetail(err, "Failed to load MCP servers"));
    } finally {
      setLoading(false);
    }
  }, [filters, page]);

  useEffect(() => {
    load();
  }, [load]);

  const syncNow = async (connectionId) => {
    setError(null);
    try {
      const res = await apiClient.post(`/tyk-connections/${connectionId}/sync?wait=true`, {});
      const run = res.data;
      setNotice(`Sync ${run.status}: ${run.proxies_added} added, ${run.proxies_updated} updated, ${run.proxies_missing} missing`);
      await load();
    } catch (err) {
      setError(apiErrorDetail(err, "Sync failed"));
    }
  };

  const setFilter = (k) => (e) => {
    setPage(1);
    setFilters({ ...filters, [k]: e.target.value });
  };

  if (loading && !status) {
    return (
      <Box sx={{ p: 3, display: "flex", justifyContent: "center" }}>
        <CircularProgress />
      </Box>
    );
  }
  if (status && !status.available) return <TykUpsell />;
  if (status && !status.enabled) return <TykDisabledNotice status={status} />;

  const activeConnections = connections.filter((c) => c.status === "active");
  const totalPages = Math.max(1, Math.ceil(total / pageSize));

  return (
    <Box>
      <TitleBox>
        <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
          <HubIcon />
          <Typography variant="h5">MCP servers</Typography>
          <Typography variant="body2" color="text.secondary">
            {total} total
          </Typography>
        </Box>
        <Box sx={{ display: "flex", gap: 1 }}>
          <Button startIcon={<RefreshIcon />} onClick={load} disabled={loading}>
            Refresh
          </Button>
          <Can permission={P.TYK_CONNECTIONS_EXECUTE}>
            {activeConnections.length > 0 && (
              <Button
                startIcon={<SyncIcon />}
                variant="outlined"
                onClick={() => syncNow(filters.connection_id || activeConnections[0].id)}
                data-testid="sync-now"
              >
                Sync now
              </Button>
            )}
          </Can>
        </Box>
      </TitleBox>

      {error && (
        <Alert severity="error" sx={{ mb: 2 }} onClose={() => setError(null)} data-testid="page-error">
          {error}
        </Alert>
      )}
      {notice && (
        <Alert severity="success" sx={{ mb: 2 }} onClose={() => setNotice(null)}>
          {notice}
        </Alert>
      )}
      {connections.length === 0 && (
        <Alert severity="info" sx={{ mb: 2 }}>
          No Tyk Dashboard is connected. Connect one under Settings → Tyk Dashboard to import its MCP proxies.
        </Alert>
      )}

      <Box sx={{ display: "flex", gap: 1, mb: 2, flexWrap: "wrap" }}>
        <TextField size="small" label="Search" value={filters.q} onChange={setFilter("q")} inputProps={{ "data-testid": "search" }} />
        <FormControl size="small" sx={{ minWidth: 180 }}>
          <InputLabel>Connection</InputLabel>
          <Select label="Connection" value={filters.connection_id} onChange={setFilter("connection_id")}>
            <MenuItem value="">All</MenuItem>
            {connections.map((c) => (
              <MenuItem key={c.id} value={c.id}>
                {c.name}
              </MenuItem>
            ))}
          </Select>
        </FormControl>
        <FormControl size="small" sx={{ minWidth: 180 }}>
          <InputLabel>Dashboard state</InputLabel>
          <Select label="Dashboard state" value={filters.state} onChange={setFilter("state")}>
            <MenuItem value="">All</MenuItem>
            {Object.entries(STATE_LABELS).map(([k, v]) => (
              <MenuItem key={k} value={k}>
                {v}
              </MenuItem>
            ))}
          </Select>
        </FormControl>
        <FormControl size="small" sx={{ minWidth: 140 }}>
          <InputLabel>Published</InputLabel>
          <Select label="Published" value={filters.published} onChange={setFilter("published")}>
            <MenuItem value="">All</MenuItem>
            <MenuItem value="true">Published</MenuItem>
            <MenuItem value="false">Unpublished</MenuItem>
          </Select>
        </FormControl>
      </Box>

      <StyledPaper>
        {servers.length === 0 ? (
          <Box sx={{ p: 3 }}>
            <Typography color="text.secondary">No MCP servers match. Run a sync to import proxies from the Dashboard.</Typography>
          </Box>
        ) : (
          <TableContainer>
            <Table size="small">
              <TableHead>
                <TableRow>
                  <StyledTableHeaderCell>Server</StyledTableHeaderCell>
                  <StyledTableHeaderCell>Connection</StyledTableHeaderCell>
                  <StyledTableHeaderCell>Kind</StyledTableHeaderCell>
                  <StyledTableHeaderCell>Auth</StyledTableHeaderCell>
                  <StyledTableHeaderCell>Dashboard</StyledTableHeaderCell>
                  <StyledTableHeaderCell>Portal</StyledTableHeaderCell>
                  <StyledTableHeaderCell>Tags</StyledTableHeaderCell>
                  <StyledTableHeaderCell>Last seen</StyledTableHeaderCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {servers.map((s) => (
                  <StyledTableRow
                    key={s.id}
                    hover
                    sx={{ cursor: "pointer" }}
                    onClick={() => navigate(`/admin/mcp-servers/${s.id}`)}
                    data-testid={`server-row-${s.id}`}
                  >
                    <StyledTableCell>
                      <Typography variant="body2" fontWeight={600}>
                        {s.name}
                      </Typography>
                      <Typography variant="caption" color="text.secondary">
                        {s.listen_path} · {s.tyk_api_id || "no api id"}
                      </Typography>
                    </StyledTableCell>
                    <StyledTableCell>{s.connection_name || s.connection_id}</StyledTableCell>
                    <StyledTableCell>
                      <KindChip kind={s.kind} />
                    </StyledTableCell>
                    <StyledTableCell>{AUTH_MODE_LABELS[s.auth_mode] || s.auth_mode}</StyledTableCell>
                    <StyledTableCell>
                      <DashboardStateChip state={s.dashboard_state} />
                    </StyledTableCell>
                    <StyledTableCell>
                      <Box sx={{ display: "flex", gap: 0.5 }}>
                        <Chip size="small" label={s.is_active ? "Published" : "Unpublished"} color={s.is_active ? "success" : "default"} />
                        {s.brokerable && (
                          <Tooltip title="Keys can be minted for Apps">
                            <Chip size="small" label="Brokerable" color="primary" variant="outlined" />
                          </Tooltip>
                        )}
                      </Box>
                    </StyledTableCell>
                    <StyledTableCell>
                      {(s.gateway_tags?.tags || []).length > 0 ? (
                        s.gateway_tags.tags.map((t) => <Chip key={t} size="small" label={t} sx={{ mr: 0.5 }} />)
                      ) : (
                        <Typography variant="caption" color="text.secondary">
                          all gateways
                        </Typography>
                      )}
                    </StyledTableCell>
                    <StyledTableCell>{s.last_seen_at ? formatTime(s.last_seen_at) : "never"}</StyledTableCell>
                  </StyledTableRow>
                ))}
              </TableBody>
            </Table>
          </TableContainer>
        )}
      </StyledPaper>
      {totalPages > 1 && (
        <Box sx={{ display: "flex", justifyContent: "flex-end", gap: 1, mt: 1, alignItems: "center" }}>
          <Button size="small" disabled={page <= 1} onClick={() => setPage(page - 1)}>
            Previous
          </Button>
          <Typography variant="body2">
            Page {page} of {totalPages}
          </Typography>
          <Button size="small" disabled={page >= totalPages} onClick={() => setPage(page + 1)}>
            Next
          </Button>
        </Box>
      )}
    </Box>
  );
};

export default MCPServers;
