import React, { useCallback, useEffect, useState } from "react";
import { Link as RouterLink, useSearchParams } from "react-router-dom";
import {
  Alert,
  Box,
  Button,
  Checkbox,
  Chip,
  CircularProgress,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  FormControl,
  FormControlLabel,
  InputLabel,
  Link,
  MenuItem,
  Select,
  Tab,
  Table,
  TableBody,
  TableContainer,
  TableHead,
  TableRow,
  Tabs,
  TextField,
  Tooltip,
  Typography,
} from "@mui/material";
import KeyIcon from "@mui/icons-material/VpnKey";
import RefreshIcon from "@mui/icons-material/Refresh";
import apiClient from "../utils/apiClient";
import Can from "../components/rbac/Can";
import { P } from "../rbac/permissions";
import { TitleBox, StyledPaper, StyledTableHeaderCell, StyledTableCell, StyledTableRow } from "../styles/sharedStyles";
import { apiErrorDetail, formatTime } from "./webhookShared";
import { TykUpsell, TykDisabledNotice } from "./tykShared";
import { RevealKeyDialog } from "../../portal/components/AppMCPAccess";

export const CREDENTIAL_STATUSES = ["minting", "active", "suspended", "rotating_out", "revoked", "failed"];
export const DRIFT_STATES = ["none", "pending_widen", "applying", "error"];

const STATUS_LABELS = {
  minting: "Minting",
  active: "Active",
  suspended: "Suspended",
  rotating_out: "Rotating out",
  revoked: "Revoked",
  failed: "Failed",
};
const DRIFT_LABELS = {
  none: "In sync",
  pending_widen: "Widening pending",
  applying: "Applying",
  error: "Error",
};

const statusColor = (s) => ({ active: "success", suspended: "warning", rotating_out: "warning", revoked: "default", failed: "error" }[s] || "default");
const driftColor = (d) => ({ none: "default", pending_widen: "info", applying: "warning", error: "error" }[d] || "default");

export const CredentialStatusChip = ({ status }) => (
  <Chip size="small" label={STATUS_LABELS[status] || status} color={statusColor(status)} data-testid={`status-${status}`} />
);
export const DriftChip = ({ drift, detail }) => (
  <Tooltip title={detail || ""}>
    <Chip size="small" variant={drift === "none" ? "outlined" : "filled"} label={DRIFT_LABELS[drift] || drift} color={driftColor(drift)} data-testid={`drift-${drift}`} />
  </Tooltip>
);

const GRANT_LABELS = { key: "Tyk key", oauth: "OAuth", keyless: "Keyless", external: "External" };

