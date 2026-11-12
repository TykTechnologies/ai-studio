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
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/TykTechnologies/midsommar/v2/metrics"
	"github.com/TykTechnologies/midsommar/v2/pkg/dbstate"
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
}

// New wraps db. A window of 0 or less holds nothing: every call goes
// straight to db. tracker, when not nil, is told of every read-only
// refusal, so the pool's connections are dialled again to the new primary
// between attempts.
func New(db *sql.DB, window time.Duration, tracker *dbstate.Tracker) *Pool {
	return &Pool{db: db, window: window, tracker: tracker}
}

// GetDBConn returns the wrapped pool (gorm.GetDBConnector).
func (p *Pool) GetDBConn() (*sql.DB, error) { return p.db, nil }

// PrepareContext prepares query, held across a failover.
func (p *Pool) PrepareContext(ctx context.Context, query string) (stmt *sql.Stmt, err error) {
	err = p.hold(ctx, func() error {
		stmt, err = p.db.PrepareContext(ctx, query)
		return err
	})
	return stmt, err
}

// ExecContext runs query, held across a failover.
func (p *Pool) ExecContext(ctx context.Context, query string, args ...any) (res sql.Result, err error) {
	err = p.hold(ctx, func() error {
		res, err = p.db.ExecContext(ctx, query, args...)
		return err
	})
	return res, err
}

// QueryContext runs query, held across a failover.
func (p *Pool) QueryContext(ctx context.Context, query string, args ...any) (rows *sql.Rows, err error) {
	err = p.hold(ctx, func() error {
		rows, err = p.db.QueryContext(ctx, query, args...)
		return err
	})
	return rows, err
}

// QueryRowContext runs query. Its error is only seen when the row is
// scanned, so it cannot be retried; instead, while the tracker says the
// database is not writable, it waits (within the window) for it to be.
func (p *Pool) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	p.waitWritable(ctx)
	return p.db.QueryRowContext(ctx, query, args...)
}

// BeginTx begins a transaction, held across a failover: nothing of the
// transaction has been sent until it has begun. What runs inside it is not
// held.
func (p *Pool) BeginTx(ctx context.Context, opts *sql.TxOptions) (tx *sql.Tx, err error) {
	err = p.hold(ctx, func() error {
		tx, err = p.db.BeginTx(ctx, opts)
		return err
	})
	return tx, err
}

// hold runs op, and again while it fails retryably, until the window has
// passed or ctx is done.
func (p *Pool) hold(ctx context.Context, op func() error) error {
	err := op()
	if err == nil || p.window <= 0 || !Retryable(err) {
		return err
	}
	started := time.Now()
	deadline := started.Add(p.window)
	delay := retryFirst
	for retries := 1; ; retries++ {
		p.noteFailure(err)
		wait := min(delay, time.Until(deadline))
		if wait <= 0 {
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
		if !Retryable(err) {
			metrics.RecordDBWriteHold(ctx, "failed", retries, time.Since(started).Seconds())
			return err
		}
	}
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

// Retryable reports whether err means a statement never reached a writable
// server, so running it again cannot apply it twice: the connection could
// not be made (or was refused while the server starts or stops), the
// server refused it as read-only (SQLSTATE 25006), or the driver reports
// that nothing was sent.
func Retryable(err error) bool {
	if err == nil {
		return false
	}
	var connectErr *pgconn.ConnectError
	switch {
	case dbstate.IsReadOnlyError(err):
		return true
	case errors.As(err, &connectErr):
		return true
	case errors.Is(err, driver.ErrBadConn):
		// database/sql retries a bad connection itself; a driver returns
		// it only when nothing was sent.
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
