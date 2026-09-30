package cluster

import (
	"context"
	"net"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stopsWithin fails the test if stop does not return within d. A Stop that
// blocks forever would otherwise hang the whole test binary, the way it hung
// Studio's startup cleanup.
func stopsWithin(t *testing.T, d time.Duration, stop func()) {
	t.Helper()
	done := make(chan struct{})
	go func() { stop(); close(done) }()
	select {
	case <-done:
	case <-time.After(d):
		t.Fatalf("Stop did not return within %s", d)
	}
}

// Stop before Start (a caller whose startup failed earlier) returns at once,
// and a Start after it does nothing.
func TestLogStopWithoutStart(t *testing.T) {
	l := NewLog(sqliteDB(t), "solo", LogOptions{})
	stopsWithin(t, 2*time.Second, l.Stop)
	assert.ErrorIs(t, l.Start(context.Background()), errStoppedBeforeStart)
}

func TestLogStopWithoutStart_Postgres(t *testing.T) {
	c := newTestCluster(t)
	l := NewLog(c.replicaDB("a"), "a", LogOptions{})
	stopsWithin(t, 2*time.Second, l.Stop)
	assert.ErrorIs(t, l.Start(context.Background()), errStoppedBeforeStart)
}

// A Start that fails (here: its context has ended, so the first query
// fails) returns the error, and Stop still returns at once.
func TestLogStopAfterFailedStart_Postgres(t *testing.T) {
	c := newTestCluster(t)
	l := NewLog(c.replicaDB("a"), "a", LogOptions{ListenerDSN: c.dsn("a")})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.Error(t, l.Start(ctx))
	stopsWithin(t, 2*time.Second, l.Stop)
}

// A listener that cannot connect does not fail the log: it reads by polling
// (events still arrive) and Stop returns at once.
func TestLogWithoutListenerFallsBackToPolling_Postgres(t *testing.T) {
	c := newTestCluster(t)
	a := c.startReplica("a", LogOptions{})

	// b's listener points at a port nothing listens on.
	u, err := url.Parse(c.dsn("b"))
	require.NoError(t, err)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	u.Host = ln.Addr().String()
	require.NoError(t, ln.Close())

	r := &replica{name: "b", db: c.replicaDB("b"), counts: map[int64]int{}}
	r.log = NewLog(r.db, "b", LogOptions{PollInterval: 200 * time.Millisecond, ListenerDSN: u.String()})
	r.log.Subscribe("*", func(ev Event) {
		r.mu.Lock()
		r.got = append(r.got, ev)
		r.counts[ev.ID]++
		r.mu.Unlock()
	})
	start := time.Now()
	require.NoError(t, r.log.Start(context.Background()))
	assert.Less(t, time.Since(start), 10*time.Second)
	assert.False(t, r.log.Stats().Listening)

	publish(t, a, "polled", "x")
	got := r.waitFor(t, 1, 5*time.Second)
	assert.Equal(t, "polled", got[0].Topic)
	stopsWithin(t, 2*time.Second, r.log.Stop)
}

// The listener takes the pool's DSN as it is: no sslmode (pgx's default,
// prefer, against a server without TLS) used to fail lib/pq, and the log's
// Start with it.
func TestLogListensWithDSNWithoutSSLMode_Postgres(t *testing.T) {
	c := newTestCluster(t)
	u, err := url.Parse(c.dsn("b"))
	require.NoError(t, err)
	q := u.Query()
	q.Del("sslmode")
	q.Set("default_query_exec_mode", "simple_protocol")
	u.RawQuery = q.Encode()

	a := c.startReplica("a", LogOptions{})
	r := &replica{name: "b", db: c.replicaDB("b"), counts: map[int64]int{}}
	// A poll interval longer than the test: only a notification delivers.
	r.log = NewLog(r.db, "b", LogOptions{PollInterval: time.Minute, ListenerDSN: u.String()})
	r.log.Subscribe("*", func(ev Event) {
		r.mu.Lock()
		r.got = append(r.got, ev)
		r.mu.Unlock()
	})
	require.NoError(t, r.log.Start(context.Background()))
	defer r.log.Stop()
	require.True(t, r.log.Stats().Listening)

	publish(t, a, "notified", "x")
	r.waitFor(t, 1, 5*time.Second)
}
