# Analytics & Monitoring

The microgateway provides comprehensive analytics and monitoring capabilities to track usage, performance, and costs across all AI/LLM interactions.

## Overview

Analytics features:
- **Real-Time Tracking**: Live request/response monitoring
- **Cost Analysis**: Detailed cost breakdowns by app, LLM, and time period
- **Performance Metrics**: Latency, throughput, and error rate monitoring
- **Usage Statistics**: Token consumption, request patterns, and trends
- **Data Retention**: Configurable retention periods for analytics data
- **Export Capabilities**: Data export for external analysis

## Analytics Data

### Event Tracking
Every LLM request generates an analytics event containing:
- Request ID and timestamp
- Application and LLM identifiers
- Endpoint and HTTP method
- Status code and error messages
- Token usage (input, output, total)
- Cost calculation
- Response latency
- Credential information

### Data Structure
```json
{
  "id": 1,
  "request_id": "req_abc123",
  "app_id": 1,
  "llm_id": 2,
  "credential_id": 1,
  "endpoint": "/llm/rest/gpt-4/chat/completions",
  "method": "POST",
  "status_code": 200,
  "request_tokens": 50,
  "response_tokens": 100,
  "total_tokens": 150,
  "cost": 0.045,
  "latency_ms": 1250,
  "error_message": "",
  "created_at": "2024-01-01T12:00:00Z"
}
```

## Accessing Analytics

### CLI Commands
```bash
# Get recent analytics events
mgw analytics events 1

# Analytics with pagination
mgw analytics events 1 --page=2 --limit=100

# Usage summary (last 7 days)
mgw analytics summary 1

# Summary for specific period
mgw analytics summary 1 \
  --start=2024-01-01T00:00:00Z \
  --end=2024-01-31T23:59:59Z

# Cost analysis
mgw analytics costs 1
```

### API Access
```bash
# Get analytics via API
curl -H "Authorization: Bearer $ADMIN_TOKEN" \
  "http://localhost:8080/api/v1/analytics/events?app_id=1&limit=50"

# Get usage summary
curl -H "Authorization: Bearer $ADMIN_TOKEN" \
  "http://localhost:8080/api/v1/analytics/summary?app_id=1"

# Get cost analysis
curl -H "Authorization: Bearer $ADMIN_TOKEN" \
  "http://localhost:8080/api/v1/analytics/costs?app_id=1"
```

## Analytics Queries

### Usage Patterns
```bash
# Requests per hour
mgw analytics events 1 --format=json | \
  jq '[.data[] | .created_at] | group_by(.[0:13]) | map(length)'

# Most used LLMs
mgw analytics events 1 --format=json | \
  jq '.data | group_by(.llm_id) | map({llm_id: .[0].llm_id, count: length}) | sort_by(.count) | reverse'

# Error rate analysis
mgw analytics events 1 --format=json | \
  jq '.data | group_by(.status_code >= 400) | map({error: .[0].status_code >= 400, count: length})'
```

### Cost Analysis
```bash
# Top cost contributors
mgw analytics events 1 --format=json | \
  jq '.data | sort_by(.cost) | reverse | .[0:10] | .[] | {cost, total_tokens, endpoint}'

# Cost by LLM
mgw analytics costs 1 --format=json | \
  jq '.data.cost_by_llm'

# Daily cost trends
mgw analytics events 1 --format=json | \
  jq '.data | group_by(.created_at[0:10]) | map({date: .[0].created_at[0:10], cost: map(.cost) | add})'
```

### Performance Analysis
```bash
# Average latency by LLM
mgw analytics events 1 --format=json | \
  jq '.data | group_by(.llm_id) | map({llm_id: .[0].llm_id, avg_latency: (map(.latency_ms) | add / length)})'

# Slowest requests
mgw analytics events 1 --format=json | \
  jq '.data | sort_by(.latency_ms) | reverse | .[0:10] | .[] | {latency_ms, endpoint, total_tokens}'

# Error analysis
mgw analytics events 1 --format=json | \
  jq '.data | map(select(.status_code >= 400)) | group_by(.status_code) | map({status: .[0].status_code, count: length})'
```

## Analytics Configuration

### Buffer Settings
```bash
# High-throughput settings
ANALYTICS_ENABLED=true
ANALYTICS_BUFFER_SIZE=5000
ANALYTICS_FLUSH_INTERVAL=5s
ANALYTICS_REALTIME=true

# Low-resource settings
ANALYTICS_BUFFER_SIZE=500
ANALYTICS_FLUSH_INTERVAL=30s
ANALYTICS_REALTIME=false
```

### Data Retention
```bash
# Retention configuration
ANALYTICS_RETENTION_DAYS=90   # Standard retention
ANALYTICS_RETENTION_DAYS=365  # Long-term retention
ANALYTICS_RETENTION_DAYS=30   # Short-term retention
```

### Real-Time Processing
```bash
# Enable real-time analytics
ANALYTICS_REALTIME=true

# This provides immediate data availability
# But increases CPU and memory usage
```

## Monitoring and Dashboards

### Prometheus Metrics
The microgateway exposes Prometheus metrics:
```bash
# View metrics
curl http://localhost:8080/metrics

# Key metrics:
# - microgateway_requests_total
# - microgateway_request_duration_seconds
# - microgateway_token_usage_total
# - microgateway_cost_total
```

