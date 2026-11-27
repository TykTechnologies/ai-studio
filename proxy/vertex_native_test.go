package proxy

import (
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"

	"github.com/TykTechnologies/midsommar/v2/models"
	vertexVendor "github.com/TykTechnologies/midsommar/v2/vendors/vertex"
)

const (
	vertexNativePath   = "/v1/projects/p/locations/us-central1/publishers/google/models/gemini-2.5-flash:generateContent"
	vertexStreamPath   = "/v1/projects/p/locations/us-central1/publishers/google/models/gemini-2.5-flash:streamGenerateContent"
	vertexNativeBody   = `{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`
	vertexTestToken    = "ya29.test-access-token"
	vertexBufferedJSON = `{"candidates":[{"content":{"role":"model","parts":[{"text":"hello"}]},"finishReason":"STOP"}],` +
		`"usageMetadata":{"promptTokenCount":7,"candidatesTokenCount":3,"totalTokenCount":10},"modelVersion":"gemini-2.5-flash"}`
	vertexSSE = "data: {\"candidates\":[{\"content\":{\"role\":\"model\",\"parts\":[{\"text\":\"hel\"}]}}],\"modelVersion\":\"gemini-2.5-flash\"}\r\n\r\n" +
		"data: {\"candidates\":[{\"content\":{\"role\":\"model\",\"parts\":[{\"text\":\"lo\"}]},\"finishReason\":\"STOP\"}]," +
		"\"usageMetadata\":{\"promptTokenCount\":8,\"candidatesTokenCount\":4,\"totalTokenCount\":12},\"modelVersion\":\"gemini-2.5-flash\"}\r\n\r\n"
)

// serveVertex answers generateContent with JSON and streamGenerateContent
// with SSE, as Vertex's REST API does.
func serveVertex(w http.ResponseWriter, r *http.Request) {
	if strings.Contains(r.URL.Path, ":streamGenerateContent") {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(vertexSSE))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(vertexBufferedJSON))
}

// vertexPrimary makes the harness's primary a Vertex LLM whose endpoint is
// the fake upstream's URL (a URL endpoint passes through UpstreamBaseURL).
func vertexPrimary(key string, allowed ...string) func(primary, fallback *models.LLM) {
	return func(primary, _ *models.LLM) {
		primary.Vendor = models.VERTEX
		primary.APIKey = key
		primary.DefaultModel = "gemini-2.5-flash"
		primary.AllowedModels = allowed
		primary.Failover = models.LLMFailover{}
	}
}

// The native Vertex route answers (it answered every request with a 400
// before): a buffered call reaches the upstream at the Vertex path with the
// LLM's access token, never the App credential, and its tokens are recorded.
func TestVertexNative_Rest(t *testing.T) {
	t.Parallel()
	h := newFailoverHarness(t, serveVertex, serveOpenAIText("never"), vertexPrimary(vertexTestToken))

	resp, body := h.post("/llm/rest/primary"+vertexNativePath, vertexNativeBody)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	assert.Contains(t, string(body), `"text":"hello"`)

	calls := h.primaryVendor.calls()
	require.Len(t, calls, 1)
	assert.Equal(t, vertexNativePath, calls[0].Path, "the Vertex path, joined onto the base unchanged")
	assert.Equal(t, "Bearer "+vertexTestToken, calls[0].Headers.Get("Authorization"))
	assertNoAppCredential(t, h, calls[0])

	recs := waitForChatRecords(t, h)
	assert.Equal(t, 7, recs[0].PromptTokens)
	assert.Equal(t, 3, recs[0].ResponseTokens)
}

