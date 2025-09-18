package performance

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/analytics"
	"github.com/TykTechnologies/midsommar/v2/performance/framework"
	"github.com/TykTechnologies/midsommar/v2/proxy"
	"github.com/TykTechnologies/midsommar/v2/services"
)

// setupLoadTestProxy creates a proxy for load testing
func setupLoadTestProxy(b *testing.B) (*proxy.Proxy, *framework.BenchmarkDB, *framework.MockLLMServer) {
	// Create benchmark database with more test data for load testing
	benchDB := framework.NewBenchmarkDB(b).SetTestDataSize(50)
	benchDB.SetupTestData(b)

	// Create services
	service := services.NewService(benchDB.GetDB())
	budgetService := services.NewBudgetService(benchDB.DB, nil)

	// Initialize analytics
	ctx := context.Background()
	analytics.InitDefault(ctx, benchDB.DB)

	// Create mock LLM server
	mockServer := framework.NewMockLLMServer()

	// Create proxy
	config := &proxy.Config{Port: 0}
	proxyInstance := proxy.New(service, budgetService, config)

	return proxyInstance, benchDB, mockServer
}

// BenchmarkSustainedLoad tests sustained RPS over time
func BenchmarkSustainedLoad(b *testing.B) {
	proxy, benchDB, mockServer := setupLoadTestProxy(b)
	defer mockServer.Close()

	// Test different sustained load levels
	loadLevels := []struct {
		name      string
		targetRPS float64
		duration  time.Duration
	}{
		{"Load_100_RPS", 100, time.Second * 30},
		{"Load_500_RPS", 500, time.Second * 30},
		{"Load_1000_RPS", 1000, time.Second * 30},
		{"Load_2000_RPS", 2000, time.Second * 15}, // Shorter duration for higher load
	}

	for _, load := range loadLevels {
		b.Run(load.name, func(b *testing.B) {
			benchDB.ResetQueryStats()

			tester := framework.NewLoadTester(framework.ConstantLoad).
				WithBaseRPS(load.targetRPS).
				WithDuration(load.duration)

			metrics := tester.Execute(b, func(ctx context.Context, workerID int, metrics *framework.PerformanceMetrics) error {
				reqBody := framework.BuildLLMRequest("gpt-4", []map[string]string{
					{"role": "user", "content": fmt.Sprintf("Sustained load test %d", workerID)},
				})

				req := httptest.NewRequest("POST", "/llm/rest/test-llm-1/chat/completions", bytes.NewBuffer(reqBody))
				req = req.WithContext(ctx)
				req.Header.Set("Authorization", "Bearer test-key-1")
				req.Header.Set("Content-Type", "application/json")

				w := httptest.NewRecorder()
				proxy.Handler().ServeHTTP(w, req)

				if w.Code == http.StatusOK {
					return nil
				}
				return fmt.Errorf("HTTP %d", w.Code)
			})

			queryCount, _ := benchDB.QueryLogger.GetStats()
			b.ReportMetric(float64(queryCount), "sustained-queries")
			b.ReportMetric(load.targetRPS, "target-rps")
			b.ReportMetric(float64(load.duration.Seconds()), "duration-sec")

			// Verify we achieved target performance
			actualRPS := metrics.RequestsPerSec
			if actualRPS < load.targetRPS*0.8 { // Allow 20% tolerance
				b.Logf("Warning: Achieved RPS (%.2f) significantly below target (%.2f)", actualRPS, load.targetRPS)
			}

			b.Logf("Sustained Load %s: %s", load.name, metrics.String())
		})
	}
}

