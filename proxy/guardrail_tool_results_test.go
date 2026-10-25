package proxy

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
	"github.com/TykTechnologies/midsommar/v2/models"
)

// Tool results are scanned by default. A filter that blocks quarantines a
// flagged tool result and the request goes on; tool_results.on_detect block
// keeps the policy error.
func TestGuardrailToolResults_DefaultQuarantineAndExplicitBlock(t *testing.T) {
	t.Parallel()
	body := `{"model":"claude-sonnet-5","max_tokens":64,"messages":[{"role":"user","content":"read it"},` +
		`{"role":"assistant","content":[{"type":"tool_use","id":"toolu_1","name":"read_file","input":{}}]},` +
		`{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_1","content":"API_KEY=SECRET"}]}]}`
	blockFilter := func(toolResults map[string]any) *models.Filter {
		f := redactGuardrail(guardrails.ScopeAllUser)
		f.Name = "secrets"
		f.Config["on_detect"] = guardrails.ActionBlock
		if toolResults != nil {
			f.Config["tool_results"] = toolResults
		}
		return f
	}
	post := func(filter *models.Filter) (*failoverHarness, *http.Response, []byte) {
		h := newFailoverHarness(t, serveAnthropic(t), serveStatus(500), func(primary, _ *models.LLM) {
			primary.Vendor = models.ANTHROPIC
			primary.Failover = models.LLMFailover{}
			primary.Filters = []*models.Filter{filter}
		})
		resp, out := h.post("/llm/rest/primary/v1/messages", body)
		return h, resp, out
	}

	h, resp, out := post(blockFilter(nil))
	require.Equal(t, http.StatusOK, resp.StatusCode, string(out))
	calls := h.primaryVendor.calls()
	require.Len(t, calls, 1)
	assert.NotContains(t, string(calls[0].Body), "SECRET")
	assert.Contains(t, string(calls[0].Body),
		`"tool_use_id":"toolu_1","content":"[content removed by policy: secrets.test detected in tool result from read_file]"`)

	h, resp, out = post(blockFilter(map[string]any{"on_detect": "block"}))
	require.Equal(t, http.StatusBadRequest, resp.StatusCode, string(out))
	assert.Contains(t, string(out), "Policy error")
	assert.Empty(t, h.primaryVendor.calls())

	// Scanning off: the tool result is not the filter's business.
	h, resp, out = post(blockFilter(map[string]any{"scan": false}))
	require.Equal(t, http.StatusOK, resp.StatusCode, string(out))
	require.Len(t, h.primaryVendor.calls(), 1)
	assert.Contains(t, string(h.primaryVendor.calls()[0].Body), "API_KEY=SECRET")
}
