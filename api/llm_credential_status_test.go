package api

import (
	"encoding/json"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A brand-new instance bootstraps OPENAI_KEY and ANTHROPIC_KEY with empty
// values, then seeds providers pointing at them and renders them as healthy.
// These pin the three cases the UI needs to tell apart.
func TestCredentialStatus(t *testing.T) {
	t.Run("no key at all", func(t *testing.T) {
		status, ref := credentialStatus("")
		assert.Equal(t, CredentialUnset, status)
		assert.Empty(t, ref)
	})

	t.Run("inline literal key", func(t *testing.T) {
		status, ref := credentialStatus("sk-abc123")
		assert.Equal(t, CredentialInline, status)
		assert.Empty(t, ref, "an inline key has no secret name to report")
	})

	t.Run("secret reference with no backing value is unresolved", func(t *testing.T) {
		// No DB ref is configured in this test, so the reference cannot resolve.
		// This is exactly the fresh-instance case: the secret exists but is empty.
		status, ref := credentialStatus("$SECRET/OPENAI_KEY")
		assert.Equal(t, CredentialUnresolved, status)
		assert.Equal(t, "OPENAI_KEY", ref, "must name the secret so the UI can say which one to fill in")
	})

	t.Run("malformed reference is unresolved rather than treated as inline", func(t *testing.T) {
		status, _ := credentialStatus("$SECRET/")
		assert.Equal(t, CredentialUnresolved, status)
	})

	t.Run("env reference is classified as a reference, not an inline key", func(t *testing.T) {
		status, ref := credentialStatus("$ENV/DEFINITELY_NOT_SET_ANYWHERE")
		assert.Equal(t, CredentialUnresolved, status)
		assert.Equal(t, "DEFINITELY_NOT_SET_ANYWHERE", ref)
	})
}

// The UI words an unresolved $ENV/ reference as an unset environment
// variable, not an empty secret, so it needs to know which one it is.
func TestCredentialSource(t *testing.T) {
	assert.Equal(t, CredentialSourceSecret, credentialSource("$SECRET/OPENAI_KEY"))
	assert.Equal(t, CredentialSourceEnv, credentialSource("$ENV/OPENAI_API_KEY"))
	assert.Equal(t, "", credentialSource("sk-inline"))
	assert.Equal(t, "", credentialSource(""))
}

// Both serialisers (single provider and list) carry credential_source, and
// it is omitted for an inline key.
func TestSerializeLLMCarriesCredentialSource(t *testing.T) {
	a := &API{}
	env := &models.LLM{Name: "env", APIKey: "$ENV/DEFINITELY_NOT_SET_ANYWHERE"}
	secret := &models.LLM{Name: "secret", APIKey: "$SECRET/SOME_SECRET"}
	inline := &models.LLM{Name: "inline", APIKey: "sk-inline"}

	assert.Equal(t, CredentialSourceEnv, a.serializeLLM(env).Attributes.CredentialSource)
	assert.Equal(t, CredentialSourceSecret, a.serializeLLM(secret).Attributes.CredentialSource)
	assert.Equal(t, "", a.serializeLLM(inline).Attributes.CredentialSource)

	list := a.serializeLLMs(models.LLMs{*env, *secret, *inline})
	require.Len(t, list, 3)
	assert.Equal(t, CredentialSourceEnv, list[0].Attributes.CredentialSource)
	assert.Equal(t, CredentialSourceSecret, list[1].Attributes.CredentialSource)
	assert.Equal(t, "", list[2].Attributes.CredentialSource)

	body, err := json.Marshal(list[2])
	require.NoError(t, err)
	assert.NotContains(t, string(body), "credential_source", "omitted for an inline key")
	body, err = json.Marshal(list[0])
	require.NoError(t, err)
	assert.Contains(t, string(body), `"credential_source":"env"`)
}
