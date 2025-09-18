# Proxy Logs

The microgateway captures detailed request and response logs for all AI/LLM interactions, providing comprehensive audit trails and debugging capabilities.

## Overview

Proxy logging features:
- **Full Request/Response Capture**: Complete HTTP request and response logging
- **Structured Logging**: JSON-formatted logs with consistent fields
- **Request Correlation**: Unique request IDs for tracing
- **Performance Tracking**: Request timing and latency measurement
- **Error Logging**: Detailed error information and stack traces
- **Configurable Retention**: Flexible log retention policies

## Log Structure

### Proxy Log Entry
Each proxied request generates a structured log entry:

```json
{
  "request_id": "req_abc123def456",
  "timestamp": "2024-01-01T12:00:00.123Z",
  "app_id": 1,
  "llm_id": 2,
  "credential_id": 1,
  "method": "POST",
  "endpoint": "/llm/rest/gpt-4/chat/completions",
  "request_headers": {
    "content-type": "application/json",
    "authorization": "[REDACTED]"
  },
  "request_body": {
    "model": "gpt-4",
    "messages": [
      {"role": "user", "content": "Hello, world!"}
    ]
  },
  "response_status": 200,
  "response_headers": {
    "content-type": "application/json"
  },
  "response_body": {
    "choices": [
      {"message": {"role": "assistant", "content": "Hello! How can I help you?"}}
    ],
    "usage": {"total_tokens": 150}
  },
  "latency_ms": 1250,
  "tokens_used": 150,
  "cost": 0.045,
  "error": null
}
```

### Log Fields

#### Request Information
- `request_id`: Unique identifier for correlation
- `timestamp`: ISO 8601 timestamp with milliseconds
- `method`: HTTP method (POST, GET, etc.)
- `endpoint`: Full request path
- `request_headers`: HTTP headers (sensitive headers redacted)
- `request_body`: Full request payload

#### Response Information
- `response_status`: HTTP status code
- `response_headers`: Response HTTP headers
- `response_body`: Full response payload
- `latency_ms`: Total request processing time
- `error`: Error details if request failed

#### Metadata
- `app_id`: Application identifier
- `llm_id`: LLM provider identifier
- `credential_id`: Credential used for authentication
- `tokens_used`: Token consumption
- `cost`: Calculated cost for the request

## Configuration

### Logging Settings
```bash
# Enable proxy logging
PROXY_LOGGING_ENABLED=true

# Log levels
LOG_LEVEL=info              # info, debug, warn, error
LOG_FORMAT=json             # json, text

# Request/response body logging
LOG_REQUEST_BODY=true       # Log full request payloads
LOG_RESPONSE_BODY=true      # Log full response payloads
LOG_HEADERS=true            # Log HTTP headers
```

### Sensitive Data Handling
```bash
# Redaction settings
REDACT_SENSITIVE_HEADERS=true    # Redact Authorization headers
REDACT_API_KEYS=true             # Redact API keys in logs
REDACT_USER_CONTENT=false        # Keep user content (for debugging)

# Custom redaction patterns
REDACTION_PATTERNS="password,secret,key,token"
```

### Log Storage
```bash
# Log file settings
LOG_FILE_PATH=/var/log/microgateway/proxy.log
LOG_FILE_MAX_SIZE=100MB
LOG_FILE_MAX_BACKUPS=10
LOG_FILE_MAX_AGE=30         # Days

# Database storage (optional)
STORE_LOGS_IN_DB=false      # Store logs in database
DB_LOG_RETENTION_DAYS=7     # Database log retention
```

## Accessing Proxy Logs

### File-Based Logs
```bash
# View recent logs
tail -f /var/log/microgateway/proxy.log

# Search logs by request ID
grep "req_abc123" /var/log/microgateway/proxy.log

# Filter by application
jq 'select(.app_id == 1)' /var/log/microgateway/proxy.log

# Filter by error status
jq 'select(.response_status >= 400)' /var/log/microgateway/proxy.log
```

### Database Logs (if enabled)
```bash
# Query proxy logs via API (if database storage enabled)
curl -H "Authorization: Bearer $ADMIN_TOKEN" \
  "http://localhost:8080/api/v1/proxy-logs?app_id=1&limit=50"

# Search by request ID
curl -H "Authorization: Bearer $ADMIN_TOKEN" \
  "http://localhost:8080/api/v1/proxy-logs/req_abc123"
```

## Log Analysis

### Request Tracing
```bash
# Follow a request through the system
REQUEST_ID="req_abc123def456"

# Find initial request
jq "select(.request_id == \"$REQUEST_ID\")" /var/log/microgateway/proxy.log

# Correlate with analytics
mgw analytics events 1 --format=json | \
  jq ".data[] | select(.request_id == \"$REQUEST_ID\")"
```

### Error Analysis
```bash
# Find failed requests
jq 'select(.response_status >= 400)' /var/log/microgateway/proxy.log

# Group errors by status code
jq 'select(.response_status >= 400) | .response_status' /var/log/microgateway/proxy.log | \
  sort | uniq -c

# Find timeout errors
jq 'select(.error and (.error | contains("timeout")))' /var/log/microgateway/proxy.log
```

