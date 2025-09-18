# Microgateway Build and Deployment Guide

This document provides complete instructions for building, compiling, and deploying the microgateway.

## Prerequisites

### Development Requirements
- **Go 1.23.0+** (with toolchain go1.23.1)
- **Git** for version control
- **Make** (optional but recommended)
- **Docker** (for containerized builds)
- **PostgreSQL 14+** or **SQLite 3** for database

### System Requirements
- **Memory**: 256MB minimum, 512MB recommended
- **CPU**: 1 core minimum, 2 cores recommended
- **Storage**: 1GB minimum for application and analytics data
- **Network**: HTTP/HTTPS ports (default: 8080)

## Building from Source

### 1. Clone Repository
```bash
# Clone the main midsommar repository
git clone https://github.com/TykTechnologies/midsommar.git
cd midsommar/microgateway

# Or if microgateway is standalone
git clone <microgateway-repo>
cd microgateway
```

### 2. Install Dependencies
```bash
# Download Go dependencies
go mod download

# Verify dependencies
go mod verify

# Clean up if needed
go mod tidy
```

### 3. Build Options

#### Build Server Only
```bash
# Using Makefile (recommended)
make build

# Using go build directly
go build -o dist/microgateway ./cmd/microgateway

# With version information
make build  # Includes git version, hash, and build time
```

#### Build CLI Only
```bash
# Using Makefile (recommended)
make build-cli

# Using go build directly
go build -o dist/mgw ./cmd/mgw
```

#### Build Both
```bash
# Build both server and CLI
make build-both

# Or separately
make build
make build-cli
```

#### Cross-Platform Builds
```bash
# Build for all supported platforms
make build-all        # Server for all platforms
make build-cli-all    # CLI for all platforms

# Manual cross-compilation
GOOS=linux GOARCH=amd64 go build -o dist/microgateway-linux-amd64 ./cmd/microgateway
GOOS=darwin GOARCH=arm64 go build -o dist/microgateway-darwin-arm64 ./cmd/microgateway
GOOS=windows GOARCH=amd64 go build -o dist/microgateway-windows-amd64.exe ./cmd/microgateway
```

### 4. Build Verification
```bash
# Test server binary
./dist/microgateway -version

# Test CLI binary
./dist/mgw --help

# Run basic health check
./dist/microgateway &  # Start server
./dist/mgw system health  # Test with CLI
kill %1  # Stop server
```

## Configuration Setup

### 1. Environment Configuration
```bash
# Copy example configuration
cp configs/.env.example .env

# Edit configuration
nano .env
```

### 2. Database Setup

#### SQLite (Development)
```bash
# SQLite requires no setup - database file created automatically
DATABASE_TYPE=sqlite
DATABASE_DSN=file:./data/microgateway.db?cache=shared&mode=rwc
```

#### PostgreSQL (Production)
```bash
# Create database and user
sudo -u postgres psql
postgres=# CREATE DATABASE microgateway;
postgres=# CREATE USER mgw_user WITH PASSWORD 'secure_password';
postgres=# GRANT ALL PRIVILEGES ON DATABASE microgateway TO mgw_user;
postgres=# \q

# Configure connection
DATABASE_TYPE=postgres
DATABASE_DSN=postgres://mgw_user:secure_password@localhost:5432/microgateway?sslmode=require
```

### 3. Security Keys
```bash
# Generate JWT secret (32+ characters)
JWT_SECRET=$(openssl rand -hex 32)

# Generate encryption key (exactly 32 characters)
ENCRYPTION_KEY=$(openssl rand -hex 16)

# Add to .env file
echo "JWT_SECRET=$JWT_SECRET" >> .env
echo "ENCRYPTION_KEY=$ENCRYPTION_KEY" >> .env
```

## Running and Testing

### 1. Database Migration
```bash
# Run migrations (automatic if DB_AUTO_MIGRATE=true)
./dist/microgateway -migrate

# Or set auto-migration in .env
DB_AUTO_MIGRATE=true
```

