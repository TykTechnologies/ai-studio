# Tyk MCP analytics

Calls, error rates, latency, per-tool usage and per-App attribution for MCP
servers proxied by Tyk Gateway, on both Tyk connection kinds (Dashboard and
OSS Gateway). Studio never sees that traffic, so it takes the Gateway's own
analytics records, delivered by Tyk Pump to an ingest endpoint.

Enterprise Edition. Epic `midsommar-clean-bv0`; plan
`~/.claude/plans/i-would-like-you-recursive-nebula.md`.

## 1. Data path

```
Tyk Gateway 5.14+ ──Redis──▶ Tyk Pump ──HTTP POST──▶ Studio /api/v1/ingest/tyk-analytics
 (enable_analytics)           http pump, or           Bearer <connection ingest token>
                              stdout pump + Vector    JSON array of AnalyticsRecord (gzip ok)
                                                              │
                                     tykmcp.AuthenticateIngest (token → connection)
                                     tykmcp.IngestAnalytics    (filter, resolve, attribute)
                                                              │
                                     analytics.RecordMCPCalls ──▶ mcp_call_records (+ Tee sinks)
```

- The Gateway writes one `AnalyticsRecord` per MCP request with an
  `mcp_stats` block (`is_mcp`, `jsonrpc_method`, `primitive_type`,
  `primitive_name`, `jsonrpc_error_code`, effective and declared protocol
  versions). Denied and rejected calls are recorded too.
- Pump drains Redis destructively, so Studio is a Pump sink, never a second
  Redis reader.
- Decision (2026-10-20): an HTTP ingest endpoint plus a generic `http` pump
  upstream in tyk-pump (bead bv0.8). No MDCB/hybrid protocol emulation. Until
  that pump ships, the stdout pump (`format: json`) feeds Vector or Fluent
  Bit, which posts batches to the same endpoint. The stdout pump wraps each
  record under its log field (`tyk-analytics-record` by default); the
  shipper must unwrap it.

## 2. Storage (core)

- `mcp_call_records` (`models.MCPCallRecord`, in `models.AnalyticsModels()`):
  timestamp, connection, server (nullable), Tyk API and org ids, App and
  credential (nullable), JSON-RPC method, primitive type and name, JSON-RPC
  error code, HTTP status, total/upstream/gateway latency, bytes in/out,
  protocol version, MCP session id (the `mcp-session-id-` tag), trace id (the
  `trace-id-` tag) and source. Indexes: `(mcp_server_id, timestamp)`,
  `(app_id, timestamp)`, `(connection_id, timestamp)`, `timestamp`.
- No column holds a raw key, a request or response body, or a client IP. The
  decoder (`services/tykmcp.TykAnalyticsRecord`) does not even declare those
  fields. `ClampLengths` cuts strings to their column sizes and drops invalid
  UTF-8 so one hostile value cannot fail a whole batch on Postgres.
- `tyk_connection_ingests`: one row per connection: the ingest token's
  SHA-256 (nullable, unique), a display prefix, who created it and when, the
  last batch time and the last unknown API id.
- `tyk_ingest_stats`: hourly counters per connection (received, stored,
  skipped non-MCP, unknown API, unattributed). One upsert per batch.
- Schema versions 12 (`mcp_call_records`) and 13 (the two ingest tables).

## 3. Recorder (core)

The optional interface `analytics.MCPCallRecorder`
(`RecordMCPCalls(ctx, []models.MCPCallRecord) error`) and the package helper
`analytics.RecordMCPCalls`. It is not part of `AnalyticsHandler`, so hosts'
own handlers compile unchanged; a handler without it does not receive MCP
calls, and the helper then drops the batch (logged once) rather than have
the pump retry something that can never succeed.

The `DatabaseHandler` writes MCP batches through a writer goroutine of its
own, so a burst never delays the chat records budgets are computed from.
When its buffer is full (or it has stopped) it returns `ErrRecorderBusy`;
the ingest endpoint answers 503 and the pump retries. `analytics.Tee`
forwards copies to the host sinks that implement the interface
(`pkg/studio` `AnalyticsSinks`), only after the primary has taken the batch.
The microgateway and `pkg/aigateway` handlers do not implement it: ingest
happens on the control plane only.

