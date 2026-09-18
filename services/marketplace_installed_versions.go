package services

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/TykTechnologies/midsommar/v2/config"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/ociplugins"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// ociRepoKey identifies an OCI repository regardless of tag or digest.
func ociRepoKey(registry, repository string) string {
	return strings.ToLower(registry) + "/" + repository
}

// marketplaceVersionsForRepo returns the marketplace versions published from
// the same OCI repository as the installed command, narrowed to one marketplace
// plugin ID. Matching on the repository (not on the plugin ID alone) means a
// second marketplace source that reuses a plugin ID cannot offer its own
// artifact as an upgrade for someone else's plugin.
func marketplaceVersionsForRepo(candidates []*models.MarketplacePlugin, manifestID string) (string, []*models.MarketplacePlugin) {
	if len(candidates) == 0 {
		return "", nil
	}
	marketplaceID := candidates[0].PluginID
	if manifestID != "" {
		for _, c := range candidates {
			if c.PluginID == manifestID {
				marketplaceID = manifestID
				break
			}
		}
	}
	versions := make([]*models.MarketplacePlugin, 0, len(candidates))
	for _, c := range candidates {
		if c.PluginID == marketplaceID {
			versions = append(versions, c)
		}
	}
	return marketplaceID, versions
}

// resolveInstalledVersion works out which marketplace version an installed
// command is. The digest is authoritative. A tag only counts when exactly one
// version carries it (versions of one plugin can share a tag). After that the
// manifest the plugin reported is the best remaining signal: it covers
// versions that were republished under a new digest or removed from the index.
func resolveInstalledVersion(ref *ociplugins.OCIReference, versions []*models.MarketplacePlugin, manifestVersion, registeredVersion string) string {
	if ref.Digest != "" {
		for _, v := range versions {
			if v.OCIDigest != "" && v.OCIDigest == ref.Digest {
				return v.Version
			}
		}
	} else if ref.Tag != "" {
		var tagged []*models.MarketplacePlugin
		for _, v := range versions {
			if v.OCITag == ref.Tag {
				tagged = append(tagged, v)
			}
		}
		if len(tagged) == 1 {
			return tagged[0].Version
		}
	}
	if manifestVersion != "" {
		return manifestVersion
	}
	return registeredVersion
}

// latestUpgradeCandidate picks the highest version an install could move to:
// deprecated versions are never offered, and enterprise-only versions are only
// offered on the Enterprise edition.
func latestUpgradeCandidate(versions []*models.MarketplacePlugin) *models.MarketplacePlugin {
	var best *models.MarketplacePlugin
	for _, v := range versions {
		if v.Deprecated || (v.EnterpriseOnly && !config.IsEnterprise()) || v.OCIReference() == "" {
			continue
		}
		if best == nil || models.IsNewerVersion(v.Version, best.Version) {
			best = v
		}
	}
	return best
}

