//go:build integration

package integration_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/pkg/testinfra/containers"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	weaviateTestDimensions = 384
)

// storeTestDocumentsWeaviate stores documents directly to Weaviate using the container helper.
func storeTestDocumentsWeaviate(t *testing.T, weaviate *containers.WeaviateContainer, className string, count int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create class
	err := weaviate.CreateClass(ctx, className, weaviateTestDimensions)
	require.NoError(t, err)

	contents := GenerateTestContents(count)
	vectors := GenerateTestVectors(count, weaviateTestDimensions)
	metadatas := GenerateTestMetadatas(count)

	objects := make([]containers.WeaviateObject, count)
	for i := 0; i < count; i++ {
		// Weaviate properties must be strings for text fields
		objects[i] = containers.WeaviateObject{
			ID:     uuid.New().String(),
			Vector: vectors[i],
			Properties: map[string]interface{}{
				"content":  contents[i],
				"source":   fmt.Sprintf("%v", metadatas[i]["source"]),
				"doc_id":   fmt.Sprintf("%v", metadatas[i]["doc_id"]),
				"category": fmt.Sprintf("%v", metadatas[i]["category"]),
			},
		}
	}

	err = weaviate.InsertObjects(ctx, className, objects)
	require.NoError(t, err)
}

// TestWeaviateContainerHealth verifies the Weaviate container is healthy and accessible.
func TestWeaviateContainerHealth(t *testing.T) {
	weaviate := requireWeaviate(t)
	ctx := context.Background()

	err := weaviate.Ping(ctx)
	require.NoError(t, err, "Weaviate container should be healthy")
}

