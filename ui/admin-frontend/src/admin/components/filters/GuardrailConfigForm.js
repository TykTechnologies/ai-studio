import React, { useEffect, useMemo, useState } from "react";
import {
  Accordion,
  AccordionDetails,
  AccordionSummary,
  Alert,
  Box,
  Button,
  Checkbox,
  Chip,
  FormControl,
  FormControlLabel,
  FormHelperText,
  Grid,
  InputLabel,
  MenuItem,
  Select,
  TextField,
  Typography,
} from "@mui/material";
import ExpandMoreIcon from "@mui/icons-material/ExpandMore";
import apiClient from "../../utils/apiClient";
import AllowInternalHostSwitch from "../common/AllowInternalHostSwitch";

// The stored shape of a guardrail filter's config. Everything the server
// defaults is left empty here so the saved config shows what the server chose.
export const emptyGuardrailConfig = () => ({
  provider: "builtin",
  connection: {},
  detectors: [],
  exclude: [],
  scope: "",
  on_detect: "block",
  redaction: { style: "placeholder", placeholder: "" },
  fail_mode: "",
  timeout_ms: 0,
  stream: { evaluate_every_chars: 0 },
  block_message: "",
});

// Each action's label and help per direction. On a response filter (LLM
// responses and tool results) the gateway cannot rewrite text, so redact is
// recorded as a log there (scripting/filter_runner.go); the label says so up
// front instead of only after it is picked.
const ACTIONS = [
  {
    value: "block",
    label: "Block",
    help: "Refuse the request and record a compliance event.",
    responseHelp: "Stop the response and record a compliance event.",
  },
  {
    value: "redact",
    label: "Redact",
    responseLabel: "Redact (logs only on responses)",
    help:
      "Rewrite the matched text before it reaches the vendor, and record a compliance event. A finding the provider cannot rewrite (such as a prompt attack) is only logged.",
    responseHelp: "Responses cannot be rewritten: the finding is recorded and the response reaches the client unchanged.",
  },
  {
    // Normalize rejects quarantine on response filters (tool output filters
    // included): a response cannot go on without the text it was flagged for.
    value: "quarantine",
    label: "Quarantine",
    requestOnly: true,
    help:
      "Replace each flagged message or tool result with a placeholder, let the request continue, and record a compliance event.",
  },
  {
    value: "log",
    label: "Log only",
    help: "Let it through and record a compliance event.",
    responseHelp: "Let it through and record a compliance event.",
  },
];

// What each action does to a flagged tool result.
const TOOL_ACTION_HELP = {
  quarantine: "The whole tool result is replaced with the placeholder and the request continues. Its tool call id is kept.",
  block:
    "The request is blocked. The client resends the tool result on its next turn and is blocked again, so the agent stops.",
  redact:
    "The matched text inside the tool result is rewritten. Findings with no location in the text (such as a prompt attack) are logged only.",
  log: "The tool result goes through unchanged and a compliance event is recorded.",
};

export const actionsFor =(responseFilter) => ACTIONS.filter((a) => !(responseFilter && a.requestOnly));

export const actionLabel = (value) => (ACTIONS.find((a) => a.value === value) || {}).label || value;

// The action a flagged tool result gets (guardrails.Config.ActionFor): the
// tool_results override, else the filter's action with block turned into
// quarantine, because a client resends a tool result every turn and blocking
// on it would stop an agent for good.
export const effectiveToolAction = (config) => {
  const override = config.tool_results && config.tool_results.on_detect;
  if (override) return override;
  return config.on_detect === "block" ? "quarantine" : config.on_detect;
};

// A missing tool_results.scan means scan: stored filters from before tool
// results were scanned do scan them.
export const scansToolResults = (config) => !(config.tool_results && config.tool_results.scan === false);

// Tool calls are scanned only when tool_calls.scan is true.
export const scansToolCalls = (config) => !!(config.tool_calls && config.tool_calls.scan === true);

// The action a flagged tool call gets (guardrails.Config.ActionFor): the
// tool_calls override, else block for a Block filter and log otherwise, since
// a call cannot be redacted or quarantined.
export const effectiveToolCallAction = (config) => {
  const override = config.tool_calls && config.tool_calls.on_detect;
  if (override) return override;
  return config.on_detect === "block" ? "block" : "log";
};

// Tool definitions are scanned only when tool_definitions.scan is true.
export const scansToolDefinitions = (config) => !!(config.tool_definitions && config.tool_definitions.scan === true);

// The action a flagged tool definition gets: the override, else remove for a
// Block or Quarantine filter, else log.
export const effectiveToolDefinitionAction = (config) => {
  const override = config.tool_definitions && config.tool_definitions.on_detect;
  if (override) return override;
  return config.on_detect === "block" || config.on_detect === "quarantine" ? "remove" : "log";
};

// Shadow (don't wait) is allowed only when every action the filter can take
// is log (guardrails.Config shadowAllowed).
export const shadowAllowed = (config) =>
  config.on_detect === "log" &&
  effectiveToolAction(config) === "log" &&
  effectiveToolCallAction(config) === "log" &&
  effectiveToolDefinitionAction(config) === "log";

