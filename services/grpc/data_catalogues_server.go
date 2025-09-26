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

// DataCataloguesServer implements the AIStudioManagementService for data catalogues management operations
type DataCataloguesServer struct {
	pb.UnimplementedAIStudioManagementServiceServer
	service *services.Service
}

// NewDataCataloguesServer creates a new data catalogues management gRPC server
func NewDataCataloguesServer(service *services.Service) *DataCataloguesServer {
	return &DataCataloguesServer{
		service: service,
	}
}

// ListDataCatalogues returns a list of data catalogues with pagination
func (s *DataCataloguesServer) ListDataCatalogues(ctx context.Context, req *pb.ListDataCataloguesRequest) (*pb.ListDataCataloguesResponse, error) {
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
	dataCatalogues, totalCount, _, err := s.service.GetAllDataCatalogues(limit, page, false)
	if err != nil {
		log.Error().Err(err).Msg("Failed to list data catalogues via gRPC")
		return nil, status.Errorf(codes.Internal, "failed to list data catalogues: %v", err)
	}

	// Convert service response to gRPC protobuf
	pbDataCatalogues := make([]*pb.DataCatalogueInfo, len(dataCatalogues))
	for i, dataCatalogue := range dataCatalogues {
		pbDataCatalogues[i] = convertDataCatalogueToPB(&dataCatalogue)
	}

	log.Debug().
		Int("data_catalogue_count", len(dataCatalogues)).
		Int64("total_count", totalCount).
		Msg("Listed data catalogues via gRPC")

	return &pb.ListDataCataloguesResponse{
		DataCatalogues: pbDataCatalogues,
		TotalCount:     totalCount,
	}, nil
}

// GetDataCatalogue returns details for a specific data catalogue
func (s *DataCataloguesServer) GetDataCatalogue(ctx context.Context, req *pb.GetDataCatalogueRequest) (*pb.GetDataCatalogueResponse, error) {
	dataCatalogueID := req.GetDataCatalogueId()
	if dataCatalogueID == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "data_catalogue_id is required")
	}

	// Call existing service method
	dataCatalogue, err := s.service.GetDataCatalogueByID(uint(dataCatalogueID))
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, status.Errorf(codes.NotFound, "data catalogue not found: %d", dataCatalogueID)
		}
		log.Error().Err(err).Uint32("data_catalogue_id", dataCatalogueID).Msg("Failed to get data catalogue via gRPC")
		return nil, status.Errorf(codes.Internal, "failed to get data catalogue: %v", err)
	}

	log.Debug().
		Uint32("data_catalogue_id", dataCatalogueID).
		Str("data_catalogue_name", dataCatalogue.Name).
		Msg("Retrieved data catalogue via gRPC")

	return &pb.GetDataCatalogueResponse{
		DataCatalogue: convertDataCatalogueToPB(dataCatalogue),
	}, nil
}

// CreateDataCatalogue creates a new data catalogue
func (s *DataCataloguesServer) CreateDataCatalogue(ctx context.Context, req *pb.CreateDataCatalogueRequest) (*pb.CreateDataCatalogueResponse, error) {
	// Validate required fields
	if req.GetName() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "name is required")
	}

	// Call existing service method
	dataCatalogue, err := s.service.CreateDataCatalogue(
		req.GetName(),
		req.GetShortDescription(),
		req.GetLongDescription(),
		req.GetIcon(),
	)
	if err != nil {
		log.Error().Err(err).Str("name", req.GetName()).Msg("Failed to create data catalogue via gRPC")
		return nil, status.Errorf(codes.Internal, "failed to create data catalogue: %v", err)
	}

	log.Info().
		Uint("data_catalogue_id", dataCatalogue.ID).
		Str("data_catalogue_name", dataCatalogue.Name).
		Msg("Created data catalogue via gRPC")

	return &pb.CreateDataCatalogueResponse{
		DataCatalogue: convertDataCatalogueToPB(dataCatalogue),
	}, nil
}

// convertDataCatalogueToPB converts a models.DataCatalogue to protobuf DataCatalogueInfo
func convertDataCatalogueToPB(dataCatalogue *models.DataCatalogue) *pb.DataCatalogueInfo {
	// Convert datasources - use the function from datasources_server.go
	pbDatasources := make([]*pb.DatasourceInfo, len(dataCatalogue.Datasources))
	for i, datasource := range dataCatalogue.Datasources {
		pbDatasources[i] = convertDatasourceToPB(&datasource)
	}

	// Convert tags (no description field in Tag model)
	pbTags := make([]*pb.TagInfo, len(dataCatalogue.Tags))
	for i, tag := range dataCatalogue.Tags {
		pbTags[i] = &pb.TagInfo{
			Id:        uint32(tag.ID),
			Name:      tag.Name,
			CreatedAt: timestamppb.New(tag.CreatedAt),
			UpdatedAt: timestamppb.New(tag.UpdatedAt),
		}
	}

	return &pb.DataCatalogueInfo{
		Id:               uint32(dataCatalogue.ID),
		Name:             dataCatalogue.Name,
		ShortDescription: dataCatalogue.ShortDescription,
		LongDescription:  dataCatalogue.LongDescription,
		Icon:             dataCatalogue.Icon,
		Datasources:      pbDatasources,
		Tags:             pbTags,
		CreatedAt:        timestamppb.New(dataCatalogue.CreatedAt),
		UpdatedAt:        timestamppb.New(dataCatalogue.UpdatedAt),
	}
}