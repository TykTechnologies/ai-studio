//go:build enterprise

package proxy

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// A request filter that blocks everything, with a message the outer hop has
// to relay verbatim.
const passthroughBlockScript = `output := {block: true, message: "no secrets"}`

// attachBlockingFilter gives the primary LLM a request filter that blocks.
// The filter is created through the LLM's association so the harness's
// db.Create of the primary writes both rows.
func attachBlockingFilter(primary, _ *models.LLM) {
	primary.Filters = []*models.Filter{{Name: "no-secrets", Script: []byte(passthroughBlockScript)}}
}

// The loopback hop answers a policy block with an OpenAI envelope so the
// outer /ai/ handler can relay the reason. Before this test the outer hop
// re-wrapped that envelope as its own "failed to generate content" error
// with the driver's "API returned unexpected status code: 400: {...}" text
// as the detail, so the caller got the reason twice, nested in two prefixes,
// and the JSON envelope quoted inside a string.
func TestPolicyBlock_OuterHopRelaysInnerEnvelope(t *testing.T) {
	const wantMessage = "[ERROR] msg: policy_violation err: Policy error: no-secrets - no secrets"

	assertRelayed := func(t *testing.T, resp *http.Response, body []byte) {
		t.Helper()
		require.Equal(t, http.StatusBadRequest, resp.StatusCode, "body: %s", body)
		var envelope OAIErrorResponse
		require.NoError(t, json.Unmarshal(body, &envelope), "body: %s", body)
		require.NotNil(t, envelope.Error)
		assert.Equal(t, wantMessage, envelope.Error.Message)
		assert.NotContains(t, envelope.Error.Message, "failed to generate content")
		assert.NotContains(t, envelope.Error.Message, "API returned unexpected status code")
		assert.Equal(t, 1, strings.Count(envelope.Error.Message, "Policy error:"))
		assert.Equal(t, oaiErrorType(http.StatusBadRequest), envelope.Error.Type)
		assert.Equal(t, oaiErrorCode(http.StatusBadRequest), envelope.Error.Code)
		assert.Empty(t, resp.Header.Get(hdrFailover), "a block is not a failover")
	}

	t.Run("ai path", func(t *testing.T) {
		h := newFailoverHarness(t, serveOpenAIText("never"), serveOpenAIText("never"), attachBlockingFilter)
		resp, body := h.post("/ai/primary/v1/chat/completions", failoverChatBody)
		assertRelayed(t, resp, body)
		assert.Empty(t, h.primaryVendor.calls(), "a blocked request must not reach the vendor")
		assert.Empty(t, h.fallbackVendor.calls(), "a block must not fail over")
	})

	t.Run("ai path streaming", func(t *testing.T) {
		h := newFailoverHarness(t, serveOpenAIText("never"), serveOpenAIText("never"), attachBlockingFilter)
		resp, body := h.post("/ai/primary/v1/chat/completions", failoverStreamBody)
		assertRelayed(t, resp, body)
		assert.Empty(t, h.primaryVendor.calls())
	})

	t.Run("unified router", func(t *testing.T) {
		h := newFailoverHarness(t, serveOpenAIText("never"), serveOpenAIText("never"), attachBlockingFilter)
		resp, body := h.post("/v1/chat/completions",
			`{"model":"primary/gpt-4o","messages":[{"role":"user","content":"Say this is a test!"}]}`)
		assertRelayed(t, resp, body)
		assert.Empty(t, h.primaryVendor.calls())
	})
}
