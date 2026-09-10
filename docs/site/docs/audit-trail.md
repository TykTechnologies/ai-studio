# Audit Trail (Enterprise)

The Audit Trail is an append-only record of every action taken through the AI Studio management API: who did it, when, from which IP address, what it touched, whether it succeeded, and, for updates, exactly which fields changed. It exists so a security team can walk back an incident and reconstruct the history of any object on the platform.

The design mirrors the [Tyk Dashboard audit log](https://tyk.io/docs/api-management/logs/audit-logs), which has been in production against enterprise requirements for years. If you already consume Tyk Dashboard audit records, the field names will look familiar.

Available in **Enterprise Edition** only. Community Edition ships the API surface (it answers `403 Enterprise Feature`) and records nothing.

---

## What is recorded

One record per request to the management surfaces (`/api/v1/*`, `/common/*`, `/auth/*`, `/oauth/*`, `/analytics/*`). By default that means:

| Recorded by default | Not recorded by default |
|---|---|
| Every `POST`, `PUT`, `PATCH`, `DELETE` (create, update, delete, activate, reload, approve, ...) | `GET` reads (enable with `AUDIT_RECORD_READS=true`) |
| Login, logout, failed login, registration, password reset, email verification | Chat and agent message traffic (already in chat history; it is user content, not administration) |
| SSO login start and callback, OAuth authorize / token / consent | Notification polling, plugin asset delivery, streaming endpoints |
| Rejected requests (`401`, `403`, `404`, `5xx`) with the error message, including handler panics recorded as `500` | LLM proxy traffic (see [Analytics](./analytics.md) and [Compliance Events](./compliance-events.md)) |

Recording happens after the handler completes, off the request path: records are queued in memory and written by a background worker in batches. A slow database never slows an admin action, and if the queue ever fills, records are dropped and counted rather than blocking (the dashboard shows a warning when that happens).

### Record fields

| Field | Description |
|---|---|
| `id` | Database identifier. |
| `req_id` | Request identifier. An inbound `X-Request-ID` header is honoured; otherwise one is minted. It is always echoed back on the response so a client can quote it. |
| `timestamp` | When the request started (UTC). |
| `ip` | Client IP (respects `X-Forwarded-For` from trusted proxies). |
| `user`, `user_id`, `user_name` | The authenticated user. For a failed login the attempted email is recorded with `user_id` 0. |
| `user_agent` | The client's user agent. |
| `action` | Human-readable action, e.g. `Update LLM`, `Delete User`, `Add User To Group`, `Roll User API Key`, `Login Failed`, `SSO Login`. |
| `method`, `url`, `route` | HTTP method, the full request URI, and the matched route template (`/api/v1/llms/:id`). |
| `status` | HTTP response status. |
| `resource_type`, `resource_id`, `resource_name` | The object touched, e.g. `llm` / `12` / `gpt-4-prod`. The name is resolved from the row (name, email, variable name, ...) so a deleted object is still identifiable. |
| `diff` | Field-level changes. See below. |
| `error` | The error message from the response body when the request failed. |
| `duration_ms` | Handler time. |
| `request_dump`, `response_dump` | Redacted headers and body. Only populated when `AUDIT_DETAILED_RECORDING=true`. |

### Diffs

For any resource backed by a database row, the middleware snapshots the row before the handler runs and again after it completes, and stores the changed columns:

```json
{
  "name":    { "old": "gpt-4",  "new": "gpt-4-turbo" },
  "api_key": { "old": "[REDACTED]", "new": "[REDACTED]" }
}
```

* **Updates** store only the columns that changed.
* **Creates** store every column as `old: null`.
* **Deletes** store every column as `new: null`, so the final configuration of a deleted object survives its deletion.
* Timestamps (`created_at`, `updated_at`, `deleted_at`) and session tokens are excluded.
* A diff larger than `AUDIT_MAX_BODY_BYTES` drops its largest fields and lists them under `_truncated_fields`.

### Redaction

Secrets never reach the trail. Any column or JSON key whose name contains `password`, `secret`, `api_key`, `token`, `auth_key`, `conn_string`, `private_key`, `credentials`, `authorization` or `cookie` is replaced with `[REDACTED]`, but a *changed* secret is still reported as changed. Extend the list for your own field names with `AUDIT_REDACT_KEYS` (and `AUDIT_REDACT_HEADERS` for headers); the built-ins always stay in force. JSON-valued columns (plugin configuration, SSO provider settings) are parsed and walked, so nested secrets are redacted too. The `value` column of the secrets table is always redacted. In detailed recording, `Authorization`, `Cookie`, `Set-Cookie` and `X-CSRF-Token` headers are masked.

---

## Configuration

All settings are environment variables on the AI Studio server.

| Variable | Default | Description |
|---|---|---|
| `AUDIT_ENABLED` | `true` | Turn recording on or off. |
| `AUDIT_STORE_TYPE` | `db` | `db` (queryable from the API and dashboard), `file` (append-only log file, not queryable), or `both`. |
| `AUDIT_FILE_PATH` | `./data/audit/audit.log` | Log file for the `file` and `both` store types. Mount it on a volume in containers. |
| `AUDIT_FILE_FORMAT` | `json` | `json` (one object per line) or `text` (one line per record). |
| `AUDIT_DETAILED_RECORDING` | `false` | Also store redacted request and response headers and bodies. Increases storage significantly. |
| `AUDIT_RECORD_READS` | `false` | Also record `GET` requests. The admin UI polls several read endpoints, so expect volume. |
| `AUDIT_RETENTION_DAYS` | `90` | Delete database records older than this, checked hourly. `0` keeps records forever. |
| `AUDIT_MAX_BODY_BYTES` | `65536` | Cap on each stored diff, request dump and response dump. |
| `AUDIT_QUEUE_SIZE` | `4096` | In-memory write queue between the request path and the background writer. |
| `AUDIT_REDACT_KEYS` | empty | Comma-separated additions to the built-in list of secret-bearing column / JSON key fragments, e.g. `ssn,customer_ref`. Case-insensitive substring match. Built-ins cannot be removed. |
| `AUDIT_REDACT_HEADERS` | empty | Comma-separated additions to the headers masked in detailed dumps, e.g. `X-Tenant-Key`. |

Records average around 700 bytes without detailed recording and 1 to 3 KB with it. Plan retention and database storage accordingly, and archive the log file externally if you use the `file` store for long-term evidence.

---

## Dashboard

**Governance → Audit trail** in the admin UI lists records newest first with:

* a date range, free-text search, and filters for user, action, resource type, HTTP method and status class;
* summary tiles for total actions, failures and distinct users in the range;
* an expandable row per record showing the request id, route, resource, error, the field-level diff, and (with detailed recording) the request and response dumps;
* CSV and JSON export of the current filter (up to 50,000 records). CSV cells that a spreadsheet would evaluate as a formula (`=`, `+`, `-`, `@` prefixes) are escaped, since attacker-influenced values such as URLs and error messages end up in this file.

---

## API

All endpoints require an admin session and live under `/api/v1/audit`. They are read-only: there is no API to modify or delete a record.

| Endpoint | Description |
|---|---|
| `GET /audit/status` | Availability, whether recording is on, store type, retention, queue depth and dropped count. |
| `GET /audit/records` | Paged list. Returns `{records, total, page, page_size}`. Dumps are omitted from list rows. |
| `GET /audit/records/{id}` | One record including dumps. |
| `GET /audit/summary` | Counts by action, user, resource type and status class plus a per-day timeline, for the same filters. |
| `GET /audit/export?format=csv\|json` | Download matching records. |
| `GET /audit/resources/{type}/{id}` | Every recorded action on one object, newest first. |

### Filters

`records`, `summary`, `export` and `resources` accept the same query parameters:

| Parameter | Description |
|---|---|
| `start_date`, `end_date` | `YYYY-MM-DD` or RFC3339. A date-only end is inclusive of that day. |
| `user` | Case-insensitive substring of the user email. |
| `user_id` | Exact user id. |
| `action` | Exact action name, e.g. `Update LLM`. |
| `resource_type`, `resource_id` | Exact match. |
| `method` | HTTP method. |
| `status` | Exact HTTP status. |
| `status_class` | `2xx`, `3xx`, `4xx` or `5xx`. |
| `ip` | Exact client IP. |
| `req_id` | Exact request id. |
| `search` | Case-insensitive substring over action, URL, user, resource name, IP and request id. |
| `page`, `page_size` | 1-based page and size (default 50, max 500). |
| `sort` | `desc` (default) or `asc`. |

### Example

Who changed the production LLM configuration last week, and what did they change?

```bash
curl -s -H "Authorization: Bearer $ADMIN_API_KEY" \
  "https://studio.example.com/api/v1/audit/resources/llm/12?start_date=2026-09-01&end_date=2026-09-07" | jq '.records[] | {timestamp, user, action, status, diff}'
```

```json
{
  "timestamp": "2026-09-03T14:22:07Z",
  "user": "ops@example.com",
  "action": "Update LLM",
  "status": 200,
  "diff": {
    "default_model": { "old": "gpt-4o", "new": "gpt-4o-mini" },
    "api_key": { "old": "[REDACTED]", "new": "[REDACTED]" }
  }
}
```

Every failed login in the last 24 hours, grouped by source address:

```bash
curl -s -H "Authorization: Bearer $ADMIN_API_KEY" \
  "https://studio.example.com/api/v1/audit/records?action=Login+Failed&start_date=$(date -u -v-1d +%FT%TZ)&page_size=500" \
  | jq -r '.records[] | "\(.ip) \(.user)"' | sort | uniq -c | sort -rn
```

---

## Operational notes

* **Immutability.** Records have no update or delete API. Retention is the only delete path. Restrict database access to the `audit_records` table accordingly.
* **Shutdown.** The queue is flushed on graceful shutdown. A hard kill can lose up to `AUDIT_QUEUE_SIZE` unwritten records.
* **Edge gateways.** Data-plane traffic through Microgateways is not part of the audit trail; that is what [Analytics](./analytics.md) and [Compliance Events](./compliance-events.md) cover. Control-plane operations against edges (reload, delete, namespace reload) are recorded.
* **SIEM forwarding.** Use `AUDIT_STORE_TYPE=both` with `AUDIT_FILE_FORMAT=json` and ship the file with your log forwarder. Each line is a complete record.
