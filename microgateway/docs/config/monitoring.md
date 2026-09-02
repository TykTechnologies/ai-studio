# Monitoring Configuration

The microgateway provides comprehensive monitoring capabilities including Prometheus metrics, health checks, and observability features.

## Overview

Monitoring configuration features:
- **Prometheus Metrics**: Standard metrics for monitoring systems
- **Health Checks**: Kubernetes-ready health and readiness probes
- **Structured Logging**: JSON-formatted logs for log aggregation
- **Distributed Tracing**: OpenTelemetry integration for request tracing
- **Performance Profiling**: Go pprof endpoints for performance analysis
- **Custom Metrics**: Application-specific metrics and dashboards

## Prometheus Metrics

### Metrics Configuration
```bash
# Enable Prometheus metrics. The endpoint is secure by default: unless a token
# is set or unauthenticated access is explicitly allowed, the route is not
# registered at all.
ENABLE_METRICS=true
METRICS_PATH=/metrics
METRICS_AUTH_TOKEN=            # requires "Authorization: Bearer <token>"
METRICS_ALLOW_UNAUTHENTICATED=false   # set true for in-cluster scraping

# Emit the pre-conventions aistudio_* series alongside the gen_ai.* ones.
# Set to false once dashboards have migrated.
METRICS_LEGACY_NAMES=true
```

Metrics are served on the main HTTP port (`PORT`, default 8080). There is no
separate metrics listener, and no namespace/subsystem configuration — metric
names are fixed.

### Available Metrics

```bash
curl -s http://localhost:8080/metrics
```

#### OpenTelemetry GenAI metrics

