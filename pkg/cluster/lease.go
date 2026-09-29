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
}

func (o LeadershipOptions) withDefaults() LeadershipOptions {
	if o.TTL <= 0 {
		o.TTL = 30 * time.Second
	}
	if o.Renew <= 0 || o.Renew >= o.TTL {
		o.Renew = o.TTL / 3
	}
	return o
}

// Leadership keeps this replica's claim on a named lease. The lease is
// taken and renewed with conditional writes against the database's clock,
// so at most one replica holds it. A replica believes it leads only until
// its last successful renewal plus the TTL minus one renewal period, by its
// own monotonic clock: it stops believing before the database would let
// another replica take over, even if it cannot reach the database to find
// out. With SQLite (one process) the single replica always leads.
type Leadership struct {
	db   *gorm.DB
	name string
	node string
	opts LeadershipOptions

	mu          sync.Mutex
	leaderUntil time.Time // local monotonic deadline of our belief
	listeners   []func(bool)
	leading     bool

	stop, done chan struct{}
	started    atomic.Bool
	startOnce  sync.Once
	stopOnce   sync.Once
}

// NewLeadership returns node's claim on the lease called name. It does
// nothing until Start.
func NewLeadership(db *gorm.DB, name, node string, opts LeadershipOptions) *Leadership {
	return &Leadership{db: db, name: name, node: node, opts: opts.withDefaults(), stop: make(chan struct{}), done: make(chan struct{})}
}

// Start tries to take the lease at once, then keeps renewing or trying.
func (l *Leadership) Start() {
	l.startOnce.Do(func() {
		l.started.Store(true)
		l.tick()
		go l.run()
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
	return time.Now().Before(l.leaderUntil)
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

func (l *Leadership) run() {
	defer close(l.done)
	t := time.NewTicker(l.opts.Renew)
	defer t.Stop()
	for {
		select {
		case <-l.stop:
			return
		case <-t.C:
			l.tick()
		}
	}
}

func (l *Leadership) tick() {
	started := time.Now()
	held, err := l.acquire()
	if err != nil {
		logger.Warnf("Cluster lease %q: could not renew or take it: %v", l.name, err)
	}
	l.mu.Lock()
	if held {
		// Believe it one renewal period short of the TTL: the database
		// lets others take it at TTL after our write, which started after
		// `started`.
		l.leaderUntil = started.Add(l.opts.TTL - l.opts.Renew)
	} else if err == nil {
		l.leaderUntil = time.Time{}
	}
	// On an error the belief simply runs out at leaderUntil.
	now := time.Now().Before(l.leaderUntil)
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
}

// acquire renews the lease if this node holds it, or takes it if it is
// free or expired, in one conditional write.
func (l *Leadership) acquire() (bool, error) {
	// The row, created expired and unheld if missing; concurrent creators
	// do not collide. The conditional update below does the claiming.
	if err := l.db.Exec(`INSERT INTO cluster_leases (name, holder, acquired_at, renewed_at, expires_at) VALUES (?, '', ?, ?, ?) ON CONFLICT (name) DO NOTHING`,
		l.name, nowExpr(l.db), nowExpr(l.db), sinceExpr(l.db, time.Second)).Error; err != nil {
		return false, fmt.Errorf("create lease: %w", err)
	}
	res := l.db.Model(&models.ClusterLease{}).
		Where("name = ? AND (holder = ? OR expires_at <= ?)", l.name, l.node, nowExpr(l.db)).
		Updates(map[string]interface{}{
			"acquired_at": gorm.Expr("CASE WHEN holder = ? THEN acquired_at ELSE ? END", l.node, nowExpr(l.db)),
			"holder":      l.node,
			"renewed_at":  nowExpr(l.db),
			"expires_at":  untilExpr(l.db, l.opts.TTL),
		})
	if res.Error != nil {
		return false, fmt.Errorf("renew lease: %w", res.Error)
	}
	return res.RowsAffected == 1, nil
}

func (l *Leadership) release() {
	res := l.db.Model(&models.ClusterLease{}).Where("name = ? AND holder = ?", l.name, l.node).
		Update("expires_at", sinceExpr(l.db, time.Second))
	if res.Error != nil {
		logger.Warnf("Cluster lease %q: releasing it failed; another replica takes over when it expires: %v", l.name, res.Error)
	}
	l.mu.Lock()
	was := l.leading
	l.leaderUntil = time.Time{}
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
