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
	p.SequenceNumber, p.BlocksIncluded, p.LLMIncluded = seq, true, true
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
	assert.Contains(t, err.Error(), "current=0.010992, estimated=0.00, limit=0.0005", "sub-cent amounts are not 0.00")
	assert.InDelta(t, 0.010992, spent, 1e-9)
	assert.Equal(t, 0.0005, limit)

	_, _, err = svc.CheckBudgetStatus(app.ID, &under.ID, 0)
	assert.NoError(t, err, "an LLM under its budget passes")
	_, _, err = svc.CheckBudgetStatus(app.ID, &none.ID, 0)
	assert.NoError(t, err, "a local budget of 0 is no limit, whatever the spend: only llm_blocks refuses a budget of 0")

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
	for in, want := range map[float64]string{0: "0.00", 0.0005: "0.0005", 0.00005: "0.00005", 0.010992: "0.010992",
		0.99999: "0.99999", 0.5: "0.50", 1: "1.00", 1234.5: "1234.50"} {
		assert.Equal(t, want, formatBudgetAmount(in), "%v", in)
	}
}

// A control plane that sends no LLM budgets (before 2.3) leaves the edge's
// LLM blocks and spend as they were: its App blocks replace the App set, but
// its sync does not mean "no LLM is blocked".
func TestBudgetSync_OldControlLeavesLLMBudgets(t *testing.T) {
	svc, h, app := newLLMBudgetTestService(t)
	llm := &database.LLM{Name: "l", Slug: "l", Vendor: "openai", MonthlyBudget: 1, IsActive: true}
	require.NoError(t, svc.db.Create(llm).Error)
	now := time.Now()
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	llmBudgetSync(t, h, 1, BudgetSyncPayload{
		LLMBudgets: map[uint32]AppBudgetData{uint32(llm.ID): {Usage: 2, PeriodStart: start, PeriodEnd: start.AddDate(0, 1, 0).Add(-time.Second)}},
		LLMBlocks:  map[uint32]string{999: "LLM monthly budget exceeded (budget is 0)"},
	})

	ev, err := eventbridge.NewEvent(BudgetSyncTopic, "control", eventbridge.DirDown,
		BudgetSyncPayload{SequenceNumber: 2, BlocksIncluded: true})
	require.NoError(t, err)
	h.HandleBudgetSync(ev)

	_, _, err = svc.CheckBudgetStatus(app.ID, &llm.ID, 0)
	assert.ErrorContains(t, err, "LLM monthly budget exceeded", "the synced spend is kept")
	id := uint(999)
	_, _, err = svc.CheckBudgetStatus(app.ID, &id, 0)
	assert.ErrorContains(t, err, "budget is 0", "the LLM blocks are kept")
}

// A sync from a control plane before 2.3 (blocks_included, no LLM fields)
// leaves an edge that has learned no LLM budgets sane: it refuses no LLM on
// the control plane's say, its own spend still enforces each LLM's budget in
// the calendar month, and the App blocks apply as before.
func TestBudgetSync_OldControlOnFreshEdge(t *testing.T) {
	svc, h, app := newLLMBudgetTestService(t)
	svc.budgetBlocks = h.blocks
	blockedApp := &database.App{Name: "blocked", IsActive: true}
	require.NoError(t, svc.db.Create(blockedApp).Error)
	spent := &database.LLM{Name: "spent", Slug: "spent", Vendor: "openai", MonthlyBudget: 1, IsActive: true}
	fresh := &database.LLM{Name: "fresh", Slug: "fresh", Vendor: "openai", MonthlyBudget: 1, IsActive: true}
	free := &database.LLM{Name: "free", Slug: "free", Vendor: "openai", IsActive: true} // monthly_budget 0
	for _, l := range []*database.LLM{spent, fresh, free} {
		require.NoError(t, svc.db.Create(l).Error)
	}

	ev, err := eventbridge.NewEvent(BudgetSyncTopic, "control", eventbridge.DirDown, BudgetSyncPayload{
		SequenceNumber: 1, BlocksIncluded: true, Blocks: map[uint32]string{uint32(blockedApp.ID): "app monthly budget is 0"}})
	require.NoError(t, err)
	h.HandleBudgetSync(ev)

	_, _, err = svc.CheckBudgetStatus(blockedApp.ID, &fresh.ID, 0)
	assert.EqualError(t, err, "budget exceeded: app monthly budget is 0", "App blocks apply as before")

	for _, l := range []*database.LLM{spent, fresh, free} {
		_, _, err = svc.CheckBudgetStatus(app.ID, &l.ID, 0)
		assert.NoError(t, err, "no LLM is refused on an old control plane's say: %s", l.Name)
	}

	require.NoError(t, svc.RecordUsage(app.ID, &spent.ID, 10, 1.5*10000, 5, 5))
	require.NoError(t, svc.RecordUsage(app.ID, &free.ID, 10, 50*10000, 5, 5))
	_, _, err = svc.CheckBudgetStatus(app.ID, &spent.ID, 0)
	assert.ErrorContains(t, err, "LLM monthly budget exceeded", "this edge's own spend enforces the budget")
	_, _, err = svc.CheckBudgetStatus(app.ID, &fresh.ID, 0)
	assert.NoError(t, err)
	_, _, err = svc.CheckBudgetStatus(app.ID, &free.ID, 0)
	assert.NoError(t, err, "monthly_budget 0 with no llm_blocks entry is no limit")
}

