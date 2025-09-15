// internal/grpc/simple_client.go
package grpc

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"strings"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// SimpleEdgeClient provides basic edge functionality for testing
type SimpleEdgeClient struct {
	config      *config.Config
	conn        *grpc.ClientConn
	client      pb.ConfigurationSyncServiceClient
	configCache *pb.ConfigurationSnapshot

	// Build information
	version    string
	buildHash  string
	buildTime  string

	// Bidirectional streaming
	stream       pb.ConfigurationSyncService_SubscribeToChangesClient
	streamCtx    context.Context
	streamCancel context.CancelFunc

	// Callback for configuration updates
	onConfigChange func(*pb.ConfigurationSnapshot)

	// Reload handling (use interface to avoid import cycle)
	reloadHandler interface{}

	// Connection state
	connected bool

	// Reconnection handling
	reconnecting      bool
	reconnectAttempts int
	maxReconnects     int
	reconnectInterval time.Duration
}

// NewSimpleEdgeClient creates a basic edge client
func NewSimpleEdgeClient(cfg *config.Config, version, buildHash, buildTime string) *SimpleEdgeClient {
	return &SimpleEdgeClient{
		config:            cfg,
		version:           version,
		buildHash:         buildHash,
		buildTime:         buildTime,
		maxReconnects:     -1,                // Unlimited reconnections
		reconnectInterval: 5 * time.Second,   // 5 second retry interval
	}
}

// Start connects to the control server and gets initial configuration
func (c *SimpleEdgeClient) Start() error {
	log.Info().
		Str("control_endpoint", c.config.HubSpoke.ControlEndpoint).
		Str("edge_id", c.config.HubSpoke.EdgeID).
		Msg("Connecting to control server")

	// Connect to control server
	conn, err := c.dialWithKeepalive()
	if err != nil {
		return fmt.Errorf("failed to connect to control server: %w", err)
	}

	c.conn = conn
	c.client = pb.NewConfigurationSyncServiceClient(conn)

	// Test basic connectivity and register
	if err := c.registerWithControl(); err != nil {
		conn.Close()
		return fmt.Errorf("failed to register with control: %w", err)
	}

	// Establish bidirectional streaming connection
	if err := c.establishStream(); err != nil {
		conn.Close()
		return fmt.Errorf("failed to establish streaming: %w", err)
	}

	log.Info().Msg("Successfully connected to control server with streaming")
	return nil
}

// registerWithControl registers this edge with the control server
func (c *SimpleEdgeClient) registerWithControl() error {
	ctx := context.Background()

	// Create registration request
	req := &pb.EdgeRegistrationRequest{
		EdgeId:        c.config.HubSpoke.EdgeID,
		EdgeNamespace: c.config.HubSpoke.EdgeNamespace,
		Version:       c.version,
		BuildHash:     c.buildHash,
		Metadata: map[string]string{
			"test":       "true",
			"build_time": c.buildTime,
		},
		Health: &pb.HealthStatus{
			Status:    pb.HealthStatus_HEALTHY,
			Message:   "Starting up",
			Timestamp: timestamppb.Now(),
		},
	}

	// Register with control
	resp, err := c.client.RegisterEdge(ctx, req)
	if err != nil {
		return fmt.Errorf("registration failed: %w", err)
	}

	if !resp.Success {
		return fmt.Errorf("registration rejected: %s", resp.Message)
	}

	// Store initial configuration if provided
	if resp.InitialConfig != nil {
		log.Info().
			Str("version", resp.InitialConfig.Version).
			Int32("llm_count", int32(len(resp.InitialConfig.Llms))).
			Int32("app_count", int32(len(resp.InitialConfig.Apps))).
			Msg("Received initial configuration from control")

		c.configCache = resp.InitialConfig

		// Notify provider if callback is set
		if c.onConfigChange != nil {
			c.onConfigChange(resp.InitialConfig)
		}
	}

	log.Info().
		Str("session_id", resp.SessionId).
		Msg("Successfully registered with control server")

	return nil
}

// SetOnConfigChange sets the callback for configuration changes
func (c *SimpleEdgeClient) SetOnConfigChange(callback func(*pb.ConfigurationSnapshot)) {
	c.onConfigChange = callback
}

// IsConnected returns true if connected to control
func (c *SimpleEdgeClient) IsConnected() bool {
	return c.conn != nil
}

