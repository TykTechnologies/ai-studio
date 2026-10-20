// The session detail's span model: ordering, the keyboard navigation lists
// (spec §6.2 "Navigation and search"), time bounds and range filtering, and
// the content-state placeholders. Spans are agent-trace JSON v1 span
// objects (pkg/agenttrace/traceschema/agent-trace-v1.json).

export const TOOL_KINDS = new Set(["tool_call", "inferred_client_tool", "datasource_call"]);
const PROBLEM_OUTCOMES = new Set(["denied", "exceeded"]);

export const ms = (t) => (t ? new Date(t).getTime() : NaN);

/** Start/end of a span in epoch ms; an open span ends where it started. */
export const spanTimes = (s) => {
  const start = ms(s.start_time);
  const end = s.end_time ? ms(s.end_time) : start;
  return { start, end: Math.max(start, end) };
};

export const durationMs = (s) => {
  const { start, end } = spanTimes(s);
  return end - start;
};

export const attrsOf = (s) => (s && s.attributes && typeof s.attributes === "object" ? s.attributes : {});

/** The gateway event outcome (allowed, denied, modified, ...), if any. */
export const eventOutcome = (s) => attrsOf(s)["tyk.event.outcome"] || "";

export const isProblem = (s) =>
  s.status === "error" || (s.kind === "gateway_event" && PROBLEM_OUTCOMES.has(eventOutcome(s)));

const byStart = (a, b) => {
  const d = ms(a.start_time) - ms(b.start_time);
  if (d) return d;
  return a.span_id < b.span_id ? -1 : a.span_id > b.span_id ? 1 : 0;
};

/** Spans in (start_time, span_id) order, the API's keyset order. */
export const sortSpans = (spans) => [...spans].sort(byStart);

export const inRange = (s, range) => {
  if (!range) return true;
  const { start, end } = spanTimes(s);
  return end >= range.from && start <= range.to;
};

/**
 * Indexes spans for the views and the keyboard: by id, in order, and the
 * navigation lists (steps, tool calls, problems, gateway events).
 */
export const indexSpans = (spans, range) => {
  const ordered = sortSpans(spans).filter((s) => inRange(s, range));
  const byId = new Map(ordered.map((s, i) => [s.span_id, { span: s, index: i }]));
  return {
    ordered,
    byId,
    steps: ordered.filter((s) => s.kind === "llm_inference").map((s) => s.span_id),
    toolCalls: ordered.filter((s) => TOOL_KINDS.has(s.kind)).map((s) => s.span_id),
    problems: ordered.filter(isProblem).map((s) => s.span_id),
    gatewayEvents: ordered.filter((s) => s.kind === "gateway_event").map((s) => s.span_id),
  };
};

/**
 * The next (dir 1) or previous (dir -1) id of list after the current span,
 * in time order. With nothing selected it starts at the list's first (or
 * last) entry; past either end it stays put (returns null).
 */
export const nextInList = (index, list, currentId, dir) => {
  if (!list.length) return null;
  const cur = currentId != null ? index.byId.get(currentId) : null;
  if (!cur) return dir > 0 ? list[0] : list[list.length - 1];
  const pos = cur.index;
  const ordered = dir > 0 ? list : [...list].reverse();
  for (const id of ordered) {
    const at = index.byId.get(id)?.index;
    if (at === undefined) continue;
    if ((dir > 0 && at > pos) || (dir < 0 && at < pos)) return id;
  }
  return null;
};

/** The session's time bounds from its header and spans (epoch ms). */
export const sessionBounds = (session, spans) => {
  let from = ms(session?.started_at);
  let to = ms(session?.ended_at || session?.last_activity_at);
  for (const s of spans) {
    const t = spanTimes(s);
    if (!(from <= t.start)) from = t.start;
    if (!(to >= t.end)) to = t.end;
  }
  if (!Number.isFinite(from)) from = Date.now();
  if (!Number.isFinite(to) || to <= from) to = from + 1;
  return { from, to };
};

/** Splits text on [REDACTED:TYPE] markers for inline chips. */
export const redactionSegments = (text) => {
  const out = [];
  const re = /\[REDACTED:([A-Z0-9_]+)\]/g;
  let last = 0;
  let m;
  const s = String(text ?? "");
  while ((m = re.exec(s))) {
    if (m.index > last) out.push({ text: s.slice(last, m.index) });
    out.push({ redacted: m[1] });
    last = m.index + m[0].length;
  }
  if (last < s.length) out.push({ text: s.slice(last) });
  return out;
};

const isTextual = (mediaType) =>
  !mediaType || mediaType.startsWith("text/") || mediaType.includes("json") || mediaType.includes("xml");

export const formatBytes = (n) => {
  const b = Number(n) || 0;
  if (b < 1024) return `${b} B`;
  if (b < 1024 * 1024) return `${(b / 1024).toFixed(1)} KB`;
  return `${(b / 1024 / 1024).toFixed(1)} MB`;
};

/**
 * What to show for a captured part before (or instead of) its content
 * (spec §6.2 content-state placeholders). loadable says whether the content
 * can be fetched at all, for a viewer allowed to read content.
 */
export const partPlaceholder = (part) => {
  const state = part?.content_state || "not_captured";
  if (!isTextual(part?.media_type)) {
    return { text: `Binary ${part.media_type} ${formatBytes(part.original_bytes)} (not stored)`, loadable: false };
  }
  switch (state) {
    case "not_captured":
    case "off":
      return { text: "Content not captured (mode: off)", loadable: false };
    case "metadata":
      return { text: `Content not captured (mode: metadata, ${formatBytes(part.original_bytes)})`, loadable: false };
    case "hashed":
      return { text: "Hashed: only a fingerprint was stored", loadable: false };
    case "dropped":
      return { text: "Dropped: the gateway was under load and kept no content", loadable: false };
    case "redacted":
      return {
        text: `Redacted (${part.redaction_count || 0} value${part.redaction_count === 1 ? "" : "s"})`,
        loadable: true,
      };
    case "truncated":
      return { text: `Truncated (stored part of ${formatBytes(part.original_bytes)})`, loadable: true };
    case "full":
      return { text: `Captured (${formatBytes(part.original_bytes)})`, loadable: true };
    default:
      return { text: `Content state: ${state}`, loadable: false };
  }
};

/** Parts of a span shown on the Input and Output tabs, in display order. */
export const INPUT_PARTS = ["system_instructions", "tool_definitions", "request", "tool_args"];
export const OUTPUT_PARTS = ["response", "tool_result", "error_body"];

export const PART_LABELS = {
  system_instructions: "System instructions",
  tool_definitions: "Tool definitions",
  request: "Request",
  tool_args: "Tool arguments",
  response: "Response",
  tool_result: "Tool result",
  error_body: "Error body",
  user_message: "User message",
};

export const KIND_LABELS = {
  llm_inference: "LLM call",
  tool_call: "Tool call",
  datasource_call: "Datasource call",
  agent: "Agent",
  gateway_event: "Gateway event",
  inferred_client_tool: "Client-side tool (inferred)",
  client_span: "Client span",
};

export const resourceName = (resources, s) => {
  if (!s?.resource_type || !s?.resource_id) return "";
  const r = (resources || []).find((x) => x.type === s.resource_type && x.id === s.resource_id);
  return r ? r.name : `${s.resource_type} ${s.resource_id}`;
};
