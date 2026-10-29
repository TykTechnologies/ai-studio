package grpc

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/eventbridge"
	"github.com/TykTechnologies/midsommar/v2/pkg/replicas"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The usage tracker folds rows into settled totals once and never re-reads
// them, so a price recalculation that rewrote their cost went unseen. After
// the spend_recalculated signal the next cycle sums afresh, names every App
// (one whose spend fell to 0 too) and tells edges they may lower.
func TestBudgetSyncService_RecalculationResumsAndLetsEdgesLower(t *testing.T) {
	db := setupBudgetSyncTestDB(t)
	bus := eventbridge.NewBus()
	payloads := make(chan BudgetSyncPayload, 16)
	bus.Subscribe(BudgetSyncTopic, func(e eventbridge.Event) {
		var p BudgetSyncPayload
		if json.Unmarshal(e.Payload, &p) == nil {
			payloads <- p
		}
	})
	next := func() BudgetSyncPayload {
		t.Helper()
		select {
		case p := <-payloads:
			return p
		case <-time.After(2 * time.Second):
			t.Fatal("no budget sync published")
			return BudgetSyncPayload{}
		}
	}

	overpriced, unpriced := models.App{Name: "overpriced"}, models.App{Name: "unpriced"}
	require.NoError(t, db.Create(&overpriced).Error)
	require.NoError(t, db.Create(&unpriced).Error)
	at := time.Now().Add(-time.Minute)
	addSpend(t, db, overpriced.ID, 200000, at) // $20
	addSpend(t, db, unpriced.ID, 0, at)

	s := NewBudgetSyncService(db, bus)
	for i := 0; i <= budgetSettleCycles; i++ { // settle the rows
		s.aggregateAndPublish()
		next()
	}

	// A recalculation rewrites the costs in place.
	require.NoError(t, db.Model(&models.LLMChatRecord{}).Where("app_id = ?", overpriced.ID).Update("cost", 0).Error)
	require.NoError(t, db.Model(&models.LLMChatRecord{}).Where("app_id = ?", unpriced.ID).Update("cost", 20000).Error)

	s.aggregateAndPublish()
	p := next()
	assert.InDelta(t, 20, p.AppBudgets[uint32(overpriced.ID)].Usage, 1e-9, "without the signal the settled figure stands")
	assert.False(t, p.SpendRecalculated)

	s.recalculated.Store(true)
	s.aggregateAndPublish()
	p = next()
	assert.True(t, p.SpendRecalculated)
	require.Contains(t, p.AppBudgets, uint32(overpriced.ID), "an App whose spend fell to 0 is named")
	assert.InDelta(t, 0, p.AppBudgets[uint32(overpriced.ID)].Usage, 1e-9)
	assert.InDelta(t, 2, p.AppBudgets[uint32(unpriced.ID)].Usage, 1e-9)

	// Only the first sync after it carries the flag.
	s.aggregateAndPublish()
	p = next()
	assert.False(t, p.SpendRecalculated)
	assert.InDelta(t, 2, p.AppBudgets[uint32(unpriced.ID)].Usage, 1e-9)
}

// A running budget sync arms the recalculation on the replica signal, which
// the recalculating replica delivers to itself and the cluster log delivers
// to the others.
func TestBudgetSyncService_ArmsOnSpendRecalculatedSignal(t *testing.T) {
	db := setupBudgetSyncTestDB(t)
	bus := eventbridge.NewBus()
	synced := make(chan struct{}, 4)
	bus.Subscribe(BudgetSyncTopic, func(eventbridge.Event) { synced <- struct{}{} })
	app := models.App{Name: "a"}
	require.NoError(t, db.Create(&app).Error)
	addSpend(t, db, app.ID, 100, time.Now())

	s := NewBudgetSyncService(db, bus)
	s.syncInterval = time.Hour
	s.Start()
	defer s.Stop()
	select {
	case <-synced: // the start-up cycle is done
	case <-time.After(2 * time.Second):
		t.Fatal("no start-up sync")
	}

	replicas.Deliver(replicas.SignalSpendRecalculated)
	assert.True(t, s.recalculated.Load())

	s.Stop()
	s.recalculated.Store(false)
	replicas.Deliver(replicas.SignalSpendRecalculated)
	assert.False(t, s.recalculated.Load(), "Stop removes the handler")
}
