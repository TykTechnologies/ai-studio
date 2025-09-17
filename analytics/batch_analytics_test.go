package analytics

import (
	"context"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupBatchTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(&models.LLMChatRecord{}, &models.ProxyLog{})
	require.NoError(t, err)

	return db
}

func TestDatabaseHandler_RecordChatRecordsBatch(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	db := setupBatchTestDB(t)
	handler := NewDatabaseHandler(ctx, db)

	// Create test chat records
	now := time.Now()
	records := []*models.LLMChatRecord{
		{
			LLMID:           1,
			AppID:           1,
			Name:            "gpt-4",
			Vendor:          "openai",
			TotalTokens:     1000,
			PromptTokens:    500,
			ResponseTokens:  500,
			Cost:            50000, // $5.00 in cents
			Currency:        "USD",
			TimeStamp:       now,
			InteractionType: models.ProxyInteraction,
		},
		{
			LLMID:           2,
			AppID:           1,
			Name:            "claude-3-sonnet",
			Vendor:          "anthropic",
			TotalTokens:     800,
			PromptTokens:    400,
			ResponseTokens:  400,
			Cost:            40000, // $4.00 in cents
			Currency:        "USD",
			TimeStamp:       now,
			InteractionType: models.ProxyInteraction,
		},
	}

	// Test batch recording
	handler.RecordChatRecordsBatch(records)

	// Wait for processing
	time.Sleep(100 * time.Millisecond)

	// Verify records were created
	var count int64
	err := db.Model(&models.LLMChatRecord{}).Count(&count).Error
	require.NoError(t, err)
	assert.Equal(t, int64(2), count)

	// Verify record details
	var savedRecords []models.LLMChatRecord
	err = db.Find(&savedRecords).Error
	require.NoError(t, err)
	assert.Len(t, savedRecords, 2)

	// Check that both records were saved correctly
	assert.Equal(t, "gpt-4", savedRecords[0].Name)
	assert.Equal(t, "claude-3-sonnet", savedRecords[1].Name)
	assert.Equal(t, float64(50000), savedRecords[0].Cost)
	assert.Equal(t, float64(40000), savedRecords[1].Cost)
}

func TestDatabaseHandler_RecordProxyLogsBatch(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	db := setupBatchTestDB(t)
	handler := NewDatabaseHandler(ctx, db)

	// Create test proxy logs
	now := time.Now()
	logs := []*models.ProxyLog{
		{
			AppID:        1,
			UserID:       0,
			Vendor:       "openai",
			RequestBody:  `{"model": "gpt-4", "messages": [...]}`,
			ResponseBody: `{"choices": [...]}`,
			ResponseCode: 200,
			TimeStamp:    now,
		},
		{
			AppID:        1,
			UserID:       0,
			Vendor:       "anthropic",
			RequestBody:  `{"model": "claude-3-sonnet", "messages": [...]}`,
			ResponseBody: `{"content": [...]}`,
			ResponseCode: 200,
			TimeStamp:    now,
		},
	}

	// Test batch recording
	handler.RecordProxyLogsBatch(logs)

	// Wait for processing
	time.Sleep(100 * time.Millisecond)

	// Verify logs were created
	var count int64
	err := db.Model(&models.ProxyLog{}).Count(&count).Error
	require.NoError(t, err)
	assert.Equal(t, int64(2), count)

	// Verify log details
	var savedLogs []models.ProxyLog
	err = db.Find(&savedLogs).Error
	require.NoError(t, err)
	assert.Len(t, savedLogs, 2)

	// Check that both logs were saved correctly
	assert.Equal(t, "openai", savedLogs[0].Vendor)
	assert.Equal(t, "anthropic", savedLogs[1].Vendor)
	assert.Equal(t, 200, savedLogs[0].ResponseCode)
	assert.Equal(t, 200, savedLogs[1].ResponseCode)
}

func TestDatabaseHandler_EmptyBatches(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	db := setupBatchTestDB(t)
	handler := NewDatabaseHandler(ctx, db)

	// Test with empty slices
	handler.RecordChatRecordsBatch([]*models.LLMChatRecord{})
	handler.RecordProxyLogsBatch([]*models.ProxyLog{})

	// Verify no records were created
	var chatCount, proxyCount int64
	err := db.Model(&models.LLMChatRecord{}).Count(&chatCount).Error
	require.NoError(t, err)
	assert.Equal(t, int64(0), chatCount)

	err = db.Model(&models.ProxyLog{}).Count(&proxyCount).Error
	require.NoError(t, err)
	assert.Equal(t, int64(0), proxyCount)
}

func TestDatabaseHandler_LargeBatch(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	db := setupBatchTestDB(t)
	handler := NewDatabaseHandler(ctx, db)

	// Create a large batch (200 records)
	now := time.Now()
	records := make([]*models.LLMChatRecord, 200)
	for i := 0; i < 200; i++ {
		records[i] = &models.LLMChatRecord{
			LLMID:           uint(i%5 + 1), // Cycle through 5 LLMs
			AppID:           1,
			Name:            "test-model",
			Vendor:          "test-vendor",
			TotalTokens:     100,
			PromptTokens:    50,
			ResponseTokens:  50,
			Cost:            1000, // $0.10 in cents
			Currency:        "USD",
			TimeStamp:       now.Add(time.Duration(i) * time.Second),
			InteractionType: models.ProxyInteraction,
		}
	}

	// Test large batch recording
	startTime := time.Now()
	handler.RecordChatRecordsBatch(records)

	// Wait for processing
	time.Sleep(200 * time.Millisecond)

	processingTime := time.Since(startTime)
	t.Logf("Large batch processing time: %v", processingTime)

	// Verify all records were created
	var count int64
	err := db.Model(&models.LLMChatRecord{}).Count(&count).Error
	require.NoError(t, err)
	assert.Equal(t, int64(200), count)

	// Performance assertion: batch should be significantly faster than individual inserts
	// This is more of a regression test to ensure batching remains efficient
	assert.Less(t, processingTime.Milliseconds(), int64(1000), "Batch processing should complete within 1 second")
}

func BenchmarkDatabaseHandler_BatchVsIndividual(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(b, err)

	err = db.AutoMigrate(&models.LLMChatRecord{}, &models.ProxyLog{})
	require.NoError(b, err)

	handler := NewDatabaseHandler(ctx, db)

	// Create test data
	now := time.Now()
	createRecord := func(i int) *models.LLMChatRecord {
		return &models.LLMChatRecord{
			LLMID:           uint(i%5 + 1),
			AppID:           1,
			Name:            "test-model",
			Vendor:          "test-vendor",
			TotalTokens:     100,
			PromptTokens:    50,
			ResponseTokens:  50,
			Cost:            1000,
			Currency:        "USD",
			TimeStamp:       now.Add(time.Duration(i) * time.Second),
			InteractionType: models.ProxyInteraction,
		}
	}

	batchSize := 50

	b.Run("Individual", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			records := make([]*models.LLMChatRecord, batchSize)
			for j := 0; j < batchSize; j++ {
				records[j] = createRecord(i*batchSize + j)
			}

			// Process individually
			for _, record := range records {
				handler.RecordChatRecord(record)
			}
		}
	})

	b.Run("Batch", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			records := make([]*models.LLMChatRecord, batchSize)
			for j := 0; j < batchSize; j++ {
				records[j] = createRecord(i*batchSize + j)
			}

			// Process as batch
			handler.RecordChatRecordsBatch(records)
		}
	})
}