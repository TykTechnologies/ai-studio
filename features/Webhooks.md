# Webhooks (ENTERPRISE EDITION ONLY)

Outbound webhooks push AI Studio events to external HTTP endpoints. Every
`system.<object>.<action>` event the platform emits on its internal event bus
(LLM, app, user, group, tool, datasource, filter, plugin, model price and
model router changes, plus topics published by enterprise plugins such as the
asset catalog) can be delivered to an administrator-approved URL as a signed
JSON payload, with retries, a dead-letter queue and a searchable delivery log.

Target URLs are an exfiltration vector, so the feature is built around
approval: a target never receives a delivery until an administrator approves
its URL, and any change to where it points sends it back for approval.

## Edition Comparison

### Community Edition

- Feature unavailable. `GET /api/v1/webhooks/status` reports
  `available: false`; every other webhook endpoint returns `403` with an
  Enterprise message; the admin UI shows an upsell page.
- The `webhook_*` tables are created (shared migration) but never written.

### Enterprise Edition

- Targets with an approval workflow, pause/resume, secret rotation and test
  events.
- Per-target payload templates (Go `text/template` producing JSON) with
  built-in presets and automatic redaction of secret-bearing fields.
- At-least-once delivery from the moment an event is persisted: exponential
  backoff with jitter, `Retry-After` support, per-target concurrency caps,
  dead-letter queue, single and bulk replay.
- Searchable, exportable delivery log with every attempt's status code,
  latency, response snippet and error.
- Audit trail integration: approvals, rejections, revocations and edits are
  recorded by the audit middleware with a diff of the target row; dead
  letters are recorded by the worker as `SYSTEM` records.
- Admin notifications for pending targets and dead letters.
- RBAC: the `webhooks` resource (Governance group) with read, write, delete
  and execute; execute covers approve, reject, revoke, test and replay.

## Architecture

```
CRUD handler ──emit──▶ event bus (pkg/eventbridge, synchronous)
                          │
                          ▼ ingestor (enterprise/features/webhooks/ingest.go)
                 redact ──▶ webhook_events (id = bus event id, idempotent)
                          │  fan-out to approved targets whose topic filters match
                          ▼
                 webhook_deliveries (queued, dedupe key event:target)
                          │
                          ▼ engine (engine.go): claim due rows with a lease
                          │  + optimistic lock (safe across Studio nodes)
                          ▼ sender (sender.go): render once, sign, POST,
                          │  classify, back off / dead-letter
                          ▼
                 webhook_delivery_attempts (one row per attempt)
```

- **Contract**: `services/webhooks/interface.go` (`Service`, DTOs, errors);
  factory + community stub alongside. Enterprise registers via
  `enterprise/features/webhooks/init.go`, linked by `main_enterprise.go`.
- **Wiring**: `services.Service.InitWebhooks` (called from `main.go` after
  the event bus is attached, in both control and standalone mode — standalone
  now creates a node-local bus), stopped first in `Service.Cleanup`.
- **Models**: `models/webhook.go` — `WebhookTarget`, `WebhookEvent`,
  `WebhookDelivery`, `WebhookDeliveryAttempt`. Custom headers and signing
  secrets are encrypted at rest with `TYK_AI_SECRET_KEY` through GORM hooks;
  `WebhookTargetResponse` never carries secret values (header names only).
  Topic filters are stored as JSON text so the audit trail can snapshot the row.
- **API**: `api/webhook_handlers.go`, routes under the admin-only
  `/api/v1/webhooks/...` group, annotated with `authz.Read/Write/Delete/
  Execute("webhooks")`.
- **UI**: `ui/admin-frontend/src/admin/pages/Webhooks.js` (targets) and
  `WebhookDeliveries.js` (log), Governance → Webhooks, gated on the
  `feature_webhooks` flag from `/common/system` and `P.WEBHOOKS_READ`.

## Target Lifecycle

| From       | Action                                         | To        |
|------------|------------------------------------------------|-----------|
| —          | create                                         | pending   |
| pending    | approve (URL policy re-checked)                | approved  |
| pending    | reject (reason)                                | rejected  |
| approved   | revoke (reason; queued deliveries cancelled)   | revoked   |
| approved   | edit URL or headers                            | pending   |
| rejected / revoked | any edit (resubmit)                    | pending   |

Name, description, topic filters, template and concurrency edits do not
re-pend an approved target (they change what is sent, not where); all edits
are audited. `WEBHOOKS_REQUIRE_DIFFERENT_APPROVER=true` rejects approval by
the administrator who created the target. Pause keeps queueing deliveries but
stops sending; resume drains the backlog. Deliveries are created only for
approved targets and the worker re-checks approval before every send.

## URL Policy

Always on, independent of the `LLM_UPSTREAM_*` environment gating used for
LLM upstreams:

- `http`/`https` only, no userinfo, no fragment, valid port.
- IP literals and hostnames in loopback, RFC1918, link-local (cloud metadata),
  unique-local and `localhost`/`.local`/`.internal` are rejected unless
  `WEBHOOKS_ALLOW_INTERNAL_TARGETS=true` or the exact host is listed in
  `WEBHOOKS_ALLOWED_HOSTS`.
