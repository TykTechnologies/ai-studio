# Webhooks

:::note Enterprise Edition
Webhooks are an Enterprise Edition feature. In Community Edition the
Governance → Webhooks page shows what the feature offers and the API answers
`403`.
:::

Webhooks push AI Studio events to your own systems: a SIEM, a ticketing tool,
a Slack channel or any HTTPS endpoint you run. Every change made through AI
Studio (LLM, app, user, group, tool, datasource, filter, plugin, model price
and model router create/update/delete, app approvals, and events published by
enterprise plugins such as the asset catalog) is delivered as a signed JSON
document with retries, a dead-letter queue and a full delivery log.

## How approval works

Sending platform data to an arbitrary URL is an exfiltration risk, so a
webhook target goes through an approval step:

1. An administrator creates a target: name, URL, topic filters, payload
   template and optional custom headers (for example an `Authorization`
   value the receiver expects). The signing secret is shown once.
2. The target is **pending**. Nothing is sent. Administrators are notified.
3. Another administrator (or the same one, unless
   `WEBHOOKS_REQUIRE_DIFFERENT_APPROVER=true`) reviews the URL and approves
   it. Header values are stored encrypted and never displayed, only their
   names.
4. Deliveries start. Changing the URL or the headers of an approved target
   sends it back to pending; revoking stops it and cancels queued deliveries.

Every one of these steps is recorded in the [audit trail](audit-trail.md).
With role-based access control, approving, revoking, testing and replaying
need the `webhooks:execute` permission; viewing the log needs
`webhooks:read`.

## URL policy

Targets on internal networks (loopback, private ranges, link-local and cloud
metadata addresses, `localhost`, `.local`, `.internal`) are rejected unless
`WEBHOOKS_ALLOW_INTERNAL_TARGETS=true`. You can restrict targets to an allow
list (`WEBHOOKS_ALLOWED_HOSTS=hooks.example.com,.partner.example.org`) or
block hosts (`WEBHOOKS_DENIED_HOSTS`). The check runs when a target is
created, edited, approved and again before every send, and the connection is
checked against the resolved IP address, so a hostname later pointed at an
internal address is still blocked. Redirects are never followed.

## Topics

Targets subscribe with glob patterns:

| Pattern | Matches |
|---|---|
| `system.llm.*` | `system.llm.created`, `system.llm.updated`, `system.llm.deleted` |
| `system.*.deleted` | every deletion |
| `*` | everything |
| `asset_catalog.*` | topics published by the asset catalog plugin |

The target editor lists the platform's system topics and every topic seen on
the bus in the last 30 days.

## Payloads

Choose a preset or write a template:

- **standard** – a full envelope: event id, type, timestamps, the object
  (with secrets redacted), the actor and delivery metadata.
- **slack** – a Slack incoming-webhook message summarising the event.
- **minimal** – identifiers only.
- **custom** – a Go `text/template` that must render to JSON. Use
  **Preview** in the editor to render it against a sample event.

Example custom template:

```
{
  "text": {{ jsonStr (printf "%s %s: %s" (title .ObjectType) .Action (default "-" (get .Object "name"))) }},
  "event_id": {{ jsonStr .Event.ID }},
  "object": {{ toJson .Object }}
}
```

Secret-bearing fields (`api_key`, `password`, `token`, `authorization`, ...)
are replaced with `[REDACTED]` before the event is stored and before any
template sees it; add your own key fragments with `WEBHOOKS_REDACT_KEYS`.

## Verifying deliveries

Each request carries:

```
X-Webhook-Id            delivery id (same on every retry)
X-Webhook-Event-Id      event id (same across replays)
X-Webhook-Topic         topic
X-Webhook-Timestamp     unix seconds
X-Webhook-Attempt       attempt number
X-Webhook-Signature     v1=<hex HMAC-SHA256(secret, timestamp + "." + body)>
```

Verify by recomputing the HMAC over `timestamp + "." + rawBody` with the
target's signing secret and comparing it, in constant time, with each
comma-separated entry in the header (two entries are sent during a secret
rotation). Reject timestamps older than a few minutes. Return any 2xx status
to acknowledge.

Retries use exponential backoff with jitter (defaults: 10 attempts starting
at 5 s, capped at 1 h); `Retry-After` is honoured. `408`, `425`, `429` and
`5xx` responses and network errors are retried; other `4xx` and `3xx`
responses dead-letter the delivery immediately. Receivers should deduplicate
on `X-Webhook-Event-Id` if they need exactly-once processing.

## The delivery log

Governance → Webhooks → **Deliveries** lists every delivery with its status
(`queued`, `in_flight`, `retrying`, `succeeded`, `dead_lettered`,
`cancelled`), attempt count and last error. Filter by target, topic, status,
kind, date and free text; expand a row to see every attempt (status code,
latency, response snippet), the exact payload that was sent and the redacted
event. Dead letters can be replayed one at a time or in bulk; queued and
retrying deliveries can be cancelled. Export the filtered log as CSV or JSON
(up to 50,000 rows).

Retention: succeeded and cancelled deliveries are kept for
`WEBHOOKS_RETENTION_DAYS` (14), dead letters for
`WEBHOOKS_DEAD_LETTER_RETENTION_DAYS` (30).

## Operations

- **Multiple Studio nodes**: every node ingests the events it emits and
  delivers from the shared database; set `WEBHOOKS_WORKER_ENABLED=false` on
  nodes that should only manage targets. SQLite deployments are single-node.
- **Health**: `GET /api/v1/webhooks/status` reports bus connectivity, queue
  depth, retrying, in-flight and dead-lettered counts and `dropped_events`
  (events this node could not persist). The Webhooks page shows the same as a
  health pill and warning banners.
- **Secrets at rest**: set `TYK_AI_SECRET_KEY`; custom header values and
  signing secrets are encrypted with it.
- **Audit**: dead letters are recorded in the audit trail as `SYSTEM`
  actions; set `WEBHOOKS_AUDIT_DELIVERIES=true` to record successes too.

See `features/Webhooks.md` in the repository for the full configuration
reference and design notes.
