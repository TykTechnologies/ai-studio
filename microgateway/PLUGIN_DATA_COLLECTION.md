# Data Collection Plugin System

The microgateway now supports pluggable data collection through the go-plugin system. This allows users to replace or supplement the default database storage for ProxyLogs, Analytics, and Budget data with custom implementations like Elasticsearch, ClickHouse, data lakes, or external APIs.

## Overview

### Architecture

The system extends the existing go-plugin architecture with a new `data_collection` hook type:

- **Existing hooks**: `pre_auth`, `auth`, `post_auth`, `on_response` (for request/response interception)
- **New hook**: `data_collection` (for data storage interception)

### Data Collection Points

1. **ProxyLogs**: Full request/response payloads from LLM calls
2. **Analytics**: Token usage, costs, performance metrics
3. **Budget**: Budget usage tracking and enforcement data

## Configuration

### Environment Variables

```bash
# File-based plugin configuration
PLUGINS_CONFIG_PATH=./config/plugins.yaml

# Service-based plugin configuration (future)
PLUGINS_CONFIG_SERVICE_URL=https://config-service.company.com/api
PLUGINS_CONFIG_SERVICE_TOKEN=eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
PLUGINS_CONFIG_POLL_INTERVAL=30s
```

### Plugin Configuration File

**Example: `config/plugins.yaml`**

```yaml
version: "1.0"
data_collection_plugins:
  - name: "elasticsearch-collector"
    path: "./plugins/elasticsearch_collector"
    enabled: true
    priority: 100
    replace_database: false  # supplement database storage
    hook_types: 
      - "proxy_log"
      - "analytics"
      - "budget"
    config:
      elasticsearch_url: "${ELASTICSEARCH_URL:-http://localhost:9200}"
      username: "${ELASTICSEARCH_USERNAME}"
      password: "${ELASTICSEARCH_PASSWORD}"
      indices:
        proxy_logs: "microgateway-proxy-logs-${NODE_ENV:-dev}"
        analytics: "microgateway-analytics-${NODE_ENV:-dev}"
        budget: "microgateway-budget-${NODE_ENV:-dev}"
      batch_size: 100
      flush_interval: "30s"
```

## Plugin Development

### Interface Definition

Data collection plugins must implement the `DataCollectionPlugin` interface:

```go
type DataCollectionPlugin interface {
    BasePlugin
    
    // Handle proxy request/response logs
    HandleProxyLog(ctx context.Context, req *ProxyLogData, pluginCtx *PluginContext) (*DataCollectionResponse, error)
    
    // Handle analytics data (tokens, cost, performance)
    HandleAnalytics(ctx context.Context, req *AnalyticsData, pluginCtx *PluginContext) (*DataCollectionResponse, error)
    
    // Handle budget usage tracking
    HandleBudgetUsage(ctx context.Context, req *BudgetUsageData, pluginCtx *PluginContext) (*DataCollectionResponse, error)
}
```

### Example Plugin Structure

```go
package main

import (
    "context"
    "github.com/TykTechnologies/midsommar/microgateway/plugins/sdk"
)

type MyDataCollector struct {
    config *MyConfig
}

func (p *MyDataCollector) Initialize(config map[string]interface{}) error {
    // Parse configuration
    return nil
}

func (p *MyDataCollector) GetHookType() sdk.HookType {
    return sdk.HookTypeDataCollection
}

func (p *MyDataCollector) HandleProxyLog(ctx context.Context, req *sdk.ProxyLogData, pluginCtx *sdk.PluginContext) (*sdk.DataCollectionResponse, error) {
    // Process proxy log data
    return &sdk.DataCollectionResponse{
        Success: true,
        Handled: true,
    }, nil
}

// Implement other methods...

func main() {
    plugin := &MyDataCollector{}
    sdk.ServePlugin(plugin)
}
```

## Data Flow

### With Plugins Supplementing Database

1. LLM request/response occurs
2. **ProxyLog created** → Execute proxy log plugins → Store in database
3. **Analytics parsed** → Execute analytics plugins → Store in database  
4. **Budget updated** → Execute budget plugins → Update database

