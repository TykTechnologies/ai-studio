# Outbound Webhooks

## Overview

Tyk AI Studio can push platform events to external HTTP endpoints (webhooks) in real time. Subscriptions match specific event topics exactly, and the platform delivers a signed JSON payload to the target URL with at-least-once delivery semantics and exponential-backoff retries.

Retry policy is configurable at three levels:
1. **Static defaults** — set at startup via environment variables (see Configuration)
2. **Global dynamic defaults** — stored in the `webhook_configs` DB singleton, updatable at runtime via API without restart
3. **Per-subscription overrides** — `retry_policy` field on each `WebhookSubscription`

All webhook endpoints require admin access.

### Use Cases

- Slack / Teams notifications on LLM changes or budget alerts
- PagerDuty incidents on repeated delivery failures
- CI/CD pipeline triggers when App configurations are updated
- SIEM ingestion of all platform CRUD audit events

---

## Event Coverage

Subscribable topics are defined in `services/system_events.go` as `KnownWebhookTopics`. Use `GET /api/v1/webhooks/topics` to retrieve the current list at runtime.

| Domain | Topics |
|---|---|
| LLM | `system.llm.created`, `system.llm.updated`, `system.llm.deleted` |
| App | `system.app.created`, `system.app.updated`, `system.app.deleted`, `system.app.approved`, `system.app.plugin_resources_changed` |
| User | `system.user.created`, `system.user.updated`, `system.user.deleted` |
| Group | `system.group.created`, `system.group.updated`, `system.group.deleted` |
| Datasource | `system.datasource.created`, `system.datasource.updated`, `system.datasource.deleted` |
| Tool | `system.tool.created`, `system.tool.updated`, `system.tool.deleted` |
| Filter | `system.filter.created`, `system.filter.updated`, `system.filter.deleted` |
| Plugin | `system.plugin.created`, `system.plugin.updated`, `system.plugin.deleted` |
| Model Price | `system.modelprice.created`, `system.modelprice.updated`, `system.modelprice.deleted` |
| Model Router | `system.modelrouter.created`, `system.modelrouter.updated`, `system.modelrouter.deleted` |

Each topic in `topics` must be an exact match against a known topic. Unknown topics are rejected at create/update time.

---

## Architecture

```mermaid
flowchart TD
    A[Service Layer] -->|Publish| B[eventbridge.Bus]
    B -->|SubscribeAll| C[WebhookService.HandleEvent]
    C -->|Persist row| D[WebhookEvent table\nstatus=pending]
    C -->|Non-blocking push| E[queue chan uint]
    E --> F[Worker goroutines]
    G[DB Poller] -->|Poll due events| D
    G -->|Push IDs| E
    F -->|resolveRetryPolicy| P[Merge: sub > DB global > static]
    P -->|SSRF check + HMAC sign| H[HTTP POST → external URL]
    H -->|2xx| I[Persist success log\nmark event delivered]
    H -->|failure| J[Persist failed log\nschedule next attempt]
    J -->|attempt < maxAttempts| K[Update next_run_at\npoller picks up]
    J -->|attempt == maxAttempts| L[Mark event exhausted]
    M[Manual RetryDelivery API] -->|New WebhookEvent row| D
```

**Key properties:**
- `HandleEvent` returns immediately (non-blocking — persists DB row then tries to enqueue)
- Events survive process restarts (stored in `webhook_events` table)
- Atomic `UPDATE WHERE status='pending' SET status='in_flight'` prevents double-delivery across workers
- DB poller recovers stale in-flight events after crashes
- Queue buffer and worker count configurable (defaults: 512 / 4)
- HTTP timeout per attempt is configurable (default: 15 seconds)
- Retry policy resolved at execution time — changing DB config takes effect immediately

---

## Delivery Guarantee

Webhooks are delivered **at-least-once**. Events are persisted to the `webhook_events` table before delivery, so they survive process restarts. Deduplication of duplicate deliveries is the receiver's responsibility. Use the `X-Tyk-Event-ID` header (UUID) and the `attempt_number` field in the delivery log to deduplicate.

