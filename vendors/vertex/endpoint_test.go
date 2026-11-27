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
		{"example.com:my-project:eu", "https://aiplatform.eu.rep.googleapis.com"},
		// A URL passes through.
		{"https://us-central1-aiplatform.googleapis.com", "https://us-central1-aiplatform.googleapis.com"},
		{"https://psc.example.internal/vertex", "https://psc.example.internal/vertex"},
		{"http://127.0.0.1:8089", "http://127.0.0.1:8089"},
		// A trailing [/v1]/projects/{p}/locations/{l} is not part of the base.
		{"https://vertex.example/v1/projects/p/locations/us-central1", "https://vertex.example"},
		{"http://127.0.0.1:8089/v1/projects/p/locations/eu/", "http://127.0.0.1:8089"},
		{"https://gw.example/vertex/projects/p/locations/global", "https://gw.example/vertex"},
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
		"https://vertex.example/v1/projects/p",
		"https://vertex.example/v1/projects/p/locations",
		"https://vertex.example/v1/projects/p/regions/us-central1",
		"https://vertex.example/v1/projects/p/locations/us-central1/publishers/google",
		"https://vertex.example/v1/projects/p/locations/US",
		"https://vertex.example/v1/projects/p/locations/us-central1?x=1",
		"https://vertex.example/v1/projects/%2e%2e/locations/us-central1",
		"my%project:us-central1",
		// Only one domain prefix may carry a colon in the project.
		"a:b:us-central1",
		"example.com:a:b:us-central1",
		"example.com::us-central1",
		":my-project:us-central1",
		".com:my-project:us-central1",
		"example..com:my-project:us-central1",
		"My-Project:us-central1",
		"my-project-:us-central1",
		"https://vertex.example/v1/projects/a:b/locations/us-central1",
		"https://vertex.example/v1/projects/example.com:a:b/locations/us-central1",
	} {
		t.Run(endpoint, func(t *testing.T) {
			_, err := UpstreamBaseURL(&models.LLM{Vendor: models.VERTEX, APIEndpoint: endpoint})
			assert.ErrorIs(t, err, ErrInvalidEndpoint)
		})
	}
}

