package api

import (
	"strconv"
	"sync"
	"time"
)

// The test panel calls the router's embedding and judge LLMs, which are not
// charged to any App's budget, so test runs are limited per user.
const (
	semanticRouterTestLimit  = 30
	semanticRouterTestWindow = time.Minute
)

// windowLimiter allows limit events per key in each fixed window. It is per
// process, which is enough to stop a loop from one admin session.
type windowLimiter struct {
	mu     sync.Mutex
	limit  int
	window time.Duration
	now    func() time.Time
	counts map[string]windowCount
}

type windowCount struct {
	start time.Time
	n     int
}

func newWindowLimiter(limit int, window time.Duration) *windowLimiter {
	return &windowLimiter{limit: limit, window: window, now: time.Now, counts: map[string]windowCount{}}
}

// allow records an event for key and reports whether it is within the limit;
// when it is not, retryAfter is the time left in the window.
func (l *windowLimiter) allow(key string) (ok bool, retryAfter time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	c := l.counts[key]
	if now.Sub(c.start) >= l.window {
		c = windowCount{start: now}
		// Drop other stale windows so the map stays small.
		for k, v := range l.counts {
			if now.Sub(v.start) >= l.window {
				delete(l.counts, k)
			}
		}
	}
	if c.n >= l.limit {
		return false, c.start.Add(l.window).Sub(now)
	}
	c.n++
	l.counts[key] = c
	return true, 0
}

var semanticRouterTests = newWindowLimiter(semanticRouterTestLimit, semanticRouterTestWindow)

func retryAfterSeconds(d time.Duration) string {
	s := int(d.Seconds() + 0.999)
	if s < 1 {
		s = 1
	}
	return strconv.Itoa(s)
}
