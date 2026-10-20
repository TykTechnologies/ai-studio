// A small hand-built session in the agent-trace JSON v1 shape, shared by the
// session detail tests: two LLM steps, a tool call, a gateway denial, an
// error and a client-side tool, with parts in several content states.

export const T0 = Date.parse("2026-10-20T10:00:00Z");
const at = (s) => new Date(T0 + s * 1000).toISOString();
export const SESSION_ID = "0192a4c8-7f00-7000-8000-000000000001";
const TRACE = "0123456789abcdef0123456789abcdef";

export const ids = {
  llm1: "0000000000000001",
  tool1: "0000000000000002",
  deny: "0000000000000003",
  llm2: "0000000000000004",
  client: "0000000000000005",
};

export const spans = [
  {
    span_id: ids.llm1, trace_id: TRACE, kind: "llm_inference", name: "chat gpt-4.1", status: "ok",
    start_time: at(0), end_time: at(1), request_model: "gpt-4.1", resource_type: "llm", resource_id: "5",
    inferred: false, content_state: "full",
    usage: { input_tokens: 100, output_tokens: 20, cost_micros: 1500 },
    parts: [
      { part: "request", content_state: "full", media_type: "application/json", original_bytes: 900 },
      { part: "response", content_state: "redacted", media_type: "application/json", original_bytes: 300, redaction_count: 2 },
    ],
  },
  {
    span_id: ids.tool1, trace_id: TRACE, parent_span_id: ids.llm1, parent_confidence: "strong", parent_rule: "mcp_name_args",
    kind: "tool_call", name: "tools/call web_search", tool_name: "web_search", status: "ok",
    start_time: at(1.2), end_time: at(2), inferred: false, content_state: "hashed",
    parts: [{ part: "tool_args", content_state: "hashed", media_type: "application/json", original_bytes: 40 }],
    links: [{ type: "caused_by", trace_id: TRACE, span_id: ids.llm1, confidence: "exact", rule: "tool_call_id" }],
  },
  {
    span_id: ids.deny, trace_id: TRACE, parent_span_id: ids.llm1, kind: "gateway_event", name: "guardrail pii-filter",
    status: "ok", start_time: at(1.1), end_time: at(1.1), inferred: false, content_state: "metadata",
    gateway_event: { type: "guardrail", outcome: "denied", name: "pii-filter" },
  },
  {
    span_id: ids.llm2, trace_id: TRACE, kind: "llm_inference", name: "chat gpt-4.1", status: "error", error_type: "upstream_503",
    start_time: at(3), end_time: at(4), inferred: false, content_state: "metadata",
    parts: [{ part: "request", content_state: "metadata", media_type: "application/json", original_bytes: 1200 }],
  },
  {
    span_id: ids.client, trace_id: TRACE, kind: "inferred_client_tool", name: "read_file", status: "ok",
    start_time: at(4.5), end_time: at(6), inferred: true, content_state: "not_captured",
  },
];

export const detail = (viewer = { can_read_content: true, can_export: true, can_purge: false }) => ({
  session: {
    id: SESSION_ID, app_id: 7, app_name: "Support bot", source: "declared", name: "refund flow", status: "closed",
    started_at: at(0), last_activity_at: at(6), ended_at: at(6), duration_ms: 6000, gateway_event_count: 1,
    models: ["gpt-4.1"], tool_names: ["web_search"], agent_names: [], content_mode: "full",
    confidence_counts: { declared: 0, exact: 1, strong: 1, weak: 0 },
    totals: { span_count: 5, llm_call_count: 2, tool_call_count: 1, error_count: 1, input_tokens: 100, output_tokens: 20, cost_micros: 1500, currency: "USD" },
  },
  agents: [],
  traces: [{ trace_id: TRACE, turn_index: 0, source: "tyk", started_at: at(0), status: "error", totals: {} }],
  viewer,
});
