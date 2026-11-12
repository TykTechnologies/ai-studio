package dbstate

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeServer answers probes with what the test last set.
type fakeServer struct {
	mu    sync.Mutex
	probe Probe
	err   error
	calls int
}

func (f *fakeServer) set(p Probe, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.probe, f.err = p, err
}

func (f *fakeServer) Probe(context.Context) (Probe, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return f.probe, f.err
}

func (f *fakeServer) probes() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func waitState(t *testing.T, tr *Tracker, want State) Status {
	t.Helper()
	require.Eventually(t, func() bool { return tr.Status().State == want }, 5*time.Second, 5*time.Millisecond, "state %s", want)
	return tr.Status()
}

// A nil Tracker (SQLite) is a writable database that never retires a
// connection.
func TestNilTrackerIsWritable(t *testing.T) {
	var tr *Tracker
	assert.Equal(t, Writable, tr.Status().State)
	assert.Zero(t, tr.Generation())
	tr.ReportReadOnly()
	tr.Stop()
	assert.Nil(t, For(nil))
}

// The three states, the generation that retires connections while the
// server is not writable, and what counts as a failover.
func TestTrackerStatesAndFailovers(t *testing.T) {
	srv := &fakeServer{probe: Probe{Server: "pg-a:5432", Timeline: 1}}
	tr := Start(srv.Probe, Options{Interval: 10 * time.Millisecond})
	defer tr.Stop()

	st := tr.Status()
	assert.Equal(t, Writable, st.State, "the first probe has run when Start returns")
	assert.Zero(t, st.Failovers)
	assert.Zero(t, tr.Generation(), "a writable server retires nothing")

	// The primary is demoted: connections to it are retired at every
	// probe until a writable server answers.
	srv.set(Probe{ReadOnly: true, Server: "pg-a:5432", Timeline: 1}, nil)
	st = waitState(t, tr, ReadOnly)
	assert.False(t, st.Since.IsZero())
	gen := tr.Generation()
	assert.NotZero(t, gen)
	require.Eventually(t, func() bool { return tr.Generation() > gen }, time.Second, 5*time.Millisecond, "retired again while still read-only")

	// The pool reaches the other node: one failover.
	srv.set(Probe{Server: "pg-b:5432", Timeline: 2}, nil)
	st = waitState(t, tr, Writable)
	assert.Equal(t, int64(1), st.Failovers)
	gen = tr.Generation()
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, gen, tr.Generation(), "writable again: nothing more is retired")

	// The database is away and comes back as it was (a restart): not a
	// failover.
	srv.set(Probe{}, errors.New("connection refused"))
	waitState(t, tr, Unreachable)
	srv.set(Probe{Server: "pg-b:5432", Timeline: 2}, nil)
	st = waitState(t, tr, Writable)
	assert.Equal(t, int64(1), st.Failovers)

	// Behind one address (a virtual IP) a promotion shows as a new
	// timeline after an outage.
	srv.set(Probe{}, errors.New("connection refused"))
	waitState(t, tr, Unreachable)
	srv.set(Probe{Server: "pg-b:5432", Timeline: 3}, nil)
	st = waitState(t, tr, Writable)
	assert.Equal(t, int64(2), st.Failovers)

	// The timeline is read from the control file, which a promoted node
	// updates a moment after it starts taking writes: a change with no
	// outage in between is not counted again.
	srv.set(Probe{Server: "pg-b:5432", Timeline: 4}, nil)
	before := srv.probes()
	require.Eventually(t, func() bool { return srv.probes() > before+1 }, time.Second, 5*time.Millisecond)
	assert.Equal(t, int64(2), tr.Status().Failovers)
}

// A write refused with SQLSTATE 25006 retires the connections at once and
// brings the probe forward, rather than waiting for the period; further
// refusals while the database already counts as read-only leave that to
// the probes.
func TestReportReadOnly(t *testing.T) {
	srv := &fakeServer{probe: Probe{Server: "pg-a:5432"}}
	tr := Start(srv.Probe, Options{Interval: time.Hour})
	defer tr.Stop()
	require.Equal(t, 1, srv.probes())

	srv.set(Probe{ReadOnly: true, Server: "pg-a:5432"}, nil)
	tr.ReportReadOnly()
	assert.Equal(t, ReadOnly, tr.Status().State, "at once")
	assert.Equal(t, uint64(1), tr.Generation())
	require.Eventually(t, func() bool { return srv.probes() == 2 }, time.Second, 5*time.Millisecond, "the probe is brought forward")
	require.Eventually(t, func() bool { return tr.Generation() == 2 }, time.Second, 5*time.Millisecond)

	for i := 0; i < 20; i++ {
		tr.ReportReadOnly()
	}
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, uint64(2), tr.Generation(), "a server that stays read-only is not dialled again for every statement")
	assert.Equal(t, 2, srv.probes())
}