const CredentialsTab = ({ connections, initial }) => {
  const [rows, setRows] = useState([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [filters, setFilters] = useState({ connection_id: "", status: "", drift: "", app_id: "", server_id: "", ...initial });
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [notice, setNotice] = useState(null);
  const [confirm, setConfirm] = useState(null); // {action, row}
  const [reason, setReason] = useState("");
  const [minted, setMinted] = useState(null);
  const pageSize = 25;

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const params = { page, page_size: pageSize };
      Object.entries(filters).forEach(([k, v]) => {
        if (v !== "") params[k] = v;
      });
      const res = await apiClient.get("/mcp-credentials", { params });
      setRows(res.data?.credentials || []);
      setTotal(res.data?.total || 0);
    } catch (err) {
      setError(apiErrorDetail(err, "Failed to load credentials"));
    } finally {
      setLoading(false);
    }
  }, [filters, page]);

  useEffect(() => {
    load();
  }, [load]);

  const setFilter = (k) => (e) => {
    setPage(1);
    setFilters({ ...filters, [k]: e.target.value });
  };

  const act = async (action, row, body) => {
    setError(null);
    setNotice(null);
    try {
      const res = await apiClient.post(`/mcp-credentials/${row.id}/${action}`, body || {});
      if (action === "rotate") {
        setMinted(res.data);
        setNotice(`Rotated the key for ${row.app_name || `app ${row.app_id}`}.`);
      } else {
        setNotice(`Credential ${row.id.slice(0, 8)}… ${action === "apply-drift" ? "updated" : action + "d"}.`);
      }
      await load();
    } catch (err) {
      setError(apiErrorDetail(err, `Failed to ${action} credential`));
    }
  };

  const totalPages = Math.max(1, Math.ceil(total / pageSize));
  return (
    <Box>
      {error && (
        <Alert severity="error" sx={{ mb: 2 }} onClose={() => setError(null)} data-testid="page-error">
          {error}
        </Alert>
      )}
      {notice && (
        <Alert severity="success" sx={{ mb: 2 }} onClose={() => setNotice(null)} data-testid="page-notice">
          {notice}
        </Alert>
      )}
      <Box sx={{ display: "flex", gap: 1, mb: 2, flexWrap: "wrap" }}>
        <FormControl size="small" sx={{ minWidth: 180 }}>
          <InputLabel>Connection</InputLabel>
          <Select label="Connection" value={filters.connection_id} onChange={setFilter("connection_id")} data-testid="filter-connection">
            <MenuItem value="">All</MenuItem>
            {connections.map((c) => (
              <MenuItem key={c.id} value={String(c.id)}>
                {c.name}
              </MenuItem>
            ))}
          </Select>
        </FormControl>
        <FormControl size="small" sx={{ minWidth: 160 }}>
          <InputLabel>Status</InputLabel>
          <Select label="Status" value={filters.status} onChange={setFilter("status")} data-testid="filter-status">
            <MenuItem value="">All</MenuItem>
            {CREDENTIAL_STATUSES.map((s) => (
              <MenuItem key={s} value={s}>
                {STATUS_LABELS[s]}
              </MenuItem>
            ))}
          </Select>
        </FormControl>
        <FormControl size="small" sx={{ minWidth: 160 }}>
          <InputLabel>Drift</InputLabel>
          <Select label="Drift" value={filters.drift} onChange={setFilter("drift")} data-testid="filter-drift">
            <MenuItem value="">All</MenuItem>
            {DRIFT_STATES.map((d) => (
              <MenuItem key={d} value={d}>
                {DRIFT_LABELS[d]}
              </MenuItem>
            ))}
          </Select>
        </FormControl>
        <TextField size="small" label="App ID" value={filters.app_id} onChange={setFilter("app_id")} inputProps={{ "data-testid": "filter-app" }} sx={{ width: 110 }} />
        <TextField size="small" label="Server ID" value={filters.server_id} onChange={setFilter("server_id")} inputProps={{ "data-testid": "filter-server" }} sx={{ width: 110 }} />
        <Button startIcon={<RefreshIcon />} onClick={load} disabled={loading}>
          Refresh
        </Button>
      </Box>

      <StyledPaper>
        <TableContainer>
          <Table size="small">
            <TableHead>
              <TableRow>
                <StyledTableHeaderCell>App</StyledTableHeaderCell>
                <StyledTableHeaderCell>Connection</StyledTableHeaderCell>
                <StyledTableHeaderCell>Key</StyledTableHeaderCell>
                <StyledTableHeaderCell>Status</StyledTableHeaderCell>
                <StyledTableHeaderCell>Policies</StyledTableHeaderCell>
                <StyledTableHeaderCell>Drift</StyledTableHeaderCell>
                <StyledTableHeaderCell>Minted</StyledTableHeaderCell>
                <StyledTableHeaderCell>Expires</StyledTableHeaderCell>
                <StyledTableHeaderCell>Actions</StyledTableHeaderCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {loading && rows.length === 0 ? (
                <TableRow>
                  <StyledTableCell colSpan={9} align="center">
                    <CircularProgress size={20} />
                  </StyledTableCell>
                </TableRow>
              ) : rows.length === 0 ? (
                <TableRow>
                  <StyledTableCell colSpan={9} align="center" data-testid="empty">
                    No credentials match. Keys are minted by App owners from the portal, or here with "Mint key".
                  </StyledTableCell>
                </TableRow>
              ) : (
                rows.map((row) => (
                  <StyledTableRow key={row.id} hover data-testid={`credential-${row.id}`}>
                    <StyledTableCell>
                      <Link component={RouterLink} to={`/admin/apps/${row.app_id}`}>
                        {row.app_name || `App ${row.app_id}`}
                      </Link>
                    </StyledTableCell>
                    <StyledTableCell>{row.connection_name || row.connection_id}</StyledTableCell>
                    <StyledTableCell>
                      <Tooltip title={row.tyk_key_hash || ""}>
                        <span style={{ fontFamily: "monospace" }}>{row.key_hint ? `…${row.key_hint}` : (row.tyk_key_hash || "").slice(0, 10)}</span>
                      </Tooltip>
                      {row.alias && (
                        <Typography variant="caption" display="block" color="text.secondary">
                          {row.alias}
                        </Typography>
                      )}
                    </StyledTableCell>
                    <StyledTableCell>
                      <CredentialStatusChip status={row.status} />
                      {row.revoke_reason && (
                        <Typography variant="caption" display="block" color="text.secondary">
                          {row.revoke_reason}
                          {row.revoke_mode === "inactive_only" && " (still present on the Dashboard)"}
                        </Typography>
                      )}
                      {row.last_error && (
                        <Typography variant="caption" display="block" color="error">
                          {row.last_error}
                        </Typography>
                      )}
                    </StyledTableCell>
                    <StyledTableCell>
                      <Box sx={{ display: "flex", gap: 0.5, flexWrap: "wrap" }}>
                        {(row.applied_policy_ids || []).map((id) => (
                          <Chip key={id} size="small" label={id} />
                        ))}
                        {(row.external_policy_ids || []).map((id) => (
                          <Tooltip key={id} title="Added on the Dashboard; not managed by AI Studio">
                            <Chip size="small" variant="outlined" color="warning" label={id} />
                          </Tooltip>
                        ))}
                      </Box>
                    </StyledTableCell>
                    <StyledTableCell>
                      <DriftChip drift={row.drift} detail={row.drift_detail} />
                    </StyledTableCell>
                    <StyledTableCell>{formatTime(row.minted_at)}</StyledTableCell>
                    <StyledTableCell>{row.expires_at ? formatTime(row.expires_at) : "Never"}</StyledTableCell>
                    <StyledTableCell>
                      <Can permission={P.MCP_CREDENTIALS_EXECUTE}>
                        <Box sx={{ display: "flex", gap: 0.5, flexWrap: "wrap" }}>
                          {row.drift === "pending_widen" && (
                            <Button size="small" variant="outlined" onClick={() => act("apply-drift", row)} data-testid={`apply-drift-${row.id}`}>
                              Apply change
                            </Button>
                          )}
                          {row.status === "active" && (
                            <Button size="small" onClick={() => setConfirm({ action: "suspend", row })} data-testid={`suspend-${row.id}`}>
                              Suspend
                            </Button>
                          )}
                          {row.status === "suspended" && (
                            <Button size="small" onClick={() => act("resume", row)} data-testid={`resume-${row.id}`}>
                              Resume
                            </Button>
                          )}
                          {["active", "suspended"].includes(row.status) && (
                            <>
                              <Button size="small" onClick={() => setConfirm({ action: "rotate", row })} data-testid={`rotate-${row.id}`}>
                                Rotate
                              </Button>
                              <Button size="small" color="error" onClick={() => setConfirm({ action: "revoke", row })} data-testid={`revoke-${row.id}`}>
                                Revoke
                              </Button>
                            </>
                          )}
                        </Box>
                      </Can>
                    </StyledTableCell>
                  </StyledTableRow>
                ))
              )}
            </TableBody>
          </Table>
        </TableContainer>
      </StyledPaper>
      <Box sx={{ display: "flex", justifyContent: "flex-end", alignItems: "center", gap: 1, mt: 1 }}>
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

      <Dialog
        open={!!confirm}
        onClose={() => {
          setConfirm(null);
          setReason("");
        }}
      >
        <DialogTitle>
          {confirm?.action === "rotate" && "Rotate this key?"}
          {confirm?.action === "suspend" && "Suspend this key?"}
          {confirm?.action === "revoke" && "Revoke this key?"}
        </DialogTitle>
        <DialogContent>
          <Typography variant="body2" sx={{ mb: 2 }}>
            {confirm?.action === "rotate" && "A new key is minted and shown to you once; the current key is revoked. The App owner must be given the new key."}
            {confirm?.action === "suspend" && "The key is switched off on the Dashboard until it is resumed."}
            {confirm?.action === "revoke" && "The key is deleted from the Dashboard. The App owner can mint a new one afterwards."}
          </Typography>
          {confirm?.action !== "rotate" && (
            <TextField fullWidth size="small" label="Reason (optional)" value={reason} onChange={(e) => setReason(e.target.value)} inputProps={{ "data-testid": "action-reason", maxLength: 255 }} />
          )}
        </DialogContent>
        <DialogActions>
          <Button
            onClick={() => {
              setConfirm(null);
              setReason("");
            }}
          >
            Cancel
          </Button>
          <Button
            variant="contained"
            color={confirm?.action === "revoke" ? "error" : "primary"}
            data-testid="confirm-action"
            onClick={() => {
              const c = confirm;
              const r = reason;
              setConfirm(null);
              setReason("");
              act(c.action, c.row, c.action === "rotate" ? {} : { reason: r });
            }}
          >
            Confirm
          </Button>
        </DialogActions>
      </Dialog>
      <RevealKeyDialog minted={minted} onClose={() => setMinted(null)} />
    </Box>
  );
};

