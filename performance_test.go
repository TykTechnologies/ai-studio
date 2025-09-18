package main

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/analytics"
	"github.com/TykTechnologies/midsommar/v2/grpc"
	"github.com/TykTechnologies/midsommar/v2/models"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// setupPerformanceTestDB creates a test database with query logging
func setupPerformanceTestDB(t *testing.T) (*gorm.DB, *QueryCountLogger) {
	queryLogger := &QueryCountLogger{}

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: queryLogger,
	})
	require.NoError(t, err)

	// Migrate all required tables
	err = db.AutoMigrate(
		&models.LLM{},
		&models.Plugin{},
		&models.Filter{},
		&models.LLMPlugin{},
		&models.LLMChatRecord{},
		&models.ProxyLog{},
		&models.EdgeInstance{},
	)
	require.NoError(t, err)

	return db, queryLogger
}

// QueryCountLogger captures database queries for performance analysis
type QueryCountLogger struct {
	QueryCount int
	Queries    []string
	mu         sync.Mutex
}

func (l *QueryCountLogger) LogMode(level logger.LogLevel) logger.Interface {
	return l
}

func (l *QueryCountLogger) Info(ctx context.Context, msg string, data ...interface{}) {}
func (l *QueryCountLogger) Warn(ctx context.Context, msg string, data ...interface{}) {}
func (l *QueryCountLogger) Error(ctx context.Context, msg string, data ...interface{}) {}

func (l *QueryCountLogger) Trace(ctx context.Context, begin time.Time, fc func() (sql string, rowsAffected int64), err error) {
	sql, _ := fc()
	l.mu.Lock()
	l.QueryCount++
	l.Queries = append(l.Queries, sql)
	l.mu.Unlock()
}

func createPerformanceTestData(t *testing.T, db *gorm.DB) {
	// Create test LLMs with relationships
	for i := 1; i <= 10; i++ {
		llm := &models.LLM{
			Model:        gorm.Model{ID: uint(i)},
			Name:         fmt.Sprintf("TestLLM%d", i),
			Vendor:       models.OPENAI,
			DefaultModel: fmt.Sprintf("test-model-%d", i),
			Active:       true,
			Namespace:    "",
		}
		err := db.Create(llm).Error
		require.NoError(t, err)
	}

	// Create test plugins
	for i := 1; i <= 20; i++ {
		plugin := &models.Plugin{
			Model:       gorm.Model{ID: uint(i)},
			Name:        fmt.Sprintf("TestPlugin%d", i),
			Slug:        fmt.Sprintf("test-plugin-%d", i),
			Description: fmt.Sprintf("Test plugin %d", i),
			Command:     fmt.Sprintf("./plugin%d", i),
			HookType:    "post_auth",
			IsActive:    true,
			Namespace:   "",
		}
		err := db.Create(plugin).Error
		require.NoError(t, err)
	}

	// Create LLM-Plugin associations (each LLM has 3-5 plugins)
	for llmID := 1; llmID <= 10; llmID++ {
		pluginCount := 3 + (llmID % 3) // 3-5 plugins per LLM
		for pluginID := 1; pluginID <= pluginCount; pluginID++ {
			assoc := &models.LLMPlugin{
				LLMID:    uint(llmID),
				PluginID: uint(pluginID),
				IsActive: true,
			}
			err := db.Create(assoc).Error
			require.NoError(t, err)
		}
	}
}

func TestLLMSerialization_N1Prevention(t *testing.T) {
	db, queryLogger := setupPerformanceTestDB(t)
	createPerformanceTestData(t, db)

	service := services.NewService(db)

	// Test: List all LLMs and serialize them (simulates API handler)
	queryLogger.QueryCount = 0
	queryLogger.Queries = []string{}

	llms, _, _, err := service.GetAllLLMs(10, 1, false)
	require.NoError(t, err)
	assert.Len(t, llms, 10)

	// Simulate serialization access (what the API handlers do)
	totalPlugins := 0
	for _, llm := range llms {
		totalPlugins += len(llm.Plugins)
		// This would trigger N+1 queries if plugins weren't preloaded
		for _, plugin := range llm.Plugins {
			assert.NotEmpty(t, plugin.Name)
		}
	}

	t.Logf("Total queries executed: %d", queryLogger.QueryCount)
	t.Logf("Total plugins accessed: %d", totalPlugins)

	// With proper preloading, query count should be minimal (2-3 queries max)
	// Without preloading, it would be 1 + 10 queries (N+1 pattern)
	assert.LessOrEqual(t, queryLogger.QueryCount, 5,
		"Query count should be minimal with preloading (got %d)", queryLogger.QueryCount)

	// Verify we accessed plugins from all LLMs
	assert.Greater(t, totalPlugins, 25, "Should have accessed plugins from all LLMs")
}

