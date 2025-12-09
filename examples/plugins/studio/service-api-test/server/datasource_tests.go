package main

import (
	"context"
	"fmt"
	"time"

	"github.com/TykTechnologies/midsommar/v2/pkg/ai_studio_sdk"
	mgmtpb "github.com/TykTechnologies/midsommar/v2/proto/ai_studio_management"
)

// RunDatasourceTests executes datasource CRUD tests and RAG/embedding API tests
// Split into two sections:
// 1. CRUD tests (Create/Update/Delete) - always run
// 2. RAG/Embedding tests - only run if a pre-configured datasource exists
func RunDatasourceTests(ctx context.Context) ([]TestResult, []uint32) {
	var results []TestResult
	var createdIDs []uint32

	// === CRUD TESTS (Create/Update/Delete) ===

	// Test 1: Create a basic datasource (minimal config, no embedder)
	result, dsID, dsName := testCreateDatasource(ctx)
	results = append(results, result)
	if result.Success && dsID != 0 {
		createdIDs = append(createdIDs, dsID)
	}

	// Test 2: Get Datasource (verify creation)
	if dsID != 0 {
		result = testGetDatasource(ctx, dsID, dsName)
		results = append(results, result)
	}

	// Test 3: Update Datasource
	if dsID != 0 {
		result, dsName = testUpdateDatasource(ctx, dsID)
		results = append(results, result)
	}

	// Test 4: Get Datasource (verify update)
	if dsID != 0 {
		result = testGetDatasource(ctx, dsID, dsName)
		results = append(results, result)
	}

	// Test 5: SearchDatasources - Search for datasources by query
	result = testSearchDatasources(ctx)
	results = append(results, result)

	// Test 6: Delete Datasource (cleanup CRUD test)
	if dsID != 0 {
		result = testDeleteDatasource(ctx, dsID)
		results = append(results, result)
		if result.Success {
			createdIDs = createdIDs[:len(createdIDs)-1]
		}
	}

	// === RAG/EMBEDDING TESTS (Require pre-configured datasource) ===
	// These tests need a datasource with:
	// - EmbedVendor configured (e.g., "openai")
	// - EmbedModel configured (e.g., "text-embedding-3-small")
	// - EmbedAPIKey set
	// - DBSourceType configured (e.g., "chroma")
	// - Vector store accessible

	// Check if any datasource exists with embedder configured
	ragDatasourceID, hasEmbedder, searchDetails := findDatasourceWithEmbedder(ctx)

	if ragDatasourceID != 0 && hasEmbedder {
		// Test 7: GenerateEmbedding - Generate embeddings for text chunks
		var embeddings [][]float32
		result, embeddings = testGenerateEmbedding(ctx, ragDatasourceID)
		results = append(results, result)

		// Test 8: StoreDocuments - Store chunks with pre-computed embeddings
		if len(embeddings) > 0 {
			result = testStoreDocuments(ctx, ragDatasourceID, embeddings)
			results = append(results, result)
		}

		// Test 9: ProcessAndStoreDocuments - Convenience method (generate + store)
		result = testProcessAndStoreDocuments(ctx, ragDatasourceID)
		results = append(results, result)

		// Test 10: QueryDatasource - Semantic search with text query
		result = testQueryDatasource(ctx, ragDatasourceID)
		results = append(results, result)

		// Test 11: QueryDatasourceByVector - Search with pre-computed vector
		if len(embeddings) > 0 {
			result = testQueryDatasourceByVector(ctx, ragDatasourceID, embeddings[0])
			results = append(results, result)
		}

		// Test 12: ProcessDatasourceEmbeddings - Async batch processing
		result = testProcessDatasourceEmbeddings(ctx, ragDatasourceID)
		results = append(results, result)

		// === ADVANCED METADATA OPERATIONS TESTS ===

		// Test 13: DeleteDocumentsByMetadata - Dry run mode
		result = testDeleteDocumentsByMetadataDryRun(ctx, ragDatasourceID)
		results = append(results, result)

		// Test 14: QueryByMetadataOnly - Find specific documents
		result = testQueryByMetadataOnly(ctx, ragDatasourceID)
		results = append(results, result)

		// Test 15: ListNamespaces - List all collections
		result = testListNamespaces(ctx, ragDatasourceID)
		results = append(results, result)

		// Test 16: DeleteDocumentsByMetadata - Actual deletion
		result = testDeleteDocumentsByMetadata(ctx, ragDatasourceID)
		results = append(results, result)
	} else {
		// Skip RAG tests if no configured datasource found
		skipMessage := fmt.Sprintf("⚠️  SKIPPED - No datasource with embedder configured found.\n\n%s\n\n📋 Setup Instructions:\n1. Go to AI Studio UI → Datasources\n2. Create or edit a datasource\n3. Set EmbedVendor to your embedder provider (e.g., 'openai', 'ollama', 'vertex')\n4. Set EmbedModel to the actual model name (e.g., 'text-embedding-3-small' for OpenAI, 'nomic-embed-text' for Ollama)\n   ⚠️  Common mistake: Setting EmbedModel='openai' - this should be the actual model name!\n5. Set EmbedAPIKey if required by your embedder\n6. Set DBSourceType to your vector store (e.g., 'chroma', 'pinecone', 'pgvector')\n7. Ensure vector store is running and accessible\n8. Re-run tests to validate RAG APIs",
			searchDetails)

		results = append(results, TestResult{
			Operation: "RAG Tests (GenerateEmbedding/StoreDocuments/QueryDatasource/etc)",
			Success:   false,
			Message:   skipMessage,
			Duration:  0,
			Timestamp: time.Now(),
		})
	}

	return results, createdIDs
}