const AccessReportTab = ({ connections, initial }) => {
  const [rows, setRows] = useState([]);
  const [filters, setFilters] = useState({ connection: "", server: "", user: "", app: "", ...initial });
  const [includeRevoked, setIncludeRevoked] = useState(false);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const params = {};
      Object.entries(filters).forEach(([k, v]) => {
        if (v !== "") params[k] = v;
      });
      if (includeRevoked) params.include_revoked = "true";
      const res = await apiClient.get("/mcp-access-report", { params });
      setRows(res.data || []);
    } catch (err) {
      setError(apiErrorDetail(err, "Failed to load the access report"));
    } finally {
      setLoading(false);
    }
  }, [filters, includeRevoked]);

  useEffect(() => {
    load();
  }, [load]);

  const setFilter = (k) => (e) => setFilters({ ...filters, [k]: e.target.value });

  return (
    <Box>
      {error && (
        <Alert severity="error" sx={{ mb: 2 }} onClose={() => setError(null)} data-testid="report-error">
          {error}
        </Alert>
      )}
      <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
        One row per App and MCP server the App reaches, with the person who owns the App and the key that backs the access. Servers that use OAuth,
        mTLS or no authentication are listed without a key.
      </Typography>
      <Box sx={{ display: "flex", gap: 1, mb: 2, flexWrap: "wrap", alignItems: "center" }}>
        <FormControl size="small" sx={{ minWidth: 180 }}>
          <InputLabel>Connection</InputLabel>
          <Select label="Connection" value={filters.connection} onChange={setFilter("connection")} data-testid="report-connection">
            <MenuItem value="">All</MenuItem>
            {connections.map((c) => (
              <MenuItem key={c.id} value={String(c.id)}>
                {c.name}
              </MenuItem>
            ))}
          </Select>
        </FormControl>
        <TextField size="small" label="Server ID" value={filters.server} onChange={setFilter("server")} inputProps={{ "data-testid": "report-server" }} sx={{ width: 110 }} />
        <TextField size="small" label="User ID" value={filters.user} onChange={setFilter("user")} inputProps={{ "data-testid": "report-user" }} sx={{ width: 110 }} />
        <TextField size="small" label="App ID" value={filters.app} onChange={setFilter("app")} inputProps={{ "data-testid": "report-app" }} sx={{ width: 110 }} />
        <FormControlLabel
          control={<Checkbox checked={includeRevoked} onChange={(e) => setIncludeRevoked(e.target.checked)} inputProps={{ "data-testid": "report-include-revoked" }} />}
          label="Include closed grants"
        />
        <Button startIcon={<RefreshIcon />} onClick={load} disabled={loading}>
          Refresh
        </Button>
      </Box>
      <StyledPaper>
        <TableContainer>
          <Table size="small">
            <TableHead>
              <TableRow>
                <StyledTableHeaderCell>User</StyledTableHeaderCell>
                <StyledTableHeaderCell>App</StyledTableHeaderCell>
                <StyledTableHeaderCell>MCP server</StyledTableHeaderCell>
                <StyledTableHeaderCell>Connection</StyledTableHeaderCell>
                <StyledTableHeaderCell>Access</StyledTableHeaderCell>
                <StyledTableHeaderCell>Credential</StyledTableHeaderCell>
                <StyledTableHeaderCell>Granted</StyledTableHeaderCell>
                <StyledTableHeaderCell>Revoked</StyledTableHeaderCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {loading && rows.length === 0 ? (
                <TableRow>
                  <StyledTableCell colSpan={8} align="center">
                    <CircularProgress size={20} />
                  </StyledTableCell>
                </TableRow>
              ) : rows.length === 0 ? (
                <TableRow>
                  <StyledTableCell colSpan={8} align="center" data-testid="report-empty">
                    No grants match.
                  </StyledTableCell>
                </TableRow>
              ) : (
                rows.map((row) => (
                  <StyledTableRow key={row.grant_id} hover data-testid={`grant-${row.grant_id}`}>
                    <StyledTableCell>{row.user_email || row.user_id}</StyledTableCell>
                    <StyledTableCell>
                      <Link component={RouterLink} to={`/admin/apps/${row.app_id}`}>
                        {row.app_name}
                      </Link>
                    </StyledTableCell>
                    <StyledTableCell>
                      <Link component={RouterLink} to={`/admin/mcp-servers/${row.server_id}`}>
                        {row.server_name}
                      </Link>
                    </StyledTableCell>
                    <StyledTableCell>{row.connection_name || row.connection_id || "—"}</StyledTableCell>
                    <StyledTableCell>
                      <Chip size="small" variant="outlined" label={GRANT_LABELS[row.grant_kind] || row.grant_kind} />
                    </StyledTableCell>
                    <StyledTableCell>
                      {row.credential_id ? (
                        <>
                          <CredentialStatusChip status={row.credential_status} />
                          <Typography variant="caption" display="block" sx={{ fontFamily: "monospace" }}>
                            {(row.credential_hash || "").slice(0, 10)}
                          </Typography>
                        </>
                      ) : (
                        <Typography variant="caption" color="text.secondary">
                          No key
                        </Typography>
                      )}
                    </StyledTableCell>
                    <StyledTableCell>{formatTime(row.granted_at)}</StyledTableCell>
                    <StyledTableCell>
                      {row.revoked_at ? formatTime(row.revoked_at) : "—"}
                      {row.revoke_reason && (
                        <Typography variant="caption" display="block" color="text.secondary">
                          {row.revoke_reason}
                        </Typography>
                      )}
                    </StyledTableCell>
                  </StyledTableRow>
                ))
              )}
            </TableBody>
          </Table>
        </TableContainer>
      </StyledPaper>
    </Box>
  );
};

