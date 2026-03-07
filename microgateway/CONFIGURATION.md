# Microgateway Configuration Guide

This document covers all configuration options, environment variables, and settings for the microgateway.

## Configuration Methods

The microgateway supports multiple configuration methods in order of precedence:

1. **Command-line flags** (highest priority)
2. **Environment variables**
3. **Configuration files** (.env)
4. **Default values** (lowest priority)

## Environment Variables

### Server Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | 8080 | HTTP server port |
| `HOST` | 0.0.0.0 | Server bind address |
| `TLS_ENABLED` | false | Enable HTTPS |
| `TLS_CERT_PATH` | - | Path to TLS certificate file |
| `TLS_KEY_PATH` | - | Path to TLS private key file |
| `READ_TIMEOUT` | 30s | HTTP read timeout |
| `WRITE_TIMEOUT` | 30s | HTTP write timeout |
| `IDLE_TIMEOUT` | 120s | HTTP idle timeout |
| `SHUTDOWN_TIMEOUT` | 30s | Graceful shutdown timeout |

### Database Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `DATABASE_TYPE` | sqlite | Database type: sqlite or postgres |
| `DATABASE_DSN` | file:./data/microgateway.db | Database connection string |
| `DB_MAX_OPEN_CONNS` | 25 | Maximum open database connections |
| `DB_MAX_IDLE_CONNS` | 25 | Maximum idle database connections |
| `DB_CONN_MAX_LIFETIME` | 5m | Maximum connection lifetime |
| `DB_AUTO_MIGRATE` | true | Run migrations automatically on startup |
| `DB_LOG_LEVEL` | warn | Database logging level |

### Cache Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `CACHE_ENABLED` | true | Enable in-memory caching |
| `CACHE_MAX_SIZE` | 1000 | Maximum cache entries |
| `CACHE_TTL` | 1h | Cache time-to-live |
| `CACHE_CLEANUP_INTERVAL` | 10m | Cache cleanup interval |
| `CACHE_PERSIST_TO_DB` | false | Persist cache to database |

### Gateway Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `GATEWAY_TIMEOUT` | 5m | Request timeout for upstream LLM calls (high default for agentic workloads) |
| `GATEWAY_MAX_REQUEST_SIZE` | 10MB | Maximum request body size |
| `GATEWAY_MAX_RESPONSE_SIZE` | 50MB | Maximum response body size |
| `GATEWAY_DEFAULT_RATE_LIMIT` | 100 | Default requests per minute |
| `GATEWAY_ENABLE_FILTERS` | true | Enable request/response filtering |
| `GATEWAY_ENABLE_ANALYTICS` | true | Enable analytics collection |

### Analytics Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `ANALYTICS_ENABLED` | true | Enable analytics collection |
| `ANALYTICS_BUFFER_SIZE` | 1000 | Analytics buffer size before flush |
| `ANALYTICS_FLUSH_INTERVAL` | 10s | Automatic buffer flush interval |
| `ANALYTICS_RETENTION_DAYS` | 90 | Days to retain analytics data |
| `ANALYTICS_REALTIME` | false | Enable real-time analytics processing |

### Security Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `JWT_SECRET` | change-me-in-production | JWT token signing secret |
| `ENCRYPTION_KEY` | change-me-in-production | AES encryption key (32 chars) |
| `BCRYPT_COST` | 10 | bcrypt hashing cost |
| `TOKEN_LENGTH` | 32 | Generated token length |
| `SESSION_TIMEOUT` | 24h | Authentication session timeout |
| `ENABLE_RATE_LIMITING` | true | Enable rate limiting |
| `ENABLE_IP_WHITELIST` | false | Enable IP address whitelisting |

### Observability Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `LOG_LEVEL` | info | Logging level: debug, info, warn, error |
| `LOG_FORMAT` | json | Log format: json or text |
| `ENABLE_METRICS` | true | Enable Prometheus metrics |
| `METRICS_PATH` | /metrics | Prometheus metrics endpoint path |
| `ENABLE_TRACING` | false | Enable distributed tracing |
| `TRACING_ENDPOINT` | - | OpenTelemetry tracing endpoint |
| `ENABLE_PROFILING` | false | Enable Go pprof endpoints |

## Configuration Files

### Environment File (.env)

Create a `.env` file in the microgateway directory:

```bash
# Server Configuration
PORT=8080
HOST=0.0.0.0
TLS_ENABLED=false

# Database Configuration  
DATABASE_TYPE=postgres
DATABASE_DSN=postgres://user:password@localhost:5432/microgateway?sslmode=disable

# Security Configuration
JWT_SECRET=your-production-jwt-secret-here
ENCRYPTION_KEY=your-32-character-encryption-key!!

# Analytics Configuration
ANALYTICS_ENABLED=true
ANALYTICS_BUFFER_SIZE=1000
ANALYTICS_FLUSH_INTERVAL=10s

# Gateway Configuration
GATEWAY_TIMEOUT=5m
GATEWAY_ENABLE_ANALYTICS=true
```

