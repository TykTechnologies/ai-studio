package studio

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/TykTechnologies/midsommar/v2/config"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}

// newTestOptions builds Options for a Studio on a fresh SQLite database. The
// database is a file, not :memory:, because background workers use their own
// connections.
func newTestOptions(t *testing.T) Options {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "studio.db")
	vals := map[string]string{
		"DATABASE_TYPE":         "sqlite",
		"DATABASE_URL":          dbPath,
		"TELEMETRY_ENABLED":     "false",
		"MARKETPLACE_ENABLED":   "false",
		"DOCS_DISABLED":         "true",
		"TYK_AI_SECRET_KEY":     "embedded-test-secret-key",
		"EXPORT_STORAGE_PATH":   filepath.Join(dir, "exports"),
		"BRANDING_STORAGE_PATH": filepath.Join(dir, "branding"),
		"SKIP_FILTER_DEFAULTS":  "true",
	}
	conf := config.LoadFrom(func(k string) string { return vals[k] })
	t.Cleanup(config.ResetGlobalConfig)

	db, err := gorm.Open(sqlite.Open(dbPath+"?_busy_timeout=5000"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})

	return Options{
		Config:          conf,
		DB:              db,
		Version:         "test",
		SkipLLMDefaults: true,
		UIAssets: fstest.MapFS{
			"index.html": {Data: []byte("<html>studio</html>")},
		},
	}
}

func stopStudio(t *testing.T, s *Studio) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	require.NoError(t, s.Stop(ctx))
}

func listenLocal() (net.Listener, error) { return net.Listen("tcp", "127.0.0.1:0") }

func dialLocal(addr string) (net.Conn, error) { return net.Dial("tcp", addr) }
