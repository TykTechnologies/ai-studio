// What a guardrail compliance event says an exception would scope to, for
// "Not a threat? Soften for this app" (features/GuardrailExceptions.md).
// Mirrors the server (api reportableBlock / eventChannel and scripting
// channelOf): only a detector's verdict on an app's request, from a filter
// the event names by id. The server checks the event again when the
// exception is created (filter_id and filter_name must match the filter).

const NOT_SOFTENABLE = new Set([
  "guardrail.error",
  "guardrail.unscanned",
  "guardrail.unreadable_request",
  "guardrail.unredactable",
  "guardrail.template_mimicry",
  "guardrail.judge",
  "guardrail.exception_ignored",
]);

const RESPONSE_SCOPES = new Set(["proxy_response", "chat_response"]);

const channelOf = (kind, role) => {
  if (kind === "tool_result" || role === "tool") return "tool_result";
  if (kind === "tool_call") return "tool_call";
  if (kind === "tool_definition") return "tool_definition";
  if (role === "system") return "text_system";
  if (role === "user" || role === "assistant") return "text_user";
  return "";
};

// softenTarget is {filterId, filterName, appId, appName, detector, channel,
// toolName, eventId}, or null when the event cannot be softened.
export const softenTarget = (event) => {
  if (!event || !event.app_id) return null;
  const type = event.event_type || "";
  if (!type.startsWith("guardrail.") || NOT_SOFTENABLE.has(type) || RESPONSE_SCOPES.has(event.filter_scope)) return null;
  const m = event.metadata || {};
  const filterId = Number(m.filter_id);
  if (!Number.isInteger(filterId) || filterId <= 0) return null;
  const detector = typeof m.detector === "string" ? m.detector : "";
  if (!detector) return null;
  const seg = Array.isArray(m.segments) && m.segments[0] && typeof m.segments[0] === "object" ? m.segments[0] : null;
  let channel = seg ? channelOf(seg.kind, seg.role) : "";
  if (!channel) {
    const kinds = Array.isArray(m.kinds) ? m.kinds : [];
    const roles = Array.isArray(m.roles) ? m.roles : [];
    channel = channelOf(kinds.length === 1 ? kinds[0] : "", roles.length === 1 ? roles[0] : "");
  }
  if (!channel) return null;
  let toolName = "";
  if (["tool_result", "tool_call", "tool_definition"].includes(channel)) {
    if (seg && typeof seg.tool === "string") toolName = seg.tool;
    else if (Array.isArray(m.tool_names) && m.tool_names.length === 1) toolName = m.tool_names[0];
  }
  return {
    eventId: event.id,
    appId: event.app_id,
    appName: event.app_name || `App ${event.app_id}`,
    filterId,
    filterName: event.filter_name || `Filter ${filterId}`,
    detector,
    channel,
    toolName,
  };
};

// exceptionsOf lists the exceptions that softened an event: the ids in
// metadata.exceptions, with their from/to from the segments.
export const exceptionsOf = (metadata) => {
  const ids = Array.isArray(metadata?.exceptions) ? metadata.exceptions.filter((x) => Number.isInteger(x)) : [];
  return ids;
};
