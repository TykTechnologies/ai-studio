import React, { useEffect, useState } from "react";
import { Autocomplete, Box, Grid, TextField, Typography } from "@mui/material";
import apiClient from "../utils/apiClient";
import { PrimaryButton } from "../styles/sharedStyles";
import { apiErrorDetail } from "./webhookShared";

const toForm = (ka) => ({
  allowed_tools: ka?.allowed_tools || [],
  rate: ka?.rate ? String(ka.rate) : "",
  per: ka?.per ? String(ka.per) : "",
  quota_max: ka?.quota_max ? String(ka.quota_max) : "",
  quota_renewal_rate: ka?.quota_renewal_rate ? String(ka.quota_renewal_rate) : "",
});

/**
 * toKeyAccess turns the form into the PUT body. Empty fields mean "no
 * limit"; exported for the test.
 */
export const toKeyAccess = (form) => ({
  allowed_tools: form.allowed_tools,
  rate: Number(form.rate) || 0,
  per: Number(form.rate) ? Number(form.per) || 0 : 0,
  quota_max: Number(form.quota_max) || 0,
  quota_renewal_rate: Number(form.quota_max) ? Number(form.quota_renewal_rate) || 0 : 0,
});

/**
 * keyAccessErrors returns the per-field problems the server would reject:
 * a rate needs a period and a quota needs a renewal period. Exported for
 * the test.
 */
export const keyAccessErrors = (form) => {
  const errors = {};
  const positive = (v) => Number(v) > 0;
  if (Number(form.rate) < 0 || Number(form.per) < 0 || Number(form.quota_max) < 0 || Number(form.quota_renewal_rate) < 0) {
    errors.negative = "Limits cannot be negative.";
  }
  if (positive(form.rate) && !positive(form.per)) errors.per = "A rate limit needs a period in seconds.";
  if (positive(form.quota_max) && !positive(form.quota_renewal_rate)) errors.quota_renewal_rate = "A quota needs a renewal period in seconds.";
  return errors;
};

/**
 * toolNamesHelp explains the allowed-tools field. Names stay free text:
 * a remote proxy's definition often lists no tools, and the server's own
 * tools/list is the authority. Names the definition does not know are
 * called out, since a typo would silently block that tool. Exported for the
 * test.
 */
export const toolNamesHelp = (allowed, known) => {
  const base = "Empty: keys may list and call every tool. Otherwise only these, and tools/list shows only these.";
  if (!allowed || allowed.length === 0) return { text: base, warning: false };
  if (!known || known.length === 0) {
    return { text: "This proxy's definition lists no tools, so the names cannot be checked here: copy them exactly from the server's tools/list.", warning: true };
  }
  const unknown = allowed.filter((t) => !known.includes(t));
  if (unknown.length === 0) return { text: base, warning: false };
  return { text: `Not a tool of this proxy: ${unknown.join(", ")}. Keys will not be able to call a tool that does not exist under that exact name.`, warning: true };
};

/**
 * MCPKeyAccessEditor sets what keys minted on a Tyk Gateway connection grant
 * for one MCP server. Open-source gateways carry no policies AI Studio can
 * rely on, so the rights are written onto each key; saving rewrites the live
 * keys of every App that uses the server.
 */
const MCPKeyAccessEditor = ({ server, onSaved, onError }) => {
  const [form, setForm] = useState(toForm(server.key_access));
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    setForm(toForm(server.key_access));
  }, [server.key_access]);

  const toolOptions = (server.primitives || []).filter((p) => p.type === "tool").map((p) => p.name);
  const errors = keyAccessErrors(form);
  const invalid = Object.keys(errors).length > 0;
  const toolsHelp = toolNamesHelp(form.allowed_tools, toolOptions);
  const set = (field) => (e) => setForm((f) => ({ ...f, [field]: e.target.value }));

  const save = async () => {
    setSaving(true);
    try {
      const res = await apiClient.put(`/mcp-servers/${server.id}/key-access`, toKeyAccess(form));
      onSaved(res.data);
    } catch (err) {
      onError(apiErrorDetail(err, "Saving key access failed"));
    } finally {
      setSaving(false);
    }
  };

  return (
    <Box data-testid="key-access-editor">
      <Grid container spacing={2}>
        <Grid item xs={12}>
          <Autocomplete
            multiple
            freeSolo
            options={toolOptions}
            value={form.allowed_tools}
            onChange={(_, value) => setForm((f) => ({ ...f, allowed_tools: value }))}
            renderInput={(params) => (
              <TextField
                {...params}
                label="Allowed tools"
                placeholder="Every tool the proxy exposes"
                helperText={toolsHelp.text}
                FormHelperTextProps={{ sx: toolsHelp.warning ? { color: "warning.main" } : undefined, "data-testid": "key-access-tools-help" }}
                inputProps={{ ...params.inputProps, "data-testid": "key-access-tools" }}
              />
            )}
          />
        </Grid>
        <Grid item xs={6} sm={3}>
          <TextField fullWidth type="number" label="Requests" value={form.rate} onChange={set("rate")} inputProps={{ min: 0, "data-testid": "key-access-rate" }} />
        </Grid>
        <Grid item xs={6} sm={3}>
          <TextField
            fullWidth
            type="number"
            label="per seconds"
            value={form.per}
            onChange={set("per")}
            error={Boolean(errors.per)}
            helperText={errors.per}
            inputProps={{ min: 0, "data-testid": "key-access-per" }}
          />
        </Grid>
        <Grid item xs={6} sm={3}>
          <TextField fullWidth type="number" label="Quota (requests)" value={form.quota_max} onChange={set("quota_max")} inputProps={{ min: 0, "data-testid": "key-access-quota" }} />
        </Grid>
        <Grid item xs={6} sm={3}>
          <TextField
            fullWidth
            type="number"
            label="renews every (seconds)"
            value={form.quota_renewal_rate}
            onChange={set("quota_renewal_rate")}
            error={Boolean(errors.quota_renewal_rate)}
            helperText={errors.quota_renewal_rate}
            inputProps={{ min: 0, "data-testid": "key-access-renewal" }}
          />
        </Grid>
      </Grid>
      <Typography variant="body2" color={errors.negative ? "error" : "text.secondary"} sx={{ mt: 1 }}>
        {errors.negative || "Limits count per App key and per server. Leave them empty for no limit."}
      </Typography>
      <Box sx={{ mt: 2 }}>
        <PrimaryButton variant="contained" onClick={save} disabled={saving || invalid} data-testid="save-key-access">
          {saving ? "Saving…" : "Save key access"}
        </PrimaryButton>
      </Box>
    </Box>
  );
};

export default MCPKeyAccessEditor;
