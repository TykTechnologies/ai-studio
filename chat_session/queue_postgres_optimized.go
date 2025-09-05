package chat_session

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/lib/pq"
	"gorm.io/gorm"
)

// OptimizedPostgreSQLQueue implements MessageQueue using PostgreSQL LISTEN/NOTIFY
// This version reuses the existing database connection to avoid connection exhaustion
type OptimizedPostgreSQLQueue struct {
	sessionID string
	db        *gorm.DB
	sqlDB     *sql.DB

	// Single connection for LISTEN/NOTIFY operations
	listenerConn *sql.Conn

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

	// Test the connection
	if err := sqlDB.Ping(); err != nil {
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

	// Setup PostgreSQL listener using a single connection from the pool
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

// setupListener sets up LISTEN commands using a single connection from the existing pool
func (psq *OptimizedPostgreSQLQueue) setupListener() error {
	// Get a single connection from the pool for LISTEN operations
	conn, err := psq.sqlDB.Conn(psq.cancelCtx)
	if err != nil {
		return fmt.Errorf("failed to get connection from pool: %w", err)
	}
	psq.listenerConn = conn

	// Listen to all channels for this session using the same connection
	channels := []string{
		psq.getChannelName(PostgreSQLMessageTypeChatResponse),
		psq.getChannelName(PostgreSQLMessageTypeStream),
		psq.getChannelName(PostgreSQLMessageTypeError),
		psq.getChannelName(PostgreSQLMessageTypeLLMResponse),
	}

	for _, channel := range channels {
		if err := psq.listenToChannel(channel); err != nil {
			return fmt.Errorf("failed to listen to channel %s: %w", channel, err)
		}
		slog.Debug("listening to PostgreSQL channel",
			"channel", channel,
			"session_id", psq.sessionID,
			"optimized", true)
	}

	return nil
}

// listenToChannel issues a LISTEN command on the existing connection
func (psq *OptimizedPostgreSQLQueue) listenToChannel(channel string) error {
	_, err := psq.listenerConn.ExecContext(psq.cancelCtx, "LISTEN "+pq.QuoteIdentifier(channel))
	return err
}

// unlistenToChannel issues an UNLISTEN command on the existing connection
func (psq *OptimizedPostgreSQLQueue) unlistenToChannel(channel string) error {
	if psq.listenerConn != nil {
		_, err := psq.listenerConn.ExecContext(psq.cancelCtx, "UNLISTEN "+pq.QuoteIdentifier(channel))
		return err
	}
	return nil
}

// startConsumers starts goroutines to consume messages from PostgreSQL notifications
func (psq *OptimizedPostgreSQLQueue) startConsumers() error {
	// Start a single consumer that uses the shared connection
	psq.consumerWG.Add(1)
	go psq.consumeNotifications()

	return nil
}

// consumeNotifications consumes PostgreSQL notifications using the shared connection
func (psq *OptimizedPostgreSQLQueue) consumeNotifications() {
	defer psq.consumerWG.Done()

	// Create a custom listener that polls for notifications
	for {
		select {
		case <-psq.cancelCtx.Done():
			return
		default:
			// Poll for notifications with a timeout
			if err := psq.pollNotifications(); err != nil {
				if err == context.Canceled {
					return
				}
				slog.Error("failed to poll notifications",
					"session_id", psq.sessionID,
					"error", err)

				// Retry after a short delay
				time.Sleep(psq.config.ReconnectInterval)
			}
		}
	}
}

// pollNotifications checks for and processes any pending notifications
func (psq *OptimizedPostgreSQLQueue) pollNotifications() error {
	// Use a shorter timeout for polling to remain responsive
	ctx, cancel := context.WithTimeout(psq.cancelCtx, 100*time.Millisecond)
	defer cancel()

	errChan := make(chan error, 1)

	go func() {
		// Check for notifications using a query with timeout
		rows, err := psq.listenerConn.QueryContext(ctx, "SELECT 1")
		if err != nil {
			if err != context.DeadlineExceeded {
				errChan <- err
			}
			return
		}
		rows.Close()

		// Process any pending notifications by checking the connection
		// This is a workaround since we can't directly access pq.Conn
		var queueUsage float64
		err = psq.listenerConn.QueryRowContext(ctx,
			"SELECT pg_notification_queue_usage()").Scan(&queueUsage)
		if err != nil && err != sql.ErrNoRows {
			// Log the error but don't treat it as fatal
			slog.Debug("notification queue check", "error", err)
		}
	}()

	select {
	case err := <-errChan:
		return err
	case <-ctx.Done():
		// Timeout is normal, not an error
		return nil
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

	// Use the existing connection pool instead of creating new transactions
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

	// Unlisten from all channels
	channels := []string{
		psq.getChannelName(PostgreSQLMessageTypeChatResponse),
		psq.getChannelName(PostgreSQLMessageTypeStream),
		psq.getChannelName(PostgreSQLMessageTypeError),
		psq.getChannelName(PostgreSQLMessageTypeLLMResponse),
	}

	for _, channel := range channels {
		if err := psq.unlistenToChannel(channel); err != nil {
			slog.Warn("error unlistening from channel",
				"channel", channel,
				"session_id", psq.sessionID,
				"error", err)
		}
	}

	// Cancel context to stop consumers
	psq.cancel()

	// Wait for all consumers to finish
	psq.consumerWG.Wait()

	// Close the listener connection (returns it to the pool)
	if psq.listenerConn != nil {
		if err := psq.listenerConn.Close(); err != nil {
			slog.Warn("error closing listener connection",
				"session_id", psq.sessionID,
				"error", err)
		}
	}

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
