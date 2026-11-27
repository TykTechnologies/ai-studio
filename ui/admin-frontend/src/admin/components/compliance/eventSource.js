// Where a guardrail compliance event's flagged text came from, read from the
// event metadata (scripting/filter_runner.go). segments, when present, name
// each flagged segment's kind, role and tool, which says exactly where each
// finding was: a tool definition reaches the provider as system text, but is
// not the system prompt (o4a.3). Otherwise: tool_names lists the tools of the
// flagged tool segments (sanitised server-side), kinds is "text",
// "tool_result", "tool_definition" and/or "tool_call", roles the message
// roles. Events from before tool results were scanned carry roles only.
// Returns [] when the metadata names nothing.
const ROLE_LABELS = {
  user: "User message",
  system: "System prompt",
  assistant: "Assistant message",
};

const asList = (v) => (Array.isArray(v) ? v.filter((x) => typeof x === "string" && x) : []);
const uniq = (xs) => [...new Set(xs)];

const named = (label, tools, sep) => (tools.length ? `${label}${sep}${tools.join(", ")}` : label);

// fromSegments labels the sources of the flagged segments, text by role and
// the rest by kind; kinds the capped segment list missed get a plain label.
const fromSegments = (segs, kinds) => {
  const textRoles = [];
  const tools = { tool_definition: [], tool_call: [], tool_result: [] };
  const seen = new Set();
  segs.forEach((g) => {
    const kind = typeof g.kind === "string" && g.kind ? g.kind : "text";
    seen.add(kind);
    if (kind === "text") {
      if (typeof g.role === "string" && g.role) textRoles.push(g.role);
    } else if (tools[kind] && typeof g.tool === "string" && g.tool) {
      tools[kind].push(g.tool);
    }
  });
  kinds.forEach((k) => seen.add(k));
  const sources = uniq(textRoles).map((r) => ROLE_LABELS[r] || `${r} message`);
  if (seen.has("text") && textRoles.length === 0) sources.push("Message text");
  if (seen.has("tool_definition")) sources.push(named("Tool definition", uniq(tools.tool_definition), ": "));
  if (seen.has("tool_call")) sources.push(named("Tool call", uniq(tools.tool_call), " to "));
  if (seen.has("tool_result")) sources.push(named("Tool result", uniq(tools.tool_result), " from "));
  return sources;
};

export const eventSources = (metadata) => {
  if (!metadata || typeof metadata !== "object") return [];
  const tools = asList(metadata.tool_names);
  const kinds = asList(metadata.kinds);
  const roles = asList(metadata.roles);
  const segs = Array.isArray(metadata.segments) ? metadata.segments.filter((g) => g && typeof g === "object") : [];
  if (segs.length > 0) return fromSegments(segs, kinds);

  const sources = [];
  // Roles name the non-tool text; "tool" is the tool results themselves.
  // Where kinds are recorded, roles only stand for text: a tool segment's
  // role (system for a definition) says nothing about the prompt.
  const textRoles = kinds.length === 0 || kinds.includes("text") ? roles.filter((r) => r !== "tool") : [];
  textRoles.forEach((r) => sources.push(ROLE_LABELS[r] || `${r} message`));
  if (textRoles.length === 0 && kinds.includes("text")) sources.push("Message text");

  const toolKinds = ["tool_definition", "tool_call", "tool_result"].filter((k) => kinds.includes(k));
  // Tool names belong to a kind only when one tool kind was flagged.
  const own = toolKinds.length === 1 ? tools : [];
  if (kinds.includes("tool_definition")) sources.push(named("Tool definition", own, ": "));
  if (kinds.includes("tool_call")) sources.push(named("Tool call", own, " to "));
  if (kinds.includes("tool_result") || (kinds.length === 0 && roles.includes("tool"))) {
    // The names are the results' when results are the only tool kind, or when
    // the event predates kinds (toolKinds is then empty: they were all results).
    sources.push(named("Tool result", toolKinds.length <= 1 ? tools : [], " from "));
  } else if (toolKinds.length === 0 && tools.length > 0) {
    sources.push(named("Tool result", tools, " from "));
  }
  return sources;
};

export const eventSourceLabel = (metadata) => eventSources(metadata).join("; ");
