# Environment Variables Reference

Complete reference for all microgateway environment variables organized by functional area.

## Server Configuration

### HTTP Server Settings
| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | 8080 | HTTP server port |
| `HOST` | 0.0.0.0 | Server bind address |
| `READ_TIMEOUT` | 30s | HTTP read timeout |
| `WRITE_TIMEOUT` | 30s | HTTP write timeout |
| `IDLE_TIMEOUT` | 120s | HTTP idle timeout |
| `SHUTDOWN_TIMEOUT` | 30s | Graceful shutdown timeout |

### TLS Configuration
| Variable | Default | Description |
|----------|---------|-------------|
| `TLS_ENABLED` | false | Enable HTTPS |
| `TLS_CERT_PATH` | - | Path to TLS certificate file |
| `TLS_KEY_PATH` | - | Path to TLS private key file |
| `TLS_MIN_VERSION` | 1.2 | Minimum TLS version |

## Database Configuration

### Connection Settings
| Variable | Default | Description |
|----------|---------|-------------|
| `DATABASE_TYPE` | sqlite | Database type: sqlite or postgres |
| `DATABASE_DSN` | file:./data/microgateway.db | Database connection string |
| `DB_AUTO_MIGRATE` | true | Run migrations automatically on startup |
| `DB_LOG_LEVEL` | warn | Database logging level |

### Connection Pool Settings
| Variable | Default | Description |
|----------|---------|-------------|
| `DB_MAX_OPEN_CONNS` | 25 | Maximum open database connections |
| `DB_MAX_IDLE_CONNS` | 25 | Maximum idle database connections |
| `DB_CONN_MAX_LIFETIME` | 5m | Maximum connection lifetime |

## Security Configuration

### Authentication and Encryption
| Variable | Default | Description |
|----------|---------|-------------|
| `JWT_SECRET` | change-me-in-production | JWT token signing secret |
| `ENCRYPTION_KEY` | change-me-in-production | AES encryption key (32 chars) |
| `BCRYPT_COST` | 10 | bcrypt hashing cost |
| `TOKEN_LENGTH` | 32 | Generated token length |
| `SESSION_TIMEOUT` | 24h | Authentication session timeout |

### Access Control
| Variable | Default | Description |
|----------|---------|-------------|
| `ENABLE_RATE_LIMITING` | true | Enable rate limiting |
| `ENABLE_IP_WHITELIST` | false | Enable IP address whitelisting |
| `DEFAULT_RATE_LIMIT` | 100 | Default requests per minute |

## Cache Configuration

### In-Memory Caching
| Variable | Default | Description |
|----------|---------|-------------|
| `CACHE_ENABLED` | true | Enable in-memory caching |
| `CACHE_MAX_SIZE` | 1000 | Maximum cache entries |
| `CACHE_TTL` | 1h | Cache time-to-live |
| `CACHE_CLEANUP_INTERVAL` | 10m | Cache cleanup interval |
| `CACHE_PERSIST_TO_DB` | false | Persist cache to database |

## Gateway Configuration

### Request Processing
| Variable | Default | Description |
|----------|---------|-------------|
| `GATEWAY_TIMEOUT` | 5m | Request timeout for upstream LLM calls (high default for agentic workloads) |
| `GATEWAY_MAX_REQUEST_SIZE` | 10MB | Maximum request body size |
| `GATEWAY_MAX_RESPONSE_SIZE` | 50MB | Maximum response body size |
| `GATEWAY_DEFAULT_RATE_LIMIT` | 100 | Default requests per minute |

### Features
| Variable | Default | Description |
|----------|---------|-------------|
| `GATEWAY_ENABLE_FILTERS` | true | Enable request/response filtering |
| `GATEWAY_ENABLE_ANALYTICS` | true | Enable analytics collection |
| `GATEWAY_ENABLE_BUDGETS` | true | Enable budget enforcement |

## Analytics Configuration

### Data Collection
| Variable | Default | Description |
|----------|---------|-------------|
| `ANALYTICS_ENABLED` | true | Enable analytics collection |
| `ANALYTICS_BUFFER_SIZE` | 1000 | Analytics buffer size before flush |
| `ANALYTICS_FLUSH_INTERVAL` | 10s | Automatic buffer flush interval |
| `ANALYTICS_RETENTION_DAYS` | 90 | Days to retain analytics data |
| `ANALYTICS_REALTIME` | false | Enable real-time analytics processing |

