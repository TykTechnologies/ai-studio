// internal/services/plugin_service.go
package services

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// PluginService implements plugin management
type PluginService struct {
	db   *gorm.DB
	repo *database.Repository
}

// NewPluginService creates a new plugin service
func NewPluginService(db *gorm.DB, repo *database.Repository) PluginServiceInterface {
	return &PluginService{
		db:   db,
		repo: repo,
	}
}

// CreatePlugin creates a new plugin
func (s *PluginService) CreatePlugin(req *CreatePluginRequest) (*database.Plugin, error) {
	// Validate required fields
	if strings.TrimSpace(req.Name) == "" {
		return nil, fmt.Errorf("plugin name cannot be empty")
	}
	if strings.TrimSpace(req.Slug) == "" {
		return nil, fmt.Errorf("plugin slug cannot be empty")
	}
	if strings.TrimSpace(req.Command) == "" {
		return nil, fmt.Errorf("plugin command cannot be empty")
	}
	if !isValidHookType(req.HookType) {
		return nil, fmt.Errorf("invalid hook type: %s", req.HookType)
	}

	// Check if slug already exists
	exists, err := s.PluginSlugExists(req.Slug)
	if err != nil {
		return nil, fmt.Errorf("failed to check plugin slug existence: %w", err)
	}
	if exists {
		return nil, fmt.Errorf("plugin slug '%s' already exists", req.Slug)
	}

	// Convert config to JSON
	var configJSON datatypes.JSON
	if req.Config != nil {
		configBytes, err := json.Marshal(req.Config)
		if err != nil {
			return nil, fmt.Errorf("invalid config format: %w", err)
		}
		configJSON = configBytes
	}

	plugin := &database.Plugin{
		Name:        req.Name,
		Slug:        req.Slug,
		Description: req.Description,
		Command:     req.Command,
		Checksum:    req.Checksum,
		Config:      configJSON,
		HookType:    req.HookType,
		IsActive:    req.IsActive,
	}

	if err := s.db.Create(plugin).Error; err != nil {
		return nil, fmt.Errorf("failed to create plugin: %w", err)
	}

	return plugin, nil
}

// GetPlugin retrieves a plugin by ID
func (s *PluginService) GetPlugin(id uint) (*database.Plugin, error) {
	var plugin database.Plugin
	err := s.db.Preload("LLMs").First(&plugin, id).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("plugin not found: %d", id)
		}
		return nil, fmt.Errorf("failed to get plugin: %w", err)
	}

	return &plugin, nil
}

// ListPlugins lists plugins with pagination and filtering
func (s *PluginService) ListPlugins(page, limit int, hookType string, isActive bool) ([]database.Plugin, int64, error) {
	var plugins []database.Plugin
	var total int64

	query := s.db.Model(&database.Plugin{})

	// Apply filters
	if hookType != "" {
		query = query.Where("hook_type = ?", hookType)
	}
	query = query.Where("is_active = ?", isActive)

	// Get total count
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count plugins: %w", err)
	}

	// Get paginated results
	offset := (page - 1) * limit
	err := query.Offset(offset).Limit(limit).
		Preload("LLMs").
		Order("created_at DESC").
		Find(&plugins).Error

	if err != nil {
		return nil, 0, fmt.Errorf("failed to list plugins: %w", err)
	}

	return plugins, total, nil
}

// UpdatePlugin updates an existing plugin
func (s *PluginService) UpdatePlugin(id uint, req *UpdatePluginRequest) (*database.Plugin, error) {
	// Get existing plugin
	plugin, err := s.GetPlugin(id)
	if err != nil {
		return nil, err
	}

	// Update fields
	if req.Name != nil {
		plugin.Name = *req.Name
	}
	if req.Description != nil {
		plugin.Description = *req.Description
	}
	if req.Command != nil {
		plugin.Command = *req.Command
	}
	if req.Checksum != nil {
		plugin.Checksum = *req.Checksum
	}
	if req.Config != nil {
		configBytes, err := json.Marshal(req.Config)
		if err != nil {
			return nil, fmt.Errorf("invalid config format: %w", err)
		}
		plugin.Config = configBytes
	}
	if req.HookType != nil {
		if !isValidHookType(*req.HookType) {
			return nil, fmt.Errorf("invalid hook type: %s", *req.HookType)
		}
		plugin.HookType = *req.HookType
	}
	if req.IsActive != nil {
		plugin.IsActive = *req.IsActive
	}

	// Save changes
	if err := s.db.Save(plugin).Error; err != nil {
		return nil, fmt.Errorf("failed to update plugin: %w", err)
	}

	return plugin, nil
}

