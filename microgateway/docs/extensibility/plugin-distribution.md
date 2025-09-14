# Plugin Distribution (OCI Format)

The microgateway supports OCI-based plugin distribution, enabling secure and standardized plugin deployment using container registries.

## Overview

OCI plugin distribution features:
- **Industry Standard**: Uses OCI artifact format for plugin distribution
- **Container Registries**: Compatible with Nexus, Harbor, Artifactory, GHCR, ECR
- **Content Addressing**: Immutable plugin versions using digest-based references
- **Digital Signatures**: Cosign-based plugin signing and verification
- **Efficient Caching**: Content-addressed storage with efficient updates
- **No Container Runtime**: Direct binary execution without container overhead

## Architecture

### Distribution Flow
```
Plugin Author → OCI Registry → Edge Gateway
    (build & sign)   (store)      (pull & verify & execute)
```

### Components
- **ORAS**: OCI Registry As Storage for pushing/pulling artifacts
- **Cosign**: Container signing for plugin verification
- **OCI Registry**: Standard container registry (Nexus/Harbor/etc.)
- **Microgateway**: Gateway pulls, verifies, and executes plugins

## Plugin Publishing

### Prerequisites
```bash
# Install required tools
# ORAS for OCI artifact management
curl -LO https://github.com/oras-project/oras/releases/download/v1.0.0/oras_1.0.0_linux_amd64.tar.gz
tar -xzf oras_1.0.0_linux_amd64.tar.gz
sudo mv oras /usr/local/bin/

# Cosign for signing
go install github.com/sigstore/cosign/v2/cmd/cosign@latest
```

### Generate Signing Keys
```bash
# Generate cosign keypair (one-time setup)
cosign generate-key-pair --output-key-prefix ./plugin-ci
# Produces: plugin-ci.key (private) and plugin-ci.pub (public)

# Store private key securely (CI/CD secrets)
# Distribute public key with gateway deployment
```

### Build and Publish Plugin
```bash
# 1. Build plugin binary
GOOS=linux GOARCH=amd64 go build -o my-plugin-linux-amd64 ./cmd/my-plugin

# 2. Create plugin metadata (optional)
cat > plugin.json << EOF
{
  "name": "my-plugin",
  "version": "1.2.3",
  "plugin_api": "2",
  "os": "linux",
  "arch": "amd64",
  "capabilities": ["network", "fs:read"]
}
EOF

# 3. Login to registry
oras login registry.company.com -u plugin-ci -p "$REGISTRY_PASSWORD"

# 4. Push plugin as OCI artifact
oras push registry.company.com/plugins/my-plugin:1.2.3 \
  --artifact-type application/vnd.tyk.plugin.binary.v1 \
  --config plugin.json:application/vnd.tyk.plugin.config.v1+json \
  ./my-plugin-linux-amd64:application/vnd.tyk.plugin.layer.v1

# 5. Sign the artifact
DIGEST=$(oras push output | grep "Digest:" | cut -d' ' -f2)
cosign sign --key ./plugin-ci.key registry.company.com/plugins/my-plugin@$DIGEST

echo "Plugin published with digest: $DIGEST"
```

### Automated Publishing (CI/CD)
```yaml
# .github/workflows/publish-plugin.yml
name: Publish Plugin

on:
  push:
    tags: ['v*']

jobs:
  publish:
    runs-on: ubuntu-latest
    steps:
    - uses: actions/checkout@v3
    
    - name: Set up Go
      uses: actions/setup-go@v3
      with:
        go-version: 1.23
        
    - name: Build plugin
      run: |
        GOOS=linux GOARCH=amd64 go build -o my-plugin-linux-amd64 ./cmd/my-plugin
        
    - name: Install tools
      run: |
        curl -LO https://github.com/oras-project/oras/releases/download/v1.0.0/oras_1.0.0_linux_amd64.tar.gz
        tar -xzf oras_1.0.0_linux_amd64.tar.gz
        sudo mv oras /usr/local/bin/
        go install github.com/sigstore/cosign/v2/cmd/cosign@latest
        
    - name: Publish plugin
      env:
        REGISTRY_PASSWORD: ${{ secrets.REGISTRY_PASSWORD }}
        COSIGN_PRIVATE_KEY: ${{ secrets.COSIGN_PRIVATE_KEY }}
      run: |
        echo "$REGISTRY_PASSWORD" | oras login registry.company.com -u plugin-ci --password-stdin
        
        cat > plugin.json << EOF
        {
          "name": "my-plugin",
          "version": "${{ github.ref_name }}",
          "plugin_api": "2",
          "os": "linux",
          "arch": "amd64"
        }
        EOF
        
        DIGEST=$(oras push registry.company.com/plugins/my-plugin:${{ github.ref_name }} \
          --artifact-type application/vnd.tyk.plugin.binary.v1 \
          --config plugin.json:application/vnd.tyk.plugin.config.v1+json \
          ./my-plugin-linux-amd64:application/vnd.tyk.plugin.layer.v1 | \
          grep "Digest:" | cut -d' ' -f2)
          
        echo "$COSIGN_PRIVATE_KEY" | cosign sign --key env://COSIGN_PRIVATE_KEY \
          registry.company.com/plugins/my-plugin@$DIGEST
```

