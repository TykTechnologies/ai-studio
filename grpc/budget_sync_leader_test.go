package grpc

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/eventbridge"
	"github.com/TykTechnologies/midsommar/v2/pkg/replicas"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
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

// budget.sync sequences are a counter in the database with the leader's
// clock as its floor (max(stored + 1, now)). They keep increasing when
// leadership moves to a replica whose clock is behind (the other site):
// the sequence used to be the leader's wall clock alone, and edges, which
// drop every sync that is not above the last one they applied, ignored the
// new leader until its clock passed the old leader's last value. They stay
// in the range of the clock, so edges and Studios of older versions mix
// with them, and a counter set back by a failover recovers at once.
func TestBudgetSyncService_SequenceAcrossLeaders_PostgresAndSQLite(t *testing.T) {
	forEachClusterDB(t, func(t *testing.T, db *gorm.DB) {
		lead := &fakeLeadership{}
		lead.leader.Store(true)
		replicas.SetBackend(lead)
		t.Cleanup(func() { replicas.SetBackend(nil) })

		owner := models.User{Email: "sequence@example.com", Name: "Sequence"}
		require.NoError(t, db.Create(&owner).Error)
		cred := models.Credential{KeyID: "sequence-key", Secret: "sequence-secret"}
		require.NoError(t, db.Create(&cred).Error)
		app := models.App{Name: "App 1", UserID: owner.ID, CredentialID: cred.ID}
		require.NoError(t, db.Create(&app).Error)
		now := time.Now()
		periodStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		require.NoError(t, db.Create(&models.LLMChatRecord{AppID: app.ID, LLMID: 1, Cost: 250000, TimeStamp: periodStart.Add(time.Hour)}).Error)

		// An edge as budget_sync_handler.go orders syncs: it applies one
		// only when its sequence is above the last it applied.
		type edge struct{ last, applied uint64 }
		connected := &edge{}
		// An edge from before the upgrade: its stored sequence is the wall
		// clock of a 2.2 leader at its last sync, half a minute ago.
		upgraded := &edge{last: uint64(now.Add(-30 * time.Second).UnixNano())}
		// The case that remains: the 2.2 leader's clock ran a minute ahead
		// of the upgraded hub's. That edge waits until the hub's clock
		// passes its stored value, once, as it did before this change.
		ahead := &edge{last: uint64(now.Add(time.Minute).UnixNano())}
		bus := eventbridge.NewBus()
		var sequences []uint64
		bus.Subscribe(BudgetSyncTopic, func(ev eventbridge.Event) {
			var p BudgetSyncPayload
			require.NoError(t, json.Unmarshal(ev.Payload, &p))
			sequences = append(sequences, p.SequenceNumber)
			for _, e := range []*edge{connected, upgraded, ahead} {
				if p.SequenceNumber > e.last {
					e.last = p.SequenceNumber
					e.applied++
				}
			}
		})

		a := NewBudgetSyncService(db, bus)
		a.aggregateAndPublish()
		a.aggregateAndPublish()
		require.Len(t, sequences, 2)
		assert.Equal(t, uint64(2), connected.applied)
		assert.Equal(t, uint64(2), upgraded.applied, "an edge holding a wall-clock sequence applies the upgraded hub's first sync")
		assert.GreaterOrEqual(t, sequences[0], uint64(now.UnixNano()), "the clock is the floor")
		assert.Less(t, sequences[1], uint64(time.Now().Add(time.Second).UnixNano()), "and the values stay in the clock's range")
		assert.Greater(t, sequences[1], sequences[0])

		// Leadership moves to B, whose clock is 60 s behind A's: the
		// stored value wins over B's clock.
		b := NewBudgetSyncService(db, bus)
		b.now = func() time.Time { return time.Now().Add(-time.Minute) }
		b.aggregateAndPublish()
		b.aggregateAndPublish()
		require.Len(t, sequences, 4)
		assert.Equal(t, sequences[1]+1, sequences[2], "B continues A's sequence")
		assert.Equal(t, sequences[2]+1, sequences[3])
		assert.Equal(t, uint64(4), connected.applied, "the edge applies the new leader's first sync")
		assert.Equal(t, uint64(4), upgraded.applied)

		// And back: A does not reuse a value B drew, and its clock is the
		// floor again.
		back := time.Now()
		a.aggregateAndPublish()
		require.Len(t, sequences, 5)
		assert.Greater(t, sequences[4], sequences[3])
		assert.GreaterOrEqual(t, sequences[4], uint64(back.UnixNano()))

		// The counter is set back by a few values (a failover that lost
		// the last commits): the next sequence is still above everything
		// edges have applied.
		require.NoError(t, db.Model(&models.ClusterCounter{}).Where("name = ?", budgetSyncSequenceName).
			Update("value", gorm.Expr("value - 5")).Error)
		a.aggregateAndPublish()
		require.Len(t, sequences, 6)
		assert.Greater(t, sequences[5], sequences[4])
		assert.Equal(t, uint64(6), connected.applied)

		// With a clock that is behind as well, the lost values are passed
		// one sync at a time instead: edges miss at most as many syncs as
		// values were lost.
		require.NoError(t, db.Model(&models.ClusterCounter{}).Where("name = ?", budgetSyncSequenceName).
			Update("value", gorm.Expr("value - 2")).Error)
		for i := 0; i < 3; i++ {
			b.aggregateAndPublish()
		}
		require.Len(t, sequences, 9)
		assert.Equal(t, uint64(7), connected.applied, "two syncs below the lost values, then accepted again")

		// A Studio from before this change takes over (a rollback, or the
		// not-yet-upgraded replica in a rolling upgrade) and sends its wall
		// clock, as it always did: with clocks that agree, edges accept it.
		old := BudgetSyncPayload{BlocksIncluded: true, ControlTimestamp: time.Now(), SequenceNumber: uint64(time.Now().UnixNano())}
		require.NoError(t, eventbridge.PublishDown(bus, "control", BudgetSyncTopic, old))
		require.Len(t, sequences, 10)
		assert.Equal(t, uint64(8), connected.applied, "an older Studio's wall-clock sequence is accepted after the new ones")

		assert.Zero(t, ahead.applied, "an edge whose 2.2 leader ran ahead waits for the clock to pass its stored value")
	})
}

