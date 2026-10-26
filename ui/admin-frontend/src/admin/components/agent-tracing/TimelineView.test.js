import React from "react";
import { fireEvent, render, screen, within } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import testTheme from "../../utils/testTheme";
import { SessionDetailProvider, useSessionDetail } from "./SessionDetailContext";
import TimelineView from "./TimelineView";
import { buildLanes, findGaps, packRows, panView, turnRules, zoomView } from "./timelineModel";

const T0 = Date.parse("2026-10-20T10:00:00Z");
const at = (s) => new Date(T0 + s * 1000).toISOString();
const A = "0123456789abcdef0123456789abcdef";
const B = "fedcba9876543210fedcba9876543210";

// Root agent "planner" with a declared sub-agent "researcher" and an
// inferred sub-agent; the client ran a tool between the two root steps.
const llm1 = { span_id: "0000000000000001", trace_id: A, agent_id: "root", kind: "llm_inference", name: "plan", status: "ok", start_time: at(0), end_time: at(1) };
const tool = { span_id: "0000000000000002", trace_id: A, agent_id: "root", kind: "inferred_client_tool", name: "read_file", status: "ok", inferred: true, start_time: at(1), end_time: at(5) };
const llm2 = {
  span_id: "0000000000000003", trace_id: B, agent_id: "root", kind: "llm_inference", name: "answer", status: "error", start_time: at(5), end_time: at(6),
  links: [{ type: "caused_by", trace_id: A, span_id: llm1.span_id, confidence: "exact", rule: "tool_call_id" }],
};
const deny = { span_id: "0000000000000004", trace_id: B, agent_id: "root", parent_span_id: llm2.span_id, kind: "gateway_event", name: "guardrail", status: "ok", start_time: at(5.5), end_time: at(5.5), gateway_event: { type: "guardrail", outcome: "denied" } };
const sub1 = { span_id: "0000000000000005", trace_id: A, agent_id: "research", kind: "llm_inference", name: "search plan", status: "ok", start_time: at(0.5), end_time: at(2) };
const sub2 = { span_id: "0000000000000006", trace_id: A, agent_id: "research", kind: "tool_call", name: "search", status: "ok", start_time: at(1), end_time: at(1.5) };
const guess = { span_id: "0000000000000007", trace_id: A, agent_id: "guess", kind: "llm_inference", name: "helper", status: "ok", start_time: at(2), end_time: at(3) };
const spans = [llm1, tool, llm2, deny, sub1, sub2, guess];
const agents = [
  { id: "root", name: "planner", declared: true, first_seen: at(0) },
  { id: "research", name: "researcher", declared: true, parent_agent_id: "root", first_seen: at(0.5) },
  { id: "guess", name: "Agent 3", declared: false, parent_agent_id: "research", first_seen: at(2) },
];
const detail = {
  session: { id: "0192a4c8-7f00-7000-8000-000000000001", app_id: 7, started_at: at(0), last_activity_at: at(6) },
  agents,
  traces: [{ trace_id: A, turn_index: 0 }, { trace_id: B, turn_index: 1 }],
  viewer: {},
};

let ctx;
const Probe = () => {
  ctx = useSessionDetail();
  return null;
};
const renderTimeline = (list = spans, d = detail) =>
  render(
    <ThemeProvider theme={testTheme}>
      <SessionDetailProvider detail={d} spans={list} resources={[]}>
        <TimelineView height={600} />
        <Probe />
      </SessionDetailProvider>
    </ThemeProvider>,
  );

const bar = (id) => screen.getAllByTestId("timeline-bar").find((b) => b.getAttribute("data-span-id") === id);
const viewOf = () => {
  const v = screen.getByTestId("timeline-view");
  return { from: Number(v.getAttribute("data-view-from")), to: Number(v.getAttribute("data-view-to")) };
};

describe("timeline model", () => {
  it("assigns lanes: root first, sub-agents indented under their parent, inferred labelled", () => {
    const lanes = buildLanes(spans, agents);
    expect(lanes.map((l) => [l.name, l.depth, l.inferred, l.spans.length])).toEqual([
      ["planner", 0, false, 4],
      ["researcher", 1, false, 2],
      ["Agent 3", 2, true, 1],
    ]);
  });

  it("stacks overlapping spans and turns gateway events into ticks", () => {
    const { rows, ticks } = packRows([sub1, sub2, llm1, deny, llm2]);
    // sub2 starts as llm1 ends, so it shares llm1's row; sub1 overlaps both.
    expect(rows.map((r) => r.map((s) => s.span_id))).toEqual([[llm1.span_id, sub2.span_id, llm2.span_id], [sub1.span_id]]);
    expect(ticks.get(llm2.span_id).map((t) => t.span_id)).toEqual([deny.span_id]);
  });

  it("finds client-time gaps between causally linked spans and the turn rules", () => {
    expect(findGaps(spans)).toEqual([{ key: `${llm1.span_id}-${llm2.span_id}`, from: T0 + 1000, to: T0 + 5000, label: "client time 4.0 s" }]);
    expect(turnRules(spans, detail.traces)).toEqual([{ traceId: B, at: T0 + 5000, turn: 1 }]);
  });

  it("pans within the session without changing the zoom", () => {
    const bounds = { from: 0, to: 1000 };
    expect(panView({ from: 100, to: 300 }, 50, bounds)).toEqual({ from: 150, to: 350 });
    expect(panView({ from: 100, to: 300 }, -500, bounds)).toEqual({ from: 0, to: 200 });
    expect(panView({ from: 100, to: 300 }, 5000, bounds)).toEqual({ from: 800, to: 1000 });
  });

  it("zooms around an anchor within the session", () => {
    const bounds = { from: 0, to: 1000 };
    expect(zoomView(bounds, 0.5, 500, bounds)).toEqual({ from: 250, to: 750 });
    expect(zoomView({ from: 0, to: 100 }, 0.5, 0, bounds)).toEqual({ from: 0, to: 50 });
    expect(zoomView({ from: 400, to: 600 }, 20, 500, bounds)).toEqual(bounds);
  });
});

