# Hub-and-Spoke Microgateway Architecture

This document describes the hub-and-spoke architecture implementation for the microgateway, enabling distributed deployments with centralized configuration management.

## Overview

The hub-and-spoke architecture allows you to run multiple microgateway instances in a distributed manner:

- **Control Instance**: Central hub that manages configuration and propagates changes
- **Edge Instances**: Lightweight gateways that receive configuration from the control instance
- **Namespace-based Filtering**: Objects are filtered based on namespaces, allowing tenant isolation

## Architecture Components

### 1. Gateway Modes

The microgateway supports three operational modes:

#### Standalone Mode (Default)
- Traditional single-instance deployment
- All configuration stored and managed locally in database
- No external dependencies

#### Control Mode
- Acts as the central hub in hub-and-spoke topology
- Manages configuration in database
- Exposes gRPC API for edge instances
- Propagates configuration changes to connected edges

#### Edge Mode  
- Lightweight gateway instance
- Receives configuration from control instance via gRPC
- Caches configuration locally with fallback mechanisms
- Operates independently after initial sync

### 2. Namespace System

Namespaces enable multi-tenant configuration isolation:

- **Global Namespace** (`""` empty string): Configurations visible to all edges
- **Specific Namespace** (`"tenant-1"`): Only visible to edges with matching namespace
- **Backwards Compatible**: Existing installations default to global namespace

### 3. Configuration Synchronization

#### gRPC Communication
- Bidirectional streaming for real-time updates
- Automatic reconnection with exponential backoff
- Authentication via bearer tokens
- Optional TLS encryption

#### Configuration Push System
- Explicit push-based configuration distribution via CLI, API, or GUI
- Namespace-based filtering ensures edges only receive relevant configurations
- ReloadCoordinator orchestrates distributed push operations with real-time status tracking
- Safe, administrator-controlled configuration updates prevent unintended production changes

## Configuration

### Environment Variables

#### Gateway Mode Selection
```bash
# Set gateway mode (standalone, control, edge)
GATEWAY_MODE=control                    # or "edge" or "standalone"
```

#### Control Instance Configuration
```bash
# gRPC server configuration (control mode)
GRPC_PORT=9090
GRPC_HOST=0.0.0.0
GRPC_TLS_ENABLED=false
GRPC_TLS_CERT_PATH=/path/to/cert.pem
GRPC_TLS_KEY_PATH=/path/to/key.pem
GRPC_AUTH_TOKEN=secure-auth-token
```

#### Edge Instance Configuration
```bash
# Connection to control instance (edge mode)
CONTROL_ENDPOINT=control.example.com:9090
EDGE_ID=edge-1
EDGE_NAMESPACE=tenant-1
EDGE_RECONNECT_INTERVAL=5s
EDGE_HEARTBEAT_INTERVAL=30s
EDGE_SYNC_TIMEOUT=10s

# Authentication
EDGE_AUTH_TOKEN=secure-auth-token
EDGE_TLS_ENABLED=false
EDGE_TLS_CERT_PATH=/path/to/client-cert.pem
EDGE_TLS_KEY_PATH=/path/to/client-key.pem
EDGE_TLS_CA_PATH=/path/to/ca.pem
EDGE_SKIP_TLS_VERIFY=false
```

## Deployment Examples

### 1. Basic Hub-and-Spoke Setup

#### Control Instance
```bash
# Start control instance
GATEWAY_MODE=control \
GRPC_PORT=9090 \
GRPC_AUTH_TOKEN=my-secure-token \
DATABASE_TYPE=postgres \
DATABASE_DSN="postgres://user:pass@localhost/control_db" \
./microgateway
```

#### Edge Instance
```bash
# Start edge instance
GATEWAY_MODE=edge \
CONTROL_ENDPOINT=control.example.com:9090 \
EDGE_ID=edge-region-1 \
EDGE_NAMESPACE=production \
EDGE_AUTH_TOKEN=my-secure-token \
./microgateway
```

### 2. Multi-Tenant Setup

#### Control Instance with Multiple Namespaces
```bash
# Control instance serves multiple tenants
GATEWAY_MODE=control \
GRPC_AUTH_TOKEN=control-token \
./microgateway
```

#### Tenant-Specific Edge Instances
```bash
# Edge for tenant-a
GATEWAY_MODE=edge \
CONTROL_ENDPOINT=control.internal:9090 \
EDGE_NAMESPACE=tenant-a \
EDGE_ID=tenant-a-edge-1 \
./microgateway

# Edge for tenant-b  
GATEWAY_MODE=edge \
CONTROL_ENDPOINT=control.internal:9090 \
EDGE_NAMESPACE=tenant-b \
EDGE_ID=tenant-b-edge-1 \
./microgateway
```

### 3. High-Availability Setup

#### Load-Balanced Control Instances
```yaml
# docker-compose.yml
version: '3.8'
services:
  control-1:
    image: microgateway:latest
    environment:
      GATEWAY_MODE: control
      DATABASE_DSN: postgres://user:pass@postgres:5432/control_db
      GRPC_PORT: 9090
    ports:
      - "9090:9090"
      - "8080:8080"
  
  control-2:
    image: microgateway:latest  
    environment:
      GATEWAY_MODE: control
      DATABASE_DSN: postgres://user:pass@postgres:5432/control_db
      GRPC_PORT: 9090
    ports:
      - "9091:9090"
      - "8081:8080"
  
  edge-1:
    image: microgateway:latest
    environment:
      GATEWAY_MODE: edge
      CONTROL_ENDPOINT: control-1:9090
      EDGE_NAMESPACE: production
      EDGE_ID: edge-1
```

