# Agent Call Tracing (Enterprise)

Agent Call Tracing shows what an AI agent did, step by step. It is built from the traffic that already passes through the gateway: every LLM call, tool call, MCP call and datasource query. The gateway groups them into **sessions** (one conversation or job), **turns** (one user message or trigger and everything it caused) and **spans** (one call each).

You can then:

- read a session as a transcript or a timeline;
- export it as JSON or Markdown;
- send it to any OpenTelemetry (OTLP) backend, such as Langfuse, Datadog LLM Observability, Grafana Tempo or Honeycomb.

The agent needs no SDK. Declaring a session and an agent name with a few request headers makes the traces sharper; see [Getting better traces](#getting-better-traces).

Available in **Enterprise Edition** only.

- **Community Edition** answers the tracing API with `402` and records nothing. Two propagation controls still work there: the baggage allowlist and the per-LLM trace context opt-out.
- **Enterprise Edition with tracing not started** (not enabled, or no key) answers `503`.

---

## How it works

1. **Capture.** For each call, the gateway records one event: the timing, App, user, model or tool, usage and cost, and the outcome of each gateway decision (filters, budgets, fallbacks, routing). If the content mode allows it, the event also holds the request and response content.
   - Content is processed on the gateway that saw it, before it is stored or sent anywhere: secrets stripped, redaction run, size capped.
   - This works the same in the gateway embedded in Studio and in edge gateways.
2. **Transport.**
   - The embedded gateway writes events to a staging table.
   - Edge gateways send them to Studio in their analytics pulse, every 10 seconds, in chunks of at most 4 MB.
   - Events are encrypted at rest from the moment they are staged.
3. **Assembly.** One Studio replica, the holder of the `agent_trace_assembler` lease, turns staged events into sessions, turns and spans, after a 30-second settle window. It also links calls that belong together (see [Correlation and confidence](#correlation-and-confidence)) and closes idle turns and sessions.
4. **Reading and export.** Admins see sessions under **Agent Sessions** in the admin UI. Optionally, the exporter sends finished turns over OTLP.

### What the gateway can and cannot see

The gateway sees what passes through it. Coverage depends on where your tools run.

| Where the work happens | What you get |
|---|---|
| LLM calls through `/llm/rest`, `/llm/stream`, `/ai/` (with each provider attempt), native API dispatch and Bedrock | Full LLM spans: model, tokens, cost, finish reasons, tool calls requested, and gateway decisions. |
| Tools, MCP servers and datasources served by Studio (`/tools/...`, the MCP shim, datasource queries) | Tool and datasource spans, linked to the LLM call that asked for them. |
| Edge plugin endpoints (`/plugins/...`) | A span for each call. The body is not parsed. |
| **Client-side tools**: tools the agent runs itself, such as shell, file edits or local MCP servers | **Inferred.** The gateway sees the model ask for the tool and later sees the result come back in the next request. It records an `inferred_client_tool` span between the two. Its duration is an upper bound (`tyk.duration.upper_bound`). |
| **MCP servers behind a Tyk Gateway** that the agent calls directly | Inferred as client-side tools from the LLM traffic. Execution details from the Tyk Gateway are planned through the Tyk Pump integration. |
| **Studio Chat and agent plugins** | Not traced yet (Phase 2). |
| Calls that bypass the gateway | Not seen. |

Providers whose tool calls carry no ids (some Gemini and Ollama responses) cannot be linked to their results.

---

## Turning it on

Set these on Studio and restart:

```bash
AGENT_TRACING_ENABLED=true
# Root key: 16 bytes or more. If unset, TYK_AI_SECRET_KEY is used.
# With neither set, tracing does not start.
AGENT_TRACING_KEY=env-specific-long-random-value
```

Once tracing runs, the **Agent Sessions** page appears in the admin UI. Edge gateways start capturing when all of these hold:

- they run an Enterprise build;
- they have a valid licence;
- they run in hub-connected mode;
- their analytics pulse is loaded;
- `AGENT_TRACING_ENABLED=true` is set on the edge;
- Studio has tracing on (its configuration sync carries the content settings). While it does not, the edge captures nothing.

An edge sends captured content only over a verified TLS connection to Studio. With `EDGE_TLS_ENABLED=false` or `EDGE_SKIP_TLS_VERIFY=true` it sends metadata only: its spans show their content as dropped, with the attribute `tyk.content.withheld=edge_link_unverified_tls`, and the edge logs a warning at start. Enable verified TLS on the edge-to-Studio connection to capture content from edges.

Edges receive the content settings and the fingerprint key from Studio in their configuration sync. Studio sends the key encrypted with `MICROGATEWAY_ENCRYPTION_KEY`, and the edge holds it in memory only.

::: warning Edge trust
All edges share one token (`EDGE_AUTH_TOKEN`, matching Studio's `GRPC_AUTH_TOKEN`). Anyone holding it can register as any edge ID and namespace, and report captures and analytics for that namespace's Apps. Those captures then appear in Agent Sessions as that namespace's traffic. Treat the edge token as a secret for the whole hub: see [the edge token](/docs/edge-gateways#the-edge-token-is-a-hub-wide-secret). Per-edge credentials are planned separately.
:::

### Keys and rotation

The root key is never used directly. Studio derives two keys from it with HKDF-SHA256:

- an HMAC key for fingerprints and the correlation index;
- an AES-256-GCM key that seals captured content.

To rotate:

1. Set the new value in `AGENT_TRACING_KEY`.
2. Move the old one to `AGENT_TRACING_KEY_PREVIOUS`.

Data sealed under either key stays readable. Nothing re-encrypts old content, so keep the previous key until payload retention has passed. Only one previous key is supported.

### Permissions

| Permission | Allows |
|---|---|
| `traces:read` | Agent Sessions: lists, transcripts and timelines (metadata), search over names and metadata |
| `trace-content:read` | Reading captured content: payloads, previews, content search and exports with content. Marked sensitive. |
| `traces:execute` | Exports |
| `traces:delete` | Purging a session, erasing an end user |
| `traces:write` | Changing any tracing setting on an App, LLM, tool or datasource |

The default Viewer, Editor and Auditor roles do not include `trace-content:read`. Grant it deliberately.

### Content access needs the audit trail

Every read of captured content is written to the [Audit Trail](/docs/audit-trail) **before** the content is returned. This covers payloads, previews, content search and exports, even exports without content. Purges are recorded the same way.

If the audit trail is disabled (`AUDIT_ENABLED=false`) or cannot accept a record:

- content reads, exports and purges answer `503` ("content access requires the audit trail");
- transcripts show metadata only, with a notice.

Two further rules apply:

- The audit middleware never stores request or response bodies of the tracing routes, even with detailed recording on.
- An end-user erasure is recorded by session ids and a keyed HMAC of the end user, never the plain value.

---

## Content capture

By default the gateway captures **no content**, only metadata: timing, tokens, cost, models, tool names and gateway decisions.

### Content modes

| Mode | What is stored for prompts, responses, tool arguments and results |
|---|---|
| `off` | Nothing. Error messages are kept, redacted and capped at 2 KB. |
| `metadata` | Structure only. For messages: roles and sizes. For tool arguments: the key names, with each value's type and size. For tool results: type and size. |
| `hashed` | Phase 2. Behaves as `metadata`. |
| `redacted` | The content, with secrets and identifiers stripped and the PII detectors applied (`[REDACTED:EMAIL]` and so on). |
| `full` | The content, with secrets and identifiers stripped. No PII detection. |

Secrets are stripped in every mode that keeps content. Field names such as `authorization`, `api_key`, `password`, `cookie`, `client_secret`, `private_key` and token fields are matched regardless of case or separators, including in nested JSON, `key=value` pairs and base64-encoded values.

Binary content is replaced by a reference unless `AGENT_TRACING_PAYLOAD_STORE_BINARY=true`.

### How the mode for a call is decided

1. Start from the stricter of the global default (`AGENT_TRACING_CONTENT_DEFAULT_MODE`) and the App's content mode. An App can only lower the global default.
2. A request may ask for a different mode with the `X-Tyk-Trace-Capture` header.
   - It can always **lower** the mode.
   - It can **raise** it only up to the request ceiling: the global `AGENT_TRACING_CONTENT_MAX_REQUEST_OVERRIDE`, or the App's ceiling if that is lower.
3. Cap the result by the resource's maximum: the LLM, tool or datasource **Maximum content mode**.
4. Cap that by `AGENT_TRACING_STORE_CONTENT_MODE_CAP`.

A call that has no App, or whose settings cannot be read, captures no content.

### The content-mode matrix

These are the effective modes with the default global settings: default `off`, request ceiling `metadata`, store cap `full`.

| App content mode | Request header | Resource maximum | Stored as |
|---|---|---|---|
| inherit | none | none | `off` |
| inherit | `full` | none | `metadata` (the ceiling) |
| `redacted` (with the global default raised to `redacted`) | none | none | `redacted` |
| `redacted` (global default `redacted`) | `off` | none | `off` (a request can always lower) |
| `full` (global default `full`) | none | `metadata` | `metadata` (the resource cap) |
| `full` (global default `full`, store cap `redacted`) | none | none | `redacted` (the store cap) |

### Size limits

| Limit | Default | Variable |
|---|---|---|
| Per content part | 256 KiB | `AGENT_TRACING_PAYLOAD_MAX_PART_BYTES` (minimum 512) |
| Per span | 1 MiB | `AGENT_TRACING_PAYLOAD_MAX_SPAN_BYTES` |
| Shown in the UI | 2 MiB | `AGENT_TRACING_UI_MAX_PAYLOAD_VIEW_BYTES` |

When a part is truncated, its JSON stays valid and the cut is marked `…[truncated N bytes]`.

Stored content is compressed and sealed with AES-256-GCM. Each payload is bound to its App, span and part, so it cannot be moved between them.

---

## Configuration reference

All variables are read at start-up. An invalid value keeps the default and logs a warning. Lists are comma-separated, and `none` means an empty list.

### Studio (hub)

| Variable | Default | Purpose |
|---|---|---|
| `AGENT_TRACING_ENABLED` | `false` | Turns tracing on |
| `AGENT_TRACING_KEY` | `TYK_AI_SECRET_KEY` | Root key, 16 bytes or more |
| `AGENT_TRACING_KEY_PREVIOUS` | empty | The previous root key, during a rotation |
| `AGENT_TRACING_QUEUE_MAX_EVENTS` | `10000` | In-memory capture queue, events |
| `AGENT_TRACING_QUEUE_MAX_BYTES` | 64 MiB | In-memory capture queue, bytes |
| `AGENT_TRACING_CONTENT_DEFAULT_MODE` | `off` | Global content mode |
| `AGENT_TRACING_CONTENT_MAX_REQUEST_OVERRIDE` | `metadata` | Highest mode `X-Tyk-Trace-Capture` may ask for |
| `AGENT_TRACING_STORE_CONTENT_MODE_CAP` | `full` | Highest mode ever stored |
| `AGENT_TRACING_REDACTION_DETECTORS` | `email,phone,payment_card,iban,us_ssn,jwt,bearer_token,api_key,private_key` | PII and secret detectors for `redacted` |
| `AGENT_TRACING_PAYLOAD_MAX_PART_BYTES` | 256 KiB | See [Size limits](#size-limits) |
| `AGENT_TRACING_PAYLOAD_MAX_SPAN_BYTES` | 1 MiB | |
| `AGENT_TRACING_PAYLOAD_STORE_BINARY` | `false` | Keep binary content instead of a reference |
| `AGENT_TRACING_MIN_LINK_CONFIDENCE` | `weak` | Weakest confidence used as a parent edge |
| `AGENT_TRACING_TOOL_MATCH_WINDOW` | `300s` | How long after a model asks for a tool its execution can match |
| `AGENT_TRACING_CLOCK_SKEW_TOLERANCE` | `2s` | Allowed clock difference between gateways |
| `AGENT_TRACING_TURN_IDLE_TIMEOUT` | `5m` | A turn closes after this long without activity |
| `AGENT_TRACING_SESSION_IDLE_TIMEOUT` | `30m` | A session closes after this long without activity (per-App override) |
| `AGENT_TRACING_TOOL_NAME_PREFIX_STRIP` | `mcp__{server}__,{server}__,{server}.` | Tool-name prefixes ignored when matching |
| `AGENT_TRACING_FORWARD_TYK_HEADERS` | `false` | Forward the `X-Tyk-*` tracing headers upstream |
| `AGENT_TRACING_BAGGAGE_ALLOWLIST` | `session.id` | W3C baggage keys forwarded upstream; `none` forwards none. Applies in every edition. |
| `AGENT_TRACING_METADATA_RETENTION_DAYS` | `30` | How long sessions, turns and spans are kept |
| `AGENT_TRACING_PAYLOAD_RETENTION_DAYS` | `7` | How long content is kept; never longer than metadata |
| `AGENT_TRACING_CORRELATION_KEY_TTL` | `24h` | Lifetime of correlation index entries |
| `AGENT_TRACING_STORE_SAMPLE_RATE` | `1.0` | Share of sessions stored, decided per session |
| `AGENT_TRACING_EXPORTERS_FILE` | empty | OTel exporters file; empty means no export |
| `AGENT_TRACING_EXPORT_SEMCONV_PROFILE` | `genai-1.41` | Attribute mapping profile, or a path to a mapping YAML. An unknown profile name falls back to the default with a warning; an unreadable file turns export off. |
| `AGENT_TRACING_EXPORT_AGENT_SPAN_MODE` | `tyk` | `semconv` names declared agents `invoke_agent {name}` |
| `AGENT_TRACING_EXPORT_GATEWAY_EVENTS` | `events` | `spans` exports gateway decisions as child spans instead of span events |
| `AGENT_TRACING_EXPORT_CONTENT` | `none` | Phase 2: `reference` and `inline` are not built yet. They log a warning and export metadata only. |
| `AGENT_TRACING_EXPORT_MAX_ATTRIBUTE_BYTES` | `16384` | Longest exported attribute value |
| `AGENT_TRACING_EXPORT_MAX_DELAY` | `30s` | Longest a finished turn waits before export |
| `AGENT_TRACING_EXPORT_INFERRED_SESSION_AS_SESSION_ID` | `true` | Put inferred sessions' ids in `session.id` |
| `AGENT_TRACING_EXPORT_CONVERSATION_ID_FALLBACK` | `never` | `inferred` also fills `gen_ai.conversation.id` from inferred sessions |
| `AGENT_TRACING_EXPORT_USER_ID_MODE` | `hashed` | How the end user is sent as `user.id`: `off`, `hashed` (a keyed pseudonym) or `raw`; see [Privacy](#privacy-and-erasure). The earlier `AGENT_TRACING_EXPORT_INCLUDE_USER_ID` still applies when this is unset: `true` means `raw`, `false` means `off`. |
| `AGENT_TRACING_METRICS_INCLUDE_APP_ID` | `true` | Add `tyk.app.id` to the metrics |
| `AGENT_TRACING_METRICS_EXPORT_INTERVAL` | `30s` | OTLP metric push interval |
| `AGENT_TRACING_UI_MAX_PAYLOAD_VIEW_BYTES` | 2 MiB | Largest content shown in the UI |

### Edge gateways

Edges read only the variables below. Everything else comes from Studio.

| Variable | Default | Purpose |
|---|---|---|
| `AGENT_TRACING_ENABLED` | `false` | Turns capture on |
| `AGENT_TRACING_QUEUE_MAX_EVENTS`, `AGENT_TRACING_QUEUE_MAX_BYTES` | as on Studio | Capture queue |
| `AGENT_TRACING_FLUSH_INTERVAL` | `10s` | How often captures go to Studio |
| `AGENT_TRACING_FLUSH_MAX_EVENTS` | `500` | Events per flush |
| `AGENT_TRACING_CHUNK_MAX_BYTES` | 4 MiB | Largest chunk per pulse; values over 8 MiB fall back to the default |
| `AGENT_TRACING_BAGGAGE_ALLOWLIST` | `session.id` | As on Studio |
| `AGENT_TRACING_KEEP_LIVE_SPAN` | `false` | Keep this edge's own OTel span for captured calls while Studio exports assembled spans; see [How export runs](#opentelemetry-export) |
| `AGENT_TRACING_COMPAT_HEADERS`, `AGENT_TRACING_HARNESS_SESSION_HEADERS` | as on Studio | Declared-context sources the edge reads |
| `AGENT_TRACING_PROPAGATE_LLM_PROVIDERS`, `AGENT_TRACING_PROPAGATE_HTTP_UPSTREAMS`, `AGENT_TRACING_PROPAGATE_MCP` | `true` | Outbound trace context, as on Studio |
| `AGENT_TRACING_OPENAI_CLIENT_REQUEST_ID` | `true` | As on Studio |
| `AGENT_TRACING_MAX_STREAM_BUFFER_BYTES` | 1 MiB | As on Studio |

### Per App

Open an App and expand **Tracing**. Any field left on *Inherit* uses the global value.

| Setting | Meaning |
|---|---|
| Tracing | *Off* stops capture for the App entirely: nothing of its traffic is recorded. |
| Content mode | The App's content mode; it can only lower the global default |
| Highest mode a request may ask for | The App's request ceiling; it can only lower the global one |
| Session idle timeout | 1m to 168h |
| Keep captured content (days) | 0 to 365, never longer than metadata retention |
| Let the App's owner view captured content in the Portal | Takes effect with the Portal trace views (Phase 2) |

### Per LLM, tool and datasource

Each LLM, tool and datasource has a **Tracing** section:

- **Maximum content mode** caps what is captured of calls to it, whatever the App asks for.
- LLMs also have **Don't send trace context to this provider**, which stops `traceparent`, `tracestate` and baggage going to that LLM. This works in every edition.

Changing any tracing setting needs `traces:write`. Without it the sections are read-only.

---

## Sessions, turns and spans

### Sessions

- **Declared:** the client names the session with a header (see [Getting better traces](#getting-better-traces)). Every call with the same id in the same App joins it.
- **Inferred:** without a declared id, calls are grouped by the App's credential, or else by the user. This is one rolling session per credential, ending after the session idle timeout. Concurrent agents on the same App credential share it, so declare sessions when several agents share an App.

A session closes after its idle timeout, and a later call reopens it.

### Turns

A turn starts when the model is called without tool results: a new user message or trigger. It continues through the calls that bring tool results back, and ends when the model answers without asking for tools.

| Turn status | Meaning |
|---|---|
| `ok` | The model answered without asking for more tools |
| `error` | The final call failed |
| `abandoned` | The model asked for tools and no result came back before the turn idle timeout |
| `in_progress` | Still open |

If the client sends a W3C `traceparent`, its trace id is the turn and is never split.

### Correlation and confidence

The gateway links calls that belong together and records how sure it is.

| Confidence | Meaning |
|---|---|
| `declared` | The client said so: `traceparent`, or `_meta.traceparent` on an MCP call |
| `exact` | Matched by id: a `tool_call_id` coming back in the next request, a `previous_response_id`, or a gateway retry or fallback |
| `strong` | A tool execution matched to the call that requested it by name and arguments, with a single candidate |
| `weak` | Name-only matches, several candidates (the nearest is chosen and the others kept as *candidate* links), or matches across a key rotation |

Each link also records its rule, for example `tool_call_id`, `mcp_name_args` or `mcp_name_only`.

- A parent below `AGENT_TRACING_MIN_LINK_CONFIDENCE` is kept as a candidate link instead of a parent.
- The session shows its weakest link in the **Correlation quality** bar.

---

## Getting better traces

The gateway works without any help, but a few headers let it group calls exactly instead of inferring.

::: warning
Wiring of the headers below into live traffic is in progress (jyx.113). Until it lands, only `traceparent`, `tracestate` and MCP `_meta.traceparent` are read.
:::

### Recommended headers

| Header | Sets | Limits |
|---|---|---|
| `traceparent`, `tracestate` | Trace id and parent span (W3C). The trace is one turn and never split. | W3C format |
| `X-Tyk-Session-Id` | The session | Printable ASCII, 200 characters |
| `X-Tyk-Session-Name` | A display name for the session | 120 characters |
| `X-Tyk-Agent-Name` | The agent making the call | 120 characters |
| `X-Tyk-Agent-Path` | The agent's place in a hierarchy, such as `/planner/researcher` | Starts with `/`, up to 16 segments of 120 characters, 512 bytes |
| `X-Tyk-Agent-Id` | A stable agent id | ASCII, 200 |
| `X-Tyk-User-Id` | The end user the agent acts for | 200 characters |
| `X-Tyk-Trace-Id`, `X-Tyk-Span-Id`, `X-Tyk-Parent-Span-Id` | Trace context when you cannot send `traceparent` | 32 and 16 lowercase hex |
| `X-Tyk-Trace-Capture` | Ask for a content mode for this request | `off`, `metadata`, `hashed`, `redacted`, `full`, within the ceiling |

A header sent twice, or with an invalid value, is ignored.

By default, the `X-Tyk-*` headers are removed before the request goes upstream.

### Compatible headers

Clients already instrumented for other gateways need no changes:

| Family | Headers |
|---|---|
| Helicone | `Helicone-Session-Id`, `Helicone-Session-Name`, `Helicone-Session-Path` (agent path) |
| Portkey | `X-Portkey-Trace-Id`, `X-Portkey-Span-Id`, `X-Portkey-Parent-Span-Id` (non-W3C ids are hashed; the original is kept as `tyk.external.trace_id`) |
| OpenRouter / OpenCode | `X-Session-Id` |
| Bifrost | `X-Bf-Session-Id` |
| Agent Router | `Agent-Session-Id` |
| Amazon Bedrock AgentCore | `X-Amzn-Bedrock-Agentcore-Runtime-Session-Id` |
| Cloudflare AI Gateway | `Cf-Aig-Otel-Trace-Id`, `Cf-Aig-Otel-Parent-Span-Id` |

The gateway also recognises these:

- **W3C baggage keys:** `session.id`, `gen_ai.conversation.id`, `user.id`, `gen_ai.agent.name`, and `tyk.agent.parent` (which builds an agent path).
- **Request body fields:** `conversation`, `metadata.session_id` and `user`.
- **Coding agents**, by their own session headers:
  - Claude Code 2.1.0 and later;
  - Codex CLI 0.160.0 and later;
  - OpenCode 1.18.0 and later.

### Precedence

- **Trace id and parent span:** the first of these that is present wins:
  1. MCP `_meta.traceparent` (MCP calls only)
  2. `traceparent`
  3. `X-Tyk-Trace-Id` / `X-Tyk-Parent-Span-Id`
  4. Portkey
  5. Cloudflare
- **Session:**
  1. `X-Tyk-Session-Id`
  2. compatible headers
  3. baggage
  4. body fields
  5. coding-agent headers

  Explicit headers win over baggage, because baggage travels ambiently across services and must not override a sub-agent's own header.
- **Session name, end user, agent name and agent path:** the `X-Tyk-*` header first, then the compatible header, then baggage, then the body.

### Example

```bash
curl https://studio.example.com/llm/rest/my-llm/v1/chat/completions \
  -H "Authorization: Bearer $APP_KEY" \
  -H "traceparent: 00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01" \
  -H "X-Tyk-Session-Id: ticket-48213" \
  -H "X-Tyk-Agent-Path: /triage/researcher" \
  -d '{"model": "gpt-4o", "messages": [...]}'
```

---

## Reading traces

**Agent Sessions** in the admin UI lists sessions with filters for:

- time, App, user, status and source;
- models, tools and agents;
- errors, cost, tokens and duration.

A session opens as:

- **Transcript:** the conversation, with tool calls and results inline, and gateway decisions as chips.
- **Timeline:** every span on a time axis, with each provider attempt.

The find bar searches names and metadata. With `trace-content:read` it also searches captured content, and that search is audited.

Captured text renders as plain Markdown:

- images are never loaded;
- raw HTML is never rendered;
- links show their target.

### API

All routes are under `/api/v1/agent-tracing` and answer with `Cache-Control: no-store`. Personal data, such as an end user or external session id, goes only in POST bodies, never in URLs.

| Route | Permission |
|---|---|
| `GET /sessions`, `POST /sessions/query` | `traces:read` |
| `GET /sessions/{id}`, `GET /sessions/{id}/spans`, `POST /sessions/{id}/search` | `traces:read` (previews and content matches also need `trace-content:read`) |
| `GET /traces/{trace_id}?app_id=`, `GET /spans/{span_id}?app_id=` | `traces:read` |
| `GET /spans/{span_id}/payloads/{part}?app_id=` | `trace-content:read` |
| `GET /sessions/{id}/export`, `GET /traces/{trace_id}/export?app_id=` | `traces:execute` |
| `DELETE /sessions/{id}`, `POST /purge` | `traces:delete` |
| `GET /config` | `traces:read` |

### Exports

| Format | Details |
|---|---|
| `format=json` (default) | Type `application/vnd.tyk.agent-trace+json; v=1`, following the [agent-trace v1 schema](/schemas/agent-trace-v1.json) |
| `format=markdown` | A readable transcript, up to 512 KiB |

- `content=true` includes captured content only for callers with `trace-content:read`.
- The `X-Tyk-Export-Content` response header says whether content is `included` or `not-included`.
- Exports over 64 MiB are refused.

Not built yet (Phase 2): OTLP-JSON and HTML exports, asynchronous bulk export, and a live view.

---

## OpenTelemetry export

Point `AGENT_TRACING_EXPORTERS_FILE` at a YAML file:

```yaml
exporters:
  - name: tempo
    protocol: grpc                 # grpc (default) or http/protobuf
    endpoint: tempo.internal:4317
    tls: { ca_file: /etc/ssl/tempo-ca.pem }
    signals: [traces, metrics]     # metrics also pushes the agent metrics
  - name: langfuse
    preset: langfuse
    endpoint: https://cloud.langfuse.com
    auth:
      basic_user: env:LANGFUSE_PUBLIC_KEY
      basic_pass: env:LANGFUSE_SECRET_KEY
  - name: datadog
    preset: datadog_llmobs
    endpoint: https://otlp.datadoghq.com
    auth: { api_key: file:/run/secrets/dd_api_key }
    sample_rate: 0.25
```

Entry keys:

| Key | Default | Purpose |
|---|---|---|
| `name` | required | Unique name, used in the metrics |
| `preset` | `generic` | `langfuse` or `datadog_llmobs` set protocol, path, headers and auth |
| `protocol` | `grpc` | `grpc` or `http/protobuf` |
| `endpoint` | required | `host:port` for gRPC, a URL for HTTP; never with `user:pass@` |
| `tls` | system roots | `ca_file`, `cert_file`, `key_file`, `insecure` |
| `headers`, `auth` | none | Values may be `env:NAME` or `file:/path` |
| `compression`, `timeout` | `gzip`, `10s` | |
| `sample_rate` | `1.0` | Share of sessions exported; a session is exported whole or not at all |
| `batch`, `retry` | SDK defaults, `1m` | `max_export_batch_size`, `max_queue_size`, `schedule_delay`; `retry.max_elapsed` |
| `signals` | `[traces]` | Add `metrics` to push the agent metrics; generic preset only |
| `metrics_endpoint` | derived | Defaults to the endpoint, with `/v1/traces` swapped for `/v1/metrics` |
| `allow_insecure_credentials` | `false` | See below |

What each preset sets:

| Preset | Protocol | Path | Auth | Extra headers | Also |
|---|---|---|---|---|---|
| `langfuse` | OTLP/HTTP | `/api/public/otel/v1/traces` | Basic, from `basic_user` and `basic_pass` | `x-langfuse-ingestion-version: 4` | Always sets `session.id`, so Langfuse groups sessions |
| `datadog_llmobs` | OTLP/HTTP | as given | `api_key`, sent as `dd-api-key` | `dd-otlp-source: llmobs` | |

**Credentials never go in clear.** An entry with credentials and an `http://` endpoint or `tls.insecure` is refused unless `allow_insecure_credentials: true` is set. Credentials here means `auth` values, or headers such as `Authorization`, `x-api-key`, `dd-api-key` or any name containing token or secret. Keep secrets out of the file itself by using `env:` or `file:` references.

A bad exporters file turns export off, not tracing.

**How export runs:**

- One replica exports, the holder of the `agent_trace_export` lease.
- A turn is exported once, after it closes and settles. Spans that arrive later are stored, not resent.
- The exported LLM span carries the gateway span id the provider already saw as its parent.
- While export runs, Studio and edge gateways leave out their own live proxy span for captured calls, so each call appears once.
- An edge that sends its own OTel spans to a **different collector** than Studio's export (a regional backend, say) would then get nothing for captured calls. Set `AGENT_TRACING_KEEP_LIVE_SPAN=true` on that edge: it keeps its live span, under its own span id and with a link to the gateway span id the assembled span is exported under, carrying metadata only. Do not set it when the edge and Studio export to the same collector, or each call appears twice.

**Content is never exported.** Export sends metadata only: `AGENT_TRACING_EXPORT_CONTENT` is `none`, and content export is Phase 2. Every attribute that can carry content is dropped. This covers the `gen_ai` message and tool content attributes, `tyk.error.body`, `http.*.body.content`, datasource queries and results, and plugin content.

### Attributes

Every `gen_ai.*` and `mcp.*` name comes from the mapping profile. The profile is `genai-1.41` (schema 1.41.0), the lowest semantic-conventions release that defines every name the exporter uses: the `mcp.*`, cache-token, `gen_ai.request.stream` and reasoning-token attributes first appear in v1.39 to v1.41. Earlier drafts named it `genai-1.37`; that name now falls back to `genai-1.41` with a warning. A stored attribute in those namespaces that the profile does not list is dropped. The tables below are generated from the profile; do not edit them by hand.

Tyk attributes use the `tyk.` prefix. Examples: `tyk.app.id`, `tyk.app.name`, `tyk.session.id`, `tyk.session.source`, `tyk.turn.index`, `tyk.span.kind`, `tyk.correlation.confidence`, `tyk.correlation.rule`, `tyk.inferred` and `tyk.duration.upper_bound`. The full list, generated from the Tyk Weaver registry the export is checked against, closes the tables below. Gateway decisions export their type as `tyk.event.type`, the exported name of the stored `tyk.event`.

<!-- BEGIN GENERATED: agent-tracing OTel attributes (make agent-tracing-docs) -->

Profile `genai-1.41` (version 1, schema `https://opentelemetry.io/schemas/1.41.0`).

#### Span attributes

| Tyk field | OTel attribute | Carries content |
|---|---|---|
| `agent_id` | `gen_ai.agent.id` | no |
| `agent_name` | `gen_ai.agent.name` | no |
| `cache_read_tokens` | `gen_ai.usage.cache_read.input_tokens` | no |
| `cache_write_tokens` | `gen_ai.usage.cache_creation.input_tokens` | no |
| `conversation_id` | `gen_ai.conversation.id` | no |
| `data_source_id` | `gen_ai.data_source.id` | no |
| `finish_reasons` | `gen_ai.response.finish_reasons` | no |
| `input_tokens` | `gen_ai.usage.input_tokens` | no |
| `mcp_method` | `mcp.method.name` | no |
| `operation` | `gen_ai.operation.name` | no |
| `output_tokens` | `gen_ai.usage.output_tokens` | no |
| `provider` | `gen_ai.provider.name` | no |
| `reasoning_tokens` | `gen_ai.usage.reasoning.output_tokens` | no |
| `request_model` | `gen_ai.request.model` | no |
| `response_model` | `gen_ai.response.model` | no |
| `tool_call_id` | `gen_ai.tool.call.id` | no |
| `tool_name` | `gen_ai.tool.name` | no |
| `tool_type` | `gen_ai.tool.type` | no |

#### Span names

Placeholders in braces are filled from the span's fields.

| Tyk span kind | OTel span name |
|---|---|
| `agent` | `agent {agent_name}` |
| `agent_semconv` | `invoke_agent {agent_name}` |
| `attempt` | `attempt {attempt_number} {provider}` |
| `datasource_call` | `retrieval {data_source_id}` |
| `datasource_call_matched` | `execute_tool {tool_name}` |
| `gateway_event` | `tyk.{event}` |
| `inferred_client_tool` | `execute_tool {tool_name}` |
| `llm_inference` | `{operation} {request_model}` |
| `tool_call` | `{mcp_method} {tool_name}` |

#### `gen_ai.operation.name` values

| Tyk operation | Value |
|---|---|
| `chat` | `chat` |
| `execute_tool` | `execute_tool` |
| `invoke_agent` | `invoke_agent` |
| `retrieval` | `retrieval` |

#### `gen_ai.tool.type` values

| Tyk tool kind | Value |
|---|---|
| `client` | `function` |
| `datasource` | `datastore` |
| `mcp` | `extension` |

#### Content attributes

Never exported while `AGENT_TRACING_EXPORT_CONTENT` is `none` (the default):

- `gen_ai.input.messages`
- `gen_ai.output.messages`
- `gen_ai.system_instructions`
- `gen_ai.tool.call.arguments`
- `gen_ai.tool.call.result`
- `gen_ai.tool.definitions`

#### Governed registry

A stored attribute under `gen_ai.*` or `mcp.*` is exported only if it is listed here:

- `gen_ai.agent.description`
- `gen_ai.agent.id`
- `gen_ai.agent.name`
- `gen_ai.conversation.id`
- `gen_ai.data_source.id`
- `gen_ai.input.messages`
- `gen_ai.operation.name`
- `gen_ai.output.messages`
- `gen_ai.output.type`
- `gen_ai.provider.name`
- `gen_ai.request.choice.count`
- `gen_ai.request.encoding_formats`
- `gen_ai.request.frequency_penalty`
- `gen_ai.request.max_tokens`
- `gen_ai.request.model`
- `gen_ai.request.presence_penalty`
- `gen_ai.request.seed`
- `gen_ai.request.stop_sequences`
- `gen_ai.request.stream`
- `gen_ai.request.temperature`
- `gen_ai.request.top_k`
- `gen_ai.request.top_p`
- `gen_ai.response.finish_reasons`
- `gen_ai.response.id`
- `gen_ai.response.model`
- `gen_ai.system_instructions`
- `gen_ai.token.type`
- `gen_ai.tool.call.arguments`
- `gen_ai.tool.call.id`
- `gen_ai.tool.call.result`
- `gen_ai.tool.definitions`
- `gen_ai.tool.description`
- `gen_ai.tool.name`
- `gen_ai.tool.type`
- `gen_ai.usage.cache_creation.input_tokens`
- `gen_ai.usage.cache_read.input_tokens`
- `gen_ai.usage.input_tokens`
- `gen_ai.usage.output_tokens`
- `gen_ai.usage.reasoning.output_tokens`
- `mcp.method.name`
- `mcp.protocol.version`
- `mcp.resource.uri`
- `mcp.session.id`

#### Tyk attributes

From the Tyk Weaver registry the exporter is checked against:

| Attribute | Type | Meaning |
|---|---|---|
| `tyk.agent.declared` | boolean | The agent was declared by the client. |
| `tyk.app.id` | string | Tyk AI Studio App the call belongs to. |
| `tyk.app.name` | string | Name of that App. |
| `tyk.attempt.number` | string | Number of a provider attempt of an /ai/ call. |
| `tyk.attempt.provider` | string | Provider of that attempt. |
| `tyk.attempt.total` | string | Number of provider attempts the call made. |
| `tyk.attribution.candidates` | string | JSON array of type:id catalogue candidates of a weak attribution. |
| `tyk.attribution.confidence` | string | Confidence of the catalogue attribution. |
| `tyk.attribution.rule` | string | Rule of the catalogue attribution. |
| `tyk.budget.id` | string | Budget a budget event applied to. |
| `tyk.content.withheld` | string | Why a span carries no content although capture was on, e.g. edge_link_unverified_tls. |
| `tyk.correlation.confidence` | string | Confidence of the parent link: declared, exact, strong or weak. |
| `tyk.correlation.rule` | string | Correlation rule that produced the parent link. |
| `tyk.cost.currency` | string | Currency of the cost. |
| `tyk.cost.micros` | int | Cost of the call in micro-units of tyk.cost.currency. |
| `tyk.credential.id` | string | Opaque id of the credential that made the call. |
| `tyk.datasource.id` | string | Studio catalogue id of the datasource. |
| `tyk.duration.upper_bound` | string | "true" when the duration is an upper bound (an inferred client-side tool). |
| `tyk.event.name` | string | Name of the policy, filter or target of the event. |
| `tyk.event.outcome` | string | Outcome of the gateway event: allow, block, skip, ... |
| `tyk.event.reason` | string | Reason given for the gateway event. |
| `tyk.event.type` | string | Gateway event type: budget, filter, fallback, router, attempt, ... Stored (and in JSON v1 exports) as tyk.event; exported under this name because tyk.event is the namespace of the other tyk.event.* attributes. |
| `tyk.external.trace_id` | string | Original non-W3C trace id a client declared. |
| `tyk.gateway.node` | string | Gateway node that served the call. |
| `tyk.guardrail.name` | string | Guardrail that acted. |
| `tyk.inferred` | boolean | The span was inferred from LLM traffic rather than observed. |
| `tyk.link.type` | string | Type of a span link: caused_by, continues, provisional_context, candidate. |
| `tyk.llm.id` | string | Studio catalogue id of the LLM. |
| `tyk.mcp.server` | string | MCP server the tool belongs to. |
| `tyk.openai.previous_response_id` | string | OpenAI Responses previous_response_id the request chained to. |
| `tyk.policy.id` | string | Policy of a policy event. |
| `tyk.rollup.cost_micros` | int | Cost under an agent span, in micro-units. |
| `tyk.rollup.errors` | int | Errors under an agent span. |
| `tyk.rollup.llm_calls` | int | LLM calls under an agent span. |
| `tyk.rollup.tokens` | int | Tokens under an agent span. |
| `tyk.rollup.tool_calls` | int | Tool calls under an agent span. |
| `tyk.server_tool_calls` | string | Number of server-side tool calls (run by the provider) in the response. |
| `tyk.session.id` | string | Agent session id (declared or inferred). |
| `tyk.session.source` | string | How the session was formed: declared or inferred. |
| `tyk.span.kind` | string | Tyk span kind: llm_inference, tool_call, datasource_call, inferred_client_tool, plugin_endpoint, gateway_event, agent, attempt. |
| `tyk.stream` | string | "true" for a streamed response. |
| `tyk.stream.incomplete` | string | "true" when the stream ended early. |
| `tyk.tool.catalogue_type` | string | Catalogue entry type: tool, mcp_server or datasource. |
| `tyk.tool.id` | string | Studio catalogue id of the tool. |
| `tyk.tool.in_catalogue` | string | "true" when the tool is in the App catalogue. |
| `tyk.tool.via` | string | Where the tool ran: client, tyk_gateway, studio_mcp or studio_rest. |
| `tyk.tool_call.ids` | string[] | Tool call ids of the span. |
| `tyk.tool_calls.not_offered` | string | JSON array of tools the model called that the request did not offer. |
| `tyk.tools.available` | string | Number of tools the request offered. |
| `tyk.tools.offered` | string | JSON array of the tool names the request offered. |
| `tyk.tools.offered_truncated` | string | "true" when tyk.tools.offered lists only the first 64 offered tools. |
| `tyk.ttft_ms` | int | Time to first token in milliseconds. |
| `tyk.turn.index` | int | Zero-based turn of the span within its session. |
| `tyk.upstream.request_id` | string | Upstream provider's request id. |

<!-- END GENERATED: agent-tracing OTel attributes -->

---

## Metrics

These metrics are on the Prometheus endpoint, and pushed over OTLP to exporter entries with `signals: [metrics]`. Exception: `capture_dropped` and `queue_depth` are on Prometheus only.

| Metric | Type | Attributes |
|---|---|---|
| `tyk_agent_session_duration_seconds` | histogram | `tyk.app.id` |
| `tyk_agent_turn_llm_calls`, `tyk_agent_turn_tool_calls` | histogram | `tyk.app.id` |
| `tyk_agent_cost_total` | counter, micro-units of currency | `tyk.app.id`, `currency` |
| `tyk_agent_sessions_active` | gauge, reported by the assembler's lease holder | none |
| `tyk_tracing_correlation_links_total` | counter | `confidence`, `rule` |
| `tyk_tracing_coverage_ratio` | gauge: share of LLM calls attributed to a session with at least `exact` confidence | `tyk.app.id` |
| `tyk_tracing_export_failures_total` | counter | `exporter` |
| `tyk_tracing_capture_dropped_total` | counter | `reason` |
| `tyk_tracing_export_attributes_dropped_total` | counter: stored `tyk.*` attributes the OTel export refused because the Tyk registry does not list them | `reason` |
| `tyk_tracing_queue_depth` | gauge | none |

- Metrics never carry session, trace, user or tool-call ids.
- Set `AGENT_TRACING_METRICS_INCLUDE_APP_ID=false` to drop `tyk.app.id` as well.
- The GenAI token and duration metrics are described in [Observability](/docs/observability).

---

## Retention, sampling and purge

- **Retention.** The retention job runs every 10 minutes on the assembler's lease holder. It deletes:
  - sessions, turns and spans after `AGENT_TRACING_METADATA_RETENTION_DAYS` of inactivity;
  - captured content after the payload retention: the global value, or the App's **Keep captured content**.
- **Sampling.** `AGENT_TRACING_STORE_SAMPLE_RATE` stores a share of sessions, decided per session. Unsampled sessions keep their summary row, with counts and cost, but no spans or content.
- **Purge a session.** `DELETE /api/v1/agent-tracing/sessions/{id}` removes its spans, links, content and correlation keys.
- **Erase an end user.** `POST /api/v1/agent-tracing/purge` with `{"end_user": "..."}` removes every session declared for that end user. The audit trail records the session ids and an HMAC of the end user, not the value itself.

### Privacy and erasure

- The end user (`X-Tyk-User-Id`, baggage `user.id`, the body `user` field, the IdP subject, or else the Studio user) reaches **every** OTel exporter as `user.id`, including third parties such as Langfuse or Datadog. `AGENT_TRACING_EXPORT_USER_ID_MODE` decides how:
  - `hashed` (default): a pseudonym, an HMAC-SHA-256 of the end user under a key derived from `AGENT_TRACING_KEY` for this purpose only, 32 hex characters. It is the same on every replica and after restarts, so a backend can still group a user's sessions; it changes when the key is rotated. **A pseudonym is still personal data**: anyone who holds the key, or can guess and hash candidate values, can link it back.
  - `raw`: the end user as declared, which may be an email address.
  - `off`: no `user.id`.
- `gen_ai.conversation.id` carries the client's declared session id (`X-Tyk-Session-Id` and its compatible headers) verbatim, whatever `AGENT_TRACING_EXPORT_USER_ID_MODE` says; with `AGENT_TRACING_EXPORT_CONVERSATION_ID_FALLBACK=inferred` it also carries inferred session ids. Clients should not put personal data, such as an email address, in their session ids.
- **A purge or erasure does not reach spans already exported.** Send erasure requests to your OTel backends too.
- Exporter credentials must travel over TLS. Use `env:` or `file:` references in the exporters file, not literal secrets.
