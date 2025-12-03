//go:build integration

package integration_test

import (
	"context"
	"testing"
	"time"

	chromago "github.com/amikos-tech/chroma-go/pkg/api/v2"
	chromaEmbeddings "github.com/amikos-tech/chroma-go/pkg/embeddings"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// noopEmbedder returns a consistent hash embedding function for GetCollection calls.
// Chroma v2 API requires an embedding function even when we're providing our own embeddings.
func noopEmbedder() chromaEmbeddings.EmbeddingFunction {
	return chromaEmbeddings.NewConsistentHashEmbeddingFunction()
}

const (
	testDimensions = 384
)

// storeTestDocumentsDirectly stores documents directly to Chroma using the native client.
// This bypasses the DataSession which requires an embedder vendor.
func storeTestDocumentsDirectly(t *testing.T, chromaAddr, collectionName string, count int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := chromago.NewHTTPClient(chromago.WithBaseURL(chromaAddr))
	require.NoError(t, err)

	// Create collection with L2 distance metric
	collection, err := client.CreateCollection(ctx, collectionName,
		chromago.WithHNSWSpaceCreate(chromaEmbeddings.L2),
		chromago.WithIfNotExistsCreate(),
	)
	require.NoError(t, err)

	contents := GenerateTestContents(count)
	vectors := GenerateTestVectors(count, testDimensions)
	metadatas := GenerateTestMetadatas(count)

	for i := 0; i < count; i++ {
		docID := chromago.DocumentID(uuid.New().String())
		emb := chromaEmbeddings.NewEmbeddingFromFloat32(vectors[i])

		chromaMetadata, err := chromago.NewDocumentMetadataFromMap(metadatas[i])
		require.NoError(t, err)

		err = collection.Add(ctx,
			chromago.WithIDs(docID),
			chromago.WithTexts(contents[i]),
			chromago.WithEmbeddings(emb),
			chromago.WithMetadatas(chromaMetadata),
		)
		require.NoError(t, err)
	}
}

// TestChromaContainerHealth verifies the Chroma container is healthy and accessible.
func TestChromaContainerHealth(t *testing.T) {
	chroma := requireChroma(t)
	ctx := context.Background()

	err := chroma.Ping(ctx)
	require.NoError(t, err, "Chroma container should be healthy")
}

// TestChromaStoreDocumentsDirectly tests storing documents using Chroma's native client.
// This validates that our test infrastructure correctly interacts with Chroma.
func TestChromaStoreDocumentsDirectly(t *testing.T) {
	chroma := requireChroma(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	collectionName := "test_store_" + uuid.New().String()[:8]

	// Store documents directly using Chroma client
	storeTestDocumentsDirectly(t, chroma.Addr(), collectionName, 3)

	// Verify documents were stored by querying Chroma directly
	client, err := chromago.NewHTTPClient(chromago.WithBaseURL(chroma.Addr()))
	require.NoError(t, err)

	collection, err := client.GetCollection(ctx, collectionName, chromago.WithEmbeddingFunctionGet(noopEmbedder()))
	require.NoError(t, err, "Collection should exist")

	count, err := collection.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 3, count, "Collection should have 3 documents")

	// Cleanup
	err = client.DeleteCollection(ctx, collectionName)
	require.NoError(t, err)
}

// TestChromaSearchByVector tests similarity search using Chroma's native query API.
func TestChromaSearchByVector(t *testing.T) {
	chroma := requireChroma(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	collectionName := "test_search_" + uuid.New().String()[:8]

	// Store documents directly using Chroma client
	storeTestDocumentsDirectly(t, chroma.Addr(), collectionName, 5)

	// Query using Chroma's native API
	client, err := chromago.NewHTTPClient(chromago.WithBaseURL(chroma.Addr()))
	require.NoError(t, err)

	collection, err := client.GetCollection(ctx, collectionName, chromago.WithEmbeddingFunctionGet(noopEmbedder()))
	require.NoError(t, err)

	// Search using a query vector (use first document's vector as query)
	queryVector := GenerateTestVectors(1, testDimensions)[0]
	queryEmb := chromaEmbeddings.NewEmbeddingFromFloat32(queryVector)

	result, err := collection.Query(ctx,
		chromago.WithQueryEmbeddings(queryEmb),
		chromago.WithNResults(3),
	)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify results - check that we got documents back
	documentsGroups := result.GetDocumentsGroups()
	assert.NotEmpty(t, documentsGroups, "Should return document groups")
	if len(documentsGroups) > 0 {
		assert.NotEmpty(t, documentsGroups[0], "First document group should have documents")
	}

	// Cleanup
	_ = client.DeleteCollection(ctx, collectionName)
}

// TestChromaSearchEmptyCollection tests searching an empty collection.
func TestChromaSearchEmptyCollection(t *testing.T) {
	chroma := requireChroma(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	collectionName := "test_empty_" + uuid.New().String()[:8]

	// Create empty collection using Chroma client
	client, err := chromago.NewHTTPClient(chromago.WithBaseURL(chroma.Addr()))
	require.NoError(t, err)

	collection, err := client.CreateCollection(ctx, collectionName)
	require.NoError(t, err)

	// Search empty collection using Chroma's native API
	queryVector := GenerateTestVectors(1, testDimensions)[0]
	queryEmb := chromaEmbeddings.NewEmbeddingFromFloat32(queryVector)

	result, err := collection.Query(ctx,
		chromago.WithQueryEmbeddings(queryEmb),
		chromago.WithNResults(5),
	)

	// Empty collection should return empty results (not error)
	require.NoError(t, err)
	documentsGroups := result.GetDocumentsGroups()
	// For empty collection, we expect either no groups or empty groups
	if len(documentsGroups) > 0 {
		assert.Empty(t, documentsGroups[0], "Empty collection should return no documents")
	}

	// Cleanup
	_ = client.DeleteCollection(ctx, collectionName)
}

// TestChromaDeleteByMetadata tests deleting documents by metadata filter using Chroma's native API.
func TestChromaDeleteByMetadata(t *testing.T) {
	chroma := requireChroma(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	collectionName := "test_delete_" + uuid.New().String()[:8]

	// Store documents directly
	storeTestDocumentsDirectly(t, chroma.Addr(), collectionName, 5)

	// Get collection
	client, err := chromago.NewHTTPClient(chromago.WithBaseURL(chroma.Addr()))
	require.NoError(t, err)

	collection, err := client.GetCollection(ctx, collectionName, chromago.WithEmbeddingFunctionGet(noopEmbedder()))
	require.NoError(t, err)

	// Verify initial count
	beforeCount, err := collection.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 5, beforeCount, "Should start with 5 documents")

	// Get documents with matching metadata to find and delete
	// Build a where filter for category = "general"
	whereFilter := chromago.EqString("category", "general")

	result, err := collection.Get(ctx, chromago.WithWhereGet(whereFilter))
	require.NoError(t, err)

	// Count matching documents
	matchCount := result.Count()

	// Delete matching documents using where filter
	if matchCount > 0 {
		err = collection.Delete(ctx, chromago.WithWhereDelete(whereFilter))
		require.NoError(t, err)
	}

	// Verify documents were deleted
	afterCount, err := collection.Count(ctx)
	require.NoError(t, err)
	assert.Less(t, afterCount, beforeCount, "Document count should decrease after delete")

	// Cleanup
	_ = client.DeleteCollection(ctx, collectionName)
}

// TestChromaQueryByMetadataOnly tests querying by metadata using Chroma's native API.
func TestChromaQueryByMetadataOnly(t *testing.T) {
	chroma := requireChroma(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	collectionName := "test_metadata_query_" + uuid.New().String()[:8]

	// Store documents directly
	storeTestDocumentsDirectly(t, chroma.Addr(), collectionName, 10)

	// Get collection
	client, err := chromago.NewHTTPClient(chromago.WithBaseURL(chroma.Addr()))
	require.NoError(t, err)

	collection, err := client.GetCollection(ctx, collectionName, chromago.WithEmbeddingFunctionGet(noopEmbedder()))
	require.NoError(t, err)

	// Query by category using where filter
	whereFilter := chromago.EqString("category", "ml")

	result, err := collection.Get(ctx, chromago.WithWhereGet(whereFilter))
	require.NoError(t, err)

	// Should find documents with "ml" category
	assert.GreaterOrEqual(t, result.Count(), 1, "Should find at least 1 document with ml category")

	// Cleanup
	_ = client.DeleteCollection(ctx, collectionName)
}

// TestChromaListNamespaces tests listing all collections using Chroma's native API.
func TestChromaListNamespaces(t *testing.T) {
	chroma := requireChroma(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create a few collections
	client, err := chromago.NewHTTPClient(chromago.WithBaseURL(chroma.Addr()))
	require.NoError(t, err)

	collections := []string{
		"test_list_1_" + uuid.New().String()[:8],
		"test_list_2_" + uuid.New().String()[:8],
	}

	for _, name := range collections {
		_, err := client.CreateCollection(ctx, name)
		require.NoError(t, err)
	}

	// List collections using Chroma client
	allCollections, err := client.ListCollections(ctx)
	require.NoError(t, err)

	// Should include our test collections
	collectionNames := make([]string, len(allCollections))
	for i, coll := range allCollections {
		collectionNames[i] = coll.Name()
	}

	for _, expected := range collections {
		assert.Contains(t, collectionNames, expected, "Should list created collection")
	}

	// Cleanup
	for _, name := range collections {
		_ = client.DeleteCollection(ctx, name)
	}
}

// TestChromaDeleteNamespace tests deleting an entire collection using Chroma's native API.
func TestChromaDeleteNamespace(t *testing.T) {
	chroma := requireChroma(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	collectionName := "test_delete_ns_" + uuid.New().String()[:8]

	// Create collection
	client, err := chromago.NewHTTPClient(chromago.WithBaseURL(chroma.Addr()))
	require.NoError(t, err)

	_, err = client.CreateCollection(ctx, collectionName)
	require.NoError(t, err)

	// Verify it exists
	_, err = client.GetCollection(ctx, collectionName, chromago.WithEmbeddingFunctionGet(noopEmbedder()))
	require.NoError(t, err, "Collection should exist before deletion")

	// Delete collection
	err = client.DeleteCollection(ctx, collectionName)
	require.NoError(t, err)

	// Verify collection no longer exists
	_, err = client.GetCollection(ctx, collectionName, chromago.WithEmbeddingFunctionGet(noopEmbedder()))
	assert.Error(t, err, "Collection should not exist after deletion")
}

// TestChromaMultipleDocumentsStorage tests storing and retrieving multiple documents.
func TestChromaMultipleDocumentsStorage(t *testing.T) {
	chroma := requireChroma(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	collectionName := "test_multi_" + uuid.New().String()[:8]

	// Create client and collection
	client, err := chromago.NewHTTPClient(chromago.WithBaseURL(chroma.Addr()))
	require.NoError(t, err)

	collection, err := client.CreateCollection(ctx, collectionName,
		chromago.WithHNSWSpaceCreate(chromaEmbeddings.L2),
	)
	require.NoError(t, err)

	// Store multiple batches
	for batch := 0; batch < 3; batch++ {
		contents := GenerateTestContents(5)
		vectors := GenerateTestVectors(5, testDimensions)
		metadatas := GenerateTestMetadatas(5)

		for i := 0; i < 5; i++ {
			docID := chromago.DocumentID(uuid.New().String())
			emb := chromaEmbeddings.NewEmbeddingFromFloat32(vectors[i])

			// Add batch info to metadata
			metadatas[i]["batch"] = batch
			chromaMetadata, err := chromago.NewDocumentMetadataFromMap(metadatas[i])
			require.NoError(t, err)

			err = collection.Add(ctx,
				chromago.WithIDs(docID),
				chromago.WithTexts(contents[i]),
				chromago.WithEmbeddings(emb),
				chromago.WithMetadatas(chromaMetadata),
			)
			require.NoError(t, err, "Batch %d, doc %d should store successfully", batch, i)
		}
	}

	// Verify total count
	count, err := collection.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 15, count, "Should have 15 documents across 3 batches")

	// Cleanup
	_ = client.DeleteCollection(ctx, collectionName)
}

// TestChromaDirectClientOperations tests direct Chroma client operations for debugging.
func TestChromaDirectClientOperations(t *testing.T) {
	chroma := requireChroma(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	collectionName := "test_direct_" + uuid.New().String()[:8]

	// Create client
	client, err := chromago.NewHTTPClient(chromago.WithBaseURL(chroma.Addr()))
	require.NoError(t, err)

	// Create collection
	collection, err := client.CreateCollection(ctx, collectionName,
		chromago.WithHNSWSpaceCreate(chromaEmbeddings.L2),
	)
	require.NoError(t, err)
	require.NotNil(t, collection)

	// Add document with embedding
	testContent := "This is a test document for direct operations"
	testVector := GenerateTestVectors(1, testDimensions)[0]
	docID := chromago.DocumentID(uuid.New().String())
	emb := chromaEmbeddings.NewEmbeddingFromFloat32(testVector)

	metadata, err := chromago.NewDocumentMetadataFromMap(map[string]any{"test_key": "test_value"})
	require.NoError(t, err)

	err = collection.Add(ctx,
		chromago.WithIDs(docID),
		chromago.WithTexts(testContent),
		chromago.WithEmbeddings(emb),
		chromago.WithMetadatas(metadata),
	)
	require.NoError(t, err)

	// Query the collection
	queryEmb := chromaEmbeddings.NewEmbeddingFromFloat32(testVector)
	result, err := collection.Query(ctx, chromago.WithQueryEmbeddings(queryEmb), chromago.WithNResults(1))
	require.NoError(t, err)
	require.NotNil(t, result)

	// Cleanup
	err = client.DeleteCollection(ctx, collectionName)
	require.NoError(t, err)
}
