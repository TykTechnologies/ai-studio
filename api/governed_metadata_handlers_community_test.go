//go:build !enterprise
// +build !enterprise

package api

import (
	"encoding/json"
	"net/http"
	"testing"

	apitest "github.com/TykTechnologies/midsommar/v2/api/testing"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func setupGovernedMetadataCERouter(t *testing.T) *gin.Engine {
	t.Helper()
	db := apitest.SetupTestDB(t)
	service := apitest.SetupTestService(db)
	config := apitest.SetupTestAuthConfig(db, service)
	authService := apitest.SetupTestAuthService(db, service)
	api := NewAPI(service, true, authService, config, nil, emptyFile, nil)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	v1 := r.Group("/api/v1")
	v1.GET("/metadata/available", api.isGovernedMetadataAvailable)
	v1.GET("/metadata/object-types", api.listMetadataObjectTypes)
	v1.GET("/metadata/schemas", api.listMetadataSchemas)
	v1.POST("/metadata/schemas", api.createMetadataSchema)
	v1.GET("/metadata/schemas/resolve", api.resolveMetadataSchema)
	v1.GET("/metadata/vocabularies", api.listMetadataVocabularies)
	v1.POST("/metadata/validate", api.validateObjectMetadata)
	v1.GET("/metadata/objects/:object_type/:object_id", api.getObjectMetadata)
	v1.PUT("/metadata/objects/:object_type/:object_id", api.setObjectMetadata)
	v1.GET("/metadata/compliance", api.getMetadataComplianceReport)
	return r
}

// TestGovernedMetadataCommunityEdition verifies CE reports the feature as unavailable
// and rejects management operations with 403, while read paths degrade gracefully.
func TestGovernedMetadataCommunityEdition(t *testing.T) {
	r := setupGovernedMetadataCERouter(t)

	t.Run("available is false", func(t *testing.T) {
		w := apitest.PerformRequest(r, "GET", "/api/v1/metadata/available", nil)
		assert.Equal(t, http.StatusOK, w.Code)
		var resp struct {
			Available bool `json:"available"`
		}
		assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.False(t, resp.Available)
	})

	for _, tc := range []struct{ method, path string }{
		{"GET", "/api/v1/metadata/schemas"},
		{"POST", "/api/v1/metadata/schemas"},
		{"GET", "/api/v1/metadata/vocabularies"},
		{"PUT", "/api/v1/metadata/objects/llm/1"},
		{"GET", "/api/v1/metadata/compliance"},
	} {
		t.Run(tc.method+" "+tc.path+" returns 403", func(t *testing.T) {
			var body interface{}
			if tc.method != "GET" {
				body = map[string]interface{}{"data": map[string]interface{}{"attributes": map[string]interface{}{"name": "x"}}, "values": map[string]interface{}{}}
			}
			w := apitest.PerformRequest(r, tc.method, tc.path, body)
			assert.Equal(t, http.StatusForbidden, w.Code)
			var errResp ErrorResponse
			assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &errResp))
			if assert.NotEmpty(t, errResp.Errors) {
				assert.Equal(t, "Enterprise Feature", errResp.Errors[0].Title)
				assert.Contains(t, errResp.Errors[0].Detail, "Enterprise Edition feature")
			}
		})
	}

	t.Run("read paths degrade gracefully", func(t *testing.T) {
		w := apitest.PerformRequest(r, "GET", "/api/v1/metadata/object-types", nil)
		assert.Equal(t, http.StatusOK, w.Code)

		w = apitest.PerformRequest(r, "GET", "/api/v1/metadata/schemas/resolve?object_type=llm", nil)
		assert.Equal(t, http.StatusOK, w.Code)
		var resolved struct {
			Fields      []interface{} `json:"fields"`
			Enforcement string        `json:"enforcement"`
		}
		assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resolved))
		assert.Empty(t, resolved.Fields)
		assert.Equal(t, "advisory", resolved.Enforcement)

		w = apitest.PerformRequest(r, "POST", "/api/v1/metadata/validate", map[string]interface{}{"object_type": "llm", "values": map[string]interface{}{"x": 1}})
		assert.Equal(t, http.StatusOK, w.Code)
		var result struct {
			Valid bool `json:"valid"`
		}
		assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
		assert.True(t, result.Valid)

		w = apitest.PerformRequest(r, "GET", "/api/v1/metadata/objects/llm/1", nil)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}
