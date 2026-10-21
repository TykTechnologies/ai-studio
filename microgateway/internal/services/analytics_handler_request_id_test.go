package services

import (
	"context"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/v2/analytics"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/requestid"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The edge's analytics row carries the gateway's request id from the request
// context (analytics stamps it) beside its own row key, for every attempt,
// blocked requests included.
func TestRecordExchange_GatewayRequestIDReachesTheAnalyticsRow(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, database.Migrate(db))
	handler := NewMicrogatewaAnalyticsHandler(db, nil, nil, nil)
	analytics.SetHandler(handler)
	t.Cleanup(func() { analytics.SetHandler(nil) })

	id := requestid.New()
	ctx := requestid.WithID(context.Background(), id, "lb-1")
	now := time.Now()
	// A failed primary attempt, then a blocked request: neither has usage.
	analytics.RecordExchange(ctx, &models.ProxyLog{AppID: 1, LLMID: 2, Vendor: "openai", ResponseCode: 503, TimeStamp: now}, nil)
	analytics.RecordExchange(ctx, &models.ProxyLog{AppID: 1, LLMID: 3, Vendor: "openai", ResponseCode: 400, TimeStamp: now}, nil)

	var events []database.AnalyticsEvent
	require.NoError(t, db.Order("id asc").Find(&events).Error)
	require.Len(t, events, 2)
	assert.NotEqual(t, events[0].RequestID, events[1].RequestID, "row keys stay unique per attempt")
	for _, e := range events {
		assert.Equal(t, id, e.GatewayRequestID)
		assert.Equal(t, "lb-1", e.ClientRequestID)
	}
}
