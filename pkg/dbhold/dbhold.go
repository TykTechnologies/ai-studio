// Package dbhold holds database statements across a Postgres failover.
//
// Pool wraps Studio's *sql.DB as gorm's connection pool. A statement issued
// outside a transaction that fails before it reached a writable server (the
// connection could not be made, the server refused it as read-only, or the
// driver reports that nothing was sent) is tried again until the hold
// window has passed, so a request made while the primary is being replaced
// waits for the new one instead of failing. Beginning a transaction is held
// the same way (nothing has been sent yet). A statement inside a
// transaction is never retried, and neither is one that may have reached
// the server: it fails as before.
//
// A statement that only reads (SELECT), preparing a statement and beginning
// a transaction have no effect to repeat, so they are also tried again when
// the connection was lost after they were sent (a pooled connection to a
// server that has gone). When a connection is found lost, the pool's idle
// connections are closed, so the next statement dials afresh rather than
// finding another dead one.
//
// When the window runs out the error is an *UnavailableError; IsUnavailable
// also recognises the errors of a transaction that lost its connection, so
// callers can answer 503 database_unavailable for both.
package dbhold

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/TykTechnologies/midsommar/v2/metrics"
	"github.com/TykTechnologies/midsommar/v2/pkg/dbstate"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// DefaultWindow is DATABASE_WRITE_RETRY_WINDOW's default.
const DefaultWindow = 15 * time.Second

// Retry delays: quick at first (a connection being re-dialled to the new
// primary), then at most once a second.
var (
	retryFirst = 100 * time.Millisecond
	retryMax   = time.Second
)

// Pool is a gorm ConnPool (and TxBeginner) over db that holds statements
// across a failover. gorm's DB() finds db through GetDBConn.
type Pool struct {
	db      *sql.DB
	window  time.Duration
	tracker *dbstate.Tracker
	maxIdle int
	held    atomic.Int64
	// gaveUp is when a hold last ran out its window (unix nanoseconds).
	gaveUp atomic.Int64
}

// New wraps db. A window of 0 or less holds nothing: every call goes
// straight to db. tracker, when not nil, is told of every read-only
// refusal, so the pool's connections are dialled again to the new primary
// between attempts. maxIdle is the pool's idle connection limit (0: the
// database/sql default), restored after idle connections are closed.
func New(db *sql.DB, window time.Duration, tracker *dbstate.Tracker, maxIdle int) *Pool {
	if maxIdle <= 0 {
		maxIdle = 2 // database/sql's default
	}
	return &Pool{db: db, window: window, tracker: tracker, maxIdle: maxIdle}
}

// GetDBConn returns the wrapped pool (gorm.GetDBConnector).
func (p *Pool) GetDBConn() (*sql.DB, error) { return p.db, nil }

// PrepareContext prepares query, held across a failover.
func (p *Pool) PrepareContext(ctx context.Context, query string) (stmt *sql.Stmt, err error) {
	err = p.hold(ctx, true, func() error {
		stmt, err = p.db.PrepareContext(ctx, query)
		return err
	})
	return stmt, err
}

// ExecContext runs query, held across a failover. Never repeatable: a
// SELECT run through Exec is there for its side effect (SELECT
// pg_notify(...)), and gorm's reads go through QueryContext.
func (p *Pool) ExecContext(ctx context.Context, query string, args ...any) (res sql.Result, err error) {
	err = p.hold(ctx, false, func() error {
		res, err = p.db.ExecContext(ctx, query, args...)
		return err
	})
	return res, err
}

// QueryContext runs query, held across a failover.
func (p *Pool) QueryContext(ctx context.Context, query string, args ...any) (rows *sql.Rows, err error) {
	err = p.hold(ctx, readOnly(query), func() error {
		rows, err = p.db.QueryContext(ctx, query, args...)
		return err
	})
	return rows, err
}

// QueryRowContext runs query. Its error is only seen when the row is
// scanned, so it cannot be retried; instead, while the tracker says the
// database is not writable, it waits (within the window) for it to be.
func (p *Pool) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	if p.enter() {
		p.waitWritable(ctx)
		p.leave()
	}
	return p.db.QueryRowContext(ctx, query, args...)
}

// BeginTx begins a transaction, held across a failover: nothing of the
// transaction has been sent until it has begun. What runs inside it is not
// held.
func (p *Pool) BeginTx(ctx context.Context, opts *sql.TxOptions) (tx *sql.Tx, err error) {
	err = p.hold(ctx, true, func() error {
		tx, err = p.db.BeginTx(ctx, opts)
		return err
	})
	return tx, err
}

