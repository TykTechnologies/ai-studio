package grpc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	dataSession "github.com/TykTechnologies/midsommar/v2/data_session"
	"github.com/TykTechnologies/midsommar/v2/models"
	pb "github.com/TykTechnologies/midsommar/v2/proto/ai_studio_management"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/gosimple/slug"
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
	schedulerServer     *SchedulerServer

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
		schedulerServer:     NewSchedulerServer(service),
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

func (s *AIStudioManagementServer) UpdateLLMPlugins(ctx context.Context, req *pb.UpdateLLMPluginsRequest) (*pb.UpdateLLMPluginsResponse, error) {
	return s.llmServer.UpdateLLMPlugins(ctx, req)
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

	// Parse metadata JSON if provided
	var metadata map[string]interface{}
	if req.GetMetadata() != "" {
		if err := json.Unmarshal([]byte(req.GetMetadata()), &metadata); err != nil {
			log.Warn().Err(err).Msg("Failed to parse metadata JSON in CreateApp request")
		}
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
		metadata,
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
	datasourceIDs := make([]uint, len(req.GetDatasourceIds()))
	for i, id := range req.GetDatasourceIds() {
		datasourceIDs[i] = uint(id)
	}

	// Parse metadata JSON if provided
	var metadata map[string]interface{}
	if req.GetMetadata() != "" {
		if err := json.Unmarshal([]byte(req.GetMetadata()), &metadata); err != nil {
			log.Warn().Err(err).Msg("Failed to parse metadata JSON in UpdateApp request")
		}
	}

	// Get existing app to preserve user_id
	existingApp, err := s.service.GetAppByID(uint(appID))
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "app not found: %d", appID)
	}

	// Call existing service method with full parameters
	app, err := s.service.UpdateApp(
		uint(appID),
		req.GetName(),
		req.GetDescription(),
		existingApp.UserID, // Preserve existing user_id
		datasourceIDs,
		llmIDs,
		toolIDs,
		req.MonthlyBudget,
		nil, // budgetStartDate
		metadata,
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

func (s *AIStudioManagementServer) CloneDatasource(ctx context.Context, req *pb.CloneDatasourceRequest) (*pb.CloneDatasourceResponse, error) {
	return s.datasourcesServer.CloneDatasource(ctx, req)
}

func (s *AIStudioManagementServer) ProcessDatasourceEmbeddings(ctx context.Context, req *pb.ProcessEmbeddingsRequest) (*pb.ProcessEmbeddingsResponse, error) {
	return s.datasourcesServer.ProcessDatasourceEmbeddings(ctx, req)
}

// === RAG/Embedding Operations ===

func (s *AIStudioManagementServer) GenerateEmbedding(ctx context.Context, req *pb.GenerateEmbeddingRequest) (*pb.GenerateEmbeddingResponse, error) {
	return s.datasourcesServer.GenerateEmbedding(ctx, req)
}

func (s *AIStudioManagementServer) StoreDocuments(ctx context.Context, req *pb.StoreDocumentsRequest) (*pb.StoreDocumentsResponse, error) {
	return s.datasourcesServer.StoreDocuments(ctx, req)
}

func (s *AIStudioManagementServer) ProcessAndStoreDocuments(ctx context.Context, req *pb.ProcessAndStoreRequest) (*pb.ProcessAndStoreResponse, error) {
	return s.datasourcesServer.ProcessAndStoreDocuments(ctx, req)
}

func (s *AIStudioManagementServer) QueryDatasourceByVector(ctx context.Context, req *pb.QueryByVectorRequest) (*pb.QueryDatasourceResponse, error) {
	return s.datasourcesServer.QueryDatasourceByVector(ctx, req)
}

// Advanced Datasource Operations - Metadata and Namespace Management

func (s *AIStudioManagementServer) DeleteDocumentsByMetadata(ctx context.Context, req *pb.DeleteDocumentsByMetadataRequest) (*pb.DeleteDocumentsByMetadataResponse, error) {
	return s.datasourcesServer.DeleteDocumentsByMetadata(ctx, req)
}

func (s *AIStudioManagementServer) QueryByMetadataOnly(ctx context.Context, req *pb.QueryByMetadataOnlyRequest) (*pb.QueryByMetadataOnlyResponse, error) {
	return s.datasourcesServer.QueryByMetadataOnly(ctx, req)
}

func (s *AIStudioManagementServer) ListNamespaces(ctx context.Context, req *pb.ListNamespacesRequest) (*pb.ListNamespacesResponse, error) {
	return s.datasourcesServer.ListNamespaces(ctx, req)
}

func (s *AIStudioManagementServer) DeleteNamespace(ctx context.Context, req *pb.DeleteNamespaceRequest) (*pb.DeleteNamespaceResponse, error) {
	return s.datasourcesServer.DeleteNamespace(ctx, req)
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

	// Extract Tool IDs from relationships
	toolIDs := make([]uint32, len(app.Tools))
	for i, tool := range app.Tools {
		toolIDs[i] = uint32(tool.ID)
	}

	// Extract Datasource IDs from relationships
	datasourceIDs := make([]uint32, len(app.Datasources))
	for i, ds := range app.Datasources {
		datasourceIDs[i] = uint32(ds.ID)
	}

	// Serialize metadata to JSON string
	var metadataJSON string
	if app.Metadata != nil && len(app.Metadata) > 0 {
		if metadataBytes, err := json.Marshal(app.Metadata); err == nil {
			metadataJSON = string(metadataBytes)
		}
	}

	// Owner email - left empty for now as User relationship may not be preloaded
	// Can be populated by caller if needed
	ownerEmail := ""

	return &pb.AppInfo{
		Id:            uint32(app.ID),
		Name:          app.Name,
		Description:   app.Description,
		IsActive:      app.IsActive,
		Namespace:     app.Namespace,
		MonthlyBudget: monthlyBudget,
		LlmIds:        llmIDs,
		ToolIds:       toolIDs,
		DatasourceIds: datasourceIDs,
		UserId:        uint32(app.UserID),
		CreatedAt:     timestamppb.New(app.CreatedAt),
		UpdatedAt:     timestamppb.New(app.UpdatedAt),
		OwnerEmail:    ownerEmail,
		Metadata:      metadataJSON,
	}
}

// Agent Plugin Operations - Wrap REST APIs with credential injection

// ExecuteTool allows agent plugins to execute tools from their associated app
func (s *AIStudioManagementServer) ExecuteTool(ctx context.Context, req *pb.ExecuteToolRequest) (*pb.ExecuteToolResponse, error) {
	// Validate plugin has tools.call scope
	plugin, err := s.validatePluginScope(ctx, models.ServiceScopeToolsCall)
	if err != nil {
		return nil, err
	}

	// Load agent config for this plugin with App.Tools preloaded
	var agentConfig models.AgentConfig
	if err := s.service.GetDB().
		Preload("App.Tools").
		Preload("App.Credential").
		Where("plugin_id = ?", plugin.ID).
		First(&agentConfig).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, status.Errorf(codes.NotFound, "no agent configuration found for plugin")
		}
		log.Error().Err(err).Uint("plugin_id", plugin.ID).Msg("Failed to load agent config")
		return nil, status.Errorf(codes.Internal, "failed to load agent configuration")
	}

	// Verify tool is in app's allowed tools
	toolID := req.GetToolId()
	toolAllowed := false
	for _, tool := range agentConfig.App.Tools {
		if tool.ID == uint(toolID) {
			toolAllowed = true
			break
		}
	}
	if !toolAllowed {
		log.Warn().
			Uint32("tool_id", toolID).
			Uint("app_id", agentConfig.App.ID).
			Msg("Tool not allowed for agent's app")
		return nil, status.Errorf(codes.PermissionDenied, "tool not allowed for this agent")
	}

	// Call existing tool execution service with app credential context
	// The service.CallToolOperation method handles authentication internally
	result, err := s.service.CallToolOperation(
		uint(toolID),
		req.GetOperationId(),
		parseJSONToMap(req.GetParamsJson()),
		parseJSONToInterface(req.GetPayloadJson()),
		parseJSONToMap(req.GetHeadersJson()),
	)
	if err != nil {
		log.Error().Err(err).
			Uint32("tool_id", toolID).
			Str("operation_id", req.GetOperationId()).
			Msg("Tool execution failed")
		return &pb.ExecuteToolResponse{
			Success:      false,
			ErrorMessage: err.Error(),
		}, nil
	}

	// Convert result to JSON
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return &pb.ExecuteToolResponse{
			Success:      false,
			ErrorMessage: "failed to serialize tool result",
		}, nil
	}

	log.Info().
		Uint32("tool_id", toolID).
		Str("operation_id", req.GetOperationId()).
		Uint("agent_id", agentConfig.ID).
		Msg("Tool executed successfully for agent")

	return &pb.ExecuteToolResponse{
		Success:    true,
		ResultJson: string(resultJSON),
		StatusCode: 200,
	}, nil
}

