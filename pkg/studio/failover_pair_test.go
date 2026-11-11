package studio

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/config"
	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/cluster"
	"github.com/TykTechnologies/midsommar/v2/pkg/dbstate"
	"github.com/TykTechnologies/midsommar/v2/pkg/pglisten"
	"github.com/TykTechnologies/midsommar/v2/pkg/testinfra/containers"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// These tests fail a real database over: a primary and a hot standby in
// Docker (containers.StartPostgresPair), with Studio's pool on the pair's
// multi-host DSN (postgres://u:p@a,b/db?target_session_attrs=read-write).
// They are skipped under -short; scripts/ci-postgres-tests.sh runs them
// (suite "failover").

func failoverConf(pair *containers.PostgresPair) *config.AppConf {
	return &config.AppConf{
		DatabaseType:            "postgres",
		DatabaseURL:             pair.DSN(),
		DatabaseMaxOpenConns:    8,
		DatabaseMaxIdleConns:    8,
		DatabaseConnMaxLifetime: 30 * time.Minute,
		DatabaseConnectTimeout:  2 * time.Second,
	}
}

func openFailoverDB(t *testing.T, pair *containers.PostgresPair) *gorm.DB {
	t.Helper()
	db, err := OpenDatabase(failoverConf(pair))
	require.NoError(t, err)
	t.Cleanup(func() {
		if pool, err := db.DB(); err == nil {
			pool.Close()
		}
	})
	return db
}

// lockedLog is a log sink safe for the tracker's goroutine.
type lockedLog struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (l *lockedLog) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.Write(p)
}

func (l *lockedLog) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.String()
}

type failoverNote struct {
	ID   uint `gorm:"primaryKey"`
	Note string
}

func serverAddr(t *testing.T, db *gorm.DB) string {
	t.Helper()
	var addr string
	require.NoError(t, db.Raw("SELECT host(inet_server_addr())").Scan(&addr).Error)
	return addr
}

// fillPool leaves n idle connections in the pool.
func fillPool(t *testing.T, db *gorm.DB, n int) {
	t.Helper()
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			assert.NoError(t, db.Exec("SELECT pg_sleep(0.3)").Error)
		}()
	}
	wg.Wait()
	pool, err := db.DB()
	require.NoError(t, err)
	require.GreaterOrEqual(t, pool.Stats().OpenConnections, n)
}

// The primary is lost and the standby promoted: the replica's pool, opened
// on the multi-host DSN, writes again without being reopened, and the
// tracker has counted the failover. The pool settings are applied and
// logged at start.
func TestFailover_PoolFollowsPromotedStandby(t *testing.T) {
	pair := containers.StartPostgresPair(t, &containers.PostgresPairConfig{Synchronous: true})
	ctx := context.Background()

	prev := logger.Log
	t.Cleanup(func() { logger.Use(prev) })
	sink := &lockedLog{}
	logger.Use(zerolog.New(sink).Level(zerolog.DebugLevel))

	db := openFailoverDB(t, pair)
	pool, err := db.DB()
	require.NoError(t, err)
	assert.Equal(t, 8, pool.Stats().MaxOpenConnections)
	assert.Contains(t, sink.String(), "Database pool: 2 host(s), target_session_attrs=read-write, max_open_conns=8, max_idle_conns=8, conn_max_lifetime=30m0s, connect_timeout=2s")

	tracker := dbstate.For(db)
	require.NotNil(t, tracker)
	assert.Equal(t, dbstate.Writable, tracker.Status().State)

	require.NoError(t, db.AutoMigrate(&failoverNote{}))
	require.NoError(t, db.Create(&failoverNote{Note: "before"}).Error)
	fillPool(t, db, 4)
	before := serverAddr(t, db)

	require.NoError(t, pair.Failover(ctx))

	// No restart, no reopen: the same *gorm.DB writes again.
	require.Eventually(t, func() bool {
		return db.Create(&failoverNote{Note: "after"}).Error == nil
	}, 30*time.Second, 100*time.Millisecond, "the pool reaches the promoted standby")
	assert.NotEqual(t, before, serverAddr(t, db))
	var notes []failoverNote
	require.NoError(t, db.Order("id").Find(&notes).Error)
	require.NotEmpty(t, notes)
	assert.Equal(t, "before", notes[0].Note, "synchronous replication: nothing acknowledged is lost")

	require.Eventually(t, func() bool {
		st := tracker.Status()
		return st.State == dbstate.Writable && st.Failovers == 1
	}, 3*dbstate.DefaultCheckInterval, 100*time.Millisecond, "the tracker saw the failover: %+v", tracker.Status())
}

