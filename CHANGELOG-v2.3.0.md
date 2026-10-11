# Tyk AI Studio 2.3.0

**Status:** Unreleased. This file collects what has merged to `main` since 2.2.1 and is updated as more merges land.
**Diff so far:** [`v2.2.1...main`](https://github.com/TykTechnologies/ai-studio/compare/v2.2.1...main)

2.3.0 so far adds two capabilities and one edge fix.

- **Decision models.** AI Studio now governs a new class of model that answers typed questions about a piece of evidence, with calibrated probabilities, instead of generating text. OpenAI's Decisions API (`gpt-6-luna`) and System One models (TypeSafe's Jev and wire-compatible self-hosts) are supported. Clients use one contract, the **OpenAI Decisions schema on `/v1/decisions`**, whatever the provider. Auth, filters, plugins, budgets and analytics apply to decision calls as they do to chat.
- **Tyk OSS Gateway connections (Enterprise).** AI Studio can manage MCP proxies and keys directly on open-source Tyk Gateways, without a Tyk Dashboard.
- **Edge stability.** Two data races in the edge's control-plane client are fixed. One of them left an extra heartbeat worker running after every reconnect.

Read **Breaking changes and upgrade notes** before upgrading. Edges must run 2.3.0 to serve decision models.

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

## Bug fixes

### Gateway
- **Native OpenAI decision calls were not recorded** ([#724](https://github.com/TykTechnologies/ai-studio/pull/724)). `/llm/rest/{openai-llm}/v1/decisions` already passed through, but response analysis rejected the path, so those calls wrote no proxy log, no cost and no budget spend.
- **Calls to a model alias were recorded at zero cost** ([#724](https://github.com/TykTechnologies/ai-studio/pull/724)). When the model a provider reports has no price, the call is now priced by the model that was requested. Jev answers `jev-latest` as `jev-1.13.0`, and OpenAI reports dated snapshots of chat models.

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
- **Gateway connections are MCP only** ([#721](https://github.com/TykTechnologies/ai-studio/pull/721)). Every call to a Tyk Gateway goes through a fixed allow-list, and AI Studio never changes or deletes proxies it did not create.
- **Proxies that AI Studio creates on Gateways set `stripAuthorizationData`**, so a client's Tyk key is not forwarded to the upstream MCP server. Dashboard connections do not; see Known issues.
- **The internal-address rule applies to every Gateway node.** With DNS discovery, AI Studio reaches each node by its resolved address. Whether an internal address may be dialled is still decided for the connection's hostname, as for any other outbound call.
- **Decision-metric labels.** `aistudio_decision_questions_total` counts only calls that a model answered or refused, so a failed call that names an invented model adds no series ([#724](https://github.com/TykTechnologies/ai-studio/pull/724)).

## Breaking changes and upgrade notes

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
- **Protocol.** The plugin, management and config-sync protos change additively: new fields and the new `CallDecision` RPC.
- **Go embedders and plugin authors.** `models.LLMVendorProvider` gains `ProvidesChat()` and `DecisionEndpoint()`, so a custom vendor implementation must add them. The plugin `StudioServices` interface is unchanged: `CallDecision` is a package-level SDK function, so custom fakes keep compiling.

### New configuration

No new runtime setting is required. Gateway connections are configured per connection in the console, not through the environment.

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
- **Dashboard-created MCP proxies do not set `stripAuthorizationData`**, so AI Studio-minted Tyk keys are forwarded to third-party upstreams. Gateway connections set it. Changing it for Dashboard connections would alter existing proxies, so it is left for a separate decision.

## Submodules

- **enterprise** `118ad19` → `e4ac63a`: Tyk OSS Gateway connections for MCP proxies and keys ([ai-studio-enterprise#70](https://github.com/TykTechnologies/ai-studio-enterprise/pull/70)), with pooled Gateway clients, parallel node work and the literal-credential warning; plus the base URL and custom-domain fixes for Dashboard connections.
