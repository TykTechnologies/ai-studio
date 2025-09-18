package analytics

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/performance/framework"
)

// BenchmarkAnalyticsBatchProcessing measures batch analytics processing performance
func BenchmarkAnalyticsBatchProcessing(b *testing.B) {
	// Create benchmark database
	benchDB := framework.NewBenchmarkDB(b).SetTestDataSize(20)
	benchDB.SetupTestData(b)

	ctx := context.Background()
	handler := NewDatabaseHandler(ctx, benchDB.DB)

	// Test different batch sizes
	batchSizes := []int{10, 50, 100, 500, 1000}

	for _, batchSize := range batchSizes {
		b.Run(fmt.Sprintf("Batch_%d", batchSize), func(b *testing.B) {
			benchDB.ResetQueryStats()

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				// Create unique test records batch for each iteration
				records := make([]*models.LLMChatRecord, batchSize)
				baseOffset := i * batchSize // Ensure unique records per iteration

				for j := 0; j < batchSize; j++ {
					recordID := baseOffset + j
					records[j] = &models.LLMChatRecord{
						LLMID:           uint(recordID%10 + 1), // Cycle through LLMs
						AppID:           uint(recordID%5 + 1),  // Cycle through apps
						Name:            fmt.Sprintf("test-model-%d", recordID),
						Vendor:          "test-vendor",
						TotalTokens:     100 + recordID,
						PromptTokens:    50 + recordID/2,
						ResponseTokens:  50 + recordID/2,
						Cost:            float64(1000 + recordID), // Cost in micro-units
						Currency:        "USD",
						TimeStamp:       time.Now().Add(-time.Duration(recordID) * time.Microsecond), // More granular timing
						InteractionType: models.ProxyInteraction,
					}
				}

				start := time.Now()
				handler.RecordChatRecordsBatch(records)
				batchDuration := time.Since(start)

				b.ReportMetric(float64(batchDuration.Nanoseconds()), "batch-ns")
				b.ReportMetric(float64(batchSize), "batch-size")
			}

			queryCount, _ := benchDB.QueryLogger.GetStats()
			b.ReportMetric(float64(queryCount), "batch-queries")
		})
	}
}

// BenchmarkAnalyticsDataInsertion measures individual record insertion performance
func BenchmarkAnalyticsDataInsertion(b *testing.B) {
	benchDB := framework.NewBenchmarkDB(b).SetTestDataSize(10)
	benchDB.SetupTestData(b)

	ctx := context.Background()
	handler := NewDatabaseHandler(ctx, benchDB.DB)

	// Test different record types
	scenarios := []struct {
		name        string
		recordType  string
		createRecord func(i int) interface{}
	}{
		{
			name:       "Chat_Records",
			recordType: "chat",
			createRecord: func(i int) interface{} {
				return &models.LLMChatRecord{
					LLMID:           uint(i%5 + 1),
					AppID:           uint(i%3 + 1),
					Name:            fmt.Sprintf("model-%d", i),
					Vendor:          "openai",
					TotalTokens:     100 + i,
					PromptTokens:    50 + i/2,
					ResponseTokens:  50 + i/2,
					Cost:            float64(1000 + i),
					Currency:        "USD",
					TimeStamp:       time.Now().Add(time.Duration(i) * time.Microsecond),
					InteractionType: models.ProxyInteraction,
				}
			},
		},
		{
			name:       "Proxy_Logs",
			recordType: "proxy",
			createRecord: func(i int) interface{} {
				return &models.ProxyLog{
					AppID:        uint(i%3 + 1),
					UserID:       uint(i%2 + 1),
					Vendor:       "openai",
					ResponseCode: 200,
					TimeStamp:    time.Now().Add(time.Duration(i) * time.Microsecond),
				}
			},
		},
	}

	for _, scenario := range scenarios {
		b.Run(scenario.name, func(b *testing.B) {
			benchDB.ResetQueryStats()

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				record := scenario.createRecord(i)

				start := time.Now()
				if scenario.recordType == "chat" {
					handler.RecordChatRecord(record.(*models.LLMChatRecord))
				} else if scenario.recordType == "proxy" {
					handler.RecordProxyLog(record.(*models.ProxyLog))
				}
				insertDuration := time.Since(start)

				b.ReportMetric(float64(insertDuration.Nanoseconds()), "insert-ns")
			}

			queryCount, _ := benchDB.QueryLogger.GetStats()
			b.ReportMetric(float64(queryCount), "insert-queries")
		})
	}
}

