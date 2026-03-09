package memory

import (
	"context"
	"sync"
	"time"
)

type entry struct {
	mu         sync.Mutex
	timestamps []time.Time
}

func (e *entry) prune(cutoff time.Time) {
	valid := e.timestamps[:0]
	for _, ts := range e.timestamps {
		if ts.After(cutoff) {
			valid = append(valid, ts)
		}
	}
	e.timestamps = valid
}

// Backend is a thread-safe, in-memory sliding window storage backend.
type Backend struct {
	mu      sync.Mutex
	entries map[string]*entry
}

// New creates an in-memory backend with a background goroutine
// that periodically evicts empty entries. The goroutine stops when ctx is cancelled.
func New(ctx context.Context, cleanupInterval time.Duration) *Backend {
	b := &Backend{
		entries: make(map[string]*entry),
	}
	go b.cleanup(ctx, cleanupInterval)
	return b
}

func (b *Backend) getOrCreate(key string) *entry {
	b.mu.Lock()
	e, ok := b.entries[key]
	if !ok {
		e = &entry{}
		b.entries[key] = e
	}
	b.mu.Unlock()
	return e
}

func (b *Backend) Record(_ context.Context, key string, window time.Duration) (int, error) {
	now := time.Now()
	e := b.getOrCreate(key)

	e.mu.Lock()
	defer e.mu.Unlock()

	e.prune(now.Add(-window))
	e.timestamps = append(e.timestamps, now)
	return len(e.timestamps), nil
}

func (b *Backend) Count(_ context.Context, key string, window time.Duration) (int, error) {
	now := time.Now()
	e := b.getOrCreate(key)

	e.mu.Lock()
	defer e.mu.Unlock()

	e.prune(now.Add(-window))
	return len(e.timestamps), nil
}

func (b *Backend) Reset(_ context.Context, key string) error {
	b.mu.Lock()
	delete(b.entries, key)
	b.mu.Unlock()
	return nil
}

func (b *Backend) Oldest(_ context.Context, key string, window time.Duration) (time.Time, error) {
	now := time.Now()
	e := b.getOrCreate(key)

	e.mu.Lock()
	defer e.mu.Unlock()

	e.prune(now.Add(-window))
	if len(e.timestamps) == 0 {
		return time.Time{}, nil
	}
	return e.timestamps[0], nil
}

func (b *Backend) cleanup(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			b.removeStale()
		}
	}
}

func (b *Backend) removeStale() {
	b.mu.Lock()
	defer b.mu.Unlock()

	for key, e := range b.entries {
		e.mu.Lock()
		if len(e.timestamps) == 0 {
			delete(b.entries, key)
		}
		e.mu.Unlock()
	}
}
