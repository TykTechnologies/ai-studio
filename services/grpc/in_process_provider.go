package grpc

import (
	"context"

	pb "github.com/TykTechnologies/midsommar/v2/proto/ai_studio_management"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/rs/zerolog/log"
)

// InProcessServiceProvider implements AIStudioServiceProvider using direct server calls
// This reuses ALL existing gRPC server implementations while eliminating network overhead
type InProcessServiceProvider struct {
	server   *AIStudioManagementServer
	pluginID uint
}

// NewInProcessServiceProvider creates a service provider for in-process plugin service access
func NewInProcessServiceProvider(service *services.Service, pluginID uint) AIStudioServiceProvider {
	return &InProcessServiceProvider{
		server:   NewAIStudioManagementServer(service),
		pluginID: pluginID,
	}
}

// Plugin Management Operations - reuse existing server implementations

func (p *InProcessServiceProvider) ListPlugins(ctx context.Context, req *pb.ListPluginsRequest) (*pb.ListPluginsResponse, error) {
	ctx = SetPluginIDInContext(ctx, p.pluginID)
	return p.server.ListPlugins(ctx, req)
}

func (p *InProcessServiceProvider) GetPlugin(ctx context.Context, req *pb.GetPluginRequest) (*pb.GetPluginResponse, error) {
	ctx = SetPluginIDInContext(ctx, p.pluginID)
	return p.server.GetPlugin(ctx, req)
}

func (p *InProcessServiceProvider) UpdatePluginConfig(ctx context.Context, req *pb.UpdatePluginConfigRequest) (*pb.UpdatePluginConfigResponse, error) {
	ctx = SetPluginIDInContext(ctx, p.pluginID)
	return p.server.UpdatePluginConfig(ctx, req)
}

// LLM Management Operations - reuse existing server implementations

func (p *InProcessServiceProvider) ListLLMs(ctx context.Context, req *pb.ListLLMsRequest) (*pb.ListLLMsResponse, error) {
	ctx = SetPluginIDInContext(ctx, p.pluginID)
	return p.server.ListLLMs(ctx, req)
}

func (p *InProcessServiceProvider) GetLLM(ctx context.Context, req *pb.GetLLMRequest) (*pb.GetLLMResponse, error) {
	ctx = SetPluginIDInContext(ctx, p.pluginID)
	return p.server.GetLLM(ctx, req)
}

func (p *InProcessServiceProvider) GetLLMPlugins(ctx context.Context, req *pb.GetLLMPluginsRequest) (*pb.GetLLMPluginsResponse, error) {
	ctx = SetPluginIDInContext(ctx, p.pluginID)
	return p.server.GetLLMPlugins(ctx, req)
}

func (p *InProcessServiceProvider) CreateLLM(ctx context.Context, req *pb.CreateLLMRequest) (*pb.CreateLLMResponse, error) {
	ctx = SetPluginIDInContext(ctx, p.pluginID)
	return p.server.CreateLLM(ctx, req)
}

func (p *InProcessServiceProvider) UpdateLLM(ctx context.Context, req *pb.UpdateLLMRequest) (*pb.UpdateLLMResponse, error) {
	ctx = SetPluginIDInContext(ctx, p.pluginID)
	return p.server.UpdateLLM(ctx, req)
}

func (p *InProcessServiceProvider) DeleteLLM(ctx context.Context, req *pb.DeleteLLMRequest) (*pb.DeleteLLMResponse, error) {
	ctx = SetPluginIDInContext(ctx, p.pluginID)
	return p.server.DeleteLLM(ctx, req)
}

// Analytics Operations - reuse existing server implementations

func (p *InProcessServiceProvider) GetAnalyticsSummary(ctx context.Context, req *pb.GetAnalyticsSummaryRequest) (*pb.GetAnalyticsSummaryResponse, error) {
	ctx = SetPluginIDInContext(ctx, p.pluginID)
	return p.server.GetAnalyticsSummary(ctx, req)
}

func (p *InProcessServiceProvider) GetUsageStatistics(ctx context.Context, req *pb.GetUsageStatisticsRequest) (*pb.GetUsageStatisticsResponse, error) {
	ctx = SetPluginIDInContext(ctx, p.pluginID)
	return p.server.GetUsageStatistics(ctx, req)
}

