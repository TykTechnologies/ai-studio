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
  template_read: "API template",
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
        Tyk Connections is an Enterprise Edition feature that makes MCP servers managed by your
        Tyk API platform part of the AI Portal:
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
        <>Tyk Connections could not start: {status.disabled_reason}</>
      ) : (
        <>
          Tyk Connections is switched off. Set <code>TYK_MCP_ENABLED=true</code> on the AI Studio
          server to enable it.
        </>
      )}
    </Alert>
  </Box>
);

// --- Connection form state shared by the settings page and the Tools import wizard ---

export const emptyForm = {
  name: "",
  description: "",
  dashboard_url: "",
  dashboard_access_token: "",
  gateway_base_url: "",
  org_id: "",
  declared_mode: "catalogue",
  sync_interval_seconds: 300,
  allow_internal_host: false,
  template_id: "",
  auto_publish: false,
  default_privacy_score: "",
  accept_handoffs: true,
  alias_prefix: "studio:",
  expires_in_seconds: 0,
  mdcb_url: "",
  mdcb_access_token: "",
  mdcb_allow_internal_host: false,
  known_gateway_tags: "",
  gateway_base_urls: [],
};

const parseTags = (text) =>
  text
    .split(",")
    .map((t) => t.trim())
    .filter(Boolean)
    .map((tag) => ({ tag }));

const baseUrlsToMap = (rows) =>
  rows.reduce((acc, row) => {
    if (row.tag.trim()) acc[row.tag.trim()] = row.url.trim();
    return acc;
  }, {});

/**
 * formToInput turns the form state into the create/patch body. Tokens are
 * only sent when the user typed one; an empty token on edit keeps the stored
 * one. Exported so the test can assert the exact payload.
 */
export const formToInput = (form, editing) => {
  const input = {
    name: form.name,
    description: form.description,
    dashboard_url: form.dashboard_url,
    gateway_base_url: form.gateway_base_url,
    org_id: form.org_id,
    declared_mode: form.declared_mode,
    sync_interval_seconds: Number(form.sync_interval_seconds) || 0,
    allow_internal_host: Boolean(form.allow_internal_host),
    template_id: (form.template_id || "").trim(),
    auto_publish: Boolean(form.auto_publish),
    accept_handoffs: Boolean(form.accept_handoffs),
    key_defaults: { alias_prefix: form.alias_prefix || "studio:", expires_in_seconds: Number(form.expires_in_seconds) || 0 },
    mdcb_url: form.mdcb_url,
    mdcb_allow_internal_host: Boolean(form.mdcb_allow_internal_host),
    known_gateway_tags: parseTags(form.known_gateway_tags),
    gateway_base_urls: baseUrlsToMap(form.gateway_base_urls),
  };
  if (form.default_privacy_score !== "" && form.default_privacy_score !== null) {
    input.default_privacy_score = Number(form.default_privacy_score);
  } else if (editing) {
    input.clear_default_privacy_score = true;
  }
  if (form.dashboard_access_token) input.dashboard_access_token = form.dashboard_access_token;
  if (form.mdcb_access_token) input.mdcb_access_token = form.mdcb_access_token;
  return input;
};

export const connectionToForm = (connection) => ({
  ...emptyForm,
  name: connection.name || "",
  description: connection.description || "",
  dashboard_url: connection.dashboard_url || "",
  gateway_base_url: connection.gateway_base_url || "",
  org_id: connection.org_id || "",
  declared_mode: connection.declared_mode || "catalogue",
  sync_interval_seconds: connection.sync_interval_seconds || 300,
  allow_internal_host: Boolean(connection.allow_internal_host),
  template_id: connection.template_id || "",
  auto_publish: Boolean(connection.auto_publish),
  default_privacy_score:
    connection.default_privacy_score === null || connection.default_privacy_score === undefined
      ? ""
      : connection.default_privacy_score,
  accept_handoffs: connection.accept_handoffs !== false,
  alias_prefix: connection.key_defaults?.alias_prefix || "studio:",
  expires_in_seconds: connection.key_defaults?.expires_in_seconds || 0,
  mdcb_url: connection.mdcb_url || "",
  mdcb_allow_internal_host: Boolean(connection.mdcb_allow_internal_host),
  known_gateway_tags: (connection.known_gateway_tags || []).map((t) => t.tag).join(", "),
  gateway_base_urls: Object.entries(connection.gateway_base_urls || {}).map(([tag, url]) => ({ tag, url })),
});

// DataPlanes lists what MDCB reported, one line per data plane.
export const DataPlanes = ({ planes }) =>
  (planes || []).length > 0 ? (
    <Box sx={{ mt: 1 }}>
      <Typography variant="subtitle2">Data planes (MDCB)</Typography>
      {planes.map((dp) => (
        <Typography key={dp.group_id} variant="body2">
          {dp.group_id}: {dp.node_count} node(s), tags {(dp.tags || []).join(", ") || "none"}
          {dp.healthy ? "" : " (unhealthy)"}
        </Typography>
      ))}
    </Box>
  ) : null;

// ProbePanel shows what a probe learned about a Dashboard.
export const ProbePanel = ({ result }) => {
  if (!result) return null;
  return (
    <Box sx={{ mt: 2 }} data-testid="probe-panel">
      <Alert severity={result.reachable ? (result.effective_mode ? "success" : "warning") : "error"} sx={{ mb: 1 }}>
        {result.reachable
          ? result.effective_mode
            ? `Dashboard reachable. Effective mode: ${MODE_LABELS[result.effective_mode]}.`
            : "Dashboard reachable but not usable in any mode."
          : "Dashboard not reachable with these settings."}
        {result.org_id ? ` Organisation ${result.org_id}.` : ""}
      </Alert>
      <CapabilityChips capabilities={result.capabilities} />
      {(result.warnings || []).length > 0 && (
        <Box sx={{ mt: 1 }}>
          {result.warnings.map((w, i) => (
            <Typography key={i} variant="body2" color="warning.main">
              {w}
            </Typography>
          ))}
        </Box>
      )}
      <DataPlanes planes={result.data_planes} />
    </Box>
  );
};
