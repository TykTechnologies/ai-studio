package pglisten

import (
	"database/sql"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
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
	db, err := sql.Open("pgx", os.Getenv("DATABASE_URL"))
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

// withParams returns a test DSN with params set (an empty value removes
// the parameter).
func withParams(t *testing.T, params map[string]string) (dsn, appName string) {
	t.Helper()
	dsn, appName = testDSN(t)
	u, err := url.Parse(dsn)
	require.NoError(t, err)
	q := u.Query()
	for k, v := range params {
		if v == "" {
			q.Del(k)
		} else {
			q.Set(k, v)
		}
	}
	u.RawQuery = q.Encode()
	return u.String(), appName
}

// The listener accepts exactly the DSNs the application's pool (pgx, through
// gorm) accepts. These three are what the Helm chart, compose file and
// PgBouncer users pass; lib/pq refused every one of them, and Studio hung
// at startup.
func TestListenerAcceptsPoolDSNs_Postgres(t *testing.T) {
	for name, params := range map[string]map[string]string{
		// pgx defaults to prefer: TLS if the server offers it, else plain.
		"no sslmode":             {"sslmode": ""},
		"sslmode=prefer":         {"sslmode": "prefer"},
		"sslmode=allow":          {"sslmode": "allow"},
		"pgx-only option":        {"default_query_exec_mode": "simple_protocol"},
		"statement cache option": {"statement_cache_capacity": "0"},
	} {
		t.Run(name, func(t *testing.T) {
			dsn, _ := withParams(t, params)
			db := admin(t)
			l, err := Acquire(dsn, Options{ConnectTimeout: 10 * time.Second, Name: "test listener"})
			require.NoError(t, err)
			defer l.Release()

			got := make(chan string, 1)
			_, err = l.Subscribe("pglisten_dsn", func(p string) { got <- p }, 5*time.Second)
			require.NoError(t, err)
			notify(t, db, "pglisten_dsn", name)
			assert.Equal(t, name, recv(t, got))
		})
	}
}

// A listener that cannot connect fails Acquire promptly with the reason,
// rather than waiting out the connect timeout.
func TestAcquireFailsFastOnRefusedConnection_Postgres(t *testing.T) {
	dsn, _ := withParams(t, map[string]string{"sslmode": "require"})
	start := time.Now()
	_, err := Acquire(dsn, Options{ConnectTimeout: 20 * time.Second, Name: "test listener"})
	if err == nil {
		t.Skip("the test server offers TLS")
	}
	assert.Less(t, time.Since(start), 10*time.Second)
	assert.Contains(t, err.Error(), "test listener could not connect")
}

// Subscribing and unsubscribing while notifications stream in: every
// channel's handler sees only its own channel, and LISTEN/UNLISTEN never
// interleave on the connection.
func TestConcurrentSubscribeWhileNotifying_Postgres(t *testing.T) {
	dsn, _ := testDSN(t)
	db := admin(t)
	l := acquire(t, dsn)
	defer l.Release()

	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			default:
			}
			_, _ = db.Exec(`SELECT pg_notify('pglisten_busy', 'x')`)
		}
	}()
	defer func() { close(stop); <-done }()

	for i := 0; i < 20; i++ {
		ch := fmt.Sprintf("pglisten_c%d", i)
		got := make(chan string, 1)
		sub, err := l.Subscribe(ch, func(p string) {
			select {
			case got <- p:
			default:
			}
		}, 5*time.Second)
		require.NoError(t, err)
		notify(t, db, ch, ch)
		assert.Equal(t, ch, recv(t, got))
		l.Unsubscribe(ch, sub, 5*time.Second)
	}
}

