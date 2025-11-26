# Plugin SDK Reference

Tyk AI Studio provides a **Unified Plugin SDK** that works seamlessly in both AI Studio and Microgateway contexts with a single API. This guide covers the core SDK concepts, capabilities, and patterns.

## Unified SDK Overview

The Unified SDK (`pkg/plugin_sdk`) is the modern, recommended approach for all plugin development. It provides:

- **Single Import**: One SDK works in both AI Studio and Microgateway
- **Automatic Runtime Detection**: SDK detects the execution environment
- **Capability-Based Design**: Implement only what you need
- **Type-Safe**: Clean Go types, no manual proto handling
- **Service Access**: Built-in KV storage, logging, and management APIs
- **Context-Rich**: Access to app, user, LLM metadata in every call

### Installation

```bash
go get github.com/TykTechnologies/midsommar/v2/pkg/plugin_sdk
```

### Basic Plugin Structure

```go
import "github.com/TykTechnologies/midsommar/v2/pkg/plugin_sdk"

type MyPlugin struct {
    plugin_sdk.BasePlugin
    // Plugin-specific fields
}

func NewMyPlugin() *MyPlugin {
    return &MyPlugin{
        BasePlugin: plugin_sdk.NewBasePlugin(
            "my-plugin",
            "1.0.0",
            "My plugin description",
        ),
    }
}

func (p *MyPlugin) Initialize(ctx plugin_sdk.Context, config map[string]string) error {
    // Initialize plugin
    return nil
}

func main() {
    plugin_sdk.Serve(NewMyPlugin())
}
```

## Plugin Capabilities

Plugins implement one or more capability interfaces. The SDK supports 12 distinct capabilities:

| Capability | Interface | Where It Works | Purpose |
|------------|-----------|----------------|---------|
| **Pre-Auth** | `PreAuthHandler` | Studio + Gateway | Process requests before authentication |
| **Auth** | `AuthHandler` | Studio + Gateway | Custom authentication with credential lookup |
| **Post-Auth** | `PostAuthHandler` | Studio + Gateway | Process requests after authentication (most common) |
| **Response** | `ResponseHandler` | Studio + Gateway | Modify response headers and body |
| **Data Collection** | `DataCollector` | Studio + Gateway | Collect telemetry (analytics, budgets, proxy logs) |
| **UI Provider** | `UIProvider` | Studio only | Serve web UI assets |
| **Config Provider** | `ConfigProvider` | Studio + Gateway | Provide JSON Schema configuration |
| **Manifest Provider** | `ManifestProvider` | Gateway only | Provide plugin manifest (gateway-only plugins) |
| **Agent** | `AgentPlugin` | Studio only | Conversational AI agent with streaming |
| **Object Hooks** | `ObjectHookHandler` | Studio only | Intercept CRUD operations on objects |
| **Scheduler** | `SchedulerPlugin` | Studio only | Execute tasks on cron-based schedules |
| **Edge Payload** | `EdgePayloadReceiver` | Studio only | Receive data from edge (gateway) plugins |

### Multi-Capability Plugins

A single plugin can implement multiple capabilities. For example, a rate limiter might implement:
- `PostAuthHandler` - Check limits before request
- `ResponseHandler` - Update counters after response
- `UIProvider` - Provide management UI

```go
type RateLimiter struct {
    plugin_sdk.BasePlugin
}

// Implement PostAuthHandler
func (p *RateLimiter) HandlePostAuth(ctx plugin_sdk.Context, req *pb.EnrichedRequest) (*pb.PluginResponse, error) {
    // Check rate limits
}

// Implement ResponseHandler
func (p *RateLimiter) OnBeforeWriteHeaders(ctx plugin_sdk.Context, req *pb.ResponseWriteRequest) (*pb.ResponseWriteResponse, error) {
    // Update counters
}

// Implement UIProvider
func (p *RateLimiter) GetAsset(path string) ([]byte, string, error) {
    // Serve UI assets
}
```

## Core Interfaces

### 1. PreAuthHandler

Process requests **before** authentication. Useful for IP filtering, request validation, etc.

```go
type PreAuthHandler interface {
    HandlePreAuth(ctx Context, req *pb.EnrichedRequest) (*pb.PluginResponse, error)
}
```

**Example:**
```go
func (p *MyPlugin) HandlePreAuth(ctx plugin_sdk.Context, req *pb.EnrichedRequest) (*pb.PluginResponse, error) {
    // Block requests from specific IPs
    if isBlockedIP(req.ClientIp) {
        return &pb.PluginResponse{
            Block:        true,
            ErrorMessage: "IP blocked",
        }, nil
    }
    return &pb.PluginResponse{Modified: false}, nil
}
```

