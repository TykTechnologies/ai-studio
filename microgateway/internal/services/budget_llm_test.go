//go:build enterprise
// +build enterprise

package services

import (
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/v2/pkg/eventbridge"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newLLMBudgetTestService is a budget service with its own LLM budget sets.
func newLLMBudgetTestService(t *testing.T) (*DatabaseBudgetService, *BudgetSyncHandler, *database.App) {
	db, repo := setupBudgetServiceTestDB(t)
	svc := NewDatabaseBudgetService(db, repo, nil).(*DatabaseBudgetService)
	svc.llmBlocks = &BudgetBlocks{blocks: map[uint]string{}}
	svc.llmSpend = NewLLMBudgetSpend()
	h := &BudgetSyncHandler{db: db, blocks: &BudgetBlocks{blocks: map[uint]string{}}, llmBlocks: svc.llmBlocks, llmSpend: svc.llmSpend}
	app := &database.App{Name: "a", IsActive: true}
	require.NoError(t, db.Create(app).Error)
	return svc, h, app
}

func llmBudgetSync(t *testing.T, h *BudgetSyncHandler, seq uint64, p BudgetSyncPayload) {
	p.SequenceNumber, p.BlocksIncluded = seq, true
	ev, err := eventbridge.NewEvent(BudgetSyncTopic, "control", eventbridge.DirDown, p)
	require.NoError(t, err)
	h.HandleBudgetSync(ev)
}

// ap6: an edge enforces an LLM's own monthly budget, as Studio does: the
// spend the control plane reports across Studio and every edge, and what
// this edge recorded since, against the LLM's budget.
func TestCheckBudgetStatus_LLMBudget(t *testing.T) {
	svc, h, app := newLLMBudgetTestService(t)
	over := &database.LLM{Name: "over", Slug: "over", Vendor: "openai", MonthlyBudget: 0.0005, IsActive: true}
	under := &database.LLM{Name: "under", Slug: "under", Vendor: "openai", MonthlyBudget: 1, IsActive: true}
	none := &database.LLM{Name: "none", Slug: "none", Vendor: "openai", IsActive: true}
	for _, l := range []*database.LLM{over, under, none} {
		require.NoError(t, svc.db.Create(l).Error)
	}
	now := time.Now()
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	period := func(usage float64) AppBudgetData {
		return AppBudgetData{Usage: usage, PeriodStart: start, PeriodEnd: start.AddDate(0, 1, 0).Add(-time.Second)}
	}
	llmBudgetSync(t, h, 1, BudgetSyncPayload{LLMBudgets: map[uint32]AppBudgetData{
		uint32(over.ID): period(0.010992), uint32(under.ID): period(0.0001), uint32(none.ID): period(5)}})

	spent, limit, err := svc.CheckBudgetStatus(app.ID, &over.ID, 0)
	require.Error(t, err, "an LLM over its budget is refused")
	assert.Contains(t, err.Error(), "budget exceeded: LLM monthly budget exceeded")
	assert.Contains(t, err.Error(), "current=0.01099, estimated=0.00, limit=0.0005", "sub-cent amounts are not 0.00")
	assert.InDelta(t, 0.010992, spent, 1e-9)
	assert.Equal(t, 0.0005, limit)

	_, _, err = svc.CheckBudgetStatus(app.ID, &under.ID, 0)
	assert.NoError(t, err, "an LLM under its budget passes")
	_, _, err = svc.CheckBudgetStatus(app.ID, &none.ID, 0)
	assert.NoError(t, err, "an LLM without a budget passes")

	// What this edge records counts before the next sync.
	require.NoError(t, svc.RecordUsage(app.ID, &under.ID, 10, 1.5*10000, 5, 5))
	_, _, err = svc.CheckBudgetStatus(app.ID, &under.ID, 0)
	assert.ErrorContains(t, err, "LLM monthly budget exceeded")

	// The control plane's verdict refuses too (a budget of 0 reaches the
	// edge this way), and the next sync releases it.
	llmBudgetSync(t, h, 2, BudgetSyncPayload{LLMBlocks: map[uint32]string{uint32(none.ID): "LLM monthly budget exceeded (budget is 0)"}})
	_, _, err = svc.CheckBudgetStatus(app.ID, &none.ID, 0)
	assert.EqualError(t, err, "budget exceeded: LLM monthly budget exceeded (budget is 0)")
	var persisted int64
	require.NoError(t, svc.db.Model(&database.LLMBudgetBlock{}).Count(&persisted).Error)
	assert.EqualValues(t, 1, persisted, "a restarted edge keeps refusing")
	llmBudgetSync(t, h, 3, BudgetSyncPayload{})
	_, _, err = svc.CheckBudgetStatus(app.ID, &none.ID, 0)
	assert.NoError(t, err)
}

// The App's budget is checked first and is unaffected by the LLM's.
func TestCheckBudgetStatus_LLMBudgetLeavesAppBudget(t *testing.T) {
	svc, h, _ := newLLMBudgetTestService(t)
	llm := &database.LLM{Name: "under", Slug: "under", Vendor: "openai", MonthlyBudget: 100, IsActive: true}
	require.NoError(t, svc.db.Create(llm).Error)
	app := &database.App{Name: "budgeted", IsActive: true, MonthlyBudget: 10}
	require.NoError(t, svc.db.Create(app).Error)
	llmBudgetSync(t, h, 1, BudgetSyncPayload{})

	usage, limit, err := svc.CheckBudgetStatus(app.ID, &llm.ID, 0)
	require.NoError(t, err)
	assert.Equal(t, 0.0, usage)
	assert.Equal(t, 10.0, limit, "the App's figures, not the LLM's")

	_, _, err = svc.CheckBudgetStatus(app.ID, &llm.ID, 11)
	assert.EqualError(t, err, "budget exceeded: current=0.00, estimated=11.00, limit=10.00")
}

func TestFormatBudgetAmount(t *testing.T) {
	for in, want := range map[float64]string{0: "0.00", 0.0005: "0.0005", 0.010992: "0.01099", 1: "1.00", 1234.5: "1234.50"} {
		assert.Equal(t, want, formatBudgetAmount(in), "%v", in)
	}
}
