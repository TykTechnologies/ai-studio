# Hub-and-Spoke Operations Guide

This guide covers operational procedures, monitoring, troubleshooting, and maintenance for hub-and-spoke microgateway deployments.

## Table of Contents

1. [Health Monitoring](#health-monitoring)
2. [Configuration Management](#configuration-management)
3. [Edge Instance Management](#edge-instance-management)
4. [Troubleshooting](#troubleshooting)
5. [Performance Monitoring](#performance-monitoring)
6. [Backup and Recovery](#backup-and-recovery)
7. [Scaling Operations](#scaling-operations)
8. [Security Operations](#security-operations)

## Health Monitoring

### System Health Checks

**Control Instance Health:**
```bash
# Basic health check
curl -f http://control:8080/health

# Detailed status
curl http://control:8080/api/v1/status

# Database connectivity
curl http://control:8080/api/v1/health/database

# gRPC service status
curl http://control:8080/api/v1/health/grpc
```

**Edge Instance Health:**
```bash
# Basic health check
curl -f http://edge:8080/health

# Sync status with control
curl http://edge:8080/api/v1/sync/status

# Configuration cache status
curl http://edge:8080/api/v1/config/status

# Connection status
curl http://edge:8080/api/v1/connection/status
```

### Monitoring Endpoints

| Endpoint | Description | Response Format |
|----------|-------------|-----------------|
| `/health` | Basic health status | `{"status": "healthy"}` |
| `/ready` | Readiness for traffic | `{"ready": true}` |
| `/metrics` | Prometheus metrics | Prometheus format |
| `/api/v1/status` | Detailed system status | JSON with components |

### Prometheus Metrics

**Control Instance Metrics:**
```prometheus
# Edge connections
microgateway_edge_connections_total
microgateway_edge_connections_active

# Configuration changes
microgateway_config_changes_total
microgateway_config_propagation_duration_seconds

# gRPC operations
microgateway_grpc_requests_total
microgateway_grpc_request_duration_seconds
```

**Edge Instance Metrics:**
```prometheus
# Sync operations
microgateway_sync_operations_total
microgateway_sync_duration_seconds
microgateway_sync_failures_total

# Configuration cache
microgateway_config_cache_size_bytes
microgateway_config_cache_age_seconds

# Connection status
microgateway_control_connection_status
microgateway_reconnection_attempts_total
```

### Health Check Scripts

**Control Instance Monitor:**
```bash
#!/bin/bash
# control-health-check.sh

CONTROL_URL="http://localhost:8080"
ALERT_WEBHOOK="https://hooks.slack.com/services/YOUR/WEBHOOK/URL"

check_health() {
    local endpoint=$1
    local name=$2
    
    if ! curl -sf "${CONTROL_URL}${endpoint}" > /dev/null; then
        echo "ALERT: Control instance ${name} check failed"
        curl -X POST -H 'Content-type: application/json' \
            --data "{\"text\":\"🚨 Control instance ${name} health check failed\"}" \
            "$ALERT_WEBHOOK"
        return 1
    fi
    return 0
}

# Run health checks
check_health "/health" "basic health" || exit 1
check_health "/api/v1/health/database" "database" || exit 1
check_health "/api/v1/health/grpc" "grpc service" || exit 1

# Check edge connections
EDGE_COUNT=$(curl -s "${CONTROL_URL}/api/v1/edges" | jq '.edges | length')
if [ "$EDGE_COUNT" -eq 0 ]; then
    echo "WARNING: No edge instances connected"
fi

echo "Control instance health: OK (${EDGE_COUNT} edges connected)"
```

**Edge Instance Monitor:**
```bash
#!/bin/bash
# edge-health-check.sh

EDGE_URL="http://localhost:8080"
EDGE_ID="${EDGE_ID:-$(hostname)}"

# Check basic health
if ! curl -sf "${EDGE_URL}/health" > /dev/null; then
    echo "CRITICAL: Edge instance health check failed"
    exit 2
fi

# Check control connection
CONNECTED=$(curl -s "${EDGE_URL}/api/v1/connection/status" | jq -r '.connected')
if [ "$CONNECTED" != "true" ]; then
    echo "WARNING: Edge ${EDGE_ID} not connected to control"
    exit 1
fi

# Check configuration cache age
CACHE_AGE=$(curl -s "${EDGE_URL}/api/v1/config/status" | jq -r '.cache_age_seconds')
if [ "$CACHE_AGE" -gt 3600 ]; then
    echo "WARNING: Edge ${EDGE_ID} configuration cache is ${CACHE_AGE}s old"
    exit 1
fi

echo "Edge instance health: OK"
```

## Configuration Management

### Managing LLM Configurations

**Create Global LLM:**
```bash
curl -X POST http://control:8080/api/v1/llms \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -d '{
    "name": "OpenAI GPT-4",
    "vendor": "openai",
    "namespace": "",
    "api_key": "sk-...",
    "default_model": "gpt-4",
    "max_tokens": 4096,
    "is_active": true
  }'
```

**Create Tenant-Specific LLM:**
```bash
curl -X POST http://control:8080/api/v1/llms \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -d '{
    "name": "Acme Claude-3",
    "vendor": "anthropic", 
    "namespace": "acme",
    "api_key": "sk-ant-...",
    "default_model": "claude-3-sonnet-20240229",
    "is_active": true
  }'
```

**List Configurations by Namespace:**
```bash
# Global configurations
curl "http://control:8080/api/v1/llms?namespace="

# Tenant-specific configurations
curl "http://control:8080/api/v1/llms?namespace=acme"

# All configurations (admin only)
curl -H "Authorization: Bearer $ADMIN_TOKEN" \
  "http://control:8080/api/v1/llms"
```

### Configuration Validation

**Validate Configuration Before Apply:**
```bash
# Dry-run configuration change
curl -X POST http://control:8080/api/v1/llms/validate \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Test LLM",
    "vendor": "openai",
    "api_key": "sk-test...",
    "default_model": "gpt-3.5-turbo"
  }'
```

**Test LLM Connectivity:**
```bash
# Test LLM from control instance
curl -X POST http://control:8080/api/v1/llms/123/test \
  -H "Authorization: Bearer $ADMIN_TOKEN"

# Test LLM from edge instance
curl -X POST http://edge:8080/api/v1/llms/123/test
```

### Configuration Rollback

**View Configuration History:**
```bash
curl "http://control:8080/api/v1/config/history?limit=10"
```

**Rollback to Previous Version:**
```bash
curl -X POST http://control:8080/api/v1/config/rollback \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -d '{"version": "2024-01-15T10:30:00Z"}'
```

## Edge Instance Management

### Edge Registration and Discovery

**List Connected Edges:**
```bash
curl http://control:8080/api/v1/edges | jq '.edges[] | {id, namespace, status, last_seen}'
```

**Get Edge Details:**
```bash
curl http://control:8080/api/v1/edges/edge-prod-us-west-1 | jq .
```

**Force Edge Synchronization:**
```bash
curl -X POST http://control:8080/api/v1/edges/edge-prod-us-west-1/sync
```

### Edge Lifecycle Management

**Gracefully Shutdown Edge:**
```bash
# Send shutdown signal to specific edge
curl -X POST http://control:8080/api/v1/edges/edge-prod-us-west-1/shutdown \
  -H "Authorization: Bearer $ADMIN_TOKEN"

# Drain edge (stop accepting new requests)
curl -X POST http://edge:8080/api/v1/drain
```

**Edge Restart Procedure:**
```bash
#!/bin/bash
# restart-edge.sh

EDGE_ID=$1
EDGE_URL="http://${EDGE_ID}:8080"

echo "Draining edge ${EDGE_ID}..."
curl -X POST "${EDGE_URL}/api/v1/drain"

echo "Waiting for active requests to complete..."
sleep 30

echo "Stopping edge instance..."
systemctl stop microgateway

echo "Starting edge instance..."
systemctl start microgateway

echo "Waiting for edge to reconnect..."
sleep 10

echo "Verifying edge health..."
curl -f "${EDGE_URL}/health"

echo "Edge ${EDGE_ID} restart complete"
```

### Multi-Edge Operations

**Bulk Configuration Update:**
```bash
#!/bin/bash
# bulk-edge-sync.sh

CONTROL_URL="http://control:8080"
EDGES=$(curl -s "${CONTROL_URL}/api/v1/edges" | jq -r '.edges[].id')

for edge in $EDGES; do
    echo "Syncing edge: $edge"
    curl -X POST "${CONTROL_URL}/api/v1/edges/${edge}/sync"
    sleep 1
done
```

**Rolling Edge Updates:**
```bash
#!/bin/bash
# rolling-edge-update.sh

NAMESPACE=$1
MAX_UNAVAILABLE=1

echo "Starting rolling update for namespace: ${NAMESPACE}"

EDGES=$(curl -s "http://control:8080/api/v1/edges?namespace=${NAMESPACE}" | jq -r '.edges[].id')
TOTAL_EDGES=$(echo "$EDGES" | wc -l)

echo "Total edges to update: $TOTAL_EDGES"

for edge in $EDGES; do
    echo "Updating edge: $edge"
    
    # Update edge (implementation depends on deployment method)
    kubectl set image deployment/microgateway-edge-${edge} \
        microgateway=microgateway:latest
    
    # Wait for rollout
    kubectl rollout status deployment/microgateway-edge-${edge}
    
    # Verify health
    sleep 10
    if ! curl -sf "http://${edge}:8080/health"; then
        echo "ERROR: Edge $edge health check failed after update"
        exit 1
    fi
    
    echo "Edge $edge updated successfully"
done

echo "Rolling update complete"
```

## Troubleshooting

### Common Issues and Solutions

#### Edge Cannot Connect to Control

**Symptoms:**
- Edge logs show connection failures
- Edge status shows "disconnected"
- Configuration not syncing

**Diagnosis:**
```bash
# Test network connectivity
nc -zv control.example.com 9090

# Check DNS resolution
nslookup control.example.com

# Test gRPC endpoint
grpc_cli call control.example.com:9090 \
  microgateway.ConfigurationSyncService.RegisterEdge \
  'edge_id: "test", edge_namespace: "test"'

# Check authentication
curl -H "Authorization: Bearer $TOKEN" \
  http://control:8080/api/v1/edges
```

**Solutions:**
```bash
# 1. Check network connectivity
ping control.example.com

# 2. Verify gRPC port accessibility
telnet control.example.com 9090

# 3. Check authentication token
export EDGE_AUTH_TOKEN=correct-token

# 4. Verify control instance health
curl http://control:8080/health

# 5. Restart edge with debug logging
LOG_LEVEL=debug ./microgateway
```

#### Configuration Not Syncing

**Symptoms:**
- Edge shows old configuration
- Recent changes not visible on edges
- Sync status shows errors

**Diagnosis:**
```bash
# Check control instance propagation
curl http://control:8080/api/v1/debug/propagation

# Check edge sync status
curl http://edge:8080/api/v1/sync/status

# Verify namespace filtering
curl "http://control:8080/api/v1/llms?namespace=production"
curl "http://edge:8080/api/v1/llms"
```

**Solutions:**
```bash
# 1. Force full synchronization
curl -X POST http://edge:8080/api/v1/sync/full

# 2. Check namespace configuration
echo "Edge namespace: $EDGE_NAMESPACE"

# 3. Verify control instance has changes
curl http://control:8080/api/v1/config/version

# 4. Restart edge instance
systemctl restart microgateway
```

#### High Memory Usage on Edge

**Symptoms:**
- Edge instances consuming excessive memory
- OOM kills in container environments
- Slow response times

**Diagnosis:**
```bash
# Check configuration cache size
curl http://edge:8080/api/v1/config/cache/stats

# Monitor memory usage
ps aux | grep microgateway
docker stats microgateway-edge

# Check for memory leaks
pprof -http=:6060 http://edge:8080/debug/pprof/heap
```

**Solutions:**
```bash
# 1. Limit configuration cache size
export CONFIG_CACHE_MAX_SIZE=50MB

# 2. Reduce sync frequency
export EDGE_HEARTBEAT_INTERVAL=60s

# 3. Filter configurations by namespace
export EDGE_NAMESPACE=production

# 4. Increase memory limits (Kubernetes)
kubectl patch deployment microgateway-edge -p \
  '{"spec":{"template":{"spec":{"containers":[{"name":"microgateway","resources":{"limits":{"memory":"256Mi"}}}]}}}}'
```

### Debug Mode Operations

**Enable Debug Logging:**
```bash
# Temporary debug mode
curl -X POST http://control:8080/api/v1/debug/logging \
  -d '{"level": "debug", "duration": "10m"}'

# Restart with debug
LOG_LEVEL=debug ./microgateway
```

**Generate Debug Report:**
```bash
#!/bin/bash
# generate-debug-report.sh

INSTANCE_TYPE=$1  # control or edge
INSTANCE_URL="http://localhost:8080"

echo "Generating debug report for ${INSTANCE_TYPE} instance..."

mkdir -p debug-report-$(date +%Y%m%d-%H%M%S)
cd debug-report-*

# Collect system information
echo "System Information" > system.txt
uname -a >> system.txt
free -h >> system.txt
df -h >> system.txt

# Collect service status
curl -s "${INSTANCE_URL}/health" > health.json
curl -s "${INSTANCE_URL}/api/v1/status" > status.json
curl -s "${INSTANCE_URL}/metrics" > metrics.txt

if [ "$INSTANCE_TYPE" = "control" ]; then
    # Control-specific information
    curl -s "${INSTANCE_URL}/api/v1/edges" > edges.json
    curl -s "${INSTANCE_URL}/api/v1/config/version" > config-version.json
elif [ "$INSTANCE_TYPE" = "edge" ]; then
    # Edge-specific information
    curl -s "${INSTANCE_URL}/api/v1/sync/status" > sync-status.json
    curl -s "${INSTANCE_URL}/api/v1/connection/status" > connection-status.json
    curl -s "${INSTANCE_URL}/api/v1/config/cache/stats" > cache-stats.json
fi

# Collect logs (last 1000 lines)
journalctl -u microgateway --lines=1000 > service.log 2>/dev/null || \
docker logs --tail=1000 microgateway > service.log 2>/dev/null || \
tail -1000 /var/log/microgateway.log > service.log 2>/dev/null

echo "Debug report generated in $(pwd)"
```

## Performance Monitoring

### Key Performance Indicators

**Control Instance KPIs:**
- Edge connection count and stability
- Configuration propagation latency
- Database query performance
- gRPC request throughput and latency

**Edge Instance KPIs:**
- Request processing latency
- Configuration sync frequency and success rate
- Memory usage and cache efficiency
- Connection uptime to control

### Performance Monitoring Queries

**Prometheus Queries:**
```prometheus
# Edge connection stability
rate(microgateway_edge_connections_total[5m])

# Configuration propagation latency
histogram_quantile(0.95, 
  rate(microgateway_config_propagation_duration_seconds_bucket[5m]))

# Edge sync success rate
rate(microgateway_sync_operations_total{result="success"}[5m]) / 
rate(microgateway_sync_operations_total[5m])

# Request processing latency
histogram_quantile(0.95, 
  rate(microgateway_request_duration_seconds_bucket[5m]))
```

### Performance Optimization

**Database Performance (Control):**
```sql
-- Analyze query performance
EXPLAIN ANALYZE SELECT * FROM llms WHERE namespace = 'production' AND is_active = true;

-- Create optimized indexes
CREATE INDEX CONCURRENTLY idx_llms_namespace_active_optimized 
ON llms (namespace, is_active) WHERE is_active = true;

-- Update table statistics
ANALYZE llms, apps, api_tokens;
```

**Memory Optimization (Edge):**
```bash
# Configure cache limits
export CONFIG_CACHE_MAX_SIZE=32MB
export CONFIG_CACHE_TTL=1h

# Optimize garbage collection
export GOGC=100
export GOMEMLIMIT=128MB
```

**Network Optimization:**
```bash
# Optimize gRPC keepalive settings
export GRPC_KEEPALIVE_TIME=30s
export GRPC_KEEPALIVE_TIMEOUT=5s
export GRPC_KEEPALIVE_PERMIT_WITHOUT_CALLS=true

# Reduce sync frequency for stable environments
export EDGE_HEARTBEAT_INTERVAL=60s
export EDGE_RECONNECT_INTERVAL=10s
```

## Backup and Recovery

### Control Instance Backup

**Database Backup:**
```bash
#!/bin/bash
# backup-control-db.sh

BACKUP_DIR="/var/backups/microgateway"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
DB_NAME="microgateway"

# Create backup directory
mkdir -p "$BACKUP_DIR"

# PostgreSQL backup
pg_dump "$DB_NAME" | gzip > "$BACKUP_DIR/microgateway_${TIMESTAMP}.sql.gz"

# Verify backup
gunzip -t "$BACKUP_DIR/microgateway_${TIMESTAMP}.sql.gz"

# Cleanup old backups (keep 30 days)
find "$BACKUP_DIR" -name "microgateway_*.sql.gz" -mtime +30 -delete

echo "Backup completed: microgateway_${TIMESTAMP}.sql.gz"
```

**Configuration Export:**
```bash
# Export all configurations
curl -H "Authorization: Bearer $ADMIN_TOKEN" \
  "http://control:8080/api/v1/config/export" > config-backup-$(date +%Y%m%d).json

# Export specific namespace
curl -H "Authorization: Bearer $ADMIN_TOKEN" \
  "http://control:8080/api/v1/config/export?namespace=production" > \
  config-production-$(date +%Y%m%d).json
```

### Disaster Recovery

**Control Instance Recovery:**
```bash
#!/bin/bash
# recover-control-instance.sh

BACKUP_FILE=$1
DB_NAME="microgateway"

if [ -z "$BACKUP_FILE" ]; then
    echo "Usage: $0 <backup-file>"
    exit 1
fi

echo "Recovering control instance from $BACKUP_FILE..."

# Stop control instance
systemctl stop microgateway

# Restore database
dropdb "$DB_NAME" 2>/dev/null || true
createdb "$DB_NAME"
gunzip -c "$BACKUP_FILE" | psql "$DB_NAME"

# Verify database
psql "$DB_NAME" -c "SELECT COUNT(*) FROM llms;"

# Start control instance
systemctl start microgateway

# Wait for startup
sleep 10

# Verify recovery
curl -f http://localhost:8080/health

echo "Recovery complete"
```

**Edge Instance Recovery:**
```bash
#!/bin/bash
# recover-edge-instance.sh

EDGE_ID=$1

echo "Recovering edge instance: $EDGE_ID"

# Edge instances are stateless - just restart and resync
systemctl restart microgateway

# Wait for connection
sleep 15

# Force full sync
curl -X POST http://localhost:8080/api/v1/sync/full

# Verify sync status
curl http://localhost:8080/api/v1/sync/status

echo "Edge recovery complete"
```

### Point-in-Time Recovery

**Configuration Rollback:**
```bash
# List available configuration versions
curl "http://control:8080/api/v1/config/versions?limit=20"

# Rollback to specific version
curl -X POST http://control:8080/api/v1/config/rollback \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -d '{
    "version": "2024-01-15T14:30:00Z",
    "reason": "Rollback due to configuration issue",
    "force": false
  }'
```

## Scaling Operations

### Horizontal Scaling

**Scale Edge Instances:**
```bash
# Kubernetes scaling
kubectl scale deployment microgateway-edge --replicas=10

# Docker Swarm scaling
docker service scale microgateway_edge=10

# Manual scaling with configuration
for i in {6..10}; do
    EDGE_ID="edge-prod-${i}" \
    EDGE_NAMESPACE="production" \
    docker run -d --name "edge-${i}" \
      -e GATEWAY_MODE=edge \
      -e CONTROL_ENDPOINT=control:9090 \
      -e EDGE_ID="edge-prod-${i}" \
      -e EDGE_NAMESPACE="production" \
      microgateway:latest
done
```

**Auto-Scaling Based on Metrics:**
```yaml
# Kubernetes HPA with custom metrics
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: microgateway-edge-hpa
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: microgateway-edge
  minReplicas: 3
  maxReplicas: 50
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 70
  - type: Pods
    pods:
      metric:
        name: microgateway_requests_per_second
      target:
        type: AverageValue
        averageValue: "100"
```

### Vertical Scaling

**Resource Optimization:**
```bash
# Monitor resource usage
kubectl top pods -l app=microgateway-edge

# Adjust resource limits
kubectl patch deployment microgateway-edge -p \
  '{"spec":{"template":{"spec":{"containers":[{
    "name":"microgateway",
    "resources":{
      "requests":{"cpu":"100m","memory":"128Mi"},
      "limits":{"cpu":"500m","memory":"256Mi"}
    }
  }]}}}}'
```

## Security Operations

### Certificate Management

**Rotate TLS Certificates:**
```bash
#!/bin/bash
# rotate-certificates.sh

CERT_DIR="/etc/microgateway/certs"
BACKUP_DIR="/var/backups/certs"

# Backup existing certificates
mkdir -p "$BACKUP_DIR"
cp "$CERT_DIR"/*.{crt,key} "$BACKUP_DIR"/

# Generate new certificates (example with Let's Encrypt)
certbot certonly --standalone \
  -d control.microgateway.company.com \
  --cert-path "$CERT_DIR/server.crt" \
  --key-path "$CERT_DIR/server.key"

# Restart control instance to load new certificates
systemctl restart microgateway

# Verify certificate
openssl x509 -in "$CERT_DIR/server.crt" -text -noout | grep "Not After"
```

**Token Rotation:**
```bash
#!/bin/bash
# rotate-auth-tokens.sh

NEW_TOKEN=$(openssl rand -hex 32)
OLD_TOKEN=$GRPC_AUTH_TOKEN

echo "Generated new token: $NEW_TOKEN"

# Update control instance (supports both old and new tokens during rotation)
curl -X POST http://control:8080/api/v1/auth/tokens \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -d "{\"token\": \"$NEW_TOKEN\", \"grace_period\": \"1h\"}"

# Update edge instances one by one
EDGES=$(curl -s http://control:8080/api/v1/edges | jq -r '.edges[].id')

for edge in $EDGES; do
    echo "Updating token for edge: $edge"
    
    # Update edge configuration (implementation varies by deployment)
    kubectl patch deployment "microgateway-edge-${edge}" -p \
      "{\"spec\":{\"template\":{\"spec\":{\"containers\":[{
        \"name\":\"microgateway\",
        \"env\":[{\"name\":\"EDGE_AUTH_TOKEN\",\"value\":\"$NEW_TOKEN\"}]
      }]}}}}"
    
    # Wait for rollout
    kubectl rollout status deployment "microgateway-edge-${edge}"
    
    # Verify connection
    sleep 10
    if curl -sf "http://${edge}:8080/health"; then
        echo "Edge $edge updated successfully"
    else
        echo "ERROR: Edge $edge failed after token update"
        exit 1
    fi
done

# Remove old token from control instance
curl -X DELETE http://control:8080/api/v1/auth/tokens \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -d "{\"token\": \"$OLD_TOKEN\"}"

echo "Token rotation complete"
```

### Security Auditing

**Access Audit:**
```bash
# Review authentication logs
curl -H "Authorization: Bearer $ADMIN_TOKEN" \
  "http://control:8080/api/v1/audit/auth?hours=24" | jq .

# Review configuration changes
curl -H "Authorization: Bearer $ADMIN_TOKEN" \
  "http://control:8080/api/v1/audit/config?hours=24" | jq .

# Review edge connections
curl -H "Authorization: Bearer $ADMIN_TOKEN" \
  "http://control:8080/api/v1/audit/edges?hours=24" | jq .
```

**Security Baseline Check:**
```bash
#!/bin/bash
# security-baseline-check.sh

echo "Performing security baseline check..."

# Check TLS configuration
if curl -k https://control:9090 2>/dev/null; then
    echo "✓ TLS enabled on gRPC port"
else
    echo "⚠ TLS not configured on gRPC port"
fi

# Check authentication
if [ -n "$GRPC_AUTH_TOKEN" ]; then
    echo "✓ Authentication token configured"
else
    echo "⚠ No authentication token set"
fi

# Check database encryption
DB_ENCRYPTED=$(psql -t -c "SELECT setting FROM pg_settings WHERE name='ssl';" 2>/dev/null)
if [ "$DB_ENCRYPTED" = "on" ]; then
    echo "✓ Database SSL enabled"
else
    echo "⚠ Database SSL not configured"
fi

# Check certificate expiration
if [ -f "/etc/microgateway/certs/server.crt" ]; then
    EXPIRY=$(openssl x509 -enddate -noout -in /etc/microgateway/certs/server.crt | cut -d= -f2)
    EXPIRY_EPOCH=$(date -d "$EXPIRY" +%s)
    CURRENT_EPOCH=$(date +%s)
    DAYS_LEFT=$(( (EXPIRY_EPOCH - CURRENT_EPOCH) / 86400 ))
    
    if [ $DAYS_LEFT -gt 30 ]; then
        echo "✓ Certificate valid for $DAYS_LEFT days"
    else
        echo "⚠ Certificate expires in $DAYS_LEFT days"
    fi
fi

echo "Security check complete"
```

This operations guide provides comprehensive procedures for managing hub-and-spoke microgateway deployments. For troubleshooting specific issues, see the [Troubleshooting Guide](./troubleshooting.md).