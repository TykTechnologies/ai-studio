package services

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/marketplace"
	"github.com/TykTechnologies/midsommar/v2/pkg/ociplugins"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// MarketplaceService handles marketplace operations
type MarketplaceService struct {
	db               *gorm.DB
	fetcher          *marketplace.Fetcher
	ociClient        *ociplugins.OCIPluginClient
	pluginService    *PluginService
	pluginManager    *AIStudioPluginManager
	cacheDir         string
	defaultIndexURL  string
	syncInterval     time.Duration
	stopCh           chan struct{}
}

// NewMarketplaceService creates a new marketplace service
func NewMarketplaceService(
	db *gorm.DB,
	ociClient *ociplugins.OCIPluginClient,
	pluginService *PluginService,
	pluginManager *AIStudioPluginManager,
	cacheDir string,
	defaultIndexURL string,
	syncInterval time.Duration,
) *MarketplaceService {
	if syncInterval == 0 {
		syncInterval = 1 * time.Hour // Default: sync every hour
	}

	return &MarketplaceService{
		db:              db,
		fetcher:         marketplace.NewFetcher(30 * time.Second),
		ociClient:       ociClient,
		pluginService:   pluginService,
		pluginManager:   pluginManager,
		cacheDir:        cacheDir,
		defaultIndexURL: defaultIndexURL,
		syncInterval:    syncInterval,
		stopCh:          make(chan struct{}),
	}
}

// Start begins background marketplace sync
func (s *MarketplaceService) Start(ctx context.Context) {
	log.Info().
		Str("default_index_url", s.defaultIndexURL).
		Dur("sync_interval", s.syncInterval).
		Msg("Starting marketplace service")

	// Initial sync
	if err := s.SyncAll(ctx); err != nil {
		log.Error().Err(err).Msg("Initial marketplace sync failed")
	}

	// Background sync loop
	ticker := time.NewTicker(s.syncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := s.SyncAll(ctx); err != nil {
				log.Error().Err(err).Msg("Marketplace sync failed")
			}
		case <-s.stopCh:
			log.Info().Msg("Stopping marketplace service")
			return
		case <-ctx.Done():
			log.Info().Msg("Marketplace service context cancelled")
			return
		}
	}
}

// Stop stops the marketplace service
func (s *MarketplaceService) Stop() {
	close(s.stopCh)
}

// SyncAll syncs all active marketplace indexes
func (s *MarketplaceService) SyncAll(ctx context.Context) error {
	log.Info().Msg("Syncing all marketplace indexes")

	indexes, err := models.GetAllActiveMarketplaceIndexes(s.db)
	if err != nil {
		return fmt.Errorf("failed to get active indexes: %w", err)
	}

	// If no indexes exist, create default
	if len(indexes) == 0 && s.defaultIndexURL != "" {
		log.Info().Str("url", s.defaultIndexURL).Msg("Creating default marketplace index")
		defaultIndex := &models.MarketplaceIndex{
			SourceURL: s.defaultIndexURL,
			IsDefault: true,
			IsActive:  true,
		}
		if err := s.db.Create(defaultIndex).Error; err != nil {
			return fmt.Errorf("failed to create default index: %w", err)
		}
		indexes = append(indexes, defaultIndex)
	}

	// Sync each index
	var lastErr error
	for _, idx := range indexes {
		if err := s.SyncIndex(ctx, idx); err != nil {
			log.Error().
				Err(err).
				Str("source_url", idx.SourceURL).
				Msg("Failed to sync marketplace index")
			lastErr = err
			continue
		}
	}

	// Check for updates on installed plugins
	if err := s.CheckForUpdates(ctx); err != nil {
		log.Error().Err(err).Msg("Failed to check for plugin updates")
	}

	return lastErr
}

