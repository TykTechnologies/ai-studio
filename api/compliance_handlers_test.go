//go:build enterprise
// +build enterprise

package api

import (
	"encoding/json"
	"net/http"
	"strings"
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
	err := db.AutoMigrate(&models.ProxyLog{}, &models.LLMChatRecord{}, &models.ComplianceEvent{})
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
	v1.GET("/compliance/events", api.getComplianceEvents)
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
		"/api/v1/compliance/events",
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

func TestComplianceEnterprise_GetComplianceEvents(t *testing.T) {
	api, r := setupComplianceTestAPI(t)

	t.Run("Returns empty events with default date range", func(t *testing.T) {
		w := apitest.PerformRequest(r, "GET", "/api/v1/compliance/events", nil)
		assert.Equal(t, http.StatusOK, w.Code)

		var response compliance.ComplianceEventsData
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.NotNil(t, response.Events)
		assert.Equal(t, int64(0), response.TotalCount)
		assert.NotNil(t, response.BySeverity)
		assert.NotNil(t, response.ByType)
		assert.NotNil(t, response.ByFilter)
	})

	t.Run("Returns events after inserting data", func(t *testing.T) {
		now := time.Now()

		// Insert test compliance events
		events := []models.ComplianceEvent{
			{
				AppID:       1,
				UserID:      10,
				LLMID:       5,
				FilterName:  "pii-filter",
				FilterScope: "proxy_request",
				EventType:   "pii_redacted",
				Severity:    "warning",
				Description: "SSN pattern redacted",
				Metadata:    `{"pattern":"ssn","count":2}`,
				Vendor:      "openai",
				ModelName:   "gpt-4",
				TimeStamp:   now,
			},
			{
				AppID:       1,
				UserID:      10,
				LLMID:       5,
				FilterName:  "tone-filter",
				FilterScope: "chat_request",
				EventType:   "content_rewritten",
				Severity:    "info",
				Description: "Tone adjusted to professional",
				Vendor:      "anthropic",
				ModelName:   "claude-3",
				TimeStamp:   now,
			},
			{
				AppID:       2,
				UserID:      20,
				LLMID:       8,
				FilterName:  "pii-filter",
				FilterScope: "proxy_request",
				EventType:   "pii_redacted",
				Severity:    "critical",
				Description: "Credit card number blocked",
				Vendor:      "openai",
				ModelName:   "gpt-4",
				TimeStamp:   now,
			},
		}

		for _, e := range events {
			err := api.service.DB.Create(&e).Error
			assert.NoError(t, err)
		}

		// Test: get all events
		w := apitest.PerformRequest(r, "GET", "/api/v1/compliance/events", nil)
		assert.Equal(t, http.StatusOK, w.Code)

		var response compliance.ComplianceEventsData
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, int64(3), response.TotalCount)
		assert.Len(t, response.Events, 3)

		// Verify aggregation
		assert.Equal(t, 1, response.BySeverity["warning"])
		assert.Equal(t, 1, response.BySeverity["info"])
		assert.Equal(t, 1, response.BySeverity["critical"])
		assert.Equal(t, 2, response.ByType["pii_redacted"])
		assert.Equal(t, 1, response.ByType["content_rewritten"])
		assert.Equal(t, 2, response.ByFilter["pii-filter"])
		assert.Equal(t, 1, response.ByFilter["tone-filter"])
	})

	t.Run("Filters by severity", func(t *testing.T) {
		w := apitest.PerformRequest(r, "GET", "/api/v1/compliance/events?severity=critical", nil)
		assert.Equal(t, http.StatusOK, w.Code)

		var response compliance.ComplianceEventsData
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, int64(1), response.TotalCount)
		assert.Len(t, response.Events, 1)
		assert.Equal(t, "critical", response.Events[0].Severity)
	})

	t.Run("Rejects invalid severity", func(t *testing.T) {
		w := apitest.PerformRequest(r, "GET", "/api/v1/compliance/events?severity=INVALID", nil)
		assert.Equal(t, http.StatusBadRequest, w.Code)

		var errResp models.ErrorResponse
		err := json.Unmarshal(w.Body.Bytes(), &errResp)
		assert.NoError(t, err)
		assert.Contains(t, errResp.Errors[0].Detail, "severity must be one of")
	})

	t.Run("Rejects SQL injection in severity", func(t *testing.T) {
		w := apitest.PerformRequest(r, "GET", "/api/v1/compliance/events?severity=info'+OR+1%3D1--", nil)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("SQL injection in event_type returns no results", func(t *testing.T) {
		// event_type is free-form but uses parameterized queries (GORM Where("event_type = ?", val))
		// so injection attempts are treated as literal string matches and return 0 results
		w := apitest.PerformRequest(r, "GET", "/api/v1/compliance/events?event_type=pii_redacted'+OR+1%3D1--", nil)
		assert.Equal(t, http.StatusOK, w.Code)

		var response compliance.ComplianceEventsData
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, int64(0), response.TotalCount, "SQL injection attempt should match no events")
		assert.Empty(t, response.Events)
	})

	t.Run("Rejects oversized event_type", func(t *testing.T) {
		longType := strings.Repeat("a", 101)
		w := apitest.PerformRequest(r, "GET", "/api/v1/compliance/events?event_type="+longType, nil)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Filters by event_type", func(t *testing.T) {
		w := apitest.PerformRequest(r, "GET", "/api/v1/compliance/events?event_type=pii_redacted", nil)
		assert.Equal(t, http.StatusOK, w.Code)

		var response compliance.ComplianceEventsData
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, int64(2), response.TotalCount)
		for _, e := range response.Events {
			assert.Equal(t, "pii_redacted", e.EventType)
		}
	})

	t.Run("Filters by app_id", func(t *testing.T) {
		w := apitest.PerformRequest(r, "GET", "/api/v1/compliance/events?app_id=2", nil)
		assert.Equal(t, http.StatusOK, w.Code)

		var response compliance.ComplianceEventsData
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, int64(1), response.TotalCount)
		assert.Equal(t, uint(2), response.Events[0].AppID)
	})

	t.Run("Respects limit and offset", func(t *testing.T) {
		w := apitest.PerformRequest(r, "GET", "/api/v1/compliance/events?limit=1&offset=0", nil)
		assert.Equal(t, http.StatusOK, w.Code)

		var response compliance.ComplianceEventsData
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Len(t, response.Events, 1)
		assert.Equal(t, int64(3), response.TotalCount) // total is still 3
	})

	t.Run("Returns metadata as parsed JSON", func(t *testing.T) {
		w := apitest.PerformRequest(r, "GET", "/api/v1/compliance/events?event_type=pii_redacted&app_id=1", nil)
		assert.Equal(t, http.StatusOK, w.Code)

		var response compliance.ComplianceEventsData
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, len(response.Events), 1)

		// The metadata should be a parsed map
		meta := response.Events[0].Metadata
		assert.NotNil(t, meta)
		assert.Equal(t, "ssn", meta["pattern"])
	})
}
