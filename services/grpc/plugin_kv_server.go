package grpc

import (
	"context"
	"time"

	pb "github.com/TykTechnologies/midsommar/v2/proto/ai_studio_management"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// PluginKVServer implements the key-value storage operations for AI Studio plugins
type PluginKVServer struct {
	pb.UnimplementedAIStudioManagementServiceServer
	kvService *services.PluginKVService
}

// NewPluginKVServer creates a new plugin KV gRPC server
func NewPluginKVServer(kvService *services.PluginKVService) *PluginKVServer {
	return &PluginKVServer{
		kvService: kvService,
	}
}

// WritePluginKV writes a key-value entry for the calling plugin
func (s *PluginKVServer) WritePluginKV(ctx context.Context, req *pb.WritePluginKVRequest) (*pb.WritePluginKVResponse, error) {
	// Get authenticated plugin from context (set by auth interceptor)
	plugin, ok := GetPluginFromContext(ctx)
	if !ok || plugin == nil {
		log.Error().Msg("Plugin context missing in WritePluginKV")
		return nil, status.Errorf(codes.Internal, "plugin authentication error")
	}

	// Validate request
	key := req.GetKey()
	if key == "" {
		return nil, status.Errorf(codes.InvalidArgument, "key cannot be empty")
	}

	value := req.GetValue()
	if value == nil {
		// Allow empty values (delete semantics could be separate)
		value = []byte{}
	}

	// Extract optional expiration from request
	var expireAt *time.Time
	if req.GetExpireAt() != nil {
		expireTime := req.GetExpireAt().AsTime()
		expireAt = &expireTime
	}

	// Write KV data - plugin ID is automatically scoped from authenticated context
	created, err := s.kvService.WriteKV(plugin.ID, key, value, expireAt)
	if err != nil {
		log.Error().
			Err(err).
			Uint("plugin_id", plugin.ID).
			Str("plugin_name", plugin.Name).
			Str("key", key).
			Msg("Failed to write plugin KV data")

		// Check for specific error types
		if err.Error() == "plugin not found" {
			return nil, status.Errorf(codes.NotFound, "plugin not found")
		}

		return nil, status.Errorf(codes.Internal, "failed to write KV data: %v", err)
	}

	var message string
	if created {
		message = "Key created successfully"
	} else {
		message = "Key updated successfully"
	}

	log.Debug().
		Uint("plugin_id", plugin.ID).
		Str("plugin_name", plugin.Name).
		Str("key", key).
		Bool("created", created).
		Int("value_size", len(value)).
		Msg("Plugin KV data written")

	return &pb.WritePluginKVResponse{
		Created: created,
		Message: message,
	}, nil
}

// ReadPluginKV reads a key-value entry for the calling plugin
func (s *PluginKVServer) ReadPluginKV(ctx context.Context, req *pb.ReadPluginKVRequest) (*pb.ReadPluginKVResponse, error) {
	// Get authenticated plugin from context
	plugin, ok := GetPluginFromContext(ctx)
	if !ok || plugin == nil {
		log.Error().Msg("Plugin context missing in ReadPluginKV")
		return nil, status.Errorf(codes.Internal, "plugin authentication error")
	}

	// Validate request
	key := req.GetKey()
	if key == "" {
		return nil, status.Errorf(codes.InvalidArgument, "key cannot be empty")
	}

	// Read KV data - plugin ID is automatically scoped from authenticated context
	value, err := s.kvService.ReadKV(plugin.ID, key)
	if err != nil {
		log.Debug().
			Err(err).
			Uint("plugin_id", plugin.ID).
			Str("plugin_name", plugin.Name).
			Str("key", key).
			Msg("Failed to read plugin KV data")

		// Check for key not found
		if err.Error() == "key not found: "+key {
			return nil, status.Errorf(codes.NotFound, "key not found: %s", key)
		}

		return nil, status.Errorf(codes.Internal, "failed to read KV data: %v", err)
	}

	log.Debug().
		Uint("plugin_id", plugin.ID).
		Str("plugin_name", plugin.Name).
		Str("key", key).
		Int("value_size", len(value)).
		Msg("Plugin KV data read")

	return &pb.ReadPluginKVResponse{
		Value:   value,
		Message: "Key retrieved successfully",
	}, nil
}

// DeletePluginKV deletes a key-value entry for the calling plugin
func (s *PluginKVServer) DeletePluginKV(ctx context.Context, req *pb.DeletePluginKVRequest) (*pb.DeletePluginKVResponse, error) {
	// Get authenticated plugin from context
	plugin, ok := GetPluginFromContext(ctx)
	if !ok || plugin == nil {
		log.Error().Msg("Plugin context missing in DeletePluginKV")
		return nil, status.Errorf(codes.Internal, "plugin authentication error")
	}

	// Validate request
	key := req.GetKey()
	if key == "" {
		return nil, status.Errorf(codes.InvalidArgument, "key cannot be empty")
	}

	// Delete KV data - plugin ID is automatically scoped from authenticated context
	deleted, err := s.kvService.DeleteKV(plugin.ID, key)
	if err != nil {
		log.Error().
			Err(err).
			Uint("plugin_id", plugin.ID).
			Str("plugin_name", plugin.Name).
			Str("key", key).
			Msg("Failed to delete plugin KV data")

		return nil, status.Errorf(codes.Internal, "failed to delete KV data: %v", err)
	}

	var message string
	if deleted {
		message = "Key deleted successfully"
	} else {
		message = "Key did not exist"
	}

	log.Debug().
		Uint("plugin_id", plugin.ID).
		Str("plugin_name", plugin.Name).
		Str("key", key).
		Bool("deleted", deleted).
		Msg("Plugin KV data deletion attempted")

	return &pb.DeletePluginKVResponse{
		Deleted: deleted,
		Message: message,
	}, nil
}