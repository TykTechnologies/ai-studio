package models

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/postgres"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm/logger"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/testutil/schemasnapshot"
)

// migrateStudioSchema runs every migration Studio runs at startup.
func migrateStudioSchema(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, InitModels(db))
	require.NoError(t, MigrateProfiles(db))
	require.NoError(t, db.AutoMigrate(&KVPair{}))
	require.NoError(t, db.AutoMigrate(AnalyticsModels()...)) // analytics.Migrate

}

// The schema Studio's migrations produce is pinned by golden files, so a
// change underneath them (a gorm upgrade, a custom column type that stops
// implementing GormDBDataType) cannot alter it unnoticed. A deliberate model
// change updates the goldens with make schema-golden.
func TestStudioSchemaSnapshot_SQLite(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "schema.db")),
		&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})
	migrateStudioSchema(t, db)
	schemasnapshot.AssertGolden(t, db, filepath.Join("testdata", "schema", "sqlite.golden"))
}

// The Postgres golden is generated on postgres:16, the version CI runs; the
// test works in a schema of its own, so it can share the database.
func TestStudioSchemaSnapshot_PostgreSQL(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set")
	}
	cfg := &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)}
	admin, err := gorm.Open(postgres.Open(dsn), cfg)
	require.NoError(t, err)
	schema := fmt.Sprintf("schema_snapshot_%d", time.Now().UnixNano())
	require.NoError(t, admin.Exec("CREATE SCHEMA "+schema).Error)
	t.Cleanup(func() {
		admin.Exec("DROP SCHEMA " + schema + " CASCADE")
		if sqlDB, err := admin.DB(); err == nil {
			sqlDB.Close()
		}
	})

	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	db, err := gorm.Open(postgres.Open(dsn+sep+"search_path="+schema), cfg)
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})
	migrateStudioSchema(t, db)
	schemasnapshot.AssertGolden(t, db, filepath.Join("testdata", "schema", "postgres.golden"))
}