### Development vs Production

#### Development Configuration
```bash
# Development settings
LOG_LEVEL=debug
LOG_FORMAT=text
DB_AUTO_MIGRATE=true
CACHE_ENABLED=true
ANALYTICS_REALTIME=true
ENABLE_PROFILING=true
```

#### Production Configuration
```bash
# Production settings
LOG_LEVEL=info
LOG_FORMAT=json
TLS_ENABLED=true
TLS_CERT_PATH=/etc/ssl/certs/microgateway.crt
TLS_KEY_PATH=/etc/ssl/private/microgateway.key
JWT_SECRET=production-secret-32-characters!
ENCRYPTION_KEY=production-encryption-key-32chars!
DATABASE_TYPE=postgres
DATABASE_DSN=postgres://mgw_user:secure_password@postgres:5432/microgateway?sslmode=require
ANALYTICS_RETENTION_DAYS=365
ENABLE_RATE_LIMITING=true
```

## Command Line Flags

The microgateway binary supports these command-line options:

```bash
./microgateway [flags]

Flags:
  -env string
        Path to environment file (default ".env")
  -migrate
        Run database migrations and exit
  -version
        Show version information and exit
  -config string
        Path to config file (optional)
```

### Examples
```bash
# Run with custom environment file
./microgateway -env=production.env

# Run migrations only
./microgateway -migrate

# Show version
./microgateway -version

# Use custom config file
./microgateway -config=config.yaml
```

## Database Configuration

### SQLite (Development)
```bash
DATABASE_TYPE=sqlite
DATABASE_DSN=file:./data/microgateway.db?cache=shared&mode=rwc
```

**Pros:** 
- No external dependencies
- Simple setup
- Good for development and testing

**Cons:**
- Not suitable for high concurrency
- Limited scalability

### PostgreSQL (Production)
```bash
DATABASE_TYPE=postgres
DATABASE_DSN=postgres://username:password@hostname:port/database?sslmode=require
```

**Connection String Options:**
- `sslmode=require` - Require SSL connection
- `sslmode=disable` - Disable SSL (development only)
- `pool_max_conns=25` - Maximum connection pool size
- `pool_min_conns=5` - Minimum connection pool size

**Recommended Production Settings:**
```bash
DATABASE_TYPE=postgres
DATABASE_DSN=postgres://mgw_user:secure_pass@postgres:5432/microgateway?sslmode=require&pool_max_conns=25&pool_min_conns=5
DB_MAX_OPEN_CONNS=25
DB_MAX_IDLE_CONNS=25
DB_CONN_MAX_LIFETIME=5m
```

## Security Configuration

### JWT Secrets
```bash
# Generate secure JWT secret (32+ characters)
JWT_SECRET=$(openssl rand -hex 32)
```

### Encryption Keys
```bash
# Generate AES encryption key (exactly 32 characters)
ENCRYPTION_KEY=$(openssl rand -hex 16)  # 32 hex chars = 16 bytes
```

### TLS Configuration
```bash
# Enable TLS
TLS_ENABLED=true
TLS_CERT_PATH=/path/to/certificate.crt
TLS_KEY_PATH=/path/to/private.key

# Generate self-signed certificate for testing
openssl req -x509 -newkey rsa:4096 -keyout private.key -out certificate.crt -days 365 -nodes
```

## Cache Configuration

### In-Memory Cache Settings
```bash
# Basic cache settings
CACHE_ENABLED=true
CACHE_MAX_SIZE=1000
CACHE_TTL=1h
CACHE_CLEANUP_INTERVAL=10m

# High-performance settings
CACHE_MAX_SIZE=10000
CACHE_TTL=30m
CACHE_CLEANUP_INTERVAL=5m
```

### Database Cache Persistence
```bash
# Enable database cache backing
CACHE_PERSIST_TO_DB=true
```

## Analytics Configuration

### Buffer Settings
```bash
# High-throughput settings
ANALYTICS_BUFFER_SIZE=5000
ANALYTICS_FLUSH_INTERVAL=5s
ANALYTICS_REALTIME=true

# Low-resource settings
ANALYTICS_BUFFER_SIZE=500
ANALYTICS_FLUSH_INTERVAL=30s
ANALYTICS_REALTIME=false
```

### Data Retention
```bash
# Retention settings
ANALYTICS_RETENTION_DAYS=90   # Standard
ANALYTICS_RETENTION_DAYS=365  # Long-term
ANALYTICS_RETENTION_DAYS=30   # Short-term
```

## Resource Limits

### Memory Usage
- **Base Memory**: ~50MB
- **With Cache (1000 entries)**: ~75MB
- **With Analytics Buffer (1000 events)**: ~100MB
- **Production Recommendation**: 256MB minimum, 512MB recommended

