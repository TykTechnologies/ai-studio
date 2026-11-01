package proxy

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
	"github.com/TykTechnologies/midsommar/v2/guardrails/templates"
	"github.com/TykTechnologies/midsommar/v2/models"
)

var hintGuardrailSeq atomic.Int64

// hintSeen is a request guardrail provider that records the
// X-Tyk-Prompt-Template hint each classification's context carries.
type hintSeen struct {
	mu    sync.Mutex
	hints []string
}

func (*hintSeen) Capabilities() guardrails.Capabilities { return guardrails.Capabilities{} }

func (s *hintSeen) Classify(ctx context.Context, _ guardrails.Input) (guardrails.Verdict, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hints = append(s.hints, templates.HintFrom(ctx))
	return guardrails.Verdict{}, nil
}

func (s *hintSeen) seen() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.hints...)
}

// hintGuardrail is a log-only request guardrail backed by s.
func hintGuardrail(s *hintSeen) *models.Filter {
	name := fmt.Sprintf("proxy_test_hint_%d", hintGuardrailSeq.Add(1))
	guardrails.RegisterSpec(guardrails.Spec{
		Name: name, DisplayName: name,
		Detectors: []guardrails.DetectorSpec{{Name: "x", Category: "other"}},
	})
	guardrails.RegisterFactory(name, func(guardrails.Config) (guardrails.Provider, error) { return s, nil })
	return &models.Filter{Name: name, Kind: models.FilterKindGuardrail, Config: models.JSONMap{
		"provider": name, "detectors": []any{map[string]any{"name": "x"}},
		"on_detect": guardrails.ActionLog, "cache": false,
	}}
}

// X-Tyk-Prompt-Template is a hint to the request guardrails: they see it
// (on the bridges across the /llm/call/ hop), and no vendor ever does.
func TestPromptTemplateHint_ReachesGuardrailsNeverTheVendor(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, path, body string }{
		{"llm rest", "/llm/rest/primary/v1/chat/completions", failoverChatBody},
		{"llm stream", "/llm/stream/primary/v1/chat/completions", failoverStreamBody},
		{"ai", "/ai/primary/v1/chat/completions", failoverChatBody},
		{"ai stream", "/ai/primary/v1/chat/completions", failoverStreamBody},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			seen := &hintSeen{}
			h := newFailoverHarness(t, serveOpenAIText("answer"), serveStatus(500), func(primary, _ *models.LLM) {
				primary.Failover = models.LLMFailover{}
				primary.Filters = []*models.Filter{hintGuardrail(seen)}
			})
			resp, out := h.post(tc.path, tc.body, templates.HintHeader, "pr-review@3")
			require.Equal(t, http.StatusOK, resp.StatusCode, string(out))

			assert.Contains(t, seen.seen(), "pr-review@3", "the guardrail's context carries the hint")
			calls := h.primaryVendor.calls()
			require.Len(t, calls, 1)
			for k := range calls[0].Headers {
				assert.NotEqual(t, http.CanonicalHeaderKey(templates.HintHeader), http.CanonicalHeaderKey(k), "the hint reached the vendor")
			}
		})
	}
}

func TestStripPromptTemplateHint(t *testing.T) {
	h := http.Header{}
	h.Set("X-Tyk-Prompt-Template", "pr-review@3")
	h["x-tyk-prompt-template"] = []string{"pr-review"} // not canonicalised
	h.Set("X-Tyk-Prompt-Templates", "kept")
	h.Set("Anthropic-Beta", "kept")

	stripFailoverHeaders(h)

	assert.Equal(t, http.Header{"X-Tyk-Prompt-Templates": {"kept"}, "Anthropic-Beta": {"kept"}}, h)
}
