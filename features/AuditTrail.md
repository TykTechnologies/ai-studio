# Audit Trail (ENTERPRISE EDITION ONLY)

## Introduction

**NOTE: The audit trail is an Enterprise Edition feature. Community Edition exposes the same API routes, which answer `403 Enterprise Feature`, and records nothing.**

The audit trail is an append-only record of every action taken through the management API: actor, source IP, action, resource, status, and a field-level diff of what changed. Its purpose is incident reconstruction: a security team must be able to answer "who changed this object, when, from where, and what did it look like before" for any object on the platform.

The record schema and storage options deliberately mirror the Tyk Dashboard audit log (`req_id`, `ip`, `user`, `action`, `method`, `url`, `status`, `diff`, `request_dump`, `response_dump`; `store_type` of `db` or `file`; `detailed_recording`), because that design has already been validated against enterprise security requirements.

User documentation: `docs/site/docs/audit-trail.md`.

---

## Edition Comparison

### Community Edition
- ❌ No recording
- ✅ `GET /api/v1/audit/status` reports `available: false`
- ❌ All other `/api/v1/audit/*` routes return `403`
- ✅ `audit_records` table is migrated (empty) so the schema is identical across editions

### Enterprise Edition
- ✅ Recording of every mutating and authentication request to the management API
- ✅ Optional recording of reads
- ✅ Field-level diffs with secret redaction
- ✅ Database and/or append-only file storage
- ✅ Retention enforcement
- ✅ Query, summary, export and per-object history API
- ✅ Admin UI page (Governance → Audit trail)

---

## System Architecture

```
HTTP request
   │
   ▼
gin.Engine
   ├─ Recovery
   ├─ Telemetry middleware (licensing)
   ├─ Audit middleware  ◄──── registered in NewAPI before setupRoutes, so it
   │     │                    wraps every route including auth and NoRoute
   │     ├─ classify(method, c.FullPath())  → action, resource type, id param
   │     ├─ snapshot(before)                → row as map, for PUT/PATCH/DELETE/POST-with-id
   │     ├─ c.Next()                        → auth middleware + handler run
   │     ├─ resolve actor from c.Get("user") (or login body on failure)
   │     ├─ snapshot(after), computeDiff, redact
   │     └─ Record() → buffered channel (never blocks)
   ├─ CSRF, CORS, ...
   └─ route handler
                                   background writer goroutine
                                   ├─ batches of 100 / every 250ms
                                   ├─ DB: CreateInBatches(audit_records)
                                   └─ file: append JSON line / text line
                                   retention goroutine (hourly)
                                   └─ DELETE WHERE timestamp < now - N days
```

### Layers

| Layer | Location | Edition |
|---|---|---|
| Model | `models/audit_record.go` (`AuditRecord`, table `audit_records`, `RawJSON` helper) | Both |
| Config | `config/config.go` (`AuditConfig`, `AUDIT_*` env parsing) | Both |
| Service contract | `services/audit/interface.go`, `factory.go`, `community.go` | Both |
| Implementation | `enterprise/features/audit/` (`service.go`, `middleware.go`, `actions.go`, `diff.go`) | ENT |
| API | `api/audit_handlers.go`, routes in `api/api.go` | Both |
| UI | `ui/admin-frontend/src/admin/pages/AuditTrail.js` | Both (gated) |

The `API` struct holds `auditService` and a cached `auditHandler`. The middleware registered on the engine dispatches through `auditHandler` at request time, which lets `SetAuditService` swap the implementation after construction (tests do this). In `TestMode` recording defaults to off unless `AUDIT_ENABLED` is set, so unrelated test suites do not write audit rows.

---

## Key Components

### Model (`models.AuditRecord`)

Plain struct, not `gorm.Model`: no soft-delete column, no update path. Indexed on `timestamp`, `request_id`, `ip`, `user_id`, `user_email`, `action`, `method`, `route`, `status`, and `(resource_type, resource_id)`.

`Diff`, `RequestDump` and `ResponseDump` are `models.RawJSON` (a string column that marshals to the API as the JSON value itself, or `null` when empty).

### Route classification (`actions.go`)

`classify(method, fullPath)` turns the matched gin route template into `{Action, ResourceType, ResourceKey, ResourceParam, Auth, IsCreate}`.

1. An explicit table keyed `"METHOD /template"` covers authentication routes and operations whose generic name would read badly (`Roll User API Key`, `Reset App Budget`, `Clear Plugin Data`, ...).
2. Everything else is derived from route shape after stripping the prefix (`/api/v1/`, `/common/`, `/api/v1/admin/`, ...):

