import React from "react";
import { FormControl, FormHelperText, InputLabel, MenuItem, Select } from "@mui/material";

// A select over JSON:API rows, by id. `rows` must include the current value
// for it to render with a label.
const RowSelect = ({ id, label, value, onChange, rows, error, helperText, emptyLabel, size }) => (
  <FormControl fullWidth error={Boolean(error)} size={size}>
    <InputLabel id={`${id}-label`}>{label}</InputLabel>
    <Select labelId={`${id}-label`} value={value ? String(value) : ""} label={label} onChange={(e) => onChange(e.target.value)}>
      {emptyLabel && (
        <MenuItem value="">
          <em>{emptyLabel}</em>
        </MenuItem>
      )}
      {rows.map((row) => (
        <MenuItem key={row.id} value={String(row.id)}>
          {row.attributes?.name || `#${row.id}`}
          {row.attributes?.vendor ? ` (${row.attributes.vendor})` : ""}
        </MenuItem>
      ))}
    </Select>
    {(error || helperText) && <FormHelperText>{error || helperText}</FormHelperText>}
  </FormControl>
);

export default RowSelect;
