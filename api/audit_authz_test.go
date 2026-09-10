package api

import (
	"net/http"
	"testing"

	apitest "github.com/TykTechnologies/midsommar/v2/api/testing"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The audit trail is the most sensitive read surface in the API. The other
// audit tests run with TestMode on, which bypasses AuthMiddleware/AdminOnly,
// so this test builds the router with TestMode OFF and proves the real
// guards: anonymous → 401, authenticated non-admin → 403, admin → 200.
// It runs in both editions: /audit/status answers 200 for an admin in CE too.
func setupAuditAuthzAPI(t *testing.T) (*gin.Engine, *models.User, *models.User) {
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

var auditReadPaths = []string{
	"/api/v1/audit/status",
	"/api/v1/audit/records",
	"/api/v1/audit/records/1",
	"/api/v1/audit/summary",
	"/api/v1/audit/export",
	"/api/v1/audit/resources/llm/1",
}

func TestAudit_AnonymousIsUnauthorized(t *testing.T) {
	r, _, _ := setupAuditAuthzAPI(t)
	for _, p := range auditReadPaths {
		w := apitest.PerformRequest(r, "GET", p, nil)
		assert.Equal(t, http.StatusUnauthorized, w.Code, p)
	}
}

func TestAudit_NonAdminIsForbidden(t *testing.T) {
	r, _, analyst := setupAuditAuthzAPI(t)
	for _, p := range auditReadPaths {
		w := apitest.PerformAuthRequest(r, "GET", p, nil, analyst.APIKey)
		assert.Equal(t, http.StatusForbidden, w.Code, p)
		assert.Contains(t, w.Body.String(), "Forbidden", p)
		assert.NotContains(t, w.Body.String(), "records", "no audit data may leak to a non-admin: %s", p)
	}
}

func TestAudit_AdminIsAllowed(t *testing.T) {
	r, admin, _ := setupAuditAuthzAPI(t)
	w := apitest.PerformAuthRequest(r, "GET", "/api/v1/audit/status", nil, admin.APIKey)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"available"`)
}