| Shape | Action |
|---|---|
| `[res]` | `Create R` / `List R` |
| `[res, :id]` | `View R` / `Update R` / `Delete R` |
| `[res, literal]` | `<Literal> R` (GET) / `Create R <Literal>` |
| `[res, :id, verb]` | `<Verb> R` (activate, reload, approve, clone, ...) |
| `[res, :id, noun]` | `Add <Noun> To R` / `Remove <Noun> From R` / `Update R <Noun>` |
| `[res, :id, noun, :param]` | `Add <Noun> To R` / `Remove <Noun> From R` |
| other | `<Verb> R <Literals...>` |

`ResourceType` is the snake_case singular of the collection segment (`data-catalogues` → `data_catalogue`). Adding a route to the router needs no audit change unless its generic name is unsatisfactory, in which case add an entry to `explicitRoutes`.

### Snapshots and diffs (`diff.go`)

`snapshotTargets` maps a collection segment to the GORM model backing it (plus the id column when it is not `id`, e.g. edges by `edge_id`, SSO profiles by `profile_id`). `snapshot()` loads the row as `map[string]interface{}` through `db.Model(m).Where(col = ?).Take(&row)`, so soft-delete scoping and `TableName()` overrides apply automatically.

Redaction rules live in a `redactor` built once per service from the built-in lists plus `AuditConfig.RedactKeys` / `RedactHeaders`; operator additions extend, never replace, the built-ins.

`redactor.computeDiff(before, after)`:
- normalises driver values (`[]byte`, `time.Time`, integers) so SQLite and PostgreSQL produce the same diff;
- skips `created_at`, `updated_at`, `deleted_at`, `session_token`, heartbeat columns;
- redacts sensitive keys (by name fragment) but still reports them as changed;
- parses JSON-valued string columns and redacts nested keys;
- caps output at `MaxBodyBytes`, dropping the largest fields into `_truncated_fields`.

Create → `old: null` for every column. Delete → `new: null` for every column. Update → changed columns only.

### Middleware (`middleware.go`)

Path policy: audited prefixes `/api/v1/`, `/common/`, `/auth/`, `/oauth/`, `/api/sso`, `/analytics/`. Always skipped: `/auth/config`, `/auth/features`, `/common/system`, `/common/me`, `/csrf-token`, chat/session/history paths, notification polling, plugin assets, anything containing `/stream`, and agent message traffic (agent *configuration* CRUD under `/api/v1/agents` is re-admitted). Requests that match no route (SPA fallback) are ignored.

Method policy: `POST/PUT/PATCH/DELETE` always; `GET` only when `RecordReads` or the route is an auth event; `OPTIONS`/`HEAD` never.

Body handling: the request body is buffered only for detailed recording or auth events (to capture the attempted email on a failed login), only for JSON/form content types, and only up to 1 MB; it is always restored for the handler. The response is teed through a bounded `bodyCapture` writer for every recorded request, because that is where created ids (`data.id`) and error messages live.

Actor: `c.Get("user")` after the chain; on auth routes without a user, the email from the login payload. `Login`/`SSO Login` with status ≥ 400 becomes `Login Failed`/`SSO Login Failed`.

Request id: inbound `X-Request-ID` (≤ 64 chars, no whitespace/quotes) or a UUID; set on the response (Go canonicalises it to `X-Request-Id`) and in the gin context as `audit_request_id`.

Panics: the middleware defers a recover that records the request as `500` with `error: "panic: ..."` and then re-panics so `gin.Recovery` (registered above it) still produces the 500 response. Without this a crashed handler would leave no trace, because Recovery unwinds past the audit frame.

### Service (`service.go`)

- `Record()` enqueues; on a full queue it increments `dropped` and returns an error without blocking.
- One writer goroutine batches (100 records or 250 ms) into `CreateInBatches` and/or the file sink; `Stop()` cancels, drains, flushes and closes the file. `API.Shutdown()` calls it.
- Retention goroutine runs 1 minute after start and then hourly when `StoreType` includes `db` and `RetentionDays > 0`.
- `List` omits dumps; `Get` includes them. `Summary` uses `date(timestamp)` which works on both PostgreSQL and SQLite; the day is scanned as a string (SQLite returns text).
- `Export` streams up to 50,000 rows using keyset pagination on `(timestamp, id)` so equal timestamps cannot skip or duplicate rows. CSV cells starting with `=`, `+`, `-`, `@`, tab or CR are prefixed with `'` (`csvSafe`) to neutralise spreadsheet formula injection.
- File-only store type makes queries return `ErrDisabled` (HTTP 409); the UI explains this.

