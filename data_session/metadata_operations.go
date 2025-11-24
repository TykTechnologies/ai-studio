package data_session

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	chromago "github.com/amikos-tech/chroma-go/pkg/api/v2"
	chromaEmbeddings "github.com/amikos-tech/chroma-go/pkg/embeddings"
	pineconeSDK "github.com/pinecone-io/go-pinecone/pinecone"
	weaviateSDK "github.com/weaviate/weaviate-go-client/v5/weaviate"
	"github.com/weaviate/weaviate-go-client/v5/weaviate/auth"
	"github.com/weaviate/weaviate-go-client/v5/weaviate/filters"
	"github.com/weaviate/weaviate-go-client/v5/weaviate/graphql"
	weaviateModels "github.com/weaviate/weaviate/entities/models"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/tmc/langchaingo/schema"
	"google.golang.org/protobuf/types/known/structpb"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// This file contains vendor-specific implementations for metadata operations
// (delete by metadata, query by metadata, namespace management)

// DeleteDocumentsByMetadata deletes documents matching metadata filter from vector store
// filterMode can be "AND" or "OR" to combine multiple metadata conditions
// If dryRun is true, returns count without actually deleting
func (ds *DataSession) DeleteDocumentsByMetadata(dsID uint, metadataFilter map[string]string, filterMode string, dryRun bool) (int, error) {
	d, ok := ds.Sources[dsID]
	if !ok {
		return 0, fmt.Errorf("datasource with id %d not found", dsID)
	}

	if len(metadataFilter) == 0 {
		return 0, fmt.Errorf("metadata filter cannot be empty")
	}

	// Validate filter mode
	if filterMode != "AND" && filterMode != "OR" {
		filterMode = "AND" // default
	}

	ctx := context.Background()

	switch d.DBSourceType {
	case VECTOR_PINECONE:
		return ds.deletePineconeByMetadata(ctx, d, metadataFilter, filterMode, dryRun)
	case VECTOR_CHROMA:
		return ds.deleteChromaByMetadata(ctx, d, metadataFilter, filterMode, dryRun)
	case VECTOR_PGVECTOR:
		return ds.deletePGVectorByMetadata(ctx, d, metadataFilter, filterMode, dryRun)
	case VECTOR_WEAVIATE:
		return ds.deleteWeaviateByMetadata(ctx, d, metadataFilter, filterMode, dryRun)
	case VECTOR_REDIS:
		return 0, fmt.Errorf("delete by metadata not fully supported for Redis vector store")
	case VECTOR_QDRANT:
		return 0, fmt.Errorf("delete by metadata not yet implemented for Qdrant vector store")
	default:
		return 0, fmt.Errorf("unsupported vector store type: %s", d.DBSourceType)
	}
}

// QueryByMetadataOnly queries documents using only metadata filters (no vector similarity)
// Supports pagination with limit and offset parameters
func (ds *DataSession) QueryByMetadataOnly(dsID uint, metadataFilter map[string]string, filterMode string, limit, offset int) ([]schema.Document, int, error) {
	d, ok := ds.Sources[dsID]
	if !ok {
		return nil, 0, fmt.Errorf("datasource with id %d not found", dsID)
	}

	if len(metadataFilter) == 0 {
		return nil, 0, fmt.Errorf("metadata filter cannot be empty")
	}

	// Validate and set defaults
	if limit <= 0 || limit > 100 {
		limit = 10
	}
	if offset < 0 {
		offset = 0
	}
	if filterMode != "AND" && filterMode != "OR" {
		filterMode = "AND" // default
	}

	ctx := context.Background()

	switch d.DBSourceType {
	case VECTOR_PINECONE:
		return ds.queryPineconeByMetadata(ctx, d, metadataFilter, filterMode, limit, offset)
	case VECTOR_CHROMA:
		return ds.queryChromaByMetadata(ctx, d, metadataFilter, filterMode, limit, offset)
	case VECTOR_PGVECTOR:
		return ds.queryPGVectorByMetadata(ctx, d, metadataFilter, filterMode, limit, offset)
	case VECTOR_WEAVIATE:
		return ds.queryWeaviateByMetadata(ctx, d, metadataFilter, filterMode, limit, offset)
	default:
		return nil, 0, fmt.Errorf("query by metadata not supported for %s vector store", d.DBSourceType)
	}
}

// ListNamespaces lists all namespaces/collections in the vector store
func (ds *DataSession) ListNamespaces(dsID uint) ([]NamespaceInfo, error) {
	d, ok := ds.Sources[dsID]
	if !ok {
		return nil, fmt.Errorf("datasource with id %d not found", dsID)
	}

	ctx := context.Background()

	switch d.DBSourceType {
	case VECTOR_PINECONE:
		return ds.listPineconeNamespaces(ctx, d)
	case VECTOR_CHROMA:
		return ds.listChromaCollections(ctx, d)
	case VECTOR_PGVECTOR:
		return ds.listPGVectorTables(ctx, d)
	case VECTOR_WEAVIATE:
		return ds.listWeaviateClasses(ctx, d)
	case VECTOR_QDRANT:
		return ds.listQdrantCollections(ctx, d)
	case VECTOR_REDIS:
		return nil, fmt.Errorf("list namespaces not supported for Redis vector store")
	default:
		return nil, fmt.Errorf("list namespaces not supported for %s vector store", d.DBSourceType)
	}
}

