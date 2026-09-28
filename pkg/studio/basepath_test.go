//go:build !enterprise

package studio

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// A host serves Studio under /ai-studio next to its own routes, mounting the
// OAuth discovery document at the host root as RFC 8414 requires.
func TestStudioEmbeddedUnderABasePath(t *testing.T) {
	opts := newTestOptions(t)
	opts.Config.BasePath = "/ai-studio"
	s, err := New(opts)
	require.NoError(t, err)
	defer stopStudio(t, s)

	host := http.NewServeMux()
	host.Handle("/ai-studio/", s.HTTPHandler())
	host.Handle("/ai-studio", s.HTTPHandler())
	host.Handle("/.well-known/oauth-authorization-server/ai-studio", s.OAuthMetadataHandler())
	host.HandleFunc("/host", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("host")) })
	srv := httptest.NewTLSServer(host)
	defer srv.Close()
	opts.Config.SiteURL = srv.URL + "/ai-studio"
	opts.Config.AuthServerURL = srv.URL + "/ai-studio"

	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	client := srv.Client()
	client.Jar = jar
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	base, _ := url.Parse(srv.URL)
	jar.SetCookies(base, []*http.Cookie{{Name: "host_session", Value: "keep-me", Path: "/"}})

	do := func(method, path string, body []byte, header http.Header) *http.Response {
		req, err := http.NewRequest(method, srv.URL+path, bytes.NewReader(body))
		require.NoError(t, err)
		for k, v := range header {
			req.Header[k] = v
		}
		// Over TLS the CSRF check requires a same-origin Referer.
		req.Header.Set("Referer", srv.URL+"/ai-studio/")
		resp, err := client.Do(req)
		require.NoError(t, err)
		t.Cleanup(func() { resp.Body.Close() })
		return resp
	}
	read := func(resp *http.Response) string {
		b, _ := io.ReadAll(resp.Body)
		return string(b)
	}

	assert.Equal(t, http.StatusOK, do("GET", "/ai-studio/health", nil, nil).StatusCode)
	assert.Equal(t, http.StatusUnauthorized, do("GET", "/ai-studio/api/v1/llms", nil, nil).StatusCode)

	bare := do("GET", "/ai-studio", nil, nil)
	assert.Equal(t, http.StatusMovedPermanently, bare.StatusCode)
	assert.Equal(t, "/ai-studio/", bare.Header.Get("Location"))

	index := read(do("GET", "/ai-studio/admin/llms", nil, nil))
	assert.Contains(t, index, `<base href="/ai-studio/">`)

	var authConfig struct {
		BasePath string `json:"basePath"`
	}
	require.NoError(t, json.Unmarshal([]byte(read(do("GET", "/ai-studio/auth/config", nil, nil))), &authConfig))
	assert.Equal(t, "/ai-studio", authConfig.BasePath)

	var metadata struct {
		Issuer                string `json:"issuer"`
		AuthorizationEndpoint string `json:"authorization_endpoint"`
	}
	require.NoError(t, json.Unmarshal([]byte(read(do("GET", "/.well-known/oauth-authorization-server/ai-studio", nil, nil))), &metadata))
	assert.Equal(t, srv.URL+"/ai-studio", metadata.Issuer)
	assert.Equal(t, srv.URL+"/ai-studio/oauth/authorize", metadata.AuthorizationEndpoint)

	// Log in: the session and CSRF cookies are scoped to the base path.
	user := &models.User{Email: "admin@example.com", Name: "Admin", IsAdmin: true, EmailVerified: true}
	require.NoError(t, user.SetPassword("Passw0rd!Passw0rd"))
	require.NoError(t, opts.DB.Create(user).Error)

	token := do("GET", "/ai-studio/csrf-token", nil, nil).Header.Get("X-CSRF-Token")
	require.NotEmpty(t, token)
	jsonHeader := http.Header{"Content-Type": {"application/json"}, "X-Csrf-Token": {token}}
	login := do("POST", "/ai-studio/auth/login", []byte(`{"data":{"type":"login","attributes":{"email":"admin@example.com","password":"Passw0rd!Passw0rd"}}}`), jsonHeader)
	require.Equal(t, http.StatusOK, login.StatusCode, read(login))
	sessionCookie := cookieNamed(login.Cookies(), "session")
	require.NotNil(t, sessionCookie)
	assert.Equal(t, "/ai-studio", sessionCookie.Path)

	assert.Equal(t, http.StatusOK, do("GET", "/ai-studio/api/v1/llms", nil, nil).StatusCode, "the session authenticates API calls under the base path")

	// Log out: Studio expires its own session, not the host's cookie.
	logout := do("POST", "/ai-studio/common/logout", nil, jsonHeader)
	require.Equal(t, http.StatusOK, logout.StatusCode, read(logout))
	for _, c := range logout.Cookies() {
		assert.NotEqual(t, "host_session", c.Name, "logout must not touch the host's cookies")
	}
	assert.Equal(t, "keep-me", cookieNamed(jar.Cookies(base), "host_session").Value)

	assert.Equal(t, "host", read(do("GET", "/host", nil, nil)), "the host's own routes are untouched")
}

func cookieNamed(cookies []*http.Cookie, name string) *http.Cookie {
	for _, c := range cookies {
		if strings.EqualFold(c.Name, name) {
			return c
		}
	}
	return nil
}
