import React, { useMemo, useRef, useState } from "react";
import { Box, Button, Collapse, Stack, Typography } from "@mui/material";
import { useSessionDetail } from "./SessionDetailContext";
import { isProblem, sessionBounds, spanTimes } from "./spanModel";
import { formatDuration } from "./sessionFilters";

// The always-visible timeline strip (spec §6.2): the whole session's
// activity at a glance, a drag sets the shared time range that filters
// every view, and the selected span is marked. Activity is drawn in fixed
// buckets, so the DOM stays the same size however many spans there are.

export const BUCKETS = 120;
const MIN_DRAG_PX = 4;

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
  const { spans, detail, range, selectedId, index, dispatch } = useSessionDetail();
  const [open, setOpen] = useState(true);
  const [drag, setDrag] = useState(null); // { x0, x1 } in px
  const ref = useRef(null);
  const bounds = useMemo(() => sessionBounds(detail.session, spans), [detail.session, spans]);
  const buckets = useMemo(() => bucketize(spans, bounds), [spans, bounds]);
  const max = Math.max(1, ...buckets.map((b) => b.count));
  const total = bounds.to - bounds.from;
  const pct = (t) => `${Math.max(0, Math.min(100, ((t - bounds.from) / total) * 100))}%`;

  const timeAt = (clientX) => {
    const rect = ref.current?.getBoundingClientRect();
    if (!rect || !rect.width) return null;
    return bounds.from + ((clientX - rect.left) / rect.width) * total;
  };

  const onMouseDown = (e) => {
    if (e.button !== 0) return;
    setDrag({ x0: e.clientX, x1: e.clientX });
  };
  const onMouseMove = (e) => {
    if (drag) setDrag({ ...drag, x1: e.clientX });
  };
  const onMouseUp = (e) => {
    if (!drag) return;
    const d = { ...drag, x1: e.clientX };
    setDrag(null);
    if (Math.abs(d.x1 - d.x0) < MIN_DRAG_PX) return;
    const a = timeAt(Math.min(d.x0, d.x1));
    const b = timeAt(Math.max(d.x0, d.x1));
    if (a != null && b != null) dispatch({ type: "range", range: { from: a, to: b } });
  };

  const selected = selectedId ? index.byId.get(selectedId)?.span : null;
  const rect = ref.current?.getBoundingClientRect();
  const dragRect =
    drag && rect?.width
      ? { left: Math.min(drag.x0, drag.x1) - rect.left, width: Math.abs(drag.x1 - drag.x0) }
      : null;

  return (
    <Box sx={{ mb: 2 }} data-testid="timeline-strip">
      <Stack direction="row" alignItems="center" spacing={1}>
        <Typography variant="caption" color="text.secondary">
          Timeline · {formatDuration(total)}
        </Typography>
        {range && (
          <>
            <Typography variant="caption" data-testid="range-label">
              Showing {formatDuration(Math.round(range.to - range.from))} from {formatDuration(Math.round(range.from - bounds.from))}
            </Typography>
            <Button size="small" onClick={() => dispatch({ type: "range", range: null })}>
              Clear range
            </Button>
          </>
        )}
        <Box sx={{ flexGrow: 1 }} />
        <Button size="small" onClick={() => setOpen((o) => !o)} aria-expanded={open}>
          {open ? "Hide timeline" : "Show timeline"}
        </Button>
      </Stack>
      <Collapse in={open}>
        <Box
          ref={ref}
          role="img"
          aria-label={`Session activity over ${formatDuration(total)}; drag to filter the views to a time range`}
          data-testid="timeline-strip-track"
          onMouseDown={onMouseDown}
          onMouseMove={onMouseMove}
          onMouseUp={onMouseUp}
          onMouseLeave={() => setDrag(null)}
          sx={{
            position: "relative",
            height: 40,
            bgcolor: "action.hover",
            borderRadius: 1,
            cursor: "crosshair",
            userSelect: "none",
            overflow: "hidden",
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
          {range && (
            <Box
              data-testid="range-rect"
              sx={{
                position: "absolute",
                top: 0,
                bottom: 0,
                left: pct(range.from),
                width: `calc(${pct(range.to)} - ${pct(range.from)})`,
                border: 2,
                borderColor: "primary.dark",
                bgcolor: "action.selected",
              }}
            />
          )}
          {dragRect && (
            <Box sx={{ position: "absolute", top: 0, bottom: 0, left: dragRect.left, width: dragRect.width, bgcolor: "action.focus" }} />
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
