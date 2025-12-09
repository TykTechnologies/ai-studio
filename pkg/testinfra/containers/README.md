# Test Infrastructure Containers

This package provides testcontainers-go helpers for integration testing across the Tyk AI Studio codebase.

## Overview

Using testcontainers-go allows us to:
- Run real external services (Redis, Chroma, Vault, etc.) in Docker containers during tests
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

### Chroma (Vector Database)

```go
import "github.com/TykTechnologies/midsommar/v2/pkg/testinfra/containers"

func TestWithChroma(t *testing.T) {
    ctx := context.Background()

    // Create Chroma container with default config
    chroma, err := containers.NewChromaContainer(ctx, nil)
    if err != nil {
        t.Fatal(err)
    }
    defer chroma.Close(ctx)

    // Use chroma.Addr() to get "http://host:port" for connection
    fmt.Println("Chroma address:", chroma.Addr())

    // Health check
    err = chroma.Ping(ctx)
    if err != nil {
        t.Fatal(err)
    }

    // Create a collection
    err = chroma.CreateCollection(ctx, "my-collection")
    if err != nil {
        t.Fatal(err)
    }

    // Reset all data between tests
    chroma.Reset(ctx)
}
```

**Configuration options:**
```go
cfg := &containers.ChromaConfig{
    Version: "1.3.5",  // Optional: Chroma image tag (default: "1.3.5")
}
chroma, err := containers.NewChromaContainer(ctx, cfg)
```

**Helper methods:**
```go
// Health check via heartbeat endpoint
err := chroma.Ping(ctx)

// Collection management
err = chroma.CreateCollection(ctx, "collection-name")
err = chroma.DeleteCollection(ctx, "collection-name")

// Clear all data (useful between tests)
err = chroma.Reset(ctx)
```

### PGVector (PostgreSQL with Vector Extension)

```go
import "github.com/TykTechnologies/midsommar/v2/pkg/testinfra/containers"

func TestWithPGVector(t *testing.T) {
    ctx := context.Background()

    // Create PGVector container with default config
    pg, err := containers.NewPGVectorContainer(ctx, nil)
    if err != nil {
        t.Fatal(err)
    }
    defer pg.Close(ctx)

    // Use pg.ConnectionString() for database connection
    fmt.Println("PGVector connection:", pg.ConnectionString())

    // Health check
    err = pg.Ping(ctx)
    if err != nil {
        t.Fatal(err)
    }

    // Create a vector table (384 dimensions)
    err = pg.CreateVectorTable(ctx, "my_vectors", 384)
    if err != nil {
        t.Fatal(err)
    }

    // Drop table when done
    pg.DropTable(ctx, "my_vectors")
}
```

**Configuration options:**
```go
cfg := &containers.PGVectorConfig{
    Version:  "0.8.0-pg16",  // Optional: pgvector image tag (default: "0.8.0-pg16")
    User:     "testuser",    // Optional: PostgreSQL user (default: "testuser")
    Password: "testpass",    // Optional: PostgreSQL password (default: "testpass")
    Database: "testdb",      // Optional: PostgreSQL database (default: "testdb")
}
pg, err := containers.NewPGVectorContainer(ctx, cfg)
```

**Helper methods:**
```go
// Health check via database ping
err := pg.Ping(ctx)

// Connection details
connStr := pg.ConnectionString()  // Full postgres:// URL
host := pg.Host()
port := pg.Port()
user := pg.User()
password := pg.Password()
database := pg.Database()

// Table management
err = pg.CreateVectorTable(ctx, "table_name", 384)  // Creates id, content, embedding, metadata columns
err = pg.DropTable(ctx, "table_name")
exists, err := pg.TableExists(ctx, "table_name")
count, err := pg.TableRowCount(ctx, "table_name")

// Direct SQL execution
err = pg.Exec(ctx, "INSERT INTO ...", args...)
rows, err := pg.Query(ctx, "SELECT ...", args...)
```

**Table schema:**
```sql
-- Created by CreateVectorTable
CREATE TABLE table_name (
    id VARCHAR(36) PRIMARY KEY,
    content TEXT NOT NULL,
    embedding vector(384),  -- dimension specified at creation
    metadata JSONB
);
```

### Qdrant (High-Performance Vector Database)

```go
import "github.com/TykTechnologies/midsommar/v2/pkg/testinfra/containers"

func TestWithQdrant(t *testing.T) {
    ctx := context.Background()

    // Create Qdrant container with default config
    qdrant, err := containers.NewQdrantContainer(ctx, nil)
    if err != nil {
        t.Fatal(err)
    }
    defer qdrant.Close(ctx)

    // Use qdrant.Addr() to get "http://host:port" for REST API
    fmt.Println("Qdrant address:", qdrant.Addr())

    // Health check
    err = qdrant.Ping(ctx)
    if err != nil {
        t.Fatal(err)
    }

    // Create a collection (384 dimensions, cosine distance)
    err = qdrant.CreateCollection(ctx, "my_collection", 384)
    if err != nil {
        t.Fatal(err)
    }

    // Delete collection when done
    qdrant.DeleteCollection(ctx, "my_collection")
}
```

**Configuration options:**
```go
cfg := &containers.QdrantConfig{
    Version: "v1.16.1",  // Optional: Qdrant image tag (default: "v1.16.1")
}
qdrant, err := containers.NewQdrantContainer(ctx, cfg)
```

