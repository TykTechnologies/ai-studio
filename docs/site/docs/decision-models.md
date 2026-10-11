# Decision Models

**Decision models** (also called *System One* models) answer typed questions about a piece of evidence instead of generating text. You send the evidence (a customer message, a document, a transcript) and a list of questions; the model returns, for each question, a calibrated probability, a choice from your options, or a score on your scale. They are fast, cheap and easy to act on in code, which makes them a good fit for routing, triage, moderation and policy checks.

AI Studio governs decision models the same way it governs chat models: Apps reach them through the AI Gateway with their own credentials, and every call goes through access control, filters and guardrails, plugins, budgets and analytics, in Studio's embedded gateway and on edge gateways alike.

## Supported providers

| Provider | Configure as | Upstream API |
|---|---|---|
| OpenAI Decisions (`gpt-6-luna`, public beta) | An **OpenAI** LLM provider | `POST /v1/decisions` |
| TypeSafe Jev (`jev-latest`, `jev-1.13.0`) | A **System One (decision models)** LLM provider | `POST /v1/systemone` |
| Self-hosted System One models (llama.cpp serving Kev, OpenJev, Laya, Julia-1 and others) | A **System One (decision models)** LLM provider with your server's URL as the endpoint | `POST /v1/systemone` |

An OpenAI LLM provider serves both chat and decision models with one key: add the decision model (for example `gpt-6-luna`) to its allowed models. A System One provider serves decision models only. It cannot back chat rooms, agents, Model Routers or Semantic Routers, and AI Studio leaves it out of those pickers.

For a System One provider the endpoint defaults to TypeSafe's hosted API (`https://api.typesafe.ai`). For a self-hosted model, set the endpoint to your server (for example `http://llama:8080`); the API key is optional.

## Calling decision models

Decision models are exposed on the gateway's unified ingress in the **OpenAI Decisions** schema, whatever the provider. Point an OpenAI SDK at the gateway and address the model as `llm-slug/model`:

```bash
curl https://gateway.example.com/v1/decisions \
  -H "Authorization: Bearer $APP_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "jev/jev-latest",
    "input": "Customer message: I was charged twice for my order last week.",
    "questions": [
      {"type": "predicate", "name": "angry", "instructions": "Is the customer angry?"},
      {"type": "choice", "name": "team", "instructions": "Which team handles this?",
       "choices": [{"value": "billing", "description": "payments, charges, refunds"},
                   {"value": "shipping", "description": "deliveries"}]},
      {"type": "score", "name": "urgency", "instructions": "How urgent is it?",
       "levels": [{"label": "can wait"}, {"label": "this week"}, {"label": "today"}]}
    ]
  }'
```

```json
{
  "object": "decision",
  "model": "jev-1.13.0",
  "answers": [
    {"type": "predicate", "name": "angry", "probability": 0.81},
    {"type": "choice", "name": "team", "choice": "billing", "confidence": 0.93,
     "probabilities": [{"value": "billing", "probability": 0.95}, {"value": "shipping", "probability": 0.05}]},
    {"type": "score", "name": "urgency", "score": 1.4, "confidence": 0.7,
     "probabilities": [{"value": 0, "label": "can wait", "probability": 0.2},
                       {"value": 1, "label": "this week", "probability": 0.2},
                       {"value": 2, "label": "today", "probability": 0.6}]}
  ],
  "usage": {"input_tokens": 120, "output_tokens": 0}
}
```

The per-provider path `/ai/llm-slug/v1/decisions` takes the same request with a bare model name.

**Translation for System One providers.** The gateway converts the request to the System One format (`input` becomes `state`, the questions array becomes a map, `predicate` becomes `noul`) and converts the answers back, in your question order. A question the model leaves unanswered comes back as `{"type": "refusal"}`. System One upstreams take text only: a request with image input to a System One provider is rejected with a `400` naming `input`. Choice questions allow up to 255 options and score questions 2 to 10 levels.

