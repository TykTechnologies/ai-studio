package services

import (
	"fmt"

	"github.com/TykTechnologies/midsommar/v2/models"
	"gorm.io/gorm"
)

const (
	// MaxKVKeyLength is the maximum length for a plugin data key
	MaxKVKeyLength = 255
	// MaxKVValueSize is the maximum size for a plugin data value (5MB)
	MaxKVValueSize = 5 * 1024 * 1024
)

// PluginKVService implements key-value storage operations for AI Studio plugins
type PluginKVService struct {
	db *gorm.DB
}

// NewPluginKVService creates a new plugin KV service
func NewPluginKVService(db *gorm.DB) *PluginKVService {
	return &PluginKVService{
		db: db,
	}
}

// WriteKV creates or updates a key-value entry for a plugin
// Returns true if a new entry was created, false if an existing entry was updated
func (s *PluginKVService) WriteKV(pluginID uint, key string, value []byte) (bool, error) {
	// Validate inputs
	if pluginID == 0 {
		return false, fmt.Errorf("plugin ID cannot be zero")
	}

	if key == "" {
		return false, fmt.Errorf("key cannot be empty")
	}

	if len(key) > MaxKVKeyLength {
		return false, fmt.Errorf("key length exceeds maximum of %d characters", MaxKVKeyLength)
	}

	if len(value) > MaxKVValueSize {
		return false, fmt.Errorf("value size exceeds maximum of %d bytes", MaxKVValueSize)
	}

	// Get plugin to verify it exists and get plugin name
	var plugin models.Plugin
	if err := s.db.First(&plugin, pluginID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return false, fmt.Errorf("plugin not found: %d", pluginID)
		}
		return false, fmt.Errorf("failed to get plugin: %w", err)
	}

	// Create plugin data entry
	pluginData := &models.PluginData{
		PluginID:   pluginID,
		PluginName: plugin.Name,
		DataKey:    key,
		DataValue:  value,
	}

	// Upsert (create or update)
	created, err := pluginData.Upsert(s.db)
	if err != nil {
		return false, fmt.Errorf("failed to write KV data: %w", err)
	}

	return created, nil
}

// ReadKV retrieves a value by key for a specific plugin
func (s *PluginKVService) ReadKV(pluginID uint, key string) ([]byte, error) {
	// Validate inputs
	if pluginID == 0 {
		return nil, fmt.Errorf("plugin ID cannot be zero")
	}

	if key == "" {
		return nil, fmt.Errorf("key cannot be empty")
	}

	// Get plugin data
	pluginData := &models.PluginData{}
	err := pluginData.GetByKey(s.db, pluginID, key)

	if err == gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("key not found: %s", key)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to read KV data: %w", err)
	}

	return pluginData.DataValue, nil
}

// DeleteKV deletes a key-value entry for a plugin
// Returns true if the key existed and was deleted, false if the key didn't exist
func (s *PluginKVService) DeleteKV(pluginID uint, key string) (bool, error) {
	// Validate inputs
	if pluginID == 0 {
		return false, fmt.Errorf("plugin ID cannot be zero")
	}

	if key == "" {
		return false, fmt.Errorf("key cannot be empty")
	}

	// Get plugin data to check if it exists
	pluginData := &models.PluginData{}
	err := pluginData.GetByKey(s.db, pluginID, key)

	if err == gorm.ErrRecordNotFound {
		// Key doesn't exist
		return false, nil
	}

	if err != nil {
		return false, fmt.Errorf("failed to check KV data existence: %w", err)
	}

	// Delete the entry
	if err := pluginData.Delete(s.db); err != nil {
		return false, fmt.Errorf("failed to delete KV data: %w", err)
	}

	return true, nil
}

// ClearAllPluginData deletes all key-value entries for a specific plugin
func (s *PluginKVService) ClearAllPluginData(pluginID uint) error {
	// Validate input
	if pluginID == 0 {
		return fmt.Errorf("plugin ID cannot be zero")
	}

	// Verify plugin exists
	var plugin models.Plugin
	if err := s.db.First(&plugin, pluginID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return fmt.Errorf("plugin not found: %d", pluginID)
		}
		return fmt.Errorf("failed to get plugin: %w", err)
	}

	// Delete all plugin data
	var collection models.PluginDataCollection
	if err := collection.DeleteAllByPluginID(s.db, pluginID); err != nil {
		return fmt.Errorf("failed to clear plugin data: %w", err)
	}

	return nil
}

// CountPluginData returns the count of key-value entries for a specific plugin
func (s *PluginKVService) CountPluginData(pluginID uint) (int64, error) {
	if pluginID == 0 {
		return 0, fmt.Errorf("plugin ID cannot be zero")
	}

	count, err := models.CountPluginDataByPluginID(s.db, pluginID)
	if err != nil {
		return 0, fmt.Errorf("failed to count plugin data: %w", err)
	}

	return count, nil
}

// ListPluginKeys returns all keys for a specific plugin
func (s *PluginKVService) ListPluginKeys(pluginID uint) ([]string, error) {
	if pluginID == 0 {
		return nil, fmt.Errorf("plugin ID cannot be zero")
	}

	var collection models.PluginDataCollection
	if err := collection.GetAllByPluginID(s.db, pluginID); err != nil {
		return nil, fmt.Errorf("failed to list plugin keys: %w", err)
	}

	keys := make([]string, len(collection))
	for i, data := range collection {
		keys[i] = data.DataKey
	}

	return keys, nil
}