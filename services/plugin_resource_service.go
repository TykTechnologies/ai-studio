package services

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/eventbridge"
	"github.com/TykTechnologies/midsommar/v2/pkg/plugin_sdk"
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

	// Look up the resource type to get plugin ID for instance detail lookup
	prt, err := s.GetPluginResourceTypeByID(resourceTypeID)
	if err != nil {
		return fmt.Errorf("failed to get resource type %d: %w", resourceTypeID, err)
	}

	// Build a map of instance details from plugin RPC (best-effort)
	instanceDetails := s.fetchInstanceDetails(prt.PluginID, prt.Slug, instanceIDs)

	// Insert new associations with cached instance details
	for _, instanceID := range instanceIDs {
		apr := &models.AppPluginResource{
			AppID:                appID,
			PluginResourceTypeID: resourceTypeID,
			InstanceID:           instanceID,
		}
		if detail, ok := instanceDetails[instanceID]; ok {
			apr.InstanceName = detail.Name
			apr.InstancePrivacyScore = detail.PrivacyScore
			apr.InstanceMetadata = detail.Metadata
		}
		if err := s.DB.Create(apr).Error; err != nil {
			return fmt.Errorf("failed to create app plugin resource association: %w", err)
		}
	}
	return nil
}

// instanceDetail holds cached instance information from the plugin
type instanceDetail struct {
	Name         string
	PrivacyScore int
	Metadata     []byte
}

