# Agent Call Tracing (Enterprise)

Agent Call Tracing turns the gateway's LLM, tool, MCP and datasource traffic into **sessions → turns → span trees**. Admins can read a session as a transcript or a timeline, export it as JSON or Markdown, and send it to any OTLP backend. It needs no SDK in the agent: the gateway already sees every call. Headers the client declares make traces better, but they are not required.

- Full specification: `enterprise/features/agenttracing/SPEC.md`. Its preface holds the decisions that amend the draft.
- Operator and developer guide: `docs/site/docs/agent-tracing.md`.
- Epic: `midsommar-clean-jyx`.

The whole feature is Enterprise (D2). Community builds keep inert hooks: the capture contract, the proxy hooks, the models and settings, and an API that answers 402. Two hooks stay active in every edition, with tracing on or off: the baggage allowlist and the per-LLM traceparent opt-out.

## Decisions

These are from the SPEC preface; where the original draft disagrees, they win.

| | Decision |
|---|---|
| **D1** Storage | Portable GORM tables on SQLite and Postgres. B-tree indexes and join tables replace arrays and GIN. Retention is a chunked delete under a leader lease. Postgres partitioning is a later optimisation (jyx.48). |
| **D2** Edition | Enterprise only, under the existing licence (no new entitlement). Core holds inert hooks; ENT holds the assembler, correlation, content and redaction, keys, query and export, OTel, and the RBAC seed and audit wiring. |
| **D3** Transport | The embedded gateway writes captures to a staging table through a `CaptureRecorder`. Edges send them in `AnalyticsPulse` field 15, with their own sub-buffer flushed every 10 s or 500 events, in chunks of at most 4 MB. Each event has an idempotency key (edge, span, seq) that the hub deduplicates on. The assembler waits a settle window before consuming. Loss on an edge restart is accepted and counted. |
| **D4** Keys and scope | Everything is scoped by `app_id`, with `team_id` denormalised. HKDF-SHA256 derives one HMAC key (fingerprints, correlation index) and one AES-256-GCM payload key from `AGENT_TRACING_KEY`, falling back to `TYK_AI_SECRET_KEY`. Without a key, tracing does not start. A 1-byte key id allows rotation (`AGENT_TRACING_KEY_PREVIOUS`). Edges receive only the HMAC key, encrypted in the config sync. Payloads are sealed at the hub. |
| **D5** Product defaults (jyx.5) | Retention is 30 d for metadata and 7 d for payloads. Turns close after 5 min idle, sessions after 30 min; both are overridable per App. Inferred sessions set `session.id` on export (switchable per exporter); `gen_ai.conversation.id` is client-supplied only. The Portal shows metadata only unless an admin opts an App in (`owner_may_view_content`). There is no server-side PDF. Tyk Gateway data arrives through the Tyk Pump HTTP sink (epic bv0), not an OTLP ingest. traceparent goes upstream by default, with a per-LLM opt-out; baggage is cut to an allowlist. Runaway-session alerts are Phase 2. |

