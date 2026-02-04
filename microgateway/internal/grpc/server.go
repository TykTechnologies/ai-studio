// internal/grpc/server.go
package grpc

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/gorm"
)

// ControlServer implements the ConfigurationSyncService for control instances
type ControlServer struct {
	pb.UnimplementedConfigurationSyncServiceServer
	
	config   *config.Config
	db       *gorm.DB
	
	// Edge instance management
	edgeInstances        map[string]*EdgeInstance
	edgeMutex            sync.RWMutex
	maxConcurrentStreams int // Maximum number of concurrent gRPC streams

	// Reload coordination (will be set after creation to avoid import cycle)
	reloadCoordinator interface{} // *services.ReloadCoordinator

	// gRPC server
	grpcServer *grpc.Server

	// Cleanup ticker for stale connections
	cleanupTicker *time.Ticker
}

// EdgeInstance represents a connected edge instance
type EdgeInstance struct {
	EdgeID        string
	Namespace     string
	SessionID     string
	Version       string
	BuildHash     string
	LastHeartbeat time.Time
	Status        string
	Stream        pb.ConfigurationSyncService_SubscribeToChangesServer
	Metadata      map[string]string
}

// NewControlServer creates a new control server
func NewControlServer(cfg *config.Config, db *gorm.DB) *ControlServer {
	// Set default connection limit if not specified
	maxStreams := cfg.HubSpoke.MaxConcurrentStreams
	if maxStreams <= 0 {
		maxStreams = 1000 // Sensible default
	}

	server := &ControlServer{
		config:               cfg,
		db:                   db,
		edgeInstances:        make(map[string]*EdgeInstance),
		maxConcurrentStreams: maxStreams,
	}

	// Start cleanup routine
	server.startCleanupRoutine()

	return server
}

// SetReloadCoordinator sets the reload coordinator reference (avoids import cycle)
func (s *ControlServer) SetReloadCoordinator(coordinator interface{}) {
	s.reloadCoordinator = coordinator
	log.Info().Msg("Reload coordinator set for control server")
}

// Start starts the gRPC server
func (s *ControlServer) Start() error {
	// Create listener
	addr := fmt.Sprintf("%s:%d", s.config.HubSpoke.GRPCHost, s.config.HubSpoke.GRPCPort)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}
	
	// Setup gRPC server options
	var opts []grpc.ServerOption

	// Add keepalive settings for reliable bidirectional streaming
	opts = append(opts, grpc.KeepaliveParams(keepalive.ServerParameters{
		Time:    30 * time.Second, // Ping client every 30 seconds if no activity
		Timeout: 5 * time.Second,  // Wait 5 seconds for ping response
	}))

	opts = append(opts, grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
		MinTime:             10 * time.Second, // Client must wait at least 10s between pings
		PermitWithoutStream: true,             // Allow pings when no active streams
	}))

	// Add TLS if enabled
	if s.config.HubSpoke.TLSEnabled {
		creds, err := credentials.NewServerTLSFromFile(
			s.config.HubSpoke.TLSCertPath,
			s.config.HubSpoke.TLSKeyPath,
		)
		if err != nil {
			return fmt.Errorf("failed to load TLS credentials: %w", err)
		}
		opts = append(opts, grpc.Creds(creds))
	}

	// Add authentication interceptor
	opts = append(opts, grpc.UnaryInterceptor(s.authInterceptor))
	opts = append(opts, grpc.StreamInterceptor(s.streamAuthInterceptor))
	
	// Create gRPC server
	s.grpcServer = grpc.NewServer(opts...)
	pb.RegisterConfigurationSyncServiceServer(s.grpcServer, s)

	log.Info().Str("address", addr).Msg("Starting gRPC control server")
	
	// Start serving
	if err := s.grpcServer.Serve(listener); err != nil {
		return fmt.Errorf("gRPC server failed: %w", err)
	}
	
	return nil
}

// Stop stops the gRPC server gracefully
func (s *ControlServer) Stop() {
	log.Info().Msg("Stopping gRPC control server")

	// Stop cleanup routine
	if s.cleanupTicker != nil {
		s.cleanupTicker.Stop()
	}

	if s.grpcServer != nil {
		s.grpcServer.GracefulStop()
	}
	
	// Close all edge connections
	s.edgeMutex.Lock()
	for _, edge := range s.edgeInstances {
		if edge.Stream != nil {
			// Send shutdown message
			edge.Stream.Send(&pb.ControlMessage{
				Message: &pb.ControlMessage_HeartbeatResponse{
					HeartbeatResponse: &pb.HeartbeatResponse{
						Acknowledged:      true,
						Message:           "Control server shutting down",
						ShutdownRequested: true,
					},
				},
			})
		}
	}
	s.edgeMutex.Unlock()
}

// Authentication interceptor for unary RPCs
func (s *ControlServer) authInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	if err := s.authenticate(ctx); err != nil {
		return nil, err
	}
	return handler(ctx, req)
}

// Authentication interceptor for streaming RPCs
func (s *ControlServer) streamAuthInterceptor(srv interface{}, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	if err := s.authenticate(stream.Context()); err != nil {
		return err
	}
	return handler(srv, stream)
}

// authenticate checks the authentication token (supports dual-token rotation)
func (s *ControlServer) authenticate(ctx context.Context) error {
	// SECURITY: Fail-closed design - reject connections if no auth tokens configured
	if s.config.HubSpoke.AuthToken == "" && s.config.HubSpoke.NextAuthToken == "" {
		log.Error().Msg("🔒 SECURITY: No authentication tokens configured - rejecting connection")
		return status.Error(codes.Unauthenticated, "authentication required but no tokens configured")
	}

	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return status.Error(codes.Unauthenticated, "missing metadata")
	}

	tokens := md.Get("authorization")
	if len(tokens) == 0 {
		return status.Error(codes.Unauthenticated, "missing authorization token")
	}

	token := tokens[0]

	// Check current token
	if s.config.HubSpoke.AuthToken != "" && token == "Bearer "+s.config.HubSpoke.AuthToken {
		return nil
	}

	// Check next token (for rotation)
	if s.config.HubSpoke.NextAuthToken != "" && token == "Bearer "+s.config.HubSpoke.NextAuthToken {
		log.Debug().Msg("Edge authenticated with next token (rotation in progress)")
		return nil
	}

	log.Warn().Msg("Authentication failed: invalid authorization token")
	return status.Error(codes.Unauthenticated, "invalid authorization token")
}

