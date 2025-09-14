# Plugin Installation

This guide covers installing plugins in the microgateway using both binary and folder-based installation methods.

## Overview

Plugin installation methods:
- **Binary Installation**: Direct plugin binary installation
- **Folder Installation**: Install from plugin directory structure
- **OCI Installation**: Install from container registries (recommended)
- **Local Development**: Install plugins during development

## Installation Methods

### Binary Installation

#### Direct Binary Placement
```bash
# Copy plugin binary to plugins directory
mkdir -p plugins
cp my_plugin plugins/

# Make executable
chmod +x plugins/my_plugin

# Configure plugin
cat > config/plugins.yaml << EOF
version: "1.0"
plugins:
  - name: "my-plugin"
    path: "./plugins/my_plugin"
    enabled: true
    hook_types: ["pre_auth"]
EOF
```

#### Binary with Configuration
```bash
# Create plugin structure
mkdir -p plugins/my-plugin
cp my_plugin plugins/my-plugin/
cp config.yaml plugins/my-plugin/

# Plugin configuration
cat > config/plugins.yaml << EOF
version: "1.0"
plugins:
  - name: "my-plugin"
    path: "./plugins/my-plugin/my_plugin"
    enabled: true
    hook_types: ["pre_auth"]
    config_file: "./plugins/my-plugin/config.yaml"
EOF
```

### Folder Installation

#### Plugin Directory Structure
```
plugins/
├── my-plugin/
│   ├── my_plugin          # Plugin binary
│   ├── plugin.yaml        # Plugin metadata
│   ├── config.yaml        # Plugin configuration
│   ├── README.md          # Documentation
│   └── LICENSE            # License file
```

#### Plugin Metadata File
```yaml
# plugins/my-plugin/plugin.yaml
name: "my-plugin"
version: "1.0.0"
description: "Custom authentication plugin"
author: "Developer Name"
license: "MIT"
plugin_api: "2"
supported_os: ["linux", "darwin"]
supported_arch: ["amd64", "arm64"]
hook_types: ["pre_auth", "auth"]
dependencies: []
```

#### Folder Installation Command
```bash
# Install from folder
mgw plugin install ./plugins/my-plugin

# Or configure directly
cat > config/plugins.yaml << EOF
version: "1.0"
plugins:
  - name: "my-plugin"
    path: "./plugins/my-plugin/my_plugin"
    enabled: true
    metadata_file: "./plugins/my-plugin/plugin.yaml"
    config_file: "./plugins/my-plugin/config.yaml"
EOF
```

### OCI Installation (Recommended)

#### Registry-Based Installation
```bash
# Install from OCI registry
mgw plugin install registry.company.com/plugins/my-plugin:v1.0.0

# Install with signature verification
mgw plugin install \
  --verify-signature \
  --trusted-key=/path/to/public.key \
  registry.company.com/plugins/my-plugin@sha256:abc123...
```

For detailed OCI installation, see [Plugin Distribution](plugin-distribution.md).

## Configuration

### Plugin Configuration File
```yaml
# config/plugins.yaml
version: "1.0"

# Global plugin settings
global:
  enabled: true
  timeout: "30s"
  max_concurrent: 10
  security:
    verify_signatures: true
    trusted_keys_path: "./keys/trusted"

# Individual plugin configurations
plugins:
  - name: "auth-plugin"
    path: "./plugins/auth_plugin"
    enabled: true
    priority: 100
    hook_types: ["pre_auth", "auth"]
    config:
      auth_endpoint: "${AUTH_SERVICE_URL}"
      timeout: "30s"
      cache_ttl: "5m"
      
  - name: "analytics-collector"
    path: "./plugins/elasticsearch_collector"
    enabled: true
    priority: 200
    hook_types: ["data_collection"]
    config:
      elasticsearch_url: "${ELASTICSEARCH_URL}"
      index_prefix: "microgateway"
      batch_size: 100
```

