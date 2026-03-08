---
title: "Docker Compose Deployment"
weight: 1
---

# Docker Compose Deployment

This guide covers deploying Tyk AI Studio using Docker Compose, with optional Microgateway for distributed hub-spoke architectures.

## Prerequisites

- Docker Engine 20.10+ and Docker Compose v2
- At least 4 GB RAM available
- A Tyk AI License key (for Enterprise Edition)

## Edition Selection

Tyk AI Studio is available in two editions:

| Component | Community Edition | Enterprise Edition |
|-----------|------------------|--------------------|
| AI Studio | `tykio/tyk-ai-studio:latest` | `tykio/tyk-ai-studio:latest-ent` |
| Microgateway | `tykio/microgateway:latest` | `tykio/microgateway:latest-ent` |

Enterprise Edition includes SSO, edge gateways, model router, and plugin marketplace features. Replace the image tags in the examples below according to your edition.

## Generate Secrets

Before starting, generate the required secret keys. These will be used in the configuration files:

```bash
# Secret key for encryption (used for secrets management and SSO)
openssl rand -hex 16
# Example output: a35b3f7b0fb4dd3a048ba4fc6e9fe0a8

# Encryption key for microgateway communication (must be exactly 32 hex chars)
openssl rand -hex 16
# Example output: 822d3d1e0e2d849263e45fc7bb842364

# gRPC auth token (for hub-spoke communication)
openssl rand -hex 16
# Example output: 9f2c4a6b8d0e1f3a5c7d9e1b3a5c7d9e
```

Save these values — you will need them for both the AI Studio and Microgateway configuration files.

---

## Option A: AI Studio Standalone

This is the simplest deployment — AI Studio with its embedded gateway and a PostgreSQL database. Suitable for single-instance setups where you don't need a separate Microgateway.

### 1. Create Directory Structure

```bash
mkdir -p tyk-ai-studio/confs
cd tyk-ai-studio
```

### 2. Create `compose.yaml`

```yaml
services:
  tyk-ai-studio:
    image: tykio/tyk-ai-studio:latest
    volumes:
      - ./confs/studio.env:/app/.env
    env_file:
      - ./confs/studio.env
    depends_on:
      - postgres
    ports:
      - "8080:8080"   # Admin UI + REST API
      - "9090:9090"   # Embedded AI Gateway
    restart: always

  postgres:
    image: postgres:16
    environment:
      POSTGRES_USER: tyk
      POSTGRES_PASSWORD: your-db-password
      POSTGRES_DB: tyk_ai_studio
    volumes:
      - pgdata:/var/lib/postgresql/data
    restart: always

volumes:
  pgdata:
```

### 3. Create `confs/studio.env`

```env
# =============================================================================
# Core Settings
# =============================================================================
DEVMODE=true  # Set to false when using HTTPS; required for login over plain HTTP
ALLOW_REGISTRATIONS=true
SITE_URL=http://localhost:8080
ADMIN_EMAIL=admin@example.com
FROM_EMAIL=noreply@example.com

# =============================================================================
# Database
# =============================================================================
DATABASE_TYPE=postgres
DATABASE_URL=postgresql://tyk:your-db-password@postgres:5432/tyk_ai_studio?sslmode=disable

# =============================================================================
# Security — CHANGE THESE (use values from "Generate Secrets" above)
# =============================================================================
TYK_AI_SECRET_KEY=CHANGE-ME-generate-with-openssl-rand-hex-16
MICROGATEWAY_ENCRYPTION_KEY=CHANGE-ME-generate-with-openssl-rand-hex-16

# =============================================================================
# Logging
# =============================================================================
LOG_LEVEL=info

# =============================================================================
# Enterprise Edition Only
# =============================================================================
# TYK_AI_LICENSE=your-license-key

# =============================================================================
# Plugin Marketplace (Optional — enables browsing and installing plugins)
# =============================================================================
# AI_STUDIO_OCI_CACHE_DIR must be set to enable the marketplace.
# Without it, the Marketplace page will be empty.
AI_STUDIO_OCI_CACHE_DIR=./cache/plugins

# =============================================================================
# SMTP (Optional — required for email invites/notifications)
# =============================================================================
# SMTP_SERVER=smtp.example.com
# SMTP_PORT=587
# SMTP_USER=apikey
# SMTP_PASS=your-smtp-password
```

### 4. Start Services

```bash
docker compose up -d
```

### 5. Verify

```bash
docker compose ps
```

Access the AI Studio UI at `http://localhost:8080` and the embedded gateway at `http://localhost:9090`.

---

## Option B: AI Studio + Microgateway (Hub-Spoke)

This deployment adds a Microgateway as a separate edge gateway. AI Studio acts as the **control plane** (hub) and the Microgateway acts as the **data plane** (spoke), receiving configuration via gRPC.

This is the recommended production architecture — it separates management from request processing and enables distributed deployments.

### 1. Create Directory Structure

```bash
mkdir -p tyk-ai-studio/confs
mkdir -p tyk-ai-studio/mgw-data
cd tyk-ai-studio
```

### 2. Create `compose.yaml`

