# Configuring Data Plugins

This guide covers detailed configuration options for data collection plugins in the microgateway.

## Overview

Data plugin configuration includes:
- **Plugin Selection**: Choose which data types each plugin handles
- **Storage Modes**: Supplement or replace database storage
- **Performance Tuning**: Buffer sizes, batch processing, flush intervals
- **External System Integration**: Connection settings and authentication
- **Error Handling**: Retry policies and failure management
- **Security Configuration**: Encryption and access controls

## Configuration Structure

### Plugin Configuration File
```yaml
# config/plugins.yaml
version: "1.0"

# Global data collection settings
data_collection:
  enabled: true
  default_timeout: "30s"
  max_concurrent_plugins: 5
  buffer_size: 1000
  flush_interval: "10s"

# Individual plugin configurations
data_collection_plugins:
  - name: "elasticsearch-collector"
    path: "./plugins/elasticsearch_collector"
    enabled: true
    priority: 100
    replace_database: false
    hook_types: ["proxy_log", "analytics", "budget"]
    config:
      # Plugin-specific configuration here
```

### Plugin-Specific Configuration

#### Elasticsearch Plugin Configuration
```yaml
data_collection_plugins:
  - name: "elasticsearch-collector"
    enabled: true
    config:
      elasticsearch_url: "${ELASTICSEARCH_URL:-http://localhost:9200}"
      username: "${ELASTICSEARCH_USERNAME}"
      password: "${ELASTICSEARCH_PASSWORD}"
      
      # Index configuration
      indices:
        proxy_logs: "microgateway-proxy-logs-${NODE_ENV:-dev}"
        analytics: "microgateway-analytics-${NODE_ENV:-dev}"
        budget: "microgateway-budget-${NODE_ENV:-dev}"
      
      # Performance settings
      batch_size: 100
      flush_interval: "30s"
      max_retries: 3
      
      # Index templates
      index_templates:
        proxy_logs:
          settings:
            number_of_shards: 1
            number_of_replicas: 1
          mappings:
            properties:
              timestamp: {"type": "date"}
              app_id: {"type": "keyword"}
              cost: {"type": "float"}
```

#### ClickHouse Plugin Configuration
```yaml
data_collection_plugins:
  - name: "clickhouse-collector"
    enabled: true
    config:
      clickhouse_url: "${CLICKHOUSE_URL:-tcp://localhost:9000}"
      username: "${CLICKHOUSE_USERNAME:-default}"
      password: "${CLICKHOUSE_PASSWORD}"
      database: "${CLICKHOUSE_DATABASE:-microgateway}"
      
      # Table configuration
      tables:
        analytics: "analytics_events"
        budget: "budget_usage"
        proxy_logs: "proxy_logs"
      
      # Performance settings
      batch_size: 1000
      flush_interval: "60s"
      compression: true
      
      # Schema configuration
      create_tables: true
      optimize_tables: true
```

#### S3 Data Lake Configuration
```yaml
data_collection_plugins:
  - name: "s3-datalake"
    enabled: true
    config:
      aws_region: "${AWS_REGION:-us-west-2}"
      bucket: "${S3_BUCKET:-microgateway-datalake}"
      
      # Authentication
      access_key_id: "${AWS_ACCESS_KEY_ID}"
      secret_access_key: "${AWS_SECRET_ACCESS_KEY}"
      
      # Partitioning strategy
      partition_by: "date"  # date, app_id, llm_id
      date_format: "year=%Y/month=%m/day=%d"
      
      # File format
      file_format: "jsonl"  # json, jsonl, parquet
      compression: "gzip"
      
      # Batching
      max_file_size: "100MB"
      max_batch_time: "10m"
```

## Data Type Configuration

### Proxy Logs Configuration
```yaml
proxy_log_config:
  include_request_body: true
  include_response_body: true
  include_headers: true
  redact_sensitive: true
  redaction_patterns:
    - "password"
    - "secret"
    - "authorization"
  max_body_size: "1MB"
  compression: true
```

### Analytics Configuration
```yaml
analytics_config:
  real_time_processing: true
  aggregation_window: "1m"
  metrics_to_collect:
    - "request_count"
    - "token_usage"
    - "cost"
    - "latency"
    - "error_rate"
  custom_dimensions:
    - "user_tier"
    - "geographic_region"
```

### Budget Configuration
```yaml
budget_config:
  track_real_time: true
  alert_thresholds:
    - 50   # 50% budget used
    - 80   # 80% budget used
    - 95   # 95% budget used
  forecast_usage: true
  cost_attribution:
    - "app_id"
    - "llm_id"
    - "user_id"
```

## Environment Variable Expansion

