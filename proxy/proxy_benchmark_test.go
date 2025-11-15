package proxy

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/performance/framework"
	"github.com/TykTechnologies/midsommar/v2/services"
)

// setupBenchmarkProxy creates a proxy with test data for benchmarking
func setupBenchmarkProxy(b *testing.B) (*Proxy, *framework.BenchmarkDB, *framework.MockLLMServer) {
	// Create benchmark database with test data
	benchDB := framework.NewBenchmarkDB(b).SetTestDataSize(10)
	benchDB.SetupTestData(b)

	// Create services
	service := services.NewService(benchDB.GetDB())
	budgetService := budget.NewService(benchDB.DB, nil)

	// Create mock LLM server
	mockServer := framework.NewMockLLMServer()

	// Create proxy
	config := &Config{Port: 0} // Use port 0 for testing
	proxy := New(service, budgetService, config)

	return proxy, benchDB, mockServer
}

// BenchmarkProxyRequestRouting measures the overhead of request routing and path parsing
func BenchmarkProxyRequestRouting(b *testing.B) {
	proxy, benchDB, mockServer := setupBenchmarkProxy(b)
	defer mockServer.Close()

	// Test various routing patterns
	scenarios := []struct {
		name    string
		path    string
		method  string
		headers map[string]string
	}{
		{
			name:   "LLM_REST_Chat",
			path:   "/llm/rest/test-llm-1/chat/completions",
			method: "POST",
			headers: map[string]string{
				"Authorization": "Bearer test-key-1",
				"Content-Type":  "application/json",
			},
		},
		{
			name:   "LLM_Stream_Chat",
			path:   "/llm/stream/test-llm-1/chat/completions",
			method: "POST",
			headers: map[string]string{
				"Authorization": "Bearer test-key-1",
				"Content-Type":  "application/json",
				"Accept":        "text/event-stream",
			},
		},
		{
			name:   "Health_Check",
			path:   "/.well-known/oauth-protected-resource",
			method: "GET",
			headers: map[string]string{},
		},
		{
			name:   "Tool_Operation",
			path:   "/tools/test-tool/operation",
			method: "POST",
			headers: map[string]string{
				"Authorization": "Bearer test-key-1",
				"Content-Type":  "application/json",
			},
		},
	}

	for _, scenario := range scenarios {
		b.Run(scenario.name, func(b *testing.B) {
			benchDB.ResetQueryStats()

			// Create test request
			reqBody := framework.BuildLLMRequest("gpt-4", []map[string]string{
				{"role": "user", "content": "Hello, world!"},
			})

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				req := httptest.NewRequest(scenario.method, scenario.path, bytes.NewBuffer(reqBody))
				for key, value := range scenario.headers {
					req.Header.Set(key, value)
				}

				w := httptest.NewRecorder()

				// This measures the routing overhead
				start := time.Now()
				proxy.createHandler().ServeHTTP(w, req)
				routingDuration := time.Since(start)

				b.ReportMetric(float64(routingDuration.Nanoseconds()), "routing-ns")
			}

			queryCount, _ := benchDB.QueryLogger.GetStats()
			b.ReportMetric(float64(queryCount), "queries")
		})
	}
}

// BenchmarkAuthenticationOverhead measures the performance impact of authentication
func BenchmarkAuthenticationOverhead(b *testing.B) {
	proxy, benchDB, mockServer := setupBenchmarkProxy(b)
	defer mockServer.Close()

	authScenarios := []struct {
		name    string
		token   string
		valid   bool
		desc    string
	}{
		{"Valid_Token", "test-key-1", true, "Valid authentication token"},
		{"Invalid_Token", "invalid-token", false, "Invalid authentication token"},
		{"Missing_Token", "", false, "Missing authentication token"},
		{"Malformed_Token", "Bearer malformed", false, "Malformed Bearer token"},
	}

	reqBody := framework.BuildLLMRequest("gpt-4", []map[string]string{
		{"role": "user", "content": "Hello, world!"},
	})

	for _, scenario := range authScenarios {
		b.Run(scenario.name, func(b *testing.B) {
			benchDB.ResetQueryStats()
			metrics := framework.NewPerformanceMetrics()
			metrics.StartMeasurement()

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				req := httptest.NewRequest("POST", "/llm/rest/test-llm-1/chat/completions", bytes.NewBuffer(reqBody))
				req.Header.Set("Content-Type", "application/json")
				if scenario.token != "" {
					req.Header.Set("Authorization", "Bearer "+scenario.token)
				}

				w := httptest.NewRecorder()

				start := time.Now()
				proxy.createHandler().ServeHTTP(w, req)
				authDuration := time.Since(start)

				metrics.RecordLatency(authDuration)
				if scenario.valid && w.Code == http.StatusOK {
					metrics.RecordSuccess()
				} else {
					metrics.RecordError()
				}
			}

			metrics.EndMeasurement(time.Since(time.Now()))
			metrics.ReportToB(b)

			queryCount, _ := benchDB.QueryLogger.GetStats()
			b.ReportMetric(float64(queryCount), "auth-queries")
		})
	}
}

