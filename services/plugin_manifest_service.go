package services

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/ociplugins"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// PluginManifestService handles plugin manifest parsing and UI registration
type PluginManifestService struct {
	db        *gorm.DB
	ociClient *ociplugins.OCIPluginClient
}

// NewPluginManifestService creates a new plugin manifest service
func NewPluginManifestService(db *gorm.DB, ociClient *ociplugins.OCIPluginClient) *PluginManifestService {
	return &PluginManifestService{
		db:        db,
		ociClient: ociClient,
	}
}

// ParsePluginManifest extracts and parses the manifest from an OCI plugin
func (s *PluginManifestService) ParsePluginManifest(plugin *models.Plugin) (*models.PluginManifest, error) {
	if !plugin.IsOCIPlugin() || plugin.OCIReference == "" {
		return nil, fmt.Errorf("plugin is not an OCI plugin")
	}

	if s.ociClient == nil {
		return nil, fmt.Errorf("OCI client not configured")
	}

	// Parse OCI reference
	ref, params, err := ociplugins.ParseOCICommand(plugin.OCIReference)
	if err != nil {
		return nil, fmt.Errorf("failed to parse OCI reference: %w", err)
	}

	// Verify plugin exists in cache or fetch it
	_, err = s.ociClient.GetPlugin(ref, params)
	if err != nil {
		// Try to fetch if not cached
		ctx := context.Background()
		_, err = s.ociClient.FetchPlugin(ctx, ref, params)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch plugin: %w", err)
		}
	}

	// For MVP, we'll parse manifest from the plugin's stored manifest field
	// In a full implementation, this would extract manifest.json from the OCI artifact
	manifest := &models.PluginManifest{}

	// Try to get manifest from the plugin's manifest field
	if len(plugin.Manifest) > 0 {
		manifestBytes, err := json.Marshal(plugin.Manifest)
		if err != nil {
			return nil, fmt.Errorf("failed to serialize manifest data: %w", err)
		}

		if err := json.Unmarshal(manifestBytes, manifest); err != nil {
			return nil, fmt.Errorf("failed to parse manifest from plugin: %w", err)
		}

		// Validate and return
		if err := manifest.ValidateManifest(); err != nil {
			return nil, fmt.Errorf("invalid manifest: %w", err)
		}

		return manifest, nil
	}

	// TODO: For full implementation, extract manifest.json from OCI artifact
	// This would require extending the OCI client to extract individual files from the artifact

	return nil, fmt.Errorf("no manifest found in plugin - ensure manifest field is populated when creating OCI plugin")
}

