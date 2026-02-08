# OCI Plugin Distribution Workflow

This guide covers the complete workflow for publishing, signing, and deploying plugins using the OCI (Open Container Initiative) distribution system.

## Table of Contents

1. [Prerequisites](#prerequisites)
2. [Plugin Publishing Workflow](#plugin-publishing-workflow)
3. [Edge Gateway Configuration](#edge-gateway-configuration)
4. [Control Gateway Plugin Management](#control-gateway-plugin-management)
5. [Verification and Testing](#verification-and-testing)
6. [Troubleshooting](#troubleshooting)

## Prerequisites

### Required Tools

```bash
# Install required tools
# 1. ORAS (OCI Registry As Storage)
curl -LO https://github.com/oras-project/oras/releases/download/v1.1.0/oras_1.1.0_linux_amd64.tar.gz
tar -xzf oras_1.1.0_linux_amd64.tar.gz
sudo mv oras /usr/local/bin/

# 2. Cosign (Container Signing)
curl -O -L "https://github.com/sigstore/cosign/releases/latest/download/cosign-linux-amd64"
sudo mv cosign-linux-amd64 /usr/local/bin/cosign
sudo chmod +x /usr/local/bin/cosign

# 3. Verify installations
oras version
cosign version
```

### Registry Setup

Ensure you have access to an OCI-compatible registry:
- **Nexus Repository Manager** (recommended)
- **Harbor**
- **Artifactory**
- **GitHub Container Registry (GHCR)**
- **Amazon ECR**
- **Local registry** (for testing)

## Plugin Publishing Workflow

### Step 1: Generate Signing Keys (One-time Setup)

```bash
# Generate cosign keypair for plugin signing
cosign generate-key-pair --output-key-prefix plugin-ci

# This creates:
# - plugin-ci.key (private key - keep secure!)
# - plugin-ci.pub (public key - distribute to gateways)

# Store private key securely (e.g., in CI/CD secrets)
# Distribute public key to all edge gateways
```

### Step 2: Build Your Plugin

```bash
# Example: Build a Go plugin for linux/amd64
GOOS=linux GOARCH=amd64 go build -o my-plugin-linux-amd64 ./cmd/my-plugin

# For multi-architecture support, build for multiple targets
GOOS=linux GOARCH=arm64 go build -o my-plugin-linux-arm64 ./cmd/my-plugin
```

### Step 3: Create Plugin Configuration (Optional)

```bash
# Create plugin metadata (optional but recommended)
cat > plugin.json << 'EOF'
{
  "name": "my-plugin",
  "version": "1.2.3",
  "plugin_api": "2",
  "os": "linux",
  "arch": "amd64",
  "host_min_version": "0.23.0",
  "capabilities": ["network", "fs:read"],
  "description": "My custom authentication plugin"
}
EOF
```

### Step 4: Login to Registry

```bash
# Login to your OCI registry
oras login nexus.company.com -u plugin-publisher -p 'your-password'

# Or using environment variables
export ORAS_USER=plugin-publisher
export ORAS_PASSWORD=your-password
oras login nexus.company.com
```

### Step 5: Push Plugin as OCI Artifact

```bash
# Push the plugin binary as an OCI artifact
oras push nexus.company.com/plugins/my-plugin:1.2.3 \
  --artifact-type application/vnd.tyk.plugin.binary.v1 \
  --config plugin.json:application/vnd.tyk.plugin.config.v1+json \
  ./my-plugin-linux-amd64:application/vnd.tyk.plugin.layer.v1

# Capture the digest from the output (important!)
# Example output: Pushed nexus.company.com/plugins/my-plugin:1.2.3
# Digest: sha256:abc123def456...
```

### Step 6: Sign the Plugin

```bash
# Sign by digest (recommended - immutable reference)
DIGEST="sha256:abc123def456..."  # From oras push output
cosign sign --key plugin-ci.key nexus.company.com/plugins/my-plugin@$DIGEST

# Or sign by tag (cosign resolves to digest)
cosign sign --key plugin-ci.key nexus.company.com/plugins/my-plugin:1.2.3

# Enter private key password when prompted
```

### Step 7: Verify the Signature (Optional)

```bash
# Verify the signature works
cosign verify --key plugin-ci.pub nexus.company.com/plugins/my-plugin@$DIGEST

# Should output signature verification details
```

## Edge Gateway Configuration

### Environment Variables (.env file)

```bash
# =============================================================================
# OCI Plugin System Configuration
# =============================================================================

# Enable OCI plugin support
OCI_PLUGINS_CACHE_DIR=/var/lib/microgateway/plugins
OCI_PLUGINS_REQUIRE_SIGNATURE=true
OCI_PLUGINS_ALLOWED_REGISTRIES=nexus.company.com,harbor.company.com

# Public Key Configuration (Embed PEM content directly)
OCI_PLUGINS_PUBKEY_CI="-----BEGIN PUBLIC KEY-----
MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEA...
[COPY YOUR PUBLIC KEY CONTENT HERE]
...
-----END PUBLIC KEY-----"

# Or use multiple keys for different purposes
OCI_PLUGINS_PUBKEY_1="-----BEGIN PUBLIC KEY-----..."
OCI_PLUGINS_PUBKEY_PROD="-----BEGIN PUBLIC KEY-----..."

# Registry Authentication
OCI_PLUGINS_REGISTRY_NEXUS_USERNAME=plugin-reader
OCI_PLUGINS_REGISTRY_NEXUS_PASSWORDENV=NEXUS_PASSWORD

# Credentials (keep secure!)
NEXUS_PASSWORD=your-registry-password

# Performance and Maintenance
OCI_PLUGINS_TIMEOUT=30s
OCI_PLUGINS_RETRY_ATTEMPTS=3
OCI_PLUGINS_GC_INTERVAL=24h
OCI_PLUGINS_KEEP_VERSIONS=3

# =============================================================================
# Hub-and-Spoke Edge Configuration
# =============================================================================

# Edge gateway configuration
GATEWAY_MODE=edge
CONTROL_ENDPOINT=control.company.com:50051
EDGE_ID=edge-us-west-1
EDGE_NAMESPACE=production
EDGE_AUTH_TOKEN=your-edge-auth-token
```

### Example Edge Gateway .env File

```bash
# Copy this to your edge gateway's .env file
# microgateway/dist/envs/edge.env

# Standard edge configuration
GATEWAY_MODE=edge
CONTROL_ENDPOINT=control.company.com:50051
EDGE_ID=edge-us-west-1
EDGE_NAMESPACE=production
EDGE_AUTH_TOKEN=your-edge-auth-token

# Database (local SQLite for edge)
DATABASE_TYPE=sqlite
DATABASE_DSN=file:./data/edge-microgateway.db?cache=shared&mode=rwc

# OCI Plugin Configuration
OCI_PLUGINS_CACHE_DIR=/var/lib/microgateway/plugins
OCI_PLUGINS_REQUIRE_SIGNATURE=true
OCI_PLUGINS_ALLOWED_REGISTRIES=nexus.company.com

# Embed public key directly (container-friendly)
OCI_PLUGINS_PUBKEY_CI="-----BEGIN PUBLIC KEY-----
MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEA4qiw8PWe4N5yKnXNAneu
TGGw6Gi6zp0SUHmQPIeP3w+2aV5PpnpNf8QzVwXFyLHb8gj9pkpUlzALVVLLSU/i
U7A8Vd5pNX4gBwR9pnT8+XtQHqgA4Q4p2lPtXqDdFJY8xvj5TgE2LqNzrOhXqYf
H2Z9GQ7+3Qz2GjYZfFQoYu6FK2CvN0q2VnSrO+Y0vf1Hf9y8D0G3Zn8m9Lb8P3F
XJ2Z9nQ7K8g9L0E+M4C7Nz2GbE3P8CwXf4D5Z1F2H8K4P9LjNvXr4R+2QIDAQAB
-----END PUBLIC KEY-----"

# Registry authentication
OCI_PLUGINS_REGISTRY_NEXUS_USERNAME=plugin-reader
OCI_PLUGINS_REGISTRY_NEXUS_PASSWORDENV=NEXUS_PASSWORD
NEXUS_PASSWORD=your-nexus-password
```

## Control Gateway Plugin Management

### Using mgw CLI to Add OCI Plugins

```bash
# Set up mgw CLI authentication
export MGW_TOKEN="your-admin-token"
export MGW_URL="https://control.company.com:8080"

# Create a new OCI plugin
./mgw plugin create \
  --name "My Authentication Plugin" \
  --slug "my-auth-plugin" \
  --description "Custom authentication plugin from OCI registry" \
  --command "oci://nexus.company.com/plugins/my-plugin@sha256:abc123def456?pubkey=CI" \
  --hook-type "auth" \
  --namespace "production"

# List all plugins
./mgw plugin list

# Get specific plugin details
./mgw plugin get <plugin-id>

# Associate plugin with an LLM
./mgw plugin assign --plugin-id <plugin-id> --llm-id <llm-id>

# Update plugin to new version
./mgw plugin update <plugin-id> \
  --command "oci://nexus.company.com/plugins/my-plugin@sha256:new-digest?pubkey=CI"
```

### Plugin Creation Examples

#### Authentication Plugin
```bash
./mgw plugin create \
  --name "OCI Auth Plugin" \
  --slug "oci-auth" \
  --description "Token authentication plugin" \
  --command "oci://nexus.company.com/plugins/auth@sha256:abc123?pubkey=CI" \
  --hook-type "auth" \
  --config '{"valid_tokens":["token1","token2"]}' \
  --namespace "production"
```

#### Rate Limiting Plugin
```bash
./mgw plugin create \
  --name "OCI Rate Limiter" \
  --slug "oci-rate-limiter" \
  --description "Rate limiting plugin with embedded key" \
  --command "oci://harbor.company.com/plugins/rate-limiter@sha256:def456?pubkey=PROD&arch=linux/amd64" \
  --hook-type "pre_auth" \
  --config '{"requests_per_minute":100,"burst_size":10}' \
  --namespace "production"
```

#### Data Collection Plugin
```bash
./mgw plugin create \
  --name "OCI Analytics Collector" \
  --slug "oci-analytics" \
  --description "Analytics data collector" \
  --command "oci://registry.company.com/plugins/analytics@sha256:ghi789?pubkey=1" \
  --hook-type "data_collection" \
  --config '{"output_format":"json","batch_size":100}' \
  --namespace "production"
```

### Direct API Usage

If you prefer using the REST API directly:

```bash
# Create plugin via API
curl -X POST https://control.company.com:8080/api/v1/plugins \
  -H "Authorization: Bearer $MGW_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "My OCI Plugin",
    "slug": "my-oci-plugin",
    "description": "Plugin distributed via OCI registry",
    "command": "oci://nexus.company.com/plugins/my-plugin@sha256:abc123?pubkey=CI",
    "hook_type": "auth",
    "config": {"key": "value"},
    "namespace": "production"
  }'

# List plugins
curl -H "Authorization: Bearer $MGW_TOKEN" \
  https://control.company.com:8080/api/v1/plugins

# Associate with LLM
curl -X POST https://control.company.com:8080/api/v1/llms/{llm-id}/plugins \
  -H "Authorization: Bearer $MGW_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"plugin_ids": [1, 2, 3]}'
```

## Verification and Testing

### Test Plugin Loading

```bash
# Check if OCI plugin system is working
./mgw system status

# Pre-fetch a plugin to test registry connectivity
curl -X POST https://control.company.com:8080/api/v1/oci-plugins/prefetch \
  -H "Authorization: Bearer $MGW_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"command": "oci://nexus.company.com/plugins/my-plugin@sha256:abc123?pubkey=CI"}'

# List cached OCI plugins
curl -H "Authorization: Bearer $MGW_TOKEN" \
  https://control.company.com:8080/api/v1/oci-plugins

# Get OCI plugin statistics
curl -H "Authorization: Bearer $MGW_TOKEN" \
  https://control.company.com:8080/api/v1/oci-plugins/stats
```

### Test Plugin Execution

```bash
# Create a test LLM request to trigger plugin execution
curl -X POST https://edge.company.com:8080/llm/rest/my-llm-slug/chat/completions \
  -H "Authorization: Bearer $APP_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4",
    "messages": [{"role": "user", "content": "Hello"}]
  }'

# Check logs for plugin execution
tail -f /var/log/microgateway/microgateway.log | grep "plugin"
```

## Complete Example Workflow

### 1. Plugin Author Workflow

```bash
# Plugin Author: Build and publish plugin

# 1. Generate keypair (one-time)
cosign generate-key-pair --output-key-prefix my-plugin-ci

# 2. Build plugin
GOOS=linux GOARCH=amd64 go build -o my-auth-plugin ./cmd/my-auth-plugin

# 3. Create metadata
cat > plugin.json << 'EOF'
{
  "name": "my-auth-plugin",
  "version": "1.0.0",
  "plugin_api": "2",
  "os": "linux",
  "arch": "amd64",
  "capabilities": ["network"]
}
EOF

# 4. Login to registry
oras login nexus.company.com -u plugin-publisher

# 5. Push artifact
oras push nexus.company.com/plugins/my-auth-plugin:1.0.0 \
  --artifact-type application/vnd.tyk.plugin.binary.v1 \
  --config plugin.json:application/vnd.tyk.plugin.config.v1+json \
  ./my-auth-plugin:application/vnd.tyk.plugin.layer.v1

# 6. Capture digest and sign
DIGEST="sha256:$(oras manifest fetch nexus.company.com/plugins/my-auth-plugin:1.0.0 | sha256sum | cut -d' ' -f1)"
cosign sign --key my-plugin-ci.key nexus.company.com/plugins/my-auth-plugin@$DIGEST

# 7. Share with gateway admin:
echo "Plugin published:"
echo "Repository: nexus.company.com/plugins/my-auth-plugin"
echo "Digest: $DIGEST"
echo "Public Key: $(cat my-plugin-ci.pub)"
```

### 2. Gateway Administrator Workflow

```bash
# Gateway Admin: Deploy plugin to production

# 1. Configure edge gateways with public key
# Add to edge gateway .env:
export OCI_PLUGINS_PUBKEY_CI="$(cat my-plugin-ci.pub)"

# 2. Add plugin to control gateway
export MGW_TOKEN="your-admin-token"
./mgw plugin create \
  --name "My Auth Plugin" \
  --slug "my-auth-plugin" \
  --description "Custom authentication plugin" \
  --command "oci://nexus.company.com/plugins/my-auth-plugin@sha256:abc123?pubkey=CI" \
  --hook-type "auth" \
  --namespace "production"

# 3. Associate with LLM
./mgw plugin assign --plugin-id <plugin-id> --llm-id <llm-id>

# 4. Test plugin loading
./mgw plugin test <plugin-id>
```

## Configuration Reference

### Edge Gateway Environment Variables

```bash
# Core OCI Plugin Configuration
OCI_PLUGINS_CACHE_DIR=/var/lib/microgateway/plugins           # Cache directory
OCI_PLUGINS_REQUIRE_SIGNATURE=true                           # Enforce signatures
OCI_PLUGINS_ALLOWED_REGISTRIES=nexus.company.com             # Allowed registries
OCI_PLUGINS_TIMEOUT=30s                                      # Network timeout
OCI_PLUGINS_RETRY_ATTEMPTS=3                                 # Retry failed requests

# Public Keys (choose one approach)
# Approach 1: Numbered keys
OCI_PLUGINS_PUBKEY_1="-----BEGIN PUBLIC KEY-----..."
OCI_PLUGINS_PUBKEY_2="-----BEGIN PUBLIC KEY-----..."

# Approach 2: Named keys
OCI_PLUGINS_PUBKEY_CI="-----BEGIN PUBLIC KEY-----..."
OCI_PLUGINS_PUBKEY_PROD="-----BEGIN PUBLIC KEY-----..."

# Approach 3: File-based keys
OCI_PLUGINS_PUBKEY_FILE_CI=/etc/microgateway/keys/ci.pub

# Registry Authentication
OCI_PLUGINS_REGISTRY_NEXUS_USERNAME=plugin-reader
OCI_PLUGINS_REGISTRY_NEXUS_PASSWORDENV=NEXUS_PASSWORD
NEXUS_PASSWORD=your-password
```

### Control Gateway Plugin Commands

```bash
# Plugin management via mgw CLI

# Create plugin with OCI reference
./mgw plugin create \
  --name "Plugin Name" \
  --slug "plugin-slug" \
  --description "Plugin description" \
  --command "oci://registry/repo@digest?pubkey=KEY_NAME" \
  --hook-type "auth|pre_auth|post_auth|response|data_collection" \
  --config '{"key":"value"}' \
  --namespace "production|staging|development"

# Update plugin to new version
./mgw plugin update <plugin-id> \
  --command "oci://registry/repo@new-digest?pubkey=KEY_NAME"

# List plugins
./mgw plugin list --namespace production

# Plugin lifecycle management
./mgw plugin activate <plugin-id>
./mgw plugin deactivate <plugin-id>
./mgw plugin delete <plugin-id>

# LLM association
./mgw plugin assign --plugin-id <plugin-id> --llm-id <llm-id>
./mgw plugin unassign --plugin-id <plugin-id> --llm-id <llm-id>

# OCI-specific operations (if implemented)
./mgw plugin oci list                    # List cached OCI plugins
./mgw plugin oci prefetch <oci-ref>      # Pre-fetch plugin
./mgw plugin oci verify <oci-ref>        # Verify signature
./mgw plugin oci cache stats             # Cache statistics
./mgw plugin oci cache clean             # Garbage collection
```

## Multi-Architecture Plugin Publishing

### Build for Multiple Architectures

```bash
# Build for multiple architectures
GOOS=linux GOARCH=amd64 go build -o my-plugin-linux-amd64 ./cmd/my-plugin
GOOS=linux GOARCH=arm64 go build -o my-plugin-linux-arm64 ./cmd/my-plugin

# Publish separate artifacts for each architecture
oras push nexus.company.com/plugins/my-plugin:1.0.0-amd64 \
  --artifact-type application/vnd.tyk.plugin.binary.v1 \
  ./my-plugin-linux-amd64:application/vnd.tyk.plugin.layer.v1

oras push nexus.company.com/plugins/my-plugin:1.0.0-arm64 \
  --artifact-type application/vnd.tyk.plugin.binary.v1 \
  ./my-plugin-linux-arm64:application/vnd.tyk.plugin.layer.v1

# Sign both
cosign sign --key plugin-ci.key nexus.company.com/plugins/my-plugin:1.0.0-amd64
cosign sign --key plugin-ci.key nexus.company.com/plugins/my-plugin:1.0.0-arm64
```

### Use Architecture-Specific Plugins

```bash
# Create plugins with architecture specification
./mgw plugin create \
  --name "My Plugin (AMD64)" \
  --command "oci://nexus.company.com/plugins/my-plugin@sha256:amd64-digest?arch=linux/amd64&pubkey=CI" \
  --hook-type "auth"

./mgw plugin create \
  --name "My Plugin (ARM64)" \
  --command "oci://nexus.company.com/plugins/my-plugin@sha256:arm64-digest?arch=linux/arm64&pubkey=CI" \
  --hook-type "auth"
```

## Troubleshooting

### Common Issues

#### 1. Signature Verification Failures
```bash
# Check public key configuration
curl -H "Authorization: Bearer $MGW_TOKEN" \
  https://control.company.com:8080/api/v1/oci-plugins/keys

# Verify signature manually
cosign verify --key plugin-ci.pub nexus.company.com/plugins/my-plugin@sha256:digest
```

#### 2. Registry Authentication Issues
```bash
# Test registry connectivity
oras pull nexus.company.com/plugins/my-plugin@sha256:digest

# Check credentials
echo $NEXUS_PASSWORD | base64  # Should not be empty
```

#### 3. Plugin Loading Failures
```bash
# Check plugin manager logs
tail -f /var/log/microgateway/microgateway.log | grep -i "plugin\|oci"

# Check OCI plugin statistics
curl -H "Authorization: Bearer $MGW_TOKEN" \
  https://control.company.com:8080/api/v1/oci-plugins/stats

# Test plugin pre-fetch
curl -X POST https://control.company.com:8080/api/v1/oci-plugins/prefetch \
  -H "Authorization: Bearer $MGW_TOKEN" \
  -d '{"command": "oci://registry/repo@digest?pubkey=CI"}'
```

#### 4. Cache Issues
```bash
# Clear plugin cache
rm -rf /var/lib/microgateway/plugins/*

# Trigger garbage collection
curl -X POST https://control.company.com:8080/api/v1/oci-plugins/gc \
  -H "Authorization: Bearer $MGW_TOKEN"
```

### Debug Mode

```bash
# Enable debug logging
export LOG_LEVEL=debug

# Check OCI plugin system status
curl -H "Authorization: Bearer $MGW_TOKEN" \
  https://control.company.com:8080/api/v1/system/health

# Monitor real-time logs
tail -f /var/log/microgateway/microgateway.log | grep -E "(OCI|plugin|signature)"
```

## Security Best Practices

### Key Management
- **Rotate keys regularly** (every 6 months)
- **Use different keys** for different environments
- **Store private keys securely** (KMS, vault, CI/CD secrets)
- **Never commit private keys** to repositories

### Registry Security
- **Use private registries** for internal plugins
- **Enable registry authentication** for all environments
- **Audit plugin access** and downloads
- **Monitor signature verification** failures

### Plugin Security
- **Always verify signatures** in production
- **Use digest references** instead of tags for immutable deployments
- **Implement plugin content scanning** if required
- **Monitor plugin behavior** in production

This workflow provides a complete, secure, and scalable approach to OCI plugin distribution in the microgateway ecosystem.