### Performance
| Variable | Default | Description |
|----------|---------|-------------|
| `ANALYTICS_WORKERS` | 5 | Number of analytics processing workers |
| `ANALYTICS_BATCH_SIZE` | 100 | Events processed per batch |
| `ANALYTICS_MAX_MEMORY` | 512MB | Maximum memory for analytics buffers |

## Budget Configuration

### Budget Enforcement
| Variable | Default | Description |
|----------|---------|-------------|
| `BUDGET_CHECK_ENABLED` | true | Enable budget enforcement |
| `BUDGET_ESTIMATION_BUFFER` | 0.1 | Safety margin for cost estimation (10%) |
| `BUDGET_RESET_TIMEZONE` | UTC | Timezone for budget reset calculations |

### Cost Calculation
| Variable | Default | Description |
|----------|---------|-------------|
| `COST_CALCULATION_ENABLED` | true | Enable cost calculation |
| `COST_PRECISION` | 4 | Decimal places for cost calculations |
| `COST_ESTIMATION_ENABLED` | true | Enable pre-request cost estimation |

## Logging Configuration

### Log Settings
| Variable | Default | Description |
|----------|---------|-------------|
| `LOG_LEVEL` | info | Logging level: debug, info, warn, error |
| `LOG_FORMAT` | json | Log format: json or text |
| `LOG_FILE_PATH` | - | Log file path (empty = stdout only) |

### Proxy Logging
| Variable | Default | Description |
|----------|---------|-------------|
| `PROXY_LOGGING_ENABLED` | true | Enable proxy request/response logging |
| `LOG_REQUEST_BODY` | false | Log full request payloads |
| `LOG_RESPONSE_BODY` | false | Log full response payloads |
| `LOG_HEADERS` | true | Log HTTP headers |

### Log Redaction
| Variable | Default | Description |
|----------|---------|-------------|
| `REDACT_SENSITIVE_HEADERS` | true | Redact Authorization headers |
| `REDACT_API_KEYS` | true | Redact API keys in logs |
| `REDACT_USER_CONTENT` | false | Redact user content for privacy |
| `REDACTION_PATTERNS` | password,secret,key,token | Comma-separated redaction patterns |

## Monitoring Configuration

### Metrics and Observability
| Variable | Default | Description |
|----------|---------|-------------|
| `ENABLE_METRICS` | true | Enable Prometheus metrics |
| `METRICS_PATH` | /metrics | Prometheus metrics endpoint path |
| `ENABLE_TRACING` | false | Enable distributed tracing |
| `TRACING_ENDPOINT` | - | OpenTelemetry tracing endpoint |
| `ENABLE_PROFILING` | false | Enable Go pprof endpoints |

### Health Checks
| Variable | Default | Description |
|----------|---------|-------------|
| `HEALTH_CHECK_ENABLED` | true | Enable health check endpoints |
| `HEALTH_CHECK_PATH` | /health | Health check endpoint path |
| `READINESS_CHECK_PATH` | /ready | Readiness check endpoint path |

## Hub-and-Spoke Configuration

### Gateway Mode
| Variable | Default | Description |
|----------|---------|-------------|
| `GATEWAY_MODE` | standalone | Gateway mode: standalone, control, edge |

### Control Mode Settings
| Variable | Default | Description |
|----------|---------|-------------|
| `GRPC_PORT` | 50051 | gRPC server port |
| `GRPC_HOST` | 0.0.0.0 | gRPC server bind address |
| `GRPC_TLS_ENABLED` | false | Enable gRPC TLS |
| `GRPC_TLS_CERT_PATH` | - | gRPC TLS certificate path |
| `GRPC_TLS_KEY_PATH` | - | gRPC TLS key path |
| `GRPC_AUTH_TOKEN` | - | gRPC authentication token |
| `MAX_EDGE_CONNECTIONS` | 100 | Maximum edge connections |
| `EDGE_HEARTBEAT_TIMEOUT` | 60s | Edge heartbeat timeout |

