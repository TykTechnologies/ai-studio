import React, { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { Box, Button, Chip, Grid, Typography } from "@mui/material";
import { teamBudgetsService, errorDetail } from "../../services/teamBudgetsService";

// AppTeamBudgetRow shows the team an App's spend counts towards and how its
// budget relates to the team pool, with an action to move a manually
// budgeted App into the pool (Enterprise).
const AppTeamBudgetRow = ({ app, onChanged, onError, FieldLabel, FieldValue }) => {
  const teamId = app?.attributes?.team_id;
  const source = app?.attributes?.budget_source;
  const budget = app?.attributes?.monthly_budget;
  const [report, setReport] = useState(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (!teamId) {
      setReport(null);
      return;
    }
    teamBudgetsService
      .getReport(teamId)
      .then(setReport)
      .catch(() => setReport(null));
  }, [teamId]);

  const pooled = report?.enabled && report?.managed;
  let chip = null;
  if (source === "team") {
    chip = budget > 0
      ? <Chip size="small" color="primary" label="Team pool allocation" />
      : <Chip size="small" color="error" label="No allocation: requests are refused" />;
  } else if (pooled) {
    chip = <Chip size="small" variant="outlined" label="Own budget (not from the team pool)" />;
  }

  const adopt = async () => {
    setBusy(true);
    try {
      await teamBudgetsService.adoptApp(app.id);
      onChanged?.();
    } catch (err) {
      onError?.(errorDetail(err, "Failed to move the App into the team pool"));
    } finally {
      setBusy(false);
    }
  };

  return (
    <>
      <Grid item xs={3}>
        <FieldLabel>Team:</FieldLabel>
      </Grid>
      <Grid item xs={9}>
        <Box sx={{ display: "flex", alignItems: "center", gap: 2, flexWrap: "wrap" }} data-testid="app-team-budget">
          <FieldValue>
            {teamId ? (
              <Link to={`/admin/groups/${teamId}`}>{report?.team_name || `Team ${teamId}`}</Link>
            ) : (
              <Typography component="span" color="text.defaultSubdued">Not attributed to a team</Typography>
            )}
          </FieldValue>
          {chip}
          {pooled && source !== "team" && (
            <Button variant="outlined" size="small" onClick={adopt} disabled={busy} data-testid="app-adopt-team-budget">
              Move into team pool
            </Button>
          )}
        </Box>
      </Grid>
    </>
  );
};

export default AppTeamBudgetRow;