// QueryDatasource allows agent plugins to query datasources from their associated app
func (s *AIStudioManagementServer) QueryDatasource(ctx context.Context, req *pb.QueryDatasourceRequest) (*pb.QueryDatasourceResponse, error) {
	// Validate plugin has datasources.query scope
	plugin, err := s.validatePluginScope(ctx, models.ServiceScopeDatasourcesQuery)
	if err != nil {
		return nil, err
	}

	// Try to load agent config for this plugin (for agent plugins)
	// If not found, allow querying any datasource (for non-agent plugins like service-api-test)
	datasourceID := req.GetDatasourceId()
	var agentConfig models.AgentConfig
	if err := s.service.GetDB().
		Preload("App.Datasources").
		Preload("App.Credential").
		Where("plugin_id = ?", plugin.ID).
		First(&agentConfig).Error; err == nil {
		// This is an agent plugin - verify datasource is in app's allowed datasources
		datasourceAllowed := false
		for _, ds := range agentConfig.App.Datasources {
			if ds.ID == uint(datasourceID) {
				datasourceAllowed = true
				break
			}
		}
		if !datasourceAllowed {
			log.Warn().
				Uint32("datasource_id", datasourceID).
				Uint("app_id", agentConfig.App.ID).
				Msg("Datasource not allowed for agent's app")
			return nil, status.Errorf(codes.PermissionDenied, "datasource not allowed for this agent")
		}
		log.Debug().Uint("agent_id", agentConfig.ID).Msg("Agent plugin querying datasource")
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		// Unexpected error loading agent config
		log.Error().Err(err).Uint("plugin_id", plugin.ID).Msg("Failed to load agent config")
		return nil, status.Errorf(codes.Internal, "failed to load agent configuration")
	}
	// If RecordNotFound, this is a non-agent plugin (like service-api-test) - allow access

	// Load the datasource for RAG query
	var datasource models.Datasource
	if err := s.service.GetDB().First(&datasource, datasourceID).Error; err != nil {
		log.Error().Err(err).Uint32("datasource_id", datasourceID).Msg("Failed to load datasource")
		return &pb.QueryDatasourceResponse{
			Success:      false,
			ErrorMessage: "datasource not found",
		}, nil
	}

	// Create data session for RAG query
	// This uses the same approach as the proxy's handleDatasourceRequest
	dataSession := dataSession.NewDataSession(map[uint]*models.Datasource{
		datasource.ID: &datasource,
	})

	// Perform similarity search
	maxResults := int(req.GetMaxResults())
	if maxResults == 0 {
		maxResults = 5 // Default to 5 results
	}

	docs, err := dataSession.Search(req.GetQuery(), maxResults)
	if err != nil {
		log.Error().Err(err).
			Uint32("datasource_id", datasourceID).
			Str("query", req.GetQuery()).
			Msg("RAG query failed")
		return &pb.QueryDatasourceResponse{
			Success:      false,
			ErrorMessage: fmt.Sprintf("query failed: %v", err),
		}, nil
	}

	// Convert results to proto format
	results := make([]*pb.DatasourceResult, 0, len(docs))
	for _, doc := range docs {
		// Extract similarity score from metadata if available
		similarityScore := 0.0
		if score, ok := doc.Metadata["score"]; ok {
			if scoreFloat, ok := score.(float64); ok {
				similarityScore = scoreFloat
			}
		}

		// Filter by similarity threshold if specified
		if req.GetSimilarityThreshold() > 0 && similarityScore < req.GetSimilarityThreshold() {
			continue
		}

		// Convert metadata to map[string]string
		metadata := make(map[string]string)
		for k, v := range doc.Metadata {
			metadata[k] = fmt.Sprintf("%v", v)
		}

		results = append(results, &pb.DatasourceResult{
			Content:         doc.PageContent,
			SimilarityScore: similarityScore,
			Metadata:        metadata,
		})
	}

	log.Info().
		Uint32("datasource_id", datasourceID).
		Str("query", req.GetQuery()).
		Int("result_count", len(results)).
		Uint("agent_id", agentConfig.ID).
		Msg("Datasource query executed successfully for agent")

	return &pb.QueryDatasourceResponse{
		Success: true,
		Results: results,
	}, nil
}

