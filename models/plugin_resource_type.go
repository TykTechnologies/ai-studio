package models

import (
	"gorm.io/gorm"
)

// PluginResourceType represents a resource type registered by a plugin.
// Plugins declare resource types via the ResourceProvider capability.
// Each type can appear in the App creation form and participate in privacy validation.
type PluginResourceType struct {
	gorm.Model
	ID                  uint   `json:"id" gorm:"primaryKey"`
	PluginID            uint   `json:"plugin_id" gorm:"uniqueIndex:idx_prt_plugin_slug"`
	Slug                string `json:"slug" gorm:"size:100;uniqueIndex:idx_prt_plugin_slug"`
	Name                string `json:"name" gorm:"size:255"`
	Description         string `json:"description"`
	Icon                string `json:"icon" gorm:"size:500"`
	HasPrivacyScore     bool   `json:"has_privacy_score" gorm:"default:false"`
	SupportsSubmissions bool   `json:"supports_submissions" gorm:"default:false"`
	SupportsMetadata    bool   `json:"supports_metadata" gorm:"default:false"` // Instances can carry governed metadata (Enterprise)
	FormComponentTag    string `json:"form_component_tag" gorm:"size:100"`
	FormComponentEntry  string `json:"form_component_entry" gorm:"size:500"`
	SubmissionSchema    string `json:"submission_schema" gorm:"type:text"` // JSON Schema (object) for community submissions; empty = free-form
	IsActive            bool   `json:"is_active" gorm:"default:true"`

	// AccessGrantedViaApp is the resolved answer to "does an App credential
	// grant access to instances of this type?". Only such types are offered
	// in the App forms, show "Build app" in the portal catalog and travel in
	// the gateway config snapshot. Resolved at registration time by
	// ResolveAccessGrantedViaApp from the declared value and the plugin's
	// hook types, so read paths never need the Plugin loaded.
	AccessGrantedViaApp bool `json:"access_granted_via_app" gorm:"default:false"`
	// AccessGrantedViaAppDeclared is what the plugin declared (nil = it left
	// the platform to decide). Kept so a later registration can re-resolve.
	AccessGrantedViaAppDeclared *bool `json:"-" gorm:"column:access_granted_via_app_declared"`
	// PortalDetailPath is a same-origin path template to an instance's portal
	// page; "{id}" is replaced with the escaped instance ID.
	PortalDetailPath string `json:"portal_detail_path" gorm:"size:500"`

	// Relationships
	Plugin *Plugin `json:"plugin,omitempty" gorm:"foreignKey:PluginID"`
}

// ResolveAccessGrantedViaApp returns the effective AccessGrantedViaApp for a
// resource type. An explicit declaration wins. Otherwise a plugin that serves
// gateway custom endpoints and provides resources (the shape of a plugin that
// proxies its resources and checks the App's bindings on each request) grants
// access through App credentials; any other plugin does not.
func ResolveAccessGrantedViaApp(declared *bool, plugin *Plugin) bool {
	if declared != nil {
		return *declared
	}
	if plugin == nil {
		return false
	}
	hooks := resolutionHookTypes(plugin)
	return hooks[HookTypeCustomEndpoint] && hooks[HookTypeResourceProvider]
}

// resolutionHookTypes is the set of hook types a plugin is known to declare:
// the row's primary and list, plus the stored manifest's capabilities when
// the row carries no list and the admin has not customised the hooks (a
// freshly registered plugin's row can lag behind its manifest).
func resolutionHookTypes(plugin *Plugin) map[string]bool {
	hooks := map[string]bool{}
	if plugin.HookType != "" {
		hooks[plugin.HookType] = true
	}
	for _, h := range plugin.HookTypes {
		hooks[h] = true
	}
	if len(plugin.HookTypes) > 0 || plugin.HookTypesCustomized || plugin.Manifest == nil {
		return hooks
	}
	caps, _ := plugin.Manifest["capabilities"].(map[string]interface{})
	if caps == nil {
		return hooks
	}
	if primary, _ := caps["primary_hook"].(string); primary != "" {
		hooks[primary] = true
	}
	if list, _ := caps["hooks"].([]interface{}); list != nil {
		for _, h := range list {
			if s, ok := h.(string); ok {
				hooks[s] = true
			}
		}
	}
	return hooks
}

// EffectiveInstanceAccessGrantedViaApp applies an optional per-instance
// override to the resolved type value.
func EffectiveInstanceAccessGrantedViaApp(typeValue bool, override *bool) bool {
	if override != nil {
		return *override
	}
	return typeValue
}

type PluginResourceTypes []PluginResourceType

func (PluginResourceType) TableName() string {
	return "plugin_resource_types"
}

func (p *PluginResourceType) Create(db *gorm.DB) error {
	return db.Create(p).Error
}

func (p *PluginResourceType) Get(db *gorm.DB, id uint) error {
	return db.Preload("Plugin").First(p, id).Error
}

func (p *PluginResourceType) Update(db *gorm.DB) error {
	return db.Save(p).Error
}

func (p *PluginResourceType) Delete(db *gorm.DB) error {
	return db.Delete(p).Error
}

// GetByPluginAndSlug finds a resource type by plugin ID and slug
func (p *PluginResourceType) GetByPluginAndSlug(db *gorm.DB, pluginID uint, slug string) error {
	return db.Where("plugin_id = ? AND slug = ?", pluginID, slug).First(p).Error
}

// DeactivateOrphanedPluginResourceTypes retires active resource types whose
// plugin no longer exists (deleted while not loaded, before DeletePlugin
// retired them). Returns the number of types deactivated.
func DeactivateOrphanedPluginResourceTypes(db *gorm.DB) (int64, error) {
	res := db.Model(&PluginResourceType{}).
		Where("is_active = ? AND plugin_id NOT IN (?)", true,
			db.Model(&Plugin{}).Select("id")).
		Update("is_active", false)
	return res.RowsAffected, res.Error
}

// GetAllActive returns all active plugin resource types
func (pts *PluginResourceTypes) GetAllActive(db *gorm.DB) error {
	return db.Where("is_active = ?", true).Preload("Plugin").Find(pts).Error
}

// GetAllSubmittable returns the active resource types that accept community
// submissions, filtered in the database rather than in memory.
func (pts *PluginResourceTypes) GetAllSubmittable(db *gorm.DB) error {
	return db.Where("is_active = ? AND supports_submissions = ?", true, true).Preload("Plugin").Find(pts).Error
}

// GetByPlugin returns all resource types for a specific plugin
func (pts *PluginResourceTypes) GetByPlugin(db *gorm.DB, pluginID uint) error {
	return db.Where("plugin_id = ?", pluginID).Find(pts).Error
}