// DeletePlugin soft deletes a plugin
func (s *PluginService) DeletePlugin(id uint) error {
	result := s.db.Delete(&database.Plugin{}, id)
	if result.Error != nil {
		return fmt.Errorf("failed to delete plugin: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("plugin not found: %d", id)
	}
	return nil
}

// GetPluginsForLLM returns plugins associated with an LLM, ordered by execution order
func (s *PluginService) GetPluginsForLLM(llmID uint) ([]database.Plugin, error) {
	var plugins []database.Plugin
	
	err := s.db.Joins("JOIN llm_plugins lp ON lp.plugin_id = plugins.id").
		Where("lp.llm_id = ? AND lp.is_active = ? AND plugins.is_active = ?", llmID, true, true).
		Order("lp.order_index ASC").
		Find(&plugins).Error

	if err != nil {
		return nil, fmt.Errorf("failed to get plugins for LLM: %w", err)
	}

	return plugins, nil
}

// UpdateLLMPlugins updates plugin associations for an LLM
func (s *PluginService) UpdateLLMPlugins(llmID uint, pluginIDs []uint) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		// Remove existing associations
		if err := tx.Where("llm_id = ?", llmID).Delete(&database.LLMPlugin{}).Error; err != nil {
			return fmt.Errorf("failed to remove existing plugin associations: %w", err)
		}

		// Add new associations
		for i, pluginID := range pluginIDs {
			llmPlugin := database.LLMPlugin{
				LLMID:      llmID,
				PluginID:   pluginID,
				IsActive:   true,
				OrderIndex: i, // Use array index as execution order
				CreatedAt:  time.Now(),
			}
			if err := tx.Create(&llmPlugin).Error; err != nil {
				return fmt.Errorf("failed to create LLM-plugin association: %w", err)
			}
		}

		return nil
	})
}

// GetLLMPluginConfig returns the configuration for a specific plugin-LLM association
func (s *PluginService) GetLLMPluginConfig(llmID, pluginID uint) (map[string]interface{}, error) {
	var llmPlugin database.LLMPlugin
	err := s.db.Where("llm_id = ? AND plugin_id = ?", llmID, pluginID).First(&llmPlugin).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("plugin-LLM association not found")
		}
		return nil, fmt.Errorf("failed to get plugin-LLM config: %w", err)
	}

	// Parse config override if it exists
	var config map[string]interface{}
	if llmPlugin.ConfigOverride != nil {
		if err := json.Unmarshal(llmPlugin.ConfigOverride, &config); err != nil {
			return nil, fmt.Errorf("failed to parse config override: %w", err)
		}
	}

	return config, nil
}

// ValidatePluginChecksum validates that the plugin file matches the stored checksum
func (s *PluginService) ValidatePluginChecksum(pluginID uint, filePath string) error {
	plugin, err := s.GetPlugin(pluginID)
	if err != nil {
		return err
	}

	if plugin.Checksum == "" {
		// No checksum stored, skip validation
		return nil
	}

	// Calculate file checksum
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open plugin file: %w", err)
	}
	defer file.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return fmt.Errorf("failed to calculate file checksum: %w", err)
	}

	calculatedChecksum := hex.EncodeToString(hasher.Sum(nil))
	if calculatedChecksum != plugin.Checksum {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", plugin.Checksum, calculatedChecksum)
	}

	return nil
}

// TestPlugin tests a plugin with provided test data (placeholder implementation)
func (s *PluginService) TestPlugin(pluginID uint, testData interface{}) (interface{}, error) {
	plugin, err := s.GetPlugin(pluginID)
	if err != nil {
		return nil, err
	}

	if !plugin.IsActive {
		return nil, fmt.Errorf("plugin is not active")
	}

	// TODO: Implement actual plugin testing using the plugin manager
	// This would involve loading the plugin and executing it with test data
	
	// For now, return a simple test result
	return map[string]interface{}{
		"plugin_id": pluginID,
		"status":    "test_passed",
		"message":   "Plugin test placeholder - not implemented yet",
	}, nil
}

// PluginSlugExists checks if a plugin slug already exists
func (s *PluginService) PluginSlugExists(slug string) (bool, error) {
	var count int64
	err := s.db.Model(&database.Plugin{}).Where("slug = ?", slug).Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("failed to check plugin slug existence: %w", err)
	}
	return count > 0, nil
}

// isValidHookType validates hook type values
func isValidHookType(hookType string) bool {
	validTypes := []string{"pre_auth", "auth", "post_auth", "on_response"}
	for _, validType := range validTypes {
		if hookType == validType {
			return true
		}
	}
	return false
}