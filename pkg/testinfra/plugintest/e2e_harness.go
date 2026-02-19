// Package plugintest provides E2E testing utilities for plugins.
// The E2EPluginHarness spawns real plugin subprocesses via go-plugin,
// enabling true integration tests that mirror production behavior.
package plugintest

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"sync"
	"time"

	gwmgmtpb "github.com/TykTechnologies/midsommar/microgateway/proto/microgateway_management"
	"github.com/TykTechnologies/midsommar/v2/pkg/plugin_sdk"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	mgmtpb "github.com/TykTechnologies/midsommar/v2/proto/ai_studio_management"
	eventpb "github.com/TykTechnologies/midsommar/v2/proto/plugin_events"
	goplugin "github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
)

// E2EPluginHarness spawns a real plugin subprocess and communicates via gRPC.
// This mirrors how AI Studio and Microgateway load plugins in production.
type E2EPluginHarness struct {
	pluginPath string // Path to compiled plugin binary

	// go-plugin state
	client       *goplugin.Client
	pluginClient pb.PluginServiceClient
	broker       *goplugin.GRPCBroker

	// Test service implementations (host-side)
	testServer        *TestManagementServer        // AI Studio management service
	testGatewayServer *TestGatewayManagementServer // Microgateway management service
	testEventSvc      *TestEventService
	brokerID          uint32
	sessionStarted    bool

	// Configuration
	runtime      plugin_sdk.RuntimeType
	config       map[string]string
	pluginConfig map[string]string

	// gRPC server for brokered services
	grpcServer   *grpc.Server
	grpcListener net.Listener

	mu sync.RWMutex
}

// NewE2EHarness creates a harness that will spawn the plugin as a subprocess.
func NewE2EHarness(pluginBinaryPath string) *E2EPluginHarness {
	return &E2EPluginHarness{
		pluginPath:        pluginBinaryPath,
		testServer:        NewTestManagementServer(),
		testGatewayServer: NewTestGatewayManagementServer(),
		testEventSvc:      NewTestEventService(),
		runtime:           plugin_sdk.RuntimeStudio,
		config:            make(map[string]string),
		pluginConfig:      make(map[string]string),
	}
}

// SetLicense configures what GetLicenseInfo will return.
func (h *E2EPluginHarness) SetLicense(licenseType string, valid bool, daysRemaining int) {
	h.mu.Lock()
	defer h.mu.Unlock()

	license := &LicenseInfo{
		Valid:         valid,
		DaysRemaining: daysRemaining,
		Type:          licenseType,
		Entitlements:  []string{},
		Organization:  "Test Organization",
	}
	h.testServer.license = license
	h.testGatewayServer.license = license
}

// SetEntitlements sets the license entitlements.
func (h *E2EPluginHarness) SetEntitlements(entitlements []string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.testServer.license == nil {
		h.testServer.license = &LicenseInfo{}
	}
	h.testServer.license.Entitlements = entitlements

	if h.testGatewayServer.license == nil {
		h.testGatewayServer.license = &LicenseInfo{}
	}
	h.testGatewayServer.license.Entitlements = entitlements
}

// SetRuntime sets the runtime type (Studio or Gateway).
func (h *E2EPluginHarness) SetRuntime(rt plugin_sdk.RuntimeType) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.runtime = rt
}

// SetKVData pre-populates the test KV store.
func (h *E2EPluginHarness) SetKVData(key string, value []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.testServer.kvStore[key] = value
	h.testGatewayServer.kvStore[key] = value
}

