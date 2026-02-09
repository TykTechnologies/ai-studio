---
title: "Architecture Overview"
weight: 0
---

# Architecture Overview

Tyk AI Studio is an AI management platform consisting of two main applications, available in two editions, working together in a hub-and-spoke architecture.

## Two Applications

### AI Studio (Hub / Control Plane)

AI Studio is the central management application. It is a single-page web app (React) backed by a Go REST API. It serves three audiences through three distinct sections:

1. **Administration** — Where all CRUD management happens. Only available to users where `IsAdmin` is true. Admins configure LLMs, tools, data sources, filters, plugins, users, groups, and budgets.

2. **AI Portal** — A self-service developer portal. Non-admin users browse available LLMs, MCP Servers, and Data Sources, then request access by creating an App. Apps go through an admin-approval step before credentials are activated. Each App can have a budget, and the portal provides analytics.

3. **Chat** — A managed chat interface for non-technical users. Instead of running ChatGPT locally, users interact with LLMs through a monitored interface. Chats can include tools (for API calls) and data sources (for RAG). These can be "templated" as Chat Experiences to pre-configure tools and RAG, reducing cognitive load.

**AI Studio also runs these services:**

| Service | Description |
|---------|-------------|
| **Embedded Gateway** | A lightweight AI Gateway for testing LLM proxying. No filters, no middleware, no plugins — just basic proxying to verify an LLM works as expected. Also used by the Chat interface. |
| **API-based Tool Access** | Each Tool defined via OpenAPI spec is also available as a REST API endpoint for developers to call directly. |
| **MCP Tool Access** | An MCP-compliant interface (shim) for tools generated from OpenAPI specs. Provides MCP-API compatibility without a separate MCP proxy. |
| **Datasource API** | A unified REST endpoint for performing vector searches against registered data sources. |
| **Documentation Server** | A bundled Vitepress documentation site, accessible from the docs icon in the UI header. Configurable via `DOCS_PORT`, `DOCS_DISABLED`, and `DOCS_URL_OVERRIDE` environment variables. |

### Microgateway (Spoke / Data Plane)

The Microgateway is an independent, dedicated AI Gateway binary. It only handles LLM traffic proxying — it does not serve Tools or Data Sources (those remain in AI Studio).

The Microgateway provides the full middleware pipeline: authentication, filters, plugins, analytics, and budget enforcement.

**Key difference from the embedded gateway:** The embedded gateway in AI Studio is "gateway-lite" for testing and chat. The Microgateway is the production data plane with the full feature set.

For details on the Microgateway middleware pipeline, see [AI Gateway](./proxy.md). For hub-and-spoke management, see [Edge Gateways](./edge-gateways.md).

## Two Editions

Tyk AI Studio ships in two editions:

| | Community Edition (CE) | Enterprise Edition |
|---|---|---|
| **AI Gateway & Proxy** | Full | Full |
| **Chat & Portal** | Full | Full |
| **Plugins** | Full | Full |
| **SSO** | Not available | OIDC, SAML, LDAP, Social |
| **Model Router** | Not available | Full |
| **Namespaces** | Single namespace only | Multiple namespaces |
| **Edge Gateway Management** | Not available | Full UI + API |
| **Budget Enforcement** | Tracking only | Hard enforcement (blocks requests) |
| **Marketplace** | Community marketplace | Custom private marketplaces |

Edition features are controlled by **Go build tags**. Community and enterprise-specific code lives in separate git submodules:
- `community/` — Community edition features
- `enterprise/` — Enterprise edition features (requires license key)

## Hub-and-Spoke Architecture

