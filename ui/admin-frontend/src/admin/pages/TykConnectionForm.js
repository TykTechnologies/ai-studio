import React, { useCallback, useEffect, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import {
  Alert,
  Box,
  Checkbox,
  Chip,
  CircularProgress,
  FormControlLabel,
  Grid,
  MenuItem,
  TextField,
  Typography,
} from "@mui/material";
import ArrowBackIcon from "@mui/icons-material/ArrowBack";
import apiClient from "../utils/apiClient";
import { usePermissions } from "../context/PermissionsContext";
import { P } from "../rbac/permissions";
import Section from "../components/common/Section";
import FeedbackSnackbar, { useFeedbackSnackbar } from "../components/common/FeedbackSnackbar";
import {
  TitleBox,
  ContentBox,
  PrimaryButton,
  SecondaryLinkButton,
  SecondaryOutlineButton,
} from "../styles/sharedStyles";
import { formatTime, apiErrorDetail } from "./webhookShared";
import {
  CONNECTION_MODES,
  MODE_LABELS,
  MODE_HELP,
  CapabilityChips,
  TykUpsell,
  TykDisabledNotice,
} from "./tykShared";

export const emptyForm = {
  name: "",
  description: "",
  dashboard_url: "",
  dashboard_access_token: "",
  gateway_base_url: "",
  org_id: "",
  declared_mode: "catalogue",
  sync_interval_seconds: 300,
  allow_internal_host: false,
  template_id: "",
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

/**
 * formToInput turns the form state into the create/patch body. Tokens are
 * only sent when the user typed one; an empty token on edit keeps the stored
 * one. Exported so the test can assert the exact payload.
 */
export const formToInput = (form, editing) => {
  const input = {
    name: form.name,
    description: form.description,
    dashboard_url: form.dashboard_url,
    gateway_base_url: form.gateway_base_url,
    org_id: form.org_id,
    declared_mode: form.declared_mode,
    sync_interval_seconds: Number(form.sync_interval_seconds) || 0,
    allow_internal_host: Boolean(form.allow_internal_host),
    template_id: (form.template_id || "").trim(),
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

export const connectionToForm = (connection) => ({
  ...emptyForm,
  name: connection.name || "",
  description: connection.description || "",
  dashboard_url: connection.dashboard_url || "",
  gateway_base_url: connection.gateway_base_url || "",
  org_id: connection.org_id || "",
  declared_mode: connection.declared_mode || "catalogue",
  sync_interval_seconds: connection.sync_interval_seconds || 300,
  allow_internal_host: Boolean(connection.allow_internal_host),
  template_id: connection.template_id || "",
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

// DataPlanes lists what MDCB reported, one line per data plane.
const DataPlanes = ({ planes }) =>
  (planes || []).length > 0 ? (
    <Box sx={{ mt: 1 }}>
      <Typography variant="subtitle2">Data planes (MDCB)</Typography>
      {planes.map((dp) => (
        <Typography key={dp.group_id} variant="body2">
          {dp.group_id}: {dp.node_count} node(s), tags {(dp.tags || []).join(", ") || "none"}
          {dp.healthy ? "" : " (unhealthy)"}
        </Typography>
      ))}
    </Box>
  ) : null;

// ProbePanel shows what a probe learned about a Dashboard.
export const ProbePanel = ({ result }) => {
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
      <DataPlanes planes={result.data_planes} />
    </Box>
  );
};

/**
 * TykConnectionForm creates or edits a connection on its own page, like the
 * LLM provider and tool catalog forms. Tokens are never loaded back from the
 * server; leaving the field empty keeps the stored token.
 */
const TykConnectionForm = () => {
  const navigate = useNavigate();
  const { id } = useParams();
  const editing = Boolean(id);
  const { can } = usePermissions();
  const canExecute = can(P.TYK_CONNECTIONS_EXECUTE);
  const { notify, snackbarProps } = useFeedbackSnackbar();

  const [status, setStatus] = useState(null);
  const [connection, setConnection] = useState(null);
  const [form, setForm] = useState(emptyForm);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [probing, setProbing] = useState(false);
  const [probe, setProbe] = useState(null);
  const [error, setError] = useState(null);

  const loadConnection = useCallback(async () => {
    const res = await apiClient.get(`/tyk-connections/${id}`);
    setConnection(res.data);
    setForm(connectionToForm(res.data));
  }, [id]);

  useEffect(() => {
    const load = async () => {
      setLoading(true);
      setError(null);
      try {
        const st = await apiClient.get("/tyk-mcp/status");
        setStatus(st.data);
        if (st.data?.available && st.data?.enabled && id) {
          await loadConnection();
        }
      } catch (err) {
        setError(apiErrorDetail(err, "Failed to load the connection"));
      } finally {
        setLoading(false);
      }
    };
    load();
  }, [id, loadConnection]);

  const set = (field) => (e) => setForm((f) => ({ ...f, [field]: e.target.value }));
  const setBool = (field) => (e) => setForm((f) => ({ ...f, [field]: e.target.checked }));

  const updateBaseURL = (i, field, value) => {
    setForm((f) => ({
      ...f,
      gateway_base_urls: f.gateway_base_urls.map((r, idx) => (idx === i ? { ...r, [field]: value } : r)),
    }));
  };

  const backToList = () => navigate("/admin/tyk-connections");

  const runProbe = async () => {
    setError(null);
    if (editing && !form.dashboard_access_token) {
      setError("Enter the access token to test unsaved settings, or use Probe now to check the saved connection.");
      return;
    }
    setProbing(true);
    try {
      const res = await apiClient.post("/tyk-connections/probe", formToInput(form, editing));
      setProbe(res.data);
    } catch (err) {
      setError(apiErrorDetail(err, "Probe failed"));
    } finally {
      setProbing(false);
    }
  };

  const probeSaved = async () => {
    setError(null);
    setProbing(true);
    try {
      const res = await apiClient.post(`/tyk-connections/${id}/probe`, {});
      setProbe(res.data);
      await loadConnection();
      notify("Probe complete");
    } catch (err) {
      setError(apiErrorDetail(err, "Probe failed"));
    } finally {
      setProbing(false);
    }
  };

  const handleSubmit = async (e) => {
    e.preventDefault();
    setSaving(true);
    setError(null);
    try {
      const input = formToInput(form, editing);
      if (editing) {
        await apiClient.patch(`/tyk-connections/${id}`, { ...input, lock_version: connection?.lock_version ?? 0 });
        navigate("/admin/tyk-connections", {
          state: { snackbar: { message: "Connection saved", severity: "success" } },
        });
      } else {
        await apiClient.post("/tyk-connections", input);
        navigate("/admin/tyk-connections", {
          state: { snackbar: { message: "Connection created; activate it to start syncing", severity: "success" } },
        });
      }
    } catch (err) {
      setError(apiErrorDetail(err, "Save failed"));
    } finally {
      setSaving(false);
    }
  };

  if (loading) {
    return (
      <Box sx={{ p: 3, display: "flex", justifyContent: "center" }}>
        <CircularProgress />
      </Box>
    );
  }
  if (status && !status.available) return <TykUpsell />;
  if (status && !status.enabled) return <TykDisabledNotice status={status} />;

  const canSave = form.name && form.dashboard_url && (editing || form.dashboard_access_token);

  return (
    <>
      <TitleBox top="64px">
        <Typography variant="headingXLarge">{editing ? "Edit connection" : "Connect a Tyk Dashboard"}</Typography>
        <SecondaryLinkButton startIcon={<ArrowBackIcon />} onClick={backToList} color="inherit">
          Back to connections
        </SecondaryLinkButton>
      </TitleBox>
      <Box sx={{ p: 3 }}>
        <Typography variant="bodyLargeDefault" color="text.defaultSubdued">
          A connection is one Tyk Dashboard, one dedicated Dashboard user and one organisation. Its trust mode caps
          what AI Studio may do there: import MCP proxies, mint keys for Apps, or create proxies and policies.
        </Typography>
      </Box>
      <ContentBox sx={{ pt: 0 }}>
        {error && (
          <Alert severity="error" sx={{ mb: 2 }} onClose={() => setError(null)} data-testid="form-error">
            {error}
          </Alert>
        )}
        {connection?.degraded && (
          <Alert severity="error" sx={{ mb: 2 }} data-testid="degraded-alert">
            {connection.degraded_reason}
          </Alert>
        )}

        {editing && connection && (
          <Section
            title="Capabilities"
            description={`Last probed ${connection.last_probe_at ? formatTime(connection.last_probe_at) : "never"}${
              connection.org_id ? ` · organisation ${connection.org_id}` : ""
            }${connection.activated_by_email ? ` · activated by ${connection.activated_by_email}` : ""}`}
            actions={
              canExecute ? (
                <SecondaryOutlineButton size="small" onClick={probeSaved} disabled={probing} data-testid="probe-now">
                  {probing ? "Probing…" : "Probe now"}
                </SecondaryOutlineButton>
              ) : null
            }
            data-testid="capabilities-section"
          >
            <CapabilityChips capabilities={connection.capabilities} />
            <DataPlanes planes={connection.data_planes} />
            {(connection.gateway_tags || []).length > 0 && (
              <Box sx={{ mt: 1, display: "flex", gap: 0.5, flexWrap: "wrap" }}>
                {connection.gateway_tags.map((t) => (
                  <Chip key={t} size="small" label={t} />
                ))}
              </Box>
            )}
          </Section>
        )}

        <Box component="form" onSubmit={handleSubmit} noValidate>
          <Section title="Connection" description="Where the Dashboard is and how much AI Studio may do there.">
            <Grid container spacing={3}>
              <Grid item xs={12} sm={6}>
                <TextField fullWidth label="Name" name="name" value={form.name} onChange={set("name")} required inputProps={{ "data-testid": "name-input" }} />
              </Grid>
              <Grid item xs={12} sm={6}>
                <TextField
                  select
                  fullWidth
                  label="Trust mode"
                  name="declared_mode"
                  value={form.declared_mode}
                  onChange={set("declared_mode")}
                  helperText={MODE_HELP[form.declared_mode]}
                  inputProps={{ "data-testid": "mode-input" }}
                >
                  {CONNECTION_MODES.map((m) => (
                    <MenuItem key={m} value={m}>
                      {MODE_LABELS[m]}
                    </MenuItem>
                  ))}
                </TextField>
              </Grid>
              <Grid item xs={12}>
                <TextField fullWidth label="Description" name="description" value={form.description} onChange={set("description")} />
              </Grid>
              <Grid item xs={12} sm={8}>
                <TextField
                  fullWidth
                  label="Dashboard URL"
                  name="dashboard_url"
                  placeholder="https://dashboard.example.com"
                  value={form.dashboard_url}
                  onChange={set("dashboard_url")}
                  required
                  inputProps={{ "data-testid": "dashboard-url-input" }}
                />
              </Grid>
              <Grid item xs={12} sm={4}>
                <TextField fullWidth label="Organisation ID (optional)" name="org_id" value={form.org_id} onChange={set("org_id")} />
              </Grid>
              <Grid item xs={12}>
                <TextField
                  fullWidth
                  type="password"
                  label={editing ? "Dashboard access token (leave empty to keep)" : "Dashboard access token"}
                  name="dashboard_access_token"
                  helperText="The API access key of a dedicated Dashboard user. Stored encrypted, never shown again."
                  value={form.dashboard_access_token}
                  onChange={set("dashboard_access_token")}
                  required={!editing}
                  autoComplete="new-password"
                  inputProps={{ "data-testid": "token-input" }}
                />
              </Grid>
              <Grid item xs={12} sm={8}>
                <TextField
                  fullWidth
                  label="Public gateway base URL"
                  name="gateway_base_url"
                  placeholder="https://gateway.example.com"
                  helperText="The URL clients use to reach MCP proxies; shown in connection instructions."
                  value={form.gateway_base_url}
                  onChange={set("gateway_base_url")}
                />
              </Grid>
              <Grid item xs={12} sm={4}>
                <TextField
                  fullWidth
                  type="number"
                  label="Sync interval (seconds)"
                  name="sync_interval_seconds"
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
            </Grid>
          </Section>

          <Section title="Governance" description="Defaults the API team sets for every proxy AI Studio publishes, and how imported servers reach the portal.">
            <Grid container spacing={3}>
              <Grid item xs={12}>
                <TextField
                  fullWidth
                  label="API template ID"
                  name="template_id"
                  value={form.template_id}
                  onChange={set("template_id")}
                  helperText="Optional. The id of a Tyk Dashboard API template (Assets → API templates). Its defaults, such as traffic logs, caching, middleware and tags, are merged into every MCP proxy AI Studio creates on this connection; the registration's own values win on conflicts."
                  inputProps={{ "data-testid": "template-id-input" }}
                />
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
                  type="number"
                  label="Default privacy score"
                  name="default_privacy_score"
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
            </Grid>
          </Section>

          <Section title="Keys" description="Applied to every Tyk key AI Studio mints on this connection.">
            <Grid container spacing={3}>
              <Grid item xs={12} sm={6}>
                <TextField fullWidth label="Key alias prefix" name="alias_prefix" value={form.alias_prefix} onChange={set("alias_prefix")} />
              </Grid>
              <Grid item xs={12} sm={6}>
                <TextField
                  fullWidth
                  type="number"
                  label="Key expiry (seconds, 0 = never)"
                  name="expires_in_seconds"
                  value={form.expires_in_seconds}
                  onChange={set("expires_in_seconds")}
                />
              </Grid>
            </Grid>
          </Section>

          <Section
            title="Gateway segmentation"
            description="Optional. In sharded deployments a proxy is loaded only by gateways whose tags match. Configure MDCB to discover data planes, or list the tags by hand."
          >
            <Grid container spacing={3}>
              <Grid item xs={12} sm={7}>
                <TextField fullWidth label="MDCB URL" name="mdcb_url" placeholder="https://mdcb.example.com" value={form.mdcb_url} onChange={set("mdcb_url")} />
              </Grid>
              <Grid item xs={12} sm={5}>
                <TextField
                  fullWidth
                  type="password"
                  label={editing ? "MDCB secret (leave empty to keep)" : "MDCB secret"}
                  name="mdcb_access_token"
                  value={form.mdcb_access_token}
                  onChange={set("mdcb_access_token")}
                  autoComplete="new-password"
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
                  label="Known gateway tags (comma separated)"
                  name="known_gateway_tags"
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
                    <TextField label="Tag" value={row.tag} onChange={(e) => updateBaseURL(i, "tag", e.target.value)} sx={{ flex: 1 }} />
                    <TextField label="URL" value={row.url} onChange={(e) => updateBaseURL(i, "url", e.target.value)} sx={{ flex: 2 }} />
                    <SecondaryOutlineButton
                      onClick={() => setForm((f) => ({ ...f, gateway_base_urls: f.gateway_base_urls.filter((_, idx) => idx !== i) }))}
                    >
                      Remove
                    </SecondaryOutlineButton>
                  </Box>
                ))}
                <SecondaryOutlineButton onClick={() => setForm((f) => ({ ...f, gateway_base_urls: [...f.gateway_base_urls, { tag: "", url: "" }] }))}>
                  Add tag URL
                </SecondaryOutlineButton>
              </Grid>
            </Grid>
          </Section>

          <ProbePanel result={probe} />

          <Box display="flex" gap={2} sx={{ mt: 3 }}>
            <SecondaryOutlineButton onClick={backToList} disabled={saving}>
              Cancel
            </SecondaryOutlineButton>
            <SecondaryOutlineButton onClick={runProbe} disabled={probing || !form.dashboard_url} data-testid="probe-button">
              {probing ? "Testing…" : "Test connection"}
            </SecondaryOutlineButton>
            <PrimaryButton type="submit" variant="contained" color="primary" disabled={saving || !canSave} data-testid="save-button">
              {saving ? <CircularProgress size={24} /> : editing ? "Save connection" : "Create connection"}
            </PrimaryButton>
          </Box>
        </Box>
      </ContentBox>
      <FeedbackSnackbar {...snackbarProps} />
    </>
  );
};

export default TykConnectionForm;
