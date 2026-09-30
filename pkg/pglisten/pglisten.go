// Package pglisten shares one PostgreSQL LISTEN connection per database
// among every subscriber in the process. The connection is opened with pgx
// from the same connection string as the application's pool (gorm uses pgx
// too), so any DSN the pool accepts works here, but it lives outside the
// pool: subscribers never hold or wait for pool connections to hear
// notifications.
//
// NOTIFY is not a delivery guarantee: notifications sent while the listener
// is reconnecting are lost. A subscriber that must not miss anything keeps
// its data in a table, treats notifications as a hint to read it, and uses
// OnReconnect to read again after a gap (the cluster event log does this).
package pglisten

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/postgres"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgconn/ctxwatch"
)

// PingInterval is how long a listener waits without a notification before
// it checks its connection; a dead connection is otherwise only noticed
// when it is next used.
var PingInterval = 90 * time.Second

// commandTimeout bounds one LISTEN, UNLISTEN or ping on the connection.
const commandTimeout = 30 * time.Second

// Handler receives the payload of a notification on one channel. It is
// called from the dispatch goroutine and must not block.
type Handler func(payload string)

// Options configure the connection of a listener's first Acquire.
type Options struct {
	// ReconnectInterval is the first wait between reconnect attempts; the
	// listener backs off from it up to ReconnectCeiling.
	ReconnectInterval time.Duration
	ReconnectCeiling  time.Duration
	// ConnectTimeout bounds the wait for the first connection, and for each
	// reconnect attempt.
	ConnectTimeout time.Duration
	// Name labels the listener in logs.
	Name string
}

func (o Options) withDefaults() Options {
	if o.ReconnectInterval <= 0 {
		o.ReconnectInterval = time.Second
	}
	if o.ReconnectCeiling < o.ReconnectInterval {
		o.ReconnectCeiling = o.ReconnectInterval
	}
	if o.ConnectTimeout <= 0 {
		o.ConnectTimeout = 30 * time.Second
	}
	if o.Name == "" {
		o.Name = "PostgreSQL listener"
	}
	return o
}

// Subscription is one handler registered on a channel.
type Subscription struct {
	handler Handler
}

// Listener fans notifications from one connection out to subscribers.
//
// One goroutine owns the connection: it waits for notifications, issues
// every LISTEN and UNLISTEN, pings and reconnects, so nothing else ever
// touches the connection. Subscribe and Unsubscribe change the set of
// subscribed channels and wake the owner, which brings the connection's
// LISTENs in line with that set. Waking it cancels its wait for a
// notification; with pgx's deadline-based cancellation that leaves the
// connection intact (a partly read message is resumed on the next read).
type Listener struct {
	dsn    string
	name   string
	opts   Options
	config *pgx.ConnConfig
	refs   int // guarded by listenersMu

	// mu guards subs, gen and onReconnect. The owner holds the read lock
	// while it calls handlers, so an unsubscribe returns only once no
	// handler of it is running.
	mu          sync.RWMutex
	subs        map[string][]*Subscription
	gen         uint64 // bumped whenever the set of channels changes
	onReconnect map[int]func()
	nextHook    int

	// stateMu guards what the owner reports and how it is woken.
	stateMu   sync.Mutex
	connected bool
	applied   uint64           // the gen whose channels are LISTENed on the connection
	failed    map[string]error // channels whose LISTEN the server refused at applied
	changed   chan struct{}    // closed and replaced whenever the above change
	pending   bool             // the channel set changed since the owner last looked
	interrupt func()           // cancels the owner's wait for a notification

	stopCtx context.Context
	stop    context.CancelFunc
	done    chan struct{}
}

var (
	listenersMu sync.Mutex
	listeners   = map[string]*Listener{}
)

// DSN returns the connection string db was opened with, or the first
// non-empty fallback (a host may open db from a *sql.DB, which leaves no DSN
// on the dialector).
func DSN(db *gorm.DB, fallbacks ...string) (string, error) {
	if db != nil {
		if d, ok := db.Dialector.(*postgres.Dialector); ok && d.Config != nil && d.Config.DSN != "" {
			return d.Config.DSN, nil
		}
	}
	for _, f := range fallbacks {
		if f != "" {
			return f, nil
		}
	}
	return "", errors.New("cannot determine the PostgreSQL connection string for the listener: set DATABASE_URL")
}

