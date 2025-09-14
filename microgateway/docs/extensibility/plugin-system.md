# How Plugins Work

The microgateway features an extensible plugin system based on HashiCorp's go-plugin framework, allowing custom functionality through secure, isolated plugin processes.

## Overview

Plugin system features:
- **Process Isolation**: Plugins run as separate processes for security and stability
- **gRPC Communication**: High-performance inter-process communication
- **Multiple Hook Types**: Various integration points throughout request lifecycle
- **Hot Loading**: Dynamic plugin loading without service restart
- **OCI Distribution**: Industry-standard plugin distribution via container registries
- **Secure Execution**: Resource limits and sandboxing capabilities

## Plugin Architecture

### Plugin Process Model
```
┌─────────────────┐    gRPC    ┌─────────────────┐
│   Microgateway  │ ◄────────► │   Plugin Process │
│   (Host)        │            │   (Binary)       │
└─────────────────┘            └─────────────────┘
```

The microgateway (host) communicates with plugins (separate processes) via gRPC:
- **Host Process**: Main microgateway service
- **Plugin Process**: Independent binary implementing plugin interface
- **gRPC Protocol**: Secure, versioned communication protocol
- **Process Lifecycle**: Host manages plugin process startup and shutdown

### Plugin Types

#### Request/Response Plugins
Process HTTP requests and responses:
- **pre_auth**: Execute before authentication
- **auth**: Custom authentication logic
- **post_auth**: Execute after authentication
- **on_response**: Process responses before returning to client

#### Data Collection Plugins
Handle data storage and analytics:
- **data_collection**: Custom storage backends for analytics, budgets, proxy logs

## Plugin Lifecycle

### Plugin Loading
1. **Discovery**: Host discovers plugins via configuration
2. **Download**: Download plugin binary (if using OCI distribution)
3. **Verification**: Verify plugin signature and integrity
4. **Launch**: Start plugin process with handshake protocol
5. **Registration**: Register plugin hooks with the host
6. **Ready**: Plugin ready to process requests

### Plugin Execution
1. **Hook Trigger**: Request triggers registered hook
2. **Data Marshaling**: Convert request data to plugin format
3. **gRPC Call**: Send data to plugin process
4. **Plugin Processing**: Plugin executes custom logic
5. **Response Handling**: Process plugin response
6. **Continue/Abort**: Continue request or abort based on plugin response

### Plugin Shutdown
1. **Graceful Shutdown**: Host signals plugin to shut down
2. **Cleanup**: Plugin completes in-flight operations
3. **Process Termination**: Plugin process exits cleanly

## Plugin Interface

### Base Plugin Interface
All plugins must implement the base interface:

```go
type BasePlugin interface {
    // Initialize plugin with configuration
    Initialize(config map[string]interface{}) error
    
    // Get the hook type this plugin implements
    GetHookType() HookType
    
    // Health check for plugin
    Health() error
    
    // Plugin metadata
    GetInfo() PluginInfo
}
```

### Hook Types
Available hook types for plugin integration:

```go
type HookType string

const (
    HookTypePreAuth        HookType = "pre_auth"
    HookTypeAuth           HookType = "auth"
    HookTypePostAuth       HookType = "post_auth"
    HookTypeOnResponse     HookType = "on_response"
    HookTypeDataCollection HookType = "data_collection"
)
```

### Plugin Context
Plugins receive context information:

```go
type PluginContext struct {
    RequestID    string
    AppID        uint
    LLMID        uint
    CredentialID uint
    UserAgent    string
    ClientIP     string
    Headers      map[string]string
    Metadata     map[string]interface{}
}
```

## Configuration

### Plugin Configuration File
```yaml
# config/plugins.yaml
version: "1.0"
plugins:
  - name: "auth-plugin"
    path: "./plugins/auth_plugin"
    enabled: true
    priority: 100
    hook_types: 
      - "pre_auth"
      - "auth"
    config:
      auth_endpoint: "${AUTH_SERVICE_URL}"
      timeout: "30s"
      
  - name: "analytics-collector"
    path: "./plugins/elasticsearch_collector"
    enabled: true
    priority: 200
    hook_types:
      - "data_collection"
    config:
      elasticsearch_url: "${ELASTICSEARCH_URL}"
      index_prefix: "microgateway"
```

### Environment Configuration
```bash
# Plugin system settings
PLUGINS_ENABLED=true
PLUGINS_CONFIG_PATH=./config/plugins.yaml
PLUGINS_DIR=./plugins
PLUGINS_TIMEOUT=30s

# Plugin distribution (OCI)
PLUGINS_REGISTRY_URL=registry.company.com
PLUGINS_REGISTRY_USER=plugin-reader
PLUGINS_REGISTRY_PASS=secret

# Plugin security
PLUGINS_VERIFY_SIGNATURES=true
PLUGINS_TRUSTED_KEYS_PATH=./keys/trusted
```

## Plugin Development

