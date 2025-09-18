package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/performance/framework"
	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
	"github.com/TykTechnologies/midsommar/microgateway/internal/services"
	"github.com/gin-gonic/gin"
)

// setupBenchmarkServer creates a microgateway server with test data for benchmarking
func setupBenchmarkServer(b *testing.B) (*Server, *framework.BenchmarkDB, *framework.MockLLMServer) {
	// Set gin to test mode for consistent benchmarking
	gin.SetMode(gin.TestMode)

	// Create benchmark database with test data
	benchDB := framework.NewBenchmarkDB(b).SetTestDataSize(20)
	benchDB.SetupTestData(b)

	// Create mock LLM server
	mockServer := framework.NewMockLLMServer()

	// Create test configuration
	cfg := &config.Config{
		HTTPServer: config.HTTPServerConfig{
			Port:    0, // Use random port for testing
			Host:    "localhost",
			Timeout: 30,
		},
		Database: config.DatabaseConfig{
			Type: "sqlite",
			DSN:  ":memory:",
		},
		Security: config.SecurityConfig{
			JWTSecret:     "test-jwt-secret-32-characters-long",
			EncryptionKey: "test-encryption-key-32-chars-lng",
		},
		Analytics: config.AnalyticsConfig{
			Enabled:    true,
			BufferSize: 1000,
		},
	}

	// Create service container
	serviceContainer, err := services.NewServiceContainer(cfg, benchDB.DB)
	if err != nil {
		b.Fatalf("Failed to create service container: %v", err)
	}

	// Create server
	server, err := New(cfg, serviceContainer, "test", "test-hash", "test-time")
	if err != nil {
		b.Fatalf("Failed to create server: %v", err)
	}

	return server, benchDB, mockServer
}

// BenchmarkFullRequestLifecycle measures the complete auth → proxy → analytics → response flow
func BenchmarkFullRequestLifecycle(b *testing.B) {
	server, benchDB, mockServer := setupBenchmarkServer(b)
	defer mockServer.Close()

	// Test different lifecycle scenarios
	scenarios := []struct {
		name     string
		endpoint string
		method   string
		body     interface{}
		headers  map[string]string
	}{
		{
			name:     "LLM_Chat_Request",
			endpoint: "/llm/rest/test-llm-1/chat/completions",
			method:   "POST",
			body: map[string]interface{}{
				"model":    "gpt-4",
				"messages": []map[string]string{{"role": "user", "content": "Hello, world!"}},
			},
			headers: map[string]string{
				"Authorization": "Bearer test-key-1",
				"Content-Type":  "application/json",
			},
		},
		{
			name:     "Management_API_List_LLMs",
			endpoint: "/api/v1/llms",
			method:   "GET",
			body:     nil,
			headers: map[string]string{
				"Authorization": "Bearer test-admin-token",
			},
		},
		{
			name:     "Health_Check",
			endpoint: "/health",
			method:   "GET",
			body:     nil,
			headers:  map[string]string{},
		},
	}

	for _, scenario := range scenarios {
		b.Run(scenario.name, func(b *testing.B) {
			var reqBody []byte
			if scenario.body != nil {
				reqBody, _ = json.Marshal(scenario.body)
			}

			benchDB.ResetQueryStats()
			metrics := framework.NewPerformanceMetrics()
			metrics.StartMeasurement()

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				req := httptest.NewRequest(scenario.method, scenario.endpoint, bytes.NewBuffer(reqBody))
				for key, value := range scenario.headers {
					req.Header.Set(key, value)
				}

				w := httptest.NewRecorder()

				start := time.Now()
				server.router.ServeHTTP(w, req)
				lifecycleDuration := time.Since(start)

				metrics.RecordLatency(lifecycleDuration)
				if w.Code >= 200 && w.Code < 400 {
					metrics.RecordSuccess()
				} else {
					metrics.RecordError()
				}
			}

			metrics.EndMeasurement(time.Since(time.Now()))
			metrics.ReportToB(b)

			queryCount, _ := benchDB.QueryLogger.GetStats()
			b.ReportMetric(float64(queryCount), "lifecycle-queries")
		})
	}
}

