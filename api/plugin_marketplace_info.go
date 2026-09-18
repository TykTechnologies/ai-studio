package api

import (
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/rs/zerolog/log"
)

// PluginMarketplaceInfo links an installed plugin back to the marketplace
// entry it came from. It is served on the plugin API (not only on the
// marketplace API) so anyone who can read plugins can see that an update exists.
type PluginMarketplaceInfo struct {
	MarketplaceID    string `json:"marketplace_id"`
	InstalledVersion string `json:"installed_version"`
	AvailableVersion string `json:"available_version"`
	UpdateAvailable  bool   `json:"update_available"`
}

func marketplaceInfoFromTracking(row *models.InstalledPluginVersion) *PluginMarketplaceInfo {
	if row == nil {
		return nil
	}
	return &PluginMarketplaceInfo{
		MarketplaceID:    row.MarketplacePluginID,
		InstalledVersion: row.InstalledVersion,
		AvailableVersion: row.AvailableVersion,
		UpdateAvailable:  row.UpdateAvailable,
	}
}

// pluginDisplayVersion is the version to show for an installed plugin: what the
// marketplace link resolved, else what the plugin's own manifest reports.
func pluginDisplayVersion(plugin *models.Plugin, row *models.InstalledPluginVersion) string {
	if row != nil && row.InstalledVersion != "" {
		return row.InstalledVersion
	}
	version, _ := plugin.Manifest["version"].(string)
	return version
}

// decoratePluginResponse adds the version and marketplace link to a serialized plugin.
func decoratePluginResponse(response *PluginResponse, plugin *models.Plugin, row *models.InstalledPluginVersion) {
	response.Version = pluginDisplayVersion(plugin, row)
	response.Marketplace = marketplaceInfoFromTracking(row)
}

// serializePluginWithMarketplace serializes one plugin including its marketplace link.
func (a *API) serializePluginWithMarketplace(plugin *models.Plugin) PluginResponse {
	response := serializePlugin(plugin)
	tracked, err := models.GetInstalledPluginVersions(a.service.DB, []uint{plugin.ID})
	if err != nil {
		log.Warn().Err(err).Uint("plugin_id", plugin.ID).Msg("Failed to load marketplace link for plugin")
	}
	decoratePluginResponse(&response, plugin, tracked[plugin.ID])
	return response
}

// notifyMarketplacePluginChanged re-links a plugin to its marketplace entry
// after a create or a command change. A no-op when the marketplace is off.
func (a *API) notifyMarketplacePluginChanged(pluginID uint) {
	if a.service.MarketplaceService != nil {
		a.service.MarketplaceService.OnPluginChanged(pluginID)
	}
}
