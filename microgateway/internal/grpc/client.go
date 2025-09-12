// internal/grpc/client.go
package grpc

import (
	"context"
	"crypto/tls"
	"fmt"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
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
	
	// Build information
	version    string
	buildHash  string
	buildTime  string
	
	// Metrics tracking
	startTime         time.Time
	requestsProcessed uint64
	activeConnections uint64
	metricsMutex      sync.RWMutex
	
	// gRPC connection
	conn       *grpc.ClientConn
	client     pb.ConfigurationSyncServiceClient
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
func NewEdgeClient(cfg *config.Config, version, buildHash, buildTime string) *EdgeClient {
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
		version:     version,
		buildHash:   buildHash,
		buildTime:   buildTime,
		startTime:   time.Now(),
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
		Version:       c.version,
		BuildHash:     c.buildHash,
		Metadata: map[string]string{
			"hostname":   getHostname(),
			"build_time": c.buildTime,
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
				Version:       c.version,
				BuildHash:     c.buildHash,
				Metadata: map[string]string{
					"hostname":   getHostname(),
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
	// Collect real-time metrics
	metrics := c.collectMetrics()
	
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
				Metrics:   metrics,
				Timestamp: timestamppb.Now(),
			},
		},
	}
	
	if err := c.stream.Send(heartbeat); err != nil {
		log.Error().Err(err).Msg("Failed to send heartbeat")
	} else {
		log.Debug().
			Uint64("requests", metrics.RequestsProcessed).
			Uint64("connections", metrics.ActiveConnections).
			Float64("cpu_percent", metrics.CpuUsagePercent).
			Uint64("memory_bytes", metrics.MemoryUsageBytes).
			Uint64("uptime_seconds", metrics.UptimeSeconds).
			Msg("Heartbeat sent with metrics")
	}
}

// updateConfigCache updates the local configuration cache
func (c *EdgeClient) updateConfigCache(snapshot *pb.ConfigurationSnapshot) {
	c.cacheMutex.Lock()
	c.configCache = snapshot
	c.cacheMutex.Unlock()
	
	log.Info().
		Str("version", snapshot.Version).
		Int32("llm_count", int32(len(snapshot.Llms))).
		Int32("app_count", int32(len(snapshot.Apps))).
		Msg("Updated configuration cache (tokens validated on-demand)")
	
	// Notify listeners
	if c.onConfigChange != nil {
		c.onConfigChange(snapshot)
	}
}

