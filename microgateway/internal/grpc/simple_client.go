// internal/grpc/simple_client.go
package grpc

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
	"github.com/TykTechnologies/midsommar/microgateway/internal/services"
	"github.com/TykTechnologies/midsommar/v2/pkg/eventbridge"
	"github.com/TykTechnologies/midsommar/v2/pkg/fipsinfo"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// BudgetSyncSubscriber is implemented by handlers that want to receive budget sync events
type BudgetSyncSubscriber interface {
	HandleBudgetSync(event eventbridge.Event)
}

// SimpleEdgeClient provides basic edge functionality for testing
type SimpleEdgeClient struct {
	config      *config.Config
	conn        *grpc.ClientConn
	client      pb.ConfigurationSyncServiceClient
	configCache *pb.ConfigurationSnapshot
	// connMu guards conn, client and connEndpoint. A reconnect replaces
	// them while token validation, the analytics pulse and the payload queue
	// use them from other goroutines, so they are only read and written
	// through connection and setConnection.
	connMu sync.RWMutex
	// connEndpoint is the control endpoint conn was dialled to.
	connEndpoint string

	// endpoints are the control planes this edge may use, in the order it
	// prefers them (config.HubSpokeConfig.Endpoints: CONTROL_ENDPOINTS, or
	// CONTROL_ENDPOINT as a list of one). endpointIdx is the one it is on,
	// or tried last. See reconnectLoop for how it moves between them and
	// failbackWorker for how it returns to the first.
	endpoints    []string
	endpointIdx  atomic.Int32
	failbackOnce sync.Once

	// Last-known-good (last_known_good.go). lastKnownGood is set while the
	// edge serves the snapshot stored in its own database because no
	// control plane answered at start-up, and cleared when a control plane
	// sends a configuration. lastContact is the time (Unix nanoseconds) of
	// the last registration a control plane accepted or heartbeat it
	// acknowledged; the onContact hooks are told of each.
	lastKnownGood atomic.Bool
	lastContact   atomic.Int64
	onContact     hookList[func(time.Time)]
	onRegistered  hookList[func()]
	// startupDeadline and startupMinAttempt are set while StartWithin runs
	// (main goroutine only): a registration then has until the deadline,
	// and at least startupMinAttempt, to be answered.
	startupDeadline   time.Time
	startupMinAttempt time.Duration

	// Config checksum tracking for sync status; guarded by configMu with
	// configCache (see setLoadedConfig).
	configMu       sync.RWMutex
	loadedChecksum string
	loadedVersion  string
	// appliedSource, when set, says which configuration the edge applied to
	// its database (services.EdgeSyncService.ReportedConfig); heartbeats
	// report that instead of the last snapshot received.
	appliedSource func() (checksum, version string, ok bool)
	// templateMatches, when set, holds the trusted prompt templates that
	// matched since the last heartbeat (services.PromptTemplateMatches);
	// guarded by configMu.
	templateMatches TemplateMatchSource
	// exceptionHits, when set, holds the guardrail exceptions that applied
	// since the last heartbeat (services.GuardrailExceptionHits); guarded
	// by configMu.
	exceptionHits TemplateMatchSource

	// Build information
	version   string
	buildHash string
	buildTime string

	// Bidirectional streaming. stream is read and replaced through
	// currentStream and setStream (a reconnect replaces it while heartbeats,
	// events and reload statuses are being sent), and it is always a
	// serialSendStream: gRPC allows one Send at a time per stream.
	streamMu     sync.RWMutex
	stream       pb.ConfigurationSyncService_SubscribeToChangesClient
	// streamDone is closed when the receive loop of the current stream has
	// ended and cleaned up after itself (guarded by streamMu). A failback
	// waits for it before it opens the stream on the first endpoint.
	streamDone   chan struct{}
	streamCtx    context.Context
	streamCancel context.CancelFunc

	// Callback for configuration updates
	onConfigChange func(*pb.ConfigurationSnapshot)

	// Reload handling (use interface to avoid import cycle). reloadMu
	// guards reloadHandler and heldReloads: the handler is set from main
	// while the stream's receive loop reads it. A push that arrives before
	// the handler is set (control delivers waiting pushes as soon as the
	// stream opens, and cmd/microgateway sets the handler after its
	// services are up) is held and handed over when it is set, instead of
	// being dropped and left for control's answer timeout.
	reloadMu      sync.Mutex
	reloadHandler interface{}
	heldReloads   []*pb.ConfigurationReloadRequest

	// Control payload queue for edge-to-control plugin communication
	controlPayloadQueue *ControlPayloadQueue

	// Connection state. Set by the goroutine that opens a stream, cleared
	// by that stream's receive loop, read by the heartbeat worker.
	connected atomic.Bool

	// Reconnection handling
	// reconnecting guards attemptReconnection: a stream that drops again
	// right after a reconnect starts a second attempt while the first is
	// still unwinding, so the check and set must be one atomic step. A
	// failback (tryFailback) holds it too, so the two never replace the
	// connection at the same time.
	// reconnectAttempts is only touched by the goroutine holding it; it
	// counts passes over the endpoint list.
	reconnecting      atomic.Bool
	reconnectAttempts int
	maxReconnects     int
	reconnectInterval time.Duration

	// Event bridge components
	eventBus      eventbridge.Bus
	streamAdapter *eventbridge.StreamAdapter
	eventBridge   *eventbridge.Bridge
	bridgeCtx     context.Context
	bridgeCancel  context.CancelFunc

	// Budget sync handler for multi-edge budget synchronization
	budgetSyncHandler BudgetSyncSubscriber

	// Shutdown channel - closed when Close() is called to stop reconnection
	stopCh chan struct{}
}

// NewSimpleEdgeClient creates a basic edge client
func NewSimpleEdgeClient(cfg *config.Config, version, buildHash, buildTime string) *SimpleEdgeClient {
	client := &SimpleEdgeClient{
		config:            cfg,
		version:           version,
		buildHash:         buildHash,
		buildTime:         buildTime,
		maxReconnects:     -1,              // Unlimited reconnections
		reconnectInterval: 5 * time.Second, // 5 second retry interval
		eventBus:          eventbridge.NewBus(),
		stopCh:            make(chan struct{}),
	}

	// EDGE_RECONNECT_INTERVAL is the base of the reconnect backoff, which
	// doubles per attempt up to maxReconnectBackoff.
	if cfg != nil && cfg.HubSpoke.ReconnectInterval > 0 {
		client.reconnectInterval = cfg.HubSpoke.ReconnectInterval
	}
	if cfg != nil {
		client.endpoints = cfg.HubSpoke.Endpoints()
	}

	log.Debug().Msg("Event bridge bus initialized for edge client")
	return client
}

