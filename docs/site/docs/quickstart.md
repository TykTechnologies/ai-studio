---
title: "Quickstart"
weight: 1
---

# Quickstart

Get Tyk AI Studio running in 5 minutes with Docker Compose. This deploys AI Studio (control plane), a Microgateway (data plane), and PostgreSQL.

## Prerequisites

- Docker Engine 20.10+ and Docker Compose v2
- At least 4 GB RAM available

## 1. Generate Secrets

Generate three secret keys used for encryption and hub-spoke communication:

```bash
# Secret key for encryption (used for secrets management and SSO)
openssl rand -hex 16

# Encryption key for microgateway communication (must be exactly 32 hex chars)
openssl rand -hex 16

# gRPC auth token (for hub-spoke communication)
openssl rand -hex 16
```

Save these three values — you will substitute them into the configuration files below.

## 2. Create Directory Structure

```bash
mkdir -p tyk-ai-studio/confs tyk-ai-studio/studio-data tyk-ai-studio/mgw-data tyk-ai-studio/mgw-plugins
cd tyk-ai-studio
```

## 3. Create `compose.yaml`

```yaml
networks:
  tyk-network:

services:
  tyk-ai-studio:
    image: tykio/tyk-ai-studio:v2.0.0  # Enterprise: tykio/tyk-ai-studio-ent:v2.0.0
    networks:
      - tyk-network
    volumes:
      - ./confs/studio.env:/opt/tyk-ai-studio/.env
      - ./studio-data:/opt/tyk-ai-studio/data
    env_file:
      - ./confs/studio.env
    depends_on:
      postgres:
        condition: service_healthy
    ports:
      - "8080:8080"   # Admin UI + REST API
      - "9090:9090"   # Embedded AI Gateway
    restart: always

  microgateway:
    image: tykio/tyk-microgateway:v2.0.0  # Enterprise: tykio/tyk-microgateway-ent:v2.0.0
    networks:
      - tyk-network
    volumes:
      - ./confs/microgateway.env:/opt/tyk-microgateway/.env
      - ./confs/analytics-pulse.yaml:/opt/tyk-microgateway/analytics-pulse.yaml
      - ./mgw-data:/opt/tyk-microgateway/data
      - ./mgw-plugins:/var/lib/microgateway
    env_file:
      - ./confs/microgateway.env
    depends_on:
      tyk-ai-studio:
        condition: service_started
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
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U tyk -d tyk_ai_studio"]
      interval: 5s
      timeout: 5s
      retries: 5
      start_period: 10s
    restart: always

volumes:
  pgdata:
```

## 4. Create `confs/studio.env`

Replace the `CHANGE-ME` placeholders with the secrets you generated in step 1.

```env
DEVMODE=true
ALLOW_REGISTRATIONS=true
SITE_URL=http://localhost:8080
ADMIN_EMAIL=admin@example.com
FROM_EMAIL=noreply@example.com

DATABASE_TYPE=postgres
DATABASE_URL=postgresql://tyk:your-db-password@postgres:5432/tyk_ai_studio?sslmode=disable

TYK_AI_SECRET_KEY=CHANGE-ME-generate-with-openssl-rand-hex-16
MICROGATEWAY_ENCRYPTION_KEY=CHANGE-ME-generate-with-openssl-rand-hex-16

GATEWAY_MODE=control
GRPC_PORT=9080
GRPC_HOST=0.0.0.0
GRPC_TLS_INSECURE=true
GRPC_AUTH_TOKEN=CHANGE-ME-generate-with-openssl-rand-hex-16

PROXY_URL=http://localhost:9091
TOOL_DISPLAY_URL=http://localhost:9091
DATASOURCE_DISPLAY_URL=http://localhost:9091

LOG_LEVEL=info
```

## 5. Create `confs/microgateway.env`

The `EDGE_AUTH_TOKEN` and `ENCRYPTION_KEY` values **must match** the corresponding values in `studio.env`.