// RegisterEdge handles edge instance registration
func (s *ControlServer) RegisterEdge(ctx context.Context, req *pb.EdgeRegistrationRequest) (*pb.EdgeRegistrationResponse, error) {
	log.Info().
		Str("edge_id", req.EdgeId).
		Str("namespace", req.EdgeNamespace).
		Str("version", req.Version).
		Msg("Edge instance registration request")

	// Comprehensive request validation
	if err := s.validateEdgeRegistrationRequest(req); err != nil {
		return nil, err
	}
	
	// Generate session ID
	sessionID := uuid.New().String()
	
	// Create edge instance record
	edge := &EdgeInstance{
		EdgeID:        req.EdgeId,
		Namespace:     req.EdgeNamespace,
		SessionID:     sessionID,
		Version:       req.Version,
		BuildHash:     req.BuildHash,
		LastHeartbeat: time.Now(),
		Status:        "registered",
		Metadata:      req.Metadata,
	}
	
	// Store in database
	dbEdge := &database.EdgeInstance{
		EdgeID:        req.EdgeId,
		Namespace:     req.EdgeNamespace,
		Version:       req.Version,
		BuildHash:     req.BuildHash,
		LastHeartbeat: &edge.LastHeartbeat,
		Status:        edge.Status,
		SessionID:     sessionID,
	}
	
	if len(req.Metadata) > 0 {
		// Convert metadata to JSON - simplified for now
		// In a real implementation, you'd use proper JSON marshaling
		dbEdge.Metadata = []byte("{}")
	}
	
	// Use upsert to handle edge instances that reconnect with the same ID
	// This updates existing records or creates new ones
	result := s.db.Where("edge_id = ?", req.EdgeId).Assign(dbEdge).FirstOrCreate(dbEdge)
	if result.Error != nil {
		log.Error().Err(result.Error).Str("edge_id", req.EdgeId).Msg("Failed to store edge instance")
		return nil, status.Error(codes.Internal, "failed to register edge instance")
	}
	
	// Register in memory
	s.edgeMutex.Lock()
	s.edgeInstances[req.EdgeId] = edge
	s.edgeMutex.Unlock()
	
	// Get initial configuration
	initialConfig, err := s.getConfigurationSnapshot(req.EdgeNamespace)
	if err != nil {
		log.Error().Err(err).Str("edge_id", req.EdgeId).Msg("Failed to get initial configuration")
		return nil, status.Error(codes.Internal, "failed to get initial configuration")
	}
	
	log.Info().
		Str("edge_id", req.EdgeId).
		Str("session_id", sessionID).
		Msg("Edge instance registered successfully")
	
	return &pb.EdgeRegistrationResponse{
		Success:       true,
		Message:       "Registration successful",
		SessionId:     sessionID,
		InitialConfig: initialConfig,
	}, nil
}

// GetFullConfiguration retrieves a complete configuration snapshot
func (s *ControlServer) GetFullConfiguration(ctx context.Context, req *pb.ConfigurationRequest) (*pb.ConfigurationSnapshot, error) {
	log.Debug().
		Str("namespace", req.EdgeNamespace).
		Msg("Full configuration request")
	
	snapshot, err := s.getConfigurationSnapshot(req.EdgeNamespace)
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to get configuration: %v", err))
	}
	
	return snapshot, nil
}

// SubscribeToChanges handles bidirectional streaming for real-time updates
func (s *ControlServer) SubscribeToChanges(stream pb.ConfigurationSyncService_SubscribeToChangesServer) error {
	// Check concurrent stream limits to prevent DoS attacks
	s.edgeMutex.RLock()
	currentConnections := len(s.edgeInstances)
	s.edgeMutex.RUnlock()

	if currentConnections >= s.maxConcurrentStreams {
		log.Warn().
			Int("current_connections", currentConnections).
			Int("max_concurrent_streams", s.maxConcurrentStreams).
			Msg("🚨 SECURITY: Maximum concurrent streams exceeded - rejecting new connection")
		return status.Error(codes.ResourceExhausted, "maximum concurrent streams exceeded")
	}

	var edgeID string
	var edge *EdgeInstance
	var streamCtx = stream.Context()

	// Create channels for coordinated shutdown
	done := make(chan struct{})
	recvErr := make(chan error, 1)

	// Handle incoming messages from edge with proper error handling
	go func() {
		defer close(done)

		for {
			// Check if context is cancelled
			select {
			case <-streamCtx.Done():
				log.Debug().Str("edge_id", edgeID).Msg("Stream context cancelled, stopping message handler")
				return
			default:
			}

			msg, err := stream.Recv()
			if err != nil {
				log.Debug().Err(err).Str("edge_id", edgeID).Msg("Edge stream receive error")
				recvErr <- err
				return
			}

			// Process message with error handling
			if err := s.handleEdgeMessage(msg, stream, &edgeID, &edge); err != nil {
				log.Error().Err(err).Str("edge_id", edgeID).Msg("Failed to handle edge message")
				// Continue processing other messages rather than failing the entire stream
			}
		}
	}()

	// Wait for stream to close or error
	select {
	case <-done:
		log.Debug().Str("edge_id", edgeID).Msg("Message handler completed")
	case err := <-recvErr:
		log.Debug().Err(err).Str("edge_id", edgeID).Msg("Stream receive error")
	case <-streamCtx.Done():
		log.Debug().Str("edge_id", edgeID).Msg("Stream context cancelled")
	}

	// Cleanup when stream closes
	if edge != nil {
		s.edgeMutex.Lock()
		if existingEdge, exists := s.edgeInstances[edgeID]; exists && existingEdge == edge {
			existingEdge.Status = "disconnected"
			existingEdge.Stream = nil
			log.Info().Str("edge_id", edgeID).Msg("Edge instance disconnected and cleaned up")
		}
		s.edgeMutex.Unlock()
	}

	log.Debug().Str("edge_id", edgeID).Msg("Edge stream closed")
	return nil
}

// handleEdgeMessage processes individual messages from edge instances
func (s *ControlServer) handleEdgeMessage(msg *pb.EdgeMessage, stream pb.ConfigurationSyncService_SubscribeToChangesServer, edgeID *string, edge **EdgeInstance) error {
	switch m := msg.Message.(type) {
	case *pb.EdgeMessage_Registration:
		return s.handleStreamRegistration(m, stream, edgeID, edge)

	case *pb.EdgeMessage_Heartbeat:
		return s.handleStreamHeartbeat(m, stream, *edge)

	case *pb.EdgeMessage_ConfigRequest:
		return s.handleStreamConfigRequest(m, stream, *edge, *edgeID)

	case *pb.EdgeMessage_ReloadResponse:
		return s.handleStreamReloadResponse(m)

	default:
		log.Warn().Str("edge_id", *edgeID).Msg("Unknown message type received from edge")
	}

	return nil
}

// handleStreamRegistration handles registration messages in stream
func (s *ControlServer) handleStreamRegistration(m *pb.EdgeMessage_Registration, stream pb.ConfigurationSyncService_SubscribeToChangesServer, edgeID *string, edge **EdgeInstance) error {
	if *edge == nil {
		*edgeID = m.Registration.EdgeId
		s.edgeMutex.RLock()
		*edge = s.edgeInstances[*edgeID]
		s.edgeMutex.RUnlock()

		if *edge != nil {
			// Safely update edge instance
			s.edgeMutex.Lock()
			(*edge).Stream = stream
			(*edge).Status = "connected"
			(*edge).LastHeartbeat = time.Now()
			s.edgeMutex.Unlock()

			// Send registration response with error handling
			response := &pb.ControlMessage{
				Message: &pb.ControlMessage_RegistrationResponse{
					RegistrationResponse: &pb.EdgeRegistrationResponse{
						Success:   true,
						Message:   "Stream connected",
						SessionId: (*edge).SessionID,
					},
				},
			}

			if err := stream.Send(response); err != nil {
				return fmt.Errorf("failed to send registration response: %w", err)
			}

			log.Info().Str("edge_id", *edgeID).Str("session_id", (*edge).SessionID).Msg("Edge stream registration successful")
		}
	}

	return nil
}