// readOnly reports whether query only reads, so running it twice is
// harmless: a SELECT (row locks taken outside a transaction end with it).
// It is a prefix check, not a parser: a SELECT that calls a function with
// side effects (pg_notify, nextval, a user function that writes) must not
// be run through QueryContext outside a transaction, or it may run twice
// after a lost connection; use Exec, which is never repeated. Today
// pg_notify goes through Exec or inside a WITH ... INSERT, and advisory
// locks through Exec or a dedicated connection.
func readOnly(query string) bool {
	q := strings.TrimLeft(query, " \t\r\n(")
	return len(q) >= 6 && strings.EqualFold(q[:6], "SELECT")
}

// hold runs op, and again while it fails retryably, until the window has
// passed or ctx is done. repeatable ops are also retried when the
// connection was lost after they were sent.
func (p *Pool) hold(ctx context.Context, repeatable bool, op func() error) error {
	retryable := func(err error) bool {
		if Retryable(err) {
			return true
		}
		if IsUnavailable(err) {
			// The connection was lost: the next attempt, and other
			// statements, must not find another dead one.
			p.closeIdle()
			return repeatable
		}
		return false
	}
	err := op()
	if err == nil || p.window <= 0 || !retryable(err) {
		return err
	}
	if time.Since(time.Unix(0, p.gaveUp.Load())) < p.window {
		// A hold ran out its window with no writable server less than a
		// window ago: the outage is longer than the window, so do not
		// make this statement (and the request after it) wait it out
		// again. A request runs several statements in turn; this keeps
		// it to about one window. After a window the next one waits
		// again, as a probe.
		NoteUnavailable()
		metrics.RecordDBWriteHold(ctx, "rejected", 0, 0)
		return &UnavailableError{Err: err}
	}
	if !p.enter() {
		// MaxHeld statements are waiting already: fail at once.
		NoteUnavailable()
		metrics.RecordDBWriteHold(ctx, "rejected", 0, 0)
		return &UnavailableError{Err: err}
	}
	defer p.leave()
	started := time.Now()
	deadline := started.Add(p.window)
	delay := retryFirst
	for retries := 1; ; retries++ {
		p.noteFailure(err)
		wait := min(delay, time.Until(deadline))
		if wait <= 0 {
			p.gaveUp.Store(time.Now().UnixNano())
			NoteUnavailable()
			metrics.RecordDBWriteHold(ctx, "gave_up", retries-1, time.Since(started).Seconds())
			return &UnavailableError{Held: time.Since(started), Err: err}
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			metrics.RecordDBWriteHold(ctx, "cancelled", retries-1, time.Since(started).Seconds())
			return fmt.Errorf("%w (held %s for a writable database: %v)", ctx.Err(), time.Since(started).Round(time.Millisecond), err)
		case <-timer.C:
		}
		delay = min(2*delay, retryMax)
		if err = op(); err == nil {
			metrics.RecordDBWriteHold(ctx, "succeeded", retries, time.Since(started).Seconds())
			return nil
		}
		if !retryable(err) {
			metrics.RecordDBWriteHold(ctx, "failed", retries, time.Since(started).Seconds())
			return err
		}
	}
}

// MaxHeld is the most statements held at once, per pool. During a failover
// under load every request could otherwise sit in the hold for the whole
// window (services mostly run statements without the request's context, so
// one stays held after its client has gone), each retrying against a
// primary that is just coming up. Beyond it a statement fails at once as
// an UnavailableError (503 database_unavailable).
const MaxHeld = 512

// enter takes a place among the held statements; false when MaxHeld are
// held already. The place is taken by compare-and-swap, so Held never
// passes MaxHeld, not even for the instant a statement over the cap is
// being refused.
func (p *Pool) enter() bool {
	for {
		n := p.held.Load()
		if n >= MaxHeld {
			return false
		}
		if p.held.CompareAndSwap(n, n+1) {
			break
		}
	}
	metrics.AddDBWritesHeld(context.Background(), 1)
	return true
}

func (p *Pool) leave() {
	p.held.Add(-1)
	metrics.AddDBWritesHeld(context.Background(), -1)
}

// Held is how many statements are held now.
func (p *Pool) Held() int64 { return p.held.Load() }

// closeIdle closes the pool's idle connections (database/sql closes those
// over a lowered limit), then restores the limit.
func (p *Pool) closeIdle() {
	p.db.SetMaxIdleConns(-1)
	p.db.SetMaxIdleConns(p.maxIdle)
}

// noteFailure tells the tracker of a read-only refusal, so the connection
// is not used again for the next attempt.
func (p *Pool) noteFailure(err error) {
	if p.tracker != nil && dbstate.IsReadOnlyError(err) {
		p.tracker.ReportReadOnly()
	}
}