```env
PORT=8080
HOST=0.0.0.0
READ_TIMEOUT=300s
WRITE_TIMEOUT=300s
SHUTDOWN_TIMEOUT=30s

DATABASE_TYPE=sqlite
DATABASE_DSN=file:./data/edge.db?cache=shared&mode=rwc
DB_AUTO_MIGRATE=true

GATEWAY_MODE=edge
CONTROL_ENDPOINT=tyk-ai-studio:9080
EDGE_ID=edge-1
EDGE_NAMESPACE=default
EDGE_HEARTBEAT_INTERVAL=30s
EDGE_ALLOW_INSECURE=true
EDGE_TLS_ENABLED=false

EDGE_AUTH_TOKEN=CHANGE-ME-must-match-studio-GRPC_AUTH_TOKEN
ENCRYPTION_KEY=CHANGE-ME-must-match-studio-MICROGATEWAY_ENCRYPTION_KEY

GATEWAY_TIMEOUT=300s
GATEWAY_ENABLE_FILTERS=true
GATEWAY_ENABLE_ANALYTICS=true

ANALYTICS_ENABLED=true
ANALYTICS_BUFFER_SIZE=1000
ANALYTICS_FLUSH_INTERVAL=10s
ANALYTICS_RETENTION_DAYS=90

PLUGINS_CONFIG_PATH=/opt/tyk-microgateway/analytics-pulse.yaml

LOG_LEVEL=info
```

## 6. Create `confs/analytics-pulse.yaml`

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

## 7. Start and Verify

> **Important:** Make sure all configuration files (`studio.env`, `microgateway.env`, `analytics-pulse.yaml`) exist before running `docker compose up`. If a file-mounted volume target does not exist, Docker will create it as a directory, causing errors.

```bash
docker compose up -d
docker compose ps
curl -s http://localhost:8080/health
curl -s http://localhost:9091/health
```

## Access Points

| Port | URL | Purpose |
|------|-----|---------|
| 8080 | `http://localhost:8080` | AI Studio UI + REST API |
| 9090 | `http://localhost:9090` | Embedded AI Gateway |
| 9091 | `http://localhost:9091` | Microgateway (Edge Gateway) |

## Enterprise Edition

To use Enterprise Edition, swap the image names in `compose.yaml` and add your license key to both env files:

| Component | Community Edition | Enterprise Edition |
|-----------|------------------|-------------------|
| AI Studio | `tykio/tyk-ai-studio` | `tykio/tyk-ai-studio-ent` |
| Microgateway | `tykio/tyk-microgateway` | `tykio/tyk-microgateway-ent` |

Add to both `studio.env` and `microgateway.env`:

```env
TYK_AI_LICENSE=your-license-key
```

Enterprise Edition includes SSO, edge gateways, model router, and plugin marketplace features.

## After Deployment

AI Studio pre-populates OpenAI and Anthropic LLM vendors on first startup with placeholder secrets (`OPENAI_KEY` and `ANTHROPIC_KEY`). To start using them:

### 1. Add Your API Keys

1. Open AI Studio at `http://localhost:8080` and register your first admin account
2. Navigate to **Governance → Secrets** in the sidebar
3. Click on **`OPENAI_KEY`** and edit it to add your OpenAI API key
4. Click on **`ANTHROPIC_KEY`** and edit it to add your Anthropic API key

### 2. Push Configuration to the Microgateway

The Microgateway needs to receive the updated configuration from AI Studio:

1. Navigate to **AI Portal → Edge Gateways** in the sidebar
2. Verify your edge gateway (`edge-1`) shows as **Connected**
3. Click **Push Configuration** to sync the latest settings to the Microgateway

Once the sync status shows **Synced**, the Microgateway is ready to proxy LLM requests on `http://localhost:9091`.

For further setup (additional LLMs, users, applications), see the **[Initial Configuration](./configuration.md)** guide.

---

## Shared Secrets Reference

These values **must match** between the AI Studio and Microgateway configuration files:

| AI Studio Variable | Microgateway Variable | Purpose |
|---|---|---|
| `GRPC_AUTH_TOKEN` | `EDGE_AUTH_TOKEN` | Authenticates the gRPC connection |
| `MICROGATEWAY_ENCRYPTION_KEY` | `ENCRYPTION_KEY` | Encrypts synced configuration data |
| `TYK_AI_LICENSE` | `TYK_AI_LICENSE` | Enterprise license (if applicable) |

## Further Reading

- **[Initial Configuration](./configuration.md)** — Additional LLMs, users, and applications
- **[Docker Compose Deployment](./deployment-docker.md)** — Full reference with troubleshooting, external databases, SMTP, marketplace, and upgrades
- **[Kubernetes / Helm](./deployment-helm-k8s.md)** — Production deployment on Kubernetes with Helm charts
- **[Bare Metal / VM](./deployment-packages.md)** — Install via DEB/RPM packages on Linux servers
