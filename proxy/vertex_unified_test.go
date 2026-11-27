package proxy

import (
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"

	"github.com/TykTechnologies/midsommar/v2/models"
	vertexVendor "github.com/TykTechnologies/midsommar/v2/vendors/vertex"
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

	// Two chunks, as Vertex streams them: the usage counts come only on the
	// last; the first carries a usageMetadata with just the (Vertex-only)
	// trafficType. Enums are numbers when the client asks for them.
	numeric := strings.Contains(r.URL.Query().Get("$alt"), "enum-encoding=int")
	finish, traffic, modality := `"STOP"`, `"ON_DEMAND"`, `"TEXT"`
	if numeric {
		finish, traffic, modality = "1", "1", "1"
	}
	first, last := `{"text":"hello "}`, `{"text":"from vertex"}`
	if _, ok := body["tools"]; ok {
		first, last = `{"text":""}`, `{"functionCall":{"name":"get_time","args":{"zone":"UTC"}}}`
	}
	chunk0 := `{"candidates":[{"content":{"role":"model","parts":[` + first + `]}}],` +
		`"usageMetadata":{"trafficType":` + traffic + `},"modelVersion":"gemini-2.5-flash"}`
	chunk1 := `{"candidates":[{"content":{"role":"model","parts":[` + last + `]},"finishReason":` + finish + `}],` +
		`"usageMetadata":{"promptTokenCount":9,"candidatesTokenCount":4,"totalTokenCount":13,"trafficType":` + traffic +
		`,"promptTokensDetails":[{"modality":` + modality + `,"tokenCount":9}]},"modelVersion":"gemini-2.5-flash"}`
	chunk := `{"candidates":[{"content":{"role":"model","parts":[{"text":"hello "},` + last + `]},"finishReason":` + finish + `}],` +
		`"usageMetadata":{"promptTokenCount":9,"candidatesTokenCount":4,"totalTokenCount":13},"modelVersion":"gemini-2.5-flash"}`
	switch {
	case strings.Contains(r.URL.Path, ":streamGenerateContent") && r.URL.Query().Get("alt") == "sse":
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: " + chunk0 + "\r\n\r\ndata: " + chunk1 + "\r\n\r\n"))
		return
	case strings.Contains(r.URL.Path, ":streamGenerateContent"):
		// Without alt=sse Vertex streams a JSON array.
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("[" + chunk0 + "\n,\r\n" + chunk1 + "\n]"))
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
	assert.Contains(t, string(out), `"content":"hello "`)
	assert.Contains(t, string(out), `"content":"from vertex"`)
	assert.Contains(t, string(out), `"usage":{"prompt_tokens":9,"completion_tokens":4,"total_tokens":13}`, "the stream's usage")
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
		assert.Contains(t, string(out), "from vertex")
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

// openAIError decodes an OpenAI error envelope.
func openAIError(t *testing.T, body []byte) (message, typ string) {
	t.Helper()
	var env struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(body, &env), string(body))
	require.NotEmpty(t, env.Error.Message, string(body))
	return env.Error.Message, env.Error.Type
}

