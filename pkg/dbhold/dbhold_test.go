package dbhold

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"net"
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
		{"admin shutdown", pg("57P01"), true, true},
		{"crash shutdown", pg("57P02"), true, true},
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
