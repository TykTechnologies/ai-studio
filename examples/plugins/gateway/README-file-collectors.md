# File-Based Data Collection Plugins

This directory contains example configurations for file-based data collection plugins that write analytics, proxy logs, and budget data to files.

## Quick Start

### 1. Run the test script
```bash
./examples/test-file-collectors.sh
```

This will:
- Check if plugins are built (and build them if needed)
- Verify configuration files exist
- Create output directories
- Show you how to start the microgateway

### 2. Start microgateway with file collectors
```bash
export PLUGINS_CONFIG_PATH=./examples/plugins-file-collectors.yaml
export DATA_OUTPUT_DIR=./data/collected
cd microgateway && ./bin/mgw
```

### 3. Make some API calls
Use your microgateway to proxy LLM requests. The plugins will automatically collect data.

### 4. Check the output files
```bash
ls -la ./data/collected/*/
tail ./data/collected/proxy_logs/proxy_logs_$(date +%Y-%m-%d).jsonl
tail ./data/collected/analytics/analytics_$(date +%Y-%m-%d).jsonl
tail ./data/collected/budget/budget_usage_$(date +%Y-%m-%d).csv
```

## Available Configurations

### plugins-file-collectors.yaml (Recommended)
Writes data to BOTH files and database. This is the safest approach for testing and provides:
- Dashboard analytics continue to work
- File-based backup for all data
- Flexibility to analyze data with external tools

**Use case:** Testing, development, observability

### plugins-file-collectors-replace.yaml
Writes data ONLY to files, skips the database entirely. This reduces database load but:
- Dashboard won't show this data
- Budget queries return no results
- You must use file-based analysis

**Use case:** High-throughput production, external analytics platform

### plugins-file-collectors-mixed.yaml
Mixed approach - different strategy per data type:
- **Proxy logs:** Files only (high volume)
- **Analytics:** Both files and database (dashboard needs it)
- **Budget:** Both files and database (critical data)

**Use case:** Production deployments with balanced performance

## Plugin Details

### File Proxy Collector
- **Hook type:** `proxy_log`
- **Output:** `proxy_logs_YYYY-MM-DD.jsonl`
- **Content:** Full request/response data with truncated previews

Example output:
```json
{"timestamp":"2023-12-01T15:30:45Z","app_id":123,"user_id":456,"vendor":"openai","response_code":200,"request_id":"proxy_123_1701439845000","request_size":150,"response_size":320,"request_preview":"{\"model\":\"gpt-4\",\"messages\":[...","response_preview":"{\"choices\":[{\"message\":{...","context":{"llm_id":1,"llm_slug":"gpt-4"}}
```

### File Analytics Collector
- **Hook type:** `analytics`
- **Output:** `analytics_YYYY-MM-DD.jsonl` or `analytics_YYYY-MM-DD.csv`
- **Content:** Token counts, costs, model information

CSV output:
```csv
timestamp,llm_id,model_name,vendor,prompt_tokens,response_tokens,cache_write_tokens,cache_read_tokens,total_tokens,cost,currency,app_id,user_id,tool_calls,choices,request_id
2023-12-01T15:30:45Z,1,gpt-4,openai,150,75,0,0,225,0.0045,USD,123,456,0,1,proxy_123_1701439845000
```

### File Budget Collector
- **Hook type:** `budget`
- **Output:** `budget_usage_YYYY-MM-DD.csv` + optional `budget_aggregate.json`
- **Content:** Budget usage tracking with optional aggregation

CSV output:
```csv
timestamp,app_id,llm_id,tokens_used,cost,requests_count,prompt_tokens,completion_tokens,period_start,period_end,request_id
2023-12-01T15:30:45Z,123,1,225,0.0045,1,150,75,2023-12-01T00:00:00Z,2023-12-31T23:59:59Z,budget_123_1701439845000
```

## Configuration Options

### Common Settings