// A stream reaches the upstream with its query (alt=sse) and is relayed as
// SSE; the stream's usage is recorded (it was a dummy response before).
func TestVertexNative_Stream(t *testing.T) {
	t.Parallel()
	h := newFailoverHarness(t, serveVertex, serveOpenAIText("never"), vertexPrimary(vertexTestToken))

	resp, body := h.post("/llm/stream/primary"+vertexStreamPath+"?alt=sse", vertexNativeBody)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	assert.Equal(t, vertexSSE, string(body))

	calls := h.primaryVendor.calls()
	require.Len(t, calls, 1)
	assert.Equal(t, vertexStreamPath, calls[0].Path)
	assert.Equal(t, "Bearer "+vertexTestToken, calls[0].Headers.Get("Authorization"))
	assertNoAppCredential(t, h, calls[0])

	recs := waitForChatRecords(t, h)
	assert.Equal(t, 8, recs[0].PromptTokens)
	assert.Equal(t, 4, recs[0].ResponseTokens)
	assert.Equal(t, "gemini-2.5-flash", recs[0].Name)
}

// Each route takes only its own kind of call.
func TestVertexNative_WrongChannel(t *testing.T) {
	t.Parallel()
	h := newFailoverHarness(t, serveVertex, serveOpenAIText("never"), vertexPrimary(vertexTestToken))

	resp, body := h.post("/llm/rest/primary"+vertexStreamPath, vertexNativeBody)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, string(body))
	resp, body = h.post("/llm/stream/primary"+vertexNativePath, vertexNativeBody)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, string(body))
	assert.Empty(t, h.primaryVendor.calls())
}

// Allowed models are matched on the model's name, without the :method the
// path carries after it.
func TestVertexNative_AllowedModels(t *testing.T) {
	t.Parallel()
	h := newFailoverHarness(t, serveVertex, serveOpenAIText("never"), vertexPrimary(vertexTestToken, "^gemini-2.5-flash$"))

	resp, body := h.post("/llm/rest/primary"+vertexNativePath, vertexNativeBody)
	assert.Equal(t, http.StatusOK, resp.StatusCode, string(body))

	denied := strings.Replace(vertexNativePath, "gemini-2.5-flash", "gemini-2.5-pro", 1)
	resp, body = h.post("/llm/rest/primary"+denied, vertexNativeBody)
	assert.Equal(t, http.StatusForbidden, resp.StatusCode, string(body))
	assert.Len(t, h.primaryVendor.calls(), 1, "the denied model never reaches the upstream")
}

// The upstream gets the token the LLM's credential mints.
func TestVertexNative_MintedToken(t *testing.T) {
	requireSerial(t)
	restore := vertexVendor.SetTokenSourceOverride(func(*models.LLM) (oauth2.TokenSource, error) {
		return oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "minted-token", Expiry: time.Now().Add(time.Hour)}), nil
	})
	t.Cleanup(restore)
	h := newFailoverHarness(t, serveVertex, serveOpenAIText("never"), vertexPrimary(`{"type":"service_account"}`))

	resp, body := h.post("/llm/rest/primary"+vertexNativePath, vertexNativeBody)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	resp, body = h.post("/llm/stream/primary"+vertexStreamPath+"?alt=sse", vertexNativeBody)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))

	calls := h.primaryVendor.calls()
	require.Len(t, calls, 2)
	for _, c := range calls {
		assert.Equal(t, "Bearer minted-token", c.Headers.Get("Authorization"), c.Path)
		assertNoAppCredential(t, h, c)
	}
}

func assertNoAppCredential(t *testing.T, h *failoverHarness, c vendorCall) {
	t.Helper()
	for name, values := range c.Headers {
		for _, v := range values {
			assert.NotContains(t, v, h.apiKey, "header %s carries the App credential", name)
		}
	}
}

func waitForChatRecords(t *testing.T, h *failoverHarness) []models.LLMChatRecord {
	t.Helper()
	var recs []models.LLMChatRecord
	require.Eventually(t, func() bool { recs = h.chatRecords(); return len(recs) == 1 }, 5*time.Second, 20*time.Millisecond)
	return recs
}

