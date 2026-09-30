package grpc

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/eventbridge"
	"github.com/TykTechnologies/midsommar/v2/pkg/replicas"
)

type fakeLeadership struct{ leader atomic.Bool }

func (f *fakeLeadership) IsLeader() bool                             { return f.leader.Load() }
func (f *fakeLeadership) Signal(ctx context.Context, n string) error { return nil }

// With several replicas only the leader sends budget.sync (the cluster
// relay carries it to every replica's edges; if each replica sent its own,
// every edge would get one per replica), but every replica keeps the spend
// figures its edge snapshots use.
func TestBudgetSyncService_OnlyTheLeaderPublishes(t *testing.T) {
	lead := &fakeLeadership{}
	replicas.SetBackend(lead)
	t.Cleanup(func() { replicas.SetBackend(nil) })

	db := setupBudgetSyncTestDB(t)
	bus := eventbridge.NewBus()
	app := models.App{Name: "App 1"}
	require.NoError(t, db.Create(&app).Error)
	now := time.Now()
	periodStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	require.NoError(t, db.Create(&models.LLMChatRecord{AppID: app.ID, LLMID: 1, Cost: 250000, TimeStamp: periodStart.Add(time.Hour)}).Error)

	var published atomic.Int32
	bus.Subscribe(BudgetSyncTopic, func(eventbridge.Event) { published.Add(1) })
	svc := NewBudgetSyncService(db, bus)

	svc.aggregateAndPublish()
	assert.Zero(t, published.Load(), "a follower does not publish")
	usage, ok := svc.PeriodUsage(app.ID, periodStart)
	require.True(t, ok, "a follower still has the spend for its snapshots")
	assert.InDelta(t, 25.0, usage, 0.0001)

	lead.leader.Store(true)
	svc.aggregateAndPublish()
	assert.Equal(t, int32(1), published.Load(), "the leader publishes")
}

// A replica that becomes the leader after its start-up sync (a restarted
// hub whose crashed predecessor held the lease a moment longer) syncs at
// once, not an interval later: edges must not keep serving an App whose
// budget is spent.
func TestBudgetSyncService_SyncsWhenItBecomesLeader(t *testing.T) {
	lead := &fakeLeadership{}
	replicas.SetBackend(lead)
	t.Cleanup(func() { replicas.SetBackend(nil) })

	db := setupBudgetSyncTestDB(t)
	bus := eventbridge.NewBus()
	app := models.App{Name: "App 1"}
	require.NoError(t, db.Create(&app).Error)
	now := time.Now()
	periodStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	require.NoError(t, db.Create(&models.LLMChatRecord{AppID: app.ID, LLMID: 1, Cost: 250000, TimeStamp: periodStart.Add(time.Hour)}).Error)
	var published atomic.Int32
	bus.Subscribe(BudgetSyncTopic, func(eventbridge.Event) { published.Add(1) })
	svc := NewBudgetSyncService(db, bus)
	svc.syncInterval = time.Hour
	svc.Start()
	t.Cleanup(svc.Stop)

	time.Sleep(200 * time.Millisecond)
	assert.Zero(t, published.Load(), "not the leader at start")

	lead.leader.Store(true)
	replicas.BecameLeader()
	require.Eventually(t, func() bool { return published.Load() == 1 }, 5*time.Second, 10*time.Millisecond, "synced on becoming the leader")
}
