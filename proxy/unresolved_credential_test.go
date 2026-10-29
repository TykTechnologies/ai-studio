package proxy

import (
	"net/http"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// lsj.7: an LLM whose key did not resolve (here an $ENV/ name not on
// AI_STUDIO_ENV_REF_ALLOWLIST) is refused with a clear 502 on the REST and
// streaming routes; the reference text never reaches the vendor.
func TestUnresolvedLLMKeyIsNotSentToTheVendor(t *testing.T) {
	h := newFailoverHarness(t, serveOpenAIText("unexpected"), serveOpenAIText("unused"), func(primary, _ *models.LLM) {
		primary.APIKey = "$ENV/LSJ7_NOT_ALLOWLISTED"
	})

	for path, reqBody := range map[string]string{
		"/llm/rest/primary/v1/chat/completions":   failoverChatBody,
		"/llm/stream/primary/v1/chat/completions": failoverStreamBody,
	} {
		resp, body := h.post(path, reqBody)
		require.Equal(t, http.StatusBadGateway, resp.StatusCode, "%s: %s", path, body)
		assert.Contains(t, string(body), "unresolved reference", path)
		assert.NotContains(t, string(body), "LSJ7_NOT_ALLOWLISTED", path)
	}
	assert.Empty(t, h.primaryVendor.calls(), "a request reached the vendor")
}
