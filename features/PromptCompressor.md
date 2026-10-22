# Prompt Compressor

Cache-safe, measured compression of LLM request bodies on the Microgateway,
starting with tool outputs. A Community Edition plugin
(`community/plugins/prompt-compressor`, manifest id
`com.tyk.prompt-compressor`), not a core filter kind. User docs:
`docs/site/docs/prompt-compressor.md`. Epic `midsommar-clean-eac`.

## 1. Decisions

- **Plugin only.** The compression library lives in the plugin module.
  There is no core filter kind, and Studio Chat is out of scope (Chat does
  not go through the proxy).
- **All CE.** Engines and every level are in the community plugin.
- **Output is a pure function of the unit text and the policy (R1).**
  Nothing depends on time, load or earlier requests; limits are by size, never
  by time. This is what keeps a compressed prefix byte-identical and the
  vendor's prompt cache hitting. Engine answers are memoised for the same
  reason.
- **Fail open.** `post_auth` never returns an error, recovers panics
  (`failed_open`), and forwards the original bytes on anything it cannot do.
  The only refusal is `failure_mode: closed` with an engine failure in
  enforce mode (502, `failed_closed`).
- **post_auth, not pre_upstream (Phase 1).** `post_auth` runs before request
  filters, so the filters screen the compressed body. The Bedrock bridges run
  no plugins and are not covered until the core `pre_upstream` hook exists
  (eac.28); `post_auth` is deliberately not extended to them (it would leak
  rate-limiter and load-balancer counters and double-run on `/ai/`).
- **Lossy levels are capped on LLMs with request filters** unless
  `lossy_with_request_filters` is set: ANSI and whitespace stripping can join
  text into a match the filter would otherwise not see. An LLM whose filter
  state is unknown (host older than 2.3, or not looked up yet) is treated as
  filtered (eac.64).
- **Client bypass defaults to off** (eac.82, Martin, 2026-10-21; it was on).
  With `allow_client_bypass` set, `X-Tyk-Compression: off` can only turn
  compression (and drop_tools) off; no header picks a policy or raises a
  level.
- **An App's unknown metadata means pass-through.** The App may have opted
  out, so the request is forwarded unchanged (`app_scope_missing` when the
  host refuses `GetApp`, `app_policy_unknown` while pending).
- **Eval gate (eac.4):** level 1 needs correctness tests, a cost replay
  including cache misses, and a smoke eval. Levels 2+ need non-inferiority on
  BFCL (1,000 cases, 1 point), long-context QA (500 questions, 2 points F1),
  SWE-bench Verified (200 tasks × 3 seeds, 5 points, turns +10% at most) and
  cost including cache effects.
- **Preview instead of live samples.** Live before/after samples are later
  work (eac.22) behind a per-edge switch; the plugin never ships text off the
  edge.

## 2. Request path (edge)

```
client ─▶ edge /llm/{rest,stream,call}
            post_auth chain (LLM plugin order)
              └─ prompt-compressor HandlePostAuth
                   resolve policy: App metadata (GetApp, 30 s cache, 25 ms wait)
                                   → app: → llm_id: → llm: → default_policy
                   request format: pkg/requestformat (vendor, path, LLM endpoint)
                   compress.Compress(format, body, policy, caps)
                     guardrails/segments.ExtractUnits → select → rewrite leaves
                     → segments.Apply → post-checks (pairing, never longer)
                   audit: forward original; enforce: forward compressed
                   pending[requestID] = what it learned (300 s, 8,192 max)
            request filters ─▶ vendor
          on_response
            usage incl. cache-read/write tokens (buffered, SSE, Bedrock event stream)
            X-Tyk-Compression, -Tokens-Saved, -Policy (streaming too: eac.14)
            records.Aggregator.Add
```

- Files: `postauth.go`, `policy.go`, `lookup.go`, `llminfo.go`,
  `pending.go`, `response.go`, `usage.go`, `compress/`, `engine/`.
- Formats: `openai`, `openai_responses` (eac.9), `anthropic`, `google_ai`,
  `vertex`, `bedrock` (Converse), `ollama`. Anything else passes through.
