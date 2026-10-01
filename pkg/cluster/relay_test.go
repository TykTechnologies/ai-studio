package cluster

import (
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/pkg/eventbridge"
)

// busRecorder records what a replica's local bus carried.
type busRecorder struct {
	mu  sync.Mutex
	evs []eventbridge.Event
}

func (b *busRecorder) record(ev eventbridge.Event) {
	b.mu.Lock()
	b.evs = append(b.evs, ev)
	b.mu.Unlock()
}

func (b *busRecorder) byTopic(topic string) []eventbridge.Event {
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []eventbridge.Event
	for _, ev := range b.evs {
		if ev.Topic == topic {
			out = append(out, ev)
		}
	}
	return out
}

type relayReplica struct {
	*replica
	bus   *eventbridge.PubSubBus
	relay *Relay
	seen  *busRecorder
}

func (c *testCluster) startRelayReplica(name string, opts RelayOptions) *relayReplica {
	c.t.Helper()
	r := &relayReplica{replica: c.startReplica(name, LogOptions{PollInterval: 50 * time.Millisecond}), bus: eventbridge.NewBus(), seen: &busRecorder{}}
	r.bus.SubscribeAll(r.seen.record)
	r.relay = NewRelay(r.log, r.bus, opts)
	r.relay.Start()
	c.t.Cleanup(r.relay.Stop)
	return r
}

func busPublish(t *testing.T, bus eventbridge.Bus, topic string, dir eventbridge.Direction) eventbridge.Event {
	t.Helper()
	ev, err := eventbridge.NewEvent(topic, "control", dir, map[string]string{"k": topic})
	require.NoError(t, err)
	bus.Publish(ev)
	return ev
}

// Edge-bound events and object changes published on one replica appear on
// every other replica's bus once, with the same ID, marked as relayed; they
// are not relayed back.
func TestRelay_CarriesEdgeAndObjectEventsToEveryReplica(t *testing.T) {
	c := newTestCluster(t)
	a := c.startRelayReplica("a", RelayOptions{})
	b := c.startRelayReplica("b", RelayOptions{})
	d := c.startRelayReplica("d", RelayOptions{})

	down := busPublish(t, a.bus, "budget.sync", eventbridge.DirDown)
	obj := busPublish(t, a.bus, "system.llm.updated", eventbridge.DirLocal)
	busPublish(t, a.bus, "plugin.local.chatter", eventbridge.DirLocal) // stays on a
	busPublish(t, a.bus, "edge.metrics", eventbridge.DirUp)            // arrived from an edge: a handles it

	for _, r := range []*relayReplica{b, d} {
		require.Eventually(t, func() bool {
			return len(r.seen.byTopic("budget.sync")) == 1 && len(r.seen.byTopic("system.llm.updated")) == 1
		}, 10*time.Second, 20*time.Millisecond, "replica %s", r.name)
		got := r.seen.byTopic("budget.sync")[0]
		assert.Equal(t, down.ID, got.ID)
		assert.Equal(t, eventbridge.DirDown, got.Dir, "still bound for edges")
		assert.Equal(t, "a", got.RelayedFrom)
		assert.JSONEq(t, string(down.Payload), string(got.Payload))
		assert.Equal(t, obj.ID, r.seen.byTopic("system.llm.updated")[0].ID)
	}

	time.Sleep(300 * time.Millisecond)
	for _, r := range []*relayReplica{b, d} {
		assert.Empty(t, r.seen.byTopic("plugin.local.chatter"), r.name)
		assert.Empty(t, r.seen.byTopic("edge.metrics"), r.name)
		assert.Len(t, r.seen.byTopic("budget.sync"), 1, "exactly once on %s", r.name)
	}
	assert.Len(t, a.seen.byTopic("budget.sync"), 1, "not relayed back to its origin")
	assert.Equal(t, RelayStats{Sent: 2}, a.relay.Stats())
	assert.Equal(t, uint64(0), b.relay.Stats().Sent, "b never relays what it received")
	assert.Equal(t, uint64(2), b.relay.Stats().Received)
}

