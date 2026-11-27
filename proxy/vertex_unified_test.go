package proxy

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// vertexEndpointSuffix makes a fake vendor's URL a Vertex endpoint naming the
// project and location, so Gemini API paths map to the Vertex path.
const vertexEndpointSuffix = "/v1/projects/p/locations/us-central1"

// vertexLoopbackPath and vertexLoopbackQuery are what the Google AI client
// asks for, buffered or streamed: it always streams, as a JSON array
// ($alt=json), with enums as numbers, and the proxy relays that to the Vertex
// path unchanged (Vertex's API takes the same system parameters).
const (
	vertexLoopbackPath  = "/v1/projects/p/locations/us-central1/publishers/google/models/gemini-2.5-flash:streamGenerateContent"
	vertexLoopbackQuery = "%24alt=json%3Benum-encoding%3Dint"
)

// vertexUpstreamCall is what the fake Vertex upstream saw.
type vertexUpstreamCall struct {
	path, query string
	header      http.Header
	body        map[string]any
}

// fakeVertex answers generateContent and streamGenerateContent (SSE with
// alt=sse, else a JSON array) as Vertex does: a text answer, or a function call when the request has tools.
type fakeVertex struct {
	mu    sync.Mutex
	calls []vertexUpstreamCall
}

func (f *fakeVertex) handler(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	var body map[string]any
	_ = json.Unmarshal(raw, &body)
	f.mu.Lock()
	f.calls = append(f.calls, vertexUpstreamCall{path: r.URL.Path, query: r.URL.RawQuery, header: r.Header.Clone(), body: body})
	f.mu.Unlock()

	part := `{"text":"hello from vertex"}`
	finish := "STOP"
	if _, ok := body["tools"]; ok {
		part = `{"functionCall":{"name":"get_time","args":{"zone":"UTC"}}}`
	}
	usage := `"usageMetadata":{"promptTokenCount":9,"candidatesTokenCount":4,"totalTokenCount":13},"modelVersion":"gemini-2.5-flash"`
	chunk := `{"candidates":[{"content":{"role":"model","parts":[` + part + `]},"finishReason":"` + finish + `"}],` + usage + `}`
	switch {
	case strings.Contains(r.URL.Path, ":streamGenerateContent") && r.URL.Query().Get("alt") == "sse":
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: " + chunk + "\r\n\r\n"))
		return
	case strings.Contains(r.URL.Path, ":streamGenerateContent"):
		// Without alt=sse Vertex streams a JSON array.
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("[" + chunk + "]"))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(chunk))
}

func (f *fakeVertex) snapshot() []vertexUpstreamCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]vertexUpstreamCall(nil), f.calls...)
}

// vertexUnifiedHarness is the failover harness with a Vertex primary behind
// a fake Vertex upstream.
func vertexUnifiedHarness(t *testing.T) (*failoverHarness, *fakeVertex) {
	fake := &fakeVertex{}
	h := newFailoverHarness(t, fake.handler, serveOpenAIText("never"), func(primary, _ *models.LLM) {
		primary.Vendor = models.VERTEX
		primary.APIEndpoint += vertexEndpointSuffix
		primary.APIKey = vertexTestToken
		primary.DefaultModel = "gemini-2.5-flash"
		primary.Failover = models.LLMFailover{}
	})
	return h, fake
}

// assertOnlyLLMCredential checks the upstream got exactly one credential, the
// LLM's token: not the caller's App credential, not the loopback's dummy key.
func assertOnlyLLMCredential(t *testing.T, h *failoverHarness, c vertexUpstreamCall, token string) {
	t.Helper()
	assert.Equal(t, []string{"Bearer " + token}, c.header.Values("Authorization"))
	assert.Empty(t, c.header.Values("X-Goog-Api-Key"))
	assert.NotContains(t, c.query, "key=")
	for name, values := range c.header {
		for _, v := range values {
			assert.NotContains(t, v, h.apiKey, "header %s carries the App credential", name)
			assert.NotContains(t, v, "internal-routing-dummy-key", "header %s carries the loopback's dummy key", name)
		}
	}
}

// Every content entry names its role: Vertex refuses one without.
func assertContentsHaveRoles(t *testing.T, body map[string]any) {
	t.Helper()
	contents, _ := body["contents"].([]any)
	require.NotEmpty(t, contents, "body: %v", body)
	for i, c := range contents {
		entry, _ := c.(map[string]any)
		assert.NotEmpty(t, entry["role"], "contents[%d] has no role: %v", i, entry)
	}
}

// /ai/{slug}/v1/chat/completions on a Vertex LLM: the Google AI client's
// request reaches the upstream at the Vertex path with the LLM's token, and
// the answer and usage come back OpenAI-shaped.
func TestVertexUnified_ChatCompletions(t *testing.T) {
	t.Parallel()
	h, fake := vertexUnifiedHarness(t)

	body := `{"model":"gemini-2.5-flash","messages":[{"role":"system","content":"be brief"},{"role":"user","content":"hi"},{"role":"assistant","content":"hello"},{"role":"user","content":"again"}]}`
	resp, out := h.post("/ai/primary/v1/chat/completions", body)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(out))

	var got ChatCompletionResponse
	require.NoError(t, json.Unmarshal(out, &got), string(out))
	require.Len(t, got.Choices, 1)
	assert.Equal(t, "hello from vertex", got.Choices[0].Message.Content)
	assert.Equal(t, 9, got.Usage.PromptTokens)
	assert.Equal(t, 4, got.Usage.CompletionTokens)

	calls := fake.snapshot()
	require.Len(t, calls, 1)
	assert.Equal(t, vertexLoopbackPath, calls[0].path)
	assertOnlyLLMCredential(t, h, calls[0], vertexTestToken)
	assertContentsHaveRoles(t, calls[0].body)

	recs := waitForChatRecords(t, h)
	assert.Equal(t, 9, recs[0].PromptTokens)
	assert.Equal(t, 4, recs[0].ResponseTokens)
}

