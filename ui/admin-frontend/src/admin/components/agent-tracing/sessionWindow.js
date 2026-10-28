import { ms } from "./spanModel";

// Large sessions (a never-idle bot can run for a day) are read through a
// window: the page loads only the spans of a few whole turns, and the
// timeline strip draws the whole session from the turn list the session
// detail already carries. Smaller sessions load in full.

/** Above this many spans a session opens windowed. */
export const WINDOWED_SPANS = 5000;
/** Spans a window aims for: whole turns are added up to this budget. */
export const WINDOW_SPANS = 2000;
/** A window stops loading here (one huge turn) and says it is cut short. */
export const WINDOW_MAX_SPANS = 5000;

export const isWindowed = (detail) => (detail?.session?.totals?.span_count || 0) > WINDOWED_SPANS;

/**
 * The session's turns in time order as { from, to, spans, problem } (epoch
 * ms). A turn still open ends where the next one starts, the last at the
 * session's last activity.
 */
export const turnsOf = (detail) => {
  const traces = [...(detail?.traces || [])].sort((a, b) => ms(a.started_at) - ms(b.started_at));
  const last = ms(detail?.session?.ended_at || detail?.session?.last_activity_at);
  return traces.map((t, i) => {
    const from = ms(t.started_at);
    let to = ms(t.ended_at);
    if (!Number.isFinite(to)) to = i + 1 < traces.length ? ms(traces[i + 1].started_at) : last;
    if (!Number.isFinite(to) || to < from) to = from;
    return {
      from,
      to,
      spans: t.totals?.span_count || 0,
      problem: t.status === "error" || (t.totals?.error_count || 0) > 0,
    };
  });
};

// The API's to is exclusive: a window ends just after its last turn.
const rangeOf = (turns, a, b) => ({ from: turns[a].from, to: Math.max(...turns.slice(a, b + 1).map((t) => t.to)) + 1 });

/**
 * The window of whole turns around turn i, growing to both sides (or only
 * one, dir -1 or 1) while the spans stay within budget. Turn i is always
 * in, however large.
 */
export const windowAround = (turns, i, { budget = WINDOW_SPANS, dir = 0 } = {}) => {
  if (!turns.length) return null;
  i = Math.max(0, Math.min(turns.length - 1, i));
  let a = i;
  let b = i;
  let total = turns[i].spans;
  for (let grew = true; grew; ) {
    grew = false;
    if (dir >= 0 && b + 1 < turns.length && total + turns[b + 1].spans <= budget) {
      b += 1;
      total += turns[b].spans;
      grew = true;
    }
    if (dir <= 0 && a > 0 && total + turns[a - 1].spans <= budget) {
      a -= 1;
      total += turns[a].spans;
      grew = true;
    }
  }
  return rangeOf(turns, a, b);
};

/** The index of the turn at time t: the last turn starting at or before it. */
export const turnAt = (turns, t) => {
  let found = 0;
  for (let i = 0; i < turns.length; i += 1) {
    if (turns[i].from <= t) found = i;
    else break;
  }
  return found;
};

/**
 * Where a large session opens: around its first turn with a problem, else
 * on its latest turns, the two places a diagnosis starts.
 */
export const openingWindow = (turns) => {
  if (!turns.length) return null;
  const firstProblem = turns.findIndex((t) => t.problem);
  if (firstProblem >= 0) return windowAround(turns, firstProblem);
  return windowAround(turns, turns.length - 1, { dir: -1 });
};

/** The window next to range: dir -1 the turns before it, 1 those after. */
export const adjacentWindow = (turns, range, dir) => {
  if (!turns.length || !range) return null;
  if (dir < 0) {
    let i = -1;
    for (let k = 0; k < turns.length && turns[k].from < range.from; k += 1) i = k;
    return i < 0 ? null : windowAround(turns, i, { dir: -1 });
  }
  const i = turns.findIndex((t) => t.from >= range.to);
  return i < 0 ? null : windowAround(turns, i, { dir: 1 });
};

/** Activity buckets for the strip from turns, weighted by their spans. */
export const bucketizeTurns = (turns, bounds, n) => {
  const width = bounds.to - bounds.from;
  const buckets = Array.from({ length: n }, () => ({ count: 0, problems: 0 }));
  for (const t of turns) {
    const a = Math.max(0, Math.min(n - 1, Math.floor(((t.from - bounds.from) / width) * n)));
    const b = Math.max(a, Math.min(n - 1, Math.floor(((t.to - bounds.from) / width) * n)));
    const share = (t.spans || 1) / (b - a + 1);
    for (let i = a; i <= b; i += 1) {
      buckets[i].count += share;
      if (t.problem) buckets[i].problems += 1;
    }
  }
  return buckets;
};
