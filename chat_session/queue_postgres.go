package chat_session

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/lib/pq"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// PostgreSQLQueue implements MessageQueue using PostgreSQL LISTEN/NOTIFY
// Provides distributed message queuing with PostgreSQL pub/sub capabilities
type PostgreSQLQueue struct {
	sessionID string
	db        *gorm.DB
	sqlDB     *sql.DB
	listener  *pq.Listener

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

// PostgreSQLConfig holds configuration for PostgreSQL queue
type PostgreSQLConfig struct {
	BufferSize          int           // Local channel buffer size
	ReconnectInterval   time.Duration // Reconnection interval for listener
	MaxReconnectRetries int           // Maximum reconnection attempts
	NotifyTimeout       time.Duration // Timeout for NOTIFY operations
}

// DefaultPostgreSQLConfig returns default configuration
func DefaultPostgreSQLConfig() PostgreSQLConfig {
	return PostgreSQLConfig{
		BufferSize:          100,
		ReconnectInterval:   2 * time.Second,
		MaxReconnectRetries: 10,
		NotifyTimeout:       5 * time.Second,
	}
}

// PostgreSQLMessage wraps all message types with metadata for JSON serialization
type PostgreSQLMessage struct {
	Type      string    `json:"type"`
	SessionID string    `json:"session_id"`
	Timestamp time.Time `json:"timestamp"`
	Data      []byte    `json:"data"`
}

// Message type constants
const (
	PostgreSQLMessageTypeChatResponse = "chat_response"
	PostgreSQLMessageTypeStream       = "stream"
	PostgreSQLMessageTypeError        = "error"
	PostgreSQLMessageTypeLLMResponse  = "llm_response"
)

// NewPostgreSQLQueue creates a new PostgreSQL-based message queue
func NewPostgreSQLQueue(sessionID string, db *gorm.DB, config PostgreSQLConfig) (*PostgreSQLQueue, error) {
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

	psq := &PostgreSQLQueue{
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

	// Setup PostgreSQL listener with reconnection logic
	if err := psq.setupListener(); err != nil {
		psq.Close()
		return nil, fmt.Errorf("failed to setup listener: %w", err)
	}

	// Start consumers for each message type
	if err := psq.startConsumers(); err != nil {
		psq.Close()
		return nil, fmt.Errorf("failed to start consumers: %w", err)
	}

	slog.Info("PostgreSQL queue created successfully", "session_id", sessionID)
	return psq, nil
}

// setupListener creates and configures the PostgreSQL listener
func (psq *PostgreSQLQueue) setupListener() error {
	// Create PostgreSQL listener with reconnection handling
	listener := psq.createListener()
	psq.listener = listener

	// Listen to all channels for this session
	channels := []string{
		psq.getChannelName(PostgreSQLMessageTypeChatResponse),
		psq.getChannelName(PostgreSQLMessageTypeStream),
		psq.getChannelName(PostgreSQLMessageTypeError),
		psq.getChannelName(PostgreSQLMessageTypeLLMResponse),
	}

	for _, channel := range channels {
		if err := listener.Listen(channel); err != nil {
			return fmt.Errorf("failed to listen to channel %s: %w", channel, err)
		}
		slog.Debug("listening to PostgreSQL channel", "channel", channel, "session_id", psq.sessionID)
	}

	return nil
}

// createListener creates a PostgreSQL listener with proper error handling
func (psq *PostgreSQLQueue) createListener() *pq.Listener {
	// Get database connection string from environment or config
	// We'll use the same database connection info as the main application
	var dsn string

	// Try to get DSN from environment first (most reliable)
	if databaseURL := os.Getenv("DATABASE_URL"); databaseURL != "" {
		dsn = databaseURL
	} else {
		// Fallback: construct DSN (this would need more sophisticated logic in production)
		slog.Warn("DATABASE_URL not found, PostgreSQL queue may not work correctly", "session_id", psq.sessionID)
		dsn = "postgres://localhost/midsommar?sslmode=disable" // fallback
	}

	return pq.NewListener(
		dsn,
		psq.config.ReconnectInterval,
		psq.config.NotifyTimeout,
		func(ev pq.ListenerEventType, err error) {
			switch ev {
			case pq.ListenerEventConnected:
				slog.Info("PostgreSQL listener connected", "session_id", psq.sessionID)
			case pq.ListenerEventDisconnected:
				slog.Warn("PostgreSQL listener disconnected", "session_id", psq.sessionID, "error", err)
			case pq.ListenerEventReconnected:
				slog.Info("PostgreSQL listener reconnected", "session_id", psq.sessionID)
			case pq.ListenerEventConnectionAttemptFailed:
				slog.Error("PostgreSQL listener connection failed", "session_id", psq.sessionID, "error", err)
			}
		},
	)
}

// startConsumers starts goroutines to consume messages from PostgreSQL notifications
func (psq *PostgreSQLQueue) startConsumers() error {
	// Start a single consumer that routes messages based on channel
	psq.consumerWG.Add(1)
	go psq.consumeNotifications()

	return nil
}

// consumeNotifications consumes PostgreSQL notifications and routes them to appropriate channels
func (psq *PostgreSQLQueue) consumeNotifications() {
	defer psq.consumerWG.Done()

	for {
		select {
		case <-psq.cancelCtx.Done():
			return
		case notification := <-psq.listener.Notify:
			if notification == nil {
				continue
			}

			if err := psq.handleNotification(notification); err != nil {
				slog.Error("failed to handle notification", "session_id", psq.sessionID, "error", err)
			}
		}
	}
}

// handleNotification processes a PostgreSQL notification and routes it to the appropriate channel
func (psq *PostgreSQLQueue) handleNotification(notification *pq.Notification) error {
	// Deserialize the message
	var pgMsg PostgreSQLMessage
	if err := json.Unmarshal([]byte(notification.Extra), &pgMsg); err != nil {
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
		case psq.messagesChan <- &chatResp:
		default:
			slog.Warn("message channel full, dropping message", "session_id", psq.sessionID)
		}

	case PostgreSQLMessageTypeStream:
		var streamData []byte
		if err := json.Unmarshal(pgMsg.Data, &streamData); err != nil {
			return fmt.Errorf("failed to unmarshal stream data: %w", err)
		}

		select {
		case psq.streamChan <- streamData:
		default:
			slog.Warn("stream channel full, dropping data", "session_id", psq.sessionID)
		}

	case PostgreSQLMessageTypeError:
		var errorStr string
		if err := json.Unmarshal(pgMsg.Data, &errorStr); err != nil {
			return fmt.Errorf("failed to unmarshal error: %w", err)
		}

		select {
		case psq.errorsChan <- fmt.Errorf(errorStr):
		default:
			slog.Warn("error channel full, dropping error", "session_id", psq.sessionID)
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
		case psq.llmResponsesChan <- fullResp:
		default:
			slog.Warn("LLM response channel full, dropping response", "session_id", psq.sessionID)
		}

	default:
		return fmt.Errorf("unknown message type: %s", pgMsg.Type)
	}

	return nil
}

// getChannelName returns the PostgreSQL channel name for a given message type
func (psq *PostgreSQLQueue) getChannelName(messageType string) string {
	return fmt.Sprintf("chat_%s_%s", messageType, psq.sessionID)
}

// PublishMessage sends a ChatResponse message via PostgreSQL NOTIFY
func (psq *PostgreSQLQueue) PublishMessage(ctx context.Context, msg *ChatResponse) error {
	return psq.publishToPostgreSQL(ctx, PostgreSQLMessageTypeChatResponse, msg)
}

// PublishStream sends stream data via PostgreSQL NOTIFY
func (psq *PostgreSQLQueue) PublishStream(ctx context.Context, data []byte) error {
	return psq.publishToPostgreSQL(ctx, PostgreSQLMessageTypeStream, data)
}

// PublishError sends an error via PostgreSQL NOTIFY
func (psq *PostgreSQLQueue) PublishError(ctx context.Context, err error) error {
	return psq.publishToPostgreSQL(ctx, PostgreSQLMessageTypeError, err.Error())
}

// PublishLLMResponse sends an LLM response via PostgreSQL NOTIFY
func (psq *PostgreSQLQueue) PublishLLMResponse(ctx context.Context, resp *LLMResponseWrapper) error {
	return psq.publishToPostgreSQL(ctx, PostgreSQLMessageTypeLLMResponse, resp)
}

// publishToPostgreSQL is a generic method to publish any message type via PostgreSQL NOTIFY
func (psq *PostgreSQLQueue) publishToPostgreSQL(ctx context.Context, messageType string, data interface{}) error {
	psq.closeMux.RLock()
	defer psq.closeMux.RUnlock()

	if psq.closed {
		return fmt.Errorf("queue closed")
	}

	var dataBytes []byte
	var err error

	// Handle LLM responses specially, similar to NATS implementation
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

	// Use a transaction with timeout context
	tx, err := psq.sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Execute NOTIFY command
	_, err = tx.ExecContext(ctx, "SELECT pg_notify($1, $2)", channel, string(msgBytes))
	if err != nil {
		return fmt.Errorf("failed to notify: %w", err)
	}

	// Commit the transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit notification: %w", err)
	}

	return nil
}

