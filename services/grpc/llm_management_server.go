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

// LLMManagementServer implements the AIStudioManagementService for LLM management operations
type LLMManagementServer struct {
	pb.UnimplementedAIStudioManagementServiceServer
	service *services.Service
}

// NewLLMManagementServer creates a new LLM management gRPC server
func NewLLMManagementServer(service *services.Service) *LLMManagementServer {
	return &LLMManagementServer{
		service: service,
	}
}

// ListLLMs returns a list of LLMs with filtering and pagination
func (s *LLMManagementServer) ListLLMs(ctx context.Context, req *pb.ListLLMsRequest) (*pb.ListLLMsResponse, error) {
	// Convert gRPC request parameters to service parameters
	page := int(req.GetPage())
	if page <= 0 {
		page = 1
	}
	limit := int(req.GetLimit())
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	// Call existing service method - simplified for MVP
	llmsWithMeta, totalCount, _, err := s.service.GetAllLLMs(limit, page, false)
	if err != nil {
		log.Error().Err(err).Msg("Failed to list LLMs via gRPC")
		return nil, status.Errorf(codes.Internal, "failed to list LLMs: %v", err)
	}

	// TODO: Apply vendor and namespace filtering in future versions
	// For MVP, return all LLMs

	// Convert service response to gRPC protobuf
	pbLLMs := make([]*pb.LLMInfo, len(llmsWithMeta))
	for i, llm := range llmsWithMeta {
		pbLLMs[i] = convertLLMToPB(&llm)
	}

	log.Debug().
		Int("llm_count", len(llmsWithMeta)).
		Int64("total_count", totalCount).
		Msg("Listed LLMs via gRPC")

	return &pb.ListLLMsResponse{
		Llms:       pbLLMs,
		TotalCount: totalCount,
	}, nil
}

// GetLLM returns details for a specific LLM
func (s *LLMManagementServer) GetLLM(ctx context.Context, req *pb.GetLLMRequest) (*pb.GetLLMResponse, error) {
	llmID := req.GetLlmId()
	if llmID == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "llm_id is required")
	}

	// Call existing service method
	llm, err := s.service.GetLLMByID(uint(llmID))
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, status.Errorf(codes.NotFound, "LLM not found: %d", llmID)
		}
		log.Error().Err(err).Uint32("llm_id", llmID).Msg("Failed to get LLM via gRPC")
		return nil, status.Errorf(codes.Internal, "failed to get LLM: %v", err)
	}

	log.Debug().
		Uint32("llm_id", llmID).
		Str("llm_name", llm.Name).
		Msg("Retrieved LLM via gRPC")

	return &pb.GetLLMResponse{
		Llm: convertLLMToPB(llm),
	}, nil
}

// GetLLMPlugins returns plugins associated with a specific LLM
func (s *LLMManagementServer) GetLLMPlugins(ctx context.Context, req *pb.GetLLMPluginsRequest) (*pb.GetLLMPluginsResponse, error) {
	llmID := req.GetLlmId()
	if llmID == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "llm_id is required")
	}

	// Call existing service method to get plugins for LLM
	plugins, err := s.service.PluginService.GetPluginsForLLM(uint(llmID))
	if err != nil {
		log.Error().Err(err).Uint32("llm_id", llmID).Msg("Failed to get LLM plugins via gRPC")
		return nil, status.Errorf(codes.Internal, "failed to get LLM plugins: %v", err)
	}

	// Convert to protobuf
	pbPlugins := make([]*pb.PluginInfo, len(plugins))
	for i, plugin := range plugins {
		pbPlugins[i] = convertPluginToPB(&plugin)
	}

	log.Debug().
		Uint32("llm_id", llmID).
		Int("plugin_count", len(plugins)).
		Msg("Retrieved LLM plugins via gRPC")

	return &pb.GetLLMPluginsResponse{
		Plugins: pbPlugins,
	}, nil
}

