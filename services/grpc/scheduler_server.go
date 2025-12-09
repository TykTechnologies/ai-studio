package grpc

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/TykTechnologies/midsommar/v2/models"
	pb "github.com/TykTechnologies/midsommar/v2/proto/ai_studio_management"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// SchedulerServer implements the AIStudioManagementService for schedule management operations
type SchedulerServer struct {
	pb.UnimplementedAIStudioManagementServiceServer
	service *services.Service
}

// NewSchedulerServer creates a new schedule management gRPC server
func NewSchedulerServer(service *services.Service) *SchedulerServer {
	return &SchedulerServer{
		service: service,
	}
}

// CreateSchedule creates a new schedule for the calling plugin
func (s *SchedulerServer) CreateSchedule(ctx context.Context, req *pb.CreateScheduleRequest) (*pb.CreateScheduleResponse, error) {
	// Validate required fields
	if req.GetScheduleId() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "schedule_id is required")
	}
	if req.GetName() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "name is required")
	}
	if req.GetCronExpr() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "cron_expr is required")
	}

	// Get plugin ID from context
	pluginID := req.GetContext().GetPluginId()

	// Set defaults
	timezone := req.GetTimezone()
	if timezone == "" {
		timezone = "UTC"
	}

	timeoutSeconds := int(req.GetTimeoutSeconds())
	if timeoutSeconds == 0 {
		timeoutSeconds = 60
	}

	// Parse config JSON to map
	var config map[string]interface{}
	if configJSON := req.GetConfigJson(); configJSON != "" {
		if err := json.Unmarshal([]byte(configJSON), &config); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid config_json: %v", err)
		}
	}

	// Call service layer
	schedule, err := s.service.CreateSchedule(
		uint(pluginID),
		req.GetScheduleId(),
		req.GetName(),
		req.GetCronExpr(),
		timezone,
		timeoutSeconds,
		config,
		req.GetEnabled(),
	)

	if err != nil {
		if err == services.ErrScheduleAlreadyExists {
			return nil, status.Errorf(codes.AlreadyExists, "schedule with ID %s already exists", req.GetScheduleId())
		}
		log.Error().Err(err).
			Uint32("plugin_id", pluginID).
			Str("schedule_id", req.GetScheduleId()).
			Msg("Failed to create schedule via gRPC")
		return nil, status.Errorf(codes.Internal, "failed to create schedule: %v", err)
	}

	log.Info().
		Uint32("plugin_id", pluginID).
		Str("schedule_id", req.GetScheduleId()).
		Str("name", req.GetName()).
		Msg("Created schedule via gRPC")

	return &pb.CreateScheduleResponse{
		Schedule: convertScheduleToPB(schedule),
	}, nil
}

// GetSchedule retrieves a specific schedule by manifest_schedule_id
func (s *SchedulerServer) GetSchedule(ctx context.Context, req *pb.GetScheduleRequest) (*pb.GetScheduleResponse, error) {
	if req.GetScheduleId() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "schedule_id is required")
	}

	pluginID := req.GetContext().GetPluginId()

	// Call service layer using manifest ID
	schedule, err := s.service.GetScheduleByManifestID(uint(pluginID), req.GetScheduleId())
	if err != nil {
		if err == services.ErrScheduleNotFound {
			return nil, status.Errorf(codes.NotFound, "schedule not found: %s", req.GetScheduleId())
		}
		log.Error().Err(err).
			Uint32("plugin_id", pluginID).
			Str("schedule_id", req.GetScheduleId()).
			Msg("Failed to get schedule via gRPC")
		return nil, status.Errorf(codes.Internal, "failed to get schedule: %v", err)
	}

	log.Debug().
		Uint32("plugin_id", pluginID).
		Str("schedule_id", req.GetScheduleId()).
		Msg("Retrieved schedule via gRPC")

	return &pb.GetScheduleResponse{
		Schedule: convertScheduleToPB(schedule),
	}, nil
}

// ListSchedules lists all schedules for the calling plugin
func (s *SchedulerServer) ListSchedules(ctx context.Context, req *pb.ListSchedulesRequest) (*pb.ListSchedulesResponse, error) {
	pluginID := req.GetContext().GetPluginId()

	// Call service layer
	schedules, err := s.service.ListSchedules(uint(pluginID))
	if err != nil {
		log.Error().Err(err).
			Uint32("plugin_id", pluginID).
			Msg("Failed to list schedules via gRPC")
		return nil, status.Errorf(codes.Internal, "failed to list schedules: %v", err)
	}

	// Convert to protobuf
	pbSchedules := make([]*pb.ScheduleInfo, len(schedules))
	for i, schedule := range schedules {
		pbSchedules[i] = convertScheduleToPB(&schedule)
	}

	log.Debug().
		Uint32("plugin_id", pluginID).
		Int("schedule_count", len(schedules)).
		Msg("Listed schedules via gRPC")

	return &pb.ListSchedulesResponse{
		Schedules:  pbSchedules,
		TotalCount: int64(len(schedules)),
	}, nil
}

