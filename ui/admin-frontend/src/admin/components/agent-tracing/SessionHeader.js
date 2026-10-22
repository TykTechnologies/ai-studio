import React, { useState } from "react";
import { Box, Button, Chip, Menu, MenuItem, Stack, Tooltip, Typography } from "@mui/material";
import ArrowDropDownIcon from "@mui/icons-material/ArrowDropDown";
import LinkIcon from "@mui/icons-material/Link";
import { useSessionDetail } from "./SessionDetailContext";
import CorrelationQualityBar from "./CorrelationQualityBar";
import SessionUser from "./SessionUser";
import { formatCost, formatDuration, formatTokens, shortId } from "./sessionFilters";
import agentTracingService from "../../services/agentTracingService";
import { copyToClipboard } from "../../utils/clipboardUtils";

// Header and summary strip of the session detail (spec §6.2 layout).

const STATUS_COLOURS = { active: "info", closed: "default", error: "error" };

const CONTENT_MODE_LEGEND = {
  off: "No content was captured",
  metadata: "Sizes and types only, no content",
  hashed: "Fingerprints only, no content",
  redacted: "Content with personal data removed",
  full: "Content as sent",
};

const Fact = ({ label, children }) => (
  <Box>
    <Typography variant="caption" color="text.secondary" display="block">
      {label}
    </Typography>
    <Typography variant="body2" component="div">
      {children}
    </Typography>
  </Box>
);

const download = (blob, filename) => {
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  a.remove();
  URL.revokeObjectURL(url);
};

export const SessionHeader = ({ onNotify }) => {
  const { detail, viewer } = useSessionDetail();
  const s = detail.session;
  const [anchor, setAnchor] = useState(null);

  const runExport = async (format, content) => {
    setAnchor(null);
    try {
      const blob = await agentTracingService.exportSession(s.id, format, content);
      download(blob, `agent-session-${shortId(s.id)}.${format === "markdown" ? "md" : "json"}`);
    } catch (err) {
      const status = err?.response?.status;
      onNotify?.(
        status === 404 ? "Export is not available on this server yet" : status === 403 ? "Your role cannot export this session" : "Export failed",
        "error",
      );
    }
  };

  const copyLink = () =>
    copyToClipboard(
      window.location.href,
      "Link",
      () => onNotify?.("Link copied", "success"),
      () => onNotify?.("Could not copy the link", "error"),
    );

  // The page title never shows the session name or external id: either may
  // be client-supplied personal data.
  return (
    <Box data-testid="session-header" sx={{ mb: 2 }}>
      <Stack direction="row" alignItems="center" spacing={1} flexWrap="wrap" useFlexGap>
        <Typography variant="headingLarge" component="h2">
          {s.name || `Session ${shortId(s.id)}`}
        </Typography>
        <Chip size="small" label={s.status} color={STATUS_COLOURS[s.status] || "default"} />
        {s.source === "inferred" && (
          <Tooltip title="Grouped by the gateway from traffic patterns">
            <Chip size="small" variant="outlined" label="inferred" />
          </Tooltip>
        )}
        <Box sx={{ flexGrow: 1 }} />
        <Button
          variant="outlined"
          endIcon={<ArrowDropDownIcon />}
          onClick={(e) => setAnchor(e.currentTarget)}
          disabled={!viewer.can_export}
          aria-haspopup="menu"
          aria-controls={anchor ? "session-export-menu" : undefined}
          title={viewer.can_export ? undefined : "Your role cannot export sessions"}
        >
          Export
        </Button>
        <Menu id="session-export-menu" anchorEl={anchor} open={Boolean(anchor)} onClose={() => setAnchor(null)}>
          <MenuItem onClick={() => runExport("json", false)}>JSON (metadata)</MenuItem>
          {viewer.can_read_content && <MenuItem onClick={() => runExport("json", true)}>JSON with content</MenuItem>}
          <MenuItem onClick={() => runExport("markdown", Boolean(viewer.can_read_content))}>Markdown transcript</MenuItem>
        </Menu>
        <Button variant="outlined" startIcon={<LinkIcon />} onClick={copyLink}>
          Copy link
        </Button>
      </Stack>
      <Stack direction="row" spacing={4} sx={{ mt: 1.5 }} flexWrap="wrap" useFlexGap>
        <Fact label="App">{s.app_name || `App ${s.app_id}`}</Fact>
        <Fact label="User">
          <SessionUser session={s} masked={!viewer.can_read_content} />
        </Fact>
        <Fact label="Started">{new Date(s.started_at).toLocaleString()}</Fact>
        <Fact label="Duration">{formatDuration(s.duration_ms)}</Fact>
        <Fact label="Tokens">{formatTokens((s.totals?.input_tokens || 0) + (s.totals?.output_tokens || 0))}</Fact>
        <Fact label="Cost">{formatCost(s.totals?.cost_micros, s.totals?.currency)}</Fact>
        <Fact label="Errors">
          <Typography variant="body2" color={s.totals?.error_count ? "error" : "text.primary"}>
            {s.totals?.error_count ?? 0}
          </Typography>
        </Fact>
      </Stack>
    </Box>
  );
};

export const SummaryStrip = () => {
  const { detail } = useSessionDetail();
  const s = detail.session;
  const agents = detail.agents || [];
  return (
    <Stack
      direction="row"
      spacing={3}
      alignItems="center"
      flexWrap="wrap"
      useFlexGap
      data-testid="summary-strip"
      sx={{ py: 1, mb: 1, borderTop: 1, borderBottom: 1, borderColor: "divider" }}
    >
      <Stack direction="row" spacing={0.5} alignItems="center" flexWrap="wrap" useFlexGap>
        <Typography variant="caption" color="text.secondary">
          Agents
        </Typography>
        {agents.length ? (
          agents.map((a) => (
            <Chip key={a.id} size="small" variant={a.declared ? "filled" : "outlined"} label={a.declared ? a.name : `${a.name} (inferred)`} />
          ))
        ) : (
          <Typography variant="body2">—</Typography>
        )}
      </Stack>
      <Fact label="Models">{(s.models || []).join(", ") || "—"}</Fact>
      <Fact label="Tools">{(s.tool_names || []).join(", ") || "—"}</Fact>
      <Fact label="Gateway events">{s.gateway_event_count ?? 0}</Fact>
      <Fact label="Correlation">
        <CorrelationQualityBar counts={s.confidence_counts} />
      </Fact>
      <Tooltip title={CONTENT_MODE_LEGEND[s.content_mode] || ""}>
        <Box>
          <Fact label="Content mode">{s.content_mode}</Fact>
        </Box>
      </Tooltip>
    </Stack>
  );
};
