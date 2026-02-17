package api

import (
	"net/http"
	"strconv"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/gin-gonic/gin"
)

// listPluginResourceInstances returns all instances of a resource type by proxying
// to the plugin's ListResourceInstances RPC. Admins see all instances.
// This is the endpoint that populates the App form and Group form selectors.
func (a *API) listPluginResourceInstances(c *gin.Context) {
	pluginID, err := strconv.ParseUint(c.Param("plugin_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid plugin ID"})
		return
	}
	slug := c.Param("slug")
	if slug == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Resource type slug is required"})
		return
	}

	// Verify the resource type exists
	prt, err := a.service.GetPluginResourceTypeByPluginAndSlug(uint(pluginID), slug)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Resource type not found"})
		return
	}

	// Call the plugin's ListResourceInstances RPC via plugin manager
	if a.service.AIStudioPluginManager == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Plugin manager not available"})
		return
	}

	instances, err := a.service.AIStudioPluginManager.ListResourceInstances(uint(pluginID), slug)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Auto-assign new instances to the default group (lazy reconciliation).
	// This mirrors the built-in resource pattern where new LLMs/Datasources/Tools
	// are added to the default catalogue so they're visible to all users by default.
	var activeIDs []string
	for _, inst := range instances {
		if inst.IsActive {
			activeIDs = append(activeIDs, inst.Id)
		}
	}
	_ = a.service.EnsureDefaultGroupAccess(prt.ID, activeIDs)

	// Convert proto instances to JSON response
	result := make([]gin.H, 0, len(instances))
	for _, inst := range instances {
		if !inst.IsActive {
			continue
		}
		result = append(result, gin.H{
			"id":            inst.Id,
			"name":          inst.Name,
			"description":   inst.Description,
			"privacy_score": inst.PrivacyScore,
			"is_active":     inst.IsActive,
		})
	}

	c.JSON(http.StatusOK, gin.H{"data": result})
}

// getUserAccessiblePluginResources returns plugin resource types and their accessible
// instances for the current user. Used by the Portal AppBuilder to show plugin resource
// selectors. Admins see all instances; non-admins see only group-accessible ones.
func (a *API) getUserAccessiblePluginResources(c *gin.Context) {
	user, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not found in context"})
		return
	}
	currentUser := user.(*models.User)

	types, err := a.service.GetPluginResourceTypes()
	if err != nil || len(types) == 0 {
		c.JSON(http.StatusOK, gin.H{"data": []interface{}{}})
		return
	}

	var result []gin.H
	for _, rt := range types {
		// Get instances from plugin RPC
		var instances []gin.H
		if a.service.AIStudioPluginManager != nil {
			protoInstances, err := a.service.AIStudioPluginManager.ListResourceInstances(rt.PluginID, rt.Slug)
			if err == nil {
				// Auto-assign new instances to default group
				var activeIDs []string
				for _, inst := range protoInstances {
					if inst.IsActive {
						activeIDs = append(activeIDs, inst.Id)
					}
				}
				_ = a.service.EnsureDefaultGroupAccess(rt.ID, activeIDs)

				// Filter by access for non-admins
				var accessibleSet map[string]bool
				if !currentUser.IsAdmin {
					accessibleIDs, err := a.service.GetAccessiblePluginResourceInstances(currentUser.ID, rt.ID)
					if err == nil {
						accessibleSet = make(map[string]bool)
						for _, id := range accessibleIDs {
							accessibleSet[id] = true
						}
					}
				}

				for _, inst := range protoInstances {
					if !inst.IsActive {
						continue
					}
					// Non-admins: filter by group access
					if accessibleSet != nil && !accessibleSet[inst.Id] {
						continue
					}
					instances = append(instances, gin.H{
						"id":            inst.Id,
						"name":          inst.Name,
						"description":   inst.Description,
						"privacy_score": inst.PrivacyScore,
					})
				}
			}
		}

		result = append(result, gin.H{
			"plugin_id":   rt.PluginID,
			"slug":        rt.Slug,
			"name":        rt.Name,
			"description": rt.Description,
			"icon":        rt.Icon,
			"instances":   instances,
		})
	}

	c.JSON(http.StatusOK, gin.H{"data": result})
}

