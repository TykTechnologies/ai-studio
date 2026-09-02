// tests/integration/inference_gateway_coexistence_test.go
//
// Proves the deployment shape documented in
// docs/site/docs/deployment-kubernetes-inference-gateway.md: the microgateway
// governing traffic into an in-cluster, OpenAI-compatible model server — the
// Service that fronts a Gateway API InferencePool — while egress to internal
// addresses is otherwise blocked.
//
// The mock upstream binds 127.0.0.1, which is an internal range, so it stands in
// for a cluster Service as far as the SSRF policy is concerned. That makes this
// an honest test of the exact interaction operators hit: turn on
// LLM_UPSTREAM_BLOCK_INTERNAL and the in-cluster upstream is refused until it is
// named in LLM_UPSTREAM_ALLOWED_INTERNAL_HOSTS.
package integration

import (
	"encoding/json"
	"net/http"
	"net/url"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/pkg/netguard"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInferenceGatewayCoexistence(t *testing.T) {
	chatBody := `{"model":"gpt-4","messages":[{"role":"user","content":"hello"}]}`

	t.Run("in-cluster upstream is refused when internal egress is blocked", func(t *testing.T) {
		t.Setenv(netguard.EnvBlockInternal, "true")
		baseURL, backend := setupTrafficTest(t)
		before := backend.GetRequestCount()

		status, body := gatewayPost(t, baseURL+"/ai/mock-openai/v1/chat/completions", trafficTestToken, chatBody)

		assert.NotEqual(t, http.StatusOK, status,
			"an internal upstream must not be reachable with blocking on: %s", body)
		assert.Equal(t, before, backend.GetRequestCount(),
			"the upstream was reached despite internal egress being blocked")
	})

	t.Run("naming the host in the internal allowlist permits it", func(t *testing.T) {
		t.Setenv(netguard.EnvBlockInternal, "true")
		t.Setenv(netguard.EnvAllowedInternalHosts, "127.0.0.1")
		baseURL, backend := setupTrafficTest(t)
		before := backend.GetRequestCount()

		status, body := gatewayPost(t, baseURL+"/ai/mock-openai/v1/chat/completions", trafficTestToken, chatBody)
		require.Equal(t, http.StatusOK, status, "body: %s", body)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &resp))
		require.NotEmpty(t, resp["choices"], "response missing choices: %s", body)

		assert.Greater(t, backend.GetRequestCount(), before,
			"the exempted in-cluster upstream never received the request")
	})

	t.Run("governance still applies to the in-cluster upstream", func(t *testing.T) {
		// The whole point of fronting an InferencePool with Tyk: authentication
		// is enforced on the way in, not delegated to the model server.
		t.Setenv(netguard.EnvBlockInternal, "true")
		t.Setenv(netguard.EnvAllowedInternalHosts, "127.0.0.1")
		baseURL, backend := setupTrafficTest(t)
		before := backend.GetRequestCount()

		status, _ := gatewayPost(t, baseURL+"/ai/mock-openai/v1/chat/completions", "", chatBody)
		assert.Equal(t, http.StatusUnauthorized, status,
			"an unauthenticated caller must not reach the model server")
		assert.Equal(t, before, backend.GetRequestCount(),
			"an unauthenticated request was forwarded upstream")
	})

	t.Run("the internal allowlist does not become a global allowlist", func(t *testing.T) {
		// Naming a cluster host must not restrict external providers the way
		// LLM_UPSTREAM_ALLOWED_HOSTS would.
		t.Setenv(netguard.EnvBlockInternal, "true")
		t.Setenv(netguard.EnvAllowedInternalHosts, ".svc.cluster.local")

		u, err := url.Parse("https://api.anthropic.com/v1/messages")
		require.NoError(t, err)
		assert.NoError(t, netguard.ValidateUpstreamURL(u),
			"an external provider must stay reachable when only cluster hosts are exempted")
	})
}
