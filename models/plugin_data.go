package models

import (
	"time"

	"gorm.io/gorm"
)

// PluginData represents key-value data storage for AI Studio plugins
// Each plugin gets its own sandboxed namespace for storing configuration
// and state data that persists beyond the plugin's config field
type PluginData struct {
	ID         uint           `json:"id" gorm:"primaryKey"`
	PluginID   uint           `json:"plugin_id" gorm:"not null;index:idx_plugin_data_plugin_id;uniqueIndex:idx_plugin_data_composite"`
	PluginName string         `json:"plugin_name" gorm:"not null;size:255;index:idx_plugin_data_plugin_name"`
	DataKey    string         `json:"data_key" gorm:"not null;size:255;uniqueIndex:idx_plugin_data_composite"`
	DataValue  []byte         `json:"data_value" gorm:"type:bytea"` // Binary data support for any serialization format
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	DeletedAt  gorm.DeletedAt `json:"deleted_at,omitempty" gorm:"index"`

	// Relationship - CASCADE delete ensures cleanup when plugin is deleted
	Plugin Plugin `json:"-" gorm:"foreignKey:PluginID;constraint:OnDelete:CASCADE"`
}

// TableName returns the table name for the PluginData model
func (PluginData) TableName() string {
	return "plugin_data"
}

// NewPluginData creates a new PluginData instance
func NewPluginData() *PluginData {
	return &PluginData{}
}

// Create creates a new plugin data entry
func (pd *PluginData) Create(db *gorm.DB) error {
	return db.Create(pd).Error
}

// Get retrieves a plugin data entry by ID
func (pd *PluginData) Get(db *gorm.DB, id uint) error {
	return db.First(pd, id).Error
}

// GetByKey retrieves a plugin data entry by plugin ID and key
func (pd *PluginData) GetByKey(db *gorm.DB, pluginID uint, key string) error {
	return db.Where("plugin_id = ? AND data_key = ?", pluginID, key).First(pd).Error
}

// Update updates an existing plugin data entry
func (pd *PluginData) Update(db *gorm.DB) error {
	return db.Save(pd).Error
}

// Delete soft deletes a plugin data entry
func (pd *PluginData) Delete(db *gorm.DB) error {
	return db.Delete(pd).Error
}

// Upsert creates or updates a plugin data entry
// Returns true if created, false if updated
func (pd *PluginData) Upsert(db *gorm.DB) (bool, error) {
	existing := &PluginData{}
	err := existing.GetByKey(db, pd.PluginID, pd.DataKey)

	if err == gorm.ErrRecordNotFound {
		// Create new entry
		if err := pd.Create(db); err != nil {
			return false, err
		}
		return true, nil
	}

	if err != nil {
		return false, err
	}

	// Update existing entry
	existing.DataValue = pd.DataValue
	existing.PluginName = pd.PluginName
	if err := existing.Update(db); err != nil {
		return false, err
	}

	// Copy updated fields back
	pd.ID = existing.ID
	pd.CreatedAt = existing.CreatedAt
	pd.UpdatedAt = existing.UpdatedAt

	return false, nil
}

// PluginDataCollection represents a collection of plugin data entries
type PluginDataCollection []PluginData

// GetAllByPluginID retrieves all plugin data entries for a specific plugin
func (pdc *PluginDataCollection) GetAllByPluginID(db *gorm.DB, pluginID uint) error {
	return db.Where("plugin_id = ?", pluginID).Find(pdc).Error
}

// DeleteAllByPluginID deletes all plugin data entries for a specific plugin
func (pdc *PluginDataCollection) DeleteAllByPluginID(db *gorm.DB, pluginID uint) error {
	return db.Where("plugin_id = ?", pluginID).Delete(&PluginData{}).Error
}

// CountByPluginID returns the count of plugin data entries for a specific plugin
func CountPluginDataByPluginID(db *gorm.DB, pluginID uint) (int64, error) {
	var count int64
	err := db.Model(&PluginData{}).Where("plugin_id = ?", pluginID).Count(&count).Error
	return count, err
}