// handleStreamHeartbeat handles heartbeat messages in stream
func (s *ControlServer) handleStreamHeartbeat(m *pb.EdgeMessage_Heartbeat, stream pb.ConfigurationSyncService_SubscribeToChangesServer, edge *EdgeInstance) error {
	if edge != nil {
		// Thread-safe heartbeat update
		s.edgeMutex.Lock()
		edge.LastHeartbeat = time.Now()
		s.edgeMutex.Unlock()

		// Send heartbeat response
		response := &pb.ControlMessage{
			Message: &pb.ControlMessage_HeartbeatResponse{
				HeartbeatResponse: &pb.HeartbeatResponse{
					Acknowledged: true,
					Message:      "Heartbeat received",
				},
			},
		}

		if err := stream.Send(response); err != nil {
			return fmt.Errorf("failed to send heartbeat response: %w", err)
		}

		log.Debug().Str("edge_id", edge.EdgeID).Msg("Heartbeat processed")
	}

	return nil
}

// handleStreamConfigRequest handles configuration request messages in stream
func (s *ControlServer) handleStreamConfigRequest(m *pb.EdgeMessage_ConfigRequest, stream pb.ConfigurationSyncService_SubscribeToChangesServer, edge *EdgeInstance, edgeID string) error {
	if edge != nil {
		snapshot, err := s.getConfigurationSnapshot(edge.Namespace)
		if err != nil {
			log.Error().Err(err).Str("edge_id", edgeID).Msg("Failed to get configuration snapshot")

			// Send error response
			response := &pb.ControlMessage{
				Message: &pb.ControlMessage_Error{
					Error: &pb.ErrorMessage{
						Code:    "CONFIG_ERROR",
						Message: fmt.Sprintf("Failed to get configuration: %v", err),
						Fatal:   false,
					},
				},
			}

			return stream.Send(response)
		} else {
			response := &pb.ControlMessage{
				Message: &pb.ControlMessage_Configuration{
					Configuration: snapshot,
				},
			}

			if err := stream.Send(response); err != nil {
				return fmt.Errorf("failed to send configuration response: %w", err)
			}

			log.Debug().Str("edge_id", edgeID).Str("config_version", snapshot.Version).Msg("Configuration sent to edge")
		}
	}

	return nil
}

// handleStreamReloadResponse handles reload response messages in stream
func (s *ControlServer) handleStreamReloadResponse(m *pb.EdgeMessage_ReloadResponse) error {
	if m.ReloadResponse != nil {
		log.Info().
			Str("operation_id", m.ReloadResponse.OperationId).
			Str("edge_id", m.ReloadResponse.EdgeId).
			Str("phase", m.ReloadResponse.Phase.String()).
			Bool("success", m.ReloadResponse.Success).
			Msg("Received reload status from edge")

		// Forward to reload coordinator if available
		if s.reloadCoordinator != nil {
			if coordinator, ok := s.reloadCoordinator.(interface{ ProcessReloadResponse(*pb.ConfigurationReloadResponse) }); ok {
				coordinator.ProcessReloadResponse(m.ReloadResponse)
			}
		}
	}

	return nil
}

// SendHeartbeat handles heartbeat requests
func (s *ControlServer) SendHeartbeat(ctx context.Context, req *pb.HeartbeatRequest) (*pb.HeartbeatResponse, error) {
	s.edgeMutex.RLock()
	edge, exists := s.edgeInstances[req.EdgeId]
	s.edgeMutex.RUnlock()
	
	if !exists {
		return nil, status.Error(codes.NotFound, "edge instance not found")
	}
	
	// Update heartbeat
	edge.LastHeartbeat = time.Now()
	
	return &pb.HeartbeatResponse{
		Acknowledged: true,
		Message:      "Heartbeat acknowledged",
	}, nil
}

// UnregisterEdge handles edge instance unregistration
func (s *ControlServer) UnregisterEdge(ctx context.Context, req *pb.EdgeUnregistrationRequest) (*emptypb.Empty, error) {
	log.Info().Str("edge_id", req.EdgeId).Str("reason", req.Reason).Msg("Edge unregistration request")
	
	s.edgeMutex.Lock()
	delete(s.edgeInstances, req.EdgeId)
	s.edgeMutex.Unlock()
	
	// Update database
	s.db.Model(&database.EdgeInstance{}).
		Where("edge_id = ?", req.EdgeId).
		Update("status", "unregistered")
	
	return &emptypb.Empty{}, nil
}

// GetConnectedEdges returns a map of connected edge instances (for reload coordinator)
func (s *ControlServer) GetConnectedEdges() map[string]interface{} {
	s.edgeMutex.RLock()
	defer s.edgeMutex.RUnlock()

	result := make(map[string]interface{})
	for edgeID, edge := range s.edgeInstances {
		// Only include edges that are truly connected (have active stream)
		if s.isEdgeStreamActive(edge) {
			// Create a simple struct with the data we need
			edgeInfo := map[string]interface{}{
				"edge_id":   edge.EdgeID,
				"namespace": edge.Namespace,
				"status":    edge.Status,
				"version":   edge.Version,
			}
			result[edgeID] = edgeInfo
		}
	}

	// Debug: Log all edge instances and their status
	log.Info().
		Int("total_edge_instances", len(s.edgeInstances)).
		Int("connected_edge_count", len(result)).
		Msg("Edge instance status for reload coordinator")

	for _, edge := range s.edgeInstances {
		log.Info().
			Str("edge_id", edge.EdgeID).
			Str("namespace", edge.Namespace).
			Str("status", edge.Status).
			Str("version", edge.Version).
			Bool("has_stream", edge.Stream != nil).
			Msg("Edge instance details")
	}

	return result
}

// SendReloadRequest sends a reload request to a specific edge instance
func (s *ControlServer) SendReloadRequest(edgeID string, reloadReq *pb.ConfigurationReloadRequest) error {
	s.edgeMutex.RLock()
	edge, exists := s.edgeInstances[edgeID]
	s.edgeMutex.RUnlock()

	if !exists {
		return fmt.Errorf("edge instance not found: %s", edgeID)
	}

	// Test stream connectivity before sending
	if !s.isEdgeStreamActive(edge) {
		log.Warn().Str("edge_id", edgeID).Msg("Edge stream is not active, marking as disconnected")
		edge.Status = "disconnected"
		edge.Stream = nil
		return fmt.Errorf("edge instance stream is not active: %s", edgeID)
	}

	// Send reload request via gRPC stream
	message := &pb.ControlMessage{
		Message: &pb.ControlMessage_ReloadRequest{
			ReloadRequest: reloadReq,
		},
	}

	if err := edge.Stream.Send(message); err != nil {
		log.Error().Err(err).Str("edge_id", edgeID).Msg("Failed to send reload request to edge")
		edge.Status = "disconnected"
		edge.Stream = nil
		return fmt.Errorf("failed to send reload request: %w", err)
	}

	log.Info().
		Str("edge_id", edgeID).
		Str("operation_id", reloadReq.OperationId).
		Msg("Reload request sent to edge via stream")

	return nil
}

