package chat_session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/TykTechnologies/midsommar/v2/config"
	"github.com/lib/pq"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Every PostgreSQL queue in the process receives its notifications through one
// shared LISTEN connection per database, opened with pq.Listener outside the
// application's pool. A session only registers its channels on it, so the
// number of sessions no longer decides how many connections are held, and a
// session never waits on the pool to start listening.

// listenerPingInterval is how often the shared listener checks its connection;
// pq.Listener only notices a dead connection when it next uses it.
const listenerPingInterval = 90 * time.Second

// notificationHandler receives the payload of a notification on one channel.
// It is called from the dispatch goroutine and must not block.
type notificationHandler func(payload string)

type listenerSubscription struct {
	handler notificationHandler
}

// sharedListener fans notifications from one pq.Listener out to subscribers.
type sharedListener struct {
	dsn      string
	listener *pq.Listener
	refs     int

	// mu guards subs. dispatch holds the read lock while it calls handlers,
	// so an unsubscribe returns only once no handler of it is running.
	mu   sync.RWMutex
	subs map[string][]*listenerSubscription

	// ctlMu orders LISTEN and UNLISTEN, so a channel's last subscriber
	// leaving cannot UNLISTEN after a new first subscriber's LISTEN.
	ctlMu sync.Mutex

	stop chan struct{}
	done chan struct{}
}

var (
	sharedListenersMu sync.Mutex
	sharedListeners   = map[string]*sharedListener{}
)

// postgresDSN returns the connection string db was opened with, falling back
// to the configured database URL (a host may open db from a *sql.DB, which
// leaves no DSN on the dialector) and then to DATABASE_URL.
func postgresDSN(db *gorm.DB) (string, error) {
	if db != nil {
		if d, ok := db.Dialector.(*postgres.Dialector); ok && d.Config != nil && d.Config.DSN != "" {
			return d.Config.DSN, nil
		}
	}
	if dsn := config.Get("").DatabaseURL; dsn != "" {
		return dsn, nil
	}
	if dsn := os.Getenv("DATABASE_URL"); dsn != "" {
		return dsn, nil
	}
	return "", errors.New("cannot determine the PostgreSQL connection string for the queue listener: set DATABASE_URL")
}

// acquireSharedListener returns the listener for dsn, connecting it if this is
// its first user. It waits at most the queue timeout for the connection.
func acquireSharedListener(dsn string, config PostgreSQLConfig) (*sharedListener, error) {
	sharedListenersMu.Lock()
	defer sharedListenersMu.Unlock()

	if sl, ok := sharedListeners[dsn]; ok {
		sl.refs++
		return sl, nil
	}

	if config.ReconnectInterval <= 0 {
		config.ReconnectInterval = DefaultPostgreSQLConfig().ReconnectInterval
	}

	firstEvent := make(chan error, 1)
	var once sync.Once
	listener := pq.NewListener(dsn, config.ReconnectInterval, reconnectCeiling(config), func(ev pq.ListenerEventType, err error) {
		switch ev {
		case pq.ListenerEventConnected:
			slog.Info("PostgreSQL queue listener connected")
			once.Do(func() { firstEvent <- nil })
		case pq.ListenerEventDisconnected:
			slog.Warn("PostgreSQL queue listener disconnected; notifications sent until it reconnects are lost", "error", err)
		case pq.ListenerEventReconnected:
			slog.Info("PostgreSQL queue listener reconnected")
		case pq.ListenerEventConnectionAttemptFailed:
			slog.Error("PostgreSQL queue listener connection failed", "error", err)
			once.Do(func() { firstEvent <- err })
		}
	})

	timeout := queueTimeout(config)
	select {
	case err := <-firstEvent:
		if err != nil {
			listener.Close()
			return nil, fmt.Errorf("queue listener could not connect to PostgreSQL: %w", err)
		}
	case <-time.After(timeout):
		listener.Close()
		return nil, fmt.Errorf("queue listener could not connect to PostgreSQL within %s", timeout)
	}

	sl := &sharedListener{
		dsn:      dsn,
		listener: listener,
		refs:     1,
		subs:     map[string][]*listenerSubscription{},
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
	}
	go sl.dispatch()
	sharedListeners[dsn] = sl
	return sl, nil
}

