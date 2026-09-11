package grpc

import (
	"context"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/models"
	pb "github.com/TykTechnologies/midsommar/v2/proto/ai_studio_management"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// RegisterPermissionResources lets a plugin (re)register the RBAC resources
// it contributes at runtime: rows in the role editor beneath the plugin's own
// entry, keyed "plugin:<manifest id>:<key>". The plugin ID always comes from
// the authenticated context, never from the request, so a plugin can only
// manage its own resources. Requires the rbac.register scope.
func (s *AIStudioManagementServer) RegisterPermissionResources(ctx context.Context, req *pb.RegisterPermissionResourcesRequest) (*pb.RegisterPermissionResourcesResponse, error) {
	plugin, err := s.validatePluginScope(ctx, models.ServiceScopeRBACRegister)
	if err != nil {
		return nil, err
	}
	if s.service == nil {
		return nil, status.Errorf(codes.Unavailable, "service not available")
	}

	rows := make([]models.PluginPermissionResource, 0, len(req.Resources))
	for _, spec := range req.Resources {
		if spec == nil {
			continue
		}
		key := strings.TrimSpace(spec.Key)
		if key == "" || strings.TrimSpace(spec.Label) == "" {
			return nil, status.Errorf(codes.InvalidArgument, "each permission resource needs a key and a label")
		}
		rows = append(rows, models.PluginPermissionResource{
			Key:         key,
			Label:       strings.TrimSpace(spec.Label),
			Description: spec.Description,
			Actions:     models.StringList(spec.Actions),
			Sensitive:   spec.Sensitive,
		})
	}

	pluginKey, removed, err := s.service.RegisterPluginPermissionResources(plugin.ID, rows, req.RemoveMissing)
	if err != nil {
		if strings.HasPrefix(err.Error(), "rbac.resources") {
			return nil, status.Errorf(codes.InvalidArgument, "%v", err)
		}
		log.Error().Err(err).Uint("plugin_id", plugin.ID).Msg("Failed to register permission resources")
		return nil, status.Errorf(codes.Internal, "failed to register permission resources: %v", err)
	}

	log.Info().Uint("plugin_id", plugin.ID).Int("registered", len(rows)).Int("removed", removed).Msg("Plugin permission resources registered")
	return &pb.RegisterPermissionResourcesResponse{
		Success:             true,
		Registered:          uint32(len(rows)),
		Removed:             uint32(removed),
		PluginPermissionKey: pluginKey,
	}, nil
}