// A credential that cannot be used answers the bridge's own error, with
// nothing sent upstream: 502 when the token cannot be minted, and a refusal
// naming the fix when the LLM has no key.
func TestVertexUnified_CredentialFailures(t *testing.T) {
	requireSerial(t)
	for _, tc := range []struct {
		name, key  string
		override   bool
		wantStatus int
		wantText   string
	}{
		{"mint failure", `{"type":"service_account"}`, true, http.StatusBadGateway, ""},
		{"no key", "", false, http.StatusBadGateway, vertexVendor.ADCSetting},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.override {
				t.Cleanup(vertexVendor.SetTokenSourceOverride(func(*models.LLM) (oauth2.TokenSource, error) {
					return failingTokenSource{}, nil
				}))
			}
			fake := &fakeVertex{}
			h := newFailoverHarness(t, fake.handler, serveOpenAIText("never"), func(primary, _ *models.LLM) {
				primary.Vendor = models.VERTEX
				primary.APIEndpoint += vertexEndpointSuffix
				primary.APIKey = tc.key
				primary.DefaultModel = "gemini-2.5-flash"
				primary.Failover = models.LLMFailover{}
			})
			for _, body := range []string{failoverChatBody, failoverStreamBody} {
				resp, out := h.post("/ai/primary/v1/chat/completions", strings.Replace(body, "gpt-4o", "gemini-2.5-flash", 1))
				if tc.wantStatus != 0 {
					assert.Equal(t, tc.wantStatus, resp.StatusCode, string(out))
				}
				assert.GreaterOrEqual(t, resp.StatusCode, 400, string(out))
				assert.NotEqual(t, http.StatusInternalServerError, resp.StatusCode, string(out))
				msg, _ := openAIError(t, out)
				assert.NotContains(t, msg, "invalid_grant", "no token-endpoint detail")
				if tc.wantText != "" {
					assert.Contains(t, msg, tc.wantText)
				}
			}
			assert.Empty(t, fake.snapshot(), "nothing reached the upstream")
		})
	}
}

// A model outside the LLM's allowed list is refused on /ai, and nothing
// reaches the upstream: the inner hop validates the mapped Vertex path.
func TestVertexUnified_AllowedModels(t *testing.T) {
	t.Parallel()
	fake := &fakeVertex{}
	h := newFailoverHarness(t, fake.handler, serveOpenAIText("never"), func(primary, _ *models.LLM) {
		primary.Vendor = models.VERTEX
		primary.APIEndpoint += vertexEndpointSuffix
		primary.APIKey = vertexTestToken
		primary.DefaultModel = "gemini-2.5-flash"
		primary.AllowedModels = []string{"^gemini-2.5-flash$"}
		primary.Failover = models.LLMFailover{}
	})
	for _, stream := range []string{"false", "true"} {
		resp, out := h.post("/ai/primary/v1/chat/completions", `{"model":"gemini-2.5-pro","stream":`+stream+`,"messages":[{"role":"user","content":"hi"}]}`)
		assert.Equal(t, http.StatusForbidden, resp.StatusCode, string(out))
		openAIError(t, out)
	}
	assert.Empty(t, fake.snapshot())

	resp, out := h.post("/ai/primary/v1/chat/completions", `{"model":"gemini-2.5-flash","messages":[{"role":"user","content":"hi"}]}`)
	assert.Equal(t, http.StatusOK, resp.StatusCode, string(out))
}

// A bare URL endpoint names no project or location: /ai and /v1 answer 400
// with the accepted forms, and nothing reaches the upstream.
func TestVertexUnified_BareURLEndpoint(t *testing.T) {
	t.Parallel()
	fake := &fakeVertex{}
	h := newFailoverHarness(t, fake.handler, serveOpenAIText("never"), func(primary, _ *models.LLM) {
		primary.Vendor = models.VERTEX
		primary.APIKey = vertexTestToken
		primary.DefaultModel = "gemini-2.5-flash"
		primary.Failover = models.LLMFailover{}
	})
	for _, route := range []struct{ path, model string }{{"/ai/primary/v1/chat/completions", "gemini-2.5-flash"}, {"/v1/chat/completions", "primary/gemini-2.5-flash"}} {
		resp, out := h.post(route.path, `{"model":"`+route.model+`","messages":[{"role":"user","content":"hi"}]}`)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "%s: %s", route.path, out)
		msg, _ := openAIError(t, out)
		assert.Contains(t, msg, "project:location", route.path)
		assert.Contains(t, msg, "/projects/{project}/locations/{location}", route.path)
	}
	assert.Empty(t, fake.snapshot())
}

