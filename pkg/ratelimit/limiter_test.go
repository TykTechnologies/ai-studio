package ratelimit

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestAllow(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	l := NewLimiter(3, time.Minute, ctx)

	for i := 0; i < 3; i++ {
		allowed, _ := l.Allow("key")
		if !allowed {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}

	allowed, retryAfter := l.Allow("key")
	if allowed {
		t.Fatal("4th request should be denied")
	}
	if retryAfter <= 0 {
		t.Fatal("retryAfter should be positive")
	}
}

func TestSeparateKeys(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	l := NewLimiter(1, time.Minute, ctx)

	allowed, _ := l.Allow("a")
	if !allowed {
		t.Fatal("first key should be allowed")
	}

	allowed, _ = l.Allow("b")
	if !allowed {
		t.Fatal("second key should be allowed independently")
	}

	allowed, _ = l.Allow("a")
	if allowed {
		t.Fatal("first key should be denied after limit")
	}
}

func TestWindowExpiry(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	window := 50 * time.Millisecond
	l := NewLimiter(1, window, ctx)

	allowed, _ := l.Allow("key")
	if !allowed {
		t.Fatal("should be allowed")
	}

	allowed, _ = l.Allow("key")
	if allowed {
		t.Fatal("should be denied")
	}

	time.Sleep(window + 10*time.Millisecond)

	allowed, _ = l.Allow("key")
	if !allowed {
		t.Fatal("should be allowed after window expires")
	}
}

func TestRetryAfterAccuracy(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	window := 200 * time.Millisecond
	l := NewLimiter(1, window, ctx)

	l.Allow("key")
	_, retryAfter := l.Allow("key")

	if retryAfter > window {
		t.Fatalf("retryAfter %v should not exceed window %v", retryAfter, window)
	}
	if retryAfter < window/2 {
		t.Fatalf("retryAfter %v seems too small for window %v", retryAfter, window)
	}
}

func TestReset(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	l := NewLimiter(1, time.Minute, ctx)

	l.Allow("key")
	allowed, _ := l.Allow("key")
	if allowed {
		t.Fatal("should be denied")
	}

	l.Reset("key")

	allowed, _ = l.Allow("key")
	if !allowed {
		t.Fatal("should be allowed after reset")
	}
}

func TestConcurrentSafety(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	l := NewLimiter(100, time.Minute, ctx)

	var wg sync.WaitGroup
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			l.Allow("key")
		}()
	}
	wg.Wait()

	// After 200 concurrent attempts with limit 100, further requests should be denied
	allowed, _ := l.Allow("key")
	if allowed {
		t.Fatal("should be denied after concurrent flood")
	}
}

func TestCleanupStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	l := NewLimiter(1, 10*time.Millisecond, ctx)
	l.Allow("key")
	cancel()
	// Just verify no panic or hang
	time.Sleep(30 * time.Millisecond)
}
