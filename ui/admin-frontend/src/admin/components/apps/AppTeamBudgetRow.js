import React, { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { Box, Grid, Typography } from "@mui/material";
import { teamBudgetsService } from "../../services/teamBudgetsService";

// AppTeamBudgetRow shows the team an App's spend counts towards and, when
// that team hands out budgets from a pool, says so (Enterprise).
const AppTeamBudgetRow = ({ app, FieldLabel, FieldValue }) => {
  const teamId = app?.attributes?.team_id;
  const [report, setReport] = useState(null);

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

  return (
    <>
      <Grid item xs={3}>
        <FieldLabel>Team:</FieldLabel>
      </Grid>
      <Grid item xs={9}>
        <Box sx={{ display: "flex", flexDirection: "column", gap: 0.5 }} data-testid="app-team-budget">
          <FieldValue>
            {teamId ? (
              <Link to={`/admin/groups/${teamId}`}>{report?.team_name || `Team ${teamId}`}</Link>
            ) : (
              <Typography component="span" color="text.defaultSubdued">Not attributed to a team</Typography>
            )}
          </FieldValue>
          {pooled && (
            <Typography variant="caption" color="text.secondary" data-testid="app-team-pool-note">
              The team has a budget pool: this App&apos;s budget counts against it, and the team&apos;s
              {report.enforcement === "hard_block" ? " blocking" : " alert-only"} budget applies on top.
            </Typography>
          )}
        </Box>
      </Grid>
    </>
  );
};

export default AppTeamBudgetRow;
