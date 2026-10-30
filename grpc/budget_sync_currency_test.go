package grpc

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/eventbridge"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A budget's currency is implied by its spend: the hub counts a
// single-currency period as it is (EUR-only included), and tells edges to
// refuse a budgeted App or LLM whose period mixes currencies (costs are not
// converted), as Studio refuses it.
func TestBudgetSyncService_MixedCurrencyBlocks(t *testing.T) {
	db := setupBudgetSyncTestDB(t)
	require.NoError(t, db.AutoMigrate(&models.LLM{}))
	bus := eventbridge.NewBus()
	got := make(chan BudgetSyncPayload, 4)
	bus.Subscribe(BudgetSyncTopic, func(event eventbridge.Event) {
		var p BudgetSyncPayload
		require.NoError(t, json.Unmarshal(event.Payload, &p))
		got <- p
	})
	svc := NewBudgetSyncService(db, bus)
	svc.SetEdgeBudgetSource(&fakeEdgeBudget{blocks: map[uint]string{}})

	limit := func(v float64) *float64 { return &v }
	apps := []models.App{
		{Name: "mixed", MonthlyBudget: limit(100)},
		{Name: "eur", MonthlyBudget: limit(100)},
		{Name: "unbudgeted"},
	}
	for i := range apps {
		require.NoError(t, db.Create(&apps[i]).Error)
	}
	llms := []models.LLM{{ID: 1, Name: "mixed", MonthlyBudget: limit(100)}, {ID: 2, Name: "eur", MonthlyBudget: limit(100)}}
	for i := range llms {
		require.NoError(t, db.Create(&llms[i]).Error)
	}
	now := time.Now()
	for _, r := range []models.LLMChatRecord{
		{AppID: apps[0].ID, LLMID: 1, Cost: 30000, Currency: "", TimeStamp: now},
		{AppID: apps[0].ID, LLMID: 1, Cost: 580000, Currency: "EUR", TimeStamp: now},
		{AppID: apps[1].ID, LLMID: 2, Cost: 10000, Currency: "EUR", TimeStamp: now},
		{AppID: apps[2].ID, LLMID: 2, Cost: 10000, Currency: "eur", TimeStamp: now},
	} {
		require.NoError(t, db.Create(&r).Error)
	}

	svc.aggregateAndPublish()
	var p BudgetSyncPayload
	select {
	case p = <-got:
	case <-time.After(time.Second):
		t.Fatal("no budget sync published")
	}
	assert.Contains(t, p.Blocks[uint32(apps[0].ID)], "app monthly budget's period has spend in EUR, USD")
	assert.InDelta(t, 1, p.AppBudgets[uint32(apps[1].ID)].Usage, 1e-9, "EUR-only counts as it is")
	assert.NotContains(t, p.Blocks, uint32(apps[1].ID))
	assert.NotContains(t, p.Blocks, uint32(apps[2].ID), "an App without a budget is not refused")

	assert.Contains(t, p.LLMBlocks[1], "LLM monthly budget's period has spend in EUR, USD")
	assert.InDelta(t, 2, p.LLMBudgets[2].Usage, 1e-9, "EUR and eur are one currency")
	assert.NotContains(t, p.LLMBlocks, uint32(2))
}