// Start connects to the control server and gets initial configuration. With
// several control endpoints it tries each once, in order, and returns the
// last error when none accepts the edge.
func (c *SimpleEdgeClient) Start() error {
	c.startFailback()
	if len(c.endpoints) == 0 {
		return c.startOn(0)
	}
	var err error
	for idx := range c.endpoints {
		if err = c.startOn(idx); err == nil {
			return nil
		}
		if idx < len(c.endpoints)-1 {
			log.Warn().Err(err).
				Str("control_endpoint", c.endpoints[idx]).
				Str("next_endpoint", c.endpoints[idx+1]).
				Msg("Control endpoint did not accept this edge, trying the next one")
		}
	}
	return err
}

// startOn makes the first connection, to the endpoint at idx.
func (c *SimpleEdgeClient) startOn(idx int) error {
	c.endpointIdx.Store(int32(idx))
	endpoint := c.currentEndpoint()
	log.Debug().
		Str("control_endpoint", endpoint).
		Str("edge_id", c.config.HubSpoke.EdgeID).
		Msg("Connecting to control server")

	// Connect to control server
	conn, err := c.dialWithKeepalive()
	if err != nil {
		return fmt.Errorf("failed to connect to control server: %w", err)
	}
	if err := c.awaitReady(conn); err != nil {
		conn.Close()
		return fmt.Errorf("failed to connect to control server: %w", err)
	}

	c.setConnection(conn, endpoint)

	// Wire the control payload queue's gRPC client if set
	if c.controlPayloadQueue != nil {
		c.controlPayloadQueue.SetGRPCClient(c.GetGRPCClient(), c.createAuthContext)
	}

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

	log.Debug().Msg("Successfully connected to control server with streaming")
	c.logConnected(endpoint)
	return nil
}

// logConnected says which control endpoint the edge is now on. With one
// endpoint that is known, and stays at debug level.
func (c *SimpleEdgeClient) logConnected(endpoint string) {
	if len(c.endpoints) < 2 {
		log.Debug().Str("control_endpoint", endpoint).Msg("Connected to control plane")
		return
	}
	log.Info().
		Str("control_endpoint", endpoint).
		Int("endpoint_position", int(c.endpointIdx.Load())+1).
		Int("endpoint_count", len(c.endpoints)).
		Msg("Connected to control plane")
}

// currentEndpoint is the control endpoint the client is on or tried last;
// empty when none is configured.
func (c *SimpleEdgeClient) currentEndpoint() string {
	idx := int(c.endpointIdx.Load())
	if idx < 0 || idx >= len(c.endpoints) {
		return ""
	}
	return c.endpoints[idx]
}

// ControlLink is an edge's connection to the control plane, for /ready and
// metrics.
type ControlLink struct {
	// Endpoint is the control endpoint the edge is connected to, or, while
	// it is not connected, the one it tried last.
	Endpoint string
	// Connected is true while the stream to Endpoint is open.
	Connected bool
	// Endpoints are the configured control endpoints, in order.
	Endpoints []string
}

// ControlLink reports which control endpoint the edge is on and whether its
// stream is open.
func (c *SimpleEdgeClient) ControlLink() ControlLink {
	link := ControlLink{Endpoints: append([]string(nil), c.endpoints...), Connected: c.connected.Load()}
	c.connMu.RLock()
	link.Endpoint = c.connEndpoint
	c.connMu.RUnlock()
	if link.Endpoint == "" {
		link.Endpoint = c.currentEndpoint()
	}
	return link
}

// registerWithControl registers this edge with the control server on the
// current connection and applies the configuration it answers with.
func (c *SimpleEdgeClient) registerWithControl() error {
	client := c.GetGRPCClient()
	if client == nil {
		return fmt.Errorf("not connected to control server")
	}
	ctx := context.Background()
	if !c.startupDeadline.IsZero() {
		// StartWithin: a control plane that accepts the connection and then
		// does not answer must not hold the start-up past its limit.
		limit := time.Until(c.startupDeadline)
		if limit < c.startupMinAttempt {
			limit = c.startupMinAttempt
		}
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, limit)
		defer cancel()
	}
	resp, err := c.registerOn(ctx, client)
	if err != nil {
		return err
	}
	c.applyRegistration(resp)
	return nil
}

// registerOn registers this edge through client.
func (c *SimpleEdgeClient) registerOn(ctx context.Context, client pb.ConfigurationSyncServiceClient) (*pb.EdgeRegistrationResponse, error) {
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
			Metrics:   fipsinfo.Current().Metrics(), // FIPS 140-3 status, for the hub
		},
	}

	// Register with control (with authentication)
	authCtx := c.createAuthContext(ctx)
	resp, err := client.RegisterEdge(authCtx, req)
	if err != nil {
		return nil, fmt.Errorf("registration failed: %w", err)
	}

	if !resp.Success {
		return nil, fmt.Errorf("registration rejected: %s", resp.Message)
	}
	c.onRegistered.each(func(fn func()) { fn() })
	c.controlContact()
	return resp, nil
}

// applyRegistration takes the configuration a registration answered with.
func (c *SimpleEdgeClient) applyRegistration(resp *pb.EdgeRegistrationResponse) {
	// Store initial configuration if provided
	if resp.InitialConfig != nil {
		log.Debug().
			Str("version", resp.InitialConfig.Version).
			Str("checksum", resp.InitialConfig.Checksum).
			Int32("llm_count", int32(len(resp.InitialConfig.Llms))).
			Int32("app_count", int32(len(resp.InitialConfig.Apps))).
			Msg("Received initial configuration from control")

		c.setLoadedConfig(resp.InitialConfig)

		// Notify provider if callback is set
		if c.onConfigChange != nil {
			c.onConfigChange(resp.InitialConfig)
		}
	}

	log.Debug().
		Str("session_id", resp.SessionId).
		Msg("Successfully registered with control server")
}

// SetOnConfigChange sets the callback for configuration changes
func (c *SimpleEdgeClient) SetOnConfigChange(callback func(*pb.ConfigurationSnapshot)) {
	c.onConfigChange = callback
}

// IsConnected returns true if connected to control
func (c *SimpleEdgeClient) IsConnected() bool {
	conn, _ := c.connection()
	return conn != nil
}

// GetCurrentConfiguration returns the cached configuration
func (c *SimpleEdgeClient) GetCurrentConfiguration() *pb.ConfigurationSnapshot {
	c.configMu.RLock()
	defer c.configMu.RUnlock()
	return c.configCache
}

// setLoadedConfig records the snapshot this edge now runs. Pushes, stream
// updates and the initial registration write it while heartbeats report
// it, so it is only read and written under configMu.
func (c *SimpleEdgeClient) setLoadedConfig(snap *pb.ConfigurationSnapshot) {
	c.configMu.Lock()
	defer c.configMu.Unlock()
	c.configCache = snap
	c.loadedChecksum = snap.Checksum
	c.loadedVersion = snap.Version
	// Every caller but UseLastKnownGood has it from a control plane.
	c.lastKnownGood.Store(false)
}

