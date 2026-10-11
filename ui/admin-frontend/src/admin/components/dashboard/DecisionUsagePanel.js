import React, { useEffect, useState } from "react";
import {
  Alert,
  Box,
  Grid,
  Paper,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
  Typography,
} from "@mui/material";
import apiClient from "../../utils/apiClient";

const formatCost = (value) =>
  value === undefined || value === null ? "-" : `$${Number(value).toFixed(4)}`;

const Stat = ({ label, value, testId }) => (
  <Grid item xs={6} sm={3}>
    <Typography variant="body2" color="text.secondary">
      {label}
    </Typography>
    <Typography variant="h6" data-testid={testId}>
      {value}
    </Typography>
  </Grid>
);

// DecisionUsagePanel answers "what are decision models doing for me?" over the
// dashboard's date range: decision calls (/v1/decisions, /v1/systemone), the
// questions they asked, how many the models refused, and what they cost.
// Decision spend is also in the dashboard's cost totals; this splits it out.
// The panel stays hidden until there is decision traffic.
const DecisionUsagePanel = ({ startDate, endDate }) => {
  const [usage, setUsage] = useState(null);
  const [error, setError] = useState("");

  useEffect(() => {
    let cancelled = false;
    apiClient
      .get("/analytics/decisions", { params: { start_date: startDate, end_date: endDate } })
      .then((response) => {
        if (cancelled) return;
        setUsage(response.data);
        setError("");
      })
      .catch(() => {
        if (!cancelled) setError("Failed to load decision model usage");
      });
    return () => {
      cancelled = true;
    };
  }, [startDate, endDate]);

  if (error) {
    return (
      <Paper elevation={3} sx={{ p: 2, mt: 3 }} data-testid="decision-usage">
        <Alert severity="error">{error}</Alert>
      </Paper>
    );
  }
  if (!usage || !usage.totals || usage.totals.requests === 0) return null;

  const { totals } = usage;
  const totalCost = Object.values(totals.cost_by_currency || {}).reduce((a, b) => a + b, 0);
  const refusalRate = totals.questions > 0 ? (totals.refusals / totals.questions) * 100 : 0;

  return (
    <Paper elevation={3} sx={{ p: 2, mt: 3 }} data-testid="decision-usage">
      <Typography variant="h6" gutterBottom>
        Decision Models
      </Typography>
      <Grid container spacing={2} sx={{ mb: 2 }}>
        <Stat label="Decision calls" value={totals.requests.toLocaleString()} testId="decision-usage-requests" />
        <Stat label="Questions answered" value={`${totals.answered.toLocaleString()} / ${totals.questions.toLocaleString()}`} testId="decision-usage-questions" />
        <Stat label="Refusal rate" value={`${refusalRate.toFixed(1)}%`} testId="decision-usage-refusals" />
        <Stat label="Cost" value={formatCost(totalCost)} testId="decision-usage-cost" />
      </Grid>
      <Box sx={{ overflowX: "auto" }}>
        <Table size="small">
          <TableHead>
            <TableRow>
              <TableCell>Model</TableCell>
              <TableCell align="right">Calls</TableCell>
              <TableCell align="right">Questions</TableCell>
              <TableCell align="right">Refused</TableCell>
              <TableCell align="right">Input tokens</TableCell>
              <TableCell align="right">Cost</TableCell>
              <TableCell align="right">Cost / 1k calls</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {(usage.by_model || []).map((m) => (
              <TableRow key={`${m.vendor}/${m.model}/${m.currency}`}>
                <TableCell>
                  {m.model}
                  <Typography component="span" variant="caption" color="text.secondary">
                    {` (${m.vendor})`}
                  </Typography>
                </TableCell>
                <TableCell align="right">{m.requests.toLocaleString()}</TableCell>
                <TableCell align="right">{m.questions.toLocaleString()}</TableCell>
                <TableCell align="right">{m.refusals.toLocaleString()}</TableCell>
                <TableCell align="right">{m.input_tokens.toLocaleString()}</TableCell>
                <TableCell align="right">{formatCost(m.cost)}</TableCell>
                <TableCell align="right">{formatCost(m.cost_per_thousand)}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </Box>
    </Paper>
  );
};

export default DecisionUsagePanel;