### CPU Usage
- **Idle**: <1% CPU
- **Under Load**: 10-20% CPU (depends on request volume)
- **Production Recommendation**: 2 CPU cores minimum

### Database Storage
- **Schema Size**: ~50KB (empty tables)
- **Per LLM**: ~1KB
- **Per App**: ~1KB  
- **Per Analytics Event**: ~500 bytes
- **Estimate**: 1GB handles ~2M analytics events

## Monitoring Configuration

### Health Check Endpoints
- **Health**: `/health` - Basic service health
- **Readiness**: `/ready` - Service ready to accept requests

### Metrics Collection
```bash
# Enable Prometheus metrics
ENABLE_METRICS=true
METRICS_PATH=/metrics

# Scrape metrics
curl http://localhost:8080/metrics
```

### Logging Configuration
```bash
# Development logging
LOG_LEVEL=debug
LOG_FORMAT=text

# Production logging
LOG_LEVEL=info
LOG_FORMAT=json

# High-verbosity debugging
LOG_LEVEL=debug
LOG_FORMAT=json
```

## Performance Tuning

### High-Performance Settings
```bash
# Database
DB_MAX_OPEN_CONNS=50
DB_MAX_IDLE_CONNS=50
DB_CONN_MAX_LIFETIME=10m

# Cache
CACHE_MAX_SIZE=10000
CACHE_TTL=15m

# Analytics  
ANALYTICS_BUFFER_SIZE=5000
ANALYTICS_FLUSH_INTERVAL=5s

# Server
READ_TIMEOUT=15s
WRITE_TIMEOUT=15s
IDLE_TIMEOUT=60s
```

### Low-Resource Settings
```bash
# Database
DB_MAX_OPEN_CONNS=10
DB_MAX_IDLE_CONNS=10

# Cache
CACHE_MAX_SIZE=500
CACHE_TTL=2h

# Analytics
ANALYTICS_BUFFER_SIZE=100
ANALYTICS_FLUSH_INTERVAL=60s
```

## Configuration Validation

The microgateway validates configuration on startup and provides helpful error messages:

### Common Validation Errors
```bash
# Invalid database type
Error: unsupported database type: mysql

# Missing TLS files
Error: TLS enabled but cert/key paths not provided

# Weak security keys  
Warning: Using default JWT secret. Change this in production!

# Invalid timeout values
Error: GATEWAY_TIMEOUT must be a valid duration (e.g., '30s', '1m')
```

### Configuration Check
```bash
# Dry-run configuration validation
./microgateway -config=myconfig.env -version
```

## Environment Templates

### Complete Production Template
```bash
# Production .env template
PORT=8080
HOST=0.0.0.0
TLS_ENABLED=true
TLS_CERT_PATH=/etc/ssl/certs/microgateway.crt
TLS_KEY_PATH=/etc/ssl/private/microgateway.key

DATABASE_TYPE=postgres
DATABASE_DSN=postgres://mgw_user:${DB_PASSWORD}@postgres:5432/microgateway?sslmode=require
DB_MAX_OPEN_CONNS=25
DB_AUTO_MIGRATE=true

JWT_SECRET=${JWT_SECRET}
ENCRYPTION_KEY=${ENCRYPTION_KEY}

CACHE_ENABLED=true
CACHE_MAX_SIZE=5000
CACHE_TTL=30m

ANALYTICS_ENABLED=true
ANALYTICS_BUFFER_SIZE=2000
ANALYTICS_RETENTION_DAYS=365

LOG_LEVEL=info
LOG_FORMAT=json
ENABLE_METRICS=true
```

### Development Template
```bash
# Development .env template
PORT=8080
DATABASE_TYPE=sqlite
DATABASE_DSN=file:./data/microgateway.db

JWT_SECRET=development-secret-key
ENCRYPTION_KEY=development-key-32-characters!

LOG_LEVEL=debug
LOG_FORMAT=text
ANALYTICS_REALTIME=true
```

## Configuration Best Practices

### Security
1. **Never use default secrets in production**
2. **Use strong, randomly generated keys**
3. **Enable TLS in production**
4. **Restrict allowed IPs when possible**
5. **Use environment variables for secrets**

### Performance
1. **Use PostgreSQL for production**
2. **Enable caching for better performance**
3. **Tune database connection pools**
4. **Adjust analytics buffer sizes based on load**
5. **Monitor memory usage and adjust cache size**

### Monitoring
1. **Enable Prometheus metrics**
2. **Set appropriate log levels**
3. **Configure analytics retention**
4. **Use structured logging (JSON) in production**
5. **Monitor health and readiness endpoints**

### Backup and Recovery
1. **Regular database backups**
2. **Test restoration procedures**
3. **Monitor disk usage for analytics data**
4. **Consider analytics data archival strategy**