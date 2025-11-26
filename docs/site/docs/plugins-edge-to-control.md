# Edge-to-Control Communication

This guide covers the **Edge-to-Control** communication system that allows plugins running on Microgateway (edge) instances to send data back to plugins running on AI Studio (control plane).

## Overview

In the hub-and-spoke architecture:
- **AI Studio** is the control plane (hub)
- **Microgateway** instances are edge nodes (spokes)

Edge plugins may need to send data back to the control plane for:
- Cache statistics aggregation
- Shared state synchronization
- Audit log centralization
- Custom analytics collection
- Alert and notification routing

The Edge-to-Control system provides a reliable, batched mechanism for this communication.

## Architecture

```
┌─────────────────────────────────────────────────────────────────────────┐
│                           AI Studio (Control Plane)                      │
│                                                                          │
│  ┌─────────────────┐    ┌──────────────────┐    ┌───────────────────┐  │
│  │ Control Server  │────│ Plugin Manager   │────│ Studio Plugin     │  │
│  │ (gRPC)          │    │                  │    │ EdgePayloadReceiver│  │
│  └────────┬────────┘    └──────────────────┘    └───────────────────┘  │
│           │                                                             │
└───────────┼─────────────────────────────────────────────────────────────┘
            │ gRPC (SendPluginControlBatch)
            │
┌───────────┼─────────────────────────────────────────────────────────────┐
│           │               Microgateway (Edge)                            │
│           ▼                                                              │
│  ┌─────────────────┐    ┌──────────────────┐    ┌───────────────────┐  │
│  │ Simple Client   │────│ Payload Queue    │◄───│ Edge Plugin       │  │
│  │ (heartbeat)     │    │ (SQLite)         │    │ SendToControl()   │  │
│  └─────────────────┘    └──────────────────┘    └───────────────────┘  │
│                                                                          │
└──────────────────────────────────────────────────────────────────────────┘
```

### Data Flow

1. **Edge Plugin** calls `SendToControl()` or `SendToControlJSON()`
2. Payload is **queued** to SQLite database (survives gateway restarts)
3. During **heartbeat** (every 30s), pending payloads are batched
4. Batch is sent via **gRPC** `SendPluginControlBatch` RPC
5. Control Server **routes** each payload to the target plugin
6. Studio plugin's `AcceptEdgePayload()` is called with the data

## Edge Plugin: Sending Data

### SDK Functions

The Microgateway SDK provides two functions for sending data to the control plane:

#### SendToControl

Send raw byte payloads:

```go
import "github.com/TykTechnologies/midsommar/v2/microgateway/plugins/sdk"

func (p *MyPlugin) HandlePostAuth(ctx context.Context, req *sdk.EnrichedRequest) (*sdk.PluginResponse, error) {
    // Send raw bytes to control plane
    pendingCount, err := sdk.SendToControl(
        ctx,
        []byte(`{"event": "request_processed", "count": 1}`), // payload (max 1MB)
        "req-12345",                                           // correlation ID (optional)
        map[string]string{                                     // metadata (optional)
            "source": "edge-us-west-1",
            "type":   "analytics",
        },
    )
    if err != nil {
        p.logger.Error("Failed to queue payload", "error", err)
    }

    p.logger.Info("Payload queued", "pending_count", pendingCount)
    return &sdk.PluginResponse{Modified: false}, nil
}
```

#### SendToControlJSON

Convenience function for JSON payloads:

```go
import "github.com/TykTechnologies/midsommar/v2/microgateway/plugins/sdk"

type CacheStats struct {
    Hits   int64   `json:"hits"`
    Misses int64   `json:"misses"`
    Size   int64   `json:"size_bytes"`
}

func (p *MyPlugin) sendCacheStats(ctx context.Context) error {
    stats := CacheStats{
        Hits:   p.cacheHits.Load(),
        Misses: p.cacheMisses.Load(),
        Size:   p.cacheSize.Load(),
    }

    pendingCount, err := sdk.SendToControlJSON(
        ctx,
        stats,                    // automatically marshaled to JSON
        "",                       // no correlation ID
        map[string]string{
            "metric_type": "cache",
        },
    )
    if err != nil {
        return fmt.Errorf("failed to queue stats: %w", err)
    }

    p.logger.Debug("Cache stats queued", "pending", pendingCount)
    return nil
}
```

