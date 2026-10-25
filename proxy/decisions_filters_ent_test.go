//go:build enterprise

package proxy

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Rewrites SECRET-123 in every message, the way a redacting guardrail does.
const decisionRedactScript = `
text := import("text")
msgs := []
for m in input.messages {
    msgs = append(msgs, {role: m.role, content: text.replace(m.content, "SECRET-123", "[REDACTED]", -1)})
}
output := {block: false, messages: msgs}
`

// Blocks decision requests whose evidence (the last user message) mentions
// "forbidden"; generative requests are let through.
const decisionBlockScript = `
text := import("text")
last := input.messages[len(input.messages)-1]
output := {
    block: input.context.request_kind == "decision" && text.contains(last.content, "forbidden"),
    message: "evidence not allowed"
}
`

func withFilter(name, script string, vendor models.Vendor) func(primary, fallback *models.LLM) {
	return func(primary, fallback *models.LLM) {
		primary.Vendor = vendor
		primary.Failover = models.LLMFailover{}
		primary.Filters = []*models.Filter{{Name: name, Script: []byte(script)}}
	}
}

func TestDecisionFilters_RedactionReachesTheUpstreamInItsOwnFormat(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		vendor models.Vendor
		model  string
		serve  http.HandlerFunc
		field  string
	}{
		"systemone": {models.SYSTEMONE, "primary/jev-latest", serveSystemOneDecision(), "state"},
		"openai":    {models.OPENAI, "primary/gpt-6-luna", serveOpenAIDecision(), "input"},
	} {
		t.Run(name, func(t *testing.T) {
			h := newFailoverHarness(t, tc.serve, serveStatus(500), withFilter("redact", decisionRedactScript, tc.vendor))
			body := `{"model":"` + tc.model + `","input":"card SECRET-123 was charged twice",` +
				`"questions":[{"type":"predicate","name":"angry","instructions":"Is the customer angry?"}]}`
			resp, out := h.post("/v1/decisions", body)
			require.Equal(t, http.StatusOK, resp.StatusCode, string(out))

			calls := h.primaryVendor.calls()
			require.Len(t, calls, 1)
			var sent map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(calls[0].Body, &sent))
			assert.JSONEq(t, `"card [REDACTED] was charged twice"`, string(sent[tc.field]))
			assert.NotContains(t, string(calls[0].Body), "SECRET-123")
		})
	}
}

// A response filter sees a decision's answers as text and can block them; the
// caller gets the block, not the answers, on the bridge and on the native
// passthrough alike.
func TestDecisionFilters_ResponseFilterBlocksAnAnswer(t *testing.T) {
	const script = `
text := import("text")
output := {
    block: text.contains(input.raw_input, "topic: choice=billing"),
    message: "billing routing is not allowed here"
}
`
	h := newFailoverHarness(t, serveSystemOneDecision(), serveStatus(500), func(primary, fallback *models.LLM) {
		asSystemOne(primary, fallback)
		primary.Filters = []*models.Filter{{Name: "no-billing", Script: []byte(script), ResponseFilter: true}}
	})

	resp, out := h.post("/v1/decisions", unifiedDecisionBody(t, "primary/jev-latest"))
	require.Equal(t, http.StatusBadRequest, resp.StatusCode, string(out))
	assert.Contains(t, decodeOAIError(t, out).Message, "billing routing is not allowed here")
	assert.NotContains(t, string(out), `"answers"`, "the blocked answers never reach the caller")

	resp, out = h.post("/llm/rest/primary/v1/systemone", decisionRequestSystemOne)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode, string(out))
	assert.NotContains(t, string(out), `"answers"`)
}

func TestDecisionFilters_BlockOnRequestKind(t *testing.T) {
	t.Parallel()
	h := newFailoverHarness(t, serveSystemOneDecision(), serveStatus(500), withFilter("block", decisionBlockScript, models.SYSTEMONE))
	body := `{"model":"primary/jev-latest","input":"this is forbidden content",` +
		`"questions":[{"type":"predicate","name":"q","instructions":"?"}]}`
	resp, out := h.post("/v1/decisions", body)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode, string(out))
	assert.Contains(t, decodeOAIError(t, out).Message, "evidence not allowed")
	assert.Empty(t, h.primaryVendor.calls())

	// The same filter lets an ordinary decision through.
	resp, out = h.post("/v1/decisions", `{"model":"primary/jev-latest","input":"fine","questions":[{"type":"predicate","name":"q","instructions":"?"}]}`)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(out))
}