// GetCurrentConfiguration returns the cached configuration
func (c *SimpleEdgeClient) GetCurrentConfiguration() *pb.ConfigurationSnapshot {
	return c.configCache
}

// ValidateTokenOnDemand validates a token by calling the control instance
func (c *SimpleEdgeClient) ValidateTokenOnDemand(token string) (*pb.TokenValidationResponse, error) {
	if c.conn == nil || c.client == nil {
		return nil, fmt.Errorf("not connected to control instance")
	}

	tokenPrefix := token
	if len(token) > 8 {
		tokenPrefix = token[:8]
	}

	log.Info().Str("token_prefix", tokenPrefix).Msg("SimpleEdgeClient: making on-demand token validation request to control")

	ctx := context.Background()

	// Create token validation request
	req := &pb.TokenValidationRequest{
		Token:         token,
		EdgeId:        c.config.HubSpoke.EdgeID,
		EdgeNamespace: c.config.HubSpoke.EdgeNamespace,
	}

	// Call control instance
	resp, err := c.client.ValidateToken(ctx, req)
	if err != nil {
		log.Info().Err(err).Str("token_prefix", tokenPrefix).Msg("SimpleEdgeClient: token validation gRPC call failed")
		return nil, fmt.Errorf("token validation failed: %w", err)
	}

	log.Info().
		Str("token_prefix", tokenPrefix).
		Bool("valid", resp.Valid).
		Uint32("app_id", resp.AppId).
		Msg("SimpleEdgeClient: received token validation response from control")

	return resp, nil
}

// SetReloadHandler sets the reload handler for processing reload requests
func (c *SimpleEdgeClient) SetReloadHandler(handler interface{}) {
	c.reloadHandler = handler
	log.Info().Msg("Reload handler set for edge client")
}

// GetGRPCClient returns the gRPC client for use by pulse manager
func (c *SimpleEdgeClient) GetGRPCClient() pb.ConfigurationSyncServiceClient {
	return c.client
}

// GetEdgeID returns the edge ID
func (c *SimpleEdgeClient) GetEdgeID() string {
	return c.config.HubSpoke.EdgeID
}

// GetEdgeNamespace returns the edge namespace
func (c *SimpleEdgeClient) GetEdgeNamespace() string {
	return c.config.HubSpoke.EdgeNamespace
}

// RequestFullSync requests a full configuration sync from control (for reload operations)
func (c *SimpleEdgeClient) RequestFullSync() error {
	log.Info().Msg("Requesting full configuration sync from control")

	ctx := context.Background()
	req := &pb.ConfigurationRequest{
		EdgeNamespace: c.config.HubSpoke.EdgeNamespace,
	}

	resp, err := c.client.GetFullConfiguration(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to request full sync: %w", err)
	}

	// Update local cache and trigger callback
	c.configCache = resp
	if c.onConfigChange != nil {
		c.onConfigChange(resp)
	}

	log.Info().
		Str("version", resp.Version).
		Int("llm_count", len(resp.Llms)).
		Int("app_count", len(resp.Apps)).
		Msg("Full configuration sync completed")

	return nil
}

// establishStream establishes bidirectional streaming with control server
func (c *SimpleEdgeClient) establishStream() error {
	log.Info().Str("edge_id", c.config.HubSpoke.EdgeID).Msg("Establishing bidirectional stream with control")

	ctx, cancel := context.WithCancel(context.Background())
	c.streamCtx = ctx
	c.streamCancel = cancel

	// Start streaming
	stream, err := c.client.SubscribeToChanges(ctx)
	if err != nil {
		cancel()
		return fmt.Errorf("failed to start streaming: %w", err)
	}

	c.stream = stream

	// Send initial registration via stream to establish connection
	regMsg := &pb.EdgeMessage{
		Message: &pb.EdgeMessage_Registration{
			Registration: &pb.EdgeRegistrationRequest{
				EdgeId:        c.config.HubSpoke.EdgeID,
				EdgeNamespace: c.config.HubSpoke.EdgeNamespace,
				Version:       c.version,
				BuildHash:     c.buildHash,
				Metadata: map[string]string{
					"test":       "true",
					"build_time": c.buildTime,
				},
				Health: &pb.HealthStatus{
					Status:    pb.HealthStatus_HEALTHY,
					Message:   "Streaming connection established",
					Timestamp: timestamppb.Now(),
				},
			},
		},
	}

	if err := c.stream.Send(regMsg); err != nil {
		cancel()
		return fmt.Errorf("failed to send stream registration: %w", err)
	}

	// Start message handling goroutines
	go c.handleIncomingMessages()

	// Start heartbeat worker
	go c.heartbeatWorker()

	c.connected = true

	log.Info().Msg("Bidirectional stream established successfully")
	return nil
}

