package proxy

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/netguard"
	"github.com/TykTechnologies/midsommar/v2/pkg/netguard/netguardtest"
)

// An /ai/ request to an LLM on an internal address, with blocking on, tells
// the caller why it was refused, as /llm/ does (w4e.4), rather than a bare
// 502 from the bridge. An IP literal is refused when the URL is checked, a
// name when its address is dialled; both carry the reason.
func TestAIBridge_InternalUpstreamRefusalCarriesTheReason(t *testing.T) {
	for _, endpoint := range []string{"IP literal", "name"} {
		for name, body := range map[string]string{"non-streaming": failoverChatBody, "streaming": failoverStreamBody} {
			t.Run(endpoint+"/"+name, func(t *testing.T) {
				h := newFailoverHarness(t, serveOpenAIText("never"), serveOpenAIText("never"), func(primary, _ *models.LLM) {
					primary.Failover = models.LLMFailover{}
					if endpoint == "name" {
						primary.APIEndpoint = strings.Replace(primary.APIEndpoint, "127.0.0.1", "localhost", 1)
					}
				})
				netguardtest.BlockInternal(t)
				t.Setenv(netguard.EnvAllowInternal, "")
				t.Setenv(netguard.EnvAllowedHosts, "")
				t.Setenv(netguard.EnvAllowedInternalHosts, "")

				resp, got := h.post("/ai/primary/v1/chat/completions", body)
				require.Equal(t, http.StatusBadGateway, resp.StatusCode, "body: %s", got)
				var out OAIErrorResponse
				require.NoError(t, json.Unmarshal(got, &out), "body: %s", got)
				require.NotNil(t, out.Error)
				assert.Contains(t, out.Error.Message, netguard.EnvBlockInternal, "the refusal names the setting")
				assert.Empty(t, h.primaryVendor.calls(), "the internal upstream was reached")
			})
		}
	}
}
