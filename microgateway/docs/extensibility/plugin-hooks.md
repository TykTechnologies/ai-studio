# Plugin Hooks and Interfaces

The microgateway plugin system provides multiple hook points throughout the request lifecycle, allowing plugins to intercept and modify requests, responses, and data processing.

## Overview

Plugin hook system features:
- **Multiple Hook Types**: Various integration points in request processing
- **Ordered Execution**: Priority-based plugin execution order
- **Request Modification**: Ability to modify requests and responses
- **Flow Control**: Continue, abort, or redirect request processing
- **Context Propagation**: Rich context information for plugins
- **Error Handling**: Graceful handling of plugin failures

## Available Hook Types

### Request/Response Hooks

#### pre_auth
Executes before authentication processing:
- **Purpose**: Request validation, logging, rate limiting
- **Can Modify**: Request headers, body, metadata
- **Can Abort**: Yes, with custom error response
- **Use Cases**: Custom rate limiting, request filtering, audit logging

```go
type PreAuthPlugin interface {
    BasePlugin
    ProcessPreAuth(ctx context.Context, req *RequestData, pluginCtx *PluginContext) (*PluginResponse, error)
}
```

#### auth
Custom authentication logic:
- **Purpose**: Alternative authentication mechanisms
- **Can Modify**: Request context, user information
- **Can Abort**: Yes, with authentication failure
- **Use Cases**: SSO integration, custom token validation, multi-factor auth

```go
type AuthPlugin interface {
    BasePlugin
    ProcessAuth(ctx context.Context, req *RequestData, pluginCtx *PluginContext) (*AuthResponse, error)
}
```

#### post_auth
Executes after successful authentication:
- **Purpose**: Post-authentication processing
- **Can Modify**: Request headers, body, routing
- **Can Abort**: Yes, with authorization failure
- **Use Cases**: Authorization checks, request enrichment, routing decisions

```go
type PostAuthPlugin interface {
    BasePlugin
    ProcessPostAuth(ctx context.Context, req *RequestData, pluginCtx *PluginContext) (*PluginResponse, error)
}
```

#### on_response
Processes responses before returning to client:
- **Purpose**: Response modification, logging, caching
- **Can Modify**: Response headers, body, status code
- **Can Abort**: No (response already generated)
- **Use Cases**: Response filtering, caching, metrics collection

```go
type ResponsePlugin interface {
    BasePlugin
    ProcessResponse(ctx context.Context, resp *ResponseData, pluginCtx *PluginContext) (*PluginResponse, error)
}
```

### Data Collection Hooks

