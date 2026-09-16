//go:build enterprise
// +build enterprise

package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	apitest "github.com/TykTechnologies/midsommar/v2/api/testing"
	"github.com/TykTechnologies/midsommar/v2/config"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/eventbridge"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTykMCPEnterprise_HandoffReachesWebhook proves the registration handoff
// travels the same road as every other event: a catalogue-mode approval
// publishes system.mcp_server.registration_handoff on the bus, the webhooks
// ingestor forwards it to an approved target, and the delivery carries the
// package summary without the submitter's upstream credential.
func TestTykMCPEnterprise_HandoffReachesWebhook(t *testing.T) {
	t.Setenv("AUDIT_ENABLED", "true")
	t.Setenv("TYK_AI_SECRET_KEY", "test-encryption-key")
	db := sharedMemoryDB(t)
	service := apitest.SetupTestService(db)
	service.SetEventBus(eventbridge.NewBus())
	service.InitTykMCP(config.TykMCPConfig{Enabled: true, SyncMinInterval: 10 * time.Second, RequestTimeout: 2 * time.Second, RateLimitPerSecond: 100}, "test")
	t.Cleanup(service.TykMCP.Stop)
	service.InitWebhooks(config.WebhooksConfig{
		Enabled: true, WorkerEnabled: true, WorkerCount: 2, MaxAttempts: 3,
		BaseBackoff: 10 * time.Millisecond, MaxBackoff: 50 * time.Millisecond,
		RequestTimeout: 2 * time.Second, AllowInternalTargets: true,
		RetentionDays: 14, DeadLetterRetentionDays: 30, MaxResponseSnippetBytes: 512,
		SecretRotationGrace: time.Hour, ShutdownDrainTimeout: 3 * time.Second,
	}, "test")
	t.Cleanup(service.Webhooks.Stop)
	authCfg := apitest.SetupTestAuthConfig(db, service)
	authService := apitest.SetupTestAuthService(db, service)
	api := NewAPI(service, true, authService, authCfg, nil, emptyFile, nil)
	t.Cleanup(func() {
		if api.auditService != nil {
			api.auditService.Stop()
		}
	})
	r := api.router
	mkUser := func(email string, admin bool) *models.User {
		u := models.NewUser()
		u.Email, u.Name, u.Password, u.IsAdmin, u.EmailVerified, u.ShowPortal = email, email, "hash", admin, true, true
		require.NoError(t, u.Create(db))
		return u
	}
	admin, second, member := mkUser("admin@tyk.io", true), mkUser("second@tyk.io", true), mkUser("member@tyk.io", false)

	// A receiver for the platform team, approved by a second administrator.
	var mu sync.Mutex
	var deliveries []struct {
		topic string
		body  string
	}
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		b, _ := io.ReadAll(req.Body)
		mu.Lock()
		deliveries = append(deliveries, struct {
			topic string
			body  string
		}{req.Header.Get("X-Webhook-Topic"), string(b)})
		mu.Unlock()
		w.WriteHeader(200)
	}))
	defer receiver.Close()
	w := apitest.PerformAuthRequest(r, "POST", "/api/v1/webhooks/targets", map[string]interface{}{
		"name": "Platform team", "url": receiver.URL + "/hook", "topic_filters": []string{"system.mcp_server.*"}, "template_preset": "standard",
	}, admin.APIKey)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var created createWebhookTargetResponse
	decodeWebhookJSON(t, w, &created)
	w = apitest.PerformAuthRequest(r, "POST", "/api/v1/webhooks/targets/"+created.Target.ID+"/approve", map[string]string{"note": "ok"}, second.APIKey)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// A catalogue-mode connection: AI Studio may not create the proxy.
	dash := newWritableFakeDashboard(t, "user-key-abcd")
	w = apitest.PerformAuthRequest(r, "POST", "/api/v1/tyk-connections", map[string]interface{}{
		"name": "Platform", "dashboard_url": dash.srv.URL, "dashboard_access_token": "user-key-abcd",
		"declared_mode": "catalogue", "allow_internal_host": true, "gateway_base_url": "https://gw.example.com",
	}, admin.APIKey)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var conn models.TykConnectionResponse
	decodeWebhookJSON(t, w, &conn)
	w = apitest.PerformAuthRequest(r, "POST", "/api/v1/tyk-connections/"+tykIDStr(conn.ID)+"/activate", nil, admin.APIKey)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// The submission, then its approval.
	w = apitest.PerformAuthRequest(r, "POST", "/common/submissions", map[string]interface{}{"data": map[string]interface{}{"attributes": map[string]interface{}{
		"resource_type": "mcp_server", "status": "submitted",
		"resource_payload": map[string]interface{}{
			"name": "Tickets MCP", "description": "Tickets for agents", "kind": "remote", "connection_id": conn.ID,
			"upstream_url": "https://tickets.example.com/mcp", "upstream_auth_header_name": "X-Token", "upstream_auth_token": "UPSTREAM-SECRET",
			"consumer_auth": "auth_token", "suggested_listen_path": "/tickets-mcp/",
		},
		"suggested_privacy": 20, "privacy_justification": "internal tickets", "primary_contact": "member@tyk.io",
	}}}, member.APIKey)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var sub struct {
		Data struct {
			ID uint `json:"id"`
		} `json:"data"`
	}
	decodeWebhookJSON(t, w, &sub)
	w = apitest.PerformAuthRequest(r, "POST", "/api/v1/submissions/"+tykIDStr(sub.Data.ID)+"/approve", map[string]interface{}{"data": map[string]interface{}{"attributes": map[string]interface{}{"final_privacy_score": 20, "review_notes": "handoff"}}}, admin.APIKey)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// The handoff is delivered; the credential never leaves AI Studio.
	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		for _, d := range deliveries {
			if d.topic == "system.mcp_server.registration_handoff" {
				return true
			}
		}
		return false
	}, 10*time.Second, 25*time.Millisecond, "the registration_handoff delivery arrived")
	mu.Lock()
	defer mu.Unlock()
	for _, d := range deliveries {
		assert.NotContains(t, d.body, "UPSTREAM-SECRET", "topic %s", d.topic)
		if d.topic == "system.mcp_server.registration_handoff" {
			assert.Contains(t, d.body, "Tickets MCP")
			assert.Contains(t, d.body, "/tickets-mcp/")
			assert.Contains(t, d.body, "member@tyk.io")
			assert.Contains(t, d.body, `"***"`, "the definition in the package is masked")
		}
	}
}
