package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	apitest "github.com/TykTechnologies/midsommar/v2/api/testing"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/authz"
	"github.com/TykTechnologies/midsommar/v2/services/rbac"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests build the router with TestMode OFF so the real authorization
// chain runs. They pass in both editions: with the community stub an admin
// resolves to the wildcard and a non-admin to nothing, which must reproduce
// the historical admin-or-not behaviour byte for byte (bar the error code).
func setupAuthzAPI(t *testing.T) (*API, *models.User, *models.User) {
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
	admin.Email = "owner@tyk.io"
	admin.Name = "Owner"
	admin.Password = "hash"
	admin.IsAdmin = true
	admin.EmailVerified = true
	require.NoError(t, admin.Create(db))

	member := models.NewUser()
	member.Email = "member@tyk.io"
	member.Name = "Member"
	member.Password = "hash"
	member.EmailVerified = true
	require.NoError(t, member.Create(db))

	return api, admin, member
}

func decodeErrorCode(t *testing.T, body []byte) string {
	t.Helper()
	var eb authz.ErrorBody
	require.NoError(t, json.Unmarshal(body, &eb), string(body))
	require.NotEmpty(t, eb.Errors)
	return eb.Errors[0].Code
}

func TestAuthzMiddleware_AnonymousIsUnauthenticated(t *testing.T) {
	api, _, _ := setupAuthzAPI(t)
	w := apitest.PerformRequest(api.router, "GET", "/api/v1/llms", nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAuthzMiddleware_NonAdminIsDeniedWithCode(t *testing.T) {
	api, _, member := setupAuthzAPI(t)
	for _, p := range []string{"/api/v1/llms", "/api/v1/users", "/api/v1/rbac/permissions", "/api/v1/audit/records"} {
		w := apitest.PerformAuthRequest(api.router, "GET", p, nil, member.APIKey)
		assert.Equal(t, http.StatusForbidden, w.Code, p)
		assert.Contains(t, w.Body.String(), "Forbidden", p)
		assert.Equal(t, authz.CodePermissionDenied, decodeErrorCode(t, w.Body.Bytes()), p)
	}
}

func TestAuthzMiddleware_AdminIsAllowed(t *testing.T) {
	api, admin, _ := setupAuthzAPI(t)
	for _, p := range []string{"/api/v1/llms", "/api/v1/users", "/api/v1/rbac/permissions"} {
		w := apitest.PerformAuthRequest(api.router, "GET", p, nil, admin.APIKey)
		assert.Equal(t, http.StatusOK, w.Code, "%s: %s", p, w.Body.String())
	}
}

func TestAuthzMiddleware_MeReportsEffectiveAccess(t *testing.T) {
	api, admin, _ := setupAuthzAPI(t)
	w := apitest.PerformAuthRequest(api.router, "GET", "/api/v1/rbac/me", nil, admin.APIKey)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp EffectiveAccessResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, admin.ID, resp.UserID)
	assert.Equal(t, []string{"*"}, resp.Permissions)
	assert.True(t, resp.IsFullAdmin)
	assert.True(t, resp.HasAdminAccess)
	assert.Equal(t, rbac.IsEnterpriseAvailable() && api.service.Authz().Enabled(), resp.Enabled)
}

func TestAuthzMiddleware_PermissionCatalogue(t *testing.T) {
	api, admin, _ := setupAuthzAPI(t)
	w := apitest.PerformAuthRequest(api.router, "GET", "/api/v1/rbac/permissions", nil, admin.APIKey)
	require.Equal(t, http.StatusOK, w.Code)
	var resp PermissionCatalogueResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, authz.Groups, resp.Groups)
	assert.Len(t, resp.Resources, len(authz.Catalogue()))
	assert.Equal(t, authz.Actions, resp.Actions)
}

