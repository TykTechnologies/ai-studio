# AI Studio Integration

This guide covers sending microgateway logs and analytics data back to AI Studio for centralized monitoring and analysis.

## Overview

AI Studio integration features:
- **Centralized Logging**: Send all microgateway logs to AI Studio
- **Real-Time Analytics**: Stream analytics data for centralized dashboards
- **Cost Attribution**: Unified cost tracking across multiple gateways
- **Performance Monitoring**: Centralized performance metrics and alerting
- **Audit Trail**: Complete audit logging for compliance
- **Multi-Gateway Management**: Manage multiple microgateway instances

## Integration Architecture

### Data Flow
```
Microgateway → Data Plugin → AI Studio API → AI Studio Dashboard
   (events)     (collection)    (streaming)     (visualization)
```

### Components
- **Microgateway**: Source of logs and analytics data
- **AI Studio Data Plugin**: Custom plugin for AI Studio integration
- **AI Studio API**: Endpoint for receiving microgateway data
- **AI Studio Dashboard**: Centralized monitoring and analytics interface

## AI Studio Data Plugin

### Plugin Configuration
```yaml
# config/plugins.yaml
data_collection_plugins:
  - name: "ai-studio-integration"
    path: "./plugins/ai_studio_collector"
    enabled: true
    priority: 50
    replace_database: false      # Supplement database storage
    hook_types: 
      - "proxy_log"
      - "analytics"
      - "budget"
    config:
      ai_studio_endpoint: "${AI_STUDIO_API_URL}/api/v1/microgateway"
      api_key: "${AI_STUDIO_API_KEY}"
      gateway_id: "${MICROGATEWAY_INSTANCE_ID}"
      
      # Data configuration
      send_full_logs: true
      send_analytics: true
      send_budget_data: true
      
      # Performance settings
      batch_size: 500
      flush_interval: "30s"
      compression: true
      
      # Security settings
      tls_enabled: true
      verify_ssl: true
```

### Environment Configuration
```bash
# AI Studio integration environment variables
AI_STUDIO_API_URL=https://ai-studio.company.com
AI_STUDIO_API_KEY=your-ai-studio-api-key
MICROGATEWAY_INSTANCE_ID=gateway-prod-us-west-1

# Optional settings
AI_STUDIO_TENANT_ID=your-tenant-id
AI_STUDIO_REGION=us-west-1
AI_STUDIO_ENVIRONMENT=production
```

## Data Types Sent to AI Studio

### Proxy Logs
Complete request/response data:
```json
{
  "source": "microgateway",
  "gateway_id": "gateway-prod-us-west-1",
  "tenant_id": "your-tenant-id",
  "log_type": "proxy_log",
  "timestamp": "2024-01-01T12:00:00Z",
  "data": {
    "request_id": "req_abc123",
    "app_id": 1,
    "llm_id": 2,
    "method": "POST",
    "endpoint": "/llm/rest/gpt-4/chat/completions",
    "status_code": 200,
    "latency_ms": 1250,
    "tokens_used": 150,
    "cost": 0.045,
    "request_body": "...",      # Optional, configurable
    "response_body": "..."      # Optional, configurable
  }
}
```

### Analytics Data
Aggregated usage metrics:
```json
{
  "source": "microgateway",
  "gateway_id": "gateway-prod-us-west-1",
  "log_type": "analytics",
  "timestamp": "2024-01-01T12:00:00Z",
  "data": {
    "app_id": 1,
    "llm_id": 2,
    "request_id": "req_abc123",
    "tokens_used": 150,
    "cost": 0.045,
    "latency_ms": 1250,
    "status_code": 200,
    "endpoint": "/llm/rest/gpt-4/chat/completions"
  }
}
```

### Budget Data
Budget usage and tracking:
```json
{
  "source": "microgateway",
  "gateway_id": "gateway-prod-us-west-1",
  "log_type": "budget",
  "timestamp": "2024-01-01T12:00:00Z",
  "data": {
    "app_id": 1,
    "llm_id": 2,
    "cost": 0.045,
    "budget_remaining": 954.25,
    "budget_limit": 1000.0,
    "period_start": "2024-01-01T00:00:00Z",
    "period_end": "2024-01-31T23:59:59Z"
  }
}
```

