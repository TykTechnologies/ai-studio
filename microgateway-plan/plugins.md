# Microgateway Plugin Architecture Specification

## Executive Summary

This specification outlines the design and implementation of a plugin-based middleware architecture for the Microgateway component of the Midsommar AI Studio. The system will use HashiCorp's go-plugin library (https://pkg.go.dev/github.com/hashicorp/go-plugin) to enable user-extensible middleware without requiring modifications to the core microgateway source code.

### Key Objectives
- Enable user-extensible middleware architecture
- Support vendor-scoped plugin execution
- Use gRPC for plugin communication
- Maintain long-running plugin connections with ReattachConfig support
- Provide four key hook points: pre-auth, auth, post-auth, and on-response
- Handle REST, streaming (SSE), and OpenAI API shim endpoints

## 1. System Context

### 1.1 Current Architecture

The Microgateway currently operates with the following request flow:

```
Request → CloudflareHeaders → OutboundRequest → CredentialValidator → ModelValidator → Handler
```

### 1.2 Proxy Endpoints

The system handles three types of LLM proxy endpoints:
- `/llm/rest/{slug}/*` - Standard REST request/response APIs
- `/llm/stream/{slug}/*` - Streaming response with SSE payloads
- `/ai/{slug}/*` - OpenAI API shim for compatibility

### 1.3 Existing Filter System

Filters are currently:
- Associated with LLMs through a many-to-many relationship
- Ordered for execution via `order_index`
- Managed entirely within microgateway
- Loaded with LLM configuration and passed to the proxy

## 2. Technical Requirements

### 2.1 Core Requirements
- **Communication Protocol**: gRPC (not net/rpc)
- **Connection Type**: Long-running connections
- **Process Management**: Support ReattachConfig for external process management
- **Vendor Scoping**: Plugins must be scoped to specific LLMs/vendors
- **Hook Points**: Four distinct middleware hooks
- **Response Types**: Handle both REST and streaming responses

### 2.2 Plugin Hook Points

1. **Pre-Auth**: Execute before authentication
   - Rate limiting
   - Request enrichment
   - Request validation

2. **Auth**: Replace or augment authentication
   - Custom authentication mechanisms
   - Token validation
   - Multi-factor authentication

3. **Post-Auth**: Process authenticated requests
   - Authorization checks
   - Request transformation
   - Content filtering

4. **On-Response**: Handle responses
   - Response transformation
   - Caching
   - Analytics enrichment
   - Separate handlers for REST and streaming

## 3. Database Schema

```sql
-- Plugins table
CREATE TABLE plugins (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    slug VARCHAR(255) UNIQUE NOT NULL,
    description TEXT,
    command VARCHAR(500) NOT NULL,  -- Plugin executable path/command
    checksum VARCHAR(255),           -- SHA256 for security
    config JSONB,                    -- Plugin-specific configuration
    hook_type VARCHAR(50) NOT NULL,  -- 'pre_auth', 'auth', 'post_auth', 'on_response'
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    deleted_at TIMESTAMP            -- Soft delete support
);

-- Plugin-LLM association table
CREATE TABLE llm_plugins (
    llm_id INTEGER REFERENCES llms(id) ON DELETE CASCADE,
    plugin_id INTEGER REFERENCES plugins(id) ON DELETE CASCADE,
    order_index INTEGER DEFAULT 0,
    is_active BOOLEAN DEFAULT true,
    config_override JSONB,  -- LLM-specific plugin config overrides
    created_at TIMESTAMP DEFAULT NOW(),
    PRIMARY KEY (llm_id, plugin_id)
);

-- Indexes for performance
CREATE INDEX idx_plugins_hook_type ON plugins(hook_type);
CREATE INDEX idx_plugins_is_active ON plugins(is_active);
CREATE INDEX idx_llm_plugins_llm_id ON llm_plugins(llm_id);
CREATE INDEX idx_llm_plugins_order ON llm_plugins(llm_id, order_index);
```

## 4. Plugin System Architecture

### 4.1 Directory Structure

