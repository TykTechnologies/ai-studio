# Tyk AI Studio 2.3.0

**Status:** Unreleased. This file collects what has merged to `main` since 2.2.1 and is updated as more merges land.

AI Studio 2.3 is built for teams running AI agents. You can now **see what your agents do**, step by step, without changing them. You can **protect agents from prompt injection** hidden in the pages, files and API responses they read. And you can **cut what agents spend on tokens**. Alongside that, 2.3 brings **FIPS 140-3 builds**, a new class of **decision models**, the **native SDK features** of OpenAI and Anthropic (batches, the Responses API and more) under full governance, **MCP governance on open-source Tyk Gateways**, and around seventy fixes from a sweep of the issue backlog.

## At a glance

- **Agent Call Tracing** (Enterprise): sessions, transcripts and timelines of what each agent did, built from the traffic the gateway already sees.
- **Guardrails for agents** (Enterprise): screen tool results, tool calls and tool definitions for prompt injection, and quarantine a poisoned tool result instead of stopping the agent.
- **Prompt Compressor** (community plugin): smaller requests and lower token bills, without breaking the vendor's prompt cache.
- **FIPS 140-3 builds** of AI Studio and the Microgateway, as packages and container images.
- **Decision models**: govern models that answer typed questions with calibrated probabilities, through one OpenAI-compatible endpoint.
- **Native SDK features**: Anthropic Message Batches and `count_tokens`, and OpenAI Responses, Conversations, Batches and Files, governed like any chat call.
- **Tyk OSS Gateway connections and MCP analytics** (Enterprise): manage MCP proxies and keys on open-source Tyk Gateways, and see how your MCP servers are used.
- **Plugins work on Bedrock everywhere**: plugins attached to a Bedrock LLM now run on every endpoint.
- **Safer by default**: internal network addresses are blocked unless you allow them, plugin signatures are required, credentials are encrypted at rest, and plugins get only the access they declare.
- **More accurate billing**: chat turns with tool calls are no longer billed two or three times, and cached tokens are billed at the cache price.

## Before you upgrade