## AI Studio Plugin Implementation

### Basic Plugin Structure
```go
package main

import (
    "bytes"
    "context"
    "encoding/json"
    "net/http"
    "github.com/TykTechnologies/midsommar/microgateway/plugins/sdk"
)

type AIStudioPlugin struct {
    endpoint   string
    apiKey     string
    gatewayID  string
    client     *http.Client
}

func (p *AIStudioPlugin) Initialize(config map[string]interface{}) error {
    p.endpoint = config["ai_studio_endpoint"].(string)
    p.apiKey = config["api_key"].(string)
    p.gatewayID = config["gateway_id"].(string)
    
    p.client = &http.Client{
        Timeout: 30 * time.Second,
    }
    
    return nil
}

func (p *AIStudioPlugin) HandleAnalytics(ctx context.Context, req *sdk.AnalyticsData, pluginCtx *sdk.PluginContext) (*sdk.DataCollectionResponse, error) {
    payload := map[string]interface{}{
        "source":     "microgateway",
        "gateway_id": p.gatewayID,
        "log_type":   "analytics",
        "timestamp":  req.Timestamp,
        "data":       req,
    }
    
    return p.sendToAIStudio(payload)
}

func (p *AIStudioPlugin) sendToAIStudio(payload map[string]interface{}) (*sdk.DataCollectionResponse, error) {
    body, _ := json.Marshal(payload)
    
    httpReq, _ := http.NewRequest("POST", p.endpoint+"/logs", bytes.NewBuffer(body))
    httpReq.Header.Set("Content-Type", "application/json")
    httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
    httpReq.Header.Set("X-Gateway-ID", p.gatewayID)
    
    resp, err := p.client.Do(httpReq)
    if err != nil {
        return &sdk.DataCollectionResponse{
            Success: false,
            Error:   err.Error(),
        }, nil
    }
    defer resp.Body.Close()
    
    return &sdk.DataCollectionResponse{
        Success: resp.StatusCode < 300,
        Handled: true,
    }, nil
}
```

## Configuration Options

### Data Selection
```yaml
config:
  # Control what data is sent
  send_proxy_logs: true
  send_analytics: true
  send_budget_data: true
  
  # Log filtering
  include_request_body: false    # Security consideration
  include_response_body: false   # Security consideration
  include_headers: true
  
  # Application filtering
  include_apps: [1, 2, 3]        # Only send data for specific apps
  exclude_apps: [99]             # Exclude sensitive apps
  
  # LLM filtering
  include_llms: [1, 2]           # Only specific LLMs
  exclude_llms: [5]              # Exclude test LLMs
```

### Performance Configuration
```yaml
config:
  # Batching settings
  batch_size: 500
  max_batch_size: 5000
  flush_interval: "30s"
  max_flush_interval: "300s"
  
  # Buffer management
  buffer_size: 10000
  max_memory_usage: "256MB"
  
  # Concurrency
  max_concurrent_requests: 10
  worker_pool_size: 5
  
  # Compression
  compression_enabled: true
  compression_algorithm: "gzip"
```

### Reliability Configuration
```yaml
config:
  # Retry settings
  max_retries: 3
  retry_backoff: "exponential"
  initial_retry_delay: "1s"
  max_retry_delay: "60s"
  
  # Circuit breaker
  circuit_breaker:
    failure_threshold: 10
    timeout: "60s"
    half_open_max_requests: 5
    
  # Dead letter queue
  dead_letter_queue:
    enabled: true
    max_size: 50000
    persistence_path: "/var/lib/microgateway/dlq"
    retry_interval: "5m"
```

## Authentication Configuration

### API Key Authentication
```yaml
config:
  authentication:
    type: "api_key"
    api_key: "${AI_STUDIO_API_KEY}"
    header_name: "Authorization"
    header_prefix: "Bearer "
```

