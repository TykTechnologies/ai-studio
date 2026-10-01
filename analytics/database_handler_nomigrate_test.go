package analytics

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm/logger"
	"github.com/stretchr/testify/require"
)

// Starting the recorder must not change the schema: an instance that writes
// analytics into a database another instance owns (a headless control plane
// next to the full Studio) must not run DDL there, and Studio's own
// migrations run under the migration lock before recording starts.
func TestDatabaseHandler_StartDoesNotMigrate(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "analytics.db")), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})

	ctx, cancel := context.WithCancel(context.Background())
	h := NewDatabaseHandler(ctx, db)
	t.Cleanup(func() {
		cancel()
		h.Stop()
	})

	for _, table := range []interface{}{
		&models.LLMChatRecord{}, &models.LLMChatLogEntry{}, &models.ToolCallRecord{},
		&models.ProxyLog{}, &models.ComplianceEvent{},
	} {
		require.False(t, db.Migrator().HasTable(table), "starting the recorder created %T's table", table)
	}

	require.NoError(t, Migrate(db))
	require.True(t, db.Migrator().HasTable(&models.ProxyLog{}))
}
