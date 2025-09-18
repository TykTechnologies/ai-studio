# Hub-and-Spoke Architecture Overview

The microgateway hub-and-spoke architecture enables distributed deployments with centralized configuration management, providing scalability, multi-tenancy, and operational efficiency.

## Architecture Concepts

### Traditional vs Hub-and-Spoke

#### Traditional Standalone Deployment
```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   Microgateway  │    │   Microgateway  │    │   Microgateway  │
│                 │    │                 │    │                 │
│   ┌─────────┐   │    │   ┌─────────┐   │    │   ┌─────────┐   │
│   │Database │   │    │   │Database │   │    │   │Database │   │
│   └─────────┘   │    │   └─────────┘   │    │   └─────────┘   │
└─────────────────┘    └─────────────────┘    └─────────────────┘
```

**Challenges:**
- Configuration drift between instances
- Manual configuration synchronization
- No central management
- Difficult to maintain consistency

#### Hub-and-Spoke Deployment
```
                    ┌─────────────────────┐
                    │   Control Instance  │
                    │                     │
                    │   ┌─────────────┐   │
                    │   │  Database   │   │
                    │   └─────────────┘   │
                    │   ┌─────────────┐   │
                    │   │  gRPC API   │   │
                    │   └─────────────┘   │
                    └──────────┬──────────┘
                               │
           ┌───────────────────┼───────────────────┐
           │                   │                   │
    ┌──────▼──────┐    ┌──────▼──────┐    ┌──────▼──────┐
    │Edge Instance│    │Edge Instance│    │Edge Instance│
    │             │    │             │    │             │
    │┌───────────┐│    │┌───────────┐│    │┌───────────┐│
    ││Config     ││    ││Config     ││    ││Config     ││
    ││Cache      ││    ││Cache      ││    ││Cache      ││
    │└───────────┘│    │└───────────┘│    │└───────────┘│
    └─────────────┘    └─────────────┘    └─────────────┘
```

**Benefits:**
- Centralized configuration management
- Real-time configuration synchronization
- Reduced operational overhead
- Consistent configuration across all instances
- Multi-tenant namespace isolation

## Core Components

### 1. Control Instance (Hub)

The control instance serves as the central configuration authority:

**Responsibilities:**
- Stores all configuration in database
- Exposes gRPC API for edge instances
- Propagates configuration changes in real-time
- Manages edge instance registration and health
- Provides configuration via REST API for management

**Key Features:**
- Database-backed configuration storage
- Real-time change propagation
- Edge instance lifecycle management
- Namespace-based configuration filtering
- Authentication and authorization

### 2. Edge Instance (Spoke)

Edge instances are lightweight gateways that receive configuration from the control:

**Responsibilities:**
- Connects to control instance via gRPC
- Receives and caches configuration locally
- Serves AI Gateway requests using cached config
- Reports health status to control
- Handles automatic reconnection

**Key Features:**
- Local configuration caching
- Automatic failover and recovery
- Namespace-based configuration filtering
- Minimal resource footprint
- Independent operation after sync

### 3. Configuration Synchronization

The synchronization system ensures configuration consistency:

**Synchronization Flow:**
```
Control Instance                 Edge Instance
       │                               │
       │◄──── Registration ────────────┤
       ├───── Initial Config ──────────►│
       │                               │
       │◄──── Heartbeat ───────────────┤
       ├───── Config Changes ──────────►│
       │                               │
       │◄──── Status Updates ──────────┤
```

**Features:**
- Bidirectional gRPC streaming
- Real-time change propagation
- Reliable delivery with retry logic
- Conflict resolution and versioning
- Bandwidth-efficient updates

## Namespace System

Namespaces enable multi-tenant configuration isolation:

### Namespace Rules

1. **Global Namespace** (`""` empty string)
   - Visible to all edge instances
   - Used for shared configurations
   - Default for existing installations

2. **Specific Namespace** (`"tenant-1"`, `"production"`, etc.)
   - Only visible to edges with matching namespace
   - Enables tenant/environment isolation
   - Supports hierarchical organization

### Configuration Visibility

```
Control Database:
├── LLM "OpenAI GPT-4" (namespace: "")           # Global
├── LLM "Tenant A GPT-4" (namespace: "tenant-a") # Tenant specific
├── App "Global App" (namespace: "")             # Global
└── App "Tenant A App" (namespace: "tenant-a")   # Tenant specific

Edge Instance (namespace: "tenant-a"):
├── ✓ LLM "OpenAI GPT-4" (global)
├── ✓ LLM "Tenant A GPT-4" (matching namespace)
├── ✓ App "Global App" (global)
├── ✓ App "Tenant A App" (matching namespace)
└── ✗ Any config with namespace: "tenant-b"

Edge Instance (namespace: ""):
├── ✓ LLM "OpenAI GPT-4" (global)
├── ✗ LLM "Tenant A GPT-4" (filtered out)
├── ✓ App "Global App" (global)
└── ✗ App "Tenant A App" (filtered out)
```

