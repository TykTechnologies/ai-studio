package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestControlPayloadMigrationUsesPostgresBinaryTypes(t *testing.T) {
	sqlBytes, err := migrationFiles.ReadFile("006_add_control_payloads.up.sql")
	require.NoError(t, err)

	migrationSQL := strings.ToLower(string(sqlBytes))
	assert.NotContains(t, migrationSQL, " blob ")
	assert.NotContains(t, migrationSQL, "autoincrement")
	assert.Contains(t, migrationSQL, "payload bytea not null")
}
