# File-Based Data Collection Plugins

Three simple data collection plugins that append data to text files instead of (or in addition to) database storage.

## Quick Start

### 1. Build the plugins
```bash
cd microgateway/plugins/examples/file_proxy_collector && go build -o file_proxy_collector main.go
cd ../file_analytics_collector && go build -o file_analytics_collector main.go  
cd ../file_budget_collector && go build -o file_budget_collector main.go
```

### 2. Configure the plugins
```bash
export PLUGINS_CONFIG_PATH=./examples/plugins-file-collectors.yaml
export DATA_OUTPUT_DIR=./data/collected
```

### 3. Start microgateway
```bash
./microgateway
```

Look for these log messages:
```
INFO Loading global data collection plugins...
INFO Global data collection plugins loaded count=3
INFO Plugin manager configured for data collection
```

### 4. Generate data
Make LLM API calls to generate data that will be captured by the plugins.

### 5. Check output files
```bash
ls -la ./data/collected/*/
tail ./data/collected/proxy_logs/proxy_logs_$(date +%Y-%m-%d).jsonl
```

## Plugin Details

### 1. File Proxy Collector
- **Hook type**: `proxy_log`
- **Output**: Daily JSONL files with request/response data
- **File format**: `proxy_logs_YYYY-MM-DD.jsonl`
- **Content**: Full proxy log data with truncated request/response previews

**Example output:**
```json
{"timestamp":"2023-12-01T15:30:45Z","app_id":123,"user_id":456,"vendor":"openai","response_code":200,"request_id":"proxy_123_1701439845000","request_size":150,"response_size":320,"request_preview":"{\"model\":\"gpt-4\",\"messages\":[{\"role\":\"user\",\"content\":\"Hello!\"}]...","response_preview":"{\"choices\":[{\"message\":{\"role\":\"assistant\",\"content\":\"Hello! How can I help you?\"}...","context":{"llm_id":1,"llm_slug":"gpt-4"}}
```

### 2. File Analytics Collector
- **Hook type**: `analytics`
- **Output**: Daily JSONL or CSV files with token usage and cost data
- **File format**: `analytics_YYYY-MM-DD.jsonl` or `analytics_YYYY-MM-DD.csv`
- **Content**: Token counts, costs, model information

**CSV output example:**
```csv
timestamp,llm_id,model_name,vendor,prompt_tokens,response_tokens,cache_write_tokens,cache_read_tokens,total_tokens,cost,currency,app_id,user_id,tool_calls,choices,request_id
2023-12-01T15:30:45Z,1,gpt-4,openai,150,75,0,0,225,0.0045,USD,123,456,0,1,proxy_123_1701439845000
```

### 3. File Budget Collector  
- **Hook type**: `budget`
- **Output**: Daily CSV/JSONL files + optional aggregate summary
- **File format**: `budget_usage_YYYY-MM-DD.csv` + `budget_aggregate.json`
- **Content**: Budget usage tracking with optional aggregation

**CSV output example:**
```csv
timestamp,app_id,llm_id,tokens_used,cost,requests_count,prompt_tokens,completion_tokens,period_start,period_end,request_id
2023-12-01T15:30:45Z,123,1,225,0.0045,1,150,75,2023-12-01T00:00:00Z,2023-12-31T23:59:59Z,budget_123_1701439845000
```

**Aggregate summary example:**
```json
{
  "app_123_llm_1_2023-12": {
    "app_id": 123,
    "llm_id": 1,
    "period_start": "2023-12-01T00:00:00Z",
    "period_end": "2023-12-31T23:59:59Z",
    "total_tokens": 1250,
    "total_cost": 0.025,
    "total_requests": 5,
    "prompt_tokens": 800,
    "completion_tokens": 450,
    "last_updated": "2023-12-01T15:35:22Z"
  }
}
```

## Configuration Options

### Per-Plugin Configuration

#### Proxy Log Collector
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
  format: "csv"    # or "jsonl"
  aggregate_mode: true  # Maintain running totals
```

## Configuration Files

- **`plugins-file-collectors.yaml`**: Supplement database storage (safe)
- **`plugins-file-collectors-replace.yaml`**: Replace database storage completely
- **`plugins-mixed-example.yaml`**: Mixed approach - different strategies per data type

## File Formats

### JSONL (JSON Lines)
- One JSON object per line
- Easy to parse with tools like `jq`
- Suitable for log processing pipelines

### CSV
- Standard CSV format with headers
- Easy to import into Excel/Google Sheets
- Suitable for analysis and reporting

## Testing

Run the verification script:
```bash
./examples/verify-plugins.sh
```

Or use the test setup script:
```bash
./examples/test-file-collectors.sh
```

## Monitoring

Check microgateway logs for plugin activity:
```bash
# Plugin loading
grep "data collection plugin" microgateway.log

# Plugin execution  
grep "Processing proxy log\|HandleProxyLog\|HandleAnalytics\|HandleBudgetUsage" microgateway.log

# File operations
ls -la ./data/collected/*/
```

## File Rotation

Files are automatically rotated daily based on timestamp:
- `proxy_logs_2023-12-01.jsonl`
- `analytics_2023-12-01.csv`
- `budget_usage_2023-12-01.csv`

## Performance

These file-based plugins are designed to be:
- **Fast**: Simple file append operations
- **Reliable**: Minimal dependencies, basic error handling
- **Observable**: Clear file structure and naming
- **Safe**: Non-blocking, isolated from database operations

Use these as templates for more sophisticated data collection plugins!