//go:build enterprise

package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func filterConnection(t *testing.T, attrs FilterAttributes) map[string]interface{} {
	t.Helper()
	conn, ok := attrs.Config["connection"].(map[string]interface{})
	require.True(t, ok, "config.connection missing: %+v", attrs.Config)
	return conn
}

// A literal guardrail key is returned as [redacted] by every filter read, as
// an LLM's api_key is; a $SECRET/ reference and non-secret fields are shown.
// Sending the redaction back keeps the stored key; the gateway (which reads
// the stored filter, not the API) still gets the real one.
func TestGuardrailConnectionSecretsRedactedOnRead(t *testing.T) {
	api, _ := setupTestAPI(t)
	endpoint := "https://classifier.example/v1"

	// Create: the response already hides the key.
	w := performRequest(api.router, "POST", "/api/v1/filters", guardrailFilterInput("g", false, httpGuardrailConfig(endpoint, "literal-key")))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var created FilterResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	conn := filterConnection(t, created.Attributes)
	assert.Equal(t, models.CredentialRedacted, conn["api_key"])
	assert.Equal(t, endpoint, conn["endpoint"])
	n, _ := strconv.ParseUint(created.ID, 10, 32)
	id := uint(n)

	// Get and list.
	w = performRequest(api.router, "GET", "/api/v1/filters/"+created.ID, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.NotContains(t, w.Body.String(), "literal-key")
	var got FilterResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, models.CredentialRedacted, filterConnection(t, got.Attributes)["api_key"])
	w = performRequest(api.router, "GET", "/api/v1/filters?all=true", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.NotContains(t, w.Body.String(), "literal-key")

	// Update with the redaction sent back (the edit form's save): the
	// stored key is kept, the rest of the change applies.
	w = performRequest(api.router, "PATCH", "/api/v1/filters/"+created.ID,
		guardrailFilterInput("g-renamed", false, httpGuardrailConfig(endpoint, models.CredentialRedacted)))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.NotContains(t, w.Body.String(), "literal-key")
	stored, err := api.service.GetFilterByID(id)
	require.NoError(t, err)
	assert.Equal(t, "g-renamed", stored.Name)
	assert.Equal(t, "literal-key", stored.Config["connection"].(map[string]interface{})["api_key"])

	// A new key replaces it.
	w = performRequest(api.router, "PATCH", "/api/v1/filters/"+created.ID,
		guardrailFilterInput("g-renamed", false, httpGuardrailConfig(endpoint, "new-key")))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	stored, err = api.service.GetFilterByID(id)
	require.NoError(t, err)
	assert.Equal(t, "new-key", stored.Config["connection"].(map[string]interface{})["api_key"])

	// The redaction under another provider stands for nothing: refused,
	// not saved as the literal "[redacted]".
	other := httpGuardrailConfig(endpoint, models.CredentialRedacted)
	other["provider"] = "lakera"
	w = performRequest(api.router, "PATCH", "/api/v1/filters/"+created.ID, guardrailFilterInput("g-renamed", false, other))
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	stored, err = api.service.GetFilterByID(id)
	require.NoError(t, err)
	assert.Equal(t, "new-key", stored.Config["connection"].(map[string]interface{})["api_key"])

	// Nor can a new filter be created with it.
	w = performRequest(api.router, "POST", "/api/v1/filters", guardrailFilterInput("g2", false, httpGuardrailConfig(endpoint, models.CredentialRedacted)))
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())

	// A reference is not a secret value and is shown as stored.
	_, err = api.service.UpdateFilterFromSpec(id, services.FilterSpec{
		Name: "g-renamed", Kind: models.FilterKindGuardrail, Config: httpGuardrailConfig(endpoint, "$SECRET/CLASSIFIER_KEY"),
	})
	require.NoError(t, err)
	w = performRequest(api.router, "GET", "/api/v1/filters/"+created.ID, nil)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, "$SECRET/CLASSIFIER_KEY", filterConnection(t, got.Attributes)["api_key"])
}

// Testing a saved filter from its edit page sends the redacted key back: the
// stored key is used for exactly the saved connection, and never sent to an
// endpoint the caller changed.
func TestFilterTestEndpoint_RedactedKeyOnlyReachesTheSavedEndpoint(t *testing.T) {
	api, _ := setupTestAPI(t)
	saved := newRecordingClassifier(t)
	attacker := newRecordingClassifier(t)

	w := performRequest(api.router, "POST", "/api/v1/filters", guardrailFilterInput("g", false, httpGuardrailConfig(saved.URL, "the-real-key")))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var created FilterResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	n, _ := strconv.ParseUint(created.ID, 10, 32)
	id := uint(n)

	// The edit page's test: saved connection with the redaction.
	var out FilterTestOutput
	w = performRequest(api.router, "POST", "/api/v1/filters/test", guardrailTestBody(id, httpGuardrailConfig(saved.URL, models.CredentialRedacted)))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	require.True(t, out.Success, out.Error)
	assert.Equal(t, []string{"Bearer the-real-key"}, saved.calls())

	// Same filter, endpoint moved: refused before any call.
	w = performRequest(api.router, "POST", "/api/v1/filters/test", guardrailTestBody(id, httpGuardrailConfig(attacker.URL, models.CredentialRedacted)))
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	assert.Contains(t, out.Error, "differ from the saved filter")

	// No filter id: the redaction stands for nothing.
	w = performRequest(api.router, "POST", "/api/v1/filters/test", guardrailTestBody(0, httpGuardrailConfig(attacker.URL, models.CredentialRedacted)))
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Empty(t, attacker.calls())
	assert.Len(t, saved.calls(), 1)
}