```
microgateway/
├── plugins/
│   ├── manager.go              # Plugin lifecycle management
│   ├── registry.go             # Plugin registration and discovery
│   ├── middleware.go           # HTTP middleware integration
│   ├── reattach.go            # ReattachConfig handling
│   ├── proto/
│   │   ├── plugin.proto        # gRPC service definitions
│   │   └── plugin.pb.go        # Generated code
│   ├── interfaces/
│   │   ├── base.go            # Base plugin interface
│   │   ├── pre_auth.go        # Pre-auth plugin interface
│   │   ├── auth.go            # Auth plugin interface
│   │   ├── post_auth.go       # Post-auth plugin interface
│   │   └── response.go        # Response plugin interface
│   ├── sdk/                   # Plugin SDK for developers
│   │   ├── plugin.go          # SDK main entry
│   │   ├── grpc_client.go     # gRPC client implementation
│   │   ├── grpc_server.go     # gRPC server implementation
│   │   └── helpers.go         # Helper functions
│   └── examples/
│       ├── rate_limiter/      # Example pre-auth plugin
│       ├── jwt_auth/          # Example auth plugin
│       ├── request_transform/ # Example post-auth plugin
│       └── response_cache/    # Example response plugin
```

### 4.2 Core Components

#### 4.2.1 Plugin Manager

```go
package plugins

import (
    "sync"
    "context"
    "github.com/hashicorp/go-plugin"
)

type PluginManager struct {
    mu              sync.RWMutex
    loadedPlugins   map[uint]*LoadedPlugin     // Plugin ID -> loaded plugin
    llmPluginMap    map[uint][]uint             // LLM ID -> Plugin IDs (ordered)
    pluginClients   map[uint]*plugin.Client     // Plugin ID -> go-plugin client
    reattachConfigs map[uint]*plugin.ReattachConfig // For reconnection
    service         PluginServiceInterface      // Database service
}

type LoadedPlugin struct {
    ID          uint
    Name        string
    Slug        string
    HookType    HookType
    Client      *plugin.Client
    Instance    interface{} // Actual plugin interface
    Config      map[string]interface{}
    Checksum    string
}

// Core methods
func (pm *PluginManager) LoadPlugin(pluginID uint) (*LoadedPlugin, error)
func (pm *PluginManager) UnloadPlugin(pluginID uint) error
func (pm *PluginManager) ReloadPlugin(pluginID uint) error
func (pm *PluginManager) GetPluginsForLLM(llmID uint, hookType HookType) ([]*LoadedPlugin, error)
func (pm *PluginManager) ExecutePluginChain(llmID uint, hookType HookType, input interface{}, ctx *PluginContext) (interface{}, error)
func (pm *PluginManager) ReattachPlugin(pluginID uint, config *plugin.ReattachConfig) error
func (pm *PluginManager) SaveReattachConfig(pluginID uint) error
```

#### 4.2.2 Plugin Context

```go
package interfaces

type PluginContext struct {
    RequestID    string
    LLM          *models.LLM
    Vendor       models.Vendor
    AppID        uint
    UserID       uint
    Metadata     map[string]interface{}
    TraceContext map[string]string  // For distributed tracing
}
```

## 5. gRPC Protocol Definition

```protobuf
syntax = "proto3";

package plugin;

option go_package = "github.com/TykTechnologies/microgateway/plugins/proto";

service PluginService {
    // Lifecycle
    rpc Initialize(InitRequest) returns (InitResponse);
    rpc Ping(PingRequest) returns (PingResponse);
    rpc Shutdown(ShutdownRequest) returns (ShutdownResponse);
    
    // Pre-auth hook
    rpc ProcessPreAuth(PluginRequest) returns (PluginResponse);
    
    // Auth hook
    rpc Authenticate(AuthRequest) returns (AuthResponse);
    
    // Post-auth hook
    rpc ProcessPostAuth(EnrichedRequest) returns (PluginResponse);
    
    // Response hooks
    rpc ProcessRESTResponse(ResponseData) returns (ResponseData);
    rpc ProcessStreamChunk(stream StreamChunk) returns (stream StreamChunk);
}

message PluginContext {
    string request_id = 1;
    string vendor = 2;
    uint32 llm_id = 3;
    uint32 app_id = 4;
    uint32 user_id = 5;
    map<string, string> metadata = 6;
    map<string, string> trace_context = 7;
}

message PluginRequest {
    string method = 1;
    string path = 2;
    map<string, string> headers = 3;
    bytes body = 4;
    PluginContext context = 5;
    string remote_addr = 6;
}

message PluginResponse {
    bool modified = 1;
    int32 status_code = 2;
    map<string, string> headers = 3;
    bytes body = 4;
    bool block = 5;  // Stop processing if true
    string error_message = 6;
}

message AuthRequest {
    string credential = 1;
    string auth_type = 2;
    PluginRequest request = 3;
}

message AuthResponse {
    bool authenticated = 1;
    string user_id = 2;
    string app_id = 3;
    map<string, string> claims = 4;
    string error_message = 5;
}

message ResponseData {
    string request_id = 1;
    int32 status_code = 2;
    map<string, string> headers = 3;
    bytes body = 4;
    PluginContext context = 5;
    int64 latency_ms = 6;
}

message StreamChunk {
    string request_id = 1;
    bytes data = 2;
    bool is_final = 3;
    PluginContext context = 4;
    int32 sequence = 5;
}
```