**Helper methods:**
```go
// Health check via /healthz endpoint
err := qdrant.Ping(ctx)

// Connection details
addr := qdrant.Addr()          // HTTP REST API: "http://host:port"
grpcAddr := qdrant.GRPCAddr()  // gRPC: "host:port"
host := qdrant.Host()
httpPort := qdrant.HTTPPort()  // Default: 6333
grpcPort := qdrant.GRPCPort()  // Default: 6334

// Collection management
err = qdrant.CreateCollection(ctx, "collection_name", 384)  // Creates collection with cosine distance
err = qdrant.DeleteCollection(ctx, "collection_name")
exists, err := qdrant.CollectionExists(ctx, "collection_name")
count, err := qdrant.CollectionPointCount(ctx, "collection_name")
names, err := qdrant.ListCollections(ctx)

// Point operations
points := []containers.QdrantPoint{
    {ID: "uuid-1", Vector: []float32{...}, Payload: map[string]interface{}{"key": "value"}},
}
err = qdrant.UpsertPoints(ctx, "collection_name", points)
results, err := qdrant.SearchPoints(ctx, "collection_name", queryVector, limit)
err = qdrant.DeletePoints(ctx, "collection_name", filter)
```

**Data structures:**
```go
// Point for storage
type QdrantPoint struct {
    ID      string                 // UUID string
    Vector  []float32              // Embedding vector
    Payload map[string]interface{} // Metadata
}

// Search result
type QdrantSearchResult struct {
    ID      string
    Score   float32
    Payload map[string]interface{}
}
```

### Weaviate (AI-Native Vector Database)

```go
import "github.com/TykTechnologies/midsommar/v2/pkg/testinfra/containers"

func TestWithWeaviate(t *testing.T) {
    ctx := context.Background()

    // Create Weaviate container with default config
    weaviate, err := containers.NewWeaviateContainer(ctx, nil)
    if err != nil {
        t.Fatal(err)
    }
    defer weaviate.Close(ctx)

    // Use weaviate.Addr() to get "http://host:port" for REST API
    fmt.Println("Weaviate address:", weaviate.Addr())

    // Health check
    err = weaviate.Ping(ctx)
    if err != nil {
        t.Fatal(err)
    }

    // Create a class (384 dimensions, cosine distance)
    err = weaviate.CreateClass(ctx, "MyClass", 384)
    if err != nil {
        t.Fatal(err)
    }

    // Delete class when done
    weaviate.DeleteClass(ctx, "MyClass")
}
```

**Configuration options:**
```go
cfg := &containers.WeaviateConfig{
    Version: "1.34.0",  // Optional: Weaviate image tag (default: "1.34.0")
}
weaviate, err := containers.NewWeaviateContainer(ctx, cfg)
```

**Helper methods:**
```go
// Health check via /v1/.well-known/ready endpoint
err := weaviate.Ping(ctx)

// Connection details
addr := weaviate.Addr()          // HTTP REST API: "http://host:port"
grpcAddr := weaviate.GRPCAddr()  // gRPC: "host:port"
host := weaviate.Host()
httpPort := weaviate.HTTPPort()  // Default: 8080
grpcPort := weaviate.GRPCPort()  // Default: 50051

// Class management
err = weaviate.CreateClass(ctx, "ClassName", 384)  // Creates class with cosine distance
err = weaviate.DeleteClass(ctx, "ClassName")
exists, err := weaviate.ClassExists(ctx, "ClassName")
count, err := weaviate.ClassObjectCount(ctx, "ClassName")
names, err := weaviate.ListClasses(ctx)

// Object operations
objects := []containers.WeaviateObject{
    {ID: "uuid-1", Vector: []float32{...}, Properties: map[string]interface{}{"content": "text"}},
}
err = weaviate.InsertObjects(ctx, "ClassName", objects)
results, err := weaviate.SearchObjects(ctx, "ClassName", queryVector, limit)
err = weaviate.DeleteObjectsByFilter(ctx, "ClassName", "propertyName", "value")
```

**Data structures:**
```go
// Object for storage
type WeaviateObject struct {
    ID         string                 // UUID string
    Vector     []float32              // Embedding vector
    Properties map[string]interface{} // Properties (content, source, etc.)
}

// Search result
type WeaviateSearchResult struct {
    ID         string
    Distance   float32                // Lower = more similar
    Properties map[string]interface{}
}
```

### HashiCorp Vault

```go
import "github.com/TykTechnologies/midsommar/v2/pkg/testinfra/containers"

func TestWithVault(t *testing.T) {
    ctx := context.Background()

    // Create Vault container in dev mode
    vault, err := containers.NewVaultContainer(ctx, nil)
    if err != nil {
        t.Fatal(err)
    }
    defer vault.Close(ctx)

    // Get connection details
    fmt.Println("Vault address:", vault.Addr())  // http://host:port
    fmt.Println("Vault token:", vault.Token())   // root token for dev mode

    // Vault dev mode has KV v2 enabled at "secret/" by default
}
```

**Configuration options:**
```go
cfg := &containers.VaultConfig{
    RootToken: "my-root-token",  // Optional: root token (default: "test-token")
    Version:   "1.15.4",         // Optional: Vault image tag (default: "latest")
}
vault, err := containers.NewVaultContainer(ctx, cfg)
```

**Helper methods:**
```go
// Enable KV v2 at a custom path (not needed for "secret/" which is enabled by default)
err := vault.EnableKVEngine(ctx, "custom-secrets")
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
