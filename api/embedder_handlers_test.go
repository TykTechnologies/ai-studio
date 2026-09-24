package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	apitest "github.com/TykTechnologies/midsommar/v2/api/testing"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupEmbedderAPI(t *testing.T) (*API, *services.Service, *gorm.DB) {
	t.Helper()
	t.Setenv("TYK_AI_SECRET_KEY", "test-key")
	db := apitest.SetupTestDB(t)
	service := apitest.SetupTestService(db)
	config := apitest.SetupTestAuthConfig(db, service)
	authService := apitest.SetupTestAuthService(db, service)
	return NewAPI(service, true, authService, config, nil, apitest.EmptyFile, nil), service, db
}

func embedderBody(attrs map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{"data": map[string]interface{}{"type": "embedders", "attributes": attrs}}
}

type embedderDoc struct {
	Data struct {
		ID         string                 `json:"id"`
		Type       string                 `json:"type"`
		Attributes map[string]interface{} `json:"attributes"`
	} `json:"data"`
}

func decodeEmbedder(t *testing.T, body []byte) embedderDoc {
	t.Helper()
	var doc embedderDoc
	require.NoError(t, json.Unmarshal(body, &doc))
	return doc
}

func errorDetail(t *testing.T, body []byte) string {
	t.Helper()
	var e ErrorResponse
	require.NoError(t, json.Unmarshal(body, &e))
	require.NotEmpty(t, e.Errors)
	return e.Errors[0].Detail
}

