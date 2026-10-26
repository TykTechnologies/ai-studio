import React from "react";
import { Box, IconButton, Link, Tooltip, Typography } from "@mui/material";
import ContentCopyIcon from "@mui/icons-material/ContentCopy";
import { Link as RouterLink } from "react-router-dom";

// What a guardrail compliance event says about the text it flagged
// (scripting/filter_runner.go): metadata.segments lists the flagged segments
// with the SHA-256 of the text the provider was sent (the key a provider such
// as Wolf Defender logs by) and, when the filter's "Show flagged text" is on,
// a masked excerpt, and excerpt_status, why there is one or not;
// metadata.request_id is the gateway's X-Tyk-AI-Request-ID
// (the request log's request ID), client_request_id the caller's X-Request-ID.
const ROLE_LABELS = {
  user: "User message",
  system: "System prompt",
  assistant: "Assistant message",
  tool: "Tool result",
};

export const segmentLabel = (seg) => {
  if (seg.kind === "tool_call") return seg.tool ? `Tool call to ${seg.tool}` : "Tool call";
  if (seg.kind === "tool_definition") return seg.tool ? `Tool definition ${seg.tool}` : "Tool definition";
  if (seg.kind === "tool_result" || seg.role === "tool") {
    return seg.tool ? `Tool result from ${seg.tool}` : "Tool result";
  }
  return ROLE_LABELS[seg.role] || (seg.role ? `${seg.role} message` : "Message");
};

const mono = { fontFamily: "monospace", fontSize: "0.75rem", wordBreak: "break-all" };

// Why a segment has no excerpt, from its excerpt_status. Events from before
// excerpt_status was recorded fall back on the event type.
const NO_EXCERPT = {
  not_scanned: "Not checked, so there is no match to show.",
  off: 'Excerpts are off: turn on "Show flagged text in compliance events" on the filter.',
  bodies_not_logged: "Not recorded: this LLM does not log request bodies.",
  unavailable: "No excerpt available for this text.",
};

export const noExcerpt = (status, notScanned) =>
  NO_EXCERPT[status] ||
  (notScanned
    ? NO_EXCERPT.not_scanned
    : 'No excerpt (if "Show flagged text in compliance events" is off on the filter, turn it on).');

const copy = (text) => {
  try {
    navigator.clipboard?.writeText(text);
  } catch (e) {
    // Clipboard access can be refused; the hash stays visible to copy by hand.
  }
};

const FlaggedText = ({ metadata, appId, eventType }) => {
  const segments = Array.isArray(metadata?.segments) ? metadata.segments.filter((s) => s && typeof s === "object") : [];
  const requestId = typeof metadata?.request_id === "string" ? metadata.request_id : "";
  const clientId = typeof metadata?.client_request_id === "string" ? metadata.client_request_id : "";
  if (segments.length === 0 && !requestId) return null;
  const notScanned = eventType === "guardrail.unscanned";
  const listed = segments.length;
  // count is the segments of an unscanned event, but the findings of a
  // detection (several can share a segment), so only the first says how many
  // segments there were.
  const total = notScanned && typeof metadata?.count === "number" ? Math.max(metadata.count, listed) : listed;

  return (
    <Box sx={{ mb: 2 }} data-testid="flagged-text">
      <Typography variant="caption" color="text.secondary">
        {notScanned ? "Not checked" : "Flagged text"}
      </Typography>
      {requestId && (
        <Typography variant="body2" sx={{ mt: 0.5 }}>
          Request ID: <Box component="span" sx={mono}>{requestId}</Box>
          {appId ? (
            <>
              {" "}
              <Link
                component={RouterLink}
                to={`/admin/apps/${appId}?log_search=${encodeURIComponent(requestId)}`}
                data-testid="flagged-request-log"
              >
                Open the request log
              </Link>
            </>
          ) : null}
        </Typography>
      )}
      {clientId && (
        <Typography variant="body2">
          Client request ID: <Box component="span" sx={mono}>{clientId}</Box>
        </Typography>
      )}
      {listed < total && (
        <Typography variant="caption" color="text.secondary" component="div" sx={{ mt: 0.5 }} data-testid="flagged-shown">
          {listed} of {total} shown
        </Typography>
      )}
      {segments.map((seg, i) => (
        <Box key={`${seg.index}-${seg.sha256 || i}`} sx={{ mt: 1 }} data-testid="flagged-segment">
          <Typography variant="body2" component="div">
            <Box component="span" sx={{ fontWeight: 500 }}>
              {segmentLabel(seg)}
            </Box>
            {typeof seg.chars === "number" && (
              <Box component="span" sx={{ color: "text.secondary" }}>{` · ${seg.chars.toLocaleString("en-US")} chars`}</Box>
            )}
            {Number.isInteger(seg.index) && (
              <Box component="span" sx={{ color: "text.secondary", fontSize: "0.75rem" }}>{` · #${seg.index}`}</Box>
            )}
          </Typography>
          {typeof seg.excerpt === "string" && seg.excerpt !== "" ? (
            <Box
              component="blockquote"
              data-testid="flagged-excerpt"
              sx={{
                m: 0,
                mt: 0.5,
                pl: 1.5,
                borderLeft: 3,
                borderColor: "warning.main",
                whiteSpace: "pre-wrap",
                wordBreak: "break-word",
                fontSize: "0.85rem",
              }}
            >
              {seg.excerpt}
            </Box>
          ) : (
            <Typography variant="caption" color="text.secondary" component="div" data-testid="flagged-no-excerpt">
              {noExcerpt(seg.excerpt_status, notScanned)}
            </Typography>
          )}
          {seg.sha256 && (
            <Typography variant="caption" color="text.secondary" component="div" sx={mono}>
              SHA-256 {seg.sha256}
              <Tooltip title="Copy SHA-256">
                <IconButton size="small" aria-label="Copy SHA-256" onClick={() => copy(seg.sha256)} sx={{ ml: 0.5, p: 0.25 }}>
                  <ContentCopyIcon fontSize="inherit" />
                </IconButton>
              </Tooltip>
            </Typography>
          )}
        </Box>
      ))}
    </Box>
  );
};

export default FlaggedText;
