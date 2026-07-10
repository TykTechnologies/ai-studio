package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/metrics"
	"github.com/TykTechnologies/midsommar/v2/pkg/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func metricsTestRequest(router *gin.Engine, bearerToken string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", "/metrics", nil)
	if bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+bearerToken)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// registerTestMetricsRoute mirrors the metrics registration in SetupRouter so
// tests can exercise it without constructing a full router config.
func registerTestMetricsRoute(router gin.IRoutes, config *RouterConfig) bool {
	metricsPath := config.MetricsPath
	if metricsPath == "" {
		metricsPath = "/metrics"
	}
	return middleware.RegisterMetricsEndpoint(router, middleware.MetricsEndpointConfig{
		Path:                 metricsPath,
		Handler:              config.MetricsHandler,
		AuthToken:            config.MetricsAuthToken,
		AllowUnauthenticated: config.MetricsAllowUnauthenticated,
	})
}

func TestMetricsEndpoint_Enabled(t *testing.T) {
	metricsHandler := metrics.Init()
	require.NotNil(t, metricsHandler)

	gin.SetMode(gin.TestMode)
	router := gin.New()

	// Explicitly opted in to unauthenticated scraping
	registered := registerTestMetricsRoute(router, &RouterConfig{
		MetricsHandler:              metricsHandler,
		MetricsAllowUnauthenticated: true,
	})
	require.True(t, registered)

	// Record a metric so we have something to scrape
	metrics.RecordRequest(context.Background(), "1", "openai", "gpt-4", 200)

	rec := metricsTestRequest(router, "")
	assert.Equal(t, http.StatusOK, rec.Code)

	body, err := io.ReadAll(rec.Body)
	require.NoError(t, err)
	bodyStr := string(body)

	// Should contain Prometheus exposition format
	assert.True(t, strings.Contains(bodyStr, "aistudio_llm_requests_total"),
		"response should contain aistudio_llm_requests_total metric")
	assert.True(t, strings.Contains(bodyStr, "# HELP") || strings.Contains(bodyStr, "# TYPE"),
		"response should contain Prometheus metadata comments")
}

func TestMetricsEndpoint_Disabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// When EnableMetrics is false, /metrics is never registered
	// (SetupRouter skips registerMetricsRoute entirely).

	rec := metricsTestRequest(router, "")
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestMetricsEndpoint_NilHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// When MetricsHandler is nil, the route should not be mounted
	registered := registerTestMetricsRoute(router, &RouterConfig{
		MetricsHandler:              nil,
		MetricsAuthToken:            "s3cret-token",
		MetricsAllowUnauthenticated: true,
	})
	assert.False(t, registered)

	rec := metricsTestRequest(router, "s3cret-token")
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestMetricsEndpoint_SecureByDefault(t *testing.T) {
	metricsHandler := metrics.Init()
	require.NotNil(t, metricsHandler)

	gin.SetMode(gin.TestMode)
	router := gin.New()

	// Metrics enabled but no auth token and no explicit unauthenticated opt-in:
	// the endpoint must not be registered at all.
	registered := registerTestMetricsRoute(router, &RouterConfig{
		MetricsHandler: metricsHandler,
	})
	assert.False(t, registered)

	rec := metricsTestRequest(router, "")
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestMetricsEndpoint_AuthToken(t *testing.T) {
	metricsHandler := metrics.Init()
	require.NotNil(t, metricsHandler)

	gin.SetMode(gin.TestMode)
	router := gin.New()

	registered := registerTestMetricsRoute(router, &RouterConfig{
		MetricsHandler:   metricsHandler,
		MetricsAuthToken: "s3cret-token",
	})
	require.True(t, registered)

	t.Run("no token returns 401", func(t *testing.T) {
		rec := metricsTestRequest(router, "")
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
		assert.NotContains(t, rec.Body.String(), "# HELP")
	})

	t.Run("wrong token returns 401", func(t *testing.T) {
		rec := metricsTestRequest(router, "wrong-token")
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
		assert.NotContains(t, rec.Body.String(), "# HELP")
	})

	t.Run("malformed authorization header returns 401", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/metrics", nil)
		req.Header.Set("Authorization", "s3cret-token") // missing "Bearer " prefix
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("correct token returns metrics", func(t *testing.T) {
		rec := metricsTestRequest(router, "s3cret-token")
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("token takes precedence over unauthenticated flag", func(t *testing.T) {
		r2 := gin.New()
		registered := registerTestMetricsRoute(r2, &RouterConfig{
			MetricsHandler:              metricsHandler,
			MetricsAuthToken:            "s3cret-token",
			MetricsAllowUnauthenticated: true,
		})
		require.True(t, registered)

		rec := metricsTestRequest(r2, "")
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})
}

func TestMetricsEndpoint_ContentType(t *testing.T) {
	metricsHandler := metrics.Init()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	registered := registerTestMetricsRoute(router, &RouterConfig{
		MetricsHandler:              metricsHandler,
		MetricsAllowUnauthenticated: true,
	})
	require.True(t, registered)

	rec := metricsTestRequest(router, "")
	assert.Equal(t, http.StatusOK, rec.Code)
	contentType := rec.Header().Get("Content-Type")
	assert.True(t, strings.Contains(contentType, "text/plain") || strings.Contains(contentType, "application/openmetrics"),
		"Content-Type should be text/plain or openmetrics, got: %s", contentType)
}