// DeleteNamespace deletes an entire namespace/collection from the vector store
func (ds *DataSession) DeleteNamespace(dsID uint, namespace string) error {
	d, ok := ds.Sources[dsID]
	if !ok {
		return fmt.Errorf("datasource with id %d not found", dsID)
	}

	if namespace == "" {
		return fmt.Errorf("namespace cannot be empty")
	}

	ctx := context.Background()

	switch d.DBSourceType {
	case VECTOR_PINECONE:
		return ds.deletePineconeNamespace(ctx, d, namespace)
	case VECTOR_CHROMA:
		return ds.deleteChromaCollection(ctx, d, namespace)
	case VECTOR_PGVECTOR:
		return ds.deletePGVectorTable(ctx, d, namespace)
	case VECTOR_WEAVIATE:
		return ds.deleteWeaviateClass(ctx, d, namespace)
	case VECTOR_QDRANT:
		return ds.deleteQdrantCollection(ctx, d, namespace)
	case VECTOR_REDIS:
		return fmt.Errorf("delete namespace not supported for Redis vector store")
	default:
		return fmt.Errorf("delete namespace not supported for %s vector store", d.DBSourceType)
	}
}

// ============================================================================
// Vendor-Specific Implementations (Stubs)
// ============================================================================

// Pinecone implementations
func (ds *DataSession) deletePineconeByMetadata(ctx context.Context, d *models.Datasource, filter map[string]string, filterMode string, dryRun bool) (int, error) {
	// Create Pinecone client
	pc, err := pineconeSDK.NewClient(pineconeSDK.NewClientParams{
		ApiKey: d.DBConnAPIKey,
	})
	if err != nil {
		return 0, fmt.Errorf("failed to create pinecone client: %w", err)
	}

	// Get index connection
	idx, err := pc.Index(pineconeSDK.NewIndexConnParams{
		Host:      d.DBConnString,
		Namespace: d.DBName,
	})
	if err != nil {
		return 0, fmt.Errorf("failed to connect to pinecone index: %w", err)
	}
	defer idx.Close()

	// Build metadata filter
	metadataFilter, err := ds.buildPineconeMetadataFilter(filter, filterMode)
	if err != nil {
		return 0, fmt.Errorf("failed to build metadata filter: %w", err)
	}

	if dryRun {
		// Get stats with filter to count matches
		stats, err := idx.DescribeIndexStatsFiltered(ctx, metadataFilter)
		if err != nil {
			return 0, fmt.Errorf("failed to describe index stats: %w", err)
		}

		// Sum up vector counts across all namespaces
		totalCount := uint32(0)
		if stats.Namespaces != nil {
			for _, ns := range stats.Namespaces {
				totalCount += ns.VectorCount
			}
		}
		return int(totalCount), nil
	}

	// Get count first using stats
	stats, err := idx.DescribeIndexStatsFiltered(ctx, metadataFilter)
	if err != nil {
		return 0, fmt.Errorf("failed to describe index stats: %w", err)
	}
	totalCount := uint32(0)
	if stats.Namespaces != nil {
		for _, ns := range stats.Namespaces {
			totalCount += ns.VectorCount
		}
	}

	// Delete vectors by filter
	err = idx.DeleteVectorsByFilter(ctx, metadataFilter)
	if err != nil {
		return 0, fmt.Errorf("failed to delete vectors: %w", err)
	}

	slog.Info("Deleted vectors from Pinecone by metadata", "count", totalCount, "namespace", d.DBName)
	return int(totalCount), nil
}

