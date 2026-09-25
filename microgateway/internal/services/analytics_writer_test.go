package services

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// openWriterTestDB returns a migrated file-backed SQLite database (WAL, as
// the gateway runs it) and its writer handle.
func openWriterTestDB(t *testing.T) (db, writer *gorm.DB) {
	t.Helper()
	cfg := database.DatabaseConfig{
		Type: "sqlite", DSN: "file:" + filepath.Join(t.TempDir(), "gw.db") + "?mode=rwc",
		MaxOpenConns: 8, MaxIdleConns: 8, LogLevel: "silent",
	}
	db, err := database.Connect(cfg)
	require.NoError(t, err)
	db.Logger = logger.Default.LogMode(logger.Silent)
	require.NoError(t, database.Migrate(db))
	writer, err = database.OpenWriter(cfg, db)
	require.NoError(t, err)
	writer.Logger = logger.Default.LogMode(logger.Silent)
	t.Cleanup(func() {
		database.Close(writer)
		database.Close(db)
	})
	return db, writer
}

func countEvents(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Model(&database.AnalyticsEvent{}).Count(&n).Error)
	return n
}

func TestAnalyticsWriterFlushesOnBatchSizeAndInterval(t *testing.T) {
	db, wdb := openWriterTestDB(t)

	// Size: a full batch is written without waiting for the interval.
	w := NewAnalyticsWriter(wdb, 100, 5, time.Hour)
	w.Start()
	for i := 0; i < 5; i++ {
		require.True(t, w.Enqueue(&database.AnalyticsEvent{RequestID: fmt.Sprintf("size-%d", i), AppID: 1, TimeStamp: time.Now()}))
	}
	require.Eventually(t, func() bool { return countEvents(t, db) == 5 }, 5*time.Second, 10*time.Millisecond)
	w.Stop()

	// Interval: a partial batch is written when the interval passes.
	w = NewAnalyticsWriter(wdb, 100, 1000, 20*time.Millisecond)
	w.Start()
	defer w.Stop()
	require.True(t, w.Enqueue(&database.AnalyticsEvent{RequestID: "interval", AppID: 1, TimeStamp: time.Now()}))
	require.Eventually(t, func() bool { return countEvents(t, db) == 6 }, 5*time.Second, 10*time.Millisecond)
	assert.EqualValues(t, 6, w.Stats().Written+5)
}

func TestAnalyticsWriterDropsWhenFullAndDrainsOnStop(t *testing.T) {
	db, wdb := openWriterTestDB(t)
	w := NewAnalyticsWriter(wdb, 2, 500, time.Hour)

	// Not started: the queue fills and the third row is dropped.
	assert.True(t, w.Enqueue(&database.AnalyticsEvent{RequestID: "a", AppID: 1, TimeStamp: time.Now()}))
	assert.True(t, w.Enqueue(&database.AnalyticsEvent{RequestID: "b", AppID: 1, TimeStamp: time.Now()}))
	assert.False(t, w.Enqueue(&database.AnalyticsEvent{RequestID: "c", AppID: 1, TimeStamp: time.Now()}))
	assert.EqualValues(t, 1, w.Stats().Dropped)

	w.Start()
	w.Stop()
	assert.EqualValues(t, 2, countEvents(t, db), "Stop writes what was queued")
}

// One row whose request ID is already stored must not cost the rest of its
// batch.
func TestAnalyticsWriterSkipsDuplicateRequestIDs(t *testing.T) {
	db, wdb := openWriterTestDB(t)
	require.NoError(t, db.Create(&database.AnalyticsEvent{RequestID: "dup", AppID: 1, TimeStamp: time.Now()}).Error)

	w := NewAnalyticsWriter(wdb, 10, 500, time.Hour)
	w.Enqueue(&database.AnalyticsEvent{RequestID: "dup", AppID: 1, TimeStamp: time.Now()})
	w.Enqueue(&database.AnalyticsEvent{RequestID: "new", AppID: 1, TimeStamp: time.Now()})
	w.Start()
	w.Stop()

	assert.EqualValues(t, 2, countEvents(t, db))
	assert.Zero(t, w.Stats().Failed)
}

