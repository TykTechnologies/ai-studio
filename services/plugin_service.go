package services

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/ociplugins"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// PluginService implements plugin management for AI Studio
type PluginService struct {
	db               *gorm.DB
	ociClient        *ociplugins.OCIPluginClient
	pluginManager    *AIStudioPluginManager // For loading plugins to get config schemas
}

// NewPluginService creates a new plugin service
func NewPluginService(db *gorm.DB) *PluginService {
	return &PluginService{
		db: db,
	}
}

// SetPluginManager sets the AI Studio plugin manager (to avoid circular dependency)
func (s *PluginService) SetPluginManager(pluginManager *AIStudioPluginManager) {
	s.pluginManager = pluginManager
}

// NewPluginServiceWithOCI creates a new plugin service with OCI support
func NewPluginServiceWithOCI(db *gorm.DB, ociConfig *ociplugins.OCIConfig) (*PluginService, error) {
	var ociClient *ociplugins.OCIPluginClient
	var err error

	if ociConfig != nil {
		ociClient, err = ociplugins.NewOCIPluginClient(ociConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to create OCI plugin client: %w", err)
		}
	}

	return &PluginService{
		db:        db,
		ociClient: ociClient,
	}, nil
}

// Plugin request/response structures (adapted from microgateway)
type CreatePluginRequest struct {
	Name            string                 `json:"name" binding:"required"`
	Slug            string                 `json:"slug" binding:"required"`
	Description     string                 `json:"description"`
	Command         string                 `json:"command" binding:"required"`
	Checksum        string                 `json:"checksum"` // Optional
	Config          map[string]interface{} `json:"config"`
	HookType        string                 `json:"hook_type" binding:"required"`
	IsActive        bool                   `json:"is_active"`
	Namespace       string                 `json:"namespace,omitempty"`
	PluginType      string                 `json:"plugin_type,omitempty"`   // "gateway" or "ai_studio"
	OCIReference    string                 `json:"oci_reference,omitempty"` // OCI artifact reference
	LoadImmediately bool                   `json:"load_immediately,omitempty"` // Auto-load AI Studio plugins
}

