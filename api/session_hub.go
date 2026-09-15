package api

import (
	"log/slog"
	"os"
	"sync"
	"time"
)

// HubSession is what the SessionHub manages: chat-room sessions and agent
// sessions both satisfy it.
type HubSession interface {
	ID() string
	Stop()
}

// SessionHub keeps live sessions in memory across HTTP requests.
//
// A session is kept alive while any caller holds a reference (a v1 SSE
// connection, a v2 run, a tool/datasource/upload request) and for an idle TTL
// after the last reference is released; a reaper then stops it. "Stop" is the
// only way a session leaves the hub apart from Remove, so a session is never
// stopped while someone is using it, and the same id never maps to two live
// session objects (which the previous ChatHub allowed on reconnect).
type SessionHub struct {
	mu      sync.Mutex
	entries map[string]*hubEntry
	ttl     time.Duration
	now     func() time.Time
	reaper  sync.Once
}

type hubEntry struct {
	session  HubSession
	refs     int
	lastUsed time.Time
	loading  chan struct{} // closed once session is set (or loadErr is set)
	loadErr  error
}

// NewSessionHub creates a hub whose idle sessions are stopped after ttl.
func NewSessionHub(ttl time.Duration) *SessionHub {
	return &SessionHub{entries: map[string]*hubEntry{}, ttl: ttl, now: time.Now}
}

// Acquire returns the live session for id, loading it with load when the hub
// has none. Concurrent callers for the same id share one load. The returned
// release must be called exactly once when the caller no longer needs the
// session; the hub keeps it alive until then and for ttl afterwards.
func (h *SessionHub) Acquire(id string, load func() (HubSession, error)) (HubSession, func(), error) {
	h.mu.Lock()
	e, ok := h.entries[id]
	if ok {
		e.refs++
		h.mu.Unlock()
		<-e.loading
		if e.loadErr != nil {
			h.release(e, id)
			return nil, nil, e.loadErr
		}
		return e.session, h.releaseFunc(e, id), nil
	}

	e = &hubEntry{refs: 1, lastUsed: h.now(), loading: make(chan struct{})}
	h.entries[id] = e
	h.mu.Unlock()

	s, err := load()
	h.mu.Lock()
	if err != nil {
		e.loadErr = err
		delete(h.entries, id)
		h.mu.Unlock()
		close(e.loading)
		return nil, nil, err
	}
	e.session = s
	h.mu.Unlock()
	close(e.loading)
	return s, h.releaseFunc(e, id), nil
}

// Add registers an already started session and takes one reference on it. If
// a session with the same id is already present the existing one wins and the
// given session is stopped.
func (h *SessionHub) Add(s HubSession) (HubSession, func()) {
	id := s.ID()
	h.mu.Lock()
	if e, ok := h.entries[id]; ok {
		e.refs++
		h.mu.Unlock()
		<-e.loading
		if e.loadErr == nil && e.session != nil {
			if e.session != s {
				s.Stop()
			}
			return e.session, h.releaseFunc(e, id)
		}
		h.release(e, id)
	}
	e := &hubEntry{session: s, refs: 1, lastUsed: h.now(), loading: make(chan struct{})}
	close(e.loading)
	h.entries[id] = e
	h.mu.Unlock()
	return s, h.releaseFunc(e, id)
}

// Get returns the live session for id without taking a reference. Use it only
// for read-only peeks; anything that outlives the current statement should
// Acquire.
func (h *SessionHub) Get(id string) (HubSession, bool) {
	h.mu.Lock()
	e, ok := h.entries[id]
	if ok {
		e.lastUsed = h.now()
	}
	h.mu.Unlock()
	if !ok {
		return nil, false
	}
	<-e.loading
	if e.loadErr != nil || e.session == nil {
		return nil, false
	}
	return e.session, true
}

// Remove stops a session and drops it from the hub regardless of references.
// Use it for explicit session termination only.
func (h *SessionHub) Remove(id string) {
	h.mu.Lock()
	e, ok := h.entries[id]
	if ok {
		delete(h.entries, id)
	}
	h.mu.Unlock()
	if ok {
		<-e.loading
		if e.session != nil {
			e.session.Stop()
		}
	}
}

// Len reports the number of live sessions.
func (h *SessionHub) Len() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.entries)
}

// Refs reports the reference count of a session (0 when absent). Test helper.
func (h *SessionHub) Refs(id string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	if e, ok := h.entries[id]; ok {
		return e.refs
	}
	return 0
}

func (h *SessionHub) releaseFunc(e *hubEntry, id string) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			h.mu.Lock()
			h.release(e, id)
			h.mu.Unlock()
		})
	}
}

// release decrements the reference count; the caller holds h.mu.
func (h *SessionHub) release(e *hubEntry, id string) {
	e.refs--
	if e.refs < 0 {
		e.refs = 0
	}
	e.lastUsed = h.now()
}

// Reap stops and removes sessions that have had no references for longer
// than the TTL. It returns how many were stopped.
func (h *SessionHub) Reap() int {
	now := h.now()
	var victims []HubSession
	h.mu.Lock()
	for id, e := range h.entries {
		if e.refs > 0 || e.session == nil {
			continue
		}
		if now.Sub(e.lastUsed) >= h.ttl {
			victims = append(victims, e.session)
			delete(h.entries, id)
		}
	}
	h.mu.Unlock()
	for _, s := range victims {
		slog.Info("stopping idle chat session", "session_id", s.ID())
		s.Stop()
	}
	return len(victims)
}

// StartReaper runs Reap periodically for the life of the process. Safe to call
// more than once.
func (h *SessionHub) StartReaper(interval time.Duration) {
	h.reaper.Do(func() {
		go func() {
			t := time.NewTicker(interval)
			defer t.Stop()
			for range t.C {
				h.Reap()
			}
		}()
	})
}

// sessionIdleTTL reads CHAT_SESSION_IDLE_TTL (a Go duration, default 10m). It
// is short on purpose: NATS and Postgres queues hold a connection per session.
func sessionIdleTTL() time.Duration {
	if v := os.Getenv("CHAT_SESSION_IDLE_TTL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
		slog.Warn("invalid CHAT_SESSION_IDLE_TTL, using default", "value", v)
	}
	return 10 * time.Minute
}
