import React, { useEffect, useState } from "react";
import { FormControl, FormHelperText, InputLabel, MenuItem, Select } from "@mui/material";
import apiClient from "../../utils/apiClient";
import { listAll } from "../../utils/listAll";
import { teamBudgetsService, formatMoney } from "../../services/teamBudgetsService";

// poolSummary describes what a team's pool can give a new or edited App.
const poolSummary = (report) => {
  if (!report) return "";
  if (!report.enabled || !report.managed) {
    return "This team has no budget pool; the App budget below applies on its own.";
  }
  const left = Math.max(0, report.unallocated);
  return `Team pool: ${formatMoney(left)} of ${formatMoney(report.monthly_budget)} unallocated. Leave the budget empty to draw the team's default of ${formatMoney(report.default_app_allocation ?? 0)}.`;
};

// AppTeamField picks the team an App's spend is attributed to (Enterprise).
// An empty value means "the owner's budget team", resolved by the server.
const AppTeamField = ({ value, onChange }) => {
  const [teams, setTeams] = useState([]);
  const [report, setReport] = useState(null);

  useEffect(() => {
    listAll(apiClient, "/groups")
      .then((res) => setTeams(res.data.data || []))
      .catch(() => setTeams([]));
  }, []);

  useEffect(() => {
    if (!value) {
      setReport(null);
      return;
    }
    teamBudgetsService
      .getReport(value)
      .then(setReport)
      .catch(() => setReport(null));
  }, [value]);

  return (
    <FormControl fullWidth>
      <InputLabel id="appform-team-label">Team</InputLabel>
      <Select
        labelId="appform-team-label"
        label="Team"
        value={value ?? ""}
        onChange={(e) => onChange(e.target.value === "" ? null : Number(e.target.value))}
        inputProps={{ "data-testid": "app-team-select" }}
      >
        <MenuItem value="">
          <em>Owner's budget team</em>
        </MenuItem>
        {teams.map((team) => (
          <MenuItem key={team.id} value={Number(team.id)}>
            {team.attributes?.name}
          </MenuItem>
        ))}
      </Select>
      <FormHelperText data-testid="app-team-pool">
        {value ? poolSummary(report) : "The team the App's spend counts towards. By default, the owner's budget team."}
      </FormHelperText>
    </FormControl>
  );
};

export default AppTeamField;
