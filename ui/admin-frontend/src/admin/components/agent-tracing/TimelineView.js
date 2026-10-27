import React, { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { VariableSizeList } from "react-window";
import { Box, Button, Stack, Tooltip, Typography } from "@mui/material";
import { useTheme } from "@mui/material/styles";
import { useSessionDetail } from "./SessionDetailContext";
import { KIND_LABELS, durationMs, isProblem, sessionBounds, spanTimes } from "./spanModel";
import { buildTimelineRows, findGaps, panView, turnRules, viewForSpan, zoomView } from "./timelineModel";
import { formatDuration, formatTokens } from "./sessionFilters";

// The Timeline (waterfall) view (spec §6.2 View 2): agent lanes, spans as
// bars coloured by kind (errors red, inferred spans hatched), gateway events
// as ticks, idle "client time" gaps between causally linked spans, and turn
// rules. One labelled row per span. Ctrl/Cmd-scroll (or a pinch) zooms,
// Shift-scroll, a sideways swipe or the scrollbar pans, a drag selects the
// shared time range, a double-click zooms to a span, Fit shows the whole
// session. Rows are virtualised and scroll vertically.

const LABEL_WIDTH = 180;
const ROW = 22;
const LANE = 26;
const MIN_DRAG_PX = 4;
// Browsers cap element widths; past this the scrollbar just gets coarser.
const MAX_SCROLL_WIDTH = 10_000_000;

const SHORT_KIND = {
  llm_inference: "LLM",
  tool_call: "Tool",
  datasource_call: "Datasource",
  inferred_client_tool: "Client tool",
  agent: "Agent",
  client_span: "Client",
};

/** A row's label: the kind, then the model or tool rather than the raw span name. */
export const spanLabel = (s) => {
  const what = s.kind === "llm_inference" ? s.response_model || s.request_model || s.name : s.tool_name || s.name;
  return `${SHORT_KIND[s.kind] || s.kind} · ${what}`;
};

const spanTooltip = (s) => {
  const parts = [`${KIND_LABELS[s.kind] || s.kind}: ${s.name}`, s.end_time ? formatDuration(durationMs(s)) : "in progress"];
  const u = s.usage || {};
  if (u.input_tokens || u.output_tokens) parts.push(`${formatTokens(u.input_tokens)} in · ${formatTokens(u.output_tokens)} out`);
  if (s.status === "error") parts.push(s.error_type || "error");
  return parts.join(" · ");
};

const TimelineView = ({ height = 520 }) => {
  const theme = useTheme();
  const { index, spans: allSpans, detail, selectedId, dispatch } = useSessionDetail();
  const spans = index.ordered;
  const bounds = useMemo(() => sessionBounds(detail.session, allSpans), [detail.session, allSpans]);
  const [view, setView] = useState(bounds);
  const [drag, setDrag] = useState(null);
  const [trackWidth, setTrackWidth] = useState(780);
  const box = useRef(null);
  const list = useRef(null);
  const scroller = useRef(null);

  useEffect(() => setView(bounds), [bounds]);
  useEffect(() => {
    const el = box.current;
    if (!el || typeof ResizeObserver === "undefined") return undefined;
    const ro = new ResizeObserver(([e]) => {
      const w = e.contentRect.width - LABEL_WIDTH;
      if (w > 0) setTrackWidth(w);
    });
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  const rows = useMemo(() => buildTimelineRows(spans, detail.agents), [spans, detail.agents]);
  const gaps = useMemo(() => findGaps(spans), [spans]);
  const rules = useMemo(() => turnRules(spans, detail.traces), [spans, detail.traces]);
  const fit = view.from <= bounds.from && view.to >= bounds.to;

  useEffect(() => {
    list.current?.resetAfterIndex(0);
  }, [rows]);

  const span = view.to - view.from || 1;
  const total = bounds.to - bounds.from || 1;
  // The horizontal scrollbar stands for the whole session at this zoom.
  const scrollWidth = Math.min(MAX_SCROLL_WIDTH, (trackWidth * total) / span);
  useEffect(() => {
    const el = scroller.current;
    if (!el) return;
    const want = ((view.from - bounds.from) / total) * scrollWidth;
    if (Math.abs(el.scrollLeft - want) > 1) el.scrollLeft = want;
  }, [view.from, bounds.from, total, scrollWidth]);
  const onScroll = (e) => {
    const from = bounds.from + (e.currentTarget.scrollLeft / scrollWidth) * total;
    setView((v) => (Math.abs(v.from - from) < 1 ? v : panView(v, from - v.from, bounds)));
  };
  const x = useCallback((t) => ((t - view.from) / span) * trackWidth, [view.from, span, trackWidth]);
  const timeAt = (clientX) => {
    const rect = box.current?.getBoundingClientRect();
    if (!rect || !rect.width) return null;
    return view.from + ((clientX - rect.left - LABEL_WIDTH) / trackWidth) * span;
  };

  const kindColour = {
    llm_inference: theme.palette.primary.main,
    tool_call: theme.palette.secondary.main,
    datasource_call: theme.palette.info.main,
    inferred_client_tool: theme.palette.warning.main,
    agent: theme.palette.grey[500],
    client_span: theme.palette.grey[600],
  };

  const onWheel = (e) => {
    if (e.ctrlKey || e.metaKey) {
      e.preventDefault();
      const anchor = timeAt(e.clientX) ?? (view.from + view.to) / 2;
      setView((v) => zoomView(v, e.deltaY > 0 ? 1.25 : 0.8, anchor, bounds));
      return;
    }
    // Shift-scroll or a sideways swipe pans; plain scrolling scrolls the rows.
    const dx = e.shiftKey ? e.deltaY || e.deltaX : Math.abs(e.deltaX) > Math.abs(e.deltaY) ? e.deltaX : 0;
    if (!dx || fit) return;
    setView((v) => panView(v, (dx / trackWidth) * (v.to - v.from), bounds));
  };
  const onMouseDown = (e) => {
    if (e.button !== 0 || e.clientX - (box.current?.getBoundingClientRect().left || 0) < LABEL_WIDTH) return;
    setDrag({ x0: e.clientX, x1: e.clientX });
  };
  const onMouseMove = (e) => drag && setDrag({ ...drag, x1: e.clientX });
  const onMouseUp = (e) => {
    if (!drag) return;
    const d = { ...drag, x1: e.clientX };
    setDrag(null);
    if (Math.abs(d.x1 - d.x0) < MIN_DRAG_PX) return;
    const a = timeAt(Math.min(d.x0, d.x1));
    const b = timeAt(Math.max(d.x0, d.x1));
    if (a != null && b != null) dispatch({ type: "range", range: { from: a, to: b } });
  };

  const Bar = ({ s, ticks }) => {
    const { start, end } = spanTimes(s);
    const left = x(start);
    const width = Math.max(2, x(Math.max(end, start + 1)) - left);
    if (left + width < 0 || left > trackWidth) return null;
    const colour = isProblem(s) ? theme.palette.error.main : kindColour[s.kind] || theme.palette.grey[500];
    const selected = s.span_id === selectedId;
    return (
      <Tooltip title={spanTooltip(s)} disableInteractive>
        <Box
          role="button"
          tabIndex={-1}
          aria-label={`${KIND_LABELS[s.kind] || s.kind}: ${s.name}`}
          aria-pressed={selected}
          data-testid="timeline-bar"
          data-span-id={s.span_id}
          onMouseDown={(e) => e.stopPropagation()}
          onClick={() => dispatch({ type: "open", id: s.span_id })}
          onDoubleClick={() => setView(viewForSpan(s, bounds))}
          sx={{
            position: "absolute",
            top: 4,
            bottom: 4,
            left,
            width,
            bgcolor: colour,
            backgroundImage: s.inferred
              ? "repeating-linear-gradient(45deg, rgba(255,255,255,0.45) 0 3px, transparent 3px 6px)"
              : "none",
            borderRadius: 0.5,
            outline: selected ? 2 : 0,
            outlineStyle: "solid",
            outlineColor: "text.primary",
            cursor: "pointer",
            overflow: "hidden",
          }}
        >
          {width > 48 && s.end_time && (
            <Typography variant="caption" sx={{ color: theme.palette.getContrastText(colour), px: 0.5, whiteSpace: "nowrap", lineHeight: "14px", display: "block" }}>
              {formatDuration(durationMs(s))}
            </Typography>
          )}
          {(ticks.get(s.span_id) || []).map((t) => (
            <Box
              key={t.span_id}
              data-testid="gateway-tick"
              title={t.name}
              sx={{
                position: "absolute",
                top: 0,
                bottom: 0,
                width: 2,
                left: Math.max(0, x(spanTimes(t).start) - left),
                bgcolor: isProblem(t) ? "error.dark" : "text.primary",
              }}
            />
          ))}
        </Box>
      </Tooltip>
    );
  };

  const Row = ({ index: i, style }) => {
    const r = rows[i];
    if (r.type === "lane") {
      return (
        <Box style={style} data-testid="timeline-lane" sx={{ display: "flex", alignItems: "center", pl: 1 + r.lane.depth * 2, borderTop: 1, borderColor: "divider" }}>
          <Typography variant="caption" sx={{ fontWeight: 600 }} noWrap>
            {r.lane.depth ? "↳ " : ""}
            {r.lane.name}
            {r.lane.inferred ? " (inferred)" : ""}
          </Typography>
        </Box>
      );
    }
    return (
      <Box style={style} role="group" aria-label={`${r.lane.name} spans`} sx={{ display: "flex", "&:hover": { bgcolor: "action.hover" } }}>
        <Box
          data-testid="timeline-label"
          onClick={() => !r.events && dispatch({ type: "open", id: r.spans[0].span_id })}
          sx={{
            width: LABEL_WIDTH,
            flexShrink: 0,
            pl: 1 + r.lane.depth * 2,
            pr: 1,
            display: "flex",
            alignItems: "center",
            gap: 0.75,
            cursor: r.events ? "default" : "pointer",
            borderRight: 1,
            borderColor: "divider",
            minWidth: 0,
          }}
        >
          {!r.events && (
            <Box sx={{ width: 8, height: 8, borderRadius: "50%", flexShrink: 0, bgcolor: isProblem(r.spans[0]) ? "error.main" : kindColour[r.spans[0].kind] || "grey.500" }} />
          )}
          <Typography variant="caption" noWrap title={r.events ? "Gateway events" : spanLabel(r.spans[0])}>
            {r.events ? "Gateway events" : spanLabel(r.spans[0])}
          </Typography>
        </Box>
        <Box sx={{ position: "relative", width: trackWidth, overflow: "hidden" }}>
          {r.spans.map((s) =>
            s.kind === "gateway_event" ? (
              <Box key={s.span_id} data-testid="gateway-tick" title={s.name} sx={{ position: "absolute", top: 0, bottom: 0, width: 2, left: x(spanTimes(s).start), bgcolor: isProblem(s) ? "error.main" : "text.secondary" }} />
            ) : (
              <Bar key={s.span_id} s={s} ticks={r.ticks} />
            ),
          )}
        </Box>
      </Box>
    );
  };

  const ticksAxis = useMemo(() => {
    const n = 6;
    return Array.from({ length: n + 1 }, (_, i) => view.from + (span * i) / n);
  }, [view.from, span]);
  const bodyHeight = Math.min(height, rows.reduce((h, r) => h + (r.type === "lane" ? LANE : ROW), 0) || LANE);
  const kindsShown = [...new Set(spans.map((s) => s.kind))].filter((k) => kindColour[k]);

  return (
    <Box data-testid="timeline-view" data-view-from={Math.round(view.from)} data-view-to={Math.round(view.to)}>
      <Stack direction="row" spacing={1} alignItems="center" sx={{ mb: 1 }}>
        <Button size="small" variant="outlined" onClick={() => setView(bounds)} disabled={fit}>
          Fit
        </Button>
        <Button size="small" onClick={() => setView((v) => zoomView(v, 0.5, (v.from + v.to) / 2, bounds))} aria-label="Zoom in">
          +
        </Button>
        <Button size="small" onClick={() => setView((v) => zoomView(v, 2, (v.from + v.to) / 2, bounds))} aria-label="Zoom out" disabled={fit}>
          −
        </Button>
        <Typography variant="caption" color="text.secondary">
          Showing {formatDuration(Math.round(span))} of {formatDuration(Math.round(total))} · Ctrl/⌘-scroll to zoom · Shift-scroll or the
          scrollbar to pan · drag to select a range · double-click a span to zoom to it
        </Typography>
      </Stack>
      <Stack direction="row" spacing={2} flexWrap="wrap" useFlexGap sx={{ mb: 1 }} data-testid="timeline-legend">
        {kindsShown.map((k) => (
          <Stack key={k} direction="row" spacing={0.5} alignItems="center">
            <Box sx={{ width: 12, height: 8, borderRadius: 0.5, bgcolor: kindColour[k] }} />
            <Typography variant="caption">{KIND_LABELS[k] || k}</Typography>
          </Stack>
        ))}
        <Stack direction="row" spacing={0.5} alignItems="center">
          <Box sx={{ width: 12, height: 8, borderRadius: 0.5, bgcolor: "error.main" }} />
          <Typography variant="caption">Error or denied</Typography>
        </Stack>
        <Stack direction="row" spacing={0.5} alignItems="center">
          <Box sx={{ width: 2, height: 12, bgcolor: "text.primary" }} />
          <Typography variant="caption">Gateway event</Typography>
        </Stack>
        {gaps.length > 0 && (
          <Stack direction="row" spacing={0.5} alignItems="center">
            <Box sx={{ width: 12, height: 8, bgcolor: "action.hover", border: 1, borderColor: "divider" }} />
            <Typography variant="caption">Client time (waiting on the agent)</Typography>
          </Stack>
        )}
        {rules.length > 0 && (
          <Stack direction="row" spacing={0.5} alignItems="center">
            <Box sx={{ width: 0, height: 12, borderLeft: 1, borderStyle: "dashed", borderColor: "text.secondary" }} />
            <Typography variant="caption">New turn</Typography>
          </Stack>
        )}
      </Stack>
      <Box
        ref={box}
        role="group"
        aria-label="Timeline"
        tabIndex={0}
        data-span-view="true"
        onWheel={onWheel}
        onMouseDown={onMouseDown}
        onMouseMove={onMouseMove}
        onMouseUp={onMouseUp}
        onMouseLeave={() => setDrag(null)}
        sx={{ position: "relative", userSelect: "none", border: 1, borderColor: "divider", borderRadius: 1 }}
      >
        <Box sx={{ display: "flex", height: 20, borderBottom: 1, borderColor: "divider" }} aria-hidden>
          <Box sx={{ width: LABEL_WIDTH, flexShrink: 0 }} />
          <Box sx={{ position: "relative", width: trackWidth }}>
            {ticksAxis.map((t) => (
              <Typography key={t} variant="caption" color="text.secondary" sx={{ position: "absolute", left: x(t), transform: "translateX(-50%)", whiteSpace: "nowrap" }}>
                {formatDuration(Math.round(t - bounds.from))}
              </Typography>
            ))}
          </Box>
        </Box>
        <VariableSizeList
          ref={list}
          height={bodyHeight}
          width="100%"
          itemCount={rows.length}
          itemSize={(i) => (rows[i].type === "lane" ? LANE : ROW)}
          estimatedItemSize={ROW}
        >
          {Row}
        </VariableSizeList>
        <Box aria-hidden sx={{ position: "absolute", top: 20, bottom: 0, left: LABEL_WIDTH, width: trackWidth, pointerEvents: "none", overflow: "hidden" }}>
          {rules.map((r) => (
            <Box key={r.traceId} data-testid="turn-rule" title={`Turn ${(r.turn ?? 0) + 1}`} sx={{ position: "absolute", top: 0, bottom: 0, left: x(r.at), borderLeft: 1, borderStyle: "dashed", borderColor: "text.secondary" }} />
          ))}
          {gaps.map((g) => (
            <Box
              key={g.key}
              data-testid="gap"
              sx={{ position: "absolute", top: 0, bottom: 0, left: x(g.from), width: Math.max(1, x(g.to) - x(g.from)), bgcolor: "action.hover" }}
            >
              <Typography variant="caption" color="text.secondary" sx={{ position: "absolute", top: 2, left: 4, whiteSpace: "nowrap" }}>
                {g.label}
              </Typography>
            </Box>
          ))}
          {drag && (
            <Box
              sx={{
                position: "absolute",
                top: 0,
                bottom: 0,
                left: Math.min(drag.x0, drag.x1) - (box.current?.getBoundingClientRect().left || 0) - LABEL_WIDTH,
                width: Math.abs(drag.x1 - drag.x0),
                bgcolor: "action.focus",
              }}
            />
          )}
        </Box>
      </Box>
      <Box sx={{ display: "flex" }}>
        <Box sx={{ width: LABEL_WIDTH, flexShrink: 0 }} />
        <Box
          ref={scroller}
          onScroll={onScroll}
          // A pointer affordance only; it duplicates Shift-scroll panning.
          aria-hidden="true"
          data-testid="timeline-scroll"
          sx={{ width: trackWidth, overflowX: fit ? "hidden" : "scroll", overflowY: "hidden" }}
        >
          <Box sx={{ width: scrollWidth, height: fit ? 0 : 1 }} />
        </Box>
      </Box>
    </Box>
  );
};

export default TimelineView;
