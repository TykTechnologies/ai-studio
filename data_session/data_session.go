package data_session

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/go-openapi/strfmt"
	"github.com/google/uuid"
	pgvectorDriver "github.com/pgvector/pgvector-go"
	pineconeSDK "github.com/pinecone-io/go-pinecone/pinecone"
	redis "github.com/redis/go-redis/v9"
	weaviateSDK "github.com/weaviate/weaviate-go-client/v5/weaviate"
	"github.com/weaviate/weaviate-go-client/v5/weaviate/auth"
	"github.com/weaviate/weaviate-go-client/v5/weaviate/graphql"
	weaviateModels "github.com/weaviate/weaviate/entities/models"
	"gorm.io/driver/postgres"

	chromago "github.com/amikos-tech/chroma-go/pkg/api/v2"
	chromaEmbeddings "github.com/amikos-tech/chroma-go/pkg/embeddings"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/switches"
	"github.com/tmc/langchaingo/embeddings"
	"github.com/tmc/langchaingo/schema"
	"github.com/tmc/langchaingo/textsplitter"
	"github.com/tmc/langchaingo/vectorstores"
	"github.com/tmc/langchaingo/vectorstores/chroma"
	"github.com/tmc/langchaingo/vectorstores/pgvector"
	"github.com/tmc/langchaingo/vectorstores/pinecone"
	"github.com/tmc/langchaingo/vectorstores/qdrant"
	"github.com/tmc/langchaingo/vectorstores/redisvector"
	"github.com/tmc/langchaingo/vectorstores/weaviate"
	"gorm.io/gorm"
)

type VectorStoreVendor string

const (
	VECTOR_PINECONE = "pinecone"
	VECTOR_CHROMA   = "chroma"
	VECTOR_PGVECTOR = "pgvector"
	VECTOR_REDIS    = "redis"
	VECTOR_QDRANT   = "qdrant"
	VECTOR_WEAVIATE = "weaviate"
)

var AVAILABLE_VECTOR_STORES = []VectorStoreVendor{
	VECTOR_CHROMA,
	VECTOR_PGVECTOR,
	VECTOR_PINECONE,
	VECTOR_REDIS,
	VECTOR_QDRANT,
	VECTOR_WEAVIATE,
}

type DataSession struct {
	Sources map[uint]*models.Datasource
}

func NewDataSession(sources map[uint]*models.Datasource) *DataSession {
	return &DataSession{
		Sources: sources,
	}
}

func (ds *DataSession) Search(query string, n int) ([]schema.Document, error) {
	var results = make([]schema.Document, 0)
	for _, d := range ds.Sources {
		embedder, err := ds.getEmbedder(d)
		if err != nil {
			return nil, err
		}

		store, err := ds.getStore(d, embedder)
		if err != nil {
			return nil, err
		}

		ctx, done := context.WithTimeout(context.Background(), 10*time.Second)
		defer done()

		docs, err := store.SimilaritySearch(ctx, query, n)
		if err != nil {
			return nil, err
		}

		for i := range docs {
			enc, ok := docs[i].Metadata["encoding"]
			if ok {
				if enc == "base64" {
					// base64 decode content
					decodedContent, err := base64.StdEncoding.DecodeString(docs[i].PageContent)
					if err != nil {
						slog.Error("error decoding base64 content", "err", err)
						continue
					}
					docs[i].PageContent = string(decodedContent)
				}
			}
		}

		results = append(results, docs...)
	}

	return results, nil
}