### With Plugins Replacing Database

1. LLM request/response occurs
2. **ProxyLog created** → Execute proxy log plugins → **Skip database** (if `replace_database: true`)
3. **Analytics parsed** → Execute analytics plugins → **Skip database** (if `replace_database: true`)
4. **Budget updated** → Execute budget plugins → **Skip database** (if `replace_database: true`)

## Plugin Types by Use Case

### 1. Elasticsearch Plugin
- **Use case**: Full-text search, log aggregation, real-time dashboards
- **Data types**: All (proxy logs, analytics, budget)
- **Benefits**: Advanced querying, visualization, scalability

### 2. ClickHouse Plugin  
- **Use case**: OLAP analytics, cost analysis, performance monitoring
- **Data types**: Analytics, budget (high-volume numerical data)
- **Benefits**: Fast aggregations, time-series analysis

### 3. Data Lake Plugin
- **Use case**: Long-term storage, compliance, data science
- **Data types**: All, with data retention policies
- **Benefits**: Cost-effective storage, ML/AI analysis

### 4. API Plugin
- **Use case**: Integration with external systems
- **Data types**: Any, filtered by business rules
- **Benefits**: Real-time notifications, external processing

## Installation & Usage

### 1. Configure Plugin

Create `config/plugins.yaml` with your plugin configuration.

### 2. Build Plugin

```bash
cd microgateway/plugins/examples/your_plugin
go build -o your_plugin main.go
```

### 3. Start Microgateway

```bash
export PLUGINS_CONFIG_PATH=./config/plugins.yaml
export ELASTICSEARCH_URL=http://localhost:9200
./microgateway
```

### 4. Monitor Logs

```
INFO Plugin manager configured for data collection
INFO Global data collection plugins loaded count=1
INFO Processing proxy log - executing data collection plugins
```

## Configuration Interface Types

The system supports multiple configuration sources:

### File-Based (Default)
- **Source**: Local YAML/JSON files
- **Use case**: Simple deployments, version control
- **Hot reload**: File system watching (5s poll)

### HTTP Service-Based (Future)
- **Source**: REST API endpoint
- **Use case**: Centralized configuration management
- **Hot reload**: Configurable polling interval

### Custom Loaders (Extensible)
- **Interface**: `PluginConfigLoader`
- **Examples**: Database, Consul, etcd, Kubernetes ConfigMaps

## Database Compatibility

### Supplement Mode (`replace_database: false`)
- Plugin processes data AND database stores data
- Provides redundancy and gradual migration
- No risk to existing functionality

### Replace Mode (`replace_database: true`) 
- Plugin processes data, database storage is skipped
- Reduces database load and storage costs
- Requires plugin reliability

## Security Considerations

- **Plugin isolation**: Plugins run in separate processes
- **Configuration security**: Environment variable expansion for secrets
- **Authentication**: HTTP loaders support token-based auth
- **Data privacy**: Plugins receive same data as database would

## Performance Notes

- **Non-blocking**: Plugin execution doesn't block request processing
- **Timeout**: 30-second timeout per plugin execution
- **Error handling**: Plugin failures don't affect database storage (unless replace mode)
- **Concurrency**: Multiple plugins can process same data simultaneously

## Monitoring and Health

- **Plugin health**: Automatic health monitoring via gRPC ping
- **Error logging**: Failed plugin executions logged but don't stop service
- **Metrics**: Plugin execution success/failure rates (future)
- **Hot reload**: Configuration changes applied without restart

## Migration Guide

### From Database-Only to Plugin-Enhanced

1. **Start with supplement mode**: `replace_database: false`
2. **Monitor plugin health**: Ensure data is flowing correctly
3. **Validate data integrity**: Compare plugin output with database
4. **Switch to replace mode**: Set `replace_database: true` when confident
5. **Monitor**: Ensure no data loss after transition

This system provides a complete solution for pluggable data collection that integrates seamlessly with the existing microgateway architecture while maintaining backward compatibility and operational safety.