import { TOOL_KINDS, durationMs, gatewayEventOf, ms, spanTimes } from "./spanModel";
import { formatDuration } from "./sessionFilters";

// The Transcript ("Trajectory") view model (spec §6.2 View 1): a session's
// spans turned into reading rows — turn dividers, the user message, one row
// per assistant step with its tool calls (parallel ones grouped) and gateway
// events, and declared sub-agents as collapsible cards. Text comes from the
// parts' inline previews when the viewer may read content; otherwise the
// rows carry metadata and content-state placeholders only.

/** A part's inline preview text, if the API included one. */
export const partText = (span, name) => {
  const p = (span?.parts || []).find((x) => x.part === name);
  return p && typeof p.content === "string" ? p.content : null;
};

export const partOf = (span, name) => (span?.parts || []).find((x) => x.part === name) || null;

const parseJSON = (s) => {
  try {
    return { ok: true, value: JSON.parse(s) };
  } catch (e) {
    return { ok: false };
  }
};

const clip = (s, n) => (s.length > n ? `${s.slice(0, n - 1)}…` : s);

/**
 * The one-line argument preview: top-level string and number values with
 * their keys, at most 80 characters (spec §6.2).
 */
export const argPreview = (text) => {
  if (text == null) return "";
  const j = parseJSON(text);
  if (!j.ok || !j.value || typeof j.value !== "object" || Array.isArray(j.value)) return clip(String(text).replace(/\s+/g, " "), 80);
  const bits = Object.entries(j.value)
    .filter(([, v]) => typeof v === "string" || typeof v === "number" || typeof v === "boolean")
    .map(([k, v]) => `${k}: ${typeof v === "string" ? JSON.stringify(v) : v}`);
  return clip(bits.join(", "), 80);
};

const sizeLabel = (n) => {
  const b = Number(n) || 0;
  if (b < 1024) return `${b} B`;
  if (b < 1024 * 1024) return `${(b / 1024).toFixed(1)} KB`;
  return `${(b / 1024 / 1024).toFixed(1)} MB`;
};

/**
 * The result preview: the first 200 characters of text, or a structural
 * summary of JSON ("JSON object, 14 keys, 2.3 KB").
 */
export const resultPreview = (text, originalBytes) => {
  if (text == null) return "";
  const j = parseJSON(text);
  const size = sizeLabel(originalBytes || text.length);
  if (j.ok && Array.isArray(j.value)) return `JSON array, ${j.value.length} item${j.value.length === 1 ? "" : "s"}, ${size}`;
  if (j.ok && j.value && typeof j.value === "object") {
    const n = Object.keys(j.value).length;
    return `JSON object, ${n} key${n === 1 ? "" : "s"}, ${size}`;
  }
  return clip(String(text), 200);
};

/** Link style for a parent edge's confidence (spec §3.3). */
export const confidenceStyle = (confidence) => {
  switch (confidence) {
    case "strong":
      return { border: "dashed", badge: "inferred" };
    case "weak":
      return { border: "dotted", badge: "inferred (weak)" };
    default:
      return { border: "solid", badge: null };
  }
};

const DENIED = new Set(["denied", "exceeded"]);
const MODIFIED = new Set(["modified", "redacted", "warned"]);

/**
 * Plain-language text and severity of a gateway event chip (spec §6.2),
 * from span.gateway_event. Unknown types and outcomes render neutrally.
 */
export const gatewayChip = (s) => {
  const g = gatewayEventOf(s);
  const outcome = g.outcome || "";
  const name = g.name || "";
  let text;
  switch (g.type) {
    case "guardrail":
      text = outcome === "denied" ? `Blocked by guardrail ${name}` : `Guardrail ${name} ${outcome || "checked"}`;
      break;
    case "fallback":
      text = `Fallback: ${g.from_model || "?"} → ${g.to_model || "?"}${g.reason ? ` after ${g.reason}` : ""}`;
      break;
    case "retry":
    case "attempt":
      text = `${g.type === "retry" ? "Retry" : "Attempt"} ${g.attempt_number ?? "?"}${g.attempt_total ? `/${g.attempt_total}` : ""}${g.provider ? ` (${g.provider})` : ""}${g.reason ? ` after ${g.reason}` : ""}`;
      break;
    case "budget":
      text = `Budget ${g.budget_id || ""}${g.budget_used_pct != null ? ` at ${Math.round(g.budget_used_pct)}%` : ""}${outcome === "exceeded" ? " exceeded" : ""}`;
      break;
    case "rate_limit":
      text = `Rate limited${g.wait_ms != null ? ` — ${formatDuration(g.wait_ms)} wait` : ""}`;
      break;
    case "policy":
      text = `Policy ${g.policy_id || name} ${outcome}`;
      break;
    default:
      text = [g.type, name, outcome].filter(Boolean).join(" ") || s.name;
  }
  return {
    text: text.replace(/\s+/g, " ").trim(),
    severity: DENIED.has(outcome) ? "error" : MODIFIED.has(outcome) ? "warning" : "default",
  };
};

