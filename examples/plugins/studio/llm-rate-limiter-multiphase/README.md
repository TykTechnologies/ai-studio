# LLM Rate Limiter (Multi-Phase) Plugin

**Multi-capability plugin demonstrating advanced patterns**: PostAuth + Response + UI Provider

This example showcases how to build a sophisticated rate limiting system that:
- Checks limits before proxying requests (PostAuth)
- Updates counters after successful responses (Response)
- Provides a custom dashboard for monitoring and configuration (UI Provider)

## Features

### 1. PostAuth Hook - Pre-Request Validation
- Check current request count against configured limits
- Block requests that exceed rate limits
- Per-app and per-user rate limiting
- Read limit configuration from KV storage

### 2. Response Hook - Post-Request Accounting
- Increment request counters after successful responses
- Track token usage and costs
- Update KV storage with latest counts
- Reset counters based on time windows

### 3. UI Provider - Dashboard
- WebComponent-based dashboard
- Real-time rate limit status display
- Configure limits per app/user
- View historical usage statistics
- Custom RPC methods for data fetching

## Architecture

### Multi-Capability Pattern

```go
type RateLimiterPlugin struct {
    plugin_sdk.BasePlugin
    limits sync.Map // app_id -> limit config
    counts sync.Map // app_id -> current count
}

// PostAuthHandler capability
func (p *RateLimiterPlugin) HandlePostAuth(ctx plugin_sdk.Context, req *pb.EnrichedRequest) (*pb.PluginResponse, error) {
    // Check if limit exceeded
}

// ResponseHandler capability
func (p *RateLimiterPlugin) OnBeforeWrite(ctx plugin_sdk.Context, req *pb.ResponseWriteRequest) (*pb.ResponseWriteResponse, error) {
    // Update counters
}

// UIProvider capability
func (p *RateLimiterPlugin) GetAsset(assetPath string) ([]byte, string, error) {
    // Serve dashboard assets
}

func (p *RateLimiterPlugin) HandleCall(method string, payload []byte) ([]byte, error) {
    // Handle RPC calls from dashboard
}
```

### Shared State Management

The plugin shares state across all capabilities:
- In-memory maps for fast access (`sync.Map` for thread-safety)
- KV storage for persistence across restarts
- Load state during `Initialize()`
- Save state during `Shutdown()`

```go
func (p *RateLimiterPlugin) Initialize(ctx plugin_sdk.Context, config map[string]string) error {
    // Load persisted state from KV
    data, err := ctx.Services.KV().Read(ctx, "rate_limits")
    if err == nil {
        json.Unmarshal(data, &p.limits)
    }

    // Extract broker ID for Service API
    if brokerIDStr, ok := config["_service_broker_id"]; ok {
        var brokerID uint32
        fmt.Sscanf(brokerIDStr, "%d", &brokerID)
        ai_studio_sdk.SetServiceBrokerID(brokerID)
    }

    return nil
}
```

## File Structure

```
llm-rate-limiter-multiphase/
├── server/
│   ├── main.go              # Plugin server implementation
│   ├── manifest.json        # Plugin manifest with UI slots
│   └── config.schema.json   # Configuration schema
├── ui/
│   ├── webc/
│   │   └── dashboard.js     # WebComponent dashboard
│   └── assets/
│       ├── styles.css       # Dashboard styles
│       └── icon.svg         # Sidebar icon
├── go.mod
├── go.sum
└── README.md
```

## Building

```bash
cd examples/plugins/studio/llm-rate-limiter-multiphase/server
go build -o llm-rate-limiter main.go
```

## Deployment

### 1. Create Plugin

```bash
curl -X POST http://localhost:3000/api/v1/plugins \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "name": "LLM Rate Limiter",
    "slug": "llm-rate-limiter",
    "command": "file:///path/to/llm-rate-limiter",
    "hook_type": "multi",
    "plugin_type": "ai_studio",
    "is_active": true,
    "config": {
      "default_limit": "100",
      "time_window": "3600"
    }
  }'
```

### 2. Access Dashboard

Navigate to `/admin/rate-limiter` in the AI Studio dashboard to:
- View current rate limits for all apps
- Configure custom limits per app/user
- Monitor real-time request counts
- View historical usage patterns

## Configuration

### Plugin Config

```json
{
  "default_limit": "100",     // Default requests per window
  "time_window": "3600",      // Time window in seconds (1 hour)
  "enable_user_limits": "true", // Enable per-user limits
  "strict_mode": "false"      // Block on first violation
}
```

### Manifest Permissions

```json
{
  "permissions": {
    "services": [
      "apps.read",       // Read app configurations
      "kv.readwrite",    // Store/read rate limit state
      "analytics.read"   // Access usage analytics
    ]
  }
}
```

## Usage Examples

