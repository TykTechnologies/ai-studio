package services

import (
	"context"
	"fmt"
	"sync"

	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/authz"
)

// Per-plugin permission resources.
//
// Every installed plugin with an administrator-facing surface (Studio pages,
// portal pages or resource types) contributes one resource to the permission
// catalogue, keyed by models.Plugin.PermissionKey ("plugin:<manifest id>"),
// with read, write and execute:
//
//   - read:    open the plugin's pages, call RPC methods the plugin declares
//              read-only, view its configuration
//   - write:   call any other RPC method, edit its configuration
//   - execute: reserved for plugin-declared side-effecting methods
//
// plugins:execute (the platform-level "call plugins" grant) implies every
// per-plugin permission (authz.Set.Has), so existing roles keep working; the
// per-plugin grants exist to narrow access to one plugin.
//
// The catalogue is in-memory, so it is rebuilt from the plugins table at
// boot (before the system roles are seeded) and kept in sync by the plugin
// service on create, update, delete and manifest registration. System roles
// are refreshed after every change so Viewer/Editor/Auditor reflect the
// catalogue without a restart.

// PluginBaseActions are the actions of a plugin's base resource.
var PluginBaseActions = []authz.Action{authz.ActionRead, authz.ActionWrite, authz.ActionExecute}

// pluginPermissionKeys remembers which key each plugin ID last registered so
// a key change (the manifest arriving after install) unregisters the old one.
var pluginPermissionKeys sync.Map // uint -> string

// PluginBaseResource builds the catalogue entry for a plugin.
func PluginBaseResource(plugin *models.Plugin) authz.Resource {
	key := plugin.PermissionKey()
	label := plugin.Name
	if label == "" {
		label = key
	}
	return authz.Resource{
		Key:         key,
		Label:       label,
		Group:       "Plugins",
		Actions:     PluginBaseActions,
		Plugin:      key,
		PluginLabel: label,
		Dynamic:     true,
		Description: fmt.Sprintf("Use the %s plugin: read opens its pages and shows its configuration, write calls its RPC methods and edits its configuration.", label),
	}
}

// PluginSubResource builds the catalogue entry for one declared or
// runtime-registered sub-resource of a plugin.
func PluginSubResource(plugin *models.Plugin, row models.PluginPermissionResource) authz.Resource {
	base := PluginBaseResource(plugin)
	actions := make([]authz.Action, 0, len(row.Actions))
	for _, a := range row.Actions {
		actions = append(actions, authz.Action(a))
	}
	return authz.Resource{
		Key:         base.Key + ":" + row.Key,
		Label:       row.Label,
		Group:       "Plugins",
		Actions:     actions,
		Sensitive:   row.Sensitive,
		Plugin:      base.Key,
		PluginLabel: base.PluginLabel,
		Dynamic:     true,
		Description: row.Description,
	}
}

// manifestPermissionRows converts the manifest's rbac.resources block into
// table rows (source manifest).
func manifestPermissionRows(plugin *models.Plugin) []models.PluginPermissionResource {
	block := plugin.ManifestRBAC()
	if block == nil {
		return nil
	}
	rows := make([]models.PluginPermissionResource, 0, len(block.Resources))
	for _, r := range block.Resources {
		rows = append(rows, models.PluginPermissionResource{
			Key:         r.Key,
			Label:       r.Label,
			Description: r.Description,
			Actions:     models.StringList(r.Actions),
			Sensitive:   r.Sensitive,
			Source:      models.PluginPermissionSourceManifest,
		})
	}
	return rows
}

// registerPluginResources puts the plugin's base resource and every stored
// sub-resource into the catalogue.
func (s *Service) registerPluginResources(plugin *models.Plugin) error {
	base := PluginBaseResource(plugin)
	if block := plugin.ManifestRBAC(); block != nil {
		base.Sensitive = block.Sensitive
	}
	if err := authz.Replace(base); err != nil {
		return err
	}
	rows, err := models.ListPluginPermissionResources(s.DB, plugin.ID)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if err := authz.Replace(PluginSubResource(plugin, row)); err != nil {
			logger.Warn(fmt.Sprintf("plugin permissions: could not register %s:%s: %v", base.Key, row.Key, err))
		}
	}
	return nil
}

