package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/metrics"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMetricsEndpoint_Enabled(t *testing.T) {
	metricsHandler := metrics.Init()
	require.NotNil(t, metricsHandler)

	gin.SetMode(gin.TestMode)
	router := gin.New()

	// Simulate what SetupRouter does when metrics are enabled
	router.GET("/metrics", gin.WrapH(metricsHandler))

	// Record a metric so we have something to scrape
	metrics.RecordRequest(context.Background(), "1", "openai", "gpt-4", 200)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/metrics", nil)
	router.ServeHTTP(rec, req)

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

	// When EnableMetrics is false or MetricsHandler is nil, /metrics should not be registered.
	// We do NOT register the route, simulating config.EnableMetrics = false.

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/metrics", nil)
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestMetricsEndpoint_NilHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// When MetricsHandler is nil, the route should not be mounted (matches router.go logic)
	var nilHandler http.Handler
	enableMetrics := true
	if enableMetrics && nilHandler != nil {
		router.GET("/metrics", gin.WrapH(nilHandler))
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/metrics", nil)
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestMetricsEndpoint_ContentType(t *testing.T) {
	metricsHandler := metrics.Init()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/metrics", gin.WrapH(metricsHandler))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/metrics", nil)
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	contentType := rec.Header().Get("Content-Type")
	assert.True(t, strings.Contains(contentType, "text/plain") || strings.Contains(contentType, "application/openmetrics"),
		"Content-Type should be text/plain or openmetrics, got: %s", contentType)
}
