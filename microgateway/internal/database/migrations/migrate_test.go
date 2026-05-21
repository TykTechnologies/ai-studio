package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestNewMigratorRejectsSQLite(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	_, err = NewMigrator(db)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "support postgres only")
}

func TestControlPayloadMigrationUsesPostgresBinaryTypes(t *testing.T) {
	sqlBytes, err := migrationFiles.ReadFile("006_add_control_payloads.up.sql")
	require.NoError(t, err)

	migrationSQL := strings.ToLower(string(sqlBytes))
	assert.NotContains(t, migrationSQL, " blob ")
	assert.NotContains(t, migrationSQL, "autoincrement")
	assert.Contains(t, migrationSQL, "payload bytea not null")
}
