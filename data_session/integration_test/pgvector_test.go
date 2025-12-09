//go:build integration

package integration_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"github.com/pgvector/pgvector-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	pgTestDimensions = 384
)

// storeTestDocumentsPGVector stores documents directly to PostgreSQL using pgvector.
func storeTestDocumentsPGVector(t *testing.T, connStr, tableName string, count int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	db, err := sql.Open("postgres", connStr)
	require.NoError(t, err)
	defer db.Close()

	// Create table with vector column
	createTableSQL := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id VARCHAR(36) PRIMARY KEY,
			content TEXT NOT NULL,
			embedding vector(%d),
			metadata JSONB
		)
	`, tableName, pgTestDimensions)
	_, err = db.ExecContext(ctx, createTableSQL)
	require.NoError(t, err)

	contents := GenerateTestContents(count)
	vectors := GenerateTestVectors(count, pgTestDimensions)
	metadatas := GenerateTestMetadatas(count)

	for i := 0; i < count; i++ {
		docID := uuid.New().String()
		metadataJSON, err := json.Marshal(metadatas[i])
		require.NoError(t, err)

		// Convert []float32 to pgvector.Vector
		pgVec := pgvector.NewVector(vectors[i])

		insertSQL := fmt.Sprintf(`
			INSERT INTO %s (id, content, embedding, metadata)
			VALUES ($1, $2, $3, $4)
		`, tableName)
		_, err = db.ExecContext(ctx, insertSQL, docID, contents[i], pgVec, string(metadataJSON))
		require.NoError(t, err)
	}
}

// TestPGVectorContainerHealth verifies the PGVector container is healthy and accessible.
func TestPGVectorContainerHealth(t *testing.T) {
	pg := requirePGVector(t)
	ctx := context.Background()

	err := pg.Ping(ctx)
	require.NoError(t, err, "PGVector container should be healthy")
}

// TestPGVectorExtensionEnabled verifies the pgvector extension is enabled.
func TestPGVectorExtensionEnabled(t *testing.T) {
	pg := requirePGVector(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	db, err := sql.Open("postgres", pg.ConnectionString())
	require.NoError(t, err)
	defer db.Close()

	// Check if vector extension exists
	var extName string
	err = db.QueryRowContext(ctx, "SELECT extname FROM pg_extension WHERE extname = 'vector'").Scan(&extName)
	require.NoError(t, err, "pgvector extension should be installed")
	assert.Equal(t, "vector", extName)
}

// TestPGVectorStoreDocumentsDirectly tests storing documents using pgvector.
func TestPGVectorStoreDocumentsDirectly(t *testing.T) {
	pg := requirePGVector(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tableName := "test_store_" + uuid.New().String()[:8]

	// Store documents directly
	storeTestDocumentsPGVector(t, pg.ConnectionString(), tableName, 3)

	// Verify documents were stored
	count, err := pg.TableRowCount(ctx, tableName)
	require.NoError(t, err)
	assert.Equal(t, 3, count, "Table should have 3 documents")

	// Cleanup
	err = pg.DropTable(ctx, tableName)
	require.NoError(t, err)
}

// TestPGVectorSearchByVector tests similarity search using pgvector's cosine distance.
func TestPGVectorSearchByVector(t *testing.T) {
	pg := requirePGVector(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tableName := "test_search_" + uuid.New().String()[:8]

	// Store documents
	storeTestDocumentsPGVector(t, pg.ConnectionString(), tableName, 5)

	// Search using a query vector
	db, err := sql.Open("postgres", pg.ConnectionString())
	require.NoError(t, err)
	defer db.Close()

	queryVector := GenerateTestVectors(1, pgTestDimensions)[0]
	pgVec := pgvector.NewVector(queryVector)

	// Vector similarity search using <=> operator (cosine distance)
	query := fmt.Sprintf(`
		SELECT id, content, metadata, 1 - (embedding <=> $1) as similarity
		FROM %s
		ORDER BY embedding <=> $1
		LIMIT 3
	`, tableName)

	rows, err := db.QueryContext(ctx, query, pgVec)
	require.NoError(t, err)
	defer rows.Close()

	var results []struct {
		ID         string
		Content    string
		Metadata   string
		Similarity float64
	}

	for rows.Next() {
		var r struct {
			ID         string
			Content    string
			Metadata   string
			Similarity float64
		}
		err := rows.Scan(&r.ID, &r.Content, &r.Metadata, &r.Similarity)
		require.NoError(t, err)
		results = append(results, r)
	}
	require.NoError(t, rows.Err())

	assert.Len(t, results, 3, "Should return 3 results")
	assert.NotEmpty(t, results[0].ID, "First result should have an ID")
	assert.NotEmpty(t, results[0].Content, "First result should have content")

	// Cleanup
	_ = pg.DropTable(ctx, tableName)
}

// TestPGVectorSearchEmptyTable tests searching an empty table.
func TestPGVectorSearchEmptyTable(t *testing.T) {
	pg := requirePGVector(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tableName := "test_empty_" + uuid.New().String()[:8]

	// Create empty table
	err := pg.CreateVectorTable(ctx, tableName, pgTestDimensions)
	require.NoError(t, err)

	// Search empty table
	db, err := sql.Open("postgres", pg.ConnectionString())
	require.NoError(t, err)
	defer db.Close()

	queryVector := GenerateTestVectors(1, pgTestDimensions)[0]
	pgVec := pgvector.NewVector(queryVector)

	query := fmt.Sprintf(`
		SELECT id, content FROM %s
		ORDER BY embedding <=> $1
		LIMIT 5
	`, tableName)

	rows, err := db.QueryContext(ctx, query, pgVec)
	require.NoError(t, err)
	defer rows.Close()

	var count int
	for rows.Next() {
		count++
	}
	require.NoError(t, rows.Err())
	assert.Equal(t, 0, count, "Empty table should return no results")

	// Cleanup
	_ = pg.DropTable(ctx, tableName)
}

// TestPGVectorDeleteByMetadata tests deleting documents by metadata filter.
func TestPGVectorDeleteByMetadata(t *testing.T) {
	pg := requirePGVector(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tableName := "test_delete_" + uuid.New().String()[:8]

	// Store documents
	storeTestDocumentsPGVector(t, pg.ConnectionString(), tableName, 5)

	// Verify initial count
	beforeCount, err := pg.TableRowCount(ctx, tableName)
	require.NoError(t, err)
	assert.Equal(t, 5, beforeCount, "Should start with 5 documents")

	// Delete documents with category = "general" using JSON operator
	db, err := sql.Open("postgres", pg.ConnectionString())
	require.NoError(t, err)
	defer db.Close()

	deleteQuery := fmt.Sprintf("DELETE FROM %s WHERE metadata->>'category' = $1", tableName)
	result, err := db.ExecContext(ctx, deleteQuery, "general")
	require.NoError(t, err)

	deleted, err := result.RowsAffected()
	require.NoError(t, err)
	assert.Greater(t, deleted, int64(0), "Should delete at least one document")

	// Verify documents were deleted
	afterCount, err := pg.TableRowCount(ctx, tableName)
	require.NoError(t, err)
	assert.Less(t, afterCount, beforeCount, "Document count should decrease after delete")

	// Cleanup
	_ = pg.DropTable(ctx, tableName)
}

// TestPGVectorQueryByMetadataOnly tests querying by metadata using JSON operators.
func TestPGVectorQueryByMetadataOnly(t *testing.T) {
	pg := requirePGVector(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tableName := "test_metadata_query_" + uuid.New().String()[:8]

	// Store documents
	storeTestDocumentsPGVector(t, pg.ConnectionString(), tableName, 10)

	// Query by category using JSON operator
	db, err := sql.Open("postgres", pg.ConnectionString())
	require.NoError(t, err)
	defer db.Close()

	query := fmt.Sprintf("SELECT id, content, metadata FROM %s WHERE metadata->>'category' = $1", tableName)
	rows, err := db.QueryContext(ctx, query, "ml")
	require.NoError(t, err)
	defer rows.Close()

	var count int
	for rows.Next() {
		count++
	}
	require.NoError(t, rows.Err())
	assert.GreaterOrEqual(t, count, 1, "Should find at least 1 document with ml category")

	// Cleanup
	_ = pg.DropTable(ctx, tableName)
}

// TestPGVectorListTables tests listing tables with pgvector schema.
func TestPGVectorListTables(t *testing.T) {
	pg := requirePGVector(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create a few tables
	tables := []string{
		"test_list_1_" + uuid.New().String()[:8],
		"test_list_2_" + uuid.New().String()[:8],
	}

	for _, name := range tables {
		err := pg.CreateVectorTable(ctx, name, pgTestDimensions)
		require.NoError(t, err)
	}

	// List tables using information_schema
	db, err := sql.Open("postgres", pg.ConnectionString())
	require.NoError(t, err)
	defer db.Close()

	query := `
		SELECT t.table_name
		FROM information_schema.tables t
		WHERE t.table_schema = 'public'
		AND t.table_type = 'BASE TABLE'
		AND EXISTS (
			SELECT 1 FROM information_schema.columns c
			WHERE c.table_name = t.table_name
			AND c.column_name IN ('id', 'content', 'embedding', 'metadata')
			GROUP BY c.table_name
			HAVING COUNT(*) = 4
		)
		ORDER BY t.table_name
	`

	rows, err := db.QueryContext(ctx, query)
	require.NoError(t, err)
	defer rows.Close()

	var foundTables []string
	for rows.Next() {
		var name string
		err := rows.Scan(&name)
		require.NoError(t, err)
		foundTables = append(foundTables, name)
	}
	require.NoError(t, rows.Err())

	// Should include our test tables
	for _, expected := range tables {
		assert.Contains(t, foundTables, expected, "Should list created table")
	}

	// Cleanup
	for _, name := range tables {
		_ = pg.DropTable(ctx, name)
	}
}

// TestPGVectorDropTable tests dropping a table.
func TestPGVectorDropTable(t *testing.T) {
	pg := requirePGVector(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tableName := "test_drop_" + uuid.New().String()[:8]

	// Create table
	err := pg.CreateVectorTable(ctx, tableName, pgTestDimensions)
	require.NoError(t, err)

	// Verify it exists
	exists, err := pg.TableExists(ctx, tableName)
	require.NoError(t, err)
	assert.True(t, exists, "Table should exist before deletion")

	// Drop table
	err = pg.DropTable(ctx, tableName)
	require.NoError(t, err)

	// Verify table no longer exists
	exists, err = pg.TableExists(ctx, tableName)
	require.NoError(t, err)
	assert.False(t, exists, "Table should not exist after deletion")
}

// TestPGVectorMultipleDocumentsStorage tests storing and retrieving multiple documents.
func TestPGVectorMultipleDocumentsStorage(t *testing.T) {
	pg := requirePGVector(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	tableName := "test_multi_" + uuid.New().String()[:8]

	// Create table
	err := pg.CreateVectorTable(ctx, tableName, pgTestDimensions)
	require.NoError(t, err)

	db, err := sql.Open("postgres", pg.ConnectionString())
	require.NoError(t, err)
	defer db.Close()

	// Store multiple batches
	for batch := 0; batch < 3; batch++ {
		contents := GenerateTestContents(5)
		vectors := GenerateTestVectors(5, pgTestDimensions)
		metadatas := GenerateTestMetadatas(5)

		for i := 0; i < 5; i++ {
			docID := uuid.New().String()
			metadatas[i]["batch"] = batch
			metadataJSON, err := json.Marshal(metadatas[i])
			require.NoError(t, err)

			pgVec := pgvector.NewVector(vectors[i])

			insertSQL := fmt.Sprintf(`
				INSERT INTO %s (id, content, embedding, metadata)
				VALUES ($1, $2, $3, $4)
			`, tableName)
			_, err = db.ExecContext(ctx, insertSQL, docID, contents[i], pgVec, string(metadataJSON))
			require.NoError(t, err, "Batch %d, doc %d should store successfully", batch, i)
		}
	}

	// Verify total count
	count, err := pg.TableRowCount(ctx, tableName)
	require.NoError(t, err)
	assert.Equal(t, 15, count, "Should have 15 documents across 3 batches")

	// Cleanup
	_ = pg.DropTable(ctx, tableName)
}

// TestPGVectorUpsertBehavior tests upsert (ON CONFLICT) behavior.
func TestPGVectorUpsertBehavior(t *testing.T) {
	pg := requirePGVector(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tableName := "test_upsert_" + uuid.New().String()[:8]

	// Create table
	err := pg.CreateVectorTable(ctx, tableName, pgTestDimensions)
	require.NoError(t, err)

	db, err := sql.Open("postgres", pg.ConnectionString())
	require.NoError(t, err)
	defer db.Close()

	docID := uuid.New().String()
	content1 := "Original content"
	content2 := "Updated content"
	vector := GenerateTestVectors(1, pgTestDimensions)[0]
	pgVec := pgvector.NewVector(vector)
	metadata := map[string]any{"version": 1}
	metadataJSON, _ := json.Marshal(metadata)

	// Insert first document
	insertSQL := fmt.Sprintf(`
		INSERT INTO %s (id, content, embedding, metadata)
		VALUES ($1, $2, $3, $4)
	`, tableName)
	_, err = db.ExecContext(ctx, insertSQL, docID, content1, pgVec, string(metadataJSON))
	require.NoError(t, err)

	// Upsert with same ID, different content
	metadata["version"] = 2
	metadataJSON, _ = json.Marshal(metadata)
	upsertSQL := fmt.Sprintf(`
		INSERT INTO %s (id, content, embedding, metadata)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (id) DO UPDATE SET
			content = EXCLUDED.content,
			embedding = EXCLUDED.embedding,
			metadata = EXCLUDED.metadata
	`, tableName)
	_, err = db.ExecContext(ctx, upsertSQL, docID, content2, pgVec, string(metadataJSON))
	require.NoError(t, err)

	// Verify only one document exists with updated content
	count, err := pg.TableRowCount(ctx, tableName)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "Should have only 1 document after upsert")

	var retrievedContent string
	err = db.QueryRowContext(ctx, fmt.Sprintf("SELECT content FROM %s WHERE id = $1", tableName), docID).Scan(&retrievedContent)
	require.NoError(t, err)
	assert.Equal(t, content2, retrievedContent, "Content should be updated")

	// Cleanup
	_ = pg.DropTable(ctx, tableName)
}

// TestPGVectorDistanceOperators tests different distance operators.
func TestPGVectorDistanceOperators(t *testing.T) {
	pg := requirePGVector(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tableName := "test_distance_" + uuid.New().String()[:8]

	// Store documents
	storeTestDocumentsPGVector(t, pg.ConnectionString(), tableName, 5)

	db, err := sql.Open("postgres", pg.ConnectionString())
	require.NoError(t, err)
	defer db.Close()

	queryVector := GenerateTestVectors(1, pgTestDimensions)[0]
	pgVec := pgvector.NewVector(queryVector)

	// Test cosine distance (<=>)
	t.Run("CosineDistance", func(t *testing.T) {
		query := fmt.Sprintf("SELECT id, embedding <=> $1 as distance FROM %s ORDER BY distance LIMIT 1", tableName)
		var id string
		var distance float64
		err := db.QueryRowContext(ctx, query, pgVec).Scan(&id, &distance)
		require.NoError(t, err)
		assert.NotEmpty(t, id)
		assert.GreaterOrEqual(t, distance, 0.0, "Cosine distance should be non-negative")
	})

	// Test L2 distance (<->)
	t.Run("L2Distance", func(t *testing.T) {
		query := fmt.Sprintf("SELECT id, embedding <-> $1 as distance FROM %s ORDER BY distance LIMIT 1", tableName)
		var id string
		var distance float64
		err := db.QueryRowContext(ctx, query, pgVec).Scan(&id, &distance)
		require.NoError(t, err)
		assert.NotEmpty(t, id)
		assert.GreaterOrEqual(t, distance, 0.0, "L2 distance should be non-negative")
	})

	// Test inner product (<#>)
	t.Run("InnerProduct", func(t *testing.T) {
		query := fmt.Sprintf("SELECT id, embedding <#> $1 as distance FROM %s ORDER BY distance LIMIT 1", tableName)
		var id string
		var distance float64
		err := db.QueryRowContext(ctx, query, pgVec).Scan(&id, &distance)
		require.NoError(t, err)
		assert.NotEmpty(t, id)
		// Inner product can be negative for normalized vectors
	})

	// Cleanup
	_ = pg.DropTable(ctx, tableName)
}
