package models

import (
	"gorm.io/gorm"
)

// GroupPluginResource maps Groups directly to plugin resource instances for access control.
// This replaces the Catalogue pattern used by built-in types (LLMs, Datasources, Tools).
// Access chain: User → Group → GroupPluginResource → Instance
type GroupPluginResource struct {
	gorm.Model
	ID                   uint   `json:"id" gorm:"primaryKey"`
	GroupID              uint   `json:"group_id" gorm:"uniqueIndex:idx_gpr_unique;index:idx_gpr_group"`
	PluginResourceTypeID uint   `json:"plugin_resource_type_id" gorm:"uniqueIndex:idx_gpr_unique"`
	InstanceID           string `json:"instance_id" gorm:"size:255;uniqueIndex:idx_gpr_unique"`

	// Relationships
	Group              *Group              `json:"group,omitempty" gorm:"foreignKey:GroupID"`
	PluginResourceType *PluginResourceType `json:"plugin_resource_type,omitempty" gorm:"foreignKey:PluginResourceTypeID"`
}

type GroupPluginResources []GroupPluginResource

func (GroupPluginResource) TableName() string {
	return "group_plugin_resources"
}

// GetByGroup returns all plugin resource access entries for a group
func (gprs *GroupPluginResources) GetByGroup(db *gorm.DB, groupID uint) error {
	return db.Where("group_id = ?", groupID).
		Preload("PluginResourceType").
		Find(gprs).Error
}

// GetByGroupAndType returns entries for a specific group and resource type
func (gprs *GroupPluginResources) GetByGroupAndType(db *gorm.DB, groupID, resourceTypeID uint) error {
	return db.Where("group_id = ? AND plugin_resource_type_id = ?", groupID, resourceTypeID).
		Find(gprs).Error
}

// DeleteByGroupAndType removes all entries for a group and resource type
func DeleteGroupPluginResourcesByType(db *gorm.DB, groupID, resourceTypeID uint) error {
	return db.Unscoped().Where("group_id = ? AND plugin_resource_type_id = ?", groupID, resourceTypeID).
		Delete(&GroupPluginResource{}).Error
}

// GetAccessibleInstanceIDs returns instance IDs accessible to a user via their groups.
// Joins: user_groups → group_plugin_resources, filtered by resource type.
func GetAccessiblePluginResourceInstanceIDs(db *gorm.DB, userID, resourceTypeID uint) ([]string, error) {
	var instanceIDs []string
	err := db.Table("group_plugin_resources").
		Select("DISTINCT group_plugin_resources.instance_id").
		Joins("JOIN user_groups ON user_groups.group_id = group_plugin_resources.group_id").
		Where("user_groups.user_id = ? AND group_plugin_resources.plugin_resource_type_id = ? AND group_plugin_resources.deleted_at IS NULL",
			userID, resourceTypeID).
		Pluck("instance_id", &instanceIDs).Error
	return instanceIDs, err
}

// GetAllAccessibleByUser returns all plugin resource access entries for a user (across all groups).
func GetAllAccessiblePluginResources(db *gorm.DB, userID uint) ([]GroupPluginResource, error) {
	var results []GroupPluginResource
	err := db.Table("group_plugin_resources").
		Select("DISTINCT group_plugin_resources.*").
		Joins("JOIN user_groups ON user_groups.group_id = group_plugin_resources.group_id").
		Where("user_groups.user_id = ? AND group_plugin_resources.deleted_at IS NULL", userID).
		Preload("PluginResourceType").
		Find(&results).Error
	return results, err
}
