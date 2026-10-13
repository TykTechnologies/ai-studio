# Tyk AI Studio 2.3.0

**Status:** Unreleased. This file collects what has merged to `main` since 2.2.1 and is updated as more merges land.
**Diff so far:** [`v2.2.1...main`](https://github.com/TykTechnologies/ai-studio/compare/v2.2.1...main)

2.3.0 so far adds three capabilities, a chat billing fix and one edge fix.

- **Decision models.** AI Studio now governs a new class of model that answers typed questions about a piece of evidence, with calibrated probabilities, instead of generating text. OpenAI's Decisions API (`gpt-6-luna`) and System One models (TypeSafe's Jev and wire-compatible self-hosts) are supported. Clients use one contract, the **OpenAI Decisions schema on `/v1/decisions`**, whatever the provider. Auth, filters, plugins, budgets and analytics apply to decision calls as they do to chat.
- **Tyk OSS Gateway connections (Enterprise).** AI Studio can manage MCP proxies and keys directly on open-source Tyk Gateways, without a Tyk Dashboard.
- **Native SDK features.** Anthropic Message Batches and `count_tokens`, and OpenAI's Responses, Conversations, Batches and Files APIs, now work through the native `/llm/` endpoints with the same governance as a chat call: filters, model allow-lists, budgets, analytics, and resources that belong to the App that created them.
- **Chat billing.** Chat sessions are priced like gateway calls: Anthropic turns with tool calls are no longer billed two or three times, and cached tokens are billed at the cache price.
- **Edge stability.** Two data races in the edge's control-plane client are fixed. One of them left an extra heartbeat worker running after every reconnect.

Read **Breaking changes and upgrade notes** before upgrading. Edges must run 2.3.0 to serve decision models, and should be upgraded with the hub for native API resource IDs.

---

## Highlights

