package analytics

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// setupBatchTestDB opens a private in-memory database on one connection.
// The handler writes on its own goroutine: with plain ":memory:" every pooled
// connection is a separate, empty database, so its writes could land where
// the test never reads them.
func setupBatchTestDB(t *testing.T) *gorm.DB {
	name := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	db, err := gorm.Open(sqlite.Open("file:"+name+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetConnMaxLifetime(0)
	t.Cleanup(func() { sqlDB.Close() })

	err = db.AutoMigrate(&models.LLMChatRecord{}, &models.ProxyLog{})
	require.NoError(t, err)

	return db
}

// waitForRows waits until model's table holds want rows: the handler's
// batch writes are asynchronous, and a fixed sleep was too short on loaded
// CI runners.
func waitForRows(t *testing.T, db *gorm.DB, model interface{}, want int64) {
	t.Helper()
	var count int64
	require.Eventually(t, func() bool {
		return db.Model(model).Count(&count).Error == nil && count >= want
	}, 10*time.Second, 10*time.Millisecond, "expected %d rows, have %d", want, count)
	require.Equal(t, want, count)
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
	handler.RecordChatRecordsBatch(ctx, records)

	waitForRows(t, db, &models.LLMChatRecord{}, 2)

	// Verify record details
	var savedRecords []models.LLMChatRecord
	err := db.Order("id").Find(&savedRecords).Error
	require.NoError(t, err)
	require.Len(t, savedRecords, 2)

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
	handler.RecordProxyLogsBatch(ctx, logs)

	waitForRows(t, db, &models.ProxyLog{}, 2)

	// Verify log details
	var savedLogs []models.ProxyLog
	err := db.Order("id").Find(&savedLogs).Error
	require.NoError(t, err)
	require.Len(t, savedLogs, 2)

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
	handler.RecordChatRecordsBatch(ctx, []*models.LLMChatRecord{})
	handler.RecordProxyLogsBatch(ctx, []*models.ProxyLog{})

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
	handler.RecordChatRecordsBatch(ctx, records)

	waitForRows(t, db, &models.LLMChatRecord{}, 200)

	processingTime := time.Since(startTime)
	t.Logf("Large batch processing time: %v", processingTime)

	// Performance assertion: batch should be significantly faster than individual inserts
	// This is more of a regression test to ensure batching remains efficient
	// Generous for loaded CI runners; row-by-row inserts of 200 records with
	// a flush each would still blow well past it.
	assert.Less(t, processingTime.Milliseconds(), int64(5000), "Batch processing should complete within 5 seconds")
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
				handler.RecordChatRecord(ctx, record)
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
			handler.RecordChatRecordsBatch(ctx, records)
		}
	})
}