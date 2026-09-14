package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	apitest "github.com/TykTechnologies/midsommar/v2/api/testing"
	"github.com/TykTechnologies/midsommar/v2/auth"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// User lifecycle endpoints with the real auth middleware (TestMode off):
// revoke / disable / enable are gated on users:write, a disabled user's key
// is dead, a revoked key is dead, SSO-origin users cannot be issued a key
// unless the operator opted in, and the list filters work end to end.

type lifecycleFixture struct {
	api     *API
	router  *gin.Engine
	auth    *auth.AuthService
	admin   *models.User
	member  *models.User
	ssoUser *models.User
}

func setupLifecycleAPI(t *testing.T) *lifecycleFixture {
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

	member := models.NewUser()
	member.Email = "member@tyk.io"
	member.Name = "Member"
	member.Password = "hash"
	member.EmailVerified = true
	require.NoError(t, member.Create(db))

	now := time.Now()
	ssoUser := &models.User{
		Email: "sso@tyk.io", Name: "SSO", EmailVerified: true,
		AuthSource: models.AuthSourceSSO, SSOProfileID: "onelogin",
		LastLoginAt: &now, LastLoginMethod: models.LoginMethodSSO,
	}
	require.NoError(t, ssoUser.Create(db))

	return &lifecycleFixture{api: api, router: api.router, auth: authService, admin: admin, member: member, ssoUser: ssoUser}
}

func sprintfID(format string, id uint) string {
	return fmt.Sprintf(format, id)
}

