import React from "react";
import { Box, Chip, IconButton, Link, Tooltip, Typography } from "@mui/material";
import ContentCopyIcon from "@mui/icons-material/ContentCopy";
import { Link as RouterLink } from "react-router-dom";
import { JUDGE_EVENT_TYPE, judgeOf, lookupOpinion, opinionColor, opinionLabel } from "./judgeOpinions";

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

// How a segment met the app's trusted prompt templates (seg.template,
// scripting/guardrail_templates.go templateRef): a matched template's ref
// (name@version) and the share of the text it exempted, whether the backstop
// still scored the template text, and the signals of a near miss or of
// template text found where the template does not put it.
const TEMPLATE_SIGNALS = {
  "template.mimicry": { label: "Template text out of place", color: "warning" },
  "template.miss": { label: "No template matched", color: "default" },
  "template.over_cap": { label: "Over the exempt cap", color: "warning" },
};

export const templateBadges = (tpl) => {
  if (!tpl || typeof tpl !== "object") return [];
  const out = [];
  if (typeof tpl.id === "string" && tpl.id !== "") {
    let label = `Matched template ${tpl.id}`;
    if (typeof tpl.exempt_share === "number") {
      label += `, ${Math.round(tpl.exempt_share * 100)}% exempt`;
    }
    out.push({ key: "match", label, color: "info" });
  }
  const signals = Array.isArray(tpl.signals) ? tpl.signals : [];
  for (const s of signals) {
    const known = TEMPLATE_SIGNALS[s];
    if (known) out.push({ key: s, ...known });
  }
  if (tpl.backstop === "enforced") {
    out.push({ key: "backstop", label: "Backstop scored the template text", color: "warning" });
  }
  return out;
};

const copy = (text) => {
  try {
    navigator.clipboard?.writeText(text);
  } catch (e) {
    // Clipboard access can be refused; the hash stays visible to copy by hand.
  }
};

// JudgeOpinion is a guardrail.judge event's own details.
const JudgeOpinion = ({ judge }) => (
  <Box sx={{ mt: 0.5 }} data-testid="judge-opinion">
    <Chip size="small" variant="outlined" color={opinionColor(judge)} label={opinionLabel(judge)} />
    <Typography variant="caption" color="text.secondary" component="div" sx={{ mt: 0.5 }}>
      {[
        judge.action ? `on a ${judge.action} verdict` : "",
        judge.coverage === "partial" ? "not shown everything that was flagged, so it could not soften" : "",
        judge.model ? `model ${judge.model}` : "",
        typeof judge.latency_ms === "number" ? `${judge.latency_ms} ms` : "",
        judge.cached ? "cached" : "",
      ]
        .filter(Boolean)
        .join(" · ")}
    </Typography>
    {judge.sha256 && (
      <Typography variant="caption" color="text.secondary" component="div" sx={mono}>
        Segment SHA-256 {judge.sha256}
      </Typography>
    )}
  </Box>
);

const FlaggedText = ({ metadata, appId, eventType, opinions }) => {
  const segments = Array.isArray(metadata?.segments) ? metadata.segments.filter((s) => s && typeof s === "object") : [];
  const requestId = typeof metadata?.request_id === "string" ? metadata.request_id : "";
  const clientId = typeof metadata?.client_request_id === "string" ? metadata.client_request_id : "";
  const ownJudge = judgeOf(metadata);
  const isJudgeEvent = eventType === JUDGE_EVENT_TYPE && !!ownJudge;
  if (segments.length === 0 && !requestId && !isJudgeEvent) return null;
  const notScanned = eventType === "guardrail.unscanned";
  const listed = segments.length;
  // count is the segments of an unscanned event, but the findings of a
  // detection (several can share a segment), so only the first says how many
  // segments there were.
  const total = notScanned && typeof metadata?.count === "number" ? Math.max(metadata.count, listed) : listed;

  return (
    <Box sx={{ mb: 2 }} data-testid="flagged-text">
      <Typography variant="caption" color="text.secondary">
        {isJudgeEvent ? "Second opinion" : notScanned ? "Not checked" : "Flagged text"}
      </Typography>
      {isJudgeEvent && <JudgeOpinion judge={ownJudge} />}
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
          {lookupOpinion(opinions, requestId, seg.sha256) && (
            <Box sx={{ mt: 0.5 }} data-testid="flagged-judge">
              <Chip
                size="small"
                variant="outlined"
                color={opinionColor(lookupOpinion(opinions, requestId, seg.sha256))}
                label={opinionLabel(lookupOpinion(opinions, requestId, seg.sha256))}
              />
            </Box>
          )}
          {seg.exception && typeof seg.exception === "object" && Number.isInteger(seg.exception.id) && (
            <Box sx={{ mt: 0.5 }} data-testid="flagged-exception">
              <Chip
                size="small"
                variant="outlined"
                color="info"
                label={`Softened by exception #${seg.exception.id}${
                  seg.exception.from && seg.exception.to ? ` (${seg.exception.from} to ${seg.exception.to})` : ""
                }`}
              />
            </Box>
          )}
          {templateBadges(seg.template).length > 0 && (
            <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.5, mt: 0.5 }} data-testid="flagged-template">
              {templateBadges(seg.template).map((b) => (
                <Chip key={b.key} size="small" variant="outlined" color={b.color} label={b.label} />
              ))}
            </Box>
          )}
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
