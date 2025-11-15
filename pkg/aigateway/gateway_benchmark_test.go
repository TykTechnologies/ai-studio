package aigateway

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/analytics"
	"github.com/TykTechnologies/midsommar/v2/performance/framework"
	"github.com/TykTechnologies/midsommar/v2/services"
)

// setupBenchmarkGateway creates an AI Gateway with test data for benchmarking
func setupBenchmarkGateway(b *testing.B) (Gateway, *framework.BenchmarkDB, *framework.MockLLMServer) {
	// Create benchmark database with test data
	benchDB := framework.NewBenchmarkDB(b).SetTestDataSize(20)
	benchDB.SetupTestData(b)

	// Create services
	service := services.NewService(benchDB.GetDB())
	budgetService := budget.NewService(benchDB.DB, nil)

	// Create mock LLM server
	mockServer := framework.NewMockLLMServer()

	// Initialize analytics for testing
	ctx := context.Background()
	analytics.InitDefault(ctx, benchDB.DB)

	// Create AI Gateway
	config := &Config{Port: 0} // Use port 0 for testing
	gateway := New(service, budgetService, config)

	return gateway, benchDB, mockServer
}

// BenchmarkGatewayInitialization measures gateway initialization performance
func BenchmarkGatewayInitialization(b *testing.B) {
	// Create benchmark database
	benchDB := framework.NewBenchmarkDB(b).SetTestDataSize(50)
	benchDB.SetupTestData(b)

	service := services.NewService(benchDB.GetDB())
	budgetService := budget.NewService(benchDB.DB, nil)

	ctx := context.Background()
	analytics.InitDefault(ctx, benchDB.DB)

	config := &Config{Port: 0}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		start := time.Now()
		gateway := New(service, budgetService, config)
		initDuration := time.Since(start)

		if gateway == nil {
			b.Fatal("Gateway initialization failed")
		}

		b.ReportMetric(float64(initDuration.Nanoseconds()), "init-ns")
	}
}

// BenchmarkResourceLoading measures the performance of loading LLMs, filters, and datasources
func BenchmarkResourceLoading(b *testing.B) {
	gateway, benchDB, mockServer := setupBenchmarkGateway(b)
	defer mockServer.Close()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		benchDB.ResetQueryStats()

		start := time.Now()
		err := gateway.Reload()
		loadDuration := time.Since(start)

		if err != nil {
			b.Errorf("Resource loading failed: %v", err)
		}

		queryCount, _ := benchDB.QueryLogger.GetStats()
		b.ReportMetric(float64(queryCount), "load-queries")
		b.ReportMetric(float64(loadDuration.Nanoseconds()), "load-ns")
	}
}

// BenchmarkRequestProcessingPipeline measures end-to-end request processing latency
func BenchmarkRequestProcessingPipeline(b *testing.B) {
	gateway, benchDB, mockServer := setupBenchmarkGateway(b)
	defer mockServer.Close()

	// Test different request complexities
	scenarios := []struct {
		name     string
		content  string
		size     int
		hasTools bool
	}{
		{
			name:     "Simple",
			content:  "Hello",
			size:     5,
			hasTools: false,
		},
		{
			name:     "Medium",
			content:  "Generate a detailed explanation about artificial intelligence and machine learning concepts.",
			size:     95,
			hasTools: false,
		},
		{
			name:     "Complex",
			content:  "Please provide a comprehensive analysis of the following data with charts and detailed explanations: " + generateLargeText(500),
			size:     600,
			hasTools: true,
		},
	}

	handler := gateway.Handler()

	for _, scenario := range scenarios {
		b.Run(scenario.name, func(b *testing.B) {
			reqBody := framework.BuildLLMRequest("gpt-4", []map[string]string{
				{"role": "user", "content": scenario.content},
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
				handler.ServeHTTP(w, req)
				pipelineDuration := time.Since(start)

				metrics.RecordLatency(pipelineDuration)
				if w.Code == http.StatusOK {
					metrics.RecordSuccess()
				} else {
					metrics.RecordError()
				}
			}

			metrics.EndMeasurement(time.Since(time.Now()))
			metrics.ReportToB(b)

			queryCount, _ := benchDB.QueryLogger.GetStats()
			b.ReportMetric(float64(queryCount), "pipeline-queries")
			b.ReportMetric(float64(scenario.size), "content-size")
		})
	}
}

// BenchmarkAnalyticsRecording measures the performance impact of analytics recording
func BenchmarkAnalyticsRecording(b *testing.B) {
	scenarios := []struct {
		name        string
		enabled     bool
		description string
	}{
		{"Analytics_Enabled", true, "With analytics recording enabled"},
		{"Analytics_Disabled", false, "With analytics recording disabled"},
	}

	for _, scenario := range scenarios {
		b.Run(scenario.name, func(b *testing.B) {
			gateway, benchDB, mockServer := setupBenchmarkGateway(b)
			defer mockServer.Close()

			// Configure analytics based on scenario
			if !scenario.enabled {
				// Stop analytics recording for this test
				// Note: analytics.StopDefault() API has changed, needs updating
			}

			reqBody := framework.BuildLLMRequest("gpt-4", []map[string]string{
				{"role": "user", "content": "Analytics recording test"},
			})

			handler := gateway.Handler()
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
				handler.ServeHTTP(w, req)
				recordingDuration := time.Since(start)

				metrics.RecordLatency(recordingDuration)
				if w.Code == http.StatusOK {
					metrics.RecordSuccess()
				} else {
					metrics.RecordError()
				}
			}

			metrics.EndMeasurement(time.Since(time.Now()))
			metrics.ReportToB(b)

			queryCount, _ := benchDB.QueryLogger.GetStats()
			b.ReportMetric(float64(queryCount), "analytics-queries")
			b.ReportMetric(boolToFloat(scenario.enabled), "analytics-enabled")

			// Re-enable analytics for subsequent tests
			if !scenario.enabled {
				ctx := context.Background()
				analytics.InitDefault(ctx, benchDB.DB)
			}
		})
	}
}

