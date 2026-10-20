package analytics

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The real handler under back-pressure: while another connection holds the
// database's write lock the MCP writer stalls, its buffer fills, and
// RecordMCPCalls refuses with ErrRecorderBusy (the ingest endpoint turns
// that into a 503 the pump retries) instead of dropping silently. Every
// batch it did take lands once the lock is released.
func TestDatabaseHandler_RecordMCPCallsBusy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "analytics.db")
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, Migrate(db))
	h := NewDatabaseHandler(context.Background(), db)
	t.Cleanup(h.Stop)

	locker, err := gorm.Open(sqlite.Open(path), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	lockDB, err := locker.DB()
	require.NoError(t, err)
	t.Cleanup(func() { lockDB.Close() })
	conn, err := lockDB.Conn(context.Background())
	require.NoError(t, err)
	_, err = conn.ExecContext(context.Background(), "BEGIN EXCLUSIVE")
	require.NoError(t, err)

	const perBatch = 10
	batch := func() []models.MCPCallRecord {
		rows := make([]models.MCPCallRecord, perBatch)
		for i := range rows {
			rows[i] = models.MCPCallRecord{Timestamp: time.Now(), ConnectionID: 1, PrimitiveName: "t", Source: models.MCPCallSourceTykPump}
		}
		return rows
	}
	accepted, busy := 0, false
	for i := 0; i < 1000 && !busy; i++ {
		switch err := h.RecordMCPCalls(context.Background(), batch()); {
		case err == nil:
			accepted++
		default:
			assert.ErrorIs(t, err, ErrRecorderBusy)
			busy = true
		}
	}
	require.True(t, busy, "a stalled writer must refuse batches, not queue without bound")
	assert.Greater(t, accepted, 0)

	// release quickly, inside the writer's lock retries
	_, err = conn.ExecContext(context.Background(), "ROLLBACK")
	require.NoError(t, err)
	require.NoError(t, conn.Close())

	want := int64(accepted * perBatch)
	var n int64
	require.Eventually(t, func() bool {
		db.Model(&models.MCPCallRecord{}).Count(&n)
		return n == want
	}, 15*time.Second, 50*time.Millisecond, "accepted %d batches, %d of %d rows landed", accepted, n, want)

	// and the writer takes batches again
	require.NoError(t, h.RecordMCPCalls(context.Background(), batch()))
}
