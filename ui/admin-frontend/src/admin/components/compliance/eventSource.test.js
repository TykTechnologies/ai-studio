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

  it("returns nothing for metadata without a source, or of the wrong shape", () => {
    expect(eventSourceLabel({})).toBe("");
    expect(eventSourceLabel(undefined)).toBe("");
    expect(eventSourceLabel({ roles: "user", tool_names: [42] })).toBe("");
  });
});
