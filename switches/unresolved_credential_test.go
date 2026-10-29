package switches

import (
	"net/http/httptest"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/secrets"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// lsj.7: Studio Chat's driver, an embedder and the gateway's auth header
// refuse a key that did not resolve, naming the LLM, instead of sending the
// reference text to the vendor.
func TestUnresolvedCredentialIsRefused(t *testing.T) {
	llm := &models.LLM{Name: "Mock OpenAI", Vendor: models.OPENAI, APIEndpoint: "http://vendor.invalid/v1", APIKey: "$SECRET/MOCK_KEY"}

	_, err := FetchDriver(llm, &models.LLMSettings{ModelName: "gpt-4o"}, nil, nil)
	require.ErrorIs(t, err, secrets.ErrUnresolvedCredential)
	assert.Contains(t, err.Error(), `"Mock OpenAI"`)
	assert.NotContains(t, err.Error(), "MOCK_KEY")

	r := httptest.NewRequest("POST", "http://vendor.invalid/v1/chat/completions", nil)
	assert.ErrorIs(t, SetVendorAuthHeader(r, llm), secrets.ErrUnresolvedCredential)
	assert.Empty(t, r.Header.Get("Authorization"))

	_, err = GetEmbedder(&models.EmbedderSpec{Vendor: models.OPENAI, Endpoint: "http://vendor.invalid/v1", APIKey: "$ENV/FOO_KEY", Model: "text-embedding-3-small"})
	assert.ErrorIs(t, err, secrets.ErrUnresolvedCredential)

	llm.APIKey = "sk-literal"
	_, err = FetchDriver(llm, &models.LLMSettings{ModelName: "gpt-4o"}, nil, nil)
	assert.NoError(t, err)
}
