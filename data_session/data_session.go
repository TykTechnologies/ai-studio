package data_session

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/switches"
	"github.com/tmc/langchaingo/embeddings"
	"github.com/tmc/langchaingo/schema"
	"github.com/tmc/langchaingo/vectorstores"
	"github.com/tmc/langchaingo/vectorstores/chroma"
	"github.com/tmc/langchaingo/vectorstores/pgvector"
	"github.com/tmc/langchaingo/vectorstores/pinecone"
	"github.com/tmc/langchaingo/vectorstores/qdrant"
	"github.com/tmc/langchaingo/vectorstores/redisvector"
	"github.com/tmc/langchaingo/vectorstores/weaviate"
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

		results = append(results, docs...)
	}

	return results, nil
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
