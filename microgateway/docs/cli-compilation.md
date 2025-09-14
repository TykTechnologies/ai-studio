# Compiling the CLI (mgw)

This guide covers building the `mgw` CLI tool for managing the microgateway.

## Prerequisites

Same as the [microgateway server compilation](compiling.md):
- **Go 1.23.0+** (with toolchain go1.23.1)
- **Git** for version control
- **Make** (optional but recommended)

## Quick Build

```bash
# Build CLI binary
make build-cli

# Or using go build directly
go build -o dist/mgw ./cmd/mgw
```

## Build Options

### Standard CLI Build
```bash
# Using Makefile (recommended)
make build-cli

# Using go build directly
go build -o dist/mgw ./cmd/mgw
```

### Build Both Server and CLI
```bash
# Build both binaries at once
make build-both

# This creates both:
# - dist/microgateway (server)
# - dist/mgw (CLI)
```

### CLI Version Information
```bash
# Makefile includes version information automatically
make build-cli

# Manual version flags (same as server)
VERSION=$(git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_HASH=$(git rev-parse HEAD 2>/dev/null || echo "unknown")
BUILD_TIME=$(date -u +%Y-%m-%dT%H:%M:%SZ)

go build -ldflags "-X main.Version=${VERSION} -X main.BuildHash=${BUILD_HASH} -X main.BuildTime=${BUILD_TIME}" \
  -o dist/mgw ./cmd/mgw
```

### Cross-Platform CLI Builds
```bash
# Build CLI for all platforms
make build-cli-all

# Manual cross-compilation
GOOS=linux GOARCH=amd64 go build -o dist/mgw-linux-amd64 ./cmd/mgw
GOOS=darwin GOARCH=arm64 go build -o dist/mgw-darwin-arm64 ./cmd/mgw
GOOS=windows GOARCH=amd64 go build -o dist/mgw-windows-amd64.exe ./cmd/mgw
```

## CLI Verification

### Test CLI Binary
```bash
# Check version
./dist/mgw --version

# Check help
./dist/mgw --help

# Test connection (requires running microgateway)
./dist/mgw system health
```

### CLI Commands
```bash
# List available commands
./dist/mgw help

# Command-specific help
./dist/mgw llm --help
./dist/mgw app --help
./dist/mgw token --help
```

## CLI Installation

### System Installation
```bash
# Install to system PATH (optional)
sudo cp dist/mgw /usr/local/bin/mgw

# Or add to PATH in shell profile
echo 'export PATH=$PATH:'$(pwd)'/dist' >> ~/.bashrc
source ~/.bashrc
```

### Development Installation
```bash
# Install using go install
go install ./cmd/mgw

# This installs to $GOPATH/bin/mgw or $HOME/go/bin/mgw
```

## CLI Configuration

### Environment Variables
```bash
# Set CLI defaults
export MGW_URL="http://localhost:8080"
export MGW_TOKEN="your-admin-token"

# Optional configuration
export MGW_FORMAT="table"  # table, json, yaml
export MGW_TIMEOUT="30s"
```

### Configuration File
```bash
# Create CLI config directory
mkdir -p ~/.mgw

# Create configuration file
cat > ~/.mgw/config.yaml << EOF
url: http://localhost:8080
token: your-admin-token
format: table
verbose: false
timeout: 30s
EOF
```

## CLI Features

### Output Formats
The CLI supports multiple output formats:

```bash
# Table format (default)
./dist/mgw llm list --format=table

# JSON format (machine-readable)
./dist/mgw llm list --format=json

# YAML format (configuration-friendly)
./dist/mgw llm list --format=yaml
```

### Global Options
```bash
# Available for all commands
./dist/mgw [command] \
  --url=http://localhost:8080 \
  --token=your-token \
  --format=json \
  --verbose \
  --timeout=60s
```

### Command Categories