// The LLM is spent once its spend reaches the budget, as on Studio.
func TestCheckBudgetStatus_LLMSpendAtBudget(t *testing.T) {
	svc, h, app := newLLMBudgetTestService(t)
	llm := &database.LLM{Name: "l", Slug: "l", Vendor: "openai", MonthlyBudget: 1, IsActive: true}
	require.NoError(t, svc.db.Create(llm).Error)
	llmBudgetSync(t, h, 1, BudgetSyncPayload{})
	require.NoError(t, svc.RecordUsage(app.ID, &llm.ID, 10, 1*10000, 5, 5))
	_, _, err := svc.CheckBudgetStatus(app.ID, &llm.ID, 0)
	assert.ErrorContains(t, err, "LLM monthly budget exceeded: current=1.00, estimated=0.00, limit=1.00")
}

// A budget reset in Studio with no spend since: the control plane sends the
// LLM's new period with $0, and the edge serves it again, its own spend from
// before the reset forgotten.
func TestCheckBudgetStatus_LLMBudgetResetWithoutSpend(t *testing.T) {
	svc, h, app := newLLMBudgetTestService(t)
	llm := &database.LLM{Name: "l", Slug: "l", Vendor: "openai", MonthlyBudget: 1, IsActive: true}
	require.NoError(t, svc.db.Create(llm).Error)
	now := time.Now()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	end := monthStart.AddDate(0, 1, 0).Add(-time.Second)
	llmBudgetSync(t, h, 1, BudgetSyncPayload{LLMBudgets: map[uint32]AppBudgetData{
		uint32(llm.ID): {Usage: 0.4, PeriodStart: monthStart, PeriodEnd: end}}})
	require.NoError(t, svc.RecordUsage(app.ID, &llm.ID, 10, 2*10000, 5, 5))
	_, _, err := svc.CheckBudgetStatus(app.ID, &llm.ID, 0)
	require.ErrorContains(t, err, "LLM monthly budget exceeded")

	resetAt := now.Add(-time.Minute).Truncate(time.Second)
	llmBudgetSync(t, h, 2, BudgetSyncPayload{LLMBudgets: map[uint32]AppBudgetData{
		uint32(llm.ID): {Usage: 0, PeriodStart: resetAt, PeriodEnd: end}}})
	_, _, err = svc.CheckBudgetStatus(app.ID, &llm.ID, 0)
	assert.NoError(t, err, "the reset LLM is served again")
}

// When an LLM's period ends, its local spend starts again.
func TestLLMBudgetSpend_NewPeriodClearsLocalSpend(t *testing.T) {
	s := NewLLMBudgetSpend()
	now := time.Date(2026, 1, 31, 23, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	s.Record(7, 3)
	s.Sync(map[uint32]AppBudgetData{7: {Usage: 1, PeriodStart: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		PeriodEnd: time.Date(2026, 1, 31, 23, 59, 59, 0, time.UTC)}})
	assert.Equal(t, 3.0, s.Spent(7), "the larger of the synced and the local figure")

	now = time.Date(2026, 2, 1, 0, 0, 1, 0, time.UTC)
	assert.Zero(t, s.Spent(7), "a new period starts from nothing")
	s.Record(7, 0.25)
	assert.Equal(t, 0.25, s.Spent(7))
}

// The LLM blocks a sync persisted are what a restarted edge loads.
func TestLLMBudgetBlocks_LoadedAfterRestart(t *testing.T) {
	svc, h, _ := newLLMBudgetTestService(t)
	llmBudgetSync(t, h, 1, BudgetSyncPayload{LLMBlocks: map[uint32]string{9: "LLM monthly budget exceeded"}})

	restarted := &BudgetBlocks{blocks: map[uint]string{}}
	loadLLMBudgetBlocks(svc.db, restarted)
	reason, blocked := restarted.Reason(9)
	assert.True(t, blocked)
	assert.Equal(t, "LLM monthly budget exceeded", reason)
}
