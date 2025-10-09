# Microgateway Plugins Guide

Microgateway plugins provide middleware hooks in the LLM proxy request/response pipeline. Use them for custom authentication, request/response transformation, content filtering, and data collection to external systems.

## Hook Types

Microgateway plugins implement one of five hook types:

### 1. Pre-Auth Hook (`pre_auth`)

Executes **before** authentication. Use for:
- Request validation and early rejection
- Request enrichment with metadata
- Header modification
- Logging and auditing

**Example**: Message modifier that adds prefixes/suffixes to requests

### 2. Auth Hook (`auth`)

**Replaces** default token authentication. Use for:
- Custom authentication schemes (OAuth, JWT, API keys)
- Integration with external identity providers
- Multi-factor authentication
- Custom authorization logic

**Example**: Custom token validator, LDAP authentication

### 3. Post-Auth Hook (`post_auth`)

Executes **after** authentication. Use for:
- Enriching requests with user-specific data
- Per-user request transformation
- Access control enforcement
- Usage quota checks

**Example**: User-specific rate limiting, request enrichment

### 4. Response Hook (`on_response`)

Modifies LLM responses before returning to client. Use for:
- Response filtering and content moderation
- Response transformation and formatting
- Injecting additional metadata
- Response validation

**Example**: PII redaction, response formatting

### 5. Data Collection Hook (`data_collection`)

Intercepts data before database storage. Use for:
- Exporting proxy logs to external systems
- Sending analytics to data warehouses
- Custom budget tracking
- Real-time monitoring and alerting

**Example**: Elasticsearch collector, ClickHouse exporter, Kafka producer

## Quick Start

### 1. Project Setup

```bash
# Create plugin directory
mkdir my-plugin && cd my-plugin

# Initialize Go module
go mod init github.com/myorg/my-plugin

# Add dependencies
go get github.com/TykTechnologies/midsommar/microgateway/plugins/sdk
```

### 2. Implement Plugin Interface

Every microgateway plugin must implement the `BasePlugin` interface:

```go
package main

import (
    "github.com/TykTechnologies/midsommar/microgateway/plugins/sdk"
)

type MyPlugin struct {
    config map[string]interface{}
}

// Initialize is called when plugin starts
func (p *MyPlugin) Initialize(config map[string]interface{}) error {
    p.config = config
    return nil
}

// GetHookType returns the hook type
func (p *MyPlugin) GetHookType() sdk.HookType {
    return sdk.HookTypePreAuth // or Auth, PostAuth, OnResponse, DataCollection
}

// GetName returns plugin name
func (p *MyPlugin) GetName() string {
    return "my-plugin"
}

// GetVersion returns plugin version
func (p *MyPlugin) GetVersion() string {
    return "1.0.0"
}

// Shutdown performs cleanup
func (p *MyPlugin) Shutdown() error {
    return nil
}

func main() {
    plugin := &MyPlugin{}
    sdk.ServePlugin(plugin) // Blocks until shutdown
}
```

### 3. Implement Hook-Specific Interface

Depending on your hook type, implement the corresponding interface:

#### PreAuthPlugin

```go
func (p *MyPlugin) ProcessPreAuth(ctx context.Context,
    req *sdk.PluginRequest,
    pluginCtx *sdk.PluginContext) (*sdk.PluginResponse, error) {

    // Modify request or reject
    return &sdk.PluginResponse{
        Modified: true,
        Headers: map[string]string{
            "X-Plugin-Processed": "true",
        },
        Body:  req.Body,
        Block: false, // Set to true to reject request
    }, nil
}
```

#### AuthPlugin

```go
func (p *MyPlugin) Authenticate(ctx context.Context,
    req *sdk.AuthRequest,
    pluginCtx *sdk.PluginContext) (*sdk.AuthResponse, error) {

    // Validate credentials
    if req.Credential == "valid-token" {
        return &sdk.AuthResponse{
            Authenticated: true,
            UserID:        "user-123",
            AppID:         "app-456",
            Claims: map[string]string{
                "role": "admin",
            },
        }, nil
    }

    return &sdk.AuthResponse{
        Authenticated: false,
        ErrorMessage:  "Invalid credentials",
    }, nil
}

func (p *MyPlugin) ValidateToken(ctx context.Context,
    token string,
    pluginCtx *sdk.PluginContext) (*sdk.AuthResponse, error) {
    // Token validation logic
    return &sdk.AuthResponse{
        Authenticated: true,
        UserID:        "user-123",
    }, nil
}
```

#### PostAuthPlugin

