package authz

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// stubAuthorizer returns configurable check results for testing middleware.
type stubAuthorizer struct {
	NoopAuthorizer
	enabled  bool
	allowed  bool
	checkErr error
}

func (s *stubAuthorizer) Enabled() bool { return s.enabled }
func (s *stubAuthorizer) Check(_ context.Context, _ uint, _ string, _ string, _ uint) (bool, error) {
	return s.allowed, s.checkErr
}
func (s *stubAuthorizer) CheckByName(_ context.Context, _ uint, _ string, _ string, _ string) (bool, error) {
	return s.allowed, s.checkErr
}

func testUserID(uid uint) UserIDFromContext {
	return func(c *gin.Context) (uint, bool) { return uid, true }
}

func noUserID() UserIDFromContext {
	return func(c *gin.Context) (uint, bool) { return 0, false }
}

func TestRequireRelation_Allowed(t *testing.T) {
	auth := &stubAuthorizer{enabled: true, allowed: true}
	handler := RequireRelation(auth, "llm", "can_use", "id", testUserID(10))

	w := httptest.NewRecorder()
	c, r := gin.CreateTestContext(w)
	r.GET("/llm/:id", handler, func(c *gin.Context) { c.Status(http.StatusOK) })
	c.Request = httptest.NewRequest("GET", "/llm/5", nil)
	r.ServeHTTP(w, c.Request)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRequireRelation_Denied(t *testing.T) {
	auth := &stubAuthorizer{enabled: true, allowed: false}
	handler := RequireRelation(auth, "llm", "can_use", "id", testUserID(10))

	w := httptest.NewRecorder()
	c, r := gin.CreateTestContext(w)
	r.GET("/llm/:id", handler, func(c *gin.Context) { c.Status(http.StatusOK) })
	c.Request = httptest.NewRequest("GET", "/llm/5", nil)
	r.ServeHTTP(w, c.Request)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestRequireRelation_NoUser(t *testing.T) {
	auth := &stubAuthorizer{enabled: true, allowed: true}
	handler := RequireRelation(auth, "llm", "can_use", "id", noUserID())

	w := httptest.NewRecorder()
	c, r := gin.CreateTestContext(w)
	r.GET("/llm/:id", handler, func(c *gin.Context) { c.Status(http.StatusOK) })
	c.Request = httptest.NewRequest("GET", "/llm/5", nil)
	r.ServeHTTP(w, c.Request)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestRequireRelation_InvalidID(t *testing.T) {
	auth := &stubAuthorizer{enabled: true, allowed: true}
	handler := RequireRelation(auth, "llm", "can_use", "id", testUserID(10))

	w := httptest.NewRecorder()
	c, r := gin.CreateTestContext(w)
	r.GET("/llm/:id", handler, func(c *gin.Context) { c.Status(http.StatusOK) })
	c.Request = httptest.NewRequest("GET", "/llm/abc", nil)
	r.ServeHTTP(w, c.Request)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestRequireRelation_CheckError(t *testing.T) {
	auth := &stubAuthorizer{enabled: true, checkErr: fmt.Errorf("backend down")}
	handler := RequireRelation(auth, "llm", "can_use", "id", testUserID(10))

	w := httptest.NewRecorder()
	c, r := gin.CreateTestContext(w)
	r.GET("/llm/:id", handler, func(c *gin.Context) { c.Status(http.StatusOK) })
	c.Request = httptest.NewRequest("GET", "/llm/5", nil)
	r.ServeHTTP(w, c.Request)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestRequireRelationByName_Allowed(t *testing.T) {
	auth := &stubAuthorizer{enabled: true, allowed: true}
	handler := RequireRelationByName(auth, "system", "admin", "1", testUserID(10))

	w := httptest.NewRecorder()
	c, r := gin.CreateTestContext(w)
	r.GET("/admin", handler, func(c *gin.Context) { c.Status(http.StatusOK) })
	c.Request = httptest.NewRequest("GET", "/admin", nil)
	r.ServeHTTP(w, c.Request)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRequireRelationByName_Denied(t *testing.T) {
	auth := &stubAuthorizer{enabled: true, allowed: false}
	handler := RequireRelationByName(auth, "system", "admin", "1", testUserID(10))

	w := httptest.NewRecorder()
	c, r := gin.CreateTestContext(w)
	r.GET("/admin", handler, func(c *gin.Context) { c.Status(http.StatusOK) })
	c.Request = httptest.NewRequest("GET", "/admin", nil)
	r.ServeHTTP(w, c.Request)

	assert.Equal(t, http.StatusForbidden, w.Code)
}
