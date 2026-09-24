package data_session

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Embedding reads the datasource's embedder; a datasource loaded without one
// fails with a clear error rather than a nil dereference or a vendor lookup
// on an empty string.
func TestGetEmbedder_UsesTheDatasourceEmbedder(t *testing.T) {
	ds := &DataSession{}

	_, err := ds.getEmbedder(&models.Datasource{Name: "Bare"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `datasource "Bare" has no embedder`)

	llmID := uint(7)
	_, err = ds.getEmbedder(&models.Datasource{Name: "Orphan", Embedder: &models.Embedder{LLMID: &llmID, ModelName: "m"}})
	require.Error(t, err)
	assert.ErrorIs(t, err, models.ErrEmbedderLLMMissing)

	_, err = ds.getEmbedder(&models.Datasource{Name: "Claude", Embedder: &models.Embedder{Vendor: models.ANTHROPIC, ModelName: "m"}})
	assert.Error(t, err, "a vendor that cannot embed is refused by switches")

	e, err := ds.getEmbedder(&models.Datasource{Name: "OK", Embedder: &models.Embedder{Vendor: models.OPENAI, APIKey: "sk", ModelName: "text-embedding-3-small"}})
	require.NoError(t, err)
	assert.NotNil(t, e)
}