// SetAppliedConfigSource makes heartbeats report the configuration the edge
// applied, as source tells it, rather than the last snapshot received: a
// snapshot that failed to apply, or was applied without some entries, then
// shows on the hub as not in sync.
func (c *SimpleEdgeClient) SetAppliedConfigSource(source func() (checksum, version string, ok bool)) {
	c.configMu.Lock()
	defer c.configMu.Unlock()
	c.appliedSource = source
}

// TemplateMatchSource holds the IDs of the trusted prompt templates that
// matched requests on this edge, for heartbeats to report.
type TemplateMatchSource interface {
	// DrainMatched returns the IDs matched since the last call.
	DrainMatched() []uint32
	// Requeue puts back IDs a heartbeat failed to deliver.
	Requeue(ids []uint32)
}

// SetTemplateMatchSource makes heartbeats report the trusted prompt
// templates that matched since the last one, so the hub's expiry counts
// edge traffic.
func (c *SimpleEdgeClient) SetTemplateMatchSource(source TemplateMatchSource) {
	c.configMu.Lock()
	defer c.configMu.Unlock()
	c.templateMatches = source
}

// SetExceptionHitSource makes heartbeats report the guardrail exceptions
// that applied since the last one, so the hub's expiry and hit counts
// include edge traffic.
func (c *SimpleEdgeClient) SetExceptionHitSource(source TemplateMatchSource) {
	c.configMu.Lock()
	defer c.configMu.Unlock()
	c.exceptionHits = source
}

// loadedConfig returns the checksum and version of the loaded snapshot.
func (c *SimpleEdgeClient) loadedConfig() (checksum, version string) {
	c.configMu.RLock()
	source := c.appliedSource
	checksum, version = c.loadedChecksum, c.loadedVersion
	c.configMu.RUnlock()
	if source != nil {
		if sum, ver, ok := source(); ok {
			return sum, ver
		}
	}
	return checksum, version
}

// GetEventBus returns the edge client's event bus for subscribing to events.
// This allows other microgateway components to subscribe to events from control
// and publish events to control.
func (c *SimpleEdgeClient) GetEventBus() eventbridge.Bus {
	return c.eventBus
}

// ValidateTokenOnDemand validates a token by calling the control instance
// tokenValidationTimeout bounds an on-demand token validation call.
const tokenValidationTimeout = 5 * time.Second

func (c *SimpleEdgeClient) ValidateTokenOnDemand(token string) (*pb.TokenValidationResponse, error) {
	conn, client := c.connection()
	if conn == nil || client == nil {
		return nil, fmt.Errorf("not connected to control instance")
	}

	tokenPrefix := token
	if len(token) > 8 {
		tokenPrefix = token[:8]
	}

	log.Debug().Str("token_prefix", tokenPrefix).Msg("SimpleEdgeClient: making on-demand token validation request to control")

	// Requests wait for this call, so it must not hang on a slow or stuck
	// control instance.
	ctx, cancel := context.WithTimeout(context.Background(), tokenValidationTimeout)
	defer cancel()

	// Create token validation request
	req := &pb.TokenValidationRequest{
		Token:         token,
		EdgeId:        c.config.HubSpoke.EdgeID,
		EdgeNamespace: c.config.HubSpoke.EdgeNamespace,
	}

	// Call control instance (with authentication)
	authCtx := c.createAuthContext(ctx)
	resp, err := client.ValidateToken(authCtx, req)
	if err != nil {
		log.Debug().Err(err).Str("token_prefix", tokenPrefix).Msg("SimpleEdgeClient: token validation gRPC call failed")
		return nil, fmt.Errorf("token validation failed: %w", err)
	}

	log.Debug().
		Str("token_prefix", tokenPrefix).
		Bool("valid", resp.Valid).
		Uint32("app_id", resp.AppId).
		Msg("SimpleEdgeClient: received token validation response from control")

	return resp, nil
}

// maxHeldReloads bounds the pushes held while no reload handler is set.
// Each push asks for the same thing (pull the current configuration), so
// dropping the oldest loses nothing but its answer; control retries it.
const maxHeldReloads = 16

// SetReloadHandler sets the reload handler for processing reload requests.
// Pushes that arrived before it was set are handed to it now, in order, on
// a goroutine of their own (a reload takes seconds; the caller is usually
// main, still starting up).
func (c *SimpleEdgeClient) SetReloadHandler(handler interface{}) {
	c.reloadMu.Lock()
	c.reloadHandler = handler
	var held []*pb.ConfigurationReloadRequest
	if handler != nil {
		held, c.heldReloads = c.heldReloads, nil
	}
	c.reloadMu.Unlock()
	log.Debug().Msg("Reload handler set for edge client")

	if len(held) > 0 {
		log.Info().Int("count", len(held)).Msg("Handing pushes received during start-up to the reload handler")
		go func() {
			for _, req := range held {
				dispatchReload(handler, req)
			}
		}()
	}
}

// SetControlPayloadQueue sets the control payload queue for edge-to-control plugin communication
func (c *SimpleEdgeClient) SetControlPayloadQueue(queue *ControlPayloadQueue) {
	c.controlPayloadQueue = queue
	// Wire the queue's gRPC client and auth context
	if client := c.GetGRPCClient(); client != nil {
		queue.SetGRPCClient(client, c.createAuthContext)
	}
	log.Debug().Msg("Control payload queue set for edge client")
}

// SetBudgetSyncHandler sets the budget sync handler for multi-edge budget synchronization.
// The handler will receive budget.sync events from the control server via the event bridge.
func (c *SimpleEdgeClient) SetBudgetSyncHandler(handler BudgetSyncSubscriber) {
	c.budgetSyncHandler = handler
	// Subscribe to budget.sync topic if event bus exists
	if c.eventBus != nil && handler != nil {
		c.eventBus.Subscribe(services.BudgetSyncTopic, handler.HandleBudgetSync)
		log.Debug().Msg("Budget sync handler subscribed to budget.sync events")
	}
}

// GetGRPCClient returns the client of the current connection to control, or
// nil while disconnected. A reconnect replaces it (and closes the old one), so
// callers must ask for it on every use and never keep it.
func (c *SimpleEdgeClient) GetGRPCClient() pb.ConfigurationSyncServiceClient {
	_, client := c.connection()
	return client
}

// connection returns the current connection and its client.
func (c *SimpleEdgeClient) connection() (*grpc.ClientConn, pb.ConfigurationSyncServiceClient) {
	c.connMu.RLock()
	defer c.connMu.RUnlock()
	return c.conn, c.client
}

