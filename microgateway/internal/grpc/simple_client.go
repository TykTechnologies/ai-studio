// internal/grpc/simple_client.go
package grpc

import (
	"context"
	"fmt"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
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
	conn, err := grpc.Dial(c.config.HubSpoke.ControlEndpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
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

// handleIncomingMessages processes messages from control server
func (c *SimpleEdgeClient) handleIncomingMessages() {
	defer func() {
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
			log.Error().Err(err).Msg("Stream receive error - connection lost")
			return
		}

		switch m := msg.Message.(type) {
		case *pb.ControlMessage_RegistrationResponse:
			log.Info().
				Bool("success", m.RegistrationResponse.Success).
				Str("message", m.RegistrationResponse.Message).
				Msg("Received stream registration response")

		case *pb.ControlMessage_Configuration:
			log.Info().
				Str("version", m.Configuration.Version).
				Int("llm_count", len(m.Configuration.Llms)).
				Int("app_count", len(m.Configuration.Apps)).
				Msg("Received configuration update via stream")

			c.configCache = m.Configuration
			if c.onConfigChange != nil {
				c.onConfigChange(m.Configuration)
			}

		case *pb.ControlMessage_ReloadRequest:
			log.Info().
				Str("operation_id", m.ReloadRequest.OperationId).
				Str("target_namespace", m.ReloadRequest.TargetNamespace).
				Msg("Received reload request via stream")

			c.HandleReloadRequest(m.ReloadRequest)

		case *pb.ControlMessage_HeartbeatResponse:
			log.Debug().Bool("acknowledged", m.HeartbeatResponse.Acknowledged).Msg("Heartbeat acknowledged")

		case *pb.ControlMessage_Error:
			log.Error().
				Str("code", m.Error.Code).
				Str("message", m.Error.Message).
				Bool("fatal", m.Error.Fatal).
				Msg("Received error from control")
		}
	}
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

// attemptReconnection handles automatic reconnection to control server
func (c *SimpleEdgeClient) attemptReconnection() {
	if c.reconnecting {
		return // Already attempting reconnection
	}

	c.reconnecting = true
	c.reconnectAttempts = 0
	defer func() {
		c.reconnecting = false
	}()

	log.Info().Msg("Starting automatic reconnection to control server")

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

		log.Info().
			Int("attempt", c.reconnectAttempts).
			Dur("retry_in", c.reconnectInterval).
			Msg("Attempting to reconnect to control server")

		// Wait before attempting reconnection
		time.Sleep(c.reconnectInterval)

		// Check if we should stop (connection was manually closed)
		if c.conn == nil {
			log.Info().Msg("Connection manually closed, stopping reconnection attempts")
			return
		}

		// Attempt to re-establish stream
		if err := c.establishStream(); err != nil {
			log.Error().
				Err(err).
				Int("attempt", c.reconnectAttempts).
				Msg("Failed to re-establish stream")
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