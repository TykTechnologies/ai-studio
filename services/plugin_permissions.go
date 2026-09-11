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

// SyncPluginPermissions registers (or refreshes) the plugin's permission
// resource, removing a resource registered under a previous key, and drops
// it when the plugin no longer has an administrator surface.
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
		if err := authz.Replace(PluginBaseResource(plugin)); err != nil {
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
		if err := authz.Replace(PluginBaseResource(p)); err != nil {
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
