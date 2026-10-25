package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/config"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/authz"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// internalHostRouter is a router whose caller holds every write permission
// on LLMs, embedders, plugins and filters, as an administrator or not.
func internalHostRouter(a *API, admin bool) *gin.Engine {
	perms := authz.NewSet(authz.Write("llms"), authz.Publish("llms"), authz.Write("embedders"),
		authz.Write("plugins"), authz.Publish("plugins"), authz.Write("filters"))
	r := gin.New()
	r.Use(func(c *gin.Context) {
		authz.WithContext(c, authz.NewContext(7, admin, func() (authz.Set, error) { return perms, nil }))
		c.Set("user", &models.User{ID: 7, Email: "editor@example.com", IsAdmin: admin})
		c.Next()
	})
	r.POST("/llms", a.createLLM)
	r.PATCH("/llms/:id", a.updateLLM)
	r.POST("/embedders", a.createEmbedder)
	r.PATCH("/embedders/:id", a.updateEmbedder)
	r.POST("/plugins", a.createPlugin)
	r.PATCH("/plugins/:id", a.updatePlugin)
	r.POST("/filters", a.createFilter)
	r.PATCH("/filters/:id", a.updateFilter)
	return r
}

// The "Allow internal host" switch (w4e) is admin-only: a non-admin who may
// otherwise write the asset gets 403 for setting or changing it, an update
// that leaves it out keeps the stored value, and an administrator may set it.
func TestAllowInternalHost_AdminOnly(t *testing.T) {
	a, db := setupTestAPI(t)
	if a.config != nil {
		testMode := a.config.TestMode
		a.config.TestMode = false
		defer func() { a.config.TestMode = testMode }()
	}
	editor := internalHostRouter(a, false)
	admin := internalHostRouter(a, true)
	on := map[string]interface{}{"allow_internal_host": true}

	// LLMs.
	w := performRequest(editor, "POST", "/llms", withAttrs(llmRefBody("ih-llm", "sk-plain", nil), on))
	assert.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	w = performRequest(admin, "POST", "/llms", withAttrs(llmRefBody("ih-llm", "sk-plain", nil), on))
	require.Less(t, w.Code, 300, w.Body.String())
	id := createdID(t, w.Body.Bytes())
	var llm models.LLM
	require.NoError(t, db.First(&llm, id).Error)
	assert.True(t, llm.AllowInternalHost)
	w = performRequest(editor, "PATCH", fmt.Sprintf("/llms/%d", id), map[string]interface{}{"data": map[string]interface{}{
		"type": "LLM", "attributes": map[string]interface{}{"short_description": "edited"}}})
	require.Less(t, w.Code, 300, "an edit that leaves the switch out needs no admin: %s", w.Body.String())
	require.NoError(t, db.First(&llm, id).Error)
	assert.True(t, llm.AllowInternalHost, "an edit without the field keeps the switch")
	w = performRequest(editor, "PATCH", fmt.Sprintf("/llms/%d", id), map[string]interface{}{"data": map[string]interface{}{
		"type": "LLM", "attributes": map[string]interface{}{"allow_internal_host": false}}})
	assert.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	w = performRequest(admin, "PATCH", fmt.Sprintf("/llms/%d", id), map[string]interface{}{"data": map[string]interface{}{
		"type": "LLM", "attributes": map[string]interface{}{"allow_internal_host": false}}})
	require.Less(t, w.Code, 300, w.Body.String())
	require.NoError(t, db.First(&llm, id).Error)
	assert.False(t, llm.AllowInternalHost)

	// Embedders.
	embedder := func(extra map[string]interface{}) map[string]interface{} {
		attrs := map[string]interface{}{"name": "ih-embedder", "vendor": "ollama", "endpoint": "http://ollama:11434", "model": "nomic", "privacy_score": 10}
		for k, v := range extra {
			attrs[k] = v
		}
		return map[string]interface{}{"data": map[string]interface{}{"type": "embedders", "attributes": attrs}}
	}
	w = performRequest(editor, "POST", "/embedders", embedder(on))
	assert.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	w = performRequest(admin, "POST", "/embedders", embedder(on))
	require.Less(t, w.Code, 300, w.Body.String())
	eid := createdID(t, w.Body.Bytes())
	w = performRequest(editor, "PATCH", fmt.Sprintf("/embedders/%d", eid), embedder(nil))
	require.Less(t, w.Code, 300, w.Body.String())
	var e models.Embedder
	require.NoError(t, db.First(&e, eid).Error)
	assert.True(t, e.AllowInternalHost, "an edit without the field keeps the switch")

	// Plugins: the switch also exempts the plugin's own internal host from
	// the plugin host rule.
	t.Setenv("ALLOW_INTERNAL_NETWORK_ACCESS", "")
	plugin := func(extra map[string]interface{}) map[string]interface{} {
		body := map[string]interface{}{"name": "ih-plugin", "command": "grpc://localhost:50051", "hook_type": "post_auth"}
		for k, v := range extra {
			body[k] = v
		}
		return body
	}
	w = performRequest(editor, "POST", "/plugins", plugin(on))
	assert.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	w = performRequest(admin, "POST", "/plugins", plugin(nil))
	assert.Equal(t, http.StatusBadRequest, w.Code, "an internal plugin host without the switch: %s", w.Body.String())
	w = performRequest(admin, "POST", "/plugins", plugin(on))
	require.Less(t, w.Code, 300, w.Body.String())

	// Guardrail filters (Enterprise): the switch is in the config.
	if config.IsEnterprise() {
		filter := func(allow bool) map[string]interface{} {
			return map[string]interface{}{"data": map[string]interface{}{"type": "filters", "attributes": map[string]interface{}{
				"name": "ih-guardrail", "description": "d", "kind": "guardrail", "config": map[string]interface{}{
					"provider": "builtin", "detectors": []interface{}{map[string]interface{}{"name": "injection"}},
					"on_detect": "block", "allow_internal_host": allow,
				}}}}
		}
		w = performRequest(editor, "POST", "/filters", filter(true))
		assert.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
		w = performRequest(admin, "POST", "/filters", filter(true))
		require.Less(t, w.Code, 300, w.Body.String())
		var created struct {
			ID string `json:"id"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
		fid := created.ID
		w = performRequest(editor, "PATCH", "/filters/"+fid, filter(false))
		assert.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
		w = performRequest(editor, "PATCH", "/filters/"+fid, filter(true))
		assert.Less(t, w.Code, 300, "an unchanged switch needs no admin: %s", w.Body.String())
	}
}
