package cluster

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/dbhold"
	"github.com/TykTechnologies/midsommar/v2/pkg/safe"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm/clause"
)

// LeaderLease is the lease singleton background work runs under: at most
// one replica holds it, and jobs that must run once for the whole cluster
// (aggregations, alerts, syncs, cleanups) check Leadership.IsLeader first.
const LeaderLease = "leader"

// DefaultLeaseTTL is how long the leader lease lasts without renewal unless
// LeadershipOptions.TTL says otherwise.
const DefaultLeaseTTL = 30 * time.Second

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
	// ClockAllowance is how far the database's clock may differ after a
	// failover from what it was before (default 30 s): the new primary is
	// another machine. Only a lease that expired longer ago than this, by
	// the database's clock, is taken on that clock's word; see acquire.
	ClockAllowance time.Duration
}

// DefaultLeaseClockAllowance is the difference between the clocks of two
// database servers that a failover between them may bring without two
// replicas ever leading at once.
const DefaultLeaseClockAllowance = 30 * time.Second

func (o LeadershipOptions) withDefaults() LeadershipOptions {
	if o.TTL <= 0 {
		o.TTL = DefaultLeaseTTL
	}
	if o.Renew <= 0 || o.Renew >= o.TTL {
		o.Renew = o.TTL / 3
	}
	if o.PredecessorSilence <= 0 {
		o.PredecessorSilence = 2 * HeartbeatInterval
	}
	if o.ClockAllowance <= 0 {
		o.ClockAllowance = DefaultLeaseClockAllowance
	}
	return o
}

// Leadership keeps this replica's claim on a named lease. The lease is
// taken and renewed with conditional writes, so at most one replica holds
// it. A replica believes it leads only until its last successful renewal
// plus the TTL minus one renewal period, by its own monotonic clock: it
// stops believing before another replica may take over, even if it cannot
// reach the database to find out.
//
// When another replica may take a lease its holder stopped renewing is
// decided by that replica's own monotonic clock too, not by the database's:
// it takes the lease once it has watched the same renewal sit unchanged for
// longer than the holder believes in it (see acquire). The database's clock
// is not one clock: after a failover it is another machine's, seconds ahead
// or behind, and a lease judged by it would be taken from a live holder
// (clock ahead) or left without one for longer than the TTL (clock behind).
//
// Why two replicas never both believe they hold it: the holder's belief
// runs from the moment it started its last successful renewal write, for
// believeFor; the taker's wait runs from the moment it first read that
// renewal, which is after the write committed and so after the holder
// started it, for takeAfter, which is longer. Each is measured on its own
// machine's monotonic clock. The wait therefore ends after the belief
// unless the two clocks' rates differ by more than the difference between
// takeAfter and believeFor (a twentieth of a renewal period: 2.4% with the
// defaults); oscillators differ by parts per million. No wall clock and no
// database clock is in the argument.
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
	// watched is the renewal of another holder this replica last saw, and
	// watchedSince when it first saw it (tick goroutine only). time.Since
	// on it uses the monotonic clock.
	watched      string
	watchedSince time.Time
	// silentSeen is the registration (holder and last refresh) of a
	// holder in another pid namespace this replica last saw, and
	// silentSince when it first saw it (tick goroutine only).
	silentSeen  string
	silentSince time.Time

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
	db = dbhold.Unheld(db) // a renewal must not wait past the lease's term
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
		var next time.Duration
		if safe.Call("cluster lease "+l.name, func() { next = l.tick() }) {
			l.dropBelief()
		}
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
	// After a panic the replica cannot vouch for its renewals: it stops
	// believing it leads at once, rather than when the belief would run out.
	safe.Loop("cluster lease "+l.name, l.stop, func() {
		l.renewLoop(next)
	}, safe.AfterPanic(func() {
		next = 0
		l.dropBelief()
	}))
}