### Environment Variables
```bash
# Plugin system configuration
PLUGINS_ENABLED=true
PLUGINS_CONFIG_PATH=./config/plugins.yaml
PLUGINS_DIR=./plugins
PLUGINS_TIMEOUT=30s

# Security settings
PLUGINS_VERIFY_SIGNATURES=true
PLUGINS_TRUSTED_KEYS_PATH=./keys/trusted
PLUGINS_ALLOW_UNSIGNED=false

# Resource limits
PLUGINS_MAX_MEMORY=256MB
PLUGINS_MAX_CPU=50
PLUGINS_MAX_PROCESSES=10
```

## Local Development

### Development Setup
```bash
# Create development plugin structure
mkdir -p dev-plugins/my-plugin
cd dev-plugins/my-plugin

# Create plugin
go mod init my-plugin
go get github.com/TykTechnologies/midsommar/microgateway/plugins/sdk

# Write plugin code
cat > main.go << 'EOF'
package main

import (
    "context"
    "github.com/TykTechnologies/midsommar/microgateway/plugins/sdk"
)

type DevPlugin struct{}

func (p *DevPlugin) Initialize(config map[string]interface{}) error {
    return nil
}

func (p *DevPlugin) GetHookType() sdk.HookType {
    return sdk.HookTypePreAuth
}

func (p *DevPlugin) ProcessRequest(ctx context.Context, req *sdk.RequestData, pluginCtx *sdk.PluginContext) (*sdk.PluginResponse, error) {
    return &sdk.PluginResponse{Continue: true}, nil
}

func main() {
    plugin := &DevPlugin{}
    sdk.ServePlugin(plugin)
}
EOF

# Build plugin
go build -o my_plugin main.go
```

### Development Configuration
```bash
# Configure for development
cat > ../../config/plugins-dev.yaml << EOF
version: "1.0"
plugins:
  - name: "dev-plugin"
    path: "./dev-plugins/my-plugin/my_plugin"
    enabled: true
    hook_types: ["pre_auth"]
    config:
      debug: true
EOF

# Start microgateway with dev plugins
PLUGINS_CONFIG_PATH=./config/plugins-dev.yaml ./dist/microgateway
```

## Plugin Installation Verification

### Verify Installation
```bash
# Check plugin status
mgw plugin list

# Test plugin health
mgw plugin health my-plugin

# View plugin logs
tail -f /var/log/microgateway/plugins.log | grep my-plugin
```

### Test Plugin Functionality
```bash
# Make test request to trigger plugin
curl -X POST http://localhost:8080/llm/rest/gpt-4/chat/completions \
  -H "Authorization: Bearer $APP_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"model": "gpt-4", "messages": [{"role": "user", "content": "test"}]}'

# Check plugin execution in logs
grep "plugin execution" /var/log/microgateway/proxy.log
```

## Plugin Management

### Plugin Lifecycle Management
```bash
# Enable plugin
mgw plugin enable my-plugin

# Disable plugin
mgw plugin disable my-plugin

# Restart plugin
mgw plugin restart my-plugin

# Reload plugin configuration
mgw plugin reload my-plugin

# Uninstall plugin
mgw plugin uninstall my-plugin
```

### Plugin Updates
```bash
# Update plugin binary
cp new_my_plugin plugins/my_plugin

# Restart plugin to load new version
mgw plugin restart my-plugin

# Or restart entire service
systemctl restart microgateway
```

## Multiple Plugin Installation

### Installing Multiple Plugins
```bash
# Install multiple plugins
mgw plugin install \
  registry.company.com/plugins/auth-plugin:v1.0.0 \
  registry.company.com/plugins/analytics-plugin:v2.1.0 \
  ./local-plugins/custom-plugin

# Batch install from configuration
mgw plugin install --config=./config/production-plugins.yaml
```

### Plugin Dependencies
```yaml
# Handle plugin dependencies
plugins:
  - name: "base-plugin"
    path: "./plugins/base_plugin"
    enabled: true
    priority: 50
    
  - name: "dependent-plugin"
    path: "./plugins/dependent_plugin"
    enabled: true
    priority: 100
    depends_on: ["base-plugin"]  # Loaded after base-plugin
```

## Configuration Management

