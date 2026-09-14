import React from "react";
import { Chip, Tooltip } from "@mui/material";
import { formatPrivacyLevel, privacyLevelForScore, normalizePrivacyScore } from "./privacyLevels";

const COLOURS = {
  public: "success",
  internal: "info",
  confidential: "warning",
  restricted: "error",
};

/**
 * Shows a privacy score as its named level plus the number ("Internal · 40")
 * so lists, detail pages and the portal all read the same way. Renders
 * "Not set" for a missing score.
 */
export default function PrivacyLevelChip({ score, size = "small", showDescription = true, ...props }) {
  const normalized = normalizePrivacyScore(score);
  const level = privacyLevelForScore(normalized);
  const label = formatPrivacyLevel(normalized);
  const chip = (
    <Chip
      size={size}
      variant="outlined"
      color={level ? COLOURS[level.key] : "default"}
      label={label}
      data-testid="privacy-level-chip"
      data-privacy-level={level ? level.key : "unset"}
      {...props}
    />
  );
  if (!level || !showDescription) return chip;
  return <Tooltip title={`${level.label} (${level.min}–${level.max}): ${level.description}`}>{chip}</Tooltip>;
}