// waitWritable waits, within the window, while the tracker says the
// database is not writable.
func (p *Pool) waitWritable(ctx context.Context) {
	if p.tracker == nil || p.window <= 0 || p.tracker.Status().State == dbstate.Writable {
		return
	}
	started := time.Now()
	deadline := started.Add(p.window)
	ticker := time.NewTicker(retryFirst)
	defer ticker.Stop()
	for p.tracker.Status().State != dbstate.Writable && time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// Retryable reports whether err means a statement cannot have executed, so
// running it again, whatever it is, cannot apply it twice. Each class, and
// why:
//   - SQLSTATE 25006 (read-only transaction): the server refused the
//     statement before executing it, so it wrote nothing.
//   - 57P03 (cannot connect now): sent only while a connection starts,
//     before any statement.
//   - *pgconn.ConnectError: the connection was never established, so the
//     statement was never sent.
//   - driver.ErrBadConn: by database/sql's contract a driver returns it
//     only when the server cannot have performed the operation ("ErrBadConn
//     should NOT be returned if there's a possibility that the database
//     server might have performed the operation", database/sql/driver).
//   - pgconn.SafeToRetry: pgx's guarantee that the error "occurred before
//     sending any data to the server".
//
// Not here: 57P01 and 57P02 (the server terminating the session). 57P02 is
// sent from the quickdie signal handler, which can run after an autocommit
// statement's commit record was flushed and before CommandComplete, so the
// write may exist; 57P01 is treated the same. Like a connection lost after
// sending, they count as IsUnavailable, and only a repeatable statement
// (a read, a prepare, BEGIN) is retried after them.
func Retryable(err error) bool {
	if err == nil {
		return false
	}
	var connectErr *pgconn.ConnectError
	var pgErr *pgconn.PgError
	switch {
	case dbstate.IsReadOnlyError(err):
		return true
	case errors.As(err, &pgErr) && pgErr.Code == "57P03":
		return true
	case errors.As(err, &connectErr):
		return true
	case errors.Is(err, driver.ErrBadConn):
		return true
	case pgconn.SafeToRetry(err):
		return true
	}
	return false
}

// UnavailableError is a statement that could not reach a writable database
// within the hold window.
type UnavailableError struct {
	Held time.Duration
	Err  error
}

func (e *UnavailableError) Error() string {
	return fmt.Sprintf("database unavailable (no writable server within %s): %v", e.Held.Round(time.Millisecond), e.Err)
}

func (e *UnavailableError) Unwrap() error { return e.Err }

// IsUnavailable reports whether err means the database could not be used
// rather than that the statement was wrong: a hold that ran out, a
// statement the hold does not retry (one inside a transaction, or with the
// window at 0) that failed for the same reasons, or a connection lost while
// it was in use (a transaction that lost its connection midway, a server
// shutting down: SQLSTATE class 08 and 57P01 to 57P03).
func IsUnavailable(err error) bool {
	if err == nil {
		return false
	}
	var unavailable *UnavailableError
	if errors.As(err, &unavailable) || Retryable(err) {
		return true
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return strings.HasPrefix(pgErr.Code, "08") || pgErr.Code == "57P01" || pgErr.Code == "57P02" || pgErr.Code == "57P03"
	}
	// *net.OpError, not net.Error: a context deadline is a net.Error too,
	// and a request that ran out of time is not a database outage.
	var opErr *net.OpError
	return errors.Is(err, sql.ErrConnDone) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.As(err, &opErr) || strings.Contains(err.Error(), "conn closed")
}

// lastUnavailable is when an unavailable database was last seen (unix
// nanoseconds), by any statement.
var lastUnavailable atomic.Int64

// NoteUnavailable records that a statement just found the database
// unavailable (IsUnavailable). The API answers a 500 written by a request
// that was running since then as 503 database_unavailable.
func NoteUnavailable() { lastUnavailable.Store(time.Now().UnixNano()) }

// UnavailableSince reports whether a statement found the database
// unavailable at or after t. It is not tied to one request: during an
// outage a 500 of any request running at the time counts as the outage's.
func UnavailableSince(t time.Time) bool { return lastUnavailable.Load() >= t.UnixNano() }

// Unheld returns db with statements that are not held: they go straight to
// the pool under the hold and fail as they always did. For the callers that
// must fail fast rather than queue: the control server's RPCs (an edge
// answered "unavailable" moves to its other control plane) and every
// background loop (a lease renewal must not wait past its term; a ticker
// tries again on its next tick). Apply it once, at construction: the
// result keeps going straight to the pool through Session, WithContext
// and transactions. A db without a hold is returned as it is.
func Unheld(db *gorm.DB) *gorm.DB {
	if db == nil {
		return nil
	}
	pool, ok := db.Statement.ConnPool.(*Pool)
	if !ok {
		if pool, ok = db.ConnPool.(*Pool); !ok {
			return db
		}
	}
	// A Session shares db's Statement unless given a Context: give it
	// one, so the pool is replaced on a copy and db stays held.
	ctx := db.Statement.Context
	if ctx == nil {
		ctx = context.Background()
	}
	tx := db.Session(&gorm.Session{Context: ctx})
	tx.Config.ConnPool = pool.db
	tx.Statement.ConnPool = pool.db
	return tx
}
