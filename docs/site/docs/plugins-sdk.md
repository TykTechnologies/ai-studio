# Plugin SDK Reference

Tyk AI Studio provides two SDKs for plugin development: Microgateway SDK and AI Studio SDK. Choose the SDK based on your plugin type.

## SDK Overview

| SDK | Location | Plugin Types | Purpose |
|-----|----------|--------------|---------|
| **Microgateway SDK** | `microgateway/plugins/sdk` | Gateway plugins | Proxy middleware hooks |
| **AI Studio SDK** | `pkg/ai_studio_sdk` | UI & Agent plugins | Service API access, plugin lifecycle |

## Microgateway SDK

Location: `github.com/TykTechnologies/midsommar/microgateway/plugins/sdk`

### Installation

```bash
go get github.com/TykTechnologies/midsommar/microgateway/plugins/sdk
```

### Core Interfaces

#### BasePlugin

All plugins must implement:

```go
type BasePlugin interface {
    Initialize(config map[string]interface{}) error
    GetHookType() HookType
    GetName() string
    GetVersion() string
    Shutdown() error
}
```

#### Hook-Specific Interfaces

**PreAuthPlugin**:
```go
type PreAuthPlugin interface {
    BasePlugin
    ProcessPreAuth(ctx context.Context, req *PluginRequest, pluginCtx *PluginContext) (*PluginResponse, error)
}
```

**AuthPlugin**:
```go
type AuthPlugin interface {
    BasePlugin
    Authenticate(ctx context.Context, req *AuthRequest, pluginCtx *PluginContext) (*AuthResponse, error)
    ValidateToken(ctx context.Context, token string, pluginCtx *PluginContext) (*AuthResponse, error)
}
```

**PostAuthPlugin**:
```go
type PostAuthPlugin interface {
    BasePlugin
    ProcessPostAuth(ctx context.Context, req *EnrichedRequest, pluginCtx *PluginContext) (*PluginResponse, error)
}
```

**ResponsePlugin**:
```go
type ResponsePlugin interface {
    BasePlugin
    ProcessResponse(ctx context.Context, req *ResponseData, pluginCtx *PluginContext) (*PluginResponse, error)
}
```

**DataCollectionPlugin**:
```go
type DataCollectionPlugin interface {
    BasePlugin
    HandleProxyLog(ctx context.Context, req *ProxyLogData, pluginCtx *PluginContext) (*DataCollectionResponse, error)
    HandleAnalytics(ctx context.Context, req *AnalyticsData, pluginCtx *PluginContext) (*DataCollectionResponse, error)
    HandleBudgetUsage(ctx context.Context, req *BudgetUsageData, pluginCtx *PluginContext) (*DataCollectionResponse, error)
}
```

### Serving Plugins

```go
func ServePlugin(impl interface{})
```

Example:
```go
func main() {
    plugin := &MyPlugin{}
    sdk.ServePlugin(plugin)
}
```

### Data Structures

