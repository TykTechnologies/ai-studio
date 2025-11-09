# Unified Plugin SDK for Midsommar

The Unified Plugin SDK provides a single, clean API for building plugins that work in **both** AI Studio and Microgateway contexts. Write your plugin once, and deploy it everywhere.

## Quick Start

```go
package main

import (
    "github.com/TykTechnologies/midsommar/v2/pkg/plugin_sdk"
    pb "github.com/TykTechnologies/midsommar/v2/proto"
)

type MyPlugin struct {
    plugin_sdk.BasePlugin
}

func NewMyPlugin() *MyPlugin {
    return &MyPlugin{
        BasePlugin: plugin_sdk.NewBasePlugin(
            "my-plugin",
            "1.0.0",
            "My awesome plugin",
        ),
    }
}

// Implement the PostAuthHandler capability
func (p *MyPlugin) HandlePostAuth(ctx plugin_sdk.Context, req *pb.EnrichedRequest) (*pb.PluginResponse, error) {
    // Your plugin logic here
    ctx.Services.Logger().Info("Request received", "app_id", ctx.AppID)

    // Allow the request
    return &pb.PluginResponse{Modified: false}, nil
}

func main() {
    plugin_sdk.Serve(NewMyPlugin())
}
```

## Key Concepts

### Runtime Detection

The SDK automatically detects whether it's running in AI Studio or Microgateway:

- **AI Studio**: Management UI, policy configuration, service orchestration
- **Microgateway**: Edge proxy, request processing, policy enforcement

Your plugin can adapt its behavior based on the runtime:

```go
func (p *MyPlugin) Initialize(ctx plugin_sdk.Context, config map[string]string) error {
    if ctx.Runtime == plugin_sdk.RuntimeStudio {
        // Setup for Studio (UI serving, etc.)
    } else {
        // Setup for Gateway (rate limiting, etc.)
    }
    return nil
}
```

### Capability-Based Design

Plugins only implement the capabilities they need. Available capabilities:

| Capability | Purpose | Example Use Cases |
|------------|---------|-------------------|
| `PreAuthHandler` | Process requests before authentication | Early validation, request rewriting |
| `AuthHandler` | Perform custom authentication | Custom auth schemes, credential validation |
| `PostAuthHandler` | Process requests after authentication | Rate limiting, content filtering, enrichment |
| `ResponseHandler` | Modify responses before sending to client | Header manipulation, response transformation |
| `DataCollector` | Collect telemetry data | Export to analytics systems, monitoring |
| `UIProvider` | Serve web UI assets for AI Studio | Management interface, dashboards |
| `ConfigProvider` | Provide configuration schema | Admin UI form generation |
| `AgentPlugin` | Implement conversational AI agent | Custom agent behavior |

### Context and Services

The `Context` object provides:

- **Runtime information**: Which environment (Studio/Gateway), request details
- **Services**: KV storage, logging, app management

```go
func (p *MyPlugin) HandlePostAuth(ctx plugin_sdk.Context, req *pb.EnrichedRequest) (*pb.PluginResponse, error) {
    // Access services
    data, err := ctx.Services.KV().Read(ctx, "my-key")
    ctx.Services.Logger().Info("Processing request", "app_id", ctx.AppID)

    // Get app details
    app, err := ctx.Services.AppManager().GetApp(ctx, ctx.AppID)

    // Your logic here
    return &pb.PluginResponse{Modified: false}, nil
}
```

## Available Services

### KV Storage

Persistent key-value storage for plugin data:

```go
// Write data
ctx.Services.KV().Write(ctx, "rate:app:123", []byte(`{"count": 5}`))

// Read data
data, err := ctx.Services.KV().Read(ctx, "rate:app:123")

// Delete data
deleted, err := ctx.Services.KV().Delete(ctx, "rate:app:123")
```

**Important**: In Studio, KV storage is PostgreSQL-backed and shared across all hosts. In Gateway, it uses local database storage per instance.

### Logging

Structured logging with standard levels:

```go
ctx.Services.Logger().Debug("Detailed debugging info", "key", value)
ctx.Services.Logger().Info("Information message", "key", value)
ctx.Services.Logger().Warn("Warning message", "key", value)
ctx.Services.Logger().Error("Error message", "key", value)
```

### App Management

Access to application configuration and metadata:

