# Clean AI Studio SDK Usage Pattern

This example demonstrates the **clean SDK pattern** for AI Studio plugins where developers simply import the SDK and call functions directly.

## Developer Experience

### Simple Plugin Structure
```go
import "github.com/TykTechnologies/midsommar/v2/pkg/ai_studio_sdk"

type MyPlugin struct {
    pluginID uint32
}

func (p *MyPlugin) OnInitialize(serviceAPI mgmtpb.AIStudioManagementServiceClient, pluginID uint32) error {
    p.pluginID = pluginID
    return nil
}

func (p *MyPlugin) HandleCall(method string, payload []byte) ([]byte, error) {
    ctx := context.Background()

    // Just call SDK functions directly - no client management needed
    plugins, err := ai_studio_sdk.ListPlugins(ctx, 1, 10)
    if err != nil {
        return nil, err
    }

    llms, err := ai_studio_sdk.ListLLMs(ctx, 1, 5)
    if err != nil {
        return nil, err
    }

    analytics, err := ai_studio_sdk.GetAnalyticsSummary(ctx, "24h")
    if err != nil {
        return nil, err
    }

    // Use the data to build response
    response := map[string]interface{}{
        "plugins": plugins.Plugins,
        "llms": llms.Llms,
        "analytics": analytics,
    }

    return json.Marshal(response)
}

func main() {
    pluginImpl := &MyPlugin{}
    ai_studio_sdk.ServePlugin(pluginImpl)
}
```

## Available SDK Functions

- `ai_studio_sdk.ListPlugins(ctx, page, limit)` - Get plugins from host
- `ai_studio_sdk.GetPlugin(ctx, pluginID)` - Get specific plugin details
- `ai_studio_sdk.ListLLMs(ctx, page, limit)` - Get LLMs from host
- `ai_studio_sdk.GetLLM(ctx, llmID)` - Get specific LLM details
- `ai_studio_sdk.ListTools(ctx, page, limit)` - Get tools from host
- `ai_studio_sdk.ListApps(ctx, page, limit)` - Get applications from host
- `ai_studio_sdk.GetAnalyticsSummary(ctx, timeRange)` - Get analytics data
- `ai_studio_sdk.GetPluginsCount(ctx)` - Helper to get total plugin count
- `ai_studio_sdk.GetLLMsCount(ctx)` - Helper to get total LLM count

## How It Works

1. **Plugin imports SDK**: No complex setup required
2. **SDK handles connection**: Uses the bidirectional gRPC connection established by go-plugin
3. **Service client injection**: Host automatically injects service client during plugin loading
4. **Function calls**: Plugin just calls SDK functions directly
5. **Authentication**: SDK automatically includes plugin context and scopes

## Migration from Old Pattern

### Old Injection Pattern (Complex)
```go
// OLD - Complex injection pattern
type Plugin struct {
    serviceProvider plugin_services.AIStudioServiceProvider
}

func (p *Plugin) InjectServiceProvider(provider plugin_services.AIStudioServiceProvider) {
    p.serviceProvider = provider
}

func (p *Plugin) someMethod() {
    if p.serviceProvider != nil {
        resp, err := p.serviceProvider.ListPlugins(ctx, &mgmtpb.ListPluginsRequest{...})
        // Complex manual request building
    }
}
```

### New Clean SDK Pattern (Simple)
```go
// NEW - Clean SDK pattern
import "github.com/TykTechnologies/midsommar/v2/pkg/ai_studio_sdk"

func (p *Plugin) someMethod() {
    // Just call functions directly
    plugins, err := ai_studio_sdk.ListPlugins(ctx, 1, 10)
    // Simple function calls
}
```

## Benefits

1. **Simple Developer Experience**: Import SDK, call functions
2. **No Client Management**: SDK handles all gRPC complexity internally
3. **No Injection Patterns**: No complex setup or provider injection needed
4. **Clean Function Calls**: Direct function calls instead of client method calls
5. **Automatic Authentication**: SDK handles plugin context and scopes automatically
6. **Same Connection**: Uses existing bidirectional gRPC connection (no extra network overhead)

## Example Implementation

See `RateLimitingUIPluginSDK` in this file for a complete working example of the clean SDK pattern.