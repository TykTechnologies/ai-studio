package ratelimit

import (
	"context"
	"errors"
	"testing"
	"time"
)

// mockBackend is a controllable Backend for unit testing the Limiter.
type mockBackend struct {
	recordCount int
	recordErr   error
	oldestTime  time.Time
	oldestErr   error
	resetErr    error
	countResult int
	countErr    error
}

func (m *mockBackend) Record(_ context.Context, _ string, _ time.Duration) (int, error) {
	return m.recordCount, m.recordErr
}

func (m *mockBackend) Count(_ context.Context, _ string, _ time.Duration) (int, error) {
	return m.countResult, m.countErr
}

func (m *mockBackend) Reset(_ context.Context, _ string) error {
	return m.resetErr
}

func (m *mockBackend) Oldest(_ context.Context, _ string, _ time.Duration) (time.Time, error) {
	return m.oldestTime, m.oldestErr
}

func TestNewLimiter(t *testing.T) {
	b := &mockBackend{}
	l := NewLimiter(b, 5, time.Minute)
	if l.limit != 5 {
		t.Fatalf("expected limit 5, got %d", l.limit)
	}
	if l.window != time.Minute {
		t.Fatalf("expected window 1m, got %v", l.window)
	}
	if l.prefix == "" {
		t.Fatal("expected non-empty prefix")
	}
}

func TestNewLimiter_UniquePrefix(t *testing.T) {
	b := &mockBackend{}
	l1 := NewLimiter(b, 1, time.Second)
	l2 := NewLimiter(b, 1, time.Second)
	if l1.prefix == l2.prefix {
		t.Fatalf("prefixes should be unique: %q == %q", l1.prefix, l2.prefix)
	}
}

func TestAllow_Success(t *testing.T) {
	b := &mockBackend{recordCount: 1}
	l := NewLimiter(b, 3, time.Minute)

	r, err := l.Allow(t.Context(), "key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !r.Allowed {
		t.Fatal("should be allowed")
	}
}

func TestAllow_RecordError(t *testing.T) {
	b := &mockBackend{recordErr: errors.New("backend down")}
	l := NewLimiter(b, 3, time.Minute)

	_, err := l.Allow(t.Context(), "key")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, b.recordErr) {
		t.Fatalf("expected wrapped backend error, got: %v", err)
	}
}

func TestAllow_Denied_OldestError(t *testing.T) {
	b := &mockBackend{
		recordCount: 4,
		oldestErr:   errors.New("oldest failed"),
	}
	l := NewLimiter(b, 3, time.Minute)

	r, err := l.Allow(t.Context(), "key")
	if err == nil {
		t.Fatal("expected error from Oldest")
	}
	if r.Allowed {
		t.Fatal("should be denied")
	}
	if r.RetryAfter != time.Minute {
		t.Fatalf("expected fallback retryAfter of window, got %v", r.RetryAfter)
	}
}

func TestAllow_Denied_OldestZero(t *testing.T) {
	b := &mockBackend{
		recordCount: 4,
		oldestTime:  time.Time{},
	}
	l := NewLimiter(b, 3, time.Minute)

	r, err := l.Allow(t.Context(), "key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Allowed {
		t.Fatal("should be denied")
	}
	if r.RetryAfter != time.Minute {
		t.Fatalf("expected retryAfter == window when oldest is zero, got %v", r.RetryAfter)
	}
}

func TestAllow_Denied_OldestNonZero(t *testing.T) {
	oldest := time.Now().Add(-30 * time.Second)
	b := &mockBackend{
		recordCount: 4,
		oldestTime:  oldest,
	}
	l := NewLimiter(b, 3, time.Minute)

	r, err := l.Allow(t.Context(), "key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Allowed {
		t.Fatal("should be denied")
	}
	// oldest was 30s ago, window is 60s, so retry after ~30s
	if r.RetryAfter < 25*time.Second || r.RetryAfter > 35*time.Second {
		t.Fatalf("expected retryAfter ~30s, got %v", r.RetryAfter)
	}
}

func TestAllow_Denied_OldestFarPast(t *testing.T) {
	// oldest is beyond the window — retryAfter should be clamped to 0
	oldest := time.Now().Add(-2 * time.Minute)
	b := &mockBackend{
		recordCount: 4,
		oldestTime:  oldest,
	}
	l := NewLimiter(b, 3, time.Minute)

	r, err := l.Allow(t.Context(), "key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.RetryAfter != 0 {
		t.Fatalf("expected retryAfter 0 for expired oldest, got %v", r.RetryAfter)
	}
}

func TestReset_Success(t *testing.T) {
	b := &mockBackend{}
	l := NewLimiter(b, 3, time.Minute)

	if err := l.Reset(t.Context(), "key"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReset_Error(t *testing.T) {
	b := &mockBackend{resetErr: errors.New("reset failed")}
	l := NewLimiter(b, 3, time.Minute)

	err := l.Reset(t.Context(), "key")
	if err == nil {
		t.Fatal("expected error")
	}
}
