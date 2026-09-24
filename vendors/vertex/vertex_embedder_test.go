package vertexVendor

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The Vertex embedder reads project:location from the embedder's endpoint
// (it used to read the datasource's vector store connection string).
func TestVertexEmbedder_EndpointIsProjectLocation(t *testing.T) {
	for _, endpoint := range []string{"", "my-project", "a:b:c"} {
		_, err := setupVertexEmbedClient(&models.EmbedderSpec{Vendor: models.VERTEX, Endpoint: endpoint, Model: "text-embedding-004"})
		require.Error(t, err, endpoint)
		assert.Contains(t, err.Error(), "project:location", endpoint)
	}
}