### Custom Dashboards
Use analytics API to build custom dashboards:
```javascript
// Example: Real-time cost tracking
async function getCostData() {
  const response = await fetch('/api/v1/analytics/costs?app_id=1');
  const data = await response.json();
  return data.data.total_cost;
}
```

### Health Monitoring
```bash
# Monitor analytics system health
mgw system health

# Check analytics buffer status
mgw system metrics | grep analytics_buffer

# Monitor data retention
mgw analytics events 1 --limit=1 --format=json | \
  jq '.data[0].created_at'
```

## Data Export

### Export Analytics Data
```bash
# Export all events for an app
mgw analytics events 1 --limit=10000 --format=json > analytics-export.json

# Export cost analysis
mgw analytics costs 1 --format=json > cost-analysis.json

# Export usage summary
mgw analytics summary 1 --format=json > usage-summary.json
```

### Integration with External Systems
```bash
# Send data to external analytics platform
mgw analytics events 1 --format=json | \
  jq '.data[]' | \
  while IFS= read -r event; do
    curl -X POST https://analytics.company.com/events \
      -H "Content-Type: application/json" \
      -d "$event"
  done
```

## Analytics Use Cases

### Cost Optimization
```bash
# Identify most expensive operations
mgw analytics events 1 --format=json | \
  jq '.data | sort_by(.cost) | reverse | .[0:10]'

# Compare costs across LLMs
mgw analytics costs 1 --format=json | \
  jq '.data.cost_by_llm'

# Find inefficient usage patterns
mgw analytics events 1 --format=json | \
  jq '.data[] | select(.cost / .total_tokens > 0.001)'
```

### Performance Monitoring
```bash
# Track response time trends
mgw analytics summary 1 --format=json | \
  jq '.data.average_latency'

# Identify performance bottlenecks
mgw analytics events 1 --format=json | \
  jq '.data | sort_by(.latency_ms) | reverse | .[0:5]'

# Monitor error rates
mgw analytics summary 1 --format=json | \
  jq '.data | .failed_requests / .total_requests * 100'
```

### Usage Analysis
```bash
# Track user adoption
mgw analytics summary 1 --format=json | \
  jq '.data.requests_per_hour'

# Identify peak usage times
mgw analytics events 1 --format=json | \
  jq '.data | group_by(.created_at[11:13]) | map({hour: .[0].created_at[11:13], count: length})'

# Model preference analysis
mgw analytics events 1 --format=json | \
  jq '.data | group_by(.llm_id) | map({llm_id: .[0].llm_id, requests: length})'
```

## Analytics API Reference

### Events Endpoint
```bash
GET /api/v1/analytics/events?app_id=1&page=1&limit=50

# Query parameters:
# - app_id (required): Application ID
# - page: Page number (default: 1)
# - limit: Items per page (default: 50, max: 1000)
```

### Summary Endpoint
```bash
GET /api/v1/analytics/summary?app_id=1&start_time=2024-01-01T00:00:00Z

# Query parameters:
# - app_id (required): Application ID
# - start_time: Start time (default: 7 days ago)
# - end_time: End time (default: now)
```

### Costs Endpoint
```bash
GET /api/v1/analytics/costs?app_id=1&start_time=2024-01-01T00:00:00Z

# Query parameters:
# - app_id (required): Application ID  
# - start_time: Start time (default: 30 days ago)
# - end_time: End time (default: now)
```

## Data Privacy and Retention

### Data Retention Policies
```bash
# Configure retention periods
ANALYTICS_RETENTION_DAYS=90

# Automatic cleanup of old data
# Runs daily to remove events older than retention period
```

### Data Anonymization
```bash
# Analytics data includes:
# - Request metadata (non-sensitive)
# - Token counts and costs
# - Performance metrics
# - Error information

# Analytics data excludes:
# - Request content/payloads
# - Response content/payloads
# - User personal information
# - API keys or credentials
```

## Best Practices

### Analytics Configuration
- Enable real-time analytics for immediate insights
- Set appropriate buffer sizes for your request volume
- Configure retention periods based on compliance requirements
- Monitor analytics system resource usage

### Data Analysis
- Regular review of cost trends and usage patterns
- Set up automated alerts for unusual usage spikes
- Use analytics to optimize LLM selection and configuration
- Export data for detailed analysis in external tools

### Performance Monitoring
- Track response time trends to identify degradation
- Monitor error rates to detect provider issues
- Use latency data to optimize timeout settings
- Analyze token usage efficiency

## Troubleshooting

### Missing Analytics Data
```bash
# Check analytics is enabled
# Verify ANALYTICS_ENABLED=true

# Check buffer settings
mgw system metrics | grep analytics_buffer

# Review system logs for analytics errors
```

### Performance Impact
```bash
# Monitor analytics overhead
mgw system metrics | grep analytics_processing_time

# Adjust buffer settings if needed
ANALYTICS_BUFFER_SIZE=1000
ANALYTICS_FLUSH_INTERVAL=10s
```

### Data Accuracy
```bash
# Verify cost calculations
mgw analytics events 1 --limit=10 --format=json | \
  jq '.data[] | {total_tokens, cost, cost_per_token: (.cost / .total_tokens)}'

# Check for missing events
mgw analytics summary 1 --format=json | \
  jq '.data.total_requests'
```

---

Analytics provide essential insights for cost optimization and performance monitoring. For cost management, see [Budgets](budgets.md). For detailed logging, see [Proxy Logs](proxy-logs.md).
