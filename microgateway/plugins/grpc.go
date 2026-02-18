// plugins/grpc.go
package plugins

import (
	"context"
	"fmt"
	"net/rpc"

	mgmtpb "github.com/TykTechnologies/midsommar/microgateway/proto/microgateway_management"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	eventpb "github.com/TykTechnologies/midsommar/v2/proto/plugin_events"
	"github.com/hashicorp/go-plugin"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
)

// Global reference to service interfaces for broker setup
var globalServiceInterfaces ServiceInterfaces

// ServiceInterfaces holds references to microgateway services for broker setup
type ServiceInterfaces struct {
	ManagementServer *mgmtpb.UnimplementedMicrogatewayManagementServiceServer
}

// SetServiceInterfaces sets the global service references for broker setup
func SetServiceInterfaces(interfaces ServiceInterfaces) {
	globalServiceInterfaces = interfaces
}

// PluginGRPC implements the hashicorp/go-plugin interface for external plugin binaries
type PluginGRPC struct {
	plugin.Plugin
}

// Server returns the gRPC server for this plugin (not used for external plugins)
func (p *PluginGRPC) Server(*plugin.MuxBroker) (interface{}, error) {
	return nil, nil
}

// Client returns the gRPC client for connecting to external plugin binaries
func (p *PluginGRPC) Client(b *plugin.MuxBroker, c *rpc.Client) (interface{}, error) {
	return nil, nil
}

// GRPCServer returns the gRPC server (not used for external plugins)
func (p *PluginGRPC) GRPCServer(broker *plugin.GRPCBroker, s *grpc.Server) error {
	return nil
}

// GRPCClient returns the gRPC client for external plugin binaries
// Now captures the broker for bidirectional communication
func (p *PluginGRPC) GRPCClient(ctx context.Context, broker *plugin.GRPCBroker, c *grpc.ClientConn) (interface{}, error) {
	// Return client wrapper that stores broker for host-side service setup
	return &MicrogatewayPluginClient{
		broker:     broker,
		pluginStub: pb.NewPluginServiceClient(c),
	}, nil
}

// MicrogatewayPluginClient wraps the plugin client with broker access for host service setup
type MicrogatewayPluginClient struct {
	broker     *plugin.GRPCBroker
	pluginStub pb.PluginServiceClient
}

// SetupServiceBroker creates a long-lived brokered server for microgateway services
// Returns the broker ID that the plugin can use to dial back to host services
func (c *MicrogatewayPluginClient) SetupServiceBroker(managementServer interface{}) (uint32, error) {
	return c.SetupServiceBrokerWithEvents(managementServer, nil)
}

// SetupServiceBrokerWithEvents creates a long-lived brokered server for microgateway services
// including the plugin event service for pub/sub.
// Returns the broker ID that the plugin can use to dial back to host services.
func (c *MicrogatewayPluginClient) SetupServiceBrokerWithEvents(managementServer interface{}, eventServer interface{}) (uint32, error) {
	if c.broker == nil {
		return 0, fmt.Errorf("broker not available")
	}

	if managementServer == nil {
		return 0, fmt.Errorf("management server not available")
	}

	// Cast to correct type
	mgmtServer, ok := managementServer.(mgmtpb.MicrogatewayManagementServiceServer)
	if !ok {
		return 0, fmt.Errorf("invalid management server type - does not implement MicrogatewayManagementServiceServer")
	}

	// Cast event server if provided
	var evtServer eventpb.PluginEventServiceServer
	if eventServer != nil {
		evtServer, ok = eventServer.(eventpb.PluginEventServiceServer)
		if !ok {
			return 0, fmt.Errorf("invalid event server type - does not implement PluginEventServiceServer")
		}
	}

	// Allocate broker ID and start brokered server
	brokerID := c.broker.NextId()

	log.Debug().
		Uint32("broker_id", brokerID).
		Bool("has_event_server", evtServer != nil).
		Msg("Setting up long-lived brokered server for microgateway service API access")

	// Start brokered server with microgateway management services
	// Note: AcceptAndServe blocks, so we run it in a goroutine.
	// The go-plugin broker handles synchronization internally - the plugin
	// will wait for connection info when it calls Dial().
	go c.broker.AcceptAndServe(brokerID, func(opts []grpc.ServerOption) *grpc.Server {
		s := grpc.NewServer(opts...)

		// Register microgateway management services on brokered server
		mgmtpb.RegisterMicrogatewayManagementServiceServer(s, mgmtServer)
		log.Debug().
			Uint32("broker_id", brokerID).
			Msg("Microgateway management services registered on brokered server")

		// Register plugin event service if provided
		if evtServer != nil {
			eventpb.RegisterPluginEventServiceServer(s, evtServer)
			log.Debug().
				Uint32("broker_id", brokerID).
				Msg("Plugin event service registered on brokered server")
		} else {
			log.Warn().
				Uint32("broker_id", brokerID).
				Msg("No event server provided - plugins will NOT be able to subscribe to events")
		}

		return s
	})

	return brokerID, nil
}