// RegisterPluginUI registers plugin UI components in the system
func (s *PluginManifestService) RegisterPluginUI(plugin *models.Plugin, manifest *models.PluginManifest) error {
	log.Info().
		Uint("plugin_id", plugin.ID).
		Str("plugin_name", plugin.Name).
		Msg("Registering plugin UI components")

	log.Info().
		Uint("received_plugin_id", plugin.ID).
		Msg("DEBUG: Plugin ID received in RegisterPluginUI")

	// Verify plugin exists in database
	var dbPlugin models.Plugin
	if err := s.db.First(&dbPlugin, plugin.ID).Error; err != nil {
		return fmt.Errorf("plugin ID %d not found in database: %w", plugin.ID, err)
	}

	log.Info().
		Uint("verified_plugin_id", dbPlugin.ID).
		Msg("DEBUG: Plugin verified in database")

	// Create or update registered plugin record
	var registeredPlugin models.RegisteredPlugin
	queryErr := s.db.Where("plugin_id = ?", plugin.ID).First(&registeredPlugin).Error

	// Determine if we need to create or update
	registeredPluginExists := true
	if queryErr != nil {
		if queryErr == gorm.ErrRecordNotFound {
			registeredPluginExists = false
		} else {
			return fmt.Errorf("failed to query registered plugin: %w", queryErr)
		}
	}

	// Convert manifest to JSON for storage
	manifestData, err := json.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("failed to serialize manifest: %w", err)
	}

	var parsedManifest map[string]interface{}
	if err := json.Unmarshal(manifestData, &parsedManifest); err != nil {
		return fmt.Errorf("failed to parse manifest for storage: %w", err)
	}

	if !registeredPluginExists {
		// Create new registered plugin
		log.Info().
			Uint("plugin_id", plugin.ID).
			Str("manifest_version", manifest.Version).
			Msg("Creating new registered plugin record")

		// Explicitly ensure plugin ID is set correctly
		pluginID := plugin.ID
		if pluginID == 0 {
			return fmt.Errorf("plugin ID is 0 - cannot create registered plugin record")
		}

		// Use the verified database plugin ID to avoid any pointer issues
		registeredPlugin = models.RegisteredPlugin{
			PluginID:        dbPlugin.ID, // Use the verified plugin from database
			ManifestVersion: manifest.Version,
			ParsedManifest:  parsedManifest,
			IsLoaded:        false,
			AssetPaths:      manifest.Assets,
		}

		log.Info().
			Uint("about_to_insert_plugin_id", registeredPlugin.PluginID).
			Uint("original_plugin_id", plugin.ID).
			Uint("db_plugin_id", dbPlugin.ID).
			Uint("pluginID_var", pluginID).
			Msg("About to insert registered plugin")

		// Debug: Print the struct before insertion
		log.Printf("DEBUG: RegisteredPlugin struct before GORM insert: ID=%d, PluginID=%d", registeredPlugin.ID, registeredPlugin.PluginID)

		// Use correct GORM struct creation
		registeredPlugin = models.RegisteredPlugin{
			PluginID:        dbPlugin.ID,
			ManifestVersion: manifest.Version,
			ParsedManifest:  parsedManifest,
			IsLoaded:        false,
			AssetPaths:      manifest.Assets,
		}

		if err := s.db.Create(&registeredPlugin).Error; err != nil {
			return fmt.Errorf("failed to create registered plugin: %w", err)
		}

		log.Info().
			Uint("inserted_plugin_id", registeredPlugin.PluginID).
			Msg("Successfully created registered plugin")
	} else {
		// Update existing
		registeredPlugin.ManifestVersion = manifest.Version
		registeredPlugin.ParsedManifest = parsedManifest
		registeredPlugin.AssetPaths = manifest.Assets
		registeredPlugin.IsLoaded = false
		registeredPlugin.LoadError = ""
		if err := s.db.Save(&registeredPlugin).Error; err != nil {
			return fmt.Errorf("failed to update registered plugin: %w", err)
		}
	}

	// Clear existing UI registry entries for this plugin
	if err := s.db.Where("plugin_id = ?", plugin.ID).Delete(&models.UIRegistry{}).Error; err != nil {
		return fmt.Errorf("failed to clear existing UI registry entries: %w", err)
	}

	// Register UI components
	if manifest.UI != nil {
		for _, slot := range manifest.UI.Slots {
			for _, item := range slot.Items {
				if item.Type == "route" {
					log.Info().
						Uint("plugin_id", plugin.ID).
						Str("original_path", item.Path).
						Str("item_title", item.Title).
						Msg("DEBUG: Processing route item from manifest")

					uiEntry := models.UIRegistry{
						PluginID:     plugin.ID,
						SlotType:     slot.Slot,
						RoutePattern: item.Path,
						ComponentTag: item.Mount.Tag,
						EntryPoint:   item.Mount.Entry,
						MountConfig: map[string]interface{}{
							"kind":   item.Mount.Kind,
							"tag":    item.Mount.Tag,
							"entry":  item.Mount.Entry,
							"props":  item.Mount.Props,
							"remote": item.Mount.Remote,
							"exposed": item.Mount.Exposed,
							"app":    item.Mount.App,
							"title":  item.Title,
							"label":  slot.Label,
							"icon":   slot.Icon,
						},
						IsActive:     true,
						LoadPriority: 0,
					}

					if err := s.db.Create(&uiEntry).Error; err != nil {
						return fmt.Errorf("failed to register UI component: %w", err)
					}

					log.Info().
						Uint("plugin_id", plugin.ID).
						Str("stored_route_pattern", uiEntry.RoutePattern).
						Str("component_tag", uiEntry.ComponentTag).
						Msg("DEBUG: Successfully stored UI registry entry")
				}
			}
		}
	}

	log.Info().
		Uint("plugin_id", plugin.ID).
		Str("plugin_name", plugin.Name).
		Str("manifest_version", manifest.Version).
		Int("ui_components", len(manifest.UI.Slots)).
		Msg("Plugin UI components registered successfully")

	return nil
}

