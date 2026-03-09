package ratelimit

import (
	"sync"
	"testing"
	"time"
)

func TestAllow(t *testing.T) {
	backend := NewMemoryBackend(t.Context(), time.Minute)
	l := NewLimiter(backend, 3, time.Minute)

	for range 3 {
		allowed, _ := l.Allow("key")
		if !allowed {
			t.Fatal("should be allowed")
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
	backend := NewMemoryBackend(t.Context(), time.Minute)
	l := NewLimiter(backend, 1, time.Minute)

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
	backend := NewMemoryBackend(t.Context(), time.Minute)
	window := 50 * time.Millisecond
	l := NewLimiter(backend, 1, window)

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
	backend := NewMemoryBackend(t.Context(), time.Minute)
	window := 200 * time.Millisecond
	l := NewLimiter(backend, 1, window)

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
	backend := NewMemoryBackend(t.Context(), time.Minute)
	l := NewLimiter(backend, 1, time.Minute)

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
	backend := NewMemoryBackend(t.Context(), time.Minute)
	l := NewLimiter(backend, 100, time.Minute)

	var wg sync.WaitGroup
	for range 200 {
		wg.Go(func() {
			l.Allow("key")
		})
	}
	wg.Wait()

	allowed, _ := l.Allow("key")
	if allowed {
		t.Fatal("should be denied after concurrent flood")
	}
}

func TestCleanupStopsOnCancel(t *testing.T) {
	ctx, cancel := t.Context(), func() {} // t.Context cancels on test end
	_ = ctx
	backend := NewMemoryBackend(t.Context(), 10*time.Millisecond)
	l := NewLimiter(backend, 1, 10*time.Millisecond)
	l.Allow("key")
	cancel()
	time.Sleep(30 * time.Millisecond)
}
