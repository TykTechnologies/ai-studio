# Service API Reference

The Service API provides 100+ gRPC operations for AI Studio plugins to interact with the platform. Access is controlled via permission scopes declared in the plugin manifest.

## Overview

The Service API is available to:
- **AI Studio UI Plugins**: Full access to manage platform resources
- **AI Studio Agent Plugins**: Access to LLMs, tools, datasources for conversational AI
- **Microgateway Plugins**: No Service API access (stateless middleware)

## Authentication

Service API calls are authenticated via plugin context and broker ID:

```go
// Automatically handled by SDK
llmsResp, err := ai_studio_sdk.ListLLMs(ctx, 1, 10)
```

The SDK manages authentication via the gRPC broker pattern. Ensure your plugin declares required scopes in its manifest.

## LLM Operations

Requires: `llms.read`, `llms.write`, or `llms.proxy` scope

### List LLMs

```go
func ListLLMs(ctx context.Context, page, limit int32) (*mgmtpb.ListLLMsResponse, error)
```

Example:
```go
llmsResp, err := ai_studio_sdk.ListLLMs(ctx, 1, 10)
if err != nil {
    return err
}

for _, llm := range llmsResp.Llms {
    log.Printf("LLM: %s - %s %s", llm.Name, llm.Vendor, llm.DefaultModel)
}
```

Response fields:
- `Llms`: Array of LLM objects
- `TotalCount`: Total number of LLMs
- `Page`: Current page
- `Limit`: Page size

### Get LLM by ID

```go
func GetLLM(ctx context.Context, llmID uint32) (*mgmtpb.LLM, error)
```

Example:
```go
llm, err := ai_studio_sdk.GetLLM(ctx, 1)
if err != nil {
    return err
}

log.Printf("LLM %s uses %s %s", llm.Name, llm.Vendor, llm.DefaultModel)
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

## Tool Operations

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

## Next Steps

- [Plugin Manifests & Permissions]([plugins-manifests](https://docs.claude.com/en/docs/plugins-manifests))
- [SDK Reference]([plugins-sdk](https://docs.claude.com/en/docs/plugins-sdk))
- [AI Studio UI Plugins Guide]([plugins-studio-ui](https://docs.claude.com/en/docs/plugins-studio-ui))
- [AI Studio Agent Plugins Guide]([plugins-studio-agent](https://docs.claude.com/en/docs/plugins-studio-agent))
