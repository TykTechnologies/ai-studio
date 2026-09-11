package models

import (
	"time"

	"gorm.io/gorm"
)

// PluginPermissionResource is a permission resource a plugin contributes to
// the RBAC catalogue beyond its base resource: declared in the manifest
// ("rbac.resources") or registered at runtime through the management API
// (asset classes defined by administrators, for example). Rows are what the
// catalogue is rebuilt from at boot, so the role editor keeps working while
// the plugin process is down.
type PluginPermissionResource struct {
	ID          uint       `json:"id" gorm:"primaryKey"`
	PluginID    uint       `json:"plugin_id" gorm:"not null;uniqueIndex:idx_plugin_permission_resources_key"`
	Key         string     `json:"key" gorm:"size:100;not null;uniqueIndex:idx_plugin_permission_resources_key"`
	Label       string     `json:"label" gorm:"size:255;not null"`
	Description string     `json:"description"`
	Actions     StringList `json:"actions" gorm:"type:text"`
	Sensitive   bool       `json:"sensitive" gorm:"default:false"`
	// Source is "manifest" or "runtime". Manifest rows are replaced on every
	// manifest registration; runtime rows only through the management API.
	Source    string    `json:"source" gorm:"size:16;not null;default:manifest"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

const (
	PluginPermissionSourceManifest = "manifest"
	PluginPermissionSourceRuntime  = "runtime"
)

func (PluginPermissionResource) TableName() string { return "plugin_permission_resources" }

// ListByPlugin returns the plugin's rows, manifest ones first.
func ListPluginPermissionResources(db *gorm.DB, pluginID uint) ([]PluginPermissionResource, error) {
	var rows []PluginPermissionResource
	err := db.Where("plugin_id = ?", pluginID).Order("source ASC, key ASC").Find(&rows).Error
	return rows, err
}

// UpsertPluginPermissionResources writes rows for one plugin and source,
// deleting rows of that source not present when removeMissing is set.
func UpsertPluginPermissionResources(db *gorm.DB, pluginID uint, source string, rows []PluginPermissionResource, removeMissing bool) error {
	return db.Transaction(func(tx *gorm.DB) error {
		keep := make([]string, 0, len(rows))
		for i := range rows {
			r := rows[i]
			r.PluginID = pluginID
			r.Source = source
			keep = append(keep, r.Key)
			var existing PluginPermissionResource
			err := tx.Where("plugin_id = ? AND key = ?", pluginID, r.Key).First(&existing).Error
			switch {
			case err == nil:
				existing.Label = r.Label
				existing.Description = r.Description
				existing.Actions = r.Actions
				existing.Sensitive = r.Sensitive
				existing.Source = source
				if err := tx.Save(&existing).Error; err != nil {
					return err
				}
			case err == gorm.ErrRecordNotFound:
				if err := tx.Create(&r).Error; err != nil {
					return err
				}
			default:
				return err
			}
		}
		if removeMissing {
			q := tx.Where("plugin_id = ? AND source = ?", pluginID, source)
			if len(keep) > 0 {
				q = q.Where("key NOT IN ?", keep)
			}
			if err := q.Delete(&PluginPermissionResource{}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// DeletePluginPermissionResources removes every row of a plugin.
func DeletePluginPermissionResources(db *gorm.DB, pluginID uint) error {
	return db.Where("plugin_id = ?", pluginID).Delete(&PluginPermissionResource{}).Error
}
