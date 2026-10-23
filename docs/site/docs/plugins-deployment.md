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

> **Can be turned off:** an installation can refuse this deployment type with `AI_STUDIO_PLUGIN_ALLOWED_SOURCES` (AI Studio) and `PLUGINS_ALLOWED_SOURCES` (Microgateway). See [Hardening: restrict plugin sources](./plugins-security.md#hardening-restrict-plugin-sources).

> **Safety settings:** By default, local filesystem plugin loading has safety restrictions that prevent loading from arbitrary paths. For development, you may need to adjust these settings in your configuration to allow local path loading and absolute paths. See the Security Considerations section below for details on `ALLOW_INTERNAL_NETWORK_ACCESS` and `PLUGIN_COMMAND_ALLOWLIST`.

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

> **Can be turned off:** an installation can refuse this deployment type with `AI_STUDIO_PLUGIN_ALLOWED_SOURCES` (AI Studio) and `PLUGINS_ALLOWED_SOURCES` (Microgateway). See [Hardening: restrict plugin sources](./plugins-security.md#hardening-restrict-plugin-sources).

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

> **Publishing to a registry?** Use `make plugin-publish NAME=<plugin>`, which
> builds, signs, pushes and indexes a plugin in one step. See
> [Publishing Plugins](plugins-publishing.md). The steps below describe how a
> gateway *consumes* an `oci://` plugin.

Deploy plugins as OCI artifacts in container registries.

> **Can be turned off:** an installation can refuse this deployment type with `AI_STUDIO_PLUGIN_ALLOWED_SOURCES` (AI Studio) and `PLUGINS_ALLOWED_SOURCES` (Microgateway). See [Hardening: restrict plugin sources](./plugins-security.md#hardening-restrict-plugin-sources).

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

OCI registry authentication is configured via environment variables using the pattern:

```
OCI_PLUGINS_REGISTRY_<REGISTRY_NAME>_<FIELD>=<value>
```

Where `<REGISTRY_NAME>` is the registry hostname with dots replaced by underscores (e.g., `docker.tyk.io` → `DOCKER_TYK_IO`).

#### Authentication Methods

**Entitlement Token** (Cloudsmith and similar registries):

```bash
# Entitlement token — sent as Basic auth (username: "token", password: entitlement)
OCI_PLUGINS_REGISTRY_DOCKER_TYK_IO_ENTITLEMENT=your-entitlement-token

# Custom username for entitlement (default: "token")
OCI_PLUGINS_REGISTRY_DOCKER_TYK_IO_ENTITLEMENTUSERNAME=custom-user
```

**Username + Password**:

```bash
OCI_PLUGINS_REGISTRY_GHCR_IO_USERNAME=github-user
OCI_PLUGINS_REGISTRY_GHCR_IO_PASSWORDENV=GITHUB_TOKEN  # reads from this env var
```

**OAuth2/Bearer Token**:

```bash
OCI_PLUGINS_REGISTRY_REGISTRY_EXAMPLE_COM_TOKEN=your-access-token
# Or read from env var:
OCI_PLUGINS_REGISTRY_REGISTRY_EXAMPLE_COM_TOKENENV=MY_REGISTRY_TOKEN
```

#### AI Studio vs Microgateway

Both runtimes use the same `OCI_PLUGINS_REGISTRY_*` environment variables for auth. The difference is in how OCI support is enabled:

| Setting | AI Studio | Microgateway |
|---------|-----------|-------------|
| **Enable OCI** | `AI_STUDIO_OCI_CACHE_DIR=/path` | `OCI_PLUGINS_CACHE_DIR=/path` (default: `/var/lib/microgateway/plugins`) |
| **Require signatures** | `AI_STUDIO_OCI_REQUIRE_SIGNATURE` (default `true`) | `OCI_PLUGINS_REQUIRE_SIGNATURE` (default `true`) |
| **Transparency log** | `AI_STUDIO_OCI_TLOG_POLICY` (default `required`) | `OCI_PLUGINS_TLOG_POLICY` (default `required`) |
| **Trusted root override** | `AI_STUDIO_OCI_TRUSTED_ROOT_FILE` | `OCI_PLUGINS_TRUSTED_ROOT_FILE` |
| **Registry auth** | `OCI_PLUGINS_REGISTRY_*` (shared) | `OCI_PLUGINS_REGISTRY_*` (shared) |

**Important**: `AI_STUDIO_OCI_CACHE_DIR` must be set for OCI plugin support to be enabled in AI Studio. Without it, OCI plugins cannot be installed or loaded, and registry auth configuration is ignored.

#### Signature Verification

> **Community Edition limitation**: OCI signature verification is an Enterprise Edition feature. Community Edition does **not** verify signatures: with `AI_STUDIO_OCI_REQUIRE_SIGNATURE` / `OCI_PLUGINS_REQUIRE_SIGNATURE` set (the default), OCI plugins still load, are recorded as unverified, and a warning is logged once at startup. Internal-network blocking for plugin hosts, however, is enforced in both editions (bypass with `ALLOW_INTERNAL_NETWORK_ACCESS=true` for local development).

In Enterprise Edition, AI Studio and the Microgateway verify signatures **in-process**: no `cosign` binary is needed, so verification works in the distroless images too. Signatures are read from the registry with the same registry auth (`OCI_PLUGINS_REGISTRY_*`), insecure-registry list and internal-network rules as the plugin itself; there is no separate Docker credential store to configure.

Both cosign signature formats are accepted:

- **cosign v2**: a signature manifest tagged `sha256-<digest>.sig`. `tools/publish-plugin.sh` writes this format.
- **cosign v3**: a Sigstore bundle attached to the plugin as an OCI referrer (cosign v3's default). Registries without the OCI referrers API work too.

The signature must name the digest that was pulled: the multi-arch index a tag points to (which `publish-plugin.sh` signs), or, for plugins signed per platform, the platform manifest that was pulled. An in-toto attestation (SLSA provenance, an SBOM) signed with the same key is not a signature and does not count.

| Setting (AI Studio / Microgateway) | Default | Values |
|---|---|---|
| `AI_STUDIO_OCI_REQUIRE_SIGNATURE` / `OCI_PLUGINS_REQUIRE_SIGNATURE` | `true` | `true`, `false`. AI Studio's default was `false` before 2.3.0 |
| `AI_STUDIO_OCI_TLOG_POLICY` / `OCI_PLUGINS_TLOG_POLICY` | `required` | `required`: the signature must have a valid Rekor (transparency-log) entry for the signing key, as `cosign verify --key` requires. `optional`: an entry is checked when there is one. `disabled`: key only (a warning is logged at startup) |
| `AI_STUDIO_OCI_TRUSTED_ROOT_FILE` / `OCI_PLUGINS_TRUSTED_ROOT_FILE` | empty | A Sigstore `trusted_root.json` used instead of the embedded Sigstore public-good root, for a private Sigstore deployment or after a Rekor key rotation |

An invalid value of any of these settings stops AI Studio or the Microgateway at startup.

Use `required` unless your signing pipeline cannot reach Rekor. `optional` gives no transparency-log guarantee against someone who can write to your registry: they can remove the log entry and the signature then verifies against the key alone. For air-gapped signing (`cosign sign --tlog-upload=false`), set the policy to `disabled`.

With signatures required and no Enterprise verifier configured, a plugin is refused, never loaded unverified. Cosign policy files (`VerifyWithPolicy`) are not supported.

Configure public keys for signature verification:

```bash
# Public keys for verifying plugin signatures (the Tyk signing key is always trusted)
OCI_PLUGINS_PUBKEY_1="$(cat /path/to/cosign.pub)"   # key content, selected with ?pubkey=1
OCI_PLUGINS_PUBKEY_FILE_CI=/path/to/ci-cosign.pub     # key file, selected with ?pubkey=CI
```

Key files must be regular files of at most 64 KiB.

To load unsigned plugins (for example, while developing one), set `AI_STUDIO_OCI_REQUIRE_SIGNATURE=false` on AI Studio and `OCI_PLUGINS_REQUIRE_SIGNATURE=false` on the Microgateway.

**Upgrading from 2.2:** AI Studio now requires signatures by default, and the first start re-verifies every cached OCI plugin against its registry. See the breaking changes in the 2.3.0 release notes (`CHANGELOG-v2.3.0.md`).

#### Cloudsmith-Specific Notes

When using Cloudsmith with a custom Docker domain (e.g., `docker.tyk.io`):

- **Entitlement auth** is the recommended method for programmatic access
- Entitlements bypass the Docker token exchange, which avoids scope issues with Cloudsmith's namespace-level scope requirements
- Each Cloudsmith repository has its own entitlement tokens — use repository-level entitlements
- The `ENTITLEMENT` auth type sends Basic auth directly on every request, which Cloudsmith supports natively

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

OCI plugins are pulled and cached locally:

```bash
# AI Studio
AI_STUDIO_OCI_CACHE_DIR=/var/cache/ai-studio/plugins

# Microgateway (defaults to /var/lib/microgateway/plugins)
OCI_PLUGINS_CACHE_DIR=/var/cache/microgateway/plugins
```

The SHA-256 of each fetched binary is recorded in the cache metadata, and a cached plugin is checked against it every time it loads. If the binary on disk has changed, the cached copy is not used and the plugin is fetched from the registry again (signature verification runs again too). If that fetch fails, the plugin does not load.

Some entries have no recorded hash: entries cached by releases before 2.3.0 (including entries an older release rewrote after a rollback), or entries whose metadata is missing. These are fetched from the registry again on their first load, which records the hash, and are checked from then on. After an upgrade, each OCI plugin is therefore fetched once more on its first load, so the registry should be reachable at that point. When signatures are optional and that fetch fails, the cached copy loads unverified with a warning, and the next load tries the registry again. When signatures are required (the default on AI Studio and the Microgateway), these entries, and entries cached while signatures were optional, load only once they have been fetched and verified again.

The check catches a binary that was corrupted or replaced on disk. It does not protect against an attacker who can rewrite both the binary and its metadata, so keep the cache directory writable only by the service user.

## Security Considerations

Plugins run as separate processes with the host's privileges, so install only plugins you trust. The [Plugin Security Model](./plugins-security.md) describes what process isolation does and does not cover (network egress, plugin configuration and secrets), which command checks are enforced and which are warning-only by default, and how to lock down OCI plugins.

In short, for production:

```bash
# Refuse internal plugin hosts and OCI registries, resolved names included
# (the default; true is platform-wide, see the Plugin Security Model)
ALLOW_INTERNAL_NETWORK_ACCESS=false
# Allow one internal registry (or plugin host) by name instead
LLM_UPSTREAM_ALLOWED_INTERNAL_HOSTS=registry.corp.internal
# Deprecated: also refuse internal addresses in host:port arguments
PLUGIN_BLOCK_INTERNAL_URLS=true
# Only load OCI plugins from these registries; signatures are required by default (Enterprise Edition)
AI_STUDIO_OCI_ALLOWED_REGISTRIES=registry.example.com
AI_STUDIO_OCI_REQUIRE_SIGNATURE=true
AI_STUDIO_OCI_TLOG_POLICY=required
```

`PLUGIN_COMMAND_ALLOWLIST` is advisory: a command that does not match it is logged with a warning but still accepted. Its entries are comma-separated substrings, not wildcards.

### Plugin Process Environment

AI Studio and the Microgateway start each `file://` and `oci://` plugin with an explicit environment. Plugins do not inherit the host process's environment variables. A plugin receives only:

- process basics and locale: `PATH`, `HOME`, `USER`, `TMPDIR`, `TZ`, `LANG`, `LC_*`
- TLS trust and proxies: `SSL_CERT_FILE`, `SSL_CERT_DIR`, `HTTP_PROXY`, `HTTPS_PROXY`, `NO_PROXY` (and their lower-case forms)
- Go runtime tuning: `GOMAXPROCS`, `GOMEMLIMIT`, `GOGC`, `GODEBUG`, `GOTRACEBACK`
- settings the plugin SDK and shipped plugins read: `PLUGIN_RUNTIME`, `GATEWAY_MODE`, `MICROGATEWAY_MODE`, `PLUGIN_GRPC_MAX_MSG_BYTES`, `EDGE_ID`
- any variables named in `PLUGIN_ENV_PASSTHROUGH`, set on the host

`PLUGIN_ENV_PASSTHROUGH` is a comma-separated list of variable names. An entry ending in `*` matches every variable with that prefix:

```bash
# Pass the GitHub RAG plugin's settings through to plugins
export PLUGIN_ENV_PASSTHROUGH="GITHUB_RAG_*"
```

`ALLOW_INTERNAL_NETWORK_ACCESS` is not passed by default. A plugin that honours it for its own checks (such as the GitHub RAG ingest plugin) only sees it when you add it to `PLUGIN_ENV_PASSTHROUGH`.

Before 2.3.0, plugins inherited the host's whole environment. Among the shipped plugins, only the GitHub RAG ingest plugin relies on that in a way that breaks on upgrade: it reads `GITHUB_RAG_SECRET_ENCRYPTION_KEY` (the key that encrypts stored GitHub PATs and SSH keys) and `GITHUB_RAG_KNOWN_HOSTS` from the host. If you set either, add `GITHUB_RAG_*` to `PLUGIN_ENV_PASSTHROUGH`, or set `secret_encryption_key` and `known_hosts_path` in the plugin configuration, **before you upgrade**. Without the key:

- repositories whose credentials were stored encrypted fail to sync with `secret is encrypted but no encryption key is configured`;
- credentials saved from then on are stored **unencrypted**. The plugin's warning about it is logged at debug level only, so it does not appear at the default `LOG_LEVEL=info`.

Once the key reaches the plugin again, encrypted credentials are readable again. Re-enter any credential saved while it was missing, so that it is stored encrypted.

The pass-through applies to every plugin the host starts, so name only the variables your plugins need. Avoid passing host credentials such as database URLs or encryption keys; the host logs a warning when the pass-through matches one. A bare `*` entry is ignored. Proxy variables are passed by default, so don't put proxy credentials in them unless plugins may use that proxy too. A plugin that needs configuration should normally receive it through its plugin config rather than the environment.

`grpc://` plugins run as their own processes, outside the host, so this does not apply to them.

### Plugin Message Size

Each request, response or stream a hook sees travels between the host and the plugin as one gRPC message: the body plus headers and context. Plugins built on the 2.3.0 SDK accept messages up to **64 MiB**, above the LLM vendors' own request limits. Set `PLUGIN_GRPC_MAX_MSG_BYTES` (in bytes) on the host to change it; the host passes it to every plugin it starts.

The same limit caps the request bodies the Microgateway reads for its plugins. On an LLM with `pre_auth`, `auth` or `post_auth` plugins, a body over the limit is refused with `413` before any plugin is called, and is not read in full.

Plugins built on an SDK before 2.3.0 keep gRPC's default of **4 MiB**. Text-only requests rarely reach that, but a few base64 screenshots or one PDF document block do. When such a plugin refuses a request for its size:

- `pre_auth`, `auth` and `post_auth`: the gateway answers `413` with `request too large for the gateway's plugins (<bytes> bytes)` and does not forward the request. It does not skip the plugin, since a request-path plugin may enforce policy. The log names the plugin; the response does not.
- response hooks: the response goes to the client without that plugin's changes, and the gateway logs the plugin name, the size and the limit.

Rebuild such plugins on the current SDK to lift the limit. `grpc://` plugins set their own server limits (`grpc.MaxRecvMsgSize`).

Calls from a plugin into the host's service API (KV, events, edge payloads) are limited to 8 MiB per message, whatever `PLUGIN_GRPC_MAX_MSG_BYTES` says.

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
2. **Reload Loop**: Use `POST /api/v1/plugins/{id}/reload` to test changes instantly
3. **Testing**: Deploy to staging with `grpc://` or `oci://`
4. **Production**: Use `oci://` with versioned tags

**See [Plugin Development Workflow](plugins-development-workflow.md)** for detailed setup, helper scripts, and the fastest development loop.

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

- **[Plugin Development Workflow](plugins-development-workflow.md)** - Fast iteration with file:// and reload API
- **[Plugin Overview](plugins-overview.md)** - All plugin types and capabilities
- **[SDK Reference](plugins-sdk.md)** - Complete SDK documentation
- **[Plugin Manifests](plugins-manifests.md)** - Manifest configuration
- **[Plugin Examples](plugins-examples.md)** - Working examples including community plugins
