# Service API Reference

The Service API provides rich management capabilities for plugins to interact with the platform. Access is available through the **Unified Plugin SDK** via the `Context.Services` interface.

## Overview

Service API access is available to all plugins using the unified SDK (`pkg/plugin_sdk`), with different capabilities depending on the runtime:

### Universal Services (Both Runtimes)
- **KV Storage**: Key-value storage (PostgreSQL in Studio, local DB in Gateway)
- **Logger**: Structured logging

### Runtime-Specific Services
- **Gateway Services**: App management, LLM info, budget status, credential validation
- **Studio Services**: Full management API (LLMs, tools, apps, filters, tags, CallLLM)

## Access Pattern

All services are accessed through the `Context.Services` interface provided to your plugin handlers:

```go
func (p *MyPlugin) HandlePostAuth(ctx plugin_sdk.Context, req *pb.EnrichedRequest) (*pb.PluginResponse, error) {
    // Universal services
    ctx.Services.Logger().Info("Processing request", "app_id", ctx.AppID)
    data, err := ctx.Services.KV().Read(ctx, "my-key")

    // Runtime-specific services
    if ctx.Runtime == plugin_sdk.RuntimeStudio {
        llms, err := ctx.Services.Studio().ListLLMs(ctx, 1, 10)
    } else if ctx.Runtime == plugin_sdk.RuntimeGateway {
        app, err := ctx.Services.Gateway().GetApp(ctx, ctx.AppID)
    }

    return &pb.PluginResponse{Modified: false}, nil
}
```

## Initialization

For Service API access, plugins must extract the broker ID during initialization:

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

    return nil
}
```

## Universal Services

These services are available in both Studio and Gateway runtimes.

### KV Storage

Key-value storage for plugin data:
- **Studio**: PostgreSQL-backed, shared across hosts, durable
- **Gateway**: Local database, per-instance, ephemeral

#### Write Data

```go
err := ctx.Services.KV().Write(ctx, "my-key", []byte("value"))
```

Returns error if write fails.

Example:
```go
settings := map[string]interface{}{
    "enabled": true,
    "rate_limit": 100,
}

data, _ := json.Marshal(settings)
err := ctx.Services.KV().Write(ctx, "settings", data)
if err != nil {
    ctx.Services.Logger().Error("Failed to write settings", "error", err)
}
```

#### Read Data

```go
data, err := ctx.Services.KV().Read(ctx, "my-key")
```

Returns error if key doesn't exist.

Example:
```go
data, err := ctx.Services.KV().Read(ctx, "settings")
if err != nil {
    ctx.Services.Logger().Warn("Settings not found", "error", err)
    // Use defaults
}

var settings map[string]interface{}
json.Unmarshal(data, &settings)
```

#### Delete Data

```go
err := ctx.Services.KV().Delete(ctx, "my-key")
```

Example:
```go
err := ctx.Services.KV().Delete(ctx, "cache:user:123")
if err != nil {
    ctx.Services.Logger().Error("Failed to delete cache", "error", err)
}
```

#### List Keys

```go
keys, err := ctx.Services.KV().List(ctx, "prefix")
```

Example:
```go
keys, err := ctx.Services.KV().List(ctx, "cache:")
if err != nil {
    return err
}