// release drops one reference; the last one closes the listener.
func (sl *sharedListener) release() {
	sharedListenersMu.Lock()
	defer sharedListenersMu.Unlock()

	sl.refs--
	if sl.refs > 0 {
		return
	}
	delete(sharedListeners, sl.dsn)
	close(sl.stop)
	<-sl.done
	if err := sl.listener.Close(); err != nil {
		slog.Warn("error closing PostgreSQL queue listener", "error", err)
	}
}

// subscribe registers handler for channel, issuing LISTEN when it is the
// channel's first subscriber. It gives up after timeout.
func (sl *sharedListener) subscribe(channel string, handler notificationHandler, timeout time.Duration) (*listenerSubscription, error) {
	sub := &listenerSubscription{handler: handler}

	sl.ctlMu.Lock()
	defer sl.ctlMu.Unlock()

	sl.mu.Lock()
	first := len(sl.subs[channel]) == 0
	sl.subs[channel] = append(sl.subs[channel], sub)
	sl.mu.Unlock()

	if !first {
		return sub, nil
	}
	if err := sl.withTimeout(timeout, func() error { return sl.listener.Listen(channel) }, func() {
		// LISTEN completed after we gave up: undo it unless someone else
		// has subscribed to the channel since.
		sl.mu.RLock()
		inUse := len(sl.subs[channel]) > 0
		sl.mu.RUnlock()
		if !inUse {
			_ = sl.listener.Unlisten(channel)
		}
	}); err != nil && !errors.Is(err, pq.ErrChannelAlreadyOpen) {
		sl.mu.Lock()
		sl.removeLocked(channel, sub)
		sl.mu.Unlock()
		return nil, fmt.Errorf("failed to listen to channel %s: %w", channel, err)
	}
	return sub, nil
}

// removeLocked drops sub from channel and reports whether none are left.
// The caller holds mu.
func (sl *sharedListener) removeLocked(channel string, sub *listenerSubscription) bool {
	subs := sl.subs[channel]
	for i, s := range subs {
		if s == sub {
			subs = append(subs[:i:i], subs[i+1:]...)
			break
		}
	}
	if len(subs) == 0 {
		delete(sl.subs, channel)
		return true
	}
	sl.subs[channel] = subs
	return false
}

// unsubscribe removes sub and issues UNLISTEN when it was the channel's last
// subscriber. Once it returns, sub's handler is not running and will not run.
func (sl *sharedListener) unsubscribe(channel string, sub *listenerSubscription, timeout time.Duration) {
	sl.ctlMu.Lock()
	defer sl.ctlMu.Unlock()

	sl.mu.Lock()
	last := sl.removeLocked(channel, sub)
	sl.mu.Unlock()

	if !last {
		return
	}
	if err := sl.withTimeout(timeout, func() error { return sl.listener.Unlisten(channel) }, nil); err != nil && !errors.Is(err, pq.ErrChannelNotOpen) {
		slog.Warn("error unlistening from PostgreSQL channel", "channel", channel, "error", err)
	}
}