func (ds *DataSession) queryPineconeByMetadata(ctx context.Context, d *models.Datasource, filter map[string]string, filterMode string, limit, offset int) ([]schema.Document, int, error) {
	// Note: Pinecone doesn't support pure metadata queries without vector similarity
	// We need to use ListVectors which returns IDs, then fetch them
	// However, this is expensive and has pagination limits

	// Create Pinecone client
	pc, err := pineconeSDK.NewClient(pineconeSDK.NewClientParams{
		ApiKey: d.DBConnAPIKey,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create pinecone client: %w", err)
	}

	// Get index connection
	idx, err := pc.Index(pineconeSDK.NewIndexConnParams{
		Host:      d.DBConnString,
		Namespace: d.DBName,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("failed to connect to pinecone index: %w", err)
	}
	defer idx.Close()

	// Build metadata filter
	metadataFilter, err := ds.buildPineconeMetadataFilter(filter, filterMode)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to build metadata filter: %w", err)
	}

	// Get total count using stats
	stats, err := idx.DescribeIndexStatsFiltered(ctx, metadataFilter)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to describe index stats: %w", err)
	}
	totalCount := uint32(0)
	if stats.Namespaces != nil {
		for _, ns := range stats.Namespaces {
			totalCount += ns.VectorCount
		}
	}

	// List vectors with pagination
	// Note: Pinecone's ListVectors has a limit of 100 per request
	limitPtr := uint32(limit)
	listReq := &pineconeSDK.ListVectorsRequest{
		Limit: &limitPtr,
	}

	// Pinecone doesn't support offset, use pagination token instead
	// For now, we'll only support the first page
	if offset > 0 {
		slog.Warn("Pinecone doesn't support offset-based pagination, returning first page only")
	}

	listResp, err := idx.ListVectors(ctx, listReq)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list vectors: %w", err)
	}

	// Fetch vectors with metadata
	if len(listResp.VectorIds) == 0 {
		return []schema.Document{}, int(totalCount), nil
	}

	// Convert []*string to []string
	vectorIds := make([]string, len(listResp.VectorIds))
	for i, idPtr := range listResp.VectorIds {
		if idPtr != nil {
			vectorIds[i] = *idPtr
		}
	}

	fetchResp, err := idx.FetchVectors(ctx, vectorIds)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to fetch vectors: %w", err)
	}

	// Convert to schema.Document
	docs := make([]schema.Document, 0)
	for _, vec := range fetchResp.Vectors {
		// Extract content from metadata
		content := ""
		metadata := make(map[string]any)

		if vec.Metadata != nil {
			for k, v := range vec.Metadata.Fields {
				if k == "content" {
					if strVal, ok := v.GetKind().(*structpb.Value_StringValue); ok {
						content = strVal.StringValue
					}
				} else {
					metadata[k] = v.AsInterface()
				}
			}
		}

		// Check for base64 encoding
		if enc, ok := metadata["encoding"]; ok {
			if encStr, ok := enc.(string); ok && encStr == "base64" {
				decodedContent, err := base64.StdEncoding.DecodeString(content)
				if err != nil {
					slog.Error("error decoding base64 content", "err", err)
				} else {
					content = string(decodedContent)
				}
			}
		}

		docs = append(docs, schema.Document{
			PageContent: content,
			Metadata:    metadata,
			Score:       0,
		})
	}

	return docs, int(totalCount), nil
}

func (ds *DataSession) listPineconeNamespaces(ctx context.Context, d *models.Datasource) ([]NamespaceInfo, error) {
	// Create Pinecone client
	pc, err := pineconeSDK.NewClient(pineconeSDK.NewClientParams{
		ApiKey: d.DBConnAPIKey,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create pinecone client: %w", err)
	}

	// Get index connection (without specific namespace to get all stats)
	idx, err := pc.Index(pineconeSDK.NewIndexConnParams{Host: d.DBConnString})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to pinecone index: %w", err)
	}
	defer idx.Close()

	// Describe index stats to get all namespaces
	stats, err := idx.DescribeIndexStats(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to describe pinecone index: %w", err)
	}

	// Convert to NamespaceInfo
	namespaces := make([]NamespaceInfo, 0)
	if stats.Namespaces != nil {
		for name, nsStats := range stats.Namespaces {
			namespaces = append(namespaces, NamespaceInfo{
				Name:          name,
				DocumentCount: int(nsStats.VectorCount),
			})
		}
	}

	return namespaces, nil
}

func (ds *DataSession) deletePineconeNamespace(ctx context.Context, d *models.Datasource, namespace string) error {
	// Create Pinecone client
	pc, err := pineconeSDK.NewClient(pineconeSDK.NewClientParams{
		ApiKey: d.DBConnAPIKey,
	})
	if err != nil {
		return fmt.Errorf("failed to create pinecone client: %w", err)
	}

	// Get index connection with specific namespace
	idx, err := pc.Index(pineconeSDK.NewIndexConnParams{
		Host:      d.DBConnString,
		Namespace: namespace,
	})
	if err != nil {
		return fmt.Errorf("failed to connect to pinecone index: %w", err)
	}
	defer idx.Close()

	// Delete all vectors in namespace
	err = idx.DeleteAllVectorsInNamespace(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete pinecone namespace '%s': %w", namespace, err)
	}

	slog.Warn("Deleted all vectors in Pinecone namespace", "namespace", namespace)
	return nil
}

// buildPineconeMetadataFilter builds a Pinecone metadata filter from a map
func (ds *DataSession) buildPineconeMetadataFilter(filter map[string]string, filterMode string) (*pineconeSDK.MetadataFilter, error) {
	if len(filter) == 0 {
		return nil, fmt.Errorf("filter cannot be empty")
	}

	// Build filter map for structpb
	filterMap := make(map[string]interface{})

	if filterMode == "OR" {
		// OR mode: {"$or": [{"key1": {"$eq": "val1"}}, {"key2": {"$eq": "val2"}}]}
		orClauses := make([]map[string]interface{}, 0, len(filter))
		for key, value := range filter {
			orClauses = append(orClauses, map[string]interface{}{
				key: map[string]interface{}{"$eq": value},
			})
		}
		filterMap["$or"] = orClauses
	} else {
		// AND mode: {"key1": {"$eq": "val1"}, "key2": {"$eq": "val2"}}
		for key, value := range filter {
			filterMap[key] = map[string]interface{}{"$eq": value}
		}
	}

	// Convert to structpb
	metadataFilter, err := structpb.NewStruct(filterMap)
	if err != nil {
		return nil, fmt.Errorf("failed to create metadata filter: %w", err)
	}

	return (*pineconeSDK.MetadataFilter)(metadataFilter), nil
}

