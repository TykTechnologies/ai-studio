package grpc

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/TykTechnologies/midsommar/v2/data_session"
	"github.com/TykTechnologies/midsommar/v2/models"
	pb "github.com/TykTechnologies/midsommar/v2/proto/ai_studio_management"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/gorm"
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

	// Handle is_active parameter
	var isActive *bool
	if req.IsActive != nil {
		value := req.GetIsActive()
		isActive = &value
	}

	// Handle user_id parameter
	var userID *uint
	if req.GetUserId() != "" {
		// Parse user_id string to uint
		if id, err := strconv.ParseUint(req.GetUserId(), 10, 32); err == nil {
			value := uint(id)
			userID = &value
		} else {
			log.Warn().Str("user_id", req.GetUserId()).Msg("Invalid user_id format in ListDatasources request")
		}
	}

	// Call enhanced service method with filtering
	datasources, totalCount, _, err := s.service.GetAllDatasourcesWithFilters(limit, page, false, isActive, userID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to list datasources via gRPC")
		return nil, status.Errorf(codes.Internal, "failed to list datasources: %v", err)
	}

	// Convert service response to gRPC protobuf
	pbDatasources := make([]*pb.DatasourceInfo, len(datasources))
	for i, datasource := range datasources {
		pbDatasources[i] = convertDatasourceToPB(&datasource)
	}

	log.Debug().
		Int("datasource_count", len(datasources)).
		Int64("total_count", totalCount).
		Interface("is_active", isActive).
		Interface("user_id", userID).
		Msg("Listed datasources with filtering via gRPC")

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
		if errors.Is(err, gorm.ErrRecordNotFound) {
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

// UpdateDatasource updates an existing datasource
func (s *DatasourcesServer) UpdateDatasource(ctx context.Context, req *pb.UpdateDatasourceRequest) (*pb.UpdateDatasourceResponse, error) {
	datasourceID := req.GetDatasourceId()
	if datasourceID == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "datasource_id is required")
	}

	// Validate required fields
	if req.GetName() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "name is required")
	}

	// Call existing service method
	datasource, err := s.service.UpdateDatasource(
		uint(datasourceID),
		req.GetName(),
		req.GetShortDescription(),
		req.GetLongDescription(),
		req.GetIcon(),
		req.GetUrl(),
		int(req.GetPrivacyScore()),
		req.GetDbConnString(),
		req.GetDbSourceType(),
		req.GetDbConnApiKey(),
		req.GetDbName(),
		req.GetEmbedVendor(),
		req.GetEmbedUrl(),
		req.GetEmbedApiKey(),
		req.GetEmbedModel(),
		req.GetActive(),
		req.GetTagNames(),
		uint(req.GetUserId()),
	)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, status.Errorf(codes.NotFound, "datasource not found: %d", datasourceID)
		}
		log.Error().Err(err).
			Uint32("datasource_id", datasourceID).
			Str("name", req.GetName()).
			Msg("Failed to update datasource via gRPC")
		return nil, status.Errorf(codes.Internal, "failed to update datasource: %v", err)
	}

	log.Info().
		Uint32("datasource_id", datasourceID).
		Str("datasource_name", datasource.Name).
		Msg("Updated datasource via gRPC")

	return &pb.UpdateDatasourceResponse{
		Datasource: convertDatasourceToPB(datasource),
	}, nil
}

// DeleteDatasource deletes a datasource
func (s *DatasourcesServer) DeleteDatasource(ctx context.Context, req *pb.DeleteDatasourceRequest) (*pb.DeleteDatasourceResponse, error) {
	datasourceID := req.GetDatasourceId()
	if datasourceID == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "datasource_id is required")
	}

	// Call existing service method
	err := s.service.DeleteDatasource(uint(datasourceID))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, status.Errorf(codes.NotFound, "datasource not found: %d", datasourceID)
		}
		log.Error().Err(err).
			Uint32("datasource_id", datasourceID).
			Msg("Failed to delete datasource via gRPC")
		return nil, status.Errorf(codes.Internal, "failed to delete datasource: %v", err)
	}

	log.Info().
		Uint32("datasource_id", datasourceID).
		Msg("Deleted datasource via gRPC")

	return &pb.DeleteDatasourceResponse{
		Success: true,
		Message: "Datasource deleted successfully",
	}, nil
}

// ProcessDatasourceEmbeddings processes embeddings for a datasource
func (s *DatasourcesServer) ProcessDatasourceEmbeddings(ctx context.Context, req *pb.ProcessEmbeddingsRequest) (*pb.ProcessEmbeddingsResponse, error) {
	datasourceID := req.GetDatasourceId()
	if datasourceID == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "datasource_id is required")
	}

	// Get datasource with files to verify it exists and has content
	datasource, err := s.service.GetDatasourceByID(uint(datasourceID))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, status.Errorf(codes.NotFound, "datasource not found: %d", datasourceID)
		}
		return nil, status.Errorf(codes.Internal, "failed to get datasource: %v", err)
	}

	// Initialize sources map for DataSession
	sources := make(map[uint]*models.Datasource)
	sources[datasource.ID] = datasource

	// Create new DataSession
	ds := data_session.NewDataSession(sources)

	// Process embeddings in a goroutine (same pattern as REST API)
	go func() {
		err := ds.ProcessRAGForDatasource(uint(datasourceID), s.service.DB)
		if err != nil {
			log.Error().Err(err).
				Uint32("datasource_id", datasourceID).
				Msg("Error processing embeddings for datasource via gRPC")
			return
		}
		log.Info().
			Uint32("datasource_id", datasourceID).
			Msg("Successfully processed embeddings for datasource via gRPC")

		// Update LastProcessedOn for all files in the datasource
		for _, file := range datasource.Files {
			file.LastProcessedOn = time.Now()
			err = file.Update(s.service.DB)
			if err != nil {
				log.Error().Err(err).
					Uint("file_id", file.ID).
					Msg("Error updating LastProcessedOn for file")
			}
		}
	}()

	log.Info().
		Uint32("datasource_id", datasourceID).
		Str("datasource_name", datasource.Name).
		Msg("Started real embedding processing for datasource via gRPC")

	return &pb.ProcessEmbeddingsResponse{
		Success: true,
		Message: "Embedding processing started successfully",
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

	// Convert metadata (JSONMap is already map[string]interface{})
	metadata := make(map[string]string)
	if datasource.Metadata != nil && len(datasource.Metadata) > 0 {
		// Convert interface{} values to strings for proto
		for k, v := range datasource.Metadata {
			if str, ok := v.(string); ok {
				metadata[k] = str
			} else {
				// Convert non-string values to JSON strings
				if jsonBytes, err := json.Marshal(v); err == nil {
					metadata[k] = string(jsonBytes)
				}
			}
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
		Metadata:         metadata, // Plugin-stored data
	}
}