const MintDialog = ({ open, connections, onClose, onMinted }) => {
  const [appId, setAppId] = useState("");
  const [connectionId, setConnectionId] = useState("");
  const [error, setError] = useState(null);
  const [busy, setBusy] = useState(false);
  const submit = async () => {
    setBusy(true);
    setError(null);
    try {
      const res = await apiClient.post("/mcp-credentials", { app_id: Number(appId), connection_id: Number(connectionId) });
      onMinted(res.data);
      setAppId("");
      setConnectionId("");
    } catch (err) {
      setError(apiErrorDetail(err, "Failed to mint the key"));
    } finally {
      setBusy(false);
    }
  };
  return (
    <Dialog open={open} onClose={onClose} data-testid="mint-dialog">
      <DialogTitle>Mint a Tyk key for an App</DialogTitle>
      <DialogContent>
        <Typography variant="body2" sx={{ mb: 2 }}>
          The App must be approved and bound to at least one MCP server on the connection. The key is shown to you once; pass it to the App owner
          securely, or ask them to mint it themselves from the portal.
        </Typography>
        {error && (
          <Alert severity="error" sx={{ mb: 2 }} data-testid="mint-error">
            {error}
          </Alert>
        )}
        <TextField fullWidth size="small" label="App ID" value={appId} onChange={(e) => setAppId(e.target.value)} inputProps={{ "data-testid": "mint-app" }} sx={{ mb: 2 }} />
        <FormControl fullWidth size="small">
          <InputLabel>Connection</InputLabel>
          <Select label="Connection" value={connectionId} onChange={(e) => setConnectionId(e.target.value)} data-testid="mint-connection">
            {connections.map((c) => (
              <MenuItem key={c.id} value={String(c.id)}>
                {c.name}
              </MenuItem>
            ))}
          </Select>
        </FormControl>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>Cancel</Button>
        <Button variant="contained" disabled={busy || !appId || !connectionId} onClick={submit} data-testid="mint-submit">
          Mint key
        </Button>
      </DialogActions>
    </Dialog>
  );
};

