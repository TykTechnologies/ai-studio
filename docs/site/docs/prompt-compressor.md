---
title: "Prompt Compressor"
weight: 43
---

# Prompt Compressor

The **Prompt Compressor** is a community plugin that shrinks LLM request bodies on the gateway before they reach the vendor. It starts with what agents send most of and the model needs least: tool outputs such as test runs, build logs, directory listings and large JSON results.

- **Cache-safe.** The output depends only on the text and the policy, never on time, load or earlier requests, so a compressed conversation prefix is byte-identical on every turn and the vendor's prompt cache keeps hitting.
- **Measured.** A dashboard shows the tokens and money saved next to the cache-read share, so a saving that costs more in cache misses than it gains is visible.
- **Fails open.** Any error, timeout or check that does not hold forwards the request exactly as the client sent it.

It is a Community Edition plugin, published in the plugin marketplace as `com.tyk.prompt-compressor`.

## What is covered

The plugin compresses on Microgateway ([edge gateway](/docs/edge-gateways)) instances, in both the Community and Enterprise editions. Studio's embedded gateway runs no gateway plugins, so a deployment needs at least one edge gateway for anything to be compressed; Studio hosts the pages and the savings records.

On the edge it runs in the `post_auth` hook of the `/llm/` endpoints. The gateway's OpenAI-compatible endpoints reach it through their internal `/llm/call/` hop.

| Traffic through an edge gateway | Compressed? |
|---|---|
| `/llm/rest`, `/llm/stream`, `/llm/call` | Yes |
| `/ai/{slug}/v1`, the unified `/v1` and `/router/` to a non-Bedrock LLM | Yes, on the internal `/llm/call/` hop |
| `/ai/` and `/anthropic/` to a Bedrock LLM | **No.** These bridges call AWS directly and run no `post_auth` plugins. A later release adds a hook for them. |
| Studio Chat, and anything through Studio's embedded gateway | **No.** |

Supported request formats: OpenAI Chat Completions (and OpenAI-compatible vendors), OpenAI Responses (Codex), Anthropic Messages, Google AI and Vertex (Gemini), Bedrock Converse on the `/llm/` endpoints, and Ollama. Any other body passes through unchanged. The format comes from the LLM's vendor, the path and the LLM's endpoint.

By default only tool outputs are compressed. A policy's `roles` can add user, assistant or system text. What is never rewritten, at any level and with any `roles`:

- results of the vendor's own server tools (Anthropic web search, web fetch, code execution and MCP connector results, Google code execution, the Responses built-in tools): the vendor may check them against signed parts beside them;
- tool call IDs, images, thinking and reasoning blocks, signatures, and `cache_control` or `cachePoint` markers;
- the structure of the body: only string values change, in place.

A rewritten unit is never longer than the original and never moves between roles, and tool calls stay paired with their results. A body that fails any of these checks after compression is forwarded unchanged.

When a request uses the vendor's own context management, the plugin stays at level 1 for it.

## Getting started

