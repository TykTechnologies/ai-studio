//go:build !enterprise

package studio

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// headerAuth stands in for a host's session handling: the user named in
// X-Host-User is signed in, "admin" is an administrator and "reject" is a
// request the host refuses.
type headerAuth struct{}

func (headerAuth) Authenticate(r *http.Request) (*Identity, error) {
	switch name := r.Header.Get("X-Host-User"); name {
	case "":
		return nil, nil
	case "reject":
		return nil, errors.New("host session expired")
	default:
		return &Identity{Subject: "host-" + name, Email: name + "@host.example.com", Name: strings.ToUpper(name), Admin: name == "admin"}, nil
	}
}

// hostCSRF refuses unsafe requests without the host's token header.
func hostCSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Header.Get("X-Host-CSRF") != "ok" {
			http.Error(w, "host csrf", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func TestStudioWithHostAuthentication(t *testing.T) {
	opts := newTestOptions(t)
	opts.Config.BasePath = "/ai-studio"
	opts.Auth = headerAuth{}
	opts.LoginURL = "/login"
	opts.LogoutURL = "/logout"
	opts.CSRF = hostCSRF
	opts.CSRFTokenHeader = "X-Host-CSRF"
	opts.CSRFTokenURL = "/host/csrf"
	s, err := New(opts)
	require.NoError(t, err)
	defer stopStudio(t, s)

	srv := httptest.NewServer(s.HTTPHandler())
	defer srv.Close()

	do := func(method, path string, header map[string]string) *http.Response {
		req, err := http.NewRequest(method, srv.URL+path, strings.NewReader("{}"))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		for k, v := range header {
			req.Header.Set(k, v)
		}
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		t.Cleanup(func() { resp.Body.Close() })
		return resp
	}

	// The host's user is provisioned and authorised by Studio's own RBAC.
	assert.Equal(t, http.StatusOK, do("GET", "/ai-studio/api/v1/llms", map[string]string{"X-Host-User": "admin"}).StatusCode)
	var admin models.User
	require.NoError(t, opts.DB.Where("external_subject = ?", "host-admin").First(&admin).Error)
	assert.Equal(t, models.AuthSourceHost, admin.AuthSource)
	assert.True(t, admin.IsAdmin)

	assert.Equal(t, http.StatusForbidden, do("GET", "/ai-studio/api/v1/llms", map[string]string{"X-Host-User": "bob"}).StatusCode,
		"a non-admin host user is not an administrator")
	assert.Equal(t, http.StatusUnauthorized, do("GET", "/ai-studio/api/v1/llms", nil).StatusCode)
	assert.Equal(t, http.StatusUnauthorized, do("GET", "/ai-studio/api/v1/llms", map[string]string{"X-Host-User": "reject"}).StatusCode)

	// Studio's own API keys still work for requests without a host identity.
	keyUser := models.NewUser()
	keyUser.Email, keyUser.Name, keyUser.IsAdmin, keyUser.EmailVerified = "svc@example.com", "Service", true, true
	require.NoError(t, keyUser.Create(opts.DB))
	assert.Equal(t, http.StatusOK, do("GET", "/ai-studio/api/v1/llms", map[string]string{"Authorization": "Bearer " + keyUser.APIKey}).StatusCode)

	// Studio's own sign-in is off.
	for _, p := range []string{"/ai-studio/auth/login", "/ai-studio/auth/register", "/ai-studio/auth/forgot-password"} {
		assert.Equal(t, http.StatusNotFound, do("POST", p, map[string]string{"X-Host-CSRF": "ok"}).StatusCode, p)
	}
	assert.Equal(t, http.StatusNotFound, do("GET", "/ai-studio/sso", nil).StatusCode)

	// The host's CSRF protection guards cookie-style writes.
	assert.Equal(t, http.StatusForbidden, do("POST", "/ai-studio/common/logout", map[string]string{"X-Host-User": "admin"}).StatusCode)
	assert.Equal(t, http.StatusOK, do("POST", "/ai-studio/common/logout", map[string]string{"X-Host-User": "admin", "X-Host-CSRF": "ok"}).StatusCode)

	// The console learns how to sign in and fetch tokens.
	body, _ := io.ReadAll(do("GET", "/ai-studio/auth/config", nil).Body)
	var cfg map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &cfg))
	assert.Equal(t, "host", cfg["authMode"])
	assert.Equal(t, "/login", cfg["loginURL"])
	assert.Equal(t, "/logout", cfg["logoutURL"])
	assert.Equal(t, "X-Host-CSRF", cfg["csrfTokenHeader"])
	assert.Equal(t, "/host/csrf", cfg["csrfTokenURL"])
	assert.Equal(t, false, cfg["tibEnabled"])
}
