// grpc/control_server.go
package grpc

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/TykTechnologies/midsommar/v2/analytics"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/config"
	"github.com/TykTechnologies/midsommar/v2/pkg/eventbridge"
	"github.com/TykTechnologies/midsommar/v2/pkg/safe"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/TykTechnologies/midsommar/v2/services/pushes"
	"github.com/TykTechnologies/midsommar/v2/guardrails"
	"github.com/TykTechnologies/midsommar/v2/secrets"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/services/edge_management"
	"github.com/TykTechnologies/midsommar/v2/services/governed_metadata"
	"github.com/google/uuid"
	"github.com/gosimple/slug"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
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

	// openedAt is when the stream registered; heartbeatSeen is set by its
	// first heartbeat (under mu). Together they say whether the edge is
	// ready for pushes (see pushReady).
	openedAt      time.Time
	heartbeatSeen bool

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

	// Governed metadata reader (Enterprise); nil means no governed metadata in snapshots.
	governedMetadata governed_metadata.SnapshotReader

	// Edge instance management
	edgeConnections      map[string]*EdgeInstanceConnection
	edgeMutex            sync.RWMutex
	maxConcurrentStreams int // Maximum number of concurrent gRPC streams

	// Edge management service (CE: forces "default", ENT: multi-tenant)
	edgeManagementService edge_management.Service

	// gRPC server, set by Serve and read by Stop
	serverMu   sync.Mutex
	grpcServer *grpc.Server

	// Cleanup ticker for stale connections
	cleanupTicker *time.Ticker

	// stopping is closed by Stop: every edge stream handler returns, so
	// the edges reconnect (to another replica) and pushes in flight on
	// them are requeued at once.
	stopping     chan struct{}
	stoppingOnce sync.Once

	// pushes delivers configuration pushes to the edges whose streams this
	// replica holds (services/pushes; set after creation).
	pushes PushDelivery
	// pushReadyGrace: a stream gets pushes once it has sent a heartbeat, or
	// once it has been open this long (see pushReady).
	pushReadyGrace time.Duration

	// Plugin manager for routing edge payloads to plugins
	pluginManager EdgePayloadRouter

	// Event bridge: local event bus for control node
	eventBus eventbridge.Bus

	// Budget sync service for multi-edge budget synchronization
	budgetSyncService *BudgetSyncService

	// encryptionKey is the validated key credentials are encrypted with for edges.
	encryptionKey string

	// nodeID is this replica's cluster node ID, recorded as the owner of the
	// edge streams it holds.
	nodeID string
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
	// EncryptionKey is the 32-character key edges use to decrypt the
	// credentials sent to them. Empty means MICROGATEWAY_ENCRYPTION_KEY.
	EncryptionKey string
	// NodeID identifies this Studio replica (pkg/cluster). It is recorded as
	// the owner of every edge stream this server holds. Empty means
	// "control", for a single replica.
	NodeID string
	// BudgetSyncInterval is how often budget usage is synced to edges.
	// Zero means BUDGET_SYNC_INTERVAL, or 30s.
	BudgetSyncInterval time.Duration

	// MaxMessageSize bounds a message in either direction, in bytes (zero:
	// 16 MB, the edge's own GRPC_MAX_MESSAGE_SIZE default). Configuration
	// snapshots and analytics pulses outgrow gRPC's 4 MB default.
	MaxMessageSize int
	// KeepaliveMinTime is the shortest interval between client pings the
	// server accepts, with or without an open stream (zero: 10s; edges ping
	// every 30s). A client pinging more often is disconnected.
	KeepaliveMinTime time.Duration
	// MaxConnectionAge, when set, closes each connection after about this
	// long, so edges spread again over replicas behind a load balancer;
	// streams get MaxConnectionAgeGrace to finish. Zero keeps connections
	// open: an edge's stream is long-lived, and pushes in flight on a closed
	// stream go back to pending.
	MaxConnectionAge      time.Duration
	MaxConnectionAgeGrace time.Duration
}

// Keepalive and size defaults for the control server.
const (
	defaultMaxMessageSize   = 16 * 1024 * 1024
	defaultKeepaliveMinTime = 10 * time.Second
	// The server pings a connection idle this long and drops it when the
	// ping is not answered within the timeout (a half-open edge).
	defaultKeepaliveTime    = 30 * time.Second
	defaultKeepaliveTimeout = 5 * time.Second
)

// serverTuning is Config's transport settings with the defaults applied.
type serverTuning struct {
	maxMessageSize                          int
	keepaliveMinTime                        time.Duration
	keepaliveTime, keepaliveTimeout         time.Duration
	maxConnectionAge, maxConnectionAgeGrace time.Duration
}

func (c *Config) serverTuning() serverTuning {
	t := serverTuning{
		maxMessageSize:        c.MaxMessageSize,
		keepaliveMinTime:      c.KeepaliveMinTime,
		keepaliveTime:         defaultKeepaliveTime,
		keepaliveTimeout:      defaultKeepaliveTimeout,
		maxConnectionAge:      c.MaxConnectionAge,
		maxConnectionAgeGrace: c.MaxConnectionAgeGrace,
	}
	if t.maxMessageSize <= 0 {
		t.maxMessageSize = defaultMaxMessageSize
	}
	if t.keepaliveMinTime <= 0 {
		t.keepaliveMinTime = defaultKeepaliveMinTime
	}
	return t
}

// serverOptions are the transport options every control server listener
// gets: keepalive that admits the edges' pings and message sizes that fit a
// full snapshot. (MaxConcurrentStreams limits edge streams across all
// connections, in SubscribeToChanges, not HTTP/2 streams per connection.)
func (t serverTuning) serverOptions() []grpc.ServerOption {
	params := keepalive.ServerParameters{Time: t.keepaliveTime, Timeout: t.keepaliveTimeout}
	if t.maxConnectionAge > 0 {
		params.MaxConnectionAge = t.maxConnectionAge
		params.MaxConnectionAgeGrace = t.maxConnectionAgeGrace
	}
	return []grpc.ServerOption{
		grpc.KeepaliveParams(params),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             t.keepaliveMinTime,
			PermitWithoutStream: true,
		}),
		grpc.MaxRecvMsgSize(t.maxMessageSize),
		grpc.MaxSendMsgSize(t.maxMessageSize),
	}
}

// validateEncryptionKey checks the key edges use to decrypt the credentials
// the control server sends them.
func validateEncryptionKey(key string) error {
	switch {
	case key == "":
		return errors.New("MICROGATEWAY_ENCRYPTION_KEY is required but not set; set a secure 32-character random key")
	case len(key) != 32:
		return fmt.Errorf("MICROGATEWAY_ENCRYPTION_KEY must be exactly 32 characters long, got %d", len(key))
	case key == DEFAULT_ENCRYPTION_KEY:
		return errors.New("MICROGATEWAY_ENCRYPTION_KEY cannot use the default insecure key; generate a secure 32-character random key")
	}
	return nil
}

// NewControlServer creates a new control server for AI Studio. It fails when
// the microgateway encryption key is missing or insecure.
func NewControlServer(cfg *Config, db *gorm.DB) (*ControlServer, error) {
	// Set default connection limit if not specified
	maxStreams := cfg.MaxConcurrentStreams
	if maxStreams <= 0 {
		maxStreams = 1000 // Sensible default
	}

	encryptionKey := cfg.EncryptionKey
	if encryptionKey == "" {
		encryptionKey = os.Getenv("MICROGATEWAY_ENCRYPTION_KEY")
	}
	if err := validateEncryptionKey(encryptionKey); err != nil {
		return nil, err
	}

	log.Info().Msg("🔒 MICROGATEWAY_ENCRYPTION_KEY configured correctly")

	server := &ControlServer{
		config:                cfg,
		db:                    db,
		edgeConnections:       make(map[string]*EdgeInstanceConnection),
		stopping:              make(chan struct{}),
		maxConcurrentStreams:  maxStreams,
		edgeManagementService: edge_management.NewService(db),
		eventBus:              eventbridge.NewBus(),
		encryptionKey:         encryptionKey,
		nodeID:                cfg.NodeID,
		pushReadyGrace:        defaultPushReadyGrace,
	}
	if server.nodeID == "" {
		server.nodeID = "control"
	}

	// Edge analytics pulses are recorded through the process-wide analytics
	// handler, which the host starts (pkg/studio does, after its migrations,
	// with a context it cancels on Stop). Starting it here used to create
	// analytics tables outside the migration lock.

	log.Debug().Msg("Event bridge bus initialized for control server")

	// Subscribe to config change events to trigger checksum recomputation
	server.subscribeToConfigChanges()

	// Initialize budget sync service for multi-edge budget synchronization
	server.budgetSyncService = NewBudgetSyncService(db, server.eventBus)
	if cfg.BudgetSyncInterval > 0 {
		server.budgetSyncService.syncInterval = cfg.BudgetSyncInterval
	}
	server.budgetSyncService.Start()
	log.Debug().Msg("Budget sync service initialized for control server")

	// Start cleanup routine
	server.startCleanupRoutine()

	return server, nil
}

// SetEdgeBudgetSource has the budget sync push budget blocks to edges and
// raise budget alerts for edge spend (Enterprise).
func (s *ControlServer) SetEdgeBudgetSource(src EdgeBudgetSource) {
	if s.budgetSyncService != nil {
		s.budgetSyncService.SetEdgeBudgetSource(src)
	}
}

// Start listens on the configured host and port and serves the gRPC control
// server until Stop is called.
func (s *ControlServer) Start() error {
	addr := fmt.Sprintf("%s:%d", s.config.GRPCHost, s.config.GRPCPort)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}
	return s.Serve(listener)
}

// Serve serves the gRPC control server on listener until Stop is called. An
// embedding host uses it to supply its own listener.
func (s *ControlServer) Serve(listener net.Listener) error {
	// Setup gRPC server options
	opts := s.config.serverTuning().serverOptions()

	// Add TLS if enabled
	if s.config.TLSEnabled {
		creds, err := credentials.NewServerTLSFromFile(
			s.config.TLSCertPath,
			s.config.TLSKeyPath,
		)
		if err != nil {
			listener.Close()
			return fmt.Errorf("failed to load TLS credentials: %w", err)
		}
		opts = append(opts, grpc.Creds(creds))
	}

	// Panic recovery, then authentication
	opts = append(opts, s.interceptors()...)

	// Create gRPC server
	server := grpc.NewServer(opts...)
	pb.RegisterConfigurationSyncServiceServer(server, s)
	s.serverMu.Lock()
	s.grpcServer = server
	s.serverMu.Unlock()

	log.Info().Str("address", listener.Addr().String()).Msg("Starting AI Studio gRPC control server")

	// Start serving
	if err := server.Serve(listener); err != nil {
		return fmt.Errorf("gRPC server failed: %w", err)
	}

	return nil
}

