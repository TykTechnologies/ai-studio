import React, { useCallback, useEffect, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import {
  Alert,
  Autocomplete,
  Box,
  Button,
  Checkbox,
  Chip,
  CircularProgress,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Divider,
  FormControlLabel,
  Grid,
  MenuItem,
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
import { P } from "../rbac/permissions";
import { TitleBox, StyledPaper } from "../styles/sharedStyles";
import { apiErrorDetail, formatTime, preStyle } from "./webhookShared";
import { AUTH_MODE_LABELS, DashboardStateChip, KindChip } from "./MCPServers";

const normaliseList = (data) => (Array.isArray(data) ? data : data?.data || data?.groups || []);

// PolicyCreator writes a partitioned Studio-managed policy to the Dashboard
// (full-mode connections) and pins it to this server in the same call.
const PolicyCreator = ({ open, server, onClose, onCreated, onError }) => {
  const [form, setForm] = useState({ kind: "access", name: "", rate: "", per: "60", quota_max: "", quota_renewal_rate: "3600", key_expires_in: "" });
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
      setForm({ kind: "access", name: "", rate: "", per: "60", quota_max: "", quota_renewal_rate: "3600", key_expires_in: "" });
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
            <TextField select fullWidth size="small" label="Kind" value={form.kind} onChange={set("kind")} inputProps={{ "data-testid": "policy-kind" }}>
              <MenuItem value="access">Access (ACL for this proxy)</MenuItem>
              <MenuItem value="consumption">Consumption (rate limit / quota)</MenuItem>
            </TextField>
          </Grid>
          <Grid item xs={12} md={7}>
            <TextField fullWidth size="small" label="Name" value={form.name} onChange={set("name")} inputProps={{ "data-testid": "policy-name" }} />
          </Grid>
          {form.kind === "consumption" && (
            <>
              <Grid item xs={6} md={3}>
                <TextField fullWidth size="small" type="number" label="Rate" value={form.rate} onChange={set("rate")} inputProps={{ "data-testid": "policy-rate" }} />
              </Grid>
              <Grid item xs={6} md={3}>
                <TextField fullWidth size="small" type="number" label="Per (seconds)" value={form.per} onChange={set("per")} />
              </Grid>
              <Grid item xs={6} md={3}>
                <TextField fullWidth size="small" type="number" label="Quota max" value={form.quota_max} onChange={set("quota_max")} helperText="-1 unlimited" />
              </Grid>
              <Grid item xs={6} md={3}>
                <TextField fullWidth size="small" type="number" label="Quota renews (s)" value={form.quota_renewal_rate} onChange={set("quota_renewal_rate")} />
              </Grid>
              <Grid item xs={12} md={6}>
                <TextField fullWidth size="small" type="number" label="Key expires in (seconds, 0 = never)" value={form.key_expires_in} onChange={set("key_expires_in")} />
              </Grid>
            </>
          )}
        </Grid>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>Cancel</Button>
        <Button variant="contained" onClick={submit} disabled={busy || !form.name.trim()} data-testid="policy-create">
          Create and pin
        </Button>
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
        onNotice("The Dashboard accepted the definition. Push to apply it.");
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
      <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
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
      <Box sx={{ display: "flex", gap: 1, mt: 1 }}>
        <Button variant="outlined" onClick={() => run(true)} disabled={busy} data-testid="validate-definition">
          Validate on the Dashboard
        </Button>
        <Button variant="contained" onClick={() => run(false)} disabled={busy || (server.origin === "dashboard" && !confirmOrigin)} data-testid="push-definition">
          Push to the Dashboard
        </Button>
      </Box>
    </Box>
  );
};

const Section = ({ title, children, action }) => (
  <StyledPaper sx={{ p: 2, mb: 2 }}>
    <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center", mb: 1 }}>
      <Typography variant="h6">{title}</Typography>
      {action}
    </Box>
    {children}
  </StyledPaper>
);

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
      <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
        A bundle is one access policy (the ACL that names this proxy) plus optional consumption policies (rate limit and
        quota partitions). Keys minted for Apps carry the union of the bundles of every MCP server they use, and Tyk applies
        policy changes to those keys at request time.
      </Typography>
      <Grid container spacing={2}>
        <Grid item xs={12} md={6}>
          <TextField select fullWidth size="small" label="Access policy" value={access} onChange={(e) => setAccess(e.target.value)} inputProps={{ "data-testid": "access-select" }}>
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
            size="small"
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
      <Box sx={{ mt: 2 }}>
        <Button variant="contained" onClick={save} disabled={saving} data-testid="save-bundle">
          Save bundle
        </Button>
      </Box>
    </Box>
  );
};

