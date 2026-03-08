package authz

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestShadowCheck_NeverBlocks(t *testing.T) {
	// Shadow mode should never block, even when authz denies.
	auth := &stubAuthorizer{enabled: true, allowed: false}
	handler := ShadowCheck(auth, "system", "admin", "1", testUserID(10), func(c *gin.Context) bool {
		return true // legacy allows
	})

	w := httptest.NewRecorder()
	_, r := gin.CreateTestContext(w)
	r.GET("/test", handler, func(c *gin.Context) { c.Status(http.StatusOK) })
	r.ServeHTTP(w, httptest.NewRequest("GET", "/test", nil))

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestShadowCheck_SkipsWhenDisabled(t *testing.T) {
	auth := &stubAuthorizer{enabled: false}
	called := false
	handler := ShadowCheck(auth, "system", "admin", "1", testUserID(10), func(c *gin.Context) bool {
		called = true
		return true
	})

	w := httptest.NewRecorder()
	_, r := gin.CreateTestContext(w)
	r.GET("/test", handler, func(c *gin.Context) { c.Status(http.StatusOK) })
	r.ServeHTTP(w, httptest.NewRequest("GET", "/test", nil))

	assert.Equal(t, http.StatusOK, w.Code)
	assert.False(t, called, "legacy check should not be called when authz is disabled")
}

func TestShadowCheck_SkipsWhenNoUser(t *testing.T) {
	auth := &stubAuthorizer{enabled: true, allowed: true}
	called := false
	handler := ShadowCheck(auth, "system", "admin", "1", noUserID(), func(c *gin.Context) bool {
		called = true
		return true
	})

	w := httptest.NewRecorder()
	_, r := gin.CreateTestContext(w)
	r.GET("/test", handler, func(c *gin.Context) { c.Status(http.StatusOK) })
	r.ServeHTTP(w, httptest.NewRequest("GET", "/test", nil))

	assert.Equal(t, http.StatusOK, w.Code)
	assert.False(t, called)
}

func TestShadowCheckResource_NeverBlocks(t *testing.T) {
	auth := &stubAuthorizer{enabled: true, allowed: false}
	handler := ShadowCheckResource(auth, "llm", "can_use", "id", testUserID(10), func(c *gin.Context) bool {
		return true
	})

	w := httptest.NewRecorder()
	_, r := gin.CreateTestContext(w)
	r.GET("/llm/:id", handler, func(c *gin.Context) { c.Status(http.StatusOK) })
	r.ServeHTTP(w, httptest.NewRequest("GET", "/llm/5", nil))

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestShadowCheckOwnership_NeverBlocks(t *testing.T) {
	auth := &stubAuthorizer{enabled: true, allowed: false}
	handler := ShadowCheckOwnership(auth, "app", "can_use", "id", testUserID(10))

	w := httptest.NewRecorder()
	_, r := gin.CreateTestContext(w)
	r.GET("/app/:id", handler, func(c *gin.Context) {
		c.Status(http.StatusForbidden) // legacy denies
	})
	r.ServeHTTP(w, httptest.NewRequest("GET", "/app/5", nil))

	// Shadow should not change the response.
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestShadowCheckOwnership_StoresResult(t *testing.T) {
	auth := &stubAuthorizer{enabled: true, allowed: true}
	handler := ShadowCheckOwnership(auth, "app", "can_use", "id", testUserID(10))

	var shadowAllowed interface{}
	w := httptest.NewRecorder()
	_, r := gin.CreateTestContext(w)
	r.GET("/app/:id", handler, func(c *gin.Context) {
		shadowAllowed, _ = c.Get("authz_shadow_allowed")
		c.Status(http.StatusOK)
	})
	r.ServeHTTP(w, httptest.NewRequest("GET", "/app/5", nil))

	assert.Equal(t, true, shadowAllowed)
}
