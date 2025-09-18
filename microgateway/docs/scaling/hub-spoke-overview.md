# Hub-and-Spoke Overview

The microgateway supports distributed hub-and-spoke architecture for scalable deployment across multiple environments and geographic regions.

## Overview

Hub-and-spoke architecture features:
- **Centralized Control**: Single control plane for distributed gateways
- **Edge Deployment**: Lightweight edge gateways for regional deployment
- **Configuration Propagation**: Real-time configuration sync across all instances
- **Namespace Isolation**: Multi-tenant configuration separation
- **High Availability**: Fault-tolerant distributed operation
- **Independent Operation**: Edge gateways operate autonomously with cached config

## Architecture Components

### Three Operational Modes

#### 1. Standalone Mode (Default)
- Traditional single-instance deployment
- All configuration stored locally in database
- No external dependencies
- Ideal for development and single-team deployments

#### 2. Control Mode (Hub)
- Central hub managing configuration for edge instances
- Database-backed configuration storage
- gRPC API for edge instance communication
- Namespace-based configuration filtering

#### 3. Edge Mode (Spoke)
- Lightweight gateway instance
- Receives configuration from control instance
- Local configuration caching with fallback mechanisms
- Independent operation after initial synchronization

## Hub-and-Spoke Benefits

### Centralized Management
- **Single Source of Truth**: All configuration managed centrally
- **Consistent Policies**: Uniform security and budget policies across edges
- **Simplified Administration**: Manage hundreds of edges from one location
- **Configuration Versioning**: Track and rollback configuration changes

### Distributed Performance
- **Regional Deployment**: Edge gateways close to users for low latency
- **Local Caching**: Configuration cached at edge for fast access
- **Independent Operation**: Edge continues operating if hub is temporarily unavailable
- **Load Distribution**: Spread load across multiple edge instances

### Multi-Tenancy
- **Namespace Isolation**: Complete separation between tenants
- **Selective Propagation**: Only relevant configuration sent to each edge
- **Tenant-Specific Edges**: Dedicated edge instances per tenant
- **Granular Access Control**: Fine-grained permission management

## Architecture Diagram

```
                    ┌─────────────────┐
                    │   Control Hub   │
                    │   (Database +   │
                    │   gRPC Server)  │
                    └─────────┬───────┘
                              │
                    ┌─────────┴───────┐
                    │                 │
            ┌───────▼──────┐  ┌───────▼──────┐
            │  Edge Node 1 │  │  Edge Node 2 │
            │ (Namespace A) │  │ (Namespace B) │
            └──────────────┘  └──────────────┘
                    │                 │
            ┌───────▼──────┐  ┌───────▼──────┐
            │   LLM APIs   │  │   LLM APIs   │
            │ (OpenAI,etc) │  │ (OpenAI,etc) │
            └──────────────┘  └──────────────┘
```

## Namespace System

### Namespace Concepts
- **Global Namespace** (`""` empty string): Configurations visible to all edges
- **Specific Namespace** (`"tenant-1"`): Only visible to edges with matching namespace
- **Backwards Compatible**: Existing installations default to global namespace

### Namespace Examples
```bash
# Global LLM (available to all edges)
mgw llm create --name="Global GPT-4" --namespace="" --vendor=openai

# Tenant-specific LLM (only for tenant-a edges)
mgw llm create --name="Tenant A LLM" --namespace="tenant-a" --vendor=openai

# Edge with specific namespace
GATEWAY_MODE=edge \
EDGE_NAMESPACE=tenant-a \
CONTROL_ENDPOINT=control:9090 \
./microgateway
```

## Configuration Synchronization

### gRPC Communication
- **Bidirectional Streaming**: Real-time configuration updates
- **Authentication**: Bearer token authentication for security
- **Compression**: gRPC compression for efficient data transfer
- **Reconnection**: Automatic reconnection with exponential backoff

### Change Propagation
- **Real-Time Updates**: Configuration changes propagated immediately
- **Namespace Filtering**: Only relevant changes sent to each edge
- **Reliable Delivery**: Retry mechanisms ensure delivery
- **Fallback Sync**: Full synchronization if incremental sync fails

### Edge Caching
- **Local Storage**: Edge caches all configuration locally
- **Fast Access**: No network calls for cached configuration
- **Fallback Operation**: Continue operating if control unavailable
- **Cache Invalidation**: Smart cache updates on configuration changes

## Deployment Patterns

### Basic Hub-and-Spoke
```bash
# Control instance
GATEWAY_MODE=control \
GRPC_PORT=9090 \
DATABASE_TYPE=postgres \
DATABASE_DSN="postgres://user:pass@localhost/control_db" \
./microgateway

# Edge instance
GATEWAY_MODE=edge \
CONTROL_ENDPOINT=control.example.com:9090 \
EDGE_ID=edge-region-1 \
EDGE_NAMESPACE=production \
./microgateway
```

### Multi-Tenant Deployment
```bash
# Control instance serves multiple tenants
GATEWAY_MODE=control ./microgateway

# Tenant A edge
GATEWAY_MODE=edge \
CONTROL_ENDPOINT=control.internal:9090 \
EDGE_NAMESPACE=tenant-a \
EDGE_ID=tenant-a-edge-1 \
./microgateway

# Tenant B edge
GATEWAY_MODE=edge \
CONTROL_ENDPOINT=control.internal:9090 \
EDGE_NAMESPACE=tenant-b \
EDGE_ID=tenant-b-edge-1 \
./microgateway
```

