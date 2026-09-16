import React, { useCallback, useEffect, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import {
  Alert,
  Autocomplete,
  Box,
  Checkbox,
  Chip,
  CircularProgress,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  FormControlLabel,
  Grid,
  MenuItem,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
  TextField,
  Typography,
  Link,
} from "@mui/material";
import ArrowBackIcon from "@mui/icons-material/ArrowBack";
import apiClient from "../utils/apiClient";
import Can from "../components/rbac/Can";
import PublishSwitch from "../components/rbac/PublishSwitch";
import Section from "../components/common/Section";
import ConfirmationDialog from "../components/common/ConfirmationDialog";
import FeedbackSnackbar, { useFeedbackSnackbar } from "../components/common/FeedbackSnackbar";
import PrivacyLevelChip from "../components/common/privacy/PrivacyLevelChip";
import RelationshipPicker from "../components/common/relationship-picker";
import { P } from "../rbac/permissions";
import {
  TitleBox,
  ContentBox,
  FieldLabel,
  FieldValue,
  PrimaryButton,
  SecondaryLinkButton,
  SecondaryOutlineButton,
  DangerOutlineButton,
} from "../styles/sharedStyles";
import { apiErrorDetail, formatTime, preStyle } from "./webhookShared";
import { AUTH_MODE_LABELS, DashboardStateChip, KindChip } from "./MCPServers";

// Tool catalogues come back as JSON:API rows; the picker wants {id, name}.
const catalogueOptions = (data) =>
  (Array.isArray(data) ? data : data?.data || []).map((c) => ({
    id: Number(c.id),
    name: c.attributes?.name ?? c.name ?? `Catalog ${c.id}`,
  }));

// How a portal user reaches a server AI Studio does not broker keys for.
export const directAccessWording = (authMode) => {
  switch (authMode) {
    case "keyless":
      return "no credential";
    case "oauth21":
    case "oauth_tyk":
    case "oauth_external":
      return "an OAuth token from the advertised authorization server";
    case "jwt":
      return "a JWT issued by the configured identity provider";
    case "mtls":
      return "a client certificate";
    default:
      return "their own credential";
  }
};

// PolicyCreator writes a partitioned Studio-managed policy to the Dashboard
// (full-mode connections) and pins it to this server in the same call.
const PolicyCreator = ({ open, server, onClose, onCreated, onError }) => {
  const blank = { kind: "access", name: "", rate: "", per: "60", quota_max: "", quota_renewal_rate: "3600", key_expires_in: "" };
  const [form, setForm] = useState(blank);
  const [busy, setBusy] = useState(false);
  const set = (k) => (e) => setForm((f) => ({ ...f, [k]: e.target.value }));
  const num = (v) => (v === "" ? 0 : Number(v));
  const submit = async () => {
    setBusy(true);
    try {
      const body = { kind: form.kind, name: form.name, server_id: server.id, pin: true };
      if (form.kind === "consumption") {
        body.rate = num(form.rate);
        body.per = num(form.per);
        body.quota_max = num(form.quota_max);
        body.quota_renewal_rate = num(form.quota_renewal_rate);
        body.key_expires_in = num(form.key_expires_in);
      }
      const res = await apiClient.post(`/tyk-connections/${server.connection_id}/policies`, body);
      onCreated(res.data);
      setForm(blank);
    } catch (err) {
      onError(apiErrorDetail(err, "Creating the policy failed"));
    } finally {
      setBusy(false);
    }
  };
  return (
    <Dialog open={open} onClose={onClose} fullWidth maxWidth="sm" data-testid="policy-creator">
      <DialogTitle>Create a Tyk policy</DialogTitle>
      <DialogContent>
        <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
          Partitioned policies only: an access policy grants this proxy, a consumption policy carries limits. Both are tagged
          studio-managed on the Dashboard and pinned to this server.
        </Typography>
        <Grid container spacing={2}>
          <Grid item xs={12} md={5}>
            <TextField select fullWidth label="Kind" value={form.kind} onChange={set("kind")} inputProps={{ "data-testid": "policy-kind" }}>
              <MenuItem value="access">Access (ACL for this proxy)</MenuItem>
              <MenuItem value="consumption">Consumption (rate limit / quota)</MenuItem>
            </TextField>
          </Grid>
          <Grid item xs={12} md={7}>
            <TextField fullWidth label="Name" value={form.name} onChange={set("name")} inputProps={{ "data-testid": "policy-name" }} />
          </Grid>
          {form.kind === "consumption" && (
            <>
              <Grid item xs={6} md={3}>
                <TextField fullWidth type="number" label="Rate" value={form.rate} onChange={set("rate")} inputProps={{ "data-testid": "policy-rate" }} />
              </Grid>
              <Grid item xs={6} md={3}>
                <TextField fullWidth type="number" label="Per (seconds)" value={form.per} onChange={set("per")} />
              </Grid>
              <Grid item xs={6} md={3}>
                <TextField fullWidth type="number" label="Quota max" value={form.quota_max} onChange={set("quota_max")} helperText="-1 unlimited" />
              </Grid>
              <Grid item xs={6} md={3}>
                <TextField fullWidth type="number" label="Quota renews (s)" value={form.quota_renewal_rate} onChange={set("quota_renewal_rate")} />
              </Grid>
              <Grid item xs={12} md={6}>
                <TextField fullWidth type="number" label="Key expires in (seconds, 0 = never)" value={form.key_expires_in} onChange={set("key_expires_in")} />
              </Grid>
            </>
          )}
        </Grid>
      </DialogContent>
      <DialogActions>
        <SecondaryOutlineButton onClick={onClose}>Cancel</SecondaryOutlineButton>
        <PrimaryButton variant="contained" onClick={submit} disabled={busy || !form.name.trim()} data-testid="policy-create">
          Create and pin
        </PrimaryButton>
      </DialogActions>
    </Dialog>
  );
};

// DefinitionEditor edits the masked definition and pushes it to the
// Dashboard. Masked values are restored server-side from the live document.
const DefinitionEditor = ({ server, onPushed, onError, onNotice }) => {
  const [text, setText] = useState("");
  const [confirmOrigin, setConfirmOrigin] = useState(false);
  const [busy, setBusy] = useState(false);
  const [preview, setPreview] = useState(null);
  useEffect(() => {
    setText(server.definition ? JSON.stringify(JSON.parse(server.definition), null, 2) : "");
    setPreview(null);
  }, [server]);
  const parsed = () => {
    try {
      return JSON.parse(text);
    } catch {
      onError("The definition is not valid JSON.");
      return null;
    }
  };
  const run = async (dryRun) => {
    const doc = parsed();
    if (!doc) return;
    setBusy(true);
    try {
      const res = await apiClient.post(`/mcp-servers/${server.id}/push${dryRun ? "?dry_run=1" : ""}`, {
        definition: doc,
        expected_hash: server.definition_hash,
        confirm_dashboard_origin: confirmOrigin,
      });
      if (dryRun) {
        setPreview(res.data);
        onNotice(res.data.dashboard_validated ? "The Dashboard accepted the definition. Push to apply it." : "AI Studio validated the definition; the Dashboard validates it on push.");
      } else {
        onPushed(res.data.server, res.data.warnings || []);
      }
    } catch (err) {
      onError(apiErrorDetail(err, dryRun ? "Validation failed" : "Push failed"));
    } finally {
      setBusy(false);
    }
  };
  return (
    <Box>
      <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
        The Dashboard is the source of truth: the push is refused if the proxy changed there since this page loaded. Leave
        masked values (***) in place; they are restored from the live definition and never shown here.
      </Typography>
      <TextField fullWidth multiline minRows={12} value={text} onChange={(e) => setText(e.target.value)} inputProps={{ "data-testid": "definition-editor", style: { fontFamily: "monospace", fontSize: "0.8rem" } }} />
      {server.origin === "dashboard" && (
        <FormControlLabel control={<Checkbox checked={confirmOrigin} onChange={(e) => setConfirmOrigin(e.target.checked)} inputProps={{ "data-testid": "confirm-origin" }} />} label="This proxy was created on the Dashboard; AI Studio may overwrite it" />
      )}
      {preview && (preview.warnings || []).map((w) => (
        <Alert key={w} severity="warning" sx={{ mt: 1 }}>
          {w}
        </Alert>
      ))}
      <Box sx={{ display: "flex", gap: 2, mt: 2 }}>
        <SecondaryOutlineButton onClick={() => run(true)} disabled={busy} data-testid="validate-definition">
          Validate definition
        </SecondaryOutlineButton>
        <PrimaryButton variant="contained" onClick={() => run(false)} disabled={busy || (server.origin === "dashboard" && !confirmOrigin)} data-testid="push-definition">
          Push to the Dashboard
        </PrimaryButton>
      </Box>
    </Box>
  );
};

// BundleEditor pins one access policy and any number of consumption
// policies from the connection's cached Tyk policies.
const BundleEditor = ({ server, policies, onSaved, onError }) => {
  const [access, setAccess] = useState("");
  const [consumption, setConsumption] = useState([]);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    const a = (server.bundle || []).find((p) => p.role === "access");
    setAccess(a ? a.policy.tyk_policy_id : "");
    setConsumption((server.bundle || []).filter((p) => p.role === "consumption").map((p) => p.policy.tyk_policy_id));
  }, [server]);

  const accessOptions = policies.filter((p) => (p.api_ids || []).includes(server.tyk_api_id) && !p.partitions?.per_api && (!p.is_partitioned || p.partitions?.acl));
  const consumptionOptions = policies.filter((p) => p.is_partitioned && !p.partitions?.acl && !p.partitions?.per_api);

  const save = async () => {
    setSaving(true);
    try {
      const pins = [];
      if (access) pins.push({ tyk_policy_id: access, role: "access" });
      consumption.forEach((id) => pins.push({ tyk_policy_id: id, role: "consumption" }));
      const res = await apiClient.put(`/mcp-servers/${server.id}/bundle`, { pins });
      onSaved(res.data);
    } catch (err) {
      onError(apiErrorDetail(err, "Saving the bundle failed"));
    } finally {
      setSaving(false);
    }
  };

  return (
    <Box>
      <Grid container spacing={3}>
        <Grid item xs={12} md={6}>
          <TextField select fullWidth label="Access policy" value={access} onChange={(e) => setAccess(e.target.value)} inputProps={{ "data-testid": "access-select" }}>
            <MenuItem value="">None</MenuItem>
            {accessOptions.map((p) => (
              <MenuItem key={p.tyk_policy_id} value={p.tyk_policy_id}>
                {p.name} {p.is_partitioned ? "(ACL partition)" : "(all-in-one)"}
              </MenuItem>
            ))}
          </TextField>
          {accessOptions.length === 0 && (
            <Typography variant="caption" color="warning.main">
              No cached policy grants this proxy ({server.tyk_api_id}). Create one on the Dashboard and sync, or use the policy creator (full mode).
            </Typography>
          )}
        </Grid>
        <Grid item xs={12} md={6}>
          <Autocomplete
            multiple
            options={consumptionOptions.map((p) => p.tyk_policy_id)}
            getOptionLabel={(id) => policies.find((p) => p.tyk_policy_id === id)?.name || id}
            value={consumption}
            onChange={(_, v) => setConsumption(v)}
            renderInput={(params) => <TextField {...params} label="Consumption policies" />}
          />
        </Grid>
      </Grid>
      {(server.bundle || []).some((p) => p.invalid_reason) && (
        <Alert severity="warning" sx={{ mt: 2 }}>
          {server.bundle
            .filter((p) => p.invalid_reason)
            .map((p) => `${p.policy?.name || p.policy?.tyk_policy_id}: ${p.invalid_reason}`)
            .join("; ")}
        </Alert>
      )}
      <Box sx={{ mt: 3 }}>
        <PrimaryButton variant="contained" onClick={save} disabled={saving} data-testid="save-bundle">
          Save bundle
        </PrimaryButton>
      </Box>
    </Box>
  );
};