// findDatasourceWithEmbedder searches for a datasource that has embedder configured
// Returns datasource ID, found status, and search details message
func findDatasourceWithEmbedder(ctx context.Context) (uint32, bool, string) {
	// List all datasources with pagination
	resp, err := ai_studio_sdk.ListDatasources(ctx, 1, 100, nil, "")
	if err != nil {
		return 0, false, fmt.Sprintf("ListDatasources failed: %v", err)
	}

	details := fmt.Sprintf("Found %d datasources (total: %d):\n", len(resp.Datasources), resp.TotalCount)

	// Find first datasource with embedder configured
	for _, ds := range resp.Datasources {
		dsInfo := fmt.Sprintf("  - ID %d: EmbedVendor='%s', EmbedModel='%s', Active=%v",
			ds.Id, ds.EmbedVendor, ds.EmbedModel, ds.Active)

		if ds.EmbedVendor != "" && ds.EmbedModel != "" && ds.Active {
			details += dsInfo + " ✅ MATCH - Using for RAG tests\n"
			return ds.Id, true, details
		} else {
			reason := ""
			if ds.EmbedVendor == "" {
				reason = " (missing EmbedVendor)"
			} else if ds.EmbedModel == "" {
				reason = " (missing EmbedModel)"
			} else if !ds.Active {
				reason = " (inactive)"
			}
			details += dsInfo + reason + "\n"
		}
	}

	details += "\n❌ No datasource found with complete embedder configuration"
	return 0, false, details
}

func testCreateDatasource(ctx context.Context) (TestResult, uint32, string) {
	start := time.Now()
	name := generateTestName("Test Datasource")

	// Create a basic datasource (minimal config for CRUD testing)
	// NOTE: This doesn't include embedder config - that should be manually configured
	resp, err := ai_studio_sdk.CreateDatasource(
		ctx,
		name,
		"Test datasource for CRUD operations",
		"This datasource tests create/update/delete operations",
		"",       // URL (optional)
		"chroma", // Vector store type
		5,        // Privacy score
		1,        // User ID
		true,     // Active
	)

	duration := time.Since(start)

	if err != nil {
		return TestResult{
			Operation: "CreateDatasource",
			Success:   false,
			Message:   fmt.Sprintf("Error: %v", err),
			Duration:  duration,
			Timestamp: time.Now(),
		}, 0, ""
	}

	return TestResult{
		Operation: "CreateDatasource",
		Success:   true,
		Message:   fmt.Sprintf("Created Datasource ID %d: %s", resp.Datasource.Id, resp.Datasource.Name),
		Duration:  duration,
		Timestamp: time.Now(),
	}, resp.Datasource.Id, resp.Datasource.Name
}

