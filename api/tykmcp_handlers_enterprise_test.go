//go:build enterprise
// +build enterprise

package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	apitest "github.com/TykTechnologies/midsommar/v2/api/testing"
	"github.com/TykTechnologies/midsommar/v2/config"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/eventbridge"
	"github.com/TykTechnologies/midsommar/v2/services/tykmcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	// Register the enterprise implementations under test.
	_ "github.com/TykTechnologies/midsommar/v2/enterprise/features/audit"
	_ "github.com/TykTechnologies/midsommar/v2/enterprise/features/tykmcp"
)

// fakeTykDashboard answers the probe endpoints of a capable Dashboard.
func fakeTykDashboard(t *testing.T, token string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Header.Get("Authorization") != token {
			w.WriteHeader(401)
			_, _ = w.Write([]byte(`{"Status":"Error","Message":"Not authorised"}`))
			return
		}
		switch r.Method + " " + r.URL.Path {
		case "GET /api/schemas/apidefs/mcp":
			_, _ = w.Write([]byte(`{"definitions":{"x-tyk-mcp-server":{}}}`))
		case "GET /api/mcps":
			_, _ = w.Write([]byte(`{"mcps":[],"pages":1}`))
		case "GET /api/portal/policies":
			_, _ = w.Write([]byte(`{"Data":[{"_id":"p1","org_id":"org-1"}],"Pages":1}`))
		case "GET /api/apis":
			_, _ = w.Write([]byte(`{"apis":[],"pages":1}`))
		case "POST /api/keys/preview":
			_, _ = w.Write([]byte(`{"key_id":"","data":{"org_id":"org-1"}}`))
		case "POST /api/mcps":
			_, _ = w.Write([]byte(`{"openapi":"3.0.3"}`))
		default:
			w.WriteHeader(404)
			_, _ = w.Write([]byte(`{"Status":"Error","Message":"Not found"}`))
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

type tykMCPEntHarness struct {
	api   *API
	admin *models.User
}

func setupTykMCPEnterpriseAPI(t *testing.T) *tykMCPEntHarness {
	t.Helper()
	t.Setenv("AUDIT_ENABLED", "true")
	t.Setenv("TYK_AI_SECRET_KEY", "test-encryption-key")
	db := sharedMemoryDB(t)
	service := apitest.SetupTestService(db)
	service.SetEventBus(eventbridge.NewBus())
	service.InitTykMCP(config.TykMCPConfig{Enabled: true, SyncMinInterval: 10 * time.Second, RequestTimeout: 2 * time.Second, RateLimitPerSecond: 100}, "test")
	t.Cleanup(service.TykMCP.Stop)

	authCfg := apitest.SetupTestAuthConfig(db, service)
	authService := apitest.SetupTestAuthService(db, service)
	api := NewAPI(service, true, authService, authCfg, nil, emptyFile, nil)
	t.Cleanup(func() {
		if api.auditService != nil {
			api.auditService.Stop()
		}
	})
	require.True(t, tykmcp.IsEnterpriseAvailable())

	admin := models.NewUser()
	admin.Email = "admin@tyk.io"
	admin.Name = "Admin"
	admin.Password = "hash"
	admin.IsAdmin = true
	admin.EmailVerified = true
	require.NoError(t, admin.Create(db))
	return &tykMCPEntHarness{api: api, admin: admin}
}

func TestTykMCPEnterprise_ConnectionLifecycle(t *testing.T) {
	h := setupTykMCPEnterpriseAPI(t)
	dash := fakeTykDashboard(t, "user-key-abcd")
	r := h.api.router
	key := h.admin.APIKey

	w := apitest.PerformAuthRequest(r, "GET", "/api/v1/tyk-mcp/status", nil, key)
	require.Equal(t, http.StatusOK, w.Code)
	var st tykmcp.Status
	decodeWebhookJSON(t, w, &st)
	assert.True(t, st.Available && st.Enabled)

	w = apitest.PerformAuthRequest(r, "GET", "/common/system", nil, key)
	var sys struct {
		Features map[string]interface{} `json:"features"`
	}
	decodeWebhookJSON(t, w, &sys)
	assert.Equal(t, true, sys.Features["feature_tyk_mcp"])

	// Test unsaved settings first.
	input := map[string]interface{}{
		"name": "Prod Dashboard", "dashboard_url": dash.URL, "dashboard_access_token": "user-key-abcd",
		"declared_mode": "full", "allow_internal_host": true, "gateway_base_url": "https://gw.example.com",
	}
	w = apitest.PerformAuthRequest(r, "POST", "/api/v1/tyk-connections/probe", input, key)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var probe tykmcp.ProbeResult
	decodeWebhookJSON(t, w, &probe)
	assert.True(t, probe.Reachable)
	assert.Equal(t, "full", probe.EffectiveMode)
	assert.Equal(t, "org-1", probe.OrgID)

	// Create.
	w = apitest.PerformAuthRequest(r, "POST", "/api/v1/tyk-connections", input, key)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var view models.TykConnectionResponse
	decodeWebhookJSON(t, w, &view)
	assert.Equal(t, "pending", view.Status)
	assert.True(t, view.HasToken)
	assert.Equal(t, "abcd", view.TokenHint)
	assert.NotContains(t, w.Body.String(), "user-key-abcd", "the token never appears in a response")

	// Wrong-scheme URL is refused with 422.
	bad := map[string]interface{}{"name": "x", "dashboard_url": "ftp://dash.example.com", "dashboard_access_token": "t"}
	w = apitest.PerformAuthRequest(r, "POST", "/api/v1/tyk-connections", bad, key)
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)

	// Activate.
	w = apitest.PerformAuthRequest(r, "POST", "/api/v1/tyk-connections/"+tykIDStr(view.ID)+"/activate", nil, key)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	decodeWebhookJSON(t, w, &view)
	assert.Equal(t, "active", view.Status)
	assert.Equal(t, "full", view.EffectiveMode)
	assert.Equal(t, "org-1", view.OrgID)

	// List and get.
	w = apitest.PerformAuthRequest(r, "GET", "/api/v1/tyk-connections", nil, key)
	require.Equal(t, http.StatusOK, w.Code)
	var list []models.TykConnectionResponse
	decodeWebhookJSON(t, w, &list)
	require.Len(t, list, 1)
	assert.NotContains(t, w.Body.String(), "user-key-abcd")

	// Patch with a stale lock version conflicts; a fresh one succeeds.
	patch := map[string]interface{}{"description": "primary", "lock_version": view.LockVersion - 1}
	w = apitest.PerformAuthRequest(r, "PATCH", "/api/v1/tyk-connections/"+tykIDStr(view.ID), patch, key)
	assert.Equal(t, http.StatusConflict, w.Code)
	patch["lock_version"] = view.LockVersion
	w = apitest.PerformAuthRequest(r, "PATCH", "/api/v1/tyk-connections/"+tykIDStr(view.ID), patch, key)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	decodeWebhookJSON(t, w, &view)
	assert.Equal(t, "primary", view.Description)

	// Sync request is accepted on an active connection.
	w = apitest.PerformAuthRequest(r, "POST", "/api/v1/tyk-connections/"+tykIDStr(view.ID)+"/sync", nil, key)
	assert.Equal(t, http.StatusAccepted, w.Code)

	// Re-probe.
	w = apitest.PerformAuthRequest(r, "POST", "/api/v1/tyk-connections/"+tykIDStr(view.ID)+"/probe", nil, key)
	require.Equal(t, http.StatusOK, w.Code)

	// Disable, then delete.
	w = apitest.PerformAuthRequest(r, "POST", "/api/v1/tyk-connections/"+tykIDStr(view.ID)+"/disable", map[string]string{"reason": "test"}, key)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	decodeWebhookJSON(t, w, &view)
	assert.Equal(t, "disabled", view.Status)

	w = apitest.PerformAuthRequest(r, "DELETE", "/api/v1/tyk-connections/"+tykIDStr(view.ID), nil, key)
	assert.Equal(t, http.StatusNoContent, w.Code)
	w = apitest.PerformAuthRequest(r, "GET", "/api/v1/tyk-connections/"+tykIDStr(view.ID), nil, key)
	assert.Equal(t, http.StatusNotFound, w.Code)

	// The audit trail (flushed in batches) recorded the HTTP actions with
	// the token redacted.
	var body string
	require.Eventually(t, func() bool {
		w = apitest.PerformAuthRequest(r, "GET", "/api/v1/audit/records?resource_type=tyk_connection", nil, key)
		body = w.Body.String()
		return w.Code == http.StatusOK && strings.Contains(body, "tyk_connection") && strings.Contains(body, "Activate")
	}, 5*time.Second, 100*time.Millisecond, "audit records: %s", body)
	assert.NotContains(t, body, "user-key-abcd")
	assert.NotContains(t, body, "$ENC/")
}

func tykIDStr(v uint) string {
	b, _ := json.Marshal(v)
	return string(b)
}
