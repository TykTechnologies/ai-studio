package vertexVendor

import (
	"net/http/httptest"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The model is recorded without the :method suffix, also when the response
// carries no modelVersion (a refused or failed request), so analytics and
// auto-created model prices use the model's name.
func TestExtractModelIDFromVertexURL_StripsMethod(t *testing.T) {
	for path, want := range map[string]string{
		"/v1/projects/p/locations/global/publishers/google/models/gemini-3.5-flash:generateContent":   "gemini-3.5-flash",
		"/v1/projects/p/locations/us/publishers/google/models/gemini-3.5-flash:streamGenerateContent": "gemini-3.5-flash",
		"/llm/rest/vx/v1/projects/p/locations/l/publishers/google/models/gemini-2.5-pro:countTokens":  "gemini-2.5-pro",
		"/v1/projects/p/locations/l/publishers/google/models/gemini-2.5-flash":                        "gemini-2.5-flash",
		"/v1/projects/p/locations/l/publishers/google/models/text-embedding-004:predict":              "text-embedding-004",
	} {
		got, err := extractModelIDFromVertexURL(path)
		require.NoError(t, err, path)
		assert.Equal(t, want, got, path)
	}
	_, err := extractModelIDFromVertexURL("/v1/projects/p/locations/l")
	assert.Error(t, err)
}

func TestAnalyzeResponse_ErrorBodyRecordsModelName(t *testing.T) {
	r := httptest.NewRequest("POST", "/llm/rest/vx/v1/projects/p/locations/global/publishers/google/models/gemini-3.5-flash:generateContent", nil)
	errBody := []byte(`{"status":502,"message":"LLM credential not usable: vertex: no credential configured"}`)
	_, _, resp, err := (&Vertex{}).AnalyzeResponse(&models.LLM{Vendor: models.VERTEX}, &models.App{}, 502, errBody, r)
	require.NoError(t, err)
	assert.Equal(t, "gemini-3.5-flash", resp.GetModel())
}
