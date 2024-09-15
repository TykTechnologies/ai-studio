package data_session

import (
	"context"
	"fmt"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/switches"
	"github.com/tmc/langchaingo/embeddings"
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
	e, err := switches.GetEmbedder(d)
	return e, err
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
