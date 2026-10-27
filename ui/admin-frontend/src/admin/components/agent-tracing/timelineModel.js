import { inRange, ms, spanTimes } from "./spanModel";

// The Timeline (waterfall) view model (spec §6.2 View 2): one lane per
// agent (root first, sub-agents indented under their parent), spans packed
// into rows so overlapping spans stack, gateway events as ticks on the span
// they belong to, idle gaps between causally linked spans, and turn
// boundaries.

export const GAP_MIN_MS = 1000;
export const ROOT_LANE = "__root__";

/**
 * Lanes in display order: the root agent's lane first, then every declared
 * or inferred sub-agent under its parent, depth first.
 */
export const buildLanes = (spans, agents = []) => {
  const known = new Map(agents.map((a) => [a.id, a]));
  const children = new Map();
  const roots = [];
  for (const a of agents) {
    if (a.parent_agent_id && known.has(a.parent_agent_id)) {
      children.set(a.parent_agent_id, [...(children.get(a.parent_agent_id) || []), a]);
    } else {
      roots.push(a);
    }
  }
  const byFirstSeen = (x, y) => ms(x.first_seen) - ms(y.first_seen) || (x.id < y.id ? -1 : 1);
  const lanes = [];
  // Spans of a root-level agent and spans with no agent share the root lane.
  const rootAgentIds = new Set(roots.map((a) => a.id));
  lanes.push({
    id: ROOT_LANE,
    name: roots.length === 1 ? roots[0].name : "Session",
    depth: 0,
    inferred: roots.length === 1 ? !roots[0].declared : false,
    spans: spans.filter((s) => !s.agent_id || rootAgentIds.has(s.agent_id) || !known.has(s.agent_id)),
  });
  const visit = (a, depth) => {
    lanes.push({ id: a.id, name: a.name, depth, inferred: !a.declared, spans: spans.filter((s) => s.agent_id === a.id) });
    (children.get(a.id) || []).sort(byFirstSeen).forEach((c) => visit(c, depth + 1));
  };
  roots.sort(byFirstSeen).forEach((r) => (children.get(r.id) || []).sort(byFirstSeen).forEach((c) => visit(c, 1)));
  return lanes.filter((l) => l.id === ROOT_LANE || l.spans.length);
};

/**
 * Packs a lane's bar spans into rows: each span goes in the first row whose
 * last span has ended, so overlapping spans stack. Gateway events are not
 * bars; they become ticks on their parent span (or stand alone).
 */
export const packRows = (laneSpans) => {
  const bars = laneSpans.filter((s) => s.kind !== "gateway_event");
  const barIds = new Set(bars.map((s) => s.span_id));
  const ticks = new Map();
  const orphanTicks = [];
  for (const s of laneSpans) {
    if (s.kind !== "gateway_event") continue;
    if (s.parent_span_id && barIds.has(s.parent_span_id)) {
      ticks.set(s.parent_span_id, [...(ticks.get(s.parent_span_id) || []), s]);
    } else orphanTicks.push(s);
  }
  const rows = [];
  const ends = [];
  [...bars]
    .sort((a, b) => ms(a.start_time) - ms(b.start_time) || (a.span_id < b.span_id ? -1 : 1))
    .forEach((s) => {
      const { start, end } = spanTimes(s);
      let r = ends.findIndex((e) => e <= start);
      if (r === -1) {
        r = rows.length;
        rows.push([]);
        ends.push(end);
      }
      rows[r].push(s);
      ends[r] = Math.max(end, start + 1);
    });
  if (orphanTicks.length) rows.push(orphanTicks);
  return { rows, ticks };
};

/**
 * The flat list the virtualised body renders: lane headers, then one row per
 * span in start order (a waterfall, so every bar has a label), then a row of
 * the lane's gateway events that belong to no span.
 */
export const buildTimelineRows = (spans, agents) => {
  const out = [];
  for (const lane of buildLanes(spans, agents)) {
    out.push({ type: "lane", key: `lane-${lane.id}`, lane });
    const { rows, ticks } = packRows(lane.spans);
    const bars = rows
      .flat()
      .filter((s) => s.kind !== "gateway_event")
      .sort((a, b) => ms(a.start_time) - ms(b.start_time) || (a.span_id < b.span_id ? -1 : 1));
    bars.forEach((s) => out.push({ type: "row", key: `${lane.id}-${s.span_id}`, lane, spans: [s], ticks }));
    const barIds = new Set(bars.map((s) => s.span_id));
    const orphans = lane.spans.filter((s) => s.kind === "gateway_event" && !barIds.has(s.parent_span_id));
    if (orphans.length) out.push({ type: "row", key: `${lane.id}-events`, lane, spans: orphans, ticks, events: true });
  }
  return out;
};

/**
 * Idle gaps over GAP_MIN_MS between causally linked spans (a caused_by
 * link): the time the client spent between one span's end and the next
 * one's start, e.g. running a client-side tool.
 */
