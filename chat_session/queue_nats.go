package chat_session

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/tmc/langchaingo/llms"
)

// convertToNATSSafeResponse converts llms.ContentResponse to ContentResponseForNATS
func convertToNATSSafeResponse(resp *llms.ContentResponse) *ContentResponseForNATS {
	if resp == nil {
		return nil
	}

	natsResp := &ContentResponseForNATS{
		Choices: make([]*ContentChoiceForNATS, len(resp.Choices)),
	}

	for i, choice := range resp.Choices {
		natsChoice := &ContentChoiceForNATS{
			Content:        choice.Content,
			StopReason:     choice.StopReason,
			ToolCalls:      make([]ToolCallForNATS, len(choice.ToolCalls)),
			GenerationInfo: choice.GenerationInfo,
		}

		for j, toolCall := range choice.ToolCalls {
			natsToolCall := ToolCallForNATS{
				ID:   toolCall.ID,
				Type: toolCall.Type,
			}

			if toolCall.FunctionCall != nil {
				natsToolCall.FunctionCall = &FunctionCallForNATS{
					Name:      toolCall.FunctionCall.Name,
					Arguments: toolCall.FunctionCall.Arguments,
				}
			}

			natsChoice.ToolCalls[j] = natsToolCall
		}

		natsResp.Choices[i] = natsChoice
	}

	return natsResp
}

// convertFromNATSSafeResponse converts ContentResponseForNATS back to llms.ContentResponse
func convertFromNATSSafeResponse(natsResp *ContentResponseForNATS) *llms.ContentResponse {
	if natsResp == nil {
		return nil
	}

	resp := &llms.ContentResponse{
		Choices: make([]*llms.ContentChoice, len(natsResp.Choices)),
	}

	for i, natsChoice := range natsResp.Choices {
		choice := &llms.ContentChoice{
			Content:        natsChoice.Content,
			StopReason:     natsChoice.StopReason,
			ToolCalls:      make([]llms.ToolCall, len(natsChoice.ToolCalls)),
			GenerationInfo: natsChoice.GenerationInfo,
		}

		for j, natsToolCall := range natsChoice.ToolCalls {
			toolCall := llms.ToolCall{
				ID:   natsToolCall.ID,
				Type: natsToolCall.Type,
			}

			if natsToolCall.FunctionCall != nil {
				toolCall.FunctionCall = &llms.FunctionCall{
					Name:      natsToolCall.FunctionCall.Name,
					Arguments: natsToolCall.FunctionCall.Arguments,
				}
			}

			choice.ToolCalls[j] = toolCall
		}

		resp.Choices[i] = choice
	}

	return resp
}

// NATSQueue implements MessageQueue using NATS JetStream
// Provides persistent message storage with automatic cleanup
type NATSQueue struct {
	sessionID string
	conn      *nats.Conn
	js        nats.JetStreamContext
	config    NATSConfig

	// Local channels for backward compatibility
	messagesChan     chan *ChatResponse
	streamChan       chan []byte
	errorsChan       chan error
	llmResponsesChan chan *LLMResponseWrapper

	// NATS subscriptions for each message type
	msgSubscription    *nats.Subscription
	streamSubscription *nats.Subscription
	errorSubscription  *nats.Subscription
	llmSubscription    *nats.Subscription

	// Lifecycle management
	closed      bool
	closeMux    sync.RWMutex
	consumerWG  sync.WaitGroup
	cancelFuncs []context.CancelFunc
}

// NATSConfig holds configuration for NATS JetStream
type NATSConfig struct {
	URL             string        `json:"url"`
	StorageType     string        `json:"storage_type"`     // "memory" | "file"
	RetentionPolicy string        `json:"retention_policy"` // "limits" | "interest" | "workqueue"
	MaxAge          time.Duration `json:"max_age"`
	MaxBytes        int64         `json:"max_bytes"`
	DurableConsumer bool          `json:"durable_consumer"`
	AckWait         time.Duration `json:"ack_wait"`
	MaxDeliver      int           `json:"max_deliver"`
	BufferSize      int           `json:"buffer_size"`
	FetchTimeout    time.Duration `json:"fetch_timeout"`  // Timeout for individual fetch operations
	RetryInterval   time.Duration `json:"retry_interval"` // Interval between fetch retries
	MaxRetries      int           `json:"max_retries"`    // Max retries for failed operations
	
	// Authentication options
	CredentialsFile string `json:"credentials_file"` // Optional NATS credentials file
	Username        string `json:"username"`         // Optional username for basic auth
	Password        string `json:"password"`         // Optional password for basic auth
	Token           string `json:"token"`            // Optional token for token-based auth
	NKeyFile        string `json:"nkey_file"`        // Optional NKey file path
	
	// TLS options
	TLSEnabled      bool   `json:"tls_enabled"`      // Enable TLS connection
	TLSCertFile     string `json:"tls_cert_file"`    // Optional client certificate file
	TLSKeyFile      string `json:"tls_key_file"`     // Optional client key file
	TLSCAFile       string `json:"tls_ca_file"`      // Optional CA certificate file
	TLSSkipVerify   bool   `json:"tls_skip_verify"`  // Skip TLS certificate verification
}

