package models

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/callbacks"
	"gorm.io/gorm/logger"
)

// gorm finds model hooks by type assertion at runtime, so a hook whose
// signature names a different gorm package (a stray import after a gorm
// move) compiles but never runs. These assertions tie every model hook to
// the callbacks interfaces of the gorm this package builds with; a mismatch
// fails to compile. Add new hooks here.
var (
	_ callbacks.BeforeSaveInterface = (*MCPCredential)(nil)
	_ callbacks.AfterSaveInterface  = (*MCPCredential)(nil)
	_ callbacks.AfterFindInterface  = (*MCPCredential)(nil)

	_ callbacks.AfterSaveInterface   = (*LLM)(nil)
	_ callbacks.AfterDeleteInterface = (*LLM)(nil)

	_ callbacks.BeforeSaveInterface  = (*Tool)(nil)
	_ callbacks.AfterSaveInterface   = (*Tool)(nil)
	_ callbacks.AfterDeleteInterface = (*Tool)(nil)

	_ callbacks.AfterSaveInterface   = (*Datasource)(nil)
	_ callbacks.AfterDeleteInterface = (*Datasource)(nil)

	_ callbacks.AfterSaveInterface   = (*Embedder)(nil)
	_ callbacks.AfterDeleteInterface = (*Embedder)(nil)

	_ callbacks.BeforeSaveInterface = (*Submission)(nil)
	_ callbacks.AfterFindInterface  = (*Submission)(nil)

	_ callbacks.BeforeSaveInterface = (*TykConnection)(nil)
	_ callbacks.AfterSaveInterface  = (*TykConnection)(nil)
	_ callbacks.AfterFindInterface  = (*TykConnection)(nil)

	_ callbacks.BeforeSaveInterface = (*WebhookTarget)(nil)
	_ callbacks.AfterSaveInterface  = (*WebhookTarget)(nil)
	_ callbacks.AfterFindInterface  = (*WebhookTarget)(nil)
)

// A live check that hooks fire: a Dashboard access token is stored encrypted
// and read back in plaintext.
func TestModelHooksFire(t *testing.T) {
	t.Setenv("TYK_AI_SECRET_KEY", "hooks-canary-secret-key")
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "hooks.db")),
		&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})
	require.NoError(t, db.AutoMigrate(&TykConnection{}))

	conn := &TykConnection{Name: "canary", DashboardURL: "http://dashboard.invalid", DashboardAccessToken: "plain-token"}
	require.NoError(t, db.Create(conn).Error)
	assert.Equal(t, "plain-token", conn.DashboardAccessToken, "AfterSave restores plaintext")

	var stored string
	require.NoError(t, db.Raw("SELECT dashboard_access_token FROM tyk_connections WHERE id = ?", conn.ID).Scan(&stored).Error)
	assert.True(t, strings.HasPrefix(stored, tykEncryptedPrefix), "BeforeSave encrypts at rest, got %q", stored)

	var loaded TykConnection
	require.NoError(t, db.First(&loaded, conn.ID).Error)
	assert.Equal(t, "plain-token", loaded.DashboardAccessToken, "AfterFind decrypts")
}
