//go:build !enterprise
// +build !enterprise

package api

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	apitest "github.com/TykTechnologies/midsommar/v2/api/testing"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services/audit"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupAuditCommunityAPI(t *testing.T) (*API, *gin.Engine) {
	t.Helper()
	db := apitest.SetupTestDB(t)
	service := apitest.SetupTestService(db)
	cfg := apitest.SetupTestAuthConfig(db, service)
	authService := apitest.SetupTestAuthService(db, service)
	api := NewAPI(service, true, authService, cfg, nil, emptyFile, nil)
	return api, api.router
}

func TestAuditCommunity_NotAvailable(t *testing.T) {
	_, r := setupAuditCommunityAPI(t)
	assert.False(t, audit.IsEnterpriseAvailable())

	w := apitest.PerformRequest(r, "GET", "/api/v1/audit/status", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var status audit.Status
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &status))
	assert.False(t, status.Available)
	assert.False(t, status.Enabled)
}

func TestAuditCommunity_QueriesReturn403(t *testing.T) {
	_, r := setupAuditCommunityAPI(t)
	for _, path := range []string{
		"/api/v1/audit/records",
		"/api/v1/audit/records/1",
		"/api/v1/audit/summary",
		"/api/v1/audit/export",
		"/api/v1/audit/resources/llm/1",
	} {
		w := apitest.PerformRequest(r, "GET", path, nil)
		assert.Equal(t, http.StatusForbidden, w.Code, path)
		assert.Contains(t, w.Body.String(), "Enterprise", path)
	}
}

func TestAuditCommunity_MiddlewareRecordsNothing(t *testing.T) {
	api, r := setupAuditCommunityAPI(t)

	body := map[string]interface{}{
		"data": map[string]interface{}{
			"type": "users",
			"attributes": map[string]interface{}{
				"email": "ce@tyk.io", "name": "CE", "password": "Sup3rSecret!",
			},
		},
	}
	w := apitest.PerformRequest(r, "POST", "/api/v1/users", body)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	assert.Empty(t, w.Header().Get("X-Request-ID"))

	time.Sleep(50 * time.Millisecond)
	var n int64
	api.service.DB.Model(&models.AuditRecord{}).Count(&n)
	assert.Zero(t, n)
}
