package cluster

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm/clause"
)

// LeaderLease is the lease singleton background work runs under: at most
// one replica holds it, and jobs that must run once for the whole cluster
// (aggregations, alerts, syncs, cleanups) check Leadership.IsLeader first.
const LeaderLease = "leader"

// LeadershipOptions tune a Leadership. The zero value gives the defaults.
type LeadershipOptions struct {
	// TTL is how long the lease lasts without renewal (default 30 s);
	// Renew how often the holder renews it and others try to take it
	// (default TTL/3).
	TTL   time.Duration
	Renew time.Duration
	// PredecessorSilence is how long a holder that ran on this machine
	// under this hostname, but in another pid namespace (typically this
	// container's predecessor in its pod), must have been silent in the
	// node registry before this replica takes the lease from it (default
	// two HeartbeatIntervals). See predecessorGone.
	PredecessorSilence time.Duration
}

func (o LeadershipOptions) withDefaults() LeadershipOptions {
	if o.TTL <= 0 {
		o.TTL = 30 * time.Second
	}
	if o.Renew <= 0 || o.Renew >= o.TTL {
		o.Renew = o.TTL / 3
	}
	if o.PredecessorSilence <= 0 {
		o.PredecessorSilence = 2 * HeartbeatInterval
	}
	return o
}

// Leadership keeps this replica's claim on a named lease. The lease is
// taken and renewed with conditional writes against the database's clock,
// so at most one replica holds it. A replica believes it leads only until
// its last successful renewal plus the TTL minus one renewal period, by its
// own monotonic clock: it stops believing before the database would let
// another replica take over, even if it cannot reach the database to find
// out.
//
// With SQLite, which one process serves, that process leads from Start to
// Stop whatever the lease row says: a restart after a crash must not wait
// for its dead predecessor's lease to expire. It still records itself as
// the holder, for the cluster status.
//
// On Postgres a replica restarted after a crash does not wait for its dead
// predecessor's lease either, when it can tell that the holder is that
// predecessor (see predecessorGone); any other holder keeps the lease until
// it expires.
type Leadership struct {
	db   *gorm.DB
	name string
	node string
	opts LeadershipOptions
	// single: the database serves one process (SQLite), which always leads.
	single bool
	proc   processInfo
	// created: the lease row is known to exist (touched only by the
	// goroutine that ticks).
	created bool
	// waitingFor is the earlier process whose silence this replica last
	// said it waits for (tick goroutine only; logs it once).
	waitingFor string

	mu sync.Mutex
	// renewedAt is when the last successful renewal started (zero: not
	// held). The belief lasts believeFor after it, measured with
	// time.Since, which uses the monotonic clock: a wall-clock step (NTP)
	// cannot extend it.
	renewedAt time.Time
	// running: between Start and Stop (what a single process's belief
	// rests on).
	running   bool
	listeners []func(bool)
	leading   bool

	stop, done chan struct{}
	started    atomic.Bool
	startOnce  sync.Once
	stopOnce   sync.Once
}

// NewLeadership returns node's claim on the lease called name. It does
// nothing until Start.
func NewLeadership(db *gorm.DB, name, node string, opts LeadershipOptions) *Leadership {
	return &Leadership{
		// Each write is one statement; gorm's default transaction around
		// it would make it three, every renewal.
		db:     db.Session(&gorm.Session{SkipDefaultTransaction: true}),
		name:   name,
		node:   node,
		opts:   opts.withDefaults(),
		single: db.Dialector.Name() != "postgres",
		proc:   thisProcess(),
		stop:   make(chan struct{}),
		done:   make(chan struct{}),
	}
}

// Start tries to take the lease at once, then keeps renewing or trying.
func (l *Leadership) Start() {
	l.startOnce.Do(func() {
		l.started.Store(true)
		l.mu.Lock()
		l.running = true
		l.mu.Unlock()
		next := l.tick()
		go l.run(next)
	})
}