// BenchmarkVendorTranslation measures vendor-specific request/response translation performance
func BenchmarkVendorTranslation(b *testing.B) {
	proxy, benchDB, mockServer := setupBenchmarkProxy(b)
	defer mockServer.Close()

	// Test different vendor translations
	vendors := []struct {
		name   string
		vendor string
		model  string
	}{
		{"OpenAI", "openai", "gpt-4"},
		{"Anthropic", "anthropic", "claude-3-haiku"},
		{"Google", "google", "gemini-pro"},
		{"Vertex", "vertex", "gemini-1.0-pro"},
	}

	for _, vendor := range vendors {
		b.Run(vendor.name, func(b *testing.B) {
			// Create request specific to vendor format
			reqBody := framework.BuildLLMRequest(vendor.model, []map[string]string{
				{"role": "user", "content": "Translate this request to " + vendor.vendor + " format"},
			})

			benchDB.ResetQueryStats()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				req := httptest.NewRequest("POST", "/llm/rest/test-llm-1/chat/completions", bytes.NewBuffer(reqBody))
				req.Header.Set("Authorization", "Bearer test-key-1")
				req.Header.Set("Content-Type", "application/json")

				w := httptest.NewRecorder()

				// Measure translation overhead
				start := time.Now()
				proxy.createHandler().ServeHTTP(w, req)
				translationDuration := time.Since(start)

				b.ReportMetric(float64(translationDuration.Nanoseconds()), "translation-ns")
			}

			queryCount, _ := benchDB.QueryLogger.GetStats()
			b.ReportMetric(float64(queryCount), "vendor-queries")
		})
	}
}

// BenchmarkStreamingVsREST compares streaming and REST request performance
func BenchmarkStreamingVsREST(b *testing.B) {
	proxy, benchDB, mockServer := setupBenchmarkProxy(b)
	defer mockServer.Close()

	modes := []struct {
		name      string
		path      string
		headers   map[string]string
		streaming bool
	}{
		{
			name: "REST",
			path: "/llm/rest/test-llm-1/chat/completions",
			headers: map[string]string{
				"Content-Type": "application/json",
			},
			streaming: false,
		},
		{
			name: "Streaming",
			path: "/llm/stream/test-llm-1/chat/completions",
			headers: map[string]string{
				"Content-Type": "application/json",
				"Accept":       "text/event-stream",
			},
			streaming: true,
		},
	}

	for _, mode := range modes {
		b.Run(mode.name, func(b *testing.B) {
			var reqBody []byte
			if mode.streaming {
				reqBody = framework.BuildStreamingLLMRequest("gpt-4", []map[string]string{
					{"role": "user", "content": "Stream this response"},
				})
			} else {
				reqBody = framework.BuildLLMRequest("gpt-4", []map[string]string{
					{"role": "user", "content": "Return this response"},
				})
			}

			benchDB.ResetQueryStats()
			metrics := framework.NewPerformanceMetrics()
			metrics.StartMeasurement()

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				req := httptest.NewRequest("POST", mode.path, bytes.NewBuffer(reqBody))
				req.Header.Set("Authorization", "Bearer test-key-1")
				for key, value := range mode.headers {
					req.Header.Set(key, value)
				}

				w := httptest.NewRecorder()

				start := time.Now()
				proxy.createHandler().ServeHTTP(w, req)
				requestDuration := time.Since(start)

				metrics.RecordLatency(requestDuration)
				if w.Code == http.StatusOK {
					metrics.RecordSuccess()
				} else {
					metrics.RecordError()
				}
			}

			metrics.EndMeasurement(time.Since(time.Now()))
			metrics.ReportToB(b)

			queryCount, _ := benchDB.QueryLogger.GetStats()
			b.ReportMetric(float64(queryCount), "mode-queries")
		})
	}
}