// A burst keeps its order per publishing replica.
// Stopping a relay while the log is delivering other replicas' events onto
// its bus is safe (under -race): the bus library's Unsubscribe is not
// synchronised with a Publish in flight on another goroutine.
func TestRelay_StopWhileDelivering(t *testing.T) {
	c := newTestCluster(t)
	a := c.startRelayReplica("a", RelayOptions{})

	// Replica b's log reader publishes a relayed event on b's bus, where it
	// reaches the relay's own subscription first (the bus invokes the newest
	// first) and is then held in the subscriber below until b's relay stops.
	// Nothing orders the reader's visit to the relay's subscription before
	// the relay's Stop except the relay itself: the test must not signal
	// from the reader, so it waits for the delivery by time.
	b := c.startReplica("b", LogOptions{PollInterval: 50 * time.Millisecond})
	bus := eventbridge.NewBus()
	stopping := make(chan struct{})
	var held atomic.Bool
	bus.SubscribeAll(func(ev eventbridge.Event) {
		if ev.RelayedFrom == "" || held.Swap(true) {
			return
		}
		select {
		case <-stopping:
		case <-time.After(5 * time.Second):
		}
	})
	relay := NewRelay(b.log, bus, RelayOptions{})
	relay.Start()
	t.Cleanup(relay.Stop)

	busPublish(t, a.bus, "system.test.stop", eventbridge.DirLocal)
	time.Sleep(time.Second)
	close(stopping)
	relay.Stop()
	assert.True(t, held.Load(), "the event was relayed")
}

func TestRelay_KeepsOrder(t *testing.T) {
	c := newTestCluster(t)
	a := c.startRelayReplica("a", RelayOptions{})
	b := c.startRelayReplica("b", RelayOptions{})

	var want []string
	for i := 0; i < 200; i++ {
		ev := busPublish(t, a.bus, "budget.sync", eventbridge.DirDown)
		want = append(want, ev.ID)
	}
	require.Eventually(t, func() bool { return len(b.seen.byTopic("budget.sync")) == 200 }, 20*time.Second, 50*time.Millisecond)
	var got []string
	for _, ev := range b.seen.byTopic("budget.sync") {
		got = append(got, ev.ID)
	}
	assert.Equal(t, want, got)
}

// While the database is unreachable events wait and are retried; they are
// delivered once it is back.
func TestRelay_RetriesWhileTheDatabaseIsDown(t *testing.T) {
	c := newTestCluster(t)
	a := c.startRelayReplica("a", RelayOptions{MaxRetries: 50})
	b := c.startRelayReplica("b", RelayOptions{})

	c.killConnections("a")
	ev := busPublish(t, a.bus, "budget.sync", eventbridge.DirDown)
	require.Eventually(t, func() bool {
		got := b.seen.byTopic("budget.sync")
		return len(got) == 1 && got[0].ID == ev.ID
	}, 20*time.Second, 50*time.Millisecond)
	assert.Zero(t, a.relay.Stats().Dropped)
}

// A full queue drops (and counts) instead of blocking the publisher: a bus
// publish runs inside API requests.
func TestRelay_FullQueueDoesNotBlockPublishers(t *testing.T) {
	c := newTestCluster(t)
	r := &relayReplica{replica: c.startReplica("a", LogOptions{}), bus: eventbridge.NewBus(), seen: &busRecorder{}}
	r.relay = NewRelay(r.log, r.bus, RelayOptions{QueueSize: 1})
	// Not started: nothing drains the queue.
	r.relay.sub = r.bus.SubscribeAll(r.relay.fromBus)

	done := make(chan struct{})
	go func() {
		for i := 0; i < 10; i++ {
			busPublish(t, r.bus, fmt.Sprintf("system.x.%d", i), eventbridge.DirLocal)
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("publishing blocked on the relay")
	}
	assert.Equal(t, uint64(9), r.relay.Stats().Dropped)
}

// Bad rows are skipped, not fatal.
func TestRelay_SkipsUndecodableEvents(t *testing.T) {
	c := newTestCluster(t)
	a := c.startRelayReplica("a", RelayOptions{})
	b := c.startRelayReplica("b", RelayOptions{})
	require.NoError(t, a.log.Publish(t.Context(), RelayTopic, []byte("{not json")))
	ok, err := json.Marshal(eventbridge.Event{ID: "e1", Topic: "system.app.updated"})
	require.NoError(t, err)
	require.NoError(t, a.log.Publish(t.Context(), RelayTopic, ok))
	require.Eventually(t, func() bool { return len(b.seen.byTopic("system.app.updated")) == 1 }, 10*time.Second, 20*time.Millisecond)
	assert.Equal(t, uint64(1), b.relay.Stats().Received)
}