#### data_collection
Handles data storage and analytics:
- **Purpose**: Custom data backends, external analytics
- **Can Modify**: Data processing behavior
- **Can Abort**: No (doesn't affect request flow)
- **Use Cases**: External analytics, custom storage, data streaming

```go
type DataCollectionPlugin interface {
    BasePlugin
    HandleProxyLog(ctx context.Context, req *ProxyLogData, pluginCtx *PluginContext) (*DataCollectionResponse, error)
    HandleAnalytics(ctx context.Context, req *AnalyticsData, pluginCtx *PluginContext) (*DataCollectionResponse, error)
    HandleBudgetUsage(ctx context.Context, req *BudgetUsageData, pluginCtx *PluginContext) (*DataCollectionResponse, error)
}
```

## Plugin Interfaces

### Base Plugin Interface
All plugins must implement the base interface:

```go
type BasePlugin interface {
    Initialize(config map[string]interface{}) error
    GetHookType() HookType
    Health() error
    GetInfo() PluginInfo
}
```

### Plugin Response Types

#### Standard Plugin Response
```go
type PluginResponse struct {
    Continue bool                 // Continue request processing
    Modified bool                 // Request was modified
    AbortWithError *ErrorResponse // Abort with custom error
    Headers map[string]string     // Additional headers to set
    Metadata map[string]interface{} // Additional metadata
}
```

#### Authentication Response
```go
type AuthResponse struct {
    Authenticated bool           // Authentication successful
    UserID string               // Authenticated user ID
    UserInfo map[string]interface{} // User information
    Headers map[string]string    // Headers to add
    AbortWithError *ErrorResponse // Authentication failure details
}
```

#### Data Collection Response
```go
type DataCollectionResponse struct {
    Success bool    // Data processing successful
    Handled bool    // Plugin handled the data
    Error string    // Error message if failed
}
```

## Plugin Context

### Context Information
Plugins receive rich context about the current request:

```go
type PluginContext struct {
    RequestID    string                 // Unique request identifier
    AppID        uint                   // Application ID
    LLMID        uint                   // LLM provider ID
    CredentialID uint                   // Credential ID used
    UserAgent    string                 // Client user agent
    ClientIP     string                 // Client IP address
    Headers      map[string]string      // HTTP headers
    Metadata     map[string]interface{} // Additional metadata
    Timestamp    time.Time              // Request timestamp
}
```

### Request Data
```go
type RequestData struct {
    Method   string                 // HTTP method
    Path     string                 // Request path
    Headers  map[string]string      // HTTP headers
    Body     []byte                 // Request body
    Query    map[string]string      // Query parameters
    Metadata map[string]interface{} // Request metadata
}
```

### Response Data
```go
type ResponseData struct {
    StatusCode int                    // HTTP status code
    Headers    map[string]string      // Response headers
    Body       []byte                 // Response body
    Latency    time.Duration          // Request processing time
    TokensUsed int                    // Tokens consumed
    Cost       float64                // Request cost
    Error      error                  // Error if request failed
}
```

## Hook Execution Order

### Request Processing Flow
```
HTTP Request
    ↓
pre_auth plugins (priority order)
    ↓
Authentication
    ↓
auth plugins (if custom auth enabled)
    ↓
post_auth plugins (priority order)
    ↓
LLM Request Processing
    ↓
LLM Response
    ↓
on_response plugins (priority order)
    ↓
data_collection plugins (async)
    ↓
HTTP Response
```

### Plugin Priority
```yaml
# Lower priority numbers execute first
plugins:
  - name: "rate-limiter"
    priority: 10
    hook_types: ["pre_auth"]
    
  - name: "auth-validator"
    priority: 20
    hook_types: ["pre_auth"]
    
  - name: "request-enricher"
    priority: 30
    hook_types: ["post_auth"]
```

## Plugin Examples

### Pre-Auth Plugin Example
```go
package main

import (
    "context"
    "fmt"
    "github.com/TykTechnologies/midsommar/microgateway/plugins/sdk"
)

type RateLimiterPlugin struct {
    maxRequests int
    window      time.Duration
}

func (p *RateLimiterPlugin) ProcessPreAuth(ctx context.Context, req *sdk.RequestData, pluginCtx *sdk.PluginContext) (*sdk.PluginResponse, error) {
    // Check rate limit for client IP
    if p.isRateLimited(pluginCtx.ClientIP) {
        return &sdk.PluginResponse{
            Continue: false,
            AbortWithError: &sdk.ErrorResponse{
                StatusCode: 429,
                Message:    "Rate limit exceeded",
            },
        }, nil
    }
    
    return &sdk.PluginResponse{Continue: true}, nil
}

func (p *RateLimiterPlugin) isRateLimited(clientIP string) bool {
    // Rate limiting logic
    return false
}
```

### Auth Plugin Example
```go
package main

import (
    "context"
    "encoding/json"
    "net/http"
    "github.com/TykTechnologies/midsommar/microgateway/plugins/sdk"
)

type SSOAuthPlugin struct {
    ssoEndpoint string
    timeout     time.Duration
}

func (p *SSOAuthPlugin) ProcessAuth(ctx context.Context, req *sdk.RequestData, pluginCtx *sdk.PluginContext) (*sdk.AuthResponse, error) {
    // Extract token from request
    token := req.Headers["Authorization"]
    if token == "" {
        return &sdk.AuthResponse{
            Authenticated: false,
            AbortWithError: &sdk.ErrorResponse{
                StatusCode: 401,
                Message:    "Missing authorization header",
            },
        }, nil
    }
    
    // Validate token against SSO service
    userInfo, err := p.validateToken(token)
    if err != nil {
        return &sdk.AuthResponse{
            Authenticated: false,
            AbortWithError: &sdk.ErrorResponse{
                StatusCode: 401,
                Message:    "Invalid token",
            },
        }, nil
    }
    
    return &sdk.AuthResponse{
        Authenticated: true,
        UserID:        userInfo.ID,
        UserInfo:      userInfo.Attributes,
    }, nil
}
```

### Response Plugin Example
```go
package main

import (
    "context"
    "encoding/json"
    "github.com/TykTechnologies/midsommar/microgateway/plugins/sdk"
)

type ResponseFilterPlugin struct {
    filterPatterns []string
}

func (p *ResponseFilterPlugin) ProcessResponse(ctx context.Context, resp *sdk.ResponseData, pluginCtx *sdk.PluginContext) (*sdk.PluginResponse, error) {
    // Filter sensitive content from response
    if p.containsSensitiveData(resp.Body) {
        filteredBody := p.filterContent(resp.Body)
        return &sdk.PluginResponse{
            Continue: true,
            Modified: true,
            ResponseBody: filteredBody,
        }, nil
    }
    
    return &sdk.PluginResponse{Continue: true}, nil
}
```

### Data Collection Plugin Example
```go
package main

import (
    "context"
    "github.com/TykTechnologies/midsommar/microgateway/plugins/sdk"
)

type ElasticsearchCollector struct {
    client *elasticsearch.Client
    index  string
}

func (p *ElasticsearchCollector) HandleAnalytics(ctx context.Context, req *sdk.AnalyticsData, pluginCtx *sdk.PluginContext) (*sdk.DataCollectionResponse, error) {
    // Send analytics data to Elasticsearch
    doc := map[string]interface{}{
        "app_id":       req.AppID,
        "llm_id":       req.LLMID,
        "tokens_used":  req.TokensUsed,
        "cost":         req.Cost,
        "latency_ms":   req.LatencyMS,
        "timestamp":    req.Timestamp,
    }
    
    err := p.indexDocument(doc)
    if err != nil {
        return &sdk.DataCollectionResponse{
            Success: false,
            Error:   err.Error(),
        }, nil
    }
    
    return &sdk.DataCollectionResponse{
        Success: true,
        Handled: true,
    }, nil
}
```

## Plugin Configuration

### Hook-Specific Configuration
```yaml
plugins:
  - name: "multi-hook-plugin"
    path: "./plugins/multi_hook"
    enabled: true
    hook_types: ["pre_auth", "post_auth", "on_response"]
    priority: 100
    config:
      pre_auth:
        rate_limit: 100
        window: "1m"
      post_auth:
        enrich_request: true
        add_headers: ["X-User-ID", "X-App-ID"]
      on_response:
        filter_enabled: true
        filter_patterns: ["password", "secret"]
```

### Environment Variable Expansion
```yaml
plugins:
  - name: "external-service-plugin"
    config:
      service_url: "${EXTERNAL_SERVICE_URL}"
      api_key: "${EXTERNAL_SERVICE_API_KEY}"
      timeout: "${EXTERNAL_SERVICE_TIMEOUT:-30s}"
```

## Error Handling

### Plugin Error Responses
```go
// Plugin can abort request with custom error
return &sdk.PluginResponse{
    Continue: false,
    AbortWithError: &sdk.ErrorResponse{
        StatusCode: 403,
        Message:    "Access denied by security policy",
        Headers: map[string]string{
            "X-Error-Code": "SEC001",
        },
    },
}, nil
```

### Error Propagation
- Plugin errors are logged but don't crash the service
- Failed plugins can be configured to fail-open or fail-closed
- Request processing continues unless plugin explicitly aborts
- Plugin health monitoring with automatic restart on failure

### Timeout Handling
```bash
# Configure plugin timeouts
PLUGINS_TIMEOUT=30s
PLUGINS_HEALTH_CHECK_INTERVAL=60s
PLUGINS_MAX_FAILURES=3
```

## Plugin Development Best Practices

### Interface Implementation
- Implement all required interface methods
- Handle context cancellation properly
- Return appropriate error responses
- Use structured logging for debugging

### Performance Considerations
- Keep plugin processing lightweight
- Use asynchronous processing for heavy operations
- Implement proper caching where appropriate
- Monitor resource usage

### Error Handling
- Graceful degradation on failures
- Meaningful error messages
- Proper logging for debugging
- Health check implementation

### Testing
- Unit tests for plugin logic
- Integration tests with mock gateway
- Load testing for performance validation
- Error scenario testing

## Plugin Debugging

### Debug Configuration
```yaml
plugins:
  - name: "debug-plugin"
    path: "./plugins/debug_plugin"
    enabled: true
    debug: true
    config:
      log_level: "debug"
      trace_requests: true
```

### Debugging Tools
```bash
# Enable plugin debugging
PLUGINS_DEBUG=true
LOG_LEVEL=debug

# Monitor plugin execution
tail -f /var/log/microgateway/plugins.log

# Test plugin standalone
./plugins/my_plugin --test-mode

# Plugin performance profiling
mgw system metrics | grep plugin_execution_duration
```

## Advanced Plugin Patterns

### Plugin Chaining
Multiple plugins of the same hook type execute in priority order:

```yaml
plugins:
  - name: "validate-request"
    priority: 10
    hook_types: ["pre_auth"]
    
  - name: "rate-limiter"
    priority: 20
    hook_types: ["pre_auth"]
    
  - name: "audit-logger"
    priority: 30
    hook_types: ["pre_auth"]
```

### Conditional Plugin Execution
```go
func (p *MyPlugin) ProcessPreAuth(ctx context.Context, req *sdk.RequestData, pluginCtx *sdk.PluginContext) (*sdk.PluginResponse, error) {
    // Only process certain applications
    if pluginCtx.AppID != p.targetAppID {
        return &sdk.PluginResponse{Continue: true}, nil
    }
    
    // Plugin logic here
    return p.processRequest(req, pluginCtx)
}
```

### Cross-Plugin Communication
```go
// Use plugin context metadata for communication
func (p *Plugin1) ProcessPreAuth(ctx context.Context, req *sdk.RequestData, pluginCtx *sdk.PluginContext) (*sdk.PluginResponse, error) {
    // Set metadata for other plugins
    return &sdk.PluginResponse{
        Continue: true,
        Metadata: map[string]interface{}{
            "plugin1_processed": true,
            "user_tier": "premium",
        },
    }, nil
}

func (p *Plugin2) ProcessPostAuth(ctx context.Context, req *sdk.RequestData, pluginCtx *sdk.PluginContext) (*sdk.PluginResponse, error) {
    // Read metadata from previous plugin
    if tier, ok := pluginCtx.Metadata["user_tier"].(string); ok {
        if tier == "premium" {
            // Apply premium user logic
        }
    }
    
    return &sdk.PluginResponse{Continue: true}, nil
}
```

## Plugin SDK Reference

### SDK Components
```go
package sdk

// Core interfaces
type BasePlugin interface { ... }
type PreAuthPlugin interface { ... }
type AuthPlugin interface { ... }
type PostAuthPlugin interface { ... }
type ResponsePlugin interface { ... }
type DataCollectionPlugin interface { ... }

// Data structures
type RequestData struct { ... }
type ResponseData struct { ... }
type PluginContext struct { ... }
type PluginResponse struct { ... }

// Helper functions
func ServePlugin(plugin BasePlugin)
func NewLogger(name string) Logger
func ParseConfig(config map[string]interface{}, target interface{}) error
```

### SDK Utilities
```go
// Configuration helpers
config := sdk.NewConfig()
config.Parse(pluginConfig)

// Logging utilities
logger := sdk.NewLogger("my-plugin")
logger.Info("Plugin initialized")

// HTTP utilities
client := sdk.NewHTTPClient(timeout)
resp, err := client.Get(url)

// Metrics helpers
counter := sdk.NewCounter("plugin_requests_total")
counter.Inc()
```

## Hook-Specific Examples

### Rate Limiting Hook
```go
type RateLimitPlugin struct {
    limiter map[string]*rate.Limiter
    mutex   sync.RWMutex
}

func (p *RateLimitPlugin) ProcessPreAuth(ctx context.Context, req *sdk.RequestData, pluginCtx *sdk.PluginContext) (*sdk.PluginResponse, error) {
    limiter := p.getLimiter(pluginCtx.ClientIP)
    
    if !limiter.Allow() {
        return &sdk.PluginResponse{
            Continue: false,
            AbortWithError: &sdk.ErrorResponse{
                StatusCode: 429,
                Message:    "Rate limit exceeded",
                Headers: map[string]string{
                    "Retry-After": "60",
                },
            },
        }, nil
    }
    
    return &sdk.PluginResponse{Continue: true}, nil
}
```

### Request Enrichment Hook
```go
type EnrichmentPlugin struct {
    userService UserService
}

func (p *EnrichmentPlugin) ProcessPostAuth(ctx context.Context, req *sdk.RequestData, pluginCtx *sdk.PluginContext) (*sdk.PluginResponse, error) {
    // Enrich request with user information
    userInfo, err := p.userService.GetUser(pluginCtx.UserID)
    if err != nil {
        return &sdk.PluginResponse{Continue: true}, nil // Continue on failure
    }
    
    // Add user information to request headers
    headers := map[string]string{
        "X-User-Tier":   userInfo.Tier,
        "X-User-Region": userInfo.Region,
    }
    
    return &sdk.PluginResponse{
        Continue: true,
        Modified: true,
        Headers:  headers,
    }, nil
}
```

### Response Processing Hook
```go
type CachePlugin struct {
    cache Cache
}

func (p *CachePlugin) ProcessResponse(ctx context.Context, resp *sdk.ResponseData, pluginCtx *sdk.PluginContext) (*sdk.PluginResponse, error) {
    // Cache successful responses
    if resp.StatusCode == 200 {
        cacheKey := p.generateCacheKey(pluginCtx.RequestID)
        p.cache.Set(cacheKey, resp.Body, time.Hour)
    }
    
    return &sdk.PluginResponse{Continue: true}, nil
}
```

## Plugin Testing

### Unit Testing
```go
func TestMyPlugin(t *testing.T) {
    plugin := &MyPlugin{}
    err := plugin.Initialize(map[string]interface{}{
        "test_mode": true,
    })
    assert.NoError(t, err)
    
    req := &sdk.RequestData{
        Method: "POST",
        Path:   "/test",
    }
    
    resp, err := plugin.ProcessPreAuth(context.Background(), req, &sdk.PluginContext{})
    assert.NoError(t, err)
    assert.True(t, resp.Continue)
}
```

### Integration Testing
```bash
# Test plugin with microgateway
PLUGINS_CONFIG_PATH=./test/plugins.yaml \
LOG_LEVEL=debug \
./dist/microgateway &

# Make test request
curl -X POST http://localhost:8080/llm/rest/test/chat/completions \
  -H "Authorization: Bearer test-token" \
  -d '{"test": true}'

# Check plugin execution
grep "plugin execution" /var/log/microgateway/microgateway.log
```

---

Plugin hooks provide flexible integration points throughout request processing. For plugin development, see [Plugin System](plugin-system.md). For data collection plugins, see [Data Plugins](data-plugins.md).
