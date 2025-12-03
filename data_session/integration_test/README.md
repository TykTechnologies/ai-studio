# Datastore Integration Tests

Integration tests for self-hosted vector datastores used by AI Studio's DataSession.

## Prerequisites

- **Docker** must be running on your machine
- Docker socket must be accessible to the test process
- Sufficient memory for containers (~2GB recommended)

## Supported Datastores

| Datastore | Version | Docker Image | Port |
|-----------|---------|--------------|------|
| Chroma | 1.3.5 | `chromadb/chroma:1.3.5` | 8000 |
| PGVector | 0.8.0-pg16 | `pgvector/pgvector:0.8.0-pg16` | 5432 |
| Qdrant | v1.16.1 | `qdrant/qdrant:v1.16.1` | 6333 (HTTP), 6334 (gRPC) |
| Weaviate | 1.34.0 | `semitechnologies/weaviate:1.34.0` | 8080 (HTTP), 50051 (gRPC) |

## Running Tests

### Run All Integration Tests

```bash
# Standard run
go test -tags=integration ./data_session/integration_test/...

# With verbose output
go test -tags=integration -v ./data_session/integration_test/...

# With timeout (containers may take time to start)
go test -tags=integration -v -timeout 5m ./data_session/integration_test/...
```

### Run Specific Datastore Tests

```bash
# Chroma tests only
go test -tags=integration -v -run 'TestChroma' ./data_session/integration_test/...

# PGVector tests only
go test -tags=integration -v -run 'TestPGVector' ./data_session/integration_test/...

# Qdrant tests only
go test -tags=integration -v -run 'TestQdrant' ./data_session/integration_test/...

# Weaviate tests only
go test -tags=integration -v -run 'TestWeaviate' ./data_session/integration_test/...
```

### Run a Single Test

```bash
# Run specific test by name
go test -tags=integration -v -run 'TestChromaSearchByVector' ./data_session/integration_test/...
go test -tags=integration -v -run 'TestPGVectorDistanceOperators' ./data_session/integration_test/...
```

## Test Structure

```
data_session/integration_test/
├── README.md           # This file
├── suite_test.go       # TestMain setup, shared container management
├── common_test.go      # MockEmbedder and test helpers
├── chroma_test.go      # Chroma integration tests
├── pgvector_test.go    # PGVector integration tests
├── qdrant_test.go      # Qdrant integration tests
└── weaviate_test.go    # Weaviate integration tests
```

### Suite Setup (`suite_test.go`)

The `TestMain` function:
1. Starts all required containers once before any tests run
2. Verifies each container is healthy
3. Runs all tests
4. Cleans up containers after tests complete

This approach significantly speeds up test execution by reusing containers across all tests.

### Helper Functions

- `requireChroma(t)` - Returns shared Chroma container or skips test
- `requirePGVector(t)` - Returns shared PGVector container or skips test
- `requireQdrant(t)` - Returns shared Qdrant container or skips test
- `requireWeaviate(t)` - Returns shared Weaviate container or skips test
- `GenerateTestVectors(count, dimensions)` - Creates deterministic test vectors
- `GenerateTestContents(count)` - Creates test document contents
- `GenerateTestMetadatas(count)` - Creates test metadata maps

## Test Coverage

### Chroma Tests (10 tests)

| Test | Description |
|------|-------------|
| `TestChromaContainerHealth` | Verifies container health check |
| `TestChromaStoreDocumentsDirectly` | Stores documents with embeddings |
| `TestChromaSearchByVector` | Similarity search using query vector |
| `TestChromaSearchEmptyCollection` | Handles empty collection gracefully |
| `TestChromaDeleteByMetadata` | Deletes documents by metadata filter |
| `TestChromaQueryByMetadataOnly` | Queries documents by metadata |
| `TestChromaListNamespaces` | Lists all collections |
| `TestChromaDeleteNamespace` | Deletes entire collection |
| `TestChromaMultipleDocumentsStorage` | Batch document storage |
| `TestChromaDirectClientOperations` | Direct Chroma client API usage |

### PGVector Tests (12 tests)

| Test | Description |
|------|-------------|
| `TestPGVectorContainerHealth` | Verifies container health check |
| `TestPGVectorExtensionEnabled` | Confirms pgvector extension installed |
| `TestPGVectorStoreDocumentsDirectly` | Stores documents with embeddings |
| `TestPGVectorSearchByVector` | Cosine similarity search |
| `TestPGVectorSearchEmptyTable` | Handles empty table gracefully |
| `TestPGVectorDeleteByMetadata` | Deletes documents by JSON metadata |
| `TestPGVectorQueryByMetadataOnly` | Queries documents by metadata |
| `TestPGVectorListTables` | Lists tables with vector schema |
| `TestPGVectorDropTable` | Drops vector table |
| `TestPGVectorMultipleDocumentsStorage` | Batch document storage |
| `TestPGVectorUpsertBehavior` | Tests ON CONFLICT upsert |
| `TestPGVectorDistanceOperators` | Tests `<=>`, `<->`, `<#>` operators |

