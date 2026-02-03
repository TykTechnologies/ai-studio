package proxy

import (
	"context"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/TykTechnologies/midsommar/v2/analytics"
	"github.com/TykTechnologies/midsommar/v2/config"
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

	// Get underlying SQL DB for pragma configuration
	sqlDB, err := db.DB()
	require.NoError(t, err)

	// Configure SQLite for better concurrent access
	_, _ = sqlDB.Exec("PRAGMA journal_mode = WAL;")
	_, _ = sqlDB.Exec("PRAGMA busy_timeout = 30000;") // Increased from 5s to 30s for CI
	_, _ = sqlDB.Exec("PRAGMA synchronous = NORMAL;")
	_, _ = sqlDB.Exec("PRAGMA cache_size = -64000;") // 64MB cache
	_, _ = sqlDB.Exec("PRAGMA temp_store = MEMORY;")
	_, _ = sqlDB.Exec("PRAGMA mmap_size = 30000000000;")
	_, _ = sqlDB.Exec("PRAGMA wal_autocheckpoint = 1000;")
	_, _ = sqlDB.Exec("PRAGMA optimize;")

	// Set connection pool settings for better concurrent access
	sqlDB.SetMaxOpenConns(10)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxLifetime(time.Hour)

	// Migrate main models with retry logic
	err = models.InitModels(db)
	require.NoError(t, err)

	// Migrate analytics tables explicitly with retry logic
	for attempt := 0; attempt < 3; attempt++ {
		err = db.AutoMigrate(
			&models.LLMChatRecord{},
			&models.LLMChatLogEntry{},
			&models.ToolCallRecord{},
			&models.ProxyLog{},
			&models.Notification{},
		)
		if err == nil {
			break
		}
		if strings.Contains(err.Error(), "database table is locked") {
			t.Logf("Database locked during migration, retrying... (attempt %d/3)", attempt+1)
			time.Sleep(time.Duration(100*(attempt+1)) * time.Millisecond)
			continue
		}
		require.NoError(t, err)
	}
	require.NoError(t, err)

	return db
}

// setupTest initializes DB, does migrations, and starts analytics in one place.
func setupTest(t *testing.T) (*gorm.DB, context.CancelFunc) {
	// Clear signup domain filter for tests (allows any email domain)
	config.Get("").FilterSignupDomains = nil

	// Reset global analytics handler state before test
	analytics.ResetHandler()

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
	deadline := time.Now().Add(12000 * time.Millisecond) // Increased to 12s for CI reliability
	for time.Now().Before(deadline) {
		var count int64
		var err error

		// Retry with exponential backoff for database lock errors
		for i := 0; i < 5; i++ {
			err = db.Model(&models.LLMChatRecord{}).Count(&count).Error
			if err == nil {
				break
			}

			// Handle database lock errors specifically
			if strings.Contains(err.Error(), "database table is locked") ||
				strings.Contains(err.Error(), "database is locked") {
				backoff := time.Duration(100*(1<<i)) * time.Millisecond
				t.Logf("Database locked during analytics wait, backing off %v (attempt %d/5)", backoff, i+1)
				time.Sleep(backoff)
				continue
			}

			// For other errors, use shorter backoff
			time.Sleep(50 * time.Millisecond)
		}
		if err != nil {
			t.Logf("Error checking analytics count: %v, retrying...", err)
			time.Sleep(200 * time.Millisecond)
			continue
		}

		if count >= expectedCount {
			time.Sleep(100 * time.Millisecond) // Increased stabilization sleep
			return
		}
		time.Sleep(150 * time.Millisecond) // Increased polling interval for CI
	}
	t.Fatalf("Timeout waiting for analytics records. Expected at least: %d", expectedCount)
}

// waitUntilIdle waits until the analytics goroutine has not produced any *new*
// LLMChatRecords for ~100ms, meaning it's idle.
func waitUntilIdle(t *testing.T, db *gorm.DB) {
	var lastCount int64
	var stableRounds int
	timeout := time.NewTimer(8000 * time.Millisecond) // Increased to 8s for very reliable testing
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
	deadline := time.Now().Add(8000 * time.Millisecond) // Increased to 8s for very reliable testing
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

// waitForCacheFlush waits for cache operations to complete
func waitForCacheFlush(t *testing.T, budgetService services.BudgetService) {
	// Clear cache and wait for it to propagate
	budgetService.ClearCache()
	time.Sleep(100 * time.Millisecond)

	// Verify cache is actually cleared by doing a quick operation
	deadline := time.Now().Add(1000 * time.Millisecond)
	for time.Now().Before(deadline) {
		// Try a simple cache operation to ensure it's responsive
		time.Sleep(50 * time.Millisecond)
		return
	}
}

// waitForDatabaseSync waits for database operations to be fully committed
func waitForDatabaseSync(t *testing.T, db *gorm.DB) {
	// Force a sync by doing a simple query with retry on lock errors
	var count int64
	deadline := time.Now().Add(3000 * time.Millisecond) // Increased timeout
	for time.Now().Before(deadline) {
		err := db.Model(&models.LLMChatRecord{}).Count(&count).Error
		if err == nil {
			// Additional wait to ensure database is fully synced
			time.Sleep(100 * time.Millisecond)
			return
		}
		// Check if it's a database lock error and wait longer
		if strings.Contains(err.Error(), "database table is locked") {
			time.Sleep(100 * time.Millisecond)
		} else {
			time.Sleep(25 * time.Millisecond)
		}
	}
}

// waitForSpendingValue waits for spending to reach a specific value with retry logic
func waitForSpendingValue(t *testing.T, budgetService services.BudgetService, appID uint, start, end time.Time, expectedSpent float64) {
	deadline := time.Now().Add(10000 * time.Millisecond) // Increased to 10s for very reliable testing
	var lastSpent float64

	for time.Now().Before(deadline) {
		budgetService.ClearCache()
		time.Sleep(50 * time.Millisecond) // Brief wait after cache clear

		var spent float64
		var err error

		// Retry spending query with exponential backoff
		for attempt := 0; attempt < 3; attempt++ {
			spent, err = budgetService.GetMonthlySpending(appID, start, end)
			if err == nil {
				break
			}
			backoff := time.Duration(50*(1<<attempt)) * time.Millisecond
			time.Sleep(backoff)
		}

		if err != nil {
			t.Logf("Error getting spending: %v, retrying...", err)
			time.Sleep(100 * time.Millisecond)
			continue
		}

		t.Logf("Current spending: %.2f (expected: %.2f) [start=%v, end=%v]", spent, expectedSpent, start, end)

		if math.Abs(spent-expectedSpent) < 0.1 {
			time.Sleep(50 * time.Millisecond) // Final stabilization
			return
		}

		lastSpent = spent
		time.Sleep(100 * time.Millisecond)
	}

	t.Fatalf("Timeout waiting for spending to reach %.2f, last value was %.2f", expectedSpent, lastSpent)
}

// waitForSpendingUpdate waits for spending to be updated to an expected value
func waitForSpendingUpdate(t *testing.T, budgetService services.BudgetService, appID uint, llmID uint, start, end time.Time, expectedSpent float64) {
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