## 6. Plugin Interfaces

### 6.1 Base Plugin Interface

```go
package interfaces

import "context"

type HookType string

const (
    HookTypePreAuth    HookType = "pre_auth"
    HookTypeAuth       HookType = "auth"
    HookTypePostAuth   HookType = "post_auth"
    HookTypeOnResponse HookType = "on_response"
)

type BasePlugin interface {
    Initialize(config map[string]interface{}) error
    GetHookType() HookType
    GetName() string
    GetVersion() string
    Shutdown() error
}
```

### 6.2 Hook-Specific Interfaces

```go
// Pre-Auth Plugin
type PreAuthPlugin interface {
    BasePlugin
    ProcessRequest(ctx context.Context, req *PluginRequest, pluginCtx *PluginContext) (*PluginResponse, error)
}

// Auth Plugin
type AuthPlugin interface {
    BasePlugin
    Authenticate(ctx context.Context, req *AuthRequest, pluginCtx *PluginContext) (*AuthResponse, error)
    ValidateToken(ctx context.Context, token string, pluginCtx *PluginContext) (*AuthResponse, error)
}

// Post-Auth Plugin
type PostAuthPlugin interface {
    BasePlugin
    ProcessRequest(ctx context.Context, req *EnrichedRequest, pluginCtx *PluginContext) (*PluginResponse, error)
}

// Response Plugin
type ResponsePlugin interface {
    BasePlugin
    ProcessRESTResponse(ctx context.Context, resp *ResponseData, pluginCtx *PluginContext) (*ResponseData, error)
    ProcessStreamChunk(ctx context.Context, chunk *StreamChunk, pluginCtx *PluginContext) (*StreamChunk, error)
}
```

## 7. Integration with Existing Middleware

### 7.1 Modified Proxy Handler Creation

```go
func (p *Proxy) createHandler() http.Handler {
    r := mux.NewRouter()
    
    // Existing routes
    r.HandleFunc("/.well-known/oauth-protected-resource", p.handleOAuthProtectedResourceMetadata)
    r.HandleFunc("/llm/rest/{llmSlug}/{rest:.*}", p.handleLLMRequest)
    r.HandleFunc("/llm/stream/{llmSlug}/{rest:.*}", p.handleStreamingLLMRequest)
    // ... other routes ...
    
    // Enhanced middleware chain with plugins
    return p.cloudflareHeadersMiddleware(
        p.preAuthPluginMiddleware(              // NEW: Pre-auth plugins
            p.outboundRequestMiddleware(
                p.authPluginOrCredValidator(    // NEW: Auth plugins or default
                    p.postAuthPluginMiddleware( // NEW: Post-auth plugins
                        p.modelValidationMiddleware(
                            p.responsePluginMiddleware( // NEW: Response plugins
                                r)))))))
}
```

### 7.2 Plugin Middleware Implementation

```go
func (p *Proxy) preAuthPluginMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        llmSlug := mux.Vars(r)["llmSlug"]
        if llmSlug == "" {
            next.ServeHTTP(w, r)
            return
        }
        
        llm, exists := p.llms[llmSlug]
        if !exists {
            next.ServeHTTP(w, r)
            return
        }
        
        pluginCtx := &PluginContext{
            RequestID: r.Header.Get("X-Request-ID"),
            LLM:       llm,
            Vendor:    llm.Vendor,
        }
        
        // Execute pre-auth plugin chain
        req := convertHTTPToPluginRequest(r)
        result, err := p.pluginManager.ExecutePluginChain(
            llm.ID,
            HookTypePreAuth,
            req,
            pluginCtx,
        )
        
        if err != nil {
            respondWithError(w, http.StatusInternalServerError, "plugin error", err, false)
            return
        }
        
        if resp, ok := result.(*PluginResponse); ok && resp.Block {
            w.WriteHeader(resp.StatusCode)
            w.Write(resp.Body)
            return
        }
        
        // Update request with plugin modifications
        r = updateRequestFromPlugin(r, result)
        next.ServeHTTP(w, r)
    })
}
```

