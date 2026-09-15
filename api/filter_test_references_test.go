//go:build enterprise

package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A recording classifier: answers "clean" and remembers what it was sent.
type recordingClassifier struct {
	*httptest.Server
	mu    sync.Mutex
	auths []string
}

func newRecordingClassifier(t *testing.T) *recordingClassifier {
	t.Helper()
	rc := &recordingClassifier{}
	rc.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rc.mu.Lock()
		rc.auths = append(rc.auths, r.Header.Get("Authorization"))
		rc.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"flagged":false,"findings":[]}`))
	}))
	t.Cleanup(rc.Close)
	return rc
}

func (rc *recordingClassifier) calls() []string {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	return append([]string(nil), rc.auths...)
}

func httpGuardrailConfig(endpoint, apiKey string) map[string]interface{} {
	return map[string]interface{}{
		"provider":   "http",
		"detectors":  []interface{}{map[string]interface{}{"name": "prompt_attack"}},
		"on_detect":  "block",
		"connection": map[string]interface{}{"endpoint": endpoint, "api_key": apiKey},
	}
}

func guardrailTestBody(filterID uint, config map[string]interface{}) FilterTestInput {
	return FilterTestInput{
		Kind:     models.FilterKindGuardrail,
		FilterID: filterID,
		Config:   config,
		Input: map[string]interface{}{
			"raw_input": `{"messages":[{"role":"user","content":"hello"}]}`,
			"messages":  []interface{}{map[string]interface{}{"role": "user", "content": "hello"}},
		},
	}
}

// The test endpoint takes a config from the caller and calls the provider
// for real. With a $SECRET/ or $ENV/ reference in the connection and an
// endpoint the same caller chose, that would resolve any secret the platform
// holds and post it to any host. References are therefore honoured only for
// the connection block saved on the filter the test names.
func TestFilterTestEndpoint_SecretReferencesOnlyReachTheSavedEndpoint(t *testing.T) {
	api, _ := setupTestAPI(t)
	t.Setenv("GUARD_TEST_CLASSIFIER_KEY", "the-real-key")

	saved := newRecordingClassifier(t)
	attacker := newRecordingClassifier(t)
	ref := "$ENV/GUARD_TEST_CLASSIFIER_KEY"

	// 1. An ad-hoc config with a reference is refused before any call.
	w := performRequest(api.router, "POST", "/api/v1/filters/test", guardrailTestBody(0, httpGuardrailConfig(attacker.URL, ref)))
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	var out FilterTestOutput
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	assert.Contains(t, out.Error, "connection.api_key")
	assert.Contains(t, out.Error, "save the filter first")
	assert.Empty(t, attacker.calls(), "nothing may be sent before the filter is saved")

	// 2. Save the filter (write permission), then test it: the reference
	//    resolves and reaches the saved endpoint.
	w = performRequest(api.router, "POST", "/api/v1/filters", guardrailFilterInput("Classifier", false, httpGuardrailConfig(saved.URL, ref)))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var created FilterResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	id64, err := strconv.ParseUint(created.ID, 10, 32)
	require.NoError(t, err)
	id := uint(id64)

	w = performRequest(api.router, "POST", "/api/v1/filters/test", guardrailTestBody(id, httpGuardrailConfig(saved.URL, ref)))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	require.True(t, out.Success, out.Error)
	require.Equal(t, []string{"Bearer the-real-key"}, saved.calls())

	// 3. The same filter id with the endpoint moved elsewhere is refused:
	//    the reference never leaves for the new host.
	w = performRequest(api.router, "POST", "/api/v1/filters/test", guardrailTestBody(id, httpGuardrailConfig(attacker.URL, ref)))
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	assert.Contains(t, out.Error, "differ from the saved filter")
	assert.Empty(t, attacker.calls())

	// 4. So is a different provider reading the same connection block.
	moved := httpGuardrailConfig(saved.URL, ref)
	moved["provider"] = "lakera"
	w = performRequest(api.router, "POST", "/api/v1/filters/test", guardrailTestBody(id, moved))
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Len(t, saved.calls(), 1, "the saved endpoint saw only the legitimate test")

	// 5. A literal key is the caller's own and needs no saved filter.
	w = performRequest(api.router, "POST", "/api/v1/filters/test", guardrailTestBody(0, httpGuardrailConfig(attacker.URL, "literal-key")))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, []string{"Bearer literal-key"}, attacker.calls())
}
