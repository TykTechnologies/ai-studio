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