### Function Signatures

```go
// SendToControl queues a raw payload for delivery to the control plane plugin
func SendToControl(
    ctx context.Context,
    payload []byte,              // Raw payload data (max 1MB)
    correlationID string,        // Optional tracking ID
    metadata map[string]string,  // Optional key-value metadata
) (pendingCount int64, err error)

// SendToControlJSON marshals value to JSON and queues for delivery
func SendToControlJSON(
    ctx context.Context,
    value interface{},           // Any JSON-serializable value
    correlationID string,        // Optional tracking ID
    metadata map[string]string,  // Optional key-value metadata
) (pendingCount int64, err error)
```

### Payload Limits

| Limit | Value |
|-------|-------|
| Maximum payload size | 1 MB |
| Maximum metadata entries | 50 |
| Maximum metadata key length | 256 bytes |
| Maximum metadata value length | 4096 bytes |
| Maximum correlation ID length | 256 bytes |

### Queue Behavior

- **Persistence**: Payloads are stored in SQLite and survive gateway restarts
- **Batching**: Payloads are batched during heartbeat (default: every 30s)
- **Batch size**: Up to 100 payloads per batch
- **Retention**: Successfully sent payloads are deleted; failed payloads are retried
- **Cleanup**: Payloads older than 24 hours are automatically cleaned up

### Example: Complete Edge Plugin

```go
package main

import (
    "context"
    "sync/atomic"
    "time"

    "github.com/TykTechnologies/midsommar/v2/microgateway/plugins/sdk"
    pb "github.com/TykTechnologies/midsommar/v2/proto"
)

type CachePlugin struct {
    sdk.BasePlugin
    hits   atomic.Int64
    misses atomic.Int64
}

func NewCachePlugin() *CachePlugin {
    return &CachePlugin{
        BasePlugin: sdk.NewBasePlugin(
            "llm-cache",
            "1.0.0",
            "LLM response caching with stats reporting",
        ),
    }
}

func (p *CachePlugin) Initialize(ctx sdk.Context, config map[string]string) error {
    // Start background stats reporter
    go p.reportStats(ctx)
    return nil
}

func (p *CachePlugin) HandlePostAuth(ctx sdk.Context, req *pb.EnrichedRequest) (*pb.PluginResponse, error) {
    // Check cache
    cacheKey := p.computeCacheKey(req)
    if cached, found := p.lookupCache(cacheKey); found {
        p.hits.Add(1)
        return &pb.PluginResponse{
            Block:        true,
            ResponseBody: cached,
            StatusCode:   200,
        }, nil
    }

    p.misses.Add(1)
    return &pb.PluginResponse{Modified: false}, nil
}

func (p *CachePlugin) reportStats(ctx context.Context) {
    ticker := time.NewTicker(5 * time.Minute)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            stats := map[string]interface{}{
                "hits":      p.hits.Load(),
                "misses":    p.misses.Load(),
                "hit_rate":  p.calculateHitRate(),
                "timestamp": time.Now().Unix(),
            }

            _, err := sdk.SendToControlJSON(ctx, stats, "", map[string]string{
                "metric_type": "cache_stats",
                "interval":    "5m",
            })
            if err != nil {
                p.logger.Warn("Failed to send stats", "error", err)
            }
        }
    }
}

func main() {
    sdk.Serve(NewCachePlugin())
}
```

## Control Plane Plugin: Receiving Data

### EdgePayloadReceiver Interface

AI Studio plugins implement `EdgePayloadReceiver` to receive payloads from edge instances:

```go
// EdgePayloadReceiver handles payloads sent from edge (microgateway) instances
type EdgePayloadReceiver interface {
    Plugin

    // AcceptEdgePayload is called when a payload arrives from an edge instance.
    // Returns:
    //   - handled: true if this plugin processed the payload
    //   - error: non-nil if processing failed
    AcceptEdgePayload(ctx Context, payload *EdgePayload) (handled bool, err error)
}
```

### EdgePayload Structure