export const DEFAULT_QUARANTINE_PLACEHOLDER = "[content removed by policy: {{detector}} detected in {{source}}]";

// Budget defaults and bounds (guardrails/config.go). Budgets apply to request
// filters with a remote provider only.
export const BUDGET_DEFAULTS = {
  max_scan_chars: 262144,
  max_pieces: 256,
  single_call_chars: 32768,
};
const MAX_BUDGET_SCAN_CHARS = 16 << 20;
const MAX_BUDGET_PIECES = 10000;
const MAX_BUDGET_TIME_MS = 300000;
const DEFAULT_TIMEOUT_MS = 2000;

const defaultBudgetTimeMs = (timeoutMs) => Math.max(timeoutMs, Math.min(4 * timeoutMs, MAX_BUDGET_TIME_MS));

// budgetErrors mirrors Budget.normalize so a bad value is named before the
// server rejects the save. It returns a map of field to message, empty when
// the budget is valid.
export const budgetErrors = (config) => {
  const budget = (config && config.budget) || {};
  const timeout = (config && config.timeout_ms) || DEFAULT_TIMEOUT_MS;
  const errs = {};
  const set = (v) => v !== undefined && v !== null && v !== "";
  if (set(budget.max_scan_chars) && (budget.max_scan_chars < 1 || budget.max_scan_chars > MAX_BUDGET_SCAN_CHARS)) {
    errs.max_scan_chars = `Between 1 and ${MAX_BUDGET_SCAN_CHARS}`;
  }
  if (set(budget.max_pieces) && (budget.max_pieces < 1 || budget.max_pieces > MAX_BUDGET_PIECES)) {
    errs.max_pieces = `Between 1 and ${MAX_BUDGET_PIECES}`;
  }
  const maxTime = Math.max(MAX_BUDGET_TIME_MS, timeout);
  if (set(budget.time_ms) && (budget.time_ms < timeout || budget.time_ms > maxTime)) {
    errs.time_ms = `Between the timeout (${timeout}) and ${maxTime}`;
  }
  const maxScan = set(budget.max_scan_chars) ? budget.max_scan_chars : BUDGET_DEFAULTS.max_scan_chars;
  if (set(budget.single_call_chars) && (budget.single_call_chars < 1 || budget.single_call_chars > maxScan)) {
    errs.single_call_chars = `Between 1 and the max characters per segment (${maxScan})`;
  }
  return errs;
};

// withoutEmpty drops keys whose value is undefined, so a setting the admin
// cleared is absent from the saved config and the server default applies.
const withoutEmpty = (obj) => {
  const out = {};
  Object.entries(obj || {}).forEach(([k, v]) => {
    if (v !== undefined && v !== "") out[k] = v;
  });
  return out;
};

const SCOPES = [
  { value: "", label: "All user messages (default)" },
  { value: "last_user", label: "User text of the current turn" },
  { value: "all_user", label: "All user messages" },
  { value: "system_and_user", label: "System and user messages" },
  { value: "all_messages", label: "Every message, including assistant turns" },
];

