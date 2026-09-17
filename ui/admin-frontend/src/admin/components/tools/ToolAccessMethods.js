import React, { useState } from "react";
import {
  Alert,
  Box,
  Chip,
  IconButton,
  Stack,
  Switch,
  Tooltip,
  Typography,
} from "@mui/material";
import ContentCopyIcon from "@mui/icons-material/ContentCopy";
import { copyToClipboard } from "../../utils/clipboardUtils";

// A tool is a chat capability first. Reaching it from an App, over REST or
// over MCP on the AI Studio gateway, is optional and switched per tool. This
// block is the one place the admin UI explains that: the tool form and the
// import wizard render it with switches, the detail page renders it read only.
//
// Both off means "chat only": the tool is not shown in the portal and cannot
// be added to an App.

export const ACCESS_METHOD_COPY = {
  intro:
    "A tool is always available to chats and agents. Turn on an access method to let Apps call it on the AI Studio gateway with their App credential. The tool then also appears in the portal.",
  chat: "Used by chats and agents that have this tool. Always on.",
  rest: "Apps call the tool's operations over HTTP.",
  mcp: "MCP clients such as Claude Desktop connect to the tool's MCP endpoint, served by AI Studio. This is separate from the MCP servers section, which lists servers behind a Tyk Gateway.",
  chatOnly:
    "Chat only. This tool is not shown in the portal and cannot be added to Apps.",
  switchedOffWarning:
    "Apps that already use this tool keep their binding, but the gateway refuses calls over a method that is off.",
};

export const accessMethodLabels = (attributes = {}) => {
  const labels = ["Chat"];
  if (attributes.rest_access_enabled) labels.push("REST");
  if (attributes.mcp_access_enabled) labels.push("MCP");
  return labels;
};

// Compact chips for list rows and headers.
export const AccessMethodChips = ({ attributes }) => (
  <Stack direction="row" spacing={0.5} data-testid="tool-access-chips">
    {accessMethodLabels(attributes).map((label) => (
      <Chip
        key={label}
        label={label}
        size="small"
        variant={label === "Chat" ? "outlined" : "filled"}
        color={label === "Chat" ? "default" : "primary"}
      />
    ))}
  </Stack>
);

const EndpointLine = ({ url, label }) => {
  const [copied, setCopied] = useState(false);
  if (!url) return null;
  const handleCopy = () =>
    copyToClipboard(url, label, () => {
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    });
  return (
    <Box sx={{ display: "flex", alignItems: "center", mt: 0.5, minWidth: 0 }}>
      <Typography
        component="code"
        sx={{
          fontFamily: "monospace",
          fontSize: "0.85rem",
          overflowWrap: "anywhere",
          color: "text.primary",
        }}
      >
        {url}
      </Typography>
      <Tooltip title={copied ? "Copied" : `Copy ${label}`}>
        <IconButton size="small" onClick={handleCopy} aria-label={`Copy ${label}`} sx={{ ml: 0.5 }}>
          <ContentCopyIcon fontSize="inherit" />
        </IconButton>
      </Tooltip>
    </Box>
  );
};

const MethodRow = ({ name, title, description, control, url, urlLabel, showUrl }) => (
  <Box
    data-testid={`tool-access-${name}`}
    sx={{
      display: "flex",
      alignItems: "flex-start",
      gap: 2,
      py: 1.5,
      borderTop: "1px solid",
      borderColor: "divider",
      "&:first-of-type": { borderTop: "none" },
    }}
  >
    <Box sx={{ width: 64, flexShrink: 0, pt: 0.25 }}>{control}</Box>
    <Box sx={{ minWidth: 0 }}>
      <Typography variant="subtitle2">{title}</Typography>
      <Typography variant="body2" color="text.secondary">
        {description}
      </Typography>
      {showUrl && <EndpointLine url={url} label={urlLabel} />}
    </Box>
  </Box>
);

const OnOffChip = ({ on }) => (
  <Chip label={on ? "On" : "Off"} size="small" color={on ? "success" : "default"} variant={on ? "filled" : "outlined"} />
);

/**
 * @param {boolean} restEnabled / mcpEnabled  current values
 * @param {(name: "rest_access_enabled"|"mcp_access_enabled", value: boolean) => void} onChange
 *        omit for a read-only block
 * @param {string} restUrl / mcpUrl  gateway URLs from the API (absent for a tool not yet saved)
 * @param {boolean} existing  the tool already exists, so switching a method off can affect Apps
 */
const ToolAccessMethods = ({
  restEnabled = false,
  mcpEnabled = false,
  onChange,
  restUrl = "",
  mcpUrl = "",
  existing = false,
  disabled = false,
}) => {
  const readOnly = !onChange;
  const chatOnly = !restEnabled && !mcpEnabled;

  const control = (name, checked, label) =>
    readOnly ? (
      <OnOffChip on={checked} />
    ) : (
      <Switch
        checked={checked}
        disabled={disabled}
        onChange={(e) => onChange(name, e.target.checked)}
        inputProps={{ "aria-label": label }}
      />
    );

  return (
    <Box data-testid="tool-access-methods">
      <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
        {ACCESS_METHOD_COPY.intro}
      </Typography>
      <MethodRow
        name="chat"
        title="Chat"
        description={ACCESS_METHOD_COPY.chat}
        control={<Chip label="Always" size="small" variant="outlined" />}
      />
      <MethodRow
        name="rest"
        title="REST API"
        description={ACCESS_METHOD_COPY.rest}
        control={control("rest_access_enabled", restEnabled, "REST API access")}
        url={restUrl}
        urlLabel="REST endpoint"
        showUrl={restEnabled}
      />
      <MethodRow
        name="mcp"
        title="MCP"
        description={ACCESS_METHOD_COPY.mcp}
        control={control("mcp_access_enabled", mcpEnabled, "MCP access")}
        url={mcpUrl}
        urlLabel="MCP endpoint"
        showUrl={mcpEnabled}
      />
      {chatOnly && (
        <Alert severity="info" sx={{ mt: 1 }} data-testid="tool-chat-only">
          {ACCESS_METHOD_COPY.chatOnly}
        </Alert>
      )}
      {!readOnly && existing && (
        <Typography variant="caption" color="text.secondary" display="block" sx={{ mt: 1 }}>
          {ACCESS_METHOD_COPY.switchedOffWarning}
        </Typography>
      )}
    </Box>
  );
};

export default ToolAccessMethods;
