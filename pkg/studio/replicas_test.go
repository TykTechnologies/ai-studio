package studio

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/pkg/replicas"
)

// A burst of triggers runs the work once; a trigger during a run runs it
// once more afterwards (so the last change is always applied); stop waits
// for a run in progress and drops what is pending.
func TestCoalescer(t *testing.T) {
	var runs atomic.Int32
	release := make(chan struct{})
	c := newCoalescer("test", 5*time.Millisecond, func() error {
		runs.Add(1)
		<-release
		return nil
	})

	for i := 0; i < 50; i++ {
		c.trigger()
	}
	require.Eventually(t, func() bool { return runs.Load() == 1 }, time.Second, time.Millisecond)

	// Triggers while running: exactly one more run.
	for i := 0; i < 10; i++ {
		c.trigger()
	}
	release <- struct{}{}
	require.Eventually(t, func() bool { return runs.Load() == 2 }, time.Second, time.Millisecond)
	release <- struct{}{}
	time.Sleep(30 * time.Millisecond)
	assert.Equal(t, int32(2), runs.Load())

	// Idle again: a new trigger runs it again.
	c.trigger()
	require.Eventually(t, func() bool { return runs.Load() == 3 }, time.Second, time.Millisecond)
	c.trigger() // pending while running
	done := make(chan struct{})
	go func() { c.stop(); close(done) }()
	select {
	case <-done:
		t.Fatal("stop returned while a run was in progress")
	case <-time.After(20 * time.Millisecond):
	}
	release <- struct{}{}
	<-done
	c.trigger() // after stop: ignored
	time.Sleep(20 * time.Millisecond)
	assert.Equal(t, int32(3), runs.Load(), "the pending trigger is dropped on stop")
}

func TestCoalescer_ErrorsDoNotStopIt(t *testing.T) {
	var runs atomic.Int32
	c := newCoalescer("test", time.Millisecond, func() error {
		runs.Add(1)
		return errors.New("database is down")
	})
	c.trigger()
	require.Eventually(t, func() bool { return runs.Load() == 1 }, time.Second, time.Millisecond)
	c.trigger()
	require.Eventually(t, func() bool { return runs.Load() == 2 }, time.Second, time.Millisecond)
	c.stop()
}

// A run that panics counts as a failed one: the coalescer runs again on the
// next trigger (a dead goroutine would leave it marked running for good).
func TestCoalescer_PanicsDoNotStopIt(t *testing.T) {
	var runs atomic.Int32
	c := newCoalescer("test", time.Millisecond, func() error {
		if runs.Add(1) == 1 {
			panic("reload bug")
		}
		return nil
	})
	c.trigger()
	require.Eventually(t, func() bool { return runs.Load() == 1 }, time.Second, time.Millisecond)
	c.trigger()
	require.Eventually(t, func() bool { return runs.Load() == 2 }, time.Second, time.Millisecond)
	c.stop()
}

type fakePublisher struct {
	mu       sync.Mutex
	sent     []string
	failures int
	block    chan struct{}
}

func (f *fakePublisher) Publish(_ context.Context, topic string, payload []byte) error {
	if f.block != nil {
		<-f.block
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failures > 0 {
		f.failures--
		return errors.New("database is down")
	}
	f.sent = append(f.sent, topic+":"+string(payload))
	return nil
}

func (f *fakePublisher) got() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.sent...)
}

// Signals never make the caller (an API request) wait for the database;
// repeats waiting to be written are sent once; a failed write is retried;
// close writes what is pending.
func TestSignalSender(t *testing.T) {
	pub := &fakePublisher{block: make(chan struct{})}
	ss := newSignalSender(pub)

	done := make(chan struct{})
	go func() {
		for i := 0; i < 20; i++ {
			ss.send("budgets")
		}
		ss.send("governed_metadata")
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("send waited for the database")
	}
	close(pub.block)
	require.Eventually(t, func() bool { return len(pub.got()) >= 2 }, 5*time.Second, 5*time.Millisecond)
	ss.close()
	got := pub.got()
	assert.LessOrEqual(t, len(got), 3, "repeats coalesced: %v", got)
	assert.Contains(t, got, signalTopic+":budgets")
	assert.Contains(t, got, signalTopic+":governed_metadata")

	flaky := &fakePublisher{failures: 2}
	ss = newSignalSender(flaky)
	ss.send("budgets")
	ss.close()
	assert.Equal(t, []string{signalTopic + ":budgets"}, flaky.got(), "retried until written, and flushed on close")
}

// An embedder change reloads the embedded gateway like a datasource change:
// datasources carry their embedder in memory, so an embedder's revoked
// "Allow internal host" switch or rotated key would otherwise not reach
// them until the datasource itself changed (w4e).
func TestReloadsGateway(t *testing.T) {
	for _, topic := range []string{"system.llm.updated", "system.datasource.created", "system.embedder.updated",
		"system.embedder.deleted", "system.filter.updated", "system.plugin.deleted"} {
		if !reloadsGateway(topic) {
			t.Errorf("%s does not reload the gateway", topic)
		}
	}
	for _, topic := range []string{"system.app.updated", "system.user.created", "system.tool.updated", "system.embedderx"} {
		if reloadsGateway(topic) {
			t.Errorf("%s reloads the gateway", topic)
		}
	}
}

// A replica that could not read the cluster event log for longer than events
// are kept rebuilds from the database what events keep current: the embedded
// gateway is reloaded, the budget caches are cleared, every replica signal's
// handlers run, and the plugins are checked against the database from before
// it went away.
func TestRelayedChangesAfterGap(t *testing.T) {
	var gateway, budgets, signalled atomic.Int32
	rc := &relayedChanges{
		reloadGateway: newCoalescer("gateway", time.Millisecond, func() error { gateway.Add(1); return nil }),
		clearBudgets:  newCoalescer("budgets", time.Millisecond, func() error { budgets.Add(1); return nil }),
		plugins:       make(chan pluginChange, 1),
		pluginsDone:   make(chan struct{}),
		unsubscribe:   func() {},
	}
	remove := replicas.OnSignal(replicas.SignalBranding, func() { signalled.Add(1) })
	t.Cleanup(remove)

	before := time.Now()
	rc.afterGap(20 * time.Minute)

	require.Eventually(t, func() bool { return gateway.Load() == 1 && budgets.Load() == 1 }, time.Second, time.Millisecond)
	assert.EqualValues(t, 1, signalled.Load())
	ch := <-rc.plugins
	assert.Zero(t, ch.id)
	assert.WithinDuration(t, before.Add(-20*time.Minute-gapMargin), ch.reconcileSince, time.Second,
		"plugins are checked from before the replica went away, with a margin for other replicas' clocks")

	// A full queue is reported, not waited for; after stop nothing is queued.
	rc.plugins <- pluginChange{id: 1}
	rc.afterGap(20 * time.Minute)
	<-rc.plugins
	close(rc.pluginsDone)
	rc.stop()
	rc.afterGap(20 * time.Minute)
}
