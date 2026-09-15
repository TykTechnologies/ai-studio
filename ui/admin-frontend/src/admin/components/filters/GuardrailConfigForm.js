import React, { useEffect, useMemo, useState } from "react";
import {
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
import apiClient from "../../utils/apiClient";

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

const ACTIONS = [
  { value: "block", label: "Block", help: "Refuse the request (or stop the response) and record a compliance event." },
  { value: "redact", label: "Redact", help: "Rewrite the matched text before it reaches the vendor, and record a compliance event." },
  { value: "log", label: "Log only", help: "Let it through and record a compliance event." },
];

const SCOPES = [
  { value: "", label: "All user messages (default)" },
  { value: "last_user", label: "Last user message only" },
  { value: "all_user", label: "All user messages" },
  { value: "system_and_user", label: "System and user messages" },
  { value: "all_messages", label: "Every message, including assistant and tool turns" },
];

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

  const catalogueDetectors = spec ? spec.detectors : [];
  const customDetectors = config.detectors.filter((d) => !catalogueDetectors.some((c) => c.name === d.name));
  const redactUnsupported = spec && !spec.redacts;

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
              {ACTIONS.map((a) => (
                <MenuItem key={a.value} value={a.value} disabled={a.value === "redact" && redactUnsupported}>
                  {a.label}
                  {a.value === "redact" && redactUnsupported ? " (provider cannot redact)" : ""}
                </MenuItem>
              ))}
            </Select>
            <FormHelperText>{(ACTIONS.find((a) => a.value === config.on_detect) || {}).help}</FormHelperText>
          </FormControl>
          {config.on_detect === "redact" && responseFilter && (
            <Alert severity="info" sx={{ mt: 1 }}>
              LLM responses are block-only. On a response filter, redact records the finding and lets the response through.
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
                    helperText={
                      (errors.connection && errors.connection[f.name]) ||
                      (f.secret
                        ? `${f.description || ""} Reference a stored secret as $SECRET/name or an environment variable as $ENV/NAME.`.trim()
                        : f.description)
                    }
                    inputProps={{ "data-testid": `guardrail-connection-${f.name}` }}
                  />
                </Grid>
              ))}
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

        {config.on_detect === "redact" && !responseFilter && (
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
              <InputLabel id="guardrail-scope-label">Messages to inspect</InputLabel>
              <Select
                labelId="guardrail-scope-label"
                label="Messages to inspect"
                value={config.scope || ""}
                onChange={(e) => update({ scope: e.target.value })}
              >
                {SCOPES.map((s) => (
                  <MenuItem key={s.value || "default"} value={s.value}>
                    {s.label}
                  </MenuItem>
                ))}
              </Select>
              <FormHelperText>Applies to LLM requests. Chat messages and tool calls always inspect the whole text.</FormHelperText>
            </FormControl>
          </Grid>
        )}

        <Grid item xs={12} md={6}>
          <FormControl fullWidth>
            <InputLabel id="guardrail-fail-mode-label">If the provider fails</InputLabel>
            <Select
              labelId="guardrail-fail-mode-label"
              label="If the provider fails"
              value={config.fail_mode || ""}
              onChange={(e) => update({ fail_mode: e.target.value })}
              inputProps={{ "data-testid": "guardrail-fail-mode" }}
            >
              <MenuItem value="">{responseFilter ? "Let it through (default for responses)" : "Block (default for requests)"}</MenuItem>
              <MenuItem value="closed">Block</MenuItem>
              <MenuItem value="open">Let it through</MenuItem>
            </Select>
            <FormHelperText>A timeout or error records a compliance event either way.</FormHelperText>
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
      </Grid>
    </Box>
  );
};

export default GuardrailConfigForm;
