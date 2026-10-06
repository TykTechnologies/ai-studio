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
  CONNECTION_KINDS,
  KIND_LABELS,
  DISCOVERY_LABELS,
  DISCOVERY_HELP,
  modeHelp,
  productName,
  CapabilityChips,
  TykUpsell,
  TykDisabledNotice,
  emptyForm,
  formToInput,
  connectionToForm,
  DataPlanes,
  ProbePanel,
} from "./tykShared";
import TykGatewayNodes from "./TykGatewayNodes";

// The form pieces the Tools import wizard shares live in tykShared; they
// stay exported from here for existing imports and tests.
export { emptyForm, formToInput, connectionToForm, ProbePanel };

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
      setError(`Enter the ${form.kind === "gateway" ? "gateway secret" : "access token"} to test unsaved settings, or use Probe now to check the saved connection.`);
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
  // A Tyk Gateway connection talks to open-source gateways directly: no
  // Dashboard, no policies, templates or segmentation; MCP proxies and keys only.
  const isGateway = form.kind === "gateway";
  const what = productName(form.kind);
  // The application AI Studio is embedded in provides this connection: its
  // Dashboard, organisation, mode, gateway URL and key are set there.
  const hostManaged = Boolean(connection?.host_managed);

  return (
    <>
      <TitleBox top="var(--studio-header-height)">
        <Typography variant="headingXLarge">{editing ? "Edit connection" : isGateway ? "Connect Tyk Gateways" : "Connect a Tyk Dashboard"}</Typography>
        <SecondaryLinkButton startIcon={<ArrowBackIcon />} onClick={backToList} color="inherit">
          Back to connections
        </SecondaryLinkButton>
      </TitleBox>
      <Box sx={{ p: 3 }}>
        <Typography variant="bodyLargeDefault" color="text.defaultSubdued">
          {isGateway
            ? "A Tyk Gateway connection is one cluster of open-source Tyk Gateways sharing a Redis, managed through each node's Gateway API for MCP proxies and keys only. Its trust mode caps what AI Studio may do there: import MCP proxies, mint keys for Apps, or create proxies on every node."
            : "A connection is one Tyk Dashboard, one dedicated Dashboard user and one organisation. Its trust mode caps what AI Studio may do there: import MCP proxies, mint keys for Apps, or create proxies and policies."}
        </Typography>
      </Box>
      <ContentBox sx={{ pt: 0 }}>
        {error && (
          <Alert severity="error" sx={{ mb: 2 }} onClose={() => setError(null)} data-testid="form-error">
            {error}
          </Alert>
        )}
        {hostManaged && (
          <Alert severity="info" sx={{ mb: 2 }} data-testid="host-managed-alert">
            This connection is provided by the application AI Studio is embedded in. Its Dashboard URL, organisation,
            trust mode, gateway URL and access key are managed there.
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
            {!isGateway && <DataPlanes planes={connection.data_planes} />}
            {(connection.gateway_tags || []).length > 0 && (
              <Box sx={{ mt: 1, display: "flex", gap: 0.5, flexWrap: "wrap" }}>
                {connection.gateway_tags.map((t) => (
                  <Chip key={t} size="small" label={t} />
                ))}
              </Box>
            )}
          </Section>
        )}

        {editing && connection && isGateway && (
          <Section
            title="Gateway nodes"
            description="Each node keeps its MCP proxies on its own disk. Every sync writes the proxies AI Studio owns to every node it finds and records which nodes serve them."
            data-testid="gateway-nodes-section"
          >
            <TykGatewayNodes connectionId={id} refreshKey={connection.last_sync_at} />
          </Section>
        )}

        <Box component="form" onSubmit={handleSubmit} noValidate>
          <Section title="Connection" description={`Where the ${what} is and how much AI Studio may do there.`}>
            <Grid container spacing={3}>
              {!editing && (
                <Grid item xs={12}>
                  <TextField
                    select
                    fullWidth
                    label="Connect to"
                    name="kind"
                    value={form.kind}
                    onChange={set("kind")}
                    helperText={
                      isGateway
                        ? "Open-source Tyk Gateways without a Dashboard. AI Studio manages MCP proxies and keys on them, nothing else."
                        : "A Tyk Dashboard: MCP proxies, policies, templates and keys across its gateways."
                    }
                    inputProps={{ "data-testid": "kind-input" }}
                  >
                    {CONNECTION_KINDS.map((k) => (
                      <MenuItem key={k} value={k}>
                        {KIND_LABELS[k]}
                      </MenuItem>
                    ))}
                  </TextField>
                </Grid>
              )}
              {editing && (
                <Grid item xs={12}>
                  <Chip label={KIND_LABELS[form.kind] || form.kind} size="small" data-testid="kind-chip" />
                </Grid>
              )}
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
                  helperText={modeHelp(form.kind, form.declared_mode)}
                  disabled={hostManaged}
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
                  label={isGateway ? "Gateway API URL" : "Dashboard URL"}
                  name="dashboard_url"
                  placeholder={isGateway ? "http://tyk-gateway:8080" : "https://dashboard.example.com"}
                  helperText={
                    isGateway
                      ? "The Gateway API of a node (the /tyk/ endpoints). Prefer a control_api_port that is not reachable from the internet."
                      : undefined
                  }
                  value={form.dashboard_url}
                  onChange={set("dashboard_url")}
                  required
                  disabled={hostManaged}
                  inputProps={{ "data-testid": "dashboard-url-input" }}
                />
              </Grid>
              <Grid item xs={12} sm={4}>
                <TextField
                  fullWidth
                  label="Organisation ID (optional)"
                  name="org_id"
                  value={form.org_id}
                  onChange={set("org_id")}
                  disabled={hostManaged}
                />
              </Grid>
              {!hostManaged && (
                <Grid item xs={12}>
                  <TextField
                    fullWidth
                    type="password"
                    label={
                      isGateway
                        ? editing
                          ? "Gateway secret (leave empty to keep)"
                          : "Gateway secret"
                        : editing
                          ? "Dashboard access token (leave empty to keep)"
                          : "Dashboard access token"
                    }
                    name="dashboard_access_token"
                    helperText={
                      isGateway
                        ? "The gateways' secret (sent as X-Tyk-Authorization). It grants full control of the gateways; AI Studio only uses it for MCP proxies and keys. Stored encrypted, never shown again."
                        : "The API access key of a dedicated Dashboard user. Stored encrypted, never shown again."
                    }
                    value={form.dashboard_access_token}
                    onChange={set("dashboard_access_token")}
                    required={!editing}
                    autoComplete="new-password"
                    inputProps={{ "data-testid": "token-input" }}
                  />
                </Grid>
              )}
              <Grid item xs={12} sm={8}>
                <TextField
                  fullWidth
                  label="Public gateway base URL"
                  name="gateway_base_url"
                  placeholder="https://gateway.example.com"
                  helperText="The URL clients use to reach MCP proxies; shown in connection instructions."
                  value={form.gateway_base_url}
                  onChange={set("gateway_base_url")}
                  disabled={hostManaged}
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
                    control={<Checkbox checked={form.allow_internal_host} onChange={setBool("allow_internal_host")} disabled={hostManaged} />}
                    label={`Allow the ${what} host${isGateway ? "s" : ""} to be on an internal network address`}
                  />
                </Grid>
              )}
            </Grid>
          </Section>

          {isGateway && (
            <Section
              title="Nodes"
              description="An open-source gateway keeps MCP proxies on its own disk and has no API for the whole cluster, so AI Studio needs to know every node. Keys live in the Redis the nodes share and need no copying."
              data-testid="gateway-discovery-section"
            >
              <Grid container spacing={3}>
                <Grid item xs={12} sm={6}>
                  <TextField
                    select
                    fullWidth
                    label="Find nodes by"
                    name="gateway_discovery"
                    value={form.gateway_discovery}
                    onChange={set("gateway_discovery")}
                    helperText={DISCOVERY_HELP[form.gateway_discovery]}
                    inputProps={{ "data-testid": "discovery-input" }}
                  >
                    {Object.keys(DISCOVERY_LABELS).map((d) => (
                      <MenuItem key={d} value={d}>
                        {DISCOVERY_LABELS[d]}
                      </MenuItem>
                    ))}
                  </TextField>
                </Grid>
                <Grid item xs={12} sm={6}>
                  <FormControlLabel
                    control={<Checkbox checked={form.gateway_shared_storage} onChange={setBool("gateway_shared_storage")} />}
                    label="The nodes share app_path (shared volume)"
                  />
                  <Typography variant="body2" color="text.secondary">
                    AI Studio then writes each proxy through one node and a group reload loads it everywhere. New nodes
                    serve the proxies from their first start.
                  </Typography>
                </Grid>
                {form.gateway_discovery === "static" && (
                  <Grid item xs={12}>
                    <TextField
                      fullWidth
                      multiline
                      minRows={3}
                      label="Other node URLs (one per line)"
                      name="gateway_node_urls"
                      value={form.gateway_node_urls}
                      onChange={set("gateway_node_urls")}
                      placeholder={"http://tyk-gateway-2:8080\nhttp://tyk-gateway-3:8080"}
                      inputProps={{ "data-testid": "node-urls-input" }}
                    />
                  </Grid>
                )}
              </Grid>
            </Section>
          )}

          <Section title="Governance" description="Defaults the API team sets for every proxy AI Studio publishes, and how imported servers reach the portal.">
            <Grid container spacing={3}>
              {!isGateway && (
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
              )}
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

          {!isGateway && (
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
          )}

          <ProbePanel result={probe} kind={form.kind} />

          <Box display="flex" gap={2} sx={{ mt: 3 }}>
            <SecondaryOutlineButton onClick={backToList} disabled={saving}>
              Cancel
            </SecondaryOutlineButton>
            <SecondaryOutlineButton onClick={runProbe} disabled={probing || !form.dashboard_url || hostManaged} data-testid="probe-button">
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
