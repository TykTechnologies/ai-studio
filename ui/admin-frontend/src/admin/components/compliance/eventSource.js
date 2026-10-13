// Where a guardrail compliance event's flagged text came from, read from the
// event metadata (scripting/filter_runner.go): tool_names lists the tools
// whose results were flagged (sanitised server-side), kinds is "text" and/or
// "tool_result", roles the message roles. Events from before tool results
// were scanned carry roles only. Returns "" when the metadata names nothing.
const ROLE_LABELS = {
  user: "User message",
  system: "System prompt",
  assistant: "Assistant message",
};

const asList = (v) => (Array.isArray(v) ? v.filter((x) => typeof x === "string" && x) : []);

export const eventSources = (metadata) => {
  if (!metadata || typeof metadata !== "object") return [];
  const tools = asList(metadata.tool_names);
  const kinds = asList(metadata.kinds);
  const roles = asList(metadata.roles);
  const sources = [];

  // Roles name the non-tool text; "tool" is the tool results themselves.
  roles
    .filter((r) => r !== "tool")
    .forEach((r) => sources.push(ROLE_LABELS[r] || `${r} message`));
  if (roles.length === 0 && kinds.includes("text")) sources.push("Message text");

  if (tools.length > 0) {
    sources.push(`Tool result from ${tools.join(", ")}`);
  } else if (kinds.includes("tool_result") || roles.includes("tool")) {
    sources.push("Tool result");
  }
  return sources;
};

export const eventSourceLabel = (metadata) => eventSources(metadata).join("; ");