// CallLLM allows agent plugins to make LLM proxy calls from their associated app
func (s *AIStudioManagementServer) CallLLM(req *pb.CallLLMRequest, stream pb.AIStudioManagementService_CallLLMServer) error {
	ctx := stream.Context()

	// Extract plugin ID from request context and add to gRPC context for validation
	// This is needed because streaming RPCs don't go through the unary interceptor
	if req.GetContext() != nil && req.GetContext().GetPluginId() != 0 {
		ctx = SetPluginIDInContext(ctx, uint(req.GetContext().GetPluginId()))
	}

	// Validate plugin has llms.proxy scope
	plugin, err := s.validatePluginScope(ctx, models.ServiceScopeLLMsProxy)
	if err != nil {
		return err
	}

	// Load agent config for this plugin with App.LLMs preloaded
	var agentConfig models.AgentConfig
	if err := s.service.GetDB().
		Preload("App.LLMs").
		Preload("App.Credential").
		Where("plugin_id = ?", plugin.ID).
		First(&agentConfig).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return status.Errorf(codes.NotFound, "no agent configuration found for plugin")
		}
		log.Error().Err(err).Uint("plugin_id", plugin.ID).Msg("Failed to load agent config")
		return status.Errorf(codes.Internal, "failed to load agent configuration")
	}

	// Verify LLM is in app's allowed LLMs
	llmID := req.GetLlmId()
	llmAllowed := false
	for _, llm := range agentConfig.App.LLMs {
		if llm.ID == uint(llmID) {
			llmAllowed = true
			break
		}
	}
	if !llmAllowed {
		log.Warn().
			Uint32("llm_id", llmID).
			Uint("app_id", agentConfig.App.ID).
			Msg("LLM not allowed for agent's app")
		return status.Errorf(codes.PermissionDenied, "LLM not allowed for this agent")
	}

	// Load the full LLM record
	var llm models.LLM
	if err := s.service.GetDB().First(&llm, llmID).Error; err != nil {
		log.Error().Err(err).Uint32("llm_id", llmID).Msg("Failed to load LLM")
		return status.Errorf(codes.Internal, "failed to load LLM")
	}

	// Generate slug from LLM name
	llmSlug := slug.Make(llm.Name)

	// Build OpenAI-compatible request body from proto request
	requestBody := map[string]interface{}{
		"model":    req.GetModel(),
		"messages": convertProtoMessagesToOpenAI(req.GetMessages()),
		"stream":   false, // Use non-streaming for simplicity
	}

	// Add optional parameters if provided
	if req.GetTemperature() > 0 {
		requestBody["temperature"] = req.GetTemperature()
	}
	if req.GetMaxTokens() > 0 {
		requestBody["max_tokens"] = req.GetMaxTokens()
	}
	if len(req.GetTools()) > 0 {
		requestBody["tools"] = convertProtoToolsToOpenAI(req.GetTools())
	}

	requestJSON, err := json.Marshal(requestBody)
	if err != nil {
		log.Error().Err(err).Msg("Failed to marshal LLM request")
		return status.Errorf(codes.Internal, "failed to marshal request")
	}

	// Make internal HTTP call to OpenAI shim endpoint (/ai/)
	// This ensures all proxy middleware (analytics, budget, filters, plugins) are applied
	proxyURL := fmt.Sprintf("http://localhost:9090/ai/%s/v1/chat/completions", llmSlug)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", proxyURL, bytes.NewReader(requestJSON))
	if err != nil {
		log.Error().Err(err).Msg("Failed to create HTTP request for LLM proxy")
		return status.Errorf(codes.Internal, "failed to create proxy request")
	}

	// Set headers including app credential for authentication
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+agentConfig.App.Credential.Secret)
	httpReq.Header.Set("Accept", "application/json")

	// Make the request with timeout
	client := &http.Client{
		Timeout: 2 * time.Minute, // Reasonable timeout for non-streaming
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		log.Error().Err(err).Str("proxy_url", proxyURL).Msg("Failed to call LLM proxy")
		if sendErr := stream.Send(&pb.CallLLMResponse{
			Success:      false,
			ErrorMessage: fmt.Sprintf("proxy call failed: %v", err),
			Done:         true,
		}); sendErr != nil {
			return status.Errorf(codes.Internal, "failed to send error response: %v", sendErr)
		}
		return nil
	}
	defer resp.Body.Close()

	// Read the complete response body
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error().Err(err).Msg("Failed to read response body")
		if sendErr := stream.Send(&pb.CallLLMResponse{
			Success:      false,
			ErrorMessage: fmt.Sprintf("failed to read response: %v", err),
			Done:         true,
		}); sendErr != nil {
			return status.Errorf(codes.Internal, "failed to send error response: %v", sendErr)
		}
		return nil
	}

	// Check for non-200 response
	if resp.StatusCode != http.StatusOK {
		log.Error().
			Int("status_code", resp.StatusCode).
			Str("response_body", string(bodyBytes)).
			Msg("LLM proxy returned error")
		if sendErr := stream.Send(&pb.CallLLMResponse{
			Success:      false,
			ErrorMessage: fmt.Sprintf("proxy error (status %d): %s", resp.StatusCode, string(bodyBytes)),
			Done:         true,
		}); sendErr != nil {
			return status.Errorf(codes.Internal, "failed to send error response: %v", sendErr)
		}
		return nil
	}

	// Parse OpenAI completion response
	var openAIResp map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &openAIResp); err != nil {
		log.Error().Err(err).Str("body", string(bodyBytes)).Msg("Failed to parse OpenAI response")
		if sendErr := stream.Send(&pb.CallLLMResponse{
			Success:      false,
			ErrorMessage: fmt.Sprintf("failed to parse response: %v", err),
			Done:         true,
		}); sendErr != nil {
			return status.Errorf(codes.Internal, "failed to send error response: %v", sendErr)
		}
		return nil
	}

	// Extract content from OpenAI response format
	// OpenAI format: {"choices": [{"message": {"content": "..."}}]}
	content := ""
	var usage *pb.LLMUsage
	var toolCalls []*pb.LLMToolCall

	if choices, ok := openAIResp["choices"].([]interface{}); ok && len(choices) > 0 {
		if choice, ok := choices[0].(map[string]interface{}); ok {
			if message, ok := choice["message"].(map[string]interface{}); ok {
				if c, ok := message["content"].(string); ok {
					content = c
				}
				// Handle tool calls if present
				if tc, ok := message["tool_calls"].([]interface{}); ok {
					toolCalls = parseToolCalls(tc)
				}
			}
		}
	}

	// Extract usage stats if present
	if usageMap, ok := openAIResp["usage"].(map[string]interface{}); ok {
		usage = &pb.LLMUsage{
			PromptTokens:     int32(getIntFromMap(usageMap, "prompt_tokens")),
			CompletionTokens: int32(getIntFromMap(usageMap, "completion_tokens")),
			TotalTokens:      int32(getIntFromMap(usageMap, "total_tokens")),
		}
	}

	// Send single response through gRPC stream
	if err := stream.Send(&pb.CallLLMResponse{
		Success:   true,
		Content:   content,
		Usage:     usage,
		ToolCalls: toolCalls,
		Done:      true,
	}); err != nil {
		return status.Errorf(codes.Internal, "failed to send response: %v", err)
	}

	log.Info().
		Uint32("llm_id", llmID).
		Str("llm_slug", llmSlug).
		Uint("app_id", agentConfig.App.ID).
		Uint("agent_id", agentConfig.ID).
		Msg("LLM proxy call completed successfully for agent")

	return nil
}

