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

// init registers the AIStudioManagementServer factory with the services package
// This avoids circular import (services -> services/grpc)
func init() {
	services.NewAIStudioManagementServerFunc = func(svc *services.Service) interface{} {
		return NewAIStudioManagementServer(svc)
	}
}

// AIStudioManagementServer is the unified server that implements all AI Studio management operations
// It delegates to specialized servers for different domains
// This server is used for plugin-to-host communication via go-plugin's broker pattern
type AIStudioManagementServer struct {
	pb.UnimplementedAIStudioManagementServiceServer

	// Specialized servers for different domains
	pluginServer         *PluginManagementServer
	llmServer           *LLMManagementServer
	toolsServer         *ToolsServer
	datasourcesServer   *DatasourcesServer
	dataCataloguesServer *DataCataloguesServer
	tagsServer          *TagsServer
	filtersServer       *FiltersServer
	vendorsServer       *VendorsServer
	modelPricingServer  *ModelPricingServer
	pluginKVServer      *PluginKVServer

	// Note: Analytics server removed - analytics functionality not available to plugins

	// Main service for direct access
	service *services.Service
}

// validatePluginScope validates that the calling plugin has the required scope
// Returns the plugin model for downstream use, or an error if validation fails
func (s *AIStudioManagementServer) validatePluginScope(ctx context.Context, requiredScope string) (*models.Plugin, error) {
	// Extract plugin ID from context (injected by plugin ID interceptor)
	pluginID := GetPluginIDFromContext(ctx)
	if pluginID == 0 {
		log.Error().Msg("Plugin ID not found in context during scope validation")
		return nil, status.Errorf(codes.Unauthenticated, "plugin authentication required")
	}

	// Load plugin from database
	var plugin models.Plugin
	if err := s.service.GetDB().First(&plugin, pluginID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			log.Error().Uint("plugin_id", pluginID).Msg("Plugin not found during scope validation")
			return nil, status.Errorf(codes.Unauthenticated, "plugin not found")
		}
		log.Error().Err(err).Uint("plugin_id", pluginID).Msg("Database error during scope validation")
		return nil, status.Errorf(codes.Internal, "authentication error")
	}

	// Check if plugin has service access authorized
	if !plugin.HasServiceAccess() {
		log.Warn().
			Uint("plugin_id", pluginID).
			Str("plugin_name", plugin.Name).
			Msg("Plugin service access not authorized")
		return nil, status.Errorf(codes.PermissionDenied, "service access not authorized for plugin %s", plugin.Name)
	}

	// Check scope authorization
	if !plugin.HasServiceScope(requiredScope) {
		log.Warn().
			Uint("plugin_id", pluginID).
			Str("plugin_name", plugin.Name).
			Str("required_scope", requiredScope).
			Strs("plugin_scopes", plugin.ServiceScopes).
			Msg("Plugin missing required scope")
		return nil, status.Errorf(codes.PermissionDenied, "insufficient scope: %s (plugin: %s)", requiredScope, plugin.Name)
	}

	log.Debug().
		Uint("plugin_id", pluginID).
		Str("plugin_name", plugin.Name).
		Str("scope", requiredScope).
		Msg("Plugin scope validated successfully")

	// Return the validated plugin - caller will inject it into context
	return &plugin, nil
}

// NewAIStudioManagementServer creates the unified AI Studio management server
func NewAIStudioManagementServer(service *services.Service) *AIStudioManagementServer {
	// Create plugin KV service
	pluginKVService := services.NewPluginKVService(service.GetDB())

	return &AIStudioManagementServer{
		pluginServer:         NewPluginManagementServer(service.PluginService),
		llmServer:           NewLLMManagementServer(service),
		toolsServer:         NewToolsServer(service),
		datasourcesServer:   NewDatasourcesServer(service),
		dataCataloguesServer: NewDataCataloguesServer(service),
		tagsServer:          NewTagsServer(service),
		filtersServer:       NewFiltersServer(service),
		vendorsServer:       NewVendorsServer(service),
		modelPricingServer:  NewModelPricingServer(service),
		pluginKVServer:      NewPluginKVServer(pluginKVService),
		service:            service,
		// Note: Analytics server removed - analytics functionality not available to plugins
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

// Analytics Operations are not available to plugins
// Analytics logic remains in the REST API layer for architectural consistency

func (s *AIStudioManagementServer) GetAnalyticsSummary(ctx context.Context, req *pb.GetAnalyticsSummaryRequest) (*pb.GetAnalyticsSummaryResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "analytics functionality is not available to plugins")
}

func (s *AIStudioManagementServer) GetUsageStatistics(ctx context.Context, req *pb.GetUsageStatisticsRequest) (*pb.GetUsageStatisticsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "analytics functionality is not available to plugins")
}

func (s *AIStudioManagementServer) GetCostAnalysis(ctx context.Context, req *pb.GetCostAnalysisRequest) (*pb.GetCostAnalysisResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "analytics functionality is not available to plugins")
}

func (s *AIStudioManagementServer) GetChatRecordsPerDay(ctx context.Context, req *pb.GetChatRecordsPerDayRequest) (*pb.GetChatRecordsPerDayResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "analytics functionality is not available to plugins")
}