// withTimeout runs op, which may block while the listener reconnects, for at
// most timeout. late runs if op finishes successfully after the timeout.
func (sl *sharedListener) withTimeout(timeout time.Duration, op func() error, late func()) error {
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

func (sl *sharedListener) dispatch() {
	defer close(sl.done)
	ping := time.NewTicker(listenerPingInterval)
	defer ping.Stop()
	for {
		select {
		case <-sl.stop:
			return
		case <-ping.C:
			go func() {
				if err := sl.listener.Ping(); err != nil {
					slog.Warn("PostgreSQL queue listener ping failed", "error", err)
				}
			}()
		case n := <-sl.listener.Notify:
			// nil means the connection was re-established; anything sent
			// while it was down is gone.
			if n == nil {
				continue
			}
			sl.mu.RLock()
			for _, sub := range sl.subs[n.Channel] {
				sub.handler(n.Extra)
			}
			sl.mu.RUnlock()
		}
	}
}

// channelSubscription is one session channel registered on a shared listener.
type channelSubscription struct {
	channel string
	sub     *listenerSubscription
}

// subscribeSessionChannels registers handler for every channel. On error the
// subscriptions made so far are returned so the caller can release them.
func subscribeSessionChannels(sl *sharedListener, channels []string, handler notificationHandler, timeout time.Duration) ([]channelSubscription, error) {
	subs := make([]channelSubscription, 0, len(channels))
	for _, channel := range channels {
		sub, err := sl.subscribe(channel, handler, timeout)
		if err != nil {
			return subs, err
		}
		subs = append(subs, channelSubscription{channel: channel, sub: sub})
	}
	return subs, nil
}

// unsubscribeSessionChannels undoes subscribeSessionChannels.
func unsubscribeSessionChannels(sl *sharedListener, subs []channelSubscription, timeout time.Duration) {
	if sl == nil {
		return
	}
	for _, s := range subs {
		sl.unsubscribe(s.channel, s.sub, timeout)
	}
}

// routePostgreSQLNotification decodes a queue notification and hands it to
// the matching local channel, dropping it when that channel is full.
func routePostgreSQLNotification(payload, sessionID string, messages chan *ChatResponse, stream chan []byte, errs chan error, llmResponses chan *LLMResponseWrapper) error {
	// Deserialize the message
	var pgMsg PostgreSQLMessage
	if err := json.Unmarshal([]byte(payload), &pgMsg); err != nil {
		return fmt.Errorf("failed to unmarshal notification: %w", err)
	}

	// Route to appropriate channel based on message type
	switch pgMsg.Type {
	case PostgreSQLMessageTypeChatResponse:
		var chatResp ChatResponse
		if err := json.Unmarshal(pgMsg.Data, &chatResp); err != nil {
			return fmt.Errorf("failed to unmarshal ChatResponse: %w", err)
		}

		select {
		case messages <- &chatResp:
		default:
			slog.Warn("message channel full, dropping message", "session_id", sessionID)
		}

	case PostgreSQLMessageTypeStream:
		var streamData []byte
		if err := json.Unmarshal(pgMsg.Data, &streamData); err != nil {
			return fmt.Errorf("failed to unmarshal stream data: %w", err)
		}

		select {
		case stream <- streamData:
		default:
			slog.Warn("stream channel full, dropping data", "session_id", sessionID)
		}

	case PostgreSQLMessageTypeError:
		var errorStr string
		if err := json.Unmarshal(pgMsg.Data, &errorStr); err != nil {
			return fmt.Errorf("failed to unmarshal error: %w", err)
		}

		select {
		case errs <- fmt.Errorf("%s", errorStr):
		default:
			slog.Warn("error channel full, dropping error", "session_id", sessionID)
		}

	case PostgreSQLMessageTypeLLMResponse:
		// For LLM responses, we need to handle the serialization carefully
		// Similar to NATS implementation, we create LLMResponseWrapper with nil Opts
		var llmResp LLMResponseWrapperForNATS
		if err := json.Unmarshal(pgMsg.Data, &llmResp); err != nil {
			return fmt.Errorf("failed to unmarshal LLM response: %w", err)
		}

		// Convert to full wrapper
		fullResp := &LLMResponseWrapper{
			Response: convertFromNATSSafeResponse(llmResp.Response),
			Opts:     nil, // Empty opts - will be regenerated from session state
		}

		select {
		case llmResponses <- fullResp:
		default:
			slog.Warn("LLM response channel full, dropping response", "session_id", sessionID)
		}

	default:
		return fmt.Errorf("unknown message type: %s", pgMsg.Type)
	}

	return nil
}

// queueTimeout bounds every wait on the database: connecting, LISTEN, and
// NOTIFY without a caller deadline.
func queueTimeout(config PostgreSQLConfig) time.Duration {
	if config.NotifyTimeout > 0 {
		return config.NotifyTimeout
	}
	return DefaultPostgreSQLConfig().NotifyTimeout
}

// reconnectCeiling is the longest pq.Listener waits between reconnect
// attempts; it backs off from ReconnectInterval up to this.
func reconnectCeiling(config PostgreSQLConfig) time.Duration {
	retries := config.MaxReconnectRetries
	if retries < 1 {
		retries = 1
	}
	ceiling := config.ReconnectInterval * time.Duration(retries)
	if ceiling < config.ReconnectInterval {
		ceiling = config.ReconnectInterval
	}
	return ceiling
}

// withQueueDeadline applies the queue timeout to ctx if it has no deadline.
func withQueueDeadline(ctx context.Context, config PostgreSQLConfig) (context.Context, context.CancelFunc) {
	if _, ok := ctx.Deadline(); ok {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, queueTimeout(config))
}
