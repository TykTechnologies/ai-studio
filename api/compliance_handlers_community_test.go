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

// TestComplianceCommunityEdition tests that compliance endpoints return enterprise feature error in CE
func TestComplianceCommunityEdition_ReturnsEnterpriseError(t *testing.T) {
	db := apitest.SetupTestDB(t)
	service := apitest.SetupTestService(db)
	config := apitest.SetupTestAuthConfig(db, service)
	authService := apitest.SetupTestAuthService(db, service)

	api := NewAPI(service, true, authService, config, nil, emptyFile, nil)
	api.InitComplianceService()

	// Setup router
	gin.SetMode(gin.TestMode)
	r := gin.New()
	v1 := r.Group("/api/v1")
	v1.GET("/compliance/available", api.isComplianceAvailable)
	v1.GET("/compliance/summary", api.getComplianceSummary)
	v1.GET("/compliance/high-risk-apps", api.getHighRiskApps)
	v1.GET("/compliance/access-issues", api.getAccessIssues)
	v1.GET("/compliance/policy-violations", api.getPolicyViolations)
	v1.GET("/compliance/budget-alerts", api.getBudgetAlerts)
	v1.GET("/compliance/errors", api.getComplianceErrors)
	v1.GET("/compliance/app/:id/risk-profile", api.getAppRiskProfile)
	v1.GET("/compliance/export", api.exportComplianceData)

	t.Run("isComplianceAvailable returns false for CE", func(t *testing.T) {
		w := apitest.PerformRequest(r, "GET", "/api/v1/compliance/available", nil)

		assert.Equal(t, http.StatusOK, w.Code)

		var response struct {
			Available bool `json:"available"`
		}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.False(t, response.Available, "Compliance should not be available in CE")
	})

	t.Run("getComplianceSummary returns 403 in CE", func(t *testing.T) {
		w := apitest.PerformRequest(r, "GET", "/api/v1/compliance/summary", nil)

		assert.Equal(t, http.StatusForbidden, w.Code)

		var errResp ErrorResponse
		err := json.Unmarshal(w.Body.Bytes(), &errResp)
		assert.NoError(t, err)
		if len(errResp.Errors) > 0 {
			assert.Contains(t, errResp.Errors[0].Detail, "Enterprise Edition feature")
		}
	})

	t.Run("getHighRiskApps returns 403 in CE", func(t *testing.T) {
		w := apitest.PerformRequest(r, "GET", "/api/v1/compliance/high-risk-apps", nil)

		assert.Equal(t, http.StatusForbidden, w.Code)

		var errResp ErrorResponse
		err := json.Unmarshal(w.Body.Bytes(), &errResp)
		assert.NoError(t, err)
		if len(errResp.Errors) > 0 {
			assert.Contains(t, errResp.Errors[0].Detail, "Enterprise Edition feature")
		}
	})

	t.Run("getAccessIssues returns 403 in CE", func(t *testing.T) {
		w := apitest.PerformRequest(r, "GET", "/api/v1/compliance/access-issues", nil)

		assert.Equal(t, http.StatusForbidden, w.Code)

		var errResp ErrorResponse
		err := json.Unmarshal(w.Body.Bytes(), &errResp)
		assert.NoError(t, err)
		if len(errResp.Errors) > 0 {
			assert.Contains(t, errResp.Errors[0].Detail, "Enterprise Edition feature")
		}
	})

	t.Run("getPolicyViolations returns 403 in CE", func(t *testing.T) {
		w := apitest.PerformRequest(r, "GET", "/api/v1/compliance/policy-violations", nil)

		assert.Equal(t, http.StatusForbidden, w.Code)

		var errResp ErrorResponse
		err := json.Unmarshal(w.Body.Bytes(), &errResp)
		assert.NoError(t, err)
		if len(errResp.Errors) > 0 {
			assert.Contains(t, errResp.Errors[0].Detail, "Enterprise Edition feature")
		}
	})

	t.Run("getBudgetAlerts returns 403 in CE", func(t *testing.T) {
		w := apitest.PerformRequest(r, "GET", "/api/v1/compliance/budget-alerts", nil)

		assert.Equal(t, http.StatusForbidden, w.Code)

		var errResp ErrorResponse
		err := json.Unmarshal(w.Body.Bytes(), &errResp)
		assert.NoError(t, err)
		if len(errResp.Errors) > 0 {
			assert.Contains(t, errResp.Errors[0].Detail, "Enterprise Edition feature")
		}
	})

	t.Run("getComplianceErrors returns 403 in CE", func(t *testing.T) {
		w := apitest.PerformRequest(r, "GET", "/api/v1/compliance/errors", nil)

		assert.Equal(t, http.StatusForbidden, w.Code)

		var errResp ErrorResponse
		err := json.Unmarshal(w.Body.Bytes(), &errResp)
		assert.NoError(t, err)
		if len(errResp.Errors) > 0 {
			assert.Contains(t, errResp.Errors[0].Detail, "Enterprise Edition feature")
		}
	})

	t.Run("getAppRiskProfile returns 403 in CE", func(t *testing.T) {
		w := apitest.PerformRequest(r, "GET", "/api/v1/compliance/app/1/risk-profile", nil)

		assert.Equal(t, http.StatusForbidden, w.Code)

		var errResp ErrorResponse
		err := json.Unmarshal(w.Body.Bytes(), &errResp)
		assert.NoError(t, err)
		if len(errResp.Errors) > 0 {
			assert.Contains(t, errResp.Errors[0].Detail, "Enterprise Edition feature")
		}
	})

	t.Run("exportComplianceData returns 403 in CE", func(t *testing.T) {
		w := apitest.PerformRequest(r, "GET", "/api/v1/compliance/export?view=summary", nil)

		assert.Equal(t, http.StatusForbidden, w.Code)

		var errResp ErrorResponse
		err := json.Unmarshal(w.Body.Bytes(), &errResp)
		assert.NoError(t, err)
		if len(errResp.Errors) > 0 {
			assert.Contains(t, errResp.Errors[0].Detail, "Enterprise Edition feature")
		}
	})
}