### 2. AuthHandler

Custom authentication with credential lookup.

```go
type AuthHandler interface {
    HandleAuth(ctx Context, req *pb.EnrichedRequest) (*pb.PluginResponse, error)
}
```

**Example:**
```go
func (p *MyPlugin) HandleAuth(ctx plugin_sdk.Context, req *pb.EnrichedRequest) (*pb.PluginResponse, error) {
    token := req.Headers["Authorization"]

    // Validate token with external service
    user, err := validateToken(token)
    if err != nil {
        return &pb.PluginResponse{
            Block:        true,
            ErrorMessage: "Invalid token",
        }, nil
    }

    return &pb.PluginResponse{
        Modified:   true,
        Credential: &pb.Credential{
            UserID:   user.ID,
            Username: user.Name,
        },
    }, nil
}
```

### 3. PostAuthHandler

Process requests **after** authentication. Most common capability for request enrichment, policy enforcement, etc.

```go
type PostAuthHandler interface {
    HandlePostAuth(ctx Context, req *pb.EnrichedRequest) (*pb.PluginResponse, error)
}
```

**Example:**
```go
func (p *MyPlugin) HandlePostAuth(ctx plugin_sdk.Context, req *pb.EnrichedRequest) (*pb.PluginResponse, error) {
    ctx.Services.Logger().Info("Processing request",
        "app_id", ctx.AppID,
        "user_id", ctx.UserID,
    )

    // Add custom header
    req.Headers["X-Custom-Header"] = "value"

    return &pb.PluginResponse{
        Modified: true,
        Request:  req,
    }, nil
}
```

### 4. ResponseHandler

Modify response headers and body. Two methods allow phased processing:

```go
type ResponseHandler interface {
    OnBeforeWriteHeaders(ctx Context, req *pb.ResponseWriteRequest) (*pb.ResponseWriteResponse, error)
    OnBeforeWrite(ctx Context, req *pb.ResponseWriteRequest) (*pb.ResponseWriteResponse, error)
}
```

**Example:**
```go
func (p *MyPlugin) OnBeforeWriteHeaders(ctx plugin_sdk.Context, req *pb.ResponseWriteRequest) (*pb.ResponseWriteResponse, error) {
    // Add tracking header
    if req.Headers == nil {
        req.Headers = make(map[string]string)
    }
    req.Headers["X-Request-Id"] = generateRequestID()

    return &pb.ResponseWriteResponse{
        Modified: true,
        Headers:  req.Headers,
    }, nil
}

func (p *MyPlugin) OnBeforeWrite(ctx plugin_sdk.Context, req *pb.ResponseWriteRequest) (*pb.ResponseWriteResponse, error) {
    // Modify response body
    modifiedBody := transformResponse(req.Body)

    return &pb.ResponseWriteResponse{
        Modified: true,
        Body:     modifiedBody,
    }, nil
}
```

### 5. DataCollector

Collect telemetry data (analytics, budgets, proxy logs).

```go
type DataCollector interface {
    HandleProxyLog(ctx Context, log *pb.ProxyLogData) error
    HandleAnalytics(ctx Context, analytics *pb.AnalyticsData) error
    HandleBudgetUsage(ctx Context, usage *pb.BudgetUsageData) error
}
```

**Example:**
```go
func (p *MyPlugin) HandleAnalytics(ctx plugin_sdk.Context, analytics *pb.AnalyticsData) error {
    // Export analytics to external system
    return exportToElasticsearch(analytics)
}

func (p *MyPlugin) HandleBudgetUsage(ctx plugin_sdk.Context, usage *pb.BudgetUsageData) error {
    // Track budget usage
    return trackBudget(usage)
}

func (p *MyPlugin) HandleProxyLog(ctx plugin_sdk.Context, log *pb.ProxyLogData) error {
    // Log proxy requests
    return logToFile(log)
}
```

### 6. AgentPlugin

Conversational AI agent with streaming support. See [Agent Plugins Guide](plugins-studio-agent.md) for details.

```go
type AgentPlugin interface {
    HandleAgentMessage(req *pb.AgentMessageRequest, stream pb.PluginService_HandleAgentMessageServer) error
}
```

### 7. ObjectHookHandler

Intercept CRUD operations on AI Studio objects (LLMs, Datasources, Tools, Users). See [Object Hooks Guide](plugins-object-hooks.md) for complete details.

```go
type ObjectHookHandler interface {
    GetObjectHookRegistrations() ([]*pb.ObjectHookRegistration, error)
    HandleObjectHook(ctx Context, req *pb.ObjectHookRequest) (*pb.ObjectHookResponse, error)
}
```