// setConnection makes conn, dialled to endpoint, the current connection, with
// a new client on it; nil clears them.
func (c *SimpleEdgeClient) setConnection(conn *grpc.ClientConn, endpoint string) {
	c.connMu.Lock()
	defer c.connMu.Unlock()
	c.conn = conn
	if conn == nil {
		c.client = nil
		c.connEndpoint = ""
		return
	}
	c.client = pb.NewConfigurationSyncServiceClient(conn)
	c.connEndpoint = endpoint
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
	log.Debug().Msg("Requesting full configuration sync from control")

	// Check if client is connected
	client := c.GetGRPCClient()
	if client == nil {
		return fmt.Errorf("not connected to control server")
	}

	ctx := context.Background()
	// EdgeId lets control resolve the namespace this edge registered under
	// and mark it in sync once the snapshot is handed over, instead of
	// waiting for the next heartbeat.
	req := &pb.ConfigurationRequest{
		EdgeId:        c.config.HubSpoke.EdgeID,
		EdgeNamespace: c.config.HubSpoke.EdgeNamespace,
	}

	authCtx := c.createAuthContext(ctx)
	resp, err := client.GetFullConfiguration(authCtx, req)
	if err != nil {
		return fmt.Errorf("failed to request full sync: %w", err)
	}

	// Update local cache and trigger callback
	c.setLoadedConfig(resp)

	if c.onConfigChange != nil {
		c.onConfigChange(resp)
	}

	log.Debug().
		Str("version", resp.Version).
		Str("checksum", resp.Checksum).
		Int("llm_count", len(resp.Llms)).
		Int("app_count", len(resp.Apps)).
		Msg("Full configuration sync completed")

	return nil
}

// establishStream establishes bidirectional streaming with control server
func (c *SimpleEdgeClient) establishStream() error {
	log.Debug().Str("edge_id", c.config.HubSpoke.EdgeID).Msg("Establishing bidirectional stream with control")

	ctx, cancel := context.WithCancel(context.Background())
	c.streamCtx = ctx
	c.streamCancel = cancel

	// Start streaming (with authentication)
	client := c.GetGRPCClient()
	if client == nil {
		cancel()
		return fmt.Errorf("failed to start streaming: not connected to control server")
	}
	authCtx := c.createAuthContext(ctx)
	stream, err := client.SubscribeToChanges(authCtx)
	if err != nil {
		cancel()
		return fmt.Errorf("failed to start streaming: %w", err)
	}

	c.setStream(stream)
	// A new stream may be to another control plane: let the budget sync
	// handler report a sequence that went backwards once more.
	if h, ok := c.budgetSyncHandler.(interface{ StreamStarted() }); ok {
		h.StreamStarted()
	}
	done := make(chan struct{})
	c.streamMu.Lock()
	c.streamDone = done
	c.streamMu.Unlock()

	// Setup event bridge for this connection
	c.setupEventBridge()

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

	if err := c.send(regMsg); err != nil {
		cancel()
		close(done) // no receive loop will run for this stream
		return fmt.Errorf("failed to send stream registration: %w", err)
	}

	// A heartbeat straight away, rather than after the first interval: it
	// reports the loaded checksum at once, and control delivers waiting
	// pushes to a stream only after its first heartbeat (or a grace period,
	// for edges that do not send one early). Pushes that arrive before the
	// reload handler is set are held (see HandleReloadRequest).
	c.sendHeartbeatMessage()

	// Connected before the goroutines start: a stream that ends at once
	// clears it from its receive loop, and setting it afterwards would
	// overwrite that with a stale true.
	c.connected.Store(true)

	// Start message handling goroutines
	go func() {
		defer close(done)
		c.handleIncomingMessages()
	}()

	// Start heartbeat worker for this stream. It gets the stream's context
	// rather than reading c.streamCtx, which the next reconnect replaces:
	// the old worker would then wait on the new stream and never stop.
	go c.heartbeatWorker(ctx)

	log.Debug().Msg("Bidirectional stream established successfully")
	return nil
}

// setupEventBridge creates and starts the event bridge for the current stream
func (c *SimpleEdgeClient) setupEventBridge() {
	// Create bridge context
	c.bridgeCtx, c.bridgeCancel = context.WithCancel(context.Background())

	// Create stream adapter that sends events to control
	c.streamAdapter = eventbridge.NewStreamAdapter(func(frame *eventbridge.EventFrame) error {
		return c.send(&pb.EdgeMessage{
			Message: &pb.EdgeMessage_Event{
				Event: &pb.EventFrame{
					Id:      frame.ID,
					Topic:   frame.Topic,
					Origin:  frame.Origin,
					Dir:     frame.Dir,
					Payload: frame.Payload,
				},
			},
		})
	}, 100)

	// Create and start bridge (edge node, not control)
	c.eventBridge = eventbridge.NewBridge(eventbridge.BridgeConfig{
		NodeID:    c.config.HubSpoke.EdgeID,
		IsControl: false,
	}, c.eventBus, c.streamAdapter)
	c.eventBridge.Start(c.bridgeCtx)

	log.Debug().Str("edge_id", c.config.HubSpoke.EdgeID).Msg("Event bridge started for edge client")
}

// stopEventBridge stops the event bridge and cleans up resources
func (c *SimpleEdgeClient) stopEventBridge() {
	if c.bridgeCancel != nil {
		c.bridgeCancel()
	}
	if c.eventBridge != nil {
		c.eventBridge.Stop()
	}
	if c.streamAdapter != nil {
		c.streamAdapter.Close()
	}
	// Closing the adapter ends the bridge's receive loop; wait for both
	// loops, so the bridge does not outlive the stream.
	if c.eventBridge != nil && !c.eventBridge.Wait(stopWaitTimeout) {
		log.Warn().Str("edge_id", c.config.HubSpoke.EdgeID).Msg("Event bridge did not stop in time")
	}
	log.Debug().Str("edge_id", c.config.HubSpoke.EdgeID).Msg("Event bridge stopped for edge client")
}

// handleIncomingMessages processes messages from control server with comprehensive error recovery
func (c *SimpleEdgeClient) handleIncomingMessages() {
	stream := c.currentStream()
	defer func() {
		// Handle panic recovery
		if r := recover(); r != nil {
			log.Error().Interface("panic", r).Msg("Panic in message handler, recovering")
		}

		c.connected.Store(false)

		// Stop event bridge before cancelling stream context
		c.stopEventBridge()

		if c.streamCancel != nil {
			c.streamCancel()
		}

		// Start reconnection process if not already stopping
		if !c.stopping() {
			go c.attemptReconnection()
		}
	}()

	for {
		msg, err := stream.Recv()
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

	case *pb.ControlMessage_Event:
		return c.handleEventMessage(m.Event)

	default:
		log.Warn().Msg("Unknown control message type received")
		return nil
	}
}

