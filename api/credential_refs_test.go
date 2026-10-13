package api

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/authz"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// credentialRouter serves the credential-bearing write handlers as a user
// holding perms (and nothing else).
func credentialRouter(a *API, perms authz.Set) *gin.Engine {
	r := gin.New()
	r.Use(func(c *gin.Context) {
		authz.WithContext(c, authz.NewContext(7, false, func() (authz.Set, error) { return perms, nil }))
		c.Set("user", &models.User{ID: 7, Email: "editor@example.com"})
		c.Next()
	})
	r.POST("/llms", a.createLLM)
	r.PATCH("/llms/:id", a.updateLLM)
	r.POST("/tools", a.createTool)
	r.PATCH("/tools/:id", a.updateTool)
	r.POST("/datasources", a.createDatasource)
	r.PATCH("/datasources/:id", a.updateDatasource)
	r.POST("/embedders", a.createEmbedder)
	r.PATCH("/embedders/:id", a.updateEmbedder)
	return r
}

func llmRefBody(name, apiKey string, metadata map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{"data": map[string]interface{}{"type": "LLM", "attributes": map[string]interface{}{
		"name": name, "vendor": string(models.OPENAI), "api_key": apiKey,
		"api_endpoint": "https://api.openai.com/v1", "default_model": "gpt-4o", "privacy_score": 10,
		"metadata": metadata,
	}}}
}

func dsRefBody(name, connKey string) map[string]interface{} {
	return map[string]interface{}{"data": map[string]interface{}{"type": "datasources", "attributes": map[string]interface{}{
		"name": name, "short_description": "d", "privacy_score": 10, "db_source_type": "pgvector",
		"db_conn_string": "postgres://db/x", "db_conn_api_key": connKey, "db_name": "x",
	}}}
}

// Adding or changing a $SECRET/ or $ENV/ reference in a credential field
// needs secrets read; keeping a stored one does not.
func TestCredentialReferencesNeedSecretsRead(t *testing.T) {
	a, db := setupTestAPI(t)
	editor := credentialRouter(a, authz.NewSet(authz.Write("llms"), authz.Write("tools"), authz.Write("datasources"),
		authz.Write("embedders"), authz.Publish("llms"), authz.Publish("tools"), authz.Publish("datasources")))
	reader := credentialRouter(a, authz.NewSet(authz.Write("llms"), authz.Write("tools"), authz.Write("datasources"),
		authz.Write("embedders"), authz.Read("secrets"), authz.Publish("llms"), authz.Publish("tools"), authz.Publish("datasources")))

	// New objects with a reference.
	for _, tc := range []struct {
		path string
		body map[string]interface{}
	}{
		{"/llms", llmRefBody("ref-llm", "$SECRET/OPENAI_KEY", nil)},
		{"/llms", llmRefBody("env-llm", "sk-plain", map[string]interface{}{"region": "$ENV/AWS_REGION"})},
		{"/tools", slugToolBody("ref-tool")},
		{"/datasources", dsRefBody("ref-ds", "$ENV/DB_KEY")},
		{"/embedders", embedderBody(map[string]interface{}{"name": "ref-emb", "vendor": "openai",
			"endpoint": "https://api.openai.com/v1", "api_key": "$SECRET/EMB_KEY", "model": "m", "privacy_score": 10})},
	} {
		body := tc.body
		if tc.path == "/tools" {
			body["data"].(map[string]interface{})["attributes"].(map[string]interface{})["auth_key"] = "$SECRET/TOOL_KEY"
		}
		w := performRequest(editor, "POST", tc.path, body)
		assert.Equal(t, http.StatusForbidden, w.Code, "%s without secrets read: %s", tc.path, w.Body.String())
	}

	// With secrets read, the same writes go through.
	w := performRequest(reader, "POST", "/llms", llmRefBody("ref-llm", "$SECRET/OPENAI_KEY", map[string]interface{}{"region": "$ENV/AWS_REGION"}))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var llm models.LLM
	require.NoError(t, db.Where("name = ?", "ref-llm").First(&llm).Error)

	tool, err := a.service.CreateTool("Keyed tool", "d", models.ToolTypeREST, testToolSpec, 0, "apiKey", "$SECRET/TOOL_KEY")
	require.NoError(t, err)

	// An editor without secrets read can still edit them: the stored
	// references are kept (sent back as is, or as the redacted placeholder).
	w = performRequest(editor, "PATCH", fmt.Sprintf("/llms/%d", llm.ID), map[string]interface{}{"data": map[string]interface{}{
		"type": "LLM", "attributes": map[string]interface{}{"short_description": "edited", "api_key": services.REDACTED_VALUE,
			"metadata": map[string]interface{}{"region": "$ENV/AWS_REGION"}}}})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.NoError(t, db.First(&llm, llm.ID).Error)
	assert.Equal(t, "$SECRET/OPENAI_KEY", llm.APIKey)
	assert.Equal(t, "$ENV/AWS_REGION", llm.Metadata["region"])

	toolEdit := slugToolBody("Keyed tool")
	toolEdit["data"].(map[string]interface{})["attributes"].(map[string]interface{})["auth_key"] = services.REDACTED_VALUE
	w = performRequest(editor, "PATCH", fmt.Sprintf("/tools/%d", tool.ID), toolEdit)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var stored models.Tool
	require.NoError(t, db.First(&stored, tool.ID).Error)
	assert.Equal(t, "$SECRET/TOOL_KEY", stored.AuthKey)

	// Changing a reference is refused.
	w = performRequest(editor, "PATCH", fmt.Sprintf("/llms/%d", llm.ID), map[string]interface{}{"data": map[string]interface{}{
		"type": "LLM", "attributes": map[string]interface{}{"api_key": "$SECRET/OTHER_KEY"}}})
	assert.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	toolEdit["data"].(map[string]interface{})["attributes"].(map[string]interface{})["auth_key"] = "$ENV/TYK_AI_SECRET_KEY"
	w = performRequest(editor, "PATCH", fmt.Sprintf("/tools/%d", tool.ID), toolEdit)
	assert.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
}
