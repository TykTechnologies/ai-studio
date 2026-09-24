package services

import (
	"testing"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The hub sends each datasource's embedder flattened into the embed_*
// fields; the edge rebuilds it in memory for the embedding code.
func TestConvertDatasource_RebuildsEmbedder(t *testing.T) {
	a := &GatewayServiceAdapter{crypto: &noopCryptoService{}}

	ds := a.convertDatabaseDatasourceToModel(&database.Datasource{
		Name: "Docs", EmbedVendor: "openai", EmbedUrl: "https://api.openai.com/v1",
		EmbedAPIKeyEncrypted: "sk-1", EmbedModel: "text-embedding-3-small",
	})
	require.NotNil(t, ds.Embedder)
	spec, err := ds.Embedder.Spec(false)
	require.NoError(t, err)
	assert.Equal(t, models.OPENAI, spec.Vendor)
	assert.Equal(t, "https://api.openai.com/v1", spec.Endpoint)
	assert.Equal(t, "sk-1", spec.APIKey)
	assert.Equal(t, "text-embedding-3-small", spec.Model)

	none := a.convertDatabaseDatasourceToModel(&database.Datasource{Name: "Bare"})
	assert.Nil(t, none.Embedder)
}

// A hub from before Embedders sent Vertex's project:location and key only in
// the vector store fields (the Vertex embedder used to read them there).
func TestConvertDatasource_VertexFromOlderHub(t *testing.T) {
	a := &GatewayServiceAdapter{crypto: &noopCryptoService{}}
	ds := a.convertDatabaseDatasourceToModel(&database.Datasource{
		Name: "V", EmbedVendor: "vertex", EmbedModel: "text-embedding-004",
		DBConnStringEncrypted: "proj:us-central1", DBConnAPIKeyEncrypted: "vertex-key",
	})
	require.NotNil(t, ds.Embedder)
	assert.Equal(t, "proj:us-central1", ds.Embedder.Endpoint)
	assert.Equal(t, "vertex-key", ds.Embedder.APIKey)
}
