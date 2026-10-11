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

// A decision call's model class and question counts reach the edge's
// analytics row, which the pulse sends to the hub.
func TestRecordExchange_DecisionRowsCarryTheRequestKind(t *testing.T) {
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
		&models.ProxyLog{AppID: 1, LLMID: 2, Vendor: "systemone", ResponseCode: 200, TimeStamp: now,
			RequestKind: models.DecisionRequest},
		&models.LLMChatRecord{AppID: 1, LLMID: 2, Vendor: "systemone", Name: "jev-1.13.0", PromptTokens: 200,
			TotalTokens: 200, Choices: 2, TimeStamp: now,
			RequestKind: models.DecisionRequest, DecisionQuestions: 3, DecisionRefusals: 1})
	// A chat exchange stays generative.
	analytics.RecordExchange(context.Background(),
		&models.ProxyLog{AppID: 1, LLMID: 3, Vendor: "openai", ResponseCode: 200, TimeStamp: now},
		&models.LLMChatRecord{AppID: 1, LLMID: 3, Vendor: "openai", TotalTokens: 9, TimeStamp: now})

	var events []database.AnalyticsEvent
	require.NoError(t, db.Order("id asc").Find(&events).Error)
	require.Len(t, events, 2)
	assert.Equal(t, "decision", events[0].RequestKind)
	assert.Equal(t, 3, events[0].DecisionQuestions)
	assert.Equal(t, 1, events[0].DecisionRefusals)
	assert.Equal(t, 2, events[0].Choices)
	assert.Equal(t, "", events[1].RequestKind)
	assert.Equal(t, 0, events[1].DecisionQuestions)
}
