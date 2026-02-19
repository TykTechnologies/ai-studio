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
	FormComponentTag    string `json:"form_component_tag" gorm:"size:100"`
	FormComponentEntry  string `json:"form_component_entry" gorm:"size:500"`
	IsActive            bool   `json:"is_active" gorm:"default:true"`

	// Relationships
	Plugin *Plugin `json:"plugin,omitempty" gorm:"foreignKey:PluginID"`
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

// GetAllActive returns all active plugin resource types
func (pts *PluginResourceTypes) GetAllActive(db *gorm.DB) error {
	return db.Where("is_active = ?", true).Preload("Plugin").Find(pts).Error
}

// GetByPlugin returns all resource types for a specific plugin
func (pts *PluginResourceTypes) GetByPlugin(db *gorm.DB, pluginID uint) error {
	return db.Where("plugin_id = ?", pluginID).Find(pts).Error
}