// BenchmarkBurstLoad tests handling of traffic spikes
func BenchmarkBurstLoad(b *testing.B) {
	proxy, benchDB, mockServer := setupLoadTestProxy(b)
	defer mockServer.Close()

	// Test different burst patterns
	burstPatterns := []struct {
		name     string
		baseRPS  float64
		burstRPS float64
		duration time.Duration
	}{
		{"Burst_100_to_1000", 100, 1000, time.Second * 20},
		{"Burst_500_to_2000", 500, 2000, time.Second * 15},
		{"Burst_1000_to_5000", 1000, 5000, time.Second * 10},
	}

	for _, pattern := range burstPatterns {
		b.Run(pattern.name, func(b *testing.B) {
			benchDB.ResetQueryStats()

			tester := framework.NewLoadTester(framework.SpikeLoad).
				WithBaseRPS(pattern.baseRPS).
				WithMaxRPS(pattern.burstRPS).
				WithDuration(pattern.duration)

			metrics := tester.Execute(b, func(ctx context.Context, workerID int, metrics *framework.PerformanceMetrics) error {
				reqBody := framework.BuildLLMRequest("gpt-4", []map[string]string{
					{"role": "user", "content": fmt.Sprintf("Burst load test %d", workerID)},
				})

				req := httptest.NewRequest("POST", "/llm/rest/test-llm-1/chat/completions", bytes.NewBuffer(reqBody))
				req = req.WithContext(ctx)
				req.Header.Set("Authorization", "Bearer test-key-1")
				req.Header.Set("Content-Type", "application/json")

				w := httptest.NewRecorder()
				proxy.Handler().ServeHTTP(w, req)

				if w.Code == http.StatusOK {
					return nil
				}
				return fmt.Errorf("HTTP %d", w.Code)
			})

			queryCount, _ := benchDB.QueryLogger.GetStats()
			b.ReportMetric(float64(queryCount), "burst-queries")
			b.ReportMetric(pattern.baseRPS, "base-rps")
			b.ReportMetric(pattern.burstRPS, "burst-rps")

			b.Logf("Burst Load %s: %s", pattern.name, metrics.String())
		})
	}
}

// BenchmarkMemoryLeakDetection runs extended tests to detect memory leaks
func BenchmarkMemoryLeakDetection(b *testing.B) {
	proxy, benchDB, mockServer := setupLoadTestProxy(b)
	defer mockServer.Close()

	// Test for different durations to detect leaks
	durations := []struct {
		name     string
		duration time.Duration
		rps      float64
	}{
		{"Short_5min", time.Minute * 5, 100},
		{"Medium_10min", time.Minute * 10, 50},
		{"Long_15min", time.Minute * 15, 25},
	}

	for _, duration := range durations {
		b.Run(duration.name, func(b *testing.B) {
			benchDB.ResetQueryStats()

			// Start memory monitoring
			leakTester := framework.NewMemoryLeakTester()
			leakTester.StartMonitoring()

			tester := framework.NewLoadTester(framework.ConstantLoad).
				WithBaseRPS(duration.rps).
				WithDuration(duration.duration)

			var requestCount int64

			metrics := tester.Execute(b, func(ctx context.Context, workerID int, metrics *framework.PerformanceMetrics) error {
				reqBody := framework.BuildLLMRequest("gpt-4", []map[string]string{
					{"role": "user", "content": fmt.Sprintf("Memory leak test %d", atomic.AddInt64(&requestCount, 1))},
				})

				req := httptest.NewRequest("POST", "/llm/rest/test-llm-1/chat/completions", bytes.NewBuffer(reqBody))
				req = req.WithContext(ctx)
				req.Header.Set("Authorization", "Bearer test-key-1")
				req.Header.Set("Content-Type", "application/json")

				w := httptest.NewRecorder()
				proxy.Handler().ServeHTTP(w, req)

				if w.Code == http.StatusOK {
					return nil
				}
				return fmt.Errorf("HTTP %d", w.Code)
			})

			// Check for memory leaks
			hasLeak, leakDesc := leakTester.StopMonitoring()
			if hasLeak {
				b.Errorf("Memory leak detected in %s: %s", duration.name, leakDesc)
				b.ReportMetric(1, "memory-leak")
			} else {
				b.ReportMetric(0, "memory-leak")
			}

			queryCount, _ := benchDB.QueryLogger.GetStats()
			b.ReportMetric(float64(queryCount), "leak-queries")
			b.ReportMetric(float64(duration.duration.Minutes()), "test-minutes")
			b.ReportMetric(float64(requestCount), "total-requests")

			b.Logf("Memory Test %s: %s", duration.name, metrics.String())
		})
	}
}

