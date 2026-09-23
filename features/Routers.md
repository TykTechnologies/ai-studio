# Routers (Enterprise)

A **router** is a gateway route that is not an LLM: the caller names the router, and the router picks the LLM and model that serve the request. There are two kinds:

- The **Model Router** maps the requested model name to a pool of vendors. It picks a vendor by round robin or weight and applies per-vendor model mappings.
- The **Semantic Router** picks a named route from what the prompt means, using keywords, embeddings, an optional LLM judge, and a default route. Each route targets an (LLM, model) pair or hands a model alias to a Model Router. See [Semantic Router](#semantic-router).

User docs: `docs/site/docs/model-router.md`, `docs/site/docs/semantic-router.md`.

## Addressing

Routers share the model namespace of the unified OpenAI-compatible ingress with LLMs:

```
POST /v1/chat/completions  {"model": "openai/gpt-4o"}   -> LLM "openai"
                           {"model": "prod/gpt-4o"}     -> Model Router "prod"
                           {"model": "smart/auto"}      -> Semantic Router "smart"
```

- The ingress (`proxy/unified_router.go`) rewrites `{route}/{model}` to `/ai/{route}/v1/...` **without resolving the route**, so authentication always runs first.
- The `/ai/{routeId}` bridge (`proxy/translator.go`) resolves a route that is not an LLM through a `proxy.RouteResolver` (`proxy/router.go`). The host installs the resolver. The **microgateway** does so (`microgateway/internal/services/router_resolver.go`); AI Studio's embedded gateway does not, so there a router slug is an unknown vendor. Routing is a data-plane feature of the edge.
- The legacy `/router/{slug}/v1/...` endpoints on the microgateway are an alias that rewrites to `/ai/{slug}/v1/...`.
- LLM slugs (`slug.Make(name)`), Model Router slugs and Semantic Router slugs must not clash with one another (`models/route_slugs.go`). Every save of any of the three checks this, across namespaces.
- The microgateway resolves both kinds with `services.RouterResolver`.

## Access model

- **Grant.** `App.ModelRouters` (`app_model_routers`) and `App.SemanticRouters` (`app_semantic_routers`). They are synced to the edge in `AppConfig.model_router_ids` / `semantic_router_ids` (23/24) and stored in the edge join tables of the same names.
- **Outer hop** (`resolveRoute`): the App must hold the router. Deprecated fallback, for Model Routers only: a Model Router called by an App without a grant is served only from the LLMs the App holds directly (`RouteRequest.Allow`). A Semantic Router always needs the grant.
- **Inner hop** (`/llm/call/{llm}`): access to the chosen LLM is inherited through the router (`routerGrantsAccess`). This requires a loopback marker (`X-Tyk-Router-*`) carrying the per-process token, an App that holds the router, and the router being able to reach that LLM (`RouteResolver.Reaches`). A failover rung whose origin was reached through the router is accepted too (`failoverGrantsAccess` uses `appAllowedLLM`). Markers are stripped before egress.
- **Privacy.** A router is a provider scored by the **minimum** privacy score of the LLMs a request's text may reach.
  - For a Model Router, these are the active LLMs behind its active vendors (`models.ModelRouterPrivacySQL`).
  - For a Semantic Router, these are its LLM targets, every active vendor of each Model Router it hands off to, and its embedding and judge LLMs (`models.SemanticRouterPrivacySQL`, over the `semantic_router_targets` rows rebuilt on every save).
  - App validation counts routers like LLMs (`services.WithModelRouters` / `WithSemanticRouters` → `routerProvidersFor`).
- **Reach** (what the inner hop accepts) for a Semantic Router covers its LLM targets, plus the vendors of the pool that each hand-off alias matches (`ModelRouterService.ReachesModel`). The judge and embedding LLMs are not reachable through it.

## Portal

- Routers are published in **LLM catalogues**.
  - Model Routers use `catalogue_model_routers` and `PUT /api/v1/model-routers/{id}/catalogues`; Semantic Routers use `catalogue_semantic_routers` and `PUT /api/v1/semantic-routers/{id}/catalogues`.
  - Memberships can also be edited from the catalogue: `GET`/`PUT /api/v1/catalogues/{id}/routers` (`model_router_ids`, `semantic_router_ids`; a list left out is unchanged). The LLM catalogue form and details page show both kinds.
  - Visibility follows `models.Accessible{Model,Semantic}RouterQuery`: user → teams → LLM catalogues → active routers.
- The portal catalog has `model_router` and `semantic_router` item types in the LLM catalogue family.
  - Both carry `router_models`, the `{slug}/{model}` strings.
  - Semantic Routers also carry `router_routes`: name, description and whether the route is the default. Keywords and examples are never shown.
  - Detail adds `router_llms`.
- The App builder and admin App editor grant routers via `model_router_ids` / `semantic_router_ids`. The portal validates visibility; admins may grant any active router.
- Deleting a router withdraws its App grants and catalogue memberships. Dependents lists both.

## Observability

- Response headers: `X-Tyk-Router`, `X-Tyk-Route` (pool or route), and `X-Tyk-Route-Reason` (`model_pattern`, or a Semantic Router stage), next to `X-Tyk-Served-LLM` and `X-Tyk-Served-Model`. They are CORS-exposed.
- `ProxyLog` gains:
  - `RouterKind`, `RouterSlug`, `RouterPool`, `Route`, `RouteReason`;
  - `RouteSourceModel`, `RouteTargetModel`, `RouteSelection`;
  - for Semantic Routers, `RouteScore` and `ShadowRoute`.
- These fields reach the hub from the edge through the analytics pulse (`AnalyticsEvent` fields 30-39). This replaced a per-second in-memory lookup, which mixed up concurrent requests and never reached the hub.

## Semantic Router

- **Contract:** `pkg/semanticrouting` holds the configuration (`Config`, `Settings`, `Route`, `Target`), `Validate`, `ModelsFor`, the `Engine`/`Router` interfaces and the engine registry. `pkg/semanticrouting/llmclient` reaches LLMs for embeddings and the judge through the vendor drivers.
- **Engine (Enterprise):** `enterprise/features/semantic_router/engine`, registered from `init()`.
  - Stages run in order: explicit, affinity, keywords, embeddings (max over examples, a threshold per route, priority breaks ties), judge (`low_confidence` or `always`), default.
  - Any classifier error falls back to the default route with reason `classifier_error`.
  - Shadow mode serves the default and records `ShadowRoute`.
  - Examples are embedded in the background with retry backoff; classification never waits for them.
- **Hub:** `models.SemanticRouter` stores the settings and routes as JSON. It is hard-deleted, clearing its targets, grants and catalogue memberships.
  - The service is `services/semantic_router` (a CE stub returning 402) plus `enterprise/features/semantic_router` (validation of every reference; `Test`).
  - The API lives in `api/semantic_router_handlers.go`, under the `semantic-routers` RBAC resource. It includes `POST /semantic-routers/{id}/test` and `POST /semantic-routers/test`, which test a draft.
- **Sync:** `ConfigurationSnapshot.semantic_routers` (field 15, `SemanticRouterConfig.config_json`) goes to the edge table `semantic_routers`.
  - `SemanticRouterService.LoadRouters` compiles the routers after each sync.
  - A router whose configuration is unchanged keeps its compiled form, so its example vectors and affinity survive syncs.
  - The engine reaches LLMs through `GatewayServiceAdapter.GetLLMByID`. The microgateway registers the engine in `cmd/microgateway/main_enterprise.go`.

## Research notes (Sept 2026)

- **Kong** is the only incumbent API gateway with native semantic routing (`ai-proxy-advanced`, `balancer.algorithm: semantic`). It embeds one description per target, matches against a threshold, and sends everything else to a CATCHALL target.
- **Apigee** and **Gravitee** have a semantic *cache* but route only on rules (headers or the model field).
- The families of approach are:
  - embedding similarity to route examples (Kong, LiteLLM, the vLLM Semantic Router embedding signal);
  - trained classifiers (the vLLM Semantic Router behind Envoy AI Gateway, agentgateway and the Gateway API Inference Extension; OpenRouter `auto`);
  - learned quality predictors (RouteLLM, Bedrock Intelligent Prompt Routing, Azure Foundry Model Router, NotDiamond);
  - LLM-as-judge (LiteLLM Auto Router v2).
- No one scopes routing to what the caller is authorised to use, offers a shadow or dry-run mode, or records why a route was chosen.
