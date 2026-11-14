package studio

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/cluster"
	"github.com/TykTechnologies/midsommar/v2/pkg/testinfra/containers"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/TykTechnologies/midsommar/v2/services/pushes"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// What the cluster's own machinery (the event log, the leader lease, edge
// pushes) does across a database failover, on a real pair: with the newest
// transactions lost (asynchronous replication) and with a new primary whose
// clock differs from the old one's by 30 s.

// With asynchronous replication a failover can lose the newest events, and
// the new primary hands their ids out again. An event published after the
// failover must reach the other replica although its id is below what that
// replica had read; the replica says that it moved its cursor back.
func TestFailover_EventPublishedAfterLostEventsReachesTheOtherReplica(t *testing.T) {
	pair := containers.StartPostgresPair(t, nil)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	a, b := openFailoverDB(t, pair), openFailoverDB(t, pair)
	require.NoError(t, a.AutoMigrate(&models.ClusterEvent{}))

	opts := cluster.LogOptions{PollInterval: time.Second}
	logA, logB := cluster.NewLog(a, "replica-a", opts), cluster.NewLog(b, "replica-b", opts)
	require.NoError(t, logA.Start(ctx))
	require.NoError(t, logB.Start(ctx))
	t.Cleanup(func() { logA.Stop(); logB.Stop() })
	var mu sync.Mutex
	var got []string
	logB.Subscribe("failover.test", func(ev cluster.Event) {
		mu.Lock()
		got = append(got, string(ev.Payload))
		mu.Unlock()
	})
	received := func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), got...)
	}

	require.NoError(t, logA.Publish(ctx, "failover.test", []byte("kept")))
	require.Eventually(t, func() bool { return len(received()) == 1 }, 15*time.Second, 50*time.Millisecond)
	require.Eventually(t, func() bool {
		n, err := pair.Standby().SQL(ctx, "SELECT count(*) FROM cluster_events")
		return err == nil && n == "1"
	}, 15*time.Second, 100*time.Millisecond, "the standby has the first event")

	// The standby stops receiving; 40 more events are published and read
	// (more than a sequence logs ahead, so their ids are handed out again).
	require.NoError(t, pair.DetachStandby(ctx))
	for i := 0; i < 40; i++ {
		require.NoError(t, logA.Publish(ctx, "failover.test", []byte(fmt.Sprintf("lost-%d", i))))
	}
	require.Eventually(t, func() bool { return len(received()) == 41 }, 15*time.Second, 50*time.Millisecond)
	cursor := logB.Stats().Cursor

	require.NoError(t, pair.Failover(ctx))

	require.Eventually(t, func() bool {
		return logA.Publish(ctx, "failover.test", []byte("after")) == nil
	}, 30*time.Second, 100*time.Millisecond, "replica A publishes on the new primary")
	require.Eventually(t, func() bool {
		all := received()
		return len(all) > 41 && all[len(all)-1] == "after"
	}, 30*time.Second, 100*time.Millisecond, "the event published after the failover reaches replica B; it has %v", received()[40:])

	stats := logB.Stats()
	assert.Less(t, stats.Cursor, cursor, "the new event's id is below what B had read")
	assert.EqualValues(t, 1, stats.CursorResets, "B says that it moved its cursor back")
	for _, p := range received()[:41] {
		assert.NotEqual(t, "after", p)
	}
}

// leaseFailover runs two replicas' leader leases over a failover to a
// standby whose clock differs by offset, sampling who believes it leads.
type leaseFailover struct {
	pair *containers.PostgresPair
	a, b *cluster.Leadership
	dbA  *gorm.DB
}

