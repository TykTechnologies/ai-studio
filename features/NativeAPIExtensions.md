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
generic code. Anthropic Message Batches (and `count_tokens`) is the first
extension. OpenAI Responses ownership is the second, and the first to use
chain hooks on the synchronous chain (see [OpenAI Responses](#openai-responses)).
OpenAI Batches and Files are the third (see
[OpenAI Batches and Files](#openai-batches-and-files)).

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
8. [Chain hooks](#chain-hooks)
9. [OpenAI Responses](#openai-responses)
10. [OpenAI Batches and Files](#openai-batches-and-files)
11. [Configuration](#configuration)
12. [Metrics](#metrics)
13. [Security model](#security-model)
14. [Known limits](#known-limits)
15. [Adding an extension](#adding-an-extension)
16. [Testing](#testing)

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
| Chain hooks on the synchronous chain: reference checks, ID wrapping (JSON and server-sent events), tracked creates | `proxy/native_chain.go` |
| Ledger model and store | `models/native_resource.go`, `services/native_resource_store.go` |
| Resource signing keys: generation, backfill, rotation; status/rotate API | `models/llm_resource_signing.go`, `services/llm_resource_signing_service.go`, `api/llm_signing_handlers.go` |
| Anthropic extension | `vendors/anthropic/nativeapi.go` |
| OpenAI extension (Responses), Responses usage analysis | `vendors/openai/nativeapi.go` |
| OpenAI Batches and Files (upload admission, results lines) | `vendors/openai/batches.go` |
| Child resources (a batch's output files) | `proxy/native_children.go` |
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
  accounted exactly once. Its usage comes from a results download
  (`Results` + `LineUsage`) or from the ended object itself (`ObjectUsage`).
  `Tracked` limits ledgering to some objects of the type (only background
  OpenAI responses); the others are accounted by the synchronous chain.
- **`Operations`**: each with an exact `Method` and canonical vendor `Path`
  (`/v1/messages/batches/{id}/results`). Matching is exact, segment by segment,
  with no prefix match. `PathResource` names the type of `{id}`. `Ledger`
  (`None|Create|Read|List|Mutate|Delete`) says what it does to the ledger.
  `Budget` applies the synchronous budget pre-check. Optional hooks:
  - `Admit` splits a body into items and joins them back after filtering.
    `Multipart` marks a multipart/form-data body: the proxy requires the
    Content-Type boundary to open the body, so `Split` (which reads the
    boundary from the first line) parses it as the vendor will.
  - `RequestRefs` and `ResponseIDs` are IDs inside bodies to unwrap or wrap.
    A `Child` response ID names a resource derived from the operation's async
    resource (see [OpenAI Batches and Files](#openai-batches-and-files)).
    An `ObjectID` ref's field holds the ID or an object whose `id` holds it
    (OpenAI's `conversation`).
  - `Transform` rewrites vendor URLs. The gateway never follows them.
  - `Results` handles JSONL downloads: `View` and `Blocked` per line.
  - `List` describes a ledger-served listing.
  - `Upstream` allow-lists the query parameters and headers that reach the
    vendor. `RefuseQuery` names parameters the gateway does not support on
    the operation (400 unless the value is `false`).
  - `ErrorShape` produces vendor-shaped errors.
- **`ChainHooks`**: hooks on the existing synchronous chain, for calls that
  stay on the legacy pass-through but hand out or name resource IDs (see
  [Chain hooks](#chain-hooks)).

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
`Allow`, unless a chain hook declares that method and path (OpenAI's
`POST /v1/responses/compact` has the shape of `/v1/responses/{id}`). **No
match leaves the request on the legacy chain byte for byte**: `POST
/v1/messages` is unchanged, and a GET elsewhere is still the legacy 405.

Matching is exact, so another spelling of a declared path would slip past
the native handling to an upstream that might route it case-insensitively
(`POST /v1/Responses` with another App's raw `previous_response_id`). So a
path that matches a declared operation or chain hook only once case and
empty segments are ignored (`Matcher.MatchesFold`: `/v1/Messages/Batches`,
`/V1/responses`) is claimed and refused with the vendor's 404 (metric
operation `path_variant`). Both the canonical path and the caller's own path
are checked, since joining the caller's path to an endpoint's path is
case-sensitive (`/V1/responses` on a `.../v1` endpoint canonicalises to
`/v1/V1/responses`). A trailing slash is cleaned away when the endpoint has a
path (the request is then served as the declared path) and refused when it
has none; either way it never bypasses the native handling. The exact path with another method is
unaffected. Likewise a top-level body key that differs only in case from a
reference field (`Previous_Response_Id`, `Input_File_Id`) is refused with
400 (`refKeyVariant`), on native operations and chain hooks.

`handleNative` then runs these stages. A claimed request never falls back to the
legacy chain.

1. **Authentication:** without an App the call is refused with 401.
2. **Method check:** an undeclared method gets 405 with `Allow`; a
   `RefuseQuery` parameter gets 400.
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
- Key: HKDF(the LLM's resource signing secret, `tyk-native-resource/secret/v1`)
  (see [Signing keys](#signing-keys)).
- `kid`: 4 bytes of SHA-256 of the key. It picks the key to verify with and
  lets logs tell a rotated key from a forgery.

The signing secret is identical on the hub and every edge, and App and LLM IDs
are the hub's, so any replica, edge or the hub verifies an ID
**statelessly**. Any failure (malformed, wrong type, LLM or App, tampered,
unknown or expired key) answers the same 404, so the check cannot be used as
an oracle. IDs are wrapped wherever they leave the gateway: response `id`
fields, list items and `first_id`/`last_id`, and `results_url`. They are
unwrapped wherever they come in: the path `{id}` and the list cursors.

**Key-rotation fallback:** when the MAC fails, the ID is still accepted if this
node's ledger holds the calling App's row for that LLM, type and vendor ID.
The ledger lookup enforces the same scope. It covers IDs whose key has left
the keyring (a rotation past its grace period) on the node that created the
resource.

### Signing keys

Each LLM has its own random 32-byte **resource signing secret**, generated by
the hub, so rotating the vendor credential no longer invalidates outstanding
IDs.

- **Storage:** `llms.resource_signing_secret`, `resource_signing_secret_prev`,
  `resource_signing_prev_expires_at` and `resource_signing_rotated_at` (schema
  version 10), the secrets encrypted with `secrets.EncryptValue`. They are
  `json:"-"`: no API response, object hook payload or system event carries
  them, and the audit log redacts the `*secret*` columns. `LLM.Update` omits
  the columns, so an edit rebuilt from JSON or loaded before a rotation never
  overwrites them; only the functions below write them
  (`models/llm_resource_signing.go`).
- **Generation:** `LLM.BeforeCreate` gives every new LLM a secret, whatever
  path creates it. At startup (under the migration lock) Studio runs
  `BackfillLLMResourceSigningSecrets` for LLMs created before secrets existed.
  Their outstanding IDs were signed with the key derived from the vendor
  credential (HKDF(credential, `tyk-native-resource/v1`),
  `models.DeriveCredentialSigningKey`), so the backfill derives that key once
  and stores it, encrypted, as the previous key for 30 days
  (`derived-key:v1:` + base64url). Because the key itself is stored, changing
  the vendor credential during those 30 days keeps pre-upgrade IDs valid,
  and the status endpoint keeps reporting their kid. An LLM without a
  credential gets no previous key. Rows backfilled by early builds hold the
  sentinel `vendor-credential` instead, which is still read (re-derived from
  the current credential). Each row is a compare-and-set, so replicas
  starting together agree on one secret.
- **Rotation:** `POST /api/v1/llms/{id}/resource-signing-key/rotate`
  (`llms` write permission, optional body `{"grace_period_hours": N}`,
  default 720, `0` revokes the old key at once, at most 8760, checked before
  it is converted to a duration so huge values are a 400) generates a new
  secret and keeps the replaced one verifiable until the grace period ends.
  Only the key just replaced is kept: rotating twice drops the older one. The
  update is a compare-and-set on the old secret, and a concurrent rotation
  gets 409. It emits `system.llm.updated`, which reloads the proxy and marks
  edges pending.
- **Status:** `GET /api/v1/llms/{id}/resource-signing-key` (`llms` read)
  returns `{"data": {"llm_id", "source", "kid", "rotated_at", "previous":
  {"kid", "source", "expires_at", "active"}}}`. `source` is `secret`,
  `vendor_credential` (an LLM not backfilled yet, or the backfill's previous
  key) or `none`. The secrets are never returned.
- **Admin UI:** an Anthropic or OpenAI LLM's details page has a "Native API resource
  IDs" section (`ui/admin-frontend/src/admin/components/llms/ResourceSigningKeySection.js`)
  showing the current and previous kid, their source and the previous key's
  expiry. Users with `llms` write get "Rotate signing key", which confirms in a
  dialog with a grace period (30 days default, 7 days, 1 day or revoke
  immediately) and warns when a still-active previous key will be dropped.
  Other vendors do not show the section (`NATIVE_RESOURCE_VENDORS`).
- **Edges:** the snapshot's `LLMConfig` carries
  `resource_signing_secret_encrypted`, `resource_signing_secret_prev_encrypted`
  (both encrypted like `api_key_encrypted`, deterministically, so the checksum
  is stable) and `resource_signing_prev_expires_at`. The edge stores them in
  `llms` (encrypted) and the gateway adapter decrypts them onto `models.LLM`.
  A rotation changes the checksum. Until an edge receives the new snapshot it
  keeps minting IDs under the old key, which the hub and updated edges still
  accept during the grace period.
- **Codec:** `nativeapi.CodecForLLM(llm, now)` signs with the secret and also
  accepts the previous key (`nativeapi.PreviousKeyForLLM`: a rotated
  secret, the stored vendor-credential key, or the legacy sentinel) until
  `resource_signing_prev_expires_at`. An LLM without a secret (a standalone
  edge's, or a hub row before the backfill) signs with the vendor-credential
  key as before.

## The native resource ledger

`models.NativeResource` is one model migrated on both sides: Studio's
`InitModels` (schema version 9 and later) and the edge's `database.Migrate`. It is
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
  tier is a batch result. An LLM's `price_multipliers` override replaces the
  vendor's multiplier for the tiers it names (see
  [Price multiplier overrides](#price-multiplier-overrides)).
- **Unbilled lines:** errored, canceled and expired lines are not billed.
- **Timestamp:** the accounting time, which matches budget-period semantics.
- **Cost breakdown:** the analytics breakdown (prompt, response, cache write
  and cache read cost per model, `analytics.GetTotalCostPerVendorAndModel`) is
  scaled to each row's recorded cost, so a batch row's components are half
  their list price and still add up to its total. Rows whose price changed
  after they were recorded split their recorded cost in today's list-price
  proportions, and unset OpenAI and Google cache prices bill at the input
  price as in `models.CachePricesFor`. There is no schema or pulse change:
  edge rows arrive with their cost already multiplied.
- **Repricing:** a model price edit with recalculation
  (`UpdateModelPriceAndRecalculate`) keeps each record's ratio of recorded to
  list cost, so batch records stay at 0.5x of the new price rather than being
  re-billed at list price. A record with no old list cost gets the new list
  price at 1x (see [Pricing.md](Pricing.md)).

**Service tier:** every priced record carries its tier.
`LLMChatRecord.ServiceTier` and `ProxyLog.ServiceTier` (schema version 10,
`size:32`, NULL on older rows) hold the tier a
`models.IPriceAdjustedResponse` reports (`batch`, `priority`, `flex`...):
`buildChatRecordFor` sets the record's, `recordAsyncUsage` and the
synchronous `AnalyzeResponse`/`AnalyzeStreamingResponse` set the proxy log's.
List-price traffic has `""`. Edge usage carries it end to end: the edge's
`analytics_events.service_tier`, `interfaces.AnalyticsData.ServiceTier` (and
`AnalyticsRequest.service_tier` for gRPC data collector plugins), the pulse's
`AnalyticsEvent.service_tier` (field 45), and the hub's pulse ingest, which
stores it on both rows. The cost on the pulse is already multiplied.
`GET /api/v1/analytics/service-tier-usage` (`analytics` read;
`start_date`, `end_date`, optional `llm_id`, `app_id` and `service_tier`,
where `none` selects untiered records, which includes NULL rows and
`default`/`auto` rows stored before normalisation) returns `by_tier` (requests, tokens,
cost by currency) and `by_model` (per tier, vendor, model and currency:
requests, prompt, response and total tokens, cost), costs in currency units
(`analytics.GetServiceTierUsage`). The admin dashboard's "Service Tiers" panel
(`ui/admin-frontend/src/admin/components/dashboard/ServiceTierUsagePanel.js`)
shows `by_tier` (untiered traffic labelled "Untiered (list price)", apart
from an explicit `standard` tier; unknown tiers shown as-is; currencies listed
side by side) and `by_model` with a tier filter, for the dashboard's date range;
it stays hidden until some traffic ran at a non-standard tier.

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
| create | `POST /v1/messages/batches` | Body ≤ limit. Every `requests[].params.model` is checked against the LLM's allowed models. Request filters run per item, with the `custom_id` in context. Duplicate keys are refused (`nativeapi.CheckJSONKeys` on each `params`, see the duplicate-key paragraph below), and the error names the body once (`invalid message batch body: request 0: params: duplicate key "Content"`). Budget pre-check and in-progress cap apply. The ledger row is written; if that keeps failing, the batch is cancelled at the vendor and the call gets 503. The ID is wrapped. |
| list | `GET /v1/messages/batches` | Served from the ledger, never forwarded: newest first, `limit` 1–1000 (default 20), `after_id`/`before_id` must be gateway IDs (else 400), `has_more` correct in both directions. |
| retrieve | `GET /v1/messages/batches/{id}` | Forwarded. IDs are wrapped, `results_url` is rewritten and the snapshot refreshed. |
| cancel | `POST /v1/messages/batches/{id}/cancel` | As retrieve. |
| results | `GET /v1/messages/batches/{id}/results` | Streamed line by line, never buffered (16 MiB line cap, 2 min idle timeout, per-chunk write deadline). Response filters run per line, and a blocked line becomes an `errored` result with the same `custom_id`. Usage is accounted on the creating node. **Refused (501) on an LLM with response body plugins**, because they cannot run on a streamed file. |
| count_tokens | `POST /v1/messages/count_tokens` | A single-body `Admit` op (`splitCountTokens`): the body's `model` is checked against the allowed models (403), duplicate keys are refused as on `/v1/messages` (400), and the request filters run on it as on a synchronous call (no `custom_id` in the script context). Budget pre-check, then forwarded; one proxy log row, no chat record (counting is free). It touches no vendor resource (`nativeResourceFree`), so it needs no direct grant, ledger or ID codec, and takes no create slot. |
| delete | `DELETE /v1/messages/batches/{id}` | 409 unless this node's ledger shows the batch settled. On the creating node, an ended, unsettled batch is accounted first. Another node always gets 409. A successful delete removes the ledger row. |

Two details apply across these operations:

- **`results_url`:** it points at the gateway, absolute from the configured
  public URL (Studio `PROXY_URL`, edge `GATEWAY_PUBLIC_URL`) or root-relative
  without one. The Python and TypeScript SDKs resolve a root-relative URL
  against their base URL, and the Go SDK ignores `results_url`. `Host` and
  `X-Forwarded-*` are never trusted for it.
- **Forwarded headers and parameters:** the headers are `anthropic-version`,
  `anthropic-beta`, `content-type`, `accept` and `x-request-id`, and the only
  query parameter is `beta`. A client's `anthropic-workspace-id` and
  `anthropic-user-profile-id` are dropped. Gateway-originated calls (reconciler
  polls and downloads) send `anthropic-version: 2023-06-01`.
- **Workspace pin:** an LLM's `vendor_workspace_id` (`models.LLM.VendorWorkspaceID`,
  schema version 10, synced to edges as `LLMConfig.vendor_workspace_id`) is
  sent as `anthropic-workspace-id` on every upstream call for the LLM: native
  operations, the reconciler's polls and downloads, and the synchronous
  `/llm/rest|stream|call` Messages calls. The Anthropic vendor's
  `ProxySetAuthHeader` sets it and removes any client-sent
  `anthropic-workspace-id` and `anthropic-user-profile-id`, on the synchronous
  chain as well, so a client can never move a call into another workspace.
  Empty sends no workspace header. It is set through the LLM API
  (`vendor_workspace_id` attribute on create and PATCH; an absent key keeps
  the stored value, `""` removes the pin; 1-128 letters, digits, `_` or `-`,
  else 400) and passed to the service as `services.WithVendorWorkspaceID`
  (an `LLMOption`). The langchaingo Anthropic driver (chat sessions, the
  translation bridges, semantic routing) sends it as well: `setupAnthropicDriver`
  and `fetchDriverWithHTTPClient` wrap the driver's HTTP client in
  `anthropicVendor.PinWorkspace`, so Chat UI traffic cannot escape the pin.
  In the admin UI it is the "Anthropic workspace ID" field under Access
  Details in the LLM form (Anthropic vendor only; the form validates the same
  pattern, and saving with another vendor sends `""`), shown as "Workspace" on
  the LLM details page.

The synchronous `POST /v1/messages` has a chain hook (`anthropic.messages`)
whose `CheckBody` (`checkMessagesKeys`) refuses duplicate keys with 400
before the filters run. Go's JSON decoding (the model check, the filter
extractors) takes the last duplicate, while the vendor may read the first, so
an exact duplicate (two `model`s, a message with two `content`s) is refused
at any depth. Go also matches struct fields case-insensitively, so case
variants (`messages` and `Messages`) are refused only in the objects the
gateway decodes into structs (`messagesFoldPaths`: the request, each message,
`tool_choice`). Elsewhere, such as a tool's `input_schema` properties (`id`
and `ID`) or a `tool_use` input (`Name` and `name`), case variants are
different keys to the gateway and the vendor alike and pass. Batch params and
`count_tokens` use the same check; OpenAI batch lines fold at the line, its
`body`, each chat message and tool (`batchLineFoldPaths`).

The Anthropic `AnalyzeResponse` treats only a `/messages` suffix as a Messages
call. Other `/v1/messages/*` paths that still reach the legacy chain (a
`count_tokens` with a trailing slash) are logged without a chat record.

## Chain hooks

A `ChainHook` (`proxy/native_chain.go`) applies to requests with exactly its
method and canonical path (no parameters) that stay on the legacy
synchronous chain: model validation, budget, request filters, response
filters and hooks, and the vendor's usage analysis all run as before. The
REST and streaming handlers (`handleLLMRequest`,
`handleStreamingLLMRequest`, so `/llm/rest`, `/llm/stream` and `/llm/call`)
ask `nativeChainFor` once the App is known, and the hook adds three things:

1. **References** (`RequestRefs`): before the budget check and the filters,
   each reference in the JSON body is verified and unwrapped for the calling
   App and LLM, exactly as the native route does for a path `{id}`. A body
   that is not a JSON object is a 400; a reference that does not verify (or
   is not a string) is the same vendor-shaped 404, and the vendor is never
   called. The body is re-encoded only when a reference was unwrapped.
2. **Wrapping** (`ResponseIDs`): the IDs of a successful JSON response are
   wrapped. With `EventObject` set, a `text/event-stream` response is
   rewritten one event at a time: the resource object at that path in each
   event's JSON data has its IDs wrapped, and events without it pass
   through byte for byte. The upstream request drops the client's
   `Accept-Encoding` (the transport asks for gzip itself and decompresses).
   A response that cannot be rewritten (not JSON, an ID outside the
   charset, an event over 16 MiB) is replaced by a vendor-shaped 502 or ends
   the stream: a raw vendor ID never reaches the client.
3. **Tracked creates**: when the created object's type (`ResponseIDs[0]`)
   is async and `Tracked` reports the object, it is written to the ledger
   (`ledgerCreated`, shared with native creates) once per request, even if
   several events carry it. If the row cannot be written the resource is
   cancelled upstream and the client gets a 503 (in a stream, a final
   `event: error` with the vendor-shaped error); without a ledger it is
   cancelled with a 501. The vendor's synchronous usage analysis must record
   no usage for a tracked object, or it would be counted twice.

`FilterFormat` names the extractor and reconstructor the request filters
read the body through when it is not the vendor's chat shape. `CheckBody`
vets the body first (a 400 naming the error); the ID codec is only
needed by hooks with references or response IDs. `DropHeaders` are removed
from the client request before it is forwarded,
so it reaches the same vendor account as the native operations and the
reconciler. The hook needs the ID codec (the LLM's signing secret or vendor credential):
without one the request is refused with 501 before it reaches the vendor.
Refused requests are logged as a proxy log row without a chat record.

## OpenAI Responses

The extension lives in `vendors/openai/nativeapi.go` and has one resource,
`openai.response` (IDs `resp_...`, stored 30 days). Before it, any App
sharing the LLM's OpenAI key could continue another App's conversation
(`previous_response_id`) or read and delete its stored responses.

| Request | Where | Behaviour |
|---|---|---|
| `POST /v1/responses` | chain hook | `previous_response_id` verified and unwrapped. `id` and `previous_response_id` of the response wrapped, in JSON and in every streamed event's `response` object (`response.queued`, `response.created`, `response.completed`, ...). A background response is ledgered. |
| `POST /v1/responses/input_tokens`, `POST /v1/responses/compact` | chain hook | `previous_response_id` verified and unwrapped. Responses passed through. |
| `GET /v1/responses/{id}` | native, read | Forwarded with `include`/`include[]`. IDs wrapped, ledger snapshot refreshed for a background response. `stream=true` (resuming a background stream) is refused with 400. |
| `POST /v1/responses/{id}/cancel` | native, mutate | Forwarded, IDs wrapped. |
| `GET /v1/responses/{id}/input_items` | native | Forwarded with `after`, `before`, `limit`, `order`, `include`. Item IDs (`msg_...`) are not response IDs and are not wrapped. |
| `DELETE /v1/responses/{id}` | native, delete | A foreground response has no ledger row: the gateway reads the vendor object and deletes it when it is not a background response (a 404 from the vendor passes through). A background response follows the batch rule: deleted only through the node that created it, accounting first. |

Headers forwarded on native operations are `content-type`, `accept` and
`openai-beta`. `OpenAI-Organization` and `OpenAI-Project` are dropped there
and, through the chain hooks' `DropHeaders`, from the creates: a response
created in another project would 404 on the reconciler's poll and never be
accounted.

**Conversations.** `openai.conversation` (IDs `conv_...`) is owned the same
way (`vendors/openai/conversations.go`). `POST /v1/conversations` wraps the
new ID; retrieve, update (`POST`) and delete of `/v1/conversations/{id}`, and
list, create, retrieve and delete of its `items` (`/items`,
`/items/{item_id}`) verify it (404 otherwise) and forward. The chain hooks
verify and unwrap `conversation` on `POST /v1/responses`, `input_tokens` and
`compact`, as a string or `{"id": ...}` (`ObjectID`), and the response's
`conversation.id` is wrapped. Nothing is ledgered (conversations cost
nothing), so any node verifies them statelessly. The items stored by a
conversation create or an items create reach the model through every
response that names the conversation, so they are admitted when stored: one
model-less item in the Responses filter format (`{"input": items}`) runs the
request filters, a rewrite is written back into `items`, and an unreadable
item refuses the request while filters are attached. An admission item with
no `Model` skips the allow-list check (it invokes no model).

**Item IDs are not owned.** Item IDs (`msg_...`, `fc_...`, `rs_...`) appear
in response output, `input_items` and conversation item listings, and an
input `item_reference` can name one. They are not wrapped: they are long
random identifiers returned only to the App whose response or conversation
holds them (and visible to admins in proxy logs), so naming another App's
item needs an ID that leaked. Owning them would mean rewriting every item ID
in every output, stream event and listing, and verifying every input item's
`id`. If that residual risk matters, the next step is to refuse
`item_reference` inputs on LLMs shared by several Apps.

**Filters.** The chain hooks set `FilterFormat: "openai_responses"`, so the
request filters and guardrails read a Responses body through
`OpenAIResponsesMessageExtractor` (`proxy/responses_filters.go`) rather than
the Chat Completions one, which found no `messages` and screened an empty
prompt. In body order, `instructions` is a system message; each
`prompt.variables` value a user message (by name); a string `input` a user
message; and each input item one message: a message item's text parts joined
(role user, assistant, or system for `system`/`developer`), a `*_call_output`
item's output as a tool message, and a `function_call`'s arguments (a
`custom_tool_call`'s input) as an assistant message. Built-in items are read
from a table of their text fields (`responsesItemTextFields`), one message
per string: what the model produced as assistant text (`reasoning`
`summary[].text` and `content[].text`, `mcp_call` and
`mcp_approval_request` `arguments`, `code_interpreter_call` `code`,
`web_search_call` `action.query`/`action.queries[]`, `file_search_call`
`queries[]`, `computer_call` `action.text`, `local_shell_call`
`action.command[]`), results as tool text (`mcp_call` `output` and `error`,
`mcp_list_tools` `tools[].description` and `error`, `code_interpreter_call`
`outputs[].logs`, `file_search_call` `results[].text`), and an
`mcp_approval_response` `reason` as user text. `item_reference` and
`image_generation_call` carry none. An item type missing from the table is
refused when filters are attached, so a type the API adds later cannot slip
past them. `input_file` parts (`file_data`, including base64 PDFs and text
files) are not screened, as with files on Chat Completions. A rewrite is
written back into the field it came from (a message's text parts become one
`input_text`/`output_text` part ahead of its images and files); filter output
that adds or drops messages fails the request. The extractor fails closed
(`MessageExtractorRegistry.FailsClosed`): with request filters attached, a
prompt field of a shape it cannot read, or an input item of an unknown type,
refuses the request (400) instead of screening an empty prompt. Batch lines with url `/v1/responses` are admitted
through the same format.

**Embeddings, completions and moderations.** `POST /v1/embeddings`,
`/v1/completions` and `/v1/moderations` have chain hooks that only set
`FilterFormat: "openai_input"` (`OpenAIInputMessageExtractor`,
`proxy/openai_input_filters.go`), and batch lines with those urls use it too.
The filters see `prompt`, `suffix` and `input` as user messages, in that
order: a string, each string of an array, or each text part of a
moderation's multimodal input (image parts are skipped). Rewrites go back
into the same position. Token arrays cannot be screened: with request
filters attached they are refused (400, fail closed); without filters the
traffic is unchanged.

**Accounting.** OpenAI's `AnalyzeResponse` and `AnalyzeStreamingResponse`
recognise the Responses paths (`responsesPathKind`): a create or compact
records `usage` (`input_tokens` including `input_tokens_details.cached_tokens`,
like `prompt_tokens`; tool calls are the `*_call` output items), streamed
usage comes from the terminal event's `response`, and `input_tokens`, errors
and background responses are logged without a chat record. A background
response is accounted by the reconciler instead: it polls
`GET /v1/responses/{id}`, and once the status is `completed`, `failed`,
`cancelled` or `incomplete` claims the row, reads `usage` from the object
(`ObjectUsage`), settles and records it at the list price (a response
cancelled before it ran has no usage). A delete on the creating node
accounts an ended background response first.

## OpenAI Batches and Files

The extension lives in `vendors/openai/batches.go` and has two resources:
`openai.file` (IDs `file-...`, no lifecycle) and `openai.batch` (IDs
`batch_...`, async). It follows the OpenAI SDK flow: upload a JSONL file with
`purpose=batch`, create a batch from it, poll it, download its output file.

| Operation | Method and path | Behaviour |
|---|---|---|
| files.create | `POST /v1/files` | A `Multipart` `Admit` op (`splitBatchFile`). The body must open with the Content-Type's boundary and end at its closing boundary (no epilogue). Parsing fails closed on anything another multipart parser could read differently: every part carries exactly the plain `Content-Disposition: form-data; name="..."` (plus `; filename="..."` on the file part only; no quoted-pair escapes, no RFC 2231 `name*`/`filename*`, no repeated parameters) and at most a `Content-Type` besides; names are allow-listed (`file`, `purpose`, `expires_after[anchor]`, `expires_after[seconds]`) and unique. Exactly one `file` part and one `purpose` field, and `purpose` must be `batch`. Every non-blank line is a request: `custom_id`, `method: POST`, a `url` among `/v1/chat/completions`, `/v1/completions`, `/v1/embeddings`, `/v1/responses` and `/v1/moderations`, and a `body` with a `model`. Exact duplicate keys are refused at any depth, case variants where the gateway decodes structs (`batchLineFoldPaths`). Each line's model is checked against the allowed models (403 naming the `custom_id`) and the request filters run on its body (format `openai`). A filter's rewrite replaces only that line (compacted onto one line) and the upload is re-encoded with the same boundary. The file is written to the ledger (no accounting) and its ID wrapped. |
| files.list | `GET /v1/files` | Served from the ledger: the App's uploaded files and the output and error files of its batches on this node. `limit` 1–1000 (default 1000), `after`. `purpose` and `order` are ignored. |
| files.retrieve | `GET /v1/files/{id}` | Forwarded, ID wrapped, ledger snapshot refreshed. |
| files.delete | `DELETE /v1/files/{id}` | Needs this node's ledger row (409 otherwise). An output or error file waits for its batch: an ended, unaccounted batch is accounted first, and the delete is refused (409) while it is not. |
| files.content | `GET /v1/files/{id}/content` | A `Results` op: streamed line by line, response filters run on each 200 result's body, a blocked line becomes a 400 result for the same `custom_id`. Not accounted here (the path names a file, not the batch): the reconciler accounts the batch. |
| batches.create | `POST /v1/batches` | `input_file_id` verified and unwrapped (404 for another App's file or a raw ID). Budget pre-check and in-progress cap. The batch is written to the ledger (pending) and its ID and file IDs wrapped. |
| batches.list | `GET /v1/batches` | Served from the ledger; `limit` 1–100 (default 20), `after`. |
| batches.retrieve, batches.cancel | `GET /v1/batches/{id}`, `POST /v1/batches/{id}/cancel` | Forwarded, IDs wrapped, snapshot refreshed. |

**Child resources.** `output_file_id` and `error_file_id` are `Child`
response IDs. Whenever the node that holds the batch's ledger row sees them
(a retrieve or cancel, or a reconciler poll), `recordNativeChildren` records
each file as an `openai.file` row with `ParentID` set to the batch's row and
the vendor's file object as its snapshot. That is what lets the output file
be listed, retrieved (its ID verifies like any other) and protected: the
delete stage checks the parent (`parentSettled`), since deleting an output
file before the batch is accounted would leave its usage unreadable. A parent
row no longer in the ledger was settled and swept. `NativeResourceStore.Get`
reads the parent by ID.

**Accounting.** The batch's `Results` locator is `GET
/v1/files/{output_file_id}/content` (only an ID matching the file charset is
followed). `LineUsage` reads each 200 result's `usage`: `prompt_tokens`,
`completion_tokens` and `prompt_tokens_details.cached_tokens` for chat
completions and embeddings, or `input_tokens`, `output_tokens` and
`input_tokens_details.cached_tokens` for responses. Usage is billed at 0.5x
(tier `batch`); results with another status, or none, are not billed. A batch
that ended without an output file (every request failed) has nothing to bill
and is settled `unaccountable` with a warning. Output files are assumed
downloadable for 30 days from the batch's creation (`results_expire_at`).

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

Per-LLM settings live on the LLM, not in the environment, so they reach edges
with the LLM config: the workspace pin (`vendor_workspace_id`, see
[Anthropic Message Batches](#anthropic-message-batches)), the resource
signing key ([Signing keys](#signing-keys)) and price multiplier overrides.

### Price multiplier overrides

Operators with negotiated pricing set `price_multipliers` on the LLM: a JSON
object from service tier to the share of the list price that tier is billed
at, for example `{"batch": 0.4}`.

- **Applied:** in `buildChatRecordFor` (`proxy/analyze_utils.go`), the one
  place a priced response's multiplier is applied, so it covers batch
  accounting (`recordAsyncUsage`, both the download and the reconciler) and
  any synchronous response that reports a tier through
  `models.IPriceAdjustedResponse`. The recorded cost and the budget charge
  both use it. A tier the map does not name keeps the vendor's multiplier;
  a response that reports no tier is never adjusted. `0` bills the tier at
  nothing.
- **Keys:** the tier the vendor reports, as the extension normalises it
  (Anthropic batch results: `batch`, `standard`, `priority`; a result
  without a tier is `batch`; OpenAI: the response's `service_tier`, for
  example `flex`, `priority` or `scale`, on synchronous chat completions and
  Responses calls (JSON and streamed) as on background responses;
  `default` and `auto` are list price and recorded as no tier). OpenAI
  tiers bill at list price (multiplier 1) unless overridden: the vendor
  layer reports the tier (`vendors/openai/service_tier.go`), not a price.
  Lowercase letters, digits, `_` or `-`, up to 32 characters; `default`
  and `auto` are refused for every vendor (400, they could never match).
  Values are 0 to 10.
- **Model and API:** `models.LLM.PriceMultipliers` (`models.LLMPriceMultipliers`,
  a JSON column stored NULL when empty, schema version 10), the
  `price_multipliers` attribute on LLM create and PATCH (an absent key keeps
  the stored map, a sent map replaces it, `null` or `{}` removes it, invalid
  input is 400), passed to the service as `services.WithPriceMultipliers`.
- **Admin UI:** the LLM form's "Price multipliers" editor
  (`ui/admin-frontend/src/admin/components/llms/LLMPriceMultipliersField.js`)
  edits tier/multiplier rows, shown for Anthropic and OpenAI (tier
  suggestions `batch`/`standard`/`priority` and `flex`/`priority`/`scale`; the
  first added row suggests `batch` or `flex`) and for any LLM that already has
  overrides. It validates the server's key and 0-10 rules, rejects `default`
  and `auto` like the server (recorded as no tier), sends the whole map
  (or `null` when no rows remain) and shows a server 400 under the editor. The
  LLM details page lists the overrides ("batch ×0.4").
- **Edges:** synced as `LLMConfig.price_multipliers` (JSON string, empty when
  none) into the edge's `llms.price_multipliers`, and decoded onto the
  runtime LLM by the gateway adapter, because the edge prices its own usage
  and the pulse carries the cost already multiplied.
- **Existing records:** an override is not retroactive. Records keep the cost
  they were written with, and a model price edit with recalculation keeps
  each record's ratio to list cost (see [Accounting](#accounting)).
- **Per LLM, not per environment:** an environment variable would have to be
  set on every edge, and the hub and edges could disagree on a batch's cost;
  an LLM setting travels with the config sync.

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
- **Responses ownership:** an OpenAI response ID names only the App and LLM
  it was handed to, so `previous_response_id`, retrieve, cancel, input items
  and delete refuse every other App's responses with the same 404.
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
- **Ambiguous settle commit.** `settleNativeWithRetry` retries a settle that
  returned a store error. If that settle had in fact committed (the connection
  dropped after the commit), the retry matches no `claimed` row and reports a
  lost claim. The same happens when that attempt committed and every later
  retry errors. The batch is then `accounted` with no chat records and no
  spend: an undercount, never a double charge. It needs an ambiguous commit
  error, so it is rare. To spot it, look for an `accounted` row in
  `native_resources` with no `llm_chat_records` row for its `app_id` and
  `llm_id` written within seconds of its `accounted_at` (usage is recorded
  right after the settle). Don't key on the proxy log's `native_resource`
  summary: it is empty on LLMs with `dont_log_bodies`. Making `Settle`
  idempotent for its token closes it (tracked as a follow-up).
- **Lost edge SQLite.** Batches whose edge lost its SQLite are never accounted.
- **Expired results.** Results expired before anyone read them make the batch
  `unaccountable`, with a warning.
- **Signing keys during a rollout.** An edge older than schema version 10
  ignores the signing secret and keeps signing with the vendor-credential key.
  The hub and updated edges accept those IDs only while the backfill's
  vendor-credential grace lasts (30 days from the upgrade). IDs the hub mints
  verify on an old edge only through the ledger fallback. Standalone
  microgateways (no hub) have no secret and sign with the vendor-credential
  key, so rotating their vendor credential still invalidates outstanding IDs.
- **One previous key.** A rotation keeps only the key it replaced; a second
  rotation within the grace period revokes the first key early.
- **Service tier on older rows.** Chat records and proxy logs written before
  schema version 10 have a NULL `service_tier`; the tier report counts them
  with the untiered traffic.
- **OpenAI Batches and Files:**
  - Uploads are limited by `NATIVE_API_MAX_BODY_BYTES` (64 MiB by default,
    at most 256 MiB; OpenAI accepts up to 200 MB) and, on an LLM with request
    filters, by `NATIVE_API_MAX_FILTERED_ITEMS` (10,000 lines by default;
    OpenAI accepts 50,000).
  - Only `purpose=batch` uploads are accepted. Other purposes never worked
    through the gateway (the legacy chain wants a JSON body with a model).
  - Files, like batches, are per node: an edge lists and deletes only the
    files created or produced through it.
  - Results downloads are not accounted at download time; the batch is
    accounted by the reconciler within one poll interval of ending.
  - `OpenAI-Organization` and `OpenAI-Project` are not forwarded on any
    Files, Batches, Responses or Conversations operation (none is in
    `nativeHeaderAllow`, and the Responses chain hooks drop them): every
    resource lives in the project of the LLM's API key, so the reconciler
    polls the same project the resource was created in.
  - Uploads must use the plain multipart form the SDKs send (see
    files.create); other encodings are refused with 400.
- **OpenAI Responses:**
  - Response IDs handed out before this change were raw and no longer
    verify: a chain continued from one gets a 404.
  - Item IDs (`msg_...`, `fc_...`, `rs_...`) inside responses and
    `item_reference` inputs are not wrapped (see "Item IDs are not owned"
    above). Conversation IDs are.
  - Resuming a background stream (`GET /v1/responses/{id}?stream=true`) is
    refused; poll the response instead.
  - Every `/v1/responses` call (and `input_tokens`, `compact`) must name
    `model`. A call that relies on a stored prompt's model
    (`{"prompt": {"id": ...}}`) or names only a conversation gets 400
    "model field is required" from the legacy chain's model validation:
    without a model the gateway could not apply the LLM's model allow-list.
    Batch lines must name `body.model` for the same reason.
  - `input_file` parts are not screened by the filters (`file_data`,
    including base64 PDFs and text files, as with files on Chat
    Completions), and a `file_id` in an `input_file` or `input_image` part is
    forwarded unverified. Only `purpose=batch` files can be uploaded through
    the gateway, so such an ID was obtained outside it.
  - The in-progress cap (`NATIVE_API_MAX_IN_PROGRESS_PER_APP`) does not apply
    to background responses (the chain cannot tell a background request
    before the vendor answers).
  - Flex-tier pricing is not applied: responses, background or not, are
    billed at the list price, as chat completions are.
  - An OpenAI-compatible server whose `/v1/responses` IDs are not `resp_...`
    gets a 502 for its creates, since the gateway will not hand out an ID it
    cannot sign.

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

## Testing

| Area | Tests |
|---|---|
| Contract, matcher, codec, JSONL, duplicate-key check | `pkg/nativeapi/*_test.go` |
| Ledger compare-and-set (SQLite and Postgres via `DATABASE_URL`) | `services/native_resource_store*_test.go` |
| Dispatch, create, results, list and delete, reconciler, metrics | `proxy/native_*_test.go` |
| Anthropic spec | `vendors/anthropic/nativeapi*_test.go` |
| Chain hooks contract | `pkg/nativeapi/chainhooks_test.go` |
| OpenAI spec, Responses usage analysis | `vendors/openai/nativeapi_test.go` |
| OpenAI Batches and Files: upload parsing and rebuild, results lines, batch states | `vendors/openai/batches_test.go` |
| OpenAI Batches and Files end to end (ownership, admission, children, accounting, delete guard; filters in the `_ent` file) | `proxy/native_openai_batches*_test.go` |
| Chain hooks end to end (ownership, streams, background accounting, duplicate keys on `/v1/messages`), event rewriter | `proxy/native_chain_test.go` |
| Responses filter extraction and write-back (unit; end to end with filters in the `_ent` file) | `proxy/responses_filters*_test.go` |
| Embeddings, completions and moderations filter extraction (unit; end to end with filters in the `_ent` file) | `proxy/openai_input_filters*_test.go` |
| Conversations ownership | `proxy/native_conversations_test.go`, `vendors/openai/nativeapi_test.go` |
| Edge store wiring | `microgateway/internal/server/native_store_test.go` |
| Service tier: recording, OpenAI sync tiers, edge row, pulse, hub ingest, report and endpoint | `proxy/native_service_tier_test.go`, `vendors/openai/service_tier_test.go`, `microgateway/internal/services/analytics_handler_service_tier_test.go`, `microgateway/internal/plugins/analytics_pulse_service_tier_test.go`, `grpc/analytics_pulse_service_tier_test.go`, `analytics/service_tier_test.go`, `api/service_tier_analytics_handlers_test.go` |
| Price multiplier overrides: model, accounting, API, snapshot, edge sync | `models/llm_price_multipliers_test.go`, `proxy/native_price_override_test.go`, `api/llm_price_multipliers_test.go`, `grpc/snapshot_signing_test.go`, `microgateway/internal/services/edge_sync_signing_test.go` |
| Workspace pin: vendor hook, every upstream call (sync, native, reconciler), langchaingo driver, API, snapshot, edge sync | `vendors/anthropic/workspace_test.go`, `switches/anthropic_workspace_test.go`, `proxy/native_workspace_test.go`, `api/llm_workspace_test.go`, `grpc/snapshot_signing_test.go`, `microgateway/internal/services/edge_sync_signing_test.go` |
| Signing keys: model, codec keyring, rotation end to end, API, snapshot, edge sync | `models/llm_resource_signing_test.go`, `pkg/nativeapi/keyring_test.go`, `proxy/native_signing_test.go`, `api/llm_signing_handlers_test.go`, `grpc/snapshot_signing_test.go`, `microgateway/internal/services/edge_sync_signing_test.go`, `microgateway/internal/providers/grpc_provider_failover_test.go` |
| Configuration | `config/native_api_test.go`, `microgateway/internal/config/native_api_test.go` |