## 8. Plugin SDK

### 8.1 SDK Structure

```go
// sdk/plugin.go
package sdk

import (
    "github.com/hashicorp/go-plugin"
    "google.golang.org/grpc"
)

var HandshakeConfig = plugin.HandshakeConfig{
    ProtocolVersion:  1,
    MagicCookieKey:   "MICROGATEWAY_PLUGIN",
    MagicCookieValue: "v1",
}

// Helper function to serve a plugin
func ServePlugin(impl interface{}) {
    var pluginMap = map[string]plugin.Plugin{}
    
    switch p := impl.(type) {
    case PreAuthPlugin:
        pluginMap["plugin"] = &PreAuthPluginGRPC{Impl: p}
    case AuthPlugin:
        pluginMap["plugin"] = &AuthPluginGRPC{Impl: p}
    case PostAuthPlugin:
        pluginMap["plugin"] = &PostAuthPluginGRPC{Impl: p}
    case ResponsePlugin:
        pluginMap["plugin"] = &ResponsePluginGRPC{Impl: p}
    }
    
    plugin.Serve(&plugin.ServeConfig{
        HandshakeConfig: HandshakeConfig,
        Plugins:         pluginMap,
        GRPCServer:      plugin.DefaultGRPCServer,
    })
}
```

## 9. Service Layer

### 9.1 Plugin Service Interface

```go
package services

type PluginServiceInterface interface {
    // CRUD operations
    CreatePlugin(req *CreatePluginRequest) (*database.Plugin, error)
    GetPlugin(id uint) (*database.Plugin, error)
    ListPlugins(page, limit int, hookType string, isActive bool) ([]database.Plugin, int64, error)
    UpdatePlugin(id uint, req *UpdatePluginRequest) (*database.Plugin, error)
    DeletePlugin(id uint) error
    
    // LLM associations
    GetPluginsForLLM(llmID uint) ([]database.Plugin, error)
    UpdateLLMPlugins(llmID uint, pluginIDs []uint) error
    GetLLMPluginConfig(llmID, pluginID uint) (map[string]interface{}, error)
    
    // Validation
    ValidatePluginChecksum(pluginID uint, filePath string) error
    TestPlugin(pluginID uint, testData interface{}) (interface{}, error)
}

type CreatePluginRequest struct {
    Name        string                 `json:"name" binding:"required"`
    Slug        string                 `json:"slug" binding:"required"`
    Description string                 `json:"description"`
    Command     string                 `json:"command" binding:"required"`
    Checksum    string                 `json:"checksum"`
    Config      map[string]interface{} `json:"config"`
    HookType    string                 `json:"hook_type" binding:"required"`
    IsActive    bool                   `json:"is_active"`
}
```

## 10. CLI Commands

```bash
# Plugin management
mgw plugin create --name "rate-limiter" --command "./plugins/rate_limiter" --hook pre_auth --config-file config.yaml
mgw plugin list --hook pre_auth --active
mgw plugin show <plugin-id>
mgw plugin update <plugin-id> --config '{"requests_per_minute": 100}'
mgw plugin delete <plugin-id>
mgw plugin test <plugin-id> --test-file test.json

# LLM-Plugin associations
mgw llm plugin list <llm-id>
mgw llm plugin add <llm-id> <plugin-id> --order 1 --config-override '{"specific": "value"}'
mgw llm plugin remove <llm-id> <plugin-id>
mgw llm plugin reorder <llm-id> --plugins "3,1,2"

# Plugin debugging
mgw plugin logs <plugin-id> --tail 100
mgw plugin restart <plugin-id>
mgw plugin status <plugin-id>
```

## 11. Example Plugin Implementation

### 11.1 Rate Limiter Plugin (Pre-Auth)

