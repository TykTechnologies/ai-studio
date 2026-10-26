package proxy

import (
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// postAuthSeen records the X-Tyk-Compression header each request had when
// the post-auth hook (where gateway plugins run) saw it, by path.
type postAuthSeen struct {
	mu   sync.Mutex
	seen map[string][]string
}

func (s *postAuthSeen) hook(_ http.ResponseWriter, r *http.Request, _ uint) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.seen == nil {
		s.seen = map[string][]string{}
	}
	s.seen[r.URL.Path] = r.Header.Values(hdrCompression)
	return false
}

// innerHop is what the post-auth hook saw on the /llm/call/ loopback hop.
func (s *postAuthSeen) innerHop(t *testing.T) []string {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	for p, v := range s.seen {
		if strings.HasPrefix(p, "/llm/call/") {
			return v
		}
	}
	t.Fatalf("no post-auth call on the /llm/call/ hop; saw %v", s.seen)
	return nil
}

func (s *postAuthSeen) at(path string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.seen[path]
}

// X-Tyk-Compression is an instruction to the gateway's plugins: the plugin
// sees it (on the native routes directly, on the bridges on the /llm/call/
// hop), and no vendor ever does, on any route.
func TestCompressionHeader_ReachesPluginsNeverTheVendor(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, path, body string
		viaLoopback      bool
		vendor           func(h *routerHarness) *fakeVendor
	}{
		{"llm rest", "/llm/rest/big/v1/chat/completions", chatBody("gpt-4o", false), false, func(h *routerHarness) *fakeVendor { return h.bigV }},
		{"llm stream", "/llm/stream/big/v1/chat/completions", chatBody("gpt-4o", true), false, func(h *routerHarness) *fakeVendor { return h.bigV }},
		{"ai", "/ai/big/v1/chat/completions", chatBody("gpt-4o", false), true, func(h *routerHarness) *fakeVendor { return h.bigV }},
		{"ai stream", "/ai/big/v1/chat/completions", chatBody("gpt-4o", true), true, func(h *routerHarness) *fakeVendor { return h.bigV }},
		{"model router", "/ai/smart/v1/chat/completions", chatBody("fast", false), true, func(h *routerHarness) *fakeVendor { return h.fastV }},
		{"unified v1", "/v1/chat/completions", chatBody("smart/big", false), true, func(h *routerHarness) *fakeVendor { return h.bigV }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			seen := &postAuthSeen{}
			h := newRouterHarness(t, routerHarnessOpts{
				grantRouter: true,
				directLLMs:  func(h *routerHarness) []uint { return []uint{h.big.ID} },
				authHooks:   &AuthHooks{PostAuth: seen.hook},
			})

			resp, body := h.do(http.MethodPost, tc.path, tc.body,
				"X-Tyk-Compression", "off", "X-Tyk-Compression-Debug", "1")
			require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)

			if tc.viaLoopback {
				assert.Equal(t, []string{"off"}, seen.innerHop(t), "the plugin on the inner hop sees the client's header")
			} else {
				assert.Equal(t, []string{"off"}, seen.at(tc.path))
			}
			calls := tc.vendor(h).calls()
			require.Len(t, calls, 1)
			for k := range calls[0].Headers {
				assert.False(t, isCompressionHeader(k), "%s reached the vendor", k)
			}
		})
	}
}

// The decisions bridge makes its own loopback request: the header reaches
// the plugin on the inner hop there too, and not the vendor.
func TestCompressionHeader_DecisionsBridge(t *testing.T) {
	t.Parallel()
	h := newFailoverHarness(t, serveOpenAIDecision(), serveStatus(500), noFailover)
	seen := &postAuthSeen{}
	h.proxy.SetAuthHooks(&AuthHooks{PostAuth: seen.hook})

	resp, out := h.post("/v1/decisions", unifiedDecisionBody(t, "primary/gpt-6-luna"),
		"X-Tyk-Compression", "off", "X-Tyk-Compression-Debug", "1")
	require.Equal(t, http.StatusOK, resp.StatusCode, string(out))

	assert.Equal(t, []string{"off"}, seen.innerHop(t))
	calls := h.primaryVendor.calls()
	require.Len(t, calls, 1)
	for k := range calls[0].Headers {
		assert.False(t, isCompressionHeader(k), "%s reached the vendor", k)
	}
}

func TestStripCompressionHeaders(t *testing.T) {
	h := http.Header{}
	h.Set("X-Tyk-Compression", "off")
	h.Set("X-Tyk-Compression-Policy", "max")
	h["x-tyk-compression"] = []string{"off"} // not canonicalised
	h.Set("X-Tyk-Compressionx", "kept")
	h.Set("Anthropic-Beta", "kept")

	stripCompressionHeaders(h)

	assert.Equal(t, http.Header{"X-Tyk-Compressionx": {"kept"}, "Anthropic-Beta": {"kept"}}, h)
}
