import React, { useMemo, useRef, useState } from "react";
import { Alert, Box, Button, Collapse, Stack, Typography } from "@mui/material";
import { useSessionDetail } from "./SessionDetailContext";
import { isProblem, sessionBounds, spanTimes } from "./spanModel";
import { formatDuration } from "./sessionFilters";
import useWindowDrag from "./useWindowDrag";
import { adjacentWindow, bucketizeTurns, WINDOW_MAX_SPANS } from "./sessionWindow";

// The always-visible timeline strip (spec §6.2): the whole session's
// activity at a glance, a drag sets the shared time range that filters
// every view, and the selected span is marked. Once set, the range has
// handles to move its start and end (by pointer or arrow keys), and its
// body drags to move the whole window. Activity is drawn in fixed buckets,
// so the DOM stays the same size however many spans there are. In a
// windowed (large) session the activity comes from the turn list, the range
// is the window of spans loaded, and Earlier/Later step it by whole turns.

export const BUCKETS = 120;
const MIN_DRAG_PX = 4;

/**
 * The range a drag produces. mode is "new" (from the press point), "start"
 * or "end" (one edge), or "move" (the whole range by the pointer's travel).
 * The result stays inside bounds and at least minMs wide.
 */
export const dragRange = (mode, { range, t0, t }, bounds, minMs) => {
  const clamp = (x) => Math.max(bounds.from, Math.min(bounds.to, x));
  t = clamp(t);
  switch (mode) {
    case "new":
      return { from: Math.min(t0, t), to: Math.max(t0, t) };
    case "start":
      return { from: Math.min(t, range.to - minMs), to: range.to };
    case "end":
      return { from: range.from, to: Math.max(t, range.from + minMs) };
    case "move": {
      const width = range.to - range.from;
      const from = Math.max(bounds.from, Math.min(bounds.to - width, range.from + (t - t0)));
      return { from, to: from + width };
    }
    default:
      return range;
  }
};

export const bucketize = (spans, bounds, n = BUCKETS) => {
  const span = bounds.to - bounds.from;
  const buckets = Array.from({ length: n }, () => ({ count: 0, problems: 0 }));
  for (const s of spans) {
    const { start, end } = spanTimes(s);
    const a = Math.max(0, Math.min(n - 1, Math.floor(((start - bounds.from) / span) * n)));
    const b = Math.max(a, Math.min(n - 1, Math.floor(((end - bounds.from) / span) * n)));
    for (let i = a; i <= b; i += 1) {
      buckets[i].count += 1;
      if (isProblem(s)) buckets[i].problems += 1;
    }
  }
  return buckets;
};

