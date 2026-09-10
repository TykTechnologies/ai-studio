//go:build enterprise
// +build enterprise

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	apitest "github.com/TykTechnologies/midsommar/v2/api/testing"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupGovernedMetadataRouter(t *testing.T) (*gin.Engine, *API, *gorm.DB) {
	t.Helper()
	db := apitest.SetupTestDB(t)
	service := apitest.SetupTestService(db)
	config := apitest.SetupTestAuthConfig(db, service)
	authService := apitest.SetupTestAuthService(db, service)
	api := NewAPI(service, true, authService, config, nil, emptyFile, nil)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	// Simulate an authenticated admin so audit attribution works.
	admin := &models.User{Email: "admin@test.local", Name: "Admin", IsAdmin: true}
	require.NoError(t, db.Create(admin).Error)
	r.Use(func(c *gin.Context) { c.Set("user", admin); c.Next() })

	v1 := r.Group("/api/v1")
	v1.GET("/metadata/available", api.isGovernedMetadataAvailable)
	v1.GET("/metadata/object-types", api.listMetadataObjectTypes)
	v1.GET("/metadata/schemas", api.listMetadataSchemas)
	v1.POST("/metadata/schemas", api.createMetadataSchema)
	v1.GET("/metadata/schemas/resolve", api.resolveMetadataSchema)
	v1.GET("/metadata/schemas/:id", api.getMetadataSchema)
	v1.PATCH("/metadata/schemas/:id", api.updateMetadataSchema)
	v1.DELETE("/metadata/schemas/:id", api.deleteMetadataSchema)
	v1.GET("/metadata/vocabularies", api.listMetadataVocabularies)
	v1.POST("/metadata/vocabularies", api.createMetadataVocabulary)
	v1.GET("/metadata/vocabularies/:id", api.getMetadataVocabulary)
	v1.PATCH("/metadata/vocabularies/:id", api.updateMetadataVocabulary)
	v1.DELETE("/metadata/vocabularies/:id", api.deleteMetadataVocabulary)
	v1.POST("/metadata/validate", api.validateObjectMetadata)
	v1.GET("/metadata/objects/:object_type/:object_id", api.getObjectMetadata)
	v1.PUT("/metadata/objects/:object_type/:object_id", api.setObjectMetadata)
	v1.DELETE("/metadata/objects/:object_type/:object_id", api.deleteObjectMetadata)
	v1.GET("/metadata/objects/:object_type/:object_id/audit", api.getObjectMetadataAudit)
	v1.GET("/metadata/compliance", api.getMetadataComplianceReport)
	return r, api, db
}

func jsonAPI(attrs map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{"data": map[string]interface{}{"type": "x", "attributes": attrs}}
}

func decode(t *testing.T, body []byte, into interface{}) {
	t.Helper()
	require.NoError(t, json.Unmarshal(body, into), string(body))
}

