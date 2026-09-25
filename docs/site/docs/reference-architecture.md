---
title: "Reference Architecture"
weight: 1
---

# Reference Architecture

This page describes the recommended production deployment of Tyk AI Studio: one **control plane** (AI Studio) managing a fleet of **data-plane** gateways (the Microgateway, running in edge mode). It shows how the pieces connect, what happens when one of them fails, and how to size the data plane using the measured performance of v2.2.0-rc10.1.

Two deployment shapes are left out on purpose:

- **Standalone Microgateway** (no control plane), and
- **Studio's embedded gateway** used as the API gateway for applications.

Both work, but neither gives you central configuration with independently scaled traffic handling. If you are planning a production build-out, start from the model on this page.

> Edge management and multiple namespaces are **Enterprise Edition** features. See [Edge Gateways](./edge-gateways.md) for the management UI and API.

## The short version

- **Studio is the brain, not the pipe.** Application traffic to LLMs goes through the Microgateways. Studio holds configuration, identity, the portal, chat and analytics, and it is only on a request's path when an edge meets an access token it has not seen in the last five minutes.
- **Edges connect out; Studio never connects in.** Each edge opens one long-lived gRPC connection to Studio on port 50051. Edges can therefore sit in private networks, other clouds or on-premises with outbound-only access.
- **Scale the data plane horizontally.** One 4-vCPU edge sustained **~365 streaming requests per second** (about 1,600 concurrent streams) or **~1,000 short non-streaming requests per second**, adding **0.5–0.6 ms at p50**. Add nodes behind a load balancer for more.
- **Run one Studio instance and make its database highly available.** In 2.2 the control plane is designed to run as a single active instance. Its availability comes from a managed, replicated PostgreSQL and a fast restart, not from multiple replicas. Running edges keep serving while Studio is down, but new edges cannot start.

## The picture

```
      Applications / agents / SDKs                  Admins, developers, chat users
                  │  HTTPS                                        │  HTTPS
                  ▼                                               ▼
       ┌─────────────────────┐                        ┌─────────────────────────┐
       │  Load balancer (L7) │                        │  Ingress (sticky, TLS)  │
       └──────────┬──────────┘                        └────────────┬────────────┘
                  │ :8080                                          │ :8080
    ┌─────────────┼──────────────┐                                 ▼
    ▼             ▼              ▼                   ┌──────────────────────────────┐
┌────────┐   ┌────────┐     ┌────────┐               │  AI Studio (control plane)   │
│ Edge 1 │   │ Edge 2 │ ... │ Edge N │               │  Admin UI · Portal · Chat    │
│ SQLite │   │ SQLite │     │ SQLite │               │  REST API · gRPC :50051      │
└───┬──┬─┘   └───┬──┬─┘     └───┬──┬─┘               └───────┬──────────────┬───────┘
    │  │         │  │           │  │                         │              │
    │  └─────────┼──┼───────────┼──┼── gRPC/TLS :50051 ─────►│              ▼
    │            │  └───────────┼──┼── (edge dials out) ────►│      ┌───────────────┐
    │            │              │  └────────────────────────►│      │  PostgreSQL   │
    │            │              │                                   │  (HA, managed)│
    ▼            ▼              ▼                                   └───────────────┘
┌─────────────────────────────────────────────┐
│ LLM providers: OpenAI, Anthropic, Bedrock,  │     Optional, from every node:
│ Vertex, Azure, self-hosted (vLLM, ...)      │     Prometheus scrape of /metrics,
│ Tool and datasource upstreams               │     OTLP traces to a collector,
└─────────────────────────────────────────────┘     OCI registry for edge plugins
```