- Never rewritten: vendor server-tool results, tool call ids, images,
  thinking/reasoning, signatures, `cache_control`/`cachePoint` (not leaves),
  and anything `segments.Generic` read. Text never moves across roles.
- Native context management in the request caps the level at 1.
- Engines (`engine/`): Headroom and LLMLingua-2 over HTTP, one call per leaf,
  8 in flight, per-call timeout, egress checks after resolution
  (link-local and metadata refused, loopback and private allowed, no proxy,
  no redirects), bearer token only over https or loopback. Answers memoised
  in process and in edge KV (7 days), keyed by text, policy hash and engine.
- Engine credentials are top-level keys (`engine_<name>_url`,
  `engine_<name>_token`) so the edge resolves `$SECRET/` references
  (eac.37).

## 3. Savings records (edge → Studio)

```
records.Aggregator (hourly Delta per App, LLM, policy hash, outcome)
  └─ every minute and on shutdown: SendToControlJSON
       (metadata metric_type: prompt_compressor_savings)
Studio AcceptEdgePayload ─▶ records.Store (plugin KV)
  per edge and hour document + edges index + hours index
  batch ids per hour document → redelivery is a no-op
```

- Counts and ids only: no prompt or tool text, no header values, no model
  name (it is used to price at the edge and dropped).
- Prices: saved tokens at the input price (`cost_saved`, an upper bound) and
  at the cache-read price (`cost_saved_at_cache_read`), from
  `GetModelPrice` with `pricing.read`.
- Store bounds (eac.49): hours more than one ahead refused; per edge and
  hour 5,000 rows and 4 MB, then an overflow row; 64 strategies, 8
  currencies, 16 reasons per row, then "other"; 500 policy markers; 90-day
  retention. The edge aggregator keeps its own key bound across failed sends
  (eac.55).
- Query (eac.61): `QueryWith` reads newest-first in buckets (1 h up to 72 h,
  6 h up to 30 days, 24 h beyond) with a byte cap that reports a partial
  view and a row cap. `from` is aligned down to the bucket. Each dashboard
  view is cached 30 s with singleflight. The cache key carries no caller
  identity, which is safe only while `savings:read` sees every App.

## 4. Studio UI

Web components in `ui/webc/` (shadow DOM; text through `textContent` only;
links via `pushState` + `popstate`). RPCs go through `HandleRPC` and the
manifest's `rbac.rpc_methods`, which the host enforces
(`pluginRPCPermission`).

| Page | Component | RPCs | Permission |
|---|---|---|---|
| Savings | `dashboard.js` | `getSavings`, `getStatus` | `savings:read` |
| Policies | `policies.js` | `getEditor`, `validateConfig`, `saveConfig`, `getStatus` | `policies:read`, `policies:write` |
| Preview | `preview.js` | `getPreviewOptions`, `preview` | `preview:execute` |

- `saveConfig` (`editor.go`, `studio_api.go`) owns only the policy keys
  (`mode`, `default_policy`, `policies`, `bindings`, `allow_client_bypass`,
  `failure_mode`, `lossy_with_request_filters`, `max_body_bytes`). A save that
  changes, adds or removes any other key is refused (eac.45), so
  `policies:write` cannot repoint an engine URL while keeping its token.
  Stored tokens are masked.
- LLM bindings use `llm_id:`; the page marks LLMs with request filters from
  `LLMInfo.has_request_filters` (eac.42; optional field, absent means
  unknown, eac.64).
- Alerts: a cache-drop banner when the cache-read share falls by more than
  `cache_drop_alert` within an hour of a policy marker, judged on complete
  hours with at least 10 requests per window only.
- Preview (`preview.go`): the same `compress` call as the edge with
  `Trace: true`, so output is byte-identical; 2 MiB cap before parsing; no
  engine is called; inputs are not stored or logged. Traces carry unit text,
  so they never go into records or logs.

## 4a. Tool definitions (eac.39, eac.40)

- Edge records carry per request the tool definitions offered (count,
  bytes/4 tokens, list hash, per-tool tokens) and the tool names the model
  called; only names the request offered count as called (eac.71), others
  fold into "other". Names are shape-checked and capped (64 per row, 8
  list versions). `getSavings` returns a per-App `tools` section: definition
  tokens and share, never-called tools and the estimated saving at input
  and cache-read price. Dashboard panel `pc-tools`.
