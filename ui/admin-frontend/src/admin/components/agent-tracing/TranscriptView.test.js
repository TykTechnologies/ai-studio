import React from "react";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import "@testing-library/jest-dom";
import { ThemeProvider } from "@mui/material/styles";
import { MemoryRouter } from "react-router-dom";
import testTheme from "../../utils/testTheme";
import { SessionDetailProvider, useSessionDetail } from "./SessionDetailContext";
import TranscriptView from "./TranscriptView";
import { argPreview, buildTranscript, gatewayChip, resultPreview } from "./transcriptModel";
import { indexSpans } from "./spanModel";

const T0 = Date.parse("2026-10-20T10:00:00Z");
const at = (s) => new Date(T0 + s * 1000).toISOString();
const TRACE = "0123456789abcdef0123456789abcdef";
const TRACE2 = "fedcba9876543210fedcba9876543210";

// A session with a user message, two steps, two parallel tool calls (one
// inferred), a guardrail denial, a client-side tool and a declared
// sub-agent, with inline previews as a content reader receives them.
const llm1 = {
  span_id: "0000000000000001", trace_id: TRACE, kind: "llm_inference", name: "chat", status: "ok",
  start_time: at(0), end_time: at(1), response_model: "gpt-4.1", ttft_ms: 120, finish_reasons: ["tool_calls"],
  usage: { input_tokens: 100, output_tokens: 20, cost_micros: 1500 }, content_state: "full",
  attributes: {
    "tyk.tools.available": "4",
    "tyk.tools.offered": '["search","lookup","refund","notify"]',
    "tyk.tool_calls.not_offered": '["delete_order","search"]',
  },
  parts: [
    { part: "user_message", content_state: "full", media_type: "text/plain", original_bytes: 40, content: "Please refund order 42 for [REDACTED:EMAIL]" },
    { part: "response", content_state: "full", media_type: "text/plain", original_bytes: 20, content: "Looking that up **now**." },
  ],
};
const search = {
  span_id: "0000000000000002", trace_id: TRACE, parent_span_id: llm1.span_id, parent_confidence: "exact", parent_rule: "tool_call_id",
  kind: "tool_call", name: "tools/call search", tool_name: "search", status: "ok", start_time: at(1.1), end_time: at(1.9),
  content_state: "full", attribution: { via: "tyk_gateway", in_catalogue: false, catalogue_type: "mcp_server", server: "crm" },
  parts: [
    { part: "tool_args", content_state: "full", media_type: "application/json", original_bytes: 30, content: '{"order":42,"verbose":true,"filter":{"x":1}}' },
    { part: "tool_result", content_state: "full", media_type: "application/json", original_bytes: 2355, content: '{"a":1,"b":2,"c":3}' },
  ],
};
const lookup = {
  span_id: "0000000000000003", trace_id: TRACE, parent_span_id: llm1.span_id, parent_confidence: "strong", parent_rule: "mcp_name_args",
  kind: "tool_call", name: "tools/call lookup", tool_name: "lookup", status: "error", start_time: at(1.2), end_time: at(1.5),
  content_state: "hashed", parts: [{ part: "tool_result", content_state: "hashed", media_type: "application/json", original_bytes: 10 }],
  attribution: { via: "studio_mcp", in_catalogue: true, catalogue_type: "tool", catalogue_id: "9" },
  attributes: {
    "tyk.attribution.confidence": "weak",
    "tyk.attribution.rule": "catalogue_ambiguous",
    "tyk.attribution.candidates": '["tool:9","mcp_server:3"]',
  },
};
const deny = {
  span_id: "0000000000000004", trace_id: TRACE, parent_span_id: llm1.span_id, kind: "gateway_event", name: "guardrail", status: "ok",
  start_time: at(0.5), end_time: at(0.5), content_state: "metadata",
  gateway_event: { type: "guardrail", outcome: "denied", name: "pii-filter" },
};
const llm2 = {
  span_id: "0000000000000005", trace_id: TRACE, kind: "llm_inference", name: "chat", status: "ok",
  start_time: at(3), end_time: at(4), response_model: "gpt-4.1", content_state: "redacted",
  links: [{ type: "caused_by", trace_id: TRACE, span_id: search.span_id, confidence: "exact", rule: "tool_call_id" }],
  parts: [{ part: "response", content_state: "redacted", media_type: "text/plain", original_bytes: 99, redaction_count: 3 }],
};
const client = {
  span_id: "0000000000000006", trace_id: TRACE2, kind: "inferred_client_tool", name: "read_file", status: "ok",
  start_time: at(5), end_time: at(7), inferred: true, content_state: "not_captured",
};
const subLLM = {
  span_id: "0000000000000007", trace_id: TRACE, agent_id: "agent-sub", kind: "llm_inference", name: "sub chat", status: "ok",
  start_time: at(2), end_time: at(2.5), response_model: "gpt-4.1-mini", usage: { input_tokens: 10, output_tokens: 5 }, content_state: "metadata",
};
const spans = [llm1, search, lookup, deny, llm2, client, subLLM];
const detail = (viewer = { can_read_content: true }) => ({
  session: { id: "0192a4c8-7f00-7000-8000-000000000001", app_id: 7, started_at: at(0), last_activity_at: at(7) },
  agents: [
    { id: "agent-root", name: "planner", declared: true, first_seen: at(0), last_seen: at(7) },
    { id: "agent-sub", name: "researcher", declared: true, parent_agent_id: "agent-root", spawned_by_span_id: search.span_id, first_seen: at(2), last_seen: at(2.5), total_tokens: 15, total_cost_micros: 300 },
  ],
  traces: [
    { trace_id: TRACE, turn_index: 0, totals: { cost_micros: 1500 } },
    { trace_id: TRACE2, turn_index: 1, totals: { cost_micros: 0 } },
  ],
  viewer,
});

