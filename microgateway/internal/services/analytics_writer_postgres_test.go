package services

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm/logger"
)

// TestAnalyticsWriterPostgres runs the writer, the ledger and the budget sync
// against Postgres, whose SQL differs from SQLite's for the parts that matter
// here (ON CONFLICT, GREATEST, enforced foreign keys). Set
// MGW_TEST_POSTGRES_DSN to run it; each run uses its own App and request IDs.
func TestAnalyticsWriterPostgres(t *testing.T) {
	dsn := os.Getenv("MGW_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("MGW_TEST_POSTGRES_DSN not set")
	}
	cfg := database.DatabaseConfig{Type: "postgres", DSN: dsn, MaxOpenConns: 8, MaxIdleConns: 8, LogLevel: "silent"}
	db, err := database.Connect(cfg)
	require.NoError(t, err)
	db.Logger = logger.Default.LogMode(logger.Silent)
	require.NoError(t, database.Migrate(db))
	wdb, err := database.OpenWriter(cfg, db)
	require.NoError(t, err)
	require.True(t, wdb == db, "Postgres shares one pool")

	run := fmt.Sprint(time.Now().UnixNano())
	id := func(s string) string { return s + "-" + run }
	app := &database.App{Name: id("pg-writer"), IsActive: true}
	require.NoError(t, db.Create(app).Error)
	start, end := ledgerPeriod()

	// Duplicate request ID and a row with a dangling foreign key: only
	// those two are lost.
	require.NoError(t, db.Create(&database.AnalyticsEvent{RequestID: id("pg-dup"), AppID: app.ID, TimeStamp: time.Now()}).Error)
	missingLLM := uint(999999)
	ledger := NewBudgetLedger(db)
	ledger.Add(app.ID, start, end, 10, 100, 5, 5)
	w := NewAnalyticsWriter(wdb, 10, 500, time.Hour)
	w.SetLedger(ledger)
	w.Enqueue(&database.AnalyticsEvent{RequestID: id("pg-dup"), AppID: app.ID, TimeStamp: time.Now()})
	w.Enqueue(&database.AnalyticsEvent{RequestID: id("pg-1"), AppID: app.ID, TimeStamp: time.Now()})
	w.Enqueue(&database.AnalyticsEvent{RequestID: id("pg-fk"), AppID: app.ID, LLMID: &missingLLM, TimeStamp: time.Now()})
	w.Enqueue(&database.AnalyticsEvent{RequestID: id("pg-2"), AppID: app.ID, TimeStamp: time.Now()})
	w.Start()
	w.Stop()

	var ids []string
	require.NoError(t, db.Model(&database.AnalyticsEvent{}).Where("app_id = ?", app.ID).Order("request_id").Pluck("request_id", &ids).Error)
	assert.Equal(t, []string{id("pg-1"), id("pg-2"), id("pg-dup")}, ids)
	assert.Equal(t, 100.0, storedUsage(t, db, app.ID).TotalCost)

	// Increments on the existing row, then the sync's max.
	ledger.Add(app.ID, start, end, 10, 50, 5, 5)
	w = NewAnalyticsWriter(wdb, 10, 500, time.Hour)
	w.SetLedger(ledger)
	w.Start()
	w.Stop()
	assert.Equal(t, 150.0, storedUsage(t, db, app.ID).TotalCost)

	h := &BudgetSyncHandler{db: db, ledger: ledger}
	h.updateLocalBudget(app.ID, 0.01, start, end) // 100 < 150: kept
	assert.Equal(t, 150.0, storedUsage(t, db, app.ID).TotalCost)
	h.updateLocalBudget(app.ID, 0.03, start, end) // 300: raised
	assert.Equal(t, 300.0, storedUsage(t, db, app.ID).TotalCost)
	spent, err := ledger.Spent(app.ID, start, end)
	require.NoError(t, err)
	assert.Equal(t, 300.0, spent)
}
