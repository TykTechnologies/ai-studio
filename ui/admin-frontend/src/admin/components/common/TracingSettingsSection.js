import React from "react";
import {
  AccordionDetails,
  AccordionSummary,
  FormControlLabel,
  Grid,
  MenuItem,
  Switch,
  TextField,
  Typography,
} from "@mui/material";
import ExpandMoreIcon from "@mui/icons-material/ExpandMore";
import { StyledAccordion } from "../../styles/sharedStyles";
import useSystemFeatures from "../../hooks/useSystemFeatures";
import { usePermissions } from "../../context/PermissionsContext";
import { P } from "../../rbac/permissions";

// Agent tracing settings (Enterprise). The API stores them on every
// edition, but the section only appears when agent tracing is running
// (feature_agent_tracing is true only on an Enterprise build with the feature
// enabled), so the Community Edition shows nothing.

export const CONTENT_MODES = [
  { value: "off", label: "Off: no content" },
  { value: "metadata", label: "Metadata: sizes and types only" },
  { value: "hashed", label: "Hashed: fingerprints only" },
  { value: "redacted", label: "Redacted: content with PII removed" },
  { value: "full", label: "Full: content as sent" },
];

const DURATION_PATTERN = /^(\d+(\.\d+)?(h|m|s))+$/;
const DURATION_PART = /(\d+(?:\.\d+)?)(h|m|s)/g;
const UNIT_SECONDS = { h: 3600, m: 60, s: 1 };
const MAX_PAYLOAD_DAYS = 365;

// Seconds in a Go-style duration of h/m/s parts ("1h30m"), or null.
const durationSeconds = (text) => {
  if (!DURATION_PATTERN.test(text)) return null;
  let total = 0;
  for (const [, n, unit] of text.matchAll(DURATION_PART)) total += parseFloat(n) * UNIT_SECONDS[unit];
  return total;
};

/** The App's tracing overrides as the API stores them: empty fields inherit. */
export const appTracingFromResponse = (tracing) => ({
  enabled: tracing?.enabled === true ? "on" : tracing?.enabled === false ? "off" : "",
  content_mode: tracing?.content_mode || "",
  max_request_override: tracing?.max_request_override || "",
  session_idle_timeout: tracing?.session_idle_timeout || "",
  payload_days: tracing?.payload_days ?? "",
});

/** The request body's tracing object; fields left on "inherit" are omitted. */
export const appTracingToRequest = (form) => {
  const out = {};
  if (form.enabled === "on") out.enabled = true;
  if (form.enabled === "off") out.enabled = false;
  if (form.content_mode) out.content_mode = form.content_mode;
  if (form.max_request_override) out.max_request_override = form.max_request_override;
  if (form.session_idle_timeout) out.session_idle_timeout = form.session_idle_timeout.trim();
  if (form.payload_days !== "" && form.payload_days !== null && form.payload_days !== undefined) {
    out.payload_days = Number(form.payload_days);
  }
  return out;
};

/**
 * Field errors for the App settings, keyed by field (empty when valid).
 * Mirrors models.AppTracingSettings.Validate: 1m to 168h, 0 to 365 days.
 */
export const validateAppTracing = (form) => {
  const errors = {};
  const timeout = (form.session_idle_timeout || "").trim();
  if (timeout) {
    const seconds = durationSeconds(timeout);
    if (seconds === null) {
      errors.session_idle_timeout = "Use a duration such as 30m, 2h or 1h30m";
    } else if (seconds < 60 || seconds > 168 * 3600) {
      errors.session_idle_timeout = "Between 1m and 168h";
    }
  }
  if (form.payload_days !== "" && form.payload_days !== null && form.payload_days !== undefined) {
    const d = Number(form.payload_days);
    if (!Number.isInteger(d) || d < 0 || d > MAX_PAYLOAD_DAYS) {
      errors.payload_days = `A whole number of days from 0 to ${MAX_PAYLOAD_DAYS}`;
    }
  }
  return errors;
};

/** Whether the section should render at all. */
export const useTracingSettingsVisible = () => {
  const { features } = useSystemFeatures();
  return Boolean(features?.feature_agent_tracing);
};

// Changing any tracing setting needs traces:write (the API refuses it
// otherwise); without it the settings are shown read-only, and an unrelated
// edit sends them back exactly as loaded.
const READ_ONLY_NOTE = "You can view these settings. Changing them needs permission to manage traces (traces:write).";

const ReadOnlyNote = () => (
  <Typography variant="body2" color="text.secondary" paragraph data-testid="tracing-read-only">
    {READ_ONLY_NOTE}
  </Typography>
);

const ModeSelect = ({ label, name, value, onChange, emptyLabel, helperText, disabled }) => (
  <TextField
    select
    fullWidth
    disabled={disabled}
    label={label}
    name={name}
    value={value || ""}
    onChange={(e) => onChange(name, e.target.value)}
    helperText={helperText}
    SelectProps={{ displayEmpty: true }}
    InputLabelProps={{ shrink: true }}
  >
    <MenuItem value="">{emptyLabel}</MenuItem>
    {CONTENT_MODES.map((m) => (
      <MenuItem key={m.value} value={m.value}>
        {m.label}
      </MenuItem>
    ))}
  </TextField>
);

