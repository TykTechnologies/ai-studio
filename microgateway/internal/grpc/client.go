// internal/grpc/client.go
package grpc

import (
	"context"
	"crypto/tls"
	"fmt"
	"sync"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	pb "github.com/TykTechnologies/midsommar/microgateway/proto"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// EdgeClient handles communication with the control instance
type EdgeClient struct {
	config     *config.Config
	edgeID     string
	sessionID  string
	
	// gRPC connection
	conn       *grpc.ClientConn
	client     pb.ConfigurationSyncServiceServer
	stream     pb.ConfigurationSyncService_SubscribeToChangesClient
	
	// Configuration cache
	configCache *pb.ConfigurationSnapshot
	cacheMutex  sync.RWMutex
	
	// Connection management
	connected   bool
	reconnectCh chan bool
	ctx         context.Context
	cancel      context.CancelFunc
	
	// Callbacks
	onConfigChange func(*pb.ConfigurationSnapshot)
}

// NewEdgeClient creates a new edge client
func NewEdgeClient(cfg *config.Config) *EdgeClient {
	ctx, cancel := context.WithCancel(context.Background())
	
	// Generate edge ID if not configured
	edgeID := cfg.HubSpoke.EdgeID
	if edgeID == "" {
		edgeID = uuid.New().String()
		log.Warn().Str("edge_id", edgeID).Msg("No edge ID configured, generated one")
	}
	
	client := &EdgeClient{
		config:      cfg,
		edgeID:      edgeID,
		reconnectCh: make(chan bool, 1),
		ctx:         ctx,
		cancel:      cancel,
	}
	
	return client
}

// Start starts the edge client and establishes connection to control
func (c *EdgeClient) Start() error {
	log.Info().
		Str("control_endpoint", c.config.HubSpoke.ControlEndpoint).
		Str("edge_id", c.edgeID).
		Str("namespace", c.config.HubSpoke.EdgeNamespace).
		Msg("Starting edge client")
	
	// Start connection management goroutine
	go c.connectionManager()
	
	// Start heartbeat goroutine
	go c.heartbeatWorker()
	
	// Initiate first connection
	c.reconnectCh <- true
	
	return nil
}

// Stop stops the edge client gracefully
func (c *EdgeClient) Stop() error {
	log.Info().Msg("Stopping edge client")
	
	if c.stream != nil {
		// Send unregistration message
		unregMsg := &pb.EdgeMessage{
			Message: &pb.EdgeMessage_Unregistration{
				Unregistration: &pb.EdgeUnregistrationRequest{
					EdgeId:    c.edgeID,
					SessionId: c.sessionID,
					Reason:    "Graceful shutdown",
				},
			},
		}
		c.stream.Send(unregMsg)
	}
	
	c.cancel()
	
	if c.conn != nil {
		c.conn.Close()
	}
	
	return nil
}

// SetOnConfigChange sets the callback for configuration changes
func (c *EdgeClient) SetOnConfigChange(callback func(*pb.ConfigurationSnapshot)) {
	c.onConfigChange = callback
}

// GetCurrentConfiguration returns the current cached configuration
func (c *EdgeClient) GetCurrentConfiguration() *pb.ConfigurationSnapshot {
	c.cacheMutex.RLock()
	defer c.cacheMutex.RUnlock()
	
	return c.configCache
}

// connectionManager handles connection lifecycle
func (c *EdgeClient) connectionManager() {
	backoffDuration := c.config.HubSpoke.ReconnectInterval
	maxBackoff := 5 * time.Minute
	
	for {
		select {
		case <-c.reconnectCh:
			backoffDuration = c.config.HubSpoke.ReconnectInterval
		case <-time.After(backoffDuration):
			// Retry connection
		case <-c.ctx.Done():
			return
		}
		
		if !c.connected {
			if err := c.connect(); err != nil {
				log.Error().Err(err).
					Dur("backoff", backoffDuration).
					Msg("Failed to connect to control, retrying")
				
				// Exponential backoff
				backoffDuration *= 2
				if backoffDuration > maxBackoff {
					backoffDuration = maxBackoff
				}
			} else {
				log.Info().Msg("Successfully connected to control instance")
				backoffDuration = c.config.HubSpoke.ReconnectInterval
			}
		}
	}
}

// connect establishes connection to the control instance
func (c *EdgeClient) connect() error {
	// Setup gRPC connection options
	var opts []grpc.DialOption
	
	// Configure TLS
	if c.config.HubSpoke.ClientTLSEnabled {
		var tlsConfig *tls.Config
		
		if c.config.HubSpoke.SkipTLSVerify {
			tlsConfig = &tls.Config{InsecureSkipVerify: true}
		} else {
			tlsConfig = &tls.Config{}
			
			// Load client certificates if configured
			if c.config.HubSpoke.ClientTLSCertPath != "" && c.config.HubSpoke.ClientTLSKeyPath != "" {
				cert, err := tls.LoadX509KeyPair(
					c.config.HubSpoke.ClientTLSCertPath,
					c.config.HubSpoke.ClientTLSKeyPath,
				)
				if err != nil {
					return fmt.Errorf("failed to load client certificates: %w", err)
				}
				tlsConfig.Certificates = []tls.Certificate{cert}
			}
		}
		
		opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)))
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}
	
	// Establish connection
	conn, err := grpc.Dial(c.config.HubSpoke.ControlEndpoint, opts...)
	if err != nil {
		return fmt.Errorf("failed to connect to control: %w", err)
	}
	
	c.conn = conn
	c.client = pb.NewConfigurationSyncServiceClient(conn)
	
	// Register with control instance
	if err := c.register(); err != nil {
		conn.Close()
		return fmt.Errorf("failed to register with control: %w", err)
	}
	
	// Start streaming
	if err := c.startStreaming(); err != nil {
		conn.Close()
		return fmt.Errorf("failed to start streaming: %w", err)
	}
	
	c.connected = true
	return nil
}

