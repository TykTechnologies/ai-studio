# Compliance Events

Compliance Events are structured audit records that filter scripts can emit to flag governance-relevant activity — PII redactions, content rewrites, policy violations, silent failures — without affecting whether the request is blocked or allowed. They flow through the same analytics pipeline as proxy logs and chat records, are stored centrally on the control plane, and surface in the Compliance dashboard for review, drill-down, and CSV export.

This page is the end-to-end reference. For filter script syntax, see [Filters](filters.md#compliance-event-reporting).

Available from **v2.1.0**.

---

## Why compliance events

Before v2.1, the only thing the compliance dashboard could count was 4xx proxy logs — i.e. requests the gateway blocked. That misses the most common governance activity in practice: a filter quietly redacts an email address, rewrites a passage of text, or flags something suspicious while still allowing the request through. Those interventions are exactly what compliance teams need visibility into, and they previously left no trace beyond a debug log line.

Compliance events fix that. Any filter script can attach a list of structured events to its output. The platform persists them, propagates them from edge gateways back to the control plane, exposes them via API and dashboard, and counts them in Prometheus — all without changing the filter's block/allow decision.

---

## Event schema

A compliance event has the following shape (`ComplianceEventOutput` in the script API):

| Field | Type | Required | Description |
|---|---|---|---|
| `event_type` | string | Yes | Free-form type. Suggested conventions: `pii_redacted`, `content_rewritten`, `policy_violation`, `harmful_content_detected`, `silent_failure`. |
| `severity` | string | No | `info`, `warning`, or `critical`. Defaults to `info` if omitted or invalid. |
| `description` | string | No | Human-readable description of what happened, shown in the dashboard event row. |
| `metadata` | map | No | Arbitrary key-value data. Stored as JSON. Conventions: `matched_pattern` (string of the trigger), `redacted_types` (array of category names). |

**Validation:**

- Events without an `event_type` are silently skipped (avoids polluting the table with empty records when a script branch builds the event partially).
- `severity` values outside `info|warning|critical` are coerced to `info`.
- `event_type` on the API query side is limited to 100 characters — any longer is a 400.
- `severity` on the API query side is validated against the same whitelist; an invalid value is a 400.
- All API queries use GORM parameterized queries, so SQL injection attempts in `event_type` or `severity` are treated as literal string matches and return empty results.

### Server-side enrichment

Scripts only declare the four fields above. The platform enriches each event at recording time with context from the filter execution site:

| Enriched field | Source |
|---|---|
| `app_id` | App that owns the request (proxy paths) — `0` for chat-session paths, where `user_id` is the principal. |
| `user_id` | Authenticated user (chat-session paths). |
| `llm_id` | LLM in scope for the request. |
| `vendor` | LLM vendor string (e.g. `openai`, `anthropic`, `bedrock`). |
| `model_name` | Model being called. |
| `filter_name` | Name of the filter that emitted the event. |
| `filter_scope` | Where the script ran (see below). |
| `timestamp` | Wall-clock time at recording. |

### Filter scopes

`filter_scope` identifies which of the six filter execution sites recorded the event:

| Scope | Site |
|---|---|
| `proxy_request` | LLM proxy request filters (before reaching the LLM). |
| `proxy_response` | LLM proxy response filters (REST and streaming). |
| `chat_request` | Chat-session message filters (before RAG search and LLM). |
| `chat_response` | Chat-session response filters (regular and streaming). |
| `file_reference` | File content filters (before RAG indexing). |
| `tool_response` | Tool response filters (after tool execution). |

You can filter on `filter_scope` indirectly via the dashboard drill-down, or by joining `compliance_events.filter_name` against your filter catalog.

---

## Emitting events from a filter script

In a Tengo filter script, set `compliance_events` on the output object:

```tengo
tyk := import("tyk")

modified_payload := tyk.redact_pattern(
    input,
    "[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\\.[a-zA-Z]{2,}",
    "[EMAIL_REDACTED]"
)

output := {
    block: false,
    payload: modified_payload,
    message: "",
    compliance_events: [
        {
            event_type: "pii_redacted",
            severity: "info",
            description: "Email addresses redacted from request",
            metadata: { "redacted_types": ["email"] }
        }
    ]
}
```

See [Filters → Compliance Event Reporting](filters.md#compliance-event-reporting) for conditional emission, response-filter examples, and metadata conventions.

### Behavior

- Recording is **asynchronous** — events are queued and written by the analytics pipeline. Filter latency is unaffected.
- Compliance events **never** affect the block/allow decision. Setting `block: false` with critical events is legitimate (the filter chose to redact rather than block); setting `block: true` with no events is also legitimate.
- The list can be empty or omitted. Most filters won't emit events on every invocation — typically only when a condition fires.
- Events are recorded **per filter, per invocation**. A single request can produce multiple events from multiple filters.

---

## Edge gateways (Microgateway)

In a hub-spoke deployment, filter scripts run on the edge Microgateway, but compliance events live on the control plane. The propagation path is:

1. Filter script on the edge sets `compliance_events` in its output.
2. `MicrogatewaAnalyticsHandler.RecordComplianceEvents` queues the events to the pulse plugin's local buffer (survives gateway restart).
3. On the next analytics pulse (every 30 seconds by default), events are batched into the gRPC `AnalyticsPulse` message as `ComplianceEventProto` entries.
4. The control server's `SendAnalyticsPulse` handler reconstructs `models.ComplianceEvent` from each entry — preserving `LLMID`, `AppID`, `UserID`, severity, type, description, metadata, vendor, and model name — and persists them.

This means there's no functional difference between an event recorded by the embedded gateway and one recorded by an edge Microgateway: both end up in the same table with the same enrichment.

If you need to verify that edge events are arriving, check the `aistudio_compliance_events_total` counter on the control-plane `/metrics` endpoint and group by source.

---

## Querying compliance events

### Admin API

```
GET /api/v1/compliance/events
```

Admin-only (requires the v1 route group's auth middleware + `AdminOnly`).

**Query parameters:**

| Param | Type | Description |
|---|---|---|
| `start_date` | ISO8601 date | Lower bound (inclusive) on `timestamp`. |
| `end_date` | ISO8601 date | Upper bound (inclusive) on `timestamp`. |
| `app_id` | uint | Filter by app. |
| `event_type` | string (≤ 100 chars) | Exact match on `event_type`. |
| `severity` | `info` / `warning` / `critical` | Whitelist; other values return 400. |
| `limit` | int | Page size (default and max are deployment-dependent). |
| `offset` | int | Page offset. |

**Example:**

```bash
curl -H "Authorization: Bearer $ADMIN_TOKEN" \
  "https://studio.example.com/api/v1/compliance/events?start_date=2026-05-01&end_date=2026-05-14&severity=critical&limit=50"
```

**Response shape:**

```json
{
  "events": [
    {
      "id": 1234,
      "app_id": 42,
      "user_id": 0,
      "llm_id": 7,
      "filter_name": "PII Detector",
      "filter_scope": "proxy_request",
      "event_type": "pii_redacted",
      "severity": "info",
      "description": "Email addresses redacted from request",
      "metadata": "{\"redacted_types\":[\"email\"]}",
      "vendor": "openai",
      "model_name": "gpt-4",
      "timestamp": "2026-05-14T09:23:17Z"
    }
  ],
  "total": 1
}
```

`metadata` is returned as a JSON-encoded string (the storage shape). Parse client-side.

### SQL

The underlying table is `compliance_events`. Useful indexes:

- `(app_id, timestamp)` — composite for per-app time-bounded queries.
- `user_id`, `llm_id`, `filter_name`, `event_type`, `severity` — single-column for filtering.
- `timestamp` — for retention sweeps.

Example:

```sql
SELECT filter_name, event_type, COUNT(*) AS n
FROM compliance_events
WHERE timestamp >= NOW() - INTERVAL '7 days'
  AND severity IN ('warning', 'critical')
GROUP BY filter_name, event_type
ORDER BY n DESC;
```

---

## Dashboard surfaces

The Compliance dashboard (`/admin/compliance`) was extended in v2.1 to surface compliance events alongside the existing blocked-request data:

### Summary cards

Two new cards next to the blocked-request total:

- **Critical Events** — count over the selected window, with trend arrow. Escalates the card style above `COMPLIANCE_EVENTS_CRITICAL_ESCALATE_AT` (1 — a single critical event already warrants attention).
- **Warning Events** — count over the selected window, with trend arrow. Escalates above `COMPLIANCE_EVENTS_WARNING_ESCALATE_AT` (20 — warnings are advisory and need to accumulate before they matter).

### Policy Violations tab

The summary is split into three counters:

- **Blocked** — 4xx proxy logs (existing).
- **Flagged** — warning + critical compliance events that passed through (new).
- **Affected Apps** — distinct apps appearing in either source.

The timeline chart merges both streams. The per-app violation list unions events into the aggregation, so apps that only ever flag (and never block) appear in the drill-down.

### Filter Events tab (new)

A dedicated tab for compliance events with:

- Severity totals (info / warning / critical) with a stacked timeline chart.
- Severity and event-type filters (event-type dropdown is populated from observed values in the window).
- Pagination over the event list.
- Expandable rows showing the raw `metadata` JSON.
- CSV export wired to `GET /compliance/events`.

### App Risk modal

Per-app risk score now includes event counts (`warningEventWeight = 1`, `criticalEventWeight = 3` — a single critical event contributes the same to the score as three warning events). Recent Violations interleaves blocked requests and compliance events sorted by timestamp, with severity-aware row rendering.

---

## Metrics

A Prometheus / OpenTelemetry counter is exposed on the `/metrics` endpoint:

```
aistudio_compliance_events_total{filter_scope, severity, event_type}
```

Use this to alert on critical-event bursts or to watch for a sudden drop in expected redaction activity (which can indicate a misconfigured filter). Combine with `aistudio_llm_requests_total` to get a per-request rate.

See [Telemetry](telemetry.md) for the full metric catalog.

---

## Suggested event-type conventions

`event_type` is free-form, but consistency across filters makes dashboards and queries much more useful. Conventions used by the bundled filter scripts:

| Event type | When to emit |
|---|---|
| `pii_redacted` | Filter replaced PII patterns with placeholders. Set `metadata.redacted_types` to an array of categories (`["email", "ssn", "phone"]`). |
| `content_rewritten` | Filter transformed user or LLM content in a way the user can't see (e.g. system-prompt augmentation, anonymization). |
| `sensitive_content_detected` | Filter found content matching a sensitive pattern. Pair with `block: true` for hard blocks; pair with `block: false` to record the detection without blocking. |
| `harmful_content_detected` | Response filter detected harmful output. Set `metadata.matched_pattern` to the trigger. |
| `policy_violation` | Filter detected a policy breach that may or may not be blocked. |
| `silent_failure` | Filter encountered an error path it chose to swallow (e.g. unreachable classifier service, falling back to allow). Use `severity: warning` so it shows up in the dashboard without paging anyone. |

For `metadata` keys, prefer:

- `matched_pattern` (string) — the substring or regex match that fired the event.
- `redacted_types` (array of strings) — categories of PII or content removed.

These keys are what the bundled filter scripts and the dashboard expect.

---

## Migration notes

- **Schema**: v2.1 GORM auto-migration creates the `compliance_events` table on first startup. No manual step required.
- **Filter scripts** written for v2.0 are forward-compatible — the `compliance_events` field is optional and ignored if absent.
- **Bundled filter library**: the ship-with scripts (PII redaction, content blocking, response guardrails) were updated in v2.1 to emit compliance events with the conventions above. Re-import them from the marketplace or copy from the updated templates in the admin UI if you want them out of the box.
- **Custom analytics handlers**: the `AnalyticsHandler` interface in `services/analytics` now takes `context.Context` on its recording methods, including the new `RecordComplianceEvents`. Custom implementations need to update their signatures.

---

## See also

- [Filters → Compliance Event Reporting](filters.md#compliance-event-reporting) — script-level syntax and examples.
- [Analytics & Monitoring](analytics.md) — the surrounding analytics pipeline.
- [Edge Gateways](edge-gateways.md) — hub-spoke architecture and the analytics pulse that carries edge events to control.
- [Telemetry](telemetry.md) — Prometheus / OTEL metrics including `aistudio_compliance_events_total`.
- [Plugin SDK Changes in v2.1](plugins-sdk-changes-v2.1.md) — `AnalyticsHandler` context change and related SDK updates.