### 8. UIProvider

Serve web UI assets for AI Studio plugins. See [UI Plugins Guide](plugins-studio-ui.md) for details.

```go
type UIProvider interface {
    GetAsset(path string) ([]byte, string, error)
    ListAssets() ([]string, error)
    GetManifest() ([]byte, error)
    HandleRPC(method string, payload []byte) ([]byte, error)
}
```

### 9. ConfigProvider

Provide JSON Schema for plugin configuration.

```go
type ConfigProvider interface {
    GetConfigSchema() ([]byte, error)
}
```

**Example:**
```go
func (p *MyPlugin) GetConfigSchema() ([]byte, error) {
    schema := map[string]interface{}{
        "type": "object",
        "properties": map[string]interface{}{
            "api_key": map[string]interface{}{
                "type":        "string",
                "description": "API key for external service",
            },
            "rate_limit": map[string]interface{}{
                "type":        "integer",
                "description": "Requests per minute",
                "default":     100,
            },
        },
        "required": []string{"api_key"},
    }
    return json.Marshal(schema)
}
```

### 10. ManifestProvider

Provide plugin manifest for gateway-only plugins (no UI).

```go
type ManifestProvider interface {
    GetManifest() ([]byte, error)
}
```

### 11. SchedulerPlugin

Execute tasks on cron-based schedules.

```go
type SchedulerPlugin interface {
    ExecuteScheduledTask(ctx Context, schedule *Schedule) error
}

type Schedule struct {
    ID             string                 // Unique identifier from manifest
    Name           string                 // Human-readable name
    Cron           string                 // Cron expression (e.g., "0 * * * *")
    Timezone       string                 // Timezone for cron evaluation
    Enabled        bool                   // Whether schedule is currently enabled
    TimeoutSeconds int                    // Maximum execution time
    Config         map[string]interface{} // Schedule-specific configuration
}
```

**Example:**
```go
func (p *MyPlugin) ExecuteScheduledTask(ctx plugin_sdk.Context, schedule *plugin_sdk.Schedule) error {
    ctx.Services.Logger().Info("Running scheduled task",
        "schedule_id", schedule.ID,
        "schedule_name", schedule.Name,
    )

    // Perform scheduled work
    return p.runCleanup(ctx)
}
```

### 12. EdgePayloadReceiver

Receive data from edge (Microgateway) plugins. This enables the hub-and-spoke communication pattern where edge plugins can send data back to the control plane. See [Edge-to-Control Communication](plugins-edge-to-control.md) for complete details.

```go
type EdgePayloadReceiver interface {
    AcceptEdgePayload(ctx Context, payload *EdgePayload) (handled bool, err error)
}

type EdgePayload struct {
    Payload           []byte            // Raw payload data from edge plugin
    EdgeID            string            // Edge instance identifier
    EdgeNamespace     string            // Namespace of the edge instance
    CorrelationID     string            // Correlation ID for tracking
    Metadata          map[string]string // Key-value metadata
    EdgeTimestamp     int64             // Unix timestamp when generated at edge
    ReceivedTimestamp int64             // Unix timestamp when received at control
}
```

**Example:**
```go
func (p *MyPlugin) AcceptEdgePayload(ctx plugin_sdk.Context, payload *plugin_sdk.EdgePayload) (bool, error) {
    // Check if this payload is for us
    if payload.Metadata["type"] != "my-plugin-data" {
        return false, nil // Not our payload
    }

    ctx.Services.Logger().Info("Received edge payload",
        "edge_id", payload.EdgeID,
        "correlation_id", payload.CorrelationID,
    )

    // Process the payload
    if err := p.processEdgeData(payload.Payload); err != nil {
        return true, err
    }

    return true, nil
}
```

## Context and Services

Every handler receives a `Context` that provides access to runtime information and services.

### Context Fields

```go
type Context struct {
    Runtime    Runtime                    // RuntimeStudio or RuntimeGateway
    AppID      uint32                     // Current application ID
    UserID     uint32                     // Current user ID (if authenticated)
    SessionID  string                     // Chat session ID (if applicable)
    LLM        *pb.LLM                    // LLM configuration (if applicable)
    Services   Services                   // Service broker
}
```

### Runtime Detection

Plugins can adapt behavior based on runtime:

```go
func (p *MyPlugin) HandlePostAuth(ctx plugin_sdk.Context, req *pb.EnrichedRequest) (*pb.PluginResponse, error) {
    if ctx.Runtime == plugin_sdk.RuntimeStudio {
        // Studio-specific logic
        ctx.Services.Logger().Info("Running in AI Studio")
    } else {
        // Gateway-specific logic
        ctx.Services.Logger().Info("Running in Microgateway")
    }

    return &pb.PluginResponse{Modified: false}, nil
}
```

