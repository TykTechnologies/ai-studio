# Test Infrastructure Containers

This package provides testcontainers-go helpers for integration testing across the Tyk AI Studio codebase.

## Overview

Using testcontainers-go allows us to:
- Run real external services (Redis, PostgreSQL, etc.) in Docker containers during tests
- Achieve reproducible, hermetic tests that work on any machine with Docker
- Avoid complex docker-compose setups for testing
- Share container setup patterns across the codebase

## Available Containers

### Redis (Single Node)

```go
import "github.com/TykTechnologies/midsommar/v2/pkg/testinfra/containers"

func TestWithRedis(t *testing.T) {
    ctx := context.Background()

    // Create Redis container with default config
    redis, err := containers.NewRedisContainer(ctx, nil)
    if err != nil {
        t.Fatal(err)
    }
    defer redis.Close(ctx)

    // Use redis.Addr() to get "host:port" for connection
    fmt.Println("Redis address:", redis.Addr())

    // Clear data between tests
    redis.FlushAll(ctx)
}
```

**Configuration options:**
```go
cfg := &containers.RedisConfig{
    Password: "mysecret",     // Optional: Redis password
    Version:  "7.2-alpine",   // Optional: Redis image tag
    Database: 0,              // Optional: Redis database number
}
redis, err := containers.NewRedisContainer(ctx, cfg)
```

### Redis Cluster

```go
import "github.com/TykTechnologies/midsommar/v2/pkg/testinfra/containers"

func TestWithRedisCluster(t *testing.T) {
    ctx := context.Background()

    cluster, err := containers.NewRedisClusterContainer(ctx, nil)
    if err != nil {
        t.Fatal(err)
    }
    defer cluster.Close(ctx)

    // Use cluster.Addrs() for cluster connection
    fmt.Println("Cluster addresses:", cluster.Addrs())
}
```

### Syslog Server

```go
import "github.com/TykTechnologies/midsommar/v2/pkg/testinfra/containers"

func TestWithSyslog(t *testing.T) {
    ctx := context.Background()

    syslog, err := containers.NewSyslogContainer(ctx, nil)
    if err != nil {
        t.Fatal(err)
    }
    defer syslog.Close(ctx)

    // TCP syslog: syslog.TCPAddr() or syslog.TCPAddrWithScheme()
    // UDP syslog: syslog.UDPAddr() or syslog.UDPAddrWithScheme()

    // Read logs to verify messages were received
    logs, err := syslog.ReadLogs(ctx)
    if err != nil {
        t.Fatal(err)
    }
    fmt.Println("Syslog messages:", logs)
}
```

## Suite Fixtures (TestMain Pattern)

For better performance, share containers across tests in a suite:

```go
//go:build integration

package mypackage_test

import (
    "context"
    "os"
    "testing"

    "github.com/TykTechnologies/midsommar/v2/pkg/testinfra/containers"
)

var sharedRedis *containers.RedisContainer

func TestMain(m *testing.M) {
    ctx := context.Background()

    var err error
    sharedRedis, err = containers.NewRedisContainer(ctx, nil)
    if err != nil {
        panic(err)
    }

    code := m.Run()

    sharedRedis.Close(ctx)
    os.Exit(code)
}

func TestSomething(t *testing.T) {
    // Use sharedRedis in your test
    ctx := context.Background()
    sharedRedis.FlushAll(ctx) // Clear data before test

    // ... test code ...
}
```

## Build Tags

Integration tests should use build tags to separate them from unit tests:

```go
//go:build integration

package mypackage_test
```

**Running tests:**
```bash
# Unit tests only (no Docker required)
go test ./...

# Integration tests (requires Docker)
go test -tags=integration ./...
```

## Requirements

- Docker must be running on the host machine
- The Docker socket must be accessible to the test process
- Sufficient resources for container startup (typically a few seconds per container)

## Adding New Containers

To add support for a new service:

1. Create a new file in this package (e.g., `postgres.go`)
2. Define a config struct and container wrapper
3. Implement `New*Container`, `Close`, and any service-specific helper methods
4. Update this README with usage examples

See `redis.go` for a complete example.