### Edge Mode Settings
| Variable | Default | Description |
|----------|---------|-------------|
| `CONTROL_ENDPOINT` | - | Control instance gRPC endpoint |
| `EDGE_ID` | - | Unique edge instance identifier |
| `EDGE_NAMESPACE` | - | Edge namespace for configuration filtering |
| `EDGE_AUTH_TOKEN` | - | Authentication token for control connection |
| `EDGE_RECONNECT_INTERVAL` | 5s | Reconnection interval |
| `EDGE_HEARTBEAT_INTERVAL` | 30s | Heartbeat interval |
| `EDGE_SYNC_TIMEOUT` | 10s | Configuration sync timeout |

### Edge TLS Settings
| Variable | Default | Description |
|----------|---------|-------------|
| `EDGE_TLS_ENABLED` | false | Enable TLS for control connection |
| `EDGE_TLS_CERT_PATH` | - | Client certificate path |
| `EDGE_TLS_KEY_PATH` | - | Client key path |
| `EDGE_TLS_CA_PATH` | - | CA certificate path |
| `EDGE_SKIP_TLS_VERIFY` | false | Skip TLS certificate verification |

## Plugin Configuration

### Plugin System
| Variable | Default | Description |
|----------|---------|-------------|
| `PLUGINS_ENABLED` | true | Enable plugin system |
| `PLUGINS_CONFIG_PATH` | ./config/plugins.yaml | Plugin configuration file path |
| `PLUGINS_DIR` | ./plugins | Plugin directory path |
| `PLUGINS_TIMEOUT` | 30s | Plugin execution timeout |

### Plugin Security
| Variable | Default | Description |
|----------|---------|-------------|
| `PLUGINS_VERIFY_SIGNATURES` | true | Verify plugin signatures |
| `PLUGINS_TRUSTED_KEYS_PATH` | ./keys/trusted | Trusted keys directory |
| `PLUGINS_ALLOW_UNSIGNED` | false | Allow unsigned plugins |
| `PLUGINS_MAX_MEMORY` | 256MB | Memory limit per plugin |
| `PLUGINS_MAX_CPU` | 50 | CPU percentage limit |

### Plugin Distribution (OCI)
| Variable | Default | Description |
|----------|---------|-------------|
| `PLUGINS_REGISTRY_URL` | - | OCI registry URL for plugins |
| `PLUGINS_REGISTRY_USER` | - | Registry username |
| `PLUGINS_REGISTRY_PASS` | - | Registry password |
| `PLUGINS_ALLOWED_REGISTRIES` | - | Comma-separated allowed registries |

## Configuration Examples

### Development Environment
```bash
# Development configuration
PORT=8080
DATABASE_TYPE=sqlite
DATABASE_DSN=file:./data/microgateway.db
JWT_SECRET=development-secret-key
ENCRYPTION_KEY=development-key-32-characters!
LOG_LEVEL=debug
LOG_FORMAT=text
ANALYTICS_REALTIME=true
PLUGINS_VERIFY_SIGNATURES=false
```

### Production Environment
```bash
# Production configuration
PORT=8080
HOST=0.0.0.0
TLS_ENABLED=true
TLS_CERT_PATH=/etc/ssl/certs/microgateway.crt
TLS_KEY_PATH=/etc/ssl/private/microgateway.key

DATABASE_TYPE=postgres
DATABASE_DSN=postgres://mgw_user:${DB_PASSWORD}@postgres:5432/microgateway?sslmode=require
DB_MAX_OPEN_CONNS=50
DB_AUTO_MIGRATE=true

JWT_SECRET=${JWT_SECRET}
ENCRYPTION_KEY=${ENCRYPTION_KEY}
ENABLE_RATE_LIMITING=true
ENABLE_IP_WHITELIST=true

CACHE_ENABLED=true
CACHE_MAX_SIZE=5000
CACHE_TTL=30m

ANALYTICS_ENABLED=true
ANALYTICS_BUFFER_SIZE=2000
ANALYTICS_RETENTION_DAYS=365

LOG_LEVEL=info
LOG_FORMAT=json
ENABLE_METRICS=true

PLUGINS_VERIFY_SIGNATURES=true
PLUGINS_TRUSTED_KEYS_PATH=/etc/microgateway/trusted
```