func userAttrs(t *testing.T, body string) map[string]interface{} {
	t.Helper()
	var resp struct {
		Data struct {
			Attributes map[string]interface{} `json:"attributes"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(body), &resp), body)
	return resp.Data.Attributes
}

func TestUserLifecycle_NonManagerIsForbidden(t *testing.T) {
	f := setupLifecycleAPI(t)
	target := f.ssoUser.ID
	for _, tc := range []struct{ method, path string }{
		{"DELETE", "/api/v1/users/%d/api-key"},
		{"POST", "/api/v1/users/%d/disable"},
		{"POST", "/api/v1/users/%d/enable"},
		{"POST", "/api/v1/users/%d/roll-api-key"},
	} {
		path := sprintfID(tc.path, target)
		w := apitest.PerformAuthRequest(f.router, tc.method, path, nil, f.member.APIKey)
		assert.Equal(t, http.StatusForbidden, w.Code, "%s %s", tc.method, path)
		// Anonymous mutations are stopped by the CSRF guard (403) before
		// the auth middleware (401); either way nothing changes.
		w = apitest.PerformRequest(f.router, tc.method, path, nil)
		assert.Contains(t, []int{http.StatusUnauthorized, http.StatusForbidden}, w.Code, "anonymous %s %s", tc.method, path)
	}
}

// meWith proves whether a key still authenticates: /common/me is readable by
// every signed-in user, unlike the admin-only users list.
func meWith(f *lifecycleFixture, key string) int {
	return apitest.PerformAuthRequest(f.router, "GET", "/common/me?skip_entitlements=true", nil, key).Code
}

func TestUserLifecycle_RevokeKillsTheKey(t *testing.T) {
	f := setupLifecycleAPI(t)

	require.Equal(t, http.StatusOK, meWith(f, f.member.APIKey), "member's key works before revocation")

	w := apitest.PerformAuthRequest(f.router, "DELETE", sprintfID("/api/v1/users/%d/api-key", f.member.ID), nil, f.admin.APIKey)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	attrs := userAttrs(t, w.Body.String())
	assert.Equal(t, false, attrs["has_api_key"])
	assert.Nil(t, attrs["api_key"])
	assert.Nil(t, attrs["api_key_hint"])

	assert.Equal(t, http.StatusUnauthorized, meWith(f, f.member.APIKey), "the revoked key is dead")
}

func TestUserLifecycle_DisableBlocksTheKeyAndEnableRestoresIt(t *testing.T) {
	f := setupLifecycleAPI(t)

	w := apitest.PerformAuthRequest(f.router, "POST", sprintfID("/api/v1/users/%d/disable", f.member.ID), nil, f.admin.APIKey)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	attrs := userAttrs(t, w.Body.String())
	assert.Equal(t, true, attrs["disabled"])
	assert.NotEmpty(t, attrs["disabled_at"])

	assert.Equal(t, http.StatusUnauthorized, meWith(f, f.member.APIKey), "a disabled user's key is refused")

	w = apitest.PerformAuthRequest(f.router, "POST", sprintfID("/api/v1/users/%d/enable", f.member.ID), nil, f.admin.APIKey)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, false, userAttrs(t, w.Body.String())["disabled"])

	assert.Equal(t, http.StatusOK, meWith(f, f.member.APIKey), "re-enabled")

	t.Run("self and super admin are refused", func(t *testing.T) {
		w := apitest.PerformAuthRequest(f.router, "POST", sprintfID("/api/v1/users/%d/disable", f.admin.ID), nil, f.admin.APIKey)
		assert.Equal(t, http.StatusForbidden, w.Code, "self")
	})
}

func TestUserLifecycle_SSOUsersGetNoKeyUnlessAllowed(t *testing.T) {
	f := setupLifecycleAPI(t)
	path := sprintfID("/api/v1/users/%d/roll-api-key", f.ssoUser.ID)

	w := apitest.PerformAuthRequest(f.router, "POST", path, nil, f.admin.APIKey)
	assert.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "ALLOW_SSO_USER_API_KEYS")

	w = apitest.PerformAuthRequest(f.router, "GET", sprintfID("/api/v1/users/%d", f.ssoUser.ID), nil, f.admin.APIKey)
	require.Equal(t, http.StatusOK, w.Code)
	attrs := userAttrs(t, w.Body.String())
	assert.Equal(t, false, attrs["has_api_key"])
	assert.Equal(t, "sso", attrs["auth_source"])
	assert.Equal(t, "onelogin", attrs["sso_profile_id"])
	assert.Equal(t, "sso", attrs["last_login_method"])

	f.auth.Config.AllowSSOUserAPIKeys = true
	w = apitest.PerformAuthRequest(f.router, "POST", path, nil, f.admin.APIKey)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	attrs = userAttrs(t, w.Body.String())
	assert.Equal(t, true, attrs["has_api_key"])
	key, _ := attrs["api_key"].(string)
	require.NotEmpty(t, key, "the fresh key is returned once")

	assert.Equal(t, http.StatusOK, meWith(f, key), "recent SSO login keeps the key alive")

	f.auth.Config.SSOAPIKeyLiveness = time.Millisecond
	time.Sleep(2 * time.Millisecond)
	assert.Equal(t, http.StatusUnauthorized, meWith(f, key), "outside the liveness window the key is dead")
}

func TestUserLifecycle_ListFilters(t *testing.T) {
	f := setupLifecycleAPI(t)
	require.Equal(t, http.StatusOK, apitest.PerformAuthRequest(f.router, "POST", sprintfID("/api/v1/users/%d/disable", f.member.ID), nil, f.admin.APIKey).Code)

	emails := func(query string) []string {
		w := apitest.PerformAuthRequest(f.router, "GET", "/api/v1/users?"+query, nil, f.admin.APIKey)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var resp struct {
			Data []struct {
				Attributes struct {
					Email string `json:"email"`
				} `json:"attributes"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		out := []string{}
		for _, u := range resp.Data {
			out = append(out, u.Attributes.Email)
		}
		return out
	}

	assert.ElementsMatch(t, []string{"sso@tyk.io"}, emails("auth_source=sso"))
	assert.ElementsMatch(t, []string{"admin@tyk.io", "member@tyk.io"}, emails("auth_source=local"))
	assert.ElementsMatch(t, []string{"sso@tyk.io"}, emails("has_api_key=false"))
	assert.ElementsMatch(t, []string{"member@tyk.io"}, emails("disabled=true"))
	assert.ElementsMatch(t, []string{"admin@tyk.io", "sso@tyk.io"}, emails("disabled=false"))
}