func testGetDatasource(ctx context.Context, dsID uint32, expectedName string) TestResult {
	start := time.Now()

	resp, err := ai_studio_sdk.GetDatasource(ctx, dsID)
	duration := time.Since(start)

	if err != nil {
		return TestResult{
			Operation: "GetDatasource",
			Success:   false,
			Message:   fmt.Sprintf("Error: %v", err),
			Duration:  duration,
			Timestamp: time.Now(),
		}
	}

	if resp.Datasource.Name != expectedName {
		return TestResult{
			Operation: "GetDatasource",
			Success:   false,
			Message:   fmt.Sprintf("Name mismatch: expected '%s', got '%s'", expectedName, resp.Datasource.Name),
			Duration:  duration,
			Timestamp: time.Now(),
		}
	}

	// Debug: Log datasource configuration to help troubleshoot
	configInfo := fmt.Sprintf("Retrieved Datasource ID %d: %s | Embedder: %s/%s | VectorStore: %s/%s | HasEmbedAPIKey: %v | HasDBAPIKey: %v",
		resp.Datasource.Id,
		resp.Datasource.Name,
		resp.Datasource.EmbedVendor,
		resp.Datasource.EmbedModel,
		resp.Datasource.DbSourceType,
		resp.Datasource.DbName,
		resp.Datasource.HasEmbedApiKey,
		resp.Datasource.HasDbConnApiKey,
	)

	return TestResult{
		Operation: "GetDatasource",
		Success:   true,
		Message:   configInfo,
		Duration:  duration,
		Timestamp: time.Now(),
	}
}

func testUpdateDatasource(ctx context.Context, dsID uint32) (TestResult, string) {
	start := time.Now()
	updatedName := generateTestName("Test Datasource Updated")

	resp, err := ai_studio_sdk.UpdateDatasource(
		ctx,
		dsID,
		updatedName,
		"Updated description",
		"Updated long description for CRUD testing",
		"",       // URL
		"chroma", // DB source type
		7,        // Updated privacy score
		1,        // User ID
		true,     // Active
	)

	duration := time.Since(start)

	if err != nil {
		return TestResult{
			Operation: "UpdateDatasource",
			Success:   false,
			Message:   fmt.Sprintf("Error: %v", err),
			Duration:  duration,
			Timestamp: time.Now(),
		}, ""
	}

	return TestResult{
		Operation: "UpdateDatasource",
		Success:   true,
		Message:   fmt.Sprintf("Updated Datasource ID %d: %s", resp.Datasource.Id, resp.Datasource.Name),
		Duration:  duration,
		Timestamp: time.Now(),
	}, resp.Datasource.Name
}

func testGenerateEmbedding(ctx context.Context, dsID uint32) (TestResult, [][]float32) {
	start := time.Now()

	// Test chunks for embedding generation
	testChunks := []string{
		"This is a test document chunk about AI Studio plugins.",
		"This chunk discusses RAG and vector embeddings.",
		"The third chunk covers semantic search capabilities.",
	}

	resp, err := ai_studio_sdk.GenerateEmbedding(ctx, dsID, testChunks)
	duration := time.Since(start)

	if err != nil {
		return TestResult{
			Operation: "GenerateEmbedding",
			Success:   false,
			Message:   fmt.Sprintf("gRPC Error: %v | Setup Required: Configure a datasource with EmbedVendor (e.g., 'openai') and EmbedModel (e.g., 'text-embedding-3-small')", err),
			Duration:  duration,
			Timestamp: time.Now(),
		}, nil
	}

	if !resp.Success {
		return TestResult{
			Operation: "GenerateEmbedding",
			Success:   false,
			Message:   fmt.Sprintf("Failed: %s | Setup Required: Ensure datasource has EmbedVendor='openai' and EmbedModel='text-embedding-3-small'", resp.ErrorMessage),
			Duration:  duration,
			Timestamp: time.Now(),
		}, nil
	}

	if len(resp.Vectors) != len(testChunks) {
		return TestResult{
			Operation: "GenerateEmbedding",
			Success:   false,
			Message:   fmt.Sprintf("Expected %d vectors, got %d", len(testChunks), len(resp.Vectors)),
			Duration:  duration,
			Timestamp: time.Now(),
		}, nil
	}

	// Extract embeddings for use in subsequent tests
	embeddings := make([][]float32, len(resp.Vectors))
	for i, vec := range resp.Vectors {
		embeddings[i] = vec.Values
	}

	return TestResult{
		Operation: "GenerateEmbedding",
		Success:   true,
		Message:   fmt.Sprintf("Generated %d embeddings (first vector has %d dimensions)", len(embeddings), len(embeddings[0])),
		Duration:  duration,
		Timestamp: time.Now(),
	}, embeddings
}

