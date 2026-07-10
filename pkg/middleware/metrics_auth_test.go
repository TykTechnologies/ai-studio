package middleware

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
	var warned string
	registered := RegisterMetricsEndpoint(router, MetricsEndpointConfig{
		Path:    "/metrics",
		Handler: dummyMetricsHandler(),
		Warn:    func(msg string) { warned = msg },
	})
	assert.False(t, registered)
	assert.Contains(t, warned, "METRICS_AUTH_TOKEN")

	rec := performMetricsRequest(router, "")
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestRegisterMetricsEndpoint_WithAuthToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	registered := RegisterMetricsEndpoint(router, MetricsEndpointConfig{
		Path:      "/metrics",
		Handler:   dummyMetricsHandler(),
		AuthToken: "s3cret-token",
	})
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

	registered := RegisterMetricsEndpoint(router, MetricsEndpointConfig{
		Path:                 "/metrics",
		Handler:              dummyMetricsHandler(),
		AllowUnauthenticated: true,
	})
	assert.True(t, registered)

	rec := performMetricsRequest(router, "")
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "test_metric")
}

func TestRegisterMetricsEndpoint_TokenTakesPrecedenceOverUnauthenticated(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// If a token is configured, auth is enforced even if the unauthenticated flag is set
	registered := RegisterMetricsEndpoint(router, MetricsEndpointConfig{
		Path:                 "/metrics",
		Handler:              dummyMetricsHandler(),
		AuthToken:            "s3cret-token",
		AllowUnauthenticated: true,
	})
	assert.True(t, registered)

	rec := performMetricsRequest(router, "")
	assert.Equal(t, http.StatusUnauthorized, rec.Code)

	rec = performMetricsRequest(router, "s3cret-token")
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestRegisterMetricsEndpoint_CustomPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	registered := RegisterMetricsEndpoint(router, MetricsEndpointConfig{
		Path:                 "/internal/metrics",
		Handler:              dummyMetricsHandler(),
		AllowUnauthenticated: true,
	})
	assert.True(t, registered)

	rec := performMetricsRequest(router, "")
	assert.Equal(t, http.StatusNotFound, rec.Code, "default /metrics path should not be mounted")

	req := httptest.NewRequest("GET", "/internal/metrics", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "test_metric")
}

func TestRegisterMetricsEndpoint_NilHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	registered := RegisterMetricsEndpoint(router, MetricsEndpointConfig{
		Path:                 "/metrics",
		AuthToken:            "s3cret-token",
		AllowUnauthenticated: true,
	})
	assert.False(t, registered)

	rec := performMetricsRequest(router, "s3cret-token")
	assert.Equal(t, http.StatusNotFound, rec.Code)
}
