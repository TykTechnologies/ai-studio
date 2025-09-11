// internal/grpc/server.go
package grpc

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	pb "github.com/TykTechnologies/midsommar/microgateway/proto"
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
	
	// gRPC server
	grpcServer *grpc.Server
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
	
	return server
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
	
	if err := s.db.Create(dbEdge).Error; err != nil {
		log.Error().Err(err).Str("edge_id", req.EdgeId).Msg("Failed to store edge instance")
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

// getConfigurationSnapshot creates a configuration snapshot for a specific namespace
func (s *ControlServer) getConfigurationSnapshot(namespace string) (*pb.ConfigurationSnapshot, error) {
	snapshot := &pb.ConfigurationSnapshot{
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
	
	// Convert LLMs to protobuf
	snapshot.Llms = make([]*pb.LLMConfig, len(llms))
	for i, llm := range llms {
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
		}
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
	
	// Convert Apps to protobuf
	snapshot.Apps = make([]*pb.AppConfig, len(apps))
	for i, app := range apps {
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
		}
	}
	
	log.Info().
		Str("namespace", namespace).
		Int("llm_count", len(snapshot.Llms)).
		Int("app_count", len(snapshot.Apps)).
		Msg("Created configuration snapshot")
	
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