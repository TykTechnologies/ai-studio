package safe

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func fastBackoff(t *testing.T) {
	t.Helper()
	min, max, reset := loopBackoffMin, loopBackoffMax, loopBackoffReset
	loopBackoffMin, loopBackoffMax, loopBackoffReset = time.Millisecond, 4*time.Millisecond, time.Hour
	t.Cleanup(func() { loopBackoffMin, loopBackoffMax, loopBackoffReset = min, max, reset })
}

func TestGoRecoversAndCounts(t *testing.T) {
	before := Panics()
	done := make(chan struct{})
	Go("test one-shot", func() {
		defer close(done)
		panic("boom")
	})
	<-done
	waitFor(t, func() bool { return Panics() == before+1 })
}

func TestRecoverDeferred(t *testing.T) {
	before := Panics()
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		defer Recover("test deferred")
		var m map[string]int
		m["x"] = 1 // nil map write: a runtime panic
	}()
	<-finished
	if got := Panics(); got != before+1 {
		t.Fatalf("Panics() = %d, want %d", got, before+1)
	}
}

func TestCallReportsPanic(t *testing.T) {
	if Call("test call", func() {}) {
		t.Fatal("Call reported a panic for a function that returned")
	}
	if !Call("test call", func() { panic(nil) }) {
		t.Fatal("Call did not report panic(nil)")
	}
}

func TestLoopReturnsWhenFnReturns(t *testing.T) {
	runs := 0
	Loop("test loop", make(chan struct{}), func() { runs++ })
	if runs != 1 {
		t.Fatalf("fn ran %d times, want 1", runs)
	}
}

func TestLoopRestartsAfterPanic(t *testing.T) {
	fastBackoff(t)
	var runs, resets int
	Loop("test loop", make(chan struct{}), func() {
		runs++
		if runs < 3 {
			panic("boom")
		}
	}, AfterPanic(func() { resets++ }))
	if runs != 3 || resets != 2 {
		t.Fatalf("runs = %d, resets = %d; want 3 and 2", runs, resets)
	}
}

func TestLoopStopsDuringBackoff(t *testing.T) {
	min := loopBackoffMin
	loopBackoffMin = time.Hour
	t.Cleanup(func() { loopBackoffMin = min })

	stop := make(chan struct{})
	var runs atomic.Int32
	done := make(chan struct{})
	go func() {
		defer close(done)
		Loop("test loop", stop, func() {
			runs.Add(1)
			panic("boom")
		})
	}()
	waitFor(t, func() bool { return runs.Load() == 1 })
	close(stop)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Loop did not return when stop closed during its backoff")
	}
	if runs.Load() != 1 {
		t.Fatalf("fn ran %d times after stop, want 1", runs.Load())
	}
}

// A panicking AfterPanic must not end the loop either.
func TestLoopSurvivesPanickingAfterPanic(t *testing.T) {
	fastBackoff(t)
	runs := 0
	Loop("test loop", make(chan struct{}), func() {
		runs++
		if runs == 1 {
			panic("boom")
		}
	}, AfterPanic(func() { panic("again") }))
	if runs != 2 {
		t.Fatalf("runs = %d, want 2", runs)
	}
}

func TestConcurrentPanicsCounted(t *testing.T) {
	before := Panics()
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			Call("test concurrent", func() { panic("boom") })
		}()
	}
	wg.Wait()
	if got := Panics(); got != before+20 {
		t.Fatalf("Panics() = %d, want %d", got, before+20)
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not met within 5s")
		}
		time.Sleep(time.Millisecond)
	}
}
