# Hub and Spoke Microgateway Controller to Edge

This guide covers the detailed implementation of microgateway controller-to-edge communication in hub-and-spoke deployments.

## Overview

Controller-to-edge communication enables:
- **Centralized Configuration Management**: Single point of control for all edge instances
- **Real-Time Synchronization**: Immediate propagation of configuration changes
- **Namespace-Based Filtering**: Selective configuration distribution
- **Edge Autonomy**: Independent operation with cached configuration
- **Secure Communication**: Authenticated gRPC communication
- **Fault Tolerance**: Graceful handling of network partitions

## Communication Architecture

### gRPC Protocol
```
Control Instance (Hub)           Edge Instance (Spoke)
┌─────────────────┐              ┌─────────────────┐
│  gRPC Server    │◄────────────►│  gRPC Client    │
│  Port 50051     │              │  Reconnecting   │
│  Database       │              │  Local Cache    │
└─────────────────┘              └─────────────────┘
```

### Communication Flow
1. **Edge Registration**: Edge connects and registers with control
2. **Initial Sync**: Control sends full configuration to edge
3. **Real-Time Updates**: Control streams configuration changes
4. **Heartbeat**: Regular health checks between control and edge
5. **Reconnection**: Automatic reconnection on network issues

## Control Instance Configuration

### Environment Variables
```bash
# Gateway mode
GATEWAY_MODE=control

# gRPC server configuration
GRPC_PORT=50051
GRPC_HOST=0.0.0.0
GRPC_TLS_ENABLED=false
GRPC_TLS_CERT_PATH=/path/to/cert.pem
GRPC_TLS_KEY_PATH=/path/to/key.pem
GRPC_AUTH_TOKEN=secure-auth-token

# Database configuration (required)
DATABASE_TYPE=postgres
DATABASE_DSN="postgres://user:pass@postgres:5432/microgateway"

# Edge management
MAX_EDGE_CONNECTIONS=100
EDGE_HEARTBEAT_TIMEOUT=60s
CONFIG_PROPAGATION_TIMEOUT=30s
```

### Control Instance Startup
```bash
# Start control instance
GATEWAY_MODE=control \
GRPC_PORT=50051 \
GRPC_AUTH_TOKEN=my-secure-token \
DATABASE_TYPE=postgres \
DATABASE_DSN="postgres://user:pass@localhost/control_db" \
./microgateway

# Control instance provides:
# - HTTP API on port 8080 (default)
# - gRPC API on port 50051
# - Configuration management
# - Edge monitoring
```

## Edge Instance Configuration

### Environment Variables
```bash
# Gateway mode
GATEWAY_MODE=edge

# Control connection
CONTROL_ENDPOINT=control.example.com:50051
EDGE_ID=edge-region-1
EDGE_NAMESPACE=production
EDGE_AUTH_TOKEN=secure-auth-token

# TLS configuration (optional)
EDGE_TLS_ENABLED=false
EDGE_TLS_CERT_PATH=/path/to/client-cert.pem
EDGE_TLS_KEY_PATH=/path/to/client-key.pem
EDGE_TLS_CA_PATH=/path/to/ca.pem
EDGE_SKIP_TLS_VERIFY=false

# Reconnection settings
EDGE_RECONNECT_INTERVAL=5s
EDGE_HEARTBEAT_INTERVAL=30s
EDGE_SYNC_TIMEOUT=10s
EDGE_MAX_RECONNECT_ATTEMPTS=10
```

### Edge Instance Startup
```bash
# Start edge instance
GATEWAY_MODE=edge \
CONTROL_ENDPOINT=control.example.com:50051 \
EDGE_ID=edge-region-1 \
EDGE_NAMESPACE=production \
EDGE_AUTH_TOKEN=my-secure-token \
./microgateway

# Edge instance provides:
# - HTTP API on port 8080 (default)
# - LLM proxy endpoints
# - Local configuration cache
# - Independent operation capability
```

## Configuration Propagation

### Synchronization Process
```
1. Edge connects to control
2. Control validates edge authentication
3. Control sends initial configuration (filtered by namespace)
4. Edge caches configuration locally
5. Control streams configuration changes
6. Edge updates local cache
7. Configuration changes take effect immediately
```

