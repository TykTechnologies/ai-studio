package services

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/authz"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// Team access to a plugin's own resource instances.
//
// These back the resource-access.manage management RPCs: a ResourceProvider
// plugin that registers a type with models.DefaultAccessExplicit decides who
// sees each instance (an asset catalog publishing an asset to chosen teams,
// say) and needs to read and write the same group_plugin_resources rows the
// Teams page edits, and to ask which instances a given user can reach. Every
// call is scoped to a resource type the calling plugin registered.

// ErrResourceTypeNotOwned is returned when a plugin names a resource type it
// did not register (or that does not exist).
var ErrResourceTypeNotOwned = errors.New("resource type not registered by this plugin")

// ErrUnknownGroup is returned when a grant names a group that does not exist.
var ErrUnknownGroup = errors.New("unknown group")

// ownedResourceType loads the calling plugin's active resource type by slug.
func (s *Service) ownedResourceType(pluginID uint, slug string) (*models.PluginResourceType, error) {
	prt := &models.PluginResourceType{}
	if err := prt.GetByPluginAndSlug(s.DB, pluginID, slug); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrResourceTypeNotOwned
		}
		return nil, err
	}
	if !prt.IsActive {
		return nil, ErrResourceTypeNotOwned
	}
	return prt, nil
}

// PluginGroupSummary is one team as a plugin sees it.
type PluginGroupSummary struct {
	ID        uint
	Name      string
	IsDefault bool
}

// ListGroupsForPlugin returns every team, ordered by name, with the Default
// team flagged.
func (s *Service) ListGroupsForPlugin() ([]PluginGroupSummary, error) {
	var groups []models.Group
	if err := s.DB.Select("id", "name").Order("name").Find(&groups).Error; err != nil {
		return nil, err
	}
	out := make([]PluginGroupSummary, 0, len(groups))
	for _, g := range groups {
		out = append(out, PluginGroupSummary{ID: g.ID, Name: g.Name, IsDefault: g.Name == models.DefaultGroupName})
	}
	return out, nil
}

// ResourceInstanceGroups returns the IDs of the teams granted one of the
// plugin's resource instances, ascending.
func (s *Service) ResourceInstanceGroups(pluginID uint, slug, instanceID string) ([]uint, error) {
	prt, err := s.ownedResourceType(pluginID, slug)
	if err != nil {
		return nil, err
	}
	return instanceGroupIDs(s.DB, prt.ID, instanceID)
}

func instanceGroupIDs(db *gorm.DB, typeID uint, instanceID string) ([]uint, error) {
	var ids []uint
	err := db.Model(&models.GroupPluginResource{}).
		Where("plugin_resource_type_id = ? AND instance_id = ?", typeID, instanceID).
		Order("group_id").
		Pluck("group_id", &ids).Error
	return ids, err
}

// SetResourceInstanceGroups grants one of the plugin's resource instances to
// groupIDs. With replace the instance ends up granted to exactly groupIDs
// (an empty list revokes every grant); otherwise the groups are added to the
// existing grants. Returns the grants after the change.
//
// Revoked rows are hard-deleted: the (group, type, instance) unique index
// would otherwise block granting the same team again later.
func (s *Service) SetResourceInstanceGroups(pluginID uint, slug, instanceID string, groupIDs []uint, replace bool) ([]uint, error) {
	if instanceID == "" {
		return nil, fmt.Errorf("instance id is required")
	}
	prt, err := s.ownedResourceType(pluginID, slug)
	if err != nil {
		return nil, err
	}
	want := make(map[uint]bool, len(groupIDs))
	for _, id := range groupIDs {
		want[id] = true
	}
	if len(want) > 0 {
		ids := make([]uint, 0, len(want))
		for id := range want {
			ids = append(ids, id)
		}
		var found int64
		if err := s.DB.Model(&models.Group{}).Where("id IN ?", ids).Count(&found).Error; err != nil {
			return nil, err
		}
		if int(found) != len(ids) {
			return nil, ErrUnknownGroup
		}
	}

	var result []uint
	err = s.DB.Transaction(func(tx *gorm.DB) error {
		current, err := instanceGroupIDs(tx, prt.ID, instanceID)
		if err != nil {
			return err
		}
		have := make(map[uint]bool, len(current))
		for _, id := range current {
			have[id] = true
		}
		if replace {
			var drop []uint
			for _, id := range current {
				if !want[id] {
					drop = append(drop, id)
				}
			}
			if len(drop) > 0 {
				if err := tx.Unscoped().
					Where("plugin_resource_type_id = ? AND instance_id = ? AND group_id IN ?", prt.ID, instanceID, drop).
					Delete(&models.GroupPluginResource{}).Error; err != nil {
					return err
				}
			}
		}
		add := make([]uint, 0, len(want))
		for id := range want {
			if !have[id] {
				add = append(add, id)
			}
		}
		sort.Slice(add, func(i, j int) bool { return add[i] < add[j] })
		for _, id := range add {
			// Purge any soft-deleted tombstone first; it would trip the unique index.
			if err := tx.Unscoped().
				Where("plugin_resource_type_id = ? AND instance_id = ? AND group_id = ? AND deleted_at IS NOT NULL", prt.ID, instanceID, id).
				Delete(&models.GroupPluginResource{}).Error; err != nil {
				return err
			}
			if err := tx.Create(&models.GroupPluginResource{GroupID: id, PluginResourceTypeID: prt.ID, InstanceID: instanceID}).Error; err != nil {
				return err
			}
		}
		result, err = instanceGroupIDs(tx, prt.ID, instanceID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// AccessibleResourceInstanceIDs answers "which of this plugin's instances of
// a type can this user reach?" with the platform's portal rule
// (AccessiblePluginResourceInstances): a user who manages teams
// (groups:write) sees every instance (seeAll), anyone else sees the
// instances granted to their teams. Instance activity is the plugin's own
// business and is not checked here.
func (s *Service) AccessibleResourceInstanceIDs(ctx context.Context, pluginID uint, slug string, userID uint) (seeAll bool, ids []string, err error) {
	prt, err := s.ownedResourceType(pluginID, slug)
	if err != nil {
		return false, nil, err
	}
	if userID == 0 {
		return false, nil, fmt.Errorf("user id is required")
	}
	user := &models.User{}
	if err := s.DB.First(user, userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, []string{}, nil
		}
		return false, nil, err
	}
	perms, err := s.Authz().Resolve(ctx, user)
	if err != nil {
		return false, nil, err
	}
	if perms.Has(authz.Write("groups")) {
		return true, nil, nil
	}
	ids, err = models.GetAccessiblePluginResourceInstanceIDs(s.DB, userID, prt.ID)
	if err != nil {
		return false, nil, err
	}
	if ids == nil {
		ids = []string{}
	}
	return false, ids, nil
}