// Start spawns the plugin subprocess and establishes gRPC connection.
func (h *E2EPluginHarness) Start() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Check plugin binary exists
	if _, err := os.Stat(h.pluginPath); os.IsNotExist(err) {
		return fmt.Errorf("plugin binary not found: %s", h.pluginPath)
	}

	// Set runtime environment variable
	env := os.Environ()
	switch h.runtime {
	case plugin_sdk.RuntimeGateway:
		env = append(env, "PLUGIN_RUNTIME=gateway")
	default:
		env = append(env, "PLUGIN_RUNTIME=studio")
	}

	// Create command with environment
	cmd := exec.Command(h.pluginPath)
	cmd.Env = env

	// Configure go-plugin client
	h.client = goplugin.NewClient(&goplugin.ClientConfig{
		HandshakeConfig: goplugin.HandshakeConfig{
			ProtocolVersion:  1,
			MagicCookieKey:   "AI_STUDIO_PLUGIN",
			MagicCookieValue: "v1",
		},
		Plugins: map[string]goplugin.Plugin{
			"plugin": &TestPluginGRPC{harness: h},
		},
		Cmd:              cmd,
		AllowedProtocols: []goplugin.Protocol{goplugin.ProtocolGRPC},
		Managed:          true,
	})

	// Start and connect
	rpcClient, err := h.client.Client()
	if err != nil {
		return fmt.Errorf("failed to create plugin client: %w", err)
	}

	// Get the plugin client
	raw, err := rpcClient.Dispense("plugin")
	if err != nil {
		return fmt.Errorf("failed to dispense plugin: %w", err)
	}

	// Extract the gRPC client
	if client, ok := raw.(*TestPluginClient); ok {
		h.pluginClient = client.stub
		h.broker = client.broker
	} else {
		return fmt.Errorf("unexpected plugin client type: %T", raw)
	}

	return nil
}

// Stop terminates the plugin subprocess.
func (h *E2EPluginHarness) Stop() {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.grpcServer != nil {
		h.grpcServer.Stop()
	}
	if h.grpcListener != nil {
		h.grpcListener.Close()
	}
	if h.client != nil {
		h.client.Kill()
	}
}

// ProcessExited returns true if the plugin process has exited.
func (h *E2EPluginHarness) ProcessExited() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if h.client == nil {
		return true
	}
	return h.client.Exited()
}

// Initialize calls the plugin's Initialize method.
func (h *E2EPluginHarness) Initialize(config map[string]string) error {
	h.mu.Lock()
	h.pluginConfig = config
	h.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := h.pluginClient.Initialize(ctx, &pb.InitRequest{
		Config: config,
	})
	if err != nil {
		return fmt.Errorf("initialize failed: %w", err)
	}
	if !resp.Success {
		return fmt.Errorf("initialize failed: %s", resp.ErrorMessage)
	}
	return nil
}

// OpenSession establishes the service broker session (triggers OnSessionReady).
func (h *E2EPluginHarness) OpenSession() error {
	h.mu.Lock()
	if h.sessionStarted {
		h.mu.Unlock()
		return nil
	}
	h.mu.Unlock()

	// Start brokered gRPC server for host services
	if err := h.startBrokeredServer(); err != nil {
		return fmt.Errorf("failed to start brokered server: %w", err)
	}

	// Open session with the broker ID
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Open session in a goroutine since it blocks until timeout or close
	errCh := make(chan error, 1)
	go func() {
		_, err := h.pluginClient.OpenSession(ctx, &pb.OpenSessionRequest{
			ServiceBrokerId: h.brokerID,
			TimeoutMs:       30000,
		})
		errCh <- err
	}()

	// Give the plugin time to connect and call OnSessionReady
	select {
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("open session failed: %w", err)
		}
	case <-time.After(2 * time.Second):
		// Session is running, plugin has had time to initialize
	}

	h.mu.Lock()
	h.sessionStarted = true
	h.mu.Unlock()

	return nil
}

// startBrokeredServer starts the gRPC server for host services.
func (h *E2EPluginHarness) startBrokeredServer() error {
	// Use the broker to get a connection ID
	h.brokerID = h.broker.NextId()

	// Start our gRPC server that plugins will connect to
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("failed to create listener: %w", err)
	}
	h.grpcListener = listener

	h.grpcServer = grpc.NewServer()

	// Register management service based on runtime type
	if h.runtime == plugin_sdk.RuntimeGateway {
		// Gateway runtime uses MicrogatewayManagementService
		gwmgmtpb.RegisterMicrogatewayManagementServiceServer(h.grpcServer, h.testGatewayServer)
	} else {
		// Studio runtime uses AIStudioManagementService
		mgmtpb.RegisterAIStudioManagementServiceServer(h.grpcServer, h.testServer)
	}

	// Register event service (same for both runtimes)
	eventpb.RegisterPluginEventServiceServer(h.grpcServer, h.testEventSvc)

	// Start server in background
	go h.grpcServer.Serve(listener)

	// Tell broker to accept connections at this address
	go h.broker.AcceptAndServe(h.brokerID, func(opts []grpc.ServerOption) *grpc.Server {
		s := grpc.NewServer(opts...)
		if h.runtime == plugin_sdk.RuntimeGateway {
			gwmgmtpb.RegisterMicrogatewayManagementServiceServer(s, h.testGatewayServer)
		} else {
			mgmtpb.RegisterAIStudioManagementServiceServer(s, h.testServer)
		}
		eventpb.RegisterPluginEventServiceServer(s, h.testEventSvc)
		return s
	})

	return nil
}