// The primary is demoted but keeps running, with the pool's connections
// still open on it (every write there now fails with SQLSTATE 25006). With
// no statement to trip over it, the periodic check moves the pool within
// one period; with a write in flight, the first refusal moves it at once.
func TestFailover_PoolLeavesDemotedPrimary(t *testing.T) {
	pair := containers.StartPostgresPair(t, &containers.PostgresPairConfig{Synchronous: true})
	ctx := context.Background()
	db := openFailoverDB(t, pair)
	tracker := dbstate.For(db)
	require.NoError(t, db.AutoMigrate(&failoverNote{}))

	t.Run("found by the periodic check", func(t *testing.T) {
		fillPool(t, db, 6)
		old := serverAddr(t, db)
		require.NoError(t, pair.DemotePrimary(ctx))

		// The replica is idle: nothing fails, so only the check can tell.
		require.Eventually(t, func() bool {
			st := tracker.Status()
			return st.State == dbstate.Writable && st.Failovers == 1
		}, dbstate.DefaultCheckInterval+5*time.Second, 50*time.Millisecond, "within one check period: %+v", tracker.Status())

		// Every connection the pool hands out now is to the new primary,
		// and the first write succeeds.
		var wg sync.WaitGroup
		var onOld atomic.Int32
		for i := 0; i < 12; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				var addr string
				if err := db.Raw("SELECT host(inet_server_addr()) FROM pg_sleep(0.1)").Scan(&addr).Error; err != nil || addr == old {
					onOld.Add(1)
				}
			}()
		}
		wg.Wait()
		assert.Zero(t, onOld.Load(), "no connection is left on the demoted primary")
		assert.NoError(t, db.Create(&failoverNote{Note: "after the check"}).Error)
	})

	t.Run("found by a refused write", func(t *testing.T) {
		require.NoError(t, pair.RejoinOldPrimary(ctx))
		fillPool(t, db, 6)
		require.NoError(t, pair.DemotePrimary(ctx))

		// Straight away, well inside the check period: the first write
		// may be refused (25006); that refusal retires the connections,
		// so the writes that follow reach the new primary.
		start := time.Now()
		refused := 0
		var err error
		for attempt := 0; attempt < 20; attempt++ {
			if err = db.Create(&failoverNote{Note: "after a refusal"}).Error; err == nil {
				break
			}
			require.True(t, dbstate.IsReadOnlyError(err), "refused for another reason: %v", err)
			refused++
		}
		require.NoError(t, err)
		assert.LessOrEqual(t, refused, 2, "one refusal is enough to move the pool")
		assert.Less(t, time.Since(start), dbstate.DefaultCheckInterval-time.Second, "not left to the periodic check")
		require.Eventually(t, func() bool { return tracker.Status().Failovers == 2 },
			2*dbstate.DefaultCheckInterval, 50*time.Millisecond, "%+v", tracker.Status())
	})
}

