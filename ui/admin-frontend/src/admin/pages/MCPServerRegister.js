import React, { useCallback, useEffect, useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";
import {
  Alert,
  Autocomplete,
  Box,
  Checkbox,
  Chip,
  CircularProgress,
  FormControlLabel,
  Grid,
  MenuItem,
  Step,
  StepLabel,
  Stepper,
  Switch,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
  TextField,
  Tooltip,
  Typography,
} from "@mui/material";
import ArrowBackIcon from "@mui/icons-material/ArrowBack";
import apiClient from "../utils/apiClient";
import Section from "../components/common/Section";
import RelationshipPicker from "../components/common/relationship-picker";
import { TitleBox, ContentBox, PrimaryButton, SecondaryLinkButton, SecondaryOutlineButton } from "../styles/sharedStyles";
import { apiErrorDetail, preStyle } from "./webhookShared";
import { TykUpsell, TykDisabledNotice } from "./tykShared";

const STEPS = ["Connection and kind", "Proxy details", "Access and governance", "Review and create"];

const slugify = (s) =>
  (s || "")
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "");

const parseList = (s) =>
  (s || "")
    .split(/[,\n]/)
    .map((v) => v.trim())
    .filter(Boolean);

/**
 * buildPayload turns the wizard state into the register request. It is
 * exported so the test can assert the exact body without driving every field.
 */
export const buildPayload = (f) => {
  const body = {
    connection_id: Number(f.connection_id),
    kind: f.kind,
    name: f.name,
    listen_path: f.listen_path,
    consumer_auth: f.consumer_auth,
    gateway_tags: f.gateway_tags,
    confirm_no_gateway_tags: f.confirm_no_gateway_tags,
    description: f.description,
    long_description: f.long_description,
    tags: parseList(f.tags),
    publish: f.publish,
    tool_catalogue_ids: (f.tool_catalogues || []).map((c) => Number(c.id)),
  };
  if (f.privacy_score !== "") body.privacy_score = Number(f.privacy_score);
  if (f.kind === "remote") {
    body.upstream_url = f.upstream_url;
    if (f.upstream_auth_token) {
      body.upstream_auth_header_name = f.upstream_auth_header_name || "Authorization";
      body.upstream_auth_token = f.upstream_auth_token;
    }
    body.allowed_tools = parseList(f.allowed_tools);
  } else {
    body.source_api_id = f.source_api_id;
    body.primitives = f.primitives
      .filter((p) => p.selected)
      .map((p) => ({ operation_id: p.operation_id, method: p.method, path: p.path, name: p.name, description: p.description }));
  }
  if (f.consumer_auth === "oauth21") {
    body.authorization_servers = parseList(f.authorization_servers);
    body.scopes_supported = parseList(f.scopes_supported);
  }
  if (f.consumer_auth === "keyless") body.confirm_keyless = f.confirm_keyless;
  return body;
};

const initialForm = {
  connection_id: "",
  kind: "remote",
  name: "",
  listen_path: "",
  upstream_url: "",
  upstream_auth_header_name: "Authorization",
  upstream_auth_token: "",
  allowed_tools: "",
  source_api_id: "",
  primitives: [],
  consumer_auth: "auth_token",
  authorization_servers: "",
  scopes_supported: "",
  confirm_keyless: false,
  gateway_tags: [],
  confirm_no_gateway_tags: false,
  description: "",
  long_description: "",
  tags: "",
  privacy_score: "",
  publish: false,
  tool_catalogues: [],
};

