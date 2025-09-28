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
	pluginServer       *PluginManagementServer
	llmServer          *LLMManagementServer
	analyticsServer    *AnalyticsServer
	toolsServer        *ToolsServer
	datasourcesServer  *DatasourcesServer
	dataCataloguesServer *DataCataloguesServer
	tagsServer         *TagsServer

	// Main service for direct access
	service *services.Service
}

// NewAIStudioManagementServer creates the unified AI Studio management server
func NewAIStudioManagementServer(service *services.Service) *AIStudioManagementServer {
	return &AIStudioManagementServer{
		pluginServer:       NewPluginManagementServer(service.PluginService),
		llmServer:          NewLLMManagementServer(service),
		analyticsServer:    NewAnalyticsServer(service),
		toolsServer:        NewToolsServer(service),
		datasourcesServer:  NewDatasourcesServer(service),
		dataCataloguesServer: NewDataCataloguesServer(service),
		tagsServer:         NewTagsServer(service),
		service:            service,
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

func (s *AIStudioManagementServer) CreateLLM(ctx context.Context, req *pb.CreateLLMRequest) (*pb.CreateLLMResponse, error) {
	return s.llmServer.CreateLLM(ctx, req)
}

func (s *AIStudioManagementServer) UpdateLLM(ctx context.Context, req *pb.UpdateLLMRequest) (*pb.UpdateLLMResponse, error) {
	return s.llmServer.UpdateLLM(ctx, req)
}

func (s *AIStudioManagementServer) DeleteLLM(ctx context.Context, req *pb.DeleteLLMRequest) (*pb.DeleteLLMResponse, error) {
	return s.llmServer.DeleteLLM(ctx, req)
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

// Detailed Analytics Operations - delegate to analytics server

func (s *AIStudioManagementServer) GetChatRecordsPerDay(ctx context.Context, req *pb.GetChatRecordsPerDayRequest) (*pb.GetChatRecordsPerDayResponse, error) {
	return s.analyticsServer.GetChatRecordsPerDay(ctx, req)
}

func (s *AIStudioManagementServer) GetModelUsage(ctx context.Context, req *pb.GetModelUsageRequest) (*pb.GetModelUsageResponse, error) {
	return s.analyticsServer.GetModelUsage(ctx, req)
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

// CreateApp creates a new app
func (s *AIStudioManagementServer) CreateApp(ctx context.Context, req *pb.CreateAppRequest) (*pb.CreateAppResponse, error) {
	// Validate required fields
	if req.GetName() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "name is required")
	}

	// Call existing service method
	app, err := s.service.CreateApp(
		req.GetName(),
		req.GetDescription(),
		uint(req.GetUserId()),
		[]uint{}, // DatasourceIDs - convert from req.GetDatasourceIds()
		[]uint{}, // LLMIDs - convert from req.GetLlmIds()
		[]uint{}, // ToolIDs - convert from req.GetToolIds()
		req.MonthlyBudget,
		nil, // BudgetStartDate
	)
	if err != nil {
		log.Error().Err(err).
			Str("name", req.GetName()).
			Uint32("user_id", req.GetUserId()).
			Msg("Failed to create app via gRPC")
		return nil, status.Errorf(codes.Internal, "failed to create app: %v", err)
	}

	log.Info().
		Uint("app_id", app.ID).
		Str("app_name", app.Name).
		Uint32("user_id", req.GetUserId()).
		Msg("Created app via gRPC")

	return &pb.CreateAppResponse{
		App: convertAppToPB(app),
	}, nil
}

// UpdateApp updates an existing app
func (s *AIStudioManagementServer) UpdateApp(ctx context.Context, req *pb.UpdateAppRequest) (*pb.UpdateAppResponse, error) {
	appID := req.GetAppId()
	if appID == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "app_id is required")
	}

	// Convert uint32 arrays to uint arrays
	llmIDs := make([]uint, len(req.GetLlmIds()))
	for i, id := range req.GetLlmIds() {
		llmIDs[i] = uint(id)
	}
	toolIDs := make([]uint, len(req.GetToolIds()))
	for i, id := range req.GetToolIds() {
		toolIDs[i] = uint(id)
	}

	// Call existing service method with full parameters
	app, err := s.service.UpdateApp(
		uint(appID),
		req.GetName(),
		req.GetDescription(),
		0, // userID - not updated via this method
		[]uint{}, // datasourceIDs - not in update request
		llmIDs,
		toolIDs,
		req.MonthlyBudget,
		nil, // budgetStartDate
	)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, status.Errorf(codes.NotFound, "app not found: %d", appID)
		}
		log.Error().Err(err).Uint32("app_id", appID).Msg("Failed to update app via gRPC")
		return nil, status.Errorf(codes.Internal, "failed to update app: %v", err)
	}

	log.Info().
		Uint32("app_id", appID).
		Str("app_name", app.Name).
		Msg("Updated app via gRPC")

	return &pb.UpdateAppResponse{
		App: convertAppToPB(app),
	}, nil
}