```go
// Get app details
appResp, err := ctx.Services.AppManager().GetApp(ctx, appID)
app := appResp.App

// List all apps
appsResp, err := ctx.Services.AppManager().ListApps(ctx, page, limit)

// Update app (including metadata)
updateReq := &mgmt.UpdateAppRequest{
    AppId:    appID,
    Name:     app.Name,
    Metadata: newMetadataJSON,
    // ... other fields
}
appResp, err := ctx.Services.AppManager().UpdateApp(ctx, updateReq)
```

## Building a Complete Plugin

Here's a complete example of a rate limiter plugin that works in both contexts:

```go
package main

import (
    "encoding/json"
    "fmt"
    "sync"

    "github.com/TykTechnologies/midsommar/v2/pkg/plugin_sdk"
    pb "github.com/TykTechnologies/midsommar/v2/proto"
)

type RateLimiterPlugin struct {
    plugin_sdk.BasePlugin
    rateLocks map[string]*sync.Mutex
}

func NewRateLimiterPlugin() *RateLimiterPlugin {
    return &RateLimiterPlugin{
        BasePlugin: plugin_sdk.NewBasePlugin(
            "rate-limiter",
            "1.0.0",
            "Token-based rate limiting",
        ),
        rateLocks: make(map[string]*sync.Mutex),
    }
}

// PostAuthHandler capability - enforces rate limits
func (p *RateLimiterPlugin) HandlePostAuth(ctx plugin_sdk.Context, req *pb.EnrichedRequest) (*pb.PluginResponse, error) {
    appID := req.Request.Context.AppId

    // Get rate limit from KV
    key := fmt.Sprintf("rate:%d", appID)
    data, err := ctx.Services.KV().Read(ctx, key)
    if err != nil {
        // No rate limit configured
        return &pb.PluginResponse{Modified: false}, nil
    }

    var rateData struct {
        Limit int `json:"limit"`
        Count int `json:"count"`
    }
    json.Unmarshal(data, &rateData)

    // Check limit
    if rateData.Count >= rateData.Limit {
        return &pb.PluginResponse{
            Block:      true,
            StatusCode: 429,
            Body:       []byte(`{"error":"Rate limit exceeded"}`),
        }, nil
    }

    // Increment counter
    rateData.Count++
    updatedData, _ := json.Marshal(rateData)
    ctx.Services.KV().Write(ctx, key, updatedData)

    return &pb.PluginResponse{Modified: false}, nil
}

// UIProvider capability - serves management UI
func (p *RateLimiterPlugin) GetAsset(path string) ([]byte, string, error) {
    // Serve embedded UI assets
    return embeddedAssets.ReadFile(path)
}

func (p *RateLimiterPlugin) GetManifest() ([]byte, error) {
    return manifestFile, nil
}

func (p *RateLimiterPlugin) HandleRPC(method string, payload []byte) ([]byte, error) {
    // Handle RPC calls from UI
    switch method {
    case "setLimit":
        var req struct {
            AppID uint32 `json:"app_id"`
            Limit int    `json:"limit"`
        }
        json.Unmarshal(payload, &req)

        // Store limit in KV
        data, _ := json.Marshal(map[string]int{
            "limit": req.Limit,
            "count": 0,
        })
        ai_studio_sdk.WritePluginKV(context.Background(),
            fmt.Sprintf("rate:%d", req.AppID), data)

        return []byte(`{"success":true}`), nil
    }
    return nil, fmt.Errorf("unknown method: %s", method)
}

func main() {
    plugin_sdk.Serve(NewRateLimiterPlugin())
}
```

## Plugin Manifest

For UI plugins, create a `manifest.json`:

```json
{
  "name": "my-plugin",
  "version": "1.0.0",
  "description": "My awesome plugin",
  "ui": {
    "entry_point": "ui/webc/main.js",
    "components": [
      {
        "type": "page",
        "id": "settings",
        "label": "Settings",
        "icon": "settings"
      }
    ]
  },
  "permissions": {
    "scopes": [
      "apps.read",
      "apps.write",
      "kv.readwrite"
    ]
  }
}
```

## Configuration Schema

Provide a JSON Schema for configuration:

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "properties": {
    "default_limit": {
      "type": "integer",
      "title": "Default Rate Limit",
      "description": "Default requests per minute",
      "default": 100
    }
  }
}
```

## Building and Deploying

### Building

```bash
cd your-plugin
go build -o plugin-name .
```

### Deploying to AI Studio

1. Upload plugin binary via Admin UI or API
2. Configure plugin settings
3. Assign to LLMs or apps

### Deploying to Microgateway

1. Place plugin binary in plugins directory
2. Configure in `microgateway.yaml`:

```yaml
plugins:
  - name: my-plugin
    command: ./plugins/my-plugin
    hook_type: post_auth
    config:
      setting1: value1
