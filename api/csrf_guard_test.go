package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/csrf"
	"github.com/stretchr/testify/assert"
)

// The CSRF middleware is disabled wholesale under TestMode, so nothing in the
// suite exercised it and a regression here would reach production unnoticed.
// These drive csrfGuard directly.
//
// The property that matters is not the status code -- it is that a rejected
// write performs no mutation. In gin, a middleware returning without calling
// c.Next() does NOT stop the chain, so an omitted Abort means the route handler
// runs and writes after the 403 has been sent.

func newCSRFTestRouter(t *testing.T) (*gin.Engine, *bool) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()

	protect := csrf.Protect(
		[]byte("0123456789abcdef0123456789abcdef"),
		csrf.Secure(false),
		csrf.Path("/"),
		csrf.TrustedOrigins([]string{"localhost:3000"}),
	)
	r.Use(csrfGuard(protect))

	mutated := false
	r.POST("/thing", func(c *gin.Context) {
		mutated = true
		c.JSON(http.StatusCreated, gin.H{"data": gin.H{"type": "thing", "id": "1"}})
	})
	// A second method on a second path: the guard is one global middleware,
	// so its verdict must not depend on the route.
	r.PATCH("/other/1", func(c *gin.Context) {
		mutated = true
		c.JSON(http.StatusCreated, gin.H{"data": gin.H{"type": "other", "id": "1"}})
	})
	r.GET("/token", func(c *gin.Context) {
		c.Header("X-CSRF-Token", csrf.Token(c.Request))
		c.Status(http.StatusOK)
	})

	return r, &mutated
}

// csrfTokenDance fetches a token and its cookie, the way the admin UI does.
func csrfTokenDance(t *testing.T, r *gin.Engine) (string, []*http.Cookie) {
	t.Helper()
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("GET", "/token", nil))
	token := rec.Header().Get("X-CSRF-Token")
	assert.NotEmpty(t, token)
	return token, rec.Result().Cookies()
}

// tokenisedWrite performs a cookie-authenticated write carrying a valid
// token and cookie plus the given extra headers.
func tokenisedWrite(r *gin.Engine, token string, cookies []*http.Cookie, method, path string, headers map[string]string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, nil)
	req.Header.Set("X-CSRF-Token", token)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	for _, c := range cookies {
		req.AddCookie(c)
	}
	r.ServeHTTP(w, req)
	return w
}

// The review saw "403 referer not supplied" on POST /users and on metadata
// PUT/PATCH while an LLM PATCH went through. That is not per-endpoint
// behaviour: the guard is one middleware, so for a given header set every
// method and path must get the same verdict.
func TestCSRFGuard_VerdictIsUniformAcrossMethodsAndPaths(t *testing.T) {
	cases := []struct {
		name    string
		headers map[string]string
		want    int
	}{
		// Behind a TLS-terminating proxy the request is HTTPS to the browser,
		// so a write with neither Origin nor Referer is rejected.
		{"https via proxy, no origin or referer", map[string]string{"X-Forwarded-Proto": "https"}, http.StatusForbidden},
		{"https via proxy, matching origin", map[string]string{"X-Forwarded-Proto": "https", "Origin": "https://example.com"}, http.StatusCreated},
		// Plain HTTP: a browser is not required to send either header.
		{"plain http, no origin or referer", nil, http.StatusCreated},
		{"plain http, matching origin", map[string]string{"Origin": "http://example.com"}, http.StatusCreated},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, mutated := newCSRFTestRouter(t)
			token, cookies := csrfTokenDance(t, r)

			post := tokenisedWrite(r, token, cookies, "POST", "/thing", tc.headers)
			postMutated := *mutated
			*mutated = false
			patch := tokenisedWrite(r, token, cookies, "PATCH", "/other/1", tc.headers)

			assert.Equal(t, tc.want, post.Code, "POST: %s", post.Body.String())
			assert.Equal(t, post.Code, patch.Code, "PATCH on another path must get the same verdict as POST: %s", patch.Body.String())
			assert.Equal(t, postMutated, *mutated)
		})
	}
}

