package analytics

import (
	"context"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// TestAsyncBatchProcessing tests that the async batch processing works correctly
func TestAsyncBatchProcessing(t *testing.T) {
	// Setup in-memory SQLite database for testing
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent), // Reduce log noise
	})
	if err != nil {
		t.Fatalf("Failed to connect to test database: %v", err)
	}

	// Create tables
	err = db.AutoMigrate(
		&models.LLMChatRecord{},
		&models.ProxyLog{},
	)
	if err != nil {
		t.Fatalf("Failed to migrate test database: %v", err)
	}

	// Create context and analytics handler
	ctx := context.Background()
	handler := NewDatabaseHandler(ctx, db)
	defer handler.Stop()

	// Test data
	testTime := time.Now()

	// Test batch chat records
	chatRecords := []*models.LLMChatRecord{
		{
			LLMID:          1,
			AppID:          1,
			Name:           "test-model-1",
			Vendor:         "test-vendor",
			TotalTokens:    100,
			PromptTokens:   50,
			ResponseTokens: 50,
			Cost:           1000, // $0.10 in cents
			TimeStamp:      testTime,
		},
		{
			LLMID:          2,
			AppID:          1,
			Name:           "test-model-2",
			Vendor:         "test-vendor",
			TotalTokens:    200,
			PromptTokens:   100,
			ResponseTokens: 100,
			Cost:           2000, // $0.20 in cents
			TimeStamp:      testTime,
		},
	}

	// Test batch proxy logs
	proxyLogs := []*models.ProxyLog{
		{
			AppID:        1,
			UserID:       1,
			Vendor:       "test-vendor",
			RequestBody:  `{"model": "test", "prompt": "hello"}`,
			ResponseBody: `{"choices": [{"text": "world"}]}`,
			ResponseCode: 200,
			TimeStamp:    testTime,
		},
	}

	// Call async batch methods - these should return immediately
	start := time.Now()
	handler.RecordChatRecordsBatch(chatRecords)
	handler.RecordProxyLogsBatch(proxyLogs)
	callDuration := time.Since(start)

	// The calls should return very quickly (non-blocking)
	if callDuration > 100*time.Millisecond {
		t.Errorf("Batch methods took too long: %v (expected < 100ms)", callDuration)
	}

	// Wait for async processing to complete
	time.Sleep(500 * time.Millisecond)

	// Verify data was inserted
	var chatRecordCount int64
	err = db.Model(&models.LLMChatRecord{}).Count(&chatRecordCount).Error
	if err != nil {
		t.Fatalf("Failed to count chat records: %v", err)
	}
	if chatRecordCount != 2 {
		t.Errorf("Expected 2 chat records, got %d", chatRecordCount)
	}

	var proxyLogCount int64
	err = db.Model(&models.ProxyLog{}).Count(&proxyLogCount).Error
	if err != nil {
		t.Fatalf("Failed to count proxy logs: %v", err)
	}
	if proxyLogCount != 1 {
		t.Errorf("Expected 1 proxy log, got %d", proxyLogCount)
	}

	t.Logf("Test passed: Async batch processing completed successfully in %v", callDuration)
}

// TestBatchMethodsWithEmptyData tests that batch methods handle empty data gracefully
func TestBatchMethodsWithEmptyData(t *testing.T) {
	// Setup in-memory SQLite database for testing
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("Failed to connect to test database: %v", err)
	}

	// Create context and analytics handler
	ctx := context.Background()
	handler := NewDatabaseHandler(ctx, db)
	defer handler.Stop()

	// Test with empty data - should not cause any issues
	handler.RecordChatRecordsBatch([]*models.LLMChatRecord{})
	handler.RecordProxyLogsBatch([]*models.ProxyLog{})
	handler.RecordChatRecordsBatch(nil)
	handler.RecordProxyLogsBatch(nil)

	t.Log("Empty data test passed")
}