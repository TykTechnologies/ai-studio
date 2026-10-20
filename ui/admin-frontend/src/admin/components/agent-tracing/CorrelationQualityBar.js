import React from "react";
import { Box, Tooltip, Typography } from "@mui/material";
import { useTheme } from "@mui/material/styles";
import { confidenceShares } from "./sessionFilters";

// How the session's spans were linked (spec §3.3): declared by the client,
// matched exactly, or inferred strongly or weakly. Colour is not the only
// signal: the tooltip and the accessible label give the counts.

const LABELS = { declared: "Declared", exact: "Exact", strong: "Inferred", weak: "Inferred (weak)" };

const CorrelationQualityBar = ({ counts, width = 96 }) => {
  const theme = useTheme();
  const colours = {
    declared: theme.palette.success.main,
    exact: theme.palette.success.light,
    strong: theme.palette.warning.light,
    weak: theme.palette.warning.dark,
  };
  const shares = confidenceShares(counts);
  const total = shares.reduce((sum, s) => sum + s.count, 0);
  const summary = total
    ? shares.filter((s) => s.count).map((s) => `${LABELS[s.key]} ${s.count}`).join(", ")
    : "No linked spans";
  return (
    <Tooltip title={summary}>
      <Box
        role="img"
        aria-label={`Correlation: ${summary}`}
        data-testid="correlation-quality-bar"
        sx={{ display: "flex", width, height: 8, borderRadius: 1, overflow: "hidden", bgcolor: "action.hover" }}
      >
        {shares
          .filter((s) => s.share > 0)
          .map((s) => (
            <Box key={s.key} data-key={s.key} sx={{ width: `${s.share * 100}%`, bgcolor: colours[s.key] }} />
          ))}
        {!total && (
          <Typography variant="caption" sx={{ position: "absolute", clip: "rect(0 0 0 0)" }}>
            {summary}
          </Typography>
        )}
      </Box>
    </Tooltip>
  );
};

export default CorrelationQualityBar;