// Helper functions for JSON parsing

func parseJSONToMap(jsonStr string) map[string][]string {
	if jsonStr == "" {
		return nil
	}
	var result map[string][]string
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		log.Error().Err(err).Str("json", jsonStr).Msg("Failed to parse JSON to map")
		return nil
	}
	return result
}

func parseJSONToInterface(jsonStr string) map[string]interface{} {
	if jsonStr == "" {
		return nil
	}
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		log.Error().Err(err).Str("json", jsonStr).Msg("Failed to parse JSON to interface")
		return nil
	}
	return result
}

// Helper functions for LLM proxy integration

// convertProtoMessagesToOpenAI converts proto LLMMessage to OpenAI chat message format
func convertProtoMessagesToOpenAI(messages []*pb.LLMMessage) []map[string]interface{} {
	result := make([]map[string]interface{}, 0, len(messages))
	for _, msg := range messages {
		openAIMsg := map[string]interface{}{
			"role":    msg.GetRole(),
			"content": msg.GetContent(),
		}
		if msg.GetName() != "" {
			openAIMsg["name"] = msg.GetName()
		}
		if msg.GetToolCallId() != "" {
			openAIMsg["tool_call_id"] = msg.GetToolCallId()
		}
		result = append(result, openAIMsg)
	}
	return result
}

