# Compiling the Microgateway

This guide covers building the microgateway server from source.

## Prerequisites

### Development Requirements
- **Go 1.23.0+** (with toolchain go1.23.1)
- **Git** for version control
- **Make** (optional but recommended)

### System Requirements
- **Memory**: 256MB minimum, 512MB recommended
- **CPU**: 1 core minimum, 2 cores recommended  
- **Storage**: 1GB minimum for application and analytics data

## Quick Build

```bash
# Clone repository (if part of midsommar project)
cd microgateway

# Build server binary
make build

# Or using go build directly
go build -o dist/microgateway ./cmd/microgateway
```

## Build Options

### Standard Build
```bash
# Using Makefile (recommended)
make build

# Using go build directly
go build -o dist/microgateway ./cmd/microgateway
```

### Build with Version Information
```bash
# Makefile includes git version, hash, and build time automatically
make build

# Manual version flags
VERSION=$(git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_HASH=$(git rev-parse HEAD 2>/dev/null || echo "unknown")
BUILD_TIME=$(date -u +%Y-%m-%dT%H:%M:%SZ)

go build -ldflags "-X main.Version=${VERSION} -X main.BuildHash=${BUILD_HASH} -X main.BuildTime=${BUILD_TIME}" \
  -o dist/microgateway ./cmd/microgateway
```

### Cross-Platform Builds
```bash
# Build for all supported platforms
make build-all

# Manual cross-compilation examples
GOOS=linux GOARCH=amd64 go build -o dist/microgateway-linux-amd64 ./cmd/microgateway
GOOS=darwin GOARCH=arm64 go build -o dist/microgateway-darwin-arm64 ./cmd/microgateway
GOOS=windows GOARCH=amd64 go build -o dist/microgateway-windows-amd64.exe ./cmd/microgateway
```

## Installation

### Install Dependencies
```bash
# Download Go dependencies
go mod download

# Verify dependencies
go mod verify

# Clean up if needed
go mod tidy
```

### Install Development Tools (Optional)
```bash
# Hot reload during development
go install github.com/cosmtrek/air@latest

# Linting
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Security scanning
go install github.com/securecodewarrior/gosec/v2/cmd/gosec@latest
```

## Build Verification

### Test Binary
```bash
# Check version
./dist/microgateway -version

# Check help
./dist/microgateway -help

# Test database migration (requires configuration)
./dist/microgateway -migrate
```

### Run Tests
```bash
# Run all tests
make test

# Run specific test types
make test-unit
make test-integration

# Generate coverage report
make coverage
```

## Development Workflow

### Local Development
```bash
# Set up development environment
cp configs/.env.example .env
# Edit .env for local settings

# Run with hot reload (if air installed)
air

# Or run normally
make run
```

### Code Quality
```bash
# Format code
make fmt

# Vet code
make vet

# Run linter
make lint

# Security scan
make security

# Run all checks
make fmt vet lint test
```

## Build Optimization

### Release Build
```bash
# Optimized build for distribution
go build -ldflags "-s -w" -o dist/microgateway ./cmd/microgateway

# With version information
make build  # Already includes optimization flags
```

### Size Optimization
```bash
# Strip debug information and reduce binary size
go build -ldflags "-s -w" -trimpath -o dist/microgateway ./cmd/microgateway

# Using UPX compression (optional)
upx --best dist/microgateway
```

## Troubleshooting

### Common Build Issues

#### Module Resolution Errors
```bash
# Clean module cache
go clean -modcache
go mod download
```

#### Version Conflicts
```bash
# Check Go version
go version  # Should be 1.23.0+

# Update dependencies
go get -u
go mod tidy
```

#### Missing Dependencies
```bash
# Install all dependencies
go mod download

# Verify replace directives work
go list -m all | grep midsommar
```

### Build Environment Issues

#### CGO Issues
```bash
# Disable CGO if not needed
CGO_ENABLED=0 go build -o dist/microgateway ./cmd/microgateway
```

#### Memory Issues During Build
```bash
# Increase Go build memory limit
GOGC=off go build -o dist/microgateway ./cmd/microgateway
```

## Docker Build

### Standard Docker Build
```bash
# Build Docker image
docker build -f deployments/Dockerfile -t microgateway:latest .

# Using Makefile
make docker-build
```

### Multi-Stage Build
```bash
# The Dockerfile uses multi-stage builds for optimization
# Final image size is typically < 50MB
```

## Binary Distribution

### Create Release Archives
```bash
# Build all platforms
make build-all

# Create release archives
tar -czf microgateway-linux-amd64.tar.gz dist/microgateway-linux-amd64
tar -czf microgateway-darwin-amd64.tar.gz dist/microgateway-darwin-amd64
tar -czf microgateway-darwin-arm64.tar.gz dist/microgateway-darwin-arm64
zip microgateway-windows-amd64.zip dist/microgateway-windows-amd64.exe
```

### Checksums
```bash
# Generate checksums for release verification
cd dist
sha256sum microgateway-* > checksums.txt
```

## Performance Considerations

### Build Performance
- Use `go build -a` to force rebuilding of all packages
- Use `GOCACHE=off` to disable build cache for clean builds
- Use `make -j$(nproc)` for parallel make operations

### Runtime Performance
- Release builds automatically include optimization flags
- Use `GOGC` environment variable to tune garbage collector
- Profile with `go tool pprof` for performance analysis

---

The microgateway binary is now ready for deployment. See the [CLI Compilation](cli-compilation.md) guide for building the management CLI tool.