const TimelineStrip = () => {
  const { spans, detail, range, viewport, selectedId, index, dispatch, windowed, turns, truncated } = useSessionDetail();
  const [open, setOpen] = useState(true);
  const [draft, setDraft] = useState(null); // the range while a drag is under way
  const ref = useRef(null);
  const startDrag = useWindowDrag();
  const bounds = useMemo(() => sessionBounds(detail.session, spans), [detail.session, spans]);
  const buckets = useMemo(
    () => (windowed ? bucketizeTurns(turns, bounds, BUCKETS) : bucketize(spans, bounds)),
    [windowed, turns, spans, bounds],
  );
  const max = Math.max(1, ...buckets.map((b) => b.count));
  const total = bounds.to - bounds.from;
  const pct = (t) => `${Math.max(0, Math.min(100, ((t - bounds.from) / total) * 100))}%`;

  const timeAt = (clientX) => {
    const rect = ref.current?.getBoundingClientRect();
    if (!rect || !rect.width) return null;
    return bounds.from + ((clientX - rect.left) / rect.width) * total;
  };

  const minMs = Math.max(1, total / 1000);
  // A drag edits a draft; the shared range (which re-filters every view)
  // changes once, on release.
  const begin = (mode) => (e) => {
    if (e.button !== 0) return;
    e.preventDefault();
    e.stopPropagation();
    const x0 = e.clientX;
    const t0 = timeAt(x0);
    if (t0 == null) return;
    let last = mode === "new" ? null : range;
    startDrag(
      (m) => {
        const t = timeAt(m.clientX);
        if (t == null || (mode === "new" && Math.abs(m.clientX - x0) < MIN_DRAG_PX)) return;
        last = dragRange(mode, { range, t0, t }, bounds, minMs);
        setDraft(last);
      },
      () => {
        setDraft(null);
        if (last && last !== range) dispatch({ type: "range", range: last });
      },
    );
  };
  const nudge = (edge) => (e) => {
    const step = (e.shiftKey ? 0.1 : 0.01) * total;
    const delta = e.key === "ArrowLeft" ? -step : e.key === "ArrowRight" ? step : 0;
    if (!delta || !range) return;
    e.preventDefault();
    const t = (edge === "start" ? range.from : range.to) + delta;
    dispatch({ type: "range", range: dragRange(edge, { range, t0: 0, t }, bounds, minMs) });
  };
  const shown = draft || range;
  const earlier = windowed ? adjacentWindow(turns, range, -1) : null;
  const later = windowed ? adjacentWindow(turns, range, 1) : null;

  const selected = selectedId ? index.byId.get(selectedId)?.span : null;
  const handle = (edge) => {
    const at = edge === "start" ? shown.from : shown.to;
    return (
      <Box
        role="slider"
        tabIndex={0}
        aria-label={edge === "start" ? "Range start" : "Range end"}
        aria-valuemin={0}
        aria-valuemax={Math.round(total)}
        aria-valuenow={Math.round(at - bounds.from)}
        aria-valuetext={formatDuration(Math.round(at - bounds.from))}
        data-testid={`range-handle-${edge}`}
        onMouseDown={begin(edge)}
        onKeyDown={nudge(edge)}
        sx={{
          position: "absolute",
          top: 0,
          bottom: 0,
          [edge === "start" ? "left" : "right"]: -7,
          width: 14,
          cursor: "ew-resize",
          display: "flex",
          justifyContent: "center",
          zIndex: 1,
          "&::after": { content: '""', width: 4, my: 1, borderRadius: 2, bgcolor: "primary.dark" },
          "&:focus-visible": { outline: 2, outlineStyle: "solid", outlineColor: "primary.main" },
        }}
      />
    );
  };

  return (
    <Box sx={{ mb: 2 }} data-testid="timeline-strip">
      <Stack direction="row" alignItems="center" spacing={1}>
        <Typography variant="caption" color="text.secondary">
          Timeline · {formatDuration(total)}
        </Typography>
        {shown && (
          <>
            <Typography variant="caption" data-testid="range-label">
              Showing {formatDuration(Math.round(shown.to - shown.from))} from {formatDuration(Math.round(shown.from - bounds.from))}
            </Typography>
            {!windowed && (
              <Button size="small" onClick={() => dispatch({ type: "range", range: null })}>
                Clear range
              </Button>
            )}
          </>
        )}
        {windowed && (
          <>
            <Button size="small" disabled={!earlier} onClick={() => dispatch({ type: "range", range: earlier })}>
              ‹ Earlier
            </Button>
            <Button size="small" disabled={!later} onClick={() => dispatch({ type: "range", range: later })}>
              Later ›
            </Button>
          </>
        )}
        <Box sx={{ flexGrow: 1 }} />
        <Button size="small" onClick={() => setOpen((o) => !o)} aria-expanded={open}>
          {open ? "Hide timeline" : "Show timeline"}
        </Button>
      </Stack>
      {windowed && (
        <Typography variant="caption" color="text.secondary" display="block" data-testid="windowed-note">
          Large session ({(detail.session?.totals?.span_count || 0).toLocaleString()} spans): the views show the highlighted
          window of whole turns. Drag on the timeline, or step Earlier and Later, to load another part.
        </Typography>
      )}
      {truncated && (
        <Alert severity="warning" sx={{ my: 1 }} data-testid="window-truncated">
          This range has more than {WINDOW_MAX_SPANS.toLocaleString()} spans; only the first {WINDOW_MAX_SPANS.toLocaleString()} are
          shown. Narrow the range to see the rest.
        </Alert>
      )}
      <Collapse in={open}>
        <Box
          ref={ref}
          role="img"
          aria-label={`Session activity over ${formatDuration(total)}; drag to filter the views to a time range`}
          data-testid="timeline-strip-track"
          onMouseDown={begin("new")}
          sx={{
            position: "relative",
            height: 40,
            bgcolor: "action.hover",
            borderRadius: 1,
            cursor: "crosshair",
            userSelect: "none",
            overflow: "visible",
          }}
        >
          {buckets.map((b, i) =>
            b.count ? (
              <Box
                key={i}
                sx={{
                  position: "absolute",
                  bottom: 0,
                  left: `${(i / BUCKETS) * 100}%`,
                  width: `${100 / BUCKETS}%`,
                  height: `${Math.max(10, (Math.log1p(b.count) / Math.log1p(max)) * 100)}%`,
                  bgcolor: b.problems ? "error.main" : "primary.main",
                  opacity: 0.7,
                }}
              />
            ) : null,
          )}
          {shown && (
            <Box
              data-testid="range-rect"
              title="Drag to move the range; drag an edge to resize it"
              onMouseDown={range ? begin("move") : undefined}
              sx={{
                position: "absolute",
                top: 0,
                bottom: 0,
                left: pct(shown.from),
                width: `calc(${pct(shown.to)} - ${pct(shown.from)})`,
                border: 2,
                borderColor: "primary.dark",
                bgcolor: "action.selected",
                cursor: range ? "grab" : "crosshair",
              }}
            >
              {range && handle("start")}
              {range && handle("end")}
            </Box>
          )}
          {viewport && (viewport.from > bounds.from || viewport.to < bounds.to) && (
            // Where the Timeline view is: a box, or a marked line when the
            // window is too narrow to draw (one turn of a long session).
            <Box
              data-testid="viewport-cursor"
              title={`Timeline view: ${formatDuration(Math.round(viewport.to - viewport.from))} from ${formatDuration(Math.round(viewport.from - bounds.from))}`}
              sx={{
                position: "absolute",
                top: -4,
                bottom: -4,
                left: pct(viewport.from),
                width: `calc(max(2px, ${pct(viewport.to)} - ${pct(viewport.from)}))`,
                border: 2,
                borderColor: "warning.main",
                borderRadius: 0.5,
                pointerEvents: "none",
                zIndex: 2,
                "&::before": {
                  content: '""',
                  position: "absolute",
                  top: -7,
                  left: "50%",
                  transform: "translateX(-50%)",
                  borderLeft: "6px solid transparent",
                  borderRight: "6px solid transparent",
                  borderTop: "6px solid",
                  borderTopColor: "warning.main",
                },
              }}
            />
          )}
          {selected && (
            <Box
              data-testid="selected-marker"
              sx={{ position: "absolute", top: 0, bottom: 0, width: 2, left: pct(spanTimes(selected).start), bgcolor: "text.primary" }}
            />
          )}
        </Box>
      </Collapse>
    </Box>
  );
};

export default TimelineStrip;
