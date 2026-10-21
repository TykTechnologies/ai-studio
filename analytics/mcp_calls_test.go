package analytics

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mcpCallsDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "analytics.db")), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})
	require.NoError(t, Migrate(db))
	return db
}

// The database handler writes MCP call batches through its worker, like the
// other batched record types, and every row of every batch lands.
func TestDatabaseHandler_RecordMCPCalls(t *testing.T) {
	db := mcpCallsDB(t)
	h := NewDatabaseHandler(context.Background(), db)
	t.Cleanup(h.Stop)

	server, app := uint(4), uint(9)
	cred := "6f1c1a52-6a4e-4a39-9a43-6c1b8f0e2b11"
	ts := time.Now().UTC().Truncate(time.Second)
	const batches, perBatch = 5, 250
	for b := 0; b < batches; b++ {
		rows := make([]models.MCPCallRecord, perBatch)
		for i := range rows {
			rows[i] = models.MCPCallRecord{
				Timestamp:        ts,
				ConnectionID:     2,
				MCPServerID:      &server,
				TykAPIID:         "api-1",
				AppID:            &app,
				CredentialID:     &cred,
				JSONRPCMethod:    "tools/call",
				PrimitiveType:    "tool",
				PrimitiveName:    "search",
				JSONRPCErrorCode: -32602,
				ResponseCode:     200,
				LatencyTotalMs:   12,
				Source:           models.MCPCallSourceTykPump,
			}
		}
		h.RecordMCPCalls(context.Background(), rows)
	}

	require.Eventually(t, func() bool {
		var n int64
		db.Model(&models.MCPCallRecord{}).Count(&n)
		return n == batches*perBatch
	}, 10*time.Second, 20*time.Millisecond)

	var got models.MCPCallRecord
	require.NoError(t, db.First(&got).Error)
	assert.Equal(t, uint(2), got.ConnectionID)
	require.NotNil(t, got.MCPServerID)
	assert.Equal(t, server, *got.MCPServerID)
	require.NotNil(t, got.AppID)
	assert.Equal(t, app, *got.AppID)
	require.NotNil(t, got.CredentialID)
	assert.Equal(t, cred, *got.CredentialID)
	assert.Equal(t, "tools/call", got.JSONRPCMethod)
	assert.Equal(t, -32602, got.JSONRPCErrorCode)
	assert.True(t, ts.Equal(got.Timestamp))
}

// A value longer than its column is cut, not left to fail the batch insert
// (Postgres rejects a varchar overflow and invalid UTF-8).
func TestMCPCallRecord_ClampLengths(t *testing.T) {
	cred := strings.Repeat("c", 50)
	r := models.MCPCallRecord{
		PrimitiveName: strings.Repeat("é", 400), // 800 bytes
		SessionID:     "ok\xffsession",
		TraceID:       "a\x00b",
		CredentialID:  &cred,
		TykAPIID:      strings.Repeat("a", 100),
	}
	r.ClampLengths()
	assert.LessOrEqual(t, len(r.PrimitiveName), 512)
	assert.Equal(t, strings.Repeat("é", 256), r.PrimitiveName)
	assert.Equal(t, "oksession", r.SessionID)
	assert.Equal(t, "ab", r.TraceID, "NUL fails a Postgres insert")
	assert.Len(t, *r.CredentialID, 36)
	assert.Len(t, r.TykAPIID, 64)
}

// Rows recorded on a stopped handler are dropped, never written.
func TestDatabaseHandler_RecordMCPCallsAfterStop(t *testing.T) {
	db := mcpCallsDB(t)
	h := NewDatabaseHandler(context.Background(), db)
	h.Stop()
	err := h.RecordMCPCalls(context.Background(), []models.MCPCallRecord{{ConnectionID: 1, Timestamp: time.Now()}})
	assert.ErrorIs(t, err, ErrRecorderBusy, "the caller learns the batch was not taken")
	time.Sleep(100 * time.Millisecond)
	var n int64
	require.NoError(t, db.Model(&models.MCPCallRecord{}).Count(&n).Error)
	assert.Zero(t, n)
}
