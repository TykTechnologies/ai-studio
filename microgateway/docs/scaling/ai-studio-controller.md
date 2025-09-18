# Hub and Spoke AI Studio Controller to Microgateway Edge

This guide covers the integration between AI Studio as a central controller and distributed microgateway edge instances.

## Overview

AI Studio controller integration features:
- **Centralized AI Management**: AI Studio manages multiple microgateway instances
- **Multi-Gateway Orchestration**: Coordinate configuration across gateway fleets
- **Unified Analytics**: Aggregate analytics from all gateway instances
- **Global Policy Management**: Consistent policies across all gateways
- **Cost Management**: Centralized budget and cost control
- **Fleet Monitoring**: Monitor health and performance of gateway fleet

## Architecture Components

### AI Studio as Central Controller
```
        ┌─────────────────────┐
        │    AI Studio        │
        │  (Central Control)  │
        └──────────┬──────────┘
                   │
        ┌──────────┼──────────┐
        │          │          │
   ┌────▼───┐ ┌───▼───┐ ┌────▼───┐
   │Gateway │ │Gateway│ │Gateway │
   │Edge 1  │ │Edge 2 │ │Edge 3  │
   └────────┘ └───────┘ └────────┘
```

### Communication Layers
- **Management API**: RESTful API for configuration management
- **Analytics Streaming**: Real-time analytics data from gateways to AI Studio
- **Policy Distribution**: Configuration and policy propagation
- **Health Monitoring**: Gateway fleet health monitoring

## AI Studio Controller Configuration

### Gateway Fleet Management
```yaml
# AI Studio configuration for gateway management
gateway_fleet:
  management_endpoint: "https://ai-studio.company.com/api/v1/gateways"
  api_key: "${AI_STUDIO_API_KEY}"
  
  # Fleet configuration
  default_configuration:
    budget_enforcement: true
    analytics_enabled: true
    rate_limiting: true
    
  # Gateway registration
  auto_registration: true
  registration_token: "${GATEWAY_REGISTRATION_TOKEN}"
  
  # Policy distribution
  policy_sync_interval: "300s"
  config_sync_mode: "push"  # push, pull
```

### Global Policies
```yaml
# AI Studio global policies
global_policies:
  # Budget policies
  budget_policy:
    default_monthly_budget: 1000.0
    budget_reset_day: 1
    overage_protection: true
    
  # Security policies
  security_policy:
    require_tls: true
    token_expiry_max: "720h"
    ip_whitelisting_required: false
    
  # Performance policies
  performance_policy:
    max_request_timeout: "60s"
    rate_limit_default: 100
    analytics_retention_days: 90
```

## Microgateway Edge Integration

### AI Studio Integration Configuration
```bash
# Environment variables for AI Studio integration
AI_STUDIO_CONTROLLER_ENABLED=true
AI_STUDIO_CONTROLLER_URL=https://ai-studio.company.com
AI_STUDIO_API_KEY=your-ai-studio-api-key
GATEWAY_INSTANCE_ID=gateway-region-1

# Registration settings
GATEWAY_AUTO_REGISTER=true
GATEWAY_REGISTRATION_TOKEN=registration-token
GATEWAY_HEARTBEAT_INTERVAL=60s

# Policy synchronization
POLICY_SYNC_ENABLED=true
POLICY_SYNC_INTERVAL=300s
POLICY_OVERRIDE_LOCAL=false  # Allow local overrides
```

### Gateway Registration Process
```
1. Gateway starts with AI Studio integration enabled
2. Gateway calls AI Studio registration API
3. AI Studio validates registration token
4. AI Studio assigns gateway ID and namespace
5. AI Studio sends initial configuration and policies
6. Gateway applies configuration and starts operation
7. Gateway begins sending analytics to AI Studio
8. Gateway polls for policy updates
```

## Configuration Synchronization

### AI Studio to Gateway Sync
```json
{
  "gateway_id": "gateway-region-1",
  "namespace": "production",
  "configuration": {
    "llms": [
      {
        "name": "Global GPT-4",
        "vendor": "openai",
        "model": "gpt-4",
        "budget": 2000.0
      }
    ],
    "apps": [
      {
        "name": "Production App",
        "budget": 1000.0,
        "rate_limit": 500
      }
    ],
    "policies": {
      "budget_enforcement": true,
      "rate_limiting": true
    }
  },
  "version": "1.2.3",
  "timestamp": "2024-01-01T12:00:00Z"
}
```

