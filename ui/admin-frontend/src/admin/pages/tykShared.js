import React from "react";
import { Alert, Box, Chip, Tooltip, Typography } from "@mui/material";

export const CONNECTION_MODES = ["catalogue", "broker", "full"];

export const MODE_LABELS = {
  catalogue: "Catalogue",
  broker: "Broker",
  full: "Full",
};

export const MODE_HELP = {
  catalogue: "Import MCP proxies and policies. Registrations become handoff packages.",
  broker: "Catalogue plus minting and revoking Tyk keys against pinned policies.",
  full: "Broker plus creating MCP proxies and policies on the Dashboard.",
};

export const CAPABILITY_LABELS = {
  mcp_supported: "MCP proxies",
  rest_to_mcp_supported: "REST API to MCP",
  mcp_read: "Read MCP proxies",
  policies_read: "Read policies",
  apis_read: "Read APIs",
  keys_write: "Mint keys",
  keys_read_by_hash: "Read keys",
  key_delete_by_hash: "Delete keys",
  mcp_write: "Create MCP proxies",
  policies_write: "Create policies",
  mdcb_read: "MDCB data planes",
};

export const connectionStatusColor = (status, degraded) => {
  if (degraded) return "error";
  switch (status) {
    case "active":
      return "success";
    case "pending":
      return "warning";
    case "disabled":
      return "default";
    default:
      return "default";
  }
};

export const capabilityColor = (state) => {
  switch (state) {
    case "ok":
      return "success";
    case "unverified":
      return "warning";
    case "denied":
      return "error";
    default:
      return "default";
  }
};

export const ConnectionStatusChip = ({ status, degraded }) => (
  <Chip
    label={degraded ? `${status} (degraded)` : status}
    size="small"
    color={connectionStatusColor(status, degraded)}
    data-testid={`connection-status-${status}`}
  />
);

export const ModeChip = ({ declared, effective }) => {
  const capped = effective && declared && effective !== declared;
  return (
    <Tooltip title={MODE_HELP[effective || declared] || ""}>
      <Chip
        label={capped ? `${MODE_LABELS[effective]} (declared ${MODE_LABELS[declared]})` : MODE_LABELS[effective || declared] || declared}
        size="small"
        color={capped ? "warning" : "primary"}
        variant="outlined"
        data-testid="mode-chip"
      />
    </Tooltip>
  );
};

// CapabilityChips renders the probe result compactly. Unknown keys are
// shown by name so a newer server never hides information.
export const CapabilityChips = ({ capabilities }) => {
  const entries = Object.entries(capabilities || {});
  if (entries.length === 0) {
    return (
      <Typography variant="body2" color="text.secondary">
        Not probed yet.
      </Typography>
    );
  }
  return (
    <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.5 }}>
      {entries.map(([key, cap]) => (
        <Tooltip key={key} title={cap.detail || ""}>
          <Chip
            size="small"
            label={`${CAPABILITY_LABELS[key] || key}: ${cap.state}`}
            color={capabilityColor(cap.state)}
            variant={cap.state === "ok" ? "filled" : "outlined"}
            data-testid={`capability-${key}`}
          />
        </Tooltip>
      ))}
    </Box>
  );
};

export const TykUpsell = () => (
  <Box sx={{ p: 3 }}>
    <Alert severity="info">
      <Typography variant="h6" gutterBottom>
        Enterprise Feature
      </Typography>
      <Typography>
        The Tyk Dashboard integration is an Enterprise Edition feature that makes MCP servers
        managed by your Tyk API platform part of the AI Portal:
      </Typography>
      <ul>
        <li>Discover MCP proxies defined in a Tyk Dashboard and publish them as portal assets</li>
        <li>Register MCP servers from AI Studio or through community submissions</li>
        <li>Mint, rotate and revoke Tyk access keys for Apps against pinned security policies</li>
        <li>An audited record of who has access to which MCP server</li>
      </ul>
      <Typography sx={{ mt: 2 }}>
        Visit{" "}
        <a href="https://tyk.io/ai-studio/pricing" target="_blank" rel="noopener noreferrer">
          tyk.io/ai-studio/pricing
        </a>{" "}
        for more information.
      </Typography>
    </Alert>
  </Box>
);

export const TykDisabledNotice = ({ status }) => (
  <Box sx={{ p: 3 }}>
    <Alert severity="warning">
      {status?.disabled_reason ? (
        <>The Tyk Dashboard integration could not start: {status.disabled_reason}</>
      ) : (
        <>
          The Tyk Dashboard integration is switched off. Set <code>TYK_MCP_ENABLED=true</code> on the
          AI Studio server to enable it.
        </>
      )}
    </Alert>
  </Box>
);