// CreateLLM creates a new LLM
func (s *LLMManagementServer) CreateLLM(ctx context.Context, req *pb.CreateLLMRequest) (*pb.CreateLLMResponse, error) {
	// Validate required fields
	if req.GetName() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "name is required")
	}
	if req.GetVendor() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "vendor is required")
	}
	if req.GetDefaultModel() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "default_model is required")
	}

	// Convert vendor string to models.Vendor
	vendor := models.Vendor(req.GetVendor())

	// Call existing service method
	llm, err := s.service.CreateLLMWithNamespace(
		req.GetName(),
		req.GetApiKey(),
		req.GetApiEndpoint(),
		int(req.GetPrivacyScore()),
		req.GetShortDescription(),
		req.GetLongDescription(),
		req.GetLogoUrl(),
		vendor,
		req.GetActive(),
		[]*models.Filter{}, // Empty filters initially
		req.GetDefaultModel(),
		req.GetAllowedModels(),
		req.MonthlyBudget,
		nil, // BudgetStartDate
		req.GetNamespace(),
	)
	if err != nil {
		log.Error().Err(err).
			Str("name", req.GetName()).
			Str("vendor", req.GetVendor()).
			Msg("Failed to create LLM via gRPC")
		return nil, status.Errorf(codes.Internal, "failed to create LLM: %v", err)
	}

	log.Info().
		Uint("llm_id", llm.ID).
		Str("llm_name", llm.Name).
		Str("vendor", string(llm.Vendor)).
		Msg("Created LLM via gRPC")

	return &pb.CreateLLMResponse{
		Llm: convertLLMToPB(llm),
	}, nil
}

// UpdateLLM updates an existing LLM
func (s *LLMManagementServer) UpdateLLM(ctx context.Context, req *pb.UpdateLLMRequest) (*pb.UpdateLLMResponse, error) {
	llmID := req.GetLlmId()
	if llmID == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "llm_id is required")
	}

	// Get existing LLM to preserve vendor (UpdateLLMRequest doesn't include vendor)
	existingLLM, err := s.service.GetLLMByID(uint(llmID))
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "LLM not found: %d", llmID)
	}

	// Call existing service method with preserved vendor
	llm, err := s.service.UpdateLLM(
		uint(llmID),
		req.GetName(),
		req.GetApiKey(),
		req.GetApiEndpoint(),
		int(req.GetPrivacyScore()),
		req.GetShortDescription(),
		req.GetLongDescription(),
		req.GetLogoUrl(),
		existingLLM.Vendor, // Preserve existing vendor
		req.GetActive(),
		[]*models.Filter{}, // Filters managed separately
		req.GetDefaultModel(),
		req.GetAllowedModels(),
		req.MonthlyBudget,
		nil, // BudgetStartDate
	)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, status.Errorf(codes.NotFound, "LLM not found: %d", llmID)
		}
		log.Error().Err(err).Uint32("llm_id", llmID).Msg("Failed to update LLM via gRPC")
		return nil, status.Errorf(codes.Internal, "failed to update LLM: %v", err)
	}

	log.Info().
		Uint32("llm_id", llmID).
		Str("llm_name", llm.Name).
		Msg("Updated LLM via gRPC")

	return &pb.UpdateLLMResponse{
		Llm: convertLLMToPB(llm),
	}, nil
}

// DeleteLLM deletes an LLM
func (s *LLMManagementServer) DeleteLLM(ctx context.Context, req *pb.DeleteLLMRequest) (*pb.DeleteLLMResponse, error) {
	llmID := req.GetLlmId()
	if llmID == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "llm_id is required")
	}

	// Call existing service method
	err := s.service.DeleteLLM(uint(llmID))
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, status.Errorf(codes.NotFound, "LLM not found: %d", llmID)
		}
		log.Error().Err(err).Uint32("llm_id", llmID).Msg("Failed to delete LLM via gRPC")
		return nil, status.Errorf(codes.Internal, "failed to delete LLM: %v", err)
	}

	log.Info().
		Uint32("llm_id", llmID).
		Msg("Deleted LLM via gRPC")

	return &pb.DeleteLLMResponse{
		Success: true,
		Message: "LLM deleted successfully",
	}, nil
}

// convertLLMToPB converts a models.LLM to protobuf LLMInfo
func convertLLMToPB(llm *models.LLM) *pb.LLMInfo {
	// Don't expose the actual API key, just indicate if it exists
	hasAPIKey := llm.APIKey != ""

	// Handle optional monthly budget
	var monthlyBudget *float64
	if llm.MonthlyBudget != nil {
		monthlyBudget = llm.MonthlyBudget
	}

	return &pb.LLMInfo{
		Id:               uint32(llm.ID),
		Name:             llm.Name,
		Vendor:           string(llm.Vendor),
		ApiEndpoint:      llm.APIEndpoint,
		HasApiKey:        hasAPIKey,
		PrivacyScore:     int32(llm.PrivacyScore),
		ShortDescription: llm.ShortDescription,
		DefaultModel:     llm.DefaultModel,
		AllowedModels:    llm.AllowedModels,
		Active:           llm.Active,
		Namespace:        llm.Namespace,
		MonthlyBudget:    monthlyBudget,
		CreatedAt:        timestamppb.New(llm.CreatedAt),
		UpdatedAt:        timestamppb.New(llm.UpdatedAt),
	}
}