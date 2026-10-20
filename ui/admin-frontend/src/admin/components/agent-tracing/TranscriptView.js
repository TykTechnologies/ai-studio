import React, { useEffect, useMemo, useRef, useState } from "react";
import { observeElementRect, useVirtualizer } from "@tanstack/react-virtual";
import { Alert, Box, Button, Chip, Collapse, Divider, Link, Stack, Tooltip, Typography } from "@mui/material";
import CheckCircleIcon from "@mui/icons-material/CheckCircle";
import ErrorIcon from "@mui/icons-material/Error";
import HourglassEmptyIcon from "@mui/icons-material/HourglassEmpty";
import { useSessionDetail } from "./SessionDetailContext";
import {
  agentRollup,
  argPreview,
  buildSubAgentRows,
  buildTranscript,
  confidenceStyle,
  gatewayChip,
  partOf,
  partText,
  resultPreview,
  rowIndexOf,
  stepLatency,
} from "./transcriptModel";
import { attrsOf, ms, partPlaceholder, resourceName, spanTimes } from "./spanModel";
import { formatCost, formatDuration, formatTokens } from "./sessionFilters";
import { RedactedText } from "./SpanDrawer";
import MarkdownMessage from "../../../portal/components/chat/MarkdownMessage";

// The Transcript ("Trajectory") view, the default (spec §6.2 View 1): the
// session read like a conversation, turn by turn. Rows are virtualised with
// variable heights, so the DOM stays bounded however many spans there are.

const USER_LINES = 6;
const ROW_ESTIMATE = 96;

const Placeholder = ({ span, part }) => {
  const p = partOf(span, part);
  if (!p) return null;
  return (
    <Typography variant="caption" color="text.secondary" display="block" data-testid="content-placeholder">
      {partPlaceholder(p).text}
    </Typography>
  );
};

const SelectableBox = ({ span, children, sx, ...rest }) => {
  const { selectedId, dispatch } = useSessionDetail();
  const selected = span && span.span_id === selectedId;
  return (
    <Box
      data-span-id={span?.span_id}
      aria-current={selected ? "true" : undefined}
      onClick={(e) => {
        e.stopPropagation();
        if (span) dispatch({ type: "open", id: span.span_id });
      }}
      sx={{
        cursor: span ? "pointer" : "default",
        borderRadius: 1,
        outline: selected ? 2 : 0,
        outlineStyle: "solid",
        outlineColor: "primary.main",
        ...sx,
      }}
      {...rest}
    >
      {children}
    </Box>
  );
};

const EventChip = ({ span }) => {
  const { text, severity } = gatewayChip(span);
  return (
    <SelectableBox span={span} component="span" sx={{ display: "inline-block", mr: 0.5, mb: 0.5 }}>
      <Chip size="small" label={text} color={severity} variant={severity === "default" ? "outlined" : "filled"} data-testid="gateway-chip" />
    </SelectableBox>
  );
};

const TurnDivider = ({ row }) => (
  <Stack direction="row" alignItems="center" spacing={1} sx={{ pt: 2, pb: 1 }} data-testid="turn-divider">
    <Divider sx={{ flexGrow: 1 }} />
    <Typography variant="caption" color="text.secondary">
      Turn {row.index} · {formatDuration(row.durationMs)} · {formatCost(row.costMicros)}
    </Typography>
    <Divider sx={{ flexGrow: 1 }} />
  </Stack>
);

const UserMessage = ({ span }) => {
  const [open, setOpen] = useState(false);
  const text = partText(span, "user_message");
  const lines = text ? text.split("\n").length : 0;
  const long = text && (lines > USER_LINES || text.length > USER_LINES * 120);
  return (
    <SelectableBox span={span} sx={{ p: 1.5, mb: 1, bgcolor: "action.hover" }} data-testid="user-message">
      <Typography variant="caption" color="text.secondary">
        User
      </Typography>
      {text ? (
        <>
          <Box
            sx={
              long && !open
                ? { display: "-webkit-box", WebkitLineClamp: USER_LINES, WebkitBoxOrient: "vertical", overflow: "hidden" }
                : undefined
            }
          >
            <Typography variant="body2" component="div">
              <RedactedText text={text} />
            </Typography>
          </Box>
          {long && (
            <Link component="button" variant="caption" onClick={(e) => {
                e.stopPropagation();
                setOpen((o) => !o);
              }}>
              {open ? "Show less" : "Show more"}
            </Link>
          )}
        </>
      ) : (
        <Placeholder span={span} part={partOf(span, "user_message") ? "user_message" : "request"} />
      )}
    </SelectableBox>
  );
};