- A policy's `drop_tools` (names or `prefix*`, at most 256) removes those
  definitions by byte span. Kept regardless: tools named by a tool call in
  the history or by `tool_choice`, and the whole array when the list would
  empty it while the history holds tool calls. The preview reports each
  tool's decision; the Policies page suggests never-called tools and warns
  about the one-off cache miss.
- **Not an access control (eac.75).** The client writes the history and
  `tool_choice`, can send the bypass header where `allow_client_bypass` is
  on, and every pass-through keeps the full list. The docs page, the
  README and the Policies page say so and point to App tool access, filters
  and guardrails. `allow_client_bypass` defaults to false (eac.82, Martin's
  decision); a stored config with an explicit true keeps it.

## 5. Core changes made for this plugin

- `pkg/requestformat`: exported request-format resolver (eac.8).
- `guardrails/segments`: OpenAI Responses (eac.9), pairing check, cache
  breakpoints and native compaction detection (eac.7).
- Response hooks on streaming responses and the real LLM slug (eac.14);
  `/ai/` loopback relays `X-Tyk-Compression*` headers.
- `X-Tyk-Compression` request header stripped at the upstream hop (eac.46).
- Multi-value request and response headers kept through plugin hooks
  (eac.26, eac.51).
- Plugin gRPC message limit raised to 64 MiB, edge `/llm/` bodies capped
  (eac.36, eac.43, eac.44).
- `$SECRET/` resolution in edge plugin configs (eac.37, eac.52).
- `LLMInfo.slug` and `has_request_filters` in the management API, on Studio
  and the edge (eac.42, eac.27).
- Host-service broker re-offered until the plugin connects, on the edge
  (eac.59) and Studio (eac.62).

## 6. Plugin order and interactions (8xf.26, 2026-10-22)

**How the chain runs.** All of an LLM's plugins run in one order (the
LLM's ordered plugin list: Studio's LLM form, `PUT /llms/{id}/plugins`,
`llm_plugins.order_index`). `pre_auth` plugins run before any `post_auth`
plugin. Each `post_auth` plugin gets the body the previous one returned, so
plugins after the compressor see the compressed body and plugins before it
see the client's body. A `post_auth` block (a cache hit, a 429, a firewall
block, `failed_closed`) answers at once: later plugins, request filters and
every response hook are skipped for that request. Response hooks run in the
same order, not reversed, and every plugin's `OnBeforeWriteHeaders` runs
before any `OnBeforeWrite` (`proxy/proxy.go`), so a header the compressor
sets is in every plugin's `OnBeforeWrite` whatever the order. There is no
per-App order and no other ordering control. On `/ai/`, `/v1/`, routers and
`/anthropic/` the chain runs once per attempt on the `/llm/call/` inner hop,
on the vendor-native body (Converse for Bedrock after 8xf.12). Each failover
rung is its own inner hop with its own request ID, so every plugin runs once
per rung.

**Known gap: hub-spoke edges ignore the order.** A standalone
microgateway honours `order_index`. An edge fed by Studio runs an LLM's
plugins in plugin-ID (install) order, because the snapshot carries no
per-LLM order (bug `midsommar-clean-701`). Until that is fixed, the order
below holds on edges only if the plugins were installed in that order.

**Recommended `post_auth` order:** LLM Firewall (if run in `post_auth`) →
response caches → Rate Limiter → Prompt Compressor → LLM Load Balancer.

