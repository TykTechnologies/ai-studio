package api

import (
	"context"
	"net/http"
	"strconv"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/marketplace"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

// MarketplaceHandlers contains handlers for marketplace operations
type MarketplaceHandlers struct {
	marketplaceService *services.MarketplaceService
}

// NewMarketplaceHandlers creates new marketplace handlers
func NewMarketplaceHandlers(marketplaceService *services.MarketplaceService) *MarketplaceHandlers {
	return &MarketplaceHandlers{
		marketplaceService: marketplaceService,
	}
}

// ListPlugins returns paginated list of marketplace plugins
// GET /api/marketplace/plugins
func (h *MarketplaceHandlers) ListPlugins(c *gin.Context) {
	// Parse query parameters
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	pageNumber, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	category := c.Query("category")
	publisher := c.Query("publisher")
	maturity := c.Query("maturity")
	search := c.Query("search")
	includeDeprecated := c.Query("include_deprecated") == "true"

	// Validate pagination
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	if pageNumber < 1 {
		pageNumber = 1
	}

	filters := &marketplace.SearchFilters{
		Query:             search,
		Category:          category,
		Publisher:         publisher,
		Maturity:          maturity,
		IncludeDeprecated: includeDeprecated,
		PageSize:          pageSize,
		PageNumber:        pageNumber,
	}

	plugins, total, totalPages, err := h.marketplaceService.SearchPlugins(filters)
	if err != nil {
		log.Error().Err(err).Msg("Failed to list marketplace plugins")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to list marketplace plugins",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"plugins":     plugins,
		"total":       total,
		"total_pages": totalPages,
		"page":        pageNumber,
		"page_size":   pageSize,
	})
}

// GetPlugin returns details for a specific marketplace plugin
// GET /api/marketplace/plugins/:id
func (h *MarketplaceHandlers) GetPlugin(c *gin.Context) {
	pluginID := c.Param("id")
	version := c.DefaultQuery("version", "")

	if version == "" {
		// Get latest version
		var plugin models.MarketplacePlugin
		if err := plugin.GetLatestVersion(h.marketplaceService.GetDB(), pluginID); err != nil {
			log.Error().Err(err).Str("plugin_id", pluginID).Msg("Failed to get latest plugin version")
			c.JSON(http.StatusNotFound, gin.H{
				"error": "Plugin not found",
			})
			return
		}

		c.JSON(http.StatusOK, plugin)
		return
	}

	// Get specific version
	plugin, err := h.marketplaceService.GetPlugin(pluginID, version)
	if err != nil {
		log.Error().Err(err).Str("plugin_id", pluginID).Str("version", version).Msg("Failed to get plugin")
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Plugin not found",
		})
		return
	}

	c.JSON(http.StatusOK, plugin)
}

// GetPluginVersions returns all versions of a marketplace plugin
// GET /api/marketplace/plugins/:id/versions
func (h *MarketplaceHandlers) GetPluginVersions(c *gin.Context) {
	pluginID := c.Param("id")

	versions, err := h.marketplaceService.GetPluginVersions(pluginID)
	if err != nil {
		log.Error().Err(err).Str("plugin_id", pluginID).Msg("Failed to get plugin versions")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to get plugin versions",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"plugin_id": pluginID,
		"versions":  versions,
	})
}

// GetInstallMetadata returns metadata needed to pre-populate the plugin creation wizard
// GET /api/marketplace/plugins/:id/install-metadata
func (h *MarketplaceHandlers) GetInstallMetadata(c *gin.Context) {
	pluginID := c.Param("id")
	version := c.Query("version")

	// Get plugin from marketplace
	var plugin *models.MarketplacePlugin
	var err error

	if version == "" {
		// Get latest version
		var p models.MarketplacePlugin
		if err := p.GetLatestVersion(h.marketplaceService.GetDB(), pluginID); err != nil {
			log.Error().Err(err).Str("plugin_id", pluginID).Msg("Failed to get latest plugin version")
			c.JSON(http.StatusNotFound, gin.H{
				"error": "Plugin not found",
			})
			return
		}
		plugin = &p
	} else {
		plugin, err = h.marketplaceService.GetPlugin(pluginID, version)
		if err != nil {
			log.Error().Err(err).Str("plugin_id", pluginID).Str("version", version).Msg("Failed to get plugin")
			c.JSON(http.StatusNotFound, gin.H{
				"error": "Plugin not found",
			})
			return
		}
	}

	// Build OCI reference
	ociReference := ""
	if plugin.OCIDigest != "" {
		ociReference = "oci://" + plugin.OCIRegistry + "/" + plugin.OCIRepository + "@" + plugin.OCIDigest
	} else if plugin.OCITag != "" {
		ociReference = "oci://" + plugin.OCIRegistry + "/" + plugin.OCIRepository + ":" + plugin.OCITag
	}

	// Determine if this is an agent plugin
	isAgent := plugin.PrimaryHook == "agent" || plugin.PrimaryHook == models.HookTypeAgent

	// Build response with wizard pre-fill data
	response := gin.H{
		"plugin_id":    plugin.PluginID,
		"version":      plugin.Version,
		"name":         plugin.Name,
		"description":  plugin.Description,
		"oci_reference": ociReference,
		"hook_type":    plugin.PrimaryHook,
		"hook_types":   plugin.Hooks,
		"required_scopes": plugin.RequiredServices,
		"config_schema_url": plugin.ConfigSchemaURL,
		"is_agent":     isAgent,
		"category":     plugin.Category,
		"publisher":    plugin.Publisher,
		"maturity":     plugin.Maturity,
		"icon_url":     plugin.IconURL,
		"platforms":    plugin.OCIPlatforms,

		// Additional metadata for display
		"documentation_url": plugin.DocumentationURL,
		"repository_url":    plugin.RepositoryURL,
		"support_url":       plugin.SupportURL,
		"license":           plugin.License,
	}

	c.JSON(http.StatusOK, response)
}

