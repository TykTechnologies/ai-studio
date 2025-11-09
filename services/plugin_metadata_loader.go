package services

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// PluginMetadata represents combined plugin metadata from a single load operation
type PluginMetadata struct {
	ConfigSchema string                 `json:"config_schema"` // JSON schema as string
	Manifest     *models.PluginManifest `json:"manifest"`      // Parsed manifest
	Command      string                 `json:"command"`       // Plugin command used for loading
	LoadTime     time.Time              `json:"load_time"`     // When this metadata was loaded
}

// PluginMetadataLoader provides unified loading of plugin configuration schema and manifest
type PluginMetadataLoader struct {
	db            *gorm.DB
	pluginManager *AIStudioPluginManager
}

// NewPluginMetadataLoader creates a new plugin metadata loader
func NewPluginMetadataLoader(db *gorm.DB, pluginManager *AIStudioPluginManager) *PluginMetadataLoader {
	return &PluginMetadataLoader{
		db:            db,
		pluginManager: pluginManager,
	}
}

// LoadPluginMetadata loads both config schema and manifest using enhanced ConfigProviderService
func (l *PluginMetadataLoader) LoadPluginMetadata(ctx context.Context, command string) (*PluginMetadata, error) {
	if l.pluginManager == nil {
		return nil, fmt.Errorf("plugin manager not configured")
	}

	log.Info().
		Str("command", command).
		Msg("Loading plugin metadata (config schema + manifest) via enhanced config provider")

	// Use the enhanced config-only loading to get both schema and manifest
	configProvider, err := l.pluginManager.LoadPluginForConfigOnly(ctx, command)
	if err != nil {
		return nil, fmt.Errorf("failed to load plugin for config-only access: %w", err)
	}
	defer l.pluginManager.UnloadConfigProvider(configProvider)

	// Get config schema
	log.Debug().Str("command", command).Msg("Getting config schema from plugin")
	schemaBytes, err := configProvider.GetConfigSchema(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get config schema: %w", err)
	}

	// Get manifest (this will only work for AI Studio plugins with the enhanced interface)
	var manifest *models.PluginManifest
	log.Debug().Str("command", command).Msg("Getting manifest from plugin")

	// Check if the config provider supports GetManifest (enhanced config provider)
	if enhancedProvider, ok := configProvider.(EnhancedConfigProvider); ok {
		manifestJSON, manifestErr := enhancedProvider.GetManifest(ctx)
		if manifestErr != nil {
			log.Warn().
				Err(manifestErr).
				Str("command", command).
				Msg("Failed to get manifest from enhanced config provider - plugin may not support manifests")
			// Don't fail the entire operation - some plugins may not have manifests
		} else {
			// Parse manifest JSON
			manifest = &models.PluginManifest{}
			if parseErr := json.Unmarshal(manifestJSON, manifest); parseErr != nil {
				log.Warn().
					Err(parseErr).
					Str("command", command).
					Msg("Failed to parse manifest JSON - continuing without manifest")
				manifest = nil
			} else {
				// Validate manifest structure
				if validateErr := manifest.ValidateManifest(); validateErr != nil {
					log.Warn().
						Err(validateErr).
						Str("command", command).
						Str("manifest_id", manifest.ID).
						Strs("blocked_scopes", manifest.GetServiceScopes()).
						Msg("Manifest validation failed - continuing without manifest")
					manifest = nil
				} else {
					log.Info().
						Str("command", command).
						Str("manifest_id", manifest.ID).
						Strs("service_scopes", manifest.GetServiceScopes()).
						Msg("Manifest loaded and validated successfully")
				}
			}
		}
	} else {
		log.Debug().
			Str("command", command).
			Msg("Config provider does not support GetManifest - continuing with schema only")
	}

	metadata := &PluginMetadata{
		ConfigSchema: string(schemaBytes),
		Manifest:     manifest,
		Command:      command,
		LoadTime:     time.Now(),
	}

	log.Info().
		Str("command", command).
		Int("schema_bytes", len(schemaBytes)).
		Bool("has_manifest", manifest != nil).
		Msg("Successfully loaded plugin metadata")

	return metadata, nil
}

// LoadPluginMetadataByID loads metadata for a plugin by its database ID
func (l *PluginMetadataLoader) LoadPluginMetadataByID(ctx context.Context, pluginID uint) (*PluginMetadata, error) {
	// Get plugin from database to access its command
	var plugin models.Plugin
	if err := l.db.First(&plugin, pluginID).Error; err != nil {
		return nil, fmt.Errorf("plugin not found: %w", err)
	}

	if !plugin.IsActive {
		return nil, fmt.Errorf("plugin %d is not active", pluginID)
	}

	return l.LoadPluginMetadata(ctx, plugin.Command)
}

// ExtractScopesFromMetadata extracts all permission scopes from metadata (if manifest is available)
// This includes both service scopes (AI Studio services) and object_hooks permissions
func (l *PluginMetadataLoader) ExtractScopesFromMetadata(metadata *PluginMetadata) []string {
	if metadata.Manifest == nil {
		log.Debug().Str("command", metadata.Command).Msg("No manifest available - no scopes to extract")
		return []string{}
	}

	scopes := metadata.Manifest.GetAllPermissionScopes()
	log.Debug().
		Str("command", metadata.Command).
		Strs("scopes", scopes).
		Int("service_scopes", len(metadata.Manifest.GetServiceScopes())).
		Int("object_hooks", len(metadata.Manifest.GetObjectHooks())).
		Msg("Extracted permission scopes from manifest")

	return scopes
}

// EnhancedConfigProvider extends ConfigProvider to include manifest support
// This interface will be implemented by config providers that support the enhanced protocol
type EnhancedConfigProvider interface {
	ConfigProvider
	// GetManifest returns the plugin manifest as JSON bytes
	GetManifest(ctx context.Context) ([]byte, error)
}