Each edge pool serves one **namespace**. A single-region deployment has one pool in the default namespace. A multi-region deployment has one pool per region, each with its own namespace (see [Topologies](#topologies)).

## Components

| Component | Role | Runs as | State |
|---|---|---|---|
| **AI Studio** | Control plane: configuration of LLMs, Apps, filters, plugins, budgets and routers; users, SSO and RBAC; AI Portal and Chat; analytics dashboards; the gRPC control server that edges connect to (`GATEWAY_MODE=control`) | One instance, `tykio/tyk-ai-studio-ent` | PostgreSQL, plus a small data directory (branding assets, exports, plugin cache) |
| **PostgreSQL** | System of record for configuration, credentials and all analytics shipped from edges | Managed service or HA cluster, PostgreSQL 14+ | Everything that matters |
| **Microgateway (edge)** | Data plane: authenticates requests, applies access control, filters, guardrails, plugins and budgets, proxies to the LLM, records analytics (`GATEWAY_MODE=edge`) | N stateless-by-design nodes per namespace, `tykio/tyk-microgateway-ent` | A local SQLite file per node: a cache of the configuration snapshot, budget counters and queued analytics. Rebuilt from Studio on every start. |
| **Load balancer** | Spreads application traffic across the edges of one pool | Any L7 load balancer or Kubernetes Ingress/Gateway | None |
| **Observability stack** (optional) | Prometheus scrapes, OTLP trace collector | Your existing tooling | Yours |
| **OCI registry** (optional) | Source of gateway plugins distributed as OCI artifacts | Your registry, or the plugin marketplace's | Plugin images |

The edge has **no dependency on Redis, NATS or a shared database**. Everything it needs to serve a request is in its own process and its own SQLite file.

## How the pieces connect

### Network paths

| From | To | Port / protocol | Direction | Needed for |
|---|---|---|---|---|
| Applications | Edge load balancer | 443 → edge `:8080` (`PORT`), HTTP/1.1 and SSE | inbound to edges | All LLM, tool and datasource traffic |
| Edge | Studio | `:50051` (`GRPC_PORT`), gRPC over HTTP/2, TLS | **outbound from edge** | Config, heartbeats, token validation, analytics, budget sync |
| Edge | LLM providers, tool and datasource upstreams | 443 | outbound | Proxying |
| Edge | OCI registry | 443 | outbound | Only if you distribute gateway plugins as `oci://` artifacts |
| Browsers, admin API clients | Studio | 443 → Studio `:8080` (`SERVER_PORT`) | inbound to Studio | Admin UI, Portal, Chat, management API |
| Studio | PostgreSQL | 5432 | outbound | Always |
| Studio | LLM providers, tools, datasources, embedders | 443 | outbound | Chat, RAG ingestion, tool calls made from Chat |
| Prometheus | Studio `:8080`, edges `:8080` at `METRICS_PATH` | HTTP | inbound | Metrics (optional) |
| Studio, edges | OTLP collector | gRPC (`TRACING_ENDPOINT`) | outbound | Traces (optional) |

Studio's embedded gateway listener (`PROXY_PORT`, 9090) and its documentation server (`DOCS_PORT`, 8989) are not part of this architecture: do not publish them.

The load balancer in front of Studio's gRPC port must support **HTTP/2 and long-lived streams**. Each edge holds one stream open indefinitely and sends a keepalive ping every 30 seconds, so idle timeouts of a minute or more are fine.

### What travels over the control link

Everything between an edge and Studio goes over the single gRPC connection that the edge opens:

| Flow | Direction | When | Notes |
|---|---|---|---|
| **Configuration snapshot** | Studio → edge | At edge start, and when an admin pushes configuration | Global objects plus the edge's own namespace: LLMs, Apps, filters, plugins, model prices, routers, tools, datasources. Secrets in the snapshot are encrypted with `MICROGATEWAY_ENCRYPTION_KEY`. Pushes are manual: admins decide when a change reaches the data plane. |
| **Heartbeat** | edge → Studio | Every 30 s (`EDGE_HEARTBEAT_INTERVAL`) | Reports the loaded configuration checksum, so Studio can show each edge as In Sync, Pending or Stale. Also carries queued [edge-to-control plugin payloads](./plugins-edge-to-control.md). |
| **Token validation** | edge → Studio → edge | On a cache miss | Credentials are never bulk-synced. An edge asks Studio about a token the first time it sees it and caches the answer in memory for 5 minutes (`EDGE_TOKEN_CACHE_TTL`). A token revoked in Studio stops working on every edge within that TTL. |
| **Analytics pulse** | edge → Studio | Every 10 s in the Helm chart and packaged example (`interval_seconds` in the file at `PLUGINS_CONFIG_PATH`), or sooner when the buffer fills | Token usage, cost, proxy logs and budget events. Up to 1,000 records per batch, buffered in memory. |
| **Budget sync** | Studio → edge | Every 30 s (`BUDGET_SYNC_INTERVAL`, set on Studio) | Estate-wide spend per App and the current block list, so every edge enforces budgets on the total, not just its own traffic. |

> **Set `PLUGINS_CONFIG_PATH`.** The analytics pulse is loaded from that file. An edge without it serves traffic normally but never reports analytics, so dashboards and budgets stay empty. If the file omits `interval_seconds`, the pulse runs every 5 minutes. Both Studio and the Microgateway log each configured path at startup (`grep 'startup path'`).

### The path of one request

For an application calling `POST /llm/call/{llm-slug}/v1/chat/completions` on an edge:

1. **Authenticate.** The edge looks up the App credential in its in-memory token cache. On a miss it asks Studio over gRPC (the only step that can involve the control plane) and caches the result.
2. **Authorise.** It checks that the App may use this LLM, using the Apps and LLMs from its local snapshot.
3. **Budget.** Enterprise edges check the App's spend against its budget from local counters, which Studio's budget sync keeps aligned with the estate-wide total.
4. **Policy.** Filters, guardrails and plugins run in-process (plugins as local subprocesses).
5. **Proxy.** The edge forwards the request to the upstream provider and relays the response, streaming chunk by chunk for SSE.
6. **Record.** Usage, cost and the proxy log are written to the edge's local database and queued for the next analytics pulse.

Steps 1–4 and 6 are what the benchmark's "0.30 ms of the gateway's own processing" measures.

## Failure behaviour

Plan the control plane's availability around this table.

| Situation | What happens | What to plan for |
|---|---|---|
| **Studio unreachable, edges running** | Edges keep serving from their loaded configuration and reconnect in the background (5 s backoff doubling to at most 5 minutes). Tokens validated in the last 5 minutes keep working. Budgets are enforced on local counters. Analytics buffer in memory and are sent after reconnection. | Short Studio outages (restarts, upgrades, a database failover) are invisible to cached callers. |
| **A token the edge has not seen, while Studio is down** | Rejected: validation fails closed. | If availability matters more than prompt revocation, set `EDGE_TOKEN_CACHE_STALE_GRACE` (for example `30m`) to keep serving expired cache entries while Studio is unreachable. An explicit rejection from Studio always wins. |
| **An edge starts while Studio is down** | The edge exits: it will not serve without a configuration from Studio, and it does not reuse the snapshot left in its local database. On Kubernetes the pod crash-loops until Studio is back. | Run enough edges to carry peak load **without** autoscaling, so that a Studio outage during a scale-up or a node replacement does not remove capacity. Treat Studio's availability as a prerequisite for scale-out. |
| **Studio down for a long time** | Buffered analytics older than 24 hours (`edge_retention_hours`) or beyond 10,000 records (`max_buffer_size`) are dropped with a warning. Budget blocks decided by the estate-wide total stop updating. | Keep Studio outages well under a day. |
| **An edge crashes or is killed** | Other edges carry the traffic. That edge's in-memory analytics buffer (up to one pulse interval of records) is lost; graceful shutdown flushes it. | Use graceful termination (the chart's 40 s grace period is enough). |
| **PostgreSQL fails over** | Studio's readiness probe (`/ready`) fails until the database is back; Studio does not need restarting. Edges are unaffected apart from the token-miss and analytics behaviour above. | Use a managed or replicated PostgreSQL with automatic failover. |

**Budget accuracy.** With several edges, a budget is enforced on the estate-wide total with a delay of roughly one analytics pulse plus one budget sync (about 40 seconds with the defaults). An App can overrun its budget by the spend it makes across all edges in that window. See [Budget Control](./budgeting.md).

## The control plane

### Run one Studio instance

Run AI Studio as **one active instance**, with its availability provided by the database and by fast restarts:

- **In 2.2, Studio keeps edge connections, the embedded gateway's routing table, loaded Studio plugins and live chat sessions in memory, per process.** With two replicas, a configuration push handled by one replica does not reach edges connected to the other, and UI requests need session affinity. The Helm chart deploys a single replica for this reason.
- **It does not need more.** Studio is not on the data path: it serves administration, the portal, chat and the edges' control traffic. Scale it vertically if chat usage grows.
- **A restart is cheap.** Edges reconnect on their own, and callers with cached tokens do not notice.

On Kubernetes, use a single-replica Deployment with the `Recreate` strategy (so two instances never run side by side during an upgrade), readiness on `/ready` and liveness on `/health`. On VMs, run one instance under systemd with an automatic restart, and optionally a cold standby that is started only if the primary host is lost.

### Database

- **PostgreSQL in production**, 14 or later (`DATABASE_TYPE=postgres`, `DATABASE_URL`). Use a managed service or a replicated cluster with automatic failover and point-in-time recovery. SQLite is for development.
- **Plan for analytics growth.** Every request handled by any edge becomes rows in Studio's analytics tables (`llm_chat_records`, `proxy_logs`, `tool_call_records`). In 2.2, Studio does not prune them. At the benchmark's soak rate (257 req/s) that is over 22 million requests a day. Size storage for your retention period and prune old rows with a scheduled job, or partition the tables by time.
- **Bodies are opt-in.** Request and response bodies are stored only when `ANALYTICS_STORE_REQUESTS` / `ANALYTICS_STORE_RESPONSES` are set on the edges (up to `ANALYTICS_MAX_BODY_SIZE`, 4 KB each). Leave them off unless you need them: they multiply database size and they copy prompts into the control plane's region (see [Data residency](#data-residency)).

### Persistent files

Studio also writes a small amount to its working directory's `data/` folder: uploaded branding assets (`BRANDING_STORAGE_PATH`), log exports and the OCI plugin cache (`AI_STUDIO_OCI_CACHE_DIR`). Mount a persistent volume there so branding survives a restart. The OCI cache is also what enables the plugin marketplace: without `AI_STUDIO_OCI_CACHE_DIR`, the marketplace does not start.

### Secrets that must match

| Setting on Studio | Setting on every edge | Purpose |
|---|---|---|
| `GRPC_AUTH_TOKEN` (and `GRPC_AUTH_TOKEN_NEXT` during rotation) | `EDGE_AUTH_TOKEN` | Authenticates edges to the control server. Studio rejects all edges if neither is set. |
| `MICROGATEWAY_ENCRYPTION_KEY` (exactly 32 characters) | `ENCRYPTION_KEY` | Encrypts provider API keys and other secrets inside configuration snapshots. |
| `TYK_AI_LICENSE` | `TYK_AI_LICENSE` | Enterprise license. |
| `TYK_AI_SECRET_KEY` | — | Encrypts secrets at rest in Studio's database. If it is unset, secrets are stored unencrypted. Back it up: losing it makes stored secrets unreadable. |

Rotate the edge token without downtime: set the new value as `GRPC_AUTH_TOKEN_NEXT` on Studio, roll the edges onto it, then promote it to `GRPC_AUTH_TOKEN`.

## The data plane

### Measured performance

From the rc10.1 benchmark: `tykio/tyk-microgateway-ent:v2.2.0-rc10.1` in edge mode on one **c7i.xlarge (4 vCPU, 8 GiB)** on AWS, connected to Studio, with analytics shipping and a budget on the App. The full method and results are in the benchmark report (`benchmarks/gateway/benchmark-results.md` in the source repository).

| | One 4-vCPU edge |
|---|---|
| Latency added, native endpoints (`/llm/call`, `/llm/rest`, `/llm/stream`), including one network hop | **0.52–0.63 ms p50, 0.81–1.01 ms p99** |
| Of which, the gateway's own processing | 0.30 ms p50, 0.7–0.9 ms p99 |
| Latency added, OpenAI-compatible endpoints (`/ai/…`, unified `/v1`) | 0.91–1.07 ms p50, 1.49–1.88 ms p99 |
| Streaming capacity (300 ms to first token, 4.3 s streams) | **~365 req/s**, ~1,600 concurrent streams, gateway p99 ≤ 3 ms, 0 errors |
| Non-streaming capacity (upstream answers in 20 ms) | **~1,000 req/s**, 0 errors |
| 3× burst to just above capacity for 30 s | 0 errors; gateway p99 peaked at 15 ms, recovered immediately |
| One hour at 70% of capacity (257 req/s) | 0 errors; no latency or memory drift; resident memory 477–600 MB |
| Cost of prompt size | about 0.02 ms per KB (a 256 KB prompt adds ~5 ms) |

Two things to take from this:

1. **Latency is not the sizing constraint.** A model takes hundreds of milliseconds to seconds to produce its first token; the gateway adds under a millisecond until the node runs out of CPU. Where you place the edge relative to your applications and your providers matters far more than the gateway itself.
2. **Size by throughput per node, and scale out.** Each edge serves from its own copy of the configuration, so capacity grows linearly with the number of nodes.

### Sizing the edge pool

1. **Estimate peak request rate per namespace**, split into streaming and non-streaming. Chat and agent traffic is mostly streaming.
2. **Use a planning figure of 70% of measured capacity per 4-vCPU node:** about **255 streaming req/s** or **700 non-streaming req/s**. This is the rate the one-hour soak ran at without drift, and it leaves room for bursts.
3. **Nodes = ceiling(peak ÷ planning figure) + 1**, and never fewer than 2 per pool, spread across availability zones. The extra node covers a node or zone loss and rolling upgrades.

| Peak load (one namespace) | Calculation | Edges (4 vCPU) |
|---|---|---|
| 50 streaming req/s (a few thousand interactive users) | 50 ÷ 255 → 1, +1 | **2** (the minimum) |
| 400 streaming req/s | 400 ÷ 255 → 2, +1 | **3** |
| 1,200 streaming req/s | 1,200 ÷ 255 → 5, +1 | **6** |
| 2,000 non-streaming req/s (batch classification) | 2,000 ÷ 700 → 3, +1 | **4** |

Adjust for your traffic:

- **Long streams.** The benchmark's streams lasted 4.3 s. Reasoning models and long generations keep streams open for 30 s or more, so the same request rate means many more concurrent streams (concurrent streams ≈ request rate × stream duration). Check concurrency as well as rate, and measure with your own stream profile if it is far from the benchmark's.
- **Large prompts.** RAG and agent workloads with prompts of tens or hundreds of KB spend more CPU per request. Lower the planning figure, or measure.
- **Policy you add.** The benchmark ran with a budget but no filters, guardrails or plugins. Guardrails that call a model, and plugins that do real work, add their own cost.
- **OpenAI-compatible endpoints** (`/ai/…`, `/v1`) do a second internal pass and add about 0.4 ms more than the native endpoints. Their capacity was not measured separately.

### Node shape

- **4 vCPU and 2 GiB of memory per edge** matches the benchmarked node with headroom: resident memory stayed at 477–600 MB at 70% load. Larger nodes help streaming (which is CPU-bound) but not non-streaming throughput, which topped out at 2–2.4 cores. More nodes is the better lever in both cases.
- **The Helm chart's default resources (500m CPU, 512 MiB limit) are for evaluation.** Set requests and limits for production. A 512 MiB limit is below the benchmarked working set.
- **Local disk for the SQLite file**, one per node, never shared between nodes. The edge rewrites its tables on every sync and its queued data carries no node identity, so two edges cannot share one database. A node-local volume (Kubernetes `emptyDir` or a small PVC) is right: the file is a cache, rebuilt from Studio on every start.
- **Disk space.** v2.2.0-rc10.1 edges store request and response bodies locally even when `ANALYTICS_STORE_REQUESTS/RESPONSES` are off. Under benchmark load the file grew to about 13 GB in an hour, and the database stalls it caused slowed requests after the heaviest runs. This is fixed after rc10.1. On rc10.1, give the data volume generous space and monitor it, or upgrade. In all 2.2 builds the local analytics table is not pruned on the edge (`ANALYTICS_RETENTION_DAYS` is not yet enforced), so the file grows until the node is replaced. Size the volume for the time between restarts, and monitor it.

### Autoscaling

Autoscaling on CPU works well for streaming traffic, which saturates CPU at its limit. The chart's HPA (off by default) targets 75% CPU, which matches the 70% planning point. Two cautions:

- Non-streaming traffic reaches its limit at about 60% CPU on a 4-vCPU node, before a CPU target fires. For REST-heavy pools, scale on request rate, or size statically.
- New edges need Studio to start. Set `minReplicas` to cover normal peak load, and let autoscaling handle bursts above it.

### Load balancing

- Use round-robin or least-connections across the edges of one pool. Edges keep no session state, so no affinity is needed.
- Health-check `GET /ready` (checks the local database and plugins) and use `GET /health` for liveness.
- Allow long-lived responses. Streams can last minutes. The edge's own read and write timeouts are 300 s (`READ_TIMEOUT`, `WRITE_TIMEOUT`); set the load balancer's idle timeout to at least your longest expected stream, and raise the edge timeouts if a stream can go beyond five minutes.
- Do not buffer SSE responses at the load balancer.
- `/metrics` is served on the same port as traffic. Protect it with `METRICS_AUTH_TOKEN`, or block it at the load balancer.

## Topologies

### Single region

The starting point for most organisations: one Studio, one edge pool in the default namespace, one region.

```
   Region A
   ┌──────────────────────────────────────────────────────────────┐
   │  AZ 1            AZ 2            AZ 3                        │
   │  ┌───────┐      ┌───────┐      ┌───────┐                     │
   │  │Edge 1 │      │Edge 2 │      │Edge 3 │  ◄── Load balancer  │
   │  └───┬───┘      └───┬───┘      └───┬───┘                     │
   │      └──────────────┼──────────────┘                         │
   │                     ▼ gRPC :50051                            │
   │             ┌──────────────┐     ┌──────────────────────┐    │
   │             │  AI Studio   │────►│ PostgreSQL (multi-AZ)│    │
   │             └──────────────┘     └──────────────────────┘    │
   └──────────────────────────────────────────────────────────────┘
```

- At least 2 edges (3 is better) across availability zones. Studio in any one zone; PostgreSQL multi-AZ.
- On Kubernetes, edges and Studio can share a cluster. Keep them on separate node pools if you want gateway capacity isolated from Chat and ingestion work in Studio.

### Multi-region and data residency

One Studio manages edge pools in several regions. Each region's pool uses its own **namespace** (`EDGE_NAMESPACE`), so it receives only the global objects plus its region's LLMs, Apps, filters and plugins. For example, an EU pool can be given only LLM configurations pointing at EU endpoints.

```
                         ┌───────────────────────────┐
                         │  Control region           │
                         │  AI Studio + PostgreSQL   │
                         └───▲──────────▲─────────▲──┘
                  gRPC :50051│          │         │ (edges dial out)
          ┌──────────────────┘          │         └──────────────────┐
  ┌───────┴─────────────┐   ┌───────────┴─────────┐   ┌──────────────┴──────┐
  │ eu-west             │   │ us-east             │   │ ap-southeast        │
  │ namespace: eu       │   │ namespace: us       │   │ namespace: apac     │
  │ 3 edges + LB        │   │ 4 edges + LB        │   │ 2 edges + LB        │
  │ → EU model endpoints│   │ → US model endpoints│   │ → AU model endpoints│
  └─────────────────────┘   └─────────────────────┘   └─────────────────────┘
```

- **Latency.** The control link is not on the request path, so the distance between a region and the control region affects only token-cache misses (one round trip, at most once per token per 5 minutes per edge), analytics delay and configuration pushes.
- **Sizing.** Size each regional pool on its own with the method above.
- **Pushes are per namespace.** An admin can roll a change out to one region before the others.
- On Kubernetes, deploy one Microgateway release per region with its own `edgeNamespace`. See [Kubernetes / Helm](./deployment-helm-k8s.md).

#### Data residency

Prompts and completions pass through the regional edge to the regional provider and **do not cross to the control region** in the default configuration. What does cross is the analytics record for every request: App, model, token counts, cost, latency and status. If you turn on body storage (`ANALYTICS_STORE_REQUESTS` / `ANALYTICS_STORE_RESPONSES`), up to 4 KB of each request and response is shipped to Studio too, so keep it off for regions under residency rules, or place Studio in the most restrictive jurisdiction. Guardrails and filters run on the edge, inside the region.

### Hybrid and on-premises edges

Because edges only dial out, an edge can run in a customer data centre, a partner network or a different cloud from Studio, with no inbound firewall rules on that side. The site needs outbound access to Studio's gRPC endpoint and to its model providers (or its self-hosted models). This is the usual shape for keeping inference traffic next to on-premises GPUs while administering centrally.

### With self-hosted models on Kubernetes

When the models are your own (vLLM, SGLang, TGI) on Kubernetes, the edge sits in front of the Gateway API Inference Extension: the edge decides whether a request may happen and what it costs, and the Inference Extension picks the model pod. See [Running Alongside the Kubernetes Inference Gateway](./deployment-kubernetes-inference-gateway.md).

## Security

- **TLS on the control link.** Studio serves gRPC with TLS by default (`GRPC_TLS_CERT_PATH`, `GRPC_TLS_KEY_PATH`), and edges require TLS unless `EDGE_ALLOW_INSECURE=true`. The Helm chart ships with `grpcTlsInsecure: "true"` for evaluation: change it, or terminate TLS for port 50051 at a load balancer that supports gRPC. Edges verify Studio's certificate against the system trust store: use a publicly trusted certificate, or add your private CA to the trust store in the edge image. Never set `EDGE_SKIP_TLS_VERIFY` in production.
- **Edge identity.** All edges authenticate with the shared token. Studio does not verify client certificates, so if you need mutual TLS between edges and Studio, provide it with a service mesh or at the load balancer.
- **Egress control.** Restrict what LLM upstreams an edge can reach with `LLM_UPSTREAM_BLOCK_INTERNAL`, `LLM_UPSTREAM_ALLOWED_HOSTS` and `LLM_UPSTREAM_ALLOWED_INTERNAL_HOSTS`, in addition to network policy.
- **Plugins.** Gateway plugins run as local subprocesses on each edge. With `oci://` plugins, each edge pulls from the registry on start; restrict registries with `OCI_PLUGINS_ALLOWED_REGISTRIES`.
- **Studio exposure.** Only Studio's `:8080` (UI and API) and `:50051` (edges) need to be reachable, and `:50051` only from the networks your edges run in.

## Observability

Every node exposes Prometheus metrics at `METRICS_PATH` (default `/metrics`) on its main port, once `METRICS_AUTH_TOKEN` is set or `METRICS_ALLOW_UNAUTHENTICATED=true`. Metrics follow the OpenTelemetry GenAI conventions, plus governance metrics. Set `ENABLE_TRACING=true` and `TRACING_ENDPOINT` to export traces over OTLP. See [Observability](./observability.md).

Watch at least:

| Signal | Where | Why |
|---|---|---|
| Edge CPU against 70% of the node | infrastructure metrics | The streaming capacity limit |
| `gen_ai_server_time_to_first_token_seconds` and request duration, by edge | edge metrics | What users feel; mostly the provider |
| Gateway overhead (`Server-Timing`, enabled with `GATEWAY_SERVER_TIMING=true`) | response headers, sampled | Should stay near 1 ms p99 below the knee |
| Edge sync status (In Sync / Pending / Stale / Disconnected) | Studio → Edge Gateways, or `GET /api/v1/sync/status` | An edge that has stopped receiving configuration or heartbeating |
| Edge data volume usage | infrastructure metrics | Local database growth (see [Node shape](#node-shape)) |
| PostgreSQL storage and analytics table sizes | database metrics | Analytics growth; prune on schedule |
| `aistudio_policy_blocks_total` | edge metrics | Budget, filter and access blocks |

## Deployment checklist

**Control plane**

- [ ] Studio: one instance, `GATEWAY_MODE=control`, `Recreate` upgrades, persistent `data/` volume
- [ ] PostgreSQL 14+, managed or replicated, with automatic failover and backups
- [ ] `TYK_AI_SECRET_KEY`, `MICROGATEWAY_ENCRYPTION_KEY` (32 characters), `GRPC_AUTH_TOKEN` and `TYK_AI_LICENSE` stored in a secret manager and backed up
- [ ] gRPC `:50051` with TLS, reachable from edge networks only
- [ ] UI/API ingress with TLS
- [ ] Analytics retention plan for the database (scheduled pruning or partitioning)
- [ ] `AI_STUDIO_OCI_CACHE_DIR` set if you use the plugin marketplace or OCI plugins

**Each edge pool (one per namespace)**

- [ ] `GATEWAY_MODE=edge`, `CONTROL_ENDPOINT`, `EDGE_NAMESPACE`, `EDGE_AUTH_TOKEN`, `ENCRYPTION_KEY`, `TYK_AI_LICENSE`; a unique `EDGE_ID` per node (the Helm chart uses the pod name)
- [ ] `PLUGINS_CONFIG_PATH` pointing at an analytics-pulse file that exists, with `interval_seconds` set; confirm with `grep 'startup path'`
- [ ] Nodes sized with the method above: at least 2, across zones, with 4 vCPU and 2 GiB each or equivalent
- [ ] Node-local SQLite volume with room to grow, and monitoring on it
- [ ] Load balancer: `/ready` health checks, idle timeout above your longest stream, no SSE buffering, `/metrics` protected
- [ ] `minReplicas` covers normal peak; autoscaling for bursts only
- [ ] Decide on `EDGE_TOKEN_CACHE_STALE_GRACE` (availability during a Studio outage versus prompt revocation)
- [ ] Body storage left off unless you need it, especially in residency-sensitive regions

## See also

- [Edge Gateways](./edge-gateways.md): managing edges, namespaces and configuration pushes
- [Kubernetes / Helm](./deployment-helm-k8s.md), [Docker Compose](./deployment-docker.md), [Bare Metal / VM](./deployment-packages.md): installing each component
- [Budget Control](./budgeting.md): how budgets are enforced across edges
- [Observability](./observability.md): metrics and tracing reference
- [AI Gateway](./proxy.md): endpoints and the request pipeline