/** Groups tool spans whose times overlap ("N parallel calls"). */
export const groupParallel = (tools) => {
  const groups = [];
  let cur = null;
  let curEnd = -Infinity;
  for (const t of tools) {
    const { start, end } = spanTimes(t);
    if (cur && start < curEnd) {
      cur.push(t);
      curEnd = Math.max(curEnd, end);
    } else {
      cur = [t];
      groups.push(cur);
      curEnd = end;
    }
  }
  return groups;
};

// The row builder for one agent's spans, shared by the root transcript and
// the (recursive) sub-agent cards.
const makeBuilder = (spans, detail) => {
  const agents = detail?.agents || [];
  const subAgentIds = new Set(agents.filter((a) => a.parent_agent_id).map((a) => a.id));
  const turnOf = new Map((detail?.traces || []).map((t) => [t.trace_id, t]));
  const byId = new Map(spans.map((s) => [s.span_id, s]));

  // The span that consumed each span's result: whoever lists it as caused_by.
  const consumer = new Map();
  for (const s of spans) {
    for (const l of s.links || []) {
      if (l.type === "caused_by" && !consumer.has(l.span_id)) consumer.set(l.span_id, s);
    }
  }

  const ownSpans = (agentId) => spans.filter((s) => (agentId ? s.agent_id === agentId : !subAgentIds.has(s.agent_id)));

  // traceId, when set, keeps sub-agent cards to the spans of that turn.
  const buildBody = (list, agentId, keyPrefix, traceId) => {
    const rows = [];
    const steps = list.filter((s) => s.kind === "llm_inference");
    const stepIds = new Set(steps.map((s) => s.span_id));
    const toolsOf = new Map();
    const eventsOf = new Map();
    const loose = [];
    for (const s of list) {
      if (TOOL_KINDS.has(s.kind)) {
        if (s.parent_span_id && stepIds.has(s.parent_span_id)) {
          toolsOf.set(s.parent_span_id, [...(toolsOf.get(s.parent_span_id) || []), s]);
        } else loose.push({ at: ms(s.start_time), row: { type: "tool", key: `${keyPrefix}t-${s.span_id}`, span: s } });
      } else if (s.kind === "gateway_event") {
        if (s.parent_span_id && byId.has(s.parent_span_id)) {
          eventsOf.set(s.parent_span_id, [...(eventsOf.get(s.parent_span_id) || []), s]);
        } else loose.push({ at: ms(s.start_time), row: { type: "event", key: `${keyPrefix}e-${s.span_id}`, span: s } });
      }
    }
    const items = steps.map((s) => ({
      at: ms(s.start_time),
      row: { type: "step", key: `${keyPrefix}s-${s.span_id}`, span: s },
    }));
    // Declared sub-agents of this agent appear where they were spawned.
    for (const a of agents.filter((x) => (agentId ? x.parent_agent_id === agentId : x.parent_agent_id && !subAgentIds.has(x.parent_agent_id)))) {
      const sub = ownSpans(a.id).filter((s) => !traceId || s.trace_id === traceId);
      if (!sub.length) continue;
      const spawn = a.spawned_by_span_id && byId.get(a.spawned_by_span_id);
      items.push({
        at: spawn ? spanTimes(spawn).end : ms(sub[0].start_time),
        row: { type: "subagent", key: `${keyPrefix}a-${a.id}`, agent: a, spans: sub },
      });
    }
    items.push(...loose);
    items.sort((x, y) => x.at - y.at);
    for (const { row } of items) {
      if (row.type === "step") {
        const tools = (toolsOf.get(row.span.span_id) || []).sort((a, b) => ms(a.start_time) - ms(b.start_time));
        row.toolGroups = groupParallel(tools).map((g) =>
          g.map((t) => ({ span: t, events: eventsOf.get(t.span_id) || [], next: consumer.get(t.span_id) || null })),
        );
        row.events = eventsOf.get(row.span.span_id) || [];
      }
      if (row.type === "tool") {
        row.events = eventsOf.get(row.span.span_id) || [];
        row.next = consumer.get(row.span.span_id) || null;
      }
      rows.push(row);
    }
    return rows;
  };

  return { agents, subAgentIds, turnOf, ownSpans, buildBody };
};