func startLeaseFailover(t *testing.T, offset time.Duration) *leaseFailover {
	t.Helper()
	ctx := context.Background()
	pair := containers.StartPostgresPair(t, &containers.PostgresPairConfig{Synchronous: true})
	require.NoError(t, pair.EnableClockOffset(ctx))
	require.NoError(t, pair.Standby().SetClockOffset(ctx, offset))
	dbA, dbB := openFailoverDB(t, pair), openFailoverDB(t, pair)
	require.NoError(t, dbA.AutoMigrate(&models.ClusterLease{}, &models.ClusterNode{}))
	f := &leaseFailover{pair: pair, dbA: dbA}
	// The defaults: a 30 s lease renewed every 10 s.
	f.a = cluster.NewLeadership(dbA, cluster.LeaderLease, "replica-a", cluster.LeadershipOptions{})
	f.a.Start()
	t.Cleanup(f.a.Stop)
	require.True(t, f.a.IsLeader())
	f.b = cluster.NewLeadership(dbB, cluster.LeaderLease, "replica-b", cluster.LeadershipOptions{})
	f.b.Start()
	t.Cleanup(f.b.Stop)
	require.False(t, f.b.IsLeader())
	return f
}

// watch samples both replicas for d and reports whether both ever believed
// they led at once, and when (from its start) b first led alone (-1: never).
func (f *leaseFailover) watch(d time.Duration) (both bool, bLeads time.Duration) {
	bLeads = -1
	start := time.Now()
	for time.Since(start) < d {
		a, b := f.a.IsLeader(), f.b.IsLeader()
		if a && b {
			both = true
		}
		if b && !a && bLeads < 0 {
			bLeads = time.Since(start)
		}
		time.Sleep(5 * time.Millisecond)
	}
	return both, bLeads
}

// The leader loses the database (it is alive, and partitioned from it) at
// the moment the database fails over to a server whose clock is 30 s ahead
// or behind, and the other replica can reach the new primary. The other
// replica takes the lease when it has watched the leader's last renewal go
// unrenewed for as long as the leader believes in it, by its own clock:
//
//   - never while the leader still believes it leads, although with the
//     clock ahead the lease looks expired the moment the new primary answers;
//   - at most one TTL and the clock-rate margin (30.5 s with the defaults)
//     after the leader's last renewal, plus the time the database was away,
//     although with the clock behind the lease looks valid for 60 s.
func TestFailover_PartitionedLeaderAndAClockOffsetNeverGiveTwoLeaders(t *testing.T) {
	for _, offset := range []time.Duration{30 * time.Second, -30 * time.Second} {
		t.Run(fmt.Sprintf("new primary's clock %+.0fs", offset.Seconds()), func(t *testing.T) {
			f := startLeaseFailover(t, offset)
			ctx := context.Background()

			// The partition: replica A's pool is gone for good.
			pool, err := f.dbA.DB()
			require.NoError(t, err)
			require.NoError(t, pool.Close())
			require.True(t, f.a.IsLeader(), "A still believes: its last renewal is recent")

			type result struct {
				both bool
				took time.Duration
			}
			watched := make(chan result, 1)
			go func() {
				both, took := f.watch(50 * time.Second)
				watched <- result{both, took}
			}()
			down := time.Now()
			require.NoError(t, f.pair.Failover(ctx))
			away := time.Since(down)

			r := <-watched
			assert.False(t, r.both, "two replicas believed they led at once")
			require.GreaterOrEqual(t, r.took, time.Duration(0), "replica B never took the lease")
			bound := cluster.DefaultLeaseTTL + 500*time.Millisecond + away + 2*time.Second
			assert.LessOrEqual(t, r.took, bound,
				"B took the lease %s after A was cut off; the bound is one TTL (30 s) and the margin (0.5 s) after A's last renewal, plus the %s the database was away and 2 s for this test's sampling",
				r.took.Round(100*time.Millisecond), away.Round(100*time.Millisecond))
			assert.False(t, f.a.IsLeader())
			assert.True(t, f.b.IsLeader())
		})
	}
}