// SearchByVector performs similarity search using a pre-computed embedding vector
// This allows querying with custom embeddings without re-embedding the query
func (ds *DataSession) SearchByVector(dsID uint, vector []float32, n int) ([]schema.Document, error) {
	d, ok := ds.Sources[dsID]
	if !ok {
		return nil, fmt.Errorf("datasource with id %d not found", dsID)
	}

	embedder, err := ds.getEmbedder(d)
	if err != nil {
		return nil, err
	}

	store, err := ds.getStore(d, embedder)
	if err != nil {
		return nil, err
	}

	ctx, done := context.WithTimeout(context.Background(), 10*time.Second)
	defer done()

	// Use vendor-specific similarity search with pre-computed vector
	var docs []schema.Document
	switch d.DBSourceType {
	case VECTOR_PINECONE:
		docs, err = ds.searchPineconeByVector(ctx, d, vector, n)
	case VECTOR_QDRANT:
		docs, err = ds.searchQdrantByVector(ctx, d, vector, n)
	case VECTOR_PGVECTOR:
		docs, err = ds.searchPGVectorByVector(ctx, d, vector, n)
	case VECTOR_CHROMA:
		docs, err = ds.searchChromaByVector(ctx, store, d, vector, n)
	case VECTOR_REDIS:
		docs, err = ds.searchRedisByVector(ctx, d, vector, n)
	case VECTOR_WEAVIATE:
		docs, err = ds.searchWeaviateByVector(ctx, d, vector, n)
	default:
		return nil, fmt.Errorf("vector search not implemented for %s", d.DBSourceType)
	}

	if err != nil {
		return nil, err
	}

	// Decode base64 content if needed
	for i := range docs {
		enc, ok := docs[i].Metadata["encoding"]
		if ok && enc == "base64" {
			decodedContent, err := base64.StdEncoding.DecodeString(docs[i].PageContent)
			if err != nil {
				slog.Error("error decoding base64 content", "err", err)
				continue
			}
			docs[i].PageContent = string(decodedContent)
		}
	}

	return docs, nil
}

func (ds *DataSession) ProcessRAGForDatasource(withDSID uint, db *gorm.DB) error {
	splitter := textsplitter.NewRecursiveCharacter(
		textsplitter.WithSeparators([]string{"\n\n", "\n", " ", ""}),
		textsplitter.WithChunkSize(2048),
	)

	dataSource, ok := ds.Sources[withDSID]
	if !ok {
		return fmt.Errorf("datasource with id %d not found", withDSID)
	}

	files := dataSource.Files

	texts := make([]string, 0)
	metas := make([]map[string]any, 0)

	for i, _ := range files {
		f := files[i]
		meta := map[string]any{
			"filename": f.FileName,
			"encoding": "base64", // for some reason it base64 encodes the pagecontent
		}

		texts = append(texts, f.Content)
		metas = append(metas, meta)

		f.LastProcessedOn = time.Now()
		err := f.Update(db)
		if err != nil {
			return err
		}
	}

	// fmt.Println("processing RAG for datasource with id", withDSID)
	asDocs, err := textsplitter.CreateDocuments(
		splitter,
		texts,
		metas)

	if err != nil {
		return err
	}

	slog.Info("creating embedding for datasource", "datasource_id", withDSID)
	err = ds.StoreEmbedding(withDSID, asDocs)
	if err != nil {
		return err
	}

	return nil
}

func (ds *DataSession) CreateEmbedding(dsID uint, texts []string) ([][]float32, error) {
	if len(ds.Sources) > 0 {
		d, ok := ds.Sources[dsID]
		if !ok {
			return nil, fmt.Errorf("datasource with id %d not found", dsID)
		}

		embedder, err := ds.getEmbedder(d)
		if err != nil {
			return nil, err
		}

		embedding, err := embedder.EmbedDocuments(context.Background(), texts)
		if err != nil {
			return nil, err
		}

		return embedding, nil
	}

	return nil, fmt.Errorf("no datasources found")
}

func (ds *DataSession) StoreEmbedding(dsID uint, docs []schema.Document) error {
	if len(ds.Sources) > 0 {
		d, ok := ds.Sources[dsID]
		if !ok {
			return fmt.Errorf("datasource with id %d not found", dsID)
		}

		embedder, err := ds.getEmbedder(d)
		if err != nil {
			return err
		}

		store, err := ds.getStore(d, embedder)
		if err != nil {
			return err
		}

		_, err = store.AddDocuments(context.Background(), docs, vectorstores.WithNameSpace(d.DBName))
		if err != nil {
			return fmt.Errorf("add documents failed: %v", err)
		}

		return nil
	}

	return fmt.Errorf("no datasources found")
}

func (ds *DataSession) getEmbedder(d *models.Datasource) (*embeddings.EmbedderImpl, error) {
	e, err := switches.GetEmbedder(d)
	return e, err
}

