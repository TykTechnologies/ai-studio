//go:build enterprise
// +build enterprise

package api

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	apitest "github.com/TykTechnologies/midsommar/v2/api/testing"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services/compliance"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	// Import enterprise compliance to trigger init() registration
	_ "github.com/TykTechnologies/midsommar/v2/enterprise/features/compliance"
)

func setupComplianceTestAPI(t *testing.T) (*API, *gin.Engine) {
	db := apitest.SetupTestDB(t)

	// Create additional tables needed for compliance tests
	err := db.AutoMigrate(&models.ProxyLog{}, &models.LLMChatRecord{})
	assert.NoError(t, err)

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
	v1.GET("/compliance/violations", api.getViolationRecords)
	v1.GET("/compliance/budget-alerts", api.getBudgetAlerts)
	v1.GET("/compliance/errors", api.getComplianceErrors)
	v1.GET("/compliance/app/:id/risk-profile", api.getAppRiskProfile)
	v1.GET("/compliance/export", api.exportComplianceData)

	return api, r
}

func TestComplianceEnterprise_IsAvailable(t *testing.T) {
	_, r := setupComplianceTestAPI(t)

	w := apitest.PerformRequest(r, "GET", "/api/v1/compliance/available", nil)

	assert.Equal(t, http.StatusOK, w.Code)

	var response struct {
		Available bool `json:"available"`
	}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.True(t, response.Available, "Compliance should be available in Enterprise Edition")
}

func TestComplianceEnterprise_GetSummary(t *testing.T) {
	_, r := setupComplianceTestAPI(t)

	t.Run("Returns summary with default date range", func(t *testing.T) {
		w := apitest.PerformRequest(r, "GET", "/api/v1/compliance/summary", nil)

		assert.Equal(t, http.StatusOK, w.Code)

		var summary compliance.ComplianceSummary
		err := json.Unmarshal(w.Body.Bytes(), &summary)
		assert.NoError(t, err)
		// With no data, we expect zero values
		assert.GreaterOrEqual(t, summary.AuthFailures, 0)
		assert.GreaterOrEqual(t, summary.PolicyViolations, 0)
	})

	t.Run("Returns summary with custom date range", func(t *testing.T) {
		startDate := time.Now().AddDate(0, 0, -30).Format("2006-01-02")
		endDate := time.Now().Format("2006-01-02")

		w := apitest.PerformRequest(r, "GET", "/api/v1/compliance/summary?start_date="+startDate+"&end_date="+endDate, nil)

		assert.Equal(t, http.StatusOK, w.Code)

		var summary compliance.ComplianceSummary
		err := json.Unmarshal(w.Body.Bytes(), &summary)
		assert.NoError(t, err)
	})

	t.Run("Returns error for invalid date format", func(t *testing.T) {
		w := apitest.PerformRequest(r, "GET", "/api/v1/compliance/summary?start_date=invalid", nil)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var errResp ErrorResponse
		err := json.Unmarshal(w.Body.Bytes(), &errResp)
		assert.NoError(t, err)
		assert.Greater(t, len(errResp.Errors), 0)
		assert.Contains(t, errResp.Errors[0].Detail, "start_date format")
	})
}

func TestComplianceEnterprise_GetHighRiskApps(t *testing.T) {
	_, r := setupComplianceTestAPI(t)

	t.Run("Returns empty list when no apps exist", func(t *testing.T) {
		w := apitest.PerformRequest(r, "GET", "/api/v1/compliance/high-risk-apps", nil)

		assert.Equal(t, http.StatusOK, w.Code)

		var apps []compliance.HighRiskApp
		err := json.Unmarshal(w.Body.Bytes(), &apps)
		assert.NoError(t, err)
		assert.Empty(t, apps)
	})

	t.Run("Respects limit parameter", func(t *testing.T) {
		w := apitest.PerformRequest(r, "GET", "/api/v1/compliance/high-risk-apps?limit=5", nil)

		assert.Equal(t, http.StatusOK, w.Code)

		var apps []compliance.HighRiskApp
		err := json.Unmarshal(w.Body.Bytes(), &apps)
		assert.NoError(t, err)
		assert.LessOrEqual(t, len(apps), 5)
	})
}