### Geographic Distribution
```bash
# Control in primary datacenter
GATEWAY_MODE=control \
REGION=us-east-1 \
./microgateway

# Edge in US West
GATEWAY_MODE=edge \
CONTROL_ENDPOINT=control-us-east.company.com:9090 \
EDGE_ID=edge-us-west-1 \
REGION=us-west-1 \
./microgateway

# Edge in Europe
GATEWAY_MODE=edge \
CONTROL_ENDPOINT=control-us-east.company.com:9090 \
EDGE_ID=edge-eu-west-1 \
REGION=eu-west-1 \
./microgateway
```

## Configuration Management

### Control Instance Configuration
```bash
# Control mode environment variables
GATEWAY_MODE=control

# gRPC server settings
GRPC_PORT=9090
GRPC_HOST=0.0.0.0
GRPC_TLS_ENABLED=false
GRPC_AUTH_TOKEN=secure-control-token

# Database configuration (required for control mode)
DATABASE_TYPE=postgres
DATABASE_DSN="postgres://user:pass@postgres:5432/microgateway"
```

### Edge Instance Configuration
```bash
# Edge mode environment variables
GATEWAY_MODE=edge

# Control connection settings
CONTROL_ENDPOINT=control.example.com:9090
EDGE_ID=edge-1
EDGE_NAMESPACE=production

# Authentication
EDGE_AUTH_TOKEN=secure-control-token

# Reconnection settings
EDGE_RECONNECT_INTERVAL=5s
EDGE_HEARTBEAT_INTERVAL=30s
EDGE_SYNC_TIMEOUT=10s
```

## Monitoring and Operations

### Health Monitoring
```bash
# Control instance health
mgw system health  # Standard health check

# Edge instance health  
mgw system health  # Includes connection status to control

# Monitor edge connections (control instance)
curl http://control:8080/api/v1/edges
```

### Edge Management
```bash
# List connected edges (control instance)
mgw edge list

# Get edge details
mgw edge get edge-1

# Force configuration sync
mgw edge sync edge-1

# Monitor edge status
mgw edge status --all
```

### Configuration Propagation
```bash
# Monitor configuration changes
tail -f /var/log/microgateway/control.log | grep "config propagation"

# Check edge synchronization status
mgw edge sync-status edge-1

# Force full resync
mgw edge resync edge-1
```

## Use Cases

### Multi-Region Deployment
- **Global Control**: Single control plane in primary region
- **Regional Edges**: Edge gateways in each geographic region
- **Latency Optimization**: Users connect to nearest edge
- **Data Residency**: Comply with regional data requirements

### Multi-Tenant SaaS
- **Tenant Isolation**: Each tenant has dedicated namespace
- **Selective Configuration**: Tenants only see their own resources
- **Dedicated Edges**: Optional tenant-specific edge instances
- **Billing Separation**: Per-tenant cost tracking and budgets

### Development Environments
- **Centralized Development Control**: Shared control for development teams
- **Developer Edges**: Individual edge instances per developer
- **Environment Isolation**: Separate namespaces for dev/staging/prod
- **Resource Sharing**: Shared LLM configurations across environments

### Enterprise Deployment
- **Department Isolation**: Separate namespaces per department
- **Compliance Boundaries**: Edges for different compliance zones
- **Cost Centers**: Budget allocation per department
- **Security Zones**: Different security policies per zone

## Migration Strategies

### From Standalone to Hub-and-Spoke
```bash
# 1. Backup existing installation
pg_dump microgateway_db > backup.sql

# 2. Run database migration to add namespace support
./microgateway -migrate

# 3. Convert to control mode
GATEWAY_MODE=control ./microgateway

# 4. Deploy edge instances
GATEWAY_MODE=edge \
CONTROL_ENDPOINT=control:9090 \
./microgateway

# 5. Verify deployment
curl "http://control:8080/api/v1/edges"
```

### Gradual Migration
```bash
# Phase 1: Deploy control alongside existing standalone
# Phase 2: Migrate configuration to control instance
# Phase 3: Deploy edge instances
# Phase 4: Redirect traffic to edge instances
# Phase 5: Decommission standalone instance
```

## Best Practices

### Control Instance
- **High Availability**: Deploy control with database clustering
- **Backup Strategy**: Regular database backups and disaster recovery
- **Monitoring**: Comprehensive monitoring of control instance health
- **Security**: Secure gRPC communication with TLS and authentication

### Edge Instances
- **Lightweight Deployment**: Minimal resource requirements for edges
- **Local Caching**: Ensure adequate local storage for configuration cache
- **Network Resilience**: Handle network partitions gracefully
- **Health Monitoring**: Monitor edge health and connectivity

### Configuration Management
- **Namespace Strategy**: Plan namespace structure for your organization
- **Change Management**: Implement approval workflows for configuration changes
- **Testing**: Test configuration changes in development before production
- **Rollback Plans**: Maintain rollback procedures for configuration changes

---

Hub-and-spoke architecture enables scalable, distributed AI/LLM management. For detailed configuration, see [Controller to Edge](controller-edge.md). For namespace management, see [Namespaces](namespaces.md).