// TestWeaviateCreateClass tests creating a class in Weaviate.
func TestWeaviateCreateClass(t *testing.T) {
	weaviate := requireWeaviate(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	className := "TestCreate" + uuid.New().String()[:8]

	// Create class
	err := weaviate.CreateClass(ctx, className, weaviateTestDimensions)
	require.NoError(t, err)

	// Verify it exists
	exists, err := weaviate.ClassExists(ctx, className)
	require.NoError(t, err)
	assert.True(t, exists, "Class should exist after creation")

	// Cleanup
	_ = weaviate.DeleteClass(ctx, className)
}

// TestWeaviateStoreDocumentsDirectly tests storing documents using Weaviate's REST API.
func TestWeaviateStoreDocumentsDirectly(t *testing.T) {
	weaviate := requireWeaviate(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	className := "TestStore" + uuid.New().String()[:8]

	// Store documents directly
	storeTestDocumentsWeaviate(t, weaviate, className, 3)

	// Verify documents were stored
	count, err := weaviate.ClassObjectCount(ctx, className)
	require.NoError(t, err)
	assert.Equal(t, 3, count, "Class should have 3 objects")

	// Cleanup
	_ = weaviate.DeleteClass(ctx, className)
}

// TestWeaviateSearchByVector tests similarity search using Weaviate's GraphQL API.
func TestWeaviateSearchByVector(t *testing.T) {
	weaviate := requireWeaviate(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	className := "TestSearch" + uuid.New().String()[:8]

	// Store documents
	storeTestDocumentsWeaviate(t, weaviate, className, 5)

	// Search using a query vector
	queryVector := GenerateTestVectors(1, weaviateTestDimensions)[0]
	results, err := weaviate.SearchObjects(ctx, className, queryVector, 3)
	require.NoError(t, err)

	assert.Len(t, results, 3, "Should return 3 results")
	assert.NotEmpty(t, results[0].ID, "First result should have an ID")
	assert.NotNil(t, results[0].Properties, "First result should have properties")

	// Cleanup
	_ = weaviate.DeleteClass(ctx, className)
}

// TestWeaviateSearchEmptyClass tests searching an empty class.
func TestWeaviateSearchEmptyClass(t *testing.T) {
	weaviate := requireWeaviate(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	className := "TestEmpty" + uuid.New().String()[:8]

	// Create empty class
	err := weaviate.CreateClass(ctx, className, weaviateTestDimensions)
	require.NoError(t, err)

	// Search empty class
	queryVector := GenerateTestVectors(1, weaviateTestDimensions)[0]
	results, err := weaviate.SearchObjects(ctx, className, queryVector, 5)
	require.NoError(t, err)
	assert.Empty(t, results, "Empty class should return no results")

	// Cleanup
	_ = weaviate.DeleteClass(ctx, className)
}

// TestWeaviateDeleteByFilter tests deleting objects by filter.
func TestWeaviateDeleteByFilter(t *testing.T) {
	weaviate := requireWeaviate(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	className := "TestDelete" + uuid.New().String()[:8]

	// Store documents
	storeTestDocumentsWeaviate(t, weaviate, className, 5)

	// Verify initial count
	beforeCount, err := weaviate.ClassObjectCount(ctx, className)
	require.NoError(t, err)
	assert.Equal(t, 5, beforeCount, "Should start with 5 objects")

	// Delete objects with category = "general"
	err = weaviate.DeleteObjectsByFilter(ctx, className, "category", "general")
	require.NoError(t, err)

	// Verify objects were deleted
	afterCount, err := weaviate.ClassObjectCount(ctx, className)
	require.NoError(t, err)
	assert.Less(t, afterCount, beforeCount, "Object count should decrease after delete")

	// Cleanup
	_ = weaviate.DeleteClass(ctx, className)
}

// TestWeaviateListClasses tests listing all classes.
func TestWeaviateListClasses(t *testing.T) {
	weaviate := requireWeaviate(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create a few classes
	classes := []string{
		"TestList1" + uuid.New().String()[:8],
		"TestList2" + uuid.New().String()[:8],
	}

	for _, name := range classes {
		err := weaviate.CreateClass(ctx, name, weaviateTestDimensions)
		require.NoError(t, err)
	}

	// List classes
	allClasses, err := weaviate.ListClasses(ctx)
	require.NoError(t, err)

	// Should include our test classes
	for _, expected := range classes {
		assert.Contains(t, allClasses, expected, "Should list created class")
	}

	// Cleanup
	for _, name := range classes {
		_ = weaviate.DeleteClass(ctx, name)
	}
}

// TestWeaviateDeleteClass tests deleting a class.
func TestWeaviateDeleteClass(t *testing.T) {
	weaviate := requireWeaviate(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	className := "TestDeleteClass" + uuid.New().String()[:8]

	// Create class
	err := weaviate.CreateClass(ctx, className, weaviateTestDimensions)
	require.NoError(t, err)

	// Verify it exists
	exists, err := weaviate.ClassExists(ctx, className)
	require.NoError(t, err)
	assert.True(t, exists, "Class should exist before deletion")

	// Delete class
	err = weaviate.DeleteClass(ctx, className)
	require.NoError(t, err)

	// Verify class no longer exists
	exists, err = weaviate.ClassExists(ctx, className)
	require.NoError(t, err)
	assert.False(t, exists, "Class should not exist after deletion")
}

// TestWeaviateMultipleDocumentsStorage tests storing and retrieving multiple documents.
func TestWeaviateMultipleDocumentsStorage(t *testing.T) {
	weaviate := requireWeaviate(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	className := "TestMulti" + uuid.New().String()[:8]

	// Create class
	err := weaviate.CreateClass(ctx, className, weaviateTestDimensions)
	require.NoError(t, err)

	// Store multiple batches
	for batch := 0; batch < 3; batch++ {
		contents := GenerateTestContents(5)
		vectors := GenerateTestVectors(5, weaviateTestDimensions)
		metadatas := GenerateTestMetadatas(5)

		objects := make([]containers.WeaviateObject, 5)
		for i := 0; i < 5; i++ {
			// Weaviate properties must be strings for text fields
			objects[i] = containers.WeaviateObject{
				ID:     uuid.New().String(),
				Vector: vectors[i],
				Properties: map[string]interface{}{
					"content":  contents[i],
					"source":   fmt.Sprintf("%v", metadatas[i]["source"]),
					"doc_id":   fmt.Sprintf("%v", metadatas[i]["doc_id"]),
					"category": fmt.Sprintf("%v", metadatas[i]["category"]),
				},
			}
		}

		err := weaviate.InsertObjects(ctx, className, objects)
		require.NoError(t, err, "Batch %d should store successfully", batch)
	}

	// Verify total count
	count, err := weaviate.ClassObjectCount(ctx, className)
	require.NoError(t, err)
	assert.Equal(t, 15, count, "Should have 15 objects across 3 batches")

	// Cleanup
	_ = weaviate.DeleteClass(ctx, className)
}

// TestWeaviateSearchWithDistanceOrdering tests search results ordering by distance.
func TestWeaviateSearchWithDistanceOrdering(t *testing.T) {
	weaviate := requireWeaviate(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	className := "TestDistance" + uuid.New().String()[:8]

	// Store documents
	storeTestDocumentsWeaviate(t, weaviate, className, 10)

	// Search using a query vector
	queryVector := GenerateTestVectors(1, weaviateTestDimensions)[0]
	results, err := weaviate.SearchObjects(ctx, className, queryVector, 5)
	require.NoError(t, err)

	assert.Len(t, results, 5, "Should return 5 results")

	// Verify results are ordered by distance (ascending - smaller distance = more similar)
	for i := 1; i < len(results); i++ {
		assert.LessOrEqual(t, results[i-1].Distance, results[i].Distance,
			"Results should be ordered by distance ascending")
	}

	// Cleanup
	_ = weaviate.DeleteClass(ctx, className)
}

// TestWeaviatePropertiesRetrieval tests that properties are correctly returned.
func TestWeaviatePropertiesRetrieval(t *testing.T) {
	weaviate := requireWeaviate(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	className := "TestProps" + uuid.New().String()[:8]

	// Create class
	err := weaviate.CreateClass(ctx, className, weaviateTestDimensions)
	require.NoError(t, err)

	vector := GenerateTestVectors(1, weaviateTestDimensions)[0]
	testProperties := map[string]interface{}{
		"content":  "Test document content",
		"source":   "test",
		"doc_id":   "doc-123",
		"category": "integration",
	}

	objects := []containers.WeaviateObject{
		{
			ID:         uuid.New().String(),
			Vector:     vector,
			Properties: testProperties,
		},
	}
	err = weaviate.InsertObjects(ctx, className, objects)
	require.NoError(t, err)

	// Search and verify properties
	results, err := weaviate.SearchObjects(ctx, className, vector, 1)
	require.NoError(t, err)
	require.Len(t, results, 1)

	assert.Equal(t, "Test document content", results[0].Properties["content"])
	assert.Equal(t, "test", results[0].Properties["source"])
	assert.Equal(t, "doc-123", results[0].Properties["doc_id"])
	assert.Equal(t, "integration", results[0].Properties["category"])

	// Cleanup
	_ = weaviate.DeleteClass(ctx, className)
}