// A row the database rejects (on Postgres, a foreign key to a configuration
// row deleted meanwhile) costs only itself, not its batch or the budget
// usage flushed with it.
func TestAnalyticsWriterIsolatesRejectedRows(t *testing.T) {
	db, wdb := openWriterTestDB(t)
	require.NoError(t, db.Exec(`CREATE TRIGGER reject_bad BEFORE INSERT ON analytics_events
		WHEN NEW.request_id = 'bad' BEGIN SELECT RAISE(ABORT, 'rejected row'); END`).Error)
	start, end := ledgerPeriod()
	ledger := NewBudgetLedger(db)
	ledger.Add(1, start, end, 10, 100, 5, 5)

	w := NewAnalyticsWriter(wdb, 10, 500, time.Hour)
	w.SetLedger(ledger)
	for _, id := range []string{"good-1", "bad", "good-2"} {
		w.Enqueue(&database.AnalyticsEvent{RequestID: id, AppID: 1, TimeStamp: time.Now()})
	}
	w.Start()
	w.Stop()

	assert.EqualValues(t, 2, countEvents(t, db))
	assert.EqualValues(t, 2, w.Stats().Written)
	assert.EqualValues(t, 1, w.Stats().Failed)
	assert.Equal(t, 100.0, storedUsage(t, db, 1).TotalCost)
}

// Concurrent requests of one App in the same second each get their own row
// with their own usage. The old handler paired proxy logs and chat records
// by App and second, so they overwrote each other's pairing and merged into
// the wrong rows.
func TestRecordExchangeConcurrentRequestsKeepTheirOwnRows(t *testing.T) {
	db, wdb := openWriterTestDB(t)
	w := NewAnalyticsWriter(wdb, 1000, 50, 10*time.Millisecond)
	w.Start()
	h := NewMicrogatewaAnalyticsHandler(wdb, nil, nil, nil)
	h.SetWriter(w)

	const n = 200
	ts := time.Now().Truncate(time.Second)
	var wg sync.WaitGroup
	for i := 1; i <= n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			h.RecordExchange(context.Background(),
				&models.ProxyLog{AppID: 7, LLMID: 3, Vendor: "openai", ResponseCode: 200, TimeStamp: ts},
				&models.LLMChatRecord{AppID: 7, LLMID: 3, Name: "gpt-x", Vendor: "openai",
					PromptTokens: i, ResponseTokens: i, TotalTokens: 2 * i, TimeStamp: ts})
		}(i)
	}
	wg.Wait()
	w.Stop()

	var events []database.AnalyticsEvent
	require.NoError(t, db.Order("prompt_tokens").Find(&events).Error)
	require.Len(t, events, n)
	for i, e := range events {
		assert.Equal(t, i+1, e.PromptTokens)
		assert.Equal(t, 2*(i+1), e.TotalTokens)
		assert.Equal(t, "gpt-x", e.Name)
		assert.Equal(t, 200, e.StatusCode)
	}
}

// A proxy log without usage (a refused or failed request) is one row of its
// own.
func TestRecordProxyLogWithoutChatRecordIsOneRow(t *testing.T) {
	db, wdb := openWriterTestDB(t)
	h := NewMicrogatewaAnalyticsHandler(wdb, nil, nil, nil)

	h.RecordProxyLog(context.Background(), &models.ProxyLog{AppID: 7, ResponseCode: 401, TimeStamp: time.Now()})
	h.RecordProxyLog(context.Background(), &models.ProxyLog{AppID: 7, ResponseCode: 401, TimeStamp: time.Now()})

	var events []database.AnalyticsEvent
	require.NoError(t, db.Find(&events).Error)
	require.Len(t, events, 2)
	assert.NotEqual(t, events[0].RequestID, events[1].RequestID)
	assert.Equal(t, 401, events[0].StatusCode)
	assert.Zero(t, events[0].TotalTokens)
}
