package models

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// On SQLite (a single Studio per file) the lock is a no-op.
func TestAcquireMigrationLockSQLiteIsNoop(t *testing.T) {
	db := setupTestDB(t)
	release, err := AcquireMigrationLock(context.Background(), db)
	require.NoError(t, err)
	release()
}