// Without a sequence from the database nothing is sent, and the flag that
// tells edges spend was recalculated waits for the next cycle.
func TestBudgetSyncService_NoSequenceNoSync(t *testing.T) {
	db := setupBudgetSyncTestDB(t)
	bus := eventbridge.NewBus()
	app := models.App{Name: "App 1"}
	require.NoError(t, db.Create(&app).Error)
	now := time.Now()
	periodStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	require.NoError(t, db.Create(&models.LLMChatRecord{AppID: app.ID, LLMID: 1, Cost: 250000, TimeStamp: periodStart.Add(time.Hour)}).Error)
	var payloads []BudgetSyncPayload
	bus.Subscribe(BudgetSyncTopic, func(ev eventbridge.Event) {
		var p BudgetSyncPayload
		require.NoError(t, json.Unmarshal(ev.Payload, &p))
		payloads = append(payloads, p)
	})
	svc := NewBudgetSyncService(db, bus)

	require.NoError(t, db.Migrator().DropTable(&models.ClusterCounter{}))
	svc.recalculated.Store(true)
	svc.aggregateAndPublish()
	assert.Empty(t, payloads, "no sync without a sequence")

	require.NoError(t, db.AutoMigrate(&models.ClusterCounter{}))
	svc.aggregateAndPublish()
	require.Len(t, payloads, 1)
	assert.True(t, payloads[0].SpendRecalculated, "the recalculation is announced once a sync goes out")

	// A counter that was set back is no reason to stop: the clock is
	// the floor.
	require.NoError(t, db.Model(&models.ClusterCounter{}).Where("name = ?", budgetSyncSequenceName).Update("value", 41).Error)
	svc.aggregateAndPublish()
	require.Len(t, payloads, 2)
	assert.Greater(t, payloads[1].SequenceNumber, payloads[0].SequenceNumber)
}
