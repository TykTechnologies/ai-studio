package dbhold

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
)

func TestRetryableAndUnavailable(t *testing.T) {
	pg := func(code string) error { return fmt.Errorf("wrapped: %w", &pgconn.PgError{Code: code}) }
	cases := []struct {
		name                   string
		err                    error
		retryable, unavailable bool
	}{
		{"read-only server", pg("25006"), true, true},
		// Session terminated: may come after the commit (quickdie), so only
		// a repeatable statement is retried after it (see hold).
		{"admin shutdown", pg("57P01"), false, true},
		{"crash shutdown", pg("57P02"), false, true},
		{"cannot connect now", pg("57P03"), true, true},
		{"bad connection", driver.ErrBadConn, true, true},
		{"held too long", &UnavailableError{Held: time.Second, Err: errors.New("x")}, false, true},
		{"connection lost after sending", io.ErrUnexpectedEOF, false, true},
		{"connection done", sql.ErrConnDone, false, true},
		{"connection failure class", pg("08006"), false, true},
		{"network", &net.OpError{Op: "read", Err: errors.New("connection reset by peer")}, false, true},
		{"unique violation", pg("23505"), false, false},
		{"no rows", sql.ErrNoRows, false, false},
		{"request ran out of time", context.DeadlineExceeded, false, false},
		{"request cancelled", context.Canceled, false, false},
		{"nil", nil, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.retryable, Retryable(tc.err), "Retryable")
			assert.Equal(t, tc.unavailable, IsUnavailable(tc.err), "IsUnavailable")
		})
	}
}

func TestReadOnlyStatements(t *testing.T) {
	for q, want := range map[string]bool{
		`SELECT * FROM "apps"`:                true,
		"  select count(*) from users":        true,
		"(SELECT 1) UNION (SELECT 2)":         true,
		`INSERT INTO "apps" ... RETURNING id`: false,
		`UPDATE "apps" SET name = $1`:         false,
		"WITH x AS (DELETE FROM a) SELECT 1":  false,
		"":                                    false,
	} {
		assert.Equal(t, want, readOnly(q), q)
	}
}

func TestUnavailableSince(t *testing.T) {
	start := time.Now()
	assert.False(t, UnavailableSince(start.Add(time.Hour)))
	NoteUnavailable()
	assert.True(t, UnavailableSince(start))
}

// fakeDriver fails every statement with err and counts them.
type fakeDriver struct {
	err   error
	calls *atomic.Int64
}

func (d fakeDriver) Open(string) (driver.Conn, error) { return fakeConn(d), nil }

type fakeConn fakeDriver

func (c fakeConn) Prepare(string) (driver.Stmt, error) { return nil, c.err }
func (fakeConn) Close() error                          { return nil }
func (fakeConn) Begin() (driver.Tx, error)             { return nil, errors.New("no") }
func (c fakeConn) ExecContext(context.Context, string, []driver.NamedValue) (driver.Result, error) {
	c.calls.Add(1)
	return nil, c.err
}
func (c fakeConn) QueryContext(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
	c.calls.Add(1)
	return nil, c.err
}

var driverSeq atomic.Int64

func fakePool(t *testing.T, err error, window time.Duration) (*Pool, *atomic.Int64) {
	t.Helper()
	calls := &atomic.Int64{}
	name := fmt.Sprintf("dbhold-fake-%d", driverSeq.Add(1))
	sql.Register(name, fakeDriver{err: err, calls: calls})
	db, openErr := sql.Open(name, "")
	if openErr != nil {
		t.Fatal(openErr)
	}
	t.Cleanup(func() { db.Close() })
	return New(db, window, nil, 0), calls
}

// After a terminated session (57P02, which can follow the commit), a write
// is not sent again; a read is.
func TestTerminatedSessionRetriesOnlyRepeatable(t *testing.T) {
	p, calls := fakePool(t, &pgconn.PgError{Code: "57P02"}, 300*time.Millisecond)
	_, err := p.ExecContext(context.Background(), "INSERT INTO a VALUES (1)")
	assert.True(t, IsUnavailable(err))
	assert.EqualValues(t, 1, calls.Load(), "a write is sent once")

	calls.Store(0)
	_, err = p.QueryContext(context.Background(), "SELECT 1")
	var unavailable *UnavailableError
	assert.True(t, errors.As(err, &unavailable), "%v", err)
	assert.Greater(t, calls.Load(), int64(1), "a read is tried again")
}

// A SELECT through Exec (SELECT pg_notify(...)) is there for its side
// effect: not repeated after a lost connection.
func TestExecIsNeverRepeatable(t *testing.T) {
	p, calls := fakePool(t, io.ErrUnexpectedEOF, 300*time.Millisecond)
	_, err := p.ExecContext(context.Background(), "SELECT pg_notify('c', 'payload')")
	assert.True(t, IsUnavailable(err))
	assert.EqualValues(t, 1, calls.Load())
}

// Beyond MaxHeld statements held at once, a statement fails at once.
func TestMaxHeld(t *testing.T) {
	p, _ := fakePool(t, &pgconn.PgError{Code: "25006"}, time.Minute)
	p.held.Store(MaxHeld)
	started := time.Now()
	_, err := p.ExecContext(context.Background(), "INSERT INTO a VALUES (1)")
	var unavailable *UnavailableError
	assert.True(t, errors.As(err, &unavailable), "%v", err)
	assert.Less(t, time.Since(started), time.Second, "not held")
	assert.EqualValues(t, MaxHeld, p.Held())

	p.held.Store(0)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = p.ExecContext(ctx, "INSERT INTO a VALUES (1)")
	}()
	assert.Eventually(t, func() bool { return p.Held() == 1 }, time.Second, 5*time.Millisecond)
	<-done
	assert.EqualValues(t, 0, p.Held(), "released when it ends")
}

// Once a hold has run out its window, statements failing within the next
// window are not held (the outage is longer than the window); after it, one
// is held again.
func TestNoSecondWaitWithinAWindowOfGivingUp(t *testing.T) {
	window := 300 * time.Millisecond
	p, _ := fakePool(t, &pgconn.PgError{Code: "25006"}, window)
	started := time.Now()
	_, err := p.ExecContext(context.Background(), "INSERT INTO a VALUES (1)")
	assert.True(t, IsUnavailable(err))
	assert.GreaterOrEqual(t, time.Since(started), window, "the first is held the window")

	started = time.Now()
	_, err = p.ExecContext(context.Background(), "INSERT INTO a VALUES (2)")
	assert.True(t, IsUnavailable(err))
	assert.Less(t, time.Since(started), window/2, "the next fails at once")

	time.Sleep(window)
	started = time.Now()
	_, _ = p.ExecContext(context.Background(), "INSERT INTO a VALUES (3)")
	assert.GreaterOrEqual(t, time.Since(started), window, "after a window, held again")
}