// Stop releases the lease if this replica holds it, so another replica
// takes over at its next attempt instead of waiting for the TTL.
func (l *Leadership) Stop() {
	l.stopOnce.Do(func() {
		close(l.stop)
		if !l.started.Load() {
			return
		}
		<-l.done
		l.release()
	})
}

// IsLeader reports whether this replica holds the lease now. It is cheap:
// call it before every run of singleton work.
func (l *Leadership) IsLeader() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.believesLocked()
}

// believeFor is how long a renewal is believed: one renewal period short of
// the TTL, since the database lets others take the lease TTL after our
// write, which started after renewedAt.
func (l *Leadership) believeFor() time.Duration { return l.opts.TTL - l.opts.Renew }

func (l *Leadership) believesLocked() bool {
	if l.single {
		return l.running
	}
	return !l.renewedAt.IsZero() && time.Since(l.renewedAt) < l.believeFor()
}

// OnChange registers fn to be called (on the lease goroutine) when this
// replica gains (true) or loses (false) the lease.
func (l *Leadership) OnChange(fn func(leading bool)) {
	l.mu.Lock()
	l.listeners = append(l.listeners, fn)
	l.mu.Unlock()
}

// Holder returns the replica holding the lease and when it expires, by the
// database. An expired or missing lease returns "".
func Holder(ctx context.Context, db *gorm.DB, name string) (string, *time.Time, error) {
	var lease models.ClusterLease
	err := db.WithContext(ctx).Where("name = ? AND expires_at > ?", name, nowExpr(db)).Take(&lease).Error
	if err == gorm.ErrRecordNotFound {
		return "", nil, nil
	}
	if err != nil {
		return "", nil, err
	}
	return lease.Holder, &lease.ExpiresAt, nil
}

// run ticks every renewal period, or sooner when a tick asked to look again
// sooner (a predecessor about to count as gone).
func (l *Leadership) run(next time.Duration) {
	defer close(l.done)
	t := time.NewTimer(l.wait(next))
	defer t.Stop()
	for {
		select {
		case <-l.stop:
			return
		case <-t.C:
			t.Reset(l.wait(l.tick()))
		}
	}
}

func (l *Leadership) wait(retry time.Duration) time.Duration {
	if retry > 0 && retry < l.opts.Renew {
		return retry
	}
	return l.opts.Renew
}

// tick renews or takes the lease and tells the listeners about a change.
// It returns how soon to look again when that is sooner than usual (0:
// the renewal period).
func (l *Leadership) tick() time.Duration {
	started := time.Now()
	held, retry, err := l.acquire()
	if err != nil {
		logger.Warnf("Cluster lease %q: could not renew or take it: %v", l.name, err)
	}
	l.mu.Lock()
	if held {
		l.renewedAt = started
	} else if err == nil {
		l.renewedAt = time.Time{}
	}
	// On an error the belief simply runs out believeFor after the last
	// successful renewal.
	now := l.believesLocked()
	changed := now != l.leading
	l.leading = now
	listeners := append(make([]func(bool), 0, len(l.listeners)), l.listeners...)
	l.mu.Unlock()
	if changed {
		if now {
			logger.Infof("This replica (%s) now holds the %q lease", l.node, l.name)
		} else {
			logger.Infof("This replica (%s) no longer holds the %q lease", l.node, l.name)
		}
		for _, fn := range listeners {
			fn(now)
		}
	}
	return retry
}

