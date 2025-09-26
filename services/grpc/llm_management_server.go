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