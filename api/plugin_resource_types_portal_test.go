package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// TestListSubmittablePluginResourceTypes covers the portal endpoint the
// Submission form uses to offer plugin resource types.
func TestListSubmittablePluginResourceTypes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, models.InitModels(db))

	service := services.NewService(db)
	api := &API{service: service}

	plugin := &models.Plugin{Name: "asset-catalog", Command: "/usr/bin/asset-catalog", HookType: models.HookTypeResourceProvider, IsActive: true}
	require.NoError(t, db.Create(plugin).Error)

	schema := `{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`
	require.NoError(t, service.RegisterPluginResourceTypes(plugin.ID, []models.PluginResourceType{
		{Slug: "agent", Name: "Agent <b>", Description: "Agents", SupportsSubmissions: true, SubmissionSchema: schema, HasPrivacyScore: true},
		{Slug: "prompt", Name: "Prompt", SupportsSubmissions: true},
		{Slug: "internal", Name: "Internal", SupportsSubmissions: false},
		{Slug: "retired", Name: "Retired", SupportsSubmissions: true},
	}))
	_, err = service.DeactivatePluginResourceTypesExcept(plugin.ID, []string{"agent", "prompt", "internal"})
	require.NoError(t, err)

	user := &models.User{Email: "user@test.com", Name: "User", EmailVerified: true}
	require.NoError(t, user.Create(db))

	router := gin.New()
	group := router.Group("/common")
	group.Use(func(c *gin.Context) { c.Set("user", user); c.Next() })
	group.GET("/plugin-resource-types", api.listSubmittablePluginResourceTypes)

	req, _ := http.NewRequest("GET", "/common/plugin-resource-types", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var body struct {
		Data []map[string]interface{} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Len(t, body.Data, 2, "only active types accepting submissions")

	bySlug := map[string]map[string]interface{}{}
	for _, entry := range body.Data {
		bySlug[entry["slug"].(string)] = entry
	}
	agent := bySlug["agent"]
	require.NotNil(t, agent)
	assert.Equal(t, "Agent &lt;b&gt;", agent["name"], "names are HTML-escaped")
	assert.Equal(t, "asset-catalog", agent["plugin_name"])
	assert.Equal(t, float64(plugin.ID), agent["plugin_id"])
	assert.Equal(t, true, agent["has_privacy_score"])
	schemaOut, err := json.Marshal(agent["submission_schema"])
	require.NoError(t, err)
	assert.JSONEq(t, schema, string(schemaOut), "schema is returned as a JSON object, not a string")

	prompt := bySlug["prompt"]
	require.NotNil(t, prompt)
	_, hasSchema := prompt["submission_schema"]
	assert.False(t, hasSchema, "no schema key when the type has none")
	_, hasSupports := prompt["supports_submissions"]
	assert.False(t, hasSupports, "portal listing does not expose admin-only fields")
}
