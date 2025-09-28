package services

import (
	"context"

	mgmtpb "github.com/TykTechnologies/midsommar/v2/proto/ai_studio_management"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// AIStudioServiceServer provides AI Studio service APIs to plugins via gRPC
// This server runs in the host process and exposes service methods to plugins
type AIStudioServiceServer struct {
	mgmtpb.UnimplementedAIStudioManagementServiceServer
	service *Service
}

// NewAIStudioServiceServer creates a new AI Studio service server for plugin access
func NewAIStudioServiceServer(service *Service) *AIStudioServiceServer {
	return &AIStudioServiceServer{
		service: service,
	}
}

// ListPlugins exposes plugin listing service to plugins
func (s *AIStudioServiceServer) ListPlugins(ctx context.Context, req *mgmtpb.ListPluginsRequest) (*mgmtpb.ListPluginsResponse, error) {
	// Plugin authentication and scope validation is handled by the gRPC interceptor
	// The interceptor ensures the plugin has "plugins.read" scope

	// For now, return a simple demo response to test the connection
	log.Info().Msg("Plugin service API call: ListPlugins")

	// Create demo plugin data
	demoPlugins := []*mgmtpb.PluginInfo{
		{
			Id:         1,
			Name:       "Demo Plugin 1",
			PluginType: "ai_studio",
			IsActive:   true,
			HookType:   "studio_ui",
		},
		{
			Id:         2,
			Name:       "Demo Plugin 2",
			PluginType: "gateway",
			IsActive:   true,
			HookType:   "pre_auth",
		},
	}

	return &mgmtpb.ListPluginsResponse{
		Plugins:    demoPlugins,
		TotalCount: int64(len(demoPlugins)),
	}, nil
}

// ListLLMs exposes LLM listing service to plugins
func (s *AIStudioServiceServer) ListLLMs(ctx context.Context, req *mgmtpb.ListLLMsRequest) (*mgmtpb.ListLLMsResponse, error) {
	// Plugin authentication and scope validation is handled by the gRPC interceptor
	// The interceptor ensures the plugin has "llms.read" scope

	// For now, return a simple demo response to test the connection
	log.Info().Msg("Plugin service API call: ListLLMs")

	// Create demo LLM data
	demoLLMs := []*mgmtpb.LLMInfo{
		{
			Id:     1,
			Name:   "Demo OpenAI GPT-4",
			Vendor: "openai",
			Active: true,
		},
		{
			Id:     2,
			Name:   "Demo Claude",
			Vendor: "anthropic",
			Active: true,
		},
	}

	return &mgmtpb.ListLLMsResponse{
		Llms:       demoLLMs,
		TotalCount: int64(len(demoLLMs)),
	}, nil
}

// ListTools exposes tool listing service to plugins
func (s *AIStudioServiceServer) ListTools(ctx context.Context, req *mgmtpb.ListToolsRequest) (*mgmtpb.ListToolsResponse, error) {
	// Plugin authentication and scope validation is handled by the gRPC interceptor
	// The interceptor ensures the plugin has "tools.read" scope

	// For now, return a simple demo response to test the connection
	log.Info().Msg("Plugin service API call: ListTools")

	// Create demo tool data
	demoTools := []*mgmtpb.ToolInfo{
		{
			Id:       1,
			Name:     "Demo API Tool",
			ToolType: "api",
			IsActive: true,
		},
		{
			Id:       2,
			Name:     "Demo Database Tool",
			ToolType: "database",
			IsActive: true,
		},
	}

	return &mgmtpb.ListToolsResponse{
		Tools:      demoTools,
		TotalCount: int64(len(demoTools)),
	}, nil
}

// Placeholder implementations for other service methods
// These can be implemented as needed for specific plugin requirements

func (s *AIStudioServiceServer) GetPlugin(ctx context.Context, req *mgmtpb.GetPluginRequest) (*mgmtpb.GetPluginResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "GetPlugin not yet implemented")
}

func (s *AIStudioServiceServer) UpdatePluginConfig(ctx context.Context, req *mgmtpb.UpdatePluginConfigRequest) (*mgmtpb.UpdatePluginConfigResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "UpdatePluginConfig not yet implemented")
}

func (s *AIStudioServiceServer) GetLLM(ctx context.Context, req *mgmtpb.GetLLMRequest) (*mgmtpb.GetLLMResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "GetLLM not yet implemented")
}

func (s *AIStudioServiceServer) GetLLMPlugins(ctx context.Context, req *mgmtpb.GetLLMPluginsRequest) (*mgmtpb.GetLLMPluginsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "GetLLMPlugins not yet implemented")
}