func (p *InProcessServiceProvider) GetCostAnalysis(ctx context.Context, req *pb.GetCostAnalysisRequest) (*pb.GetCostAnalysisResponse, error) {
	ctx = SetPluginIDInContext(ctx, p.pluginID)
	return p.server.GetCostAnalysis(ctx, req)
}

func (p *InProcessServiceProvider) GetChatRecordsPerDay(ctx context.Context, req *pb.GetChatRecordsPerDayRequest) (*pb.GetChatRecordsPerDayResponse, error) {
	ctx = SetPluginIDInContext(ctx, p.pluginID)
	return p.server.GetChatRecordsPerDay(ctx, req)
}

func (p *InProcessServiceProvider) GetModelUsage(ctx context.Context, req *pb.GetModelUsageRequest) (*pb.GetModelUsageResponse, error) {
	ctx = SetPluginIDInContext(ctx, p.pluginID)
	return p.server.GetModelUsage(ctx, req)
}

func (p *InProcessServiceProvider) GetVendorUsage(ctx context.Context, req *pb.GetVendorUsageRequest) (*pb.GetVendorUsageResponse, error) {
	ctx = SetPluginIDInContext(ctx, p.pluginID)
	// Note: This method needs to be implemented in analytics server
	log.Warn().Msg("GetVendorUsage not yet implemented in analytics server")
	return &pb.GetVendorUsageResponse{}, nil
}

func (p *InProcessServiceProvider) GetTokenUsagePerApp(ctx context.Context, req *pb.GetTokenUsagePerAppRequest) (*pb.GetTokenUsagePerAppResponse, error) {
	ctx = SetPluginIDInContext(ctx, p.pluginID)
	// Note: This method needs to be implemented in analytics server
	log.Warn().Msg("GetTokenUsagePerApp not yet implemented in analytics server")
	return &pb.GetTokenUsagePerAppResponse{}, nil
}

func (p *InProcessServiceProvider) GetToolUsageStatistics(ctx context.Context, req *pb.GetToolUsageStatisticsRequest) (*pb.GetToolUsageStatisticsResponse, error) {
	ctx = SetPluginIDInContext(ctx, p.pluginID)
	// Note: This method needs to be implemented in analytics server
	log.Warn().Msg("GetToolUsageStatistics not yet implemented in analytics server")
	return &pb.GetToolUsageStatisticsResponse{}, nil
}

// App Management Operations - reuse existing server implementations

func (p *InProcessServiceProvider) ListApps(ctx context.Context, req *pb.ListAppsRequest) (*pb.ListAppsResponse, error) {
	ctx = SetPluginIDInContext(ctx, p.pluginID)
	return p.server.ListApps(ctx, req)
}

func (p *InProcessServiceProvider) GetApp(ctx context.Context, req *pb.GetAppRequest) (*pb.GetAppResponse, error) {
	ctx = SetPluginIDInContext(ctx, p.pluginID)
	return p.server.GetApp(ctx, req)
}

func (p *InProcessServiceProvider) CreateApp(ctx context.Context, req *pb.CreateAppRequest) (*pb.CreateAppResponse, error) {
	ctx = SetPluginIDInContext(ctx, p.pluginID)
	return p.server.CreateApp(ctx, req)
}

func (p *InProcessServiceProvider) UpdateApp(ctx context.Context, req *pb.UpdateAppRequest) (*pb.UpdateAppResponse, error) {
	ctx = SetPluginIDInContext(ctx, p.pluginID)
	return p.server.UpdateApp(ctx, req)
}

func (p *InProcessServiceProvider) DeleteApp(ctx context.Context, req *pb.DeleteAppRequest) (*pb.DeleteAppResponse, error) {
	ctx = SetPluginIDInContext(ctx, p.pluginID)
	return p.server.DeleteApp(ctx, req)
}

// Tool Management Operations - reuse existing server implementations

func (p *InProcessServiceProvider) ListTools(ctx context.Context, req *pb.ListToolsRequest) (*pb.ListToolsResponse, error) {
	ctx = SetPluginIDInContext(ctx, p.pluginID)
	return p.server.ListTools(ctx, req)
}

