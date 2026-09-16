import React, { useCallback, useEffect, useState } from "react";
import {
  Alert,
  Box,
  Button,
  Checkbox,
  Chip,
  CircularProgress,
  Collapse,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Divider,
  FormControlLabel,
  Grid,
  IconButton,
  Menu,
  MenuItem,
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
import AddIcon from "@mui/icons-material/Add";
import MoreVertIcon from "@mui/icons-material/MoreVert";
import ExpandMoreIcon from "@mui/icons-material/ExpandMore";
import ExpandLessIcon from "@mui/icons-material/ExpandLess";
import apiClient from "../utils/apiClient";
import { usePermissions } from "../context/PermissionsContext";
import { P } from "../rbac/permissions";
import Can from "../components/rbac/Can";
import {
  TitleBox,
  StyledPaper,
  StyledTableHeaderCell,
  StyledTableCell,
  StyledTableRow,
} from "../styles/sharedStyles";
import { formatTime, apiErrorDetail } from "./webhookShared";
import {
  CONNECTION_MODES,
  MODE_LABELS,
  MODE_HELP,
  ConnectionStatusChip,
  ModeChip,
  CapabilityChips,
  TykUpsell,
  TykDisabledNotice,
} from "./tykShared";

const emptyForm = {
  name: "",
  description: "",
  dashboard_url: "",
  dashboard_access_token: "",
  gateway_base_url: "",
  org_id: "",
  declared_mode: "catalogue",
  sync_interval_seconds: 300,
  allow_internal_host: false,
  auto_publish: false,
  default_privacy_score: "",
  accept_handoffs: true,
  alias_prefix: "studio:",
  expires_in_seconds: 0,
  mdcb_url: "",
  mdcb_access_token: "",
  mdcb_allow_internal_host: false,
  known_gateway_tags: "",
  gateway_base_urls: [],
};

const parseTags = (text) =>
  text
    .split(",")
    .map((t) => t.trim())
    .filter(Boolean)
    .map((tag) => ({ tag }));

const baseUrlsToMap = (rows) =>
  rows.reduce((acc, row) => {
    if (row.tag.trim()) acc[row.tag.trim()] = row.url.trim();
    return acc;
  }, {});

const formToInput = (form, editing) => {
  const input = {
    name: form.name,
    description: form.description,
    dashboard_url: form.dashboard_url,
    gateway_base_url: form.gateway_base_url,
    org_id: form.org_id,
    declared_mode: form.declared_mode,
    sync_interval_seconds: Number(form.sync_interval_seconds) || 0,
    allow_internal_host: Boolean(form.allow_internal_host),
    auto_publish: Boolean(form.auto_publish),
    accept_handoffs: Boolean(form.accept_handoffs),
    key_defaults: { alias_prefix: form.alias_prefix || "studio:", expires_in_seconds: Number(form.expires_in_seconds) || 0 },
    mdcb_url: form.mdcb_url,
    mdcb_allow_internal_host: Boolean(form.mdcb_allow_internal_host),
    known_gateway_tags: parseTags(form.known_gateway_tags),
    gateway_base_urls: baseUrlsToMap(form.gateway_base_urls),
  };
  if (form.default_privacy_score !== "" && form.default_privacy_score !== null) {
    input.default_privacy_score = Number(form.default_privacy_score);
  } else if (editing) {
    input.clear_default_privacy_score = true;
  }
  if (form.dashboard_access_token) input.dashboard_access_token = form.dashboard_access_token;
  if (form.mdcb_access_token) input.mdcb_access_token = form.mdcb_access_token;
  return input;
};

// ProbePanel shows what a probe learned about a Dashboard.
const ProbePanel = ({ result }) => {
  if (!result) return null;
  return (
    <Box sx={{ mt: 2 }} data-testid="probe-panel">
      <Alert severity={result.reachable ? (result.effective_mode ? "success" : "warning") : "error"} sx={{ mb: 1 }}>
        {result.reachable
          ? result.effective_mode
            ? `Dashboard reachable. Effective mode: ${MODE_LABELS[result.effective_mode]}.`
            : "Dashboard reachable but not usable in any mode."
          : "Dashboard not reachable with these settings."}
        {result.org_id ? ` Organisation ${result.org_id}.` : ""}
      </Alert>
      <CapabilityChips capabilities={result.capabilities} />
      {(result.warnings || []).length > 0 && (
        <Box sx={{ mt: 1 }}>
          {result.warnings.map((w, i) => (
            <Typography key={i} variant="body2" color="warning.main">
              {w}
            </Typography>
          ))}
        </Box>
      )}
      {(result.data_planes || []).length > 0 && (
        <Box sx={{ mt: 1 }}>
          <Typography variant="subtitle2">Data planes (MDCB)</Typography>
          {result.data_planes.map((dp) => (
            <Typography key={dp.group_id} variant="body2">
              {dp.group_id}: {dp.node_count} node(s), tags {dp.tags.join(", ") || "none"}
              {dp.healthy ? "" : " (unhealthy)"}
            </Typography>
          ))}
        </Box>
      )}
    </Box>
  );
};

// ConnectionForm creates or edits a connection. Tokens are never loaded
// back from the server; leaving the field empty keeps the stored token.
const ConnectionForm = ({ open, connection, canExecute, onClose, onSaved }) => {
  const editing = Boolean(connection);
  const [form, setForm] = useState(emptyForm);
  const [saving, setSaving] = useState(false);
  const [probing, setProbing] = useState(false);
  const [probe, setProbe] = useState(null);
  const [error, setError] = useState(null);

  useEffect(() => {
    if (!open) return;
    setError(null);
    setProbe(null);
    if (connection) {
      setForm({
        ...emptyForm,
        name: connection.name || "",
        description: connection.description || "",
        dashboard_url: connection.dashboard_url || "",
        gateway_base_url: connection.gateway_base_url || "",
        org_id: connection.org_id || "",
        declared_mode: connection.declared_mode || "catalogue",
        sync_interval_seconds: connection.sync_interval_seconds || 300,
        allow_internal_host: Boolean(connection.allow_internal_host),
        auto_publish: Boolean(connection.auto_publish),
        default_privacy_score:
          connection.default_privacy_score === null || connection.default_privacy_score === undefined
            ? ""
            : connection.default_privacy_score,
        accept_handoffs: connection.accept_handoffs !== false,
        alias_prefix: connection.key_defaults?.alias_prefix || "studio:",
        expires_in_seconds: connection.key_defaults?.expires_in_seconds || 0,
        mdcb_url: connection.mdcb_url || "",
        mdcb_allow_internal_host: Boolean(connection.mdcb_allow_internal_host),
        known_gateway_tags: (connection.known_gateway_tags || []).map((t) => t.tag).join(", "),
        gateway_base_urls: Object.entries(connection.gateway_base_urls || {}).map(([tag, url]) => ({ tag, url })),
      });
    } else {
      setForm(emptyForm);
    }
  }, [open, connection]);

  const set = (field) => (e) => setForm({ ...form, [field]: e.target.value });
  const setBool = (field) => (e) => setForm({ ...form, [field]: e.target.checked });

  const runProbe = async () => {
    setProbing(true);
    setError(null);
    try {
      const input = formToInput(form, editing);
      if (editing && !form.dashboard_access_token) {
        setError("Enter the access token to test unsaved settings, or save and use Probe from the menu.");
        setProbing(false);
        return;
      }
      const res = await apiClient.post("/tyk-connections/probe", input);
      setProbe(res.data);
    } catch (err) {
      setError(apiErrorDetail(err, "Probe failed"));
    } finally {
      setProbing(false);
    }
  };

  const save = async () => {
    setSaving(true);
    setError(null);
    try {
      const input = formToInput(form, editing);
      if (editing) {
        const res = await apiClient.patch(`/tyk-connections/${connection.id}`, {
          ...input,
          lock_version: connection.lock_version,
        });
        onSaved(res.data);
      } else {
        const res = await apiClient.post("/tyk-connections", input);
        onSaved(res.data);
      }
      onClose();
    } catch (err) {
      setError(apiErrorDetail(err, "Save failed"));
    } finally {
      setSaving(false);
    }
  };

  const updateBaseURL = (i, field, value) => {
    const rows = form.gateway_base_urls.map((r, idx) => (idx === i ? { ...r, [field]: value } : r));
    setForm({ ...form, gateway_base_urls: rows });
  };

  return (
    <Dialog open={open} onClose={onClose} maxWidth="md" fullWidth>
      <DialogTitle>{editing ? `Edit ${connection.name}` : "Connect a Tyk Dashboard"}</DialogTitle>
      <DialogContent>
        {error && (
          <Alert severity="error" sx={{ mb: 2 }} data-testid="form-error">
            {error}
          </Alert>
        )}
        <Grid container spacing={2} sx={{ mt: 0 }}>
          <Grid item xs={12} sm={6}>
            <TextField fullWidth size="small" label="Name" value={form.name} onChange={set("name")} required />
          </Grid>
          <Grid item xs={12} sm={6}>
            <TextField
              select
              fullWidth
              size="small"
              label="Trust mode"
              value={form.declared_mode}
              onChange={set("declared_mode")}
              helperText={MODE_HELP[form.declared_mode]}
            >
              {CONNECTION_MODES.map((m) => (
                <MenuItem key={m} value={m}>
                  {MODE_LABELS[m]}
                </MenuItem>
              ))}
            </TextField>
          </Grid>
          <Grid item xs={12}>
            <TextField fullWidth size="small" label="Description" value={form.description} onChange={set("description")} />
          </Grid>
          <Grid item xs={12} sm={8}>
            <TextField
              fullWidth
              size="small"
              label="Dashboard URL"
              placeholder="https://dashboard.example.com"
              value={form.dashboard_url}
              onChange={set("dashboard_url")}
              required
            />
          </Grid>
          <Grid item xs={12} sm={4}>
            <TextField fullWidth size="small" label="Organisation ID (optional)" value={form.org_id} onChange={set("org_id")} />
          </Grid>
          <Grid item xs={12}>
            <TextField
              fullWidth
              size="small"
              type="password"
              label={editing ? "Dashboard access token (leave empty to keep)" : "Dashboard access token"}
              helperText="The API access key of a dedicated Dashboard user. Stored encrypted, never shown again."
              value={form.dashboard_access_token}
              onChange={set("dashboard_access_token")}
              required={!editing}
              inputProps={{ "data-testid": "token-input" }}
            />
          </Grid>
          <Grid item xs={12} sm={8}>
            <TextField
              fullWidth
              size="small"
              label="Public gateway base URL"
              placeholder="https://gateway.example.com"
              helperText="The URL clients use to reach MCP proxies; shown in connection instructions."
              value={form.gateway_base_url}
              onChange={set("gateway_base_url")}
            />
          </Grid>
          <Grid item xs={12} sm={4}>
            <TextField
              fullWidth
              size="small"
              type="number"
              label="Sync interval (seconds)"
              value={form.sync_interval_seconds}
              onChange={set("sync_interval_seconds")}
            />
          </Grid>
          {canExecute && (
            <Grid item xs={12}>
              <FormControlLabel
                control={<Checkbox checked={form.allow_internal_host} onChange={setBool("allow_internal_host")} />}
                label="Allow this Dashboard host to be on an internal network address"
              />
            </Grid>
          )}

          <Grid item xs={12}>
            <Divider />
            <Typography variant="subtitle2" sx={{ mt: 1 }}>
              Publishing and keys
            </Typography>
          </Grid>
          <Grid item xs={12} sm={4}>
            <FormControlLabel
              control={<Checkbox checked={form.auto_publish} onChange={setBool("auto_publish")} />}
              label="Auto-publish imported servers"
            />
          </Grid>
          <Grid item xs={12} sm={4}>
            <TextField
              fullWidth
              size="small"
              type="number"
              label="Default privacy score"
              helperText={form.auto_publish ? "Required for auto-publish" : "Optional; admins set scores before publishing"}
              value={form.default_privacy_score}
              onChange={set("default_privacy_score")}
              inputProps={{ min: 0, max: 100 }}
            />
          </Grid>
          <Grid item xs={12} sm={4}>
            <FormControlLabel
              control={<Checkbox checked={form.accept_handoffs} onChange={setBool("accept_handoffs")} />}
              label="Accept community registrations as handoffs"
            />
          </Grid>
          <Grid item xs={12} sm={6}>
            <TextField fullWidth size="small" label="Key alias prefix" value={form.alias_prefix} onChange={set("alias_prefix")} />
          </Grid>
          <Grid item xs={12} sm={6}>
            <TextField
              fullWidth
              size="small"
              type="number"
              label="Key expiry (seconds, 0 = never)"
              value={form.expires_in_seconds}
              onChange={set("expires_in_seconds")}
            />
          </Grid>

          <Grid item xs={12}>
            <Divider />
            <Typography variant="subtitle2" sx={{ mt: 1 }}>
              Gateway segmentation (optional)
            </Typography>
            <Typography variant="body2" color="text.secondary">
              In sharded deployments a proxy is loaded only by gateways whose tags match. Configure MDCB to
              discover data planes, or list the tags by hand.
            </Typography>
          </Grid>
          <Grid item xs={12} sm={7}>
            <TextField
              fullWidth
              size="small"
              label="MDCB URL"
              placeholder="https://mdcb.example.com"
              value={form.mdcb_url}
              onChange={set("mdcb_url")}
            />
          </Grid>
          <Grid item xs={12} sm={5}>
            <TextField
              fullWidth
              size="small"
              type="password"
              label={editing ? "MDCB secret (leave empty to keep)" : "MDCB secret"}
              value={form.mdcb_access_token}
              onChange={set("mdcb_access_token")}
            />
          </Grid>
          {canExecute && (
            <Grid item xs={12}>
              <FormControlLabel
                control={<Checkbox checked={form.mdcb_allow_internal_host} onChange={setBool("mdcb_allow_internal_host")} />}
                label="Allow the MDCB host to be on an internal network address"
              />
            </Grid>
          )}
          <Grid item xs={12}>
            <TextField
              fullWidth
              size="small"
              label="Known gateway tags (comma separated)"
              value={form.known_gateway_tags}
              onChange={set("known_gateway_tags")}
            />
          </Grid>
          <Grid item xs={12}>
            <Typography variant="body2" sx={{ mb: 1 }}>
              Public base URL per tag
            </Typography>
            {form.gateway_base_urls.map((row, i) => (
              <Box key={i} sx={{ display: "flex", gap: 1, mb: 1 }}>
                <TextField size="small" label="Tag" value={row.tag} onChange={(e) => updateBaseURL(i, "tag", e.target.value)} sx={{ flex: 1 }} />
                <TextField size="small" label="URL" value={row.url} onChange={(e) => updateBaseURL(i, "url", e.target.value)} sx={{ flex: 2 }} />
                <Button size="small" onClick={() => setForm({ ...form, gateway_base_urls: form.gateway_base_urls.filter((_, idx) => idx !== i) })}>
                  Remove
                </Button>
              </Box>
            ))}
            <Button size="small" onClick={() => setForm({ ...form, gateway_base_urls: [...form.gateway_base_urls, { tag: "", url: "" }] })}>
              Add tag URL
            </Button>
          </Grid>
        </Grid>
        <ProbePanel result={probe} />
      </DialogContent>
      <DialogActions>
        <Button onClick={runProbe} disabled={probing || !form.dashboard_url} data-testid="probe-button">
          {probing ? "Testing…" : "Test connection"}
        </Button>
        <Box sx={{ flex: 1 }} />
        <Button onClick={onClose} disabled={saving}>
          Cancel
        </Button>
        <Button
          variant="contained"
          onClick={save}
          disabled={saving || !form.name || !form.dashboard_url || (!editing && !form.dashboard_access_token)}
          data-testid="save-button"
        >
          {editing ? "Save" : "Create (pending activation)"}
        </Button>
      </DialogActions>
    </Dialog>
  );
};

const ConfirmDialog = ({ action, connection, onClose, onConfirm }) => {
  const [text, setText] = useState("");
  useEffect(() => setText(""), [action]);
  if (!action) return null;
  const copy = {
    activate: {
      title: "Activate",
      body: `Activating probes ${connection?.dashboard_url} and lets AI Studio import MCP proxies${
        connection?.declared_mode !== "catalogue" ? " and write to the Dashboard" : ""
      }.`,
      color: "primary",
    },
    disable: {
      title: "Disable",
      body: "Syncing stops and every key minted on this connection is suspended on the Dashboard.",
      color: "warning",
    },
    delete: {
      title: "Delete",
      body: "The connection and its imported servers are removed from AI Studio. Live credentials must be revoked first.",
      color: "error",
    },
  }[action];
  return (
    <Dialog open onClose={onClose}>
      <DialogTitle>
        {copy.title} {connection?.name}
      </DialogTitle>
      <DialogContent>
        <Typography sx={{ mb: 2 }}>{copy.body}</Typography>
        {action !== "activate" && (
          <TextField fullWidth size="small" label="Reason (optional)" value={text} onChange={(e) => setText(e.target.value)} />
        )}
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>Cancel</Button>
        <Button variant="contained" color={copy.color} onClick={() => onConfirm(text)} data-testid="confirm-action">
          {copy.title}
        </Button>
      </DialogActions>
    </Dialog>
  );
};

const ConnectionRow = ({ conn, onMenu }) => {
  const [open, setOpen] = useState(false);
  return (
    <>
      <StyledTableRow hover data-testid={`connection-row-${conn.id}`}>
        <StyledTableCell>
          <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
            <IconButton size="small" onClick={() => setOpen(!open)} aria-label="details">
              {open ? <ExpandLessIcon fontSize="small" /> : <ExpandMoreIcon fontSize="small" />}
            </IconButton>
            <Box>
              <Typography variant="body2" fontWeight={600}>
                {conn.name}
              </Typography>
              <Typography variant="caption" color="text.secondary">
                {conn.dashboard_url}
              </Typography>
            </Box>
          </Box>
        </StyledTableCell>
        <StyledTableCell>
          <ModeChip declared={conn.declared_mode} effective={conn.effective_mode} />
        </StyledTableCell>
        <StyledTableCell>
          <ConnectionStatusChip status={conn.status} degraded={conn.degraded} />
        </StyledTableCell>
        <StyledTableCell>
          {conn.last_sync_at ? (
            <Tooltip title={conn.last_sync_error || ""}>
              <span>
                {formatTime(conn.last_sync_at)} {conn.last_sync_status ? `(${conn.last_sync_status})` : ""}
              </span>
            </Tooltip>
          ) : (
            <Typography variant="caption" color="text.secondary">
              never
            </Typography>
          )}
        </StyledTableCell>
        <StyledTableCell>
          {(conn.gateway_tags || []).length > 0 ? (
            <Chip size="small" label={`${conn.gateway_tags.length} tag(s)`} variant="outlined" />
          ) : (
            <Typography variant="caption" color="text.secondary">
              none
            </Typography>
          )}
        </StyledTableCell>
        <StyledTableCell align="right">
          <IconButton size="small" onClick={(e) => onMenu(e, conn)} aria-label="actions" data-testid={`menu-${conn.id}`}>
            <MoreVertIcon fontSize="small" />
          </IconButton>
        </StyledTableCell>
      </StyledTableRow>
      <TableRow>
        <StyledTableCell colSpan={6} sx={{ p: 0, borderBottom: open ? undefined : "none" }}>
          <Collapse in={open} unmountOnExit>
            <Box sx={{ p: 2 }}>
              {conn.degraded && (
                <Alert severity="error" sx={{ mb: 1 }}>
                  {conn.degraded_reason}
                </Alert>
              )}
              <Typography variant="subtitle2">Capabilities</Typography>
              <CapabilityChips capabilities={conn.capabilities} />
              <Typography variant="caption" color="text.secondary">
                Last probed {conn.last_probe_at ? formatTime(conn.last_probe_at) : "never"}
                {conn.org_id ? ` · organisation ${conn.org_id}` : ""}
                {conn.activated_by_email ? ` · activated by ${conn.activated_by_email}` : ""}
              </Typography>
              {(conn.data_planes || []).length > 0 && (
                <Box sx={{ mt: 1 }}>
                  <Typography variant="subtitle2">Data planes</Typography>
                  {conn.data_planes.map((dp) => (
                    <Typography key={dp.group_id} variant="body2">
                      {dp.group_id}: {dp.node_count} node(s), tags {dp.tags.join(", ") || "none"}
                      {dp.healthy ? "" : " (unhealthy)"}
                    </Typography>
                  ))}
                </Box>
              )}
              {(conn.gateway_tags || []).length > 0 && (
                <Box sx={{ mt: 1, display: "flex", gap: 0.5, flexWrap: "wrap" }}>
                  {conn.gateway_tags.map((t) => (
                    <Chip key={t} size="small" label={t} />
                  ))}
                </Box>
              )}
            </Box>
          </Collapse>
        </StyledTableCell>
      </TableRow>
    </>
  );
};

const TykConnections = () => {
  const { can } = usePermissions();
  const canExecute = can(P.TYK_CONNECTIONS_EXECUTE);
  const [status, setStatus] = useState(null);
  const [connections, setConnections] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [notice, setNotice] = useState(null);
  const [formOpen, setFormOpen] = useState(false);
  const [editing, setEditing] = useState(null);
  const [menu, setMenu] = useState({ anchor: null, conn: null });
  const [confirm, setConfirm] = useState({ action: null, conn: null });

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const st = await apiClient.get("/tyk-mcp/status");
      setStatus(st.data);
      if (st.data?.available && st.data?.enabled) {
        const res = await apiClient.get("/tyk-connections");
        setConnections(res.data || []);
      }
    } catch (err) {
      setError(apiErrorDetail(err, "Failed to load connections"));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  const closeMenu = () => setMenu({ anchor: null, conn: null });

  const run = async (fn, success) => {
    setError(null);
    try {
      await fn();
      if (success) setNotice(success);
      await load();
    } catch (err) {
      setError(apiErrorDetail(err, "Action failed"));
    }
  };

  const onConfirm = async (reason) => {
    const { action, conn } = confirm;
    setConfirm({ action: null, conn: null });
    if (action === "activate") {
      await run(() => apiClient.post(`/tyk-connections/${conn.id}/activate`, {}), `${conn.name} activated`);
    } else if (action === "disable") {
      await run(() => apiClient.post(`/tyk-connections/${conn.id}/disable`, { reason }), `${conn.name} disabled`);
    } else if (action === "delete") {
      await run(() => apiClient.delete(`/tyk-connections/${conn.id}`), `${conn.name} deleted`);
    }
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

  return (
    <Box>
      <TitleBox>
        <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
          <HubIcon />
          <Typography variant="h5">Tyk Dashboard connections</Typography>
        </Box>
        <Box sx={{ display: "flex", gap: 1 }}>
          <Button startIcon={<RefreshIcon />} onClick={load} disabled={loading}>
            Refresh
          </Button>
          <Can permission={P.TYK_CONNECTIONS_WRITE}>
            <Button
              variant="contained"
              startIcon={<AddIcon />}
              onClick={() => {
                setEditing(null);
                setFormOpen(true);
              }}
              data-testid="add-connection"
            >
              Connect Dashboard
            </Button>
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

      <StyledPaper>
        {connections.length === 0 ? (
          <Box sx={{ p: 3 }}>
            <Typography color="text.secondary">
              No Tyk Dashboard is connected yet. Connect one to import its MCP proxies into the AI Portal and
              broker access keys for Apps.
            </Typography>
          </Box>
        ) : (
          <TableContainer>
            <Table size="small">
              <TableHead>
                <TableRow>
                  <StyledTableHeaderCell>Connection</StyledTableHeaderCell>
                  <StyledTableHeaderCell>Mode</StyledTableHeaderCell>
                  <StyledTableHeaderCell>Status</StyledTableHeaderCell>
                  <StyledTableHeaderCell>Last sync</StyledTableHeaderCell>
                  <StyledTableHeaderCell>Gateway tags</StyledTableHeaderCell>
                  <StyledTableHeaderCell align="right">Actions</StyledTableHeaderCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {connections.map((conn) => (
                  <ConnectionRow key={conn.id} conn={conn} onMenu={(e, c) => setMenu({ anchor: e.currentTarget, conn: c })} />
                ))}
              </TableBody>
            </Table>
          </TableContainer>
        )}
      </StyledPaper>

      <Menu anchorEl={menu.anchor} open={Boolean(menu.anchor)} onClose={closeMenu}>
        <Can permission={P.TYK_CONNECTIONS_WRITE}>
          <MenuItem
            onClick={() => {
              setEditing(menu.conn);
              setFormOpen(true);
              closeMenu();
            }}
          >
            Edit
          </MenuItem>
        </Can>
        <Can permission={P.TYK_CONNECTIONS_EXECUTE}>
          {menu.conn?.status !== "active" && (
            <MenuItem
              onClick={() => {
                setConfirm({ action: "activate", conn: menu.conn });
                closeMenu();
              }}
              data-testid="menu-activate"
            >
              Activate
            </MenuItem>
          )}
          {menu.conn?.status === "active" && (
            <MenuItem
              onClick={() => {
                const c = menu.conn;
                closeMenu();
                run(() => apiClient.post(`/tyk-connections/${c.id}/sync`, {}), "Sync requested");
              }}
            >
              Sync now
            </MenuItem>
          )}
          <MenuItem
            onClick={() => {
              const c = menu.conn;
              closeMenu();
              run(() => apiClient.post(`/tyk-connections/${c.id}/probe`, {}), "Probe complete");
            }}
          >
            Probe
          </MenuItem>
          {menu.conn?.status === "active" && (
            <MenuItem
              onClick={() => {
                setConfirm({ action: "disable", conn: menu.conn });
                closeMenu();
              }}
            >
              Disable
            </MenuItem>
          )}
        </Can>
        <Can permission={P.TYK_CONNECTIONS_DELETE}>
          <MenuItem
            onClick={() => {
              setConfirm({ action: "delete", conn: menu.conn });
              closeMenu();
            }}
            sx={{ color: "error.main" }}
          >
            Delete
          </MenuItem>
        </Can>
      </Menu>

      <ConnectionForm
        open={formOpen}
        connection={editing}
        canExecute={canExecute}
        onClose={() => setFormOpen(false)}
        onSaved={() => {
          setNotice(editing ? "Connection saved" : "Connection created; activate it to start syncing");
          load();
        }}
      />
      <ConfirmDialog
        action={confirm.action}
        connection={confirm.conn}
        onClose={() => setConfirm({ action: null, conn: null })}
        onConfirm={onConfirm}
      />
    </Box>
  );
};

export default TykConnections;