See [Microgateway Plugins Guide]([plugins-microgateway](https://docs.claude.com/en/docs/plugins-microgateway)) for complete reference.

## AI Studio SDK

Location: `github.com/TykTechnologies/midsommar/v2/pkg/ai_studio_sdk`

### Installation

```bash
go get github.com/TykTechnologies/midsommar/v2/pkg/ai_studio_sdk
```

### Plugin Interfaces

#### AIStudioPluginImplementation (UI Plugins)

```go
type AIStudioPluginImplementation interface {
    OnInitialize(serviceAPI mgmtpb.AIStudioManagementServiceClient, pluginID uint32) error
    OnShutdown() error
    GetAsset(assetPath string) ([]byte, string, error)
    GetManifest() ([]byte, error)
    HandleCall(method string, payload []byte) ([]byte, error)
    GetConfigSchema() ([]byte, error)
}
```

#### AgentPluginImplementation (Agent Plugins)

```go
type AgentPluginImplementation interface {
    OnInitialize(serviceAPI mgmtpb.AIStudioManagementServiceClient, pluginID uint32) error
    OnShutdown() error
    HandleAgentMessage(req *pb.AgentMessageRequest, stream grpc.ServerStreamingServer[pb.AgentMessageChunk]) error
    GetManifest() ([]byte, error)
    GetConfigSchema() ([]byte, error)
}
```

### Serving Plugins

```go
// UI Plugin
func ServePlugin(impl AIStudioPluginImplementation)

// Agent Plugin
func ServeAgentPlugin(impl AgentPluginImplementation)
```

Example:
```go
func main() {
    plugin := &MyUIPlugin{}
    ai_studio_sdk.ServePlugin(plugin)
}
```

### Service API Helpers

#### Initialization

```go
func Initialize(grpcServer *grpc.Server, broker *goplugin.GRPCBroker, pluginID uint32) error
func IsInitialized() bool
func SetPluginID(pluginID uint32)
func SetServiceBrokerID(brokerID uint32)
```

#### LLM Operations

```go
// Call LLM with streaming/non-streaming
func CallLLM(
    ctx context.Context,
    llmID uint32,
    model string,
    messages []*mgmtpb.LLMMessage,
    temperature float64,
    maxTokens int32,
    tools []*mgmtpb.LLMTool,
    stream bool,
) (mgmtpb.AIStudioManagementService_CallLLMClient, error)

// Simple non-streaming call
func CallLLMSimple(
    ctx context.Context,
    llmID uint32,
    model string,
    userMessage string,
) (string, error)

// List LLMs
func ListLLMs(ctx context.Context, page, limit int32) (*mgmtpb.ListLLMsResponse, error)

// Get LLM by ID
func GetLLM(ctx context.Context, llmID uint32) (*mgmtpb.LLM, error)

// Get LLMs count
func GetLLMsCount(ctx context.Context) (int64, error)
```

#### Tool Operations

```go
// List tools
func ListTools(ctx context.Context, page, limit int32) (*mgmtpb.ListToolsResponse, error)

// Get tool by ID
func GetTool(ctx context.Context, toolID uint32) (*mgmtpb.Tool, error)

// Execute tool
func ExecuteTool(
    ctx context.Context,
    toolID uint32,
    operationID string,
    parameters map[string]interface{},
) (*mgmtpb.ExecuteToolResponse, error)
```

#### Plugin Operations

```go
// List plugins
func ListPlugins(ctx context.Context, page, limit int32) (*mgmtpb.ListPluginsResponse, error)

// Get plugin by ID
func GetPlugin(ctx context.Context, pluginID uint32) (*mgmtpb.Plugin, error)

// Get plugins count
func GetPluginsCount(ctx context.Context) (int64, error)
```

#### App Operations

```go
// List apps
func ListApps(ctx context.Context, page, limit int32) (*mgmtpb.ListAppsResponse, error)

// Get app by ID
func GetApp(ctx context.Context, appID uint32) (*mgmtpb.App, error)
```

#### KV Storage Operations

```go
// Write data
func WritePluginKV(ctx context.Context, key string, value []byte) (bool, error)

// Read data
func ReadPluginKV(ctx context.Context, key string) ([]byte, error)

// Delete data
func DeletePluginKV(ctx context.Context, key string) error

// List keys
func ListPluginKVKeys(ctx context.Context, prefix string) ([]string, error)
```

### Common Patterns

#### Initialize and Store Service API

```go
type MyPlugin struct {
    serviceAPI mgmtpb.AIStudioManagementServiceClient
    pluginID   uint32
}

func (p *MyPlugin) OnInitialize(
    serviceAPI mgmtpb.AIStudioManagementServiceClient,
    pluginID uint32) error {

    p.serviceAPI = serviceAPI
    p.pluginID = pluginID
    return nil
}
```

#### Call LLM via SDK

```go
func (p *MyPlugin) callLLM(ctx context.Context, userMessage string) (string, error) {
    // Get first available LLM
    llmsResp, err := ai_studio_sdk.ListLLMs(ctx, 1, 1)
    if err != nil {
        return "", err
    }

    if len(llmsResp.Llms) == 0 {
        return "", fmt.Errorf("no LLMs available")
    }

    llm := llmsResp.Llms[0]

    // Call LLM
    response, err := ai_studio_sdk.CallLLMSimple(ctx, llm.Id, llm.DefaultModel, userMessage)
    if err != nil {
        return "", err
    }

    return response, nil
}
```

#### Use KV Storage

```go
func (p *MyPlugin) saveSettings(ctx context.Context, settings map[string]interface{}) error {
    data, err := json.Marshal(settings)
    if err != nil {
        return err
    }

    _, err = ai_studio_sdk.WritePluginKV(ctx, "settings", data)
    return err
}

func (p *MyPlugin) loadSettings(ctx context.Context) (map[string]interface{}, error) {
    data, err := ai_studio_sdk.ReadPluginKV(ctx, "settings")
    if err != nil {
        return nil, err
    }

    var settings map[string]interface{}
    if err := json.Unmarshal(data, &settings); err != nil {
        return nil, err
    }

    return settings, nil
}
```

## Error Handling

### Microgateway SDK

Return errors in response:

```go
return &sdk.PluginResponse{
    Block:        true,
    ErrorMessage: "Validation failed: invalid input",
}, nil
```

### AI Studio SDK

Return errors from methods:

```go
func (p *MyPlugin) HandleCall(method string, payload []byte) ([]byte, error) {
    if method == "" {
        return nil, fmt.Errorf("method cannot be empty")
    }
    // ...
}
```

For agents, send ERROR chunks:

```go
return stream.Send(&pb.AgentMessageChunk{
    Type:    pb.AgentMessageChunk_ERROR,
    Content: "Failed to process request",
    IsFinal: true,
})
```

## Testing

### Unit Testing Plugins

```go
func TestPluginInitialize(t *testing.T) {
    plugin := &MyPlugin{}

    config := map[string]interface{}{
        "api_key": "test123",
    }

    err := plugin.Initialize(config)
    if err != nil {
        t.Fatalf("Initialize failed: %v", err)
    }

    if plugin.apiKey != "test123" {
        t.Errorf("Expected apiKey=test123, got %s", plugin.apiKey)
    }
}
```

### Integration Testing

```go
func TestPluginWithLLM(t *testing.T) {
    // Requires running AI Studio instance
    ctx := context.Background()

    plugin := &MyPlugin{}
    plugin.OnInitialize(mockServiceAPI, 1)

    response, err := plugin.callLLM(ctx, "Hello")
    if err != nil {
        t.Fatalf("callLLM failed: %v", err)
    }

    if response == "" {
        t.Error("Expected non-empty response")
    }
}
```

## Best Practices

### Initialization

- Validate configuration in `Initialize()` or `OnInitialize()`
- Store service API and plugin ID references
- Set sensible defaults
- Return errors for invalid config

### Error Handling

- Always check errors from SDK calls
- Provide descriptive error messages
- Log errors with context (plugin ID, session ID)
- Implement graceful fallbacks

### Resource Management

- Close connections in `Shutdown()` or `OnShutdown()`
- Clean up goroutines
- Release file handles
- Clear caches

### Performance

- Use context timeouts for SDK calls
- Cache frequently accessed data
- Minimize Service API calls
- Use connection pooling for external services

## Next Steps

- [Microgateway Plugins Guide]([plugins-microgateway](https://docs.claude.com/en/docs/plugins-microgateway))
- [AI Studio UI Plugins Guide]([plugins-studio-ui](https://docs.claude.com/en/docs/plugins-studio-ui))
- [AI Studio Agent Plugins Guide]([plugins-studio-agent](https://docs.claude.com/en/docs/plugins-studio-agent))
- [Service API Reference]([plugins-service-api](https://docs.claude.com/en/docs/plugins-service-api))
