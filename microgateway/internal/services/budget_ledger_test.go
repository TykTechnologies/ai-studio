package services

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func ledgerPeriod() (time.Time, time.Time) {
	now := time.Now()
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	return start, start.AddDate(0, 1, 0).Add(-time.Second)
}

func storedUsage(t *testing.T, db *gorm.DB, appID uint) database.BudgetUsage {
	t.Helper()
	var u database.BudgetUsage
	require.NoError(t, db.Where("app_id = ?", appID).First(&u).Error)
	return u
}

// Recorded usage counts towards the budget at once and reaches budget_usage
// with the writer's next batch, as increments on the stored row.
func TestBudgetLedgerRecordsInMemoryAndFlushesIncrements(t *testing.T) {
	db, wdb := openWriterTestDB(t)
	start, end := ledgerPeriod()
	require.NoError(t, db.Create(&database.BudgetUsage{AppID: 1, PeriodStart: start, PeriodEnd: end, TotalCost: 1000, TokensUsed: 10, RequestsCount: 1}).Error)

	ledger := NewBudgetLedger(db)
	spent, err := ledger.Spent(1, start, end)
	require.NoError(t, err)
	assert.Equal(t, 1000.0, spent)

	ledger.Add(1, start, end, 100, 250, 60, 40)
	ledger.Add(1, start, end, 100, 250, 60, 40)
	spent, err = ledger.Spent(1, start, end)
	require.NoError(t, err)
	assert.Equal(t, 1500.0, spent, "unflushed usage counts at once")
	assert.Equal(t, 1000.0, storedUsage(t, db, 1).TotalCost, "nothing written yet")

	// An App with no row yet gets one.
	ledger.Add(2, start, end, 5, 50, 3, 2)

	w := NewAnalyticsWriter(wdb, 10, 500, time.Hour)
	w.SetLedger(ledger)
	w.Start()
	w.Stop()

	u := storedUsage(t, db, 1)
	assert.Equal(t, 1500.0, u.TotalCost)
	assert.EqualValues(t, 210, u.TokensUsed)
	assert.EqualValues(t, 3, u.RequestsCount)
	assert.EqualValues(t, 120, u.PromptTokens)
	assert.EqualValues(t, 80, u.CompletionTokens)
	assert.Equal(t, 50.0, storedUsage(t, db, 2).TotalCost)

	spent, err = ledger.Spent(1, start, end)
	require.NoError(t, err)
	assert.Equal(t, 1500.0, spent, "flushed usage is not counted twice")
}

// A failed flush keeps its amounts for the next one.
func TestBudgetLedgerFailedFlushKeepsAmounts(t *testing.T) {
	db, _ := openWriterTestDB(t)
	start, end := ledgerPeriod()
	ledger := NewBudgetLedger(db)
	ledger.Add(1, start, end, 10, 100, 5, 5)

	f := ledger.take()
	require.False(t, f.empty())
	spent, err := ledger.Spent(1, start, end)
	require.NoError(t, err)
	assert.Equal(t, 100.0, spent, "in-flight usage still counts")

	ledger.finish(f, false)
	f = ledger.take()
	require.Len(t, f.amounts, 1)
	assert.Equal(t, 100.0, f.amounts[0].cost)
	assert.EqualValues(t, 1, f.amounts[0].requests)
}

// The writer re-reads the stored totals (refresh), so writes the ledger did
// not make (a budget sync, another gateway) are seen; the request path does
// not read again. A sync's figure raises the ledger at once.
func TestBudgetLedgerRefreshAndReconcile(t *testing.T) {
	db, _ := openWriterTestDB(t)
	start, end := ledgerPeriod()
	require.NoError(t, db.Create(&database.BudgetUsage{AppID: 1, PeriodStart: start, PeriodEnd: end, TotalCost: 100}).Error)

	ledger := NewBudgetLedger(db)
	spent, _ := ledger.Spent(1, start, end)
	assert.Equal(t, 100.0, spent)

	require.NoError(t, db.Model(&database.BudgetUsage{}).Where("app_id = ?", 1).Update("total_cost", 400).Error)
	spent, _ = ledger.Spent(1, start, end)
	assert.Equal(t, 100.0, spent, "the request path keeps the ledger's copy")

	ledger.refresh()
	spent, _ = ledger.Spent(1, start, end)
	assert.Equal(t, 400.0, spent)

	ledger.Reconcile(1, start, 900)
	spent, _ = ledger.Spent(1, start, end)
	assert.Equal(t, 900.0, spent)
	ledger.Reconcile(1, start, 500)
	spent, _ = ledger.Spent(1, start, end)
	assert.Equal(t, 900.0, spent, "a lower figure never lowers the spend")
}

// Concurrent first checks of an App share one database read. Under load a
// read per request queued on the connection pool and stalled the request path.
func TestBudgetLedgerFirstReadIsShared(t *testing.T) {
	db, _ := openCountingDB(t)
	start, end := ledgerPeriod()
	require.NoError(t, db.Create(&database.BudgetUsage{AppID: 1, PeriodStart: start, PeriodEnd: end, TotalCost: 100}).Error)
	var reads atomic.Int64
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register("test:slow_budget_read", func(tx *gorm.DB) {
		if tx.Statement.Table == "budget_usage" {
			reads.Add(1)
			time.Sleep(50 * time.Millisecond)
		}
	}))

	ledger := NewBudgetLedger(db)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			spent, err := ledger.Spent(1, start, end)
			assert.NoError(t, err)
			assert.Equal(t, 100.0, spent)
		}()
	}
	wg.Wait()
	for i := 0; i < 10; i++ {
		_, _ = ledger.Spent(1, start, end)
	}
	assert.EqualValues(t, 1, reads.Load())
}

// The budget sync raises the stored total to the control plane's figure in
// one statement, keeping a higher local total.
func TestBudgetSyncRaisesStoredUsageAndLedger(t *testing.T) {
	db, _ := openWriterTestDB(t)
	start, end := ledgerPeriod()
	ledger := NewBudgetLedger(db)
	h := &BudgetSyncHandler{db: db, ledger: ledger}

	h.updateLocalBudget(1, 0.05, start, end) // creates: $0.05 = 500 stored
	assert.Equal(t, 500.0, storedUsage(t, db, 1).TotalCost)

	spent, _ := ledger.Spent(1, start, end)
	assert.Equal(t, 500.0, spent)
	h.updateLocalBudget(1, 0.02, start, end) // lower: kept
	assert.Equal(t, 500.0, storedUsage(t, db, 1).TotalCost)
	h.updateLocalBudget(1, 0.08, start, end) // higher: raised
	assert.Equal(t, 800.0, storedUsage(t, db, 1).TotalCost)
	spent, _ = ledger.Spent(1, start, end)
	assert.Equal(t, 800.0, spent)
}