func TestGovernedMetadataHandlers_Enterprise(t *testing.T) {
	r, _, db := setupGovernedMetadataRouter(t)

	w := apitest.PerformRequest(r, "GET", "/api/v1/metadata/available", nil)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"available":true`)

	// --- vocabulary create / duplicate / update
	w = apitest.PerformRequest(r, "POST", "/api/v1/metadata/vocabularies", jsonAPI(map[string]interface{}{
		"name": "Risk Tier", "slug": "risk_tier",
		"terms": []map[string]interface{}{{"value": "low", "label": "Low"}, {"value": "high", "label": "High"}},
	}))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var vocabResp struct {
		Data struct {
			ID         uint                      `json:"id"`
			Attributes models.MetadataVocabulary `json:"attributes"`
		} `json:"data"`
	}
	decode(t, w.Body.Bytes(), &vocabResp)
	vocabID := vocabResp.Data.ID
	assert.Equal(t, "admin", vocabResp.Data.Attributes.Source)

	w = apitest.PerformRequest(r, "POST", "/api/v1/metadata/vocabularies", jsonAPI(map[string]interface{}{"name": "Dup", "slug": "risk_tier", "terms": []map[string]interface{}{{"value": "x"}}}))
	assert.Equal(t, http.StatusBadRequest, w.Code)

	w = apitest.PerformRequest(r, "PATCH", fmt.Sprintf("/api/v1/metadata/vocabularies/%d", vocabID), jsonAPI(map[string]interface{}{"description": "How risky"}))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	decode(t, w.Body.Bytes(), &vocabResp)
	assert.Equal(t, "How risky", vocabResp.Data.Attributes.Description)
	assert.Len(t, vocabResp.Data.Attributes.Terms, 2, "terms preserved on partial patch")

	// --- schema create with a bad key → 400, then valid
	w = apitest.PerformRequest(r, "POST", "/api/v1/metadata/schemas", jsonAPI(map[string]interface{}{
		"name": "Bad", "applies_to": []string{"llm"}, "fields": []map[string]interface{}{{"key": "Risk Tier", "type": "string"}},
	}))
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "Invalid Definition")

	w = apitest.PerformRequest(r, "POST", "/api/v1/metadata/schemas", jsonAPI(map[string]interface{}{
		"name": "Core", "applies_to": []string{"*"}, "enforcement": "enforce",
		"fields": []map[string]interface{}{
			{"key": "risk_tier", "label": "Risk tier", "type": "vocabulary", "vocabulary_slug": "risk_tier", "required": true, "portal_visible": true},
			{"key": "support_contact", "label": "Support", "type": "email"},
		},
	}))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var schemaResp struct {
		Data struct {
			ID         uint                  `json:"id"`
			Attributes models.MetadataSchema `json:"attributes"`
		} `json:"data"`
	}
	decode(t, w.Body.Bytes(), &schemaResp)
	schemaID := schemaResp.Data.ID
	assert.Equal(t, "core", schemaResp.Data.Attributes.Slug)
	assert.True(t, schemaResp.Data.Attributes.Active)

	// Vocabulary now in use → 409
	w = apitest.PerformRequest(r, "DELETE", fmt.Sprintf("/api/v1/metadata/vocabularies/%d", vocabID), nil)
	assert.Equal(t, http.StatusConflict, w.Code)

	// Colliding key in an overlapping schema → 409
	w = apitest.PerformRequest(r, "POST", "/api/v1/metadata/schemas", jsonAPI(map[string]interface{}{
		"name": "Clash", "applies_to": []string{"tool"}, "fields": []map[string]interface{}{{"key": "risk_tier", "type": "string"}},
	}))
	assert.Equal(t, http.StatusConflict, w.Code)
	assert.Contains(t, w.Body.String(), "Field Key Collision")

	// --- resolve
	w = apitest.PerformRequest(r, "GET", "/api/v1/metadata/schemas/resolve?object_type=tool", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var resolved struct {
		Fields      []models.MetadataFieldDef `json:"fields"`
		Enforcement string                    `json:"enforcement"`
		JSONSchema  map[string]interface{}    `json:"json_schema"`
	}
	decode(t, w.Body.Bytes(), &resolved)
	assert.Len(t, resolved.Fields, 2)
	assert.Equal(t, "enforce", resolved.Enforcement)
	assert.Contains(t, resolved.JSONSchema, "properties")

	// --- validate (no save)
	w = apitest.PerformRequest(r, "POST", "/api/v1/metadata/validate", map[string]interface{}{"object_type": "llm", "values": map[string]interface{}{"support_contact": "nope"}})
	require.Equal(t, http.StatusOK, w.Code)
	var result struct {
		Valid    bool `json:"valid"`
		Enforced bool `json:"enforced"`
		Errors   []struct {
			Field string `json:"field"`
			Code  string `json:"code"`
		} `json:"errors"`
	}
	decode(t, w.Body.Bytes(), &result)
	assert.False(t, result.Valid)
	assert.True(t, result.Enforced)
	assert.Len(t, result.Errors, 2)

	// --- set under enforcement → 422 with pointers
	require.NoError(t, db.Create(&models.LLM{Name: "gpt", Vendor: "openai"}).Error)
	w = apitest.PerformRequest(r, "PUT", "/api/v1/metadata/objects/llm/1", map[string]interface{}{"values": map[string]interface{}{"support_contact": "nope"}})
	require.Equal(t, http.StatusUnprocessableEntity, w.Code, w.Body.String())
	var verr MetadataValidationErrorResponse
	decode(t, w.Body.Bytes(), &verr)
	pointers := []string{}
	for _, e := range verr.Errors {
		assert.Equal(t, "Metadata Validation Failed", e.Title)
		if e.Source != nil {
			pointers = append(pointers, e.Source.Pointer)
		}
	}
	assert.ElementsMatch(t, []string{"/data/attributes/governed_metadata/risk_tier", "/data/attributes/governed_metadata/support_contact"}, pointers)

	// Unknown object type → 400
	w = apitest.PerformRequest(r, "PUT", "/api/v1/metadata/objects/spaceship/1", map[string]interface{}{"values": map[string]interface{}{}})
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// Valid set → 200 with record + validation, audit attributed to the admin
	w = apitest.PerformRequest(r, "PUT", "/api/v1/metadata/objects/llm/1", map[string]interface{}{"values": map[string]interface{}{"risk_tier": "high"}})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var setResp struct {
		Data struct {
			Values           map[string]interface{} `json:"values"`
			ValidationStatus string                 `json:"validation_status"`
			UpdatedByUserID  uint                   `json:"updated_by_user_id"`
			UpdatedBySource  string                 `json:"updated_by_source"`
		} `json:"data"`
		Validation struct {
			Valid bool `json:"valid"`
		} `json:"validation"`
	}
	decode(t, w.Body.Bytes(), &setResp)
	assert.Equal(t, "high", setResp.Data.Values["risk_tier"])
	assert.Equal(t, "valid", setResp.Data.ValidationStatus)
	assert.Equal(t, uint(1), setResp.Data.UpdatedByUserID)
	assert.Equal(t, "admin", setResp.Data.UpdatedBySource)
	assert.True(t, setResp.Validation.Valid)

	// Merge keeps risk_tier
	w = apitest.PerformRequest(r, "PUT", "/api/v1/metadata/objects/llm/1", map[string]interface{}{"values": map[string]interface{}{"support_contact": "a@b.co"}, "merge": true})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	decode(t, w.Body.Bytes(), &setResp)
	assert.Equal(t, "high", setResp.Data.Values["risk_tier"])
	assert.Equal(t, "a@b.co", setResp.Data.Values["support_contact"])

	w = apitest.PerformRequest(r, "GET", "/api/v1/metadata/objects/llm/1", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	w = apitest.PerformRequest(r, "GET", "/api/v1/metadata/objects/llm/1/audit", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var auditResp struct {
		Data []models.ObjectMetadataAudit `json:"data"`
	}
	decode(t, w.Body.Bytes(), &auditResp)
	require.Len(t, auditResp.Data, 2)
	assert.Equal(t, "merge", auditResp.Data[0].Action)
	assert.Equal(t, uint(1), auditResp.Data[0].UserID)

	// --- compliance report: LLM 1 valid, LLM 2 missing
	require.NoError(t, db.Create(&models.LLM{Name: "claude", Vendor: "anthropic"}).Error)
	w = apitest.PerformRequest(r, "GET", "/api/v1/metadata/compliance?object_type=llm", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var report struct {
		Entries []struct {
			ObjectName string `json:"object_name"`
			Status     string `json:"status"`
		} `json:"entries"`
		Counts map[string]int `json:"counts"`
	}
	decode(t, w.Body.Bytes(), &report)
	require.Len(t, report.Entries, 2)
	assert.Equal(t, 1, report.Counts["missing"])
	assert.Equal(t, 1, report.Counts["valid"])

	w = apitest.PerformRequest(r, "GET", "/api/v1/metadata/compliance?status=missing", nil)
	decode(t, w.Body.Bytes(), &report)
	require.Len(t, report.Entries, 1)
	assert.Equal(t, "claude", report.Entries[0].ObjectName)

	// --- schema PATCH toggles enforcement while preserving fields
	w = apitest.PerformRequest(r, "PATCH", fmt.Sprintf("/api/v1/metadata/schemas/%d", schemaID), jsonAPI(map[string]interface{}{"enforcement": "advisory"}))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	decode(t, w.Body.Bytes(), &schemaResp)
	assert.Equal(t, "advisory", schemaResp.Data.Attributes.Enforcement)
	assert.Len(t, schemaResp.Data.Attributes.Fields, 2)
	assert.Equal(t, 2, schemaResp.Data.Attributes.Version)

	// Now invalid values are accepted (advisory) with status recorded
	w = apitest.PerformRequest(r, "PUT", "/api/v1/metadata/objects/llm/2", map[string]interface{}{"values": map[string]interface{}{"support_contact": "nope"}})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	decode(t, w.Body.Bytes(), &setResp)
	assert.Equal(t, "invalid", setResp.Data.ValidationStatus)

	// --- delete metadata, schema, then vocabulary
	w = apitest.PerformRequest(r, "DELETE", "/api/v1/metadata/objects/llm/1", nil)
	assert.Equal(t, http.StatusNoContent, w.Code)
	w = apitest.PerformRequest(r, "GET", "/api/v1/metadata/objects/llm/1", nil)
	assert.Equal(t, http.StatusNotFound, w.Code)

	w = apitest.PerformRequest(r, "DELETE", fmt.Sprintf("/api/v1/metadata/schemas/%d", schemaID), nil)
	assert.Equal(t, http.StatusNoContent, w.Code)
	w = apitest.PerformRequest(r, "DELETE", fmt.Sprintf("/api/v1/metadata/vocabularies/%d", vocabID), nil)
	assert.Equal(t, http.StatusNoContent, w.Code)
	w = apitest.PerformRequest(r, "GET", fmt.Sprintf("/api/v1/metadata/schemas/%d", schemaID), nil)
	assert.Equal(t, http.StatusNotFound, w.Code)

	w = apitest.PerformRequest(r, "GET", "/api/v1/metadata/schemas", nil)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"data":[]`)
}
