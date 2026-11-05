import React, { useCallback, useEffect, useState } from "react";
import {
  Alert,
  Box,
  Button,
  Checkbox,
  CircularProgress,
  FormControlLabel,
  TextField,
  Typography,
} from "@mui/material";
import CollapsibleSection from "../../common/CollapsibleSection";
import { appQuotaService } from "../../../services/appQuotaService";
import { errorDetail } from "../../../services/teamBudgetsService";

const describe = (max, unlimited) => {
  if (unlimited) return "Unlimited";
  return `${max} ${max === 1 ? "app" : "apps"} per member`;
};

// TeamAppQuotaPanel sets how many Apps each member of the team may create
// through the portal (Enterprise). Admin-created Apps do not count against
// it; a member of several teams gets the most generous quota.
const TeamAppQuotaPanel = ({ teamId }) => {
  const [quota, setQuota] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [max, setMax] = useState("");
  const [unlimited, setUnlimited] = useState(false);
  const [saving, setSaving] = useState(false);

  const apply = (q) => {
    setQuota(q);
    setMax(q.set && !q.unlimited && q.max_apps_per_member != null ? String(q.max_apps_per_member) : "");
    setUnlimited(Boolean(q.set && q.unlimited));
  };

  const load = useCallback(async () => {
    setLoading(true);
    try {
      apply(await appQuotaService.getTeamQuota(teamId));
      setError("");
    } catch (err) {
      setError(errorDetail(err, "Failed to load the team's app quota"));
    } finally {
      setLoading(false);
    }
  }, [teamId]);

  useEffect(() => {
    if (teamId) load();
  }, [teamId, load]);

  const save = async () => {
    const n = Number(max);
    if (!unlimited && (max === "" || !Number.isInteger(n) || n < 0)) {
      setError("Enter a whole number of apps per member (0 or more), or choose unlimited.");
      return;
    }
    setSaving(true);
    try {
      apply(await appQuotaService.setTeamQuota(teamId, unlimited ? { unlimited: true } : { max_apps_per_member: n }));
      setError("");
    } catch (err) {
      setError(errorDetail(err, "Failed to save the team's app quota"));
    } finally {
      setSaving(false);
    }
  };

  const clear = async () => {
    setSaving(true);
    try {
      await appQuotaService.removeTeamQuota(teamId);
      await load();
    } catch (err) {
      setError(errorDetail(err, "Failed to remove the team's app quota"));
    } finally {
      setSaving(false);
    }
  };

  let body;
  if (loading && !quota) {
    body = <CircularProgress size={24} />;
  } else if (!quota) {
    body = <Alert severity="error">{error}</Alert>;
  } else {
    body = (
      <Box sx={{ display: "flex", flexDirection: "column", gap: 2 }} data-testid="team-app-quota-panel">
        {error && <Alert severity="error">{error}</Alert>}
        <Typography variant="bodyLargeDefault" color="text.defaultSubdued" data-testid="team-app-quota-current">
          {quota.set
            ? `Members may create ${describe(quota.max_apps_per_member, quota.unlimited).toLowerCase()} through the portal.`
            : `No team quota: members get the default of ${describe(quota.default_max, quota.default_unlimited).toLowerCase()}.`}
        </Typography>
        <Typography variant="bodySmallDefault" color="text.defaultSubdued">
          Applies to apps members create themselves. A member of several teams gets the most generous quota. Apps an administrator creates do not count.
        </Typography>
        <Box sx={{ display: "flex", alignItems: "center", gap: 2, flexWrap: "wrap" }}>
          <TextField
            label="Apps per member"
            type="number"
            size="small"
            value={max}
            disabled={unlimited}
            onChange={(e) => setMax(e.target.value)}
            inputProps={{ min: "0", step: "1", "data-testid": "team-app-quota-input" }}
          />
          <FormControlLabel
            control={
              <Checkbox
                checked={unlimited}
                onChange={(e) => setUnlimited(e.target.checked)}
                inputProps={{ "data-testid": "team-app-quota-unlimited" }}
              />
            }
            label="Unlimited"
          />
          <Button variant="outlined" onClick={save} disabled={saving} data-testid="team-app-quota-save">
            Save
          </Button>
          {quota.set && (
            <Button variant="text" color="error" onClick={clear} disabled={saving} data-testid="team-app-quota-clear">
              Use the default
            </Button>
          )}
        </Box>
      </Box>
    );
  }

  return (
    <CollapsibleSection title="App quota" defaultExpanded>
      {body}
    </CollapsibleSection>
  );
};

export default TeamAppQuotaPanel;
