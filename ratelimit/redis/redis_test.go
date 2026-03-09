package redis_test

import (
	"os"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/TykTechnologies/midsommar/v2/ratelimit"
	rlredis "github.com/TykTechnologies/midsommar/v2/ratelimit/redis"
)

func setup(t *testing.T) *ratelimit.Limiter {
	t.Helper()
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		t.Skip("REDIS_URL not set, skipping Redis backend tests")
	}

	opts, err := goredis.ParseURL(redisURL)
	if err != nil {
		t.Fatalf("invalid REDIS_URL: %v", err)
	}
	client := goredis.NewClient(opts)
	t.Cleanup(func() { client.Close() })

	ctx := t.Context()
	if err := client.Ping(ctx).Err(); err != nil {
		t.Fatalf("redis ping failed: %v", err)
	}

	backend := rlredis.New(client, "test:ratelimit:")
	return ratelimit.NewLimiter(backend, 3, time.Minute)
}

func TestAllow(t *testing.T) {
	l := setup(t)
	ctx := t.Context()

	// Reset to ensure clean state
	l.Reset(ctx, "test-allow")

	for range 3 {
		r, err := l.Allow(ctx, "test-allow")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !r.Allowed {
			t.Fatal("should be allowed")
		}
	}

	r, err := l.Allow(ctx, "test-allow")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Allowed {
		t.Fatal("4th request should be denied")
	}
	if r.RetryAfter <= 0 {
		t.Fatal("retryAfter should be positive")
	}

	// Cleanup
	l.Reset(ctx, "test-allow")
}

func TestReset(t *testing.T) {
	l := setup(t)
	ctx := t.Context()

	l.Reset(ctx, "test-reset")

	for range 3 {
		l.Allow(ctx, "test-reset")
	}

	r, _ := l.Allow(ctx, "test-reset")
	if r.Allowed {
		t.Fatal("should be denied")
	}

	if err := l.Reset(ctx, "test-reset"); err != nil {
		t.Fatalf("reset error: %v", err)
	}

	r, err := l.Allow(ctx, "test-reset")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !r.Allowed {
		t.Fatal("should be allowed after reset")
	}

	l.Reset(ctx, "test-reset")
}
