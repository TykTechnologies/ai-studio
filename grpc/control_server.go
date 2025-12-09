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
	"github.com/TykTechnologies/midsommar/v2/pkg/config"
	"github.com/TykTechnologies/midsommar/v2/pkg/eventbridge"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/TykTechnologies/midsommar/v2/secrets"
	"github.com/TykTechnologies/midsommar/v2/services/edge_management"
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

// DEFAULT_ENCRYPTION_KEY is used when MICROGATEWAY_ENCRYPTION_KEY is not set
// CRITICAL SECURITY WARNING: This MUST be changed in production!
const DEFAULT_ENCRYPTION_KEY = "DEFAULT_INSECURE_KEY_CHANGE_ME!!"

// EdgePayloadRouter interface for routing edge payloads to plugins
type EdgePayloadRouter interface {
	RouteEdgePayload(ctx context.Context, payload *pb.PluginControlPayload) error
}

// EdgeInstance represents an active edge instance connection
type EdgeInstanceConnection struct {
	EdgeID        string
	Namespace     string
	Status        string
	Version       string
	SessionID     string
	Stream        pb.ConfigurationSyncService_SubscribeToChangesServer
	LastHeartbeat time.Time

	// Event bridge components for this connection
	streamAdapter *eventbridge.StreamAdapter
	eventBridge   *eventbridge.Bridge
	bridgeCtx     context.Context
	bridgeCancel  context.CancelFunc

	// Mutex to protect concurrent access to EdgeInstanceConnection fields
	mu sync.RWMutex
}

// ControlServer implements the ConfigurationSyncService for AI Studio control instances
type ControlServer struct {
	pb.UnimplementedConfigurationSyncServiceServer

	db     *gorm.DB
	config *Config

	// Edge instance management
	edgeConnections      map[string]*EdgeInstanceConnection
	edgeMutex            sync.RWMutex
	maxConcurrentStreams int // Maximum number of concurrent gRPC streams

	// Edge management service (CE: forces "default", ENT: multi-tenant)
	edgeManagementService edge_management.Service

	// gRPC server
	grpcServer *grpc.Server

	// Cleanup ticker for stale connections
	cleanupTicker *time.Ticker

	// Reload coordination (set after creation to avoid import cycle)
	reloadCoordinator interface{} // Will be *services.ReloadCoordinator

	// Plugin manager for routing edge payloads to plugins
	pluginManager EdgePayloadRouter

	// Event bridge: local event bus for control node
	eventBus eventbridge.Bus
}

// Config holds the control server configuration
type Config struct {
	GRPCPort             int
	GRPCHost             string
	TLSEnabled           bool
	TLSCertPath          string
	TLSKeyPath           string
	AuthToken            string
	NextAuthToken        string
	MaxConcurrentStreams int // Maximum number of concurrent gRPC streams (default 1000)
}

