package analytics

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm/logger"
)

// A stopped Studio's recorder is gone once ResetHandler returns: its worker
// must not outlive it into the next Studio started in the process.
func TestResetHandlerWaitsForAStoppedRecorder(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "analytics.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})

	ctx, cancel := context.WithCancel(context.Background())
	h := NewDatabaseHandler(ctx, db)
	SetHandler(h)
	t.Cleanup(func() { SetHandler(nil) })

	cancel()
	ResetHandler()
	select {
	case <-h.workerDone:
	default:
		t.Fatal("ResetHandler returned before the stopped recorder's worker")
	}
	assert.Nil(t, GetHandler())

	// A running recorder is not waited for.
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	running := NewDatabaseHandler(ctx2, db)
	SetHandler(running)
	done := make(chan struct{})
	go func() { ResetHandler(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("ResetHandler waited for a running recorder")
	}
}