### OAuth2 Authentication
```yaml
config:
  authentication:
    type: "oauth2"
    client_id: "${AI_STUDIO_CLIENT_ID}"
    client_secret: "${AI_STUDIO_CLIENT_SECRET}"
    token_url: "${AI_STUDIO_TOKEN_URL}"
    scopes: ["microgateway:write"]
    token_cache_ttl: "3600s"
```

### Mutual TLS Authentication
```yaml
config:
  authentication:
    type: "mtls"
    client_cert: "/etc/ssl/certs/microgateway.crt"
    client_key: "/etc/ssl/private/microgateway.key"
    ca_cert: "/etc/ssl/certs/ai-studio-ca.crt"
```

## Data Formatting

### Standard Format
```yaml
config:
  data_format:
    type: "standard"
    include_metadata: true
    include_gateway_info: true
    timestamp_format: "RFC3339"
    
  # Field mapping
  field_mapping:
    gateway_id: "source_gateway"
    app_id: "application_id"
    llm_id: "model_provider_id"
```

### Custom Format
```yaml
config:
  data_format:
    type: "custom"
    template: |
      {
        "microgateway": {
          "instance": "{{.gateway_id}}",
          "request": {
            "id": "{{.request_id}}",
            "app": {{.app_id}},
            "provider": {{.llm_id}},
            "tokens": {{.tokens_used}},
            "cost": {{.cost}},
            "latency": {{.latency_ms}}
          }
        }
      }
```

## Monitoring Integration

### Health Monitoring
```yaml
config:
  health_monitoring:
    enabled: true
    health_check_endpoint: "/health"
    health_check_interval: "60s"
    
  # Send health data to AI Studio
  send_health_data: true
  health_data_interval: "300s"
```

### Performance Metrics
```yaml
config:
  performance_metrics:
    enabled: true
    metrics_interval: "60s"
    
  # Metrics to send
  metrics_to_send:
    - "request_rate"
    - "error_rate"
    - "average_latency"
    - "token_usage_rate"
    - "cost_rate"
    - "budget_utilization"
```

## Multi-Gateway Configuration

### Gateway Identification
```yaml
config:
  gateway_info:
    gateway_id: "${MICROGATEWAY_INSTANCE_ID}"
    environment: "${NODE_ENV:-production}"
    region: "${DEPLOYMENT_REGION:-us-west-1}"
    cluster: "${CLUSTER_NAME:-default}"
    version: "${MICROGATEWAY_VERSION}"
    
  # Additional metadata
  metadata:
    datacenter: "${DATACENTER:-primary}"
    availability_zone: "${AZ:-us-west-1a}"
    instance_type: "${INSTANCE_TYPE:-standard}"
```

### Hub-and-Spoke Integration
```yaml
config:
  hub_spoke:
    enabled: true
    mode: "${GATEWAY_MODE:-standalone}"  # standalone, control, edge
    
    # Control instance settings
    control_instance:
      control_id: "${CONTROL_INSTANCE_ID}"
      namespace: "${GATEWAY_NAMESPACE}"
      
    # Edge instance settings
    edge_instance:
      edge_id: "${EDGE_INSTANCE_ID}"
      control_endpoint: "${CONTROL_ENDPOINT}"
```

## Data Privacy and Compliance

### Data Redaction
```yaml
config:
  privacy:
    # Redact sensitive information
    redact_request_body: true
    redact_response_body: true
    redact_headers: ["authorization", "x-api-key"]
    
    # PII redaction patterns
    redaction_patterns:
      - pattern: "email"
        replacement: "[EMAIL_REDACTED]"
      - pattern: "phone"
        replacement: "[PHONE_REDACTED]"
      - pattern: "ssn"
        replacement: "[SSN_REDACTED]"
```

### Compliance Configuration
```yaml
config:
  compliance:
    # Data residency
    data_residency: "us"
    
    # Retention policies
    retention_policy:
      proxy_logs: "90days"
      analytics: "2years"
      budget_data: "7years"
      
    # Encryption requirements
    encryption_in_transit: true
    encryption_at_rest: true
    
    # Audit settings
    audit_all_requests: true
    audit_failed_requests: true
    audit_configuration_changes: true
```

