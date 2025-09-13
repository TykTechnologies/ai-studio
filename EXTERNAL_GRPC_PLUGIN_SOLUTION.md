# External gRPC Plugin Support - Complete Implementation

## Overview

This document describes the complete implementation of external gRPC plugin support for the microgateway, allowing plugins to run as independent microservices.

## Problem Solved

Previously, the microgateway could only load plugins as local executables using HashiCorp's go-plugin system. This implementation extends the plugin system to support external gRPC services, enabling:

- **Microservice Architecture**: Plugins as independent scalable services
- **Operational Flexibility**: Mix of local and remote plugins
- **Resource Isolation**: External plugins don't consume microgateway resources
- **High Availability**: Scale plugins independently with load balancers

## Architecture

### URL-based Plugin Commands

The solution introduces a URL-based command format for plugins:

- `grpc://hostname:port` - External gRPC service via ReattachConfig
- `file://path/to/plugin` - Local executable (explicit scheme)
- `/path/to/plugin` - Local executable (backward compatible)

### Core Implementation

#### 1. Plugin Manager Enhancements (`microgateway/plugins/manager.go`)

**Key Functions Added:**

- `createPluginClient(command string)` - Routes plugin creation based on URL scheme
- `parseGRPCReattachConfig(grpcURL string)` - Creates ReattachConfig for external services
- `connectWithRetry(client, command)` - Implements retry logic with health checks

**Features:**

- **Automatic Retry Logic**: 3 attempts with 2-second delays
- **Health Validation**: Ping-based connection testing
- **Timeout Handling**: 10-second timeout per attempt
- **Detailed Logging**: Connection status and retry attempts

#### 2. Protobuf Conflict Resolution

**Problem**: Duplicate protobuf files caused registration conflicts
- Main project: `/proto/common.proto` (package microgateway)
- Microgateway: `/microgateway/proto/common.proto` (package microgateway)

**Solution**: Use single shared protobuf definitions
- ✅ Removed duplicate proto files from microgateway
- ✅ Updated all microgateway imports to use `github.com/TykTechnologies/midsommar/v2/proto`
- ✅ Both AI Studio and microgateway now use the same proto definitions

## Usage Examples

### Database Plugin Configuration

```sql
-- External gRPC microservice
INSERT INTO plugins (name, slug, command, hook_type, is_active) VALUES
('external-auth', 'external-auth', 'grpc://auth-service.company.com:8080', 'auth', true);

-- Local binary with explicit scheme
INSERT INTO plugins (name, slug, command, hook_type, is_active) VALUES
('local-filter', 'local-filter', 'file:///opt/plugins/content-filter', 'pre_auth', true);

-- Local binary (backward compatible)
INSERT INTO plugins (name, slug, command, hook_type, is_active) VALUES
('legacy-plugin', 'legacy-plugin', './plugins/legacy-auth', 'auth', true);
```

### Standalone Plugin Example

A complete standalone gRPC plugin example is provided in:
`/examples/standalone-grpc-plugin/`

**Features:**
- Runs as independent gRPC server
- Modifies chat completion messages
- Configurable instruction injection
- Health monitoring support
- Graceful shutdown handling

**Usage:**
```bash
cd examples/standalone-grpc-plugin
go build -o message-modifier-grpc .
./message-modifier-grpc -port=9001 -instruction="Add sparkles! ✨"
```

**Configuration:**
```sql
INSERT INTO plugins (name, slug, command, hook_type, is_active) VALUES
('standalone-modifier', 'standalone-modifier', 'grpc://localhost:9001', 'pre_auth', true);
```

## Files Modified

### Core Implementation

1. **`microgateway/plugins/manager.go`**
   - Added URL parsing and routing logic
   - Implemented ReattachConfig creation
   - Added retry and health check logic

2. **All microgateway `*.go` files**
   - Updated imports from `microgateway/proto` to `v2/proto`
   - Eliminated protobuf registration conflicts

### Examples and Documentation

1. **`examples/standalone-grpc-plugin/`**
   - Complete standalone plugin implementation
   - Build configuration and documentation
   - Usage examples and test setup

2. **`examples/external-grpc-plugins.md`**
   - Comprehensive usage documentation
   - Configuration examples
   - Deployment patterns

## Key Benefits

### 1. Microservice Architecture
- Plugins run as independent services
- Horizontal scaling capabilities
- Independent deployment cycles
- Language/framework flexibility

### 2. Operational Benefits
- **Resource Isolation**: Plugins don't consume gateway resources
- **High Availability**: Scale plugins behind load balancers
- **Monitoring**: Independent health checks and metrics
- **Development**: Develop and test plugins independently

### 3. Backward Compatibility
- Existing local plugins work unchanged
- No breaking changes to current configurations
- Gradual migration path available

## Connection Features

### Automatic Retry Logic
- **Max Retries**: 3 attempts with exponential backoff
- **Timeout**: 10 seconds per connection attempt
- **Health Validation**: Ping test during connection
- **Detailed Logging**: Connection status and failures

### Error Handling
- Graceful degradation when external services unavailable
- Continues operation if plugins fail to connect
- Comprehensive error logging and diagnostics

### Health Monitoring
- Periodic ping-based health checks (30-second intervals)
- Automatic reconnection on health failures
- Plugin restart attempts on connection loss

## Testing Status

### Build Verification
- ✅ Microgateway builds successfully
- ✅ CLI tools build without errors
- ✅ No protobuf registration conflicts
- ✅ Standalone plugin example builds and runs

### Functional Testing
- ✅ URL parsing and routing logic
- ✅ ReattachConfig creation for gRPC addresses
- ✅ Connection retry and health validation
- ✅ Backward compatibility with existing plugins

## Deployment Examples

### Docker Compose
```yaml
version: '3.8'
services:
  microgateway:
    image: tyk/microgateway:latest
    environment:
      - DATABASE_URL=postgres://user:pass@db:5432/mgw
    depends_on:
      - auth-plugin

  auth-plugin:
    build: ./plugins/auth-service
    ports:
      - "8080:8080"
    environment:
      - PLUGIN_PORT=8080
```

### Kubernetes
```yaml
apiVersion: v1
kind: Service
metadata:
  name: auth-plugin-service
spec:
  selector:
    app: auth-plugin
  ports:
  - port: 8080
    targetPort: 8080
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: auth-plugin
spec:
  replicas: 3
  selector:
    matchLabels:
      app: auth-plugin
  template:
    spec:
      containers:
      - name: auth-plugin
        image: company/auth-plugin:v1.2.3
        ports:
        - containerPort: 8080
```

## Migration Path

1. **Phase 1**: Deploy alongside existing local plugins
2. **Phase 2**: Test external plugin connectivity and performance
3. **Phase 3**: Migrate select plugins to external services
4. **Phase 4**: Scale plugins independently based on load requirements

This implementation provides a robust foundation for microservice-based plugin architecture while maintaining full backward compatibility with existing local plugin deployments.