## 4. Ingest endpoint (core)

`POST /api/v1/ingest/tyk-analytics`, on the public router group and exempt
from CSRF (it never uses a cookie). `api/tyk_ingest_handlers.go`.

| Case | Status |
|---|---|
| Accepted | 202 `{accepted, stored, skipped_non_mcp, unknown_api, unattributed}` |
| Missing, malformed, unknown or revoked token; disabled or deleted connection | 401 + `WWW-Authenticate: Bearer` (the pump drops the batch) |
| Not a JSON array, bad gzip | 400 |
| Body over 16 MiB as sent, or over 16 MiB after gzip expansion; over 5000 records | 413 |
| `Content-Encoding` other than gzip/identity | 415 |
| Over 20 requests/s (burst 40) for the connection | 429 + `Retry-After` |
| The analytics recorder cannot take the batch (buffer full) | 503 + `Retry-After` (the pump retries) |
| Community Edition, or the integration disabled | 404 |

The token is resolved before the body is read. The decoder streams the array
and stops at the record cap.

## 5. Service contract (core) and ingester (enterprise)

`services/tykmcp.Service`: `AuthenticateIngest`, `IngestAnalytics`,
`RotateIngestToken`, `RevokeIngestToken`, `GetIngestStatus`. The CE stub
returns `ErrEnterpriseFeature`. Enterprise: `enterprise/features/tykmcp/analytics.go`.

- **Token**: `tai_ing_` plus 32 random bytes (base64url). Stored as SHA-256
  hex only; looked up by hash, then compared in constant time. Rotation
  replaces the hash (the old token stops at once); revoke clears it. Both
  are audited with the display prefix only. Deleting a connection deletes its
  ingest row and counters.
- **Filter**: records without `mcp_stats.is_mcp` count as `skipped_non_mcp`.
- **Server**: `(connection_id, tyk_api_id)` against live `mcp_servers`, one
  `IN` query per batch, scoped to the token's connection. A Dashboard
  connection with an `org_id` also drops records of another org. Misses count
  as `unknown_api` and are not stored; the last one is kept for the UI.
- **App**: one credential query per batch on the connection: by
  `tyk_key_hash` (the Gateway's analytics `api_key` is the same hash when
  `hash_keys` is on), by the decrypted unhashed key id (Gateways with
  `hash_keys` off report the raw key), else by the key alias
  `<alias_prefix>app:<id>`, trusted only for an App that holds (or held) a
  credential on this connection. A key match sets App and credential; an
  alias match sets the App only; otherwise the call is stored unattributed.
  The key is never copied into a row.
- Every replica accepts batches; all state is in the shared database.

## 6. Statistics (core)

`analytics/mcp_stats.go`. Error = HTTP status >= 400 or a non-zero JSON-RPC
error code. Latency percentiles are interpolated: `percentile_cont` on
Postgres, window functions on SQLite (tested to give the same values). All
filtering, grouping and paging is in SQL; percentiles are computed only for
the rows of the page.

Admin routes (permission `analytics` read), each taking `start_date` and
`end_date` (YYYY-MM-DD, end inclusive):

| Route | Returns |
|---|---|
| `GET /api/v1/analytics/mcp/overview` | summary + top 10 tools across servers |
| `GET /api/v1/analytics/mcp/servers/:id/summary` | calls, errors, error rate, avg/p50/p95 ms, last seen |
| `GET /api/v1/analytics/mcp/servers/:id/timeseries?interval=hour\|day` | points with calls, errors, p50, p95 (empty buckets omitted) |
| `GET /api/v1/analytics/mcp/servers/:id/tools?page&page_size&search` | paged per-primitive table |
| `GET /api/v1/analytics/mcp/servers/:id/apps?page&page_size&search` | paged per-App table, unattributed as `app_id: null` |
| `GET /api/v1/analytics/mcp/apps/:id` | the App's servers |

Portal: `GET /common/apps/:id/analytics/mcp`, the App's servers, for the
App's owner only (403 otherwise), the same rule as the App's other portal
analytics.

Ingest administration (under the Tyk connection):

| Route | Permission |
|---|---|
| `GET /api/v1/tyk-connections/:id/ingest-status` | `tyk-connections` read |
| `POST /api/v1/tyk-connections/:id/ingest-token` (plaintext once, `Cache-Control: no-store`) | `tyk-connections` write |
| `DELETE /api/v1/tyk-connections/:id/ingest-token` | `tyk-connections` write |