// acquire renews the lease if this node holds it, or takes it if it is
// free or expired, in one conditional write. On SQLite it takes it
// whoever holds it. On Postgres, when another replica holds it, it takes
// it over if that replica is this one's dead predecessor; retry is how
// soon to look again when that may soon be known.
func (l *Leadership) acquire() (held bool, retry time.Duration, err error) {
	for attempt := 0; ; attempt++ {
		if !l.created {
			// The row, created expired and unheld if missing; concurrent
			// creators do not collide. The conditional update below does
			// the claiming. Once it exists it is never deleted, so this
			// runs once rather than on every renewal.
			if err := l.db.Exec(`INSERT INTO cluster_leases (name, holder, acquired_at, renewed_at, expires_at) VALUES (?, '', ?, ?, ?) ON CONFLICT (name) DO NOTHING`,
				l.name, nowExpr(l.db), nowExpr(l.db), sinceExpr(l.db, time.Second)).Error; err != nil {
				return false, 0, fmt.Errorf("create lease: %w", err)
			}
			l.created = true
		}
		q := l.db.Model(&models.ClusterLease{}).Where("name = ?", l.name)
		if !l.single {
			q = q.Where("holder = ? OR expires_at <= ?", l.node, nowExpr(l.db))
		}
		res := q.Updates(l.claim())
		if res.Error != nil {
			return false, 0, fmt.Errorf("renew lease: %w", res.Error)
		}
		if res.RowsAffected == 1 {
			return true, 0, nil
		}
		if l.single {
			// Only a missing row stops the claim: create it again.
			if attempt == 0 {
				l.created = false
				continue
			}
			return false, 0, nil
		}
		h, found, err := l.holder()
		if err != nil {
			return false, 0, fmt.Errorf("read lease: %w", err)
		}
		if !found {
			if attempt == 0 {
				l.created = false
				continue
			}
			return false, 0, nil
		}
		return l.takeOver(h)
	}
}

// claim is the update that makes this node the holder for another TTL.
func (l *Leadership) claim() map[string]interface{} {
	return map[string]interface{}{
		"acquired_at": gorm.Expr("CASE WHEN holder = ? THEN acquired_at ELSE ? END", l.node, nowExpr(l.db)),
		"holder":      l.node,
		"renewed_at":  nowExpr(l.db),
		"expires_at":  untilExpr(l.db, l.opts.TTL),
	}
}

// leaseHolder is the replica holding a lease, with its registration (all
// empty when it has none).
type leaseHolder struct {
	Holder       string `gorm:"column:holder"`
	Registered   bool   `gorm:"column:registered"`
	Hostname     string `gorm:"column:hostname"`
	BootID       string `gorm:"column:boot_id"`
	PIDNamespace string `gorm:"column:pid_namespace"`
	PID          int    `gorm:"column:pid"`
	// Silent is how long ago, in seconds by the database's clock, the
	// holder last refreshed its registration.
	Silent float64 `gorm:"column:silent"`
}

// holder reads the lease's holder and its registration in one statement
// (Postgres).
func (l *Leadership) holder() (leaseHolder, bool, error) {
	var h leaseHolder
	res := l.db.Raw(`SELECT l.holder, n.node_id IS NOT NULL AS registered,
		COALESCE(n.hostname, '') AS hostname, COALESCE(n.boot_id, '') AS boot_id,
		COALESCE(n.pid_namespace, '') AS pid_namespace, COALESCE(n.pid, 0) AS pid,
		COALESCE(EXTRACT(EPOCH FROM now() - n.last_seen), 0)::float8 AS silent
		FROM cluster_leases l LEFT JOIN cluster_nodes n ON n.node_id = l.holder
		WHERE l.name = ?`, l.name).Scan(&h)
	if res.Error != nil {
		return h, false, res.Error
	}
	return h, res.RowsAffected > 0, nil
}

// takeOver takes the lease from h if h is this replica's dead predecessor.
func (l *Leadership) takeOver(h leaseHolder) (bool, time.Duration, error) {
	gone, bySilence, why, retry := l.predecessorGone(h)
	if !gone {
		return false, retry, nil
	}
	// Conditional on the holder still being h (nobody took it meanwhile)
	// and, when silence is the evidence, on h still being silent.
	q := l.db.Model(&models.ClusterLease{}).Where("name = ? AND holder = ?", l.name, h.Holder)
	if bySilence {
		q = q.Where("NOT EXISTS (SELECT 1 FROM cluster_nodes WHERE node_id = ? AND last_seen > ?)", h.Holder, sinceExpr(l.db, l.opts.PredecessorSilence))
	}
	res := q.Updates(l.claim())
	if res.Error != nil {
		return false, 0, fmt.Errorf("take over lease: %w", res.Error)
	}
	if res.RowsAffected != 1 {
		return false, 0, nil
	}
	logger.Infof("Cluster lease %q: taken over from %s, an earlier Studio process on this host that is gone (%s), without waiting for the lease to expire", l.name, h.Holder, why)
	return true, 0, nil
}