// A plain-HTTP deployment (dev, tests, a stack with no TLS in front) cannot
// require Origin or Referer: browsers legitimately omit both on same-origin
// requests, and gorilla's Referer check exists only to defeat HTTP
// machine-in-the-middle injection against an HTTPS site.
func TestCSRFGuard_PlainHTTPNeedsNoOriginOrReferer(t *testing.T) {
	r, mutated := newCSRFTestRouter(t)
	token, cookies := csrfTokenDance(t, r)

	w := tokenisedWrite(r, token, cookies, "POST", "/thing", nil)

	assert.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	assert.True(t, *mutated, "a tokenised write over plain HTTP must go through without Origin or Referer")
}

// When a proxy terminates TLS the browser is talking HTTPS, so the strict
// Referer check must stay in force even though r.TLS is nil here.
func TestCSRFGuard_ForwardedHTTPSStillRequiresReferer(t *testing.T) {
	for _, proto := range []string{"https", "HTTPS", "https, http"} {
		t.Run(proto, func(t *testing.T) {
			r, mutated := newCSRFTestRouter(t)
			token, cookies := csrfTokenDance(t, r)

			w := tokenisedWrite(r, token, cookies, "POST", "/thing", map[string]string{"X-Forwarded-Proto": proto})

			assert.Equal(t, http.StatusForbidden, w.Code)
			assert.Contains(t, w.Body.String(), "referer not supplied")
			assert.False(t, *mutated)

			// With a same-origin HTTPS referer the same request passes.
			w = tokenisedWrite(r, token, cookies, "POST", "/thing", map[string]string{
				"X-Forwarded-Proto": proto, "Referer": "https://example.com/admin",
			})
			assert.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		})
	}
}

// On plain HTTP the browser's Origin is http://<host>. gorilla compares it
// against a URL whose scheme defaults to https, so without the plaintext
// marker a same-host http Origin is rejected unless it is in TrustedOrigins.
func TestCSRFGuard_SameHostHTTPOriginOnPlainHTTP(t *testing.T) {
	r, mutated := newCSRFTestRouter(t)
	token, cookies := csrfTokenDance(t, r)

	w := tokenisedWrite(r, token, cookies, "POST", "/thing", map[string]string{"Origin": "http://example.com"})
	assert.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	assert.True(t, *mutated)

	// A cross-site Origin is still rejected on plain HTTP.
	*mutated = false
	w = tokenisedWrite(r, token, cookies, "POST", "/thing", map[string]string{"Origin": "http://evil.example"})
	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.False(t, *mutated)

	// A same-host https Origin over a plain-http socket is what a
	// TLS-terminating proxy that does not set X-Forwarded-Proto produces. It
	// passed before plaintext marking existed and must keep passing.
	*mutated = false
	w = tokenisedWrite(r, token, cookies, "POST", "/thing", map[string]string{"Origin": "https://example.com"})
	assert.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	assert.True(t, *mutated)
}

func TestCSRFGuard_RejectedWriteDoesNotMutate(t *testing.T) {
	cases := []struct {
		name    string
		headers map[string]string
	}{
		{"no headers", nil},
		{"origin and referer but no token", map[string]string{
			"Origin": "http://localhost:3000", "Referer": "http://localhost:3000/",
		}},
		{"referer but no token", map[string]string{"Referer": "http://localhost:3000/"}},
		{"forged token", map[string]string{
			"Referer": "http://localhost:3000/", "X-CSRF-Token": "not-a-real-token",
		}},
		{"cross-site referer", map[string]string{"Referer": "http://evil.example/"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, mutated := newCSRFTestRouter(t)

			w := httptest.NewRecorder()
			req := httptest.NewRequest("POST", "/thing", nil)
			for k, v := range tc.headers {
				req.Header.Set(k, v)
			}
			r.ServeHTTP(w, req)

			assert.Equal(t, http.StatusForbidden, w.Code)
			assert.False(t, *mutated, "a request answered with 403 must not mutate")
			assert.NotContains(t, w.Body.String(), `"type":"thing"`,
				"a rejected request must not have a success envelope appended to the 403")
		})
	}
}

