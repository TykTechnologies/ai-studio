# Tyk AI Studio 2.3.0

**Status:** Unreleased. This file collects what has merged to `main` since 2.2.1 and is updated as more merges land.
**Diff so far:** [`v2.2.1...main`](https://github.com/TykTechnologies/ai-studio/compare/v2.2.1...main)

2.3.0 so far adds four capabilities, a chat billing fix, one edge fix, and the fixes from a bugsmash of the GitHub issue backlog.

- **Decision models.** AI Studio now governs a new class of model that answers typed questions about a piece of evidence, with calibrated probabilities, instead of generating text. OpenAI's Decisions API (`gpt-6-luna`) and System One models (TypeSafe's Jev and wire-compatible self-hosts) are supported. Clients use one contract, the **OpenAI Decisions schema on `/v1/decisions`**, whatever the provider. Auth, filters, plugins, budgets and analytics apply to decision calls as they do to chat.
- **Tyk OSS Gateway connections (Enterprise).** AI Studio can manage MCP proxies and keys directly on open-source Tyk Gateways, without a Tyk Dashboard.
- **Guardrails for tool results (Enterprise).** Request guardrails now screen what tools return (a fetched page, a file, an MCP or API response) for indirect prompt injection, in every vendor format, in the Responses API and native batches, and in Studio chat. A new `quarantine` action replaces a flagged tool result with a placeholder so the agent goes on without it. **Existing guardrails start scanning tool results on upgrade, and remote providers start receiving tool output.**
- **Native SDK features.** Anthropic Message Batches and `count_tokens`, and OpenAI's Responses, Conversations, Batches and Files APIs, now work through the native `/llm/` endpoints with the same governance as a chat call: filters, model allow-lists, budgets, analytics, and resources that belong to the App that created them.
- **Chat billing.** Chat sessions are priced like gateway calls: Anthropic turns with tool calls are no longer billed two or three times, and cached tokens are billed at the cache price.
- **Edge stability.** Two data races in the edge's control-plane client are fixed. One of them left an extra heartbeat worker running after every reconnect.
- **Bugsmash.** Around seventy fixes from the GitHub issue backlog, across AI Studio, the Microgateway and eight plugins:
  - Credential references (`$SECRET/`, `$ENV/`) follow one operator policy everywhere.
  - Plugins get an explicit environment and only the service-API scopes they declare.
  - Vendor clients no longer read keys or endpoints from the environment.
  - Tool and LLM names must have unique slugs, and edges apply what they can store instead of refusing a whole configuration.
  - AI Studio opens SQLite files in WAL mode.
  - Long chat sessions send their newest messages.

Read **Breaking changes and upgrade notes** before upgrading. Edges must run 2.3.0 to serve decision models and to scan tool results, and should be upgraded with the hub for native API resource IDs. Existing request guardrails scan tool results after the upgrade, and a guardrail on a remote provider (Lakera, Azure, Bedrock Guardrails, Presidio, an HTTP classifier) sends them to that provider: check each one before you upgrade.

The bugsmash adds upgrade notes of its own:
- list the `$ENV/` names your configuration uses in `AI_STUDIO_ENV_REF_ALLOWLIST`, or move those credentials to the secret store;
- set keys and endpoints on each LLM, embedder and datasource rather than through vendor environment variables;
- set `TYK_AI_SECRET_KEY` if you use SSO;
- give plugins that read host environment variables a `PLUGIN_ENV_PASSTHROUGH` entry.

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
- **Guardrails for tool results** (Enterprise). Tool output is how instructions reach a model without the user typing them. Request guardrails now inspect every tool result, across the whole conversation history, whatever the filter's scope.
  - **Every format:** OpenAI, Ollama, Anthropic (including server-tool and MCP results), Google AI and Vertex, Bedrock Converse, the Responses API's tool outputs, each item of an Anthropic Message Batch and each line of an OpenAI batch file, and the tools of Studio chat.
  - **Quarantine, not block:** a flagged tool result is replaced with a placeholder and the request goes on, because blocking stops an agent for good (its next request carries the same tool result). A `block` filter quarantines tool results unless told otherwise.
  - **Affordable to run:** each node caches provider verdicts, so history an agent sends again is not classified again, and per-request budgets bound what a sender can make the gateway scan. Text past a budget is flagged as unscanned, never passed as clean.
  - **A new seeded filter**, **Prompt injection in tool results (quarantine)**, unattached.

  See `docs/site/docs/guardrails.md` (Tool results, Quarantine, Budgets and the verdict cache).