```

3. Restart gateway

## Migration from Old SDKs

### From AI Studio SDK

**Before** (AI Studio SDK):
```go
type MyPlugin struct {
    pb.UnimplementedPluginServiceServer
    serviceAPI mgmt.AIStudioManagementServiceClient
}

func (p *MyPlugin) OnInitialize(serviceAPI mgmt.AIStudioManagementServiceClient, pluginID uint32, config map[string]string) error {
    p.serviceAPI = serviceAPI
    return nil
}

func main() {
    plugin := &MyPlugin{}
    ai_studio_sdk.ServePlugin(plugin)
}
```

**After** (Unified SDK):
```go
type MyPlugin struct {
    plugin_sdk.BasePlugin
}

func NewMyPlugin() *MyPlugin {
    return &MyPlugin{
        BasePlugin: plugin_sdk.NewBasePlugin("my-plugin", "1.0.0", "Description"),
    }
}

func (p *MyPlugin) Initialize(ctx plugin_sdk.Context, config map[string]string) error {
    // Services available via ctx.Services
    return nil
}

func main() {
    plugin_sdk.Serve(NewMyPlugin())
}
```

### From Microgateway SDK

**Before** (Microgateway SDK):
```go
type MyPlugin struct{}

func (p *MyPlugin) ProcessRequest(ctx context.Context, req *sdk.EnrichedRequest, pluginCtx *sdk.PluginContext) (*sdk.PluginResponse, error) {
    // Process request
}

func (p *MyPlugin) GetHookType() sdk.HookType {
    return sdk.HookTypePostAuth
}

func main() {
    sdk.ServePlugin(&MyPlugin{})
}
```

**After** (Unified SDK):
```go
type MyPlugin struct {
    plugin_sdk.BasePlugin
}

func (p *MyPlugin) HandlePostAuth(ctx plugin_sdk.Context, req *pb.EnrichedRequest) (*pb.PluginResponse, error) {
    // Same processing logic
}

func main() {
    plugin_sdk.Serve(NewMyPlugin())
}
```

## Complete Example

See the [llm-rate-limiter example](examples/llm-rate-limiter/) for a complete, production-ready plugin that:

- ✅ Works in both AI Studio and Microgateway
- ✅ Provides UI for policy management
- ✅ Enforces rate limits at the edge
- ✅ Uses KV storage for policies and rate state
- ✅ Includes configuration schema
- ✅ Demonstrates all major capabilities

## Troubleshooting

### Plugin not loading

1. Check plugin binary is executable: `chmod +x plugin-name`
2. Verify handshake configuration matches (should be automatic with unified SDK)
3. Check logs for initialization errors

### Services not available

Ensure you're accessing services through the context:
```go
func (p *MyPlugin) HandlePostAuth(ctx plugin_sdk.Context, req *pb.EnrichedRequest) (*pb.PluginResponse, error) {
    // ✅ Correct
    ctx.Services.KV().Read(ctx, "key")

    // ❌ Wrong - services are on the context
    // p.kv.Read(ctx, "key")
}
```

### Service broker not initialized

If you see "service broker ID not set" errors, the broker configuration is not being passed correctly. The SDK automatically extracts the broker ID from the config in the `Initialize` method, checking for both `_service_broker_id` (Microgateway) and `service_broker_id` (AI Studio). This should happen automatically - if it doesn't, check that your plugin manager is calling `Initialize` with the config map.

### KV data not persisting

In Gateway context, KV data is local to each gateway instance. For shared data across gateways, store in App metadata via the AppManager service.

## Best Practices

1. **Use Context Services**: Always access KV, logging, and app management through `ctx.Services`
2. **Handle Both Runtimes**: Check `ctx.Runtime` and adapt behavior accordingly
3. **Embed Assets**: Use `//go:embed` for UI assets and manifests
4. **Graceful Degradation**: If a service is unavailable, fail gracefully
5. **Structured Logging**: Use the Logger service with key-value pairs
6. **Atomic Operations**: Use mutexes for rate limiting and concurrent access
7. **Configuration Schema**: Always provide a JSON schema for your config

## API Reference

See the godoc for complete API reference:

```bash
godoc -http=:6060
# Visit http://localhost:6060/pkg/github.com/TykTechnologies/midsommar/v2/pkg/plugin_sdk/
```

## Support

For issues, questions, or contributions, see the main Midsommar repository.