### Hub-and-Spoke Control Instance
```bash
# Control instance configuration
GATEWAY_MODE=control
GRPC_PORT=50051
GRPC_AUTH_TOKEN=${GRPC_AUTH_TOKEN}
DATABASE_TYPE=postgres
DATABASE_DSN=postgres://control:${DB_PASSWORD}@postgres:5432/control_db?sslmode=require
MAX_EDGE_CONNECTIONS=100
```

### Hub-and-Spoke Edge Instance
```bash
# Edge instance configuration
GATEWAY_MODE=edge
CONTROL_ENDPOINT=control.company.com:50051
EDGE_ID=edge-${REGION}-${INSTANCE}
EDGE_NAMESPACE=${TENANT_NAMESPACE}
EDGE_AUTH_TOKEN=${GRPC_AUTH_TOKEN}
EDGE_RECONNECT_INTERVAL=5s
```

## Configuration Validation

### Required Variables
These variables must be set for production:
```bash
# Security (required)
JWT_SECRET=must-be-32-characters-or-longer
ENCRYPTION_KEY=must-be-exactly-32-characters!!

# Hub-and-Spoke Security (required for hub-spoke deployments)
MICROGATEWAY_ENCRYPTION_KEY=must-be-exactly-32-characters!!
GRPC_AUTH_TOKEN=secure-random-token

# Plugin Security (optional but recommended)
PLUGIN_COMMAND_ALLOWLIST=/usr/bin,/usr/local/bin,python,docker
PLUGIN_BLOCK_INTERNAL_URLS=true

# Database (production)
DATABASE_TYPE=postgres
DATABASE_DSN=postgres://user:pass@host:port/db
```

### Security Validation
```bash
# Security configuration warnings
Warning: Using default JWT secret. Change this in production!
Warning: Using default encryption key. Change this in production!
Warning: TLS not enabled. Enable TLS for production!

# NEW: Hub-spoke security warnings
🔒 STARTUP SECURITY WARNING: MICROGATEWAY_ENCRYPTION_KEY not configured!
⚠️  SECURITY WARNING: MICROGATEWAY_ENCRYPTION_KEY not set - sending plaintext API key over gRPC!

# NEW: Plugin security warnings
⚠️  PLUGIN SECURITY WARNING: Plugin command uses absolute path outside standard directories
⚠️  PLUGIN SECURITY WARNING: Plugin command may target internal network address
ℹ️  PLUGIN INFO: No PLUGIN_COMMAND_ALLOWLIST configured

# Configuration validation
./microgateway --validate-config
```

## Environment-Specific Configuration

### Development Overrides
```bash
# Development-specific settings
LOG_LEVEL=debug
LOG_FORMAT=text
DB_AUTO_MIGRATE=true
ANALYTICS_REALTIME=true
ENABLE_PROFILING=true
PLUGINS_ALLOW_UNSIGNED=true
```

### Production Hardening
```bash
# Production security settings
TLS_ENABLED=true
ENABLE_RATE_LIMITING=true
PLUGINS_VERIFY_SIGNATURES=true
REDACT_SENSITIVE_HEADERS=true
LOG_REQUEST_BODY=false
LOG_RESPONSE_BODY=false
```

### High-Performance Settings
```bash
# High-performance configuration
DB_MAX_OPEN_CONNS=50
DB_MAX_IDLE_CONNS=50
CACHE_MAX_SIZE=10000
ANALYTICS_BUFFER_SIZE=5000
ANALYTICS_FLUSH_INTERVAL=5s
GATEWAY_TIMEOUT=15s
```

## Configuration Sources

### Environment File (.env)
```bash
# Create .env file
PORT=8080
DATABASE_TYPE=postgres
DATABASE_DSN=postgres://user:password@localhost:5432/microgateway
JWT_SECRET=your-production-jwt-secret-here
ENCRYPTION_KEY=your-32-character-encryption-key!!
LOG_LEVEL=info
```