// ValidateToken validates an API token on-demand (NEW)
func (s *ControlServer) ValidateToken(ctx context.Context, req *pb.TokenValidationRequest) (*pb.TokenValidationResponse, error) {
	// Validate request
	if err := s.validateTokenValidationRequest(req); err != nil {
		return nil, err
	}

	tokenPrefix := req.Token
	if len(req.Token) > 8 {
		tokenPrefix = req.Token[:8]
	}

	log.Info().
		Str("token_prefix", tokenPrefix).
		Str("edge_id", req.EdgeId).
		Str("edge_namespace", req.EdgeNamespace).
		Msg("Control server: on-demand token validation request")

	// Query token with namespace filtering
	var apiToken database.APIToken
	tokenQuery := s.db.Where("token = ? AND is_active = ?", req.Token, true).Preload("App")
	
	// Apply namespace filtering - token must be global or match edge namespace
	if req.EdgeNamespace == "" {
		// Global edge - only sees global tokens
		tokenQuery = tokenQuery.Where("namespace = ''")
	} else {
		// Specific namespace edge - sees global + matching tokens
		tokenQuery = tokenQuery.Where("(namespace = '' OR namespace = ?)", req.EdgeNamespace)
	}
	
	err := tokenQuery.First(&apiToken).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			log.Info().
				Str("token_prefix", tokenPrefix).
				Str("edge_namespace", req.EdgeNamespace).
				Msg("Control server: token not found or not accessible from edge namespace")
			
			return &pb.TokenValidationResponse{
				Valid:        false,
				ErrorMessage: "Token not found or not accessible from this namespace",
			}, nil
		}
		
		log.Error().Err(err).Str("token_prefix", tokenPrefix).Msg("Control server: token validation database error")
		return nil, status.Error(codes.Internal, "token validation failed")
	}

	// Check expiration
	if apiToken.ExpiresAt != nil && apiToken.ExpiresAt.Before(time.Now()) {
		log.Info().Str("token_prefix", tokenPrefix).Msg("Control server: token is expired")
		return &pb.TokenValidationResponse{
			Valid:        false,
			ErrorMessage: "Token is expired",
		}, nil
	}

	// Check if app is active
	if apiToken.App != nil && !apiToken.App.IsActive {
		log.Info().Str("token_prefix", tokenPrefix).Uint("app_id", apiToken.AppID).Msg("Control server: token's app is inactive")
		return &pb.TokenValidationResponse{
			Valid:        false,
			ErrorMessage: "Associated app is inactive",
		}, nil
	}

	// Parse scopes
	var scopes []string
	if len(apiToken.Scopes) > 0 {
		if err := json.Unmarshal(apiToken.Scopes, &scopes); err != nil {
			log.Warn().Err(err).Str("token_prefix", tokenPrefix).Msg("Failed to parse token scopes")
			scopes = []string{}
		}
	}

	var expiresAt *timestamppb.Timestamp
	if apiToken.ExpiresAt != nil {
		expiresAt = timestamppb.New(*apiToken.ExpiresAt)
	}

	log.Info().
		Str("token_prefix", tokenPrefix).
		Uint("app_id", apiToken.AppID).
		Str("app_name", apiToken.App.Name).
		Msg("Control server: token validation successful")

	// Build AppConfig for pull-on-miss: allows edge to cache app locally
	appConfig := s.buildAppConfig(apiToken.App)

	return &pb.TokenValidationResponse{
		Valid:     true,
		AppId:     uint32(apiToken.AppID),
		AppName:   apiToken.App.Name,
		Scopes:    scopes,
		ExpiresAt: expiresAt,
		UserId:    uint32(apiToken.App.UserID),
		AppConfig: appConfig,
	}, nil
}

// buildAppConfig converts a database App to protobuf AppConfig for pull-on-miss
// This is used when returning token validation responses to include the full app config
func (s *ControlServer) buildAppConfig(app *database.App) *pb.AppConfig {
	if app == nil {
		return nil
	}

	// Query LLM relationships for this app
	var appLLMs []database.AppLLM
	if err := s.db.Where("app_id = ? AND is_active = ?", app.ID, true).Find(&appLLMs).Error; err != nil {
		log.Warn().Err(err).Uint("app_id", app.ID).Msg("Failed to query app_llms for AppConfig")
	}
	llmIDs := make([]uint32, len(appLLMs))
	for i, appLLM := range appLLMs {
		llmIDs[i] = uint32(appLLM.LLMID)
	}

	// Query credentials for this app
	var credentials []database.Credential
	if err := s.db.Where("app_id = ?", app.ID).Find(&credentials).Error; err != nil {
		log.Warn().Err(err).Uint("app_id", app.ID).Msg("Failed to query credentials for AppConfig")
	}
	credentialIDs := make([]uint32, len(credentials))
	for i, cred := range credentials {
		credentialIDs[i] = uint32(cred.ID)
	}

	// Query tokens for this app
	var tokens []database.APIToken
	if err := s.db.Where("app_id = ?", app.ID).Find(&tokens).Error; err != nil {
		log.Warn().Err(err).Uint("app_id", app.ID).Msg("Failed to query tokens for AppConfig")
	}
	tokenIDs := make([]uint32, len(tokens))
	for i, token := range tokens {
		tokenIDs[i] = uint32(token.ID)
	}

	// Convert budget start date to string if available
	var budgetStartDate string
	if app.BudgetStartDate != nil {
		budgetStartDate = app.BudgetStartDate.Format(time.RFC3339)
	}

	appConfig := &pb.AppConfig{
		Id:              uint32(app.ID),
		Name:            app.Name,
		Description:     app.Description,
		OwnerEmail:      app.OwnerEmail,
		IsActive:        app.IsActive,
		MonthlyBudget:   app.MonthlyBudget,
		BudgetResetDay:  int32(app.BudgetResetDay),
		RateLimitRpm:    int32(app.RateLimitRPM),
		Namespace:       app.Namespace,
		CreatedAt:       timestamppb.New(app.CreatedAt),
		UpdatedAt:       timestamppb.New(app.UpdatedAt),
		UserId:          uint32(app.UserID),
		BudgetStartDate: budgetStartDate,
		AllowedIps:      string(app.AllowedIPs),
		Metadata:        string(app.Metadata),
		LlmIds:          llmIDs,
		CredentialIds:   credentialIDs,
		TokenIds:        tokenIDs,
	}

	log.Debug().
		Uint("app_id", app.ID).
		Str("app_name", app.Name).
		Int("llm_count", len(llmIDs)).
		Int("credential_count", len(credentialIDs)).
		Int("token_count", len(tokenIDs)).
		Msg("Built AppConfig for token validation response (pull-on-miss)")

	return appConfig
}

