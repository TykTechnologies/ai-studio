package models

import (
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

// Plugin represents a plugin configuration in the hub-and-spoke system
type Plugin struct {
	gorm.Model
	ID          uint                   `json:"id" gorm:"primaryKey"`
	Name        string                 `json:"name" gorm:"not null"`
	Slug        string                 `json:"slug" gorm:"uniqueIndex;not null"`
	Description string                 `json:"description"`
	Command     string                 `json:"command" gorm:"not null;size:500"`
	Checksum    string                 `json:"checksum" gorm:"size:255"` // Optional - for future use
	Config      map[string]interface{} `json:"config" gorm:"serializer:json"`
	HookType    string                 `json:"hook_type" gorm:"not null;size:50;index:idx_plugins_hook_type"`
	IsActive    bool                   `json:"is_active" gorm:"index:idx_plugins_is_active"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
	DeletedAt   gorm.DeletedAt         `json:"deleted_at,omitempty" gorm:"index"`

	// Hub-and-Spoke Configuration
	Namespace   string                 `json:"namespace" gorm:"default:'';index:idx_plugin_namespace"`

	// Plugin Type and OCI Support
	PluginType  string                 `json:"plugin_type" gorm:"not null;default:'gateway';size:50;index:idx_plugins_type"` // "gateway" or "ai_studio"
	OCIReference string                `json:"oci_reference" gorm:"size:500"`                                                // OCI artifact reference (for OCI plugins)
	Manifest    map[string]interface{} `json:"manifest" gorm:"serializer:json"`                                              // Plugin manifest for UI extensions

	// Service Access Control (for AI Studio plugins)
	ServiceAccessAuthorized bool     `json:"service_access_authorized" gorm:"default:false;index:idx_plugins_service_access"` // Admin authorization for service access
	ServiceScopes          []string `json:"service_scopes" gorm:"serializer:json"`                                            // Authorized service scopes from manifest

	// Relationships
	LLMs []LLM `json:"llms,omitempty" gorm:"many2many:llm_plugins;"`
}

// TableName returns the table name for the Plugin model
func (Plugin) TableName() string {
	return "plugins"
}

type Plugins []Plugin

// Plugin hook type constants
const (
	// Gateway plugin hook types
	HookTypePreAuth        = "pre_auth"
	HookTypeAuth           = "auth"
	HookTypePostAuth       = "post_auth"
	HookTypeOnResponse     = "on_response"
	HookTypeDataCollection = "data_collection"

	// AI Studio plugin hook types
	HookTypeStudioUI = "studio_ui" // AI Studio UI extension plugins
	HookTypeAgent    = "agent"     // AI Studio agent plugins
)

// Plugin type constants
const (
	PluginTypeGateway   = "gateway"   // Microgateway plugins
	PluginTypeAIStudio  = "ai_studio" // AI Studio UI extension plugins
	PluginTypeAgent     = "agent"     // AI Studio agent plugins
)

// NewPlugin creates a new Plugin instance
func NewPlugin() *Plugin {
	return &Plugin{
		IsActive:   true,
		Config:     make(map[string]interface{}),
		PluginType: PluginTypeGateway, // Default to gateway plugin
		Manifest:   make(map[string]interface{}),
	}
}

// Get retrieves a plugin by ID
func (p *Plugin) Get(db *gorm.DB, id uint) error {
	return db.Preload("LLMs").First(p, id).Error
}

// GetBySlug retrieves a plugin by slug
func (p *Plugin) GetBySlug(db *gorm.DB, slug string) error {
	return db.Preload("LLMs").Where("slug = ?", slug).First(p).Error
}

// Create creates a new plugin
func (p *Plugin) Create(db *gorm.DB) error {
	return db.Create(p).Error
}

// Update updates an existing plugin
func (p *Plugin) Update(db *gorm.DB) error {
	return db.Save(p).Error
}

// Delete soft deletes a plugin
func (p *Plugin) Delete(db *gorm.DB) error {
	return db.Delete(p).Error
}

// IsValidHookType validates if the hook type is supported
func (p *Plugin) IsValidHookType() bool {
	validTypes := []string{
		HookTypePreAuth,
		HookTypeAuth,
		HookTypePostAuth,
		HookTypeOnResponse,
		HookTypeDataCollection,
		HookTypeStudioUI,
		HookTypeAgent,
	}

	for _, validType := range validTypes {
		if p.HookType == validType {
			return true
		}
	}
	return false
}

// IsValidPluginType validates if the plugin type is supported
func (p *Plugin) IsValidPluginType() bool {
	return p.PluginType == PluginTypeGateway || p.PluginType == PluginTypeAIStudio || p.PluginType == PluginTypeAgent
}

// IsAIStudioPlugin returns true if this is an AI Studio plugin
func (p *Plugin) IsAIStudioPlugin() bool {
	return p.PluginType == PluginTypeAIStudio
}

// IsGatewayPlugin returns true if this is a Gateway plugin
func (p *Plugin) IsGatewayPlugin() bool {
	return p.PluginType == PluginTypeGateway
}

// IsAgentPlugin returns true if this is an Agent plugin
func (p *Plugin) IsAgentPlugin() bool {
	return p.PluginType == PluginTypeAgent
}

// IsOCIPlugin returns true if this plugin uses OCI (determined by command prefix)
func (p *Plugin) IsOCIPlugin() bool {
	return strings.HasPrefix(p.Command, "oci://")
}

// IsLocalPlugin returns true if this plugin is a local binary
func (p *Plugin) IsLocalPlugin() bool {
	return !strings.HasPrefix(p.Command, "oci://") && !strings.HasPrefix(p.Command, "grpc://")
}

// IsGRPCPlugin returns true if this plugin connects to external gRPC
func (p *Plugin) IsGRPCPlugin() bool {
	return strings.HasPrefix(p.Command, "grpc://")
}

// GetPluginsForLLM returns plugins associated with an LLM, ordered by execution order
func (plugins *Plugins) GetPluginsForLLM(db *gorm.DB, llmID uint) error {
	return db.Joins("JOIN llm_plugins lp ON lp.plugin_id = plugins.id").
		Where("lp.llm_id = ? AND lp.is_active = ? AND plugins.is_active = ?", llmID, true, true).
		Order("lp.order_index ASC").
		Find(plugins).Error
}

// GetPluginsByHookType returns plugins filtered by hook type
func (plugins *Plugins) GetPluginsByHookType(db *gorm.DB, hookType string) error {
	return db.Where("hook_type = ? AND is_active = ?", hookType, true).
		Order("created_at DESC").
		Find(plugins).Error
}

// GetPluginsInNamespace returns plugins in a specific namespace (including global)
func (plugins *Plugins) GetPluginsInNamespace(db *gorm.DB, namespace string) error {
	query := db.Where("is_active = ?", true)
	if namespace == "" || namespace == "__ALL_NAMESPACES__" {
		// No namespace filtering - return all plugins (matches App/Filter behavior)
		// No additional WHERE clause needed
	} else {
		// Specific namespace - global + matching namespace
		query = query.Where("(namespace = '' OR namespace = ?)", namespace)
	}

	return query.Order("created_at DESC").Find(plugins).Error
}

// ListWithPagination returns paginated list of plugins with filtering
func (plugins *Plugins) ListWithPagination(db *gorm.DB, pageSize, pageNumber int, all bool, hookType string, isActive bool, namespace string) (int64, int, error) {
	var totalCount int64
	query := db.Model(&Plugin{})

	// Apply filters
	if hookType != "" {
		query = query.Where("hook_type = ?", hookType)
	}
	query = query.Where("is_active = ?", isActive)

	// Apply namespace filtering
	if namespace == "__ALL_NAMESPACES__" {
		// No namespace filtering - return plugins from all namespaces
		// No additional WHERE clause needed
	} else if namespace == "" {
		// Empty namespace means global plugins only (plugins with no namespace)
		query = query.Where("namespace = ''")
	} else {
		// Specific namespace: include global plugins (empty namespace) + plugins in specified namespace
		query = query.Where("namespace = '' OR namespace = ?", namespace)
	}

	if err := query.Count(&totalCount).Error; err != nil {
		return 0, 0, err
	}

	totalPages := 0
	if totalCount > 0 {
		if all {
			totalPages = 1
		} else {
			totalPages = int(totalCount) / pageSize
			if int(totalCount)%pageSize != 0 {
				totalPages++
			}
		}
	}

	if !all {
		offset := (pageNumber - 1) * pageSize
		query = query.Offset(offset).Limit(pageSize)
	}

	err := query.Preload("LLMs").Order("created_at DESC").Find(plugins).Error
	return totalCount, totalPages, err
}

// ListAllWithPagination returns paginated list of all plugins (active and inactive) with filtering
func (plugins *Plugins) ListAllWithPagination(db *gorm.DB, pageSize, pageNumber int, all bool, hookType string, namespace string) (int64, int, error) {
	var totalCount int64
	query := db.Model(&Plugin{})

	// Apply filters
	if hookType != "" {
		query = query.Where("hook_type = ?", hookType)
	}
	// Note: No is_active filter - this returns both active and inactive

	// Apply namespace filtering
	if namespace == "__ALL_NAMESPACES__" {
		// No namespace filtering - return plugins from all namespaces
		// No additional WHERE clause needed
	} else if namespace == "" {
		// Empty namespace means global plugins only (plugins with no namespace)
		query = query.Where("namespace = ''")
	} else {
		// Specific namespace: include global plugins (empty namespace) + plugins in specified namespace
		query = query.Where("namespace = '' OR namespace = ?", namespace)
	}

	if err := query.Count(&totalCount).Error; err != nil {
		return 0, 0, err
	}

	totalPages := 0
	if totalCount > 0 {
		if all {
			totalPages = 1
		} else {
			totalPages = int(totalCount) / pageSize
			if int(totalCount)%pageSize != 0 {
				totalPages++
			}
		}
	}

	if !all {
		offset := (pageNumber - 1) * pageSize
		query = query.Offset(offset).Limit(pageSize)
	}

	err := query.Preload("LLMs").Order("created_at DESC").Find(plugins).Error
	return totalCount, totalPages, err
}

// CountPlugins returns the total number of plugins
func (p *Plugin) CountPlugins(db *gorm.DB) (int64, error) {
	var count int64
	err := db.Model(&Plugin{}).Count(&count).Error
	return count, err
}

// CountActivePlugins returns the count of active plugins
func (p *Plugin) CountActivePlugins(db *gorm.DB) (int64, error) {
	var count int64
	err := db.Model(&Plugin{}).Where("is_active = ?", true).Count(&count).Error
	return count, err
}

// CountPluginsByHookType returns the count of plugins by hook type
func (p *Plugin) CountPluginsByHookType(db *gorm.DB, hookType string) (int64, error) {
	var count int64
	err := db.Model(&Plugin{}).Where("hook_type = ? AND is_active = ?", hookType, true).Count(&count).Error
	return count, err
}

// Service Access Control Methods

// HasServiceAccess returns true if the plugin is authorized for service access
func (p *Plugin) HasServiceAccess() bool {
	return p.ServiceAccessAuthorized
}

// HasServiceScope returns true if the plugin has the specified service scope
func (p *Plugin) HasServiceScope(scope string) bool {
	for _, s := range p.ServiceScopes {
		if s == scope {
			return true
		}
	}
	return false
}

// AuthorizeServiceAccess grants service access to the plugin with specified scopes
func (p *Plugin) AuthorizeServiceAccess(db *gorm.DB, scopes []string) error {
	p.ServiceAccessAuthorized = true
	p.ServiceScopes = scopes
	return db.Save(p).Error
}

// RevokeServiceAccess revokes service access from the plugin
func (p *Plugin) RevokeServiceAccess(db *gorm.DB) error {
	p.ServiceAccessAuthorized = false
	p.ServiceScopes = []string{}
	return db.Save(p).Error
}

// UpdateServiceScopes updates the authorized service scopes for the plugin
func (p *Plugin) UpdateServiceScopes(db *gorm.DB, scopes []string) error {
	if !p.ServiceAccessAuthorized {
		return fmt.Errorf("service access not authorized for plugin")
	}
	p.ServiceScopes = scopes
	return db.Save(p).Error
}

// Service scope constants for AI Studio plugins
const (
	// Plugin management scopes
	ServiceScopePluginsRead   = "plugins.read"
	ServiceScopePluginsWrite  = "plugins.write"
	ServiceScopePluginsConfig = "plugins.config"

	// LLM management scopes
	ServiceScopeLLMsRead     = "llms.read"
	ServiceScopeLLMsWrite    = "llms.write"
	ServiceScopeLLMsConfig   = "llms.config"
	ServiceScopeLLMsProxy    = "llms.proxy" // Proxy LLM requests (for agent plugins)

	// Analytics scopes
	ServiceScopeAnalyticsRead = "analytics.read"

	// App management scopes
	ServiceScopeAppsRead  = "apps.read"
	ServiceScopeAppsWrite = "apps.write"

	// Tool management scopes
	ServiceScopeToolsRead       = "tools.read"
	ServiceScopeToolsWrite      = "tools.write"
	ServiceScopeToolsOperations = "tools.operations" // Call tool operations
	ServiceScopeToolsCall       = "tools.call"       // Execute tool operations

	// Datasource management scopes
	ServiceScopeDatasourcesRead       = "datasources.read"
	ServiceScopeDatasourcesWrite      = "datasources.write"
	ServiceScopeDatasourcesEmbeddings = "datasources.embeddings"
	ServiceScopeDatasourcesQuery      = "datasources.query" // Query datasources (for agent plugins)

	// Data catalogue management scopes
	ServiceScopeDataCataloguesRead  = "data-catalogues.read"
	ServiceScopeDataCataloguesWrite = "data-catalogues.write"

	// Tags management scopes
	ServiceScopeTagsRead  = "tags.read"
	ServiceScopeTagsWrite = "tags.write"

	// Filter management scopes
	ServiceScopeFiltersRead  = "filters.read"
	ServiceScopeFiltersWrite = "filters.write"

	// Model pricing scopes
	ServiceScopePricingRead  = "pricing.read"
	ServiceScopePricingWrite = "pricing.write"

	// Vendor information scopes
	ServiceScopeVendorsRead = "vendors.read"

	// Advanced analytics scopes
	ServiceScopeAnalyticsDetailed = "analytics.detailed"
	ServiceScopeAnalyticsReports  = "analytics.reports"

	// System scopes
	ServiceScopeSystemRead = "system.read"

	// Key-Value storage scopes
	ServiceScopeKVReadWrite = "kv.readwrite" // Plugin key-value storage access
)