// convertProtoToolsToOpenAI converts proto LLMTool to OpenAI tools format
func convertProtoToolsToOpenAI(tools []*pb.LLMTool) []map[string]interface{} {
	result := make([]map[string]interface{}, 0, len(tools))
	for _, tool := range tools {
		openAITool := map[string]interface{}{
			"type": tool.GetType(),
		}
		if fn := tool.GetFunction(); fn != nil {
			fnMap := map[string]interface{}{
				"name":        fn.GetName(),
				"description": fn.GetDescription(),
			}
			// Parse parameters JSON
			if fn.GetParametersJson() != "" {
				var params map[string]interface{}
				if err := json.Unmarshal([]byte(fn.GetParametersJson()), &params); err == nil {
					fnMap["parameters"] = params
				}
			}
			openAITool["function"] = fnMap
		}
		result = append(result, openAITool)
	}
	return result
}

// parseToolCalls parses tool_calls from OpenAI response format
func parseToolCalls(toolCallsArray []interface{}) []*pb.LLMToolCall {
	result := make([]*pb.LLMToolCall, 0, len(toolCallsArray))
	for _, tc := range toolCallsArray {
		if tcMap, ok := tc.(map[string]interface{}); ok {
			toolCall := &pb.LLMToolCall{
				Id:   getStringFromMap(tcMap, "id"),
				Type: getStringFromMap(tcMap, "type"),
			}
			if fn, ok := tcMap["function"].(map[string]interface{}); ok {
				toolCall.Function = &pb.LLMFunctionCall{
					Name:      getStringFromMap(fn, "name"),
					Arguments: getStringFromMap(fn, "arguments"),
				}
			}
			result = append(result, toolCall)
		}
	}
	return result
}