### Setting Custom Limits

```javascript
// From dashboard WebComponent
async function setLimit(appID, limit) {
    const response = await fetch('/plugin/com.example.rate-limiter/rpc', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
            method: 'set_limit',
            params: { app_id: appID, limit: limit }
        })
    });
    return response.json();
}
```

### Checking Current Status

```javascript
async function getStatus() {
    const response = await fetch('/plugin/com.example.rate-limiter/rpc', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
            method: 'get_status',
            params: {}
        })
    });
    return response.json();
}
```

## Key Patterns

### 1. Shared State Between Capabilities

```go
type RateLimiterPlugin struct {
    plugin_sdk.BasePlugin
    mu     sync.RWMutex
    limits map[uint32]int  // Shared across PostAuth, Response, and UI
}

// PostAuth reads limits
func (p *RateLimiterPlugin) HandlePostAuth(...) {
    p.mu.RLock()
    limit := p.limits[ctx.AppID]
    p.mu.RUnlock()
}

// UI writes limits
func (p *RateLimiterPlugin) HandleCall(method string, payload []byte) {
    if method == "set_limit" {
        p.mu.Lock()
        p.limits[appID] = newLimit
        p.mu.Unlock()
    }
}
```

### 2. Persistent State with KV Storage

```go
// Save state periodically
func (p *RateLimiterPlugin) saveState(ctx plugin_sdk.Context) error {
    p.mu.RLock()
    defer p.mu.RUnlock()

    data, _ := json.Marshal(map[string]interface{}{
        "limits": p.limits,
        "counts": p.counts,
        "updated_at": time.Now(),
    })

    return ctx.Services.KV().Write(ctx, "rate_limiter_state", data)
}
```

### 3. Request/Response Correlation

```go
// Add request ID in PostAuth
func (p *RateLimiterPlugin) HandlePostAuth(ctx plugin_sdk.Context, req *pb.EnrichedRequest) (*pb.PluginResponse, error) {
    requestID := generateID()
    req.Headers["X-Rate-Limiter-Request-ID"] = requestID

    // Store request metadata
    p.pendingRequests.Store(requestID, &RequestMetadata{
        AppID:     ctx.AppID,
        StartTime: time.Now(),
    })

    return &pb.PluginResponse{Modified: true, Request: req}, nil
}

// Use request ID in Response
func (p *RateLimiterPlugin) OnBeforeWrite(ctx plugin_sdk.Context, req *pb.ResponseWriteRequest) (*pb.ResponseWriteResponse, error) {
    requestID := req.Headers["X-Rate-Limiter-Request-ID"]

    if meta, ok := p.pendingRequests.LoadAndDelete(requestID); ok {
        duration := time.Since(meta.StartTime)
        // Update counters with correlation data
    }

    return &pb.ResponseWriteResponse{Modified: false}, nil
}
```

## Testing

### Unit Tests

```bash
go test ./server/...
```

### Integration Test

```bash
# Start AI Studio with plugin loaded
# Send test requests
curl -X POST http://localhost:3000/api/v1/ai/chat \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "app_id": 1,
    "message": "Test message"
  }'

# Check dashboard to verify counters updated
```

## Troubleshooting

### Limits Not Being Enforced

1. Check plugin is active and loaded
2. Verify PostAuth hook is registered
3. Check logs for initialization errors
4. Ensure KV storage is accessible

### Dashboard Not Showing Data

1. Verify broker ID is set correctly during initialization
2. Check RPC method names match dashboard calls
3. Verify Service API permissions (`apps.read`, `kv.readwrite`)
4. Check browser console for JavaScript errors

### Counters Not Persisting

1. Verify KV write operations succeed
2. Check `Shutdown()` is being called on restart
3. Increase auto-save frequency if needed
4. Check KV storage database connectivity

## Best Practices Demonstrated

1. **Multi-Capability Design**: Single plugin provides multiple related features
2. **Shared State Management**: Thread-safe state shared across capabilities
3. **Persistent Storage**: KV storage for durability across restarts
4. **Request Correlation**: Track requests from auth through response
5. **Rich Dashboard**: WebComponent UI with RPC backend
6. **Performance**: In-memory caching with periodic persistence
7. **Error Handling**: Graceful degradation if external systems fail

## Related Documentation

- [Multi-Capability Patterns](../../../../docs/site/docs/plugins-studio-ui.md#multi-capability-patterns)
- [PostAuth Handlers](../../../../docs/site/docs/plugins-microgateway.md#postauth-handler)
- [Response Handlers](../../../../docs/site/docs/plugins-microgateway.md#response-handler)
- [UI Provider](../../../../docs/site/docs/plugins-studio-ui.md)
- [Best Practices](../../../../docs/site/docs/plugins-best-practices.md)

## License

Apache 2.0