/** The rows of a declared sub-agent's card. */
export const buildSubAgentRows = (spans, detail, agentId) => {
  const b = makeBuilder(spans, detail);
  return b.buildBody(b.ownSpans(agentId), agentId, `${agentId}-`);
};

/**
 * Builds the transcript rows. spans are the (range-filtered) spans in time
 * order; detail is the session detail (agents, traces).
 */
export const buildTranscript = (spans, detail) => {
  const { subAgentIds, turnOf, ownSpans, buildBody } = makeBuilder(spans, detail);

  // Root rows, turn by turn.
  const root = ownSpans(null);
  const turns = new Map();
  for (const s of spans) {
    if (!turns.has(s.trace_id)) turns.set(s.trace_id, []);
  }
  const rows = [];
  const orderedTurns = [...turns.keys()].sort((a, b) => {
    const ta = turnOf.get(a)?.turn_index ?? Infinity;
    const tb = turnOf.get(b)?.turn_index ?? Infinity;
    return ta - tb || ms(spans.find((s) => s.trace_id === a)?.start_time) - ms(spans.find((s) => s.trace_id === b)?.start_time);
  });
  orderedTurns.forEach((traceId, i) => {
    const tr = turnOf.get(traceId);
    const turnSpans = root.filter((s) => s.trace_id === traceId);
    const subSpans = spans.filter((s) => s.trace_id === traceId && subAgentIds.has(s.agent_id));
    if (!turnSpans.length && !subSpans.length) return;
    const all = [...turnSpans, ...subSpans];
    const from = Math.min(...all.map((s) => spanTimes(s).start));
    const to = Math.max(...all.map((s) => spanTimes(s).end));
    rows.push({
      type: "turn",
      key: `turn-${traceId}`,
      traceId,
      index: tr ? tr.turn_index + 1 : i + 1,
      durationMs: to - from,
      costMicros: tr?.totals?.cost_micros ?? turnSpans.reduce((sum, s) => sum + (s.usage?.cost_micros || 0), 0),
    });
    const firstStep = turnSpans.find((s) => s.kind === "llm_inference");
    if (firstStep) rows.push({ type: "user", key: `user-${traceId}`, span: firstStep });
    rows.push(...buildBody(turnSpans, null, `${traceId}-`, traceId));
  });
  return rows;
};

/** For each span id, the index of the row that shows it. */
export const rowIndexOf = (rows) => {
  const out = new Map();
  rows.forEach((r, i) => {
    const put = (id) => {
      if (id && !out.has(id)) out.set(id, i);
    };
    put(r.span?.span_id);
    (r.events || []).forEach((e) => put(e.span_id));
    (r.toolGroups || []).flat().forEach((t) => {
      put(t.span.span_id);
      t.events.forEach((e) => put(e.span_id));
    });
    (r.spans || []).forEach((s) => put(s.span_id));
  });
  return out;
};

/** Rollups of a sub-agent card. */
export const agentRollup = (agent, spans) => ({
  llmCalls: spans.filter((s) => s.kind === "llm_inference").length,
  toolCalls: spans.filter((s) => TOOL_KINDS.has(s.kind)).length,
  errors: spans.filter((s) => s.status === "error").length,
  tokens: agent.total_tokens || spans.reduce((n, s) => n + (s.usage?.input_tokens || 0) + (s.usage?.output_tokens || 0), 0),
  costMicros: agent.total_cost_micros || spans.reduce((n, s) => n + (s.usage?.cost_micros || 0), 0),
  durationMs: spans.length ? Math.max(...spans.map((s) => spanTimes(s).end)) - Math.min(...spans.map((s) => spanTimes(s).start)) : 0,
});

export const stepLatency = (s) => (s.end_time ? durationMs(s) : null);
