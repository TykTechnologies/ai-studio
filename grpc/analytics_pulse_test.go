package grpc

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/analytics"
	"github.com/TykTechnologies/midsommar/v2/models"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupPulseTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(&models.LLMChatRecord{}, &models.ProxyLog{}, &models.EdgeInstance{})
	require.NoError(t, err)

	return db
}

func setupControlServer(t *testing.T, db *gorm.DB) *ControlServer {
	ctx := context.Background()
	analytics.InitDefault(ctx, db)

	// Set required environment variable for testing
	os.Setenv("MICROGATEWAY_ENCRYPTION_KEY", "12345678901234567890123456789012")

	config := &Config{
		GRPCPort:    8080,
		GRPCHost:    "localhost",
		TLSEnabled:  false,
		AuthToken:   "test-token",
	}

	return NewControlServer(config, db)
}

func TestSendAnalyticsPulse_BatchProcessing(t *testing.T) {
	db := setupPulseTestDB(t)
	server := setupControlServer(t, db)

	// Create test analytics pulse with multiple events
	now := time.Now()
	pulse := &pb.AnalyticsPulse{
		EdgeId:        "test-edge-1",
		EdgeNamespace: "test",
		SequenceNumber: 1,
		TotalRecords:  3,
		AnalyticsEvents: []*pb.AnalyticsEvent{
			{
				RequestId:      "req-1",
				AppId:          1,
				LlmId:          1,
				ModelName:      "gpt-4",
				Vendor:         "openai",
				Endpoint:       "/v1/chat/completions",
				StatusCode:     200,
				RequestTokens:  100,
				ResponseTokens: 150,
				TotalTokens:    250,
				Cost:           0.005, // $0.005
				Timestamp:      timestamppb.New(now),
				RequestBody:    `{"model": "gpt-4"}`,
				ResponseBody:   `{"choices": [...]}`,
			},
			{
				RequestId:      "req-2",
				AppId:          1,
				LlmId:          2,
				ModelName:      "claude-3-sonnet",
				Vendor:         "anthropic",
				Endpoint:       "/v1/messages",
				StatusCode:     200,
				RequestTokens:  80,
				ResponseTokens: 120,
				TotalTokens:    200,
				Cost:           0.004, // $0.004
				Timestamp:      timestamppb.New(now.Add(1 * time.Second)),
				RequestBody:    `{"model": "claude-3-sonnet"}`,
				ResponseBody:   `{"content": [...]}`,
			},
			{
				RequestId:      "req-3",
				AppId:          2,
				LlmId:          1,
				ModelName:      "gpt-4",
				Vendor:         "openai",
				Endpoint:       "/v1/chat/completions",
				StatusCode:     200,
				RequestTokens:  200,
				ResponseTokens: 300,
				TotalTokens:    500,
				Cost:           0.010, // $0.010
				Timestamp:      timestamppb.New(now.Add(2 * time.Second)),
				RequestBody:    `{"model": "gpt-4"}`,
				ResponseBody:   `{"choices": [...]}`,
			},
		},
		BudgetEvents: []*pb.BudgetUsageEvent{
			{
				AppId:     1,
				LlmId:     1,
				Cost:      0.005,
				Timestamp: timestamppb.New(now),
			},
		},
		ProxySummaries: []*pb.ProxyLogSummary{
			{
				AppId:        1,
				Vendor:       "openai",
				RequestCount: 100,
				TotalCost:    0.50,
			},
		},
	}

	// Process the analytics pulse
	startTime := time.Now()
	response, err := server.SendAnalyticsPulse(context.Background(), pulse)
	processingTime := time.Since(startTime)

	require.NoError(t, err)
	assert.True(t, response.Success)
	assert.Equal(t, "Analytics pulse processed successfully", response.Message)
	assert.Equal(t, uint64(5), response.ProcessedRecords) // 3 analytics + 1 budget + 1 proxy summary

	t.Logf("Batch processing completed in %v", processingTime)

	// Wait for async analytics processing
	time.Sleep(200 * time.Millisecond)

	// Verify that all analytics events were processed in batch
	var chatRecordCount int64
	err = db.Model(&models.LLMChatRecord{}).Count(&chatRecordCount).Error
	require.NoError(t, err)
	assert.Equal(t, int64(3), chatRecordCount, "All 3 analytics events should create chat records")

	var proxyLogCount int64
	err = db.Model(&models.ProxyLog{}).Count(&proxyLogCount).Error
	require.NoError(t, err)
	assert.Equal(t, int64(3), proxyLogCount, "All 3 analytics events should create proxy logs")

	// Verify the data integrity of batch-processed records
	var chatRecords []models.LLMChatRecord
	err = db.Find(&chatRecords).Error
	require.NoError(t, err)

	var proxyLogs []models.ProxyLog
	err = db.Find(&proxyLogs).Error
	require.NoError(t, err)

	// Check that costs were properly stored in dollars
	expectedCosts := []float64{0.005, 0.004, 0.01} // Stored in dollars
	for i, record := range chatRecords {
		assert.Equal(t, expectedCosts[i], record.Cost)
		assert.Equal(t, models.ProxyInteraction, record.InteractionType)
	}

	// Performance assertion: batch processing should be fast
	assert.Less(t, processingTime.Milliseconds(), int64(500),
		"Batch processing should complete quickly (got %v)", processingTime)
}

