# Tyk AI Studio - Development Environment

This directory contains the Docker Compose-based development environment for Tyk AI Studio. It provides hot reloading, multi-component support, and easy log management.

## Quick Start

### Prerequisites

- **Docker** and **Docker Compose v2** (included with Docker Desktop)
- **Git** (with access to enterprise repo if using ENT edition)
- At least **8GB RAM** available for Docker

### First-Time Setup

1. **Clone the repository:**
   ```bash
   git clone https://github.com/TykTechnologies/midsommar.git
   cd midsommar
   ```

2. **Start the development environment:**
   ```bash
   make dev
   ```

3. **Access the application:**
   - **Frontend (React):** http://localhost:3000 (or `FRONTEND_PORT`, see below)
   - **Backend API:** http://localhost:8080 (or `STUDIO_PORT`, see below)
   - **Embedded gateway:** localhost:9090
   - **gRPC Control Server:** localhost:50051

That's it! The environment will automatically:
- Create a `.env` file from the template
- Start PostgreSQL, AI Studio, and the React frontend
- Enable hot reloading for both Go and React code

## Development Commands

### Starting the Environment

| Command | Description |
|---------|-------------|
| `make dev` | Start minimal env (Studio + Frontend + Postgres) |
| `make dev-full` | Start full stack (+ Gateway + Plugin watcher) |
| `make dev-ent` | Start enterprise minimal env |
| `make dev-full-ent` | Start enterprise full stack (no plugin builds) |
| `make dev-full-ent-plugins` | Enterprise full stack + plugin builder/watcher |

### Managing the Environment

| Command | Description |
|---------|-------------|
| `make dev-down` | Stop all containers |
| `make dev-logs` | View all logs (follow mode) |
| `make dev-logs-studio` | View Studio logs only |
| `make dev-logs-gateway` | View Gateway logs only |
| `make dev-logs-frontend` | View Frontend logs only |
| `make dev-logs-postgres` | View PostgreSQL logs only |
| `make dev-shell-studio` | Shell into Studio container |
| `make dev-shell-gateway` | Shell into Gateway container |
| `make dev-status` | Show container status |
| `make dev-clean` | Stop and remove all data (fresh start) |
| `make dev-help` | Show all available commands |

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                     Docker Network                               │
├────────────────┬────────────────┬────────────────┬──────────────┤
│   Frontend     │    Studio      │    Gateway     │   Plugins    │
│   (React HMR)  │  (Air reload)  │  (Air reload)  │  (watcher)   │
│    :3000       │  :8080/:9090   │    :8081       │  (builds)    │
├────────────────┴────────────────┴────────────────┴──────────────┤
│              PostgreSQL :5432 (host: 55432)                      │
└─────────────────────────────────────────────────────────────────┘
```

### Services

| Service | Port(s) | Description |
|---------|---------|-------------|
| `postgres` | 55432 → 5432 | PostgreSQL 17 database (host port 55432) |
| `studio` | 8080 (`STUDIO_PORT`), 9090 | AI Studio (control plane) with Air hot reload |
| `frontend` | 3000 (`FRONTEND_PORT`) | React development server with HMR |
| `gateway` | 8081 | Microgateway (data plane) - full mode only |
| `plugins` | - | Plugin watcher/builder - full mode only |

Binaries built by the watcher land in the shared `plugin_builds` volume, mounted at `/app/bin/plugins` in Studio and the gateway. Register one from **Admin → Plugins → Add Plugin** with the command `file:///app/bin/plugins/<name>` (that directory is allow-listed via `AI_STUDIO_PLUGIN_ALLOWED_DIRS` in full mode).

## Hot Reloading

### How It Works

- **Frontend (React):** Uses React's built-in Hot Module Replacement (HMR). Changes to JSX, CSS, or other frontend files are reflected instantly in the browser without a full page reload.