// SendAnalyticsPulse handles analytics pulse data from edge instances
func (s *ControlServer) SendAnalyticsPulse(ctx context.Context, req *pb.AnalyticsPulse) (*pb.AnalyticsPulseResponse, error) {
	// Validate request
	if err := s.validateAnalyticsPulseRequest(req); err != nil {
		return nil, err
	}

	log.Info().
		Str("edge_id", req.EdgeId).
		Str("edge_namespace", req.EdgeNamespace).
		Uint64("sequence_number", req.SequenceNumber).
		Uint32("total_records", req.TotalRecords).
		Int("analytics_events", len(req.AnalyticsEvents)).
		Int("budget_events", len(req.BudgetEvents)).
		Int("proxy_summaries", len(req.ProxySummaries)).
		Msg("Microgateway control server: received analytics pulse from edge")

	processedRecords := uint64(0)

	// Process analytics events by storing them in local microgateway database
	for _, event := range req.AnalyticsEvents {
		// Create analytics event record (matching LLMChatRecord schema)
		analyticsEvent := &database.AnalyticsEvent{
			RequestID:              event.RequestId,
			AppID:                  uint(event.AppId),

			// Fields matching LLMChatRecord for parity
			UserID:                 uint(event.UserId),
			Name:                   event.ModelName,
			Vendor:                 event.Vendor,
			InteractionType:        event.InteractionType,
			Choices:                int(event.Choices),
			ToolCalls:              int(event.ToolCalls),
			ChatID:                 event.ChatId,
			Currency:               event.Currency,

			// Request/Response details
			Endpoint:               event.Endpoint,
			Method:                 event.Method,
			StatusCode:             int(event.StatusCode),

			// Token tracking (using new field names)
			PromptTokens:           int(event.RequestTokens),
			ResponseTokens:         int(event.ResponseTokens),
			TotalTokens:            int(event.TotalTokens),
			CacheWritePromptTokens: int(event.CacheWritePromptTokens),
			CacheReadPromptTokens:  int(event.CacheReadPromptTokens),

			// Cost and timing
			Cost:                   event.Cost,
			TotalTimeMS:            int(event.LatencyMs),

			ErrorMessage:           event.ErrorMessage,
			TimeStamp:              event.Timestamp.AsTime(),
			CreatedAt:              event.Timestamp.AsTime(),
		}

		// Set LLM ID if provided
		if event.LlmId > 0 {
			llmID := uint(event.LlmId)
			analyticsEvent.LLMID = &llmID
		}

		// Store request/response bodies if provided
		if len(event.RequestBody) > 0 {
			analyticsEvent.RequestBody = event.RequestBody
		}
		if len(event.ResponseBody) > 0 {
			analyticsEvent.ResponseBody = event.ResponseBody
		}

		// Create the analytics event in local database
		if err := s.db.Create(analyticsEvent).Error; err != nil {
			log.Error().Err(err).Str("request_id", event.RequestId).Msg("Failed to store analytics event from pulse")
		} else {
			processedRecords++
			log.Debug().
				Str("edge_id", req.EdgeId).
				Str("request_id", event.RequestId).
				Str("model", event.ModelName).
				Int("total_tokens", int(event.TotalTokens)).
				Float64("cost", event.Cost).
				Msg("Analytics event stored from pulse")
		}
	}

	// Process budget events (store as separate records if needed)
	for _, budget := range req.BudgetEvents {
		log.Debug().
			Str("edge_id", req.EdgeId).
			Uint32("app_id", budget.AppId).
			Uint32("llm_id", budget.LlmId).
			Int64("tokens_used", budget.TokensUsed).
			Float64("cost", budget.Cost).
			Msg("Budget usage data from edge - logged")
		processedRecords++
	}

	// Process proxy summaries (could be stored in separate summary table)
	for _, proxy := range req.ProxySummaries {
		log.Debug().
			Str("edge_id", req.EdgeId).
			Uint32("app_id", proxy.AppId).
			Str("vendor", proxy.Vendor).
			Uint32("request_count", proxy.RequestCount).
			Float64("total_cost", proxy.TotalCost).
			Msg("Proxy summary from edge - logged")
		processedRecords++
	}

	log.Info().
		Str("edge_id", req.EdgeId).
		Uint64("sequence_number", req.SequenceNumber).
		Uint64("processed_records", processedRecords).
		Msg("Analytics pulse processed by microgateway control server")

	return &pb.AnalyticsPulseResponse{
		Success:          true,
		Message:          "Analytics pulse processed successfully",
		ProcessedRecords: processedRecords,
		SequenceNumber:   req.SequenceNumber,
		ProcessedAt:      timestamppb.New(time.Now()),
	}, nil
}

