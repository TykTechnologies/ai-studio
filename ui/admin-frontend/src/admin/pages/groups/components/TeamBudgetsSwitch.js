import React, { useEffect, useState } from "react";
import { Alert, Box, FormControlLabel, Switch, Typography } from "@mui/material";
import { useEdition } from "../../../context/EditionContext";
import { teamBudgetsService, errorDetail } from "../../../services/teamBudgetsService";

// TeamBudgetsSwitch is the global team budget switch (Enterprise). While it
// is off, Apps and spend are still attributed to teams for reporting, but no
// allocation or enforcement happens.
const TeamBudgetsSwitch = () => {
  const { isEnterprise } = useEdition();
  const [enabled, setEnabled] = useState(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (!isEnterprise) return;
    teamBudgetsService
      .getSettings()
      .then((s) => setEnabled(!!s.enabled))
      .catch((err) => setError(errorDetail(err, "Failed to load team budget settings")));
  }, [isEnterprise]);

  if (!isEnterprise || enabled === null) {
    return error ? <Alert severity="error">{error}</Alert> : null;
  }

  const toggle = async (e) => {
    setBusy(true);
    try {
      const s = await teamBudgetsService.setEnabled(e.target.checked);
      setEnabled(!!s.enabled);
      setError("");
    } catch (err) {
      setError(errorDetail(err, "Failed to change team budgets"));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Box sx={{ display: "flex", flexDirection: "column", gap: 1 }}>
      <FormControlLabel
        control={
          <Switch
            checked={enabled}
            onChange={toggle}
            disabled={busy}
            inputProps={{ "data-testid": "team-budgets-switch" }}
          />
        }
        label={<Typography variant="bodyLargeBold">Team budgets</Typography>}
      />
      <Typography variant="bodyMediumDefault" color="text.defaultSubdued">
        {enabled
          ? "New Apps draw their budget from their team's pool, and teams in blocking mode have their Apps refused once the team budget is spent. Teams with no budget, and the Apps created before this was switched on, are unaffected. The Default team starts with an empty pool."
          : "Off: spend is reported per team, but no team budget is allocated or enforced."}
      </Typography>
      {error && <Alert severity="error">{error}</Alert>}
    </Box>
  );
};

export default TeamBudgetsSwitch;
