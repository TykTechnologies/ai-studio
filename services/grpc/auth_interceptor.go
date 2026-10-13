package grpc

import (
	"context"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// Plugin context key for storing authenticated plugin ID
// Using a string key to avoid type identity issues across packages
const pluginContextKeyString = "midsommar:plugin:id"

// Plugin info context key for storing full plugin model
// Using a string key to avoid type identity issues
const pluginInfoKeyString = "midsommar:plugin:info"

// PluginAuthInterceptor creates a gRPC interceptor for plugin authentication and authorization
func PluginAuthInterceptor(db *gorm.DB) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		ctx, err := authorizePluginCall(ctx, db, info.FullMethod)
		if err != nil {
			return nil, err
		}
		return handler(ctx, req)
	}
}

// managementServicePrefix is the method prefix of the management service.
const managementServicePrefix = "/ai_studio_management.AIStudioManagementService/"

// ManagementScopeInterceptors check every management-service call on a
// plugin's brokered server against the calling plugin's approved scopes (see
// authorizePluginCall). Calls to other services on the same server, such as
// the plugin event service, pass through.
func ManagementScopeInterceptors(db *gorm.DB) (grpc.UnaryServerInterceptor, grpc.StreamServerInterceptor) {
	unary := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		if !strings.HasPrefix(info.FullMethod, managementServicePrefix) {
			return handler(ctx, req)
		}
		if isSelfCall(ctx, info.FullMethod, req) {
			return handler(ctx, req)
		}
		ctx, err := authorizePluginCall(ctx, db, info.FullMethod)
		if err != nil {
			return nil, err
		}
		return handler(ctx, req)
	}
	stream := func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if !strings.HasPrefix(info.FullMethod, managementServicePrefix) {
			return handler(srv, ss)
		}
		ctx, err := authorizePluginCall(ss.Context(), db, info.FullMethod)
		if err != nil {
			return err
		}
		return handler(srv, &authorizedServerStream{ServerStream: ss, ctx: ctx})
	}
	return unary, stream
}

type authorizedServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *authorizedServerStream) Context() context.Context { return s.ctx }

// authorizePluginCall checks that the plugin whose ID the broker put in ctx
// may call fullMethod: it exists, its service access is approved, and it holds
// the method's scope. A method with no scope mapping is refused, unless it is
// one of scopeFreeMethods (which still need the broker's plugin ID). It
// returns ctx with the plugin added.
func authorizePluginCall(ctx context.Context, db *gorm.DB, fullMethod string) (context.Context, error) {
	pluginID := GetPluginIDFromContext(ctx)
	if pluginID == 0 {
		return nil, status.Errorf(codes.Unauthenticated, "plugin authentication required")
	}
	if scopeFreeMethods[fullMethod] {
		return ctx, nil
	}

	requiredScope := extractScopeFromMethod(fullMethod)
	if requiredScope == "" {
		log.Warn().Str("method", fullMethod).Msg("No scope mapping found for method")
		return nil, status.Errorf(codes.Internal, "no scope mapping for method")
	}

	// Block analytics functionality - not available to plugins
	if isAnalyticsScope(requiredScope) {
		log.Warn().
			Str("method", fullMethod).
			Str("scope", requiredScope).
			Msg("Analytics functionality blocked for plugins")
		return nil, status.Errorf(codes.Unimplemented, "analytics functionality is not available to plugins - analytics logic remains in REST API layer")
	}

	var plugin models.Plugin
	if err := db.First(&plugin, pluginID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			log.Error().Uint("plugin_id", pluginID).Msg("Plugin not found during auth")
			return nil, status.Errorf(codes.Unauthenticated, "plugin not found")
		}
		log.Error().Err(err).Uint("plugin_id", pluginID).Msg("Database error during plugin auth")
		return nil, status.Errorf(codes.Internal, "authentication error")
	}

	if !plugin.HasServiceAccess() {
		log.Warn().
			Uint("plugin_id", pluginID).
			Str("plugin_name", plugin.Name).
			Msg("Plugin service access not authorized")
		return nil, status.Errorf(codes.PermissionDenied, "service access not authorized for plugin %s", plugin.Name)
	}

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
		Str("method", fullMethod).
		Msg("Plugin authenticated and authorized")

	return SetPluginInContext(ctx, &plugin), nil
}