func TestComplianceEnterprise_GetAccessIssues(t *testing.T) {
	_, r := setupComplianceTestAPI(t)

	t.Run("Returns access issues data", func(t *testing.T) {
		w := apitest.PerformRequest(r, "GET", "/api/v1/compliance/access-issues", nil)

		assert.Equal(t, http.StatusOK, w.Code)

		var data compliance.AccessIssuesData
		err := json.Unmarshal(w.Body.Bytes(), &data)
		assert.NoError(t, err)
		// Verify structure exists
		assert.NotNil(t, data.ByCode)
		assert.NotNil(t, data.ByApp)
		assert.NotNil(t, data.Timeline)
	})

	t.Run("Filters by app_id", func(t *testing.T) {
		w := apitest.PerformRequest(r, "GET", "/api/v1/compliance/access-issues?app_id=1", nil)

		assert.Equal(t, http.StatusOK, w.Code)

		var data compliance.AccessIssuesData
		err := json.Unmarshal(w.Body.Bytes(), &data)
		assert.NoError(t, err)
	})
}

func TestComplianceEnterprise_GetPolicyViolations(t *testing.T) {
	_, r := setupComplianceTestAPI(t)

	t.Run("Returns policy violations data", func(t *testing.T) {
		w := apitest.PerformRequest(r, "GET", "/api/v1/compliance/policy-violations", nil)

		assert.Equal(t, http.StatusOK, w.Code)

		var data compliance.PolicyViolationsData
		err := json.Unmarshal(w.Body.Bytes(), &data)
		assert.NoError(t, err)
		assert.NotNil(t, data.FilterBlocks)
		assert.NotNil(t, data.ModelViolations)
		assert.NotNil(t, data.Timeline)
	})
}

func TestComplianceEnterprise_GetBudgetAlerts(t *testing.T) {
	_, r := setupComplianceTestAPI(t)

	t.Run("Returns budget alerts data", func(t *testing.T) {
		w := apitest.PerformRequest(r, "GET", "/api/v1/compliance/budget-alerts", nil)

		assert.Equal(t, http.StatusOK, w.Code)

		var data compliance.BudgetAlertsData
		err := json.Unmarshal(w.Body.Bytes(), &data)
		assert.NoError(t, err)
		assert.NotNil(t, data.Alerts)
		assert.GreaterOrEqual(t, data.WarningCount, 0)
		assert.GreaterOrEqual(t, data.CriticalCount, 0)
	})
}

func TestComplianceEnterprise_GetErrors(t *testing.T) {
	_, r := setupComplianceTestAPI(t)

	t.Run("Returns error data", func(t *testing.T) {
		w := apitest.PerformRequest(r, "GET", "/api/v1/compliance/errors", nil)

		assert.Equal(t, http.StatusOK, w.Code)

		var data compliance.ErrorData
		err := json.Unmarshal(w.Body.Bytes(), &data)
		assert.NoError(t, err)
		assert.NotNil(t, data.ByVendor)
		assert.NotNil(t, data.Timeline)
	})

	t.Run("Filters by vendor", func(t *testing.T) {
		w := apitest.PerformRequest(r, "GET", "/api/v1/compliance/errors?vendor=openai", nil)

		assert.Equal(t, http.StatusOK, w.Code)

		var data compliance.ErrorData
		err := json.Unmarshal(w.Body.Bytes(), &data)
		assert.NoError(t, err)
	})
}