### Service Broker

The context provides access to services through `ctx.Services`:

#### Universal Services (Both Runtimes)

**KV Storage:**
```go
// Write data
err := ctx.Services.KV().Write(ctx, "key", []byte("value"))

// Read data
data, err := ctx.Services.KV().Read(ctx, "key")

// Delete data
err := ctx.Services.KV().Delete(ctx, "key")

// List keys
keys, err := ctx.Services.KV().List(ctx, "prefix")
```

**Note on KV Storage:**
- **Studio**: PostgreSQL-backed, shared across hosts, durable
- **Gateway**: Local database, per-instance, ephemeral

**Logging:**
```go
ctx.Services.Logger().Info("Message", "key", "value")
ctx.Services.Logger().Warn("Warning", "error", err)
ctx.Services.Logger().Error("Error", "details", details)
ctx.Services.Logger().Debug("Debug info", "data", data)
```

#### Runtime-Specific Services

**Gateway Services** (`ctx.Services.Gateway()`):
```go
if ctx.Runtime == plugin_sdk.RuntimeGateway {
    // Get app
    app, err := ctx.Services.Gateway().GetApp(ctx, appID)

    // List apps
    apps, err := ctx.Services.Gateway().ListApps(ctx)

    // Get LLM
    llm, err := ctx.Services.Gateway().GetLLM(ctx, llmID)

    // Get budget status
    status, err := ctx.Services.Gateway().GetBudgetStatus(ctx, appID)

    // Validate credential
    valid, err := ctx.Services.Gateway().ValidateCredential(ctx, token)
}
```

**Studio Services** (`ctx.Services.Studio()`):
```go
if ctx.Runtime == plugin_sdk.RuntimeStudio {
    // Get app
    app, err := ctx.Services.Studio().GetApp(ctx, appID)

    // Update app with metadata
    err := ctx.Services.Studio().UpdateAppWithMetadata(ctx, appID, metadata)

    // List LLMs
    llms, err := ctx.Services.Studio().ListLLMs(ctx, page, limit)

    // List tools
    tools, err := ctx.Services.Studio().ListTools(ctx, page, limit)

    // Call LLM
    stream, err := ctx.Services.Studio().CallLLM(ctx, llmID, model, messages, temp, maxTokens, tools, stream)
}
```

## Initialization Pattern

Plugins should extract the service broker ID during initialization for Service API access:

```go
func (p *MyPlugin) Initialize(ctx plugin_sdk.Context, config map[string]string) error {
    // Extract broker ID for Service API access
    brokerIDStr := ""
    if id, ok := config["_service_broker_id"]; ok {
        brokerIDStr = id
    } else if id, ok := config["service_broker_id"]; ok {
        brokerIDStr = id
    }

    if brokerIDStr != "" {
        var brokerID uint32
        fmt.Sscanf(brokerIDStr, "%d", &brokerID)
        ai_studio_sdk.SetServiceBrokerID(brokerID)
    }

    // Parse plugin-specific config
    p.apiKey = config["api_key"]

    return nil
}
```

## BasePlugin Convenience Struct

The SDK provides `BasePlugin` to reduce boilerplate:

```go
type MyPlugin struct {
    plugin_sdk.BasePlugin
    apiKey string
}

func NewMyPlugin() *MyPlugin {
    return &MyPlugin{
        BasePlugin: plugin_sdk.NewBasePlugin(
            "my-plugin",
            "1.0.0",
            "My plugin description",
        ),
    }
}
```

`BasePlugin` provides default implementations for common methods, which you can override as needed.

## Error Handling

### Blocking Requests

Return a response with `Block: true`:

```go
return &pb.PluginResponse{
    Block:        true,
    ErrorMessage: "Request blocked: invalid input",
}, nil
```

### Non-Blocking Errors

Log the error and continue:

```go
if err != nil {
    ctx.Services.Logger().Error("Failed to process", "error", err)
    return &pb.PluginResponse{Modified: false}, nil
}
```

### Agent Errors

Send ERROR chunks for streaming agents:

```go
return stream.Send(&pb.AgentMessageChunk{
    Type:    pb.AgentMessageChunk_ERROR,
    Content: "Failed to process request",
    IsFinal: true,
})
```

## Complete Example: Multi-Capability Plugin

