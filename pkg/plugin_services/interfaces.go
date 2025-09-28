package plugin_services

import (
	"context"

	pb "github.com/TykTechnologies/midsommar/v2/proto/ai_studio_management"
)

// AIStudioServiceProvider defines the interface for AI Studio plugins to access management services
// This interface is dependency-free and can be implemented by any package
type AIStudioServiceProvider interface {
	// Plugin Management Operations
	ListPlugins(ctx context.Context, req *pb.ListPluginsRequest) (*pb.ListPluginsResponse, error)
	GetPlugin(ctx context.Context, req *pb.GetPluginRequest) (*pb.GetPluginResponse, error)
	UpdatePluginConfig(ctx context.Context, req *pb.UpdatePluginConfigRequest) (*pb.UpdatePluginConfigResponse, error)

	// LLM Management Operations
	ListLLMs(ctx context.Context, req *pb.ListLLMsRequest) (*pb.ListLLMsResponse, error)
	GetLLM(ctx context.Context, req *pb.GetLLMRequest) (*pb.GetLLMResponse, error)
	GetLLMPlugins(ctx context.Context, req *pb.GetLLMPluginsRequest) (*pb.GetLLMPluginsResponse, error)
	CreateLLM(ctx context.Context, req *pb.CreateLLMRequest) (*pb.CreateLLMResponse, error)
	UpdateLLM(ctx context.Context, req *pb.UpdateLLMRequest) (*pb.UpdateLLMResponse, error)
	DeleteLLM(ctx context.Context, req *pb.DeleteLLMRequest) (*pb.DeleteLLMResponse, error)

	// Analytics Operations
	GetAnalyticsSummary(ctx context.Context, req *pb.GetAnalyticsSummaryRequest) (*pb.GetAnalyticsSummaryResponse, error)
	GetUsageStatistics(ctx context.Context, req *pb.GetUsageStatisticsRequest) (*pb.GetUsageStatisticsResponse, error)
	GetCostAnalysis(ctx context.Context, req *pb.GetCostAnalysisRequest) (*pb.GetCostAnalysisResponse, error)
	GetChatRecordsPerDay(ctx context.Context, req *pb.GetChatRecordsPerDayRequest) (*pb.GetChatRecordsPerDayResponse, error)
	GetModelUsage(ctx context.Context, req *pb.GetModelUsageRequest) (*pb.GetModelUsageResponse, error)
	GetVendorUsage(ctx context.Context, req *pb.GetVendorUsageRequest) (*pb.GetVendorUsageResponse, error)
	GetTokenUsagePerApp(ctx context.Context, req *pb.GetTokenUsagePerAppRequest) (*pb.GetTokenUsagePerAppResponse, error)
	GetToolUsageStatistics(ctx context.Context, req *pb.GetToolUsageStatisticsRequest) (*pb.GetToolUsageStatisticsResponse, error)

	// App Management Operations
	ListApps(ctx context.Context, req *pb.ListAppsRequest) (*pb.ListAppsResponse, error)
	GetApp(ctx context.Context, req *pb.GetAppRequest) (*pb.GetAppResponse, error)
	CreateApp(ctx context.Context, req *pb.CreateAppRequest) (*pb.CreateAppResponse, error)
	UpdateApp(ctx context.Context, req *pb.UpdateAppRequest) (*pb.UpdateAppResponse, error)
	DeleteApp(ctx context.Context, req *pb.DeleteAppRequest) (*pb.DeleteAppResponse, error)

	// Tool Management Operations
	ListTools(ctx context.Context, req *pb.ListToolsRequest) (*pb.ListToolsResponse, error)
	GetTool(ctx context.Context, req *pb.GetToolRequest) (*pb.GetToolResponse, error)
	GetToolOperations(ctx context.Context, req *pb.GetToolOperationsRequest) (*pb.GetToolOperationsResponse, error)
	CallToolOperation(ctx context.Context, req *pb.CallToolOperationRequest) (*pb.CallToolOperationResponse, error)
	CreateTool(ctx context.Context, req *pb.CreateToolRequest) (*pb.CreateToolResponse, error)
	UpdateTool(ctx context.Context, req *pb.UpdateToolRequest) (*pb.UpdateToolResponse, error)
	DeleteTool(ctx context.Context, req *pb.DeleteToolRequest) (*pb.DeleteToolResponse, error)

	// Datasource Management Operations
	ListDatasources(ctx context.Context, req *pb.ListDatasourcesRequest) (*pb.ListDatasourcesResponse, error)
	GetDatasource(ctx context.Context, req *pb.GetDatasourceRequest) (*pb.GetDatasourceResponse, error)
	CreateDatasource(ctx context.Context, req *pb.CreateDatasourceRequest) (*pb.CreateDatasourceResponse, error)
	UpdateDatasource(ctx context.Context, req *pb.UpdateDatasourceRequest) (*pb.UpdateDatasourceResponse, error)
	DeleteDatasource(ctx context.Context, req *pb.DeleteDatasourceRequest) (*pb.DeleteDatasourceResponse, error)
	SearchDatasources(ctx context.Context, req *pb.SearchDatasourcesRequest) (*pb.SearchDatasourcesResponse, error)
	ProcessDatasourceEmbeddings(ctx context.Context, req *pb.ProcessEmbeddingsRequest) (*pb.ProcessEmbeddingsResponse, error)

	// Data Catalogues Management Operations
	ListDataCatalogues(ctx context.Context, req *pb.ListDataCataloguesRequest) (*pb.ListDataCataloguesResponse, error)
	GetDataCatalogue(ctx context.Context, req *pb.GetDataCatalogueRequest) (*pb.GetDataCatalogueResponse, error)
	CreateDataCatalogue(ctx context.Context, req *pb.CreateDataCatalogueRequest) (*pb.CreateDataCatalogueResponse, error)
	UpdateDataCatalogue(ctx context.Context, req *pb.UpdateDataCatalogueRequest) (*pb.UpdateDataCatalogueResponse, error)
	DeleteDataCatalogue(ctx context.Context, req *pb.DeleteDataCatalogueRequest) (*pb.DeleteDataCatalogueResponse, error)

	// Tags Management Operations
	ListTags(ctx context.Context, req *pb.ListTagsRequest) (*pb.ListTagsResponse, error)
	GetTag(ctx context.Context, req *pb.GetTagRequest) (*pb.GetTagResponse, error)
	CreateTag(ctx context.Context, req *pb.CreateTagRequest) (*pb.CreateTagResponse, error)
	UpdateTag(ctx context.Context, req *pb.UpdateTagRequest) (*pb.UpdateTagResponse, error)
	DeleteTag(ctx context.Context, req *pb.DeleteTagRequest) (*pb.DeleteTagResponse, error)
	SearchTags(ctx context.Context, req *pb.SearchTagsRequest) (*pb.SearchTagsResponse, error)

	// Model Price Management Operations (Critical Priority)
	ListModelPrices(ctx context.Context, req *pb.ListModelPricesRequest) (*pb.ListModelPricesResponse, error)
	GetModelPrice(ctx context.Context, req *pb.GetModelPriceRequest) (*pb.GetModelPriceResponse, error)
	CreateModelPrice(ctx context.Context, req *pb.CreateModelPriceRequest) (*pb.CreateModelPriceResponse, error)
	UpdateModelPrice(ctx context.Context, req *pb.UpdateModelPriceRequest) (*pb.UpdateModelPriceResponse, error)
	DeleteModelPrice(ctx context.Context, req *pb.DeleteModelPriceRequest) (*pb.DeleteModelPriceResponse, error)
	GetModelPricesByVendor(ctx context.Context, req *pb.GetModelPricesByVendorRequest) (*pb.GetModelPricesByVendorResponse, error)

	// Filter Management Operations
	ListFilters(ctx context.Context, req *pb.ListFiltersRequest) (*pb.ListFiltersResponse, error)
	GetFilter(ctx context.Context, req *pb.GetFilterRequest) (*pb.GetFilterResponse, error)
	CreateFilter(ctx context.Context, req *pb.CreateFilterRequest) (*pb.CreateFilterResponse, error)
	UpdateFilter(ctx context.Context, req *pb.UpdateFilterRequest) (*pb.UpdateFilterResponse, error)
	DeleteFilter(ctx context.Context, req *pb.DeleteFilterRequest) (*pb.DeleteFilterResponse, error)

	// Vendor Information Operations
	GetAvailableLLMDrivers(ctx context.Context, req *pb.GetAvailableLLMDriversRequest) (*pb.GetAvailableLLMDriversResponse, error)
	GetAvailableEmbedders(ctx context.Context, req *pb.GetAvailableEmbeddersRequest) (*pb.GetAvailableEmbeddersResponse, error)
	GetAvailableVectorStores(ctx context.Context, req *pb.GetAvailableVectorStoresRequest) (*pb.GetAvailableVectorStoresResponse, error)
}

// ServiceProviderInjectable interface for plugins that can receive service providers
type ServiceProviderInjectable interface {
	InjectServiceProvider(provider AIStudioServiceProvider)
}

// Context manipulation utilities (dependency-free)
type pluginContextKey struct{}

func AddPluginIDToContext(ctx context.Context, pluginID uint) context.Context {
	return context.WithValue(ctx, pluginContextKey{}, pluginID)
}

func GetPluginIDFromContext(ctx context.Context) (uint, bool) {
	if pluginID, ok := ctx.Value(pluginContextKey{}).(uint); ok {
		return pluginID, true
	}
	return 0, false
}