// UpdateSchedule updates an existing schedule
func (s *SchedulerServer) UpdateSchedule(ctx context.Context, req *pb.UpdateScheduleRequest) (*pb.UpdateScheduleResponse, error) {
	if req.GetScheduleId() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "schedule_id is required")
	}

	pluginID := req.GetContext().GetPluginId()

	// First, get the schedule to find its database ID
	existingSchedule, err := s.service.GetScheduleByManifestID(uint(pluginID), req.GetScheduleId())
	if err != nil {
		if err == services.ErrScheduleNotFound {
			return nil, status.Errorf(codes.NotFound, "schedule not found: %s", req.GetScheduleId())
		}
		return nil, status.Errorf(codes.Internal, "failed to find schedule: %v", err)
	}

	// Build updates map with optional fields
	updates := make(map[string]interface{})

	if req.Name != nil {
		updates["name"] = req.GetName()
	}
	if req.CronExpr != nil {
		updates["cron_expr"] = req.GetCronExpr()
	}
	if req.Timezone != nil {
		updates["timezone"] = req.GetTimezone()
	}
	if req.TimeoutSeconds != nil {
		updates["timeout_seconds"] = req.GetTimeoutSeconds()
	}
	if req.ConfigJson != nil {
		// Validate JSON if provided
		configJSON := req.GetConfigJson()
		var config map[string]interface{}
		if err := json.Unmarshal([]byte(configJSON), &config); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid config_json: %v", err)
		}
		updates["config"] = configJSON
	}
	if req.Enabled != nil {
		updates["enabled"] = req.GetEnabled()
	}

	// Call service layer
	schedule, err := s.service.UpdateSchedule(uint(pluginID), existingSchedule.ID, updates)
	if err != nil {
		if err == services.ErrScheduleNotFound {
			return nil, status.Errorf(codes.NotFound, "schedule not found: %s", req.GetScheduleId())
		}
		log.Error().Err(err).
			Uint32("plugin_id", pluginID).
			Str("schedule_id", req.GetScheduleId()).
			Msg("Failed to update schedule via gRPC")
		return nil, status.Errorf(codes.Internal, "failed to update schedule: %v", err)
	}

	log.Info().
		Uint32("plugin_id", pluginID).
		Str("schedule_id", req.GetScheduleId()).
		Msg("Updated schedule via gRPC")

	return &pb.UpdateScheduleResponse{
		Schedule: convertScheduleToPB(schedule),
	}, nil
}

// DeleteSchedule deletes a schedule
func (s *SchedulerServer) DeleteSchedule(ctx context.Context, req *pb.DeleteScheduleRequest) (*pb.DeleteScheduleResponse, error) {
	if req.GetScheduleId() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "schedule_id is required")
	}

	pluginID := req.GetContext().GetPluginId()

	// First, get the schedule to find its database ID
	existingSchedule, err := s.service.GetScheduleByManifestID(uint(pluginID), req.GetScheduleId())
	if err != nil {
		if err == services.ErrScheduleNotFound {
			return nil, status.Errorf(codes.NotFound, "schedule not found: %s", req.GetScheduleId())
		}
		return nil, status.Errorf(codes.Internal, "failed to find schedule: %v", err)
	}

	// Call service layer
	if err := s.service.DeleteSchedule(uint(pluginID), existingSchedule.ID); err != nil {
		if err == services.ErrScheduleNotFound {
			return nil, status.Errorf(codes.NotFound, "schedule not found: %s", req.GetScheduleId())
		}
		log.Error().Err(err).
			Uint32("plugin_id", pluginID).
			Str("schedule_id", req.GetScheduleId()).
			Msg("Failed to delete schedule via gRPC")
		return nil, status.Errorf(codes.Internal, "failed to delete schedule: %v", err)
	}

	log.Info().
		Uint32("plugin_id", pluginID).
		Str("schedule_id", req.GetScheduleId()).
		Msg("Deleted schedule via gRPC")

	return &pb.DeleteScheduleResponse{
		Success: true,
		Message: fmt.Sprintf("Schedule %s deleted successfully", req.GetScheduleId()),
	}, nil
}

// convertScheduleToPB converts a models.PluginSchedule to protobuf ScheduleInfo
func convertScheduleToPB(schedule *models.PluginSchedule) *pb.ScheduleInfo {
	if schedule == nil {
		return nil
	}

	pbSchedule := &pb.ScheduleInfo{
		Id:             uint32(schedule.ID),
		PluginId:       uint32(schedule.PluginID),
		ScheduleId:     schedule.ManifestScheduleID,
		Name:           schedule.Name,
		CronExpr:       schedule.CronExpr,
		Timezone:       schedule.Timezone,
		Enabled:        schedule.Enabled,
		Config:         schedule.Config,
		TimeoutSeconds: int32(schedule.TimeoutSeconds),
		CreatedAt:      timestamppb.New(schedule.CreatedAt),
		UpdatedAt:      timestamppb.New(schedule.UpdatedAt),
	}

	// Add optional timestamp fields
	if schedule.LastRun != nil {
		pbSchedule.LastRun = timestamppb.New(*schedule.LastRun)
	}
	if schedule.NextRun != nil {
		pbSchedule.NextRun = timestamppb.New(*schedule.NextRun)
	}

	return pbSchedule
}
