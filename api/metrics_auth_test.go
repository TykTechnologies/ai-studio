package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func dummyMetricsHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("# HELP test_metric\ntest_metric 1\n"))
	})
}

func performMetricsRequest(router *gin.Engine, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", "/metrics", nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestRegisterMetricsEndpoint_DisabledByDefault(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// No auth token and no explicit unauthenticated opt-in: endpoint must not be registered
	registered := registerMetricsEndpoint(router, "/metrics", dummyMetricsHandler(), "", false)
	assert.False(t, registered)

	rec := performMetricsRequest(router, "")
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestRegisterMetricsEndpoint_WithAuthToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	registered := registerMetricsEndpoint(router, "/metrics", dummyMetricsHandler(), "s3cret-token", false)
	assert.True(t, registered)

	t.Run("no token returns 401", func(t *testing.T) {
		rec := performMetricsRequest(router, "")
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
		assert.NotContains(t, rec.Body.String(), "test_metric")
	})

	t.Run("wrong token returns 401", func(t *testing.T) {
		rec := performMetricsRequest(router, "wrong-token")
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
		assert.NotContains(t, rec.Body.String(), "test_metric")
	})

	t.Run("malformed authorization header returns 401", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/metrics", nil)
		req.Header.Set("Authorization", "s3cret-token") // missing "Bearer " prefix
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("correct token returns metrics", func(t *testing.T) {
		rec := performMetricsRequest(router, "s3cret-token")
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), "test_metric")
	})
}

func TestRegisterMetricsEndpoint_ExplicitUnauthenticated(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	registered := registerMetricsEndpoint(router, "/metrics", dummyMetricsHandler(), "", true)
	assert.True(t, registered)

	rec := performMetricsRequest(router, "")
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "test_metric")
}

func TestRegisterMetricsEndpoint_TokenTakesPrecedenceOverUnauthenticated(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// If a token is configured, auth is enforced even if the unauthenticated flag is set
	registered := registerMetricsEndpoint(router, "/metrics", dummyMetricsHandler(), "s3cret-token", true)
	assert.True(t, registered)

	rec := performMetricsRequest(router, "")
	assert.Equal(t, http.StatusUnauthorized, rec.Code)

	rec = performMetricsRequest(router, "s3cret-token")
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestRegisterMetricsEndpoint_NilHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	registered := registerMetricsEndpoint(router, "/metrics", nil, "s3cret-token", true)
	assert.False(t, registered)

	rec := performMetricsRequest(router, "s3cret-token")
	assert.Equal(t, http.StatusNotFound, rec.Code)
}
