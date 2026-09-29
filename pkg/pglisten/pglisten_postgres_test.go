package pglisten

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testDSN returns DATABASE_URL with a unique application_name, so each test
// gets a listener of its own (listeners are shared per DSN) and can find its
// backend in pg_stat_activity.
func testDSN(t *testing.T) (dsn, appName string) {
	t.Helper()
	base := os.Getenv("DATABASE_URL")
	if base == "" {
		t.Skip("DATABASE_URL not set")
	}
	appName = fmt.Sprintf("pglisten_test_%d", time.Now().UnixNano())
	u, err := url.Parse(base)
	require.NoError(t, err)
	q := u.Query()
	q.Set("application_name", appName)
	u.RawQuery = q.Encode()
	return u.String(), appName
}

func admin(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("postgres", os.Getenv("DATABASE_URL"))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return db
}

func notify(t *testing.T, db *sql.DB, channel, payload string) {
	t.Helper()
	_, err := db.Exec(`SELECT pg_notify($1, $2)`, channel, payload)
	require.NoError(t, err)
}

func acquire(t *testing.T, dsn string) *Listener {
	t.Helper()
	l, err := Acquire(dsn, Options{ReconnectInterval: 50 * time.Millisecond, ReconnectCeiling: 200 * time.Millisecond, ConnectTimeout: 10 * time.Second, Name: "test listener"})
	require.NoError(t, err)
	return l
}

func TestSubscribeReceivesNotifications_Postgres(t *testing.T) {
	dsn, _ := testDSN(t)
	db := admin(t)
	l := acquire(t, dsn)
	defer l.Release()

	got := make(chan string, 4)
	sub, err := l.Subscribe("pglisten_a", func(p string) { got <- p }, 5*time.Second)
	require.NoError(t, err)

	notify(t, db, "pglisten_a", "one")
	notify(t, db, "pglisten_other", "not mine")
	notify(t, db, "pglisten_a", "two")
	assert.Equal(t, "one", recv(t, got))
	assert.Equal(t, "two", recv(t, got))

	l.Unsubscribe("pglisten_a", sub, 5*time.Second)
	notify(t, db, "pglisten_a", "after unsubscribe")
	select {
	case p := <-got:
		t.Fatalf("handler ran after Unsubscribe returned: %q", p)
	case <-time.After(300 * time.Millisecond):
	}
}

// Two subscribers on one channel both hear it; removing one leaves the
// other (and the channel's LISTEN) in place.
func TestSubscribersShareAChannel_Postgres(t *testing.T) {
	dsn, _ := testDSN(t)
	db := admin(t)
	l := acquire(t, dsn)
	defer l.Release()

	a, b := make(chan string, 4), make(chan string, 4)
	subA, err := l.Subscribe("pglisten_shared", func(p string) { a <- p }, 5*time.Second)
	require.NoError(t, err)
	_, err = l.Subscribe("pglisten_shared", func(p string) { b <- p }, 5*time.Second)
	require.NoError(t, err)

	notify(t, db, "pglisten_shared", "x")
	assert.Equal(t, "x", recv(t, a))
	assert.Equal(t, "x", recv(t, b))

	l.Unsubscribe("pglisten_shared", subA, 5*time.Second)
	notify(t, db, "pglisten_shared", "y")
	assert.Equal(t, "y", recv(t, b))
}

// Acquire shares one listener per DSN; the last Release closes it.
func TestAcquireIsReferenceCounted_Postgres(t *testing.T) {
	dsn, appName := testDSN(t)
	db := admin(t)
	l1 := acquire(t, dsn)
	l2 := acquire(t, dsn)
	assert.Same(t, l1, l2)
	assert.Equal(t, 1, backends(t, db, appName))

	l1.Release()
	assert.Equal(t, 1, backends(t, db, appName), "still referenced")
	l2.Release()
	require.Eventually(t, func() bool { return backends(t, db, appName) == 0 }, 5*time.Second, 50*time.Millisecond)
}

// When the listener's connection is killed, OnReconnect hooks run once it is
// back (anything sent in between is lost, and the hook is how a subscriber
// finds out), and notifications flow again on the re-issued LISTEN.
func TestReconnectRunsHooksAndResumes_Postgres(t *testing.T) {
	dsn, appName := testDSN(t)
	db := admin(t)
	l := acquire(t, dsn)
	defer l.Release()

	got := make(chan string, 8)
	_, err := l.Subscribe("pglisten_reconnect", func(p string) { got <- p }, 5*time.Second)
	require.NoError(t, err)
	var reconnects atomic.Int32
	remove := l.OnReconnect(func() { reconnects.Add(1) })

	notify(t, db, "pglisten_reconnect", "before")
	assert.Equal(t, "before", recv(t, got))

	_, err = db.Exec(`SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE application_name = $1`, appName)
	require.NoError(t, err)

	require.Eventually(t, func() bool { return reconnects.Load() >= 1 }, 15*time.Second, 50*time.Millisecond, "OnReconnect hook")
	require.Eventually(t, func() bool {
		notify(t, db, "pglisten_reconnect", "after")
		select {
		case p := <-got:
			return p == "after"
		case <-time.After(200 * time.Millisecond):
			return false
		}
	}, 10*time.Second, 50*time.Millisecond, "notifications after reconnect")

	remove()
	n := reconnects.Load()
	_, err = db.Exec(`SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE application_name = $1`, appName)
	require.NoError(t, err)
	time.Sleep(time.Second)
	assert.Equal(t, n, reconnects.Load(), "a removed hook must not run")
}

// Payloads arrive intact up to NOTIFY's limit (just under 8000 bytes).
func TestLargePayload_Postgres(t *testing.T) {
	dsn, _ := testDSN(t)
	db := admin(t)
	l := acquire(t, dsn)
	defer l.Release()

	got := make(chan string, 1)
	_, err := l.Subscribe("pglisten_large", func(p string) { got <- p }, 5*time.Second)
	require.NoError(t, err)
	payload := strings.Repeat("x", 7900)
	notify(t, db, "pglisten_large", payload)
	assert.Equal(t, payload, recv(t, got))
}

func recv(t *testing.T, ch chan string) string {
	t.Helper()
	select {
	case p := <-ch:
		return p
	case <-time.After(5 * time.Second):
		t.Fatal("no notification within 5s")
		return ""
	}
}

func backends(t *testing.T, db *sql.DB, appName string) int {
	t.Helper()
	var n int
	require.NoError(t, db.QueryRow(`SELECT count(*) FROM pg_stat_activity WHERE application_name = $1`, appName).Scan(&n))
	return n
}
