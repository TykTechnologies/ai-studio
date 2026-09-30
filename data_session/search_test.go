package data_session

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// fakeVectorBackend serves an OpenAI-compatible embeddings endpoint and a
// Qdrant search endpoint: collection "good" returns one document, any other
// collection fails.
func fakeVectorBackend(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/embeddings", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"object": "list",
			"model":  "m",
			"data":   []map[string]any{{"object": "embedding", "index": 0, "embedding": []float32{0.1, 0.2, 0.3}}},
		})
	})
	mux.HandleFunc("/collections/good/points/search", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"result": []map[string]any{{"score": 0.9, "payload": map[string]any{"content": "from the good source"}}},
		})
	})
	mux.HandleFunc("/collections/", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"status":{"error":"collection not found"}}`, http.StatusNotFound)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func qdrantSource(id uint, backend, collection string) *models.Datasource {
	return &models.Datasource{
		ID:           id,
		Name:         collection,
		DBSourceType: VECTOR_QDRANT,
		DBConnString: backend,
		DBName:       collection,
		Embedder:     &models.Embedder{Vendor: models.OPENAI, Endpoint: backend, APIKey: "sk", ModelName: "m"},
	}
}

// One failing datasource (unreachable store, missing embedder, or Chroma in
// a build without cgo) must not hide the results of the others.
func TestSearch_SkipsAFailingSource(t *testing.T) {
	backend := fakeVectorBackend(t).URL
	ds := &DataSession{Sources: map[uint]*models.Datasource{
		1: qdrantSource(1, backend, "good"),
		2: qdrantSource(2, backend, "missing"),
		3: {ID: 3, Name: "no-embedder", DBSourceType: VECTOR_QDRANT},
	}}

	docs, err := ds.Search("hello", 3)
	require.NoError(t, err)
	require.Len(t, docs, 1)
	assert.Equal(t, "from the good source", docs[0].PageContent)
}

// When every datasource fails, Search reports the failures.
func TestSearch_ErrorsWhenEverySourceFails(t *testing.T) {
	backend := fakeVectorBackend(t).URL
	ds := &DataSession{Sources: map[uint]*models.Datasource{
		2: qdrantSource(2, backend, "missing"),
		3: {ID: 3, Name: "no-embedder", DBSourceType: VECTOR_QDRANT},
	}}

	docs, err := ds.Search("hello", 3)
	require.Error(t, err)
	assert.Nil(t, docs)
	assert.Contains(t, err.Error(), `datasource "no-embedder" has no embedder`)
	assert.Contains(t, err.Error(), "missing")
}

// No datasources is not an error: there is simply nothing to find.
func TestSearch_NoSources(t *testing.T) {
	docs, err := (&DataSession{}).Search("hello", 3)
	require.NoError(t, err)
	assert.Empty(t, docs)
}