for _, key := range keys {
    ctx.Services.Logger().Debug("Found key", "key", key)
}
```

### Logger

Structured logging with key-value pairs:

```go
ctx.Services.Logger().Info("Message", "key", "value")
ctx.Services.Logger().Warn("Warning", "error", err)
ctx.Services.Logger().Error("Error", "details", details)
ctx.Services.Logger().Debug("Debug info", "data", data)
```

Example:
```go
func (p *MyPlugin) HandlePostAuth(ctx plugin_sdk.Context, req *pb.EnrichedRequest) (*pb.PluginResponse, error) {
    ctx.Services.Logger().Info("Request received",
        "app_id", ctx.AppID,
        "user_id", ctx.UserID,
        "path", req.Path,
        "method", req.Method,
    )

    // Process request...

    ctx.Services.Logger().Info("Request processed",
        "app_id", ctx.AppID,
        "duration_ms", time.Since(startTime).Milliseconds(),
    )

    return &pb.PluginResponse{Modified: false}, nil
}
```

## Studio Services

Available when `ctx.Runtime == plugin_sdk.RuntimeStudio`.

### LLM Operations

Requires: `llms.read`, `llms.write`, or `llms.proxy` scope

#### List LLMs

```go
llms, err := ctx.Services.Studio().ListLLMs(ctx, page, limit)
```

**Alternative** (direct SDK call):
```go
llmsResp, err := ai_studio_sdk.ListLLMs(ctx, 1, 10)
```

Example:
```go
if ctx.Runtime == plugin_sdk.RuntimeStudio {
    llms, err := ctx.Services.Studio().ListLLMs(ctx, 1, 10)
    if err != nil {
        return err
    }

    // Type assert the response
    llmsResp := llms.(*studiomgmt.ListLLMsResponse)
    for _, llm := range llmsResp.Llms {
        ctx.Services.Logger().Info("LLM found",
            "name", llm.Name,
            "vendor", llm.Vendor,
            "model", llm.DefaultModel,
        )
    }
}
```

#### Get LLM

```go
llm, err := ctx.Services.Studio().GetLLM(ctx, llmID)
```

**Alternative** (direct SDK call):
```go
llm, err := ai_studio_sdk.GetLLM(ctx, 1)
```

### Call LLM (Streaming)

Requires: `llms.proxy` scope

```go
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
```

Example:
```go
messages := []*mgmtpb.LLMMessage{
    {Role: "user", Content: "What is the capital of France?"},
}

llmStream, err := ai_studio_sdk.CallLLM(ctx, 1, "gpt-4", messages, 0.7, 1000, nil, false)
if err != nil {
    return err
}

var response string
for {
    resp, err := llmStream.Recv()
    if err == io.EOF {
        break
    }
    if err != nil {
        return err
    }

    response += resp.Content

    if resp.Done {
        break
    }
}

log.Printf("LLM response: %s", response)
```

### Call LLM (Simple)

Convenience method for simple calls:

```go
func CallLLMSimple(ctx context.Context, llmID uint32, model string, userMessage string) (string, error)
```

Example:
```go
response, err := ai_studio_sdk.CallLLMSimple(ctx, 1, "gpt-4", "Hello, world!")
if err != nil {
    return err
}

log.Printf("Response: %s", response)
```

### Get LLMs Count

```go
func GetLLMsCount(ctx context.Context) (int64, error)
```

Example:
```go
count, err := ai_studio_sdk.GetLLMsCount(ctx)
if err != nil {
    return err
}

