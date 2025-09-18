# Data Plugins

Data collection plugins allow replacement or supplementation of the default database storage for analytics, budgets, and proxy logs with custom implementations.

## Overview

Data plugin features:
- **Pluggable Storage**: Replace or supplement database storage
- **Multiple Data Types**: Analytics, budgets, proxy logs
- **External Integration**: Elasticsearch, ClickHouse, data lakes, APIs
- **Flexible Modes**: Supplement or replace database storage
- **Real-Time Processing**: Live data streaming to external systems
- **High Performance**: Non-blocking plugin execution

## Data Collection Points

The microgateway collects three types of data that can be handled by plugins:

### 1. Proxy Logs
Complete request/response data from LLM interactions:
- Full HTTP request and response payloads
- Request timing and performance metrics
- Error information and debugging data
- Security and audit information

### 2. Analytics
Aggregated usage and performance metrics:
- Token consumption and cost calculations
- Request frequency and patterns
- Performance metrics (latency, throughput)
- Error rates and categorization

### 3. Budget Usage
Budget tracking and enforcement data:
- Real-time budget consumption
- Cost attribution by app and LLM
- Budget threshold monitoring
- Usage forecasting data

## Plugin Interface

### Data Collection Plugin Interface
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

### Data Structures

#### Proxy Log Data
```go
type ProxyLogData struct {
    RequestID      string                 `json:"request_id"`
    Timestamp      time.Time              `json:"timestamp"`
    AppID          uint                   `json:"app_id"`
    LLMID          uint                   `json:"llm_id"`
    CredentialID   uint                   `json:"credential_id"`
    Method         string                 `json:"method"`
    Endpoint       string                 `json:"endpoint"`
    RequestHeaders map[string]string      `json:"request_headers"`
    RequestBody    []byte                 `json:"request_body"`
    ResponseStatus int                    `json:"response_status"`
    ResponseHeaders map[string]string     `json:"response_headers"`
    ResponseBody   []byte                 `json:"response_body"`
    LatencyMS      int64                  `json:"latency_ms"`
    TokensUsed     int                    `json:"tokens_used"`
    Cost           float64                `json:"cost"`
    Error          string                 `json:"error,omitempty"`
}
```

#### Analytics Data
```go
type AnalyticsData struct {
    AppID        uint      `json:"app_id"`
    LLMID        uint      `json:"llm_id"`
    CredentialID uint      `json:"credential_id"`
    RequestID    string    `json:"request_id"`
    Endpoint     string    `json:"endpoint"`
    Method       string    `json:"method"`
    StatusCode   int       `json:"status_code"`
    TokensUsed   int       `json:"tokens_used"`
    Cost         float64   `json:"cost"`
    LatencyMS    int64     `json:"latency_ms"`
    Timestamp    time.Time `json:"timestamp"`
    ErrorMessage string    `json:"error_message,omitempty"`
}
```

#### Budget Usage Data
```go
type BudgetUsageData struct {
    AppID         uint      `json:"app_id"`
    LLMID         uint      `json:"llm_id"`
    TokensUsed    int       `json:"tokens_used"`
    Cost          float64   `json:"cost"`
    BudgetRemaining float64 `json:"budget_remaining"`
    BudgetLimit   float64   `json:"budget_limit"`
    PeriodStart   time.Time `json:"period_start"`
    PeriodEnd     time.Time `json:"period_end"`
    Timestamp     time.Time `json:"timestamp"`
}
```

## Data Plugin Configuration

### Basic Configuration
```yaml
# config/plugins.yaml
version: "1.0"
data_collection_plugins:
  - name: "elasticsearch-collector"
    path: "./plugins/elasticsearch_collector"
    enabled: true
    priority: 100
    replace_database: false  # Supplement database storage
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

### Advanced Configuration
```yaml
data_collection_plugins:
  - name: "multi-backend-collector"
    path: "./plugins/multi_backend"
    enabled: true
    replace_database: true  # Replace database storage entirely
    config:
      backends:
        elasticsearch:
          url: "${ELASTICSEARCH_URL}"
          indices:
            analytics: "microgateway-analytics"
        clickhouse:
          url: "${CLICKHOUSE_URL}"
          database: "microgateway"
        s3:
          bucket: "${S3_BUCKET}"
          region: "${AWS_REGION}"
      routing:
        proxy_logs: ["elasticsearch", "s3"]
        analytics: ["clickhouse"]
        budget: ["elasticsearch"]