// handleIncomingMessages processes messages from control server with comprehensive error recovery
func (c *SimpleEdgeClient) handleIncomingMessages() {
	defer func() {
		// Handle panic recovery
		if r := recover(); r != nil {
			log.Error().Interface("panic", r).Msg("Panic in message handler, recovering")
		}

		c.connected = false
		if c.streamCancel != nil {
			c.streamCancel()
		}

		// Start reconnection process if not already stopping
		if c.conn != nil {
			go c.attemptReconnection()
		}
	}()

	for {
		msg, err := c.stream.Recv()
		if err != nil {
			// Categorize the error and handle accordingly
			errorCategory := c.categorizeStreamError(err)
			c.handleStreamError(err, errorCategory)
			return
		}

		// Process message with error handling
		if err := c.processControlMessage(msg); err != nil {
			log.Error().Err(err).Msg("Failed to process control message")
			// Continue processing other messages rather than failing the entire stream
		}
	}
}

// categorizeStreamError categorizes stream errors for appropriate recovery
func (c *SimpleEdgeClient) categorizeStreamError(err error) string {
	errStr := err.Error()

	switch {
	case strings.Contains(errStr, "connection refused"):
		return "CONNECTION_REFUSED"
	case strings.Contains(errStr, "deadline exceeded"):
		return "TIMEOUT"
	case strings.Contains(errStr, "context canceled"):
		return "CONTEXT_CANCELED"
	case strings.Contains(errStr, "transport is closing"):
		return "TRANSPORT_CLOSING"
	case strings.Contains(errStr, "connection reset"):
		return "CONNECTION_RESET"
	case strings.Contains(errStr, "EOF"):
		return "EOF"
	case strings.Contains(errStr, "keepalive watchdog timeout"):
		return "KEEPALIVE_TIMEOUT"
	case strings.Contains(errStr, "Unavailable"):
		return "SERVICE_UNAVAILABLE"
	default:
		return "UNKNOWN"
	}
}

// handleStreamError handles different categories of stream errors
func (c *SimpleEdgeClient) handleStreamError(err error, category string) {
	switch category {
	case "CONNECTION_REFUSED", "SERVICE_UNAVAILABLE":
		log.Error().Err(err).Str("category", category).Msg("Control server unavailable - will retry with backoff")
	case "TIMEOUT", "KEEPALIVE_TIMEOUT":
		log.Error().Err(err).Str("category", category).Msg("Stream timeout - connection may be unstable")
	case "CONTEXT_CANCELED":
		log.Debug().Err(err).Str("category", category).Msg("Stream context cancelled - stopping gracefully")
	case "TRANSPORT_CLOSING", "CONNECTION_RESET", "EOF":
		log.Error().Err(err).Str("category", category).Msg("Connection lost - initiating reconnection")
	default:
		log.Error().Err(err).Str("category", category).Msg("Unknown stream error - will attempt recovery")
	}
}

// processControlMessage processes individual control messages with error handling
func (c *SimpleEdgeClient) processControlMessage(msg *pb.ControlMessage) error {
	switch m := msg.Message.(type) {
	case *pb.ControlMessage_RegistrationResponse:
		return c.handleRegistrationResponse(m.RegistrationResponse)

	case *pb.ControlMessage_Configuration:
		return c.handleConfigurationUpdate(m.Configuration)

	case *pb.ControlMessage_Change:
		return c.handleConfigurationChange(m.Change)

	case *pb.ControlMessage_HeartbeatResponse:
		return c.handleHeartbeatResponse(m.HeartbeatResponse)

	case *pb.ControlMessage_Error:
		return c.handleControlError(m.Error)

	case *pb.ControlMessage_ReloadRequest:
		return c.handleReloadRequest(m.ReloadRequest)

	default:
		log.Warn().Msg("Unknown control message type received")
		return nil
	}
}

