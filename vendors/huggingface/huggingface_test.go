package hfVendor

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/third_party/langchaingo/llms/huggingface"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The embedder asks for the model it was configured with, not an empty one.
func TestHFWrapperUsesModelName(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[[0.1,0.2]]`))
	}))
	t.Cleanup(srv.Close)

	w, err := NewHFWrapper("sentence-transformers/all-MiniLM-L6-v2",
		huggingface.WithToken("hf_test"), huggingface.WithURL(srv.URL))
	require.NoError(t, err)
	assert.Equal(t, "sentence-transformers/all-MiniLM-L6-v2", w.ModelName)

	_, err = w.CreateEmbedding(context.Background(), []string{"hi"})
	require.NoError(t, err)
	assert.Equal(t, "/models/sentence-transformers/all-MiniLM-L6-v2", gotPath)
}
