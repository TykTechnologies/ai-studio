# Quick Start: File-Based Data Collection Plugins

Ready-to-use data collection plugins that append data to text files.

## ✅ What's Ready

### 3 Compiled Plugins
- **Proxy Log Collector**: `./plugins/examples/file_proxy_collector/file_proxy_collector`
- **Analytics Collector**: `./plugins/examples/file_analytics_collector/file_analytics_collector`  
- **Budget Collector**: `./plugins/examples/file_budget_collector/file_budget_collector`

### 3 Configuration Examples
- **`plugins-file-collectors.yaml`**: Supplement database (safe mode)
- **`plugins-file-collectors-replace.yaml`**: Replace database completely  
- **`plugins-mixed-example.yaml`**: Mixed strategies per data type

### Test Scripts
- **`verify-plugins.sh`**: Test plugin loading and configuration
- **`test-file-collectors.sh`**: Full test setup guide

## 🚀 One-Command Test

```bash
# Set configuration and test
export PLUGINS_CONFIG_PATH=./examples/plugins-file-collectors.yaml
export DATA_OUTPUT_DIR=./data/collected

# Start microgateway (plugins will load automatically)
./microgateway
```

**Expected output:**
```
INFO Loading global data collection plugins...
INFO Loaded global data collection plugin plugin=proxy-log-files
INFO Loaded global data collection plugin plugin=analytics-files  
INFO Loaded global data collection plugin plugin=budget-files
INFO Global data collection plugins loaded count=3
INFO Plugin manager configured for data collection
```

## 📁 Output Files

After making API calls, you'll see files like:

```
./data/collected/
├── proxy_logs/
│   └── proxy_logs_2023-12-01.jsonl
├── analytics/
│   └── analytics_2023-12-01.jsonl
└── budget/
    ├── budget_usage_2023-12-01.csv
    └── budget_aggregate.json (if aggregate_mode: true)
```

## 🎯 Data Flow Examples

### Proxy Logs (JSONL)
```json
{"timestamp":"2023-12-01T15:30:45Z","app_id":123,"vendor":"openai","response_code":200,"request_preview":"{\"model\":\"gpt-4\",\"messages\":[...","context":{"llm_slug":"gpt-4"}}
```

### Analytics (CSV)  
```csv
timestamp,llm_id,model_name,vendor,prompt_tokens,response_tokens,total_tokens,cost,currency,app_id
2023-12-01T15:30:45Z,1,gpt-4,openai,150,75,225,0.0045,USD,123
```

### Budget Usage (CSV)
```csv
timestamp,app_id,llm_id,tokens_used,cost,requests_count,period_start,period_end
2023-12-01T15:30:45Z,123,1,225,0.0045,1,2023-12-01T00:00:00Z,2023-12-31T23:59:59Z
```

## 🔧 Configuration Modes

### Safe Mode (Supplement Database)
```yaml
replace_database: false  # Data goes to BOTH files AND database
```

### Replace Mode (Files Only)
```yaml
replace_database: true   # Data goes ONLY to files, skips database
```

## 💡 Use Cases

- **Development**: File outputs for debugging and development
- **Compliance**: Audit trails and data retention requirements
- **Analytics**: Export to external analysis tools (Excel, R, Python)
- **Backup**: Secondary storage for critical data
- **Integration**: Feed data to external systems and pipelines

All three plugins are ready to use with the existing microgateway infrastructure! 🚀