let ctx;
const Probe = () => {
  ctx = useSessionDetail();
  return null;
};

const renderTranscript = ({ list = spans, d = detail(), previews = "included" } = {}) =>
  render(
    <ThemeProvider theme={testTheme}>
      <SessionDetailProvider detail={d} spans={list} resources={[]} previews={previews}>
        <TranscriptView height={4000} />
        <Probe />
      </SessionDetailProvider>
    </ThemeProvider>,
  );

describe("transcript model", () => {
  it("builds turns, the user message, steps with grouped tools, and the sub-agent", () => {
    const rows = buildTranscript(indexSpans(spans).ordered, detail());
    expect(rows.map((r) => r.type)).toEqual(["turn", "user", "step", "subagent", "step", "turn", "tool"]);
    const step = rows[2];
    expect(step.toolGroups).toHaveLength(1);
    expect(step.toolGroups[0].map((t) => t.span.tool_name)).toEqual(["search", "lookup"]);
    expect(step.toolGroups[0][0].next.span_id).toBe(llm2.span_id);
    expect(step.events.map((e) => e.span_id)).toEqual([deny.span_id]);
  });

  it("previews arguments and results", () => {
    expect(argPreview('{"order":42,"verbose":true,"filter":{"x":1}}')).toBe("order: 42, verbose: true");
    expect(argPreview(`{"q":"${"x".repeat(200)}"}`)).toHaveLength(80);
    expect(resultPreview('{"a":1,"b":2}', 2355)).toBe("JSON object, 2 keys, 2.3 KB");
    expect(resultPreview("[1,2,3]", 7)).toBe("JSON array, 3 items, 7 B");
    expect(resultPreview("x".repeat(500))).toHaveLength(200);
  });

  it("words gateway events in plain language", () => {
    expect(gatewayChip(deny)).toEqual({ text: "Blocked by guardrail pii-filter", severity: "error" });
    expect(
      gatewayChip({ gateway_event: { type: "fallback", outcome: "modified", from_model: "claude-x", to_model: "gpt-y", reason: "503" } }),
    ).toEqual({ text: "Fallback: claude-x → gpt-y after 503", severity: "warning" });
    expect(gatewayChip({ gateway_event: { type: "retry", attempt_number: 2, attempt_total: 3 } }).text).toBe("Retry 2/3");
    expect(gatewayChip({ gateway_event: { type: "attempt", attempt_number: 1, attempt_total: 2, provider: "openai", reason: "429" } }).text).toBe(
      "Attempt 1/2 (openai) after 429",
    );
    expect(gatewayChip({ gateway_event: { type: "budget", budget_id: "team-a", budget_used_pct: 92 } }).text).toBe("Budget team-a at 92%");
    expect(gatewayChip({ gateway_event: { type: "rate_limit", wait_ms: 1200 } }).text).toBe("Rate limited — 1.2 s wait");
    // Unknown types and outcomes render neutrally, never fail.
    expect(gatewayChip({ name: "x", gateway_event: { type: "overload", outcome: "shed" } })).toEqual({ text: "overload shed", severity: "default" });
  });
});

