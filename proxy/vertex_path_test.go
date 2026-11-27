package proxy

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// vertexCallHarness is a Vertex primary on project p, location us-central1,
// whose upstream requests all reach the fake vendor.
func vertexCallHarness(t *testing.T, allowed ...string) *failoverHarness {
	h := newFailoverHarness(t, serveVertex, serveOpenAIText("never"), func(primary, fallback *models.LLM) {
		vertexPrimary(vertexTestToken, allowed...)(primary, fallback)
		primary.APIEndpoint = "p:us-central1"
	})
	routeUpstreamTo(t, h.primaryVendor.server.URL)
	return h
}

// A Gemini API path on /llm/call reaches the upstream at the Vertex path,
// buffered on the REST handler and streamed (alt=sse kept) on the stream one.
func TestVertexCall_GeminiAPIPath(t *testing.T) {
	h := vertexCallHarness(t)

	resp, body := h.post("/llm/call/primary/v1beta/models/gemini-2.5-flash:generateContent", vertexNativeBody)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	assert.Contains(t, string(body), `"text":"hello"`)

	resp, body = h.post("/llm/call/primary/v1/models/gemini-2.5-flash:streamGenerateContent?alt=sse", vertexNativeBody)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	assert.Equal(t, vertexSSE, string(body), "relayed by the stream handler")

	// A Vertex path on /llm/call is left as it is.
	resp, body = h.post("/llm/call/primary"+vertexNativePath, vertexNativeBody)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))

	calls := h.primaryVendor.calls()
	require.Len(t, calls, 3)
	assert.Equal(t, vertexNativePath, calls[0].Path)
	assert.Equal(t, vertexStreamPath, calls[1].Path)
	assert.Equal(t, vertexNativePath, calls[2].Path)
	for _, c := range calls {
		assert.Equal(t, "Bearer "+vertexTestToken, c.Headers.Get("Authorization"))
		assertNoAppCredential(t, h, c)
	}
}

// Model validation sees the mapped path, so it allows and denies by the
// model's name.
func TestVertexCall_AllowedModels(t *testing.T) {
	h := vertexCallHarness(t, "^gemini-2.5-flash$")

	resp, body := h.post("/llm/call/primary/v1beta/models/gemini-2.5-flash:generateContent", vertexNativeBody)
	assert.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	resp, body = h.post("/llm/call/primary/v1beta/models/gemini-2.5-pro:streamGenerateContent?alt=sse", vertexNativeBody)
	assert.Equal(t, http.StatusForbidden, resp.StatusCode, string(body))
	assert.Len(t, h.primaryVendor.calls(), 1)
}

// A bare URL endpoint names no project or location, so a Gemini API path is
// refused with the reason rather than sent where Vertex cannot serve it.
func TestVertexCall_URLEndpointRefusesGeminiPath(t *testing.T) {
	t.Parallel()
	h := newFailoverHarness(t, serveVertex, serveOpenAIText("never"), vertexPrimary(vertexTestToken))

	resp, body := h.post("/llm/call/primary/v1beta/models/gemini-2.5-flash:generateContent", vertexNativeBody)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, string(body))
	assert.Contains(t, string(body), "project:location")
	assert.Empty(t, h.primaryVendor.calls())

	// The Vertex path still works on it.
	resp, body = h.post("/llm/call/primary"+vertexNativePath, vertexNativeBody)
	assert.Equal(t, http.StatusOK, resp.StatusCode, string(body))
}

// Other vendors' paths are not touched.
func TestVertexCall_GoogleAIUntouched(t *testing.T) {
	t.Parallel()
	h := newFailoverHarness(t, serveVertex, serveOpenAIText("never"), func(primary, _ *models.LLM) {
		primary.Vendor = models.GOOGLEAI
		primary.Failover = models.LLMFailover{}
	})
	resp, body := h.post("/llm/call/primary/v1beta/models/gemini-2.5-flash:generateContent", vertexNativeBody)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	calls := h.primaryVendor.calls()
	require.Len(t, calls, 1)
	assert.True(t, strings.HasPrefix(calls[0].Path, "/v1beta/models/gemini-2.5-flash:generateContent"), calls[0].Path)
}

// A URL endpoint naming the project and location in its path maps Gemini API
// paths to it (how a test server or private endpoint is pointed at).
func TestVertexCall_URLEndpointWithProjectLocation(t *testing.T) {
	t.Parallel()
	h := newFailoverHarness(t, serveVertex, serveOpenAIText("never"), vertexPrimary(vertexTestToken))
	h.primary.APIEndpoint = h.primaryVendor.server.URL + "/v1/projects/p/locations/us-central1"
	require.NoError(t, h.db.Save(h.primary).Error)
	require.NoError(t, h.proxy.loadResources())

	resp, body := h.post("/llm/call/primary/v1beta/models/gemini-2.5-flash:generateContent", vertexNativeBody)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	resp, body = h.post("/llm/call/primary/v1beta/models/gemini-2.5-flash:streamGenerateContent?alt=sse", vertexNativeBody)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	calls := h.primaryVendor.calls()
	require.Len(t, calls, 2)
	assert.Equal(t, vertexNativePath, calls[0].Path)
	assert.Equal(t, vertexStreamPath, calls[1].Path)
}