// handleRegistrationResponse processes registration responses
func (c *SimpleEdgeClient) handleRegistrationResponse(resp *pb.EdgeRegistrationResponse) error {
	log.Info().
		Bool("success", resp.Success).
		Str("message", resp.Message).
		Msg("Received stream registration response")

	if !resp.Success {
		return fmt.Errorf("registration failed: %s", resp.Message)
	}

	return nil
}

// handleConfigurationUpdate processes configuration updates
func (c *SimpleEdgeClient) handleConfigurationUpdate(config *pb.ConfigurationSnapshot) error {
	log.Info().
		Str("version", config.Version).
		Int("llm_count", len(config.Llms)).
		Int("app_count", len(config.Apps)).
		Msg("Received configuration update via stream")

	c.configCache = config

	// Call configuration change callback if set
	if c.onConfigChange != nil {
		c.onConfigChange(config)
	}

	return nil
}

// handleConfigurationChange processes incremental configuration changes
func (c *SimpleEdgeClient) handleConfigurationChange(change *pb.ConfigurationChange) error {
	log.Info().
		Str("change_type", change.ChangeType.String()).
		Str("entity_type", change.EntityType.String()).
		Uint32("entity_id", change.EntityId).
		Msg("Received configuration change via stream")

	// For now, just log the change. In a full implementation, this would
	// apply incremental updates to the configuration cache
	return nil
}

// handleHeartbeatResponse processes heartbeat responses
func (c *SimpleEdgeClient) handleHeartbeatResponse(resp *pb.HeartbeatResponse) error {
	log.Debug().
		Bool("acknowledged", resp.Acknowledged).
		Str("message", resp.Message).
		Bool("request_full_sync", resp.RequestFullSync).
		Bool("shutdown_requested", resp.ShutdownRequested).
		Msg("Received heartbeat response")

	// Handle control directives
	if resp.RequestFullSync {
		log.Info().Msg("Control server requested full configuration sync")
		return c.RequestFullSync()
	}

	if resp.ShutdownRequested {
		log.Info().Msg("Control server requested graceful shutdown")
		// Initiate graceful shutdown
		go c.Stop()
	}

	return nil
}

// handleControlError processes error messages from control server
func (c *SimpleEdgeClient) handleControlError(errMsg *pb.ErrorMessage) error {
	log.Error().
		Str("error_code", errMsg.Code).
		Str("error_message", errMsg.Message).
		Bool("fatal", errMsg.Fatal).
		Msg("Received error from control server")

	if errMsg.Fatal {
		log.Error().Msg("Fatal error from control server - initiating shutdown")
		go c.Stop()
		return fmt.Errorf("fatal error from control: %s", errMsg.Message)
	}

	return nil
}

// handleReloadRequest processes reload requests from control server
func (c *SimpleEdgeClient) handleReloadRequest(req *pb.ConfigurationReloadRequest) error {
	log.Info().
		Str("operation_id", req.OperationId).
		Str("target_namespace", req.TargetNamespace).
		Msg("Received reload request via stream")

	// Delegate to the existing reload handling logic
	c.HandleReloadRequest(req)
	return nil
}

// SendReloadStatus sends a reload status update to control server via stream
func (c *SimpleEdgeClient) SendReloadStatus(response *pb.ConfigurationReloadResponse) error {
	if c.stream == nil {
		return fmt.Errorf("no stream available for sending reload status")
	}

	log.Info().
		Str("operation_id", response.OperationId).
		Str("phase", response.Phase.String()).
		Bool("success", response.Success).
		Msg("Sending reload status to control via stream")

	msg := &pb.EdgeMessage{
		Message: &pb.EdgeMessage_ReloadResponse{
			ReloadResponse: response,
		},
	}

	if err := c.stream.Send(msg); err != nil {
		log.Error().Err(err).Msg("Failed to send reload status via stream")
		return fmt.Errorf("failed to send reload status: %w", err)
	}

	return nil
}