// handleEventMessage handles event bridge messages from control
func (c *SimpleEdgeClient) handleEventMessage(event *pb.EventFrame) error {
	if event == nil {
		log.Debug().Msg("handleEventMessage called with nil event")
		return nil
	}

	log.Debug().
		Str("event_id", event.Id).
		Str("topic", event.Topic).
		Str("origin", event.Origin).
		Int32("direction", event.Dir).
		Int("payload_len", len(event.Payload)).
		Msg("Edge received event from control")

	// Enqueue the event for the bridge to process
	if c.streamAdapter != nil {
		enqueued := c.streamAdapter.EnqueueProtoEvent(event)
		log.Debug().
			Str("event_id", event.Id).
			Str("topic", event.Topic).
			Bool("enqueued", enqueued).
			Msg("Edge enqueued event for bridge processing")
	} else {
		log.Warn().
			Str("event_id", event.Id).
			Str("topic", event.Topic).
			Msg("Edge streamAdapter is nil - cannot enqueue event")
	}

	return nil
}

// handleRegistrationResponse processes registration responses
func (c *SimpleEdgeClient) handleRegistrationResponse(resp *pb.EdgeRegistrationResponse) error {
	log.Debug().
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
	log.Debug().
		Str("version", config.Version).
		Str("checksum", config.Checksum).
		Int("llm_count", len(config.Llms)).
		Int("app_count", len(config.Apps)).
		Msg("Received configuration update via stream")

	c.setLoadedConfig(config)

	log.Info().
		Str("checksum", config.Checksum).
		Str("version", config.Version).
		Msg("Configuration loaded and checksum stored for sync tracking")

	// Call configuration change callback if set
	if c.onConfigChange != nil {
		c.onConfigChange(config)
	}

	return nil
}

