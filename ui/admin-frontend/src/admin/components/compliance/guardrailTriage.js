// Triage for a guardrail compliance event, from the event type and the
// metadata the gateway records (scripting/filter_runner.go,
// scripting/guardrail_scan.go): a one-line headline (outcome and cause), the
// reasons that led to the outcome, and at most two fixes. Returns null for
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

const KIND_PLURAL = {
  tool_result: "tool results",
  tool_call: "tool calls",
  tool_definition: "tool definitions",
};

// What the event is about: one segment by name, several by count and kind.
const subject = (m, segs) => {
  const n = typeof m.count === "number" ? m.count : segs.length;
  if (n <= 1) return where(segs[0]);
  const kinds = Array.isArray(m.kinds) ? m.kinds : [...new Set(segs.map((s) => s.kind))];
  const plural = kinds.length === 1 ? KIND_PLURAL[kinds[0]] || "messages" : "messages and tool results";
  return `${fmt(n)} ${plural}`;
};

const OUTCOME = {
  block: "Blocked",
  quarantine: "Quarantined",
  remove: "Tool removed",
  redact: "Redacted",
  log: "Logged only",
};

// What the filter's action did, as a reason line.
const ACTION_DID = {
  block: "this filter blocks the request",
  quarantine: "this filter replaces it with the quarantine placeholder",
  remove: "this filter removes the tool",
  redact: "this filter redacts it",
  log: "this filter only logs it",
};

const CATEGORY = {
  injection: "likely prompt injection",
  secrets: "credentials",
  pii: "personal data",
  moderation: "harmful content",
  leak: "likely system prompt or data leak",
  topic: "blocked topic",
};

// errorText names a guardrails.ErrorClass the way the headline uses it.
export const errorText = (cls) => {
  if (typeof cls !== "string" || cls === "") return "";
  const http = /^http_(\d{3})$/.exec(cls);
  if (http) return `HTTP ${http[1]}`;
  return (
    {
      timeout: "timed out",
      canceled: "call cancelled",
      suspended: "suspended after repeated failures",
      panic: "crashed",
      input_rejected: "refused the text",
      error: "network or response error",
    }[cls] || cls
  );
};

// providerFix is the one fix that fits how the provider failed.
const providerFix = (cls) => {
  const http = /^http_(\d{3})$/.exec(cls || "");
  const status = http ? Number(http[1]) : 0;
  if (status === 401 || status === 403) return "Check the provider's API key (the Studio secret) and its client-IP allowlist.";
  if (status === 429) return "The provider is rate limiting: give it more capacity or send it less traffic.";
  if (status === 413 || status === 400 || status === 422) return "The provider refused the request body: check its size limits.";
  if (status >= 500) return "The provider or a proxy in front of it failed: check its health (Wolf Defender: /readyz) and logs at this time.";
  switch (cls) {
    case "timeout":
      return "The provider is too slow: check its load, or raise the filter's timeout.";
    case "suspended":
      return "Calls resume on their own once the provider answers again; check why it kept failing.";
    case "error":
      return "Check the endpoint is reachable from the gateway (DNS, TLS, network).";
    default:
      return "Check the provider is up, reachable from the gateway, and its credentials are valid.";
  }
};

// failModeWhy is why the fail mode did not decide a provider failure
// (metadata.fail_mode_skipped, scripting/guardrail_scan.go notSmall*), or ""
// when there is nothing to explain.
const failModeWhy = (m) => {
  if (m.fail_mode === "closed") return "";
  const fm = `"${m.fail_mode || "open"}"`;
  switch (m.fail_mode_skipped) {
    case "run_chars":
      return `Fail mode ${fm} only covers requests up to ${fmt(m.single_call_chars)} chars; this one had ${fmt(m.run_chars)}`;
    case "run_calls":
      return `Fail mode ${fm} only covers requests that need a few provider calls; this one needed more`;
    case "seams":
      return `Fail mode ${fm} does not cover a redacting scan of text split into pieces`;
    case "resplit":
      return `Fail mode ${fm} does not cover a request the provider refused part of`;
    case "local":
      return "An in-process provider's failures always count as unchecked";
    default:
      return `Fail mode ${fm} does not cover large requests (they could be padded to make the provider fail)`;
  }
};

const UNSCANNED_CAUSE = {
  input_rejected: "Provider refused the text (often its own size limit)",
  provider_panic: "Provider crashed on the text",
  busy: "No in-process scan slot was free in time",
  shadow_dropped: "Shadow check dropped under load",
  seam: "Text across a split could not be redacted",
  undecodable: "Text could not be decoded for scanning",
};

const finish = (g, m) => {
  const shadow = m.shadow ? " (shadow: traffic unchanged)" : "";
  return { ...g, headline: g.headline + shadow, fix: g.fix.slice(0, 2) };
};

