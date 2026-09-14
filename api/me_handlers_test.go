package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	apitest "github.com/TykTechnologies/midsommar/v2/api/testing"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Self-service profile routes under /common/me with the real auth
// middleware (TestMode off): the profile fields on /common/me, the
// notification preferences, and rolling/revoking one's own API key under
// the same SSO policy as the admin routes, without losing the session.

func meAttrs(t *testing.T, w *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp struct {
		Attributes map[string]interface{} `json:"attributes"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return resp.Attributes
}

func dataOf(t *testing.T, w *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var resp struct {
		Data map[string]interface{} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp), w.Body.String())
	return resp.Data
}

// cookieRequest performs a safe (GET) request authenticated by the session
// cookie rather than a key.
func cookieRequest(r http.Handler, path, token string) *httptest.ResponseRecorder {
	req, _ := http.NewRequest("GET", path, nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: token})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestMe_ProfileFields(t *testing.T) {
	f := setupLifecycleAPI(t)

	attrs := meAttrs(t, apitest.PerformAuthRequest(f.router, "GET", "/common/me?skip_entitlements=true", nil, f.admin.APIKey))
	assert.Equal(t, models.RoleSuperAdmin, attrs["account_type"])
	assert.Equal(t, models.AuthSourceLocal, attrs["auth_source"])
	assert.Equal(t, true, attrs["has_api_key"])
	assert.Equal(t, true, attrs["sso_api_keys_allowed"])
	assert.Equal(t, true, attrs["email_notifications_enabled"], "email delivery defaults on")
	assert.Equal(t, false, attrs["notifications_enabled"])
	_, hasKey := attrs["api_key"]
	assert.False(t, hasKey, "the key itself is never included")
	// The stamp is written after the user was loaded for this request, so
	// it shows from the second key-authenticated call on.
	attrs = meAttrs(t, apitest.PerformAuthRequest(f.router, "GET", "/common/me?skip_entitlements=true", nil, f.admin.APIKey))
	assert.NotNil(t, attrs["api_key_last_used_at"])

	attrs = meAttrs(t, apitest.PerformAuthRequest(f.router, "GET", "/common/me?skip_entitlements=true", nil, f.member.APIKey))
	assert.Equal(t, models.RoleDeveloper, attrs["account_type"], "NewUser() shows the portal")

	// An SSO-origin user without a key can only be checked through a
	// session; give them one and read the policy flag.
	require.NoError(t, f.auth.Config.DB.Model(&models.User{}).Where("id = ?", f.ssoUser.ID).Update("session_token", "sso-session").Error)
	attrs = meAttrs(t, cookieRequest(f.router, "/common/me?skip_entitlements=true", "sso-session"))
	assert.Equal(t, models.AuthSourceSSO, attrs["auth_source"])
	assert.Equal(t, false, attrs["has_api_key"])
	assert.Equal(t, false, attrs["sso_api_keys_allowed"])

	f.auth.Config.AllowSSOUserAPIKeys = true
	attrs = meAttrs(t, cookieRequest(f.router, "/common/me?skip_entitlements=true", "sso-session"))
	assert.Equal(t, true, attrs["sso_api_keys_allowed"])
}

func TestMe_Preferences(t *testing.T) {
	f := setupLifecycleAPI(t)

	w := apitest.PerformAuthRequest(f.router, "GET", "/common/me/preferences", nil, f.member.APIKey)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	prefs := dataOf(t, w)
	assert.Equal(t, false, prefs["notifications_enabled"])
	assert.Equal(t, true, prefs["email_notifications_enabled"])

	// Switch email off: the response and a fresh read both show it off.
	w = apitest.PerformAuthRequest(f.router, "PATCH", "/common/me/preferences", gin.H{"email_notifications_enabled": false}, f.member.APIKey)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, false, dataOf(t, w)["email_notifications_enabled"])
	w = apitest.PerformAuthRequest(f.router, "GET", "/common/me/preferences", nil, f.member.APIKey)
	assert.Equal(t, false, dataOf(t, w)["email_notifications_enabled"])
	attrs := meAttrs(t, apitest.PerformAuthRequest(f.router, "GET", "/common/me?skip_entitlements=true", nil, f.member.APIKey))
	assert.Equal(t, false, attrs["email_notifications_enabled"])

	// And back on.
	w = apitest.PerformAuthRequest(f.router, "PATCH", "/common/me/preferences", gin.H{"email_notifications_enabled": true}, f.member.APIKey)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, true, dataOf(t, w)["email_notifications_enabled"])

	// The in-app fan-out flag is admin-only, the same rule as the user form.
	w = apitest.PerformAuthRequest(f.router, "PATCH", "/common/me/preferences", gin.H{"notifications_enabled": true}, f.member.APIKey)
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	w = apitest.PerformAuthRequest(f.router, "PATCH", "/common/me/preferences", gin.H{"notifications_enabled": true, "email_notifications_enabled": false}, f.admin.APIKey)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	prefs = dataOf(t, w)
	assert.Equal(t, true, prefs["notifications_enabled"])
	assert.Equal(t, false, prefs["email_notifications_enabled"])

	// An empty body changes nothing.
	w = apitest.PerformAuthRequest(f.router, "PATCH", "/common/me/preferences", gin.H{}, f.admin.APIKey)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// Unauthenticated callers are refused.
	w = apitest.PerformRequest(f.router, "GET", "/common/me/preferences", nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestMe_RollAPIKeyKeepsSession(t *testing.T) {
	f := setupLifecycleAPI(t)
	db := f.auth.Config.DB

	// The member is signed in through a browser session as well.
	require.NoError(t, db.Model(&models.User{}).Where("id = ?", f.member.ID).Update("session_token", "member-session").Error)
	require.Equal(t, http.StatusOK, cookieRequest(f.router, "/common/me?skip_entitlements=true", "member-session").Code)

	oldKey := f.member.APIKey
	w := apitest.PerformAuthRequest(f.router, "POST", "/common/me/api-key/roll", nil, oldKey)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	newKey, _ := dataOf(t, w)["api_key"].(string)
	require.NotEmpty(t, newKey)
	assert.NotEqual(t, oldKey, newKey)

	assert.Equal(t, http.StatusUnauthorized, meWith(f, oldKey), "the old key is dead")
	assert.Equal(t, http.StatusOK, meWith(f, newKey), "the new key works")
	assert.Equal(t, http.StatusOK, cookieRequest(f.router, "/common/me?skip_entitlements=true", "member-session").Code,
		"rolling the key must not sign the browser out")

	var stored models.User
	require.NoError(t, db.First(&stored, f.member.ID).Error)
	assert.Equal(t, "member-session", stored.SessionToken)
	assert.Equal(t, newKey, stored.APIKey)
}

func TestMe_RollAPIKeySSOPolicy(t *testing.T) {
	f := setupLifecycleAPI(t)
	db := f.auth.Config.DB
	require.NoError(t, db.Model(&models.User{}).Where("id = ?", f.ssoUser.ID).Update("session_token", "sso-session").Error)

	roll := func() *httptest.ResponseRecorder {
		// A session-authenticated POST would need a CSRF token; the policy
		// check runs before anything else, so a Bearer request with a
		// (deliberately invalid) header is not usable either. Exercise the
		// handler through the router with the member's key to prove the
		// route shape, then the SSO user through a direct handler call.
		req, _ := http.NewRequest("POST", "/common/me/api-key/roll", nil)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = req
		var u models.User
		require.NoError(t, db.First(&u, f.ssoUser.ID).Error)
		c.Set("user", &u)
		f.api.rollMyAPIKey(c)
		return w
	}

	w := roll()
	assert.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "ALLOW_SSO_USER_API_KEYS")
	var stored models.User
	require.NoError(t, db.First(&stored, f.ssoUser.ID).Error)
	assert.Empty(t, stored.APIKey, "no key was issued")

	f.auth.Config.AllowSSOUserAPIKeys = true
	w = roll()
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	key, _ := dataOf(t, w)["api_key"].(string)
	require.NotEmpty(t, key)
	assert.Equal(t, http.StatusOK, meWith(f, key), "recent SSO login keeps the key alive")
	assert.Equal(t, http.StatusOK, cookieRequest(f.router, "/common/me?skip_entitlements=true", "sso-session").Code)
}

func TestMe_RevokeAPIKeyKeepsSession(t *testing.T) {
	f := setupLifecycleAPI(t)
	db := f.auth.Config.DB
	require.NoError(t, db.Model(&models.User{}).Where("id = ?", f.member.ID).Update("session_token", "member-session").Error)

	w := apitest.PerformAuthRequest(f.router, "DELETE", "/common/me/api-key", nil, f.member.APIKey)
	assert.Equal(t, http.StatusNoContent, w.Code, w.Body.String())

	assert.Equal(t, http.StatusUnauthorized, meWith(f, f.member.APIKey), "the revoked key is dead")
	attrs := meAttrs(t, cookieRequest(f.router, "/common/me?skip_entitlements=true", "member-session"))
	assert.Equal(t, false, attrs["has_api_key"])
}