#### System Commands
```bash
./dist/mgw system health    # Health check
./dist/mgw system ready     # Readiness check
./dist/mgw system version   # Version information
./dist/mgw system metrics   # Prometheus metrics
```

#### LLM Management
```bash
./dist/mgw llm list         # List LLMs
./dist/mgw llm create       # Create LLM
./dist/mgw llm get <id>     # Get LLM details
./dist/mgw llm update <id>  # Update LLM
./dist/mgw llm delete <id>  # Delete LLM
./dist/mgw llm stats <id>   # LLM statistics
```

#### Application Management
```bash
./dist/mgw app list         # List applications
./dist/mgw app create       # Create application
./dist/mgw app get <id>     # Get application details
./dist/mgw app update <id>  # Update application
./dist/mgw app delete <id>  # Delete application
./dist/mgw app llms <id>    # Manage LLM associations
```

#### Token Management
```bash
./dist/mgw token list       # List tokens
./dist/mgw token create     # Create token
./dist/mgw token info <id>  # Token information
./dist/mgw token validate   # Validate token
./dist/mgw token revoke     # Revoke token
```

#### Budget & Analytics
```bash
./dist/mgw budget list      # List budgets
./dist/mgw budget usage     # Budget usage
./dist/mgw budget update    # Update budget
./dist/mgw budget history   # Budget history

./dist/mgw analytics events   # Analytics events
./dist/mgw analytics summary  # Usage summary
./dist/mgw analytics costs    # Cost analysis
```

## Development

### CLI Development
```bash
# Run CLI in development
go run ./cmd/mgw --help

# Build and test
go build -o dist/mgw ./cmd/mgw
./dist/mgw --version
```

### Testing CLI
```bash
# Run CLI-specific tests
go test ./cmd/mgw/...

# Integration tests (requires running microgateway)
./scripts/test-cli-integration.sh
```

## Troubleshooting

### Connection Issues
```bash
# Test connectivity
./dist/mgw system health

# Check configuration
./dist/mgw system config

# Debug mode
./dist/mgw --verbose system health
```

### Authentication Issues
```bash
# Validate token
./dist/mgw token validate

# Check token scopes
./dist/mgw token info

# Test with explicit token
./dist/mgw --token=your-token system health
```

### Build Issues
Same troubleshooting steps as [server compilation](compiling.md#troubleshooting).

## CLI Distribution

### Create CLI Packages
```bash
# Build CLI for all platforms
make build-cli-all

# Package for distribution
tar -czf mgw-cli-linux-amd64.tar.gz dist/mgw-linux-amd64
tar -czf mgw-cli-darwin-amd64.tar.gz dist/mgw-darwin-amd64
tar -czf mgw-cli-darwin-arm64.tar.gz dist/mgw-darwin-arm64
zip mgw-cli-windows-amd64.zip dist/mgw-windows-amd64.exe
```

### Shell Completion
```bash
# Generate shell completion (future feature)
./dist/mgw completion bash > mgw-completion.bash
./dist/mgw completion zsh > mgw-completion.zsh
./dist/mgw completion fish > mgw-completion.fish
```

## Installation Scripts

### Quick Install Script
```bash
#!/bin/bash
# install-mgw.sh

set -e

# Detect OS and architecture
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case $ARCH in
    x86_64) ARCH="amd64";;
    arm64|aarch64) ARCH="arm64";;
    *) echo "Unsupported architecture: $ARCH"; exit 1;;
esac

# Download and install
BINARY="mgw-${OS}-${ARCH}"
if [[ "$OS" == "windows" ]]; then
    BINARY="${BINARY}.exe"
fi

echo "Downloading mgw for ${OS}/${ARCH}..."
curl -L "https://releases.../mgw-${OS}-${ARCH}.tar.gz" | tar -xz
sudo mv mgw /usr/local/bin/mgw
echo "mgw installed successfully!"
```

---

The CLI tool is now ready for use. See the [CLI Usage](cli-usage.md) guide for comprehensive usage examples and workflows.
