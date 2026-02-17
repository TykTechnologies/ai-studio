package services

import (
	"fmt"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// --- Plugin Resource Type Registration ---

// RegisterPluginResourceTypes upserts resource type records for a plugin.
// Called when a plugin is loaded and its manifest/capabilities are parsed.
func (s *Service) RegisterPluginResourceTypes(pluginID uint, registrations []models.PluginResourceType) error {
	for _, reg := range registrations {
		existing := &models.PluginResourceType{}
		err := existing.GetByPluginAndSlug(s.DB, pluginID, reg.Slug)
		if err != nil {
			// Not found — create
			reg.PluginID = pluginID
			reg.IsActive = true
			if err := reg.Create(s.DB); err != nil {
				return fmt.Errorf("failed to create resource type %s: %w", reg.Slug, err)
			}
		} else {
			// Found — update
			existing.Name = reg.Name
			existing.Description = reg.Description
			existing.Icon = reg.Icon
			existing.HasPrivacyScore = reg.HasPrivacyScore
			existing.SupportsSubmissions = reg.SupportsSubmissions
			existing.FormComponentTag = reg.FormComponentTag
			existing.FormComponentEntry = reg.FormComponentEntry
			existing.IsActive = true
			if err := existing.Update(s.DB); err != nil {
				return fmt.Errorf("failed to update resource type %s: %w", reg.Slug, err)
			}
		}
	}
	return nil
}

// DeactivatePluginResourceTypes marks all resource types for a plugin as inactive.
// Called when a plugin is unloaded.
func (s *Service) DeactivatePluginResourceTypes(pluginID uint) error {
	return s.DB.Model(&models.PluginResourceType{}).
		Where("plugin_id = ?", pluginID).
		Update("is_active", false).Error
}

// GetPluginResourceTypes returns all active resource types across all plugins.
func (s *Service) GetPluginResourceTypes() ([]models.PluginResourceType, error) {
	var types models.PluginResourceTypes
	if err := types.GetAllActive(s.DB); err != nil {
		return nil, err
	}
	return types, nil
}

// GetPluginResourceTypeByID returns a resource type by its ID.
func (s *Service) GetPluginResourceTypeByID(id uint) (*models.PluginResourceType, error) {
	prt := &models.PluginResourceType{}
	if err := prt.Get(s.DB, id); err != nil {
		return nil, err
	}
	return prt, nil
}

// GetPluginResourceTypeByPluginAndSlug returns a resource type by plugin ID and slug.
func (s *Service) GetPluginResourceTypeByPluginAndSlug(pluginID uint, slug string) (*models.PluginResourceType, error) {
	prt := &models.PluginResourceType{}
	if err := prt.GetByPluginAndSlug(s.DB, pluginID, slug); err != nil {
		return nil, err
	}
	return prt, nil
}

// --- App ↔ Plugin Resource Associations ---

// SetAppPluginResources replaces all plugin resource associations for an app
// and a given resource type. This is a full replace (delete old, insert new).
func (s *Service) SetAppPluginResources(appID, resourceTypeID uint, instanceIDs []string) error {
	// Delete existing associations for this app+type
	if err := models.DeleteAppPluginResourcesByType(s.DB, appID, resourceTypeID); err != nil {
		return fmt.Errorf("failed to clear app plugin resources: %w", err)
	}

	// Insert new associations
	for _, instanceID := range instanceIDs {
		apr := &models.AppPluginResource{
			AppID:                appID,
			PluginResourceTypeID: resourceTypeID,
			InstanceID:           instanceID,
		}
		if err := s.DB.Create(apr).Error; err != nil {
			return fmt.Errorf("failed to create app plugin resource association: %w", err)
		}
	}
	return nil
}

// GetAppPluginResources returns all plugin resource associations for an app.
func (s *Service) GetAppPluginResources(appID uint) ([]models.AppPluginResource, error) {
	var aprs models.AppPluginResources
	if err := aprs.GetByApp(s.DB, appID); err != nil {
		return nil, err
	}
	return aprs, nil
}

// ClearAppPluginResources removes all plugin resource associations for an app.
func (s *Service) ClearAppPluginResources(appID uint) error {
	return models.DeleteAppPluginResources(s.DB, appID)
}

// --- Group ↔ Plugin Resource Access Control ---

// SetGroupPluginResources replaces all plugin resource access entries for a group
// and a given resource type.
func (s *Service) SetGroupPluginResources(groupID, resourceTypeID uint, instanceIDs []string) error {
	// Delete existing entries for this group+type
	if err := models.DeleteGroupPluginResourcesByType(s.DB, groupID, resourceTypeID); err != nil {
		return fmt.Errorf("failed to clear group plugin resources: %w", err)
	}

	// Insert new entries
	for _, instanceID := range instanceIDs {
		gpr := &models.GroupPluginResource{
			GroupID:              groupID,
			PluginResourceTypeID: resourceTypeID,
			InstanceID:           instanceID,
		}
		if err := s.DB.Create(gpr).Error; err != nil {
			return fmt.Errorf("failed to create group plugin resource entry: %w", err)
		}
	}
	return nil
}

// GetGroupPluginResources returns all plugin resource access entries for a group.
func (s *Service) GetGroupPluginResources(groupID uint) ([]models.GroupPluginResource, error) {
	var gprs models.GroupPluginResources
	if err := gprs.GetByGroup(s.DB, groupID); err != nil {
		return nil, err
	}
	return gprs, nil
}

// GetAccessiblePluginResourceInstances returns instance IDs accessible to a user
// for a given resource type, by joining through their group memberships.
func (s *Service) GetAccessiblePluginResourceInstances(userID, resourceTypeID uint) ([]string, error) {
	return models.GetAccessiblePluginResourceInstanceIDs(s.DB, userID, resourceTypeID)
}

// GetAllAccessiblePluginResources returns all plugin resource access entries for a user.
func (s *Service) GetAllAccessiblePluginResources(userID uint) ([]models.GroupPluginResource, error) {
	return models.GetAllAccessiblePluginResources(s.DB, userID)
}
