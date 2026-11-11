// Package dbstate tracks whether the PostgreSQL server a Studio replica is
// connected to takes writes.
//
// With a replicated database the node behind a connection can stop being
// the primary while the connection stays open: it was demoted, and every
// write on it fails with SQLSTATE 25006 until the connection is closed and
// dialled again (to the new primary, with a multi-host DATABASE_URL and
// target_session_attrs=read-write). A Tracker probes the server on a
// period and is told about 25006 as it happens; when the server is not
// writable it raises a generation number, and the pool discards every
// connection dialled under an older generation the next time it would be
// reused (pkg/studio wires that to the driver).
//
// The Tracker's Status is what readiness probes and the write-hold wrapper
// read. SQLite has no tracker: For returns nil, and a nil Tracker reports a
// writable database.
package dbstate

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/pkg/safe"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// State is what the database can do for this replica.
type State string

const (
	// Writable: the server answers and takes writes.
	Writable State = "writable"
	// ReadOnly: the server answers but refuses writes (a hot standby, or a
	// primary that was demoted).
	ReadOnly State = "read_only"
	// Unreachable: no server answers.
	Unreachable State = "unreachable"
)

// ReadOnlySQLState is the SQLSTATE of a write refused by a server that is
// read-only (read_only_sql_transaction).
const ReadOnlySQLState = "25006"

// DefaultCheckInterval is how often a Tracker probes the server.
const DefaultCheckInterval = 5 * time.Second

// probeTimeout bounds one probe, connection included.
const probeTimeout = 3 * time.Second

// Status is a Tracker's view of the database.
type Status struct {
	State State
	// Since is when State last changed.
	Since time.Time
	// Failovers counts the times the writable server changed (another
	// node, or the same address on a new timeline) since the tracker
	// started.
	Failovers int64
}

// Probe is one look at the server.
type Probe struct {
	// ReadOnly: the server is in recovery, or its sessions are read-only.
	ReadOnly bool
	// Server identifies the node (address and port), and Timeline its
	// timeline when it can be read (0 otherwise); a promotion starts a
	// new one.
	Server   string
	Timeline int64
}

// ProbeFunc looks at the server through the pool.
type ProbeFunc func(ctx context.Context) (Probe, error)

// Tracker follows one database.
type Tracker struct {
	probe    ProbeFunc
	interval time.Duration

	generation atomic.Uint64

	mu        sync.Mutex
	state     State
	since     time.Time
	failovers int64
	server    string
	timeline  int64
	// wasDown: the server was not writable since the last writable probe.
	wasDown bool

	// pool is set by Register, to forget the tracker when the pool closes.
	pool atomic.Pointer[sql.DB]

	kick     chan struct{}
	stop     chan struct{}
	stopOnce sync.Once
	done     chan struct{}
}

// Start returns a Tracker that probes every interval (DefaultCheckInterval
// when it is not positive) until Stop, or until the probe reports that the
// database was closed. The first probe has run when it returns.
func Start(probe ProbeFunc, interval time.Duration) *Tracker {
	if interval <= 0 {
		interval = DefaultCheckInterval
	}
	t := &Tracker{
		probe: probe, interval: interval,
		state: Writable, since: time.Now(),
		kick: make(chan struct{}, 1), stop: make(chan struct{}), done: make(chan struct{}),
	}
	t.check()
	go func() {
		defer close(t.done)
		safe.Loop("database state check", t.stop, t.loop)
	}()
	return t
}

// Stop ends the probes.
func (t *Tracker) Stop() {
	if t == nil {
		return
	}
	t.stopOnce.Do(func() { close(t.stop) })
	<-t.done
}

