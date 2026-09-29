package studio

import (
	"errors"
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