// SyncIndex syncs a single marketplace index
func (s *MarketplaceService) SyncIndex(ctx context.Context, idx *models.MarketplaceIndex) error {
	startTime := time.Now()

	log.Info().
		Uint("index_id", idx.ID).
		Str("source_url", idx.SourceURL).
		Msg("Syncing marketplace index")

	// Update sync status
	idx.SyncStatus = "in_progress"
	s.db.Save(idx)

	// Fetch index with conditional request
	index, metadata, modified, err := s.fetcher.FetchIndexConditional(
		ctx,
		idx.SourceURL,
		idx.ETag,
		idx.LastModified,
	)

	if err != nil {
		idx.SyncStatus = "error"
		idx.SyncError = err.Error()
		s.db.Save(idx)
		return fmt.Errorf("failed to fetch index: %w", err)
	}

	// Not modified - still update last synced time
	if !modified {
		idx.SyncStatus = "success"
		idx.LastSynced = time.Now()
		idx.SyncError = ""
		s.db.Save(idx)
		log.Debug().Str("source_url", idx.SourceURL).Msg("Marketplace index not modified")
		return nil
	}

	// Validate index
	if err := marketplace.ValidateIndex(index); err != nil {
		idx.SyncStatus = "error"
		idx.SyncError = fmt.Sprintf("index validation failed: %v", err)
		s.db.Save(idx)
		return fmt.Errorf("index validation failed: %w", err)
	}

	// Process index and update database
	result, err := s.processIndex(ctx, index, idx.SourceURL)
	if err != nil {
		idx.SyncStatus = "error"
		idx.SyncError = err.Error()
		s.db.Save(idx)
		return fmt.Errorf("failed to process index: %w", err)
	}

	// Update index metadata
	idx.SyncStatus = "success"
	idx.SyncError = ""
	idx.LastSynced = time.Now()
	idx.LastModified = metadata.LastModified
	idx.ETag = metadata.ETag
	idx.APIVersion = index.APIVersion
	idx.PluginCount = result.PluginsAdded + result.PluginsUpdated
	s.db.Save(idx)

	duration := time.Since(startTime)
	log.Info().
		Str("source_url", idx.SourceURL).
		Int("added", result.PluginsAdded).
		Int("updated", result.PluginsUpdated).
		Dur("duration", duration).
		Msg("Marketplace index synced successfully")

	return nil
}

// processIndex processes the marketplace index and updates database
func (s *MarketplaceService) processIndex(ctx context.Context, index *marketplace.MarketplaceIndex, sourceURL string) (*marketplace.SyncResult, error) {
	result := &marketplace.SyncResult{
		Success:    true,
		LastSynced: time.Now(),
	}

	// Track existing plugins for this source
	existingPlugins := make(map[string]map[string]bool) // pluginID -> version -> exists

	// Process each plugin and its versions
	for pluginID, versions := range index.Plugins {
		existingPlugins[pluginID] = make(map[string]bool)

		for _, indexedPlugin := range versions {
			// Check if plugin version already exists
			var existing models.MarketplacePlugin
			err := s.db.Where("plugin_id = ? AND version = ? AND synced_from_url = ?",
				indexedPlugin.ID, indexedPlugin.Version, sourceURL).First(&existing).Error

			isNew := err == gorm.ErrRecordNotFound

			// Convert indexed plugin to database model
			dbPlugin := s.indexedPluginToModel(&indexedPlugin, sourceURL)

			// Marshal maintainers to JSON (simplified - using empty array for now)
			maintainersJSON, _ := json.Marshal([]map[string]string{})
			dbPlugin.Maintainers = string(maintainersJSON)

			if isNew {
				// Create new entry
				if err := s.db.Create(dbPlugin).Error; err != nil {
					log.Error().
						Err(err).
						Str("plugin_id", indexedPlugin.ID).
						Str("version", indexedPlugin.Version).
						Msg("Failed to create marketplace plugin")
					result.Errors = append(result.Errors, fmt.Sprintf("Failed to create %s@%s: %v", indexedPlugin.ID, indexedPlugin.Version, err))
					result.Success = false
					continue
				}
				result.PluginsAdded++
			} else {
				// Update existing entry
				dbPlugin.ID = existing.ID
				if err := s.db.Save(dbPlugin).Error; err != nil {
					log.Error().
						Err(err).
						Str("plugin_id", indexedPlugin.ID).
						Str("version", indexedPlugin.Version).
						Msg("Failed to update marketplace plugin")
					result.Errors = append(result.Errors, fmt.Sprintf("Failed to update %s@%s: %v", indexedPlugin.ID, indexedPlugin.Version, err))
					result.Success = false
					continue
				}
				result.PluginsUpdated++
			}

			existingPlugins[pluginID][indexedPlugin.Version] = true
		}
	}

	return result, nil
}