---

## SSRF Protection

Webhook URLs are validated at both **write time** (Create/Update) and **delivery time** (defence-in-depth). The validator:
- Rejects empty URLs
- Rejects non-HTTP/HTTPS schemes
- Resolves the hostname via DNS and rejects any IP in private/internal CIDRs:
  - `10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`
  - `127.0.0.0/8` (loopback), `169.254.0.0/16` (link-local)
  - `::1/128`, `fc00::/7` (IPv6 private)

SSRF protection is controlled by `WebhookServiceConfig.AllowInternalNetwork`, which is resolved from the `ALLOW_INTERNAL_NETWORK_ACCESS` environment variable at service construction time. Setting this to `true` disables SSRF checks — useful for local development or internal deployments.

---

## HMAC Signature & Replay Protection

When a `secret` is configured on the subscription, each delivery includes two headers:

```
X-Tyk-Timestamp: 1709123456
X-Tyk-Signature: sha256=<hex-encoded-HMAC-SHA256>
```

The signature is computed over `"<unix_timestamp>.<body>"` — the timestamp is part of the signed material. This prevents replay attacks: an attacker who captures a valid `(timestamp, body, signature)` tuple cannot reuse it once the receiver's tolerance window (typically 5 minutes) has elapsed, because the timestamp embedded in the signed material will be stale.

### Verification (Go)

```go
import (
    "crypto/hmac"
    "crypto/sha256"
    "encoding/hex"
    "fmt"
    "net/http"
    "strconv"
    "time"
)

const maxSkew = 5 * time.Minute

func verifySignature(r *http.Request, body []byte, secret string) error {
    tsStr := r.Header.Get("X-Tyk-Timestamp")
    tsUnix, err := strconv.ParseInt(tsStr, 10, 64)
    if err != nil {
        return fmt.Errorf("missing or invalid X-Tyk-Timestamp")
    }
    if d := time.Since(time.Unix(tsUnix, 0)); d > maxSkew || d < -maxSkew {
        return fmt.Errorf("timestamp outside tolerance window (possible replay)")
    }
    signed := []byte(fmt.Sprintf("%d.%s", tsUnix, body))
    mac := hmac.New(sha256.New, []byte(secret))
    mac.Write(signed)
    expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
    if !hmac.Equal([]byte(r.Header.Get("X-Tyk-Signature")), []byte(expected)) {
        return fmt.Errorf("invalid signature")
    }
    return nil
}
```

### Verification (Python)

```python
import hmac, hashlib, time

MAX_SKEW = 300  # seconds

def verify(secret: str, body: bytes, headers: dict) -> None:
    ts = int(headers["X-Tyk-Timestamp"])
    if abs(time.time() - ts) > MAX_SKEW:
        raise ValueError("stale timestamp — possible replay attack")
    signed = f"{ts}.".encode() + body
    expected = "sha256=" + hmac.new(secret.encode(), signed, hashlib.sha256).hexdigest()
    if not hmac.compare_digest(expected, headers["X-Tyk-Signature"]):
        raise ValueError("invalid signature")
```

If `secret` is empty, no signature or timestamp headers are sent and verification is skipped.

---

## Request Format

Each delivery is an HTTP POST with the following headers:

| Header | Value |
|---|---|
| `Content-Type` | `application/json` |
| `X-Tyk-Timestamp` | Unix epoch of the delivery attempt (omitted if no secret) |
| `X-Tyk-Signature` | `sha256=<hmac>` (omitted if no secret) |
| `X-Tyk-Event-Topic` | Event topic string |
| `X-Tyk-Event-ID` | Event UUID |
| `X-Tyk-Delivery-Attempt` | Attempt number (1-based) |

The body is the JSON-marshalled `eventbridge.Event`. For manual retries, the original payload from the delivery log is replayed verbatim.

Response body is capped (default 4KB, configurable) before storage in the delivery log.