func testStoreDocuments(ctx context.Context, dsID uint32, embeddings [][]float32) TestResult {
	start := time.Now()

	// Create documents with pre-computed embeddings
	documents := make([]*mgmtpb.DocumentWithEmbedding, len(embeddings))
	for i, embedding := range embeddings {
		documents[i] = &mgmtpb.DocumentWithEmbedding{
			Content:   fmt.Sprintf("Test document chunk %d with custom chunking", i+1),
			Embedding: embedding,
			Metadata: map[string]string{
				"source":      "service-api-test",
				"chunk_index": fmt.Sprintf("%d", i),
				"test_type":   "pre_computed_embeddings",
			},
		}
	}

	resp, err := ai_studio_sdk.StoreDocuments(ctx, dsID, documents)
	duration := time.Since(start)

	if err != nil {
		return TestResult{
			Operation: "StoreDocuments",
			Success:   false,
			Message:   fmt.Sprintf("Error: %v", err),
			Duration:  duration,
			Timestamp: time.Now(),
		}
	}

	if !resp.Success {
		return TestResult{
			Operation: "StoreDocuments",
			Success:   false,
			Message:   fmt.Sprintf("Failed: %s | Setup Required: Datasource needs vector store configured (DBSourceType + DBConnString) and running", resp.ErrorMessage),
			Duration:  duration,
			Timestamp: time.Now(),
		}
	}

	return TestResult{
		Operation: "StoreDocuments",
		Success:   true,
		Message:   fmt.Sprintf("Stored %d documents with pre-computed embeddings", resp.StoredCount),
		Duration:  duration,
		Timestamp: time.Now(),
	}
}

func testProcessAndStoreDocuments(ctx context.Context, dsID uint32) TestResult {
	start := time.Now()

	// Create chunks (embeddings will be generated automatically)
	chunks := make([]*mgmtpb.DocumentChunk, 3)
	for i := range chunks {
		chunks[i] = &mgmtpb.DocumentChunk{
			Content: fmt.Sprintf("Auto-embedded chunk %d about AI Studio RAG capabilities", i+1),
			Metadata: map[string]string{
				"source":      "service-api-test",
				"chunk_index": fmt.Sprintf("%d", i),
				"test_type":   "auto_embedded",
			},
		}
	}

	resp, err := ai_studio_sdk.ProcessAndStoreDocuments(ctx, dsID, chunks)
	duration := time.Since(start)

	if err != nil {
		return TestResult{
			Operation: "ProcessAndStoreDocuments",
			Success:   false,
			Message:   fmt.Sprintf("Error: %v", err),
			Duration:  duration,
			Timestamp: time.Now(),
		}
	}

	if !resp.Success {
		return TestResult{
			Operation: "ProcessAndStoreDocuments",
			Success:   false,
			Message:   fmt.Sprintf("Failed: %s | Setup Required: Datasource needs both embedder (EmbedVendor/EmbedModel/EmbedAPIKey) AND vector store (DBSourceType/DBConnString) configured", resp.ErrorMessage),
			Duration:  duration,
			Timestamp: time.Now(),
		}
	}

	return TestResult{
		Operation: "ProcessAndStoreDocuments",
		Success:   true,
		Message:   fmt.Sprintf("Processed and stored %d documents", resp.ProcessedCount),
		Duration:  duration,
		Timestamp: time.Now(),
	}
}