func (ds *DataSession) getStore(d *models.Datasource, embedder *embeddings.EmbedderImpl) (vectorstores.VectorStore, error) {
	var store vectorstores.VectorStore
	var err error

	switch d.DBSourceType {
	case VECTOR_PINECONE:
		store, err = pinecone.New(
			pinecone.WithHost(d.DBConnString),
			pinecone.WithEmbedder(embedder),
			pinecone.WithAPIKey(d.DBConnAPIKey),
			pinecone.WithNameSpace(d.DBName))

	case VECTOR_PGVECTOR:
		store, err = pgvector.New(
			context.Background(),
			pgvector.WithConnectionURL(d.DBConnString),
			pgvector.WithEmbedder(embedder),
			pgvector.WithCollectionName(d.DBName),
		)

	case VECTOR_CHROMA:
		store, err = chroma.New(
			chroma.WithChromaURL(d.DBConnString),
			chroma.WithEmbedder(embedder),
			chroma.WithNameSpace(d.DBName),
		)

	case VECTOR_REDIS:
		store, err = redisvector.New(context.Background(),
			redisvector.WithConnectionURL(d.DBConnString),
			redisvector.WithEmbedder(embedder),
			redisvector.WithIndexName(d.DBName, false),
		)

	case VECTOR_QDRANT:
		url, err := url.Parse(d.DBConnString)
		if err != nil {
			return nil, err
		}

		store, err = qdrant.New(
			qdrant.WithAPIKey(d.DBConnAPIKey),
			qdrant.WithCollectionName(d.DBName),
			qdrant.WithEmbedder(embedder),
			qdrant.WithURL(*url),
		)

	case VECTOR_WEAVIATE:
		url, err := url.Parse(d.DBConnString)
		if err != nil {
			return nil, err
		}

		split := strings.Split(d.DBName, ":")
		if len(split) != 2 {
			return nil, fmt.Errorf("namespace must be in the form of indexName:namespace")
		}

		indexName := split[0]
		namespace := split[1]

		store, err = weaviate.New(
			weaviate.WithHost(url.Host),
			weaviate.WithAPIKey(d.DBConnAPIKey),
			weaviate.WithEmbedder(embedder),
			weaviate.WithNameSpace(namespace),
			weaviate.WithScheme(url.Scheme),
			weaviate.WithIndexName(indexName),
		)

	default:
		return nil, fmt.Errorf("unsupported store type")
	}

	if err != nil {
		return nil, err
	}

	return store, nil
}

// StoreDocumentsWithVectors stores documents with pre-computed embedding vectors
// This bypasses the embedder and uses vendor-specific APIs to store vectors directly
func (ds *DataSession) StoreDocumentsWithVectors(dsID uint, contents []string, vectors [][]float32, metadatas []map[string]any) error {
	if len(contents) != len(vectors) {
		return fmt.Errorf("contents and vectors length mismatch: %d vs %d", len(contents), len(vectors))
	}

	if len(ds.Sources) == 0 {
		return fmt.Errorf("no datasources found")
	}

	d, ok := ds.Sources[dsID]
	if !ok {
		return fmt.Errorf("datasource with id %d not found", dsID)
	}

	// Get embedder (needed for vector store initialization, even though we won't use it for embedding)
	embedder, err := ds.getEmbedder(d)
	if err != nil {
		return err
	}

	// Get the vector store
	store, err := ds.getStore(d, embedder)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Use vendor-specific logic to store pre-computed vectors
	switch d.DBSourceType {
	case VECTOR_PINECONE:
		return ds.storeToPinecone(ctx, store, d, contents, vectors, metadatas)
	case VECTOR_QDRANT:
		return ds.storeToQdrant(ctx, store, d, contents, vectors, metadatas)
	case VECTOR_PGVECTOR:
		return ds.storeToPGVector(ctx, store, d, contents, vectors, metadatas)
	case VECTOR_CHROMA:
		return ds.storeToChroma(ctx, store, d, contents, vectors, metadatas)
	case VECTOR_REDIS:
		return ds.storeToRedis(ctx, store, d, contents, vectors, metadatas)
	case VECTOR_WEAVIATE:
		return ds.storeToWeaviate(ctx, store, d, contents, vectors, metadatas)
	default:
		return fmt.Errorf("pre-computed vector storage not implemented for %s - please use ProcessAndStoreDocuments instead", d.DBSourceType)
	}
}

