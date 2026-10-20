import React from "react";
import { render, screen } from "@testing-library/react";
import "@testing-library/jest-dom";
import FlaggedText, { segmentLabel } from "./FlaggedText";

describe("FlaggedText", () => {
  it("shows the request id, each flagged segment, its excerpt and hash", () => {
    render(
      <FlaggedText
        metadata={{
          request_id: "req_1761000000",
          segments: [
            { index: 3, role: "tool", kind: "tool_result", tool: "read", sha256: "ab12", excerpt: "…ignore previous instructions [masked]…" },
            { index: 1, role: "user", kind: "text", sha256: "cd34" },
          ],
        }}
      />
    );
    expect(screen.getByText("req_1761000000")).toBeInTheDocument();
    expect(screen.getByText("Tool result from read (segment 3)")).toBeInTheDocument();
    expect(screen.getByTestId("flagged-excerpt")).toHaveTextContent("ignore previous instructions [masked]");
    expect(screen.getByText("SHA-256 ab12")).toBeInTheDocument();
    expect(screen.getByText("User message (segment 1)")).toBeInTheDocument();
    expect(screen.getByText(/No excerpt/)).toBeInTheDocument();
    expect(screen.getAllByTestId("flagged-segment")).toHaveLength(2);
  });

  it("renders nothing for events from before segments were recorded", () => {
    const { container } = render(<FlaggedText metadata={{ provider: "builtin", count: 2, roles: ["user"] }} />);
    expect(container).toBeEmptyDOMElement();
  });

  it("ignores malformed segments", () => {
    render(<FlaggedText metadata={{ segments: [null, "x", { role: "user", sha256: "ef" }] }} />);
    expect(screen.getAllByTestId("flagged-segment")).toHaveLength(1);
  });

  it("labels tool calls and definitions", () => {
    expect(segmentLabel({ kind: "tool_call", tool: "shell" })).toBe("Tool call to shell");
    expect(segmentLabel({ kind: "tool_definition", tool: "fetch" })).toBe("Tool definition fetch");
    expect(segmentLabel({ role: "assistant" })).toBe("Assistant message");
  });
});