// DefaultNATSConfig returns the hybrid persistent configuration (Option 3)
func DefaultNATSConfig() NATSConfig {
	return NATSConfig{
		URL:             "nats://localhost:4222",
		StorageType:     "file",
		RetentionPolicy: "interest",
		MaxAge:          2 * time.Hour,
		MaxBytes:        100 * 1024 * 1024, // 100MB
		DurableConsumer: true,
		AckWait:         30 * time.Second,
		MaxDeliver:      3,
		BufferSize:      100,
		FetchTimeout:    5 * time.Second, // Default 5 second fetch timeout
		RetryInterval:   1 * time.Second, // Default 1 second between retries
		MaxRetries:      3,               // Default max 3 retries for operations
		
		// Authentication defaults (empty - no auth by default)
		CredentialsFile: "",
		Username:        "",
		Password:        "",
		Token:           "",
		NKeyFile:        "",
		
		// TLS defaults (disabled by default)
		TLSEnabled:      false,
		TLSCertFile:     "",
		TLSKeyFile:      "",
		TLSCAFile:       "",
		TLSSkipVerify:   false,
	}
}

// NATSMessage wraps all message types with metadata
type NATSMessage struct {
	Type      string    `json:"type"`
	SessionID string    `json:"session_id"`
	Timestamp time.Time `json:"timestamp"`
	Data      []byte    `json:"data"`
}

// NATS-safe structures for tool calls
type FunctionCallForNATS struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type ToolCallForNATS struct {
	ID           string               `json:"id"`
	Type         string               `json:"type"`
	FunctionCall *FunctionCallForNATS `json:"function_call,omitempty"`
}

type ContentChoiceForNATS struct {
	Content        string            `json:"content"`
	StopReason     string            `json:"stop_reason,omitempty"`
	ToolCalls      []ToolCallForNATS `json:"tool_calls,omitempty"`
	GenerationInfo map[string]any    `json:"generation_info,omitempty"`
}

type ContentResponseForNATS struct {
	Choices []*ContentChoiceForNATS `json:"choices"`
}

// LLMResponseWrapperForNATS is a NATS-serializable version of LLMResponseWrapper
// that excludes the non-serializable Opts field and uses NATS-safe tool call structures
type LLMResponseWrapperForNATS struct {
	Response *ContentResponseForNATS `json:"response"`
}

// Message type constants
const (
	MessageTypeChatResponse = "chat_response"
	MessageTypeStream       = "stream"
	MessageTypeError        = "error"
	MessageTypeLLMResponse  = "llm_response"
)