// Helper methods for vendor-specific vector storage
// These use the underlying vector store's direct upsert methods

func (ds *DataSession) storeToPinecone(ctx context.Context, store vectorstores.VectorStore, d *models.Datasource, contents []string, vectors [][]float32, metadatas []map[string]any) error {
	// Create Pinecone client directly
	pc, err := pineconeSDK.NewClient(pineconeSDK.NewClientParams{
		ApiKey: d.DBConnAPIKey,
	})
	if err != nil {
		return fmt.Errorf("failed to create pinecone client: %w", err)
	}

	// Parse host to get index name
	// Expected format: https://index-name-abc123.svc.pinecone.io
	parsedURL, err := url.Parse(d.DBConnString)
	if err != nil {
		return fmt.Errorf("failed to parse pinecone host: %w", err)
	}

	// Extract index name from subdomain
	hostParts := strings.Split(parsedURL.Host, ".")
	if len(hostParts) < 1 {
		return fmt.Errorf("invalid pinecone host format")
	}
	indexName := hostParts[0]

	// Get index connection
	idx, err := pc.Index(pineconeSDK.NewIndexConnParams{Host: d.DBConnString})
	if err != nil {
		return fmt.Errorf("failed to connect to pinecone index: %w", err)
	}

	// Prepare vectors for upsert
	pineconeVectors := make([]*pineconeSDK.Vector, len(contents))
	for i := range contents {
		vectorID := fmt.Sprintf("%s-%d-%s", indexName, i, uuid.New().String())

		// Convert metadata to map for JSON marshaling
		metadataMap := make(map[string]interface{})
		if len(metadatas) > i {
			for k, v := range metadatas[i] {
				metadataMap[k] = v
			}
		}
		metadataMap["content"] = contents[i] // Store content in metadata

		// Marshal to JSON and back to create structpb-compatible metadata
		metadataJSON, err := json.Marshal(metadataMap)
		if err != nil {
			return fmt.Errorf("failed to marshal metadata: %w", err)
		}

		var metadata pineconeSDK.Metadata
		err = json.Unmarshal(metadataJSON, &metadata)
		if err != nil {
			return fmt.Errorf("failed to unmarshal metadata: %w", err)
		}

		pineconeVectors[i] = &pineconeSDK.Vector{
			Id:       vectorID,
			Values:   vectors[i],
			Metadata: &metadata,
		}
	}

	// Upsert vectors
	namespace := d.DBName
	_, err = idx.UpsertVectors(ctx, pineconeVectors)
	if err != nil {
		return fmt.Errorf("failed to upsert vectors to pinecone (namespace: %s): %w", namespace, err)
	}

	slog.Info("Successfully stored vectors in Pinecone", "count", len(vectors), "namespace", namespace)
	return nil
}

func (ds *DataSession) storeToQdrant(ctx context.Context, store vectorstores.VectorStore, d *models.Datasource, contents []string, vectors [][]float32, metadatas []map[string]any) error {
	// Qdrant SDK not available in go.mod - would need to add github.com/qdrant/go-client
	slog.Warn("Qdrant SDK not available - add github.com/qdrant/go-client to go.mod to enable pre-computed storage")
	return fmt.Errorf("qdrant pre-computed storage requires SDK installation - use ProcessAndStoreDocuments")
}