- **Credential references follow an operator policy** ([#794](https://github.com/TykTechnologies/ai-studio/pull/794), [#789](https://github.com/TykTechnologies/ai-studio/pull/789), [#788](https://github.com/TykTechnologies/ai-studio/pull/788), [#766](https://github.com/TykTechnologies/ai-studio/pull/766)).
  - **`$ENV/NAME` resolves only for names in `AI_STUDIO_ENV_REF_ALLOWLIST`**, which is empty by default. This applies on every path: LLMs, tools, datasources, embedders, guardrail connections and plugin configuration.
  - **Setting or changing a `$SECRET/` or `$ENV/` reference needs `secrets:read`.** Keeping a stored reference needs nothing extra. Submissions and plugins cannot set references.
  - **Edges receive resolved values.** A reference the hub does not resolve reaches an edge as an unusable value, never as a name the edge could resolve itself.
  - **Tool auth keys stored as references are resolved when AI Studio calls the tool**, in chat and on the embedded gateway, and stay references when the tool is edited.

  See `docs/site/docs/secrets.md`.
- **Plugin hardening** ([#750](https://github.com/TykTechnologies/ai-studio/pull/750), [#775](https://github.com/TykTechnologies/ai-studio/pull/775), [#741](https://github.com/TykTechnologies/ai-studio/pull/741), [#776](https://github.com/TykTechnologies/ai-studio/pull/776), [#745](https://github.com/TykTechnologies/ai-studio/pull/745), [#768](https://github.com/TykTechnologies/ai-studio/pull/768), [#749](https://github.com/TykTechnologies/ai-studio/pull/749)).
  - **Explicit environment.** Plugins run with an environment allowlist instead of the host's environment. `PLUGIN_ENV_PASSTHROUGH` adds variables.
  - **Declared scopes.** Brokered service-API calls are checked against the scopes the plugin's manifest declares and an admin approved. A plugin reads and writes its own configuration without a scope.
  - **Plugin hosts.** Hosts are classified by parsed address, not by substring, and `100.64.0.0/10` counts as internal.
  - **Cached binaries.** Cached OCI plugin binaries are checked against their fetch-time hash before they load.
  - **Broker readiness.** The plugin broker waits for the plugin's ready signal.

  See `docs/site/docs/plugins-security.md`.

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

### Guardrails for tool results (Enterprise)
- **Tool results are read where they are.** A guardrail reads a request body block by block in every vendor format (`guardrails/segments`): each message's text and each tool result (OpenAI `tool` and `function` messages, Anthropic `tool_result` and server-tool results such as `web_fetch_tool_result` and `mcp_tool_result`, Google `functionResponse`, Bedrock `toolResult`, Responses `function_call_output`, `custom_tool_call_output` and the outputs of built-in tool items). A redaction rewrites only the matched strings, so tool calls, ids, images and `cache_control` reach the vendor unchanged. A JSON body in another shape is read as all of its decoded strings.
- **Scanned by default, across the history.** `tool_results.scan` (default on) inspects every tool result whatever the scope, and `tool_results.on_detect` sets an action for them; unset, they get the filter's action, except that `block` becomes `quarantine`. The scope `last_user` now means the user text of the current turn.
- **`quarantine` action** (requests only). The flagged tool result, or message, is replaced whole with a placeholder (`quarantine.placeholder`, default `[content removed by policy: {{detector}} detected in {{source}}]`, with `{{category}}` and `{{tool_name}}` too), keeping its tool call id and name. Tool names are reduced to safe characters before they are inserted. Where nothing can be replaced in place, the request is blocked.
- **What the model reads is what is scanned.** A tool result whose text is JSON, or holds JSON escapes, is also classified decoded, so an instruction behind `\u` or `\n` escapes is caught. A message or tool result of several blocks is classified as a whole, so text split across blocks, even inside a word, is caught. Text documents sent as base64 (Bedrock `txt`, `md`, `html`, `csv`; Anthropic `text/` documents) are decoded and classified.
- **Verdict cache and budgets.** Each node caches the provider's verdict per segment (`GUARDRAIL_VERDICT_CACHE_*`), keyed on the filter version and configuration, the provider and its version, the direction, the segment's role, kind and tool name, its text and the app (or user or chat session); a request with none of those is not cached. For remote providers, a filter's `budget` (`max_scan_chars`, `max_pieces`, `time_ms`, `single_call_chars`) bounds one request. Text beyond a budget, refused by the provider (HTTP 400, 413, 422) or left unfinished is **unscanned** and takes the filter's action; a call the provider refuses is re-sent segment by segment, so only the refused text is flagged. Tool results never share a provider call with the user's or the system's text. `"cache": false` turns the cache off for one filter.
- **Built-in library on agent traffic.** The library is up to 16 times faster on prose (16 MiB in 1.1 s instead of 17.7 s); text dense in numbers gains less. On tool results it uses narrower variants of `role_hijack`, `pseudo_tags`, `markdown_exfil`, `indirect_marker` and `base64_blob`, measured against a corpus of realistic tool output on which `secrets` and `injection` raise nothing. In-process scans run in two pools (small and large), with a per-tenant share and queue, so a request with at most 32K characters of tool results (and of other text) never waits behind a flood of large tool results. A segment larger than `GUARDRAIL_LOCAL_MAX_SCAN_BYTES` (8 MiB) is unscanned.
- **Vendor providers handle tool results.** Lakera Guard gets each tool result as a `tool` message (screened as untrusted content), Azure AI Content Safety Prompt Shields as a document (indirect attacks), Presidio and Azure AI Language PII analyse and redact it like any other text, and Bedrock Guardrails assesses it as input. The HTTP classifier contract adds `kind: "tool_result"`, `tool_name` and `tool_call_id` (cut to 256 bytes) on tool segments; other segments are unchanged, so existing services keep working.
- **Provider SDK.** `guardrails.Capabilities` gains `ToolSegments`, `SegmentPerCall`, `MaxCallChars`, `Contextual` and `Version`. Every provider must pass the new conformance suite, `guardrails/conformance`: mixed roles, empty and maximum-size tool results, findings tied to the right segment, no panics, and a provider that ignores its context fails. A provider that panics is recovered, and on requests its text counts as unscanned (quarantined or blocked whatever the fail mode); one that does not return after its timeout is given up on (`aistudio_guardrail_calls_abandoned_total`), and a remote provider whose calls in small requests keep being given up on, across tenants, is suspended (one probe every 10 seconds).
- **Studio chat.** The guardrails attached to a chat as request filters screen what its REST and client tools return (and the error text of a failed call) before the model, the user's screen or the stored conversation sees it: quarantine replaces it, redact rewrites it, block stops the turn and stores a policy notice. The chat's script request filters also run on a user's answer to a client tool card.
- **Native batches.** Each item of an Anthropic Message Batch and each line of an OpenAI batch file is screened, and a quarantine rewrites only that item. One batch may make at most `NATIVE_API_MAX_GUARDRAIL_CALLS` remote provider calls (10,000); a batch that needs more is refused with HTTP 400 naming the limit. An OpenAI batch runs only under the request filters that screened its file.
- **Script filters.** `input.segments` lists each message's text and each tool result (`index`, `role`, `kind`, `tool_name`, `text`), `input.segments_readable` says whether the body could be read, and `tyk.quarantine_segment(input, index, placeholder)` returns the body with one segment replaced. The template **Built-in library: scan tool results** uses them.
- **Admin console.** The guardrail form has a **Tool results** section (**Scan tool results**, **Action on tool results**, **Quarantine placeholder**) and puts the verdict cache and the budgets under **Advanced: verdict cache and budgets**; quarantine is offered as an action on request filters only; the detail page shows them. The **Test** panel reads raw input as a vendor request body (**Request body format**), can insert a tool-result sample, and shows the body as it would reach the vendor. The compliance **Filter Events** tab has a **Source** column (`Tool result from fetch_url`).
- **Compliance events and metrics.**
  - Guardrail events carry `kinds` and `tool_names` beside `roles`.
  - `guardrail.unscanned` records segments not scanned in full, with the reason (`size`, `pieces`, `time_budget`, `busy`, `input_rejected`, `provider_error`, `provider_panic`, `seam`, `undecodable`), `guardrail.unredactable` a redaction that could not be written into the body (the tool result was quarantined or the request blocked instead), and `guardrail.unreadable_request` a request refused because its keys read differently to different parsers.
  - New metrics `aistudio_guardrail_segments_total{provider,segment_kind,result}`, `aistudio_guardrail_cache_total{provider,outcome}` and `aistudio_guardrail_calls_abandoned_total{provider}`. `aistudio_guardrail_latency_seconds` gains a `segment_kind` label, and `aistudio_guardrail_errors_total` a `fail_mode` value of `unscanned`.
- **New seeded filter.** **Prompt injection in tool results (quarantine)**: the built-in injection heuristics, recording a flagged user message and quarantining a flagged tool result. It is seeded unattached on new installs, and on upgraded ones on their next start (unless `SKIP_FILTER_DEFAULTS=true`); existing filters are left as they are. Measured before release: none of 16 benign tool-output samples flagged, every injected sample caught.
- **Live end-to-end suite.** `tests/guardrail_tool_results/` runs an Enterprise AI Studio and an edge against a recording mock upstream and checks every vendor format through both, buffered and streamed, plus Studio chat and native batches. It needs no vendor account.

### Guardrails for agent actions (Enterprise)
New settings default to off, so existing filters behave exactly as before.
- **Tool calls as segments.** With `tool_calls.scan` on, a guardrail reads the arguments of every tool call the model makes, in a request's history and in a buffered response, in every vendor format (OpenAI `tool_calls` and `function_call`, Ollama, Anthropic `tool_use`, `server_tool_use` and `mcp_tool_use`, Google `functionCall`, Bedrock `toolUse`, Responses `function_call`, `custom_tool_call`, `mcp_call` and `mcp_approval_request`), on `/llm/`, on the Bedrock bridges and in native batch results. `tool_calls.on_detect` is `block` or `log` (unset: `block` on a Block filter, else `log`); on a response, `block` refuses it before the client can run the call. Calls are classified as the body holds them and decoded, across the whole history, never in a provider call with the client's own text or with tool results. Compliance events gain the `tool_call` kind, and the `segment_kind` metric label the `tool_call` value. The admin form has a **Tool calls** section, and the detail page shows it.
- **Streamed tool calls are held until classified** (native `/llm/` streams, and the `/ai/` bridge's inner hop) when a response guardrail scans tool calls: OpenAI, Anthropic, Responses, Google (SSE and JSON array) and Ollama. Text before a call streams as before; from the call on, events are held in order, released byte for byte when the calls are clean, and never sent when one is flagged under `block` (the stream ends with the vendor's error event). A call over 1 MiB or held over 30 s is unscanned and gets the tool-call action. Zero overhead without tool-call scanning.
- **Studio chat refuses flagged tool calls.** The chat's request guardrails that scan tool calls screen each call the model asks for, with the user's latest message as its goal, before a REST tool runs or a client tool card is shown. A blocked call is never executed: a policy notice is stored as its result and the turn stops.
- **Goal context.** Each tool call carries the latest user text before it and, with `tool_calls.include_system`, the system prompt (each cut to 4,096 characters), for providers that judge a call against the goal: `guardrails.Capabilities.WantsGoal`, declared by the HTTP classifier, whose contract gains `kind: "tool_call"` and a `context` object on tool call segments only. The verdict cache keys a call on its context. Tool results never carry one.

- **Tool definitions as segments.** With `tool_definitions.scan` on, a request guardrail classifies each tool a request offers the model (OpenAI `tools` and `functions`, Ollama, Anthropic, Google `functionDeclarations`, Bedrock `toolConfig`, Responses function and custom tools), once per definition thanks to the verdict cache (reordered lists hit too). `tool_definitions.on_detect` is `remove` (drop the tool from the request and go on; blocked when that would leave no tool or the request forces it), `block` or `log`; unset, `remove` on a Block or Quarantine filter, else `log`. The tools of a Studio tool served over MCP are screened by the tool's request guardrails when its MCP server is built, and a Studio chat's tools by the chat's request guardrails each turn. Events gain the `tool_definition` kind; the HTTP contract sends `kind: "tool_definition"` with `tool_name`. The form has a **Tool definitions** section on request filters.
- **Shadow (don't wait).** `"shadow": true` on a guardrail whose every action is `log` (anything else is refused with 400) returns at once and classifies in a bounded background pool per node (`GUARDRAIL_SHADOW_WORKERS`, `GUARDRAIL_SHADOW_QUEUE_BYTES`, half per tenant), recording the same compliance events as an inline log run when it finishes, on requests, responses, Studio chat and tool filters, at the hub and at edges. A full pool drops the evaluation (`aistudio_guardrail_shadow_dropped_total`, one `guardrail.unscanned` event with reason `shadow_dropped`) rather than delay anything; shutdown waits up to 5 s. The form has a **Shadow (don't wait)** toggle.
- **HTTP classifier over a Unix socket.** The endpoint can be `unix:///path/to.sock` (with the new `path` connection field for the request path), so a classifier on the same host needs no port and can run with `network_mode: none`. Sockets are refused unless listed in the new `GUARDRAIL_UNIX_SOCKET_ALLOWLIST` (exact paths, or directories ending in `/`; empty by default), checked when a filter is saved or tested and before every call on every node, with symlinks resolved and the target required to be a socket. A socket a node refuses is a provider error there.

### Platform and console
- **`$SECRET/` references in plugin configuration** ([#766](https://github.com/TykTechnologies/ai-studio/pull/766)). A plugin's config fields can name a stored secret, which is resolved for the plugin and stays masked in the config form. Adding one needs `secrets:read`.
- **`tyk.llm` `return_errors`** ([ai-studio-enterprise#76](https://github.com/TykTechnologies/ai-studio-enterprise/pull/76)). With `return_errors: true`, a failed call returns an error value instead of stopping the script, so a guardrail script can block when its checker is unavailable.
- **Startup warning for `FILTER_SCRIPT_ALLOW_OS`** ([#790](https://github.com/TykTechnologies/ai-studio/pull/790), [ai-studio-enterprise#85](https://github.com/TykTechnologies/ai-studio-enterprise/pull/85)). AI Studio and the Microgateway log a warning at startup when filter scripts get Tengo's `os` module, and the runner and the warning read the switch the same way.
- **Catalogue picker on the submission Approve dialog** ([#756](https://github.com/TykTechnologies/ai-studio/pull/756)).
- **Privacy warning** ([#757](https://github.com/TykTechnologies/ai-studio/pull/757)). The tool and datasource forms warn when the chosen privacy level is above every active LLM provider.
- **Push reminder** ([#752](https://github.com/TykTechnologies/ai-studio/pull/752)). After you save an object edges use, the console says straight away that edges need a configuration push.
- **Helm `config.logLevel`** ([#748](https://github.com/TykTechnologies/ai-studio/pull/748)) sets AI Studio's `LOG_LEVEL` (default `info`).

## Bug fixes

### Chat
- **Long sessions sent their oldest messages** ([#791](https://github.com/TykTechnologies/ai-studio/pull/791)). A session loaded its first 100 messages, so once it passed 100 the newest ones, including the question being asked, were not sent. Sessions now send the newest 100 messages, with the system prompt kept in front.
- **History trimming broke tool-using conversations** ([#791](https://github.com/TykTechnologies/ai-studio/pull/791)).
  - Trimming to the LLM settings' max length removed single messages, so a tool call could lose its results and the vendor refused the request. History is now trimmed a whole turn at a time, and tool calls count toward the estimate.
  - The latest user message and what follows it are never trimmed.
  - An unset max length now means no limit. It used to drop almost the whole history.
- **Long replies failed on the Postgres chat queue** ([#761](https://github.com/TykTechnologies/ai-studio/pull/761)) with `payload string too long (SQLSTATE 22023)`. Messages over the `NOTIFY` limit are now split.
- **`$SECRET/` tool auth keys were sent as the literal reference** on chat and embedded-gateway tool calls ([#788](https://github.com/TykTechnologies/ai-studio/pull/788)).

### Tools
- **An `oas_spec` the tool cannot use is refused** ([#758](https://github.com/TykTechnologies/ai-studio/pull/758)). A spec that was not base64-encoded OpenAPI was stored and left the tool unusable. A REST tool now gets `400`, on create and on an update that changes the spec, and **Import from URL** says why a document was refused.
- **One duplicate tool name froze an edge's configuration** ([#785](https://github.com/TykTechnologies/ai-studio/pull/785), [#786](https://github.com/TykTechnologies/ai-studio/pull/786)). Two tools whose names give the same slug made every push to an edge fail as a whole. A name whose slug is taken is now refused, and the console shows the conflict under the Name field. Edges skip entries they cannot store and report them on the push. See the upgrade notes.
- **Editing a tool with an `$ENV/` auth key stored the environment value** in place of the reference ([#788](https://github.com/TykTechnologies/ai-studio/pull/788)).

### Filters and scripts
- **Built-in filter templates** ([#739](https://github.com/TykTechnologies/ai-studio/pull/739)). The pattern-match template matches case-insensitively, and the LLM Check template fences the screened text and blocks when the check fails.
- **A response filter let the response through when `tyk.llm` failed** ([ai-studio-enterprise#76](https://github.com/TykTechnologies/ai-studio-enterprise/pull/76)). Scripts can now handle the failure with `return_errors` (see New features).
- **Guardrail Redact on response filters** ([#746](https://github.com/TykTechnologies/ai-studio/pull/746)). The form now says up front that Redact only logs on a response filter.

### Plugins
- **The plugin broker gave up waiting after 500 ms** ([#768](https://github.com/TykTechnologies/ai-studio/pull/768)) and carried on whether the plugin was ready or not. It now waits for the plugin's ready signal, and the SDK buffers events until the broker is up instead of dropping them.
- **Plugin config forms showed secret fields in clear** when a field had a long description ([#769](https://github.com/TykTechnologies/ai-studio/pull/769)).
- **A plugin's sidebar section showed Configuration twice** when the plugin had its own Configuration page ([#744](https://github.com/TykTechnologies/ai-studio/pull/744)).
- **The plugin examples, scaffold and docs warm up the broker with a call that needs no scope** ([#774](https://github.com/TykTechnologies/ai-studio/pull/774), [ai-studio-community-plugins#28](https://github.com/TykTechnologies/ai-studio-community-plugins/pull/28)), so they keep working under scope enforcement.

### Console
- **Deep links survive sign-in** ([#742](https://github.com/TykTechnologies/ai-studio/pull/742)). Navigation names are unique, and the catalogs and datasources routes are consistent.
- **View in Catalog on a submission** opens the resource it created ([#754](https://github.com/TykTechnologies/ai-studio/pull/754)).
- **The quick start names empty provider secrets**, and the onboarding cards state their prerequisites ([#751](https://github.com/TykTechnologies/ai-studio/pull/751)).
- **Edit app** sits in the App details header ([#759](https://github.com/TykTechnologies/ai-studio/pull/759)).
- **Multi-select fields release outside clicks**, and a failed submit scrolls to the first error ([#762](https://github.com/TykTechnologies/ai-studio/pull/762)).
- **Credential status says an unresolved `$ENV/` provider key is an unset environment variable** ([#763](https://github.com/TykTechnologies/ai-studio/pull/763)). The credential help texts point at the secret store ([#796](https://github.com/TykTechnologies/ai-studio/pull/796)).

### Datasources
- **PDF extraction** ([#753](https://github.com/TykTechnologies/ai-studio/pull/753), [#765](https://github.com/TykTechnologies/ai-studio/pull/765)). `pdftotext` is looked up once, only on the `PATH`, and runs with a minimal environment. The log names the extractor in use once.
- **The vector store list** ([#777](https://github.com/TykTechnologies/ai-studio/pull/777)) offers the stores datasources implement (it listed Milvus and left out PGVector and Redis).

### Guardrails
- **A guardrail redaction flattened tool-using conversations.** Redacting anything in a request rebuilt the body as plain `{role, content}` messages, dropping tool calls, tool results and their ids, images and `cache_control`, so the vendor refused the request or saw a different conversation. Redactions are now written into the body in place.
- **A redaction that could not be applied fell to the fail mode**, so a fail-open guardrail forwarded the unredacted text. Such a request is now blocked (or, for a tool result, quarantined) whatever the fail mode.
- **JSON escapes hid text from guardrails.** A body in a shape the guardrail did not read was classified as raw JSON, so an instruction after an escaped newline, or spelled with `\u` escapes, was not matched. Every string is now decoded first.
- **False positives on placeholder passwords.** `secrets.connection_string` and `secrets.basic_auth_url` skip a password that is a variable (`$DB_PASSWORD`, `${DB_PASSWORD}`), a placeholder of words in angle brackets (`<password>`), or a mask (`****`), as configuration files and READMEs write them.
- **Response filters missed text in streams** (native `/llm/` paths). Text was read per 1 KiB network read, so an event cut across two reads was lost, and Responses API, Google JSON-array and Ollama streams yielded no text at all, so response guardrails never ran on them. Streams are now read event by event in every format, and each event reaches the client once the filters have seen it (unchanged bytes; without response filters, unchanged relaying).
- **Native Bedrock `converse-stream` dropped tools and every non-text block.** The streaming path rebuilt the request from its text only, so `toolConfig`, tool use and tool results, images, documents and model-specific fields never reached Bedrock (no tool use when streaming, and nothing for tool-result guardrails to act on), and it re-encoded the response events, dropping reasoning and citations. It now forwards the request as the client wrote it, signed like the native REST path, and relays the event stream frame by frame, byte for byte. Response guardrails now run on it (per chunk and once at the end; they never did before) and hold its `toolUse` when they scan tool calls; a block ends the stream with a `validationException`. A non-streaming operation on the streaming route is refused (400) rather than reinterpreted. A Bedrock endpoint that names no region takes it from the LLM's `metadata.aws_region`.
- **Guardrails never ran on the Bedrock bridges' streams.** The `/ai/` and `/anthropic/` bridges to Bedrock handed response filters text they could not read, so guardrails never evaluated a streamed Bedrock answer there, and neither bridge had an end-of-stream pass. Both now screen the SSE they write as the native streams are screened: text per event and at the end, tool calls held until classified, a block ending the stream with an OpenAI error and `[DONE]` or an Anthropic `event: error`.
- **A Bedrock LLM moved to another endpoint kept calling the old one** until the gateway restarted, when its credentials stayed the same: the cached client is now keyed on the endpoint too.
- **A blocked stream ended with an error SDKs ignored.** The gateway wrote bare JSON with no `data:` line. It now ends the stream with the vendor's own error event (OpenAI error object and `[DONE]`, Anthropic and Responses `event: error`, Google error object, Ollama error line), which the official OpenAI, Anthropic and Google SDKs raise.
- **`/llm/rest` with a streaming body was not filtered.** With response filters it buffered the stream and then let it through unread; it now streams through the streaming path and is filtered. Bedrock `invoke-with-response-stream` on an LLM with response filters is refused (HTTP 400) instead of passing unfiltered.
- **The filter form said response filters do not run on tools.** They run on a tool's output; the help text now says what each kind of filter does there.

### Gateway
- **OpenAI `/v1/responses` calls were not recorded** (issue [#697](https://github.com/TykTechnologies/ai-studio/issues/697), [#737](https://github.com/TykTechnologies/ai-studio/pull/737)). Response analysis rejected the path, so those calls wrote no proxy log, no cost and no budget spend, which let an App use the Responses API past its budget.
- **The service tier of synchronous OpenAI calls was ignored** ([#737](https://github.com/TykTechnologies/ai-studio/pull/737)). `flex` and `priority` calls are now recorded with their tier, so price overrides and the tier report apply to them. With no override they are still billed at the list price.
- **Anthropic `count_tokens` and batch calls wrote zero-cost chat records** ([#730](https://github.com/TykTechnologies/ai-studio/pull/730)). Only a `/messages` path is now read as a Messages call; the others still write their proxy log.
- **Native OpenAI decision calls were not recorded** ([#724](https://github.com/TykTechnologies/ai-studio/pull/724)). `/llm/rest/{openai-llm}/v1/decisions` already passed through, but response analysis rejected the path, so those calls wrote no proxy log, no cost and no budget spend.
- **OpenAI and Ollama `/llm/` requests with array content were refused.** A chat completions body whose message content is a list of parts (text and `image_url` parts, a tool message with text parts) got `400` ("cannot unmarshal array") from the gateway's request screen before reaching the vendor. The screen now reads only the fields it needs.
- **Calls to a model alias were recorded at zero cost** ([#724](https://github.com/TykTechnologies/ai-studio/pull/724)). When the model a provider reports has no price, the call is now priced by the model that was requested. Jev answers `jev-latest` as `jev-1.13.0`, and OpenAI reports dated snapshots of chat models.
- **OpenAI moderations without a `model` were refused** ([#755](https://github.com/TykTechnologies/ai-studio/pull/755)). They pass, since the moderations endpoint picks its own model.

### Analytics
- **Anthropic Chat turns with several content blocks were billed two or three times** ([#736](https://github.com/TykTechnologies/ai-studio/pull/736)). The usage of a turn with text and a tool call (or a thinking block) was counted once per block, in tokens, cost and budget. It is now read once per turn.
- **Cached tokens in Chat sessions were billed at the input price** ([#736](https://github.com/TykTechnologies/ai-studio/pull/736)). Chat-session records now use the same pricing as gateway records: cached tokens are billed at the cache price, and an unset cache price falls back to the input price as before.
- **The cost breakdown of batch records did not add up** ([#730](https://github.com/TykTechnologies/ai-studio/pull/730)). The per-model breakdown is scaled to each record's recorded cost, and OpenAI and Google cache cost with an unset cache price is no longer reported as zero.
- **Model price recalculation** ([#730](https://github.com/TykTechnologies/ai-studio/pull/730)) keeps each past record's ratio to the list price, so batch records stay at half price, and it now runs on SQLite too.

### Control plane and edges
- **Edge control-client data races** ([#725](https://github.com/TykTechnologies/ai-studio/pull/725)).
  - The connected flag was shared between the connect path, the stream's receive loop and the heartbeat worker without synchronisation. A stream that dropped immediately could also be left marked as connected.
  - The heartbeat worker watched the client's current stream context, which a reconnect replaces. The old worker then waited on the new stream and never stopped, so every reconnect left one more heartbeat worker running.
- **An edge that dropped mid-heartbeat could stay marked as connected** ([#779](https://github.com/TykTechnologies/ai-studio/pull/779)).
- **A push could lose an edge's READY report** under contention ([#773](https://github.com/TykTechnologies/ai-studio/pull/773)), leaving the push waiting.
- **Heartbeats report the configuration an edge applied** ([#785](https://github.com/TykTechnologies/ai-studio/pull/785)), not the last one it received. An edge whose last sync failed now shows as not in sync.
- **SQLite writes failed with "database is locked" under concurrent writes** ([#800](https://github.com/TykTechnologies/ai-studio/pull/800)). AI Studio opens SQLite files with the Microgateway's defaults (WAL, a busy timeout, write transactions that take the lock up front). See the upgrade notes.
- **Stop waits for background work** ([#728](https://github.com/TykTechnologies/ai-studio/pull/728), [#792](https://github.com/TykTechnologies/ai-studio/pull/792)). The legacy secret re-encryption and the notification body backfill are cancelled and waited for, so they no longer outlive a stopped Studio.

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
- **The Helm chart renders `extraEnv`, `extraVolumes` and `extraVolumeMounts` for AI Studio** ([#735](https://github.com/TykTechnologies/ai-studio/pull/735)).
- **The quickstart ships placeholders instead of sample secrets** ([#729](https://github.com/TykTechnologies/ai-studio/pull/729)). Its env files carry placeholders that must be replaced before the first start, and the deployment docs' sample outputs are marked as samples.

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
- **Ambiguous request bodies with request filters attached.** With any request filter on the LLM, a request whose JSON repeats a key is now refused with `400` on every `/llm/` route and format, not only on the native batch and Anthropic messages paths, and a guardrail also refuses a body that spells a field it reads in different case (`"Content"`). A filter and the vendor could otherwise read different copies of the text. The refusal names only the format's own field names, never other keys, since keys can be tool data.
- **Guardrail error events no longer quote the provider.** `guardrail.error` names the class of failure (`timeout`, `http_503`, `input_rejected`) instead of the provider's error message, which could quote the screened text. A provider panic is logged without its value for the same reason.
- **Decision-metric labels.** `aistudio_decision_questions_total` counts only calls that a model answered or refused, so a failed call that names an invented model adds no series ([#724](https://github.com/TykTechnologies/ai-studio/pull/724)).
- **Credential references follow an operator policy** ([#794](https://github.com/TykTechnologies/ai-studio/pull/794), [#789](https://github.com/TykTechnologies/ai-studio/pull/789), [#788](https://github.com/TykTechnologies/ai-studio/pull/788)). See Highlights and the upgrade notes.
- **Plugin service-API calls are checked against the plugin's approved scopes** ([#775](https://github.com/TykTechnologies/ai-studio/pull/775), [ai-studio-enterprise#80](https://github.com/TykTechnologies/ai-studio-enterprise/pull/80), [ai-studio-community-plugins#28](https://github.com/TykTechnologies/ai-studio-community-plugins/pull/28)), and a plugin can update only its own configuration.
- **Vendor clients take keys and endpoints only from the object they serve** ([#797](https://github.com/TykTechnologies/ai-studio/pull/797)). The OpenAI, Anthropic, Google AI, Hugging Face and Ollama clients, and the Pinecone and Chroma vector stores, no longer fill an empty key or endpoint from environment variables.
- **The SSO nonce endpoint requires the identity broker's shared secret** ([#787](https://github.com/TykTechnologies/ai-studio/pull/787)). With `TYK_AI_SECRET_KEY` unset, every call is refused, and the secret is compared in constant time.
- **Assigning catalogues while approving a submission needs catalogue write permission** ([#772](https://github.com/TykTechnologies/ai-studio/pull/772)), as assigning them anywhere else does.
- **Plugin host checks** ([#741](https://github.com/TykTechnologies/ai-studio/pull/741), [#776](https://github.com/TykTechnologies/ai-studio/pull/776), [ai-studio-enterprise#81](https://github.com/TykTechnologies/ai-studio-enterprise/pull/81)). Plugin hosts are classified by parsed address rather than by substring, `100.64.0.0/10` counts as internal, and the Enterprise validator resolves a hostname and refuses one that does not resolve.
- **Cached OCI plugin binaries are verified against their fetch-time hash** before they load ([#745](https://github.com/TykTechnologies/ai-studio/pull/745)).
- **`pdftotext` runs only from the `PATH`, with a minimal environment** ([#765](https://github.com/TykTechnologies/ai-studio/pull/765)).
- **Secret fields stay masked in plugin config forms** ([#769](https://github.com/TykTechnologies/ai-studio/pull/769)), including the Redis passwords of advanced-llm-cache ([ai-studio-enterprise#78](https://github.com/TykTechnologies/ai-studio-enterprise/pull/78)) and rate-limiter ([ai-studio-community-plugins#27](https://github.com/TykTechnologies/ai-studio-community-plugins/pull/27)).

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
- **Existing request guardrails scan tool results** (Enterprise). A guardrail saved before 2.3.0 has no `tool_results` setting, which means scanning on: after the upgrade it inspects every tool result in the conversation, whatever its scope, and applies its action to them, with `block` becoming `quarantine`. For the seeded filters this means:
  - **Credentials in prompts (block)**, when attached, quarantines a tool result that contains a credential (a `.env` file an agent read, a config with a key in it). The agent goes on without that result.
  - **Personal data before the vendor (redact)** redacts personal data in tool output.
  - **Prompt injection heuristics (log)** records injection markers found in tool output.

  To keep a filter as it was, set `"tool_results": {"scan": false}` in its config (or untick **Scan tool results**).
- **Remote guardrail providers receive tool output.** Existing filters on Lakera Guard, Azure AI Content Safety, Azure AI Language PII, Amazon Bedrock Guardrails, Microsoft Presidio or an HTTP classifier start sending tool results to that provider on upgrade. Tool output can hold file contents, database rows and credentials an agent read. If a provider must not see that data, set `tool_results.scan` to `false` **and** use a scope other than `all_messages` (which inspects tool messages with scanning off, as it always has).
- **More provider calls.** Lakera Guard and Bedrock Guardrails now get one call per message and tool result (each screened in full, and every finding tied to its message). Lakera bills per request, and Bedrock per text unit with at least one per call, so a conversation of many short messages costs more than before. Azure AI Language PII sends one document per request instead of batching them: more requests, the same billed text records. The verdict cache answers history an agent sends again; `budget.max_pieces` (256) caps the calls of one request, and text beyond it is unscanned. A message or tool result of several blocks is classified twice (joined with a separator and run together), so it costs about twice.
- **Guardrail providers in shared address space are refused when internal upstreams are blocked** ([#776](https://github.com/TykTechnologies/ai-studio/pull/776)). Guardrail providers connect under the same outbound policy as LLM upstreams (`pkg/netguard`), which now counts `100.64.0.0/10` (shared address space, used by Tailscale and some Kubernetes CNIs) as internal. With `LLM_UPSTREAM_BLOCK_INTERNAL` on, a provider endpoint there (an HTTP classifier, Presidio) is refused and every call to it fails: under fail mode `open` small requests then pass unscreened, recorded only as `guardrail.error` events, and larger ones are unscanned. Name the host in `LLM_UPSTREAM_ALLOWED_INTERNAL_HOSTS` to exempt it from internal-range blocking.
- **Fail-open is narrower for large requests.**
  - When a request holds more than `single_call_chars` (32,768) characters to scan, or needs more provider calls than an ordinary request (for the per-message providers, more than four messages or four tool results), a provider failure leaves the affected segments unscanned, and the filter's action applies: a fail-open filter can block or quarantine such a request during an outage. Ordinary requests keep the fail mode. There is no outage-only switch, and changing the fail mode does not help: raise `budget.single_call_chars` (up to `max_scan_chars`) to bring more requests under it. Setting `tool_results.on_detect` to `log` also lets large agent requests through, but it stops the filter quarantining injected tool results at all times, not only during the outage, so use it only as a deliberate, temporary measure and set it back once the provider has recovered.
  - Text the provider refuses (HTTP 400, 413 or 422) is unscanned, not a fail-mode error.
  - The built-in library leaves a segment larger than `GUARDRAIL_LOCAL_MAX_SCAN_BYTES` (8 MiB), a scan that does not finish in time, or one that gets no slot, unscanned on requests, whatever the fail mode.
  - A native batch create or batch file upload whose screening needs more than `NATIVE_API_MAX_GUARDRAIL_CALLS` remote provider calls is refused with `400`.
- **Guardrails protect what reaches the vendor, not what AI Studio stores.** The proxy log keeps the body as it was forwarded, so redactions and quarantines show there, but a request a guardrail blocked is logged as it was received, with the text it was blocked for, and responses are logged as the vendor sent them. Do not read "credentials in tool results are quarantined" as "removed from the platform": set the LLM's **Disable Request/Response Body Logging** (`dont_log_bodies`) where that traffic must not be stored, and keep `ANALYTICS_STORE_REQUESTS` and `ANALYTICS_STORE_RESPONSES` off (the default) on the edges that serve it.
- **Edges must run 2.3.0 to scan tool results.** An older edge ignores the settings it does not know (tool-result scanning, the placeholder, budgets) and receives `block` in place of `quarantine`, so during a rolling upgrade it blocks a request a 2.3.0 edge would quarantine.
- **OpenAI batch files uploaded before 2.3.0 must be uploaded again** on an LLM with request filters: a batch created from one is refused with `400`. The same applies whenever the LLM's request filters change after an upload (a rotated provider key or moved endpoint does not count). A batch on such an LLM must be created through the gateway that received its file (the same edge, or AI Studio, whose replicas share one ledger).
- **Chat script filters see client tool answers.** A chat's script request filters now run on a user's answer to a client tool card, as on a typed message; a script that blocks also stops that turn.
- **Schema version 11** (`native_resources.screening_digest`, additive). `MinReaderSchemaVersion` stays 1.
- **Embedding hosts.** `AppConf.FilterLimits` (and the filter limits of `pkg/studio`) apply only to the names in `config.FilterLimitNames`, which now include the `GUARDRAIL_*` tuning variables; other names fall back to the environment.
- **Protocol.** The plugin, management and config-sync protos change additively: new fields and the new `CallDecision` RPC. [#737](https://github.com/TykTechnologies/ai-studio/pull/737) adds `LLMConfig` fields 27 to 31 (signing keys, workspace, price multipliers), `AnalyticsEvent.service_tier` and the plugin `AnalyticsRequest.service_tier`.
- **Go embedders and plugin authors.** `models.LLMVendorProvider` gains `ProvidesChat()` and `DecisionEndpoint()`, so a custom vendor implementation must add them. The plugin `StudioServices` interface is unchanged: `CallDecision` is a package-level SDK function, so custom fakes keep compiling.

- **`$ENV/` references resolve only for allowlisted names** ([#794](https://github.com/TykTechnologies/ai-studio/pull/794), [#789](https://github.com/TykTechnologies/ai-studio/pull/789)). List the environment variables your configuration names in `AI_STUDIO_ENV_REF_ALLOWLIST` (comma-separated, empty by default), or move those credentials to the secret store as `$SECRET/` references.
  - This covers LLMs, tools, datasources, embedders, guardrail connections and plugin configuration.
  - Do it **before** upgrading. References are stored as written, so a database dump lists the names in use: `pg_dump --data-only "$DATABASE_URL" | grep -oE '\$ENV/[A-Za-z_][A-Za-z0-9_]*' | sort -u` (or `sqlite3 midsommar.db .dump | …`).
  - A reference that is not on the list is sent as written: chat and the embedded gateway use the text `$ENV/NAME` as the key, so the provider or tool answers `401` (edges send `refused-credential-reference`), and a guardrail applies its fail mode. The variable's value is never sent.
  - Nothing is logged, and the console shows the LLM's credential as unresolved with "environment variable NAME, which is not set on the server" even when the variable is set: the cause is the allowlist. See `docs/site/docs/secrets.md` (Upgrading from 2.2).
- **Setting or changing a reference needs `secrets:read`** ([#794](https://github.com/TykTechnologies/ai-studio/pull/794), [#789](https://github.com/TykTechnologies/ai-studio/pull/789), [#766](https://github.com/TykTechnologies/ai-studio/pull/766)). A create or update that adds or changes a `$SECRET/` or `$ENV/` value answers `403` without it. Keeping a stored reference, or sending the redacted placeholder back, needs nothing extra. Submissions and plugin service-API writes cannot set references.
- **Vendor keys and endpoints are no longer read from the environment** ([#797](https://github.com/TykTechnologies/ai-studio/pull/797)). An LLM, embedder or datasource with an empty key or endpoint is no longer filled from `OPENAI_API_KEY`, `ANTHROPIC_API_KEY`, `GOOGLE_API_KEY`, `HF_TOKEN`, `PINECONE_API_KEY`, `CHROMA_URL` or `OLLAMA_HOST`. Set them on the object. Upgrade edges with the hub, and do not set vendor key variables on edges: older edges still read them.
- **Plugins get an environment allowlist** ([#750](https://github.com/TykTechnologies/ai-studio/pull/750)). A plugin no longer inherits the host's environment. Name any variables a plugin needs in `PLUGIN_ENV_PASSTHROUGH` (see `docs/site/docs/plugins-deployment.md`).
  - **github-rag-ingest** is the only shipped plugin whose 2.2 setup this breaks. It reads `GITHUB_RAG_SECRET_ENCRYPTION_KEY` and `GITHUB_RAG_KNOWN_HOSTS` from the host. If you set them, add `PLUGIN_ENV_PASSTHROUGH=GITHUB_RAG_*` (or set `secret_encryption_key` and `known_hosts_path` in the plugin configuration) **before upgrading**. Without the key, repositories whose PATs or SSH keys were stored encrypted fail to sync with "secret is encrypted but no encryption key is configured", and credentials saved afterwards are stored **unencrypted**; the plugin's warning about it is only visible at `LOG_LEVEL=debug`. Re-enter any credential saved while the key was missing, so it is stored encrypted.
  - `ALLOW_INTERNAL_NETWORK_ACCESS` is withheld on purpose: llm-price-sync and github-rag-ingest refuse internal price-source and repository URLs that worked on 2.2 unless you pass it through.
- **Plugins get only the scopes they declare** ([#775](https://github.com/TykTechnologies/ai-studio/pull/775)). A brokered service-API call outside the plugin's approved scopes is refused. Upgrading a plugin whose manifest declares new scopes asks an admin to approve them: advanced-llm-cache 1.2.2 adds `apps.write`. `GetPluginsCount`, the broker warm-up the 2.2 examples and scaffold used, still answers without `plugins.read` (the count only, no plugin rows), so plugins built from them keep working without a refusal in the log.
- **Cached OCI plugins are fetched once more after the upgrade** ([#745](https://github.com/TykTechnologies/ai-studio/pull/745)). Plugins cached by 2.2 have no recorded hash, so each is fetched from the registry again on its first load to record one; the registry should be reachable then. If it is not and signatures are optional, the cached copy loads unverified with a warning and the next load tries again. A rollback to 2.2 drops the recorded hashes, so rolling forward fetches the plugins once more.
- **Plugin hosts in shared address space** ([#776](https://github.com/TykTechnologies/ai-studio/pull/776), [ai-studio-enterprise#81](https://github.com/TykTechnologies/ai-studio-enterprise/pull/81)). A plugin command or endpoint in `100.64.0.0/10` counts as internal. In Enterprise, a hostname is resolved when the plugin is validated, and one that does not resolve is refused.
- **Tool and LLM names must have unique slugs** ([#785](https://github.com/TykTechnologies/ai-studio/pull/785)). A name that differs from an existing one only in case, spaces or punctuation is refused: `409` for tools and `400` for LLMs from the admin API, `AlreadyExists` from the plugin service API. Duplicates stored by earlier versions stay editable. Edges leave them out (the lowest ID keeps the slug) and show as Pending until they are renamed.
- **LLMs and tools that need a filter or plugin from another namespace** ([#785](https://github.com/TykTechnologies/ai-studio/pull/785)). A global LLM or tool attached to a filter or plugin filed in another namespace is left out on that namespace's edges. An endpoint whose auth plugin an edge does not receive is left out too. Move the filter or plugin to the global namespace or to the LLM's namespace.
- **Edge heartbeats report the configuration an edge applied** ([#785](https://github.com/TykTechnologies/ai-studio/pull/785)), so an edge whose last sync failed shows as not in sync.
- **SSO needs `TYK_AI_SECRET_KEY`** ([#787](https://github.com/TykTechnologies/ai-studio/pull/787)). The identity broker authenticates its nonce requests with it. With it unset, every SSO login is refused, and AI Studio logs a warning at startup when the broker is enabled without it.
- **SQLite runs in WAL mode** ([#800](https://github.com/TykTechnologies/ai-studio/pull/800)). AI Studio opens a SQLite file with `_journal_mode=WAL`, `_busy_timeout=5000`, `_synchronous=NORMAL` and `_txlock=immediate` unless `DATABASE_URL` sets them, and drops `cache=shared`. `-wal` and `-shm` files appear next to the database, so its directory must be writable. WAL does not work on network filesystems (NFS, SMB): set `_journal_mode=DELETE` in `DATABASE_URL` there. In-memory databases are unchanged.
- **Chat sessions send their history** ([#791](https://github.com/TykTechnologies/ai-studio/pull/791)). On LLM settings without a max length, a session now sends its conversation (up to the newest 100 messages) instead of little more than the latest message. Set **Max Length** on the LLM settings to cap the input.
- **Approving a submission into catalogues needs catalogue write permission** ([#772](https://github.com/TykTechnologies/ai-studio/pull/772)).
- **The quickstart needs your own secrets** ([#729](https://github.com/TykTechnologies/ai-studio/pull/729)). Replace the `__REPLACE_WITH_…__` placeholders in its env files before the first start (`quickstart/README.md` lists them); left in place, AI Studio and the Microgateway stop at startup with an error about the key length. `TYK_AI_SECRET_KEY` is now a placeholder too. A deployment that still uses values copied from an earlier quickstart or documentation sample should replace them with values you generate.
- **advanced-llm-cache 1.2.2** ([ai-studio-enterprise#75](https://github.com/TykTechnologies/ai-studio-enterprise/pull/75), [ai-studio-enterprise#84](https://github.com/TykTechnologies/ai-studio-enterprise/pull/84)).
  - Redis `tls_enabled` now takes effect: check that your Redis endpoint accepts TLS before upgrading.
  - The database and cluster addresses set in the configuration form now apply.
  - A stored `tls_skip_verify` stays ignored, with a warning. Use `tls_insecure` to turn off certificate verification.
  - The sharding, Redis timeout and cost-rule settings, which nothing implemented, are removed from the form.

### New configuration

No new runtime setting is required. Gateway connections are configured per connection in the console, not through the environment.

Bugsmash settings, on AI Studio and the Microgateway:

| Variable | Default | Purpose |
|---|---|---|
| `AI_STUDIO_ENV_REF_ALLOWLIST` | empty | Comma-separated environment variables a `$ENV/NAME` reference may resolve ([#794](https://github.com/TykTechnologies/ai-studio/pull/794)). Edges never resolve `$ENV/`; the hub sends resolved values |
| `PLUGIN_ENV_PASSTHROUGH` | empty | Comma-separated host variables passed to plugin processes, besides the built-in allowlist; a trailing `*` matches a prefix ([#750](https://github.com/TykTechnologies/ai-studio/pull/750)) |

The Helm chart adds `config.logLevel` ([#748](https://github.com/TykTechnologies/ai-studio/pull/748)), rendered as `LOG_LEVEL`.

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

Guardrails (Enterprise), on AI Studio and the Microgateway; an embedding host can set them in its filter limits (see `docs/site/docs/guardrails.md`):

| Variable | Default | Purpose |
|---|---|---|
| `NATIVE_API_MAX_GUARDRAIL_CALLS` | `10000` | Remote guardrail provider calls one batch create or batch file upload may make; cached verdicts and the built-in library do not count |
| `GUARDRAIL_VERDICT_CACHE_ENTRIES` | `100000` | Verdicts cached per node |
| `GUARDRAIL_VERDICT_CACHE_MAX_BYTES` | `67108864` (64 MiB) | Memory the verdict cache may hold per node |
| `GUARDRAIL_VERDICT_CACHE_TTL` | `10m` | How long a verdict is reused; `0` turns the cache off |
| `GUARDRAIL_LOCAL_SCAN_SLOTS` | half the CPUs (at least 1) | Large built-in library scans at once |
| `GUARDRAIL_LOCAL_SCAN_SMALL_SLOTS` | a quarter of the CPUs (at least 1) | Small scans (at most `single_call_chars`) at once, besides the large ones |
| `GUARDRAIL_LOCAL_SCAN_QUEUE_BYTES` | `67108864` (64 MiB) | Text one tenant may have waiting for a scan slot |
| `GUARDRAIL_LOCAL_MAX_SCAN_BYTES` | `8388608` (8 MiB) | Largest request segment the built-in library scans (64 KiB to 64 MiB); a larger one is unscanned |
| `GUARDRAIL_LOCAL_SCAN_MS_PER_MIB` | `2000` | Scan time per MiB of text, beyond the filter's `timeout_ms` (100 to 60000); text not scanned in time is unscanned |
| `GUARDRAIL_SHADOW_WORKERS` | 4 per CPU, at least 8 | Shadow guardrail evaluations running at once per process |
| `GUARDRAIL_SHADOW_QUEUE_BYTES` | `67108864` | Text waiting for a shadow evaluation (one tenant at most half); what does not fit is dropped and recorded as unscanned |
| `GUARDRAIL_UNIX_SOCKET_ALLOWLIST` | empty | Comma-separated Unix socket paths, or directories ending in `/`, that an HTTP classifier endpoint `unix:///...` may use. Empty refuses every socket endpoint |

On a 2-CPU host both scan pools have one slot, so a flood of large tool results can keep both cores busy. Run gateways that scan agent traffic with the built-in library on 4 or more CPUs, and set `GUARDRAIL_LOCAL_SCAN_SLOTS=1` on small hosts.

Per guardrail filter, in its config (the form saves only the keys you set, so untouched filters follow the defaults): `tool_results` (`scan`, `on_detect`), `on_detect: "quarantine"` with `quarantine.placeholder`, `budget` (`max_scan_chars` 262144, `max_pieces` 256, `time_ms` 4 × `timeout_ms`, `single_call_chars` 32768) and `cache`.

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

## Plugins

Plugins fixed in the bugsmash. Each is released to the marketplace for 2.3.0 at the version below.

| Plugin | Version | Changes |
|---|---|---|
| llm-cache | 1.2.1 | Lifecycle, edge ID and logging fixes: no `/tmp` debug log, no leaked goroutine, no double-close panic ([ai-studio-community-plugins#15](https://github.com/TykTechnologies/ai-studio-community-plugins/pull/15)) |
| llm-load-balancer (Enterprise) | 1.1.1 | Selection race, upstream URL validation and redaction, audit log file mode, invalid stored entries dropped at start-up with a logged error ([ai-studio-enterprise#74](https://github.com/TykTechnologies/ai-studio-enterprise/pull/74), [ai-studio-enterprise#83](https://github.com/TykTechnologies/ai-studio-enterprise/pull/83)) |
| advanced-llm-cache (Enterprise) | 1.2.2 | Backend swap race, Redis TLS, idempotent shutdown, masked Redis password, configuration swapped as one snapshot, form keys aligned with what the plugin reads, `apps.write` scope ([ai-studio-enterprise#75](https://github.com/TykTechnologies/ai-studio-enterprise/pull/75), [ai-studio-enterprise#78](https://github.com/TykTechnologies/ai-studio-enterprise/pull/78), [ai-studio-enterprise#80](https://github.com/TykTechnologies/ai-studio-enterprise/pull/80), [ai-studio-enterprise#84](https://github.com/TykTechnologies/ai-studio-enterprise/pull/84)) |
| rate-limiter | 1.1.1 | Redis client closed, corrupt counters handled, serialised rule saves, masked `redis_password` ([ai-studio-community-plugins#16](https://github.com/TykTechnologies/ai-studio-community-plugins/pull/16), [ai-studio-community-plugins#18](https://github.com/TykTechnologies/ai-studio-community-plugins/pull/18), [ai-studio-community-plugins#27](https://github.com/TykTechnologies/ai-studio-community-plugins/pull/27), [ai-studio-community-plugins#28](https://github.com/TykTechnologies/ai-studio-community-plugins/pull/28)) |
| llm-price-sync | 1.1.1 | Fetch URL checked against internal addresses, locked history updates, scheduler context, errors reported ([ai-studio-community-plugins#20](https://github.com/TykTechnologies/ai-studio-community-plugins/pull/20), [ai-studio-community-plugins#22](https://github.com/TykTechnologies/ai-studio-community-plugins/pull/22), [ai-studio-community-plugins#28](https://github.com/TykTechnologies/ai-studio-community-plugins/pull/28)) |
| llm-firewall | 1.1.1 | JSON-safe block response, optional `fail_closed`, all prompt text inspected ([ai-studio-community-plugins#21](https://github.com/TykTechnologies/ai-studio-community-plugins/pull/21), [ai-studio-community-plugins#25](https://github.com/TykTechnologies/ai-studio-community-plugins/pull/25)) |
| github-rag-ingest | 1.1.1 | Private clone cache with path checks, random secret IDs, index errors reported, `CancelJob` stops the running ingestion, masked `vault_token`, secret references for credentials ([ai-studio-community-plugins#17](https://github.com/TykTechnologies/ai-studio-community-plugins/pull/17), [ai-studio-community-plugins#19](https://github.com/TykTechnologies/ai-studio-community-plugins/pull/19), [ai-studio-community-plugins#23](https://github.com/TykTechnologies/ai-studio-community-plugins/pull/23), [ai-studio-community-plugins#24](https://github.com/TykTechnologies/ai-studio-community-plugins/pull/24), [ai-studio-community-plugins#26](https://github.com/TykTechnologies/ai-studio-community-plugins/pull/26), [ai-studio-community-plugins#28](https://github.com/TykTechnologies/ai-studio-community-plugins/pull/28), [ai-studio-community-plugins#29](https://github.com/TykTechnologies/ai-studio-community-plugins/pull/29)) |
| oauth2-client-credentials (Enterprise) | 1.3.2 | JWKS cache TTL and bounded stale keys; scopes documented ([ai-studio-enterprise#77](https://github.com/TykTechnologies/ai-studio-enterprise/pull/77), [ai-studio-enterprise#80](https://github.com/TykTechnologies/ai-studio-enterprise/pull/80)) |

github-rag-ingest 1.1.1 needs AI Studio 2.3.0 for `$SECRET/` references in its configuration and for `PLUGIN_ENV_PASSTHROUGH`.

## Known issues

- **Model and Semantic Routers do not route decision calls yet.** They answer `400`.
- **Decision conformance has not been run against a llama.cpp self-host.** It is green against OpenAI `gpt-6-luna` and TypeSafe `jev-latest`.
- **Raw OpenAI file and item IDs are forwarded unchecked.** A `file_id` in an `input_file` or `input_image` part and an `item_reference` ID are passed to OpenAI without an ownership check. Only batch input files can be uploaded through the gateway, and item IDs are only returned to the App that owns them, so such an ID comes from outside the gateway or from a leak. Whether to verify or refuse them is still being decided.
- **`input_file` content is not screened** by the filters, as with files on Chat Completions.
- **Standalone microgateways sign with the API key.** Without a hub there is no signing secret, so rotating a standalone microgateway's vendor API key still invalidates outstanding resource IDs.
- **An ambiguous database commit can lose a batch's usage.** If the connection drops just after the gateway marks a batch as accounted, the batch stays accounted with no usage recorded: an undercount, never a double charge. This is rare.
- **The proxy-log API does not return `service_tier` yet**; the tier report and chat records have it.
- **What guardrails do not inspect.** Binary documents (PDF, Word) and images; files referenced by id and conversation state the vendor holds (`previous_response_id`, a stored conversation); results of tools the vendor runs within one call (Anthropic server tools, OpenAI built-in tools) until the client sends them back; datasource context in Studio chat. A tool result's JSON escapes are decoded one level, so text escaped twice inside it is read with the inner escapes left in.
- **The built-in injection heuristics are a baseline.** On tool results they are deliberately narrower, to keep false positives on real tool output near zero, so paraphrased attacks get through. Pair them with Lakera Guard, Azure AI Content Safety Prompt Shields or Bedrock Guardrails for robust coverage. The narrower variants apply to anything labelled a tool result, including tool results a client writes itself, which matters only for direct-jailbreak policy. `markdown_exfil` is weak on tool output by design; use it on response filters (the seeded **Credential leakage in responses (block)** includes it).
- **Script filters see segments, not decoded views.** `tyk.detect` scans the text it is given; it does not read a tool result's JSON escapes decoded as guardrails do. An empty `input.segments` with a non-empty `raw_input` means an unreadable body, which a script that screens tool results should block.
- **Large tool results can still wait on slow hosts.** An ordinary request whose tool result is larger than `single_call_chars` uses the large scan pool and can wait behind a flood of near-limit scans. The per-tenant share limits one flooding tenant to half the slots; a lower `GUARDRAIL_LOCAL_MAX_SCAN_BYTES` shortens each flood scan.
- **Keys that differ only in case** can still be read differently by a script filter and a pass-through upstream; guardrails refuse such bodies.
- **Dashboard-created MCP proxies do not set `stripAuthorizationData`**, so AI Studio-minted Tyk keys are forwarded to third-party upstreams. Gateway connections set it. Changing it for Dashboard connections would alter existing proxies, so it is left for a separate decision.

## Submodules

- **enterprise** `118ad19` → `d7094dd`: Tyk OSS Gateway connections for MCP proxies and keys ([ai-studio-enterprise#70](https://github.com/TykTechnologies/ai-studio-enterprise/pull/70)), with pooled Gateway clients, parallel node work and the literal-credential warning; plus the base URL and custom-domain fixes for Dashboard connections.
  Bugsmash: plugin fixes and scopes ([ai-studio-enterprise#74](https://github.com/TykTechnologies/ai-studio-enterprise/pull/74), [ai-studio-enterprise#75](https://github.com/TykTechnologies/ai-studio-enterprise/pull/75), [ai-studio-enterprise#77](https://github.com/TykTechnologies/ai-studio-enterprise/pull/77), [ai-studio-enterprise#78](https://github.com/TykTechnologies/ai-studio-enterprise/pull/78), [ai-studio-enterprise#80](https://github.com/TykTechnologies/ai-studio-enterprise/pull/80), [ai-studio-enterprise#83](https://github.com/TykTechnologies/ai-studio-enterprise/pull/83), [ai-studio-enterprise#84](https://github.com/TykTechnologies/ai-studio-enterprise/pull/84)), `tyk.llm` `return_errors` ([ai-studio-enterprise#76](https://github.com/TykTechnologies/ai-studio-enterprise/pull/76)), plugin host classification ([ai-studio-enterprise#81](https://github.com/TykTechnologies/ai-studio-enterprise/pull/81)), the `FILTER_SCRIPT_ALLOW_OS` switch read in one place ([ai-studio-enterprise#85](https://github.com/TykTechnologies/ai-studio-enterprise/pull/85)), and guardrail tool-result scanning ([ai-studio-enterprise#82](https://github.com/TykTechnologies/ai-studio-enterprise/pull/82)).
- **community** `2e23cdf` → `3afb9f8`: bugsmash fixes to llm-cache, rate-limiter, llm-price-sync, llm-firewall and github-rag-ingest ([ai-studio-community-plugins#15](https://github.com/TykTechnologies/ai-studio-community-plugins/pull/15) to [ai-studio-community-plugins#29](https://github.com/TykTechnologies/ai-studio-community-plugins/pull/29)).