```yaml
networks:
  tyk-network:

services:
  tyk-ai-studio:
    image: tykio/tyk-ai-studio:latest
    networks:
      - tyk-network
    volumes:
      - ./confs/studio.env:/app/.env
    env_file:
      - ./confs/studio.env
    depends_on:
      - postgres
    ports:
      - "8080:8080"   # Admin UI + REST API
      - "9090:9090"   # Embedded AI Gateway
    restart: always

  microgateway:
    image: tykio/microgateway:latest
    networks:
      - tyk-network
    volumes:
      - ./confs/microgateway.env:/app/.env
      - ./confs/analytics-pulse.yaml:/app/analytics-pulse.yaml
      - ./mgw-data:/app/data
    env_file:
      - ./confs/microgateway.env
    ports:
      - "9091:8080"   # AI Gateway (external 9091 -> internal 8080)
    restart: always

  postgres:
    image: postgres:16
    networks:
      - tyk-network
    environment:
      POSTGRES_USER: tyk
      POSTGRES_PASSWORD: your-db-password
      POSTGRES_DB: tyk_ai_studio
    volumes:
      - pgdata:/var/lib/postgresql/data
    restart: always

volumes:
  pgdata:
```

### 3. Create `confs/studio.env`

```env
# =============================================================================
# Core Settings
# =============================================================================
DEVMODE=true  # Set to false when using HTTPS; required for login over plain HTTP
ALLOW_REGISTRATIONS=true
SITE_URL=http://localhost:8080
ADMIN_EMAIL=admin@example.com
FROM_EMAIL=noreply@example.com

# =============================================================================
# Database
# =============================================================================
DATABASE_TYPE=postgres
DATABASE_URL=postgresql://tyk:your-db-password@postgres:5432/tyk_ai_studio?sslmode=disable

# =============================================================================
# Security — CHANGE THESE (use values from "Generate Secrets" above)
# =============================================================================
TYK_AI_SECRET_KEY=CHANGE-ME-generate-with-openssl-rand-hex-16
MICROGATEWAY_ENCRYPTION_KEY=CHANGE-ME-generate-with-openssl-rand-hex-16

# =============================================================================
# Hub-Spoke: Control Plane Mode
# =============================================================================
GATEWAY_MODE=control
GRPC_PORT=9080
GRPC_HOST=0.0.0.0
GRPC_TLS_INSECURE=true
GRPC_AUTH_TOKEN=CHANGE-ME-generate-with-openssl-rand-hex-16

# =============================================================================
# Proxy — Point to external Microgateway URL
# =============================================================================
PROXY_URL=http://localhost:9091
TOOL_DISPLAY_URL=http://localhost:9091
DATASOURCE_DISPLAY_URL=http://localhost:9091

# =============================================================================
# Logging
# =============================================================================
LOG_LEVEL=info

# =============================================================================
# Enterprise Edition Only
# =============================================================================
# TYK_AI_LICENSE=your-license-key

# =============================================================================
# Plugin Marketplace (Optional — enables browsing and installing plugins)
# =============================================================================
# AI_STUDIO_OCI_CACHE_DIR must be set to enable the marketplace.
# Without it, the Marketplace page will be empty.
AI_STUDIO_OCI_CACHE_DIR=./cache/plugins

# =============================================================================
# SMTP (Optional — required for email invites/notifications)
# =============================================================================
# SMTP_SERVER=smtp.example.com
# SMTP_PORT=587
# SMTP_USER=apikey
# SMTP_PASS=your-smtp-password
```

### 4. Create `confs/microgateway.env`

```env
# =============================================================================
# Server Configuration
# =============================================================================
PORT=8080
HOST=0.0.0.0
READ_TIMEOUT=300s
WRITE_TIMEOUT=300s
SHUTDOWN_TIMEOUT=30s

# =============================================================================
# Database (SQLite — default for edge deployments)
# =============================================================================
DATABASE_TYPE=sqlite
DATABASE_DSN=file:./data/edge.db?cache=shared&mode=rwc
DB_AUTO_MIGRATE=true

# =============================================================================
# Hub-Spoke: Edge Mode
# =============================================================================
GATEWAY_MODE=edge
CONTROL_ENDPOINT=tyk-ai-studio:9080
EDGE_ID=edge-1
EDGE_NAMESPACE=default
EDGE_HEARTBEAT_INTERVAL=30s
EDGE_ALLOW_INSECURE=true
EDGE_TLS_ENABLED=false

# =============================================================================
# Security — MUST MATCH AI Studio values
# =============================================================================
EDGE_AUTH_TOKEN=CHANGE-ME-must-match-studio-GRPC_AUTH_TOKEN
ENCRYPTION_KEY=CHANGE-ME-must-match-studio-MICROGATEWAY_ENCRYPTION_KEY

# =============================================================================
# Gateway
# =============================================================================
GATEWAY_TIMEOUT=300s
GATEWAY_ENABLE_FILTERS=true
GATEWAY_ENABLE_ANALYTICS=true

# =============================================================================
# Analytics
# =============================================================================
ANALYTICS_ENABLED=true
ANALYTICS_BUFFER_SIZE=1000
ANALYTICS_FLUSH_INTERVAL=10s
ANALYTICS_RETENTION_DAYS=90

# =============================================================================
# Analytics Pulse Plugin (sends data to control plane)
# =============================================================================
PLUGINS_CONFIG_PATH=/app/analytics-pulse.yaml

# =============================================================================
# Cache
# =============================================================================
CACHE_ENABLED=true
CACHE_TTL=1h

# =============================================================================
# Logging
# =============================================================================
LOG_LEVEL=info

# =============================================================================
# Enterprise Edition Only
# =============================================================================
# TYK_AI_LICENSE=your-license-key
```