These follow the [OpenTelemetry GenAI semantic conventions](https://opentelemetry.io/docs/specs/semconv/gen-ai/).
Attributes are `gen_ai_provider_name`, `gen_ai_request_model`,
`gen_ai_operation_name`, `gen_ai_token_type`, and `error_type` on failures.

| Metric | Type | Description |
|---|---|---|
| `gen_ai_server_request_duration_seconds` | histogram | End-to-end request latency |
| `gen_ai_server_time_to_first_token_seconds` | histogram | Streaming: time to the first chunk |
| `gen_ai_server_time_per_output_token_seconds` | histogram | Streaming: mean inter-token latency |
| `gen_ai_client_token_usage` | histogram | Tokens per request, by `gen_ai_token_type` |
| `gen_ai_client_operation_duration_seconds` | histogram | Tool execution duration |

> The GenAI conventions are still marked *Development* upstream, not Stable, so
> these names may change.

#### AI Studio metrics

These have no counterpart in the conventions and keep their own names.

| Metric | Type | Labels | Description |
|---|---|---|---|
| `aistudio_llm_requests_total` | counter | `app_id`, `vendor`, `model`, `status_code` | Proxied LLM requests |
| `aistudio_llm_cost_total` | counter | `vendor`, `model`, `app_id` | Cumulative cost |
| `aistudio_llm_inflight_requests` | gauge | `vendor` | Requests currently in flight |
| `aistudio_policy_blocks_total` | counter | `rule_name`, `block_type` | Requests blocked by budget, firewall or filter |
| `aistudio_compliance_events_total` | counter | `event_type`, `severity`, `filter_name` | Events raised by filter scripts |
| `aistudio_tool_calls_total` | counter | `tool_name`, `app_id` | Tool / MCP invocations |

#### Legacy metrics

Emitted alongside the GenAI ones while `METRICS_LEGACY_NAMES=true` (the default),
and removed from the output when it is `false`:

| Legacy metric | Replaced by |
|---|---|
| `aistudio_llm_request_duration_seconds` | `gen_ai_server_request_duration_seconds` |
| `aistudio_llm_tokens_total` | `gen_ai_client_token_usage` |
| `aistudio_tool_execution_duration_seconds` | `gen_ai_client_operation_duration_seconds` |

## Health Checks

### Health Endpoints
```bash
# Basic health check
GET /health
# Returns: {"status": "ok", "service": "microgateway"}

# Readiness check with dependency validation
GET /ready
# Returns: {"status": "ready", "service": "microgateway"}

# Health check configuration
HEALTH_CHECK_ENABLED=true
HEALTH_CHECK_PATH=/health
READINESS_CHECK_PATH=/ready
HEALTH_CHECK_TIMEOUT=10s
```

### Kubernetes Health Checks
```yaml
# Kubernetes probe configuration
livenessProbe:
  httpGet:
    path: /health
    port: 8080
  initialDelaySeconds: 10
  periodSeconds: 10
  timeoutSeconds: 5
  failureThreshold: 3

readinessProbe:
  httpGet:
    path: /ready
    port: 8080
  initialDelaySeconds: 5
  periodSeconds: 5
  timeoutSeconds: 5
  failureThreshold: 3
```

### Custom Health Checks
```bash
# Database health check
# Included in /ready endpoint
# Tests database connectivity and query performance

# Cache health check
# Validates cache functionality and memory usage

# Plugin health check
# Verifies plugin processes are responsive

# External dependency health checks
HEALTH_CHECK_EXTERNAL_DEPS=true
HEALTH_CHECK_TIMEOUT=30s
```

## Logging Configuration

### Structured Logging
```bash
# Logging configuration
LOG_LEVEL=info              # debug, info, warn, error
LOG_FORMAT=json             # json, text
LOG_OUTPUT=stdout           # stdout, file, both

# Log file configuration
LOG_FILE_PATH=/var/log/microgateway/microgateway.log
LOG_FILE_MAX_SIZE=100MB
LOG_FILE_MAX_BACKUPS=10
LOG_FILE_MAX_AGE=30         # Days
LOG_FILE_COMPRESS=true
```

### Log Levels
```bash
# Debug logging (development)
LOG_LEVEL=debug
# Includes: Request/response details, cache operations, database queries

# Info logging (production)
LOG_LEVEL=info
# Includes: Service events, configuration changes, errors

# Warning logging (minimal)
LOG_LEVEL=warn
# Includes: Warnings and errors only

# Error logging (critical only)
LOG_LEVEL=error
# Includes: Errors only
```

### Log Formatting
```bash
# JSON format (production)
LOG_FORMAT=json
# Structured logging for log aggregation systems

# Text format (development)
LOG_FORMAT=text
# Human-readable logging for development

# Example JSON log entry:
{
  "timestamp": "2024-01-01T12:00:00Z",
  "level": "info",
  "service": "microgateway",
  "request_id": "req_abc123",
  "app_id": 1,
  "message": "LLM request processed",
  "latency_ms": 1250,
  "status_code": 200
}
```

## Distributed Tracing

### OpenTelemetry Configuration
```bash
# Enable distributed tracing
ENABLE_TRACING=true
TRACING_ENDPOINT=http://jaeger:14268/api/traces
TRACING_SERVICE_NAME=microgateway
TRACING_SERVICE_VERSION=v1.0.0

# Sampling configuration
TRACING_SAMPLE_RATE=0.1     # Sample 10% of requests
TRACING_SAMPLE_RATE=1.0     # Sample all requests (development)

# Trace export configuration
TRACING_EXPORT_TIMEOUT=30s
TRACING_EXPORT_BATCH_SIZE=100
```

### Trace Context Propagation
```bash
# Trace headers
# - traceparent: W3C trace context
# - tracestate: Vendor-specific trace state
# - X-Request-ID: Custom request correlation

# Automatic trace context injection
TRACING_INJECT_CONTEXT=true
TRACING_EXTRACT_CONTEXT=true
```

### Jaeger Integration
```bash
# Jaeger collector configuration
JAEGER_ENDPOINT=http://jaeger-collector:14268/api/traces
JAEGER_AGENT_ENDPOINT=jaeger-agent:6831
JAEGER_SERVICE_NAME=microgateway
JAEGER_TAGS=environment=production,region=us-west-1
```

## Performance Profiling

### pprof Configuration
```bash
# Enable Go pprof endpoints
ENABLE_PROFILING=true
PROFILING_PATH=/debug/pprof

# Available profiling endpoints:
# - /debug/pprof/profile (CPU profile)
# - /debug/pprof/heap (memory profile)
# - /debug/pprof/goroutine (goroutine profile)
# - /debug/pprof/block (blocking profile)
# - /debug/pprof/mutex (mutex profile)
```

### Profiling Usage
```bash
# CPU profiling
go tool pprof http://localhost:8080/debug/pprof/profile?seconds=30

# Memory profiling
go tool pprof http://localhost:8080/debug/pprof/heap

# Goroutine analysis
go tool pprof http://localhost:8080/debug/pprof/goroutine

# Generate profiling report
go tool pprof -http=:8081 http://localhost:8080/debug/pprof/profile
```

## Monitoring Integration

### Prometheus Configuration
```yaml
# prometheus.yml
global:
  scrape_interval: 15s

scrape_configs:
  - job_name: 'microgateway'
    static_configs:
      - targets: ['microgateway:8080']
    scrape_interval: 10s
    metrics_path: /metrics
    scheme: https
    tls_config:
      ca_file: /etc/prometheus/ca.crt
```

### Grafana Dashboards
```json
{
  "dashboard": {
    "title": "Microgateway Monitoring",
    "panels": [
      {
        "title": "Request Rate",
        "targets": [
          {
            "expr": "sum(rate(aistudio_llm_requests_total[5m]))",
            "legendFormat": "{{method}} {{endpoint}}"
          }
        ]
      },
      {
        "title": "Response Latency",
        "targets": [
          {
            "expr": "histogram_quantile(0.95, sum by (le) (rate(gen_ai_server_request_duration_seconds_bucket[5m])))",
            "legendFormat": "95th percentile"
          }
        ]
      },
      {
        "title": "Error Rate",
        "targets": [
          {
            "expr": "sum(rate(aistudio_llm_requests_total{status_code=~\"4..|5..\"}[5m])) / sum(rate(aistudio_llm_requests_total[5m]))",
            "legendFormat": "Error Rate"
          }
        ]
      }
    ]
  }
}
```

### Alertmanager Rules
```yaml
# alerting-rules.yml
groups:
  - name: microgateway
    rules:
      - alert: HighLatency
        expr: histogram_quantile(0.95, sum by (le) (rate(gen_ai_server_request_duration_seconds_bucket[5m]))) > 5
        for: 2m
        labels:
          severity: warning
        annotations:
          summary: "High request latency detected"
          
      - alert: HighErrorRate
        expr: sum(rate(aistudio_llm_requests_total{status_code=~"5.."}[5m])) / sum(rate(aistudio_llm_requests_total[5m])) > 0.05
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "High error rate detected"
          
      - alert: HighPolicyBlockRate
        expr: sum(rate(aistudio_policy_blocks_total[5m])) / sum(rate(aistudio_llm_requests_total[5m])) > 0.1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "More than 10% of requests are being blocked by budget or filter policy"

      - alert: SlowTimeToFirstToken
        expr: histogram_quantile(0.95, sum by (le) (rate(gen_ai_server_time_to_first_token_seconds_bucket[5m]))) > 5
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "P95 time to first token above 5s"
```

## Log Aggregation

### ELK Stack Integration
```yaml
# Logstash configuration
input {
  file {
    path => "/var/log/microgateway/*.log"
    codec => "json"
    type => "microgateway"
  }
}

filter {
  if [type] == "microgateway" {
    mutate {
      add_tag => ["microgateway", "ai-gateway"]
    }
    
    # Parse request_id for correlation
    if [request_id] {
      mutate {
        add_field => { "trace_id" => "%{request_id}" }
      }
    }
  }
}

output {
  elasticsearch {
    hosts => ["elasticsearch:9200"]
    index => "microgateway-logs-%{+YYYY.MM.dd}"
    template_name => "microgateway"
  }
}
```

### Fluentd Integration
```yaml
# fluent.conf
<source>
  @type tail
  path /var/log/microgateway/*.log
  pos_file /var/log/fluentd/microgateway.log.pos
  tag microgateway.*
  format json
  time_key timestamp
  time_format %Y-%m-%dT%H:%M:%S.%LZ
</source>

<match microgateway.**>
  @type elasticsearch
  host elasticsearch
  port 9200
  index_name microgateway-logs
  type_name _doc
  
  <buffer>
    @type file
    path /var/log/fluentd/microgateway.buffer
    flush_interval 10s
  </buffer>
</match>
```

## Application Performance Monitoring

### APM Integration
```bash
# Datadog APM
DD_TRACE_ENABLED=true
DD_SERVICE=microgateway
DD_ENV=production
DD_VERSION=v1.0.0
DD_TRACE_AGENT_URL=http://datadog-agent:8126

# New Relic APM
NEW_RELIC_ENABLED=true
NEW_RELIC_APP_NAME=microgateway
NEW_RELIC_LICENSE_KEY=${NEW_RELIC_LICENSE_KEY}

# Elastic APM
ELASTIC_APM_ENABLED=true
ELASTIC_APM_SERVICE_NAME=microgateway
ELASTIC_APM_SERVER_URL=http://apm-server:8200
```

### Custom Application Metrics
```bash
# Business metrics
CUSTOM_METRICS_ENABLED=true
BUSINESS_METRICS_INTERVAL=60s

# Track business KPIs:
# - Active applications
# - Token usage by application
# - Cost per application
# - LLM usage distribution
# - Geographic request distribution
```

## Alerting Configuration

### Alert Destinations
```bash
# Webhook alerting
ALERT_WEBHOOK_ENABLED=true
ALERT_WEBHOOK_URL=https://alerts.company.com/webhook
ALERT_WEBHOOK_TIMEOUT=10s

# Email alerting
ALERT_EMAIL_ENABLED=true
ALERT_EMAIL_SMTP_HOST=smtp.company.com
ALERT_EMAIL_SMTP_PORT=587
ALERT_EMAIL_FROM=alerts@company.com
ALERT_EMAIL_TO=ops@company.com

# Slack alerting
ALERT_SLACK_ENABLED=true
ALERT_SLACK_WEBHOOK_URL=${SLACK_WEBHOOK_URL}
ALERT_SLACK_CHANNEL=#alerts
```

### Alert Conditions
```bash
# Performance alerts
ALERT_HIGH_LATENCY_THRESHOLD=5000    # 5 seconds
ALERT_HIGH_ERROR_RATE_THRESHOLD=0.05 # 5% error rate
ALERT_HIGH_MEMORY_THRESHOLD=0.8      # 80% memory usage
ALERT_HIGH_CPU_THRESHOLD=0.9         # 90% CPU usage

# Business alerts
ALERT_BUDGET_THRESHOLD=0.9           # 90% budget used
ALERT_RATE_LIMIT_VIOLATIONS=100      # 100 violations/hour
ALERT_AUTH_FAILURES=50               # 50 failures/hour
```

## Monitoring Dashboards

### Service Overview Dashboard
```bash
# Key service metrics
# - Request rate (requests/second)
# - Response latency (percentiles)
# - Error rate (percentage)
# - Active connections
# - Memory and CPU usage
# - Cache hit ratio

# Example Grafana panel queries:
# Request rate: sum(rate(aistudio_llm_requests_total[5m]))
# P95 latency: histogram_quantile(0.95, sum by (le) (rate(gen_ai_server_request_duration_seconds_bucket[5m])))
# Error rate: sum(rate(aistudio_llm_requests_total{status_code=~"5.."}[5m]))
```

### Business Metrics Dashboard
```bash
# Business intelligence metrics
# - Cost per hour/day/month
# - Token usage trends
# - Application usage patterns
# - LLM provider distribution
# - Geographic usage distribution

# Example queries:
# Cost rate: sum(rate(aistudio_llm_cost_total[1h]))
# Token usage: sum(rate(gen_ai_client_token_usage_sum[5m]))
# App distribution: sum by (app_id) (aistudio_llm_requests_total)
```

### Infrastructure Dashboard
```bash
# Infrastructure metrics
# - Database connection pool usage
# - Cache performance
# - Plugin execution statistics
# - Network connections
# - Disk usage

# Example queries:
# In-flight requests:  sum by (vendor) (aistudio_llm_inflight_requests)
# Policy blocks:       sum by (block_type) (rate(aistudio_policy_blocks_total[5m]))
# Compliance events:   sum by (severity) (rate(aistudio_compliance_events_total[5m]))
# Tool latency (p95):  histogram_quantile(0.95, sum by (le) (rate(gen_ai_client_operation_duration_seconds_bucket[5m])))
#
# Note: there are no database-connection, cache or plugin-execution metrics
# today. Use the /health/detailed endpoint for database and plugin status.
```

## Log Monitoring

### Log Aggregation Configuration
```bash
# Log shipping configuration
LOG_SHIPPING_ENABLED=true
LOG_SHIPPING_ENDPOINT=https://logs.company.com/api/v1/logs
LOG_SHIPPING_API_KEY=${LOG_SHIPPING_API_KEY}
LOG_SHIPPING_BATCH_SIZE=1000
LOG_SHIPPING_FLUSH_INTERVAL=30s
```

### Log Analysis
```bash
# Log analysis queries
# Error analysis
jq 'select(.level == "error")' /var/log/microgateway/microgateway.log

# Performance analysis
jq 'select(.latency_ms > 5000)' /var/log/microgateway/microgateway.log

# Request pattern analysis
jq '.endpoint' /var/log/microgateway/microgateway.log | sort | uniq -c

# Authentication analysis
jq 'select(.message | contains("auth"))' /var/log/microgateway/microgateway.log
```

## Monitoring Security

### Metrics Security
```bash
# Secure metrics endpoint
METRICS_AUTH_REQUIRED=true
METRICS_AUTH_TOKEN=${METRICS_AUTH_TOKEN}

# Access metrics securely
curl -H "Authorization: Bearer $METRICS_AUTH_TOKEN" \
  http://localhost:8080/metrics

# TLS for metrics
METRICS_TLS_ENABLED=true
```

### Sensitive Data in Metrics
```bash
# Redact sensitive information from metrics
METRICS_REDACT_SENSITIVE=true

# Labels to exclude from metrics
METRICS_EXCLUDED_LABELS=api_key,user_email,client_ip

# Metric value redaction
METRICS_REDACT_VALUES=true  # Replace actual values with placeholder
```

## Monitoring Best Practices

### Metric Collection
- Collect metrics at appropriate intervals (10-60 seconds)
- Use histograms for latency and size measurements
- Use counters for event counting
- Use gauges for current state measurements
- Avoid high-cardinality labels

### Dashboard Design
- Focus on key business and technical metrics
- Use appropriate time ranges for different metrics
- Include both current state and trend information
- Group related metrics together
- Provide drill-down capabilities

### Alerting Strategy
- Set appropriate thresholds based on baseline performance
- Use multiple severity levels (info, warning, critical)
- Implement escalation procedures
- Avoid alert fatigue with proper tuning
- Include actionable information in alert messages

### Log Management
- Use structured logging for better analysis
- Implement log retention policies
- Monitor log volume and storage usage
- Use log sampling for high-volume systems
- Implement log-based alerting for critical errors

## Monitoring Tools Integration

### Prometheus + Grafana
```bash
# Complete monitoring stack
# Prometheus: Metrics collection and storage
# Grafana: Visualization and dashboards
# Alertmanager: Alert routing and notification

# Docker Compose monitoring stack
version: '3.8'
services:
  prometheus:
    image: prom/prometheus
    ports: ["9090:9090"]
    command: ["--config.file=/etc/prometheus/prometheus.yml"]
    
  grafana:
    image: grafana/grafana
    ports: ["3000:3000"]
    environment:
      GF_SECURITY_ADMIN_PASSWORD: admin
      
  alertmanager:
    image: prom/alertmanager
    ports: ["9093:9093"]
```

### Cloud Monitoring
```bash
# AWS CloudWatch
AWS_CLOUDWATCH_ENABLED=true
AWS_CLOUDWATCH_REGION=us-west-2
AWS_CLOUDWATCH_NAMESPACE=MicroGateway

# Google Cloud Monitoring
GCP_MONITORING_ENABLED=true
GCP_PROJECT_ID=my-project
GCP_MONITORING_RESOURCE_TYPE=generic_task

# Azure Monitor
AZURE_MONITOR_ENABLED=true
AZURE_MONITOR_INSTRUMENTATION_KEY=${AZURE_INSTRUMENTATION_KEY}
```

## Troubleshooting Monitoring

### Metrics Issues
```bash
# Check metrics endpoint
curl http://localhost:8080/metrics

# Validate metric format
curl -s http://localhost:8080/metrics | promtool check metrics

# Debug metrics collection
ENABLE_METRICS_DEBUG=true
LOG_LEVEL=debug
```

### Health Check Issues
```bash
# Test health endpoints
curl http://localhost:8080/health
curl http://localhost:8080/ready

# Debug health check failures
HEALTH_CHECK_DEBUG=true
LOG_LEVEL=debug
```

### Tracing Issues
```bash
# Check tracing configuration
echo $TRACING_ENDPOINT
echo $TRACING_SAMPLE_RATE

# Test trace export
curl -X POST $TRACING_ENDPOINT/api/traces \
  -H "Content-Type: application/json" \
  -d '{"test": "trace"}'

# Debug trace collection
TRACING_DEBUG=true
LOG_LEVEL=debug
```

---

Monitoring configuration ensures visibility into microgateway performance and health. For performance optimization, see [Performance Tuning](performance.md). For security monitoring, see [Security Configuration](security.md).