```go
func (p *MyPlugin) ProcessPostAuth(ctx context.Context,
    req *sdk.EnrichedRequest,
    pluginCtx *sdk.PluginContext) (*sdk.PluginResponse, error) {

    // Access authenticated user info
    userID := req.UserID
    appID := req.AppID

    // Enrich or modify request
    return &sdk.PluginResponse{
        Modified: true,
        Body:     enrichedBody,
    }, nil
}
```

#### ResponsePlugin

```go
func (p *MyPlugin) ProcessResponse(ctx context.Context,
    req *sdk.ResponseData,
    pluginCtx *sdk.PluginContext) (*sdk.PluginResponse, error) {

    // Modify response
    modifiedBody := filterContent(req.ResponseBody)

    return &sdk.PluginResponse{
        Modified:   true,
        Body:       modifiedBody,
        StatusCode: req.StatusCode,
        Headers:    req.Headers,
    }, nil
}
```

#### DataCollectionPlugin

```go
func (p *MyPlugin) HandleProxyLog(ctx context.Context,
    req *sdk.ProxyLogData,
    pluginCtx *sdk.PluginContext) (*sdk.DataCollectionResponse, error) {

    // Send to external system
    err := p.sendToElasticsearch(req)
    return &sdk.DataCollectionResponse{
        Success: err == nil,
        Handled: true, // Set false to also store in database
    }, nil
}

func (p *MyPlugin) HandleAnalytics(ctx context.Context,
    req *sdk.AnalyticsData,
    pluginCtx *sdk.PluginContext) (*sdk.DataCollectionResponse, error) {

    // Process analytics data
    return &sdk.DataCollectionResponse{
        Success: true,
        Handled: true,
    }, nil
}

func (p *MyPlugin) HandleBudgetUsage(ctx context.Context,
    req *sdk.BudgetUsageData,
    pluginCtx *sdk.PluginContext) (*sdk.DataCollectionResponse, error) {

    // Track budget usage
    return &sdk.DataCollectionResponse{
        Success: true,
        Handled: true,
    }, nil
}
```

### 4. Build Plugin

```bash
# Build for current platform
go build -o my-plugin main.go

# Build for Linux (if deploying to Docker/K8s)
GOOS=linux GOARCH=amd64 go build -o my-plugin-linux main.go
```

### 5. Deploy Plugin

Create plugin in AI Studio dashboard or via API:

```bash
curl -X POST http://localhost:3000/api/v1/plugins \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "My Plugin",
    "slug": "my-plugin",
    "description": "Custom plugin",
    "command": "file:///path/to/my-plugin",
    "hook_type": "pre_auth",
    "is_active": true,
    "plugin_type": "gateway"
  }'
```

### 6. Attach to LLM

Associate the plugin with an LLM to activate it:

```bash
curl -X PUT http://localhost:3000/api/v1/llms/1/plugins \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "plugin_ids": [1, 2, 3]
  }'
```

## Configuration Schema

Provide JSON Schema for plugin configuration using the `ConfigSchemaProvider` interface:

```go
//go:embed config.schema.json
var configSchema []byte

func (p *MyPlugin) GetConfigSchema() ([]byte, error) {
    return configSchema, nil
}
```

Example `config.schema.json`:

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "properties": {
    "api_key": {
      "type": "string",
      "description": "API key for external service",
      "minLength": 1
    },
    "endpoint": {
      "type": "string",
      "format": "uri",
      "description": "External service endpoint",
      "default": "https://api.example.com"
    },
    "batch_size": {
      "type": "integer",
      "description": "Batch size for data collection",
      "minimum": 1,
      "maximum": 1000,
      "default": 100
    }
  },
  "required": ["api_key"]
}
```

Configuration values are passed to `Initialize()` and can be updated via the API.

## Complete Examples

### Example 1: Custom Authentication Plugin

```go
package main

import (
    "context"
    "strings"

    "github.com/TykTechnologies/midsommar/microgateway/plugins/sdk"
)

type CustomAuthPlugin struct {
    validToken string
}

func (p *CustomAuthPlugin) Initialize(config map[string]interface{}) error {
    if token, ok := config["valid_token"].(string); ok {
        p.validToken = token
    } else {
        p.validToken = "default-token"
    }
    return nil
}

func (p *CustomAuthPlugin) GetHookType() sdk.HookType {
    return sdk.HookTypeAuth
}

func (p *CustomAuthPlugin) GetName() string {
    return "custom-auth"
}

func (p *CustomAuthPlugin) GetVersion() string {
    return "1.0.0"
}

func (p *CustomAuthPlugin) Shutdown() error {
    return nil
}

