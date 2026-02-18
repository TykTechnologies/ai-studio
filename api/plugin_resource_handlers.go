package api

import (
	"html"
	"log"
	"net/http"
	"strconv"
	"sync"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/gin-gonic/gin"
)

// sanitizeString escapes HTML entities in plugin-provided strings to prevent
// stored XSS if the frontend renders them without escaping.
func sanitizeString(s string) string {
	return html.EscapeString(s)
}

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
	_, err = a.service.GetPluginResourceTypeByPluginAndSlug(uint(pluginID), slug)
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

	// Convert proto instances to JSON response with sanitization
	result := make([]gin.H, 0, len(instances))
	for _, inst := range instances {
		if !inst.IsActive {
			continue
		}
		result = append(result, gin.H{
			"id":            inst.Id,
			"name":          sanitizeString(inst.Name),
			"description":   sanitizeString(inst.Description),
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

	// For non-admins, batch-fetch all accessible plugin resources in one query
	// to avoid N+1 per resource type.
	var accessibleByType map[uint]map[string]bool
	if !currentUser.IsAdmin {
		allAccessible, err := a.service.GetAllAccessiblePluginResources(currentUser.ID)
		if err != nil {
			// Fail-closed: if we can't determine access, return empty
			log.Printf("Warning: failed to fetch accessible plugin resources for user %d: %v", currentUser.ID, err)
			c.JSON(http.StatusOK, gin.H{"data": []interface{}{}})
			return
		}
		accessibleByType = make(map[uint]map[string]bool)
		for _, gpr := range allAccessible {
			if accessibleByType[gpr.PluginResourceTypeID] == nil {
				accessibleByType[gpr.PluginResourceTypeID] = make(map[string]bool)
			}
			accessibleByType[gpr.PluginResourceTypeID][gpr.InstanceID] = true
		}
	}

	// Fetch instances from all plugins concurrently
	type typeResult struct {
		Index     int
		Instances []gin.H
	}
	resultCh := make(chan typeResult, len(types))
	var wg sync.WaitGroup

	for i, rt := range types {
		if a.service.AIStudioPluginManager == nil {
			resultCh <- typeResult{Index: i, Instances: nil}
			continue
		}
		wg.Add(1)
		go func(idx int, rt models.PluginResourceType) {
			defer wg.Done()
			var instances []gin.H

			protoInstances, err := a.service.AIStudioPluginManager.ListResourceInstances(rt.PluginID, rt.Slug)
			if err != nil {
				resultCh <- typeResult{Index: idx, Instances: nil}
				return
			}

			// Filter by pre-fetched access set for non-admins
			accessibleSet := accessibleByType[rt.ID] // nil for admins

			for _, inst := range protoInstances {
				if !inst.IsActive {
					continue
				}
				if accessibleSet != nil && !accessibleSet[inst.Id] {
					continue
				}
				instances = append(instances, gin.H{
					"id":            inst.Id,
					"name":          sanitizeString(inst.Name),
					"description":   sanitizeString(inst.Description),
					"privacy_score": inst.PrivacyScore,
				})
			}
			resultCh <- typeResult{Index: idx, Instances: instances}
		}(i, rt)
	}

	// Close channel after all goroutines complete
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	// Collect results into ordered slice
	instancesByIndex := make(map[int][]gin.H)
	for tr := range resultCh {
		instancesByIndex[tr.Index] = tr.Instances
	}

	result := make([]gin.H, 0, len(types))
	for i, rt := range types {
		result = append(result, gin.H{
			"plugin_id":   rt.PluginID,
			"slug":        rt.Slug,
			"name":        sanitizeString(rt.Name),
			"description": sanitizeString(rt.Description),
			"icon":        sanitizeString(rt.Icon),
			"instances":   instancesByIndex[i],
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
			"name":                 sanitizeString(t.Name),
			"description":          sanitizeString(t.Description),
			"icon":                 sanitizeString(t.Icon),
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
			entry["plugin_name"] = sanitizeString(t.Plugin.Name)
		}
		result = append(result, entry)
	}

	c.JSON(http.StatusOK, gin.H{"data": result})
}

// getAppPluginResources returns plugin resource associations for an app.
// Requires the caller to be an admin or the owner of the app.
func (a *API) getAppPluginResources(c *gin.Context) {
	appID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid app ID"})
		return
	}

	// Authorization: verify caller is admin or app owner
	user, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not found in context"})
		return
	}
	currentUser := user.(*models.User)

	app, err := a.service.GetAppByID(uint(appID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "App not found"})
		return
	}
	if !currentUser.IsAdmin && app.UserID != currentUser.ID {
		c.JSON(http.StatusForbidden, gin.H{"error": "You do not have permission to view this app's resources"})
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
				typeName = sanitizeString(apr.PluginResourceType.Name)
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
				typeName = sanitizeString(gpr.PluginResourceType.Name)
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

// setGroupPluginResources replaces plugin resource access for a group.
// Delegates to the service layer which performs all updates atomically in a transaction.
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

	// Batch-resolve all resource type slugs to IDs in a single query
	keys := make([]services.PluginResourceTypeKey, len(input.Resources))
	for i, r := range input.Resources {
		keys[i] = services.PluginResourceTypeKey{PluginID: r.PluginID, Slug: r.ResourceTypeSlug}
	}
	typeMap, err := a.service.GetPluginResourceTypesBatch(keys)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var updates []services.GroupPluginResourceUpdate
	for _, r := range input.Resources {
		key := services.PluginResourceTypeKey{PluginID: r.PluginID, Slug: r.ResourceTypeSlug}
		prt, ok := typeMap[key]
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Unknown resource type: " + r.ResourceTypeSlug})
			return
		}
		updates = append(updates, services.GroupPluginResourceUpdate{
			ResourceTypeID: prt.ID,
			InstanceIDs:    r.InstanceIDs,
		})
	}

	if err := a.service.SetGroupPluginResourcesBatch(uint(groupID), updates); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}
