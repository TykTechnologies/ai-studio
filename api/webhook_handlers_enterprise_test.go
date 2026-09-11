//go:build enterprise
// +build enterprise

package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	apitest "github.com/TykTechnologies/midsommar/v2/api/testing"
	"github.com/TykTechnologies/midsommar/v2/config"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/eventbridge"
	"github.com/TykTechnologies/midsommar/v2/services/audit"
	"github.com/TykTechnologies/midsommar/v2/services/webhooks"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	// Register the enterprise implementations under test.
	_ "github.com/TykTechnologies/midsommar/v2/enterprise/features/audit"
	_ "github.com/TykTechnologies/midsommar/v2/enterprise/features/webhooks"
)

type webhookEntHarness struct {
	api    *API
	router *gin.Engine
	admin  *models.User
	second *models.User
}

// sharedMemoryDB opens a named shared-cache in-memory database. The webhook
// workers and the audit writer run on their own goroutines with pooled
// connections; with a plain ":memory:" DSN each connection would see an
// empty database.
func sharedMemoryDB(t *testing.T) *gorm.DB {
	t.Helper()
	config.Get("").FilterSignupDomains = nil
	name := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	db, err := gorm.Open(sqlite.Open("file:"+name+"?mode=memory&cache=shared&_busy_timeout=5000"), &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, models.InitModels(db))
	return db
}

// setupWebhooksEnterpriseAPI wires a real event bus, the enterprise
// webhooks service and the enterprise audit trail behind the HTTP API.
func setupWebhooksEnterpriseAPI(t *testing.T) *webhookEntHarness {
	t.Helper()
	t.Setenv("AUDIT_ENABLED", "true")
	t.Setenv("TYK_AI_SECRET_KEY", "test-encryption-key") // webhooks refuse to start without it
	db := sharedMemoryDB(t)
	service := apitest.SetupTestService(db)
	service.SetEventBus(eventbridge.NewBus())
	cfg := config.WebhooksConfig{
		Enabled: true, WorkerEnabled: true, WorkerCount: 2, MaxAttempts: 3,
		BaseBackoff: 10 * time.Millisecond, MaxBackoff: 50 * time.Millisecond,
		RequestTimeout: 2 * time.Second, AllowInternalTargets: true,
		RetentionDays: 14, DeadLetterRetentionDays: 30, MaxResponseSnippetBytes: 512,
		SecretRotationGrace: time.Hour, ShutdownDrainTimeout: 3 * time.Second,
	}
	service.InitWebhooks(cfg, "test")
	t.Cleanup(service.Webhooks.Stop)

	authCfg := apitest.SetupTestAuthConfig(db, service)
	authService := apitest.SetupTestAuthService(db, service)
	api := NewAPI(service, true, authService, authCfg, nil, emptyFile, nil)
	t.Cleanup(func() {
		if api.auditService != nil {
			api.auditService.Stop()
		}
	})
	require.True(t, webhooks.IsEnterpriseAvailable())
	require.NotNil(t, service.Audit(), "audit trail propagated to the service")

	mkAdmin := func(email string) *models.User {
		u := models.NewUser()
		u.Email = email
		u.Name = email
		u.Password = "hash"
		u.IsAdmin = true
		u.EmailVerified = true
		require.NoError(t, u.Create(db))
		return u
	}
	return &webhookEntHarness{api: api, router: api.router, admin: mkAdmin("admin@tyk.io"), second: mkAdmin("second@tyk.io")}
}

func decodeWebhookJSON(t *testing.T, w *httptest.ResponseRecorder, into interface{}) {
	t.Helper()
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), into), w.Body.String())
}

