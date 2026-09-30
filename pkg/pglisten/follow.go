package pglisten

import (
	"log/slog"
	"sync"
	"time"
)

// FollowRetryInterval is how often a Follower that could not subscribe
// tries again.
var FollowRetryInterval = 30 * time.Second

// followSubscribeTimeout bounds a Follower's wait for its LISTEN.
const followSubscribeTimeout = 30 * time.Second

// Follower keeps one channel subscribed for a service that also polls the
// database, so notifications only make it react sooner (the cluster event
// log, edge push delivery). Such a service must not fail, or hang, because
// the listener cannot connect: Follow never fails. When the listener cannot
// connect or subscribe at first, the service runs on its poll alone and
// the Follower keeps trying in the background.
type Follower struct {
	dsn         string
	channel     string
	opts        Options
	handler     Handler
	onReconnect func()

	mu       sync.Mutex
	listener *Listener
	sub      *Subscription
	unhook   func()

	stop      chan struct{}
	done      chan struct{}
	closeOnce sync.Once
}

// Follow subscribes handler to channel on dsn's listener. onReconnect runs
// whenever notifications may have been missed: after the listener
// reconnects (on its dispatch goroutine), and when a subscription that
// failed at first is made later (on the Follower's own goroutine). It must
// not block. If subscribing fails now, Follow logs
// a warning and retries every FollowRetryInterval until Close.
func Follow(dsn, channel string, opts Options, handler Handler, onReconnect func()) *Follower {
	opts = opts.withDefaults()
	f := &Follower{
		dsn:         dsn,
		channel:     channel,
		opts:        opts,
		handler:     handler,
		onReconnect: onReconnect,
		stop:        make(chan struct{}),
		done:        make(chan struct{}),
	}
	err := f.subscribe()
	if err == nil {
		close(f.done)
		return f
	}
	slog.Warn(opts.Name+" is not available; relying on polling until it is (retrying every "+FollowRetryInterval.String()+")", "error", err)
	go f.retry()
	return f
}

// Listening reports whether the channel is subscribed.
func (f *Follower) Listening() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.listener != nil
}

// Close ends the subscription, or the attempts to make it.
func (f *Follower) Close() {
	f.closeOnce.Do(func() {
		close(f.stop)
		<-f.done
		f.mu.Lock()
		l, sub, unhook := f.listener, f.sub, f.unhook
		f.listener, f.sub, f.unhook = nil, nil, nil
		f.mu.Unlock()
		if l != nil {
			unhook()
			l.Unsubscribe(f.channel, sub, 10*time.Second)
			l.Release()
		}
	})
}

func (f *Follower) subscribe() error {
	l, err := Acquire(f.dsn, f.opts)
	if err != nil {
		return err
	}
	sub, err := l.Subscribe(f.channel, f.handler, followSubscribeTimeout)
	if err != nil {
		l.Release()
		return err
	}
	unhook := l.OnReconnect(f.onReconnect)
	f.mu.Lock()
	f.listener, f.sub, f.unhook = l, sub, unhook
	f.mu.Unlock()
	return nil
}

func (f *Follower) retry() {
	defer close(f.done)
	for {
		select {
		case <-f.stop:
			return
		case <-time.After(FollowRetryInterval):
		}
		if err := f.subscribe(); err != nil {
			slog.Debug(f.opts.Name+" is still not available", "error", err)
			continue
		}
		slog.Info(f.opts.Name + " is available again; notifications resume")
		// Whatever was published while it was missing was only polled for.
		f.onReconnect()
		return
	}
}