func (ds *DataSession) storeToPGVector(ctx context.Context, store vectorstores.VectorStore, d *models.Datasource, contents []string, vectors [][]float32, metadatas []map[string]any) error {
	// Get the underlying database connection from the pgvector store
	// Since langchaingo doesn't expose it, we need to create our own connection
	db, err := gorm.Open(postgres.Open(d.DBConnString), &gorm.Config{})
	if err != nil {
		return fmt.Errorf("failed to connect to postgres: %w", err)
	}

	// Table name is the collection name
	tableName := d.DBName

	// Insert vectors directly using SQL
	for i := range contents {
		vectorID := uuid.New().String()

		// Convert metadata to JSON
		metadataJSON, err := json.Marshal(metadatas[i])
		if err != nil {
			return fmt.Errorf("failed to marshal metadata: %w", err)
		}

		// Create pgvector.Vector from float32 slice
		pgVector := pgvectorDriver.NewVector(vectors[i])

		// Insert into the table
		// Assuming table structure: id, content, embedding, metadata
		query := fmt.Sprintf(`
			INSERT INTO %s (id, content, embedding, metadata)
			VALUES (?, ?, ?, ?)
			ON CONFLICT (id) DO UPDATE SET
				content = EXCLUDED.content,
				embedding = EXCLUDED.embedding,
				metadata = EXCLUDED.metadata
		`, tableName)

		err = db.Exec(query, vectorID, contents[i], pgVector, string(metadataJSON)).Error
		if err != nil {
			return fmt.Errorf("failed to insert vector into pgvector: %w", err)
		}
	}

	slog.Info("Successfully stored vectors in PGVector", "count", len(vectors), "table", tableName)
	return nil
}