### Policy Distribution
```bash
# AI Studio pushes policies to all gateways
curl -X POST https://ai-studio.company.com/api/v1/gateways/policies/distribute \
  -H "Authorization: Bearer $AI_STUDIO_API_KEY" \
  -d '{
    "policy_type": "budget",
    "policy_data": {
      "default_budget": 1500.0,
      "enforcement_enabled": true
    },
    "target_gateways": ["gateway-region-1", "gateway-region-2"],
    "effective_date": "2024-01-01T00:00:00Z"
  }'
```

## Analytics Integration

### Gateway to AI Studio Analytics
```json
{
  "source_gateway": "gateway-region-1",
  "namespace": "production",
  "analytics_batch": [
    {
      "app_id": 1,
      "llm_id": 2,
      "tokens_used": 150,
      "cost": 0.045,
      "latency_ms": 1250,
      "timestamp": "2024-01-01T12:00:00Z",
      "request_id": "req_abc123"
    }
  ],
  "batch_metadata": {
    "batch_size": 100,
    "time_window": "60s",
    "gateway_version": "v1.0.0"
  }
}
```

### Real-Time Streaming
```yaml
# Real-time analytics streaming configuration
analytics_streaming:
  enabled: true
  stream_endpoint: "wss://ai-studio.company.com/ws/gateway-analytics"
  
  # Stream settings
  buffer_size: 1000
  flush_interval: "30s"
  compression: true
  
  # Reconnection
  reconnect_interval: "30s"
  max_reconnect_attempts: 10
  
  # Data filtering
  include_request_bodies: false
  include_response_bodies: false
  include_metadata: true
```

## Fleet Management

### Gateway Registration API
```bash
# Register gateway with AI Studio
curl -X POST https://ai-studio.company.com/api/v1/gateways/register \
  -H "Authorization: Bearer $REGISTRATION_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "gateway_id": "gateway-region-1",
    "environment": "production",
    "region": "us-west-1",
    "version": "v1.0.0",
    "capabilities": ["llm_proxy", "analytics", "budgets"],
    "metadata": {
      "deployment_type": "kubernetes",
      "cluster": "prod-cluster"
    }
  }'
```

### Fleet Monitoring
```bash
# List all registered gateways
curl https://ai-studio.company.com/api/v1/gateways \
  -H "Authorization: Bearer $AI_STUDIO_API_KEY"

# Get gateway status
curl https://ai-studio.company.com/api/v1/gateways/gateway-region-1/status \
  -H "Authorization: Bearer $AI_STUDIO_API_KEY"

# Gateway fleet metrics
curl https://ai-studio.company.com/api/v1/gateways/fleet/metrics \
  -H "Authorization: Bearer $AI_STUDIO_API_KEY"
```

## Configuration Management

### Centralized LLM Management
```bash
# AI Studio manages LLM configurations for all gateways
# Create LLM configuration in AI Studio
curl -X POST https://ai-studio.company.com/api/v1/gateway-configs/llms \
  -H "Authorization: Bearer $AI_STUDIO_API_KEY" \
  -d '{
    "name": "Enterprise GPT-4",
    "vendor": "openai",
    "model": "gpt-4",
    "api_key": "sk-...",
    "budget": 5000.0,
    "target_gateways": ["gateway-region-1", "gateway-region-2"],
    "namespaces": ["production"]
  }'

# Configuration automatically distributed to target gateways
```

### Application Management
```bash
# Create application across multiple gateways
curl -X POST https://ai-studio.company.com/api/v1/gateway-configs/apps \
  -H "Authorization: Bearer $AI_STUDIO_API_KEY" \
  -d '{
    "name": "Multi-Region App",
    "owner_email": "ops@company.com",
    "budget": 2000.0,
    "target_gateways": ["gateway-us-west", "gateway-us-east"],
    "llm_associations": ["enterprise-gpt-4", "claude-sonnet"]
  }'
```

