package studio

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/config"
)

func TestOpenDatabase(t *testing.T) {
	t.Run("sqlite", func(t *testing.T) {
		conf := &config.AppConf{DatabaseType: "sqlite", DatabaseURL: filepath.Join(t.TempDir(), "studio.db")}
		db, err := OpenDatabase(conf)
		require.NoError(t, err)
		sqlDB, err := db.DB()
		require.NoError(t, err)
		assert.NoError(t, sqlDB.Ping())
		assert.NoError(t, sqlDB.Close())
	})

	t.Run("unsupported type", func(t *testing.T) {
		_, err := OpenDatabase(&config.AppConf{DatabaseType: "mysql", DatabaseURL: "x"})
		assert.ErrorContains(t, err, `unsupported database type: "mysql"`)
	})

	t.Run("unreachable", func(t *testing.T) {
		_, err := OpenDatabase(&config.AppConf{DatabaseType: "sqlite", DatabaseURL: filepath.Join(t.TempDir(), "missing", "studio.db")})
		assert.Error(t, err)
	})
}

// Without pkg/studio/sqlitedb registered, asking for SQLite names the package
// to import instead of failing as an unknown type.
func TestOpenDatabaseSQLiteNotRegistered(t *testing.T) {
	databaseDriversMu.Lock()
	open := databaseDrivers["sqlite"]
	delete(databaseDrivers, "sqlite")
	databaseDriversMu.Unlock()
	t.Cleanup(func() { RegisterDatabaseDriver("sqlite", open) })

	_, err := OpenDatabase(&config.AppConf{DatabaseType: "sqlite", DatabaseURL: filepath.Join(t.TempDir(), "studio.db")})
	if err == nil || !strings.Contains(err.Error(), "pkg/studio/sqlitedb") {
		t.Fatalf("err = %v, want one naming pkg/studio/sqlitedb", err)
	}
}