log.Printf("Total LLMs: %d", count)
```

**Note**: For complete Studio Services documentation including Tools, Apps, Plugins, Datasources, and Filters, see the examples in the working plugins at `examples/plugins/studio/service-api-test/`.

## Gateway Services

Available when `ctx.Runtime == plugin_sdk.RuntimeGateway`.

Gateway Services provide read-only access to essential gateway information.

### Get App

```go
app, err := ctx.Services.Gateway().GetApp(ctx, appID)
```

Returns app configuration. Type assert to `*gwmgmt.GetAppResponse`.

Example:
```go
if ctx.Runtime == plugin_sdk.RuntimeGateway {
    app, err := ctx.Services.Gateway().GetApp(ctx, ctx.AppID)
    if err != nil {
        ctx.Services.Logger().Error("Failed to get app", "error", err)
        return &pb.PluginResponse{Modified: false}, nil
    }

    appResp := app.(*gwmgmt.GetAppResponse)
    ctx.Services.Logger().Info("Processing request for app",
        "app_name", appResp.Name,
        "llm_count", len(appResp.Llms),
    )
}
```

### List Apps

```go
apps, err := ctx.Services.Gateway().ListApps(ctx)
```

Returns all apps accessible to the gateway. Type assert to `*gwmgmt.ListAppsResponse`.

### Get LLM

```go
llm, err := ctx.Services.Gateway().GetLLM(ctx, llmID)
```

Returns LLM configuration. Type assert to `*gwmgmt.GetLLMResponse`.

### List LLMs

```go
llms, err := ctx.Services.Gateway().ListLLMs(ctx)
```

Returns all LLMs configured for the gateway. Type assert to `*gwmgmt.ListLLMsResponse`.

### Get Budget Status

```go
status, err := ctx.Services.Gateway().GetBudgetStatus(ctx, appID)
```

Returns current budget status for an app. Type assert to `*gwmgmt.GetBudgetStatusResponse`.

Example:
```go
if ctx.Runtime == plugin_sdk.RuntimeGateway {
    status, err := ctx.Services.Gateway().GetBudgetStatus(ctx, ctx.AppID)
    if err != nil {
        ctx.Services.Logger().Error("Failed to get budget", "error", err)
        return &pb.PluginResponse{Modified: false}, nil
    }

    budgetResp := status.(*gwmgmt.GetBudgetStatusResponse)
    if budgetResp.RemainingBudget <= 0 {
        return &pb.PluginResponse{
            Block:        true,
            ErrorMessage: "Budget exceeded",
        }, nil
    }
}
```

### Get Model Price

```go
price, err := ctx.Services.Gateway().GetModelPrice(ctx, vendor, model)
```

Returns pricing information for a model. Type assert to `*gwmgmt.GetModelPriceResponse`.

### Validate Credential

```go
valid, err := ctx.Services.Gateway().ValidateCredential(ctx, token)
```

Validates a credential token. Type assert to `*gwmgmt.ValidateCredentialResponse`.

Example:
```go
if ctx.Runtime == plugin_sdk.RuntimeGateway {
    valid, err := ctx.Services.Gateway().ValidateCredential(ctx, req.Headers["Authorization"])
    if err != nil || !valid.(*gwmgmt.ValidateCredentialResponse).Valid {
        return &pb.PluginResponse{
            Block:        true,
            ErrorMessage: "Invalid credentials",
        }, nil
    }
}
```

## Tool Operations (Studio Only)

Requires: `tools.read`, `tools.write`, or `tools.execute` scope

### List Tools

```go
func ListTools(ctx context.Context, page, limit int32) (*mgmtpb.ListToolsResponse, error)
```

Example:
```go
toolsResp, err := ai_studio_sdk.ListTools(ctx, 1, 50)
if err != nil {
    return err
}

for _, tool := range toolsResp.Tools {
    log.Printf("Tool: %s (%s) - %s", tool.Name, tool.Slug, tool.Description)
    for _, op := range tool.Operations {
        log.Printf("  Operation: %s", op)
    }
}
```

### Get Tool by ID

```go
func GetTool(ctx context.Context, toolID uint32) (*mgmtpb.Tool, error)
```

Example:
```go
tool, err := ai_studio_sdk.GetTool(ctx, 1)
if err != nil {
    return err
}

log.Printf("Tool: %s - Type: %s", tool.Name, tool.ToolType)
```

### Execute Tool

Requires: `tools.execute` scope

```go
func ExecuteTool(
    ctx context.Context,
    toolID uint32,
    operationID string,
    parameters map[string]interface{},
) (*mgmtpb.ExecuteToolResponse, error)
```

Example:
```go
params := map[string]interface{}{
    "url": "https://api.example.com/users",
    "method": "GET",
}

result, err := ai_studio_sdk.ExecuteTool(ctx, 1, "http_request", params)
if err != nil {
    return err
}

log.Printf("Tool result: %s", result.Data)
```

## Plugin Operations

Requires: `plugins.read` or `plugins.write` scope

### List Plugins

```go
func ListPlugins(ctx context.Context, page, limit int32) (*mgmtpb.ListPluginsResponse, error)
```

Example:
```go
pluginsResp, err := ai_studio_sdk.ListPlugins(ctx, 1, 10)
if err != nil {
    return err
}

for _, plugin := range pluginsResp.Plugins {
    log.Printf("Plugin: %s - Type: %s, Active: %t",
        plugin.Name, plugin.PluginType, plugin.IsActive)
}
```

### Get Plugin by ID

```go
func GetPlugin(ctx context.Context, pluginID uint32) (*mgmtpb.Plugin, error)
```

Example:
```go
plugin, err := ai_studio_sdk.GetPlugin(ctx, 1)
if err != nil {
    return err
}

