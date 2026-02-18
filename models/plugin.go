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
	Description string                 `json:"description"`
	Command     string                 `json:"command" gorm:"not null;size:500"`
	Checksum    string                 `json:"checksum" gorm:"size:255"` // Optional - for future use
	Config      map[string]interface{} `json:"config" gorm:"serializer:json"`
	HookType    string                 `json:"hook_type" gorm:"not null;size:50;index:idx_plugins_hook_type"`
	HookTypes           []string               `json:"hook_types" gorm:"serializer:json"`                        // All hook types this plugin supports
	HookTypesCustomized bool                   `json:"hook_types_customized" gorm:"default:false"`               // True if user overrode manifest hooks
	IsActive    bool                   `json:"is_active" gorm:"index:idx_plugins_is_active"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
	DeletedAt   gorm.DeletedAt         `json:"deleted_at,omitempty" gorm:"index"`

	// Hub-and-Spoke Configuration
	Namespace   string                 `json:"namespace" gorm:"default:'';index:idx_plugin_namespace"`

	// OCI Support
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
	HookTypeStudioUI       = "studio_ui"        // AI Studio UI extension plugins
	HookTypePortalUI       = "portal_ui"        // AI Portal UI extension plugins (end-user facing)
	HookTypeAgent          = "agent"             // AI Studio agent plugins
	HookTypeObjectHooks    = "object_hooks"      // AI Studio object interaction hooks (CRUD operations)
	HookTypeCustomEndpoint  = "custom_endpoint"   // Custom HTTP endpoints served by plugin
	HookTypeResourceProvider = "resource_provider" // Plugin provides custom resource types for Apps
)

// validHookTypes contains all valid hook type constants
var validHookTypes = []string{
	HookTypePreAuth,
	HookTypeAuth,
	HookTypePostAuth,
	HookTypeOnResponse,
	HookTypeDataCollection,
	HookTypeStudioUI,
	HookTypePortalUI,
	HookTypeAgent,
	HookTypeObjectHooks,
	HookTypeCustomEndpoint,
	HookTypeResourceProvider,
}

// IsValidHookType validates if a hook type string is valid
func IsValidHookType(hookType string) bool {
	// Allow "pending" as a temporary placeholder during plugin creation
	if hookType == "pending" {
		return true
	}
	for _, valid := range validHookTypes {
		if hookType == valid {
			return true
		}
	}
	return false
}

// GetValidHookTypes returns all valid hook types
func GetValidHookTypes() []string {
	result := make([]string, len(validHookTypes))
	copy(result, validHookTypes)
	return result
}

// NewPlugin creates a new Plugin instance
func NewPlugin() *Plugin {
	return &Plugin{
		IsActive:  true,
		Config:    make(map[string]interface{}),
		Manifest:  make(map[string]interface{}),
		HookTypes: []string{}, // Empty, must be set explicitly
	}
}

// Get retrieves a plugin by ID
func (p *Plugin) Get(db *gorm.DB, id uint) error {
	return db.Preload("LLMs").First(p, id).Error
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
	return IsValidHookType(p.HookType)
}

// SupportsHookType checks if plugin supports a specific hook type
func (p *Plugin) SupportsHookType(hookType string) bool {
	// Check primary hook
	if p.HookType == hookType {
		return true
	}
	// Check additional hooks
	for _, ht := range p.HookTypes {
		if ht == hookType {
			return true
		}
	}
	return false
}

// GetAllHookTypes returns all hook types (primary + additional)
func (p *Plugin) GetAllHookTypes() []string {
	hookSet := make(map[string]bool)

	// Add primary hook
	if p.HookType != "" {
		hookSet[p.HookType] = true
	}

	// Add additional hooks
	for _, ht := range p.HookTypes {
		if ht != "" {
			hookSet[ht] = true
		}
	}

	// Convert to sorted slice for consistency (using validHookTypes order)
	result := make([]string, 0, len(hookSet))
	for _, validHook := range validHookTypes {
		if hookSet[validHook] {
			result = append(result, validHook)
		}
	}

	return result
}

// GetCapabilityCategory returns a human-readable category string
func (p *Plugin) GetCapabilityCategory() string {
	hooks := p.GetAllHookTypes()

	hasGateway := false
	hasUI := false
	hasAgent := false

	for _, ht := range hooks {
		switch ht {
		case HookTypeAuth, HookTypePreAuth, HookTypePostAuth, HookTypeOnResponse, HookTypeDataCollection, HookTypeCustomEndpoint:
			hasGateway = true
		case HookTypeStudioUI:
			hasUI = true
		case HookTypeAgent:
			hasAgent = true
		}
	}

	// Return specific combinations
	if hasGateway && hasUI && hasAgent {
		return "Full-Stack Plugin"
	}
	if hasGateway && hasUI {
		return "Gateway + UI"
	}
	if hasGateway && hasAgent {
		return "Gateway + Agent"
	}
	if hasUI && hasAgent {
		return "Agent + UI"
	}
	if hasGateway {
		return "Gateway Plugin"
	}
	if hasUI {
		return "UI Extension"
	}
	if hasAgent {
		return "Agent Plugin"
	}

	return "Uncategorized"
}

// ValidateHookTypes validates all hook types are valid
func (p *Plugin) ValidateHookTypes() error {
	allHooks := p.GetAllHookTypes()

	if len(allHooks) == 0 {
		return fmt.Errorf("plugin must declare at least one hook type")
	}

	for _, hook := range allHooks {
		if !IsValidHookType(hook) {
			return fmt.Errorf("invalid hook type '%s' - must be one of: %v", hook, GetValidHookTypes())
		}
	}

	return nil
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

	// Scheduler management scopes
	ServiceScopeSchedulerManage = "scheduler.manage" // Manage plugin schedules
)