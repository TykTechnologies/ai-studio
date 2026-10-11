# Decision Models

Decision models ("System One" models) answer typed questions about evidence with calibrated probabilities instead of generating text. They are billed by input tokens (output is usually free) and their wire schemas differ from chat. This spec covers how the gateway, Studio and the edge govern them. User docs: `docs/site/docs/decision-models.md`.

## Wire formats

| | OpenAI Decisions | System One |
|---|---|---|
| Who | OpenAI `gpt-6-luna` (beta 2026-10-06) | TypeSafe Jev; llama.cpp `/v1/systemone` (Kev, OpenJev, Laya, Julia-1, ...) |
| Path | `POST /v1/decisions` | `POST /v1/systemone` |
| Evidence | `input`: string or user messages (`input_text`, inline-base64 `input_image`) | `state`: string, object or array (text only) |
| Questions | `questions[]` `{type, name, instructions, choices[] / levels[]}` | `questions{name: {type, instructions, criteria}}` |
| Types | `predicate`, `choice`, `score` | `noul`, `choice` (≤255 options), `score` (2-10 levels) |
| Answers | `answers[]` in question order; `refusal` answers | `answers{name}`; unanswered questions are absent |
| Usage | `usage` (`input_tokens` or `prompt_tokens` spelling accepted) | `usage {input_tokens, output_tokens}` |

`pkg/decisions` is the one parser and translator (a dependency-free leaf used by the proxy, vendors, plugin SDK and microgateway):

- Types for both formats. `SystemOneRequest` keeps question order on marshal/unmarshal (a Go map would sort it; Jev is option-order sensitive). Choice criteria are written as ordered objects.
- `Validate(req, target)`, `ToSystemOne`, `FromSystemOne(so, req)` (answers back in question order, missing or mistyped answers become refusals), `ToOpenAIRequest`, `ParseRequest`, `ParseResponse` (detects the format from the `answers` shape), `UsageFromBody`, `Summarize`, `RenderAnswers`.
- `FormatForPath` / `IsDecisionPath` / `KindForPath` classify an upstream path by its trailing endpoint.

## Model class and request kind

There is no new resource: decision models are `LLM` records, because Apps call them through the proxy and they need everything an LLM carries (credentials, App ACLs, AllowedModels, privacy, filters, plugins, budgets, failover, edge sync). The model class belongs to the **request**, determined by the upstream endpoint, plus a **vendor capability**:

- `models.LLMVendorProvider` gains `ProvidesChat()` and `DecisionEndpoint()` ("" = no decision models). OpenAI: chat + `/v1/decisions`. New vendor `systemone` (`vendors/systemone`): decisions only (`/v1/systemone`); `GetDriver`/`GetEmbedder` return `ErrDecisionsOnly`; not in `AVAILABLE_LLM_DRIVERS` (the chat-settings driver list).
- `switches`: `DecisionVendors`, `SupportsDecisions`, `DecisionEndpoint`, `SupportsChat`, `VendorCapabilities`, and `UpstreamEndpoint(llm)`, which falls back to a vendor default when the LLM has no endpoint (`systemone` → `https://api.typesafe.ai`). Both proxy upstream sites use it.
- `models.RequestKind`: `""` (generative) or `"decision"`. Orthogonal to `InteractionType` (chat vs proxy origin).

## Gateway paths

