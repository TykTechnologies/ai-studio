package proxy

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/TykTechnologies/midsommar/v2/analytics"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
)

// setupDB now enables WAL mode + busy timeout for in-memory SQLite to reduce "database is locked" errors.
func setupDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{
		// This can reduce some locking overhead by skipping the default transaction wrapper per statement.
		SkipDefaultTransaction: true,
	})
	require.NoError(t, err)

	// Enable WAL + set busy_timeout so concurrent writes won't fail with "locked".
	sqlDB, err := db.DB()
	require.NoError(t, err)
	_, _ = sqlDB.Exec("PRAGMA journal_mode = WAL;")
	_, _ = sqlDB.Exec("PRAGMA busy_timeout = 5000;")
	// Optionally reduce sync overhead further
	_, _ = sqlDB.Exec("PRAGMA synchronous = NORMAL;")

	// Migrate main models.
	err = models.InitModels(db)
	require.NoError(t, err)

	// Migrate analytics tables explicitly.
	err = db.AutoMigrate(
		&models.LLMChatRecord{},
		&models.LLMChatLogEntry{},
		&models.ToolCallRecord{},
		&models.ProxyLog{},
		&models.Notification{}, // Add notifications table
	)
	require.NoError(t, err)

	return db
}

// setupTest initializes DB, does migrations, and starts analytics in one place.
func setupTest(t *testing.T) (*gorm.DB, context.CancelFunc) {
	db := setupDB(t)

	// Now start analytics AFTER migrations, ensuring the table is ready.
	ctx, cancel := context.WithCancel(context.Background())
	analytics.StartRecording(ctx, db)

	return db, cancel
}

// tearDownTest ensures the analytics background goroutine does not outlive the DB.
func tearDownTest(db *gorm.DB, cancel context.CancelFunc) {
	// Wait for any pending analytics to complete
	time.Sleep(200 * time.Millisecond)
	cancel()                           // stop analytics goroutine
	time.Sleep(100 * time.Millisecond) // Give analytics goroutine time to clean up
	// Optionally close underlying DB connection:
	sqlDB, err := db.DB()
	if err == nil {
		_ = sqlDB.Close()
	}
}

// waitForAnalytics ensures we have at least `expectedCount` LLMChatRecords.
func waitForAnalytics(t *testing.T, db *gorm.DB, expectedCount int64) {
	deadline := time.Now().Add(5000 * time.Millisecond) // Increase timeout to 5 seconds
	for time.Now().Before(deadline) {
		var count int64
		var err error
		for i := 0; i < 5; i++ { // retry a few times if locked
			err = db.Model(&models.LLMChatRecord{}).Count(&count).Error
			if err == nil {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		if err != nil {
			continue // skip this round if still locked
		}

		if count >= expectedCount {
			time.Sleep(200 * time.Millisecond) // extra wait to ensure all operations complete
			return
		}
		time.Sleep(200 * time.Millisecond) // Increase delay between checks
	}
	t.Fatalf("Timeout waiting for analytics records. Expected at least: %d", expectedCount)
}

// waitUntilIdle waits until the analytics goroutine has not produced any *new*
// LLMChatRecords for ~200ms, meaning it's idle. This helps avoid "table is locked"
// if we do a DB Update concurrent with an ongoing analytics insert.
func waitUntilIdle(t *testing.T, db *gorm.DB) {
	var lastCount int64
	var stableRounds int
	timeout := time.NewTimer(5 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer timeout.Stop()
	defer ticker.Stop()

	// capture initial count
	for i := 0; i < 5; i++ { // retry a few times if locked
		err := db.Model(&models.LLMChatRecord{}).Count(&lastCount).Error
		if err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	for {
		select {
		case <-timeout.C:
			t.Fatalf("Timeout waiting for DB to go idle (analytics).")
		case <-ticker.C:
			var curCount int64
			var err error
			for i := 0; i < 5; i++ { // retry a few times if locked
				err = db.Model(&models.LLMChatRecord{}).Count(&curCount).Error
				if err == nil {
					break
				}
				time.Sleep(100 * time.Millisecond)
			}
			if err != nil {
				continue // skip this round if still locked
			}
			if curCount == lastCount {
				stableRounds++
				// If stable for ~2 intervals (200ms), assume idle.
				if stableRounds >= 2 {
					time.Sleep(200 * time.Millisecond) // extra wait to ensure all operations complete
					return
				}
			} else {
				// changed, reset
				stableRounds = 0
				lastCount = curCount
			}
		}
	}
}

// waitForRecordWithCost waits for a record to be written with a non-zero cost and returns the record
func waitForRecordWithCost(t *testing.T, db *gorm.DB) *models.LLMChatRecord {
	deadline := time.Now().Add(5000 * time.Millisecond) // Increase timeout to 5 seconds
	for time.Now().Before(deadline) {
		var record models.LLMChatRecord
		var err error
		for i := 0; i < 5; i++ { // retry a few times if locked
			err = db.First(&record).Error
			if err == nil {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		if err != nil {
			continue // skip this round if still locked
		}

		t.Logf("Found record: cost=%f prompt_tokens=%d response_tokens=%d timestamp=%v",
			record.Cost, record.PromptTokens, record.ResponseTokens, record.TimeStamp)

		if record.Cost > 0 {
			// Wait a bit to ensure the record is fully committed
			time.Sleep(200 * time.Millisecond) // Increase delay
			return &record
		}
		time.Sleep(200 * time.Millisecond) // Increase delay between checks
	}
	t.Fatal("Timeout waiting for record with cost")
	return nil
}

// waitForSpendingUpdate waits for spending to be updated to an expected value
func waitForSpendingUpdate(t *testing.T, budgetService *services.BudgetService, appID uint, llmID uint, start, end time.Time, expectedSpent float64) {
	deadline := time.Now().Add(5000 * time.Millisecond) // Increase timeout to 5 seconds
	for time.Now().Before(deadline) {
		budgetService.ClearCache()

		var appSpent, llmSpent float64
		var err error

		// Retry app spending query if locked
		for i := 0; i < 5; i++ {
			appSpent, err = budgetService.GetMonthlySpending(appID, start, end)
			if err == nil {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		if err != nil {
			continue // skip this round if still locked
		}

		// Retry llm spending query if locked
		for i := 0; i < 5; i++ {
			llmSpent, err = budgetService.GetLLMMonthlySpending(llmID, start, end)
			if err == nil {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		if err != nil {
			continue // skip this round if still locked
		}

		t.Logf("Current spending - app: %.2f, llm: %.2f (expected: %.2f) [start=%v, end=%v]",
			appSpent, llmSpent, expectedSpent, start, end)

		if appSpent == expectedSpent && llmSpent == expectedSpent {
			time.Sleep(200 * time.Millisecond) // extra wait to ensure all operations complete
			return
		}

		time.Sleep(200 * time.Millisecond) // Increase delay between checks
	}
	t.Fatalf("Timeout waiting for spending to update to %.2f", expectedSpent)
}
