//go:build enterprise

package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/authz"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Saving a guardrail whose connection adds a $SECRET/ or $ENV/ reference
// needs secrets read: the gateway resolves it and sends the value to the
// endpoint the filter names, which the same writer chose. Filters write
// alone may keep an existing reference or use literals.
func TestGuardrailFilterWritesRequireSecretReadForNewReferences(t *testing.T) {
	api, _ := setupTestAPI(t)
	filterEditor := authz.NewSet(authz.Write("filters"))
	secretReader := authz.NewSet(authz.Write("filters"), authz.Read("secrets"))

	call := func(perms authz.Set, method, id string, input FilterInput) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		body, _ := json.Marshal(input)
		c.Request = httptest.NewRequest(method, "/api/v1/filters", bytes.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		if id != "" {
			c.Params = gin.Params{{Key: "id", Value: id}}
		}
		authz.WithContext(c, authz.NewContext(7, false, func() (authz.Set, error) { return perms, nil }))
		if method == http.MethodPost {
			api.createFilter(c)
		} else {
			api.updateFilter(c)
		}
		return w
	}
	cfg := func(apiKey string) map[string]interface{} {
		return httpGuardrailConfig("https://classifier.example/v1", apiKey)
	}

	// Create
	w := call(filterEditor, http.MethodPost, "", guardrailFilterInput("g1", false, cfg("$SECRET/CLASSIFIER_KEY")))
	assert.Equal(t, http.StatusForbidden, w.Code, "new $SECRET/ reference without secrets read: %s", w.Body.String())
	w = call(filterEditor, http.MethodPost, "", guardrailFilterInput("g2", false, cfg("$ENV/CLASSIFIER_KEY")))
	assert.Equal(t, http.StatusForbidden, w.Code, "new $ENV/ reference without secrets read: %s", w.Body.String())
	w = call(filterEditor, http.MethodPost, "", guardrailFilterInput("g2", false, cfg("literal-key")))
	assert.Equal(t, http.StatusCreated, w.Code, "a literal needs no secrets read: %s", w.Body.String())
	w = call(secretReader, http.MethodPost, "", guardrailFilterInput("g3", false, cfg("$SECRET/CLASSIFIER_KEY")))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var created FilterResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	id := created.ID

	// Update: keeping the stored reference is fine, adding one is not
	w = call(filterEditor, http.MethodPatch, id, guardrailFilterInput("g3-renamed", false, cfg("$SECRET/CLASSIFIER_KEY")))
	assert.Equal(t, http.StatusOK, w.Code, "kept reference: %s", w.Body.String())
	withToken := cfg("$SECRET/CLASSIFIER_KEY")
	withToken["connection"].(map[string]interface{})["token"] = "$SECRET/OTHER_SECRET"
	w = call(filterEditor, http.MethodPatch, id, guardrailFilterInput("g3", false, withToken))
	assert.Equal(t, http.StatusForbidden, w.Code, "added reference without secrets read: %s", w.Body.String())
	w = call(secretReader, http.MethodPatch, id, guardrailFilterInput("g3", false, withToken))
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// Nothing was stored by the refused writes
	n, _ := strconv.ParseUint(id, 10, 32)
	stored, err := api.service.GetFilterByID(uint(n))
	require.NoError(t, err)
	assert.Equal(t, "g3", stored.Name)
	_ = services.FilterSpec{}
	_ = models.FilterKindGuardrail
}