- **Native pass-through** (`/llm/rest|call/{slug}/v1/decisions|systemone`): the OpenAI vendor's `AnalyzeResponse` used to reject `/v1/decisions` ("unknown completions endpoint"), so decision traffic was proxied but never logged, priced or budgeted. Both vendors now analyse it into `responses.DecisionResponse` (`models.IDecisionResponse`). A JSON body without answers (errors, budget denials, policy blocks) analyses to zero answers so the exchange is still logged. `ProxyScreenRequest` rejects streaming. Stream detection never streams `systemone`.
- **Bridge** `/ai/{slug}/v1/decisions` (`proxy/decisions_bridge.go`), and the unified ingress `{base}/decisions`, which `NewUnifiedRouterHandler` rewrites to it before auth: the request is validated against the primary's format up front (a request the primary cannot take is a 400, not a failover). Then it walks `planFailover`, filtered by `decisionPlan` to rungs whose vendor serves decisions. Each rung's body is shaped for its vendor (OpenAI: the caller's fields as sent, model replaced; System One: `ToSystemOne`) and sent over the loopback `/llm/call/{slug}{DecisionEndpoint}` with `newInternalRoutingClient`, so auth, filters, plugins, budget and analytics run once, on the native hop. System One answers are translated back with `FromSystemOne`. Errors use the OpenAI envelope: `422` → `400 invalid_request_error`, `529` → `503`, loopback refusals relayed via `parseLoopbackError`. Routers (`lookupRouter`) get a 400; routing decisions is a follow-up.
- The chat bridge answers a `systemone` route with a 400 ("serves decision models only") instead of a driver error.
- Waterfalls are filtered per request kind before the loop (`planKeeping`/`planKeepingAttempts`): a decision request drops rungs whose vendor serves no decision models and rungs whose wire format cannot carry the request (image input to a text-only System One model); a chat request (`chatPlan`, buffered and streaming) drops decision-only rungs. A dropped rung never replaces the primary's own failure with its "unsupported" error. The admin UI's failover picker only offers targets that share a request kind with the primary (`failoverCompatible`).
- `GET {base}/models` entries carry a non-standard `capabilities` array.
- The microgateway mounts the same proxy handler, so it gets every path above. A standalone edge creates `systemone` LLMs through `ManagementService.CreateLLM` (key required for the hosted API, optional with a self-host endpoint). `tests/integration/decision_traffic_test.go` boots the real edge with a mock decision upstream and runs in CI on every change.

## Filters and guardrails

`proxy/decisions_filters.go` registers extractor/reconstructor formats `openai_decisions` and `systemone`. The native hop picks them by path (`decisionFilterFormat`) whatever the vendor. Messages: one system message per question (instructions, in order), then the evidence as the final user message, so `last_user` guardrail scopes scan the evidence. Reconstruction writes edited text back (OpenAI message inputs keep image parts; untouched System One object state and instructions keep their JSON shape; option and question order survive). `ScriptInput.Context["request_kind"]` is set. Response filters see `RenderAnswers` text (`switches.ExtractResponseText` detects decision bodies by shape).

## Plugins and SDK

- `PluginContext.request_kind` (proto field 9) is set by the edge pre-auth, auth and post-auth hooks, the edge plugin middleware, and the response-hook context (`proxy.PluginContext` gains `Vendor` and `RequestKind`; the edge adapter now passes the vendor through, which used to be empty). The SDK exposes `Context.RequestKind` / `IsDecision()`.
- `AnalyticsRequest` (data collectors) gains `request_kind`, `decision_questions`, `decision_refusals`, and its context the vendor and kind.
- `pkg/plugin_sdk/decisions.go`: type aliases over `pkg/decisions`, `ParseDecisionRequest(path, body)`, `ParseDecisionResponse(body, req)`, `DecisionAnswerText`, and `CallDecision(ctx, llmID, req)`.
- `CallDecision` RPC (unary, `llms.proxy` scope, JSON in and out to stay independent of beta schemas) calls `/ai/{slug}/v1/decisions` with the agent App's credential. It shares `agentLLMForPlugin` (agent config, App must hold the LLM, credential present) with `CallLLM`. It is a package-level SDK function, not a `StudioServices` method, so third-party fakes of that interface keep compiling.

## Analytics, budgets, metrics

- `LLMChatRecord` gains `RequestKind` (defaulted `''`, indexed), `DecisionQuestions` and `DecisionRefusals`; answered questions are `Choices`. `ProxyLog` gains `RequestKind`. Schema version 8. Cost uses the existing formula; output-free pricing is `CPT = 0`. No price rows are seeded.
- Edge chain: mgw `AnalyticsEvent` has the same three columns (AutoMigrate); `eventFromProxyLog`/`applyChatRecord`, `interfaces.AnalyticsData`, the pulse buffer and `buildPulseMessage` carry them; `pb.AnalyticsEvent` fields 42-44; the hub's `SendAnalyticsPulse` copies them and, for decisions, takes `Choices` from the event instead of its default of 1.
- Pricing: `proxy.priceForExchange` prices by the model the vendor reported and, when that model has no price, by the model the caller asked for. Jev answers the alias `jev-latest` as `jev-1.13.0`; without the fallback a call priced under the alias was recorded at zero cost (found by the live conformance suite).
- Budgets need no new code: decision requests pass `CheckBudget` on the native hop and their cost flows into App, LLM and team spend.
- Upgrade: the new columns default to `''`, so rows written before the upgrade read as generative, never NULL (tested on SQLite and Postgres; the analytics package is in CI's Postgres job).
- `GET /api/v1/analytics/decisions` (`analytics.GetDecisionUsage`): totals, per-day series, per-model with cost per thousand calls; optional `llm_id`, `app_id`. A dedicated endpoint rather than a filter on every stats query, whose signatures are shared with the plugin service layer. Dashboard panel `DecisionUsagePanel`.
- Metric `aistudio_decision_questions_total{vendor, model, outcome}`.

## UI

`vendorLogos.js` knows `systemone` (capabilities, default endpoint, no key required) with `isDecisionOnlyVendor`, `vendorServesDecisions`, `getChatVendorCodes`, `chatCapableLLMs`. Chat rooms, the chat-room and quick-start wizards, Model Router and Semantic Router forms leave decision-only LLMs and vendors out. The LLM form explains decision vendors.

## Compatibility and follow-ups

- Edges older than this release do not know `systemone` (that LLM fails there); OpenAI decision calls pass through them unrecorded.
- Live-verified 2026-10-11 by the decision conformance suite (`features/VendorConformance.md` §10, `make test-vendors-decisions`): `gpt-6-luna` answers `{model, answers, usage}` with no `id`/`object`; `usage` is `input_tokens`/`output_tokens`/`total_tokens` plus `input_tokens_details {cached_tokens, cache_write_tokens}` (read into the chat record's cache counters) and `output_tokens_details {reasoning_tokens}`. System One has not been run live yet.
- Follow-ups: Model/Semantic Router support for decisions; semantic cache keys for decisions; Studio consumers (a `decision_model` guardrail provider, decision-based routing, a chat/agent tool); example plugin; per-request pricing if a vendor introduces it.
