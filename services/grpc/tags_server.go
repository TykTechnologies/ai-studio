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

// TagsServer implements the AIStudioManagementService for tags management operations
type TagsServer struct {
	pb.UnimplementedAIStudioManagementServiceServer
	service *services.Service
}

// NewTagsServer creates a new tags management gRPC server
func NewTagsServer(service *services.Service) *TagsServer {
	return &TagsServer{
		service: service,
	}
}

// ListTags returns a list of tags with pagination
func (s *TagsServer) ListTags(ctx context.Context, req *pb.ListTagsRequest) (*pb.ListTagsResponse, error) {
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
	tags, totalCount, _, err := s.service.GetAllTags(limit, page, false)
	if err != nil {
		log.Error().Err(err).Msg("Failed to list tags via gRPC")
		return nil, status.Errorf(codes.Internal, "failed to list tags: %v", err)
	}

	// Convert service response to gRPC protobuf
	pbTags := make([]*pb.TagInfo, len(tags))
	for i, tag := range tags {
		pbTags[i] = convertTagToPB(&tag)
	}

	log.Debug().
		Int("tag_count", len(tags)).
		Int64("total_count", totalCount).
		Msg("Listed tags via gRPC")

	return &pb.ListTagsResponse{
		Tags:       pbTags,
		TotalCount: totalCount,
	}, nil
}

// GetTag returns details for a specific tag
func (s *TagsServer) GetTag(ctx context.Context, req *pb.GetTagRequest) (*pb.GetTagResponse, error) {
	tagID := req.GetTagId()
	if tagID == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "tag_id is required")
	}

	// Call existing service method
	tag, err := s.service.GetTagByID(uint(tagID))
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, status.Errorf(codes.NotFound, "tag not found: %d", tagID)
		}
		log.Error().Err(err).Uint32("tag_id", tagID).Msg("Failed to get tag via gRPC")
		return nil, status.Errorf(codes.Internal, "failed to get tag: %v", err)
	}

	log.Debug().
		Uint32("tag_id", tagID).
		Str("tag_name", tag.Name).
		Msg("Retrieved tag via gRPC")

	return &pb.GetTagResponse{
		Tag: convertTagToPB(tag),
	}, nil
}

// CreateTag creates a new tag
func (s *TagsServer) CreateTag(ctx context.Context, req *pb.CreateTagRequest) (*pb.CreateTagResponse, error) {
	// Validate required fields
	if req.GetName() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "name is required")
	}

	// Call existing service method (only takes name)
	tag, err := s.service.CreateTag(req.GetName())
	if err != nil {
		log.Error().Err(err).Str("name", req.GetName()).Msg("Failed to create tag via gRPC")
		return nil, status.Errorf(codes.Internal, "failed to create tag: %v", err)
	}

	log.Info().
		Uint("tag_id", tag.ID).
		Str("tag_name", tag.Name).
		Msg("Created tag via gRPC")

	return &pb.CreateTagResponse{
		Tag: convertTagToPB(tag),
	}, nil
}

// SearchTags searches for tags by query
func (s *TagsServer) SearchTags(ctx context.Context, req *pb.SearchTagsRequest) (*pb.SearchTagsResponse, error) {
	query := req.GetQuery()
	if query == "" {
		return nil, status.Errorf(codes.InvalidArgument, "search query is required")
	}

	// Call existing service method
	tags, err := s.service.SearchTagsByNameStub(query)
	if err != nil {
		log.Error().Err(err).Str("query", query).Msg("Failed to search tags via gRPC")
		return nil, status.Errorf(codes.Internal, "failed to search tags: %v", err)
	}

	// Convert service response to gRPC protobuf
	pbTags := make([]*pb.TagInfo, len(tags))
	for i, tag := range tags {
		pbTags[i] = convertTagToPB(&tag)
	}

	log.Debug().
		Str("query", query).
		Int("result_count", len(tags)).
		Msg("Searched tags via gRPC")

	return &pb.SearchTagsResponse{
		Tags: pbTags,
	}, nil
}

// UpdateTag updates an existing tag
func (s *TagsServer) UpdateTag(ctx context.Context, req *pb.UpdateTagRequest) (*pb.UpdateTagResponse, error) {
	tagID := req.GetTagId()
	if tagID == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "tag_id is required")
	}

	// Validate required fields
	if req.GetName() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "name is required")
	}

	// Call existing service method
	tag, err := s.service.UpdateTag(uint(tagID), req.GetName())
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, status.Errorf(codes.NotFound, "tag not found: %d", tagID)
		}
		log.Error().Err(err).
			Uint32("tag_id", tagID).
			Str("name", req.GetName()).
			Msg("Failed to update tag via gRPC")
		return nil, status.Errorf(codes.Internal, "failed to update tag: %v", err)
	}

	log.Info().
		Uint32("tag_id", tagID).
		Str("tag_name", tag.Name).
		Msg("Updated tag via gRPC")

	return &pb.UpdateTagResponse{
		Tag: convertTagToPB(tag),
	}, nil
}

// DeleteTag deletes a tag
func (s *TagsServer) DeleteTag(ctx context.Context, req *pb.DeleteTagRequest) (*pb.DeleteTagResponse, error) {
	tagID := req.GetTagId()
	if tagID == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "tag_id is required")
	}

	// Call existing service method
	err := s.service.DeleteTag(uint(tagID))
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, status.Errorf(codes.NotFound, "tag not found: %d", tagID)
		}
		log.Error().Err(err).
			Uint32("tag_id", tagID).
			Msg("Failed to delete tag via gRPC")
		return nil, status.Errorf(codes.Internal, "failed to delete tag: %v", err)
	}

	log.Info().
		Uint32("tag_id", tagID).
		Msg("Deleted tag via gRPC")

	return &pb.DeleteTagResponse{
		Success: true,
		Message: "Tag deleted successfully",
	}, nil
}

// convertTagToPB converts a models.Tag to protobuf TagInfo
func convertTagToPB(tag *models.Tag) *pb.TagInfo {
	return &pb.TagInfo{
		Id:        uint32(tag.ID),
		Name:      tag.Name,
		CreatedAt: timestamppb.New(tag.CreatedAt),
		UpdatedAt: timestamppb.New(tag.UpdatedAt),
	}
}