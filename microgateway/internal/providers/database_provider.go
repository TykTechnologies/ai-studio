// internal/providers/database_provider.go
package providers

import (
	"fmt"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/v2/pkg/config"
	"github.com/rs/zerolog/log"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// DatabaseProvider implements ConfigurationProvider using direct database access
type DatabaseProvider struct {
	db               *gorm.DB
	namespace        string
	namespaceFilter  NamespaceFilter
}

// NewDatabaseProvider creates a new database-backed configuration provider
func NewDatabaseProvider(db *gorm.DB, namespace string) *DatabaseProvider {
	return &DatabaseProvider{
		db:              db,
		namespace:       namespace,
		namespaceFilter: &DefaultNamespaceFilter{},
	}
}

// GetProviderType returns the provider type
func (p *DatabaseProvider) GetProviderType() ProviderType {
	return ProviderTypeDatabase
}

// IsHealthy checks if the database connection is healthy
func (p *DatabaseProvider) IsHealthy() bool {
	sqlDB, err := p.db.DB()
	if err != nil {
		return false
	}
	return sqlDB.Ping() == nil
}

// GetNamespace returns the provider's namespace
func (p *DatabaseProvider) GetNamespace() string {
	return p.namespace
}

// GetLLM retrieves an LLM by ID with namespace filtering
func (p *DatabaseProvider) GetLLM(id uint) (*database.LLM, error) {
	var llm database.LLM
	
	query := p.db.Where("id = ?", id)
	
	// Apply namespace filtering if provider has a namespace
	if p.namespace != "" {
		query = query.Where("(namespace = '' OR namespace = ?)", p.namespace)
	}
	
	err := query.Preload("Filters").Preload("Plugins").First(&llm).Error
	if err != nil {
		return nil, err
	}
	
	// Additional check with namespace filter
	if !p.namespaceFilter.MatchesNamespace(llm.Namespace, p.namespace) {
		return nil, gorm.ErrRecordNotFound
	}
	
	return &llm, nil
}

// GetLLMBySlug retrieves an LLM by slug with namespace filtering
func (p *DatabaseProvider) GetLLMBySlug(slug string) (*database.LLM, error) {
	var llm database.LLM
	
	query := p.db.Where("slug = ?", slug)
	
	// Apply namespace filtering
	if p.namespace != "" {
		query = query.Where("(namespace = '' OR namespace = ?)", p.namespace)
	}
	
	err := query.Preload("Filters").Preload("Plugins").First(&llm).Error
	if err != nil {
		return nil, err
	}
	
	return &llm, nil
}

// ListLLMs retrieves LLMs with namespace and active filtering
func (p *DatabaseProvider) ListLLMs(namespace string, active bool) ([]database.LLM, error) {
	var llms []database.LLM
	
	query := p.db.Model(&database.LLM{})
	
	// Filter by active status
	if active {
		query = query.Where("is_active = ?", true)
	}
	
	// Apply namespace filtering
	targetNamespace := namespace
	if targetNamespace == "" {
		targetNamespace = p.namespace
	}
	
	// Namespace filtering logic:
	// - If provider namespace is empty (""), only show global objects (namespace = "")
	// - If provider namespace is specific, show global + matching namespace objects
	if targetNamespace == "" {
		// Global provider - only global objects
		query = query.Where("namespace = ''")
	} else {
		// Specific namespace provider - global + matching objects
		query = query.Where("(namespace = '' OR namespace = ?)", targetNamespace)
	}
	
	err := query.Preload("Filters").Preload("Plugins").Find(&llms).Error
	return llms, err
}

// GetApp retrieves an app by ID with namespace filtering
func (p *DatabaseProvider) GetApp(id uint) (*database.App, error) {
	var app database.App
	
	query := p.db.Where("id = ?", id)
	
	if p.namespace != "" {
		query = query.Where("(namespace = '' OR namespace = ?)", p.namespace)
	}
	
	err := query.Preload("LLMs").First(&app).Error
	if err != nil {
		return nil, err
	}
	
	return &app, nil
}

// ListApps retrieves apps with namespace filtering
func (p *DatabaseProvider) ListApps(namespace string, active bool) ([]database.App, error) {
	var apps []database.App
	
	query := p.db.Model(&database.App{})
	
	if active {
		query = query.Where("is_active = ?", true)
	}
	
	targetNamespace := namespace
	if targetNamespace == "" {
		targetNamespace = p.namespace
	}
	
	// Apply same namespace logic as ListLLMs
	if targetNamespace == "" {
		query = query.Where("namespace = ''")
	} else {
		query = query.Where("(namespace = '' OR namespace = ?)", targetNamespace)
	}
	
	err := query.Preload("LLMs").Find(&apps).Error
	return apps, err
}

// GetToken retrieves and validates a token with namespace filtering
func (p *DatabaseProvider) GetToken(token string) (*database.APIToken, error) {
	var apiToken database.APIToken
	
	query := p.db.Where("token = ?", token)
	
	if p.namespace != "" {
		query = query.Where("(namespace = '' OR namespace = ?)", p.namespace)
	}
	
	err := query.Preload("App").First(&apiToken).Error
	return &apiToken, err
}

// ValidateToken validates a token and returns it if valid and accessible
func (p *DatabaseProvider) ValidateToken(token string) (*database.APIToken, error) {
	apiToken, err := p.GetToken(token)
	if err != nil {
		return nil, err
	}
	
	// Check if token is active
	if !apiToken.IsActive {
		return nil, fmt.Errorf("token is inactive")
	}
	
	// Check expiration
	if apiToken.ExpiresAt != nil && apiToken.ExpiresAt.Before(time.Now()) {
		return nil, fmt.Errorf("token is expired")
	}
	
	return apiToken, nil
}

// GetModelPrice retrieves model pricing with namespace filtering
func (p *DatabaseProvider) GetModelPrice(vendor, model string) (*database.ModelPrice, error) {
	var price database.ModelPrice
	
	query := p.db.Where("vendor = ? AND model_name = ?", vendor, model)
	
	if p.namespace != "" {
		query = query.Where("(namespace = '' OR namespace = ?)", p.namespace)
	}
	
	err := query.First(&price).Error
	return &price, err
}

// ListModelPrices retrieves model prices with namespace filtering
func (p *DatabaseProvider) ListModelPrices(namespace string) ([]database.ModelPrice, error) {
	var prices []database.ModelPrice
	
	query := p.db.Model(&database.ModelPrice{})
	
	targetNamespace := namespace
	if targetNamespace == "" {
		targetNamespace = p.namespace
	}
	
	if targetNamespace != "" {
		query = query.Where("(namespace = '' OR namespace = ?)", targetNamespace)
	}
	
	err := query.Find(&prices).Error
	return prices, err
}

// GetFilter retrieves a filter by ID with namespace filtering
func (p *DatabaseProvider) GetFilter(id uint) (*database.Filter, error) {
	var filter database.Filter
	
	query := p.db.Where("id = ?", id)
	
	if p.namespace != "" {
		query = query.Where("(namespace = '' OR namespace = ?)", p.namespace)
	}
	
	err := query.First(&filter).Error
	return &filter, err
}

// GetFiltersForLLM retrieves filters associated with an LLM, respecting namespace
func (p *DatabaseProvider) GetFiltersForLLM(llmID uint) ([]database.Filter, error) {
	// First check if we can access the LLM
	_, err := p.GetLLM(llmID)
	if err != nil {
		return nil, err
	}
	
	var filters []database.Filter
	
	query := p.db.Joins("JOIN llm_filters lf ON lf.filter_id = filters.id").
		Where("lf.llm_id = ? AND lf.is_active = ? AND filters.is_active = ?", llmID, true, true)
	
	// Apply namespace filtering to filters
	if p.namespace != "" {
		query = query.Where("(filters.namespace = '' OR filters.namespace = ?)", p.namespace)
	}
	
	err = query.Order("lf.order_index ASC").Find(&filters).Error
	return filters, err
}

// ListFilters retrieves filters with namespace filtering
func (p *DatabaseProvider) ListFilters(namespace string, active bool) ([]database.Filter, error) {
	var filters []database.Filter
	
	query := p.db.Model(&database.Filter{})
	
	if active {
		query = query.Where("is_active = ?", true)
	}
	
	targetNamespace := namespace
	if targetNamespace == "" {
		targetNamespace = p.namespace
	}
	
	if targetNamespace != "" {
		query = query.Where("(namespace = '' OR namespace = ?)", targetNamespace)
	}
	
	err := query.Find(&filters).Error
	return filters, err
}

// GetPlugin retrieves a plugin by ID with namespace filtering
func (p *DatabaseProvider) GetPlugin(id uint) (*database.Plugin, error) {
	var plugin database.Plugin
	
	query := p.db.Where("id = ?", id)
	
	if p.namespace != "" {
		query = query.Where("(namespace = '' OR namespace = ?)", p.namespace)
	}
	
	err := query.First(&plugin).Error
	return &plugin, err
}

// GetPluginsForLLM retrieves plugins associated with an LLM with merged configurations, respecting namespace
func (p *DatabaseProvider) GetPluginsForLLM(llmID uint) ([]database.Plugin, error) {
	// First check if we can access the LLM
	_, err := p.GetLLM(llmID)
	if err != nil {
		return nil, err
	}

	// Get LLM-plugin associations first
	var llmPlugins []database.LLMPlugin
	err = p.db.Where("llm_id = ? AND is_active = ?", llmID, true).
		Order("order_index ASC").
		Find(&llmPlugins).Error

	if err != nil {
		return nil, fmt.Errorf("failed to get LLM plugin associations: %w", err)
	}

	if len(llmPlugins) == 0 {
		return []database.Plugin{}, nil
	}

	// Get plugin IDs
	pluginIDs := make([]uint, len(llmPlugins))
	for i, lp := range llmPlugins {
		pluginIDs[i] = lp.PluginID
	}

	// Get plugins with active filter and namespace filtering
	var plugins []database.Plugin
	query := p.db.Where("id IN ? AND is_active = ?", pluginIDs, true)

	// Apply namespace filtering to plugins
	if p.namespace != "" {
		query = query.Where("(namespace = '' OR namespace = ?)", p.namespace)
	}

	err = query.Find(&plugins).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get plugins: %w", err)
	}

	// Create plugin map for fast lookup
	pluginMap := make(map[uint]database.Plugin)
	for _, plugin := range plugins {
		pluginMap[plugin.ID] = plugin
	}

	// Build result with merged configurations, maintaining order
	result := make([]database.Plugin, 0, len(llmPlugins))

	for _, llmPlugin := range llmPlugins {
		plugin, exists := pluginMap[llmPlugin.PluginID]
		if !exists {
			// Plugin might be inactive or filtered out by namespace, skip it
			continue
		}

		// Merge base plugin config with per-LLM override
		baseConfigJSON := []byte(plugin.Config)
		overrideConfigJSON := []byte(llmPlugin.ConfigOverride)

		mergedConfigJSON, err := config.MergePluginConfigsJSON(baseConfigJSON, overrideConfigJSON)
		if err != nil {
			log.Error().Err(err).
				Uint("plugin_id", plugin.ID).
				Uint("llm_id", llmID).
				Msg("Failed to merge plugin config, using base config")
			mergedConfigJSON = baseConfigJSON
		}

		// Update plugin with merged config
		plugin.Config = datatypes.JSON(mergedConfigJSON)

		log.Debug().
			Uint("plugin_id", plugin.ID).
			Uint("llm_id", llmID).
			Bool("has_override", len(overrideConfigJSON) > 0).
			Msg("Merged plugin configuration for LLM in database provider")

		result = append(result, plugin)
	}

	return result, nil
}

