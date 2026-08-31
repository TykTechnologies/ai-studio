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
	r.GET("/token", func(c *gin.Context) {
		c.Header("X-CSRF-Token", csrf.Token(c.Request))
		c.Status(http.StatusOK)
	})

	return r, &mutated
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
