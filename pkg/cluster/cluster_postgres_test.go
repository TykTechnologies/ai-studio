package cluster

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/postgres"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm/logger"
)

// testCluster is a schema of its own in DATABASE_URL, shared by the
// replicas a test opens on it.
type testCluster struct {
	t      *testing.T
	base   string
	schema string
	admin  *gorm.DB
}

func newTestCluster(t *testing.T) *testCluster {
	t.Helper()
	base := os.Getenv("DATABASE_URL")
	if base == "" {
		t.Skip("DATABASE_URL not set")
	}
	c := &testCluster{t: t, base: base, schema: fmt.Sprintf("cluster_test_%d", time.Now().UnixNano())}
	var err error
	c.admin, err = gorm.Open(postgres.Open(base), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, c.admin.Exec(`CREATE SCHEMA "`+c.schema+`"`).Error)
	t.Cleanup(func() {
		c.admin.Exec(`DROP SCHEMA "` + c.schema + `" CASCADE`)
		if sqlDB, err := c.admin.DB(); err == nil {
			sqlDB.Close()
		}
	})
	db := c.replicaDB("setup")
	require.NoError(t, db.AutoMigrate(&models.ClusterNode{}, &models.ClusterEvent{}))
	return c
}

// dsn is the schema's DSN for one replica: its own application_name, so it
// gets its own pool and its own listener, and the test can kill its
// connections alone.
func (c *testCluster) dsn(replica string) string {
	u, err := url.Parse(c.base)
	require.NoError(c.t, err)
	q := u.Query()
	q.Set("search_path", c.schema)
	q.Set("application_name", c.appName(replica))
	u.RawQuery = q.Encode()
	return u.String()
}

func (c *testCluster) appName(replica string) string { return c.schema + "_" + replica }

func (c *testCluster) replicaDB(replica string) *gorm.DB {
	db, err := gorm.Open(postgres.Open(c.dsn(replica)), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(c.t, err)
	c.t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})
	return db
}

// killConnections terminates every backend of a replica: its pool and its
// listener.
func (c *testCluster) killConnections(replica string) int {
	var n int
	require.NoError(c.t, c.admin.Raw(`SELECT count(pg_terminate_backend(pid)) FROM pg_stat_activity WHERE application_name = ?`, c.appName(replica)).Scan(&n).Error)
	return n
}

// replica is one simulated Studio replica's event log plus what it received.
type replica struct {
	name string
	db   *gorm.DB
	log  *Log

	mu     sync.Mutex
	got    []Event
	counts map[int64]int
}

func (c *testCluster) startReplica(name string, opts LogOptions) *replica {
	c.t.Helper()
	r := &replica{name: name, db: c.replicaDB(name), counts: map[int64]int{}}
	opts.ListenerDSN = c.dsn(name)
	r.log = NewLog(r.db, name, opts)
	r.log.Subscribe("*", func(ev Event) {
		r.mu.Lock()
		r.got = append(r.got, ev)
		r.counts[ev.ID]++
		r.mu.Unlock()
	})
	require.NoError(c.t, r.log.Start(context.Background()))
	c.t.Cleanup(r.log.Stop)
	return r
}

func (r *replica) received() []Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Event(nil), r.got...)
}

func (r *replica) topics() []string {
	var ts []string
	for _, ev := range r.received() {
		ts = append(ts, ev.Topic)
	}
	return ts
}

func (r *replica) waitFor(t *testing.T, n int, within time.Duration) []Event {
	t.Helper()
	require.Eventually(t, func() bool { return len(r.received()) >= n }, within, 20*time.Millisecond,
		"%s received %d of %d events: %v", r.name, len(r.received()), n, r.topics())
	return r.received()
}

func (r *replica) assertNoDuplicates(t *testing.T) {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, n := range r.counts {
		assert.Equal(t, 1, n, "%s handled event %d %d times", r.name, id, n)
	}
}

