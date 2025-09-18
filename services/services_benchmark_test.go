package services

import (
	"fmt"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/performance/framework"
)

// BenchmarkServiceCRUDOperations measures CRUD operation performance
func BenchmarkServiceCRUDOperations(b *testing.B) {
	benchDB := framework.NewBenchmarkDB(b).SetTestDataSize(20)
	benchDB.SetupTestData(b)

	service := NewService(benchDB.DB)

	// Test different CRUD operations
	operations := []struct {
		name   string
		operation func() error
	}{
		{
			name: "Create_LLM",
			operation: func() error {
				llm := &models.LLM{
					Name:         "Benchmark LLM",
					Vendor:       models.OPENAI,
					DefaultModel: "gpt-4",
					Active:       true,
					MaxTokens:    4096,
					Timeout:      30,
				}
				_, err := service.CreateLLM(llm)
				return err
			},
		},
		{
			name: "Get_LLM",
			operation: func() error {
				_, err := service.GetLLMByID(1)
				return err
			},
		},
		{
			name: "Update_LLM",
			operation: func() error {
				llm, err := service.GetLLMByID(1)
				if err != nil {
					return err
				}
				llm.Name = "Updated Benchmark LLM"
				_, err = service.UpdateLLM(llm)
				return err
			},
		},
		{
			name: "List_LLMs",
			operation: func() error {
				_, _, _, err := service.GetAllLLMs(10, 1, false)
				return err
			},
		},
		{
			name: "Create_App",
			operation: func() error {
				app := &models.App{
					Name:          "Benchmark App",
					Owner:         1,
					Active:        true,
					MonthlyBudget: 1000.0,
				}
				_, err := service.CreateApp(app)
				return err
			},
		},
		{
			name: "Get_App",
			operation: func() error {
				_, err := service.GetAppByID(1)
				return err
			},
		},
	}

	for _, op := range operations {
		b.Run(op.name, func(b *testing.B) {
			benchDB.ResetQueryStats()
			metrics := framework.NewPerformanceMetrics()
			metrics.StartMeasurement()

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				start := time.Now()
				err := op.operation()
				opDuration := time.Since(start)

				metrics.RecordLatency(opDuration)
				if err != nil {
					metrics.RecordError()
				} else {
					metrics.RecordSuccess()
				}
			}

			metrics.EndMeasurement(time.Since(time.Now()))
			metrics.ReportToB(b)

			queryCount, _ := benchDB.QueryLogger.GetStats()
			b.ReportMetric(float64(queryCount), "crud-queries")
		})
	}
}

// BenchmarkServiceComplexQueries measures performance of complex database queries
func BenchmarkServiceComplexQueries(b *testing.B) {
	benchDB := framework.NewBenchmarkDB(b).SetTestDataSize(50)
	benchDB.SetupTestData(b)

	service := NewService(benchDB.DB)

	// Test complex queries
	queries := []struct {
		name  string
		query func() error
	}{
		{
			name: "LLMs_With_Plugins",
			query: func() error {
				_, _, _, err := service.GetAllLLMs(20, 1, true) // With preloading
				return err
			},
		},
		{
			name: "Apps_With_Credentials",
			query: func() error {
				apps, err := service.GetAllApps()
				if err != nil {
					return err
				}
				// Simulate loading credentials for each app
				for _, app := range apps {
					_, err := service.GetCredentialsByAppID(app.ID)
					if err != nil {
						return err
					}
				}
				return nil
			},
		},
		{
			name: "Active_LLMs_Only",
			query: func() error {
				_, err := service.GetActiveLLMs()
				return err
			},
		},
		{
			name: "Search_LLMs_By_Vendor",
			query: func() error {
				// Simulate vendor-based search
				allLLMs, _, _, err := service.GetAllLLMs(100, 1, false)
				if err != nil {
					return err
				}

				// Filter by vendor (simulating search)
				var filtered []models.LLM
				for _, llm := range allLLMs {
					if llm.Vendor == models.OPENAI {
						filtered = append(filtered, llm)
					}
				}
				return nil
			},
		},
		{
			name: "User_Apps_Access",
			query: func() error {
				// Get all apps for a user (complex ownership check)
				user, err := service.GetUserByID(1)
				if err != nil {
					return err
				}

				apps, err := service.GetAllApps()
				if err != nil {
					return err
				}

				// Filter apps by ownership
				var userApps []models.App
				for _, app := range apps {
					if app.Owner == user.ID {
						userApps = append(userApps, app)
					}
				}
				return nil
			},
		},
	}

	for _, query := range queries {
		b.Run(query.name, func(b *testing.B) {
			benchDB.ResetQueryStats()
			metrics := framework.NewPerformanceMetrics()
			metrics.StartMeasurement()

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				start := time.Now()
				err := query.query()
				queryDuration := time.Since(start)

				metrics.RecordLatency(queryDuration)
				if err != nil {
					metrics.RecordError()
				} else {
					metrics.RecordSuccess()
				}
			}

			metrics.EndMeasurement(time.Since(time.Now()))
			metrics.ReportToB(b)

			queryCount, _ := benchDB.QueryLogger.GetStats()
			b.ReportMetric(float64(queryCount), "complex-queries")
		})
	}
}

