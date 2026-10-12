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

// The service tier of edge usage (a batch accounted on the edge) reaches the
// edge's analytics row, which the pulse sends to the hub.
func TestRecordExchange_ServiceTierReachesTheAnalyticsRow(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, database.Migrate(db))
	handler := NewMicrogatewaAnalyticsHandler(db, nil, nil, nil)
	analytics.SetHandler(handler)
	t.Cleanup(func() { analytics.SetHandler(nil) })

	now := time.Now()
	analytics.RecordExchange(context.Background(),
		&models.ProxyLog{AppID: 1, LLMID: 2, Vendor: "anthropic", ResponseCode: 200, TimeStamp: now, ServiceTier: "batch"},
		&models.LLMChatRecord{AppID: 1, LLMID: 2, Vendor: "anthropic", Name: "claude-luna", TotalTokens: 10, Cost: 5,
			TimeStamp: now, ServiceTier: "batch"})
	analytics.RecordExchange(context.Background(),
		&models.ProxyLog{AppID: 1, LLMID: 2, Vendor: "anthropic", ResponseCode: 200, TimeStamp: now},
		&models.LLMChatRecord{AppID: 1, LLMID: 2, Vendor: "anthropic", TotalTokens: 9, TimeStamp: now})

	var events []database.AnalyticsEvent
	require.NoError(t, db.Order("id asc").Find(&events).Error)
	require.Len(t, events, 2)
	assert.Equal(t, "batch", events[0].ServiceTier)
	assert.Equal(t, "", events[1].ServiceTier)
}