// Chroma implementations
func (ds *DataSession) deleteChromaByMetadata(ctx context.Context, d *models.Datasource, filter map[string]string, filterMode string, dryRun bool) (int, error) {
	// Create Chroma v2 client
	client, err := chromago.NewHTTPClient(chromago.WithBaseURL(d.DBConnString))
	if err != nil {
		return 0, fmt.Errorf("failed to create chroma client: %w", err)
	}

	// Get collection with no-op embedder (we're not doing vector operations)
	noopEmbedder := chromaEmbeddings.NewConsistentHashEmbeddingFunction()
	collection, err := client.GetCollection(ctx, d.DBName, chromago.WithEmbeddingFunctionGet(noopEmbedder))
	if err != nil {
		return 0, fmt.Errorf("failed to get chroma collection '%s': %w", d.DBName, err)
	}

	// Build WHERE clause from metadata filter
	whereFilter, err := ds.buildChromaWhereFilter(filter, filterMode)
	if err != nil {
		return 0, fmt.Errorf("failed to build where filter: %w", err)
	}

	if dryRun {
		// Get documents with filter to count exact matches
		result, err := collection.Get(ctx, chromago.WithWhereGet(whereFilter))
		if err != nil {
			return 0, fmt.Errorf("failed to get matching documents: %w", err)
		}

		return result.Count(), nil
	}

	// Get matching documents first to count them
	result, err := collection.Get(ctx, chromago.WithWhereGet(whereFilter))
	if err != nil {
		return 0, fmt.Errorf("failed to get matching documents: %w", err)
	}
	count := result.Count()

	// Delete documents
	err = collection.Delete(ctx, chromago.WithWhereDelete(whereFilter))
	if err != nil {
		return 0, fmt.Errorf("failed to delete documents: %w", err)
	}

	slog.Info("Deleted documents from Chroma by metadata", "count", count, "collection", d.DBName)
	return count, nil
}

func (ds *DataSession) queryChromaByMetadata(ctx context.Context, d *models.Datasource, filter map[string]string, filterMode string, limit, offset int) ([]schema.Document, int, error) {
	// Create Chroma v2 client
	client, err := chromago.NewHTTPClient(chromago.WithBaseURL(d.DBConnString))
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create chroma client: %w", err)
	}

	// Get collection with no-op embedder (we're not doing vector operations)
	noopEmbedder := chromaEmbeddings.NewConsistentHashEmbeddingFunction()
	collection, err := client.GetCollection(ctx, d.DBName, chromago.WithEmbeddingFunctionGet(noopEmbedder))
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get chroma collection '%s': %w", d.DBName, err)
	}

	// Build WHERE clause
	whereFilter, err := ds.buildChromaWhereFilter(filter, filterMode)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to build where filter: %w", err)
	}

	// Get total count first (for pagination)
	totalResult, err := collection.Get(ctx, chromago.WithWhereGet(whereFilter))
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get total count: %w", err)
	}
	totalCount := totalResult.Count()

	// Get documents with pagination
	result, err := collection.Get(ctx,
		chromago.WithWhereGet(whereFilter),
		chromago.WithLimitGet(limit),
		chromago.WithOffsetGet(offset),
		chromago.WithIncludeGet(chromago.IncludeMetadatas),
	)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get documents: %w", err)
	}

	// Convert to Records for easier access to all fields
	records := result.ToRecords()

	docs := make([]schema.Document, 0, len(records))
	for _, record := range records {
		content := record.Document().ContentString()
		chromaMeta := record.Metadata()

		// Check for base64 encoding in metadata
		if enc, ok := chromaMeta.GetString("encoding"); ok && enc == "base64" {
			decodedContent, err := base64.StdEncoding.DecodeString(content)
			if err != nil {
				slog.Error("error decoding base64 content", "err", err)
			} else {
				content = string(decodedContent)
			}
		}

		// Store metadata as an opaque interface - Chroma v2 doesn't provide iteration
		// Users can access specific keys via GetString, GetInt, etc. if needed
		metadata := make(map[string]any)
		metadata["_chroma_metadata"] = chromaMeta

		// Try to extract common metadata fields if they exist
		if val, ok := chromaMeta.GetString("source"); ok {
			metadata["source"] = val
		}
		if val, ok := chromaMeta.GetString("file_path"); ok {
			metadata["file_path"] = val
		}
		if val, ok := chromaMeta.GetString("chunk_index"); ok {
			metadata["chunk_index"] = val
		}
		if val, ok := chromaMeta.GetString("test_type"); ok {
			metadata["test_type"] = val
		}

		docs = append(docs, schema.Document{
			PageContent: content,
			Metadata:    metadata,
			Score:       0, // No similarity score for metadata-only query
		})
	}

	return docs, totalCount, nil
}