const InfoRow = ({ label, children }) => (
  <>
    <Grid item xs={12} sm={3}>
      <FieldLabel>{label}:</FieldLabel>
    </Grid>
    <Grid item xs={12} sm={9}>
      <FieldValue component="div">{children}</FieldValue>
    </Grid>
  </>
);

const MCPServerDetail = () => {
  const { id } = useParams();
  const navigate = useNavigate();
  const { notify, snackbarProps } = useFeedbackSnackbar();
  const [server, setServer] = useState(null);
  const [policies, setPolicies] = useState([]);
  const [catalogues, setCatalogues] = useState([]);
  const [error, setError] = useState(null);
  const [form, setForm] = useState({ name: "", description: "", long_description: "", logo_url: "", tags: "", privacy_score: "" });
  const [selectedCatalogues, setSelectedCatalogues] = useState([]);
  const [savingCatalogues, setSavingCatalogues] = useState(false);
  const [connection, setConnection] = useState(null);
  const [creatorOpen, setCreatorOpen] = useState(false);
  const [handoff, setHandoff] = useState(null);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [deleteForce, setDeleteForce] = useState(false);

  const load = useCallback(async () => {
    setError(null);
    try {
      const res = await apiClient.get(`/mcp-servers/${id}`);
      const s = res.data;
      setServer(s);
      setForm({
        name: s.name || "",
        description: s.description || "",
        long_description: s.long_description || "",
        logo_url: s.logo_url || "",
        tags: (s.tags || []).join(", "),
        privacy_score: s.privacy_score === null || s.privacy_score === undefined ? "" : s.privacy_score,
      });
      setSelectedCatalogues((s.tool_catalogues || []).map((c) => ({ id: Number(c.id), name: c.name })));
      const [pols, cats] = await Promise.all([
        s.connection_id ? apiClient.get(`/tyk-connections/${s.connection_id}/policies`) : Promise.resolve({ data: [] }),
        apiClient.get("/tool-catalogues", { params: { all: true } }),
      ]);
      setPolicies(pols.data || []);
      setCatalogues(catalogueOptions(cats.data));
      if (s.connection_id) {
        try {
          const conn = await apiClient.get(`/tyk-connections/${s.connection_id}`);
          setConnection(conn?.data || null);
        } catch {
          setConnection(null);
        }
      }
      if (s.dashboard_state === "pending_platform") {
        try {
          const pkg = await apiClient.get(`/mcp-servers/${s.id}/handoff`);
          setHandoff(pkg?.data || null);
        } catch {
          setHandoff(null);
        }
      }
    } catch (err) {
      setError(apiErrorDetail(err, "Failed to load MCP server"));
    }
  }, [id]);

  useEffect(() => {
    load();
  }, [load]);

  const savePresentation = async () => {
    setError(null);
    try {
      const body = {
        name: form.name,
        description: form.description,
        long_description: form.long_description,
        logo_url: form.logo_url,
        tags: form.tags.split(",").map((t) => t.trim()).filter(Boolean),
        lock_version: server.lock_version,
      };
      if (form.privacy_score === "") body.clear_privacy_score = true;
      else body.privacy_score = Number(form.privacy_score);
      const res = await apiClient.patch(`/mcp-servers/${server.id}`, body);
      setServer(res.data);
      notify("Saved");
    } catch (err) {
      setError(apiErrorDetail(err, "Save failed"));
    }
  };

  const togglePublish = async (e) => {
    const next = e.target.checked;
    setError(null);
    try {
      const res = await apiClient.post(`/mcp-servers/${server.id}/${next ? "activate" : "deactivate"}`, {});
      setServer(res.data);
      notify(next ? "Published to the portal" : "Unpublished");
    } catch (err) {
      setError(apiErrorDetail(err, "Publish failed"));
    }
  };

  const saveCatalogues = async () => {
    setError(null);
    setSavingCatalogues(true);
    try {
      const res = await apiClient.put(`/mcp-servers/${server.id}/catalogues`, {
        tool_catalogue_ids: selectedCatalogues.map((c) => Number(c.id)),
      });
      setServer(res.data);
      setSelectedCatalogues((res.data.tool_catalogues || []).map((c) => ({ id: Number(c.id), name: c.name })));
      notify("Catalogs saved");
    } catch (err) {
      setError(apiErrorDetail(err, "Saving catalogs failed"));
    } finally {
      setSavingCatalogues(false);
    }
  };

  const remove = async () => {
    setError(null);
    try {
      await apiClient.delete(`/mcp-servers/${server.id}${deleteForce ? "?force=true" : ""}`);
      navigate("/admin/mcp-servers");
    } catch (err) {
      setDeleteOpen(false);
      setError(apiErrorDetail(err, "Delete failed"));
    }
  };

  const downloadHandoff = async (includeSecrets) => {
    setError(null);
    try {
      const res = await apiClient.get(`/mcp-servers/${server.id}/handoff${includeSecrets ? "?include_secrets=true" : ""}`);
      const blob = new Blob([JSON.stringify(res.data, null, 2)], { type: "application/json" });
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = `mcp-handoff-${server.slug || server.id}${includeSecrets ? "-with-credentials" : ""}.json`;
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      URL.revokeObjectURL(url);
      notify(includeSecrets ? "Handoff package downloaded with the upstream credential; this download is audited." : "Handoff package downloaded");
    } catch (err) {
      setError(apiErrorDetail(err, "Download failed"));
    }
  };

  const linkTo = async (tykApiId) => {
    setError(null);
    try {
      const res = await apiClient.post(`/mcp-servers/${server.id}/link`, { tyk_api_id: tykApiId });
      navigate(`/admin/mcp-servers/${res.data.id}`);
    } catch (err) {
      setError(apiErrorDetail(err, "Link failed"));
    }
  };

  const reloadPolicies = async () => {
    try {
      const pols = await apiClient.get(`/tyk-connections/${server.connection_id}/policies`);
      setPolicies(pols.data || []);
    } catch {
      // the bundle response already carries the pinned policy
    }
  };

  if (!server && !error) {
    return (
      <Box sx={{ p: 3, display: "flex", justifyContent: "center" }}>
        <CircularProgress />
      </Box>
    );
  }
  if (!server) {
    return (
      <Alert severity="error" data-testid="page-error">
        {error}
      </Alert>
    );
  }

  const auth = server.auth_details || {};
  const canPublish = server.dashboard_state === "active" && server.privacy_score !== null && server.privacy_score !== undefined;
  const fullMode = connection?.effective_mode === "full" && connection?.status === "active";
  const onDashboard = server.dashboard_state !== "missing" && server.dashboard_state !== "pending_platform";
  const studioOwned = server.origin === "studio" || server.origin === "submission";
  const canDelete = !onDashboard || (studioOwned && fullMode);
  const prettyDefinition = server.definition ? JSON.stringify(JSON.parse(server.definition), null, 2) : "not available";

  return (
    <>
      <TitleBox top="64px">
        <Box sx={{ display: "flex", alignItems: "center", gap: 1, flexWrap: "wrap" }}>
          <Typography variant="headingXLarge">{server.name}</Typography>
          <KindChip kind={server.kind} />
          <DashboardStateChip state={server.dashboard_state} />
          {server.brokerable && <Chip size="small" color="primary" variant="outlined" label="Brokerable" />}
        </Box>
        <Stack direction="row" spacing={2} alignItems="center">
          <PublishSwitch
            permission={P.MCP_SERVERS_PUBLISH}
            checked={server.is_active}
            onChange={togglePublish}
            name="is_active"
            label="Published"
            disabled={!server.is_active && !canPublish}
          />
          <SecondaryLinkButton startIcon={<ArrowBackIcon />} onClick={() => navigate("/admin/mcp-servers")} color="inherit">
            Back to MCP servers
          </SecondaryLinkButton>
        </Stack>
      </TitleBox>
      <Box sx={{ p: 3 }}>
        <Typography variant="bodyLargeDefault" color="text.defaultSubdued">
          {server.description ||
            "An MCP server proxied by the Tyk Gateway. Publish it to tool catalogs so portal users can find it, and pin a policy bundle so AI Studio can mint keys for the Apps that use it."}
        </Typography>
      </Box>
      <ContentBox sx={{ pt: 0 }}>
        {error && (
          <Alert severity="error" sx={{ mb: 2 }} onClose={() => setError(null)} data-testid="page-error">
            {error}
          </Alert>
        )}
        {!canPublish && !server.is_active && (
          <Alert severity="info" sx={{ mb: 2 }}>
            {server.dashboard_state !== "active"
              ? "This server is not active on the Tyk Dashboard and cannot be published."
              : "Set a privacy score before publishing this server to the portal."}
          </Alert>
        )}

        {server.dashboard_state === "pending_platform" && (
          <Section
            title="Awaiting the platform team"
            description="This server was approved on a connection AI Studio may not write to."
            actions={
              <>
                <SecondaryOutlineButton size="small" onClick={() => downloadHandoff(false)} data-testid="download-handoff">
                  Download handoff package
                </SecondaryOutlineButton>
                <Can permission={P.MCP_SERVERS_EXECUTE}>
                  <SecondaryOutlineButton size="small" onClick={() => downloadHandoff(true)} data-testid="download-handoff-secrets">
                    Download with credential
                  </SecondaryOutlineButton>
                </Can>
              </>
            }
          >
            <Typography variant="body2" sx={{ mb: 1 }}>
              The platform team creates the proxy from the handoff package; once a sync has imported it, link it here so the
              submitter's ownership, privacy score and catalog visibility carry over.
            </Typography>
            {handoff && (
              <>
                <Typography variant="body2" color="text.secondary">
                  Submitted by {handoff.submitter?.name || "unknown"} ({handoff.submitter?.email || "no email"})
                  {handoff.submitter?.primary_contact ? `, contact ${handoff.submitter.primary_contact}` : ""}
                  {(handoff.requested_gateway_tags || []).length > 0 ? ` · requested target: ${handoff.requested_gateway_tags.join(", ")}` : ""}
                  {handoff.template_id ? ` · Dashboard template: ${handoff.template_id}` : ""}
                </Typography>
                <Box component="ol" sx={{ pl: 3, mt: 1 }}>
                  {(handoff.instructions || []).map((step) => (
                    <Typography key={step} component="li" variant="body2">
                      {step}
                    </Typography>
                  ))}
                </Box>
                {(handoff.candidates || []).length > 0 ? (
                  <Box sx={{ mt: 1 }} data-testid="link-candidates">
                    <Typography variant="subtitle2">Imported proxies that look like this server</Typography>
                    {handoff.candidates.map((c) => (
                      <Box key={c.id} sx={{ display: "flex", alignItems: "center", gap: 1, mt: 0.5 }}>
                        <Typography variant="body2">
                          {c.name} · {c.listen_path} · <code>{c.tyk_api_id}</code>
                        </Typography>
                        <Can permission={P.MCP_SERVERS_EXECUTE}>
                          <PrimaryButton size="small" variant="contained" onClick={() => linkTo(c.tyk_api_id)} data-testid={`link-${c.tyk_api_id}`}>
                            Link
                          </PrimaryButton>
                        </Can>
                      </Box>
                    ))}
                  </Box>
                ) : (
                  <Typography variant="body2" color="text.secondary" sx={{ mt: 1 }}>
                    No imported proxy matches yet. Run a sync after the platform team has created it.
                  </Typography>
                )}
              </>
            )}
          </Section>
        )}

        <Section title="Server information">
          <Grid container spacing={2}>
            <InfoRow label="Connection">{server.connection_name || server.connection_id}</InfoRow>
            <InfoRow label="Kind">
              <KindChip kind={server.kind} />
            </InfoRow>
            <InfoRow label="Dashboard">
              <DashboardStateChip state={server.dashboard_state} />
              {server.tyk_api_id ? (
                <Typography variant="caption" color="text.secondary" sx={{ ml: 1 }}>
                  API id {server.tyk_api_id}
                </Typography>
              ) : (
                <Typography variant="caption" color="text.secondary" sx={{ ml: 1 }}>
                  not on the Dashboard yet
                </Typography>
              )}
            </InfoRow>
            <InfoRow label="Listen path">
              {server.listen_path}
              {server.transport_path ? ` (transport ${server.transport_path})` : ""}
            </InfoRow>
            <InfoRow label="Endpoint">
              {server.endpoint_url || "set a gateway base URL on the connection"}
              {Object.entries(server.endpoint_urls || {}).map(([tag, url]) => (
                <Typography key={tag} variant="body2" component="div">
                  {tag}: {url}
                </Typography>
              ))}
            </InfoRow>
            <InfoRow label="Upstream">{server.upstream_url || "—"}</InfoRow>
            <InfoRow label="Consumer auth">
              {AUTH_MODE_LABELS[server.auth_mode] || server.auth_mode}
              {auth.header_name ? ` (header ${auth.header_name})` : ""}
              {auth.prm && (
                <Typography variant="body2" component="div" color="text.secondary">
                  OAuth authorization servers: {(auth.prm.authorization_servers || []).join(", ") || "none"}
                  {auth.prm.url ? ` · metadata ${auth.prm.url}` : ""}
                </Typography>
              )}
            </InfoRow>
            <InfoRow label="Privacy level">
              {server.privacy_score === null || server.privacy_score === undefined ? "Not set" : <PrivacyLevelChip score={server.privacy_score} />}
            </InfoRow>
            <InfoRow label="Deployed to">
              {(server.gateway_tags?.tags || []).length > 0
                ? server.gateway_tags.tags.map((t) => <Chip key={t} size="small" label={t} sx={{ mr: 0.5 }} />)
                : "every non-segmented gateway"}
            </InfoRow>
            <InfoRow label="Origin">
              {server.origin} · last seen {server.last_seen_at ? formatTime(server.last_seen_at) : "never"}
            </InfoRow>
          </Grid>
        </Section>

        <Section title="Presentation and governance" description="What portal users read about this server. Definition-derived fields above always follow the Dashboard.">
          <Grid container spacing={3}>
            <Grid item xs={12} md={6}>
              <TextField fullWidth label="Name" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} helperText={server.name_overridden ? "Overridden; the Dashboard name no longer applies" : "Follows the Dashboard until edited"} />
            </Grid>
            <Grid item xs={12} md={3}>
              <TextField fullWidth type="number" label="Privacy score" value={form.privacy_score} onChange={(e) => setForm({ ...form, privacy_score: e.target.value })} inputProps={{ min: 0, max: 100, "data-testid": "privacy-score" }} />
            </Grid>
            <Grid item xs={12} md={3}>
              <TextField fullWidth label="Logo URL" value={form.logo_url} onChange={(e) => setForm({ ...form, logo_url: e.target.value })} />
            </Grid>
            <Grid item xs={12}>
              <TextField fullWidth label="Short description" value={form.description} onChange={(e) => setForm({ ...form, description: e.target.value })} />
            </Grid>
            <Grid item xs={12}>
              <TextField fullWidth multiline rows={3} label="Long description" value={form.long_description} onChange={(e) => setForm({ ...form, long_description: e.target.value })} />
            </Grid>
            <Grid item xs={12}>
              <TextField fullWidth label="Tags (comma separated)" value={form.tags} onChange={(e) => setForm({ ...form, tags: e.target.value })} />
            </Grid>
          </Grid>
          <Can permission={P.MCP_SERVERS_WRITE}>
            <Box sx={{ mt: 3 }}>
              <PrimaryButton variant="contained" onClick={savePresentation} data-testid="save-presentation">
                Save
              </PrimaryButton>
            </Box>
          </Can>
        </Section>

        <Section title="Portal visibility" description="Portal users see this server through the teams granted these catalogs.">
          {!server.brokerable && (
            <Alert severity="info" sx={{ mb: 2 }} data-testid="not-brokerable">
              AI Studio does not broker access to this server. Portal users connect to it directly with {directAccessWording(server.auth_mode)}; the
              portal shows no Build app button for it.
            </Alert>
          )}
          <Can permission={P.MCP_SERVERS_WRITE} fallback={<Typography variant="body2">{selectedCatalogues.map((c) => c.name).join(", ") || "Not in any catalog."}</Typography>}>
            <RelationshipPicker
              label="Tool catalogs this server is published in"
              itemLabel="catalog"
              value={selectedCatalogues}
              onChange={setSelectedCatalogues}
              options={catalogues}
              idField="id"
              getOptionLabel={(c) => c.name}
            />
            <Box sx={{ mt: 3 }}>
              <PrimaryButton variant="contained" onClick={saveCatalogues} disabled={savingCatalogues} data-testid="save-catalogues">
                Save catalogs
              </PrimaryButton>
            </Box>
          </Can>
        </Section>

        <Section
          title="Access policies"
          description="A bundle is one access policy (the ACL that names this proxy) plus optional consumption policies (rate limit and quota partitions). Keys minted for Apps carry the union of the bundles of every MCP server they use; Tyk applies policy changes to those keys at request time."
          actions={
            fullMode && server.tyk_api_id ? (
              <Can permission={P.MCP_SERVERS_EXECUTE}>
                <SecondaryOutlineButton size="small" onClick={() => setCreatorOpen(true)} data-testid="open-policy-creator">
                  Create policy
                </SecondaryOutlineButton>
              </Can>
            ) : null
          }
        >
          <Can permission={P.MCP_SERVERS_WRITE} fallback={<Typography variant="body2">{(server.bundle || []).map((p) => `${p.role}: ${p.policy?.name}`).join(", ") || "No bundle pinned."}</Typography>}>
            <BundleEditor server={server} policies={policies} onSaved={(s) => { setServer(s); notify("Bundle saved"); }} onError={setError} />
          </Can>
          <PolicyCreator
            open={creatorOpen}
            server={server}
            onClose={() => setCreatorOpen(false)}
            onError={setError}
            onCreated={async (pol) => {
              setCreatorOpen(false);
              notify(`Policy ${pol.name} created on the Dashboard and pinned`);
              await reloadPolicies();
              await load();
            }}
          />
        </Section>

        <Section title={`Primitives (${(server.primitives || []).length})`}>
          {(server.primitives || []).length === 0 ? (
            <Typography variant="body2" color="text.secondary">
              No tools, resources or prompts are declared in the definition.
            </Typography>
          ) : (
            <Table size="small">
              <TableHead>
                <TableRow>
                  <TableCell>Type</TableCell>
                  <TableCell>Name</TableCell>
                  <TableCell>Description</TableCell>
                  <TableCell>Auth</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {server.primitives.map((p) => (
                  <TableRow key={`${p.type}:${p.name}`}>
                    <TableCell>{p.type}</TableCell>
                    <TableCell>{p.name}</TableCell>
                    <TableCell>{p.description || p.source || ""}</TableCell>
                    <TableCell>
                      {p.auth?.ignore_authentication ? "no auth" : ""}
                      {(p.auth?.scopes || []).length > 0 ? `scopes: ${p.auth.scopes.join(", ")}` : ""}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </Section>

        <Section
          title="Credentials using this server"
          actions={
            <SecondaryOutlineButton size="small" onClick={() => navigate(`/admin/mcp-credentials?server_id=${server.id}`)} data-testid="view-credentials">
              View minted keys
            </SecondaryOutlineButton>
          }
        >
          <Typography variant="body2" color="text.secondary">
            Keys minted for Apps bound to this server are listed in the MCP credentials ledger, together with the access report of who reaches it.{" "}
            <Link component="button" variant="body2" onClick={() => navigate(`/admin/mcp-credentials?tab=report&server=${server.id}`)} data-testid="view-access-report">
              Open the access report
            </Link>
          </Typography>
        </Section>

        {fullMode && onDashboard ? (
          <Section title="Definition" description="Edit the Tyk OAS document and push it to the Dashboard.">
            <Can permission={P.MCP_SERVERS_EXECUTE} fallback={<Box component="pre" sx={{ ...preStyle, maxHeight: 480 }}>{prettyDefinition}</Box>}>
              <DefinitionEditor
                server={server}
                onError={setError}
                onNotice={notify}
                onPushed={(s, warnings) => {
                  setServer(s);
                  notify(warnings.length ? `Pushed. ${warnings.join(" ")}` : "Pushed to the Dashboard");
                }}
              />
            </Can>
          </Section>
        ) : (
          <Section title="Definition" description="As synced from the Dashboard, upstream credentials masked.">
            <Box component="pre" sx={{ ...preStyle, maxHeight: 480 }}>
              {prettyDefinition}
            </Box>
          </Section>
        )}

        {canDelete && (
          <Can permission={P.MCP_SERVERS_DELETE}>
            <Section
              title="Danger zone"
              description={onDashboard ? "Removes the proxy from the Tyk Dashboard and this record from AI Studio." : "Removes this record from AI Studio."}
            >
              <DangerOutlineButton onClick={() => (onDashboard ? setDeleteOpen(true) : remove())} data-testid="delete-server">
                {onDashboard ? "Delete from the Dashboard" : "Delete record"}
              </DangerOutlineButton>
            </Section>
          </Can>
        )}
      </ContentBox>

      <ConfirmationDialog
        open={deleteOpen}
        data-testid="delete-dialog"
        title={`Delete ${server.name} from the Tyk Dashboard?`}
        message={
          <>
            The proxy is removed from the Dashboard and the gateways stop serving it. Apps lose the binding; keys that reach nothing else on
            this connection are revoked when you tick the box below, otherwise the delete is refused while such keys exist.
            <FormControlLabel
              sx={{ display: "flex", mt: 1 }}
              control={<Checkbox checked={deleteForce} onChange={(e) => setDeleteForce(e.target.checked)} inputProps={{ "data-testid": "delete-force" }} />}
              label="Revoke keys that would be left without access"
            />
          </>
        }
        confirmText="This cannot be undone."
        buttonLabel="Delete"
        onConfirm={remove}
        onCancel={() => setDeleteOpen(false)}
        iconName="hexagon-exclamation"
        iconColor="background.buttonCritical"
        titleColor="text.criticalDefault"
        backgroundColor="background.surfaceCriticalDefault"
        borderColor="border.criticalDefaultSubdue"
        primaryButtonComponent="danger"
      />
      <FeedbackSnackbar {...snackbarProps} />
    </>
  );
};

export default MCPServerDetail;
