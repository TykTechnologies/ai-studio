package services

import (
	"testing"
	"time"
)

// The broker wait follows the ready signal, not a fixed sleep: a server that
// registers late is still waited for, and a server that never registers
// releases the caller after the bounded timeout.
func TestWaitBrokeredServer(t *testing.T) {
	orig := brokeredServerReadyTimeout
	t.Cleanup(func() { brokeredServerReadyTimeout = orig })

	t.Run("late ready signal is waited for", func(t *testing.T) {
		brokeredServerReadyTimeout = 2 * time.Second
		ready := make(chan struct{})
		closed := make(chan time.Time, 1)
		go func() {
			time.Sleep(700 * time.Millisecond) // longer than the old fixed 500ms wait
			closed <- time.Now()
			close(ready)
		}()
		waitBrokeredServer(ready, 1, 7, "test")
		returned := time.Now()
		if at := <-closed; returned.Before(at) {
			t.Fatal("returned before the server was ready")
		}
	})

	t.Run("never ready returns after the timeout", func(t *testing.T) {
		brokeredServerReadyTimeout = 50 * time.Millisecond
		start := time.Now()
		waitBrokeredServer(make(chan struct{}), 1, 7, "test")
		if waited := time.Since(start); waited < 50*time.Millisecond || waited > 2*time.Second {
			t.Fatalf("waited %s, want about the 50ms timeout", waited)
		}
	})
}
