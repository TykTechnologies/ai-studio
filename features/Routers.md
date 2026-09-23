# Routers (Enterprise)

A **router** is a gateway route that is not an LLM: the caller names the router, and the router picks the LLM and model that serve the request. Today the only kind is the **Model Router** (pools matched on the requested model name, vendors picked by round robin or weight, per-vendor model mappings). A **Semantic Router** (routes chosen from what the prompt means) is planned on the same foundation; see [Roadmap](#roadmap-semantic-routing).

User docs: `docs/site/docs/model-router.md`.

## Addressing

Routers share the model namespace of the unified OpenAI-compatible ingress with LLMs:

```
POST /v1/chat/completions  {"model": "openai/gpt-4o"}   -> LLM "openai"
                           {"model": "prod/gpt-4o"}     -> Model Router "prod"
```

- The ingress (`proxy/unified_router.go`) rewrites `{route}/{model}` to `/ai/{route}/v1/...` **without resolving the route**, so authentication always runs first.
- The `/ai/{routeId}` bridge (`proxy/translator.go`) resolves a route that is not an LLM through a `proxy.RouteResolver` (`proxy/router.go`). The host installs the resolver. The **microgateway** does so (`microgateway/internal/services/model_router_resolver.go`); AI Studio's embedded gateway does not, so there a router slug is an unknown vendor. Routing is a data-plane feature of the edge.
- The legacy `/router/{slug}/v1/...` endpoints on the microgateway are an alias that rewrites to `/ai/{slug}/v1/...`.
- A router slug and an LLM's slug (`slug.Make(name)`) must not clash (`models/route_slugs.go`). Both LLM saves and router saves check this, across namespaces.

## Access model

- **Grant.** `App.ModelRouters` (`app_model_routers`), synced to the edge in `AppConfig.model_router_ids` and stored in the edge's `app_model_routers` table.
- **Outer hop** (`resolveRoute`): the App must hold the router. Deprecated fallback: a Model Router called by an App without a grant is served only from the LLMs the App holds directly (`RouteRequest.Allow`).
- **Inner hop** (`/llm/call/{llm}`): access to the chosen LLM is inherited through the router (`routerGrantsAccess`). This requires a loopback marker (`X-Tyk-Router-*`) carrying the per-process token, an App that holds the router, and the router being able to reach that LLM (`RouteResolver.Reaches`). A failover rung whose origin was reached through the router is accepted too (`failoverGrantsAccess` uses `appAllowedLLM`). Markers are stripped before egress.
- **Privacy.** A router is a provider scored by the **minimum** privacy score of the active LLMs behind its active vendors (`models.ModelRouterPrivacySQL`). App validation counts it like an LLM (`services.WithModelRouters` → `routerProvidersFor`).

## Portal

- Routers are published in **LLM catalogues** (`catalogue_model_routers`; `PUT /api/v1/model-routers/{id}/catalogues`). Visibility follows `models.AccessibleModelRouterQuery`: user → teams → LLM catalogues → active routers.
- The portal catalog has a `model_router` item type in the LLM catalogue family, with `router_models` (the `{slug}/{model}` strings) and, on detail, `router_llms`.
- The App builder and admin App editor grant routers via `model_router_ids`. The portal validates visibility; admins may grant any active router.
- Deleting a router withdraws its App grants and catalogue memberships. Dependents lists both.

## Observability

- Response headers: `X-Tyk-Router`, `X-Tyk-Route` (pool or route), `X-Tyk-Route-Reason` (`model_pattern`), next to `X-Tyk-Served-LLM` and `X-Tyk-Served-Model`. They are CORS-exposed.
- `ProxyLog` gains `RouterKind`, `RouterSlug`, `RouterPool`, `Route` and `RouteReason`. They reach the hub from the edge through the analytics pulse (`AnalyticsEvent` fields 30-34). This replaced a per-second in-memory lookup, which mixed up concurrent requests and never reached the hub.

## Roadmap: semantic routing

Research summary (Sept 2026):

- **Kong** is the only incumbent API gateway with native semantic routing (`ai-proxy-advanced`, `balancer.algorithm: semantic`). It embeds one description per target, matches against a threshold, and sends everything else to a CATCHALL target.
- **Apigee** and **Gravitee** have a semantic *cache* but route only on rules (headers or the model field).
- The families of approach are:
  - embedding similarity to route examples (Kong, LiteLLM, the vLLM Semantic Router embedding signal);
  - trained classifiers (the vLLM Semantic Router behind Envoy AI Gateway, agentgateway and the Gateway API Inference Extension; OpenRouter `auto`);
  - learned quality predictors (RouteLLM, Bedrock Intelligent Prompt Routing, Azure Foundry Model Router, NotDiamond);
  - LLM-as-judge (LiteLLM Auto Router v2).
- No one scopes routing to what the caller is authorised to use, offers a shadow or dry-run mode, or records why a route was chosen.

Planned design (phases 2 and 3):

- A standalone **Semantic Router** resource: named routes, each targeting either an (LLM, model) pair or a Model Router alias. It needs no pools of its own.
- Stages run in order: affinity pin, explicit route (`smart/<route>`), keywords, embedding similarity (max over example utterances, a threshold per route), an optional LLM judge, then a mandatory default route.
- It fails open to the default route, and has a shadow mode that computes and records the decision but serves the default.
- A test-prompt endpoint on the hub shows the per-stage trace.
- It reuses everything above: `RouteResolver` (a new `RouterKind`), grants, catalogue publishing, loopback markers and analytics fields.
