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

// DatasourcesServer implements the AIStudioManagementService for datasources management operations
type DatasourcesServer struct {
	pb.UnimplementedAIStudioManagementServiceServer
	service *services.Service
}

// NewDatasourcesServer creates a new datasources management gRPC server
func NewDatasourcesServer(service *services.Service) *DatasourcesServer {
	return &DatasourcesServer{
		service: service,
	}
}

// ListDatasources returns a list of datasources with filtering and pagination
func (s *DatasourcesServer) ListDatasources(ctx context.Context, req *pb.ListDatasourcesRequest) (*pb.ListDatasourcesResponse, error) {
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
	datasources, totalCount, _, err := s.service.GetAllDatasources(limit, page, false)
	if err != nil {
		log.Error().Err(err).Msg("Failed to list datasources via gRPC")
		return nil, status.Errorf(codes.Internal, "failed to list datasources: %v", err)
	}

	// TODO: Apply is_active and user_id filtering in future versions
	// For MVP, return all datasources

	// Convert service response to gRPC protobuf
	pbDatasources := make([]*pb.DatasourceInfo, len(datasources))
	for i, datasource := range datasources {
		pbDatasources[i] = convertDatasourceToPB(&datasource)
	}

	log.Debug().
		Int("datasource_count", len(datasources)).
		Int64("total_count", totalCount).
		Msg("Listed datasources via gRPC")

	return &pb.ListDatasourcesResponse{
		Datasources: pbDatasources,
		TotalCount:  totalCount,
	}, nil
}

// GetDatasource returns details for a specific datasource
func (s *DatasourcesServer) GetDatasource(ctx context.Context, req *pb.GetDatasourceRequest) (*pb.GetDatasourceResponse, error) {
	datasourceID := req.GetDatasourceId()
	if datasourceID == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "datasource_id is required")
	}

	// Call existing service method
	datasource, err := s.service.GetDatasourceByID(uint(datasourceID))
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, status.Errorf(codes.NotFound, "datasource not found: %d", datasourceID)
		}
		log.Error().Err(err).Uint32("datasource_id", datasourceID).Msg("Failed to get datasource via gRPC")
		return nil, status.Errorf(codes.Internal, "failed to get datasource: %v", err)
	}

	log.Debug().
		Uint32("datasource_id", datasourceID).
		Str("datasource_name", datasource.Name).
		Msg("Retrieved datasource via gRPC")

	return &pb.GetDatasourceResponse{
		Datasource: convertDatasourceToPB(datasource),
	}, nil
}

// CreateDatasource creates a new datasource
func (s *DatasourcesServer) CreateDatasource(ctx context.Context, req *pb.CreateDatasourceRequest) (*pb.CreateDatasourceResponse, error) {
	// Validate required fields
	if req.GetName() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "name is required")
	}

	// Call existing service method
	datasource, err := s.service.CreateDatasource(
		req.GetName(),
		req.GetShortDescription(),
		req.GetLongDescription(),
		req.GetIcon(),
		req.GetUrl(),
		int(req.GetPrivacyScore()),
		uint(req.GetUserId()),
		req.GetTagNames(),
		req.GetDbConnString(),
		req.GetDbSourceType(),
		req.GetDbConnApiKey(),
		req.GetDbName(),
		req.GetEmbedVendor(),
		req.GetEmbedUrl(),
		req.GetEmbedApiKey(),
		req.GetEmbedModel(),
		req.GetActive(),
	)
	if err != nil {
		log.Error().Err(err).Str("name", req.GetName()).Msg("Failed to create datasource via gRPC")
		return nil, status.Errorf(codes.Internal, "failed to create datasource: %v", err)
	}

	log.Info().
		Uint("datasource_id", datasource.ID).
		Str("datasource_name", datasource.Name).
		Msg("Created datasource via gRPC")

	return &pb.CreateDatasourceResponse{
		Datasource: convertDatasourceToPB(datasource),
	}, nil
}

// SearchDatasources searches for datasources by query
func (s *DatasourcesServer) SearchDatasources(ctx context.Context, req *pb.SearchDatasourcesRequest) (*pb.SearchDatasourcesResponse, error) {
	query := req.GetQuery()
	if query == "" {
		return nil, status.Errorf(codes.InvalidArgument, "search query is required")
	}

	// Call existing service method
	datasources, err := s.service.SearchDatasources(query)
	if err != nil {
		log.Error().Err(err).Str("query", query).Msg("Failed to search datasources via gRPC")
		return nil, status.Errorf(codes.Internal, "failed to search datasources: %v", err)
	}

	// Convert service response to gRPC protobuf
	pbDatasources := make([]*pb.DatasourceInfo, len(datasources))
	for i, datasource := range datasources {
		pbDatasources[i] = convertDatasourceToPB(&datasource)
	}

	log.Debug().
		Str("query", query).
		Int("result_count", len(datasources)).
		Msg("Searched datasources via gRPC")

	return &pb.SearchDatasourcesResponse{
		Datasources: pbDatasources,
	}, nil
}

// convertDatasourceToPB converts a models.Datasource to protobuf DatasourceInfo
func convertDatasourceToPB(datasource *models.Datasource) *pb.DatasourceInfo {
	// Convert tags (no description field in Tag model)
	pbTags := make([]*pb.TagInfo, len(datasource.Tags))
	for i, tag := range datasource.Tags {
		pbTags[i] = &pb.TagInfo{
			Id:        uint32(tag.ID),
			Name:      tag.Name,
			CreatedAt: timestamppb.New(tag.CreatedAt),
			UpdatedAt: timestamppb.New(tag.UpdatedAt),
		}
	}

	return &pb.DatasourceInfo{
		Id:               uint32(datasource.ID),
		Name:             datasource.Name,
		ShortDescription: datasource.ShortDescription,
		LongDescription:  datasource.LongDescription,
		Icon:             datasource.Icon,
		Url:              datasource.Url,
		PrivacyScore:     int32(datasource.PrivacyScore),
		UserId:           uint32(datasource.UserID),
		Tags:             pbTags,
		DbSourceType:     datasource.DBSourceType,
		DbName:           datasource.DBName,
		EmbedVendor:      string(datasource.EmbedVendor),
		EmbedModel:       datasource.EmbedModel,
		Active:           datasource.Active,
		HasDbConnApiKey:  datasource.DBConnAPIKey != "",
		HasEmbedApiKey:   datasource.EmbedAPIKey != "",
		CreatedAt:        timestamppb.New(datasource.CreatedAt),
		UpdatedAt:        timestamppb.New(datasource.UpdatedAt),
	}
}