## Communication Protocol

### gRPC Service Definition

The configuration synchronization uses a bidirectional gRPC stream:

```protobuf
service ConfigurationSyncService {
  rpc RegisterEdge(EdgeRegistrationRequest) returns (EdgeRegistrationResponse);
  rpc GetFullConfiguration(ConfigurationRequest) returns (ConfigurationSnapshot);
  rpc SubscribeToChanges(stream EdgeMessage) returns (stream ControlMessage);
  rpc SendHeartbeat(HeartbeatRequest) returns (HeartbeatResponse);
  rpc UnregisterEdge(EdgeUnregistrationRequest) returns (Empty);
}
```

### Message Flow

1. **Edge Startup**
   ```
   Edge ──RegisterEdge──► Control
   Edge ◄─Initial Config─ Control
   Edge ──Subscribe──────► Control (start stream)
   ```

2. **Runtime Operations**
   ```
   Edge ──Heartbeat──────► Control (periodic)
   Edge ◄─Config Change── Control (real-time)
   Edge ──Status Update──► Control (on change)
   ```

3. **Configuration Changes**
   ```
   Admin ──Update Config─► Control (via REST API)
   Control ──Propagate───► Edge1, Edge2, EdgeN (filtered by namespace)
   ```

## Deployment Patterns

### 1. Single Control, Multiple Edges

**Use Case:** Central management with distributed execution
```
┌─────────────┐
│   Control   │
│  (Primary)  │
└─────┬───────┘
      │
┌─────┼─────┐
│     │     │
▼     ▼     ▼
Edge  Edge  Edge
(A)   (B)   (C)
```

### 2. Multi-Tenant with Namespace Isolation

**Use Case:** SaaS providers serving multiple tenants
```
                ┌─────────────┐
                │   Control   │
                │             │
                └─────┬───────┘
                      │
        ┌─────────────┼─────────────┐
        │             │             │
        ▼             ▼             ▼
┌─────────────┐ ┌─────────────┐ ┌─────────────┐
│    Edge     │ │    Edge     │ │    Edge     │
│ (tenant-a)  │ │ (tenant-b)  │ │ (tenant-c)  │
└─────────────┘ └─────────────┘ └─────────────┘
```

### 3. Geographic Distribution

**Use Case:** Global deployment with regional edges
```
                ┌─────────────┐
                │   Control   │
                │   (US-East) │
                └─────┬───────┘
                      │
        ┌─────────────┼─────────────┐
        │             │             │
        ▼             ▼             ▼
┌─────────────┐ ┌─────────────┐ ┌─────────────┐
│    Edge     │ │    Edge     │ │    Edge     │
│  (US-West)  │ │   (Europe)  │ │   (APAC)    │
└─────────────┘ └─────────────┘ └─────────────┘
```

## Security Considerations

### Authentication
- Bearer token authentication for all gRPC communication
- Configurable authentication tokens per deployment
- Support for token rotation without downtime

### Network Security
- Optional TLS encryption for control-edge communication
- Certificate-based mutual authentication
- Network isolation between tenants

### Data Privacy
- Namespace-based data isolation
- No cross-tenant data leakage
- Audit logging for configuration access

## Performance Characteristics

### Resource Usage

**Control Instance:**
- Memory: Base + (Number of edges × ~1MB)
- CPU: Low baseline + spikes during config changes
- Network: Minimal + (Config changes × Number of edges)
- Database: Standard CRUD operations

**Edge Instance:**
- Memory: ~50MB + Configuration cache size
- CPU: Minimal overhead for sync operations
- Network: Periodic heartbeats + config updates
- No database required

### Scalability Limits

- **Maximum Edges per Control:** 1000+ (tested configuration)
- **Configuration Size:** Up to 100MB cached per edge
- **Sync Latency:** < 100ms for configuration changes
- **Heartbeat Frequency:** 30 seconds (configurable)

## Operational Benefits

### Simplified Management
- Single point of configuration control
- Consistent configuration across all instances
- Automated configuration deployment
- Centralized monitoring and alerting

### Enhanced Reliability
- Local configuration caching on edges
- Automatic failover and recovery
- Health monitoring and alerting
- Graceful degradation on connectivity loss

### Cost Optimization
- Reduced operational overhead
- Efficient resource utilization
- Automated scaling and provisioning
- Simplified backup and disaster recovery

## Next Steps

For detailed configuration instructions, see:
- [Configuration Guide](./hub-spoke-configuration.md)
- [Deployment Examples](./hub-spoke-deployment.md)
- [Operations Guide](./hub-spoke-operations.md)