```
┌─────────────────────────────────────────────────┐
│              AI Studio (Hub)                     │
│                                                  │
│  ┌──────────┐  ┌──────────┐  ┌───────────────┐  │
│  │  Admin   │  │  Portal  │  │    Chat        │  │
│  │  UI      │  │  UI      │  │    Interface   │  │
│  └──────────┘  └──────────┘  └───────────────┘  │
│                                                  │
│  ┌──────────────┐  ┌──────────────────────────┐  │
│  │  REST API    │  │  Embedded Gateway (lite)  │  │
│  └──────────────┘  └──────────────────────────┘  │
│                                                  │
│  ┌──────────────────────────────────────┐        │
│  │  gRPC Control Server (port 50051)    │        │
│  └──────────────────────────────────────┘        │
└──────────────┬──────────────┬────────────────────┘
               │    gRPC      │
       ┌───────▼──────┐ ┌────▼─────────┐
       │ Microgateway │ │ Microgateway │  ...
       │ (Namespace A)│ │ (Namespace B)│
       └──────────────┘ └──────────────┘
```

**How it works:**

1. **On startup**, each Microgateway connects to AI Studio via gRPC and pulls a full configuration snapshot.
2. **Snapshots are checksummed** (SHA-256). Microgateways report their loaded checksum in heartbeats. When checksums mismatch, the admin UI shows the gateway as "Pending" or "Stale".
3. **On configuration change**, an admin pushes a reload signal. This can target all gateways or a specific namespace. Each gateway then pulls the latest snapshot.
4. **Namespaces** control what gets loaded onto each gateway. LLMs, Apps, Filters, and Plugins can all be namespaced.
5. **If the hub is unreachable**, gateways continue operating from their last-known snapshot stored in a local database (SQLite or PostgreSQL).

### What Gets Synced to Gateways

| Synced (part of config snapshot) | NOT synced (Studio-only) |
|---|---|
| LLM Configurations | Tools |
| Apps | Data Sources |
| Filters | Chat configurations |
| Plugins | User management |
| Model Prices | |
| Model Routers (Enterprise) | |

> **Note:** Apps are included in the sync but are **not** part of the checksum calculation because they change frequently. Credentials are **not** pulled until a gateway actually needs them — this is a pull-on-miss caching strategy that ensures the admin retains ongoing control over access tokens.

### Analytics Flow

Both the Microgateway and AI Studio record analytics for all client interactions. In the Microgateway, analytics are batched and sent back to AI Studio every few seconds (configurable).

> **Important:** Analytics must be **explicitly enabled** in the Microgateway configuration for data to appear in Studio dashboards. This is a common stumbling block — see the [Analytics](./analytics.md) docs for configuration details.

### Distributed Budget Control

Since Microgateways can be horizontally scaled, budget tracking faces a split-brain problem. The solution:

1. All gateways send analytics batches back to AI Studio, giving Studio a complete view of token spend across the estate.
2. AI Studio sends a periodic **budget pulse** containing the total spend for each access token.
3. Gateways update their local spend counter if Studio's number is higher than what they have locally.

This provides **eventually-accurate** budget control across a multi-gateway environment. See [Budget Control](./budgeting.md) for details.

## Proxy Modes

The Microgateway (and embedded gateway) offer two ways to proxy LLM traffic:

| Mode | Endpoint | Description | Tradeoff |
|------|----------|-------------|----------|
| **SDK-Compatible** (Unified) | `/llm/call/{slug}/...` | Pass-through to the vendor's native API format. No request manipulation beyond analytics/budget tracking. | Full feature access, resilient to vendor API changes. Best for users working directly with a vendor's SDK. |
| **OpenAI-Compatible** | `/llm/call/{slug}/v1/chat/completions` | Accepts only OpenAI-format input and translates to the upstream vendor's API format. | Maximum client-side compatibility (one format for all vendors), but reduced feature access for vendor-specific capabilities. |

Both modes support streaming and non-streaming responses.

There are also two **legacy endpoints** (`/llm/rest/{slug}/...` and `/llm/stream/{slug}/...`) from before the unified endpoint existed. While not actively used by end-users, the underlying code is still used internally by the proxy to handle each response style.

For full proxy documentation, see [AI Gateway](./proxy.md).

## Next Steps

- [Quickstart](./quickstart.md) — Get running with Docker Compose
- [Core Concepts](./core-concepts.md) — Understand the key entities
- [AI Gateway](./proxy.md) — Deep dive into the proxy
- [Edge Gateways](./edge-gateways.md) — Hub-and-spoke management (Enterprise)
- [Plugin System](./plugins-overview.md) — Extend the platform