## Plugin Consumption

### Gateway Configuration
```yaml
# config/plugins.yaml
version: "1.0"
plugins:
  - name: "my-plugin"
    registry: "registry.company.com"
    repository: "plugins/my-plugin"
    digest: "sha256:abc123def456..."  # Pin to specific digest
    arch: "linux/amd64"
    cache_dir: "/var/lib/microgateway/plugins"
    cosign_pubkeys:
      - "/etc/microgateway/trusted/plugin-ci.pub"
    auth:
      username: "plugin-reader"
      password_env: "PLUGIN_READER_PASSWORD"
    config:
      my_setting: "value"
```

### Plugin Pull and Verification
```bash
# Manual plugin pull for testing
oras pull registry.company.com/plugins/my-plugin@sha256:abc123... \
  -o /tmp/plugin-test

# Verify signature
cosign verify \
  --key /etc/microgateway/trusted/plugin-ci.pub \
  registry.company.com/plugins/my-plugin@sha256:abc123...

# Test plugin binary
chmod +x /tmp/plugin-test/my-plugin-linux-amd64
./tmp/plugin-test/my-plugin-linux-amd64 --version
```

## Plugin Caching

### Cache Management
The microgateway caches plugins for efficient operation:

```bash
# Cache directory structure
/var/lib/microgateway/plugins/
├── cas/                    # Content-addressed storage
├── bin-<digest>           # Materialized executables  
└── active/
    └── my-plugin -> ../bin-<digest>  # Active version symlink
```

### Cache Operations
```bash
# View plugin cache
ls -la /var/lib/microgateway/plugins/

# Clear plugin cache
rm -rf /var/lib/microgateway/plugins/cas/*
rm -f /var/lib/microgateway/plugins/bin-*

# Force plugin re-download
mgw plugin refresh my-plugin
```

## Plugin Updates and Rollback

### Plugin Updates
```bash
# Update plugin configuration with new digest
# Edit config/plugins.yaml:
digest: "sha256:new-version-digest..."

# Restart microgateway or reload plugins
mgw plugin reload

# Gateway pulls new version and updates symlink atomically
```

### Rollback
```bash
# Rollback to previous version
# Edit config/plugins.yaml with previous digest:
digest: "sha256:previous-version-digest..."

# Reload plugins
mgw plugin reload

# Old binary remains in cache for instant rollback
```

### Multi-Architecture Support
```bash
# Publish for multiple architectures
GOOS=linux GOARCH=amd64 go build -o my-plugin-linux-amd64 ./cmd/my-plugin
GOOS=linux GOARCH=arm64 go build -o my-plugin-linux-arm64 ./cmd/my-plugin

# Publish each architecture separately
oras push registry.company.com/plugins/my-plugin:1.2.3-amd64 \
  --artifact-type application/vnd.tyk.plugin.binary.v1 \
  ./my-plugin-linux-amd64:application/vnd.tyk.plugin.layer.v1

oras push registry.company.com/plugins/my-plugin:1.2.3-arm64 \
  --artifact-type application/vnd.tyk.plugin.binary.v1 \
  ./my-plugin-linux-arm64:application/vnd.tyk.plugin.layer.v1
```