// CallRPC invokes HandleRPC on the plugin via the Call gRPC method.
func (h *E2EPluginHarness) CallRPC(method string, payload []byte) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := h.pluginClient.Call(ctx, &pb.CallRequest{
		Method:  method,
		Payload: string(payload),
	})
	if err != nil {
		return nil, fmt.Errorf("RPC call failed: %w", err)
	}
	if !resp.Success {
		return nil, fmt.Errorf("RPC call failed: %s", resp.ErrorMessage)
	}
	return []byte(resp.Data), nil
}

// CallPostAuth invokes HandlePostAuth on the plugin.
func (h *E2EPluginHarness) CallPostAuth(req *pb.EnrichedRequest) (*pb.PluginResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	return h.pluginClient.ProcessPostAuth(ctx, req)
}

// CallOnBeforeWrite invokes OnBeforeWrite on the plugin.
func (h *E2EPluginHarness) CallOnBeforeWrite(req *pb.ResponseWriteRequest) (*pb.ResponseWriteResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	return h.pluginClient.OnBeforeWrite(ctx, req)
}

// CallOnStreamComplete invokes OnStreamComplete on the plugin.
func (h *E2EPluginHarness) CallOnStreamComplete(req *pb.StreamCompleteRequest) (*pb.StreamCompleteResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	return h.pluginClient.OnStreamComplete(ctx, req)
}

// LicenseWasChecked returns true if the plugin called GetLicenseInfo.
func (h *E2EPluginHarness) LicenseWasChecked() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if h.runtime == plugin_sdk.RuntimeGateway {
		return h.testGatewayServer.licenseChecked
	}
	return h.testServer.licenseChecked
}

// GetKVWrites returns all KV writes made by the plugin.
func (h *E2EPluginHarness) GetKVWrites() []KVWrite {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if h.runtime == plugin_sdk.RuntimeGateway {
		return h.testGatewayServer.kvWrites
	}
	return h.testServer.kvWrites
}

// GatewayServer returns the test gateway management server for configuration.
// This allows tests to configure gateway-specific test data like apps, LLMs, budgets.
func (h *E2EPluginHarness) GatewayServer() *TestGatewayManagementServer {
	return h.testGatewayServer
}

// GetPublishedEvents returns all events published by the plugin.
func (h *E2EPluginHarness) GetPublishedEvents() []Event {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.testEventSvc.GetPublishedEvents()
}

// InjectEvent simulates receiving an event from the event bus.
func (h *E2EPluginHarness) InjectEvent(topic string, payload []byte) {
	h.testEventSvc.InjectEvent(topic, payload)
}

// ============================================================================
// go-plugin implementation types
// ============================================================================

// TestPluginGRPC implements goplugin.Plugin for test harness.
type TestPluginGRPC struct {
	goplugin.NetRPCUnsupportedPlugin
	harness *E2EPluginHarness
}

func (p *TestPluginGRPC) GRPCServer(broker *goplugin.GRPCBroker, s *grpc.Server) error {
	return nil
}

func (p *TestPluginGRPC) GRPCClient(ctx context.Context, broker *goplugin.GRPCBroker, c *grpc.ClientConn) (interface{}, error) {
	return &TestPluginClient{
		stub:   pb.NewPluginServiceClient(c),
		broker: broker,
	}, nil
}

// TestPluginClient wraps the plugin gRPC client.
type TestPluginClient struct {
	stub   pb.PluginServiceClient
	broker *goplugin.GRPCBroker
}

// ============================================================================
// Test data types
// ============================================================================

// LicenseInfo represents license configuration for tests.
type LicenseInfo struct {
	Valid         bool
	DaysRemaining int
	Type          string
	Entitlements  []string
	Organization  string
}

// KVWrite represents a KV write operation for tracking.
type KVWrite struct {
	Key       string
	Value     []byte
	ExpireAt  *time.Time
	Timestamp time.Time
}

// Event represents a published event for tracking.
type Event struct {
	Topic     string
	Payload   json.RawMessage
	Direction int32
	Timestamp time.Time
}