func (s *AIStudioManagementServer) GetModelUsage(ctx context.Context, req *pb.GetModelUsageRequest) (*pb.GetModelUsageResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "analytics functionality is not available to plugins")
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

	namespace := req.GetNamespace()
	// Empty namespace means "all namespaces" - no need to change it

	// Handle is_active parameter
	var isActive *bool
	if req.IsActive != nil {
		value := req.GetIsActive()
		isActive = &value
	}

	// Call enhanced service method with filtering
	apps, totalCount, _, err := s.service.ListAppsWithFilters(limit, page, false, "-created_at", namespace, isActive)
	if err != nil {
		log.Error().Err(err).Msg("Failed to list apps via gRPC")
		return nil, status.Errorf(codes.Internal, "failed to list apps: %v", err)
	}

	// Convert service response to gRPC protobuf
	pbApps := make([]*pb.AppInfo, len(apps))
	for i, app := range apps {
		pbApps[i] = convertAppToPB(&app)
	}

	log.Debug().
		Int("app_count", len(apps)).
		Int64("total_count", totalCount).
		Str("namespace", namespace).
		Interface("is_active", isActive).
		Msg("Listed apps with filtering via gRPC")

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
		if errors.Is(err, gorm.ErrRecordNotFound) {
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
		if errors.Is(err, gorm.ErrRecordNotFound) {
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
		if errors.Is(err, gorm.ErrRecordNotFound) {
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

// Missing Tool CRUD Operations - delegate to tools server

func (s *AIStudioManagementServer) CreateTool(ctx context.Context, req *pb.CreateToolRequest) (*pb.CreateToolResponse, error) {
	return s.toolsServer.CreateTool(ctx, req)
}

func (s *AIStudioManagementServer) UpdateTool(ctx context.Context, req *pb.UpdateToolRequest) (*pb.UpdateToolResponse, error) {
	return s.toolsServer.UpdateTool(ctx, req)
}

func (s *AIStudioManagementServer) DeleteTool(ctx context.Context, req *pb.DeleteToolRequest) (*pb.DeleteToolResponse, error) {
	return s.toolsServer.DeleteTool(ctx, req)
}

// Missing Data Management CRUD Operations - delegate to respective servers

func (s *AIStudioManagementServer) UpdateDatasource(ctx context.Context, req *pb.UpdateDatasourceRequest) (*pb.UpdateDatasourceResponse, error) {
	return s.datasourcesServer.UpdateDatasource(ctx, req)
}

func (s *AIStudioManagementServer) DeleteDatasource(ctx context.Context, req *pb.DeleteDatasourceRequest) (*pb.DeleteDatasourceResponse, error) {
	return s.datasourcesServer.DeleteDatasource(ctx, req)
}

func (s *AIStudioManagementServer) ProcessDatasourceEmbeddings(ctx context.Context, req *pb.ProcessEmbeddingsRequest) (*pb.ProcessEmbeddingsResponse, error) {
	return s.datasourcesServer.ProcessDatasourceEmbeddings(ctx, req)
}

func (s *AIStudioManagementServer) UpdateDataCatalogue(ctx context.Context, req *pb.UpdateDataCatalogueRequest) (*pb.UpdateDataCatalogueResponse, error) {
	return s.dataCataloguesServer.UpdateDataCatalogue(ctx, req)
}

func (s *AIStudioManagementServer) DeleteDataCatalogue(ctx context.Context, req *pb.DeleteDataCatalogueRequest) (*pb.DeleteDataCatalogueResponse, error) {
	return s.dataCataloguesServer.DeleteDataCatalogue(ctx, req)
}

func (s *AIStudioManagementServer) UpdateTag(ctx context.Context, req *pb.UpdateTagRequest) (*pb.UpdateTagResponse, error) {
	return s.tagsServer.UpdateTag(ctx, req)
}

func (s *AIStudioManagementServer) DeleteTag(ctx context.Context, req *pb.DeleteTagRequest) (*pb.DeleteTagResponse, error) {
	return s.tagsServer.DeleteTag(ctx, req)
}

// Filter Management Operations - delegate to filters server

func (s *AIStudioManagementServer) ListFilters(ctx context.Context, req *pb.ListFiltersRequest) (*pb.ListFiltersResponse, error) {
	return s.filtersServer.ListFilters(ctx, req)
}

func (s *AIStudioManagementServer) GetFilter(ctx context.Context, req *pb.GetFilterRequest) (*pb.GetFilterResponse, error) {
	return s.filtersServer.GetFilter(ctx, req)
}

func (s *AIStudioManagementServer) CreateFilter(ctx context.Context, req *pb.CreateFilterRequest) (*pb.CreateFilterResponse, error) {
	return s.filtersServer.CreateFilter(ctx, req)
}

func (s *AIStudioManagementServer) UpdateFilter(ctx context.Context, req *pb.UpdateFilterRequest) (*pb.UpdateFilterResponse, error) {
	return s.filtersServer.UpdateFilter(ctx, req)
}

func (s *AIStudioManagementServer) DeleteFilter(ctx context.Context, req *pb.DeleteFilterRequest) (*pb.DeleteFilterResponse, error) {
	return s.filtersServer.DeleteFilter(ctx, req)
}

// Vendor Information Operations - delegate to vendors server

func (s *AIStudioManagementServer) GetAvailableLLMDrivers(ctx context.Context, req *pb.GetAvailableLLMDriversRequest) (*pb.GetAvailableLLMDriversResponse, error) {
	return s.vendorsServer.GetAvailableLLMDrivers(ctx, req)
}

func (s *AIStudioManagementServer) GetAvailableEmbedders(ctx context.Context, req *pb.GetAvailableEmbeddersRequest) (*pb.GetAvailableEmbeddersResponse, error) {
	return s.vendorsServer.GetAvailableEmbedders(ctx, req)
}

func (s *AIStudioManagementServer) GetAvailableVectorStores(ctx context.Context, req *pb.GetAvailableVectorStoresRequest) (*pb.GetAvailableVectorStoresResponse, error) {
	return s.vendorsServer.GetAvailableVectorStores(ctx, req)
}

// Model Pricing Operations - delegate to model pricing server

func (s *AIStudioManagementServer) ListModelPrices(ctx context.Context, req *pb.ListModelPricesRequest) (*pb.ListModelPricesResponse, error) {
	return s.modelPricingServer.ListModelPrices(ctx, req)
}

func (s *AIStudioManagementServer) GetModelPrice(ctx context.Context, req *pb.GetModelPriceRequest) (*pb.GetModelPriceResponse, error) {
	return s.modelPricingServer.GetModelPrice(ctx, req)
}

func (s *AIStudioManagementServer) CreateModelPrice(ctx context.Context, req *pb.CreateModelPriceRequest) (*pb.CreateModelPriceResponse, error) {
	return s.modelPricingServer.CreateModelPrice(ctx, req)
}

func (s *AIStudioManagementServer) UpdateModelPrice(ctx context.Context, req *pb.UpdateModelPriceRequest) (*pb.UpdateModelPriceResponse, error) {
	return s.modelPricingServer.UpdateModelPrice(ctx, req)
}

func (s *AIStudioManagementServer) DeleteModelPrice(ctx context.Context, req *pb.DeleteModelPriceRequest) (*pb.DeleteModelPriceResponse, error) {
	return s.modelPricingServer.DeleteModelPrice(ctx, req)
}

func (s *AIStudioManagementServer) GetModelPricesByVendor(ctx context.Context, req *pb.GetModelPricesByVendorRequest) (*pb.GetModelPricesByVendorResponse, error) {
	return s.modelPricingServer.GetModelPricesByVendor(ctx, req)
}

// Analytics Operations - not available to plugins

func (s *AIStudioManagementServer) GetVendorUsage(ctx context.Context, req *pb.GetVendorUsageRequest) (*pb.GetVendorUsageResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "analytics functionality is not available to plugins")
}

func (s *AIStudioManagementServer) GetTokenUsagePerApp(ctx context.Context, req *pb.GetTokenUsagePerAppRequest) (*pb.GetTokenUsagePerAppResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "analytics functionality is not available to plugins")
}

func (s *AIStudioManagementServer) GetToolUsageStatistics(ctx context.Context, req *pb.GetToolUsageStatisticsRequest) (*pb.GetToolUsageStatisticsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "analytics functionality is not available to plugins")
}