- `WEBHOOKS_DENIED_HOSTS` always wins; `WEBHOOKS_ALLOWED_HOSTS` (exact or
  `.suffix`) restricts targets when set.
- The dialer re-checks the *resolved* IP at connect time, closing DNS
  rebinding, and the client never follows redirects (a 3xx is a permanent
  failure that needs a new approval).
- The policy is applied on create, on update, on approve and again before
  every send.

## Payloads and Templates

Templates are Go `text/template` documents that must render to valid JSON.
Presets: `standard` (full envelope), `slack` (incoming-webhook message),
`minimal` (identifiers). `custom` takes a body (≤ 64 KB) that is compiled and
rendered against a sample event on save; rendering is capped at 2 s and 1 MB.

Template data: `.Event{ID,Topic,Origin,ReceivedAt}`, `.ObjectType`,
`.Action`, `.ObjectID`, `.ActorUserID`, `.Timestamp`, `.Object` (redacted
map), `.Payload` (redacted map), `.Target{ID,Name}`,
`.Delivery{ID,Attempt}`, `.Studio{Edition,Version}`.

Functions: `toJson`, `toPrettyJson`, `jsonStr`, `upper`, `lower`, `title`,
`trim`, `default`, `coalesce`, `now`, `date`, `unixTime`, `truncate`,
`redact`, `get`, `has`, `join`, `toString`, `int` plus the standard
`printf`. There is no environment, file or network access.

Redaction runs before an event is stored and again on every value a template
reads: any key whose lower-cased form contains `password`, `secret`,
`api_key`, `apikey`, `token`, `auth_key`, `conn_string`, `private_key`,
`passphrase`, `credential`, `client_key`, `access_key`, `authorization`,
`cookie` or `bearer` (extend with `WEBHOOKS_REDACT_KEYS`) becomes
`[REDACTED]`. The raw object is never persisted.

## Delivery Guarantees

- **At-least-once from persistence.** The ingestor runs on the emitter's
  goroutine and persists the event and its fan-out in one short transaction
  before the CRUD call returns. If the database refuses the write, the event
  spills to a bounded in-memory queue (1000) and is retried; overflow is
  counted in `dropped_events` on the status endpoint and shown in the UI.
- **Events are emitted after commit** with no commit hook, so a crash between
  the commit and the emit loses that one event.
- **Deduplication** on the bus event ID (events) and on `event:target`
  (deliveries); a redelivered bus event fans out once. Replays and test events
  are new deliveries with the same `X-Webhook-Event-Id`, so receivers wanting
  exactly-once should deduplicate on that header per target.
- **Retries**: `min(max, base·2^(n−1))` with full jitter, `Retry-After`
  honoured, `408/425/429/5xx` and transport errors retryable, other `4xx` and
  `3xx` dead-letter immediately. Byte-identical body and `X-Webhook-Id` on
  every attempt; `X-Webhook-Attempt`, timestamp and signature change.
- **Multi-node**: every Studio node ingests its own local events and, with
  `WEBHOOKS_WORKER_ENABLED=true`, claims due rows from the shared database
  with a lease and an optimistic lock; expired leases are reclaimed. No
  ordering guarantee across targets. SQLite deployments are single-node.
- **Shutdown**: `Stop` drains in-flight sends up to
  `WEBHOOKS_SHUTDOWN_DRAIN_TIMEOUT` and hands its leases back.

## Request Format

```
POST <target url>
Content-Type: application/json
User-Agent: tyk-ai-studio-webhooks/<version>
X-Webhook-Id: <delivery id>
X-Webhook-Event-Id: <event id>
X-Webhook-Topic: system.llm.created
X-Webhook-Timestamp: 1757500000
X-Webhook-Attempt: 1
X-Webhook-Signature: v1=<hex hmac-sha256>[,v1=<hex hmac-sha256 with previous secret>]
<custom headers>
```

Signature: `HMAC-SHA256(secret, timestamp + "." + body)`. During a rotation
(`WEBHOOKS_SECRET_ROTATION_GRACE`, default 24h) two entries are sent, new
secret first. Reference verification:

```go
func verify(header, secret, ts string, body []byte) bool {
    mac := hmac.New(sha256.New, []byte(secret))
    mac.Write([]byte(ts + "."))
    mac.Write(body)
    want := "v1=" + hex.EncodeToString(mac.Sum(nil))
    for _, part := range strings.Split(header, ",") {
        if hmac.Equal([]byte(strings.TrimSpace(part)), []byte(want)) {
            return true
        }
    }
    return false
}
```

```js
const crypto = require("crypto");
function verify(header, secret, ts, body) {
  const want = "v1=" + crypto.createHmac("sha256", secret).update(`${ts}.`).update(body).digest("hex");
  return header.split(",").some((p) => crypto.timingSafeEqual(Buffer.from(p.trim()), Buffer.from(want)));
}
```

Reject timestamps older than a few minutes to defeat replay.

## Configuration