### Docker Environment
```yaml
# docker-compose.yml
services:
  microgateway:
    environment:
      DATABASE_TYPE: postgres
      DATABASE_DSN: postgres://gateway:gateway123@postgres:5432/microgateway?sslmode=disable
      JWT_SECRET: ${JWT_SECRET}
      ENCRYPTION_KEY: ${ENCRYPTION_KEY}
      LOG_LEVEL: info
      ANALYTICS_ENABLED: "true"
```

### Kubernetes ConfigMap
```yaml
# ConfigMap for non-sensitive configuration
apiVersion: v1
kind: ConfigMap
metadata:
  name: microgateway-config
data:
  LOG_LEVEL: "info"
  CACHE_ENABLED: "true"
  ANALYTICS_ENABLED: "true"
  GATEWAY_TIMEOUT: "5m"

---
# Secret for sensitive configuration
apiVersion: v1
kind: Secret
metadata:
  name: microgateway-secrets
type: Opaque
stringData:
  JWT_SECRET: "your-jwt-secret"
  ENCRYPTION_KEY: "your-encryption-key"
  DATABASE_DSN: "postgres://user:pass@postgres:5432/microgateway"
```

## Configuration Patterns

### Environment-Based Configuration
```bash
# Use environment-specific files
.env.development
.env.staging  
.env.production

# Load based on environment
ENV_FILE=.env.${NODE_ENV:-development}
```

### Template-Based Configuration
```bash
# Configuration template with variable substitution
# .env.template
PORT=${PORT:-8080}
DATABASE_DSN=postgres://${DB_USER}:${DB_PASS}@${DB_HOST}:${DB_PORT}/${DB_NAME}
JWT_SECRET=${JWT_SECRET}
ENCRYPTION_KEY=${ENCRYPTION_KEY}
LOG_LEVEL=${LOG_LEVEL:-info}

# Generate actual configuration
envsubst < .env.template > .env
```

### Secret Management Integration
```bash
# Vault integration
export JWT_SECRET=$(vault kv get -field=jwt_secret secret/microgateway)
export ENCRYPTION_KEY=$(vault kv get -field=encryption_key secret/microgateway)
export DATABASE_DSN=$(vault kv get -field=database_dsn secret/microgateway)

# AWS Secrets Manager
export JWT_SECRET=$(aws secretsmanager get-secret-value \
  --secret-id microgateway/jwt-secret \
  --query SecretString --output text)
```

## Troubleshooting Configuration

### Common Configuration Errors
```bash
# Invalid duration format
Error: GATEWAY_TIMEOUT must be a valid duration (e.g., '30s', '1m')

# Invalid database type
Error: unsupported database type: mysql

# Missing TLS files
Error: TLS enabled but cert/key paths not provided

# Weak security configuration
Warning: Using default JWT secret. Change this in production!
```

### Configuration Validation
```bash
# Test configuration without starting service
./microgateway --test-config

# Validate specific settings
./microgateway --validate-env

# Check configuration loading
LOG_LEVEL=debug ./microgateway | grep "configuration loaded"
```

### Environment Variable Debugging
```bash
# Show all microgateway environment variables
env | grep -E "(PORT|DATABASE|JWT|ENCRYPTION|LOG|ANALYTICS|CACHE|GATEWAY|GRPC|EDGE)" | sort

# Test environment variable expansion
echo $DATABASE_DSN
echo $JWT_SECRET
echo $ENCRYPTION_KEY
```

## Configuration Best Practices

### Security
- Always change default secrets in production
- Use strong, randomly generated keys
- Enable TLS for production deployments
- Store secrets in external secret management systems
- Regular rotation of security credentials

### Performance
- Tune database connection pools for your workload
- Adjust cache sizes based on memory availability
- Configure analytics buffers for your request volume
- Monitor configuration impact on performance

### Monitoring
- Enable metrics collection for observability
- Configure appropriate log levels
- Set up health and readiness checks
- Monitor configuration changes in production

### Deployment
- Use environment-specific configuration files
- Validate configuration before deployment
- Test configuration changes in staging first
- Maintain configuration documentation and change logs

---

Environment variables provide flexible configuration for all microgateway features. For database-specific settings, see [Database Configuration](database.md). For security settings, see [Security Configuration](security.md).
