package cluster

import (
	"net/url"
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
	"github.com/TykTechnologies/midsommar/v2/pkg/safe"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/postgres"
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

	// After a failover to a server whose clock is ahead, a live sibling
	// looks silent by the database's clock until its next refresh. That
	// alone is not evidence: this replica must have watched the silence.
	t.Run("live container that only looks silent by the database's clock", func(t *testing.T) {
		c := leaseCluster(t)
		db := c.replicaDB("new")
		// Refreshed a moment ago by the old clock: 20 s ago by the new one.
		holdLease(t, db, "sibling", other, 20*time.Second)
		l := NewLeadership(db, LeaderLease, "new", LeadershipOptions{Renew: 200 * time.Millisecond, PredecessorSilence: opts.PredecessorSilence})
		l.Start()
		t.Cleanup(l.Stop)
		assert.False(t, l.IsLeader(), "not on the database clock's word")
		// The sibling refreshes within its heartbeat interval, and keeps on.
		time.Sleep(500 * time.Millisecond)
		stop := make(chan struct{})
		done := make(chan struct{})
		go func() {
			defer close(done)
			for {
				db.Exec(`UPDATE cluster_nodes SET last_seen = now() WHERE node_id = 'sibling'`)
				db.Exec(`UPDATE cluster_leases SET renewed_at = now(), expires_at = now() + interval '30 seconds' WHERE holder = 'sibling'`)
				select {
				case <-stop:
					return
				case <-time.After(200 * time.Millisecond):
				}
			}
		}()
		t.Cleanup(func() { close(stop); <-done })
		time.Sleep(3 * time.Second)
		assert.False(t, l.IsLeader(), "a live sibling keeps the lease")
	})

	// Silent for longer than any clock difference explains: taken at once.
	t.Run("predecessor silent beyond the clock allowance", func(t *testing.T) {
		c := leaseCluster(t)
		db := c.replicaDB("new")
		holdLease(t, db, "previous-container", other, time.Minute)
		l := NewLeadership(db, LeaderLease, "restarted-container", opts)
		l.Start()
		t.Cleanup(l.Stop)
		assert.True(t, l.IsLeader())
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

// A panic in a lease listener (here on gaining the lease, at Start and again
// in the renewal loop) drops the belief at once and tells the listeners;
// the loop restarts and the replica leads again.
func TestLeadership_PanicDropsBeliefAndRecovers_Postgres(t *testing.T) {
	c := leaseCluster(t)
	l := NewLeadership(c.replicaDB("a"), LeaderLease, "a", LeadershipOptions{TTL: time.Minute, Renew: 100 * time.Millisecond})
	var mu sync.Mutex
	var seen []bool
	l.OnChange(func(leading bool) {
		mu.Lock()
		seen = append(seen, leading)
		n := len(seen)
		mu.Unlock()
		if n == 1 || n == 3 {
			panic("listener bug")
		}
	})
	before := safe.Panics()
	l.Start()
	t.Cleanup(l.Stop)

	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(seen) == 5
	}, 10*time.Second, 20*time.Millisecond, "gained, dropped, gained, dropped, gained")
	assert.True(t, l.IsLeader())
	mu.Lock()
	assert.Equal(t, []bool{true, false, true, false, true}, seen)
	mu.Unlock()
	assert.Equal(t, before+2, safe.Panics())
}

// cutOff closes a replica's pool: it can no longer renew, as when the
// database is unreachable from it.
func cutOff(t *testing.T, l *Leadership) {
	t.Helper()
	sqlDB, err := l.db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
}

// watchLeaders samples both replicas until b leads or the deadline passes,
// and reports whether both ever believed they led at once and when b took
// over.
func watchLeaders(a, b *Leadership, deadline time.Duration) (overlap bool, took time.Duration) {
	start := time.Now()
	for time.Since(start) < deadline {
		aLeads, bLeads := a.IsLeader(), b.IsLeader()
		if aLeads && bLeads {
			overlap = true
		}
		if bLeads && !aLeads {
			return overlap, time.Since(start)
		}
		time.Sleep(5 * time.Millisecond)
	}
	return overlap, -1
}

// After a failover the database's clock is the new primary's. When that
// clock is ahead, the lease its holder renewed a moment ago looks expired at
// once. Another replica must not take it on that: the holder still believes
// it leads until its belief runs out by its own clock.
func TestLeadership_DatabaseClockAheadNeverGivesTwoLeaders_Postgres(t *testing.T) {
	c := leaseCluster(t)
	opts := LeadershipOptions{TTL: 3 * time.Second, Renew: 300 * time.Millisecond}
	a := NewLeadership(c.replicaDB("a"), LeaderLease, "a", opts)
	a.Start()
	t.Cleanup(a.Stop)
	require.True(t, a.IsLeader())
	b := NewLeadership(c.replicaDB("b"), LeaderLease, "b", opts)
	b.Start()
	t.Cleanup(b.Stop)
	time.Sleep(500 * time.Millisecond)
	require.False(t, b.IsLeader())

	// The failover: a loses the database right after a renewal, and on the
	// new primary, whose clock is 20 s ahead, that renewal's lease expired
	// 17 s ago.
	cutOff(t, a)
	require.NoError(t, c.admin.Exec(`UPDATE "`+c.schema+`".cluster_leases SET expires_at = expires_at - interval '20 seconds'`).Error)
	require.True(t, a.IsLeader(), "a still believes: its last renewal is a moment old")

	overlap, took := watchLeaders(a, b, 3*opts.TTL)
	require.GreaterOrEqual(t, took, time.Duration(0), "b takes over")
	assert.False(t, overlap, "never two leaders")
	assert.GreaterOrEqual(t, took, a.believeFor()-opts.Renew-500*time.Millisecond, "b waited for a's belief to run out, although the lease looked expired")
	assert.LessOrEqual(t, took, opts.TTL+opts.Renew, "and no longer than a TTL")
}

// When the new primary's clock is behind, the lease looks valid for longer
// than its TTL. A replica that watches it go unrenewed takes it after a
// TTL all the same: the cluster is not left without a leader for the
// difference between two servers' clocks.
func TestLeadership_DatabaseClockBehindDoesNotDelayTakeOver_Postgres(t *testing.T) {
	c := leaseCluster(t)
	opts := LeadershipOptions{TTL: 3 * time.Second, Renew: 300 * time.Millisecond}
	a := NewLeadership(c.replicaDB("a"), LeaderLease, "a", opts)
	a.Start()
	t.Cleanup(a.Stop)
	require.True(t, a.IsLeader())
	b := NewLeadership(c.replicaDB("b"), LeaderLease, "b", opts)
	b.Start()
	t.Cleanup(b.Stop)

	// a is gone, and by the new primary's clock (30 s behind) its lease
	// lasts another 33 s.
	cutOff(t, a)
	require.NoError(t, c.admin.Exec(`UPDATE "`+c.schema+`".cluster_leases SET expires_at = expires_at + interval '30 seconds'`).Error)

	overlap, took := watchLeaders(a, b, 4*opts.TTL)
	require.GreaterOrEqual(t, took, time.Duration(0), "b takes over although the lease has not expired by the database's clock")
	assert.False(t, overlap, "never two leaders")
	assert.LessOrEqual(t, took, opts.TTL+opts.Renew, "within a TTL")
}

// A lease whose holder keeps renewing is never taken, however long another
// replica watches it and whatever the expiry says.
func TestLeadership_RenewingHolderKeepsTheLeaseWhateverTheClock_Postgres(t *testing.T) {
	c := leaseCluster(t)
	opts := LeadershipOptions{TTL: 2 * time.Second, Renew: 200 * time.Millisecond}
	a := NewLeadership(c.replicaDB("a"), LeaderLease, "a", opts)
	a.Start()
	t.Cleanup(a.Stop)
	b := NewLeadership(c.replicaDB("b"), LeaderLease, "b", opts)
	b.Start()
	t.Cleanup(b.Stop)

	// For three TTLs every renewal of a's is made to look expired by the
	// database's clock, as with a clock that runs ahead.
	stop := time.Now().Add(3 * opts.TTL)
	for time.Now().Before(stop) {
		require.NoError(t, c.admin.Exec(`UPDATE "`+c.schema+`".cluster_leases SET expires_at = now() - interval '1 second' WHERE holder = 'a'`).Error)
		require.False(t, b.IsLeader(), "b took a lease its holder was renewing")
		time.Sleep(50 * time.Millisecond)
	}
	assert.True(t, a.IsLeader())
}

// A cluster that starts after everything was down for a while finds a lease
// that expired long ago: no clock difference explains that, and the first
// replica takes it at once.
func TestLeadership_LongExpiredLeaseIsTakenAtOnce_Postgres(t *testing.T) {
	c := leaseCluster(t)
	require.NoError(t, c.admin.Exec(`INSERT INTO "`+c.schema+`".cluster_leases (name, holder, acquired_at, renewed_at, expires_at)
		VALUES (?, 'gone', now() - interval '10 minutes', now() - interval '5 minutes', now() - interval '4 minutes')`, LeaderLease).Error)
	b := NewLeadership(c.replicaDB("b"), LeaderLease, "b", LeadershipOptions{TTL: time.Minute})
	b.Start()
	t.Cleanup(b.Stop)
	assert.True(t, b.IsLeader())
}

// readOnlyReplicaDB is a replica's pool on a server that answers reads and
// refuses writes (SQLSTATE 25006): what a pool still connected to a demoted
// primary has until it is moved to the new one.
func (c *testCluster) readOnlyReplicaDB(replica string) *gorm.DB {
	u, err := url.Parse(c.dsn(replica))
	require.NoError(c.t, err)
	q := u.Query()
	q.Set("options", "-c default_transaction_read_only=on")
	u.RawQuery = q.Encode()
	db, err := gorm.Open(postgres.Open(u.String()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(c.t, err)
	c.t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})
	return db
}

// A replica whose pool is still on a demoted primary could read the lease
// there: the old holder's last renewal, which nobody renews on that server
// any more. It never gets as far as acting on it. Every tick begins with a
// write (renew, or take a lease nobody holds), the demoted server refuses
// it, and the tick ends there with an error: the replica does not come to
// believe it leads on the word of a server that is no longer the primary.
// It leads only once a write succeeds, when its pool has moved to the new
// primary.
func TestLeadership_TakerOnAReadOnlyServerDoesNotLead_Postgres(t *testing.T) {
	c := leaseCluster(t)
	opts := LeadershipOptions{TTL: 2 * time.Second, Renew: 200 * time.Millisecond}
	a := NewLeadership(c.replicaDB("a"), LeaderLease, "a", opts)
	a.Start()
	t.Cleanup(a.Stop)
	require.True(t, a.IsLeader())

	// b ran before the demotion (the lease row is known to exist) and its
	// connections are now on the demoted server. Its ticks are made by hand
	// here, to see what each one does.
	b := NewLeadership(c.readOnlyReplicaDB("b"), LeaderLease, "b", opts)
	b.created = true

	// a is gone; its last renewal stays in the row.
	cutOff(t, a)

	// Well past the time a taker waits, tick after tick: each is refused at
	// its first statement and b never believes it leads.
	until := time.Now().Add(b.takeAfter() + 500*time.Millisecond)
	for ticks := 0; time.Now().Before(until) || ticks < 3; ticks++ {
		held, _, err := b.acquire()
		require.Error(t, err, "a tick's first statement is a write, which a demoted primary refuses")
		assert.Contains(t, err.Error(), "25006", "read_only_sql_transaction")
		require.False(t, held)
		require.False(t, b.IsLeader())
		time.Sleep(opts.Renew)
	}
	var holder string
	require.NoError(t, c.admin.Raw(`SELECT holder FROM "`+c.schema+`".cluster_leases WHERE name = ?`, LeaderLease).Scan(&holder).Error)
	assert.Equal(t, "a", holder, "the row is as the old holder left it")

	// The same replica on a server that takes writes does take it.
	w := NewLeadership(c.replicaDB("b"), LeaderLease, "b", opts)
	w.Start()
	t.Cleanup(w.Stop)
	require.Eventually(t, w.IsLeader, 3*opts.TTL, 20*time.Millisecond)
}
