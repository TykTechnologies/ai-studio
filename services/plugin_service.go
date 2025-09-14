package services

import (
	"fmt"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/models"
	"gorm.io/gorm"
)

// PluginService implements plugin management for AI Studio
type PluginService struct {
	db *gorm.DB
}

// NewPluginService creates a new plugin service
func NewPluginService(db *gorm.DB) *PluginService {
	return &PluginService{
		db: db,
	}
}

// Plugin request/response structures (adapted from microgateway)
type CreatePluginRequest struct {
	Name        string                 `json:"name" binding:"required"`
	Slug        string                 `json:"slug" binding:"required"`
	Description string                 `json:"description"`
	Command     string                 `json:"command" binding:"required"`
	Checksum    string                 `json:"checksum"` // Optional
	Config      map[string]interface{} `json:"config"`
	HookType    string                 `json:"hook_type" binding:"required"`
	IsActive    bool                   `json:"is_active"`
	Namespace   string                 `json:"namespace,omitempty"`
}

type UpdatePluginRequest struct {
	Name        *string                `json:"name"`
	Description *string                `json:"description"`
	Command     *string                `json:"command"`
	Checksum    *string                `json:"checksum"`
	Config      map[string]interface{} `json:"config"`
	HookType    *string                `json:"hook_type"`
	IsActive    *bool                  `json:"is_active"`
	Namespace   *string                `json:"namespace"`
}

// PluginServiceInterface defines the interface for plugin operations (adapted from microgateway)
type PluginServiceInterface interface {
	// CRUD operations
	CreatePlugin(req *CreatePluginRequest) (*models.Plugin, error)
	GetPlugin(id uint) (*models.Plugin, error)
	ListPlugins(page, limit int, hookType string, isActive bool) ([]models.Plugin, int64, error)
	ListAllPlugins(page, limit int, hookType string) ([]models.Plugin, int64, error)
	UpdatePlugin(id uint, req *UpdatePluginRequest) (*models.Plugin, error)
	DeletePlugin(id uint) error
	
	// LLM associations
	GetPluginsForLLM(llmID uint) ([]models.Plugin, error)
	UpdateLLMPlugins(llmID uint, pluginIDs []uint) error
	GetLLMPluginConfig(llmID, pluginID uint) (map[string]interface{}, error)
	
	// Validation and utilities
	TestPlugin(pluginID uint, testData interface{}) (interface{}, error)
	PluginSlugExists(slug string) (bool, error)
}

// CreatePlugin creates a new plugin (adapted from microgateway)
func (s *PluginService) CreatePlugin(req *CreatePluginRequest) (*models.Plugin, error) {
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

	plugin := &models.Plugin{
		Name:        req.Name,
		Slug:        req.Slug,
		Description: req.Description,
		Command:     req.Command,
		Checksum:    req.Checksum,
		Config:      req.Config,
		HookType:    req.HookType,
		IsActive:    req.IsActive,
		Namespace:   req.Namespace,
	}

	if err := plugin.Create(s.db); err != nil {
		return nil, fmt.Errorf("failed to create plugin: %w", err)
	}

	return plugin, nil
}

// GetPlugin retrieves a plugin by ID (adapted from microgateway)
func (s *PluginService) GetPlugin(id uint) (*models.Plugin, error) {
	plugin := models.NewPlugin()
	if err := plugin.Get(s.db, id); err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("plugin not found: %d", id)
		}
		return nil, fmt.Errorf("failed to get plugin: %w", err)
	}

	return plugin, nil
}

// ListPlugins lists plugins with pagination and filtering (adapted from microgateway)
func (s *PluginService) ListPlugins(page, limit int, hookType string, isActive bool, namespace string) ([]models.Plugin, int64, error) {
	var plugins models.Plugins
	totalCount, _, err := plugins.ListWithPagination(s.db, limit, page, false, hookType, isActive, namespace)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list plugins: %w", err)
	}

	return []models.Plugin(plugins), totalCount, nil
}

// ListAllPlugins lists all plugins (both active and inactive) with pagination and filtering
func (s *PluginService) ListAllPlugins(page, limit int, hookType string, namespace string) ([]models.Plugin, int64, error) {
	var plugins models.Plugins
	totalCount, _, err := plugins.ListAllWithPagination(s.db, limit, page, false, hookType, namespace)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list all plugins: %w", err)
	}

	return []models.Plugin(plugins), totalCount, nil
}

// UpdatePlugin updates an existing plugin (adapted from microgateway)
func (s *PluginService) UpdatePlugin(id uint, req *UpdatePluginRequest) (*models.Plugin, error) {
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
		plugin.Config = req.Config
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
	if req.Namespace != nil {
		plugin.Namespace = *req.Namespace
	}

	if err := plugin.Update(s.db); err != nil {
		return nil, fmt.Errorf("failed to update plugin: %w", err)
	}

	return plugin, nil
}

// DeletePlugin soft deletes a plugin (adapted from microgateway)
func (s *PluginService) DeletePlugin(id uint) error {
	plugin, err := s.GetPlugin(id)
	if err != nil {
		return err
	}

	// Remove all LLM associations first
	if err := models.DeleteAssociationsForPlugin(s.db, id); err != nil {
		return fmt.Errorf("failed to remove plugin associations: %w", err)
	}

	if err := plugin.Delete(s.db); err != nil {
		return fmt.Errorf("failed to delete plugin: %w", err)
	}

	return nil
}

