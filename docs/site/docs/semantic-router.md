# Semantic Router (Enterprise)

A **Semantic Router** chooses the model from what the prompt says. You describe a few **routes**, for example "complex reasoning goes to Claude Opus, everything else goes to Haiku". Clients then send one model name, and the gateway classifies each request and serves it from the matching route.

```
POST /v1/chat/completions   {"model": "smart/auto", "messages": [...]}
```

The Semantic Router stands on its own. A route targets an LLM and model directly, so `complex -> Opus` needs no pools. When you do want balancing or vendor mixing, a route can instead hand a model alias to a [Model Router](./model-router.md). The Semantic Router picks the tier and the Model Router picks the vendor. Neither needs the other.

Semantic Routers run on the **microgateway** (edge). AI Studio hosts their configuration, the portal catalogue and a test panel. The embedded gateway in AI Studio does not route them.

## How a request is classified

The router reads the request's text: by default the last user message, or every user message if **Input scope** is set to "all user messages". It keeps the end of long texts, up to 4,000 characters by default. The stages then run in order, and the first stage that decides wins:

| Stage | Decides when | Cost |
|---|---|---|
| **Explicit route** | The client sent `smart/complex` instead of `smart/auto` and explicit routes are allowed. | none |
| **Session affinity** | Affinity is on and this App and session (the `X-Tyk-Session-Id` header by default) was routed recently. | none |
| **Keywords** | A route's keyword appears in the text. Literals are case-insensitive substrings; regular expressions are also supported. Higher-priority routes are checked first. | none |
| **Embeddings** | The text is most similar to one of a route's example utterances, and that similarity reaches the route's threshold (default 0.75). If two routes score the same, the higher priority wins. | one embedding call |
| **LLM judge** | Optional. An LLM reads the route names and descriptions and names one. By default it runs only when the embedding stage found nothing ("low confidence"); set it to "always" to let it decide instead of the embeddings. | one small completion |
| **Default route** | Nothing else decided. | none |

- **The router always answers.** If the embedding model or the judge fails or times out (1.5 s and 3 s by default), the request goes to the default route and the decision is recorded with the reason `classifier_error`. A classifier outage slows nothing down beyond its timeout and never fails a request.
- **Examples are embedded on the gateway, in the background,** when the router loads and again when its configuration changes. Until they are ready, the embedding stage is skipped: keywords, the judge and the default route still work. Vectors never leave the gateway.
- **The judge can only name one of your routes.** Any other answer is ignored, so a prompt that tries to steer the judge can at worst pick another of the router's own routes. The App was granted all of them.

### Shadow mode

In **shadow** mode the router classifies every request and records the route it would have chosen, but always serves the default route. Use it to measure a new router against real traffic before it changes anything: the analytics show the `shadow_route` next to the route actually served.

## Creating a Semantic Router

In the admin UI, open **Semantic Routers** and choose **Create**.

1. **Name and slug.** The slug is the first half of the model string (`smart` in `smart/auto`). It must not be the name of an LLM or the slug of a Model Router.
2. **Routes.** For each route, set:
   - a **name**: lowercase, and not `auto`;
   - a **description**, which the judge reads and the portal shows;
   - a **priority**;
   - a **target**: an LLM and model, or a Model Router and the alias to send it;
   - optionally, **keywords** and **example utterances**. A handful of varied, realistic examples per route works better than many similar ones.
3. **Default route.** This is required. It serves everything no stage claims.
4. **Embedding model.** This is required when any route has examples. Choose an LLM whose vendor provides embeddings (OpenAI, Ollama, Google AI, Vertex, Hugging Face) and its embedding model, for example `text-embedding-3-small`.
5. **Optional settings:** the LLM judge, session affinity, shadow mode, and **Allow explicit routes**. The last one lets clients send `smart/<route>` to skip classification.
6. **Test** the router with the **Test prompt** panel before you publish it. The panel runs the same engine the gateway runs against your draft. It shows the chosen route, the reason, each route's similarity score and every stage's latency.
7. **Publish.** Set the router active, add it to one or more LLM catalogues, and grant it to Apps.

## Access and the portal

A Semantic Router is granted and published like an LLM:

- It sits in **LLM catalogues**. Teams that hold one of those catalogues see it in the portal catalogue as "Semantic Router". The portal shows its model strings and route descriptions; keywords and examples are never shown.
- An **App** is granted the router, in the portal App builder or the admin App editor. The grant lets the App reach every LLM the router's routes can send to, but **only through the router**. It is not a grant of those LLMs. A route that hands off to a Model Router needs no separate grant of that Model Router.
- An App that is not granted the router gets `403`. Unlike Model Routers, there is no grant-less fallback.
- **Privacy.** A router's privacy score is the lowest score among the LLMs it may send a request's text to. That includes the embedding and judge LLMs, because they see the prompt too, and all vendors of a Model Router it hands off to. An App's data sources and tools must fit within it, as they would for an LLM.
- `GET /v1/models` lists `smart/auto`, plus `smart/<route>` for each route when explicit routes are allowed, for Apps that hold the router.

## Observability

Every routed response carries:

| Header | Value |
|---|---|
| `X-Tyk-Router` | the router's slug |
| `X-Tyk-Route` | the route that served the request |
| `X-Tyk-Route-Reason` | `explicit`, `affinity`, `keyword`, `embedding`, `judge`, `default` or `classifier_error` |
| `X-Tyk-Served-LLM`, `X-Tyk-Served-Model` | the LLM and model that answered |

Proxy logs record the same decision: router kind and slug, route, reason, the embedding similarity that decided (if any), the shadow route, and the model asked for and forwarded. Edges send these to AI Studio with their analytics.

## Costs and limits

- An embedding call per request (on requests keywords did not decide), and a judge call when the judge runs. These are the router's own calls. They are **not** charged to the App's budget and do not pass through the App's filters.
- Session affinity is kept in memory on each gateway node, and is not shared between nodes.
- Only the OpenAI-compatible unified endpoint (`/v1/chat/completions`, `/v1/completions`) routes Semantic Routers.
