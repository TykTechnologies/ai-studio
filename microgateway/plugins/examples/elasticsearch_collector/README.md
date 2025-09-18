# Elasticsearch Data Collection Plugin

This plugin demonstrates how to create a data collection plugin that sends ProxyLogs, Analytics, and Budget usage data to Elasticsearch instead of (or in addition to) the database.

## Features

- **ProxyLogs**: Stores full request/response data with metadata
- **Analytics**: Stores token usage, costs, and performance metrics
- **Budget**: Stores budget usage tracking data
- **Time-based indices**: Creates daily indices for better performance
- **Environment variable support**: Configuration via environment variables
- **Configurable replacement**: Can supplement or replace database storage

## Configuration

### Environment Variables

```bash
# Elasticsearch connection
ELASTICSEARCH_URL=http://localhost:9200
ELASTICSEARCH_USERNAME=elastic
ELASTICSEARCH_PASSWORD=changeme

# Plugin configuration file
PLUGINS_CONFIG_PATH=./config/plugins.yaml
```

### Plugin Configuration (plugins.yaml)

```yaml
version: "1.0"
data_collection_plugins:
  - name: "elasticsearch-collector"
    path: "./plugins/elasticsearch_collector"
    enabled: true
    priority: 100
    replace_database: false  # Set to true to skip database storage
    hook_types: 
      - "proxy_log"   # Store proxy request/response logs
      - "analytics"   # Store token usage and cost data
      - "budget"      # Store budget usage tracking
    config:
      elasticsearch_url: "${ELASTICSEARCH_URL:-http://localhost:9200}"
      username: "${ELASTICSEARCH_USERNAME}"
      password: "${ELASTICSEARCH_PASSWORD}"
      
      # Index configuration (supports date suffixes)
      indices:
        proxy_logs: "microgateway-proxy-logs"    # Will become: microgateway-proxy-logs-2023.12.01
        analytics: "microgateway-analytics"      # Will become: microgateway-analytics-2023.12.01
        budget: "microgateway-budget"            # Will become: microgateway-budget-2023.12.01
      
      # Connection settings
      timeout: "10s"
      batch_size: 100
      flush_interval: "30s"
      use_index_templates: true
```

## Building the Plugin

```bash
# Build the plugin binary
cd microgateway/plugins/examples/elasticsearch_collector
go build -o elasticsearch_collector main.go

# Or build all examples
cd microgateway
make build-plugins  # (if makefile target exists)
```

## Data Formats

### ProxyLogs Index
```json
{
  "@timestamp": "2023-12-01T15:30:45Z",
  "app_id": 123,
  "user_id": 456,
  "vendor": "openai", 
  "request_body": "{\"model\":\"gpt-4\",\"messages\":[...]}",
  "response_body": "{\"choices\":[{\"message\":{...}}]}",
  "response_code": 200,
  "request_id": "proxy_123_1701439845000",
  "context": {
    "llm_id": 1,
    "llm_slug": "gpt-4"
  }
}
```

### Analytics Index
```json
{
  "@timestamp": "2023-12-01T15:30:45Z",
  "llm_id": 1,
  "model_name": "gpt-4",
  "vendor": "openai",
  "prompt_tokens": 150,
  "response_tokens": 75, 
  "cache_write_prompt_tokens": 0,
  "cache_read_prompt_tokens": 0,
  "total_tokens": 225,
  "cost": 0.0045,
  "currency": "USD",
  "app_id": 123,
  "user_id": 456,
  "tool_calls": 0,
  "choices": 1,
  "request_id": "proxy_123_1701439845000"
}
```

### Budget Index
```json
{
  "@timestamp": "2023-12-01T15:30:45Z",
  "app_id": 123,
  "llm_id": 1,
  "tokens_used": 225,
  "cost": 0.0045,
  "requests_count": 1,
  "prompt_tokens": 150,
  "completion_tokens": 75,
  "period_start": "2023-12-01T00:00:00Z",
  "period_end": "2023-12-31T23:59:59Z",
  "request_id": "budget_123_1701439845000"
}
```

## Usage

1. **Configure Elasticsearch connection** via environment variables
2. **Enable the plugin** in your `plugins.yaml` configuration  
3. **Start microgateway** - plugin will be loaded automatically
4. **Monitor logs** for plugin activity and any errors
5. **Query Elasticsearch** to see collected data

## Advanced Usage

### Replace Database Storage
Set `replace_database: true` to completely skip database storage and only use Elasticsearch.

### Multiple Plugins
Configure multiple data collection plugins for different data types:

```yaml
data_collection_plugins:
  - name: "elasticsearch-logs"
    hook_types: ["proxy_log"]
    # ... elasticsearch config
    
  - name: "clickhouse-analytics" 
    hook_types: ["analytics", "budget"]
    # ... clickhouse config
```

### Index Templates

The plugin supports automatic Elasticsearch index template creation for optimal performance and mapping.

## Monitoring

Check microgateway logs for plugin activity:
- Plugin loading success/failure
- Data indexing success/failure  
- Connection issues
- Configuration errors

## Troubleshooting

### Plugin Won't Load
- Check plugin binary exists and is executable
- Verify configuration file path and syntax
- Check environment variable expansion
- Review microgateway logs for detailed error messages

### Data Not Appearing in Elasticsearch
- Verify Elasticsearch connectivity with `curl $ELASTICSEARCH_URL/_cluster/health`
- Check authentication credentials
- Verify index names and permissions
- Check for indexing errors in logs