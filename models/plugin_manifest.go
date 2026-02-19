package models

import (
	"fmt"
	"time"

	"gorm.io/gorm"
)

// PluginManifest represents the manifest structure defined in Hot-load-ui-plugins-plan.md
type PluginManifest struct {
	// Basic plugin information
	ID          string `json:"id" binding:"required"`
	Version     string `json:"version" binding:"required"`
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`

	// Capabilities declares what hooks this plugin implements
	Capabilities *PluginCapabilities `json:"capabilities" binding:"required"`

	// Permissions and security
	Permissions struct {
		KV          []string `json:"kv"`           // KV access permissions: read, write, list
		RPC         []string `json:"rpc"`          // RPC permissions: call
		Routes      []string `json:"routes"`       // Route patterns this plugin can register
		UI          []string `json:"ui"`           // UI permissions: sidebar.register, route.register
		PortalUI    []string `json:"portal_ui"`    // Portal UI permissions: sidebar.register, route.register
		Services    []string `json:"services"`     // AI Studio service access scopes: analytics.read, plugins.config, etc.
		ObjectHooks []string `json:"object_hooks"` // Object hook permissions: llm.before_create, datasource.after_update, etc.
	} `json:"permissions"`

	// Key-value namespace for plugin data
	KVNamespace string `json:"kvNamespace"`

	// RPC configuration
	RPC *struct {
		BasePath   string `json:"basePath"`   // Base path for RPC endpoints
		Proto      string `json:"proto"`      // Path to proto file
		Entrypoint string `json:"entrypoint"` // gRPC service method name
	} `json:"rpc,omitempty"`

	// UI configuration (admin interface)
	UI *struct {
		Slots []UISlot `json:"slots"` // UI slots this plugin registers in admin
	} `json:"ui,omitempty"`

	// Portal UI configuration (end-user portal)
	Portal *struct {
		Slots []PortalUISlot `json:"slots"` // UI slots this plugin registers in portal
	} `json:"portal,omitempty"`

	// Compatibility requirements
	Compat struct {
		App string   `json:"app"` // App version compatibility (semver range)
		API []string `json:"api"` // Required API versions
	} `json:"compat"`

	// Security settings
	Security *struct {
		CSP string `json:"csp"` // Content Security Policy for plugin UI
	} `json:"security,omitempty"`

	// Static assets
	Assets []string `json:"assets"`

	// Scheduled tasks
	Schedules []ScheduleDefinition `json:"schedules,omitempty"`

	// Resource types provided by this plugin (for ResourceProvider capability)
	ResourceTypes []ManifestResourceType `json:"resource_types,omitempty"`
}

// ManifestResourceType declares a resource type in the plugin manifest
type ManifestResourceType struct {
	Slug                string `json:"slug" binding:"required"`
	Name                string `json:"name" binding:"required"`
	Description         string `json:"description"`
	Icon                string `json:"icon"`
	HasPrivacyScore     bool   `json:"has_privacy_score"`
	SupportsSubmissions bool   `json:"supports_submissions"`
	FormComponent       *struct {
		Tag        string `json:"tag"`
		EntryPoint string `json:"entry_point"`
	} `json:"form_component,omitempty"`
}

// ScheduleDefinition represents a cron-based task schedule in the manifest
type ScheduleDefinition struct {
	ID             string                 `json:"id" binding:"required"`           // Unique schedule identifier
	Name           string                 `json:"name" binding:"required"`         // Human-readable name
	Cron           string                 `json:"cron" binding:"required"`         // Cron expression
	Timezone       string                 `json:"timezone,omitempty"`              // Timezone (default: UTC)
	Enabled        bool                   `json:"enabled"`                         // Whether enabled (default: true)
	TimeoutSeconds int                    `json:"timeout_seconds,omitempty"`       // Max execution time (default: 60)
	Config         map[string]interface{} `json:"config,omitempty"`                // Schedule-specific config
}

// PortalUISlot represents a portal UI extension point with group-based visibility filtering.
// If Groups is empty, the slot is visible to all portal users.
// If Groups has values, only users belonging to at least one of those groups can see it.
type PortalUISlot struct {
	Slot   string       `json:"slot"`             // Slot identifier (e.g., "portal_sidebar.section")
	Label  string       `json:"label"`            // Display label
	Icon   string       `json:"icon"`             // Icon path/URL
	Groups []string     `json:"groups,omitempty"` // Allowed groups (empty = all portal users)
	Items  []UISlotItem `json:"items"`            // Items to mount in this slot
}

// UISlot represents a UI extension point where plugins can mount components
type UISlot struct {
	Slot  string     `json:"slot"`  // Slot identifier (e.g., "sidebar.section")
	Label string     `json:"label"` // Display label
	Icon  string     `json:"icon"`  // Icon path/URL
	Items []UISlotItem `json:"items"` // Items to mount in this slot
}

// UISlotItem represents an individual UI component or route
type UISlotItem struct {
	Type  string    `json:"type"`  // "route" or "component"
	Path  string    `json:"path"`  // Route path
	Title string    `json:"title"` // Display title
	Mount UIMount   `json:"mount"` // Mount configuration
}

// UIMount defines how a UI component should be mounted
type UIMount struct {
	Kind  string                 `json:"kind"`  // "webc", "module-federation", "iframe"
	Tag   string                 `json:"tag,omitempty"`   // Web component tag name
	Entry string                 `json:"entry,omitempty"` // Entry point file
	Props map[string]interface{} `json:"props,omitempty"` // Props to pass to component

	// Module Federation specific
	Remote  string `json:"remote,omitempty"`  // Remote entry point for MF
	Exposed string `json:"exposed,omitempty"` // Exposed module name

	// iFrame specific
	App string `json:"app,omitempty"` // App HTML file for iframe
}

// PluginCapabilities declares plugin hook capabilities
type PluginCapabilities struct {
	Hooks       []string `json:"hooks" binding:"required,min=1"`
	PrimaryHook string   `json:"primary_hook,omitempty"`
}

// RegisteredPlugin represents a plugin with its parsed manifest and runtime info
type RegisteredPlugin struct {
	gorm.Model
	PluginID        uint                   `json:"plugin_id" gorm:"index;constraint:OnUpdate:CASCADE,OnDelete:SET NULL"` // References plugins.id
	ManifestVersion string                 `json:"manifest_version"`
	ParsedManifest  map[string]interface{} `json:"parsed_manifest" gorm:"serializer:json"`
	IsLoaded        bool                   `json:"is_loaded" gorm:"default:false"`
	LoadedAt        *time.Time             `json:"loaded_at"`
	LoadError       string                 `json:"load_error"`
	AssetPaths      []string               `json:"asset_paths" gorm:"serializer:json"`
	CreatedAt       time.Time              `json:"created_at"`
	UpdatedAt       time.Time              `json:"updated_at"`

	// Relationships
	Plugin *Plugin `json:"plugin,omitempty" gorm:"foreignKey:PluginID"`
}

// UIRegistry represents the runtime registry of loaded plugin UI components
type UIRegistry struct {
	gorm.Model
	PluginID       uint                   `json:"plugin_id" gorm:"index;constraint:OnUpdate:CASCADE,OnDelete:SET NULL"`
	SlotType       string                 `json:"slot_type" gorm:"size:100"` // e.g., "sidebar.section"
	RoutePattern   string                 `json:"route_pattern" gorm:"size:255"`
	ComponentTag   string                 `json:"component_tag" gorm:"size:100"`
	EntryPoint     string                 `json:"entry_point" gorm:"size:500"`
	MountConfig    map[string]interface{} `json:"mount_config" gorm:"serializer:json"`
	IsActive       bool                   `json:"is_active" gorm:"default:true"`
	LoadPriority   int                    `json:"load_priority" gorm:"default:0"`
	Scope          string                 `json:"scope" gorm:"size:20;default:admin"`          // "admin" or "portal"
	AllowedGroups  []string               `json:"allowed_groups" gorm:"serializer:json"`        // Empty = all users (portal scope only)
	CreatedAt      time.Time              `json:"created_at"`
	UpdatedAt      time.Time              `json:"updated_at"`

	// Relationships
	Plugin *Plugin `json:"plugin,omitempty" gorm:"foreignKey:PluginID"`
}

// ValidateManifest validates the plugin manifest structure
func (pm *PluginManifest) ValidateManifest() error {
	if pm.ID == "" {
		return fmt.Errorf("manifest ID is required")
	}
	if pm.Version == "" {
		return fmt.Errorf("manifest version is required")
	}
	if pm.Name == "" {
		return fmt.Errorf("manifest name is required")
	}

	// Validate capabilities (required for all plugins)
	if pm.Capabilities == nil {
		return fmt.Errorf("manifest must declare capabilities")
	}
	if len(pm.Capabilities.Hooks) == 0 {
		return fmt.Errorf("manifest must declare at least one hook in capabilities.hooks")
	}

	// Validate each hook type
	for _, hook := range pm.Capabilities.Hooks {
		if !IsValidHookType(hook) {
			return fmt.Errorf("invalid hook type '%s' in manifest - must be one of: %v", hook, GetValidHookTypes())
		}
	}

	// Validate primary hook if specified
	if pm.Capabilities.PrimaryHook != "" {
		if !IsValidHookType(pm.Capabilities.PrimaryHook) {
			return fmt.Errorf("invalid primary_hook '%s' in manifest", pm.Capabilities.PrimaryHook)
		}

		// Ensure primary hook is in hooks array
		found := false
		for _, hook := range pm.Capabilities.Hooks {
			if hook == pm.Capabilities.PrimaryHook {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("primary_hook '%s' must be included in capabilities.hooks array", pm.Capabilities.PrimaryHook)
		}
	}

	// Block analytics service scopes - not available to plugins
	for _, scope := range pm.Permissions.Services {
		if isAnalyticsServiceScope(scope) {
			return fmt.Errorf("analytics functionality is not available to plugins - scope '%s' not allowed", scope)
		}
	}

	// Validate UI slots if present
	if pm.UI != nil {
		for i, slot := range pm.UI.Slots {
			if slot.Slot == "" {
				return fmt.Errorf("slot %d missing slot identifier", i)
			}
			for j, item := range slot.Items {
				if item.Type == "" {
					return fmt.Errorf("slot %d item %d missing type", i, j)
				}
				if item.Mount.Kind == "" {
					return fmt.Errorf("slot %d item %d missing mount kind", i, j)
				}
			}
		}
	}

	// Validate Portal UI slots if present
	if pm.Portal != nil {
		for i, slot := range pm.Portal.Slots {
			if slot.Slot == "" {
				return fmt.Errorf("portal slot %d missing slot identifier", i)
			}
			for j, item := range slot.Items {
				if item.Type == "" {
					return fmt.Errorf("portal slot %d item %d missing type", i, j)
				}
				if item.Mount.Kind == "" {
					return fmt.Errorf("portal slot %d item %d missing mount kind", i, j)
				}
			}
		}
	}

	return nil
}

// GetUIRoutes extracts all routes defined in the manifest
func (pm *PluginManifest) GetUIRoutes() []UISlotItem {
	var routes []UISlotItem
	if pm.UI == nil {
		return routes
	}

	for _, slot := range pm.UI.Slots {
		for _, item := range slot.Items {
			if item.Type == "route" {
				routes = append(routes, item)
			}
		}
	}
	return routes
}

// GetSidebarItems extracts sidebar menu items from the manifest
func (pm *PluginManifest) GetSidebarItems() []UISlot {
	var sidebarSlots []UISlot
	if pm.UI == nil {
		return sidebarSlots
	}

	for _, slot := range pm.UI.Slots {
		if slot.Slot == "sidebar.section" {
			sidebarSlots = append(sidebarSlots, slot)
		}
	}
	return sidebarSlots
}

// GetPortalRoutes extracts all routes defined in the portal manifest section
func (pm *PluginManifest) GetPortalRoutes() []UISlotItem {
	var routes []UISlotItem
	if pm.Portal == nil {
		return routes
	}

	for _, slot := range pm.Portal.Slots {
		for _, item := range slot.Items {
			if item.Type == "route" {
				routes = append(routes, item)
			}
		}
	}
	return routes
}

// GetPortalSidebarItems extracts portal sidebar menu items from the manifest
func (pm *PluginManifest) GetPortalSidebarItems() []PortalUISlot {
	var sidebarSlots []PortalUISlot
	if pm.Portal == nil {
		return sidebarSlots
	}

	for _, slot := range pm.Portal.Slots {
		if slot.Slot == "portal_sidebar.section" {
			sidebarSlots = append(sidebarSlots, slot)
		}
	}
	return sidebarSlots
}

// HasPermission checks if the manifest declares a specific permission
func (pm *PluginManifest) HasPermission(permType, permission string) bool {
	switch permType {
	case "kv":
		for _, perm := range pm.Permissions.KV {
			if perm == permission {
				return true
			}
		}
	case "rpc":
		for _, perm := range pm.Permissions.RPC {
			if perm == permission {
				return true
			}
		}
	case "ui":
		for _, perm := range pm.Permissions.UI {
			if perm == permission {
				return true
			}
		}
	case "portal_ui":
		for _, perm := range pm.Permissions.PortalUI {
			if perm == permission {
				return true
			}
		}
	case "services":
		for _, perm := range pm.Permissions.Services {
			if perm == permission {
				return true
			}
		}
	case "object_hooks":
		for _, perm := range pm.Permissions.ObjectHooks {
			if perm == permission {
				return true
			}
		}
	}
	return false
}

// isAnalyticsServiceScope checks if a service scope is analytics-related
func isAnalyticsServiceScope(scope string) bool {
	analyticsScopes := []string{
		"analytics.read",
		"analytics.detailed",
		"analytics.reports",
	}

	for _, analyticsScope := range analyticsScopes {
		if scope == analyticsScope {
			return true
		}
	}
	return false
}

// GetServiceScopes returns all service scopes declared in the manifest
func (pm *PluginManifest) GetServiceScopes() []string {
	return pm.Permissions.Services
}

// HasServiceScope checks if the manifest declares a specific service scope
func (pm *PluginManifest) HasServiceScope(scope string) bool {
	return pm.HasPermission("services", scope)
}

// GetObjectHooks returns all object hook permissions declared in the manifest
func (pm *PluginManifest) GetObjectHooks() []string {
	return pm.Permissions.ObjectHooks
}

// GetAllPermissionScopes returns all permission scopes (services + object_hooks) for approval workflow
func (pm *PluginManifest) GetAllPermissionScopes() []string {
	scopes := make([]string, 0, len(pm.Permissions.Services)+len(pm.Permissions.ObjectHooks))
	scopes = append(scopes, pm.Permissions.Services...)
	scopes = append(scopes, pm.Permissions.ObjectHooks...)
	return scopes
}

// TableName returns table name for RegisteredPlugin
func (RegisteredPlugin) TableName() string {
	return "registered_plugins"
}

// TableName returns table name for UIRegistry
func (UIRegistry) TableName() string {
	return "ui_registry"
}