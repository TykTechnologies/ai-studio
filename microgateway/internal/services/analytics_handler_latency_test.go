package services

import (
	"context"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/v2/analytics"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every edge analytics row carries its request's latency (total_time_ms),
// taken from the start the proxy put on the context. It was 0 on every row,
// errors included.
func TestRecordExchange_RowsCarryLatency(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, database.Migrate(db))
	handler := NewMicrogatewaAnalyticsHandler(db, nil, nil, nil)

	end := time.Now()
	ctx := analytics.WithRequestStart(context.Background(), end.Add(-180*time.Millisecond))

	// An error: a proxy log with no chat record.
	handler.RecordExchange(ctx, &models.ProxyLog{AppID: 1, LLMID: 2, Vendor: "openai", ResponseCode: 502, TimeStamp: end}, nil)
	// A served request whose record carries no latency of its own.
	handler.RecordExchange(ctx, &models.ProxyLog{AppID: 1, LLMID: 2, Vendor: "openai", ResponseCode: 200, TimeStamp: end},
		&models.LLMChatRecord{AppID: 1, LLMID: 2, Vendor: "openai", TotalTokens: 9, TimeStamp: end})
	// A record that does carry one keeps it.
	handler.RecordExchange(ctx, &models.ProxyLog{AppID: 1, LLMID: 2, Vendor: "openai", ResponseCode: 200, TimeStamp: end},
		&models.LLMChatRecord{AppID: 1, LLMID: 2, Vendor: "openai", TotalTokens: 9, TotalTimeMS: 42, TimeStamp: end})

	var events []database.AnalyticsEvent
	require.NoError(t, db.Order("id").Find(&events).Error)
	require.Len(t, events, 3)
	assert.Equal(t, 180, events[0].TotalTimeMS, "error row")
	assert.Equal(t, 180, events[1].TotalTimeMS, "served row")
	assert.Equal(t, 42, events[2].TotalTimeMS, "the record's own latency")
}