// NotWritableSince is set when the database stops taking writes (by a
// probe or a refused write), kept while it moves between read-only and
// unreachable, and cleared when it is writable again.
func TestNotWritableSince(t *testing.T) {
	srv := &fakeServer{probe: Probe{Server: "pg-a:5432"}}
	tr := Start(srv.Probe, Options{Interval: 10 * time.Millisecond})
	defer tr.Stop()
	assert.True(t, tr.Status().NotWritableSince.IsZero(), "writable")

	before := time.Now()
	srv.set(Probe{ReadOnly: true, Server: "pg-a:5432"}, nil)
	tr.ReportReadOnly()
	st := tr.Status()
	require.Equal(t, ReadOnly, st.State)
	since := st.NotWritableSince
	assert.False(t, since.Before(before), "set by the refused write")
	assert.Equal(t, st.Since, since)

	srv.set(Probe{}, errors.New("connection refused"))
	st = waitState(t, tr, Unreachable)
	assert.Equal(t, since, st.NotWritableSince, "kept from read-only to unreachable")
	assert.True(t, st.Since.After(since), "Since follows the state")
	srv.set(Probe{ReadOnly: true, Server: "pg-b:5432"}, nil)
	assert.Equal(t, since, waitState(t, tr, ReadOnly).NotWritableSince, "and back")

	srv.set(Probe{Server: "pg-b:5432"}, nil)
	assert.True(t, waitState(t, tr, Writable).NotWritableSince.IsZero(), "cleared when writable")

	before = time.Now()
	srv.set(Probe{}, errors.New("connection refused"))
	assert.False(t, waitState(t, tr, Unreachable).NotWritableSince.Before(before), "set by a probe")
}

func TestIsReadOnlyError(t *testing.T) {
	assert.True(t, IsReadOnlyError(&pgconn.PgError{Code: "25006"}))
	assert.True(t, IsReadOnlyError(errors.Join(errors.New("create app"), &pgconn.PgError{Code: "25006"})))
	assert.False(t, IsReadOnlyError(&pgconn.PgError{Code: "23505"}))
	assert.False(t, IsReadOnlyError(errors.New("read-only")))
	assert.False(t, IsReadOnlyError(nil))
}

// The tracker ends, and is forgotten, when its pool is closed.
func TestTrackerEndsWithItsPool(t *testing.T) {
	srv := &fakeServer{probe: Probe{Server: "pg-a:5432"}}
	tr := Start(srv.Probe, Options{Interval: 5 * time.Millisecond})
	pool := &sql.DB{}
	Register(pool, tr)
	_, ok := trackers.Load(pool)
	require.True(t, ok)

	srv.set(Probe{}, errors.New("sql: database is closed"))
	require.Eventually(t, func() bool { _, ok := trackers.Load(pool); return !ok }, time.Second, 5*time.Millisecond)
	tr.Stop() // returns: the loop has ended
	assert.Equal(t, Writable, tr.Status().State, "a closed pool is not an unreachable database")
}

// A probe may have to dial, and with several hosts the first may not answer
// at all: the probe is given the time its caller says that takes, so a slow
// answer is still an answer, and only a probe that outlasts its bound makes
// the database unreachable.
func TestProbeTimeout(t *testing.T) {
	var deadlines []time.Duration
	var mu sync.Mutex
	delay := 150 * time.Millisecond
	slow := func(ctx context.Context) (Probe, error) {
		if d, ok := ctx.Deadline(); ok {
			mu.Lock()
			deadlines = append(deadlines, time.Until(d))
			mu.Unlock()
		}
		select {
		case <-time.After(delay):
			return Probe{Server: "pg-b:5432"}, nil
		case <-ctx.Done():
			return Probe{}, ctx.Err()
		}
	}

	// The first host costs 150 ms before the second answers: inside the
	// bound, so the database is writable.
	tr := Start(slow, Options{Interval: time.Hour, ProbeTimeout: 2 * time.Second})
	assert.Equal(t, Writable, tr.Status().State)
	tr.Stop()
	mu.Lock()
	require.Len(t, deadlines, 1)
	assert.Greater(t, deadlines[0], time.Second, "the probe gets the configured bound")
	mu.Unlock()

	// A bound shorter than the answer takes: unreachable.
	tr = Start(slow, Options{Interval: time.Hour, ProbeTimeout: 20 * time.Millisecond})
	assert.Equal(t, Unreachable, tr.Status().State)
	tr.Stop()

	// Unset: the default, which is long enough to dial.
	deadlines = nil
	tr = Start(slow, Options{Interval: time.Hour})
	tr.Stop()
	mu.Lock()
	require.Len(t, deadlines, 1)
	assert.Greater(t, deadlines[0], DefaultProbeTimeout-time.Second)
	mu.Unlock()
}
