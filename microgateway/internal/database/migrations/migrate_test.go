package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestControlPayloadSQLMigrationKeepsSQLiteAutoincrement(t *testing.T) {
	sqlBytes, err := migrationFiles.ReadFile("006_add_control_payloads.up.sql")
	require.NoError(t, err)

	migrationSQL := strings.ToLower(string(sqlBytes))
	assert.Contains(t, migrationSQL, "id integer primary key autoincrement")
	assert.Contains(t, migrationSQL, "payload blob not null")
}
