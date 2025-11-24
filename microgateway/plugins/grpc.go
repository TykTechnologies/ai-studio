// plugins/grpc.go
package plugins

import (
	"context"
	"fmt"
	"net/rpc"

	pb "github.com/TykTechnologies/midsommar/v2/proto"
	mgmtpb "github.com/TykTechnologies/midsommar/microgateway/proto/microgateway_management"
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

	// Allocate broker ID and start brokered server
	brokerID := c.broker.NextId()

	log.Info().
		Uint32("broker_id", brokerID).
		Msg("Setting up long-lived brokered server for microgateway service API access")

	// Start brokered server with microgateway management services
	go c.broker.AcceptAndServe(brokerID, func(opts []grpc.ServerOption) *grpc.Server {
		s := grpc.NewServer(opts...)

		// Register microgateway management services on brokered server
		mgmtpb.RegisterMicrogatewayManagementServiceServer(s, mgmtServer)
		log.Info().
			Uint32("broker_id", brokerID).
			Msg("✅ Microgateway management services registered on long-lived brokered server")

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