func TestSendAnalyticsPulse_EmptyPulse(t *testing.T) {
	db := setupPulseTestDB(t)
	server := setupControlServer(t, db)

	// Create empty analytics pulse
	pulse := &pb.AnalyticsPulse{
		EdgeId:         "test-edge-1",
		EdgeNamespace:  "test",
		SequenceNumber: 1,
		TotalRecords:   0,
		AnalyticsEvents: []*pb.AnalyticsEvent{},
		BudgetEvents:   []*pb.BudgetUsageEvent{},
		ProxySummaries: []*pb.ProxyLogSummary{},
	}

	// Process the empty pulse
	response, err := server.SendAnalyticsPulse(context.Background(), pulse)

	require.NoError(t, err)
	assert.True(t, response.Success)
	assert.Equal(t, uint64(0), response.ProcessedRecords)

	// Verify no records were created
	var chatRecordCount, proxyLogCount int64
	err = db.Model(&models.LLMChatRecord{}).Count(&chatRecordCount).Error
	require.NoError(t, err)
	assert.Equal(t, int64(0), chatRecordCount)

	err = db.Model(&models.ProxyLog{}).Count(&proxyLogCount).Error
	require.NoError(t, err)
	assert.Equal(t, int64(0), proxyLogCount)
}

func BenchmarkSendAnalyticsPulse_BatchProcessing(b *testing.B) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(b, err)

	err = db.AutoMigrate(&models.LLMChatRecord{}, &models.ProxyLog{}, &models.EdgeInstance{})
	require.NoError(b, err)

	ctx := context.Background()
	analytics.InitDefault(ctx, db)

	// Set required environment variable for testing
	os.Setenv("MICROGATEWAY_ENCRYPTION_KEY", "12345678901234567890123456789012")

	config := &Config{
		GRPCPort:    8080,
		GRPCHost:    "localhost",
		TLSEnabled:  false,
		AuthToken:   "test-token",
	}

	server := NewControlServer(config, db)

	// Create a pulse with many analytics events
	createPulse := func(eventCount int) *pb.AnalyticsPulse {
		events := make([]*pb.AnalyticsEvent, eventCount)
		now := time.Now()

		for i := 0; i < eventCount; i++ {
			events[i] = &pb.AnalyticsEvent{
				RequestId:      fmt.Sprintf("req-%d", i),
				AppId:          uint32(i%5 + 1), // Cycle through 5 apps
				LlmId:          uint32(i%3 + 1), // Cycle through 3 LLMs
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

		return &pb.AnalyticsPulse{
			EdgeId:          "bench-edge",
			EdgeNamespace:   "bench",
			SequenceNumber:  1,
			TotalRecords:    uint32(eventCount),
			AnalyticsEvents: events,
		}
	}

	b.Run("SmallBatch_10", func(b *testing.B) {
		pulse := createPulse(10)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := server.SendAnalyticsPulse(context.Background(), pulse)
			require.NoError(b, err)
		}
	})

	b.Run("MediumBatch_100", func(b *testing.B) {
		pulse := createPulse(100)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := server.SendAnalyticsPulse(context.Background(), pulse)
			require.NoError(b, err)
		}
	})

	b.Run("LargeBatch_1000", func(b *testing.B) {
		pulse := createPulse(1000)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := server.SendAnalyticsPulse(context.Background(), pulse)
			require.NoError(b, err)
		}
	})
}