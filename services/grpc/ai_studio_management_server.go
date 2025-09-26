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

// AIStudioManagementServer is the unified server that implements all AI Studio management operations
// It delegates to specialized servers for different domains
type AIStudioManagementServer struct {
	pb.UnimplementedAIStudioManagementServiceServer

	// Specialized servers for different domains
	pluginServer   *PluginManagementServer
	llmServer      *LLMManagementServer
	analyticsServer *AnalyticsServer

	// Main service for direct access
	service *services.Service
}

// NewAIStudioManagementServer creates the unified AI Studio management server
func NewAIStudioManagementServer(service *services.Service) *AIStudioManagementServer {
	return &AIStudioManagementServer{
		pluginServer:   NewPluginManagementServer(service.PluginService),
		llmServer:      NewLLMManagementServer(service),
		analyticsServer: NewAnalyticsServer(service),
		service:        service,
	}
}

// Plugin Management Operations - delegate to plugin server

func (s *AIStudioManagementServer) ListPlugins(ctx context.Context, req *pb.ListPluginsRequest) (*pb.ListPluginsResponse, error) {
	return s.pluginServer.ListPlugins(ctx, req)
}

func (s *AIStudioManagementServer) GetPlugin(ctx context.Context, req *pb.GetPluginRequest) (*pb.GetPluginResponse, error) {
	return s.pluginServer.GetPlugin(ctx, req)
}

func (s *AIStudioManagementServer) UpdatePluginConfig(ctx context.Context, req *pb.UpdatePluginConfigRequest) (*pb.UpdatePluginConfigResponse, error) {
	return s.pluginServer.UpdatePluginConfig(ctx, req)
}

// LLM Management Operations - delegate to LLM server

func (s *AIStudioManagementServer) ListLLMs(ctx context.Context, req *pb.ListLLMsRequest) (*pb.ListLLMsResponse, error) {
	return s.llmServer.ListLLMs(ctx, req)
}

func (s *AIStudioManagementServer) GetLLM(ctx context.Context, req *pb.GetLLMRequest) (*pb.GetLLMResponse, error) {
	return s.llmServer.GetLLM(ctx, req)
}

func (s *AIStudioManagementServer) GetLLMPlugins(ctx context.Context, req *pb.GetLLMPluginsRequest) (*pb.GetLLMPluginsResponse, error) {
	return s.llmServer.GetLLMPlugins(ctx, req)
}

// Analytics Operations - delegate to analytics server

func (s *AIStudioManagementServer) GetAnalyticsSummary(ctx context.Context, req *pb.GetAnalyticsSummaryRequest) (*pb.GetAnalyticsSummaryResponse, error) {
	return s.analyticsServer.GetAnalyticsSummary(ctx, req)
}

func (s *AIStudioManagementServer) GetUsageStatistics(ctx context.Context, req *pb.GetUsageStatisticsRequest) (*pb.GetUsageStatisticsResponse, error) {
	return s.analyticsServer.GetUsageStatistics(ctx, req)
}

func (s *AIStudioManagementServer) GetCostAnalysis(ctx context.Context, req *pb.GetCostAnalysisRequest) (*pb.GetCostAnalysisResponse, error) {
	return s.analyticsServer.GetCostAnalysis(ctx, req)
}

// App Management Operations - implement directly

func (s *AIStudioManagementServer) ListApps(ctx context.Context, req *pb.ListAppsRequest) (*pb.ListAppsResponse, error) {
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
	apps, totalCount, _, err := s.service.ListAppsWithPagination(limit, page, false, "created_at DESC")
	if err != nil {
		log.Error().Err(err).Msg("Failed to list apps via gRPC")
		return nil, status.Errorf(codes.Internal, "failed to list apps: %v", err)
	}

	// TODO: Apply namespace and isActive filtering in future versions
	// For MVP, return all apps

	// Convert service response to gRPC protobuf
	pbApps := make([]*pb.AppInfo, len(apps))
	for i, app := range apps {
		pbApps[i] = convertAppToPB(&app)
	}

	log.Debug().
		Int("app_count", len(apps)).
		Int64("total_count", totalCount).
		Msg("Listed apps via gRPC")

	return &pb.ListAppsResponse{
		Apps:       pbApps,
		TotalCount: totalCount,
	}, nil
}

func (s *AIStudioManagementServer) GetApp(ctx context.Context, req *pb.GetAppRequest) (*pb.GetAppResponse, error) {
	appID := req.GetAppId()
	if appID == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "app_id is required")
	}

	// Call existing service method
	app, err := s.service.GetAppByID(uint(appID))
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, status.Errorf(codes.NotFound, "app not found: %d", appID)
		}
		log.Error().Err(err).Uint32("app_id", appID).Msg("Failed to get app via gRPC")
		return nil, status.Errorf(codes.Internal, "failed to get app: %v", err)
	}

	log.Debug().
		Uint32("app_id", appID).
		Str("app_name", app.Name).
		Msg("Retrieved app via gRPC")

	return &pb.GetAppResponse{
		App: convertAppToPB(app),
	}, nil
}

// convertAppToPB converts a models.App to protobuf AppInfo
func convertAppToPB(app *models.App) *pb.AppInfo {
	// Handle optional monthly budget
	var monthlyBudget *float64
	if app.MonthlyBudget != nil {
		monthlyBudget = app.MonthlyBudget
	}

	// Extract LLM IDs from relationships
	llmIDs := make([]uint32, len(app.LLMs))
	for i, llm := range app.LLMs {
		llmIDs[i] = uint32(llm.ID)
	}

	return &pb.AppInfo{
		Id:            uint32(app.ID),
		Name:          app.Name,
		Description:   app.Description,
		IsActive:      app.IsActive,
		Namespace:     app.Namespace,
		MonthlyBudget: monthlyBudget,
		LlmIds:        llmIDs,
		CreatedAt:     timestamppb.New(app.CreatedAt),
		UpdatedAt:     timestamppb.New(app.UpdatedAt),
	}
}