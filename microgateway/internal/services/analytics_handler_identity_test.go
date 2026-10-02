package services

import (
	"context"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/v2/analytics"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/authidentity"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The subject and acting agent an auth plugin named reach the edge's
// analytics row, which the pulse sends to the hub.
func TestRecordExchange_RowsCarryTheIdentity(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, database.Migrate(db))
	handler := NewMicrogatewaAnalyticsHandler(db, nil, nil, nil)
	analytics.SetHandler(handler)
	t.Cleanup(func() { analytics.SetHandler(nil) })

	ctx := authidentity.With(context.Background(), &authidentity.Identity{
		AppID: 1, Method: authidentity.MethodPlugin, Subject: "alice@example.com",
		Claims: map[string]string{authidentity.ClaimActor: "agent-7"},
	})
	now := time.Now()
	analytics.RecordExchange(ctx,
		&models.ProxyLog{AppID: 1, LLMID: 2, Vendor: "openai", ResponseCode: 200, TimeStamp: now},
		&models.LLMChatRecord{AppID: 1, LLMID: 2, Vendor: "openai", TotalTokens: 9, TimeStamp: now})

	var events []database.AnalyticsEvent
	require.NoError(t, db.Find(&events).Error)
	require.Len(t, events, 1)
	assert.Equal(t, "alice@example.com", events[0].OnBehalfOf)
	assert.Equal(t, "agent-7", events[0].ActingAgent)
}
