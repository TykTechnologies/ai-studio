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

// A quarantining guardrail on the pass-through replaces a flagged tool result
// and the request reaches the vendor, in each vendor's format, with every
// call id and name intact. The tool results are in front of the guardrail
// through scope all_messages until tool-result scanning is on by default.

func quarantineGuardrail(placeholder string) *models.Filter {
	f := redactGuardrail(guardrails.ScopeAllMessages)
	f.Name = "quarantine"
	f.Config["on_detect"] = guardrails.ActionQuarantine
	f.Config["quarantine"] = map[string]any{"placeholder": placeholder}
	return f
}

func serveGemini(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"ok"}]},"finishReason":"STOP"}],` +
		`"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":1,"totalTokenCount":6}}`))
}

func TestGuardrailQuarantine_ReachesTheVendor(t *testing.T) {
	t.Parallel()
	const placeholder = "[removed: {{source}}]"
	cases := []struct {
		name   string
		vendor models.Vendor
		path   string
		serve  http.HandlerFunc
		body   string
		want   string
	}{
		{
			name: "anthropic", vendor: models.ANTHROPIC, path: "/llm/rest/primary/v1/messages", serve: serveAnthropic(t),
			body: `{"model":"claude-sonnet-5","max_tokens":64,"messages":[{"role":"user","content":"read it"},` +
				`{"role":"assistant","content":[{"type":"tool_use","id":"toolu_1","name":"read_file","input":{}}]},` +
				`{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_1","content":[{"type":"text","text":"API_KEY=SECRET"}]}]}]}`,
			want: `{"model":"claude-sonnet-5","max_tokens":64,"messages":[{"role":"user","content":"read it"},` +
				`{"role":"assistant","content":[{"type":"tool_use","id":"toolu_1","name":"read_file","input":{}}]},` +
				`{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_1","content":"[removed: tool result from read_file]"}]}]}`,
		},
		{
			name: "openai", vendor: models.OPENAI, path: "/llm/rest/primary/v1/chat/completions", serve: serveOpenAI,
			body: `{"model":"gpt-4o","messages":[{"role":"user","content":"read it"},` +
				`{"role":"assistant","content":null,"tool_calls":[{"id":"call_1","type":"function","function":{"name":"read_file","arguments":"{}"}}]},` +
				`{"role":"tool","tool_call_id":"call_1","content":"API_KEY=SECRET"}]}`,
			want: `{"model":"gpt-4o","messages":[{"role":"user","content":"read it"},` +
				`{"role":"assistant","content":null,"tool_calls":[{"id":"call_1","type":"function","function":{"name":"read_file","arguments":"{}"}}]},` +
				`{"role":"tool","tool_call_id":"call_1","content":"[removed: tool result from read_file]"}]}`,
		},
		{
			name: "google", vendor: models.GOOGLEAI, path: "/llm/rest/primary/v1beta/models/gemini-2.5-flash:generateContent", serve: serveGemini,
			body: `{"contents":[{"role":"user","parts":[{"text":"read it"}]},` +
				`{"role":"model","parts":[{"functionCall":{"name":"read_file","args":{}}}]},` +
				`{"role":"user","parts":[{"functionResponse":{"name":"read_file","response":{"content":"API_KEY=SECRET"}}}]}]}`,
			want: `{"contents":[{"role":"user","parts":[{"text":"read it"}]},` +
				`{"role":"model","parts":[{"functionCall":{"name":"read_file","args":{}}}]},` +
				`{"role":"user","parts":[{"functionResponse":{"name":"read_file","response":{"output":"[removed: tool result from read_file]"}}}]}]}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newFailoverHarness(t, tc.serve, serveStatus(500), func(primary, _ *models.LLM) {
				primary.Vendor = tc.vendor
				primary.Failover = models.LLMFailover{}
				primary.Filters = []*models.Filter{quarantineGuardrail(placeholder)}
			})
			resp, out := h.post(tc.path, tc.body)
			require.Equal(t, http.StatusOK, resp.StatusCode, string(out))

			calls := h.primaryVendor.calls()
			require.Len(t, calls, 1)
			assert.NotContains(t, string(calls[0].Body), "SECRET")
			assert.Equal(t, decodeJSON(t, []byte(tc.want)), decodeJSON(t, calls[0].Body))
		})
	}
}

// Every filter after the quarantining one sees the quarantined body.
func TestGuardrailQuarantine_NextFilterSeesThePlaceholder(t *testing.T) {
	p := New(nil, nil, &Config{})
	block := redactGuardrail(guardrails.ScopeAllMessages)
	block.Name = "block secrets"
	block.Config["on_detect"] = guardrails.ActionBlock
	llm := &models.LLM{Vendor: models.ANTHROPIC, Filters: []*models.Filter{quarantineGuardrail("[gone]"), block}}
	r, _ := http.NewRequest(http.MethodPost, "/llm/rest/primary/v1/messages", strings.NewReader(anthropicToolConversation))
	out, err := p.runRequestFilters(llm, r, []byte(anthropicToolConversation), "anthropic", "claude-sonnet-5")
	require.NoError(t, err)
	assert.NotContains(t, string(out), "SECRET")
	assert.Contains(t, string(out), `"tool_use_id":"toolu_01"`)
}
