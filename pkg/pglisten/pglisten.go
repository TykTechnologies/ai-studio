// Package pglisten shares one PostgreSQL LISTEN connection per database
// among every subscriber in the process. The connection is a pq.Listener
// opened outside the application's pool, so subscribers never hold or wait
// for pool connections to hear notifications.
//
// NOTIFY is not a delivery guarantee: notifications sent while the listener
// is reconnecting are lost. A subscriber that must not miss anything keeps
// its data in a table, treats notifications as a hint to read it, and uses
// OnReconnect to read again after a gap (the cluster event log does this).
package pglisten

import (
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/postgres"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/lib/pq"
)

// PingInterval is how often a listener checks its connection; pq.Listener
// only notices a dead connection when it next uses it.
var PingInterval = 90 * time.Second

// Handler receives the payload of a notification on one channel. It is
// called from the dispatch goroutine and must not block.
type Handler func(payload string)

// Options configure the connection of a listener's first Acquire.
type Options struct {
	// ReconnectInterval is the first wait between reconnect attempts;
	// pq.Listener backs off from it up to ReconnectCeiling.
	ReconnectInterval time.Duration
	ReconnectCeiling  time.Duration
	// ConnectTimeout bounds the wait for the first connection.
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

// Listener fans notifications from one pq.Listener out to subscribers.
type Listener struct {
	dsn      string
	name     string
	listener *pq.Listener
	refs     int

	// mu guards subs and onReconnect. dispatch holds the read lock while it
	// calls handlers, so an unsubscribe returns only once no handler of it is
	// running.
	mu          sync.RWMutex
	subs        map[string][]*Subscription
	onReconnect map[int]func()
	nextHook    int

	// ctlMu orders LISTEN and UNLISTEN, so a channel's last subscriber
	// leaving cannot UNLISTEN after a new first subscriber's LISTEN.
	ctlMu sync.Mutex

	stop chan struct{}
	done chan struct{}
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
// connection. Each Acquire needs a Release.
func Acquire(dsn string, opts Options) (*Listener, error) {
	listenersMu.Lock()
	defer listenersMu.Unlock()

	if l, ok := listeners[dsn]; ok {
		l.refs++
		return l, nil
	}
	opts = opts.withDefaults()

	firstEvent := make(chan error, 1)
	var once sync.Once
	l := &Listener{
		dsn:         dsn,
		name:        opts.Name,
		refs:        1,
		subs:        map[string][]*Subscription{},
		onReconnect: map[int]func(){},
		stop:        make(chan struct{}),
		done:        make(chan struct{}),
	}
	l.listener = pq.NewListener(dsn, opts.ReconnectInterval, opts.ReconnectCeiling, func(ev pq.ListenerEventType, err error) {
		switch ev {
		case pq.ListenerEventConnected:
			slog.Info(opts.Name + " connected")
			once.Do(func() { firstEvent <- nil })
		case pq.ListenerEventDisconnected:
			slog.Warn(opts.Name+" disconnected; notifications sent until it reconnects are lost", "error", err)
		case pq.ListenerEventReconnected:
			slog.Info(opts.Name + " reconnected")
		case pq.ListenerEventConnectionAttemptFailed:
			slog.Error(opts.Name+" connection failed", "error", err)
			once.Do(func() { firstEvent <- err })
		}
	})

	select {
	case err := <-firstEvent:
		if err != nil {
			l.listener.Close()
			return nil, fmt.Errorf("%s could not connect to PostgreSQL: %w", opts.Name, err)
		}
	case <-time.After(opts.ConnectTimeout):
		l.listener.Close()
		return nil, fmt.Errorf("%s could not connect to PostgreSQL within %s", opts.Name, opts.ConnectTimeout)
	}

	go l.dispatch()
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
	close(l.stop)
	<-l.done
	if err := l.listener.Close(); err != nil {
		slog.Warn("error closing "+l.name, "error", err)
	}
}

// Subscribe registers handler for channel, issuing LISTEN when it is the
// channel's first subscriber. It gives up after timeout.
func (l *Listener) Subscribe(channel string, handler Handler, timeout time.Duration) (*Subscription, error) {
	sub := &Subscription{handler: handler}

	l.ctlMu.Lock()
	defer l.ctlMu.Unlock()

	l.mu.Lock()
	first := len(l.subs[channel]) == 0
	l.subs[channel] = append(l.subs[channel], sub)
	l.mu.Unlock()

	if !first {
		return sub, nil
	}
	if err := l.withTimeout(timeout, func() error { return l.listener.Listen(channel) }, func() {
		// LISTEN completed after we gave up: undo it unless someone else
		// has subscribed to the channel since.
		l.mu.RLock()
		inUse := len(l.subs[channel]) > 0
		l.mu.RUnlock()
		if !inUse {
			_ = l.listener.Unlisten(channel)
		}
	}); err != nil && !errors.Is(err, pq.ErrChannelAlreadyOpen) {
		l.mu.Lock()
		l.removeLocked(channel, sub)
		l.mu.Unlock()
		return nil, fmt.Errorf("failed to listen to channel %s: %w", channel, err)
	}
	return sub, nil
}

// Unsubscribe removes sub and issues UNLISTEN when it was the channel's last
// subscriber. Once it returns, sub's handler is not running and will not run.
func (l *Listener) Unsubscribe(channel string, sub *Subscription, timeout time.Duration) {
	l.ctlMu.Lock()
	defer l.ctlMu.Unlock()

	l.mu.Lock()
	last := l.removeLocked(channel, sub)
	l.mu.Unlock()

	if !last {
		return
	}
	if err := l.withTimeout(timeout, func() error { return l.listener.Unlisten(channel) }, nil); err != nil && !errors.Is(err, pq.ErrChannelNotOpen) {
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

// withTimeout runs op, which may block while the listener reconnects, for at
// most timeout. late runs if op finishes successfully after the timeout.
func (l *Listener) withTimeout(timeout time.Duration, op func() error, late func()) error {
	result := make(chan error, 1)
	var mu sync.Mutex
	gaveUp := false
	go func() {
		err := op()
		mu.Lock()
		defer mu.Unlock()
		if gaveUp {
			if err == nil && late != nil {
				late()
			}
			return
		}
		result <- err
	}()
	select {
	case err := <-result:
		return err
	case <-time.After(timeout):
		mu.Lock()
		defer mu.Unlock()
		select {
		case err := <-result:
			return err
		default:
		}
		gaveUp = true
		return fmt.Errorf("PostgreSQL listener did not answer within %s", timeout)
	}
}

func (l *Listener) dispatch() {
	defer close(l.done)
	ping := time.NewTicker(PingInterval)
	defer ping.Stop()
	for {
		select {
		case <-l.stop:
			return
		case <-ping.C:
			go func() {
				if err := l.listener.Ping(); err != nil {
					slog.Warn(l.name+" ping failed", "error", err)
				}
			}()
		case n := <-l.listener.Notify:
			// nil means the connection was re-established; anything sent
			// while it was down is gone.
			if n == nil {
				l.mu.RLock()
				for _, fn := range l.onReconnect {
					fn()
				}
				l.mu.RUnlock()
				continue
			}
			l.mu.RLock()
			for _, sub := range l.subs[n.Channel] {
				sub.handler(n.Extra)
			}
			l.mu.RUnlock()
		}
	}
}
