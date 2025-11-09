# Plugin Deployment Options

Tyk AI Studio supports three plugin deployment methods: local filesystem (`file://`), remote gRPC (`grpc://`), and OCI registry (`oci://`). Choose the deployment method based on your environment and requirements.

## Deployment Methods Comparison

| Method | Use Case | Pros | Cons |
|--------|----------|------|------|
| `file://` | Development, testing | Fast, simple, easy debugging | Not suitable for production, requires filesystem access |
| `grpc://` | Production, distributed systems | Remote deployment, scalable | Requires network setup, more complex |
| `oci://` | Production, containerized | Version control, registry management | Requires OCI registry, packaging overhead |

## file:// - Local Filesystem

Deploy plugins from the local filesystem.

### Building Your Plugin

```bash
# Build for current platform
go build -o my-plugin main.go

# Build for Linux (Docker/K8s deployment)
GOOS=linux GOARCH=amd64 go build -o my-plugin-linux main.go

# Make executable
chmod +x my-plugin
```

### Creating Plugin via API

```bash
curl -X POST http://localhost:3000/api/v1/plugins \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "My Plugin",
    "slug": "my-plugin",
    "command": "file:///absolute/path/to/my-plugin",
    "hook_type": "pre_auth",
    "plugin_type": "gateway",
    "is_active": true
  }'
```

**Important**: Use absolute paths with `file://`:

```bash
✅ file:///usr/local/bin/my-plugin
✅ file:///home/user/plugins/my-plugin
❌ file://./my-plugin  # Relative paths not supported
❌ /usr/local/bin/my-plugin  # Missing file:// prefix
```

### Docker Deployment

When deploying with Docker, mount plugins into the container:

```yaml
# docker-compose.yml
services:
  ai-studio:
    image: tykio/ai-studio:latest
    volumes:
      - ./plugins:/plugins
    environment:
      - ALLOW_INTERNAL_NETWORK_ACCESS=true  # For development only
```

Then register with container path:

```bash
curl -X POST http://localhost:3000/api/v1/plugins \
  -d '{"command": "file:///plugins/my-plugin", ...}'
```

### Kubernetes Deployment

Mount plugins via ConfigMap or PersistentVolume:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: plugins
binaryData:
  my-plugin: <base64-encoded-binary>
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ai-studio
spec:
  template:
    spec:
      containers:
      - name: ai-studio
        image: tykio/ai-studio:latest
        volumeMounts:
        - name: plugins
          mountPath: /plugins
      volumes:
      - name: plugins
        configMap:
          name: plugins
          defaultMode: 0755
```

## grpc:// - Remote gRPC

Deploy plugins as remote gRPC services.

### Running Plugin as gRPC Server

Your plugin already implements gRPC via go-plugin. To run it as a remote service, you need a gRPC wrapper:

```go
package main

import (
    "log"
    "net"

    "github.com/TykTechnologies/midsommar/v2/pkg/ai_studio_sdk"
    "google.golang.org/grpc"
)

func main() {
    // Create gRPC server
    lis, err := net.Listen("tcp", ":50051")
    if err != nil {
        log.Fatalf("Failed to listen: %v", err)
    }

    grpcServer := grpc.NewServer()

    // Register your plugin
    plugin := &MyPlugin{}
    ai_studio_sdk.RegisterPluginServer(grpcServer, plugin)

    log.Printf("Plugin gRPC server listening on :50051")
    if err := grpcServer.Serve(lis); err != nil {
        log.Fatalf("Failed to serve: %v", err)
    }
}
```

### Deploying with Docker

```dockerfile
FROM golang:1.21-alpine AS builder
WORKDIR /build
COPY . .
RUN go build -o plugin-server main.go

FROM alpine:latest
COPY --from=builder /build/plugin-server /usr/local/bin/
EXPOSE 50051
CMD ["/usr/local/bin/plugin-server"]
```

```bash
# Build and run
docker build -t my-plugin-server .
docker run -d -p 50051:50051 my-plugin-server
```

### Register Remote Plugin

```bash
curl -X POST http://localhost:3000/api/v1/plugins \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "name": "My Remote Plugin",
    "slug": "my-remote-plugin",
    "command": "grpc://plugin-server:50051",
    "hook_type": "pre_auth",
    "plugin_type": "gateway",
    "is_active": true
  }'