---

## Retry Schedule (Default)

| Attempt | Delay Before Next Retry |
|---|---|
| 1 (initial failure) | 10 seconds |
| 2 | 30 seconds |
| 3 | 2 minutes |
| 4 | 10 minutes |
| 5 (final) | No retry |

Configurable at global level (DB singleton) or per subscription (see `retry_policy` field).

---

## Configuration

### Static Startup Configuration (Environment Variables)

These are set once at startup and serve as the last-resort fallback.

| Env Var | Default | Description |
|---|---|---|
| `WEBHOOK_WORKERS` | `4` | Number of worker goroutines |
| `WEBHOOK_QUEUE_SIZE` | `512` | Job queue buffer size |
| `WEBHOOK_MAX_RETRIES` | `5` | Maximum delivery attempts |
| `WEBHOOK_BACKOFF_SECONDS` | `10,30,120,600,1800` | Comma-separated backoff delays |
| `WEBHOOK_HTTP_TIMEOUT` | `15` | HTTP timeout per attempt (seconds) |
| `WEBHOOK_MAX_RESP_BODY` | `4096` | Max response body bytes to store |
| `ALLOW_INTERNAL_NETWORK_ACCESS` | `false` | Disable SSRF protection (development/internal only) |

### Global Dynamic Configuration (DB Singleton, runtime-updatable)

Managed via `GET/PUT /api/v1/webhooks/config`. Fields with zero values defer to static defaults.

```json
{
  "workers": 0,
  "queue_size": 0,
  "default_retry_policy": {
    "max_attempts": 3,
    "backoff_seconds": [5, 15, 60],
    "timeout_seconds": 10
  },
  "max_response_body_bytes": 0
}
```

### Per-Subscription Override

Set `retry_policy` and/or `transport_config` on any subscription. Zero values in `retry_policy` inherit from the DB global config or static defaults.

```json
{
  "retry_policy": {
    "max_attempts": 1,
    "timeout_seconds": 5
  },
  "transport_config": {
    "proxy_url": "http://proxy.internal:3128",
    "insecure_skip_verify": false,
    "tls_ca_cert": "-----BEGIN CERTIFICATE-----\n...",
    "tls_client_cert": "-----BEGIN CERTIFICATE-----\n...",
    "tls_client_key": "-----BEGIN EC PRIVATE KEY-----\n..."
  }
}
```

**Retry policy merge precedence:** per-subscription > DB global > static startup

---

## API Endpoints

All endpoints require authentication (`Authorization: Bearer <token>`) and admin role.

### Subscription Management

| Method | Path | Description |
|---|---|---|
| `POST` | `/api/v1/webhooks` | Create a new subscription |
| `GET` | `/api/v1/webhooks` | List all subscriptions |
| `GET` | `/api/v1/webhooks/topics` | List all subscribable event topics |
| `GET` | `/api/v1/webhooks/:id` | Get a subscription by ID |
| `PUT` | `/api/v1/webhooks/:id` | Update a subscription |
| `DELETE` | `/api/v1/webhooks/:id` | Delete a subscription (204) |
| `GET` | `/api/v1/webhooks/:id/deliveries` | List delivery logs (`?limit=50`) |
| `POST` | `/api/v1/webhooks/:id/test` | Send a test event |
| `POST` | `/api/v1/webhooks/:id/deliveries/:log_id/retry` | Replay a specific delivery (202) |