### 2. Start Services
```bash
# Start microgateway server
./dist/microgateway

# Server starts on configured port (default: 8080)
# Logs show startup information and health status
```

### 3. Basic Testing
```bash
# Health checks
curl http://localhost:8080/health
curl http://localhost:8080/ready

# API testing (requires admin token)
export MGW_URL="http://localhost:8080"
export MGW_TOKEN="your-admin-token"

# Test CLI
./dist/mgw system health
./dist/mgw llm list
```

## Docker Deployment

### 1. Build Docker Image
```bash
# Build from Dockerfile
docker build -f deployments/Dockerfile -t microgateway:latest .

# Using Makefile
make docker-build
```

### 2. Run with Docker Compose
```bash
# Start full stack (microgateway + PostgreSQL)
docker-compose -f deployments/docker-compose.yml up -d

# View logs
docker-compose logs -f microgateway

# Stop services
docker-compose down
```

### 3. Environment Variables for Docker
```yaml
# docker-compose.yml environment section
environment:
  DATABASE_TYPE: postgres
  DATABASE_DSN: postgres://gateway:gateway123@postgres:5432/microgateway?sslmode=disable
  JWT_SECRET: ${JWT_SECRET}
  ENCRYPTION_KEY: ${ENCRYPTION_KEY}
  LOG_LEVEL: info
  DB_AUTO_MIGRATE: "true"
```

## Kubernetes Deployment

### 1. Create Namespace
```bash
kubectl create namespace ai-gateway
```

### 2. Deploy Configuration
```bash
# Create ConfigMap
kubectl create configmap microgateway-config \
  --from-literal=LOG_LEVEL=info \
  --from-literal=CACHE_ENABLED=true \
  --from-literal=ANALYTICS_ENABLED=true \
  -n ai-gateway

# Create Secrets
kubectl create secret generic microgateway-secrets \
  --from-literal=jwt-secret=$JWT_SECRET \
  --from-literal=encryption-key=$ENCRYPTION_KEY \
  --from-literal=database-dsn=$DATABASE_DSN \
  -n ai-gateway
```

### 3. Deploy Application
```bash
# Apply Kubernetes manifests
kubectl apply -f deployments/k8s/ -n ai-gateway

# Check deployment status
kubectl get pods -n ai-gateway
kubectl get services -n ai-gateway

# View logs
kubectl logs -f deployment/microgateway -n ai-gateway
```

### 4. Kubernetes Health Checks
```yaml
# Liveness probe
livenessProbe:
  httpGet:
    path: /health
    port: 8080
  initialDelaySeconds: 10
  periodSeconds: 10

# Readiness probe  
readinessProbe:
  httpGet:
    path: /ready
    port: 8080
  initialDelaySeconds: 5
  periodSeconds: 5
```

## Production Deployment

### 1. Infrastructure Setup
```bash
# Database
- PostgreSQL 14+ with appropriate sizing
- Connection pooling configured
- Regular backups scheduled
- Monitoring enabled

# Load Balancer
- Health check endpoints configured
- SSL termination (optional)
- Rate limiting (optional)

# Monitoring
- Prometheus metrics scraping
- Log aggregation configured
- Alerting rules set up
```

### 2. Security Hardening
```bash
# TLS Configuration
TLS_ENABLED=true
TLS_CERT_PATH=/etc/ssl/certs/microgateway.crt
TLS_KEY_PATH=/etc/ssl/private/microgateway.key

# Security Settings
ENABLE_RATE_LIMITING=true
ENABLE_IP_WHITELIST=true
JWT_SECRET=<strong-random-secret-32chars+>
ENCRYPTION_KEY=<strong-random-key-32chars>
```