```go
type EdgePayload struct {
    Payload           []byte            // Raw payload data from edge plugin
    EdgeID            string            // Edge instance identifier
    EdgeNamespace     string            // Namespace of the edge instance
    CorrelationID     string            // Correlation ID for tracking
    Metadata          map[string]string // Key-value metadata
    EdgeTimestamp     int64             // Unix timestamp when generated at edge
    ReceivedTimestamp int64             // Unix timestamp when received at control
}
```

### Example: Complete Control Plane Plugin

```go
package main

import (
    "encoding/json"
    "fmt"
    "sync"
    "time"

    "github.com/TykTechnologies/midsommar/v2/pkg/plugin_sdk"
)

type CacheStatsAggregator struct {
    plugin_sdk.BasePlugin

    mu    sync.RWMutex
    stats map[string]*EdgeStats // keyed by edge ID
}

type EdgeStats struct {
    EdgeID     string
    Hits       int64
    Misses     int64
    LastUpdate time.Time
}

type IncomingStats struct {
    Hits      int64 `json:"hits"`
    Misses    int64 `json:"misses"`
    HitRate   float64 `json:"hit_rate"`
    Timestamp int64 `json:"timestamp"`
}

func NewCacheStatsAggregator() *CacheStatsAggregator {
    return &CacheStatsAggregator{
        BasePlugin: plugin_sdk.NewBasePlugin(
            "cache-stats-aggregator",
            "1.0.0",
            "Aggregates cache statistics from edge instances",
        ),
        stats: make(map[string]*EdgeStats),
    }
}

func (p *CacheStatsAggregator) Initialize(ctx plugin_sdk.Context, config map[string]string) error {
    ctx.Services.Logger().Info("Cache stats aggregator initialized")
    return nil
}

// Implement EdgePayloadReceiver interface
func (p *CacheStatsAggregator) AcceptEdgePayload(ctx plugin_sdk.Context, payload *plugin_sdk.EdgePayload) (bool, error) {
    // Check if this payload is for us
    metricType, ok := payload.Metadata["metric_type"]
    if !ok || metricType != "cache_stats" {
        // Not our payload, return handled=false so other plugins can process it
        return false, nil
    }

    // Parse the payload
    var stats IncomingStats
    if err := json.Unmarshal(payload.Payload, &stats); err != nil {
        ctx.Services.Logger().Error("Failed to parse stats payload",
            "edge_id", payload.EdgeID,
            "error", err,
        )
        return true, fmt.Errorf("invalid payload format: %w", err)
    }

    // Store the stats
    p.mu.Lock()
    p.stats[payload.EdgeID] = &EdgeStats{
        EdgeID:     payload.EdgeID,
        Hits:       stats.Hits,
        Misses:     stats.Misses,
        LastUpdate: time.Unix(payload.EdgeTimestamp, 0),
    }
    p.mu.Unlock()

    ctx.Services.Logger().Info("Received cache stats from edge",
        "edge_id", payload.EdgeID,
        "namespace", payload.EdgeNamespace,
        "hits", stats.Hits,
        "misses", stats.Misses,
        "hit_rate", stats.HitRate,
    )

    // Optionally persist to KV storage
    key := fmt.Sprintf("edge-stats:%s", payload.EdgeID)
    data, _ := json.Marshal(p.stats[payload.EdgeID])
    ctx.Services.KV().Write(ctx, key, data)

    return true, nil
}

// GetAggregatedStats returns combined stats from all edges
func (p *CacheStatsAggregator) GetAggregatedStats() map[string]interface{} {
    p.mu.RLock()
    defer p.mu.RUnlock()

    var totalHits, totalMisses int64
    edgeCount := len(p.stats)

    for _, s := range p.stats {
        totalHits += s.Hits
        totalMisses += s.Misses
    }

    hitRate := float64(0)
    if totalHits+totalMisses > 0 {
        hitRate = float64(totalHits) / float64(totalHits+totalMisses) * 100
    }

    return map[string]interface{}{
        "edge_count":   edgeCount,
        "total_hits":   totalHits,
        "total_misses": totalMisses,
        "hit_rate":     hitRate,
    }
}

func main() {
    plugin_sdk.Serve(NewCacheStatsAggregator())
}
```

## Plugin ID Matching

The control plane routes payloads based on **plugin ID**. When an edge plugin sends a payload, it's associated with that plugin's numeric ID. The control plane then delivers it to the AI Studio plugin with the **same ID**.

