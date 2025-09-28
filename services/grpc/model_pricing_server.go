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

// ModelPricingServer implements the AIStudioManagementService for model pricing operations
type ModelPricingServer struct {
	pb.UnimplementedAIStudioManagementServiceServer
	service *services.Service
}

// NewModelPricingServer creates a new model pricing gRPC server
func NewModelPricingServer(service *services.Service) *ModelPricingServer {
	return &ModelPricingServer{
		service: service,
	}
}

// ListModelPrices returns a list of model prices with filtering and pagination
func (s *ModelPricingServer) ListModelPrices(ctx context.Context, req *pb.ListModelPricesRequest) (*pb.ListModelPricesResponse, error) {
	// Convert gRPC request parameters to service parameters
	page := int(req.GetPage())
	if page <= 0 {
		page = 1
	}
	limit := int(req.GetLimit())
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	vendor := req.GetVendor()

	// Call existing service method
	var modelPrices models.ModelPrices
	var totalCount int64
	var err error

	if vendor != "" {
		// Filter by vendor
		modelPrices, err = s.service.GetModelPricesByVendor(vendor)
		totalCount = int64(len(modelPrices))
	} else {
		// Get all model prices
		modelPrices, totalCount, _, err = s.service.GetAllModelPrices(limit, page, false)
	}

	if err != nil {
		log.Error().Err(err).Msg("Failed to list model prices via gRPC")
		return nil, status.Errorf(codes.Internal, "failed to list model prices: %v", err)
	}

	// Convert service response to gRPC protobuf
	pbModelPrices := make([]*pb.ModelPriceInfo, len(modelPrices))
	for i, modelPrice := range modelPrices {
		pbModelPrices[i] = convertModelPriceToPB(&modelPrice)
	}

	log.Debug().
		Int("model_price_count", len(modelPrices)).
		Int64("total_count", totalCount).
		Str("vendor_filter", vendor).
		Msg("Listed model prices via gRPC")

	return &pb.ListModelPricesResponse{
		ModelPrices: pbModelPrices,
		TotalCount:  totalCount,
	}, nil
}

// GetModelPrice returns details for a specific model price
func (s *ModelPricingServer) GetModelPrice(ctx context.Context, req *pb.GetModelPriceRequest) (*pb.GetModelPriceResponse, error) {
	modelPriceID := req.GetModelPriceId()
	if modelPriceID == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "model_price_id is required")
	}

	// Call existing service method
	modelPrice, err := s.service.GetModelPriceByID(uint(modelPriceID))
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, status.Errorf(codes.NotFound, "model price not found: %d", modelPriceID)
		}
		log.Error().Err(err).Uint32("model_price_id", modelPriceID).Msg("Failed to get model price via gRPC")
		return nil, status.Errorf(codes.Internal, "failed to get model price: %v", err)
	}

	log.Debug().
		Uint32("model_price_id", modelPriceID).
		Str("model_name", modelPrice.ModelName).
		Str("vendor", modelPrice.Vendor).
		Msg("Retrieved model price via gRPC")

	return &pb.GetModelPriceResponse{
		ModelPrice: convertModelPriceToPB(modelPrice),
	}, nil
}

// CreateModelPrice creates a new model price
func (s *ModelPricingServer) CreateModelPrice(ctx context.Context, req *pb.CreateModelPriceRequest) (*pb.CreateModelPriceResponse, error) {
	// Validate required fields
	if req.GetModelName() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "model_name is required")
	}
	if req.GetVendor() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "vendor is required")
	}

	// Call existing service method
	modelPrice, err := s.service.CreateModelPrice(
		req.GetModelName(),
		req.GetVendor(),
		req.GetCpt(),
		req.GetCpit(),
		req.GetCacheWritePt(),
		req.GetCacheReadPt(),
		req.GetCurrency(),
	)
	if err != nil {
		log.Error().Err(err).
			Str("model_name", req.GetModelName()).
			Str("vendor", req.GetVendor()).
			Msg("Failed to create model price via gRPC")
		return nil, status.Errorf(codes.Internal, "failed to create model price: %v", err)
	}

	log.Info().
		Uint("model_price_id", modelPrice.ID).
		Str("model_name", modelPrice.ModelName).
		Str("vendor", modelPrice.Vendor).
		Msg("Created model price via gRPC")

	return &pb.CreateModelPriceResponse{
		ModelPrice: convertModelPriceToPB(modelPrice),
	}, nil
}

