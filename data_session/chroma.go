//go:build cgo

// Chroma support. chroma-go's v2 client imports its default embedding
// function, which loads a tokenizer and the ONNX runtime through cgo, so this
// file builds only with cgo. chroma_nocgo.go stands in for it otherwise, and
// a CGO_ENABLED=0 build (such as a host embedding pkg/studio) has no Chroma.

package data_session

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"

	"github.com/TykTechnologies/midsommar/v2/third_party/langchaingo/embeddings"
	"github.com/TykTechnologies/midsommar/v2/third_party/langchaingo/schema"
	"github.com/TykTechnologies/midsommar/v2/third_party/langchaingo/vectorstores"
	"github.com/TykTechnologies/midsommar/v2/third_party/langchaingo/vectorstores/chroma"
	chromago "github.com/amikos-tech/chroma-go/pkg/api/v2"
	chromaEmbeddings "github.com/amikos-tech/chroma-go/pkg/embeddings"
	"github.com/google/uuid"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// ChromaSupported reports whether this build can use Chroma, which needs cgo.
const ChromaSupported = true

func newChromaStore(d *models.Datasource, embedder *embeddings.EmbedderImpl) (vectorstores.VectorStore, error) {
	return chroma.New(
		chroma.WithChromaURL(d.DBConnString),
		chroma.WithEmbedder(embedder),
		chroma.WithNameSpace(d.DBName),
	)
}

// errPrecomputedEmbeddings is what precomputedEmbeddings returns if chroma-go
// ever asks it to embed text.
var errPrecomputedEmbeddings = errors.New("chroma: Studio supplies precomputed vectors; this collection handle cannot embed text")

// precomputedEmbeddings is the embedding function Studio hands chroma-go for
// every collection it opens directly. Studio computes vectors itself (or
// receives them) and always passes them in, so chroma-go never needs to embed
// anything; this function refuses rather than producing vectors of its own.
//
// It must be passed explicitly: when GetCollection has no embedding function
// it fails validation, and CreateCollection then builds chroma-go's default
// one, which downloads an ONNX runtime whose API version the linked
// onnxruntime_go may not accept (v1.26 asks for API 24; chroma-go v0.2.5
// fetches ORT 1.21), failing every store and search.
type precomputedEmbeddings struct{}

func (precomputedEmbeddings) EmbedDocuments(context.Context, []string) ([]chromaEmbeddings.Embedding, error) {
	return nil, errPrecomputedEmbeddings
}

func (precomputedEmbeddings) EmbedQuery(context.Context, string) (chromaEmbeddings.Embedding, error) {
	return nil, errPrecomputedEmbeddings
}

// openChromaCollection gets the named collection, creating it (L2 space) if
// it does not exist, always with precomputedEmbeddings.
func openChromaCollection(ctx context.Context, client chromago.Client, d *models.Datasource, purpose string) (chromago.Collection, error) {
	ef := precomputedEmbeddings{}
	collection, err := client.GetCollection(ctx, d.DBName, chromago.WithEmbeddingFunctionGet(ef))
	if err == nil {
		return collection, nil
	}
	slog.Info("Chroma collection not found, creating new one", "collection", d.DBName, "purpose", purpose, "error", err.Error())
	collection, err = client.CreateCollection(ctx, d.DBName,
		chromago.WithEmbeddingFunctionCreate(ef),
		chromago.WithHNSWSpaceCreate(chromaEmbeddings.L2),
		chromago.WithIfNotExistsCreate(),
	)
	if err != nil {
		return nil, err
	}
	slog.Info("Created new Chroma collection", "collection", d.DBName, "purpose", purpose)
	return collection, nil
}

// chromaMetadataValue unwraps a Chroma v2 MetadataValue (or a pointer to
// one) into a plain Go value. ok is false for any other type.
func chromaMetadataValue(v any) (any, bool) {
	var mv *chromago.MetadataValue
	switch t := v.(type) {
	case chromago.MetadataValue:
		mv = &t
	case *chromago.MetadataValue:
		mv = t
	default:
		return nil, false
	}
	if rawVal, ok := mv.GetRaw(); ok {
		return rawVal, true
	}
	return mv.String(), true
}

