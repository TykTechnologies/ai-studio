package services

import (
	"context"
	"sync"
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

// usageRecorder is a budget service that keeps the usage recorded on it.
type usageRecorder struct {
	BudgetServiceInterface
	mu    sync.Mutex
	costs []float64
	llms  []uint
}

func (u *usageRecorder) RecordUsage(appID uint, llmID *uint, tokens int64, cost float64, promptTokens, completionTokens int64) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.costs = append(u.costs, cost)
	if llmID != nil {
		u.llms = append(u.llms, *llmID)
	}
	return nil
}

// Batch usage accounted on an edge (proxy.recordAsyncUsage) is a proxy log
// plus a chat record handed to analytics.RecordExchange with no request on
// the context: the row reaches the edge's analytics events (which the pulse
// sends to the hub) with the batch price the edge applied and its identity,
// and the edge budget records the same cost.
func TestRecordExchange_NativeBatchUsageReachesEventsAndBudget(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, database.Migrate(db))
	budget := &usageRecorder{}
	handler := NewMicrogatewaAnalyticsHandler(db, nil, nil, budget)
	analytics.SetHandler(handler)
	t.Cleanup(func() { analytics.SetHandler(nil) })

	now := time.Now()
	// 1500 prompt + 300 completion tokens at half of a $2/$10 per MTok price,
	// in the gateway's ×10000 cost units.
	const batchCost = (1500*0.000002 + 300*0.00001) * 0.5 * 10000
	analytics.RecordExchange(context.Background(),
		&models.ProxyLog{AppID: 4, UserID: 9, LLMID: 2, Vendor: "anthropic", ModelName: "Claude", ResponseCode: 200, TimeStamp: now,
			ResponseBody: `{"native_resource":"anthropic.message_batch","model":"claude-luna","requests":2}`,
			OnBehalfOf:   "user:alice", ActingAgent: "agent:bot"},
		&models.LLMChatRecord{AppID: 4, UserID: 9, LLMID: 2, Vendor: "anthropic", Name: "claude-luna",
			PromptTokens: 1500, ResponseTokens: 300, TotalTokens: 1800, Cost: batchCost, Currency: "USD", TimeStamp: now,
			OnBehalfOf: "user:alice", ActingAgent: "agent:bot", InteractionType: models.ProxyInteraction})

	var events []database.AnalyticsEvent
	require.Eventually(t, func() bool {
		return db.Find(&events).Error == nil && len(events) == 1
	}, 2*time.Second, 20*time.Millisecond)
	ev := events[0]
	assert.Equal(t, uint(4), ev.AppID)
	assert.Equal(t, 1500, ev.PromptTokens)
	assert.Equal(t, 300, ev.ResponseTokens)
	assert.InDelta(t, batchCost, ev.Cost, 1e-9, "the hub stores the edge's cost as is")
	assert.Equal(t, "user:alice", ev.OnBehalfOf)
	assert.Equal(t, "agent:bot", ev.ActingAgent)

	budget.mu.Lock()
	defer budget.mu.Unlock()
	require.Len(t, budget.costs, 1)
	assert.InDelta(t, batchCost, budget.costs[0], 1e-9)
	assert.Equal(t, []uint{2}, budget.llms)
}
