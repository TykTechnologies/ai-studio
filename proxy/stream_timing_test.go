package proxy

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestStreamTimingNilIsSafe(t *testing.T) {
	// Non-streaming requests carry no timing, so every accessor must tolerate nil.
	var timing *streamTiming
	timing.markFirstChunk(time.Now())
	if _, ok := timing.timeToFirstToken(); ok {
		t.Error("nil timing must not report a first token")
	}
	if _, ok := timing.sinceFirstToken(time.Now()); ok {
		t.Error("nil timing must not report time since first token")
	}
	if got := streamTimingFrom(context.Background()); got != nil {
		t.Errorf("expected nil timing from a bare context, got %v", got)
	}
}

func TestStreamTimingNoChunkNeverReports(t *testing.T) {
	timing := newStreamTiming(time.Now())
	if _, ok := timing.timeToFirstToken(); ok {
		t.Error("a stream with no chunks must not report a first token")
	}
	if _, ok := timing.sinceFirstToken(time.Now()); ok {
		t.Error("a stream with no chunks must not report time since first token")
	}
}

func TestStreamTimingMarksOnlyTheFirstChunk(t *testing.T) {
	start := time.Now()
	timing := newStreamTiming(start)

	timing.markFirstChunk(start.Add(100 * time.Millisecond))
	timing.markFirstChunk(start.Add(900 * time.Millisecond))

	ttft, ok := timing.timeToFirstToken()
	if !ok {
		t.Fatal("expected a first token observation")
	}
	if ttft != 100*time.Millisecond {
		t.Errorf("time to first token = %v, want 100ms — a later chunk overwrote it", ttft)
	}

	elapsed, ok := timing.sinceFirstToken(start.Add(600 * time.Millisecond))
	if !ok {
		t.Fatal("expected a since-first-token observation")
	}
	if elapsed != 500*time.Millisecond {
		t.Errorf("time after first token = %v, want 500ms", elapsed)
	}
}

func TestStreamTimingRoundTripsOnContext(t *testing.T) {
	start := time.Now()
	timing := newStreamTiming(start)
	ctx := withStreamTiming(context.Background(), timing)

	got := streamTimingFrom(ctx)
	if got != timing {
		t.Fatalf("expected the same timing back from the context, got %v", got)
	}

	got.markFirstChunk(start.Add(50 * time.Millisecond))
	if ttft, ok := timing.timeToFirstToken(); !ok || ttft != 50*time.Millisecond {
		t.Errorf("marking through the context value did not reach the original: %v %v", ttft, ok)
	}
}

func TestStreamTimingConcurrentMarkAndRead(t *testing.T) {
	// The analytics goroutine reads the landmark while the stream is still being
	// drained, so this must be race-free under -race.
	start := time.Now()
	timing := newStreamTiming(start)

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			timing.markFirstChunk(start.Add(time.Duration(i+1) * time.Millisecond))
		}(i)
		go func() {
			defer wg.Done()
			timing.timeToFirstToken()
			timing.sinceFirstToken(time.Now())
		}()
	}
	wg.Wait()

	if _, ok := timing.timeToFirstToken(); !ok {
		t.Error("expected a first token observation after concurrent marking")
	}
}
