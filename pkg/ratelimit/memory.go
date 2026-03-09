package ratelimit

import (
	"context"
	"sync"
	"time"
)

type entry struct {
	mu         sync.Mutex
	timestamps []time.Time
}

// MemoryBackend is a thread-safe, in-memory sliding window rate limit backend.
type MemoryBackend struct {
	mu      sync.Mutex
	entries map[string]*entry
}

// NewMemoryBackend creates an in-memory backend with a background goroutine
// that periodically evicts expired entries. The goroutine stops when ctx is cancelled.
func NewMemoryBackend(ctx context.Context, cleanupInterval time.Duration) *MemoryBackend {
	m := &MemoryBackend{
		entries: make(map[string]*entry),
	}
	go m.cleanup(ctx, cleanupInterval)
	return m
}

func (m *MemoryBackend) Allow(key string, limit int, window time.Duration) (bool, time.Duration) {
	now := time.Now()
	cutoff := now.Add(-window)

	m.mu.Lock()
	e, ok := m.entries[key]
	if !ok {
		e = &entry{}
		m.entries[key] = e
	}
	m.mu.Unlock()

	e.mu.Lock()
	defer e.mu.Unlock()

	valid := e.timestamps[:0]
	for _, ts := range e.timestamps {
		if ts.After(cutoff) {
			valid = append(valid, ts)
		}
	}
	e.timestamps = valid

	if len(e.timestamps) >= limit {
		retryAfter := max(e.timestamps[0].Add(window).Sub(now), 0)
		return false, retryAfter
	}

	e.timestamps = append(e.timestamps, now)
	return true, 0
}

func (m *MemoryBackend) Reset(key string) {
	m.mu.Lock()
	delete(m.entries, key)
	m.mu.Unlock()
}

func (m *MemoryBackend) cleanup(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.removeExpired()
		}
	}
}

func (m *MemoryBackend) removeExpired() {
	now := time.Now()

	m.mu.Lock()
	defer m.mu.Unlock()

	for key, e := range m.entries {
		e.mu.Lock()
		cutoff := now.Add(-time.Hour)
		valid := e.timestamps[:0]
		for _, ts := range e.timestamps {
			if ts.After(cutoff) {
				valid = append(valid, ts)
			}
		}
		e.timestamps = valid
		if len(e.timestamps) == 0 {
			delete(m.entries, key)
		}
		e.mu.Unlock()
	}
}