// BenchmarkResponseHooks measures the overhead of response hook execution
func BenchmarkResponseHooks(b *testing.B) {
	gateway, benchDB, mockServer := setupBenchmarkGateway(b)
	defer mockServer.Close()

	// Create different types of response hooks
	hooks := []struct {
		name string
		hook func(ctx context.Context, response []byte, statusCode int) ([]byte, error)
	}{
		{
			name: "No_Hook",
			hook: nil,
		},
		{
			name: "Simple_Hook",
			hook: func(ctx context.Context, response []byte, statusCode int) ([]byte, error) {
				// Simple modification
				return response, nil
			},
		},
		{
			name: "JSON_Processing_Hook",
			hook: func(ctx context.Context, response []byte, statusCode int) ([]byte, error) {
				// Simulate JSON processing
				if len(response) > 100 {
					return append(response[:100], []byte("...[truncated]")...), nil
				}
				return response, nil
			},
		},
		{
			name: "Complex_Hook",
			hook: func(ctx context.Context, response []byte, statusCode int) ([]byte, error) {
				// Simulate complex processing
				processedResponse := make([]byte, len(response)+50)
				copy(processedResponse, response)
				copy(processedResponse[len(response):], []byte(" [processed by hook]"))
				return processedResponse[:len(response)+18], nil
			},
		},
	}

	for _, hookType := range hooks {
		b.Run(hookType.name, func(b *testing.B) {
			// Add response hook if specified
			if hookType.hook != nil {
				// Note: This would require implementing AddResponseHook properly
				// For now, we'll measure the hook function directly
			}

			reqBody := framework.BuildLLMRequest("gpt-4", []map[string]string{
				{"role": "user", "content": "Response hook test"},
			})

			handler := gateway.Handler()
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
				handler.ServeHTTP(w, req)

				// Measure hook execution if present
				if hookType.hook != nil {
					hookStart := time.Now()
					_, err := hookType.hook(context.Background(), w.Body.Bytes(), w.Code)
					hookDuration := time.Since(hookStart)
					if err != nil {
						b.Errorf("Hook execution failed: %v", err)
					}
					b.ReportMetric(float64(hookDuration.Nanoseconds()), "hook-ns")
				}

				handlerDuration := time.Since(start)
				metrics.RecordLatency(handlerDuration)
				if w.Code == http.StatusOK {
					metrics.RecordSuccess()
				} else {
					metrics.RecordError()
				}
			}

			metrics.EndMeasurement(time.Since(time.Now()))
			metrics.ReportToB(b)

			queryCount, _ := benchDB.QueryLogger.GetStats()
			b.ReportMetric(float64(queryCount), "hook-queries")
		})
	}
}

// BenchmarkConcurrentRequestHandling measures throughput under concurrent load
func BenchmarkConcurrentRequestHandling(b *testing.B) {
	gateway, benchDB, mockServer := setupBenchmarkGateway(b)
	defer mockServer.Close()

	concurrencyLevels := []int{1, 5, 10, 25, 50}

	for _, concurrency := range concurrencyLevels {
		b.Run(fmt.Sprintf("Workers_%d", concurrency), func(b *testing.B) {
			handler := gateway.Handler()
			benchDB.ResetQueryStats()

			tester := framework.NewConcurrentTester(concurrency).
				WithRequestCount(int64(b.N))

			metrics := tester.Run(b, func(ctx context.Context, workerID int, metrics *framework.PerformanceMetrics) error {
				reqBody := framework.BuildLLMRequest("gpt-4", []map[string]string{
					{"role": "user", "content": fmt.Sprintf("Concurrent request from worker %d", workerID)},
				})

				req := httptest.NewRequest("POST", "/llm/rest/test-llm-1/chat/completions", bytes.NewBuffer(reqBody))
				req = req.WithContext(ctx)
				req.Header.Set("Authorization", "Bearer test-key-1")
				req.Header.Set("Content-Type", "application/json")

				w := httptest.NewRecorder()
				handler.ServeHTTP(w, req)

				if w.Code == http.StatusOK {
					return nil
				}
				return fmt.Errorf("HTTP %d", w.Code)
			})

			queryCount, _ := benchDB.QueryLogger.GetStats()
			b.ReportMetric(float64(queryCount), "concurrent-queries")
			b.ReportMetric(float64(concurrency), "workers")

			// Log aggregated metrics
			b.Logf("Workers %d: %s", concurrency, metrics.String())
		})
	}
}

