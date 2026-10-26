package proxy

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"math"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/andybalholm/brotli"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/analytics"
	"github.com/TykTechnologies/midsommar/v2/config"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/testinfra/sqlitetemplate"
	"github.com/TykTechnologies/midsommar/v2/services"
)

// setupDBSeq makes each setupDB database name unique, so a test never shares a
// database with an earlier one whose background writers are still draining.
var setupDBSeq atomic.Int64

// setupDBTemplate is migrated once per test binary (models.InitModels and the
// analytics tables) and copied into every test's database: migrating each one
// cost ~300 ms a test under -race. Migration itself still runs for real, here
// and in the schema goldens in models.
var setupDBTemplate = sqlitetemplate.New(func(db *gorm.DB) error {
	if err := models.InitModels(db); err != nil {
		return err
	}
	if err := db.AutoMigrate(
		&models.LLMChatRecord{},
		&models.LLMChatLogEntry{},
		&models.ToolCallRecord{},
		&models.ProxyLog{},
		&models.Notification{},
	); err != nil {
		return err
	}
	// What setupTest used to run on each test's database.
	return analytics.Migrate(db)
})

// setupDB opens a private in-memory SQLite database for one test, holding a
// copy of the migrated template.
//
// It used to open "file::memory:?cache=shared" with 10 pooled connections:
// one database for the whole package, and table-level locks between those
// connections. The analytics writer, the proxy's log writes and a test's own
// UPDATE then collided with "database table is locked" (SQLITE_LOCKED), which
// busy_timeout does not retry, so tests failed at random under load. A named
// database per test, on a single connection, serialises every statement
// instead: no table lock can be contended.
func setupDB(t *testing.T) *gorm.DB {
	name := fmt.Sprintf("proxytest_%d", setupDBSeq.Add(1))
	// No PrepareStmt: with gorm's prepared-statement cache, a transaction
	// holding the one connection waited for another goroutine to finish
	// preparing the same statement, and that goroutine waited for the
	// connection (TestSetupDBTransactionDoesNotWaitOnAnotherGoroutinesPrepare).
	// Studio does not enable PrepareStmt either.
	db, err := gorm.Open(sqlite.Open("file:"+name+"?mode=memory&cache=shared&_busy_timeout=5000"), &gorm.Config{
		SkipDefaultTransaction: true,
	})
	require.NoError(t, err)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	// One connection: statements from the test, the proxy and the analytics
	// writer queue for it rather than contend for table locks. It must never
	// be closed while idle, or the in-memory database goes with it.
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetConnMaxLifetime(0)

	require.NoError(t, setupDBTemplate.CopyInto(db))
	// The join models InitModels registers live on the gorm handle, not in
	// the database, so this handle registers them itself.
	require.NoError(t, models.RegisterJoinModels(db))

	return db
}

// setupTest initializes DB, does migrations, and starts analytics in one place.
// The returned cancel stops the analytics recorder and waits for its writer to
// return, so nothing is written to the database after it.
func setupTest(t *testing.T) (*gorm.DB, context.CancelFunc) {
	// Clear signup domain filter for tests (allows any email domain)
	config.Get("").FilterSignupDomains = nil

	// Reset global analytics handler state before test
	analytics.ResetHandler()

	// The template is migrated for analytics already (analytics.Migrate).
	db := setupDB(t)

	ctx, cancel := context.WithCancel(context.Background())
	analytics.StartRecording(ctx, db)
	recorder := analytics.GetHandler()

	return db, func() {
		cancel()
		if s, ok := recorder.(interface{ Stop() }); ok {
			s.Stop()
		}
	}
}

// tearDownTest stops the analytics recorder, waiting for its writer so it
// does not outlive the DB, then closes the DB.
func tearDownTest(db *gorm.DB, cancel context.CancelFunc) {
	cancel()
	sqlDB, err := db.DB()
	if err == nil {
		_ = sqlDB.Close()
	}
}

// waitPollInterval is how often the wait helpers re-check their condition.
// Each helper checks before it first sleeps, so a condition that already
// holds costs nothing, and none sleeps after its condition holds: the
// deadlines are the only allowance for a slow machine.
const waitPollInterval = 10 * time.Millisecond

// waitForAnalytics ensures we have at least `expectedCount` LLMChatRecords.
func waitForAnalytics(t *testing.T, db *gorm.DB, expectedCount int64) {
	deadline := time.Now().Add(12000 * time.Millisecond) // Increased to 12s for CI reliability
	var count int64
	var lastErr error
	for {
		err := db.Model(&models.LLMChatRecord{}).Count(&count).Error
		if err == nil && count >= expectedCount {
			return
		}
		if err != nil {
			lastErr = err
		}
		if !time.Now().Before(deadline) {
			break
		}
		time.Sleep(waitPollInterval)
	}
	t.Fatalf("Timeout waiting for analytics records. Expected at least: %d, have %d (last error: %v)", expectedCount, count, lastErr)
}

