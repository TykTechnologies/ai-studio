package proxy

import (
	"net/http"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
	"github.com/TykTechnologies/midsommar/v2/models"
)

var registerPanickingProvider sync.Once

// panickingGuardrail is a request guardrail whose provider factory panics.
func panickingGuardrail() *models.Filter {
	registerPanickingProvider.Do(func() {
		guardrails.RegisterSpec(guardrails.Spec{
			Name: "proxy_test_panicking", DisplayName: "panics",
			Detectors: []guardrails.DetectorSpec{{Name: "x", Category: "other"}},
		})
		guardrails.RegisterFactory("proxy_test_panicking", func(guardrails.Config) (guardrails.Provider, error) {
			panic("boom: secret-tool-output")
		})
	})
	return &models.Filter{Name: "panicking", Kind: models.FilterKindGuardrail, Config: models.JSONMap{
		"provider": "proxy_test_panicking", "detectors": []any{map[string]any{"name": "x"}},
		"on_detect": guardrails.ActionLog, "fail_mode": guardrails.FailOpen,
	}}
}

// A guardrail that panics refuses the request, on the pass-through and on a
// batch item (whose filters run on worker goroutines no HTTP server
// recovers), and the gateway goes on serving.
func TestGuardrailPanic_RefusesAndSurvives(t *testing.T) {
	h := newFailoverHarness(t, serveOpenAI, serveStatus(500), func(primary, _ *models.LLM) {
		primary.Failover = models.LLMFailover{}
		primary.Filters = []*models.Filter{panickingGuardrail()}
	})
	for i := 0; i < 2; i++ {
		resp, out := h.post("/llm/rest/primary/v1/chat/completions", failoverChatBody)
		require.Equal(t, http.StatusBadRequest, resp.StatusCode, string(out))
		assert.NotContains(t, string(out), "secret-tool-output")
	}
	assert.Empty(t, h.primaryVendor.calls())

	n := newNativeHarness(t, nativeHarnessOpts{mutateLLM: func(l *models.LLM) {
		l.Filters = []*models.Filter{panickingGuardrail()}
	}})
	for i := 0; i < 2; i++ {
		resp, out := n.do(n.keyA, http.MethodPost, batchesPath, batchCreateBody("claude-luna", "claude-luna"))
		require.Equal(t, http.StatusBadRequest, resp.StatusCode, string(out))
		assert.NotContains(t, string(out), "secret-tool-output")
	}
	assert.Empty(t, n.vendor.callsTo(http.MethodPost, "/v1/messages/batches"))
}