// getConfigurationSnapshot creates a configuration snapshot for a specific namespace
func (s *ControlServer) getConfigurationSnapshot(namespace string) (*pb.ConfigurationSnapshot, error) {
	// Generate version based on timestamp for now (in production, use proper versioning)
	version := fmt.Sprintf("v1.0.0-%d", time.Now().Unix())
	
	snapshot := &pb.ConfigurationSnapshot{
		Version:       version,
		EdgeNamespace: namespace,
		SnapshotTime:  timestamppb.Now(),
	}
	
	// Query LLMs with namespace filtering
	var llms []database.LLM
	query := s.db.Model(&database.LLM{}).Where("is_active = ?", true)
	if namespace == "" {
		// Global namespace - only global objects
		query = query.Where("namespace = ''")
	} else {
		// Specific namespace - global + matching objects
		query = query.Where("(namespace = '' OR namespace = ?)", namespace)
	}
	
	if err := query.Find(&llms).Error; err != nil {
		return nil, fmt.Errorf("failed to query LLMs: %w", err)
	}

	// Collect all LLM IDs for bulk queries to avoid N+1 problem
	var llmIDs []uint
	for _, llm := range llms {
		llmIDs = append(llmIDs, llm.ID)
	}

	// Bulk query all relationships to avoid N+1 queries
	var allLLMAppRelations []database.AppLLM
	var allLLMPlugins []database.LLMPlugin
	var allLLMFilters []database.LLMFilter

	if len(llmIDs) > 0 {
		// Bulk query app_llms relationships
		if err := s.db.Where("llm_id IN ? AND is_active = ?", llmIDs, true).Find(&allLLMAppRelations).Error; err != nil {
			log.Warn().Err(err).Msg("Failed to bulk query app_llms join table")
		}

		// Bulk query llm_plugins relationships
		if err := s.db.Where("llm_id IN ? AND is_active = ?", llmIDs, true).Find(&allLLMPlugins).Error; err != nil {
			log.Warn().Err(err).Msg("Failed to bulk query llm_plugins join table")
		}

		// Bulk query llm_filters relationships
		if err := s.db.Where("llm_id IN ? AND is_active = ?", llmIDs, true).Find(&allLLMFilters).Error; err != nil {
			log.Warn().Err(err).Msg("Failed to bulk query llm_filters join table")
		}
	}

	// Create lookup maps for efficient relationship matching
	appLLMsByLLMID := make(map[uint][]database.AppLLM)
	for _, appLLM := range allLLMAppRelations {
		appLLMsByLLMID[appLLM.LLMID] = append(appLLMsByLLMID[appLLM.LLMID], appLLM)
	}

	pluginsByLLMID := make(map[uint][]database.LLMPlugin)
	for _, llmPlugin := range allLLMPlugins {
		pluginsByLLMID[llmPlugin.LLMID] = append(pluginsByLLMID[llmPlugin.LLMID], llmPlugin)
	}

	filtersByLLMID := make(map[uint][]database.LLMFilter)
	for _, llmFilter := range allLLMFilters {
		filtersByLLMID[llmFilter.LLMID] = append(filtersByLLMID[llmFilter.LLMID], llmFilter)
	}

	// Convert LLMs to protobuf with embedded relationships
	snapshot.Llms = make([]*pb.LLMConfig, len(llms))
	for i, llm := range llms {
		// Get app relationships from lookup map
		appLLMs := appLLMsByLLMID[llm.ID]
		appIDs := make([]uint32, len(appLLMs))
		for j, appLLM := range appLLMs {
			appIDs[j] = uint32(appLLM.AppID)
		}

		// Get plugin relationships from lookup map
		llmPlugins := pluginsByLLMID[llm.ID]
		pluginIDs := make([]uint32, len(llmPlugins))
		for j, llmPlugin := range llmPlugins {
			pluginIDs[j] = uint32(llmPlugin.PluginID)
		}

		// Get filter relationships from lookup map
		llmFilters := filtersByLLMID[llm.ID]
		filterIDs := make([]uint32, len(llmFilters))
		for j, llmFilter := range llmFilters {
			filterIDs[j] = uint32(llmFilter.FilterID)
		}

		snapshot.Llms[i] = &pb.LLMConfig{
			Id:              uint32(llm.ID),
			Name:            llm.Name,
			Slug:            llm.Slug,
			Vendor:          llm.Vendor,
			Endpoint:        llm.Endpoint,
			ApiKeyEncrypted: llm.APIKeyEncrypted,
			DefaultModel:    llm.DefaultModel,
			MaxTokens:       int32(llm.MaxTokens),
			TimeoutSeconds:  int32(llm.TimeoutSeconds),
			RetryCount:      int32(llm.RetryCount),
			IsActive:        llm.IsActive,
			MonthlyBudget:   llm.MonthlyBudget,
			RateLimitRpm:    int32(llm.RateLimitRPM),
			Namespace:       llm.Namespace,
			CreatedAt:       timestamppb.New(llm.CreatedAt),
			UpdatedAt:       timestamppb.New(llm.UpdatedAt),
			
			// JSON fields as strings
			Metadata:      string(llm.Metadata),
			AllowedModels: string(llm.AllowedModels),
			AuthMechanism: llm.AuthMechanism,
			AuthConfig:    string(llm.AuthConfig),
			
			// Embedded relationship data
			AppIds:    appIDs,    // Which apps can access this LLM
			FilterIds: filterIDs, // Which filters apply to this LLM
			PluginIds: pluginIDs, // Which plugins apply to this LLM
		}

		log.Debug().
			Uint("llm_id", llm.ID).
			Str("llm_slug", llm.Slug).
			Int("app_count", len(appIDs)).
			Int("filter_count", len(filterIDs)).
			Int("plugin_count", len(pluginIDs)).
			Msg("LLM relationships embedded in sync")
	}
	
	// Query Apps with namespace filtering
	var apps []database.App
	appQuery := s.db.Model(&database.App{}).Where("is_active = ?", true)
	if namespace == "" {
		appQuery = appQuery.Where("namespace = ''")
	} else {
		appQuery = appQuery.Where("(namespace = '' OR namespace = ?)", namespace)
	}
	
	if err := appQuery.Find(&apps).Error; err != nil {
		return nil, fmt.Errorf("failed to query Apps: %w", err)
	}

	// Collect all App IDs for bulk queries to avoid N+1 problem
	var appIDs []uint
	for _, app := range apps {
		appIDs = append(appIDs, app.ID)
	}

	// Bulk query all app relationships to avoid N+1 queries
	var allAppLLMs []database.AppLLM
	var allCredentials []database.Credential
	var allTokens []database.APIToken

	if len(appIDs) > 0 {
		// Bulk query app_llms relationships
		if err := s.db.Where("app_id IN ? AND is_active = ?", appIDs, true).Find(&allAppLLMs).Error; err != nil {
			log.Warn().Err(err).Msg("Failed to bulk query app_llms join table")
		}

		// Bulk query credentials
		if err := s.db.Where("app_id IN ?", appIDs).Find(&allCredentials).Error; err != nil {
			log.Warn().Err(err).Msg("Failed to bulk query app credentials")
		}

		// Bulk query tokens
		if err := s.db.Where("app_id IN ?", appIDs).Find(&allTokens).Error; err != nil {
			log.Warn().Err(err).Msg("Failed to bulk query app tokens")
		}
	}

	// Create lookup maps for efficient relationship matching
	llmsByAppID := make(map[uint][]database.AppLLM)
	for _, appLLM := range allAppLLMs {
		llmsByAppID[appLLM.AppID] = append(llmsByAppID[appLLM.AppID], appLLM)
	}

	credentialsByAppID := make(map[uint][]database.Credential)
	for _, credential := range allCredentials {
		credentialsByAppID[credential.AppID] = append(credentialsByAppID[credential.AppID], credential)
	}

	tokensByAppID := make(map[uint][]database.APIToken)
	for _, token := range allTokens {
		tokensByAppID[token.AppID] = append(tokensByAppID[token.AppID], token)
	}

	// Convert Apps to protobuf with embedded relationships
	snapshot.Apps = make([]*pb.AppConfig, len(apps))
	for i, app := range apps {
		// Get LLM relationships from lookup map
		appLLMs := llmsByAppID[app.ID]
		llmIDs := make([]uint32, len(appLLMs))
		for j, appLLM := range appLLMs {
			llmIDs[j] = uint32(appLLM.LLMID)
		}

		// Get credentials from lookup map
		credentials := credentialsByAppID[app.ID]
		credentialIDs := make([]uint32, len(credentials))
		for j, cred := range credentials {
			credentialIDs[j] = uint32(cred.ID)
		}

		// Get tokens from lookup map
		tokens := tokensByAppID[app.ID]
		tokenIDs := make([]uint32, len(tokens))
		for j, token := range tokens {
			tokenIDs[j] = uint32(token.ID)
		}
		
		// Convert budget start date to string if available
		var budgetStartDate string
		if app.BudgetStartDate != nil {
			budgetStartDate = app.BudgetStartDate.Format(time.RFC3339)
		}

		snapshot.Apps[i] = &pb.AppConfig{
			Id:             uint32(app.ID),
			Name:           app.Name,
			Description:    app.Description,
			OwnerEmail:     app.OwnerEmail,
			IsActive:       app.IsActive,
			MonthlyBudget:  app.MonthlyBudget,
			BudgetResetDay: int32(app.BudgetResetDay),
			RateLimitRpm:   int32(app.RateLimitRPM),
			Namespace:      app.Namespace,
			CreatedAt:      timestamppb.New(app.CreatedAt),
			UpdatedAt:      timestamppb.New(app.UpdatedAt),
			
			// JSON fields as strings
			BudgetStartDate: budgetStartDate,
			AllowedIps:      string(app.AllowedIPs),
			Metadata:        string(app.Metadata),
			
			// Embedded relationship data - THE CRITICAL FIX
			LlmIds:        llmIDs,        // From app_llms join table - enables LLM access validation
			CredentialIds: credentialIDs, // From credentials table
			TokenIds:      tokenIDs,      // From api_tokens table
		}

		log.Debug().
			Uint("app_id", app.ID).
			Str("app_name", app.Name).
			Int("llm_count", len(llmIDs)).
			Int("credential_count", len(credentialIDs)).
			Int("token_count", len(tokenIDs)).
			Msg("App relationships embedded in sync")
	}
	
	// Query Filters with namespace filtering
	var filters []database.Filter
	filterQuery := s.db.Model(&database.Filter{}).Where("is_active = ?", true)
	if namespace == "" {
		filterQuery = filterQuery.Where("namespace = ''")
	} else {
		filterQuery = filterQuery.Where("(namespace = '' OR namespace = ?)", namespace)
	}
	
	if err := filterQuery.Find(&filters).Error; err != nil {
		return nil, fmt.Errorf("failed to query Filters: %w", err)
	}

	// Collect all Filter IDs for bulk queries to avoid N+1 problem
	var filterIDs []uint
	for _, filter := range filters {
		filterIDs = append(filterIDs, filter.ID)
	}

	// Bulk query filter relationships to avoid N+1 queries
	var allFilterLLMs []database.LLMFilter
	if len(filterIDs) > 0 {
		if err := s.db.Where("filter_id IN ? AND is_active = ?", filterIDs, true).Find(&allFilterLLMs).Error; err != nil {
			log.Warn().Err(err).Msg("Failed to bulk query llm_filters join table")
		}
	}

	// Create lookup map for efficient relationship matching
	llmsByFilterID := make(map[uint][]database.LLMFilter)
	for _, llmFilter := range allFilterLLMs {
		llmsByFilterID[llmFilter.FilterID] = append(llmsByFilterID[llmFilter.FilterID], llmFilter)
	}

	// Convert Filters to protobuf with embedded relationships
	snapshot.Filters = make([]*pb.FilterConfig, len(filters))
	for i, filter := range filters {
		// Get LLM relationships from lookup map
		llmFilters := llmsByFilterID[filter.ID]
		llmIDs := make([]uint32, len(llmFilters))
		for j, llmFilter := range llmFilters {
			llmIDs[j] = uint32(llmFilter.LLMID)
		}

		snapshot.Filters[i] = &pb.FilterConfig{
			Id:          uint32(filter.ID),
			Name:        filter.Name,
			Description: filter.Description,
			Script:      filter.Script,
			IsActive:    filter.IsActive,
			OrderIndex:  int32(filter.OrderIndex),
			Namespace:   filter.Namespace,
			CreatedAt:   timestamppb.New(filter.CreatedAt),
			UpdatedAt:   timestamppb.New(filter.UpdatedAt),
			LlmIds:      llmIDs, // Which LLMs use this filter
		}

		log.Debug().
			Uint("filter_id", filter.ID).
			Str("filter_name", filter.Name).
			Int("llm_count", len(llmIDs)).
			Msg("Filter relationships embedded in sync")
	}

	// Query Plugins with namespace filtering
	var plugins []database.Plugin
	pluginQuery := s.db.Model(&database.Plugin{}).Where("is_active = ?", true)
	if namespace == "" {
		pluginQuery = pluginQuery.Where("namespace = ''")
	} else {
		pluginQuery = pluginQuery.Where("(namespace = '' OR namespace = ?)", namespace)
	}
	
	if err := pluginQuery.Find(&plugins).Error; err != nil {
		return nil, fmt.Errorf("failed to query Plugins: %w", err)
	}

	// Collect all Plugin IDs for bulk queries to avoid N+1 problem
	var pluginIDs []uint
	for _, plugin := range plugins {
		pluginIDs = append(pluginIDs, plugin.ID)
	}

	// Bulk query plugin relationships to avoid N+1 queries
	var allPluginLLMs []database.LLMPlugin
	if len(pluginIDs) > 0 {
		if err := s.db.Where("plugin_id IN ? AND is_active = ?", pluginIDs, true).Order("order_index ASC").Find(&allPluginLLMs).Error; err != nil {
			log.Warn().Err(err).Msg("Failed to bulk query llm_plugins join table")
		}
	}

	// Create lookup map for efficient relationship matching
	llmsByPluginID := make(map[uint][]database.LLMPlugin)
	for _, llmPlugin := range allPluginLLMs {
		llmsByPluginID[llmPlugin.PluginID] = append(llmsByPluginID[llmPlugin.PluginID], llmPlugin)
	}

	// Convert Plugins to protobuf with embedded relationships
	snapshot.Plugins = make([]*pb.PluginConfig, len(plugins))
	for i, plugin := range plugins {
		// Get LLM relationships from lookup map (already ordered by OrderIndex)
		llmPlugins := llmsByPluginID[plugin.ID]
		llmIDs := make([]uint32, len(llmPlugins))
		for j, llmPlugin := range llmPlugins {
			llmIDs[j] = uint32(llmPlugin.LLMID)
		}

		snapshot.Plugins[i] = &pb.PluginConfig{
			Id:          uint32(plugin.ID),
			Name:        plugin.Name,
			Description: plugin.Description,
			Command:     plugin.Command,
			Checksum:    plugin.Checksum,
			Config:      string(plugin.Config), // JSON field as string
			HookType:    plugin.HookType,
			IsActive:    plugin.IsActive,
			Namespace:   plugin.Namespace,
			CreatedAt:   timestamppb.New(plugin.CreatedAt),
			UpdatedAt:   timestamppb.New(plugin.UpdatedAt),
			LlmIds:      llmIDs, // Which LLMs use this plugin
		}

		log.Debug().
			Uint("plugin_id", plugin.ID).
			Str("plugin_name", plugin.Name).
			Str("hook_type", plugin.HookType).
			Int("llm_count", len(llmIDs)).
			Msg("Plugin relationships embedded in sync")
	}

	// Query Model Prices with namespace filtering
	var modelPrices []database.ModelPrice
	priceQuery := s.db.Model(&database.ModelPrice{})
	if namespace == "" {
		priceQuery = priceQuery.Where("namespace = ''")
	} else {
		priceQuery = priceQuery.Where("(namespace = '' OR namespace = ?)", namespace)
	}
	
	if err := priceQuery.Find(&modelPrices).Error; err != nil {
		return nil, fmt.Errorf("failed to query ModelPrices: %w", err)
	}
	
	// Convert Model Prices to protobuf
	snapshot.ModelPrices = make([]*pb.ModelPriceConfig, len(modelPrices))
	for i, price := range modelPrices {
		snapshot.ModelPrices[i] = &pb.ModelPriceConfig{
			Id:           uint32(price.ID),
			Vendor:       price.Vendor,
			ModelName:    price.ModelName,
			Cpt:          price.CPT,
			Cpit:         price.CPIT,
			CacheWritePt: price.CacheWritePT,
			CacheReadPt:  price.CacheReadPT,
			Currency:     price.Currency,
			Namespace:    price.Namespace,
			CreatedAt:    timestamppb.New(price.CreatedAt),
			UpdatedAt:    timestamppb.New(price.UpdatedAt),
		}

		log.Debug().
			Uint("price_id", price.ID).
			Str("vendor", price.Vendor).
			Str("model", price.ModelName).
			Msg("Model price synced")
	}
	
	// Note: Tokens are validated on-demand via gRPC, not synced to edges
	
	log.Info().
		Str("namespace", namespace).
		Str("version", version).
		Int("llm_count", len(snapshot.Llms)).
		Int("app_count", len(snapshot.Apps)).
		Int("filter_count", len(snapshot.Filters)).
		Int("plugin_count", len(snapshot.Plugins)).
		Int("model_price_count", len(snapshot.ModelPrices)).
		Msg("Created complete configuration snapshot (tokens validated on-demand)")
	
	return snapshot, nil
}