**Errors** use the OpenAI error envelope. A System One validation failure (`422`) is reported as `400 invalid_request_error`, and "overloaded" (`529`) as a retryable `503`. Decision models do not stream; a request with `"stream": true` is rejected.

**Native pass-through.** Callers that already speak a provider's own format can keep using `/llm/rest/llm-slug/v1/decisions` (OpenAI) or `/llm/rest/llm-slug/v1/systemone` (System One). These calls are governed and recorded exactly like bridged ones.

**Model discovery.** `GET /v1/models` lists every route the App can use. Each entry carries a `capabilities` field (`chat`, `embeddings`, `decisions`) so clients can find decision-capable routes. Standard OpenAI clients ignore it.

**Failover.** An LLM's failover waterfall applies to decision calls. Rungs whose provider serves no decision models, or cannot take the request (image input to a text-only System One model), are skipped, and the request is re-shaped for each rung's provider, so a System One primary can fail over to an OpenAI fallback. Chat requests likewise skip decision-only fallbacks, and the LLM form only offers fallbacks that serve a request kind the primary serves. Model Routers and Semantic Routers do not route decision calls yet.

## Filters and guardrails

Request filters and guardrails see a decision request as messages: one **system** message per question (its instructions, in order) and a final **user** message holding the evidence. Guardrail scopes such as *last user message* therefore check the evidence, which is the untrusted part of the request. A filter that redacts text has its edits written back into the evidence and instructions, in the provider's own format; image parts are kept.

Filter scripts can tell decision traffic apart: `input.context.request_kind` is `"decision"` for decision requests and empty otherwise.

Response filters see the answers rendered as text, one per line (`team: choice=billing (confidence 0.93)`), and can block them like any response.

## Pricing and budgets

Decision models are billed by input tokens; output is usually free. Add a [model price](./model-prices.md) with the provider's input price and an output price of `0`, for example:

| Model | Vendor | Input price (per 1M tokens) | Output price |
|---|---|---|---|
| `gpt-6-luna` | openai | $0.10 | $0 |
| `jev-1.13.0` (and the aliases you use) | systemone | $0.042 | $0 |

You can price a model under the name you configure (`jev-latest`) or the version it resolves to (`jev-1.13.0`): calls are priced by the model the provider reports and, when that has no price, by the model you asked for.

Decision spend counts against App, LLM and team budgets like any other spend, and a spent budget refuses decision calls with the usual `403`.

## Analytics

Every decision call is recorded with its token usage and cost, plus how many questions it asked and how many the model refused. The **Decision Models** panel on the Dashboard shows calls, questions answered, refusal rate, cost and cost per thousand calls per model for the selected period. It appears once there is decision traffic. The same report is available from the API:

```
GET /api/v1/analytics/decisions?start_date=2026-10-01&end_date=2026-10-08[&llm_id=3][&app_id=7]
```

Decision spend is also included in the Dashboard's cost totals. Edge gateways report decision traffic to Studio in their analytics pulse.

The metric `aistudio_decision_questions_total{vendor, model, outcome}` counts questions by outcome (`answered`, `refused`). See [Observability](./observability.md).

## Plugins

Gateway and Studio plugins receive the request kind in their context. `ctx.RequestKind` is `"decision"` for decision calls and `ctx.IsDecision()` reports it. Data collectors receive `request_kind`, `decision_questions` and `decision_refusals` with each analytics record.

The plugin SDK reads either wire format as one OpenAI-shaped type:

```go
if ctx.IsDecision() {
    req, err := plugin_sdk.ParseDecisionRequest(r.Request.Path, r.Request.Body)
    // req.Questions, req.Input ...
}
```

Studio agent plugins can ask a decision model through the governed path with `plugin_sdk.CallDecision(ctx, llmID, req)`, which needs the `llms.proxy` scope and an agent App that holds the LLM. See [Plugin SDK](./plugins-sdk.md).

## Edge gateways

Edge gateways serve decision models once they run this release: the System One vendor is unknown to older edges, which then refuse that LLM. OpenAI decision calls pass through older edges but are not recorded there.
