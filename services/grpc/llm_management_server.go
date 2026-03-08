package grpc

import (
	"context"
	"errors"

	"github.com/TykTechnologies/midsommar/v2/models"
	pb "github.com/TykTechnologies/midsommar/v2/proto/ai_studio_management"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/gorm"
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

	// Apply filtering based on request parameters
	var llmsWithMeta []models.LLM
	var totalCount int64
	var err error

	// Check if namespace filtering is requested
	if req.GetNamespace() != "" {
		// Use namespace-aware filtering
		llmsWithMeta, err = s.service.GetActiveLLMsInNamespace(ctx, req.GetNamespace())
		if err != nil {
			log.Error().Err(err).Str("namespace", req.GetNamespace()).Msg("Failed to get LLMs by namespace via gRPC")
			return nil, status.Errorf(codes.Internal, "failed to get LLMs by namespace: %v", err)
		}
		totalCount = int64(len(llmsWithMeta))

		// Apply manual pagination since service method doesn't support it
		start := (page - 1) * limit
		end := start + limit
		if start < len(llmsWithMeta) {
			if end > len(llmsWithMeta) {
				end = len(llmsWithMeta)
			}
			llmsWithMeta = llmsWithMeta[start:end]
		} else {
			llmsWithMeta = []models.LLM{}
		}
	} else {
		// Use standard pagination without namespace filtering
		llmsWithMeta, totalCount, _, err = s.service.GetAllLLMs(limit, page, false)
		if err != nil {
			log.Error().Err(err).Msg("Failed to list LLMs via gRPC")
			return nil, status.Errorf(codes.Internal, "failed to list LLMs: %v", err)
		}
	}

	// Apply vendor filtering if requested (post-processing since no direct service method)
	if req.GetVendor() != "" {
		filteredLLMs := make([]models.LLM, 0)
		for _, llm := range llmsWithMeta {
			if string(llm.Vendor) == req.GetVendor() {
				filteredLLMs = append(filteredLLMs, llm)
			}
		}
		llmsWithMeta = filteredLLMs
		totalCount = int64(len(filteredLLMs))
	}

	// Convert service response to gRPC protobuf
	pbLLMs := make([]*pb.LLMInfo, len(llmsWithMeta))
	for i, llm := range llmsWithMeta {
		pbLLMs[i] = convertLLMToPB(&llm)
	}

	log.Debug().
		Int("llm_count", len(llmsWithMeta)).
		Int64("total_count", totalCount).
		Str("vendor_filter", req.GetVendor()).
		Str("namespace_filter", req.GetNamespace()).
		Msg("Listed LLMs with filtering via gRPC")

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
	llm, err := s.service.GetLLMByID(ctx, uint(llmID))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
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
	existingLLM, err := s.service.GetLLMByID(ctx, uint(llmID))
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "LLM not found: %d", llmID)
	}

	// Determine namespace: use from request if provided, otherwise preserve existing
	namespace := req.GetNamespace()
	if namespace == "" {
		namespace = existingLLM.Namespace
	}

	// Call existing service method with preserved vendor
	llm, err := s.service.UpdateLLM(
		ctx,
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
		nil,       // BudgetStartDate
		namespace, // Support namespace updates via gRPC
	)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
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
	err := s.service.DeleteLLM(ctx, uint(llmID))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
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

// UpdateLLMPlugins updates plugin associations for an LLM
func (s *LLMManagementServer) UpdateLLMPlugins(ctx context.Context, req *pb.UpdateLLMPluginsRequest) (*pb.UpdateLLMPluginsResponse, error) {
	llmID := req.GetLlmId()
	if llmID == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "llm_id is required")
	}

	// Convert plugin IDs from uint32 to uint
	newPluginIDs := make([]uint, len(req.GetPluginIds()))
	for i, id := range req.GetPluginIds() {
		newPluginIDs[i] = uint(id)
	}

	var finalPluginIDs []uint

	if req.GetAppend() {
		// Append mode: get existing plugins and merge with new ones
		existingPlugins, err := s.service.PluginService.GetPluginsForLLM(uint(llmID))
		if err != nil {
			log.Error().Err(err).Uint32("llm_id", llmID).Msg("Failed to get existing LLM plugins")
			return nil, status.Errorf(codes.Internal, "failed to get existing plugins: %v", err)
		}

		// Build set of existing plugin IDs
		existingSet := make(map[uint]bool)
		for _, p := range existingPlugins {
			existingSet[p.ID] = true
			finalPluginIDs = append(finalPluginIDs, p.ID)
		}

		// Add new plugins that aren't already associated
		for _, newID := range newPluginIDs {
			if !existingSet[newID] {
				finalPluginIDs = append(finalPluginIDs, newID)
			}
		}
	} else {
		// Replace mode: use new plugin IDs directly
		finalPluginIDs = newPluginIDs
	}

	// Update the LLM-plugin associations
	err := s.service.PluginService.UpdateLLMPlugins(uint(llmID), finalPluginIDs)
	if err != nil {
		log.Error().Err(err).
			Uint32("llm_id", llmID).
			Ints("plugin_ids", toIntSlice(finalPluginIDs)).
			Msg("Failed to update LLM plugins via gRPC")
		return nil, status.Errorf(codes.Internal, "failed to update LLM plugins: %v", err)
	}

	// Convert final plugin IDs back to uint32
	resultPluginIDs := make([]uint32, len(finalPluginIDs))
	for i, id := range finalPluginIDs {
		resultPluginIDs[i] = uint32(id)
	}

	log.Info().
		Uint32("llm_id", llmID).
		Ints("plugin_ids", toIntSlice(finalPluginIDs)).
		Bool("append_mode", req.GetAppend()).
		Msg("Updated LLM plugins via gRPC")

	return &pb.UpdateLLMPluginsResponse{
		Success:   true,
		Message:   "LLM plugins updated successfully",
		PluginIds: resultPluginIDs,
	}, nil
}

// toIntSlice converts []uint to []int for logging
func toIntSlice(uints []uint) []int {
	ints := make([]int, len(uints))
	for i, u := range uints {
		ints[i] = int(u)
	}
	return ints
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