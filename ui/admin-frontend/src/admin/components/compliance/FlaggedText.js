import React from "react";
import { Box, Link, Typography } from "@mui/material";
import { Link as RouterLink } from "react-router-dom";

// What a guardrail compliance event says about the text it flagged
// (scripting/filter_runner.go): metadata.segments lists the flagged segments
// with the SHA-256 of the text the provider was sent (the key a provider such
// as Wolf Defender logs by) and, when the filter's "Show flagged text" is on,
// a masked excerpt; metadata.request_id is the gateway's X-Tyk-AI-Request-ID
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

const FlaggedText = ({ metadata, appId }) => {
  const segments = Array.isArray(metadata?.segments) ? metadata.segments.filter((s) => s && typeof s === "object") : [];
  const requestId = typeof metadata?.request_id === "string" ? metadata.request_id : "";
  const clientId = typeof metadata?.client_request_id === "string" ? metadata.client_request_id : "";
  if (segments.length === 0 && !requestId) return null;

  return (
    <Box sx={{ mb: 2 }} data-testid="flagged-text">
      <Typography variant="caption" color="text.secondary">
        Flagged text
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
      {segments.map((seg, i) => (
        <Box key={`${seg.index}-${seg.sha256 || i}`} sx={{ mt: 1 }} data-testid="flagged-segment">
          <Typography variant="body2" sx={{ fontWeight: 500 }}>
            {segmentLabel(seg)}
            {Number.isInteger(seg.index) ? ` (segment ${seg.index})` : ""}
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
            <Typography variant="caption" color="text.secondary" component="div">
              No excerpt: turn on "Show flagged text in compliance events" on the filter to record one.
            </Typography>
          )}
          {seg.sha256 && (
            <Typography variant="caption" color="text.secondary" component="div" sx={mono}>
              SHA-256 {seg.sha256}
            </Typography>
          )}
        </Box>
      ))}
    </Box>
  );
};

export default FlaggedText;