// NewControlServer creates a new control server for AI Studio
func NewControlServer(cfg *Config, db *gorm.DB) *ControlServer {
	// Set default connection limit if not specified
	maxStreams := cfg.MaxConcurrentStreams
	if maxStreams <= 0 {
		maxStreams = 1000 // Sensible default
	}

	// Security check: REQUIRE MICROGATEWAY_ENCRYPTION_KEY to be configured
	encryptionKey := os.Getenv("MICROGATEWAY_ENCRYPTION_KEY")
	if encryptionKey == "" {
		log.Fatal().
			Msg("🚨 CRITICAL SECURITY ERROR: MICROGATEWAY_ENCRYPTION_KEY environment variable is required but not set! Please set a secure 32-character random key.")
	}
	if len(encryptionKey) != 32 {
		log.Fatal().
			Int("current_length", len(encryptionKey)).
			Int("required_length", 32).
			Msg("🚨 CRITICAL SECURITY ERROR: MICROGATEWAY_ENCRYPTION_KEY must be exactly 32 characters long!")
	}
	if encryptionKey == DEFAULT_ENCRYPTION_KEY {
		log.Fatal().
			Msg("🚨 CRITICAL SECURITY ERROR: MICROGATEWAY_ENCRYPTION_KEY cannot use the default insecure key! Please generate a secure 32-character random key.")
	}

	log.Info().Msg("🔒 MICROGATEWAY_ENCRYPTION_KEY configured correctly")

	server := &ControlServer{
		config:                cfg,
		db:                    db,
		edgeConnections:       make(map[string]*EdgeInstanceConnection),
		maxConcurrentStreams:  maxStreams,
		edgeManagementService: edge_management.NewService(db),
		eventBus:              eventbridge.NewBus(),
	}

	// Initialize AI Studio's analytics system for processing edge pulse data
	ctx := context.Background()
	analytics.StartRecording(ctx, db)
	log.Debug().Msg("AI Studio analytics system initialized for control server")

	log.Debug().Msg("Event bridge bus initialized for control server")

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
	// Normalize namespace through edge management service
	// CE: Always returns "default" (silent enforcement)
	// ENT: Returns requested namespace or "default" if empty
	namespace := s.edgeManagementService.GetNamespaceForEdge(req.EdgeNamespace)

	log.Debug().
		Str("edge_id", req.EdgeId).
		Str("requested_namespace", req.EdgeNamespace).
		Str("assigned_namespace", namespace).
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
			Namespace: namespace, // Use normalized namespace
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
		edgeInstance.Namespace = namespace // Update to normalized namespace (CE: forces "default")
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

	// Get initial configuration using normalized namespace
	initialConfig, err := s.getConfigurationSnapshot(namespace)
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
	// Check concurrent stream limits to prevent DoS attacks
	s.edgeMutex.RLock()
	currentConnections := len(s.edgeConnections)
	s.edgeMutex.RUnlock()

	if currentConnections >= s.maxConcurrentStreams {
		log.Warn().
			Int("current_connections", currentConnections).
			Int("max_concurrent_streams", s.maxConcurrentStreams).
			Msg("🚨 SECURITY: Maximum concurrent streams exceeded - rejecting new connection")
		return status.Error(codes.ResourceExhausted, "maximum concurrent streams exceeded")
	}

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
					// Normalize namespace (CE: forces "default", ENT: accepts as-is)
					normalizedNamespace := s.edgeManagementService.GetNamespaceForEdge(m.Registration.EdgeNamespace)

					// Create bridge context for this connection
					bridgeCtx, bridgeCancel := context.WithCancel(context.Background())

					// Create stream adapter for event bridge
					streamAdapter := eventbridge.NewStreamAdapter(func(frame *eventbridge.EventFrame) error {
						log.Debug().
							Str("edge_id", edgeID).
							Str("event_id", frame.ID).
							Str("topic", frame.Topic).
							Str("origin", frame.Origin).
							Int32("direction", frame.Dir).
							Int("payload_len", len(frame.Payload)).
							Msg("Control sending event to edge via stream")

						err := stream.Send(&pb.ControlMessage{
							Message: &pb.ControlMessage_Event{
								Event: &pb.EventFrame{
									Id:      frame.ID,
									Topic:   frame.Topic,
									Origin:  frame.Origin,
									Dir:     frame.Dir,
									Payload: frame.Payload,
								},
							},
						})
						if err != nil {
							log.Error().
								Err(err).
								Str("edge_id", edgeID).
								Str("event_id", frame.ID).
								Msg("Control failed to send event to edge")
						} else {
							log.Debug().
								Str("edge_id", edgeID).
								Str("event_id", frame.ID).
								Str("topic", frame.Topic).
								Msg("Control successfully sent event to edge")
						}
						return err
					}, 100)

					// Create and start event bridge for this edge connection
					bridge := eventbridge.NewBridge(eventbridge.BridgeConfig{
						NodeID:    "control",
						IsControl: true,
					}, s.eventBus, streamAdapter)
					bridge.Start(bridgeCtx)

					s.edgeMutex.Lock()
					edgeConnection = &EdgeInstanceConnection{
						EdgeID:        m.Registration.EdgeId,
						Namespace:     normalizedNamespace, // Use normalized namespace for in-memory connection
						Status:        "connected",
						Version:       m.Registration.Version,
						Stream:        stream,
						LastHeartbeat: time.Now(),
						streamAdapter: streamAdapter,
						eventBridge:   bridge,
						bridgeCtx:     bridgeCtx,
						bridgeCancel:  bridgeCancel,
					}
					s.edgeConnections[edgeID] = edgeConnection
					s.edgeMutex.Unlock()

					log.Debug().Str("edge_id", edgeID).Msg("Event bridge started for edge connection")

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
					edgeConnection.mu.Lock()
					edgeConnection.LastHeartbeat = time.Now()
					edgeConnection.mu.Unlock()

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
						if coordinator, ok := s.reloadCoordinator.(interface {
							ProcessReloadResponse(*pb.ConfigurationReloadResponse)
						}); ok {
							coordinator.ProcessReloadResponse(m.ReloadResponse)
						}
					}
				}

			case *pb.EdgeMessage_Event:
				// Handle event bridge message from edge
				if m.Event != nil && edgeConnection != nil && edgeConnection.streamAdapter != nil {
					log.Trace().
						Str("event_id", m.Event.Id).
						Str("topic", m.Event.Topic).
						Str("origin", m.Event.Origin).
						Str("edge_id", edgeID).
						Msg("Received event from edge")

					// Enqueue the event for the bridge to process
					edgeConnection.streamAdapter.EnqueueProtoEvent(m.Event)
				}
			}
		}
	}()

	// Keep connection alive and handle outgoing messages
	<-stream.Context().Done()

	// Cleanup when stream closes
	if edgeConnection != nil {
		// Stop event bridge for this connection
		if edgeConnection.bridgeCancel != nil {
			edgeConnection.bridgeCancel()
		}
		if edgeConnection.eventBridge != nil {
			edgeConnection.eventBridge.Stop()
		}
		if edgeConnection.streamAdapter != nil {
			edgeConnection.streamAdapter.Close()
		}

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

		log.Debug().Str("edge_id", edgeID).Msg("Event bridge stopped for edge connection")
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

	// Update heartbeat with thread safety
	edge.mu.Lock()
	edge.LastHeartbeat = time.Now()
	edge.mu.Unlock()

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

// GetEventBus returns the control server's event bus for subscribing to events.
// This allows other AI Studio components to subscribe to events from edges.
func (s *ControlServer) GetEventBus() eventbridge.Bus {
	return s.eventBus
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

	log.Debug().
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
		log.Debug().Str("token_prefix", tokenPrefix).Uint("credential_id", credential.ID).Msg("AI Studio control server: app not found or inactive")
		return &pb.TokenValidationResponse{
			Valid:        false,
			ErrorMessage: "Associated app not found or inactive",
		}, nil
	}

	log.Debug().
		Str("token_prefix", tokenPrefix).
		Uint("app_id", app.ID).
		Str("app_name", app.Name).
		Msg("AI Studio control server: token validation successful")

	return &pb.TokenValidationResponse{
		Valid:     true,
		AppId:     uint32(app.ID),
		AppName:   app.Name,
		UserId:    uint32(app.UserID), // Owner user ID for analytics tracking
		Scopes:    []string{},         // AI Studio doesn't use scopes like microgateway
		ExpiresAt: nil,                // AI Studio credentials don't expire
	}, nil
}