// BenchmarkConnectionPoolExhaustion tests behavior when connection pool is exhausted
func BenchmarkConnectionPoolExhaustion(b *testing.B) {
	proxy, benchDB, mockServer := setupLoadTestProxy(b)
	defer mockServer.Close()

	// Test connection pool exhaustion scenarios
	scenarios := []struct {
		name        string
		concurrency int
		duration    time.Duration
	}{
		{"Pool_Low_Pressure", 10, time.Second * 15},
		{"Pool_High_Pressure", 50, time.Second * 15},
		{"Pool_Exhaustion", 100, time.Second * 10},
	}

	for _, scenario := range scenarios {
		b.Run(scenario.name, func(b *testing.B) {
			benchDB.ResetQueryStats()

			tester := framework.NewConnectionPoolTester(25). // Assume 25 max connections
									WithConnectionDelay(time.Millisecond * 10).
									WithHoldDuration(time.Millisecond * 100)

			metrics := tester.TestExhaustion(b,
				func() error {
					// Simulate acquiring a database connection
					reqBody := framework.BuildLLMRequest("gpt-4", []map[string]string{
						{"role": "user", "content": "Connection pool test"},
					})

					req := httptest.NewRequest("POST", "/llm/rest/test-llm-1/chat/completions", bytes.NewBuffer(reqBody))
					req.Header.Set("Authorization", "Bearer test-key-1")
					req.Header.Set("Content-Type", "application/json")

					w := httptest.NewRecorder()
					proxy.Handler().ServeHTTP(w, req)

					if w.Code >= 500 {
						return fmt.Errorf("Server error: %d", w.Code)
					}
					return nil
				},
				func() {
					// Simulate releasing connection (no-op in this case)
				},
			)

			queryCount, _ := benchDB.QueryLogger.GetStats()
			b.ReportMetric(float64(queryCount), "pool-queries")
			b.ReportMetric(float64(scenario.concurrency), "concurrency")

			b.Logf("Connection Pool %s: %s", scenario.name, metrics.String())
		})
	}
}

// BenchmarkGracefulDegradation tests system behavior under overload
func BenchmarkGracefulDegradation(b *testing.B) {
	proxy, benchDB, mockServer := setupLoadTestProxy(b)
	defer mockServer.Close()

	// Test overload scenarios
	overloadLevels := []struct {
		name     string
		targetRPS float64
		duration time.Duration
	}{
		{"Normal_Load", 500, time.Second * 10},
		{"High_Load", 2000, time.Second * 10},
		{"Overload", 5000, time.Second * 10},
		{"Extreme_Overload", 10000, time.Second * 5},
	}

	for _, level := range overloadLevels {
		b.Run(level.name, func(b *testing.B) {
			benchDB.ResetQueryStats()
			metrics := framework.NewPerformanceMetrics()
			metrics.StartMeasurement()

			tester := framework.NewLoadTester(framework.ConstantLoad).
				WithBaseRPS(level.targetRPS).
				WithDuration(level.duration)

			var totalErrors int64
			var totalRequests int64

			loadMetrics := tester.Execute(b, func(ctx context.Context, workerID int, loadMetrics *framework.PerformanceMetrics) error {
				atomic.AddInt64(&totalRequests, 1)

				reqBody := framework.BuildLLMRequest("gpt-4", []map[string]string{
					{"role": "user", "content": fmt.Sprintf("Degradation test %d", workerID)},
				})

				req := httptest.NewRequest("POST", "/llm/rest/test-llm-1/chat/completions", bytes.NewBuffer(reqBody))
				req = req.WithContext(ctx)
				req.Header.Set("Authorization", "Bearer test-key-1")
				req.Header.Set("Content-Type", "application/json")

				w := httptest.NewRecorder()
				proxy.Handler().ServeHTTP(w, req)

				if w.Code >= 400 {
					atomic.AddInt64(&totalErrors, 1)
					return fmt.Errorf("HTTP %d", w.Code)
				}
				return nil
			})

			// Calculate error rate
			errorRate := float64(totalErrors) / float64(totalRequests) * 100

			queryCount, _ := benchDB.QueryLogger.GetStats()
			b.ReportMetric(float64(queryCount), "degradation-queries")
			b.ReportMetric(level.targetRPS, "target-rps")
			b.ReportMetric(errorRate, "error-rate-percent")
			b.ReportMetric(float64(totalErrors), "total-errors")
			b.ReportMetric(float64(totalRequests), "total-requests")

			// Check if system degraded gracefully
			if errorRate > 50 && level.targetRPS >= 5000 {
				b.Logf("Expected high error rate under extreme load: %.2f%%", errorRate)
			} else if errorRate > 10 && level.targetRPS < 2000 {
				b.Logf("Warning: High error rate under normal load: %.2f%%", errorRate)
			}

			b.Logf("Degradation %s: %s, Error Rate: %.2f%%", level.name, loadMetrics.String(), errorRate)
		})
	}
}

