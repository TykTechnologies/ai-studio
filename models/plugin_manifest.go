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

	// Permissions and security
	Permissions struct {
		KV       []string `json:"kv"`       // KV access permissions: read, write, list
		RPC      []string `json:"rpc"`      // RPC permissions: call
		Routes   []string `json:"routes"`   // Route patterns this plugin can register
		UI       []string `json:"ui"`       // UI permissions: sidebar.register, route.register
		Services []string `json:"services"` // AI Studio service access scopes: analytics.read, plugins.config, etc.
	} `json:"permissions"`

	// Key-value namespace for plugin data
	KVNamespace string `json:"kvNamespace"`

	// RPC configuration
	RPC *struct {
		BasePath   string `json:"basePath"`   // Base path for RPC endpoints
		Proto      string `json:"proto"`      // Path to proto file
		Entrypoint string `json:"entrypoint"` // gRPC service method name
	} `json:"rpc,omitempty"`

	// UI configuration
	UI *struct {
		Slots []UISlot `json:"slots"` // UI slots this plugin registers
	} `json:"ui,omitempty"`

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
	case "services":
		for _, perm := range pm.Permissions.Services {
			if perm == permission {
				return true
			}
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

// TableName returns table name for RegisteredPlugin
func (RegisteredPlugin) TableName() string {
	return "registered_plugins"
}

// TableName returns table name for UIRegistry
func (UIRegistry) TableName() string {
	return "ui_registry"
}