### Namespace Filtering
```sql
-- Control filters configuration by namespace
SELECT * FROM llms 
WHERE namespace = '' OR namespace = @edge_namespace;

SELECT * FROM apps 
WHERE namespace = '' OR namespace = @edge_namespace;

-- Global namespace (empty string) visible to all edges
-- Specific namespaces only visible to matching edges
```

### Explicit Configuration Push
```bash
# Administrators trigger configuration pushes explicitly
mgw namespace reload tenant-a    # Push to all edges in namespace
mgw edge reload edge-1 edge-2   # Push to specific edges

# Real-time status monitoring
mgw namespace reload tenant-a --watch

# API-based push operations
curl -X POST /api/v1/namespace/reload -d '{"namespace": "tenant-a"}'
```

## Edge Management

### Edge Registration
```json
{
  "edge_id": "edge-region-1",
  "namespace": "production",
  "version": "v1.0.0",
  "build_hash": "abc123def456",
  "metadata": {
    "region": "us-west-1",
    "environment": "production",
    "instance_type": "standard"
  }
}
```

### Edge Status Monitoring
```bash
# List connected edges (control instance)
curl http://control:8080/api/v1/edges

# Get specific edge status
curl http://control:8080/api/v1/edges/edge-region-1

# Example response:
{
  "data": {
    "edge_id": "edge-region-1",
    "namespace": "production",
    "status": "connected",
    "last_heartbeat": "2024-01-01T12:00:00Z",
    "version": "v1.0.0",
    "uptime": "24h30m"
  }
}
```

### Edge Operations
```bash
# Force configuration sync to edge
curl -X POST http://control:8080/api/v1/edges/edge-region-1/sync

# Restart edge instance
curl -X POST http://control:8080/api/v1/edges/edge-region-1/restart

# Get edge configuration cache
curl http://edge:8080/api/v1/cache/config
```

## Network Configuration

### Security Settings
```bash
# TLS configuration for secure communication
GRPC_TLS_ENABLED=true
GRPC_TLS_CERT_PATH=/etc/ssl/certs/control.crt
GRPC_TLS_KEY_PATH=/etc/ssl/private/control.key

# Edge TLS configuration
EDGE_TLS_ENABLED=true
EDGE_TLS_CA_PATH=/etc/ssl/certs/ca.crt
EDGE_TLS_CERT_PATH=/etc/ssl/certs/edge.crt
EDGE_TLS_KEY_PATH=/etc/ssl/private/edge.key
```

### Firewall Configuration
```bash
# Control instance (inbound)
# Port 8080: HTTP API
# Port 50051: gRPC API

# Edge instance (outbound)
# Port 50051: gRPC to control instance
# Port 8080: HTTP API (local)

# Example firewall rules
iptables -A INPUT -p tcp --dport 50051 -s edge-subnet -j ACCEPT  # Control
iptables -A OUTPUT -p tcp --dport 50051 -d control-host -j ACCEPT # Edge
```

## High Availability

### Control Instance HA
```yaml
# High availability control setup
services:
  control-1:
    environment:
      GATEWAY_MODE: control
      GRPC_PORT: 50051
      DATABASE_DSN: postgres://user:pass@postgres-cluster:5432/microgateway

  control-2:
    environment:
      GATEWAY_MODE: control
      GRPC_PORT: 50051
      DATABASE_DSN: postgres://user:pass@postgres-cluster:5432/microgateway
  
  # Load balancer for edge connections
  control-lb:
    ports:
      - "50051:50051"
    upstream:
      - control-1:50051
      - control-2:50051
```

### Edge Failover
```bash
# Edge with multiple control endpoints
CONTROL_ENDPOINT=control-primary.company.com:50051,control-backup.company.com:50051
EDGE_FAILOVER_ENABLED=true
EDGE_FAILOVER_TIMEOUT=30s

# Edge automatically switches to backup if primary fails
```

### Database Clustering
```bash
# PostgreSQL clustering for control instance
DATABASE_DSN="postgres://user:pass@postgres-primary:5432/microgateway?fallback_application_name=postgres-secondary:5432"

# Database replication ensures configuration availability
```

