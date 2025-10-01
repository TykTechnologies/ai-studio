package grpc

import (
	"context"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

// Plugin context key for storing authenticated plugin ID
type pluginContextKey struct{}

// PluginAuthInterceptor creates a gRPC interceptor for plugin authentication and authorization
func PluginAuthInterceptor(db *gorm.DB) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		// Extract plugin ID from request context (set by AI Studio plugin manager)
		pluginID := GetPluginIDFromContext(ctx)
		if pluginID == 0 {
			log.Debug().Str("method", info.FullMethod).Msg("No plugin ID in context, checking metadata")

			// Fallback: try to get from gRPC metadata
			if md, ok := metadata.FromIncomingContext(ctx); ok {
				if pluginIDs := md.Get("plugin-id"); len(pluginIDs) > 0 {
					log.Debug().Str("plugin_id", pluginIDs[0]).Msg("Found plugin ID in metadata")
					// Note: Would need to parse string to uint here if using metadata approach
				}
			}

			return nil, status.Errorf(codes.Unauthenticated, "plugin authentication required")
		}

		// Extract required scope from method info
		requiredScope := extractScopeFromMethod(info.FullMethod)
		if requiredScope == "" {
			log.Warn().Str("method", info.FullMethod).Msg("No scope mapping found for method")
			return nil, status.Errorf(codes.Internal, "no scope mapping for method")
		}

		// Block analytics functionality - not available to plugins
		if isAnalyticsScope(requiredScope) {
			log.Warn().
				Str("method", info.FullMethod).
				Str("scope", requiredScope).
				Msg("Analytics functionality blocked for plugins")
			return nil, status.Errorf(codes.Unimplemented, "analytics functionality is not available to plugins - analytics logic remains in REST API layer")
		}

		// Validate plugin service access authorization
		var plugin models.Plugin
		if err := db.First(&plugin, pluginID).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				log.Error().Uint("plugin_id", pluginID).Msg("Plugin not found during auth")
				return nil, status.Errorf(codes.Unauthenticated, "plugin not found")
			}
			log.Error().Err(err).Uint("plugin_id", pluginID).Msg("Database error during plugin auth")
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
			Str("method", info.FullMethod).
			Msg("Plugin authenticated and authorized")

		// Add plugin info to context for downstream services
		ctx = SetPluginInContext(ctx, &plugin)

		return handler(ctx, req)
	}
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
		"/ai_studio_management.AIStudioManagementService/CreateLLM":     models.ServiceScopeLLMsWrite,
		"/ai_studio_management.AIStudioManagementService/UpdateLLM":     models.ServiceScopeLLMsWrite,
		"/ai_studio_management.AIStudioManagementService/DeleteLLM":     models.ServiceScopeLLMsWrite,

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
	if pluginID, ok := ctx.Value(pluginContextKey{}).(uint); ok {
		return pluginID
	}
	return 0
}

// SetPluginIDInContext stores the plugin ID in the context
// Used by the AI Studio plugin manager when making calls
func SetPluginIDInContext(ctx context.Context, pluginID uint) context.Context {
	return context.WithValue(ctx, pluginContextKey{}, pluginID)
}

// SetPluginInContext stores the full plugin info in the context for downstream use
func SetPluginInContext(ctx context.Context, plugin *models.Plugin) context.Context {
	type pluginInfoKey struct{}
	return context.WithValue(ctx, pluginInfoKey{}, plugin)
}

// GetPluginFromContext retrieves the full plugin info from the context
func GetPluginFromContext(ctx context.Context) (*models.Plugin, bool) {
	type pluginInfoKey struct{}
	if plugin, ok := ctx.Value(pluginInfoKey{}).(*models.Plugin); ok {
		return plugin, true
	}
	return nil, false
}