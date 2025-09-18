# Hub-and-Spoke Configuration Guide

This guide provides comprehensive configuration instructions for deploying the microgateway in hub-and-spoke architecture.

## Table of Contents

1. [Gateway Modes](#gateway-modes)
2. [Environment Variables](#environment-variables)
3. [Control Instance Configuration](#control-instance-configuration)
4. [Edge Instance Configuration](#edge-instance-configuration)
5. [Security Configuration](#security-configuration)
6. [Network Configuration](#network-configuration)
7. [Database Configuration](#database-configuration)
8. [Namespace Management](#namespace-management)

## Gateway Modes

The microgateway operates in one of three modes, controlled by the `GATEWAY_MODE` environment variable:

### Standalone Mode (Default)
```bash
GATEWAY_MODE=standalone  # or omit entirely
```
- Traditional single-instance deployment
- All configuration stored locally in database
- No external dependencies

### Control Mode
```bash
GATEWAY_MODE=control
```
- Central hub managing edge instances
- Exposes gRPC API on configurable port
- Requires database for configuration storage

### Edge Mode
```bash
GATEWAY_MODE=edge
```
- Lightweight gateway connecting to control instance
- Receives configuration via gRPC
- No local database required for core config

## Environment Variables

### Core Configuration

| Variable | Mode | Default | Description |
|----------|------|---------|-------------|
| `GATEWAY_MODE` | All | `standalone` | Gateway operational mode |
| `PORT` | All | `8080` | HTTP API server port |
| `HOST` | All | `0.0.0.0` | HTTP API server bind address |
| `DATABASE_TYPE` | Control/Standalone | `sqlite` | Database type (`sqlite` or `postgres`) |
| `DATABASE_DSN` | Control/Standalone | `file:./data/microgateway.db` | Database connection string |

### Control Instance Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `GRPC_PORT` | No | `9090` | gRPC server port |
| `GRPC_HOST` | No | `0.0.0.0` | gRPC server bind address |
| `GRPC_TLS_ENABLED` | No | `false` | Enable TLS for gRPC |
| `GRPC_TLS_CERT_PATH` | If TLS | - | Path to TLS certificate |
| `GRPC_TLS_KEY_PATH` | If TLS | - | Path to TLS private key |
| `GRPC_AUTH_TOKEN` | No | - | Authentication token for edges |

### Edge Instance Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `CONTROL_ENDPOINT` | Yes | - | Control instance gRPC endpoint |
| `EDGE_ID` | No | Generated | Unique identifier for this edge |
| `EDGE_NAMESPACE` | No | `""` | Namespace for configuration filtering |
| `EDGE_AUTH_TOKEN` | If control requires | - | Authentication token |
| `EDGE_RECONNECT_INTERVAL` | No | `5s` | Reconnection attempt interval |
| `EDGE_HEARTBEAT_INTERVAL` | No | `30s` | Heartbeat frequency |
| `EDGE_SYNC_TIMEOUT` | No | `10s` | Configuration sync timeout |

### Security Variables (Edge)

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `EDGE_TLS_ENABLED` | No | `false` | Enable TLS for edge connection |
| `EDGE_TLS_CERT_PATH` | If mutual TLS | - | Client certificate path |
| `EDGE_TLS_KEY_PATH` | If mutual TLS | - | Client private key path |
| `EDGE_TLS_CA_PATH` | If custom CA | - | CA certificate path |
| `EDGE_SKIP_TLS_VERIFY` | No | `false` | Skip TLS certificate verification |

## Control Instance Configuration

### Basic Setup

Create a configuration file `.env` for the control instance:

```bash
# Control instance configuration
GATEWAY_MODE=control

# HTTP API configuration
PORT=8080
HOST=0.0.0.0

# Database configuration
DATABASE_TYPE=postgres
DATABASE_DSN=postgres://mgw_user:secure_password@localhost:5432/microgateway_control

# gRPC configuration
GRPC_PORT=9090
GRPC_HOST=0.0.0.0
GRPC_AUTH_TOKEN=your-secure-authentication-token-here

# Optional: Enable TLS
GRPC_TLS_ENABLED=false
# GRPC_TLS_CERT_PATH=/path/to/server.crt
# GRPC_TLS_KEY_PATH=/path/to/server.key

# Logging
LOG_LEVEL=info
LOG_FORMAT=json
```

### Database Setup

#### PostgreSQL (Recommended for Production)

1. Create database and user:
```sql
CREATE DATABASE microgateway_control;
CREATE USER mgw_user WITH ENCRYPTED PASSWORD 'secure_password';
GRANT ALL PRIVILEGES ON DATABASE microgateway_control TO mgw_user;
```

2. Set connection string:
```bash
DATABASE_DSN=postgres://mgw_user:secure_password@localhost:5432/microgateway_control?sslmode=require
```

#### SQLite (Development Only)

```bash
DATABASE_TYPE=sqlite
DATABASE_DSN=file:./data/control.db?cache=shared&mode=rwc
```

### TLS Configuration

For production deployments, enable TLS:

1. Generate certificates:
```bash
# Using openssl
openssl req -x509 -newkey rsa:4096 -keyout server.key -out server.crt -days 365 -nodes

# Or use Let's Encrypt, internal CA, etc.
```

2. Configure TLS:
```bash
GRPC_TLS_ENABLED=true
GRPC_TLS_CERT_PATH=/etc/microgateway/certs/server.crt
GRPC_TLS_KEY_PATH=/etc/microgateway/certs/server.key
```

## Edge Instance Configuration

### Basic Setup

Create a configuration file `.env` for each edge instance:

```bash
# Edge instance configuration
GATEWAY_MODE=edge

# Connection to control instance
CONTROL_ENDPOINT=control.internal.company.com:9090
EDGE_ID=edge-production-us-west-1
EDGE_NAMESPACE=production

# Authentication
EDGE_AUTH_TOKEN=your-secure-authentication-token-here

# HTTP API configuration (for local requests)
PORT=8080
HOST=0.0.0.0

# Connection behavior
EDGE_RECONNECT_INTERVAL=5s
EDGE_HEARTBEAT_INTERVAL=30s
EDGE_SYNC_TIMEOUT=10s

# Logging
LOG_LEVEL=info
LOG_FORMAT=json
```

### TLS Configuration for Edge

When the control instance uses TLS:

```bash
# Enable TLS
EDGE_TLS_ENABLED=true

# Skip verification (development only)
EDGE_SKIP_TLS_VERIFY=false

# Custom CA (if using internal certificates)
EDGE_TLS_CA_PATH=/etc/microgateway/certs/ca.crt

# Mutual TLS (if required by control)
EDGE_TLS_CERT_PATH=/etc/microgateway/certs/client.crt
EDGE_TLS_KEY_PATH=/etc/microgateway/certs/client.key
```

### Edge ID Management

Edge IDs must be unique across all edges connected to a control instance:

**Auto-generated (Default):**
```bash
# EDGE_ID will be automatically generated
# Format: random UUID
```

**Custom ID (Recommended):**
```bash
# Use descriptive, unique identifiers
EDGE_ID=edge-prod-us-west-1
EDGE_ID=edge-staging-eu-central-1
EDGE_ID=edge-tenant-abc-region-1
```

**ID Patterns:**
- `edge-{environment}-{region}-{instance}`
- `{tenant}-edge-{datacenter}-{number}`
- `mgw-{purpose}-{location}-{version}`

## Security Configuration

### Authentication Tokens

Generate secure authentication tokens:

```bash
# Generate random token (recommended method)
openssl rand -hex 32

# Alternative methods
uuidgen
head -c 32 /dev/urandom | base64
```

#### Basic Authentication Setup

**Control instance:**
```bash
GRPC_AUTH_TOKEN=a1b2c3d4e5f6789012345678901234567890abcdef1234567890abcdef123456
```

**Edge instances:**
```bash
EDGE_AUTH_TOKEN=a1b2c3d4e5f6789012345678901234567890abcdef1234567890abcdef123456
```

#### Zero-Downtime Token Rotation

For production environments, use dual-token rotation to update authentication tokens without service interruption:

**Phase 1 - Enable dual-token mode (Control instance):**
```bash
# Current production token
GRPC_AUTH_TOKEN=a1b2c3d4e5f6789012345678901234567890abcdef1234567890abcdef123456

# New token for rotation
GRPC_AUTH_TOKEN_NEXT=f9e8d7c6b5a49382716051948372615038472950173846281739504827394857
```

**Phase 2 - Update edges with new token:**
```bash
# Update all edge instances
EDGE_AUTH_TOKEN=f9e8d7c6b5a49382716051948372615038472950173846281739504827394857
```

**Phase 3 - Complete rotation (Control instance):**
```bash
# Promote new token to primary
GRPC_AUTH_TOKEN=f9e8d7c6b5a49382716051948372615038472950173846281739504827394857

# Remove rotation token (unset GRPC_AUTH_TOKEN_NEXT)
```

#### Security Best Practices

- ✅ **Use strong tokens**: Minimum 32 bytes of random data
- ✅ **Rotate regularly**: Rotate tokens every 30-90 days
- ✅ **Secure storage**: Store tokens in secrets management systems
- ✅ **Monitor access**: Log authentication failures and anomalies
- ✅ **Emergency rotation**: Have procedures for immediate token rotation

### Network Security

#### Firewall Rules

**Control Instance:**
```bash
# Allow HTTP API (management)
ufw allow 8080/tcp

# Allow gRPC (edge connections)
ufw allow 9090/tcp

# Allow SSH (management)
ufw allow 22/tcp
```

**Edge Instance:**
```bash
# Allow HTTP API (client requests)
ufw allow 8080/tcp

# Allow SSH (management)
ufw allow 22/tcp

# Block direct database access
ufw deny 5432/tcp
```

#### VPC Configuration

**Recommended Network Topology:**
```
┌─────────────────────────────────────────┐
│              Management VPC              │
│  ┌─────────────────┐                    │
│  │  Control        │  ← Admin Access    │
│  │  Instance       │                    │
│  └─────────────────┘                    │
└────────────┬────────────────────────────┘
             │ gRPC (9090)
             │
┌────────────▼────────────────────────────┐
│              Edge VPC(s)                │
│  ┌─────────────────┐ ┌─────────────────┐ │
│  │  Edge Instance  │ │  Edge Instance  │ │
│  │       A         │ │       B         │ │
│  └─────────────────┘ └─────────────────┘ │
│           ▲                   ▲         │
└───────────│───────────────────│─────────┘
            │                   │
         Client              Client
        Traffic             Traffic
```

## Network Configuration

### DNS and Service Discovery

**Static Configuration:**
```bash
CONTROL_ENDPOINT=mgw-control.internal.company.com:9090
```

**Service Discovery Integration:**
```bash
# Consul
CONTROL_ENDPOINT=mgw-control.service.consul:9090

# Kubernetes
CONTROL_ENDPOINT=microgateway-control.microgateway.svc.cluster.local:9090

# AWS ELB
CONTROL_ENDPOINT=mgw-control-elb-123456789.us-west-2.elb.amazonaws.com:9090
```

### Load Balancing

For high availability, deploy multiple control instances behind a load balancer:

**Application Load Balancer Configuration:**
```yaml
# HAProxy example
backend mgw-control
    balance roundrobin
    server control1 10.0.1.10:9090 check
    server control2 10.0.1.11:9090 check
    server control3 10.0.1.12:9090 check

frontend mgw-control-lb
    bind *:9090
    default_backend mgw-control
```

**Edge Configuration:**
```bash
CONTROL_ENDPOINT=mgw-control-lb.internal.company.com:9090
```

## Database Configuration

### Schema Migration

Before starting the control instance, run database migrations:

```bash
./microgateway -migrate
```

### Database Optimization

**PostgreSQL Configuration:**
```postgresql
# postgresql.conf optimizations for control instance
shared_buffers = 256MB
max_connections = 100
checkpoint_completion_target = 0.9
wal_buffers = 16MB
default_statistics_target = 100

# For high-frequency updates
synchronous_commit = off
checkpoint_segments = 32
```

**Index Creation:**
```sql
-- Additional indexes for performance
CREATE INDEX CONCURRENTLY idx_llms_namespace_active 
ON llms(namespace, is_active) WHERE is_active = true;

CREATE INDEX CONCURRENTLY idx_apps_namespace_active 
ON apps(namespace, is_active) WHERE is_active = true;

CREATE INDEX CONCURRENTLY idx_tokens_namespace_active 
ON api_tokens(namespace, is_active) WHERE is_active = true;
```

## Namespace Management

### Namespace Strategy

Choose a namespace strategy based on your use case:

#### Single Tenant (Simple)
```bash
# All edges use global namespace
EDGE_NAMESPACE=""
```

#### Multi-Tenant by Customer
```bash
# Tenant-specific edges
EDGE_NAMESPACE=customer-acme
EDGE_NAMESPACE=customer-globex
EDGE_NAMESPACE=customer-initech
```

#### Multi-Environment
```bash
# Environment-specific edges
EDGE_NAMESPACE=production
EDGE_NAMESPACE=staging
EDGE_NAMESPACE=development
```

#### Hierarchical (Advanced)
```bash
# Combine tenant and environment
EDGE_NAMESPACE=acme-production
EDGE_NAMESPACE=acme-staging
EDGE_NAMESPACE=globex-production
```

### Configuration Examples

**Global LLM Configuration:**
```bash
curl -X POST http://control:8080/api/v1/llms \
  -H "Content-Type: application/json" \
  -d '{
    "name": "OpenAI GPT-4",
    "vendor": "openai",
    "namespace": "",
    "api_key": "sk-...",
    "default_model": "gpt-4"
  }'
```

**Tenant-Specific LLM Configuration:**
```bash
curl -X POST http://control:8080/api/v1/llms \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Acme Custom GPT-4",
    "vendor": "openai", 
    "namespace": "acme",
    "api_key": "sk-acme-...",
    "default_model": "gpt-4"
  }'
```

## Validation and Testing

### Configuration Validation

Test configuration before deployment:

```bash
# Validate control instance configuration
GATEWAY_MODE=control ./microgateway -validate-config

# Validate edge instance configuration  
GATEWAY_MODE=edge ./microgateway -validate-config
```

### Connection Testing

Test gRPC connectivity:

```bash
# Test from edge to control
grpc_cli call control.company.com:9090 \
  microgateway.ConfigurationSyncService.RegisterEdge \
  'edge_id: "test-edge", edge_namespace: "test"'
```

### Health Checks

Monitor instance health:

```bash
# Control instance health
curl http://control:8080/health

# Edge instance health
curl http://edge:8080/health

# Configuration sync status
curl http://edge:8080/api/v1/sync/status
```

## Common Configuration Patterns

### Development Environment

**Control Instance:**
```bash
GATEWAY_MODE=control
DATABASE_TYPE=sqlite
DATABASE_DSN=file:./dev-control.db
GRPC_AUTH_TOKEN=dev-token-123
LOG_LEVEL=debug
```

**Edge Instance:**
```bash
GATEWAY_MODE=edge
CONTROL_ENDPOINT=localhost:9090
EDGE_NAMESPACE=dev
EDGE_AUTH_TOKEN=dev-token-123
LOG_LEVEL=debug
```

### Production Environment

**Control Instance:**
```bash
GATEWAY_MODE=control
DATABASE_TYPE=postgres
DATABASE_DSN=postgres://mgw:$DB_PASSWORD@postgres-primary:5432/microgateway
GRPC_TLS_ENABLED=true
GRPC_TLS_CERT_PATH=/etc/certs/server.crt
GRPC_TLS_KEY_PATH=/etc/certs/server.key
GRPC_AUTH_TOKEN=$GRPC_AUTH_TOKEN
LOG_LEVEL=info
LOG_FORMAT=json
```

**Edge Instance:**
```bash
GATEWAY_MODE=edge
CONTROL_ENDPOINT=mgw-control.internal.company.com:9090
EDGE_NAMESPACE=production
EDGE_TLS_ENABLED=true
EDGE_AUTH_TOKEN=$EDGE_AUTH_TOKEN
LOG_LEVEL=info
LOG_FORMAT=json
```

For deployment examples and operational procedures, see the [Deployment Guide](./hub-spoke-deployment.md) and [Operations Guide](./hub-spoke-operations.md).