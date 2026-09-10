//go:build enterprise
// +build enterprise

package api

import (
	"encoding/json"
	"net/http"
	"testing"

	apitest "github.com/TykTechnologies/midsommar/v2/api/testing"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services/governed_metadata"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGovernedMetadataEnforcementOnLLMHandlers exercises the seam between the
// built-in LLM handlers and the governed metadata service.
func TestGovernedMetadataEnforcementOnLLMHandlers(t *testing.T) {
	t.Setenv("TYK_AI_SECRET_KEY", "test-key")
	r, api, db := setupGovernedMetadataRouter(t)
	v1 := r.Group("/api/v1")
	v1.POST("/llms", api.createLLM)
	v1.GET("/llms/:id", api.getLLM)
	v1.PATCH("/llms/:id", api.updateLLM)
	v1.DELETE("/llms/:id", api.deleteLLM)
	v1.GET("/llms", api.listLLMs)

	svc := api.governedMetadata()
	require.NoError(t, svc.CreateVocabulary(&models.MetadataVocabulary{Name: "Risk", Slug: "risk_tier",
		Terms: []models.VocabularyTerm{{Value: "high", Label: "High"}, {Value: "low", Label: "Low"}}}))
	schema := &models.MetadataSchema{Name: "Core", Slug: "core", AppliesTo: []string{"*"}, Active: true,
		Enforcement: models.MetadataEnforcementEnforce,
		Fields: []models.MetadataFieldDef{
			{Key: "risk_tier", Label: "Risk tier", Type: "vocabulary", VocabularySlug: "risk_tier", Required: true, PortalVisible: true},
			{Key: "support_contact", Label: "Support", Type: "email"},
		}}
	require.NoError(t, svc.CreateSchema(schema))

	llmBody := func(governed interface{}) map[string]interface{} {
		attrs := map[string]interface{}{
			"name": "gpt", "api_key": "k", "api_endpoint": "https://api.openai.com/v1", "vendor": "openai",
			"active": false, "default_model": "gpt-4", "privacy_score": 50,
		}
		if governed != nil {
			attrs["governed_metadata"] = governed
		}
		return map[string]interface{}{"data": map[string]interface{}{"type": "LLM", "attributes": attrs}}
	}

	// Enforced: omitting governed_metadata on create is treated as {} → 422.
	w := apitest.PerformRequest(r, "POST", "/api/v1/llms", llmBody(nil))
	require.Equal(t, http.StatusUnprocessableEntity, w.Code, w.Body.String())
	var verr MetadataValidationErrorResponse
	decode(t, w.Body.Bytes(), &verr)
	require.NotEmpty(t, verr.Errors)
	assert.Equal(t, "/data/attributes/governed_metadata/risk_tier", verr.Errors[0].Source.Pointer)
	var count int64
	db.Model(&models.LLM{}).Count(&count)
	assert.Equal(t, int64(0), count, "object must not be created when metadata is rejected")

	// Enforced: invalid value → 422, nothing created.
	w = apitest.PerformRequest(r, "POST", "/api/v1/llms", llmBody(map[string]interface{}{"risk_tier": "nonsense"}))
	require.Equal(t, http.StatusUnprocessableEntity, w.Code, w.Body.String())

	// Valid → 201 with governed_metadata + status at the top level of data.
	w = apitest.PerformRequest(r, "POST", "/api/v1/llms", llmBody(map[string]interface{}{"risk_tier": "high", "support_contact": "s@x.io"}))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	created := parseObjectResponse(t, w.Body.Bytes())
	assert.Equal(t, "high", created.Data.GovernedMetadata["risk_tier"])
	assert.Equal(t, "valid", created.Data.GovernedMetadataStatus)
	assert.Nil(t, created.Meta)
	_, inAttrs := created.Data.Attributes["governed_metadata"]
	assert.False(t, inAttrs, "governed_metadata lives beside attributes, not inside")
	id := created.Data.ID

	rec, err := svc.GetObjectMetadata("llm", id)
	require.NoError(t, err)
	assert.Equal(t, uint(1), rec.UpdatedByUserID)
	assert.Equal(t, "admin", rec.UpdatedBySource)

	// GET and list carry it.
	w = apitest.PerformRequest(r, "GET", "/api/v1/llms/"+id, nil)
	require.Equal(t, http.StatusOK, w.Code)
	created = parseObjectResponse(t, w.Body.Bytes())
	assert.Equal(t, "high", created.Data.GovernedMetadata["risk_tier"])

	w = apitest.PerformRequest(r, "GET", "/api/v1/llms", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var list struct {
		Data []struct {
			GovernedMetadataStatus string `json:"governed_metadata_status"`
		} `json:"data"`
	}
	decode(t, w.Body.Bytes(), &list)
	require.Len(t, list.Data, 1)
	assert.Equal(t, "valid", list.Data[0].GovernedMetadataStatus)

	// PATCH without the attribute leaves metadata untouched.
	w = apitest.PerformRequest(r, "PATCH", "/api/v1/llms/"+id, map[string]interface{}{"data": map[string]interface{}{"type": "LLM", "attributes": map[string]interface{}{"short_description": "changed"}}})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	created = parseObjectResponse(t, w.Body.Bytes())
	assert.Equal(t, "high", created.Data.GovernedMetadata["risk_tier"])
	assert.Equal(t, "s@x.io", created.Data.GovernedMetadata["support_contact"])

	// PATCH with an invalid value under enforcement → 422 and object unchanged.
	w = apitest.PerformRequest(r, "PATCH", "/api/v1/llms/"+id, map[string]interface{}{"data": map[string]interface{}{"type": "LLM", "attributes": map[string]interface{}{"governed_metadata": map[string]interface{}{"risk_tier": "high", "support_contact": "bad"}}}})
	require.Equal(t, http.StatusUnprocessableEntity, w.Code, w.Body.String())

	// PATCH with a full replacement drops support_contact.
	w = apitest.PerformRequest(r, "PATCH", "/api/v1/llms/"+id, map[string]interface{}{"data": map[string]interface{}{"type": "LLM", "attributes": map[string]interface{}{"governed_metadata": map[string]interface{}{"risk_tier": "low"}}}})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	created = parseObjectResponse(t, w.Body.Bytes())
	assert.Equal(t, "low", created.Data.GovernedMetadata["risk_tier"])
	_, hasContact := created.Data.GovernedMetadata["support_contact"]
	assert.False(t, hasContact)

	// Portal shape: display-ready list with portal-visible fields only.
	portal := api.withLLMGovernedMetadata([]LLMResponse{{ID: id}}, true)
	display, ok := portal[0].GovernedMetadata.([]governed_metadata.DisplayField)
	require.True(t, ok, "%T", portal[0].GovernedMetadata)
	require.Len(t, display, 1)
	assert.Equal(t, "risk_tier", display[0].Key)
	assert.Equal(t, "Risk tier", display[0].Label)
	assert.Equal(t, "Low", display[0].Value)
	raw, _ := json.Marshal(portal[0])
	assert.Contains(t, string(raw), `"governed_metadata":[{"key":"risk_tier"`)
	assert.NotContains(t, string(raw), "governed_metadata_status")

	// Advisory: invalid values are accepted and status recorded.
	schema.Enforcement = models.MetadataEnforcementAdvisory
	require.NoError(t, svc.UpdateSchema(schema))
	w = apitest.PerformRequest(r, "POST", "/api/v1/llms", llmBody(map[string]interface{}{"support_contact": "bad"}))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	created = parseObjectResponse(t, w.Body.Bytes())
	assert.Equal(t, "invalid", created.Data.GovernedMetadataStatus)

	// Advisory: omitting the attribute creates without a metadata row.
	w = apitest.PerformRequest(r, "POST", "/api/v1/llms", llmBody(nil))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	created = parseObjectResponse(t, w.Body.Bytes())
	assert.Nil(t, created.Data.GovernedMetadata)
	assert.Empty(t, created.Data.GovernedMetadataStatus)

	// Delete removes the metadata row.
	w = apitest.PerformRequest(r, "DELETE", "/api/v1/llms/"+id, nil)
	require.Equal(t, http.StatusNoContent, w.Code)
	_, err = svc.GetObjectMetadata("llm", id)
	assert.ErrorIs(t, err, governed_metadata.ErrNotFound)
}

// TestGovernedMetadataEnforcementOnToolAndDatasource covers the tool and datasource seams.
func TestGovernedMetadataEnforcementOnToolAndDatasource(t *testing.T) {
	t.Setenv("TYK_AI_SECRET_KEY", "test-key")
	r, api, _ := setupGovernedMetadataRouter(t)
	v1 := r.Group("/api/v1")
	v1.POST("/tools", api.createTool)
	v1.GET("/tools/:id", api.getTool)
	v1.DELETE("/tools/:id", api.deleteTool)
	v1.POST("/datasources", api.createDatasource)
	v1.PATCH("/datasources/:id", api.updateDatasource)
	v1.GET("/datasources/:id", api.getDatasource)

	svc := api.governedMetadata()
	require.NoError(t, svc.CreateSchema(&models.MetadataSchema{Name: "Owners", Slug: "owners", AppliesTo: []string{"tool", "datasource"}, Active: true,
		Enforcement: models.MetadataEnforcementEnforce,
		Fields:      []models.MetadataFieldDef{{Key: "team", Label: "Team", Type: "string", Required: true}}}))

	toolBody := func(governed interface{}) map[string]interface{} {
		attrs := map[string]interface{}{"name": "weather", "tool_type": "REST", "description": "d", "privacy_score": 10, "oas_spec": testOASSpec(`{"openapi": "3.0.0"}`)}
		if governed != nil {
			attrs["governed_metadata"] = governed
		}
		return map[string]interface{}{"data": map[string]interface{}{"type": "tool", "attributes": attrs}}
	}
	w := apitest.PerformRequest(r, "POST", "/api/v1/tools", toolBody(nil))
	require.Equal(t, http.StatusUnprocessableEntity, w.Code, w.Body.String())
	w = apitest.PerformRequest(r, "POST", "/api/v1/tools", toolBody(map[string]interface{}{"team": "platform"}))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	created := parseObjectResponse(t, w.Body.Bytes())
	assert.Equal(t, "platform", created.Data.GovernedMetadata["team"])
	toolID := created.Data.ID
	w = apitest.PerformRequest(r, "GET", "/api/v1/tools/"+toolID, nil)
	require.Equal(t, http.StatusOK, w.Code)
	created = parseObjectResponse(t, w.Body.Bytes())
	assert.Equal(t, "valid", created.Data.GovernedMetadataStatus)
	w = apitest.PerformRequest(r, "DELETE", "/api/v1/tools/"+toolID, nil)
	require.Equal(t, http.StatusNoContent, w.Code)
	_, err := svc.GetObjectMetadata("tool", toolID)
	assert.ErrorIs(t, err, governed_metadata.ErrNotFound)

	dsBody := func(governed interface{}) map[string]interface{} {
		attrs := map[string]interface{}{"name": "docs", "short_description": "d", "db_source_type": "qdrant", "embed_vendor": "openai", "embed_model": "m", "active": true}
		if governed != nil {
			attrs["governed_metadata"] = governed
		}
		return map[string]interface{}{"data": map[string]interface{}{"type": "datasource", "attributes": attrs}}
	}
	w = apitest.PerformRequest(r, "POST", "/api/v1/datasources", dsBody(nil))
	require.Equal(t, http.StatusUnprocessableEntity, w.Code, w.Body.String())
	w = apitest.PerformRequest(r, "POST", "/api/v1/datasources", dsBody(map[string]interface{}{"team": "data"}))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	created = parseObjectResponse(t, w.Body.Bytes())
	assert.Equal(t, "data", created.Data.GovernedMetadata["team"])
	dsID := created.Data.ID

	// Update without the attribute keeps it; with an empty map under enforcement → 422.
	w = apitest.PerformRequest(r, "PATCH", "/api/v1/datasources/"+dsID, dsBody(nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	created = parseObjectResponse(t, w.Body.Bytes())
	assert.Equal(t, "data", created.Data.GovernedMetadata["team"])
	w = apitest.PerformRequest(r, "PATCH", "/api/v1/datasources/"+dsID, dsBody(map[string]interface{}{}))
	require.Equal(t, http.StatusUnprocessableEntity, w.Code, w.Body.String())
}

// objectResponse is the {data, meta} envelope for LLM/Tool/Datasource responses.
type objectResponse struct {
	Data struct {
		ID                     string                 `json:"id"`
		GovernedMetadata       map[string]interface{} `json:"governed_metadata"`
		GovernedMetadataStatus string                 `json:"governed_metadata_status"`
		Attributes             map[string]interface{} `json:"attributes"`
	} `json:"data"`
	Meta map[string]interface{} `json:"meta"`
}

// parseObjectResponse decodes into a fresh struct every time: json.Unmarshal
// merges into a reused non-nil map, which would hide replaced values.
func parseObjectResponse(t *testing.T, body []byte) objectResponse {
	t.Helper()
	var out objectResponse
	decode(t, body, &out)
	return out
}