// isEdgeStreamActive checks if an edge's stream is still active
func (s *ControlServer) isEdgeStreamActive(edge *EdgeInstance) bool {
	if edge == nil || edge.Stream == nil {
		return false
	}

	// Check if the stream context is still active
	ctx := edge.Stream.Context()
	if ctx.Err() != nil {
		return false
	}

	// Check heartbeat age (consider stale if no heartbeat for 10 minutes)
	heartbeatAge := time.Since(edge.LastHeartbeat)
	if heartbeatAge > 10*time.Minute {
		log.Warn().
			Str("edge_id", edge.EdgeID).
			Dur("heartbeat_age", heartbeatAge).
			Msg("Edge heartbeat is stale")
		return false
	}

	return true
}

// startCleanupRoutine starts the periodic cleanup of stale connections
func (s *ControlServer) startCleanupRoutine() {
	s.cleanupTicker = time.NewTicker(2 * time.Minute) // Run cleanup every 2 minutes
	go func() {
		for range s.cleanupTicker.C {
			s.cleanupStaleConnections()
		}
	}()
	log.Info().Msg("Started edge connection cleanup routine")
}

// cleanupStaleConnections removes disconnected and stale edge connections
func (s *ControlServer) cleanupStaleConnections() {
	s.edgeMutex.Lock()
	defer s.edgeMutex.Unlock()

	var toRemove []string
	for edgeID, edge := range s.edgeInstances {
		if !s.isEdgeStreamActive(edge) {
			log.Info().
				Str("edge_id", edgeID).
				Str("status", edge.Status).
				Time("last_heartbeat", edge.LastHeartbeat).
				Msg("Removing stale edge connection")

			// Update database status
			s.db.Model(&database.EdgeInstance{}).
				Where("edge_id = ?", edgeID).
				Update("status", "disconnected")

			toRemove = append(toRemove, edgeID)
		}
	}

	for _, edgeID := range toRemove {
		delete(s.edgeInstances, edgeID)
	}

	if len(toRemove) > 0 {
		log.Info().Int("removed_count", len(toRemove)).Msg("Cleaned up stale edge connections")
	}
}

