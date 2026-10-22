import React, { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { VariableSizeList } from "react-window";
import { Box, Button, Stack, Tooltip, Typography } from "@mui/material";
import { useTheme } from "@mui/material/styles";
import { useSessionDetail } from "./SessionDetailContext";
import { KIND_LABELS, durationMs, isProblem, sessionBounds, spanTimes } from "./spanModel";
import { buildTimelineRows, findGaps, turnRules, viewForSpan, zoomView } from "./timelineModel";
import { formatDuration } from "./sessionFilters";

// The Timeline (waterfall) view (spec §6.2 View 2): agent lanes, spans as
// bars coloured by kind (errors red, inferred spans hatched), gateway events
// as ticks, idle "client time" gaps between causally linked spans, and turn
// rules. Ctrl/Cmd-scroll (or a pinch) zooms, a drag selects the shared time
// range, a double-click zooms to a span, Fit shows the whole session.
// Rows are virtualised; zoomed out, they shrink to single-pixel bars.

const LABEL_WIDTH = 180;
const ROW = 22;
const LANE = 26;
const MIN_DRAG_PX = 4;

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
  const barRows = rows.filter((r) => r.type === "row").length;
  // Zoomed out with more rows than fit, rows shrink, down to 1 px bars.
  const rowHeight = fit && barRows * ROW > height ? Math.max(1, Math.floor((height - LANE * (rows.length - barRows)) / barRows)) : ROW;
  const compact = rowHeight < 8;

  useEffect(() => {
    list.current?.resetAfterIndex(0);
  }, [rowHeight, rows]);

  const span = view.to - view.from || 1;
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
    if (!(e.ctrlKey || e.metaKey)) return;
    e.preventDefault();
    const anchor = timeAt(e.clientX) ?? (view.from + view.to) / 2;
    setView((v) => zoomView(v, e.deltaY > 0 ? 1.25 : 0.8, anchor, bounds));
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
      <Tooltip title={`${s.name} · ${s.end_time ? formatDuration(durationMs(s)) : "in progress"}`} disableInteractive>
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
            top: compact ? 0 : 3,
            bottom: compact ? 0 : 3,
            left,
            width,
            bgcolor: colour,
            backgroundImage: s.inferred
              ? "repeating-linear-gradient(45deg, rgba(255,255,255,0.45) 0 3px, transparent 3px 6px)"
              : "none",
            borderRadius: compact ? 0 : 0.5,
            outline: selected ? 2 : 0,
            outlineStyle: "solid",
            outlineColor: "text.primary",
            cursor: "pointer",
            overflow: "hidden",
          }}
        >
          {!compact && width > 60 && (
            <Typography variant="caption" sx={{ color: theme.palette.getContrastText(colour), px: 0.5, whiteSpace: "nowrap" }}>
              {s.name}
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
      <Box style={style} role="group" aria-label={`${r.lane.name} spans`} sx={{ display: "flex" }}>
        <Box sx={{ width: LABEL_WIDTH, flexShrink: 0 }} />
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
  const bodyHeight = Math.min(height, rows.reduce((h, r) => h + (r.type === "lane" ? LANE : rowHeight), 0) || LANE);

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
          Showing {formatDuration(Math.round(span))} · Ctrl/⌘-scroll to zoom · drag to select · double-click a span to zoom to it
        </Typography>
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
          itemSize={(i) => (rows[i].type === "lane" ? LANE : rowHeight)}
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
    </Box>
  );
};

export default TimelineView;