### Policy Management
```bash
# Deploy security policy to all gateways
curl -X POST https://ai-studio.company.com/api/v1/gateway-policies \
  -H "Authorization: Bearer $AI_STUDIO_API_KEY" \
  -d '{
    "policy_name": "enterprise-security",
    "policy_type": "security",
    "policy_data": {
      "require_ip_whitelisting": true,
      "max_token_lifetime": "24h",
      "enforce_tls": true
    },
    "target_gateways": "all",
    "effective_immediately": true
  }'
```

## Multi-Region Deployment

### Regional Gateway Configuration
```yaml
# AI Studio regional gateway setup
regional_gateways:
  us_west:
    gateway_id: "gateway-us-west-1"
    region: "us-west-1"
    endpoint: "https://gateway-us-west.company.com"
    namespace: "production"
    
  us_east:
    gateway_id: "gateway-us-east-1"
    region: "us-east-1"
    endpoint: "https://gateway-us-east.company.com"
    namespace: "production"
    
  eu_west:
    gateway_id: "gateway-eu-west-1"
    region: "eu-west-1"
    endpoint: "https://gateway-eu-west.company.com"
    namespace: "production-eu"
```

### Traffic Routing
```yaml
# AI Studio traffic routing rules
traffic_routing:
  routing_strategy: "geographic"  # geographic, load_based, latency_based
  
  geographic_rules:
    - source_regions: ["us-west-1", "us-west-2"]
      target_gateway: "gateway-us-west-1"
    - source_regions: ["us-east-1", "us-east-2"]
      target_gateway: "gateway-us-east-1"
    - source_regions: ["eu-west-1", "eu-central-1"]
      target_gateway: "gateway-eu-west-1"
      
  failover_rules:
    - primary: "gateway-us-west-1"
      backup: "gateway-us-east-1"
      health_check_interval: "30s"
```

## Monitoring and Analytics

### Fleet Analytics
```bash
# AI Studio aggregates analytics from all gateways
# Provides unified view of:
# - Total token usage across all gateways
# - Cost breakdown by region and application
# - Performance metrics across the fleet
# - Error rates and patterns
# - Usage trends and forecasting

# Example fleet analytics API
curl https://ai-studio.company.com/api/v1/analytics/fleet \
  -H "Authorization: Bearer $AI_STUDIO_API_KEY"
```

### Performance Monitoring
```json
{
  "fleet_metrics": {
    "total_gateways": 15,
    "active_gateways": 14,
    "total_requests_per_hour": 50000,
    "average_latency_ms": 850,
    "error_rate": 0.02,
    "total_cost_per_day": 2500.0
  },
  "regional_breakdown": {
    "us-west": {
      "gateways": 5,
      "requests_per_hour": 20000,
      "average_latency_ms": 750
    },
    "us-east": {
      "gateways": 5,
      "requests_per_hour": 18000,
      "average_latency_ms": 800
    },
    "eu-west": {
      "gateways": 4,
      "requests_per_hour": 12000,
      "average_latency_ms": 950
    }
  }
}
```

## Configuration Distribution

### Push-Based Configuration
```bash
# AI Studio pushes configuration to gateways
# Immediate distribution of configuration changes
# Real-time policy updates
# Centralized change management

# AI Studio tracks configuration state per gateway
# Ensures consistency across all instances
```

### Pull-Based Configuration
```bash
# Gateways poll AI Studio for configuration updates
# Configurable poll intervals
# Efficient change detection
# Network-friendly for large fleets

# Gateway configuration
AI_STUDIO_POLL_INTERVAL=300s
AI_STUDIO_CONFIG_ENDPOINT=https://ai-studio.company.com/api/v1/gateway-config
```

## Multi-Tenant Management

### Tenant-Specific Gateways
```yaml
# AI Studio tenant configuration
tenants:
  tenant_a:
    dedicated_gateways: ["gateway-tenant-a-1", "gateway-tenant-a-2"]
    namespace: "tenant-a"
    budget_limit: 10000.0
    allowed_regions: ["us-west", "us-east"]
    
  tenant_b:
    shared_gateways: true  # Use shared gateway pool
    namespace: "tenant-b"
    budget_limit: 5000.0
    allowed_regions: ["us-west"]
```