// UpdateModelPrice updates an existing model price
func (s *ModelPricingServer) UpdateModelPrice(ctx context.Context, req *pb.UpdateModelPriceRequest) (*pb.UpdateModelPriceResponse, error) {
	modelPriceID := req.GetModelPriceId()
	if modelPriceID == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "model_price_id is required")
	}

	// Call existing service method
	modelPrice, err := s.service.UpdateModelPrice(
		uint(modelPriceID),
		req.GetModelName(),
		req.GetVendor(),
		req.GetCpt(),
		req.GetCpit(),
		req.GetCacheWritePt(),
		req.GetCacheReadPt(),
		req.GetCurrency(),
	)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, status.Errorf(codes.NotFound, "model price not found: %d", modelPriceID)
		}
		log.Error().Err(err).Uint32("model_price_id", modelPriceID).Msg("Failed to update model price via gRPC")
		return nil, status.Errorf(codes.Internal, "failed to update model price: %v", err)
	}

	log.Info().
		Uint32("model_price_id", modelPriceID).
		Str("model_name", modelPrice.ModelName).
		Str("vendor", modelPrice.Vendor).
		Msg("Updated model price via gRPC")

	return &pb.UpdateModelPriceResponse{
		ModelPrice: convertModelPriceToPB(modelPrice),
	}, nil
}

// DeleteModelPrice deletes a model price
func (s *ModelPricingServer) DeleteModelPrice(ctx context.Context, req *pb.DeleteModelPriceRequest) (*pb.DeleteModelPriceResponse, error) {
	modelPriceID := req.GetModelPriceId()
	if modelPriceID == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "model_price_id is required")
	}

	// Call existing service method
	err := s.service.DeleteModelPrice(uint(modelPriceID))
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, status.Errorf(codes.NotFound, "model price not found: %d", modelPriceID)
		}
		log.Error().Err(err).Uint32("model_price_id", modelPriceID).Msg("Failed to delete model price via gRPC")
		return nil, status.Errorf(codes.Internal, "failed to delete model price: %v", err)
	}

	log.Info().
		Uint32("model_price_id", modelPriceID).
		Msg("Deleted model price via gRPC")

	return &pb.DeleteModelPriceResponse{
		Success: true,
		Message: "Model price deleted successfully",
	}, nil
}

// GetModelPricesByVendor returns model prices for a specific vendor
func (s *ModelPricingServer) GetModelPricesByVendor(ctx context.Context, req *pb.GetModelPricesByVendorRequest) (*pb.GetModelPricesByVendorResponse, error) {
	vendor := req.GetVendor()
	if vendor == "" {
		return nil, status.Errorf(codes.InvalidArgument, "vendor is required")
	}

	// Call existing service method
	modelPrices, err := s.service.GetModelPricesByVendor(vendor)
	if err != nil {
		log.Error().Err(err).Str("vendor", vendor).Msg("Failed to get model prices by vendor via gRPC")
		return nil, status.Errorf(codes.Internal, "failed to get model prices by vendor: %v", err)
	}

	// Convert service response to gRPC protobuf
	pbModelPrices := make([]*pb.ModelPriceInfo, len(modelPrices))
	for i, modelPrice := range modelPrices {
		pbModelPrices[i] = convertModelPriceToPB(&modelPrice)
	}

	log.Debug().
		Str("vendor", vendor).
		Int("model_price_count", len(modelPrices)).
		Msg("Retrieved model prices by vendor via gRPC")

	return &pb.GetModelPricesByVendorResponse{
		ModelPrices: pbModelPrices,
	}, nil
}

// convertModelPriceToPB converts a models.ModelPrice to protobuf ModelPriceInfo
func convertModelPriceToPB(modelPrice *models.ModelPrice) *pb.ModelPriceInfo {
	return &pb.ModelPriceInfo{
		Id:           uint32(modelPrice.ID),
		ModelName:    modelPrice.ModelName,
		Vendor:       modelPrice.Vendor,
		Cpt:          modelPrice.CPT,
		Cpit:         modelPrice.CPIT,
		CacheWritePt: modelPrice.CacheWritePT,
		CacheReadPt:  modelPrice.CacheReadPT,
		Currency:     modelPrice.Currency,
		CreatedAt:    timestamppb.New(modelPrice.CreatedAt),
		UpdatedAt:    timestamppb.New(modelPrice.UpdatedAt),
	}
}