// grpc/control_server.go
package grpc

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/TykTechnologies/midsommar/v2/analytics"
	"github.com/TykTechnologies/midsommar/v2/models"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/TykTechnologies/midsommar/v2/secrets"
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

// EdgeInstance represents an active edge instance connection
type EdgeInstanceConnection struct {
	EdgeID        string
	Namespace     string
	Status        string
	Version       string
	SessionID     string
	Stream        pb.ConfigurationSyncService_SubscribeToChangesServer
	LastHeartbeat time.Time
}

// ControlServer implements the ConfigurationSyncService for AI Studio control instances
type ControlServer struct {
	pb.UnimplementedConfigurationSyncServiceServer
	
	db     *gorm.DB
	config *Config
	
	// Edge instance management
	edgeConnections map[string]*EdgeInstanceConnection
	edgeMutex       sync.RWMutex

	// gRPC server
	grpcServer *grpc.Server

	// Cleanup ticker for stale connections
	cleanupTicker *time.Ticker

	// Reload coordination (set after creation to avoid import cycle)
	reloadCoordinator interface{} // Will be *services.ReloadCoordinator
}

// Config holds the control server configuration
type Config struct {
	GRPCPort       int
	GRPCHost       string
	TLSEnabled     bool
	TLSCertPath    string
	TLSKeyPath     string
	AuthToken      string
	NextAuthToken  string
}

// NewControlServer creates a new control server for AI Studio
func NewControlServer(cfg *Config, db *gorm.DB) *ControlServer {
	server := &ControlServer{
		config:          cfg,
		db:              db,
		edgeConnections: make(map[string]*EdgeInstanceConnection),
	}

	// Initialize AI Studio's analytics system for processing edge pulse data
	ctx := context.Background()
	analytics.StartRecording(ctx, db)
	log.Info().Msg("AI Studio analytics system initialized for control server")

	// Start cleanup routine
	server.startCleanupRoutine()

	return server
}

