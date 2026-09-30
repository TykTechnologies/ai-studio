package cluster

import (
	"os"
	"os/exec"
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

// leaseCluster is a Postgres schema with the lease and node tables, for
// tests that run several replicas: SQLite serves one process, which always
// leads.
func leaseCluster(t *testing.T) *testCluster {
	c := newTestCluster(t)
	require.NoError(t, c.replicaDB("setup").AutoMigrate(&models.ClusterLease{}, &models.ClusterNode{}))
	return c
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
func TestLeadership_OneHolder_Postgres(t *testing.T) {
	c := leaseCluster(t)
	open := c.replicaDB
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
}

// Stopping the holder hands over at once (not after the TTL).
func TestLeadership_StopHandsOver_Postgres(t *testing.T) {
	c := leaseCluster(t)
	open := c.replicaDB
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
}

// A holder that can no longer reach the database stops believing it leads
// before the lease expires in the database, so there is never a moment
// with two replicas acting as leader; another replica takes over after the
// TTL.
func TestLeadership_LostDatabaseEndsBeliefBeforeTakeOver_Postgres(t *testing.T) {
	c := leaseCluster(t)
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

// SQLite after a crash: the lease row still names the dead process and has
// not expired. The new process leads at once instead of waiting out the
// TTL (leader-only start-up work such as the marketplace sync would be
// skipped meanwhile), and records itself as the holder.
func TestLeadership_SQLiteLeadsAtOnceAfterACrash(t *testing.T) {
	db := leaseSQLite(t)
	require.NoError(t, db.Exec(`INSERT INTO cluster_leases (name, holder, acquired_at, renewed_at, expires_at) VALUES (?, 'crashed-process', ?, ?, ?)`,
		LeaderLease, nowExpr(db), nowExpr(db), untilExpr(db, 30*time.Second)).Error)

	l := NewLeadership(db, LeaderLease, "restarted", LeadershipOptions{})
	var gained atomic.Int32
	l.OnChange(func(leading bool) {
		if leading {
			gained.Add(1)
		}
	})
	l.Start()
	assert.True(t, l.IsLeader(), "leads at once")
	assert.Equal(t, int32(1), gained.Load())
	holder, _, err := Holder(t.Context(), db, LeaderLease)
	require.NoError(t, err)
	assert.Equal(t, "restarted", holder)

	l.Stop()
	assert.False(t, l.IsLeader(), "not after Stop")
}

// exitedPID returns the pid of a process that has exited (and was reaped).
func exitedPID(t *testing.T) int {
	cmd := exec.Command("true")
	require.NoError(t, cmd.Run())
	pid := cmd.ProcessState.Pid()
	require.False(t, processAlive(pid))
	return pid
}

// holdLease registers node as a replica that ran as proc and was last seen
// silent ago, holding the leader lease for another 30 s: what a replica
// that was killed leaves behind.
func holdLease(t *testing.T, db *gorm.DB, node string, proc processInfo, silent time.Duration) {
	t.Helper()
	require.NoError(t, db.Exec(`INSERT INTO cluster_nodes (node_id, hostname, version, started_at, last_seen, pid, boot_id, pid_namespace) VALUES (?, ?, 'test', ?, ?, ?, ?, ?)`,
		node, proc.Hostname, sinceExpr(db, time.Hour), sinceExpr(db, silent), proc.PID, proc.BootID, proc.PIDNamespace).Error)
	require.NoError(t, db.Exec(`INSERT INTO cluster_leases (name, holder, acquired_at, renewed_at, expires_at) VALUES (?, ?, now(), now(), now() + interval '30 seconds')
		ON CONFLICT (name) DO UPDATE SET holder = EXCLUDED.holder, expires_at = EXCLUDED.expires_at`, LeaderLease, node).Error)
}

// A replica restarted on the same host after its predecessor was killed
// takes the lease at once when the predecessor's process is gone (checked
// by pid in the same pid namespace), instead of waiting up to the 30 s TTL.
func TestLeadership_TakesOverFromDeadPredecessor_Postgres(t *testing.T) {
	me := thisProcess()
	require.NotEmpty(t, me.BootID, "this test needs a readable boot ID")

	for name, pid := range map[string]int{
		"its process exited":                  exitedPID(t),
		"its pid is now this process (pid 1)": os.Getpid(),
	} {
		t.Run(name, func(t *testing.T) {
			c := leaseCluster(t)
			db := c.replicaDB("new")
			old := me
			old.PID = pid
			holdLease(t, db, "old-incarnation", old, time.Second)

			l := NewLeadership(db, LeaderLease, "new-incarnation", LeadershipOptions{})
			l.Start()
			t.Cleanup(l.Stop)
			assert.True(t, l.IsLeader(), "leads at once")
			holder, _, err := Holder(t.Context(), db, LeaderLease)
			require.NoError(t, err)
			assert.Equal(t, "new-incarnation", holder)
		})
	}
}

// A holder that may be alive keeps the lease until it expires: a live
// process on this host (two Studios on one machine sharing a database), a
// replica of this very process, a holder on another machine (even one with
// the same hostname and a pid that does not exist here), and a holder
// registered before processes were recorded.
func TestLeadership_KeepsOffAHolderThatMayBeAlive_Postgres(t *testing.T) {
	me := thisProcess()
	require.NotEmpty(t, me.BootID, "this test needs a readable boot ID")
	with := func(f func(p *processInfo)) processInfo { p := me; f(&p); return p }

	cases := map[string]struct {
		proc    processInfo
		running bool // the holder is a replica this process runs
	}{
		"live process on this host": {proc: with(func(p *processInfo) { p.PID = os.Getppid() })},
		"replica in this process":   {proc: me, running: true},
		"another machine, same hostname": {proc: with(func(p *processInfo) {
			p.BootID, p.PID = "another-machine", exitedPID(t)
		})},
		"another hostname on this machine": {proc: with(func(p *processInfo) { p.Hostname, p.PID = "other-host", exitedPID(t) })},
		"registered before pids were":      {proc: processInfo{Hostname: me.Hostname}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			c := leaseCluster(t)
			db := c.replicaDB("new")
			holdLease(t, db, "holder", tc.proc, time.Second)
			if tc.running {
				nodesHere.Store("holder", struct{}{})
				t.Cleanup(func() { nodesHere.Delete("holder") })
			}

			l := NewLeadership(db, LeaderLease, "new", LeadershipOptions{Renew: 100 * time.Millisecond, PredecessorSilence: 200 * time.Millisecond})
			l.Start()
			t.Cleanup(l.Stop)
			time.Sleep(time.Second)
			assert.False(t, l.IsLeader())
			holder, _, err := Holder(t.Context(), db, LeaderLease)
			require.NoError(t, err)
			assert.Equal(t, "holder", holder)
		})
	}
}

// A container restarted in its pod: same machine and hostname, pid 1 again
// but in a new pid namespace, so the old pid cannot be checked. The lease
// is taken over once the holder has been silent in the registry for
// PredecessorSilence (well before the TTL), and never while it refreshes
// its registration (two live containers given one hostname).
func TestLeadership_OtherPIDNamespaceTakenOverOnceSilent_Postgres(t *testing.T) {
	me := thisProcess()
	require.NotEmpty(t, me.BootID, "this test needs a readable boot ID")
	other := me
	other.PIDNamespace, other.PID = "pid:[4026532999]", 1
	opts := LeadershipOptions{Renew: 10 * time.Second, PredecessorSilence: 1500 * time.Millisecond}

	t.Run("silent predecessor", func(t *testing.T) {
		c := leaseCluster(t)
		db := c.replicaDB("new")
		holdLease(t, db, "previous-container", other, 0)
		l := NewLeadership(db, LeaderLease, "restarted-container", opts)
		start := time.Now()
		l.Start()
		t.Cleanup(l.Stop)
		assert.False(t, l.IsLeader(), "not while it may still be alive")
		require.Eventually(t, l.IsLeader, 5*time.Second, 20*time.Millisecond, "taken over once silent, before the next renewal and the TTL")
		assert.GreaterOrEqual(t, time.Since(start), time.Second)
	})

	t.Run("live container with the same hostname", func(t *testing.T) {
		c := leaseCluster(t)
		db := c.replicaDB("new")
		holdLease(t, db, "sibling", other, 0)
		stop := make(chan struct{})
		done := make(chan struct{})
		go func() {
			defer close(done)
			for {
				select {
				case <-stop:
					return
				case <-time.After(200 * time.Millisecond):
					db.Exec(`UPDATE cluster_nodes SET last_seen = now() WHERE node_id = 'sibling'`)
				}
			}
		}()
		t.Cleanup(func() { close(stop); <-done })
		l := NewLeadership(db, LeaderLease, "new", LeadershipOptions{Renew: 200 * time.Millisecond, PredecessorSilence: opts.PredecessorSilence})
		l.Start()
		t.Cleanup(l.Stop)
		time.Sleep(3 * time.Second)
		assert.False(t, l.IsLeader())
	})
}
