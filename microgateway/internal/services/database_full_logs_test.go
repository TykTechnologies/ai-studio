package services

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// lockedBuffer is a log sink the writer's goroutine and the test share.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) count(substr string) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return strings.Count(b.buf.String(), substr)
}

// A gateway whose database has no room left keeps flushing analytics and
// budget usage, several times a second. Each flush fails the same way, so
// logging it each time floods the log volume during the very incident (bead
// midsommar-clean-4x5y.48: about 7,200 lines a minute at load). The
// condition is logged once when it begins and once when it ends
// (database.NoteWrite); the counters carry the detail.
func TestDatabaseFull_WriterLogsTheConditionOnceNotEachFlush(t *testing.T) {
	db, limit := sizeLimitedEdgeDB(t)
	t.Cleanup(func() { _ = database.NoteWrite(database.WriteAnalytics, nil) })

	const apps = 300 // more budget_usage rows than the pages it has hold
	for i := 1; i <= apps; i++ {
		require.NoError(t, db.Create(&database.App{Name: fmt.Sprintf("app-%d", i), IsActive: true}).Error)
	}

	logs := &lockedBuffer{}
	prev := log.Logger
	log.Logger = zerolog.New(logs).Level(zerolog.InfoLevel)
	t.Cleanup(func() { log.Logger = prev })

	start, end := ledgerPeriod()
	ledger := NewBudgetLedger(db)
	w := NewAnalyticsWriter(db, 10000, 50, 10*time.Millisecond)
	w.SetLedger(ledger)
	w.Start()
	var stopOnce sync.Once
	stop := func() { stopOnce.Do(w.Stop) }
	t.Cleanup(stop) // before the database closes, also when an assertion stops the test

	body := strings.Repeat("b", 2000)
	seq := 0
	var addedCost float64
	load := func(d time.Duration) {
		for deadline := time.Now().Add(d); time.Now().Before(deadline); {
			for i := 0; i < 10; i++ {
				seq++
				app := uint(seq%apps + 1)
				w.Enqueue(&database.AnalyticsEvent{RequestID: fmt.Sprintf("r-%d", seq), AppID: app, TimeStamp: time.Now(), RequestBody: body})
				ledger.Add(app, start, end, 10, 1, 5, 5)
				addedCost++
			}
			time.Sleep(5 * time.Millisecond)
		}
	}

	limit(true)
	before := w.Stats()
	load(1500 * time.Millisecond)
	during := w.Stats()

	flushes := during.Batches - before.Batches
	require.GreaterOrEqual(t, flushes, uint64(10), "the writer flushed many times while the database was full")
	assert.Greater(t, during.Failed, before.Failed, "rows it could not store are counted")
	full, _, failing := database.StorageStatus()
	require.True(t, full)
	assert.Contains(t, failing, database.WriteAnalytics)
	assert.Greater(t, database.StorageFullFailures()[database.WriteAnalytics], uint64(0), "refusals are counted")

	assert.Equal(t, 1, logs.count("The gateway's database has no room left"), "the condition is logged once when it begins")
	for _, line := range []string{
		"Budget ledger: writing the App's usage failed",
		"Analytics writer: batch failed, writing its rows one by one",
		"Analytics writer: event dropped",
		"Analytics writer: flush failed",
	} {
		assert.Zero(t, logs.count(line), "not logged per flush while the database is full: %q", line)
	}

	// Room again: the condition clears, once, and the usage kept in the
	// ledger meanwhile is stored.
	limit(false)
	load(200 * time.Millisecond)
	stop()
	assert.Equal(t, 1, logs.count("The gateway's database has room again"), "and once when it ends")
	_, _, failing = database.StorageStatus()
	assert.NotContains(t, failing, database.WriteAnalytics)
	var stored float64
	for i := uint(1); i <= apps; i++ {
		stored += storedUsage(t, db, i).TotalCost
	}
	assert.Equal(t, addedCost, stored, "no usage was lost to the full database")
}
