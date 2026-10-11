package proxy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// contextCapturingHook records the plugin context response hooks receive and
// changes nothing.
type contextCapturingHook struct {
	mu   sync.Mutex
	seen []PluginContext
}

func (h *contextCapturingHook) GetName() string { return "context-capture" }

func (h *contextCapturingHook) OnBeforeWriteHeaders(ctx context.Context, req *HeadersRequest) (*HeadersResponse, error) {
	h.mu.Lock()
	h.seen = append(h.seen, *req.Context)
	h.mu.Unlock()
	return &HeadersResponse{Headers: req.Headers}, nil
}

func (h *contextCapturingHook) OnBeforeWrite(ctx context.Context, req *ResponseWriteRequest) (*ResponseWriteResponse, error) {
	return &ResponseWriteResponse{Body: req.Body, Headers: req.Headers}, nil
}

func (h *contextCapturingHook) OnStreamComplete(ctx context.Context, req *StreamCompleteRequest) (*StreamCompleteResponse, error) {
	return &StreamCompleteResponse{}, nil
}

// Response hooks (gateway response plugins, run on the edge) learn the
// vendor and the request kind. The vendor used to reach edge plugins empty,
// so a plugin keyed on it now sees a value where it saw "".
func TestResponseHooks_SeeVendorAndRequestKind(t *testing.T) {
	cases := []struct {
		name   string
		vendor models.Vendor
		path   string
		kind   string
	}{
		{"decision", models.SYSTEMONE, "/llm/rest/jev/v1/systemone", "decision"},
		{"openai decision", models.OPENAI, "/llm/rest/oai/v1/decisions", "decision"},
		{"chat", models.OPENAI, "/llm/rest/oai/v1/chat/completions", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			manager := NewDefaultResponseHookManager()
			hook := &contextCapturingHook{}
			manager.AddHook(hook)
			p := &Proxy{responseHookManager: manager}

			r := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader("{}"))
			r = r.WithContext(context.WithValue(r.Context(), "request_id", "req-1"))
			capture := newBufferedResponseCapture(httptest.NewRecorder())
			capture.WriteHeader(http.StatusOK)
			_, _ = capture.Write([]byte(`{}`))

			llm := &models.LLM{ID: 3, Name: "route", Vendor: tc.vendor}
			require.NoError(t, p.executeBufferedResponseHooks(capture, llm, &models.App{ID: 1}, r))

			require.Len(t, hook.seen, 1)
			got := hook.seen[0]
			assert.Equal(t, string(tc.vendor), got.Vendor)
			assert.Equal(t, tc.kind, got.RequestKind)
			assert.Equal(t, "req-1", got.RequestID)
		})
	}
}
