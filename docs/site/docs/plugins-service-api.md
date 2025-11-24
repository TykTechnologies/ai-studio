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
| ListDatasources, GetDatasource | `datasources.read` |
| CreateDatasource, UpdateDatasource, DeleteDatasource | `datasources.write` |
| GenerateEmbedding, StoreDocuments, ProcessAndStoreDocuments | `datasources.embeddings` |
| QueryDatasource, QueryDatasourceByVector | `datasources.query` |
| CreateSchedule, GetSchedule, ListSchedules, UpdateSchedule, DeleteSchedule | `scheduler.manage` |

## RAG & Embedding Services

AI Studio provides comprehensive RAG (Retrieval-Augmented Generation) capabilities through the Service API, enabling plugins to build custom document ingestion and semantic search workflows.

### Overview

The RAG Service APIs allow plugins to:
- Generate embeddings using configured embedders (OpenAI, Ollama, Vertex, etc.)
- Store pre-computed embeddings with custom chunking strategies
- Query vector stores with semantic search
- Build complex ingestion plugins (GitHub, Confluence, custom document processors)

**Key Benefit**: Plugins have **full control** over chunking, embedding generation, and storage - no forced workflows.

### Core RAG APIs

#### GenerateEmbedding

Generate embeddings for text chunks without storing them.

```go
resp, err := ai_studio_sdk.GenerateEmbedding(ctx, datasourceID, []string{
    "First chunk of text",
    "Second chunk of text",
    "Third chunk of text",
})

if err != nil || !resp.Success {
    return err
}

// resp.Vectors contains the embedding vectors
for i, vector := range resp.Vectors {
    fmt.Printf("Chunk %d embedding dimensions: %d\n", i, len(vector.Values))
}
```

**Required Scope**: `datasources.embeddings`

**Use Case**: Custom chunking workflows where you generate embeddings first, then decide what to store.

#### StoreDocuments

Store pre-computed embeddings in the vector store without regenerating them.

```go
documents := make([]*mgmtpb.DocumentWithEmbedding, len(chunks))
for i, chunk := range chunks {
    documents[i] = &mgmtpb.DocumentWithEmbedding{
        Content:   chunk,
        Embedding: preComputedEmbeddings[i],
        Metadata: map[string]string{
            "source":      "github",
            "repo":        "my-repo",
            "file":        "README.md",
            "chunk_index": fmt.Sprintf("%d", i),
        },
    }
}

resp, err := ai_studio_sdk.StoreDocuments(ctx, datasourceID, documents)
if err != nil || !resp.Success {
    return err
}

fmt.Printf("Stored %d documents\n", resp.StoredCount)
```

**Required Scope**: `datasources.embeddings`

**Use Case**: Complete control over embeddings - use custom models, external services, or cached embeddings.

**Supported Vector Stores**:
- ✅ Pinecone
- ✅ PGVector
- ✅ Chroma (v0.2.5+)
- ✅ Weaviate
- ⚠️ Qdrant (requires SDK installation)
- ⚠️ Redis (requires RediSearch configuration)

#### ProcessAndStoreDocuments

Convenience method that generates embeddings and stores in one step.

```go
chunks := make([]*mgmtpb.DocumentChunk, len(texts))
for i, text := range texts {
    chunks[i] = &mgmtpb.DocumentChunk{
        Content: text,
        Metadata: map[string]string{
            "source": "api",
            "index":  fmt.Sprintf("%d", i),
        },
    }
}

resp, err := ai_studio_sdk.ProcessAndStoreDocuments(ctx, datasourceID, chunks)
if err != nil || !resp.Success {
    return err
}

fmt.Printf("Processed %d documents\n", resp.ProcessedCount)
```

**Required Scope**: `datasources.embeddings`

**Use Case**: Simplified workflow when you don't need to inspect or cache embeddings.

#### QueryDatasource

Semantic search using a text query (embedding generated automatically).

```go
resp, err := ai_studio_sdk.QueryDatasource(ctx, datasourceID,
    "How do I configure RAG in AI Studio?",
    10,   // maxResults
    0.75, // similarityThreshold
)

if err != nil || !resp.Success {
    return err
}

for _, result := range resp.Results {
    fmt.Printf("Score: %.2f | Content: %s\n",
        result.SimilarityScore,
        result.Content)
    // Access metadata
    for k, v := range result.Metadata {
        fmt.Printf("  %s: %s\n", k, v)
    }
}
```