| Variable | Default | Purpose |
|---|---|---|
| `WEBHOOKS_ENABLED` | `true` | Turn the feature on |
| `WEBHOOKS_WORKER_ENABLED` | `true` | Run the delivery worker on this node |
| `WEBHOOKS_WORKER_COUNT` | `4` | Concurrent workers per node (1–64) |
| `WEBHOOKS_MAX_ATTEMPTS` | `10` | Attempts before dead-lettering (1–50) |
| `WEBHOOKS_BASE_BACKOFF` / `WEBHOOKS_MAX_BACKOFF` | `5s` / `1h` | Retry schedule bounds |
| `WEBHOOKS_REQUEST_TIMEOUT` | `10s` | Per-attempt timeout (≤ 60s) |
| `WEBHOOKS_ALLOW_INTERNAL_TARGETS` | `false` | Permit internal addresses |
| `WEBHOOKS_ALLOWED_HOSTS` / `WEBHOOKS_DENIED_HOSTS` | — | CSV host lists (exact or `.suffix`) |
| `WEBHOOKS_RETENTION_DAYS` | `14` | Purge succeeded/cancelled deliveries |
| `WEBHOOKS_DEAD_LETTER_RETENTION_DAYS` | `30` | Purge dead letters |
| `WEBHOOKS_MAX_RESPONSE_SNIPPET_BYTES` | `4096` | Stored response body per attempt |
| `WEBHOOKS_REQUIRE_DIFFERENT_APPROVER` | `false` | Four-eyes approval |
| `WEBHOOKS_SECRET_ROTATION_GRACE` | `24h` | Previous secret validity after rotation |
| `WEBHOOKS_REDACT_KEYS` | — | Extra key fragments to redact |
| `WEBHOOKS_AUDIT_DELIVERIES` | `false` | Also audit successful deliveries |
| `WEBHOOKS_SHUTDOWN_DRAIN_TIMEOUT` | `15s` | Drain wait on shutdown |

Secrets at rest require `TYK_AI_SECRET_KEY`; without it, header values and
signing secrets are stored in plaintext (a startup warning is logged).

## API

All routes are admin-only under `/api/v1` and require the `webhooks`
permission (`status` needs any admin):

- `GET /webhooks/status`, `GET /webhooks/topics`,
  `GET /webhooks/templates/presets`, `POST /webhooks/templates/preview` (read)
- `GET /webhooks/targets` (read), `POST /webhooks/targets` (write),
  `GET|PATCH|DELETE /webhooks/targets/:id` (read / write / delete)
- `POST /webhooks/targets/:id/{approve|reject|revoke|test}` (execute),
  `POST /webhooks/targets/:id/{pause|resume|rotate-secret}` (write)
- `GET /webhooks/deliveries` (filters: `target_id`, `topic`, `status`,
  `event_id`, `kind`, `start_date`, `end_date`, `search`, `page`,
  `page_size`, `sort`), `GET /webhooks/deliveries/:id`,
  `GET /webhooks/deliveries/export?format=csv|json`, `GET /webhooks/stats` (read);
  `POST /webhooks/deliveries/:id/{replay|cancel}`,
  `POST /webhooks/deliveries/replay` (bulk dead letters) (execute)

Errors: `403` Enterprise feature / same approver, `404`, `409` conflict or
invalid state, `422` URL policy or template, `400` validation.

## Audit Trail Integration

- HTTP actions on targets and deliveries are classified in
  `enterprise/features/audit/actions.go` (`Approve Webhook Target`,
  `Revoke Webhook Target`, `Replay Webhook Delivery`, ...) with
  `resource_type` `webhook_target` / `webhook_delivery`; target rows are
  snapshotted for field diffs, with secrets redacted.
- Dead letters (and successes when `WEBHOOKS_AUDIT_DELIVERIES=true`) are
  recorded by the worker through `audit.Service.Record` with
  `method=SYSTEM`, `route=webhooks/worker`, `user=system`.
- The target detail panel in the UI shows the object's audit history from
  `GET /audit/resources/webhook_target/:id`.

## Testing

- `go test ./config/ ./services/webhooks/ ./api/ ./pkg/authz/ -run Webhook`
  (CE stubs, handler validation, authorisation with TestMode off, catalogue).
- `cd enterprise && go test -tags enterprise ./features/webhooks/` (policy,
  redaction, templates, signing, backoff, classification, lifecycle, claim
  concurrency, end-to-end delivery against `httptest`, retention).
- `go test -tags enterprise ./api/ -run WebhooksEnterprise` (HTTP → bus →
  delivery → audit records).
- `cd ui/admin-frontend && npm test -- Webhook`.

Tests that involve the delivery workers or the audit writer must use a named
shared-cache SQLite DSN (see `sharedMemoryDB` in
`api/webhook_handlers_enterprise_test.go`); a plain `:memory:` database gives
every pooled connection its own empty database.

## Limitations

- No inbound endpoint for receipts; receivers acknowledge with a 2xx only.
- Templates are administrator-authored; the function set has no I/O and
  redaction bounds what an event can leak, but a template can still shape
  arbitrary JSON from the redacted event.
- The bus is node-local: an event is ingested on the node whose API call
  emitted it. Hub/spoke edges do not emit `system.*` events.