// register registers this edge instance with the control
func (c *EdgeClient) register() error {
	ctx := context.Background()
	
	// Add authentication if configured
	if c.config.HubSpoke.ClientToken != "" {
		md := metadata.New(map[string]string{
			"authorization": "Bearer " + c.config.HubSpoke.ClientToken,
		})
		ctx = metadata.NewOutgoingContext(ctx, md)
	}
	
	// Create registration request
	req := &pb.EdgeRegistrationRequest{
		EdgeId:        c.edgeID,
		EdgeNamespace: c.config.HubSpoke.EdgeNamespace,
		Version:       "dev", // TODO: Get from build info
		BuildHash:     "unknown", // TODO: Get from build info
		Metadata: map[string]string{
			"hostname": getHostname(),
		},
		Health: &pb.HealthStatus{
			Status:    pb.HealthStatus_HEALTHY,
			Message:   "Starting up",
			Timestamp: timestamppb.Now(),
		},
	}
	
	// Register with timeout
	ctx, cancel := context.WithTimeout(ctx, c.config.HubSpoke.SyncTimeout)
	defer cancel()
	
	resp, err := c.client.RegisterEdge(ctx, req)
	if err != nil {
		return fmt.Errorf("registration failed: %w", err)
	}
	
	if !resp.Success {
		return fmt.Errorf("registration rejected: %s", resp.Message)
	}
	
	c.sessionID = resp.SessionId
	
	// Store initial configuration
	if resp.InitialConfig != nil {
		c.updateConfigCache(resp.InitialConfig)
	}
	
	log.Info().
		Str("session_id", c.sessionID).
		Msg("Successfully registered with control instance")
	
	return nil
}

// startStreaming starts the bidirectional streaming connection
func (c *EdgeClient) startStreaming() error {
	ctx := context.Background()
	
	// Add authentication if configured
	if c.config.HubSpoke.ClientToken != "" {
		md := metadata.New(map[string]string{
			"authorization": "Bearer " + c.config.HubSpoke.ClientToken,
		})
		ctx = metadata.NewOutgoingContext(ctx, md)
	}
	
	stream, err := c.client.SubscribeToChanges(ctx)
	if err != nil {
		return fmt.Errorf("failed to start streaming: %w", err)
	}
	
	c.stream = stream
	
	// Send initial registration message
	regMsg := &pb.EdgeMessage{
		Message: &pb.EdgeMessage_Registration{
			Registration: &pb.EdgeRegistrationRequest{
				EdgeId:        c.edgeID,
				EdgeNamespace: c.config.HubSpoke.EdgeNamespace,
				Version:       "dev",
				BuildHash:     "unknown",
			},
		},
	}
	
	if err := stream.Send(regMsg); err != nil {
		return fmt.Errorf("failed to send registration: %w", err)
	}
	
	// Start message handling goroutine
	go c.handleStreamMessages()
	
	return nil
}

