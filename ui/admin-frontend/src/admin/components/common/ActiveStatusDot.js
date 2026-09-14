import React from "react";
import { Box, Tooltip } from "@mui/material";
import FiberManualRecordIcon from "@mui/icons-material/FiberManualRecord";

/**
 * The green/red Active dot used in list columns, the same idiom as the LLM
 * list's CredentialStatusDot (which adds a credential warning on top). Kept
 * separate so tools, data sources and model routers share one rendering.
 */
const ActiveStatusDot = ({
  active,
  activeLabel = "Active",
  inactiveLabel = "Inactive",
  showLabel = false,
}) => {
  const label = active ? activeLabel : inactiveLabel;
  return (
    <Box
      sx={{ display: "flex", alignItems: "center", gap: 1 }}
      data-testid="active-status-dot"
      data-active={active ? "true" : "false"}
    >
      <Tooltip title={label}>
        <FiberManualRecordIcon
          sx={{ color: active ? "green" : "red", fontSize: showLabel ? 12 : undefined }}
          titleAccess={label}
        />
      </Tooltip>
      {showLabel && label}
    </Box>
  );
};

export default ActiveStatusDot;