### Cross-Tenant Isolation
```bash
# AI Studio ensures tenant isolation
# - Separate namespaces per tenant
# - Isolated analytics and billing
# - Independent policy management
# - Secure credential separation
```

## Gateway Lifecycle Management

### Gateway Provisioning
```bash
# AI Studio API for gateway provisioning
curl -X POST https://ai-studio.company.com/api/v1/gateways/provision \
  -H "Authorization: Bearer $AI_STUDIO_API_KEY" \
  -d '{
    "gateway_template": "standard",
    "region": "us-west-1",
    "namespace": "production",
    "initial_config": {
      "llms": ["gpt-4", "claude-sonnet"],
      "budget": 2000.0
    }
  }'

# AI Studio orchestrates gateway deployment
# Configures infrastructure and networking
# Deploys gateway with initial configuration
```

### Gateway Updates
```bash
# AI Studio manages gateway updates
# Rolling updates with zero downtime
# Configuration validation before deployment
# Automatic rollback on failure

# Update fleet configuration
curl -X PUT https://ai-studio.company.com/api/v1/gateways/fleet/config \
  -H "Authorization: Bearer $AI_STUDIO_API_KEY" \
  -d '{
    "update_strategy": "rolling",
    "max_unavailable": "25%",
    "configuration": {
      "budget_enforcement": true,
      "new_feature_enabled": true
    }
  }'
```

### Gateway Decommissioning
```bash
# Graceful gateway shutdown
curl -X DELETE https://ai-studio.company.com/api/v1/gateways/gateway-region-1 \
  -H "Authorization: Bearer $AI_STUDIO_API_KEY" \
  -d '{
    "drain_timeout": "300s",
    "migrate_traffic": true,
    "target_gateways": ["gateway-region-2", "gateway-region-3"]
  }'
```

## Cost Management Integration

### Centralized Billing
```yaml
# AI Studio centralizes billing across all gateways
billing_integration:
  provider: "stripe"  # stripe, billing_service
  api_key: "${BILLING_API_KEY}"
  
  # Cost aggregation
  aggregate_by: ["tenant", "region", "application"]
  billing_cycle: "monthly"
  currency: "USD"
  
  # Budget alerts
  budget_alerts:
    - threshold: 80
      notification: "email"
    - threshold: 95
      notification: "slack"
    - threshold: 100
      action: "suspend"
```

### Multi-Gateway Cost Attribution
```bash
# AI Studio aggregates costs from all gateways
# Provides unified billing view
# Tracks cost per tenant, application, region
# Manages budgets across gateway fleet

# Example cost API
curl https://ai-studio.company.com/api/v1/billing/costs \
  -H "Authorization: Bearer $AI_STUDIO_API_KEY" \
  -d '{
    "time_range": {
      "start": "2024-01-01T00:00:00Z",
      "end": "2024-01-31T23:59:59Z"
    },
    "group_by": ["tenant", "region", "llm_provider"]
  }'
```

## Security and Compliance

### Gateway Authentication
```yaml
# AI Studio manages gateway authentication
gateway_auth:
  # Gateway registration tokens
  registration_tokens:
    - token: "reg-token-1"
      allowed_regions: ["us-west", "us-east"]
      expires_at: "2024-12-31T23:59:59Z"
      
  # API keys for ongoing communication
  api_keys:
    rotate_interval: "30d"
    key_length: 64
    encryption: "AES-256"
```

### Compliance Management
```yaml
# AI Studio enforces compliance policies
compliance:
  # Data residency
  data_residency_rules:
    - tenant: "eu-tenant"
      allowed_regions: ["eu-west-1", "eu-central-1"]
      data_classification: "gdpr"
      
  # Audit requirements
  audit_policies:
    log_all_requests: true
    retain_logs_days: 2555  # 7 years
    audit_configuration_changes: true
    
  # Security policies
  security_requirements:
    tls_required: true
    token_rotation_days: 90
    ip_whitelisting_enforced: true
```

## Integration Examples