log.Printf("Plugin: %s - Hook: %s", plugin.Name, plugin.HookType)
```

### Get Plugins Count

```go
func GetPluginsCount(ctx context.Context) (int64, error)
```

Example:
```go
count, err := ai_studio_sdk.GetPluginsCount(ctx)
if err != nil {
    return err
}

log.Printf("Total plugins: %d", count)
```

## App Operations

Requires: `apps.read` or `apps.write` scope

### List Apps

```go
func ListApps(ctx context.Context, page, limit int32) (*mgmtpb.ListAppsResponse, error)
```

Example:
```go
appsResp, err := ai_studio_sdk.ListApps(ctx, 1, 10)
if err != nil {
    return err
}

for _, app := range appsResp.Apps {
    log.Printf("App: %s - LLMs: %d, Tools: %d",
        app.Name, len(app.Llms), len(app.Tools))
}
```

### Get App by ID

```go
func GetApp(ctx context.Context, appID uint32) (*mgmtpb.App, error)
```

Example:
```go
app, err := ai_studio_sdk.GetApp(ctx, 1)
if err != nil {
    return err
}

log.Printf("App: %s - Description: %s", app.Name, app.Description)
```

## KV Storage Operations

Requires: `kv.read` or `kv.readwrite` scope

### Write Data

```go
func WritePluginKV(ctx context.Context, key string, value []byte) (bool, error)
```

Returns `true` if created, `false` if updated.

Example:
```go
settings := map[string]interface{}{
    "enabled": true,
    "rate_limit": 100,
}

data, _ := json.Marshal(settings)
created, err := ai_studio_sdk.WritePluginKV(ctx, "settings", data)
if err != nil {
    return err
}

if created {
    log.Println("Settings created")
} else {
    log.Println("Settings updated")
}
```

### Read Data

```go
func ReadPluginKV(ctx context.Context, key string) ([]byte, error)
```

Example:
```go
data, err := ai_studio_sdk.ReadPluginKV(ctx, "settings")
if err != nil {
    return err
}

var settings map[string]interface{}
json.Unmarshal(data, &settings)

log.Printf("Settings: %+v", settings)
```

### Delete Data

```go
func DeletePluginKV(ctx context.Context, key string) error
```

Example:
```go
err := ai_studio_sdk.DeletePluginKV(ctx, "settings")
if err != nil {
    log.Printf("Failed to delete: %v", err)
}
```

### List Keys

```go
func ListPluginKVKeys(ctx context.Context, prefix string) ([]string, error)
```

Example:
```go
keys, err := ai_studio_sdk.ListPluginKVKeys(ctx, "config:")
if err != nil {
    return err
}

for _, key := range keys {
    log.Printf("Key: %s", key)
}
```

## Data Types

### LLMMessage

```go
type LLMMessage struct {
    Role    string  // "user", "assistant", "system"
    Content string  // Message content
}
```

### LLMTool (for tool calling)

```go
type LLMTool struct {
    Type     string  // "function"
    Function *LLMToolFunction
}

type LLMToolFunction struct {
    Name        string
    Description string
    Parameters  map[string]interface{}  // JSON Schema
}
```

### Tool

```go
type Tool struct {
    Id              uint32
    Name            string
    Slug            string
    Description     string
    ToolType        string  // "rest", "graphql", "grpc", etc.
    Operations      []string
    IsActive        bool
    PrivacyScore    int32
}
```

### Plugin

```go
type Plugin struct {
    Id         uint32
    Name       string
    Slug       string
    PluginType string  // "gateway", "ai_studio", "agent"
    HookType   string  // Hook type
    IsActive   bool
    Command    string  // file://, grpc://, oci://
}
```

### App

```go
type App struct {
    Id          uint32
    Name        string
    Description string
    Llms        []*LLM
    Tools       []*Tool
    Datasources []*Datasource
}
```

## Error Handling

Service API calls return standard Go errors:

```go
llmsResp, err := ai_studio_sdk.ListLLMs(ctx, 1, 10)
if err != nil {
    log.Printf("Failed to list LLMs: %v", err)
    return err
}
```

Common error types:
- Permission denied: Missing required scope
- Not found: Resource doesn't exist
- Invalid argument: Bad request parameters
- Unavailable: Service not ready

## Rate Limiting

Service API calls are subject to rate limiting:

- Default: 1000 requests/minute per plugin
- Configurable via platform settings
- Implement exponential backoff for retries

Example retry logic:

```go
func callWithRetry(ctx context.Context, fn func() error) error {
    maxRetries := 3
    backoff := time.Second

    for i := 0; i < maxRetries; i++ {
        err := fn()
        if err == nil {
            return nil
        }

        if i < maxRetries-1 {
            time.Sleep(backoff)
            backoff *= 2
        }
    }

    return fmt.Errorf("max retries exceeded")
}
```

## Context and Timeouts

Always use contexts with timeouts:

```go
ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()