// Delegate all PluginServiceClient methods to the plugin stub (with correct signatures)
func (c *MicrogatewayPluginClient) Initialize(ctx context.Context, req *pb.InitRequest, opts ...grpc.CallOption) (*pb.InitResponse, error) {
	return c.pluginStub.Initialize(ctx, req, opts...)
}

func (c *MicrogatewayPluginClient) Ping(ctx context.Context, req *pb.PingRequest, opts ...grpc.CallOption) (*pb.PingResponse, error) {
	return c.pluginStub.Ping(ctx, req, opts...)
}

func (c *MicrogatewayPluginClient) Shutdown(ctx context.Context, req *pb.ShutdownRequest, opts ...grpc.CallOption) (*pb.ShutdownResponse, error) {
	return c.pluginStub.Shutdown(ctx, req, opts...)
}

func (c *MicrogatewayPluginClient) GetManifest(ctx context.Context, req *pb.GetManifestRequest, opts ...grpc.CallOption) (*pb.GetManifestResponse, error) {
	return c.pluginStub.GetManifest(ctx, req, opts...)
}

func (c *MicrogatewayPluginClient) GetConfigSchema(ctx context.Context, req *pb.GetConfigSchemaRequest, opts ...grpc.CallOption) (*pb.GetConfigSchemaResponse, error) {
	return c.pluginStub.GetConfigSchema(ctx, req, opts...)
}

func (c *MicrogatewayPluginClient) GetAsset(ctx context.Context, req *pb.GetAssetRequest, opts ...grpc.CallOption) (*pb.GetAssetResponse, error) {
	return c.pluginStub.GetAsset(ctx, req, opts...)
}

func (c *MicrogatewayPluginClient) ListAssets(ctx context.Context, req *pb.ListAssetsRequest, opts ...grpc.CallOption) (*pb.ListAssetsResponse, error) {
	return c.pluginStub.ListAssets(ctx, req, opts...)
}

func (c *MicrogatewayPluginClient) Call(ctx context.Context, req *pb.CallRequest, opts ...grpc.CallOption) (*pb.CallResponse, error) {
	return c.pluginStub.Call(ctx, req, opts...)
}

func (c *MicrogatewayPluginClient) PortalCall(ctx context.Context, req *pb.PortalCallRequest, opts ...grpc.CallOption) (*pb.PortalCallResponse, error) {
	return c.pluginStub.PortalCall(ctx, req, opts...)
}

func (c *MicrogatewayPluginClient) ProcessPreAuth(ctx context.Context, req *pb.PluginRequest, opts ...grpc.CallOption) (*pb.PluginResponse, error) {
	return c.pluginStub.ProcessPreAuth(ctx, req, opts...)
}

