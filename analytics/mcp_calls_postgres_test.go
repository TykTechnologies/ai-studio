package analytics

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/postgres"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDatabaseHandler_RecordMCPCalls_Postgres writes hostile Gateway values
// through the batched recorder on Postgres, which (unlike SQLite) rejects a
// varchar overflow, invalid UTF-8 and NUL, and fails the whole batch insert
// on any one of them. It drops and recreates mcp_call_records: point
// ANALYTICS_TEST_POSTGRES_DSN at a scratch database to run it.
func TestDatabaseHandler_RecordMCPCalls_Postgres(t *testing.T) {
	dsn := os.Getenv("ANALYTICS_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("ANALYTICS_TEST_POSTGRES_DSN not set")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.Migrator().DropTable(&models.MCPCallRecord{}))
	require.NoError(t, db.AutoMigrate(&models.MCPCallRecord{}))
	t.Cleanup(func() { db.Exec("DELETE FROM mcp_call_records") })

	h := NewDatabaseHandler(context.Background(), db)
	t.Cleanup(h.Stop)

	ts := time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)
	cred := strings.Repeat("c", 80)
	hostile := []models.MCPCallRecord{
		// the longest names a Gateway can pass on, in multi-byte characters
		{PrimitiveName: strings.Repeat("名", 600), SessionID: strings.Repeat("s", 500), TraceID: strings.Repeat("ü", 200)},
		// invalid UTF-8 in client-supplied values
		{PrimitiveName: "bad\xff\xfename", JSONRPCMethod: "tools/\xc3call", SessionID: "\xff"},
		// NUL in client-supplied values (midsommar-clean-n2i)
		{PrimitiveName: "x\x00", JSONRPCMethod: "tools/call\x00", SessionID: "\x00sess", TraceID: "t\x00"},
		// oversized identifiers
		{TykAPIID: strings.Repeat("a", 300), OrgID: strings.Repeat("o", 300), CredentialID: &cred, ProtocolVersion: strings.Repeat("9", 64), PrimitiveType: strings.Repeat("t", 64)},
	}
	batch := make([]models.MCPCallRecord, 0, 200)
	for i := 0; i < 196; i++ {
		batch = append(batch, models.MCPCallRecord{PrimitiveName: "search"})
	}
	batch = append(batch, hostile...)
	for i := range batch {
		batch[i].Timestamp = ts
		batch[i].ConnectionID = 1
		batch[i].Source = models.MCPCallSourceTykPump
	}

	h.RecordMCPCalls(context.Background(), batch)

	var n int64
	require.Eventually(t, func() bool {
		db.Model(&models.MCPCallRecord{}).Count(&n)
		return n == int64(len(batch))
	}, 10*time.Second, 50*time.Millisecond, "a hostile value lost the batch: %d of %d rows landed", n, len(batch))

	var rows []models.MCPCallRecord
	require.NoError(t, db.Where("primitive_name <> ?", "search").Order("id").Find(&rows).Error)
	require.Len(t, rows, len(hostile))
	for _, r := range rows {
		for _, s := range []string{r.PrimitiveName, r.JSONRPCMethod, r.SessionID, r.TraceID} {
			assert.NotContains(t, s, "\x00")
		}
	}
	assert.Equal(t, strings.Repeat("名", 170), rows[0].PrimitiveName)
	assert.Equal(t, "badname", rows[1].PrimitiveName)
	assert.Len(t, rows[3].TykAPIID, 64)
}