// SyncPluginPermissions registers (or refreshes) the plugin's permission
// resources: the base resource, the sub-resources its manifest declares
// (stored with source manifest, replacing the previous declaration) and any
// runtime-registered ones. A resource registered under a previous key is
// removed, and everything is dropped when the plugin no longer has an
// administrator surface.
func (s *Service) SyncPluginPermissions(plugin *models.Plugin) {
	if plugin == nil || plugin.ID == 0 {
		return
	}
	key := plugin.PermissionKey()
	if prev, ok := pluginPermissionKeys.Load(plugin.ID); ok && prev.(string) != key {
		authz.UnregisterPlugin(prev.(string))
	}
	changed := false
	if plugin.HasAdminSurface() && plugin.DeletedAt.Time.IsZero() {
		if s.DB != nil {
			if err := models.UpsertPluginPermissionResources(s.DB, plugin.ID, models.PluginPermissionSourceManifest, manifestPermissionRows(plugin), true); err != nil {
				logger.Warn(fmt.Sprintf("plugin permissions: could not store manifest resources for %s: %v", key, err))
			}
		}
		// Re-register from scratch so sub-resources dropped from the manifest disappear.
		authz.UnregisterPlugin(key)
		if err := s.registerPluginResources(plugin); err != nil {
			logger.Warn(fmt.Sprintf("plugin permissions: could not register %s: %v", key, err))
			return
		}
		pluginPermissionKeys.Store(plugin.ID, key)
		changed = true
	} else if authz.UnregisterPlugin(key) > 0 {
		pluginPermissionKeys.Delete(plugin.ID)
		changed = true
	}
	if changed {
		s.refreshSystemRolesAfterCatalogueChange()
	}
}

// RegisterPluginPermissionResources stores runtime-registered sub-resources
// for a plugin (management API) and refreshes the catalogue. With
// removeMissing, runtime rows not in rows are deleted; manifest rows are
// untouched. It returns the plugin's permission key.
func (s *Service) RegisterPluginPermissionResources(pluginID uint, rows []models.PluginPermissionResource, removeMissing bool) (string, int, error) {
	plugin := &models.Plugin{}
	if err := plugin.Get(s.DB, pluginID); err != nil {
		return "", 0, err
	}
	for i := range rows {
		spec := models.ManifestPermissionResource{Key: rows[i].Key, Label: rows[i].Label, Description: rows[i].Description, Actions: []string(rows[i].Actions), Sensitive: rows[i].Sensitive}
		if err := (&models.ManifestRBAC{Resources: []models.ManifestPermissionResource{spec}}).Validate(); err != nil {
			return "", 0, err
		}
	}
	before, err := models.ListPluginPermissionResources(s.DB, pluginID)
	if err != nil {
		return "", 0, err
	}
	if err := models.UpsertPluginPermissionResources(s.DB, pluginID, models.PluginPermissionSourceRuntime, rows, removeMissing); err != nil {
		return "", 0, err
	}
	after, err := models.ListPluginPermissionResources(s.DB, pluginID)
	if err != nil {
		return "", 0, err
	}
	removed := 0
	if len(before) > len(after) {
		removed = len(before) - len(after)
	}
	s.SyncPluginPermissions(plugin)
	return plugin.PermissionKey(), removed, nil
}

// RemovePluginPermissions unregisters everything the plugin contributed.
func (s *Service) RemovePluginPermissions(plugin *models.Plugin) {
	if plugin == nil {
		return
	}
	keys := []string{plugin.PermissionKey()}
	if prev, ok := pluginPermissionKeys.Load(plugin.ID); ok && prev.(string) != keys[0] {
		keys = append(keys, prev.(string))
	}
	pluginPermissionKeys.Delete(plugin.ID)
	removed := 0
	for _, k := range keys {
		removed += authz.UnregisterPlugin(k)
	}
	if s.DB != nil {
		if err := models.DeletePluginPermissionResources(s.DB, plugin.ID); err != nil {
			logger.Warn(fmt.Sprintf("plugin permissions: could not delete stored resources for %d: %v", plugin.ID, err))
		}
	}
	if removed > 0 {
		s.refreshSystemRolesAfterCatalogueChange()
	}
}

// RebuildPermissionCatalogue registers the resource of every installed
// plugin. main.go calls it before seeding the system roles so Viewer, Editor
// and Auditor are computed against the full catalogue.
func (s *Service) RebuildPermissionCatalogue() error {
	if s == nil || s.DB == nil {
		return nil
	}
	var plugins []models.Plugin
	if err := s.DB.Find(&plugins).Error; err != nil {
		return fmt.Errorf("plugin permissions: list plugins: %w", err)
	}
	for i := range plugins {
		p := &plugins[i]
		if !p.HasAdminSurface() {
			continue
		}
		if err := s.registerPluginResources(p); err != nil {
			logger.Warn(fmt.Sprintf("plugin permissions: could not register %s: %v", p.PermissionKey(), err))
			continue
		}
		pluginPermissionKeys.Store(p.ID, p.PermissionKey())
	}
	return nil
}

func (s *Service) refreshSystemRolesAfterCatalogueChange() {
	if err := s.Authz().RefreshSystemRoles(context.Background()); err != nil {
		logger.Warn(fmt.Sprintf("plugin permissions: could not refresh system roles: %v", err))
	}
}

// pluginPermissionSyncer is what the plugin and manifest services call after
// a lifecycle change; the Service wires it at construction.
type pluginPermissionSyncer func(plugin *models.Plugin, removed bool)

func (s *Service) syncPluginPermissionsHook(plugin *models.Plugin, removed bool) {
	if removed {
		s.RemovePluginPermissions(plugin)
		return
	}
	s.SyncPluginPermissions(plugin)
}