func publish(t *testing.T, r *replica, topic string, payload string) {
	t.Helper()
	require.NoError(t, r.log.Publish(context.Background(), topic, []byte(payload)))
}

// Events reach every other replica, once each, in publish order, with the
// payload intact; the publisher does not hear its own.
func TestEventsReachEveryOtherReplicaOnce_Postgres(t *testing.T) {
	c := newTestCluster(t)
	a := c.startReplica("a", LogOptions{})
	b := c.startReplica("b", LogOptions{})
	d := c.startReplica("d", LogOptions{})

	for i := 0; i < 5; i++ {
		publish(t, a, fmt.Sprintf("from-a.%d", i), fmt.Sprintf(`{"n":%d}`, i))
	}
	publish(t, b, "from-b", "b")

	gotB := b.waitFor(t, 5, 5*time.Second)
	gotD := d.waitFor(t, 6, 5*time.Second)
	gotA := a.waitFor(t, 1, 5*time.Second)

	assert.Equal(t, []string{"from-a.0", "from-a.1", "from-a.2", "from-a.3", "from-a.4"}, b.topics())
	assert.Equal(t, []string{"from-b"}, a.topics())
	assert.Len(t, gotD, 6)
	assert.Equal(t, `{"n":3}`, string(gotB[3].Payload))
	assert.Equal(t, "a", gotB[0].Origin)
	assert.Equal(t, "b", gotA[0].Origin)
	for i := 1; i < len(gotD); i++ {
		assert.Less(t, gotD[i-1].ID, gotD[i].ID, "events handled in id order")
	}

	time.Sleep(1500 * time.Millisecond) // a few more polls
	for _, r := range []*replica{a, b, d} {
		r.assertNoDuplicates(t)
	}
	assert.Len(t, b.received(), 5)
	assert.Len(t, d.received(), 6)
}

// NOTIFY only makes delivery fast: an event inserted without one (as if the
// notification were lost) still arrives by polling.
func TestEventWithoutNotificationArrivesByPolling_Postgres(t *testing.T) {
	c := newTestCluster(t)
	b := c.startReplica("b", LogOptions{PollInterval: 200 * time.Millisecond})
	require.NoError(t, c.replicaDB("writer").Exec(
		`INSERT INTO cluster_events (topic, origin, payload, created_at) VALUES ('silent', 'a', '', now())`).Error)
	got := b.waitFor(t, 1, 3*time.Second)
	assert.Equal(t, "silent", got[0].Topic)
}

// With the replica's database connections killed (pool and listener), events
// published meanwhile still arrive: reads fail and are retried, the listener
// reconnects and triggers a catch-up read.
func TestEventsSurviveLosingTheDatabaseConnections_Postgres(t *testing.T) {
	c := newTestCluster(t)
	a := c.startReplica("a", LogOptions{})
	b := c.startReplica("b", LogOptions{PollInterval: 200 * time.Millisecond})

	publish(t, a, "before", "")
	b.waitFor(t, 1, 5*time.Second)

	require.Greater(t, c.killConnections("b"), 0)
	publish(t, a, "during.1", "")
	publish(t, a, "during.2", "")

	b.waitFor(t, 3, 20*time.Second)
	assert.Equal(t, []string{"before", "during.1", "during.2"}, b.topics())
	require.Eventually(t, func() bool { return b.log.Stats().ListenerReconnects >= 1 }, 20*time.Second, 50*time.Millisecond)
	assert.Empty(t, b.log.Stats().LastError, "the log recovers once the database answers again")
	b.assertNoDuplicates(t)
}