### Configuration

1. **Create AI Studio plugin** (control plane receiver)
2. **Note the plugin ID** (e.g., `42`)
3. **Deploy Microgateway plugin** with the same ID configuration
4. Edge payloads from plugin 42 will be routed to AI Studio plugin 42

### Example Setup

```yaml
# AI Studio plugin (plugin_id: 42)
plugins:
  - id: 42
    name: "Cache Stats Aggregator"
    command: "file:///plugins/cache-stats-aggregator"
    hook_type: "post_auth"
    plugin_type: "studio"

# Microgateway config
plugins_config:
  plugins:
    - id: 42
      name: "LLM Cache"
      command: "file:///plugins/llm-cache"
      hook_type: "post_auth"
```

## Error Handling

### Edge Plugin Errors

```go
pendingCount, err := sdk.SendToControl(ctx, payload, "", nil)
if err != nil {
    switch {
    case errors.Is(err, sdk.ErrPayloadTooLarge):
        // Payload exceeds 1MB limit
        p.logger.Error("Payload too large, dropping")
    case errors.Is(err, sdk.ErrQueueFull):
        // Queue has too many pending payloads
        p.logger.Warn("Queue full, payload dropped")
    default:
        // Other error (e.g., serialization failed)
        p.logger.Error("Failed to queue payload", "error", err)
    }
}
```

### Control Plane Plugin Errors

```go
func (p *MyPlugin) AcceptEdgePayload(ctx plugin_sdk.Context, payload *plugin_sdk.EdgePayload) (bool, error) {
    // Return handled=false if this isn't your payload
    if !p.isMyPayload(payload) {
        return false, nil
    }

    // Return error for processing failures
    if err := p.processPayload(payload); err != nil {
        ctx.Services.Logger().Error("Failed to process payload",
            "edge_id", payload.EdgeID,
            "correlation_id", payload.CorrelationID,
            "error", err,
        )
        // Return handled=true with error - payload won't be retried
        return true, err
    }

    return true, nil
}
```

## Best Practices

### Edge Plugins

1. **Batch locally first**: Aggregate data before sending to reduce payload count
2. **Use correlation IDs**: For request/response matching or debugging
3. **Include metadata**: Add context like metric type, source, timestamp
4. **Handle errors gracefully**: Queue failures shouldn't crash your plugin
5. **Monitor pending count**: High pending counts may indicate connectivity issues

### Control Plane Plugins

1. **Check metadata first**: Return `handled=false` quickly for irrelevant payloads
2. **Validate payloads**: Don't trust edge data - validate before processing
3. **Use KV for persistence**: Store aggregated data for dashboard/API access
4. **Log with context**: Include edge ID and correlation ID in logs
5. **Handle duplicates**: Network issues may cause duplicate deliveries

### Performance

1. **Keep payloads small**: Smaller payloads = faster transmission
2. **Aggregate at edges**: Send summaries, not individual events
3. **Use appropriate intervals**: Balance freshness vs. overhead
4. **Monitor queue depth**: Alert on growing queues

## Troubleshooting

### Payloads Not Arriving

1. **Check edge connectivity**: Verify edge can reach control plane
2. **Check plugin IDs**: Edge and control plugins must have matching IDs
3. **Check logs**: Look for errors in both edge and control logs
4. **Verify plugin loaded**: Ensure control plane plugin is active

### High Latency

1. **Check batch interval**: Default is 30s heartbeat
2. **Check payload size**: Large payloads take longer to transmit
3. **Check network**: Latency between edge and control
4. **Check queue depth**: High pending count = backlog

### Payloads Being Dropped

1. **Check payload size**: Must be under 1MB
2. **Check queue capacity**: Queue may be full
3. **Check retention**: Payloads older than 24h are cleaned up
4. **Check errors**: Processing errors may cause drops

## Related Documentation

- [Plugin SDK Reference](plugins-sdk.md) - Complete SDK documentation
- [Microgateway Plugins Guide](plugins-microgateway.md) - Gateway plugin development
- [Service API Reference](plugins-service-api.md) - KV storage and logging APIs
- [Hub-and-Spoke Architecture](architecture-hub-spoke.md) - System architecture overview