// listPluginResourceTypes returns all active registered resource types
func (a *API) listPluginResourceTypes(c *gin.Context) {
	types, err := a.service.GetPluginResourceTypes()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	result := make([]gin.H, 0, len(types))
	for _, t := range types {
		entry := gin.H{
			"id":                   t.ID,
			"plugin_id":            t.PluginID,
			"slug":                 t.Slug,
			"name":                 t.Name,
			"description":          t.Description,
			"icon":                 t.Icon,
			"has_privacy_score":    t.HasPrivacyScore,
			"supports_submissions": t.SupportsSubmissions,
			"is_active":            t.IsActive,
		}
		if t.FormComponentTag != "" {
			entry["form_component"] = gin.H{
				"tag":         t.FormComponentTag,
				"entry_point": t.FormComponentEntry,
			}
		}
		if t.Plugin != nil {
			entry["plugin_name"] = t.Plugin.Name
		}
		result = append(result, entry)
	}

	c.JSON(http.StatusOK, gin.H{"data": result})
}

// getAppPluginResources returns plugin resource associations for an app
func (a *API) getAppPluginResources(c *gin.Context) {
	appID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid app ID"})
		return
	}

	aprs, err := a.service.GetAppPluginResources(uint(appID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Group by resource type
	grouped := make(map[uint]*PluginResourceOutput)
	for _, apr := range aprs {
		key := apr.PluginResourceTypeID
		if _, exists := grouped[key]; !exists {
			typeName := ""
			pluginID := uint(0)
			slug := ""
			if apr.PluginResourceType != nil {
				typeName = apr.PluginResourceType.Name
				pluginID = apr.PluginResourceType.PluginID
				slug = apr.PluginResourceType.Slug
			}
			grouped[key] = &PluginResourceOutput{
				PluginID:         pluginID,
				ResourceTypeSlug: slug,
				ResourceTypeName: typeName,
				InstanceIDs:      []string{},
			}
		}
		grouped[key].InstanceIDs = append(grouped[key].InstanceIDs, apr.InstanceID)
	}

	result := make([]PluginResourceOutput, 0, len(grouped))
	for _, pr := range grouped {
		result = append(result, *pr)
	}

	c.JSON(http.StatusOK, gin.H{"data": result})
}

// getGroupPluginResources returns plugin resource access entries for a group
func (a *API) getGroupPluginResources(c *gin.Context) {
	groupID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid group ID"})
		return
	}

	gprs, err := a.service.GetGroupPluginResources(uint(groupID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Group by resource type
	grouped := make(map[uint]*PluginResourceOutput)
	for _, gpr := range gprs {
		key := gpr.PluginResourceTypeID
		if _, exists := grouped[key]; !exists {
			typeName := ""
			pluginID := uint(0)
			slug := ""
			if gpr.PluginResourceType != nil {
				typeName = gpr.PluginResourceType.Name
				pluginID = gpr.PluginResourceType.PluginID
				slug = gpr.PluginResourceType.Slug
			}
			grouped[key] = &PluginResourceOutput{
				PluginID:         pluginID,
				ResourceTypeSlug: slug,
				ResourceTypeName: typeName,
				InstanceIDs:      []string{},
			}
		}
		grouped[key].InstanceIDs = append(grouped[key].InstanceIDs, gpr.InstanceID)
	}

	result := make([]PluginResourceOutput, 0, len(grouped))
	for _, pr := range grouped {
		result = append(result, *pr)
	}

	c.JSON(http.StatusOK, gin.H{"data": result})
}

// setGroupPluginResources replaces plugin resource access for a group
func (a *API) setGroupPluginResources(c *gin.Context) {
	groupID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid group ID"})
		return
	}

	var input struct {
		Resources []struct {
			PluginID         uint     `json:"plugin_id"`
			ResourceTypeSlug string   `json:"resource_type_slug"`
			InstanceIDs      []string `json:"instance_ids"`
		} `json:"resources"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	for _, r := range input.Resources {
		prt, err := a.service.GetPluginResourceTypeByPluginAndSlug(r.PluginID, r.ResourceTypeSlug)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Unknown resource type: " + r.ResourceTypeSlug})
			return
		}
		if err := a.service.SetGroupPluginResources(uint(groupID), prt.ID, r.InstanceIDs); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}