// Stop stops the gRPC server gracefully
func (s *ControlServer) Stop() {
	log.Info().Msg("Stopping AI Studio gRPC control server")

	// Stop budget sync service
	if s.budgetSyncService != nil {
		s.budgetSyncService.Stop()
	}

	// Stop cleanup routine
	if s.cleanupTicker != nil {
		s.cleanupTicker.Stop()
	}

	// End the edge streams first: GracefulStop waits for every stream to
	// end, and an edge's stream only ends when this side or the edge ends
	// it. The edges reconnect with backoff, to another replica if there is
	// one. (No ShutdownRequested notice: the edge client stops for good on
	// it, which is wrong when only this replica is going away.)
	s.stoppingOnce.Do(func() { close(s.stopping) })

	s.serverMu.Lock()
	server := s.grpcServer
	s.serverMu.Unlock()
	if server != nil {
		done := make(chan struct{})
		go func() {
			server.GracefulStop()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(stopGracePeriod):
			log.Warn().Dur("grace", stopGracePeriod).Msg("gRPC control server did not stop gracefully in time; closing remaining connections")
			server.Stop()
			<-done
		}
	}
}

// stopGracePeriod bounds how long Stop waits for in-flight unary calls.
var stopGracePeriod = 10 * time.Second

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

		// Omit the stream ownership columns: a stream on another replica may
		// have claimed the edge since the row was read.
		if err := s.db.Omit("OwnerNodeID", "StreamSessionID").Save(&edgeInstance).Error; err != nil {
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

	// Update edge sync status - since the edge is receiving the latest config at registration,
	// it should be marked as in_sync to avoid false "pending" status before first heartbeat
	if initialConfig.Checksum != "" {
		now := time.Now()
		edgeInstance.SyncStatus = models.EdgeSyncStatusInSync
		edgeInstance.LoadedChecksum = initialConfig.Checksum
		edgeInstance.LoadedVersion = initialConfig.Version
		edgeInstance.LastSyncAck = &now
		if err := edgeInstance.UpdateSyncStatus(s.db, initialConfig.Checksum, initialConfig.Version, models.EdgeSyncStatusInSync); err != nil {
			log.Error().Err(err).Str("edge_id", req.EdgeId).Msg("Failed to update edge sync status on registration")
		} else {
			log.Debug().
				Str("edge_id", req.EdgeId).
				Str("checksum", initialConfig.Checksum).
				Str("version", initialConfig.Version).
				Msg("Set edge sync status to in_sync on registration")
		}
	}

	return &pb.EdgeRegistrationResponse{
		Success:       true,
		Message:       "Edge registered successfully with AI Studio",
		SessionId:     sessionID,
		InitialConfig: initialConfig,
	}, nil
}

// GetFullConfiguration retrieves a complete configuration snapshot for an edge.
// This is the path a reload uses (microgateway RequestFullSync). The namespace
// is resolved the way registration resolved it -- the registered edge's stored
// namespace when the edge is known, else the normalised request namespace --
// so an edge whose EDGE_NAMESPACE is unset ("") pulls the same snapshot its
// heartbeat is compared against. The pulling edge is marked in sync at once,
// as the stream ConfigRequest path does, rather than waiting for a heartbeat.
func (s *ControlServer) GetFullConfiguration(ctx context.Context, req *pb.ConfigurationRequest) (*pb.ConfigurationSnapshot, error) {
	namespace := s.edgeManagementService.GetNamespaceForEdge(req.EdgeNamespace)
	edgeKnown := false
	if req.EdgeId != "" {
		var edgeInstance models.EdgeInstance
		if err := edgeInstance.GetByEdgeID(s.db, req.EdgeId); err == nil {
			namespace = edgeInstance.Namespace
			edgeKnown = true
		}
	}

	log.Debug().
		Str("edge_id", req.EdgeId).
		Str("requested_namespace", req.EdgeNamespace).
		Str("namespace", namespace).
		Msg("AI Studio control server: full configuration request")

	snapshot, err := s.getConfigurationSnapshot(namespace)
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to get configuration: %v", err))
	}

	// After the snapshot: getConfigurationSnapshot may have marked every edge
	// in the namespace pending (checksum changed); this edge now holds it.
	if edgeKnown {
		s.markEdgeInSync(req.EdgeId, snapshot)
	}

	return snapshot, nil
}

// markEdgeInSync records that the edge has just been handed the snapshot:
// sync status in_sync, loaded checksum/version and the ack time. Shared by
// the unary GetFullConfiguration and the stream ConfigRequest paths.
func (s *ControlServer) markEdgeInSync(edgeID string, snapshot *pb.ConfigurationSnapshot) {
	if snapshot == nil || snapshot.Checksum == "" {
		return
	}
	var edgeInstance models.EdgeInstance
	if err := edgeInstance.GetByEdgeID(s.db, edgeID); err != nil {
		log.Debug().Err(err).Str("edge_id", edgeID).Msg("Edge not found; sync status not updated after config delivery")
		return
	}
	if err := edgeInstance.UpdateSyncStatus(s.db, snapshot.Checksum, snapshot.Version, models.EdgeSyncStatusInSync); err != nil {
		log.Warn().Err(err).Str("edge_id", edgeID).Msg("Failed to update edge sync status after config delivery")
		return
	}
	log.Debug().
		Str("edge_id", edgeID).
		Str("checksum", snapshot.Checksum).
		Msg("Updated edge sync status to in_sync after config delivery")
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

	// Heartbeat responses, events, config snapshots and pushes are sent on
	// this stream from different goroutines; gRPC allows one Send at a time.
	stream = &serialSendStream{ConfigurationSyncService_SubscribeToChangesServer: stream}

	var edgeID string
	var edgeConnection *EdgeInstanceConnection

	// Handle incoming messages from edge. A panic handling one ends this
	// edge's stream (the edge reconnects and its pushes are retried), not
	// the process.
	recvPanicked := make(chan struct{})
	go func() {
		defer safe.RecoverWith("edge stream receive", func() { close(recvPanicked) })
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

					// Every stream gets a session of its own: writes that end
					// it are conditional on it (see models.ReleaseEdgeStream).
					streamSession := uuid.New().String()

					s.edgeMutex.Lock()
					edgeConnection = &EdgeInstanceConnection{
						EdgeID:        m.Registration.EdgeId,
						Namespace:     normalizedNamespace, // Use normalized namespace for in-memory connection
						Status:        "connected",
						Version:       m.Registration.Version,
						SessionID:     streamSession,
						Stream:        stream,
						LastHeartbeat: time.Now(),
						openedAt:      time.Now(),
						streamAdapter: streamAdapter,
						eventBridge:   bridge,
						bridgeCtx:     bridgeCtx,
						bridgeCancel:  bridgeCancel,
					}
					s.edgeConnections[edgeID] = edgeConnection
					s.edgeMutex.Unlock()

					if found, err := models.ClaimEdgeStream(s.db, edgeID, s.nodeID, streamSession); err != nil {
						log.Error().Err(err).Str("edge_id", edgeID).Msg("Failed to record this replica as the edge's stream owner; pushes to it may not be routed here until its next heartbeat")
					} else if !found {
						log.Warn().Str("edge_id", edgeID).Msg("Edge opened a stream without being registered; it cannot receive pushes until it registers")
					}

					log.Debug().Str("edge_id", edgeID).Str("stream_session", streamSession).Msg("Event bridge started for edge connection")

					// Deliver any push waiting for this edge.
					if p := s.pushDelivery(); p != nil {
						p.StreamOpened(edgeID)
					}

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
					firstHeartbeat := !edgeConnection.heartbeatSeen
					edgeConnection.heartbeatSeen = true
					edgeConnection.mu.Unlock()

					// Get loaded config info from heartbeat
					loadedChecksum := m.Heartbeat.LoadedConfigChecksum
					loadedVersion := m.Heartbeat.LoadedConfigVersion

					// Update database with heartbeat and sync tracking
					var edgeInstance models.EdgeInstance
					var expectedChecksum string
					var isInSync bool
					var requestFullSync bool

					if err := edgeInstance.GetByEdgeID(s.db, edgeID); err == nil {
						if reclaimed, err := models.TouchEdgeStream(s.db, edgeID, s.nodeID, edgeConnection.SessionID); err != nil {
							log.Warn().Err(err).Str("edge_id", edgeID).Msg("Failed to record edge heartbeat")
						} else if reclaimed {
							log.Info().Str("edge_id", edgeID).Msg("Edge stream ownership restored from its heartbeat")
						}

						// Check sync status against namespace expected checksum
						// The checksum is updated via config change events, so we use the cached value
						var syncStatus models.NamespaceSyncStatus
						if err := syncStatus.GetByNamespace(s.db, edgeInstance.Namespace); err == nil {
							expectedChecksum = syncStatus.ExpectedChecksum
							isInSync = loadedChecksum != "" && loadedChecksum == expectedChecksum

							// Update edge sync status
							syncStatusValue := models.EdgeSyncStatusPending
							if isInSync {
								syncStatusValue = models.EdgeSyncStatusInSync
							} else if loadedChecksum == "" {
								syncStatusValue = models.EdgeSyncStatusUnknown
							}
							edgeInstance.UpdateSyncStatus(s.db, loadedChecksum, loadedVersion, syncStatusValue)

							// Log sync status change for audit if out of sync
							if !isInSync && loadedChecksum != "" {
								auditLog := &models.SyncAuditLog{
									EventType:     models.SyncEventEdgeOutOfSync,
									Namespace:     edgeInstance.Namespace,
									EdgeID:        &edgeID,
									Checksum:      loadedChecksum,
									ConfigVersion: loadedVersion,
									Details:       fmt.Sprintf("Edge out of sync. Expected: %s, Got: %s", expectedChecksum, loadedChecksum),
								}
								auditLog.Create(s.db)

								// NOTE: We intentionally do NOT set requestFullSync = true here.
								// Config pushes are manual-only per design requirement.
								// Users must explicitly push config changes via the UI or API.
							}
						}
					}

					// Send heartbeat response with sync info
					response := &pb.ControlMessage{
						Message: &pb.ControlMessage_HeartbeatResponse{
							HeartbeatResponse: &pb.HeartbeatResponse{
								Acknowledged:     true,
								Message:          "Heartbeat received by AI Studio",
								ExpectedChecksum: expectedChecksum,
								IsInSync:         isInSync,
								RequestFullSync:  requestFullSync,
							},
						},
					}
					stream.Send(response)

					// The edge is ready for pushes now; deliver any waiting.
					if firstHeartbeat {
						if p := s.pushDelivery(); p != nil {
							p.StreamOpened(edgeID)
						}
					}
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

						// Update edge sync status immediately since it's receiving the latest config
						s.markEdgeInSync(edgeID, snapshot)
					}
				}

			case *pb.EdgeMessage_ReloadResponse:
				// Handle reload status response
				if m.ReloadResponse != nil {
					log.Info().
						Str("operation_id", m.ReloadResponse.OperationId).
						Str("edge_id", edgeID).
						Str("phase", m.ReloadResponse.Phase.String()).
						Bool("success", m.ReloadResponse.Success).
						Msg("Received reload status from edge")

					if edgeConnection == nil {
						log.Warn().Str("operation_id", m.ReloadResponse.OperationId).Msg("Reload status on a stream that has not registered; ignored")
					} else if p := s.pushDelivery(); p != nil {
						// The stream says which edge this is, not the message.
						resp := proto.Clone(m.ReloadResponse).(*pb.ConfigurationReloadResponse)
						resp.EdgeId = edgeID
						p.HandleReloadResponse(resp)
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
	var streamErr error
	select {
	case <-stream.Context().Done():
	case <-s.stopping:
		log.Debug().Str("edge_id", edgeID).Msg("Control server stopping; ending edge stream")
	case <-recvPanicked:
		log.Error().Str("edge_id", edgeID).Msg("Ending the edge's stream after a panic handling its messages; the edge reconnects")
		streamErr = errRecovered
	}

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

		// Mark the edge disconnected only if this is still its current
		// stream: it may already have reconnected, here or to another replica.
		if changed, err := models.ReleaseEdgeStream(s.db, edgeID, edgeConnection.SessionID); err != nil {
			log.Warn().Err(err).Str("edge_id", edgeID).Msg("Failed to record the end of the edge's stream")
		} else if !changed {
			log.Debug().Str("edge_id", edgeID).Msg("Edge already on a newer stream; its state is left as is")
		}

		// Pushes sent on this stream and not yet answered are retried on
		// the edge's next stream, on whichever replica it reaches.
		if p := s.pushDelivery(); p != nil {
			p.StreamClosed(edgeID, edgeConnection.SessionID)
		}

		log.Debug().Str("edge_id", edgeID).Msg("Event bridge stopped for edge connection")
	}

	log.Debug().Str("edge_id", edgeID).Msg("Edge stream closed")
	return streamErr
}

