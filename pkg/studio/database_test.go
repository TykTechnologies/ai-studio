package studio

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/config"
	"github.com/TykTechnologies/midsommar/v2/pkg/dbstate"
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

// The tracker's probe may have to dial every host in turn, twice each when
// sslmode allows a TLS and a plain attempt.
func TestProbeTimeoutCoversEveryAttempt(t *testing.T) {
	c, err := pgx.ParseConfig("postgres://u:p@pg-a:5432,pg-b:5432/db?target_session_attrs=read-write&connect_timeout=10&sslmode=disable")
	require.NoError(t, err)
	assert.Equal(t, 23*time.Second, probeTimeout(c), "two hosts, one attempt each")
	assert.Equal(t, 2, hostCount(c))

	c, err = pgx.ParseConfig("postgres://u:p@pg-a:5432,pg-b:5432/db?target_session_attrs=read-write&connect_timeout=10&sslmode=prefer")
	require.NoError(t, err)
	assert.Equal(t, 43*time.Second, probeTimeout(c), "two hosts, a TLS and a plain attempt each")
	assert.Equal(t, 2, hostCount(c), "still two servers")

	c, err = pgx.ParseConfig("postgres://u:p@pg-a:5432/db")
	require.NoError(t, err)
	c.ConnectTimeout = 0
	assert.Equal(t, dbstate.DefaultProbeTimeout, probeTimeout(c))
}
