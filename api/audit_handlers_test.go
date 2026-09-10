//go:build enterprise
// +build enterprise

package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	apitest "github.com/TykTechnologies/midsommar/v2/api/testing"
	"github.com/TykTechnologies/midsommar/v2/config"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services/audit"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	// Import enterprise audit to trigger init() registration
	_ "github.com/TykTechnologies/midsommar/v2/enterprise/features/audit"
)

// setupAuditTestAPI builds the real API (so the middleware is registered in
// NewAPI exactly as in production) with an explicitly enabled audit service.
func setupAuditTestAPI(t *testing.T) (*API, *gin.Engine) {
	t.Helper()
	db := apitest.SetupTestDB(t)
	service := apitest.SetupTestService(db)
	cfg := apitest.SetupTestAuthConfig(db, service)
	authService := apitest.SetupTestAuthService(db, service)

	api := NewAPI(service, true, authService, cfg, nil, emptyFile, nil)
	api.SetAuditService(audit.NewService(db, config.AuditConfig{
		Enabled:       true,
		StoreType:     config.AuditStoreDB,
		RetentionDays: 30,
		MaxBodyBytes:  4096,
		QueueSize:     256,
	}))
	t.Cleanup(api.auditService.Stop)
	return api, api.router
}

func seedAuditRows(t *testing.T, api *API) {
	t.Helper()
	base := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	rows := models.AuditRecords{
		{RequestID: "r1", Timestamp: base, IP: "10.0.0.1", UserID: 1, UserEmail: "admin@tyk.io", Action: "Update LLM", Method: "PATCH", URL: "/api/v1/llms/1", Route: "/api/v1/llms/:id", Status: 200, ResourceType: "llm", ResourceID: "1", ResourceName: "gpt-4", Diff: `{"name":{"old":"a","new":"b"}}`, RequestDump: `{"headers":{"Content-Type":"application/json"}}`},
		{RequestID: "r2", Timestamp: base.Add(time.Hour), IP: "10.0.0.2", UserID: 0, UserEmail: "intruder@evil.io", Action: "Login Failed", Method: "POST", URL: "/auth/login", Route: "/auth/login", Status: 401, ResourceType: "session", Error: "Invalid email or password"},
		{RequestID: "r3", Timestamp: base.Add(2 * time.Hour), IP: "10.0.0.1", UserID: 1, UserEmail: "admin@tyk.io", Action: "Delete LLM", Method: "DELETE", URL: "/api/v1/llms/1", Route: "/api/v1/llms/:id", Status: 200, ResourceType: "llm", ResourceID: "1", ResourceName: "gpt-4"},
	}
	require.NoError(t, rows.CreateBatch(api.service.DB, 10))
}

func waitForAuditRows(t *testing.T, api *API, n int64) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var got int64
		api.service.DB.Model(&models.AuditRecord{}).Count(&got)
		if got >= n {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d audit rows", n)
}

func TestAuditEnterprise_Status(t *testing.T) {
	_, r := setupAuditTestAPI(t)
	w := apitest.PerformRequest(r, "GET", "/api/v1/audit/status", nil)
	require.Equal(t, http.StatusOK, w.Code)

	var status audit.Status
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &status))
	assert.True(t, status.Available)
	assert.True(t, status.Enabled)
	assert.Equal(t, "db", status.StoreType)
	assert.Equal(t, 30, status.RetentionDays)
}

func TestAuditEnterprise_ListFilterAndPaginate(t *testing.T) {
	api, r := setupAuditTestAPI(t)
	seedAuditRows(t, api)

	w := apitest.PerformRequest(r, "GET", "/api/v1/audit/records", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var page audit.Page
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &page))
	assert.Equal(t, int64(3), page.Total)
	assert.Equal(t, "r3", page.Records[0].RequestID, "newest first")
	assert.Equal(t, 1, page.Page)
	assert.Equal(t, 50, page.PageSize)

	w = apitest.PerformRequest(r, "GET", "/api/v1/audit/records?action=Update+LLM", nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &page))
	assert.Equal(t, int64(1), page.Total)

	w = apitest.PerformRequest(r, "GET", "/api/v1/audit/records?status_class=4xx", nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &page))
	assert.Equal(t, int64(1), page.Total)
	assert.Equal(t, "Login Failed", page.Records[0].Action)

	w = apitest.PerformRequest(r, "GET", "/api/v1/audit/records?user=ADMIN&sort=asc&page=1&page_size=1", nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &page))
	assert.Equal(t, int64(2), page.Total)
	require.Len(t, page.Records, 1)
	assert.Equal(t, "r1", page.Records[0].RequestID)

	w = apitest.PerformRequest(r, "GET", "/api/v1/audit/records?start_date=2026-09-01T11:30:00Z&end_date=2026-09-01", nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &page))
	assert.Equal(t, int64(1), page.Total, "RFC3339 start and date-only end (end of day)")

	// Diff is returned as a JSON object, not a string.
	w = apitest.PerformRequest(r, "GET", "/api/v1/audit/records?req_id=r1", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var raw map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &raw))
	rec := raw["records"].([]interface{})[0].(map[string]interface{})
	diff := rec["diff"].(map[string]interface{})
	assert.Equal(t, "b", diff["name"].(map[string]interface{})["new"])
	assert.Nil(t, rec["request_dump"], "dumps are omitted from list rows")
}