// An id is allocated when a row is inserted but becomes visible when its
// transaction commits. A row whose transaction commits after a later row was
// read (lower id, later commit) is still delivered.
func TestLateCommittedEventIsDelivered_Postgres(t *testing.T) {
	c := newTestCluster(t)
	b := c.startReplica("b", LogOptions{PollInterval: 100 * time.Millisecond})
	writer := c.replicaDB("writer")

	slow := writer.Begin()
	require.NoError(t, slow.Error)
	require.NoError(t, slow.Exec(`INSERT INTO cluster_events (topic, origin, payload, created_at) VALUES ('late', 'a', '', now())`).Error)

	a := c.startReplica("a", LogOptions{})
	publish(t, a, "on-time", "")
	b.waitFor(t, 1, 5*time.Second)
	time.Sleep(300 * time.Millisecond) // cursor is now past the uncommitted id
	require.NoError(t, slow.Commit().Error)

	got := b.waitFor(t, 2, 5*time.Second)
	assert.Equal(t, []string{"on-time", "late"}, []string{got[0].Topic, got[1].Topic})
	assert.Less(t, got[1].ID, got[0].ID, "the late event has the lower id")
	b.assertNoDuplicates(t)
}

// A replica does not replay what was published before it started.
func TestStartDoesNotReplayHistory_Postgres(t *testing.T) {
	c := newTestCluster(t)
	a := c.startReplica("a", LogOptions{})
	publish(t, a, "old.1", "")
	publish(t, a, "old.2", "")

	b := c.startReplica("b", LogOptions{PollInterval: 100 * time.Millisecond})
	publish(t, a, "new", "")
	b.waitFor(t, 1, 5*time.Second)
	time.Sleep(500 * time.Millisecond)
	assert.Equal(t, []string{"new"}, b.topics())
}

// Payloads are rows, not NOTIFY payloads, so they are not limited to
// NOTIFY's 8000 bytes.
func TestLargePayload_Postgres(t *testing.T) {
	c := newTestCluster(t)
	a := c.startReplica("a", LogOptions{})
	b := c.startReplica("b", LogOptions{})
	big := strings.Repeat("y", 256*1024)
	publish(t, a, "big", big)
	got := b.waitFor(t, 1, 5*time.Second)
	assert.Equal(t, big, string(got[0].Payload))
}

// A burst larger than a batch is delivered completely, including when the
// window is full of already-seen rows.
func TestBurstLargerThanABatch_Postgres(t *testing.T) {
	c := newTestCluster(t)
	a := c.startReplica("a", LogOptions{})
	b := c.startReplica("b", LogOptions{BatchSize: 7, PollInterval: 100 * time.Millisecond})
	for i := 0; i < 40; i++ {
		publish(t, a, fmt.Sprintf("burst.%02d", i), "")
	}
	b.waitFor(t, 40, 10*time.Second)
	for i := 0; i < 25; i++ {
		publish(t, a, fmt.Sprintf("second.%02d", i), "")
	}
	b.waitFor(t, 65, 10*time.Second)
	b.assertNoDuplicates(t)
}

// A panicking handler is logged and does not stop later events or other
// handlers.
func TestPanickingHandlerDoesNotStopDelivery_Postgres(t *testing.T) {
	c := newTestCluster(t)
	a := c.startReplica("a", LogOptions{})
	b := c.startReplica("b", LogOptions{})
	b.log.Subscribe("boom", func(Event) { panic("handler bug") })
	publish(t, a, "boom", "")
	publish(t, a, "after", "")
	got := b.waitFor(t, 2, 5*time.Second)
	assert.Equal(t, "after", got[1].Topic)
}

// A topic handler only sees its topic; unsubscribing stops it.
func TestTopicSubscription_Postgres(t *testing.T) {
	c := newTestCluster(t)
	a := c.startReplica("a", LogOptions{})
	b := c.startReplica("b", LogOptions{})
	var mu sync.Mutex
	var mine []string
	unsub := b.log.Subscribe("mine", func(ev Event) { mu.Lock(); mine = append(mine, string(ev.Payload)); mu.Unlock() })
	publish(t, a, "mine", "1")
	publish(t, a, "other", "x")
	b.waitFor(t, 2, 5*time.Second)
	unsub()
	publish(t, a, "mine", "2")
	b.waitFor(t, 3, 5*time.Second)
	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, []string{"1"}, mine)
}