// BenchmarkAnalyticsQuerying measures analytics query performance
func BenchmarkAnalyticsQuerying(b *testing.B) {
	// Create larger dataset for querying
	benchDB := framework.NewBenchmarkDB(b).SetTestDataSize(50)
	benchDB.SetupTestData(b)

	// Insert test analytics data
	now := time.Now()
	for i := 0; i < 1000; i++ {
		record := &models.LLMChatRecord{
			LLMID:           uint(i%10 + 1),
			AppID:           uint(i%5 + 1),
			Name:            fmt.Sprintf("model-%d", i%3),
			Vendor:          []string{"openai", "anthropic", "google"}[i%3],
			TotalTokens:     100 + i,
			PromptTokens:    50 + i/2,
			ResponseTokens:  50 + i/2,
			Cost:            float64(1000 + i)*10,
			Currency:        "USD",
			TimeStamp:       now.Add(-time.Duration(i) * time.Minute),
			InteractionType: models.ProxyInteraction,
		}
		benchDB.DB.Create(record)
	}

	// Test different query patterns
	queries := []struct {
		name  string
		query func() error
	}{
		{
			name: "Usage_By_App",
			query: func() error {
				var results []struct {
					AppID       uint
					TotalTokens int
					TotalCost   int
				}
				return benchDB.DB.Model(&models.LLMChatRecord{}).
					Select("app_id, sum(total_tokens) as total_tokens, sum(cost) as total_cost").
					Group("app_id").
					Find(&results).Error
			},
		},
		{
			name: "Usage_By_Vendor",
			query: func() error {
				var results []struct {
					Vendor      string
					TotalTokens int
					TotalCost   int
				}
				return benchDB.DB.Model(&models.LLMChatRecord{}).
					Select("vendor, sum(total_tokens) as total_tokens, sum(cost) as total_cost").
					Group("vendor").
					Find(&results).Error
			},
		},
		{
			name: "Daily_Usage",
			query: func() error {
				var results []struct {
					Date        string
					TotalTokens int
					TotalCost   int
				}
				return benchDB.DB.Model(&models.LLMChatRecord{}).
					Select("DATE(time_stamp) as date, sum(total_tokens) as total_tokens, sum(cost) as total_cost").
					Where("time_stamp >= ?", now.Add(-24*time.Hour)).
					Group("DATE(time_stamp)").
					Find(&results).Error
			},
		},
		{
			name: "Top_Models",
			query: func() error {
				var results []struct {
					Name        string
					RequestCount int
					TotalTokens int
				}
				return benchDB.DB.Model(&models.LLMChatRecord{}).
					Select("name, count(*) as request_count, sum(total_tokens) as total_tokens").
					Group("name").
					Order("request_count DESC").
					Limit(10).
					Find(&results).Error
			},
		},
	}

	for _, query := range queries {
		b.Run(query.name, func(b *testing.B) {
			benchDB.ResetQueryStats()

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				start := time.Now()
				err := query.query()
				queryDuration := time.Since(start)

				if err != nil {
					b.Errorf("Query failed: %v", err)
				}

				b.ReportMetric(float64(queryDuration.Nanoseconds()), "query-ns")
			}

			queryCount, _ := benchDB.QueryLogger.GetStats()
			b.ReportMetric(float64(queryCount), "analytics-queries")
		})
	}
}

