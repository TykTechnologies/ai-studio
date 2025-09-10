// plugins/sdk/grpc_wrappers.go
package sdk

import (
	"context"

	"github.com/TykTechnologies/midsommar/microgateway/plugins/interfaces"
	pb "github.com/TykTechnologies/midsommar/microgateway/plugins/proto"
	"github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
)

// PreAuthPluginGRPC implements the plugin.Plugin interface for pre-auth plugins
type PreAuthPluginGRPC struct {
	plugin.Plugin
	Impl interfaces.PreAuthPlugin
}

func (p *PreAuthPluginGRPC) GRPCServer(broker *plugin.GRPCBroker, s *grpc.Server) error {
	pb.RegisterPluginServiceServer(s, &PreAuthGRPCServer{Impl: p.Impl})
	return nil
}

func (p *PreAuthPluginGRPC) GRPCClient(ctx context.Context, broker *plugin.GRPCBroker, c *grpc.ClientConn) (interface{}, error) {
	return pb.NewPluginServiceClient(c), nil
}

// AuthPluginGRPC implements the plugin.Plugin interface for auth plugins
type AuthPluginGRPC struct {
	plugin.Plugin
	Impl interfaces.AuthPlugin
}

func (p *AuthPluginGRPC) GRPCServer(broker *plugin.GRPCBroker, s *grpc.Server) error {
	pb.RegisterPluginServiceServer(s, &AuthGRPCServer{Impl: p.Impl})
	return nil
}

func (p *AuthPluginGRPC) GRPCClient(ctx context.Context, broker *plugin.GRPCBroker, c *grpc.ClientConn) (interface{}, error) {
	return pb.NewPluginServiceClient(c), nil
}

// PostAuthPluginGRPC implements the plugin.Plugin interface for post-auth plugins
type PostAuthPluginGRPC struct {
	plugin.Plugin
	Impl interfaces.PostAuthPlugin
}

func (p *PostAuthPluginGRPC) GRPCServer(broker *plugin.GRPCBroker, s *grpc.Server) error {
	pb.RegisterPluginServiceServer(s, &PostAuthGRPCServer{Impl: p.Impl})
	return nil
}

func (p *PostAuthPluginGRPC) GRPCClient(ctx context.Context, broker *plugin.GRPCBroker, c *grpc.ClientConn) (interface{}, error) {
	return pb.NewPluginServiceClient(c), nil
}

// ResponsePluginGRPC implements the plugin.Plugin interface for response plugins
type ResponsePluginGRPC struct {
	plugin.Plugin
	Impl interfaces.ResponsePlugin
}

func (p *ResponsePluginGRPC) GRPCServer(broker *plugin.GRPCBroker, s *grpc.Server) error {
	pb.RegisterPluginServiceServer(s, &ResponseGRPCServer{Impl: p.Impl})
	return nil
}

func (p *ResponsePluginGRPC) GRPCClient(ctx context.Context, broker *plugin.GRPCBroker, c *grpc.ClientConn) (interface{}, error) {
	return pb.NewPluginServiceClient(c), nil
}