- **Decision models** ([#724](https://github.com/TykTechnologies/ai-studio/pull/724)).
  - **One client contract:** `POST {base}/decisions` on the unified `/v1` endpoint and `POST /ai/{slug}/v1/decisions` take OpenAI's Decisions schema. Each call is translated to the provider's own format and back.
  - **New System One provider:** decisions only, for TypeSafe's hosted API or a self-hosted model.
  - **Governed like chat:** each call is logged, priced and counted against budgets. A new Dashboard panel reports decision calls, questions answered, refusals and cost.

  See `docs/site/docs/decision-models.md`.
- **Tyk OSS Gateway connections** (Enterprise, [#721](https://github.com/TykTechnologies/ai-studio/pull/721), [ai-studio-enterprise#70](https://github.com/TykTechnologies/ai-studio-enterprise/pull/70)). A new connection kind for teams that want governed MCP without the full Tyk Dashboard APIM.
  - Open-source Gateways have no cluster API, so AI Studio stores the proxies it owns, finds the nodes on every sync (a single URL, a list, or DNS) and reconciles each node. A new **Nodes** panel shows each node as In sync, Pending, Unreachable or Gone.
  - Keys carry their rights inline rather than through policies, so a key works on every node at once. Each MCP server has a **Key access** section for the tool allow-list, rate and quota.
  - **MCP only:** every Gateway call is checked against a fixed allow-list, and AI Studio never changes or deletes proxies it did not create.

  See `docs/site/docs/tyk-oss-gateway-mcp.md`.
- **Native API extensions** ([#730](https://github.com/TykTechnologies/ai-studio/pull/730), [#737](https://github.com/TykTechnologies/ai-studio/pull/737)). SDK features that are not a single synchronous `POST` now work on `/llm/{rest,stream,call}/{slug}/...`, through a small extension framework (`pkg/nativeapi`).
  - **Anthropic:** Message Batches (create, retrieve, list, cancel, results, delete) and `count_tokens`.
  - **OpenAI:** Responses (including background responses), Conversations, and Batches with batch input Files.
  - **Governed like chat:** every batch request and uploaded batch line passes the model allow-list and the LLM's filters, results lines pass the response filters, and batch usage is recorded once, at the batch price, on the hub or the edge that created it.
  - **Owned resources:** batch, file, response and conversation IDs are signed for the App and LLM that created them. Another App's ID gets `404`, and the request never reaches the vendor.

  See `docs/site/docs/proxy.md` and `features/NativeAPIExtensions.md`.

## New features

### Decision models
- **Unified ingress `POST {base}/decisions`** (OpenAI schema, `route/model` naming as for chat) and **`POST /ai/{slug}/v1/decisions`** ([#724](https://github.com/TykTechnologies/ai-studio/pull/724)). The bridge validates the request, shapes the body for the provider and calls the native `/llm/call/` hop, so auth, filters, plugins, budgets and analytics run once, on that hop.
- **`systemone` vendor** (System One, decisions only). The endpoint defaults to `https://api.typesafe.ai`. A self-host (llama.cpp serving Kev, OpenJev, Laya and others) sets its own endpoint and may omit the key.
- **`pkg/decisions`:** a dependency-free translator between the OpenAI and System One formats.
  - Question and option order are preserved.
  - A question the model leaves unanswered becomes a `refusal`.
  - System One's `422` and `529` answer as OpenAI-style `400` and `503`.
- **Vendor capabilities.** The vendor interface gains `ProvidesChat()` and `DecisionEndpoint()`. Entries from `GET /v1/models` gain a `capabilities` field listing what the route serves (`chat`, `embeddings`, `decisions`).
- **Failover per request kind.**
  - A decision call skips fallbacks that serve no decision models, or that cannot take the request (image input to a text-only System One model), and is reshaped for each fallback's provider.
  - A chat call skips decision-only fallbacks.
  - A skipped fallback never replaces the primary's own error.
- **Analytics.**
  - `request_kind`, `decision_questions` and `decision_refusals` on `proxy_logs`, `llm_chat_records` and the edge's `analytics_events`, carried to AI Studio in the edge pulse.
  - `GET /api/v1/analytics/decisions` and the Dashboard **Decision Models** panel.
  - The Prometheus metric `aistudio_decision_questions_total{vendor,model,outcome}`, with `outcome` `answered` or `refused`.
- **Filters and guardrails.**
  - A decision request is read as messages: each question's instructions as a system message, and the evidence as the last user message.
  - Redactions are written back in the provider's own format.
  - Response filters see the answers rendered as text.
  - Scripts get `input.context.request_kind`.
- **Plugins and SDK.**
  - `PluginContext.request_kind` is set on edge auth, middleware and response hooks, and data-collector requests carry the decision fields.
  - The plugin SDK adds `ParseDecisionRequest` and `ParseDecisionResponse`, which normalise either wire format to the OpenAI shape.
  - It also adds `CallDecision`, backed by a new `CallDecision` management RPC with the same agent access checks as `CallLLM`.
- **Admin console.** The LLM form offers the System One provider. Chat, router and wizard pickers leave decision-only LLMs out, and the failover picker only offers fallbacks that share a request kind with the primary.
- **Live conformance.** `make test-vendors-decisions` runs the decision suite against real providers, configured in `test-secrets/vendors.env` (see `features/VendorConformance.md` §10).

### Tyk connections (Enterprise)
- **Gateway connection kind** ([#721](https://github.com/TykTechnologies/ai-studio/pull/721), [ai-studio-enterprise#70](https://github.com/TykTechnologies/ai-studio-enterprise/pull/70)).
  - The connection form has a **Connect to** toggle (Dashboard or Gateway), a **Nodes** section and a **Gateway nodes** table.
  - Discovery is `single`, `static` or `dns`, with an optional shared-storage mode that writes once and reloads the group.
  - Nodes are reconciled on every sync: new and empty nodes are filled in, and edits made behind AI Studio's back are restored.
  - Requires Tyk Gateway 5.13 or later.
- **Key access per MCP server**, with `PUT /api/v1/mcp-servers/{id}/key-access`. Saving it rewrites live keys.
- **Node status**, with `GET /api/v1/tyk-connections/{id}/nodes`.
- **Partial proxies.** An operator proxy found on only some nodes is flagged "On n/m nodes". It is never copied, gets no keys and cannot be bound to an App.
- **Performance and safety.**
  - Gateway clients are pooled per connection and node.
  - Work on nodes runs in parallel, at most 8 at a time. Shared storage stays sequential.
  - A literal upstream credential (rather than a `$secret_env` reference) on a gateway connection shows a warning.
- **`pkg/netguard`:** `NewPinnedHTTPClient` reaches one of several servers behind a hostname. It dials one resolved address while the URL, `Host` header and TLS name keep the hostname, and applies the internal-address rule to the hostname.

### Native API extensions
- **Extension framework** ([#730](https://github.com/TykTechnologies/ai-studio/pull/730)). `pkg/nativeapi` lets a vendor declare SDK operations (exact method and path, the resources they create, how items are admitted and results read) without changing the proxy's generic code. A native route claims only the paths its vendor declares, and everything else stays on the existing chain unchanged. Resource IDs look like `msgbatch_01Hk….t1k3f2a9c1d.9b…`: they are signed and verify on any replica, hub or edge without shared state.
- **Anthropic Message Batches** ([#730](https://github.com/TykTechnologies/ai-studio/pull/730)).
  - Create: each `requests[].params.model` must be allowed (403 naming the `custom_id`), and request filters run on every request with `custom_id` and `batch_index` in the script context. Only requests a filter changed are re-encoded.
  - Results are streamed line by line, response filters run on each line, and a blocked line becomes an `errored` result with the same `custom_id`.
  - Usage is recorded once per batch at half the list price, by whichever comes first: a complete results download on the creating node, or a background reconciler. Edge usage reaches AI Studio budgets and analytics through the existing pulse.
  - List is served from the gateway's own ledger (`native_resources`), never forwarded. Delete answers `409` until the batch's usage is accounted.
  - `results_url` points at the gateway, absolute when `PROXY_URL` (edge: `GATEWAY_PUBLIC_URL`) is set.
- **Anthropic `count_tokens`** ([#737](https://github.com/TykTechnologies/ai-studio/pull/737)). A native operation: the model allow-list and request filters apply, and each call writes one proxy log row and no chat record.
- **OpenAI Responses** ([#737](https://github.com/TykTechnologies/ai-studio/pull/737)).
  - Response IDs are signed in JSON and streamed responses, and `previous_response_id` is checked before the filters run. Retrieve, delete, cancel and `input_items` are owned operations.
  - Background responses are recorded in the ledger and accounted once by the reconciler when they end. Deleting one accounts it first.
  - Usage is recorded for synchronous and streamed calls (it was not recorded at all before; see Bug fixes).
- **OpenAI Conversations** ([#737](https://github.com/TykTechnologies/ai-studio/pull/737)). Signed `conv_` IDs on every `/v1/conversations` route and on the `conversation` parameter of `/v1/responses`. Items stored in a conversation pass the request filters.
- **OpenAI Batches and Files** ([#737](https://github.com/TykTechnologies/ai-studio/pull/737)). `files.create` with `purpose=batch` admits every line of the file (model allow-list, request filters). `batches.create` takes only the App's own file IDs. Output and error files are wrapped and listed with the App's files, and an output file cannot be deleted before its batch is accounted. Usage is recorded once at half the list price.
- **Request screening on OpenAI** ([#737](https://github.com/TykTechnologies/ai-studio/pull/737)). Filters and guardrails now read the prompt of `/v1/responses` (instructions, input items of every built-in type, prompt variables) and of `/v1/embeddings`, `/v1/completions` and `/v1/moderations` (`input`, `prompt`, `suffix`), and redactions are written back into the same field.
- **Per-LLM resource signing keys** ([#737](https://github.com/TykTechnologies/ai-studio/pull/737)). The hub generates a signing secret per LLM, stores it encrypted and syncs it to edges, so rotating a vendor API key no longer invalidates outstanding IDs. Keys can be rotated with a grace period (30 days by default, at most a year) during which the previous key still verifies. The LLM details page shows the key status and has **Rotate signing key**.
- **Anthropic workspace pin** ([#737](https://github.com/TykTechnologies/ai-studio/pull/737)). An LLM's `vendor_workspace_id` (**Anthropic workspace ID** under **Access Details** in the LLM form) is sent as `anthropic-workspace-id` on every call the gateway makes for that LLM: messages, batches, the reconciler, Chat sessions and the `/ai/` and `/v1` bridges, on AI Studio and on edges.
- **Price multiplier overrides** ([#737](https://github.com/TykTechnologies/ai-studio/pull/737)). An LLM's `price_multipliers` maps a service tier to the share of the list price it is billed at, for example `{"batch": 0.4}` for negotiated batch pricing (**Add price override** in the LLM form). It applies to the recorded cost and the budget charge of usage recorded after the change, on AI Studio and on edges.
- **Service tier analytics** ([#737](https://github.com/TykTechnologies/ai-studio/pull/737)). Chat records, proxy logs and edge analytics events carry the tier a call was billed at (`batch`, `flex`, `priority`...). `GET /api/v1/analytics/service-tier-usage` reports requests, tokens and cost by tier and model, and the Dashboard has a **Service Tiers** panel.
- **Metrics** ([#730](https://github.com/TykTechnologies/ai-studio/pull/730)). `aistudio_native_requests_total`, `aistudio_native_request_duration_seconds`, `aistudio_native_results_bytes_total` and `aistudio_native_reconcile_outcomes_total`.

## Bug fixes

### Gateway
- **OpenAI `/v1/responses` calls were not recorded** (issue [#697](https://github.com/TykTechnologies/ai-studio/issues/697), [#737](https://github.com/TykTechnologies/ai-studio/pull/737)). Response analysis rejected the path, so those calls wrote no proxy log, no cost and no budget spend, which let an App use the Responses API past its budget.
- **The service tier of synchronous OpenAI calls was ignored** ([#737](https://github.com/TykTechnologies/ai-studio/pull/737)). `flex` and `priority` calls are now recorded with their tier, so price overrides and the tier report apply to them. With no override they are still billed at the list price.
- **Anthropic `count_tokens` and batch calls wrote zero-cost chat records** ([#730](https://github.com/TykTechnologies/ai-studio/pull/730)). Only a `/messages` path is now read as a Messages call; the others still write their proxy log.
- **Native OpenAI decision calls were not recorded** ([#724](https://github.com/TykTechnologies/ai-studio/pull/724)). `/llm/rest/{openai-llm}/v1/decisions` already passed through, but response analysis rejected the path, so those calls wrote no proxy log, no cost and no budget spend.
- **Calls to a model alias were recorded at zero cost** ([#724](https://github.com/TykTechnologies/ai-studio/pull/724)). When the model a provider reports has no price, the call is now priced by the model that was requested. Jev answers `jev-latest` as `jev-1.13.0`, and OpenAI reports dated snapshots of chat models.

### Analytics
- **Anthropic Chat turns with several content blocks were billed two or three times** ([#736](https://github.com/TykTechnologies/ai-studio/pull/736)). The usage of a turn with text and a tool call (or a thinking block) was counted once per block, in tokens, cost and budget. It is now read once per turn.
- **Cached tokens in Chat sessions were billed at the input price** ([#736](https://github.com/TykTechnologies/ai-studio/pull/736)). Chat-session records now use the same pricing as gateway records: cached tokens are billed at the cache price, and an unset cache price falls back to the input price as before.
- **The cost breakdown of batch records did not add up** ([#730](https://github.com/TykTechnologies/ai-studio/pull/730)). The per-model breakdown is scaled to each record's recorded cost, and OpenAI and Google cache cost with an unset cache price is no longer reported as zero.
- **Model price recalculation** ([#730](https://github.com/TykTechnologies/ai-studio/pull/730)) keeps each past record's ratio to the list price, so batch records stay at half price, and it now runs on SQLite too.

### Control plane and edges
- **Edge control-client data races** ([#725](https://github.com/TykTechnologies/ai-studio/pull/725)).
  - The connected flag was shared between the connect path, the stream's receive loop and the heartbeat worker without synchronisation. A stream that dropped immediately could also be left marked as connected.
  - The heartbeat worker watched the client's current stream context, which a reconnect replaces. The old worker then waited on the new stream and never stopped, so every reconnect left one more heartbeat worker running.

### Tyk connections (Enterprise)
- **A changed base URL now reaches imported servers** ([ai-studio-enterprise#70](https://github.com/TykTechnologies/ai-studio-enterprise/pull/70)). Changing a connection's public base URL updates the portal URL of every server on the next sync. Before, servers kept the old URL until their definition changed. This applies to Dashboard connections too.
- **Custom domains are honoured.** A proxy with an enabled custom domain gets that host as its portal URL. It used to get a URL that returned 404.

### Deployment
- **The CE quickstart works with the 2.2 images** (`725d561a`).
  - The images are distroless and run as a non-root user from `/opt`. The CE quickstart still used the `/app` layout.
  - The edge dialled port 9080 while CE AI Studio listened on 50051, so the edge crash-looped.
  - The plugin cache directory was unwritable, so OCI plugins and the marketplace were off.
  - CE now uses the enterprise quickstart's `/opt` layout, and the database stays at the same host path.
- **The Helm chart, quickstarts and deployment docs default to v2.2.1** (`725d561a`).

## Security
- **Unauthenticated admin access to the microgateway, leading to malicious plugin loading** ([#731](https://github.com/TykTechnologies/ai-studio/pull/731), also released as 2.2.2). `POST /api/v1/auth/token` needed no credentials and issued a token for any App with any scopes, `admin` included. Anyone who could reach a microgateway's HTTP port could use it to load a malicious plugin of their own onto the gateway and attach it to its LLMs, where it sees and can change every prompt and response. They could also change LLMs, Apps, filters and prices, read analytics with stored bodies, and mint tokens. Edges and standalone microgateways were both affected. The route is removed in every mode, and an edge gateway (`GATEWAY_MODE=edge`) now mounts no `/api/v1` at all, since its configuration comes only from the control plane. See `CHANGELOG-v2.2.2.md` for what to check on existing gateways.
- **Gateway connections are MCP only** ([#721](https://github.com/TykTechnologies/ai-studio/pull/721)). Every call to a Tyk Gateway goes through a fixed allow-list, and AI Studio never changes or deletes proxies it did not create.
- **Proxies that AI Studio creates on Gateways set `stripAuthorizationData`**, so a client's Tyk key is not forwarded to the upstream MCP server. Dashboard connections do not; see Known issues.
- **The internal-address rule applies to every Gateway node.** With DNS discovery, AI Studio reaches each node by its resolved address. Whether an internal address may be dialled is still decided for the connection's hostname, as for any other outbound call.
- **Cross-App reuse of OpenAI responses and conversations** ([#737](https://github.com/TykTechnologies/ai-studio/pull/737)). Any App sharing an OpenAI LLM could continue another App's conversation (`previous_response_id`, `conversation`) or read and delete its stored responses and conversations. IDs are now signed for the App and LLM; another App's ID gets `404` without reaching OpenAI. Anthropic batches got the same ownership in [#730](https://github.com/TykTechnologies/ai-studio/pull/730).
- **Filters saw an empty prompt** on OpenAI `/v1/responses`, `/v1/embeddings`, `/v1/completions` and `/v1/moderations`, and on batch requests ([#730](https://github.com/TykTechnologies/ai-studio/pull/730), [#737](https://github.com/TykTechnologies/ai-studio/pull/737)), so filters and guardrails did not screen those calls. With request filters attached, a prompt the gateway cannot read is now refused rather than passed unscreened.
- **Batch file upload smuggling** ([#737](https://github.com/TykTechnologies/ai-studio/pull/737)). Multipart forms that another parser could read differently (escaped or RFC 2231 field names, repeated or unknown fields, data after the closing boundary) are refused, so an unscreened part cannot become the vendor's batch file.
- **Path spellings that skipped ownership checks** ([#737](https://github.com/TykTechnologies/ai-studio/pull/737)). A different spelling of a native path (`/v1/Responses`, `/V1/responses`) reached the vendor on the ordinary chain without its checks. It now gets the vendor's `404`.
- **Duplicate JSON keys** ([#730](https://github.com/TykTechnologies/ai-studio/pull/730), [#737](https://github.com/TykTechnologies/ai-studio/pull/737)). A body with a key given twice, or twice in different case where the gateway reads it case-insensitively (`messages` and `Messages`), is refused on batches, `count_tokens`, batch file lines and synchronous Anthropic `/v1/messages`, so the filters and the vendor cannot read different prompts. Body keys that differ only in case from a checked field (`Previous_Response_Id`) are refused too.
- **Decision-metric labels.** `aistudio_decision_questions_total` counts only calls that a model answered or refused, so a failed call that names an invented model adds no series ([#724](https://github.com/TykTechnologies/ai-studio/pull/724)).

## Breaking changes and upgrade notes

- **Edge gateways answer `404` on every `/api/v1` path**, and the `mgw` CLI no longer works against an edge. Configure edges from AI Studio. A standalone microgateway keeps its authenticated management API, but `POST /api/v1/auth/token` is removed: create tokens with `POST /api/v1/tokens` (admin token) or `microgateway -create-admin-token`.
- **Edges must run 2.3.0 to serve decision models.** Older edges do not know the `systemone` vendor and refuse that LLM. OpenAI decision calls pass through older edges but are not recorded there.
- **Price decision models.** Decision models are billed by input tokens, and output is usually free. Add a model price with the provider's input price and an output price of `0`. You can price the alias (`jev-latest`) or the version it resolves to (`jev-1.13.0`). An unpriced call is recorded at zero cost.
- **Edge response plugins now receive the LLM's `vendor`** in their context. It used to be sent empty.
- **`CallLLM` returns `FailedPrecondition`** when the agent's App has no credential. It used to fail with a `401` from the gateway.
- **Price auto-creation.** When the model a provider reports has no price and differs from the requested model, AI Studio's price lookup may also create a zero-priced row for the requested model name.
- **Admin console pickers.** Chat rooms, agents, Model Routers, Semantic Routers and the wizards no longer offer decision-only LLMs. The failover picker only offers fallbacks that share a request kind with the primary.
- **Schema version 8** (7 for [#721](https://github.com/TykTechnologies/ai-studio/pull/721), 8 for [#724](https://github.com/TykTechnologies/ai-studio/pull/724)).
  - New tables: `tyk_gateway_nodes` and `tyk_gateway_definitions`.
  - New columns:
    - `tyk_connections.kind` (defaulted) and its `gateway_*` columns;
    - `mcp_servers.key_access`, `gateway_coverage` and `gateway_partial`;
    - `proxy_logs.request_kind` and `llm_chat_records.request_kind` (defaulted to `''`, read as generative, with an index on `request_kind` and `time_stamp`);
    - `llm_chat_records.decision_questions` and `decision_refusals`.
  - The edge's `analytics_events` gains `request_kind`, `decision_questions` and `decision_refusals`.

  Every change is additive, and `MinReaderSchemaVersion` stays 1, so a 2.2.1 binary still runs on a 2.3.0 database. After a rollback, System One LLMs stop working (their vendor is unknown), and Gateway connections need 2.3.0.
- **Schema versions 9 and 10** (9 for [#730](https://github.com/TykTechnologies/ai-studio/pull/730), 10 for [#737](https://github.com/TykTechnologies/ai-studio/pull/737)).
  - New table: `native_resources` (the native API ledger; each edge keeps its own).
  - New columns: `llms.vendor_workspace_id`, `price_multipliers` and the `resource_signing_*` columns; `llm_chat_records.service_tier` and `proxy_logs.service_tier`. The edge's `llms` and `analytics_events` gain the matching columns.

  Every change is additive and `MinReaderSchemaVersion` stays 1. On first start, AI Studio gives every LLM a signing secret and keeps the key derived from its API key as the previous key for 30 days, so IDs handed out before the upgrade keep working.
- **Upgrade edges with the hub.** An older edge ignores the signing secret and keeps signing with the API key. The hub accepts its IDs for the 30-day backfill grace, but IDs the hub mints do not verify on an old edge, except for resources in that edge's own ledger.
- **OpenAI response IDs handed out before 2.3.0 no longer verify** as `previous_response_id` or on retrieve: the first call gets `404`, and the client starts a new chain.
- **OpenAI `/v1/responses`, `input_tokens` and `compact` must name `model`.** A call that relies on a stored prompt's model or names only a conversation gets `400` ("model field is required"), because the gateway checks the model against the allow-list. Batch lines must name `body.model`.
- **`/v1/files` accepts only `purpose=batch`**, in the plain multipart form the SDKs send. Uploads are limited to 64 MiB and, on an LLM with request filters, to 10,000 lines (`NATIVE_API_MAX_BODY_BYTES`, `NATIVE_API_MAX_FILTERED_ITEMS`; OpenAI accepts 200 MB and 50,000 lines).
- **Screening fails closed when request filters are attached.** Unknown Responses input item types, Responses input the gateway cannot read and token-array inputs on embeddings, completions and moderations get `400`. Without filters, these calls are unchanged.
- **Non-canonical native paths.** A path that differs from a declared native path only in case or empty segments gets the vendor's `404` instead of reaching the vendor.
- **Headers not forwarded.** `OpenAI-Organization` and `OpenAI-Project` are dropped on every OpenAI native call and every Responses call, and `anthropic-user-profile-id` and a client's `anthropic-workspace-id` on every Anthropic call. Resources live in the API key's project or workspace, or in the LLM's pinned workspace.
- **Plugin upstream overrides** no longer apply to `count_tokens` or the OpenAI native operations, which always go to the LLM's own endpoint.
- **Chat-session costs change** ([#736](https://github.com/TykTechnologies/ai-studio/pull/736)). Anthropic turns with tool calls or thinking drop to the correct single amount, OpenAI and Google cached tokens are billed at the cache price when one is set, and Anthropic chat rows' total tokens now include cache tokens, as gateway rows do.
- **Protocol.** The plugin, management and config-sync protos change additively: new fields and the new `CallDecision` RPC. [#737](https://github.com/TykTechnologies/ai-studio/pull/737) adds `LLMConfig` fields 27 to 31 (signing keys, workspace, price multipliers), `AnalyticsEvent.service_tier` and the plugin `AnalyticsRequest.service_tier`.
- **Go embedders and plugin authors.** `models.LLMVendorProvider` gains `ProvidesChat()` and `DecisionEndpoint()`, so a custom vendor implementation must add them. The plugin `StudioServices` interface is unchanged: `CallDecision` is a package-level SDK function, so custom fakes keep compiling.

### New configuration

No new runtime setting is required. Gateway connections are configured per connection in the console, not through the environment.

Native API operations ([#730](https://github.com/TykTechnologies/ai-studio/pull/730)); AI Studio and the Microgateway read the same names, and unset or `0` keeps the default (see `docs/site/docs/configuration.md`):

| Variable | Default | Purpose |
|---|---|---|
| `NATIVE_API_MAX_BODY_BYTES` | `67108864` (64 MiB, at most 256 MiB) | Body limit of a batch create or batch file upload |
| `NATIVE_API_MAX_CONCURRENT_CREATES` | `4` | Creates admitted at once; others wait |
| `NATIVE_API_MAX_FILTERED_ITEMS` | `10000` | Requests per batch or file when the LLM has request filters |
| `NATIVE_API_MAX_IN_PROGRESS_PER_APP` | `0` (no cap) | In-progress batches per App and LLM |
| `NATIVE_API_RECONCILE_INTERVAL` | `30s` | How often the reconciler looks for due batches |
| `NATIVE_API_RECONCILE_BATCH` | `50` | Batches one reconciler pass takes |
| `NATIVE_API_RECONCILE_WORKERS` | `4` | Batches reconciled at once |
| `NATIVE_API_POLL_INTERVAL` | `2m` | Wait before polling an unfinished batch again |
| `NATIVE_API_POLL_MAX_BACKOFF` | `30m` | Longest wait after failed polls |

`PROXY_URL` (Microgateway: `GATEWAY_PUBLIC_URL`) is the base of a batch's `results_url`.

Per LLM ([#737](https://github.com/TykTechnologies/ai-studio/pull/737)), in the LLM form and the LLM API:

- `vendor_workspace_id`: the Anthropic workspace every call for the LLM runs in.
- `price_multipliers`: service tier to share of the list price, for example `{"batch": 0.4}`.
- `GET /api/v1/llms/{id}/resource-signing-key` shows the signing key status (never the secret), and `POST /api/v1/llms/{id}/resource-signing-key/rotate` rotates it with a grace period.
- `GET /api/v1/analytics/service-tier-usage` reports usage by service tier.

For the live decision conformance suite only (`test-secrets/vendors.env`):

| Variable | Purpose |
|---|---|
| `VT_OPENAI_MODEL_DECISION` | OpenAI decision model to test, for example `gpt-6-luna`; uses the OpenAI key |
| `VT_SYSTEMONE_MODEL` | System One model to test, for example `jev-latest` |
| `VT_SYSTEMONE_ENDPOINT` | System One endpoint; default `https://api.typesafe.ai` |
| `VT_SYSTEMONE_API_KEY` | Key for the hosted System One API; optional for a self-host |

## Known issues

- **Model and Semantic Routers do not route decision calls yet.** They answer `400`.
- **Decision conformance has not been run against a llama.cpp self-host.** It is green against OpenAI `gpt-6-luna` and TypeSafe `jev-latest`.
- **Raw OpenAI file and item IDs are forwarded unchecked.** A `file_id` in an `input_file` or `input_image` part and an `item_reference` ID are passed to OpenAI without an ownership check. Only batch input files can be uploaded through the gateway, and item IDs are only returned to the App that owns them, so such an ID comes from outside the gateway or from a leak. Whether to verify or refuse them is still being decided.
- **`input_file` content is not screened** by the filters, as with files on Chat Completions.
- **Standalone microgateways sign with the API key.** Without a hub there is no signing secret, so rotating a standalone microgateway's vendor API key still invalidates outstanding resource IDs.
- **An ambiguous database commit can lose a batch's usage.** If the connection drops just after the gateway marks a batch as accounted, the batch stays accounted with no usage recorded: an undercount, never a double charge. This is rare.
- **The proxy-log API does not return `service_tier` yet**; the tier report and chat records have it.
- **Dashboard-created MCP proxies do not set `stripAuthorizationData`**, so AI Studio-minted Tyk keys are forwarded to third-party upstreams. Gateway connections set it. Changing it for Dashboard connections would alter existing proxies, so it is left for a separate decision.

## Submodules

- **enterprise** `118ad19` → `e4ac63a`: Tyk OSS Gateway connections for MCP proxies and keys ([ai-studio-enterprise#70](https://github.com/TykTechnologies/ai-studio-enterprise/pull/70)), with pooled Gateway clients, parallel node work and the literal-credential warning; plus the base URL and custom-domain fixes for Dashboard connections.
