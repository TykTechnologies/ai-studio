package proxy

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// lsj.26: the cache and load-balancer plugins set X-Cache-Status and
// X-LB-Upstream/X-LB-Pool in the header phase (OnBeforeWriteHeaders) as well
// as on a buffered body, because the buffered /ai and /v1 bridges stream
// their inner hop and a stream has no body phase. Header-phase headers reach
// the client on every route, buffered and streamed, to a non-Bedrock vendor.
func TestLoopbackRelay_HeaderPhasePluginHeadersOnEveryRoute(t *testing.T) {
	set := map[string]string{"X-Cache-Status": "MISS", "X-LB-Upstream": "oa-1", "X-LB-Pool": "oa"}
	routes := map[string]struct{ path, body string }{
		"direct/buffered": {"/llm/call/primary/v1/chat/completions", failoverChatBody},
		"direct/stream":   {"/llm/call/primary/v1/chat/completions", failoverStreamBody},
		"ai/buffered":     {"/ai/primary/v1/chat/completions", failoverChatBody},
		"ai/stream":       {"/ai/primary/v1/chat/completions", failoverStreamBody},
		"v1/buffered":     {"/v1/chat/completions", `{"model":"primary/gpt-4o","messages":[{"role":"user","content":"hi"}]}`},
		"v1/stream":       {"/v1/chat/completions", `{"model":"primary/gpt-4o","stream":true,"messages":[{"role":"user","content":"hi"}]}`},
	}
	for name, r := range routes {
		t.Run(name, func(t *testing.T) {
			h := newFailoverHarness(t, serveOpenAIText("answer"), serveOpenAIText("never"), nil)
			h.proxy.AddResponseHook(&headerHook{set: set})
			resp, out := h.post(r.path, r.body)
			require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", out)
			for k, v := range set {
				assert.Equal(t, v, resp.Header.Get(k), k)
			}
		})
	}
}