// Start starts the gRPC control server
func (s *ControlServer) Start() error {
	// Create listener
	addr := fmt.Sprintf("%s:%d", s.config.GRPCHost, s.config.GRPCPort)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}
	
	// Setup gRPC server options
	var opts []grpc.ServerOption
	
	// Add TLS if enabled
	if s.config.TLSEnabled {
		creds, err := credentials.NewServerTLSFromFile(
			s.config.TLSCertPath,
			s.config.TLSKeyPath,
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

	log.Info().Str("address", addr).Msg("Starting AI Studio gRPC control server")
	
	// Start serving
	if err := s.grpcServer.Serve(listener); err != nil {
		return fmt.Errorf("gRPC server failed: %w", err)
	}
	
	return nil
}

// Stop stops the gRPC server gracefully
func (s *ControlServer) Stop() {
	log.Info().Msg("Stopping AI Studio gRPC control server")

	// Stop cleanup routine
	if s.cleanupTicker != nil {
		s.cleanupTicker.Stop()
	}

	if s.grpcServer != nil {
		s.grpcServer.GracefulStop()
	}
	
	// Close all edge connections
	s.edgeMutex.Lock()
	for _, edge := range s.edgeConnections {
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

// RegisterEdge handles edge instance registration
func (s *ControlServer) RegisterEdge(ctx context.Context, req *pb.EdgeRegistrationRequest) (*pb.EdgeRegistrationResponse, error) {
	log.Info().
		Str("edge_id", req.EdgeId).
		Str("namespace", req.EdgeNamespace).
		Str("version", req.Version).
		Msg("AI Studio control server: edge registration request")

	// Generate session ID
	sessionID := uuid.New().String()

	// Create or update edge instance in database
	var edgeInstance models.EdgeInstance
	err := edgeInstance.GetByEdgeID(s.db, req.EdgeId)
	if err != nil && err != gorm.ErrRecordNotFound {
		return nil, status.Error(codes.Internal, "failed to check edge instance")
	}

	if err == gorm.ErrRecordNotFound {
		// Create new edge instance
		edgeInstance = models.EdgeInstance{
			EdgeID:    req.EdgeId,
			Namespace: req.EdgeNamespace,
			Version:   req.Version,
			BuildHash: req.BuildHash,
			Status:    models.EdgeStatusRegistered,
			SessionID: sessionID,
		}
		
		// Convert metadata
		if req.Metadata != nil {
			metadata := make(map[string]interface{})
			for k, v := range req.Metadata {
				metadata[k] = v
			}
			edgeInstance.Metadata = metadata
		}
		
		if err := edgeInstance.Create(s.db); err != nil {
			return nil, status.Error(codes.Internal, "failed to create edge instance")
		}
	} else {
		// Update existing edge instance
		edgeInstance.Version = req.Version
		edgeInstance.BuildHash = req.BuildHash
		edgeInstance.Status = models.EdgeStatusRegistered
		edgeInstance.SessionID = sessionID
		
		if req.Metadata != nil {
			metadata := make(map[string]interface{})
			for k, v := range req.Metadata {
				metadata[k] = v
			}
			edgeInstance.Metadata = metadata
		}
		
		if err := edgeInstance.Update(s.db); err != nil {
			return nil, status.Error(codes.Internal, "failed to update edge instance")
		}
	}

	// Get initial configuration
	initialConfig, err := s.getConfigurationSnapshot(req.EdgeNamespace)
	if err != nil {
		log.Error().Err(err).Str("edge_id", req.EdgeId).Msg("Failed to get initial configuration")
		initialConfig = &pb.ConfigurationSnapshot{
			Version: "0",
			Llms:    []*pb.LLMConfig{},
			Apps:    []*pb.AppConfig{},
		}
	}

	return &pb.EdgeRegistrationResponse{
		Success:       true,
		Message:       "Edge registered successfully with AI Studio",
		SessionId:     sessionID,
		InitialConfig: initialConfig,
	}, nil
}

// GetFullConfiguration retrieves a complete configuration snapshot for an edge
func (s *ControlServer) GetFullConfiguration(ctx context.Context, req *pb.ConfigurationRequest) (*pb.ConfigurationSnapshot, error) {
	log.Debug().
		Str("namespace", req.EdgeNamespace).
		Msg("AI Studio control server: full configuration request")
	
	snapshot, err := s.getConfigurationSnapshot(req.EdgeNamespace)
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to get configuration: %v", err))
	}
	
	return snapshot, nil
}

// SubscribeToChanges handles bidirectional streaming for real-time updates
func (s *ControlServer) SubscribeToChanges(stream pb.ConfigurationSyncService_SubscribeToChangesServer) error {
	var edgeID string
	var edgeConnection *EdgeInstanceConnection
	
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
				if edgeConnection == nil {
					edgeID = m.Registration.EdgeId
					s.edgeMutex.Lock()
					edgeConnection = &EdgeInstanceConnection{
						EdgeID:        m.Registration.EdgeId,
						Namespace:     m.Registration.EdgeNamespace,
						Status:        "connected",
						Version:       m.Registration.Version,
						Stream:        stream,
						LastHeartbeat: time.Now(),
					}
					s.edgeConnections[edgeID] = edgeConnection
					s.edgeMutex.Unlock()
					
					// Send registration response
					response := &pb.ControlMessage{
						Message: &pb.ControlMessage_RegistrationResponse{
							RegistrationResponse: &pb.EdgeRegistrationResponse{
								Success:   true,
								Message:   "Stream connected to AI Studio",
								SessionId: edgeConnection.SessionID,
							},
						},
					}
					stream.Send(response)
				}
				
			case *pb.EdgeMessage_Heartbeat:
				// Handle heartbeat
				if edgeConnection != nil {
					edgeConnection.LastHeartbeat = time.Now()
					
					// Update database
					var edgeInstance models.EdgeInstance
					if err := edgeInstance.GetByEdgeID(s.db, edgeID); err == nil {
						edgeInstance.UpdateHeartbeat(s.db)
					}
					
					// Send heartbeat response
					response := &pb.ControlMessage{
						Message: &pb.ControlMessage_HeartbeatResponse{
							HeartbeatResponse: &pb.HeartbeatResponse{
								Acknowledged: true,
								Message:      "Heartbeat received by AI Studio",
							},
						},
					}
					stream.Send(response)
				}
				
			case *pb.EdgeMessage_ConfigRequest:
				// Handle configuration request
				if edgeConnection != nil {
					snapshot, err := s.getConfigurationSnapshot(edgeConnection.Namespace)
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
	if edgeConnection != nil {
		s.edgeMutex.Lock()
		if existingEdge, exists := s.edgeConnections[edgeID]; exists && existingEdge == edgeConnection {
			existingEdge.Status = "disconnected"
			existingEdge.Stream = nil
		}
		s.edgeMutex.Unlock()
		
		// Update database
		var edgeInstance models.EdgeInstance
		if err := edgeInstance.GetByEdgeID(s.db, edgeID); err == nil {
			edgeInstance.UpdateStatus(s.db, models.EdgeStatusDisconnected)
		}
	}
	
	log.Debug().Str("edge_id", edgeID).Msg("Edge stream closed")
	return nil
}

// SendHeartbeat handles heartbeat requests
func (s *ControlServer) SendHeartbeat(ctx context.Context, req *pb.HeartbeatRequest) (*pb.HeartbeatResponse, error) {
	s.edgeMutex.RLock()
	edge, exists := s.edgeConnections[req.EdgeId]
	s.edgeMutex.RUnlock()
	
	if !exists {
		return nil, status.Error(codes.NotFound, "edge instance not found")
	}
	
	// Update heartbeat
	edge.LastHeartbeat = time.Now()
	
	// Update database
	var edgeInstance models.EdgeInstance
	if err := edgeInstance.GetByEdgeID(s.db, req.EdgeId); err == nil {
		edgeInstance.UpdateHeartbeat(s.db)
	}
	
	return &pb.HeartbeatResponse{
		Acknowledged: true,
		Message:      "Heartbeat acknowledged by AI Studio",
	}, nil
}

// UnregisterEdge handles edge instance unregistration
func (s *ControlServer) UnregisterEdge(ctx context.Context, req *pb.EdgeUnregistrationRequest) (*emptypb.Empty, error) {
	log.Info().Str("edge_id", req.EdgeId).Str("reason", req.Reason).Msg("Edge unregistration request")
	
	s.edgeMutex.Lock()
	delete(s.edgeConnections, req.EdgeId)
	s.edgeMutex.Unlock()
	
	// Update database
	var edgeInstance models.EdgeInstance
	if err := edgeInstance.GetByEdgeID(s.db, req.EdgeId); err == nil {
		edgeInstance.UpdateStatus(s.db, "unregistered")
	}
	
	return &emptypb.Empty{}, nil
}

// ValidateToken validates an API token on-demand with namespace filtering
func (s *ControlServer) ValidateToken(ctx context.Context, req *pb.TokenValidationRequest) (*pb.TokenValidationResponse, error) {
	tokenPrefix := req.Token
	if len(req.Token) > 8 {
		tokenPrefix = req.Token[:8]
	}
	
	log.Info().
		Str("token_prefix", tokenPrefix).
		Str("edge_id", req.EdgeId).
		Str("edge_namespace", req.EdgeNamespace).
		Msg("AI Studio control server: on-demand token validation request")

	// Query credential (secret) - tokens are global in AI Studio
	var credential models.Credential
	err := s.db.Where("secret = ? AND active = ?", req.Token, true).
		First(&credential).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			log.Info().
				Str("token_prefix", tokenPrefix).
				Str("edge_namespace", req.EdgeNamespace).
				Msg("AI Studio control server: credential not found")
			
			return &pb.TokenValidationResponse{
				Valid:        false,
				ErrorMessage: "Invalid token",
			}, nil
		}
		
		log.Error().Err(err).Str("token_prefix", tokenPrefix).Msg("AI Studio control server: credential validation database error")
		return nil, status.Error(codes.Internal, "token validation failed")
	}

	// Get the associated app
	var app models.App
	if err := s.db.Where("credential_id = ? AND is_active = ?", credential.ID, true).First(&app).Error; err != nil {
		log.Info().Str("token_prefix", tokenPrefix).Uint("credential_id", credential.ID).Msg("AI Studio control server: app not found or inactive")
		return &pb.TokenValidationResponse{
			Valid:        false,
			ErrorMessage: "Associated app not found or inactive",
		}, nil
	}

	log.Info().
		Str("token_prefix", tokenPrefix).
		Uint("app_id", app.ID).
		Str("app_name", app.Name).
		Msg("AI Studio control server: token validation successful")

	return &pb.TokenValidationResponse{
		Valid:     true,
		AppId:     uint32(app.ID),
		AppName:   app.Name,
		Scopes:    []string{}, // AI Studio doesn't use scopes like microgateway
		ExpiresAt: nil,        // AI Studio credentials don't expire
	}, nil
}

// SendAnalyticsPulse handles analytics pulse data from edge instances
func (s *ControlServer) SendAnalyticsPulse(ctx context.Context, req *pb.AnalyticsPulse) (*pb.AnalyticsPulseResponse, error) {
	log.Info().
		Str("edge_id", req.EdgeId).
		Str("edge_namespace", req.EdgeNamespace).
		Uint64("sequence_number", req.SequenceNumber).
		Uint32("total_records", req.TotalRecords).
		Int("analytics_events", len(req.AnalyticsEvents)).
		Int("budget_events", len(req.BudgetEvents)).
		Int("proxy_summaries", len(req.ProxySummaries)).
		Msg("AI Studio control server: received analytics pulse from edge")

	processedRecords := uint64(0)

	// Process analytics events using AI Studio's native analytics system
	for _, event := range req.AnalyticsEvents {
		// Use model name and vendor from pulse event (extracted from actual request)
		modelName := event.ModelName
		vendor := event.Vendor
		if modelName == "" {
			modelName = "unknown-model"
		}
		if vendor == "" {
			vendor = s.extractVendorFromEvent(event)
		}

		// Create ProxyLog for request/response tracking
		proxyLog := &models.ProxyLog{
			AppID:        uint(event.AppId),
			UserID:       0, // Edge doesn't track individual users
			Vendor:       vendor,
			RequestBody:  event.RequestBody,  // Now included from pulse if configured
			ResponseBody: event.ResponseBody, // Now included from pulse if configured
			ResponseCode: int(event.StatusCode),
			TimeStamp:    event.Timestamp.AsTime(),
		}

		// Create LLMChatRecord for analytics (tokens, cost, usage tracking)
		chatRecord := &models.LLMChatRecord{
			LLMID:           uint(event.LlmId),
			AppID:           uint(event.AppId),
			Name:            modelName, // Use actual model name from request
			Vendor:          vendor,
			TotalTokens:     int(event.TotalTokens),
			PromptTokens:    int(event.RequestTokens),
			ResponseTokens:  int(event.ResponseTokens),
			Cost:            event.Cost * 10000, // Convert to cents for AI Studio format
			Currency:        "USD",
			TimeStamp:       event.Timestamp.AsTime(),
			InteractionType: models.ProxyInteraction, // Mark as proxy interaction
			UserID:          0, // Edge doesn't track individual users
			ChatID:          "", // Not applicable for proxy
			Choices:         1, // Default
			ToolCalls:       0, // Default for proxy
		}

		// Use AI Studio's native analytics system for both records
		analytics.RecordProxyLog(proxyLog)
		analytics.RecordChatRecord(chatRecord)
		processedRecords++

		log.Debug().
			Str("edge_id", req.EdgeId).
			Str("request_id", event.RequestId).
			Str("model", modelName).
			Int("total_tokens", int(event.TotalTokens)).
			Float64("cost", event.Cost).
			Bool("has_request_body", len(event.RequestBody) > 0).
			Bool("has_response_body", len(event.ResponseBody) > 0).
			Msg("Analytics event processed via AI Studio analytics system")
	}

	// Process budget events (for now just log - AI Studio budget integration would need budget service)
	for _, budget := range req.BudgetEvents {
		log.Debug().
			Str("edge_id", req.EdgeId).
			Uint32("app_id", budget.AppId).
			Uint32("llm_id", budget.LlmId).
			Int64("tokens_used", budget.TokensUsed).
			Float64("cost", budget.Cost).
			Msg("Budget usage data from edge - processed")
		processedRecords++
	}

	// Process proxy summaries (for now just log - could be stored in separate summary table)
	for _, proxy := range req.ProxySummaries {
		log.Debug().
			Str("edge_id", req.EdgeId).
			Uint32("app_id", proxy.AppId).
			Str("vendor", proxy.Vendor).
			Uint32("request_count", proxy.RequestCount).
			Float64("total_cost", proxy.TotalCost).
			Msg("Proxy summary from edge - processed")
		processedRecords++
	}

	log.Info().
		Str("edge_id", req.EdgeId).
		Uint64("sequence_number", req.SequenceNumber).
		Uint64("processed_records", processedRecords).
		Msg("Analytics pulse processed via AI Studio native analytics system")

	return &pb.AnalyticsPulseResponse{
		Success:          true,
		Message:          "Analytics pulse processed successfully",
		ProcessedRecords: processedRecords,
		SequenceNumber:   req.SequenceNumber,
		ProcessedAt:      timestamppb.Now(),
		// UpdatedConfig: nil, // No config updates for now
	}, nil
}

// extractVendorFromEvent extracts vendor from analytics event
func (s *ControlServer) extractVendorFromEvent(event *pb.AnalyticsEvent) string {
	// Fallback: lookup LLM vendor from database
	if event.LlmId > 0 {
		var llm models.LLM
		if err := s.db.First(&llm, event.LlmId).Error; err == nil {
			return string(llm.Vendor)
		}
	}

	// Extract vendor from endpoint as fallback
	if event.Endpoint != "" {
		if strings.Contains(event.Endpoint, "openai") || strings.Contains(event.Endpoint, "/v1/chat") {
			return "openai"
		}
		if strings.Contains(event.Endpoint, "anthropic") {
			return "anthropic"
		}
		if strings.Contains(event.Endpoint, "vertex") {
			return "vertex"
		}
	}

	return "unknown"
}

// extractModelNameFromEvent extracts model name from analytics event
func (s *ControlServer) extractModelNameFromEvent(event *pb.AnalyticsEvent) string {
	// Primary source: lookup LLM default model from database
	if event.LlmId > 0 {
		var llm models.LLM
		if err := s.db.First(&llm, event.LlmId).Error; err == nil {
			if llm.DefaultModel != "" {
				return llm.DefaultModel
			}
			// Fallback to LLM name if no default model
			return llm.Name
		}
	}

	// Extract model from endpoint as fallback
	if event.Endpoint != "" {
		if strings.Contains(event.Endpoint, "gpt-4") {
			return "gpt-4"
		}
		if strings.Contains(event.Endpoint, "gpt-3.5") {
			return "gpt-3.5-turbo"
		}
		if strings.Contains(event.Endpoint, "claude") {
			return "claude-3-sonnet"
		}
	}

	return "unknown-model"
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
	// If no auth tokens configured, allow connection
	if s.config.AuthToken == "" && s.config.NextAuthToken == "" {
		log.Warn().Msg("No authentication tokens configured - allowing unauthenticated access")
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

	// Check current token
	if s.config.AuthToken != "" && token == "Bearer "+s.config.AuthToken {
		return nil
	}

	// Check next token (for rotation)
	if s.config.NextAuthToken != "" && token == "Bearer "+s.config.NextAuthToken {
		log.Debug().Msg("Edge authenticated with next token (rotation in progress)")
		return nil
	}

	log.Warn().Msg("Authentication failed: invalid authorization token")
	return status.Error(codes.Unauthenticated, "invalid authorization token")
}

// getConfigurationSnapshot generates a complete configuration snapshot for an edge namespace
func (s *ControlServer) getConfigurationSnapshot(namespace string) (*pb.ConfigurationSnapshot, error) {
	snapshot := &pb.ConfigurationSnapshot{
		Version: fmt.Sprintf("%d", time.Now().Unix()),
		Llms:    []*pb.LLMConfig{},
		Apps:    []*pb.AppConfig{},
		ModelPrices: []*pb.ModelPriceConfig{},
		Filters: []*pb.FilterConfig{},
		Plugins: []*pb.PluginConfig{},
	}

	// Get LLMs for namespace with preloaded relationships
	var llms []models.LLM
	llmQuery := s.db.Preload("Filters").Where("active = ?", true)
	if namespace == "" {
		// Global namespace - only global LLMs
		llmQuery = llmQuery.Where("namespace = ''")
	} else {
		// Specific namespace - global + matching namespace
		llmQuery = llmQuery.Where("(namespace = '' OR namespace = ?)", namespace)
	}
	
	if err := llmQuery.Find(&llms).Error; err != nil {
		return nil, fmt.Errorf("failed to get LLMs: %w", err)
	}

	// Convert LLMs to protobuf with complete configuration
	for _, llm := range llms {
		// Create slug from name (microgateway expects slugs)
		slug := strings.ToLower(strings.ReplaceAll(llm.Name, " ", "-"))
		
		// Get filter IDs for this LLM
		filterIDs := make([]uint32, len(llm.Filters))
		for i, filter := range llm.Filters {
			filterIDs[i] = uint32(filter.ID)
		}
		
		// Handle optional monthly budget
		var monthlyBudget float64
		if llm.MonthlyBudget != nil {
			monthlyBudget = *llm.MonthlyBudget
		}
		
		// Resolve secret references for microgateway
		resolvedAPIKey := secrets.GetValue(llm.APIKey, false) // false to resolve actual value
		resolvedEndpoint := secrets.GetValue(llm.APIEndpoint, false)
		
		// Encrypt API key using microgateway's encryption format
		encryptedAPIKey, err := s.encryptForMicrogateway(resolvedAPIKey)
		if err != nil {
			log.Error().Err(err).Uint("llm_id", llm.ID).Msg("Failed to encrypt API key for microgateway")
			encryptedAPIKey = resolvedAPIKey // Fallback to plaintext
		}
		
		pbLLM := &pb.LLMConfig{
			Id:               uint32(llm.ID),
			Name:             llm.Name,
			Slug:             slug,
			Vendor:           string(llm.Vendor),
			Endpoint:         resolvedEndpoint,
			ApiKeyEncrypted:  encryptedAPIKey, // Encrypted using microgateway's format
			DefaultModel:     llm.DefaultModel,
			MaxTokens:        4096, // Default value
			TimeoutSeconds:   30,   // Default value
			RetryCount:       3,    // Default value
			IsActive:         llm.Active,
			MonthlyBudget:    monthlyBudget,
			RateLimitRpm:     0,    // AI Studio doesn't have this field yet
			Namespace:        llm.Namespace,
			FilterIds:        filterIDs,
			CreatedAt:        timestamppb.New(llm.CreatedAt),
			UpdatedAt:        timestamppb.New(llm.UpdatedAt),
		}
		snapshot.Llms = append(snapshot.Llms, pbLLM)
	}

	// Get Apps for namespace with relationships
	var apps []models.App
	appQuery := s.db.Preload("LLMs").Where("is_active = ?", true)
	if namespace == "" {
		// Global namespace - only global apps
		appQuery = appQuery.Where("namespace = ''")
	} else {
		// Specific namespace - global + matching namespace
		appQuery = appQuery.Where("(namespace = '' OR namespace = ?)", namespace)
	}
	
	if err := appQuery.Find(&apps).Error; err != nil {
		return nil, fmt.Errorf("failed to get Apps: %w", err)
	}

	// Convert Apps to protobuf with LLM associations
	for _, app := range apps {
		// Get associated LLM IDs
		llmIDs := make([]uint32, len(app.LLMs))
		for i, llm := range app.LLMs {
			llmIDs[i] = uint32(llm.ID)
		}
		
		// Handle optional monthly budget
		var monthlyBudget float64
		if app.MonthlyBudget != nil {
			monthlyBudget = *app.MonthlyBudget
		}
		
		pbApp := &pb.AppConfig{
			Id:            uint32(app.ID),
			Name:          app.Name,
			Description:   app.Description,
			OwnerEmail:    "", // AI Studio doesn't have owner email field yet
			IsActive:      app.IsActive,
			MonthlyBudget: monthlyBudget,
			Namespace:     app.Namespace,
			LlmIds:        llmIDs,
			CreatedAt:     timestamppb.New(app.CreatedAt),
			UpdatedAt:     timestamppb.New(app.UpdatedAt),
		}
		snapshot.Apps = append(snapshot.Apps, pbApp)
	}

	// Get Filters for namespace
	var filters []models.Filter
	filterQuery := s.db
	if namespace == "" {
		filterQuery = filterQuery.Where("namespace = ''")
	} else {
		filterQuery = filterQuery.Where("(namespace = '' OR namespace = ?)", namespace)
	}
	
	if err := filterQuery.Find(&filters).Error; err != nil {
		return nil, fmt.Errorf("failed to get Filters: %w", err)
	}

	// Convert Filters to protobuf
	for _, filter := range filters {
		pbFilter := &pb.FilterConfig{
			Id:          uint32(filter.ID),
			Name:        filter.Name,
			Description: filter.Description,
			Script:      string(filter.Script),
			IsActive:    true, // AI Studio Filter model doesn't have IsActive field yet
			OrderIndex:  0,    // AI Studio doesn't have OrderIndex field yet
			Namespace:   filter.Namespace,
			CreatedAt:   timestamppb.New(filter.CreatedAt),
			UpdatedAt:   timestamppb.New(filter.UpdatedAt),
		}
		snapshot.Filters = append(snapshot.Filters, pbFilter)
	}

	// Get ModelPrices for namespace
	var modelPrices []models.ModelPrice
	priceQuery := s.db
	// ModelPrice doesn't have namespace field in AI Studio yet, so get all for now
	if err := priceQuery.Find(&modelPrices).Error; err != nil {
		return nil, fmt.Errorf("failed to get ModelPrices: %w", err)
	}

	// Convert ModelPrices to protobuf
	for _, price := range modelPrices {
		pbPrice := &pb.ModelPriceConfig{
			Id:           uint32(price.ID),
			Vendor:       price.Vendor,
			ModelName:    price.ModelName,
			Cpt:          price.CPT,
			Cpit:         price.CPIT,
			CacheWritePt: price.CacheWritePT,
			CacheReadPt:  price.CacheReadPT,
			Currency:     price.Currency,
			Namespace:    "", // AI Studio ModelPrice doesn't have namespace yet
			CreatedAt:    timestamppb.New(price.CreatedAt),
			UpdatedAt:    timestamppb.New(price.UpdatedAt),
		}
		snapshot.ModelPrices = append(snapshot.ModelPrices, pbPrice)
	}

	// Get Plugins for namespace
	log.Info().Str("namespace", namespace).Msg("Starting plugin query for configuration snapshot")
	
	// First, let's test without Preload to eliminate interference
	var plugins []models.Plugin
	var pluginQuery *gorm.DB
	
	// Test basic plugin count first
	var totalActivePlugins int64
	if err := s.db.Model(&models.Plugin{}).Where("is_active = ?", true).Count(&totalActivePlugins).Error; err != nil {
		log.Error().Err(err).Msg("Failed to count total active plugins")
	} else {
		log.Info().Int64("total_active_plugins", totalActivePlugins).Msg("Total active plugins in database")
	}
	
	// Test with namespace filter but WITHOUT is_active check (like filters do)
	var pluginsWithoutActiveCheck []models.Plugin
	testQuery := s.db.Model(&models.Plugin{})
	if namespace == "" {
		testQuery = testQuery.Where("namespace = ''")
	} else {
		testQuery = testQuery.Where("(namespace = '' OR namespace = ?)", namespace)
	}
	if err := testQuery.Find(&pluginsWithoutActiveCheck).Error; err != nil {
		log.Error().Err(err).Msg("Failed to query plugins without is_active check")
	} else {
		log.Info().Int("plugins_without_active_check", len(pluginsWithoutActiveCheck)).Msg("Plugins found WITHOUT is_active filter")
	}
	
	// Now test with namespace filter AND is_active check  
	pluginQuery = s.db.Model(&models.Plugin{})
	if namespace == "" {
		log.Info().Msg("Querying plugins for global namespace only")
		pluginQuery = pluginQuery.Where("namespace = '' AND is_active = ?", true)
	} else {
		log.Info().
			Str("target_namespace", namespace).
			Str("expected_plugin_namespace", "tenant-a").
			Bool("namespaces_match", namespace == "tenant-a").
			Msg("Querying plugins for specific namespace (global + tenant)")
		pluginQuery = pluginQuery.Where("(namespace = '' OR namespace = ?) AND is_active = ?", namespace, true)
		
		// Additional debug: Test the exact plugin we expect
		var specificPlugin models.Plugin
		if err := s.db.Where("id = ? AND namespace = ? AND is_active = ?", 2, namespace, true).First(&specificPlugin).Error; err != nil {
			log.Error().Err(err).Uint("plugin_id", 2).Str("namespace", namespace).Msg("Failed to find specific plugin by ID, namespace, and active status")
		} else {
			log.Info().
				Uint("plugin_id", specificPlugin.ID).
				Str("plugin_name", specificPlugin.Name).
				Str("plugin_namespace", specificPlugin.Namespace).
				Bool("plugin_active", specificPlugin.IsActive).
				Msg("Successfully found specific plugin by ID")
		}
	}
	
	// Debug: Log the actual SQL query being executed
	pluginQuery = pluginQuery.Debug()
	
	if err := pluginQuery.Find(&plugins).Error; err != nil {
		return nil, fmt.Errorf("failed to get Plugins: %w", err)
	}
	
	log.Info().
		Str("namespace", namespace).
		Int("found_plugins", len(plugins)).
		Int64("total_active", totalActivePlugins).
		Msg("Plugin query completed")

	// Convert Plugins to protobuf
	for _, plugin := range plugins {
		// Get associated LLM IDs for this plugin (ordered by order_index for consistent execution order)
		var llmPlugins []models.LLMPlugin
		if err := s.db.Where("plugin_id = ? AND is_active = ?", plugin.ID, true).Order("order_index ASC").Find(&llmPlugins).Error; err != nil {
			log.Warn().Err(err).Uint("plugin_id", plugin.ID).Msg("Failed to query llm_plugins join table")
		}
		
		llmIDs := make([]uint32, len(llmPlugins))
		for i, lp := range llmPlugins {
			llmIDs[i] = uint32(lp.LLMID)
		}

		log.Debug().
			Uint("plugin_id", plugin.ID).
			Str("plugin_name", plugin.Name).
			Str("hook_type", plugin.HookType).
			Int("llm_count", len(llmIDs)).
			Msg("Plugin relationships embedded in sync")
		
		// Convert config to JSON string
		var configJSON string
		if plugin.Config != nil {
			if configBytes, err := json.Marshal(plugin.Config); err == nil {
				configJSON = string(configBytes)
			}
		}
		
		pbPlugin := &pb.PluginConfig{
			Id:          uint32(plugin.ID),
			Name:        plugin.Name,
			Slug:        plugin.Slug,
			Description: plugin.Description,
			Command:     plugin.Command,
			Checksum:    plugin.Checksum,
			Config:      configJSON,
			HookType:    plugin.HookType,
			IsActive:    plugin.IsActive,
			Namespace:   plugin.Namespace,
			LlmIds:      llmIDs,
			CreatedAt:   timestamppb.New(plugin.CreatedAt),
			UpdatedAt:   timestamppb.New(plugin.UpdatedAt),
		}
		snapshot.Plugins = append(snapshot.Plugins, pbPlugin)
	}

	log.Info().
		Str("namespace", namespace).
		Int("llm_count", len(snapshot.Llms)).
		Int("app_count", len(snapshot.Apps)).
		Int("filter_count", len(snapshot.Filters)).
		Int("price_count", len(snapshot.ModelPrices)).
		Int("plugin_count", len(snapshot.Plugins)).
		Msg("Generated configuration snapshot for edge")

	return snapshot, nil
}


// encryptForMicrogateway encrypts a plaintext string using microgateway's expected AES-GCM format
func (s *ControlServer) encryptForMicrogateway(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}

	// Get microgateway encryption key from environment
	encryptionKey := os.Getenv("MICROGATEWAY_ENCRYPTION_KEY")
	if encryptionKey == "" {
		// Fallback to sending plaintext if no encryption key is configured
		log.Warn().Msg("MICROGATEWAY_ENCRYPTION_KEY not set, sending plaintext API key")
		return plaintext, nil
	}

	if len(encryptionKey) != 32 {
		log.Error().Int("key_length", len(encryptionKey)).Msg("MICROGATEWAY_ENCRYPTION_KEY must be exactly 32 bytes")
		return plaintext, nil // Fallback to plaintext
	}

	// Create AES cipher
	block, err := aes.NewCipher([]byte(encryptionKey))
	if err != nil {
		return "", fmt.Errorf("failed to create AES cipher: %w", err)
	}

	// Create GCM mode
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	// Create a random nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Encrypt the plaintext
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)

	// Encode to base64 for transmission
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// SetReloadCoordinator sets the reload coordinator reference (avoids import cycle)
func (s *ControlServer) SetReloadCoordinator(coordinator interface{}) {
	s.reloadCoordinator = coordinator
	log.Info().Msg("Reload coordinator set for control server")
}

// SendReloadRequest sends a reload request to a specific edge instance
func (s *ControlServer) SendReloadRequest(edgeID string, reloadReq *pb.ConfigurationReloadRequest) error {
	s.edgeMutex.RLock()
	edge, exists := s.edgeConnections[edgeID]
	s.edgeMutex.RUnlock()

	if !exists {
		return fmt.Errorf("edge instance not found: %s", edgeID)
	}

	if edge.Stream == nil || (edge.Status != "connected" && edge.Status != "registered") {
		return fmt.Errorf("edge instance not available for reload: %s (status: %s, has_stream: %v)", edgeID, edge.Status, edge.Stream != nil)
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

// GetConnectedEdges returns all connected edge instances as interface{} map
func (s *ControlServer) GetConnectedEdges() map[string]interface{} {
	s.edgeMutex.RLock()
	defer s.edgeMutex.RUnlock()
	
	result := make(map[string]interface{})
	for edgeID, edge := range s.edgeConnections {
		// Only include edges that are truly connected (have active stream)
		if s.isEdgeStreamActive(edge) {
			// Convert edge connection to interface{} map format expected by reload coordinator
			result[edgeID] = map[string]interface{}{
				"edge_id":        edge.EdgeID,
				"namespace":      edge.Namespace,
				"status":         edge.Status,
				"version":        edge.Version,
				"session_id":     edge.SessionID,
				"last_heartbeat": edge.LastHeartbeat,
			}
		}
	}
	
	log.Debug().
		Int("connected_edges", len(result)).
		Msg("Retrieved connected edges for reload coordinator")
	
	return result
}

// isEdgeStreamActive checks if an edge's stream is still active
func (s *ControlServer) isEdgeStreamActive(edge *EdgeInstanceConnection) bool {
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
	for edgeID, edge := range s.edgeConnections {
		if !s.isEdgeStreamActive(edge) {
			log.Info().
				Str("edge_id", edgeID).
				Str("status", edge.Status).
				Time("last_heartbeat", edge.LastHeartbeat).
				Msg("Removing stale edge connection")

			// Update database status
			var edgeInstance models.EdgeInstance
			if err := edgeInstance.GetByEdgeID(s.db, edgeID); err == nil {
				edgeInstance.UpdateStatus(s.db, models.EdgeStatusDisconnected)
			}

			toRemove = append(toRemove, edgeID)
		}
	}

	for _, edgeID := range toRemove {
		delete(s.edgeConnections, edgeID)
	}

	if len(toRemove) > 0 {
		log.Info().Int("removed_count", len(toRemove)).Msg("Cleaned up stale edge connections")
	}
}