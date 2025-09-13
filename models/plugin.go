package models

import (
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
	HookTypePreAuth        = "pre_auth"
	HookTypeAuth           = "auth"
	HookTypePostAuth       = "post_auth"
	HookTypeOnResponse     = "on_response"
	HookTypeDataCollection = "data_collection"
)

// NewPlugin creates a new Plugin instance
func NewPlugin() *Plugin {
	return &Plugin{
		IsActive: true,
		Config:   make(map[string]interface{}),
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
	}
	
	for _, validType := range validTypes {
		if p.HookType == validType {
			return true
		}
	}
	return false
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
	if namespace == "" {
		// Global namespace - only global plugins
		query = query.Where("namespace = ''")
	} else {
		// Specific namespace - global + matching namespace
		query = query.Where("(namespace = '' OR namespace = ?)", namespace)
	}
	
	return query.Order("created_at DESC").Find(plugins).Error
}

// ListWithPagination returns paginated list of plugins with filtering
func (plugins *Plugins) ListWithPagination(db *gorm.DB, pageSize, pageNumber int, all bool, hookType string, isActive bool) (int64, int, error) {
	var totalCount int64
	query := db.Model(&Plugin{})

	// Apply filters
	if hookType != "" {
		query = query.Where("hook_type = ?", hookType)
	}
	query = query.Where("is_active = ?", isActive)

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