func TestEmbedderHandlers_CRUD(t *testing.T) {
	a, _, _ := setupEmbedderAPI(t)
	r := a.Router()

	w := apitest.PerformRequest(r, "POST", "/api/v1/embedders", embedderBody(map[string]interface{}{
		"name": "OpenAI small", "vendor": "openai", "endpoint": "https://api.openai.com/v1",
		"api_key": "sk-secret", "model": "text-embedding-3-small", "privacy_score": 60,
	}))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	created := decodeEmbedder(t, w.Body.Bytes())
	assert.Equal(t, "embedders", created.Data.Type)
	assert.Equal(t, "[redacted]", created.Data.Attributes["api_key"], "keys are never returned")
	assert.Equal(t, true, created.Data.Attributes["has_api_key"])
	assert.Equal(t, false, created.Data.Attributes["linked"])
	assert.EqualValues(t, 60, created.Data.Attributes["privacy_score"])
	id := created.Data.ID

	w = apitest.PerformRequest(r, "GET", "/api/v1/embedders?all=true", nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "OpenAI small")
	assert.NotContains(t, w.Body.String(), "sk-secret")

	// PATCH with the redacted key keeps it.
	w = apitest.PerformRequest(r, "PATCH", "/api/v1/embedders/"+id, embedderBody(map[string]interface{}{
		"name": "OpenAI small", "vendor": "openai", "endpoint": "https://proxy/v1",
		"api_key": "[redacted]", "model": "text-embedding-3-small", "privacy_score": 60,
	}))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "https://proxy/v1", decodeEmbedder(t, w.Body.Bytes()).Data.Attributes["endpoint"])

	w = apitest.PerformRequest(r, "GET", "/api/v1/embedders/"+id+"/dependents", nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"total":0`)

	w = apitest.PerformRequest(r, "DELETE", "/api/v1/embedders/"+id, nil)
	assert.Equal(t, http.StatusNoContent, w.Code)
	w = apitest.PerformRequest(r, "GET", "/api/v1/embedders/"+id, nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestEmbedderHandlers_Validation(t *testing.T) {
	a, _, db := setupEmbedderAPI(t)
	r := a.Router()

	for name, attrs := range map[string]map[string]interface{}{
		"no model":           {"name": "x", "vendor": "openai"},
		"no vendor":          {"name": "x", "model": "m"},
		"vendor cannot":      {"name": "x", "vendor": "anthropic", "model": "m"},
		"privacy over range": {"name": "x", "vendor": "openai", "model": "m", "privacy_score": 150},
	} {
		w := apitest.PerformRequest(r, "POST", "/api/v1/embedders", embedderBody(attrs))
		assert.Equal(t, http.StatusBadRequest, w.Code, "%s: %s", name, w.Body.String())
	}

	llm := &models.LLM{Name: "Prod", Vendor: models.OPENAI, PrivacyScore: 75, Active: true}
	require.NoError(t, db.Create(llm).Error)
	w := apitest.PerformRequest(r, "POST", "/api/v1/embedders", embedderBody(map[string]interface{}{
		"name": "linked", "llm_id": llm.ID, "model": "text-embedding-3-small",
	}))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	doc := decodeEmbedder(t, w.Body.Bytes())
	assert.Equal(t, true, doc.Data.Attributes["linked"])
	assert.Equal(t, "Prod", doc.Data.Attributes["llm_name"])
	assert.Equal(t, "openai", doc.Data.Attributes["vendor"])
	assert.EqualValues(t, 75, doc.Data.Attributes["privacy_score"], "a linked embedder shows the LLM's score")
}

func TestEmbedderHandlers_ConflictsListDependents(t *testing.T) {
	a, service, db := setupEmbedderAPI(t)
	r := a.Router()

	e, err := service.CreateEmbedder(&models.Embedder{Name: "used", Vendor: models.OPENAI, ModelName: "m1", PrivacyScore: 90}, 1)
	require.NoError(t, err)
	_, err = service.CreateDatasource("Handbook", "", "", "", "", 40, 1, nil, "", "pgvector", "", "db",
		services.EmbedderInput{EmbedderID: &e.ID}, true)
	require.NoError(t, err)
	path := fmt.Sprintf("/api/v1/embedders/%d", e.ID)

	w := apitest.PerformRequest(r, "DELETE", path, nil)
	require.Equal(t, http.StatusConflict, w.Code)
	assert.Contains(t, errorDetail(t, w.Body.Bytes()), "Handbook")

	w = apitest.PerformRequest(r, "PATCH", path, embedderBody(map[string]interface{}{
		"name": "used", "vendor": "openai", "api_key": "[redacted]", "model": "m2", "privacy_score": 90,
	}))
	require.Equal(t, http.StatusConflict, w.Code, "model change while a datasource uses it")
	assert.Contains(t, errorDetail(t, w.Body.Bytes()), "re-process")

	w = apitest.PerformRequest(r, "GET", path+"/dependents", nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "Handbook")

	// Deleting the LLM a linked embedder uses is refused too.
	llm := &models.LLM{Name: "Linked LLM", Vendor: models.OPENAI, Active: true}
	require.NoError(t, db.Create(llm).Error)
	_, err = service.CreateEmbedder(&models.Embedder{Name: "on-llm", LLMID: &llm.ID, ModelName: "m"}, 1)
	require.NoError(t, err)
	w = apitest.PerformRequest(r, "DELETE", fmt.Sprintf("/api/v1/llms/%d", llm.ID), nil)
	assert.Equal(t, http.StatusConflict, w.Code, w.Body.String())
}

func TestEmbedderHandlers_Vendors(t *testing.T) {
	a, _, _ := setupEmbedderAPI(t)
	for _, path := range []string{"/api/v1/embedders/vendors", "/api/v1/vendors/embedders"} {
		w := apitest.PerformRequest(a.Router(), "GET", path, nil)
		require.Equal(t, http.StatusOK, w.Code, path)
		var list VendorListResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &list))
		assert.Contains(t, list.Data, "huggingface", path)
		assert.Contains(t, list.Data, "openai", path)
		assert.NotContains(t, list.Data, "anthropic", path)
	}
}

// The datasource API keeps its embed_* shape and gains embedder_id; writes
// may name an embedder directly or use the legacy fields.
func TestDatasourceHandlers_EmbedderFields(t *testing.T) {
	a, service, _ := setupEmbedderAPI(t)
	r := a.Router()

	e, err := service.CreateEmbedder(&models.Embedder{Name: "ollama", Vendor: models.OLLAMA, Endpoint: "http://ollama:11434", ModelName: "nomic-embed-text", PrivacyScore: 100}, 1)
	require.NoError(t, err)

	body := func(attrs map[string]interface{}) map[string]interface{} {
		base := map[string]interface{}{"name": "Docs", "privacy_score": 50, "db_source_type": "pgvector", "db_name": "docs", "active": true}
		for k, v := range attrs {
			base[k] = v
		}
		return map[string]interface{}{"data": map[string]interface{}{"type": "datasources", "attributes": base}}
	}

	w := apitest.PerformRequest(r, "POST", "/api/v1/datasources", body(map[string]interface{}{"embedder_id": e.ID}))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var created struct {
		Data DatasourceResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	attrs := created.Data.Attributes
	assert.Equal(t, e.ID, *attrs.EmbedderID)
	assert.Equal(t, "ollama", attrs.EmbedderName)
	assert.Equal(t, "ollama", attrs.EmbedVendor, "embed_* fields are the flattened embedder")
	assert.Equal(t, "http://ollama:11434", attrs.EmbedUrl)
	assert.Equal(t, "nomic-embed-text", attrs.EmbedModel)
	assert.False(t, attrs.HasEmbedAPIKey)

	w = apitest.PerformRequest(r, "POST", "/api/v1/datasources", body(map[string]interface{}{
		"name": "Legacy", "embed_vendor": "openai", "embed_url": "https://api.openai.com/v1", "embed_api_key": "sk-x", "embed_model": "text-embedding-3-small",
	}))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	assert.NotNil(t, created.Data.Attributes.EmbedderID, "legacy fields are resolved to an embedder")
	assert.Equal(t, "[redacted]", created.Data.Attributes.EmbedAPIKey)
	assert.True(t, created.Data.Attributes.HasEmbedAPIKey)

	low, err := service.CreateEmbedder(&models.Embedder{Name: "low", Vendor: models.OPENAI, ModelName: "m", PrivacyScore: 10}, 1)
	require.NoError(t, err)
	w = apitest.PerformRequest(r, "POST", "/api/v1/datasources", body(map[string]interface{}{"name": "Too private", "embedder_id": low.ID}))
	assert.Equal(t, http.StatusBadRequest, w.Code, "an embedder trusted with less than the datasource is refused")
}
