# Configuration Reference

Complete reference for all microgateway configuration options, environment variables, and settings.

## Table of Contents

1. [Gateway Modes](#gateway-modes)
2. [Core Configuration](#core-configuration)
3. [Database Configuration](#database-configuration)
4. [Hub-and-Spoke Configuration](#hub-and-spoke-configuration)
5. [Security Configuration](#security-configuration)
6. [Observability Configuration](#observability-configuration)
7. [Performance Configuration](#performance-configuration)
8. [Advanced Configuration](#advanced-configuration)

## Gateway Modes

| Variable | Values | Default | Description |
|----------|--------|---------|-------------|
| `GATEWAY_MODE` | `standalone`, `control`, `edge` | `standalone` | Operational mode of the gateway |

### Mode Descriptions

- **`standalone`**: Traditional single-instance deployment with local database
- **`control`**: Central hub managing edge instances via gRPC
- **`edge`**: Lightweight instance receiving configuration from control

## Core Configuration

### Server Configuration

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `PORT` | int | `8080` | HTTP API server port |
| `HOST` | string | `0.0.0.0` | HTTP API server bind address |
| `READ_TIMEOUT` | duration | `30s` | HTTP read timeout |
| `WRITE_TIMEOUT` | duration | `30s` | HTTP write timeout |
| `IDLE_TIMEOUT` | duration | `120s` | HTTP idle timeout |
| `SHUTDOWN_TIMEOUT` | duration | `30s` | Graceful shutdown timeout |

### TLS Configuration

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `TLS_ENABLED` | bool | `false` | Enable TLS for HTTP server |
| `TLS_CERT_PATH` | string | - | Path to TLS certificate file |
| `TLS_KEY_PATH` | string | - | Path to TLS private key file |

**Example:**
```bash
TLS_ENABLED=true
TLS_CERT_PATH=/etc/certs/server.crt
TLS_KEY_PATH=/etc/certs/server.key
```

### Gateway Configuration

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `GATEWAY_TIMEOUT` | duration | `30s` | Default timeout for LLM requests |
| `GATEWAY_MAX_REQUEST_SIZE` | bytes | `10MB` | Maximum request body size |
| `GATEWAY_MAX_RESPONSE_SIZE` | bytes | `50MB` | Maximum response size |
| `GATEWAY_DEFAULT_RATE_LIMIT` | int | `100` | Default requests per minute limit |
| `GATEWAY_ENABLE_FILTERS` | bool | `true` | Enable filter processing |
| `GATEWAY_ENABLE_ANALYTICS` | bool | `true` | Enable analytics collection |

## Database Configuration

### Database Connection

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `DATABASE_TYPE` | string | `sqlite` | Database type (`sqlite` or `postgres`) |
| `DATABASE_DSN` | string | `file:./data/microgateway.db` | Database connection string |
| `DB_MAX_OPEN_CONNS` | int | `25` | Maximum open database connections |
| `DB_MAX_IDLE_CONNS` | int | `25` | Maximum idle database connections |
| `DB_CONN_MAX_LIFETIME` | duration | `5m` | Maximum connection lifetime |
| `DB_AUTO_MIGRATE` | bool | `true` | Automatically run database migrations |
| `DB_LOG_LEVEL` | string | `warn` | Database logging level |

### SQLite Configuration

```bash
# File-based SQLite
DATABASE_TYPE=sqlite
DATABASE_DSN="file:./data/microgateway.db?cache=shared&mode=rwc"

# In-memory SQLite (testing only)
DATABASE_DSN="file::memory:?cache=shared"
```

### PostgreSQL Configuration

```bash
# Basic PostgreSQL connection
DATABASE_TYPE=postgres
DATABASE_DSN="postgres://username:password@localhost:5432/microgateway"

# PostgreSQL with SSL
DATABASE_DSN="postgres://username:password@localhost:5432/microgateway?sslmode=require"

# PostgreSQL with custom options
DATABASE_DSN="postgres://username:password@localhost:5432/microgateway?sslmode=require&connect_timeout=10"
```

### Connection Pool Optimization

| Use Case | Max Open | Max Idle | Lifetime |
|----------|----------|----------|----------|
| Development | 10 | 5 | 1m |
| Production (Low) | 25 | 10 | 5m |
| Production (High) | 100 | 25 | 10m |

## Hub-and-Spoke Configuration

### Control Instance Configuration

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `GRPC_PORT` | int | `9090` | gRPC server port |
| `GRPC_HOST` | string | `0.0.0.0` | gRPC server bind address |
| `GRPC_TLS_ENABLED` | bool | `false` | Enable TLS for gRPC server |
| `GRPC_TLS_CERT_PATH` | string | - | Path to gRPC TLS certificate |
| `GRPC_TLS_KEY_PATH` | string | - | Path to gRPC TLS private key |
| `GRPC_AUTH_TOKEN` | string | - | Current authentication token for edges |
| `GRPC_AUTH_TOKEN_NEXT` | string | - | Next authentication token (for zero-downtime rotation) |

### Edge Instance Configuration

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `CONTROL_ENDPOINT` | string | - | Control instance gRPC endpoint |
| `EDGE_ID` | string | auto-generated | Unique identifier for this edge |
| `EDGE_NAMESPACE` | string | `""` | Namespace for configuration filtering |
| `EDGE_RECONNECT_INTERVAL` | duration | `5s` | Reconnection attempt interval |
| `EDGE_HEARTBEAT_INTERVAL` | duration | `30s` | Heartbeat frequency to control |
| `EDGE_SYNC_TIMEOUT` | duration | `10s` | Configuration sync timeout |

### Edge Authentication Configuration

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `EDGE_AUTH_TOKEN` | string | - | Authentication token |
| `EDGE_TLS_ENABLED` | bool | `false` | Enable TLS for edge connection |
| `EDGE_TLS_CERT_PATH` | string | - | Client certificate path |
| `EDGE_TLS_KEY_PATH` | string | - | Client private key path |
| `EDGE_TLS_CA_PATH` | string | - | CA certificate path |
| `EDGE_SKIP_TLS_VERIFY` | bool | `false` | Skip TLS certificate verification |

### Namespace Configuration Examples

```bash
# Global namespace (sees all global configs)
EDGE_NAMESPACE=""

# Production namespace
EDGE_NAMESPACE="production"

# Multi-tenant namespace
EDGE_NAMESPACE="tenant-acme"

# Hierarchical namespace
EDGE_NAMESPACE="acme-production-us-west"
```

### Token Rotation Configuration

The microgateway supports zero-downtime token rotation using dual-token mode:

```bash
# Step 1: Enable dual-token mode on control instance
GRPC_AUTH_TOKEN="current-secure-token"
GRPC_AUTH_TOKEN_NEXT="new-secure-token"
# Control server now accepts both tokens

# Step 2: Update edge instances with new token
EDGE_AUTH_TOKEN="new-secure-token"
# Edges reconnect with new token

# Step 3: Complete rotation (after all edges updated)
GRPC_AUTH_TOKEN="new-secure-token"
# Remove GRPC_AUTH_TOKEN_NEXT
# Control server now only accepts new token
```

**Rotation Benefits:**
- ✅ Zero downtime during token updates
- ✅ Works with hundreds of edge instances
- ✅ Kubernetes-compatible with secrets
- ✅ Automatic rollback if needed

## Security Configuration

### Authentication and Authorization

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `JWT_SECRET` | string | `change-me-in-production` | JWT signing secret |
| `ENCRYPTION_KEY` | string | `change-me-in-production` | Encryption key (32 chars) |
| `BCRYPT_COST` | int | `12` | BCrypt hashing cost (4-31) |
| `SESSION_TIMEOUT` | duration | `24h` | Session timeout duration |
| `TOKEN_EXPIRY` | duration | `720h` | API token default expiry |

### Security Headers

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `SECURITY_ENABLE_HSTS` | bool | `true` | Enable HTTP Strict Transport Security |
| `SECURITY_ENABLE_CSP` | bool | `true` | Enable Content Security Policy |
| `SECURITY_ENABLE_CSRF` | bool | `true` | Enable CSRF protection |
| `SECURITY_CORS_ORIGINS` | string | `*` | Allowed CORS origins |

### Rate Limiting

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `RATE_LIMIT_ENABLED` | bool | `true` | Enable global rate limiting |
| `RATE_LIMIT_REQUESTS` | int | `1000` | Requests per minute per IP |
| `RATE_LIMIT_BURST` | int | `100` | Burst request allowance |
| `RATE_LIMIT_CLEANUP_INTERVAL` | duration | `1m` | Cleanup interval for rate limiter |

## Observability Configuration

### Logging

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `LOG_LEVEL` | string | `info` | Logging level (`trace`, `debug`, `info`, `warn`, `error`) |
| `LOG_FORMAT` | string | `json` | Log format (`json` or `text`) |
| `LOG_OUTPUT` | string | `stdout` | Log output destination |
| `LOG_FILE_PATH` | string | - | Path to log file (if `LOG_OUTPUT=file`) |
| `LOG_MAX_SIZE` | int | `100` | Maximum log file size in MB |
| `LOG_MAX_BACKUPS` | int | `3` | Maximum number of log file backups |
| `LOG_MAX_AGE` | int | `28` | Maximum age of log files in days |

### Metrics

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `METRICS_ENABLED` | bool | `true` | Enable Prometheus metrics |
| `METRICS_PATH` | string | `/metrics` | Metrics endpoint path |
| `METRICS_PORT` | int | `8080` | Metrics server port (0 = same as main) |
| `METRICS_NAMESPACE` | string | `microgateway` | Metrics namespace |

### Tracing

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `TRACING_ENABLED` | bool | `false` | Enable distributed tracing |
| `TRACING_ENDPOINT` | string | - | Jaeger/OTLP endpoint |
| `TRACING_SERVICE_NAME` | string | `microgateway` | Service name for tracing |
| `TRACING_SAMPLE_RATE` | float | `0.1` | Trace sampling rate (0.0-1.0) |

## Performance Configuration

### Caching

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `CACHE_ENABLED` | bool | `true` | Enable response caching |
| `CACHE_MAX_SIZE` | int | `1000` | Maximum cache entries |
| `CACHE_TTL` | duration | `1h` | Cache time-to-live |
| `CACHE_CLEANUP_INTERVAL` | duration | `10m` | Cache cleanup interval |
| `CACHE_PERSIST_TO_DB` | bool | `false` | Persist cache to database |

### Analytics

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `ANALYTICS_ENABLED` | bool | `true` | Enable analytics collection |
| `ANALYTICS_BUFFER_SIZE` | int | `1000` | Analytics buffer size |
| `ANALYTICS_FLUSH_INTERVAL` | duration | `10s` | Analytics flush interval |
| `ANALYTICS_RETENTION_DAYS` | int | `90` | Analytics data retention |
| `ANALYTICS_REALTIME` | bool | `false` | Enable real-time analytics |
| `ANALYTICS_STORE_REQUESTS` | bool | `false` | Store request bodies |
| `ANALYTICS_STORE_RESPONSES` | bool | `false` | Store response bodies |
| `ANALYTICS_MAX_BODY_SIZE` | int | `4096` | Maximum body size to store |

### Connection Management

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `HTTP_MAX_IDLE_CONNS` | int | `100` | Maximum idle HTTP connections |
| `HTTP_MAX_IDLE_CONNS_PER_HOST` | int | `10` | Maximum idle connections per host |
| `HTTP_IDLE_CONN_TIMEOUT` | duration | `90s` | Idle connection timeout |
| `HTTP_KEEPALIVE_TIMEOUT` | duration | `30s` | Keep-alive timeout |

## Advanced Configuration

### Plugin Configuration

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `PLUGINS_CONFIG_PATH` | string | - | Path to plugin configuration file |
| `PLUGINS_CONFIG_SERVICE_URL` | string | - | Plugin config service URL |
| `PLUGINS_CONFIG_SERVICE_TOKEN` | string | - | Plugin config service token |
| `PLUGINS_CONFIG_POLL_INTERVAL` | duration | `30s` | Plugin config poll interval |

### Feature Flags

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `FEATURE_ASYNC_PROCESSING` | bool | `true` | Enable async request processing |
| `FEATURE_RESPONSE_STREAMING` | bool | `true` | Enable response streaming |
| `FEATURE_REQUEST_VALIDATION` | bool | `true` | Enable request validation |
| `FEATURE_CIRCUIT_BREAKER` | bool | `false` | Enable circuit breaker |

### Development Configuration

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `DEV_MODE` | bool | `false` | Enable development mode |
| `DEBUG_ENDPOINTS` | bool | `false` | Enable debug endpoints |
| `PROFILE_ENABLED` | bool | `false` | Enable profiling endpoints |
| `RELOAD_CONFIG` | bool | `false` | Enable config hot reload |

## Configuration Files

### Environment File (.env)

```bash
# .env file example
GATEWAY_MODE=control
DATABASE_TYPE=postgres
DATABASE_DSN=postgres://mgw:password@localhost:5432/microgateway
GRPC_PORT=9090
GRPC_AUTH_TOKEN=secure-token-here
LOG_LEVEL=info
LOG_FORMAT=json
```

### YAML Configuration (Optional)

```yaml
# microgateway.yaml
server:
  port: 8080
  host: "0.0.0.0"
  tls:
    enabled: false

database:
  type: postgres
  dsn: postgres://mgw:password@localhost:5432/microgateway
  max_open_conns: 25
  max_idle_conns: 10

hub_spoke:
  mode: control
  grpc:
    port: 9090
    auth_token: secure-token

logging:
  level: info
  format: json
```

## Configuration Validation

### Required Variables by Mode

**Standalone Mode:**
- `DATABASE_DSN` (if not using default SQLite)

**Control Mode:**
- `DATABASE_DSN`
- `GRPC_AUTH_TOKEN` (recommended)

**Edge Mode:**
- `CONTROL_ENDPOINT` (required)
- `EDGE_ID` (recommended)
- `EDGE_AUTH_TOKEN` (if control requires)

### Configuration Validation Commands

```bash
# Validate configuration
./microgateway -validate-config

# Check specific configuration section
./microgateway -validate-config -section=database

# Dry-run with configuration
./microgateway -dry-run
```

### Common Configuration Patterns

**Development Environment:**
```bash
GATEWAY_MODE=standalone
DATABASE_TYPE=sqlite
DATABASE_DSN="file:./dev.db"
LOG_LEVEL=debug
DEV_MODE=true
```

**Production Control Instance:**
```bash
GATEWAY_MODE=control
DATABASE_TYPE=postgres
DATABASE_DSN="postgres://mgw:${DB_PASSWORD}@postgres:5432/microgateway"
GRPC_TLS_ENABLED=true
GRPC_TLS_CERT_PATH=/etc/certs/server.crt
GRPC_TLS_KEY_PATH=/etc/certs/server.key
GRPC_AUTH_TOKEN=${GRPC_AUTH_TOKEN}
LOG_LEVEL=info
LOG_FORMAT=json
METRICS_ENABLED=true
```

**Production Edge Instance:**
```bash
GATEWAY_MODE=edge
CONTROL_ENDPOINT=control.internal:9090
EDGE_NAMESPACE=production
EDGE_TLS_ENABLED=true
EDGE_AUTH_TOKEN=${EDGE_AUTH_TOKEN}
LOG_LEVEL=info
LOG_FORMAT=json
CACHE_ENABLED=true
```

## Configuration Security

### Sensitive Variables

These variables should be stored securely and not committed to version control:

- `DATABASE_DSN` (contains password)
- `GRPC_AUTH_TOKEN`
- `EDGE_AUTH_TOKEN`
- `JWT_SECRET`
- `ENCRYPTION_KEY`
- API keys in LLM configurations

### Best Practices

1. **Use environment variables** for sensitive data
2. **Use secret management systems** (Kubernetes Secrets, AWS Secrets Manager)
3. **Rotate credentials regularly**
4. **Use strong, unique tokens**
5. **Enable TLS in production**
6. **Validate configuration before deployment**

### Example Secret Management

```bash
# Kubernetes Secrets
kubectl create secret generic microgateway-secrets \
  --from-literal=database-dsn="postgres://..." \
  --from-literal=grpc-auth-token="secure-token" \
  --from-literal=jwt-secret="jwt-secret"

# Docker Secrets
echo "secure-token" | docker secret create grpc_auth_token -

# AWS Secrets Manager
aws secretsmanager create-secret \
  --name microgateway/grpc-token \
  --secret-string "secure-token"
```

This configuration reference provides complete documentation for all microgateway settings. For deployment-specific examples, see the [Deployment Guide](./hub-spoke-deployment.md).