func TestParseProjectLocation(t *testing.T) {
	for endpoint, want := range map[string][2]string{
		"my-project:us-central1":             {"my-project", "us-central1"},
		"123456789012:eu":                    {"123456789012", "eu"},
		"example.com:my-project:us-central1": {"example.com:my-project", "us-central1"},
		"corp.example.co.uk:my-proj:global":  {"corp.example.co.uk:my-proj", "global"},
	} {
		project, location, ok := ParseProjectLocation(endpoint)
		require.True(t, ok, endpoint)
		assert.Equal(t, want[0], project, endpoint)
		assert.Equal(t, want[1], location, endpoint)
	}
	for _, endpoint := range []string{"a:b:us-central1", "example.com::us-central1", "example.com:my-project:", "example.com:my-project"} {
		_, _, ok := ParseProjectLocation(endpoint)
		assert.False(t, ok, endpoint)
	}

	_, _, ok := ParseProjectLocation("https://us-central1-aiplatform.googleapis.com")
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

func TestParseURLEndpoint(t *testing.T) {
	for _, tc := range []struct {
		endpoint, base, project, location string
	}{
		{"https://vertex.example/v1/projects/p/locations/us-central1", "https://vertex.example", "p", "us-central1"},
		{"https://vertex.example/projects/p/locations/us-central1", "https://vertex.example", "p", "us-central1"},
		{"https://vertex.example/v1/projects/p/locations/us-central1/", "https://vertex.example", "p", "us-central1"},
		{"http://127.0.0.1:8089/v1beta1/projects/123456/locations/global", "http://127.0.0.1:8089", "123456", "global"},
		{"https://gw.example/vertex/v1/projects/example.com:p/locations/eu", "https://gw.example/vertex", "example.com:p", "eu"},
		{"https://vertex.example/v1/projects/example.com:my-proj/locations/us-central1/", "https://vertex.example", "example.com:my-proj", "us-central1"},
		{"https://vertex.example", "https://vertex.example", "", ""},
		{"https://vertex.example/prefix/", "https://vertex.example/prefix/", "", ""},
	} {
		t.Run(tc.endpoint, func(t *testing.T) {
			base, project, location, err := ParseURLEndpoint(tc.endpoint)
			require.NoError(t, err)
			assert.Equal(t, tc.base, base)
			assert.Equal(t, tc.project, project)
			assert.Equal(t, tc.location, location)
		})
	}
}

func TestProxySetAuthHeader_DropsCallerGeminiKey(t *testing.T) {
	r := httptest.NewRequest("POST", "https://us-central1-aiplatform.googleapis.com/v1/projects/p/locations/l/publishers/google/models/m:streamGenerateContent?alt=sse&key=caller", nil)
	r.Header.Set("x-goog-api-key", "caller")
	r.Header.Set("Authorization", "Bearer caller")
	require.NoError(t, (&Vertex{}).ProxySetAuthHeader(r, &models.LLM{Vendor: models.VERTEX, APIKey: "ya29.llm"}))
	assert.Equal(t, []string{"Bearer ya29.llm"}, r.Header.Values("Authorization"))
	assert.Empty(t, r.Header.Get("x-goog-api-key"))
	assert.Equal(t, "alt=sse", r.URL.RawQuery)
}

func TestNativePathFromGeminiAPI(t *testing.T) {
	llm := &models.LLM{Vendor: models.VERTEX, APIEndpoint: "my-project:us-central1"}
	native := "/v1/projects/my-project/locations/us-central1/publishers/google/models/gemini-2.5-flash:"
	for path, want := range map[string]string{
		"/v1beta/models/gemini-2.5-flash:generateContent":       native + "generateContent",
		"/v1beta/models/gemini-2.5-flash:streamGenerateContent": native + "streamGenerateContent",
		"/v1/models/gemini-2.5-flash:generateContent":           native + "generateContent",
		"/v1beta/models/gemini-2.5-flash:countTokens":           native + "countTokens",
	} {
		got, changed, err := NativePathFromGeminiAPI(llm, path)
		require.NoError(t, err, path)
		assert.True(t, changed, path)
		assert.Equal(t, want, got, path)
	}

	// Anything else is left as it is: a Vertex path, other Gemini API calls.
	for _, path := range []string{
		native + "generateContent",
		"/v1beta/models",
		"/v1beta/models/gemini-2.5-flash",
		"/v1beta/models/gemini-2.5-flash:generateContent/x",
		"/v1beta/tunedModels/x:generateContent",
		"/v2/models/gemini-2.5-flash:generateContent",
	} {
		got, changed, err := NativePathFromGeminiAPI(llm, path)
		require.NoError(t, err, path)
		assert.False(t, changed, path)
		assert.Equal(t, path, got)
	}

	// A domain-scoped project keeps its colon; multi-region and global work.
	got, _, err := NativePathFromGeminiAPI(&models.LLM{APIEndpoint: "example.com:proj:global"}, "/v1beta/models/m:generateContent")
	require.NoError(t, err)
	assert.Equal(t, "/v1/projects/example.com:proj/locations/global/publishers/google/models/m:generateContent", got)

	// A URL endpoint names no project or location.
	_, _, err = NativePathFromGeminiAPI(&models.LLM{APIEndpoint: "https://vertex.example"}, "/v1beta/models/m:generateContent")
	assert.ErrorIs(t, err, ErrNoProjectLocation)
	_, changed, err := NativePathFromGeminiAPI(&models.LLM{APIEndpoint: "https://vertex.example"}, native+"generateContent")
	assert.NoError(t, err, "a Vertex path needs no mapping")
	assert.False(t, changed)
}