// getIntFromMap safely extracts an int value from a map
func getIntFromMap(m map[string]interface{}, key string) int {
	if val, ok := m[key]; ok {
		switch v := val.(type) {
		case int:
			return v
		case int32:
			return int(v)
		case int64:
			return int(v)
		case float64:
			return int(v)
		}
	}
	return 0
}

// getStringFromMap safely extracts a string value from a map
func getStringFromMap(m map[string]interface{}, key string) string {
	if val, ok := m[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

// Schedule Management Operations - delegate to scheduler server

func (s *AIStudioManagementServer) CreateSchedule(ctx context.Context, req *pb.CreateScheduleRequest) (*pb.CreateScheduleResponse, error) {
	return s.schedulerServer.CreateSchedule(ctx, req)
}

// GetLicenseInfo returns license information for plugins to check enterprise features
// This is a special RPC that doesn't require scope validation - all plugins can check license status
func (s *AIStudioManagementServer) GetLicenseInfo(ctx context.Context, req *pb.GetLicenseInfoRequest) (*pb.GetLicenseInfoResponse, error) {
	// Get licensing service from main service
	licensingSvc := s.service.LicensingService
	if licensingSvc == nil {
		// No licensing service means community edition
		log.Debug().Msg("GetLicenseInfo called but no licensing service configured (community mode)")
		return &pb.GetLicenseInfoResponse{
			LicenseValid:   true, // Community is always "valid"
			DaysRemaining:  -1,   // -1 means never expires
			LicenseType:    "community",
			Entitlements:   []string{},
			Organization:   "",
			ExpiresAt:      nil,
		}, nil
	}

	// Get license info from the licensing service
	licenseInfo := licensingSvc.GetLicenseInfo()
	isValid := licensingSvc.IsValid()
	daysLeft := licensingSvc.DaysLeft()

	// Build response
	resp := &pb.GetLicenseInfoResponse{
		LicenseValid:  isValid,
		DaysRemaining: int32(daysLeft),
		LicenseType:   "community", // Default to community
	}

	if licenseInfo != nil {
		// Enterprise license present
		resp.LicenseType = "enterprise"
		resp.ExpiresAt = timestamppb.New(licenseInfo.ExpiresAt)

		// Extract entitlement names
		var entitlements []string
		for name := range licenseInfo.Features {
			entitlements = append(entitlements, name)
		}
		resp.Entitlements = entitlements
	}

	log.Debug().
		Bool("license_valid", resp.LicenseValid).
		Int32("days_remaining", resp.DaysRemaining).
		Str("license_type", resp.LicenseType).
		Int("entitlement_count", len(resp.Entitlements)).
		Msg("GetLicenseInfo called by plugin")

	return resp, nil
}

func (s *AIStudioManagementServer) GetSchedule(ctx context.Context, req *pb.GetScheduleRequest) (*pb.GetScheduleResponse, error) {
	return s.schedulerServer.GetSchedule(ctx, req)
}

func (s *AIStudioManagementServer) ListSchedules(ctx context.Context, req *pb.ListSchedulesRequest) (*pb.ListSchedulesResponse, error) {
	return s.schedulerServer.ListSchedules(ctx, req)
}

func (s *AIStudioManagementServer) UpdateSchedule(ctx context.Context, req *pb.UpdateScheduleRequest) (*pb.UpdateScheduleResponse, error) {
	return s.schedulerServer.UpdateSchedule(ctx, req)
}

func (s *AIStudioManagementServer) DeleteSchedule(ctx context.Context, req *pb.DeleteScheduleRequest) (*pb.DeleteScheduleResponse, error) {
	return s.schedulerServer.DeleteSchedule(ctx, req)
}