import React from "react";
import { Box, Typography } from "@mui/material";
import { describePlacement, isConst, utf8Length } from "./promptTemplateText";

// Where a template sits in a message: its constants and slots in order,
// with the text the anchor lets through before or after it.

const blockSx = {
  px: 1,
  py: 0.5,
  borderRadius: 1,
  fontFamily: "monospace",
  fontSize: "0.8rem",
  border: "1px solid",
};

const PlacementDiagram = ({ placement, parts }) => {
  const anchor = placement?.anchor || "both";
  let constIndex = 0;
  return (
    <Box data-testid="placement-diagram">
      <Typography variant="body2" sx={{ mb: 1 }}>
        {describePlacement(placement)}
      </Typography>
      <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.5, alignItems: "center" }}>
        <Typography variant="caption" color="text.secondary" sx={{ mr: 0.5 }}>
          {placement?.role || "user"} message:
        </Typography>
        {anchor === "end" && (
          <Box sx={{ ...blockSx, borderStyle: "dashed", color: "text.secondary" }}>any text (scored)</Box>
        )}
        {(parts || []).map((p, i) => {
          if (isConst(p)) {
            constIndex += 1;
            return (
              <Box key={i} sx={{ ...blockSx, borderColor: "success.main", bgcolor: "rgba(46, 125, 50, 0.08)" }}>
                constant {constIndex} ({utf8Length(p.const)} bytes, exempt)
              </Box>
            );
          }
          return (
            <Box key={i} sx={{ ...blockSx, borderColor: "warning.main", bgcolor: "rgba(237, 108, 2, 0.08)" }}>
              {"{"}
              {p.slot}
              {"}"} (scored)
            </Box>
          );
        })}
        {anchor === "start" && (
          <Box sx={{ ...blockSx, borderStyle: "dashed", color: "text.secondary" }}>any text (scored)</Box>
        )}
      </Box>
    </Box>
  );
};

export default PlacementDiagram;