// ConsumeMessages returns the local channel for ChatResponse messages
func (psq *PostgreSQLQueue) ConsumeMessages(ctx context.Context) <-chan *ChatResponse {
	return psq.messagesChan
}

// ConsumeStream returns the local channel for stream data
func (psq *PostgreSQLQueue) ConsumeStream(ctx context.Context) <-chan []byte {
	return psq.streamChan
}

// ConsumeErrors returns the local channel for error messages
func (psq *PostgreSQLQueue) ConsumeErrors(ctx context.Context) <-chan error {
	return psq.errorsChan
}

// ConsumeLLMResponses returns the local channel for LLM responses
func (psq *PostgreSQLQueue) ConsumeLLMResponses(ctx context.Context) <-chan *LLMResponseWrapper {
	return psq.llmResponsesChan
}

// Close closes all channels and PostgreSQL connections
func (psq *PostgreSQLQueue) Close() error {
	psq.closeMux.Lock()
	defer psq.closeMux.Unlock()

	if psq.closed {
		return nil // Already closed
	}

	psq.closed = true

	// Cancel context to stop consumers
	psq.cancel()

	// Wait for all consumers to finish
	psq.consumerWG.Wait()

	// Close PostgreSQL listener
	if psq.listener != nil {
		if err := psq.listener.Close(); err != nil {
			slog.Warn("error closing PostgreSQL listener", "session_id", psq.sessionID, "error", err)
		}
	}

	// Close local channels
	close(psq.messagesChan)
	close(psq.streamChan)
	close(psq.errorsChan)
	close(psq.llmResponsesChan)

	slog.Info("PostgreSQL queue closed", "session_id", psq.sessionID)
	return nil
}