// waitUntilIdle waits until the analytics goroutine has not produced any *new*
// LLMChatRecords for ~100ms, meaning it's idle. Unlike the other helpers it
// has to let real time pass: it checks that nothing more arrives, which no
// single read can show.
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
	var record models.LLMChatRecord
	var err error
	for {
		record = models.LLMChatRecord{}
		err = db.First(&record).Error
		if err == nil && record.Cost > 0 {
			// Scale cost for analytics tests, keep raw value for budget tests
			scaledCost := record.Cost / 10000.0
			t.Logf("Found record: cost=%f (raw=%f) prompt_tokens=%d response_tokens=%d timestamp=%v",
				scaledCost, record.Cost, record.PromptTokens, record.ResponseTokens, record.TimeStamp)
			// Return scaled cost for analytics tests, raw cost for budget tests
			if strings.Contains(t.Name(), "TestAnalyze") {
				record.Cost = scaledCost
			}
			return &record
		}
		if !time.Now().Before(deadline) {
			break
		}
		time.Sleep(waitPollInterval)
	}
	t.Fatalf("Timeout waiting for record with cost (last read: cost=%f, error: %v)", record.Cost, err)
	return nil
}

// waitForCacheFlush clears the budget service's spend cache. ClearCache is
// synchronous, so there is nothing to wait for once it returns.
func waitForCacheFlush(t *testing.T, budgetService services.BudgetService) {
	budgetService.ClearCache()
}

// waitForDatabaseSync reads the database once it answers. Every test database
// is on one connection, so a write that has returned is already visible; this
// only waits out a read that fails.
func waitForDatabaseSync(t *testing.T, db *gorm.DB) {
	var count int64
	deadline := time.Now().Add(3000 * time.Millisecond) // Increased timeout
	for {
		err := db.Model(&models.LLMChatRecord{}).Count(&count).Error
		if err == nil || !time.Now().Before(deadline) {
			return
		}
		time.Sleep(waitPollInterval)
	}
}

// waitForSpendingValue waits for spending to reach a specific value with retry logic
func waitForSpendingValue(t *testing.T, budgetService services.BudgetService, appID uint, start, end time.Time, expectedSpent float64) {
	deadline := time.Now().Add(10000 * time.Millisecond) // Increased to 10s for very reliable testing
	var lastSpent float64
	var lastErr error

	for {
		budgetService.ClearCache()
		spent, err := budgetService.GetMonthlySpending(appID, start, end)
		if err == nil {
			if math.Abs(spent-expectedSpent) < 0.1 {
				t.Logf("Current spending: %.2f (expected: %.2f) [start=%v, end=%v]", spent, expectedSpent, start, end)
				return
			}
			lastSpent = spent
		} else {
			lastErr = err
		}
		if !time.Now().Before(deadline) {
			break
		}
		time.Sleep(waitPollInterval)
	}

	t.Fatalf("Timeout waiting for spending to reach %.2f, last value was %.2f [start=%v, end=%v] (last error: %v)", expectedSpent, lastSpent, start, end, lastErr)
}

// waitForProxyLog waits for a ProxyLog with specific app_id and response_code to exist
func waitForProxyLog(t *testing.T, db *gorm.DB, appID uint, statusCode int) {
	deadline := time.Now().Add(5000 * time.Millisecond)
	var lastErr error
	for {
		var count int64
		err := db.Model(&models.ProxyLog{}).Where("app_id = ? AND response_code = ?", appID, statusCode).Count(&count).Error
		if err == nil && count >= 1 {
			return
		}
		if err != nil {
			lastErr = err
		}
		if !time.Now().Before(deadline) {
			break
		}
		time.Sleep(waitPollInterval)
	}
	t.Fatalf("Timeout waiting for ProxyLog with app_id=%d and response_code=%d (last error: %v)", appID, statusCode, lastErr)
}

// waitForSpendingUpdate waits for spending to be updated to an expected value
func waitForSpendingUpdate(t *testing.T, budgetService services.BudgetService, appID uint, llmID uint, start, end time.Time, expectedSpent float64) {
	deadline := time.Now().Add(3000 * time.Millisecond) // Increased from 1.5s to 3s for more reliable testing
	var appSpent, llmSpent float64
	var lastErr error
	for {
		budgetService.ClearCache()

		var err error
		appSpent, err = budgetService.GetMonthlySpending(appID, start, end)
		if err == nil {
			llmSpent, err = budgetService.GetLLMMonthlySpending(llmID, start, end)
		}
		if err == nil && appSpent == expectedSpent && llmSpent == expectedSpent {
			return
		}
		if err != nil {
			lastErr = err
		}
		if !time.Now().Before(deadline) {
			break
		}
		time.Sleep(waitPollInterval)
	}
	t.Fatalf("Timeout waiting for spending to update to %.2f: app %.2f, llm %.2f [start=%v, end=%v] (last error: %v)",
		expectedSpent, appSpent, llmSpent, start, end, lastErr)
}

func compressWithGzip(t *testing.T, data []byte) []byte {
	t.Helper()

	var buf bytes.Buffer
	writer := gzip.NewWriter(&buf)

	_, err := writer.Write(data)
	require.NoError(t, err, "failed to write data to gzip writer")

	err = writer.Close()
	require.NoError(t, err, "failed to close gzip writer")

	return buf.Bytes()
}

func compressWithBrotli(t *testing.T, data []byte) []byte {
	t.Helper()

	var buf bytes.Buffer
	writer := brotli.NewWriter(&buf)

	_, err := writer.Write(data)
	require.NoError(t, err, "failed to write data to brotli writer")

	err = writer.Close()
	require.NoError(t, err, "failed to close brotli writer")

	return buf.Bytes()
}