// DeleteApp deletes an app
func (s *AIStudioManagementServer) DeleteApp(ctx context.Context, req *pb.DeleteAppRequest) (*pb.DeleteAppResponse, error) {
	appID := req.GetAppId()
	if appID == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "app_id is required")
	}

	// Call existing service method
	err := s.service.DeleteApp(uint(appID))
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, status.Errorf(codes.NotFound, "app not found: %d", appID)
		}
		log.Error().Err(err).Uint32("app_id", appID).Msg("Failed to delete app via gRPC")
		return nil, status.Errorf(codes.Internal, "failed to delete app: %v", err)
	}

	log.Info().
		Uint32("app_id", appID).
		Msg("Deleted app via gRPC")

	return &pb.DeleteAppResponse{
		Success: true,
		Message: "App deleted successfully",
	}, nil
}

// Tool Management Operations - delegate to tools server

func (s *AIStudioManagementServer) ListTools(ctx context.Context, req *pb.ListToolsRequest) (*pb.ListToolsResponse, error) {
	return s.toolsServer.ListTools(ctx, req)
}

func (s *AIStudioManagementServer) GetTool(ctx context.Context, req *pb.GetToolRequest) (*pb.GetToolResponse, error) {
	return s.toolsServer.GetTool(ctx, req)
}

func (s *AIStudioManagementServer) GetToolOperations(ctx context.Context, req *pb.GetToolOperationsRequest) (*pb.GetToolOperationsResponse, error) {
	return s.toolsServer.GetToolOperations(ctx, req)
}

func (s *AIStudioManagementServer) CallToolOperation(ctx context.Context, req *pb.CallToolOperationRequest) (*pb.CallToolOperationResponse, error) {
	return s.toolsServer.CallToolOperation(ctx, req)
}

// Datasource Management Operations - delegate to datasources server

func (s *AIStudioManagementServer) ListDatasources(ctx context.Context, req *pb.ListDatasourcesRequest) (*pb.ListDatasourcesResponse, error) {
	return s.datasourcesServer.ListDatasources(ctx, req)
}

func (s *AIStudioManagementServer) GetDatasource(ctx context.Context, req *pb.GetDatasourceRequest) (*pb.GetDatasourceResponse, error) {
	return s.datasourcesServer.GetDatasource(ctx, req)
}

func (s *AIStudioManagementServer) CreateDatasource(ctx context.Context, req *pb.CreateDatasourceRequest) (*pb.CreateDatasourceResponse, error) {
	return s.datasourcesServer.CreateDatasource(ctx, req)
}

func (s *AIStudioManagementServer) SearchDatasources(ctx context.Context, req *pb.SearchDatasourcesRequest) (*pb.SearchDatasourcesResponse, error) {
	return s.datasourcesServer.SearchDatasources(ctx, req)
}

// Data Catalogues Management Operations - delegate to data catalogues server

func (s *AIStudioManagementServer) ListDataCatalogues(ctx context.Context, req *pb.ListDataCataloguesRequest) (*pb.ListDataCataloguesResponse, error) {
	return s.dataCataloguesServer.ListDataCatalogues(ctx, req)
}

func (s *AIStudioManagementServer) GetDataCatalogue(ctx context.Context, req *pb.GetDataCatalogueRequest) (*pb.GetDataCatalogueResponse, error) {
	return s.dataCataloguesServer.GetDataCatalogue(ctx, req)
}

func (s *AIStudioManagementServer) CreateDataCatalogue(ctx context.Context, req *pb.CreateDataCatalogueRequest) (*pb.CreateDataCatalogueResponse, error) {
	return s.dataCataloguesServer.CreateDataCatalogue(ctx, req)
}

// Tags Management Operations - delegate to tags server

func (s *AIStudioManagementServer) ListTags(ctx context.Context, req *pb.ListTagsRequest) (*pb.ListTagsResponse, error) {
	return s.tagsServer.ListTags(ctx, req)
}

func (s *AIStudioManagementServer) GetTag(ctx context.Context, req *pb.GetTagRequest) (*pb.GetTagResponse, error) {
	return s.tagsServer.GetTag(ctx, req)
}

func (s *AIStudioManagementServer) CreateTag(ctx context.Context, req *pb.CreateTagRequest) (*pb.CreateTagResponse, error) {
	return s.tagsServer.CreateTag(ctx, req)
}

func (s *AIStudioManagementServer) SearchTags(ctx context.Context, req *pb.SearchTagsRequest) (*pb.SearchTagsResponse, error) {
	return s.tagsServer.SearchTags(ctx, req)
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