func (p *InProcessServiceProvider) GetTool(ctx context.Context, req *pb.GetToolRequest) (*pb.GetToolResponse, error) {
	ctx = SetPluginIDInContext(ctx, p.pluginID)
	return p.server.GetTool(ctx, req)
}

func (p *InProcessServiceProvider) GetToolOperations(ctx context.Context, req *pb.GetToolOperationsRequest) (*pb.GetToolOperationsResponse, error) {
	ctx = SetPluginIDInContext(ctx, p.pluginID)
	return p.server.GetToolOperations(ctx, req)
}

func (p *InProcessServiceProvider) CallToolOperation(ctx context.Context, req *pb.CallToolOperationRequest) (*pb.CallToolOperationResponse, error) {
	ctx = SetPluginIDInContext(ctx, p.pluginID)
	return p.server.CallToolOperation(ctx, req)
}

func (p *InProcessServiceProvider) CreateTool(ctx context.Context, req *pb.CreateToolRequest) (*pb.CreateToolResponse, error) {
	ctx = SetPluginIDInContext(ctx, p.pluginID)
	return p.server.CreateTool(ctx, req)
}

func (p *InProcessServiceProvider) UpdateTool(ctx context.Context, req *pb.UpdateToolRequest) (*pb.UpdateToolResponse, error) {
	ctx = SetPluginIDInContext(ctx, p.pluginID)
	return p.server.UpdateTool(ctx, req)
}

func (p *InProcessServiceProvider) DeleteTool(ctx context.Context, req *pb.DeleteToolRequest) (*pb.DeleteToolResponse, error) {
	ctx = SetPluginIDInContext(ctx, p.pluginID)
	return p.server.DeleteTool(ctx, req)
}

// Datasource Management Operations - reuse existing server implementations

func (p *InProcessServiceProvider) ListDatasources(ctx context.Context, req *pb.ListDatasourcesRequest) (*pb.ListDatasourcesResponse, error) {
	ctx = SetPluginIDInContext(ctx, p.pluginID)
	return p.server.ListDatasources(ctx, req)
}

func (p *InProcessServiceProvider) GetDatasource(ctx context.Context, req *pb.GetDatasourceRequest) (*pb.GetDatasourceResponse, error) {
	ctx = SetPluginIDInContext(ctx, p.pluginID)
	return p.server.GetDatasource(ctx, req)
}

func (p *InProcessServiceProvider) CreateDatasource(ctx context.Context, req *pb.CreateDatasourceRequest) (*pb.CreateDatasourceResponse, error) {
	ctx = SetPluginIDInContext(ctx, p.pluginID)
	return p.server.CreateDatasource(ctx, req)
}

func (p *InProcessServiceProvider) UpdateDatasource(ctx context.Context, req *pb.UpdateDatasourceRequest) (*pb.UpdateDatasourceResponse, error) {
	ctx = SetPluginIDInContext(ctx, p.pluginID)
	return p.server.UpdateDatasource(ctx, req)
}

func (p *InProcessServiceProvider) DeleteDatasource(ctx context.Context, req *pb.DeleteDatasourceRequest) (*pb.DeleteDatasourceResponse, error) {
	ctx = SetPluginIDInContext(ctx, p.pluginID)
	return p.server.DeleteDatasource(ctx, req)
}

func (p *InProcessServiceProvider) SearchDatasources(ctx context.Context, req *pb.SearchDatasourcesRequest) (*pb.SearchDatasourcesResponse, error) {
	ctx = SetPluginIDInContext(ctx, p.pluginID)
	return p.server.SearchDatasources(ctx, req)
}

func (p *InProcessServiceProvider) ProcessDatasourceEmbeddings(ctx context.Context, req *pb.ProcessEmbeddingsRequest) (*pb.ProcessEmbeddingsResponse, error) {
	ctx = SetPluginIDInContext(ctx, p.pluginID)
	// Note: This method needs to be implemented in datasources server
	log.Warn().Msg("ProcessDatasourceEmbeddings not yet implemented in datasources server")
	return &pb.ProcessEmbeddingsResponse{Success: false, Message: "Not implemented"}, nil
}