// fetchInstanceDetails calls the plugin's ListResourceInstances RPC to get
// instance details for caching. Returns a map of instanceID -> details.
// Best-effort: returns an empty map if the plugin manager is not available.
func (s *Service) fetchInstanceDetails(pluginID uint, slug string, instanceIDs []string) map[string]instanceDetail {
	details := make(map[string]instanceDetail)

	if s.AIStudioPluginManager == nil {
		return details
	}

	// Call ListResourceInstances via plugin RPC
	instances, err := s.AIStudioPluginManager.ListResourceInstances(pluginID, slug)
	if err != nil {
		return details
	}

	// Build lookup map
	for _, inst := range instances {
		details[inst.Id] = instanceDetail{
			Name:         inst.Name,
			PrivacyScore: int(inst.PrivacyScore),
			Metadata:     inst.Metadata,
		}
	}
	return details
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

// GroupPluginResourceUpdate represents a single resource type update for a group.
type GroupPluginResourceUpdate struct {
	ResourceTypeID uint
	InstanceIDs    []string
}

// SetGroupPluginResourcesBatch atomically replaces plugin resource access entries
// for a group across multiple resource types in a single transaction.
func (s *Service) SetGroupPluginResourcesBatch(groupID uint, updates []GroupPluginResourceUpdate) error {
	tx := s.DB.Begin()
	if tx.Error != nil {
		return fmt.Errorf("failed to start transaction: %w", tx.Error)
	}

	for _, u := range updates {
		if err := models.DeleteGroupPluginResourcesByType(tx, groupID, u.ResourceTypeID); err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to clear group plugin resources for type %d: %w", u.ResourceTypeID, err)
		}
		for _, instanceID := range u.InstanceIDs {
			gpr := &models.GroupPluginResource{
				GroupID:              groupID,
				PluginResourceTypeID: u.ResourceTypeID,
				InstanceID:           instanceID,
			}
			if err := tx.Create(gpr).Error; err != nil {
				tx.Rollback()
				return fmt.Errorf("failed to create group plugin resource entry: %w", err)
			}
		}
	}

	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
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

// EnsureDefaultGroupAccess checks all instances reported by the plugin and ensures
// any new instances are automatically assigned to the default group. This mirrors the
// built-in resource pattern where new LLMs/Datasources/Tools are added to the default
// catalogue so they're immediately visible to all users.
//
// Called when listing instances (lazy reconciliation) to avoid requiring plugins
// to explicitly manage group assignments.
func (s *Service) EnsureDefaultGroupAccess(resourceTypeID uint, instanceIDs []string) error {
	// Find the default group
	defaultGroup := &models.Group{}
	if err := s.DB.Where("name = ?", models.DefaultGroupName).First(defaultGroup).Error; err != nil {
		// No default group — nothing to do
		return nil
	}

	// Get currently assigned instance IDs for this type in the default group
	var existing models.GroupPluginResources
	if err := existing.GetByGroupAndType(s.DB, defaultGroup.ID, resourceTypeID); err != nil {
		return nil
	}

	existingSet := make(map[string]bool)
	for _, gpr := range existing {
		existingSet[gpr.InstanceID] = true
	}

	// Add any new instances that aren't already in the default group
	for _, id := range instanceIDs {
		if existingSet[id] {
			continue
		}
		gpr := &models.GroupPluginResource{
			GroupID:              defaultGroup.ID,
			PluginResourceTypeID: resourceTypeID,
			InstanceID:           id,
		}
		if err := s.DB.Create(gpr).Error; err != nil {
			// Log but don't fail — this is best-effort
			continue
		}
	}
	return nil
}

// SubscribeResourceInstanceChanges registers an event handler that refreshes
// denormalized instance data in app_plugin_resources when a plugin notifies
// that an instance has changed. This keeps cached names, privacy scores, and
// metadata consistent with the plugin's source of truth.
//
// Call this after SetEventBus to activate the subscription.
func (s *Service) SubscribeResourceInstanceChanges(bus eventbridge.Bus) {
	if bus == nil {
		return
	}

	bus.Subscribe(plugin_sdk.ResourceInstanceChangedEvent, func(evt eventbridge.Event) {
		var payload struct {
			ResourceTypeSlug string `json:"resource_type_slug"`
			InstanceID       string `json:"instance_id"`
		}
		if err := json.Unmarshal(evt.Payload, &payload); err != nil {
			log.Printf("Warning: failed to parse instance_changed event payload: %v", err)
			return
		}

		if payload.ResourceTypeSlug == "" || payload.InstanceID == "" {
			return
		}

		s.refreshInstanceDetails(payload.ResourceTypeSlug, payload.InstanceID)
	})

	log.Printf("Subscribed to %s events for denormalized data refresh", plugin_sdk.ResourceInstanceChangedEvent)
}

// refreshInstanceDetails fetches updated instance details from the plugin and
// updates all AppPluginResource rows that reference the given instance.
func (s *Service) refreshInstanceDetails(resourceTypeSlug, instanceID string) {
	// Find the resource type to get the plugin ID
	var prt models.PluginResourceType
	if err := s.DB.Where("slug = ? AND is_active = ?", resourceTypeSlug, true).First(&prt).Error; err != nil {
		return // Unknown type, nothing to refresh
	}

	if s.AIStudioPluginManager == nil {
		return
	}

	// Fetch updated instance data from the plugin
	instances, err := s.AIStudioPluginManager.ListResourceInstances(prt.PluginID, resourceTypeSlug)
	if err != nil {
		log.Printf("Warning: failed to fetch instances for refresh (plugin %d, type %s): %v", prt.PluginID, resourceTypeSlug, err)
		return
	}

	// Find the specific instance
	for _, inst := range instances {
		if inst.Id != instanceID {
			continue
		}

		// Update all AppPluginResource rows that reference this instance
		result := s.DB.Model(&models.AppPluginResource{}).
			Where("plugin_resource_type_id = ? AND instance_id = ?", prt.ID, instanceID).
			Updates(map[string]interface{}{
				"instance_name":          inst.Name,
				"instance_privacy_score": int(inst.PrivacyScore),
				"instance_metadata":      inst.Metadata,
			})

		if result.Error != nil {
			log.Printf("Warning: failed to refresh instance %s details: %v", instanceID, result.Error)
		} else if result.RowsAffected > 0 {
			log.Printf("Refreshed denormalized data for instance %s (%d rows updated)", instanceID, result.RowsAffected)
		}
		return
	}

	log.Printf("Warning: instance %s not found in plugin response during refresh", instanceID)
}
