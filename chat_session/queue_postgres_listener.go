package chat_session

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/TykTechnologies/midsommar/v2/config"
	"github.com/TykTechnologies/midsommar/v2/pkg/pglisten"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// Every PostgreSQL queue in the process receives its notifications through one
// shared LISTEN connection per database (package pglisten), opened outside the
// application's pool. A session only registers its channels on it, so the
// number of sessions no longer decides how many connections are held, and a
// session never waits on the pool to start listening.

// sharedListener is the process-wide listener for one database.
type sharedListener = pglisten.Listener

// notificationHandler receives the payload of a notification on one channel.
// It is called from the dispatch goroutine and must not block.
type notificationHandler = pglisten.Handler

// postgresDSN returns the connection string db was opened with, falling back
// to the configured database URL (a host may open db from a *sql.DB, which
// leaves no DSN on the dialector) and then to DATABASE_URL.
func postgresDSN(db *gorm.DB) (string, error) {
	return pglisten.DSN(db, config.Get("").DatabaseURL, os.Getenv("DATABASE_URL"))
}

// acquireSharedListener returns the listener for dsn, connecting it if this is
// its first user. It waits at most the queue timeout for the connection.
func acquireSharedListener(dsn string, config PostgreSQLConfig) (*sharedListener, error) {
	if config.ReconnectInterval <= 0 {
		config.ReconnectInterval = DefaultPostgreSQLConfig().ReconnectInterval
	}
	l, err := pglisten.Acquire(dsn, pglisten.Options{
		ReconnectInterval: config.ReconnectInterval,
		ReconnectCeiling:  reconnectCeiling(config),
		ConnectTimeout:    queueTimeout(config),
		Name:              "PostgreSQL queue listener",
	})
	if err != nil {
		return nil, fmt.Errorf("queue listener: %w", err)
	}
	return l, nil
}

// channelSubscription is one session channel registered on a shared listener.
type channelSubscription struct {
	channel string
	sub     *pglisten.Subscription
}

// subscribeSessionChannels registers handler for every channel. On error the
// subscriptions made so far are returned so the caller can release them.
func subscribeSessionChannels(sl *sharedListener, channels []string, handler notificationHandler, timeout time.Duration) ([]channelSubscription, error) {
	subs := make([]channelSubscription, 0, len(channels))
	for _, channel := range channels {
		sub, err := sl.Subscribe(channel, handler, timeout)
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
		sl.Unsubscribe(s.channel, s.sub, timeout)
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