// BenchmarkServiceCaching measures cache performance impact
func BenchmarkServiceCaching(b *testing.B) {
	benchDB := framework.NewBenchmarkDB(b).SetTestDataSize(30)
	benchDB.SetupTestData(b)

	service := NewService(benchDB.DB)

	// Test cache scenarios
	scenarios := []struct {
		name        string
		description string
		operation   func() error
		warmCache   bool
	}{
		{
			name:        "Cold_Cache_LLM_Lookup",
			description: "LLM lookup without cache warming",
			warmCache:   false,
			operation: func() error {
				_, err := service.GetLLMByID(1)
				return err
			},
		},
		{
			name:        "Warm_Cache_LLM_Lookup",
			description: "LLM lookup with warmed cache",
			warmCache:   true,
			operation: func() error {
				_, err := service.GetLLMByID(1)
				return err
			},
		},
		{
			name:        "Cold_Cache_App_Lookup",
			description: "App lookup without cache warming",
			warmCache:   false,
			operation: func() error {
				_, err := service.GetAppByID(1)
				return err
			},
		},
		{
			name:        "Warm_Cache_App_Lookup",
			description: "App lookup with warmed cache",
			warmCache:   true,
			operation: func() error {
				_, err := service.GetAppByID(1)
				return err
			},
		},
	}

	for _, scenario := range scenarios {
		b.Run(scenario.name, func(b *testing.B) {
			// Warm cache if requested
			if scenario.warmCache {
				scenario.operation() // Prime the cache
			}

			benchDB.ResetQueryStats()
			metrics := framework.NewPerformanceMetrics()
			metrics.StartMeasurement()

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				start := time.Now()
				err := scenario.operation()
				cacheDuration := time.Since(start)

				metrics.RecordLatency(cacheDuration)
				if err != nil {
					metrics.RecordError()
				} else {
					metrics.RecordSuccess()
				}
			}

			metrics.EndMeasurement(time.Since(time.Now()))
			metrics.ReportToB(b)

			queryCount, _ := benchDB.QueryLogger.GetStats()
			b.ReportMetric(float64(queryCount), "cache-queries")
			b.ReportMetric(boolToFloat(scenario.warmCache), "cache-warmed")
		})
	}
}

// BenchmarkBudgetServicePerformance measures budget service performance
func BenchmarkBudgetServicePerformance(b *testing.B) {
	benchDB := framework.NewBenchmarkDB(b).SetTestDataSize(20)
	benchDB.SetupTestData(b)

	budgetService := NewBudgetService(benchDB.DB, nil)

	// Test budget operations
	operations := []struct {
		name      string
		operation func() error
	}{
		{
			name: "Check_App_Budget",
			operation: func() error {
				cost := 0.05 // $0.05
				return budgetService.CheckBudget(1, 1, cost, 100, 50, 50)
			},
		},
		{
			name: "Check_LLM_Budget",
			operation: func() error {
				cost := 0.10 // $0.10
				return budgetService.CheckLLMBudget(1, cost, 200, 100, 100)
			},
		},
		{
			name: "Update_Usage",
			operation: func() error {
				cost := 0.03 // $0.03
				budgetService.UpdateUsage(1, 1, cost, 75, 25, 50)
				return nil
			},
		},
		{
			name: "Get_Usage_Summary",
			operation: func() error {
				_, _, err := budgetService.GetUsageSummary(1, time.Now().AddDate(0, -1, 0), time.Now())
				return err
			},
		},
	}

	for _, op := range operations {
		b.Run(op.name, func(b *testing.B) {
			benchDB.ResetQueryStats()
			metrics := framework.NewPerformanceMetrics()
			metrics.StartMeasurement()

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				start := time.Now()
				err := op.operation()
				budgetDuration := time.Since(start)

				metrics.RecordLatency(budgetDuration)
				if err != nil {
					metrics.RecordError()
				} else {
					metrics.RecordSuccess()
				}
			}

			metrics.EndMeasurement(time.Since(time.Now()))
			metrics.ReportToB(b)

			queryCount, _ := benchDB.QueryLogger.GetStats()
			b.ReportMetric(float64(queryCount), "budget-queries")
		})
	}
}

// BenchmarkServiceConcurrency measures service performance under concurrent load
func BenchmarkServiceConcurrency(b *testing.B) {
	benchDB := framework.NewBenchmarkDB(b).SetTestDataSize(20)
	benchDB.SetupTestData(b)

	service := NewService(benchDB.DB)

	concurrencyLevels := []int{1, 5, 10, 20}

	for _, concurrency := range concurrencyLevels {
		b.Run(fmt.Sprintf("Concurrent_%d", concurrency), func(b *testing.B) {
			benchDB.ResetQueryStats()

			tester := framework.NewConcurrentTester(concurrency).
				WithRequestCount(int64(b.N))

			metrics := tester.Run(b, func(ctx context.Context, workerID int, metrics *framework.PerformanceMetrics) error {
				// Mix of different service operations
				operations := []func() error{
					func() error { _, err := service.GetLLMByID(uint(workerID%10 + 1)); return err },
					func() error { _, err := service.GetAppByID(uint(workerID%5 + 1)); return err },
					func() error { _, err := service.GetActiveLLMs(); return err },
					func() error { _, _, _, err := service.GetAllLLMs(10, 1, false); return err },
				}

				op := operations[workerID%len(operations)]
				return op()
			})

			queryCount, _ := benchDB.QueryLogger.GetStats()
			b.ReportMetric(float64(queryCount), "concurrent-service-queries")
			b.ReportMetric(float64(concurrency), "service-workers")

			b.Logf("Service Concurrency %d: %s", concurrency, metrics.String())
		})
	}
}