```

## Plugin Operation Modes

### Supplement Mode (Default)
```yaml
replace_database: false
```
- Plugin processes data AND database stores data
- Provides redundancy and gradual migration path
- No risk to existing functionality
- Allows comparison between plugin and database data

### Replace Mode
```yaml
replace_database: true
```
- Plugin processes data, database storage is skipped
- Reduces database load and storage costs
- Requires plugin reliability for data integrity
- Full responsibility for data persistence

## Data Flow

### Supplement Mode Flow
```
LLM Request/Response
    ↓
Data Collection
    ↓
├── Database Storage (default)
└── Plugin Processing (supplement)
```

### Replace Mode Flow
```
LLM Request/Response
    ↓
Data Collection
    ↓
Plugin Processing (primary)
    ↓
Database Storage (skipped)
```

## Example Data Plugins

### Elasticsearch Plugin
```go
package main

import (
    "context"
    "encoding/json"
    "github.com/elastic/go-elasticsearch/v8"
    "github.com/TykTechnologies/midsommar/microgateway/plugins/sdk"
)

type ElasticsearchPlugin struct {
    client  *elasticsearch.Client
    indices map[string]string
}

func (p *ElasticsearchPlugin) Initialize(config map[string]interface{}) error {
    // Parse configuration
    cfg := elasticsearch.Config{
        Addresses: []string{config["elasticsearch_url"].(string)},
    }
    
    client, err := elasticsearch.NewClient(cfg)
    if err != nil {
        return err
    }
    
    p.client = client
    p.indices = config["indices"].(map[string]string)
    return nil
}

func (p *ElasticsearchPlugin) HandleAnalytics(ctx context.Context, req *sdk.AnalyticsData, pluginCtx *sdk.PluginContext) (*sdk.DataCollectionResponse, error) {
    doc := map[string]interface{}{
        "app_id":      req.AppID,
        "llm_id":      req.LLMID,
        "tokens_used": req.TokensUsed,
        "cost":        req.Cost,
        "latency_ms":  req.LatencyMS,
        "timestamp":   req.Timestamp,
    }
    
    body, _ := json.Marshal(doc)
    _, err := p.client.Index(
        p.indices["analytics"],
        strings.NewReader(string(body)),
    )
    
    return &sdk.DataCollectionResponse{
        Success: err == nil,
        Handled: true,
        Error:   errToString(err),
    }, nil
}
```

### ClickHouse Plugin
```go
package main

import (
    "context"
    "database/sql"
    _ "github.com/ClickHouse/clickhouse-go/v2"
    "github.com/TykTechnologies/midsommar/microgateway/plugins/sdk"
)

type ClickHousePlugin struct {
    db *sql.DB
}

func (p *ClickHousePlugin) HandleAnalytics(ctx context.Context, req *sdk.AnalyticsData, pluginCtx *sdk.PluginContext) (*sdk.DataCollectionResponse, error) {
    query := `
        INSERT INTO analytics_events 
        (app_id, llm_id, tokens_used, cost, latency_ms, timestamp)
        VALUES (?, ?, ?, ?, ?, ?)
    `
    
    _, err := p.db.ExecContext(ctx, query,
        req.AppID, req.LLMID, req.TokensUsed,
        req.Cost, req.LatencyMS, req.Timestamp,
    )
    
    return &sdk.DataCollectionResponse{
        Success: err == nil,
        Handled: true,
        Error:   errToString(err),
    }, nil
}
```

### S3 Data Lake Plugin
```go
package main

import (
    "context"
    "encoding/json"
    "github.com/aws/aws-sdk-go/service/s3"
    "github.com/TykTechnologies/midsommar/microgateway/plugins/sdk"
)

type S3DataLakePlugin struct {
    s3Client *s3.S3
    bucket   string
}

