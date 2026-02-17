package models

import (
	"gorm.io/gorm"
)

// AppPluginResource is the join table that associates Apps with plugin resource instances.
// This is the extensible equivalent of the hardcoded app_llms, app_datasources, app_tools tables.
type AppPluginResource struct {
	gorm.Model
	ID                   uint   `json:"id" gorm:"primaryKey"`
	AppID                uint   `json:"app_id" gorm:"uniqueIndex:idx_apr_unique;index:idx_apr_app"`
	PluginResourceTypeID uint   `json:"plugin_resource_type_id" gorm:"uniqueIndex:idx_apr_unique"`
	InstanceID           string `json:"instance_id" gorm:"size:255;uniqueIndex:idx_apr_unique"`

	// Relationships
	App                *App                `json:"app,omitempty" gorm:"foreignKey:AppID"`
	PluginResourceType *PluginResourceType `json:"plugin_resource_type,omitempty" gorm:"foreignKey:PluginResourceTypeID"`
}

type AppPluginResources []AppPluginResource

func (AppPluginResource) TableName() string {
	return "app_plugin_resources"
}

// GetByApp returns all plugin resource associations for an app
func (aprs *AppPluginResources) GetByApp(db *gorm.DB, appID uint) error {
	return db.Where("app_id = ?", appID).
		Preload("PluginResourceType").
		Find(aprs).Error
}

// DeleteByAppAndType removes all associations for an app and resource type
func DeleteAppPluginResourcesByType(db *gorm.DB, appID, resourceTypeID uint) error {
	return db.Where("app_id = ? AND plugin_resource_type_id = ?", appID, resourceTypeID).
		Delete(&AppPluginResource{}).Error
}

// DeleteByApp removes all plugin resource associations for an app
func DeleteAppPluginResources(db *gorm.DB, appID uint) error {
	return db.Where("app_id = ?", appID).Delete(&AppPluginResource{}).Error
}
