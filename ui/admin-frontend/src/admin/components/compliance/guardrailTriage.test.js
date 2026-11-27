import React from "react";
import { render, screen } from "@testing-library/react";
import "@testing-library/jest-dom";
import { eventGuide } from "./guardrailTriage";
import EventGuide from "./EventGuide";

// The prod event that prompted the rewrite: Wolf Defender failed on six tool
// results in a request too large for fail open.
const prodUnscanned = {
  action: "block",
  count: 6,
  detector: "unscanned",
  kinds: ["tool_result"],
  max_chars: 14311,
  provider: "http",
  reason: "provider_error",
  reason_code: "guardrail_unavailable",
  roles: ["tool"],
  error_class: "http_502",
  fail_mode: "open",
  fail_mode_skipped: "run_chars",
  run_chars: 40112,
  single_call_chars: 32768,
  segments: [{ chars: 14311, index: 58, kind: "tool_result", role: "tool", tool: "extract", sha256: "73d3", excerpt_status: "not_scanned" }],
};

describe("eventGuide", () => {
  it("explains a provider failure that fail open did not cover", () => {
    const g = eventGuide("guardrail.unscanned", prodUnscanned, "Wolf Defender: stop conversation");
    expect(g.headline).toBe("Blocked: provider failed (HTTP 502) on 6 tool results");
    expect(g.why).toEqual([
      "Provider call failed: HTTP 502",
      'Fail mode "open" only covers requests up to 32,768 chars; this one had 40,112',
      "Unchecked text counts as flagged; this filter blocks the request",
    ]);
    expect(g.fix[0]).toContain("/readyz");
    expect(g.fix[1]).toContain("single_call_chars above 40,112");
    expect(g.code).toBe("guardrail_unavailable");
  });

  it("names the right fix for each error class, and copes with older events", () => {
    const fix = (cls) => eventGuide("guardrail.unscanned", { reason: "provider_error", error_class: cls }).fix[0];
    expect(fix("http_429")).toContain("rate limiting");
    expect(fix("http_401")).toContain("API key");
    expect(fix("timeout")).toContain("too slow");
    const old = eventGuide("guardrail.unscanned", { reason: "provider_error", action: "block", segments: [{ kind: "tool_result", tool: "read" }] });
    expect(old.headline).toBe("Blocked: provider failed on a tool result from read");
    expect(old.why[1]).toContain("does not cover large requests");
    expect(old.fix).toHaveLength(1);
  });

  it("says why an in-process provider failure is unchecked, and skips the fail mode under closed", () => {
    const local = eventGuide("guardrail.unscanned", { reason: "provider_error", fail_mode: "open", fail_mode_skipped: "local" });
    expect(local.why[1]).toContain("in-process");
    const closed = eventGuide("guardrail.unscanned", { reason: "provider_error", fail_mode: "closed", action: "block" });
    expect(closed.why).toHaveLength(2);
  });

  it("explains an oversize tool result and names the setting to raise", () => {
    const g = eventGuide(
      "guardrail.unscanned",
      {
        reason: "size",
        action: "block",
        limit: 262144,
        max_chars: 321097,
        count: 1,
        reason_code: "guardrail_too_large_to_scan",
        segments: [{ kind: "tool_result", role: "tool", tool: "fetch_results", chars: 321097 }],
      },
      "Wolf Defender: stop conversation"
    );
    expect(g.code).toBe("guardrail_too_large_to_scan");
    expect(g.headline).toBe("Blocked: a tool result from fetch_results too large to scan");
    expect(g.why[0]).toBe("321,097 chars, over the 262,144 limit (budget.max_scan_chars)");
    expect(g.fix[0]).toContain('"Wolf Defender: stop conversation" above 321,097');
  });

  it("explains a detection with its score and threshold, and how to lift a Wolf Defender false flag", () => {
    const g = eventGuide("guardrail.tool_result.critical", {
      action: "block",
      category: "injection",
      provider: "http",
      detector: "tool_result.critical",
      max_score: 0.97,
      threshold: 0.01,
      segments: [{ kind: "tool_result", tool: "read", sha256: "ab" }],
    });
    expect(g.code).toBe("guardrail_detected");
    expect(g.headline).toBe("Blocked: likely prompt injection in a tool result from read");
    expect(g.why).toEqual(["Detector tool_result.critical: score 0.97 (threshold 0.01)", "On a detection this filter blocks the request"]);
    expect(g.fix[0]).toContain("/v1/admin/allow");
    expect(g.fix).toHaveLength(2);
  });

  it("explains a removed tool definition without a block code", () => {
    const g = eventGuide("guardrail.tool_definition.block", {
      action: "remove",
      category: "injection",
      segments: [{ kind: "tool_definition", role: "system", tool: "refine-core_run_next" }],
    });
    expect(g.code).toBe("");
    expect(g.headline).toBe("Tool removed: likely prompt injection in the definition of the tool refine-core_run_next");
  });

  it("explains a provider failure by fail mode, quarantined tool results, and marks shadow events", () => {
    const closed = eventGuide("guardrail.error", { blocked: true, fail_mode: "closed", error_class: "timeout" });
    expect(closed.code).toBe("guardrail_unavailable");
    expect(closed.headline).toBe("Blocked: provider failed (timed out)");
    expect(closed.why[1]).toBe('Fail mode "closed": refuse the request');
    const quarantined = eventGuide("guardrail.error", { blocked: false, action: "quarantine", fail_mode: "closed", error_class: "http_503", count: 3 });
    expect(quarantined.headline).toBe("Quarantined: provider failed (HTTP 503) on 3 tool results");
    const shadow = eventGuide("guardrail.tool_result.review", { action: "log", shadow: true, category: "injection" });
    expect(shadow.headline).toContain("(shadow: traffic unchanged)");
  });

  // tkg.6: a judge event is a second opinion on a verdict, not a detection.
  describe("guardrail.judge", () => {
    const judge = (j) => ({ provider: "http", action: "log", blocked: false, request_id: "r1", judge: { action: "block", detectors: ["injection"], mode: "record", ...j } });

    it("says the judge confirmed the verdict, with its reason", () => {
      const g = eventGuide("guardrail.judge", judge({ verdict: "attack", reason: "role_override", agreed: true, coverage: "full", model: "claude-haiku-5-5", latency_ms: 840 }), "Wolf");
      expect(g.headline).toBe("Judge confirmed the block verdict: an attack (role override)");
      expect(g.why).toEqual(["Flagged by injection", "Asked claude-haiku-5-5 (840 ms)"]);
      expect(g.fix).toEqual(["Nothing to fix: the judge backs the filter."]);
      expect(g.code).toBe("");
    });

    it("says the judge disputed the verdict in record mode, and how to act on it", () => {
      const g = eventGuide("guardrail.judge", judge({ verdict: "benign", reason: "quoted_example", agreed: false, coverage: "full" }), "Wolf");
      expect(g.headline).toBe("Judge disputed the block verdict: benign (quoted example)");
      expect(g.why).toContain("Record mode: the opinion is kept beside the verdict, which stands");
      expect(g.fix[0]).toMatch(/detector is too strict/);
      expect(g.fix[1]).toMatch(/soften mode/);
    });

    it("explains why a benign opinion with partial coverage could not soften", () => {
      const g = eventGuide("guardrail.judge", judge({ verdict: "benign", reason: "code_or_data", agreed: false, coverage: "partial", mode: "soften", action: "quarantine" }), "Wolf");
      expect(g.headline).toBe("Judge disputed the quarantine verdict: benign (code or data)");
      expect(g.why).toContain("The judge was not shown everything flagged, so its opinion cannot soften the verdict");
      expect(g.fix).toHaveLength(1);
    });

    it("says which verdict the judge softened, and to what", () => {
      const g = eventGuide("guardrail.judge", judge({ verdict: "benign", reason: "code_or_data", agreed: false, coverage: "full", mode: "soften", softened_from: "block", softened_to: "log", cached: true }), "Wolf");
      expect(g.headline).toBe("Judge softened the block verdict to log-only: benign (code or data)");
      expect(g.why).toEqual(["Flagged by injection", "Soften mode: a benign opinion on everything flagged lowers the action to log-only", "Cached opinion"]);
      expect(g.fix[0]).toMatch(/back to record mode/);
    });

    it("says why the judge gave no opinion, and the fix for it", () => {
      const g = eventGuide("guardrail.judge", judge({ verdict: "none", error: "timeout" }), "Wolf");
      expect(g.headline).toBe("Judge gave no opinion on the block verdict (timed out)");
      expect(g.why).toEqual(["Flagged by injection", "The block verdict stands"]);
      expect(g.fix).toEqual(['Raise the judge\'s timeout on "Wolf", or pick a faster judge model.']);
    });

    it("never falls back to the detector triage", () => {
      for (const j of [{ verdict: "attack", agreed: true }, { verdict: "benign", agreed: false }, { verdict: "none" }, {}]) {
        const g = eventGuide("guardrail.judge", { action: "log", judge: j }, "Wolf");
        expect(g.headline).not.toMatch(/Logged only|Detector judge/);
        expect(g.fix.join(" ")).not.toMatch(/Raise the detector's threshold or exclude it/);
      }
      expect(eventGuide("guardrail.judge", undefined, "").headline).toBe("Judge gave no opinion on the filter's verdict");
    });
  });

  it("never gives more than two fixes", () => {
    const types = ["guardrail.unscanned", "guardrail.error", "guardrail.unreadable_request", "guardrail.unredactable", "guardrail.x"];
    for (const t of types) {
      for (const reason of ["size", "pieces", "time_budget", "provider_error", "input_rejected", "busy", undefined]) {
        const g = eventGuide(t, { ...prodUnscanned, reason });
        expect(g.fix.length).toBeLessThanOrEqual(2);
        expect(g.why.length).toBeLessThanOrEqual(3);
      }
    }
  });

  it("ignores events that are not guardrail events, and tolerates missing metadata", () => {
    expect(eventGuide("pii_redacted", {})).toBeNull();
    expect(eventGuide("guardrail.unscanned", null).headline).toContain("not scanned");
  });

  it("renders the guide", () => {
    render(<EventGuide event={{ event_type: "guardrail.unscanned", metadata: prodUnscanned, filter_name: "F" }} />);
    expect(screen.getByTestId("event-guide-headline")).toHaveTextContent("Blocked: provider failed (HTTP 502) on 6 tool results");
    expect(screen.getByTestId("event-guide-why").children.length).toBe(3);
    expect(screen.getByTestId("event-guide-fix").children.length).toBe(2);
  });
});