// Acquire returns the listener for dsn, connecting it if this is its first
// user (opts apply only then). It waits at most opts.ConnectTimeout for the
// connection and fails at once when the connection is refused. Each Acquire
// needs a Release.
func Acquire(dsn string, opts Options) (*Listener, error) {
	listenersMu.Lock()
	defer listenersMu.Unlock()

	if l, ok := listeners[dsn]; ok {
		l.refs++
		return l, nil
	}
	opts = opts.withDefaults()

	// pgx.ParseConfig is what gorm's postgres driver parses the DSN with,
	// so the listener takes the same sslmode default (prefer), the same
	// options (default_query_exec_mode...) and the same runtime parameters
	// (search_path...) as the pool.
	config, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", opts.Name, err)
	}
	// Waking the owner cancels its wait for a notification. Deadline-based
	// cancellation (pgx's default, set here so a future default cannot
	// change it) only interrupts the read; a CancelRequest would not be
	// needed and the connection stays usable.
	config.BuildContextWatcherHandler = func(c *pgconn.PgConn) ctxwatch.Handler {
		return &pgconn.DeadlineContextWatcherHandler{Conn: c.Conn()}
	}

	ctx, cancel := context.WithTimeout(context.Background(), opts.ConnectTimeout)
	conn, err := pgx.ConnectConfig(ctx, config)
	timedOut := errors.Is(ctx.Err(), context.DeadlineExceeded)
	cancel()
	if err != nil {
		slog.Error(opts.Name+" connection failed", "error", err)
		if timedOut {
			return nil, fmt.Errorf("%s could not connect to PostgreSQL within %s: %w", opts.Name, opts.ConnectTimeout, err)
		}
		return nil, fmt.Errorf("%s could not connect to PostgreSQL: %w", opts.Name, err)
	}
	slog.Info(opts.Name + " connected")

	stopCtx, stop := context.WithCancel(context.Background())
	l := &Listener{
		dsn:         dsn,
		name:        opts.Name,
		opts:        opts,
		config:      config,
		refs:        1,
		subs:        map[string][]*Subscription{},
		onReconnect: map[int]func(){},
		changed:     make(chan struct{}),
		stopCtx:     stopCtx,
		stop:        stop,
		done:        make(chan struct{}),
	}
	go l.run(conn)
	listeners[dsn] = l
	return l, nil
}

// Release drops one reference; the last one closes the listener.
func (l *Listener) Release() {
	listenersMu.Lock()
	defer listenersMu.Unlock()

	l.refs--
	if l.refs > 0 {
		return
	}
	delete(listeners, l.dsn)
	l.stop()
	<-l.done
}

// Subscribe registers handler for channel, issuing LISTEN when it is the
// channel's first subscriber. It waits for the LISTEN (and, while the
// listener is reconnecting, for the connection) for at most timeout.
func (l *Listener) Subscribe(channel string, handler Handler, timeout time.Duration) (*Subscription, error) {
	sub := &Subscription{handler: handler}

	l.mu.Lock()
	first := len(l.subs[channel]) == 0
	l.subs[channel] = append(l.subs[channel], sub)
	if first {
		l.gen++
	}
	gen := l.gen
	l.mu.Unlock()

	if !first {
		return sub, nil
	}
	l.wake()
	if err := l.await(gen, channel, timeout, true); err != nil {
		// The owner drops the LISTEN again if it got that far, unless
		// someone else has subscribed to the channel since.
		l.mu.Lock()
		if l.removeLocked(channel, sub) {
			l.gen++
		}
		l.mu.Unlock()
		l.wake()
		return nil, fmt.Errorf("failed to listen to channel %s: %w", channel, err)
	}
	return sub, nil
}

