package services

import (
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Budget syncs only ever raise the edge's figures, so an App or LLM blocked
// at an overpriced rate stayed blocked after the control plane corrected the
// price and recalculated. The first sync after a recalculation
// (SpendRecalculated) is taken as it is, even when lower; later syncs raise
// again.
func TestBudgetSyncHandler_SpendRecalculatedLowers(t *testing.T) {
	db := setupBudgetSyncHandlerTestDB(t)
	handler := NewBudgetSyncHandler(db)
	handler.ledger = NewBudgetLedger(db)
	handler.llmSpend = NewLLMBudgetSpend()

	now := time.Now()
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	end := start.AddDate(0, 1, 0).Add(-time.Second)
	require.NoError(t, db.Create(&database.BudgetUsage{AppID: 1, PeriodStart: start, PeriodEnd: end, TotalCost: 200000, CreatedAt: now, UpdatedAt: now}).Error)
	spent, err := handler.ledger.Spent(1, start, end)
	require.NoError(t, err)
	require.InDelta(t, 200000, spent, 1e-9)
	handler.llmSpend.Record(7, 20) // recorded here at the old price

	stored := func() float64 {
		var u database.BudgetUsage
		require.NoError(t, db.Where("app_id = ? AND period_start = ?", 1, start).First(&u).Error)
		return u.TotalCost
	}
	sync := func(seq uint64, app, llm float64, recalculated bool) {
		limit := 10.0
		handler.HandleBudgetSync(createBudgetSyncEvent(BudgetSyncPayload{
			AppBudgets:        map[uint32]AppBudgetData{1: {Usage: app, PeriodStart: start, PeriodEnd: end}},
			LLMBudgets:        map[uint32]AppBudgetData{7: {Usage: llm, PeriodStart: start, PeriodEnd: end, Limit: &limit}},
			LLMIncluded:       true,
			ControlTimestamp:  now,
			SequenceNumber:    seq,
			SpendRecalculated: recalculated,
		}))
	}

	sync(1, 0.2, 0.2, false)
	assert.InDelta(t, 200000, stored(), 1e-9, "an ordinary sync does not lower")
	assert.InDelta(t, 20, handler.llmSpend.Spent(7), 1e-9)

	sync(2, 0.2, 0.2, true)
	assert.InDelta(t, 2000, stored(), 1e-9)
	spent, err = handler.ledger.Spent(1, start, end)
	require.NoError(t, err)
	assert.InDelta(t, 2000, spent, 1e-9)
	assert.InDelta(t, 0.2, handler.llmSpend.Spent(7), 1e-9)

	sync(3, 0.1, 0.1, false)
	assert.InDelta(t, 2000, stored(), 1e-9, "later syncs only raise again")
	sync(4, 0.5, 0.5, false)
	assert.InDelta(t, 5000, stored(), 1e-9)
	assert.InDelta(t, 0.5, handler.llmSpend.Spent(7), 1e-9)

	// An App that had no row and still spent nothing gets none.
	sync(5, 0, 0, true)
	var n int64
	require.NoError(t, db.Model(&database.BudgetUsage{}).Where("app_id = ?", 2).Count(&n).Error)
	assert.Zero(t, n)
	assert.InDelta(t, 0, stored(), 1e-9)
}