// LoadPluginUI marks a plugin's UI as loaded and updates the registry
func (s *PluginManifestService) LoadPluginUI(pluginID uint) error {
	now := time.Now()

	// Update registered plugin status
	result := s.db.Model(&models.RegisteredPlugin{}).
		Where("plugin_id = ?", pluginID).
		Updates(map[string]interface{}{
			"is_loaded":  true,
			"loaded_at":  &now,
			"load_error": "",
		})

	if result.Error != nil {
		return fmt.Errorf("failed to update plugin load status: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("plugin not found in registry")
	}

	log.Info().Uint("plugin_id", pluginID).Msg("Plugin UI marked as loaded")
	return nil
}

// UnloadPluginUI marks a plugin's UI as unloaded
func (s *PluginManifestService) UnloadPluginUI(pluginID uint) error {
	// Update registered plugin status
	result := s.db.Model(&models.RegisteredPlugin{}).
		Where("plugin_id = ?", pluginID).
		Updates(map[string]interface{}{
			"is_loaded": false,
			"loaded_at": nil,
		})

	if result.Error != nil {
		return fmt.Errorf("failed to update plugin unload status: %w", result.Error)
	}

	// Deactivate UI registry entries
	if err := s.db.Model(&models.UIRegistry{}).
		Where("plugin_id = ?", pluginID).
		Update("is_active", false).Error; err != nil {
		return fmt.Errorf("failed to deactivate UI components: %w", err)
	}

	log.Info().Uint("plugin_id", pluginID).Msg("Plugin UI marked as unloaded")
	return nil
}

// GetUIRegistry returns all active UI components for the frontend
func (s *PluginManifestService) GetUIRegistry() ([]models.UIRegistry, error) {
	var entries []models.UIRegistry
	err := s.db.Preload("Plugin").
		Where("is_active = ?", true).
		Order("load_priority DESC, created_at ASC").
		Find(&entries).Error

	if err != nil {
		return nil, fmt.Errorf("failed to get UI registry: %w", err)
	}

	return entries, nil
}

// GetSidebarMenuItems returns sidebar menu items from registered plugins grouped by plugin
func (s *PluginManifestService) GetSidebarMenuItems() ([]SidebarMenuItem, error) {
	var entries []models.UIRegistry
	err := s.db.Preload("Plugin").
		Where("is_active = ? AND slot_type = ?", true, "sidebar.section").
		Order("load_priority DESC, created_at ASC").
		Find(&entries).Error

	if err != nil {
		return nil, fmt.Errorf("failed to get sidebar menu items: %w", err)
	}

	// Group entries by plugin to create collapsible sections
	pluginGroups := make(map[uint][]models.UIRegistry)
	for _, entry := range entries {
		if entry.Plugin == nil {
			continue
		}
		pluginGroups[entry.PluginID] = append(pluginGroups[entry.PluginID], entry)
	}

	var menuItems []SidebarMenuItem
	for pluginID, pluginEntries := range pluginGroups {
		if len(pluginEntries) == 0 {
			continue
		}

		// Get plugin info from first entry
		firstEntry := pluginEntries[0]
		sectionLabel, _ := firstEntry.MountConfig["label"].(string)
		sectionIcon, _ := firstEntry.MountConfig["icon"].(string)

		// Create sub-items for this plugin
		var subItems []SidebarSubItem
		for _, entry := range pluginEntries {
			title, _ := entry.MountConfig["title"].(string)
			subItems = append(subItems, SidebarSubItem{
				ID:           fmt.Sprintf("plugin_%d_%s", entry.PluginID, entry.ComponentTag),
				Text:         title,
				Path:         entry.RoutePattern,
				ComponentTag: entry.ComponentTag,
				EntryPoint:   entry.EntryPoint,
				MountConfig:  entry.MountConfig,
			})
		}

		// Create main plugin section
		menuItem := SidebarMenuItem{
			ID:       fmt.Sprintf("plugin_%d", pluginID),
			Label:    sectionLabel,
			Icon:     sectionIcon,
			PluginID: pluginID,
			PluginName: firstEntry.Plugin.Name,
			SubItems: subItems,
		}

		menuItems = append(menuItems, menuItem)
	}

	return menuItems, nil
}

// GetPluginAssets returns asset paths for a plugin
func (s *PluginManifestService) GetPluginAssets(pluginID uint) ([]string, error) {
	var registeredPlugin models.RegisteredPlugin
	err := s.db.Where("plugin_id = ?", pluginID).First(&registeredPlugin).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return []string{}, nil
		}
		return nil, fmt.Errorf("failed to get plugin assets: %w", err)
	}

	return registeredPlugin.AssetPaths, nil
}

