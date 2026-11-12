# Tyk AI Studio 2.3.0

**Status:** Unreleased. This file collects what has merged to `main` since 2.2.1 and is updated as more merges land.

AI Studio 2.3 is built for teams running AI agents. You can now **see what your agents do**, step by step, without changing them. You can **protect agents from prompt injection** hidden in the pages, files and API responses they read. And you can **cut what agents spend on tokens**. Alongside that, 2.3 brings **FIPS 140-3 builds**, a new class of **decision models**, the **native SDK features** of OpenAI and Anthropic (batches, the Responses API and more) under full governance, **MCP governance on open-source Tyk Gateways**, and around seventy fixes from a sweep of the issue backlog.

## At a glance

- **Agent Call Tracing** (Enterprise): sessions, transcripts and timelines of what each agent did, built from the traffic the gateway already sees.
- **Guardrails for agents** (Enterprise): screen tool results, tool calls and tool definitions for prompt injection, quarantine a poisoned tool result instead of stopping the agent, fix a false positive for one App with an exception, and get a second opinion from an LLM.
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

1. **Get a 2.3 license first** (Enterprise). A license issued for 2.2 or earlier is not accepted: AI Studio and Enterprise microgateways on 2.3 do not start with it. Speak to your account manager for a 2.3 license before you upgrade, and set it as `TYK_AI_LICENSE` on AI Studio and on every Enterprise microgateway. Community Edition needs no license. [Details](#license-enterprise).
2. **Internal addresses are blocked by default.** If an LLM, embedder, tool, data source, vector store or guardrail provider runs on your own network (a local Ollama, an in-cluster model server), allow it first. [Details](#network-access).
3. **`$ENV/` references only resolve for names you allow.** List them in `AI_STUDIO_ENV_REF_ALLOWLIST`, or move those credentials to the secret store. [Details](#credentials-and-secrets).
4. **Vendor keys are no longer read from environment variables** such as `OPENAI_API_KEY`. Set keys and endpoints on each LLM, embedder and data source. [Details](#credentials-and-secrets).
5. **OCI plugins must be signed** (Enterprise), and plugins no longer inherit the host's environment. [Details](#plugins).
6. **Upgrade the shipped plugins** to their 2.3.0 versions, and rebuild your own request-path plugins on the 2.3 SDK. [Details](#plugins) and [Plugin releases](#plugin-releases).
7. **Existing guardrails start scanning tool results** (Enterprise), and remote guardrail providers start receiving tool output. [Details](#guardrails).
8. **Upgrade edges together with AI Studio.** Several features and credential changes need both on 2.3. [Details](#edges).
9. **Take a database backup** before the first 2.3 start. Restoring it is the only way to roll back. An older AI Studio still starts on a database 2.3 has migrated, but its LLMs with literal keys then fail. [Details](#database).
10. **Set `TYK_AI_SECRET_KEY`** if you use SSO, and so that LLM credentials are encrypted at rest. [Details](#credentials-and-secrets).
11. **Using the quickstart?** Replace its placeholder secrets before the first start. [Details](#other-changes-in-behaviour).

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
- **Trusted prompt templates.** An App that wraps untrusted text (a pull request, an issue) in its own instructions can have that template approved by an administrator. Request guardrails then score only the parts that vary, so the App's own instructions stop being flagged as injection. Matching is exact: anything short of an approved, byte-for-byte match is scored whole, and tool results are never exempt. A backstop also scores the full message, logging by default; for Apps whose text comes from untrusted authors, set it to enforce. Templates are proposed and approved on the App page, where the approval view shows each constant's exact bytes with hidden characters marked and a diff against the previous version; only a full administrator approves. They apply on edge gateways too. An HTTP classifier that handles templates itself, such as the Wolf Defender guardrail, is enabled with `templates: "true"` on its connection. A constant at the anchored start or end of a template may be as short as 16 bytes (`PROMPT_TEMPLATE_MIN_ANCHORED_CONST_BYTES`, 8 to 32), for prompts that open with a short fixed header before variable text. See [Trusted prompt templates](docs/site/docs/guardrails.md#trusted-prompt-templates).
- **Guardrail exceptions: fix a false positive for one App.** When an App's legitimate traffic keeps being flagged (a code-review bot whose pull requests contain injection test vectors, a security tool quoting exploits), you no longer have to turn the whole filter to log. From the flagged event in **Compliance → Filter Events**, choose **Not a threat? Soften for this app**: the dialog is filled in from the event, and you pick what happens instead of the block (for example log). An exception changes the consequence of a verdict for one App, filter, detector and channel; it never changes detection, is never keyed on the text itself, and a second detector without an exception still blocks. A full administrator's exception applies at once; anyone else who may edit the App proposes one for a full administrator to approve, and App owners can **Report false positive** on a blocked request in the Portal, which proposes one too. Exceptions expire after 30 days without a hit, record every hit on the compliance event, and apply on edge gateways after the next push. See [Exceptions](docs/site/docs/guardrails.md#exceptions-softening-a-verdict-for-one-app).
- **App guardrail capability.** How far an exception may soften is bounded by what the App can do, which a full administrator attests on the App page: Read-only, Reads with tools or Acting. AI Studio suggests one from the App's tools, MCP servers and datasources, and an App nobody has attested is treated as Acting, the strictest. If someone other than a full administrator changes an attested App's tools, datasources, MCP servers or plugin resources, the attestation is cleared until a full administrator attests again; where the App's LLMs run guardrails, the editor warns first and administrators are notified.
- **Log and raise event.** A new guardrail action lets traffic through like log and raises a named event, `guardrail.alert.<name>`, that webhooks (Enterprise) can subscribe to, so your team hears when an exception lets something through or a log-only filter fires. It is available on the filter, per tool channel and on exceptions, and works in shadow mode. Alerts carry no flagged text unless you opt in.
- **Outcome events for blocks, quarantines and judge softenings.** Every guardrail block, quarantine and softening by the second opinion now raises an event (`guardrail.outcome.blocked`, `.quarantined`, `.judge_softened`), with nothing to configure. Webhooks (Enterprise) and AI Studio plugins can subscribe, for example to hear in Slack about everything the judge let through. The events carry the filter, App, detector, action and the judge's opinion, never the flagged text. See [Outcome events](docs/site/docs/guardrails.md#outcome-events-blocked-quarantined-softened-by-the-judge).
- **Click through from an alert to the event.** Guardrail alerts and outcome events link to their compliance event, and the Slack preset shows an **Open in AI Studio** link (set `SITE_URL` for the full URL). The link opens **Compliance → Filter Events** with that event expanded, whatever its age; anyone following it still signs in and needs permission to read compliance.
- **A second opinion from an LLM.** A guardrail can ask an LLM you configure in AI Studio (the judge) whether what it blocks or quarantines is in fact harmless for the App that sent it. Start with **Record only**: the request keeps the filter's verdict, the judge answers in the background, and **Filter Events** shows how often it disagreed ("judge said benign on N of M flags"). Once enough opinions are recorded, **Soften blocks** lets a benign opinion turn a block into the softest action the App's attested capability allows, and never further. The judge only answers in a fixed form and treats the flagged text as data. It is only asked about prompt-injection findings, never about secrets, personal data, moderation, denied topics or blocklists, nor about text it would not see in full, and any failure keeps the verdict. Its calls go straight to the judge LLM and are charged to that LLM's budget, never the App's (interaction type `guardrail_judge` in analytics). The judge LLM must have a privacy score at least that of the LLMs the filter guards. Studio chat is not judged. See [Second opinion (LLM judge)](docs/site/docs/guardrails.md#second-opinion-llm-judge).
- **Works with the providers you use.** Lakera Guard, Azure AI Content Safety Prompt Shields, Azure AI Language PII, Presidio, Bedrock Guardrails and HTTP classifiers all handle tool results. HTTP classifiers can also run over a local Unix socket, with no network.
- **A ready-made filter**, *Prompt injection in tool results (quarantine)*, is added unattached so you can try it straight away.
- **Wolf Defender guardrail images.** Two open-source images (`tykio/ai-studio-guardrail-wolf` for CPU and `tykio/ai-studio-guardrail-wolf-gpu` for NVIDIA) run Wolf Defender Jev classifier by Patronus Protect (Casdo Labs GmbH) as an HTTP guardrail provider. They score tool results, tool calls, user messages and tool definitions. The CPU image runs next to the gateway with no network access and suits log-only rollouts; the GPU image takes about 20 ms per message, fast enough to enforce inline. See the [Wolf Defender guide](docs/site/docs/guardrails-wolf-defender.md).

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

- Plugin response headers (`X-RateLimit-*`, `X-LB-*`, `X-Cache*`) now reach clients on those endpoints, for every vendor. With the 2.3 plugin releases, `X-Cache-Status`, `X-LB-Upstream` and `X-LB-Pool` are on every response, streams included. `X-LB-Latency-Ms`, and on a cache miss `X-Cache-Key` and `X-Cache-Backend`, are only on responses buffered end to end (buffered `/llm/` calls, and buffered Bedrock calls on `/ai/` and `/v1`).
- Load-balancer overrides to another AWS region now work.
- Bedrock inference profiles and ARN model ids work on every `/llm/` route.
- When a plugin refuses a Bedrock call, AWS SDK clients get a proper AWS error with the plugin's message.
- New Plugin SDK helpers (`RequestModel`, `IsStreamingRequest`, `ResponseUsage`) read the model, streaming flag and token usage in every vendor's format.

This changes what plugins and filter scripts see for Bedrock traffic. Read the [Bedrock upgrade notes](#bedrock) before upgrading.

### Active-active control planes and edge resilience

AI Studio can run as two control planes in two sites over one replicated PostgreSQL, and edges keep working when either site is lost.

- **Edges with several control planes.** `CONTROL_ENDPOINTS` gives an edge an ordered list of control planes, its own site first. When its connection fails, the edge tries the others straight away and backs off only once all have failed; a configuration push in flight completes on the control plane it arrives at, and keys created just before the loss validate through the other one. `EDGE_CONTROL_FAILBACK_INTERVAL` returns edges to their first control plane when it is back, without dropping requests. The endpoint in use is in the edge's log, `/ready` and the new metric `microgateway_control_connected`. `CONTROL_ENDPOINT` works as before. See [Several control planes](docs/site/docs/edge-gateways.md#several-control-planes).
- **Edges can start without a control plane.** With `EDGE_START_FROM_LAST_KNOWN_GOOD=true`, an edge that restarts while no control plane answers starts from the configuration snapshot it last applied, which it keeps encrypted in its own database, instead of exiting. It serves, reports `config_source: last_known_good` on `/ready`, and takes the live configuration when it reconnects. Off by default: such an edge serves what it last knew until it reconnects. It needs a database that survives the restart. See [Starting without a control plane](docs/site/docs/edge-gateways.md#starting-without-a-control-plane).

### Other improvements

- **Request ids you can trace.** Every gateway request gets its own id, returned in `X-Tyk-AI-Request-ID`. It stays the same across failover attempts and appears in request logs, compliance events and edge analytics. Your own `X-Request-ID` is kept and recorded alongside it, and you can search request logs by either.
- **Secrets in plugin configuration.** Plugin config fields can reference a stored secret (`$SECRET/name`). The value stays masked in the form, and edges receive it encrypted.
- **Credential references are consistent everywhere.** `$SECRET/` references work the same way for LLMs, tools, datasources, embedders, guardrails and plugins, and `$ENV/` references for all of them except plugins, and edges only ever receive resolved values. See the [secrets guide](docs/site/docs/secrets.md).
- **Guardrail scripts can handle checker failures.** With `return_errors: true`, a failed `tyk.llm` call returns an error instead of stopping the script, so a script can decide to block when its checker is unavailable.
- **Privacy warning.** The tool and datasource forms warn when the chosen privacy level is higher than every active LLM can accept.
- **Push reminder.** After you save something that edges use, the console tells you straight away that edges need a configuration push.
- **Catalogue picker when approving a submission.**
- **Helm:** `config.logLevel` sets AI Studio's log level.
- **Helm (Microgateway chart):** `config.controlEndpoints` (a list, used instead of `controlEndpoint`), `controlFailbackInterval`, and `startFromLastKnownGood` with `startupControlTimeout` and `lastKnownGoodMaxAge` set the new edge options. The chart warns on install when `startFromLastKnownGood` is set without `kind: StatefulSet` and `persistence.enabled`, because a rescheduled pod then starts with an empty database. Rendered output with default values is unchanged.
- **Startup warning** when filter scripts are given access to the operating system (`FILTER_SCRIPT_ALLOW_OS`).
- **Warning when replicas disagree on a shared secret.** With several AI Studio replicas, the cluster status (`GET /api/v1/cluster/status`) and each replica's start-up log name the setting and the replicas when `CSRF_KEY`, `TYK_AI_SECRET_KEY`, `MICROGATEWAY_ENCRYPTION_KEY` or `GRPC_AUTH_TOKEN` differ between them, and when more than one replica runs without `CSRF_KEY` (each then generates its own key, and a form loaded from one replica is refused by another). Set the same `CSRF_KEY` on every replica.
- **The logo and favicon are stored in the database**, not on the disk of the replica that received the upload, so every replica serves them and no branding volume is needed. Existing files are imported at the first start (see [Upgrade notes](#upgrade-notes)).
- **New Prometheus metrics** for decision models (`aistudio_decision_questions_total`), native API operations (`aistudio_native_*`) and guardrails (`aistudio_guardrail_segments_total`, `aistudio_guardrail_cache_total`, `aistudio_guardrail_calls_abandoned_total`, `aistudio_guardrail_shadow_dropped_total`, `aistudio_guardrail_templates_total`, `aistudio_guardrail_template_exempt_share`).
- **One-hour prompt caching for Claude on Bedrock.** Claude Code and other Anthropic clients that ask for a one-hour prompt cache (`cache_control.ttl: "1h"`, or Claude Code's `promptCacheTtl: "1h"`) now get it when they use a Bedrock LLM through `/anthropic/`. Before, Bedrock always cached for 5 minutes, so after a longer pause the whole prompt was written to the cache again. Claude 3 models and non-Anthropic models (such as Amazon Nova) only support 5-minute caching and keep it. For now, one-hour cache writes are billed at the LLM's single cache-write price.

---

## Security

- **Microgateway admin API could be used without credentials** (also fixed in 2.2.2). `POST /api/v1/auth/token` issued admin tokens to anyone who could reach a microgateway's HTTP port. That was enough to load a malicious plugin and read or change all traffic. The route is removed, and edge gateways no longer expose `/api/v1` at all. See `CHANGELOG-v2.2.2.md` for what to check on existing gateways.
- **Internal networks are protected by default.** Outbound calls for every managed asset are now covered by the internal-address block: LLMs, Studio chat, embedders, tools, data sources and vector stores, guardrail providers, plugin registries and the MCP wizard. Previously, a submitted tool or data source could probe internal services. Each asset has an admin-only **Allow internal host** switch for legitimate internal services.
- **LLM credentials are encrypted at rest.** API keys and credential fields (such as Bedrock's AWS keys) are encrypted with `TYK_AI_SECRET_KEY`. They are no longer copied into events, object hooks, SQL logs or the edge database in plain text.
- **Credentials stay out of events and hooks.** Tool auth keys, datasource connection strings, user password hashes and tokens, App gateway keys and guardrail secrets now appear as `[redacted]` in system events and object hooks.
- **Guardrail provider credentials reach edges encrypted**, like LLM keys.
- **The API no longer returns guardrail provider keys.** A filter's literal secret connection fields (such as Lakera, Azure and HTTP `api_key`, and Bedrock `secret_access_key`) read `[redacted]` for every role, as an LLM's API key does. `$SECRET/` and `$ENV/` references and endpoints are still shown. Saving with `[redacted]` keeps the stored key; sending it on create, or with a different provider, is refused with `400`.
- **Plugin source lockdown.** `AI_STUDIO_PLUGIN_ALLOWED_SOURCES` and `PLUGINS_ALLOWED_SOURCES` restrict which plugin deployment types (`oci`, `file`, `grpc`) AI Studio and each edge accept. See the [plugin security guide](docs/site/docs/plugins-security.md#hardening-restrict-plugin-sources).
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
- **Standalone microgateways no longer store API tokens.** `api_tokens` holds a keyed hash of each token (HMAC-SHA-256, with a key derived from `ENCRYPTION_KEY`) and tokens are checked by it, so a copy of the database or a backup contains no usable token. Existing tokens are converted on the first start and keep working. Token listings never showed token values and still do not. See [Upgrade notes](#credentials-and-secrets).
- **The edge's cache of validated App keys holds hashes, not keys.** It is indexed by a keyed hash of each key (HMAC-SHA-256, with a key derived from the edge's `ENCRYPTION_KEY`) instead of the key itself, so an edge holds a key only while it handles a request that carries it.
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
- **A chat whose LLM is blocked by the internal-address block or `LLM_UPSTREAM_ALLOWED_HOSTS` showed a retryable network error**, with nothing logged. Chat now reports `upstream endpoint not permitted`, naming the setting, and AI Studio logs a warning.
- **Studio Chat failed on Claude Sonnet 5 and later**, which reject `temperature` and `top_p` (`400 temperature is deprecated for this model`), and chat titles weren't generated there. New model call settings default `temperature` and `top_p` to `0`, which means not sent, so the vendor's default applies. The title call no longer sends a temperature.

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
- **The LLM judge gave no opinion on about a third of flags with Claude Sonnet 5.5**, which added keys to its answer or wrote a sentence as the reason. The judge prompt (now `guardrail-judge/2`) tells it to answer in the fixed form only; in our tests unreadable answers fell from 34% to 4%, with no attack judged benign.
- **The LLM judge could never soften a finding from an HTTP classifier in a long message.** AI Studio dropped the position (`span`) every HTTP classifier reported, so the judge was never sure it had seen what was flagged. Spans are now kept, and moved onto the whole message when it was sent in pieces.
- **The filter form's help text** now says correctly what each kind of filter does on tools, and that Redact only logs on a response filter.

### Gateway
- **Semantic routers whose judge is Claude Sonnet 5 or later failed every judgement** with `400`. The judge call no longer sends `temperature: 0`. OpenAI and Ollama judges still run at temperature 0; Google AI judges now use the client's default of 0.5.
- **Unresolved credential references were sent to vendors as the key.** An LLM key, embedder key or REST tool auth key whose `$SECRET/` can't be found or decrypted, or whose `$ENV/` name isn't allowlisted, was sent as the reference text (or, from an edge, as `refused-credential-reference`). It is now never sent: the gateway answers `502` (`LLM credential not usable`), and Studio Chat, embedders and tool calls fail with `credential is an unresolved reference`.
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
- **Edges ignored budget syncs after the leader changed** to a replica whose clock was behind the previous leader's, new budget blocks included, until that clock caught up. The sync's sequence number now continues from the last one stored in the database, whatever the new leader's clock says, and an edge that receives a lower sequence logs a warning.
- **A replica left on a demoted Postgres primary kept failing every write** (`read-only transaction`) until it was restarted. AI Studio now notices from the first refused write, or within 5 seconds when idle, and reconnects, to the new primary when `DATABASE_URL` names both hosts with `target_session_attrs=read-write`. Its LISTEN/NOTIFY connection does the same, and no longer settles on a hot standby.
- **After a Postgres failover that lost the newest transactions, replicas could miss each other's changes.** With asynchronous replication the new primary reuses the ids of cluster events it never received, and a replica that had read the lost ones skipped the new ones, so a change made on one replica (an LLM, a plugin, a budget) did not reach the others until restart. Events are now told apart by more than their id, a replica whose position is ahead of the table moves back and says so in the cluster status, and a replica cut off from the database for longer than events are kept (15 minutes) reloads its gateway configuration, budget caches and plugins from the database.
- **Two replicas could both act as leader after a Postgres failover**, or none for up to a minute. The leader lease was judged by the database's clock, which after a failover is another server's: seconds ahead, and a second replica took the lease while the first still held it; behind, and nobody took a dead leader's lease in time. A replica now takes the lease only after watching it go unrenewed on its own clock, which holds whatever the database's clock says (for differences up to 30 s).
- **Configuration pushes could time out early or be sent twice after a Postgres failover** to a server whose clock was ahead. Open pushes now keep the time they had left, and a replica that only looks stopped by the new clock keeps its pushes.
- **Analytics and spend could be counted twice after a control-plane failure.** If the control plane stored an edge's analytics pulse but went away before acknowledging it, the edge sent the pulse again and it was stored a second time. Each pulse now carries an identity that stays the same when it is sent again, AI Studio stores it once (whichever replica receives it), and an acknowledged pulse is committed to the database rather than queued. Both AI Studio and the edge must run 2.3; pulses from older edges are stored as before.

### Console and management API
- **A partial `PATCH` wiped the fields it left out.** For example, renaming a group removed its members, and a secret could lose its name and break every reference to it. Every `PATCH` now changes only the fields you send.
- **Deep links survive sign-in.**
- **View in Catalog on a submission** opens the resource it created.
- **pgvector data sources couldn't ingest documents** (`add documents failed: unsupported options`, also in 2.2.2).
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
- **`config.ociRequireSignature: false` in the Helm chart was ignored** unless written as the string `"false"`, so signatures stayed required. An unquoted boolean, or `--set`, now turns them off.
- **The quickstart ships placeholders instead of sample secrets.**
- **Stopping AI Studio** waits for background work to finish.
- **Start-up exiting when the database is briefly away.** AI Studio on Postgres now waits for its database at start-up, up to `DATABASE_STARTUP_TIMEOUT` (60 seconds by default), instead of exiting at the first failed connection and relying on its supervisor to restart it. It also waits while only a read-only server (a hot standby, or a demoted primary) answers, since it migrates the schema at start; when the time is up it exits with a message that names the setting. Neither `/health` nor `/ready` answers during the wait, so give a Kubernetes deployment a `startupProbe` that allows at least `DATABASE_STARTUP_TIMEOUT` plus the time migrations take. `DATABASE_STARTUP_TIMEOUT=0` keeps the old fail-fast behaviour.

---

## Upgrade notes

### License (Enterprise)

- **2.3 needs a license issued for 2.3.** A 2.3 license names the product it covers and carries the limits of your deployment. A license issued for 2.2 or earlier has neither and is refused, even if it has not expired. Speak to your account manager for a new one before you upgrade.
- **Set it everywhere a license is read**: `TYK_AI_LICENSE` on every AI Studio replica (the same license on all of them) and on every Enterprise microgateway, edge or standalone.
- **What you see without it.** AI Studio stops at start-up with `license validation failed at boot: license does not include the Tyk AI Studio SKU`. An Enterprise microgateway stops with `Microgateway license validation failed at boot` and the same reason.
- **Keep your 2.2 license** until the upgrade is confirmed. You need it if you restore the backup and return to 2.2.
- **Check the limits.** The license limits edge namespaces, users with admin access, users with portal access, Apps and LLMs. If you already use more than a limit allows, nothing is removed, but new ones are refused (HTTP `409`, `QUOTA_EXCEEDED`) until usage is below the limit. Owners see the limits and current usage under **Admin > Settings > License**. See [License and Quotas](docs/site/docs/license.md).
- **Community Edition** needs no license, and none of this applies to it.

### Network access

> **Internal network addresses are blocked by default.** `LLM_UPSTREAM_BLOCK_INTERNAL` now defaults to `true` on AI Studio and the Microgateway. After the upgrade, calls fail if they reach a loopback, private, link-local or other internal address, by IP or by a hostname that resolves to one. This covers an LLM, embedder, tool, data source or vector store, guardrail provider, or a filter's `tyk.makeHTTPRequest`; for example a local Ollama at `http://localhost:11434`, an in-cluster model server, or a classifier on `10.x`. Before upgrading, do one of the following:
> - name those hosts in `LLM_UPSTREAM_ALLOWED_INTERNAL_HOSTS` (comma-separated; `.svc.cluster.local` matches every Service), which keeps blocking on for everything else (recommended);
> - have an administrator turn on **Allow internal host** on that LLM, embedder, tool, data source, guardrail, plugin or MCP server, which exempts only its own host;
> - or set `LLM_UPSTREAM_BLOCK_INTERNAL=false` to restore the 2.2 behaviour.
>
> Set this on every edge too, because each process applies its own setting. A value that isn't a boolean (such as `yes`) now stops startup; `1`, `0`, `t` and `f` are accepted.

- **Helm.** Studio's chart sets `config.blockInternalUpstreams: "true"` and takes `config.allowedInternalHosts`. The Microgateway chart's `config.blockInternalUpstreams` now defaults to `true`.
- **More ranges count as internal**: multicast, reserved, benchmarking (`198.18.0.0/15`), `192.0.0.0/24`, IPv6 site-local, NAT64, 6to4, and shared address space (`100.64.0.0/10`, used by Tailscale and some Kubernetes CNIs). A guardrail provider or plugin host in one of these ranges is refused unless allowed.
- **`ALLOW_INTERNAL_NETWORK_ACCESS=true`** still lifts every internal-address check, platform-wide. Use it for development only. Both opt-outs log a warning at startup.
- **Allow internal host** can only be set by administrators. Portal submissions can't set it, and edges older than 2.3 ignore it and keep blocking.
- See [Outbound network policy](docs/site/docs/proxy.md#outbound-network-policy).

### Credentials and secrets

- **`$ENV/` references resolve only for allowlisted names.** List the environment variables your configuration uses in `AI_STUDIO_ENV_REF_ALLOWLIST` (comma-separated, empty by default), or move those credentials to the secret store as `$SECRET/` references. This applies to LLMs, tools, datasources, embedders and guardrails. Plugin configuration resolves only `$SECRET/` references: a `$ENV/` value is passed to the plugin unresolved, and the plugin can read only the variables in its allowlist and `PLUGIN_ENV_PASSTHROUGH`.
  - Do this **before** upgrading. To list the names in use: `pg_dump --data-only "$DATABASE_URL" | grep -oE '\$ENV/[A-Za-z_][A-Za-z0-9_]*' | sort -u` (or `sqlite3 midsommar.db .dump | …`).
  - A reference that isn't allowlisted is not resolved. An LLM key, embedder key or REST tool auth key that is still a reference is never sent: the gateway answers `502` (`LLM credential not usable`), and Studio Chat, embedders and tool calls fail with `credential is an unresolved reference`. Other credentials are sent as written, so the provider answers `401`. A guardrail applies its fail mode. The console shows the credential as unresolved, even when the variable is set. See the [secrets guide](docs/site/docs/secrets.md) (Upgrading from 2.2).
- **Vendor keys and endpoints are no longer read from the environment.** An LLM, embedder or datasource with an empty key or endpoint is no longer filled from `OPENAI_API_KEY`, `ANTHROPIC_API_KEY`, `GOOGLE_API_KEY`, `HF_TOKEN`, `PINECONE_API_KEY`, `CHROMA_URL` or `OLLAMA_HOST`. Set them on the object. Don't set these variables on edges: older edges still read them.
- **Setting or changing a `$SECRET/` or `$ENV/` reference needs `secrets:read`** (`403` otherwise). Keeping an existing reference needs nothing extra. Submissions and plugins can't set references.
- **LLM credentials are encrypted at rest.** On first start, AI Studio encrypts every literal LLM API key and credential field with `TYK_AI_SECRET_KEY`.
  - Without `TYK_AI_SECRET_KEY`, values stay as written and AI Studio logs a security warning. Setting the key later encrypts them at the next start.
  - If the key changes or goes missing, everything encrypted with it becomes unreadable: stored LLM credentials and the values in the secret store (so `$SECRET/` references fail too), as well as other encrypted settings. Calls that need them fail. Restore the key, or re-enter the LLM credentials and the secret values. `$ENV/` references are unaffected.
- **SSO needs `TYK_AI_SECRET_KEY`.** Without it, every SSO login is refused, and AI Studio warns at startup.
- **Standalone microgateway API tokens are hashed at rest, one way.** The first 2.3 microgateway to start on a database replaces every stored API token with a keyed hash (made with `ENCRYPTION_KEY`) and a redacted reference. Existing tokens keep working, and nothing that lists tokens changes (listings never included token values).
  - The tokens can no longer be read from the database. A version before 2.3 accepts none of them afterwards: upgrade every microgateway that shares the database together, and take a backup first if you may need to go back.
  - Changing `ENCRYPTION_KEY` now invalidates every API token.
  - The unused `token_cache` table, where present, is dropped.
  - An Enterprise microgateway whose license is missing, expired or not valid exits before it opens its database (the license used to be checked after the migration). So a 2.3 microgateway that will not start has not converted the tokens, and the previous version can still be started on that database. `./microgateway -migrate` run as a separate step now needs the license too: give it the same `TYK_AI_LICENSE` as the gateway, or it exits without migrating.
- **SQL logs no longer contain values.** Set `DB_LOG_SQL_PARAMS=true` only for local debugging, never in production.
- **The quickstart needs your own secrets.** Replace the `__REPLACE_WITH_…__` placeholders in its env files before the first start. Otherwise AI Studio refuses to start, naming the setting that still holds a placeholder (`TYK_AI_SECRET_KEY`, `MICROGATEWAY_ENCRYPTION_KEY` or `GRPC_AUTH_TOKEN`), and the Microgateway stops with a key-length error. If your deployment uses values copied from an earlier quickstart or documentation sample, replace them with values you generate.

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
- **Cached plugins are downloaded once more** after the upgrade, to record their hash. The registry should be reachable then. With signatures required, AI Studio and each edge log a warning per plugin (`Cached OCI plugin failed its integrity check, fetching it again`, `cache corrupted … no recorded binary hash`), followed by a passed signature check. This is expected, and happens again if an older version runs on the same plugin cache.
- **Plugins no longer see credentials.** LLM, tool, datasource and user credentials appear as `[redacted]` in system events and object hooks. A `before_*` hook that returns a value still `[redacted]` keeps the stored one.
- **Edge response plugins now receive the LLM's `vendor`.**
- **Plugin order on edges.** Edges now run an LLM's plugins in the order set in AI Studio, not install order. Check the order on each LLM with more than one plugin before upgrading edges.

### Bedrock

Plugins attached to a Bedrock LLM now run on `/ai/`, `/v1`, routers and `/anthropic/`, and filters there now read the Bedrock Converse format. Before upgrading:

- **Upgrade the shipped plugins** to advanced-llm-cache 1.3.1, llm-cache 1.3.1, llm-load-balancer 1.2.1, rate-limiter 1.2.1 and llm-firewall 1.2.0. Earlier versions mishandle Bedrock: caches mix up models and cache errors, token rules count no Bedrock tokens, firewall model rules never match, and the load balancer counts errors as successes.
- **Check your own plugins.** On Bedrock there's no `model` in the body (it's in the path), usage is camelCase, and streaming is chosen by the path (`converse-stream`). Use the SDK helpers `RequestModel`, `IsStreamingRequest` and `ResponseUsage`.
- **Check custom filter scripts on Bedrock LLMs.** They get `input.format == "bedrock"` and a Converse `raw_input`, not the OpenAI or Anthropic shape the client sent. Read `input.messages`, or handle Converse. Guardrails and the redaction helpers already handle it.
- **Data collectors** see Converse bodies for Bedrock traffic on these endpoints.
- **No automatic AWS SDK retries.** Each failover step tries a Bedrock call once, and failover handles 5xx and 429 errors.
- **Status codes.** Refusals reach the client with their real status: `400` for a guardrail block, the plugin's own status for a plugin block, and `403` for a budget refusal.
- **Request logs** show the LLM's name as the model for buffered Bedrock calls on these endpoints. Chat records keep the model id.
- **AI Studio's embedded gateway** applies budgets, filters and analytics to Bedrock the same way.

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
- **Apps start with no guardrail capability attested**, which counts as Acting. Guardrail exceptions for an App whose tools only read need its capability attested first; the exception dialog and the App page offer AI Studio's suggestion. Softening tool results, tool calls or tool definitions is possible only for Read-only and Reads with tools Apps. Editing a bound tool's OpenAPI spec or an MCP server's annotations does not clear an attestation: attest the App again after such an edit.
- **Guardrail alerts from edges arrive with the analytics pulse**, and changes to exceptions reach edges at the next configuration push. On an edge, an expired exception stops applying after AI Studio's hourly expiry check plus the next push.

### Edges

Upgrade edges together with AI Studio. Edges older than 2.3:

- **block guardrailed traffic**, even under `fail_mode: open`, because guardrail credentials now reach edges only encrypted;
- **fail Bedrock calls**, because AWS credentials now reach edges only encrypted (an old edge never falls back to its own AWS credentials);
- **can't serve decision models** (the `systemone` vendor), and don't record OpenAI decision calls;
- **don't scan tool results**, and block where a 2.3 edge would quarantine;
- **don't enforce LLM budgets**, ignore **Allow internal host**, and don't verify resource ids minted by AI Studio.

After the upgrade:

- **Edges resync when they reconnect** to the upgraded AI Studio, and normally show In sync. An edge that starts without its local configuration shows Pending until you push the configuration.
- **Duplicate tool or LLM names** stored by earlier versions are left out on edges (the lowest id keeps the name) until renamed. AI Studio logs which entry to rename, and a push succeeds with a warning that lists it as skipped; the edge still shows In sync.
- **An LLM or tool that needs a filter or plugin from another namespace** is left out on that namespace's edges. Move the filter or plugin to the global namespace or the LLM's namespace.
- **Edge errors now reach AI Studio's request logs.** Policy blocks and errors on edges appear as request log rows with no spend, so request counts include them.
- **The request id header is `X-Tyk-AI-Request-ID`.** Edges no longer overwrite `X-Request-ID`.
- **Edges keep their last applied configuration snapshot in their own database**, in a new table `applied_snapshots` (one row, replaced on every applied configuration). It is encrypted with a key derived from the edge's `ENCRYPTION_KEY` and can be as large as a snapshot (up to `GRPC_MAX_MESSAGE_SIZE`, 16 MB by default), so allow for that on the edge's volume.

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

- **Rolling back means restoring the backup.** Take a backup before upgrading. An older AI Studio does not refuse a database 2.3 has migrated: it starts with one warning (`The database schema is version 34 …, newer than this build's …; keeping the newer record`) and sends the encrypted form of every literal LLM key as the key itself, to vendors and to edges, so those calls fail with `401`. LLMs keyed with `$SECRET/` or `$ENV/` references keep working. Only an older headless replica refuses the database. To roll back, stop AI Studio and restore the backup; anything changed since the upgrade is lost.
- **Upgrade replicas together.** Stop every older AI Studio replica before the first 2.3 replica starts. An older full replica sharing the database joins the cluster, can become the leader, and serves the encrypted keys. The Helm chart's default `Recreate` strategy does this; don't use `RollingUpdate` for the upgrade to 2.3.
- **The leader lease and mixed versions.** A replica older than 2.3 still judges the leader lease by the database's clock, so after a Postgres failover to a server whose clock is ahead two replicas can both act as leader until every replica runs 2.3. A 2.3 replica also waits about 20 seconds before taking a lease that an older replica released. Both end when the last older replica stops: one more reason to upgrade replicas together.
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

- **Model call settings saved before 2.3 keep their `temperature` and `top_p`.** For Claude Sonnet 5 and later, set both to `0` (not sent), or Studio Chat on those models fails with `400`.
  - The call-settings form's default preset now leaves `temperature` and `top_p` unset (`0`, not sent), so the vendor's default applies. One exception: on OpenAI models other than reasoning models (`gpt-4o` and similar), AI Studio's client sends `temperature: 0` when none is set, so Studio Chat on a call setting created from the default preset runs at temperature 0. Set a temperature in the call setting for a different value; non-zero values are sent as before. This affects Studio Chat only: gateway and API traffic don't use call settings.
- **Chat sessions send their history.** On LLM settings without a max length, a session now sends the conversation (up to the newest 100 messages), not just the latest message. Set **Max Length** to cap input.
- **Chat costs change.** Anthropic turns with tool calls or thinking drop to the correct amount, cached tokens are billed at the cache price, and Anthropic chat records' total tokens include cache tokens.
- **Tool and LLM names must be unique** ignoring case, spaces and punctuation. Duplicates stored earlier stay editable.
- **Decision-only LLMs** don't appear in chat, agent, router and wizard pickers. The failover picker only offers compatible models.
- **Edge gateways answer `404` on `/api/v1`**, and the `mgw` CLI no longer works against an edge; configure edges from AI Studio. A standalone microgateway keeps its management API, but create tokens with `POST /api/v1/tokens` or `microgateway -create-admin-token`. On an edge (`GATEWAY_MODE=edge`), `-create-admin-token` now fails with a clear error instead of minting a token nothing accepts.
- **advanced-llm-cache 1.3.0:** Redis `tls_enabled` now takes effect, so check that your Redis accepts TLS. Use `tls_insecure` instead of `tls_skip_verify`. The cache starts empty after the upgrade.
- **MCP analytics need traffic logs on the proxy.** Proxies created outside AI Studio record nothing until `trafficLogs` is enabled. Tyk Pump needs `dont_purge_uptime_data: true` unless you run an uptime store. Pump drops a batch it can't deliver once its retries run out, so MCP records sent while AI Studio is restarting can be lost: use `max_retries: 8`, as in the AI Studio snippet.
- **Custom logo and favicon move into the database.** Start the first 2.3 instance with the branding directory (`BRANDING_STORAGE_PATH`, default `./data/branding`) still in place: it imports the files once and logs `Branding: imported the logo from …`. The directory is not used after that. An instance started without the files serves the default logo until one that has them starts, or until you upload the logo again. A custom logo or favicon that cannot be found is now answered with the default one, not `404`.
- **Agent tracing content ceiling.** If `AGENT_TRACING_CONTENT_CEILING` is unset, it is `metadata`, which overrides Apps set to Redacted or Full. Set it to the highest mode you want to allow.
- **`AGENT_TRACING_BAGGAGE_ALLOWLIST`** (default `session.id`, all editions) limits the W3C `baggage` header sent upstream to the listed keys. Set it to the keys your providers rely on, or `none`.

### For plugin authors and Go embedders

- `models.LLMVendorProvider` gains `ProvidesChat()` and `DecisionEndpoint()`, so custom vendor implementations must add them.
- `CallLLM` returns `FailedPrecondition` when the agent's App has no credential, instead of a `401`.
- Plugins and hooks see the new request id format (`tyai_` + UUIDv7) in `request_id` and `PluginContext.RequestID`.
- The plugin, management and config-sync protocols change additively (new fields and a `CallDecision` RPC).
- When embedding with `pkg/studio`, `AppConf.FilterLimits` applies only to names in `config.FilterLimitNames`, which now include the `GUARDRAIL_*` settings.
- Analytics sinks can implement `analytics.MCPCallRecorder` to receive MCP calls. Existing handlers compile unchanged.
- `studio.OpenDatabase` waits up to `AppConf.DatabaseStartupTimeout` for a writable Postgres database. `config.LoadFrom` sets it to 60 seconds; an `AppConf` built by hand has 0, one attempt as before.

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

The Helm chart adds `config.logLevel` (AI Studio's `LOG_LEVEL`), and `config.pluginAllowedSources` (AI Studio) and `config.pluginsAllowedSources` (Microgateway chart) for the plugin sources below.

**Plugin signatures** (Enterprise; an invalid value stops startup):

| Variable (AI Studio / Microgateway) | Default | Purpose |
|---|---|---|
| `AI_STUDIO_OCI_REQUIRE_SIGNATURE` / `OCI_PLUGINS_REQUIRE_SIGNATURE` | `true` / `true` | Require a valid signature on OCI plugins (AI Studio's default was `false`) |
| `AI_STUDIO_OCI_TLOG_POLICY` / `OCI_PLUGINS_TLOG_POLICY` | `required` | `required`, `optional` (checked when present) or `disabled` (key only) |
| `AI_STUDIO_OCI_TRUSTED_ROOT_FILE` / `OCI_PLUGINS_TRUSTED_ROOT_FILE` | empty | A Sigstore `trusted_root.json` for a private Sigstore deployment |

**Plugin sources** (an invalid value stops startup):

| Variable (AI Studio / Microgateway) | Default | Purpose |
|---|---|---|
| `AI_STUDIO_PLUGIN_ALLOWED_SOURCES` / `PLUGINS_ALLOWED_SOURCES` | empty (all) | Plugin deployment types accepted: `oci`, `file`, `grpc` (comma-separated). An edge applies its own list to the plugins AI Studio pushes |

**Active-active control planes and edge resilience** (all optional):

| Variable | Where | Default | Purpose |
|---|---|---|---|
| `CLUSTER_NODE_LABEL` | AI Studio | `studio` | Names a replica (for example its site) on the cluster status and in the Edge Gateways "Held by" column. At most 64 characters; an invalid value stops startup |
| `DATABASE_MAX_OPEN_CONNS` | AI Studio | no limit | Most Postgres connections a replica opens |
| `DATABASE_MAX_IDLE_CONNS` | AI Studio | `2` | Idle Postgres connections kept open |
| `DATABASE_CONN_MAX_LIFETIME` | AI Studio | `30m` | Close and replace connections of this age, so they also move after a planned switchover (`0`: never) |
| `DATABASE_CONNECT_TIMEOUT` | AI Studio | `10s` | Time for one connection attempt, unless `DATABASE_URL` sets `connect_timeout` |
| `DATABASE_STARTUP_TIMEOUT` | AI Studio | `60s` | How long start-up waits for a Postgres database that answers and takes writes before exiting (`0`: one attempt) |
| `CONTROL_ENDPOINTS` | edge | unset | Ordered, comma-separated list of control planes; `CONTROL_ENDPOINT` is a list of one (setting both is a start-up error) |
| `EDGE_CONTROL_FAILBACK_INTERVAL` | edge | off | How often an edge that is not on its first control plane tries to return to it |
| `EDGE_START_FROM_LAST_KNOWN_GOOD` | edge | `false` | Start from the stored configuration snapshot when no control plane answers at start-up |
| `EDGE_STARTUP_CONTROL_TIMEOUT` | edge | `30s` | How long the edge keeps trying its control endpoints before it uses the stored snapshot |
| `EDGE_LAST_KNOWN_GOOD_MAX_AGE` | edge | `72h` | The longest an edge may have been without a control plane and still start from the stored snapshot (`0`: no limit) |

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
| llm-cache | 1.3.1 † | Works correctly with Bedrock (errors never cached, token savings counted). No longer replays other plugins' headers on a hit. `X-Cache-Status` is set on every response, streams and `/ai/` and `/v1` included. Stability and logging fixes |
| llm-load-balancer (Enterprise) | 1.2.1 † | Counts Bedrock errors as failures, and client errors (400, 413, 422) no longer count against an upstream. Fixes a selection race. Validates and redacts upstream URLs. `X-LB-Upstream` and `X-LB-Pool` are set on every response, streams and `/ai/` and `/v1` included. Pools saved from the plugin's pages now load on edges and after a restart (with 1.2.0 they didn't, and edges sent every request to the LLM's own endpoint) |
| advanced-llm-cache (Enterprise) | 1.3.1 † | Works correctly with Bedrock. `X-Cache-Status` is set on every response, streams and `/ai/` and `/v1` included. Cache keys include the LLM, vendor, path and model, so the cache starts empty after the upgrade. Redis TLS now applies, and the Redis password is masked. Enter the Redis password itself: a `$SECRET/` reference there isn't resolved (it is nested), so Redis refuses it and the plugin quietly falls back to an unshared in-memory cache. Needs the `apps.write` scope |
| rate-limiter | 1.2.1 † | Token rules count Bedrock, Google and streamed usage. Concurrency slots are given back for requests answered before the vendor. Masked Redis password. Changing a rule on the Rules page no longer reverts the settings saved in the configuration form (with 1.2.0, the Redis backend, URL and password, `fail_open` and the window size went back to the values the plugin started with, and edges fell back to per-instance counters after the next push) |
| llm-firewall | 1.2.0 † | Model rules match Bedrock model ids, inference profiles and ARNs. Inspects all prompt text, including Bedrock text documents. Optional `fail_closed` |
| llm-price-sync | 1.1.1 | Refuses internal fetch URLs. Reliability fixes |
| github-rag-ingest | 1.1.1 | Secret references for credentials. Cancelling a job stops it. Masked `vault_token`. Hardened clone cache. Needs AI Studio 2.3.0 for secret references and `PLUGIN_ENV_PASSTHROUGH` |
| oauth2-client-credentials (Enterprise) | 1.3.3 † | Rebuilt on the 2.3 SDK: with 1.3.2, a 2.3 gateway refuses authenticated requests over 4 MiB. Better JWKS caching |

---

## Known issues

- **Model and Semantic Routers don't route decision calls yet.** They answer `400`.
- **Studio Chat can't use a Bedrock LLM.** A chat on one is saved, but its session fails to start. This is not new in 2.3.
- **Decision models haven't been tested against a self-hosted llama.cpp server.** They are tested against OpenAI `gpt-6-luna` and TypeSafe `jev-latest`.
- **Some OpenAI ids aren't ownership-checked.** A `file_id` in an `input_file` or `input_image` part, and an `item_reference` id, are passed to OpenAI unchecked.
- **`input_file` content isn't screened** by filters.
- **Standalone microgateways sign resource ids with the vendor API key**, so rotating that key invalidates outstanding ids.
- **After a Postgres failover to a server whose clock is ahead, replicas can show as stopped for up to 5 seconds** in the cluster status and the edge owner view. It corrects itself; pushes and the leader lease no longer act on it.
- **A dropped database connection at the wrong moment can lose a batch's usage.** This is rare, and it undercounts; it never double-charges.
- **The proxy-log API doesn't return `service_tier` yet.** The tier report and chat records have it.
- **What guardrails don't inspect:** binary documents (PDF, Word) and images; files referenced by id and conversation state held by the vendor; results of tools the vendor runs within one call, until the client sends them back; and datasource context in Studio chat.
- **The built-in injection heuristics are a baseline.** They are deliberately narrow on tool results to avoid false positives, so paraphrased attacks get through. Pair them with Lakera Guard, Azure Prompt Shields, Bedrock Guardrails or the Wolf Defender images for robust coverage.
- **Script filters see tool results as sent**, not decoded as guardrails do. An empty `input.segments` with a non-empty `raw_input` means an unreadable body, which a script that screens tool results should block.
- **Large tool results can wait on slow hosts** behind a flood of large scans.
- **Keys that differ only in case** can still be read differently by a script filter and the upstream. Guardrails refuse such bodies.
- **Dashboard-created MCP proxies don't set `stripAuthorizationData`**, so AI Studio-minted Tyk keys are forwarded to third-party upstreams. Gateway connections set it.
- **Rate limiter concurrency slots can be held for up to ten minutes** by a request that ends without a response.
- **A revocation does not reach an edge that is cut off from every control plane.** An edge started from last-known-good (`EDGE_START_FROM_LAST_KNOWN_GOOD`), like one running through an outage, serves the configuration it last knew: a removed LLM or App, or a key revoked meanwhile, keeps working there until the edge reconnects. `EDGE_LAST_KNOWN_GOOD_MAX_AGE` (72 h) bounds how long an edge may have been cut off and still start this way.

---

Full list of changes: [`v2.2.1...v2.3.0`](https://github.com/TykTechnologies/ai-studio/compare/v2.2.1...v2.3.0)