// QueueDepth returns the current depth of all local channels
// Note: This doesn't include messages pending in PostgreSQL notifications
func (psq *PostgreSQLQueue) QueueDepth() (messages, stream, errors, llmResponses int) {
	psq.closeMux.RLock()
	defer psq.closeMux.RUnlock()

	if psq.closed {
		return 0, 0, 0, 0
	}

	return len(psq.messagesChan), len(psq.streamChan), len(psq.errorsChan), len(psq.llmResponsesChan)
}

// PostgreSQLQueueFactory creates PostgreSQL queue instances
type PostgreSQLQueueFactory struct {
	db     *gorm.DB
	config PostgreSQLConfig
}

// NewPostgreSQLQueueFactory creates a new PostgreSQL factory with specified configuration
func NewPostgreSQLQueueFactory(db *gorm.DB, config PostgreSQLConfig) *PostgreSQLQueueFactory {
	return &PostgreSQLQueueFactory{
		db:     db,
		config: config,
	}
}

// CreateQueue creates a new PostgreSQL queue with session-specific configuration
func (f *PostgreSQLQueueFactory) CreateQueue(sessionID string, config map[string]interface{}) (MessageQueue, error) {
	pgConfig := f.config

	// Apply configuration overrides
	if config != nil {
		if bufferSize, ok := config["bufferSize"].(int); ok && bufferSize > 0 {
			pgConfig.BufferSize = bufferSize
		}
	}

	return NewPostgreSQLQueue(sessionID, f.db, pgConfig)
}