// indexedPluginToModel converts an IndexedPlugin to MarketplacePlugin model
func (s *MarketplaceService) indexedPluginToModel(indexed *marketplace.IndexedPlugin, sourceURL string) *models.MarketplacePlugin {
	return &models.MarketplacePlugin{
		PluginID:         indexed.ID,
		Version:          indexed.Version,
		Name:             indexed.Name,
		Description:      indexed.Description,
		Category:         indexed.Category,
		Maturity:         indexed.Maturity,
		Publisher:        indexed.Publisher,
		OCIRegistry:      indexed.OCIRegistry,
		OCIRepository:    indexed.OCIRepository,
		OCITag:           indexed.OCITag,
		OCIDigest:        indexed.OCIDigest,
		OCIPlatforms:     indexed.OCIPlatform,
		IconURL:          indexed.Icon,
		PrimaryHook:      indexed.PrimaryHook,
		MinStudioVersion: indexed.MinStudioVersion,
		PluginCreatedAt:  indexed.CreatedAt,
		PluginUpdatedAt:  indexed.UpdatedAt,
		Deprecated:       indexed.Deprecated,
		RequiredServices: indexed.RequiredServices,
		RequiredKV:       indexed.RequiredKV,
		RequiredRPC:      indexed.RequiredRPC,
		RequiredUI:       indexed.RequiredUI,
		LastSynced:       time.Now(),
		SyncedFromURL:    sourceURL,
	}
}

// InstallFromMarketplace installs a plugin from the marketplace
func (s *MarketplaceService) InstallFromMarketplace(ctx context.Context, req *marketplace.InstallRequest) (*marketplace.InstallResponse, error) {
	log.Info().
		Str("plugin_id", req.PluginID).
		Str("version", req.Version).
		Msg("Installing plugin from marketplace")

	// Get marketplace plugin info
	var marketplacePlugin models.MarketplacePlugin
	if err := marketplacePlugin.GetByPluginIDAndVersion(s.db, req.PluginID, req.Version); err != nil {
		return nil, fmt.Errorf("plugin not found in marketplace: %w", err)
	}

	// Build OCI reference
	ociRef := fmt.Sprintf("oci://%s/%s@%s",
		marketplacePlugin.OCIRegistry,
		marketplacePlugin.OCIRepository,
		marketplacePlugin.OCIDigest,
	)

	// Use existing PluginService to create and load the plugin
	pluginName := req.Name
	if pluginName == "" {
		pluginName = marketplacePlugin.Name
	}

	// Create plugin in database
	plugin := &models.Plugin{
		Name:         pluginName,
		Description:  marketplacePlugin.Description,
		Command:      ociRef,
		OCIReference: ociRef,
		HookType:     marketplacePlugin.PrimaryHook,
		HookTypes:    marketplacePlugin.Hooks, // Set all hook types from marketplace
		Config:       req.Config,
		IsActive:     true,
		Namespace:    req.Namespace,
	}

	// Set service scopes from marketplace manifest
	if len(marketplacePlugin.RequiredServices) > 0 {
		plugin.ServiceScopes = marketplacePlugin.RequiredServices
		// If user accepted scopes, authorize immediately
		if len(req.AcceptedScopes) > 0 {
			plugin.ServiceAccessAuthorized = true
		}
	}

	if err := plugin.Create(s.db); err != nil {
		return nil, fmt.Errorf("failed to create plugin: %w", err)
	}

	// Download and load the plugin via OCI client
	// Note: This is handled by the plugin manager when the plugin is loaded

	// Track installed version
	installedVersion := &models.InstalledPluginVersion{
		PluginID:            plugin.ID,
		MarketplacePluginID: marketplacePlugin.PluginID,
		InstalledVersion:    marketplacePlugin.Version,
		AvailableVersion:    marketplacePlugin.Version,
		UpdateAvailable:     false,
		AutoUpdate:          req.AutoUpdate,
		LastChecked:         time.Now(),
		InstallSource:       "marketplace",
	}

	if err := s.db.Create(installedVersion).Error; err != nil {
		log.Error().Err(err).Msg("Failed to create installed version tracking")
		// Non-fatal - continue
	}

	return &marketplace.InstallResponse{
		Success:          true,
		Message:          fmt.Sprintf("Plugin %s@%s installed successfully", marketplacePlugin.Name, marketplacePlugin.Version),
		PluginID:         plugin.ID,
		MarketplaceID:    marketplacePlugin.PluginID,
		Version:          marketplacePlugin.Version,
		InstalledAt:      time.Now(),
		RequiresApproval: len(marketplacePlugin.RequiredServices) > 0 && !plugin.ServiceAccessAuthorized,
	}, nil
}