func (c *MicrogatewayPluginClient) Authenticate(ctx context.Context, req *pb.AuthRequest, opts ...grpc.CallOption) (*pb.AuthResponse, error) {
	return c.pluginStub.Authenticate(ctx, req, opts...)
}

func (c *MicrogatewayPluginClient) GetAppByCredential(ctx context.Context, req *pb.GetAppRequest, opts ...grpc.CallOption) (*pb.GetAppResponse, error) {
	return c.pluginStub.GetAppByCredential(ctx, req, opts...)
}

func (c *MicrogatewayPluginClient) GetUserByCredential(ctx context.Context, req *pb.GetUserRequest, opts ...grpc.CallOption) (*pb.GetUserResponse, error) {
	return c.pluginStub.GetUserByCredential(ctx, req, opts...)
}

func (c *MicrogatewayPluginClient) ProcessPostAuth(ctx context.Context, req *pb.EnrichedRequest, opts ...grpc.CallOption) (*pb.PluginResponse, error) {
	return c.pluginStub.ProcessPostAuth(ctx, req, opts...)
}

func (c *MicrogatewayPluginClient) OnBeforeWriteHeaders(ctx context.Context, req *pb.HeadersRequest, opts ...grpc.CallOption) (*pb.HeadersResponse, error) {
	return c.pluginStub.OnBeforeWriteHeaders(ctx, req, opts...)
}

func (c *MicrogatewayPluginClient) OnBeforeWrite(ctx context.Context, req *pb.ResponseWriteRequest, opts ...grpc.CallOption) (*pb.ResponseWriteResponse, error) {
	return c.pluginStub.OnBeforeWrite(ctx, req, opts...)
}

func (c *MicrogatewayPluginClient) OnStreamComplete(ctx context.Context, req *pb.StreamCompleteRequest, opts ...grpc.CallOption) (*pb.StreamCompleteResponse, error) {
	return c.pluginStub.OnStreamComplete(ctx, req, opts...)
}

func (c *MicrogatewayPluginClient) HandleProxyLog(ctx context.Context, req *pb.ProxyLogRequest, opts ...grpc.CallOption) (*pb.DataCollectionResponse, error) {
	return c.pluginStub.HandleProxyLog(ctx, req, opts...)
}

func (c *MicrogatewayPluginClient) HandleAnalytics(ctx context.Context, req *pb.AnalyticsRequest, opts ...grpc.CallOption) (*pb.DataCollectionResponse, error) {
	return c.pluginStub.HandleAnalytics(ctx, req, opts...)
}

func (c *MicrogatewayPluginClient) HandleBudgetUsage(ctx context.Context, req *pb.BudgetUsageRequest, opts ...grpc.CallOption) (*pb.DataCollectionResponse, error) {
	return c.pluginStub.HandleBudgetUsage(ctx, req, opts...)
}

func (c *MicrogatewayPluginClient) HandleAgentMessage(ctx context.Context, req *pb.AgentMessageRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[pb.AgentMessageChunk], error) {
	return c.pluginStub.HandleAgentMessage(ctx, req, opts...)
}

func (c *MicrogatewayPluginClient) GetObjectHookRegistrations(ctx context.Context, req *pb.GetObjectHookRegistrationsRequest, opts ...grpc.CallOption) (*pb.GetObjectHookRegistrationsResponse, error) {
	return c.pluginStub.GetObjectHookRegistrations(ctx, req, opts...)
}

func (c *MicrogatewayPluginClient) HandleObjectHook(ctx context.Context, req *pb.ObjectHookRequest, opts ...grpc.CallOption) (*pb.ObjectHookResponse, error) {
	return c.pluginStub.HandleObjectHook(ctx, req, opts...)
}

func (c *MicrogatewayPluginClient) ExecuteScheduledTask(ctx context.Context, req *pb.ExecuteScheduledTaskRequest, opts ...grpc.CallOption) (*pb.ExecuteScheduledTaskResponse, error) {
	return c.pluginStub.ExecuteScheduledTask(ctx, req, opts...)
}