func (ds *DataSession) listChromaCollections(ctx context.Context, d *models.Datasource) ([]NamespaceInfo, error) {
	// Create Chroma v2 client
	client, err := chromago.NewHTTPClient(chromago.WithBaseURL(d.DBConnString))
	if err != nil {
		return nil, fmt.Errorf("failed to create chroma client: %w", err)
	}

	// List all collections
	collections, err := client.ListCollections(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list chroma collections: %w", err)
	}

	// Convert to NamespaceInfo
	namespaces := make([]NamespaceInfo, 0, len(collections))
	for _, coll := range collections {
		count, err := coll.Count(ctx)
		if err != nil {
			slog.Warn("Failed to count documents in collection", "collection", coll.Name(), "error", err)
			count = -1 // Mark as unknown
		}

		namespaces = append(namespaces, NamespaceInfo{
			Name:          coll.Name(),
			DocumentCount: count,
		})
	}

	return namespaces, nil
}

func (ds *DataSession) deleteChromaCollection(ctx context.Context, d *models.Datasource, namespace string) error {
	// Create Chroma v2 client
	client, err := chromago.NewHTTPClient(chromago.WithBaseURL(d.DBConnString))
	if err != nil {
		return fmt.Errorf("failed to create chroma client: %w", err)
	}

	// Delete collection
	err = client.DeleteCollection(ctx, namespace)
	if err != nil {
		return fmt.Errorf("failed to delete chroma collection '%s': %w", namespace, err)
	}

	slog.Warn("Deleted Chroma collection", "collection", namespace)
	return nil
}

// buildChromaWhereFilter builds a Chroma WHERE filter from a metadata map
func (ds *DataSession) buildChromaWhereFilter(filter map[string]string, filterMode string) (chromago.WhereClause, error) {
	if len(filter) == 0 {
		return nil, fmt.Errorf("filter cannot be empty")
	}

	// Build individual clauses
	clauses := make([]chromago.WhereClause, 0, len(filter))
	for key, value := range filter {
		clauses = append(clauses, chromago.EqString(key, value))
	}

	// Combine with AND or OR
	if filterMode == "OR" {
		return chromago.Or(clauses...), nil
	}
	return chromago.And(clauses...), nil
}

// PGVector implementations
func (ds *DataSession) deletePGVectorByMetadata(ctx context.Context, d *models.Datasource, filter map[string]string, filterMode string, dryRun bool) (int, error) {
	// Connect to PostgreSQL
	db, err := gorm.Open(postgres.Open(d.DBConnString), &gorm.Config{})
	if err != nil {
		return 0, fmt.Errorf("failed to connect to postgres: %w", err)
	}

	tableName := d.DBName

	// Build WHERE clause from metadata filter
	whereClause, args := ds.buildPGVectorWhereClause(filter, filterMode)

	if dryRun {
		// Count matching documents
		var count int64
		query := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE %s", tableName, whereClause)
		err := db.Raw(query, args...).Count(&count).Error
		if err != nil {
			return 0, fmt.Errorf("failed to count matching documents: %w", err)
		}
		return int(count), nil
	}

	// Get count first
	var count int64
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE %s", tableName, whereClause)
	err = db.Raw(countQuery, args...).Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("failed to count documents before deletion: %w", err)
	}

	// Delete documents
	deleteQuery := fmt.Sprintf("DELETE FROM %s WHERE %s", tableName, whereClause)
	err = db.Exec(deleteQuery, args...).Error
	if err != nil {
		return 0, fmt.Errorf("failed to delete documents: %w", err)
	}

	slog.Info("Deleted documents from PGVector by metadata", "count", count, "table", tableName)
	return int(count), nil
}