```yaml
- name: "plugin-name"
  path: "./examples/plugins/gateway/plugin_name/plugin_binary"
  enabled: true
  priority: 100  # Lower numbers run first
  replace_database: false  # false = supplement, true = replace
  hook_types:
    - "proxy_log"  # or "analytics" or "budget"
  config:
    output_directory: "./data/collected/plugin_data"
    enabled: true
```

### Per-Plugin Settings

#### Proxy Collector
```yaml
config:
  output_directory: "./data/proxy_logs"
  enabled: true
```

#### Analytics Collector
```yaml
config:
  output_directory: "./data/analytics"
  enabled: true
  format: "jsonl"  # or "csv"
```

#### Budget Collector
```yaml
config:
  output_directory: "./data/budget"
  enabled: true
  format: "csv"  # or "jsonl"
  aggregate_mode: true  # Maintain running totals
```

## Environment Variables

- `PLUGINS_CONFIG_PATH`: Path to plugin configuration YAML file
- `DATA_OUTPUT_DIR`: Base directory for data files (can be overridden per plugin)

## File Formats

### JSONL (JSON Lines)
- One JSON object per line
- Easy to parse with `jq`: `cat file.jsonl | jq '.vendor'`
- Ideal for log processing pipelines (Logstash, Fluentd, etc.)
- Good for streaming data

### CSV
- Standard CSV format with headers
- Easy to import into Excel, Google Sheets
- Great for business analysis and reporting
- Better for aggregated data

## Monitoring

Check microgateway logs for plugin activity:
```bash
# Plugin loading
grep "data collection plugin" microgateway.log

# Plugin execution
grep "HandleProxyLog\|HandleAnalytics\|HandleBudgetUsage" microgateway.log
```

## Troubleshooting

### Plugins not loading
Check that:
1. Plugin binaries are built: `ls -la examples/plugins/gateway/*/file_*_collector`
2. Paths in YAML are correct and relative to microgateway working directory
3. `PLUGINS_CONFIG_PATH` environment variable is set

### No data in files
Check that:
1. Output directories exist and are writable
2. Plugins are enabled in config: `enabled: true`
3. You're making actual API calls through the microgateway
4. Microgateway logs show plugin execution

### Permission errors
```bash
# Ensure output directories are writable
mkdir -p ./data/collected/{proxy_logs,analytics,budget}
chmod -R 755 ./data/collected
```

## Advanced Usage

### Custom output directory per plugin
```yaml
config:
  output_directory: "/var/log/midsommar/proxy"  # Absolute path
```

### Disable specific plugins
```yaml
- name: "proxy-log-files"
  enabled: false  # Plugin won't run
```

### Change file format
```yaml
config:
  format: "csv"  # Switch from JSONL to CSV
```

### Enable budget aggregation
```yaml
config:
  aggregate_mode: true  # Creates budget_aggregate.json
```

## File Rotation

Files are automatically rotated daily:
- `proxy_logs_2023-12-01.jsonl`
- `proxy_logs_2023-12-02.jsonl`
- etc.

No automatic cleanup is provided. Use logrotate or similar tools for file retention management.

## Performance

These plugins are designed to be:
- **Fast:** Simple file append operations
- **Reliable:** Minimal dependencies, basic error handling
- **Observable:** Clear file structure and naming
- **Safe:** Non-blocking, isolated from database operations

Typical overhead: < 1ms per request

## Building Plugins

To rebuild all plugins:
```bash
cd examples/plugins/gateway/file_proxy_collector && go build -o file_proxy_collector main.go
cd ../file_analytics_collector && go build -o file_analytics_collector main.go
cd ../file_budget_collector && go build -o file_budget_collector main.go
```

Or use the test script which will build missing plugins automatically.

## Related Documentation

- [FILE_COLLECTORS.md](plugins/gateway/FILE_COLLECTORS.md) - Detailed plugin documentation
- [Plugin SDK README](../pkg/plugin_sdk/README.md) - SDK documentation for building custom plugins
- [Microgateway README](../microgateway/README.md) - Main microgateway documentation