**Required Scope**: `datasources.query`

**Use Case**: Standard semantic search - plugin provides text, system handles embedding.

#### QueryDatasourceByVector

Semantic search using a pre-computed embedding vector.

```go
// Generate query embedding
queryResp, _ := ai_studio_sdk.GenerateEmbedding(ctx, datasourceID, []string{"search query"})
queryVector := queryResp.Vectors[0].Values

// Search with the pre-computed vector
resp, err := ai_studio_sdk.QueryDatasourceByVector(ctx, datasourceID,
    queryVector,
    10,   // maxResults
    0.75, // similarityThreshold
)

for _, result := range resp.Results {
    fmt.Printf("Match: %s (score: %.2f)\n", result.Content, result.SimilarityScore)
}
```

**Required Scope**: `datasources.query`

**Use Case**: Advanced workflows with custom query embeddings or hybrid search strategies.

**Supported Vector Stores**:
- ✅ Pinecone
- ✅ PGVector
- ✅ Chroma
- ✅ Weaviate
- ⚠️ Qdrant (requires SDK)
- ⚠️ Redis (requires RediSearch)

### Complete Custom Ingestion Example

Building a GitHub repository documentation ingestion plugin:

```go
func (p *GitHubDocsPlugin) IngestRepository(ctx plugin_sdk.Context, repo string, datasourceID uint32) error {
    // Step 1: Fetch markdown files from GitHub
    files, err := p.fetchMarkdownFiles(repo)
    if err != nil {
        return err
    }

    // Step 2: Custom chunking strategy (semantic chunking by headers)
    var allChunks []string
    var allMetadata []map[string]string

    for _, file := range files {
        chunks := p.semanticChunker(file.Content) // Your custom logic
        for i, chunk := range chunks {
            allChunks = append(allChunks, chunk)
            allMetadata = append(allMetadata, map[string]string{
                "source":      "github",
                "repo":        repo,
                "file":        file.Path,
                "chunk_index": fmt.Sprintf("%d", i),
                "updated_at":  file.UpdatedAt,
            })
        }
    }

    // Step 3: Generate embeddings for all chunks
    embResp, err := ai_studio_sdk.GenerateEmbedding(ctx, datasourceID, allChunks)
    if err != nil || !embResp.Success {
        return fmt.Errorf("embedding generation failed: %v", err)
    }

    // Step 4: Store with pre-computed embeddings
    documents := make([]*mgmtpb.DocumentWithEmbedding, len(allChunks))
    for i := range allChunks {
        documents[i] = &mgmtpb.DocumentWithEmbedding{
            Content:   allChunks[i],
            Embedding: embResp.Vectors[i].Values,
            Metadata:  allMetadata[i],
        }
    }

    storeResp, err := ai_studio_sdk.StoreDocuments(ctx, datasourceID, documents)
    if err != nil || !storeResp.Success {
        return fmt.Errorf("storage failed: %v", err)
    }

    ctx.Services.Logger().Info("Successfully ingested repository",
        "repo", repo,
        "chunks", storeResp.StoredCount)

    return nil
}
```

### Datasource Configuration

For RAG APIs to work, datasources must be configured with:

**Embedder Configuration**:
- `EmbedVendor`: Embedder provider (`"openai"`, `"ollama"`, `"vertex"`, `"googleai"`)
- `EmbedModel`: Model name (e.g., `"text-embedding-3-small"` for OpenAI, `"nomic-embed-text"` for Ollama)
- `EmbedAPIKey`: API key if required by embedder
- `EmbedUrl`: Embedder endpoint URL

**Vector Store Configuration**:
- `DBSourceType`: Vector store type (`"pinecone"`, `"chroma"`, `"pgvector"`, `"qdrant"`, `"redis"`, `"weaviate"`)
- `DBConnString`: Connection URL for vector store
- `DBConnAPIKey`: API key if required
- `DBName`: Collection/namespace/table name