// GetPluginsForLLM returns plugins associated with an LLM, ordered by execution order (adapted from microgateway)
func (s *PluginService) GetPluginsForLLM(llmID uint) ([]models.Plugin, error) {
	var llmPlugins models.LLMPlugins
	if err := llmPlugins.GetPluginsForLLM(s.db, llmID); err != nil {
		return nil, fmt.Errorf("failed to get plugins for LLM: %w", err)
	}

	// Extract plugin objects from associations
	plugins := make([]models.Plugin, len(llmPlugins))
	for i, lp := range llmPlugins {
		if lp.Plugin != nil {
			plugins[i] = *lp.Plugin
		}
	}

	return plugins, nil
}

// UpdateLLMPlugins updates plugin associations for an LLM (adapted from microgateway)
func (s *PluginService) UpdateLLMPlugins(llmID uint, pluginIDs []uint) error {
	return models.UpdatePluginOrder(s.db, llmID, pluginIDs)
}

// GetLLMPluginConfig returns the configuration for a specific plugin-LLM association (adapted from microgateway)
func (s *PluginService) GetLLMPluginConfig(llmID, pluginID uint) (map[string]interface{}, error) {
	var llmPlugin models.LLMPlugin
	if err := llmPlugin.Get(s.db, llmID, pluginID); err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("plugin-LLM association not found")
		}
		return nil, fmt.Errorf("failed to get plugin-LLM config: %w", err)
	}

	return llmPlugin.ConfigOverride, nil
}

// TestPlugin tests a plugin with provided test data (simplified from microgateway)
func (s *PluginService) TestPlugin(pluginID uint, testData interface{}) (interface{}, error) {
	plugin, err := s.GetPlugin(pluginID)
	if err != nil {
		return nil, err
	}

	if !plugin.IsActive {
		return nil, fmt.Errorf("plugin is not active")
	}

	// Create a test result structure (simplified - no binary execution)
	testResult := map[string]interface{}{
		"plugin_id":   pluginID,
		"plugin_name": plugin.Name,
		"plugin_slug": plugin.Slug,
		"hook_type":   plugin.HookType,
		"status":      "passed",
		"message":     "Plugin configuration validation completed successfully",
		"details": map[string]interface{}{
			"command_valid":     plugin.Command != "",
			"hook_type_valid":   plugin.IsValidHookType(),
			"config_keys":       len(plugin.Config),
			"has_checksum":      plugin.Checksum != "",
			"namespace":         plugin.Namespace,
			"tests_run": []string{
				"command_validation",
				"hook_type_validation",
				"config_parsing",
			},
		},
	}

	// Validate hook type
	if !plugin.IsValidHookType() {
		testResult["status"] = "failed"
		testResult["message"] = fmt.Sprintf("Invalid hook type: %s", plugin.HookType)
		return testResult, nil
	}

	// Validate command is present
	if plugin.Command == "" {
		testResult["status"] = "failed"
		testResult["message"] = "Plugin command is empty"
		return testResult, nil
	}

	return testResult, nil
}

// PluginSlugExists checks if a plugin slug already exists (adapted from microgateway)
func (s *PluginService) PluginSlugExists(slug string) (bool, error) {
	var count int64
	err := s.db.Model(&models.Plugin{}).Where("slug = ?", slug).Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("failed to check plugin slug existence: %w", err)
	}
	return count > 0, nil
}

// GetPluginsInNamespace returns plugins in a specific namespace (AI Studio specific)
func (s *PluginService) GetPluginsInNamespace(namespace string) ([]models.Plugin, error) {
	var plugins models.Plugins
	if err := plugins.GetPluginsInNamespace(s.db, namespace); err != nil {
		return nil, fmt.Errorf("failed to get plugins in namespace: %w", err)
	}
	return []models.Plugin(plugins), nil
}

// GetActivePluginsInNamespace returns active plugins in a specific namespace (AI Studio specific)
func (s *PluginService) GetActivePluginsInNamespace(namespace string) ([]models.Plugin, error) {
	var plugins []models.Plugin
	
	query := s.db.Where("is_active = ?", true)
	if namespace == "" {
		// Global namespace - only global plugins
		query = query.Where("namespace = ''")
	} else {
		// Specific namespace - global + matching namespace
		query = query.Where("(namespace = '' OR namespace = ?)", namespace)
	}
	
	if err := query.Order("created_at DESC").Find(&plugins).Error; err != nil {
		return nil, fmt.Errorf("failed to get active plugins in namespace: %w", err)
	}

	return plugins, nil
}

// isValidHookType validates hook type values (copied from microgateway)
func isValidHookType(hookType string) bool {
	validTypes := []string{
		models.HookTypePreAuth,
		models.HookTypeAuth,
		models.HookTypePostAuth,
		models.HookTypeOnResponse,
		models.HookTypeDataCollection,
	}
	for _, validType := range validTypes {
		if hookType == validType {
			return true
		}
	}
	return false
}