func (ds *DataSession) storeToChroma(ctx context.Context, store vectorstores.VectorStore, d *models.Datasource, contents []string, vectors [][]float32, metadatas []map[string]any) error {
	// Create Chroma v2 client
	client, err := chromago.NewHTTPClient(chromago.WithBaseURL(d.DBConnString))
	if err != nil {
		return fmt.Errorf("failed to create chroma client: %w", err)
	}

	// Get or create collection (we're providing pre-computed embeddings)
	collection, err := openChromaCollection(ctx, client, d, "store")
	if err != nil {
		return fmt.Errorf("failed to get/create chroma collection '%s' at %s: %w", d.DBName, d.DBConnString, err)
	}

	// Add documents with embeddings using v2 API
	// The v2 API uses functional options and adds documents one at a time or in batch
	for i := range contents {
		docID := chromago.DocumentID(uuid.New().String())

		// Convert float32 to Embedding
		emb := chromaEmbeddings.NewEmbeddingFromFloat32(vectors[i])

		// Prepare metadata
		var chromaMetadata chromago.DocumentMetadata
		if len(metadatas) > i {
			chromaMetadata, err = chromago.NewDocumentMetadataFromMap(metadatas[i])
			if err != nil {
				return fmt.Errorf("failed to create metadata for document %d: %w", i, err)
			}
		} else {
			chromaMetadata = chromago.NewDocumentMetadata()
		}

		// Add document with pre-computed embedding
		err = collection.Add(ctx,
			chromago.WithIDs(docID),
			chromago.WithTexts(contents[i]),
			chromago.WithEmbeddings(emb),
			chromago.WithMetadatas(chromaMetadata),
		)
		if err != nil {
			return fmt.Errorf("failed to add document %d to chroma: %w", i, err)
		}
	}

	slog.Info("Successfully stored vectors in Chroma", "count", len(vectors), "collection", d.DBName)
	return nil
}

