//go:build integration

package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/pkg/testinfra/containers"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	qdrantTestDimensions = 384
)

// storeTestDocumentsQdrant stores documents directly to Qdrant using the container helper.
func storeTestDocumentsQdrant(t *testing.T, qdrant *containers.QdrantContainer, collectionName string, count int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create collection
	err := qdrant.CreateCollection(ctx, collectionName, qdrantTestDimensions)
	require.NoError(t, err)

	contents := GenerateTestContents(count)
	vectors := GenerateTestVectors(count, qdrantTestDimensions)
	metadatas := GenerateTestMetadatas(count)

	points := make([]containers.QdrantPoint, count)
	for i := 0; i < count; i++ {
		points[i] = containers.QdrantPoint{
			ID:     uuid.New().String(),
			Vector: vectors[i],
			Payload: map[string]interface{}{
				"content":  contents[i],
				"source":   metadatas[i]["source"],
				"doc_id":   metadatas[i]["doc_id"],
				"category": metadatas[i]["category"],
			},
		}
	}

	err = qdrant.UpsertPoints(ctx, collectionName, points)
	require.NoError(t, err)
}

// TestQdrantContainerHealth verifies the Qdrant container is healthy and accessible.
func TestQdrantContainerHealth(t *testing.T) {
	qdrant := requireQdrant(t)
	ctx := context.Background()

	err := qdrant.Ping(ctx)
	require.NoError(t, err, "Qdrant container should be healthy")
}