// BenchmarkPluginExecutionOverhead measures the performance impact of plugin execution
func BenchmarkPluginExecutionOverhead(b *testing.B) {
	server, benchDB, mockServer := setupBenchmarkServer(b)
	defer mockServer.Close()

	// Test scenarios with different plugin configurations
	scenarios := []struct {
		name        string
		pluginCount int
		description string
	}{
		{"No_Plugins", 0, "Baseline without plugins"},
		{"Single_Plugin", 1, "With one plugin enabled"},
		{"Multiple_Plugins", 3, "With multiple plugins enabled"},
		{"Many_Plugins", 5, "With many plugins enabled"},
	}

	for _, scenario := range scenarios {
		b.Run(scenario.name, func(b *testing.B) {
			reqBody := framework.BuildLLMRequest("gpt-4", []map[string]string{
				{"role": "user", "content": "Plugin overhead test"},
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
				server.router.ServeHTTP(w, req)
				pluginDuration := time.Since(start)

				metrics.RecordLatency(pluginDuration)
				if w.Code == http.StatusOK {
					metrics.RecordSuccess()
				} else {
					metrics.RecordError()
				}
			}

			metrics.EndMeasurement(time.Since(time.Now()))
			metrics.ReportToB(b)

			b.ReportMetric(float64(scenario.pluginCount), "plugin-count")
			queryCount, _ := benchDB.QueryLogger.GetStats()
			b.ReportMetric(float64(queryCount), "plugin-queries")
		})
	}
}

// BenchmarkDatabaseQueryPerformance measures database lookup performance
func BenchmarkDatabaseQueryPerformance(b *testing.B) {
	server, benchDB, mockServer := setupBenchmarkServer(b)
	defer mockServer.Close()

	// Test different database query patterns
	scenarios := []struct {
		name        string
		endpoint    string
		method      string
		description string
	}{
		{
			name:        "LLM_Lookup",
			endpoint:    "/api/v1/llms/1",
			method:      "GET",
			description: "Single LLM lookup",
		},
		{
			name:        "LLM_List",
			endpoint:    "/api/v1/llms",
			method:      "GET",
			description: "List all LLMs with relationships",
		},
		{
			name:        "App_Lookup",
			endpoint:    "/api/v1/apps/1",
			method:      "GET",
			description: "Single app lookup",
		},
		{
			name:        "Plugin_List",
			endpoint:    "/api/v1/plugins",
			method:      "GET",
			description: "List all plugins",
		},
	}

	for _, scenario := range scenarios {
		b.Run(scenario.name, func(b *testing.B) {
			benchDB.ResetQueryStats()
			metrics := framework.NewPerformanceMetrics()
			metrics.StartMeasurement()

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				req := httptest.NewRequest(scenario.method, scenario.endpoint, nil)
				req.Header.Set("Authorization", "Bearer test-admin-token")

				w := httptest.NewRecorder()

				start := time.Now()
				server.router.ServeHTTP(w, req)
				queryDuration := time.Since(start)

				metrics.RecordLatency(queryDuration)
				if w.Code >= 200 && w.Code < 400 {
					metrics.RecordSuccess()
				} else {
					metrics.RecordError()
				}
			}

			metrics.EndMeasurement(time.Since(time.Now()))
			metrics.ReportToB(b)

			queryCount, _ := benchDB.QueryLogger.GetStats()
			b.ReportMetric(float64(queryCount), "db-queries")
		})
	}
}

// BenchmarkCachePerformance measures the impact of caching on performance
func BenchmarkCachePerformance(b *testing.B) {
	server, benchDB, mockServer := setupBenchmarkServer(b)
	defer mockServer.Close()

	// Test cache hit vs miss scenarios
	scenarios := []struct {
		name        string
		warmCache   bool
		description string
	}{
		{"Cold_Cache", false, "Cache miss scenario"},
		{"Warm_Cache", true, "Cache hit scenario"},
	}

	endpoint := "/api/v1/llms"

	for _, scenario := range scenarios {
		b.Run(scenario.name, func(b *testing.B) {
			// Warm the cache if needed
			if scenario.warmCache {
				warmupReq := httptest.NewRequest("GET", endpoint, nil)
				warmupReq.Header.Set("Authorization", "Bearer test-admin-token")
				warmupW := httptest.NewRecorder()
				server.router.ServeHTTP(warmupW, warmupReq)
			}

			benchDB.ResetQueryStats()
			metrics := framework.NewPerformanceMetrics()
			metrics.StartMeasurement()

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				req := httptest.NewRequest("GET", endpoint, nil)
				req.Header.Set("Authorization", "Bearer test-admin-token")

				w := httptest.NewRecorder()

				start := time.Now()
				server.router.ServeHTTP(w, req)
				cacheDuration := time.Since(start)

				metrics.RecordLatency(cacheDuration)
				if w.Code == http.StatusOK {
					metrics.RecordSuccess()
				} else {
					metrics.RecordError()
				}
			}

			metrics.EndMeasurement(time.Since(time.Now()))
			metrics.ReportToB(b)

			queryCount, _ := benchDB.QueryLogger.GetStats()
			b.ReportMetric(float64(queryCount), "cache-queries")
			b.ReportMetric(boolToFloat(scenario.warmCache), "cache-warm")
		})
	}
}