// Plugin KV Storage Operations - delegate to plugin KV server

func (s *AIStudioManagementServer) WritePluginKV(ctx context.Context, req *pb.WritePluginKVRequest) (*pb.WritePluginKVResponse, error) {
	// Validate plugin has kv.readwrite scope and inject plugin into context
	plugin, err := s.validatePluginScope(ctx, models.ServiceScopeKVReadWrite)
	if err != nil {
		return nil, err
	}

	// Update context with validated plugin for downstream use
	ctx = SetPluginInContext(ctx, plugin)

	return s.pluginKVServer.WritePluginKV(ctx, req)
}

func (s *AIStudioManagementServer) ReadPluginKV(ctx context.Context, req *pb.ReadPluginKVRequest) (*pb.ReadPluginKVResponse, error) {
	// Validate plugin has kv.readwrite scope and inject plugin into context
	plugin, err := s.validatePluginScope(ctx, models.ServiceScopeKVReadWrite)
	if err != nil {
		return nil, err
	}

	// Update context with validated plugin for downstream use
	ctx = SetPluginInContext(ctx, plugin)

	return s.pluginKVServer.ReadPluginKV(ctx, req)
}

func (s *AIStudioManagementServer) DeletePluginKV(ctx context.Context, req *pb.DeletePluginKVRequest) (*pb.DeletePluginKVResponse, error) {
	// Validate plugin has kv.readwrite scope and inject plugin into context
	plugin, err := s.validatePluginScope(ctx, models.ServiceScopeKVReadWrite)
	if err != nil {
		return nil, err
	}

	// Update context with validated plugin for downstream use
	ctx = SetPluginInContext(ctx, plugin)

	return s.pluginKVServer.DeletePluginKV(ctx, req)
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