### Performance Analysis
```bash
# Find slow requests
jq 'select(.latency_ms > 5000)' /var/log/microgateway/proxy.log

# Average latency by LLM
jq 'group_by(.llm_id) | map({llm_id: .[0].llm_id, avg_latency: (map(.latency_ms) | add / length)})' \
  /var/log/microgateway/proxy.log

# Token usage analysis
jq '.tokens_used' /var/log/microgateway/proxy.log | \
  awk '{sum+=$1; count++} END {print "Average tokens:", sum/count}'
```

## Log Monitoring

### Real-Time Monitoring
```bash
# Monitor logs in real-time
tail -f /var/log/microgateway/proxy.log | jq '.'

# Filter for errors only
tail -f /var/log/microgateway/proxy.log | \
  jq 'select(.response_status >= 400)'

# Monitor specific application
tail -f /var/log/microgateway/proxy.log | \
  jq 'select(.app_id == 1)'
```

### Log Aggregation
```bash
# Daily request summary
cat /var/log/microgateway/proxy.log | \
  jq -r '.timestamp[0:10]' | \
  sort | uniq -c

# Error summary
cat /var/log/microgateway/proxy.log | \
  jq 'select(.response_status >= 400) | .response_status' | \
  sort | uniq -c

# Cost summary
cat /var/log/microgateway/proxy.log | \
  jq '.cost' | \
  awk '{sum+=$1} END {print "Total cost:", sum}'
```

## Integration with External Systems

### Log Forwarding
```bash
# Forward logs to external system
tail -f /var/log/microgateway/proxy.log | \
  while IFS= read -r line; do
    curl -X POST https://logs.company.com/api/logs \
      -H "Content-Type: application/json" \
      -d "$line"
  done
```

### ELK Stack Integration
```bash
# Logstash configuration for microgateway logs
input {
  file {
    path => "/var/log/microgateway/proxy.log"
    codec => "json"
  }
}

filter {
  if [app_id] {
    mutate {
      add_tag => ["microgateway", "proxy"]
    }
  }
}

output {
  elasticsearch {
    hosts => ["localhost:9200"]
    index => "microgateway-proxy-%{+YYYY.MM.dd}"
  }
}
```

### Splunk Integration
```bash
# Splunk forwarder configuration
[monitor:///var/log/microgateway/proxy.log]
disabled = false
sourcetype = microgateway:proxy
index = microgateway
```

## Security Considerations

### Sensitive Data Redaction
```bash
# Configure redaction
REDACT_SENSITIVE_HEADERS=true
REDACT_API_KEYS=true
REDACT_USER_CONTENT=false    # Set to true for sensitive applications

# Custom redaction patterns
REDACTION_PATTERNS="password,secret,key,token,email"
```

### Access Control
```bash
# Restrict log file access
chmod 640 /var/log/microgateway/proxy.log
chown microgateway:microgateway /var/log/microgateway/proxy.log

# Log rotation with compression
logrotate /etc/logrotate.d/microgateway
```

### Compliance
```bash
# For compliance environments
LOG_REQUEST_BODY=false       # Disable request body logging
LOG_RESPONSE_BODY=false      # Disable response body logging
ANALYTICS_ONLY=true          # Only log metadata for analytics
```

## Log Rotation and Maintenance

### Log Rotation Configuration
```bash
# /etc/logrotate.d/microgateway
/var/log/microgateway/*.log {
    daily
    rotate 30
    compress
    delaycompress
    missingok
    notifempty
    copytruncate
}
```

### Cleanup Scripts
```bash
#!/bin/bash
# cleanup-logs.sh

# Remove logs older than 30 days
find /var/log/microgateway -name "*.log*" -mtime +30 -delete

# Remove empty log files
find /var/log/microgateway -name "*.log" -size 0 -delete

# Compress old logs
find /var/log/microgateway -name "*.log.*" ! -name "*.gz" -exec gzip {} \;
```

## Troubleshooting

### Missing Logs
```bash
# Check logging configuration
echo $PROXY_LOGGING_ENABLED
echo $LOG_FILE_PATH

# Verify log directory permissions
ls -la /var/log/microgateway/

# Check disk space
df -h /var/log
```

### Log File Issues
```bash
# Check log file permissions
ls -la /var/log/microgateway/proxy.log

# Test log writing
echo '{"test": "log entry"}' >> /var/log/microgateway/proxy.log

# Monitor log file growth
watch -n 1 'ls -lh /var/log/microgateway/proxy.log'
```

### Performance Impact
```bash
# Monitor logging overhead
mgw system metrics | grep log_processing_time

# Disable body logging if performance is impacted
LOG_REQUEST_BODY=false
LOG_RESPONSE_BODY=false
```

---

Proxy logs provide detailed audit trails and debugging information. For performance analysis, see [Analytics](analytics.md). For cost tracking, see [Budgets](budgets.md).