// GetAllLLMAssociatedPlugins returns all active plugins linked to at least one
// active LLM association, using a single JOIN query.
func (p *DatabaseProvider) GetAllLLMAssociatedPlugins() ([]database.Plugin, error) {
	var plugins []database.Plugin
	query := p.db.
		Distinct("plugins.*").
		Joins("JOIN llm_plugins ON llm_plugins.plugin_id = plugins.id AND llm_plugins.is_active = ?", true).
		Where("plugins.is_active = ? AND plugins.deleted_at IS NULL", true)

	if p.namespace != "" {
		query = query.Where("(plugins.namespace = '' OR plugins.namespace = ?)", p.namespace)
	}

	if err := query.Find(&plugins).Error; err != nil {
		return nil, fmt.Errorf("failed to get all LLM-associated plugins: %w", err)
	}
	return plugins, nil
}

// ListPlugins retrieves plugins with namespace and type filtering
func (p *DatabaseProvider) ListPlugins(namespace string, hookType string, active bool) ([]database.Plugin, error) {
	var plugins []database.Plugin
	
	query := p.db.Model(&database.Plugin{})
	
	if active {
		query = query.Where("is_active = ?", true)
	}
	
	if hookType != "" {
		query = query.Where("hook_type = ?", hookType)
	}
	
	targetNamespace := namespace
	if targetNamespace == "" {
		targetNamespace = p.namespace
	}
	
	if targetNamespace != "" {
		query = query.Where("(namespace = '' OR namespace = ?)", targetNamespace)
	}
	
	err := query.Find(&plugins).Error
	return plugins, err
}