func testQueryDatasource(ctx context.Context, dsID uint32) TestResult {
	start := time.Now()

	// Query with text (embedding generated automatically)
	resp, err := ai_studio_sdk.QueryDatasource(ctx, dsID, "AI Studio plugins RAG", 5, 0.0)
	duration := time.Since(start)

	if err != nil {
		return TestResult{
			Operation: "QueryDatasource",
			Success:   false,
			Message:   fmt.Sprintf("Error: %v", err),
			Duration:  duration,
			Timestamp: time.Now(),
		}
	}

	if !resp.Success {
		return TestResult{
			Operation: "QueryDatasource",
			Success:   false,
			Message:   fmt.Sprintf("Failed: %s | Setup Required: Datasource needs embedder configured AND vector store running with indexed data", resp.ErrorMessage),
			Duration:  duration,
			Timestamp: time.Now(),
		}
	}

	return TestResult{
		Operation: "QueryDatasource",
		Success:   true,
		Message:   fmt.Sprintf("Query returned %d results", len(resp.Results)),
		Duration:  duration,
		Timestamp: time.Now(),
	}
}

func testQueryDatasourceByVector(ctx context.Context, dsID uint32, queryVector []float32) TestResult {
	start := time.Now()

	// Query with pre-computed vector
	resp, err := ai_studio_sdk.QueryDatasourceByVector(ctx, dsID, queryVector, 5, 0.0)
	duration := time.Since(start)

	if err != nil {
		return TestResult{
			Operation: "QueryDatasourceByVector",
			Success:   false,
			Message:   fmt.Sprintf("Error: %v", err),
			Duration:  duration,
			Timestamp: time.Now(),
		}
	}

	if !resp.Success {
		return TestResult{
			Operation: "QueryDatasourceByVector",
			Success:   false,
			Message:   fmt.Sprintf("Failed: %s | Setup Required: Datasource needs vector store configured and running with indexed data", resp.ErrorMessage),
			Duration:  duration,
			Timestamp: time.Now(),
		}
	}

	return TestResult{
		Operation: "QueryDatasourceByVector",
		Success:   true,
		Message:   fmt.Sprintf("Vector query returned %d results", len(resp.Results)),
		Duration:  duration,
		Timestamp: time.Now(),
	}
}

func testProcessDatasourceEmbeddings(ctx context.Context, dsID uint32) TestResult {
	start := time.Now()

	// Trigger async embedding processing
	resp, err := ai_studio_sdk.ProcessDatasourceEmbeddings(ctx, dsID)
	duration := time.Since(start)

	if err != nil {
		return TestResult{
			Operation: "ProcessDatasourceEmbeddings",
			Success:   false,
			Message:   fmt.Sprintf("Error: %v", err),
			Duration:  duration,
			Timestamp: time.Now(),
		}
	}

	if !resp.Success {
		return TestResult{
			Operation: "ProcessDatasourceEmbeddings",
			Success:   false,
			Message:   fmt.Sprintf("Failed: %s | Setup Required: Datasource needs embedder + vector store configured, and files uploaded to the datasource", resp.Message),
			Duration:  duration,
			Timestamp: time.Now(),
		}
	}

	return TestResult{
		Operation: "ProcessDatasourceEmbeddings",
		Success:   true,
		Message:   fmt.Sprintf("Embedding processing started (job_id: %s)", resp.JobId),
		Duration:  duration,
		Timestamp: time.Now(),
	}
}

func testSearchDatasources(ctx context.Context) TestResult {
	start := time.Now()

	resp, err := ai_studio_sdk.SearchDatasources(ctx, "test")
	duration := time.Since(start)

	if err != nil {
		return TestResult{
			Operation: "SearchDatasources",
			Success:   false,
			Message:   fmt.Sprintf("Error: %v", err),
			Duration:  duration,
			Timestamp: time.Now(),
		}
	}

	return TestResult{
		Operation: "SearchDatasources",
		Success:   true,
		Message:   fmt.Sprintf("Search found %d datasources", len(resp.Datasources)),
		Duration:  duration,
		Timestamp: time.Now(),
	}
}

func testDeleteDatasource(ctx context.Context, dsID uint32) TestResult {
	start := time.Now()

	err := ai_studio_sdk.DeleteDatasource(ctx, dsID)
	duration := time.Since(start)

	if err != nil {
		return TestResult{
			Operation: "DeleteDatasource",
			Success:   false,
			Message:   fmt.Sprintf("Error: %v", err),
			Duration:  duration,
			Timestamp: time.Now(),
		}
	}

	return TestResult{
		Operation: "DeleteDatasource",
		Success:   true,
		Message:   fmt.Sprintf("Deleted Datasource ID %d", dsID),
		Duration:  duration,
		Timestamp: time.Now(),
	}
}

