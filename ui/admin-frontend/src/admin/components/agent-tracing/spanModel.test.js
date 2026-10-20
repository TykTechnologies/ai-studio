import {
  indexSpans,
  inRange,
  isProblem,
  nextInList,
  partPlaceholder,
  redactionSegments,
  sessionBounds,
} from "./spanModel";
import { bucketize, BUCKETS } from "./TimelineStrip";
import { T0, ids, spans, detail } from "./testFixture";

describe("indexSpans", () => {
  const index = indexSpans(spans);

  it("orders by start time and builds the navigation lists", () => {
    expect(index.ordered.map((s) => s.span_id)).toEqual([ids.llm1, ids.deny, ids.tool1, ids.llm2, ids.client]);
    expect(index.steps).toEqual([ids.llm1, ids.llm2]);
    expect(index.toolCalls).toEqual([ids.tool1, ids.client]);
    expect(index.problems).toEqual([ids.deny, ids.llm2]);
    expect(index.gatewayEvents).toEqual([ids.deny]);
  });

  it("moves through a list from the current span in time order", () => {
    expect(nextInList(index, index.steps, null, 1)).toBe(ids.llm1);
    expect(nextInList(index, index.steps, null, -1)).toBe(ids.llm2);
    expect(nextInList(index, index.steps, ids.llm1, 1)).toBe(ids.llm2);
    expect(nextInList(index, index.steps, ids.llm2, 1)).toBeNull();
    // From a tool call, the next step is the one after it in time.
    expect(nextInList(index, index.steps, ids.tool1, 1)).toBe(ids.llm2);
    expect(nextInList(index, index.toolCalls, ids.llm2, -1)).toBe(ids.tool1);
    expect(nextInList(index, [], null, 1)).toBeNull();
  });

  it("filters by the shared time range", () => {
    const range = { from: T0 + 2500, to: T0 + 7000 };
    const r = indexSpans(spans, range);
    expect(r.ordered.map((s) => s.span_id)).toEqual([ids.llm2, ids.client]);
    expect(inRange(spans[0], range)).toBe(false);
  });

  it("counts errors and gateway denials as problems", () => {
    expect(isProblem(spans.find((s) => s.span_id === ids.deny))).toBe(true);
    expect(isProblem(spans.find((s) => s.span_id === ids.llm1))).toBe(false);
  });
});

describe("content placeholders", () => {
  it.each([
    [{ content_state: "not_captured" }, "Content not captured (mode: off)", false],
    [{ content_state: "metadata", original_bytes: 2048 }, "Content not captured (mode: metadata, 2.0 KB)", false],
    [{ content_state: "hashed" }, "Hashed: only a fingerprint was stored", false],
    [{ content_state: "dropped" }, "Dropped: the gateway was under load and kept no content", false],
    [{ content_state: "redacted", redaction_count: 3 }, "Redacted (3 values)", true],
    [{ content_state: "truncated", original_bytes: 1468006 }, "Truncated (stored part of 1.4 MB)", true],
    [{ content_state: "full", original_bytes: 10 }, "Captured (10 B)", true],
    [{ content_state: "full", media_type: "image/png", original_bytes: 1258291 }, "Binary image/png 1.2 MB (not stored)", false],
  ])("%j", (part, text, loadable) => {
    expect(partPlaceholder(part)).toEqual({ text, loadable });
  });
});

it("splits redaction markers out of text", () => {
  expect(redactionSegments("mail [REDACTED:EMAIL] now [REDACTED:PHONE]")).toEqual([
    { text: "mail " },
    { redacted: "EMAIL" },
    { text: " now " },
    { redacted: "PHONE" },
  ]);
});

it("buckets activity into a fixed number of bars", () => {
  const bounds = sessionBounds(detail().session, spans);
  expect(bounds).toEqual({ from: T0, to: T0 + 6000 });
  const many = Array.from({ length: 5000 }, (_, i) => ({ ...spans[0], span_id: String(i) }));
  const buckets = bucketize(many, bounds);
  expect(buckets).toHaveLength(BUCKETS);
  const withProblems = bucketize(spans, bounds).filter((b) => b.problems).length;
  expect(withProblems).toBeGreaterThan(0);
});