### Environment-Specific Configurations
```bash
# Development
PLUGINS_CONFIG_PATH=./config/plugins-dev.yaml

# Staging
PLUGINS_CONFIG_PATH=./config/plugins-staging.yaml

# Production
PLUGINS_CONFIG_PATH=./config/plugins-prod.yaml
```

### Configuration Templates
```yaml
# Production template
version: "1.0"
global:
  enabled: true
  timeout: "30s"
  verify_signatures: true
  
plugins:
  - name: "prod-auth"
    registry: "registry.company.com"
    repository: "plugins/auth"
    digest: "sha256:abc123..."
    enabled: true
    hook_types: ["auth"]
    
  - name: "prod-analytics"
    registry: "registry.company.com"
    repository: "plugins/elasticsearch"
    digest: "sha256:def456..."
    enabled: true
    hook_types: ["data_collection"]
```

## Installation Scripts

### Automated Installation
```bash
#!/bin/bash
# install-plugins.sh

set -e

PLUGINS_DIR="./plugins"
CONFIG_FILE="./config/plugins.yaml"

# Create directories
mkdir -p $PLUGINS_DIR
mkdir -p $(dirname $CONFIG_FILE)

# Install plugins
plugins=(
  "auth-plugin:./binaries/auth_plugin"
  "analytics-plugin:./binaries/analytics_plugin"
  "custom-plugin:./binaries/custom_plugin"
)

for plugin in "${plugins[@]}"; do
  IFS=':' read -r name path <<< "$plugin"
  echo "Installing $name from $path"
  
  cp "$path" "$PLUGINS_DIR/"
  chmod +x "$PLUGINS_DIR/$(basename $path)"
done

# Generate configuration
cat > $CONFIG_FILE << 'EOF'
version: "1.0"
plugins:
  - name: "auth-plugin"
    path: "./plugins/auth_plugin"
    enabled: true
    hook_types: ["auth"]
  - name: "analytics-plugin"
    path: "./plugins/analytics_plugin"
    enabled: true
    hook_types: ["data_collection"]
EOF

echo "Plugin installation complete"
```

### Docker Installation
```dockerfile
# Dockerfile with plugins
FROM microgateway:base

# Copy plugins
COPY plugins/ /app/plugins/
COPY config/plugins.yaml /app/config/

# Set permissions
RUN chmod +x /app/plugins/*

# Configure plugins
ENV PLUGINS_ENABLED=true
ENV PLUGINS_CONFIG_PATH=/app/config/plugins.yaml
```

## Troubleshooting

### Installation Issues
```bash
# Check plugin permissions
ls -la plugins/

# Verify plugin binary
file plugins/my_plugin

# Test plugin execution
./plugins/my_plugin --help
```

### Configuration Issues
```bash
# Validate configuration file
cat config/plugins.yaml | yq .

# Check environment variables
env | grep PLUGINS_

# Test configuration loading
./dist/microgateway --test-config
```

### Runtime Issues
```bash
# Check plugin status
mgw system health

# View plugin logs
tail -f /var/log/microgateway/plugins.log

# Monitor plugin processes
ps aux | grep plugin

# Check resource usage
top -p $(pgrep -f my_plugin)
```

### Plugin Communication Issues
```bash
# Check gRPC communication
netstat -tlnp | grep :plugin-port

# Test plugin health
mgw plugin health my-plugin

# Review plugin handshake
grep "handshake" /var/log/microgateway/microgateway.log
```

## Plugin Removal

### Uninstalling Plugins
```bash
# Disable plugin first
mgw plugin disable my-plugin

# Remove from configuration
# Edit config/plugins.yaml to remove plugin entry

# Remove plugin binary
rm plugins/my_plugin

# Restart microgateway
systemctl restart microgateway
```

### Clean Removal
```bash
# Complete plugin removal
mgw plugin uninstall my-plugin --purge

# This removes:
# - Plugin binary
# - Plugin configuration
# - Plugin cache data
# - Plugin logs
```

---

Plugin installation provides flexible deployment options for extending microgateway functionality. For plugin development, see [Plugin System](plugin-system.md). For distribution, see [Plugin Distribution](plugin-distribution.md).
