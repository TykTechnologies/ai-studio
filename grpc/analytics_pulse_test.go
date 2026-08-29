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

	// A sqlite ":memory:" database belongs to its connection, so a pooled
	// second connection opens an empty one and the analytics worker writes
	// into a database with no tables. Pin the pool to a single connection.
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	err = db.AutoMigrate(&models.LLMChatRecord{}, &models.ProxyLog{}, &models.EdgeInstance{},
		&models.ComplianceEvent{}, &models.ToolCallRecord{})
	require.NoError(t, err)

	return db
}

func setupControlServer(t *testing.T, db *gorm.DB) *ControlServer {
	ctx := context.Background()
	// Reset and reinitialize to ensure this test's DB is used
	analytics.ResetHandler()
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

	// LLMID must be propagated from the pulse to the ProxyLog so that
	// GetProxyLogsForLLM (which filters on llm_id) can return edge-sourced
	// logs in the LLM detail view. Without this, no logs show up at all.
	expectedLLMIDs := map[string]uint{"openai": 1, "anthropic": 2}
	for _, pl := range proxyLogs {
		want, ok := expectedLLMIDs[pl.Vendor]
		require.True(t, ok, "unexpected vendor %q in proxy log", pl.Vendor)
		assert.Equal(t, want, pl.LLMID, "ProxyLog for vendor %q should carry LLMID from pulse", pl.Vendor)
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

func TestSendAnalyticsPulse_ComplianceEvents(t *testing.T) {
	db := setupPulseTestDB(t)
	server := setupControlServer(t, db)

	now := time.Now()
	pulse := &pb.AnalyticsPulse{
		EdgeId:         "test-edge-compliance",
		EdgeNamespace:  "test",
		SequenceNumber: 1,
		TotalRecords:   3,
		ComplianceEvents: []*pb.ComplianceEventProto{
			{
				AppId:       1,
				UserId:      10,
				LlmId:       5,
				FilterName:  "pii-filter",
				FilterScope: "proxy_request",
				EventType:   "pii_redacted",
				Severity:    "warning",
				Description: "SSN pattern found and redacted",
				Metadata:    `{"pattern":"ssn","count":2}`,
				Vendor:      "openai",
				ModelName:   "gpt-4",
				Timestamp:   timestamppb.New(now),
			},
			{
				AppId:       1,
				UserId:      10,
				LlmId:       5,
				FilterName:  "tone-filter",
				FilterScope: "proxy_request",
				EventType:   "content_rewritten",
				Severity:    "info",
				Description: "Tone adjusted to professional",
				Vendor:      "openai",
				ModelName:   "gpt-4",
				Timestamp:   timestamppb.New(now.Add(1 * time.Second)),
			},
			{
				AppId:       2,
				UserId:      20,
				LlmId:       8,
				FilterName:  "pii-filter",
				FilterScope: "proxy_response",
				EventType:   "pii_redacted",
				Severity:    "critical",
				Description: "Credit card number detected in response",
				Metadata:    `{"pattern":"credit_card"}`,
				Vendor:      "anthropic",
				ModelName:   "claude-3",
				Timestamp:   timestamppb.New(now.Add(2 * time.Second)),
			},
		},
	}

	response, err := server.SendAnalyticsPulse(context.Background(), pulse)

	require.NoError(t, err)
	assert.True(t, response.Success)
	assert.Equal(t, uint64(3), response.ProcessedRecords)

	// Wait for async analytics processing
	time.Sleep(200 * time.Millisecond)

	// Verify compliance events were stored in the database
	var complianceCount int64
	err = db.Model(&models.ComplianceEvent{}).Count(&complianceCount).Error
	require.NoError(t, err)
	assert.Equal(t, int64(3), complianceCount, "All 3 compliance events should be stored")

	// Verify the data integrity of stored events
	var events []models.ComplianceEvent
	err = db.Order("time_stamp ASC").Find(&events).Error
	require.NoError(t, err)
	require.Len(t, events, 3)

	// First event
	assert.Equal(t, uint(1), events[0].AppID)
	assert.Equal(t, uint(10), events[0].UserID)
	assert.Equal(t, uint(5), events[0].LLMID)
	assert.Equal(t, "pii-filter", events[0].FilterName)
	assert.Equal(t, "proxy_request", events[0].FilterScope)
	assert.Equal(t, "pii_redacted", events[0].EventType)
	assert.Equal(t, "warning", events[0].Severity)
	assert.Equal(t, "SSN pattern found and redacted", events[0].Description)
	assert.Equal(t, `{"pattern":"ssn","count":2}`, events[0].Metadata)
	assert.Equal(t, "openai", events[0].Vendor)
	assert.Equal(t, "gpt-4", events[0].ModelName)

	// Third event - different app, different scope
	assert.Equal(t, uint(2), events[2].AppID)
	assert.Equal(t, "proxy_response", events[2].FilterScope)
	assert.Equal(t, "critical", events[2].Severity)
	assert.Equal(t, "anthropic", events[2].Vendor)
}

func TestSendAnalyticsPulse_ComplianceEventsOnly(t *testing.T) {
	db := setupPulseTestDB(t)
	server := setupControlServer(t, db)

	// Pulse with ONLY compliance events (no analytics, no budget, no proxy)
	pulse := &pb.AnalyticsPulse{
		EdgeId:         "test-edge-compliance-only",
		EdgeNamespace:  "test",
		SequenceNumber: 1,
		TotalRecords:   1,
		ComplianceEvents: []*pb.ComplianceEventProto{
			{
				AppId:     1,
				EventType: "solo_event",
				Severity:  "info",
				Timestamp: timestamppb.Now(),
			},
		},
	}

	response, err := server.SendAnalyticsPulse(context.Background(), pulse)
	require.NoError(t, err)
	assert.True(t, response.Success)
	assert.Equal(t, uint64(1), response.ProcessedRecords)

	time.Sleep(200 * time.Millisecond)

	var count int64
	db.Model(&models.ComplianceEvent{}).Count(&count)
	assert.Equal(t, int64(1), count)

	// No other records should be created
	var chatCount, proxyCount int64
	db.Model(&models.LLMChatRecord{}).Count(&chatCount)
	db.Model(&models.ProxyLog{}).Count(&proxyCount)
	assert.Equal(t, int64(0), chatCount)
	assert.Equal(t, int64(0), proxyCount)
}

func TestSendAnalyticsPulse_ComplianceEventsWithZeroFields(t *testing.T) {
	db := setupPulseTestDB(t)
	server := setupControlServer(t, db)

	// Events with minimal fields — should still store without error
	pulse := &pb.AnalyticsPulse{
		EdgeId:         "test-edge-minimal",
		EdgeNamespace:  "test",
		SequenceNumber: 1,
		TotalRecords:   1,
		ComplianceEvents: []*pb.ComplianceEventProto{
			{
				// Only event_type and severity set, everything else zero/empty
				EventType: "minimal",
				Severity:  "info",
				Timestamp: timestamppb.Now(),
			},
		},
	}

	response, err := server.SendAnalyticsPulse(context.Background(), pulse)
	require.NoError(t, err)
	assert.True(t, response.Success)

	time.Sleep(200 * time.Millisecond)

	var events []models.ComplianceEvent
	db.Find(&events)
	require.Len(t, events, 1)
	assert.Equal(t, "minimal", events[0].EventType)
	assert.Equal(t, uint(0), events[0].AppID)
	assert.Equal(t, uint(0), events[0].UserID)
	assert.Equal(t, "", events[0].FilterName)
	assert.Equal(t, "", events[0].Vendor)
}

func TestSendAnalyticsPulse_EmptyComplianceEvents(t *testing.T) {
	db := setupPulseTestDB(t)
	server := setupControlServer(t, db)

	// Pulse with empty compliance_events list (not nil, but empty)
	pulse := &pb.AnalyticsPulse{
		EdgeId:           "test-edge-empty-compliance",
		EdgeNamespace:    "test",
		SequenceNumber:   1,
		TotalRecords:     0,
		ComplianceEvents: []*pb.ComplianceEventProto{},
	}

	response, err := server.SendAnalyticsPulse(context.Background(), pulse)
	require.NoError(t, err)
	assert.True(t, response.Success)
	assert.Equal(t, uint64(0), response.ProcessedRecords)

	var count int64
	db.Model(&models.ComplianceEvent{}).Count(&count)
	assert.Equal(t, int64(0), count)
}

func TestSendAnalyticsPulse_MixedAnalyticsAndCompliance(t *testing.T) {
	db := setupPulseTestDB(t)
	server := setupControlServer(t, db)

	now := time.Now()
	pulse := &pb.AnalyticsPulse{
		EdgeId:         "test-edge-mixed",
		EdgeNamespace:  "test",
		SequenceNumber: 1,
		TotalRecords:   3,
		AnalyticsEvents: []*pb.AnalyticsEvent{
			{
				RequestId:      "req-001",
				AppId:          1,
				LlmId:          1,
				ModelName:      "gpt-4",
				Vendor:         "openai",
				StatusCode:     200,
				TotalTokens:    100,
				RequestTokens:  80,
				ResponseTokens: 20,
				Cost:           0.002,
				Timestamp:      timestamppb.New(now),
			},
		},
		ComplianceEvents: []*pb.ComplianceEventProto{
			{
				AppId:       1,
				UserId:      10,
				LlmId:       1,
				FilterName:  "pii-filter",
				FilterScope: "proxy_request",
				EventType:   "pii_redacted",
				Severity:    "warning",
				Description: "PII detected in request",
				Vendor:      "openai",
				ModelName:   "gpt-4",
				Timestamp:   timestamppb.New(now),
			},
		},
		BudgetEvents: []*pb.BudgetUsageEvent{
			{
				AppId:     1,
				LlmId:     1,
				Cost:      0.002,
				Timestamp: timestamppb.New(now),
			},
		},
	}

	response, err := server.SendAnalyticsPulse(context.Background(), pulse)

	require.NoError(t, err)
	assert.True(t, response.Success)
	assert.Equal(t, uint64(3), response.ProcessedRecords) // 1 analytics + 1 compliance + 1 budget

	// Wait for async processing
	time.Sleep(200 * time.Millisecond)

	// Verify both types of records were stored
	var chatRecordCount int64
	db.Model(&models.LLMChatRecord{}).Count(&chatRecordCount)
	assert.Equal(t, int64(1), chatRecordCount)

	var complianceCount int64
	db.Model(&models.ComplianceEvent{}).Count(&complianceCount)
	assert.Equal(t, int64(1), complianceCount)
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