export const eventGuide = (eventType, metadata, filterName) => {
  if (typeof eventType !== "string" || !eventType.startsWith("guardrail.")) return null;
  const m = metadata && typeof metadata === "object" ? metadata : {};
  const segs = Array.isArray(m.segments) ? m.segments.filter((s) => s && typeof s === "object") : [];
  const first = segs[0];
  const filter = filterName ? `"${filterName}"` : "the filter";
  const outcome = OUTCOME[m.action] || "Flagged";
  const did = ACTION_DID[m.action];
  const actionWhy = did ? `Unchecked text counts as flagged; ${did}` : "Unchecked text counts as flagged";

  if (eventType === "guardrail.unscanned") {
    const what = subject(m, segs);
    switch (m.reason) {
      case "size":
        return finish(
          {
            code: m.reason_code || "guardrail_too_large_to_scan",
            headline: `${outcome}: ${what} too large to scan`,
            why: [`${fmt(m.max_chars)} chars, over the ${fmt(m.limit)} limit (budget.max_scan_chars)`, actionWhy],
            fix: [`Raise "Max characters scanned per segment" on ${filter} above ${fmt(m.max_chars)} (up to 16,777,216), then push to edges.`],
          },
          m
        );
      case "pieces":
        return finish(
          {
            code: m.reason_code || "guardrail_scan_budget_exceeded",
            headline: `${outcome}: ${what} not scanned, request too large`,
            why: [`Request needed more than ${fmt(m.limit)} provider pieces (budget.max_pieces)`, actionWhy],
            fix: [`Raise "Max provider pieces per request" on ${filter}, then push to edges.`],
          },
          m
        );
      case "time_budget":
        return finish(
          {
            code: m.reason_code || "guardrail_scan_budget_exceeded",
            headline: `${outcome}: scan ran out of time on ${what}`,
            why: [`Scan used up its ${fmt(m.limit)} ms budget (budget.time_ms)`, failModeWhy(m), actionWhy].filter(Boolean),
            fix: [providerFix(m.error_class || "timeout"), `Or raise the time budget (budget.time_ms) on ${filter}.`],
          },
          m
        );
      case "provider_error": {
        const err = errorText(m.error_class);
        const fix = [providerFix(m.error_class)];
        if (m.fail_mode_skipped === "run_chars" && m.fail_mode !== "closed") {
          fix.push(`To let such requests through when the provider fails, raise budget.single_call_chars above ${fmt(m.run_chars)} (less coverage).`);
        }
        return finish(
          {
            code: m.reason_code || "guardrail_unavailable",
            headline: `${outcome}: provider failed${err ? ` (${err})` : ""} on ${what}`,
            why: [err ? `Provider call failed: ${err}` : "Provider call failed", failModeWhy(m), actionWhy].filter(Boolean),
            fix,
          },
          m
        );
      }
      default: {
        const cause = UNSCANNED_CAUSE[m.reason] || `Not scanned (${m.reason || "unknown reason"})`;
        const fix =
          m.reason === "input_rejected"
            ? ["Check the provider's input size limit, or lower the filter's call size."]
            : m.reason === "busy" || m.reason === "shadow_dropped"
            ? ["Give the gateway more capacity, or send it less traffic."]
            : [providerFix(m.error_class)];
        return finish(
          {
            code: m.reason_code || "guardrail_unscanned",
            headline: `${outcome}: ${what} not scanned`,
            why: [cause, actionWhy],
            fix,
          },
          m
        );
      }
    }
  }

  if (eventType === "guardrail.error") {
    const err = errorText(m.error_class) || "error";
    const fm = `"${m.fail_mode || "default"}"`;
    if (m.action === "quarantine") {
      const n = typeof m.count === "number" ? m.count : 0;
      return finish(
        {
          code: "",
          headline: `Quarantined: provider failed (${err}) on ${n === 1 ? "a tool result" : `${fmt(n)} tool results`}`,
          why: [`Provider call failed: ${err}`, `Fail mode ${fm}: the tool results were replaced, the rest went on`],
          fix: [providerFix(m.error_class)],
        },
        m
      );
    }
    const blocked = m.blocked === true;
    return finish(
      {
        code: blocked ? "guardrail_unavailable" : "",
        headline: blocked ? `Blocked: provider failed (${err})` : `Let through unchecked: provider failed (${err})`,
        why: [`Provider call failed: ${err}`, `Fail mode ${fm}: ${blocked ? "refuse the request" : "let the request through"}`],
        fix: [
          providerFix(m.error_class),
          blocked
            ? `To let requests through during outages, set "If the provider fails" on ${filter} to let them through.`
            : "Requests passed unchecked; consider an alert on aistudio_guardrail_errors_total.",
        ],
      },
      m
    );
  }

  if (eventType === "guardrail.unreadable_request") {
    return finish(
      {
        code: "guardrail_unreadable_request",
        headline: "Blocked: request body cannot be screened",
        why: ["The body repeats a JSON key (or a case variant of one)", "Parsers would disagree on what it says"],
        fix: ["Fix the client that builds the body: send each key once."],
      },
      m
    );
  }

  if (eventType === "guardrail.unredactable") {
    return finish(
      {
        code: "",
        headline: `${outcome}: redaction could not be applied`,
        why: ["The match could not be rewritten in this body's shape", "The stronger action was used instead"],
        fix: ["Use quarantine or block for this content, or report the body shape."],
      },
      m
    );
  }

  // A detection: guardrail.<detector>.
  const detector = m.detector || eventType.slice("guardrail.".length);
  const found = CATEGORY[m.category] || detector;
  const score =
    typeof m.max_score === "number"
      ? `Detector ${detector}: score ${m.max_score}${typeof m.threshold === "number" ? ` (threshold ${m.threshold})` : ""}`
      : `Detector ${detector} matched`;
  const why = [score];
  if (typeof m.count === "number" && m.count > 1) why.push(`${fmt(m.count)} findings`);
  if (did) why.push(`On a detection ${did}`);
  const fix = [
    m.provider === "http"
      ? "False positive? Lift it in Wolf Defender by its SHA-256 (POST /v1/admin/allow)."
      : `False positive? Raise the detector's threshold or exclude it on ${filter}.`,
  ];
  if (m.provider === "http") fix.push("Or raise that question's threshold in the Wolf Defender policy.");
  return finish(
    {
      code: m.action === "block" ? m.reason_code || "guardrail_detected" : "",
      headline: `${outcome}: ${found} in ${where(first)}`,
      why,
      fix,
    },
    m
  );
};
