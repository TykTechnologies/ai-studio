package api

import (
	"net/http"
	"testing"

	apitest "github.com/TykTechnologies/midsommar/v2/api/testing"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/authz"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Webhook targets are an exfiltration vector, so the routes must be reachable
// by administrators only. The other webhook tests run with TestMode on, which
// bypasses AuthMiddleware/AdminOnly; this one builds the router with TestMode
// OFF and proves the real guards in both editions.
func setupWebhookAuthzAPI(t *testing.T) (*gin.Engine, *models.User, *models.User) {
	t.Helper()
	db := apitest.SetupTestDB(t)
	service := apitest.SetupTestService(db)
	cfg := apitest.SetupTestAuthConfig(db, service)
	cfg.TestMode = false
	authService := apitest.SetupTestAuthService(db, service)
	authService.Config.TestMode = false

	api := NewAPI(service, true, authService, cfg, nil, emptyFile, nil)
	t.Cleanup(func() {
		if api.auditService != nil {
			api.auditService.Stop()
		}
	})

	admin := models.NewUser()
	admin.Email = "admin@tyk.io"
	admin.Name = "Admin"
	admin.Password = "hash"
	admin.IsAdmin = true
	admin.EmailVerified = true
	require.NoError(t, admin.Create(db))

	analyst := models.NewUser()
	analyst.Email = "analyst@tyk.io"
	analyst.Name = "Analyst"
	analyst.Password = "hash"
	analyst.IsAdmin = false
	analyst.EmailVerified = true
	require.NoError(t, analyst.Create(db))

	return api.router, admin, analyst
}

var webhookAuthzRequests = []struct {
	method, path string
}{
	{"GET", "/api/v1/webhooks/status"},
	{"GET", "/api/v1/webhooks/topics"},
	{"GET", "/api/v1/webhooks/targets"},
	{"POST", "/api/v1/webhooks/targets"},
	{"POST", "/api/v1/webhooks/targets/abc/approve"},
	{"GET", "/api/v1/webhooks/deliveries"},
	{"GET", "/api/v1/webhooks/deliveries/export"},
	{"POST", "/api/v1/webhooks/deliveries/replay"},
	{"GET", "/api/v1/webhooks/stats"},
}

func TestWebhooks_AnonymousIsUnauthorized(t *testing.T) {
	r, _, _ := setupWebhookAuthzAPI(t)
	for _, rq := range webhookAuthzRequests {
		w := apitest.PerformRequest(r, rq.method, rq.path, nil)
		if rq.method == "GET" {
			assert.Equal(t, http.StatusUnauthorized, w.Code, "%s %s", rq.method, rq.path)
		} else {
			// Anonymous mutations are stopped by CSRF (403) before auth (401);
			// either way nothing reaches the handler.
			assert.Contains(t, []int{http.StatusUnauthorized, http.StatusForbidden}, w.Code, "%s %s", rq.method, rq.path)
		}
		assert.NotContains(t, w.Body.String(), "deliveries", "%s %s", rq.method, rq.path)
		assert.NotContains(t, w.Body.String(), "targets", "%s %s", rq.method, rq.path)
	}
}

func TestWebhooks_NonAdminIsForbidden(t *testing.T) {
	r, _, analyst := setupWebhookAuthzAPI(t)
	for _, rq := range webhookAuthzRequests {
		w := apitest.PerformAuthRequest(r, rq.method, rq.path, nil, analyst.APIKey)
		assert.Equal(t, http.StatusForbidden, w.Code, "%s %s", rq.method, rq.path)
		assert.NotContains(t, w.Body.String(), "deliveries", "no webhook data may leak to a non-admin: %s", rq.path)
	}
}

func TestWebhooks_AdminCanReadStatus(t *testing.T) {
	r, admin, _ := setupWebhookAuthzAPI(t)
	w := apitest.PerformAuthRequest(r, "GET", "/api/v1/webhooks/status", nil, admin.APIKey)
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

// The permission catalogue must know the resource the routes are annotated
// with, otherwise role editors cannot grant it.
func TestWebhooks_PermissionCatalogued(t *testing.T) {
	var found bool
	for _, r := range authz.Catalogue() {
		if r.Key == "webhooks" {
			found = true
			assert.Equal(t, "Governance", r.Group)
			assert.True(t, r.Privileged, "approving a target sends data out; role editors should be warned")
			assert.Len(t, r.Actions, 4)
		}
	}
	assert.True(t, found, "webhooks resource registered in pkg/authz catalogue")
}
