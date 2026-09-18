package models

import (
	"github.com/Masterminds/semver/v3"
	"gorm.io/gorm"
)

// Install sources recorded on InstalledPluginVersion.
const (
	InstallSourceMarketplace = "marketplace"
)

// OCIReference returns the oci:// command for this marketplace version. It is
// pinned to the digest when the index records one and falls back to the tag,
// so what gets installed is the artifact the marketplace entry describes.
func (mp *MarketplacePlugin) OCIReference() string {
	if mp.OCIRegistry == "" || mp.OCIRepository == "" {
		return ""
	}
	base := "oci://" + mp.OCIRegistry + "/" + mp.OCIRepository
	if mp.OCIDigest != "" {
		return base + "@" + mp.OCIDigest
	}
	if mp.OCITag != "" {
		return base + ":" + mp.OCITag
	}
	return ""
}

// IsNewerVersion reports whether candidate is a later version than installed.
// Both are compared as semver. An unparseable candidate is never newer; an
// unknown or unparseable installed version cannot be compared, so it is not
// reported as outdated either (the caller can still offer a version change).
func IsNewerVersion(candidate, installed string) bool {
	cv, err := semver.NewVersion(candidate)
	if err != nil {
		return false
	}
	iv, err := semver.NewVersion(installed)
	if err != nil {
		return false
	}
	return cv.GreaterThan(iv)
}

// GetInstalledPluginVersions returns the marketplace tracking rows for the
// given plugin IDs, keyed by plugin ID. Plugins that did not come from a
// marketplace have no entry.
func GetInstalledPluginVersions(db *gorm.DB, pluginIDs []uint) (map[uint]*InstalledPluginVersion, error) {
	result := make(map[uint]*InstalledPluginVersion, len(pluginIDs))
	if len(pluginIDs) == 0 {
		return result, nil
	}
	var rows []*InstalledPluginVersion
	if err := db.Where("plugin_id IN ?", pluginIDs).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		result[row.PluginID] = row
	}
	return result, nil
}

// ListInstalledPluginVersionsByMarketplaceIDs returns the tracking rows for the
// given marketplace plugin IDs, grouped by marketplace plugin ID.
func ListInstalledPluginVersionsByMarketplaceIDs(db *gorm.DB, marketplaceIDs []string) (map[string][]*InstalledPluginVersion, error) {
	result := make(map[string][]*InstalledPluginVersion, len(marketplaceIDs))
	if len(marketplaceIDs) == 0 {
		return result, nil
	}
	var rows []*InstalledPluginVersion
	if err := db.Where("marketplace_plugin_id IN ?", marketplaceIDs).Order("plugin_id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		result[row.MarketplacePluginID] = append(result[row.MarketplacePluginID], row)
	}
	return result, nil
}

// CountPluginUpdatesAvailable counts installed plugins with a newer marketplace version.
func CountPluginUpdatesAvailable(db *gorm.DB) (int64, error) {
	var count int64
	err := db.Model(&InstalledPluginVersion{}).Where("update_available = ?", true).Count(&count).Error
	return count, err
}

// DeleteInstalledPluginVersion removes the tracking row for a plugin. The row
// is hard-deleted: plugin_id carries a unique index, so a soft-deleted row
// would block tracking the same plugin again.
func DeleteInstalledPluginVersion(db *gorm.DB, pluginID uint) error {
	return db.Unscoped().Where("plugin_id = ?", pluginID).Delete(&InstalledPluginVersion{}).Error
}
