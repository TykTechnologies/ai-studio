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
   - **Frontend (React):** http://localhost:3000
   - **Backend API:** http://localhost:8080
   - **Embedded gateway:** localhost:9090
   - **gRPC Control Server:** localhost:9091

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
| `make dev-full-ent` | Start enterprise full stack |

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
│                         PostgreSQL :5432                         │
└─────────────────────────────────────────────────────────────────┘
```

### Services

| Service | Port(s) | Description |
|---------|---------|-------------|
| `postgres` | 5432 | PostgreSQL 17 database |
| `studio` | 8080, 9090 | AI Studio (control plane) with Air hot reload |
| `frontend` | 3000 | React development server with HMR |
| `gateway` | 8081 | Microgateway (data plane) - full mode only |
| `plugins` | - | Plugin watcher/builder - full mode only |

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
| `CONTROL_ENDPOINT` | Control plane gRPC endpoint | `studio:9090` |
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
├── Dockerfile.studio           # Development container
├── .env.dev                    # AI Studio env template
├── .env.gateway.dev            # Microgateway env template
├── analytics-pulse-config.yaml # Gateway analytics config
└── air/
    ├── .air.studio.toml        # Air config for Studio
    └── .air.gateway.toml       # Air config for Gateway
```

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

### Clean slate

If things are really broken, start fresh:

```bash
make dev-clean
make dev
```

## Port Reference

| Port | Service | Protocol | Description |
|------|---------|----------|-------------|
| 3000 | Frontend | HTTP | React development server |
| 8080 | Studio | HTTP | REST API endpoints |
| 9090 | Studio | HTTP | Embedded AI Gateway |
| 9091 | Studio | gRPC | Control server (edge sync) |
| 9898 | Studio | HTTP | API Documentation server |
| 8081 | Gateway | HTTP | Gateway REST/Proxy API |
| 5432 | PostgreSQL | TCP | Database |

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