## Performance Optimization

### gRPC Performance
```bash
# gRPC server tuning
GRPC_MAX_CONCURRENT_STREAMS=1000
GRPC_KEEPALIVE_TIME=30s
GRPC_KEEPALIVE_TIMEOUT=5s
GRPC_MAX_CONNECTION_IDLE=5m

# Edge client tuning
EDGE_MAX_RECEIVE_MESSAGE_SIZE=4MB
EDGE_MAX_SEND_MESSAGE_SIZE=4MB
EDGE_KEEPALIVE_TIME=30s
```

### Configuration Caching
```bash
# Edge cache configuration
EDGE_CACHE_SIZE=1000
EDGE_CACHE_TTL=1h
EDGE_CACHE_CLEANUP_INTERVAL=10m

# Control cache configuration
CONTROL_EDGE_CACHE_SIZE=10000
CONTROL_CONFIG_CACHE_TTL=5m
```

### Batch Configuration Updates
```bash
# Batch configuration changes for efficiency
# Control batches multiple changes before propagation
CONFIG_BATCH_SIZE=100
CONFIG_BATCH_TIMEOUT=5s
CONFIG_PROPAGATION_WORKERS=10
```

## Monitoring and Debugging

### Control Instance Monitoring
```bash
# Monitor control instance
mgw system health

# Edge connection metrics
curl http://control:8080/metrics | grep edge_connections

# Configuration propagation metrics
curl http://control:8080/metrics | grep config_propagation
```

### Edge Instance Monitoring
```bash
# Monitor edge instance
mgw system health

# Connection status
mgw edge connection-status

# Configuration sync status
mgw edge sync-status

# Cache statistics
mgw edge cache-stats
```

### Debug Logging
```bash
# Enable debug logging on control
LOG_LEVEL=debug GATEWAY_MODE=control ./microgateway

# Enable debug logging on edge
LOG_LEVEL=debug GATEWAY_MODE=edge ./microgateway

# Monitor gRPC communication
GRPC_GO_LOG_VERBOSITY_LEVEL=99 \
GRPC_GO_LOG_SEVERITY_LEVEL=info \
./microgateway
```

## Troubleshooting

### Connection Issues
```bash
# Test network connectivity
nc -zv control.example.com 50051

# Check authentication
curl -H "Authorization: Bearer $EDGE_AUTH_TOKEN" \
  http://control:8080/api/v1/edges

# Monitor connection logs
tail -f /var/log/microgateway/edge.log | grep "control connection"
```

### Configuration Sync Issues
```bash
# Force full sync from edge
curl -X POST http://edge:8080/api/v1/sync

# Check control instance logs
tail -f /var/log/microgateway/control.log | grep "config sync"

# Verify namespace configuration
echo $EDGE_NAMESPACE
```

### Performance Issues
```bash
# Monitor gRPC performance
ss -tulpn | grep :50051

# Check database performance (control)
EXPLAIN SELECT * FROM llms WHERE namespace = 'tenant-1';

# Monitor memory usage (edge)
ps aux | grep microgateway
```

## Configuration Examples

### Simple Hub-and-Spoke
```yaml
# docker-compose.yml
version: '3.8'
services:
  control:
    image: microgateway:latest
    environment:
      GATEWAY_MODE: control
      DATABASE_DSN: postgres://user:pass@postgres:5432/microgateway
      GRPC_AUTH_TOKEN: secure-token
    ports:
      - "8080:8080"
      - "50051:50051"
  
  edge-1:
    image: microgateway:latest
    environment:
      GATEWAY_MODE: edge
      CONTROL_ENDPOINT: control:50051
      EDGE_ID: edge-1
      EDGE_NAMESPACE: production
      EDGE_AUTH_TOKEN: secure-token
    ports:
      - "8081:8080"
  
  postgres:
    image: postgres:15
    environment:
      POSTGRES_DB: microgateway
      POSTGRES_USER: user
      POSTGRES_PASSWORD: pass
```

