package proxy

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
	"github.com/TykTechnologies/midsommar/v2/models"
)

// Responses API bodies are read by their own extractor, which lists tool
// outputs as role tool messages: guardrails scan them by default, and a
// blocking filter quarantines them through the Responses reconstructor.
func TestGuardrailToolResults_ResponsesFunctionCallOutput(t *testing.T) {
	body := `{"model":"gpt-4.1","input":[{"role":"user","content":"read it"},` +
		`{"type":"function_call","call_id":"c1","name":"read_file","arguments":"{}"},` +
		`{"type":"function_call_output","call_id":"c1","output":"API_KEY=SECRET"}]}`
	p := New(nil, nil, &Config{})
	run := func(toolResults map[string]any) ([]byte, error) {
		f := redactGuardrail(guardrails.ScopeAllUser)
		f.Config["on_detect"] = guardrails.ActionBlock
		f.Config["quarantine"] = map[string]any{"placeholder": "[gone]"}
		if toolResults != nil {
			f.Config["tool_results"] = toolResults
		}
		r, _ := http.NewRequest(http.MethodPost, "/llm/rest/primary/v1/responses", strings.NewReader(body))
		return p.runRequestFilters(&models.LLM{Vendor: models.OPENAI, Filters: []*models.Filter{f}}, r, []byte(body), requestFilterFormatOpenAIResponses, "gpt-4.1")
	}

	out, err := run(nil)
	require.NoError(t, err)
	assert.NotContains(t, string(out), "SECRET")
	assert.Contains(t, string(out), `"call_id":"c1"`)
	assert.Contains(t, string(out), "[gone]")

	_, err = run(map[string]any{"on_detect": "block"})
	require.ErrorContains(t, err, "Policy error")

	out, err = run(map[string]any{"scan": false})
	require.NoError(t, err)
	assert.Contains(t, string(out), "API_KEY=SECRET")
}