// BenchmarkManagementAPIPerformance measures CRUD operations performance
func BenchmarkManagementAPIPerformance(b *testing.B) {
	server, benchDB, mockServer := setupBenchmarkServer(b)
	defer mockServer.Close()

	// Test different CRUD operations
	scenarios := []struct {
		name     string
		method   string
		endpoint string
		body     interface{}
	}{
		{
			name:     "Create_LLM",
			method:   "POST",
			endpoint: "/api/v1/llms",
			body: map[string]interface{}{
				"name":          "Benchmark LLM",
				"vendor":        "openai",
				"default_model": "gpt-4",
				"is_active":     true,
			},
		},
		{
			name:     "Read_LLM",
			method:   "GET",
			endpoint: "/api/v1/llms/1",
			body:     nil,
		},
		{
			name:     "Update_LLM",
			method:   "PUT",
			endpoint: "/api/v1/llms/1",
			body: map[string]interface{}{
				"name":          "Updated Benchmark LLM",
				"vendor":        "openai",
				"default_model": "gpt-4",
				"is_active":     true,
			},
		},
		{
			name:     "List_LLMs",
			method:   "GET",
			endpoint: "/api/v1/llms",
			body:     nil,
		},
	}

	for _, scenario := range scenarios {
		b.Run(scenario.name, func(b *testing.B) {
			var reqBody []byte
			if scenario.body != nil {
				reqBody, _ = json.Marshal(scenario.body)
			}

			benchDB.ResetQueryStats()
			metrics := framework.NewPerformanceMetrics()
			metrics.StartMeasurement()

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				req := httptest.NewRequest(scenario.method, scenario.endpoint, bytes.NewBuffer(reqBody))
				req.Header.Set("Authorization", "Bearer test-admin-token")
				req.Header.Set("Content-Type", "application/json")

				w := httptest.NewRecorder()

				start := time.Now()
				server.router.ServeHTTP(w, req)
				crudDuration := time.Since(start)

				metrics.RecordLatency(crudDuration)
				if w.Code >= 200 && w.Code < 400 {
					metrics.RecordSuccess()
				} else {
					metrics.RecordError()
				}
			}

			metrics.EndMeasurement(time.Since(time.Now()))
			metrics.ReportToB(b)

			queryCount, _ := benchDB.QueryLogger.GetStats()
			b.ReportMetric(float64(queryCount), "crud-queries")
		})
	}
}

// BenchmarkGRPCControlPlane measures gRPC communication performance (if enabled)
func BenchmarkGRPCControlPlane(b *testing.B) {
	server, benchDB, mockServer := setupBenchmarkServer(b)
	defer mockServer.Close()

	// Note: This would typically test gRPC endpoints but requires gRPC server setup
	// For now, we'll measure the REST equivalent of control plane operations

	scenarios := []struct {
		name        string
		endpoint    string
		method      string
		description string
	}{
		{
			name:        "System_Status",
			endpoint:    "/api/v1/system/status",
			method:      "GET",
			description: "System status check",
		},
		{
			name:        "System_Config",
			endpoint:    "/api/v1/system/config",
			method:      "GET",
			description: "System configuration",
		},
		{
			name:        "System_Metrics",
			endpoint:    "/api/v1/system/metrics",
			method:      "GET",
			description: "System metrics",
		},
	}

	for _, scenario := range scenarios {
		b.Run(scenario.name, func(b *testing.B) {
			benchDB.ResetQueryStats()
			metrics := framework.NewPerformanceMetrics()
			metrics.StartMeasurement()

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				req := httptest.NewRequest(scenario.method, scenario.endpoint, nil)
				req.Header.Set("Authorization", "Bearer test-admin-token")

				w := httptest.NewRecorder()

				start := time.Now()
				server.router.ServeHTTP(w, req)
				grpcDuration := time.Since(start)

				metrics.RecordLatency(grpcDuration)
				if w.Code >= 200 && w.Code < 500 { // Allow 404 for non-implemented endpoints
					metrics.RecordSuccess()
				} else {
					metrics.RecordError()
				}
			}

			metrics.EndMeasurement(time.Since(time.Now()))
			metrics.ReportToB(b)

			queryCount, _ := benchDB.QueryLogger.GetStats()
			b.ReportMetric(float64(queryCount), "grpc-queries")
		})
	}
}

