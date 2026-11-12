package auth_test

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/auth"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/postgres"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	gormlogger "github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm/logger"
)

// unreachableDB is a Postgres database nothing answers for: every
// statement fails to connect, as during a failover.
func unreachableDB(t *testing.T) *gorm.DB {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := l.Addr().String()
	require.NoError(t, l.Close())
	db, err := gorm.Open(postgres.Open("postgres://u:p@"+addr+"/db?sslmode=disable&connect_timeout=2"),
		&gorm.Config{DisableAutomaticPing: true, Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
	require.NoError(t, err)
	t.Cleanup(func() {
		if pool, err := db.DB(); err == nil {
			pool.Close()
		}
	})
	return db
}

func assertDatabaseUnavailable(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	require.Equal(t, http.StatusServiceUnavailable, w.Code, w.Body.String())
	assert.NotEmpty(t, w.Header().Get("Retry-After"))
	var body struct {
		Errors []struct {
			Code string `json:"code"`
		} `json:"errors"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Len(t, body.Errors, 1)
	assert.Equal(t, "database_unavailable", body.Errors[0].Code)
}

// While the database cannot be used, a login and a request with a session
// cookie or an API key are answered 503 database_unavailable, not 401: the
// credentials were not wrong, they could not be checked.
func TestDatabaseUnavailableIsNot401(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := unreachableDB(t)
	svc := services.NewService(db)
	a := auth.NewAuthService(&auth.Config{DB: db, Service: svc, CookieName: "session"}, newMockMailService(), svc, nil)

	r := gin.New()
	r.POST("/auth/login", func(c *gin.Context) {
		if err := a.Login(c, "someone@example.com", "a-password"); err != nil {
			return
		}
		c.Status(http.StatusOK)
	})
	r.GET("/api/v1/me", a.AuthMiddleware(), func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader("{}")))
	assertDatabaseUnavailable(t, w)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: "a-session-token"})
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assertDatabaseUnavailable(t, w)

	req = httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	req.Header.Set("Authorization", "Bearer an-api-key")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assertDatabaseUnavailable(t, w)
}

// With the database up, unknown credentials are still a 401.
func TestUnknownCredentialsAreStill401(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(t)
	svc := services.NewService(db)
	a := auth.NewAuthService(&auth.Config{DB: db, Service: svc, CookieName: "session"}, newMockMailService(), svc, nil)
	r := gin.New()
	r.GET("/api/v1/me", a.AuthMiddleware(), func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: "no-such-session"})
	req.Header.Set("Authorization", "Bearer no-such-key")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}