// GetAvailableUpdates returns plugins with available updates
// GET /api/marketplace/updates
func (h *MarketplaceHandlers) GetAvailableUpdates(c *gin.Context) {
	updates, err := h.marketplaceService.GetAvailableUpdates()
	if err != nil {
		log.Error().Err(err).Msg("Failed to get available updates")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to get available updates",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, updates)
}

// SyncMarketplace triggers a manual marketplace sync
// POST /api/marketplace/sync
func (h *MarketplaceHandlers) SyncMarketplace(c *gin.Context) {
	log.Info().Msg("Manual marketplace sync triggered")

	// Run sync in background with a new context (not tied to HTTP request)
	go func() {
		ctx := context.Background()
		if err := h.marketplaceService.SyncAll(ctx); err != nil {
			log.Error().Err(err).Msg("Manual marketplace sync failed")
		} else {
			log.Info().Msg("Manual marketplace sync completed successfully")
		}
	}()

	c.JSON(http.StatusAccepted, gin.H{
		"message": "Marketplace sync initiated",
		"status":  "in_progress",
	})
}

// GetSyncStatus returns the current sync status
// GET /api/marketplace/sync-status
func (h *MarketplaceHandlers) GetSyncStatus(c *gin.Context) {
	// Get all indexes and their status
	indexes, err := models.GetAllActiveMarketplaceIndexes(h.marketplaceService.GetDB())
	if err != nil {
		log.Error().Err(err).Msg("Failed to get marketplace indexes")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to get sync status",
			"details": err.Error(),
		})
		return
	}

	indexStatuses := make([]gin.H, 0, len(indexes))
	for _, idx := range indexes {
		indexStatuses = append(indexStatuses, gin.H{
			"source_url":   idx.SourceURL,
			"is_default":   idx.IsDefault,
			"last_synced":  idx.LastSynced,
			"plugin_count": idx.PluginCount,
			"sync_status":  idx.SyncStatus,
			"sync_error":   idx.SyncError,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"indexes": indexStatuses,
	})
}

// GetCategories returns available plugin categories
// GET /api/marketplace/categories
func (h *MarketplaceHandlers) GetCategories(c *gin.Context) {
	// Query distinct categories from marketplace plugins
	var categories []string
	if err := h.marketplaceService.GetDB().
		Model(&models.MarketplacePlugin{}).
		Distinct("category").
		Where("category != ''").
		Pluck("category", &categories).Error; err != nil {
		log.Error().Err(err).Msg("Failed to get categories")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to get categories",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"categories": categories,
	})
}

// GetPublishers returns available plugin publishers
// GET /api/marketplace/publishers
func (h *MarketplaceHandlers) GetPublishers(c *gin.Context) {
	// Query distinct publishers from marketplace plugins
	var publishers []string
	if err := h.marketplaceService.GetDB().
		Model(&models.MarketplacePlugin{}).
		Distinct("publisher").
		Where("publisher != ''").
		Pluck("publisher", &publishers).Error; err != nil {
		log.Error().Err(err).Msg("Failed to get publishers")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to get publishers",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"publishers": publishers,
	})
}

// GetStats returns marketplace statistics
// GET /api/marketplace/stats
func (h *MarketplaceHandlers) GetStats(c *gin.Context) {
	var totalPlugins int64
	var totalVersions int64
	var deprecatedCount int64

	db := h.marketplaceService.GetDB()

	// Count unique plugins
	db.Model(&models.MarketplacePlugin{}).
		Distinct("plugin_id").
		Count(&totalPlugins)

	// Count total versions
	db.Model(&models.MarketplacePlugin{}).
		Count(&totalVersions)

	// Count deprecated plugins
	db.Model(&models.MarketplacePlugin{}).
		Where("deprecated = ?", true).
		Count(&deprecatedCount)

	// Get category breakdown
	type CategoryCount struct {
		Category string
		Count    int64
	}
	var categoryBreakdown []CategoryCount
	db.Model(&models.MarketplacePlugin{}).
		Select("category, COUNT(DISTINCT plugin_id) as count").
		Group("category").
		Scan(&categoryBreakdown)

	// Get publisher breakdown
	type PublisherCount struct {
		Publisher string
		Count     int64
	}
	var publisherBreakdown []PublisherCount
	db.Model(&models.MarketplacePlugin{}).
		Select("publisher, COUNT(DISTINCT plugin_id) as count").
		Group("publisher").
		Scan(&publisherBreakdown)

	c.JSON(http.StatusOK, gin.H{
		"total_plugins":       totalPlugins,
		"total_versions":      totalVersions,
		"deprecated_plugins":  deprecatedCount,
		"category_breakdown":  categoryBreakdown,
		"publisher_breakdown": publisherBreakdown,
	})
}