const MCPCredentials = () => {
  const [searchParams] = useSearchParams();
  const [status, setStatus] = useState(null);
  const [connections, setConnections] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [tab, setTab] = useState(searchParams.get("tab") === "report" ? 1 : 0);
  const [mintOpen, setMintOpen] = useState(false);
  const [minted, setMinted] = useState(null);
  const [reloadKey, setReloadKey] = useState(0);

  useEffect(() => {
    const load = async () => {
      try {
        const st = await apiClient.get("/tyk-mcp/status");
        setStatus(st.data);
        if (st.data?.available && st.data?.enabled) {
          const conns = await apiClient.get("/tyk-connections");
          setConnections(conns.data || []);
        }
      } catch (err) {
        setError(apiErrorDetail(err, "Failed to load"));
      } finally {
        setLoading(false);
      }
    };
    load();
  }, []);

  if (loading) {
    return (
      <Box sx={{ p: 3, display: "flex", justifyContent: "center" }}>
        <CircularProgress />
      </Box>
    );
  }
  if (status && !status.available) return <TykUpsell />;
  if (status && !status.enabled) return <TykDisabledNotice status={status} />;

  const initialCredentialFilters = {};
  ["connection_id", "status", "drift", "app_id", "server_id"].forEach((k) => {
    if (searchParams.get(k)) initialCredentialFilters[k] = searchParams.get(k);
  });
  const initialReportFilters = {};
  ["connection", "server", "user", "app"].forEach((k) => {
    if (searchParams.get(k)) initialReportFilters[k] = searchParams.get(k);
  });

  return (
    <Box>
      <TitleBox>
        <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
          <KeyIcon />
          <Typography variant="h5">MCP credentials</Typography>
        </Box>
        <Can permission={P.MCP_CREDENTIALS_EXECUTE}>
          <Button variant="contained" startIcon={<KeyIcon />} onClick={() => setMintOpen(true)} data-testid="open-mint" disabled={connections.length === 0}>
            Mint key
          </Button>
        </Can>
      </TitleBox>
      {error && (
        <Alert severity="error" sx={{ mb: 2 }} onClose={() => setError(null)}>
          {error}
        </Alert>
      )}
      <Tabs value={tab} onChange={(_, v) => setTab(v)} sx={{ mb: 2 }}>
        <Tab label="Minted keys" data-testid="tab-credentials" />
        <Tab label="Access report" data-testid="tab-report" />
      </Tabs>
      {tab === 0 ? (
        <CredentialsTab key={reloadKey} connections={connections} initial={initialCredentialFilters} />
      ) : (
        <AccessReportTab connections={connections} initial={initialReportFilters} />
      )}
      <MintDialog
        open={mintOpen}
        connections={connections}
        onClose={() => setMintOpen(false)}
        onMinted={(m) => {
          setMintOpen(false);
          setMinted(m);
          setReloadKey((k) => k + 1);
        }}
      />
      <RevealKeyDialog minted={minted} onClose={() => setMinted(null)} />
    </Box>
  );
};

export default MCPCredentials;