- **Caches before the compressor.** A hit answers in `post_auth` and the
  compressor never runs. Keys come from the client's body, so a policy
  change does not empty the cache and the semantic layer embeds what the
  client wrote. Cache hits are not in the compressor's savings.
  - Consequence, accepted: an entry stored while a policy was enforcing
    holds an answer the model gave to the compressed prompt, and it is
    served to the same client body after the policy changes, or to a client
    that sent `X-Tyk-Compression: off` (the bypass skips compression, not
    the cache). Compression is meant to leave answers equivalent, and
    entries expire with their TTL. A client that wants a fresh,
    uncompressed answer sends both bypasses (`X-Cache: bypass` works for
    both caches).
  - With the compressor first instead: keys are taken from the compressed
    body. That is still deterministic (R1), so identical requests under one
    policy hit. A policy or level change, or a client bypass, misses. Every
    hit still pays for compression (cheap for repeats, since engine answers
    are memoised), and each hit leaves a pending record the response hooks
    never take. That record ages out (300 s, 8,192 entries, oldest first),
    so a very high hit rate can push out the record of a long stream.
  - A cache must not store the headers that describe one request:
    `X-Tyk-Compression*` (always present by `OnBeforeWrite`), `X-RateLimit-*`,
    `X-LB-*`, `Server-Timing` and request IDs. None of those plugins runs
    for a hit. Both caches drop them in `storableHeaders`: Advanced LLM
    Cache 1.3.0 when storing and again when replaying (w3m), LLM Cache
    1.3.0 likewise (d8r).
- **Rate Limiter before the compressor.** It does not read the body (keys
  are App, user, LLM, API key). It has no pre-request token estimate: token
  rules add the vendor's reported usage once per request, from the response
  hooks. That usage is what the vendor counted for the compressed body, so
  compression is reflected whatever the order. Cache reads count where the
  vendor's total includes them (OpenAI, Google, Bedrock) and not where
  there is no total (Anthropic: input + output). Putting it first means a
  request refused with 429 is never compressed. Cache hits before it are
  not rate limited at all. A request answered before the vendor (a block
  later in the chain, a request filter) never runs the response hooks that
  give back its concurrent slots. From 1.2.0 the plugin keeps an in-process
  record of each request's slots and a sweep releases those of requests
  older than 10 minutes; its own 429 or 503 releases the slots earlier
  rules took at once (1g2). The record is per process: a plugin restart
  forgets it (8xf.33), and with a shared Redis each gateway releases only
  its own requests' slots.
- **LLM Firewall screens the client's body.** By default it runs in
  `pre_auth`, before every `post_auth` plugin, so compression can neither
  hide a phrase (levels 2 and 3 drop lines, cut head and tail, mask turns)
  nor create one. Level 1 strips ANSI escapes and carriage-return frames,
  which can join a phrase split by them in the forwarded body; the firewall
  does not see that form. Request filters and guardrails screen the forwarded body (they
  run after `post_auth`), which is why lossy levels are capped on LLMs with
  request filters. A firewall in `post_auth` must come before the
  compressor. From 1.2.0 it checks only in the configured `hook_phase`
  (before, it checked in both phases: ayb).
- **LLM Load Balancer last.** It never blocks and never changes the body;
  it sets `upstream_override`. Token-aware routing estimates tokens from
  the body it sees, so after the compressor it estimates what the upstream
  will receive (before it, it overestimates, which is safe). Last also
  means any block happens before it counts an active connection. A request
  blocked after it, by a request filter, holds that count until the
  5-minute stale sweep. It retries nothing itself: gateway failover re-runs
  the chain per rung on `/ai/`.

**When the compressor moves to `pre_upstream` (eac.28/eac.29)**, it runs
after the whole `post_auth` chain and the request filters. Every plugin
above then sees the client's body whatever the order, and caches always key
on it. The caches still need their header filtering, and the Load
Balancer then always overestimates.

## 6a. Editions

Runs on Microgateway instances in both CE and Enterprise (CE edges use the
single `default` namespace and the push works). Studio's embedded gateway
wires no `post_auth` hooks (only `microgateway/internal/api/auth_hooks.go`
does), so Studio-only deployments compress nothing. The edge-gateways page's
Enterprise-only note is over-broad: bead `midsommar-clean-utx`.

## 7. Open work

- eac.24 / eac.31: eval reports (levels 1–2, then level 3 and engines). The
  docs page's Evaluation section waits for them.
- eac.28 / eac.29: `pre_upstream` hook and the plugin's switch to it
  (Bedrock bridges).
- eac.39 / eac.40: tool-schema slimming, measured, then a per-App drop list.
- eac.22: opt-in live samples. eac.32: reversible compression.
- eac.41: rebuild request-path plugins on the 2.3 SDK.