**Important**: `EmbedModel` must be the actual model name (e.g., `"text-embedding-3-small"`), NOT the vendor name!

### RAG Workflow Patterns

#### Pattern 1: Separate Generate & Store (Full Control)

```go
// Generate embeddings
embeddings, _ := ai_studio_sdk.GenerateEmbedding(ctx, dsID, customChunks)

// Store with pre-computed embeddings (no re-embedding!)
ai_studio_sdk.StoreDocuments(ctx, dsID, documentsWithEmbeddings)
```

**Best for**: Custom chunking algorithms, caching embeddings, using external embedding services.

#### Pattern 2: Process & Store (Convenience)

```go
// Generate and store in one step
ai_studio_sdk.ProcessAndStoreDocuments(ctx, dsID, chunks)
```

**Best for**: Simple ingestion when you don't need to inspect or cache embeddings.

#### Pattern 3: Hybrid Search

```go
// Generate embeddings for multiple query variants
variants := []string{"original query", "rephrased query", "expanded query"}
embeddings, _ := ai_studio_sdk.GenerateEmbedding(ctx, dsID, variants)

// Search with each variant and merge results
allResults := []Result{}
for _, emb := range embeddings.Vectors {
    results, _ := ai_studio_sdk.QueryDatasourceByVector(ctx, dsID, emb.Values, 5, 0.7)
    allResults = append(allResults, results.Results...)
}

// Deduplicate and rank
finalResults := deduplicateAndRank(allResults)
```

**Best for**: Advanced search strategies, query expansion, multi-vector search.

### Datasource Management APIs

For managing datasources programmatically:

```go
// List all datasources
datasources, err := ai_studio_sdk.ListDatasources(ctx, 1, 100, nil, "")

// Get specific datasource
ds, err := ai_studio_sdk.GetDatasource(ctx, datasourceID)

// Create datasource with full configuration
ds, err := ai_studio_sdk.CreateDatasourceWithEmbedder(ctx,
    "My RAG Datasource",
    "Short description",
    "Long description",
    "",                      // URL
    "http://localhost:8000", // Chroma connection
    "chroma",                // Vector store type
    "",                      // DB API key
    "my-collection",         // Collection name
    "openai",                // Embedder vendor
    "https://api.openai.com/v1/embeddings", // Embedder URL
    "sk-...",                // Embed API key
    "text-embedding-3-small", // Embed model
    5, 1, true,
)

// Update datasource
ds, err := ai_studio_sdk.UpdateDatasource(ctx, datasourceID, name, ...)

// Delete datasource
err := ai_studio_sdk.DeleteDatasource(ctx, datasourceID)

// Search datasources
results, err := ai_studio_sdk.SearchDatasources(ctx, "query")
```

**Required Scopes**: `datasources.read` (list/get/search), `datasources.write` (create/update/delete)

### Error Handling

```go
resp, err := ai_studio_sdk.GenerateEmbedding(ctx, dsID, chunks)
if err != nil {
    // gRPC communication error
    return fmt.Errorf("gRPC error: %w", err)
}

if !resp.Success {
    // Server-side validation or processing error
    ctx.Services.Logger().Error("Embedding generation failed",
        "error", resp.ErrorMessage,
        "datasource_id", dsID)
    return fmt.Errorf("embedding failed: %s", resp.ErrorMessage)
}

// Success - use resp.Vectors
```

**Common Errors**:
- `"datasource does not have embedder configured"` - Set EmbedVendor/EmbedModel/EmbedAPIKey
- `"datasource does not have vector store configured"` - Set DBSourceType/DBConnString/DBName
- `"failed to generate embeddings with openai/openai"` - EmbedModel should be model name, not vendor!
- `"vector store connection failed"` - Ensure vector store is running and accessible

### Advanced Datasource Operations

These operations provide fine-grained control over vector store data through metadata filtering and namespace management.

#### Delete Documents by Metadata

Delete specific documents from vector stores using metadata filters:

```go
// Delete all chunks for a specific file
count, err := ai_studio_sdk.DeleteDocumentsByMetadata(
    ctx,
    datasourceID,
    map[string]string{"file_path": "old-file.md"},
    "AND",  // filter mode: "AND" or "OR"
    false,  // dry_run: set true to preview without deleting
)

// Example with OR mode (delete documents matching any condition)
count, err := ai_studio_sdk.DeleteDocumentsByMetadata(
    ctx,
    datasourceID,
    map[string]string{
        "status": "archived",
        "expired": "true",
    },
    "OR",   // Matches documents with status=archived OR expired=true
    false,
)
```

**Parameters:**
- `metadataFilter`: map of metadata key-value pairs to match
- `filterMode`: `"AND"` (all conditions must match) or `"OR"` (any condition matches)
- `dryRun`: if `true`, returns count without deleting

**Returns:** Number of documents deleted (or would be deleted if dry-run)

**Scope Required**: `datasources.write`

#### Query by Metadata Only

Query documents using only metadata filters (no vector similarity):

```go
results, totalCount, err := ai_studio_sdk.QueryByMetadataOnly(
    ctx,
    datasourceID,
    map[string]string{"source": "internal-docs"},
    "AND",
    10,  // limit
    0,   // offset
)

// Process results
for _, result := range results {
    fmt.Printf("Content: %s\nMetadata: %v\n", result.Content, result.Metadata)
}
fmt.Printf("Total matching documents: %d\n", totalCount)
```

**Parameters:**
- `metadataFilter`: metadata key-value pairs to match
- `filterMode`: `"AND"` or `"OR"`
- `limit`: max results per page (1-100, default: 10)
- `offset`: pagination offset

**Returns:** Array of results and total count (for pagination)

**Scope Required**: `datasources.query`

#### List Namespaces

List all namespaces/collections in a vector store:

```go
namespaces, err := ai_studio_sdk.ListNamespaces(ctx, datasourceID)
for _, ns := range namespaces {
    fmt.Printf("Namespace: %s, Documents: %d\n", ns.Name, ns.DocumentCount)
}
```

**Returns:** Array of namespace info with document counts

**Scope Required**: `datasources.read`

**Note:** Document count may be `-1` if not supported by the vector store.

#### Delete Namespace

Delete an entire namespace/collection (bulk operation):

```go
// Requires confirm=true for safety
err := ai_studio_sdk.DeleteNamespace(ctx, datasourceID, "old-namespace", true)
```

**Parameters:**
- `namespace`: namespace/collection name to delete
- `confirm`: must be `true` to proceed (safety check)

**Scope Required**: `datasources.write`

**Warning:** This is a destructive operation that deletes all documents in the namespace. Use with caution.

**Supported Vector Stores:**
- ✅ Full support: Chroma, PGVector, Pinecone, Weaviate
- ⚠️ Limited: Redis (delete/query by metadata not fully supported)
- ⚠️ Partial: Qdrant (namespace management only)

## Schedule Management

**Scope Required**: `scheduler.manage`
**Available in**: AI Studio only

Plugins can programmatically manage their scheduled tasks using the Schedule Management API. This complements manifest-based schedule declarations.

### Overview

Schedules can be created in two ways:
1. **Manifest Schedules**: Declared in `plugin.manifest.json`, auto-registered when plugin loads
2. **API Schedules**: Created programmatically via SDK during `Initialize()` or at runtime

Both types execute via the `ExecuteScheduledTask()` capability method.

### CreateSchedule

Create a new schedule for your plugin:

```go
schedule, err := ai_studio_sdk.CreateSchedule(
    ctx,
    "hourly-sync",              // Schedule ID (unique per plugin)
    "Hourly Data Sync",         // Human-readable name
    "0 * * * *",                // Cron expression (5-field format)
    "UTC",                      // Timezone
    120,                        // Timeout in seconds
    map[string]interface{}{     // Config passed to ExecuteScheduledTask
        "batch_size": 100,
    },
    true,                       // Enabled
)
```

**Returns**: `*mgmtpb.ScheduleInfo` with schedule details
**Errors**: `AlreadyExists` if schedule_id already exists for this plugin

### GetSchedule

Retrieve schedule details by manifest schedule ID:

```go
schedule, err := ai_studio_sdk.GetSchedule(ctx, "hourly-sync")
```

**Returns**: `*mgmtpb.ScheduleInfo`
**Errors**: `NotFound` if schedule doesn't exist

### ListSchedules