### 5. Create `confs/analytics-pulse.yaml`

This configures the Microgateway to send analytics data back to the AI Studio control plane:

```yaml
version: "1.0"

data_collection_plugins:
  - name: "analytics_pulse"
    enabled: true
    hook_types: ["analytics", "budget", "proxy_log"]
    replace_database: false
    priority: 100
    config:
      interval_seconds: 10
      max_batch_size: 1000
      max_buffer_size: 10000
      compression_enabled: true
      include_proxy_summaries: true
      include_request_response_data: true
      edge_retention_hours: 24
      excluded_vendors: ["mock", "test"]
      timeout_seconds: 30
      max_retries: 3
      retry_interval_secs: 5
```

### 6. Start Services

```bash
docker compose up -d
```

### 7. Verify

```bash
# Check all services are running
docker compose ps

# Check AI Studio is responding
curl -s http://localhost:8080/health

# Check Microgateway is responding
curl -s http://localhost:9091/health

# Check AI Studio logs for successful edge connection
docker compose logs tyk-ai-studio | grep -i "edge\|grpc"
```

Access points:
- **AI Studio UI**: `http://localhost:8080`
- **Embedded Gateway**: `http://localhost:9090`
- **Microgateway (Edge Gateway)**: `http://localhost:9091`

---

## Shared Secrets Reference

When running AI Studio with a Microgateway, these values **must match** between the two configuration files:

| AI Studio Variable | Microgateway Variable | Purpose |
|---|---|---|
| `GRPC_AUTH_TOKEN` | `EDGE_AUTH_TOKEN` | Authenticates the gRPC connection |
| `MICROGATEWAY_ENCRYPTION_KEY` | `ENCRYPTION_KEY` | Encrypts synced configuration data |
| `TYK_AI_LICENSE` | `TYK_AI_LICENSE` | Enterprise license (if applicable) |

## Port Reference

| Port | Component | Purpose |
|------|-----------|---------|
| 8080 | AI Studio | Admin UI + REST API |
| 9090 | AI Studio | Embedded AI Gateway |
| 9080 | AI Studio | gRPC control server (internal, hub-spoke only) |
| 9091 | Microgateway | Edge AI Gateway (mapped from internal 8080) |
| 5432 | PostgreSQL | Database |

## Using an External Database

To use an existing PostgreSQL instance instead of the bundled container, remove the `postgres` service and `pgdata` volume from `compose.yaml`, then update `studio.env`:

```env
DATABASE_TYPE=postgres
DATABASE_URL=postgresql://user:password@your-db-host:5432/tyk_ai_studio?sslmode=require
```

## Upgrading

```bash
docker compose pull
docker compose up -d
```

## Troubleshooting

### Services fail to start

```bash
docker compose logs <service-name>
```

### Microgateway cannot connect to AI Studio

- Verify `CONTROL_ENDPOINT` in `microgateway.env` matches the AI Studio service name and gRPC port (e.g., `tyk-ai-studio:9080`)
- Verify `EDGE_AUTH_TOKEN` matches `GRPC_AUTH_TOKEN`
- Verify `ENCRYPTION_KEY` matches `MICROGATEWAY_ENCRYPTION_KEY`
- Check that `GATEWAY_MODE=control` is set in `studio.env`

### Database connection errors

- Ensure the `postgres` container is healthy: `docker compose ps`
- Verify `DATABASE_URL` credentials match the `POSTGRES_USER`/`POSTGRES_PASSWORD` in `compose.yaml`
- For external databases, verify network connectivity and SSL mode

### Marketplace page is empty

The Plugin Marketplace requires `AI_STUDIO_OCI_CACHE_DIR` to be set. Without it, the marketplace service does not start and no plugins will appear. Add this to your `studio.env`:

```env
AI_STUDIO_OCI_CACHE_DIR=./cache/plugins
```

Restart AI Studio after making this change. The marketplace is enabled by default (`MARKETPLACE_ENABLED=true`), but it will not function without the OCI cache directory configured.

### Port conflicts

If ports 8080, 9090, or 9091 are already in use, change the **left-hand side** of the port mapping in `compose.yaml`:

```yaml
ports:
  - "8585:8080"   # Map to 8585 instead of 8080
```

Then update `SITE_URL` in `studio.env` accordingly.

## Next Steps

Once deployed, proceed to the [Initial Configuration](./configuration.md) guide to set up your first LLM, users, and applications.
