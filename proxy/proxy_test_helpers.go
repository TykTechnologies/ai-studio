package proxy

import (
	"context"
	"strings"
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
		SkipDefaultTransaction: true,
		PrepareStmt:            true, // Enable prepared statement cache
	})
	require.NoError(t, err)

	// Enable WAL + set busy_timeout so concurrent writes won't fail with "locked".
	sqlDB, err := db.DB()
	require.NoError(t, err)
	_, _ = sqlDB.Exec("PRAGMA journal_mode = WAL;")
	_, _ = sqlDB.Exec("PRAGMA busy_timeout = 5000;")
	_, _ = sqlDB.Exec("PRAGMA synchronous = NORMAL;")
	_, _ = sqlDB.Exec("PRAGMA cache_size = 2000;")       // Increase cache size
	_, _ = sqlDB.Exec("PRAGMA temp_store = MEMORY;")     // Store temp tables in memory
	_, _ = sqlDB.Exec("PRAGMA mmap_size = 30000000000;") // Use memory-mapped I/O

	// Migrate main models.
	err = models.InitModels(db)
	require.NoError(t, err)

	// Migrate analytics tables explicitly.
	err = db.AutoMigrate(
		&models.LLMChatRecord{},
		&models.LLMChatLogEntry{},
		&models.ToolCallRecord{},
		&models.ProxyLog{},
		&models.Notification{},
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
	time.Sleep(25 * time.Millisecond)
	cancel()
	time.Sleep(25 * time.Millisecond)
	sqlDB, err := db.DB()
	if err == nil {
		_ = sqlDB.Close()
	}
}

// waitForAnalytics ensures we have at least `expectedCount` LLMChatRecords.
func waitForAnalytics(t *testing.T, db *gorm.DB, expectedCount int64) {
	deadline := time.Now().Add(3000 * time.Millisecond) // Increased from 1.5s to 3s for more reliable testing
	for time.Now().Before(deadline) {
		var count int64
		var err error
		for i := 0; i < 3; i++ {
			err = db.Model(&models.LLMChatRecord{}).Count(&count).Error
			if err == nil {
				break
			}
			time.Sleep(100 * time.Millisecond) // Increased inner retry sleep
		}
		if err != nil {
			continue
		}

		if count >= expectedCount {
			time.Sleep(50 * time.Millisecond) // Increased sleep before returning
			return
		}
		time.Sleep(100 * time.Millisecond) // Increased outer polling interval
	}
	t.Fatalf("Timeout waiting for analytics records. Expected at least: %d", expectedCount)
}

// waitUntilIdle waits until the analytics goroutine has not produced any *new*
// LLMChatRecords for ~100ms, meaning it's idle.
func waitUntilIdle(t *testing.T, db *gorm.DB) {
	var lastCount int64
	var stableRounds int
	timeout := time.NewTimer(3000 * time.Millisecond) // Increased from 1.5s to 3s for more reliable testing
	ticker := time.NewTicker(50 * time.Millisecond)   // Increased back to 50ms for more stability
	defer timeout.Stop()
	defer ticker.Stop()

	// capture initial count
	for i := 0; i < 3; i++ {
		err := db.Model(&models.LLMChatRecord{}).Count(&lastCount).Error
		if err == nil {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}

	for {
		select {
		case <-timeout.C:
			t.Fatalf("Timeout waiting for DB to go idle (analytics).")
		case <-ticker.C:
			var curCount int64
			var err error
			for i := 0; i < 3; i++ {
				err = db.Model(&models.LLMChatRecord{}).Count(&curCount).Error
				if err == nil {
					break
				}
				time.Sleep(25 * time.Millisecond)
			}
			if err != nil {
				continue
			}
			if curCount == lastCount {
				stableRounds++
				if stableRounds >= 2 {
					time.Sleep(25 * time.Millisecond)
					return
				}
			} else {
				stableRounds = 0
				lastCount = curCount
			}
		}
	}
}

// waitForRecordWithCost waits for a record to be written with a non-zero cost and returns the record
func waitForRecordWithCost(t *testing.T, db *gorm.DB) *models.LLMChatRecord {
	deadline := time.Now().Add(3000 * time.Millisecond) // Increased from 1.5s to 3s for more reliable testing
	for time.Now().Before(deadline) {
		var record models.LLMChatRecord
		var err error
		for i := 0; i < 3; i++ {
			err = db.First(&record).Error
			if err == nil {
				break
			}
			time.Sleep(50 * time.Millisecond) // Increased sleep interval for more stability
		}
		if err != nil {
			continue
		}

		// Scale cost for analytics tests, keep raw value for budget tests
		scaledCost := record.Cost / 10000.0
		t.Logf("Found record: cost=%f (raw=%f) prompt_tokens=%d response_tokens=%d timestamp=%v",
			scaledCost, record.Cost, record.PromptTokens, record.ResponseTokens, record.TimeStamp)

		if record.Cost > 0 {
			time.Sleep(25 * time.Millisecond)
			// Return scaled cost for analytics tests, raw cost for budget tests
			if strings.Contains(t.Name(), "TestAnalyze") {
				record.Cost = scaledCost
			}
			return &record
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("Timeout waiting for record with cost")
	return nil
}

// waitForSpendingUpdate waits for spending to be updated to an expected value
func waitForSpendingUpdate(t *testing.T, budgetService *services.BudgetService, appID uint, llmID uint, start, end time.Time, expectedSpent float64) {
	deadline := time.Now().Add(3000 * time.Millisecond) // Increased from 1.5s to 3s for more reliable testing
	for time.Now().Before(deadline) {
		budgetService.ClearCache()

		var appSpent, llmSpent float64
		var err error

		// Retry app spending query if locked
		for i := 0; i < 3; i++ {
			appSpent, err = budgetService.GetMonthlySpending(appID, start, end)
			if err == nil {
				break
			}
			time.Sleep(50 * time.Millisecond) // Increased sleep interval for more stability
		}
		if err != nil {
			continue
		}

		// Retry llm spending query if locked
		for i := 0; i < 3; i++ {
			llmSpent, err = budgetService.GetLLMMonthlySpending(llmID, start, end)
			if err == nil {
				break
			}
			time.Sleep(25 * time.Millisecond)
		}
		if err != nil {
			continue
		}

		t.Logf("Current spending - app: %.2f, llm: %.2f (expected: %.2f) [start=%v, end=%v]",
			appSpent, llmSpent, expectedSpent, start, end)

		if appSpent == expectedSpent && llmSpent == expectedSpent {
			time.Sleep(25 * time.Millisecond)
			return
		}

		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("Timeout waiting for spending to update to %.2f", expectedSpent)
}