```go
package main

import (
    "encoding/json"
    "github.com/TykTechnologies/midsommar/v2/pkg/plugin_sdk"
    "github.com/TykTechnologies/midsommar/v2/pkg/plugin_sdk/pb"
)

type RequestLogger struct {
    plugin_sdk.BasePlugin
}

func NewRequestLogger() *RequestLogger {
    return &RequestLogger{
        BasePlugin: plugin_sdk.NewBasePlugin(
            "request-logger",
            "1.0.0",
            "Logs requests and responses",
        ),
    }
}

// PostAuthHandler: Log incoming requests
func (p *RequestLogger) HandlePostAuth(ctx plugin_sdk.Context, req *pb.EnrichedRequest) (*pb.PluginResponse, error) {
    ctx.Services.Logger().Info("Incoming request",
        "app_id", ctx.AppID,
        "user_id", ctx.UserID,
        "path", req.Path,
        "method", req.Method,
    )

    // Store request metadata in KV
    metadata := map[string]interface{}{
        "timestamp": time.Now().Unix(),
        "path":      req.Path,
        "method":    req.Method,
    }
    data, _ := json.Marshal(metadata)
    ctx.Services.KV().Write(ctx, fmt.Sprintf("req:%s", req.RequestId), data)

    return &pb.PluginResponse{Modified: false}, nil
}

// ResponseHandler: Log responses
func (p *RequestLogger) OnBeforeWriteHeaders(ctx plugin_sdk.Context, req *pb.ResponseWriteRequest) (*pb.ResponseWriteResponse, error) {
    ctx.Services.Logger().Info("Outgoing response",
        "app_id", ctx.AppID,
        "status_code", req.StatusCode,
        "request_id", req.RequestId,
    )

    return &pb.ResponseWriteResponse{Modified: false}, nil
}

// ConfigProvider: Provide config schema
func (p *RequestLogger) GetConfigSchema() ([]byte, error) {
    schema := map[string]interface{}{
        "type": "object",
        "properties": map[string]interface{}{
            "log_level": map[string]interface{}{
                "type":    "string",
                "enum":    []string{"debug", "info", "warn", "error"},
                "default": "info",
            },
        },
    }
    return json.Marshal(schema)
}

func main() {
    plugin_sdk.Serve(NewRequestLogger())
}
```

## Testing Plugins

### Unit Testing

```go
func TestPluginLogic(t *testing.T) {
    plugin := NewRequestLogger()

    ctx := plugin_sdk.Context{
        Runtime: plugin_sdk.RuntimeStudio,
        AppID:   1,
    }

    req := &pb.EnrichedRequest{
        Path:   "/api/v1/chat",
        Method: "POST",
    }

    resp, err := plugin.HandlePostAuth(ctx, req)
    if err != nil {
        t.Fatalf("HandlePostAuth failed: %v", err)
    }

    if resp.Block {
        t.Error("Expected request to not be blocked")
    }
}
```

### Integration Testing

See working examples in [`examples/plugins/`](../../../examples/plugins/) for integration test patterns.

## Best Practices

### Configuration
- Validate configuration in `Initialize()`
- Extract broker ID for Service API access
- Set sensible defaults
- Return errors for invalid config

### Service API Usage
- Always check runtime before calling runtime-specific services
- Use context timeouts for external calls
- Cache frequently accessed data in KV storage
- Handle service errors gracefully

### Performance
- Minimize Service API calls in request path
- Use KV storage for caching
- Avoid blocking operations in handlers
- Use goroutines for async work (clean up in Shutdown)

### Resource Management
- Clean up resources in `Shutdown()` method
- Close connections and file handles
- Cancel background goroutines
- Clear caches

### Security
- Validate all inputs
- Sanitize log output (no secrets)
- Use secure defaults
- Follow least privilege principle

## Migration from Old SDKs

If you have existing plugins using the old Microgateway SDK (`microgateway/plugins/sdk`) or AI Studio SDK (`pkg/ai_studio_sdk`), see the [Migration Guide](plugins-migration-guide.md) for step-by-step instructions.

## Next Steps

- [Object Hooks Guide](plugins-object-hooks.md) - Intercept CRUD operations
- [Microgateway Plugins Guide](plugins-microgateway.md) - Gateway-specific patterns
- [AI Studio UI Plugins Guide](plugins-studio-ui.md) - Build plugin UIs
- [AI Studio Agent Plugins Guide](plugins-studio-agent.md) - Build conversational agents
- [Edge-to-Control Communication](plugins-edge-to-control.md) - Send data from edge to control plane
- [Service API Reference](plugins-service-api.md) - Complete API documentation
- [Plugin Examples](plugins-examples.md) - Browse working examples
- [Best Practices Guide](plugins-best-practices.md) - Advanced patterns