func (p *S3DataLakePlugin) HandleProxyLog(ctx context.Context, req *sdk.ProxyLogData, pluginCtx *sdk.PluginContext) (*sdk.DataCollectionResponse, error) {
    // Create S3 object key with date partitioning
    key := fmt.Sprintf("proxy-logs/year=%d/month=%02d/day=%02d/%s.json",
        req.Timestamp.Year(), req.Timestamp.Month(), req.Timestamp.Day(),
        req.RequestID)
    
    body, _ := json.Marshal(req)
    
    _, err := p.s3Client.PutObjectWithContext(ctx, &s3.PutObjectInput{
        Bucket: aws.String(p.bucket),
        Key:    aws.String(key),
        Body:   strings.NewReader(string(body)),
    })
    
    return &sdk.DataCollectionResponse{
        Success: err == nil,
        Handled: true,
        Error:   errToString(err),
    }, nil
}
```

## Data Plugin Use Cases

### Analytics Enhancement
```yaml
# Real-time analytics to multiple systems
data_collection_plugins:
  - name: "elasticsearch-analytics"
    config:
      elasticsearch_url: "https://elastic.company.com"
      index: "microgateway-analytics"
      
  - name: "datadog-metrics"
    config:
      datadog_api_key: "${DATADOG_API_KEY}"
      metric_prefix: "microgateway"
      
  - name: "custom-dashboard"
    config:
      webhook_url: "https://dashboard.company.com/webhook"
```

### Compliance Logging
```yaml
# Separate compliance and operational data
data_collection_plugins:
  - name: "compliance-logger"
    config:
      compliance_endpoint: "https://compliance.company.com/api"
      encryption_key: "${COMPLIANCE_ENCRYPTION_KEY}"
      retention_policy: "7years"
      
  - name: "operational-analytics"
    config:
      operational_db: "postgres://analytics:pass@db:5432/ops"
      retention_policy: "30days"
```

### Cost Attribution
```yaml
# Detailed cost tracking and attribution
data_collection_plugins:
  - name: "cost-attribution"
    config:
      billing_system: "https://billing.company.com/api"
      cost_center_mapping: "/etc/microgateway/cost-centers.yaml"
      real_time_alerts: true
```

## Configuration Management

### Environment Variables
```bash
# Data plugin configuration
PLUGINS_CONFIG_PATH=./config/plugins.yaml

# Plugin-specific environment variables
ELASTICSEARCH_URL=http://localhost:9200
ELASTICSEARCH_USERNAME=elastic
ELASTICSEARCH_PASSWORD=secret
CLICKHOUSE_URL=tcp://localhost:9000
AWS_REGION=us-west-2
S3_BUCKET=microgateway-data-lake
```

### Hot Reload
```bash
# Reload plugin configuration without restart
kill -HUP $(pgrep microgateway)

# Or use API
curl -X POST http://localhost:8080/api/v1/plugins/reload \
  -H "Authorization: Bearer $ADMIN_TOKEN"
```

### Configuration Validation
```bash
# Test plugin configuration
./dist/microgateway --test-plugins

# Validate specific plugin
mgw plugin validate elasticsearch-collector
```

## Monitoring Data Plugins

### Plugin Health
```bash
# Check data plugin health
mgw plugin health elasticsearch-collector

# Monitor plugin metrics
mgw system metrics | grep data_collection

# View plugin logs
tail -f /var/log/microgateway/plugins.log | grep data_collection
```

### Performance Monitoring
```bash
# Plugin execution metrics
# - data_collection_executions_total
# - data_collection_duration_seconds
# - data_collection_errors_total
# - data_collection_success_rate

curl http://localhost:8080/metrics | grep data_collection
```

### Error Handling
```bash
# Monitor plugin failures
mgw system metrics | grep data_collection_errors

# Check plugin error logs
grep "data collection failed" /var/log/microgateway/plugins.log

# Plugin failure doesn't affect request processing
# Errors are logged but requests continue normally
```

## Data Plugin Examples

### Multi-Destination Plugin
```go
type MultiDestinationPlugin struct {
    elasticsearch *ElasticsearchClient
    clickhouse    *ClickHouseClient
    s3            *S3Client
}

