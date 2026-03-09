package memory_test

import (
	"sync"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/ratelimit"
	"github.com/TykTechnologies/midsommar/v2/ratelimit/memory"
)

func TestAllow(t *testing.T) {
	ctx := t.Context()
	backend := memory.New(ctx, time.Minute)
	l := ratelimit.NewLimiter(backend, 3, time.Minute)

	for range 3 {
		allowed, _ := l.Allow(ctx, "key")
		if !allowed {
			t.Fatal("should be allowed")
		}
	}

	allowed, retryAfter := l.Allow(ctx, "key")
	if allowed {
		t.Fatal("4th request should be denied")
	}
	if retryAfter <= 0 {
		t.Fatal("retryAfter should be positive")
	}
}

func TestSeparateKeys(t *testing.T) {
	ctx := t.Context()
	backend := memory.New(ctx, time.Minute)
	l := ratelimit.NewLimiter(backend, 1, time.Minute)

	allowed, _ := l.Allow(ctx, "a")
	if !allowed {
		t.Fatal("first key should be allowed")
	}

	allowed, _ = l.Allow(ctx, "b")
	if !allowed {
		t.Fatal("second key should be allowed independently")
	}

	allowed, _ = l.Allow(ctx, "a")
	if allowed {
		t.Fatal("first key should be denied after limit")
	}
}

func TestWindowExpiry(t *testing.T) {
	ctx := t.Context()
	backend := memory.New(ctx, time.Minute)
	window := 50 * time.Millisecond
	l := ratelimit.NewLimiter(backend, 1, window)

	allowed, _ := l.Allow(ctx, "key")
	if !allowed {
		t.Fatal("should be allowed")
	}

	allowed, _ = l.Allow(ctx, "key")
	if allowed {
		t.Fatal("should be denied")
	}

	time.Sleep(window + 10*time.Millisecond)

	allowed, _ = l.Allow(ctx, "key")
	if !allowed {
		t.Fatal("should be allowed after window expires")
	}
}

func TestRetryAfterAccuracy(t *testing.T) {
	ctx := t.Context()
	backend := memory.New(ctx, time.Minute)
	window := 200 * time.Millisecond
	l := ratelimit.NewLimiter(backend, 1, window)

	l.Allow(ctx, "key")
	_, retryAfter := l.Allow(ctx, "key")

	if retryAfter > window {
		t.Fatalf("retryAfter %v should not exceed window %v", retryAfter, window)
	}
	if retryAfter < window/2 {
		t.Fatalf("retryAfter %v seems too small for window %v", retryAfter, window)
	}
}

func TestReset(t *testing.T) {
	ctx := t.Context()
	backend := memory.New(ctx, time.Minute)
	l := ratelimit.NewLimiter(backend, 1, time.Minute)

	l.Allow(ctx, "key")
	allowed, _ := l.Allow(ctx, "key")
	if allowed {
		t.Fatal("should be denied")
	}

	l.Reset(ctx, "key")

	allowed, _ = l.Allow(ctx, "key")
	if !allowed {
		t.Fatal("should be allowed after reset")
	}
}

func TestConcurrentSafety(t *testing.T) {
	ctx := t.Context()
	backend := memory.New(ctx, time.Minute)
	l := ratelimit.NewLimiter(backend, 100, time.Minute)

	var wg sync.WaitGroup
	for range 200 {
		wg.Go(func() {
			l.Allow(ctx, "key")
		})
	}
	wg.Wait()

	allowed, _ := l.Allow(ctx, "key")
	if allowed {
		t.Fatal("should be denied after concurrent flood")
	}
}

func TestCleanupStopsOnCancel(t *testing.T) {
	ctx := t.Context()
	backend := memory.New(ctx, 10*time.Millisecond)
	l := ratelimit.NewLimiter(backend, 1, 10*time.Millisecond)
	l.Allow(ctx, "key")
	time.Sleep(30 * time.Millisecond)
}