// A connection field that names a URL is dialled by the gateway, not the
// browser, so "localhost" in it does not mean the admin's machine.
const isUrlField = (f) =>
  !f.secret &&
  (f.name === "endpoint" || /url$/i.test(f.name || "") || /^https?:\/\//i.test(f.example || ""));

const URL_FIELD_HELP =
  "Resolved from the gateway's network, not your browser: localhost means the gateway container itself.";

const connectionFieldHelp = (f) => {
  if (f.secret) {
    return `${f.description || ""} Reference a stored secret as $SECRET/name (environment variables as $ENV/NAME only when your operator allows them).`.trim();
  }
  if (isUrlField(f)) {
    return `${f.description || ""} ${URL_FIELD_HELP}`.trim();
  }
  return f.description;
};

const GuardrailConfigForm = ({ value, onChange, responseFilter = false, errors = {} }) => {
  const [providers, setProviders] = useState([]);
  const [loadError, setLoadError] = useState("");
  const [newDetector, setNewDetector] = useState("");

  useEffect(() => {
    let cancelled = false;
    apiClient
      .get("/filters/guardrail-providers")
      .then((response) => {
        if (!cancelled) setProviders(Array.isArray(response.data) ? response.data : []);
      })
      .catch((error) => {
        console.error("Error loading guardrail providers", error);
        if (!cancelled) setLoadError("Failed to load the guardrail provider list");
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const config = useMemo(() => ({ ...emptyGuardrailConfig(), ...(value || {}) }), [value]);
  const spec = providers.find((p) => p.name === config.provider);

  const update = (patch) => onChange({ ...config, ...patch });

  const detectorEnabled = (name) => config.detectors.some((d) => d.name === name);
  const detectorThreshold = (name) => {
    const d = config.detectors.find((x) => x.name === name);
    return d && d.threshold !== undefined && d.threshold !== null ? d.threshold : "";
  };

  const toggleDetector = (name, on) => {
    const detectors = on
      ? [...config.detectors.filter((d) => d.name !== name), { name }]
      : config.detectors.filter((d) => d.name !== name);
    update({ detectors });
  };

  const setThreshold = (name, raw) => {
    const detectors = config.detectors.map((d) => {
      if (d.name !== name) return d;
      if (raw === "") {
        const { threshold, ...rest } = d;
        return rest;
      }
      return { ...d, threshold: Number(raw) };
    });
    update({ detectors });
  };

  const addDetector = () => {
    const name = newDetector.trim();
    if (!name || detectorEnabled(name)) return;
    update({ detectors: [...config.detectors, { name }] });
    setNewDetector("");
  };

  const handleProviderChange = (e) => {
    // A new provider has its own detectors and connection fields; keep the
    // policy (action, fail mode, timeout) since that is about the filter.
    update({ provider: e.target.value, detectors: [], connection: {}, exclude: [] });
  };

  // tool_results, quarantine, budget and cache carry only what the admin set:
  // a key left out takes the server default, and a stored filter without
  // tool_results scans tool results, so a cleared setting is removed rather
  // than written as its default.
  const setToolResults = (patch) => {
    const next = withoutEmpty({ ...(config.tool_results || {}), ...patch });
    if (next.scan === true) delete next.scan;
    update({ tool_results: Object.keys(next).length ? next : undefined });
  };

  // tool_calls carries only what the admin set; scanning is off unless
  // scan is true, so an unchecked box removes the key.
  const setToolCalls = (patch) => {
    const next = withoutEmpty({ ...(config.tool_calls || {}), ...patch });
    if (next.scan === false) delete next.scan;
    if (next.include_system === false) delete next.include_system;
    update({ tool_calls: Object.keys(next).length ? next : undefined });
  };

  // evidence carries only what the admin set; the excerpt is off unless set.
  const setEvidence = (patch) => {
    const next = withoutEmpty({ ...(config.evidence || {}), ...patch });
    if (!next.excerpt) delete next.excerpt_chars;
    update({ evidence: Object.keys(next).length ? next : undefined });
  };

  const setToolDefinitions = (patch) => {
    const next = withoutEmpty({ ...(config.tool_definitions || {}), ...patch });
    if (next.scan === false) delete next.scan;
    update({ tool_definitions: Object.keys(next).length ? next : undefined });
  };

  const setQuarantinePlaceholder = (placeholder) => {
    const next = withoutEmpty({ ...(config.quarantine || {}), placeholder });
    update({ quarantine: Object.keys(next).length ? next : undefined });
  };

  const setBudget = (field, raw) => {
    const next = withoutEmpty({ ...(config.budget || {}), [field]: raw === "" ? undefined : Number(raw) });
    update({ budget: Object.keys(next).length ? next : undefined });
  };

  const catalogueDetectors = spec ? spec.detectors : [];
  const customDetectors = config.detectors.filter((d) => !catalogueDetectors.some((c) => c.name === d.name));
  const redactUnsupported = spec && !spec.redacts;
  const selectedAction = ACTIONS.find((a) => a.value === config.on_detect) || {};
  const scanTools = scansToolResults(config);
  const toolAction = effectiveToolAction(config);
  const toolOverride = (config.tool_results && config.tool_results.on_detect) || "";
  const showQuarantine = config.on_detect === "quarantine" || (scanTools && toolAction === "quarantine");
  const scanCalls = scansToolCalls(config);
  const callOverride = (config.tool_calls && config.tool_calls.on_detect) || "";
  const scanDefs = scansToolDefinitions(config);
  const defOverride = (config.tool_definitions && config.tool_definitions.on_detect) || "";
  const remoteProvider = spec && !spec.local;
  const budget = config.budget || {};
  const budgetErr = budgetErrors(config);
  const timeoutMs = config.timeout_ms || DEFAULT_TIMEOUT_MS;

  return (
    <Box>
      {loadError && (
        <Alert severity="error" sx={{ mb: 2 }}>
          {loadError}
        </Alert>
      )}
      <Grid container spacing={3}>
        <Grid item xs={12} md={6}>
          <FormControl fullWidth error={!!errors.provider}>
            <InputLabel id="guardrail-provider-label">Provider</InputLabel>
            <Select
              labelId="guardrail-provider-label"
              label="Provider"
              value={providers.some((p) => p.name === config.provider) ? config.provider : ""}
              onChange={handleProviderChange}
              inputProps={{ "data-testid": "guardrail-provider" }}
            >
              {providers.map((p) => (
                <MenuItem key={p.name} value={p.name}>
                  {p.display_name}
                  {!p.available ? " (not available in this edition)" : ""}
                </MenuItem>
              ))}
            </Select>
            <FormHelperText>{errors.provider || (spec ? spec.description : "")}</FormHelperText>
          </FormControl>
          {spec && !spec.available && (
            <Alert severity="warning" sx={{ mt: 1 }}>
              This provider is not implemented in this edition. The filter can be saved but will not be enforced.
            </Alert>
          )}
        </Grid>

        <Grid item xs={12} md={6}>
          <FormControl fullWidth>
            <InputLabel id="guardrail-action-label">On detection</InputLabel>
            <Select
              labelId="guardrail-action-label"
              label="On detection"
              value={config.on_detect}
              onChange={(e) => update({ on_detect: e.target.value })}
              inputProps={{ "data-testid": "guardrail-action" }}
            >
              {actionsFor(responseFilter).map((a) => (
                <MenuItem key={a.value} value={a.value} disabled={a.value === "redact" && redactUnsupported}>
                  {(responseFilter && a.responseLabel) || a.label}
                  {a.value === "redact" && redactUnsupported ? " (provider cannot redact)" : ""}
                </MenuItem>
              ))}
            </Select>
            <FormHelperText>{responseFilter ? selectedAction.responseHelp : selectedAction.help}</FormHelperText>
          </FormControl>
          {config.on_detect === "redact" && responseFilter && (
            <Alert severity="warning" sx={{ mt: 1 }} data-testid="guardrail-response-redact-warning">
              Redact does not change responses. On a response filter, a detection is recorded as a compliance event and
              the response (or tool result) reaches the client unchanged. Choose Block to stop it.
            </Alert>
          )}
        </Grid>

        {spec && spec.connection_fields && spec.connection_fields.length > 0 && (
          <Grid item xs={12}>
            <Typography variant="subtitle2" gutterBottom>
              Connection
            </Typography>
            <Grid container spacing={2}>
              {spec.connection_fields.map((f) => (
                <Grid item xs={12} md={6} key={f.name}>
                  <TextField
                    fullWidth
                    label={f.label + (f.required ? " *" : "")}
                    value={config.connection[f.name] || ""}
                    onChange={(e) => update({ connection: { ...config.connection, [f.name]: e.target.value } })}
                    placeholder={f.example || ""}
                    error={!!(errors.connection && errors.connection[f.name])}
                    helperText={(errors.connection && errors.connection[f.name]) || connectionFieldHelp(f)}
                    inputProps={{ "data-testid": `guardrail-connection-${f.name}` }}
                  />
                </Grid>
              ))}
              <Grid item xs={12}>
                <AllowInternalHostSwitch
                  checked={!!config.allow_internal_host}
                  onChange={(v) => update({ allow_internal_host: v })}
                  hostLabel="guardrail's endpoints"
                />
              </Grid>
            </Grid>
          </Grid>
        )}

        <Grid item xs={12}>
          <Typography variant="subtitle2" gutterBottom>
            Detectors *
          </Typography>
          {errors.detectors && <FormHelperText error>{errors.detectors}</FormHelperText>}
          <Grid container spacing={1}>
            {catalogueDetectors.map((d) => {
              const on = detectorEnabled(d.name);
              return (
                <Grid item xs={12} md={6} key={d.name}>
                  <Box sx={{ display: "flex", alignItems: "flex-start", gap: 1 }}>
                    <FormControlLabel
                      control={
                        <Checkbox
                          checked={on}
                          onChange={(e) => toggleDetector(d.name, e.target.checked)}
                          inputProps={{ "data-testid": `guardrail-detector-${d.name}` }}
                        />
                      }
                      label={
                        <Box>
                          <Typography variant="body2">{d.label || d.name}</Typography>
                          {d.description && (
                            <Typography variant="caption" color="text.secondary">
                              {d.description}
                            </Typography>
                          )}
                        </Box>
                      }
                      sx={{ flex: 1, alignItems: "flex-start" }}
                    />
                    {on && d.has_threshold && (
                      <TextField
                        size="small"
                        type="number"
                        label="Threshold"
                        value={detectorThreshold(d.name)}
                        onChange={(e) => setThreshold(d.name, e.target.value)}
                        helperText={d.threshold_hint}
                        sx={{ width: 160 }}
                        inputProps={{ step: "any", "data-testid": `guardrail-threshold-${d.name}` }}
                      />
                    )}
                  </Box>
                </Grid>
              );
            })}
          </Grid>
          {spec && spec.open_detectors && (
            <Box sx={{ mt: 2, display: "flex", gap: 1, alignItems: "center", flexWrap: "wrap" }}>
              <TextField
                size="small"
                label="Add detector"
                value={newDetector}
                onChange={(e) => setNewDetector(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Enter") {
                    e.preventDefault();
                    addDetector();
                  }
                }}
                helperText="A detector name the provider defines, e.g. pii/email or a custom category"
                inputProps={{ "data-testid": "guardrail-add-detector" }}
              />
              <Button variant="outlined" onClick={addDetector} disabled={!newDetector.trim()}>
                Add
              </Button>
              {customDetectors.map((d) => (
                <Chip key={d.name} label={d.name} onDelete={() => toggleDetector(d.name, false)} />
              ))}
            </Box>
          )}
          {config.provider === "builtin" && (
            <TextField
              fullWidth
              sx={{ mt: 2 }}
              label="Exclude patterns"
              value={(config.exclude || []).join(", ")}
              onChange={(e) =>
                update({
                  exclude: e.target.value
                    .split(",")
                    .map((s) => s.trim())
                    .filter(Boolean),
                })
              }
              placeholder="pii.ipv4, pii.street_address"
              helperText="Comma-separated pattern ids to leave out of an enabled category"
              inputProps={{ "data-testid": "guardrail-exclude" }}
            />
          )}
        </Grid>

        {(config.on_detect === "redact" || (scanTools && toolOverride === "redact")) && !responseFilter && (
          <>
            <Grid item xs={12} md={4}>
              <FormControl fullWidth>
                <InputLabel id="guardrail-redaction-style-label">Redaction style</InputLabel>
                <Select
                  labelId="guardrail-redaction-style-label"
                  label="Redaction style"
                  value={config.redaction.style || "placeholder"}
                  onChange={(e) => update({ redaction: { ...config.redaction, style: e.target.value } })}
                >
                  <MenuItem value="placeholder">Placeholder</MenuItem>
                  <MenuItem value="mask">Mask (same length)</MenuItem>
                  <MenuItem value="hash">Short hash</MenuItem>
                </Select>
              </FormControl>
            </Grid>
            {(config.redaction.style || "placeholder") === "placeholder" && (
              <Grid item xs={12} md={8}>
                <TextField
                  fullWidth
                  label="Placeholder"
                  value={config.redaction.placeholder || ""}
                  onChange={(e) => update({ redaction: { ...config.redaction, placeholder: e.target.value } })}
                  placeholder="[REDACTED:{{type}}]"
                  helperText="{{type}} is the detector, e.g. EMAIL; {{category}} is SECRETS, PII, ... Leave empty for the default."
                />
              </Grid>
            )}
          </>
        )}

        {!responseFilter && (
          <Grid item xs={12} md={6}>
            <FormControl fullWidth>
              <InputLabel id="guardrail-scope-label" shrink>Messages to inspect</InputLabel>
              <Select
                labelId="guardrail-scope-label"
                label="Messages to inspect"
                displayEmpty
                notched
                value={config.scope || ""}
                onChange={(e) => update({ scope: e.target.value })}
              >
                {SCOPES.map((s) => (
                  <MenuItem key={s.value || "default"} value={s.value}>
                    {s.label}
                  </MenuItem>
                ))}
              </Select>
              <FormHelperText>
                Applies to LLM requests. The current turn is everything since the model last spoke. Tool results are
                inspected across the whole history whatever you choose here, unless tool-result scanning is off. Chat
                messages and tool calls always inspect the whole text.
              </FormHelperText>
            </FormControl>
          </Grid>
        )}

        {!responseFilter && (
          <Grid item xs={12}>
            <Typography variant="subtitle2" gutterBottom>
              Tool results
            </Typography>
            <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
              Tool output (a fetched web page, a file, an MCP or API response) is the main way instructions reach a
              model without the user typing them. Clients resend tool results on every turn, so blocking a request for
              one stops an agent for good: a Block filter quarantines a flagged tool result instead, unless you choose
              Block below.
            </Typography>
            <Grid container spacing={2} alignItems="flex-start">
              <Grid item xs={12} md={4}>
                <FormControlLabel
                  control={
                    <Checkbox
                      checked={scanTools}
                      onChange={(e) => setToolResults({ scan: e.target.checked })}
                      inputProps={{ "data-testid": "guardrail-tool-results-scan" }}
                    />
                  }
                  label="Scan tool results"
                />
                <FormHelperText>
                  On by default, including for filters saved before tool results were scanned. Off, tool results are
                  inspected only under "Every message".
                </FormHelperText>
              </Grid>
              <Grid item xs={12} md={8}>
                <FormControl fullWidth disabled={!scanTools}>
                  <InputLabel id="guardrail-tool-action-label" shrink>
                    Action on tool results
                  </InputLabel>
                  <Select
                    labelId="guardrail-tool-action-label"
                    label="Action on tool results"
                    displayEmpty
                    notched
                    value={toolOverride}
                    onChange={(e) => setToolResults({ on_detect: e.target.value })}
                    inputProps={{ "data-testid": "guardrail-tool-action" }}
                  >
                    <MenuItem value="">
                      {config.on_detect === "block"
                        ? "Quarantine (default for Block filters)"
                        : `Same as the filter (${actionLabel(config.on_detect)})`}
                    </MenuItem>
                    {ACTIONS.map((a) => (
                      <MenuItem key={a.value} value={a.value} disabled={a.value === "redact" && redactUnsupported}>
                        {a.label}
                        {a.value === "redact" && redactUnsupported ? " (provider cannot redact)" : ""}
                      </MenuItem>
                    ))}
                  </Select>
                  <FormHelperText>{TOOL_ACTION_HELP[toolAction]}</FormHelperText>
                </FormControl>
              </Grid>
              {scanTools && remoteProvider && (
                <Grid item xs={12}>
                  <Alert severity="info" data-testid="guardrail-tool-results-remote">
                    This provider runs outside the gateway, so tool output is sent to it, and that can include file
                    contents, database rows or credentials an agent read. Turn scanning off if that data must not leave.
                  </Alert>
                </Grid>
              )}
            </Grid>
          </Grid>
        )}

        <Grid item xs={12}>
          <Typography variant="subtitle2" gutterBottom>
            Tool calls
          </Typography>
          <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
            The arguments of the tools the model asks to run, in the request history and in responses. An injection
            that reached the model shows up here as an action, such as data sent to an address the attacker chose. Off
            by default.
          </Typography>
          <Grid container spacing={2} alignItems="flex-start">
            <Grid item xs={12} md={4}>
              <FormControlLabel
                control={
                  <Checkbox
                    checked={scanCalls}
                    onChange={(e) => setToolCalls({ scan: e.target.checked })}
                    inputProps={{ "data-testid": "guardrail-tool-calls-scan" }}
                  />
                }
                label="Scan tool calls"
              />
              <FormHelperText>
                {responseFilter
                  ? "On a response, a flagged call is never sent to the client, so it never runs."
                  : "On a request the call has already run on the client: block stops the conversation, log records it."}
              </FormHelperText>
            </Grid>
            <Grid item xs={12} md={4}>
              <FormControl fullWidth disabled={!scanCalls}>
                <InputLabel id="guardrail-tool-call-action-label" shrink>
                  Action on tool calls
                </InputLabel>
                <Select
                  labelId="guardrail-tool-call-action-label"
                  label="Action on tool calls"
                  displayEmpty
                  notched
                  value={callOverride}
                  onChange={(e) => setToolCalls({ on_detect: e.target.value })}
                  inputProps={{ "data-testid": "guardrail-tool-call-action" }}
                >
                  <MenuItem value="">{`Default (${actionLabel(effectiveToolCallAction(config))})`}</MenuItem>
                  <MenuItem value="block">{actionLabel("block")}</MenuItem>
                  <MenuItem value="log">{actionLabel("log")}</MenuItem>
                </Select>
                <FormHelperText>A tool call cannot be redacted or quarantined: it is blocked or logged.</FormHelperText>
              </FormControl>
            </Grid>
            <Grid item xs={12} md={4}>
              <FormControlLabel
                control={
                  <Checkbox
                    checked={!!(config.tool_calls && config.tool_calls.include_system)}
                    disabled={!scanCalls}
                    onChange={(e) => setToolCalls({ include_system: e.target.checked })}
                    inputProps={{ "data-testid": "guardrail-tool-calls-include-system" }}
                  />
                }
                label="Include the system prompt in the call's context"
              />
              <FormHelperText>
                Providers that judge a call against the user's goal (the HTTP classifier) get the latest user text with
                each call; this adds the system prompt.
              </FormHelperText>
            </Grid>
            {scanCalls && remoteProvider && (
              <Grid item xs={12}>
                <Alert severity="info" data-testid="guardrail-tool-calls-remote">
                  This provider runs outside the gateway, so tool arguments are sent to it, and they can hold data an
                  agent read.
                </Alert>
              </Grid>
            )}
          </Grid>
        </Grid>

        {!responseFilter && (
          <Grid item xs={12}>
            <Typography variant="subtitle2" gutterBottom>
              Tool definitions
            </Typography>
            <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
              The tools a request offers the model: name, description and parameter schema. Definitions often come
              from MCP servers or API specs a third party wrote, and a description can carry instructions the model
              follows. Each definition is scanned once. Off by default.
            </Typography>
            <Grid container spacing={2} alignItems="flex-start">
              <Grid item xs={12} md={4}>
                <FormControlLabel
                  control={
                    <Checkbox
                      checked={scanDefs}
                      onChange={(e) => setToolDefinitions({ scan: e.target.checked })}
                      inputProps={{ "data-testid": "guardrail-tool-definitions-scan" }}
                    />
                  }
                  label="Scan tool definitions"
                />
              </Grid>
              <Grid item xs={12} md={8}>
                <FormControl fullWidth disabled={!scanDefs}>
                  <InputLabel id="guardrail-tool-def-action-label" shrink>
                    Action on tool definitions
                  </InputLabel>
                  <Select
                    labelId="guardrail-tool-def-action-label"
                    label="Action on tool definitions"
                    displayEmpty
                    notched
                    value={defOverride}
                    onChange={(e) => setToolDefinitions({ on_detect: e.target.value })}
                    inputProps={{ "data-testid": "guardrail-tool-definition-action" }}
                  >
                    <MenuItem value="">{`Default (${effectiveToolDefinitionAction(config) === "remove" ? "Remove the tool" : actionLabel("log")})`}</MenuItem>
                    <MenuItem value="remove">Remove the tool</MenuItem>
                    <MenuItem value="block">{actionLabel("block")}</MenuItem>
                    <MenuItem value="log">{actionLabel("log")}</MenuItem>
                  </Select>
                  <FormHelperText>
                    Remove drops the tool from the request and lets it go on (the request is blocked when that would
                    leave no tool, or when it forces the removed tool). Tools AI Studio serves over MCP and a chat's
                    tools are screened when their list is built.
                  </FormHelperText>
                </FormControl>
              </Grid>
            </Grid>
          </Grid>
        )}

        {(shadowAllowed(config) || config.shadow) && (
          <Grid item xs={12}>
            <FormControlLabel
              control={
                <Checkbox
                  checked={!!config.shadow}
                  onChange={(e) => update({ shadow: e.target.checked || undefined })}
                  inputProps={{ "data-testid": "guardrail-shadow" }}
                />
              }
              label="Shadow (don't wait)"
            />
            <FormHelperText>
              The traffic goes on at once and the provider runs in the background, so a slow provider adds no latency.
              Detections are recorded as compliance events when it finishes; when the background queue is full, the
              check is skipped and recorded as not scanned. For a log-only rollout.
            </FormHelperText>
            {config.shadow && !shadowAllowed(config) && (
              <Alert severity="error" sx={{ mt: 1 }} data-testid="guardrail-shadow-invalid">
                Shadow needs every action to be Log: the filter's, the tool results' and the tool calls'.
              </Alert>
            )}
          </Grid>
        )}

        {showQuarantine && !responseFilter && (
          <Grid item xs={12}>
            <TextField
              fullWidth
              label="Quarantine placeholder"
              value={(config.quarantine && config.quarantine.placeholder) || ""}
              onChange={(e) => setQuarantinePlaceholder(e.target.value)}
              placeholder={DEFAULT_QUARANTINE_PLACEHOLDER}
              // The placeholder is the server default; an unshrunk label hides it.
              InputLabelProps={{ shrink: true }}
              helperText="Replaces a quarantined message or tool result. {{detector}} and {{category}} are what flagged it, {{source}} where it came from (tool result from read_file, user message), {{tool_name}} the tool. Leave empty for the default."
              inputProps={{ maxLength: 1000, "data-testid": "guardrail-quarantine-placeholder" }}
            />
          </Grid>
        )}

        <Grid item xs={12} md={6}>
          <FormControl fullWidth>
            <InputLabel id="guardrail-fail-mode-label" shrink>If the provider fails</InputLabel>
            <Select
              labelId="guardrail-fail-mode-label"
              label="If the provider fails"
              displayEmpty
              notched
              value={config.fail_mode || ""}
              onChange={(e) => update({ fail_mode: e.target.value })}
              inputProps={{ "data-testid": "guardrail-fail-mode" }}
            >
              {/* Server default: closed for request filters, open for response filters. */}
              <MenuItem value="">{responseFilter ? "Let the response through (default)" : "Block the request (default)"}</MenuItem>
              <MenuItem value="closed">{responseFilter ? "Block the response" : "Block the request"}</MenuItem>
              <MenuItem value="open">{responseFilter ? "Let the response through" : "Let the request through"}</MenuItem>
            </Select>
            <FormHelperText>
              Fail closed blocks every request while the provider is unreachable. A timeout or error records a compliance event either way.
            </FormHelperText>
          </FormControl>
        </Grid>

        <Grid item xs={12} md={4}>
          <TextField
            fullWidth
            type="number"
            label="Timeout (ms)"
            value={config.timeout_ms || ""}
            onChange={(e) => update({ timeout_ms: e.target.value === "" ? 0 : Number(e.target.value) })}
            placeholder="2000"
            helperText="Per provider call. Empty uses the default of 2000."
          />
        </Grid>

        {responseFilter && (
          <Grid item xs={12} md={4}>
            <TextField
              fullWidth
              type="number"
              label="Streaming: evaluate every N characters"
              value={config.stream?.evaluate_every_chars || ""}
              onChange={(e) =>
                update({ stream: { evaluate_every_chars: e.target.value === "" ? 0 : Number(e.target.value) } })
              }
              placeholder={config.provider === "builtin" ? "250" : "1000"}
              helperText="How often the accumulated response is re-checked while streaming. Empty uses the provider default."
            />
          </Grid>
        )}

        <Grid item xs={12} md={responseFilter ? 4 : 8}>
          <TextField
            fullWidth
            label="Block message"
            value={config.block_message || ""}
            onChange={(e) => update({ block_message: e.target.value })}
            placeholder={responseFilter ? "Response blocked by policy" : "Request blocked by policy"}
            helperText="Shown to the caller when the guardrail blocks. Findings are never included."
          />
        </Grid>

        <Grid item xs={12}>
          <FormControlLabel
            control={
              <Checkbox
                checked={!!config.evidence?.excerpt}
                onChange={(e) => setEvidence({ excerpt: e.target.checked || undefined })}
                inputProps={{ "data-testid": "guardrail-evidence-excerpt" }}
              />
            }
            label="Show flagged text in compliance events"
          />
          <FormHelperText>
            Each compliance event names the flagged messages and tool results and the SHA-256 of the text the provider
            was sent. With this on it also records a short excerpt of that text, with credentials and personal data
            masked. Never recorded for an LLM set not to log bodies.
          </FormHelperText>
          <FormControlLabel
            sx={{ mt: 1, display: "flex" }}
            control={
              <Checkbox
                checked={!!config.evidence?.share_with_app_owners}
                onChange={(e) => setEvidence({ share_with_app_owners: e.target.checked || undefined })}
                inputProps={{ "data-testid": "guardrail-share-with-owners" }}
              />
            }
            label="Show this filter's blocks to app owners"
          />
          <FormHelperText>
            In the portal, an app's owners see the requests of their app this filter blocked, quarantined or changed:
            when, why (the reason the caller got), the filter and the request ID. Never the flagged text.
          </FormHelperText>
          {config.evidence?.excerpt && (
            <TextField
              sx={{ mt: 1, maxWidth: 260 }}
              type="number"
              label="Excerpt length (characters)"
              value={config.evidence?.excerpt_chars || ""}
              onChange={(e) => setEvidence({ excerpt_chars: e.target.value === "" ? undefined : Number(e.target.value) })}
              placeholder="200"
              helperText="Up to 1000. Empty uses 200."
              inputProps={{ min: 1, max: 1000, "data-testid": "guardrail-evidence-chars" }}
            />
          )}
        </Grid>

        {!responseFilter && (
          <Grid item xs={12}>
            {errors.budget && <FormHelperText error>{errors.budget}</FormHelperText>}
            <Accordion
              disableGutters
              variant="outlined"
              defaultExpanded={!!config.budget || config.cache === false || Object.keys(budgetErr).length > 0}
            >
              <AccordionSummary expandIcon={<ExpandMoreIcon />} aria-controls="guardrail-advanced-content">
                <Typography variant="subtitle2">Advanced: verdict cache and budgets</Typography>
              </AccordionSummary>
              <AccordionDetails id="guardrail-advanced-content">
                <FormControlLabel
                  control={
                    <Checkbox
                      checked={config.cache !== false}
                      onChange={(e) => update({ cache: e.target.checked ? undefined : false })}
                      inputProps={{ "data-testid": "guardrail-cache" }}
                    />
                  }
                  label="Reuse verdicts for text already scanned"
                />
                <FormHelperText sx={{ mb: 2 }}>
                  History an agent resends is answered from the cache, so the provider sees each message and tool
                  result once. Turn this off for a provider whose verdict depends on more than the text, such as an HTTP
                  classifier that reads the request metadata.
                </FormHelperText>
                {remoteProvider ? (
                  <Grid container spacing={2}>
                    <Grid item xs={12}>
                      <Typography variant="body2" color="text.secondary">
                        Budgets bound what one request spends on this provider. Text beyond a budget is never treated as
                        clean: it is flagged as unscanned and the filter's action applies. Empty uses the default.
                      </Typography>
                    </Grid>
                    {[
                      {
                        field: "max_scan_chars",
                        label: "Max characters per segment",
                        placeholder: String(BUDGET_DEFAULTS.max_scan_chars),
                        help: "Most characters of one message or tool result sent for scanning.",
                      },
                      {
                        field: "max_pieces",
                        label: "Max provider pieces per request",
                        placeholder: String(BUDGET_DEFAULTS.max_pieces),
                        help: "Segments, or parts of long ones, sent per request. Cached verdicts do not count.",
                      },
                      {
                        field: "time_ms",
                        label: "Time budget (ms)",
                        placeholder: String(defaultBudgetTimeMs(timeoutMs)),
                        help: "Wall time for all of one request's provider calls. At least the timeout; default 4 × timeout.",
                      },
                      {
                        field: "single_call_chars",
                        label: "Fail mode applies up to (characters)",
                        placeholder: String(Math.min(BUDGET_DEFAULTS.single_call_chars, budget.max_scan_chars || BUDGET_DEFAULTS.max_scan_chars)),
                        help: "Largest request whose provider failure the fail mode decides. Above it, text whose call failed is flagged as unscanned.",
                      },
                    ].map((b) => (
                      <Grid item xs={12} md={6} key={b.field}>
                        <TextField
                          fullWidth
                          type="number"
                          label={b.label}
                          value={budget[b.field] ?? ""}
                          onChange={(e) => setBudget(b.field, e.target.value)}
                          placeholder={b.placeholder}
                          InputLabelProps={{ shrink: true }}
                          error={!!budgetErr[b.field]}
                          helperText={budgetErr[b.field] || b.help}
                          inputProps={{ min: 1, "data-testid": `guardrail-budget-${b.field}` }}
                        />
                      </Grid>
                    ))}
                  </Grid>
                ) : (
                  <Typography variant="body2" color="text.secondary">
                    This provider runs in-process and scans every segment in full, so request budgets do not apply.
                  </Typography>
                )}
              </AccordionDetails>
            </Accordion>
          </Grid>
        )}
      </Grid>
    </Box>
  );
};

export default GuardrailConfigForm;
