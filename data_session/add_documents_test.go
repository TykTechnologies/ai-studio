package data_session

import (
	"context"
	"os"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/third_party/langchaingo/embeddings"
	"github.com/TykTechnologies/midsommar/v2/third_party/langchaingo/schema"
	"github.com/TykTechnologies/midsommar/v2/third_party/langchaingo/vectorstores"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pgvector refuses a namespace on add; the other stores take the
// datasource's DBName as it.
func TestAddDocumentsOptions(t *testing.T) {
	for _, typ := range []string{VECTOR_PINECONE, VECTOR_CHROMA, VECTOR_REDIS, VECTOR_QDRANT, VECTOR_WEAVIATE} {
		opts := vectorstores.Options{}
		for _, o := range addDocumentsOptions(&models.Datasource{DBSourceType: typ, DBName: "docs"}) {
			o(&opts)
		}
		assert.Equal(t, "docs", opts.NameSpace, typ)
	}
	assert.Empty(t, addDocumentsOptions(&models.Datasource{DBSourceType: VECTOR_PGVECTOR, DBName: "docs"}))
}

// PGVECTOR_TEST_DSN is a Postgres with the vector extension, e.g.
// docker run -p 5433:5432 -e POSTGRES_PASSWORD=pw pgvector/pgvector:pg16
// and postgres://postgres:pw@127.0.0.1:5433/postgres?sslmode=disable.
func TestPGVectorAddDocuments(t *testing.T) {
	dsn := os.Getenv("PGVECTOR_TEST_DSN")
	if dsn == "" {
		t.Skip("PGVECTOR_TEST_DSN not set")
	}
	d := &models.Datasource{
		DBSourceType: VECTOR_PGVECTOR, DBConnString: dsn, DBName: "lsj37_docs",
		AllowInternalHost: true,
	}
	embedder, err := embeddings.NewEmbedder(embeddings.EmbedderClientFunc(
		func(_ context.Context, texts []string) ([][]float32, error) {
			out := make([][]float32, len(texts))
			for i := range texts {
				out[i] = []float32{1, float32(i), 0}
			}
			return out, nil
		}))
	require.NoError(t, err)

	store, err := (&DataSession{}).getStore(d, embedder)
	require.NoError(t, err)
	_, err = store.AddDocuments(context.Background(),
		[]schema.Document{{PageContent: "hello pgvector"}}, addDocumentsOptions(d)...)
	require.NoError(t, err)

	docs, err := store.SimilaritySearch(context.Background(), "hello", 1)
	require.NoError(t, err)
	require.Len(t, docs, 1)
	assert.Equal(t, "hello pgvector", docs[0].PageContent)
}