## Deployment Examples

### Production Deployment
```yaml
# Production AI Studio integration
data_collection_plugins:
  - name: "ai-studio-prod"
    registry: "registry.company.com"
    repository: "plugins/ai-studio"
    digest: "sha256:prod-digest..."
    enabled: true
    replace_database: false
    config:
      ai_studio_endpoint: "https://ai-studio.company.com/api/v1/microgateway"
      api_key: "${AI_STUDIO_PROD_API_KEY}"
      gateway_id: "microgateway-${ENVIRONMENT}-${REGION}"
      
      # Production settings
      batch_size: 1000
      flush_interval: "60s"
      compression: true
      
      # Security
      tls_enabled: true
      verify_ssl: true
      
      # Data filtering
      send_full_logs: false      # Exclude request/response bodies
      send_analytics: true
      send_budget_data: true
```

### Development/Staging Deployment
```yaml
# Development AI Studio integration
data_collection_plugins:
  - name: "ai-studio-dev"
    path: "./plugins/ai_studio_dev"
    enabled: true
    replace_database: false
    config:
      ai_studio_endpoint: "https://ai-studio-dev.company.com/api/v1/microgateway"
      api_key: "${AI_STUDIO_DEV_API_KEY}"
      gateway_id: "microgateway-dev-${USER}"
      
      # Development settings
      batch_size: 10
      flush_interval: "10s"
      send_full_logs: true       # Include full logs for debugging
      debug_logging: true
```

## AI Studio API Integration

### API Endpoints
```bash
# AI Studio endpoints for microgateway data
POST /api/v1/microgateway/logs        # Proxy logs
POST /api/v1/microgateway/analytics   # Analytics data
POST /api/v1/microgateway/budget      # Budget usage
POST /api/v1/microgateway/health      # Health status
POST /api/v1/microgateway/metrics     # Performance metrics
```

### Request Format
```bash
# Send data to AI Studio
curl -X POST https://ai-studio.company.com/api/v1/microgateway/analytics \
  -H "Authorization: Bearer $AI_STUDIO_API_KEY" \
  -H "Content-Type: application/json" \
  -H "X-Gateway-ID: microgateway-prod-us-west-1" \
  -d '{
    "batch": [
      {
        "app_id": 1,
        "llm_id": 2,
        "tokens_used": 150,
        "cost": 0.045,
        "latency_ms": 1250,
        "timestamp": "2024-01-01T12:00:00Z"
      }
    ]
  }'
```

### Response Handling
```json
{
  "status": "success",
  "processed": 1,
  "errors": [],
  "next_flush": "2024-01-01T12:01:00Z"
}
```

## Advanced Integration Features

### Custom Data Enrichment
```yaml
config:
  data_enrichment:
    # Add custom fields
    custom_fields:
      environment: "${NODE_ENV}"
      region: "${AWS_REGION}"
      cluster: "${CLUSTER_NAME}"
      version: "${MICROGATEWAY_VERSION}"
      
    # Dynamic enrichment
    enrich_with_app_metadata: true
    enrich_with_user_context: true
    
    # Geographic enrichment
    geoip_enabled: true
    geoip_fields: ["country", "region", "city"]
```

### Real-Time Streaming
```yaml
config:
  streaming:
    enabled: true
    stream_endpoint: "wss://ai-studio.company.com/ws/microgateway"
    
    # Stream configuration
    stream_buffer_size: 100
    stream_flush_interval: "5s"
    reconnect_interval: "30s"
    max_reconnect_attempts: 10
    
    # Stream filtering
    stream_high_priority_events: true
    priority_thresholds:
      high_cost: 10.0          # Cost > $10
      high_latency: 30000      # Latency > 30s
      errors: true             # All errors
```