Get all schedules for your plugin:

```go
schedules, err := ai_studio_sdk.ListSchedules(ctx)
```

**Returns**: `[]*mgmtpb.ScheduleInfo` array

### UpdateSchedule

Update schedule fields (all fields optional):

```go
enabled := false
timeout := int32(180)

schedule, err := ai_studio_sdk.UpdateSchedule(ctx, "hourly-sync", ai_studio_sdk.UpdateScheduleOptions{
    Name:           stringPtr("Updated Sync Task"),
    CronExpr:       stringPtr("30 * * * *"),  // Every hour at :30
    Timezone:       stringPtr("America/New_York"),
    TimeoutSeconds: &timeout,
    Enabled:        &enabled,
    Config: map[string]interface{}{
        "batch_size": 200,
    },
})
```

**Returns**: `*mgmtpb.ScheduleInfo` with updated schedule
**Errors**: `NotFound` if schedule doesn't exist

### DeleteSchedule

Remove a schedule:

```go
err := ai_studio_sdk.DeleteSchedule(ctx, "hourly-sync")
```

**Errors**: `NotFound` if schedule doesn't exist

### Complete Example

```go
package main

import (
    "context"
    "github.com/TykTechnologies/midsommar/v2/pkg/ai_studio_sdk"
    "github.com/TykTechnologies/midsommar/v2/pkg/plugin_sdk"
)

type MyPlugin struct {
    plugin_sdk.BasePlugin
}

// Initialize creates API-managed schedules
func (p *MyPlugin) Initialize(ctx plugin_sdk.Context, config map[string]string) error {
    if ctx.Runtime != plugin_sdk.RuntimeStudio {
        return nil
    }

    apiCtx := context.Background()

    // Check if schedule exists (idempotent)
    if _, err := ai_studio_sdk.GetSchedule(apiCtx, "data-refresh"); err != nil {
        // Create new schedule
        _, err := ai_studio_sdk.CreateSchedule(
            apiCtx,
            "data-refresh",
            "Refresh External Data",
            "*/15 * * * *",  // Every 15 minutes
            "UTC",
            300,  // 5 minute timeout
            map[string]interface{}{
                "api_endpoint": "https://api.example.com/data",
            },
            true,
        )
        if err != nil {
            return fmt.Errorf("failed to create schedule: %w", err)
        }
    }

    return nil
}

// ExecuteScheduledTask handles all scheduled executions
func (p *MyPlugin) ExecuteScheduledTask(ctx plugin_sdk.Context, schedule *plugin_sdk.Schedule) error {
    switch schedule.ID {
    case "data-refresh":
        return p.refreshData(ctx, schedule)
    default:
        return fmt.Errorf("unknown schedule: %s", schedule.ID)
    }
}

func (p *MyPlugin) refreshData(ctx plugin_sdk.Context, schedule *plugin_sdk.Schedule) error {
    // Access config from schedule
    endpoint := schedule.Config["api_endpoint"].(string)

    // Perform sync logic...
    ctx.Services.Logger().Info("Refreshing data", "endpoint", endpoint)

    return nil
}
```

### Manifest vs API Schedules

**Use Manifest When**:
- Schedule is core to plugin functionality
- Configuration is static
- Want schedules registered automatically

**Use API When**:
- Schedules are dynamic (based on external data)
- Need runtime modification
- Want conditional schedule creation
- Building schedule management UI

**Example**: Plugin manifest declares one immutable daily report, creates hourly syncs via API based on configured data sources.

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
- **Scheduler**: `examples/plugins/studio/scheduler-demo/` - Scheduled tasks with manifest and API patterns

## Next Steps

- **[Plugin SDK Reference](plugins-sdk.md)** - Core SDK documentation
- **[Plugin Manifests & Permissions](plugins-manifests.md)** - Declare service permissions
- **[AI Studio UI Plugins Guide](plugins-studio-ui.md)** - Build plugin UIs
- **[AI Studio Agent Plugins Guide](plugins-studio-agent.md)** - Build conversational agents
- **[Microgateway Plugins Guide](plugins-microgateway.md)** - Gateway-specific patterns
- **[Plugin Examples](plugins-examples.md)** - Browse all working examples