const StatusIcon = ({ span }) => {
  if (span.status === "error") return <ErrorIcon fontSize="small" color="error" titleAccess="Error" />;
  if (!span.end_time || span.status === "in_progress") return <HourglassEmptyIcon fontSize="small" color="disabled" titleAccess="No result observed" />;
  return <CheckCircleIcon fontSize="small" color="success" titleAccess="OK" />;
};

const NextConnector = ({ tool, next }) => {
  const { dispatch } = useSessionDetail();
  if (!next) return null;
  const gap = ms(next.start_time) - spanTimes(tool).end;
  const model = next.response_model || next.request_model || next.name;
  return (
    <Typography variant="caption" color="text.secondary" display="block" sx={{ pl: 2, py: 0.5 }} data-testid="next-connector">
      →{" "}
      <Link
        component="button"
        variant="caption"
        onClick={(e) => {
          e.stopPropagation();
          dispatch({ type: "open", id: next.span_id });
        }}
      >
        next: {next.kind === "llm_inference" ? "LLM call" : next.name} <code>{model}</code>
      </Link>{" "}
      ({gap >= 0 ? `${formatDuration(gap)} later` : "overlapping"}) used this result
    </Typography>
  );
};

const ToolCard = ({ item }) => {
  const { resources } = useSessionDetail();
  const { span, events, next } = item;
  const a = attrsOf(span);
  const style = confidenceStyle(span.parent_confidence);
  const args = partText(span, "tool_args");
  const result = partText(span, "tool_result");
  const resultPart = partOf(span, "tool_result");
  const clientSide = span.kind === "inferred_client_tool";
  const noResult = !span.end_time || (clientSide && !resultPart);
  return (
    <Box sx={{ mb: 0.5 }}>
      <SelectableBox
        span={span}
        data-testid="tool-card"
        sx={{
          p: 1,
          borderLeft: 3,
          borderLeftStyle: style.border,
          borderLeftColor: span.status === "error" ? "error.main" : "primary.light",
          bgcolor: "background.paper",
          border: 1,
          borderColor: "divider",
        }}
      >
        <Stack direction="row" spacing={1} alignItems="center" flexWrap="wrap" useFlexGap>
          <StatusIcon span={span} />
          <Typography variant="body2" sx={{ fontFamily: "monospace" }}>
            {span.tool_name || span.name}({args != null ? argPreview(args) : ""})
          </Typography>
          {span.end_time && (
            <Typography variant="caption" color="text.secondary">
              {formatDuration(stepLatency(span))}
            </Typography>
          )}
          {a["tyk.mcp.server"] && <Chip size="small" variant="outlined" label={`MCP · ${a["tyk.mcp.server"]}`} />}
          {span.kind === "datasource_call" && (
            <Chip size="small" variant="outlined" label={`datasource · ${resourceName(resources, span) || span.name}`} />
          )}
          {clientSide && <Chip size="small" variant="outlined" label="client-side (inferred)" data-testid="client-side-badge" />}
          {noResult && <Chip size="small" variant="outlined" label="no result observed" />}
          {style.badge && (
            <Tooltip title={`Linked by rule ${span.parent_rule || "?"} (${span.parent_confidence} confidence)`}>
              <Chip size="small" color="warning" variant="outlined" label={style.badge} data-testid="inferred-badge" />
            </Tooltip>
          )}
        </Stack>
        {result != null ? (
          <Typography variant="caption" color="text.secondary" display="block" sx={{ mt: 0.5 }} data-testid="result-preview">
            {resultPreview(result, resultPart?.original_bytes)}
          </Typography>
        ) : (
          <Placeholder span={span} part="tool_result" />
        )}
        {events.length > 0 && <Box sx={{ mt: 0.5 }}>{events.map((e) => <EventChip key={e.span_id} span={e} />)}</Box>}
      </SelectableBox>
      <NextConnector tool={span} next={next} />
    </Box>
  );
};

