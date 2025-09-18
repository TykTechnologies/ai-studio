package models

import (
	"time"

	"gorm.io/gorm"
)

// LLMPlugin represents the many-to-many relationship between LLMs and plugins
type LLMPlugin struct {
	LLMID          uint                   `json:"llm_id" gorm:"primaryKey;index:idx_llm_plugins_llm_id"`
	PluginID       uint                   `json:"plugin_id" gorm:"primaryKey"`
	OrderIndex     int                    `json:"order_index" gorm:"default:0;index:idx_llm_plugins_order"`
	IsActive       bool                   `json:"is_active" gorm:"default:true"`
	ConfigOverride map[string]interface{} `json:"config_override" gorm:"serializer:json"`
	CreatedAt      time.Time              `json:"created_at"`
	UpdatedAt      time.Time              `json:"updated_at"`
	
	// Relationships
	LLM    *LLM    `json:"llm,omitempty" gorm:"foreignKey:LLMID"`
	Plugin *Plugin `json:"plugin,omitempty" gorm:"foreignKey:PluginID"`
}

type LLMPlugins []LLMPlugin

// NewLLMPlugin creates a new LLMPlugin association
func NewLLMPlugin() *LLMPlugin {
	return &LLMPlugin{
		IsActive:       true,
		ConfigOverride: make(map[string]interface{}),
	}
}

// Get retrieves an LLM-Plugin association by LLM ID and Plugin ID
func (lp *LLMPlugin) Get(db *gorm.DB, llmID, pluginID uint) error {
	return db.Preload("LLM").Preload("Plugin").Where("llm_id = ? AND plugin_id = ?", llmID, pluginID).First(lp).Error
}

// Create creates a new LLM-Plugin association
func (lp *LLMPlugin) Create(db *gorm.DB) error {
	return db.Create(lp).Error
}

// Update updates an existing LLM-Plugin association
func (lp *LLMPlugin) Update(db *gorm.DB) error {
	return db.Save(lp).Error
}

// Delete removes an LLM-Plugin association
func (lp *LLMPlugin) Delete(db *gorm.DB) error {
	return db.Where("llm_id = ? AND plugin_id = ?", lp.LLMID, lp.PluginID).Delete(lp).Error
}

// UpdateOrder updates the execution order for this association
func (lp *LLMPlugin) UpdateOrder(db *gorm.DB, newOrder int) error {
	lp.OrderIndex = newOrder
	return db.Model(lp).Where("llm_id = ? AND plugin_id = ?", lp.LLMID, lp.PluginID).Update("order_index", newOrder).Error
}

// UpdateConfig updates the configuration override for this association
func (lp *LLMPlugin) UpdateConfig(db *gorm.DB, config map[string]interface{}) error {
	lp.ConfigOverride = config
	return db.Model(lp).Where("llm_id = ? AND plugin_id = ?", lp.LLMID, lp.PluginID).Update("config_override", config).Error
}

// Activate activates this LLM-Plugin association
func (lp *LLMPlugin) Activate(db *gorm.DB) error {
	lp.IsActive = true
	return db.Model(lp).Where("llm_id = ? AND plugin_id = ?", lp.LLMID, lp.PluginID).Update("is_active", true).Error
}

// Deactivate deactivates this LLM-Plugin association
func (lp *LLMPlugin) Deactivate(db *gorm.DB) error {
	lp.IsActive = false
	return db.Model(lp).Where("llm_id = ? AND plugin_id = ?", lp.LLMID, lp.PluginID).Update("is_active", false).Error
}

// GetPluginsForLLM returns all active plugins for a specific LLM, ordered by execution order
func (llmPlugins *LLMPlugins) GetPluginsForLLM(db *gorm.DB, llmID uint) error {
	return db.Preload("Plugin").Where("llm_id = ? AND is_active = ?", llmID, true).
		Order("order_index ASC").Find(llmPlugins).Error
}

// GetLLMsForPlugin returns all LLMs associated with a specific plugin
func (llmPlugins *LLMPlugins) GetLLMsForPlugin(db *gorm.DB, pluginID uint) error {
	return db.Preload("LLM").Where("plugin_id = ? AND is_active = ?", pluginID, true).
		Order("order_index ASC").Find(llmPlugins).Error
}

// GetActiveAssociations returns all active LLM-Plugin associations
func (llmPlugins *LLMPlugins) GetActiveAssociations(db *gorm.DB) error {
	return db.Preload("LLM").Preload("Plugin").Where("is_active = ?", true).
		Order("llm_id ASC, order_index ASC").Find(llmPlugins).Error
}

// DeleteAssociationsForLLM removes all plugin associations for a specific LLM
func DeleteAssociationsForLLM(db *gorm.DB, llmID uint) error {
	return db.Where("llm_id = ?", llmID).Delete(&LLMPlugin{}).Error
}

// DeleteAssociationsForPlugin removes all LLM associations for a specific plugin
func DeleteAssociationsForPlugin(db *gorm.DB, pluginID uint) error {
	return db.Where("plugin_id = ?", pluginID).Delete(&LLMPlugin{}).Error
}

// UpdatePluginOrder updates the execution order for all plugins associated with an LLM
func UpdatePluginOrder(db *gorm.DB, llmID uint, orderedPluginIDs []uint) error {
	return db.Transaction(func(tx *gorm.DB) error {
		// Remove existing associations
		if err := tx.Where("llm_id = ?", llmID).Delete(&LLMPlugin{}).Error; err != nil {
			return err
		}

		// Add new associations with proper order
		for i, pluginID := range orderedPluginIDs {
			llmPlugin := LLMPlugin{
				LLMID:      llmID,
				PluginID:   pluginID,
				OrderIndex: i,
				IsActive:   true,
				CreatedAt:  time.Now(),
				UpdatedAt:  time.Now(),
			}
			if err := tx.Create(&llmPlugin).Error; err != nil {
				return err
			}
		}

		return nil
	})
}

// CountAssociationsForLLM returns the count of active plugin associations for an LLM
func CountAssociationsForLLM(db *gorm.DB, llmID uint) (int64, error) {
	var count int64
	err := db.Model(&LLMPlugin{}).Where("llm_id = ? AND is_active = ?", llmID, true).Count(&count).Error
	return count, err
}

// CountAssociationsForPlugin returns the count of LLM associations for a plugin
func CountAssociationsForPlugin(db *gorm.DB, pluginID uint) (int64, error) {
	var count int64
	err := db.Model(&LLMPlugin{}).Where("plugin_id = ? AND is_active = ?", pluginID, true).Count(&count).Error
	return count, err
}