// Status reports the database as last seen. A nil Tracker (SQLite, or a
// database opened without one) is writable.
func (t *Tracker) Status() Status {
	if t == nil {
		return Status{State: Writable}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return Status{State: t.state, Since: t.since, Failovers: t.failovers}
}

// Generation is the number connections are stamped with when they are
// dialled; one stamped with a lower number than the current one must not be
// reused. A nil Tracker's generation is always 0.
func (t *Tracker) Generation() uint64 {
	if t == nil {
		return 0
	}
	return t.generation.Load()
}

// ReportReadOnly tells the tracker that a statement was just refused with
// SQLSTATE 25006. The first report while the database counted as writable
// retires the pool's connections at once and brings the next probe
// forward; while it already counts as not writable the probes do that, once
// per interval, so a server that stays read-only is not dialled again for
// every statement.
func (t *Tracker) ReportReadOnly() {
	if t == nil {
		return
	}
	t.mu.Lock()
	first := t.state == Writable
	if first {
		t.setStateLocked(ReadOnly)
		t.generation.Add(1)
	}
	t.mu.Unlock()
	if first {
		logger.Warn("The database refused a write as read-only (SQLSTATE 25006): its connections are closed and dialled again")
		select {
		case t.kick <- struct{}{}:
		default:
		}
	}
}

// IsReadOnlyError reports whether err is a write refused by a read-only
// server (SQLSTATE 25006).
func IsReadOnlyError(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == ReadOnlySQLState
}

func (t *Tracker) loop() {
	ticker := time.NewTicker(t.interval)
	defer ticker.Stop()
	for {
		select {
		case <-t.stop:
			return
		case <-ticker.C:
		case <-t.kick:
		}
		if closed := t.check(); closed {
			if pool := t.pool.Load(); pool != nil {
				Unregister(pool)
			}
			t.stopOnce.Do(func() { close(t.stop) })
			return
		}
	}
}

// check probes once and reports whether the database was closed.
func (t *Tracker) check() (closed bool) {
	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	p, err := t.probe(ctx)
	cancel()
	if err != nil && (errors.Is(err, sql.ErrConnDone) || strings.Contains(err.Error(), "sql: database is closed")) {
		return true
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	switch {
	case err != nil:
		if t.state != Unreachable {
			logger.Warnf("The database is unreachable: %v", err)
		}
		t.setStateLocked(Unreachable)
		t.wasDown = true
	case p.ReadOnly:
		if t.state != ReadOnly {
			logger.Warnf("The database server %s is read-only (a standby, or a demoted primary): its connections are closed and dialled again until a writable server answers", p.Server)
		}
		t.setStateLocked(ReadOnly)
		t.wasDown = true
		// Every connection dialled so far is to a server that does not
		// take writes: none is reused.
		t.generation.Add(1)
	default:
		moved := t.server != "" && p.Server != t.server
		promoted := t.server != "" && !moved && t.wasDown && p.Timeline != 0 && t.timeline != 0 && p.Timeline != t.timeline
		if moved || promoted {
			t.failovers++
			logger.Warnf("The database failed over: the writable server is now %s (was %s)", p.Server, t.server)
		} else if t.state != Writable {
			logger.Infof("The database is writable again (%s)", p.Server)
		}
		t.server, t.timeline, t.wasDown = p.Server, p.Timeline, false
		t.setStateLocked(Writable)
	}
	return false
}

func (t *Tracker) setStateLocked(s State) {
	if t.state != s {
		t.state, t.since = s, time.Now()
	}
}

// trackers maps a pool to its tracker, so code that has only the *gorm.DB
// (the readiness handler, the cluster status) finds it.
var trackers sync.Map // *sql.DB -> *Tracker

// Register makes For return t for databases on pool.
func Register(pool *sql.DB, t *Tracker) {
	t.pool.Store(pool)
	trackers.Store(pool, t)
}

// Unregister forgets pool's tracker.
func Unregister(pool *sql.DB) {
	trackers.Delete(pool)
}

// For returns the tracker of db's pool, or nil when it has none.
func For(db *gorm.DB) *Tracker {
	if db == nil {
		return nil
	}
	pool, err := db.DB()
	if err != nil {
		return nil
	}
	if t, ok := trackers.Load(pool); ok {
		return t.(*Tracker)
	}
	return nil
}