// Unsubscribe removes sub and issues UNLISTEN when it was the channel's last
// subscriber. Once it returns, sub's handler is not running and will not run.
func (l *Listener) Unsubscribe(channel string, sub *Subscription, timeout time.Duration) {
	l.mu.Lock()
	last := l.removeLocked(channel, sub)
	if last {
		l.gen++
	}
	gen := l.gen
	l.mu.Unlock()

	if !last {
		return
	}
	l.wake()
	if err := l.await(gen, "", timeout, false); err != nil {
		slog.Warn("error unlistening from PostgreSQL channel", "channel", channel, "error", err)
	}
}

// OnReconnect registers fn to run after the listener's connection was lost
// and re-established, when any notification sent in between is gone. fn
// runs on the dispatch goroutine and must not block. The returned function
// removes it.
func (l *Listener) OnReconnect(fn func()) (remove func()) {
	l.mu.Lock()
	id := l.nextHook
	l.nextHook++
	l.onReconnect[id] = fn
	l.mu.Unlock()
	return func() {
		l.mu.Lock()
		delete(l.onReconnect, id)
		l.mu.Unlock()
	}
}

// removeLocked drops sub from channel and reports whether none are left.
// The caller holds mu.
func (l *Listener) removeLocked(channel string, sub *Subscription) bool {
	subs := l.subs[channel]
	for i, s := range subs {
		if s == sub {
			subs = append(subs[:i:i], subs[i+1:]...)
			break
		}
	}
	if len(subs) == 0 {
		delete(l.subs, channel)
		return true
	}
	l.subs[channel] = subs
	return false
}

// wake tells the owner the channel set changed.
func (l *Listener) wake() {
	l.stateMu.Lock()
	l.pending = true
	if l.interrupt != nil {
		l.interrupt()
	}
	l.stateMu.Unlock()
}

// await waits until the connection carries the channel set of gen and
// returns the server's error for channel, if it refused its LISTEN. With
// needConnected false (an UNLISTEN), a listener that is reconnecting
// counts as done: the new connection only LISTENs what is subscribed then.
func (l *Listener) await(gen uint64, channel string, timeout time.Duration, needConnected bool) error {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for {
		l.stateMu.Lock()
		connected, applied, changed := l.connected, l.applied, l.changed
		err := l.failed[channel]
		l.stateMu.Unlock()
		if connected && applied >= gen {
			return err
		}
		if !connected && !needConnected {
			return nil
		}
		select {
		case <-changed:
		case <-deadline.C:
			return fmt.Errorf("PostgreSQL listener did not answer within %s", timeout)
		case <-l.stopCtx.Done():
			return errors.New("PostgreSQL listener closed")
		}
	}
}

// setState updates what the owner reports and wakes every await.
func (l *Listener) setState(f func()) {
	l.stateMu.Lock()
	f()
	close(l.changed)
	l.changed = make(chan struct{})
	l.stateMu.Unlock()
}

var errStopped = errors.New("listener stopped")

// run owns the connection until Release.
func (l *Listener) run(conn *pgx.Conn) {
	defer close(l.done)
	listened := map[string]bool{}
	reconnected := false
	for {
		err := l.reconcile(conn, listened)
		if err == nil && reconnected {
			reconnected = false
			slog.Info(l.name + " reconnected")
			// Anything sent while the connection was down is gone.
			l.mu.RLock()
			for _, fn := range l.onReconnect {
				fn()
			}
			l.mu.RUnlock()
		}
		if err == nil {
			err = l.wait(conn)
		}
		if err == nil {
			continue
		}
		if l.stopCtx.Err() != nil {
			l.close(conn)
			return
		}
		slog.Warn(l.name+" disconnected; notifications sent until it reconnects are lost", "error", err)
		l.setState(func() { l.connected = false })
		l.close(conn)
		if conn = l.reconnect(); conn == nil {
			return
		}
		listened = map[string]bool{}
		reconnected = true
	}
}