func (ds *DataSession) queryPGVectorByMetadata(ctx context.Context, d *models.Datasource, filter map[string]string, filterMode string, limit, offset int) ([]schema.Document, int, error) {
	// Connect to PostgreSQL
	db, err := gorm.Open(postgres.Open(d.DBConnString), &gorm.Config{})
	if err != nil {
		return nil, 0, fmt.Errorf("failed to connect to postgres: %w", err)
	}

	tableName := d.DBName

	// Build WHERE clause
	whereClause, args := ds.buildPGVectorWhereClause(filter, filterMode)

	// Get total count
	var totalCount int64
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE %s", tableName, whereClause)
	err = db.Raw(countQuery, args...).Count(&totalCount).Error
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count documents: %w", err)
	}

	// Query documents with pagination
	type Row struct {
		ID       string `gorm:"column:id"`
		Content  string `gorm:"column:content"`
		Metadata string `gorm:"column:metadata"`
	}

	var rows []Row
	query := fmt.Sprintf("SELECT id, content, metadata FROM %s WHERE %s LIMIT ? OFFSET ?", tableName, whereClause)
	args = append(args, limit, offset)
	err = db.Raw(query, args...).Scan(&rows).Error
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query documents: %w", err)
	}

	// Convert to schema.Document
	docs := make([]schema.Document, 0, len(rows))
	for _, row := range rows {
		content := row.Content

		// Parse metadata JSON
		var metadata map[string]any
		if row.Metadata != "" {
			if err := json.Unmarshal([]byte(row.Metadata), &metadata); err != nil {
				slog.Warn("Failed to parse metadata JSON", "error", err)
				metadata = make(map[string]any)
			}
		} else {
			metadata = make(map[string]any)
		}

		// Check for base64 encoding
		if enc, ok := metadata["encoding"]; ok {
			if encStr, ok := enc.(string); ok && encStr == "base64" {
				decodedContent, err := base64.StdEncoding.DecodeString(content)
				if err != nil {
					slog.Error("error decoding base64 content", "err", err)
				} else {
					content = string(decodedContent)
				}
			}
		}

		docs = append(docs, schema.Document{
			PageContent: content,
			Metadata:    metadata,
			Score:       0, // No similarity score for metadata-only query
		})
	}

	return docs, int(totalCount), nil
}