const ToolGroup = ({ group }) =>
  group.length > 1 ? (
    <Box sx={{ pl: 1, borderLeft: 2, borderColor: "divider", mb: 1 }} data-testid="parallel-group">
      <Typography variant="caption" color="text.secondary">
        {group.length} parallel calls
      </Typography>
      {group.map((item) => (
        <ToolCard key={item.span.span_id} item={item} />
      ))}
    </Box>
  ) : (
    <ToolCard item={group[0]} />
  );

const AssistantStep = ({ row }) => {
  const s = row.span;
  const text = partText(s, "response");
  const reasoning = partText(s, "reasoning");
  const [showReasoning, setShowReasoning] = useState(false);
  const u = s.usage || {};
  const toolsAvailable = attrsOf(s)["tyk.tools.available"];
  return (
    <Box sx={{ mb: 1 }} data-testid="assistant-step">
      <SelectableBox span={s} sx={{ p: 1.5 }}>
        <Typography variant="caption" color="text.secondary">
          Assistant
        </Typography>
        {reasoning != null && (
          <Box>
            <Link component="button" variant="caption" onClick={(e) => {
                e.stopPropagation();
                setShowReasoning((o) => !o);
              }}>
              {showReasoning ? "Hide reasoning" : "Show reasoning"}
            </Link>
            <Collapse in={showReasoning}>
              <Typography variant="body2" color="text.secondary" sx={{ fontStyle: "italic" }}>
                <RedactedText text={reasoning} />
              </Typography>
            </Collapse>
          </Box>
        )}
        {text != null ? <MarkdownMessage content={text} /> : <Placeholder span={s} part="response" />}
        <Stack direction="row" spacing={2} flexWrap="wrap" useFlexGap sx={{ mt: 0.5 }} data-testid="step-meta">
          <Typography variant="caption" color="text.secondary">
            {s.response_model || s.request_model || "model ?"}
          </Typography>
          <Typography variant="caption" color="text.secondary">
            {formatTokens(u.input_tokens)} in · {formatTokens(u.output_tokens)} out
          </Typography>
          {u.cost_micros ? (
            <Typography variant="caption" color="text.secondary">
              {formatCost(u.cost_micros)}
            </Typography>
          ) : null}
          {stepLatency(s) != null && (
            <Typography variant="caption" color="text.secondary">
              {formatDuration(stepLatency(s))}
            </Typography>
          )}
          {s.ttft_ms != null && (
            <Typography variant="caption" color="text.secondary">
              TTFT {s.ttft_ms} ms
            </Typography>
          )}
          {(s.finish_reasons || []).length > 0 && (
            <Typography variant="caption" color="text.secondary">
              {s.finish_reasons.join(", ")}
            </Typography>
          )}
          {s.status === "error" && (
            <Typography variant="caption" color="error">
              {s.error_type || "error"}
            </Typography>
          )}
          {toolsAvailable != null && (
            <Typography variant="caption" color="text.secondary">
              tools available: {toolsAvailable}
            </Typography>
          )}
        </Stack>
        {row.events.length > 0 && <Box sx={{ mt: 0.5 }}>{row.events.map((e) => <EventChip key={e.span_id} span={e} />)}</Box>}
      </SelectableBox>
      <Box sx={{ pl: 2 }}>
        {row.toolGroups.map((g) => (
          <ToolGroup key={g[0].span.span_id} group={g} />
        ))}
      </Box>
    </Box>
  );
};

const SubAgentCard = ({ row, depth }) => {
  const { detail, spans, expandAll } = useSessionDetail();
  const [open, setOpen] = useState(false);
  useEffect(() => setOpen(expandAll), [expandAll]);
  const r = agentRollup(row.agent, row.spans);
  const rows = useMemo(() => (open ? buildSubAgentRows(spans, detail, row.agent.id) : []), [open, spans, detail, row.agent.id]);
  return (
    <Box sx={{ mb: 1, ml: depth ? 2 : 0, border: 1, borderColor: "divider", borderRadius: 1 }} data-testid="subagent-card">
      <Button
        fullWidth
        onClick={() => setOpen((o) => !o)}
        aria-expanded={open}
        sx={{ justifyContent: "flex-start", textTransform: "none", p: 1 }}
      >
        <Stack sx={{ textAlign: "left" }}>
          <Typography variant="subtitle2">
            {open ? "▾" : "▸"} Sub-agent: {row.agent.name}
            {!row.agent.declared && " (inferred)"}
          </Typography>
          <Typography variant="caption" color="text.secondary">
            {r.llmCalls} LLM calls · {r.toolCalls} tool calls · {formatTokens(r.tokens)} tokens · {formatCost(r.costMicros)}
            {r.errors ? ` · ${r.errors} errors` : ""} · {formatDuration(r.durationMs)}
          </Typography>
        </Stack>
      </Button>
      <Collapse in={open} unmountOnExit>
        <Box sx={{ p: 1, pt: 0 }}>
          {rows.map((child) => (
            <RowContent key={child.key} row={child} depth={depth + 1} />
          ))}
        </Box>
      </Collapse>
    </Box>
  );
};

