package grpc

import (
	"context"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/models"
	pb "github.com/TykTechnologies/midsommar/v2/proto/ai_studio_management"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// FiltersServer implements the AIStudioManagementService for filters management operations
type FiltersServer struct {
	pb.UnimplementedAIStudioManagementServiceServer
	service *services.Service
}

// NewFiltersServer creates a new filters management gRPC server
func NewFiltersServer(service *services.Service) *FiltersServer {
	return &FiltersServer{
		service: service,
	}
}

// ListFilters returns a list of filters with filtering and pagination
func (s *FiltersServer) ListFilters(ctx context.Context, req *pb.ListFiltersRequest) (*pb.ListFiltersResponse, error) {
	// Convert gRPC request parameters to service parameters
	page := int(req.GetPage())
	if page <= 0 {
		page = 1
	}
	limit := int(req.GetLimit())
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	// Call existing service method
	filters, totalCount, _, err := s.service.GetAllFilters(limit, page, false)
	if err != nil {
		log.Error().Err(err).Msg("Failed to list filters via gRPC")
		return nil, status.Errorf(codes.Internal, "failed to list filters: %v", err)
	}

	// TODO: Apply filter_type, is_active, and namespace filtering in future versions
	// For MVP, return all filters

	// Convert service response to gRPC protobuf
	pbFilters := make([]*pb.FilterInfo, len(filters))
	for i, filter := range filters {
		pbFilters[i] = convertFilterToPB(&filter)
	}

	log.Debug().
		Int("filter_count", len(filters)).
		Int64("total_count", totalCount).
		Msg("Listed filters via gRPC")

	return &pb.ListFiltersResponse{
		Filters:    pbFilters,
		TotalCount: totalCount,
	}, nil
}

// GetFilter returns details for a specific filter
func (s *FiltersServer) GetFilter(ctx context.Context, req *pb.GetFilterRequest) (*pb.GetFilterResponse, error) {
	filterID := req.GetFilterId()
	if filterID == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "filter_id is required")
	}

	// Call existing service method
	filter, err := s.service.GetFilterByID(uint(filterID))
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, status.Errorf(codes.NotFound, "filter not found: %d", filterID)
		}
		log.Error().Err(err).Uint32("filter_id", filterID).Msg("Failed to get filter via gRPC")
		return nil, status.Errorf(codes.Internal, "failed to get filter: %v", err)
	}

	log.Debug().
		Uint32("filter_id", filterID).
		Str("filter_name", filter.Name).
		Msg("Retrieved filter via gRPC")

	return &pb.GetFilterResponse{
		Filter: convertFilterToPB(filter),
	}, nil
}

// CreateFilter creates a new filter
func (s *FiltersServer) CreateFilter(ctx context.Context, req *pb.CreateFilterRequest) (*pb.CreateFilterResponse, error) {
	// Validate required fields
	if req.GetName() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "name is required")
	}

	// Call existing service method
	filter, err := s.service.CreateFilter(
		req.GetName(),
		req.GetDescription(),
		[]byte(req.GetScript()),
	)
	if err != nil {
		log.Error().Err(err).
			Str("name", req.GetName()).
			Msg("Failed to create filter via gRPC")
		return nil, status.Errorf(codes.Internal, "failed to create filter: %v", err)
	}

	log.Info().
		Uint("filter_id", filter.ID).
		Str("filter_name", filter.Name).
		Msg("Created filter via gRPC")

	return &pb.CreateFilterResponse{
		Filter: convertFilterToPB(filter),
	}, nil
}

// UpdateFilter updates an existing filter
func (s *FiltersServer) UpdateFilter(ctx context.Context, req *pb.UpdateFilterRequest) (*pb.UpdateFilterResponse, error) {
	filterID := req.GetFilterId()
	if filterID == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "filter_id is required")
	}

	// Validate required fields
	if req.GetName() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "name is required")
	}

	// Call existing service method
	filter, err := s.service.UpdateFilter(
		uint(filterID),
		req.GetName(),
		req.GetDescription(),
		[]byte(req.GetScript()),
	)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, status.Errorf(codes.NotFound, "filter not found: %d", filterID)
		}
		log.Error().Err(err).
			Uint32("filter_id", filterID).
			Str("name", req.GetName()).
			Msg("Failed to update filter via gRPC")
		return nil, status.Errorf(codes.Internal, "failed to update filter: %v", err)
	}

	log.Info().
		Uint32("filter_id", filterID).
		Str("filter_name", filter.Name).
		Msg("Updated filter via gRPC")

	return &pb.UpdateFilterResponse{
		Filter: convertFilterToPB(filter),
	}, nil
}

// DeleteFilter deletes a filter
func (s *FiltersServer) DeleteFilter(ctx context.Context, req *pb.DeleteFilterRequest) (*pb.DeleteFilterResponse, error) {
	filterID := req.GetFilterId()
	if filterID == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "filter_id is required")
	}

	// Call existing service method
	err := s.service.DeleteFilter(uint(filterID))
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, status.Errorf(codes.NotFound, "filter not found: %d", filterID)
		}
		log.Error().Err(err).
			Uint32("filter_id", filterID).
			Msg("Failed to delete filter via gRPC")
		return nil, status.Errorf(codes.Internal, "failed to delete filter: %v", err)
	}

	log.Info().
		Uint32("filter_id", filterID).
		Msg("Deleted filter via gRPC")

	return &pb.DeleteFilterResponse{
		Success: true,
		Message: "Filter deleted successfully",
	}, nil
}

// convertFilterToPB converts a models.Filter to protobuf FilterInfo
func convertFilterToPB(filter *models.Filter) *pb.FilterInfo {
	// For security, truncate script if it's too long
	script := string(filter.Script)
	if len(script) > 1000 {
		script = script[:1000] + "... [truncated for security]"
	}

	return &pb.FilterInfo{
		Id:          uint32(filter.ID),
		Name:        filter.Name,
		Description: filter.Description,
		Script:      script,
		IsActive:    true,               // Filter model doesn't have IsActive field - default to true
		OrderIndex:  0,                  // Filter model doesn't have OrderIndex field - default to 0
		Namespace:   filter.Namespace,   // Filter model has Namespace field
		LlmIds:      []uint32{},         // TODO: Query actual LLM relationships if needed
		CreatedAt:   timestamppb.New(filter.CreatedAt),
		UpdatedAt:   timestamppb.New(filter.UpdatedAt),
	}
}