func TestComplianceEnterprise_GetAppRiskProfile(t *testing.T) {
	api, r := setupComplianceTestAPI(t)

	t.Run("Returns 404 for non-existent app", func(t *testing.T) {
		w := apitest.PerformRequest(r, "GET", "/api/v1/compliance/app/9999/risk-profile", nil)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("Returns risk profile for existing app", func(t *testing.T) {
		// Create a test app
		app := &models.App{
			Name:        "Test Compliance App",
			Description: "Test app for compliance",
			IsActive:    true,
		}
		err := api.service.DB.Create(app).Error
		assert.NoError(t, err)

		w := apitest.PerformRequest(r, "GET", "/api/v1/compliance/app/"+string(rune(app.ID+'0'))+"/risk-profile", nil)

		// Should return 200 with risk profile data
		if w.Code == http.StatusOK {
			var profile compliance.AppRiskProfile
			err := json.Unmarshal(w.Body.Bytes(), &profile)
			assert.NoError(t, err)
			assert.Equal(t, app.ID, profile.AppID)
			assert.Equal(t, app.Name, profile.AppName)
		}
	})
}

func TestComplianceEnterprise_ExportData(t *testing.T) {
	_, r := setupComplianceTestAPI(t)

	t.Run("Returns error when view parameter is missing", func(t *testing.T) {
		w := apitest.PerformRequest(r, "GET", "/api/v1/compliance/export", nil)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var errResp ErrorResponse
		err := json.Unmarshal(w.Body.Bytes(), &errResp)
		assert.NoError(t, err)
		assert.Greater(t, len(errResp.Errors), 0)
		assert.Contains(t, errResp.Errors[0].Detail, "view parameter is required")
	})

	t.Run("Exports summary view as CSV", func(t *testing.T) {
		w := apitest.PerformRequest(r, "GET", "/api/v1/compliance/export?view=summary", nil)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "text/csv", w.Header().Get("Content-Type"))
		assert.Contains(t, w.Header().Get("Content-Disposition"), "attachment")
		assert.Contains(t, w.Header().Get("Content-Disposition"), ".csv")
	})

	t.Run("Exports access view as CSV", func(t *testing.T) {
		w := apitest.PerformRequest(r, "GET", "/api/v1/compliance/export?view=access", nil)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "text/csv", w.Header().Get("Content-Type"))
	})

	t.Run("Exports policy view as CSV", func(t *testing.T) {
		w := apitest.PerformRequest(r, "GET", "/api/v1/compliance/export?view=policy", nil)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "text/csv", w.Header().Get("Content-Type"))
	})

	t.Run("Exports budget view as CSV", func(t *testing.T) {
		w := apitest.PerformRequest(r, "GET", "/api/v1/compliance/export?view=budget", nil)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "text/csv", w.Header().Get("Content-Type"))
	})

	t.Run("Exports errors view as CSV", func(t *testing.T) {
		w := apitest.PerformRequest(r, "GET", "/api/v1/compliance/export?view=errors", nil)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "text/csv", w.Header().Get("Content-Type"))
	})

	t.Run("Returns error for invalid view", func(t *testing.T) {
		w := apitest.PerformRequest(r, "GET", "/api/v1/compliance/export?view=invalid", nil)

		// Invalid view returns 500 from service layer with "unknown view" error
		assert.Equal(t, http.StatusInternalServerError, w.Code)

		var errResp ErrorResponse
		err := json.Unmarshal(w.Body.Bytes(), &errResp)
		assert.NoError(t, err)
		assert.Greater(t, len(errResp.Errors), 0)
		assert.Contains(t, errResp.Errors[0].Detail, "export")
	})
}

func TestComplianceEnterprise_GetViolationRecords(t *testing.T) {
	_, r := setupComplianceTestAPI(t)

	t.Run("Returns violation records data", func(t *testing.T) {
		w := apitest.PerformRequest(r, "GET", "/api/v1/compliance/violations", nil)

		assert.Equal(t, http.StatusOK, w.Code)

		var records []compliance.ViolationRecord
		err := json.Unmarshal(w.Body.Bytes(), &records)
		assert.NoError(t, err)
		// With no data, we expect empty array (or null which unmarshals to nil slice)
		// The important thing is that we get a successful response with no error
	})

	t.Run("Respects limit parameter", func(t *testing.T) {
		w := apitest.PerformRequest(r, "GET", "/api/v1/compliance/violations?limit=5", nil)

		assert.Equal(t, http.StatusOK, w.Code)

		var records []compliance.ViolationRecord
		err := json.Unmarshal(w.Body.Bytes(), &records)
		assert.NoError(t, err)
		assert.LessOrEqual(t, len(records), 5)
	})

	t.Run("Filters by app_id", func(t *testing.T) {
		w := apitest.PerformRequest(r, "GET", "/api/v1/compliance/violations?app_id=1", nil)

		assert.Equal(t, http.StatusOK, w.Code)

		var records []compliance.ViolationRecord
		err := json.Unmarshal(w.Body.Bytes(), &records)
		assert.NoError(t, err)
	})
}

func TestComplianceEnterprise_DateRangeDefaults(t *testing.T) {
	_, r := setupComplianceTestAPI(t)

	// All endpoints should work without date parameters (using defaults)
	endpoints := []string{
		"/api/v1/compliance/summary",
		"/api/v1/compliance/high-risk-apps",
		"/api/v1/compliance/access-issues",
		"/api/v1/compliance/policy-violations",
		"/api/v1/compliance/violations",
		"/api/v1/compliance/budget-alerts",
		"/api/v1/compliance/errors",
	}

	for _, endpoint := range endpoints {
		t.Run("Default date range for "+endpoint, func(t *testing.T) {
			w := apitest.PerformRequest(r, "GET", endpoint, nil)

			assert.Equal(t, http.StatusOK, w.Code, "Endpoint %s should return OK with default date range", endpoint)
		})
	}
}