```go
package main

import (
    "context"
    "fmt"
    "sync"
    "time"
    
    "github.com/TykTechnologies/microgateway/plugins/sdk"
)

type RateLimiterPlugin struct {
    mu      sync.RWMutex
    config  Config
    buckets map[string]*TokenBucket
}

type Config struct {
    RequestsPerMinute int    `json:"requests_per_minute"`
    BurstSize        int    `json:"burst_size"`
    KeyExtractor     string `json:"key_extractor"` // "ip", "app", "user"
}

func (p *RateLimiterPlugin) Initialize(config map[string]interface{}) error {
    // Parse configuration
    p.config = parseConfig(config)
    p.buckets = make(map[string]*TokenBucket)
    
    // Start cleanup goroutine
    go p.cleanupExpiredBuckets()
    
    return nil
}

func (p *RateLimiterPlugin) ProcessRequest(
    ctx context.Context,
    req *sdk.PluginRequest,
    pluginCtx *sdk.PluginContext,
) (*sdk.PluginResponse, error) {
    // Extract rate limit key based on configuration
    key := p.extractKey(req, pluginCtx)
    
    // Get or create token bucket
    bucket := p.getBucket(key)
    
    // Check rate limit
    if !bucket.Allow() {
        return &sdk.PluginResponse{
            Modified:   true,
            StatusCode: 429,
            Headers: map[string]string{
                "X-RateLimit-Limit":     fmt.Sprintf("%d", p.config.RequestsPerMinute),
                "X-RateLimit-Remaining": "0",
                "X-RateLimit-Reset":     fmt.Sprintf("%d", bucket.ResetTime().Unix()),
                "Retry-After":           fmt.Sprintf("%d", bucket.RetryAfter()),
            },
            Body: []byte(`{"error": "rate limit exceeded"}`),
            Block: true,
        }, nil
    }
    
    // Add rate limit headers to response
    return &sdk.PluginResponse{
        Modified: true,
        Headers: map[string]string{
            "X-RateLimit-Limit":     fmt.Sprintf("%d", p.config.RequestsPerMinute),
            "X-RateLimit-Remaining": fmt.Sprintf("%d", bucket.Remaining()),
        },
    }, nil
}

func (p *RateLimiterPlugin) extractKey(req *sdk.PluginRequest, ctx *sdk.PluginContext) string {
    switch p.config.KeyExtractor {
    case "app":
        return fmt.Sprintf("app:%d:%d", ctx.LLM.ID, ctx.AppID)
    case "user":
        return fmt.Sprintf("user:%d:%d", ctx.LLM.ID, ctx.UserID)
    default: // "ip"
        return fmt.Sprintf("ip:%d:%s", ctx.LLM.ID, req.RemoteAddr)
    }
}

func (p *RateLimiterPlugin) GetHookType() sdk.HookType {
    return sdk.HookTypePreAuth
}

func (p *RateLimiterPlugin) GetName() string {
    return "rate-limiter"
}

func (p *RateLimiterPlugin) GetVersion() string {
    return "1.0.0"
}

func (p *RateLimiterPlugin) Shutdown() error {
    // Cleanup resources
    return nil
}

func main() {
    sdk.ServePlugin(&RateLimiterPlugin{})
}
```

## 12. Configuration

### 12.1 Plugin Configuration File

```yaml
# plugins.yaml
plugins:
  enabled: true
  plugin_dir: "./plugins"
  max_plugin_memory: "100MB"
  plugin_timeout: "30s"
  
  security:
    verify_checksum: true
    allow_network: false
    sandbox_enabled: true
  
  grpc:
    max_message_size: "4MB"
    keepalive_time: "30s"
    keepalive_timeout: "10s"
```

### 12.2 Environment Variables

```bash
# Plugin system configuration
PLUGIN_ENABLED=true
PLUGIN_DIR=/opt/microgateway/plugins
PLUGIN_LOG_LEVEL=info
PLUGIN_GRPC_PORT_MIN=10000
PLUGIN_GRPC_PORT_MAX=10100
PLUGIN_REATTACH_TIMEOUT=60s
```

## 13. Testing Strategy

### 13.1 Unit Tests

```go
// plugins/manager_test.go
func TestPluginManager_LoadPlugin(t *testing.T) {
    // Test plugin loading
}

func TestPluginManager_ExecutePluginChain(t *testing.T) {
    // Test plugin chain execution
}

func TestPluginManager_ReattachPlugin(t *testing.T) {
    // Test plugin reattachment
}
```

### 13.2 Integration Tests

```go
// plugins/integration_test.go
func TestPluginIntegration_PreAuth(t *testing.T) {
    // Test pre-auth plugin integration
}

func TestPluginIntegration_Streaming(t *testing.T) {
    // Test streaming response handling
}
```

### 13.3 Plugin Test Framework