// predecessorGone decides whether the lease holder h is a Studio process
// that ran on this host before this one and is gone (it crashed or was
// killed, so it never released the lease), in which case this replica may
// take the lease over instead of waiting up to the TTL. It must never
// answer yes for a live replica: two replicas would both lead. So it
// answers yes only on evidence local to this host:
//
//   - The holder registered from this machine (same kernel boot ID, which
//     differs between machines even when they share a hostname, and after
//     a reboot) and under this hostname. Anything else, or a holder with no
//     registration, keeps the lease until it expires.
//   - In the same pid namespace (bare metal, a VM, one container), the
//     holder's pid is checked directly: gone when no such process exists.
//     When the pid is this process's own, the holder is gone unless it is
//     a replica this process runs (tests run several in one process).
//   - In another pid namespace on the same machine and hostname, the pid
//     cannot be checked. That is what a container restarted in its pod
//     looks like (pid 1 again, new namespace), but also two live containers
//     given the same hostname. Silence tells them apart: a live replica
//     refreshes its registration every HeartbeatInterval, so a holder
//     silent for PredecessorSilence (two intervals by default) is gone.
//     retry says when that will be the case.
func (l *Leadership) predecessorGone(h leaseHolder) (gone, bySilence bool, why string, retry time.Duration) {
	me := l.proc
	if !h.Registered || h.Holder == l.node || me.BootID == "" || h.BootID != me.BootID || h.Hostname != me.Hostname {
		return false, false, "", 0
	}
	if h.PIDNamespace != "" && h.PIDNamespace == me.PIDNamespace {
		if h.PID == me.PID {
			if _, here := nodesHere.Load(h.Holder); here {
				return false, false, "", 0
			}
			return true, false, fmt.Sprintf("its pid %d is now this process", h.PID), 0
		}
		if h.PID > 0 && !processAlive(h.PID) {
			return true, false, fmt.Sprintf("process %d has exited", h.PID), 0
		}
		return false, false, "", 0
	}
	silent := time.Duration(h.Silent * float64(time.Second))
	if silent >= l.opts.PredecessorSilence {
		return true, true, fmt.Sprintf("it ran in another pid namespace and has been silent for %s", silent.Round(time.Second)), 0
	}
	if l.waitingFor != h.Holder {
		l.waitingFor = h.Holder
		logger.Infof("Cluster lease %q is held by %s, which ran on this host in another pid namespace (a previous container?); taking it over if it stays silent for %s", l.name, h.Holder, l.opts.PredecessorSilence)
	}
	return false, false, "", l.opts.PredecessorSilence - silent + 100*time.Millisecond
}

func (l *Leadership) release() {
	res := l.db.Model(&models.ClusterLease{}).Where("name = ? AND holder = ?", l.name, l.node).
		Update("expires_at", sinceExpr(l.db, time.Second))
	if res.Error != nil {
		logger.Warnf("Cluster lease %q: releasing it failed; another replica takes over when it expires: %v", l.name, res.Error)
	}
	l.mu.Lock()
	was := l.leading
	l.renewedAt = time.Time{}
	l.running = false
	l.leading = false
	listeners := append(make([]func(bool), 0, len(l.listeners)), l.listeners...)
	l.mu.Unlock()
	if was {
		for _, fn := range listeners {
			fn(false)
		}
	}
}

// untilExpr is the database's current time plus d.
func untilExpr(db *gorm.DB, d time.Duration) clause.Expr {
	if db.Dialector.Name() == "postgres" {
		return gorm.Expr("now() + make_interval(secs => ?)", d.Seconds())
	}
	return gorm.Expr("datetime('now', ?)", fmt.Sprintf("+%d seconds", int(d.Seconds())))
}
