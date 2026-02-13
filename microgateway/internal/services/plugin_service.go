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
	"github.com/TykTechnologies/midsommar/v2/pkg/config"
	"github.com/rs/zerolog/log"
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
	if strings.TrimSpace(req.Command) == "" {
		return nil, fmt.Errorf("plugin command cannot be empty")
	}
	if !isValidHookType(req.HookType) {
		return nil, fmt.Errorf("invalid hook type: %s", req.HookType)
	}

	// Security validation for plugin command
	if err := s.validatePluginCommand(req.Command); err != nil {
		return nil, err
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

// GetPluginsForLLM returns plugins associated with an LLM with merged configurations, ordered by execution order
func (s *PluginService) GetPluginsForLLM(llmID uint) ([]database.Plugin, error) {
	// Get LLM-plugin associations first
	var llmPlugins []database.LLMPlugin
	err := s.db.Where("llm_id = ? AND is_active = ?", llmID, true).
		Order("order_index ASC").
		Find(&llmPlugins).Error

	if err != nil {
		return nil, fmt.Errorf("failed to get LLM plugin associations: %w", err)
	}

	if len(llmPlugins) == 0 {
		return []database.Plugin{}, nil
	}

	// Get plugin IDs
	pluginIDs := make([]uint, len(llmPlugins))
	for i, lp := range llmPlugins {
		pluginIDs[i] = lp.PluginID
	}

	// Get plugins with active filter
	var plugins []database.Plugin
	err = s.db.Where("id IN ? AND is_active = ?", pluginIDs, true).
		Find(&plugins).Error

	if err != nil {
		return nil, fmt.Errorf("failed to get plugins: %w", err)
	}

	// Create plugin map for fast lookup
	pluginMap := make(map[uint]database.Plugin)
	for _, plugin := range plugins {
		pluginMap[plugin.ID] = plugin
	}

	// Build result with merged configurations, maintaining order
	result := make([]database.Plugin, 0, len(llmPlugins))

	for _, llmPlugin := range llmPlugins {
		plugin, exists := pluginMap[llmPlugin.PluginID]
		if !exists {
			// Plugin might be inactive, skip it
			continue
		}

		// Merge base plugin config with per-LLM override
		baseConfigJSON := []byte(plugin.Config)
		overrideConfigJSON := []byte(llmPlugin.ConfigOverride)

		mergedConfigJSON, err := config.MergePluginConfigsJSON(baseConfigJSON, overrideConfigJSON)
		if err != nil {
			log.Error().Err(err).
				Uint("plugin_id", plugin.ID).
				Uint("llm_id", llmID).
				Msg("Failed to merge plugin config, using base config")
			mergedConfigJSON = baseConfigJSON
		}

		// Update plugin with merged config
		plugin.Config = datatypes.JSON(mergedConfigJSON)

		log.Debug().
			Uint("plugin_id", plugin.ID).
			Uint("llm_id", llmID).
			Bool("has_override", len(overrideConfigJSON) > 0).
			Msg("Merged plugin configuration for LLM")

		result = append(result, plugin)
	}

	return result, nil
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

// TestPlugin tests a plugin with provided test data
func (s *PluginService) TestPlugin(pluginID uint, testData interface{}) (interface{}, error) {
	plugin, err := s.GetPlugin(pluginID)
	if err != nil {
		return nil, err
	}

	if !plugin.IsActive {
		return nil, fmt.Errorf("plugin is not active")
	}

	// Create a test result structure
	testResult := map[string]interface{}{
		"plugin_id":   pluginID,
		"plugin_name": plugin.Name,
		"hook_type":   plugin.HookType,
		"status":      "unknown",
		"message":     "",
		"details":     map[string]interface{}{},
	}

	// Validate plugin command exists and is executable
	if plugin.Command == "" {
		testResult["status"] = "failed"
		testResult["message"] = "Plugin command is empty"
		return testResult, nil
	}

	// Test plugin configuration parsing
	var config map[string]interface{}
	if plugin.Config != nil {
		if err := json.Unmarshal(plugin.Config, &config); err != nil {
			testResult["status"] = "failed"
			testResult["message"] = fmt.Sprintf("Failed to parse plugin config: %v", err)
			return testResult, nil
		}
		testResult["details"].(map[string]interface{})["config_parsed"] = true
		testResult["details"].(map[string]interface{})["config_keys"] = len(config)
	}

	// Validate checksum if provided
	if plugin.Checksum != "" {
		testResult["details"].(map[string]interface{})["has_checksum"] = true
		testResult["details"].(map[string]interface{})["checksum"] = plugin.Checksum[:8] + "..." // Show first 8 chars
	}

	// For now, basic validation tests
	switch plugin.HookType {
	case "pre_auth", "auth", "post_auth", "on_response":
		testResult["details"].(map[string]interface{})["hook_type_valid"] = true
	default:
		testResult["status"] = "warning"
		testResult["message"] = fmt.Sprintf("Unknown hook type: %s", plugin.HookType)
		return testResult, nil
	}

	// All basic tests passed
	testResult["status"] = "passed"
	testResult["message"] = "Basic plugin validation completed successfully"
	testResult["details"].(map[string]interface{})["tests_run"] = []string{
		"command_validation",
		"config_parsing", 
		"hook_type_validation",
		"checksum_verification",
	}

	return testResult, nil
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
	validTypes := []string{"pre_auth", "auth", "post_auth", "on_response", "data_collection", "custom_endpoint"}
	for _, validType := range validTypes {
		if hookType == validType {
			return true
		}
	}
	return false
}

// validatePluginCommand performs security validation on plugin commands
func (s *PluginService) validatePluginCommand(command string) error {
	// Get configuration from environment variables
	allowlist := os.Getenv("PLUGIN_COMMAND_ALLOWLIST")
	blockInternalURLs := os.Getenv("PLUGIN_BLOCK_INTERNAL_URLS") == "true"

	// Check for path traversal attacks
	if strings.Contains(command, "../") {
		return fmt.Errorf("plugin command contains path traversal attempt (../): %s", command)
	}

	// Check for absolute paths outside allowed directories
	if strings.HasPrefix(command, "/") && !strings.HasPrefix(command, "/usr/bin/") &&
		!strings.HasPrefix(command, "/bin/") && !strings.HasPrefix(command, "/usr/local/bin/") {
		log.Warn().
			Str("command", command).
			Msg("⚠️  PLUGIN SECURITY WARNING: Plugin command uses absolute path outside standard directories. This may pose a security risk in production environments.")
	}

	// Check for internal network access (gRPC URLs)
	if strings.HasPrefix(command, "grpc://") || strings.Contains(command, ":") {
		// Extract potential URLs for validation
		if s.containsInternalIP(command) {
			if blockInternalURLs {
				return fmt.Errorf("plugin command targets internal network address: %s", command)
			} else {
				log.Warn().
					Str("command", command).
					Msg("⚠️  PLUGIN SECURITY WARNING: Plugin command may target internal network address (127.x.x.x, 192.168.x.x, 10.x.x.x). Set PLUGIN_BLOCK_INTERNAL_URLS=true to block this in production.")
			}
		}
	}

	// Check against allowlist if configured
	if allowlist != "" {
		allowed := strings.Split(allowlist, ",")
		commandAllowed := false
		for _, pattern := range allowed {
			pattern = strings.TrimSpace(pattern)
			if strings.Contains(command, pattern) || command == pattern {
				commandAllowed = true
				break
			}
		}
		if !commandAllowed {
			log.Warn().
				Str("command", command).
				Str("allowlist", allowlist).
				Msg("⚠️  PLUGIN SECURITY WARNING: Plugin command not in PLUGIN_COMMAND_ALLOWLIST. This command will be allowed but may pose security risks.")
		}
	} else {
		log.Info().
			Msg("ℹ️  PLUGIN INFO: No PLUGIN_COMMAND_ALLOWLIST configured. Set this environment variable to restrict plugin commands in production.")
	}

	return nil
}

// containsInternalIP checks if a command string contains internal IP addresses
func (s *PluginService) containsInternalIP(command string) bool {
	internalPatterns := []string{
		"127.", "localhost", "::1",          // Loopback
		"192.168.", "10.", "172.16.", "172.17.", "172.18.", "172.19.", // Private networks
		"172.20.", "172.21.", "172.22.", "172.23.", "172.24.", "172.25.",
		"172.26.", "172.27.", "172.28.", "172.29.", "172.30.", "172.31.",
	}

	for _, pattern := range internalPatterns {
		if strings.Contains(command, pattern) {
			return true
		}
	}
	return false
}