### Basic Plugin Structure
```go
package main

import (
    "context"
    "github.com/TykTechnologies/midsommar/microgateway/plugins/sdk"
)

type MyPlugin struct {
    config *MyConfig
}

func (p *MyPlugin) Initialize(config map[string]interface{}) error {
    // Parse configuration
    return nil
}

func (p *MyPlugin) GetHookType() sdk.HookType {
    return sdk.HookTypePreAuth
}

func (p *MyPlugin) Health() error {
    // Health check logic
    return nil
}

func (p *MyPlugin) GetInfo() sdk.PluginInfo {
    return sdk.PluginInfo{
        Name:    "my-plugin",
        Version: "1.0.0",
        Author:  "Developer Name",
    }
}

func (p *MyPlugin) ProcessRequest(ctx context.Context, req *sdk.RequestData, pluginCtx *sdk.PluginContext) (*sdk.PluginResponse, error) {
    // Custom logic here
    return &sdk.PluginResponse{
        Continue: true,
        Modified: false,
    }, nil
}

func main() {
    plugin := &MyPlugin{}
    sdk.ServePlugin(plugin)
}
```

### Building Plugins
```bash
# Build plugin binary
go build -o my_plugin main.go

# Test plugin locally
./my_plugin  # Should wait for gRPC connection

# Build for distribution
GOOS=linux GOARCH=amd64 go build -o my_plugin-linux-amd64 main.go
```

## Plugin Execution

### Request Flow with Plugins
```
HTTP Request → Pre-Auth Plugins → Authentication → Post-Auth Plugins → LLM Request → Response Plugins → HTTP Response
```

### Plugin Response Handling
Plugins can:
- **Continue**: Allow request to proceed normally
- **Modify**: Modify request/response data
- **Abort**: Stop request processing and return error
- **Redirect**: Redirect request to different endpoint

### Error Handling
- Plugin failures don't crash the main service
- Failed plugins are logged and disabled
- Request processing continues if plugin fails (configurable)
- Plugin health monitoring with automatic restart

## Plugin Security

### Process Isolation
- Plugins run in separate processes
- No shared memory between host and plugins
- Resource limits enforced by OS
- Plugin crashes don't affect main service

### Communication Security
- gRPC with optional TLS encryption
- Authentication tokens for plugin communication
- Request data validation and sanitization
- Timeout controls for plugin execution

### Resource Limits
```bash
# Plugin resource configuration
PLUGINS_MAX_MEMORY=256MB      # Memory limit per plugin
PLUGINS_MAX_CPU=50            # CPU percentage limit
PLUGINS_TIMEOUT=30s           # Execution timeout
PLUGINS_MAX_PROCESSES=10      # Maximum concurrent plugins
```

## Plugin Management

### Plugin Status
```bash
# List loaded plugins (future CLI command)
mgw plugin list

# Check plugin health
mgw plugin health my-plugin

# Reload plugin configuration
mgw plugin reload

# Stop plugin
mgw plugin stop my-plugin
```

### Plugin Monitoring
```bash
# Monitor plugin performance
mgw system metrics | grep plugin

# Plugin execution statistics
# - plugin_executions_total
# - plugin_execution_duration_seconds
# - plugin_errors_total
# - plugin_health_status
```

## Plugin Examples

### Authentication Plugin
```bash
# Custom authentication against external service
# Validates tokens against company SSO system
# Returns user information for request context
```

### Rate Limiting Plugin
```bash
# Advanced rate limiting with custom logic
# Per-user rate limits
# Burst capacity handling
# Geographic rate limiting
```

### Analytics Plugin
```bash
# Send analytics to external systems
# Real-time streaming to data lakes
# Custom metrics calculation
# Business intelligence integration
```

### Audit Plugin
```bash
# Compliance logging
# Detailed audit trails
# Regulatory reporting
# Data retention management
```

## Plugin SDK

### SDK Components
The plugin SDK provides:
- **Base interfaces**: Common plugin functionality
- **Helper functions**: Request/response processing utilities
- **Configuration helpers**: Environment variable parsing
- **Logging utilities**: Structured logging for plugins
- **Testing framework**: Unit testing support for plugins

### SDK Installation
```bash
# Import SDK in plugin code
import "github.com/TykTechnologies/midsommar/microgateway/plugins/sdk"

# SDK provides all necessary interfaces and utilities
```

## Best Practices

### Plugin Development
- Keep plugins lightweight and focused
- Implement proper error handling and timeouts
- Use configuration for all environment-specific settings
- Include comprehensive logging for debugging
- Write unit tests for plugin logic

### Plugin Deployment
- Use OCI distribution for consistent deployment
- Sign plugins for security verification
- Test plugins in development environments first
- Monitor plugin performance in production
- Implement plugin health checks

### Plugin Security
- Minimize plugin privileges and resource usage
- Validate all input data in plugins
- Use secure communication channels
- Regular security audits of plugin code
- Keep plugins updated with security patches

---

The plugin system provides powerful extensibility for the microgateway. For installation instructions, see [Plugin Installation](plugin-installation.md). For distribution, see [Plugin Distribution](plugin-distribution.md).