func TestAuditEnterprise_ListValidation(t *testing.T) {
	_, r := setupAuditTestAPI(t)
	for _, q := range []string{
		"start_date=yesterday",
		"end_date=2026-13-01",
		"start_date=2026-09-02&end_date=2026-09-01",
		"page=0",
		"page_size=-1",
		"status=99",
		"status_class=9xx",
		"user_id=abc",
		"search=" + strings.Repeat("x", 300),
	} {
		w := apitest.PerformRequest(r, "GET", "/api/v1/audit/records?"+q, nil)
		assert.Equal(t, http.StatusBadRequest, w.Code, q)
	}
}

func TestAuditEnterprise_GetRecord(t *testing.T) {
	api, r := setupAuditTestAPI(t)
	seedAuditRows(t, api)
	var first models.AuditRecord
	require.NoError(t, api.service.DB.Where("request_id = ?", "r1").First(&first).Error)

	w := apitest.PerformRequest(r, "GET", "/api/v1/audit/records/"+itoa(first.ID), nil)
	require.Equal(t, http.StatusOK, w.Code)
	var raw map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &raw))
	assert.Equal(t, "r1", raw["req_id"])
	assert.NotNil(t, raw["request_dump"], "single record includes dumps")

	w = apitest.PerformRequest(r, "GET", "/api/v1/audit/records/999999", nil)
	assert.Equal(t, http.StatusNotFound, w.Code)

	w = apitest.PerformRequest(r, "GET", "/api/v1/audit/records/abc", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAuditEnterprise_Summary(t *testing.T) {
	api, r := setupAuditTestAPI(t)
	seedAuditRows(t, api)

	w := apitest.PerformRequest(r, "GET", "/api/v1/audit/summary", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var sum audit.Summary
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &sum))
	assert.Equal(t, int64(3), sum.Total)
	assert.Equal(t, int64(1), sum.Failed)
	assert.Equal(t, int64(2), sum.DistinctUsers)
	assert.NotEmpty(t, sum.ByAction)
	assert.Len(t, sum.Timeline, 1)
}

func TestAuditEnterprise_Export(t *testing.T) {
	api, r := setupAuditTestAPI(t)
	seedAuditRows(t, api)

	w := apitest.PerformRequest(r, "GET", "/api/v1/audit/export", nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "text/csv")
	assert.Contains(t, w.Header().Get("Content-Disposition"), "audit_trail_")
	assert.Equal(t, 4, len(strings.Split(strings.TrimSpace(w.Body.String()), "\n")))

	w = apitest.PerformRequest(r, "GET", "/api/v1/audit/export?format=json&action=Delete+LLM", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var recs []models.AuditRecord
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &recs))
	assert.Len(t, recs, 1)

	w = apitest.PerformRequest(r, "GET", "/api/v1/audit/export?format=xml", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAuditEnterprise_ResourceHistory(t *testing.T) {
	api, r := setupAuditTestAPI(t)
	seedAuditRows(t, api)

	w := apitest.PerformRequest(r, "GET", "/api/v1/audit/resources/llm/1", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var page audit.Page
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &page))
	assert.Equal(t, int64(2), page.Total)
	assert.Equal(t, "Delete LLM", page.Records[0].Action)
	assert.Equal(t, "Update LLM", page.Records[1].Action)
}

// TestAuditEnterprise_MiddlewareRecordsRealRoutes drives the real router: a
// user created through the real handler shows up in the trail with a diff.
func TestAuditEnterprise_MiddlewareRecordsRealRoutes(t *testing.T) {
	api, r := setupAuditTestAPI(t)

	body := map[string]interface{}{
		"data": map[string]interface{}{
			"type": "users",
			"attributes": map[string]interface{}{
				"email":    "audited@tyk.io",
				"name":     "Audited",
				"password": "Sup3rSecret!",
				"is_admin": false,
			},
		},
	}
	w := apitest.PerformRequest(r, "POST", "/api/v1/users", body)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	assert.NotEmpty(t, w.Header().Get("X-Request-ID"))

	waitForAuditRows(t, api, 1)
	var rec models.AuditRecord
	require.NoError(t, api.service.DB.Order("id DESC").First(&rec).Error)
	assert.Equal(t, "Create User", rec.Action)
	assert.Equal(t, "POST", rec.Method)
	assert.Equal(t, "/api/v1/users", rec.Route)
	assert.Equal(t, http.StatusCreated, rec.Status)
	assert.Equal(t, "user", rec.ResourceType)
	assert.NotEmpty(t, rec.ResourceID)
	assert.Equal(t, "audited@tyk.io", rec.ResourceName)
	assert.NotContains(t, string(rec.Diff), "Sup3rSecret", "password never reaches the trail")
	assert.Contains(t, string(rec.Diff), `"email"`)

	// Reads (the audit endpoints themselves) are not recorded by default.
	w = apitest.PerformRequest(r, "GET", "/api/v1/audit/records", nil)
	require.Equal(t, http.StatusOK, w.Code)
	time.Sleep(300 * time.Millisecond)
	var n int64
	api.service.DB.Model(&models.AuditRecord{}).Count(&n)
	assert.Equal(t, int64(1), n)
}

func itoa(n uint) string {
	return strconv.FormatUint(uint64(n), 10)
}