// handleConfigurationChange processes incremental configuration changes
func (c *SimpleEdgeClient) handleConfigurationChange(change *pb.ConfigurationChange) error {
	log.Debug().
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
		Bool("is_in_sync", resp.IsInSync).
		Str("expected_checksum", resp.ExpectedChecksum).
		Bool("request_full_sync", resp.RequestFullSync).
		Bool("shutdown_requested", resp.ShutdownRequested).
		Msg("Received heartbeat response")

	if resp.Acknowledged {
		c.controlContact()
	}

	// Log sync status warning if out of sync
	if resp.ExpectedChecksum != "" && !resp.IsInSync {
		loaded, _ := c.loadedConfig()
		log.Warn().
			Str("loaded_checksum", loaded).
			Str("expected_checksum", resp.ExpectedChecksum).
			Msg("Edge is out of sync with control - configuration update pending")
	}

	// Handle control directives
	if resp.RequestFullSync {
		log.Info().Msg("Control server requested full configuration sync")
		return c.RequestFullSync()
	}

	if resp.ShutdownRequested {
		log.Debug().Msg("Control server requested graceful shutdown")
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
	log.Debug().
		Str("operation_id", req.OperationId).
		Str("target_namespace", req.TargetNamespace).
		Msg("Received reload request via stream")

	// Delegate to the existing reload handling logic
	c.HandleReloadRequest(req)
	return nil
}

// SendReloadStatus sends a reload status update to control server via stream
func (c *SimpleEdgeClient) SendReloadStatus(response *pb.ConfigurationReloadResponse) error {
	if c.currentStream() == nil {
		return fmt.Errorf("no stream available for sending reload status")
	}

	log.Debug().
		Str("operation_id", response.OperationId).
		Str("phase", response.Phase.String()).
		Bool("success", response.Success).
		Msg("Sending reload status to control via stream")

	msg := &pb.EdgeMessage{
		Message: &pb.EdgeMessage_ReloadResponse{
			ReloadResponse: response,
		},
	}

	if err := c.send(msg); err != nil {
		log.Error().Err(err).Msg("Failed to send reload status via stream")
		return fmt.Errorf("failed to send reload status: %w", err)
	}

	return nil
}

// HandleReloadRequest processes reload requests (to be called when reload messages are received)
func (c *SimpleEdgeClient) HandleReloadRequest(req *pb.ConfigurationReloadRequest) {
	log.Debug().
		Str("operation_id", req.OperationId).
		Msg("SimpleEdgeClient received reload request")

	c.reloadMu.Lock()
	handler := c.reloadHandler
	if handler == nil {
		// Not ready yet: hold it for SetReloadHandler. A push control sends
		// again (same operation) replaces the one held.
		for i, h := range c.heldReloads {
			if h.OperationId == req.OperationId {
				c.heldReloads = append(c.heldReloads[:i], c.heldReloads[i+1:]...)
				break
			}
		}
		if len(c.heldReloads) >= maxHeldReloads {
			c.heldReloads = c.heldReloads[1:]
		}
		c.heldReloads = append(c.heldReloads, req)
		c.reloadMu.Unlock()
		log.Info().Str("operation_id", req.OperationId).Msg("Reload request held until the reload handler is set")
		return
	}
	c.reloadMu.Unlock()
	dispatchReload(handler, req)
}

// dispatchReload hands req to handler.
func dispatchReload(handler interface{}, req *pb.ConfigurationReloadRequest) {
	if h, ok := handler.(interface{ HandleReloadRequest(*pb.ConfigurationReloadRequest) }); ok {
		h.HandleReloadRequest(req)
		return
	}
	log.Error().Msg("Reload handler does not implement HandleReloadRequest method")
}

// Stop closes the connection to control server
func (c *SimpleEdgeClient) Stop() error {
	log.Debug().Msg("Stopping SimpleEdgeClient")

	// Signal shutdown to stop reconnection attempts
	select {
	case <-c.stopCh:
		// Already closed
	default:
		close(c.stopCh)
	}

	// Cancel stream context
	if c.streamCancel != nil {
		c.streamCancel()
	}

	// Close connection
	var err error
	if conn, _ := c.connection(); conn != nil {
		log.Debug().Msg("Closing connection to control server")
		err = conn.Close()
	}

	// Let the receive loop of the stream end and clean up (it stops the
	// event bridge), so nothing of this client runs once Stop returns.
	c.streamMu.RLock()
	done := c.streamDone
	c.streamMu.RUnlock()
	if done != nil {
		select {
		case <-done:
		case <-time.After(stopWaitTimeout):
			log.Warn().Msg("Stopping the edge client: the stream's receive loop did not end in time")
		}
	}
	return err
}

// stopWaitTimeout bounds how long Stop waits for the stream's goroutines.
const stopWaitTimeout = 5 * time.Second

// heartbeatWorker sends periodic heartbeats to the control instance until
// streamCtx, the context of the stream it was started for, is done.
func (c *SimpleEdgeClient) heartbeatWorker(streamCtx context.Context) {
	ticker := time.NewTicker(c.config.HubSpoke.HeartbeatInterval)
	defer ticker.Stop()

	log.Debug().
		Dur("interval", c.config.HubSpoke.HeartbeatInterval).
		Msg("Starting heartbeat worker")

	for {
		select {
		case <-ticker.C:
			if c.connected.Load() && c.currentStream() != nil {
				c.sendHeartbeat()
			} else {
				log.Debug().
					Bool("connected", c.connected.Load()).
					Bool("stream_not_nil", c.currentStream() != nil).
					Msg("Skipping heartbeat - not connected or stream is nil")
			}
		case <-streamCtx.Done():
			log.Debug().Msg("Heartbeat worker stopping")
			return
		}
	}
}

// sendHeartbeat sends a heartbeat message to the control instance
func (c *SimpleEdgeClient) sendHeartbeat() {
	c.sendHeartbeatMessage()

	// Send pending control payloads (piggybacking on heartbeat interval)
	c.sendPendingControlPayloads()
}

// sendHeartbeatMessage sends one heartbeat with the loaded checksum.
func (c *SimpleEdgeClient) sendHeartbeatMessage() {
	loadedChecksum, loadedVersion := c.loadedConfig()
	heartbeat := &pb.EdgeMessage{
		Message: &pb.EdgeMessage_Heartbeat{
			Heartbeat: &pb.HeartbeatRequest{
				EdgeId:    c.config.HubSpoke.EdgeID,
				SessionId: "", // SimpleEdgeClient doesn't track session ID
				Health: &pb.HealthStatus{
					Status:    pb.HealthStatus_HEALTHY,
					Message:   "Operational",
					Timestamp: timestamppb.Now(),
					Metrics:   fipsinfo.Current().Metrics(), // FIPS 140-3 status, for the hub
				},
				Metrics:              c.collectBasicMetrics(),
				Timestamp:            timestamppb.Now(),
				LoadedConfigChecksum: loadedChecksum,
				LoadedConfigVersion:  loadedVersion,
			},
		},
	}
	c.configMu.RLock()
	matches := c.templateMatches
	hits := c.exceptionHits
	c.configMu.RUnlock()
	var matched, hit []uint32
	if matches != nil {
		matched = matches.DrainMatched()
		heartbeat.GetHeartbeat().MatchedPromptTemplateIds = matched
	}
	if hits != nil {
		hit = hits.DrainMatched()
		heartbeat.GetHeartbeat().HitGuardrailExceptionIds = hit
	}

	if err := c.send(heartbeat); err != nil {
		if len(matched) > 0 {
			matches.Requeue(matched)
		}
		if len(hit) > 0 {
			hits.Requeue(hit)
		}
		log.Error().Err(err).Msg("Failed to send heartbeat")
	} else {
		log.Debug().
			Str("edge_id", c.config.HubSpoke.EdgeID).
			Str("checksum", loadedChecksum).
			Msg("Heartbeat sent with config checksum")
	}
}

// sendPendingControlPayloads sends pending plugin control payloads to the control server
func (c *SimpleEdgeClient) sendPendingControlPayloads() {
	if c.controlPayloadQueue == nil {
		return
	}

	// Check if there are pending payloads
	pendingCount := c.controlPayloadQueue.GetPendingCount()
	if pendingCount == 0 {
		return
	}

	log.Debug().
		Int64("pending_count", pendingCount).
		Msg("Sending pending control payloads to control server")

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Send the batch
	if err := c.controlPayloadQueue.SendPendingBatch(ctx); err != nil {
		log.Error().Err(err).Msg("Failed to send control payload batch")
	}

	// Run periodic cleanup of old payloads
	if err := c.controlPayloadQueue.CleanupOldPayloads(); err != nil {
		log.Warn().Err(err).Msg("Failed to cleanup old control payloads")
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
	if !c.reconnecting.CompareAndSwap(false, true) {
		return // Already attempting reconnection
	}
	defer c.reconnecting.Store(false)
	// The stream whose end asked for this may have been replaced since: a
	// failback ends the stream itself and opens the next one while it holds
	// the guard, and the ended stream's request to reconnect can arrive
	// after the guard is free again. Reconnecting then would drop a
	// connection that is fine.
	if c.connected.Load() {
		return
	}
	c.reconnectLoop()
}

// reconnectLoop reconnects until it succeeds or the client stops. The caller
// holds c.reconnecting.
//
// With one control endpoint it waits out a backoff (doubling from
// EDGE_RECONNECT_INTERVAL, jittered, capped at 30 s) before every attempt.
// With several, the connection that just ended counts as that endpoint's
// failure: the others are tried at once, in list order after it, and only
// when all of them have failed does the backoff begin. From then on each
// backoff is followed by one pass over the whole list from its first entry,
// so the backoff applies per pass, not per endpoint.
func (c *SimpleEdgeClient) reconnectLoop() {
	c.reconnectAttempts = 0

	log.Debug().Msg("Starting automatic reconnection to control server with exponential backoff")

	// Initial backoff parameters
	baseDelay := c.reconnectInterval
	maxDelay := reconnectBackoffCap(baseDelay)
	backoffMultiplier := 2.0
	jitterFactor := 0.1

	if others := len(c.endpoints) - 1; others > 0 {
		// A short random wait first: when a control plane goes down, every
		// edge it held arrives at the next one, and should not in the same
		// instant.
		select {
		case <-time.After(time.Duration(rand.Float64() * jitterFactor * float64(baseDelay))):
		case <-c.stopCh:
			return
		}
		if c.tryEndpoints(int(c.endpointIdx.Load())+1, others) {
			return
		}
	}

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

		log.Debug().
			Int("attempt", c.reconnectAttempts).
			Dur("backoff_delay", backoffDelay).
			Msg("Attempting to reconnect to control server")

		// Wait with exponential backoff before attempting reconnection
		select {
		case <-time.After(backoffDelay):
			// Continue with reconnection attempt
		case <-c.stopCh:
			log.Debug().Msg("Reconnection cancelled due to client shutdown")
			return
		}

		// Stop() closes stopCh. A nil c.conn is not a stop signal: a failed
		// reconnectWithRetry clears it, and treating that as "manually closed"
		// left edges disconnected after any outage longer than one attempt.
		if c.stopping() {
			log.Debug().Msg("Client stopped, stopping reconnection attempts")
			return
		}

		// Attempt to re-establish connection and stream: one pass over the
		// endpoints, from the first.
		if !c.tryEndpoints(0, max(len(c.endpoints), 1)) {
			if c.stopping() {
				return
			}
			log.Error().
				Int("attempt", c.reconnectAttempts).
				Dur("next_retry_in", c.calculateBackoffDelay(baseDelay, maxDelay, backoffMultiplier, jitterFactor, c.reconnectAttempts+1)).
				Msg("Failed to reconnect, will retry with exponential backoff")
			continue
		}

		// Success!
		log.Debug().
			Int("attempts", c.reconnectAttempts).
			Msg("Successfully reconnected to control server")

		// Reset attempt counter
		c.reconnectAttempts = 0
		return
	}
}

// tryEndpoints tries count endpoints in list order, starting at index start
// (wrapping round), and reports whether one accepted the edge. It stops
// early when the client is stopped.
func (c *SimpleEdgeClient) tryEndpoints(start, count int) bool {
	n := max(len(c.endpoints), 1)
	for k := 0; k < count; k++ {
		if c.stopping() {
			return false
		}
		idx := (start + k) % n
		c.endpointIdx.Store(int32(idx))
		if err := c.reconnectWithRetry(); err != nil {
			log.Error().
				Err(err).
				Str("control_endpoint", c.currentEndpoint()).
				Int("attempt", c.reconnectAttempts).
				Msg("Failed to reconnect to control endpoint")
			continue
		}
		return true
	}
	return false
}

// maxReconnectBackoff caps the reconnect backoff (before jitter). The
// backoff doubles from EDGE_RECONNECT_INTERVAL; uncapped (it used to stop
// at 5 minutes) an edge could come back minutes after control did: with a
// 2 s interval and a 70 s outage, about a minute late. 30 s keeps a large
// fleet from hammering a recovering control plane while bounding how late
// an edge reconnects.
const maxReconnectBackoff = 30 * time.Second

// reconnectBackoffCap is the backoff cap for a reconnect interval: 30 s,
// or the interval itself if that is longer.
func reconnectBackoffCap(interval time.Duration) time.Duration {
	if interval > maxReconnectBackoff {
		return interval
	}
	return maxReconnectBackoff
}

// stopping reports whether Stop has been called.
func (c *SimpleEdgeClient) stopping() bool {
	select {
	case <-c.stopCh:
		return true
	default:
		return false
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
	if old, _ := c.connection(); old != nil {
		c.setConnection(nil, "")
		if err := old.Close(); err != nil {
			log.Debug().Err(err).Msg("Error closing broken connection during reconnect")
		}
	}

	// Re-establish gRPC connection, to the endpoint the loop is on
	endpoint := c.currentEndpoint()
	conn, err := c.dialWithKeepalive()
	if err != nil {
		return fmt.Errorf("failed to re-establish gRPC connection: %w", err)
	}
	if err := c.awaitReady(conn); err != nil {
		conn.Close()
		return fmt.Errorf("failed to re-establish gRPC connection: %w", err)
	}

	// Everything that talks to control (token validation, the analytics
	// pulse, the payload queue) picks the new client up from here.
	c.setConnection(conn, endpoint)

	// Re-wire the control payload queue's gRPC client if set
	if c.controlPayloadQueue != nil {
		c.controlPayloadQueue.SetGRPCClient(c.GetGRPCClient(), c.createAuthContext)
	}

	// Test connectivity by registering with control
	if err := c.registerWithControl(); err != nil {
		c.setConnection(nil, "")
		conn.Close()
		return fmt.Errorf("failed to re-register with control: %w", err)
	}

	// Re-establish stream
	if err := c.establishStream(); err != nil {
		c.setConnection(nil, "")
		conn.Close()
		return fmt.Errorf("failed to re-establish stream: %w", err)
	}

	log.Debug().Msg("Successfully reconnected to control server")
	c.logConnected(endpoint)
	return nil
}

// controlConnectTimeout bounds how long an edge with several control
// endpoints waits for a connection to one of them before it tries the next.
// Without it an endpoint that does not answer at all (a site whose network
// is gone, rather than one that refuses the connection) holds the edge for
// gRPC's own connect timeout of 20 s on every pass. An edge with a single
// endpoint has nowhere else to go and keeps gRPC's behaviour.
const controlConnectTimeout = 5 * time.Second

// awaitReady waits, on an edge with several control endpoints, until conn is
// connected, and fails when it is not within controlConnectTimeout.
func (c *SimpleEdgeClient) awaitReady(conn *grpc.ClientConn) error {
	if len(c.endpoints) < 2 {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), controlConnectTimeout)
	defer cancel()
	conn.Connect()
	for {
		state := conn.GetState()
		if state == connectivity.Ready {
			return nil
		}
		if state == connectivity.TransientFailure || state == connectivity.Shutdown {
			return fmt.Errorf("control endpoint %s is not reachable", conn.Target())
		}
		if !conn.WaitForStateChange(ctx, state) {
			return fmt.Errorf("control endpoint %s did not answer within %s", conn.Target(), controlConnectTimeout)
		}
	}
}

// failbackDrain is how long the connection an edge moved away from is kept
// open after a failback, so the calls in flight on it (token validations,
// an analytics pulse, a payload batch) finish instead of failing.
const failbackDrain = 30 * time.Second

// failbackProbeTimeout bounds the registration with which a failback asks
// the first endpoint whether it is back.
const failbackProbeTimeout = 10 * time.Second

// startFailback starts the failback worker, once, when
// EDGE_CONTROL_FAILBACK_INTERVAL is set and there is an endpoint to return
// to.
func (c *SimpleEdgeClient) startFailback() {
	if c.config == nil || c.config.HubSpoke.ControlFailbackInterval <= 0 || len(c.endpoints) < 2 {
		return
	}
	c.failbackOnce.Do(func() { go c.failbackWorker(c.config.HubSpoke.ControlFailbackInterval) })
}

// failbackWorker tries, every interval, to return an edge that is not on
// its first control endpoint to it.
func (c *SimpleEdgeClient) failbackWorker(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			c.tryFailback()
		case <-c.stopCh:
			return
		}
	}
}