// Data Catalogues Management Operations - reuse existing server implementations

func (p *InProcessServiceProvider) ListDataCatalogues(ctx context.Context, req *pb.ListDataCataloguesRequest) (*pb.ListDataCataloguesResponse, error) {
	ctx = SetPluginIDInContext(ctx, p.pluginID)
	return p.server.ListDataCatalogues(ctx, req)
}

func (p *InProcessServiceProvider) GetDataCatalogue(ctx context.Context, req *pb.GetDataCatalogueRequest) (*pb.GetDataCatalogueResponse, error) {
	ctx = SetPluginIDInContext(ctx, p.pluginID)
	return p.server.GetDataCatalogue(ctx, req)
}

func (p *InProcessServiceProvider) CreateDataCatalogue(ctx context.Context, req *pb.CreateDataCatalogueRequest) (*pb.CreateDataCatalogueResponse, error) {
	ctx = SetPluginIDInContext(ctx, p.pluginID)
	return p.server.CreateDataCatalogue(ctx, req)
}

func (p *InProcessServiceProvider) UpdateDataCatalogue(ctx context.Context, req *pb.UpdateDataCatalogueRequest) (*pb.UpdateDataCatalogueResponse, error) {
	ctx = SetPluginIDInContext(ctx, p.pluginID)
	// Note: This method needs to be implemented in data catalogues server
	log.Warn().Msg("UpdateDataCatalogue not yet implemented in data catalogues server")
	return &pb.UpdateDataCatalogueResponse{}, nil
}

func (p *InProcessServiceProvider) DeleteDataCatalogue(ctx context.Context, req *pb.DeleteDataCatalogueRequest) (*pb.DeleteDataCatalogueResponse, error) {
	ctx = SetPluginIDInContext(ctx, p.pluginID)
	// Note: This method needs to be implemented in data catalogues server
	log.Warn().Msg("DeleteDataCatalogue not yet implemented in data catalogues server")
	return &pb.DeleteDataCatalogueResponse{Success: false, Message: "Not implemented"}, nil
}

// Tags Management Operations - reuse existing server implementations

func (p *InProcessServiceProvider) ListTags(ctx context.Context, req *pb.ListTagsRequest) (*pb.ListTagsResponse, error) {
	ctx = SetPluginIDInContext(ctx, p.pluginID)
	return p.server.ListTags(ctx, req)
}

func (p *InProcessServiceProvider) GetTag(ctx context.Context, req *pb.GetTagRequest) (*pb.GetTagResponse, error) {
	ctx = SetPluginIDInContext(ctx, p.pluginID)
	return p.server.GetTag(ctx, req)
}

func (p *InProcessServiceProvider) CreateTag(ctx context.Context, req *pb.CreateTagRequest) (*pb.CreateTagResponse, error) {
	ctx = SetPluginIDInContext(ctx, p.pluginID)
	return p.server.CreateTag(ctx, req)
}

func (p *InProcessServiceProvider) UpdateTag(ctx context.Context, req *pb.UpdateTagRequest) (*pb.UpdateTagResponse, error) {
	ctx = SetPluginIDInContext(ctx, p.pluginID)
	// Note: This method needs to be implemented in tags server
	log.Warn().Msg("UpdateTag not yet implemented in tags server")
	return &pb.UpdateTagResponse{}, nil
}

func (p *InProcessServiceProvider) DeleteTag(ctx context.Context, req *pb.DeleteTagRequest) (*pb.DeleteTagResponse, error) {
	ctx = SetPluginIDInContext(ctx, p.pluginID)
	// Note: This method needs to be implemented in tags server
	log.Warn().Msg("DeleteTag not yet implemented in tags server")
	return &pb.DeleteTagResponse{Success: false, Message: "Not implemented"}, nil
}

func (p *InProcessServiceProvider) SearchTags(ctx context.Context, req *pb.SearchTagsRequest) (*pb.SearchTagsResponse, error) {
	ctx = SetPluginIDInContext(ctx, p.pluginID)
	return p.server.SearchTags(ctx, req)
}

// GetPluginID returns the plugin ID for logging and debugging
func (p *InProcessServiceProvider) GetPluginID() uint {
	return p.pluginID
}