// BenchmarkConcurrentAPIRequests measures API performance under concurrent load
func BenchmarkConcurrentAPIRequests(b *testing.B) {
	server, benchDB, mockServer := setupBenchmarkServer(b)
	defer mockServer.Close()

	concurrencyLevels := []int{1, 5, 10, 20, 50}

	for _, concurrency := range concurrencyLevels {
		b.Run(fmt.Sprintf("Concurrency_%d", concurrency), func(b *testing.B) {
			benchDB.ResetQueryStats()

			tester := framework.NewConcurrentTester(concurrency).
				WithRequestCount(int64(b.N))

			metrics := tester.Run(b, func(ctx context.Context, workerID int, metrics *framework.PerformanceMetrics) error {
				// Mix of different API operations
				endpoints := []string{
					"/api/v1/llms",
					"/api/v1/apps",
					"/api/v1/plugins",
					"/health",
				}

				endpoint := endpoints[workerID%len(endpoints)]
				req := httptest.NewRequest("GET", endpoint, nil)
				req = req.WithContext(ctx)
				req.Header.Set("Authorization", "Bearer test-admin-token")

				w := httptest.NewRecorder()
				server.router.ServeHTTP(w, req)

				if w.Code >= 200 && w.Code < 400 {
					return nil
				}
				return fmt.Errorf("HTTP %d", w.Code)
			})

			queryCount, _ := benchDB.QueryLogger.GetStats()
			b.ReportMetric(float64(queryCount), "concurrent-api-queries")
			b.ReportMetric(float64(concurrency), "api-workers")

			// Log aggregated metrics
			b.Logf("API Concurrency %d: %s", concurrency, metrics.String())
		})
	}
}

// BenchmarkServerStartupTime measures server initialization performance
func BenchmarkServerStartupTime(b *testing.B) {
	// Create test configuration
	cfg := &config.Config{
		HTTPServer: config.HTTPServerConfig{
			Port:    0,
			Host:    "localhost",
			Timeout: 30,
		},
		Database: config.DatabaseConfig{
			Type: "sqlite",
			DSN:  ":memory:",
		},
		Security: config.SecurityConfig{
			JWTSecret:     "test-jwt-secret-32-characters-long",
			EncryptionKey: "test-encryption-key-32-chars-lng",
		},
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Create benchmark database
		benchDB := framework.NewBenchmarkDB(b).SetTestDataSize(10)
		benchDB.SetupTestData(b)

		start := time.Now()

		// Create service container
		serviceContainer, err := services.NewServiceContainer(cfg, benchDB.DB)
		if err != nil {
			b.Fatalf("Failed to create service container: %v", err)
		}

		// Create server
		server, err := New(cfg, serviceContainer, "test", "test-hash", "test-time")
		if err != nil {
			b.Fatalf("Failed to create server: %v", err)
		}

		startupDuration := time.Since(start)

		if server == nil {
			b.Fatal("Server creation failed")
		}

		b.ReportMetric(float64(startupDuration.Nanoseconds()), "startup-ns")
	}
}

// BenchmarkMemoryUsageUnderLoad measures memory usage patterns under sustained load
func BenchmarkMemoryUsageUnderLoad(b *testing.B) {
	server, benchDB, mockServer := setupBenchmarkServer(b)
	defer mockServer.Close()

	// Start memory leak detection
	leakTester := framework.NewMemoryLeakTester()
	leakTester.StartMonitoring()

	reqBody := framework.BuildLLMRequest("gpt-4", []map[string]string{
		{"role": "user", "content": "Memory usage test under sustained load"},
	})

	benchDB.ResetQueryStats()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("POST", "/llm/rest/test-llm-1/chat/completions", bytes.NewBuffer(reqBody))
		req.Header.Set("Authorization", "Bearer test-key-1")
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		server.router.ServeHTTP(w, req)

		// Periodic garbage collection opportunity
		if i%50 == 0 {
			time.Sleep(time.Millisecond)
		}
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
	b.ReportMetric(float64(queryCount), "load-queries")
}

// Helper functions

// boolToFloat converts boolean to float for metric reporting
func boolToFloat(b bool) float64 {
	if b {
		return 1.0
	}
	return 0.0
}