// ValidatePluginPermissions checks if a plugin has the required permissions
func (s *PluginManifestService) ValidatePluginPermissions(pluginID uint, requiredPermissions map[string][]string) error {
	var registeredPlugin models.RegisteredPlugin
	err := s.db.Where("plugin_id = ?", pluginID).First(&registeredPlugin).Error
	if err != nil {
		return fmt.Errorf("plugin not found in registry: %w", err)
	}

	// Parse manifest to check permissions
	var manifest models.PluginManifest
	manifestBytes, err := json.Marshal(registeredPlugin.ParsedManifest)
	if err != nil {
		return fmt.Errorf("failed to serialize manifest: %w", err)
	}

	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return fmt.Errorf("failed to parse stored manifest: %w", err)
	}

	// Check each required permission type
	for permType, permissions := range requiredPermissions {
		for _, permission := range permissions {
			if !manifest.HasPermission(permType, permission) {
				return fmt.Errorf("plugin lacks required %s permission: %s", permType, permission)
			}
		}
	}

	return nil
}

// SidebarMenuItem represents a plugin-contributed sidebar menu item
type SidebarMenuItem struct {
	ID           string                 `json:"id"`
	Label        string                 `json:"label"`
	Icon         string                 `json:"icon"`
	Path         string                 `json:"path,omitempty"`
	Title        string                 `json:"title,omitempty"`
	PluginID     uint                   `json:"plugin_id"`
	PluginName   string                 `json:"plugin_name"`
	ComponentTag string                 `json:"component_tag,omitempty"`
	EntryPoint   string                 `json:"entry_point,omitempty"`
	MountConfig  map[string]interface{} `json:"mount_config,omitempty"`
	SubItems     []SidebarSubItem       `json:"sub_items,omitempty"`
}

// SidebarSubItem represents a sub-item within a plugin sidebar section
type SidebarSubItem struct {
	ID           string                 `json:"id"`
	Text         string                 `json:"text"`
	Path         string                 `json:"path"`
	ComponentTag string                 `json:"component_tag"`
	EntryPoint   string                 `json:"entry_point"`
	MountConfig  map[string]interface{} `json:"mount_config"`
}

// ServePluginAsset serves plugin assets with proper path resolution
func (s *PluginManifestService) ServePluginAsset(pluginID uint, assetPath string) (string, error) {
	// Validate plugin exists
	var plugin models.Plugin
	if err := s.db.First(&plugin, pluginID).Error; err != nil {
		return "", fmt.Errorf("plugin not found: %w", err)
	}

	if !plugin.IsOCIPlugin() {
		return "", fmt.Errorf("asset serving only supported for OCI plugins")
	}

	// Security: validate asset path
	cleanPath := filepath.Clean(assetPath)
	if strings.Contains(cleanPath, "..") || strings.HasPrefix(cleanPath, "/") {
		return "", fmt.Errorf("invalid asset path: %s", assetPath)
	}

	// For now, return a placeholder URL - in a full implementation,
	// this would extract the asset from the OCI artifact and serve it
	// or return the path where it's been extracted
	assetURL := fmt.Sprintf("/plugins/assets/%d/%s", pluginID, cleanPath)

	return assetURL, nil
}