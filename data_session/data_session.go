package data_session

import (
	"context"
	"fmt"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/tmc/langchaingo/embeddings"
	"github.com/tmc/langchaingo/llms/openai"
	"github.com/tmc/langchaingo/schema"
	"github.com/tmc/langchaingo/vectorstores"
	"github.com/tmc/langchaingo/vectorstores/pinecone"
)

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
	var llm embeddings.EmbedderClient
	var err error

	switch d.EmbedVendor {
	case models.OPENAI:
		opts := []openai.Option{}
		if d.EmbedAPIKey != "" {
			opts = append(opts, openai.WithToken(d.EmbedAPIKey))
		}
		if d.EmbedUrl != "" {
			opts = append(opts, openai.WithBaseURL(d.EmbedUrl))
		}
		if d.EmbedModel == "" {
			return nil, fmt.Errorf("missing embed model")
		}

		opts = append(opts, openai.WithEmbeddingModel(d.EmbedModel))
		llm, err = openai.New(opts...)
	default:
		return nil, fmt.Errorf("unsupported embed vendor")
	}

	if err != nil {
		return nil, err
	}

	e, err := embeddings.NewEmbedder(llm)
	if err != nil {
		return nil, err
	}

	return e, nil
}

func (ds *DataSession) getStore(d *models.Datasource, embedder *embeddings.EmbedderImpl) (vectorstores.VectorStore, error) {
	var store vectorstores.VectorStore
	var err error

	switch d.DBSourceType {
	case "pinecone":
		store, err = pinecone.New(
			pinecone.WithHost(d.DBConnString),
			pinecone.WithEmbedder(embedder),
			pinecone.WithAPIKey(d.DBConnAPIKey),
			pinecone.WithNameSpace(d.DBName))
	default:
		return nil, fmt.Errorf("unsupported store type")
	}

	if err != nil {
		return nil, err
	}

	return store, nil
}
