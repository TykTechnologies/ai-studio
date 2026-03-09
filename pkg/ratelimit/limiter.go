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

type Limiter struct {
	limit   int
	window  time.Duration
	mu      sync.Mutex
	entries map[string]*entry
}

func NewLimiter(limit int, window time.Duration, cleanupCtx context.Context) *Limiter {
	l := &Limiter{
		limit:   limit,
		window:  window,
		entries: make(map[string]*entry),
	}
	go l.cleanup(cleanupCtx)
	return l
}

func (l *Limiter) Allow(key string) (bool, time.Duration) {
	now := time.Now()
	cutoff := now.Add(-l.window)

	l.mu.Lock()
	e, ok := l.entries[key]
	if !ok {
		e = &entry{}
		l.entries[key] = e
	}
	l.mu.Unlock()

	e.mu.Lock()
	defer e.mu.Unlock()

	// Remove expired timestamps
	valid := e.timestamps[:0]
	for _, ts := range e.timestamps {
		if ts.After(cutoff) {
			valid = append(valid, ts)
		}
	}
	e.timestamps = valid

	if len(e.timestamps) >= l.limit {
		retryAfter := e.timestamps[0].Add(l.window).Sub(now)
		if retryAfter < 0 {
			retryAfter = 0
		}
		return false, retryAfter
	}

	e.timestamps = append(e.timestamps, now)
	return true, 0
}

func (l *Limiter) Reset(key string) {
	l.mu.Lock()
	delete(l.entries, key)
	l.mu.Unlock()
}

func (l *Limiter) cleanup(ctx context.Context) {
	ticker := time.NewTicker(l.window)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			l.removeExpired()
		}
	}
}

func (l *Limiter) removeExpired() {
	now := time.Now()
	cutoff := now.Add(-l.window)

	l.mu.Lock()
	defer l.mu.Unlock()

	for key, e := range l.entries {
		e.mu.Lock()
		valid := e.timestamps[:0]
		for _, ts := range e.timestamps {
			if ts.After(cutoff) {
				valid = append(valid, ts)
			}
		}
		e.timestamps = valid
		if len(e.timestamps) == 0 {
			delete(l.entries, key)
		}
		e.mu.Unlock()
	}
}