// SendAnalyticsPulse handles analytics pulse data from edge instances
func (s *ControlServer) SendAnalyticsPulse(ctx context.Context, req *pb.AnalyticsPulse) (*pb.AnalyticsPulseResponse, error) {
	// Performance monitoring: track total processing time
	startTime := time.Now()

	log.Debug().
		Str("edge_id", req.EdgeId).
		Str("edge_namespace", req.EdgeNamespace).
		Uint64("sequence_number", req.SequenceNumber).
		Uint32("total_records", req.TotalRecords).
		Int("analytics_events", len(req.AnalyticsEvents)).
		Int("budget_events", len(req.BudgetEvents)).
		Int("proxy_summaries", len(req.ProxySummaries)).
		Msg("AI Studio control server: received analytics pulse from edge")

	processedRecords := uint64(0)

	// Process analytics events using AI Studio's native analytics system with batch processing
	if len(req.AnalyticsEvents) > 0 {
		proxyLogs := make([]*models.ProxyLog, len(req.AnalyticsEvents))
		chatRecords := make([]*models.LLMChatRecord, len(req.AnalyticsEvents))

		for i, event := range req.AnalyticsEvents {
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
			proxyLogs[i] = &models.ProxyLog{
				AppID:        uint(event.AppId),
				UserID:       uint(event.UserId), // User ID synced from edge (via config sync)
				Vendor:       vendor,
				RequestBody:  event.RequestBody,  // Now included from pulse if configured
				ResponseBody: event.ResponseBody, // Now included from pulse if configured
				ResponseCode: int(event.StatusCode),
				TimeStamp:    event.Timestamp.AsTime(),
			}

			// Create LLMChatRecord for analytics (tokens, cost, usage tracking)
			chatRecords[i] = &models.LLMChatRecord{
				LLMID:                  uint(event.LlmId),
				AppID:                  uint(event.AppId),
				Name:                   modelName, // Use actual model name from request
				Vendor:                 vendor,
				TotalTokens:            int(event.TotalTokens),
				PromptTokens:           int(event.RequestTokens),
				ResponseTokens:         int(event.ResponseTokens),
				CacheWritePromptTokens: int(event.CacheWritePromptTokens),
				CacheReadPromptTokens:  int(event.CacheReadPromptTokens),
				Cost:                   event.Cost, // Already in AI Studio format (dollars * 10000)
				Currency:               "USD",
				TimeStamp:              event.Timestamp.AsTime(),
				InteractionType:        models.ProxyInteraction, // Mark as proxy interaction
				UserID:                 uint(event.UserId),      // User ID synced from edge (via config sync)
				ChatID:                 "",                      // Not applicable for proxy
				Choices:                1,                       // Default
				ToolCalls:              0,                       // Default for proxy
			}

			log.Debug().
				Str("edge_id", req.EdgeId).
				Str("request_id", event.RequestId).
				Str("model", modelName).
				Int("total_tokens", int(event.TotalTokens)).
				Float64("cost", event.Cost).
				Bool("has_request_body", len(event.RequestBody) > 0).
				Bool("has_response_body", len(event.ResponseBody) > 0).
				Msg("Analytics event processed for batch")
		}

		// Use batch processing for improved performance
		analytics.RecordProxyLogsBatch(proxyLogs)
		analytics.RecordChatRecordsBatch(chatRecords)
		processedRecords += uint64(len(req.AnalyticsEvents))

		log.Debug().
			Str("edge_id", req.EdgeId).
			Int("analytics_events", len(req.AnalyticsEvents)).
			Msg("Analytics events processed via batch operations")
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

	// Performance monitoring: calculate total processing time
	totalProcessingTime := time.Since(startTime)

	log.Debug().
		Str("edge_id", req.EdgeId).
		Uint64("sequence_number", req.SequenceNumber).
		Uint64("processed_records", processedRecords).
		Int64("total_processing_time_ms", totalProcessingTime.Milliseconds()).
		Float64("records_per_second", float64(processedRecords)/totalProcessingTime.Seconds()).
		Msg("Analytics pulse processed via AI Studio native analytics system with batch processing")

	return &pb.AnalyticsPulseResponse{
		Success:          true,
		Message:          "Analytics pulse processed successfully",
		ProcessedRecords: processedRecords,
		SequenceNumber:   req.SequenceNumber,
		ProcessedAt:      timestamppb.Now(),
		// UpdatedConfig: nil, // No config updates for now
	}, nil
}

// SendPluginControlBatch handles plugin control payloads from edge instances
// This enables plugins running on edge (microgateway) to send data back to AI Studio control plane
func (s *ControlServer) SendPluginControlBatch(ctx context.Context, req *pb.PluginControlBatch) (*pb.PluginControlBatchResponse, error) {
	startTime := time.Now()

	log.Debug().
		Str("edge_id", req.EdgeId).
		Str("edge_namespace", req.EdgeNamespace).
		Uint64("sequence_number", req.SequenceNumber).
		Uint32("total_payloads", req.TotalPayloads).
		Int("payloads_count", len(req.Payloads)).
		Msg("AI Studio control server: received plugin control batch from edge")

	var processedCount uint64
	var errors []*pb.PluginPayloadError

	// Process each payload - route to corresponding plugin
	for _, payload := range req.Payloads {
		err := s.routeEdgePayloadToPlugin(ctx, payload)
		if err != nil {
			log.Warn().
				Err(err).
				Uint32("plugin_id", payload.PluginId).
				Str("correlation_id", payload.CorrelationId).
				Msg("Failed to route edge payload to plugin")

			errors = append(errors, &pb.PluginPayloadError{
				PluginId:      payload.PluginId,
				CorrelationId: payload.CorrelationId,
				ErrorMessage:  err.Error(),
			})
		} else {
			processedCount++
		}
	}

	totalProcessingTime := time.Since(startTime)

	log.Debug().
		Str("edge_id", req.EdgeId).
		Uint64("sequence_number", req.SequenceNumber).
		Uint64("processed_count", processedCount).
		Int("error_count", len(errors)).
		Int64("processing_time_ms", totalProcessingTime.Milliseconds()).
		Msg("Plugin control batch processed")

	return &pb.PluginControlBatchResponse{
		Success:        len(errors) == 0,
		Message:        fmt.Sprintf("Processed %d/%d payloads", processedCount, len(req.Payloads)),
		ProcessedCount: processedCount,
		SequenceNumber: req.SequenceNumber,
		ProcessedAt:    timestamppb.Now(),
		Errors:         errors,
	}, nil
}

// routeEdgePayloadToPlugin routes an edge payload to the corresponding AI Studio plugin
func (s *ControlServer) routeEdgePayloadToPlugin(ctx context.Context, payload *pb.PluginControlPayload) error {
	// Check if plugin manager is available (set after server creation)
	if s.pluginManager == nil {
		return fmt.Errorf("plugin manager not available")
	}

	// Route to plugin manager which will handle AcceptEdgePayload call
	return s.pluginManager.RouteEdgePayload(ctx, payload)
}

// SetPluginManager sets the plugin manager reference for routing edge payloads
func (s *ControlServer) SetPluginManager(manager interface{}) {
	if pm, ok := manager.(EdgePayloadRouter); ok {
		s.pluginManager = pm
		log.Debug().Msg("Plugin manager set for edge payload routing")
	} else {
		log.Warn().Msg("Plugin manager does not implement EdgePayloadRouter interface")
	}
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
	// SECURITY: Fail-closed design - reject connections if no auth tokens configured
	if s.config.AuthToken == "" && s.config.NextAuthToken == "" {
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
		Version:      fmt.Sprintf("%d", time.Now().Unix()),
		Llms:         []*pb.LLMConfig{},
		Apps:         []*pb.AppConfig{},
		ModelPrices:  []*pb.ModelPriceConfig{},
		Filters:      []*pb.FilterConfig{},
		Plugins:      []*pb.PluginConfig{},
		ModelRouters: []*pb.ModelRouterConfig{},
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

		// Serialize metadata to JSON string
		var metadataJSON string
		if llm.Metadata != nil {
			if metadataBytes, err := json.Marshal(llm.Metadata); err == nil {
				metadataJSON = string(metadataBytes)
			}
		}

		pbLLM := &pb.LLMConfig{
			Id:              uint32(llm.ID),
			Name:            llm.Name,
			Slug:            slug,
			Vendor:          string(llm.Vendor),
			Endpoint:        resolvedEndpoint,
			ApiKeyEncrypted: encryptedAPIKey, // Encrypted using microgateway's format
			DefaultModel:    llm.DefaultModel,
			MaxTokens:       4096, // Default value
			TimeoutSeconds:  30,   // Default value
			RetryCount:      3,    // Default value
			IsActive:        llm.Active,
			MonthlyBudget:   monthlyBudget,
			RateLimitRpm:    0, // AI Studio doesn't have this field yet
			Metadata:        metadataJSON,
			Namespace:       llm.Namespace,
			FilterIds:       filterIDs,
			CreatedAt:       timestamppb.New(llm.CreatedAt),
			UpdatedAt:       timestamppb.New(llm.UpdatedAt),
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

		// Serialize metadata to JSON string
		var metadataJSON string
		if app.Metadata != nil {
			if metadataBytes, err := json.Marshal(app.Metadata); err == nil {
				metadataJSON = string(metadataBytes)
			}
		}

		// Format budget start date if available
		budgetStartDate := ""
		if app.BudgetStartDate != nil {
			budgetStartDate = app.BudgetStartDate.Format(time.RFC3339)
		}

		pbApp := &pb.AppConfig{
			Id:              uint32(app.ID),
			Name:            app.Name,
			Description:     app.Description,
			OwnerEmail:      "", // AI Studio doesn't have owner email field yet
			IsActive:        app.IsActive,
			MonthlyBudget:   monthlyBudget,
			BudgetStartDate: budgetStartDate,
			Metadata:        metadataJSON,
			Namespace:       app.Namespace,
			UserId:          uint32(app.UserID), // Owner user ID for analytics tracking
			LlmIds:          llmIDs,
			CreatedAt:       timestamppb.New(app.CreatedAt),
			UpdatedAt:       timestamppb.New(app.UpdatedAt),
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

	// Query llm_filters join table to get LLM associations for each filter
	var llmFilterAssociations []struct {
		FilterID uint
		LLMID    uint
	}
	llmFilterQuery := s.db.Table("llm_filters").
		Select("llm_filters.filter_id, llm_filters.llm_id").
		Joins("JOIN llms ON llms.id = llm_filters.llm_id").
		Where("llms.active = ?", true)

	if namespace == "" {
		llmFilterQuery = llmFilterQuery.Where("llms.namespace = ''")
	} else {
		llmFilterQuery = llmFilterQuery.Where("(llms.namespace = '' OR llms.namespace = ?)", namespace)
	}

	if err := llmFilterQuery.Find(&llmFilterAssociations).Error; err != nil {
		log.Warn().Err(err).Msg("Failed to query llm_filters associations for filters")
	}

	// Build map of filter_id -> []llm_id for efficient lookup
	filterLLMMap := make(map[uint][]uint32)
	for _, assoc := range llmFilterAssociations {
		filterLLMMap[assoc.FilterID] = append(filterLLMMap[assoc.FilterID], uint32(assoc.LLMID))
	}

	// Convert Filters to protobuf
	for _, filter := range filters {
		llmIDs := filterLLMMap[filter.ID]
		pbFilter := &pb.FilterConfig{
			Id:             uint32(filter.ID),
			Name:           filter.Name,
			Description:    filter.Description,
			Script:         string(filter.Script),
			ResponseFilter: filter.ResponseFilter,
			IsActive:       true, // AI Studio Filter model doesn't have IsActive field yet
			OrderIndex:     0,    // AI Studio doesn't have OrderIndex field yet
			Namespace:      filter.Namespace,
			LlmIds:         llmIDs, // Populated from llm_filters join table
			CreatedAt:      timestamppb.New(filter.CreatedAt),
			UpdatedAt:      timestamppb.New(filter.UpdatedAt),
		}
		snapshot.Filters = append(snapshot.Filters, pbFilter)

		log.Debug().
			Uint("filter_id", filter.ID).
			Str("filter_name", filter.Name).
			Int("llm_count", len(llmIDs)).
			Msg("Filter synced with LLM associations")
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

	// Get Plugins for namespace with preloaded LLM associations to avoid N+1 queries
	log.Debug().Str("namespace", namespace).Msg("Starting plugin query for configuration snapshot")

	var plugins []models.Plugin
	var pluginQuery *gorm.DB

	pluginQuery = s.db.Model(&models.Plugin{})
	if namespace == "" {
		log.Debug().Msg("Querying plugins for global namespace only")
		pluginQuery = pluginQuery.Where("namespace = '' AND is_active = ?", true)
	} else {
		log.Debug().
			Str("target_namespace", namespace).
			Msg("Querying plugins for specific namespace (global + tenant)")
		pluginQuery = pluginQuery.Where("(namespace = '' OR namespace = ?) AND is_active = ?", namespace, true)
	}

	if err := pluginQuery.Find(&plugins).Error; err != nil {
		return nil, fmt.Errorf("failed to get Plugins: %w", err)
	}

	// Preload all LLMPlugin associations in a single query to avoid N+1
	var pluginIDs []uint
	for _, plugin := range plugins {
		pluginIDs = append(pluginIDs, plugin.ID)
	}

	var allLLMPlugins []models.LLMPlugin
	if len(pluginIDs) > 0 {
		if err := s.db.Where("plugin_id IN ? AND is_active = ?", pluginIDs, true).
			Order("plugin_id ASC, order_index ASC").
			Find(&allLLMPlugins).Error; err != nil {
			log.Warn().Err(err).Msg("Failed to preload LLM plugin associations")
		}
	}

	// Create a map of plugin_id -> []LLMPlugin for fast lookup
	llmPluginMap := make(map[uint][]models.LLMPlugin)
	for _, lp := range allLLMPlugins {
		llmPluginMap[lp.PluginID] = append(llmPluginMap[lp.PluginID], lp)
	}

	log.Debug().
		Str("namespace", namespace).
		Int("found_plugins", len(plugins)).
		Msg("Plugin query completed")

	// Convert Plugins to protobuf with merged configurations for each LLM
	for _, plugin := range plugins {
		// Use preloaded LLM associations to avoid N+1 queries
		llmPlugins := llmPluginMap[plugin.ID]
		// Data is already sorted by order_index from the query

		log.Debug().
			Uint("plugin_id", plugin.ID).
			Str("plugin_name", plugin.Name).
			Str("hook_type", plugin.HookType).
			Int("llm_count", len(llmPlugins)).
			Msg("Plugin relationships embedded in sync")

		// If plugin has LLM-specific configurations, create one PluginConfig per LLM association
		// with merged configuration (base + override)
		if len(llmPlugins) > 0 {
			for _, llmPlugin := range llmPlugins {
				// Merge base plugin config with LLM-specific override
				merged, err := config.MergePluginConfigMaps(plugin.Config, llmPlugin.ConfigOverride)
				if err != nil {
					log.Error().Err(err).
						Uint("plugin_id", plugin.ID).
						Uint("llm_id", llmPlugin.LLMID).
						Msg("Failed to merge plugin config, using base config")
					merged = plugin.Config
				}

				// Convert merged config to JSON string
				var mergedConfigJSON string
				if merged != nil {
					if configBytes, err := json.Marshal(merged); err == nil {
						mergedConfigJSON = string(configBytes)
					}
				}

				log.Debug().
					Uint("plugin_id", plugin.ID).
					Str("plugin_name", plugin.Name).
					Uint("llm_id", llmPlugin.LLMID).
					Bool("has_override", len(llmPlugin.ConfigOverride) > 0).
					Str("hook_type", plugin.HookType).
					Strs("hook_types", plugin.HookTypes).
					Int("hook_types_count", len(plugin.HookTypes)).
					Msg("Syncing plugin to edge with hook types")

				pbPlugin := &pb.PluginConfig{
					Id:            uint32(plugin.ID),
					Name:          plugin.Name,
					Description:   plugin.Description,
					Command:       plugin.Command,
					Checksum:      plugin.Checksum,
					Config:        mergedConfigJSON, // Merged configuration for this LLM
					HookType:      plugin.HookType,
					HookTypes:     plugin.HookTypes, // NEW: All hook types for hybrid plugins
					IsActive:      plugin.IsActive,
					Namespace:     plugin.Namespace,
					LlmIds:        []uint32{uint32(llmPlugin.LLMID)}, // Only for this specific LLM
					ServiceScopes: plugin.ServiceScopes,              // Service API scopes
					CreatedAt:     timestamppb.New(plugin.CreatedAt),
					UpdatedAt:     timestamppb.New(plugin.UpdatedAt),
				}
				snapshot.Plugins = append(snapshot.Plugins, pbPlugin)
			}
		} else {
			// Plugin has no LLM associations, use base config only
			log.Debug().
				Uint("plugin_id", plugin.ID).
				Str("plugin_name", plugin.Name).
				Str("hook_type", plugin.HookType).
				Strs("hook_types", plugin.HookTypes).
				Int("hook_types_count", len(plugin.HookTypes)).
				Msg("Syncing plugin to edge (no LLM associations)")

			var configJSON string
			if plugin.Config != nil {
				if configBytes, err := json.Marshal(plugin.Config); err == nil {
					configJSON = string(configBytes)
				}
			}

			pbPlugin := &pb.PluginConfig{
				Id:            uint32(plugin.ID),
				Name:          plugin.Name,
				Description:   plugin.Description,
				Command:       plugin.Command,
				Checksum:      plugin.Checksum,
				Config:        configJSON,
				HookType:      plugin.HookType,
				HookTypes:     plugin.HookTypes, // NEW: All hook types for hybrid plugins
				IsActive:      plugin.IsActive,
				Namespace:     plugin.Namespace,
				LlmIds:        []uint32{},           // No LLM associations
				ServiceScopes: plugin.ServiceScopes, // Service API scopes
				CreatedAt:     timestamppb.New(plugin.CreatedAt),
				UpdatedAt:     timestamppb.New(plugin.UpdatedAt),
			}
			snapshot.Plugins = append(snapshot.Plugins, pbPlugin)
		}
	}

	// Get Model Routers for namespace (Enterprise feature)
	var modelRouters []models.ModelRouter
	routerQuery := s.db.Preload("Pools.Vendors.LLM").Preload("Pools.Mappings").Where("active = ?", true)
	if namespace == "" {
		routerQuery = routerQuery.Where("namespace = ''")
	} else {
		routerQuery = routerQuery.Where("(namespace = '' OR namespace = ?)", namespace)
	}

	if err := routerQuery.Find(&modelRouters).Error; err != nil {
		log.Warn().Err(err).Msg("Failed to get Model Routers (Enterprise feature may not be enabled)")
		// Don't fail - model routers are optional Enterprise feature
	}

	// Convert Model Routers to protobuf
	for _, router := range modelRouters {
		pbRouter := &pb.ModelRouterConfig{
			Id:          uint32(router.ID),
			Name:        router.Name,
			Slug:        router.Slug,
			Description: router.Description,
			ApiCompat:   router.APICompat,
			IsActive:    router.Active,
			Namespace:   router.Namespace,
			CreatedAt:   timestamppb.New(router.CreatedAt),
			UpdatedAt:   timestamppb.New(router.UpdatedAt),
		}

		// Convert pools
		for _, pool := range router.Pools {
			pbPool := &pb.ModelPoolConfig{
				Id:                 uint32(pool.ID),
				Name:               pool.Name,
				ModelPattern:       pool.ModelPattern,
				SelectionAlgorithm: string(pool.SelectionAlgorithm),
				Priority:           int32(pool.Priority),
			}

			// Convert vendors
			for _, vendor := range pool.Vendors {
				llmSlug := ""
				if vendor.LLM != nil {
					llmSlug = strings.ToLower(strings.ReplaceAll(vendor.LLM.Name, " ", "-"))
				}
				pbVendor := &pb.PoolVendorConfig{
					Id:       uint32(vendor.ID),
					LlmId:    uint32(vendor.LLMID),
					LlmSlug:  llmSlug,
					Weight:   int32(vendor.Weight),
					IsActive: vendor.Active,
				}
				pbPool.Vendors = append(pbPool.Vendors, pbVendor)
			}

			// Convert mappings
			for _, mapping := range pool.Mappings {
				pbMapping := &pb.ModelMappingConfig{
					Id:          uint32(mapping.ID),
					SourceModel: mapping.SourceModel,
					TargetModel: mapping.TargetModel,
				}
				pbPool.Mappings = append(pbPool.Mappings, pbMapping)
			}

			pbRouter.Pools = append(pbRouter.Pools, pbPool)
		}

		snapshot.ModelRouters = append(snapshot.ModelRouters, pbRouter)

		log.Debug().
			Uint("router_id", router.ID).
			Str("router_slug", router.Slug).
			Int("pool_count", len(router.Pools)).
			Msg("Model Router synced to snapshot")
	}

	log.Debug().
		Str("namespace", namespace).
		Int("llm_count", len(snapshot.Llms)).
		Int("app_count", len(snapshot.Apps)).
		Int("filter_count", len(snapshot.Filters)).
		Int("price_count", len(snapshot.ModelPrices)).
		Int("plugin_count", len(snapshot.Plugins)).
		Int("model_router_count", len(snapshot.ModelRouters)).
		Msg("Generated configuration snapshot for edge")

	return snapshot, nil
}

// encryptForMicrogateway encrypts a plaintext string using microgateway's expected AES-GCM format
func (s *ControlServer) encryptForMicrogateway(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}

	// Get microgateway encryption key from environment - this MUST be set at startup
	encryptionKey := os.Getenv("MICROGATEWAY_ENCRYPTION_KEY")
	if encryptionKey == "" {
		return "", fmt.Errorf("MICROGATEWAY_ENCRYPTION_KEY environment variable is required but not set")
	}

	if len(encryptionKey) != 32 {
		return "", fmt.Errorf("MICROGATEWAY_ENCRYPTION_KEY must be exactly 32 characters long, got %d", len(encryptionKey))
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
	log.Debug().Msg("Reload coordinator set for control server")
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
			edge.mu.RLock()
			lastHeartbeat := edge.LastHeartbeat
			edge.mu.RUnlock()

			// Convert edge connection to interface{} map format expected by reload coordinator
			result[edgeID] = map[string]interface{}{
				"edge_id":        edge.EdgeID,
				"namespace":      edge.Namespace,
				"status":         edge.Status,
				"version":        edge.Version,
				"session_id":     edge.SessionID,
				"last_heartbeat": lastHeartbeat,
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
	edge.mu.RLock()
	heartbeatAge := time.Since(edge.LastHeartbeat)
	edge.mu.RUnlock()

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
	log.Debug().Msg("Started edge connection cleanup routine")
}

// cleanupStaleConnections removes disconnected and stale edge connections
func (s *ControlServer) cleanupStaleConnections() {
	s.edgeMutex.Lock()
	defer s.edgeMutex.Unlock()

	var toRemove []string
	for edgeID, edge := range s.edgeConnections {
		if !s.isEdgeStreamActive(edge) {
			edge.mu.RLock()
			lastHeartbeat := edge.LastHeartbeat
			edge.mu.RUnlock()

			log.Info().
				Str("edge_id", edgeID).
				Str("status", edge.Status).
				Time("last_heartbeat", lastHeartbeat).
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