func TestCSRFGuard_ValidTokenIsAllowed(t *testing.T) {
	r, mutated := newCSRFTestRouter(t)

	// Fetch a token and its cookie, the way the admin UI does.
	tokenRec := httptest.NewRecorder()
	tokenReq := httptest.NewRequest("GET", "/token", nil)
	r.ServeHTTP(tokenRec, tokenReq)
	token := tokenRec.Header().Get("X-CSRF-Token")
	assert.NotEmpty(t, token)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/thing", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	req.Header.Set("Referer", "http://localhost:3000/")
	req.Header.Set("X-CSRF-Token", token)
	for _, c := range tokenRec.Result().Cookies() {
		req.AddCookie(c)
	}
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
	assert.True(t, *mutated, "a properly tokenised write must go through")
}

func TestCSRFGuard_TokenAuthBypassesCSRF(t *testing.T) {
	// Token-authenticated calls are not browser-driven. A cross-site attacker
	// cannot set Authorization without a CORS preflight the server must approve.
	r, mutated := newCSRFTestRouter(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/thing", nil)
	req.Header.Set("Authorization", "Bearer some-api-key")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
	assert.True(t, *mutated)
}

func TestCSRFGuard_OAuthPathsBypassCSRF(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(csrfGuard(csrf.Protect([]byte("0123456789abcdef0123456789abcdef"), csrf.Secure(false))))

	reached := false
	r.POST("/oauth/token", func(c *gin.Context) {
		reached = true
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("POST", "/oauth/token", nil))

	assert.True(t, reached, "OAuth endpoints run their own state/PKCE checks")
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestDevCSRFTrustedOrigins(t *testing.T) {
	cases := []struct {
		name    string
		siteURL string
		extra   string
		want    []string
	}{
		{"nothing set keeps the default", "", "", []string{"localhost:3000"}},
		{"site url on another port is trusted", "http://localhost:3100", "",
			[]string{"localhost:3000", "localhost:3100"}},
		{"site url on the default port is not duplicated", "http://localhost:3000", "",
			[]string{"localhost:3000"}},
		{"site url with a path keeps only host:port", "https://studio.example.com/app/", "",
			[]string{"localhost:3000", "studio.example.com"}},
		{"extra origins are appended and trimmed", "http://localhost:3100", " 127.0.0.1:5173 ,, localhost:3100",
			[]string{"localhost:3000", "localhost:3100", "127.0.0.1:5173"}},
		{"unparseable site url is ignored", "://nope", "", []string{"localhost:3000"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, devCSRFTrustedOrigins(tc.siteURL, tc.extra))
		})
	}
}

// A browser on the SITE_URL port must be able to complete the token dance even
// though the proxied request Host (studio:8080) never matches its Origin.
func TestCSRFGuard_SiteURLPortIsTrusted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(csrfGuard(csrf.Protect(
		[]byte("0123456789abcdef0123456789abcdef"),
		csrf.Secure(false),
		csrf.Path("/"),
		csrf.TrustedOrigins(devCSRFTrustedOrigins("http://localhost:3100", "")),
	)))
	mutated := false
	r.POST("/thing", func(c *gin.Context) { mutated = true; c.Status(http.StatusCreated) })
	r.GET("/token", func(c *gin.Context) {
		c.Header("X-CSRF-Token", csrf.Token(c.Request))
		c.Status(http.StatusOK)
	})

	tokenRec := httptest.NewRecorder()
	tokenReq := httptest.NewRequest("GET", "/token", nil)
	tokenReq.Host = "studio:8080"
	r.ServeHTTP(tokenRec, tokenReq)

	for _, origin := range []string{"http://localhost:3100", "http://localhost:3000"} {
		mutated = false
		w := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/thing", nil)
		req.Host = "studio:8080"
		req.Header.Set("Origin", origin)
		req.Header.Set("X-CSRF-Token", tokenRec.Header().Get("X-CSRF-Token"))
		for _, c := range tokenRec.Result().Cookies() {
			req.AddCookie(c)
		}
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusCreated, w.Code, origin)
		assert.True(t, mutated, origin)
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/thing", nil)
	req.Host = "studio:8080"
	req.Header.Set("Origin", "http://localhost:4000")
	req.Header.Set("X-CSRF-Token", tokenRec.Header().Get("X-CSRF-Token"))
	for _, c := range tokenRec.Result().Cookies() {
		req.AddCookie(c)
	}
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code, "an untrusted port is still rejected")
}