// ReconcileInstalledVersions is the single writer of installed_plugin_versions.
// It links installed OCI plugins back to their marketplace entry by OCI
// repository, records the installed and latest available versions, and removes
// tracking rows for plugins that no longer match anything (deleted, moved to a
// non-marketplace command). With no IDs it reconciles every plugin; with IDs
// it touches only those.
//
// Because the link is derived from the command, installs made through the
// creation wizard - and installs that predate this tracking - are picked up
// without any install-time bookkeeping.
func (s *MarketplaceService) ReconcileInstalledVersions(ctx context.Context, pluginIDs ...uint) error {
	db := s.db.WithContext(ctx)

	pluginQuery := db.Model(&models.Plugin{}).Where("command LIKE ?", "oci://%")
	if len(pluginIDs) > 0 {
		pluginQuery = pluginQuery.Where("id IN ?", pluginIDs)
	}
	var plugins []models.Plugin
	if err := pluginQuery.Find(&plugins).Error; err != nil {
		return fmt.Errorf("failed to list OCI plugins: %w", err)
	}

	var marketplacePlugins []*models.MarketplacePlugin
	if err := db.Find(&marketplacePlugins).Error; err != nil {
		return fmt.Errorf("failed to list marketplace plugins: %w", err)
	}
	byRepo := make(map[string][]*models.MarketplacePlugin)
	for _, mp := range marketplacePlugins {
		key := ociRepoKey(mp.OCIRegistry, mp.OCIRepository)
		byRepo[key] = append(byRepo[key], mp)
	}

	ids := make([]uint, 0, len(plugins))
	for i := range plugins {
		ids = append(ids, plugins[i].ID)
	}

	registeredVersions := make(map[uint]string, len(ids))
	if len(ids) > 0 {
		var registered []models.RegisteredPlugin
		if err := db.Select("plugin_id", "manifest_version").Where("plugin_id IN ?", ids).Find(&registered).Error; err != nil {
			return fmt.Errorf("failed to list registered plugins: %w", err)
		}
		for _, r := range registered {
			registeredVersions[r.PluginID] = r.ManifestVersion
		}
	}

	trackingQuery := db.Model(&models.InstalledPluginVersion{})
	if len(pluginIDs) > 0 {
		trackingQuery = trackingQuery.Where("plugin_id IN ?", pluginIDs)
	}
	var trackingRows []*models.InstalledPluginVersion
	if err := trackingQuery.Find(&trackingRows).Error; err != nil {
		return fmt.Errorf("failed to list installed plugin versions: %w", err)
	}
	tracking := make(map[uint]*models.InstalledPluginVersion, len(trackingRows))
	for _, row := range trackingRows {
		tracking[row.PluginID] = row
	}

	now := time.Now()
	matched := make(map[uint]bool, len(plugins))

	for i := range plugins {
		plugin := &plugins[i]
		ref, _, err := ociplugins.ParseOCICommand(plugin.Command)
		if err != nil {
			log.Debug().Err(err).Uint("plugin_id", plugin.ID).Msg("Skipping plugin with unparseable OCI command")
			continue
		}

		manifestID, _ := plugin.Manifest["id"].(string)
		manifestVersion, _ := plugin.Manifest["version"].(string)

		marketplaceID, versions := marketplaceVersionsForRepo(byRepo[ociRepoKey(ref.Registry, ref.Repository)], manifestID)
		if marketplaceID == "" {
			continue
		}
		matched[plugin.ID] = true

		installed := resolveInstalledVersion(ref, versions, manifestVersion, registeredVersions[plugin.ID])
		available := ""
		if candidate := latestUpgradeCandidate(versions); candidate != nil {
			available = candidate.Version
		}

		row := tracking[plugin.ID]
		if row == nil {
			row = &models.InstalledPluginVersion{PluginID: plugin.ID, InstallSource: models.InstallSourceMarketplace}
		}
		row.MarketplacePluginID = marketplaceID
		row.InstalledVersion = installed
		row.AvailableVersion = available
		row.UpdateAvailable = models.IsNewerVersion(available, installed)
		row.LastChecked = now

		// Save writes every column, so a cleared update_available is persisted too.
		if err := db.Omit("Plugin").Save(row).Error; err != nil {
			log.Error().Err(err).Uint("plugin_id", plugin.ID).Msg("Failed to save installed plugin version")
		}
	}

	for pluginID := range tracking {
		if matched[pluginID] {
			continue
		}
		if err := models.DeleteInstalledPluginVersion(db, pluginID); err != nil {
			log.Error().Err(err).Uint("plugin_id", pluginID).Msg("Failed to remove stale installed plugin version")
		}
	}

	return nil
}

// reconcileInstalledVersionsBestEffort is for callers where tracking is a side
// effect of another operation (create, update, delete of a plugin).
func (s *MarketplaceService) reconcileInstalledVersionsBestEffort(pluginIDs ...uint) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := s.ReconcileInstalledVersions(ctx, pluginIDs...); err != nil {
		log.Warn().Err(err).Uints("plugin_ids", pluginIDs).Msg("Failed to reconcile installed plugin versions")
	}
}

// OnPluginChanged re-links one plugin after its command may have changed.
func (s *MarketplaceService) OnPluginChanged(pluginID uint) {
	if s == nil {
		return
	}
	s.reconcileInstalledVersionsBestEffort(pluginID)
}

// UpgradeTargets returns the marketplace versions the given installed plugin
// can move to (any version from the same OCI repository and marketplace ID,
// newest first), along with its tracking row.
func (s *MarketplaceService) UpgradeTargets(plugin *models.Plugin) (*models.InstalledPluginVersion, []*models.MarketplacePlugin, error) {
	tracked, err := models.GetInstalledPluginVersions(s.db, []uint{plugin.ID})
	if err != nil {
		return nil, nil, err
	}
	row := tracked[plugin.ID]
	if row == nil {
		return nil, nil, gorm.ErrRecordNotFound
	}
	ref, _, err := ociplugins.ParseOCICommand(plugin.Command)
	if err != nil {
		return nil, nil, fmt.Errorf("plugin command is not an OCI reference: %w", err)
	}
	all, err := models.GetAllPluginVersions(s.db, row.MarketplacePluginID)
	if err != nil {
		return nil, nil, err
	}
	repoKey := ociRepoKey(ref.Registry, ref.Repository)
	versions := make([]*models.MarketplacePlugin, 0, len(all))
	for _, v := range all {
		if ociRepoKey(v.OCIRegistry, v.OCIRepository) == repoKey {
			versions = append(versions, v)
		}
	}
	return row, versions, nil
}
