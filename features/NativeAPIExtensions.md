# Native API Extensions

## Introduction

The vendor-native routes (`/llm/rest|stream|call/{llmSlug}/...`, see
[Proxy.md](Proxy.md)) were built for one call shape: a single synchronous
`POST` whose body names a top-level `model`. SDK features outside that shape
used to fail or slip past the gateway's controls. Anthropic's Message Batches
API is the first example. Its routes use GET and DELETE, a batch carries
`requests[].params.messages` that filters never saw, the usage only appears in
a JSONL results file billed at half price, and the vendor workspace is shared
by every App on the LLM.

**Native API extensions** are a small, declarative system that lets a vendor
add SDK-specific endpoints to the native routes without touching the proxy's
generic code. Anthropic Message Batches is the first extension. OpenAI
Files/Batches, Responses IDs and `count_tokens` are designed to fit the same
contract (see [Future extensions](#future-extensions)).

Extensions apply to the native routes only. The translation bridges (`/ai/`,
`/anthropic/`, the unified `/v1`) are unchanged.

---

## Table of Contents

1. [Components](#components)
2. [The contract (`pkg/nativeapi`)](#the-contract-pkgnativeapi)
3. [Dispatch pipeline](#dispatch-pipeline)
4. [Signed resource IDs](#signed-resource-ids)
5. [The native resource ledger](#the-native-resource-ledger)
6. [Accounting](#accounting)
7. [Anthropic Message Batches](#anthropic-message-batches)
8. [Configuration](#configuration)
9. [Metrics](#metrics)
10. [Security model](#security-model)
11. [Known limits](#known-limits)
12. [Adding an extension](#adding-an-extension)
13. [Future extensions](#future-extensions)
14. [Testing](#testing)

---

## Components

| Concern | Where |
|---|---|
| Contract: Spec, ResourceType, Operation, Usage, matcher, ID codec, JSONL reader | `pkg/nativeapi/` (imports `models` only) |
| Native route, matching, pinned upstream requests, ID wrapping, proxy log | `proxy/native_dispatch.go` |
| Create: body limit, per-item admission and filters, budget, in-progress cap, ledger write | `proxy/native_create.go` |
| Results downloads: streaming, per-line filters, download-time accounting | `proxy/native_results.go` |
| Listings and settlement-guarded deletes | `proxy/native_list.go` |
| Usage aggregation, claim-then-settle, recording | `proxy/native_accounting.go` |
| Background reconciler | `proxy/native_reconciler.go` |
| Ledger model and store | `models/native_resource.go`, `services/native_resource_store.go` |
| Anthropic extension | `vendors/anthropic/nativeapi.go` |
| Studio wiring (leader-gated reconciler) | `pkg/studio/studio.go`, `pkg/studio/native_workers.go` |
| Edge wiring (store, reconciler) | `microgateway/internal/server/server.go`, `microgateway/internal/services/gateway_adapter.go` |
| Metrics | `metrics/native.go` |

## The contract (`pkg/nativeapi`)

A vendor opts in by implementing `nativeapi.Provider` on its
`LLMVendorProvider`:

```go
func (v *Anthropic) NativeAPI() nativeapi.Spec { return nativeSpec }
```

The proxy type-asserts it on `switches.VendorMap[vendor]()`, the same way it
discovers `IDecisionResponse`, and compiles the Spec once per vendor
(`nativeVendorFor`). An invalid Spec is logged and disables that vendor's
extensions. The legacy chain keeps working.

A `Spec` holds:

- **`Resources`** (`ResourceType`): vendor objects the gateway hands out IDs
  for. `Name` is namespaced (`anthropic.message_batch`) and part of the signed
  ID. `IDCharset` is an anchored regexp the raw vendor ID must fully match, and
  the ID must contain a letter or digit, so `.` and `..` can never become path
  segments. A non-nil `Async` (`Poll`, `State`, `Results`, `LineUsage`,
  `Cancel`) marks a long-running resource that is ledgered, reconciled and
  accounted exactly once.
- **`Operations`**: each with an exact `Method` and canonical vendor `Path`
  (`/v1/messages/batches/{id}/results`). Matching is exact, segment by segment,
  with no prefix match. `PathResource` names the type of `{id}`. `Ledger`
  (`None|Create|Read|List|Mutate|Delete`) says what it does to the ledger.
  `Budget` applies the synchronous budget pre-check. Optional hooks:
  - `Admit` splits a body into items and joins them back after filtering.
  - `RequestRefs` and `ResponseIDs` are IDs inside bodies to unwrap or wrap.
  - `Transform` rewrites vendor URLs. The gateway never follows them.
  - `Results` handles JSONL downloads: `View` and `Blocked` per line.
  - `List` describes a ledger-served listing.
  - `Upstream` allow-lists the query parameters and headers that reach the vendor.
  - `ErrorShape` produces vendor-shaped errors.
- **`ChainHooks`**: reserved for hooks on the existing synchronous chain
  (OpenAI Responses IDs). It is declared so the contract does not change, and
  the proxy ignores it today.

Vendor code is data plus small pure functions. It never calls the proxy. The
proxy supplies everything stateful: filters, model checks, the store, the ID
codec and accounting.

## Dispatch pipeline

The native route is registered ahead of the legacy `/llm/` routes, inside
`credValidator.Middleware`. Authentication, PreAuth/PostAuth plugins and the
LLM ACL have all run by the time it sees a request. It runs before stream
detection, model validation and the `/llm/call` rewrite.

`nativeMatch` does the following:

1. Rejects any `..` segment (`hasTraversalSegment`).
2. Resolves the LLM.
3. Asserts the vendor's `Provider`.
4. Computes `canonicalVendorPath(endpoint path, rest)`, so endpoints with a
   corporate path prefix still match.
5. Matches the method and path exactly.

A native path asked with an undeclared method gets the vendor's 405 with
`Allow`. **No match leaves the request on the legacy chain byte for byte**:
`POST /v1/messages` is unchanged, and a GET elsewhere is still the legacy 405.

`handleNative` then runs these stages. A claimed request never falls back to the
legacy chain.

1. **Authentication:** without an App the call is refused with 401.
2. **Method check:** an undeclared method gets 405 with `Allow`.
3. **Direct grant** (`appHoldsLLM`): access inherited as a failover rung or
   through a router does not extend to vendor resources, and loopback markers
   are refused (403).
4. **Prerequisites:** a ledger store (`nativeStoreOf`) and a non-empty vendor
   credential for the ID codec. Without either the call gets 501.
5. **Budget** pre-check (`Budget: true`), then the per-App in-progress cap on
   creates (429).
6. **Create slot:** a bounded semaphore, taken before the body is read.
7. **Upstream path:** the canonical operation path, with `{id}` verified and
   unwrapped. Any verification failure gets 404 `not_found_error`.
8. **Body:** a bounded read (413 beyond the limit), with `RequestRefs` unwrapped.
9. **Admission** (creates): per-item model allow-list (403 naming the
   `custom_id`), then request filters per item on a bounded worker pool with
   one runner set. One block rejects the whole batch (400), and only items a
   filter changed are rewritten.
10. **Store health** check before a create reaches the vendor.
11. **Serve:** ledger-served list, settlement-guarded delete, streamed results,
    or forward.
12. **Proxy log** for every call (`analytics.RecordExchange` with no chat
    record), plus metrics.

Upstream requests are built only from `UpstreamEndpoint(llm)`, the canonical
path and a validated vendor ID. They pass `netguard.ValidateUpstreamURL`, use
the shared transport and carry the LLM's vendor credential.
`upstream_override` is ignored, because the resources live in that LLM's
vendor account. Only allow-listed headers and query parameters are forwarded.
A vendor-supplied URL is never followed.

## Signed resource IDs

Edges each own their SQLite, so ownership cannot rely on a shared table.
Every vendor ID handed to a client is wrapped as
`{vendor_id}.t1k{kid}.{mac}`:

- `mac`: HMAC-SHA256 truncated to 16 bytes, over the length-prefixed
  `type`, `vendor_id`, `llm_id` and `app_id`.
- Key: HKDF(LLM vendor credential, `tyk-native-resource/v1`).
- `kid`: 4 bytes of SHA-256 of the key, so logs can tell a rotated key from a
  forgery.

The credential is identical on the hub and every edge, and App and LLM IDs are
the hub's, so any replica, edge or the hub verifies an ID **statelessly**. Any
failure (malformed, wrong type, LLM or App, tampered) answers the same 404, so
the check cannot be used as an oracle. IDs are wrapped wherever they leave the
gateway: response `id` fields, list items and `first_id`/`last_id`, and
`results_url`. They are unwrapped wherever they come in: the path `{id}` and
the list cursors.

**Key-rotation fallback:** when the MAC fails, the ID is still accepted if this
node's ledger holds the calling App's row for that LLM, type and vendor ID.
The ledger lookup enforces the same scope.

## The native resource ledger

`models.NativeResource` is one model migrated on both sides: Studio's
`InitModels` (schema version 9) and the edge's `database.Migrate`. It is
**node-local**. Studio replicas share the Studio database, and each edge keeps
its own rows in its own SQLite. Only the node that created a resource has its
row.

| Columns | Purpose |
|---|---|
| `vendor`, `llm_id`, `app_id`, `user_id`, `on_behalf_of`, `acting_agent` | Owner and identity of the creating request (stamped on usage recorded later) |
| `resource_type`, `vendor_resource_id`, `parent_id` | The resource; unique `(llm_id, resource_type, vendor_resource_id)` |
| `state`, `snapshot`, `ended_at`, `results_expire_at` | Last vendor view (snapshot is the wrapped object, served by list) |
| `accounting_state`, `claim_token`, `claimed_at`, `accounted_at` | Accounting: `none`, `pending`, `claimed`, `accounted`, `unaccountable`, `lost` |
| `next_poll_at`, `poll_failures` | Reconciler scheduling; index `(accounting_state, next_poll_at)` |

Other properties of the ledger:

- **No soft delete.** A tombstone would block re-creating a vendor ID.
- **Keyset index.** Listings use the index
  `(app_id, llm_id, resource_type, created_at, id)`.
- **Store access.** The proxy gets the store through `nativeStoreOf`:
  `(*services.Service).NativeResourceStore()` on Studio, and
  `GatewayServiceAdapter.NativeResourceStore()` on the edge. The edge store
  reads through the pool and writes through the SQLite writer handle.
- **Single-statement transitions.** Every accounting transition is one
  conditional `UPDATE` (compare and set), with no read-then-write transaction:
  - claim: `pending`, or a claim older than the lease, to `claimed`
  - settle: claimed with this token, to a terminal state
  - release: claimed with this token, back to `pending`

  Concurrent settlers on any replica agree on one winner, on SQLite and
  Postgres alike.

## Accounting

A batch's usage is only known from its results file. It is recorded **exactly
once per batch** by whichever comes first:

- **Download time:** a complete results download on the creating node
  aggregates usage per model while streaming, then claims and settles the row
  at end of file.
- **The reconciler:** `StartNativeWorkers` polls due rows with backoff and
  bounded concurrency. When a batch has ended, it claims the row, streams the
  results through `LineUsage`, settles and records.
  - **Studio:** gated by the cluster leader.
  - **Edge:** ungated (the compare-and-set keeps it correct). Started in
    `Server.Start`, stopped before the analytics drain.

Both paths **claim, settle, and record only if the settle won**
(`settleNativeAccounting`, `accountNativeRow`). A Settle that hits a transient
store error is retried while the claim token is still held.

Recording (`recordAsyncUsage`) writes one proxy log and one chat record per
model and price through `analytics.RecordExchange`, then adds the spend to the
budget, so edge usage reaches Studio budgets and analytics through the
existing pulse. It never uses `RecordChatRecordsBatch`, which the edge does not
pulse. Pricing:

- **Multiplier:** cost is the list price times the result's tier multiplier
  (`models.IPriceAdjustedResponse`). Batch is ×0.5, and a result without a
  tier is a batch result.
- **Unbilled lines:** errored, canceled and expired lines are not billed.
- **Timestamp:** the accounting time, which matches budget-period semantics.

Outcomes of an accounting attempt:

| Outcome | Meaning |
|---|---|
| `accounted` | Recorded by this attempt |
| `settled` | Another settler already holds or finished the row |
| `not_ended` | Still processing; next poll scheduled, snapshot refreshed |
| `unaccountable` | Ended, but the results expired or are gone (404/410); usage lost, warned |
| `lost` | The vendor no longer knows the batch (404) |
| `failed` | Transient error (network, vendor 5xx, store); retried with backoff |

The reconciler's timings are configurable (see [Configuration](#configuration));
the defaults are:

- **Pass:** every 30 s, up to 50 rows, 4 workers.
- **Polling:** each batch is polled every 2 min. After failures the wait
  doubles, up to 30 min (never less than the poll interval).
- **Sweeping:** settled rows past `results_expire_at` are swept.

The accounting claim lease (5 min) is not configurable.

## Anthropic Message Batches

The extension lives in `vendors/anthropic/nativeapi.go` and has one resource,
`anthropic.message_batch` (IDs `msgbatch_...`, results kept 29 days from
creation).

| Operation | Method and path | Behaviour |
|---|---|---|
| create | `POST /v1/messages/batches` | Body ≤ limit. Every `requests[].params.model` is checked against the LLM's allowed models. Request filters run per item, with the `custom_id` in context. Duplicate or case-folded duplicate keys are refused. Budget pre-check and in-progress cap apply. The ledger row is written; if that keeps failing, the batch is cancelled at the vendor and the call gets 503. The ID is wrapped. |
| list | `GET /v1/messages/batches` | Served from the ledger, never forwarded: newest first, `limit` 1–1000 (default 20), `after_id`/`before_id` must be gateway IDs (else 400), `has_more` correct in both directions. |
| retrieve | `GET /v1/messages/batches/{id}` | Forwarded. IDs are wrapped, `results_url` is rewritten and the snapshot refreshed. |
| cancel | `POST /v1/messages/batches/{id}/cancel` | As retrieve. |
| results | `GET /v1/messages/batches/{id}/results` | Streamed line by line, never buffered (16 MiB line cap, 2 min idle timeout, per-chunk write deadline). Response filters run per line, and a blocked line becomes an `errored` result with the same `custom_id`. Usage is accounted on the creating node. **Refused (501) on an LLM with response body plugins**, because they cannot run on a streamed file. |
| delete | `DELETE /v1/messages/batches/{id}` | 409 unless this node's ledger shows the batch settled. On the creating node, an ended, unsettled batch is accounted first. Another node always gets 409. A successful delete removes the ledger row. |

Two details apply across these operations:

- **`results_url`:** it points at the gateway, absolute from the configured
  public URL (Studio `PROXY_URL`, edge `GATEWAY_PUBLIC_URL`) or root-relative
  without one. The Python and TypeScript SDKs resolve a root-relative URL
  against their base URL, and the Go SDK ignores `results_url`. `Host` and
  `X-Forwarded-*` are never trusted for it.
- **Forwarded headers and parameters:** the headers are `anthropic-version`,
  `anthropic-beta`, `content-type`, `accept` and `x-request-id`, and the only
  query parameter is `beta`. `anthropic-workspace-id` and
  `anthropic-user-profile-id` are dropped. Gateway-originated calls (reconciler
  polls and downloads) send `anthropic-version: 2023-06-01`.

The Anthropic `AnalyzeResponse` treats only a `/messages` suffix as a Messages
call. Other `/v1/messages/*` paths on the legacy chain (`count_tokens`) are
logged without a chat record.

## Configuration

| Studio env | Microgateway env | Default | Meaning |
|---|---|---|---|
| `PROXY_URL` | `GATEWAY_PUBLIC_URL` | — | Public base URL for gateway URLs handed out (`results_url`); empty means root-relative |
| `NATIVE_API_MAX_BODY_BYTES` | `NATIVE_API_MAX_BODY_BYTES` | `67108864` (64 MiB) | Create body limit; a larger value is capped at 256 MiB (`config.NativeMaxBodyBytesLimit`) when the config is loaded, with a warning |
| `NATIVE_API_MAX_CONCURRENT_CREATES` | same | `4` | Creates admitted at once (bodies are held in memory); others wait |
| `NATIVE_API_MAX_FILTERED_ITEMS` | same | `10000` | Max requests per batch when the LLM has request filters |
| `NATIVE_API_MAX_IN_PROGRESS_PER_APP` | same | `0` (no cap) | Max in-progress batches per App and LLM, as this node's ledger counts them |
| `NATIVE_API_RECONCILE_INTERVAL` | same | `30s` | How often the reconciler looks for due ledger rows (Go duration) |
| `NATIVE_API_RECONCILE_BATCH` | same | `50` | Ledger rows one reconciler pass takes |
| `NATIVE_API_RECONCILE_WORKERS` | same | `4` | Rows reconciled at once (vendor polls and results downloads in parallel) |
| `NATIVE_API_POLL_INTERVAL` | same | `2m` | Wait before polling an unfinished batch again (Go duration) |
| `NATIVE_API_POLL_MAX_BACKOFF` | same | `30m` | Cap of the doubling wait after failed polls; never below the poll interval (Go duration) |

`0` or unset keeps the default; an invalid or negative value is logged and
ignored. Embedders set the same settings on `proxy.Config` / `aigateway.Config`
(`NativeMaxBodyBytes`, `NativeMaxConcurrentCreates`, `NativeMaxFilteredItems`,
`NativeMaxInProgressPerApp`, `NativePublicURL`, `NativeReconcileInterval`,
`NativeReconcileBatch`, `NativeReconcileWorkers`, `NativePollInterval`,
`NativePollMaxBackoff`).

## Metrics

See [Observability.md](Observability.md).

| Metric | Type | Labels |
|---|---|---|
| `aistudio_native_requests_total` | counter | `vendor`, `operation`, `status_code` |
| `aistudio_native_request_duration_seconds` | histogram | `vendor`, `operation`, `status_code` |
| `aistudio_native_results_bytes_total` | counter | `vendor`, `operation` |
| `aistudio_native_reconcile_outcomes_total` | counter | `vendor`, `resource_type`, `outcome` |

`operation` is the extension's operation name
(`anthropic.message_batches.create`), or `method_not_allowed`.

## Security model

- **Ownership:** signed IDs bind a vendor ID to its type, LLM and App. An App
  can never see, fetch, cancel or delete another App's batch, and listings are
  scoped by the ledger.
- **No filter bypass:** every batch item passes the model allow-list and
  request filters, and every results line passes the response filters. Results
  are refused where response body plugins would be skipped.
- **No spend bypass:**
  - The budget pre-check runs on create.
  - Usage is accounted once, at the tier price, from the results.
  - A delete cannot dodge accounting, because it requires a settled ledger row.
- **No gateway bypass:** `results_url` points at the gateway, and upstream URLs
  are pinned to the LLM's endpoint and pass `netguard`.
- **Single writer:** only the creating node accounts a batch. Compare-and-set
  claims make concurrent settlers safe.

## Known limits

- **Listings are per node.** On an edge, list shows only that edge's batches,
  and so do the in-progress cap and the delete guard. Studio replicas share one
  ledger.
- **Deletes need settlement on the creating node.** A batch created on another
  edge cannot be deleted through this one, and an in-progress batch cannot be
  deleted.
- **At-most-once crash window.** A crash between the settle and the async
  analytics hand-off loses that batch's record, as for today's async
  analytics.
- **Lost edge SQLite.** Batches whose edge lost its SQLite are never accounted.
- **Expired results.** Results expired before anyone read them make the batch
  `unaccountable`, with a warning.
- **Signing key.** The signing key is derived from the vendor credential:
  rotating it invalidates outstanding IDs, except through the ledger fallback
  on the creating node. A dedicated hub-generated secret is a follow-up.
- **No service tier column.** There is no `ServiceTier` column on chat records
  yet (it needs a pulse proto field). The cost is already correct.

## Adding an extension

The full walk-through is in `pkg/nativeapi/doc.go`.

1. **Opt in:** implement `NativeAPI()` on the vendor, returning a package-level
   `Spec`.
2. **Resources:** declare each resource with a strict `IDCharset`. Add `Async`
   if its usage arrives later.
3. **Operations:** declare each with an exact method and canonical path, its
   `Ledger` effect and the hooks it needs.
4. **Errors:** give every operation the vendor's `ErrorShape`.
5. **Tests:** validate with `nativeapi.NewMatcher`, then add proxy harness
   tests (`newNativeHarness`, `fakeBatchAPI` in `proxy/native_harness_test.go`).

## Future extensions

The contract was checked against these. Their hooks are declared but not
implemented.

- **OpenAI Batches:**
  - `POST /v1/files` uses `Admit` with per-line items and mints
    `openai.file.batch_input`.
  - `POST /v1/batches` unwraps `input_file_id` through `RequestRefs`.
  - `GET /v1/batches/{id}` mints its output and error file IDs through
    `ResponseIDs`.
  - `GET /v1/files/{id}/content` is a `Results` op.
- **OpenAI Responses:** needs `ChainHooks` on the synchronous chain to mint IDs
  and verify `previous_response_id`.
- **Anthropic `count_tokens`:** a single-body `Admit` op.

## Testing

| Area | Tests |
|---|---|
| Contract, matcher, codec, JSONL | `pkg/nativeapi/*_test.go` |
| Ledger compare-and-set (SQLite and Postgres via `DATABASE_URL`) | `services/native_resource_store*_test.go` |
| Dispatch, create, results, list and delete, reconciler, metrics | `proxy/native_*_test.go` |
| Anthropic spec | `vendors/anthropic/nativeapi*_test.go` |
| Edge store wiring | `microgateway/internal/server/native_store_test.go` |
| Configuration | `config/native_api_test.go`, `microgateway/internal/config/native_api_test.go` |
