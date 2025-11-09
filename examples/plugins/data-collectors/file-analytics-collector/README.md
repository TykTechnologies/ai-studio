# File Analytics Collector Plugin

A data collection plugin that writes analytics data to files in either CSV or JSONL format using the unified Plugin SDK.

## Features

- Collects token usage and cost analytics from LLM interactions
- Supports both CSV and JSONL output formats
- Daily log rotation (creates separate files per day)
- Automatic directory creation
- Configurable output location

## Configuration

```yaml
data_collection_plugins:
  - name: "analytics-files"
    path: "./examples/plugins/unified/data-collectors/file-analytics-collector/file_analytics_collector"
    enabled: true
    priority: 200
    replace_database: false
    hook_types:
      - "analytics"
    config:
      output_directory: "./data/collected/analytics"
      enabled: "true"
      format: "jsonl"  # or "csv"
```

### Configuration Options

- `output_directory`: Directory where analytics files will be written (default: `./data/analytics`)
- `enabled`: Enable/disable the collector (`true` or `false`, default: `true`)
- `format`: Output format - `jsonl` for JSON Lines or `csv` for CSV (default: `jsonl`)

## Data Collected

The plugin collects the following analytics data:

- Timestamp
- LLM ID and model name
- Vendor information
- Token counts (prompt, response, cache write/read, total)
- Cost and currency
- App ID and User ID
- Tool calls and choices count
- Request ID
- Context information (LLM slug)

## Output Format

### JSONL Format
```json
{"timestamp":"2024-01-15T10:30:45Z","llm_id":1,"model_name":"gpt-4","vendor":"openai","prompt_tokens":100,"response_tokens":50,"cache_write_prompt_tokens":0,"cache_read_prompt_tokens":0,"total_tokens":150,"cost":0.003,"currency":"USD","app_id":1,"user_id":1,"tool_calls":0,"choices":1,"request_id":"req_123","context":{"llm_slug":"openai-gpt4"}}
```

### CSV Format
```csv
timestamp,llm_id,model_name,vendor,prompt_tokens,response_tokens,cache_write_tokens,cache_read_tokens,total_tokens,cost,currency,app_id,user_id,tool_calls,choices,request_id
2024-01-15T10:30:45Z,1,gpt-4,openai,100,50,0,0,150,0.003000,USD,1,1,0,1,req_123
```

## Building

```bash
cd examples/plugins/unified/data-collectors/file-analytics-collector
go mod tidy
go build -o file_analytics_collector
```

## Usage

The plugin is automatically invoked by the Microgateway when configured. It will:

1. Create the output directory if it doesn't exist
2. Create daily log files (e.g., `analytics_2024-01-15.jsonl`)
3. Append analytics data as it's generated
4. For CSV mode, automatically create headers for new files

## File Naming

Files are named with a daily timestamp pattern:
- JSONL: `analytics_YYYY-MM-DD.jsonl`
- CSV: `analytics_YYYY-MM-DD.csv`

## Replace Database Mode

When `replace_database: true` is set in the configuration, this plugin will handle analytics collection instead of storing data in the database. This is useful for:

- Custom analytics pipelines
- External data warehouses
- Reduced database load
- Custom data retention policies