// tryFailback moves the edge to its first control endpoint if it is
// connected to another one and the first accepts a registration. Requests
// are not dropped: token validation and the other calls keep using the
// connection the edge is leaving until the new one is registered, and that
// connection stays open for failbackDrain so calls in flight on it finish.
// Only the stream is briefly closed, as in any reconnect.
func (c *SimpleEdgeClient) tryFailback() {
	if c.endpointIdx.Load() == 0 || !c.connected.Load() || c.stopping() {
		return
	}
	if !c.reconnecting.CompareAndSwap(false, true) {
		return // a reconnect is under way; it starts from the first endpoint anyway
	}
	defer func() {
		c.reconnecting.Store(false)
		// A stream that ended while the failback held the guard found it
		// taken and did not start a reconnection: start it now.
		if !c.connected.Load() && !c.stopping() {
			go c.attemptReconnection()
		}
	}()
	if c.endpointIdx.Load() == 0 || !c.connected.Load() {
		return
	}

	first := c.endpoints[0]
	from := c.currentEndpoint()
	conn, err := c.dial(first)
	if err != nil {
		log.Debug().Err(err).Str("control_endpoint", first).Msg("Failback: cannot dial the first control endpoint")
		return
	}
	if err := c.awaitReady(conn); err != nil {
		conn.Close()
		log.Debug().Err(err).Str("control_endpoint", first).Msg("Failback: the first control endpoint is not reachable yet")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), failbackProbeTimeout)
	resp, err := c.registerOn(ctx, pb.NewConfigurationSyncServiceClient(conn))
	cancel()
	if err != nil {
		conn.Close()
		log.Debug().Err(err).Str("control_endpoint", first).Msg("Failback: the first control endpoint did not accept a registration yet")
		return
	}

	// The first endpoint is back. End the stream to the current one and
	// wait for its receive loop to finish (it clears the state the new
	// stream is about to set); it finds the reconnect guard held and
	// leaves the reconnecting to us.
	log.Info().Str("from", from).Str("control_endpoint", first).Msg("First control endpoint is back, moving to it")
	old, _ := c.connection()
	c.streamMu.RLock()
	done := c.streamDone
	c.streamMu.RUnlock()
	if c.streamCancel != nil {
		c.streamCancel()
	}
	if done != nil {
		select {
		case <-done:
		case <-c.stopCh:
			conn.Close()
			return
		}
	}

	c.endpointIdx.Store(0)
	c.setConnection(conn, first)
	if c.controlPayloadQueue != nil {
		c.controlPayloadQueue.SetGRPCClient(c.GetGRPCClient(), c.createAuthContext)
	}
	c.applyRegistration(resp)
	if err := c.establishStream(); err != nil {
		// The first endpoint went away again. Reconnect as after any lost
		// stream, starting with the other endpoints.
		log.Warn().Err(err).Str("control_endpoint", first).Msg("Failback: could not open a stream on the first control endpoint, reconnecting")
		c.setConnection(nil, "")
		conn.Close()
		if old != nil {
			old.Close()
		}
		c.reconnectLoop()
		return
	}
	c.logConnected(first)

	if old != nil {
		go func() {
			select {
			case <-time.After(failbackDrain):
			case <-c.stopCh:
			}
			old.Close()
		}()
	}
}

