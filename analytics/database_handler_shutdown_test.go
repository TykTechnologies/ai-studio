package analytics

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm/logger"
	"github.com/stretchr/testify/require"
)

// Stopping the handler while requests are still recording must not panic.
// A recorder checks recStarted, releases the lock and then sends; the worker
// used to close the channels on shutdown, so that send could land on a closed
// channel (a panic, and a data race under -race).
func TestDatabaseHandler_StopWhileRecording(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "analytics.db")), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})

	for round := 0; round < 20; round++ {
		h := NewDatabaseHandler(context.Background(), db)
		ctx := context.Background()

		var wg sync.WaitGroup
		stop := make(chan struct{})
		for g := 0; g < 16; g++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for {
					select {
					case <-stop:
						return
					default:
					}
					h.RecordChatRecord(ctx, &models.LLMChatRecord{Name: "m"})
					h.RecordProxyLog(ctx, &models.ProxyLog{})
					h.RecordToolCall(ctx, "tool", time.Now(), 1, 1)
					h.RecordChatLogEntry(ctx, &models.LLMChatLogEntry{})
					h.RecordChatRecordsBatch(ctx, []*models.LLMChatRecord{{Name: "m"}})
					h.RecordProxyLogsBatch(ctx, []*models.ProxyLog{{}})
					h.RecordComplianceEvents(ctx, []*models.ComplianceEvent{{}})
				}
			}()
		}

		time.Sleep(5 * time.Millisecond)
		h.Stop()
		time.Sleep(5 * time.Millisecond)
		close(stop)
		wg.Wait()
	}
}