// A project:location endpoint sends both routes to the location's service
// host; a plugin's upstream_override still wins over it.
func TestVertexNative_EndpointHostAndOverride(t *testing.T) {
	for _, tc := range []struct {
		name, endpoint, override, wantHost string
	}{
		{"regional", "p:us-central1", "", "us-central1-aiplatform.googleapis.com"},
		{"multi-regional", "p:eu", "", "aiplatform.eu.rep.googleapis.com"},
		{"global", "p:global", "", "aiplatform.googleapis.com"},
		{"override wins", "p:us-central1", "https://europe-west4-aiplatform.googleapis.com", "europe-west4-aiplatform.googleapis.com"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newFailoverHarness(t, serveVertex, serveOpenAIText("never"), func(primary, fallback *models.LLM) {
				vertexPrimary(vertexTestToken)(primary, fallback)
				primary.APIEndpoint = tc.endpoint
			})
			host := routeUpstreamTo(t, h.primaryVendor.server.URL)
			if tc.override != "" {
				h.proxy.SetAuthHooks(overridePostAuth(tc.override))
			}
			for _, route := range []string{"/llm/rest/primary" + vertexNativePath, "/llm/stream/primary" + vertexStreamPath + "?alt=sse"} {
				resp, body := h.post(route, vertexNativeBody)
				require.Equal(t, http.StatusOK, resp.StatusCode, "%s: %s", route, body)
				assert.Equal(t, tc.wantHost, host(), route)
			}
		})
	}
}

// A URL endpoint carrying the project and location in its path sends the
// caller's Vertex path to the URL's host, without repeating that path.
func TestVertexNative_URLEndpointWithProjectLocation(t *testing.T) {
	t.Parallel()
	h := newFailoverHarness(t, serveVertex, serveOpenAIText("never"), vertexPrimary(vertexTestToken))
	h.primary.APIEndpoint = h.primaryVendor.server.URL + "/v1/projects/p/locations/us-central1/"
	require.NoError(t, h.db.Save(h.primary).Error)
	require.NoError(t, h.proxy.loadResources())

	resp, body := h.post("/llm/rest/primary"+vertexNativePath, vertexNativeBody)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	calls := h.primaryVendor.calls()
	require.Len(t, calls, 1)
	assert.Equal(t, vertexNativePath, calls[0].Path)
}

// failingTokenSource is a credential whose token request fails (a revoked
// key, an unreachable token endpoint).
type failingTokenSource struct{}

func (failingTokenSource) Token() (*oauth2.Token, error) {
	return nil, errors.New("oauth2: cannot fetch token: 400 invalid_grant")
}

// A token that cannot be minted answers 502 on both routes, and the request
// is never sent upstream unauthenticated (auth is set at RoundTrip).
func TestVertexNative_TokenMintFailure(t *testing.T) {
	requireSerial(t)
	restore := vertexVendor.SetTokenSourceOverride(func(*models.LLM) (oauth2.TokenSource, error) {
		return failingTokenSource{}, nil
	})
	t.Cleanup(restore)
	h := newFailoverHarness(t, serveVertex, serveOpenAIText("never"), vertexPrimary(`{"type":"service_account"}`))

	for _, path := range []string{"/llm/rest/primary" + vertexNativePath, "/llm/stream/primary" + vertexStreamPath + "?alt=sse"} {
		resp, body := h.post(path, vertexNativeBody)
		assert.Equal(t, http.StatusBadGateway, resp.StatusCode, "%s: %s", path, body)
		assert.NotContains(t, string(body), "invalid_grant", "the token endpoint's detail is logged, not returned")
	}
	assert.Empty(t, h.primaryVendor.calls(), "nothing reached the upstream")
}