// validateEdgeRegistrationRequest validates edge registration request fields
func (s *ControlServer) validateEdgeRegistrationRequest(req *pb.EdgeRegistrationRequest) error {
	if req.EdgeId == "" {
		return status.Error(codes.InvalidArgument, "edge_id is required")
	}

	if len(req.EdgeId) > 64 {
		return status.Error(codes.InvalidArgument, "edge_id must be 64 characters or less")
	}

	if len(req.EdgeNamespace) > 64 {
		return status.Error(codes.InvalidArgument, "edge_namespace must be 64 characters or less")
	}

	if req.Version == "" {
		return status.Error(codes.InvalidArgument, "version is required")
	}

	if len(req.Version) > 32 {
		return status.Error(codes.InvalidArgument, "version must be 32 characters or less")
	}

	if req.Health == nil {
		return status.Error(codes.InvalidArgument, "health status is required")
	}

	// Validate metadata size
	if len(req.Metadata) > 10 {
		return status.Error(codes.InvalidArgument, "metadata cannot have more than 10 entries")
	}

	for key, value := range req.Metadata {
		if len(key) > 64 {
			return status.Error(codes.InvalidArgument, "metadata key must be 64 characters or less")
		}
		if len(value) > 256 {
			return status.Error(codes.InvalidArgument, "metadata value must be 256 characters or less")
		}
	}

	return nil
}

// validateTokenValidationRequest validates token validation request fields
func (s *ControlServer) validateTokenValidationRequest(req *pb.TokenValidationRequest) error {
	if req.Token == "" {
		return status.Error(codes.InvalidArgument, "token is required")
	}

	if len(req.Token) > 128 {
		return status.Error(codes.InvalidArgument, "token must be 128 characters or less")
	}

	if req.EdgeId == "" {
		return status.Error(codes.InvalidArgument, "edge_id is required")
	}

	if len(req.EdgeId) > 64 {
		return status.Error(codes.InvalidArgument, "edge_id must be 64 characters or less")
	}

	if len(req.EdgeNamespace) > 64 {
		return status.Error(codes.InvalidArgument, "edge_namespace must be 64 characters or less")
	}

	return nil
}

// validateAnalyticsPulseRequest validates analytics pulse request fields
func (s *ControlServer) validateAnalyticsPulseRequest(req *pb.AnalyticsPulse) error {
	if req.EdgeId == "" {
		return status.Error(codes.InvalidArgument, "edge_id is required")
	}

	if len(req.EdgeId) > 64 {
		return status.Error(codes.InvalidArgument, "edge_id must be 64 characters or less")
	}

	if req.PulseTimestamp == nil {
		return status.Error(codes.InvalidArgument, "pulse_timestamp is required")
	}

	if req.DataFrom == nil {
		return status.Error(codes.InvalidArgument, "data_from is required")
	}

	if req.DataTo == nil {
		return status.Error(codes.InvalidArgument, "data_to is required")
	}

	// Validate data ranges
	if req.DataFrom.AsTime().After(req.DataTo.AsTime()) {
		return status.Error(codes.InvalidArgument, "data_from must be before data_to")
	}

	// Validate record limits
	totalRecords := len(req.AnalyticsEvents) + len(req.BudgetEvents) + len(req.ProxySummaries)
	if totalRecords > 10000 {
		return status.Error(codes.InvalidArgument, "pulse cannot contain more than 10000 total records")
	}

	if req.TotalRecords != uint32(totalRecords) {
		return status.Error(codes.InvalidArgument, "total_records field does not match actual record count")
	}

	return nil
}