### Basic AI Studio Integration
```bash
# Start gateway with AI Studio integration
AI_STUDIO_CONTROLLER_ENABLED=true \
AI_STUDIO_CONTROLLER_URL=https://ai-studio.company.com \
AI_STUDIO_API_KEY=your-api-key \
GATEWAY_INSTANCE_ID=gateway-1 \
./microgateway
```

### Enterprise Integration
```yaml
# Enterprise AI Studio integration
ai_studio_integration:
  enabled: true
  controller_url: "https://ai-studio.enterprise.com"
  tenant_id: "enterprise-tenant-1"
  
  # Authentication
  authentication:
    type: "oauth2"
    client_id: "${AI_STUDIO_CLIENT_ID}"
    client_secret: "${AI_STUDIO_CLIENT_SECRET}"
    token_url: "https://auth.enterprise.com/oauth/token"
    
  # Configuration
  configuration:
    sync_mode: "hybrid"  # push + pull
    sync_interval: "60s"
    batch_size: 1000
    
  # Analytics
  analytics:
    real_time_streaming: true
    batch_upload: true
    retention_days: 365
    
  # Compliance
  compliance_mode: true
  data_classification: "confidential"
  encryption_required: true
```

## Monitoring and Operations

### Fleet Health Monitoring
```bash
# AI Studio monitors gateway fleet health
# Tracks gateway availability, performance, errors
# Provides alerts for gateway issues
# Manages automatic failover and recovery

# Fleet health API
curl https://ai-studio.company.com/api/v1/gateways/fleet/health \
  -H "Authorization: Bearer $AI_STUDIO_API_KEY"
```

### Operational Dashboards
```json
{
  "fleet_status": {
    "total_gateways": 20,
    "healthy_gateways": 19,
    "degraded_gateways": 1,
    "failed_gateways": 0
  },
  "performance_metrics": {
    "total_requests_per_minute": 5000,
    "average_response_time_ms": 850,
    "error_rate_percentage": 0.1,
    "cost_per_hour": 125.50
  },
  "alerts": [
    {
      "gateway_id": "gateway-region-3",
      "severity": "warning",
      "message": "High latency detected",
      "timestamp": "2024-01-01T12:00:00Z"
    }
  ]
}
```

## Best Practices

### AI Studio Configuration
- **Centralized Policy Management**: Use AI Studio for consistent policies
- **Gradual Rollouts**: Test configuration changes on subset of gateways first
- **Monitoring Integration**: Comprehensive monitoring of gateway fleet
- **Backup Strategies**: Regular backup of AI Studio configuration

### Gateway Integration
- **Heartbeat Monitoring**: Regular health checks with AI Studio
- **Local Caching**: Cache configuration for offline operation
- **Graceful Degradation**: Continue operation if AI Studio unavailable
- **Security Best Practices**: Secure communication and authentication

### Fleet Management
- **Capacity Planning**: Monitor usage patterns for scaling decisions
- **Performance Optimization**: Optimize based on fleet-wide metrics
- **Cost Optimization**: Use AI Studio analytics for cost optimization
- **Disaster Recovery**: Plan for AI Studio and gateway failures

## Troubleshooting

### Integration Issues
```bash
# Test AI Studio connectivity
curl -I https://ai-studio.company.com/api/v1/health

# Verify authentication
curl -H "Authorization: Bearer $AI_STUDIO_API_KEY" \
  https://ai-studio.company.com/api/v1/auth/validate

# Check gateway registration
curl https://ai-studio.company.com/api/v1/gateways/gateway-region-1 \
  -H "Authorization: Bearer $AI_STUDIO_API_KEY"
```

### Configuration Sync Issues
```bash
# Force configuration sync
curl -X POST https://ai-studio.company.com/api/v1/gateways/gateway-region-1/sync \
  -H "Authorization: Bearer $AI_STUDIO_API_KEY"

# Check configuration version
mgw system config | grep ai_studio_config_version

# Compare local vs AI Studio configuration
diff <(mgw llm list --format=json) \
     <(curl -s https://ai-studio.company.com/api/v1/gateways/gateway-region-1/config)
```

---

AI Studio controller integration enables enterprise-scale management of microgateway fleets. For distributed architecture, see [Hub-and-Spoke Overview](hub-spoke-overview.md). For namespace management, see [Namespaces](namespaces.md).