// BenchmarkResponseCapture measures the overhead of response capture and modification
func BenchmarkResponseCapture(b *testing.B) {
	proxy, benchDB, mockServer := setupBenchmarkProxy(b)
	defer mockServer.Close()

	// Test different response sizes
	responseSizes := []struct {
		name    string
		content string
		size    int
	}{
		{"Small", "Hello", 5},
		{"Medium", strings.Repeat("Hello, this is a medium response. ", 10), 340},
		{"Large", strings.Repeat("This is a large response with lots of content. ", 100), 4700},
		{"ExtraLarge", strings.Repeat("This is an extra large response with extensive content for testing. ", 1000), 67000},
	}

	for _, responseSize := range responseSizes {
		b.Run(responseSize.name, func(b *testing.B) {
			reqBody := framework.BuildLLMRequest("gpt-4", []map[string]string{
				{"role": "user", "content": responseSize.content},
			})

			benchDB.ResetQueryStats()
			metrics := framework.NewPerformanceMetrics()
			metrics.StartMeasurement()

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				req := httptest.NewRequest("POST", "/llm/rest/test-llm-1/chat/completions", bytes.NewBuffer(reqBody))
				req.Header.Set("Authorization", "Bearer test-key-1")
				req.Header.Set("Content-Type", "application/json")

				w := httptest.NewRecorder()

				start := time.Now()
				proxy.createHandler().ServeHTTP(w, req)
				captureDuration := time.Since(start)

				metrics.RecordLatency(captureDuration)
				if w.Code == http.StatusOK {
					metrics.RecordSuccess()
				} else {
					metrics.RecordError()
				}

				// Record response size for analysis
				responseSize := len(w.Body.Bytes())
				metrics.RecordCustomMetric("response-size", float64(responseSize))
			}

			metrics.EndMeasurement(time.Since(time.Now()))
			metrics.ReportToB(b)

			b.ReportMetric(float64(responseSize.size), "expected-size")
		})
	}
}

// BenchmarkErrorHandling measures performance of different error scenarios
func BenchmarkErrorHandling(b *testing.B) {
	proxy, benchDB, mockServer := setupBenchmarkProxy(b)
	defer mockServer.Close()

	errorScenarios := []struct {
		name     string
		path     string
		token    string
		body     []byte
		headers  map[string]string
		expected int
	}{
		{
			name:     "Invalid_Path",
			path:     "/invalid/path",
			token:    "test-key-1",
			body:     framework.BuildLLMRequest("gpt-4", []map[string]string{{"role": "user", "content": "test"}}),
			headers:  map[string]string{"Content-Type": "application/json"},
			expected: 404,
		},
		{
			name:     "Unauthorized",
			path:     "/llm/rest/test-llm-1/chat/completions",
			token:    "",
			body:     framework.BuildLLMRequest("gpt-4", []map[string]string{{"role": "user", "content": "test"}}),
			headers:  map[string]string{"Content-Type": "application/json"},
			expected: 401,
		},
		{
			name:     "Invalid_JSON",
			path:     "/llm/rest/test-llm-1/chat/completions",
			token:    "test-key-1",
			body:     []byte(`{"invalid": json`),
			headers:  map[string]string{"Content-Type": "application/json"},
			expected: 400,
		},
		{
			name:     "Missing_Content_Type",
			path:     "/llm/rest/test-llm-1/chat/completions",
			token:    "test-key-1",
			body:     framework.BuildLLMRequest("gpt-4", []map[string]string{{"role": "user", "content": "test"}}),
			headers:  map[string]string{},
			expected: 400,
		},
	}

	for _, scenario := range errorScenarios {
		b.Run(scenario.name, func(b *testing.B) {
			benchDB.ResetQueryStats()
			metrics := framework.NewPerformanceMetrics()
			metrics.StartMeasurement()

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				req := httptest.NewRequest("POST", scenario.path, bytes.NewBuffer(scenario.body))
				if scenario.token != "" {
					req.Header.Set("Authorization", "Bearer "+scenario.token)
				}
				for key, value := range scenario.headers {
					req.Header.Set(key, value)
				}

				w := httptest.NewRecorder()

				start := time.Now()
				proxy.createHandler().ServeHTTP(w, req)
				errorDuration := time.Since(start)

				metrics.RecordLatency(errorDuration)
				if w.Code == scenario.expected {
					metrics.RecordSuccess()
				} else {
					metrics.RecordError()
				}
			}

			metrics.EndMeasurement(time.Since(time.Now()))
			metrics.ReportToB(b)

			b.ReportMetric(float64(scenario.expected), "expected-status")
		})
	}
}