// HandleReloadRequest processes reload requests (to be called when reload messages are received)
func (c *SimpleEdgeClient) HandleReloadRequest(req *pb.ConfigurationReloadRequest) {
	log.Info().
		Str("operation_id", req.OperationId).
		Msg("SimpleEdgeClient received reload request")

	if c.reloadHandler != nil {
		if handler, ok := c.reloadHandler.(interface{ HandleReloadRequest(*pb.ConfigurationReloadRequest) }); ok {
			handler.HandleReloadRequest(req)
		} else {
			log.Error().Msg("Reload handler does not implement HandleReloadRequest method")
		}
	} else {
		log.Warn().Msg("No reload handler set - reload request ignored")
	}
}

// Stop closes the connection to control server
func (c *SimpleEdgeClient) Stop() error {
	log.Info().Msg("Stopping SimpleEdgeClient")

	// Cancel stream context
	if c.streamCancel != nil {
		c.streamCancel()
	}

	// Close connection
	if c.conn != nil {
		log.Info().Msg("Closing connection to control server")
		return c.conn.Close()
	}

	return nil
}

// heartbeatWorker sends periodic heartbeats to the control instance
func (c *SimpleEdgeClient) heartbeatWorker() {
	ticker := time.NewTicker(c.config.HubSpoke.HeartbeatInterval)
	defer ticker.Stop()

	log.Info().
		Dur("interval", c.config.HubSpoke.HeartbeatInterval).
		Msg("Starting heartbeat worker")

	for {
		select {
		case <-ticker.C:
			if c.connected && c.stream != nil {
				c.sendHeartbeat()
			} else {
				log.Debug().
					Bool("connected", c.connected).
					Bool("stream_not_nil", c.stream != nil).
					Msg("Skipping heartbeat - not connected or stream is nil")
			}
		case <-c.streamCtx.Done():
			log.Info().Msg("Heartbeat worker stopping")
			return
		}
	}
}

// sendHeartbeat sends a heartbeat message to the control instance
func (c *SimpleEdgeClient) sendHeartbeat() {
	heartbeat := &pb.EdgeMessage{
		Message: &pb.EdgeMessage_Heartbeat{
			Heartbeat: &pb.HeartbeatRequest{
				EdgeId:    c.config.HubSpoke.EdgeID,
				SessionId: "", // SimpleEdgeClient doesn't track session ID
				Health: &pb.HealthStatus{
					Status:    pb.HealthStatus_HEALTHY,
					Message:   "Operational",
					Timestamp: timestamppb.Now(),
				},
				Metrics:   c.collectBasicMetrics(),
				Timestamp: timestamppb.Now(),
			},
		},
	}

	if err := c.stream.Send(heartbeat); err != nil {
		log.Error().Err(err).Msg("Failed to send heartbeat")
	} else {
		log.Debug().
			Str("edge_id", c.config.HubSpoke.EdgeID).
			Msg("Heartbeat sent successfully")
	}
}

// collectBasicMetrics gathers basic runtime metrics for heartbeat
func (c *SimpleEdgeClient) collectBasicMetrics() *pb.EdgeMetrics {
	// Simple metrics collection for SimpleEdgeClient
	return &pb.EdgeMetrics{
		RequestsProcessed: 0, // SimpleEdgeClient doesn't track requests
		ActiveConnections: 0, // SimpleEdgeClient doesn't track connections
		CpuUsagePercent:   0, // Simplified - no CPU monitoring
		MemoryUsageBytes:  0, // Simplified - no memory monitoring
		UptimeSeconds:     0, // Simplified - no uptime tracking
	}
}

