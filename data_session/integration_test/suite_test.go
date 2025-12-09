//go:build integration

package integration_test

import (
	"context"
	"log"
	"os"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/pkg/testinfra/containers"
)

// Shared container references for suite-level reuse.
// These are initialized once in TestMain and shared across all tests.
var (
	sharedChromaContainer   *containers.ChromaContainer
	sharedPGVectorContainer *containers.PGVectorContainer
	sharedQdrantContainer   *containers.QdrantContainer
	sharedWeaviateContainer *containers.WeaviateContainer
)

// TestMain sets up shared containers once for the entire test suite.
// This significantly speeds up integration tests by avoiding container
// startup/shutdown overhead for each test.
func TestMain(m *testing.M) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	var exitCode int
	defer func() {
		cleanupContainers(context.Background())
		os.Exit(exitCode)
	}()

	if err := setupContainers(ctx); err != nil {
		log.Printf("Failed to set up test containers: %v", err)
		exitCode = 1
		return
	}

	log.Println("All test containers started successfully")
	exitCode = m.Run()
}

func setupContainers(ctx context.Context) error {
	var err error

	// Start Chroma container (required for vector store tests)
	log.Println("Starting Chroma container...")
	sharedChromaContainer, err = containers.NewChromaContainer(ctx, nil)
	if err != nil {
		return err
	}
	log.Printf("Chroma container started at %s", sharedChromaContainer.Addr())

	// Verify Chroma is responsive
	if err := sharedChromaContainer.Ping(ctx); err != nil {
		return err
	}
	log.Println("Chroma container is healthy")

	// Start PGVector container (PostgreSQL with pgvector extension)
	log.Println("Starting PGVector container...")
	sharedPGVectorContainer, err = containers.NewPGVectorContainer(ctx, nil)
	if err != nil {
		return err
	}
	log.Printf("PGVector container started at %s", sharedPGVectorContainer.ConnectionString())

	// Verify PGVector is responsive
	if err := sharedPGVectorContainer.Ping(ctx); err != nil {
		return err
	}
	log.Println("PGVector container is healthy")

	// Start Qdrant container (high-performance vector database)
	log.Println("Starting Qdrant container...")
	sharedQdrantContainer, err = containers.NewQdrantContainer(ctx, nil)
	if err != nil {
		return err
	}
	log.Printf("Qdrant container started at %s", sharedQdrantContainer.Addr())

	// Verify Qdrant is responsive
	if err := sharedQdrantContainer.Ping(ctx); err != nil {
		return err
	}
	log.Println("Qdrant container is healthy")

	// Start Weaviate container (AI-native vector database)
	log.Println("Starting Weaviate container...")
	sharedWeaviateContainer, err = containers.NewWeaviateContainer(ctx, nil)
	if err != nil {
		return err
	}
	log.Printf("Weaviate container started at %s", sharedWeaviateContainer.Addr())

	// Verify Weaviate is responsive
	if err := sharedWeaviateContainer.Ping(ctx); err != nil {
		return err
	}
	log.Println("Weaviate container is healthy")

	return nil
}

func cleanupContainers(ctx context.Context) {
	if sharedChromaContainer != nil {
		log.Println("Stopping Chroma container...")
		if err := sharedChromaContainer.Close(ctx); err != nil {
			log.Printf("Warning: Failed to stop Chroma container: %v", err)
		}
	}
	if sharedPGVectorContainer != nil {
		log.Println("Stopping PGVector container...")
		if err := sharedPGVectorContainer.Close(ctx); err != nil {
			log.Printf("Warning: Failed to stop PGVector container: %v", err)
		}
	}
	if sharedQdrantContainer != nil {
		log.Println("Stopping Qdrant container...")
		if err := sharedQdrantContainer.Close(ctx); err != nil {
			log.Printf("Warning: Failed to stop Qdrant container: %v", err)
		}
	}
	if sharedWeaviateContainer != nil {
		log.Println("Stopping Weaviate container...")
		if err := sharedWeaviateContainer.Close(ctx); err != nil {
			log.Printf("Warning: Failed to stop Weaviate container: %v", err)
		}
	}
}

// requireChroma returns the shared Chroma container or skips the test if not available.
func requireChroma(t *testing.T) *containers.ChromaContainer {
	t.Helper()
	if sharedChromaContainer == nil {
		t.Skip("Chroma container not available")
	}
	return sharedChromaContainer
}

// requirePGVector returns the shared PGVector container or skips the test if not available.
func requirePGVector(t *testing.T) *containers.PGVectorContainer {
	t.Helper()
	if sharedPGVectorContainer == nil {
		t.Skip("PGVector container not available")
	}
	return sharedPGVectorContainer
}

// requireQdrant returns the shared Qdrant container or skips the test if not available.
func requireQdrant(t *testing.T) *containers.QdrantContainer {
	t.Helper()
	if sharedQdrantContainer == nil {
		t.Skip("Qdrant container not available")
	}
	return sharedQdrantContainer
}

// requireWeaviate returns the shared Weaviate container or skips the test if not available.
func requireWeaviate(t *testing.T) *containers.WeaviateContainer {
	t.Helper()
	if sharedWeaviateContainer == nil {
		t.Skip("Weaviate container not available")
	}
	return sharedWeaviateContainer
}