// BenchmarkAnalyticsAggregation measures aggregation performance
func BenchmarkAnalyticsAggregation(b *testing.B) {
	benchDB := framework.NewBenchmarkDB(b).SetTestDataSize(20)
	benchDB.SetupTestData(b)

	// Insert substantial analytics data for aggregation
	now := time.Now()
	recordCount := 5000
	records := make([]*models.LLMChatRecord, recordCount)

	for i := 0; i < recordCount; i++ {
		records[i] = &models.LLMChatRecord{
			LLMID:           uint(i%20 + 1),
			AppID:           uint(i%10 + 1),
			Name:            fmt.Sprintf("model-%d", i%5),
			Vendor:          []string{"openai", "anthropic", "google", "vertex"}[i%4],
			TotalTokens:     50 + i%500,
			PromptTokens:    25 + i%250,
			ResponseTokens:  25 + i%250,
			Cost:            float64(500 + i%2000),
			Currency:        "USD",
			TimeStamp:       now.Add(-time.Duration(i) * time.Second),
			InteractionType: models.ProxyInteraction,
		}
	}

	// Batch insert for performance
	batchSize := 100
	for i := 0; i < len(records); i += batchSize {
		end := i + batchSize
		if end > len(records) {
			end = len(records)
		}
		benchDB.DB.Create(records[i:end])
	}

	// Test different aggregation scenarios
	aggregations := []struct {
		name        string
		description string
		query       func() error
	}{
		{
			name:        "Hourly_Aggregation",
			description: "Aggregate data by hour",
			query: func() error {
				var results []struct {
					Hour        int
					TotalTokens int
					TotalCost   int64
					RequestCount int
				}
				return benchDB.DB.Model(&models.LLMChatRecord{}).
					Select("strftime('%H', time_stamp) as hour, sum(total_tokens) as total_tokens, sum(cost) as total_cost, count(*) as request_count").
					Where("time_stamp >= ?", now.Add(-24*time.Hour)).
					Group("strftime('%H', time_stamp)").
					Find(&results).Error
			},
		},
		{
			name:        "App_LLM_Matrix",
			description: "Cross-tabulate apps and LLMs",
			query: func() error {
				var results []struct {
					AppID       uint
					LLMID       uint
					TotalTokens int
					TotalCost   int64
					RequestCount int
				}
				return benchDB.DB.Model(&models.LLMChatRecord{}).
					Select("app_id, llm_id, sum(total_tokens) as total_tokens, sum(cost) as total_cost, count(*) as request_count").
					Group("app_id, llm_id").
					Find(&results).Error
			},
		},
		{
			name:        "Cost_Distribution",
			description: "Analyze cost distribution",
			query: func() error {
				var results []struct {
					CostRange   string
					Count       int
					TotalCost   int64
				}
				return benchDB.DB.Raw(`
					SELECT
						CASE
							WHEN cost < 1000 THEN 'Low (< $0.10)'
							WHEN cost < 5000 THEN 'Medium ($0.10-$0.50)'
							WHEN cost < 10000 THEN 'High ($0.50-$1.00)'
							ELSE 'Very High (> $1.00)'
						END as cost_range,
						COUNT(*) as count,
						SUM(cost) as total_cost
					FROM llm_chat_records
					GROUP BY cost_range
				`).Find(&results).Error
			},
		},
	}

	for _, agg := range aggregations {
		b.Run(agg.name, func(b *testing.B) {
			benchDB.ResetQueryStats()

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				start := time.Now()
				err := agg.query()
				aggDuration := time.Since(start)

				if err != nil {
					b.Errorf("Aggregation failed: %v", err)
				}

				b.ReportMetric(float64(aggDuration.Nanoseconds()), "aggregation-ns")
			}

			queryCount, _ := benchDB.QueryLogger.GetStats()
			b.ReportMetric(float64(queryCount), "agg-queries")
			b.ReportMetric(float64(recordCount), "dataset-size")
		})
	}
}