type UpdatePluginRequest struct {
	Name            *string                `json:"name"`
	Description     *string                `json:"description"`
	Command         *string                `json:"command"`
	Checksum        *string                `json:"checksum"`
	Config          map[string]interface{} `json:"config"`
	HookType        *string                `json:"hook_type"`
	IsActive        *bool                  `json:"is_active"`
	Namespace       *string                `json:"namespace"`
	PluginType      *string                `json:"plugin_type"`
	OCIReference    *string                `json:"oci_reference"`
	LoadImmediately *bool                  `json:"load_immediately,omitempty"` // Auto-load AI Studio plugins
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

	// Security validation for plugin command
	if err := s.validatePluginCommand(req.Command); err != nil {
		return nil, err
	}

	// Check if slug already exists
	exists, err := s.PluginSlugExists(req.Slug)
	if err != nil {
		return nil, fmt.Errorf("failed to check plugin slug existence: %w", err)
	}
	if exists {
		return nil, fmt.Errorf("plugin slug '%s' already exists", req.Slug)
	}

	// Set default plugin type if not specified
	pluginType := req.PluginType
	if pluginType == "" {
		pluginType = models.PluginTypeGateway
	}

	// Set default hook type for AI Studio plugins
	hookType := req.HookType
	if pluginType == models.PluginTypeAIStudio && hookType == "" {
		hookType = models.HookTypeStudioUI
	}

	plugin := &models.Plugin{
		Name:         req.Name,
		Slug:         req.Slug,
		Description:  req.Description,
		Command:      req.Command,
		Checksum:     req.Checksum,
		Config:       req.Config,
		HookType:     hookType,
		IsActive:     req.IsActive,
		Namespace:    req.Namespace,
		PluginType:   pluginType,
		OCIReference: req.OCIReference,
		Manifest:     make(map[string]interface{}),
	}

	// Validate plugin type
	if !plugin.IsValidPluginType() {
		return nil, fmt.Errorf("invalid plugin type: %s", pluginType)
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
	if req.PluginType != nil {
		plugin.PluginType = *req.PluginType
		// Validate plugin type
		if !plugin.IsValidPluginType() {
			return nil, fmt.Errorf("invalid plugin type: %s", *req.PluginType)
		}
	}
	// IsOCI is now determined by command prefix, no need to set explicitly
	if req.OCIReference != nil {
		plugin.OCIReference = *req.OCIReference
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

// UpdateLLMPluginConfig updates the configuration override for a specific plugin-LLM association
func (s *PluginService) UpdateLLMPluginConfig(llmID, pluginID uint, configOverride map[string]interface{}) error {
	var llmPlugin models.LLMPlugin
	if err := llmPlugin.Get(s.db, llmID, pluginID); err != nil {
		if err == gorm.ErrRecordNotFound {
			return fmt.Errorf("plugin-LLM association not found")
		}
		return fmt.Errorf("failed to get plugin-LLM association: %w", err)
	}

	// Update the configuration override
	if err := llmPlugin.UpdateConfig(s.db, configOverride); err != nil {
		return fmt.Errorf("failed to update plugin-LLM config: %w", err)
	}

	log.Info().
		Uint("llm_id", llmID).
		Uint("plugin_id", pluginID).
		Int("config_keys", len(configOverride)).
		Msg("Updated LLM plugin configuration override")

	return nil
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
		models.HookTypeStudioUI,
	}
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

// OCI Plugin Management Methods

// CreateOCIPluginFromReference creates a plugin from an OCI reference
func (s *PluginService) CreateOCIPluginFromReference(req *CreateOCIPluginRequest) (*models.Plugin, error) {
	if s.ociClient == nil {
		return nil, fmt.Errorf("OCI client not configured")
	}

	// Parse OCI reference
	ref, params, err := ociplugins.ParseOCICommand(req.OCIReference)
	if err != nil {
		return nil, fmt.Errorf("failed to parse OCI reference: %w", err)
	}

	// Fetch plugin from registry to verify it exists
	ctx := context.Background()
	_, err = s.ociClient.FetchPlugin(ctx, ref, params)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch OCI plugin: %w", err)
	}

	// For MVP, we'll use a default manifest structure
	// In full implementation, this would extract manifest.json from the OCI artifact
	manifest := map[string]interface{}{
		"id":      fmt.Sprintf("plugin-%s", req.Slug),
		"version": "1.0.0",
		"name":    req.Name,
		"description": req.Description,
		// Manifest will be populated later when parsed from the OCI artifact
	}

	// Create plugin record
	plugin := &models.Plugin{
		Name:         req.Name,
		Slug:         req.Slug,
		Description:  req.Description,
		Command:      req.OCIReference, // Store OCI reference as command
		PluginType:   models.PluginTypeAIStudio,
		OCIReference: req.OCIReference,
		HookType:     models.HookTypeStudioUI, // AI Studio plugins use studio_ui hook type
		IsActive:     req.IsActive,
		Namespace:    req.Namespace,
		Config:       req.Config,
		Manifest:     manifest,
	}

	if err := plugin.Create(s.db); err != nil {
		return nil, fmt.Errorf("failed to create OCI plugin record: %w", err)
	}

	return plugin, nil
}

// ListCachedOCIPlugins returns all cached OCI plugins
func (s *PluginService) ListCachedOCIPlugins() ([]*ociplugins.LocalPlugin, error) {
	if s.ociClient == nil {
		return nil, fmt.Errorf("OCI client not configured")
	}
	return s.ociClient.ListCached()
}

// RefreshOCIPlugin refreshes an OCI plugin from the registry
func (s *PluginService) RefreshOCIPlugin(pluginID uint) (*models.Plugin, error) {
	if s.ociClient == nil {
		return nil, fmt.Errorf("OCI client not configured")
	}

	plugin, err := s.GetPlugin(pluginID)
	if err != nil {
		return nil, err
	}

	if !plugin.IsOCIPlugin() {
		return nil, fmt.Errorf("plugin is not an OCI plugin")
	}

	// Parse OCI reference
	ref, params, err := ociplugins.ParseOCICommand(plugin.OCIReference)
	if err != nil {
		return nil, fmt.Errorf("failed to parse OCI reference: %w", err)
	}

	// Fetch latest version to verify it exists and update cache
	ctx := context.Background()
	_, err = s.ociClient.FetchPlugin(ctx, ref, params)
	if err != nil {
		return nil, fmt.Errorf("failed to refresh OCI plugin: %w", err)
	}

	// For MVP, maintain existing manifest
	// In full implementation, this would re-extract manifest.json from the updated OCI artifact
	// The manifest will be properly populated when ParsePluginManifest is called

	if err := plugin.Update(s.db); err != nil {
		return nil, fmt.Errorf("failed to update plugin: %w", err)
	}

	return plugin, nil
}

// GetPluginsByType returns plugins filtered by type
func (s *PluginService) GetPluginsByType(pluginType string) ([]models.Plugin, error) {
	var plugins []models.Plugin

	if err := s.db.Where("plugin_type = ? AND is_active = ?", pluginType, true).
		Order("created_at DESC").Find(&plugins).Error; err != nil {
		return nil, fmt.Errorf("failed to get plugins by type: %w", err)
	}

	return plugins, nil
}

// GetAIStudioPluginsWithManifests returns AI Studio plugins that have UI manifests
func (s *PluginService) GetAIStudioPluginsWithManifests() ([]models.Plugin, error) {
	var plugins []models.Plugin

	if err := s.db.Where("plugin_type = ? AND is_active = ? AND manifest IS NOT NULL AND manifest != '{}'",
		models.PluginTypeAIStudio, true).
		Order("created_at DESC").Find(&plugins).Error; err != nil {
		return nil, fmt.Errorf("failed to get AI Studio plugins with manifests: %w", err)
	}

	return plugins, nil
}

// CreateOCIPluginRequest represents a request to create a plugin from an OCI artifact
type CreateOCIPluginRequest struct {
	Name         string                 `json:"name" binding:"required"`
	Slug         string                 `json:"slug" binding:"required"`
	Description  string                 `json:"description"`
	OCIReference string                 `json:"oci_reference" binding:"required"`
	Config       map[string]interface{} `json:"config"`
	HookType     string                 `json:"hook_type" binding:"required"`
	IsActive     bool                   `json:"is_active"`
	Namespace    string                 `json:"namespace,omitempty"`
}

// Plugin Config Schema Methods

// GetPluginConfigSchema retrieves the configuration schema for a plugin, using cache when possible
func (s *PluginService) GetPluginConfigSchema(ctx context.Context, pluginID uint) (string, error) {
	// Get plugin to access its command
	plugin, err := s.GetPlugin(pluginID)
	if err != nil {
		return "", fmt.Errorf("failed to get plugin: %w", err)
	}

	return s.GetPluginConfigSchemaByCommand(ctx, plugin.Command)
}

// GetPluginConfigSchemaByCommand retrieves schema by command, using cache when possible
func (s *PluginService) GetPluginConfigSchemaByCommand(ctx context.Context, command string) (string, error) {
	// Check cache first
	cachedSchema := &models.PluginConfigSchema{}
	err := cachedSchema.GetByCommand(s.db, command)

	if err == nil {
		// Cache hit - check if it's still fresh (24 hour TTL)
		if !cachedSchema.IsStale(24 * 60 * 60 * time.Second) {
			log.Debug().
				Str("command", command).
				Time("last_fetched", cachedSchema.LastFetched).
				Msg("Using cached plugin config schema")
			return cachedSchema.SchemaJSON, nil
		}
	} else if err != gorm.ErrRecordNotFound {
		return "", fmt.Errorf("failed to check schema cache: %w", err)
	}

	// Cache miss or stale - fetch from plugin
	log.Debug().
		Str("command", command).
		Msg("Cache miss or stale - fetching schema from plugin")

	schemaJSON, err := s.fetchSchemaFromPlugin(ctx, command)
	if err != nil {
		return "", fmt.Errorf("failed to fetch schema from plugin: %w", err)
	}

	// Update cache
	if err := cachedSchema.Upsert(s.db, command, schemaJSON); err != nil {
		log.Warn().Err(err).Str("command", command).Msg("Failed to update schema cache")
		// Don't fail the request just because caching failed
	}

	return schemaJSON, nil
}

// RefreshPluginConfigSchema forces a refresh of the schema from the plugin
func (s *PluginService) RefreshPluginConfigSchema(ctx context.Context, pluginID uint) (string, error) {
	plugin, err := s.GetPlugin(pluginID)
	if err != nil {
		return "", fmt.Errorf("failed to get plugin: %w", err)
	}

	log.Info().
		Uint("plugin_id", pluginID).
		Str("command", plugin.Command).
		Msg("Refreshing plugin config schema - fetching fresh from plugin")

	// Fetch fresh schema from plugin
	schemaJSON, err := s.fetchSchemaFromPlugin(ctx, plugin.Command)
	if err != nil {
		return "", fmt.Errorf("failed to fetch fresh schema from plugin: %w", err)
	}

	// Use GORM's proper upsert with Where + Updates
	now := time.Now()
	result := s.db.Model(&models.PluginConfigSchema{}).
		Where("command = ?", plugin.Command).
		Updates(map[string]interface{}{
			"schema_json":  schemaJSON,
			"last_fetched": now,
			"updated_at":   now,
		})

	if result.Error != nil {
		log.Warn().Err(result.Error).Str("command", plugin.Command).Msg("Failed to update schema cache")
	} else if result.RowsAffected == 0 {
		// No existing record to update, create new one
		cachedSchema := &models.PluginConfigSchema{
			Command:     plugin.Command,
			SchemaJSON:  schemaJSON,
			LastFetched: now,
		}
		if err := s.db.Create(cachedSchema).Error; err != nil {
			log.Warn().Err(err).Str("command", plugin.Command).Msg("Failed to create new schema cache entry")
		}
	}

	log.Info().
		Uint("plugin_id", pluginID).
		Str("command", plugin.Command).
		Int("schema_bytes", len(schemaJSON)).
		Msg("Successfully refreshed plugin config schema")

	return schemaJSON, nil
}

// InvalidateSchemaCache removes a schema from the cache
func (s *PluginService) InvalidateSchemaCache(command string) error {
	cachedSchema := &models.PluginConfigSchema{}
	err := cachedSchema.GetByCommand(s.db, command)
	if err == gorm.ErrRecordNotFound {
		return nil // Already not cached
	}
	if err != nil {
		return fmt.Errorf("failed to find cached schema: %w", err)
	}

	if err := cachedSchema.Delete(s.db); err != nil {
		return fmt.Errorf("failed to delete cached schema: %w", err)
	}

	log.Debug().Str("command", command).Msg("Invalidated plugin config schema cache")
	return nil
}

// fetchSchemaFromPlugin loads a plugin and calls GetConfigSchema
func (s *PluginService) fetchSchemaFromPlugin(ctx context.Context, command string) (string, error) {
	// Check if we have the AI Studio plugin manager available
	if s.pluginManager == nil {
		log.Warn().Str("command", command).Msg("Plugin manager not available - SetPluginManager may not have been called")

		// Return an error instead of default schema to help diagnose issues
		return "", fmt.Errorf("plugin manager not available - cannot fetch schema from plugin")
	}

	// Use plugin manager to load plugin for config-only access
	log.Info().Str("command", command).Msg("Loading plugin for schema extraction")

	configProvider, err := s.pluginManager.LoadPluginForConfigOnly(ctx, command)
	if err != nil {
		log.Error().Err(err).Str("command", command).Msg("Failed to load plugin for schema extraction")
		return "", fmt.Errorf("failed to load plugin for schema extraction: %w", err)
	}
	defer s.pluginManager.UnloadConfigProvider(configProvider)

	// Get schema from plugin
	schemaBytes, err := configProvider.GetConfigSchema(ctx)
	if err != nil {
		log.Error().Err(err).Str("command", command).Msg("Failed to get schema from plugin")
		return "", fmt.Errorf("failed to get config schema from plugin: %w", err)
	}

	log.Debug().
		Str("command", command).
		Int("schema_bytes", len(schemaBytes)).
		Msg("Successfully fetched schema from plugin")

	return string(schemaBytes), nil
}

// Universal Manifest Loading Methods

// LoadPluginManifestViaGRPC loads manifest from any plugin type using the GetManifest gRPC method
func (s *PluginService) LoadPluginManifestViaGRPC(pluginID uint) (*models.PluginManifest, error) {
	// Verify plugin exists and is AI Studio type
	plugin, err := s.GetPlugin(pluginID)
	if err != nil {
		return nil, fmt.Errorf("failed to get plugin: %w", err)
	}

	if !plugin.IsAIStudioPlugin() {
		return nil, fmt.Errorf("manifest loading only supported for AI Studio plugins")
	}

	// Check if AI Studio plugin manager is available
	if s.pluginManager == nil {
		return nil, fmt.Errorf("AI Studio plugin manager not configured")
	}

	// Get manifest via gRPC (works for all deployment types: file, remote, OCI)
	manifestJSON, err := s.pluginManager.GetPluginManifest(pluginID)
	if err != nil {
		return nil, fmt.Errorf("failed to get manifest via gRPC: %w", err)
	}

	// Parse manifest
	var manifest models.PluginManifest
	if err := json.Unmarshal([]byte(manifestJSON), &manifest); err != nil {
		return nil, fmt.Errorf("failed to parse manifest JSON: %w", err)
	}

	// Validate manifest structure
	if err := manifest.ValidateManifest(); err != nil {
		return nil, fmt.Errorf("invalid manifest: %w", err)
	}

	log.Info().
		Uint("plugin_id", pluginID).
		Str("plugin_name", plugin.Name).
		Int("service_scopes", len(manifest.GetServiceScopes())).
		Msg("Successfully loaded plugin manifest via gRPC")

	return &manifest, nil
}

// ExtractAndStoreServiceScopes extracts service scopes from manifest and stores them in the database
func (s *PluginService) ExtractAndStoreServiceScopes(pluginID uint) error {
	// Load manifest via gRPC
	manifest, err := s.LoadPluginManifestViaGRPC(pluginID)
	if err != nil {
		return fmt.Errorf("failed to load plugin manifest: %w", err)
	}

	// Extract service scopes from manifest
	serviceScopes := manifest.GetServiceScopes()

	// Get plugin from database
	plugin, err := s.GetPlugin(pluginID)
	if err != nil {
		return fmt.Errorf("failed to get plugin: %w", err)
	}

	// Update plugin with extracted scopes (but don't authorize yet - admin must approve)
	plugin.ServiceScopes = serviceScopes
	plugin.ServiceAccessAuthorized = false // Require admin authorization

	if err := plugin.Update(s.db); err != nil {
		return fmt.Errorf("failed to update plugin with service scopes: %w", err)
	}

	log.Info().
		Uint("plugin_id", pluginID).
		Str("plugin_name", plugin.Name).
		Strs("service_scopes", serviceScopes).
		Msg("Extracted and stored service scopes from manifest")

	return nil
}

// AuthorizePluginServiceAccess authorizes service access for a plugin (admin operation)
func (s *PluginService) AuthorizePluginServiceAccess(pluginID uint, adminApproval bool) error {
	plugin, err := s.GetPlugin(pluginID)
	if err != nil {
		return fmt.Errorf("failed to get plugin: %w", err)
	}

	if len(plugin.ServiceScopes) == 0 {
		return fmt.Errorf("plugin has no service scopes declared - load manifest first")
	}

	if adminApproval {
		// Authorize access with the scopes declared in manifest
		if err := plugin.AuthorizeServiceAccess(s.db, plugin.ServiceScopes); err != nil {
			return fmt.Errorf("failed to authorize service access: %w", err)
		}

		log.Info().
			Uint("plugin_id", pluginID).
			Str("plugin_name", plugin.Name).
			Strs("authorized_scopes", plugin.ServiceScopes).
			Msg("Admin authorized plugin service access")
	} else {
		// Revoke access
		if err := plugin.RevokeServiceAccess(s.db); err != nil {
			return fmt.Errorf("failed to revoke service access: %w", err)
		}

		log.Info().
			Uint("plugin_id", pluginID).
			Str("plugin_name", plugin.Name).
			Msg("Admin revoked plugin service access")
	}

	return nil
}