```go
// sdk/testing/framework.go
package testing

type PluginTestHarness struct {
    plugin   interface{}
    server   *grpc.Server
    client   *grpc.ClientConn
}

func NewTestHarness(plugin interface{}) *PluginTestHarness {
    // Create test harness for plugin testing
}

func (h *PluginTestHarness) TestRequest(req *PluginRequest) (*PluginResponse, error) {
    // Test plugin with request
}
```

## 14. Monitoring and Observability

### 14.1 Metrics

```go
// plugins/metrics.go
var (
    pluginExecutionDuration = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name: "plugin_execution_duration_seconds",
            Help: "Plugin execution duration in seconds",
        },
        []string{"plugin", "hook_type", "llm"},
    )
    
    pluginErrors = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "plugin_errors_total",
            Help: "Total number of plugin errors",
        },
        []string{"plugin", "hook_type", "error_type"},
    )
)
```

### 14.2 Logging

```go
// Structured logging for plugins
log.Info().
    Str("plugin", plugin.Name).
    Str("hook_type", string(plugin.HookType)).
    Uint("llm_id", llmID).
    Dur("duration", duration).
    Msg("Plugin executed successfully")
```

## 15. Security Considerations

### 15.1 Plugin Verification
- SHA256 checksum verification before loading
- Digital signature support (future)
- Sandbox execution environment

### 15.2 Resource Limits
- Memory limits per plugin
- CPU limits via cgroups
- Network isolation options

### 15.3 Authentication
- mTLS between gateway and plugins
- Plugin-specific credentials
- Audit logging for all plugin operations

## 16. Implementation Phases

### Phase 1: Core Infrastructure (Week 1-2)
- [ ] Implement plugin database schema
- [ ] Create plugin service layer
- [ ] Build plugin manager
- [ ] Define gRPC protocol
- [ ] Create base plugin interfaces

### Phase 2: Plugin Lifecycle (Week 3)
- [ ] Implement plugin loading/unloading
- [ ] Add ReattachConfig support
- [ ] Create plugin registry
- [ ] Build health checking

### Phase 3: Middleware Integration (Week 4)
- [ ] Integrate pre-auth middleware
- [ ] Add auth plugin support
- [ ] Implement post-auth middleware
- [ ] Add response handling

### Phase 4: SDK and Examples (Week 5)
- [ ] Create plugin SDK
- [ ] Build example plugins
- [ ] Write plugin developer guide
- [ ] Create testing framework

### Phase 5: Management and Operations (Week 6)
- [ ] Add CLI commands
- [ ] Implement monitoring
- [ ] Add admin API endpoints
- [ ] Create deployment guides

### Phase 6: Testing and Documentation (Week 7-8)
- [ ] Comprehensive testing
- [ ] Performance testing
- [ ] Security audit
- [ ] Complete documentation

## 17. Future Enhancements

### 17.1 Advanced Features (Post-Launch)
- WebAssembly plugin support
- Hot-reload without process restart
- Plugin marketplace integration
- Visual plugin builder

### 17.2 Additional Hook Points
- `OnAnalyticsRecord` - Intercept analytics
- `OnProxyLog` - Modify proxy logs  
- `OnBudgetCheck` - Custom budget validation
- `OnError` - Error handling customization

## 18. Migration Strategy

### 18.1 Backward Compatibility
- Existing middleware continues to work
- Plugins augment, not replace initially
- Gradual migration path provided

### 18.2 Migration Steps
1. Deploy plugin infrastructure (no breaking changes)
2. Allow plugins to run alongside existing middleware
3. Provide migration guides for common patterns
4. Enable plugin-only mode (opt-in)
5. Deprecate old middleware (long-term)

## 19. Performance Considerations

### 19.1 Optimization Strategies
- Plugin result caching
- Connection pooling for gRPC
- Lazy loading of plugins
- Parallel plugin execution where possible

### 19.2 Benchmarks
- Target: <4ms overhead per plugin
- Support 100+ concurrent plugin executions
- Handle 10,000+ requests/second with plugins

## 20. Success Criteria

### 20.1 Functional Requirements
- ✓ All four hook points operational
- ✓ Vendor-scoped plugin execution
- ✓ ReattachConfig support
- ✓ REST and streaming response handling

### 20.2 Non-Functional Requirements  
- ✓ low latency overhead
- ✓ 99.99% availability
- ✓ Zero-downtime plugin updates
- ✓ Comprehensive monitoring

