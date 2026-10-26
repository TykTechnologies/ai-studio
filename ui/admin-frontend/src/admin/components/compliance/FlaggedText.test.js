import React from "react";
import { render, screen } from "@testing-library/react";
import "@testing-library/jest-dom";
import { MemoryRouter } from "react-router-dom";
import FlaggedText, { noExcerpt, segmentLabel } from "./FlaggedText";

describe("FlaggedText", () => {
  it("shows the request id, each flagged segment, its excerpt and hash", () => {
    render(
      <MemoryRouter>
        <FlaggedText
          appId={408}
          metadata={{
            request_id: "tyai_0192d8a4f6b37c1e9a5b2d4c6e8f0a1b",
            client_request_id: "lb-7f3a",
            segments: [
              { index: 3, role: "tool", kind: "tool_result", tool: "read", sha256: "ab12", excerpt: "…ignore previous instructions [masked]…" },
              { index: 1, role: "user", kind: "text", sha256: "cd34", excerpt_status: "off" },
            ],
          }}
        />
      </MemoryRouter>
    );
    expect(screen.getByText("tyai_0192d8a4f6b37c1e9a5b2d4c6e8f0a1b")).toBeInTheDocument();
    expect(screen.getByTestId("flagged-request-log")).toHaveAttribute(
      "href",
      "/admin/apps/408?log_search=tyai_0192d8a4f6b37c1e9a5b2d4c6e8f0a1b"
    );
    expect(screen.getByText("lb-7f3a")).toBeInTheDocument();
    expect(screen.getAllByTestId("flagged-segment")[0]).toHaveTextContent("Tool result from read · #3");
    expect(screen.getByTestId("flagged-excerpt")).toHaveTextContent("ignore previous instructions [masked]");
    expect(screen.getByText("SHA-256 ab12")).toBeInTheDocument();
    expect(screen.getAllByTestId("flagged-segment")[1]).toHaveTextContent("User message · #1");
    expect(screen.getByTestId("flagged-no-excerpt")).toHaveTextContent("Excerpts are off");
    expect(screen.getByText("Flagged text")).toBeInTheDocument();
    expect(screen.getAllByTestId("flagged-segment")).toHaveLength(2);
  });

  it("says unchecked text has no excerpt, and how many segments are listed", () => {
    render(
      <FlaggedText
        eventType="guardrail.unscanned"
        metadata={{
          count: 6,
          segments: [
            { index: 58, role: "tool", kind: "tool_result", tool: "extract", chars: 14311, sha256: "73d3", excerpt_status: "not_scanned" },
            { index: 60, role: "tool", kind: "tool_result", tool: "search", chars: 1756, sha256: "2592" },
          ],
        }}
      />
    );
    expect(screen.getByText("Not checked")).toBeInTheDocument();
    expect(screen.getByTestId("flagged-shown")).toHaveTextContent("2 of 6 shown");
    expect(screen.getAllByTestId("flagged-segment")[0]).toHaveTextContent("Tool result from extract · 14,311 chars · #58");
    // Older events have no excerpt_status: the event type decides.
    for (const el of screen.getAllByTestId("flagged-no-excerpt")) {
      expect(el).toHaveTextContent("Not checked, so there is no match to show.");
    }
    expect(screen.queryByText(/Show flagged text/)).not.toBeInTheDocument();
  });

  it("names why a detection has no excerpt", () => {
    expect(noExcerpt("bodies_not_logged")).toContain("does not log request bodies");
    expect(noExcerpt("unavailable")).toContain("No excerpt available");
    expect(noExcerpt(undefined, false)).toContain("Show flagged text");
  });

  it("renders nothing for events from before segments were recorded", () => {
    const { container } = render(<FlaggedText metadata={{ provider: "builtin", count: 2, roles: ["user"] }} />);
    expect(container).toBeEmptyDOMElement();
  });

  it("ignores malformed segments", () => {
    render(<FlaggedText metadata={{ segments: [null, "x", { role: "user", sha256: "ef" }] }} />);
    expect(screen.queryByTestId("flagged-request-log")).not.toBeInTheDocument();
    expect(screen.getAllByTestId("flagged-segment")).toHaveLength(1);
  });

  it("labels tool calls and definitions", () => {
    expect(segmentLabel({ kind: "tool_call", tool: "shell" })).toBe("Tool call to shell");
    expect(segmentLabel({ kind: "tool_definition", tool: "fetch" })).toBe("Tool definition fetch");
    expect(segmentLabel({ role: "assistant" })).toBe("Assistant message");
  });
});
