package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// bodyRecorder wraps a fake vendor and keeps the last request body it saw.
type bodyRecorder struct {
	t    *testing.T
	mu   sync.Mutex
	last map[string]any
}

func newBodyRecorder(t *testing.T) *bodyRecorder {
	return &bodyRecorder{t: t}
}

// wrap records each request body before serve answers it. The handler runs on
// the server's goroutine, where require (t.FailNow) must not be called, so an
// unreadable body fails the test with assert and answers 400.
func (b *bodyRecorder) wrap(serve func(w http.ResponseWriter, r *http.Request)) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if !assert.NoError(b.t, err, "read upstream request body") {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var body map[string]any
		if err := json.Unmarshal(raw, &body); !assert.NoError(b.t, err, "upstream request body is not JSON: %s", raw) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		b.mu.Lock()
		b.last = body
		b.mu.Unlock()
		r.Body = io.NopCloser(bytes.NewReader(raw))
		serve(w, r)
	}
}

func (b *bodyRecorder) body() map[string]any {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.last
}

// The OpenAI Chat Completions API has two names for the output cap:
// max_tokens (still what most SDKs and clients send) and max_completion_tokens.
// The Main Ingress and the /ai/ bridge must honour either, whatever the
// upstream vendor, and max_completion_tokens wins when a client sends both.
func TestBridge_HonoursMaxTokensAndMaxCompletionTokens(t *testing.T) {
	// Distinct values, and neither is a driver default (Anthropic's is 2048),
	// so the upstream body shows which field the cap came from.
	const (
		maxTokens           = 37
		maxCompletionTokens = 41
	)
	vendors := []struct {
		name   string
		vendor models.Vendor
		serve  func(w http.ResponseWriter, r *http.Request)
		model  string
		field  string // the field the vendor receives the cap in
	}{
		{"openai", models.OPENAI, serveOpenAI, "gpt-4o", "max_completion_tokens"},
		{"anthropic", models.ANTHROPIC, serveAnthropic(t), "claude-sonnet-5", "max_tokens"},
	}
	cases := []struct {
		name   string
		limits string
		want   float64
	}{
		{"max_tokens", fmt.Sprintf(`"max_tokens":%d`, maxTokens), maxTokens},
		{"max_completion_tokens", fmt.Sprintf(`"max_completion_tokens":%d`, maxCompletionTokens), maxCompletionTokens},
		{"both: max_completion_tokens wins",
			fmt.Sprintf(`"max_tokens":%d,"max_completion_tokens":%d`, maxTokens, maxCompletionTokens), maxCompletionTokens},
	}
	for _, v := range vendors {
		t.Run(v.name, func(t *testing.T) {
			rec := newBodyRecorder(t)
			h := newEndpointShapeHarness(t, v.vendor, "", rec.wrap(v.serve))
			for _, c := range cases {
				t.Run(c.name, func(t *testing.T) {
					for _, stream := range []bool{false, true} {
						streamField := ""
						if stream {
							streamField = `"stream":true,`
						}
						resp, body := h.post("/v1/chat/completions",
							`{"model":"shape-route/`+v.model+`",`+streamField+c.limits+`,"messages":[{"role":"user","content":"hi"}]}`)
						require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)
						got := rec.body()
						require.NotNil(t, got)
						assert.Equal(t, c.want, got[v.field], "stream=%v upstream body: %v", stream, got)
						if v.field == "max_completion_tokens" {
							assert.NotContains(t, got, "max_tokens", "only one cap goes upstream")
						}
					}
				})
			}
		})
	}
}