// handleStreamMessages processes incoming messages from the control instance
func (c *EdgeClient) handleStreamMessages() {
	defer func() {
		c.connected = false
		c.stream = nil
		
		// Trigger reconnection
		select {
		case c.reconnectCh <- true:
		default:
		}
	}()
	
	for {
		msg, err := c.stream.Recv()
		if err != nil {
			log.Error().Err(err).Msg("Stream receive error")
			break
		}
		
		switch m := msg.Message.(type) {
		case *pb.ControlMessage_RegistrationResponse:
			log.Debug().
				Bool("success", m.RegistrationResponse.Success).
				Str("message", m.RegistrationResponse.Message).
				Msg("Received registration response")
				
		case *pb.ControlMessage_Configuration:
			log.Info().Msg("Received full configuration update")
			c.updateConfigCache(m.Configuration)
			
		case *pb.ControlMessage_Change:
			log.Info().
				Str("type", m.Change.ChangeType.String()).
				Str("entity_type", m.Change.EntityType.String()).
				Uint32("entity_id", m.Change.EntityId).
				Msg("Received configuration change")
			c.handleConfigurationChange(m.Change)
			
		case *pb.ControlMessage_HeartbeatResponse:
			if m.HeartbeatResponse.ShutdownRequested {
				log.Info().Str("message", m.HeartbeatResponse.Message).Msg("Shutdown requested by control")
				c.cancel()
				return
			}
			
		case *pb.ControlMessage_Error:
			log.Error().
				Str("code", m.Error.Code).
				Str("message", m.Error.Message).
				Bool("fatal", m.Error.Fatal).
				Msg("Received error from control")
				
			if m.Error.Fatal {
				log.Fatal().Msg("Fatal error from control, shutting down")
			}
		}
	}
}

// heartbeatWorker sends periodic heartbeats to the control instance
func (c *EdgeClient) heartbeatWorker() {
	ticker := time.NewTicker(c.config.HubSpoke.HeartbeatInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			if c.connected && c.stream != nil {
				c.sendHeartbeat()
			}
		case <-c.ctx.Done():
			return
		}
	}
}

// sendHeartbeat sends a heartbeat message to the control instance
func (c *EdgeClient) sendHeartbeat() {
	heartbeat := &pb.EdgeMessage{
		Message: &pb.EdgeMessage_Heartbeat{
			Heartbeat: &pb.HeartbeatRequest{
				EdgeId:    c.edgeID,
				SessionId: c.sessionID,
				Health: &pb.HealthStatus{
					Status:    pb.HealthStatus_HEALTHY,
					Message:   "Operational",
					Timestamp: timestamppb.Now(),
				},
				Metrics: &pb.EdgeMetrics{
					RequestsProcessed: 0, // TODO: Get real metrics
					ActiveConnections: 0,
					CpuUsagePercent:   0.0,
					MemoryUsageBytes:  0,
					UptimeSeconds:     0,
				},
				Timestamp: timestamppb.Now(),
			},
		},
	}
	
	if err := c.stream.Send(heartbeat); err != nil {
		log.Error().Err(err).Msg("Failed to send heartbeat")
	}
}

// updateConfigCache updates the local configuration cache
func (c *EdgeClient) updateConfigCache(snapshot *pb.ConfigurationSnapshot) {
	c.cacheMutex.Lock()
	c.configCache = snapshot
	c.cacheMutex.Unlock()
	
	log.Info().
		Int32("llm_count", int32(len(snapshot.Llms))).
		Int32("app_count", int32(len(snapshot.Apps))).
		Int32("token_count", int32(len(snapshot.Tokens))).
		Msg("Updated configuration cache")
	
	// Notify listeners
	if c.onConfigChange != nil {
		c.onConfigChange(snapshot)
	}
}

// handleConfigurationChange processes individual configuration changes
func (c *EdgeClient) handleConfigurationChange(change *pb.ConfigurationChange) {
	// TODO: Implement incremental configuration updates
	// For now, request full configuration
	if c.stream != nil {
		configReq := &pb.EdgeMessage{
			Message: &pb.EdgeMessage_ConfigRequest{
				ConfigRequest: &pb.ConfigurationRequest{
					EdgeNamespace: c.config.HubSpoke.EdgeNamespace,
				},
			},
		}
		c.stream.Send(configReq)
	}
}

// RequestFullSync requests a full configuration synchronization
func (c *EdgeClient) RequestFullSync() error {
	if !c.connected || c.stream == nil {
		return fmt.Errorf("not connected to control instance")
	}
	
	configReq := &pb.EdgeMessage{
		Message: &pb.EdgeMessage_ConfigRequest{
			ConfigRequest: &pb.ConfigurationRequest{
				EdgeNamespace: c.config.HubSpoke.EdgeNamespace,
			},
		},
	}
	
	return c.stream.Send(configReq)
}

// IsConnected returns true if connected to control instance
func (c *EdgeClient) IsConnected() bool {
	return c.connected
}

// getHostname returns the system hostname
func getHostname() string {
	// TODO: Implement proper hostname detection
	return "unknown"
}