package responses

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/pkg/decisions"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const systemOneRequest = `{"model":"jev-latest","state":"x","questions":{` +
	`"a":{"type":"noul","instructions":"?"},"b":{"type":"noul","instructions":"?"},"c":{"type":"noul","instructions":"?"}}}`

func TestDecisionResponse_CountsAnswersAndRefusals(t *testing.T) {
	dr, err := NewDecisionResponse([]byte(`{"model":"jev-1.13.0","answers":{"a":{"type":"noul","noul":0.4}},` +
		`"usage":{"input_tokens":50,"output_tokens":0}}`))
	require.NoError(t, err)
	dr.ApplyRequest([]byte(systemOneRequest), decisions.FormatSystemOne)

	assert.Equal(t, "jev-1.13.0", dr.GetModel())
	assert.Equal(t, 50, dr.GetPromptTokens())
	assert.Equal(t, 0, dr.GetResponseTokens())
	assert.Equal(t, 1, dr.GetChoiceCount())
	assert.Equal(t, 3, dr.GetDecisionQuestions())
	assert.Equal(t, 2, dr.GetDecisionRefusals(), "System One leaves unanswered questions out")
}

// liveOpenAIDecision is the response shape gpt-6-luna returned on the live
// API (captured 2026-10-11 by the decision conformance suite): no id or
// object, and a cache breakdown inside usage.
const liveOpenAIDecision = `{"model":"gpt-6-luna","answers":[` +
	`{"type":"predicate","name":"is_billing","probability":1.0},` +
	`{"type":"score","name":"urgency","score":0.54,"probabilities":[{"value":0,"label":"low","probability":0.46},{"value":1,"label":"high","probability":0.54}],"confidence":0.08}],` +
	`"usage":{"input_tokens":261,"input_tokens_details":{"cached_tokens":40,"cache_write_tokens":10},"output_tokens":0,"output_tokens_details":{"reasoning_tokens":0},"total_tokens":261}}`

func TestDecisionResponse_LiveOpenAIShapeCarriesCacheTokens(t *testing.T) {
	dr, err := NewDecisionResponse([]byte(liveOpenAIDecision))
	require.NoError(t, err)
	assert.Equal(t, "gpt-6-luna", dr.GetModel())
	assert.Equal(t, 261, dr.GetPromptTokens(), "input_tokens includes the cached share, as chat prompt_tokens does")
	assert.Equal(t, 40, dr.GetCacheReadPromptTokens())
	assert.Equal(t, 10, dr.GetCacheWritePromptTokens())
	assert.Equal(t, 2, dr.GetChoiceCount())
	assert.Equal(t, 0, dr.GetDecisionRefusals())
}

// A failed call (upstream error, budget denial, policy block) is recorded,
// but its questions were never put to the model: they are not refusals.
func TestDecisionResponse_ErrorBodiesAreNotRefusals(t *testing.T) {
	for _, body := range []string{`{"detail":"overloaded"}`, `{"error":"policy_violation","detail":"blocked"}`} {
		dr, err := NewDecisionResponse([]byte(body))
		require.NoError(t, err)
		dr.ApplyRequest([]byte(systemOneRequest), decisions.FormatSystemOne)
		assert.Equal(t, 3, dr.GetDecisionQuestions(), body)
		assert.Equal(t, 0, dr.GetDecisionRefusals(), body)
		assert.Equal(t, 0, dr.GetChoiceCount(), body)
	}

	_, err := NewDecisionResponse([]byte("not json"))
	assert.Error(t, err)
}