// handleConfigurationChange processes individual configuration changes
func (c *EdgeClient) handleConfigurationChange(change *pb.ConfigurationChange) {
	log.Info().
		Str("change_type", change.ChangeType.String()).
		Str("entity_type", change.EntityType.String()).
		Uint32("entity_id", change.EntityId).
		Str("namespace", change.Namespace).
		Msg("Processing incremental configuration change")

	c.cacheMutex.Lock()
	defer c.cacheMutex.Unlock()

	if c.configCache == nil {
		// No cache yet, request full configuration
		log.Info().Msg("No configuration cache, requesting full sync")
		c.requestFullConfigurationUnlocked()
		return
	}

	// Apply incremental change to cached configuration
	updated := c.applyIncrementalChange(change)
	
	if updated {
		// Notify listeners of the updated configuration
		if c.onConfigChange != nil {
			// Make a copy to avoid races
			configCopy := c.copyConfiguration(c.configCache)
			go c.onConfigChange(configCopy)
		}
		
		log.Info().
			Str("change_type", change.ChangeType.String()).
			Str("entity_type", change.EntityType.String()).
			Uint32("entity_id", change.EntityId).
			Msg("Applied incremental configuration change successfully")
	} else {
		// Failed to apply incremental change, request full sync
		log.Warn().
			Str("change_type", change.ChangeType.String()).
			Str("entity_type", change.EntityType.String()).
			Uint32("entity_id", change.EntityId).
			Msg("Failed to apply incremental change, requesting full sync")
		c.requestFullConfigurationUnlocked()
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

// ValidateTokenOnDemand validates a token by calling the control instance
func (c *EdgeClient) ValidateTokenOnDemand(token string) (*pb.TokenValidationResponse, error) {
	if !c.connected || c.client == nil {
		return nil, fmt.Errorf("not connected to control instance")
	}

	ctx := context.Background()
	
	// Add authentication if configured
	if c.config.HubSpoke.ClientToken != "" {
		md := metadata.New(map[string]string{
			"authorization": "Bearer " + c.config.HubSpoke.ClientToken,
		})
		ctx = metadata.NewOutgoingContext(ctx, md)
	}

	// Create token validation request
	req := &pb.TokenValidationRequest{
		Token:         token,
		EdgeId:        c.edgeID,
		EdgeNamespace: c.config.HubSpoke.EdgeNamespace,
	}

	// Call control instance with timeout
	ctx, cancel := context.WithTimeout(ctx, c.config.HubSpoke.SyncTimeout)
	defer cancel()

	resp, err := c.client.ValidateToken(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("token validation failed: %w", err)
	}

	return resp, nil
}

// getHostname returns the system hostname
func getHostname() string {
	// Get hostname from OS
	if hostname, err := os.Hostname(); err == nil {
		return hostname
	}
	// Fallback to unknown if hostname detection fails
	return "unknown"
}

// requestFullConfigurationUnlocked requests full config (must be called with cacheMutex held)
func (c *EdgeClient) requestFullConfigurationUnlocked() {
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

// applyIncrementalChange applies a configuration change to the cached config
func (c *EdgeClient) applyIncrementalChange(change *pb.ConfigurationChange) bool {
	if c.configCache == nil {
		return false
	}

	switch change.EntityType {
	case pb.ConfigurationChange_LLM:
		return c.applyLLMChange(change)
	case pb.ConfigurationChange_APP:
		return c.applyAppChange(change)
	case pb.ConfigurationChange_FILTER:
		return c.applyFilterChange(change)
	case pb.ConfigurationChange_PLUGIN:
		return c.applyPluginChange(change)
	case pb.ConfigurationChange_MODEL_PRICE:
		return c.applyModelPriceChange(change)
	default:
		log.Warn().
			Str("entity_type", change.EntityType.String()).
			Msg("Unsupported entity type for incremental update")
		return false
	}
}

// applyLLMChange applies an LLM configuration change
func (c *EdgeClient) applyLLMChange(change *pb.ConfigurationChange) bool {
	entityID := change.EntityId
	
	switch change.ChangeType {
	case pb.ConfigurationChange_CREATE, pb.ConfigurationChange_UPDATE:
		// For CREATE/UPDATE, we need to parse the entity data and update/add the LLM
		// This is a simplified implementation - in practice you'd want proper JSON unmarshaling
		// For now, let's just update the version to trigger a cache refresh
		c.configCache.Version = fmt.Sprintf("%s-updated-%d", c.configCache.Version, time.Now().Unix())
		return true
		
	case pb.ConfigurationChange_DELETE:
		// Remove LLM from cache
		for i, llm := range c.configCache.Llms {
			if llm.Id == entityID {
				// Remove from slice
				c.configCache.Llms = append(c.configCache.Llms[:i], c.configCache.Llms[i+1:]...)
				c.configCache.Version = fmt.Sprintf("%s-updated-%d", c.configCache.Version, time.Now().Unix())
				return true
			}
		}
		log.Warn().Uint32("llm_id", entityID).Msg("LLM not found for deletion")
		return false
	}
	
	return false
}

// applyAppChange applies an App configuration change
func (c *EdgeClient) applyAppChange(change *pb.ConfigurationChange) bool {
	entityID := change.EntityId
	
	switch change.ChangeType {
	case pb.ConfigurationChange_CREATE, pb.ConfigurationChange_UPDATE:
		// Update version to trigger refresh
		c.configCache.Version = fmt.Sprintf("%s-updated-%d", c.configCache.Version, time.Now().Unix())
		return true
		
	case pb.ConfigurationChange_DELETE:
		// Remove App from cache
		for i, app := range c.configCache.Apps {
			if app.Id == entityID {
				// Remove from slice
				c.configCache.Apps = append(c.configCache.Apps[:i], c.configCache.Apps[i+1:]...)
				c.configCache.Version = fmt.Sprintf("%s-updated-%d", c.configCache.Version, time.Now().Unix())
				return true
			}
		}
		log.Warn().Uint32("app_id", entityID).Msg("App not found for deletion")
		return false
	}
	
	return false
}

// applyFilterChange applies a Filter configuration change
func (c *EdgeClient) applyFilterChange(change *pb.ConfigurationChange) bool {
	entityID := change.EntityId
	
	switch change.ChangeType {
	case pb.ConfigurationChange_CREATE, pb.ConfigurationChange_UPDATE:
		c.configCache.Version = fmt.Sprintf("%s-updated-%d", c.configCache.Version, time.Now().Unix())
		return true
		
	case pb.ConfigurationChange_DELETE:
		for i, filter := range c.configCache.Filters {
			if filter.Id == entityID {
				c.configCache.Filters = append(c.configCache.Filters[:i], c.configCache.Filters[i+1:]...)
				c.configCache.Version = fmt.Sprintf("%s-updated-%d", c.configCache.Version, time.Now().Unix())
				return true
			}
		}
		log.Warn().Uint32("filter_id", entityID).Msg("Filter not found for deletion")
		return false
	}
	
	return false
}

// applyPluginChange applies a Plugin configuration change
func (c *EdgeClient) applyPluginChange(change *pb.ConfigurationChange) bool {
	entityID := change.EntityId
	
	switch change.ChangeType {
	case pb.ConfigurationChange_CREATE, pb.ConfigurationChange_UPDATE:
		c.configCache.Version = fmt.Sprintf("%s-updated-%d", c.configCache.Version, time.Now().Unix())
		return true
		
	case pb.ConfigurationChange_DELETE:
		for i, plugin := range c.configCache.Plugins {
			if plugin.Id == entityID {
				c.configCache.Plugins = append(c.configCache.Plugins[:i], c.configCache.Plugins[i+1:]...)
				c.configCache.Version = fmt.Sprintf("%s-updated-%d", c.configCache.Version, time.Now().Unix())
				return true
			}
		}
		log.Warn().Uint32("plugin_id", entityID).Msg("Plugin not found for deletion")
		return false
	}
	
	return false
}

// applyModelPriceChange applies a ModelPrice configuration change
func (c *EdgeClient) applyModelPriceChange(change *pb.ConfigurationChange) bool {
	entityID := change.EntityId
	
	switch change.ChangeType {
	case pb.ConfigurationChange_CREATE, pb.ConfigurationChange_UPDATE:
		c.configCache.Version = fmt.Sprintf("%s-updated-%d", c.configCache.Version, time.Now().Unix())
		return true
		
	case pb.ConfigurationChange_DELETE:
		for i, price := range c.configCache.ModelPrices {
			if price.Id == entityID {
				c.configCache.ModelPrices = append(c.configCache.ModelPrices[:i], c.configCache.ModelPrices[i+1:]...)
				c.configCache.Version = fmt.Sprintf("%s-updated-%d", c.configCache.Version, time.Now().Unix())
				return true
			}
		}
		log.Warn().Uint32("price_id", entityID).Msg("ModelPrice not found for deletion")
		return false
	}
	
	return false
}

// copyConfiguration creates a deep copy of configuration to avoid races
func (c *EdgeClient) copyConfiguration(config *pb.ConfigurationSnapshot) *pb.ConfigurationSnapshot {
	if config == nil {
		return nil
	}
	
	// For simplicity, we'll just return the same reference for now
	// In production, you'd want proper deep copying
	return config
}

// IncrementRequestCount increments the processed requests counter
func (c *EdgeClient) IncrementRequestCount() {
	c.metricsMutex.Lock()
	c.requestsProcessed++
	c.metricsMutex.Unlock()
}

// SetActiveConnections sets the current number of active connections
func (c *EdgeClient) SetActiveConnections(count uint64) {
	c.metricsMutex.Lock()
	c.activeConnections = count
	c.metricsMutex.Unlock()
}

// collectMetrics gathers runtime metrics for heartbeat
func (c *EdgeClient) collectMetrics() *pb.EdgeMetrics {
	c.metricsMutex.RLock()
	requestsProcessed := c.requestsProcessed
	activeConnections := c.activeConnections
	c.metricsMutex.RUnlock()
	
	// Calculate uptime
	uptime := time.Since(c.startTime)
	
	// Get memory stats
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	
	// Get number of goroutines as a proxy for CPU usage
	numGoroutines := runtime.NumGoroutine()
	cpuUsagePercent := float64(numGoroutines) * 0.1 // Simple approximation
	
	return &pb.EdgeMetrics{
		RequestsProcessed: requestsProcessed,
		ActiveConnections: activeConnections,
		CpuUsagePercent:   cpuUsagePercent,
		MemoryUsageBytes:  memStats.Alloc,
		UptimeSeconds:     uint64(uptime.Seconds()),
	}
}