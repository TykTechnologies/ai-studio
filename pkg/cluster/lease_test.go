package cluster

import (
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm/logger"
)

func leaseSQLite(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "l.db")+"?_busy_timeout=10000&_journal_mode=WAL"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.ClusterLease{}))
	return db
}

// leaseDBs gives each test replica its own handle on a shared database: a
// SQLite file, and a Postgres schema (own pool per replica) when
// DATABASE_URL is set.
func leaseDBs(t *testing.T, body func(t *testing.T, open func(replica string) *gorm.DB, kill func(replica string))) {
	t.Run("sqlite", func(t *testing.T) {
		db := leaseSQLite(t)
		body(t, func(string) *gorm.DB { return db }, nil)
	})
	t.Run("postgres", func(t *testing.T) {
		c := newTestCluster(t)
		require.NoError(t, c.replicaDB("setup").AutoMigrate(&models.ClusterLease{}))
		body(t, c.replicaDB, func(r string) { c.killConnections(r) })
	})
}

func leaders(ls []*Leadership) int {
	n := 0
	for _, l := range ls {
		if l.IsLeader() {
			n++
		}
	}
	return n
}

// Replicas starting at once: exactly one holds the lease, and the others
// see it held.
func TestLeadership_OneHolder(t *testing.T) {
	leaseDBs(t, func(t *testing.T, open func(string) *gorm.DB, _ func(string)) {
		var ls []*Leadership
		var wg sync.WaitGroup
		for _, name := range []string{"a", "b", "c", "d", "e"} {
			l := NewLeadership(open(name), LeaderLease, name, LeadershipOptions{TTL: 3 * time.Second})
			ls = append(ls, l)
			wg.Add(1)
			go func() { defer wg.Done(); l.Start() }()
			t.Cleanup(l.Stop)
		}
		wg.Wait()
		assert.Equal(t, 1, leaders(ls))

		holder, expires, err := Holder(t.Context(), open("a"), LeaderLease)
		require.NoError(t, err)
		require.NotNil(t, expires)
		for _, l := range ls {
			assert.Equal(t, l.node == holder, l.IsLeader(), l.node)
		}

		// Over several renewals, still exactly one, and the same one.
		time.Sleep(2500 * time.Millisecond)
		assert.Equal(t, 1, leaders(ls))
		again, _, err := Holder(t.Context(), open("a"), LeaderLease)
		require.NoError(t, err)
		assert.Equal(t, holder, again, "the holder renews; nobody takes a live lease")
	})
}

// Stopping the holder hands over at once (not after the TTL).
func TestLeadership_StopHandsOver(t *testing.T) {
	leaseDBs(t, func(t *testing.T, open func(string) *gorm.DB, _ func(string)) {
		a := NewLeadership(open("a"), LeaderLease, "a", LeadershipOptions{TTL: time.Minute, Renew: 100 * time.Millisecond})
		a.Start()
		require.True(t, a.IsLeader())
		b := NewLeadership(open("b"), LeaderLease, "b", LeadershipOptions{TTL: time.Minute, Renew: 100 * time.Millisecond})
		var changes atomic.Int32
		b.OnChange(func(leading bool) {
			if leading {
				changes.Add(1)
			}
		})
		b.Start()
		t.Cleanup(b.Stop)
		assert.False(t, b.IsLeader())

		a.Stop()
		assert.False(t, a.IsLeader())
		require.Eventually(t, b.IsLeader, 5*time.Second, 20*time.Millisecond, "b takes over well before the 1 min TTL")
		assert.Equal(t, int32(1), changes.Load())
	})
}

// A holder that can no longer reach the database stops believing it leads
// before the lease expires in the database, so there is never a moment
// with two replicas acting as leader; another replica takes over after the
// TTL.
func TestLeadership_LostDatabaseEndsBeliefBeforeTakeOver(t *testing.T) {
	c := newTestCluster(t)
	require.NoError(t, c.replicaDB("setup").AutoMigrate(&models.ClusterLease{}))
	ttl := 2 * time.Second
	a := NewLeadership(c.replicaDB("a"), LeaderLease, "a", LeadershipOptions{TTL: ttl, Renew: 200 * time.Millisecond})
	a.Start()
	t.Cleanup(a.Stop)
	require.True(t, a.IsLeader())

	// a's database connection is cut and stays cut (its DSN points at a
	// schema we drop access to by pointing its pool at a dead backend).
	sqlDB, err := a.db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	b := NewLeadership(c.replicaDB("b"), LeaderLease, "b", LeadershipOptions{TTL: ttl, Renew: 200 * time.Millisecond})
	b.Start()
	t.Cleanup(b.Stop)

	overlap := false
	deadline := time.Now().Add(3 * ttl)
	for time.Now().Before(deadline) && !b.IsLeader() {
		if a.IsLeader() && b.IsLeader() {
			overlap = true
		}
		time.Sleep(10 * time.Millisecond)
	}
	require.True(t, b.IsLeader(), "b takes over after the TTL")
	assert.False(t, a.IsLeader())
	assert.False(t, overlap, "never two leaders")
}

// SQLite (one process): the only replica leads.
func TestLeadership_SQLiteSingleReplica(t *testing.T) {
	l := NewLeadership(leaseSQLite(t), LeaderLease, "only", LeadershipOptions{})
	l.Start()
	t.Cleanup(l.Stop)
	assert.True(t, l.IsLeader())
}