### Variable Substitution
```yaml
config:
  # Environment variable with default
  elasticsearch_url: "${ELASTICSEARCH_URL:-http://localhost:9200}"
  
  # Required environment variable
  api_key: "${ELASTICSEARCH_API_KEY}"
  
  # Nested variable expansion
  index_name: "microgateway-${NODE_ENV:-dev}-${CLUSTER_NAME:-local}"
  
  # Boolean environment variables
  ssl_enabled: "${ELASTICSEARCH_SSL:-false}"
  
  # Numeric environment variables
  timeout_seconds: "${ELASTICSEARCH_TIMEOUT:-30}"
```

### Environment Configuration
```bash
# Set environment variables
export ELASTICSEARCH_URL="https://elastic.company.com:9200"
export ELASTICSEARCH_USERNAME="microgateway"
export ELASTICSEARCH_PASSWORD="secure-password"
export NODE_ENV="production"
export CLUSTER_NAME="us-west-1"

# Variables are expanded when plugin configuration is loaded
```

## Plugin Priority and Execution

### Priority Configuration
```yaml
data_collection_plugins:
  # High priority - executes first
  - name: "audit-logger"
    priority: 10
    hook_types: ["proxy_log"]
    
  # Medium priority
  - name: "analytics-processor"
    priority: 50
    hook_types: ["analytics"]
    
  # Low priority - executes last
  - name: "backup-storage"
    priority: 100
    hook_types: ["proxy_log", "analytics", "budget"]
```

### Selective Data Processing
```yaml
data_collection_plugins:
  - name: "selective-collector"
    config:
      # Only process specific data types
      handle_proxy_logs: true
      handle_analytics: true
      handle_budget: false
      
      # Filter by application
      target_apps: [1, 2, 3]
      exclude_apps: [99]
      
      # Filter by LLM
      target_llms: [1, 2]
      exclude_llms: [5]
```

## Performance Configuration

### Buffer and Batch Settings
```yaml
performance_config:
  # Buffer settings
  buffer_size: 5000              # Events to buffer before processing
  max_buffer_time: "30s"         # Maximum time to buffer events
  
  # Batch processing
  batch_size: 100                # Events per batch
  max_batch_time: "10s"          # Maximum batch processing time
  
  # Concurrency
  max_concurrent_batches: 3      # Parallel batch processing
  worker_pool_size: 10           # Worker goroutines
  
  # Memory management
  max_memory_usage: "512MB"      # Memory limit per plugin
  gc_interval: "5m"              # Garbage collection interval
```

### Network Configuration
```yaml
network_config:
  # Connection settings
  connect_timeout: "10s"
  read_timeout: "30s"
  write_timeout: "30s"
  idle_timeout: "60s"
  
  # Connection pooling
  max_connections: 100
  max_idle_connections: 10
  connection_lifetime: "1h"
  
  # Retry settings
  max_retries: 3
  retry_backoff: "exponential"   # linear, exponential
  retry_delay: "1s"
  max_retry_delay: "30s"
```

## Error Handling Configuration

### Retry Policies
```yaml
error_handling:
  # Retry configuration
  retry_policy:
    max_attempts: 3
    backoff_strategy: "exponential"
    initial_delay: "1s"
    max_delay: "60s"
    
  # Failure handling
  failure_policy:
    fail_open: true              # Continue on plugin failure
    circuit_breaker:
      failure_threshold: 5       # Failures before circuit opens
      timeout: "30s"             # Circuit breaker timeout
      
  # Dead letter queue
  dead_letter_queue:
    enabled: true
    max_size: 10000
    persistence: "disk"          # memory, disk
    retry_interval: "5m"
```

### Error Logging
```yaml
logging_config:
  # Plugin logging
  log_level: "info"              # debug, info, warn, error
  log_format: "json"             # json, text
  
  # Error details
  include_stack_traces: true
  include_request_data: false    # For debugging (security risk)
  
  # Log destinations
  log_file: "/var/log/microgateway/data-plugins.log"
  log_to_console: true
```

## Security Configuration

### Authentication
```yaml
security_config:
  # TLS configuration
  tls:
    enabled: true
    cert_file: "/etc/ssl/certs/plugin.crt"
    key_file: "/etc/ssl/private/plugin.key"
    ca_file: "/etc/ssl/certs/ca.crt"
    insecure_skip_verify: false
    
  # Authentication methods
  auth:
    type: "bearer_token"         # basic, bearer_token, oauth2
    bearer_token: "${API_TOKEN}"
    
    # Basic auth
    username: "${USERNAME}"
    password: "${PASSWORD}"
    
    # OAuth2
    client_id: "${OAUTH_CLIENT_ID}"
    client_secret: "${OAUTH_CLIENT_SECRET}"
    token_url: "${OAUTH_TOKEN_URL}"
```

