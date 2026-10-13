import React from "react";
import { Box, Button, Grid, IconButton, TextField, Tooltip, Typography } from "@mui/material";
import AddIcon from "@mui/icons-material/Add";
import DeleteIcon from "@mui/icons-material/Delete";

// Service tiers each vendor bills at a price multiplier (IPriceAdjustedResponse);
// the first is the suggested first row. The server accepts any tier name.
export const PRICE_TIERS_BY_VENDOR = {
  anthropic: ["batch", "standard", "priority"],
  openai: ["flex", "priority", "scale"],
};

// "default" and "auto" (OpenAI's list-price tiers) are recorded as no tier, so
// an override keyed on them could never apply; the server rejects them for
// every vendor.
const LIST_PRICE_TIERS = ["default", "auto"];

// Mirrors the server's price_multipliers rules.
const TIER_PATTERN = /^[a-z0-9_-]{1,32}$/;
const MAX_MULTIPLIER = 10;

export const rowsFromPriceMultipliers = (map) =>
  Object.entries(map || {}).map(([tier, value]) => ({ tier, value: String(value) }));

/**
 * Checks the editor rows and builds the price_multipliers map. Fully empty
 * rows are ignored; no overrides gives null, which clears them on PATCH.
 */
export const validatePriceMultipliers = (rows) => {
  const rowErrors = {};
  const map = {};
  const seen = new Set();
  rows.forEach((row, i) => {
    const tier = row.tier.trim();
    const raw = String(row.value).trim();
    if (!tier && !raw) return;
    const err = {};
    if (!tier) err.tier = "Enter a tier";
    else if (!TIER_PATTERN.test(tier)) err.tier = "1-32 lowercase letters, digits, '_' or '-'";
    else if (LIST_PRICE_TIERS.includes(tier))
      err.tier = `"${tier}" is billed at list price and recorded as no tier, so it has no multiplier to override`;
    else if (seen.has(tier)) err.tier = "This tier is listed more than once";
    if (tier) seen.add(tier);
    const value = Number(raw);
    if (!raw) err.value = "Enter a multiplier or remove the row";
    else if (!Number.isFinite(value) || value < 0 || value > MAX_MULTIPLIER)
      err.value = `Must be a number between 0 and ${MAX_MULTIPLIER}`;
    if (Object.keys(err).length) {
      rowErrors[i] = err;
    } else {
      map[tier] = value;
    }
  });
  return { rowErrors, map: Object.keys(map).length ? map : null };
};

/**
 * Per-LLM price multiplier overrides by service tier (negotiated pricing),
 * e.g. {"batch": 0.4}. Shown for vendors with priced tiers, and for any LLM
 * that already has overrides so they can be removed.
 */
const LLMPriceMultipliersField = ({ vendor, rows, onChange, rowErrors = {}, serverError }) => {
  const suggestions = PRICE_TIERS_BY_VENDOR[vendor] || [];
  if (!suggestions.length && rows.length === 0) return null;

  const update = (i, field, value) =>
    onChange(rows.map((row, j) => (j === i ? { ...row, [field]: value } : row)));

  const add = () => {
    const used = new Set(rows.map((r) => r.tier));
    const tier = suggestions.find((t) => !used.has(t)) || "";
    onChange([...rows, { tier, value: "" }]);
  };

  return (
    <Box data-testid="llm-price-multipliers">
      <Typography variant="subtitle1">Price multipliers</Typography>
      <Typography variant="body2" color="text.secondary" paragraph>
        Overrides the vendor's price multiplier for usage billed at a service tier on this LLM, for
        negotiated pricing: 1 is list price, 0 is not billed, and Anthropic batch usage defaults to
        0.5. Tiers not listed keep the vendor rate. Applies to usage recorded after you save (not
        retroactive).
      </Typography>
      <datalist id="llm-price-tiers">
        {suggestions.map((t) => (
          <option key={t} value={t} />
        ))}
      </datalist>
      {rows.map((row, i) => (
        <Grid container spacing={2} key={i} sx={{ mb: 1 }} alignItems="flex-start">
          <Grid item xs={6} md={4}>
            <TextField
              fullWidth
              size="small"
              label={`Tier ${i + 1}`}
              value={row.tier}
              onChange={(e) => update(i, "tier", e.target.value)}
              inputProps={{ list: "llm-price-tiers" }}
              error={!!rowErrors[i]?.tier}
              helperText={rowErrors[i]?.tier}
            />
          </Grid>
          <Grid item xs={5} md={3}>
            <TextField
              fullWidth
              size="small"
              type="number"
              label={`Multiplier ${i + 1}`}
              value={row.value}
              onChange={(e) => update(i, "value", e.target.value)}
              inputProps={{ min: 0, max: MAX_MULTIPLIER, step: "any" }}
              error={!!rowErrors[i]?.value}
              helperText={rowErrors[i]?.value}
            />
          </Grid>
          <Grid item xs={1}>
            <Tooltip title="Remove">
              <IconButton
                aria-label={`Remove price override ${i + 1}`}
                onClick={() => onChange(rows.filter((_, j) => j !== i))}
              >
                <DeleteIcon />
              </IconButton>
            </Tooltip>
          </Grid>
        </Grid>
      ))}
      {serverError && (
        <Typography variant="body2" color="error" sx={{ mb: 1 }}>
          {serverError}
        </Typography>
      )}
      <Button startIcon={<AddIcon />} onClick={add} size="small">
        Add price override
      </Button>
    </Box>
  );
};

export default LLMPriceMultipliersField;