// Pruning removes rows past the retention and keeps the rest.
func TestPruneKeepsRecentEvents_Postgres(t *testing.T) {
	c := newTestCluster(t)
	db := c.replicaDB("writer")
	require.NoError(t, db.Exec(`INSERT INTO cluster_events (topic, origin, payload, created_at) VALUES ('ancient', 'a', '', now() - interval '2 hours'), ('fresh', 'a', '', now())`).Error)
	l := NewLog(db, "pruner", LogOptions{Window: time.Second, Retention: time.Hour})
	l.prune()
	var topics []string
	require.NoError(t, db.Model(&models.ClusterEvent{}).Order("id").Pluck("topic", &topics).Error)
	assert.Equal(t, []string{"fresh"}, topics)
}

// No handler runs after Stop returns.
func TestNoDeliveryAfterStop_Postgres(t *testing.T) {
	c := newTestCluster(t)
	a := c.startReplica("a", LogOptions{})
	b := c.startReplica("b", LogOptions{PollInterval: 50 * time.Millisecond})
	publish(t, a, "one", "")
	b.waitFor(t, 1, 5*time.Second)
	b.log.Stop()
	publish(t, a, "two", "")
	time.Sleep(500 * time.Millisecond)
	assert.Len(t, b.received(), 1)
}

// Nodes: a registered node is live; a stopped one is gone at once; one that
// stops refreshing expires after the liveness window.
func TestNodeLiveness_Postgres(t *testing.T) {
	c := newTestCluster(t)
	origWindow, origBeat := LivenessWindow, HeartbeatInterval
	LivenessWindow, HeartbeatInterval = 2*time.Second, 200*time.Millisecond
	t.Cleanup(func() { LivenessWindow, HeartbeatInterval = origWindow, origBeat })

	db := c.replicaDB("nodes")
	a, err := StartNode(db, "node-a", "test")
	require.NoError(t, err)
	b, err := StartNode(db, "node-b", "test")
	require.NoError(t, err)

	live, err := LiveNodes(db)
	require.NoError(t, err)
	assert.Equal(t, []string{"node-a", "node-b"}, nodeIDs(live))

	// Graceful stop: gone at once.
	b.Stop(context.Background())
	ok, err := IsLive(db, "node-b")
	require.NoError(t, err)
	assert.False(t, ok)

	// A crashed node (row left behind, no refreshes) expires.
	close(a.stop)
	<-a.done
	ok, err = IsLive(db, "node-a")
	require.NoError(t, err)
	assert.True(t, ok, "still inside the window")
	require.Eventually(t, func() bool {
		ok, err := IsLive(db, "node-a")
		return err == nil && !ok
	}, 5*time.Second, 100*time.Millisecond, "node-a expires")

	ok, err = IsLive(db, "")
	require.NoError(t, err)
	assert.False(t, ok)
}

// A running node refreshes its row, so it stays live well past the window.
func TestRunningNodeStaysLive_Postgres(t *testing.T) {
	c := newTestCluster(t)
	origWindow, origBeat := LivenessWindow, HeartbeatInterval
	LivenessWindow, HeartbeatInterval = time.Second, 100*time.Millisecond
	t.Cleanup(func() { LivenessWindow, HeartbeatInterval = origWindow, origBeat })

	db := c.replicaDB("nodes")
	n, err := StartNode(db, "node-steady", "test")
	require.NoError(t, err)
	defer n.Stop(context.Background())
	for i := 0; i < 6; i++ {
		time.Sleep(500 * time.Millisecond)
		ok, err := IsLive(db, "node-steady")
		require.NoError(t, err)
		require.True(t, ok, "live after %s", time.Duration(i+1)*500*time.Millisecond)
	}
}

func nodeIDs(nodes []models.ClusterNode) []string {
	var ids []string
	for _, n := range nodes {
		ids = append(ids, n.NodeID)
	}
	return ids
}

func TestNodePrunesLongStoppedNodes_Postgres(t *testing.T) {
	pruneCase(t, newTestCluster(t).replicaDB("nodes"))
}
