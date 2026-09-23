import React, { useCallback, useEffect, useState } from "react";
import {
  Alert,
  Box,
  Button,
  Chip,
  CircularProgress,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  FormControl,
  InputAdornment,
  InputLabel,
  LinearProgress,
  MenuItem,
  Select,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
  TextField,
  Typography,
} from "@mui/material";
import CollapsibleSection from "../../common/CollapsibleSection";
import {
  teamBudgetsService,
  errorDetail,
  formatMoney,
} from "../../../services/teamBudgetsService";

const formatDate = (value) =>
  value ? new Date(value).toLocaleDateString(undefined, { dateStyle: "medium" }) : "—";

const Stat = ({ label, value, testId }) => (
  <Box sx={{ minWidth: 140 }} data-testid={testId}>
    <Typography variant="bodySmallDefault" color="text.defaultSubdued">
      {label}
    </Typography>
    <Typography variant="bodyLargeBold" color="text.primary" component="div">
      {value}
    </Typography>
  </Box>
);

const appStatus = (row) => {
  if (row.deleted) return <Chip size="small" label="Decommissioned" />;
  if (row.blocked) return <Chip size="small" color="error" label="No allocation" />;
  if (row.uncapped) return <Chip size="small" color="warning" label="Uncapped" />;
  if (row.budget_source === "team") return <Chip size="small" color="primary" label="Team pool" />;
  return <Chip size="small" variant="outlined" label="Own budget" />;
};

const emptyForm = {
  monthly_budget: "",
  default_app_allocation: "",
  enforcement: "alert_only",
  budget_start_date: "",
};

const toNumberOrNull = (v) => (v === "" || v === null || v === undefined ? null : Number(v));

