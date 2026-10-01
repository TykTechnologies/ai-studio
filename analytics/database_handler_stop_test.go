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
	"github.com/stretchr/testify/require"
)

// Nothing is written after Stop returns, even with records still buffered:
// a stopped handler's worker used to go on draining its buffers (select
// picks among ready cases at random), writing into a database its owner was
// about to close or remove.
func TestDatabaseHandler_NoWritesAfterStop(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "analytics.db")), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})
	require.NoError(t, Migrate(db))

	h := NewDatabaseHandler(context.Background(), db)
	ctx := context.Background()
	for i := 0; i < 500; i++ {
		h.RecordProxyLog(ctx, &models.ProxyLog{})
	}
	h.Stop()

	count := func() int64 {
		var n int64
		require.NoError(t, db.Model(&models.ProxyLog{}).Count(&n).Error)
		return n
	}
	after := count()
	time.Sleep(300 * time.Millisecond)
	require.Equal(t, after, count(), "the handler wrote records after Stop returned")
}