### 3. Performance Tuning
```bash
# Database Connections
DB_MAX_OPEN_CONNS=50
DB_MAX_IDLE_CONNS=25
DB_CONN_MAX_LIFETIME=10m

# Cache Settings
CACHE_MAX_SIZE=10000
CACHE_TTL=30m

# Analytics
ANALYTICS_BUFFER_SIZE=5000
ANALYTICS_FLUSH_INTERVAL=5s
```

### 4. Monitoring Setup
```bash
# Prometheus configuration
ENABLE_METRICS=true
METRICS_PATH=/metrics

# Logging
LOG_LEVEL=info
LOG_FORMAT=json

# Health checks
# Configure load balancer to use /health and /ready endpoints
```

## Development Workflow

### 1. Local Development
```bash
# Set up development environment
cp .env.example .env
# Edit .env for local settings

# Install air for hot reloading (optional)
go install github.com/cosmtrek/air@latest

# Run with hot reload
air
# Or run normally
make run
```

### 2. Testing
```bash
# Run all tests
make test

# Run specific test types
make test-unit
make test-integration

# Generate coverage report
make coverage
open coverage.html
```

### 3. Code Quality
```bash
# Format code
go fmt ./...

# Vet code
go vet ./...

# Run linter (if golangci-lint installed)
golangci-lint run

# Using Makefile
make fmt vet lint
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
# Ensure correct Go version
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

### Common Runtime Issues

#### Database Connection Errors
```bash
# Check database connectivity
psql $DATABASE_DSN

# Verify DSN format
# postgres://user:password@host:port/database?options
```

#### Authentication Failures
```bash
# Verify JWT secret is set
echo $JWT_SECRET

# Check token generation
./dist/mgw token create --app-id=1 --name="test"
```

#### Port Conflicts
```bash
# Check if port is in use
lsof -i :8080

# Use different port
PORT=8081 ./dist/microgateway
```

## CI/CD Integration

### GitHub Actions Example
```yaml
name: Build and Test

on: [push, pull_request]

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
    - uses: actions/checkout@v3
    - uses: actions/setup-go@v3
      with:
        go-version: 1.23.0
    
    - name: Build
      run: |
        cd microgateway
        make build-both
    
    - name: Test
      run: |
        cd microgateway  
        make test
    
    - name: Upload artifacts
      uses: actions/upload-artifact@v3
      with:
        name: binaries
        path: microgateway/dist/
```

### Docker Registry Publishing
```bash
# Tag and push Docker image
docker tag microgateway:latest your-registry.com/microgateway:v1.0.0
docker push your-registry.com/microgateway:v1.0.0

# Using Makefile
make docker-build docker-push
```

## Performance Benchmarks

### Typical Performance
- **Throughput**: 1,000+ requests/second
- **Latency**: <10ms overhead (plus LLM response time)
- **Memory**: 100-200MB under normal load
- **CPU**: 10-20% under normal load

### Load Testing
```bash
# Install hey for load testing
go install github.com/rakyll/hey@latest

# Test health endpoint
hey -n 10000 -c 100 http://localhost:8080/health

# Test API endpoint (with auth)
hey -n 1000 -c 10 -H "Authorization: Bearer $TOKEN" \
    http://localhost:8080/api/v1/llms
```

## Backup and Recovery

### Database Backup
```bash
# PostgreSQL backup
pg_dump $DATABASE_DSN > microgateway_backup.sql

# SQLite backup
cp data/microgateway.db microgateway_backup.db
```

### Configuration Backup
```bash
# Backup configuration
tar -czf microgateway-config-$(date +%Y%m%d).tar.gz \
    .env configs/ data/

# Backup using CLI (export all configurations)
./dist/mgw llm list --format=yaml > backup/llms.yaml
./dist/mgw app list --format=yaml > backup/apps.yaml
```

### Disaster Recovery
```bash
# Restore database
psql $DATABASE_DSN < microgateway_backup.sql

# Restart services
./dist/microgateway -migrate  # Run migrations if needed
./dist/microgateway           # Start service
```

This comprehensive build and deployment guide ensures successful microgateway deployment in any environment from development to enterprise production.