// attemptReconnection handles automatic reconnection to control server with exponential backoff
func (c *SimpleEdgeClient) attemptReconnection() {
	if c.reconnecting {
		return // Already attempting reconnection
	}

	c.reconnecting = true
	c.reconnectAttempts = 0
	defer func() {
		c.reconnecting = false
	}()

	log.Info().Msg("Starting automatic reconnection to control server with exponential backoff")

	// Initial backoff parameters
	baseDelay := c.reconnectInterval
	maxDelay := 5 * time.Minute
	backoffMultiplier := 2.0
	jitterFactor := 0.1

	for {
		c.reconnectAttempts++

		// Check if we should stop reconnecting
		if c.maxReconnects > 0 && c.reconnectAttempts > c.maxReconnects {
			log.Error().
				Int("attempts", c.reconnectAttempts).
				Int("max_reconnects", c.maxReconnects).
				Msg("Maximum reconnection attempts reached")
			return
		}

		// Calculate exponential backoff with jitter
		backoffDelay := c.calculateBackoffDelay(baseDelay, maxDelay, backoffMultiplier, jitterFactor, c.reconnectAttempts)

		log.Info().
			Int("attempt", c.reconnectAttempts).
			Dur("backoff_delay", backoffDelay).
			Msg("Attempting to reconnect to control server")

		// Wait with exponential backoff before attempting reconnection
		select {
		case <-time.After(backoffDelay):
			// Continue with reconnection attempt
		case <-c.streamCtx.Done():
			log.Info().Msg("Reconnection cancelled due to context cancellation")
			return
		}

		// Check if we should stop (connection was manually closed)
		if c.conn == nil {
			log.Info().Msg("Connection manually closed, stopping reconnection attempts")
			return
		}

		// Attempt to re-establish connection and stream
		if err := c.reconnectWithRetry(); err != nil {
			log.Error().
				Err(err).
				Int("attempt", c.reconnectAttempts).
				Dur("next_retry_in", c.calculateBackoffDelay(baseDelay, maxDelay, backoffMultiplier, jitterFactor, c.reconnectAttempts+1)).
				Msg("Failed to reconnect, will retry with exponential backoff")
			continue
		}

		// Success!
		log.Info().
			Int("attempts", c.reconnectAttempts).
			Msg("Successfully reconnected to control server")

		// Reset attempt counter
		c.reconnectAttempts = 0
		return
	}
}

// calculateBackoffDelay calculates the delay for exponential backoff with jitter
func (c *SimpleEdgeClient) calculateBackoffDelay(baseDelay, maxDelay time.Duration, multiplier, jitterFactor float64, attempt int) time.Duration {
	// Calculate exponential backoff
	backoff := float64(baseDelay) * math.Pow(multiplier, float64(attempt-1))

	// Cap at maximum delay
	if backoff > float64(maxDelay) {
		backoff = float64(maxDelay)
	}

	// Add jitter (random variation of ±jitterFactor * backoff)
	if jitterFactor > 0 {
		jitter := backoff * jitterFactor * (2.0*rand.Float64() - 1.0) // ±jitterFactor
		backoff += jitter

		// Ensure minimum delay
		if backoff < float64(baseDelay) {
			backoff = float64(baseDelay)
		}
	}

	return time.Duration(backoff)
}

// reconnectWithRetry attempts to reconnect with proper error handling
func (c *SimpleEdgeClient) reconnectWithRetry() error {
	// First, try to close the existing broken connection
	if c.conn != nil {
		if err := c.conn.Close(); err != nil {
			log.Debug().Err(err).Msg("Error closing broken connection during reconnect")
		}
		c.conn = nil
		c.client = nil
	}

	// Re-establish gRPC connection
	conn, err := c.dialWithKeepalive()
	if err != nil {
		return fmt.Errorf("failed to re-establish gRPC connection: %w", err)
	}

	c.conn = conn
	c.client = pb.NewConfigurationSyncServiceClient(conn)

	// Test connectivity by registering with control
	if err := c.registerWithControl(); err != nil {
		conn.Close()
		c.conn = nil
		c.client = nil
		return fmt.Errorf("failed to re-register with control: %w", err)
	}

	// Re-establish stream
	if err := c.establishStream(); err != nil {
		conn.Close()
		c.conn = nil
		c.client = nil
		return fmt.Errorf("failed to re-establish stream: %w", err)
	}

	log.Info().Msg("Successfully reconnected to control server")
	return nil
}

// dialWithKeepalive creates a gRPC connection with proper keepalive settings
func (c *SimpleEdgeClient) dialWithKeepalive() (*grpc.ClientConn, error) {
	// Configure keepalive settings for reliable bidirectional streaming
	keepaliveParams := keepalive.ClientParameters{
		Time:                30 * time.Second, // Send pings every 30 seconds if no activity
		Timeout:             5 * time.Second,  // Wait 5 seconds for ping response
		PermitWithoutStream: true,             // Send pings even without active streams
	}

	// Setup dial options
	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithKeepaliveParams(keepaliveParams),
	}

	// Dial with keepalive settings
	conn, err := grpc.Dial(c.config.HubSpoke.ControlEndpoint, opts...)
	if err != nil {
		return nil, err
	}

	return conn, nil
}