// dialWithKeepalive dials the control endpoint the client is on.
func (c *SimpleEdgeClient) dialWithKeepalive() (*grpc.ClientConn, error) {
	return c.dial(c.currentEndpoint())
}

// dial creates a gRPC connection to endpoint with proper keepalive settings.
// With TLS, the certificate is checked against endpoint's host unless
// EDGE_TLS_SERVER_NAME names another; the CA and the client certificate are
// the same for every endpoint.
func (c *SimpleEdgeClient) dial(endpoint string) (*grpc.ClientConn, error) {
	if endpoint == "" {
		return nil, fmt.Errorf("no control endpoint configured (CONTROL_ENDPOINTS or CONTROL_ENDPOINT)")
	}
	// Configure keepalive settings for reliable bidirectional streaming
	keepaliveParams := keepalive.ClientParameters{
		Time:                30 * time.Second, // Send pings every 30 seconds if no activity
		Timeout:             5 * time.Second,  // Wait 5 seconds for ping response
		PermitWithoutStream: true,             // Send pings even without active streams
	}

	// Configure max message size (default 16MB, configurable via GRPC_MAX_MESSAGE_SIZE)
	maxMsgSize := c.config.HubSpoke.MaxMessageSize
	if maxMsgSize <= 0 {
		maxMsgSize = 16 * 1024 * 1024 // 16MB fallback
	}

	// Setup dial options
	opts := []grpc.DialOption{
		grpc.WithKeepaliveParams(keepaliveParams),
		grpc.WithUnaryInterceptor(c.authUnaryInterceptor()),
		grpc.WithStreamInterceptor(c.authStreamInterceptor()),
		// Config snapshots can exceed the gRPC default of 4MB with many plugins, LLMs, tools, and apps
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(maxMsgSize)),
	}

	// Configure transport credentials (TLS or insecure)
	if c.config.HubSpoke.ClientTLSEnabled {
		tlsConfig, err := clientTLSConfig(c.config.HubSpoke)
		if err != nil {
			return nil, err
		}
		if c.config.HubSpoke.SkipTLSVerify {
			log.Warn().Msg("🔒 SECURITY: TLS certificate verification disabled - not recommended for production")
		}

		opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)))
		log.Debug().Bool("skip_verify", c.config.HubSpoke.SkipTLSVerify).Msg("Using secure TLS credentials for gRPC connection")
	} else {
		// Security: Require explicit opt-in for insecure connections
		if !c.config.HubSpoke.AllowInsecure {
			return nil, fmt.Errorf("🔒 SECURITY: Insecure gRPC connections are disabled by default. To enable for development, set EDGE_ALLOW_INSECURE=true")
		}
		log.Warn().Msg("⚠️  SECURITY WARNING: Using insecure credentials for gRPC connection - DEVELOPMENT ONLY")
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	// Dial with keepalive settings
	conn, err := grpc.Dial(endpoint, opts...)
	if err != nil {
		return nil, err
	}

	return conn, nil
}

// createAuthContext creates a context with authentication metadata
func (c *SimpleEdgeClient) createAuthContext(ctx context.Context) context.Context {
	if c.config.HubSpoke.ClientToken == "" {
		log.Warn().Msg("No authentication token configured for edge client - requests may be rejected")
		return ctx
	}

	// Add Bearer token to metadata
	md := metadata.New(map[string]string{
		"authorization": "Bearer " + c.config.HubSpoke.ClientToken,
	})

	return metadata.NewOutgoingContext(ctx, md)
}

// authUnaryInterceptor returns a unary client interceptor that adds authentication metadata
func (c *SimpleEdgeClient) authUnaryInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		// Add authentication metadata to context
		authCtx := c.createAuthContext(ctx)
		// Invoke the actual RPC with authenticated context
		return invoker(authCtx, method, req, reply, cc, opts...)
	}
}

// authStreamInterceptor returns a stream client interceptor that adds authentication metadata
func (c *SimpleEdgeClient) authStreamInterceptor() grpc.StreamClientInterceptor {
	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		// Add authentication metadata to context
		authCtx := c.createAuthContext(ctx)
		// Create the stream with authenticated context
		return streamer(authCtx, desc, cc, method, opts...)
	}
}
// serialSendStream lets one goroutine at a time Send on a stream: gRPC
// streams do not allow concurrent Send calls, and heartbeats, events and
// reload statuses are sent from different goroutines. A torn or dropped
// reload status would leave control waiting for an answer that never comes.
type serialSendStream struct {
	pb.ConfigurationSyncService_SubscribeToChangesClient
	mu sync.Mutex
}

func (s *serialSendStream) Send(msg *pb.EdgeMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ConfigurationSyncService_SubscribeToChangesClient.Send(msg)
}

func (c *SimpleEdgeClient) setStream(stream pb.ConfigurationSyncService_SubscribeToChangesClient) {
	if stream != nil {
		if _, ok := stream.(*serialSendStream); !ok {
			stream = &serialSendStream{ConfigurationSyncService_SubscribeToChangesClient: stream}
		}
	}
	c.streamMu.Lock()
	c.stream = stream
	c.streamMu.Unlock()
}

func (c *SimpleEdgeClient) currentStream() pb.ConfigurationSyncService_SubscribeToChangesClient {
	c.streamMu.RLock()
	defer c.streamMu.RUnlock()
	return c.stream
}

// send writes msg on the current stream.
func (c *SimpleEdgeClient) send(msg *pb.EdgeMessage) error {
	stream := c.currentStream()
	if stream == nil {
		return fmt.Errorf("no stream to control")
	}
	return stream.Send(msg)
}
