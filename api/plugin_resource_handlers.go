package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

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