// dropBelief ends this replica's belief that it holds the lease (not on
// SQLite, where the one process always leads) and tells the listeners.
func (l *Leadership) dropBelief() {
	l.mu.Lock()
	l.renewedAt = time.Time{}
	now := l.believesLocked()
	changed := now != l.leading
	l.leading = now
	listeners := append(make([]func(bool), 0, len(l.listeners)), l.listeners...)
	l.mu.Unlock()
	if changed {
		logger.Warnf("This replica (%s) no longer holds the %q lease: its renewals stopped after a panic", l.node, l.name)
		for _, fn := range listeners {
			safe.Call("cluster lease listener", func() { fn(now) })
		}
	}
}

func (l *Leadership) renewLoop(next time.Duration) {
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
// free, each in one conditional write. On SQLite it takes it whoever holds
// it. On Postgres the lease of another replica is free for the taking when:
//
//   - nobody holds it: it was never taken, or its holder released it (Stop);
//   - it expired more than ClockAllowance ago by the database's clock: no
//     clock difference a failover brings explains that, so its holder
//     stopped long ago (a cluster starting after everything was down);
//   - this replica has watched the same renewal (holder and renewal time)
//     for longer than a holder believes in one (takeAfter), by its own
//     monotonic clock. A live holder renews every Renew, which changes what
//     is watched and starts the wait again; one that cannot reach the
//     database stopped believing before the wait ends. This holds whatever
//     the database's clock says, and whichever server answers;
//   - its holder is this replica's dead predecessor on this host
//     (predecessorGone).
//
// retry is how soon to look again when the answer may change sooner than
// the next renewal period.
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
			// Renew, or take a lease nobody holds.
			q = q.Where("holder = ? OR holder = ''", l.node)
		}
		res := q.Updates(l.claim())
		if res.Error != nil {
			return false, 0, fmt.Errorf("renew lease: %w", res.Error)
		}
		if res.RowsAffected == 1 {
			l.watched = ""
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
		held, retry, err := l.takeOver(h)
		if held || err != nil {
			return held, retry, err
		}
		if wait := l.takeUnrenewed(h); wait >= 0 {
			// Not yet: look again when the wait is over, or sooner if the
			// predecessor check asked for that.
			if retry <= 0 || wait < retry {
				retry = wait
			}
			return false, retry, nil
		}
		return l.takeFrom(h)
	}
}

// takeAfter is how long this replica watches another holder's renewal
// before it takes the lease: as long as the holder believes in that renewal
// (it started the renewal before this replica could see it), and a twentieth
// of a renewal period more for the difference between two machines' clock
// rates.
func (l *Leadership) takeAfter() time.Duration { return l.believeFor() + l.opts.Renew/20 }

// takeUnrenewed reports how long to wait before h's lease may be taken as
// no longer renewed (at least a millisecond), or -1 when it may be taken
// now.
func (l *Leadership) takeUnrenewed(h leaseHolder) time.Duration {
	if h.ExpiredFor >= l.opts.ClockAllowance.Seconds() {
		return -1
	}
	if h.Renewal != l.watched {
		l.watched, l.watchedSince = h.Renewal, time.Now()
	}
	if wait := l.takeAfter() - time.Since(l.watchedSince); wait > 0 {
		return max(wait, time.Millisecond)
	}
	return -1
}