// stream:true comes back as OpenAI chunks; the stream's usage is recorded.
func TestVertexUnified_ChatCompletionsStream(t *testing.T) {
	t.Parallel()
	h, fake := vertexUnifiedHarness(t)

	resp, out := h.post("/ai/primary/v1/chat/completions", `{"model":"gemini-2.5-flash","stream":true,"messages":[{"role":"user","content":"hi"}]}`)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(out))
	assert.Contains(t, resp.Header.Get("Content-Type"), "text/event-stream")
	assert.Contains(t, string(out), "hello from vertex")
	assert.Contains(t, string(out), "data: [DONE]")

	calls := fake.snapshot()
	require.Len(t, calls, 1)
	assert.Equal(t, vertexLoopbackPath, calls[0].path)
	assert.Equal(t, vertexLoopbackQuery, calls[0].query)
	assertOnlyLLMCredential(t, h, calls[0], vertexTestToken)
	assertContentsHaveRoles(t, calls[0].body)

	recs := waitForChatRecords(t, h)
	assert.Equal(t, 9, recs[0].PromptTokens)
	assert.Equal(t, 4, recs[0].ResponseTokens)
}

// A function call comes back as an OpenAI tool call, buffered and streamed.
func TestVertexUnified_ToolCalls(t *testing.T) {
	t.Parallel()
	h, fake := vertexUnifiedHarness(t)
	tools := `"tools":[{"type":"function","function":{"name":"get_time","parameters":{"type":"object","properties":{"zone":{"type":"string"}}}}}]`

	resp, out := h.post("/ai/primary/v1/chat/completions", `{"model":"gemini-2.5-flash","messages":[{"role":"user","content":"time?"}],`+tools+`}`)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(out))
	var got struct {
		Choices []struct {
			FinishReason string `json:"finish_reason"`
			Message      struct {
				ToolCalls []struct {
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
	}
	require.NoError(t, json.Unmarshal(out, &got), string(out))
	require.Len(t, got.Choices, 1)
	require.Len(t, got.Choices[0].Message.ToolCalls, 1, string(out))
	assert.Equal(t, "get_time", got.Choices[0].Message.ToolCalls[0].Function.Name)
	assert.JSONEq(t, `{"zone":"UTC"}`, got.Choices[0].Message.ToolCalls[0].Function.Arguments)

	resp, out = h.post("/ai/primary/v1/chat/completions", `{"model":"gemini-2.5-flash","stream":true,"messages":[{"role":"user","content":"time?"}],`+tools+`}`)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(out))
	assert.Contains(t, string(out), `"get_time"`)
	assert.Contains(t, string(out), "tool_calls")

	for _, c := range fake.snapshot() {
		assert.Contains(t, c.body, "tools", c.path)
	}
}

// The unified /v1 route addresses a Vertex LLM by slug/model.
func TestVertexUnified_UnifiedRoute(t *testing.T) {
	t.Parallel()
	h, fake := vertexUnifiedHarness(t)

	resp, out := h.post("/v1/chat/completions", `{"model":"primary/gemini-2.5-flash","messages":[{"role":"user","content":"hi"}]}`)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(out))
	assert.Contains(t, string(out), "hello from vertex")
	calls := fake.snapshot()
	require.Len(t, calls, 1)
	assert.Equal(t, vertexLoopbackPath, calls[0].path)
	assertOnlyLLMCredential(t, h, calls[0], vertexTestToken)
}

// An OpenAI primary that fails over to a Vertex fallback: the fallback's
// Vertex upstream serves the answer with the fallback's token.
func TestVertexUnified_FailoverOpenAIToVertex(t *testing.T) {
	t.Parallel()
	fake := &fakeVertex{}
	h := newFailoverHarness(t, serveStatus(http.StatusInternalServerError), fake.handler, func(primary, fallback *models.LLM) {
		fallback.Vendor = models.VERTEX
		fallback.APIEndpoint += vertexEndpointSuffix
		fallback.APIKey = "ya29.fallback-token"
		fallback.DefaultModel = "gemini-2.5-flash"
		primary.Failover.Targets[0].Model = "gemini-2.5-flash"
	})

	for _, body := range []string{failoverChatBody, failoverStreamBody} {
		resp, out := h.post("/ai/primary/v1/chat/completions", body)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(out))
		assert.Contains(t, string(out), "hello from vertex")
		assert.Equal(t, "fallback", resp.Header.Get(hdrServedLLM))
	}

	calls := fake.snapshot()
	require.Len(t, calls, 2)
	assert.Equal(t, vertexLoopbackPath, calls[0].path)
	assert.Equal(t, vertexLoopbackPath, calls[1].path)
	for _, c := range calls {
		assertOnlyLLMCredential(t, h, c, "ya29.fallback-token")
	}
	require.Eventually(t, func() bool { return len(h.primaryVendor.calls()) >= 2 }, 5*time.Second, 10*time.Millisecond)
}