func TestWebhooksEnterprise_EndToEnd(t *testing.T) {
	h := setupWebhooksEnterpriseAPI(t)

	var mu sync.Mutex
	var received []http.Header
	var bodies [][]byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		received = append(received, r.Header.Clone())
		bodies = append(bodies, b)
		mu.Unlock()
		w.WriteHeader(200)
	}))
	defer srv.Close()

	// Status and feature flag.
	w := apitest.PerformAuthRequest(h.router, "GET", "/api/v1/webhooks/status", nil, h.admin.APIKey)
	require.Equal(t, http.StatusOK, w.Code)
	var st webhooks.Status
	decodeWebhookJSON(t, w, &st)
	assert.True(t, st.Available && st.Enabled && st.BusConnected)

	w = apitest.PerformAuthRequest(h.router, "GET", "/common/system", nil, h.admin.APIKey)
	var sys struct {
		Features map[string]interface{} `json:"features"`
	}
	decodeWebhookJSON(t, w, &sys)
	assert.Equal(t, true, sys.Features["feature_webhooks"])

	// Create (pending) with a custom header and preset.
	w = apitest.PerformAuthRequest(h.router, "POST", "/api/v1/webhooks/targets", map[string]interface{}{
		"name": "Ops", "url": srv.URL + "/hook", "topic_filters": []string{"system.llm.*"},
		"headers": map[string]string{"Authorization": "Bearer xyz"}, "template_preset": "standard",
	}, h.admin.APIKey)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var created createWebhookTargetResponse
	decodeWebhookJSON(t, w, &created)
	assert.Equal(t, "pending", created.Target.Status)
	assert.Len(t, created.SigningSecret, 64)
	assert.Equal(t, []string{"Authorization"}, created.Target.HeaderNames)
	assert.NotContains(t, w.Body.String(), "Bearer xyz", "header values never returned")
	id := created.Target.ID

	// Approval by the second admin.
	w = apitest.PerformAuthRequest(h.router, "POST", "/api/v1/webhooks/targets/"+id+"/approve", map[string]string{"note": "ok"}, h.second.APIKey)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var approved models.WebhookTargetResponse
	decodeWebhookJSON(t, w, &approved)
	assert.Equal(t, "approved", approved.Status)
	assert.Equal(t, h.second.Email, approved.ApprovedByEmail)

	// Pending targets list is empty now.
	w = apitest.PerformAuthRequest(h.router, "GET", "/api/v1/webhooks/targets?status=pending", nil, h.admin.APIKey)
	var list webhooks.TargetList
	decodeWebhookJSON(t, w, &list)
	assert.Empty(t, list.Targets)
	assert.Equal(t, int64(0), list.PendingCount)

	// Emit a system event through the real emitter.
	h.api.service.SystemEvents.EmitLLMCreated(map[string]interface{}{"id": 5, "name": "GPT", "api_key": "sk-live-abc"}, 5, uint(h.admin.ID))

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(received)
		mu.Unlock()
		if n >= 1 {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	mu.Lock()
	require.Len(t, received, 1, "receiver got the delivery")
	hdr, body := received[0], bodies[0]
	mu.Unlock()
	assert.Equal(t, "system.llm.created", hdr.Get("X-Webhook-Topic"))
	assert.Equal(t, "Bearer xyz", hdr.Get("Authorization"))
	assert.NotEmpty(t, hdr.Get("X-Webhook-Signature"))
	assert.Contains(t, string(body), "[REDACTED]")
	assert.NotContains(t, string(body), "sk-live")

	// Delivery log.
	var page webhooks.DeliveryPage
	for time.Now().Before(deadline) {
		w = apitest.PerformAuthRequest(h.router, "GET", "/api/v1/webhooks/deliveries?target_id="+id, nil, h.admin.APIKey)
		require.Equal(t, http.StatusOK, w.Code)
		decodeWebhookJSON(t, w, &page)
		if page.Total == 1 && page.Deliveries[0].Status == "succeeded" {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	require.Equal(t, int64(1), page.Total)
	assert.Equal(t, "succeeded", page.Deliveries[0].Status)
	deliveryID := page.Deliveries[0].ID

	w = apitest.PerformAuthRequest(h.router, "GET", "/api/v1/webhooks/deliveries/"+deliveryID, nil, h.admin.APIKey)
	require.Equal(t, http.StatusOK, w.Code)
	var detail webhooks.DeliveryDetail
	decodeWebhookJSON(t, w, &detail)
	assert.Len(t, detail.Attempts, 1)
	assert.Equal(t, 200, detail.Attempts[0].StatusCode)
	assert.NotEmpty(t, detail.Delivery.RenderedPayload)
	assert.NotContains(t, string(detail.Attempts[0].RequestHeaders), "Bearer xyz")

	w = apitest.PerformAuthRequest(h.router, "GET", "/api/v1/webhooks/stats?window=1h", nil, h.admin.APIKey)
	require.Equal(t, http.StatusOK, w.Code)
	w = apitest.PerformAuthRequest(h.router, "GET", "/api/v1/webhooks/deliveries/export?format=csv", nil, h.admin.APIKey)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Disposition"), "webhook_deliveries_")
	assert.Contains(t, w.Body.String(), deliveryID)

	// Replay.
	w = apitest.PerformAuthRequest(h.router, "POST", "/api/v1/webhooks/deliveries/"+deliveryID+"/replay", nil, h.admin.APIKey)
	require.Equal(t, http.StatusAccepted, w.Code, w.Body.String())

	// Changing the URL re-pends; the audit trail recorded every action.
	w = apitest.PerformAuthRequest(h.router, "PATCH", "/api/v1/webhooks/targets/"+id, map[string]interface{}{
		"lock_version": approved.LockVersion, "url": srv.URL + "/moved",
	}, h.admin.APIKey)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var upd updateWebhookTargetResponse
	decodeWebhookJSON(t, w, &upd)
	assert.True(t, upd.Repended)
	assert.Equal(t, "pending", upd.Target.Status)

	// Stale lock version → 409.
	w = apitest.PerformAuthRequest(h.router, "PATCH", "/api/v1/webhooks/targets/"+id, map[string]interface{}{
		"lock_version": approved.LockVersion, "name": "again",
	}, h.admin.APIKey)
	assert.Equal(t, http.StatusConflict, w.Code)

	// Bad URL → 422.
	w = apitest.PerformAuthRequest(h.router, "POST", "/api/v1/webhooks/targets", map[string]interface{}{
		"name": "bad", "url": "ftp://x", "topic_filters": []string{"*"},
	}, h.admin.APIKey)
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)

	// Audit trail: HTTP actions recorded by the middleware with named actions
	// against the target resource.
	var recs *audit.Page
	for time.Now().Before(deadline) {
		var err error
		recs, err = h.api.auditService.List(nil, audit.Query{ResourceType: "webhook_target", ResourceID: id, PageSize: 50})
		require.NoError(t, err)
		if recs.Total >= 3 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	require.NotNil(t, recs)
	actions := map[string]bool{}
	for _, r := range recs.Records {
		actions[r.Action] = true
	}
	assert.True(t, actions["Approve Webhook Target"], "actions: %v", actions)
	assert.True(t, actions["Update Webhook Target"], "actions: %v", actions)
	for _, r := range recs.Records {
		if r.Action == "Approve Webhook Target" {
			assert.Equal(t, h.second.Email, r.UserEmail)
			assert.Contains(t, string(r.Diff), "approved", "diff shows the status change: %s", r.Diff)
		}
	}
	w = apitest.PerformAuthRequest(h.router, "GET", fmt.Sprintf("/api/v1/audit/resources/webhook_target/%s", id), nil, h.admin.APIKey)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "Approve Webhook Target")
}

func TestWebhooksEnterprise_DeadLetterIsAudited(t *testing.T) {
	h := setupWebhooksEnterpriseAPI(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) }))
	defer srv.Close()

	w := apitest.PerformAuthRequest(h.router, "POST", "/api/v1/webhooks/targets", map[string]interface{}{
		"name": "Flaky", "url": srv.URL, "topic_filters": []string{"*"},
	}, h.admin.APIKey)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var created createWebhookTargetResponse
	decodeWebhookJSON(t, w, &created)
	w = apitest.PerformAuthRequest(h.router, "POST", "/api/v1/webhooks/targets/"+created.Target.ID+"/approve", nil, h.second.APIKey)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	w = apitest.PerformAuthRequest(h.router, "POST", "/api/v1/webhooks/targets/"+created.Target.ID+"/test", map[string]string{"topic": "system.app.created"}, h.admin.APIKey)
	require.Equal(t, http.StatusAccepted, w.Code, w.Body.String())

	deadline := time.Now().Add(15 * time.Second)
	var recs *audit.Page
	for time.Now().Before(deadline) {
		var err error
		recs, err = h.api.auditService.List(nil, audit.Query{Method: "SYSTEM", ResourceType: "webhook_delivery", PageSize: 10})
		require.NoError(t, err)
		if recs.Total >= 1 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	require.NotNil(t, recs)
	require.GreaterOrEqual(t, recs.Total, int64(1), "dead letter recorded in the audit trail")
	rec := recs.Records[0]
	assert.Equal(t, "Webhook Delivery Dead-Lettered", rec.Action)
	assert.Equal(t, "SYSTEM", rec.Method)
	assert.Equal(t, "system", rec.UserEmail)
	assert.Equal(t, 500, rec.Status)
	assert.Contains(t, string(rec.Diff), "system.app.created")

	// Admin notifications for dead letters are covered by the enterprise unit
	// tests (fake notifier); here the test admins have notifications off.

	// Dead letters list + bulk replay endpoint.
	w = apitest.PerformAuthRequest(h.router, "GET", "/api/v1/webhooks/deliveries?status=dead_lettered", nil, h.admin.APIKey)
	require.Equal(t, http.StatusOK, w.Code)
	var page webhooks.DeliveryPage
	decodeWebhookJSON(t, w, &page)
	require.Equal(t, int64(1), page.Total)
	w = apitest.PerformAuthRequest(h.router, "POST", "/api/v1/webhooks/deliveries/replay", map[string]interface{}{"target_id": created.Target.ID}, h.admin.APIKey)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var res webhooks.ReplayResult
	decodeWebhookJSON(t, w, &res)
	assert.Equal(t, 1, res.Replayed)
}
