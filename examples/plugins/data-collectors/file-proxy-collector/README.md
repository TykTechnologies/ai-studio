# File Proxy Collector Plugin

A data collection plugin that writes raw proxy request/response logs to JSONL files using the unified Plugin SDK.

## Features

- Collects raw HTTP request/response data from LLM proxy interactions
- JSONL output format
- Daily log rotation (creates separate files per day)
- Request/response body preview (first 200 characters)
- Automatic directory creation
- Configurable output location

## Configuration

```yaml
data_collection_plugins:
  - name: "proxy-files"
    path: "./examples/plugins/unified/data-collectors/file-proxy-collector/file_proxy_collector"
    enabled: true
    priority: 200
    replace_database: false
    hook_types:
      - "proxy_log"
    config:
      output_directory: "./data/collected/proxy"
      enabled: "true"
```

### Configuration Options

- `output_directory`: Directory where proxy logs will be written (default: `./data/proxy_logs`)
- `enabled`: Enable/disable the collector (`true` or `false`, default: `true`)

## Data Collected

The plugin collects the following proxy log data:

- Timestamp
- App ID and User ID
- Vendor information
- HTTP response code
- Request ID
- Request and response body sizes
- Preview of request/response bodies (first 200 characters)
- Context information (LLM ID, LLM slug)

## Output Format

### JSONL Format
```json
{"timestamp":"2024-01-15T10:30:45Z","app_id":1,"user_id":1,"vendor":"openai","response_code":200,"request_id":"req_123","request_size":256,"response_size":512,"request_preview":"{\"model\":\"gpt-4\",\"messages\":[{\"role\":\"user\",\"content\":\"Hello\"}]}","response_preview":"{\"id\":\"chatcmpl-123\",\"object\":\"chat.completion\",\"created\":1677652288,\"model\":\"gpt-4\",\"choices\":[{\"index\":0,\"message\":{\"role\":\"assistant\",\"content\":\"Hi there!\"}}]}","context":{"llm_id":1,"llm_slug":"openai-gpt4"}}
```

## Use Cases

This plugin is useful for:

- **Debugging**: Full request/response logging for troubleshooting
- **Auditing**: Complete audit trail of LLM interactions
- **Compliance**: Record keeping for regulatory requirements
- **Analysis**: Request/response pattern analysis
- **Billing verification**: Cross-check actual API usage

## Privacy Considerations

**Important**: This plugin logs request and response bodies which may contain:
- Sensitive user data
- Personal information
- Confidential business data

Use this plugin only in environments where:
- You have appropriate data handling policies
- Users are aware of logging
- Data retention and security measures are in place
- You comply with relevant privacy regulations (GDPR, CCPA, etc.)

Consider:
- Encrypting log files
- Setting up log rotation and retention policies
- Restricting access to log directories
- Redacting sensitive information before logging

## Building

```bash
cd examples/plugins/unified/data-collectors/file-proxy-collector
go mod tidy
go build -o file_proxy_collector
```

## Usage

The plugin is automatically invoked by the Microgateway when configured. It will:

1. Create the output directory if it doesn't exist
2. Create daily log files (e.g., `proxy_logs_2024-01-15.jsonl`)
3. Append proxy log data as requests are processed
4. Include body previews for quick inspection

## File Naming

Files are named with a daily timestamp pattern:
- JSONL: `proxy_logs_YYYY-MM-DD.jsonl`

## Body Preview

To prevent extremely large log entries, request and response bodies are truncated to 200 characters in the preview fields. The full body sizes are recorded in `request_size` and `response_size` fields.

If you need full bodies, you can modify the truncation length in the plugin code or implement a separate full-body storage mechanism.

## Replace Database Mode

This plugin typically runs alongside database logging (`replace_database: false`). The database stores metadata while this plugin provides detailed request/response logs.

However, if you want to use only file-based logging, you can set `replace_database: true`.
