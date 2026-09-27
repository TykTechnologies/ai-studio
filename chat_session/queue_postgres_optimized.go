package chat_session

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"gorm.io/gorm"
)

// OptimizedPostgreSQLQueue implements MessageQueue using PostgreSQL LISTEN/NOTIFY
// This version reuses the existing database connection to avoid connection exhaustion:
// NOTIFY goes through the application's pool, and notifications arrive on the
// process-wide shared listener (see queue_postgres_listener.go), so a session
// holds no connection of its own.
type OptimizedPostgreSQLQueue struct {
	sessionID string
	db        *gorm.DB
	sqlDB     *sql.DB

	// The process-wide LISTEN connection and this session's channels on it
	listener      *sharedListener
	subscriptions []channelSubscription

	// Local channels for backward compatibility
	messagesChan     chan *ChatResponse
	streamChan       chan []byte
	errorsChan       chan error
	llmResponsesChan chan *LLMResponseWrapper

	// Lifecycle management
	closed     bool
	closeMux   sync.RWMutex
	consumerWG sync.WaitGroup
	cancelCtx  context.Context
	cancel     context.CancelFunc

	// Configuration
	config PostgreSQLConfig
}

// NewOptimizedPostgreSQLQueue creates a new PostgreSQL-based message queue that reuses connections
func NewOptimizedPostgreSQLQueue(sessionID string, db *gorm.DB, config PostgreSQLConfig) (*OptimizedPostgreSQLQueue, error) {
	// Get the underlying SQL database connection
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get SQL database: %w", err)
	}

	// Test the connection, without waiting forever on an exhausted pool
	pingCtx, pingCancel := context.WithTimeout(context.Background(), queueTimeout(config))
	defer pingCancel()
	if err := sqlDB.PingContext(pingCtx); err != nil {
		return nil, fmt.Errorf("database connection failed: %w", err)
	}

	// Create context for lifecycle management
	ctx, cancel := context.WithCancel(context.Background())

	psq := &OptimizedPostgreSQLQueue{
		sessionID:        sessionID,
		db:               db,
		sqlDB:            sqlDB,
		messagesChan:     make(chan *ChatResponse, config.BufferSize),
		streamChan:       make(chan []byte, config.BufferSize),
		errorsChan:       make(chan error, config.BufferSize),
		llmResponsesChan: make(chan *LLMResponseWrapper, config.BufferSize),
		closed:           false,
		cancelCtx:        ctx,
		cancel:           cancel,
		config:           config,
	}

	// Register this session's channels on the shared listener
	if err := psq.setupListener(); err != nil {
		psq.Close()
		return nil, fmt.Errorf("failed to setup listener: %w", err)
	}

	// Start consumers for each message type
	if err := psq.startConsumers(); err != nil {
		psq.Close()
		return nil, fmt.Errorf("failed to start consumers: %w", err)
	}

	slog.Info("Optimized PostgreSQL queue created successfully",
		"session_id", sessionID,
		"connection_reused", true)
	return psq, nil
}

// setupListener registers this session's channels on the shared listener.
// It used to take a connection from the pool per session and never read a
// notification from it, so sessions exhausted the pool and received nothing.
func (psq *OptimizedPostgreSQLQueue) setupListener() error {
	dsn, err := postgresDSN(psq.db)
	if err != nil {
		return err
	}
	listener, err := acquireSharedListener(dsn, psq.config)
	if err != nil {
		return err
	}
	psq.listener = listener

	channels := psq.sessionChannels()
	subs, err := subscribeSessionChannels(listener, channels, psq.handleNotification, queueTimeout(psq.config))
	psq.subscriptions = subs
	if err != nil {
		return err
	}
	for _, channel := range channels {
		slog.Debug("listening to PostgreSQL channel",
			"channel", channel,
			"session_id", psq.sessionID,
			"optimized", true)
	}

	return nil
}

// sessionChannels lists this session's channel for every message type
func (psq *OptimizedPostgreSQLQueue) sessionChannels() []string {
	return []string{
		psq.getChannelName(PostgreSQLMessageTypeChatResponse),
		psq.getChannelName(PostgreSQLMessageTypeStream),
		psq.getChannelName(PostgreSQLMessageTypeError),
		psq.getChannelName(PostgreSQLMessageTypeLLMResponse),
	}
}

// startConsumers has nothing to start: the shared listener dispatches
// notifications to handleNotification
func (psq *OptimizedPostgreSQLQueue) startConsumers() error {
	return nil
}

// handleNotification routes a notification to this session's local channels
func (psq *OptimizedPostgreSQLQueue) handleNotification(payload string) {
	if err := routePostgreSQLNotification(payload, psq.sessionID, psq.messagesChan, psq.streamChan, psq.errorsChan, psq.llmResponsesChan); err != nil {
		slog.Error("failed to handle notification",
			"session_id", psq.sessionID,
			"error", err)
	}
}

// getChannelName returns the PostgreSQL channel name for a given message type
func (psq *OptimizedPostgreSQLQueue) getChannelName(messageType string) string {
	return fmt.Sprintf("chat_%s_%s", messageType, psq.sessionID)
}

// PublishMessage sends a ChatResponse message via PostgreSQL NOTIFY
func (psq *OptimizedPostgreSQLQueue) PublishMessage(ctx context.Context, msg *ChatResponse) error {
	return psq.publishToPostgreSQL(ctx, PostgreSQLMessageTypeChatResponse, msg)
}

