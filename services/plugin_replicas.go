package services

import (
	"errors"
	"fmt"
	"time"

	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/authz"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// ApplyPluginChangeFromReplica brings this replica in line with a plugin
// change another replica made (a relayed system.plugin.* event). The other
// replica already wrote the database, refreshed the system roles and
// restarted its own copy; this one only updates what it holds in memory:
//
//   - the permission catalogue entries for the plugin, re-read from the
//     database (no writes, no role refresh);
//   - the running plugin: a deleted or deactivated plugin is stopped, an
//     updated one that was running is restarted with its new command and
//     configuration, and one Studio loads at start (UI, agent, object hooks)
//     is started if it is not running.
//
// It may take seconds (a plugin process starts); callers run it off the
// event reader's goroutine, in event order.
func (s *Service) ApplyPluginChangeFromReplica(topic string, pluginID uint) {
	if s == nil || s.DB == nil || pluginID == 0 {
		return
	}
	m := s.AIStudioPluginManager
	var plugin models.Plugin
	err := s.DB.Unscoped().First(&plugin, pluginID).Error
	gone := topic == TopicPluginDeleted || errors.Is(err, gorm.ErrRecordNotFound) || (err == nil && plugin.DeletedAt.Valid)
	if err != nil && !gone {
		logger.Errorf("Plugin %d changed on another replica, but reading it failed; this replica keeps its current copy: %v", pluginID, err)
		return
	}

	if gone {
		s.forgetPluginPermissions(pluginID)
		if m != nil && m.IsPluginLoaded(pluginID) {
			if err := m.UnloadPlugin(pluginID); err != nil {
				logger.Warnf("Stopping plugin %d (deleted on another replica) failed: %v", pluginID, err)
			}
		}
		return
	}

	s.reloadPluginPermissions(&plugin)
	if m == nil {
		return
	}
	wasLoaded := m.IsPluginLoaded(pluginID)
	if wasLoaded {
		if err := m.UnloadPlugin(pluginID); err != nil {
			logger.Warnf("Stopping plugin %d to restart it after a change on another replica failed: %v", pluginID, err)
		}
	}
	if plugin.IsActive && (wasLoaded || loadsAtStart(&plugin)) {
		if _, err := m.LoadPlugin(pluginID); err != nil {
			logger.Errorf("Starting plugin %s (%d) after a change on another replica failed: %v", plugin.Name, pluginID, err)
		}
	}
}

// forgetPluginPermissions drops the plugin's entries from this replica's
// permission catalogue (memory only).
func (s *Service) forgetPluginPermissions(pluginID uint) {
	if prev, ok := pluginPermissionKeys.Load(pluginID); ok {
		authz.UnregisterPlugin(prev.(string))
	}
	pluginPermissionKeys.Delete(pluginID)
	pluginPermissionInfos.Delete(pluginID)
}

// reloadPluginPermissions re-reads the plugin's permission entries into
// this replica's catalogue (memory only; the database is current).
func (s *Service) reloadPluginPermissions(plugin *models.Plugin) {
	s.forgetPluginPermissions(plugin.ID)
	if !plugin.HasAdminSurface() {
		return
	}
	if err := s.registerPluginResources(plugin); err != nil {
		logger.Warn(fmt.Sprintf("plugin permissions: could not register %s after a change on another replica: %v", plugin.PermissionKey(), err))
		return
	}
	pluginPermissionKeys.Store(plugin.ID, plugin.PermissionKey())
	cachePluginPermissionInfo(plugin)
}

// ReconcilePluginsAfterGap brings this replica's plugins in line with the
// database after it may have missed other replicas' plugin changes (it
// could not read the cluster event log for longer than events are kept).
// Every plugin changed or deleted since `since` is applied as its change
// event would have been (ApplyPluginChangeFromReplica); of the others, only
// those whose running state contradicts the database: running although
// deleted or deactivated, or not running although Studio loads them at
// start. A plugin that is as the database says is left running, not
// restarted. It may take seconds per plugin; call it off the event reader's
// goroutine.
func (s *Service) ReconcilePluginsAfterGap(since time.Time) {
	if s == nil || s.DB == nil {
		return
	}
	var plugins []models.Plugin
	if err := s.DB.Unscoped().Find(&plugins).Error; err != nil {
		logger.Errorf("Reloading plugins after this replica missed cluster events failed; it keeps its current plugins until they change again or it restarts: %v", err)
		return
	}
	m := s.AIStudioPluginManager
	applied := 0
	for i := range plugins {
		p := &plugins[i]
		gone := p.DeletedAt.Valid
		loaded := m != nil && m.IsPluginLoaded(p.ID)
		changed := !p.UpdatedAt.Before(since) || (gone && !p.DeletedAt.Time.Before(since))
		switch {
		case changed:
		case loaded && (gone || !p.IsActive):
		case !loaded && !gone && p.IsActive && m != nil && loadsAtStart(p):
		default:
			continue
		}
		topic := TopicPluginUpdated
		if gone {
			topic = TopicPluginDeleted
		}
		s.ApplyPluginChangeFromReplica(topic, p.ID)
		applied++
	}
	logger.Infof("Reloaded %d of %d plugin(s) from the database after this replica missed cluster events", applied, len(plugins))
}
