package models

import (
	"sort"
	"time"

	"github.com/Masterminds/semver/v3"
	"gorm.io/gorm"
)

// sortPluginsBySemverDesc sorts marketplace plugins by semantic version descending (highest first).
// Entries with invalid semver strings are sorted after valid ones.
func sortPluginsBySemverDesc(plugins []*MarketplacePlugin) {
	sort.Slice(plugins, func(i, j int) bool {
		vi, erri := semver.NewVersion(plugins[i].Version)
		vj, errj := semver.NewVersion(plugins[j].Version)
		if erri != nil && errj != nil {
			return plugins[i].Version > plugins[j].Version
		}
		if erri != nil {
			return false
		}
		if errj != nil {
			return true
		}
		return vi.GreaterThan(vj)
	})
}

// MarketplacePlugin represents a cached plugin entry from the marketplace index
type MarketplacePlugin struct {
	gorm.Model
	ID              uint      `json:"id" gorm:"primaryKey"`
	PluginID        string    `json:"plugin_id" gorm:"uniqueIndex:idx_marketplace_plugin_version_source;not null;size:255"` // e.g. "com.tyk.echo-agent"
	Version         string    `json:"version" gorm:"uniqueIndex:idx_marketplace_plugin_version_source;not null;size:100"`
	Name            string    `json:"name" gorm:"not null;size:255"`
	Description     string    `json:"description" gorm:"type:text"`
	Category        string    `json:"category" gorm:"size:100;index:idx_marketplace_category"`
	Keywords        []string  `json:"keywords" gorm:"serializer:json"`
	Maturity        string    `json:"maturity" gorm:"size:50"` // alpha, beta, stable
	Publisher       string    `json:"publisher" gorm:"size:100;index:idx_marketplace_publisher"`
	License         string    `json:"license" gorm:"size:100"`

	// OCI Distribution
	OCIRegistry     string    `json:"oci_registry" gorm:"size:255"`
	OCIRepository   string    `json:"oci_repository" gorm:"size:500"`
	OCITag          string    `json:"oci_tag" gorm:"size:100"`
	OCIDigest       string    `json:"oci_digest" gorm:"size:255;index:idx_marketplace_digest"`
	OCIPlatforms    []string  `json:"oci_platforms" gorm:"serializer:json"`

	// Links
	IconURL         string    `json:"icon_url" gorm:"size:500"`
	DocumentationURL string   `json:"documentation_url" gorm:"size:500"`
	RepositoryURL   string    `json:"repository_url" gorm:"size:500"`
	SupportURL      string    `json:"support_url" gorm:"size:500"`
	HomepageURL     string    `json:"homepage_url" gorm:"size:500"`
	IssuesURL       string    `json:"issues_url" gorm:"size:500"`
	Screenshots     []string  `json:"screenshots" gorm:"serializer:json"`

	// Capabilities
	PrimaryHook     string    `json:"primary_hook" gorm:"size:50"`
	Hooks           []string  `json:"hooks" gorm:"serializer:json"`

	// Requirements
	MinStudioVersion string   `json:"min_studio_version" gorm:"size:50"`
	APIVersions     []string  `json:"api_versions" gorm:"serializer:json"`
	Dependencies    []string  `json:"dependencies" gorm:"serializer:json"`

	// Permissions
	RequiredServices []string `json:"required_services" gorm:"serializer:json"`
	RequiredKV      []string  `json:"required_kv" gorm:"serializer:json"`
	RequiredRPC     []string  `json:"required_rpc" gorm:"serializer:json"`
	RequiredUI      []string  `json:"required_ui" gorm:"serializer:json"`

	// Config Schema
	ConfigSchemaURL string    `json:"config_schema_url" gorm:"size:500"`

	// Verification
	AttestationEnabled bool   `json:"attestation_enabled" gorm:"default:false"`
	AttestationURL    string  `json:"attestation_url" gorm:"size:500"`

	// Maintainers (JSON array)
	Maintainers     string    `json:"maintainers" gorm:"type:text"` // JSON array of {name, email, organization}

	// Metadata
	PluginCreatedAt time.Time `json:"plugin_created_at"`
	PluginUpdatedAt time.Time `json:"plugin_updated_at"`
	Deprecated      bool      `json:"deprecated" gorm:"default:false;index:idx_marketplace_deprecated"`
	DeprecatedMessage string  `json:"deprecated_message" gorm:"type:text"`
	ReplacementPlugin string  `json:"replacement_plugin" gorm:"size:255"`

	// Enterprise
	EnterpriseOnly  bool      `json:"enterprise_only" gorm:"default:false;index:idx_marketplace_enterprise"`

	// Cache info
	LastSynced      time.Time `json:"last_synced"`
	SyncedFromURL   string    `json:"synced_from_url" gorm:"uniqueIndex:idx_marketplace_plugin_version_source;size:500"`

	// Full manifest data (for reference)
	ManifestData    string    `json:"manifest_data" gorm:"type:text"` // Full YAML/JSON manifest

	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// TableName returns the table name for MarketplacePlugin
func (MarketplacePlugin) TableName() string {
	return "marketplace_plugins"
}

// MarketplaceIndex represents the cached marketplace index metadata
type MarketplaceIndex struct {
	gorm.Model
	ID              uint      `json:"id" gorm:"primaryKey"`
	SourceURL       string    `json:"source_url" gorm:"uniqueIndex;not null;size:500"` // URL of index.yaml
	APIVersion      string    `json:"api_version" gorm:"size:50"`
	LastSynced      time.Time `json:"last_synced"`
	LastModified    time.Time `json:"last_modified"` // From HTTP Last-Modified header
	ETag            string    `json:"etag" gorm:"size:255"` // From HTTP ETag header
	PluginCount     int       `json:"plugin_count"`
	SyncStatus      string    `json:"sync_status" gorm:"size:50"` // success, error, in_progress
	SyncError       string    `json:"sync_error" gorm:"type:text"`
	IsDefault       bool      `json:"is_default" gorm:"default:false"` // Is this the default Tyk marketplace
	IsActive        bool      `json:"is_active" gorm:"default:true;index:idx_marketplace_index_active"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// TableName returns the table name for MarketplaceIndex
func (MarketplaceIndex) TableName() string {
	return "marketplace_indexes"
}

// InstalledPluginVersion tracks installed plugins and their available updates
type InstalledPluginVersion struct {
	gorm.Model
	ID                  uint      `json:"id" gorm:"primaryKey"`
	PluginID            uint      `json:"plugin_id" gorm:"uniqueIndex;not null"` // References plugins.id
	MarketplacePluginID string    `json:"marketplace_plugin_id" gorm:"size:255;index:idx_installed_marketplace_id"` // e.g. "com.tyk.echo-agent"
	InstalledVersion    string    `json:"installed_version" gorm:"size:100"`
	AvailableVersion    string    `json:"available_version" gorm:"size:100"`
	UpdateAvailable     bool      `json:"update_available" gorm:"default:false;index:idx_update_available"`
	AutoUpdate          bool      `json:"auto_update" gorm:"default:false"`
	LastChecked         time.Time `json:"last_checked"`
	InstallSource       string    `json:"install_source" gorm:"size:100"` // marketplace, manual, oci
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`

	// Relationships
	Plugin              *Plugin            `json:"plugin,omitempty" gorm:"foreignKey:PluginID"`
}

// TableName returns the table name for InstalledPluginVersion
func (InstalledPluginVersion) TableName() string {
	return "installed_plugin_versions"
}

// MarketplaceConfig holds marketplace configuration
type MarketplaceConfig struct {
	gorm.Model
	ID                   uint      `json:"id" gorm:"primaryKey"`
	Key                  string    `json:"key" gorm:"uniqueIndex;not null;size:100"` // e.g. "sync_interval", "default_index_url"
	Value                string    `json:"value" gorm:"type:text"`
	Description          string    `json:"description" gorm:"type:text"`
	IsEditable           bool      `json:"is_editable" gorm:"default:true"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

// TableName returns the table name for MarketplaceConfig
func (MarketplaceConfig) TableName() string {
	return "marketplace_config"
}

// GetMarketplacePlugin retrieves a marketplace plugin by plugin_id and version
func (mp *MarketplacePlugin) GetByPluginIDAndVersion(db *gorm.DB, pluginID, version string) error {
	return db.Where("plugin_id = ? AND version = ?", pluginID, version).First(mp).Error
}

// GetLatestVersion retrieves the latest version of a marketplace plugin by semver
func (mp *MarketplacePlugin) GetLatestVersion(db *gorm.DB, pluginID string) error {
	var all []*MarketplacePlugin
	if err := db.Where("plugin_id = ?", pluginID).Find(&all).Error; err != nil {
		return err
	}
	if len(all) == 0 {
		return gorm.ErrRecordNotFound
	}
	sortPluginsBySemverDesc(all)
	*mp = *all[0]
	return nil
}

// ListMarketplacePlugins returns paginated marketplace plugins with filtering.
// Only the highest semver version of each plugin_id is returned.
func ListMarketplacePlugins(db *gorm.DB, pageSize, pageNumber int, category, publisher, maturity, search string, includeDeprecated bool) ([]*MarketplacePlugin, int64, int, error) {
	var allPlugins []*MarketplacePlugin

	query := db.Model(&MarketplacePlugin{})

	// Apply filters
	if category != "" && category != "all" {
		query = query.Where("category = ?", category)
	}
	if publisher != "" && publisher != "all" {
		query = query.Where("publisher = ?", publisher)
	}
	if maturity != "" && maturity != "all" {
		query = query.Where("maturity = ?", maturity)
	}
	if !includeDeprecated {
		query = query.Where("deprecated = ?", false)
	}
	if search != "" {
		searchPattern := "%" + search + "%"
		query = query.Where("name LIKE ? OR description LIKE ? OR plugin_id LIKE ?", searchPattern, searchPattern, searchPattern)
	}

	if err := query.Find(&allPlugins).Error; err != nil {
		return nil, 0, 0, err
	}

	// Sort by semver descending, then deduplicate to keep only the highest version per plugin_id
	sortPluginsBySemverDesc(allPlugins)
	seen := make(map[string]bool)
	var latest []*MarketplacePlugin
	for _, p := range allPlugins {
		if !seen[p.PluginID] {
			seen[p.PluginID] = true
			latest = append(latest, p)
		}
	}

	// Pagination
	totalCount := int64(len(latest))
	totalPages := 0
	if totalCount > 0 {
		totalPages = int(totalCount) / pageSize
		if int(totalCount)%pageSize != 0 {
			totalPages++
		}
	}

	offset := (pageNumber - 1) * pageSize
	end := offset + pageSize
	if offset > int(totalCount) {
		offset = int(totalCount)
	}
	if end > int(totalCount) {
		end = int(totalCount)
	}

	return latest[offset:end], totalCount, totalPages, nil
}

// GetAllVersions returns all versions of a specific plugin from marketplace, sorted by semver descending
func GetAllPluginVersions(db *gorm.DB, pluginID string) ([]*MarketplacePlugin, error) {
	var versions []*MarketplacePlugin
	if err := db.Where("plugin_id = ?", pluginID).Find(&versions).Error; err != nil {
		return nil, err
	}
	sortPluginsBySemverDesc(versions)
	return versions, nil
}

// GetMarketplaceIndex retrieves the active default marketplace index
func GetDefaultMarketplaceIndex(db *gorm.DB) (*MarketplaceIndex, error) {
	var index MarketplaceIndex
	err := db.Where("is_default = ? AND is_active = ?", true, true).First(&index).Error
	return &index, err
}

// GetAllActiveIndexes retrieves all active marketplace indexes
func GetAllActiveMarketplaceIndexes(db *gorm.DB) ([]*MarketplaceIndex, error) {
	var indexes []*MarketplaceIndex
	err := db.Where("is_active = ?", true).Order("is_default DESC, created_at DESC").Find(&indexes).Error
	return indexes, err
}

// CheckForUpdates checks if updates are available for installed plugins
func CheckForUpdates(db *gorm.DB) ([]*InstalledPluginVersion, error) {
	var versionsWithUpdates []*InstalledPluginVersion
	err := db.Where("update_available = ?", true).
		Preload("Plugin").
		Order("last_checked DESC").
		Find(&versionsWithUpdates).Error
	return versionsWithUpdates, err
}

// GetInstalledPluginVersion gets version tracking for an installed plugin
func GetInstalledPluginVersion(db *gorm.DB, pluginID uint) (*InstalledPluginVersion, error) {
	var version InstalledPluginVersion
	err := db.Where("plugin_id = ?", pluginID).
		Preload("Plugin").
		First(&version).Error
	return &version, err
}
