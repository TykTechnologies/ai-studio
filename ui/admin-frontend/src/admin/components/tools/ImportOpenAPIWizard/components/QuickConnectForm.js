import React, { useState } from "react";
import { useNavigate } from "react-router-dom";
import { Alert, Box, Checkbox, FormControlLabel, Grid, TextField, Typography } from "@mui/material";
import { PrimaryButton, SecondaryLinkButton, SecondaryOutlineButton } from "../../../../styles/sharedStyles";
import { usePermissions } from "../../../../context/PermissionsContext";
import { P } from "../../../../rbac/permissions";
import { apiErrorDetail } from "../../../../pages/webhookShared";
import { emptyForm, formToInput, ProbePanel } from "../../../../pages/tykShared";
import apiClient from "../../../../utils/apiClient";

/**
 * QuickConnectForm adds a Tyk connection without leaving the import wizard:
 * the few fields a read-only import needs, the same probe the settings page
 * runs, and the same create call. Everything else (trust mode, gateway URLs,
 * MDCB, templates) keeps its default and can be edited later under
 * Settings → Tyk Connections.
 */
const QuickConnectForm = ({ onCreated, onCancel }) => {
  const navigate = useNavigate();
  const { can } = usePermissions();
  const canExecute = can(P.TYK_CONNECTIONS_EXECUTE);
  const [form, setForm] = useState({
    name: "",
    dashboard_url: "",
    dashboard_access_token: "",
    org_id: "",
    allow_internal_host: false,
  });
  const [probe, setProbe] = useState(null);
  const [probing, setProbing] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");

  const set = (field) => (e) => setForm((f) => ({ ...f, [field]: e.target.value }));
  const payload = () => formToInput({ ...emptyForm, ...form }, false);
  const complete = form.name.trim() && form.dashboard_url.trim() && form.dashboard_access_token;

  const runProbe = async () => {
    setError("");
    setProbing(true);
    try {
      const res = await apiClient.post("/tyk-connections/probe", payload());
      setProbe(res.data);
    } catch (err) {
      setError(apiErrorDetail(err, "Probe failed"));
    } finally {
      setProbing(false);
    }
  };

  const save = async () => {
    setError("");
    setSaving(true);
    try {
      const res = await apiClient.post("/tyk-connections", payload());
      onCreated(res.data);
    } catch (err) {
      setError(apiErrorDetail(err, "Could not save the connection"));
    } finally {
      setSaving(false);
    }
  };

  return (
    <Box sx={{ mt: 2, p: 2, border: 1, borderColor: "divider", borderRadius: 1 }} data-testid="quick-connect">
      <Typography variant="subtitle1" gutterBottom>
        Connect a Tyk Dashboard
      </Typography>
      <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
        One Dashboard, one dedicated Dashboard user. The access token is stored encrypted and never shown again.
        The connection is created in catalogue mode; an administrator can widen it later.
      </Typography>
      {error && (
        <Alert severity="error" sx={{ mb: 2 }} onClose={() => setError("")} data-testid="quick-connect-error">
          {error}
        </Alert>
      )}
      <Grid container spacing={2}>
        <Grid item xs={12} sm={5}>
          <TextField fullWidth label="Name" value={form.name} onChange={set("name")} required inputProps={{ "data-testid": "quick-connect-name" }} />
        </Grid>
        <Grid item xs={12} sm={7}>
          <TextField
            fullWidth
            label="Dashboard URL"
            placeholder="https://dashboard.example.com"
            value={form.dashboard_url}
            onChange={set("dashboard_url")}
            required
            inputProps={{ "data-testid": "quick-connect-url" }}
          />
        </Grid>
        <Grid item xs={12} sm={8}>
          <TextField
            fullWidth
            type="password"
            label="Dashboard access token"
            helperText="The API access key of a dedicated Dashboard user."
            value={form.dashboard_access_token}
            onChange={set("dashboard_access_token")}
            required
            autoComplete="new-password"
            inputProps={{ "data-testid": "quick-connect-token" }}
          />
        </Grid>
        <Grid item xs={12} sm={4}>
          <TextField fullWidth label="Organisation ID (optional)" value={form.org_id} onChange={set("org_id")} inputProps={{ "data-testid": "quick-connect-org" }} />
        </Grid>
        {canExecute && (
          <Grid item xs={12}>
            <FormControlLabel
              control={
                <Checkbox
                  checked={form.allow_internal_host}
                  onChange={(e) => setForm((f) => ({ ...f, allow_internal_host: e.target.checked }))}
                  inputProps={{ "data-testid": "quick-connect-internal" }}
                />
              }
              label="Allow an internal Dashboard host (private network address)"
            />
          </Grid>
        )}
      </Grid>
      <ProbePanel result={probe} />
      <Box sx={{ mt: 2, display: "flex", gap: 1, alignItems: "center", flexWrap: "wrap" }}>
        <SecondaryOutlineButton onClick={runProbe} disabled={!complete || probing || saving} data-testid="quick-connect-probe">
          {probing ? "Testing…" : "Test connection"}
        </SecondaryOutlineButton>
        <PrimaryButton variant="contained" onClick={save} disabled={!complete || saving || probing} data-testid="quick-connect-save">
          {saving ? "Saving…" : "Save connection"}
        </PrimaryButton>
        <SecondaryLinkButton onClick={onCancel} disabled={saving}>
          Cancel
        </SecondaryLinkButton>
        <Box sx={{ flexGrow: 1 }} />
        <SecondaryLinkButton onClick={() => navigate("/admin/tyk-connections/new")} data-testid="quick-connect-settings">
          More options in Settings → Tyk Connections
        </SecondaryLinkButton>
      </Box>
    </Box>
  );
};

export default QuickConnectForm;