// BenchmarkAnalyticsConcurrency measures performance under concurrent analytics operations
func BenchmarkAnalyticsConcurrency(b *testing.B) {
	benchDB := framework.NewBenchmarkDB(b).SetTestDataSize(10)
	benchDB.SetupTestData(b)

	ctx := context.Background()
	handler := NewDatabaseHandler(ctx, benchDB.DB)

	concurrencyLevels := []int{1, 5, 10, 20}

	for _, concurrency := range concurrencyLevels {
		b.Run(fmt.Sprintf("Workers_%d", concurrency), func(b *testing.B) {
			benchDB.ResetQueryStats()

			tester := framework.NewConcurrentTester(concurrency).
				WithRequestCount(int64(b.N))

			metrics := tester.Run(b, func(ctx context.Context, workerID int, metrics *framework.PerformanceMetrics) error {
				// Create a batch of analytics records with proper concurrency safety
				batchSize := 10
				records := make([]*models.LLMChatRecord, batchSize)

				// Use nanosecond timestamp to ensure uniqueness across all workers and iterations
				baseTime := time.Now().UnixNano()

				for i := 0; i < batchSize; i++ {
					uniqueID := int64(workerID)*100000 + int64(i) + baseTime // Ensure global uniqueness
					records[i] = &models.LLMChatRecord{
						LLMID:           uint(uniqueID%10 + 1),
						AppID:           uint(uniqueID%5 + 1),
						Name:            fmt.Sprintf("worker-%d-model-%d-%d", workerID, i, baseTime),
						Vendor:          "concurrent-test",
						TotalTokens:     100 + i,
						PromptTokens:    50 + i/2,
						ResponseTokens:  50 + i/2,
						Cost:            float64(1000 + i)*float64(workerID+1), // +1 to avoid zero cost
						Currency:        "USD",
						TimeStamp:       time.Unix(0, baseTime+int64(i)*1000), // Unique nanosecond timestamps
						InteractionType: models.ProxyInteraction,
					}
				}

				// Add small delay between workers to reduce database contention
				if workerID > 0 {
					time.Sleep(time.Duration(workerID) * time.Microsecond * 10)
				}

				handler.RecordChatRecordsBatch(records)
				return nil
			})

			queryCount, _ := benchDB.QueryLogger.GetStats()
			b.ReportMetric(float64(queryCount), "concurrent-queries")
			b.ReportMetric(float64(concurrency), "analytics-workers")

			b.Logf("Analytics Concurrency %d: %s", concurrency, metrics.String())
		})
	}
}

// BenchmarkAnalyticsMemoryUsage measures memory usage during analytics processing
func BenchmarkAnalyticsMemoryUsage(b *testing.B) {
	benchDB := framework.NewBenchmarkDB(b).SetTestDataSize(10)
	benchDB.SetupTestData(b)

	ctx := context.Background()
	handler := NewDatabaseHandler(ctx, benchDB.DB)

	// Start memory monitoring
	leakTester := framework.NewMemoryLeakTester()
	leakTester.StartMonitoring()

	// Test different batch sizes for memory usage
	batchSizes := []int{100, 500, 1000, 2000}

	for _, batchSize := range batchSizes {
		b.Run(fmt.Sprintf("Memory_Batch_%d", batchSize), func(b *testing.B) {
			benchDB.ResetQueryStats()

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				// Create unique records for each iteration to avoid ID conflicts
				records := make([]*models.LLMChatRecord, batchSize)
				baseOffset := i * batchSize

				for j := 0; j < batchSize; j++ {
					recordID := baseOffset + j
					records[j] = &models.LLMChatRecord{
						LLMID:           uint(recordID%10 + 1),
						AppID:           uint(recordID%5 + 1),
						Name:            fmt.Sprintf("memory-test-model-%d", recordID),
						Vendor:          "memory-test",
						TotalTokens:     100 + recordID,
						PromptTokens:    50 + recordID/2,
						ResponseTokens:  50 + recordID/2,
						Cost:            float64(1000 + recordID),
						Currency:        "USD",
						TimeStamp:       time.Now().Add(time.Duration(recordID) * time.Nanosecond),
						InteractionType: models.ProxyInteraction,
					}
				}

				handler.RecordChatRecordsBatch(records)

				// Periodic memory sampling
				if i%10 == 0 {
					time.Sleep(time.Millisecond)
				}
			}

			b.ReportMetric(float64(batchSize), "memory-batch-size")
			queryCount, _ := benchDB.QueryLogger.GetStats()
			b.ReportMetric(float64(queryCount), "memory-queries")
		})
	}

	// Check for memory leaks
	hasLeak, leakDesc := leakTester.StopMonitoring()
	if hasLeak {
		b.Logf("Memory leak detected: %s", leakDesc)
		b.ReportMetric(1, "analytics-memory-leak")
	} else {
		b.ReportMetric(0, "analytics-memory-leak")
	}
}