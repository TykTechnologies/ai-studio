package grpc

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/TykTechnologies/midsommar/v2/models"
	pb "github.com/TykTechnologies/midsommar/v2/proto/ai_studio_management"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/gorm"
)

// PluginManagementServer implements the AIStudioManagementService for plugin management operations
type PluginManagementServer struct {
	pb.UnimplementedAIStudioManagementServiceServer
	pluginService *services.PluginService
}

// NewPluginManagementServer creates a new plugin management gRPC server
func NewPluginManagementServer(pluginService *services.PluginService) *PluginManagementServer {
	return &PluginManagementServer{
		pluginService: pluginService,
	}
}

// ListPlugins returns a list of plugins with filtering and pagination
func (s *PluginManagementServer) ListPlugins(ctx context.Context, req *pb.ListPluginsRequest) (*pb.ListPluginsResponse, error) {
	// Note: Authentication and authorization handled by interceptor

	// Convert gRPC request parameters to service parameters
	page := int(req.GetPage())
	if page <= 0 {
		page = 1
	}
	limit := int(req.GetLimit())
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	hookType := req.GetHookType()
	namespace := req.GetNamespace()

	// Handle is_active parameter
	var isActive bool
	var filterByActive bool
	if req.IsActive != nil {
		isActive = req.GetIsActive()
		filterByActive = true
	}

	// Call existing service method
	var plugins []models.Plugin
	var totalCount int64
	var err error

	if filterByActive {
		plugins, totalCount, err = s.pluginService.ListPlugins(page, limit, hookType, isActive, namespace)
	} else {
		plugins, totalCount, err = s.pluginService.ListAllPlugins(page, limit, hookType, namespace)
	}

	if err != nil {
		log.Error().Err(err).Msg("Failed to list plugins via gRPC")
		return nil, status.Errorf(codes.Internal, "failed to list plugins: %v", err)
	}

	// Convert service response to gRPC protobuf
	pbPlugins := make([]*pb.PluginInfo, len(plugins))
	for i, plugin := range plugins {
		pbPlugins[i] = convertPluginToPB(&plugin)
	}

	// Calculate total pages
	totalPages := int32(totalCount) / int32(limit)
	if int32(totalCount)%int32(limit) != 0 {
		totalPages++
	}

	log.Debug().
		Int("plugin_count", len(plugins)).
		Int64("total_count", totalCount).
		Msg("Listed plugins via gRPC")

	return &pb.ListPluginsResponse{
		Plugins:    pbPlugins,
		TotalCount: totalCount,
		TotalPages: totalPages,
	}, nil
}

// GetPlugin returns details for a specific plugin
func (s *PluginManagementServer) GetPlugin(ctx context.Context, req *pb.GetPluginRequest) (*pb.GetPluginResponse, error) {
	pluginID := req.GetPluginId()
	if pluginID == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "plugin_id is required")
	}

	// Call existing service method
	plugin, err := s.pluginService.GetPlugin(uint(pluginID))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, status.Errorf(codes.NotFound, "plugin not found: %d", pluginID)
		}
		log.Error().Err(err).Uint32("plugin_id", pluginID).Msg("Failed to get plugin via gRPC")
		return nil, status.Errorf(codes.Internal, "failed to get plugin: %v", err)
	}

	log.Debug().
		Uint32("plugin_id", pluginID).
		Str("plugin_name", plugin.Name).
		Msg("Retrieved plugin via gRPC")

	return &pb.GetPluginResponse{
		Plugin: convertPluginToPB(plugin),
	}, nil
}

// UpdatePluginConfig updates the configuration for a specific plugin
func (s *PluginManagementServer) UpdatePluginConfig(ctx context.Context, req *pb.UpdatePluginConfigRequest) (*pb.UpdatePluginConfigResponse, error) {
	pluginID := req.GetPluginId()
	if pluginID == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "plugin_id is required")
	}

	configJSON := req.GetConfigJson()
	if configJSON == "" {
		return nil, status.Errorf(codes.InvalidArgument, "config_json is required")
	}

	// Validate JSON format
	var configMap map[string]interface{}
	if err := json.Unmarshal([]byte(configJSON), &configMap); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid JSON configuration: %v", err)
	}

	// Use UpdatePlugin method with config-only update
	updateReq := &services.UpdatePluginRequest{
		Config: configMap,
	}

	_, err := s.pluginService.UpdatePlugin(uint(pluginID), updateReq)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, status.Errorf(codes.NotFound, "plugin not found: %d", pluginID)
		}
		log.Error().Err(err).Uint32("plugin_id", pluginID).Msg("Failed to update plugin config via gRPC")
		return nil, status.Errorf(codes.Internal, "failed to update plugin config: %v", err)
	}

	log.Info().
		Uint32("plugin_id", pluginID).
		Int("config_keys", len(configMap)).
		Msg("Updated plugin configuration via gRPC")

	return &pb.UpdatePluginConfigResponse{
		Success: true,
		Message: "Plugin configuration updated successfully",
	}, nil
}

// convertPluginToPB converts a models.Plugin to protobuf PluginInfo
func convertPluginToPB(plugin *models.Plugin) *pb.PluginInfo {
	// Convert config to JSON string
	var configJSON string
	if plugin.Config != nil {
		if configBytes, err := json.Marshal(plugin.Config); err == nil {
			configJSON = string(configBytes)
		}
	}

	return &pb.PluginInfo{
		Id:                      uint32(plugin.ID),
		Name:                    plugin.Name,
		Slug:                    plugin.Slug,
		Description:             plugin.Description,
		Command:                 plugin.Command,
		ConfigJson:              configJSON,
		HookType:                plugin.HookType,
		IsActive:                plugin.IsActive,
		Namespace:               plugin.Namespace,
		PluginType:              plugin.PluginType,
		ServiceAccessAuthorized: plugin.ServiceAccessAuthorized,
		ServiceScopes:           plugin.ServiceScopes,
		CreatedAt:               timestamppb.New(plugin.CreatedAt),
		UpdatedAt:               timestamppb.New(plugin.UpdatedAt),
	}
}