describe("TranscriptView", () => {
  it("renders the user message, the step with its metadata, and markdown text", () => {
    renderTranscript();
    const user = screen.getByTestId("user-message");
    expect(user).toHaveTextContent("Please refund order 42 for");
    expect(within(user).getByText("REDACTED:EMAIL")).toBeInTheDocument();
    const step = screen.getAllByTestId("assistant-step")[0];
    expect(within(step).getByText("now").tagName).toBe("STRONG");
    expect(within(step).getByTestId("step-meta")).toHaveTextContent("gpt-4.1");
    expect(within(step).getByTestId("step-meta")).toHaveTextContent("TTFT 120 ms");
    expect(within(step).getByTestId("step-meta")).toHaveTextContent("tools available: 4");
  });

  it("shows tool card → result → next-step connector, the parallel group and the inferred badge", () => {
    renderTranscript();
    const group = screen.getByTestId("parallel-group");
    expect(group).toHaveTextContent("2 parallel calls");
    const [searchCard, lookupCard] = within(group).getAllByTestId("tool-card");
    expect(searchCard).toHaveTextContent("search(order: 42, verbose: true)");
    expect(searchCard).toHaveTextContent("MCP · crm");
    expect(within(searchCard).getByTestId("via-gateway-badge")).toBeInTheDocument();
    expect(within(searchCard).getByTestId("not-in-catalogue-badge")).toBeInTheDocument();
    expect(within(searchCard).getByTestId("result-preview")).toHaveTextContent("JSON object, 3 keys, 2.3 KB");
    expect(within(group).getByTestId("next-connector")).toHaveTextContent("next: LLM call gpt-4.1");
    expect(within(lookupCard).getByTestId("inferred-badge")).toHaveTextContent("inferred");
    expect(within(lookupCard).getByTitle("Error")).toBeInTheDocument();
  });

  // jyx.51 attributes: the offered tools behind the count, a warning for a
  // call to a tool the request did not offer, and where Studio ran a tool.
  it("shows the tools offered, a not-offered call and the Studio route", async () => {
    renderTranscript();
    const step = screen.getAllByTestId("assistant-step")[0];
    expect(within(step).getByTestId("not-offered-chip")).toHaveTextContent("called a tool not offered: delete_order, search");
    // The card of a call the request did not offer carries the badge.
    const searchCard = screen.getAllByTestId("tool-card").find((c) => c.textContent.includes("search("));
    expect(within(searchCard).getByTestId("not-offered-badge")).toBeInTheDocument();
    expect(screen.getAllByTestId("not-offered-badge")).toHaveLength(1);
    fireEvent.mouseOver(within(step).getByTestId("tools-available"));
    expect(await screen.findByRole("tooltip")).toHaveTextContent("Offered: search, lookup, refund, notify");
    const lookupCard = screen.getAllByTestId("tool-card").find((c) => c.textContent.includes("lookup"));
    expect(within(lookupCard).getByTestId("via-studio-badge")).toHaveTextContent("via Studio MCP");
    expect(screen.getAllByTestId("via-studio-badge")).toHaveLength(1);
  });

  it("explains a weak catalogue match with its rule and candidates", async () => {
    renderTranscript();
    const badge = screen.getByTestId("weak-match-badge");
    fireEvent.mouseOver(badge);
    expect(await screen.findByRole("tooltip")).toHaveTextContent(
      "Catalogue match by rule catalogue_ambiguous (weak confidence); candidates: tool:9, mcp_server:3"
    );
    // The inferred-link tooltip names the catalogue match too.
    fireEvent.mouseOut(badge);
    fireEvent.mouseOver(screen.getByTestId("inferred-badge"));
    await waitFor(() => expect(screen.getByRole("tooltip")).toHaveTextContent("candidates: tool:9, mcp_server:3"));
  });

  // jyx.164/166: an edge without verified TLS to Studio sends no content;
  // the empty blocks say why.
  it("explains withheld content instead of showing empty blocks", () => {
    const withheld = {
      span_id: "00000000000000a1", trace_id: TRACE, kind: "llm_inference", name: "chat", status: "ok",
      start_time: at(0), end_time: at(1), response_model: "gpt-4.1", content_state: "not_captured",
      attributes: { "tyk.content.withheld": "edge_link_unverified_tls" },
    };
    renderTranscript({ list: [withheld] });
    const notices = screen.getAllByTestId("content-withheld").map((n) => n.textContent);
    expect(notices.length).toBeGreaterThan(0);
    expect(new Set(notices)).toEqual(new Set(["Content withheld: the edge's connection to Studio is not verified TLS"]));
  });

  it("never shows an unknown withheld reason, which an edge could forge", () => {
    const forged = {
      span_id: "00000000000000a2", trace_id: TRACE, kind: "llm_inference", name: "chat", status: "ok",
      start_time: at(0), end_time: at(1), response_model: "gpt-4.1", content_state: "not_captured",
      attributes: { "tyk.content.withheld": "call support at evil.example" },
    };
    renderTranscript({ list: [forged] });
    const notices = screen.getAllByTestId("content-withheld").map((n) => n.textContent);
    expect(new Set(notices)).toEqual(new Set(["Content withheld"]));
    expect(screen.queryByText(/evil\.example/)).toBeNull();
  });

  it("explains the inferred link's rule in a tooltip", async () => {
    renderTranscript();
    fireEvent.mouseOver(screen.getByTestId("inferred-badge"));
    expect(await screen.findByRole("tooltip")).toHaveTextContent("Linked by rule mcp_name_args (strong confidence)");
  });

  it("shows gateway chips in plain language, denials in red", () => {
    renderTranscript();
    const chip = screen.getAllByTestId("gateway-chip").find((c) => c.textContent === "Blocked by guardrail pii-filter");
    expect(chip).toHaveClass("MuiChip-colorError");
  });

  it("shows the client-side tool and every content-state placeholder", () => {
    renderTranscript();
    expect(screen.getByTestId("client-side-badge")).toBeInTheDocument();
    expect(screen.getByText("no result observed")).toBeInTheDocument();
    const placeholders = screen.getAllByTestId("content-placeholder").map((p) => p.textContent);
    expect(placeholders).toEqual(expect.arrayContaining(["Hashed: only a fingerprint was stored", "Redacted (3 values)"]));
  });

  it("expands a sub-agent card in place, keeping the scroll position", () => {
    renderTranscript();
    const feed = screen.getByTestId("transcript");
    feed.scrollTop = 0;
    const card = screen.getByTestId("subagent-card");
    expect(card).toHaveTextContent("Sub-agent: researcher");
    expect(card).toHaveTextContent("1 LLM calls · 0 tool calls · 15 tokens");
    expect(within(card).queryByText("sub chat")).not.toBeInTheDocument();
    fireEvent.click(within(card).getByRole("button", { expanded: false }));
    expect(within(card).getByRole("button", { expanded: true })).toBeInTheDocument();
    expect(within(card).getAllByTestId("assistant-step")).toHaveLength(1);
    expect(feed.scrollTop).toBe(0);
  });

  it("expands every sub-agent with w (expand all)", async () => {
    renderTranscript();
    ctx.dispatch({ type: "toggleExpandAll" });
    await waitFor(() => expect(within(screen.getByTestId("subagent-card")).getAllByTestId("assistant-step")).toHaveLength(1));
  });

  it("selects and opens a span when its card is clicked", () => {
    renderTranscript();
    fireEvent.click(screen.getAllByTestId("tool-card")[0]);
    expect(ctx.selectedId).toBe(search.span_id);
    expect(ctx.drawerSpanId).toBe(search.span_id);
  });

  it("tells a viewer without content access that only structure is shown, with placeholders", () => {
    const stripped = spans.map((s) => ({ ...s, parts: (s.parts || []).map(({ content, ...p }) => p) }));
    renderTranscript({ list: stripped, d: detail({ can_read_content: false }), previews: null });
    expect(screen.getByText("Your role shows the structure of this session but not its content.")).toBeInTheDocument();
    expect(screen.queryByText(/Please refund/)).not.toBeInTheDocument();
    expect(screen.getByTestId("user-message")).toHaveTextContent("Captured (40 B)");
  });

  it("says when previews were denied or the audit log was unavailable", () => {
    const { unmount } = renderTranscript({ previews: "denied" });
    expect(screen.getByText(/Content previews are not available/)).toBeInTheDocument();
    unmount();
    renderTranscript({ previews: "audit_unavailable" });
    expect(screen.getByText(/audit log could not be written/)).toBeInTheDocument();
  });

  it("keeps the DOM bounded for 5,000 spans", () => {
    const many = [];
    for (let i = 0; i < 5000; i += 1) {
      many.push({
        span_id: String(i).padStart(16, "0"), trace_id: TRACE, kind: i % 2 ? "tool_call" : "llm_inference",
        parent_span_id: i % 2 ? String(i - 1).padStart(16, "0") : undefined, name: `span ${i}`, tool_name: "t",
        status: "ok", start_time: at(i), end_time: at(i + 0.5), content_state: "metadata",
      });
    }
    renderTranscript({ list: many, d: { ...detail(), agents: [] } });
    const feed = screen.getByTestId("transcript");
    expect(screen.getAllByTestId("transcript-row").length).toBeLessThan(80);
    // eslint-disable-next-line testing-library/no-node-access
    expect(feed.querySelectorAll("*").length).toBeLessThan(5000);
  });

  it("explains once, with how to enable it, when the session captured no content", () => {
    const bare = [
      { ...llm2, parts: [{ part: "request", content_state: "metadata", media_type: "application/json", original_bytes: 2048 }] },
      { ...client, parts: [] },
    ];
    render(
      <MemoryRouter>
        <ThemeProvider theme={testTheme}>
          <SessionDetailProvider detail={{ ...detail(), session: { ...detail().session, content_mode: "metadata" }, agents: [] }} spans={bare} resources={[]} previews="included">
            <TranscriptView height={4000} />
          </SessionDetailProvider>
        </ThemeProvider>
      </MemoryRouter>,
    );
    const notice = screen.getByTestId("content-mode-notice");
    expect(notice).toHaveTextContent("content mode: metadata");
    expect(notice).toHaveTextContent("AGENT_TRACING_CONTENT_CEILING");
    expect(within(notice).getByRole("link", { name: "App's Tracing settings" })).toHaveAttribute("href", "/admin/apps/7");
    // Rows no longer repeat "Content not captured (mode: …)".
    expect(screen.queryByText(/Content not captured/)).not.toBeInTheDocument();
    expect(screen.getAllByText("2.0 KB, not captured").length).toBeGreaterThan(0);
  });
});
