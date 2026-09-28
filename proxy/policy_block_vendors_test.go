//go:build enterprise

package proxy

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// A request-filter block on a route whose vendor is not OpenAI-shaped must
// still reach the /ai/ caller with the filter's message. On a Google AI route
// the caller got "googleapi: Error 400:" with an empty message: the Google
// driver cannot read the inner hop's OpenAI envelope, so the reason (and which
// guardrail blocked) was lost although the block itself worked.
func TestPolicyBlock_ReasonReachesTheClientOnEveryVendor(t *testing.T) {
	const wantMessage = "[ERROR] msg: policy_violation err: Policy error: no-secrets - no secrets"
	for _, vendor := range []models.Vendor{models.GOOGLEAI, models.ANTHROPIC} {
		for name, body := range map[string]string{"non-streaming": failoverChatBody, "streaming": failoverStreamBody} {
			t.Run(fmt.Sprintf("%s/%s", vendor, name), func(t *testing.T) {
				h := newFailoverHarness(t, serveOpenAIText("never"), serveOpenAIText("never"), func(primary, fallback *models.LLM) {
					asVendor(vendor)(primary, fallback)
					attachBlockingFilter(primary, fallback)
				})
				resp, got := h.post("/ai/primary/v1/chat/completions", body)
				require.Equal(t, http.StatusBadRequest, resp.StatusCode, "body: %s", got)
				apiErr := decodeOAIError(t, got)
				assert.Equal(t, wantMessage, apiErr.Message)
				assert.Equal(t, "invalid_request_error", apiErr.Type)
				assert.NotContains(t, string(got), "googleapi")
				assert.Empty(t, h.primaryVendor.calls(), "a blocked request must not reach the vendor")
			})
		}
	}
}