func (p *MultiDestinationPlugin) HandleAnalytics(ctx context.Context, req *sdk.AnalyticsData, pluginCtx *sdk.PluginContext) (*sdk.DataCollectionResponse, error) {
    // Send to multiple destinations concurrently
    var wg sync.WaitGroup
    errors := make([]error, 3)
    
    // Elasticsearch for search
    wg.Add(1)
    go func() {
        defer wg.Done()
        errors[0] = p.elasticsearch.Index(req)
    }()
    
    // ClickHouse for analytics
    wg.Add(1)
    go func() {
        defer wg.Done()
        errors[1] = p.clickhouse.Insert(req)
    }()
    
    // S3 for long-term storage
    wg.Add(1)
    go func() {
        defer wg.Done()
        errors[2] = p.s3.Store(req)
    }()
    
    wg.Wait()
    
    // Check for any failures
    for _, err := range errors {
        if err != nil {
            return &sdk.DataCollectionResponse{
                Success: false,
                Error:   err.Error(),
            }, nil
        }
    }
    
    return &sdk.DataCollectionResponse{
        Success: true,
        Handled: true,
    }, nil
}
```

### Filtering Plugin
```go
type FilteringPlugin struct {
    sensitiveApps map[uint]bool
}

func (p *FilteringPlugin) HandleProxyLog(ctx context.Context, req *sdk.ProxyLogData, pluginCtx *sdk.PluginContext) (*sdk.DataCollectionResponse, error) {
    // Skip sensitive applications
    if p.sensitiveApps[req.AppID] {
        return &sdk.DataCollectionResponse{
            Success: true,
            Handled: false, // Skip processing
        }, nil
    }
    
    // Redact sensitive information
    req.RequestBody = p.redactSensitiveData(req.RequestBody)
    req.ResponseBody = p.redactSensitiveData(req.ResponseBody)
    
    // Process redacted data
    return p.processLog(req)
}
```

### Batch Processing Plugin
```go
type BatchProcessingPlugin struct {
    buffer    []interface{}
    batchSize int
    ticker    *time.Ticker
    mutex     sync.Mutex
}

func (p *BatchProcessingPlugin) HandleAnalytics(ctx context.Context, req *sdk.AnalyticsData, pluginCtx *sdk.PluginContext) (*sdk.DataCollectionResponse, error) {
    p.mutex.Lock()
    defer p.mutex.Unlock()
    
    // Add to buffer
    p.buffer = append(p.buffer, req)
    
    // Process batch if full
    if len(p.buffer) >= p.batchSize {
        go p.processBatch(append([]interface{}{}, p.buffer...))
        p.buffer = p.buffer[:0]
    }
    
    return &sdk.DataCollectionResponse{
        Success: true,
        Handled: true,
    }, nil
}

func (p *BatchProcessingPlugin) processBatch(batch []interface{}) {
    // Batch processing logic
    err := p.sendBatch(batch)
    if err != nil {
        // Handle batch failure
        p.retryBatch(batch)
    }
}
```

## Migration Strategies

### Gradual Migration
```yaml
# Start with supplement mode
replace_database: false

# Verify data integrity between plugin and database
# Monitor plugin performance and reliability
# Switch to replace mode when confident

replace_database: true
```

### A/B Testing
```yaml
# Route different apps to different backends
data_collection_plugins:
  - name: "test-backend"
    enabled: true
    config:
      target_apps: [1, 2, 3]  # Test apps
      
  - name: "production-backend"
    enabled: true
    config:
      target_apps: [4, 5, 6]  # Production apps
```

### Failover Configuration
```yaml
# Primary and backup data collection
data_collection_plugins:
  - name: "primary-collector"
    priority: 100
    config:
      primary: true
      
  - name: "backup-collector"
    priority: 200
    config:
      activate_on_failure: true
      health_check_url: "primary-collector-health"
```

## Troubleshooting

### Plugin Not Executing
```bash
# Check plugin configuration
cat config/plugins.yaml | grep -A 10 data_collection

# Verify plugin is loaded
mgw plugin list | grep data_collection

# Check plugin health
mgw plugin health my-data-plugin
```

### Data Not Appearing
```bash
# Check plugin success rate
mgw system metrics | grep data_collection_success

# Review plugin error logs
grep "data collection" /var/log/microgateway/plugins.log

# Test external system connectivity
curl -I $ELASTICSEARCH_URL
```

### Performance Issues
```bash
# Monitor plugin execution time
mgw system metrics | grep data_collection_duration

# Check buffer sizes and batch settings
grep "batch_size\|flush_interval" config/plugins.yaml

# Adjust configuration for performance
# Increase batch sizes, adjust flush intervals
```

---

Data plugins provide flexible data collection and storage options. For configuration details, see [Data Plugin Configuration](data-plugin-config.md). For AI Studio integration, see [AI Studio Integration](ai-studio-logs.md).