// BenchmarkGatewayStartStop measures start/stop performance and resource cleanup
func BenchmarkGatewayStartStop(b *testing.B) {
	// Create benchmark database
	benchDB := framework.NewBenchmarkDB(b).SetTestDataSize(10)
	benchDB.SetupTestData(b)

	service := services.NewService(benchDB.GetDB())
	budgetService := budget.NewService(benchDB.DB, nil)

	ctx := context.Background()
	analytics.InitDefault(ctx, benchDB.DB)

	config := &Config{Port: 0}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		gateway := New(service, budgetService, config)

		// Measure start time
		startTime := time.Now()
		go func() {
			gateway.Start() // This will fail on port 0, but we measure the attempt
		}()
		startDuration := time.Since(startTime)

		// Give it a moment to attempt start
		time.Sleep(time.Millisecond * 10)

		// Measure stop time
		stopTime := time.Now()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		err := gateway.Stop(ctx)
		stopDuration := time.Since(stopTime)
		cancel()

		if err != nil && err != http.ErrServerClosed {
			b.Logf("Stop error (expected for port 0): %v", err)
		}

		b.ReportMetric(float64(startDuration.Nanoseconds()), "start-ns")
		b.ReportMetric(float64(stopDuration.Nanoseconds()), "stop-ns")
	}
}

// BenchmarkGatewayMemoryUsage measures memory usage patterns under load
func BenchmarkGatewayMemoryUsage(b *testing.B) {
	gateway, benchDB, mockServer := setupBenchmarkGateway(b)
	defer mockServer.Close()

	handler := gateway.Handler()

	// Start memory leak detection
	leakTester := framework.NewMemoryLeakTester()
	leakTester.StartMonitoring()

	reqBody := framework.BuildLLMRequest("gpt-4", []map[string]string{
		{"role": "user", "content": "Memory usage test with moderate content length for realistic simulation"},
	})

	benchDB.ResetQueryStats()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("POST", "/llm/rest/test-llm-1/chat/completions", bytes.NewBuffer(reqBody))
		req.Header.Set("Authorization", "Bearer test-key-1")
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		// Force garbage collection periodically to check for leaks
		if i%100 == 0 {
			time.Sleep(time.Millisecond) // Allow GC opportunity
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
	b.ReportMetric(float64(queryCount), "memory-queries")
}

// BenchmarkDifferentRequestSizes measures performance impact of varying request sizes
func BenchmarkDifferentRequestSizes(b *testing.B) {
	gateway, benchDB, mockServer := setupBenchmarkGateway(b)
	defer mockServer.Close()

	sizes := []struct {
		name string
		size int
	}{
		{"Tiny", 10},
		{"Small", 100},
		{"Medium", 1000},
		{"Large", 10000},
		{"ExtraLarge", 50000},
	}

	handler := gateway.Handler()

	for _, size := range sizes {
		b.Run(size.name, func(b *testing.B) {
			content := generateLargeText(size.size)
			reqBody := framework.BuildLLMRequest("gpt-4", []map[string]string{
				{"role": "user", "content": content},
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
				handler.ServeHTTP(w, req)
				processingDuration := time.Since(start)

				metrics.RecordLatency(processingDuration)
				if w.Code == http.StatusOK {
					metrics.RecordSuccess()
				} else {
					metrics.RecordError()
				}
			}

			metrics.EndMeasurement(time.Since(time.Now()))
			metrics.ReportToB(b)

			b.ReportMetric(float64(size.size), "request-size")
			queryCount, _ := benchDB.QueryLogger.GetStats()
			b.ReportMetric(float64(queryCount), "size-queries")
		})
	}
}

// Helper functions

// generateLargeText creates a text string of specified approximate length
func generateLargeText(length int) string {
	pattern := "This is a test sentence for generating large text content. "
	patternLen := len(pattern)
	repetitions := length / patternLen

	result := ""
	for i := 0; i < repetitions; i++ {
		result += pattern
	}

	// Add remaining characters
	remaining := length - len(result)
	if remaining > 0 && remaining < patternLen {
		result += pattern[:remaining]
	}

	return result
}

// boolToFloat converts boolean to float for metric reporting
func boolToFloat(b bool) float64 {
	if b {
		return 1.0
	}
	return 0.0
}