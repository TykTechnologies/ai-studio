import React from "react";
import { Box, Chip } from "@mui/material";

const formatValue = (item) => {
  const { value } = item;
  if (Array.isArray(value)) return value.join(", ");
  if (typeof value === "boolean") return value ? "Yes" : "No";
  if (value === null || value === undefined) return "";
  return String(value);
};

/**
 * Renders the portal-visible governed metadata (Enterprise) an object carries,
 * as "Label: value" chips. `items` is the display-ready list the backend
 * emits at data[i].governed_metadata: [{ key, label, type, value }].
 */
const GovernedMetadataBadges = ({ items, sx }) => {
  if (!Array.isArray(items) || items.length === 0) return null;
  return (
    <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.5, ...sx }} data-testid="governed-metadata-badges">
      {items.map((item) => (
        <Chip
          key={item.key}
          size="small"
          variant="outlined"
          label={`${item.label || item.key}: ${formatValue(item)}`}
        />
      ))}
    </Box>
  );
};

export default GovernedMetadataBadges;