// The permission context is attached on /common too, so portal handlers can
// use authz.Can for admin-or-owner decisions.
func TestAuthzMiddleware_ContextOnCommonGroup(t *testing.T) {
	api, admin, member := setupAuthzAPI(t)
	var got map[string]bool
	api.router.GET("/common/authz-probe", api.auth.AuthMiddleware(), api.rbacContext(), func(c *gin.Context) {
		got = map[string]bool{
			"llms:write": authz.Can(c, authz.Write("llms")),
			"any":        authz.Can(c, authz.AnyAdmin),
		}
		c.Status(http.StatusNoContent)
	})

	apitest.PerformAuthRequest(api.router, "GET", "/common/authz-probe", nil, admin.APIKey)
	assert.True(t, got["llms:write"])
	assert.True(t, got["any"])

	apitest.PerformAuthRequest(api.router, "GET", "/common/authz-probe", nil, member.APIKey)
	assert.False(t, got["llms:write"])
	assert.False(t, got["any"])
}

func TestAuthzMiddleware_UnannotatedRouteFailsClosed(t *testing.T) {
	api, admin, member := setupAuthzAPI(t)
	// Register a route on the v1 group that bypasses the annotation wrapper.
	api.router.GET("/api/v1/unannotated-probe", api.auth.AuthMiddleware(), api.rbacContext(), api.requireRoutePermission(), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	w := apitest.PerformAuthRequest(api.router, "GET", "/api/v1/unannotated-probe", nil, member.APIKey)
	assert.Equal(t, http.StatusForbidden, w.Code)
	w = apitest.PerformAuthRequest(api.router, "GET", "/api/v1/unannotated-probe", nil, admin.APIKey)
	assert.Equal(t, http.StatusNoContent, w.Code, "full admins still pass an unannotated route")
}

func TestAuthzMiddleware_TestModeBypasses(t *testing.T) {
	api, _ := setupTestAPI(t) // TestMode on
	w := apitest.PerformRequest(api.router, "GET", "/api/v1/rbac/permissions", nil)
	assert.Equal(t, http.StatusOK, w.Code)
}

// Community Edition answers every RBAC management call with 402 so the UI
// can show an upgrade prompt instead of a permission error.
func TestRBACHandlers_CommunityReturnsPaymentRequired(t *testing.T) {
	if rbac.IsEnterpriseAvailable() {
		t.Skip("enterprise build: management endpoints are live")
	}
	api, admin, _ := setupAuthzAPI(t)
	cases := []struct{ method, path string }{
		{"GET", "/api/v1/rbac/roles"},
		{"GET", "/api/v1/rbac/roles/1"},
		{"POST", "/api/v1/rbac/roles"},
		{"PATCH", "/api/v1/rbac/roles/1"},
		{"DELETE", "/api/v1/rbac/roles/1"},
		{"POST", "/api/v1/rbac/roles/1/clone"},
		{"GET", "/api/v1/rbac/bindings"},
		{"POST", "/api/v1/rbac/bindings"},
		{"DELETE", "/api/v1/rbac/bindings/1"},
	}
	for _, tc := range cases {
		body := map[string]interface{}{"name": "x", "permissions": []string{}, "subject_type": "user", "subject_id": 1, "role_id": 1}
		w := apitest.PerformAuthRequest(api.router, tc.method, tc.path, body, admin.APIKey)
		assert.Equal(t, http.StatusPaymentRequired, w.Code, "%s %s: %s", tc.method, tc.path, w.Body.String())
		assert.Equal(t, authz.CodeEnterpriseRequired, decodeErrorCode(t, w.Body.Bytes()), tc.path)
	}

	// Effective access still works in CE from the admin flag.
	w := apitest.PerformAuthRequest(api.router, "GET", "/api/v1/rbac/users/"+strconv.FormatUint(uint64(admin.ID), 10)+"/effective", nil, admin.APIKey)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp EffectiveAccessResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, []string{"*"}, resp.Permissions)
	assert.False(t, resp.Enabled)
}