const MCPServerDetail = () => {
  const { id } = useParams();
  const navigate = useNavigate();
  const [server, setServer] = useState(null);
  const [policies, setPolicies] = useState([]);
  const [groups, setGroups] = useState([]);
  const [error, setError] = useState(null);
  const [notice, setNotice] = useState(null);
  const [form, setForm] = useState({ name: "", description: "", long_description: "", logo_url: "", tags: "", privacy_score: "" });
  const [groupIds, setGroupIds] = useState([]);
  const [connection, setConnection] = useState(null);
  const [creatorOpen, setCreatorOpen] = useState(false);
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
      setGroupIds(s.group_ids || []);
      const [pols, grps] = await Promise.all([
        s.connection_id ? apiClient.get(`/tyk-connections/${s.connection_id}/policies`) : Promise.resolve({ data: [] }),
        apiClient.get("/groups"),
      ]);
      setPolicies(pols.data || []);
      setGroups(normaliseList(grps.data));
      if (s.connection_id) {
        try {
          const conn = await apiClient.get(`/tyk-connections/${s.connection_id}`);
          setConnection(conn?.data || null);
        } catch {
          setConnection(null);
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
      setNotice("Saved");
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
      setNotice(next ? "Published to the portal" : "Unpublished");
    } catch (err) {
      setError(apiErrorDetail(err, "Publish failed"));
    }
  };

  const saveGroups = async () => {
    setError(null);
    try {
      const res = await apiClient.put(`/mcp-servers/${server.id}/groups`, { group_ids: groupIds });
      setServer(res.data);
      setNotice("Teams saved");
    } catch (err) {
      setError(apiErrorDetail(err, "Saving teams failed"));
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

  return (
    <Box>
      <TitleBox>
        <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
          <Button startIcon={<ArrowBackIcon />} onClick={() => navigate("/admin/mcp-servers")}>
            Back
          </Button>
          <Typography variant="h5">{server.name}</Typography>
          <KindChip kind={server.kind} />
          <DashboardStateChip state={server.dashboard_state} />
          {server.brokerable && <Chip size="small" color="primary" variant="outlined" label="Brokerable" />}
        </Box>
        <PublishSwitch
          permission={P.MCP_SERVERS_PUBLISH}
          checked={server.is_active}
          onChange={togglePublish}
          name="is_active"
          label="Published"
          disabled={!server.is_active && !canPublish}
        />
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
      {!canPublish && !server.is_active && (
        <Alert severity="info" sx={{ mb: 2 }}>
          {server.dashboard_state !== "active"
            ? "This server is not active on the Tyk Dashboard and cannot be published."
            : "Set a privacy score before publishing this server to the portal."}
        </Alert>
      )}

      <Section title="On the Tyk Gateway">
        <Grid container spacing={2}>
          <Grid item xs={12} md={6}>
            <Typography variant="body2">
              <strong>Connection:</strong> {server.connection_name || server.connection_id}
            </Typography>
            <Typography variant="body2">
              <strong>API id:</strong> {server.tyk_api_id || "not on the Dashboard yet"}
            </Typography>
            <Typography variant="body2">
              <strong>Listen path:</strong> {server.listen_path} {server.transport_path ? `(transport ${server.transport_path})` : ""}
            </Typography>
            <Typography variant="body2">
              <strong>Endpoint:</strong> {server.endpoint_url || "set a gateway base URL on the connection"}
            </Typography>
            {Object.entries(server.endpoint_urls || {}).map(([tag, url]) => (
              <Typography key={tag} variant="body2">
                <strong>{tag}:</strong> {url}
              </Typography>
            ))}
            <Typography variant="body2">
              <strong>Upstream:</strong> {server.upstream_url}
            </Typography>
          </Grid>
          <Grid item xs={12} md={6}>
            <Typography variant="body2">
              <strong>Consumer auth:</strong> {AUTH_MODE_LABELS[server.auth_mode] || server.auth_mode}
              {auth.header_name ? ` (header ${auth.header_name})` : ""}
            </Typography>
            {auth.prm && (
              <Typography variant="body2">
                <strong>OAuth authorization servers:</strong> {(auth.prm.authorization_servers || []).join(", ") || "none"}
                {auth.prm.url ? ` · metadata ${auth.prm.url}` : ""}
              </Typography>
            )}
            <Typography variant="body2" component="div">
              <strong>Deployed to:</strong>{" "}
              {(server.gateway_tags?.tags || []).length > 0 ? (
                server.gateway_tags.tags.map((t) => <Chip key={t} size="small" label={t} sx={{ mr: 0.5 }} />)
              ) : (
                "every non-segmented gateway"
              )}
            </Typography>
            <Typography variant="body2">
              <strong>Origin:</strong> {server.origin} · last seen {server.last_seen_at ? formatTime(server.last_seen_at) : "never"}
            </Typography>
          </Grid>
        </Grid>
      </Section>

      <Section title="Presentation and governance">
        <Grid container spacing={2}>
          <Grid item xs={12} md={6}>
            <TextField fullWidth size="small" label="Name" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} helperText={server.name_overridden ? "Overridden; the Dashboard name no longer applies" : "Follows the Dashboard until edited"} />
          </Grid>
          <Grid item xs={12} md={3}>
            <TextField fullWidth size="small" type="number" label="Privacy score" value={form.privacy_score} onChange={(e) => setForm({ ...form, privacy_score: e.target.value })} inputProps={{ min: 0, max: 100, "data-testid": "privacy-score" }} />
          </Grid>
          <Grid item xs={12} md={3}>
            <TextField fullWidth size="small" label="Logo URL" value={form.logo_url} onChange={(e) => setForm({ ...form, logo_url: e.target.value })} />
          </Grid>
          <Grid item xs={12}>
            <TextField fullWidth size="small" label="Short description" value={form.description} onChange={(e) => setForm({ ...form, description: e.target.value })} />
          </Grid>
          <Grid item xs={12}>
            <TextField fullWidth size="small" multiline minRows={2} label="Long description" value={form.long_description} onChange={(e) => setForm({ ...form, long_description: e.target.value })} />
          </Grid>
          <Grid item xs={12}>
            <TextField fullWidth size="small" label="Tags (comma separated)" value={form.tags} onChange={(e) => setForm({ ...form, tags: e.target.value })} />
          </Grid>
        </Grid>
        <Can permission={P.MCP_SERVERS_WRITE}>
          <Box sx={{ mt: 2 }}>
            <Button variant="contained" onClick={savePresentation} data-testid="save-presentation">
              Save
            </Button>
          </Box>
        </Can>
      </Section>

      <Section title="Teams that can see this server">
        <Autocomplete
          multiple
          size="small"
          options={groups.map((g) => g.id)}
          getOptionLabel={(gid) => groups.find((g) => g.id === gid)?.name || String(gid)}
          value={groupIds}
          onChange={(_, v) => setGroupIds(v)}
          renderInput={(params) => <TextField {...params} label="Teams" />}
        />
        <Can permission={P.MCP_SERVERS_WRITE}>
          <Box sx={{ mt: 2 }}>
            <Button variant="outlined" onClick={saveGroups}>
              Save teams
            </Button>
          </Box>
        </Can>
      </Section>

      <Section
        title="Policy bundle"
        action={
          fullMode && server.tyk_api_id ? (
            <Can permission={P.MCP_SERVERS_EXECUTE}>
              <Button size="small" variant="outlined" onClick={() => setCreatorOpen(true)} data-testid="open-policy-creator">
                Create policy
              </Button>
            </Can>
          ) : null
        }
      >
        <Can permission={P.MCP_SERVERS_WRITE} fallback={<Typography variant="body2">{(server.bundle || []).map((p) => `${p.role}: ${p.policy?.name}`).join(", ") || "No bundle pinned."}</Typography>}>
          <BundleEditor server={server} policies={policies} onSaved={(s) => { setServer(s); setNotice("Bundle saved"); }} onError={setError} />
        </Can>
        <PolicyCreator
          open={creatorOpen}
          server={server}
          onClose={() => setCreatorOpen(false)}
          onError={setError}
          onCreated={async (pol) => {
            setCreatorOpen(false);
            setNotice(`Policy ${pol.name} created on the Dashboard and pinned`);
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
        title="Credentials"
        action={
          <Button size="small" onClick={() => navigate(`/admin/mcp-credentials?server_id=${server.id}`)} data-testid="view-credentials">
            View minted keys
          </Button>
        }
      >
        <Typography variant="body2" color="text.secondary">
          Keys minted for Apps bound to this server are listed in the MCP credentials ledger, together with the access report of who reaches it.
          {" "}
          <Link component="button" variant="body2" onClick={() => navigate(`/admin/mcp-credentials?tab=report&server=${server.id}`)} data-testid="view-access-report">
            Open the access report
          </Link>
        </Typography>
      </Section>

      {fullMode && onDashboard ? (
        <Section title="Definition (edit and push to the Dashboard)">
          <Can permission={P.MCP_SERVERS_EXECUTE} fallback={<Box component="pre" sx={{ ...preStyle, maxHeight: 480 }}>{server.definition ? JSON.stringify(JSON.parse(server.definition), null, 2) : "not available"}</Box>}>
            <DefinitionEditor
              server={server}
              onError={setError}
              onNotice={setNotice}
              onPushed={(s, warnings) => {
                setServer(s);
                setNotice(warnings.length ? `Pushed. ${warnings.join(" ")}` : "Pushed to the Dashboard");
              }}
            />
          </Can>
        </Section>
      ) : (
        <Section title="Definition (from the Dashboard, upstream credentials masked)">
          <Box component="pre" sx={{ ...preStyle, maxHeight: 480 }}>
            {server.definition ? JSON.stringify(JSON.parse(server.definition), null, 2) : "not available"}
          </Box>
        </Section>
      )}

      {(!onDashboard || (studioOwned && fullMode)) && (
        <Can permission={P.MCP_SERVERS_DELETE}>
          <Divider sx={{ my: 2 }} />
          <Button color="error" variant="outlined" onClick={() => (onDashboard ? setDeleteOpen(true) : remove())} data-testid="delete-server">
            {onDashboard ? "Delete from the Dashboard" : "Delete record"}
          </Button>
        </Can>
      )}
      <Dialog open={deleteOpen} onClose={() => setDeleteOpen(false)}>
        <DialogTitle>Delete this MCP proxy from the Tyk Dashboard?</DialogTitle>
        <DialogContent>
          <Typography variant="body2" sx={{ mb: 1 }}>
            The proxy is removed from the Dashboard and the gateways stop serving it. Apps lose the binding; keys that reach
            nothing else on this connection are revoked when you tick the box below, otherwise the delete is refused while
            such keys exist.
          </Typography>
          <FormControlLabel control={<Checkbox checked={deleteForce} onChange={(e) => setDeleteForce(e.target.checked)} inputProps={{ "data-testid": "delete-force" }} />} label="Revoke keys that would be left without access" />
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setDeleteOpen(false)}>Cancel</Button>
          <Button color="error" variant="contained" onClick={remove} data-testid="confirm-delete">
            Delete
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
};

export default MCPServerDetail;
