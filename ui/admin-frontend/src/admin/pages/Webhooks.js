import React, { useCallback, useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import {
  Alert,
  Autocomplete,
  Badge,
  Box,
  Button,
  Chip,
  CircularProgress,
  Collapse,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
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
import WebhookIcon from "@mui/icons-material/Webhook";
import RefreshIcon from "@mui/icons-material/Refresh";
import AddIcon from "@mui/icons-material/Add";
import MoreVertIcon from "@mui/icons-material/MoreVert";
import ExpandMoreIcon from "@mui/icons-material/ExpandMore";
import ExpandLessIcon from "@mui/icons-material/ExpandLess";
import ListAltIcon from "@mui/icons-material/ListAlt";
import apiClient from "../utils/apiClient";
import {
  TitleBox,
  StyledPaper,
  StyledTableHeaderCell,
  StyledTableCell,
  StyledTableRow,
} from "../styles/sharedStyles";
import {
  TARGET_STATUSES,
  StatusChip,
  HealthPill,
  EnterpriseUpsell,
  DisabledNotice,
  BusWarning,
  formatTime,
  apiErrorDetail,
  humanStatus,
  preStyle,
} from "./webhookShared";

const emptyForm = {
  name: "",
  description: "",
  url: "",
  topic_filters: ["system.*.*"],
  template_preset: "standard",
  template_body: "",
  headers: [],
  max_concurrency: 0,
  signing_secret: "",
};

const headersToMap = (rows) =>
  rows.reduce((acc, row) => {
    if (row.name.trim()) acc[row.name.trim()] = row.value;
    return acc;
  }, {});

// TargetForm creates or edits a target. Header values are never loaded
// back from the server; editing headers means re-entering them.
const TargetForm = ({ open, target, presets, topics, onClose, onSaved }) => {
  const editing = Boolean(target);
  const [form, setForm] = useState(emptyForm);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState(null);
  const [preview, setPreview] = useState(null);
  const [headersTouched, setHeadersTouched] = useState(false);

  useEffect(() => {
    if (!open) return;
    setError(null);
    setPreview(null);
    setHeadersTouched(false);
    if (target) {
      setForm({
        name: target.name || "",
        description: target.description || "",
        url: target.url || "",
        topic_filters: target.topic_filters || [],
        template_preset: target.template_preset || "standard",
        template_body: target.template_body || "",
        headers: (target.header_names || []).map((name) => ({ name, value: "" })),
        max_concurrency: target.max_concurrency || 0,
        signing_secret: "",
      });
    } else {
      setForm(emptyForm);
    }
  }, [open, target]);

  const set = (field) => (e) => setForm({ ...form, [field]: e.target.value });

  const runPreview = async () => {
    setError(null);
    try {
      const res = await apiClient.post("/webhooks/templates/preview", {
        template_preset: form.template_preset,
        template_body: form.template_body,
        topic: (form.topic_filters[0] || "").includes("*") ? "" : form.topic_filters[0],
      });
      setPreview(res.data);
    } catch (err) {
      setError(apiErrorDetail(err, "Preview failed"));
    }
  };

  const save = async () => {
    setSaving(true);
    setError(null);
    try {
      const body = {
        name: form.name,
        description: form.description,
        url: form.url,
        topic_filters: form.topic_filters,
        template_preset: form.template_preset,
        template_body: form.template_preset === "custom" ? form.template_body : "",
        max_concurrency: Number(form.max_concurrency) || 0,
      };
      if (editing) {
        if (headersTouched) body.headers = headersToMap(form.headers);
        const res = await apiClient.patch(`/webhooks/targets/${target.id}`, {
          ...body,
          lock_version: target.lock_version,
        });
        onSaved({ target: res.data.target, repended: res.data.repended });
      } else {
        body.headers = headersToMap(form.headers);
        if (form.signing_secret) body.signing_secret = form.signing_secret;
        const res = await apiClient.post("/webhooks/targets", body);
        onSaved({ target: res.data.target, secret: res.data.signing_secret });
      }
    } catch (err) {
      setError(apiErrorDetail(err, "Save failed"));
    } finally {
      setSaving(false);
    }
  };

  const updateHeader = (i, field, value) => {
    const rows = form.headers.map((r, idx) => (idx === i ? { ...r, [field]: value } : r));
    setHeadersTouched(true);
    setForm({ ...form, headers: rows });
  };

  return (
    <Dialog open={open} onClose={onClose} maxWidth="md" fullWidth>
      <DialogTitle>{editing ? "Edit webhook target" : "New webhook target"}</DialogTitle>
      <DialogContent>
        {editing && target.status === "approved" && (
          <Alert severity="info" sx={{ mb: 2 }}>
            Changing the URL or the headers of an approved target sends it back for approval.
          </Alert>
        )}
        {error && (
          <Alert severity="error" sx={{ mb: 2 }}>
            {error}
          </Alert>
        )}
        <Grid container spacing={2} sx={{ mt: 0 }}>
          <Grid item xs={12} sm={6}>
            <TextField fullWidth size="small" label="Name" value={form.name} onChange={set("name")} required />
          </Grid>
          <Grid item xs={12} sm={6}>
            <TextField
              fullWidth
              size="small"
              label="Max concurrency"
              type="number"
              helperText="0 = default (4 in-flight deliveries)"
              value={form.max_concurrency}
              onChange={set("max_concurrency")}
            />
          </Grid>
          <Grid item xs={12}>
            <TextField
              fullWidth
              size="small"
              label="URL"
              placeholder="https://hooks.example.com/tyk"
              value={form.url}
              onChange={set("url")}
              required
              helperText="http or https. Internal addresses are rejected unless WEBHOOKS_ALLOW_INTERNAL_TARGETS is set."
            />
          </Grid>
          <Grid item xs={12}>
            <TextField fullWidth size="small" label="Description" value={form.description} onChange={set("description")} />
          </Grid>
          <Grid item xs={12}>
            <Autocomplete
              multiple
              freeSolo
              options={topics}
              value={form.topic_filters}
              onChange={(_, value) => setForm({ ...form, topic_filters: value })}
              renderTags={(value, getTagProps) =>
                value.map((option, index) => (
                  <Chip variant="outlined" label={option} size="small" {...getTagProps({ index })} />
                ))
              }
              renderInput={(params) => (
                <TextField
                  {...params}
                  size="small"
                  label="Topic filters"
                  helperText="Glob patterns, e.g. system.llm.* or *. Press Enter to add."
                />
              )}
            />
          </Grid>
          <Grid item xs={12} sm={4}>
            <TextField
              select
              fullWidth
              size="small"
              label="Payload template"
              value={form.template_preset}
              onChange={set("template_preset")}
            >
              {presets.map((p) => (
                <MenuItem key={p.name} value={p.name}>
                  {p.name}
                </MenuItem>
              ))}
              <MenuItem value="custom">custom</MenuItem>
            </TextField>
          </Grid>
          <Grid item xs={12} sm={8} sx={{ display: "flex", alignItems: "center", gap: 1 }}>
            <Typography variant="body2" color="text.secondary">
              {presets.find((p) => p.name === form.template_preset)?.description ||
                "Go text/template producing JSON. Secrets in the event are redacted before rendering."}
            </Typography>
            <Button size="small" onClick={runPreview}>
              Preview
            </Button>
          </Grid>
          {form.template_preset === "custom" && (
            <Grid item xs={12}>
              <TextField
                fullWidth
                multiline
                minRows={6}
                size="small"
                label="Template body"
                value={form.template_body}
                onChange={set("template_body")}
                inputProps={{ style: { fontFamily: "monospace", fontSize: "0.8rem" } }}
              />
            </Grid>
          )}
          {preview && (
            <Grid item xs={12}>
              <Typography variant="subtitle2">
                Preview {preview.valid ? "" : "(invalid)"}
              </Typography>
              {preview.valid ? (
                <Box component="pre" sx={preStyle} data-testid="template-preview">
                  {preview.rendered}
                </Box>
              ) : (
                <Alert severity="error">{preview.error}</Alert>
              )}
            </Grid>
          )}
          <Grid item xs={12}>
            <Typography variant="subtitle2" sx={{ mb: 1 }}>
              Custom headers
            </Typography>
            {editing && form.headers.length > 0 && !headersTouched && (
              <Typography variant="caption" color="text.secondary" display="block" sx={{ mb: 1 }}>
                Header values are stored encrypted and never shown. Leave untouched to keep them, or
                re-enter every value to replace them.
              </Typography>
            )}
            {form.headers.map((row, i) => (
              <Box key={i} sx={{ display: "flex", gap: 1, mb: 1 }}>
                <TextField
                  size="small"
                  label="Header"
                  value={row.name}
                  onChange={(e) => updateHeader(i, "name", e.target.value)}
                  sx={{ flex: 1 }}
                />
                <TextField
                  size="small"
                  label="Value"
                  type="password"
                  value={row.value}
                  onChange={(e) => updateHeader(i, "value", e.target.value)}
                  sx={{ flex: 2 }}
                />
                <Button
                  size="small"
                  onClick={() => {
                    setHeadersTouched(true);
                    setForm({ ...form, headers: form.headers.filter((_, idx) => idx !== i) });
                  }}
                >
                  Remove
                </Button>
              </Box>
            ))}
            <Button
              size="small"
              onClick={() => {
                setHeadersTouched(true);
                setForm({ ...form, headers: [...form.headers, { name: "", value: "" }] });
              }}
            >
              Add header
            </Button>
          </Grid>
          {!editing && (
            <Grid item xs={12}>
              <TextField
                fullWidth
                size="small"
                label="Signing secret (optional)"
                helperText="Leave empty to generate one. Shown once after creation."
                value={form.signing_secret}
                onChange={set("signing_secret")}
              />
            </Grid>
          )}
        </Grid>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose} disabled={saving}>
          Cancel
        </Button>
        <Button variant="contained" onClick={save} disabled={saving || !form.name || !form.url}>
          {editing ? "Save" : "Create (pending approval)"}
        </Button>
      </DialogActions>
    </Dialog>
  );
};

const SecretDialog = ({ secret, onClose }) => (
  <Dialog open={Boolean(secret)} onClose={onClose}>
    <DialogTitle>Signing secret</DialogTitle>
    <DialogContent>
      <Typography sx={{ mb: 2 }}>
        Store this secret in the receiving system now. It is not shown again. Every delivery
        carries <code>X-Webhook-Signature: v1=HMAC-SHA256(secret, timestamp + "." + body)</code>.
      </Typography>
      <Box component="pre" sx={preStyle} data-testid="signing-secret">
        {secret}
      </Box>
    </DialogContent>
    <DialogActions>
      <Button variant="contained" onClick={onClose}>
        I have saved it
      </Button>
    </DialogActions>
  </Dialog>
);

// ReasonDialog covers approve (note), reject and revoke (reason).
const ReasonDialog = ({ action, target, onClose, onConfirm }) => {
  const [text, setText] = useState("");
  useEffect(() => setText(""), [action]);
  if (!action) return null;
  const isApprove = action === "approve";
  return (
    <Dialog open onClose={onClose}>
      <DialogTitle>
        {isApprove ? "Approve" : action === "reject" ? "Reject" : "Revoke"} {target?.name}
      </DialogTitle>
      <DialogContent>
        {isApprove ? (
          <Alert severity="warning" sx={{ mb: 2 }}>
            Approving allows AI Studio to POST event data to <code>{target?.url}</code>. Custom header
            values are stored encrypted and are not shown here; only their names are:{" "}
            {(target?.header_names || []).join(", ") || "none"}.
          </Alert>
        ) : (
          <Typography sx={{ mb: 2 }}>
            {action === "reject"
              ? "The target will not receive deliveries until it is edited and approved again."
              : "Queued deliveries are cancelled and nothing more is sent to this URL."}
          </Typography>
        )}
        <TextField
          fullWidth
          size="small"
          label={isApprove ? "Note (optional)" : "Reason"}
          value={text}
          onChange={(e) => setText(e.target.value)}
        />
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>Cancel</Button>
        <Button
          variant="contained"
          color={isApprove ? "primary" : "error"}
          onClick={() => onConfirm(text)}
          data-testid="confirm-action"
        >
          {isApprove ? "Approve" : action === "reject" ? "Reject" : "Revoke"}
        </Button>
      </DialogActions>
    </Dialog>
  );
};

const TargetDetails = ({ target, detail, history }) => (
  <Box sx={{ p: 2, backgroundColor: "action.hover" }} data-testid="target-details">
    <Grid container spacing={2}>
      <Grid item xs={12} md={6}>
        <Typography variant="subtitle2">Details</Typography>
        <Typography variant="body2">{target.description || "No description"}</Typography>
        <Typography variant="body2" sx={{ mt: 1 }}>
          Template: <code>{target.template_preset}</code> · Headers:{" "}
          {(target.header_names || []).join(", ") || "none"} · Concurrency: {target.max_concurrency || "default"}
        </Typography>
        <Typography variant="body2">
          Created by {target.created_by_email || "?"} on {formatTime(target.created_at)}
          {target.approved_by_email && ` · approved by ${target.approved_by_email} on ${formatTime(target.approved_at)}`}
          {target.revoked_by_email && ` · revoked by ${target.revoked_by_email} on ${formatTime(target.revoked_at)}`}
        </Typography>
        {target.rejected_reason && <Alert severity="error" sx={{ mt: 1, py: 0 }}>Rejected: {target.rejected_reason}</Alert>}
        {target.revoked_reason && <Alert severity="error" sx={{ mt: 1, py: 0 }}>Revoked: {target.revoked_reason}</Alert>}
      </Grid>
      <Grid item xs={12} md={6}>
        <Typography variant="subtitle2">Last 24 hours</Typography>
        {detail === undefined ? (
          <CircularProgress size={18} />
        ) : detail ? (
          <Typography variant="body2">
            {detail.stats.succeeded} succeeded · {detail.stats.dead_lettered} dead-lettered ·{" "}
            {detail.stats.queued + detail.stats.retrying} waiting · {detail.stats.in_flight} in flight · p50{" "}
            {detail.stats.p50_ms} ms · p95 {detail.stats.p95_ms} ms
          </Typography>
        ) : (
          <Typography variant="body2" color="text.secondary">
            Statistics unavailable
          </Typography>
        )}
      </Grid>
      {history && history.length > 0 && (
        <Grid item xs={12}>
          <Typography variant="subtitle2">Audit history</Typography>
          <Table size="small" data-testid="target-audit-history">
            <TableBody>
              {history.map((rec) => (
                <TableRow key={rec.id}>
                  <StyledTableCell sx={{ whiteSpace: "nowrap" }}>{formatTime(rec.timestamp)}</StyledTableCell>
                  <StyledTableCell>{rec.action}</StyledTableCell>
                  <StyledTableCell>{rec.user || "system"}</StyledTableCell>
                  <StyledTableCell>{rec.status}</StyledTableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </Grid>
      )}
    </Grid>
  </Box>
);

const Webhooks = () => {
  const navigate = useNavigate();
  const [status, setStatus] = useState(null);
  const [statusError, setStatusError] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [notice, setNotice] = useState(null);
  const [targets, setTargets] = useState([]);
  const [pendingCount, setPendingCount] = useState(0);
  const [statusFilter, setStatusFilter] = useState("");
  const [presets, setPresets] = useState([]);
  const [topics, setTopics] = useState([]);
  const [auditAvailable, setAuditAvailable] = useState(false);

  const [formOpen, setFormOpen] = useState(false);
  const [editing, setEditing] = useState(null);
  const [secret, setSecret] = useState(null);
  const [menu, setMenu] = useState({ anchor: null, target: null });
  const [reason, setReason] = useState({ action: null, target: null });
  const [expanded, setExpanded] = useState(null);
  const [details, setDetails] = useState({});
  const [history, setHistory] = useState({});

  useEffect(() => {
    let cancelled = false;
    apiClient
      .get("/webhooks/status")
      .then((res) => {
        if (!cancelled) setStatus(res.data);
      })
      .catch(() => {
        if (!cancelled) setStatusError("Failed to check webhook availability");
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    apiClient
      .get("/audit/status")
      .then((res) => {
        if (!cancelled) setAuditAvailable(Boolean(res.data?.available && res.data?.enabled));
      })
      .catch(() => {});
    return () => {
      cancelled = true;
    };
  }, []);

  const usable = status?.available && status?.enabled;

  const fetchTargets = useCallback(async () => {
    if (!usable) return;
    setLoading(true);
    setError(null);
    try {
      const params = {};
      if (statusFilter) params.status = statusFilter;
      const [list, st] = await Promise.all([
        apiClient.get("/webhooks/targets", { params }),
        apiClient.get("/webhooks/status"),
      ]);
      setTargets(list.data.targets || []);
      setPendingCount(list.data.pending_count || 0);
      setStatus(st.data);
    } catch (err) {
      if (err.response?.status === 403) {
        setStatus((s) => ({ ...(s || {}), available: false }));
      } else {
        setError(apiErrorDetail(err, "Failed to load webhook targets"));
      }
    } finally {
      setLoading(false);
    }
  }, [usable, statusFilter]);

  useEffect(() => {
    fetchTargets();
  }, [fetchTargets]);

  useEffect(() => {
    if (!usable) return;
    apiClient.get("/webhooks/templates/presets").then((res) => setPresets(res.data || [])).catch(() => {});
    apiClient
      .get("/webhooks/topics")
      .then((res) => {
        const all = new Set([...(res.data?.known || []), ...(res.data?.seen || [])]);
        setTopics(Array.from(all).sort());
      })
      .catch(() => {});
  }, [usable]);

  const act = async (target, action, body) => {
    setError(null);
    try {
      await apiClient.post(`/webhooks/targets/${target.id}/${action}`, body || {});
      setNotice(`${target.name}: ${humanStatus(action)} done`);
      fetchTargets();
    } catch (err) {
      setError(apiErrorDetail(err, `Failed to ${action} target`));
    }
  };

  const remove = async (target) => {
    setError(null);
    try {
      await apiClient.delete(`/webhooks/targets/${target.id}`);
      setNotice(`${target.name} deleted`);
      fetchTargets();
    } catch (err) {
      setError(apiErrorDetail(err, "Failed to delete target"));
    }
  };

  const rotate = async (target) => {
    setError(null);
    try {
      const res = await apiClient.post(`/webhooks/targets/${target.id}/rotate-secret`, {});
      setSecret(res.data.signing_secret);
      setNotice(`Previous secret stays valid until ${formatTime(res.data.previous_valid_until)}`);
    } catch (err) {
      setError(apiErrorDetail(err, "Failed to rotate secret"));
    }
  };

  const sendTest = async (target) => {
    setError(null);
    try {
      const res = await apiClient.post(`/webhooks/targets/${target.id}/test`, {});
      setNotice(`Test event queued (delivery ${res.data.delivery_id})`);
    } catch (err) {
      setError(apiErrorDetail(err, "Failed to send test event"));
    }
  };

  const toggleExpand = async (target) => {
    if (expanded === target.id) {
      setExpanded(null);
      return;
    }
    setExpanded(target.id);
    if (details[target.id] === undefined) {
      try {
        const res = await apiClient.get(`/webhooks/targets/${target.id}`);
        setDetails((d) => ({ ...d, [target.id]: res.data }));
      } catch (err) {
        setDetails((d) => ({ ...d, [target.id]: null }));
      }
    }
    if (auditAvailable && history[target.id] === undefined) {
      try {
        const res = await apiClient.get(`/audit/resources/webhook_target/${target.id}`, { params: { page_size: 25 } });
        setHistory((h) => ({ ...h, [target.id]: res.data.records || [] }));
      } catch (err) {
        setHistory((h) => ({ ...h, [target.id]: [] }));
      }
    }
  };

  const onSaved = ({ target, secret: newSecret, repended }) => {
    setFormOpen(false);
    setEditing(null);
    if (newSecret) setSecret(newSecret);
    setNotice(
      repended
        ? `${target.name} saved and sent back for approval`
        : newSecret
        ? `${target.name} created and awaiting approval`
        : `${target.name} saved`
    );
    fetchTargets();
  };

  const openMenu = (e, target) => {
    e.stopPropagation();
    setMenu({ anchor: e.currentTarget, target });
  };
  const closeMenu = () => setMenu({ anchor: null, target: null });
  const menuTarget = menu.target;

  const header = (
    <TitleBox top="64px">
      <Box sx={{ display: "flex", alignItems: "center", gap: 2 }}>
        <WebhookIcon sx={{ fontSize: 32 }} />
        <Typography variant="headingXLarge">Webhooks</Typography>
        <Chip label="Enterprise" color="primary" size="small" />
        {usable && <HealthPill status={status} />}
      </Box>
      {usable && (
        <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
          <Button variant="outlined" startIcon={<ListAltIcon />} onClick={() => navigate("/admin/webhooks/deliveries")}>
            Deliveries
          </Button>
          <Tooltip title="Refresh">
            <IconButton onClick={fetchTargets} disabled={loading} aria-label="refresh">
              <RefreshIcon />
            </IconButton>
          </Tooltip>
          <Button
            variant="contained"
            startIcon={<AddIcon />}
            onClick={() => {
              setEditing(null);
              setFormOpen(true);
            }}
          >
            New target
          </Button>
        </Box>
      )}
    </TitleBox>
  );

  if (loading && status === null) {
    return (
      <Box>
        {header}
        <Box sx={{ display: "flex", justifyContent: "center", p: 5 }}>
          <CircularProgress />
        </Box>
      </Box>
    );
  }
  if (statusError) {
    return (
      <Box>
        {header}
        <Box sx={{ p: 3 }}>
          <Alert severity="error">{statusError}</Alert>
        </Box>
      </Box>
    );
  }
  if (!status?.available) {
    return (
      <Box>
        {header}
        <EnterpriseUpsell />
      </Box>
    );
  }
  if (!status.enabled) {
    return (
      <Box>
        {header}
        <DisabledNotice status={status} />
      </Box>
    );
  }

  return (
    <Box>
      {header}
      <Box sx={{ p: 3 }}>
        <BusWarning status={status} />
        {notice && (
          <Alert severity="success" sx={{ mb: 2 }} onClose={() => setNotice(null)}>
            {notice}
          </Alert>
        )}
        {error && (
          <Alert severity="error" sx={{ mb: 2 }} onClose={() => setError(null)}>
            {error}
          </Alert>
        )}

        <StyledPaper sx={{ p: 2, mb: 3, display: "flex", alignItems: "center", gap: 2, flexWrap: "wrap" }}>
          <Badge badgeContent={pendingCount} color="warning" data-testid="pending-badge">
            <Chip
              label="Awaiting approval"
              variant={statusFilter === "pending" ? "filled" : "outlined"}
              color="warning"
              onClick={() => setStatusFilter(statusFilter === "pending" ? "" : "pending")}
            />
          </Badge>
          <TextField
            select
            size="small"
            label="Status"
            value={statusFilter}
            onChange={(e) => setStatusFilter(e.target.value)}
            sx={{ minWidth: 180 }}
          >
            <MenuItem value="">All targets</MenuItem>
            {TARGET_STATUSES.map((s) => (
              <MenuItem key={s} value={s}>
                {humanStatus(s)}
              </MenuItem>
            ))}
          </TextField>
          <Typography variant="body2" color="text.secondary">
            {status.queue_depth} queued · {status.retrying} retrying · {status.dead_lettered} dead-lettered
          </Typography>
        </StyledPaper>

        <StyledPaper>
          <TableContainer>
            <Table size="small" aria-label="webhook targets">
              <TableHead>
                <TableRow>
                  <StyledTableHeaderCell sx={{ width: 40 }} />
                  <StyledTableHeaderCell>Name</StyledTableHeaderCell>
                  <StyledTableHeaderCell>URL</StyledTableHeaderCell>
                  <StyledTableHeaderCell>Status</StyledTableHeaderCell>
                  <StyledTableHeaderCell>Topics</StyledTableHeaderCell>
                  <StyledTableHeaderCell>Last success</StyledTableHeaderCell>
                  <StyledTableHeaderCell>Failures</StyledTableHeaderCell>
                  <StyledTableHeaderCell sx={{ width: 40 }} />
                </TableRow>
              </TableHead>
              <TableBody>
                {loading ? (
                  <TableRow>
                    <StyledTableCell colSpan={8} align="center">
                      <CircularProgress size={24} />
                    </StyledTableCell>
                  </TableRow>
                ) : targets.length === 0 ? (
                  <TableRow>
                    <StyledTableCell colSpan={8} align="center">
                      <Typography color="text.secondary" sx={{ py: 3 }}>
                        No webhook targets yet. Create one; deliveries start after an administrator approves it.
                      </Typography>
                    </StyledTableCell>
                  </TableRow>
                ) : (
                  targets.map((t) => (
                    <React.Fragment key={t.id}>
                      <StyledTableRow
                        hover
                        sx={{ cursor: "pointer" }}
                        onClick={() => toggleExpand(t)}
                        data-testid={`target-row-${t.id}`}
                      >
                        <StyledTableCell>
                          <IconButton size="small" aria-label="expand">
                            {expanded === t.id ? <ExpandLessIcon /> : <ExpandMoreIcon />}
                          </IconButton>
                        </StyledTableCell>
                        <StyledTableCell>{t.name}</StyledTableCell>
                        <StyledTableCell sx={{ fontFamily: "monospace", wordBreak: "break-all" }}>{t.url}</StyledTableCell>
                        <StyledTableCell>
                          <Box sx={{ display: "flex", gap: 0.5 }}>
                            <StatusChip status={t.status} kind="target" />
                            {t.paused && <Chip label="paused" size="small" />}
                          </Box>
                        </StyledTableCell>
                        <StyledTableCell>
                          <Box sx={{ display: "flex", gap: 0.5, flexWrap: "wrap" }}>
                            {(t.topic_filters || []).map((f) => (
                              <Chip key={f} label={f} size="small" variant="outlined" />
                            ))}
                          </Box>
                        </StyledTableCell>
                        <StyledTableCell sx={{ whiteSpace: "nowrap" }}>{formatTime(t.last_success_at) || "—"}</StyledTableCell>
                        <StyledTableCell>
                          {t.consecutive_failures > 0 ? (
                            <Chip label={`${t.consecutive_failures} in a row`} size="small" color="error" />
                          ) : (
                            "—"
                          )}
                        </StyledTableCell>
                        <StyledTableCell>
                          <IconButton size="small" aria-label={`actions for ${t.name}`} onClick={(e) => openMenu(e, t)}>
                            <MoreVertIcon />
                          </IconButton>
                        </StyledTableCell>
                      </StyledTableRow>
                      <TableRow>
                        <StyledTableCell colSpan={8} sx={{ p: 0, border: 0 }}>
                          <Collapse in={expanded === t.id} timeout="auto" unmountOnExit>
                            <TargetDetails target={t} detail={details[t.id]} history={history[t.id]} />
                          </Collapse>
                        </StyledTableCell>
                      </TableRow>
                    </React.Fragment>
                  ))
                )}
              </TableBody>
            </Table>
          </TableContainer>
        </StyledPaper>
      </Box>

      <Menu anchorEl={menu.anchor} open={Boolean(menu.anchor)} onClose={closeMenu}>
        {menuTarget?.status === "pending" && (
          <MenuItem
            onClick={() => {
              setReason({ action: "approve", target: menuTarget });
              closeMenu();
            }}
          >
            Approve
          </MenuItem>
        )}
        {menuTarget?.status === "pending" && (
          <MenuItem
            onClick={() => {
              setReason({ action: "reject", target: menuTarget });
              closeMenu();
            }}
          >
            Reject
          </MenuItem>
        )}
        {menuTarget?.status === "approved" && (
          <MenuItem
            onClick={() => {
              act(menuTarget, menuTarget.paused ? "resume" : "pause");
              closeMenu();
            }}
          >
            {menuTarget.paused ? "Resume" : "Pause"}
          </MenuItem>
        )}
        {menuTarget?.status === "approved" && (
          <MenuItem
            onClick={() => {
              sendTest(menuTarget);
              closeMenu();
            }}
          >
            Send test event
          </MenuItem>
        )}
        {menuTarget?.status === "approved" && (
          <MenuItem
            onClick={() => {
              setReason({ action: "revoke", target: menuTarget });
              closeMenu();
            }}
          >
            Revoke
          </MenuItem>
        )}
        <MenuItem
          onClick={() => {
            rotate(menuTarget);
            closeMenu();
          }}
        >
          Rotate signing secret
        </MenuItem>
        <MenuItem
          onClick={() => {
            setEditing(menuTarget);
            setFormOpen(true);
            closeMenu();
          }}
        >
          Edit
        </MenuItem>
        <MenuItem
          onClick={() => {
            if (window.confirm(`Delete ${menuTarget.name}? Queued deliveries are cancelled; the log is kept.`)) {
              remove(menuTarget);
            }
            closeMenu();
          }}
        >
          Delete
        </MenuItem>
      </Menu>

      <TargetForm
        open={formOpen}
        target={editing}
        presets={presets}
        topics={topics}
        onClose={() => {
          setFormOpen(false);
          setEditing(null);
        }}
        onSaved={onSaved}
      />
      <SecretDialog secret={secret} onClose={() => setSecret(null)} />
      <ReasonDialog
        action={reason.action}
        target={reason.target}
        onClose={() => setReason({ action: null, target: null })}
        onConfirm={(text) => {
          const { action, target } = reason;
          setReason({ action: null, target: null });
          act(target, action, action === "approve" ? { note: text } : { reason: text });
        }}
      />
    </Box>
  );
};

export default Webhooks;