// BenchmarkConcurrentRequests measures proxy performance under concurrent load
func BenchmarkConcurrentRequests(b *testing.B) {
	proxy, benchDB, mockServer := setupBenchmarkProxy(b)
	defer mockServer.Close()

	concurrencyLevels := []int{1, 10, 50, 100}

	for _, concurrency := range concurrencyLevels {
		b.Run(fmt.Sprintf("Concurrency_%d", concurrency), func(b *testing.B) {
			benchDB.ResetQueryStats()

			tester := framework.NewConcurrentTester(concurrency).
				WithRequestCount(int64(b.N))

			metrics := tester.Run(b, func(ctx context.Context, workerID int, metrics *framework.PerformanceMetrics) error {
				reqBody := framework.BuildLLMRequest("gpt-4", []map[string]string{
					{"role": "user", "content": fmt.Sprintf("Request from worker %d", workerID)},
				})

				req := httptest.NewRequest("POST", "/llm/rest/test-llm-1/chat/completions", bytes.NewBuffer(reqBody))
				req.Header.Set("Authorization", "Bearer test-key-1")
				req.Header.Set("Content-Type", "application/json")

				w := httptest.NewRecorder()
				proxy.createHandler().ServeHTTP(w, req)

				if w.Code == http.StatusOK {
					return nil
				}
				return fmt.Errorf("HTTP %d", w.Code)
			})

			queryCount, _ := benchDB.QueryLogger.GetStats()
			b.ReportMetric(float64(queryCount), "total-queries")
			b.ReportMetric(float64(concurrency), "workers")

			// Log the aggregated metrics
			b.Logf("Concurrency %d: %s", concurrency, metrics.String())
		})
	}
}

// BenchmarkMemoryAllocation measures memory allocation patterns
func BenchmarkMemoryAllocation(b *testing.B) {
	proxy, benchDB, mockServer := setupBenchmarkProxy(b)
	defer mockServer.Close()

	reqBody := framework.BuildLLMRequest("gpt-4", []map[string]string{
		{"role": "user", "content": "Memory allocation test"},
	})

	benchDB.ResetQueryStats()

	// Run a memory leak tester
	leakTester := framework.NewMemoryLeakTester()
	leakTester.StartMonitoring()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("POST", "/llm/rest/test-llm-1/chat/completions", bytes.NewBuffer(reqBody))
		req.Header.Set("Authorization", "Bearer test-key-1")
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		proxy.createHandler().ServeHTTP(w, req)
	}

	// Check for memory leaks
	hasLeak, leakDesc := leakTester.StopMonitoring()
	if hasLeak {
		b.Logf("Memory leak detected: %s", leakDesc)
		b.ReportMetric(1, "memory-leak")
	} else {
		b.ReportMetric(0, "memory-leak")
	}

	queryCount, _ := benchDB.QueryLogger.GetStats()
	b.ReportMetric(float64(queryCount), "allocation-queries")
}

// BenchmarkProxyReload measures the performance impact of configuration reloading
func BenchmarkProxyReload(b *testing.B) {
	proxy, benchDB, mockServer := setupBenchmarkProxy(b)
	defer mockServer.Close()

	// Ensure proxy is initially loaded
	err := proxy.loadResources()
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		start := time.Now()
		err := proxy.Reload()
		reloadDuration := time.Since(start)

		if err != nil {
			b.Errorf("Reload failed: %v", err)
		}

		b.ReportMetric(float64(reloadDuration.Nanoseconds()), "reload-ns")
	}

	queryCount, _ := benchDB.QueryLogger.GetStats()
	b.ReportMetric(float64(queryCount), "reload-queries")
}

// BenchmarkEndToEndLatency measures complete request-response latency
func BenchmarkEndToEndLatency(b *testing.B) {
	proxy, benchDB, mockServer := setupBenchmarkProxy(b)
	defer mockServer.Close()

	reqBody := framework.BuildLLMRequest("gpt-4", []map[string]string{
		{"role": "user", "content": "End-to-end latency test"},
	})

	benchDB.ResetQueryStats()
	metrics := framework.NewPerformanceMetrics()
	metrics.StartMeasurement()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("POST", "/llm/rest/test-llm-1/chat/completions", bytes.NewBuffer(reqBody))
		req.Header.Set("Authorization", "Bearer test-key-1")
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()

		start := time.Now()
		proxy.createHandler().ServeHTTP(w, req)
		e2eDuration := time.Since(start)

		metrics.RecordLatency(e2eDuration)
		if w.Code == http.StatusOK {
			metrics.RecordSuccess()
		} else {
			metrics.RecordError()
		}
	}

	metrics.EndMeasurement(time.Since(time.Now()))
	metrics.ReportToB(b)

	queryCount, _ := benchDB.QueryLogger.GetStats()
	b.ReportMetric(float64(queryCount), "e2e-queries")

	// Report mock server statistics
	serverStats := mockServer.GetRequestStats()
	b.ReportMetric(float64(serverStats.TotalRequests), "mock-requests")
	b.ReportMetric(float64(serverStats.AvgResponseTime.Nanoseconds()), "mock-avg-response-ns")
}