// Helper function for creating a PostgreSQL queue with default settings
func NewDefaultPostgreSQLQueue(sessionID string, db *gorm.DB) (MessageQueue, error) {
	config := DefaultPostgreSQLConfig()
	return NewPostgreSQLQueue(sessionID, db, config)
}

// DeferredPostgreSQLQueueFactory creates PostgreSQL queues by connecting to the database at queue creation time
type DeferredPostgreSQLQueueFactory struct {
	config PostgreSQLConfig
}

// NewDeferredPostgreSQLQueueFactory creates a deferred PostgreSQL factory
func NewDeferredPostgreSQLQueueFactory(config PostgreSQLConfig) *DeferredPostgreSQLQueueFactory {
	return &DeferredPostgreSQLQueueFactory{
		config: config,
	}
}

// CreateQueue creates a new PostgreSQL queue by connecting to the database using DATABASE_URL
func (f *DeferredPostgreSQLQueueFactory) CreateQueue(sessionID string, config map[string]interface{}) (MessageQueue, error) {
	// Connect to database using environment configuration
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL environment variable is required for PostgreSQL queues")
	}

	// Import the required database packages
	db, err := gorm.Open(postgres.Open(databaseURL), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to PostgreSQL database: %w", err)
	}

	// Test the connection
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get SQL database: %w", err)
	}
	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("PostgreSQL database not accessible: %w", err)
	}

	// Configure connection pool to prevent exhaustion
	// Limit connections per queue factory instance
	sqlDB.SetMaxOpenConns(25)                 // Reduced from default to prevent exhaustion
	sqlDB.SetMaxIdleConns(5)                  // Keep fewer idle connections
	sqlDB.SetConnMaxLifetime(5 * time.Minute) // Recycle connections regularly

	psqlConfig := f.config

	// Apply configuration overrides
	if config != nil {
		if bufferSize, ok := config["bufferSize"].(int); ok && bufferSize > 0 {
			psqlConfig.BufferSize = bufferSize
		}
	}

	// Create with connection pooling configured
	slog.Info("Creating PostgreSQL queue with connection pooling",
		"session_id", sessionID,
		"max_connections", 25,
		"connection_pooling", true)

	return NewPostgreSQLQueue(sessionID, db, psqlConfig)
}

// SharedPostgreSQLQueueFactory creates PostgreSQL queues using a shared database connection
// This prevents connection exhaustion by reusing the application's existing connection pool
type SharedPostgreSQLQueueFactory struct {
	db     *gorm.DB
	config PostgreSQLConfig
}

// NewSharedPostgreSQLQueueFactory creates a shared PostgreSQL factory that reuses database connections
func NewSharedPostgreSQLQueueFactory(db *gorm.DB, config PostgreSQLConfig) *SharedPostgreSQLQueueFactory {
	return &SharedPostgreSQLQueueFactory{
		db:     db,
		config: config,
	}
}

// CreateQueue creates a new PostgreSQL queue using the shared database connection
func (f *SharedPostgreSQLQueueFactory) CreateQueue(sessionID string, config map[string]interface{}) (MessageQueue, error) {
	psqlConfig := f.config

	// Apply configuration overrides
	if config != nil {
		if bufferSize, ok := config["bufferSize"].(int); ok && bufferSize > 0 {
			psqlConfig.BufferSize = bufferSize
		}
	}

	// Create with shared connection - no new database connections
	slog.Info("Creating PostgreSQL queue with shared connection pool",
		"session_id", sessionID,
		"connection_shared", true)

	return NewPostgreSQLQueue(sessionID, f.db, psqlConfig)
}