func (p *CustomAuthPlugin) Authenticate(ctx context.Context,
    req *sdk.AuthRequest,
    pluginCtx *sdk.PluginContext) (*sdk.AuthResponse, error) {

    token := strings.TrimPrefix(req.Credential, "Bearer ")

    if token == p.validToken {
        return &sdk.AuthResponse{
            Authenticated: true,
            UserID:        "plugin-user",
            AppID:         "plugin-app",
            Claims: map[string]string{
                "source": "custom-auth-plugin",
            },
        }, nil
    }

    return &sdk.AuthResponse{
        Authenticated: false,
        ErrorMessage:  "Invalid token",
    }, nil
}

func (p *CustomAuthPlugin) ValidateToken(ctx context.Context,
    token string,
    pluginCtx *sdk.PluginContext) (*sdk.AuthResponse, error) {

    token = strings.TrimPrefix(token, "Bearer ")
    if token == p.validToken {
        return &sdk.AuthResponse{
            Authenticated: true,
            UserID:        "plugin-user",
            AppID:         "plugin-app",
        }, nil
    }

    return &sdk.AuthResponse{
        Authenticated: false,
        ErrorMessage:  "Invalid token",
    }, nil
}

func main() {
    plugin := &CustomAuthPlugin{}
    sdk.ServePlugin(plugin)
}
```

### Example 2: Elasticsearch Data Collector

```go
package main

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "time"

    "github.com/TykTechnologies/midsommar/microgateway/plugins/sdk"
)

type ElasticsearchCollector struct {
    esURL  string
    client *http.Client
}

func (p *ElasticsearchCollector) Initialize(config map[string]interface{}) error {
    if url, ok := config["elasticsearch_url"].(string); ok {
        p.esURL = url
    } else {
        p.esURL = "http://localhost:9200"
    }

    p.client = &http.Client{Timeout: 10 * time.Second}
    return nil
}

func (p *ElasticsearchCollector) GetHookType() sdk.HookType {
    return sdk.HookTypeDataCollection
}

func (p *ElasticsearchCollector) GetName() string {
    return "elasticsearch-collector"
}

func (p *ElasticsearchCollector) GetVersion() string {
    return "1.0.0"
}

func (p *ElasticsearchCollector) Shutdown() error {
    return nil
}

func (p *ElasticsearchCollector) HandleProxyLog(ctx context.Context,
    req *sdk.ProxyLogData,
    pluginCtx *sdk.PluginContext) (*sdk.DataCollectionResponse, error) {

    doc := map[string]interface{}{
        "@timestamp":    req.Timestamp.Format(time.RFC3339),
        "app_id":        req.AppID,
        "user_id":       req.UserID,
        "vendor":        req.Vendor,
        "request_body":  string(req.RequestBody),
        "response_body": string(req.ResponseBody),
        "response_code": req.ResponseCode,
        "request_id":    req.RequestID,
    }

    indexName := fmt.Sprintf("microgateway-proxy-logs-%s",
        req.Timestamp.Format("2006.01.02"))

    if err := p.indexDocument(ctx, indexName, doc); err != nil {
        return &sdk.DataCollectionResponse{
            Success:      false,
            Handled:      false,
            ErrorMessage: err.Error(),
        }, nil
    }

    return &sdk.DataCollectionResponse{
        Success: true,
        Handled: true, // Don't store in database
    }, nil
}

func (p *ElasticsearchCollector) HandleAnalytics(ctx context.Context,
    req *sdk.AnalyticsData,
    pluginCtx *sdk.PluginContext) (*sdk.DataCollectionResponse, error) {

    doc := map[string]interface{}{
        "@timestamp":      req.Timestamp.Format(time.RFC3339),
        "llm_id":         req.LLMID,
        "model_name":     req.ModelName,
        "vendor":         req.Vendor,
        "prompt_tokens":  req.PromptTokens,
        "response_tokens": req.ResponseTokens,
        "total_tokens":   req.TotalTokens,
        "cost":           req.Cost,
        "request_id":     req.RequestID,
    }

    indexName := fmt.Sprintf("microgateway-analytics-%s",
        req.Timestamp.Format("2006.01.02"))

    if err := p.indexDocument(ctx, indexName, doc); err != nil {
        return &sdk.DataCollectionResponse{
            Success:      false,
            Handled:      false,
            ErrorMessage: err.Error(),
        }, nil
    }

    return &sdk.DataCollectionResponse{
        Success: true,
        Handled: true,
    }, nil
}