/**
 * The "Tracing" section of the LLM, Tool and Datasource forms: a cap on the
 * content mode for calls to the resource, and (LLMs) the traceparent
 * propagation opt-out.
 */
export const ResourceTracingSection = ({ contentModeMax, onContentModeMaxChange, propagationOptOut, onPropagationOptOutChange, resourceLabel }) => {
  const visible = useTracingSettingsVisible();
  const { can } = usePermissions();
  if (!visible) return null;
  const readOnly = !can(P.TRACES_WRITE);
  const showPropagation = typeof onPropagationOptOutChange === "function";
  return (
    <StyledAccordion data-testid="tracing-settings-section">
      <AccordionSummary expandIcon={<ExpandMoreIcon />}>
        <Typography>Tracing</Typography>
      </AccordionSummary>
      <AccordionDetails>
        {readOnly && <ReadOnlyNote />}
        <Grid container spacing={3}>
          <Grid item xs={12}>
            <ModeSelect
              disabled={readOnly}
              label="Maximum content mode"
              name="tracing_content_mode_max"
              value={contentModeMax}
              onChange={(_, v) => onContentModeMaxChange(v)}
              emptyLabel="No cap"
              helperText={`Agent traces of calls to this ${resourceLabel} never capture more than this, whatever the App asks for.`}
            />
          </Grid>
          {showPropagation && (
            <Grid item xs={12}>
              <FormControlLabel
                control={
                  <Switch
                    checked={Boolean(propagationOptOut)}
                    onChange={(e) => onPropagationOptOutChange(e.target.checked)}
                    name="tracing_propagation_opt_out"
                    color="primary"
                    disabled={readOnly}
                  />
                }
                label="Don't send trace context to this provider"
              />
              <Typography variant="caption" color="text.secondary" display="block">
                By default the gateway sends a traceparent header upstream so the provider's own traces line up with
                Studio's. Turn this on to stop sending it to this LLM.
              </Typography>
            </Grid>
          )}
        </Grid>
      </AccordionDetails>
    </StyledAccordion>
  );
};

/**
 * The "Tracing" section of the App form: overrides of the global agent
 * tracing settings. value is the form state from appTracingFromResponse.
 */
export const AppTracingSection = ({ value, onChange, errors = {} }) => {
  const visible = useTracingSettingsVisible();
  const { can } = usePermissions();
  if (!visible) return null;
  const set = (name, v) => onChange({ ...value, [name]: v });
  const readOnly = !can(P.TRACES_WRITE);
  return (
    <StyledAccordion data-testid="tracing-settings-section">
      <AccordionSummary expandIcon={<ExpandMoreIcon />}>
        <Typography>Tracing</Typography>
      </AccordionSummary>
      <AccordionDetails>
        <Typography variant="body2" color="text.secondary" paragraph>
          Overrides of the global agent tracing settings for this App. Leave a field on Inherit to use the global value.
        </Typography>
        {readOnly && <ReadOnlyNote />}
        <Grid container spacing={3}>
          <Grid item xs={12} md={6}>
            <TextField
              select
              fullWidth
              label="Tracing"
              name="enabled"
              disabled={readOnly}
              value={value.enabled}
              onChange={(e) => set("enabled", e.target.value)}
              SelectProps={{ displayEmpty: true }}
              InputLabelProps={{ shrink: true }}
            >
              <MenuItem value="">Inherit</MenuItem>
              <MenuItem value="on">On</MenuItem>
              <MenuItem value="off">Off</MenuItem>
            </TextField>
          </Grid>
          <Grid item xs={12} md={6}>
            <ModeSelect
              label="Content mode"
              name="content_mode"
              value={value.content_mode}
              onChange={set}
              emptyLabel="Inherit"
              disabled={readOnly}
              helperText="What is captured of prompts, responses and tool data, up to the global ceiling. Inherit uses the global default."
            />
          </Grid>
          <Grid item xs={12} md={6}>
            <ModeSelect
              label="Highest mode a request may ask for"
              name="max_request_override"
              value={value.max_request_override}
              onChange={set}
              emptyLabel="Inherit"
              disabled={readOnly}
              helperText="A client can raise capture with a request header up to this mode."
            />
          </Grid>
          <Grid item xs={12} md={6}>
            <TextField
              fullWidth
              label="Session idle timeout"
              name="session_idle_timeout"
              disabled={readOnly}
              value={value.session_idle_timeout}
              onChange={(e) => set("session_idle_timeout", e.target.value)}
              placeholder="Inherit (for example 30m or 2h)"
              InputLabelProps={{ shrink: true }}
              error={Boolean(errors.session_idle_timeout)}
              helperText={errors.session_idle_timeout || "A session closes after this long without activity."}
            />
          </Grid>
          <Grid item xs={12} md={6}>
            <TextField
              fullWidth
              type="number"
              label="Keep captured content (days)"
              name="payload_days"
              disabled={readOnly}
              value={value.payload_days}
              onChange={(e) => set("payload_days", e.target.value)}
              placeholder="Inherit"
              InputLabelProps={{ shrink: true }}
              inputProps={{ min: 0, max: MAX_PAYLOAD_DAYS, step: 1 }}
              error={Boolean(errors.payload_days)}
              helperText={errors.payload_days || "Content is deleted sooner than trace metadata."}
            />
          </Grid>
        </Grid>
      </AccordionDetails>
    </StyledAccordion>
  );
};