// addNATSAuthOptions configures NATS authentication options
func addNATSAuthOptions(opts *[]nats.Option, config NATSConfig) error {
	// Handle credentials file authentication (JWT/NKeys)
	if config.CredentialsFile != "" {
		*opts = append(*opts, nats.UserCredentials(config.CredentialsFile))
		slog.Info("NATS authentication configured with credentials file", "file", config.CredentialsFile)
	}
	
	// Handle NKey file authentication
	if config.NKeyFile != "" {
		*opts = append(*opts, nats.UserCredentials(config.NKeyFile))
		slog.Info("NATS authentication configured with NKey file", "file", config.NKeyFile)
	}
	
	// Handle basic username/password authentication
	if config.Username != "" && config.Password != "" {
		*opts = append(*opts, nats.UserInfo(config.Username, config.Password))
		slog.Info("NATS authentication configured with username/password", "username", config.Username)
	}
	
	// Handle token-based authentication
	if config.Token != "" {
		*opts = append(*opts, nats.Token(config.Token))
		slog.Info("NATS authentication configured with token")
	}
	
	// Handle TLS configuration
	if config.TLSEnabled {
		tlsConfig := &tls.Config{
			InsecureSkipVerify: config.TLSSkipVerify,
		}
		
		// Load client certificate if provided
		if config.TLSCertFile != "" && config.TLSKeyFile != "" {
			cert, err := tls.LoadX509KeyPair(config.TLSCertFile, config.TLSKeyFile)
			if err != nil {
				return fmt.Errorf("failed to load TLS client certificate: %w", err)
			}
			tlsConfig.Certificates = []tls.Certificate{cert}
			slog.Info("NATS TLS client certificate configured", "cert_file", config.TLSCertFile, "key_file", config.TLSKeyFile)
		}
		
		// Load CA certificate if provided
		if config.TLSCAFile != "" {
			// Note: For CA files, users typically need to handle this through the NATS server configuration
			// or by setting up the CA in the system trust store. NATS Go client doesn't have a direct
			// option for custom CA files, but we can configure InsecureSkipVerify for testing.
			slog.Info("NATS TLS CA file specified - ensure CA is properly configured", "ca_file", config.TLSCAFile)
		}
		
		*opts = append(*opts, nats.Secure(tlsConfig))
		slog.Info("NATS TLS connection enabled", "skip_verify", config.TLSSkipVerify)
	}
	
	return nil
}

// NewNATSQueue creates a new NATS-based message queue
func NewNATSQueue(sessionID string, config NATSConfig) (*NATSQueue, error) {
	// Connect to NATS
	opts := []nats.Option{
		nats.ReconnectWait(1 * time.Second),
		nats.MaxReconnects(-1), // Unlimited reconnects
		nats.ReconnectHandler(func(conn *nats.Conn) {
			slog.Info("NATS reconnected", "session_id", sessionID, "url", conn.ConnectedUrl())
		}),
		nats.DisconnectHandler(func(conn *nats.Conn) {
			slog.Warn("NATS disconnected", "session_id", sessionID)
		}),
		nats.ErrorHandler(func(conn *nats.Conn, s *nats.Subscription, err error) {
			slog.Error("NATS error", "session_id", sessionID, "error", err)
		}),
	}
	
	// Add authentication options
	if err := addNATSAuthOptions(&opts, config); err != nil {
		return nil, fmt.Errorf("failed to configure NATS authentication: %w", err)
	}

	conn, err := nats.Connect(config.URL, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}

	js, err := conn.JetStream()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to create JetStream context: %w", err)
	}

	nq := &NATSQueue{
		sessionID:        sessionID,
		conn:             conn,
		js:               js,
		config:           config,
		messagesChan:     make(chan *ChatResponse, config.BufferSize),
		streamChan:       make(chan []byte, config.BufferSize),
		errorsChan:       make(chan error, config.BufferSize),
		llmResponsesChan: make(chan *LLMResponseWrapper, config.BufferSize),
		closed:           false,
		cancelFuncs:      make([]context.CancelFunc, 0),
	}

	// Setup streams and consumers
	if err := nq.setupStreams(); err != nil {
		nq.Close()
		return nil, fmt.Errorf("failed to setup streams: %w", err)
	}

	if err := nq.setupConsumers(); err != nil {
		nq.Close()
		return nil, fmt.Errorf("failed to setup consumers: %w", err)
	}

	return nq, nil
}

// setupStreams creates JetStream streams for each message type
func (nq *NATSQueue) setupStreams() error {
	streamNames := []string{
		nq.getStreamName(MessageTypeChatResponse),
		nq.getStreamName(MessageTypeStream),
		nq.getStreamName(MessageTypeError),
		nq.getStreamName(MessageTypeLLMResponse),
	}

	// Convert retention policy string to NATS constant
	var retention nats.RetentionPolicy
	switch nq.config.RetentionPolicy {
	case "interest":
		retention = nats.InterestPolicy
	case "limits":
		retention = nats.LimitsPolicy
	case "workqueue":
		retention = nats.WorkQueuePolicy
	default:
		retention = nats.InterestPolicy
	}

	// Convert storage type string to NATS constant
	var storage nats.StorageType
	switch nq.config.StorageType {
	case "file":
		storage = nats.FileStorage
	case "memory":
		storage = nats.MemoryStorage
	default:
		storage = nats.FileStorage
	}

	for i, streamName := range streamNames {
		// Map stream names to subjects
		var subject string
		switch i {
		case 0:
			subject = nq.getSubject(MessageTypeChatResponse)
		case 1:
			subject = nq.getSubject(MessageTypeStream)
		case 2:
			subject = nq.getSubject(MessageTypeError)
		case 3:
			subject = nq.getSubject(MessageTypeLLMResponse)
		}

		streamConfig := &nats.StreamConfig{
			Name:      streamName,
			Subjects:  []string{subject},
			Storage:   storage,
			Retention: retention,
			MaxAge:    nq.config.MaxAge,
			MaxBytes:  nq.config.MaxBytes,
		}

		// Try to create stream, ignore error if it already exists
		_, err := nq.js.AddStream(streamConfig)
		if err != nil && err != nats.ErrStreamNameAlreadyInUse {
			return fmt.Errorf("failed to create stream %s: %w", streamName, err)
		}
	}

	return nil
}