// Both replicas keep the database through the failover, to a server whose
// clock is 30 s ahead: the leader goes on leading (it renews on the new
// primary), nobody else takes the lease although it looked expired there,
// and 30 s after the failover exactly one replica leads.
func TestFailover_LeaderKeepsTheLeaseAcrossAClockOffset(t *testing.T) {
	f := startLeaseFailover(t, 30*time.Second)
	watched := make(chan bool, 1)
	go func() {
		both, _ := f.watch(35 * time.Second)
		watched <- both
	}()
	require.NoError(t, f.pair.Failover(context.Background()))
	assert.False(t, <-watched, "two replicas believed they led at once")
	leaders := 0
	for _, l := range []*cluster.Leadership{f.a, f.b} {
		if l.IsLeader() {
			leaders++
		}
	}
	assert.Equal(t, 1, leaders, "exactly one replica leads 30 s after the failover")
}

// pushStreams is one replica's edge streams, for the push coordinator.
type pushStreams struct {
	mu      sync.Mutex
	streams map[string]string
	sent    int
}

func (s *pushStreams) LocalStreams() map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string]string{}
	for k, v := range s.streams {
		out[k] = v
	}
	return out
}

func (s *pushStreams) SendReload(edgeID, session string, _ *pb.ConfigurationReloadRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.streams[edgeID] != session {
		return pushes.ErrNoStream
	}
	s.sent++
	return nil
}

// pushAcrossFailover sends a push to its edge and, before the edge answers,
// fails the database over to a server whose clock differs by offset, with
// two replicas' coordinators (dispatcher and janitor) running. With restart,
// the replica that issued the push is stopped before the failover and a new
// one started right after it: a replica with no reading of the old clock.
// It reports how far the push's deadline was moved, after checking that
// through the time in which the sender looks dead and its command silent by
// the new clock nothing was requeued or retried, and that the edge's answer
// completes the push on its first attempt.
func pushAcrossFailover(t *testing.T, offset time.Duration, restart bool) (moved time.Duration) {
	t.Helper()
	ctx := context.Background()
	pair := containers.StartPostgresPair(t, &containers.PostgresPairConfig{Synchronous: true})
	require.NoError(t, pair.EnableClockOffset(ctx))
	require.NoError(t, pair.Standby().SetClockOffset(ctx, offset))
	dbA, dbB := openFailoverDB(t, pair), openFailoverDB(t, pair)
	// Studio's whole schema: in the Enterprise build a push also reads
	// tables of other domains (licensed_namespaces, through the
	// entitlements service), so a hand-picked list of tables falls short.
	require.NoError(t, models.InitModels(dbA))

	sender := &pushStreams{streams: map[string]string{"edge-1": "session-1"}}
	start := func(db *gorm.DB, id string, streams *pushStreams) (*pushes.Coordinator, func()) {
		node, err := cluster.StartNode(db, id, "test")
		require.NoError(t, err)
		c := pushes.New(db, id, streams, pushes.Options{})
		require.NoError(t, c.Start(ctx))
		var once sync.Once
		stop := func() { once.Do(func() { c.Stop(); node.Stop(context.Background()) }) }
		t.Cleanup(stop)
		return c, stop
	}
	issuer, stopIssuer := start(dbA, "node-a", &pushStreams{streams: map[string]string{}})
	holder, _ := start(dbB, "node-b", sender)
	now := time.Now()
	require.NoError(t, dbA.Create(&models.EdgeInstance{EdgeID: "edge-1", Namespace: "default", Status: models.EdgeStatusConnected, OwnerNodeID: "node-b", LastHeartbeat: &now}).Error)

	res, err := issuer.Push(ctx, pushes.Request{Scope: pushes.ScopeEdge, EdgeIDs: []string{"edge-1"}, InitiatedBy: "admin@example.com"})
	require.NoError(t, err)
	op := res.Operation.OperationID
	command := func() (cmd models.EdgePushCommand, err error) {
		err = dbB.Where("operation_id = ? AND edge_id = ?", op, "edge-1").First(&cmd).Error
		return cmd, err
	}
	require.Eventually(t, func() bool {
		cmd, err := command()
		return err == nil && cmd.Status == models.PushCommandSent
	}, 15*time.Second, 50*time.Millisecond, "node-b sends the push to its edge")
	before, err := command()
	require.NoError(t, err)
	// Each janitor has read the old primary's clock at least once.
	time.Sleep(5 * time.Second)

	if restart {
		stopIssuer()
	}
	require.NoError(t, pair.Failover(ctx))
	if restart {
		// A new process: a pool of its own, opened after the failover.
		start(openFailoverDB(t, pair), "node-a-restarted", &pushStreams{streams: map[string]string{}})
	}

	// A replica that measured the step has decided about the open pushes.
	require.Eventually(t, func() bool {
		var decided int64
		err := dbB.Model(&models.ClusterCounter{}).Where("name LIKE ? AND value = 2", "push-clock:%").Count(&decided).Error
		return err == nil && decided == 1
	}, 40*time.Second, 200*time.Millisecond, "the failover is marked as decided")
	after, err := command()
	require.NoError(t, err)
	moved = after.DeadlineAt.Sub(before.DeadlineAt)

	deadline := time.Now().Add(12 * time.Second)
	for time.Now().Before(deadline) {
		if cmd, err := command(); err == nil {
			require.Equal(t, models.PushCommandSent, cmd.Status, "the push was taken from its sender: %s", cmd.Message)
			require.Equal(t, before.Attempts, cmd.Attempts)
			require.InDelta(t, moved.Seconds(), cmd.DeadlineAt.Sub(before.DeadlineAt).Seconds(), 0.001, "moved a second time")
		}
		time.Sleep(250 * time.Millisecond)
	}

	holder.HandleReloadResponse(&pb.ConfigurationReloadResponse{OperationId: op, EdgeId: "edge-1", Phase: pb.ReloadPhase_READY, Success: true})
	require.Eventually(t, func() bool {
		cmd, err := command()
		return err == nil && cmd.Status == models.PushCommandSucceeded
	}, 15*time.Second, 100*time.Millisecond)
	done, err := command()
	require.NoError(t, err)
	assert.Equal(t, 1, done.Attempts, "its history shows one attempt")
	sender.mu.Lock()
	assert.Equal(t, 1, sender.sent, "sent once")
	sender.mu.Unlock()
	assert.False(t, strings.Contains(done.Message, "stopped"), done.Message)
	require.Eventually(t, func() bool {
		var o models.PushOperation
		return dbB.Where("operation_id = ?", op).First(&o).Error == nil && o.Status != models.PushOperationInProgress
	}, 15*time.Second, 200*time.Millisecond, "the operation is settled")
	return moved
}

