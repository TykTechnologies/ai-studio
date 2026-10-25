package proxy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/gosimple/slug"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// headerHook sets, changes or drops response headers in the header phase,
// the way a plugin's OnBeforeWriteHeaders does: it returns the full set.
type headerHook struct {
	set   map[string]string
	drop  []string
	delay time.Duration

	mu      sync.Mutex
	calls   int
	slugs   []string
	gotSent map[string]string
}

func (h *headerHook) GetName() string { return "header-hook" }

func (h *headerHook) OnBeforeWriteHeaders(ctx context.Context, req *HeadersRequest) (*HeadersResponse, error) {
	h.mu.Lock()
	h.calls++
	h.slugs = append(h.slugs, req.Context.LLMSlug)
	h.gotSent = req.Headers
	h.mu.Unlock()
	if h.delay > 0 {
		time.Sleep(h.delay) // a plugin that ignores the deadline
	}
	out := make(map[string]string, len(req.Headers)+len(h.set))
	for k, v := range req.Headers {
		out[k] = v
	}
	for _, k := range h.drop {
		delete(out, k)
	}
	for k, v := range h.set {
		out[k] = v
	}
	return &HeadersResponse{Modified: true, Headers: out}, nil
}

func (h *headerHook) OnBeforeWrite(ctx context.Context, req *ResponseWriteRequest) (*ResponseWriteResponse, error) {
	return &ResponseWriteResponse{Body: req.Body, Headers: req.Headers}, nil
}

func (h *headerHook) OnStreamComplete(ctx context.Context, req *StreamCompleteRequest) (*StreamCompleteResponse, error) {
	return &StreamCompleteResponse{}, nil
}

func (h *headerHook) seenSlugs() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), h.slugs...)
}

// serveStreamWithHeaders is serveOpenAIText with extra upstream headers, one
// of them repeated.
func serveStreamWithHeaders(text string) http.HandlerFunc {
	inner := serveOpenAIText(text)
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("X-Upstream-Multi", "one")
		w.Header().Add("X-Upstream-Multi", "two")
		w.Header().Set("X-Upstream-Drop", "gone")
		inner(w, r)
	}
}

// A response hook's header phase reaches streaming responses: headers it adds
// are sent, headers it drops are not, repeated headers it left alone keep
// every value, and the SSE framing headers do not change.
func TestStreamingResponse_RunsHeaderHooks(t *testing.T) {
	t.Parallel()
	h := newFailoverHarness(t, serveStreamWithHeaders("streamed answer"), serveOpenAIText("never"), nil)
	hook := &headerHook{
		set:  map[string]string{"X-Tyk-Compression": "applied", "X-Tyk-Compression-Saved-Tokens": "120", "Content-Type": "text/plain"},
		drop: []string{"X-Upstream-Drop"},
	}
	h.proxy.AddResponseHook(hook)

	resp, body := h.post("/llm/stream/primary/v1/chat/completions", failoverStreamBody)
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)
	assert.Contains(t, string(body), "data: ")
	assert.Contains(t, string(body), "[DONE]")
	assert.Equal(t, "applied", resp.Header.Get("X-Tyk-Compression"))
	assert.Equal(t, "120", resp.Header.Get("X-Tyk-Compression-Saved-Tokens"))
	assert.True(t, strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream"), "Content-Type = %q", resp.Header.Get("Content-Type"))
	assert.Empty(t, resp.Header.Get("X-Upstream-Drop"))
	assert.Equal(t, []string{"one", "two"}, resp.Header.Values("X-Upstream-Multi"))
	assert.Equal(t, []string{"primary"}, hook.seenSlugs(), "the hook gets the route slug")
}

// A header hook that does not answer in time is cut off: the stream goes out
// with the upstream headers and is not held.
func TestStreamingResponse_SlowHeaderHookIsCutOff(t *testing.T) {
	old := streamHeaderHookTimeout
	streamHeaderHookTimeout = 100 * time.Millisecond
	t.Cleanup(func() { streamHeaderHookTimeout = old })

	h := newFailoverHarness(t, serveOpenAIText("streamed answer"), serveOpenAIText("never"), nil)
	hook := &headerHook{set: map[string]string{"X-Tyk-Compression": "late"}, delay: 2 * time.Second}
	h.proxy.AddResponseHook(hook)

	start := time.Now()
	resp, body := h.post("/llm/stream/primary/v1/chat/completions", failoverStreamBody)
	elapsed := time.Since(start)
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)
	assert.Contains(t, string(body), "[DONE]")
	assert.Empty(t, resp.Header.Get("X-Tyk-Compression"), "a late answer is not applied")
	assert.Less(t, elapsed, 1500*time.Millisecond, "the stream waited for the slow hook")
}

// The header a plugin sets on the inner /llm/call/ hop reaches /ai/ and
// unified /v1 clients, buffered and streaming; other X-Tyk- headers do not.
func TestLoopbackRelay_CompressionHeadersReachOpenAICompatibleClients(t *testing.T) {
	t.Parallel()
	cases := []struct{ name, path, body string }{
		{"ai endpoint", "/ai/primary/v1/chat/completions", failoverChatBody},
		{"ai endpoint streaming", "/ai/primary/v1/chat/completions", failoverStreamBody},
		{"unified ingress streaming", "/v1/chat/completions",
			`{"model":"primary/gpt-4o","stream":true,"messages":[{"role":"user","content":"Say this is a test!"}]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newFailoverHarness(t, serveOpenAIText("answer"), serveOpenAIText("never"), nil)
			h.proxy.AddResponseHook(&headerHook{set: map[string]string{
				"X-Tyk-Compression":              "applied",
				"X-Tyk-Compression-Saved-Tokens": "42",
				"X-Tyk-Private":                  "not relayed",
			}})
			resp, body := h.post(tc.path, tc.body)
			require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)
			assert.Equal(t, "applied", resp.Header.Get("X-Tyk-Compression"))
			assert.Equal(t, "42", resp.Header.Get("X-Tyk-Compression-Saved-Tokens"))
			assert.Empty(t, resp.Header.Get("X-Tyk-Private"))
		})
	}
}

// Buffered response hooks get the route slug too, not the LLM's display name.
func TestBufferedResponse_HookGetsSlug(t *testing.T) {
	manager := NewDefaultResponseHookManager()
	hook := &headerHook{set: map[string]string{"X-Tyk-Compression": "applied"}}
	manager.AddHook(hook)
	p := &Proxy{responseHookManager: manager}

	r := httptest.NewRequest(http.MethodPost, "/llm/rest/my-route-llm/v1/chat/completions", strings.NewReader("{}"))
	r = r.WithContext(context.WithValue(r.Context(), "request_id", "req-1"))
	capture := newBufferedResponseCapture(httptest.NewRecorder())
	capture.WriteHeader(http.StatusOK)
	_, _ = capture.Write([]byte(`{}`))

	llm := &models.LLM{ID: 3, Name: "My Route LLM", Vendor: models.OPENAI}
	require.NoError(t, p.executeBufferedResponseHooks(capture, llm, &models.App{ID: 1}, r))
	assert.Equal(t, []string{"my-route-llm"}, hook.seenSlugs())
	assert.Equal(t, slug.Make(llm.Name), hook.seenSlugs()[0])
}