## 7. Known limits

- At-least-once delivery: records carry no id, so a retried batch can be
  counted twice.
- Per-App attribution needs key-backed (brokered) servers. OAuth, JWT, mTLS
  and keyless servers get per-server statistics only.
- The Gateway does not record a tool result's `isError`, the JSON-RPC id or
  the client name. JSON-RPC errors are detected in the first 64 KiB of the
  response only, and on Gateway 5.15.1 an upstream's JSON-RPC error (an
  unknown tool from an mcp-go server) arrives as HTTP 200 with code 0
  (live E2E, bv0.9). The error rate is therefore mostly HTTP and
  Gateway-generated errors.
- The Gateway writes analytics for an OAS API only when
  `x-tyk-api-gateway.middleware.global.trafficLogs.enabled` is true. Studio
  sets it on the proxies it registers; operator-created proxies need it set
  by the operator.
- Calls without a key (keyless APIs, and calls the Gateway refused for a
  missing or bad key) carry `api_key` `00000000` and an empty alias: they are
  stored unattributed, and their primitive name is whatever the client sent.
- `initialize` and `tools/list` arrive with an empty primitive type and the
  method as the primitive name; they show as rows of their own in the
  per-tool table.
- Pump 1.16's stdout pump omits zero-valued `mcp_stats` fields; the decoder
  defaults them.
- Gateway 5.15.1 writes no analytics record for a `tools/call` refused by the
  key's `mcp_access_rights` (HTTP 403, JSON-RPC -32002), so tools blocked by
  Studio key access never appear (bead emn). Auth refusals (401 no key, 403
  bad key) are recorded, with `api_key` `00000000` and no App.
- Records that arrive after their server was deleted count as `unknown_api`
  and are dropped (seen for calls in the seconds before a delete).
- Bridge: Vector's `docker_logs` source reads from when it attaches, so Pump
  output written while Vector is down is lost (Pump has already drained
  Redis).
- Pump 1.16.0 crashes at start unless `dont_purge_uptime_data` is true (the
  uptime pump defaults to Mongo).
- REST-as-MCP proxies report an upstream latency of 0; use the total.
- Without `mcp_only` (http pump) or `filters.api_ids`, every API's records
  travel to Studio and are dropped on arrival.

## 8. Verification

- Unit and API tests on SQLite and Postgres 16: `analytics/mcp_*_test.go`,
  `api/tyk_ingest_*_test.go`, `api/mcp_analytics_handlers_test.go`,
  `enterprise/features/tykmcp/analytics*_test.go` (including a live capture
  of 12 records, `testdata/mcp_analytics_live_records.json`), and the
  enterprise core tests for the ingest flow and token RBAC.
- Live E2E (`tests/tykmcp/oss/live_e2e.py`, 2026-10-20): ENT Studio against
  two Tyk OSS Gateway 5.15.1 nodes and Redis. 86/86 checks with
  `PUMP_PROFILE=analytics` (Tyk Pump 1.16.0 stdout pump + Vector 0.49.0,
  `tests/tykmcp/oss/vector.toml`) and 86/86 with `PUMP_PROFILE=httppump`
  (local tyk-pump build with the http pump, `feat/http-pump`). Checks 2b
  (ingest token generated and shown once, wrong token 401, Pump profile
  starts) and 6b (per-tool counts: get-weather 7 with no errors, initialize
  7 with the keyless 401 as one error, tools/list 3; latency; App
  attribution by key hash; App usage lists the server; an operator proxy
  without trafficLogs records nothing; REST traffic never reaches Studio;
  the ingest status counts the records).

## 9. Follow-ups

- emn: tools refused by key access rights are invisible (Gateway gap).

- bv0.11 retention pruning for `mcp_call_records` (and old `tyk_ingest_stats` hours).
- bv0.12 MDCB-embedded host hook (headless control plane passes
  `PurgeAnalyticsData` batches to the same ingester).
- bv0.13 Dashboard `/api/activity/mcp` backfill for Dashboard connections.
- bv0.14 ingesting pre-aggregated MCP records for high-volume sites.