## Registry Configuration

### Registry Setup
Most organizations use existing container registries:

#### Nexus Repository
```bash
# Enable Docker/OCI hosted repository
# Create service account with read permissions
# Use standard Docker login
```

#### Harbor Registry
```bash
# Create project for plugins
# Configure robot accounts for access
# Enable OCI artifact support
```

#### GitHub Container Registry
```bash
# Use GitHub packages
# Configure GitHub token with packages:read scope
# Use standard OCI workflow
```

### Authentication
```bash
# Basic authentication
PLUGIN_REGISTRY_USER=plugin-reader
PLUGIN_REGISTRY_PASS=secret-password

# Token-based authentication  
PLUGIN_REGISTRY_TOKEN=ghp_token_here

# Registry-specific authentication
# Configure per registry requirements
```

## Security Considerations

### Signature Verification
```bash
# Required for production
PLUGINS_VERIFY_SIGNATURES=true
PLUGINS_TRUSTED_KEYS_PATH=/etc/microgateway/trusted

# Public key distribution
# Include trusted public keys in gateway deployment
# Rotate keys following standard PKI practices
```

### Registry Security
```bash
# Registry allowlist
PLUGINS_ALLOWED_REGISTRIES="registry.company.com,ghcr.io"

# Digest pinning (recommended)
# Always use digest references in production
digest: "sha256:abc123..."  # Immutable reference

# Tag references (development only)
tag: "latest"  # Mutable reference
```

### Air-Gapped Deployment
```bash
# Export plugin for air-gapped environments
oras pull registry.company.com/plugins/my-plugin@sha256:abc123... \
  --format=oci-dir:/tmp/plugin-export

# Copy to air-gapped environment
rsync -av /tmp/plugin-export/ airgapped:/var/lib/plugins/

# Install from local directory
oras pull oci-dir:/var/lib/plugins/my-plugin \
  -o /var/lib/microgateway/plugins/cas
```

## Plugin Metadata

### OCI Artifact Structure
```bash
# Artifact type
application/vnd.tyk.plugin.binary.v1

# Layers
# Layer[0]: Plugin binary (application/vnd.tyk.plugin.layer.v1)
# Config: Plugin metadata (application/vnd.tyk.plugin.config.v1+json)
```

### Plugin Metadata Schema
```json
{
  "name": "my-plugin",
  "version": "1.2.3",
  "plugin_api": "2",
  "os": "linux",
  "arch": "amd64",
  "libc": "glibc",
  "host_min_version": "0.23.0",
  "capabilities": ["network", "fs:read"],
  "dependencies": [],
  "description": "Custom plugin for authentication",
  "author": "Developer Name",
  "license": "MIT"
}
```

## Registry Operations

### Plugin Discovery
```bash
# List available plugins in registry
oras repo ls registry.company.com

# List plugin versions
oras repo tags registry.company.com/plugins/my-plugin

# Get plugin manifest
oras manifest fetch registry.company.com/plugins/my-plugin:1.2.3
```

### Plugin Information
```bash
# Get plugin metadata
oras pull registry.company.com/plugins/my-plugin:1.2.3 \
  --format=go-template='{{.config.data | fromjson}}'

# Verify plugin signature
cosign verify \
  --key /etc/microgateway/trusted/plugin-ci.pub \
  registry.company.com/plugins/my-plugin:1.2.3
```

## Best Practices

### Plugin Versioning
- Use semantic versioning (v1.2.3)
- Pin to digest in production configurations
- Use tags for development and testing
- Maintain compatibility across plugin API versions

### Security
- Always sign plugins in CI/CD pipelines
- Verify signatures in production deployments
- Use digest references for immutable deployments
- Regular key rotation following security policies

### Registry Management
- Use private registries for internal plugins
- Implement proper access controls
- Monitor registry storage usage
- Regular cleanup of old plugin versions

### Deployment
- Test plugins in development environments
- Use canary deployments for plugin updates
- Monitor plugin performance after updates
- Implement automated rollback procedures

## Troubleshooting

### Publishing Issues
```bash
# Authentication failure
oras login registry.company.com -u username -p password

# Push failure
# Check registry permissions and artifact type support

# Signing failure
cosign verify --key public.key registry.company.com/plugins/my-plugin:tag
```