### Alerting Integration
```yaml
config:
  alerting:
    enabled: true
    alert_endpoint: "https://ai-studio.company.com/api/v1/alerts"
    
    # Alert conditions
    alert_conditions:
      budget_threshold: 0.9    # 90% budget used
      error_rate_threshold: 0.05  # 5% error rate
      latency_threshold: 10000  # 10s latency
      
    # Alert batching
    alert_batch_size: 10
    alert_flush_interval: "60s"
```

## Monitoring and Troubleshooting

### Plugin Health Monitoring
```bash
# Check AI Studio plugin health
mgw plugin health ai-studio-integration

# Monitor plugin metrics
mgw system metrics | grep ai_studio

# Check connection to AI Studio
curl -I https://ai-studio.company.com/api/v1/health
```

### Data Verification
```bash
# Verify data is reaching AI Studio
# Check AI Studio dashboard for microgateway data

# Monitor plugin success rate
mgw system metrics | grep ai_studio_success_rate

# Check for failed transmissions
grep "ai studio failed" /var/log/microgateway/plugins.log
```

### Common Issues
```bash
# Authentication issues
# Verify AI_STUDIO_API_KEY is correct
curl -H "Authorization: Bearer $AI_STUDIO_API_KEY" \
  https://ai-studio.company.com/api/v1/auth/validate

# Network connectivity issues
# Test connection to AI Studio
curl -I https://ai-studio.company.com/api/v1/health

# Rate limiting issues
# Check AI Studio rate limits and adjust batch sizes
```

## Configuration Examples

### Minimal Configuration
```yaml
data_collection_plugins:
  - name: "ai-studio-basic"
    path: "./plugins/ai_studio"
    enabled: true
    config:
      ai_studio_endpoint: "${AI_STUDIO_URL}"
      api_key: "${AI_STUDIO_API_KEY}"
      gateway_id: "${HOSTNAME}"
```

### Enterprise Configuration
```yaml
data_collection_plugins:
  - name: "ai-studio-enterprise"
    registry: "registry.company.com"
    repository: "plugins/ai-studio-enterprise"
    digest: "sha256:enterprise-digest..."
    enabled: true
    config:
      ai_studio_endpoint: "${AI_STUDIO_ENTERPRISE_URL}"
      api_key: "${AI_STUDIO_ENTERPRISE_API_KEY}"
      tenant_id: "${AI_STUDIO_TENANT_ID}"
      gateway_id: "microgateway-${ENVIRONMENT}-${REGION}-${INSTANCE_ID}"
      
      # Enterprise features
      encryption_enabled: true
      compliance_mode: true
      audit_logging: true
      
      # High availability
      endpoints:
        primary: "${AI_STUDIO_PRIMARY_URL}"
        secondary: "${AI_STUDIO_SECONDARY_URL}"
      failover_enabled: true
      
      # Performance
      batch_size: 2000
      compression: true
      connection_pooling: true
```

## Security Best Practices

### API Key Management
```bash
# Use secure secret management
# Store API keys in secret management systems
# Rotate API keys regularly
# Monitor API key usage

# Environment variable example
export AI_STUDIO_API_KEY=$(vault kv get -field=api_key secret/ai-studio)
```

### Network Security
```yaml
config:
  network_security:
    # TLS configuration
    tls_min_version: "1.2"
    verify_ssl_certificates: true
    
    # Certificate pinning
    certificate_pins:
      - "sha256:abc123def456..."
      
    # IP restrictions
    allowed_destinations:
      - "ai-studio.company.com"
      - "backup-ai-studio.company.com"
```

### Data Security
```yaml
config:
  data_security:
    # Field-level redaction
    redact_sensitive_fields: true
    sensitive_field_patterns:
      - "password"
      - "secret"
      - "key"
      - "token"
      
    # Encryption
    encrypt_payload: true
    encryption_key: "${PAYLOAD_ENCRYPTION_KEY}"
```

---

AI Studio integration provides centralized monitoring and management capabilities for distributed microgateway deployments. For data plugin development, see [Data Plugins](data-plugins.md). For configuration details, see [Data Plugin Configuration](data-plugin-config.md).