const MCPServerRegister = () => {
  const navigate = useNavigate();
  const [status, setStatus] = useState(null);
  const [connections, setConnections] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [step, setStep] = useState(0);
  const [form, setForm] = useState(initialForm);
  const [tagOptions, setTagOptions] = useState([]);
  const [sourceAPIs, setSourceAPIs] = useState([]);
  const [catalogues, setCatalogues] = useState([]);
  const [opsLoading, setOpsLoading] = useState(false);
  const [preview, setPreview] = useState(null);
  const [busy, setBusy] = useState(false);

  const set = (k) => (e) => setForm((f) => ({ ...f, [k]: e?.target?.type === "checkbox" ? e.target.checked : e.target.value }));

  useEffect(() => {
    const load = async () => {
      try {
        const st = await apiClient.get("/tyk-mcp/status");
        setStatus(st.data);
        if (st.data?.available && st.data?.enabled) {
          const conns = await apiClient.get("/tyk-connections");
          setConnections((conns.data || []).filter((c) => c.status === "active" && c.effective_mode === "full"));
          try {
            const cats = await apiClient.get("/tool-catalogues", { params: { all: true } });
            const rows = Array.isArray(cats.data) ? cats.data : cats.data?.data || [];
            setCatalogues(rows.map((c) => ({ id: Number(c.id), name: c.attributes?.name ?? c.name ?? `Catalog ${c.id}` })));
          } catch {
            setCatalogues([]);
          }
        }
      } catch (err) {
        setError(apiErrorDetail(err, "Failed to load"));
      } finally {
        setLoading(false);
      }
    };
    load();
  }, []);

  const connection = useMemo(() => connections.find((c) => String(c.id) === String(form.connection_id)), [connections, form.connection_id]);
  const restSupported = connection?.capabilities?.rest_to_mcp_supported?.state === "ok";

  // Deployment targets and source APIs follow the chosen connection.
  useEffect(() => {
    if (!form.connection_id) return;
    let cancelled = false;
    (async () => {
      try {
        const tags = await apiClient.get(`/tyk-connections/${form.connection_id}/gateway-tags`);
        if (!cancelled) setTagOptions(tags.data || []);
      } catch {
        if (!cancelled) setTagOptions([]);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [form.connection_id]);

  useEffect(() => {
    if (!form.connection_id || form.kind !== "rest_to_mcp") return;
    let cancelled = false;
    (async () => {
      try {
        const apis = await apiClient.get(`/tyk-connections/${form.connection_id}/apis`);
        if (!cancelled) setSourceAPIs(apis.data || []);
      } catch (err) {
        if (!cancelled) setError(apiErrorDetail(err, "Failed to list the Dashboard's APIs"));
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [form.connection_id, form.kind]);

  const loadOperations = useCallback(
    async (apiId) => {
      setOpsLoading(true);
      try {
        const ops = await apiClient.get(`/tyk-connections/${form.connection_id}/apis/${encodeURIComponent(apiId)}/operations`);
        setForm((f) => ({
          ...f,
          source_api_id: apiId,
          primitives: (ops.data || []).map((op) => ({
            ...op,
            selected: false,
            name: op.operation_id || `${op.method.toLowerCase()}_${op.path.replace(/[^a-zA-Z0-9]+/g, "_").replace(/^_+|_+$/g, "")}`,
            description: op.summary || "",
          })),
        }));
      } catch (err) {
        setError(apiErrorDetail(err, "Failed to list operations"));
      } finally {
        setOpsLoading(false);
      }
    },
    [form.connection_id]
  );

  const validateStep = () => {
    switch (step) {
      case 0:
        if (!form.connection_id) return "Choose a connection.";
        return null;
      case 1:
        if (!form.name.trim()) return "A name is required.";
        if (form.kind === "remote" && !form.upstream_url.trim()) return "The upstream MCP URL is required.";
        if (form.kind === "rest_to_mcp" && !form.source_api_id) return "Choose the source API.";
        if (form.kind === "rest_to_mcp" && !form.primitives.some((p) => p.selected)) return "Select at least one operation to expose.";
        return null;
      case 2:
        if (form.consumer_auth === "keyless" && !form.confirm_keyless) return "Confirm that the proxy may be keyless.";
        if (form.consumer_auth === "oauth21" && parseList(form.authorization_servers).length === 0) return "Enter at least one authorization server.";
        if (tagOptions.length > 0 && form.gateway_tags.length === 0 && !form.confirm_no_gateway_tags) return "Choose a deployment target or confirm that none is wanted.";
        if (form.publish && form.privacy_score === "") return "Publishing needs a privacy score.";
        return null;
      default:
        return null;
    }
  };

  const next = async () => {
    const problem = validateStep();
    if (problem) {
      setError(problem);
      return;
    }
    setError(null);
    if (step === 2) {
      setBusy(true);
      try {
        const res = await apiClient.post("/mcp-servers/register?dry_run=1", buildPayload(form));
        setPreview(res.data);
        setStep(3);
      } catch (err) {
        setError(apiErrorDetail(err, "The Dashboard rejected the definition"));
      } finally {
        setBusy(false);
      }
      return;
    }
    setStep(step + 1);
  };

  const create = async () => {
    setBusy(true);
    setError(null);
    try {
      const res = await apiClient.post("/mcp-servers/register", buildPayload(form));
      navigate(`/admin/mcp-servers/${res.data.server.id}`);
    } catch (err) {
      setError(apiErrorDetail(err, "Registration failed"));
    } finally {
      setBusy(false);
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

  const setPrimitive = (i, patch) => setForm((f) => ({ ...f, primitives: f.primitives.map((q, j) => (j === i ? { ...q, ...patch } : q)) }));

  return (
    <>
      <TitleBox top="64px">
        <Typography variant="headingXLarge">Register MCP server</Typography>
        <SecondaryLinkButton startIcon={<ArrowBackIcon />} onClick={() => navigate("/admin/mcp-servers")} color="inherit">
          Back to MCP servers
        </SecondaryLinkButton>
      </TitleBox>
      <Box sx={{ p: 3 }}>
        <Typography variant="bodyLargeDefault" color="text.defaultSubdued">
          Create an MCP proxy on a connected Tyk Dashboard from here. The Tyk Gateway serves it; AI Studio catalogues it, publishes it to tool
          catalogs and mints keys for the Apps that use it.
        </Typography>
      </Box>
      <ContentBox sx={{ pt: 0 }}>
        <Stepper activeStep={step} sx={{ mb: 3 }}>
          {STEPS.map((label) => (
            <Step key={label}>
              <StepLabel>{label}</StepLabel>
            </Step>
          ))}
        </Stepper>
        {error && (
          <Alert severity="error" sx={{ mb: 2 }} onClose={() => setError(null)} data-testid="page-error">
            {error}
          </Alert>
        )}

        {step === 0 && (
          <Section title="Connection and kind" description="Where the proxy is created and what it fronts.">
            <Grid container spacing={3}>
              <Grid item xs={12} md={6}>
                <TextField select fullWidth label="Tyk connection" value={form.connection_id} onChange={set("connection_id")} inputProps={{ "data-testid": "connection" }} helperText="Only active connections in full mode can create proxies.">
                  {connections.map((c) => (
                    <MenuItem key={c.id} value={String(c.id)}>
                      {c.name}
                    </MenuItem>
                  ))}
                </TextField>
                {connections.length === 0 && (
                  <Alert severity="info" sx={{ mt: 2 }}>
                    No connection runs in full mode. Registration needs a Dashboard user that may create APIs and policies; set the connection's mode to full under Settings → Tyk Connections.
                  </Alert>
                )}
              </Grid>
              <Grid item xs={12} md={6}>
                <TextField select fullWidth label="Kind" value={form.kind} onChange={set("kind")} inputProps={{ "data-testid": "kind" }}>
                  <MenuItem value="remote">Remote MCP server (proxy an existing MCP endpoint)</MenuItem>
                  <MenuItem value="rest_to_mcp" disabled={!restSupported}>
                    REST API to MCP {restSupported ? "" : "(needs Tyk 5.15+)"}
                  </MenuItem>
                </TextField>
              </Grid>
            </Grid>
          </Section>
        )}

        {step === 1 && (
          <Section title="Proxy details" description={form.kind === "remote" ? "The remote MCP endpoint the gateway fronts." : "The Tyk OAS API whose operations become MCP tools."}>
            <Grid container spacing={3}>
              <Grid item xs={12} md={6}>
                <TextField fullWidth label="Name" value={form.name} onChange={(e) => setForm((f) => ({ ...f, name: e.target.value, listen_path: f.listen_path || "" }))} inputProps={{ "data-testid": "name" }} required />
              </Grid>
              <Grid item xs={12} md={6}>
                <TextField fullWidth label="Listen path" value={form.listen_path} onChange={set("listen_path")} placeholder={form.name ? `/${slugify(form.name)}/` : "/my-mcp/"} inputProps={{ "data-testid": "listen-path" }} helperText="Lowercase letters, digits and dashes, wrapped in slashes. Left empty, it is derived from the name." />
              </Grid>
              {form.kind === "remote" ? (
                <>
                  <Grid item xs={12}>
                    <TextField fullWidth label="Upstream MCP URL" value={form.upstream_url} onChange={set("upstream_url")} inputProps={{ "data-testid": "upstream-url" }} helperText="The remote MCP server's base URL; the gateway appends /mcp itself (a pasted /mcp suffix is removed). AI Studio never calls it; the Tyk Gateway does." required />
                  </Grid>
                  <Grid item xs={12} md={4}>
                    <TextField fullWidth label="Upstream auth header" value={form.upstream_auth_header_name} onChange={set("upstream_auth_header_name")} inputProps={{ "data-testid": "upstream-header" }} />
                  </Grid>
                  <Grid item xs={12} md={8}>
                    <TextField fullWidth type="password" label="Upstream auth value (optional)" value={form.upstream_auth_token} onChange={set("upstream_auth_token")} inputProps={{ "data-testid": "upstream-token" }} helperText="Sent to the Dashboard once and never stored in AI Studio." autoComplete="new-password" />
                  </Grid>
                  <Grid item xs={12}>
                    <TextField fullWidth label="Allowed tools (optional, comma separated)" value={form.allowed_tools} onChange={set("allowed_tools")} inputProps={{ "data-testid": "allowed-tools" }} helperText="When set, every other tool the upstream server offers is blocked by the gateway." />
                  </Grid>
                </>
              ) : (
                <>
                  <Grid item xs={12}>
                    <TextField select fullWidth label="Source API (Tyk OAS)" value={form.source_api_id} onChange={(e) => loadOperations(e.target.value)} inputProps={{ "data-testid": "source-api" }}>
                      {sourceAPIs.map((a) => (
                        <MenuItem key={a.api_id} value={a.api_id}>
                          {a.name} ({a.listen_path})
                        </MenuItem>
                      ))}
                    </TextField>
                  </Grid>
                  <Grid item xs={12}>
                    {opsLoading ? (
                      <CircularProgress size={20} />
                    ) : form.primitives.length > 0 ? (
                      <Table size="small" data-testid="operations">
                        <TableHead>
                          <TableRow>
                            <TableCell>Expose</TableCell>
                            <TableCell>Operation</TableCell>
                            <TableCell>Tool name</TableCell>
                            <TableCell>Description</TableCell>
                          </TableRow>
                        </TableHead>
                        <TableBody>
                          {form.primitives.map((p, i) => (
                            <TableRow key={`${p.method} ${p.path}`}>
                              <TableCell>
                                <Checkbox checked={p.selected} onChange={(e) => setPrimitive(i, { selected: e.target.checked })} inputProps={{ "data-testid": `op-${i}` }} />
                              </TableCell>
                              <TableCell>
                                <code>
                                  {p.method} {p.path}
                                </code>
                                {p.operation_id && (
                                  <Typography variant="caption" display="block" color="text.secondary">
                                    {p.operation_id}
                                  </Typography>
                                )}
                              </TableCell>
                              <TableCell>
                                <TextField size="small" value={p.name} onChange={(e) => setPrimitive(i, { name: e.target.value })} inputProps={{ "data-testid": `op-name-${i}` }} />
                              </TableCell>
                              <TableCell>
                                <TextField size="small" fullWidth value={p.description} onChange={(e) => setPrimitive(i, { description: e.target.value })} />
                              </TableCell>
                            </TableRow>
                          ))}
                        </TableBody>
                      </Table>
                    ) : (
                      form.source_api_id && <Typography variant="body2">This API has no operations.</Typography>
                    )}
                  </Grid>
                </>
              )}
            </Grid>
          </Section>
        )}

        {step === 2 && (
          <>
            <Section title="Consumer authentication" description="How clients authenticate to the gateway. AI Studio mints keys only for API-key proxies.">
              <Grid container spacing={3}>
                <Grid item xs={12} md={4}>
                  <TextField select fullWidth label="Consumer authentication" value={form.consumer_auth} onChange={set("consumer_auth")} inputProps={{ "data-testid": "consumer-auth" }}>
                    <MenuItem value="auth_token">API key (AI Studio can mint keys)</MenuItem>
                    <MenuItem value="oauth21">OAuth 2.1 (clients bring their own token)</MenuItem>
                    <MenuItem value="keyless">Keyless (no credential)</MenuItem>
                  </TextField>
                </Grid>
                {form.consumer_auth === "oauth21" && (
                  <>
                    <Grid item xs={12} md={4}>
                      <TextField fullWidth label="Authorization servers (comma separated)" value={form.authorization_servers} onChange={set("authorization_servers")} inputProps={{ "data-testid": "authorization-servers" }} />
                    </Grid>
                    <Grid item xs={12} md={4}>
                      <TextField fullWidth label="Scopes (optional)" value={form.scopes_supported} onChange={set("scopes_supported")} />
                    </Grid>
                  </>
                )}
                {form.consumer_auth !== "auth_token" && (
                  <Grid item xs={12} md={8}>
                    <Alert severity={form.consumer_auth === "keyless" ? "warning" : "info"}>
                      {form.consumer_auth === "keyless"
                        ? "Anyone who can reach the gateway can call a keyless proxy. "
                        : "Portal users connect with a token from the authorization server; "}
                      AI Studio does not broker access to this server and the portal shows no Build app button for it.
                      {form.consumer_auth === "keyless" && (
                        <FormControlLabel sx={{ ml: 1 }} control={<Checkbox checked={form.confirm_keyless} onChange={set("confirm_keyless")} inputProps={{ "data-testid": "confirm-keyless" }} />} label="I understand" />
                      )}
                    </Alert>
                  </Grid>
                )}
                {tagOptions.length > 0 && (
                  <Grid item xs={12}>
                    <Typography variant="subtitle2" sx={{ mb: 1 }}>
                      Deployment target
                    </Typography>
                    <Autocomplete
                      multiple
                      options={tagOptions.map((o) => o.tag)}
                      value={form.gateway_tags}
                      onChange={(_, v) => setForm((f) => ({ ...f, gateway_tags: v }))}
                      renderOption={(props, tag) => {
                        const o = tagOptions.find((x) => x.tag === tag);
                        return (
                          <li {...props} key={tag}>
                            <Box>
                              <Typography variant="body2">
                                {tag}
                                {o?.label ? ` · ${o.label}` : ""}
                              </Typography>
                              <Typography variant="caption" color="text.secondary">
                                {o?.verified ? `${o.data_planes.length} data plane(s) reported by MDCB` : "not reported by any data plane"}
                                {o?.description ? ` · ${o.description}` : ""}
                              </Typography>
                            </Box>
                          </li>
                        );
                      }}
                      renderTags={(value, getTagProps) => value.map((tag, index) => <Chip {...getTagProps({ index })} key={tag} size="small" label={tag} color={tagOptions.find((x) => x.tag === tag)?.verified ? "primary" : "default"} />)}
                      renderInput={(params) => <TextField {...params} label="Gateway tags" placeholder="Choose the gateways that should load this proxy" inputProps={{ ...params.inputProps, "data-testid": "gateway-tags" }} />}
                    />
                    {form.gateway_tags.length === 0 && (
                      <FormControlLabel
                        control={<Checkbox checked={form.confirm_no_gateway_tags} onChange={set("confirm_no_gateway_tags")} inputProps={{ "data-testid": "confirm-no-tags" }} />}
                        label="No target: only non-segmented gateways should load this proxy"
                      />
                    )}
                  </Grid>
                )}
              </Grid>
            </Section>

            <Section title="Portal presentation" description="What portal users read about this server, and where it shows up.">
              <Grid container spacing={3}>
                <Grid item xs={12} md={8}>
                  <TextField fullWidth label="Description" value={form.description} onChange={set("description")} inputProps={{ "data-testid": "description" }} />
                </Grid>
                <Grid item xs={12} md={4}>
                  <TextField fullWidth label="Tags (comma separated)" value={form.tags} onChange={set("tags")} />
                </Grid>
                <Grid item xs={12}>
                  <TextField fullWidth multiline rows={3} label="Long description" value={form.long_description} onChange={set("long_description")} />
                </Grid>
                <Grid item xs={12}>
                  <RelationshipPicker
                    label="Tool catalogs this server is published in"
                    itemLabel="catalog"
                    value={form.tool_catalogues}
                    onChange={(next) => setForm((f) => ({ ...f, tool_catalogues: next }))}
                    options={catalogues}
                    idField="id"
                    getOptionLabel={(c) => c.name}
                    helperText="Portal users see this server through the teams granted these catalogs."
                  />
                </Grid>
                <Grid item xs={12} md={4}>
                  <TextField fullWidth type="number" label="Privacy score" value={form.privacy_score} onChange={set("privacy_score")} inputProps={{ "data-testid": "privacy-score", min: 0, max: 100 }} helperText="Required before the server can be published." />
                </Grid>
                <Grid item xs={12} md={4}>
                  <Tooltip title={form.privacy_score === "" ? "Set a privacy score first" : ""}>
                    <FormControlLabel control={<Switch checked={form.publish} onChange={set("publish")} inputProps={{ "data-testid": "publish" }} disabled={form.privacy_score === ""} />} label="Publish to the portal right away" />
                  </Tooltip>
                </Grid>
              </Grid>
            </Section>
          </>
        )}

        {step === 3 && preview && (
          <Section title="Review" description="The definition AI Studio sends to the Dashboard, upstream credentials masked.">
            <Box data-testid="preview">
              <Typography variant="body2" sx={{ mb: 1 }}>
                {preview.dashboard_validated
                  ? "The Tyk Dashboard validated this definition."
                  : "AI Studio validated this definition; the Dashboard validates it when the proxy is created."}{" "}
                Clients will reach the server at <code>{preview.endpoint_url || "(set a gateway base URL on the connection)"}</code>.
              </Typography>
              {(preview.warnings || []).map((wng) => (
                <Alert key={wng} severity="warning" sx={{ mb: 1 }}>
                  {wng}
                </Alert>
              ))}
              <Box component="pre" sx={{ ...preStyle, maxHeight: 400 }} data-testid="preview-definition">
                {JSON.stringify(preview.definition, null, 2)}
              </Box>
            </Box>
          </Section>
        )}

        <Box sx={{ display: "flex", justifyContent: "space-between", mb: 3 }}>
          <SecondaryOutlineButton disabled={step === 0 || busy} onClick={() => setStep(step - 1)} data-testid="back">
            Back
          </SecondaryOutlineButton>
          {step < 3 ? (
            <PrimaryButton variant="contained" onClick={next} disabled={busy} data-testid="next">
              {step === 2 ? "Validate and review" : "Next"}
            </PrimaryButton>
          ) : (
            <PrimaryButton variant="contained" onClick={create} disabled={busy} data-testid="create">
              Create on the Dashboard
            </PrimaryButton>
          )}
        </Box>
      </ContentBox>
    </>
  );
};

export default MCPServerRegister;