## Data Model Changes

### Database Schema

New namespace columns added to core tables:
```sql
-- Add namespace support
ALTER TABLE llms ADD COLUMN namespace VARCHAR(255) NOT NULL DEFAULT '';
ALTER TABLE apps ADD COLUMN namespace VARCHAR(255) NOT NULL DEFAULT '';  
ALTER TABLE api_tokens ADD COLUMN namespace VARCHAR(255) NOT NULL DEFAULT '';
ALTER TABLE model_prices ADD COLUMN namespace VARCHAR(255) NOT NULL DEFAULT '';
ALTER TABLE filters ADD COLUMN namespace VARCHAR(255) NOT NULL DEFAULT '';
ALTER TABLE plugins ADD COLUMN namespace VARCHAR(255) NOT NULL DEFAULT '';

-- Edge instance tracking (control mode only)
CREATE TABLE edge_instances (
    id SERIAL PRIMARY KEY,
    edge_id VARCHAR(255) UNIQUE NOT NULL,
    namespace VARCHAR(255) NOT NULL DEFAULT '',
    version VARCHAR(100),
    build_hash VARCHAR(64),
    metadata JSON,
    last_heartbeat TIMESTAMP,
    status VARCHAR(50) DEFAULT 'registered',
    session_id VARCHAR(255),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Configuration change tracking
CREATE TABLE configuration_changes (
    id SERIAL PRIMARY KEY,
    change_type VARCHAR(20) NOT NULL, -- CREATE, UPDATE, DELETE
    entity_type VARCHAR(50) NOT NULL, -- LLM, APP, TOKEN, etc.
    entity_id INTEGER NOT NULL,
    entity_data JSON,
    namespace VARCHAR(255) NOT NULL DEFAULT '',
    propagated_to_edges JSON,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    processed BOOLEAN DEFAULT FALSE
);
```

## API Changes

### Configuration Management

All existing APIs continue to work with optional namespace parameter:

```bash
# Create LLM in specific namespace
curl -X POST /api/v1/llms \
  -H "Content-Type: application/json" \
  -d '{"name": "OpenAI GPT-4", "namespace": "tenant-1", ...}'

# List LLMs in namespace
curl "/api/v1/llms?namespace=tenant-1"

# Global objects (empty namespace)
curl -X POST /api/v1/llms \
  -H "Content-Type: application/json" \
  -d '{"name": "Global LLM", "namespace": "", ...}'
```

### Edge Management API

New APIs for managing edge instances (control mode only):

```bash
# List connected edges
curl "/api/v1/edges"

# Get edge details
curl "/api/v1/edges/edge-1"

# Force configuration sync to specific edge
curl -X POST "/api/v1/edges/edge-1/sync"
```

## Operational Considerations

### 1. Security

- **Authentication**: All gRPC communication requires authentication tokens
- **TLS**: Optional TLS encryption for control-edge communication  
- **Network Isolation**: Edges only need outbound access to control instance
- **Token Rotation**: Support for rotating authentication tokens

### 2. Monitoring

- **Health Checks**: Built-in health endpoints for all instances
- **Metrics**: Prometheus-compatible metrics for monitoring
- **Logging**: Structured logging with correlation IDs
- **Alerting**: Edge disconnection and sync failure alerts

### 3. Backup and Recovery

- **Control Instance**: Standard database backup procedures
- **Edge Instances**: Stateless - can be recreated from control
- **Configuration Versioning**: Track configuration changes over time

### 4. Performance

- **Local Caching**: Edge instances cache configuration locally
- **Efficient Sync**: Only changed configurations are propagated
- **Connection Pooling**: Optimized gRPC connection management
- **Namespace Filtering**: Reduces configuration payload size

## Troubleshooting

### Common Issues

#### Edge Cannot Connect to Control
```bash
# Check network connectivity
nc -zv control.example.com 9090

# Verify TLS configuration
openssl s_client -connect control.example.com:9090

# Check authentication token
EDGE_AUTH_TOKEN=wrong-token ./microgateway
```

#### Configuration Not Syncing
```bash
# Force full sync from edge
curl -X POST "http://edge:8080/api/v1/sync"

# Check control instance logs
tail -f /var/log/microgateway/control.log

# Verify namespace configuration
echo $EDGE_NAMESPACE
```

#### Performance Issues
```bash
# Monitor gRPC connections
ss -tulpn | grep :9090

# Check database performance (control)
EXPLAIN SELECT * FROM llms WHERE namespace = 'tenant-1';

# Monitor memory usage (edge)  
ps aux | grep microgateway
```

### Debugging Commands

```bash
# Enable debug logging
export LOG_LEVEL=debug

# Test configuration provider
./microgateway -test-config-provider

# Dump current configuration cache (edge)
curl "http://localhost:8080/api/v1/debug/config-cache"

# Show edge instance status (control)  
curl "http://localhost:8080/api/v1/debug/edges"
```

## Migration Path

### From Standalone to Hub-and-Spoke

1. **Backup existing installation**
   ```bash
   pg_dump microgateway_db > backup.sql
   ```

2. **Run database migration**  
   ```bash
   ./microgateway -migrate
   ```

3. **Convert to control mode**
   ```bash
   # Add environment variable
   GATEWAY_MODE=control ./microgateway
   ```

4. **Deploy edge instances**
   ```bash
   GATEWAY_MODE=edge \
   CONTROL_ENDPOINT=control:9090 \
   ./microgateway
   ```

5. **Verify deployment**
   ```bash
   curl "http://control:8080/api/v1/edges"
   ```

This implementation provides a robust, scalable foundation for distributed microgateway deployments while maintaining backward compatibility with existing installations.