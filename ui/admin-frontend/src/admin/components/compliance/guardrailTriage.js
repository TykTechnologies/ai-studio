// Plain-English triage for a guardrail compliance event: what happened and
// how to fix it, from the event type and the metadata the gateway records
// (scripting/filter_runner.go, scripting/guardrail_scan.go). Returns null for
// events that are not guardrail events.

const fmt = (n) => (typeof n === "number" ? n.toLocaleString("en-US") : String(n ?? "?"));

export const where = (seg) => {
  if (!seg) return "a message or tool result";
  switch (seg.kind) {
    case "tool_result":
      return seg.tool ? `a tool result from ${seg.tool}` : "a tool result";
    case "tool_call":
      return seg.tool ? `a call to the tool ${seg.tool}` : "a tool call";
    case "tool_definition":
      return seg.tool ? `the definition of the tool ${seg.tool}` : "a tool definition";
    default:
      if (seg.role === "system") return "the system prompt";
      if (seg.role === "assistant") return "an assistant message";
      return "a user message";
  }
};

const actionWords = {
  block: "The request was refused (HTTP 400) with the filter's block message and this reason.",
  quarantine: "The flagged message or tool result was replaced with the quarantine placeholder; the request went on.",
  remove: "The flagged tool was removed from the request; the request went on.",
  redact: "The matched text was redacted; the request went on.",
  log: "Recorded only; the request went on unchanged.",
};

const CATEGORY = {
  injection: "a likely prompt injection",
  secrets: "credentials",
  pii: "personal data",
  moderation: "harmful content",
  leak: "a likely system prompt or data leak",
  topic: "a blocked topic",
};

export const eventGuide = (eventType, metadata, filterName) => {
  if (typeof eventType !== "string" || !eventType.startsWith("guardrail.")) return null;
  const m = metadata && typeof metadata === "object" ? metadata : {};
  const segs = Array.isArray(m.segments) ? m.segments.filter((s) => s && typeof s === "object") : [];
  const first = segs[0];
  const filter = filterName ? `"${filterName}"` : "this filter";
  const action = actionWords[m.action] || "";
  const shadow = m.shadow ? " This filter runs in shadow: traffic was not changed." : "";

  if (eventType === "guardrail.unscanned") {
    const size = first && first.chars ? ` (${fmt(first.chars)} characters)` : "";
    switch (m.reason) {
      case "size":
        return {
          code: m.reason_code || "guardrail_too_large_to_scan",
          what: `${where(first)}${size} was longer than ${filter} scans (${fmt(m.limit)} characters per message or tool result), so it was not checked. Text that is not checked is treated as flagged. ${action}${shadow}`,
          fix: [
            `If this traffic is expected, raise "Max characters scanned per segment" (budget.max_scan_chars) on ${filter} above ${fmt(m.max_chars)}, up to 16,777,216, then push the configuration to edges. Long text is sent to the provider in pieces.`,
            "Skipping the app in the guardrail's own policy does not help here: the gateway decides before it calls the provider.",
          ],
        };
      case "pieces":
        return {
          code: m.reason_code || "guardrail_scan_budget_exceeded",
          what: `The request had more text than ${filter} scans in one request (${fmt(m.limit)} pieces), so ${where(first)} was not checked. ${action}${shadow}`,
          fix: [`Raise "Max provider pieces per request" (budget.max_pieces) on ${filter}, then push to edges.`],
        };
      case "time_budget":
        return {
          code: m.reason_code || "guardrail_scan_budget_exceeded",
          what: `${filter} ran out of time (${fmt(m.limit)} ms for the whole request) before it checked ${where(first)}. ${action}${shadow}`,
          fix: [`Raise the timeout or the time budget (budget.time_ms) on ${filter}, or check why the provider is slow.`],
        };
      case "provider_error":
      case "provider_panic":
      case "busy":
      case "shadow_dropped":
        return {
          code: m.reason_code || "guardrail_unavailable",
          what: `The guardrail provider could not check ${where(first)} (${m.reason}). ${action}${shadow}`,
          fix: [
            "Check that the provider is up and reachable from the gateway (for Wolf Defender: its /readyz and the client-IP allowlist), and its credentials.",
            "If it is busy, give it more capacity or less traffic.",
          ],
        };
      default:
        return {
          code: m.reason_code || "guardrail_unscanned",
          what: `${where(first)} could not be screened (${m.reason || "unknown reason"}). ${action}${shadow}`,
          fix: ["input_rejected means the provider refused the text, often because of its own size limit."],
        };
    }
  }

  if (eventType === "guardrail.error") {
    const blocked = m.blocked === true;
    return {
      code: blocked ? "guardrail_unavailable" : "",
      what: `The guardrail provider failed (${m.error_class || "error"}). The filter's fail mode is ${m.fail_mode || "its default"}, so the request was ${blocked ? "refused" : "let through unchecked"}.${shadow}`,
      fix: [
        "Check the provider's health, endpoint and API key (the Studio secret).",
        blocked
          ? `If availability matters more than coverage, set "If the provider fails" on ${filter} to let the request through.`
          : "Text passed unchecked while the provider was failing; consider an alert on aistudio_guardrail_errors_total.",
      ],
    };
  }

  if (eventType === "guardrail.unreadable_request") {
    return {
      code: "guardrail_unreadable_request",
      what: "The request body has a repeated or case-variant JSON key, so different parsers read it differently and it cannot be screened. It was refused.",
      fix: ["Fix the client that builds the body: send each key once."],
    };
  }

  if (eventType === "guardrail.unredactable") {
    return {
      code: "",
      what: "A redaction could not be written into the body, so the tool result was quarantined or the request refused instead.",
      fix: ["Use quarantine or block for this content, or report the body shape if it should have been redactable."],
    };
  }

  // A detection: guardrail.<detector>.
  const detector = m.detector || eventType.slice("guardrail.".length);
  const found = CATEGORY[m.category] || `content matching ${detector}`;
  const fix = [];
  if (m.action === "remove" || eventType.includes("tool_definition")) {
    const tool = first && first.tool ? `the tool ${first.tool}` : "the tool";
    fix.push(`${tool} was dropped from the model's tools. If its description is legitimate this is a false positive: check the excerpt.`.replace(/^t/, "T"));
  }
  fix.push(
    'Check the excerpt under "Flagged text" (turn on "Show flagged text in compliance events" on the filter if there is none).',
    m.provider === "http"
      ? "If it is a false positive and the provider is Wolf Defender: lift the stored flag with its SHA-256 (POST /v1/admin/allow on the guardrail, or python -m wolfguard.store allow SHA256), and consider raising that question's threshold in its policy."
      : `If it is a false positive: raise the detector's threshold on ${filter}, exclude the detector, or adjust the provider's policy.`
  );
  return {
    code: m.action === "block" ? m.reason_code || "guardrail_detected" : "",
    what: `${filter} found ${found} in ${where(first)}${typeof m.max_score === "number" ? ` (score ${m.max_score})` : ""}. ${action}${shadow}`,
    fix,
  };
};