// The LISTEN connection is opened from the same multi-host DSN. When its
// server is demoted (it stays connected, but NOTIFYs are now sent on the
// other node) it reconnects to the new primary and runs its reconnect
// hooks; cluster events published by one replica reach the other again,
// by notification.
func TestFailover_ListenerAndClusterEventsMoveToNewPrimary(t *testing.T) {
	prevCheck := pglisten.RoleCheckInterval
	pglisten.RoleCheckInterval = time.Second
	t.Cleanup(func() { pglisten.RoleCheckInterval = prevCheck })

	pair := containers.StartPostgresPair(t, &containers.PostgresPairConfig{Synchronous: true})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	a, b := openFailoverDB(t, pair), openFailoverDB(t, pair)
	require.NoError(t, a.AutoMigrate(&models.ClusterEvent{}))

	// A long poll interval: events must arrive by NOTIFY, not by polling.
	opts := cluster.LogOptions{PollInterval: time.Hour}
	logA, logB := cluster.NewLog(a, "replica-a", opts), cluster.NewLog(b, "replica-b", opts)
	require.NoError(t, logA.Start(ctx))
	require.NoError(t, logB.Start(ctx))
	t.Cleanup(func() { logA.Stop(); logB.Stop() })
	got := make(chan string, 16)
	logB.Subscribe("failover.test", func(ev cluster.Event) { got <- string(ev.Payload) })
	expect := func(want string) {
		t.Helper()
		select {
		case payload := <-got:
			assert.Equal(t, want, payload)
		case <-time.After(15 * time.Second):
			t.Fatalf("event %q did not reach the other replica", want)
		}
	}
	require.NoError(t, logA.Publish(ctx, "failover.test", []byte("before")))
	expect("before")

	// A subscriber of its own on the shared listener, to see the reconnect.
	dsn, err := pglisten.DSN(a)
	require.NoError(t, err)
	listener, err := pglisten.Acquire(dsn, pglisten.Options{})
	require.NoError(t, err)
	t.Cleanup(listener.Release)
	reconnected := make(chan struct{}, 4)
	t.Cleanup(listener.OnReconnect(func() { reconnected <- struct{}{} }))
	notified := make(chan string, 4)
	sub, err := listener.Subscribe("failover_test_channel", func(p string) { notified <- p }, 10*time.Second)
	require.NoError(t, err)
	t.Cleanup(func() { listener.Unsubscribe("failover_test_channel", sub, time.Second) })

	require.NoError(t, pair.DemotePrimary(ctx))

	select {
	case <-reconnected:
	case <-time.After(20 * time.Second):
		t.Fatal("the listener stayed on the demoted primary")
	}
	// The pools move too (pg_notify alone is not a write, so nothing is
	// refused: the periodic check moves them); then a NOTIFY on the new
	// primary is heard.
	require.Eventually(t, func() bool { return dbstate.For(a).Status().Failovers == 1 },
		3*dbstate.DefaultCheckInterval, 100*time.Millisecond)
	require.NoError(t, a.Exec("SELECT pg_notify('failover_test_channel', 'after')").Error)
	select {
	case p := <-notified:
		assert.Equal(t, "after", p)
	case <-time.After(15 * time.Second):
		t.Fatal("no notification from the new primary")
	}

	require.Eventually(t, func() bool {
		return logA.Publish(ctx, "failover.test", []byte("after")) == nil
	}, 20*time.Second, 100*time.Millisecond)
	// "after" may be published more than once if a first attempt was
	// refused after its row was written; one arrival is what matters.
	expect("after")
}

// A hot standby refuses LISTEN. That used to be recorded as the channel's
// failure on a connection the listener then kept; it is now a reason to
// reconnect, so with only a standby to reach a subscriber waits and gets a
// plain error instead of a listener that never hears anything.
func TestFailover_ListenerDoesNotSettleOnAStandby(t *testing.T) {
	pair := containers.StartPostgresPair(t, nil)
	listener, err := pglisten.Acquire(pair.Standby().DSN(), pglisten.Options{ReconnectInterval: 200 * time.Millisecond, ReconnectCeiling: time.Second})
	require.NoError(t, err)
	t.Cleanup(listener.Release)

	_, err = listener.Subscribe("standby_channel", func(string) {}, 3*time.Second)
	require.Error(t, err)
	assert.False(t, strings.Contains(fmt.Sprint(err), "25006"), "not a per-channel failure: %v", err)
}
