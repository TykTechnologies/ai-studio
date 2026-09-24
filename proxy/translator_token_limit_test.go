package proxy

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// bodyRecorder wraps a fake vendor and keeps the last request body it saw.
type bodyRecorder struct {
	mu   sync.Mutex
	last map[string]any
}

func (b *bodyRecorder) wrap(serve func(w http.ResponseWriter, r *http.Request)) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		b.mu.Lock()
		b.last = body
		b.mu.Unlock()
		r.Body = io.NopCloser(strings.NewReader(string(raw)))
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
		{"max_tokens", `"max_tokens":37`, 37},
		{"max_completion_tokens", `"max_completion_tokens":41`, 41},
		{"both: max_completion_tokens wins", `"max_tokens":37,"max_completion_tokens":41`, 41},
	}
	for _, v := range vendors {
		t.Run(v.name, func(t *testing.T) {
			rec := &bodyRecorder{}
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
