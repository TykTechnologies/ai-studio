# Tyk AI Studio 2.1.0

**Release date:** 2026-05-14
**Diff:** [`v2.0.0...main`](https://github.com/TykTechnologies/ai-studio/compare/v2.0.0...main)

This is the first feature release on the 2.x line. It adds AWS Bedrock as a first-class LLM vendor across both the embedded gateway and the microgateway, surfaces filter-script compliance events in the dashboard, introduces an OAuth2 client-credentials enterprise plugin, and ships a community rate-limiter plugin alongside a long list of analytics, secret-interpolation, and supply-chain fixes.

---

## Highlights

- **AWS Bedrock end-to-end.** Native AWS SDK driver, OpenAI-compatible translation, microgateway native protocol pass-through, full streaming, and Bedrock-aware UI in the LLM form.
- **Compliance events from filter scripts.** Tengo filter scripts can now emit `compliance_events` (PII redactions, content rewrites, guardrail triggers) that flow through analytics into a new Filter Events dashboard tab and CSV export.
- **OAuth2 client-credentials enterprise plugin.** Validates JWTs from external IdPs (Auth0, Microsoft Entra ID, Okta) and auto-provisions Apps from templates mapped to IdP permission tags.
- **Community rate-limiter plugin.** Sliding-window rate limiting with composable key dimensions, match conditions, shadow mode, Redis backend, atomic concurrent counters, and optimistic locking.
- **Prometheus / OpenTelemetry metrics endpoint** on both AI Studio and the microgateway.
- **Per-LLM body capture toggle** (`DontLogBodies`) for privacy/compliance-sensitive providers.
- **Supply-chain hardening** across all GitHub Actions, Dockerfiles, npm/pip installs, and Go dependencies (6 CVEs patched).

---

## New features

### LLM proxy and microgateway

- **AWS Bedrock driver** for the microgateway LLM proxy ([#357](https://github.com/TykTechnologies/ai-studio/pull/357)). Uses AWS SDK v2 `Converse` and `ConverseStream` APIs directly for universal model support, with OpenAI ↔ Converse translation for `/ai/v1/chat/completions`, SDK-mediated streaming for `/llm/stream/`, and SigV4-signed reverse proxy for `/llm/rest/`. Token usage extracted on-the-fly from streaming metadata events, with per-LLM client caching.
- **Bedrock native protocol support** in the microgateway ([#363](https://github.com/TykTechnologies/ai-studio/pull/363)). Native AWS SDK clients (boto3 etc.) can call Bedrock through the microgateway transparently — responses re-encoded as binary event stream so unmodified SDK clients work, with SigV4 auth extraction and `bedrock-mantle` cross-region endpoint support.
- **Per-LLM body capture toggle** ([#355](https://github.com/TykTechnologies/ai-studio/pull/355)). New `DontLogBodies` flag on the LLM model clears request and response bodies before storage in `ProxyLog`, `LLMChatLogEntry`, and analytics events across both the embedded gateway and microgateway. Defaults to `false` for backward compatibility.
- **Prometheus / OpenTelemetry metrics endpoint** ([#356](https://github.com/TykTechnologies/ai-studio/pull/356)). New `/metrics` endpoint exposes:
  - `aistudio_llm_requests_total`, `aistudio_llm_tokens_total`, `aistudio_llm_cost_total`, `aistudio_tool_calls_total`, `aistudio_policy_blocks_total` (counters)
  - `aistudio_llm_request_duration_seconds`, `aistudio_tool_execution_duration_seconds` (histograms)
  - `aistudio_llm_inflight_requests` (gauge)
  - `aistudio_compliance_events_total` (counter)

### Compliance and governance

- **General-purpose compliance event reporting from filter scripts** ([#371](https://github.com/TykTechnologies/ai-studio/pull/371)). Filter scripts set `compliance_events` in their Tengo output to record redactions, rewrites, PII detection, or silent failures with severity (`info` / `warning` / `critical`) and structured metadata. Events flow through an async analytics pipeline, are stored to a new `compliance_events` table, propagate edge → control via the gRPC analytics pulse, and are exposed through `GET /compliance/events` with filter and aggregation support.
- **Compliance events surfaced in the dashboard** ([#375](https://github.com/TykTechnologies/ai-studio/pull/375)). Previously the compliance dashboard only counted 4xx proxy logs, so script-emitted events were invisible. The dashboard now reports Critical/Warning event counts and trends, splits Blocked vs Flagged in Policy Violations, weights per-app risk scores by event severity, includes events in the app drill-down, and adds a new Filter Events tab with severity/event-type filters, timeline chart, expandable metadata, and CSV export.
- **Updated filter-script library** ([#372](https://github.com/TykTechnologies/ai-studio/pull/372)). All pre-provided scripts (PII redaction, content blocking, response guardrails) and the embedded UI templates now demonstrate compliance event reporting with consistent metadata keys (`matched_pattern`, `redacted_types`).

### Plugin SDK

- **`StoreApp` on `GatewayServices`** ([#366](https://github.com/TykTechnologies/ai-studio/pull/366)). Gateway plugins can write Apps directly into the local SQLite database with upsert semantics including LLM, Tool, and Datasource associations. Enables dynamic provisioning from auth plugins without waiting for periodic config sync. Requires `apps.write` scope in the plugin manifest.
- **`ListModelPrices` / `GetModelPricesByVendor` SDK wrappers** ([#369](https://github.com/TykTechnologies/ai-studio/pull/369)), used by the new community `llm-price-sync` plugin.
- **UTF-8 sanitization at the gRPC boundary** ([#361](https://github.com/TykTechnologies/ai-studio/pull/361)). New `SanitizeUTF8()` utility applied in `Call()` and `PortalCall()` protects all plugins from `string field contains invalid UTF-8` gRPC marshaling errors when wrapping `[]byte` RPC responses.

### UI / UX

- **Export chat conversations to PDF** ([#349](https://github.com/TykTechnologies/ai-studio/pull/349)) — print button on both `ChatView` and `AgentChat`.
- **Bedrock LLM form** — vendor dropdown entry with logo and default endpoint, labelled AWS credential fields (Access Key ID, Secret Access Key, Session Token) that pack into `metadata` on save and extract on load. Helper text guides users toward `$SECRET/` references for credential storage.
- **Sensitive metadata redaction in API responses.** LLM responses now include the `metadata` field with values redacted for keys containing `secret`, `token`, or `key`, unless they use `$SECRET/` references (which are passed through).

### Enterprise plugins (submodule)

- **OAuth2 client-credentials auth plugin.** Validates JWT access tokens from external IdPs (Auth0, MS Entra ID, Okta) and maps them to AI Studio Apps for gateway access control. Features JWKS caching with OIDC auto-discovery, multi-provider configuration with claim mappings, template-based App auto-provisioning (admins create template Apps and map them to IdP permission tags), per-key mutex to prevent duplicate provisioning under concurrency, KV-based app lookup cache, and a Studio admin UI for provider CRUD. Tested end-to-end against Auth0 with live token validation. Includes runtime enterprise license enforcement via `plugin_sdk.GetLicenseInfo`.

### Community plugins (submodule)

- **Rate-limiter plugin.** Sliding-window rate limiting for LLM requests with composable key dimensions, request/token/concurrent counters, shadow mode, fail-open behavior, and Redis backend support. Rules support match conditions for entity-specific targeting (e.g. `llm_id=2`, `app_id=10`) and propagate to edge gateways via config snapshots instead of runtime-local KV. Atomic concurrent counters via Lua-backed `IncrementIfBelow`, optimistic locking on rulesets, bounded striped lock pool, and accurate sliding-window `reset_at` (now + window) for `Retry-After`.
- **LLM price sync plugin.** Scheduled plugin that fetches LLM pricing from llm-prices.com and syncs it into the control plane's `model_prices` table. Supports vendor filtering, dry-run mode, auto-create toggle, and ships with a dashboard UI.

---

## Improvements

### Analytics and logging

- **Proxy logs attributed to the specific LLM, not the vendor** ([#376](https://github.com/TykTechnologies/ai-studio/pull/376)). Previously two LLM entries sharing a vendor (e.g. a personal Anthropic key and a work Anthropic key) cross-pollinated in the LLM detail page because `GetProxyLogsForLLM` filtered by vendor string. Added an `LLMID` column to `ProxyLog` with a composite `(llm_id, time_stamp)` index, populated at every construction site that has the LLM in scope, and switched the query to filter by `llm_id` directly. Historical rows have `llm_id = 0` and remain unattributed (pre-date the column).
- **`LLMID` propagated through the analytics pulse** ([#377](https://github.com/TykTechnologies/ai-studio/pull/377)). Studio-side gRPC pulse reconstruction now copies `LlmId` from the proto onto `ProxyLog`, so edge-pulsed logs are visible in the per-LLM view.
- **Context propagation through analytics** for OTEL trace–metric correlation: request contexts (or `context.WithoutCancel` for async goroutines) flow through `AnalyzeResponse`, `AnalyzeStreamingResponse`, and `SendAnalyticsPulse` instead of using `context.Background()`.

### Secret interpolation

- **`$SECRET/` and `$ENV/` references in LLM `Metadata`** — matches existing behavior on `APIKey`/`APIEndpoint`. Allows Bedrock AWS credentials to be encrypted at rest via the secrets manager. Applied in all four LLM retrieval paths (`GetLLMByID`, `GetActiveLLMs`, `GetLLMsInNamespace`, `GetActiveLLMsInNamespace`).
- **Datasource secrets resolved in all gRPC data-plane operations.** New `GetDatasourceByIDResolved` service method bypasses preserve-ref behavior so `GenerateEmbedding`, `StoreDocuments`, `ProcessAndStoreDocuments`, `QueryDatasourceByVector`, `DeleteDocumentsByMetadata`, `QueryByMetadataOnly`, `ListNamespaces`, `DeleteNamespace`, and the REST `ProcessRAGForDatasource` handler authenticate with real credentials. `QueryDatasource` in `ai_studio_management_server` also fixed (was bypassing secret resolution entirely via raw DB query).
- **Chat resume and datasource embedding keys** ([#359](https://github.com/TykTechnologies/ai-studio/pull/359)). `loadExistingSession` now reloads via the service layer so `$SECRET/` references resolve, and `AddDatasource` resolves `EmbedAPIKey` and `DBConnAPIKey` after loading.
- **LLM `Metadata` resolved during microgateway gRPC config sync** so Bedrock AWS credentials reach edge deployments correctly.

### Plugin reliability

- **Plugins reload when configuration changes** ([#344](https://github.com/TykTechnologies/ai-studio/pull/344)) — no more stale plugin state after config updates.
- **Soft-delete conflicts on plugin resources fixed** ([#346](https://github.com/TykTechnologies/ai-studio/pull/346)).

---

## Bug fixes

- **Bedrock OpenAI-compatible endpoint auth and analytics.** Credential validation middleware now applied to `/ai/` routes so the direct AWS SDK path gets app context for auth, budget, and logging. Proxy logs now recorded for both Bedrock streaming paths (previously only chat records were written). Text buffers always accumulate for logging regardless of response filters. `ModelName` populated for all Bedrock paths and in the control server pulse-to-proxy-log conversion. Context cancellation on client disconnect after stream completion no longer logged as an error. `context.WithoutCancel` used for async analytics so records aren't lost when HTTP context is canceled after the response is sent. Budget analysis triggered after Bedrock chat record creation. ProxyLog written before ChatRecord in streaming goroutines so the gateway analytics merge pipeline works correctly.
- **Tool `AuthKey` redacted in API responses** ([#348](https://github.com/TykTechnologies/ai-studio/pull/348)). Previously returned in plaintext on GET/LIST/mutation responses — visible in browser devtools. Now follows the same `[redacted]` + `has_auth_key` pattern as LLM and Datasource handlers; PATCH echo of `[redacted]` is ignored so the stored key is preserved.
- **CSRF fixes** ([#350](https://github.com/TykTechnologies/ai-studio/pull/350)). `localhost:3000` added as a trusted origin in dev mode so the frontend dev proxy no longer trips `gorilla/csrf`. Gin middleware now properly aborts the handler chain on CSRF failure, preventing handlers from executing after rejection. CSRF in the Community QuickStart resolved separately ([#368](https://github.com/TykTechnologies/ai-studio/pull/368)).
- **Quickstart Docker Compose fixes** ([#379](https://github.com/TykTechnologies/ai-studio/pull/379)). CE and EE quickstart compose files referenced `tykio/microgateway`, which is not where the release workflow publishes images — corrected to `tykio/tyk-microgateway` (CE) and `tykio/tyk-microgateway-ent` (EE), with the EE studio reference aligned to `tykio/tyk-ai-studio-ent`. All four images pinned to v2.0.0. `wget`-based healthchecks removed (published images are distroless with no shell, so the healthchecks always failed and blocked the microgateway from starting via `depends_on: service_healthy`). Enterprise quickstart port also fixed.
- **Manual marketplace sync now cache-busts the GitHub CDN** ([#362](https://github.com/TykTechnologies/ai-studio/pull/362)) — adds a `_cb=<timestamp>` query parameter and skips conditional headers when sync is user-triggered, forcing a fresh response. Background and scheduled syncs continue using efficient conditional requests.
- **Enterprise: `FetchIndexConditional` missing `forceRefresh` argument** fixed in the enterprise submodule.

### Community submodule fixes

- **Simple chunker no longer splits multi-byte UTF-8 characters.** `getOverlap()` was slicing strings by byte offset, producing chunks starting with orphaned continuation bytes and breaking `proto.Marshal`. Slice boundary now advances to the next valid UTF-8 character start.
- **File processing sanitizes non-UTF-8 bytes** before chunking, with an improved binary file detection heuristic — fixes "string field contains invalid UTF-8" errors on Latin-1 and other non-UTF-8 files.
- **Rate-limiter robustness:** `saveRuleSetToKV` now returns an error when existing data fails JSON unmarshal instead of silently overwriting. Consistent JSON format across Redis and KV backends (Lua scripts replace `INCRBY` plain-int writes so `ReadConcurrentState` works against both). Full SHA256 (64 hex chars) API key hash eliminates birthday-problem collision risk. Optimistic locking via version numbers on `RuleSet` rejects stale writes with `ErrVersionConflict`. Bounded `[256]sync.Mutex` striped lock pool replaces unbounded `map[string]*sync.Mutex` so memory is O(1) regardless of key cardinality.

---

## Security

### CVE fixes ([#334](https://github.com/TykTechnologies/ai-studio/pull/334))

| Dependency | From | To | Advisory |
|---|---|---|---|
| `go.opentelemetry.io/otel/sdk` | v1.34.0 | v1.40.0 | GO-2026-4394 (arbitrary code execution) |
| `eclipse/paho.mqtt.golang` | v1.2.0 | v1.5.1 | GO-2025-4173 (string encoding overflow) |
| `gorilla/csrf` | v1.7.2 | v1.7.3 | GO-2025-3607 (CSRF bypass) |
| `redis/go-redis/v9` | v9.5.3 | v9.6.3 | GO-2025-3540 (out-of-order responses) |
| `filippo.io/edwards25519` | v1.1.0 | v1.1.1 | GO-2026-4503 |
| `golang.org/x/crypto` | v0.43.0 | v0.45.0 | GO-2025-4134, GO-2025-4135 |

### Supply-chain hardening ([#364](https://github.com/TykTechnologies/ai-studio/pull/364), [#365](https://github.com/TykTechnologies/ai-studio/pull/365))

- All GitHub Actions pinned to full commit SHAs.
- `TykTechnologies/github-actions` pinned to `@42304ed`.
- `npm install` → `npm ci --ignore-scripts` everywhere (Dockerfile, docker-compose, CI).
- `--no-deps` added to all `pip install` commands.
- `go install` tool versions pinned to release tags.
- All Docker base images pinned by digest.
- `requirements.txt`: `>=` → `==` pinning.
- `curl | bash` flagged in dev Dockerfile.
- `go mod verify` step added to CI.
- SHA-pinned Docker output tags alongside `:latest` in `prod.yml`.
- Reusable `dep-guard` workflow replaces direct `dependency-review-action` usage; build jobs gated behind it.

### CI auth migration ([#373](https://github.com/TykTechnologies/ai-studio/pull/373), [#374](https://github.com/TykTechnologies/ai-studio/pull/374))

- Migrated from `ORG_GH_TOKEN` PAT to GitHub App tokens for enterprise submodule checkout and `git config insteadOf` in `release.yml` and `prod.yml`.
- Fixed `x-access-token:` prefix and trailing slash on `url.insteadOf` so it works when `actions/checkout` has set up a credential helper.

---

## CI / infrastructure

- Migrated CI to warpbuild runners ([#345](https://github.com/TykTechnologies/ai-studio/pull/345)).
- SBOM generation step removed ([#347](https://github.com/TykTechnologies/ai-studio/pull/347)).
- GitHub Actions SHA bumps for pinned upstream actions.
- Community submodule CI: SentinelOne CNS vulnerability scan, Visor automated code review, matrix CI that discovers and tests each plugin module with coverage upload, `go mod tidy` step before plugin builds to fix stale `go.sum` entries after CI rewrites `replace` directives.

---

## Submodule bumps

- `community`: `1bba94a` → `d88481e`
- `enterprise`: `295601c` → `0a13253`

---

## Upgrade notes

- **Database migration.** GORM auto-migration on startup adds the `llm_id` column to `proxy_logs` (with a composite index on `(llm_id, time_stamp)`) and creates the `compliance_events` table. Historical proxy log rows have `llm_id = 0` and will not appear in per-LLM detail views — this is intentional, they pre-date the column.
- **Bedrock credentials.** AWS credentials for Bedrock LLMs are stored in the `metadata` map (`aws_access_key_id`, `aws_secret_access_key`, `aws_session_token`) rather than `APIKey`. The LLM form handles packing/unpacking transparently. Use `$SECRET/NAME` references for credentials at rest.
- **`AnalyticsHandler` interface** now takes `context.Context` on its recording methods — plugin authors implementing custom handlers will need to update signatures.
- **Quickstart users:** if you were running off the v2.0.0 quickstart compose files with `tykio/microgateway:*` image references and failing healthchecks, pull the v2.1.0 quickstart compose and the corrected `tykio/tyk-microgateway[-ent]` images.
