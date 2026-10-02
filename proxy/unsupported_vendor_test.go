package proxy

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Vertex and HuggingFace LLMs cannot be served on /ai and /v1: their drivers
// cannot route through /llm/. The caller gets a 400 that says so and names the
// /llm/ routes, and nothing leaves the gateway. Vertex used to send the
// caller's App credential to generativelanguage.googleapis.com and fail there;
// HuggingFace answered 500.
func TestAI_UnsupportedVendorIsAClearClientError(t *testing.T) {
	for _, vendor := range []models.Vendor{models.VERTEX, models.HUGGINGFACE} {
		for name, body := range map[string]string{"buffered": failoverChatBody, "streaming": failoverStreamBody} {
			t.Run(string(vendor)+"/"+name, func(t *testing.T) {
				h := newFailoverHarness(t, serveOpenAIText("never"), serveOpenAIText("never"), func(primary, _ *models.LLM) {
					primary.Vendor = vendor
					primary.Failover = models.LLMFailover{}
				})

				resp, respBody := h.post("/ai/primary/v1/chat/completions", body)
				require.Equal(t, http.StatusBadRequest, resp.StatusCode, "body: %s", respBody)
				var envelope struct {
					Error struct {
						Message string `json:"message"`
						Type    string `json:"type"`
						Code    string `json:"code"`
					} `json:"error"`
				}
				require.NoError(t, json.Unmarshal(respBody, &envelope), "body: %s", respBody)
				assert.Equal(t, "invalid_request_error", envelope.Error.Type)
				assert.Equal(t, "unsupported_vendor", envelope.Error.Code)
				assert.Contains(t, envelope.Error.Message, string(vendor))
				assert.Contains(t, envelope.Error.Message, "/llm/rest/{slug}")
				assert.Empty(t, h.primaryVendor.calls(), "no request may be sent for an unsupported vendor")
			})
		}
	}
}

// An unsupported primary still fails over: the rung's driver cannot be built,
// which says nothing about the next rung's vendor.
func TestAI_UnsupportedVendorFailsOverToTheNextRung(t *testing.T) {
	h := newFailoverHarness(t, serveOpenAIText("never"), serveOpenAIText("from the fallback"), func(primary, _ *models.LLM) {
		primary.Vendor = models.VERTEX
	})

	resp, body := h.post("/ai/primary/v1/chat/completions", failoverChatBody)
	require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", body)
	var out ChatCompletionResponse
	require.NoError(t, json.Unmarshal(body, &out))
	require.Len(t, out.Choices, 1)
	assert.Equal(t, "from the fallback", out.Choices[0].Message.Content)
	assert.Equal(t, "fallback", resp.Header.Get(hdrServedLLM))
	assert.Empty(t, h.primaryVendor.calls())
}