```

### Network Configuration

#### Internal Network Access

By default, plugins cannot access internal networks. For development:

```bash
export ALLOW_INTERNAL_NETWORK_ACCESS=true
```

For production, use allowlist:

```bash
export PLUGIN_COMMAND_ALLOWLIST="grpc://10.0.0.0/8,grpc://172.16.0.0/12"
```

#### Load Balancing

Use Kubernetes services for load balancing:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: my-plugin
spec:
  selector:
    app: my-plugin
  ports:
  - port: 50051
    targetPort: 50051
  type: ClusterIP
```

Register with service DNS:

```bash
"command": "grpc://my-plugin.default.svc.cluster.local:50051"
```

## oci:// - OCI Registry

Deploy plugins as OCI artifacts in container registries.

### Prerequisites

- Docker or Podman
- OCI-compatible registry (Docker Hub, GHCR, ECR, GCR, Harbor, etc.)
- Registry credentials configured

### Packaging Plugin as OCI Artifact

Create a simple OCI image:

```dockerfile
FROM scratch
COPY my-plugin /plugin
ENTRYPOINT ["/plugin"]
```

Build and push:

```bash
# Build plugin binary
go build -o my-plugin main.go

# Build OCI image
docker build -t registry.example.com/plugins/my-plugin:v1.0.0 .

# Push to registry
docker push registry.example.com/plugins/my-plugin:v1.0.0
```

### Registering OCI Plugin

```bash
curl -X POST http://localhost:3000/api/v1/plugins \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "name": "My OCI Plugin",
    "slug": "my-oci-plugin",
    "command": "oci://registry.example.com/plugins/my-plugin:v1.0.0",
    "oci_reference": "registry.example.com/plugins/my-plugin:v1.0.0",
    "hook_type": "pre_auth",
    "plugin_type": "gateway",
    "is_active": true
  }'
```

### Registry Authentication

Configure registry credentials via environment variables:

```bash
# Docker Hub
export OCI_REGISTRY_USERNAME=myusername
export OCI_REGISTRY_PASSWORD=mypassword

# GitHub Container Registry
export OCI_REGISTRY_URL=ghcr.io
export OCI_REGISTRY_USERNAME=github_username
export OCI_REGISTRY_TOKEN=ghp_xxxxxxxxxxxxx

# AWS ECR
export OCI_REGISTRY_URL=123456789.dkr.ecr.us-east-1.amazonaws.com
export AWS_ACCESS_KEY_ID=xxxxx
export AWS_SECRET_ACCESS_KEY=xxxxx
export AWS_REGION=us-east-1
```

Or configure via Kubernetes secret:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: oci-registry-creds
type: kubernetes.io/dockerconfigjson
data:
  .dockerconfigjson: <base64-encoded-docker-config>
```

### Version Management

Use tags for version management:

```bash
# Development
oci://registry.example.com/plugins/my-plugin:latest

# Staging
oci://registry.example.com/plugins/my-plugin:v1.2.3-rc.1

# Production
oci://registry.example.com/plugins/my-plugin:v1.2.3

# Immutable digest
oci://registry.example.com/plugins/my-plugin@sha256:abc123...
```

### Caching

OCI plugins are pulled and cached locally. Configure cache settings:

```bash
export OCI_PLUGIN_CACHE_DIR=/var/cache/ai-studio/plugins
export OCI_PLUGIN_CACHE_TTL=3600  # Cache for 1 hour
```

## Security Considerations

### Command Validation

The platform validates plugin commands for security:

1. **Absolute Paths**: `file://` commands must use absolute paths
2. **Internal Network Block**: By default, `grpc://` commands cannot target internal IPs (10.x.x.x, 172.16-31.x.x, 192.168.x.x, 127.x.x.x, localhost)
3. **Allowlist**: Configure allowed commands via `PLUGIN_COMMAND_ALLOWLIST`

