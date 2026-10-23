package grpc

import (
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/eventbridge"
	"github.com/TykTechnologies/midsommar/v2/services/budget"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeEdgeBudget struct {
	mu       sync.Mutex
	blocks   map[uint]string
	err      error
	analyzed [][]uint
}

func (f *fakeEdgeBudget) EdgeBlocks() (map[uint]string, error) { return f.blocks, f.err }

func (f *fakeEdgeBudget) AnalyzeApps(ids []uint) {
	f.mu.Lock()
	f.analyzed = append(f.analyzed, ids)
	f.mu.Unlock()
}

func TestBudgetSyncService_BudgetBlocks(t *testing.T) {
	db := setupBudgetSyncTestDB(t)
	bus := eventbridge.NewBus()

	var mu sync.Mutex
	var got []BudgetSyncPayload
	bus.Subscribe(BudgetSyncTopic, func(event eventbridge.Event) {
		var p BudgetSyncPayload
		require.NoError(t, json.Unmarshal(event.Payload, &p))
		mu.Lock()
		got = append(got, p)
		mu.Unlock()
	})
	count := func() int {
		mu.Lock()
		defer mu.Unlock()
		return len(got)
	}

	svc := NewBudgetSyncService(db, bus)
	src := &fakeEdgeBudget{blocks: map[uint]string{3: "app monthly budget is 0"}}
	svc.SetEdgeBudgetSource(src)

	// Published even with no App usage, so blocks reach edges.
	svc.aggregateAndPublish()
	require.Eventually(t, func() bool { return count() == 1 }, time.Second, 10*time.Millisecond)
	mu.Lock()
	assert.True(t, got[0].BlocksIncluded)
	assert.Equal(t, "app monthly budget is 0", got[0].Blocks[3])
	mu.Unlock()

	// An empty set is still sent, so edges release the App.
	src.blocks = map[uint]string{}
	svc.aggregateAndPublish()
	require.Eventually(t, func() bool { return count() == 2 }, time.Second, 10*time.Millisecond)
	mu.Lock()
	assert.True(t, got[1].BlocksIncluded)
	assert.Empty(t, got[1].Blocks)
	mu.Unlock()

	// A failure leaves the edges' last set in place: the sync, which still
	// carries the LLM budgets, says nothing about App blocks.
	src.err = errors.New("db down")
	svc.aggregateAndPublish()
	require.Eventually(t, func() bool { return count() == 3 }, time.Second, 10*time.Millisecond)
	mu.Lock()
	assert.False(t, got[2].BlocksIncluded)
	assert.True(t, got[2].LLMIncluded)
	mu.Unlock()
}

func TestBudgetSyncService_AnalyzesAppsWhoseEdgeSpendMoved(t *testing.T) {
	db := setupBudgetSyncTestDB(t)
	bus := eventbridge.NewBus()
	svc := NewBudgetSyncService(db, bus)
	src := &fakeEdgeBudget{blocks: map[uint]string{}}
	svc.SetEdgeBudgetSource(src)

	now := time.Now()
	for _, appID := range []uint{1, 2} {
		require.NoError(t, db.Create(&models.App{ID: appID, Name: "a"}).Error)
		require.NoError(t, db.Create(&models.LLMChatRecord{AppID: appID, Cost: 10000, TimeStamp: now}).Error)
	}

	svc.aggregateAndPublish()
	svc.aggregateAndPublish() // nothing moved
	require.NoError(t, db.Create(&models.LLMChatRecord{AppID: 2, Cost: 5000, TimeStamp: now}).Error)
	svc.aggregateAndPublish()

	src.mu.Lock()
	defer src.mu.Unlock()
	require.Len(t, src.analyzed, 2, "first sync and the sync after app 2 spent")
	assert.ElementsMatch(t, []uint{1, 2}, src.analyzed[0])
	assert.Equal(t, []uint{2}, src.analyzed[1])
}

// spendAnalyzingBudget also implements budget.SpendAnalyzer.
type spendAnalyzingBudget struct {
	fakeEdgeBudget
	spend []map[uint]budget.AppPeriodSpend
}

func (f *spendAnalyzingBudget) AnalyzeAppSpend(spend map[uint]budget.AppPeriodSpend) {
	f.mu.Lock()
	f.spend = append(f.spend, spend)
	f.mu.Unlock()
}

// A source that can use the sync's own figures gets them, instead of being
// asked to re-read every moved App's spend for its whole period.
func TestBudgetSyncService_HandsSpendToSpendAnalyzer(t *testing.T) {
	db := setupBudgetSyncTestDB(t)
	svc := NewBudgetSyncService(db, eventbridge.NewBus())
	src := &spendAnalyzingBudget{fakeEdgeBudget: fakeEdgeBudget{blocks: map[uint]string{}}}
	svc.SetEdgeBudgetSource(src)

	now := time.Now()
	require.NoError(t, db.Create(&models.App{ID: 1, Name: "a"}).Error)
	require.NoError(t, db.Create(&models.LLMChatRecord{AppID: 1, Cost: 25000, TimeStamp: now}).Error)
	svc.aggregateAndPublish()

	src.mu.Lock()
	defer src.mu.Unlock()
	assert.Empty(t, src.analyzed, "AnalyzeApps is not called when the spend is handed over")
	require.Len(t, src.spend, 1)
	got := src.spend[0][1]
	assert.InDelta(t, 2.5, got.Spent, 1e-9)
	wantStart, _ := calculateBudgetPeriod(nil, now)
	assert.True(t, got.PeriodStart.Equal(wantStart))
}

// ap6: the sync carries the spend of every LLM with a monthly budget in its
// own period, across every App, and the LLMs edges must refuse: a budget of
// 0, or one spent, as Studio's check refuses them.
func TestBudgetSyncService_LLMBudgets(t *testing.T) {
	db := setupBudgetSyncTestDB(t)
	bus := eventbridge.NewBus()
	got := make(chan BudgetSyncPayload, 4)
	bus.Subscribe(BudgetSyncTopic, func(event eventbridge.Event) {
		var p BudgetSyncPayload
		require.NoError(t, json.Unmarshal(event.Payload, &p))
		got <- p
	})
	svc := NewBudgetSyncService(db, bus)
	svc.SetEdgeBudgetSource(&fakeEdgeBudget{blocks: map[uint]string{}})

	budget := func(v float64) *float64 { return &v }
	now := time.Now()
	resetAt := now.Add(-time.Hour).Truncate(time.Second)
	llms := []models.LLM{
		{ID: 1, Name: "over", MonthlyBudget: budget(0.0005)},
		{ID: 2, Name: "under", MonthlyBudget: budget(5)},
		{ID: 3, Name: "zero", MonthlyBudget: budget(0)},
		{ID: 4, Name: "unbudgeted"},
		// Reset an hour ago: earlier spend is not in its period.
		{ID: 5, Name: "reset", MonthlyBudget: budget(1), BudgetStartDate: &resetAt},
	}
	for i := range llms {
		require.NoError(t, db.Create(&llms[i]).Error)
	}
	for _, r := range []models.LLMChatRecord{
		{AppID: 1, LLMID: 1, Cost: 50, TimeStamp: now}, // $0.005, across two Apps
		{AppID: 2, LLMID: 1, Cost: 60, TimeStamp: now},
		{AppID: 1, LLMID: 2, Cost: 10000, TimeStamp: now},
		{AppID: 1, LLMID: 4, Cost: 10000, TimeStamp: now},
		{AppID: 1, LLMID: 5, Cost: 90000, TimeStamp: now.Add(-2 * time.Hour)},
		{AppID: 1, LLMID: 5, Cost: 1000, TimeStamp: now},
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
	require.True(t, p.BlocksIncluded)
	require.True(t, p.LLMIncluded)
	assert.InDelta(t, 0.011, p.LLMBudgets[1].Usage, 1e-9)
	assert.InDelta(t, 1.0, p.LLMBudgets[2].Usage, 1e-9)
	assert.NotContains(t, p.LLMBudgets, uint32(4), "an LLM without a budget is not tracked")
	require.Contains(t, p.LLMBudgets, uint32(3), "an unspent LLM is sent too, with its period")
	assert.Zero(t, p.LLMBudgets[3].Usage)
	assert.False(t, p.LLMBudgets[3].PeriodStart.IsZero())
	assert.InDelta(t, 0.1, p.LLMBudgets[5].Usage, 1e-9)
	assert.True(t, p.LLMBudgets[5].PeriodStart.Equal(resetAt), "the LLM's own period")
	assert.Equal(t, map[uint32]string{
		1: "LLM monthly budget exceeded",
		3: "LLM monthly budget exceeded (budget is 0)",
	}, p.LLMBlocks)
}

// A failure to compute LLM budgets leaves the edges' LLM state alone
// (llm_included unset) but still sends the App blocks.
func TestBudgetSyncService_LLMBudgetFailureKeepsAppBlocks(t *testing.T) {
	db := setupBudgetSyncTestDB(t)
	bus := eventbridge.NewBus()
	got := make(chan BudgetSyncPayload, 1)
	bus.Subscribe(BudgetSyncTopic, func(event eventbridge.Event) {
		var p BudgetSyncPayload
		require.NoError(t, json.Unmarshal(event.Payload, &p))
		got <- p
	})
	svc := NewBudgetSyncService(db, bus)
	svc.SetEdgeBudgetSource(&fakeEdgeBudget{blocks: map[uint]string{3: "app monthly budget is 0"}})
	require.NoError(t, db.Migrator().DropTable(&models.LLM{}))

	svc.aggregateAndPublish()
	select {
	case p := <-got:
		assert.True(t, p.BlocksIncluded)
		assert.Equal(t, "app monthly budget is 0", p.Blocks[3])
		assert.False(t, p.LLMIncluded)
	case <-time.After(time.Second):
		t.Fatal("no budget sync published")
	}
}