2.3 changes several defaults to make deployments safer. Check these before upgrading. Each one is explained in [Upgrade notes](#upgrade-notes).

1. **Internal addresses are blocked by default.** If an LLM, embedder, tool, data source, vector store or guardrail provider runs on your own network (a local Ollama, an in-cluster model server), allow it first. [Details](#network-access).
2. **`$ENV/` references only resolve for names you allow.** List them in `AI_STUDIO_ENV_REF_ALLOWLIST`, or move those credentials to the secret store. [Details](#credentials-and-secrets).
3. **Vendor keys are no longer read from environment variables** such as `OPENAI_API_KEY`. Set keys and endpoints on each LLM, embedder and data source. [Details](#credentials-and-secrets).
4. **OCI plugins must be signed** (Enterprise), and plugins no longer inherit the host's environment. [Details](#plugins).
5. **Upgrade the shipped plugins** to their 2.3.0 versions, and rebuild your own request-path plugins on the 2.3 SDK. [Details](#plugins) and [Plugin releases](#plugin-releases).
6. **Existing guardrails start scanning tool results** (Enterprise), and remote guardrail providers start receiving tool output. [Details](#guardrails).
7. **Upgrade edges together with AI Studio.** Several features and credential changes need both on 2.3. [Details](#edges).
8. **Take a database backup.** Restoring it is the only way to roll back. An older AI Studio still starts on a database 2.3 has migrated, but its LLMs with literal keys then fail. [Details](#database).
9. **Set `TYK_AI_SECRET_KEY`** if you use SSO, and so that LLM credentials are encrypted at rest. [Details](#credentials-and-secrets).
10. **Using the quickstart?** Replace its placeholder secrets before the first start. [Details](#other-changes-in-behaviour).

---

## What's new

### See what your agents are doing: Agent Call Tracing (Enterprise)

When an agent misbehaves, the question is always "what did it actually do?". The gateway already sees every LLM call, tool call, MCP call and datasource query an agent makes. AI Studio now turns that traffic into a readable account of each agent session. Nothing changes in the agent: no SDK and no code changes.

- **Agent Sessions** in the admin console: a filterable list of sessions, with a **transcript** and a **timeline** for each one, and a find bar. The timeline shows one labelled row per step, with a colour legend, zoom and pan, and stays usable for sessions with hundreds of turns.
- **The whole picture.** Captures traffic everywhere the gateway runs: the `/llm/` and `/ai/` endpoints, Bedrock, Studio tools, MCP, datasources and edge plugin endpoints, on AI Studio and on edges. The gateway's own decisions appear too: a filter or budget block, a fallback to another model, a routing choice.
- **Tool calls linked to the LLM call that asked for them**, with how confident the link is. Tools that an agent runs itself are inferred from its LLM traffic.
- **Private by default.** Tracing is off until you turn it on (`AGENT_TRACING_ENABLED`). Capturing prompt and response content is a separate choice: `off`, `metadata`, `redacted` or `full`. You set it globally, per App and per request, under a ceiling you control. Secrets are always stripped, and captured content is encrypted at rest.
- **Access is controlled and audited.** Reading captured content needs the new `trace-content:read` permission, and every read is audited. Viewers, Editors and Auditors see end users as stable pseudonyms rather than real ids.
- **Export and erase.** Export a session as JSON or Markdown, purge sessions, and erase everything held about an end user.
- **Send traces to your observability stack.** Finished turns can be exported over OpenTelemetry (GenAI semantic conventions), with presets for Langfuse and Datadog LLM Observability.
- **Built to scale.** Measured at about 8,000 spans per second on a 2-CPU Postgres 16, holding steady from about 4 to 10 million stored spans.

See the [agent tracing guide](docs/site/docs/agent-tracing.md).

### Protect agents from prompt injection: guardrails for agent traffic (Enterprise)

Agents read web pages, files and API responses, and any of them can carry hidden instructions. This is "indirect" prompt injection: the attacker never talks to the model directly. 2.3 extends guardrails to every part of an agent conversation.

- **Tool results are screened.** Request guardrails now inspect everything tools return, across the whole conversation history. This covers every vendor format: OpenAI, Anthropic (including server tools and MCP), Google, Bedrock, Ollama, the Responses API, each item of a batch, and Studio chat's own tools.
- **Quarantine instead of block.** Blocking a request stops an agent for good, because its next request carries the same tool result. The new `quarantine` action replaces just the flagged tool result with a placeholder, and the agent carries on without it.
- **Tool calls are screened** (opt-in). A guardrail can check the arguments of each tool call the model makes, judged against the user's goal, and refuse a dangerous call before the client runs it, even in a stream. In Studio chat, a blocked call never runs.
- **Tool definitions are screened** (opt-in). A poisoned tool description can be removed from the request before the model sees it.
- **Shadow mode.** Try a guardrail on live traffic without adding latency: it logs what it would have done, in the background.
- **Affordable at agent scale.** Each node caches verdicts, so the history an agent resends on every turn isn't classified again. Per-request budgets cap the cost of a single request. Text over a budget is flagged as unscanned, never passed as clean. The built-in pattern library is up to 16 times faster on prose.
- **Works with the providers you use.** Lakera Guard, Azure AI Content Safety Prompt Shields, Azure AI Language PII, Presidio, Bedrock Guardrails and HTTP classifiers all handle tool results. HTTP classifiers can also run over a local Unix socket, with no network.
- **A ready-made filter**, *Prompt injection in tool results (quarantine)*, is added unattached so you can try it straight away.
- **Wolf Defender guardrail images.** Two open-source images (`tykio/ai-studio-guardrail-wolf` for CPU and `tykio/ai-studio-guardrail-wolf-gpu` for NVIDIA) run Patronus AI's Wolf Defender Jev classifier as an HTTP guardrail provider. They score tool results, tool calls, user messages and tool definitions. The CPU image runs next to the gateway with no network access and suits log-only rollouts; the GPU image takes about 20 ms per message, fast enough to enforce inline. See the [Wolf Defender guide](docs/site/docs/guardrails-wolf-defender.md).

**Blocks now explain themselves.** When a guardrail blocks a request, the client gets the reason, a stable reason code and a request id, for example "a tool result from fetch_results of 321,097 characters is longer than this guardrail scans". The matched text is never included. In the console, each compliance event opens with what happened, why, and how to fix it. You can optionally show an excerpt of the flagged text, with credentials and personal data masked. App owners can be allowed to see their own App's recent blocks in the portal.

See the [guardrails guide](docs/site/docs/guardrails.md).

### Spend fewer tokens: Prompt Compressor (community plugin)

Agents send a lot of tool output back to the model: test runs, logs, directory listings, long JSON. The new **Prompt Compressor** plugin (`com.tyk.prompt-compressor`) shrinks these requests on edge gateways before they reach the vendor, so you pay for fewer input tokens. It works on Community and Enterprise.

- **Keeps your prompt cache working.** The same input always compresses to the same output, so the vendor's prompt cache keeps hitting.
- **Three levels:** lossless clean-up (JSON, colour codes, progress bars, whitespace); noise reduction for test runs, logs and listings that keeps every failure; and cutting to a size budget, optionally with an external engine (Headroom or LLMLingua-2).
- **Try it safely.** By default it runs in audit mode: it records what it would save and sends the original request. Switch to enforce mode when you're ready. If anything goes wrong, the request goes through unchanged.
- **See what you save.** A savings dashboard shows tokens and cost saved, with a warning if the cache-read share drops. There is also a policy editor (per LLM or per App) and a preview tool.
- **Trim unused tools.** The dashboard shows what each App's tool definitions cost and which tools it never calls, and a policy can drop named tool definitions from requests.
- Works with OpenAI, Anthropic, Google AI, Vertex, Bedrock and Ollama, on the `/llm/`, `/ai/`, `/v1`, router and `/anthropic/` endpoints. It doesn't apply to Studio Chat or AI Studio's embedded gateway.

Version 1.0.2 needs AI Studio 2.3.0. See the [Prompt Compressor guide](docs/site/docs/prompt-compressor.md).

### FIPS 140-3 builds

AI Studio and the Microgateway are now available as FIPS 140-3 compliant builds, using the Go Cryptographic Module v1.0.0 ([CMVP certificate #5247](https://csrc.nist.gov/projects/cryptographic-module-validation-program/certificate/5247)). Standard builds are unchanged.

- **Packages** for both editions on amd64, arm64 and s390x: `tyk-ai-studio-fips`, `tyk-ai-studio-fips-ee`, `tyk-microgateway-fips` and `tyk-microgateway-fips-ee`.
- **Images** `tykio/tyk-ai-studio-fips` and `tykio/tyk-microgateway-fips` (amd64 and arm64, Enterprise edition), with the same paths, ports and settings as the standard images.
- **Check what you're running.** Both binaries log a `FIPS 140-3` line at startup and report `fips` from `GET /health`. Edges report their FIPS status to the Edge Gateways page.
- **Move databases both ways.** In FIPS mode, passwords and secrets are stored in approved formats, and every build reads both old and new formats. A database can therefore move between standard and FIPS builds. Existing secrets are re-encrypted in the background, and passwords are updated at each user's next login.
- **FIPS plugins.** Plugins can be published as FIPS variants, and AI Studio warns when it starts a plugin that isn't a FIPS build.

See the [FIPS guide](docs/site/docs/fips.md). Moving an existing deployment to FIPS has its own [upgrade notes](#moving-to-fips-140-3-builds).

### Decision models

AI Studio now governs a new kind of model. Instead of generating text, a decision model answers typed questions about a piece of evidence, with calibrated probabilities. Supported: OpenAI's Decisions API (`gpt-6-luna`), and System One models (TypeSafe's hosted Jev, or a compatible self-hosted model).

- **One contract for every provider.** Clients call `POST /v1/decisions` with OpenAI's Decisions schema (also available as `POST /ai/{slug}/v1/decisions`), and AI Studio translates for each provider.
- **Governed like chat.** Authentication, filters, guardrails, plugins, budgets and analytics all apply. A new Dashboard panel shows decision calls, questions answered, refusals and cost.
- **Failover that understands decisions.** A decision call only fails over to models that can answer it, and chat calls skip decision-only models.
- **New System One provider** in the LLM form.

See the [decision models guide](docs/site/docs/decision-models.md).

### Native SDK features, fully governed

Some of the most useful OpenAI and Anthropic SDK features aren't a simple request and response. They now work through AI Studio's native `/llm/` endpoints, with the same model allow-lists, filters, budgets and analytics as any chat call.

- **Anthropic:** Message Batches and `count_tokens`.
- **OpenAI:** the Responses API (including background responses), Conversations, and Batches with batch input files.
- **Each App's resources stay its own.** Batch, file, response and conversation ids are tied to the App that created them. Another App using the same LLM gets `404`, and its request never reaches the vendor.
- **Batches are screened and billed correctly.** Every request in a batch passes the model allow-list and the LLM's filters, and batch usage is recorded once, at the batch price.
- **Billing to match your contract.** Set a price multiplier per service tier, for example `{"batch": 0.4}` for negotiated batch pricing. A new **Service Tiers** Dashboard panel reports usage by tier (`batch`, `flex`, `priority`).
- **Anthropic workspaces.** Pin an LLM to an Anthropic workspace, and every call made for it runs there.
- **Signing keys you can rotate.** Each LLM has its own signing key for resource ids, so rotating the vendor API key no longer breaks outstanding ids. You can rotate the signing key from the LLM page, with a grace period.
- **Better screening on OpenAI.** Filters and guardrails now read the prompts of `/v1/responses`, `/v1/embeddings`, `/v1/completions` and `/v1/moderations`.

See the [proxy guide](docs/site/docs/proxy.md).

### MCP on Tyk Gateway: OSS Gateway connections and MCP analytics (Enterprise)

- **Govern MCP without the Tyk Dashboard.** AI Studio can now manage MCP proxies and keys directly on open-source Tyk Gateways (5.13 or later). It discovers the nodes (a single URL, a list, or DNS), keeps every node in sync and repairs out-of-band edits. A **Nodes** panel shows each node as In sync, Pending, Unreachable or Gone.
- **Per-server key access.** Each MCP server has a **Key access** section for its tool allow-list, rate and quota. Keys work on every node at once.
- **Stays in its lane.** AI Studio only manages MCP proxies, and never changes or deletes proxies it didn't create. See the [OSS Gateway MCP guide](docs/site/docs/tyk-oss-gateway-mcp.md).
- **MCP analytics.** Tyk Pump can now deliver Tyk Gateway analytics to AI Studio, for Dashboard and OSS Gateway connections. Each MCP server gets a **Usage** tab (calls, errors, latency, per-tool and per-App breakdowns), App pages show MCP usage (App owners see their own Apps in the portal), and the Dashboard has an **MCP traffic** panel. No API keys, bodies or client addresses are stored. See the [Tyk MCP integration guide](docs/site/docs/tyk-mcp-integration.md).

### Plugins on Bedrock work on every endpoint

Until now, only authentication plugins ran when clients reached a Bedrock LLM through `/ai/`, the unified `/v1`, routers or `/anthropic/`. Now every plugin runs there, as for every other vendor: caches, rate limiters, load balancers, firewalls and the Prompt Compressor.

- Plugin response headers (`X-RateLimit-*`, `X-LB-*`, `X-Cache*`) now reach clients on those endpoints, for every vendor.
- Load-balancer overrides to another AWS region now work.
- Bedrock inference profiles and ARN model ids work on every `/llm/` route.
- When a plugin refuses a Bedrock call, AWS SDK clients get a proper AWS error with the plugin's message.
- New Plugin SDK helpers (`RequestModel`, `IsStreamingRequest`, `ResponseUsage`) read the model, streaming flag and token usage in every vendor's format.

This changes what plugins and filter scripts see for Bedrock traffic. Read the [Bedrock upgrade notes](#bedrock) before upgrading.

### Other improvements

- **Request ids you can trace.** Every gateway request gets its own id, returned in `X-Tyk-AI-Request-ID`. It stays the same across failover attempts and appears in request logs, compliance events and edge analytics. Your own `X-Request-ID` is kept and recorded alongside it, and you can search request logs by either.
- **Secrets in plugin configuration.** Plugin config fields can reference a stored secret (`$SECRET/name`). The value stays masked in the form, and edges receive it encrypted.
- **Credential references are consistent everywhere.** `$SECRET/` and `$ENV/` references work the same way for LLMs, tools, datasources, embedders, guardrails and plugins, and edges only ever receive resolved values. See the [secrets guide](docs/site/docs/secrets.md).
- **Guardrail scripts can handle checker failures.** With `return_errors: true`, a failed `tyk.llm` call returns an error instead of stopping the script, so a script can decide to block when its checker is unavailable.
- **Privacy warning.** The tool and datasource forms warn when the chosen privacy level is higher than every active LLM can accept.
- **Push reminder.** After you save something that edges use, the console tells you straight away that edges need a configuration push.
- **Catalogue picker when approving a submission.**
- **Helm:** `config.logLevel` sets AI Studio's log level.
- **Startup warning** when filter scripts are given access to the operating system (`FILTER_SCRIPT_ALLOW_OS`).
- **New Prometheus metrics** for decision models (`aistudio_decision_questions_total`), native API operations (`aistudio_native_*`) and guardrails (`aistudio_guardrail_segments_total`, `aistudio_guardrail_cache_total`, `aistudio_guardrail_calls_abandoned_total`, `aistudio_guardrail_shadow_dropped_total`).

---

## Security

- **Microgateway admin API could be used without credentials** (also fixed in 2.2.2). `POST /api/v1/auth/token` issued admin tokens to anyone who could reach a microgateway's HTTP port. That was enough to load a malicious plugin and read or change all traffic. The route is removed, and edge gateways no longer expose `/api/v1` at all. See `CHANGELOG-v2.2.2.md` for what to check on existing gateways.
- **Internal networks are protected by default.** Outbound calls for every managed asset are now covered by the internal-address block: LLMs, Studio chat, embedders, tools, data sources and vector stores, guardrail providers, plugin registries and the MCP wizard. Previously, a submitted tool or data source could probe internal services. Each asset has an admin-only **Allow internal host** switch for legitimate internal services.
- **LLM credentials are encrypted at rest.** API keys and credential fields (such as Bedrock's AWS keys) are encrypted with `TYK_AI_SECRET_KEY`. They are no longer copied into events, object hooks, SQL logs or the edge database in plain text.
- **Credentials stay out of events and hooks.** Tool auth keys, datasource connection strings, user password hashes and tokens, App gateway keys and guardrail secrets now appear as `[redacted]` in system events and object hooks.
- **Guardrail provider credentials reach edges encrypted**, like LLM keys.
- **Chat no longer loads images from model responses by default.** A prompt injection could otherwise make the user's browser send data to any address through an image URL. Turn on **Render images in responses** for chats that should show images.
- **One App can't use another App's OpenAI responses, conversations or batches.** Ids are tied to the App that created them.
- **Requests that filters and the vendor could read differently are refused.** This covers duplicate JSON keys, keys differing only in case, ambiguous multipart uploads and alternative path spellings. Otherwise a filter could screen one prompt while the vendor received another.
- **Prompts filters couldn't read are no longer passed unscreened** when request filters are attached.
- **Plugins get only what they declare.** Plugins run with an explicit environment instead of the host's, can call only the service APIs whose scopes an admin approved, and can only change their own configuration.
- **Plugin signatures are verified in-process** (Enterprise), with no external `cosign` binary, so verification also works in the distroless images. Both cosign v2 and v3 signature formats are accepted, and transparency-log entries are checked.
- **Cached plugin binaries are checked** against their original hash before they load.
- **Plugin hosts are checked properly.** Plugin hosts are classified by parsed address, and shared address space (`100.64.0.0/10`) counts as internal.
- **Vendor clients no longer pick up keys or endpoints from environment variables.**
- **`$ENV/` references only resolve for allowlisted names**, and changing a credential reference needs the `secrets:read` permission.
- **SSO nonce endpoint now requires the identity broker's shared secret.** SSO nonce tokens are now generated with a cryptographically secure random source.
- **A credential pointing at an empty secret no longer reaches edges as an empty key**, which a client could fill from the edge's own environment.
- **Guardrail error events no longer quote the provider's error message**, which could contain the screened text.
- **Approving a submission into catalogues needs catalogue write permission.**
- **Secret fields stay masked in plugin config forms**, including the Redis passwords of advanced-llm-cache and rate-limiter.
- **`pdftotext` runs only from the `PATH`, with a minimal environment.**
- **Earlier `-fips` packages were not FIPS builds.** Packages published under `-fips` names before this release were standard builds. 2.3's `-fips` packages are real FIPS builds, checked before release.
- **Go 1.26.10.** Fixes reachable standard library vulnerabilities in `crypto/x509`, `crypto/tls`, `net/http`, `net/textproto` and `html/template`.

---

## Fixes

### Chat
- **Long chat sessions forgot the latest message.** Once a session passed 100 messages, the newest ones (including the question being asked) weren't sent. Sessions now send the newest 100 messages, with the system prompt kept.
- **Tool-using conversations broke when history was trimmed.** History is now trimmed a whole turn at a time, so a tool call never loses its results. The latest user message is never trimmed. An unset max length now means no limit, rather than dropping almost all of the history.
- **Long replies failed on the Postgres chat queue** with `payload string too long`.
- **Anthropic turns with tool calls were billed two or three times**, and cached tokens were billed at the full input price. Chat sessions are now priced exactly like gateway calls.
- **Tool auth keys stored as `$SECRET/` references were sent as the literal reference text.**

### Guardrails and filters
- **Redacting a request flattened tool-using conversations**, dropping tool calls, images and cache settings, so the vendor refused the request. Redactions are now made in place.
- **A redaction that couldn't be applied could let unredacted text through** on a fail-open guardrail. Such requests are now blocked, or the tool result quarantined.
- **Escaped text could hide from guardrails**, for example instructions after an escaped newline or written as `\u` escapes. Text is now decoded before it's checked.
- **Response guardrails missed text in streams**, and never ran on Responses API, Google and Ollama streams or on Bedrock streams through the bridges. Every stream format is now screened.
- **A blocked stream ended with an error the SDKs ignored.** It now ends with the vendor's own error event, which the OpenAI, Anthropic and Google SDKs raise.
- **A response filter's block on `/ai/` and `/v1` could be swallowed**, so the client saw a normal finish. Clients now get a clear policy error.
- **Buffered Responses API answers weren't screened** by response filters.
- **False positives on placeholder passwords** such as `$DB_PASSWORD`, `<password>` or `****` in configuration files and READMEs.
- **The filter script editor crashed** if a chat page had been opened earlier in the same browser session.
- **Built-in filter templates:** pattern matching is case-insensitive, and the LLM Check template blocks when its check fails.
- **A response filter let responses through when its `tyk.llm` call failed.** Scripts can now handle this with `return_errors`.
- **The filter form's help text** now says correctly what each kind of filter does on tools, and that Redact only logs on a response filter.

### Gateway
- **OpenAI `/v1/responses` calls weren't recorded**, so they cost nothing against budgets and an App could exceed its budget through them.
- **Agent loops through `/ai/` and `/v1` failed.** Tool calls and tool results in the conversation were dropped or sent in the wrong shape, and multi-part message content was lost. They now reach every vendor intact.
- **Bedrock streaming dropped tools, images and documents** on `converse-stream`, so tool use didn't work when streaming. Requests and events are now passed through intact.
- **Bedrock streams could end early under load** on the `/ai/` and `/anthropic/` bridges, and a broken Bedrock stream looked like a complete answer. It now ends with an error.
- **A Bedrock LLM moved to a new endpoint kept calling the old one** until a restart.
- **OpenAI and Ollama requests with multi-part content** (text and images) were refused with `400`.
- **Calls to a model alias were recorded at zero cost.** They're now priced by the requested model when the reported one has no price.
- **`flex` and `priority` OpenAI calls** are now recorded with their service tier.
- **Anthropic `count_tokens` and batch calls** no longer create zero-cost chat records.
- **OpenAI moderations without a `model`** are no longer refused.
- **Native OpenAI decision calls weren't recorded.**
- **A filtered stream with a `Content-Length` could arrive truncated.**
- **`/llm/rest` with a streaming request skipped response filters.** It's now filtered.

### Edges and control plane
- **Edges ran an LLM's plugins in install order** instead of the order set in AI Studio. The order you set now applies on edges too.
- **Edges didn't enforce an LLM's monthly budget** (Enterprise). An LLM budget now applies across AI Studio and every edge, and is kept across restarts. AI Studio and the edges must all run 2.3.
- **One non-UTF-8 record stopped an edge's analytics for good**, along with its budget usage and compliance events.
- **One duplicate tool name froze an edge's configuration.** Edges now skip entries they can't store and report them on the push, and duplicate names are refused up front.
- **Edges could leave extra heartbeat workers running** after reconnecting, or stay marked as connected after dropping.
- **A push could wait forever** if it lost an edge's ready report.
- **Heartbeats report the configuration an edge applied**, not just the last one it received, so a failed sync shows as not in sync.
- **"Database is locked" errors on SQLite** under concurrent writes. SQLite now runs in WAL mode.

### Console and management API
- **A partial `PATCH` wiped the fields it left out.** For example, renaming a group removed its members, and a secret could lose its name and break every reference to it. Every `PATCH` now changes only the fields you send.
- **Deep links survive sign-in.**
- **View in Catalog on a submission** opens the resource it created.
- **Secret fields in plugin config forms** were shown in clear when a field had a long description.
- **A plugin's sidebar showed Configuration twice.**
- **Multi-select fields** close on outside clicks, and a failed submit scrolls to the first error.
- **Edit app** is now in the App details header.
- **Credential status explains an unresolved `$ENV/` key**, and credential help points to the secret store.
- **The quick start names empty provider secrets**, and onboarding cards list their prerequisites.
- **An unusable OpenAPI spec is refused** when creating or updating a REST tool, instead of leaving the tool broken. **Import from URL** says why.
- **The vector store list** shows the stores actually supported (PGVector and Redis, not Milvus).
- **Model price recalculation** keeps batch records at batch price and works on SQLite. The cost breakdown of batch records now adds up.

### Plugin runtime
- **Plugin SDK module graph referenced a private module.** A plugin importing `pkg/plugin_sdk` inherited a requirement on `github.com/TykTechnologies/ai-studio-enterprise/v2`, which nobody outside Tyk can fetch, so `go list -m all` (and tools built on it, such as govulncheck, Dependabot and Renovate) failed. Building was not affected. From 2.3.0 the SDK's module graph resolves from the public Go proxy. If you added a placeholder `replace` for that module to your plugin's `go.mod`, remove it after upgrading the SDK; existing plugin binaries don't need rebuilding.
- **The plugin broker gave up waiting for plugins after 500 ms.** It now waits until the plugin is ready, and plugin events are no longer dropped during startup.
- **The plugin examples and scaffold** keep working with scope enforcement.

### Tyk connections (Enterprise)
- **Changing a connection's base URL** now updates the portal URL of every imported server.
- **Custom domains** are used as the portal URL instead of a URL that returned 404.

### Deployment
- **The Community Edition quickstart works with the distroless images** (the edge no longer crash-loops, and OCI plugins work).
- **The Helm chart** renders `extraEnv`, `extraVolumes` and `extraVolumeMounts` for AI Studio.
- **The quickstart ships placeholders instead of sample secrets.**
- **Stopping AI Studio** waits for background work to finish.

---

## Upgrade notes

### Network access

> **Internal network addresses are blocked by default.** `LLM_UPSTREAM_BLOCK_INTERNAL` now defaults to `true` on AI Studio and the Microgateway. After the upgrade, calls fail if they reach a loopback, private, link-local or other internal address, by IP or by a hostname that resolves to one. This covers an LLM, embedder, tool, data source or vector store, guardrail provider, or a filter's `tyk.makeHTTPRequest`; for example a local Ollama at `http://localhost:11434`, an in-cluster model server, or a classifier on `10.x`. Before upgrading, do one of the following:
> - name those hosts in `LLM_UPSTREAM_ALLOWED_INTERNAL_HOSTS` (comma-separated; `.svc.cluster.local` matches every Service), which keeps blocking on for everything else (recommended);
> - have an administrator turn on **Allow internal host** on that LLM, embedder, tool, data source, guardrail, plugin or MCP server, which exempts only its own host;
> - or set `LLM_UPSTREAM_BLOCK_INTERNAL=false` to restore the 2.2 behaviour.
>
> Set this on every edge too, because each process applies its own setting. A value other than `true` or `false` now stops startup.

- **Helm.** Studio's chart sets `config.blockInternalUpstreams: "true"` and takes `config.allowedInternalHosts`. The Microgateway chart's `config.blockInternalUpstreams` now defaults to `true`.
- **More ranges count as internal**: multicast, reserved, benchmarking (`198.18.0.0/15`), `192.0.0.0/24`, IPv6 site-local, NAT64, 6to4, and shared address space (`100.64.0.0/10`, used by Tailscale and some Kubernetes CNIs). A guardrail provider or plugin host in one of these ranges is refused unless allowed.
- **`ALLOW_INTERNAL_NETWORK_ACCESS=true`** still lifts every internal-address check, platform-wide. Use it for development only. Both opt-outs log a warning at startup.
- **Allow internal host** can only be set by administrators. Portal submissions can't set it, and edges older than 2.3 ignore it and keep blocking.
- See [Outbound network policy](docs/site/docs/proxy.md#outbound-network-policy).

### Credentials and secrets

- **`$ENV/` references resolve only for allowlisted names.** List the environment variables your configuration uses in `AI_STUDIO_ENV_REF_ALLOWLIST` (comma-separated, empty by default), or move those credentials to the secret store as `$SECRET/` references. This applies to LLMs, tools, datasources, embedders, guardrails and plugin configuration.
  - Do this **before** upgrading. To list the names in use: `pg_dump --data-only "$DATABASE_URL" | grep -oE '\$ENV/[A-Za-z_][A-Za-z0-9_]*' | sort -u` (or `sqlite3 midsommar.db .dump | …`).
  - A reference that isn't allowlisted is not resolved: the provider or tool answers `401`, and a guardrail applies its fail mode. The console shows the credential as unresolved, even when the variable is set. See the [secrets guide](docs/site/docs/secrets.md) (Upgrading from 2.2).
- **Vendor keys and endpoints are no longer read from the environment.** An LLM, embedder or datasource with an empty key or endpoint is no longer filled from `OPENAI_API_KEY`, `ANTHROPIC_API_KEY`, `GOOGLE_API_KEY`, `HF_TOKEN`, `PINECONE_API_KEY`, `CHROMA_URL` or `OLLAMA_HOST`. Set them on the object. Don't set these variables on edges: older edges still read them.
- **Setting or changing a `$SECRET/` or `$ENV/` reference needs `secrets:read`** (`403` otherwise). Keeping an existing reference needs nothing extra. Submissions and plugins can't set references.
- **LLM credentials are encrypted at rest.** On first start, AI Studio encrypts every literal LLM API key and credential field with `TYK_AI_SECRET_KEY`.
  - Without `TYK_AI_SECRET_KEY`, values stay as written and AI Studio logs a security warning. Setting the key later encrypts them at the next start.
  - If the key changes or goes missing, everything encrypted with it becomes unreadable: stored LLM credentials and the values in the secret store (so `$SECRET/` references fail too), as well as other encrypted settings. Calls that need them fail. Restore the key, or re-enter the LLM credentials and the secret values. `$ENV/` references are unaffected.
- **SSO needs `TYK_AI_SECRET_KEY`.** Without it, every SSO login is refused, and AI Studio warns at startup.
- **SQL logs no longer contain values.** Set `DB_LOG_SQL_PARAMS=true` only for local debugging, never in production.
- **The quickstart needs your own secrets.** Replace the `__REPLACE_WITH_…__` placeholders in its env files before the first start. Otherwise AI Studio and the Microgateway stop with a key-length error. If your deployment uses values copied from an earlier quickstart or documentation sample, replace them with values you generate.

### Plugins

- **OCI plugins must be signed** (Enterprise). `AI_STUDIO_OCI_REQUIRE_SIGNATURE` now defaults to `true`, as the Microgateway's `OCI_PLUGINS_REQUIRE_SIGNATURE` already did, and the Helm charts now require signatures too. An unsigned plugin, or one signed with a key that isn't configured, no longer loads. Sign your plugins (`tools/publish-plugin.sh` does) and configure their keys (`OCI_PLUGINS_PUBKEY_*`), or set `AI_STUDIO_OCI_REQUIRE_SIGNATURE=false`. Community Edition can't verify signatures: it warns once at startup and loads OCI plugins as unverified.
  - An invalid value (such as `yes`) now stops startup.
  - The first start after the upgrade checks every cached plugin against its registry once more. With signatures required, a plugin whose registry is unreachable then fails to load.
  - A plugin whose command names no key (`?pubkey=`), such as one installed from the marketplace, is accepted if any configured key verifies it, the Tyk key first. To trust a third-party publisher, add their key (`OCI_PLUGINS_PUBKEY_<NAME>`) on AI Studio and every Microgateway; the install command doesn't change. `?pubkey=` still restricts a plugin to one key. Removing a key stops the plugins it verified from loading.
- **Transparency-log entries are required by default** (Enterprise). For plugins signed without one (`--tlog-upload=false`, for example air-gapped signing), set `AI_STUDIO_OCI_TLOG_POLICY` / `OCI_PLUGINS_TLOG_POLICY` to `disabled`. A private Sigstore deployment sets `*_TRUSTED_ROOT_FILE`.
- **The `cosign` binary is no longer shipped or needed** at runtime. Keep using the cosign CLI to sign plugins. Cosign policy files (`VerifyWithPolicy`) are no longer supported.
- **Plugins no longer inherit the host's environment.** List any variables a plugin needs in `PLUGIN_ENV_PASSTHROUGH` (see [plugin deployment](docs/site/docs/plugins-deployment.md)).
  - **github-rag-ingest** is the only shipped plugin whose setup this breaks. If you set `GITHUB_RAG_SECRET_ENCRYPTION_KEY` or `GITHUB_RAG_KNOWN_HOSTS`, add `PLUGIN_ENV_PASSTHROUGH=GITHUB_RAG_*` (or set `secret_encryption_key` and `known_hosts_path` in the plugin configuration) **before upgrading**. Without the key, repositories with encrypted credentials fail to sync, and credentials saved afterwards are stored **unencrypted**. Re-enter any credential saved while the key was missing.
  - `ALLOW_INTERNAL_NETWORK_ACCESS` isn't passed through, so llm-price-sync and github-rag-ingest refuse internal URLs unless you pass it through.
- **Plugins get only the scopes they declare.** A service-API call outside a plugin's approved scopes is refused. Upgrading a plugin that declares new scopes asks an admin to approve them (advanced-llm-cache adds `apps.write`).
- **Rebuild your request-path plugins on the 2.3 SDK.** Plugins now accept messages up to 64 MiB (`PLUGIN_GRPC_MAX_MSG_BYTES`). One built on an older SDK keeps the 4 MiB limit: larger requests get `413`, and the log names the plugin. Run the 2.3.0 releases of the shipped plugins (see [Plugin releases](#plugin-releases)).
- **Plugins on an internal registry or host are refused**, installed ones included, unless the host is named in `LLM_UPSTREAM_ALLOWED_INTERNAL_HOSTS` (on AI Studio and every Microgateway that loads them). Look for log lines that start `plugin host "…" is an internal address`. `PLUGIN_BLOCK_INTERNAL_URLS` is deprecated.
  - If the registry name doesn't resolve (DNS unavailable), a cached plugin that passes its checks still loads, with a warning.
  - Behind an HTTP(S) proxy, the registry name must still resolve on the host. On proxy-only networks, list the registry and the hosts it redirects downloads to in `LLM_UPSTREAM_ALLOWED_INTERNAL_HOSTS` (for `docker.tyk.io`: `docker.tyk.io,dl.cloudsmith.io`).
- **Cached plugins are downloaded once more** after the upgrade, to record their hash. The registry should be reachable then.
- **Plugins no longer see credentials.** LLM, tool, datasource and user credentials appear as `[redacted]` in system events and object hooks. A `before_*` hook that returns a value still `[redacted]` keeps the stored one.
- **Edge response plugins now receive the LLM's `vendor`.**
- **Plugin order on edges.** Edges now run an LLM's plugins in the order set in AI Studio, not install order. Check the order on each LLM with more than one plugin before upgrading edges.

### Bedrock

Plugins attached to a Bedrock LLM now run on `/ai/`, `/v1`, routers and `/anthropic/`, and filters there now read the Bedrock Converse format. Before upgrading:

- **Upgrade the shipped plugins** to advanced-llm-cache 1.3.0, llm-cache 1.3.0, llm-load-balancer 1.2.0, rate-limiter 1.2.0 and llm-firewall 1.2.0. Earlier versions mishandle Bedrock: caches mix up models and cache errors, token rules count no Bedrock tokens, firewall model rules never match, and the load balancer counts errors as successes.
- **Check your own plugins.** On Bedrock there's no `model` in the body (it's in the path), usage is camelCase, and streaming is chosen by the path (`converse-stream`). Use the SDK helpers `RequestModel`, `IsStreamingRequest` and `ResponseUsage`.
- **Check custom filter scripts on Bedrock LLMs.** They get `input.format == "bedrock"` and a Converse `raw_input`, not the OpenAI or Anthropic shape the client sent. Read `input.messages`, or handle Converse. Guardrails and the redaction helpers already handle it.
- **Data collectors** see Converse bodies for Bedrock traffic on these endpoints.
- **No automatic AWS SDK retries.** Each failover step tries a Bedrock call once, and failover handles 5xx and 429 errors.
- **Status codes.** Refusals reach the client with their real status: `400` for a guardrail block, the plugin's own status for a plugin block, and `403` for a budget refusal.
- **Request logs** show the LLM's name as the model for buffered Bedrock calls on these endpoints. Chat records keep the model id.
- **AI Studio's embedded gateway** (including Chat) applies budgets, filters and analytics to Bedrock the same way.

### Guardrails

- **Existing request guardrails scan tool results** (Enterprise). A guardrail saved before 2.3 inspects every tool result after the upgrade, and a `block` action becomes `quarantine` for tool results. For the seeded filters:
  - *Credentials in prompts (block)* quarantines a tool result containing a credential, and the agent goes on without it.
  - *Personal data before the vendor (redact)* redacts personal data in tool output.
  - *Prompt injection heuristics (log)* records injection markers in tool output.

  To keep a filter as it was, untick **Scan tool results** (or set `"tool_results": {"scan": false}`).
- **Remote providers receive tool output.** Filters on Lakera Guard, Azure AI Content Safety, Azure AI Language PII, Bedrock Guardrails, Presidio or an HTTP classifier start sending tool results to that provider. Tool output can contain file contents, database rows and credentials. If a provider must not see that data, turn off tool-result scanning **and** use a scope other than `all_messages`.
- **More provider calls.** Lakera Guard and Bedrock Guardrails now get one call per message and tool result, so conversations of many short messages cost more. Azure AI Language PII sends one document per request. The verdict cache answers history an agent resends, and `budget.max_pieces` (256) caps the calls of one request.
- **Fail-open is narrower for large requests.** When a request is larger than `single_call_chars` (32,768 characters) or needs more provider calls than usual, a provider failure leaves that text unscanned and the filter's action applies, so a fail-open filter can block during an outage. Raise `budget.single_call_chars` to bring more requests under the fail mode. Text a provider refuses as too large, or that the built-in library can't scan in time, is also unscanned.
- **Guardrails protect what reaches the vendor, not what AI Studio stores.** A blocked request is logged as received. Where that traffic must not be stored, turn on the LLM's **Disable Request/Response Body Logging**, and keep `ANALYTICS_STORE_REQUESTS` and `ANALYTICS_STORE_RESPONSES` off on its edges.
- **Guardrail block messages are longer.** They now end with "Reason: … (code). Request ID: …". Match on the reason code (`reason_code`, `X-Tyk-AI-Guardrail-Reason`) rather than the exact text.
- **Chat script filters see client tool answers**, as they see typed messages.
- **Small hosts.** On a 2-CPU host, a flood of large tool results can keep both cores busy. Run gateways that scan agent traffic with the built-in library on 4 or more CPUs, or set `GUARDRAIL_LOCAL_SCAN_SLOTS=1`.

### Edges

Upgrade edges together with AI Studio. Edges older than 2.3:

- **block guardrailed traffic**, even under `fail_mode: open`, because guardrail credentials now reach edges only encrypted;
- **fail Bedrock calls**, because AWS credentials now reach edges only encrypted (an old edge never falls back to its own AWS credentials);
- **can't serve decision models** (the `systemone` vendor), and don't record OpenAI decision calls;
- **don't scan tool results**, and block where a 2.3 edge would quarantine;
- **don't enforce LLM budgets**, ignore **Allow internal host**, and don't verify resource ids minted by AI Studio.

After the upgrade:

- **Edges show Pending once.** Push the configuration.
- **Duplicate tool or LLM names** stored by earlier versions are left out on edges (the lowest id keeps the name) until renamed. The edge shows as Pending until then.
- **An LLM or tool that needs a filter or plugin from another namespace** is left out on that namespace's edges. Move the filter or plugin to the global namespace or the LLM's namespace.
- **Edge errors now reach AI Studio's request logs.** Policy blocks and errors on edges appear as request log rows with no spend, so request counts include them.
- **The request id header is `X-Tyk-AI-Request-ID`.** Edges no longer overwrite `X-Request-ID`.

### Native APIs and OpenAI

- **OpenAI response ids issued before 2.3 no longer verify**: the first call gets `404`, and the client starts a new chain.
- **`/v1/responses` must name a `model`**, as must batch lines (`body.model`), so the model allow-list can be checked.
- **`/v1/files` accepts only `purpose=batch` uploads**, up to 64 MiB and, on an LLM with request filters, 10,000 lines.
- **With request filters attached, unreadable input is refused** (`400`) rather than passed unscreened.
- **OpenAI batch files uploaded before 2.3** must be uploaded again on an LLM with request filters, and again whenever its request filters change. A batch must be created through the gateway that received its file.
- **Some headers are no longer forwarded**: `OpenAI-Organization`, `OpenAI-Project`, `anthropic-user-profile-id` and a client's `anthropic-workspace-id`. Use the LLM's workspace setting instead.
- **Plugin upstream overrides** no longer apply to `count_tokens` or OpenAI native operations.
- **Decision models need a price.** Add a model price with the provider's input price and an output price of `0`. Unpriced calls are recorded at zero cost.

### Database

- **Rolling back means restoring the backup.** Take a backup before upgrading. An older AI Studio does not refuse a database 2.3 has migrated: it starts with one warning (`The database schema is version 28 …, newer than this build's …; keeping the newer record`) and sends the encrypted form of every literal LLM key as the key itself, to vendors and to edges, so those calls fail with `401`. LLMs keyed with `$SECRET/` or `$ENV/` references keep working. Only an older headless replica refuses the database. To roll back, stop AI Studio and restore the backup; anything changed since the upgrade is lost.
- **Upgrade replicas together.** Stop every older AI Studio replica before the first 2.3 replica starts. An older full replica sharing the database joins the cluster, can become the leader, and serves the encrypted keys. The Helm chart's default `Recreate` strategy does this; don't use `RollingUpdate` for the upgrade to 2.3.
- **SQLite runs in WAL mode.** `-wal` and `-shm` files appear next to the database, so its directory must be writable. On network filesystems (NFS, SMB), set `_journal_mode=DELETE` in `DATABASE_URL`.
  - To back up a running 2.3 SQLite database, use `sqlite3 midsommar.db ".backup backup.db"` (or `VACUUM INTO`). A copy of the database file alone misses the changes still in the `-wal` file. A 2.2 database has no `-wal` file, so a copy of it taken with 2.2 stopped is complete.
  - To restore a SQLite backup, stop AI Studio, delete `midsommar.db-wal` and `midsommar.db-shm`, then copy the backup over `midsommar.db`. If 2.3 didn't shut down cleanly, a leftover `-wal` file is replayed onto the restored copy and corrupts it (`database disk image is malformed`). Keep the original backup file until the restored database has started.
- **MCP call records** have no retention pruning yet, so that table grows with MCP traffic.

### Moving to FIPS 140-3 builds

See the [FIPS guide](docs/site/docs/fips.md).

- **Re-encryption.** A FIPS build re-encrypts stored secrets at startup, in the background, and logs when it's done. Before enforcing strict mode (`GODEBUG=fips140=only`), start once in FIPS mode and let it finish.
- **Plugins.** A deployment is only FIPS compliant if every plugin is a FIPS build too.
- **TLS.** FIPS mode allows only TLS 1.2 and 1.3 with approved ciphers and curves. Peers that offer only ChaCha20-Poly1305, or only plain X25519, fail to connect. This applies to LLM vendors, the hub-to-edge link, plugins over gRPC and your own endpoints.
- **Keys.** TLS certificates must use RSA 2048+, ECDSA P-256/P-384/P-521 or Ed25519. Use RSA 2048+ or ECDSA for plugin-signing keys and SSO identity providers too.
- **Postgres.** Use SCRAM-SHA-256 authentication, not `md5`.
- **Resource signing.** Set a signing secret for every LLM. Resource ids signed in standard mode from a short credential don't verify in FIPS mode, and vice versa.

### Other changes in behaviour

- **Chat sessions send their history.** On LLM settings without a max length, a session now sends the conversation (up to the newest 100 messages), not just the latest message. Set **Max Length** to cap input.
- **Chat costs change.** Anthropic turns with tool calls or thinking drop to the correct amount, cached tokens are billed at the cache price, and Anthropic chat records' total tokens include cache tokens.
- **Tool and LLM names must be unique** ignoring case, spaces and punctuation. Duplicates stored earlier stay editable.
- **Decision-only LLMs** don't appear in chat, agent, router and wizard pickers. The failover picker only offers compatible models.
- **Edge gateways answer `404` on `/api/v1`**, and the `mgw` CLI no longer works against an edge; configure edges from AI Studio. A standalone microgateway keeps its management API, but create tokens with `POST /api/v1/tokens` or `microgateway -create-admin-token`.
- **advanced-llm-cache 1.3.0:** Redis `tls_enabled` now takes effect, so check that your Redis accepts TLS. Use `tls_insecure` instead of `tls_skip_verify`. The cache starts empty after the upgrade.
- **MCP analytics need traffic logs on the proxy.** Proxies created outside AI Studio record nothing until `trafficLogs` is enabled. Tyk Pump 1.16.0 needs `dont_purge_uptime_data: true` unless you run an uptime store.
- **Agent tracing content ceiling.** If `AGENT_TRACING_CONTENT_CEILING` is unset, it is `metadata`, which overrides Apps set to Redacted or Full. Set it to the highest mode you want to allow.
- **`AGENT_TRACING_BAGGAGE_ALLOWLIST`** (default `session.id`, all editions) limits the W3C `baggage` header sent upstream to the listed keys. Set it to the keys your providers rely on, or `none`.

### For plugin authors and Go embedders

- `models.LLMVendorProvider` gains `ProvidesChat()` and `DecisionEndpoint()`, so custom vendor implementations must add them.
- `CallLLM` returns `FailedPrecondition` when the agent's App has no credential, instead of a `401`.
- Plugins and hooks see the new request id format (`tyai_` + UUIDv7) in `request_id` and `PluginContext.RequestID`.
- The plugin, management and config-sync protocols change additively (new fields and a `CallDecision` RPC).
- When embedding with `pkg/studio`, `AppConf.FilterLimits` applies only to names in `config.FilterLimitNames`, which now include the `GUARDRAIL_*` settings.
- Analytics sinks can implement `analytics.MCPCallRecorder` to receive MCP calls. Existing handlers compile unchanged.

---

## New configuration

No new setting is required to run 2.3. Defaults that changed are listed in [Upgrade notes](#upgrade-notes).

**General** (AI Studio and the Microgateway):

| Variable | Default | Purpose |
|---|---|---|
| `AI_STUDIO_ENV_REF_ALLOWLIST` | empty | Environment variables a `$ENV/NAME` reference may resolve (comma-separated) |
| `PLUGIN_ENV_PASSTHROUGH` | empty | Host variables passed to plugins, besides the built-in allowlist; a trailing `*` matches a prefix |
| `PLUGIN_GRPC_MAX_MSG_BYTES` | `67108864` (64 MiB) | Largest message between the host and its plugins |
| `DB_LOG_SQL_PARAMS` | `false` | Log SQL values (local debugging only) |

The Helm chart adds `config.logLevel` (AI Studio's `LOG_LEVEL`).

**Plugin signatures** (Enterprise; an invalid value stops startup):

| Variable (AI Studio / Microgateway) | Default | Purpose |
|---|---|---|
| `AI_STUDIO_OCI_REQUIRE_SIGNATURE` / `OCI_PLUGINS_REQUIRE_SIGNATURE` | `true` / `true` | Require a valid signature on OCI plugins (AI Studio's default was `false`) |
| `AI_STUDIO_OCI_TLOG_POLICY` / `OCI_PLUGINS_TLOG_POLICY` | `required` | `required`, `optional` (checked when present) or `disabled` (key only) |
| `AI_STUDIO_OCI_TRUSTED_ROOT_FILE` / `OCI_PLUGINS_TRUSTED_ROOT_FILE` | empty | A Sigstore `trusted_root.json` for a private Sigstore deployment |

**Agent Call Tracing** (Enterprise): `AGENT_TRACING_*` settings, all off by default. The [agent tracing guide](docs/site/docs/agent-tracing.md) lists them. The content ceiling and default mode (`AGENT_TRACING_CONTENT_CEILING`, `AGENT_TRACING_CONTENT_DEFAULT_MODE`) can only be set in the environment; the console shows them read-only. An invalid content mode is read as `off`.

**FIPS 140-3:** a FIPS build starts in FIPS mode. `GODEBUG=fips140=off` turns it off, and `GODEBUG=fips140=only` refuses every non-approved algorithm (for testing). `make build-fips` builds FIPS binaries from source.

**Native API operations** (AI Studio and the Microgateway; unset or `0` keeps the default):

| Variable | Default | Purpose |
|---|---|---|
| `NATIVE_API_MAX_BODY_BYTES` | `67108864` (64 MiB, at most 256 MiB) | Size limit of a batch create or batch file upload |
| `NATIVE_API_MAX_CONCURRENT_CREATES` | `4` | Batch creates handled at once |
| `NATIVE_API_MAX_FILTERED_ITEMS` | `10000` | Requests per batch or file when the LLM has request filters |
| `NATIVE_API_MAX_IN_PROGRESS_PER_APP` | `0` (no cap) | In-progress batches per App and LLM |
| `NATIVE_API_RECONCILE_INTERVAL` | `30s` | How often finished batches are looked for |
| `NATIVE_API_RECONCILE_BATCH` | `50` | Batches checked per pass |
| `NATIVE_API_RECONCILE_WORKERS` | `4` | Batches reconciled at once |
| `NATIVE_API_POLL_INTERVAL` | `2m` | Wait before polling an unfinished batch again |
| `NATIVE_API_POLL_MAX_BACKOFF` | `30m` | Longest wait after failed polls |

`PROXY_URL` (Microgateway: `GATEWAY_PUBLIC_URL`) is the base of a batch's `results_url`.

**Guardrails** (Enterprise; AI Studio and the Microgateway; see the [guardrails guide](docs/site/docs/guardrails.md)):

| Variable | Default | Purpose |
|---|---|---|
| `NATIVE_API_MAX_GUARDRAIL_CALLS` | `10000` | Remote provider calls one batch may make |
| `GUARDRAIL_VERDICT_CACHE_ENTRIES` | `100000` | Verdicts cached per node |
| `GUARDRAIL_VERDICT_CACHE_MAX_BYTES` | `67108864` (64 MiB) | Memory the verdict cache may use per node |
| `GUARDRAIL_VERDICT_CACHE_TTL` | `10m` | How long a verdict is reused; `0` turns the cache off |
| `GUARDRAIL_LOCAL_SCAN_SLOTS` | half the CPUs (at least 1) | Large built-in library scans at once |
| `GUARDRAIL_LOCAL_SCAN_SMALL_SLOTS` | a quarter of the CPUs (at least 1) | Small scans at once, besides the large ones |
| `GUARDRAIL_LOCAL_SCAN_QUEUE_BYTES` | `67108864` (64 MiB) | Text one tenant may have waiting for a scan |
| `GUARDRAIL_LOCAL_MAX_SCAN_BYTES` | `8388608` (8 MiB) | Largest segment the built-in library scans; larger ones are unscanned |
| `GUARDRAIL_LOCAL_SCAN_MS_PER_MIB` | `2000` | Extra scan time per MiB of text, beyond the filter's timeout |
| `GUARDRAIL_SHADOW_WORKERS` | 4 per CPU, at least 8 | Shadow evaluations running at once |
| `GUARDRAIL_SHADOW_QUEUE_BYTES` | `67108864` | Text waiting for a shadow evaluation; overflow is dropped and recorded as unscanned |
| `GUARDRAIL_UNIX_SOCKET_ALLOWLIST` | empty | Unix socket paths (or directories ending in `/`) an HTTP classifier may use |

Per guardrail filter: `tool_results` (`scan`, `on_detect`), `tool_calls`, `tool_definitions`, the `quarantine` action and its `placeholder`, `shadow`, `evidence`, `budget` (`max_scan_chars` 262144, `max_pieces` 256, `time_ms` 4 × `timeout_ms`, `single_call_chars` 32768) and `cache`. The form only saves the keys you set.

**Per LLM**, in the LLM form and API: `vendor_workspace_id` (Anthropic workspace), `price_multipliers` (for example `{"batch": 0.4}`), **Allow internal host**, and the resource signing key (status and rotation on the LLM page).

---

## Plugin releases

These plugins are released to the marketplace for 2.3.0. Versions marked † need AI Studio 2.3.0 and are built on the 2.3 Plugin SDK. Each has its own `CHANGELOG.md`.

| Plugin | Version | What changed |
|---|---|---|
| prompt-compressor (new) | 1.0.2 | New plugin, needs AI Studio 2.3.0: see [Prompt Compressor](#spend-fewer-tokens-prompt-compressor-community-plugin) |
| llm-cache | 1.3.0 † | Works correctly with Bedrock (errors never cached, token savings counted). No longer replays other plugins' headers on a hit. Stability and logging fixes |
| llm-load-balancer (Enterprise) | 1.2.0 † | Counts Bedrock errors as failures, and client errors (400, 413, 422) no longer count against an upstream. Fixes a selection race. Validates and redacts upstream URLs |
| advanced-llm-cache (Enterprise) | 1.3.0 † | Works correctly with Bedrock. Cache keys include the LLM, vendor, path and model, so the cache starts empty after the upgrade. Redis TLS now applies, and the Redis password is masked. Needs the `apps.write` scope |
| rate-limiter | 1.2.0 † | Token rules count Bedrock, Google and streamed usage. Concurrency slots are given back for requests answered before the vendor. Masked Redis password |
| llm-firewall | 1.2.0 † | Model rules match Bedrock model ids, inference profiles and ARNs. Inspects all prompt text, including Bedrock text documents. Optional `fail_closed` |
| llm-price-sync | 1.1.1 | Refuses internal fetch URLs. Reliability fixes |
| github-rag-ingest | 1.1.1 | Secret references for credentials. Cancelling a job stops it. Masked `vault_token`. Hardened clone cache. Needs AI Studio 2.3.0 for secret references and `PLUGIN_ENV_PASSTHROUGH` |
| oauth2-client-credentials (Enterprise) | 1.3.3 † | Rebuilt on the 2.3 SDK: with 1.3.2, a 2.3 gateway refuses authenticated requests over 4 MiB. Better JWKS caching |

---

## Known issues

- **Model and Semantic Routers don't route decision calls yet.** They answer `400`.
- **Decision models haven't been tested against a self-hosted llama.cpp server.** They are tested against OpenAI `gpt-6-luna` and TypeSafe `jev-latest`.
- **Some OpenAI ids aren't ownership-checked.** A `file_id` in an `input_file` or `input_image` part, and an `item_reference` id, are passed to OpenAI unchecked.
- **`input_file` content isn't screened** by filters.
- **Standalone microgateways sign resource ids with the vendor API key**, so rotating that key invalidates outstanding ids.
- **A dropped database connection at the wrong moment can lose a batch's usage.** This is rare, and it undercounts; it never double-charges.
- **The proxy-log API doesn't return `service_tier` yet.** The tier report and chat records have it.
- **What guardrails don't inspect:** binary documents (PDF, Word) and images; files referenced by id and conversation state held by the vendor; results of tools the vendor runs within one call, until the client sends them back; and datasource context in Studio chat.
- **The built-in injection heuristics are a baseline.** They are deliberately narrow on tool results to avoid false positives, so paraphrased attacks get through. Pair them with Lakera Guard, Azure Prompt Shields, Bedrock Guardrails or the Wolf Defender images for robust coverage.
- **Script filters see tool results as sent**, not decoded as guardrails do. An empty `input.segments` with a non-empty `raw_input` means an unreadable body, which a script that screens tool results should block.
- **Large tool results can wait on slow hosts** behind a flood of large scans.
- **Keys that differ only in case** can still be read differently by a script filter and the upstream. Guardrails refuse such bodies.
- **Dashboard-created MCP proxies don't set `stripAuthorizationData`**, so AI Studio-minted Tyk keys are forwarded to third-party upstreams. Gateway connections set it.
- **Rate limiter concurrency slots can be held for up to ten minutes** by a request that ends without a response.

---

Full list of changes: [`v2.2.1...v2.3.0`](https://github.com/TykTechnologies/ai-studio/compare/v2.2.1...v2.3.0)