// A push in flight when the database fails over to a server whose clock is
// 30 s ahead. By that clock the push is 30 s nearer its deadline and its
// answer timeout, and the replica that sent it looks dead until it
// refreshes its registration. The push is not requeued, retried or expired
// for any of that: its times are moved forward by the step, once although
// two janitors run, so its deadline is honoured to within the error of the
// measurement (two readings of the database's clock, a round trip each).
func TestFailover_PushInFlightCompletesWithTheClockAhead(t *testing.T) {
	moved := pushAcrossFailover(t, 30*time.Second, false)
	assert.InDelta(t, 30, moved.Seconds(), 0.5, "the deadline is moved by the step")
}

// The same with the new primary's clock 30 s behind: nothing is moved (the
// push only has 30 s longer before it would time out, which loses nothing)
// and it completes the same way.
func TestFailover_PushInFlightCompletesWithTheClockBehind(t *testing.T) {
	moved := pushAcrossFailover(t, -30*time.Second, false)
	assert.Zero(t, moved, "a step backwards moves nothing")
}

// The replica that issued the push is restarted during the failover: the
// new process has no reading of the old clock and decides nothing, for its
// first liveness window it leaves the sender's command alone although the
// sender looks dead, and the replica that kept running moves the push.
func TestFailover_PushInFlightCompletesWithAReplicaRestartedDuringIt(t *testing.T) {
	moved := pushAcrossFailover(t, 30*time.Second, true)
	assert.InDelta(t, 30, moved.Seconds(), 0.5, "the deadline is moved by the step, by the replica that measured it")
}