// CheckForUpdates checks for available updates for installed plugins
func (s *MarketplaceService) CheckForUpdates(ctx context.Context) error {
	log.Debug().Msg("Checking for plugin updates")

	// Get all installed plugin versions
	var installedVersions []models.InstalledPluginVersion
	if err := s.db.Preload("Plugin").Find(&installedVersions).Error; err != nil {
		return fmt.Errorf("failed to get installed versions: %w", err)
	}

	for _, installed := range installedVersions {
		if installed.MarketplacePluginID == "" {
			continue // Not from marketplace
		}

		// Get latest version from marketplace
		var latestPlugin models.MarketplacePlugin
		if err := latestPlugin.GetLatestVersion(s.db, installed.MarketplacePluginID); err != nil {
			if err == gorm.ErrRecordNotFound {
				continue // Plugin removed from marketplace
			}
			log.Error().Err(err).Str("plugin_id", installed.MarketplacePluginID).Msg("Failed to get latest version")
			continue
		}

		// Compare versions
		updateAvailable := latestPlugin.Version != installed.InstalledVersion

		// Update tracking
		installed.AvailableVersion = latestPlugin.Version
		installed.UpdateAvailable = updateAvailable
		installed.LastChecked = time.Now()

		if err := s.db.Save(&installed).Error; err != nil {
			log.Error().Err(err).Uint("id", installed.ID).Msg("Failed to update version tracking")
		}

		if updateAvailable {
			log.Info().
				Str("plugin", installed.Plugin.Name).
				Str("installed", installed.InstalledVersion).
				Str("available", latestPlugin.Version).
				Msg("Update available for plugin")
		}
	}

	return nil
}

// SearchPlugins searches marketplace plugins
func (s *MarketplaceService) SearchPlugins(filters *marketplace.SearchFilters) ([]*models.MarketplacePlugin, int64, int, error) {
	return models.ListMarketplacePlugins(
		s.db,
		filters.PageSize,
		filters.PageNumber,
		filters.Category,
		filters.Publisher,
		filters.Maturity,
		filters.Query,
		filters.IncludeDeprecated,
	)
}

// GetPlugin gets a specific marketplace plugin
func (s *MarketplaceService) GetPlugin(pluginID, version string) (*models.MarketplacePlugin, error) {
	var plugin models.MarketplacePlugin
	err := plugin.GetByPluginIDAndVersion(s.db, pluginID, version)
	return &plugin, err
}

// GetPluginVersions gets all versions of a marketplace plugin
func (s *MarketplaceService) GetPluginVersions(pluginID string) ([]*models.MarketplacePlugin, error) {
	return models.GetAllPluginVersions(s.db, pluginID)
}

// GetAvailableUpdates gets all plugins with available updates
func (s *MarketplaceService) GetAvailableUpdates() (*marketplace.UpdateCheckResponse, error) {
	versionsWithUpdates, err := models.CheckForUpdates(s.db)
	if err != nil {
		return nil, err
	}

	response := &marketplace.UpdateCheckResponse{
		UpdatesAvailable: len(versionsWithUpdates),
		Plugins:          make([]marketplace.PluginUpdateInfo, 0, len(versionsWithUpdates)),
	}

	for _, v := range versionsWithUpdates {
		if v.Plugin == nil {
			continue
		}

		response.Plugins = append(response.Plugins, marketplace.PluginUpdateInfo{
			PluginID:         v.PluginID,
			Name:             v.Plugin.Name,
			MarketplaceID:    v.MarketplacePluginID,
			InstalledVersion: v.InstalledVersion,
			AvailableVersion: v.AvailableVersion,
			// TODO: Fetch changelog from marketplace
		})
	}

	return response, nil
}

// EnsureCacheDirectory ensures the marketplace cache directory exists
func (s *MarketplaceService) EnsureCacheDirectory() error {
	if s.cacheDir == "" {
		s.cacheDir = filepath.Join(".", ".marketplace-cache")
	}
	// Directory creation is handled by ociplugins storage
	return nil
}

// GetDB returns the database connection (for handlers)
func (s *MarketplaceService) GetDB() *gorm.DB {
	return s.db
}
