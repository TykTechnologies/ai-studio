# External gRPC Plugin Configuration Examples

This document shows how to configure plugins to run as external gRPC microservices using the new URL-based command format.

## Plugin Command Formats

The microgateway now supports multiple plugin command formats:

- `grpc://hostname:port` - External gRPC service
- `file://path/to/plugin` - Local executable (explicit)
- `/path/to/plugin` - Local executable (backward compatible)

## Database Configuration Examples

### External gRPC Plugin

```sql
-- External authentication plugin running as microservice
INSERT INTO plugins (name, slug, command, hook_type, is_active) VALUES
('external-auth', 'external-auth', 'grpc://auth-service.company.com:8080', 'auth', true);

-- External content filter running on internal network
INSERT INTO plugins (name, slug, command, hook_type, is_active) VALUES
('content-filter', 'content-filter', 'grpc://content-filter:9090', 'pre_auth', true);

-- External analytics plugin with custom port
INSERT INTO plugins (name, slug, command, hook_type, is_active) VALUES
('analytics-plugin', 'analytics-plugin', 'grpc://10.0.1.100:8443', 'on_response', true);
```

### Local Plugins (Backward Compatible)

```sql
-- Traditional local binary (still works)
INSERT INTO plugins (name, slug, command, hook_type, is_active) VALUES
('legacy-plugin', 'legacy-plugin', './plugins/legacy-auth', 'auth', true);

-- Explicit file:// scheme for local binary
INSERT INTO plugins (name, slug, command, hook_type, is_active) VALUES
('local-filter', 'local-filter', 'file:///opt/plugins/content-filter', 'pre_auth', true);
```

## Global Data Collection Plugin Configuration

For file-based global plugins, use the same URL schemes in your `plugins.yaml`:

```yaml
version: "1.0"
data_collection_plugins:
  - name: "external-elasticsearch"
    path: "grpc://elasticsearch-plugin:8080"  # External gRPC service
    enabled: true
    replace_database: false
    hook_types:
      - "analytics"
      - "budget"
    config:
      elasticsearch_url: "http://elasticsearch:9200"

  - name: "local-collector"
    path: "file:///opt/plugins/local-collector"  # Local binary
    enabled: true
    replace_database: false
    hook_types:
      - "proxy_log"
```

## Connection Features

### Automatic Retry Logic
External gRPC plugins include automatic retry with exponential backoff:
- **Max Retries**: 3 attempts
- **Retry Delay**: 2 seconds between attempts
- **Connection Timeout**: 10 seconds per attempt
- **Health Check**: Ping test during connection

### Error Handling
- Graceful degradation when external services unavailable
- Health monitoring with automatic reconnection attempts
- Detailed logging for connection failures
- Backward compatibility for existing local plugins

### Health Monitoring
External gRPC plugins are monitored with:
- Periodic ping health checks (30-second intervals)
- Automatic restart on health check failures
- Connection retry logic on service restarts
- Status tracking for operational visibility

## Deployment Examples

### Docker Compose
```yaml
version: '3.8'
services:
  microgateway:
    image: tyk/microgateway:latest
    environment:
      - DATABASE_URL=postgres://user:pass@db:5432/mgw
    depends_on:
      - auth-plugin
      - content-filter

  auth-plugin:
    image: company/auth-plugin:v1.2.3
    ports:
      - "8080:8080"
    environment:
      - PLUGIN_PORT=8080

  content-filter:
    image: company/content-filter:v2.1.0
    ports:
      - "9090:9090"
    environment:
      - PLUGIN_PORT=9090
```

### Kubernetes
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: auth-plugin
spec:
  replicas: 3
  selector:
    matchLabels:
      app: auth-plugin
  template:
    metadata:
      labels:
        app: auth-plugin
    spec:
      containers:
      - name: auth-plugin
        image: company/auth-plugin:v1.2.3
        ports:
        - containerPort: 8080
---
apiVersion: v1
kind: Service
metadata:
  name: auth-plugin
spec:
  selector:
    app: auth-plugin
  ports:
  - port: 8080
    targetPort: 8080
```

## Benefits

1. **Microservice Architecture**: Plugins as independent scalable services
2. **Operational Flexibility**: Mix local and remote plugins as needed
3. **Resource Isolation**: External plugins don't consume microgateway resources
4. **High Availability**: Scale plugins independently with load balancers
5. **Development Workflow**: Develop plugins in any language/framework
6. **Backward Compatibility**: Existing local plugins work unchanged

## Migration Path

1. **Start Mixed**: Keep existing local plugins, add external ones
2. **Test External**: Verify external plugin connectivity and performance
3. **Gradual Migration**: Move plugins to external services over time
4. **Scale Independently**: Scale plugins based on load requirements