llmsResp, err := ai_studio_sdk.ListLLMs(ctx, 1, 10)
```

## Best Practices

1. **Check SDK Initialization**:
   ```go
   if !ai_studio_sdk.IsInitialized() {
       return fmt.Errorf("SDK not initialized")
   }
   ```

2. **Handle Pagination**:
   ```go
   page := int32(1)
   limit := int32(100)

   for {
       resp, err := ai_studio_sdk.ListTools(ctx, page, limit)
       if err != nil {
           return err
       }

       // Process tools...

       if len(resp.Tools) < int(limit) {
           break  // Last page
       }

       page++
   }
   ```

3. **Cache Results**:
   ```go
   // Cache LLM list for 5 minutes
   var cachedLLMs []*mgmtpb.LLM
   var cacheTime time.Time

   if time.Since(cacheTime) > 5*time.Minute {
       resp, _ := ai_studio_sdk.ListLLMs(ctx, 1, 100)
       cachedLLMs = resp.Llms
       cacheTime = time.Now()
   }
   ```

4. **Error Logging**:
   ```go
   llmsResp, err := ai_studio_sdk.ListLLMs(ctx, 1, 10)
   if err != nil {
       log.Printf("[Plugin %d] Failed to list LLMs: %v", p.pluginID, err)
       return err
   }
   ```

## Scope Requirements Summary

| Operation | Required Scope |
|-----------|----------------|
| ListLLMs, GetLLM | `llms.read` |
| CallLLM | `llms.proxy` |
| CreateLLM, UpdateLLM | `llms.write` |
| ListTools, GetTool | `tools.read` |
| ExecuteTool | `tools.execute` |
| CreateTool, UpdateTool | `tools.write` |
| ListApps, GetApp | `apps.read` |
| CreateApp, UpdateApp | `apps.write` |
| ListPlugins, GetPlugin | `plugins.read` |
| ReadPluginKV, ListPluginKVKeys | `kv.read` |
| WritePluginKV, DeletePluginKV | `kv.readwrite` |

## Best Practices Summary

1. **Runtime Detection**: Always check `ctx.Runtime` before calling runtime-specific services
2. **Type Assertions**: Gateway and Studio services return `interface{}`, type assert to correct response types
3. **Error Handling**: Always check errors from Service API calls
4. **Logging**: Use `ctx.Services.Logger()` for consistent structured logging
5. **KV Storage**: Understand storage differences between Studio (durable) and Gateway (ephemeral)
6. **Broker ID**: Extract and set broker ID during plugin initialization for Service API access
7. **Context Timeouts**: Use context timeouts for external calls
8. **Caching**: Cache frequently accessed data in KV storage to reduce API calls

## Complete Examples

For complete working examples of Service API usage:
- **Studio**: `examples/plugins/studio/service-api-test/` - Comprehensive Studio Services testing
- **Gateway**: `examples/plugins/gateway/gateway-service-test/` - Gateway Services examples
- **Rate Limiter**: `examples/plugins/studio/llm-rate-limiter-multiphase/` - Multi-capability plugin with KV storage

## Next Steps

- **[Plugin SDK Reference](plugins-sdk.md)** - Core SDK documentation
- **[Plugin Manifests & Permissions](plugins-manifests.md)** - Declare service permissions
- **[AI Studio UI Plugins Guide](plugins-studio-ui.md)** - Build plugin UIs
- **[AI Studio Agent Plugins Guide](plugins-studio-agent.md)** - Build conversational agents
- **[Microgateway Plugins Guide](plugins-microgateway.md)** - Gateway-specific patterns
- **[Plugin Examples](plugins-examples.md)** - Browse all working examples
