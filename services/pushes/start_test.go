package pushes

import (
	"context"
	"net"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// stopsWithin fails the test if stop does not return within d, rather than
// hanging the test binary the way a blocked Stop hung Studio's startup
// cleanup.
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
func TestCoordinatorStopWithoutStart(t *testing.T) {
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		c := New(db, "a", newFakeStreams(), Options{})
		stopsWithin(t, 2*time.Second, c.Stop)
		assert.ErrorIs(t, c.Start(context.Background()), errStoppedBeforeStart)
	})
}

// A Start that fails (its context has ended, so the first query fails)
// returns the error, and Stop still returns at once.
func TestCoordinatorStopAfterFailedStart_Postgres(t *testing.T) {
	if os.Getenv("DATABASE_URL") == "" {
		t.Skip("DATABASE_URL not set")
	}
	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		if db.Dialector.Name() != "postgres" {
			return
		}
		c := New(db, "a", newFakeStreams(), Options{})
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		require.Error(t, c.Start(ctx))
		stopsWithin(t, 2*time.Second, c.Stop)
	})
}

// A listener that cannot connect does not fail delivery: the coordinator
// starts, polls, and stops at once.
func TestCoordinatorWithoutListenerPolls_Postgres(t *testing.T) {
	base := os.Getenv("DATABASE_URL")
	if base == "" {
		t.Skip("DATABASE_URL not set")
	}
	u, err := url.Parse(base)
	require.NoError(t, err)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	u.Host = ln.Addr().String()
	require.NoError(t, ln.Close())

	forEachDB(t, func(t *testing.T, db *gorm.DB) {
		if db.Dialector.Name() != "postgres" {
			return
		}
		r := newReplica(t, db, "a", Options{ListenerDSN: u.String(), PollInterval: 100 * time.Millisecond})
		start := time.Now()
		require.NoError(t, r.c.Start(context.Background()))
		assert.Less(t, time.Since(start), 10*time.Second)
		assert.False(t, r.c.follower.Listening())

		addEdge(t, db, "edge-1", "default", models.EdgeStatusConnected, "a")
		r.streams.open("edge-1", "s1")
		res := push(t, r.c, Request{Scope: ScopeEdge, EdgeIDs: []string{"edge-1"}})
		require.Eventually(t, func() bool { return len(r.streams.sends()) == 1 }, 5*time.Second, 20*time.Millisecond,
			"the poll delivers the push of operation %s", res.Operation.OperationID)
		stopsWithin(t, 2*time.Second, r.c.Stop)
	})
}
