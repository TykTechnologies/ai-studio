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
	edgeInstances map[string]*EdgeInstance
	edgeMutex     sync.RWMutex
	
	// Change propagation
	changeChan chan *pb.ConfigurationChange
	ctx        context.Context
	cancel     context.CancelFunc
	
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
	ctx, cancel := context.WithCancel(context.Background())

	server := &ControlServer{
		config:        cfg,
		db:            db,
		edgeInstances: make(map[string]*EdgeInstance),
		changeChan:    make(chan *pb.ConfigurationChange, 100),
		ctx:           ctx,
		cancel:        cancel,
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
	
	// Start change propagation worker
	go s.changePropagationWorker()
	
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

	s.cancel()

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

// authenticate checks the authentication token
func (s *ControlServer) authenticate(ctx context.Context) error {
	if s.config.HubSpoke.AuthToken == "" {
		// No authentication required
		return nil
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
	if token != "Bearer "+s.config.HubSpoke.AuthToken {
		return status.Error(codes.Unauthenticated, "invalid authorization token")
	}
	
	return nil
}

// RegisterEdge handles edge instance registration
func (s *ControlServer) RegisterEdge(ctx context.Context, req *pb.EdgeRegistrationRequest) (*pb.EdgeRegistrationResponse, error) {
	log.Info().
		Str("edge_id", req.EdgeId).
		Str("namespace", req.EdgeNamespace).
		Str("version", req.Version).
		Msg("Edge instance registration request")
	
	// Validate request
	if req.EdgeId == "" {
		return nil, status.Error(codes.InvalidArgument, "edge_id is required")
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
	var edgeID string
	var edge *EdgeInstance
	
	// Handle incoming messages from edge
	go func() {
		for {
			msg, err := stream.Recv()
			if err != nil {
				log.Debug().Err(err).Str("edge_id", edgeID).Msg("Edge stream receive error")
				break
			}
			
			switch m := msg.Message.(type) {
			case *pb.EdgeMessage_Registration:
				// Handle registration in stream
				if edge == nil {
					edgeID = m.Registration.EdgeId
					s.edgeMutex.RLock()
					edge = s.edgeInstances[edgeID]
					s.edgeMutex.RUnlock()
					
					if edge != nil {
						edge.Stream = stream
						edge.Status = "connected"
						
						// Send registration response
						response := &pb.ControlMessage{
							Message: &pb.ControlMessage_RegistrationResponse{
								RegistrationResponse: &pb.EdgeRegistrationResponse{
									Success:   true,
									Message:   "Stream connected",
									SessionId: edge.SessionID,
								},
							},
						}
						stream.Send(response)
					}
				}
				
			case *pb.EdgeMessage_Heartbeat:
				// Handle heartbeat
				if edge != nil {
					edge.LastHeartbeat = time.Now()
					
					// Send heartbeat response
					response := &pb.ControlMessage{
						Message: &pb.ControlMessage_HeartbeatResponse{
							HeartbeatResponse: &pb.HeartbeatResponse{
								Acknowledged: true,
								Message:      "Heartbeat received",
							},
						},
					}
					stream.Send(response)
				}
				
			case *pb.EdgeMessage_ConfigRequest:
				// Handle configuration request
				if edge != nil {
					snapshot, err := s.getConfigurationSnapshot(edge.Namespace)
					if err != nil {
						log.Error().Err(err).Str("edge_id", edgeID).Msg("Failed to get configuration snapshot")
					} else {
						response := &pb.ControlMessage{
							Message: &pb.ControlMessage_Configuration{
								Configuration: snapshot,
							},
						}
						stream.Send(response)
					}
				}
				
			case *pb.EdgeMessage_ReloadResponse:
				// Handle reload status response
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
			}
		}
	}()
	
	// Keep connection alive and handle outgoing messages
	<-stream.Context().Done()
	
	// Cleanup when stream closes
	if edge != nil {
		s.edgeMutex.Lock()
		if existingEdge, exists := s.edgeInstances[edgeID]; exists && existingEdge == edge {
			existingEdge.Status = "disconnected"
			existingEdge.Stream = nil
		}
		s.edgeMutex.Unlock()
	}
	
	log.Debug().Str("edge_id", edgeID).Msg("Edge stream closed")
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

	return &pb.TokenValidationResponse{
		Valid:     true,
		AppId:     uint32(apiToken.AppID),
		AppName:   apiToken.App.Name,
		Scopes:    scopes,
		ExpiresAt: expiresAt,
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
	
	// Convert LLMs to protobuf with embedded relationships
	snapshot.Llms = make([]*pb.LLMConfig, len(llms))
	for i, llm := range llms {
		// Query app_llms join table to get which apps can access this LLM
		var appLLMs []database.AppLLM
		if err := s.db.Where("llm_id = ? AND is_active = ?", llm.ID, true).Find(&appLLMs).Error; err != nil {
			log.Warn().Err(err).Uint("llm_id", llm.ID).Msg("Failed to query app_llms join table")
		}
		
		appIDs := make([]uint32, len(appLLMs))
		for j, appLLM := range appLLMs {
			appIDs[j] = uint32(appLLM.AppID)
		}

		// Query llm_plugins join table
		var llmPlugins []database.LLMPlugin
		if err := s.db.Where("llm_id = ? AND is_active = ?", llm.ID, true).Find(&llmPlugins).Error; err != nil {
			log.Warn().Err(err).Uint("llm_id", llm.ID).Msg("Failed to query llm_plugins join table")
		}
		
		pluginIDs := make([]uint32, len(llmPlugins))
		for j, llmPlugin := range llmPlugins {
			pluginIDs[j] = uint32(llmPlugin.PluginID)
		}

		// Query llm_filters join table
		var llmFilters []database.LLMFilter
		if err := s.db.Where("llm_id = ? AND is_active = ?", llm.ID, true).Find(&llmFilters).Error; err != nil {
			log.Warn().Err(err).Uint("llm_id", llm.ID).Msg("Failed to query llm_filters join table")
		}
		
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
	
	// Convert Apps to protobuf with embedded relationships
	snapshot.Apps = make([]*pb.AppConfig, len(apps))
	for i, app := range apps {
		// Query app_llms join table to get LLM associations - THE CRITICAL MISSING PIECE
		var appLLMs []database.AppLLM
		if err := s.db.Where("app_id = ? AND is_active = ?", app.ID, true).Find(&appLLMs).Error; err != nil {
			log.Warn().Err(err).Uint("app_id", app.ID).Msg("Failed to query app_llms join table")
		}
		
		// Extract LLM IDs that this app can access
		llmIDs := make([]uint32, len(appLLMs))
		for j, appLLM := range appLLMs {
			llmIDs[j] = uint32(appLLM.LLMID)
		}

		// Query credentials for this app
		var credentials []database.Credential
		if err := s.db.Where("app_id = ?", app.ID).Find(&credentials).Error; err != nil {
			log.Warn().Err(err).Uint("app_id", app.ID).Msg("Failed to query app credentials")
		}
		
		credentialIDs := make([]uint32, len(credentials))
		for j, cred := range credentials {
			credentialIDs[j] = uint32(cred.ID)
		}

		// Query tokens for this app (even though we do on-demand validation, include for completeness)
		var tokens []database.APIToken
		if err := s.db.Where("app_id = ?", app.ID).Find(&tokens).Error; err != nil {
			log.Warn().Err(err).Uint("app_id", app.ID).Msg("Failed to query app tokens")
		}
		
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
	
	// Convert Filters to protobuf with embedded relationships
	snapshot.Filters = make([]*pb.FilterConfig, len(filters))
	for i, filter := range filters {
		// Query llm_filters join table to get which LLMs use this filter
		var llmFilters []database.LLMFilter
		if err := s.db.Where("filter_id = ? AND is_active = ?", filter.ID, true).Find(&llmFilters).Error; err != nil {
			log.Warn().Err(err).Uint("filter_id", filter.ID).Msg("Failed to query llm_filters join table")
		}
		
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
	
	// Convert Plugins to protobuf with embedded relationships
	snapshot.Plugins = make([]*pb.PluginConfig, len(plugins))
	for i, plugin := range plugins {
		// Query llm_plugins join table to get which LLMs use this plugin (ordered by OrderIndex)
		var llmPlugins []database.LLMPlugin
		if err := s.db.Where("plugin_id = ? AND is_active = ?", plugin.ID, true).Order("order_index ASC").Find(&llmPlugins).Error; err != nil {
			log.Warn().Err(err).Uint("plugin_id", plugin.ID).Msg("Failed to query llm_plugins join table")
		}
		
		llmIDs := make([]uint32, len(llmPlugins))
		for j, llmPlugin := range llmPlugins {
			llmIDs[j] = uint32(llmPlugin.LLMID)
		}

		snapshot.Plugins[i] = &pb.PluginConfig{
			Id:          uint32(plugin.ID),
			Name:        plugin.Name,
			Slug:        plugin.Slug,
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

// changePropagationWorker processes configuration changes and propagates to edges
func (s *ControlServer) changePropagationWorker() {
	for {
		select {
		case change := <-s.changeChan:
			s.propagateChange(change)
		case <-s.ctx.Done():
			return
		}
	}
}

// propagateChange sends a configuration change to relevant edge instances
func (s *ControlServer) propagateChange(change *pb.ConfigurationChange) {
	s.edgeMutex.RLock()
	defer s.edgeMutex.RUnlock()
	
	for edgeID, edge := range s.edgeInstances {
		// Check if edge should receive this change based on namespace
		if change.Namespace == "" || change.Namespace == edge.Namespace {
			if edge.Stream != nil && edge.Status == "connected" {
				message := &pb.ControlMessage{
					Message: &pb.ControlMessage_Change{
						Change: change,
					},
				}
				
				if err := edge.Stream.Send(message); err != nil {
					log.Error().Err(err).Str("edge_id", edgeID).Msg("Failed to send change to edge")
					edge.Status = "unhealthy"
				}
			}
		}
	}
}

// PropagateChange queues a configuration change for propagation
func (s *ControlServer) PropagateChange(change *pb.ConfigurationChange) {
	select {
	case s.changeChan <- change:
		log.Debug().
			Str("type", change.ChangeType.String()).
			Str("entity_type", change.EntityType.String()).
			Uint32("entity_id", change.EntityId).
			Str("namespace", change.Namespace).
			Msg("Queued configuration change for propagation")
	default:
		log.Warn().Msg("Change propagation channel full, dropping change")
	}
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
		for {
			select {
			case <-s.cleanupTicker.C:
				s.cleanupStaleConnections()
			case <-s.ctx.Done():
				return
			}
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