// takeFrom takes the lease from h on the condition that it is still the
// renewal that was read: a holder that renewed meanwhile keeps it, and of
// two replicas taking it at once one succeeds.
func (l *Leadership) takeFrom(h leaseHolder) (bool, time.Duration, error) {
	res := l.db.Model(&models.ClusterLease{}).
		Where("name = ? AND holder = ? AND renewed_at::text = ?", l.name, h.Holder, h.RenewedAt).
		Updates(l.claim())
	if res.Error != nil {
		return false, 0, fmt.Errorf("take lease: %w", res.Error)
	}
	if res.RowsAffected != 1 {
		return false, 0, nil
	}
	l.watched = ""
	logger.Infof("Cluster lease %q: taken from %s, which stopped renewing it", l.name, h.Holder)
	return true, 0, nil
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
	// RenewedAt is the lease's renewal time as the database prints it, and
	// Renewal what identifies this renewal: the holder with that time.
	RenewedAt string `gorm:"column:renewed"`
	Renewal   string `gorm:"-"`
	// Seen identifies the holder's last refresh of its registration.
	Seen string `gorm:"column:seen"`
	// ExpiredFor is how long ago, in seconds by the database's clock, the
	// lease expired (negative: it has not).
	ExpiredFor float64 `gorm:"column:expired_for"`
}

// holder reads the lease's holder and its registration in one statement
// (Postgres).
func (l *Leadership) holder() (leaseHolder, bool, error) {
	var h leaseHolder
	res := l.db.Raw(`SELECT l.holder, n.node_id IS NOT NULL AS registered,
		COALESCE(n.hostname, '') AS hostname, COALESCE(n.boot_id, '') AS boot_id,
		COALESCE(n.pid_namespace, '') AS pid_namespace, COALESCE(n.pid, 0) AS pid,
		COALESCE(EXTRACT(EPOCH FROM now() - n.last_seen), 0)::float8 AS silent,
		COALESCE(l.renewed_at::text, '') AS renewed,
		l.holder || '@' || COALESCE(n.last_seen::text, '') AS seen,
		EXTRACT(EPOCH FROM now() - l.expires_at)::float8 AS expired_for
		FROM cluster_leases l LEFT JOIN cluster_nodes n ON n.node_id = l.holder
		WHERE l.name = ?`, l.name).Scan(&h)
	if res.Error != nil {
		return h, false, res.Error
	}
	h.Renewal = h.Holder + "@" + h.RenewedAt
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
	l.watched = ""
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
//     The silence must be this replica's own observation (it watched the
//     registration go unrefreshed that long), unless the database's clock
//     puts it beyond PredecessorSilence plus ClockAllowance: after a
//     failover the database's clock may be ahead of the one that stamped
//     the registration. retry says when that will be the case.
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
	// Silence by the database's clock alone is evidence only when no clock
	// difference a failover brings explains it. Short of that, this replica
	// must itself have watched the holder's registration go unrefreshed
	// for PredecessorSilence, by its own clock: after a failover to a
	// server whose clock is ahead, a live replica looks silent until its
	// next refresh.
	if h.Seen != l.silentSeen {
		l.silentSeen, l.silentSince = h.Seen, time.Now()
	}
	watched := time.Since(l.silentSince)
	if silent >= l.opts.PredecessorSilence+l.opts.ClockAllowance || (silent >= l.opts.PredecessorSilence && watched >= l.opts.PredecessorSilence) {
		return true, true, fmt.Sprintf("it ran in another pid namespace and has been silent for %s", silent.Round(time.Second)), 0
	}
	if silent >= l.opts.PredecessorSilence {
		// Silent by the database's clock; not yet watched for long enough.
		return false, false, "", l.opts.PredecessorSilence - watched + 100*time.Millisecond
	}
	if l.waitingFor != h.Holder {
		l.waitingFor = h.Holder
		logger.Infof("Cluster lease %q is held by %s, which ran on this host in another pid namespace (a previous container?); taking it over if it stays silent for %s", l.name, h.Holder, l.opts.PredecessorSilence)
	}
	return false, false, "", l.opts.PredecessorSilence - silent + 100*time.Millisecond
}

func (l *Leadership) release() {
	// Nobody holds it now: the next replica to look takes it at once. (The
	// expiry in the past is what replicas of an earlier version go by.)
	res := l.db.Model(&models.ClusterLease{}).Where("name = ? AND holder = ?", l.name, l.node).
		Updates(map[string]interface{}{"holder": "", "expires_at": sinceExpr(l.db, time.Second)})
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