// PublishStream sends stream data via PostgreSQL NOTIFY
func (psq *OptimizedPostgreSQLQueue) PublishStream(ctx context.Context, data []byte) error {
	return psq.publishToPostgreSQL(ctx, PostgreSQLMessageTypeStream, data)
}

// PublishError sends an error via PostgreSQL NOTIFY
func (psq *OptimizedPostgreSQLQueue) PublishError(ctx context.Context, err error) error {
	return psq.publishToPostgreSQL(ctx, PostgreSQLMessageTypeError, err.Error())
}

// PublishLLMResponse sends an LLM response via PostgreSQL NOTIFY
func (psq *OptimizedPostgreSQLQueue) PublishLLMResponse(ctx context.Context, resp *LLMResponseWrapper) error {
	return psq.publishToPostgreSQL(ctx, PostgreSQLMessageTypeLLMResponse, resp)
}

// publishToPostgreSQL is a generic method to publish any message type via PostgreSQL NOTIFY
func (psq *OptimizedPostgreSQLQueue) publishToPostgreSQL(ctx context.Context, messageType string, data interface{}) error {
	psq.closeMux.RLock()
	defer psq.closeMux.RUnlock()

	if psq.closed {
		return fmt.Errorf("queue closed")
	}

	var dataBytes []byte
	var err error

	// Handle LLM responses specially
	if messageType == PostgreSQLMessageTypeLLMResponse {
		if llmResp, ok := data.(*LLMResponseWrapper); ok {
			// Convert to PostgreSQL-safe version (without Opts field)
			pgResp := LLMResponseWrapperForNATS{
				Response: convertToNATSSafeResponse(llmResp.Response),
			}
			dataBytes, err = json.Marshal(pgResp)
		} else {
			err = fmt.Errorf("expected *LLMResponseWrapper for LLM response type")
		}
	} else {
		// Standard serialization for other message types
		dataBytes, err = json.Marshal(data)
	}

	if err != nil {
		return fmt.Errorf("failed to serialize message: %w", err)
	}

	// Create PostgreSQL message with metadata
	pgMsg := PostgreSQLMessage{
		Type:      messageType,
		SessionID: psq.sessionID,
		Timestamp: time.Now(),
		Data:      dataBytes,
	}

	msgBytes, err := json.Marshal(pgMsg)
	if err != nil {
		return fmt.Errorf("failed to serialize PostgreSQL message: %w", err)
	}

	// Send NOTIFY command with timeout
	channel := psq.getChannelName(messageType)

	// Use the existing connection pool instead of creating new transactions,
	// bounded so an exhausted pool cannot block the caller forever
	ctx, cancel := withQueueDeadline(ctx, psq.config)
	defer cancel()
	_, err = psq.sqlDB.ExecContext(ctx, "SELECT pg_notify($1, $2)", channel, string(msgBytes))
	if err != nil {
		return fmt.Errorf("failed to notify: %w", err)
	}

	return nil
}

// ConsumeMessages returns the local channel for ChatResponse messages
func (psq *OptimizedPostgreSQLQueue) ConsumeMessages(ctx context.Context) <-chan *ChatResponse {
	return psq.messagesChan
}

// ConsumeStream returns the local channel for stream data
func (psq *OptimizedPostgreSQLQueue) ConsumeStream(ctx context.Context) <-chan []byte {
	return psq.streamChan
}

// ConsumeErrors returns the local channel for error messages
func (psq *OptimizedPostgreSQLQueue) ConsumeErrors(ctx context.Context) <-chan error {
	return psq.errorsChan
}

// ConsumeLLMResponses returns the local channel for LLM responses
func (psq *OptimizedPostgreSQLQueue) ConsumeLLMResponses(ctx context.Context) <-chan *LLMResponseWrapper {
	return psq.llmResponsesChan
}

// Close closes all channels and PostgreSQL connections
func (psq *OptimizedPostgreSQLQueue) Close() error {
	psq.closeMux.Lock()
	defer psq.closeMux.Unlock()

	if psq.closed {
		return nil // Already closed
	}

	psq.closed = true

	// Leave the shared listener. Once this returns no notification is being
	// routed to the local channels, so they can be closed below.
	unsubscribeSessionChannels(psq.listener, psq.subscriptions, queueTimeout(psq.config))
	psq.subscriptions = nil
	if psq.listener != nil {
		psq.listener.release()
		psq.listener = nil
	}

	// Cancel context to stop consumers
	psq.cancel()

	// Wait for all consumers to finish
	psq.consumerWG.Wait()

	// Close local channels
	close(psq.messagesChan)
	close(psq.streamChan)
	close(psq.errorsChan)
	close(psq.llmResponsesChan)

	slog.Info("Optimized PostgreSQL queue closed", "session_id", psq.sessionID)
	return nil
}

// QueueDepth returns the current depth of all local channels
func (psq *OptimizedPostgreSQLQueue) QueueDepth() (messages, stream, errors, llmResponses int) {
	psq.closeMux.RLock()
	defer psq.closeMux.RUnlock()

	if psq.closed {
		return 0, 0, 0, 0
	}

	return len(psq.messagesChan), len(psq.streamChan), len(psq.errorsChan), len(psq.llmResponsesChan)
}
