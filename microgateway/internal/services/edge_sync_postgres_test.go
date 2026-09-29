package services

import (
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// openPostgresSchema connects to MGW_TEST_POSTGRES_DSN in a schema of its own,
// dropped when the test ends, and migrates it.
func openPostgresSchema(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("MGW_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("MGW_TEST_POSTGRES_DSN not set")
	}
	schema := fmt.Sprintf("edge_sync_%d", time.Now().UnixNano())
	admin, err := database.Connect(database.DatabaseConfig{Type: "postgres", DSN: dsn, MaxOpenConns: 2, MaxIdleConns: 2, LogLevel: "silent"})
	require.NoError(t, err)
	require.NoError(t, admin.Exec("CREATE SCHEMA "+schema).Error)
	t.Cleanup(func() {
		admin.Exec("DROP SCHEMA " + schema + " CASCADE")
		database.Close(admin)
	})

	u, err := url.Parse(dsn)
	require.NoError(t, err)
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	db, err := database.Connect(database.DatabaseConfig{Type: "postgres", DSN: u.String(), MaxOpenConns: 4, MaxIdleConns: 4, LogLevel: "silent"})
	require.NoError(t, err)
	db.Logger = logger.Default.LogMode(logger.Silent)
	t.Cleanup(func() { database.Close(db) })
	require.NoError(t, database.Migrate(db))
	return db
}

// Postgres enforces the foreign keys from analytics_events and budget_usage:
// a resync must not delete Apps or LLMs they reference.
func TestEdgeSyncResyncWithReferencingRowsPostgres(t *testing.T) {
	db := openPostgresSchema(t)
	testEdgeSyncResyncWithReferencingRows(t, db, 1)
}

// An App hard-deleted while its usage waits in the ledger fails that usage's
// write on Postgres; the other Apps' usage is still written, and the lost
// App's is dropped instead of failing every later flush.
func TestAnalyticsWriterLedgerDeletedAppPostgres(t *testing.T) {
	db := openPostgresSchema(t)
	a := &database.App{Name: "kept", IsActive: true}
	g := &database.App{Name: "deleted", IsActive: true}
	require.NoError(t, db.Create(a).Error)
	require.NoError(t, db.Create(g).Error)
	start, end := ledgerPeriod()
	ledger := NewBudgetLedger(db)
	ledger.Add(a.ID, start, end, 10, 100, 5, 5)
	ledger.Add(g.ID, start, end, 10, 90, 5, 5)
	require.NoError(t, db.Unscoped().Delete(g).Error)

	w := NewAnalyticsWriter(db, 10, 500, time.Hour)
	w.SetLedger(ledger)
	w.Start()
	w.Stop()
	assert.Equal(t, 100.0, storedUsage(t, db, a.ID).TotalCost)
	assert.True(t, ledger.take().empty(), "the deleted App's usage is not retried")

	// Later usage of the kept App keeps flowing.
	ledger.Add(a.ID, start, end, 10, 50, 5, 5)
	w = NewAnalyticsWriter(db, 10, 500, time.Hour)
	w.SetLedger(ledger)
	w.Start()
	w.Stop()
	assert.Equal(t, 150.0, storedUsage(t, db, a.ID).TotalCost)
}
