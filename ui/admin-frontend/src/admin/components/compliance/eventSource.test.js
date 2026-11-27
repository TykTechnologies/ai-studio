import { eventSourceLabel, eventSources } from "./eventSource";

describe("eventSource", () => {
  it("names the tools whose results were flagged", () => {
    expect(eventSourceLabel({ roles: ["tool"], kinds: ["tool_result"], tool_names: ["fetch_url"] })).toBe(
      "Tool result from fetch_url"
    );
  });

  it("lists user text and tool results flagged by one detector", () => {
    expect(eventSources({ roles: ["tool", "user"], kinds: ["text", "tool_result"], tool_names: ["read_file"] })).toEqual([
      "User message",
      "Tool result from read_file",
    ]);
  });

  it("says tool result when no tool name was recorded", () => {
    expect(eventSourceLabel({ roles: ["tool"], kinds: ["tool_result"] })).toBe("Tool result");
  });

  it("reads events recorded before tool results were scanned", () => {
    expect(eventSourceLabel({ roles: ["system", "user"] })).toBe("System prompt; User message");
  });

  // o4a.3: a tool definition is sent to the provider as system text.
  it("labels a flagged tool definition by its kind, not as the system prompt", () => {
    const remove = {
      roles: ["system", "tool"],
      kinds: ["tool_definition", "tool_result"],
      tool_names: ["lookup_weather", "search_kb"],
      segments: [
        { index: 3, kind: "tool_definition", role: "system", tool: "lookup_weather" },
        { index: 7, kind: "tool_result", role: "tool", tool: "search_kb" },
      ],
    };
    expect(eventSourceLabel(remove)).toBe("Tool definition: lookup_weather; Tool result from search_kb");
    expect(eventSourceLabel({ ...remove, segments: [remove.segments[0]], kinds: ["tool_definition"] })).toBe("Tool definition: lookup_weather");
  });

  it("without segments, keeps a definition's system role out of the label", () => {
    expect(eventSourceLabel({ roles: ["system"], kinds: ["tool_definition"], tool_names: ["lookup_weather"] })).toBe(
      "Tool definition: lookup_weather"
    );
    expect(eventSourceLabel({ roles: ["system", "tool"], kinds: ["tool_definition", "tool_result"], tool_names: ["a", "b"] })).toBe(
      "Tool definition; Tool result"
    );
    expect(eventSourceLabel({ roles: ["system", "user"], kinds: ["text", "tool_definition"] })).toBe("System prompt; User message; Tool definition");
  });

  it("labels text segments by role and the kinds the capped list missed", () => {
    expect(
      eventSources({ kinds: ["text", "tool_call"], segments: [{ kind: "text", role: "user" }, { kind: "text", role: "user" }] })
    ).toEqual(["User message", "Tool call"]);
  });

  it("returns nothing for metadata without a source, or of the wrong shape", () => {
    expect(eventSourceLabel({})).toBe("");
    expect(eventSourceLabel(undefined)).toBe("");
    expect(eventSourceLabel({ roles: "user", tool_names: [42] })).toBe("");
  });
});