// setupConsumers creates durable consumers for each message type
func (nq *NATSQueue) setupConsumers() error {
	// Setup message consumer
	if err := nq.setupMessageConsumer(); err != nil {
		return fmt.Errorf("failed to setup message consumer: %w", err)
	}

	// Setup stream consumer
	if err := nq.setupStreamConsumer(); err != nil {
		return fmt.Errorf("failed to setup stream consumer: %w", err)
	}

	// Setup error consumer
	if err := nq.setupErrorConsumer(); err != nil {
		return fmt.Errorf("failed to setup error consumer: %w", err)
	}

	// Setup LLM response consumer
	if err := nq.setupLLMConsumer(); err != nil {
		return fmt.Errorf("failed to setup LLM consumer: %w", err)
	}

	return nil
}

// setupMessageConsumer creates consumer for ChatResponse messages
func (nq *NATSQueue) setupMessageConsumer() error {
	subject := nq.getSubject(MessageTypeChatResponse)

	var opts []nats.SubOpt
	if nq.config.DurableConsumer {
		durableName := fmt.Sprintf("%s-messages", nq.sessionID)
		opts = append(opts, nats.Durable(durableName))
	}
	opts = append(opts, nats.AckWait(nq.config.AckWait))
	opts = append(opts, nats.MaxDeliver(nq.config.MaxDeliver))

	sub, err := nq.js.PullSubscribe(subject, "", opts...)
	if err != nil {
		return err
	}

	nq.msgSubscription = sub

	// Start consumer goroutine
	ctx, cancel := context.WithCancel(context.Background())
	nq.cancelFuncs = append(nq.cancelFuncs, cancel)

	nq.consumerWG.Add(1)
	go nq.consumeMessages(ctx, sub)

	return nil
}

// setupStreamConsumer creates consumer for stream data
func (nq *NATSQueue) setupStreamConsumer() error {
	subject := nq.getSubject(MessageTypeStream)

	var opts []nats.SubOpt
	if nq.config.DurableConsumer {
		durableName := fmt.Sprintf("%s-stream", nq.sessionID)
		opts = append(opts, nats.Durable(durableName))
	}
	opts = append(opts, nats.AckWait(nq.config.AckWait))
	opts = append(opts, nats.MaxDeliver(nq.config.MaxDeliver))

	sub, err := nq.js.PullSubscribe(subject, "", opts...)
	if err != nil {
		return err
	}

	nq.streamSubscription = sub

	ctx, cancel := context.WithCancel(context.Background())
	nq.cancelFuncs = append(nq.cancelFuncs, cancel)

	nq.consumerWG.Add(1)
	go nq.consumeStream(ctx, sub)

	return nil
}

// setupErrorConsumer creates consumer for error messages
func (nq *NATSQueue) setupErrorConsumer() error {
	subject := nq.getSubject(MessageTypeError)

	var opts []nats.SubOpt
	if nq.config.DurableConsumer {
		durableName := fmt.Sprintf("%s-errors", nq.sessionID)
		opts = append(opts, nats.Durable(durableName))
	}
	opts = append(opts, nats.AckWait(nq.config.AckWait))
	opts = append(opts, nats.MaxDeliver(nq.config.MaxDeliver))

	sub, err := nq.js.PullSubscribe(subject, "", opts...)
	if err != nil {
		return err
	}

	nq.errorSubscription = sub

	ctx, cancel := context.WithCancel(context.Background())
	nq.cancelFuncs = append(nq.cancelFuncs, cancel)

	nq.consumerWG.Add(1)
	go nq.consumeErrors(ctx, sub)

	return nil
}