### Production Hub-and-Spoke
```yaml
# Kubernetes deployment
apiVersion: apps/v1
kind: Deployment
metadata:
  name: microgateway-control
spec:
  replicas: 2
  template:
    spec:
      containers:
      - name: microgateway
        image: microgateway:v1.0.0
        env:
        - name: GATEWAY_MODE
          value: "control"
        - name: DATABASE_DSN
          valueFrom:
            secretKeyRef:
              name: db-credentials
              key: dsn
        ports:
        - containerPort: 8080
        - containerPort: 50051

---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: microgateway-edge
spec:
  replicas: 5
  template:
    spec:
      containers:
      - name: microgateway
        image: microgateway:v1.0.0
        env:
        - name: GATEWAY_MODE
          value: "edge"
        - name: CONTROL_ENDPOINT
          value: "control-service:50051"
        - name: EDGE_NAMESPACE
          value: "production"
        ports:
        - containerPort: 8080
```

## Security Implementation

### Authentication
```bash
# Generate secure authentication token
GRPC_AUTH_TOKEN=$(openssl rand -hex 32)

# Use same token on control and edge instances
# Control validates token on edge connection
# Edge presents token for authentication
```

### TLS Configuration
```bash
# Generate certificates for secure communication
# Control certificate
openssl req -x509 -newkey rsa:4096 \
  -keyout control.key -out control.crt \
  -days 365 -nodes \
  -subj "/CN=control.example.com"

# Edge certificate (client certificate)
openssl req -x509 -newkey rsa:4096 \
  -keyout edge.key -out edge.crt \
  -days 365 -nodes \
  -subj "/CN=edge.example.com"

# Control configuration
GRPC_TLS_ENABLED=true
GRPC_TLS_CERT_PATH=/etc/ssl/certs/control.crt
GRPC_TLS_KEY_PATH=/etc/ssl/private/control.key

# Edge configuration
EDGE_TLS_ENABLED=true
EDGE_TLS_CERT_PATH=/etc/ssl/certs/edge.crt
EDGE_TLS_KEY_PATH=/etc/ssl/private/edge.key
EDGE_TLS_CA_PATH=/etc/ssl/certs/control.crt
```

## Configuration Management

### Control Instance Operations
```bash
# Create LLM (propagated to all edges)
mgw llm create --name="Global GPT-4" --namespace="" --vendor=openai

# Create namespace-specific LLM
mgw llm create --name="Tenant LLM" --namespace="tenant-1" --vendor=openai

# Create application (propagated to matching edges)
mgw app create --name="App" --namespace="tenant-1" --email=user@tenant1.com
```

### Edge Configuration Filtering
```
Control Database:
├── LLM 1 (namespace: "")        → Sent to ALL edges
├── LLM 2 (namespace: "tenant-a") → Sent to tenant-a edges only
├── LLM 3 (namespace: "tenant-b") → Sent to tenant-b edges only
└── APP 1 (namespace: "tenant-a") → Sent to tenant-a edges only

Edge tenant-a receives: LLM 1, LLM 2, APP 1
Edge tenant-b receives: LLM 1, LLM 3
Edge global receives: LLM 1 (if namespace="")
```

## Edge Operations

### Edge Startup Process
```
1. Edge starts and reads environment configuration
2. Edge establishes gRPC connection to control
3. Edge authenticates using EDGE_AUTH_TOKEN
4. Edge registers with control (sends edge_id, namespace, metadata)
5. Control validates edge and adds to active edges list
6. Control sends initial configuration (filtered by namespace)
7. Edge caches configuration locally
8. Edge becomes ready to serve requests
9. Edge starts heartbeat process
```

### Edge Caching
```bash
# Edge cache structure
/var/lib/microgateway/edge-cache/
├── llms.json           # Cached LLM configurations
├── apps.json           # Cached application configurations
├── tokens.json         # Cached token configurations
├── model_prices.json   # Cached pricing information
└── sync_state.json     # Synchronization state
```

### Edge Failover
```bash
# Edge cache fallback
# If control is unavailable:
# 1. Edge continues using cached configuration
# 2. Edge logs warnings about control unavailability
# 3. Edge attempts reconnection per EDGE_RECONNECT_INTERVAL
# 4. Edge resumes normal operation when control returns

# Cache persistence ensures continued operation
```

## Explicit Configuration Push System