Example warnings:

```
⚠️  PLUGIN SECURITY WARNING: Plugin command uses absolute path outside standard directories.
⚠️  PLUGIN SECURITY WARNING: Plugin command targets internal network address
```

### Production Security

For production deployments:

1. **Disable Internal Network Access**:
   ```bash
   export ALLOW_INTERNAL_NETWORK_ACCESS=false
   export PLUGIN_BLOCK_INTERNAL_URLS=true
   ```

2. **Use Allowlist**:
   ```bash
   export PLUGIN_COMMAND_ALLOWLIST="/usr/local/plugins/*,grpc://plugins.prod.svc/*"
   ```

3. **Use OCI with Signed Images**:
   - Sign images with Cosign or Notary
   - Verify signatures before deployment
   - Use content trust

4. **Principle of Least Privilege**:
   - Run plugins with minimal permissions
   - Use read-only filesystems where possible
   - Implement network policies in Kubernetes

## Troubleshooting

### Plugin Not Loading

**Symptoms**: Plugin shows as inactive, errors in logs

**Solutions**:
- Verify plugin binary has execute permissions (`chmod +x`)
- Check absolute path is correct for `file://`
- Verify network connectivity for `grpc://`
- Check registry authentication for `oci://`
- Review plugin logs for initialization errors

### Permission Denied

**Symptoms**: "Permission denied" error when loading plugin

**Solutions**:
```bash
# Check file permissions
ls -la /path/to/plugin

# Make executable
chmod +x /path/to/plugin

# Check SELinux context (if applicable)
chcon -t container_file_t /path/to/plugin
```

### Network Connection Errors

**Symptoms**: "connection refused", "no route to host" for `grpc://`

**Solutions**:
- Verify plugin server is running: `telnet plugin-host 50051`
- Check firewall rules
- Verify Kubernetes service is created
- Check DNS resolution
- Enable internal network access if needed (development only)

### OCI Pull Failures

**Symptoms**: "Failed to pull image", "authentication required"

**Solutions**:
- Verify registry URL is correct
- Check credentials are configured
- Test manual pull: `docker pull registry.example.com/plugins/my-plugin:v1.0.0`
- Check registry permissions
- Verify image exists with correct tag

### Plugin Crashes on Start

**Symptoms**: Plugin loads but immediately crashes

**Solutions**:
- Check plugin logs for panics
- Verify Go version compatibility
- Check for missing dependencies
- Test plugin standalone: `./my-plugin`
- Review initialization code for errors

## Best Practices

### Development Workflow

1. **Local Development**: Use `file://` for fast iteration
2. **Testing**: Deploy to staging with `grpc://` or `oci://`
3. **Production**: Use `oci://` with versioned tags

### Version Management

1. Use semantic versioning
2. Tag releases in Git and OCI registry
3. Never reuse tags (immutable releases)
4. Document breaking changes

### Deployment Pipeline

```bash
# 1. Build
go build -o plugin main.go

# 2. Test locally
./plugin  # Verify it runs

# 3. Package
docker build -t registry.example.com/plugins/my-plugin:v1.2.3 .

# 4. Push
docker push registry.example.com/plugins/my-plugin:v1.2.3

# 5. Deploy to staging
curl -X POST .../plugins -d '{"command": "oci://registry.../my-plugin:v1.2.3-rc.1", ...}'

# 6. Test in staging
# Run integration tests

# 7. Deploy to production
curl -X POST .../plugins -d '{"command": "oci://registry.../my-plugin:v1.2.3", ...}'
```

### Monitoring

- Monitor plugin health via `/api/v1/plugins/{id}/health`
- Track plugin performance metrics
- Set up alerts for plugin failures
- Log all plugin operations

## Next Steps

- [Plugin Overview]([plugins-overview](https://docs.claude.com/en/docs/plugins-overview))
- [SDK Reference]([plugins-sdk](https://docs.claude.com/en/docs/plugins-sdk))
- [Plugin Manifests]([plugins-manifests](https://docs.claude.com/en/docs/plugins-manifests))
