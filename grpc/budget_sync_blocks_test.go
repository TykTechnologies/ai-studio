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

	// A failure leaves the edges' last set in place (and, with no usage,
	// nothing is published at all).
	src.err = errors.New("db down")
	svc.aggregateAndPublish()
	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, 2, count())
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