func (ds *DataSession) listPGVectorTables(ctx context.Context, d *models.Datasource) ([]NamespaceInfo, error) {
	// Connect to PostgreSQL
	db, err := gorm.Open(postgres.Open(d.DBConnString), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to postgres: %w", err)
	}

	// Query for tables with the expected schema (id, content, embedding, metadata columns)
	type TableInfo struct {
		TableName string `gorm:"column:table_name"`
		RowCount  int64  `gorm:"column:row_count"`
	}

	var tables []TableInfo
	query := `
		SELECT
			t.table_name,
			COALESCE(s.n_live_tup, 0) as row_count
		FROM information_schema.tables t
		LEFT JOIN pg_stat_user_tables s ON t.table_name = s.relname
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

	err = db.Raw(query).Scan(&tables).Error
	if err != nil {
		return nil, fmt.Errorf("failed to list pgvector tables: %w", err)
	}

	// Convert to NamespaceInfo
	namespaces := make([]NamespaceInfo, 0, len(tables))
	for _, table := range tables {
		namespaces = append(namespaces, NamespaceInfo{
			Name:          table.TableName,
			DocumentCount: int(table.RowCount),
		})
	}

	return namespaces, nil
}

func (ds *DataSession) deletePGVectorTable(ctx context.Context, d *models.Datasource, namespace string) error {
	// Connect to PostgreSQL
	db, err := gorm.Open(postgres.Open(d.DBConnString), &gorm.Config{})
	if err != nil {
		return fmt.Errorf("failed to connect to postgres: %w", err)
	}

	// Drop the table (with CASCADE to remove dependencies)
	query := fmt.Sprintf("DROP TABLE IF EXISTS %s CASCADE", namespace)
	err = db.Exec(query).Error
	if err != nil {
		return fmt.Errorf("failed to drop pgvector table '%s': %w", namespace, err)
	}

	slog.Warn("Deleted PGVector table", "table", namespace)
	return nil
}

// buildPGVectorWhereClause builds a PostgreSQL WHERE clause for JSON metadata filtering
func (ds *DataSession) buildPGVectorWhereClause(filter map[string]string, filterMode string) (string, []interface{}) {
	if len(filter) == 0 {
		return "1=1", []interface{}{}
	}

	var clauses []string
	var args []interface{}
	argIndex := 1

	for key, value := range filter {
		// Use JSON operators to query metadata column
		// metadata->>'key' = value
		clause := fmt.Sprintf("metadata->>'%s' = $%d", key, argIndex)
		clauses = append(clauses, clause)
		args = append(args, value)
		argIndex++
	}

	// Combine with AND or OR
	operator := " AND "
	if filterMode == "OR" {
		operator = " OR "
	}

	return strings.Join(clauses, operator), args
}

// Weaviate implementations
func (ds *DataSession) deleteWeaviateByMetadata(ctx context.Context, d *models.Datasource, filter map[string]string, filterMode string, dryRun bool) (int, error) {
	// Parse namespace format: ClassName:Namespace
	split := strings.Split(d.DBName, ":")
	if len(split) != 2 {
		return 0, fmt.Errorf("invalid weaviate namespace format, expected ClassName:Namespace")
	}
	className := split[0]

	// Create client
	parsedURL, err := url.Parse(d.DBConnString)
	if err != nil {
		return 0, fmt.Errorf("failed to parse weaviate URL: %w", err)
	}

	cfg := weaviateSDK.Config{
		Host:   parsedURL.Host,
		Scheme: parsedURL.Scheme,
	}
	if d.DBConnAPIKey != "" {
		cfg.AuthConfig = auth.ApiKey{Value: d.DBConnAPIKey}
	}

	client, err := weaviateSDK.NewClient(cfg)
	if err != nil {
		return 0, fmt.Errorf("failed to create weaviate client: %w", err)
	}

	// Build where filter
	whereFilter, err := ds.buildWeaviateWhereFilter(filter, filterMode)
	if err != nil {
		return 0, fmt.Errorf("failed to build where filter: %w", err)
	}

	if dryRun {
		// Count matching objects using aggregate query
		result, err := client.GraphQL().Aggregate().
			WithClassName(className).
			WithWhere(whereFilter).
			WithFields(graphql.Field{Name: "meta", Fields: []graphql.Field{{Name: "count"}}}).
			Do(ctx)

		if err != nil {
			return 0, fmt.Errorf("failed to count matching objects: %w", err)
		}

		count := ds.extractWeaviateCount(result, className)
		return count, nil
	}

	// Get count first
	countResult, err := client.GraphQL().Aggregate().
		WithClassName(className).
		WithWhere(whereFilter).
		WithFields(graphql.Field{Name: "meta", Fields: []graphql.Field{{Name: "count"}}}).
		Do(ctx)

	if err != nil {
		return 0, fmt.Errorf("failed to count objects before deletion: %w", err)
	}

	count := ds.extractWeaviateCount(countResult, className)

	// Delete objects using batch delete with where filter
	result, err := client.Batch().ObjectsBatchDeleter().
		WithClassName(className).
		WithWhere(whereFilter).
		Do(ctx)

	if err != nil {
		return 0, fmt.Errorf("failed to delete weaviate objects: %w", err)
	}

	// Check result for actual deleted count
	if result != nil && result.Results != nil {
		count = int(result.Results.Successful)
	}

	slog.Info("Deleted objects from Weaviate by metadata", "count", count, "class", className)
	return count, nil
}

func (ds *DataSession) queryWeaviateByMetadata(ctx context.Context, d *models.Datasource, filter map[string]string, filterMode string, limit, offset int) ([]schema.Document, int, error) {
	// Parse namespace format
	split := strings.Split(d.DBName, ":")
	if len(split) != 2 {
		return nil, 0, fmt.Errorf("invalid weaviate namespace format, expected ClassName:Namespace")
	}
	className := split[0]

	// Create client
	parsedURL, err := url.Parse(d.DBConnString)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to parse weaviate URL: %w", err)
	}

	cfg := weaviateSDK.Config{
		Host:   parsedURL.Host,
		Scheme: parsedURL.Scheme,
	}
	if d.DBConnAPIKey != "" {
		cfg.AuthConfig = auth.ApiKey{Value: d.DBConnAPIKey}
	}

	client, err := weaviateSDK.NewClient(cfg)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create weaviate client: %w", err)
	}

	// Build where filter
	whereFilter, err := ds.buildWeaviateWhereFilter(filter, filterMode)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to build where filter: %w", err)
	}

	// Get total count
	countResult, err := client.GraphQL().Aggregate().
		WithClassName(className).
		WithWhere(whereFilter).
		WithFields(graphql.Field{Name: "meta", Fields: []graphql.Field{{Name: "count"}}}).
		Do(ctx)

	if err != nil {
		return nil, 0, fmt.Errorf("failed to count objects: %w", err)
	}

	totalCount := ds.extractWeaviateCount(countResult, className)

	// Query objects with pagination
	fields := []graphql.Field{
		{Name: "content"},
		{Name: "_additional", Fields: []graphql.Field{{Name: "id"}}},
	}

	// Add all filter keys as fields to retrieve
	for key := range filter {
		fields = append(fields, graphql.Field{Name: key})
	}

	result, err := client.GraphQL().Get().
		WithClassName(className).
		WithWhere(whereFilter).
		WithLimit(limit).
		WithOffset(offset).
		WithFields(fields...).
		Do(ctx)

	if err != nil {
		return nil, 0, fmt.Errorf("failed to query weaviate objects: %w", err)
	}

	// Parse results
	docs := make([]schema.Document, 0)
	if result != nil && result.Data != nil {
		if getMap, ok := result.Data["Get"].(map[string]interface{}); ok {
			if classResults, ok := getMap[className].([]interface{}); ok {
				for _, item := range classResults {
					if itemMap, ok := item.(map[string]interface{}); ok {
						content := ""
						if contentVal, ok := itemMap["content"]; ok {
							if contentStr, ok := contentVal.(string); ok {
								content = contentStr
							}
						}

						// Extract metadata (all fields except content and _additional)
						metadata := make(map[string]any)
						for k, v := range itemMap {
							if k != "content" && k != "_additional" {
								metadata[k] = v
							}
						}

						// Check for base64 encoding
						if enc, ok := metadata["encoding"]; ok {
							if encStr, ok := enc.(string); ok && encStr == "base64" {
								decodedContent, err := base64.StdEncoding.DecodeString(content)
								if err != nil {
									slog.Error("error decoding base64 content", "err", err)
								} else {
									content = string(decodedContent)
								}
							}
						}

						docs = append(docs, schema.Document{
							PageContent: content,
							Metadata:    metadata,
							Score:       0,
						})
					}
				}
			}
		}
	}

	return docs, totalCount, nil
}

func (ds *DataSession) listWeaviateClasses(ctx context.Context, d *models.Datasource) ([]NamespaceInfo, error) {
	// Create client
	parsedURL, err := url.Parse(d.DBConnString)
	if err != nil {
		return nil, fmt.Errorf("failed to parse weaviate URL: %w", err)
	}

	cfg := weaviateSDK.Config{
		Host:   parsedURL.Host,
		Scheme: parsedURL.Scheme,
	}
	if d.DBConnAPIKey != "" {
		cfg.AuthConfig = auth.ApiKey{Value: d.DBConnAPIKey}
	}

	client, err := weaviateSDK.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create weaviate client: %w", err)
	}

	// Get schema to list all classes
	schema, err := client.Schema().Getter().Do(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get weaviate schema: %w", err)
	}

	// Convert to NamespaceInfo
	namespaces := make([]NamespaceInfo, 0)
	if schema != nil && schema.Classes != nil {
		for _, class := range schema.Classes {
			// Get object count for each class using aggregate
			countResult, err := client.GraphQL().Aggregate().
				WithClassName(class.Class).
				WithFields(graphql.Field{Name: "meta", Fields: []graphql.Field{{Name: "count"}}}).
				Do(ctx)

			count := -1
			if err == nil {
				count = ds.extractWeaviateCount(countResult, class.Class)
			} else {
				slog.Warn("Failed to count objects in class", "class", class.Class, "error", err)
			}

			namespaces = append(namespaces, NamespaceInfo{
				Name:          class.Class,
				DocumentCount: count,
			})
		}
	}

	return namespaces, nil
}

func (ds *DataSession) deleteWeaviateClass(ctx context.Context, d *models.Datasource, namespace string) error {
	// Create client
	parsedURL, err := url.Parse(d.DBConnString)
	if err != nil {
		return fmt.Errorf("failed to parse weaviate URL: %w", err)
	}

	cfg := weaviateSDK.Config{
		Host:   parsedURL.Host,
		Scheme: parsedURL.Scheme,
	}
	if d.DBConnAPIKey != "" {
		cfg.AuthConfig = auth.ApiKey{Value: d.DBConnAPIKey}
	}

	client, err := weaviateSDK.NewClient(cfg)
	if err != nil {
		return fmt.Errorf("failed to create weaviate client: %w", err)
	}

	// Delete class (schema)
	err = client.Schema().ClassDeleter().WithClassName(namespace).Do(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete weaviate class '%s': %w", namespace, err)
	}

	slog.Warn("Deleted Weaviate class", "class", namespace)
	return nil
}

// buildWeaviateWhereFilter builds a Weaviate where filter from a metadata map
func (ds *DataSession) buildWeaviateWhereFilter(filter map[string]string, filterMode string) (*filters.WhereBuilder, error) {
	if len(filter) == 0 {
		return nil, fmt.Errorf("filter cannot be empty")
	}

	// Build individual where clauses
	var whereFilters []*filters.WhereBuilder
	for key, value := range filter {
		whereFilters = append(whereFilters, filters.Where().
			WithPath([]string{key}).
			WithOperator(filters.Equal).
			WithValueString(value))
	}

	// Combine with AND or OR
	if len(whereFilters) == 1 {
		return whereFilters[0], nil
	}

	if filterMode == "OR" {
		// Combine with OR operator
		combined := whereFilters[0]
		for i := 1; i < len(whereFilters); i++ {
			combined = combined.WithOperator(filters.Or).WithOperands([]*filters.WhereBuilder{whereFilters[i]})
		}
		return combined, nil
	}

	// Combine with AND operator
	combined := whereFilters[0]
	for i := 1; i < len(whereFilters); i++ {
		combined = combined.WithOperator(filters.And).WithOperands([]*filters.WhereBuilder{whereFilters[i]})
	}
	return combined, nil
}

// extractWeaviateCount extracts count from Weaviate aggregate result
func (ds *DataSession) extractWeaviateCount(result *weaviateModels.GraphQLResponse, className string) int {
	if result == nil || result.Data == nil {
		return 0
	}

	if aggMap, ok := result.Data["Aggregate"].(map[string]interface{}); ok {
		if classResults, ok := aggMap[className].([]interface{}); ok && len(classResults) > 0 {
			if firstResult, ok := classResults[0].(map[string]interface{}); ok {
				if meta, ok := firstResult["meta"].(map[string]interface{}); ok {
					if count, ok := meta["count"].(float64); ok {
						return int(count)
					}
					if count, ok := meta["count"].(int); ok {
						return count
					}
				}
			}
		}
	}

	return 0
}

// Qdrant implementations
func (ds *DataSession) listQdrantCollections(ctx context.Context, d *models.Datasource) ([]NamespaceInfo, error) {
	return nil, fmt.Errorf("qdrant list collections: not yet implemented")
}

func (ds *DataSession) deleteQdrantCollection(ctx context.Context, d *models.Datasource, namespace string) error {
	return fmt.Errorf("qdrant delete collection: not yet implemented")
}