func (ds *DataSession) storeToChroma(ctx context.Context, store vectorstores.VectorStore, d *models.Datasource, contents []string, vectors [][]float32, metadatas []map[string]any) error {
	// Create Chroma v2 client
	client, err := chromago.NewHTTPClient(chromago.WithBaseURL(d.DBConnString))
	if err != nil {
		return fmt.Errorf("failed to create chroma client: %w", err)
	}

	// Get or create collection
	collection, err := client.GetCollection(ctx, d.DBName)
	if err != nil {
		// Try to create if doesn't exist
		slog.Info("Chroma collection not found, creating new one", "collection", d.DBName, "error", err.Error())

		// Create with no-op embedding function (we're providing pre-computed embeddings)
		// Use default embedding function which won't be called since we provide embeddings
		collection, err = client.CreateCollection(ctx, d.DBName,
			chromago.WithHNSWSpaceCreate(chromaEmbeddings.L2),
			chromago.WithIfNotExistsCreate(),
		)
		if err != nil {
			return fmt.Errorf("failed to get/create chroma collection '%s' at %s: %w", d.DBName, d.DBConnString, err)
		}
		slog.Info("Created new Chroma collection", "collection", d.DBName)
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

func (ds *DataSession) storeToRedis(ctx context.Context, store vectorstores.VectorStore, d *models.Datasource, contents []string, vectors [][]float32, metadatas []map[string]any) error {
	// Create Redis client
	opt, err := redis.ParseURL(d.DBConnString)
	if err != nil {
		return fmt.Errorf("failed to parse redis URL: %w", err)
	}

	client := redis.NewClient(opt)
	defer client.Close()

	// Ping to verify connection
	if err := client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("failed to connect to redis: %w", err)
	}

	// Store vectors using hash structure
	// Key format: {indexName}:vector:{id}
	indexName := d.DBName

	for i := range contents {
		vectorID := uuid.New().String()
		key := fmt.Sprintf("%s:vector:%s", indexName, vectorID)

		// Serialize vector as JSON
		vectorJSON, err := json.Marshal(vectors[i])
		if err != nil {
			return fmt.Errorf("failed to marshal vector: %w", err)
		}

		// Serialize metadata
		metadataJSON, err := json.Marshal(metadatas[i])
		if err != nil {
			return fmt.Errorf("failed to marshal metadata: %w", err)
		}

		// Store as Redis hash
		err = client.HSet(ctx, key, map[string]interface{}{
			"id":       vectorID,
			"content":  contents[i],
			"vector":   string(vectorJSON),
			"metadata": string(metadataJSON),
		}).Err()

		if err != nil {
			return fmt.Errorf("failed to store vector in redis: %w", err)
		}
	}

	slog.Info("Successfully stored vectors in Redis", "count", len(vectors), "index", indexName)
	return nil
}

func (ds *DataSession) storeToWeaviate(ctx context.Context, store vectorstores.VectorStore, d *models.Datasource, contents []string, vectors [][]float32, metadatas []map[string]any) error {
	// Parse namespace format: IndexName:Namespace
	split := strings.Split(d.DBName, ":")
	if len(split) != 2 {
		return fmt.Errorf("invalid weaviate namespace format, expected IndexName:Namespace, got: %s", d.DBName)
	}
	className := split[0]

	// Parse connection URL
	parsedURL, err := url.Parse(d.DBConnString)
	if err != nil {
		return fmt.Errorf("failed to parse weaviate URL: %w", err)
	}

	// Create Weaviate client
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

	// Prepare batch objects
	objects := make([]*weaviateModels.Object, len(contents))
	for i := range contents {
		vectorID := uuid.New().String()

		// Prepare properties
		properties := make(map[string]interface{})
		properties["content"] = contents[i]

		// Add metadata to properties
		if len(metadatas) > i {
			for k, v := range metadatas[i] {
				properties[k] = v
			}
		}

		objects[i] = &weaviateModels.Object{
			Class:      className,
			ID:         strfmt.UUID(vectorID),
			Vector:     vectors[i],
			Properties: properties,
		}
	}

	// Batch create objects
	results, err := client.Batch().ObjectsBatcher().
		WithObjects(objects...).
		Do(ctx)

	if err != nil {
		return fmt.Errorf("failed to batch create weaviate objects: %w", err)
	}

	// Check for errors in results
	if results != nil {
		for i, result := range results {
			if result.Result != nil && result.Result.Errors != nil && result.Result.Errors.Error != nil {
				slog.Warn("Error storing object in Weaviate", "index", i, "error", result.Result.Errors.Error[0].Message)
			}
		}
	}

	slog.Info("Successfully stored vectors in Weaviate", "count", len(vectors), "class", className)
	return nil
}

// === Vector Search Helper Methods ===

func (ds *DataSession) searchPineconeByVector(ctx context.Context, d *models.Datasource, vector []float32, topK int) ([]schema.Document, error) {
	// Create Pinecone client
	pc, err := pineconeSDK.NewClient(pineconeSDK.NewClientParams{
		ApiKey: d.DBConnAPIKey,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create pinecone client: %w", err)
	}

	// Get index connection
	idx, err := pc.Index(pineconeSDK.NewIndexConnParams{Host: d.DBConnString})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to pinecone index: %w", err)
	}

	// Query with vector
	queryResp, err := idx.QueryByVectorValues(ctx, &pineconeSDK.QueryByVectorValuesRequest{
		Vector:          vector,
		TopK:            uint32(topK),
		IncludeMetadata: true,
		IncludeValues:   false,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to query pinecone: %w", err)
	}

	// Convert results to schema.Document
	docs := make([]schema.Document, 0, len(queryResp.Matches))
	for _, match := range queryResp.Matches {
		metadata := make(map[string]any)
		if match.Vector != nil && match.Vector.Metadata != nil {
			for k, v := range match.Vector.Metadata.Fields {
				metadata[k] = v
			}
		}

		// Extract content from metadata
		content := ""
		if contentVal, ok := metadata["content"]; ok {
			if contentStr, ok := contentVal.(string); ok {
				content = contentStr
			}
		}

		docs = append(docs, schema.Document{
			PageContent: content,
			Metadata:    metadata,
			Score:       match.Score,
		})
	}

	return docs, nil
}

func (ds *DataSession) searchQdrantByVector(ctx context.Context, d *models.Datasource, vector []float32, topK int) ([]schema.Document, error) {
	// Qdrant SDK not available
	return nil, fmt.Errorf("qdrant vector search requires SDK installation")
}

func (ds *DataSession) searchPGVectorByVector(ctx context.Context, d *models.Datasource, vector []float32, topK int) ([]schema.Document, error) {
	// Connect to PostgreSQL
	db, err := gorm.Open(postgres.Open(d.DBConnString), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to postgres: %w", err)
	}

	tableName := d.DBName
	pgVector := pgvectorDriver.NewVector(vector)

	// Query with vector similarity
	type Result struct {
		ID       string
		Content  string
		Metadata string
		Distance float32
	}

	var results []Result
	query := fmt.Sprintf(`
		SELECT id, content, metadata, 1 - (embedding <=> ?) as distance
		FROM %s
		ORDER BY embedding <=> ?
		LIMIT ?
	`, tableName)

	err = db.Raw(query, pgVector, pgVector, topK).Scan(&results).Error
	if err != nil {
		return nil, fmt.Errorf("failed to query pgvector: %w", err)
	}

	// Convert to schema.Document
	docs := make([]schema.Document, 0, len(results))
	for _, r := range results {
		metadata := make(map[string]any)
		if r.Metadata != "" {
			json.Unmarshal([]byte(r.Metadata), &metadata)
		}

		docs = append(docs, schema.Document{
			PageContent: r.Content,
			Metadata:    metadata,
			Score:       r.Distance,
		})
	}

	return docs, nil
}

func (ds *DataSession) searchChromaByVector(ctx context.Context, store vectorstores.VectorStore, d *models.Datasource, vector []float32, topK int) ([]schema.Document, error) {
	// Create Chroma v2 client
	client, err := chromago.NewHTTPClient(chromago.WithBaseURL(d.DBConnString))
	if err != nil {
		return nil, fmt.Errorf("failed to create chroma client: %w", err)
	}

	// Get collection - if it doesn't exist or has validation issues, try to create it
	collection, err := client.GetCollection(ctx, d.DBName)
	if err != nil {
		slog.Info("Chroma collection not found for query, creating new one", "collection", d.DBName, "error", err.Error())

		// Create collection with L2 distance metric
		collection, err = client.CreateCollection(ctx, d.DBName,
			chromago.WithHNSWSpaceCreate(chromaEmbeddings.L2),
			chromago.WithIfNotExistsCreate(),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to get/create chroma collection '%s' for query: %w", d.DBName, err)
		}
		slog.Info("Created new Chroma collection for query", "collection", d.DBName)
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
				docMeta := metadatasGroups[groupIdx][docIdx]
				// Convert DocumentMetadata interface to map by extracting values
				// Store as opaque object for now
				metadata["_chroma_metadata"] = docMeta
			}

			score := float32(0)
			if len(distancesGroups) > groupIdx && len(distancesGroups[groupIdx]) > docIdx {
				score = float32(distancesGroups[groupIdx][docIdx])
			}

			// v2 Document interface has ContentString() method
			content := doc.ContentString()

			docs = append(docs, schema.Document{
				PageContent: content,
				Metadata:    metadata,
				Score:       score,
			})
		}
	}

	return docs, nil
}