// TestQdrantCreateCollection tests creating a collection in Qdrant.
func TestQdrantCreateCollection(t *testing.T) {
	qdrant := requireQdrant(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	collectionName := "test_create_" + uuid.New().String()[:8]

	// Create collection
	err := qdrant.CreateCollection(ctx, collectionName, qdrantTestDimensions)
	require.NoError(t, err)

	// Verify it exists
	exists, err := qdrant.CollectionExists(ctx, collectionName)
	require.NoError(t, err)
	assert.True(t, exists, "Collection should exist after creation")

	// Cleanup
	_ = qdrant.DeleteCollection(ctx, collectionName)
}

// TestQdrantStoreDocumentsDirectly tests storing documents using Qdrant's REST API.
func TestQdrantStoreDocumentsDirectly(t *testing.T) {
	qdrant := requireQdrant(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	collectionName := "test_store_" + uuid.New().String()[:8]

	// Store documents directly
	storeTestDocumentsQdrant(t, qdrant, collectionName, 3)

	// Verify documents were stored
	count, err := qdrant.CollectionPointCount(ctx, collectionName)
	require.NoError(t, err)
	assert.Equal(t, 3, count, "Collection should have 3 points")

	// Cleanup
	_ = qdrant.DeleteCollection(ctx, collectionName)
}

// TestQdrantSearchByVector tests similarity search using Qdrant's search API.
func TestQdrantSearchByVector(t *testing.T) {
	qdrant := requireQdrant(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	collectionName := "test_search_" + uuid.New().String()[:8]

	// Store documents
	storeTestDocumentsQdrant(t, qdrant, collectionName, 5)

	// Search using a query vector
	queryVector := GenerateTestVectors(1, qdrantTestDimensions)[0]
	results, err := qdrant.SearchPoints(ctx, collectionName, queryVector, 3)
	require.NoError(t, err)

	assert.Len(t, results, 3, "Should return 3 results")
	assert.NotEmpty(t, results[0].ID, "First result should have an ID")
	assert.NotNil(t, results[0].Payload, "First result should have payload")
	assert.Greater(t, results[0].Score, float32(0), "First result should have positive score")

	// Cleanup
	_ = qdrant.DeleteCollection(ctx, collectionName)
}

// TestQdrantSearchEmptyCollection tests searching an empty collection.
func TestQdrantSearchEmptyCollection(t *testing.T) {
	qdrant := requireQdrant(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	collectionName := "test_empty_" + uuid.New().String()[:8]

	// Create empty collection
	err := qdrant.CreateCollection(ctx, collectionName, qdrantTestDimensions)
	require.NoError(t, err)

	// Search empty collection
	queryVector := GenerateTestVectors(1, qdrantTestDimensions)[0]
	results, err := qdrant.SearchPoints(ctx, collectionName, queryVector, 5)
	require.NoError(t, err)
	assert.Empty(t, results, "Empty collection should return no results")

	// Cleanup
	_ = qdrant.DeleteCollection(ctx, collectionName)
}

// TestQdrantDeleteByFilter tests deleting points by filter.
func TestQdrantDeleteByFilter(t *testing.T) {
	qdrant := requireQdrant(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	collectionName := "test_delete_" + uuid.New().String()[:8]

	// Store documents
	storeTestDocumentsQdrant(t, qdrant, collectionName, 5)

	// Verify initial count
	beforeCount, err := qdrant.CollectionPointCount(ctx, collectionName)
	require.NoError(t, err)
	assert.Equal(t, 5, beforeCount, "Should start with 5 points")

	// Delete points with category = "general" using filter
	filter := map[string]interface{}{
		"must": []map[string]interface{}{
			{
				"key":   "category",
				"match": map[string]interface{}{"value": "general"},
			},
		},
	}
	err = qdrant.DeletePoints(ctx, collectionName, filter)
	require.NoError(t, err)

	// Verify points were deleted
	afterCount, err := qdrant.CollectionPointCount(ctx, collectionName)
	require.NoError(t, err)
	assert.Less(t, afterCount, beforeCount, "Point count should decrease after delete")

	// Cleanup
	_ = qdrant.DeleteCollection(ctx, collectionName)
}

// TestQdrantListCollections tests listing all collections.
func TestQdrantListCollections(t *testing.T) {
	qdrant := requireQdrant(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create a few collections
	collections := []string{
		"test_list_1_" + uuid.New().String()[:8],
		"test_list_2_" + uuid.New().String()[:8],
	}

	for _, name := range collections {
		err := qdrant.CreateCollection(ctx, name, qdrantTestDimensions)
		require.NoError(t, err)
	}

	// List collections
	allCollections, err := qdrant.ListCollections(ctx)
	require.NoError(t, err)

	// Should include our test collections
	for _, expected := range collections {
		assert.Contains(t, allCollections, expected, "Should list created collection")
	}

	// Cleanup
	for _, name := range collections {
		_ = qdrant.DeleteCollection(ctx, name)
	}
}

// TestQdrantDeleteCollection tests deleting a collection.
func TestQdrantDeleteCollection(t *testing.T) {
	qdrant := requireQdrant(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	collectionName := "test_delete_coll_" + uuid.New().String()[:8]

	// Create collection
	err := qdrant.CreateCollection(ctx, collectionName, qdrantTestDimensions)
	require.NoError(t, err)

	// Verify it exists
	exists, err := qdrant.CollectionExists(ctx, collectionName)
	require.NoError(t, err)
	assert.True(t, exists, "Collection should exist before deletion")

	// Delete collection
	err = qdrant.DeleteCollection(ctx, collectionName)
	require.NoError(t, err)

	// Verify collection no longer exists
	exists, err = qdrant.CollectionExists(ctx, collectionName)
	require.NoError(t, err)
	assert.False(t, exists, "Collection should not exist after deletion")
}

// TestQdrantMultipleDocumentsStorage tests storing and retrieving multiple documents.
func TestQdrantMultipleDocumentsStorage(t *testing.T) {
	qdrant := requireQdrant(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	collectionName := "test_multi_" + uuid.New().String()[:8]

	// Create collection
	err := qdrant.CreateCollection(ctx, collectionName, qdrantTestDimensions)
	require.NoError(t, err)

	// Store multiple batches
	for batch := 0; batch < 3; batch++ {
		contents := GenerateTestContents(5)
		vectors := GenerateTestVectors(5, qdrantTestDimensions)
		metadatas := GenerateTestMetadatas(5)

		points := make([]containers.QdrantPoint, 5)
		for i := 0; i < 5; i++ {
			points[i] = containers.QdrantPoint{
				ID:     uuid.New().String(),
				Vector: vectors[i],
				Payload: map[string]interface{}{
					"content":  contents[i],
					"batch":    batch,
					"source":   metadatas[i]["source"],
					"doc_id":   metadatas[i]["doc_id"],
					"category": metadatas[i]["category"],
				},
			}
		}

		err := qdrant.UpsertPoints(ctx, collectionName, points)
		require.NoError(t, err, "Batch %d should store successfully", batch)
	}

	// Verify total count
	count, err := qdrant.CollectionPointCount(ctx, collectionName)
	require.NoError(t, err)
	assert.Equal(t, 15, count, "Should have 15 points across 3 batches")

	// Cleanup
	_ = qdrant.DeleteCollection(ctx, collectionName)
}

// TestQdrantUpsertBehavior tests upsert behavior with same ID.
func TestQdrantUpsertBehavior(t *testing.T) {
	qdrant := requireQdrant(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	collectionName := "test_upsert_" + uuid.New().String()[:8]

	// Create collection
	err := qdrant.CreateCollection(ctx, collectionName, qdrantTestDimensions)
	require.NoError(t, err)

	pointID := uuid.New().String()
	vector := GenerateTestVectors(1, qdrantTestDimensions)[0]

	// Insert first point
	points1 := []containers.QdrantPoint{
		{
			ID:     pointID,
			Vector: vector,
			Payload: map[string]interface{}{
				"content": "Original content",
				"version": 1,
			},
		},
	}
	err = qdrant.UpsertPoints(ctx, collectionName, points1)
	require.NoError(t, err)

	// Upsert with same ID, different payload
	points2 := []containers.QdrantPoint{
		{
			ID:     pointID,
			Vector: vector,
			Payload: map[string]interface{}{
				"content": "Updated content",
				"version": 2,
			},
		},
	}
	err = qdrant.UpsertPoints(ctx, collectionName, points2)
	require.NoError(t, err)

	// Verify only one point exists
	count, err := qdrant.CollectionPointCount(ctx, collectionName)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "Should have only 1 point after upsert")

	// Search and verify the payload was updated
	results, err := qdrant.SearchPoints(ctx, collectionName, vector, 1)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "Updated content", results[0].Payload["content"], "Content should be updated")
	assert.Equal(t, float64(2), results[0].Payload["version"], "Version should be updated")

	// Cleanup
	_ = qdrant.DeleteCollection(ctx, collectionName)
}

// TestQdrantSearchWithScoreThreshold tests search results ordering by score.
func TestQdrantSearchWithScoreThreshold(t *testing.T) {
	qdrant := requireQdrant(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	collectionName := "test_score_" + uuid.New().String()[:8]

	// Store documents
	storeTestDocumentsQdrant(t, qdrant, collectionName, 10)

	// Search using a query vector
	queryVector := GenerateTestVectors(1, qdrantTestDimensions)[0]
	results, err := qdrant.SearchPoints(ctx, collectionName, queryVector, 5)
	require.NoError(t, err)

	assert.Len(t, results, 5, "Should return 5 results")

	// Verify results are ordered by score (descending)
	for i := 1; i < len(results); i++ {
		assert.GreaterOrEqual(t, results[i-1].Score, results[i].Score,
			"Results should be ordered by score descending")
	}

	// Cleanup
	_ = qdrant.DeleteCollection(ctx, collectionName)
}

// TestQdrantPayloadRetrieval tests that payloads are correctly returned.
func TestQdrantPayloadRetrieval(t *testing.T) {
	qdrant := requireQdrant(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	collectionName := "test_payload_" + uuid.New().String()[:8]

	// Create collection
	err := qdrant.CreateCollection(ctx, collectionName, qdrantTestDimensions)
	require.NoError(t, err)

	vector := GenerateTestVectors(1, qdrantTestDimensions)[0]
	testPayload := map[string]interface{}{
		"content":      "Test document content",
		"source":       "test",
		"numeric_id":   42,
		"is_important": true,
		"tags":         []string{"test", "integration"},
	}

	points := []containers.QdrantPoint{
		{
			ID:      uuid.New().String(),
			Vector:  vector,
			Payload: testPayload,
		},
	}
	err = qdrant.UpsertPoints(ctx, collectionName, points)
	require.NoError(t, err)

	// Search and verify payload
	results, err := qdrant.SearchPoints(ctx, collectionName, vector, 1)
	require.NoError(t, err)
	require.Len(t, results, 1)

	assert.Equal(t, "Test document content", results[0].Payload["content"])
	assert.Equal(t, "test", results[0].Payload["source"])
	assert.Equal(t, float64(42), results[0].Payload["numeric_id"])
	assert.Equal(t, true, results[0].Payload["is_important"])

	// Cleanup
	_ = qdrant.DeleteCollection(ctx, collectionName)
}
