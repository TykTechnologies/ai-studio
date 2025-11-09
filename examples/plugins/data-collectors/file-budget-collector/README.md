# File Budget Collector Plugin

A data collection plugin that writes budget usage data to files in either CSV or JSONL format using the unified Plugin SDK.

## Features

- Collects budget consumption and token usage data per app/LLM
- Supports both CSV and JSONL output formats
- Daily log rotation (creates separate files per day)
- Optional aggregate mode for maintaining running totals
- Automatic directory creation
- Configurable output location

## Configuration

```yaml
data_collection_plugins:
  - name: "budget-files"
    path: "./examples/plugins/unified/data-collectors/file-budget-collector/file_budget_collector"
    enabled: true
    priority: 200
    replace_database: false
    hook_types:
      - "budget"
    config:
      output_directory: "./data/collected/budget"
      enabled: "true"
      format: "jsonl"  # or "csv"
      aggregate_mode: "true"  # optional
```

### Configuration Options

- `output_directory`: Directory where budget files will be written (default: `./data/budget`)
- `enabled`: Enable/disable the collector (`true` or `false`, default: `true`)
- `format`: Output format - `jsonl` for JSON Lines or `csv` for CSV (default: `jsonl`)
- `aggregate_mode`: Enable aggregation of budget data per app/LLM/period (`true` or `false`, default: `false`)

## Data Collected

The plugin collects the following budget usage data:

- Timestamp
- App ID and LLM ID
- Tokens used (total, prompt, completion)
- Cost
- Request count
- Budget period (start and end dates)
- Request ID

## Output Format

### JSONL Format
```json
{"timestamp":"2024-01-15T10:30:45Z","app_id":1,"llm_id":1,"tokens_used":1000,"cost":0.02,"requests_count":5,"prompt_tokens":800,"completion_tokens":200,"period_start":"2024-01-01T00:00:00Z","period_end":"2024-01-31T23:59:59Z","request_id":"req_123","context":{"period_duration_days":30}}
```

### CSV Format
```csv
timestamp,app_id,llm_id,tokens_used,cost,requests_count,prompt_tokens,completion_tokens,period_start,period_end,request_id
2024-01-15T10:30:45Z,1,1,1000,0.020000,5,800,200,2024-01-01T00:00:00Z,2024-01-31T23:59:59Z,req_123
```

## Aggregate Mode

When `aggregate_mode` is enabled, the plugin maintains a separate `budget_aggregate.json` file that contains running totals for each app/LLM/period combination:

```json
{
  "app_1_llm_1_2024-01": {
    "app_id": 1,
    "llm_id": 1,
    "period_start": "2024-01-01T00:00:00Z",
    "period_end": "2024-01-31T23:59:59Z",
    "total_tokens": 15000,
    "total_cost": 0.30,
    "total_requests": 75,
    "prompt_tokens": 12000,
    "completion_tokens": 3000,
    "last_updated": "2024-01-15T10:30:45Z"
  }
}
```

This is useful for:
- Quick budget summaries
- Monitoring spending trends
- Alert triggers based on thresholds

## Building

```bash
cd examples/plugins/unified/data-collectors/file-budget-collector
go mod tidy
go build -o file_budget_collector
```

## Usage

The plugin is automatically invoked by the Microgateway when configured. It will:

1. Create the output directory if it doesn't exist
2. Create daily log files (e.g., `budget_usage_2024-01-15.jsonl`)
3. Append budget usage data as it's generated
4. For CSV mode, automatically create headers for new files
5. If aggregate mode is enabled, maintain running totals in `budget_aggregate.json`

## File Naming

Files are named with a daily timestamp pattern:
- JSONL: `budget_usage_YYYY-MM-DD.jsonl`
- CSV: `budget_usage_YYYY-MM-DD.csv`
- Aggregate: `budget_aggregate.json` (single file, continuously updated)

## Replace Database Mode

When `replace_database: true` is set in the configuration, this plugin will handle budget tracking instead of storing data in the database. This is useful for:

- Custom budget tracking systems
- External billing systems
- Reduced database load
- Custom budget enforcement policies