// reconcile LISTENs every subscribed channel the connection is not
// listening to yet and UNLISTENs every channel nobody is subscribed to any
// more. It returns an error only when the connection is unusable.
func (l *Listener) reconcile(conn *pgx.Conn, listened map[string]bool) error {
	l.stateMu.Lock()
	l.pending = false
	l.stateMu.Unlock()

	l.mu.RLock()
	gen := l.gen
	want := make(map[string]bool, len(l.subs))
	for channel := range l.subs {
		want[channel] = true
	}
	l.mu.RUnlock()

	failed := map[string]error{}
	for channel := range want {
		if listened[channel] {
			continue
		}
		if err := l.exec(conn, "LISTEN "+pgx.Identifier{channel}.Sanitize()); err != nil {
			if !serverRefused(conn, err) {
				return err
			}
			failed[channel] = err
			continue
		}
		listened[channel] = true
	}
	for channel := range listened {
		if want[channel] {
			continue
		}
		if err := l.exec(conn, "UNLISTEN "+pgx.Identifier{channel}.Sanitize()); err != nil {
			if !serverRefused(conn, err) {
				return err
			}
			slog.Warn("error unlistening from PostgreSQL channel", "channel", channel, "error", err)
		}
		delete(listened, channel)
	}
	l.setState(func() {
		l.connected = true
		l.applied = gen
		l.failed = failed
	})
	return nil
}

// wait delivers notifications until the channel set changes (nil), the
// listener stops or the connection fails (an error).
func (l *Listener) wait(conn *pgx.Conn) error {
	for {
		ctx, cancel := context.WithTimeout(l.stopCtx, PingInterval)
		l.stateMu.Lock()
		if l.pending {
			l.stateMu.Unlock()
			cancel()
			return nil
		}
		l.interrupt = cancel
		l.stateMu.Unlock()

		n, err := conn.WaitForNotification(ctx)

		l.stateMu.Lock()
		l.interrupt = nil
		l.stateMu.Unlock()
		ctxErr := ctx.Err()
		cancel()

		if n != nil {
			l.dispatch(n)
		}
		switch {
		case err == nil:
		case l.stopCtx.Err() != nil:
			return errStopped
		case conn.IsClosed() || ctxErr == nil:
			// Not our cancellation: the connection failed.
			return err
		case errors.Is(ctxErr, context.DeadlineExceeded):
			// Quiet for PingInterval: make sure the connection is alive.
			pctx, pcancel := context.WithTimeout(l.stopCtx, commandTimeout)
			err := conn.Ping(pctx)
			pcancel()
			if err != nil {
				if l.stopCtx.Err() != nil {
					return errStopped
				}
				slog.Warn(l.name+" ping failed", "error", err)
				return err
			}
		default:
			// Woken by wake: the loop's next turn returns to reconcile.
		}
	}
}

func (l *Listener) dispatch(n *pgconn.Notification) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	for _, sub := range l.subs[n.Channel] {
		sub.handler(n.Payload)
	}
}

func (l *Listener) exec(conn *pgx.Conn, sql string) error {
	ctx, cancel := context.WithTimeout(l.stopCtx, commandTimeout)
	defer cancel()
	// PgConn's Exec is the simple protocol whatever the DSN's
	// default_query_exec_mode: nothing is prepared, which also suits
	// PgBouncer.
	_, err := conn.PgConn().Exec(ctx, sql).ReadAll()
	return err
}

// serverRefused reports whether err is the server rejecting a statement on
// a connection that is still usable (a LISTEN on a hot standby, say),
// rather than the connection failing.
func serverRefused(conn *pgx.Conn, err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && !conn.IsClosed()
}

// reconnect connects again, backing off from ReconnectInterval up to
// ReconnectCeiling. It returns nil once the listener is released.
func (l *Listener) reconnect() *pgx.Conn {
	wait := l.opts.ReconnectInterval
	for {
		select {
		case <-l.stopCtx.Done():
			return nil
		case <-time.After(wait):
		}
		ctx, cancel := context.WithTimeout(l.stopCtx, l.opts.ConnectTimeout)
		conn, err := pgx.ConnectConfig(ctx, l.config)
		cancel()
		if err == nil {
			return conn
		}
		if l.stopCtx.Err() != nil {
			return nil
		}
		slog.Error(l.name+" connection failed", "error", err)
		if wait *= 2; wait > l.opts.ReconnectCeiling {
			wait = l.opts.ReconnectCeiling
		}
	}
}

func (l *Listener) close(conn *pgx.Conn) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := conn.Close(ctx); err != nil {
		slog.Debug("error closing "+l.name, "error", err)
	}
}
