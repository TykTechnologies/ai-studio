package integration

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/pkg/netguard"
	"github.com/TykTechnologies/midsommar/v2/pkg/netguard/netguardtest"
	"github.com/stretchr/testify/assert"
)

// The upstreams in these tests are httptest servers on 127.0.0.1, and
// internal-range blocking is on by default: turn it off for the package.
// TestEdgeRefusesInternalUpstream turns it back on.
func init() { netguardtest.AllowInternalForPackage() }

// Each process enforces its own LLM_UPSTREAM_BLOCK_INTERNAL (sf5): with
// blocking on at the edge, an LLM whose upstream the hub configured on an
// internal address (stored in the edge database, as edge sync writes it) is
// refused at the edge and the upstream is never reached.
func TestEdgeRefusesInternalUpstream(t *testing.T) {
	netguardtest.BlockInternal(t)
	t.Setenv(netguard.EnvAllowInternal, "")
	t.Setenv(netguard.EnvAllowedHosts, "")
	t.Setenv(netguard.EnvAllowedInternalHosts, "")

	var hits atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"c","object":"chat.completion","model":"gpt-4","choices":[]}`))
	}))
	t.Cleanup(upstream.Close)
	baseURL := setupStreamingMetricsTest(t, upstream.URL)

	status, body := gatewayPost(t, baseURL+"/llm/rest/sse-model/v1/chat/completions", streamingTestToken,
		`{"model":"gpt-4","messages":[{"role":"user","content":"hello"}]}`)
	assert.NotEqual(t, http.StatusOK, status, "body: %s", body)
	assert.Zero(t, hits.Load(), "the edge reached an internal upstream")
}