export const RowContent = ({ row, depth = 0 }) => {
  switch (row.type) {
    case "turn":
      return <TurnDivider row={row} />;
    case "user":
      return <UserMessage span={row.span} />;
    case "step":
      return <AssistantStep row={row} />;
    case "tool":
      return <ToolCard item={{ span: row.span, events: row.events || [], next: row.next }} />;
    case "event":
      return (
        <Box sx={{ mb: 1 }}>
          <EventChip span={row.span} />
        </Box>
      );
    case "subagent":
      return <SubAgentCard row={row} depth={depth} />;
    default:
      return null;
  }
};

const PreviewsNotice = () => {
  const { viewer, previews } = useSessionDetail();
  if (!viewer.can_read_content) {
    return (
      <Alert severity="info" sx={{ mb: 1 }}>
        Your role shows the structure of this session but not its content.
      </Alert>
    );
  }
  if (previews === "audit_unavailable") {
    return (
      <Alert severity="warning" sx={{ mb: 1 }}>
        Content previews are unavailable because the audit log could not be written. Open a step to load its content.
      </Alert>
    );
  }
  if (previews && previews !== "included") {
    return (
      <Alert severity="info" sx={{ mb: 1 }}>
        Content previews are not available. Open a step to load its content.
      </Alert>
    );
  }
  return null;
};

const TranscriptView = ({ height = 640 }) => {
  const { index, detail, selectedId } = useSessionDetail();
  const parent = useRef(null);
  const rows = useMemo(() => buildTranscript(index.ordered, detail), [index.ordered, detail]);
  const rowOf = useMemo(() => rowIndexOf(rows), [rows]);
  const virtualizer = useVirtualizer({
    count: rows.length,
    getScrollElement: () => parent.current,
    estimateSize: () => ROW_ESTIMATE,
    // A row that measures 0 (not laid out yet) keeps its estimate, so the
    // window never grows to every row.
    measureElement: (el) => el.getBoundingClientRect().height || ROW_ESTIMATE,
    overscan: 6,
    initialRect: { width: 960, height },
    // A container that reports no size yet (not laid out, or a test DOM)
    // still renders a viewport's worth of rows.
    observeElementRect: (instance, cb) =>
      observeElementRect(instance, (rect) => cb(rect.height ? rect : { width: rect.width || 960, height })),
  });

  useEffect(() => {
    const i = selectedId ? rowOf.get(selectedId) : undefined;
    if (i !== undefined) virtualizer.scrollToIndex(i, { align: "auto" });
  }, [selectedId, rowOf, virtualizer]);

  if (!rows.length) {
    return (
      <Typography variant="body2" color="text.secondary" sx={{ p: 2 }}>
        No spans in this range.
      </Typography>
    );
  }
  return (
    <>
      <PreviewsNotice />
      <Box
        ref={parent}
        role="feed"
        aria-label="Transcript"
        aria-busy="false"
        tabIndex={0}
        data-span-view="true"
        data-testid="transcript"
        sx={{ height, overflow: "auto", "&:focus-visible": { outline: 2, outlineStyle: "solid", outlineColor: "primary.main" } }}
      >
        <Box sx={{ height: virtualizer.getTotalSize(), position: "relative", width: "100%" }}>
          {virtualizer.getVirtualItems().map((v) => (
            <Box
              key={rows[v.index].key}
              data-index={v.index}
              ref={virtualizer.measureElement}
              role="article"
              data-testid="transcript-row"
              sx={{ position: "absolute", top: 0, left: 0, width: "100%", transform: `translateY(${v.start}px)`, px: 1 }}
            >
              <RowContent row={rows[v.index]} />
            </Box>
          ))}
        </Box>
      </Box>
    </>
  );
};

export default TranscriptView;
