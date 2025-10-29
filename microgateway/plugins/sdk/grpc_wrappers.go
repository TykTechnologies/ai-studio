// plugins/sdk/grpc_wrappers.go
package sdk

import (
	"context"

	"github.com/TykTechnologies/midsommar/microgateway/plugins/interfaces"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	configpb "github.com/TykTechnologies/midsommar/v2/proto/configpb"
	"github.com/hashicorp/go-plugin"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
)

// PreAuthPluginGRPC implements the plugin.Plugin interface for pre-auth plugins
type PreAuthPluginGRPC struct {
	plugin.Plugin
	Impl interfaces.PreAuthPlugin
}

func (p *PreAuthPluginGRPC) GRPCServer(broker *plugin.GRPCBroker, s *grpc.Server) error {
	// Initialize SDK with broker access for service API
	if err := InitializeServiceAPI(s, broker, 0); err != nil {
		log.Warn().Err(err).Msg("Failed to initialize microgateway service API SDK")
	}

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
	// Initialize SDK with broker access for service API
	if err := InitializeServiceAPI(s, broker, 0); err != nil {
		log.Warn().Err(err).Msg("Failed to initialize microgateway service API SDK")
	}

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
	// Initialize SDK with broker access for service API
	// Plugin ID will be set later during Initialize() call from config
	if err := InitializeServiceAPI(s, broker, 0); err != nil {
		log.Warn().Err(err).Msg("Failed to initialize microgateway service API SDK")
	}

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

// DataCollectionPluginGRPC implements the plugin.Plugin interface for data collection plugins
type DataCollectionPluginGRPC struct {
	plugin.Plugin
	Impl interfaces.DataCollectionPlugin
}

func (p *DataCollectionPluginGRPC) GRPCServer(broker *plugin.GRPCBroker, s *grpc.Server) error {
	pb.RegisterPluginServiceServer(s, &DataCollectionGRPCServer{Impl: p.Impl})
	return nil
}

func (p *DataCollectionPluginGRPC) GRPCClient(ctx context.Context, broker *plugin.GRPCBroker, c *grpc.ClientConn) (interface{}, error) {
	return pb.NewPluginServiceClient(c), nil
}

// ConfigProviderPluginGRPC implements the plugin.Plugin interface for config-only service
type ConfigProviderPluginGRPC struct {
	plugin.Plugin
	Impl interfaces.BasePlugin
}

func (p *ConfigProviderPluginGRPC) GRPCServer(broker *plugin.GRPCBroker, s *grpc.Server) error {
	configpb.RegisterConfigProviderServiceServer(s, &ConfigProviderGRPC{Impl: p.Impl})
	return nil
}

func (p *ConfigProviderPluginGRPC) GRPCClient(ctx context.Context, broker *plugin.GRPCBroker, c *grpc.ClientConn) (interface{}, error) {
	return configpb.NewConfigProviderServiceClient(c), nil
}