// TeamBudgetPanel shows a team's budget position for the current period and
// lets an administrator set, reset or remove its budget (Enterprise).
const TeamBudgetPanel = ({ teamId }) => {
  const [report, setReport] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [editing, setEditing] = useState(false);
  const [form, setForm] = useState(emptyForm);
  const [formError, setFormError] = useState("");
  const [saving, setSaving] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      setReport(await teamBudgetsService.getReport(teamId));
      setError("");
    } catch (err) {
      setError(errorDetail(err, "Failed to load the team budget"));
    } finally {
      setLoading(false);
    }
  }, [teamId]);

  useEffect(() => {
    if (teamId) load();
  }, [teamId, load]);

  const openEditor = () => {
    setForm({
      monthly_budget: report?.monthly_budget ?? "",
      default_app_allocation: report?.default_app_allocation ?? "",
      enforcement: report?.enforcement || "alert_only",
      budget_start_date: report?.budget_start_date ? report.budget_start_date.split("T")[0] : "",
    });
    setFormError("");
    setEditing(true);
  };

  const save = async () => {
    const budget = toNumberOrNull(form.monthly_budget);
    if (budget === null) {
      setFormError("Enter a monthly budget (0 is an empty pool), or remove the budget instead.");
      return;
    }
    setSaving(true);
    try {
      const next = await teamBudgetsService.setBudget(teamId, {
        monthly_budget: budget,
        default_app_allocation: toNumberOrNull(form.default_app_allocation),
        enforcement: form.enforcement,
        budget_start_date: form.budget_start_date ? new Date(form.budget_start_date).toISOString() : null,
      });
      setReport(next);
      setEditing(false);
    } catch (err) {
      setFormError(errorDetail(err, "Failed to save the team budget"));
    } finally {
      setSaving(false);
    }
  };

  const remove = async () => {
    setSaving(true);
    try {
      await teamBudgetsService.removeBudget(teamId);
      setEditing(false);
      await load();
    } catch (err) {
      setFormError(errorDetail(err, "Failed to remove the team budget"));
    } finally {
      setSaving(false);
    }
  };

  const reset = async () => {
    try {
      await teamBudgetsService.resetBudget(teamId);
      await load();
    } catch (err) {
      setError(errorDetail(err, "Failed to reset the budget period"));
    }
  };

  let body;
  if (loading && !report) {
    body = <CircularProgress size={24} />;
  } else if (error && !report) {
    body = <Alert severity="error">{error}</Alert>;
  } else if (report) {
    const budget = report.monthly_budget;
    const hasCeiling = report.managed && budget > 0;
    const spentPct = hasCeiling ? Math.min(100, (report.spent / budget) * 100) : 0;
    const allocPct = hasCeiling ? Math.min(100, (report.allocated / budget) * 100) : 0;

    body = (
      <Box sx={{ display: "flex", flexDirection: "column", gap: 2 }} data-testid="team-budget-panel">
        {error && <Alert severity="error">{error}</Alert>}
        {!report.enabled && (
          <Alert severity="info">
            Team budgets are switched off: spend is reported, but nothing is allocated or enforced. Switch them on from the Teams page.
          </Alert>
        )}
        {report.over_budget && (
          <Alert severity="error" data-testid="team-over-budget">
            The team has spent its budget for this period
            {report.enforcement === "hard_block" ? "; its Apps are refused until the period ends or the budget is raised or reset." : " (alert only; its Apps keep working)."}
          </Alert>
        )}
        {report.over_allocated && (
          <Alert severity="warning" data-testid="team-over-allocated">
            App allocations add up to {formatMoney(report.allocated)}, more than the team budget of {formatMoney(budget)}.
          </Alert>
        )}

        {!report.managed ? (
          <Typography variant="bodyLargeDefault" color="text.defaultSubdued">
            This team has no budget. It spent {formatMoney(report.spent)} this month.
          </Typography>
        ) : (
          <>
            <Box sx={{ display: "flex", flexWrap: "wrap", gap: 3 }}>
              <Stat label="Monthly budget" value={formatMoney(budget)} testId="team-budget-amount" />
              <Stat label="Spent this period" value={`${formatMoney(report.spent)}${hasCeiling ? ` (${report.usage.toFixed(0)}%)` : ""}`} testId="team-budget-spent" />
              <Stat label="Allocated to Apps" value={formatMoney(report.allocated)} testId="team-budget-allocated" />
              <Stat label="Unallocated" value={formatMoney(report.unallocated)} testId="team-budget-unallocated" />
              <Stat label="Default per new App" value={formatMoney(report.default_app_allocation ?? 0)} />
              <Stat label="When exceeded" value={report.enforcement === "hard_block" ? "Block the team's Apps" : "Alert only"} />
              <Stat label="Period" value={`${formatDate(report.period_start)} – ${formatDate(report.period_end)}`} />
            </Box>
            {hasCeiling && (
              <Box>
                <Typography variant="bodySmallDefault" color="text.defaultSubdued">Spent</Typography>
                <LinearProgress variant="determinate" value={spentPct} color={report.over_budget ? "error" : "primary"} sx={{ height: 8, borderRadius: 1, mb: 1 }} />
                <Typography variant="bodySmallDefault" color="text.defaultSubdued">Allocated</Typography>
                <LinearProgress variant="determinate" value={allocPct} color={report.over_allocated ? "warning" : "secondary"} sx={{ height: 8, borderRadius: 1 }} />
              </Box>
            )}
          </>
        )}

        {report.chat_spent > 0 && (
          <Typography variant="bodySmallDefault" color="text.defaultSubdued">
            Includes {formatMoney(report.chat_spent)} of chat by team members.
          </Typography>
        )}

        {report.apps?.length > 0 && (
          <Table size="small" data-testid="team-budget-apps">
            <TableHead>
              <TableRow>
                <TableCell>App</TableCell>
                <TableCell>Owner</TableCell>
                <TableCell align="right">Allocation</TableCell>
                <TableCell align="right">Spent this period</TableCell>
                <TableCell>Status</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {report.apps.map((row) => (
                <TableRow key={row.app_id}>
                  <TableCell>{row.name}</TableCell>
                  <TableCell>{row.owner_email || "—"}</TableCell>
                  <TableCell align="right">{row.deleted ? "—" : formatMoney(row.allocation)}</TableCell>
                  <TableCell align="right">{formatMoney(row.spent)}</TableCell>
                  <TableCell>{appStatus(row)}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}

        <Box sx={{ display: "flex", gap: 1 }}>
          <Button variant="outlined" onClick={openEditor} data-testid="team-budget-edit">
            {report.managed ? "Edit budget" : "Set budget"}
          </Button>
          {report.managed && (
            <Button variant="text" onClick={reset} data-testid="team-budget-reset">
              Start a new period now
            </Button>
          )}
        </Box>
      </Box>
    );
  }

  return (
    <CollapsibleSection title="Budget" defaultExpanded>
      {body}
      <Dialog open={editing} onClose={() => setEditing(false)} fullWidth maxWidth="sm">
        <DialogTitle>Team budget</DialogTitle>
        <DialogContent sx={{ display: "flex", flexDirection: "column", gap: 2, pt: "8px !important" }}>
          {formError && <Alert severity="error">{formError}</Alert>}
          <TextField
            label="Monthly budget"
            type="number"
            inputProps={{ step: "0.01", min: "0", "data-testid": "team-budget-input" }}
            value={form.monthly_budget}
            onChange={(e) => setForm({ ...form, monthly_budget: e.target.value })}
            InputProps={{ startAdornment: <InputAdornment position="start">$</InputAdornment> }}
            helperText="The most the team's Apps may spend together each period. 0 is an empty pool: new Apps get nothing."
          />
          <TextField
            label="Default allocation per new App"
            type="number"
            inputProps={{ step: "0.01", min: "0", "data-testid": "team-default-allocation-input" }}
            value={form.default_app_allocation}
            onChange={(e) => setForm({ ...form, default_app_allocation: e.target.value })}
            InputProps={{ startAdornment: <InputAdornment position="start">$</InputAdornment> }}
            helperText="Drawn from the unallocated pool when an App is created; capped at what is left."
          />
          <FormControl fullWidth>
            <InputLabel id="team-budget-enforcement">When the budget is reached</InputLabel>
            <Select
              labelId="team-budget-enforcement"
              label="When the budget is reached"
              value={form.enforcement}
              onChange={(e) => setForm({ ...form, enforcement: e.target.value })}
              inputProps={{ "data-testid": "team-enforcement-input" }}
            >
              <MenuItem value="alert_only">Alert administrators only</MenuItem>
              <MenuItem value="hard_block">Alert and block the team's Apps</MenuItem>
            </Select>
          </FormControl>
          <TextField
            label="Budget start date"
            type="date"
            value={form.budget_start_date}
            onChange={(e) => setForm({ ...form, budget_start_date: e.target.value })}
            InputLabelProps={{ shrink: true }}
            helperText="Periods start on this day of each month; empty means the 1st."
          />
        </DialogContent>
        <DialogActions>
          {report?.managed && (
            <Button color="error" onClick={remove} disabled={saving} sx={{ mr: "auto" }}>
              Remove budget
            </Button>
          )}
          <Button onClick={() => setEditing(false)} disabled={saving}>Cancel</Button>
          <Button variant="contained" onClick={save} disabled={saving} data-testid="team-budget-save">
            Save
          </Button>
        </DialogActions>
      </Dialog>
    </CollapsibleSection>
  );
};

export default TeamBudgetPanel;
