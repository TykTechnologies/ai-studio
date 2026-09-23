import React from "react";
import {
  Box,
  FormControl,
  FormHelperText,
  InputAdornment,
  InputLabel,
  MenuItem,
  Select,
  TextField,
} from "@mui/material";

// BudgetField edits a monthly budget where "not set" and 0 are different
// things: null is the empty choice (no limit, or on a new App the default
// that applies), and a number is a limit. 0 is a real limit: nothing may be
// spent and requests are refused.
const BudgetField = ({
  label = "Monthly budget",
  value,
  onChange,
  disabled = false,
  emptyLabel = "No limit",
  emptyHelp = "Spending is not capped.",
  testIdPrefix = "budget",
}) => {
  const hasLimit = value !== null && value !== undefined;
  const mode = hasLimit ? "amount" : "empty";

  const setMode = (next) => {
    if (next === "empty") onChange(null);
    else if (!hasLimit) onChange(0);
  };

  const setAmount = (raw) => {
    const n = raw === "" ? 0 : parseFloat(raw);
    onChange(Number.isNaN(n) ? 0 : Math.max(0, n));
  };

  let help = emptyHelp;
  if (hasLimit) {
    help = Number(value) === 0
      ? "A budget of 0: nothing may be spent, and requests are refused."
      : "Requests are refused once this much has been spent in the period.";
  }

  return (
    <Box sx={{ display: "flex", gap: 2, alignItems: "flex-start" }}>
      <FormControl sx={{ minWidth: 180 }} disabled={disabled}>
        <InputLabel id={`${testIdPrefix}-mode-label`}>{label}</InputLabel>
        <Select
          labelId={`${testIdPrefix}-mode-label`}
          label={label}
          value={mode}
          onChange={(e) => setMode(e.target.value)}
          inputProps={{ "data-testid": `${testIdPrefix}-mode` }}
        >
          <MenuItem value="empty">{emptyLabel}</MenuItem>
          <MenuItem value="amount">Fixed amount</MenuItem>
        </Select>
        <FormHelperText data-testid={`${testIdPrefix}-help`}>{help}</FormHelperText>
      </FormControl>
      {hasLimit && (
        <TextField
          label="Amount"
          type="number"
          value={value}
          onChange={(e) => setAmount(e.target.value)}
          disabled={disabled}
          inputProps={{ step: "0.01", min: "0", "data-testid": `${testIdPrefix}-amount` }}
          InputProps={{ startAdornment: <InputAdornment position="start">$</InputAdornment> }}
          sx={{ flex: 1 }}
        />
      )}
    </Box>
  );
};

export default BudgetField;