describe("TimelineView", () => {
  it("renders lanes, bars by kind, inferred hatching, error colour, gateway ticks, the gap label and turn rules", () => {
    renderTimeline();
    expect(screen.getAllByTestId("timeline-lane").map((l) => l.textContent)).toEqual(["planner", "↳ researcher", "↳ Agent 3 (inferred)"]);
    expect(bar(tool.span_id)).toHaveStyle({ backgroundImage: expect.stringContaining("repeating-linear-gradient") });
    expect(bar(llm2.span_id)).toHaveStyle({ backgroundColor: testTheme.palette.error.main });
    expect(within(bar(llm2.span_id)).getByTestId("gateway-tick")).toBeInTheDocument();
    expect(screen.getByTestId("gap")).toHaveTextContent("client time 4.0 s");
    expect(screen.getAllByTestId("turn-rule")).toHaveLength(1);
  });

  it("sets the shared time range from a drag", () => {
    renderTimeline();
    const grid = screen.getByRole("group", { name: "Timeline" });
    grid.getBoundingClientRect = () => ({ left: 0, top: 0, width: 960, height: 600, right: 960, bottom: 600 });
    // Track starts after the 180 px label column and is 780 px wide.
    fireEvent.mouseDown(grid, { button: 0, clientX: 180 + 78 });
    fireEvent.mouseMove(grid, { clientX: 180 + 390 });
    fireEvent.mouseUp(grid, { clientX: 180 + 390 });
    expect(ctx.range.from).toBeCloseTo(T0 + 600, -1);
    expect(ctx.range.to).toBeCloseTo(T0 + 3000, -1);
  });

  it("zooms to a span on double-click and back with Fit; Ctrl-scroll zooms", () => {
    renderTimeline();
    expect(viewOf()).toEqual({ from: T0, to: T0 + 6000 });
    fireEvent.doubleClick(bar(sub2.span_id));
    const zoomed = viewOf();
    // The span (1.0-1.5 s) with a 10% margin either side.
    expect(zoomed).toEqual({ from: T0 + 950, to: T0 + 1550 });
    fireEvent.click(screen.getByRole("button", { name: "Fit" }));
    expect(viewOf()).toEqual({ from: T0, to: T0 + 6000 });
    const grid = screen.getByRole("group", { name: "Timeline" });
    grid.getBoundingClientRect = () => ({ left: 0, top: 0, width: 960, height: 600, right: 960, bottom: 600 });
    fireEvent.wheel(grid, { deltaY: -100, ctrlKey: true, clientX: 180 + 390 });
    const v = viewOf();
    expect(v.to - v.from).toBeLessThan(6000);
  });

  it("selects and opens a span on click", () => {
    renderTimeline();
    fireEvent.click(bar(llm1.span_id));
    expect(ctx.drawerSpanId).toBe(llm1.span_id);
    expect(bar(llm1.span_id)).toHaveAttribute("aria-pressed", "true");
  });

  it("keeps the DOM bounded for 5,000 spans, shrinking rows when zoomed out", () => {
    const many = Array.from({ length: 5000 }, (_, i) => ({
      span_id: String(i).padStart(16, "0"), trace_id: A, kind: "tool_call", name: `t${i}`, status: "ok",
      // Every span overlaps the next, so each needs its own row.
      start_time: at(i * 0.01), end_time: at(i * 0.01 + 100),
    }));
    renderTimeline(many, { ...detail, agents: [] });
    expect(screen.getAllByTestId("timeline-bar").length).toBeLessThan(800);
    // Rows keep a readable height and the list scrolls instead of squashing them.
    expect(screen.getAllByRole("group", { name: / spans$/ })[0]).toHaveStyle({ height: "22px" });
  });

  it("labels each row with the span's kind and model or tool, with a legend", () => {
    renderTimeline();
    const labels = screen.getAllByTestId("timeline-label").map((l) => l.textContent);
    expect(labels.slice(0, 3)).toEqual(["LLM · plan", "Client tool · read_file", "LLM · answer"]);
    expect(screen.getByTestId("timeline-legend")).toHaveTextContent("LLM call");
    fireEvent.click(screen.getAllByTestId("timeline-label")[1]);
    expect(ctx.drawerSpanId).toBe(tool.span_id);
  });

  it("pans a zoomed view with Shift-scroll and the scrollbar, never past the session", () => {
    renderTimeline();
    fireEvent.click(screen.getByRole("button", { name: "Zoom in" }));
    expect(viewOf()).toEqual({ from: T0 + 1500, to: T0 + 4500 });
    const grid = screen.getByRole("group", { name: "Timeline" });
    // 780 px of track shows 3 s, so 260 px is 1 s.
    fireEvent.wheel(grid, { deltaY: 260, shiftKey: true });
    expect(viewOf()).toEqual({ from: T0 + 2500, to: T0 + 5500 });
    fireEvent.wheel(grid, { deltaY: 2600, shiftKey: true });
    expect(viewOf()).toEqual({ from: T0 + 3000, to: T0 + 6000 });
    const bar = screen.getByTestId("timeline-scroll");
    bar.scrollLeft = 0;
    fireEvent.scroll(bar);
    expect(viewOf()).toEqual({ from: T0, to: T0 + 3000 });
  });
});
