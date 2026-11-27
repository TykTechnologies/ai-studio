package vertexVendor

import (
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
)

func TestUpstreamBaseURL(t *testing.T) {
	for _, tc := range []struct {
		endpoint, want string
	}{
		{"my-project:us-central1", "https://us-central1-aiplatform.googleapis.com"},
		{"my-project:europe-west4", "https://europe-west4-aiplatform.googleapis.com"},
		{" my-project:us-central1 ", "https://us-central1-aiplatform.googleapis.com"},
		{"my-project:us", "https://aiplatform.us.rep.googleapis.com"},
		{"my-project:eu", "https://aiplatform.eu.rep.googleapis.com"},
		{"my-project:global", "https://aiplatform.googleapis.com"},
		// A domain-scoped project ID keeps its own colon.
		{"example.com:my-project:us-central1", "https://us-central1-aiplatform.googleapis.com"},
		// A URL passes through.
		{"https://us-central1-aiplatform.googleapis.com", "https://us-central1-aiplatform.googleapis.com"},
		{"https://psc.example.internal/vertex", "https://psc.example.internal/vertex"},
		{"http://127.0.0.1:8089", "http://127.0.0.1:8089"},
	} {
		t.Run(tc.endpoint, func(t *testing.T) {
			got, err := UpstreamBaseURL(&models.LLM{Vendor: models.VERTEX, APIEndpoint: tc.endpoint})
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestUpstreamBaseURL_Invalid(t *testing.T) {
	for _, endpoint := range []string{
		"",
		"my-project",
		"my-project:",
		":us-central1",
		"my-project:US-CENTRAL1",
		"my-project:evil.com/x",
		"my-project:us-central1.evil.com",
		"my-project:a",
		"ftp://example.com",
		"https://",
	} {
		t.Run(endpoint, func(t *testing.T) {
			_, err := UpstreamBaseURL(&models.LLM{Vendor: models.VERTEX, APIEndpoint: endpoint})
			assert.ErrorIs(t, err, ErrInvalidEndpoint)
		})
	}
}

func TestParseProjectLocation(t *testing.T) {
	project, location, ok := ParseProjectLocation("example.com:my-project:us-central1")
	require.True(t, ok)
	assert.Equal(t, "example.com:my-project", project)
	assert.Equal(t, "us-central1", location)

	_, _, ok = ParseProjectLocation("https://us-central1-aiplatform.googleapis.com")
	assert.False(t, ok)
}

func TestProxyScreenRequest(t *testing.T) {
	v := &Vertex{}
	base := "/llm/rest/v/v1/projects/p/locations/l/publishers/google/models/gemini-2.5-flash"
	buffered := httptest.NewRequest("POST", base+":generateContent", nil)
	stream := httptest.NewRequest("POST", base+":streamGenerateContent?alt=sse", nil)
	count := httptest.NewRequest("POST", base+":countTokens", nil)

	assert.NoError(t, v.ProxyScreenRequest(nil, buffered, false))
	assert.NoError(t, v.ProxyScreenRequest(nil, count, false))
	assert.NoError(t, v.ProxyScreenRequest(nil, stream, true))
	assert.Error(t, v.ProxyScreenRequest(nil, stream, false), "a stream on the REST route")
	assert.Error(t, v.ProxyScreenRequest(nil, buffered, true), "a buffered call on the stream route")
}

func TestAnalyzeStreamingResponse(t *testing.T) {
	sse := "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"hi\"}]}}]}\r\n\r\n" +
		"data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"!\"}]}}],\"usageMetadata\":{\"promptTokenCount\":5,\"candidatesTokenCount\":2,\"thoughtsTokenCount\":1}}\r\n\r\n"
	r := httptest.NewRequest("POST", "/llm/stream/v/v1/projects/p/locations/l/publishers/google/models/gemini-2.5-flash:streamGenerateContent?alt=sse", nil)
	_, _, resp, err := (&Vertex{}).AnalyzeStreamingResponse(&models.LLM{}, &models.App{}, 200, []byte(sse), r, nil)
	require.NoError(t, err)
	assert.Equal(t, 5, resp.GetPromptTokens())
	assert.Equal(t, 3, resp.GetResponseTokens())
	assert.Equal(t, "gemini-2.5-flash", resp.GetModel(), "taken from the path when the stream names none")
}