func (ds *DataSession) searchChromaByVector(ctx context.Context, store vectorstores.VectorStore, d *models.Datasource, vector []float32, topK int) ([]schema.Document, error) {
	// Create Chroma v2 client
	client, err := chromago.NewHTTPClient(chromago.WithBaseURL(d.DBConnString))
	if err != nil {
		return nil, fmt.Errorf("failed to create chroma client: %w", err)
	}

	// Get collection - if it doesn't exist, create it (L2 distance metric)
	collection, err := openChromaCollection(ctx, client, d, "query")
	if err != nil {
		return nil, fmt.Errorf("failed to get/create chroma collection '%s' for query: %w", d.DBName, err)
	}

	// Convert to Chroma embedding
	emb := chromaEmbeddings.NewEmbeddingFromFloat32(vector)

	// Query with embedding using v2 API
	// Make sure to include documents in the results
	queryResult, err := collection.Query(ctx,
		chromago.WithQueryEmbeddings(emb),
		chromago.WithNResults(topK),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query chroma: %w", err)
	}

	// Convert v2 API results
	docs := make([]schema.Document, 0)

	documentsGroups := queryResult.GetDocumentsGroups()
	metadatasGroups := queryResult.GetMetadatasGroups()
	distancesGroups := queryResult.GetDistancesGroups()

	slog.Info("Chroma query results",
		"num_groups", len(documentsGroups),
		"total_results", queryResult.CountGroups())

	// Iterate through result groups
	for groupIdx := range documentsGroups {
		documents := documentsGroups[groupIdx]
		slog.Info("Processing result group", "group_idx", groupIdx, "num_docs", len(documents))

		for docIdx, doc := range documents {
			metadata := make(map[string]any)

			// Extract metadata if available
			if len(metadatasGroups) > groupIdx && len(metadatasGroups[groupIdx]) > docIdx {
				chromaMeta := metadatasGroups[groupIdx][docIdx]

				slog.Info("ChromaDB metadata extraction",
					"groupIdx", groupIdx,
					"docIdx", docIdx,
					"metadata_available", chromaMeta != nil)

				// Extract all standard and custom metadata fields
				// AI Studio standard fields
				if val, ok := chromaMeta.GetString("filename"); ok {
					metadata["filename"] = val
					slog.Info("Extracted field", "key", "filename", "value", val)
				}
				if val, ok := chromaMeta.GetString("file_name"); ok {
					metadata["file_name"] = val
					slog.Info("Extracted field", "key", "file_name", "value", val)
				}
				if val, ok := chromaMeta.GetString("title"); ok {
					metadata["title"] = val
				}
				if val, ok := chromaMeta.GetString("text"); ok {
					metadata["text"] = val
				}
				if val, ok := chromaMeta.GetString("start"); ok {
					metadata["start"] = val
				}
				if val, ok := chromaMeta.GetString("end"); ok {
					metadata["end"] = val
				}

				// GitHub RAG plugin fields
				if val, ok := chromaMeta.GetString("source"); ok {
					metadata["source"] = val
				}
				if val, ok := chromaMeta.GetString("repo_id"); ok {
					metadata["repo_id"] = val
				}
				if val, ok := chromaMeta.GetString("repo_name"); ok {
					metadata["repo_name"] = val
				}
				if val, ok := chromaMeta.GetString("repo_owner"); ok {
					metadata["repo_owner"] = val
				}
				if val, ok := chromaMeta.GetString("repo_host"); ok {
					metadata["repo_host"] = val
				}
				if val, ok := chromaMeta.GetString("branch"); ok {
					metadata["branch"] = val
				}
				if val, ok := chromaMeta.GetString("commit_sha"); ok {
					metadata["commit_sha"] = val
				}
				if val, ok := chromaMeta.GetString("file_path"); ok {
					metadata["file_path"] = val
				}
				if val, ok := chromaMeta.GetString("file_type"); ok {
					metadata["file_type"] = val
				}
				if val, ok := chromaMeta.GetString("chunk_index"); ok {
					metadata["chunk_index"] = val
				}
				if val, ok := chromaMeta.GetString("total_chunks"); ok {
					metadata["total_chunks"] = val
				}
				if val, ok := chromaMeta.GetString("line_start"); ok {
					metadata["line_start"] = val
				}
				if val, ok := chromaMeta.GetString("line_end"); ok {
					metadata["line_end"] = val
				}
				if val, ok := chromaMeta.GetString("github_url"); ok {
					metadata["github_url"] = val
				}
				if val, ok := chromaMeta.GetString("ingestion_timestamp"); ok {
					metadata["ingestion_timestamp"] = val
				}
				if val, ok := chromaMeta.GetString("namespace"); ok {
					metadata["namespace"] = val
				}

				// Other common fields
				if val, ok := chromaMeta.GetString("encoding"); ok {
					metadata["encoding"] = val
				}
				if val, ok := chromaMeta.GetString("test_type"); ok {
					metadata["test_type"] = val
				}
			}

			score := float32(0)
			if len(distancesGroups) > groupIdx && len(distancesGroups[groupIdx]) > docIdx {
				score = float32(distancesGroups[groupIdx][docIdx])
			}

			// v2 Document interface has ContentString() method
			content := doc.ContentString()

			// Log final metadata before creating document
			slog.Info("Final metadata for document",
				"docIdx", docIdx,
				"metadata_keys", len(metadata),
				"metadata", metadata)

			docs = append(docs, schema.Document{
				PageContent: content,
				Metadata:    metadata,
				Score:       score,
			})
		}
	}

	return docs, nil
}

func (ds *DataSession) deleteChromaByMetadata(ctx context.Context, d *models.Datasource, filter map[string]string, filterMode string, dryRun bool) (int, error) {
	// Create Chroma v2 client
	client, err := chromago.NewHTTPClient(chromago.WithBaseURL(d.DBConnString))
	if err != nil {
		return 0, fmt.Errorf("failed to create chroma client: %w", err)
	}

	// Get collection with no-op embedder (we're not doing vector operations)
	collection, err := client.GetCollection(ctx, d.DBName, chromago.WithEmbeddingFunctionGet(precomputedEmbeddings{}))
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
	collection, err := client.GetCollection(ctx, d.DBName, chromago.WithEmbeddingFunctionGet(precomputedEmbeddings{}))
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