// Waking the owner (every first Subscribe and last Unsubscribe does) cancels
// its wait for a notification. That must leave the connection as it was:
// same backend, no reconnect, and a quiet period that reaches PingInterval
// pings rather than reconnects.
func TestWakingTheListenerKeepsItsConnection_Postgres(t *testing.T) {
	old := PingInterval
	PingInterval = 100 * time.Millisecond
	defer func() { PingInterval = old }()

	dsn, appName := testDSN(t)
	db := admin(t)
	l := acquire(t, dsn)
	defer l.Release()
	var reconnects atomic.Int32
	defer l.OnReconnect(func() { reconnects.Add(1) })()

	pid := func() int {
		var p int
		require.NoError(t, db.QueryRow(`SELECT pid FROM pg_stat_activity WHERE application_name = $1`, appName).Scan(&p))
		return p
	}
	before := pid()

	got := make(chan string, 64)
	for i := 0; i < 50; i++ {
		ch := fmt.Sprintf("pglisten_wake_%d", i)
		sub, err := l.Subscribe(ch, func(p string) { got <- p }, 5*time.Second)
		require.NoError(t, err)
		if i%10 == 0 {
			notify(t, db, ch, ch)
			assert.Equal(t, ch, recv(t, got))
		}
		l.Unsubscribe(ch, sub, 5*time.Second)
	}
	time.Sleep(500 * time.Millisecond) // several ping intervals

	_, err := l.Subscribe("pglisten_wake_last", func(p string) { got <- p }, 5*time.Second)
	require.NoError(t, err)
	notify(t, db, "pglisten_wake_last", "still here")
	assert.Equal(t, "still here", recv(t, got))
	assert.Equal(t, before, pid(), "the listener must keep its backend")
	assert.Zero(t, reconnects.Load())
	assert.Equal(t, 1, backends(t, db, appName))
}

// Follow on a listener that cannot connect returns at once (the service
// polls meanwhile), keeps trying, and once the database is reachable
// subscribes and runs onReconnect so the service catches up.
func TestFollowRetriesUntilReachable_Postgres(t *testing.T) {
	old := FollowRetryInterval
	FollowRetryInterval = 100 * time.Millisecond
	defer func() { FollowRetryInterval = old }()

	base, _ := testDSN(t)
	db := admin(t)
	u, err := url.Parse(base)
	require.NoError(t, err)
	target := u.Host

	// A port nothing listens on yet.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close())
	u.Host = addr
	dsn := u.String()

	var caughtUp atomic.Int32
	got := make(chan string, 4)
	start := time.Now()
	f := Follow(dsn, "pglisten_follow", Options{ConnectTimeout: 2 * time.Second, Name: "follow test"}, func(p string) { got <- p }, func() { caughtUp.Add(1) })
	defer f.Close()
	assert.Less(t, time.Since(start), 2*time.Second, "Follow must not wait for an unreachable database")
	assert.False(t, f.Listening())

	// Now the database appears on that port.
	proxy, err := net.Listen("tcp", addr)
	require.NoError(t, err)
	defer proxy.Close()
	go func() {
		for {
			c, err := proxy.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				up, err := net.Dial("tcp", target)
				if err != nil {
					return
				}
				defer up.Close()
				go func() { _, _ = io.Copy(up, c) }()
				_, _ = io.Copy(c, up)
			}()
		}
	}()

	require.Eventually(t, f.Listening, 10*time.Second, 20*time.Millisecond)
	require.Eventually(t, func() bool { return caughtUp.Load() == 1 }, 5*time.Second, 20*time.Millisecond)
	notify(t, db, "pglisten_follow", "hello")
	assert.Equal(t, "hello", recv(t, got))
}

// Closing a Follower that never managed to subscribe returns at once.
func TestFollowCloseWhileUnreachable_Postgres(t *testing.T) {
	base, _ := testDSN(t)
	u, err := url.Parse(base)
	require.NoError(t, err)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	u.Host = ln.Addr().String()
	require.NoError(t, ln.Close())

	f := Follow(u.String(), "pglisten_follow_close", Options{ConnectTimeout: time.Second, Name: "follow test"}, func(string) {}, func() {})
	start := time.Now()
	f.Close()
	assert.Less(t, time.Since(start), time.Second)
	assert.False(t, f.Listening())
}
