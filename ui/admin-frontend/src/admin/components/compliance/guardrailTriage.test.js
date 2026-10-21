import React from "react";
import { render, screen } from "@testing-library/react";
import "@testing-library/jest-dom";
import { eventGuide } from "./guardrailTriage";
import EventGuide from "./EventGuide";

describe("eventGuide", () => {
  it("explains an oversize tool result and names the setting to raise", () => {
    const g = eventGuide(
      "guardrail.unscanned",
      {
        reason: "size",
        action: "block",
        limit: 262144,
        max_chars: 321097,
        reason_code: "guardrail_too_large_to_scan",
        segments: [{ kind: "tool_result", role: "tool", tool: "fetch_results", chars: 321097 }],
      },
      "Wolf Defender: stop conversation"
    );
    expect(g.code).toBe("guardrail_too_large_to_scan");
    expect(g.what).toContain("a tool result from fetch_results (321,097 characters)");
    expect(g.what).toContain("262,144 characters");
    expect(g.what).toContain("refused (HTTP 400)");
    expect(g.fix[0]).toContain("budget.max_scan_chars");
    expect(g.fix[0]).toContain('"Wolf Defender: stop conversation"');
    expect(g.fix[1]).toContain("does not help");
  });

  it("explains a detection and how to lift a Wolf Defender false flag", () => {
    const g = eventGuide("guardrail.tool_result.critical", {
      action: "block",
      category: "injection",
      provider: "http",
      max_score: 1,
      segments: [{ kind: "tool_result", tool: "read", sha256: "ab" }],
    });
    expect(g.code).toBe("guardrail_detected");
    expect(g.what).toContain("a likely prompt injection in a tool result from read (score 1)");
    expect(g.fix.join(" ")).toContain("/v1/admin/allow");
  });

  it("explains a removed tool definition without a block code", () => {
    const g = eventGuide("guardrail.tool_definition.block", {
      action: "remove",
      category: "injection",
      segments: [{ kind: "tool_definition", role: "system", tool: "refine-core_run_next" }],
    });
    expect(g.code).toBe("");
    expect(g.what).toContain("the definition of the tool refine-core_run_next");
    expect(g.fix[0]).toMatch(/^The tool refine-core_run_next was dropped/);
  });

  it("explains a provider failure by fail mode, and marks shadow events", () => {
    const closed = eventGuide("guardrail.error", { blocked: true, fail_mode: "closed", error_class: "timeout" });
    expect(closed.code).toBe("guardrail_unavailable");
    expect(closed.what).toContain("refused");
    const shadow = eventGuide("guardrail.tool_result.review", { action: "log", shadow: true, category: "injection" });
    expect(shadow.what).toContain("shadow");
  });

  it("ignores events that are not guardrail events, and tolerates missing metadata", () => {
    expect(eventGuide("pii_redacted", {})).toBeNull();
    expect(eventGuide("guardrail.unscanned", null).what).toContain("could not be screened");
  });

  it("renders the guide", () => {
    render(<EventGuide event={{ event_type: "guardrail.error", metadata: { blocked: false }, filter_name: "F" }} />);
    expect(screen.getByTestId("event-guide-what")).toHaveTextContent("let through unchecked");
    expect(screen.getByTestId("event-guide-fix").children.length).toBe(2);
  });
});
