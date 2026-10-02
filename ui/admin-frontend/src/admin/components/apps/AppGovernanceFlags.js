import React from "react";
import { Box, Chip, Tooltip } from "@mui/material";

// Governance flags are raised on an App by governance plugins through the
// apps.lifecycle scope (an asset catalog marking it ownerless, deprecated or
// with a lapsed review). They are informational and read-only here: the
// plugin that raised a flag clears it.

// Flags that record a link rather than a problem.
const INFO_FLAGS = new Set(["governed_by_asset"]);

const humanize = (name) =>
  name.replace(/_/g, " ").replace(/^\w/, (c) => c.toUpperCase());

const describe = (flag) => {
  const parts = [];
  if (flag.reason) parts.push(flag.reason);
  if (flag.set_by) parts.push(`Set by ${flag.set_by}`);
  if (flag.at) {
    const at = new Date(flag.at);
    if (!Number.isNaN(at.getTime())) parts.push(at.toLocaleString());
  }
  return parts.join(" · ");
};

const AppGovernanceFlags = ({ metadata }) => {
  const flags = metadata && metadata.governance_flags;
  if (!flags || typeof flags !== "object") return null;
  const names = Object.keys(flags).sort();
  if (names.length === 0) return null;
  return (
    <Box display="flex" flexWrap="wrap" gap={1} data-testid="app-governance-flags">
      {names.map((name) => {
        const flag = flags[name] || {};
        const value = typeof flag.value === "string" ? flag.value : "";
        return (
          <Tooltip key={name} title={describe(flag)}>
            <Chip
              size="small"
              variant="outlined"
              color={INFO_FLAGS.has(name) ? "default" : "warning"}
              label={value ? `${humanize(name)}: ${value}` : humanize(name)}
            />
          </Tooltip>
        );
      })}
    </Box>
  );
};

export default AppGovernanceFlags;
