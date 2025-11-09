package grpc

import (
	"context"

	pb "github.com/TykTechnologies/midsommar/v2/proto/ai_studio_management"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// VendorsServer implements the AIStudioManagementService for vendor information operations
type VendorsServer struct {
	pb.UnimplementedAIStudioManagementServiceServer
	service *services.Service
}

// NewVendorsServer creates a new vendor information gRPC server
func NewVendorsServer(service *services.Service) *VendorsServer {
	return &VendorsServer{
		service: service,
	}
}

// GetAvailableLLMDrivers returns available LLM drivers
func (s *VendorsServer) GetAvailableLLMDrivers(ctx context.Context, req *pb.GetAvailableLLMDriversRequest) (*pb.GetAvailableLLMDriversResponse, error) {
	// Call real service method to get actual available drivers
	driverInfos, err := s.service.GetAvailableLLMDrivers()
	if err != nil {
		log.Error().Err(err).Msg("Failed to get available LLM drivers via gRPC")
		return nil, status.Errorf(codes.Internal, "failed to get available LLM drivers: %v", err)
	}

	// Convert to protobuf format
	drivers := make([]*pb.VendorDriverInfo, len(driverInfos))
	for i, driver := range driverInfos {
		drivers[i] = &pb.VendorDriverInfo{
			Name:              driver.Name,
			Vendor:            driver.Vendor,
			Version:           driver.Version,
			Description:       driver.Description,
			SupportedFeatures: driver.SupportedFeatures,
		}
	}

	log.Debug().
		Int("driver_count", len(drivers)).
		Msg("Retrieved available LLM drivers via gRPC")

	return &pb.GetAvailableLLMDriversResponse{
		Drivers: drivers,
	}, nil
}

// GetAvailableEmbedders returns available embedders
func (s *VendorsServer) GetAvailableEmbedders(ctx context.Context, req *pb.GetAvailableEmbeddersRequest) (*pb.GetAvailableEmbeddersResponse, error) {
	// Call real service method to get actual available embedders
	embedderInfos, err := s.service.GetAvailableEmbedders()
	if err != nil {
		log.Error().Err(err).Msg("Failed to get available embedders via gRPC")
		return nil, status.Errorf(codes.Internal, "failed to get available embedders: %v", err)
	}

	// Convert to protobuf format
	embedders := make([]*pb.VendorDriverInfo, len(embedderInfos))
	for i, embedder := range embedderInfos {
		embedders[i] = &pb.VendorDriverInfo{
			Name:              embedder.Name,
			Vendor:            embedder.Vendor,
			Version:           embedder.Version,
			Description:       embedder.Description,
			SupportedFeatures: embedder.SupportedFeatures,
		}
	}

	log.Debug().
		Int("embedder_count", len(embedders)).
		Msg("Retrieved available embedders via gRPC")

	return &pb.GetAvailableEmbeddersResponse{
		Embedders: embedders,
	}, nil
}

// GetAvailableVectorStores returns available vector stores
func (s *VendorsServer) GetAvailableVectorStores(ctx context.Context, req *pb.GetAvailableVectorStoresRequest) (*pb.GetAvailableVectorStoresResponse, error) {
	// Call real service method to get actual available vector stores
	vectorStoreInfos, err := s.service.GetAvailableVectorStores()
	if err != nil {
		log.Error().Err(err).Msg("Failed to get available vector stores via gRPC")
		return nil, status.Errorf(codes.Internal, "failed to get available vector stores: %v", err)
	}

	// Convert to protobuf format
	vectorStores := make([]*pb.VendorDriverInfo, len(vectorStoreInfos))
	for i, vectorStore := range vectorStoreInfos {
		vectorStores[i] = &pb.VendorDriverInfo{
			Name:              vectorStore.Name,
			Vendor:            vectorStore.Vendor,
			Version:           vectorStore.Version,
			Description:       vectorStore.Description,
			SupportedFeatures: vectorStore.SupportedFeatures,
		}
	}

	log.Debug().
		Int("vector_store_count", len(vectorStores)).
		Msg("Retrieved available vector stores via gRPC")

	return &pb.GetAvailableVectorStoresResponse{
		VectorStores: vectorStores,
	}, nil
}