### Data Encryption
```yaml
encryption_config:
  # At-rest encryption
  encrypt_at_rest: true
  encryption_key: "${DATA_ENCRYPTION_KEY}"
  encryption_algorithm: "AES-256-GCM"
  
  # In-transit encryption
  encrypt_in_transit: true
  tls_min_version: "1.2"
  
  # Field-level encryption
  encrypt_fields:
    - "request_body"
    - "response_body"
    - "user_data"
```

## Configuration Examples

### Development Configuration
```yaml
# config/plugins-dev.yaml
version: "1.0"
data_collection_plugins:
  - name: "dev-logger"
    path: "./plugins/dev_logger"
    enabled: true
    replace_database: false      # Keep database for comparison
    config:
      output_format: "pretty"
      log_file: "./logs/dev-data.log"
      include_debug_info: true
      sample_rate: 1.0          # Log everything in dev
```

### Production Configuration
```yaml
# config/plugins-prod.yaml
version: "1.0"
data_collection_plugins:
  - name: "elasticsearch-prod"
    registry: "registry.company.com"
    repository: "plugins/elasticsearch"
    digest: "sha256:abc123..."
    enabled: true
    replace_database: true       # Replace database in production
    config:
      elasticsearch_url: "${ELASTICSEARCH_CLUSTER_URL}"
      username: "${ELASTICSEARCH_USER}"
      password: "${ELASTICSEARCH_PASS}"
      batch_size: 1000
      flush_interval: "60s"
      compression: true
      
  - name: "s3-archival"
    registry: "registry.company.com"
    repository: "plugins/s3-archival"
    digest: "sha256:def456..."
    enabled: true
    priority: 200
    config:
      bucket: "${S3_ARCHIVE_BUCKET}"
      retention_days: 2555      # 7 years
      compression: "gzip"
```

### Multi-Environment Configuration
```yaml
# Use environment-specific settings
data_collection_plugins:
  - name: "adaptive-collector"
    config:
      environment: "${NODE_ENV:-dev}"
      
      # Environment-specific settings
      dev_config:
        sample_rate: 1.0
        debug_logging: true
        local_storage: true
        
      staging_config:
        sample_rate: 0.5
        debug_logging: false
        external_storage: true
        
      prod_config:
        sample_rate: 1.0
        debug_logging: false
        external_storage: true
        redundancy: true
```

## Configuration Validation

### Validation Rules
```yaml
validation:
  # Required fields
  required_fields: ["name", "path", "enabled"]
  
  # Field validation
  field_validation:
    priority: 
      type: "integer"
      min: 1
      max: 1000
    batch_size:
      type: "integer"
      min: 1
      max: 10000
    flush_interval:
      type: "duration"
      min: "1s"
      max: "1h"
```

### Configuration Testing
```bash
# Validate configuration file
mgw plugin validate-config config/plugins.yaml

# Test specific plugin configuration
mgw plugin test-config elasticsearch-collector

# Dry-run configuration
./dist/microgateway --test-plugins --dry-run
```

## Dynamic Configuration

### Hot Reload
```yaml
# Enable configuration hot reload
hot_reload:
  enabled: true
  watch_interval: "30s"
  reload_on_change: true
  
# Supported changes:
# - Enable/disable plugins
# - Update plugin configuration
# - Change priority orders
# - Modify buffer/batch settings
```

### Configuration Updates
```bash
# Reload plugin configuration
curl -X POST http://localhost:8080/api/v1/plugins/reload \
  -H "Authorization: Bearer $ADMIN_TOKEN"

# Reload specific plugin
curl -X POST http://localhost:8080/api/v1/plugins/elasticsearch-collector/reload \
  -H "Authorization: Bearer $ADMIN_TOKEN"

# Check reload status
mgw plugin status elasticsearch-collector
```

## Configuration Sources

### File-Based Configuration
```yaml
# Local configuration file
config_source:
  type: "file"
  path: "./config/plugins.yaml"
  watch: true
  reload_interval: "30s"
```

### HTTP Service Configuration
```yaml
# Remote configuration service
config_source:
  type: "http"
  url: "${CONFIG_SERVICE_URL}/api/plugins"
  authentication:
    type: "bearer_token"
    token: "${CONFIG_SERVICE_TOKEN}"
  poll_interval: "5m"
  cache_enabled: true
  cache_ttl: "10m"
```

### Database Configuration
```yaml
# Database-stored configuration
config_source:
  type: "database"
  table: "plugin_configurations"
  poll_interval: "2m"
  cache_enabled: true
```

## Configuration Best Practices

