import React, { useEffect, useMemo, useState } from "react";
import {
  Alert,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  FormControl,
  FormHelperText,
  Grid,
  InputLabel,
  MenuItem,
  Select,
  TextField,
  Typography,
} from "@mui/material";
import { PrimaryButton, SecondaryOutlineButton } from "../../../styles/sharedStyles";
import {
  ALERT_EVENT_PATTERN,
  allowedActions,
  apiErrorMessage,
  channelLabel,
  createGuardrailException,
  exceptionActionLabel,
  isToolChannel,
  updateGuardrailException,
} from "../../../services/guardrailExceptionService";

const DEFAULT_EXPIRY_DAYS = 30;

const blank = (filters) => ({
  filter_id: filters[0]?.id ? Number(filters[0].id) : "",
  detector: "",
  channel: "text_user",
  tool_name: "",
  action: "",
  alert_event: "",
  justification: "",
  expires_after_days: DEFAULT_EXPIRY_DAYS,
});

// GuardrailExceptionEditor proposes a new exception or changes one. The
// actions offered are only those the app's capability allows on the channel
// (the floor table), softest first; the server checks again.
const GuardrailExceptionEditor = ({ open, appId, exception, filters, meta, capability, isFullAdmin, onClose, onSaved }) => {
  const [form, setForm] = useState(blank(filters));
  const [error, setError] = useState(null);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (!open) return;
    setError(null);
    setForm(
      exception
        ? {
            filter_id: exception.filter_id,
            detector: exception.detector || "",
            channel: exception.channel,
            tool_name: exception.tool_name || "",
            action: exception.action,
            alert_event: exception.alert_event || "",
            justification: exception.justification || "",
            expires_after_days: exception.expires_after_days || DEFAULT_EXPIRY_DAYS,
          }
        : blank(filters)
    );
  }, [open, exception, filters]);

  const channels = Object.keys(meta?.actions || {});
  const actions = useMemo(() => allowedActions(meta, capability, form.channel), [meta, capability, form.channel]);
  // Default to the softest the capability allows.
  useEffect(() => {
    if (!open) return;
    if (!actions.includes(form.action)) setForm((f) => ({ ...f, action: actions[0] || "" }));
  }, [open, actions, form.action]);

  const set = (patch) => setForm((f) => ({ ...f, ...patch }));
  const alertInvalid = form.action === "alert" && !ALERT_EVENT_PATTERN.test(form.alert_event.trim());
  const valid =
    form.filter_id &&
    form.channel &&
    form.action &&
    !alertInvalid &&
    form.justification.trim() &&
    Number(form.expires_after_days) >= 1;

  const save = async () => {
    setSaving(true);
    setError(null);
    const body = {
      filter_id: Number(form.filter_id),
      detector: form.detector.trim(),
      channel: form.channel,
      tool_name: isToolChannel(form.channel) ? form.tool_name.trim() : "",
      action: form.action,
      ...(form.action === "alert" ? { alert_event: form.alert_event.trim() } : {}),
      justification: form.justification.trim(),
      expires_after_days: Number(form.expires_after_days),
    };
    try {
      const saved = exception
        ? await updateGuardrailException(appId, exception.id, body)
        : await createGuardrailException(appId, body);
      onSaved(saved);
    } catch (e) {
      setError(apiErrorMessage(e, "Could not save the exception"));
    } finally {
      setSaving(false);
    }
  };

  return (
    <Dialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
      <DialogTitle>{exception ? "Change guardrail exception" : "Propose guardrail exception"}</DialogTitle>
      <DialogContent>
        <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
          An exception changes what a filter does when it flags this app's traffic, never what it detects. The
          compliance event is still recorded, tagged with the exception.
          {!isFullAdmin && " An administrator must approve it before it has any effect."}
        </Typography>
        {error && (
          <Alert severity="error" sx={{ mb: 2 }}>
            {error}
          </Alert>
        )}
        <Grid container spacing={2}>
          <Grid item xs={12}>
            <FormControl fullWidth>
              <InputLabel id="gx-filter-label">Filter</InputLabel>
              <Select
                labelId="gx-filter-label"
                label="Filter"
                value={form.filter_id ? String(form.filter_id) : ""}
                onChange={(e) => set({ filter_id: Number(e.target.value) })}
                inputProps={{ "data-testid": "gx-filter" }}
              >
                {filters.map((f) => (
                  <MenuItem key={f.id} value={String(f.id)}>
                    {f.name}
                  </MenuItem>
                ))}
              </Select>
            </FormControl>
          </Grid>
          <Grid item xs={12} md={6}>
            <TextField
              fullWidth
              label="Detector"
              value={form.detector}
              onChange={(e) => set({ detector: e.target.value })}
              placeholder="Any detector"
              InputLabelProps={{ shrink: true }}
              helperText="As compliance events name it. Empty covers every detector of the filter."
              inputProps={{ "data-testid": "gx-detector" }}
            />
          </Grid>
          <Grid item xs={12} md={6}>
            <FormControl fullWidth>
              <InputLabel id="gx-channel-label">Where</InputLabel>
              <Select
                labelId="gx-channel-label"
                label="Where"
                value={form.channel}
                onChange={(e) => set({ channel: e.target.value })}
                inputProps={{ "data-testid": "gx-channel" }}
              >
                {channels.map((c) => (
                  <MenuItem key={c} value={c}>
                    {channelLabel(c)}
                  </MenuItem>
                ))}
              </Select>
            </FormControl>
          </Grid>
          {isToolChannel(form.channel) && (
            <Grid item xs={12}>
              <TextField
                fullWidth
                label="Tool name"
                value={form.tool_name}
                onChange={(e) => set({ tool_name: e.target.value })}
                placeholder="Any tool"
                InputLabelProps={{ shrink: true }}
                inputProps={{ "data-testid": "gx-tool" }}
              />
            </Grid>
          )}
          <Grid item xs={12} md={6}>
            <FormControl fullWidth error={actions.length === 0}>
              <InputLabel id="gx-action-label">Instead of blocking</InputLabel>
              <Select
                labelId="gx-action-label"
                label="Instead of blocking"
                value={actions.includes(form.action) ? form.action : ""}
                onChange={(e) => set({ action: e.target.value })}
                disabled={actions.length === 0}
                inputProps={{ "data-testid": "gx-action" }}
              >
                {actions.map((a) => (
                  <MenuItem key={a} value={a}>
                    {exceptionActionLabel(a)}
                  </MenuItem>
                ))}
              </Select>
              <FormHelperText>
                {actions.length === 0
                  ? "This app's capability allows no softening here. If its tools only read, a full administrator can attest a lower capability."
                  : "Only what the app's capability allows is offered."}
              </FormHelperText>
            </FormControl>
          </Grid>
          {form.action === "alert" && (
            <Grid item xs={12}>
              <TextField
                fullWidth
                required
                label="Event name"
                value={form.alert_event}
                onChange={(e) => set({ alert_event: e.target.value })}
                error={!!form.alert_event && alertInvalid}
                helperText="Webhooks subscribe to guardrail.alert.<name>. Lowercase letters, digits, - and _; unique across filters and exceptions."
                inputProps={{ "data-testid": "gx-alert-event" }}
              />
            </Grid>
          )}
          <Grid item xs={12} md={6}>
            <TextField
              fullWidth
              type="number"
              label="Expires after (days without a hit)"
              value={form.expires_after_days}
              onChange={(e) => set({ expires_after_days: e.target.value })}
              inputProps={{ min: 1, max: 3650, "data-testid": "gx-expiry" }}
              helperText="It lapses back to proposed when it has not applied for this long."
            />
          </Grid>
          <Grid item xs={12}>
            <TextField
              fullWidth
              required
              label="Justification"
              value={form.justification}
              onChange={(e) => set({ justification: e.target.value })}
              placeholder="Why this traffic is legitimate"
              inputProps={{ maxLength: 500, "data-testid": "gx-justification" }}
            />
          </Grid>
        </Grid>
      </DialogContent>
      <DialogActions>
        <SecondaryOutlineButton onClick={onClose}>Cancel</SecondaryOutlineButton>
        <PrimaryButton onClick={save} disabled={!valid || saving} data-testid="gx-save">
          {isFullAdmin ? "Save" : "Propose"}
        </PrimaryButton>
      </DialogActions>
    </Dialog>
  );
};

export default GuardrailExceptionEditor;