// A Vertex primary that answers 429 or 5xx fails over to the next rung; with
// no rung, a 429 reaches the caller as a 429 (SDKs retry on it).
func TestVertexUnified_FailoverFromVertex(t *testing.T) {
	t.Parallel()
	for _, status := range []int{http.StatusTooManyRequests, http.StatusServiceUnavailable} {
		h := newFailoverHarness(t, serveStatus(status), serveOpenAIText("from the fallback"), func(primary, _ *models.LLM) {
			primary.Vendor = models.VERTEX
			primary.APIEndpoint += vertexEndpointSuffix
			primary.APIKey = vertexTestToken
			primary.DefaultModel = "gemini-2.5-flash"
		})
		for _, body := range []string{failoverChatBody, failoverStreamBody} {
			resp, out := h.post("/ai/primary/v1/chat/completions", strings.Replace(body, "gpt-4o", "gemini-2.5-flash", 1))
			require.Equal(t, http.StatusOK, resp.StatusCode, "%d: %s", status, out)
			assert.Contains(t, string(out), "fallback")
			assert.Equal(t, "fallback", resp.Header.Get(hdrServedLLM))
		}
		assert.NotEmpty(t, h.primaryVendor.calls(), "the Vertex rung was tried")
	}

	h := newFailoverHarness(t, serveStatus(http.StatusTooManyRequests), serveOpenAIText("never"), func(primary, _ *models.LLM) {
		primary.Vendor = models.VERTEX
		primary.APIEndpoint += vertexEndpointSuffix
		primary.APIKey = vertexTestToken
		primary.DefaultModel = "gemini-2.5-flash"
		primary.Failover = models.LLMFailover{}
	})
	for _, body := range []string{failoverChatBody, failoverStreamBody} {
		resp, out := h.post("/ai/primary/v1/chat/completions", strings.Replace(body, "gpt-4o", "gemini-2.5-flash", 1))
		assert.Equal(t, http.StatusTooManyRequests, resp.StatusCode, string(out))
	}
}

// The request's model, not the LLM's default, is the one in the upstream path.
func TestVertexUnified_RequestModel(t *testing.T) {
	t.Parallel()
	h, fake := vertexUnifiedHarness(t)
	resp, out := h.post("/ai/primary/v1/chat/completions", `{"model":"gemini-2.5-pro","messages":[{"role":"user","content":"hi"}]}`)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(out))
	calls := fake.snapshot()
	require.Len(t, calls, 1)
	assert.Equal(t, strings.Replace(vertexLoopbackPath, "gemini-2.5-flash", "gemini-2.5-pro", 1), calls[0].path)
}

// A response recorded from Vertex on the loopback's query (candidate text
// and thought signature replaced): two chunks, the first with a
// usageMetadata of only trafficType, the counts on the last; gzip-encoded
// when the client accepts it, as Vertex sends it. Both the caller's usage
// and the chat record carry the counts.
func TestVertexUnified_RecordedVertexStream(t *testing.T) {
	t.Parallel()
	recorded, err := os.ReadFile("testdata/vertex_stream_live.json")
	require.NoError(t, err)
	serve := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=UTF-8")
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			_, _ = w.Write(recorded)
			return
		}
		w.Header().Set("Content-Encoding", "gzip")
		gz := gzip.NewWriter(w)
		_, _ = gz.Write(recorded)
		_ = gz.Close()
	}
	h := newFailoverHarness(t, serve, serveOpenAIText("never"), func(primary, _ *models.LLM) {
		primary.Vendor = models.VERTEX
		primary.APIEndpoint += vertexEndpointSuffix
		primary.APIKey = vertexTestToken
		primary.DefaultModel = "gemini-3.5-flash"
		primary.Failover = models.LLMFailover{}
	})

	resp, out := h.post("/ai/primary/v1/chat/completions", `{"model":"gemini-3.5-flash","messages":[{"role":"user","content":"Reply with the single word: pong"}],"max_tokens":256}`)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(out))
	var got ChatCompletionResponse
	require.NoError(t, json.Unmarshal(out, &got), string(out))
	require.Len(t, got.Choices, 1)
	assert.Equal(t, "x", got.Choices[0].Message.Content)
	assert.Equal(t, 7, got.Usage.PromptTokens, string(out))
	assert.Equal(t, 1, got.Usage.CompletionTokens, string(out))

	recs := waitForChatRecords(t, h)
	assert.Equal(t, 7, recs[0].PromptTokens)
	assert.Equal(t, 96, recs[0].ResponseTokens, "analytics count the thinking tokens with the output")
}