- **Backend (Go):** Uses [Air](https://github.com/air-verse/air) for hot reloading. When you save a Go file, Air:
  1. Detects the file change (~1 second)
  2. Rebuilds the binary (~2-3 seconds)
  3. Restarts the server automatically

- **Plugins:** In full mode (`make dev-full`), a watcher monitors the `examples/plugins/` directory and rebuilds plugins automatically when files change.

### What Triggers a Rebuild?

| Component | Watched Files | Excluded |
|-----------|--------------|----------|
| Studio | `*.go` in root | `ui/`, `microgateway/`, `tests/`, `dev/` |
| Gateway | `*.go` in `microgateway/` | `tests/`, `deployments/` |
| Plugins | All files in `examples/plugins/` | - |



## Edition Switching

### Community Edition (CE) - Default

```bash
make dev
# or
make dev-full
```

### Enterprise Edition (ENT)

1. **Initialize the enterprise submodule** (one-time):
   ```bash
   make init-enterprise
   ```

2. **Create a secrets file** with your license key:
   ```bash
   # Create dev/.env.secrets (this file is gitignored)
   echo "TYK_AI_LICENSE=your-license-key-here" > dev/.env.secrets

   # Optionally add API keys for LLM providers
   echo "OPENAI_API_KEY=sk-..." >> dev/.env.secrets
   echo "ANTHROPIC_AI_KEY=sk-ant-..." >> dev/.env.secrets
   ```

   > **Why `.env.secrets`?** This file is automatically merged into `.env` when you run the dev commands. It's gitignored, so your secrets won't be committed. This approach works with both manual runs and automated tools like Claude Code skills.

3. **Start in enterprise mode**:
   ```bash
   make dev-ent
   # or for full stack:
   make dev-full-ent
   ```

## Configuration

### Environment Files

The development environment uses separate environment files for different components:

| File | Purpose | Created From |
|------|---------|--------------|
| `.env` | AI Studio (control plane) settings | `.env.dev` |
| `.env.gateway` | Microgateway (edge) settings | `.env.gateway.dev` |
| `.env.secrets` | **Your secrets** (license, API keys) | Created manually |

These files (except `.env.secrets`) are automatically created when you run `make dev` or `make dev-full`.

### Secrets File (`.env.secrets`)

Create `dev/.env.secrets` to store your license key and API keys. This file is:
- **Gitignored** - won't be committed
- **Automatically merged** - values override the template when running `make dev-ent` etc.
- **Persistent** - survives `make dev-clean`

Example:
```bash
# dev/.env.secrets
TYK_AI_LICENSE=your-enterprise-license-key
OPENAI_API_KEY=sk-...
ANTHROPIC_AI_KEY=sk-ant-...
```

### AI Studio Environment (`.env`)

Copy the template and customize:
```bash
cp dev/.env.dev dev/.env
```

Key variables:

| Variable | Description | Default |
|----------|-------------|---------|
| `EDITION` | `ce` or `ent` | `ce` |
| `LOG_LEVEL` | `trace`, `debug`, `info`, `warn`, `error` | `debug` |
| `OPENAI_API_KEY` | OpenAI API key | (empty) |
| `ANTHROPIC_AI_KEY` | Anthropic API key | (empty) |
| `TYK_AI_SECRET_KEY` | Encryption key | `dev-secret-key...` |
| `TYK_AI_LICENSE` | Enterprise license key | (empty) |
| `GRPC_AUTH_TOKEN` | gRPC auth token for edge connections | `dev-grpc-auth-token...` |
| `GATEWAY_MODE` | `standalone` or `control` | `control` |

### Microgateway Environment (`.env.gateway`)

For full stack mode, the gateway needs its own configuration:
```bash
cp dev/.env.gateway.dev dev/.env.gateway
```

Key variables:

| Variable | Description | Default |
|----------|-------------|---------|
| `GATEWAY_MODE` | `standalone`, `control`, or `edge` | `edge` |
| `CONTROL_ENDPOINT` | Control plane gRPC endpoint | `studio:50051` |
| `EDGE_ID` | Unique identifier for this edge | `edge-dev-docker` |
| `EDGE_AUTH_TOKEN` | Auth token (must match Studio's `GRPC_AUTH_TOKEN`) | `dev-grpc-auth-token...` |
| `PLUGINS_CONFIG_PATH` | Path to analytics config | `/app/dev/analytics-pulse-config.yaml` |

### Analytics Configuration

The gateway uses `analytics-pulse-config.yaml` to configure how analytics data is sent to the control plane. This file is pre-configured for development and enables:
- Analytics batching and compression
- Budget tracking
- Proxy logging

### Compose File Structure

```
dev/
├── docker-compose.yml          # Base: postgres, studio, frontend
├── docker-compose.full.yml     # Adds: gateway, plugins
├── docker-compose.ent.yml      # Enterprise: studio, frontend config
├── docker-compose.full-ent.yml # Enterprise: gateway, plugins config
├── docker-compose.active-active.yml # Two-site stack (make dev-aa), separate project
├── aa/                         # Two-site stack: aa.sh, PostgreSQL entrypoint, front proxy
├── Dockerfile.studio           # Development container
├── .env.dev                    # AI Studio env template
├── .env.gateway.dev            # Microgateway env template
├── analytics-pulse-config.yaml # Gateway analytics config
└── air/
    ├── .air.studio.toml        # Air config for Studio
    └── .air.gateway.toml       # Air config for Gateway
```

## Active-active stack (two sites)

A second, separate stack for working on and demonstrating active-active
control planes (`features/ActiveActive.md`): two Studio replicas in two
"sites" over one replicated PostgreSQL, and two edges that know both control
planes. It is small enough to break by hand.

```bash
make dev-aa            # start it (Community Edition), detached
make dev-aa-ent        # the same, Enterprise Edition (licence from dev/.env.secrets)
make dev-aa-status     # roles, replication, replicas, edges
make dev-aa-rebuild    # rebuild (same edition) from the working tree, restart replicas and edges
make dev-aa-down       # stop; databases and roles are kept
make dev-aa-clean      # stop and remove its volumes
```

```
              site-a                                   site-b
   studio-a (label site-a) :8180           studio-b (label site-b) :8280
   cp-a  gRPC proxy        :50151          cp-b  gRPC proxy        :50251
   pg-a  PostgreSQL        :55441  <────►  pg-b  PostgreSQL        :55442
         primary              synchronous        hot standby
   edge-a :8181  CONTROL_ENDPOINTS=cp-a,cp-b
   edge-b :8281  CONTROL_ENDPOINTS=cp-b,cp-a

   front (sticky proxy over both replicas) :8100     UI (React dev server) :3100
```

- **One database, two nodes.** Both replicas use the same multi-host
  `DATABASE_URL` (`pg-a,pg-b`, `target_session_attrs=read-write`) and write
  to whichever node is the primary. `pg-b` starts as a synchronous hot
  standby of `pg-a`.
- **The same secrets on both sites**: `TYK_AI_SECRET_KEY`,
  `MICROGATEWAY_ENCRYPTION_KEY`, `GRPC_AUTH_TOKEN`, `CSRF_KEY` and
  `TYK_AI_LICENSE` come from `dev/.env` (with `dev/.env.secrets` merged in, as
  for the other targets) for both replicas. Nothing secret is in the compose
  file. `QUEUE_TYPE=postgres`, so chat works across replicas.
- **One build.** The `build` service compiles Studio and the microgateway
  once into a volume; replicas and edges run those binaries. There is no hot
  reload here: run `make dev-aa-rebuild` after changing Go code.
- **Edges keep their own database on a volume** and list their own site's
  control plane first.
- **A database of its own.** The stack starts empty: register a user in the
  UI (the first one is not an administrator automatically; promote it with
  `UPDATE users SET is_admin = true, email_verified = true WHERE email = ...`
  on the primary, e.g. `docker exec midsommar-aa-pg-a psql -U postgres -d midsommar`).

### Breaking it: `dev/aa/aa.sh`

Nothing in this stack fails over by itself. `dev/aa/aa.sh` plays the failover
manager (Patroni, CloudNativePG, a managed database), one step at a time:

| Command | What it does |
|---|---|
| `aa.sh status` | Each site's PostgreSQL role, Studio and control endpoint; replication; the replicas in `cluster_nodes` with their labels; which replica holds each edge |
| `aa.sh stop-site a\|b` | Stops that site's Studio, control endpoint and PostgreSQL node. If it held the standby, replication on the primary becomes asynchronous so commits do not wait for it |
| `aa.sh promote` | Promotes the standby (the primary's site must be stopped): the database failover. Until this is run, writes fail |
| `aa.sh rejoin a\|b` | Brings a stopped site back after a failover: its PostgreSQL node is rebuilt as the synchronous standby of the current primary, then Studio and the control endpoint start |
| `aa.sh start-site a\|b` | Starts a stopped site as it was, when no failover happened in between. Refuses if the other node was promoted meanwhile (that would be a split brain; use `rejoin`) |
| `aa.sh switchover` | Planned move of the primary role to the standby, with both sites up |
| `aa.sh cut-cp a\|b` / `restore-cp a\|b` | Cuts or restores the gRPC path from edges to that site's control plane; Studio itself keeps running |

Every command is undone by another one; none needs `make dev-aa-clean`. A
typical run:

```bash
make dev-aa && make dev-aa-status
dev/aa/aa.sh stop-site a     # site A is lost: Studio, control plane and the primary
dev/aa/aa.sh promote         # the standby in site B takes over
make dev-aa-status           # both edges are held by site-b
dev/aa/aa.sh rejoin a        # site A returns; pg-a is now the standby
dev/aa/aa.sh switchover      # and the primary role moves back to site A
```

### Ports and volumes

The stack is a compose project of its own (`midsommar-aa`), with its own
network, container names (`midsommar-aa-*`) and volumes
(`midsommar-aa-*`), and it uses none of the standard stack's host ports, so
both can run at the same time and `make dev-clean` does not touch it.

| Port | Service |
|------|---------|
| 3100 | UI (React dev server, through the front proxy) |
| 8100 | API through the sticky front proxy (either site) |
| 8180 / 8280 | site-a / site-b Studio API |
| 50151 / 50251 | site-a / site-b control gRPC (the cuttable proxies) |
| 8181 / 8281 | edge-a / edge-b gateway |
| 55441 / 55442 | pg-a / pg-b |

If the standard stack was started with `FRONTEND_PORT=3100`, the two UIs
collide: use another port there.

## Plugin Development

### Building Plugins

Plugins are automatically rebuilt when using `make dev-full`. For manual builds:

```bash
make plugins
```

### Plugin Directory Structure

```
examples/plugins/
├── studio/
│   └── my-plugin/
│       ├── server/
│       │   ├── main.go
│       │   └── manifest.json
│       └── plugin.json
└── gateway/
    └── my-gateway-plugin/
        └── main.go
```

### Plugin SDK

See [pkg/plugin_sdk/README.md](../pkg/plugin_sdk/README.md) for the unified Plugin SDK documentation.

## Troubleshooting

### Air not rebuilding on file changes

1. **Check if Air is running:**
   ```bash
   docker compose logs studio | grep -i air
   ```

2. **Verify file watching:**
   ```bash
   docker compose exec studio ls -la tmp/
   ```

3. **Docker Desktop on Mac/Windows:** Ensure "Use gRPC FUSE for file sharing" is enabled in Docker Desktop settings.

### Cannot connect to PostgreSQL

1. **Wait for health check:**
   ```bash
   make dev-status
   # postgres should show "healthy"
   ```

2. **Check PostgreSQL logs:**
   ```bash
   make dev-logs-postgres
   ```

### Enterprise features not working

1. **Verify submodule:**
   ```bash
   ls -la enterprise/
   make show-edition  # Should show "ent"
   ```

2. **Check license:**
   ```bash
   grep TYK_AI_LICENSE dev/.env
   ```

### Frontend can't connect to backend

1. **Check if Studio is healthy:**
   ```bash
   curl http://localhost:8080/health
   ```

2. **How the proxy works in Docker:**
   - The frontend uses `setupProxy.js` which reads `PROXY_TARGET` from the environment
   - In Docker Compose, `PROXY_TARGET=http://studio:8080` (uses Docker service name)
   - For local development outside Docker, it falls back to `http://localhost:8080`

3. **Check proxy configuration:**
   - `ui/admin-frontend/src/setupProxy.js` - proxy middleware configuration
   - `ui/admin-frontend/package.json` has a fallback `"proxy": "http://localhost:8080"`

4. **Verify the containers can communicate:**
   ```bash
   docker compose exec frontend wget -qO- http://studio:8080/health
   ```

### Port already in use

If you see "port already allocated" errors:

```bash
# Stop any existing containers
make dev-down

# Check what's using the port
lsof -i :8080
lsof -i :3000

# Kill the process or change the port in docker-compose.yml
```

If port 3000 or 8080 belongs to something you cannot move (a Tyk Dashboard and
Gateway, say), remap the host ports instead. Set `FRONTEND_PORT` and/or
`STUDIO_PORT` in `dev/.env` (or the shell) and start as usual:

```bash
FRONTEND_PORT=3100 STUDIO_PORT=8090 make dev   # or make dev-start, dev-full, dev-ent, ...
open http://localhost:3100
```

`SITE_URL` follows `FRONTEND_PORT`, and in `DEVMODE` the API adds the
`SITE_URL` host to the CSRF trusted origins, so login works on the new port
without any other change. Need a further origin trusted (say a production build
served from another port)? Add it to `CSRF_TRUSTED_ORIGINS` as a
comma-separated `host[:port]` list.

### Clean slate

If things are really broken, start fresh:

```bash
make dev-clean
make dev
```

## Port Reference

| Port | Service | Protocol | Description |
|------|---------|----------|-------------|
| 3000 | Frontend | HTTP | React development server (host port; override with `FRONTEND_PORT`) |
| 8080 | Studio | HTTP | REST API endpoints (host port; override with `STUDIO_PORT`) |
| 9090 | Studio | HTTP | Embedded AI Gateway |
| 50051 | Studio | gRPC | Control server (edge sync) |
| 9898 | Studio | HTTP | API Documentation server |
| 8081 | Gateway | HTTP | Gateway REST/Proxy API |
| 55432 | PostgreSQL | TCP | Database (mapped to container port 5432) |

## Volume Reference

| Volume | Purpose |
|--------|---------|
| `midsommar-postgres-data` | PostgreSQL data persistence |
| `midsommar-studio-tmp` | Air build artifacts for Studio |
| `midsommar-studio-data` | Studio data directory |
| `midsommar-gateway-tmp` | Air build artifacts for Gateway |
| `midsommar-gateway-data` | Gateway data directory |
| `midsommar-go-cache` | Go module cache (shared) |
| `midsommar-plugin-builds` | Built plugin binaries |

## Comparison with Legacy Setup

| Feature | New (`make dev`) | Legacy (`start-dev`) |
|---------|-----------------|---------------------|
| Hot reload (Go) | Yes (Air) | No |
| Hot reload (React) | Yes (HMR) | Yes (HMR) |
| Database | PostgreSQL | SQLite |
| Multi-component | Yes | No |
| Edition switching | Easy | Manual |
| Log viewing | `docker compose logs` | Screen windows |
| Container isolation | Yes | No |

The new setup is recommended for all development. The legacy `make start-dev` is kept for backward compatibility but is deprecated.

## Contributing

When making changes to the development environment:

1. Test with both `make dev` (minimal) and `make dev-full` (full stack)
2. Test both CE and ENT editions if applicable
3. Update this README if you add new features or commands
4. Ensure backward compatibility with existing workflows