export const findGaps = (spans) => {
  const byId = new Map(spans.map((s) => [s.span_id, s]));
  const out = [];
  for (const s of spans) {
    for (const l of s.links || []) {
      if (l.type !== "caused_by") continue;
      const src = byId.get(l.span_id);
      if (!src) continue;
      const from = spanTimes(src).end;
      const to = ms(s.start_time);
      if (to - from > GAP_MIN_MS) out.push({ key: `${src.span_id}-${s.span_id}`, from, to, label: `client time ${((to - from) / 1000).toFixed(1)} s` });
    }
  }
  return out;
};

/** Start times of every turn after the first: the vertical turn rules. */
export const turnRules = (spans, traces = []) => {
  const starts = new Map();
  for (const s of spans) {
    const t = ms(s.start_time);
    if (!starts.has(s.trace_id) || t < starts.get(s.trace_id)) starts.set(s.trace_id, t);
  }
  const index = new Map(traces.map((t) => [t.trace_id, t.turn_index]));
  return [...starts.entries()]
    .map(([traceId, at]) => ({ traceId, at, turn: index.get(traceId) }))
    .sort((a, b) => a.at - b.at)
    .slice(1);
};

/** Zooms view by factor around anchor, clamped to bounds (factor < 1 zooms in). */
export const zoomView = (view, factor, anchor, bounds, minSpanMs = 1) => {
  const span = Math.max(minSpanMs, (view.to - view.from) * factor);
  const total = bounds.to - bounds.from;
  if (span >= total) return { ...bounds };
  const ratio = (anchor - view.from) / (view.to - view.from || 1);
  let from = anchor - span * ratio;
  let to = from + span;
  if (from < bounds.from) {
    from = bounds.from;
    to = from + span;
  }
  if (to > bounds.to) {
    to = bounds.to;
    from = to - span;
  }
  return { from, to };
};

/** The view that shows span with a margin either side. */
export const viewForSpan = (span, bounds) => {
  const { start, end } = spanTimes(span);
  const pad = Math.max((end - start) * 0.1, 5);
  return { from: Math.max(bounds.from, start - pad), to: Math.min(bounds.to, Math.max(end, start + 1) + pad) };
};

/** The view moved by deltaMs, kept inside bounds without changing its width. */
export const panView = (view, deltaMs, bounds) => {
  const width = view.to - view.from;
  const from = Math.min(Math.max(bounds.from, view.from + deltaMs), bounds.to - width);
  return { from, to: from + width };
};

// A window with more turns (or bar rows) than this opens zoomed to its
// first turn: across many turns, sub-second spans are slivers at Fit.
export const ZOOM_OPEN_TURNS = 20;
export const ZOOM_OPEN_ROWS = 200;

/** Each turn's time window (its spans' extent), in start order. */
export const turnWindows = (spans) => {
  const by = new Map();
  for (const s of spans) {
    const { start, end } = spanTimes(s);
    const w = by.get(s.trace_id);
    if (!w) by.set(s.trace_id, { traceId: s.trace_id, from: start, to: end });
    else {
      w.from = Math.min(w.from, start);
      w.to = Math.max(w.to, end);
    }
  }
  return [...by.values()].sort((a, b) => a.from - b.from);
};

/** A turn's window with a margin either side, inside bounds. */
export const viewForTurn = (w, bounds) => {
  const pad = Math.max((w.to - w.from) * 0.05, 50);
  return { from: Math.max(bounds.from, w.from - pad), to: Math.min(bounds.to, Math.max(w.to, w.from + 1) + pad) };
};

/** The index of the turn the view is on: the last one starting at or before its middle. */
export const currentTurn = (windows, view) => {
  const mid = (view.from + view.to) / 2;
  let i = 0;
  windows.forEach((w, j) => {
    if (w.from <= mid) i = j;
  });
  return i;
};

/** The view a session (or selected range) opens on: all of it, or for a large one its first turn. */
export const openingView = (orderedSpans, bounds, windows) => {
  const bars = orderedSpans.filter((s) => s.kind !== "gateway_event");
  if ((bars.length <= ZOOM_OPEN_ROWS && windows.length <= ZOOM_OPEN_TURNS) || !windows.length) return bounds;
  return viewForTurn(windows[0], bounds);
};

/**
 * The rows with a span in view, and the lane headers that still have rows:
 * zoomed in, the list holds what the window shows instead of every span.
 */
export const visibleTimelineRows = (rows, view) => {
  const out = [];
  let lane = null;
  for (const r of rows) {
    if (r.type === "lane") {
      lane = r;
      continue;
    }
    if (!r.spans.some((s) => inRange(s, view))) continue;
    if (lane) {
      out.push(lane);
      lane = null;
    }
    out.push(r);
  }
  return out;
};