func (ds *DataSession) searchRedisByVector(ctx context.Context, d *models.Datasource, vector []float32, topK int) ([]schema.Document, error) {
	// Redis vector search would require RediSearch module and specific index setup
	// This is a simplified implementation
	return nil, fmt.Errorf("redis vector search requires RediSearch module configuration")
}

func (ds *DataSession) searchWeaviateByVector(ctx context.Context, d *models.Datasource, vector []float32, topK int) ([]schema.Document, error) {
	// Parse namespace
	split := strings.Split(d.DBName, ":")
	if len(split) != 2 {
		return nil, fmt.Errorf("invalid weaviate namespace format")
	}
	className := split[0]

	// Parse URL
	parsedURL, err := url.Parse(d.DBConnString)
	if err != nil {
		return nil, fmt.Errorf("failed to parse weaviate URL: %w", err)
	}

	// Create client
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

	// Build nearVector query
	nearVector := client.GraphQL().NearVectorArgBuilder().
		WithVector(vector)

	// Query
	fields := []graphql.Field{
		{Name: "content"},
	}

	result, err := client.GraphQL().Get().
		WithClassName(className).
		WithNearVector(nearVector).
		WithLimit(topK).
		WithFields(fields...).
		Do(ctx)

	if err != nil {
		return nil, fmt.Errorf("failed to query weaviate: %w", err)
	}

	// Parse results
	docs := make([]schema.Document, 0)
	if result != nil && result.Data != nil {
		// Extract data from GraphQL response
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

						docs = append(docs, schema.Document{
							PageContent: content,
							Metadata:    itemMap,
							Score:       0, // Weaviate doesn't return scores directly in this query
						})
					}
				}
			}
		}
	}

	return docs, nil
}