// BenchmarkServiceMemoryUsage measures memory usage patterns
func BenchmarkServiceMemoryUsage(b *testing.B) {
	benchDB := framework.NewBenchmarkDB(b).SetTestDataSize(10)
	benchDB.SetupTestData(b)

	service := NewService(benchDB.DB)

	// Start memory monitoring
	leakTester := framework.NewMemoryLeakTester()
	leakTester.StartMonitoring()

	benchDB.ResetQueryStats()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Perform various service operations
		service.GetLLMByID(uint(i%10 + 1))
		service.GetAppByID(uint(i%5 + 1))
		service.GetActiveLLMs()

		// Create and delete operations to test cleanup
		if i%100 == 0 {
			app := &models.App{
				Name:          fmt.Sprintf("Memory Test App %d", i),
				Owner:         1,
				Active:        true,
				MonthlyBudget: 100.0,
			}
			createdApp, err := service.CreateApp(app)
			if err == nil && i%200 == 0 { // Delete every other created app
				service.DeleteApp(createdApp.ID)
			}
		}

		// Periodic memory check
		if i%50 == 0 {
			time.Sleep(time.Millisecond)
		}
	}

	// Check for memory leaks
	hasLeak, leakDesc := leakTester.StopMonitoring()
	if hasLeak {
		b.Logf("Memory leak detected: %s", leakDesc)
		b.ReportMetric(1, "service-memory-leak")
	} else {
		b.ReportMetric(0, "service-memory-leak")
	}

	queryCount, _ := benchDB.QueryLogger.GetStats()
	b.ReportMetric(float64(queryCount), "memory-queries")
}

// BenchmarkServiceNPlusOneQueries tests for N+1 query issues
func BenchmarkServiceNPlusOneQueries(b *testing.B) {
	benchDB := framework.NewBenchmarkDB(b).SetTestDataSize(30)
	benchDB.SetupTestData(b)

	service := NewService(benchDB.DB)

	// Test potential N+1 scenarios
	scenarios := []struct {
		name        string
		description string
		operation   func() error
	}{
		{
			name:        "LLMs_Without_Preloading",
			description: "Get LLMs and access plugins individually",
			operation: func() error {
				llms, _, _, err := service.GetAllLLMs(20, 1, false) // No preloading
				if err != nil {
					return err
				}
				// This might trigger N+1 if plugins are lazy loaded
				for _, llm := range llms {
					_ = len(llm.Plugins) // Access plugins
				}
				return nil
			},
		},
		{
			name:        "LLMs_With_Preloading",
			description: "Get LLMs with preloaded relationships",
			operation: func() error {
				llms, _, _, err := service.GetAllLLMs(20, 1, true) // With preloading
				if err != nil {
					return err
				}
				// Should not trigger additional queries
				for _, llm := range llms {
					_ = len(llm.Plugins) // Access plugins
				}
				return nil
			},
		},
		{
			name:        "Apps_With_Credentials",
			description: "Get apps and load credentials",
			operation: func() error {
				apps, err := service.GetAllApps()
				if err != nil {
					return err
				}
				// Load credentials for each app (potential N+1)
				for _, app := range apps[:5] { // Limit to first 5 to avoid excessive queries
					_, err := service.GetCredentialsByAppID(app.ID)
					if err != nil {
						return err
					}
				}
				return nil
			},
		},
	}

	for _, scenario := range scenarios {
		b.Run(scenario.name, func(b *testing.B) {
			benchDB.ResetQueryStats()
			metrics := framework.NewPerformanceMetrics()
			metrics.StartMeasurement()

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				start := time.Now()
				err := scenario.operation()
				opDuration := time.Since(start)

				metrics.RecordLatency(opDuration)
				if err != nil {
					metrics.RecordError()
				} else {
					metrics.RecordSuccess()
				}
			}

			metrics.EndMeasurement(time.Since(time.Now()))
			metrics.ReportToB(b)

			queryCount, _ := benchDB.QueryLogger.GetStats()
			b.ReportMetric(float64(queryCount), "nplus1-queries")

			// Log query count to help identify N+1 issues
			b.Logf("Scenario %s: %d queries for %d iterations", scenario.name, queryCount, b.N)
		})
	}
}

// Helper functions

// boolToFloat converts boolean to float for metric reporting
func boolToFloat(b bool) float64 {
	if b {
		return 1.0
	}
	return 0.0
}