Also recorded:
- **FIPS readiness.** Stdlib crypto only.
- **End-user pseudonyms (jyx.174, Martin's option B).** Without `trace-content:read`, `end_user` and `external_id` are HMAC pseudonyms (keys.PseudonymizeUserID / PseudonymizeExternalID, separate domains) on every read and export. jsonv1 leaves them out unless the caller passes `Identity` or `RawIdentity`, so a new read fails closed. Exact-id filters take the raw id only. Studio users are not masked.
- **Content ceiling and default (§5.5 amendment, jyx.172).** `AGENT_TRACING_CONTENT_CEILING` bounds every App, default and header. `AGENT_TRACING_CONTENT_DEFAULT_MODE` applies to Apps without their own mode. Unset, the ceiling is the higher of the default and the request override. The resource maximum and the store cap bound everything. Both settings are environment-only until jyx.79, and edges get the ceiling in the snapshot (missing is off).
- **Header precedence (§4.2).** For session, user and agent, explicit `X-Tyk-*` headers and compatibility headers win over W3C baggage. `traceparent` still wins for the trace id and the parent span id.

## Data flow

```
 client ──► gateway (embedded in Studio, or an edge microgateway)
             │  proxy hooks capture one Event per call (pkg/agenttrace)
             │  content pipeline: mode, secret strip, redact, truncate (ENT)
             ▼
   embedded: analytics CaptureRecorder ─► agent_trace_capture_events (sealed)
   edge:     own outbox ─► AnalyticsPulse field 15 ─► hub ingest ─► same table
             ▼
   assembler (lease agent_trace_assembler): settle ─► sessions / turns / spans,
             correlation, payloads, links  ─► read API + UI, JSON/Markdown export
             ▼
   OTel worker (lease agent_trace_export) ─► OTLP exporters;  metrics ─► Prometheus/OTLP
```

### Capture (core, `pkg/agenttrace`, `proxy/agent_trace_*.go`)

- **Contract.** `pkg/agenttrace` defines the contract. It must not import proxy, models or GORM.
  - `Begin`/`Fork`/`From` attach a recorder to the request context.
  - `SetSink` sets where finished events go.
  - `CaptureRaw` hands raw parts to the registered content processor.
- **Request start.** `proxy/proxy.go` `requestStartMiddleware` calls `beginRequestCapture` (`agent_trace_identity.go`), which fixes the identity: App, user, credential and declared context.
- **LLM calls** (`agent_trace_llm.go`).
  - `captureLLM` runs on the analytics goroutine and copies usage and cost from the chat record.
  - `/ai/` outer calls make their inner hops attempts, through an internal span header.
  - Bedrock is handled in `agent_trace_bedrock.go`.
- **Tools** (`agent_trace_tools.go`).
  - MCP: `_meta.traceparent` is injected, and the recorder forks on SSE.
  - REST tool operations, datasources and plugin endpoints are captured too.
- **Gateway events.** Retries, fallbacks, filter and guardrail decisions, budget and auth outcomes come from `agent_trace_events.go`. They become `gateway_event` child spans.
- **Propagation.**
  - `injectUpstreamTrace` sends `traceparent` upstream, honouring the per-LLM opt-out.
  - `pkg/tracing/baggage.go` cuts baggage down to the allowlist.
- **Declared context.** Extraction lives in `pkg/agenttrace/extract/`; declared session ids and harness headers are in `pkg/agenttrace/declared/`.

### Embedded staging (core)

- `analytics/capture.go` batches events: 500 per batch, flushed every 1 s.
- Before writing, `pkg/agenttrace/staging.go` runs the content processor, then seals the whole event with AES-GCM.
  - If no sealer is installed, it fails closed: content is stripped and the event is stored plain.
- Rows are deduplicated by their unique idempotency key.

### Edge transport

- **Outbox.** `microgateway/internal/plugins/agent_trace_capture.go` keeps its own outbox, separate from billing analytics (10 s, 500 events, 4 MiB chunks). It makes one last flush attempt on stop.
- **Hub ingest.** `grpc/agent_trace_ingest.go`:
  - checks the edge's namespace and each App's scope;
  - discards captures while tracing is off;
  - fails a capture-only pulse with `Unavailable`, so the edge retries.
- **Edge settings.** Edges receive in the config snapshot (`proto/common.proto`), only while tracing runs on the hub:
  - `AgentTracingKeys`: the HMAC key, encrypted like LLM keys and held in memory only.
  - `AgentTracingContentPolicy`: the global content settings and `assembled_export_active`.
- **Edge content.** `microgateway/internal/services/agent_tracing_content.go` builds the edge content pipeline from that policy plus App and resource overrides in the edge DB. Content is redacted on the edge, before it enters the pulse. Without a policy, the edge captures no content.

### Assembly (ENT, `enterprise/features/agenttracing/assembler`)

- **Lease.** Runs only on the holder of lease `agent_trace_assembler`.
- **Each pass** reads settled staged rows (default settle window 30 s) in batches of 500. For each row it:
  - assigns a session: declared, or R6-inferred from App + credential;
  - assigns a turn;
  - builds the spans;
  - runs correlation (`correlate/`, rules R1–R3; R4/R5 are Phase 2);
  - computes the rollups.
- **One transaction per batch.** Spans, links, payloads, agents, traces and sessions are written, and the staged rows deleted, together. Reads come first and writes after, so the pattern is safe on SQLite.
- **Background jobs.**
  - `CloseIdle` closes turns and sessions after their idle timeouts. A turn that asked for tools and never saw results is closed as `abandoned`.
  - `Retain` deletes, in chunks, rows past their retention. It runs every 10 min.
- **Correlation index.** `keystore.go` stores HMAC keys in the DB, with an LRU in front.

### Read, export, UI

- **Routes and handlers.** Routes are under `/api/v1/agent-tracing`. Core handlers are in `api/agent_tracing_handlers.go` and `api/agent_tracing_content_handlers.go`; ENT ones in `readapi.go`, `contentapi.go`, `previews.go` and `exportapi.go`.
  - Every response is `Cache-Control: no-store`.
  - Personal data, such as the end user or an external id, is sent only in POST bodies, never in URLs.
- **Content access is audited and fails closed.** Payload reads, previews, content search, exports and purges need the ENT audit trail to accept a record. If it doesn't: reads return 503, previews return `audit_unavailable`, and purges are refused.
- **Export.**
  - JSON v1 is built in `jsonv1/`; its schema is `pkg/agenttrace/traceschema/agent-trace-v1.json`.
  - Markdown is built in `export/markdown.go`.
  - The export cap is 64 MiB.
- **UI.**
  - Pages: `AgentSessions.js` and `AgentSessionDetail.js`.
  - Components (`components/agent-tracing/`): Transcript, Timeline, span drawer and find bar.
  - Settings sections in the App and resource forms: `components/common/TracingSettingsSection.js`.
  - Nav entry: `api/nav.go`, shown only when `feature_agent_tracing` is set.
  - Captured Markdown never loads images or raw HTML (`CapturedMarkdown`).

### OTel export and metrics (ENT, `otelexport`)

- **Worker.** Exports settled turns under lease `agent_trace_export`, poll 5 s, 100 turns per pass. `traces.exported_at` marks turns already sent; spans that arrive later are stored but not resent.
- **Attribute names** all come from `mappings/genai-1.41.yaml`. The docs table is generated from it (`make agent-tracing-docs`) and checked in CI.
- **One call, one span.** The assembled LLM span is exported under the gateway span id. While export is active, the proxy leaves out its own live span for captured requests. Hub replicas set this when the worker starts; edges get it through `assembled_export_active` (jyx.90).
- **Metrics.**
  - Core instruments: `metrics/agenttracing.go` and `metrics/agentmetrics.go`.
  - The assembler records them after each commit.
  - Exporter entries with `signals: [metrics]` also push them over OTLP from every replica.

## Tables (`models/agent_trace.go`)

The tables are migrated with the analytics models. None uses `gorm.Model` or soft delete.

| Table | Holds |
|---|---|
| `agent_trace_sessions` | One conversation or job: App, team, user, end user, external id/source, declared/inferred, status, counters, tokens, cost, weakest confidence, content mode, retention. |
| `agent_trace_session_models` / `_tools` / `_agents` | Name join tables for list filters. |
| `agent_trace_traces` | One turn. PK (app_id, trace_id). Status, rollups, `exported_at`. |
| `agent_trace_spans` | One call: LLM inference, tool/MCP, datasource, plugin endpoint, gateway event, agent grouping, inferred client tool. Includes the parent edge with its confidence and rule. |
| `agent_trace_span_links` | caused_by, continues, subagent_of, retry_of, provisional_context and candidate links, each with confidence and rule. |
| `agent_trace_agents` | Declared or inferred agents, with rollups. |
| `agent_trace_payloads` | Captured content per part, AES-256-GCM sealed and bound to app, span, part and row. Has `expires_at`. |
| `agent_trace_correlation_keys` | HMAC key hashes for correlation. Has `expires_at`. |
| `agent_trace_capture_events` | D3 staging. Unique idempotency key. The event is sealed, or stored plain with its content stripped when no sealer is installed. |

Settings live in:
- `apps.tracing` (`models/agent_trace_settings.go`);
- `tracing_content_mode_max` on LLMs, tools and datasources;
- `llms.tracing_propagation_opt_out`.

Changing any of them needs `traces:write` (jyx.99).

## Cluster behaviour

- **Leased work:** `agent_trace_assembler` (assembly, idle closure, retention, the active-sessions gauge) and `agent_trace_export` (the OTel worker).
- **Every replica:**
  - embedded capture and staging;
  - edge pulse ingest;
  - the read, content and export API;
  - the content processor and override cache, invalidated by `system.*` events;
  - OTLP metric push;
  - the export-active flag.

## Known gaps

**Not built yet, by bead**
- `hashed` content mode (the UI offers it; it behaves as `metadata`), reference/inline content export, mirrors, and the `genai-dev-2026-10` profile: jyx.43.
- Correlation rules R4 and R5: .38, .39.
- Tree view: .40. Live SSE view: .41. Portal views: .42.
- OTLP JSON, an HTML report, bulk export: .44.
- Tyk Gateway MCP ops enrichment: .45.
- An OTLP receiver: .46.
- Studio Chat and agent-plugin tool capture: .47.
- Postgres partitioning: .48.
- Coverage panel: .49.
- Runaway-session alerts: .53.
- Licence expiry at runtime: .56.

**Behaviour today**
- Late inference results are not added as links.
- There is no OTel Collector nightly compatibility job.
- `tyk_tracing_capture_dropped`, `tyk_tracing_export_attributes_dropped` and `tyk_tracing_queue_depth` are on Prometheus only, not the OTLP meter.
- The hub bounds an edge's gateway events at ingest (jyx.171): it keeps only known types and outcomes, at most 64 per span, and drops an over-long name. Drops count as `tyk_tracing_capture_dropped{reason="edge_gateway_event_bounds"}`. The OTel export passes a stored `tyk.*` attribute only if `weaver/model/tyk.yaml` lists it.
- `owner_may_view_content` is stored but not yet consumed (Portal views, .42).
- The default Auditor role does not get `trace-content`. This is pending a product confirmation; restoring it is one line in `enterprise/features/rbac/seed.go`.
- Several configuration variables are parsed but not yet used. The operator guide lists the ones that work.