### Global Config

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/v1/webhooks/config` | Get global webhook config singleton |
| `PUT` | `/api/v1/webhooks/config` | Update global webhook config singleton |

### Create / Update Payload

```json
{
  "name": "slack-notifications",
  "url": "https://hooks.slack.com/services/...",
  "secret": "my-signing-secret",
  "topics": ["system.llm.created", "system.app.approved"],
  "enabled": true,
  "description": "Notify Slack on key platform events",
  "retry_policy": {
    "max_attempts": 3,
    "timeout_seconds": 10
  },
  "transport_config": {
    "proxy_url": "",
    "insecure_skip_verify": false
  }
}
```

---

## Data Models

### `WebhookRetryPolicy` (value type, embedded in subscription and config)

| Field | Type | Description |
|---|---|---|
| `max_attempts` | int | 0 = use next-level default |
| `backoff_seconds` | []int | nil = use next-level default |
| `timeout_seconds` | int | 0 = use next-level default |

### `WebhookTransportConfig` (value type, embedded in subscription)

| Field | Type | Description |
|---|---|---|
| `proxy_url` | string | HTTP/HTTPS/SOCKS5 proxy for outbound requests |
| `insecure_skip_verify` | bool | Disable TLS certificate validation (self-signed certs) |
| `tls_ca_cert` | string | PEM-encoded CA certificate to trust (private CA) |
| `tls_client_cert` | string | PEM-encoded client certificate for mTLS |
| `tls_client_key` | string | PEM-encoded client private key for mTLS |

### `WebhookSubscription`

| Field | Type | Description |
|---|---|---|
| `id` | uint | Auto-increment primary key |
| `name` | string | Human-readable name |
| `url` | string | Target HTTP endpoint |
| `secret` | string | HMAC signing secret (optional) |
| `topics` | []WebhookTopic | Subscribed event topics (join table) |
| `enabled` | bool | Default: true |
| `description` | string | Optional description |
| `retry_policy` | WebhookRetryPolicy | Per-subscription retry overrides |
| `transport_config` | WebhookTransportConfig | Per-subscription HTTP transport settings |
| `created_at` | time | GORM timestamp |
| `updated_at` | time | GORM timestamp |

### `WebhookTopic` (join table)

| Field | Type | Description |
|---|---|---|
| `id` | uint | Auto-increment primary key |
| `subscription_id` | uint | Foreign key → `webhook_subscriptions.id` (CASCADE delete) |
| `topic` | string | Exact event topic string |

Composite unique index on `(subscription_id, topic)` prevents duplicates.

### `WebhookEvent` (persistent delivery queue)

| Field | Type | Description |
|---|---|---|
| `id` | uint | Auto-increment primary key |
| `subscription_id` | uint | Foreign key (indexed) |
| `event_topic` | string | Topic of the event |
| `event_id` | string | UUID of the event |
| `payload` | text | Full JSON payload to deliver |
| `attempt_number` | int | Current attempt (1-based) |
| `status` | string | `pending` / `in_flight` / `delivered` / `exhausted` |
| `next_run_at` | time | When this event is next due (indexed) |

### `WebhookDeliveryLog` (audit log)

| Field | Type | Description |
|---|---|---|
| `id` | uint | Auto-increment primary key |
| `subscription_id` | uint | Foreign key (indexed) |
| `event_topic` | string | Topic of the delivered event |
| `event_id` | string | UUID of the event |
| `payload` | text | Full JSON payload sent |
| `attempt_number` | int | 1-based attempt counter |
| `status` | string | `success` / `failed` |
| `http_status_code` | int | HTTP response code |
| `response_body` | text | Response body (capped at configured limit) |
| `error_message` | text | Network/HTTP error message |
| `attempted_at` | time | When this attempt was made |
| `next_retry_at` | *time | Scheduled retry time (nil if done) |

### `WebhookConfig` (singleton, ID=1)

| Field | Type | Description |
|---|---|---|
| `id` | uint | Always 1 (singleton) |
| `workers` | int | 0 = use static default |
| `queue_size` | int | 0 = use static default |
| `default_retry_policy` | WebhookRetryPolicy | Global defaults for all subscriptions |
| `max_response_body_bytes` | int | 0 = use static default |

---

## Code References

| Component | File |
|---|---|
| Data models | `models/webhook.go` |
| AutoMigrate registration | `models/models.go` |
| Service (dispatch, HMAC, retry, CRUD, config) | `services/webhook_service.go` |
| Service wiring (struct, SetEventBus, Cleanup) | `services/service.go` |
| Standalone mode + worker startup | `main.go` |
| HTTP handlers | `api/webhook_handlers.go` |
| Route registration | `api/api.go` |
| Static config env vars | `config/config.go` |
| Known topics list | `services/system_events.go` |
| Unit tests | `services/webhook_service_test.go` |

---

## Testing Strategy

### Unit Tests (`services/webhook_service_test.go`)

- **`TestTopicMatching_Exact`** — exact topic matches; wrong topic does not
- **`TestComputeHMAC`** — deterministic for same inputs; differs when timestamp differs
- **`TestComputeHMAC_EmptySecret`** — empty secret returns empty string
- **`TestDefaultBackoffDuration`** — default config backoff values match spec
- **`TestValidateWebhookURL_Empty`** — empty URL rejected
- **`TestValidateWebhookURL_BadScheme`** — non-HTTP/HTTPS scheme rejected
- **`TestValidateWebhookURL_PrivateLoopback`** — loopback address rejected by SSRF check
- **`TestValidateWebhookURL_AllowInternal`** — loopback allowed when `AllowInternalNetwork=true`
- **`TestHandleEvent_PersistsQueue`** — HandleEvent creates WebhookEvent row in DB
- **`TestDelivery_Success`** — mock server 200; event `status=delivered`, log `status=success`
- **`TestDelivery_Failure`** — mock server 500; event `status=exhausted`, log `status=failed`
- **`TestDelivery_Retry`** — server 500 then 200; second log row `status=success`
- **`TestDelivery_MaxRetries`** — server always 503; exactly N log rows, event `status=exhausted`
- **`TestTestWebhook_Success`** — smoke-tests TestWebhook against a 200 server
- **`TestTestWebhook_Failure`** — TestWebhook against a 403 server returns error
- **`TestWebhookCRUD`** — create, get, list, update, delete subscription
- **`TestRetryPolicy_Resolution_StaticDefaults`** — zero-value sub uses static defaults
- **`TestRetryPolicy_Resolution_DBGlobal`** — DB global overrides static defaults
- **`TestRetryPolicy_Resolution_PerSubscriptionOverride`** — sub override wins over DB global
- **`TestRetryDelivery`** — manual retry creates new WebhookEvent; delivery succeeds
- **`TestRetryDelivery_NotFound`** — unknown log ID returns error
- **`TestWebhookConfig_DefaultsAndUpdate`** — singleton read/write round-trip
- **`TestListWebhooks`** — list returns all subscriptions
- **`TestWebhookTransportConfig_Proxy`** — invalid proxy URL rejected; valid proxy builds client
- **`TestWebhookTransportConfig_InsecureSkipVerify`** — InsecureSkipVerify propagated to transport
- **`TestWebhookTransportConfig_InvalidCACert`** — malformed PEM CA cert returns error

### Manual Smoke Test

```bash
# 1. List available topics
curl http://localhost:8080/api/v1/webhooks/topics \
  -H "Authorization: Bearer <token>"

# 2. Create a subscription
curl -X POST http://localhost:8080/api/v1/webhooks \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "test",
    "url": "https://webhook.site/your-id",
    "secret": "mysecret",
    "topics": ["system.llm.created"],
    "enabled": true
  }'

# 3. Trigger an event (e.g. create an LLM via the admin UI or API)

# 4. Check delivery logs
curl http://localhost:8080/api/v1/webhooks/1/deliveries \
  -H "Authorization: Bearer <token>"

# 5. Manually retry a specific delivery
curl -X POST http://localhost:8080/api/v1/webhooks/1/deliveries/5/retry \
  -H "Authorization: Bearer <token>"

# 6. View / update global config
curl http://localhost:8080/api/v1/webhooks/config \
  -H "Authorization: Bearer <token>"

curl -X PUT http://localhost:8080/api/v1/webhooks/config \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"default_retry_policy":{"max_attempts":3,"backoff_seconds":[5,30,120]}}'
```