// selfMethods are management methods a plugin may call on its own plugin ID
// without a scope: reading and updating its own configuration. Shipped
// plugins rely on this (they keep state in their config and push it to
// edges). On another plugin's ID the method's scope applies, and the handler
// refuses config updates to other plugins altogether.
var selfMethods = map[string]bool{
	managementServicePrefix + "GetPlugin":          true,
	managementServicePrefix + "UpdatePluginConfig": true,
}

// scopeFreeMethods are management methods any plugin on a broker may call
// without a scope (the call still needs the broker's plugin ID).
var scopeFreeMethods = map[string]bool{
	// License status. Not sensitive, and enterprise plugins check it on their
	// first session, before an admin has approved their scopes; a plugin's
	// own license check is unaffected.
	managementServicePrefix + "GetLicenseInfo": true,
}

// isSelfCall reports whether req is a selfMethods call naming the calling
// plugin's own ID.
func isSelfCall(ctx context.Context, fullMethod string, req interface{}) bool {
	if !selfMethods[fullMethod] {
		return false
	}
	target, ok := req.(interface{ GetPluginId() uint32 })
	caller := GetPluginIDFromContext(ctx)
	return ok && caller != 0 && uint(target.GetPluginId()) == caller
}

// extractScopeFromMethod maps gRPC method names to required service scopes
func extractScopeFromMethod(fullMethod string) string {
	// Map gRPC method names to required scopes
	scopeMap := map[string]string{
		// Plugin management methods
		"/ai_studio_management.AIStudioManagementService/ListPlugins":        models.ServiceScopePluginsRead,
		"/ai_studio_management.AIStudioManagementService/GetPlugin":          models.ServiceScopePluginsRead,
		"/ai_studio_management.AIStudioManagementService/UpdatePluginConfig": models.ServiceScopePluginsConfig,

		// LLM management methods
		"/ai_studio_management.AIStudioManagementService/ListLLMs":      models.ServiceScopeLLMsRead,
		"/ai_studio_management.AIStudioManagementService/GetLLM":        models.ServiceScopeLLMsRead,
		"/ai_studio_management.AIStudioManagementService/GetLLMPlugins": models.ServiceScopeLLMsRead,
		"/ai_studio_management.AIStudioManagementService/CreateLLM":       models.ServiceScopeLLMsWrite,
		"/ai_studio_management.AIStudioManagementService/UpdateLLM":       models.ServiceScopeLLMsWrite,
		"/ai_studio_management.AIStudioManagementService/DeleteLLM":       models.ServiceScopeLLMsWrite,
		"/ai_studio_management.AIStudioManagementService/UpdateLLMPlugins": models.ServiceScopeLLMsWrite,

		// Analytics methods
		"/ai_studio_management.AIStudioManagementService/GetAnalyticsSummary": models.ServiceScopeAnalyticsRead,
		"/ai_studio_management.AIStudioManagementService/GetUsageStatistics": models.ServiceScopeAnalyticsRead,
		"/ai_studio_management.AIStudioManagementService/GetCostAnalysis":    models.ServiceScopeAnalyticsRead,

		// Detailed analytics methods
		"/ai_studio_management.AIStudioManagementService/GetChatRecordsPerDay":   models.ServiceScopeAnalyticsDetailed,
		"/ai_studio_management.AIStudioManagementService/GetModelUsage":         models.ServiceScopeAnalyticsDetailed,
		"/ai_studio_management.AIStudioManagementService/GetVendorUsage":        models.ServiceScopeAnalyticsDetailed,
		"/ai_studio_management.AIStudioManagementService/GetTokenUsagePerApp":   models.ServiceScopeAnalyticsReports,
		"/ai_studio_management.AIStudioManagementService/GetToolUsageStatistics": models.ServiceScopeAnalyticsReports,

		// App management methods
		"/ai_studio_management.AIStudioManagementService/ListApps": models.ServiceScopeAppsRead,
		"/ai_studio_management.AIStudioManagementService/GetApp":   models.ServiceScopeAppsRead,
		"/ai_studio_management.AIStudioManagementService/PatchAppMetadata": models.ServiceScopeAppsWrite,

		// Governed metadata methods (Enterprise)
		"/ai_studio_management.AIStudioManagementService/GetObjectMetadata":         models.ServiceScopeMetadataRead,
		"/ai_studio_management.AIStudioManagementService/GetResolvedMetadataSchema": models.ServiceScopeMetadataRead,
		"/ai_studio_management.AIStudioManagementService/ValidateObjectMetadata":    models.ServiceScopeMetadataRead,
		"/ai_studio_management.AIStudioManagementService/SetObjectMetadata":         models.ServiceScopeMetadataWrite,
		"/ai_studio_management.AIStudioManagementService/DeleteObjectMetadata":      models.ServiceScopeMetadataWrite,
		"/ai_studio_management.AIStudioManagementService/ListObjectMetadataAudit":   models.ServiceScopeMetadataRead,

		// Governance read methods
		"/ai_studio_management.AIStudioManagementService/ListAuditRecords":    models.ServiceScopeAuditRead,
		"/ai_studio_management.AIStudioManagementService/ListMCPServers":      models.ServiceScopeMCPServersRead,
		"/ai_studio_management.AIStudioManagementService/GetMCPServer":        models.ServiceScopeMCPServersRead,
		"/ai_studio_management.AIStudioManagementService/ListModelRouters":    models.ServiceScopeRoutersRead,
		"/ai_studio_management.AIStudioManagementService/GetModelRouter":      models.ServiceScopeRoutersRead,
		"/ai_studio_management.AIStudioManagementService/ListSemanticRouters": models.ServiceScopeRoutersRead,
		"/ai_studio_management.AIStudioManagementService/GetSemanticRouter":   models.ServiceScopeRoutersRead,

		// Plugin resource access methods
		"/ai_studio_management.AIStudioManagementService/ListGroups":                      models.ServiceScopeResourceAccessManage,
		"/ai_studio_management.AIStudioManagementService/GetResourceInstanceGroups":       models.ServiceScopeResourceAccessManage,
		"/ai_studio_management.AIStudioManagementService/SetResourceInstanceGroups":       models.ServiceScopeResourceAccessManage,
		"/ai_studio_management.AIStudioManagementService/ListAccessibleResourceInstances": models.ServiceScopeResourceAccessManage,
		"/ai_studio_management.AIStudioManagementService/SetAppGovernanceState":           models.ServiceScopeAppsLifecycle,
		"/ai_studio_management.AIStudioManagementService/CreateApp": models.ServiceScopeAppsWrite,
		"/ai_studio_management.AIStudioManagementService/UpdateApp": models.ServiceScopeAppsWrite,
		"/ai_studio_management.AIStudioManagementService/DeleteApp": models.ServiceScopeAppsWrite,

		// Tool management methods
		"/ai_studio_management.AIStudioManagementService/ListTools":         models.ServiceScopeToolsRead,
		"/ai_studio_management.AIStudioManagementService/GetTool":           models.ServiceScopeToolsRead,
		"/ai_studio_management.AIStudioManagementService/GetToolOperations": models.ServiceScopeToolsOperations,
		"/ai_studio_management.AIStudioManagementService/CallToolOperation": models.ServiceScopeToolsCall,
		"/ai_studio_management.AIStudioManagementService/CreateTool":        models.ServiceScopeToolsWrite,
		"/ai_studio_management.AIStudioManagementService/UpdateTool":        models.ServiceScopeToolsWrite,
		"/ai_studio_management.AIStudioManagementService/DeleteTool":        models.ServiceScopeToolsWrite,

		// Datasource management methods
		"/ai_studio_management.AIStudioManagementService/ListDatasources":        models.ServiceScopeDatasourcesRead,
		"/ai_studio_management.AIStudioManagementService/GetDatasource":         models.ServiceScopeDatasourcesRead,
		"/ai_studio_management.AIStudioManagementService/CreateDatasource":      models.ServiceScopeDatasourcesWrite,
		"/ai_studio_management.AIStudioManagementService/UpdateDatasource":      models.ServiceScopeDatasourcesWrite,
		"/ai_studio_management.AIStudioManagementService/DeleteDatasource":      models.ServiceScopeDatasourcesWrite,
		"/ai_studio_management.AIStudioManagementService/SearchDatasources":     models.ServiceScopeDatasourcesRead,
		"/ai_studio_management.AIStudioManagementService/ProcessDatasourceEmbeddings": models.ServiceScopeDatasourcesEmbeddings,

		// RAG/Embedding operations
		"/ai_studio_management.AIStudioManagementService/GenerateEmbedding":          models.ServiceScopeDatasourcesEmbeddings,
		"/ai_studio_management.AIStudioManagementService/StoreDocuments":             models.ServiceScopeDatasourcesEmbeddings,
		"/ai_studio_management.AIStudioManagementService/ProcessAndStoreDocuments":   models.ServiceScopeDatasourcesEmbeddings,
		"/ai_studio_management.AIStudioManagementService/QueryDatasourceByVector":    models.ServiceScopeDatasourcesQuery,

		// Advanced datasource operations - metadata and namespace management
		"/ai_studio_management.AIStudioManagementService/DeleteDocumentsByMetadata": models.ServiceScopeDatasourcesWrite,
		"/ai_studio_management.AIStudioManagementService/QueryByMetadataOnly":       models.ServiceScopeDatasourcesQuery,
		"/ai_studio_management.AIStudioManagementService/ListNamespaces":            models.ServiceScopeDatasourcesRead,
		"/ai_studio_management.AIStudioManagementService/DeleteNamespace":           models.ServiceScopeDatasourcesWrite,

		// Data catalogues management methods
		"/ai_studio_management.AIStudioManagementService/ListDataCatalogues":   models.ServiceScopeDataCataloguesRead,
		"/ai_studio_management.AIStudioManagementService/GetDataCatalogue":     models.ServiceScopeDataCataloguesRead,
		"/ai_studio_management.AIStudioManagementService/CreateDataCatalogue":  models.ServiceScopeDataCataloguesWrite,
		"/ai_studio_management.AIStudioManagementService/UpdateDataCatalogue":  models.ServiceScopeDataCataloguesWrite,
		"/ai_studio_management.AIStudioManagementService/DeleteDataCatalogue":  models.ServiceScopeDataCataloguesWrite,

		// Tags management methods
		"/ai_studio_management.AIStudioManagementService/ListTags":   models.ServiceScopeTagsRead,
		"/ai_studio_management.AIStudioManagementService/GetTag":     models.ServiceScopeTagsRead,
		"/ai_studio_management.AIStudioManagementService/CreateTag":  models.ServiceScopeTagsWrite,
		"/ai_studio_management.AIStudioManagementService/UpdateTag":  models.ServiceScopeTagsWrite,
		"/ai_studio_management.AIStudioManagementService/DeleteTag":  models.ServiceScopeTagsWrite,
		"/ai_studio_management.AIStudioManagementService/SearchTags": models.ServiceScopeTagsRead,

		// Model pricing methods
		"/ai_studio_management.AIStudioManagementService/ListModelPrices":        models.ServiceScopePricingRead,
		"/ai_studio_management.AIStudioManagementService/GetModelPrice":         models.ServiceScopePricingRead,
		"/ai_studio_management.AIStudioManagementService/CreateModelPrice":      models.ServiceScopePricingWrite,
		"/ai_studio_management.AIStudioManagementService/UpdateModelPrice":      models.ServiceScopePricingWrite,
		"/ai_studio_management.AIStudioManagementService/DeleteModelPrice":      models.ServiceScopePricingWrite,
		"/ai_studio_management.AIStudioManagementService/GetModelPricesByVendor": models.ServiceScopePricingRead,

		// Filter management methods
		"/ai_studio_management.AIStudioManagementService/ListFilters":   models.ServiceScopeFiltersRead,
		"/ai_studio_management.AIStudioManagementService/GetFilter":     models.ServiceScopeFiltersRead,
		"/ai_studio_management.AIStudioManagementService/CreateFilter":  models.ServiceScopeFiltersWrite,
		"/ai_studio_management.AIStudioManagementService/UpdateFilter":  models.ServiceScopeFiltersWrite,
		"/ai_studio_management.AIStudioManagementService/DeleteFilter":  models.ServiceScopeFiltersWrite,

		// Vendor information methods
		"/ai_studio_management.AIStudioManagementService/GetAvailableLLMDrivers":    models.ServiceScopeVendorsRead,
		"/ai_studio_management.AIStudioManagementService/GetAvailableEmbedders":    models.ServiceScopeVendorsRead,
		"/ai_studio_management.AIStudioManagementService/GetAvailableVectorStores": models.ServiceScopeVendorsRead,

		// Plugin KV storage methods
		"/ai_studio_management.AIStudioManagementService/WritePluginKV":  models.ServiceScopeKVReadWrite,
		"/ai_studio_management.AIStudioManagementService/ReadPluginKV":   models.ServiceScopeKVReadWrite,
		"/ai_studio_management.AIStudioManagementService/DeletePluginKV": models.ServiceScopeKVReadWrite,

		// Schedule management methods
		"/ai_studio_management.AIStudioManagementService/CreateSchedule": models.ServiceScopeSchedulerManage,
		"/ai_studio_management.AIStudioManagementService/GetSchedule":    models.ServiceScopeSchedulerManage,
		"/ai_studio_management.AIStudioManagementService/ListSchedules":  models.ServiceScopeSchedulerManage,
		"/ai_studio_management.AIStudioManagementService/UpdateSchedule": models.ServiceScopeSchedulerManage,
		"/ai_studio_management.AIStudioManagementService/DeleteSchedule": models.ServiceScopeSchedulerManage,

		// Methods whose handlers also check their scope (the interceptor
		// checks it first, so a handler that forgets is still covered).
		"/ai_studio_management.AIStudioManagementService/CloneDatasource":             models.ServiceScopeDatasourcesWrite,
		"/ai_studio_management.AIStudioManagementService/ExecuteTool":                 models.ServiceScopeToolsCall,
		"/ai_studio_management.AIStudioManagementService/QueryDatasource":             models.ServiceScopeDatasourcesQuery,
		"/ai_studio_management.AIStudioManagementService/CallLLM":                     models.ServiceScopeLLMsProxy,
		"/ai_studio_management.AIStudioManagementService/CallDecision":                models.ServiceScopeLLMsProxy,
		"/ai_studio_management.AIStudioManagementService/CreateNotification":          models.ServiceScopeNotificationsWrite,
		"/ai_studio_management.AIStudioManagementService/RegisterResourceTypes":       models.ServiceScopeResourceTypesManage,
		"/ai_studio_management.AIStudioManagementService/RegisterPermissionResources": models.ServiceScopeRBACRegister,
	}

	return scopeMap[fullMethod]
}

