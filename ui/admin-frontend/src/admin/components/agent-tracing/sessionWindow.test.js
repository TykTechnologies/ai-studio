import {
  adjacentWindow,
  bucketizeTurns,
  isWindowed,
  openingWindow,
  turnAt,
  turnsOf,
  windowAround,
  WINDOWED_SPANS,
} from "./sessionWindow";

const T0 = Date.UTC(2026, 9, 27, 0, 0, 0);
const iso = (ms) => new Date(T0 + ms).toISOString();

// Ten turns a minute apart, each 10 s long with 500 spans.
const trace = (i, extra = {}) => ({
  trace_id: `t${i}`,
  turn_index: i,
  started_at: iso(i * 60_000),
  ended_at: iso(i * 60_000 + 10_000),
  status: "ok",
  totals: { span_count: 500, error_count: 0 },
  ...extra,
});
const detailOf = (traces, spanCount = 5000) => ({
  session: { started_at: iso(0), last_activity_at: iso(10 * 60_000), totals: { span_count: spanCount } },
  traces,
});
const ten = () => Array.from({ length: 10 }, (_, i) => trace(i));

describe("isWindowed", () => {
  it("windows only sessions above the span threshold", () => {
    expect(isWindowed(detailOf([], WINDOWED_SPANS))).toBe(false);
    expect(isWindowed(detailOf([], WINDOWED_SPANS + 1))).toBe(true);
    expect(isWindowed(null)).toBe(false);
  });
});

describe("turnsOf", () => {
  it("orders turns and ends an open one at the next turn or the session's last activity", () => {
    const turns = turnsOf(
      detailOf([trace(2, { ended_at: undefined }), trace(0), trace(1, { ended_at: undefined, status: "error" })]),
    );
    expect(turns.map((t) => t.from)).toEqual([T0, T0 + 60_000, T0 + 120_000]);
    expect(turns[1].to).toBe(T0 + 120_000);
    expect(turns[1].problem).toBe(true);
    expect(turns[2].to).toBe(T0 + 600_000);
  });
});

describe("windowAround", () => {
  const turns = turnsOf(detailOf(ten()));

  it("takes whole turns around the anchor within the span budget", () => {
    // 2000 spans = 4 turns: 5, then 6, 4, 7 alternately (later first).
    expect(windowAround(turns, 5)).toEqual({ from: T0 + 4 * 60_000, to: T0 + 7 * 60_000 + 10_001 });
  });

  it("grows in one direction only when asked", () => {
    expect(windowAround(turns, 9, { dir: -1 })).toEqual({ from: T0 + 6 * 60_000, to: T0 + 9 * 60_000 + 10_001 });
    expect(windowAround(turns, 0, { dir: 1 })).toEqual({ from: T0, to: T0 + 3 * 60_000 + 10_001 });
  });

  it("keeps an anchor turn larger than the budget", () => {
    const big = turnsOf(detailOf([trace(0), trace(1, { totals: { span_count: 9000 } }), trace(2)]));
    expect(windowAround(big, 1)).toEqual({ from: T0 + 60_000, to: T0 + 70_001 });
  });
});

describe("openingWindow", () => {
  it("opens on the first problem turn", () => {
    const traces = ten();
    traces[2] = trace(2, { status: "error" });
    traces[7] = trace(7, { status: "error" });
    const w = openingWindow(turnsOf(detailOf(traces)));
    expect(w.from).toBeLessThanOrEqual(T0 + 2 * 60_000);
    expect(w.to).toBeGreaterThan(T0 + 2 * 60_000 + 10_000);
    expect(w.to).toBeLessThan(T0 + 7 * 60_000);
  });

  it("opens on the latest turns without problems", () => {
    expect(openingWindow(turnsOf(detailOf(ten())))).toEqual({ from: T0 + 6 * 60_000, to: T0 + 9 * 60_000 + 10_001 });
  });
});

describe("adjacentWindow and turnAt", () => {
  const turns = turnsOf(detailOf(ten()));
  const mid = { from: T0 + 4 * 60_000, to: T0 + 5 * 60_000 + 10_001 };

  it("steps to the turns before and after a window", () => {
    expect(adjacentWindow(turns, mid, -1)).toEqual({ from: T0, to: T0 + 3 * 60_000 + 10_001 });
    expect(adjacentWindow(turns, mid, 1)).toEqual({ from: T0 + 6 * 60_000, to: T0 + 9 * 60_000 + 10_001 });
    expect(adjacentWindow(turns, { from: T0, to: T0 + 10_001 }, -1)).toBeNull();
    expect(adjacentWindow(turns, { from: T0 + 9 * 60_000, to: T0 + 9 * 60_000 + 10_001 }, 1)).toBeNull();
  });

  it("finds the turn at a time", () => {
    expect(turnAt(turns, T0 + 4 * 60_000 + 5)).toBe(4);
    expect(turnAt(turns, T0 - 1)).toBe(0);
  });
});

describe("bucketizeTurns", () => {
  it("spreads each turn's spans over its buckets and marks problem turns", () => {
    const traces = [trace(0), trace(5, { status: "error" })];
    const turns = turnsOf(detailOf(traces));
    const buckets = bucketizeTurns(turns, { from: T0, to: T0 + 10 * 60_000 }, 10);
    expect(buckets[0].count).toBe(500);
    expect(buckets[5].problems).toBe(1);
    expect(buckets.reduce((n, b) => n + b.count, 0)).toBe(1000);
  });
});