1. Install the plugin from **Plugins > Marketplace** and approve its scopes (see [Scopes](#scopes)).
2. Attach it to one or more LLMs. The edge gateways load it on the next configuration push.
3. Leave `mode` at `audit` (the default). The plugin compresses every request, records what it would have saved, and forwards the original body.
4. Watch **Prompt Compressor > Savings** for a few days. Try your policies on real bodies in **Prompt Compressor > Preview**.
5. Switch to `enforce` on the **Policies** page and push the configuration to the edges.

## Modes

| Mode | What happens |
|---|---|
| `off` | Nothing. |
| `audit` | Compresses, records the result, and forwards the **original** body. The dashboard shows projected savings. This is the default. |
| `enforce` | Forwards the compressed body. The dashboard shows actual savings. |

## Levels

Levels are cumulative: each one applies everything below it.

| Level | Name | What it does |
|---|---|---|
| 0 | off | Nothing. |
| 1 | lossless | JSON is compacted on its bytes, keeping key order, duplicate keys, number spelling and escapes. Text loses ANSI escape codes, carriage-return progress frames (rendered as a terminal shows them), trailing whitespace and runs of blank lines. |
| 2 | noise reduction | Level 1, then a reducer for text with a shape it knows. Test runners (pytest, jest, go test, cargo test, Maven surefire, Gradle) keep every failure with its message and location, and the totals. Logs and listings (terraform plan, kubectl, git diff, grep and ripgrep, directory listings, build logs, stack traces) keep every line that mentions an error. Long JSON arrays keep their first and last 5 items and every item that failed or stands out. Text no reducer recognises, such as prose or code, gets level 1 only. |
| 3 | aggressive | Level 2, then outputs still over the size budget (`max_unit_tokens`, default 6000) are cut to their head and tail. With `mask_after_turns` set, tool results older than that many turns become a placeholder (off by default). Optionally, an external engine compresses what is left. |

Every cut leaves an inline marker in a fixed format, so the model knows something was removed:

```
[tyk-compressed: pytest output, 18,400 → 1,150 tokens; 3 failures kept]
[tyk-compressed: 12 lines]
```

Lines matching a policy's `protect_patterns` (RE2 regular expressions, such as ticket IDs) survive levels 2 and 3. `protect_recent_turns` (default 1) keeps the newest turns at level 1, so the model reads its latest tool output in full.

Levels 3 and the external engines are skipped on LLMs that have [request filters](/docs/filters), unless `lossy_with_request_filters` is set: the filters run after the plugin and would screen the compressed body. Studio 2.3 and later tell the edge which LLMs have request filters. With an older Studio, level 3 is capped at 2 everywhere.

## Configuration

The plugin has one configuration for every LLM it is attached to. Named **policies** hold the compression settings; **bindings** choose a policy per LLM or App.

```json
{
  "mode": "audit",
  "default_policy": "safe",
  "policies": {
    "safe":   {"level": 1},
    "agents": {"level": 2, "min_unit_tokens": 400,
               "tool_overrides": {"Bash": {"max_unit_tokens": 4000},
                                  "read_contract": {"exclude": true}},
               "protect_patterns": ["[A-Z]{2,6}-[0-9]+"]}
  },
  "bindings": {"llm_id:3": "agents", "app:42": "off"}
}
```

### Top-level settings

| Key | Default | Meaning |
|---|---|---|
| `mode` | `audit` | `off`, `audit` or `enforce`. |
| `default_policy` | `default` | The policy for requests no binding matches. `default` means the built-in defaults when no policy of that name exists; `off` turns compression off. |
| `policies` | `{}` | Named policies. Each starts from the defaults and lists only what it changes. `off` is reserved. |
| `bindings` | `{}` | Keys `llm:` plus a slug, `llm_id:` plus an ID, or `app:` plus an ID; values are a policy name or `off`. |
| `allow_client_bypass` | `false` | Honour the request header `X-Tyk-Compression: off`, so a client can skip compression for its own requests. |
| `failure_mode` | `open` | What an external engine failure does: `open` forwards the unit uncompressed, `closed` answers 502 (enforce mode only). Every other failure always forwards the request. |
| `lossy_with_request_filters` | `false` | Run level 3 and engines on LLMs with request filters. |
| `max_body_bytes` | `4194304` | Larger bodies pass through, at most 16 MiB. Limits are by size, never by time. |
| `max_in_flight_bytes` | `16777216` | The body bytes compressed at once, all requests together (at most 256 MiB, at least `max_body_bytes`). A request that would go over it passes through unchanged with reason `over_budget`, so memory stays bounded under load; that request may cost a prompt-cache rewrite. Not editable on the Policies page. |
| `cache_drop_alert` | `0.25` | The dashboard warns when the cache-read share falls by more than this fraction within an hour of a policy change. |
| `engine_NAME_url`, `engine_NAME_token` | | Credentials for an external engine named `NAME`. `$SECRET/` references work. |

### Policy settings

| Key | Default | Meaning |
|---|---|---|
| `level` | `1` | 0 to 3, see [Levels](#levels). |
| `roles` | `["tool"]` | Which messages are compressed. |
| `min_unit_tokens` | `300` | Smaller units are left alone. |
| `protect_recent_turns` | `1` | The newest turns stay at level 1 at most. |
| `cache_mode` | `deterministic` | `deterministic` compresses everything selected, the same way every turn. `skip_cached` leaves everything up to the last cache breakpoint alone. |
| `dedupe_references` | `false` | A tool output identical to an earlier one in the same request becomes a reference to it. |
| `tool_overrides` | | By tool name: `exclude`, or a `level` or `max_unit_tokens` for that tool. |
| `protect_patterns` | | RE2 regular expressions whose matches lossy levels keep. |
| `max_unit_tokens` | `6000` | Level 3's size budget per tool output. |
| `mask_after_turns` | `0` | Level 3 masks tool results older than this many turns; `0` turns masking off. Each masking step rewrites the cached prefix, so it pays where cached input is not much cheaper than input (OpenAI chat models with a 50% cache discount) and costs more than it saves where cache reads are very cheap (Anthropic, GPT-5). |
| `mask_step_tokens` | `20000` | Level 3 moves the masking boundary only when it masks at least this many more tokens. |
| `engine` | | An external engine (`name`, `kind` of `headroom` or `llmlingua2`, `timeout_ms`) for what levels 1 to 3 selected. |
| `drop_tools` | `[]` | Tool definitions to remove from requests, by name or by a prefix ending in `*`, at any level. A token-cost setting, not an access control: see [Dropping tool definitions](#dropping-tool-definitions). |

Token counts are estimates (UTF-8 bytes divided by 4), so they depend only on the text.

An invalid configuration is logged and turns compression off; it never stops the plugin loading.

### Which policy applies

The first match wins:

1. The App's metadata key `prompt_compressor.policy` (needs the `apps.read` scope; read every 30 seconds per App).
2. An `app:` binding.
3. An `llm_id:` binding, then an `llm:` binding.
4. `default_policy`.

Until the plugin knows an App's metadata, that App's requests are forwarded unchanged, because the App may have opted out. Don't use the per-LLM plugin configuration override for bindings: the gateway applies it only when the plugin is attached to exactly one LLM.

### External engines

A level-3 policy can send what it selected to a Headroom (`POST /v1/compress`) or LLMLingua-2 (`POST /compressPrompt`) service. System prompts, tool definitions, protected turns and masked results are never sent. Every answer is remembered by the text sent, the policy and the engine, so a unit compresses to the same bytes on every later turn even if the engine is down or not deterministic. The memory survives a restart through the edge's KV store (7 days), but the compressed text is stored there only when the gateway keeps the LLM's request bodies (`ANALYTICS_STORE_REQUESTS` on the edge, and the LLM not set to `dont_log_bodies`). Otherwise only the decision to leave a unit unchanged is stored, and after a restart a unit goes to the engine again; with an engine that is not deterministic that can cost one prompt-cache miss.

- The URL must be `http` or `https` and must not carry credentials. Put the token in `engine_NAME_token`; it is sent as a bearer token, and only over https or to a loopback host.
- The plugin refuses link-local addresses, including cloud metadata endpoints, after name resolution. Loopback and private addresses are allowed, for sidecars. Proxy settings are ignored and redirects are not followed.
- The **Policies** page cannot change engine credentials. Edit them in the plugin's own configuration form, which needs plugin edit rights.

### Dropping tool definitions

Agents often send the definitions of many tools on every request and call only a few. A policy's `drop_tools` removes the definitions it names (`["legacy_*", "browser_screenshot"]`) from OpenAI Chat Completions, Responses, Anthropic, Google and Bedrock Converse requests, at any level. The dashboard's **Tool definitions** panel lists the tools each App offered but never called, and the **Policies** page suggests them.

- A tool named by a tool call in the conversation, or by `tool_choice`, is kept, and a list that would remove every tool while the conversation holds tool calls is ignored, so a request never becomes invalid.
- The kept definitions, and the rest of the body, are sent byte for byte.
- Changing the list changes the tools every App on the policy sends, so each of their sessions misses the prompt cache once. Bind a separate policy to an App to change only that App.

::: warning drop_tools is not an access control
It saves tokens; it cannot keep a tool from a model. The client decides whether a tool survives: by naming it in its own conversation history or in `tool_choice`, by sending `X-Tyk-Compression: off` where an admin has turned on `allow_client_bypass`, and whenever a request passes through uncompressed (`audit` mode, an App whose metadata is not known yet, a body over `max_body_bytes`, any error). To keep a tool away from a model, don't give the App access to it, or block its calls with a [filter](/docs/filters) or a [guardrail](/docs/guardrails) on tool calls.
:::

## Headers

| Header | Direction | Meaning |
|---|---|---|
| `X-Tyk-Compression: off` | request | Skip compression for this request, when `allow_client_bypass` is on. No header can choose a policy or raise a level. The gateway removes it before the vendor. |
| `X-Tyk-Compression` | response | `applied`, `audit`, `bypassed`, `skipped`, `rejected` or `failed_open`. |
| `X-Tyk-Compression-Tokens-Saved` | response | Estimated tokens removed (or that would be, in audit mode). |
| `X-Tyk-Compression-Policy` | response | The short hash of the policy that applied. |

The response headers are also on streaming responses, and they reach the client through the `/ai/` and unified `/v1` endpoints.

## Studio pages

The plugin adds a **Prompt Compressor** section to the admin sidebar. Each page has its own permission, granted through roles like any plugin permission (see [Manifests & Permissions](/docs/plugins-manifests)).

| Page | Permission | What it does |
|---|---|---|
| Savings | Compression savings: read | The dashboard. |
| Policies | Compression policies: read, write | Edit the mode, the policies and their bindings. Validates before saving and warns that a change makes every affected session miss the prompt cache once. Edge gateways apply the change after the next configuration push. |
| Preview | Compression preview: execute | Paste or upload a request body or a tool output, pick a policy, and see the output with every change marked, each strategy's decision, and which tool definitions `drop_tools` removed or kept and why. Nothing is stored and no engine is called. |

### The savings dashboard

Each edge sends hourly totals per App, LLM, policy and outcome to Studio about once a minute. The records carry counts and IDs only: no prompt or tool text leaves the edge. Studio keeps 90 days.

The dashboard shows, for a range from 24 hours to 90 days and optionally one App or LLM:

- **Tokens saved** (actual in enforce mode, projected in audit mode) and bytes saved.
- **Cost saved at input price.** Saved tokens priced at the model's input price. This is an upper bound: tokens that would have been cache reads were worth less.
- **Cost saved, cache-adjusted.** Saved tokens priced at the cache-read price in proportion to the cache-read share. Real savings lie between the two figures.
- **Cache-read share**: cache reads as a share of prompt tokens, over time, with a marker at every policy change.
- A **cache-drop warning** when the cache-read share falls by more than `cache_drop_alert` within an hour of a policy change.
- **Tool definitions**, per App: the tokens tool definitions take and their share of prompt tokens, how many tool-list versions were sent (each change misses the cache once), the tools offered but never called, and what dropping those would have saved at the input and cache-read price.
- Savings by strategy, requests not compressed by reason, and the latency the plugin added.

## Prompt caching

Vendors bill cached prompt tokens at a fraction of the input price, so a compressor that breaks the cache can cost more than it saves. The plugin is built to avoid that, and the dashboard shows whether it did.

- **Deterministic output.** A unit compresses to the same bytes every time, so the compressed prefix of a conversation is identical from one turn to the next. Engine answers are remembered for the same reason.
- **Policy changes cost one miss.** Changing a policy changes the bytes of every affected conversation's prefix once. Change policies outside busy hours, and watch the cache-read share marker afterwards.
- **`skip_cached`** leaves everything up to the request's last cache breakpoint alone, for clients that set breakpoints and want only the new part compressed.
- **Masking is off by default.** When `mask_after_turns` is set, level 3 moves its masking boundary only when that masks at least `mask_step_tokens` more, so each one-off cache rewrite buys a large saving. With vendors whose cache reads are very cheap (Anthropic, GPT-5) a rewrite still costs more than it saves; leave it off there.
- **Edges and engines.** The engine memo is per edge. If one session's requests go to several edges and the engine is not deterministic, the edges may compress a unit differently.

## Plugin order with the cache plugins

Plugins attached to an LLM run in the order they are listed on the LLM. With the LLM Cache or Advanced LLM Cache plugin, list the cache **before** the Prompt Compressor:

- A cache hit answers before the compressor runs, so hits cost no compression work.
- Cache keys are taken from the body the client sent. With the compressor first, the keys would be taken from the compressed body and so depend on the policy: every policy change would make the response cache miss.
- A semantic cache compares the client's own text, not a compressed version of it.

Requests answered from the cache never reach the compressor, so they do not appear in its savings.

## Scopes

| Scope | Why |
|---|---|
| `kv.readwrite` | Savings rollups and remembered engine answers. |
| `control.send` | Savings records from the edges to Studio. |
| `pricing.read` | Model prices, including the cache-read price, to cost the savings. |
| `apps.read` | The per-App policy in App metadata, and the App picker. **Without it nothing sent through an App is compressed**, and the dashboard says so. |
| `llms.read` | The LLM picker, and the LLM's endpoint, which decides the request format. |

## Evaluation

Level 1 is lossless and is the default policy level, on correctness tests, a cost replay that includes cache misses, and a smoke evaluation. Levels 2 and above are opt-in. They would become a recommended default only when they are not worse than no compression on all of:

- BFCL (function calling), at least 1,000 cases, within 1 point;
- long-context question answering, at least 500 questions, within 2 points of F1;
- SWE-bench Verified, 200 tasks with 3 seeds each, within 5 points, with no more than 10% more turns per task;
- cost, including the effect on caching.

The [evaluation report](https://github.com/TykTechnologies/ai-studio-community-plugins/blob/main/plugins/prompt-compressor/docs/eval-levels-1-2.md) has the offline and end-to-end results:

- **Level 1** passes its gate and stays the default. It cuts session cost by 0.4%, and output is byte-exact end to end.
- **Level 2** cuts the cost of a 30-turn agent session by 11.4%, including cache effects, with no cache breaks. It stays opt-in until the quality suites above have run.
- **Level 3's `mask_after_turns`** pays only on OpenAI Chat pricing.

Before enforcing level 2 or 3, run it in `audit` mode first and compare the dashboard with your own quality signals.
