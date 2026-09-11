//go:build !enterprise
// +build !enterprise

package api

import (
	"encoding/json"
	"net/http"
	"testing"

	apitest "github.com/TykTechnologies/midsommar/v2/api/testing"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services/webhooks"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupWebhooksCommunityAPI returns the router and an admin API key. Mutating
// webhook routes need an actor on the context, and in TestMode the auth
// middleware only sets one when a token is presented.
func setupWebhooksCommunityAPI(t *testing.T) (*gin.Engine, string) {
	t.Helper()
	db := apitest.SetupTestDB(t)
	service := apitest.SetupTestService(db)
	cfg := apitest.SetupTestAuthConfig(db, service)
	authService := apitest.SetupTestAuthService(db, service)
	api := NewAPI(service, true, authService, cfg, nil, emptyFile, nil)

	admin := models.NewUser()
	admin.Email = "admin@tyk.io"
	admin.Name = "Admin"
	admin.Password = "hash"
	admin.IsAdmin = true
	admin.EmailVerified = true
	require.NoError(t, admin.Create(db))
	return api.router, admin.APIKey
}

func TestWebhooksCommunity_NotAvailable(t *testing.T) {
	r, _ := setupWebhooksCommunityAPI(t)
	assert.False(t, webhooks.IsEnterpriseAvailable())

	w := apitest.PerformRequest(r, "GET", "/api/v1/webhooks/status", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var status webhooks.Status
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &status))
	assert.False(t, status.Available)
	assert.False(t, status.Enabled)

	w = apitest.PerformRequest(r, "GET", "/api/v1/webhooks/templates/presets", nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "[]", w.Body.String())

	w = apitest.PerformRequest(r, "GET", "/common/system", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var sys struct {
		Features map[string]interface{} `json:"features"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &sys))
	assert.Equal(t, false, sys.Features["feature_webhooks"])
}

func TestWebhooksCommunity_EverythingElseReturns403(t *testing.T) {
	r, key := setupWebhooksCommunityAPI(t)
	target := map[string]interface{}{
		"name": "t", "url": "https://hooks.example.com/x", "topic_filters": []string{"system.llm.*"},
	}
	for _, rq := range []struct {
		method, path string
		body         interface{}
	}{
		{"GET", "/api/v1/webhooks/topics", nil},
		{"POST", "/api/v1/webhooks/templates/preview", map[string]string{"template_preset": "standard"}},
		{"GET", "/api/v1/webhooks/targets", nil},
		{"POST", "/api/v1/webhooks/targets", target},
		{"GET", "/api/v1/webhooks/targets/abc", nil},
		{"PATCH", "/api/v1/webhooks/targets/abc", map[string]interface{}{"lock_version": 0, "name": "x"}},
		{"DELETE", "/api/v1/webhooks/targets/abc", nil},
		{"POST", "/api/v1/webhooks/targets/abc/approve", nil},
		{"POST", "/api/v1/webhooks/targets/abc/reject", map[string]string{"reason": "no"}},
		{"POST", "/api/v1/webhooks/targets/abc/revoke", map[string]string{"reason": "no"}},
		{"POST", "/api/v1/webhooks/targets/abc/pause", nil},
		{"POST", "/api/v1/webhooks/targets/abc/resume", nil},
		{"POST", "/api/v1/webhooks/targets/abc/rotate-secret", nil},
		{"POST", "/api/v1/webhooks/targets/abc/test", nil},
		{"GET", "/api/v1/webhooks/deliveries", nil},
		{"GET", "/api/v1/webhooks/deliveries/export", nil},
		{"POST", "/api/v1/webhooks/deliveries/replay", map[string]interface{}{"max": 10}},
		{"GET", "/api/v1/webhooks/deliveries/abc", nil},
		{"POST", "/api/v1/webhooks/deliveries/abc/replay", nil},
		{"POST", "/api/v1/webhooks/deliveries/abc/cancel", nil},
		{"GET", "/api/v1/webhooks/stats", nil},
	} {
		w := apitest.PerformAuthRequest(r, rq.method, rq.path, rq.body, key)
		assert.Equal(t, http.StatusForbidden, w.Code, "%s %s: %s", rq.method, rq.path, w.Body.String())
		assert.Contains(t, w.Body.String(), "Enterprise", "%s %s", rq.method, rq.path)
	}
}

// Query validation happens before the service is consulted, in every edition.
func TestWebhooksCommunity_QueryValidation(t *testing.T) {
	r, key := setupWebhooksCommunityAPI(t)
	for _, p := range []string{
		"/api/v1/webhooks/deliveries?status=bogus",
		"/api/v1/webhooks/deliveries?page=0",
		"/api/v1/webhooks/deliveries?page_size=9999",
		"/api/v1/webhooks/deliveries?start_date=2026-02-01&end_date=2026-01-01",
		"/api/v1/webhooks/deliveries?start_date=yesterday",
		"/api/v1/webhooks/deliveries/export?format=xml",
		"/api/v1/webhooks/stats?window=1y",
	} {
		w := apitest.PerformRequest(r, "GET", p, nil)
		assert.Equal(t, http.StatusBadRequest, w.Code, p)
	}
	w := apitest.PerformAuthRequest(r, "POST", "/api/v1/webhooks/deliveries/replay", map[string]interface{}{"max": 100000}, key)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}
