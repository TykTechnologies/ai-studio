import React from "react";
import { Box, FormControl, FormHelperText, InputLabel, ListItemText, MenuItem, Select, TextField } from "@mui/material";
import {
  PRIVACY_LEVELS,
  PRIVACY_MIN,
  PRIVACY_MAX,
  normalizePrivacyScore,
  privacyLevelForScore,
} from "./privacyLevels";

export const PRIVACY_RULE_HELPER =
  "LLM providers can only be used with tools and data sources at or below their level.";

/**
 * The one privacy control (UX review M4). A named level select and a 0–100
 * number kept in sync:
 *
 *   - choosing a level sets the number to that level's default score, unless
 *     the current number already sits inside the band;
 *   - typing a number moves the level to the band it falls in.
 *
 * `value` is the stored `privacy_score` (a number, or "" while the user
 * clears the field); `onChange` receives the new number. The number input
 * keeps `name="privacy_score"` so form handlers and e2e selectors that key
 * on it still work.
 */
export default function PrivacyLevelInput({
  value,
  onChange,
  label = "Privacy level",
  error = false,
  helperText,
  disabled = false,
  required = false,
  size = "medium",
  name = "privacy_score",
  levelSelectProps = {},
}) {
  const level = privacyLevelForScore(value);
  const levelKey = level ? level.key : "";
  const labelId = `${name}-level-label`;

  const handleLevelChange = (event) => {
    const next = PRIVACY_LEVELS.find((l) => l.key === event.target.value);
    if (!next) return;
    const current = normalizePrivacyScore(value);
    const inBand = current !== null && current >= next.min && current <= next.max;
    onChange(inBand ? current : next.defaultScore);
  };

  const handleNumberChange = (event) => {
    const raw = event.target.value;
    if (raw === "") {
      onChange("");
      return;
    }
    const parsed = parseInt(raw, 10);
    if (Number.isNaN(parsed)) return;
    onChange(Math.min(PRIVACY_MAX, Math.max(PRIVACY_MIN, parsed)));
  };

  return (
    <Box data-testid="privacy-level-input">
      <Box sx={{ display: "flex", gap: 2, alignItems: "flex-start", flexWrap: "wrap" }}>
        <FormControl sx={{ flex: "1 1 240px", minWidth: 200 }} size={size} error={error} disabled={disabled} required={required}>
          <InputLabel id={labelId}>{label}</InputLabel>
          <Select
            labelId={labelId}
            label={label}
            value={levelKey}
            onChange={handleLevelChange}
            displayEmpty={false}
            inputProps={{ "data-testid": "privacy-level-select", name: `${name}_level` }}
            renderValue={(key) => {
              const l = PRIVACY_LEVELS.find((x) => x.key === key);
              return l ? `${l.label} (${l.min}–${l.max})` : "";
            }}
            {...levelSelectProps}
          >
            {PRIVACY_LEVELS.map((l) => (
              <MenuItem key={l.key} value={l.key} data-privacy-level={l.key}>
                <ListItemText primary={`${l.label} (${l.min}–${l.max})`} secondary={l.description} />
              </MenuItem>
            ))}
          </Select>
        </FormControl>
        <TextField
          type="number"
          label="Score"
          name={name}
          value={value === null || value === undefined ? "" : value}
          onChange={handleNumberChange}
          error={error}
          disabled={disabled}
          required={required}
          size={size}
          inputProps={{ min: PRIVACY_MIN, max: PRIVACY_MAX, step: 1, "aria-label": `${label} score` }}
          sx={{ width: 120 }}
        />
      </Box>
      <FormHelperText error={error} sx={{ mx: 0 }}>
        {helperText || PRIVACY_RULE_HELPER}
      </FormHelperText>
    </Box>
  );
}
