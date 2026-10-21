package proxy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/guardrails"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/requestid"
	"github.com/TykTechnologies/midsommar/v2/scripting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A guardrail block tells the caller why, which filter, and the request id to
// quote: in the JSON body and a header on a direct call, and in the message
// text, which is all the bridges and the Bedrock and Anthropic shims carry.
func TestPolicyBlock_ExplainsTheGuardrailBlockToTheClient(t *testing.T) {
	p := New(nil, nil, &Config{})
	block := redactGuardrail(guardrails.ScopeAllMessages)
	block.Name = "Wolf Defender: stop conversation"
	block.Config["on_detect"] = guardrails.ActionBlock
	block.Config["block_message"] = "This conversation was stopped by the security guardrail."

	id := requestid.New()
	r := httptest.NewRequest(http.MethodPost, "/llm/rest/primary/v1/messages", strings.NewReader(anthropicToolConversation))
	r = r.WithContext(requestid.WithID(r.Context(), id, ""))
	_, err := p.runRequestFilters(&models.LLM{Vendor: models.ANTHROPIC, Filters: []*models.Filter{block}},
		r, []byte(anthropicToolConversation), "anthropic", "claude-sonnet-5")
	require.Error(t, err)
	pb := policyBlockOf(err)
	require.NotNil(t, pb, "%v", err)
	assert.Equal(t, scripting.ReasonDetected, pb.Code)
	assert.Equal(t, id, pb.RequestID)
	assert.Contains(t, err.Error(), "Policy error: Wolf Defender: stop conversation - This conversation was stopped by the security guardrail.")
	assert.Contains(t, err.Error(), "Reason: the guardrail found")
	assert.Contains(t, err.Error(), "(guardrail_detected)")
	assert.Contains(t, err.Error(), "Request ID: "+id)
	assert.NotContains(t, err.Error(), "SECRET", "the matched text reached the client")

	// Direct call: structured fields and the header.
	rec := httptest.NewRecorder()
	respondPolicyBlock(rec, r, err)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, scripting.ReasonDetected, rec.Header().Get(hdrGuardrailReason))
	var body ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, scripting.ReasonDetected, body.ReasonCode)
	assert.Equal(t, "Wolf Defender: stop conversation", body.Filter)
	assert.Equal(t, id, body.RequestID)
	assert.NotEmpty(t, body.Reason)
	assert.Equal(t, err.Error(), body.Message)

	// The gateway's own loopback hop answers the OpenAI envelope; its message
	// carries the reason to the bridge's client.
	hop := r.Clone(r.Context())
	hop.Header.Set(hdrInternalHop, internalHopToken)
	rec = httptest.NewRecorder()
	respondPolicyBlock(rec, hop, err)
	assert.Contains(t, rec.Body.String(), "guardrail_detected")
	assert.Contains(t, rec.Body.String(), id)
}

// A script filter's block is unchanged: no reason, no header.
func TestPolicyBlock_ScriptFilterUnchanged(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/llm/rest/x/v1/chat/completions", nil)
	err := newPolicyBlock(r, "PII Detector", &scripting.ScriptOutput{Block: true, Message: "PII found"})
	assert.Equal(t, "Policy error: PII Detector - PII found", err.Error())
	rec := httptest.NewRecorder()
	respondPolicyBlock(rec, r, err)
	assert.Empty(t, rec.Header().Get(hdrGuardrailReason))
	assert.NotContains(t, rec.Body.String(), "reason_code")
}

func TestExplainBlock(t *testing.T) {
	assert.Equal(t, "Blocked. Reason: too big (guardrail_too_large_to_scan). Request ID: tyai_x.",
		explainBlock("Blocked", "guardrail_too_large_to_scan", "too big", "tyai_x"))
	assert.Equal(t, "Blocked.", explainBlock("Blocked.", "", "", "tyai_x"), "no reason, no id")
}
