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
# Enable Prometheus metrics
ENABLE_METRICS=true
METRICS_PATH=/metrics
METRICS_PORT=8080           # Same as HTTP server port

# Metrics namespace
METRICS_NAMESPACE=microgateway
METRICS_SUBSYSTEM=api
```

### Available Metrics
```bash
# Core service metrics
curl http://localhost:8080/metrics | grep microgateway

# Key metrics include:
# - microgateway_info (service information)
# - microgateway_requests_total (request counter)
# - microgateway_request_duration_seconds (request latency histogram)
# - microgateway_response_size_bytes (response size histogram)
# - microgateway_concurrent_requests (active requests gauge)
```

### Custom Metrics
```bash
# Business metrics
# - microgateway_token_usage_total (token consumption)
# - microgateway_cost_total (cost tracking)
# - microgateway_budget_utilization (budget usage percentage)
# - microgateway_llm_requests_total (per-LLM request counter)
# - microgateway_error_rate (error percentage)

# Technical metrics
# - microgateway_cache_hit_ratio (cache performance)
# - microgateway_db_connections_active (database connections)
# - microgateway_plugin_executions_total (plugin usage)
# - microgateway_config_reloads_total (configuration changes)
```

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
            "expr": "rate(microgateway_requests_total[5m])",
            "legendFormat": "{{method}} {{endpoint}}"
          }
        ]
      },
      {
        "title": "Response Latency",
        "targets": [
          {
            "expr": "histogram_quantile(0.95, rate(microgateway_request_duration_seconds_bucket[5m]))",
            "legendFormat": "95th percentile"
          }
        ]
      },
      {
        "title": "Error Rate",
        "targets": [
          {
            "expr": "rate(microgateway_requests_total{status_code=~\"4..|5..\"}[5m]) / rate(microgateway_requests_total[5m])",
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
        expr: histogram_quantile(0.95, rate(microgateway_request_duration_seconds_bucket[5m])) > 5
        for: 2m
        labels:
          severity: warning
        annotations:
          summary: "High request latency detected"
          
      - alert: HighErrorRate
        expr: rate(microgateway_requests_total{status_code=~"5.."}[5m]) / rate(microgateway_requests_total[5m]) > 0.05
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "High error rate detected"
          
      - alert: DatabaseConnectionExhaustion
        expr: microgateway_db_connections_in_use / microgateway_db_connections_max > 0.9
        for: 30s
        labels:
          severity: warning
        annotations:
          summary: "Database connection pool nearly exhausted"
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
# Request rate: rate(microgateway_requests_total[5m])
# P95 latency: histogram_quantile(0.95, rate(microgateway_request_duration_seconds_bucket[5m]))
# Error rate: rate(microgateway_requests_total{status_code=~"5.."}[5m])
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
# Cost rate: rate(microgateway_cost_total[1h])
# Token usage: rate(microgateway_token_usage_total[5m])
# App distribution: microgateway_requests_total by (app_id)
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
# DB connections: microgateway_db_connections_in_use
# Cache hit rate: microgateway_cache_hits_total / microgateway_cache_requests_total
# Plugin errors: rate(microgateway_plugin_errors_total[5m])
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