// SendHeartbeat handles unary heartbeat requests, a deprecated RPC: edges
// heartbeat on their stream (SubscribeToChanges), which also tracks stream
// ownership. The unary call is answered from the
// database, so it works on any replica, not only the one holding the edge's
// stream; it records the heartbeat and leaves ownership alone.
func (s *ControlServer) SendHeartbeat(ctx context.Context, req *pb.HeartbeatRequest) (*pb.HeartbeatResponse, error) {
	var edgeInstance models.EdgeInstance
	if err := edgeInstance.GetByEdgeID(s.db.WithContext(ctx), req.EdgeId); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, status.Error(codes.NotFound, "edge instance not found")
		}
		return nil, status.Error(codes.Unavailable, "failed to look up edge instance")
	}
	if err := edgeInstance.UpdateHeartbeat(s.db.WithContext(ctx)); err != nil {
		return nil, status.Error(codes.Unavailable, "failed to record heartbeat")
	}

	// This replica holds the edge's stream: keep its view current too.
	s.edgeMutex.RLock()
	edge, exists := s.edgeConnections[req.EdgeId]
	s.edgeMutex.RUnlock()
	if exists {
		edge.mu.Lock()
		edge.LastHeartbeat = time.Now()
		edge.mu.Unlock()
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

	// Get the associated app with LLM/Tool/Datasource relationships (preload for pull-on-miss sync)
	var app models.App
	if err := s.db.Where("credential_id = ?", credential.ID).
		Preload("LLMs").Preload("Tools").Preload("Datasources").Preload("ModelRouters").Preload("SemanticRouters").First(&app).Error; err != nil {
		// Only a missing or inactive App is a rejection. Any other error is
		// the hub's own fault and goes back as one: the edge then treats the
		// hub as unavailable (keeping its cache, and its stale grace if
		// configured) instead of refusing a valid credential with 401.
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			log.Error().Err(err).Str("token_prefix", tokenPrefix).Uint("credential_id", credential.ID).Msg("AI Studio control server: app lookup database error during token validation")
			return nil, status.Error(codes.Internal, "token validation failed")
		}
		log.Debug().Str("token_prefix", tokenPrefix).Uint("credential_id", credential.ID).Msg("AI Studio control server: app not found or inactive")
		return &pb.TokenValidationResponse{
			Valid:        false,
			ErrorMessage: "Associated app not found or inactive",
		}, nil
	}
	if !app.IsActive {
		// Said in so many words: the edge answers 403 "app is inactive" for
		// this, as the embedded gateway does, rather than 401 for a bad key.
		log.Debug().Str("token_prefix", tokenPrefix).Uint("app_id", app.ID).Msg("AI Studio control server: app is inactive")
		return &pb.TokenValidationResponse{
			Valid:        false,
			ErrorMessage: services.AppInactiveMessage,
		}, nil
	}

	log.Debug().
		Str("token_prefix", tokenPrefix).
		Uint("app_id", app.ID).
		Str("app_name", app.Name).
		Msg("AI Studio control server: token validation successful")

	response := &pb.TokenValidationResponse{
		Valid:     true,
		AppId:     uint32(app.ID),
		AppName:   app.Name,
		UserId:    uint32(app.UserID), // Owner user ID for analytics tracking
		Scopes:    []string{},         // AI Studio doesn't use scopes like microgateway
		ExpiresAt: nil,                // AI Studio credentials don't expire
	}

	// Check if App should be included for pull-on-miss sync (namespace match)
	if s.shouldIncludeAppInResponse(app.Namespace, req.EdgeNamespace) {
		response.App = s.convertAppToProto(&app)

		log.Debug().
			Uint("app_id", app.ID).
			Str("app_namespace", app.Namespace).
			Str("edge_namespace", req.EdgeNamespace).
			Int("llm_count", len(app.LLMs)).
			Msg("Including App in token validation response for pull-on-miss sync")
	}

	return response, nil
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
		Int("compliance_events", len(req.ComplianceEvents)).
		Int("tool_calls", len(req.ToolCalls)).
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

			// Create ProxyLog for request/response tracking.
			// LLMID must be propagated from the pulse event so that the LLM
			// detail view can isolate logs by specific LLM entry — without
			// it, edge-sourced logs land with llm_id = 0 and are invisible
			// to GetProxyLogsForLLM, which filters on llm_id directly.
			proxyLogs[i] = &models.ProxyLog{
				AppID:        uint(event.AppId),
				UserID:       uint(event.UserId), // User ID synced from edge (via config sync)
				LLMID:        uint(event.LlmId),
				Vendor:       vendor,
				ModelName:    modelName,
				RequestBody:  event.RequestBody,  // Now included from pulse if configured
				ResponseBody: event.ResponseBody, // Now included from pulse if configured
				ResponseCode: int(event.StatusCode),
				TimeStamp:    event.Timestamp.AsTime(),
			}
			// Failover marker: which primary this rung was failing over from.
			// The attempt index is copied on its own so a rung reported
			// without a from-id is still not counted as a primary request.
			if event.FailoverFromLlmId != 0 {
				from := uint(event.FailoverFromLlmId)
				proxyLogs[i].FailoverFromLLMID = &from
			}
			if event.FailoverAttempt != 0 {
				proxyLogs[i].FailoverAttempt = int(event.FailoverAttempt)
			}
			// Routing decision, when the request was addressed to a router.
			proxyLogs[i].RouterKind = event.RouterKind
			proxyLogs[i].RouterSlug = event.RouterSlug
			proxyLogs[i].RouterPool = event.RouterPool
			proxyLogs[i].Route = event.Route
			proxyLogs[i].RouteReason = event.RouteReason
			proxyLogs[i].RouteSourceModel = event.RouteSourceModel
			proxyLogs[i].RouteTargetModel = event.RouteTargetModel
			proxyLogs[i].RouteSelection = event.RouteSelection
			proxyLogs[i].RouteScore = event.RouteScore
			proxyLogs[i].ShadowRoute = event.ShadowRoute

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
				TotalTimeMS:            int(event.LatencyMs),
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
		analytics.RecordProxyLogsBatch(ctx, proxyLogs)
		analytics.RecordChatRecordsBatch(ctx, chatRecords)
		processedRecords += uint64(len(req.AnalyticsEvents))

		log.Debug().
			Str("edge_id", req.EdgeId).
			Int("analytics_events", len(req.AnalyticsEvents)).
			Msg("Analytics events processed via batch operations")
	}

	// Process compliance events from edge filter scripts
	if len(req.ComplianceEvents) > 0 {
		complianceEvents := make([]*models.ComplianceEvent, len(req.ComplianceEvents))
		for i, ce := range req.ComplianceEvents {
			complianceEvents[i] = &models.ComplianceEvent{
				AppID:       uint(ce.AppId),
				UserID:      uint(ce.UserId),
				LLMID:       uint(ce.LlmId),
				FilterName:  ce.FilterName,
				FilterScope: ce.FilterScope,
				EventType:   ce.EventType,
				Severity:    ce.Severity,
				Description: ce.Description,
				Metadata:    ce.Metadata,
				Vendor:      ce.Vendor,
				ModelName:   ce.ModelName,
				TimeStamp:   ce.Timestamp.AsTime(),
			}
		}
		analytics.RecordComplianceEvents(ctx, complianceEvents)
		processedRecords += uint64(len(req.ComplianceEvents))

		log.Debug().
			Str("edge_id", req.EdgeId).
			Int("compliance_events", len(req.ComplianceEvents)).
			Msg("Compliance events processed from edge pulse")
	}

	// Process per-operation tool calls served by the edge. Without these the
	// tool analytics endpoints only ever counted calls served by the control
	// plane's embedded gateway, so the figures under-reported by whatever
	// share of traffic a distributed deployment routes through its edges.
	for _, tc := range req.ToolCalls {
		timestamp := time.Now()
		if tc.Timestamp != nil {
			timestamp = tc.Timestamp.AsTime()
		}
		analytics.RecordToolCall(ctx, tc.OperationId, timestamp, int(tc.ExecTimeMs), uint(tc.ToolId))
		processedRecords++
	}
	if len(req.ToolCalls) > 0 {
		log.Debug().
			Str("edge_id", req.EdgeId).
			Int("tool_calls", len(req.ToolCalls)).
			Msg("Tool calls processed from edge pulse")
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
	// "", "global" and "default" are one namespace: same object set (global
	// objects plus those filed under "default"), same checksum, same sync row.
	namespace = models.CanonicalNamespace(namespace)

	// Taken before any row is read: an edge keeps Apps created after it,
	// which it learnt of from token validation (see EdgeSyncService).
	takenAt := time.Now()
	snapshot := &pb.ConfigurationSnapshot{
		Version:      fmt.Sprintf("%d", takenAt.Unix()),
		SnapshotTime: timestamppb.New(takenAt),
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

	if err := llmQuery.Order("id ASC").Find(&llms).Error; err != nil {
		return nil, fmt.Errorf("failed to get LLMs: %w", err)
	}
	// Filters run top to bottom in the arranged order; the edge stores the
	// FilterIds position as llm_filters.order_index.
	if err := models.OrderLLMFilterList(s.db, llms); err != nil {
		return nil, fmt.Errorf("failed to order LLM filters: %w", err)
	}

	governedLLMs := s.loadGovernedMetadata(models.GovernedObjectTypeLLM)

	// A filter's position in its chain; when it sits in several chains the
	// lowest position is reported on the FilterConfig (the per-LLM order is
	// what FilterIds carries).
	filterOrderIndex := map[uint]int32{}

	// Convert LLMs to protobuf with complete configuration
	for _, llm := range llms {
		// Same slug the proxy routes /llm/.../{slug}/ by; edges look the LLM
		// up by this value (post-auth plugins, model-router rewrites).
		llmSlug := slug.Make(llm.Name)

		// Get filter IDs for this LLM
		filterIDs := make([]uint32, len(llm.Filters))
		for i, filter := range llm.Filters {
			filterIDs[i] = uint32(filter.ID)
			if cur, seen := filterOrderIndex[filter.ID]; !seen || int32(i) < cur {
				filterOrderIndex[filter.ID] = int32(i)
			}
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
			// Never send the key in plaintext: leave the LLM out, as for
			// tools, datasources and tokens.
			log.Error().Err(err).Uint("llm_id", llm.ID).Msg("Failed to encrypt LLM API key - excluding LLM from snapshot")
			continue
		}

		// Resolve secret references in metadata and serialize to JSON string
		var metadataJSON string
		if llm.Metadata != nil {
			resolvedMetadata := make(models.JSONMap, len(llm.Metadata))
			for k, v := range llm.Metadata {
				if strVal, ok := v.(string); ok {
					resolvedMetadata[k] = secrets.GetValue(strVal, false)
				} else {
					resolvedMetadata[k] = v
				}
			}
			if metadataBytes, err := json.Marshal(resolvedMetadata); err == nil {
				metadataJSON = string(metadataBytes)
			}
		}

		// Serialize allowed_models to JSON string
		var allowedModelsJSON string
		if len(llm.AllowedModels) > 0 {
			if allowedModelsBytes, err := json.Marshal(llm.AllowedModels); err == nil {
				allowedModelsJSON = string(allowedModelsBytes)
			}
		}

		// Serialize the failover waterfall the same way; empty when none so the
		// edge stores NULL and the checksum only moves when a waterfall exists.
		var failoverJSON string
		if llm.Failover.Enabled() {
			if failoverBytes, err := json.Marshal(llm.Failover); err == nil {
				failoverJSON = string(failoverBytes)
			}
		}

		pbLLM := &pb.LLMConfig{
			Id:               uint32(llm.ID),
			Name:             llm.Name,
			Slug:             llmSlug,
			Vendor:           string(llm.Vendor),
			Endpoint:         resolvedEndpoint,
			ApiKeyEncrypted:  encryptedAPIKey, // Encrypted using microgateway's format
			DefaultModel:     llm.DefaultModel,
			MaxTokens:        4096, // Default value
			TimeoutSeconds:   30,   // Default value
			RetryCount:       3,    // Default value
			IsActive:         llm.Active,
			MonthlyBudget:    monthlyBudget,
			RateLimitRpm:     0, // AI Studio doesn't have this field yet
			Metadata:         metadataJSON,
			GovernedMetadata: s.governedMetadataJSON(models.GovernedObjectTypeLLM, governedLLMs[models.BuiltinObjectID(llm.ID)]),
			AllowedModels:    allowedModelsJSON,
			Failover:         failoverJSON,
			Namespace:        llm.Namespace,
			DontLogBodies:    llm.DontLogBodies,
			FilterIds:        filterIDs,
			CreatedAt:        timestamppb.New(llm.CreatedAt),
			UpdatedAt:        timestamppb.New(llm.UpdatedAt),
		}
		snapshot.Llms = append(snapshot.Llms, pbLLM)
	}

	// Get Apps for namespace with relationships
	var apps []models.App
	appQuery := s.db.Preload("LLMs").Preload("Tools").Preload("Datasources").Preload("ModelRouters").Preload("SemanticRouters").Where("is_active = ?", true)
	if namespace == "" {
		// Global namespace - only global apps
		appQuery = appQuery.Where("namespace = ''")
	} else {
		// Specific namespace - global + matching namespace
		appQuery = appQuery.Where("(namespace = '' OR namespace = ?)", namespace)
	}

	if err := appQuery.Order("id ASC").Find(&apps).Error; err != nil {
		return nil, fmt.Errorf("failed to get Apps: %w", err)
	}

	// Batch-fetch all plugin resource associations for snapshot apps (avoids N+1)
	var allAppPluginResources []models.AppPluginResource
	if len(apps) > 0 {
		appIDs := make([]uint, len(apps))
		for i, a := range apps {
			appIDs[i] = a.ID
		}
		s.db.Where("app_id IN ?", appIDs).
			Preload("PluginResourceType").
			Find(&allAppPluginResources)
	}
	// Group by app ID for O(1) lookup
	pluginResourcesByApp := make(map[uint][]models.AppPluginResource)
	for _, apr := range allAppPluginResources {
		pluginResourcesByApp[apr.AppID] = append(pluginResourcesByApp[apr.AppID], apr)
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

		// Calculate current period usage from llm_chat_records for budget sync to edge
		var currentPeriodUsage float64
		if monthlyBudget > 0 {
			// Calculate budget period using app's BudgetStartDate (handles mid-period resets)
			now := time.Now()
			periodStart, periodEnd := calculateBudgetPeriod(app.BudgetStartDate, now)

			// The budget sync's latest figure when it has one: summing the
			// whole period here, for every budgeted App on every snapshot,
			// is a scan of the App's period each time.
			var totalCostCents float64
			if usage, ok := s.budgetSyncService.PeriodUsage(app.ID, periodStart); ok {
				currentPeriodUsage = usage
			} else if err := s.db.Model(&models.LLMChatRecord{}).
				Select("COALESCE(SUM(cost), 0)").
				Where("app_id = ? AND time_stamp >= ? AND time_stamp <= ?", app.ID, periodStart, periodEnd).
				Scan(&totalCostCents).Error; err != nil {
				log.Warn().Err(err).Uint("app_id", app.ID).Msg("Failed to calculate current period usage for app")
			} else {
				// Convert from cents*10000 to dollars
				currentPeriodUsage = totalCostCents / 10000.0
			}
		}

		// Get associated Tool IDs
		toolIDs := make([]uint32, len(app.Tools))
		for i, tool := range app.Tools {
			toolIDs[i] = uint32(tool.ID)
		}

		// Get associated Datasource IDs
		datasourceIDs := make([]uint32, len(app.Datasources))
		for i, ds := range app.Datasources {
			datasourceIDs[i] = uint32(ds.ID)
		}

		pbApp := &pb.AppConfig{
			Id:                 uint32(app.ID),
			Name:               app.Name,
			Description:        app.Description,
			OwnerEmail:         "", // AI Studio doesn't have owner email field yet
			IsActive:           app.IsActive,
			MonthlyBudget:      monthlyBudget,
			BudgetStartDate:    budgetStartDate,
			Metadata:           metadataJSON,
			Namespace:          app.Namespace,
			UserId:             uint32(app.UserID), // Owner user ID for analytics tracking
			LlmIds:             llmIDs,
			ToolIds:            toolIDs,
			DatasourceIds:      datasourceIDs,
			ModelRouterIds:     appModelRouterIDs(&app),
			SemanticRouterIds:  appSemanticRouterIDs(&app),
			CurrentPeriodUsage: currentPeriodUsage, // Current spending synced to edge for budget enforcement
			CreatedAt:          timestamppb.New(app.CreatedAt),
			UpdatedAt:          timestamppb.New(app.UpdatedAt),
		}

		// Add plugin resource associations (from batch query above)
		if appPluginResources, ok := pluginResourcesByApp[app.ID]; ok && len(appPluginResources) > 0 {
			// Group by (plugin_id, resource_type_slug)
			type prKey struct {
				PluginID uint
				Slug     string
			}
			grouped := make(map[prKey]*pb.PluginResourceAssociation)
			for _, apr := range appPluginResources {
				// Only types an App credential grants access to reach the
				// gateways: informational or plugin-gated resources are never
				// checked at request time, so they have no business in the
				// snapshot. Associations to such types (from before the type
				// was classified) stay on the App but are not shipped.
				if apr.PluginResourceType == nil || !apr.PluginResourceType.AccessGrantedViaApp {
					continue
				}
				k := prKey{apr.PluginResourceType.PluginID, apr.PluginResourceType.Slug}
				if _, exists := grouped[k]; !exists {
					grouped[k] = &pb.PluginResourceAssociation{
						PluginId:         uint32(apr.PluginResourceType.PluginID),
						ResourceTypeSlug: apr.PluginResourceType.Slug,
					}
				}
				grouped[k].InstanceIds = append(grouped[k].InstanceIds, apr.InstanceID)
				grouped[k].Instances = append(grouped[k].Instances, &pb.ResourceInstanceSnapshot{
					Id:           apr.InstanceID,
					Name:         apr.InstanceName,
					PrivacyScore: int32(apr.InstancePrivacyScore),
					Metadata:     apr.InstanceMetadata,
				})
			}
			for _, pra := range grouped {
				pbApp.PluginResources = append(pbApp.PluginResources, pra)
			}
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

	if err := filterQuery.Order("id ASC").Find(&filters).Error; err != nil {
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

	if err := llmFilterQuery.Order("llm_filters.llm_id ASC, llm_filters.filter_id ASC").Find(&llmFilterAssociations).Error; err != nil {
		log.Warn().Err(err).Msg("Failed to query llm_filters associations for filters")
	}

	// Build map of filter_id -> []llm_id for efficient lookup
	filterLLMMap := make(map[uint][]uint32)
	for _, assoc := range llmFilterAssociations {
		filterLLMMap[assoc.FilterID] = append(filterLLMMap[assoc.FilterID], uint32(assoc.LLMID))
	}

	// Guardrail connection references are resolved once per distinct
	// reference for the whole snapshot, not once per filter field: GetValue
	// reads the secret store on every call.
	resolvedRefs := map[string]string{}
	resolveRef := func(v string) string {
		if !strings.HasPrefix(v, "$") {
			return v
		}
		if r, ok := resolvedRefs[v]; ok {
			return r
		}
		r := secrets.GetValue(v, false)
		resolvedRefs[v] = r
		return r
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
			OrderIndex:     filterOrderIndex[filter.ID], // Position in the LLM chain (lowest across LLMs)
			Namespace:      filter.Namespace,
			LlmIds:         llmIDs, // Populated from llm_filters join table
			CreatedAt:      timestamppb.New(filter.CreatedAt),
			UpdatedAt:      timestamppb.New(filter.UpdatedAt),
			Kind:           filter.Kind,
		}
		if filter.IsGuardrail() {
			// Edges have no secret store: resolve connection references here,
			// as the LLM API keys above are.
			configJSON, err := guardrails.ConfigJSONForEdge(filter.Config, resolveRef)
			if err != nil {
				log.Warn().Err(err).Uint("filter_id", filter.ID).Msg("Skipping guardrail filter with invalid config in snapshot")
				continue
			}
			pbFilter.Config = configJSON
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
	if err := priceQuery.Order("id ASC").Find(&modelPrices).Error; err != nil {
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

	if err := pluginQuery.Order("id ASC").Find(&plugins).Error; err != nil {
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
	routerQuery := s.db.Preload("Pools.Vendors.LLM").Preload("Pools.Vendors.Mappings").Where("active = ?", true)
	if namespace == "" {
		routerQuery = routerQuery.Where("namespace = ''")
	} else {
		routerQuery = routerQuery.Where("(namespace = '' OR namespace = ?)", namespace)
	}

	if err := routerQuery.Order("id ASC").Find(&modelRouters).Error; err != nil {
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

			// Convert vendors with their mappings
			for _, vendor := range pool.Vendors {
				llmSlug := ""
				if vendor.LLM != nil {
					llmSlug = slug.Make(vendor.LLM.Name)
				}
				pbVendor := &pb.PoolVendorConfig{
					Id:       uint32(vendor.ID),
					LlmId:    uint32(vendor.LLMID),
					LlmSlug:  llmSlug,
					Weight:   int32(vendor.Weight),
					IsActive: vendor.Active,
				}

				// Convert vendor-specific mappings
				for _, mapping := range vendor.Mappings {
					pbMapping := &pb.ModelMappingConfig{
						Id:          uint32(mapping.ID),
						SourceModel: mapping.SourceModel,
						TargetModel: mapping.TargetModel,
					}
					pbVendor.Mappings = append(pbVendor.Mappings, pbMapping)
				}

				pbPool.Vendors = append(pbPool.Vendors, pbVendor)
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

	// Semantic Routers (Enterprise). The configuration travels as JSON; the
	// edge compiles it with the engine. The router's embedder is flattened
	// in: a linked one as an LLM reference, a standalone one inline with its
	// key encrypted (fail-closed: a key that cannot be encrypted keeps the
	// router off the edge).
	var semanticRouters []models.SemanticRouter
	semanticQuery := s.db.Preload("Embedder.LLM").Where("active = ?", true)
	if namespace == "" {
		semanticQuery = semanticQuery.Where("namespace = ''")
	} else {
		semanticQuery = semanticQuery.Where("(namespace = '' OR namespace = ?)", namespace)
	}
	if err := semanticQuery.Order("id ASC").Find(&semanticRouters).Error; err != nil {
		log.Warn().Err(err).Msg("Failed to get Semantic Routers")
	}
	for _, router := range semanticRouters {
		if router.Embedder != nil {
			if err := router.Embedder.UsableInNamespace(router.Namespace); err != nil {
				log.Error().Err(err).Uint("router_id", router.ID).Msg("Semantic Router embedder not available in its namespace; not synced")
				continue
			}
		}
		cfg, err := router.EdgeConfigJSON(s.db, s.encryptForMicrogateway)
		if err != nil {
			log.Error().Err(err).Uint("router_id", router.ID).Msg("Failed to encode Semantic Router; not synced")
			continue
		}
		snapshot.SemanticRouters = append(snapshot.SemanticRouters, &pb.SemanticRouterConfig{
			Id:         uint32(router.ID),
			Name:       router.Name,
			Slug:       router.Slug,
			Namespace:  router.Namespace,
			IsActive:   router.Active,
			ConfigJson: cfg,
			CreatedAt:  timestamppb.New(router.CreatedAt),
			UpdatedAt:  timestamppb.New(router.UpdatedAt),
		})
	}

	// Get Tools for namespace with relationships
	var tools []models.Tool
	toolQuery := s.db.Preload("Filters").Where("active = ?", true)
	if namespace == "" {
		toolQuery = toolQuery.Where("namespace = ''")
	} else {
		toolQuery = toolQuery.Where("(namespace = '' OR namespace = ?)", namespace)
	}

	if err := toolQuery.Order("id ASC").Find(&tools).Error; err != nil {
		return nil, fmt.Errorf("failed to get Tools: %w", err)
	}

	// Query tool_filters join table for filter IDs
	var toolFilterAssociations []struct {
		ToolID   uint
		FilterID uint
	}
	if len(tools) > 0 {
		var toolIDs []uint
		for _, tool := range tools {
			toolIDs = append(toolIDs, tool.ID)
		}
		if err := s.db.Table("tool_filters").
			Select("tool_id, filter_id").
			Where("tool_id IN ?", toolIDs).
			Find(&toolFilterAssociations).Error; err != nil {
			log.Warn().Err(err).Msg("Failed to query tool_filters associations")
		}
	}

	// Build map of tool_id -> []filter_id
	toolFilterMap := make(map[uint][]uint32)
	for _, assoc := range toolFilterAssociations {
		toolFilterMap[assoc.ToolID] = append(toolFilterMap[assoc.ToolID], uint32(assoc.FilterID))
	}

	// Query app_tools join table for app IDs
	var appToolAssociations []struct {
		ToolID uint
		AppID  uint
	}
	if len(tools) > 0 {
		var toolIDs []uint
		for _, tool := range tools {
			toolIDs = append(toolIDs, tool.ID)
		}
		if err := s.db.Table("app_tools").
			Select("tool_id, app_id").
			Where("tool_id IN ?", toolIDs).
			Find(&appToolAssociations).Error; err != nil {
			log.Warn().Err(err).Msg("Failed to query app_tools associations")
		}
	}

	// Build map of tool_id -> []app_id
	toolAppMap := make(map[uint][]uint32)
	for _, assoc := range appToolAssociations {
		toolAppMap[assoc.ToolID] = append(toolAppMap[assoc.ToolID], uint32(assoc.AppID))
	}

	// Convert Tools to protobuf
	governedTools := s.loadGovernedMetadata(models.GovernedObjectTypeTool)
	for _, tool := range tools {
		// Resolve and encrypt auth key for edge transit (fail-closed: skip tool if encryption fails)
		resolvedAuthKey := secrets.GetValue(tool.AuthKey, false)
		encryptedAuthKey := ""
		if resolvedAuthKey != "" {
			encrypted, err := s.encryptForMicrogateway(resolvedAuthKey)
			if err != nil {
				log.Error().Err(err).Uint("tool_id", tool.ID).Msg("Failed to encrypt tool auth key - excluding tool from snapshot")
				continue
			}
			encryptedAuthKey = encrypted
		}

		// Serialize metadata to JSON string
		var metadataJSON string
		if tool.Metadata != nil {
			if metadataBytes, err := json.Marshal(tool.Metadata); err == nil {
				metadataJSON = string(metadataBytes)
			}
		}

		pbTool := &pb.ToolConfig{
			Id:                  uint32(tool.ID),
			Name:                tool.Name,
			Slug:                tool.Slug,
			Description:         tool.Description,
			ToolType:            tool.ToolType,
			OasSpec:             tool.OASSpec,
			AvailableOperations: tool.AvailableOperations,
			PrivacyScore:        int32(tool.PrivacyScore),
			AuthKeyEncrypted:    encryptedAuthKey,
			AuthSchemaName:      tool.AuthSchemaName,
			IsActive:            tool.Active,
			RestAccessDisabled:  tool.RESTAccessDisabled,
			McpAccessDisabled:   tool.MCPAccessDisabled,
			Namespace:           tool.Namespace,
			Metadata:            metadataJSON,
			GovernedMetadata:    s.governedMetadataJSON(models.GovernedObjectTypeTool, governedTools[models.BuiltinObjectID(tool.ID)]),
			FilterIds:           toolFilterMap[tool.ID],
			AppIds:              toolAppMap[tool.ID],
			CreatedAt:           timestamppb.New(tool.CreatedAt),
			UpdatedAt:           timestamppb.New(tool.UpdatedAt),
		}
		snapshot.Tools = append(snapshot.Tools, pbTool)
	}

	// Get Datasources for namespace
	var datasources []models.Datasource
	// Embedders are flattened into each datasource's embed_* fields, so the
	// edge needs no Embedder objects; a linked embedder resolves its LLM's
	// connection here.
	dsQuery := s.db.Preload("Embedder.LLM").Where("active = ?", true)
	if namespace == "" {
		dsQuery = dsQuery.Where("namespace = ''")
	} else {
		dsQuery = dsQuery.Where("(namespace = '' OR namespace = ?)", namespace)
	}

	if err := dsQuery.Order("id ASC").Find(&datasources).Error; err != nil {
		return nil, fmt.Errorf("failed to get Datasources: %w", err)
	}

	// Query app_datasources join table for app IDs
	var appDsAssociations []struct {
		DatasourceID uint
		AppID        uint
	}
	if len(datasources) > 0 {
		var dsIDs []uint
		for _, ds := range datasources {
			dsIDs = append(dsIDs, ds.ID)
		}
		if err := s.db.Table("app_datasources").
			Select("datasource_id, app_id").
			Where("datasource_id IN ?", dsIDs).
			Find(&appDsAssociations).Error; err != nil {
			log.Warn().Err(err).Msg("Failed to query app_datasources associations")
		}
	}

	// Build map of datasource_id -> []app_id
	dsAppMap := make(map[uint][]uint32)
	for _, assoc := range appDsAssociations {
		dsAppMap[assoc.DatasourceID] = append(dsAppMap[assoc.DatasourceID], uint32(assoc.AppID))
	}

	// Convert Datasources to protobuf (fail-closed: skip datasource if any secret encryption fails)
	governedDatasources := s.loadGovernedMetadata(models.GovernedObjectTypeDatasource)
	for _, ds := range datasources {
		// Resolve and encrypt secrets for edge transit
		resolvedConnString := secrets.GetValue(ds.DBConnString, false)
		encryptedConnString := ""
		if resolvedConnString != "" {
			encrypted, err := s.encryptForMicrogateway(resolvedConnString)
			if err != nil {
				log.Error().Err(err).Uint("ds_id", ds.ID).Msg("Failed to encrypt datasource connection string - excluding from snapshot")
				continue
			}
			encryptedConnString = encrypted
		}

		resolvedConnAPIKey := secrets.GetValue(ds.DBConnAPIKey, false)
		encryptedConnAPIKey := ""
		if resolvedConnAPIKey != "" {
			encrypted, err := s.encryptForMicrogateway(resolvedConnAPIKey)
			if err != nil {
				log.Error().Err(err).Uint("ds_id", ds.ID).Msg("Failed to encrypt datasource API key - excluding from snapshot")
				continue
			}
			encryptedConnAPIKey = encrypted
		}

		// An embedder linked to an LLM scoped to another namespace must not
		// carry that LLM's credentials here (the API refuses such links;
		// this covers rows saved before it did).
		embed := ds.EmbedFields(true)
		if ds.Embedder != nil {
			if err := ds.Embedder.UsableInNamespace(ds.Namespace); err != nil {
				log.Error().Err(err).Uint("ds_id", ds.ID).Msg("Datasource embedder not available in its namespace - syncing it without embedding settings")
				embed = models.LegacyEmbed{}
			}
		}
		resolvedEmbedAPIKey := embed.APIKey
		encryptedEmbedAPIKey := ""
		if resolvedEmbedAPIKey != "" {
			encrypted, err := s.encryptForMicrogateway(resolvedEmbedAPIKey)
			if err != nil {
				log.Error().Err(err).Uint("ds_id", ds.ID).Msg("Failed to encrypt embedder API key - excluding from snapshot")
				continue
			}
			encryptedEmbedAPIKey = encrypted
		}

		// Serialize metadata to JSON string
		var metadataJSON string
		if ds.Metadata != nil {
			if metadataBytes, err := json.Marshal(ds.Metadata); err == nil {
				metadataJSON = string(metadataBytes)
			}
		}

		pbDS := &pb.DatasourceConfig{
			Id:                    uint32(ds.ID),
			Name:                  ds.Name,
			ShortDescription:      ds.ShortDescription,
			LongDescription:       ds.LongDescription,
			Icon:                  ds.Icon,
			Url:                   ds.Url,
			PrivacyScore:          int32(ds.PrivacyScore),
			DbSourceType:          ds.DBSourceType,
			DbConnStringEncrypted: encryptedConnString,
			DbConnApiKeyEncrypted: encryptedConnAPIKey,
			DbName:                ds.DBName,
			EmbedVendor:           string(embed.Vendor),
			EmbedUrl:              embed.URL,
			EmbedApiKeyEncrypted:  encryptedEmbedAPIKey,
			EmbedModel:            embed.Model,
			IsActive:              ds.Active,
			Namespace:             ds.Namespace,
			Metadata:              metadataJSON,
			GovernedMetadata:      s.governedMetadataJSON(models.GovernedObjectTypeDatasource, governedDatasources[models.BuiltinObjectID(ds.ID)]),
			AppIds:                dsAppMap[ds.ID],
			CreatedAt:             timestamppb.New(ds.CreatedAt),
			UpdatedAt:             timestamppb.New(ds.UpdatedAt),
		}
		snapshot.Datasources = append(snapshot.Datasources, pbDS)
	}

	// Get OAuth Clients for MCP authentication on edges
	var oauthClients []models.OAuthClient
	if err := s.db.Order("id ASC").Find(&oauthClients).Error; err != nil {
		log.Warn().Err(err).Msg("Failed to get OAuth clients for edge sync")
		// Don't fail - OAuth is optional
	}

	for _, client := range oauthClients {
		var userID uint32
		if client.UserID != nil {
			userID = uint32(*client.UserID)
		}
		pbClient := &pb.OAuthClientConfig{
			Id:               uint32(client.ID),
			ClientId:         client.ClientID,
			ClientSecretHash: client.ClientSecret, // Already bcrypt hashed, safe to transmit
			ClientName:       client.ClientName,
			RedirectUris:     client.RedirectURIs,
			UserId:           userID,
			Scope:            client.Scope,
			CreatedAt:        timestamppb.New(client.CreatedAt),
			UpdatedAt:        timestamppb.New(client.UpdatedAt),
		}
		snapshot.OauthClients = append(snapshot.OauthClients, pbClient)
	}

	// Get non-expired Access Tokens for MCP authentication on edges
	var accessTokens []models.AccessToken
	if err := s.db.Where("expires_at > ?", time.Now()).Order("id ASC").Find(&accessTokens).Error; err != nil {
		log.Warn().Err(err).Msg("Failed to get access tokens for edge sync")
		// Don't fail - OAuth is optional
	}

	for _, token := range accessTokens {
		// Encrypt token string for secure transit (fail-closed: skip token if encryption fails)
		encryptedToken := ""
		tokenHash := ""
		if token.Token != "" {
			encrypted, err := s.encryptForMicrogateway(token.Token)
			if err != nil {
				log.Error().Err(err).Uint("token_id", token.ID).Msg("Failed to encrypt access token - excluding from snapshot")
				continue
			}
			encryptedToken = encrypted
			// Compute SHA-256 hash for O(1) lookup on edge (avoids iterating all tokens)
			h := sha256.Sum256([]byte(token.Token))
			tokenHash = fmt.Sprintf("%x", h[:])
		}

		// The app binding travels with the token: without it the edge cannot run
		// the same tool ACL the hub does, and would have to refuse the token.
		var appID uint32
		if token.AppID != nil {
			appID = uint32(*token.AppID)
		}

		pbToken := &pb.AccessTokenConfig{
			Id:             uint32(token.ID),
			TokenEncrypted: encryptedToken,
			TokenHash:      tokenHash,
			ClientId:       token.ClientID,
			UserId:         uint32(token.UserID),
			AppId:          appID,
			Scope:          token.Scope,
			ExpiresAt:      timestamppb.New(token.ExpiresAt),
			CreatedAt:      timestamppb.New(token.CreatedAt),
			UpdatedAt:      timestamppb.New(token.UpdatedAt),
		}
		snapshot.AccessTokens = append(snapshot.AccessTokens, pbToken)
	}

	log.Debug().
		Str("namespace", namespace).
		Int("llm_count", len(snapshot.Llms)).
		Int("app_count", len(snapshot.Apps)).
		Int("filter_count", len(snapshot.Filters)).
		Int("price_count", len(snapshot.ModelPrices)).
		Int("plugin_count", len(snapshot.Plugins)).
		Int("model_router_count", len(snapshot.ModelRouters)).
		Int("tool_count", len(snapshot.Tools)).
		Int("datasource_count", len(snapshot.Datasources)).
		Int("oauth_client_count", len(snapshot.OauthClients)).
		Int("access_token_count", len(snapshot.AccessTokens)).
		Msg("Generated configuration snapshot for edge")

	// Compute checksum for the snapshot
	checksum, err := ComputeSnapshotChecksum(snapshot)
	if err != nil {
		log.Error().Err(err).Str("namespace", namespace).Msg("Failed to compute snapshot checksum")
		// Don't fail the snapshot, just log the error - edge can still sync
	} else {
		snapshot.Checksum = checksum

		log.Info().
			Str("namespace", namespace).
			Str("checksum", checksum).
			Str("version", snapshot.Version).
			Msg("Computed snapshot checksum")

		// Update namespace sync status in database
		if err := s.updateNamespaceSyncStatus(namespace, checksum, snapshot.Version); err != nil {
			log.Error().Err(err).Str("namespace", namespace).Msg("Failed to update namespace sync status")
		}
	}

	return snapshot, nil
}

// updateNamespaceSyncStatus updates the sync status for a namespace in the database.
// When the recomputed checksum equals the stored one the configuration edges must
// hold has not changed, so nothing is written and no edge is marked pending. This
// keeps snapshot regeneration (edge fetches, no-op edits, governed metadata that
// is not gateway-visible) from churning sync status.
//
// A row that exists with an empty checksum was created by a push issued
// before any snapshot (models.MarkNamespacePushed); filling it in is the
// first computation, not a change, so edges are left alone and nothing is
// audited.
func (s *ControlServer) updateNamespaceSyncStatus(namespace, checksum, version string) error {
	namespace = models.CanonicalNamespace(namespace)

	var previous models.NamespaceSyncStatus
	previousErr := previous.GetByNamespace(s.db, namespace)
	if previousErr == nil && previous.ExpectedChecksum == checksum {
		log.Debug().
			Str("namespace", namespace).
			Str("checksum", checksum).
			Msg("Namespace snapshot unchanged; sync status left as is")
		return nil
	}
	firstFill := previousErr == nil && previous.ExpectedChecksum == ""

	status := &models.NamespaceSyncStatus{
		Namespace:        namespace,
		ExpectedChecksum: checksum,
		ConfigVersion:    version,
		LastConfigChange: time.Now(),
	}

	if err := status.Upsert(s.db); err != nil {
		return fmt.Errorf("failed to upsert namespace sync status: %w", err)
	}

	if firstFill {
		log.Debug().
			Str("namespace", namespace).
			Str("checksum", checksum).
			Msg("Recorded first namespace checksum on a row created by a push; edges left as is")
		return nil
	}

	// Log audit event for config change
	auditLog := &models.SyncAuditLog{
		EventType:     models.SyncEventConfigChanged,
		Namespace:     namespace,
		Checksum:      checksum,
		ConfigVersion: version,
		Details:       fmt.Sprintf("Configuration snapshot generated with checksum %s", checksum),
	}
	if err := auditLog.Create(s.db); err != nil {
		log.Warn().Err(err).Str("namespace", namespace).Msg("Failed to create sync audit log")
	}

	// Mark all active edges in this namespace as pending sync
	edgeInstance := &models.EdgeInstance{}
	if err := edgeInstance.MarkEdgesAsPendingInNamespace(s.db, namespace); err != nil {
		log.Warn().Err(err).Str("namespace", namespace).Msg("Failed to mark edges as pending sync")
	}

	log.Debug().
		Str("namespace", namespace).
		Str("checksum", checksum).
		Str("version", version).
		Msg("Updated namespace sync status")

	return nil
}

// encryptForMicrogateway encrypts a plaintext string using microgateway's expected AES-GCM format
func (s *ControlServer) encryptForMicrogateway(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}

	// The key is validated in NewControlServer; a server built without it
	// falls back to the environment.
	encryptionKey := s.encryptionKey
	if encryptionKey == "" {
		encryptionKey = os.Getenv("MICROGATEWAY_ENCRYPTION_KEY")
	}
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

	// Derive the nonce from the plaintext instead of drawing it at random.
	//
	// The snapshot checksum edges report back is computed over the encrypted
	// snapshot, so a random nonce made every regeneration hash differently:
	// edges could never be "in sync" once an LLM carried an API key, and every
	// heartbeat logged an out-of-sync audit row. A synthetic nonce keyed by
	// HMAC-SHA256 over the plaintext (with a key derived from, but distinct
	// from, the AES key) gives identical ciphertext for identical secrets while
	// keeping the GCM invariant that distinct plaintexts never share a nonce.
	// The only thing this reveals is that two objects hold the same secret,
	// which the edge (holding the key) can see anyway. The wire format is
	// unchanged: the 12-byte nonce still prefixes the ciphertext, so existing
	// edges decrypt it as before.
	nonce := deriveSnapshotNonce(encryptionKey, plaintext, gcm.NonceSize())

	// Encrypt the plaintext
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)

	// Encode to base64 for transmission
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// snapshotNonceDomain separates the nonce-derivation key from the AES key.
const snapshotNonceDomain = "midsommar:microgateway:snapshot-nonce:v1"

// deriveSnapshotNonce returns a deterministic GCM nonce for a plaintext.
// nonceKey = SHA-256(domain || encryptionKey); nonce = HMAC-SHA256(nonceKey, plaintext)[:size].
func deriveSnapshotNonce(encryptionKey, plaintext string, size int) []byte {
	nonceKey := sha256.Sum256([]byte(snapshotNonceDomain + encryptionKey))
	mac := hmac.New(sha256.New, nonceKey[:])
	mac.Write([]byte(plaintext))
	return mac.Sum(nil)[:size]
}

// PushDelivery is the durable push coordinator (services/pushes.Coordinator),
// told about this replica's edge streams and the edges' reload reports.
type PushDelivery interface {
	StreamOpened(edgeID string)
	StreamClosed(edgeID, session string)
	HandleReloadResponse(*pb.ConfigurationReloadResponse)
}

// SetPushDelivery connects the push coordinator. The coordinator in turn
// uses the server's LocalStreams and SendReload.
func (s *ControlServer) SetPushDelivery(p PushDelivery) {
	s.edgeMutex.Lock()
	s.pushes = p
	s.edgeMutex.Unlock()
}

func (s *ControlServer) pushDelivery() PushDelivery {
	s.edgeMutex.RLock()
	defer s.edgeMutex.RUnlock()
	return s.pushes
}

// defaultPushReadyGrace is how long a stream that has not sent a heartbeat
// waits before it is given pushes anyway.
const defaultPushReadyGrace = 10 * time.Second

// pushReady reports whether pushes may be sent on this stream. An edge
// opens its stream while it is still starting up; up to v2.2 (and until
// this check existed) the microgateway set its reload handler only after
// its services were up, dropped a push that arrived before then, and the
// push waited for control's answer timeout (a minute) before it was sent
// again. So a stream gets pushes after its first heartbeat, which current
// edges send as soon as the stream is open (and they hold a push that
// arrives before the handler is set), or, for older edges whose first
// heartbeat comes a full interval later, once it has been open for the
// grace period. The caller holds s.edgeMutex.
func (s *ControlServer) pushReady(edge *EdgeInstanceConnection) bool {
	edge.mu.RLock()
	defer edge.mu.RUnlock()
	return edge.heartbeatSeen || time.Since(edge.openedAt) >= s.pushReadyGrace
}

// LocalStreams lists the edges with a live stream on this replica that are
// ready for pushes (see pushReady), with the stream's session.
func (s *ControlServer) LocalStreams() map[string]string {
	s.edgeMutex.RLock()
	defer s.edgeMutex.RUnlock()
	out := make(map[string]string, len(s.edgeConnections))
	for edgeID, edge := range s.edgeConnections {
		if edge.Stream != nil && edge.Stream.Context().Err() == nil && s.pushReady(edge) {
			out[edgeID] = edge.SessionID
		}
	}
	return out
}

// SendReload sends a push on the edge's stream, provided it is still the
// stream with the given session: a push must go out on the stream it was
// claimed for, or not at all.
func (s *ControlServer) SendReload(edgeID, session string, req *pb.ConfigurationReloadRequest) error {
	s.edgeMutex.RLock()
	edge, ok := s.edgeConnections[edgeID]
	var stream pb.ConfigurationSyncService_SubscribeToChangesServer
	var current string
	if ok {
		stream, current = edge.Stream, edge.SessionID
	}
	s.edgeMutex.RUnlock()

	switch {
	case !ok || stream == nil:
		return fmt.Errorf("%w: edge %s has no stream here", pushes.ErrNoStream, edgeID)
	case current != session:
		return fmt.Errorf("%w: edge %s reconnected (stream %s replaced %s)", pushes.ErrNoStream, edgeID, current, session)
	case stream.Context().Err() != nil:
		return fmt.Errorf("%w: edge %s stream is closed (%v)", pushes.ErrNoStream, edgeID, stream.Context().Err())
	}
	if err := stream.Send(&pb.ControlMessage{Message: &pb.ControlMessage_ReloadRequest{ReloadRequest: req}}); err != nil {
		return fmt.Errorf("send on edge %s stream: %w", edgeID, err)
	}
	log.Info().Str("edge_id", edgeID).Str("operation_id", req.OperationId).Str("stream_session", session).Msg("Configuration push sent to edge")
	return nil
}

// serialSendStream lets one goroutine at a time Send on an edge stream,
// which gRPC requires. A torn message could lose a push or its answer.
type serialSendStream struct {
	pb.ConfigurationSyncService_SubscribeToChangesServer
	mu sync.Mutex
}

func (s *serialSendStream) Send(msg *pb.ControlMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ConfigurationSyncService_SubscribeToChangesServer.Send(msg)
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
	ticks := s.cleanupTicker.C
	// Ends with Stop: a stopped ticker never closes its channel.
	go safe.Loop("edge connection cleanup", s.stopping, func() {
		for {
			select {
			case <-s.stopping:
				return
			case <-ticks:
				s.cleanupStaleConnections()
			}
		}
	})
	log.Debug().Msg("Started edge connection cleanup routine")
}

// cleanupStaleConnections removes disconnected and stale edge connections
func (s *ControlServer) cleanupStaleConnections() {
	type closed struct{ edgeID, session string }
	var ended []closed
	defer func() {
		if p := s.pushDelivery(); p != nil {
			for _, e := range ended {
				p.StreamClosed(e.edgeID, e.session)
			}
		}
	}()

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

			// Only if this stream is still the edge's current one.
			if _, err := models.ReleaseEdgeStream(s.db, edgeID, edge.SessionID); err != nil {
				log.Warn().Err(err).Str("edge_id", edgeID).Msg("Failed to record the end of a stale edge stream")
			}

			toRemove = append(toRemove, edgeID)
			ended = append(ended, closed{edgeID, edge.SessionID})
		}
	}

	for _, edgeID := range toRemove {
		delete(s.edgeConnections, edgeID)
	}

	if len(toRemove) > 0 {
		log.Info().Int("removed_count", len(toRemove)).Msg("Cleaned up stale edge connections")
	}
}

// Config change event topics (defined locally to avoid import cycle with services package)
// Note: Only objects included in the microgateway ConfigurationSnapshot trigger sync.
// Users and Groups are AI Studio-only and don't affect edge sync. Tools are in
// the snapshot (edges serve /tools/{slug} and its MCP endpoint), so a tool edit
// such as flipping an access-method switch has to mark edges as pending.
const (
	topicLLMCreated         = "system.llm.created"
	topicLLMUpdated         = "system.llm.updated"
	topicLLMDeleted         = "system.llm.deleted"
	topicAppCreated         = "system.app.created"
	topicAppUpdated         = "system.app.updated"
	topicAppDeleted         = "system.app.deleted"
	topicFilterCreated      = "system.filter.created"
	topicFilterUpdated      = "system.filter.updated"
	topicFilterDeleted      = "system.filter.deleted"
	topicPluginCreated      = "system.plugin.created"
	topicPluginUpdated      = "system.plugin.updated"
	topicPluginDeleted      = "system.plugin.deleted"
	topicModelPriceCreated  = "system.model_price.created"
	topicModelPriceUpdated  = "system.model_price.updated"
	topicModelPriceDeleted  = "system.model_price.deleted"
	topicModelRouterCreated = "system.model_router.created"
	topicModelRouterUpdated = "system.model_router.updated"
	topicModelRouterDeleted = "system.model_router.deleted"
	topicSemanticRouterCreated = "system.semantic_router.created"
	topicSemanticRouterUpdated = "system.semantic_router.updated"
	topicSemanticRouterDeleted = "system.semantic_router.deleted"
	topicToolCreated        = "system.tool.created"
	topicToolUpdated        = "system.tool.updated"
	topicToolDeleted        = "system.tool.deleted"

	// Embedders are flattened into the datasources (and semantic routers)
	// that use them, so an embedder edit changes the snapshot.
	topicEmbedderCreated = "system.embedder.created"
	topicEmbedderUpdated = "system.embedder.updated"
	topicEmbedderDeleted = "system.embedder.deleted"

	// Governed metadata (Enterprise): gateway-visible fields are part of the snapshot.
	topicGovernedMetadataUpdated = "system.governed_metadata.updated"
	topicGovernedMetadataDeleted = "system.governed_metadata.deleted"
)

// subscribeToConfigChanges sets up event subscriptions for configuration changes.
// When any relevant config change occurs, checksums are recomputed for all namespaces.
func (s *ControlServer) subscribeToConfigChanges() {
	if s.eventBus == nil {
		log.Warn().Msg("Event bus not available, config change subscriptions not set up")
		return
	}

	configTopics := []string{
		topicLLMCreated, topicLLMUpdated, topicLLMDeleted,
		topicAppCreated, topicAppUpdated, topicAppDeleted,
		topicFilterCreated, topicFilterUpdated, topicFilterDeleted,
		topicPluginCreated, topicPluginUpdated, topicPluginDeleted,
		topicModelPriceCreated, topicModelPriceUpdated, topicModelPriceDeleted,
		topicModelRouterCreated, topicModelRouterUpdated, topicModelRouterDeleted,
		topicSemanticRouterCreated, topicSemanticRouterUpdated, topicSemanticRouterDeleted,
		topicToolCreated, topicToolUpdated, topicToolDeleted,
		topicEmbedderCreated, topicEmbedderUpdated, topicEmbedderDeleted,
		topicGovernedMetadataUpdated, topicGovernedMetadataDeleted,
	}

	for _, topic := range configTopics {
		topic := topic // Capture for closure
		s.eventBus.Subscribe(topic, func(event eventbridge.Event) {
			// The replica that made the change recomputes every namespace
			// from the database; relayed copies of its event need not.
			if event.RelayedFrom != "" {
				return
			}
			s.onConfigurationChanged(topic, event)
		})
	}

	log.Info().Int("topic_count", len(configTopics)).Msg("Subscribed to configuration change events for sync status tracking")
}

// onConfigurationChanged handles configuration change events by recomputing
// checksums for all namespaces and marking affected edges as pending.
func (s *ControlServer) onConfigurationChanged(topic string, event eventbridge.Event) {
	log.Info().
		Str("topic", topic).
		Str("event_id", event.ID).
		Msg("Configuration changed, recomputing namespace checksums")

	// Every namespace the database knows about, not just those with edges on
	// this replica: other replicas hold streams too, and an edge that is
	// offline now must see the drift when it comes back.
	namespaces := s.namespacesToRecompute()

	for _, namespace := range namespaces {
		// Recompute the snapshot and checksum for this namespace. This updates
		// NamespaceSyncStatus and marks edges pending only when the checksum
		// actually changed (see updateNamespaceSyncStatus), so changes that do not
		// alter the snapshot (a description edit, governed metadata that is not
		// gateway-visible) never churn edges.
		snapshot, err := s.getConfigurationSnapshot(namespace)
		if err != nil {
			log.Error().Err(err).Str("namespace", namespace).Msg("Failed to recompute snapshot on config change")
			continue
		}

		log.Info().
			Str("namespace", namespace).
			Str("checksum", snapshot.Checksum).
			Str("version", snapshot.Version).
			Msg("Recomputed namespace checksum after config change")
	}
}

// namespacesToRecompute returns every namespace an edge is registered in
// or a sync status is kept for, in canonical spelling, plus "default". It
// reads the database, so the answer is the same on every replica.
func (s *ControlServer) namespacesToRecompute() []string {
	set := map[string]bool{models.CanonicalNamespace("default"): true}
	var fromEdges, fromStatus []string
	if err := s.db.Model(&models.EdgeInstance{}).Distinct("namespace").Pluck("namespace", &fromEdges).Error; err != nil {
		log.Warn().Err(err).Msg("Failed to list edge namespaces for checksum recompute")
	}
	if err := s.db.Model(&models.NamespaceSyncStatus{}).Distinct("namespace").Pluck("namespace", &fromStatus).Error; err != nil {
		log.Warn().Err(err).Msg("Failed to list namespace sync statuses for checksum recompute")
	}
	for _, ns := range append(fromEdges, fromStatus...) {
		set[models.CanonicalNamespace(ns)] = true
	}
	namespaces := make([]string, 0, len(set))
	for ns := range set {
		namespaces = append(namespaces, ns)
	}
	sort.Strings(namespaces)
	return namespaces
}

// shouldIncludeAppInResponse checks if an App should be included in the token validation
// response for pull-on-miss sync. Returns true if:
// - App has global namespace (empty string) - syncs to all edges
// - App's namespace matches the requesting edge's namespace
func (s *ControlServer) shouldIncludeAppInResponse(appNamespace, edgeNamespace string) bool {
	// Global apps (empty namespace) sync to all edges
	if appNamespace == "" {
		return true
	}
	// Namespaced apps sync only to matching edges
	return appNamespace == edgeNamespace
}

// appModelRouterIDs lists the Model Routers an App was granted.
func appModelRouterIDs(app *models.App) []uint32 {
	ids := make([]uint32, len(app.ModelRouters))
	for i, r := range app.ModelRouters {
		ids[i] = uint32(r.ID)
	}
	return ids
}

// appSemanticRouterIDs lists the Semantic Routers an App was granted.
func appSemanticRouterIDs(app *models.App) []uint32 {
	ids := make([]uint32, len(app.SemanticRouters))
	for i, r := range app.SemanticRouters {
		ids[i] = uint32(r.ID)
	}
	return ids
}

// convertAppToProto converts a models.App to a pb.AppConfig for pull-on-miss sync.
// Note: CurrentPeriodUsage is set to 0 - the edge tracks budget locally via analytics.
func (s *ControlServer) convertAppToProto(app *models.App) *pb.AppConfig {
	// Get associated LLM IDs
	llmIDs := make([]uint32, len(app.LLMs))
	for i, llm := range app.LLMs {
		llmIDs[i] = uint32(llm.ID)
	}

	// Get associated Tool IDs
	toolIDs := make([]uint32, len(app.Tools))
	for i, tool := range app.Tools {
		toolIDs[i] = uint32(tool.ID)
	}

	// Get associated Datasource IDs
	datasourceIDs := make([]uint32, len(app.Datasources))
	for i, ds := range app.Datasources {
		datasourceIDs[i] = uint32(ds.ID)
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

	// Note: CurrentPeriodUsage is intentionally set to 0 for pull-on-miss sync.
	// The edge gateway maintains its own budget tracking via local analytics.
	// Setting it to 0 here avoids an expensive SUM(cost) query on llm_chat_records
	// for every token validation, and prevents overwriting the edge's local budget data.

	return &pb.AppConfig{
		Id:                 uint32(app.ID),
		Name:               app.Name,
		Description:        app.Description,
		OwnerEmail:         "", // AI Studio doesn't have owner email field yet
		IsActive:           app.IsActive,
		MonthlyBudget:      monthlyBudget,
		BudgetStartDate:    budgetStartDate,
		Metadata:           metadataJSON,
		Namespace:          app.Namespace,
		UserId:             uint32(app.UserID), // Owner user ID for analytics tracking
		LlmIds:             llmIDs,
		ToolIds:            toolIDs,
		DatasourceIds:      datasourceIDs,
		ModelRouterIds:     appModelRouterIDs(app),
		SemanticRouterIds:  appSemanticRouterIDs(app),
		CurrentPeriodUsage: 0, // Intentionally 0 - edge tracks budget locally
		CreatedAt:          timestamppb.New(app.CreatedAt),
		UpdatedAt:          timestamppb.New(app.UpdatedAt),
	}
}

// SetGovernedMetadataReader wires the governed metadata service so gateway-visible
// fields are included in configuration snapshots. Safe to leave unset (CE).
func (s *ControlServer) SetGovernedMetadataReader(reader governed_metadata.SnapshotReader) {
	s.governedMetadata = reader
}

// loadGovernedMetadata batch-loads all governed metadata records for an object type.
// Returns nil when no reader is configured or the lookup fails.
func (s *ControlServer) loadGovernedMetadata(objectType string) map[string]*models.ObjectMetadata {
	if s.governedMetadata == nil {
		return nil
	}
	recs, err := s.governedMetadata.ListObjectMetadata(objectType, nil)
	if err != nil {
		log.Warn().Err(err).Str("object_type", objectType).Msg("Failed to load governed metadata for snapshot")
		return nil
	}
	return recs
}

// governedMetadataJSON serialises the gateway-visible governed metadata for one object.
// Returns "" when there is nothing to send so CE snapshots are byte-identical.
func (s *ControlServer) governedMetadataJSON(objectType string, rec *models.ObjectMetadata) string {
	if s.governedMetadata == nil || rec == nil {
		return ""
	}
	values := s.governedMetadata.VisibleValues(objectType, rec, governed_metadata.VisibilityGateway)
	if len(values) == 0 {
		return ""
	}
	data, err := json.Marshal(values)
	if err != nil {
		return ""
	}
	return string(data)
}
