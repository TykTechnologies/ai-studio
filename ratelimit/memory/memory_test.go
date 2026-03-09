package memory_test

import (
	"sync"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/ratelimit"
	"github.com/TykTechnologies/midsommar/v2/ratelimit/memory"
)

func allow(t *testing.T, l *ratelimit.Limiter, key string) ratelimit.Result {
	t.Helper()
	r, err := l.Allow(t.Context(), key)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !r.Allowed {
		t.Fatalf("expected %q allowed, got denied", key)
	}
	return r
}

func deny(t *testing.T, l *ratelimit.Limiter, key string) ratelimit.Result {
	t.Helper()
	r, err := l.Allow(t.Context(), key)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Allowed {
		t.Fatalf("expected %q denied, got allowed", key)
	}
	return r
}

func TestAllow(t *testing.T) {
	backend := memory.New(t.Context(), time.Minute)
	l := ratelimit.NewLimiter(backend, 3, time.Minute)

	for range 3 {
		allow(t, l, "key")
	}

	r := deny(t, l, "key")
	if r.RetryAfter <= 0 {
		t.Fatal("retryAfter should be positive")
	}
}

func TestSeparateKeys(t *testing.T) {
	backend := memory.New(t.Context(), time.Minute)
	l := ratelimit.NewLimiter(backend, 1, time.Minute)

	allow(t, l, "a")
	allow(t, l, "b")
	deny(t, l, "a")
}

func TestWindowExpiry(t *testing.T) {
	backend := memory.New(t.Context(), time.Minute)
	window := 50 * time.Millisecond
	l := ratelimit.NewLimiter(backend, 1, window)

	allow(t, l, "key")
	deny(t, l, "key")

	time.Sleep(window + 10*time.Millisecond)

	allow(t, l, "key")
}

func TestRetryAfterAccuracy(t *testing.T) {
	backend := memory.New(t.Context(), time.Minute)
	window := 200 * time.Millisecond
	l := ratelimit.NewLimiter(backend, 1, window)

	allow(t, l, "key")
	r := deny(t, l, "key")

	if r.RetryAfter > window {
		t.Fatalf("retryAfter %v should not exceed window %v", r.RetryAfter, window)
	}
	if r.RetryAfter < window/2 {
		t.Fatalf("retryAfter %v seems too small for window %v", r.RetryAfter, window)
	}
}

func TestReset(t *testing.T) {
	backend := memory.New(t.Context(), time.Minute)
	l := ratelimit.NewLimiter(backend, 1, time.Minute)

	allow(t, l, "key")
	deny(t, l, "key")

	if err := l.Reset(t.Context(), "key"); err != nil {
		t.Fatalf("reset error: %v", err)
	}

	allow(t, l, "key")
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

	deny(t, l, "key")
}

func TestCleanupStopsOnCancel(t *testing.T) {
	ctx := t.Context()
	backend := memory.New(ctx, 10*time.Millisecond)
	l := ratelimit.NewLimiter(backend, 1, 10*time.Millisecond)
	l.Allow(ctx, "key")
	time.Sleep(30 * time.Millisecond)
}