// BenchmarkMultiVendorConcurrent tests concurrent requests to different vendors
func BenchmarkMultiVendorConcurrent(b *testing.B) {
	proxy, benchDB, mockServer := setupLoadTestProxy(b)
	defer mockServer.Close()

	// Test concurrent requests to different LLM vendors
	vendors := []struct {
		name   string
		llmID  string
		model  string
		vendor string
	}{
		{"OpenAI", "test-llm-1", "gpt-4", "openai"},
		{"Anthropic", "test-llm-2", "claude-3-haiku", "anthropic"},
		{"Google", "test-llm-3", "gemini-pro", "google"},
		{"Vertex", "test-llm-4", "gemini-1.0-pro", "vertex"},
	}

	b.Run("Multi_Vendor_Load", func(b *testing.B) {
		benchDB.ResetQueryStats()

		concurrency := len(vendors) * 5 // 5 workers per vendor
		tester := framework.NewConcurrentTester(concurrency).
			WithDuration(time.Second * 30)

		metrics := tester.Run(b, func(ctx context.Context, workerID int, metrics *framework.PerformanceMetrics) error {
			// Select vendor based on worker ID
			vendor := vendors[workerID%len(vendors)]

			reqBody := framework.BuildLLMRequest(vendor.model, []map[string]string{
				{"role": "user", "content": fmt.Sprintf("Multi-vendor test for %s", vendor.vendor)},
			})

			req := httptest.NewRequest("POST", fmt.Sprintf("/llm/rest/%s/chat/completions", vendor.llmID), bytes.NewBuffer(reqBody))
			req = req.WithContext(ctx)
			req.Header.Set("Authorization", "Bearer test-key-1")
			req.Header.Set("Content-Type", "application/json")

			w := httptest.NewRecorder()
			proxy.Handler().ServeHTTP(w, req)

			if w.Code == http.StatusOK {
				return nil
			}
			return fmt.Errorf("HTTP %d for %s", w.Code, vendor.vendor)
		})

		queryCount, _ := benchDB.QueryLogger.GetStats()
		b.ReportMetric(float64(queryCount), "multivendor-queries")
		b.ReportMetric(float64(len(vendors)), "vendor-count")
		b.ReportMetric(float64(concurrency), "total-workers")

		b.Logf("Multi-Vendor Load: %s", metrics.String())
	})
}

// BenchmarkStepLoadIncrease tests gradual load increases
func BenchmarkStepLoadIncrease(b *testing.B) {
	proxy, benchDB, mockServer := setupLoadTestProxy(b)
	defer mockServer.Close()

	b.Run("Step_Load_Pattern", func(b *testing.B) {
		benchDB.ResetQueryStats()

		tester := framework.NewLoadTester(framework.StepLoad).
			WithBaseRPS(100).
			WithMaxRPS(2000).
			WithDuration(time.Minute * 2).
			WithStepConfig(200, time.Second*15) // Increase by 200 RPS every 15 seconds

		var stepErrors []int64
		var stepRequests []int64

		metrics := tester.Execute(b, func(ctx context.Context, workerID int, metrics *framework.PerformanceMetrics) error {
			reqBody := framework.BuildLLMRequest("gpt-4", []map[string]string{
				{"role": "user", "content": fmt.Sprintf("Step load test %d", workerID)},
			})

			req := httptest.NewRequest("POST", "/llm/rest/test-llm-1/chat/completions", bytes.NewBuffer(reqBody))
			req = req.WithContext(ctx)
			req.Header.Set("Authorization", "Bearer test-key-1")
			req.Header.Set("Content-Type", "application/json")

			w := httptest.NewRecorder()
			proxy.Handler().ServeHTTP(w, req)

			// Track per-step metrics (simplified)
			step := int(time.Since(time.Now().Add(-time.Minute*2)).Seconds() / 15)
			if step >= 0 && step < 8 {
				if len(stepErrors) <= step {
					for len(stepErrors) <= step {
						stepErrors = append(stepErrors, 0)
						stepRequests = append(stepRequests, 0)
					}
				}
				stepRequests[step]++
				if w.Code >= 400 {
					stepErrors[step]++
				}
			}

			if w.Code == http.StatusOK {
				return nil
			}
			return fmt.Errorf("HTTP %d", w.Code)
		})

		queryCount, _ := benchDB.QueryLogger.GetStats()
		b.ReportMetric(float64(queryCount), "step-queries")
		b.ReportMetric(200.0, "step-size-rps")
		b.ReportMetric(15.0, "step-duration-sec")

		b.Logf("Step Load: %s", metrics.String())
	})
}