// setupLLMConsumer creates consumer for LLM responses
func (nq *NATSQueue) setupLLMConsumer() error {
	subject := nq.getSubject(MessageTypeLLMResponse)

	var opts []nats.SubOpt
	if nq.config.DurableConsumer {
		durableName := fmt.Sprintf("%s-llm", nq.sessionID)
		opts = append(opts, nats.Durable(durableName))
	}
	opts = append(opts, nats.AckWait(nq.config.AckWait))
	opts = append(opts, nats.MaxDeliver(nq.config.MaxDeliver))

	sub, err := nq.js.PullSubscribe(subject, "", opts...)
	if err != nil {
		return err
	}

	nq.llmSubscription = sub

	ctx, cancel := context.WithCancel(context.Background())
	nq.cancelFuncs = append(nq.cancelFuncs, cancel)

	nq.consumerWG.Add(1)
	go nq.consumeLLMResponses(ctx, sub)

	return nil
}

// getSubject returns the NATS subject for a given message type
func (nq *NATSQueue) getSubject(messageType string) string {
	return fmt.Sprintf("chat.sessions.%s.%s", nq.sessionID, messageType)
}

// getStreamName returns the NATS stream name for a given message type
func (nq *NATSQueue) getStreamName(messageType string) string {
	return fmt.Sprintf("CHAT_%s_%s", nq.sessionID, messageType)
}

// PublishMessage sends a ChatResponse message to NATS
func (nq *NATSQueue) PublishMessage(ctx context.Context, msg *ChatResponse) error {
	return nq.publishToNATS(ctx, MessageTypeChatResponse, msg)
}

// PublishStream sends stream data to NATS
func (nq *NATSQueue) PublishStream(ctx context.Context, data []byte) error {
	return nq.publishToNATS(ctx, MessageTypeStream, data)
}

// PublishError sends an error to NATS
func (nq *NATSQueue) PublishError(ctx context.Context, err error) error {
	return nq.publishToNATS(ctx, MessageTypeError, err.Error())
}

// PublishLLMResponse sends an LLM response to NATS
func (nq *NATSQueue) PublishLLMResponse(ctx context.Context, resp *LLMResponseWrapper) error {
	return nq.publishToNATS(ctx, MessageTypeLLMResponse, resp)
}