func (p *ElasticsearchCollector) HandleBudgetUsage(ctx context.Context,
    req *sdk.BudgetUsageData,
    pluginCtx *sdk.PluginContext) (*sdk.DataCollectionResponse, error) {

    doc := map[string]interface{}{
        "@timestamp":      req.Timestamp.Format(time.RFC3339),
        "app_id":         req.AppID,
        "llm_id":         req.LLMID,
        "tokens_used":    req.TokensUsed,
        "cost":           req.Cost,
        "requests_count": req.RequestsCount,
        "period_start":   req.PeriodStart.Format(time.RFC3339),
        "period_end":     req.PeriodEnd.Format(time.RFC3339),
    }

    indexName := fmt.Sprintf("microgateway-budget-%s",
        req.Timestamp.Format("2006.01.02"))

    if err := p.indexDocument(ctx, indexName, doc); err != nil {
        return &sdk.DataCollectionResponse{
            Success:      false,
            Handled:      false,
            ErrorMessage: err.Error(),
        }, nil
    }

    return &sdk.DataCollectionResponse{
        Success: true,
        Handled: true,
    }, nil
}

func (p *ElasticsearchCollector) indexDocument(ctx context.Context,
    indexName string, doc map[string]interface{}) error {

    jsonDoc, err := json.Marshal(doc)
    if err != nil {
        return err
    }

    url := fmt.Sprintf("%s/%s/_doc", p.esURL, indexName)
    req, err := http.NewRequestWithContext(ctx, "POST", url,
        bytes.NewBuffer(jsonDoc))
    if err != nil {
        return err
    }

    req.Header.Set("Content-Type", "application/json")

    resp, err := p.client.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()

    if resp.StatusCode >= 400 {
        return fmt.Errorf("elasticsearch returned status %d", resp.StatusCode)
    }

    return nil
}

func main() {
    plugin := &ElasticsearchCollector{}
    sdk.ServePlugin(plugin)
}
```

## Plugin Context

The `PluginContext` provides contextual information about the request:

```go
type PluginContext struct {
    RequestID    string                 // Unique request ID
    LLMID        uint                   // LLM being called
    LLMSlug      string                 // LLM slug identifier
    Vendor       string                 // LLM vendor (openai, anthropic, etc.)
    AppID        uint                   // App making the request
    UserID       uint                   // User making the request
    Metadata     map[string]interface{} // Additional metadata
    TraceContext map[string]string      // Distributed tracing headers
}
```

Use this context for logging, tracing, and per-request customization.

## Testing Your Plugin

### Unit Testing

```go
func TestProcessPreAuth(t *testing.T) {
    plugin := &MyPlugin{}
    plugin.Initialize(map[string]interface{}{
        "setting": "value",
    })

    req := &sdk.PluginRequest{
        Method: "POST",
        Path:   "/v1/chat/completions",
        Body:   []byte(`{"messages": []}`),
    }

    ctx := &sdk.PluginContext{
        RequestID: "test-123",
        LLMID:     1,
    }

    resp, err := plugin.ProcessPreAuth(context.Background(), req, ctx)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }

    if !resp.Modified {
        t.Error("expected response to be modified")
    }
}
```

### Integration Testing

Use `file://` deployment to test with real LLM requests:

```bash
# Build plugin
go build -o my-plugin main.go

# Create plugin in AI Studio
curl -X POST http://localhost:3000/api/v1/plugins \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "command": "file:///full/path/to/my-plugin",
    ...
  }'

# Test LLM request
curl -X POST http://localhost:3000/api/v1/llms/1/proxy/v1/chat/completions \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"messages": [{"role": "user", "content": "test"}]}'
```

## Best Practices

### Performance

- Keep plugin logic lightweight and fast
- Use timeouts for external API calls
- Implement connection pooling for external services
- Cache frequently accessed data
- Return early for requests that don't need processing

### Error Handling

- Log errors with context (request ID, LLM ID, etc.)
- Return descriptive error messages
- Don't panic - return errors properly
- Implement graceful degradation

### Security

- Validate all configuration inputs
- Sanitize user-provided data
- Use secure connections for external services
- Don't log sensitive data (tokens, PII)
- Implement rate limiting for external calls

### Configuration

- Provide sensible defaults
- Use JSON Schema for validation
- Document all configuration options
- Support configuration updates without restart

## Troubleshooting

### Plugin Not Loading

- Check plugin command path is absolute with `file://`
- Verify plugin binary has execute permissions
- Check logs for initialization errors
- Ensure plugin implements all required interfaces

### Plugin Crashes

- Check plugin logs for panics
- Verify external service connectivity
- Test with minimal configuration
- Use defensive error handling

### Performance Issues

- Profile plugin with Go profiler
- Check for blocking operations
- Monitor external service latency
- Review resource usage (CPU, memory)

## Next Steps

- [Plugin Deployment Options]([plugins-deployment](https://docs.claude.com/en/docs/plugins-deployment))
- [SDK Reference]([plugins-sdk](https://docs.claude.com/en/docs/plugins-sdk))
- [Plugin Overview]([plugins-overview](https://docs.claude.com/en/docs/plugins-overview))