// BenchmarkLongRunningStability tests system stability over extended periods
func BenchmarkLongRunningStability(b *testing.B) {
	if testing.Short() {
		b.Skip("Skipping long-running stability test in short mode")
	}

	proxy, benchDB, mockServer := setupLoadTestProxy(b)
	defer mockServer.Close()

	b.Run("Stability_1Hour", func(b *testing.B) {
		benchDB.ResetQueryStats()

		// Start comprehensive monitoring
		leakTester := framework.NewMemoryLeakTester()
		leakTester.StartMonitoring()

		duration := time.Hour // Full hour test
		if testing.Short() {
			duration = time.Minute * 10 // Reduced for CI
		}

		tester := framework.NewLoadTester(framework.ConstantLoad).
			WithBaseRPS(100). // Moderate sustained load
			WithDuration(duration)

		var samples []time.Time
		var errorCount int64
		var requestCount int64

		metrics := tester.Execute(b, func(ctx context.Context, workerID int, metrics *framework.PerformanceMetrics) error {
			atomic.AddInt64(&requestCount, 1)
			samples = append(samples, time.Now())

			reqBody := framework.BuildLLMRequest("gpt-4", []map[string]string{
				{"role": "user", "content": fmt.Sprintf("Stability test %d", atomic.LoadInt64(&requestCount))},
			})

			req := httptest.NewRequest("POST", "/llm/rest/test-llm-1/chat/completions", bytes.NewBuffer(reqBody))
			req = req.WithContext(ctx)
			req.Header.Set("Authorization", "Bearer test-key-1")
			req.Header.Set("Content-Type", "application/json")

			w := httptest.NewRecorder()
			proxy.Handler().ServeHTTP(w, req)

			if w.Code >= 400 {
				atomic.AddInt64(&errorCount, 1)
				return fmt.Errorf("HTTP %d", w.Code)
			}
			return nil
		})

		// Check for memory leaks
		hasLeak, leakDesc := leakTester.StopMonitoring()

		queryCount, _ := benchDB.QueryLogger.GetStats()
		errorRate := float64(errorCount) / float64(requestCount) * 100

		b.ReportMetric(float64(queryCount), "stability-queries")
		b.ReportMetric(float64(duration.Minutes()), "duration-minutes")
		b.ReportMetric(errorRate, "error-rate-percent")
		b.ReportMetric(float64(requestCount), "total-requests")
		if hasLeak {
			b.ReportMetric(1, "memory-leak")
		} else {
			b.ReportMetric(0, "memory-leak")
		}

		b.Logf("Stability Test: %s", metrics.String())
		b.Logf("Error Rate: %.4f%%, Memory Leak: %t", errorRate, hasLeak)
		if hasLeak {
			b.Logf("Leak Details: %s", leakDesc)
		}

		// Verify system remained stable
		if errorRate > 1.0 {
			b.Errorf("High error rate during stability test: %.2f%%", errorRate)
		}
		if hasLeak {
			b.Errorf("Memory leak detected during stability test: %s", leakDesc)
		}
	})
}