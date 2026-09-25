package proxy

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A cache plugin answers on the inner /llm/call/ hop and labels the response
// with X-Cache-* headers. The OpenAI-compatible endpoints (/ai/{slug}/v1 and
// the unified /v1 ingress) reach that hop through the SDK driver, which
// consumes the loopback response, so the labels have to be relayed or a hit
// is indistinguishable from a vendor answer (seen on the Main Ingress).
func withCacheHeaders(next http.HandlerFunc, kv ...string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		for i := 0; i+1 < len(kv); i += 2 {
			w.Header().Set(kv[i], kv[i+1])
		}
		next(w, r)
	}
}

func TestLoopbackRelay_CacheHeadersReachOpenAICompatibleClients(t *testing.T) {
	hit := withCacheHeaders(serveOpenAIText("cached answer"),
		"X-Cache-Status", "HIT",
		"X-Cache-Match", "semantic",
		"X-Cache-Similarity", "0.9712",
		"X-Vendor-Private", "not relayed",
	)
	cases := []struct {
		name, path, body string
	}{
		{"ai endpoint", "/ai/primary/v1/chat/completions", failoverChatBody},
		{"ai endpoint streaming", "/ai/primary/v1/chat/completions", failoverStreamBody},
		{"unified ingress", "/v1/chat/completions",
			`{"model":"primary/gpt-4o","messages":[{"role":"user","content":"Say this is a test!"}]}`},
		{"unified ingress streaming", "/v1/chat/completions",
			`{"model":"primary/gpt-4o","stream":true,"messages":[{"role":"user","content":"Say this is a test!"}]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newFailoverHarness(t, hit, serveOpenAIText("never"), nil)

			resp, body := h.post(tc.path, tc.body)
			require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)
			assert.Contains(t, string(body), "cached", "the cached answer is served")
			assert.Equal(t, "HIT", resp.Header.Get("X-Cache-Status"))
			assert.Equal(t, "semantic", resp.Header.Get("X-Cache-Match"))
			assert.Equal(t, "0.9712", resp.Header.Get("X-Cache-Similarity"))
			assert.Empty(t, resp.Header.Get("X-Vendor-Private"), "only cache headers are relayed")
			assert.Empty(t, h.fallbackVendor.calls())
		})
	}
}

// After a failover the cache headers describe the rung that served, never a
// failed one.
func TestLoopbackRelay_FailedRungHeadersDoNotLeak(t *testing.T) {
	failed := withCacheHeaders(serveStatus(503), "X-Cache-Status", "MISS")
	for _, body := range []string{failoverChatBody, failoverStreamBody} {
		h := newFailoverHarness(t, failed, serveOpenAIText("from the fallback"), nil)

		resp, out := h.post("/ai/primary/v1/chat/completions", body)
		require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", out)
		assert.Equal(t, "true", resp.Header.Get(hdrFailover))
		assert.Empty(t, resp.Header.Get("X-Cache-Status"))
	}
}

// A vendor or CDN can also send X-Cache-* headers on the inner hop, so the
// relay caps what it copies onto the client response.
func TestLoopbackRelay_CapsRelayedHeaders(t *testing.T) {
	src := http.Header{}
	for i := 0; i < 100; i++ {
		src.Add(fmt.Sprintf("X-Cache-H%03d", i), "v")
	}
	relay := &loopbackRelay{}
	relay.capture(src)
	dst := http.Header{}
	relay.applyTo(dst)
	assert.Len(t, dst, maxRelayedHeaderValues)
	assert.Equal(t, "v", dst.Get("X-Cache-H000"), "the cap keeps keys in sorted order")

	big := http.Header{}
	big.Set("X-Cache-A", strings.Repeat("a", maxRelayedHeaderBytes))
	big.Set("X-Cache-Status", "HIT")
	relay.capture(big)
	dst = http.Header{}
	relay.applyTo(dst)
	assert.Empty(t, dst.Get("X-Cache-A"), "an oversized value is dropped")
	assert.Equal(t, "HIT", dst.Get("X-Cache-Status"))
}

func TestIsRelayedLoopbackHeader(t *testing.T) {
	for key, want := range map[string]bool{
		"X-Cache":        true,
		"X-Cache-Status": true,
		"x-cache-ttl":    true,
		"X-Cache-":       false,
		"X-Cached":       false,
		"Content-Length": false,
		"X-Request-Id":   false,
	} {
		assert.Equal(t, want, isRelayedLoopbackHeader(key), key)
	}
}