### Environment Management
```bash
# Use environment-specific configuration files
config/
├── plugins-dev.yaml
├── plugins-staging.yaml
└── plugins-prod.yaml

# Select configuration based on environment
PLUGINS_CONFIG_PATH=./config/plugins-${NODE_ENV}.yaml
```

### Secret Management
```yaml
# Use environment variables for secrets
config:
  api_key: "${SECRET_API_KEY}"           # From environment
  password: "${VAULT_PASSWORD}"          # From secret manager
  
# Never commit secrets to configuration files
# Use external secret management systems
```

### Configuration Versioning
```yaml
# Version configuration for rollback
version: "1.2"
metadata:
  created_by: "ops-team"
  created_at: "2024-01-01T00:00:00Z"
  description: "Production data collection setup"
  
# Track configuration changes
# Implement approval workflows for production changes
```

### Performance Tuning
```yaml
# High-throughput configuration
performance_tuning:
  high_volume:
    batch_size: 5000
    flush_interval: "30s"
    max_concurrent_batches: 10
    buffer_size: 50000
    
  low_latency:
    batch_size: 10
    flush_interval: "1s"
    real_time_processing: true
    
  balanced:
    batch_size: 500
    flush_interval: "10s"
    max_concurrent_batches: 3
```

## Monitoring Configuration

### Health Check Configuration
```yaml
health_checks:
  enabled: true
  interval: "30s"
  timeout: "10s"
  failure_threshold: 3
  
  # Plugin-specific health checks
  custom_checks:
    elasticsearch:
      endpoint: "/_cluster/health"
      expected_status: "green"
    clickhouse:
      query: "SELECT 1"
      timeout: "5s"
```

### Metrics Configuration
```yaml
metrics:
  enabled: true
  prefix: "microgateway_data_collection"
  labels:
    - "plugin_name"
    - "data_type"
    - "destination"
  
  # Custom metrics
  custom_metrics:
    - name: "batch_processing_duration"
      type: "histogram"
      buckets: [0.1, 0.5, 1.0, 5.0, 10.0]
```

## Troubleshooting Configuration

### Common Configuration Issues

#### Invalid YAML Syntax
```bash
# Validate YAML syntax
yamllint config/plugins.yaml

# Check for parsing errors
./dist/microgateway --test-config
```

#### Missing Environment Variables
```bash
# Check environment variable expansion
envsubst < config/plugins.yaml

# Validate required variables
./dist/microgateway --validate-env
```

#### Plugin Path Issues
```bash
# Verify plugin binary exists
ls -la plugins/

# Check executable permissions
file plugins/my_plugin
chmod +x plugins/my_plugin
```

### Configuration Debugging
```bash
# Enable configuration debugging
LOG_LEVEL=debug ./dist/microgateway

# Dump effective configuration
mgw plugin config elasticsearch-collector

# Test configuration without starting service
./dist/microgateway --test-config --verbose
```

## Configuration Migration

### Migrating from Database-Only
```yaml
# Phase 1: Add plugin in supplement mode
data_collection_plugins:
  - name: "new-backend"
    enabled: true
    replace_database: false  # Supplement existing database
    
# Monitor and validate data consistency
# Compare plugin output with database data

# Phase 2: Switch to replace mode
replace_database: true       # Replace database storage
```

### Configuration Updates
```bash
# Safe configuration update procedure
# 1. Backup current configuration
cp config/plugins.yaml config/plugins.yaml.backup

# 2. Update configuration
# Edit config/plugins.yaml

# 3. Validate new configuration
./dist/microgateway --test-config

# 4. Reload plugins
mgw plugin reload

# 5. Monitor for issues
mgw plugin health --all
```

## Advanced Configuration

### Conditional Plugin Loading
```yaml
data_collection_plugins:
  - name: "conditional-plugin"
    enabled: "${ENABLE_ADVANCED_ANALYTICS:-false}"
    condition:
      environment: ["production", "staging"]
      app_ids: [1, 2, 3]
      feature_flags: ["advanced_analytics"]
```

### Plugin Composition
```yaml
# Compose multiple plugins for complex workflows
data_collection_plugins:
  - name: "preprocessor"
    priority: 10
    config:
      transform_data: true
      add_metadata: true
      
  - name: "router"
    priority: 20
    config:
      routing_rules:
        high_cost: "premium_storage"
        standard: "standard_storage"
        
  - name: "storage-backend"
    priority: 30
    config:
      backends: ["premium_storage", "standard_storage"]
```

---

Proper configuration ensures optimal data plugin performance and reliability. For specific plugin examples, see [Data Plugins](data-plugins.md). For AI Studio integration, see [AI Studio Integration](ai-studio-logs.md).