// isAnalyticsScope checks if a scope is analytics-related and should be blocked
func isAnalyticsScope(scope string) bool {
	analyticsScopes := []string{
		models.ServiceScopeAnalyticsRead,
		models.ServiceScopeAnalyticsDetailed,
		models.ServiceScopeAnalyticsReports,
	}

	for _, analyticsScope := range analyticsScopes {
		if scope == analyticsScope {
			return true
		}
	}
	return false
}

// GetPluginIDFromContext extracts the plugin ID from the context
// This should be set by the AI Studio plugin manager during plugin calls
func GetPluginIDFromContext(ctx context.Context) uint {
	if pluginID, ok := ctx.Value(pluginContextKeyString).(uint); ok {
		return pluginID
	}
	return 0
}

// SetPluginIDInContext stores the plugin ID in the context
// Used by the AI Studio plugin manager when making calls
func SetPluginIDInContext(ctx context.Context, pluginID uint) context.Context {
	return context.WithValue(ctx, pluginContextKeyString, pluginID)
}

// PluginContextKeyString is exported for use in other packages to avoid circular imports
const PluginContextKeyString = pluginContextKeyString

// SetPluginInContext stores the full plugin info in the context for downstream use
func SetPluginInContext(ctx context.Context, plugin *models.Plugin) context.Context {
	return context.WithValue(ctx, pluginInfoKeyString, plugin)
}

// GetPluginFromContext retrieves the full plugin info from the context
func GetPluginFromContext(ctx context.Context) (*models.Plugin, bool) {
	if plugin, ok := ctx.Value(pluginInfoKeyString).(*models.Plugin); ok {
		return plugin, true
	}
	return nil, false
}

// CreatePluginIDInterceptor creates an interceptor that injects plugin ID into all requests
// Used for brokered servers to ensure plugin authentication context is available
func CreatePluginIDInterceptor(pluginID uint) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		// Inject plugin ID into context
		ctx = SetPluginIDInContext(ctx, pluginID)
		return handler(ctx, req)
	}
}