### Gateway Issues
```bash
# Pull failure
# Check network connectivity to registry
curl -I https://registry.company.com/v2/

# Verification failure
# Check public key path and content
cat /etc/microgateway/trusted/plugin-ci.pub

# Execution failure
# Check plugin binary permissions and architecture
file /var/lib/microgateway/plugins/bin-<digest>
```

### Registry Issues
```bash
# Registry connectivity
oras repo ls registry.company.com

# Authentication issues
oras login registry.company.com --debug

# Artifact format issues
oras manifest fetch registry.company.com/plugins/my-plugin:tag
```

## Migration from Other Distribution Methods

### From File-Based Distribution
```bash
# Convert existing plugin to OCI
# 1. Package existing binary
oras push registry.company.com/plugins/existing-plugin:1.0.0 \
  --artifact-type application/vnd.tyk.plugin.binary.v1 \
  ./existing-plugin:application/vnd.tyk.plugin.layer.v1

# 2. Sign artifact
cosign sign --key ./plugin-ci.key \
  registry.company.com/plugins/existing-plugin:1.0.0

# 3. Update gateway configuration
# Replace path with registry configuration
```

### From ZIP Distribution
```bash
# Extract ZIP and publish binary
unzip plugin-package.zip
cd plugin-package

oras push registry.company.com/plugins/zip-plugin:1.0.0 \
  --artifact-type application/vnd.tyk.plugin.binary.v1 \
  ./plugin-binary:application/vnd.tyk.plugin.layer.v1
```

## Advanced Features

### Plugin Catalogs
```bash
# Create plugin catalog
cat > plugin-catalog.yaml << EOF
plugins:
  - name: "auth-plugin"
    repository: "registry.company.com/plugins/auth"
    latest_version: "1.2.3"
    digest: "sha256:abc123..."
    description: "Enterprise authentication plugin"
    
  - name: "analytics-plugin"
    repository: "registry.company.com/plugins/analytics"
    latest_version: "2.1.0"
    digest: "sha256:def456..."
    description: "Advanced analytics collection"
EOF

# Install from catalog
mgw plugin install --catalog=plugin-catalog.yaml auth-plugin
```

### Plugin Discovery
```bash
# Search available plugins
mgw plugin search --registry=registry.company.com

# Get plugin information
mgw plugin info registry.company.com/plugins/my-plugin:1.2.3

# List plugin versions
mgw plugin versions registry.company.com/plugins/my-plugin
```

### Batch Installation
```bash
# Install multiple plugins
mgw plugin install \
  registry.company.com/plugins/auth:1.0.0 \
  registry.company.com/plugins/analytics:2.0.0 \
  registry.company.com/plugins/audit:1.5.0

# Install from requirement file
cat > requirements.yaml << EOF
plugins:
  - registry.company.com/plugins/auth@sha256:abc123...
  - registry.company.com/plugins/analytics@sha256:def456...
EOF

mgw plugin install --requirements=requirements.yaml
```

## Example Workflow

### Complete Plugin Workflow
```bash
# 1. Plugin Author: Build and publish
cd my-plugin-project
make build-linux
oras push registry.company.com/plugins/my-plugin:1.0.0 \
  --artifact-type application/vnd.tyk.plugin.binary.v1 \
  ./dist/my-plugin-linux-amd64:application/vnd.tyk.plugin.layer.v1
DIGEST=$(oras push output | grep Digest | cut -d' ' -f2)
cosign sign --key ./signing.key registry.company.com/plugins/my-plugin@$DIGEST

# 2. Gateway Operator: Configure plugin
cat >> config/plugins.yaml << EOF
  - name: "my-plugin"
    registry: "registry.company.com"
    repository: "plugins/my-plugin"
    digest: "$DIGEST"
    cosign_pubkeys: ["/etc/microgateway/trusted/signing.pub"]
EOF

# 3. Deploy and verify
systemctl restart microgateway
mgw plugin list
mgw plugin health my-plugin
```

---

OCI-based plugin distribution provides a standardized, secure approach to plugin deployment. For plugin development, see [Plugin System](plugin-system.md). For configuration, see [Plugin Installation](plugin-installation.md).