---

## API Integration

Routes are registered on the admin-only `v1` group in `api/api.go`:

```
GET /api/v1/audit/status
GET /api/v1/audit/records
GET /api/v1/audit/records/:id
GET /api/v1/audit/summary
GET /api/v1/audit/export
GET /api/v1/audit/resources/:type/:id
```

Filters are parsed once by `parseAuditQuery` (dates as `YYYY-MM-DD` or RFC3339, bounded string lengths, validated status/page values). Errors map: `ErrEnterpriseFeature` → 403, `ErrDisabled` → 409, `ErrNotFound` → 404, anything else → 500.

---

## Configuration Requirements

| Variable | Default | Notes |
|---|---|---|
| `AUDIT_ENABLED` | `true` | |
| `AUDIT_STORE_TYPE` | `db` | `db`, `file`, `both` |
| `AUDIT_FILE_PATH` | `./data/audit/audit.log` | |
| `AUDIT_FILE_FORMAT` | `json` | `json`, `text` |
| `AUDIT_DETAILED_RECORDING` | `false` | |
| `AUDIT_RECORD_READS` | `false` | |
| `AUDIT_RETENTION_DAYS` | `90` | `0` = forever |
| `AUDIT_MAX_BODY_BYTES` | `65536` | |
| `AUDIT_QUEUE_SIZE` | `4096` | |
| `AUDIT_REDACT_KEYS` | empty | comma-separated extra key fragments |
| `AUDIT_REDACT_HEADERS` | empty | comma-separated extra header names |

---

## Testing

- `enterprise/features/audit/*_test.go` (`go test -tags enterprise ./features/audit/` from `enterprise/`): route classification, diff/redaction rules including operator additions, queue/flush/drop behaviour, list/summary/export/retention on SQLite, CSV formula escaping, file sink, and an end-to-end gin router (with `gin.Recovery` in production order) exercising create/update/delete/login/panic/detailed-recording through the middleware. Passes under `-race`.
- `enterprise/features/audit/service_postgres_test.go`: the same query, summary, export, retention and middleware-diff paths on PostgreSQL. Skipped unless `DATABASE_URL` is set (e.g. `docker run --rm -e POSTGRES_PASSWORD=postgres -e POSTGRES_DB=audit_test -p 55432:5432 postgres:16-alpine`).
- `api/audit_handlers_test.go` (enterprise tag): handlers through the real `NewAPI` router, including a real `POST /api/v1/users` producing a redacted create diff and confirming reads are not recorded.
- `api/audit_handlers_community_test.go` (no tag): CE returns 403 and records nothing.
- `ui/admin-frontend/src/admin/pages/AuditTrail.test.js`: upsell, disabled and file-only states, record rendering, row expansion with diff, filter application.
- Live smoke (manual, needs a license): boot the ENT binary from the repo root with `AUDIT_STORE_TYPE=both AUDIT_DETAILED_RECORDING=true`, then register → failed login → login → anonymous 401 → create/update/delete an LLM with rotated secrets → create a non-admin → logout, and verify the trail via `/api/v1/audit/*`, the CSV export and the file sink contain the actions, diffs and redactions and none of the secret values.

Each enterprise test opens its own named shared-cache SQLite memory database so the background writer's pooled connections see the same data without tests seeing each other.

---

## Potential Enhancements

- Record control-plane operations that do not arrive over HTTP (gRPC edge heartbeat-driven config pushes, scheduled plugin runs) via `Service.Record()`, which already exists for this purpose.
- Per-object "History" tab on LLM/App/User detail pages backed by `GET /audit/resources/{type}/{id}`.
- Webhook/SIEM push of audit records themselves in addition to the file sink.
  (Platform events are already pushed by the Webhooks feature, see
  `features/Webhooks.md`; the worker writes dead letters into this trail
  through `Service.Record` as `SYSTEM` records, and every approval,
  rejection, revocation and edit of a webhook target is classified in
  `actions.go` with a diff of the `webhook_targets` row.)
- License entitlement gating (`audit_trail` feature flag) in addition to edition gating.
- Namespace scoping of records when multi-tenant namespaces reach the management API.