// publishToNATS is a generic method to publish any message type
func (nq *NATSQueue) publishToNATS(ctx context.Context, messageType string, data interface{}) error {
	nq.closeMux.RLock()
	defer nq.closeMux.RUnlock()

	if nq.closed {
		return fmt.Errorf("queue closed")
	}

	var dataBytes []byte
	var err error

	// Special handling for LLM responses to make them NATS-serializable
	if messageType == MessageTypeLLMResponse {
		if llmResp, ok := data.(*LLMResponseWrapper); ok {
			// Convert to NATS-safe version (without Opts field)
			natsResp := LLMResponseWrapperForNATS{
				Response: convertToNATSSafeResponse(llmResp.Response),
			}
			dataBytes, err = json.Marshal(natsResp)
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

	// Create NATS message with metadata
	natsMsg := NATSMessage{
		Type:      messageType,
		SessionID: nq.sessionID,
		Timestamp: time.Now(),
		Data:      dataBytes,
	}

	msgBytes, err := json.Marshal(natsMsg)
	if err != nil {
		return fmt.Errorf("failed to serialize NATS message: %w", err)
	}

	// Publish to appropriate subject
	subject := nq.getSubject(messageType)
	_, err = nq.js.PublishAsync(subject, msgBytes)
	if err != nil {
		return fmt.Errorf("failed to publish message: %w", err)
	}

	// Wait for acknowledgment with context timeout
	select {
	case <-nq.js.PublishAsyncComplete():
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// ConsumeMessages returns the local channel for ChatResponse messages
func (nq *NATSQueue) ConsumeMessages(ctx context.Context) <-chan *ChatResponse {
	return nq.messagesChan
}

// ConsumeStream returns the local channel for stream data
func (nq *NATSQueue) ConsumeStream(ctx context.Context) <-chan []byte {
	return nq.streamChan
}

// ConsumeErrors returns the local channel for errors
func (nq *NATSQueue) ConsumeErrors(ctx context.Context) <-chan error {
	return nq.errorsChan
}

// ConsumeLLMResponses returns the local channel for LLM responses
func (nq *NATSQueue) ConsumeLLMResponses(ctx context.Context) <-chan *LLMResponseWrapper {
	return nq.llmResponsesChan
}

// consumeMessages goroutine that consumes ChatResponse messages from NATS
func (nq *NATSQueue) consumeMessages(ctx context.Context, sub *nats.Subscription) {
	defer nq.consumerWG.Done()

	retries := 0
	for {
		select {
		case <-ctx.Done():
			return
		default:
			// Create timeout context for this fetch operation
			fetchCtx, cancel := context.WithTimeout(ctx, nq.config.FetchTimeout)

			// Fetch next message with timeout
			msgs, err := sub.Fetch(1, nats.Context(fetchCtx))
			cancel() // Clean up timeout context

			if err != nil {
				if err == context.Canceled {
					return // Parent context cancelled
				}
				if err == nats.ErrTimeout || err == context.DeadlineExceeded {
					// Normal timeout, continue polling
					continue
				}

				// Handle other errors with backoff
				retries++
				if retries <= nq.config.MaxRetries {
					slog.Warn("fetch message error, retrying", "session_id", nq.sessionID, "error", err, "retry", retries, "max_retries", nq.config.MaxRetries)
					select {
					case <-ctx.Done():
						return
					case <-time.After(nq.config.RetryInterval * time.Duration(retries)):
						// Exponential backoff
					}
					continue
				} else {
					slog.Error("failed to fetch message after retries", "session_id", nq.sessionID, "error", err, "retries", retries)
					retries = 0 // Reset retry counter
					// Wait before trying again
					select {
					case <-ctx.Done():
						return
					case <-time.After(nq.config.RetryInterval * 5):
						// Longer wait after max retries
					}
					continue
				}
			}

			// Reset retry counter on successful fetch
			retries = 0

			for _, msg := range msgs {
				if err := nq.handleNATSMessage(msg, nq.messagesChan); err != nil {
					slog.Error("failed to handle message", "session_id", nq.sessionID, "error", err)
				}
				msg.Ack()
			}
		}
	}
}

// consumeStream goroutine that consumes stream data from NATS
func (nq *NATSQueue) consumeStream(ctx context.Context, sub *nats.Subscription) {
	defer nq.consumerWG.Done()

	retries := 0
	for {
		select {
		case <-ctx.Done():
			return
		default:
			// Create timeout context for this fetch operation
			fetchCtx, cancel := context.WithTimeout(ctx, nq.config.FetchTimeout)

			// Fetch next message with timeout
			msgs, err := sub.Fetch(1, nats.Context(fetchCtx))
			cancel() // Clean up timeout context

			if err != nil {
				if err == context.Canceled {
					return // Parent context cancelled
				}
				if err == nats.ErrTimeout || err == context.DeadlineExceeded {
					// Normal timeout, continue polling
					continue
				}

				// Handle other errors with backoff
				retries++
				if retries <= nq.config.MaxRetries {
					slog.Warn("fetch stream error, retrying", "session_id", nq.sessionID, "error", err, "retry", retries, "max_retries", nq.config.MaxRetries)
					select {
					case <-ctx.Done():
						return
					case <-time.After(nq.config.RetryInterval * time.Duration(retries)):
						// Exponential backoff
					}
					continue
				} else {
					slog.Error("failed to fetch stream after retries", "session_id", nq.sessionID, "error", err, "retries", retries)
					retries = 0 // Reset retry counter
					// Wait before trying again
					select {
					case <-ctx.Done():
						return
					case <-time.After(nq.config.RetryInterval * 5):
						// Longer wait after max retries
					}
					continue
				}
			}

			// Reset retry counter on successful fetch
			retries = 0

			for _, msg := range msgs {
				if err := nq.handleNATSMessage(msg, nq.streamChan); err != nil {
					slog.Error("failed to handle stream", "session_id", nq.sessionID, "error", err)
				}
				msg.Ack()
			}
		}
	}
}

// consumeErrors goroutine that consumes error messages from NATS
func (nq *NATSQueue) consumeErrors(ctx context.Context, sub *nats.Subscription) {
	defer nq.consumerWG.Done()

	retries := 0
	for {
		select {
		case <-ctx.Done():
			return
		default:
			// Create timeout context for this fetch operation
			fetchCtx, cancel := context.WithTimeout(ctx, nq.config.FetchTimeout)

			// Fetch next message with timeout
			msgs, err := sub.Fetch(1, nats.Context(fetchCtx))
			cancel() // Clean up timeout context

			if err != nil {
				if err == context.Canceled {
					return // Parent context cancelled
				}
				if err == nats.ErrTimeout || err == context.DeadlineExceeded {
					// Normal timeout, continue polling
					continue
				}

				// Handle other errors with backoff
				retries++
				if retries <= nq.config.MaxRetries {
					slog.Warn("fetch error error, retrying", "session_id", nq.sessionID, "error", err, "retry", retries, "max_retries", nq.config.MaxRetries)
					select {
					case <-ctx.Done():
						return
					case <-time.After(nq.config.RetryInterval * time.Duration(retries)):
						// Exponential backoff
					}
					continue
				} else {
					slog.Error("failed to fetch error after retries", "session_id", nq.sessionID, "error", err, "retries", retries)
					retries = 0 // Reset retry counter
					// Wait before trying again
					select {
					case <-ctx.Done():
						return
					case <-time.After(nq.config.RetryInterval * 5):
						// Longer wait after max retries
					}
					continue
				}
			}

			// Reset retry counter on successful fetch
			retries = 0

			for _, msg := range msgs {
				if err := nq.handleNATSMessage(msg, nq.errorsChan); err != nil {
					slog.Error("failed to handle error message", "session_id", nq.sessionID, "error", err)
				}
				msg.Ack()
			}
		}
	}
}

// consumeLLMResponses goroutine that consumes LLM responses from NATS
func (nq *NATSQueue) consumeLLMResponses(ctx context.Context, sub *nats.Subscription) {
	defer nq.consumerWG.Done()

	retries := 0
	for {
		select {
		case <-ctx.Done():
			return
		default:
			// Create timeout context for this fetch operation
			fetchCtx, cancel := context.WithTimeout(ctx, nq.config.FetchTimeout)

			// Fetch next message with timeout
			msgs, err := sub.Fetch(1, nats.Context(fetchCtx))
			cancel() // Clean up timeout context

			if err != nil {
				if err == context.Canceled {
					return // Parent context cancelled
				}
				if err == nats.ErrTimeout || err == context.DeadlineExceeded {
					// Normal timeout, continue polling
					continue
				}

				// Handle other errors with backoff
				retries++
				if retries <= nq.config.MaxRetries {
					slog.Warn("fetch LLM response error, retrying", "session_id", nq.sessionID, "error", err, "retry", retries, "max_retries", nq.config.MaxRetries)
					select {
					case <-ctx.Done():
						return
					case <-time.After(nq.config.RetryInterval * time.Duration(retries)):
						// Exponential backoff
					}
					continue
				} else {
					slog.Error("failed to fetch LLM response after retries", "session_id", nq.sessionID, "error", err, "retries", retries)
					retries = 0 // Reset retry counter
					// Wait before trying again
					select {
					case <-ctx.Done():
						return
					case <-time.After(nq.config.RetryInterval * 5):
						// Longer wait after max retries
					}
					continue
				}
			}

			// Reset retry counter on successful fetch
			retries = 0

			for _, msg := range msgs {
				if err := nq.handleNATSMessage(msg, nq.llmResponsesChan); err != nil {
					slog.Error("failed to handle LLM response", "session_id", nq.sessionID, "error", err)
				}
				msg.Ack()
			}
		}
	}
}

// handleNATSMessage processes a NATS message and routes to appropriate channel
func (nq *NATSQueue) handleNATSMessage(msg *nats.Msg, targetChan interface{}) error {
	// Deserialize NATS message
	var natsMsg NATSMessage
	if err := json.Unmarshal(msg.Data, &natsMsg); err != nil {
		return fmt.Errorf("failed to unmarshal NATS message: %w", err)
	}

	// Route to appropriate channel based on type
	switch natsMsg.Type {
	case MessageTypeChatResponse:
		var chatResp ChatResponse
		if err := json.Unmarshal(natsMsg.Data, &chatResp); err != nil {
			return fmt.Errorf("failed to unmarshal ChatResponse: %w", err)
		}

		ch := targetChan.(chan *ChatResponse)
		select {
		case ch <- &chatResp:
		default:
			slog.Warn("message channel full, dropping message", "session_id", nq.sessionID)
		}

	case MessageTypeStream:
		var streamData []byte
		if err := json.Unmarshal(natsMsg.Data, &streamData); err != nil {
			return fmt.Errorf("failed to unmarshal stream data: %w", err)
		}

		ch := targetChan.(chan []byte)
		select {
		case ch <- streamData:
		default:
			slog.Warn("stream channel full, dropping data", "session_id", nq.sessionID)
		}

	case MessageTypeError:
		var errorStr string
		if err := json.Unmarshal(natsMsg.Data, &errorStr); err != nil {
			return fmt.Errorf("failed to unmarshal error: %w", err)
		}

		ch := targetChan.(chan error)
		select {
		case ch <- fmt.Errorf(errorStr):
		default:
			slog.Warn("error channel full, dropping error", "session_id", nq.sessionID)
		}

	case MessageTypeLLMResponse:
		slog.Debug("handling LLM response message", "session_id", nq.sessionID, "data_size", len(natsMsg.Data))

		// First deserialize the NATS-safe version (without Opts field)
		var natsResp LLMResponseWrapperForNATS
		if err := json.Unmarshal(natsMsg.Data, &natsResp); err != nil {
			return fmt.Errorf("failed to unmarshal LLM response for NATS: %w", err)
		}

		// Create the full LLMResponseWrapper with empty Opts
		// (options will be regenerated from session state when needed)
		llmResp := &LLMResponseWrapper{
			Response: convertFromNATSSafeResponse(natsResp.Response),
			Opts:     nil, // Empty opts - will be regenerated from session state
		}

		slog.Debug("converted LLM response", "session_id", nq.sessionID,
			"choices_count", len(llmResp.Response.Choices),
			"content", func() string {
				if len(llmResp.Response.Choices) > 0 {
					return llmResp.Response.Choices[0].Content
				}
				return ""
			}())

		ch := targetChan.(chan *LLMResponseWrapper)
		select {
		case ch <- llmResp:
			slog.Debug("successfully sent LLM response to channel", "session_id", nq.sessionID)
		default:
			slog.Warn("LLM response channel full, dropping response", "session_id", nq.sessionID,
				"channel_len", len(ch), "channel_cap", cap(ch))
		}

	default:
		return fmt.Errorf("unknown message type: %s", natsMsg.Type)
	}

	return nil
}

// Close closes all channels and NATS connections
func (nq *NATSQueue) Close() error {
	nq.closeMux.Lock()
	defer nq.closeMux.Unlock()

	if nq.closed {
		return nil // Already closed
	}

	nq.closed = true

	// Cancel all consumer contexts
	for _, cancel := range nq.cancelFuncs {
		cancel()
	}

	// Wait for all consumers to finish
	nq.consumerWG.Wait()

	// Close local channels
	close(nq.messagesChan)
	close(nq.streamChan)
	close(nq.errorsChan)
	close(nq.llmResponsesChan)

	// Close NATS connection
	if nq.conn != nil {
		nq.conn.Close()
	}

	return nil
}

// QueueDepth returns the current depth of all local channels
// Note: This doesn't include messages pending in NATS streams
func (nq *NATSQueue) QueueDepth() (messages, stream, errors, llmResponses int) {
	nq.closeMux.RLock()
	defer nq.closeMux.RUnlock()

	if nq.closed {
		return 0, 0, 0, 0
	}

	return len(nq.messagesChan), len(nq.streamChan), len(nq.errorsChan), len(nq.llmResponsesChan)
}

// NATSQueueFactory creates NATS queue instances
type NATSQueueFactory struct {
	config NATSConfig
}

// NewNATSQueueFactory creates a new NATS factory with specified configuration
func NewNATSQueueFactory(config NATSConfig) *NATSQueueFactory {
	return &NATSQueueFactory{
		config: config,
	}
}

// CreateQueue creates a new NATS queue with session-specific configuration
func (f *NATSQueueFactory) CreateQueue(sessionID string, config map[string]interface{}) (MessageQueue, error) {
	natsConfig := f.config

	// Apply configuration overrides
	if config != nil {
		if bufferSize, ok := config["bufferSize"].(int); ok && bufferSize > 0 {
			natsConfig.BufferSize = bufferSize
		}
		if url, ok := config["natsURL"].(string); ok && url != "" {
			natsConfig.URL = url
		}
		if maxAge, ok := config["maxAge"].(string); ok && maxAge != "" {
			if duration, err := time.ParseDuration(maxAge); err == nil {
				natsConfig.MaxAge = duration
			}
		}
	}

	return NewNATSQueue(sessionID, natsConfig)
}

// Helper function for creating a NATS queue with default persistent settings
func NewDefaultNATSQueue(sessionID, natsURL string) (MessageQueue, error) {
	config := DefaultNATSConfig()
	if natsURL != "" {
		config.URL = natsURL
	}
	return NewNATSQueue(sessionID, config)
}
