# Testing Guide

This document is the single source of truth for running tests in the Tyk AI Studio (Midsommar) project.

## Quick Start

```bash
# Run all tests (unit + integration) - fastest way to validate changes
make test-all

# Run only unit tests (fast feedback loop)
make test-quick

# Run enterprise tests
make test-all EDITION=ent

# Run with verbose output and coverage
make test-all TEST_VERBOSE=true TEST_COVERAGE=true

# See all available test commands
make test-help
```

## Test Architecture

### Components

| Component | Location | Description |
|-----------|----------|-------------|
| **AI Studio (Go)** | Root | Main application backend (~280 test files) |
| **Microgateway** | `microgateway/` | Edge gateway component (separate go.mod) |
| **Frontend** | `ui/admin-frontend/` | React admin interface (Jest tests) |
| **Plugins** | `enterprise/plugins/`, `community/plugins/` | Plugin integration and E2E tests |
| **UI E2E** | `tests/ui/` | Playwright browser tests |

### Test Types

| Type | Description | Requirements |
|------|-------------|--------------|
| **unit** | Fast, isolated tests | None |
| **integration** | Tests requiring external services | Docker |
| **e2e** | Full end-to-end tests | Docker, built binaries |

### Editions

| Edition | Build Tag | Description |
|---------|-----------|-------------|
| **ce** (Community) | None | Open source features |
| **ent** (Enterprise) | `-tags enterprise` | Enterprise features (requires submodule) |

## Running Tests Locally

### Unified Test Commands

```bash
# Primary commands
make test-all                    # Run unit + integration tests
make test-quick                  # Unit tests only (fast)
make test-ci                     # CI-style tests with coverage

# Component-specific
make test-studio-unit            # AI Studio Go unit tests
make test-studio-integration     # AI Studio integration tests
make test-microgateway-unit      # Microgateway unit tests
make test-microgateway-integration  # Microgateway integration tests
make test-frontend-unit          # Frontend Jest tests
make test-plugins-unit           # Plugin unit tests
make test-plugins-integration    # Plugin integration tests (Docker)
make test-plugins-e2e            # Plugin E2E tests (Docker)
make test-ui-e2e                 # UI Playwright tests
make test-ui-e2e-with-env        # UI tests with auto-started environment
```

### Configuration Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `TEST_EDITION` | auto-detect | `ce` or `ent` |
| `TEST_COMPONENTS` | `studio microgateway frontend plugins` | Space-separated list |
| `TEST_TYPES` | `unit integration` | Space-separated list |
| `TEST_VERBOSE` | `false` | Enable verbose output |
| `TEST_COVERAGE` | `false` | Generate coverage.out |
| `TEST_TIMEOUT` | `30m` | Test timeout duration |

### Examples

```bash
# Test only studio with verbose output
make test-all TEST_COMPONENTS="studio" TEST_VERBOSE=true

# Test only unit tests for enterprise edition
make test-all TEST_TYPES="unit" EDITION=ent

# Test everything with coverage
make test-all TEST_COVERAGE=true

# Test specific components
make test-all TEST_COMPONENTS="studio frontend" TEST_TYPES="unit"
```

## Prerequisites

### For Unit Tests

- Go 1.22+ (check `go.mod` for exact version)
- Node.js 20+ (for frontend tests)

### For Integration Tests

- Docker and Docker Compose
- Enterprise submodule (for enterprise plugin tests)

### For UI E2E Tests

- Docker and Docker Compose
- Chromium browser (installed via Playwright)

### Enterprise Edition Setup

```bash
# Initialize enterprise submodule
make init-enterprise

# Verify edition
make show-edition
```

### Environment Variables

For some tests, you may need:

```bash
# LLM provider keys (for certain integration tests)
export OPENAI_API_KEY=your-key
export ANTHROPIC_AI_KEY=your-key

# License (for enterprise features)
export TYK_AI_LICENSE=your-license
export TYK_AI_SECRET_KEY=your-secret
```

## CI/CD

### Automatic Tests (on every PR)

The following tests run automatically on pull requests:

1. **Frontend Tests** - Jest unit tests for React components
2. **Go Unit Tests** - Matrix testing for CE and ENT editions
3. **Documentation Build** - Validates docs build correctly

Workflow: `.github/workflows/ci-test.yml`

### UI E2E Tests (on PR and main)

Playwright browser tests run on:
- Pull requests
- Pushes to `main`, `release-*`, `QA-*` branches

Workflow: `.github/workflows/ui-tests.yml`

### Comprehensive Tests (manual trigger)

For heavy E2E and full integration tests, use the manual workflow:

1. Go to Actions > "Comprehensive Test Suite"
2. Click "Run workflow"
3. Select options:
   - Edition: `ce`, `ent`, or `both`
   - Include plugin E2E: yes/no
   - Include UI E2E: yes/no
   - Include full integration (Redis cluster + syslog): yes/no

Workflow: `.github/workflows/test-comprehensive.yml`

## Writing Tests

### Build Tags

Use build tags to control when tests run:

```go
//go:build integration

package mypackage

import "testing"

func TestMyIntegration(t *testing.T) {
    // This test only runs with -tags integration
}
```

Common tags:
- `integration` - Requires Docker containers
- `e2e` - End-to-end tests
- `enterprise` - Requires enterprise features

### Test Infrastructure

The project provides test infrastructure in `pkg/testinfra/`:

```go
import (
    "context"
    "testing"

    "github.com/TykTechnologies/midsommar/v2/pkg/testinfra/containers"
    "github.com/stretchr/testify/require"
)

func TestWithRedis(t *testing.T) {
    ctx := context.Background()

    // Start a Redis container
    redis, err := containers.NewRedisContainer(ctx, nil)
    require.NoError(t, err)
    defer redis.Close(ctx)

    // Use redis.Addr() for connection
    addr := redis.Addr()
    // ... your test code
}
```

Available containers:
- `RedisContainer` - Single Redis instance
- `RedisClusterContainer` - Redis cluster
- `VaultContainer` - HashiCorp Vault
- `SyslogContainer` - Syslog server
- `ChromaContainer` - Chroma vector DB
- `PgVectorContainer` - PostgreSQL with pgvector
- `QdrantContainer` - Qdrant vector DB
- `WeaviateContainer` - Weaviate vector DB

### Plugin Testing

For plugin tests, see the examples in:
- `enterprise/plugins/advanced-llm-cache/tests/`
- `enterprise/plugins/llm-load-balancer/tests/`

## Troubleshooting

### Tests Timeout

Increase the timeout:
```bash
make test-all TEST_TIMEOUT=60m
```

### Enterprise Tests Skip

Ensure the enterprise submodule is initialized:
```bash
make init-enterprise
make show-edition  # Should show "ent"
```

### Docker Container Issues

Clean up and retry:
```bash
docker compose -f tests/compose.yml down -v --remove-orphans
make test-plugins-integration
```

### Frontend Tests Fail

Ensure dependencies are installed:
```bash
cd ui/admin-frontend
rm -rf node_modules
npm ci
npm test
```

### Coverage Report

Generate and view coverage:
```bash
make test-all TEST_COVERAGE=true
go tool cover -html=coverage.out -o coverage.html
open coverage.html
```

### Debugging Specific Tests

Run a single test with verbose output:
```bash
# Go test
go test -v -run TestMyFunction ./path/to/package/...

# With enterprise tag
go test -v -tags enterprise -run TestMyFunction ./path/to/package/...
```

## Legacy Test Commands

These commands still work but `test-all` is preferred:

```bash
make test                        # Basic Go tests (studio + microgateway)
make test-integration            # Enterprise plugin integration tests
make test-integration-plugin-cache  # Cache plugin tests
make test-integration-plugin-cache-full  # Full cache plugin tests
```

## Test File Locations

| Test Type | Location Pattern |
|-----------|------------------|
| Go unit tests | `*_test.go` co-located with source |
| Go integration tests | `tests/integration/` or `*/tests/integration/` |
| Frontend tests | `*.test.js` in `ui/admin-frontend/src/` |
| UI E2E tests | `tests/ui/tests/*.spec.ts` |
| Plugin tests | `enterprise/plugins/*/tests/` |