func TestAnalyticsPulseBatchProcessing_Performance(t *testing.T) {
	// Set required environment variable for testing (32 characters)
	testEncryptionKey := "12345678901234567890123456789012"
	os.Setenv("MICROGATEWAY_ENCRYPTION_KEY", testEncryptionKey)
	t.Cleanup(func() {
		os.Unsetenv("MICROGATEWAY_ENCRYPTION_KEY")
	})

	db, _ := setupPerformanceTestDB(t)

	ctx := context.Background()
	analytics.InitDefault(ctx, db)

	config := &grpc.Config{
		GRPCPort:   8080,
		GRPCHost:   "localhost",
		TLSEnabled: false,
		AuthToken:  "test-token",
	}

	server := grpc.NewControlServer(config, db)

	// Create a large analytics pulse to test batch performance
	eventCount := 500
	now := time.Now()
	events := make([]*pb.AnalyticsEvent, eventCount)

	for i := 0; i < eventCount; i++ {
		events[i] = &pb.AnalyticsEvent{
			RequestId:      fmt.Sprintf("req-%d", i),
			AppId:          uint32(i%10 + 1), // Cycle through 10 apps
			LlmId:          uint32(i%5 + 1),  // Cycle through 5 LLMs
			ModelName:      "gpt-4",
			Vendor:         "openai",
			Endpoint:       "/v1/chat/completions",
			StatusCode:     200,
			RequestTokens:  100,
			ResponseTokens: 150,
			TotalTokens:    250,
			Cost:           0.005,
			Timestamp:      timestamppb.New(now.Add(time.Duration(i) * time.Millisecond)),
			RequestBody:    `{"model": "gpt-4"}`,
			ResponseBody:   `{"choices": [...]}`,
		}
	}

	pulse := &pb.AnalyticsPulse{
		EdgeId:          "perf-test-edge",
		EdgeNamespace:   "test",
		SequenceNumber:  1,
		TotalRecords:    uint32(eventCount),
		AnalyticsEvents: events,
	}

	// Measure batch processing performance
	startTime := time.Now()
	response, err := server.SendAnalyticsPulse(context.Background(), pulse)
	batchProcessingTime := time.Since(startTime)

	require.NoError(t, err)
	assert.True(t, response.Success)
	assert.Equal(t, uint64(eventCount), response.ProcessedRecords)

	t.Logf("Batch processed %d events in %v", eventCount, batchProcessingTime)
	t.Logf("Processing rate: %.2f events/second", float64(eventCount)/batchProcessingTime.Seconds())

	// Wait for async analytics processing
	time.Sleep(500 * time.Millisecond)

	// Verify all records were created
	var chatRecordCount, proxyLogCount int64
	err = db.Model(&models.LLMChatRecord{}).Count(&chatRecordCount).Error
	require.NoError(t, err)
	assert.Equal(t, int64(eventCount), chatRecordCount)

	err = db.Model(&models.ProxyLog{}).Count(&proxyLogCount).Error
	require.NoError(t, err)
	assert.Equal(t, int64(eventCount), proxyLogCount)

	// Performance expectations:
	// - Should handle 500 events in under 2 seconds
	// - Should achieve at least 100 events/second processing rate
	assert.Less(t, batchProcessingTime.Seconds(), 2.0,
		"Large batch should process within 2 seconds (got %v)", batchProcessingTime)

	eventsPerSecond := float64(eventCount) / batchProcessingTime.Seconds()
	assert.Greater(t, eventsPerSecond, 100.0,
		"Should achieve at least 100 events/second (got %.2f)", eventsPerSecond)
}

func TestBatchAnalytics_Interface(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	db, _ := setupPerformanceTestDB(t)
	handler := analytics.NewDatabaseHandler(ctx, db)

	// Test that batch methods are available and working
	records := []*models.LLMChatRecord{
		{
			LLMID:           1,
			AppID:           1,
			Name:            "test-model",
			Vendor:          "test-vendor",
			TotalTokens:     100,
			Cost:            1000,
			Currency:        "USD",
			TimeStamp:       time.Now(),
			InteractionType: models.ProxyInteraction,
		},
	}

	logs := []*models.ProxyLog{
		{
			AppID:        1,
			UserID:       0,
			Vendor:       "test-vendor",
			ResponseCode: 200,
			TimeStamp:    time.Now(),
		},
	}

	// Test batch interface methods
	require.NotPanics(t, func() {
		handler.RecordChatRecordsBatch(records)
		handler.RecordProxyLogsBatch(logs)
	}, "Batch methods should not panic")

	// Wait for processing
	time.Sleep(100 * time.Millisecond)

	// Verify records were created
	var chatCount, proxyCount int64
	err := db.Model(&models.LLMChatRecord{}).Count(&chatCount).Error
	require.NoError(t, err)
	assert.Equal(t, int64(1), chatCount)

	err = db.Model(&models.ProxyLog{}).Count(&proxyCount).Error
	require.NoError(t, err)
	assert.Equal(t, int64(1), proxyCount)
}