// === Advanced Metadata Operations Tests ===

func testDeleteDocumentsByMetadataDryRun(ctx context.Context, dsID uint32) TestResult {
	start := time.Now()

	// Test dry-run mode with a metadata filter
	metadata := map[string]string{"source": "service-api-test"}

	count, err := ai_studio_sdk.DeleteDocumentsByMetadata(ctx, dsID, metadata, "AND", true)
	duration := time.Since(start)

	if err != nil {
		return TestResult{
			Operation: "DeleteDocumentsByMetadata (dry-run)",
			Success:   false,
			Message:   fmt.Sprintf("Error: %v", err),
			Duration:  duration,
			Timestamp: time.Now(),
		}
	}

	return TestResult{
		Operation: "DeleteDocumentsByMetadata (dry-run)",
		Success:   true,
		Message:   fmt.Sprintf("Would delete %d document(s) with source='service-api-test'", count),
		Duration:  duration,
		Timestamp: time.Now(),
	}
}

func testDeleteDocumentsByMetadata(ctx context.Context, dsID uint32) TestResult {
	start := time.Now()

	// Delete documents with specific test_type metadata
	metadata := map[string]string{"test_type": "auto_embedded"}

	count, err := ai_studio_sdk.DeleteDocumentsByMetadata(ctx, dsID, metadata, "AND", false)
	duration := time.Since(start)

	if err != nil {
		return TestResult{
			Operation: "DeleteDocumentsByMetadata",
			Success:   false,
			Message:   fmt.Sprintf("Error: %v", err),
			Duration:  duration,
			Timestamp: time.Now(),
		}
	}

	return TestResult{
		Operation: "DeleteDocumentsByMetadata",
		Success:   true,
		Message:   fmt.Sprintf("Deleted %d document(s) with test_type='auto_embedded'", count),
		Duration:  duration,
		Timestamp: time.Now(),
	}
}

func testQueryByMetadataOnly(ctx context.Context, dsID uint32) TestResult {
	start := time.Now()

	// Query for documents with specific metadata
	metadata := map[string]string{"source": "service-api-test"}

	results, totalCount, err := ai_studio_sdk.QueryByMetadataOnly(ctx, dsID, metadata, "AND", 10, 0)
	duration := time.Since(start)

	if err != nil {
		return TestResult{
			Operation: "QueryByMetadataOnly",
			Success:   false,
			Message:   fmt.Sprintf("Error: %v", err),
			Duration:  duration,
			Timestamp: time.Now(),
		}
	}

	// Display first result as sample
	sampleContent := ""
	if len(results) > 0 {
		sampleContent = fmt.Sprintf("\n   First result preview: %s (metadata: %v)",
			truncateString(results[0].Content, 60),
			results[0].Metadata)
	}

	return TestResult{
		Operation: "QueryByMetadataOnly",
		Success:   true,
		Message:   fmt.Sprintf("Found %d/%d document(s) with source='service-api-test'%s", len(results), totalCount, sampleContent),
		Duration:  duration,
		Timestamp: time.Now(),
	}
}

func testListNamespaces(ctx context.Context, dsID uint32) TestResult {
	start := time.Now()

	namespaces, err := ai_studio_sdk.ListNamespaces(ctx, dsID)
	duration := time.Since(start)

	if err != nil {
		return TestResult{
			Operation: "ListNamespaces",
			Success:   false,
			Message:   fmt.Sprintf("Error: %v", err),
			Duration:  duration,
			Timestamp: time.Now(),
		}
	}

	// Build namespace summary
	namespaceSummary := ""
	for i, ns := range namespaces {
		if i < 3 { // Show first 3
			namespaceSummary += fmt.Sprintf("\n   - %s (%d docs)", ns.Name, ns.DocumentCount)
		}
	}
	if len(namespaces) > 3 {
		namespaceSummary += fmt.Sprintf("\n   ... and %d more", len(namespaces)-3)
	}

	return TestResult{
		Operation: "ListNamespaces",
		Success:   true,
		Message:   fmt.Sprintf("Found %d namespace(s)%s", len(namespaces), namespaceSummary),
		Duration:  duration,
		Timestamp: time.Now(),
	}
}

// Helper function to truncate long strings for display
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