### Qdrant Tests (12 tests)

| Test | Description |
|------|-------------|
| `TestQdrantContainerHealth` | Verifies container health check |
| `TestQdrantCreateCollection` | Creates collection with cosine distance |
| `TestQdrantStoreDocumentsDirectly` | Stores points with vectors and payloads |
| `TestQdrantSearchByVector` | Similarity search using query vector |
| `TestQdrantSearchEmptyCollection` | Handles empty collection gracefully |
| `TestQdrantDeleteByFilter` | Deletes points by filter |
| `TestQdrantListCollections` | Lists all collections |
| `TestQdrantDeleteCollection` | Deletes entire collection |
| `TestQdrantMultipleDocumentsStorage` | Batch point storage |
| `TestQdrantUpsertBehavior` | Tests upsert with same ID |
| `TestQdrantSearchWithScoreThreshold` | Verifies score ordering |
| `TestQdrantPayloadRetrieval` | Verifies payload data returned correctly |

### Weaviate Tests (11 tests)

| Test | Description |
|------|-------------|
| `TestWeaviateContainerHealth` | Verifies container health check |
| `TestWeaviateCreateClass` | Creates class with cosine distance |
| `TestWeaviateStoreDocumentsDirectly` | Stores objects with vectors and properties |
| `TestWeaviateSearchByVector` | Similarity search using nearVector |
| `TestWeaviateSearchEmptyClass` | Handles empty class gracefully |
| `TestWeaviateDeleteByFilter` | Deletes objects by property filter |
| `TestWeaviateListClasses` | Lists all classes in schema |
| `TestWeaviateDeleteClass` | Deletes entire class |
| `TestWeaviateMultipleDocumentsStorage` | Batch object storage |
| `TestWeaviateSearchWithDistanceOrdering` | Verifies distance ordering |
| `TestWeaviatePropertiesRetrieval` | Verifies property data returned correctly |

## Troubleshooting

### Container Startup Issues

If containers fail to start:

```bash
# Check Docker is running
docker ps

# Check for port conflicts
lsof -i :8000  # Chroma port
lsof -i :5432  # PostgreSQL port
lsof -i :6333  # Qdrant HTTP port
lsof -i :6334  # Qdrant gRPC port
lsof -i :8080  # Weaviate HTTP port
lsof -i :50051 # Weaviate gRPC port

# Pull images manually if needed
docker pull chromadb/chroma:1.3.5
docker pull pgvector/pgvector:0.8.0-pg16
docker pull qdrant/qdrant:v1.16.1
docker pull semitechnologies/weaviate:1.34.0
```

### Test Timeout

If tests timeout during container startup:

```bash
# Increase timeout
go test -tags=integration -v -timeout 10m ./data_session/integration_test/...
```

### Verbose Container Logs

The test suite logs container startup information. Look for:
```
Starting Chroma container...
Chroma container started at http://localhost:XXXXX
Chroma container is healthy
Starting PGVector container...
PGVector container started at postgres://testuser:testpass@localhost:XXXXX/testdb?sslmode=disable
PGVector container is healthy
Starting Qdrant container...
Qdrant container started at http://localhost:XXXXX
Qdrant container is healthy
Starting Weaviate container...
Weaviate container started at http://localhost:XXXXX
Weaviate container is healthy
All test containers started successfully
```

### Cleanup Stuck Containers

If containers aren't cleaned up properly:

```bash
# List running containers
docker ps | grep -E 'chroma|pgvector|qdrant|weaviate'

# Stop and remove manually
docker stop $(docker ps -q --filter ancestor=chromadb/chroma:1.3.5)
docker stop $(docker ps -q --filter ancestor=pgvector/pgvector:0.8.0-pg16)
docker stop $(docker ps -q --filter ancestor=qdrant/qdrant:v1.16.1)
docker stop $(docker ps -q --filter ancestor=semitechnologies/weaviate:1.34.0)
```

## Adding New Datastore Tests

1. Add container to `pkg/testinfra/containers/` (see existing implementations)
2. Update `suite_test.go`:
   - Add `shared*Container` variable
   - Add setup in `setupContainers()`
   - Add cleanup in `cleanupContainers()`
   - Add `require*()` helper function
3. Create `*_test.go` file with tests
4. Update this README