// A Vertex LLM with no key (and host ADC not enabled) is refused with the
// reason, never sent upstream unauthenticated.
func TestVertexNative_EmptyKeyRefused(t *testing.T) {
	t.Parallel()
	h := newFailoverHarness(t, serveVertex, serveOpenAIText("never"), vertexPrimary(""))

	for _, path := range []string{"/llm/rest/primary" + vertexNativePath, "/llm/stream/primary" + vertexStreamPath + "?alt=sse"} {
		resp, body := h.post(path, vertexNativeBody)
		assert.GreaterOrEqual(t, resp.StatusCode, 400, "%s: %s", path, body)
		assert.Contains(t, string(body), vertexVendor.ADCSetting, path)
	}
	assert.Empty(t, h.primaryVendor.calls(), "nothing reached the upstream")
}

// The stream's query reaches the upstream: without alt=sse Vertex answers a
// JSON array, not the SSE the client asked for.
func TestVertexNative_StreamQueryForwarded(t *testing.T) {
	t.Parallel()
	queries := make(chan string, 1)
	serve := func(w http.ResponseWriter, r *http.Request) {
		queries <- r.URL.RawQuery
		serveVertex(w, r)
	}
	h := newFailoverHarness(t, serve, serveOpenAIText("never"), vertexPrimary(vertexTestToken))

	resp, body := h.post("/llm/stream/primary"+vertexStreamPath+"?alt=sse", vertexNativeBody)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	assert.Equal(t, "alt=sse", <-queries)
}

// A rotated key is used from the next request after the LLM reloads: the
// proxy keeps no auth of its own past a reload.
func TestVertexNative_KeyRotation(t *testing.T) {
	t.Parallel()
	h := newFailoverHarness(t, serveVertex, serveOpenAIText("never"), vertexPrimary("ya29.first"))

	resp, body := h.post("/llm/rest/primary"+vertexNativePath, vertexNativeBody)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))

	h.primary.APIKey = "ya29.second"
	require.NoError(t, h.db.Save(h.primary).Error)
	require.NoError(t, h.proxy.loadResources())
	for _, path := range []string{"/llm/rest/primary" + vertexNativePath, "/llm/stream/primary" + vertexStreamPath + "?alt=sse"} {
		resp, body = h.post(path, vertexNativeBody)
		require.Equal(t, http.StatusOK, resp.StatusCode, "%s: %s", path, body)
	}

	calls := h.primaryVendor.calls()
	require.Len(t, calls, 3)
	assert.Equal(t, "Bearer ya29.first", calls[0].Headers.Get("Authorization"))
	assert.Equal(t, "Bearer ya29.second", calls[1].Headers.Get("Authorization"))
	assert.Equal(t, "Bearer ya29.second", calls[2].Headers.Get("Authorization"))
}

// Google credentials a caller sends are not forwarded: the LLM's token
// replaces a caller's Authorization, and a Gemini API key (header or key=)
// is dropped, so the upstream sees exactly one credential, the LLM's.
func TestVertexNative_CallerGoogleCredentialsDropped(t *testing.T) {
	t.Parallel()
	queries := make(chan string, 2)
	serve := func(w http.ResponseWriter, r *http.Request) {
		queries <- r.URL.RawQuery
		serveVertex(w, r)
	}
	h := newFailoverHarness(t, serve, serveOpenAIText("never"), vertexPrimary(vertexTestToken))

	for _, path := range []string{"/llm/rest/primary" + vertexNativePath + "?key=caller-key", "/llm/stream/primary" + vertexStreamPath + "?alt=sse&key=caller-key"} {
		resp, body := h.post(path, vertexNativeBody, "x-goog-api-key", "caller-key")
		require.Equal(t, http.StatusOK, resp.StatusCode, "%s: %s", path, body)
		assert.NotContains(t, <-queries, "key=", path)
	}
	for _, c := range h.primaryVendor.calls() {
		assert.Equal(t, []string{"Bearer " + vertexTestToken}, c.Headers.Values("Authorization"), c.Path)
		assert.Empty(t, c.Headers.Get("x-goog-api-key"), c.Path)
	}
}