func (c *MicrogatewayPluginClient) AcceptEdgePayload(ctx context.Context, req *pb.EdgePayloadRequest, opts ...grpc.CallOption) (*pb.EdgePayloadResponse, error) {
	return c.pluginStub.AcceptEdgePayload(ctx, req, opts...)
}

// OpenSession opens a long-lived session for broker access.
// This blocks until timeout or CloseSession is called.
func (c *MicrogatewayPluginClient) OpenSession(ctx context.Context, req *pb.OpenSessionRequest, opts ...grpc.CallOption) (*pb.OpenSessionResponse, error) {
	return c.pluginStub.OpenSession(ctx, req, opts...)
}

// CloseSession explicitly closes an active session.
func (c *MicrogatewayPluginClient) CloseSession(ctx context.Context, req *pb.CloseSessionRequest, opts ...grpc.CallOption) (*pb.CloseSessionResponse, error) {
	return c.pluginStub.CloseSession(ctx, req, opts...)
}

// GetEndpointRegistrations queries the plugin for custom endpoint registrations.
func (c *MicrogatewayPluginClient) GetEndpointRegistrations(ctx context.Context, req *pb.GetEndpointRegistrationsRequest, opts ...grpc.CallOption) (*pb.GetEndpointRegistrationsResponse, error) {
	return c.pluginStub.GetEndpointRegistrations(ctx, req, opts...)
}

// HandleEndpointRequest forwards an HTTP request to the plugin's custom endpoint handler.
func (c *MicrogatewayPluginClient) HandleEndpointRequest(ctx context.Context, req *pb.EndpointRequest, opts ...grpc.CallOption) (*pb.EndpointResponse, error) {
	return c.pluginStub.HandleEndpointRequest(ctx, req, opts...)
}

// HandleEndpointRequestStream forwards a streaming HTTP request to the plugin.
func (c *MicrogatewayPluginClient) HandleEndpointRequestStream(ctx context.Context, req *pb.EndpointRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[pb.EndpointResponseChunk], error) {
	return c.pluginStub.HandleEndpointRequestStream(ctx, req, opts...)
}

// --- Resource Provider Methods (pass-through, gateway doesn't use these directly) ---

func (c *MicrogatewayPluginClient) GetResourceTypeRegistrations(ctx context.Context, req *pb.GetResourceTypeRegistrationsRequest, opts ...grpc.CallOption) (*pb.GetResourceTypeRegistrationsResponse, error) {
	return c.pluginStub.GetResourceTypeRegistrations(ctx, req, opts...)
}

func (c *MicrogatewayPluginClient) ListResourceInstances(ctx context.Context, req *pb.ListResourceInstancesRequest, opts ...grpc.CallOption) (*pb.ListResourceInstancesResponse, error) {
	return c.pluginStub.ListResourceInstances(ctx, req, opts...)
}

func (c *MicrogatewayPluginClient) GetResourceInstance(ctx context.Context, req *pb.GetResourceInstanceRequest, opts ...grpc.CallOption) (*pb.GetResourceInstanceResponse, error) {
	return c.pluginStub.GetResourceInstance(ctx, req, opts...)
}

func (c *MicrogatewayPluginClient) ValidateResourceSelection(ctx context.Context, req *pb.ValidateResourceSelectionRequest, opts ...grpc.CallOption) (*pb.ValidateResourceSelectionResponse, error) {
	return c.pluginStub.ValidateResourceSelection(ctx, req, opts...)
}

func (c *MicrogatewayPluginClient) CreateResourceInstance(ctx context.Context, req *pb.CreateResourceInstanceRequest, opts ...grpc.CallOption) (*pb.CreateResourceInstanceResponse, error) {
	return c.pluginStub.CreateResourceInstance(ctx, req, opts...)
}