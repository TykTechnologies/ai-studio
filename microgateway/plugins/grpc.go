// plugins/grpc.go
package plugins

import (
	"context"
	"net/rpc"

	pb "github.com/TykTechnologies/midsommar/microgateway/plugins/proto"
	"github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
)

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
func (p *PluginGRPC) GRPCClient(ctx context.Context, broker *plugin.GRPCBroker, c *grpc.ClientConn) (interface{}, error) {
	return pb.NewPluginServiceClient(c), nil
}