### Push Operation Process
```
1. Administrator modifies configuration via AI Studio GUI/API
2. Administrator explicitly triggers push via CLI, API, or GUI
3. ReloadCoordinator identifies target edges by namespace
4. Control sends reload requests to target edges via gRPC
5. Edges fetch fresh configuration from control server
6. Edges update local SQLite cache and apply changes
7. Real-time status tracking reports progress back to administrator
```

### Push Triggers
```bash
# CLI-based push operations
mgw namespace reload tenant-a              # Push to namespace
mgw edge reload edge-1 edge-2             # Push to specific edges
mgw namespace reload tenant-a --watch     # Monitor progress

# API-based push operations
POST /api/v1/namespace/reload {"namespace": "tenant-a"}
GET  /api/v1/namespace/reload/{operation-id}/status

# GUI-based push operations
# Available in AI Studio edge management interface
```

### Supported Configuration Types
```bash
# All configuration entities support push-based updates:
# - LLM configurations (endpoints, models, budgets)
# - Application settings (budgets, rate limits)
# - Authentication tokens and credentials
# - Model pricing configurations
# - Plugin and filter configurations
```

## Monitoring and Alerting

### Control Instance Metrics
```bash
# Edge connection metrics
curl http://control:8080/metrics | grep edge_

# Key metrics:
# - edge_connections_total
# - edge_connections_active
# - edge_reload_operations_total
# - edge_reload_operation_duration_seconds
# - edge_heartbeat_failures_total
```

### Edge Instance Metrics
```bash
# Edge metrics
curl http://edge:8080/metrics | grep edge_

# Key metrics:
# - edge_connection_status
# - edge_config_sync_total
# - edge_cache_hits_total
# - edge_control_unavailable_duration_seconds
```

### Health Monitoring
```bash
# Monitor edge health from control
mgw edge health --all

# Monitor control health from edge
mgw system health  # Includes control connection status

# Set up alerting for edge disconnections
# Alert if edge hasn't sent heartbeat in > 2 minutes
```

## Scalability Considerations

### Control Instance Scaling
```bash
# Vertical scaling
# - Increase CPU and memory for more edges
# - Optimize database for configuration queries
# - Tune gRPC server for concurrent connections

# Horizontal scaling (future)
# - Multiple control instances with shared database
# - Load balancing of edge connections
# - Configuration change coordination
```

### Edge Instance Scaling
```bash
# Edge instances are stateless
# - Scale horizontally by adding more edge instances
# - Load balance traffic across edges
# - Each edge operates independently

# Auto-scaling configuration
# Scale based on:
# - Request volume
# - CPU/memory usage
# - Response latency
```

### Database Scaling
```bash
# Database optimization for control instance
# - Index namespace columns for fast filtering
# - Optimize queries for configuration retrieval
# - Use read replicas for edge configuration queries
# - Implement connection pooling

# PostgreSQL configuration
max_connections = 200
shared_buffers = 256MB
effective_cache_size = 1GB
```

## Operational Procedures

### Adding New Edge
```bash
# 1. Deploy edge instance with configuration
GATEWAY_MODE=edge \
CONTROL_ENDPOINT=control:50051 \
EDGE_ID=new-edge-region-2 \
EDGE_NAMESPACE=production \
./microgateway

# 2. Verify edge registration
mgw edge list | grep new-edge-region-2

# 3. Monitor edge health
mgw edge health new-edge-region-2
```

### Edge Maintenance
```bash
# Graceful edge shutdown
kill -TERM $(pgrep microgateway)  # Edge process

# Edge automatically deregisters from control
# Control marks edge as disconnected
# Traffic routes to other edges

# Edge restart
# Configuration automatically syncs on reconnection
```

### Control Instance Maintenance
```bash
# Graceful control shutdown
# 1. Stop accepting new edge connections
# 2. Complete in-flight configuration updates
# 3. Notify connected edges of shutdown
# 4. Edges continue with cached configuration

# Control restart
# Edges automatically reconnect and sync
```

---

Controller-to-edge communication enables centralized management of distributed microgateway deployments. For AI Studio integration, see [AI Studio Controller](ai-studio-controller.md). For namespace management, see [Namespaces](namespaces.md).
