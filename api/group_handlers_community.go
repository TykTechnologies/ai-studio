//go:build !enterprise
// +build !enterprise

package api

import (
	"net/http"
	"strconv"

	"github.com/TykTechnologies/midsommar/v2/helpers"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/gin-gonic/gin"
)

// Group management endpoints - CE: All return 402 Payment Required

// @Summary Create a new group
// @Description Create a new group (Enterprise Edition only)
// @Tags groups
// @Accept json
// @Produce json
// @Success 201 {object} map[string]interface{}
// @Failure 402 {object} ErrorResponse
// @Router /api/v1/groups [post]
// @Security BearerAuth
func (a *API) createGroup(c *gin.Context) {
	helpers.SendErrorResponse(c, helpers.NewPaymentRequiredError("Group management requires Enterprise Edition"))
}

// @Summary Get group by ID
// @Description Get details of a specific group (CE: Returns Default group only)
// @Tags groups
// @Accept json
// @Produce json
// @Param id path int true "Group ID"
// @Success 200 {object} map[string]interface{}
// @Failure 404 {object} ErrorResponse
// @Router /api/v1/groups/{id} [get]
// @Security BearerAuth
func (a *API) getGroup(c *gin.Context) {
	// CE: Only allow access to Default group
	defaultGroup, err := models.GetOrCreateDefaultGroup(a.service.DB)
	if err != nil {
		helpers.SendErrorResponse(c, helpers.NewInternalServerError("Failed to get default group"))
		return
	}

	// Check if requested ID matches Default group
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || uint(id) != defaultGroup.ID {
		helpers.SendErrorResponse(c, helpers.NewNotFoundError("Group not found"))
		return
	}

	// Preload associations
	if err := defaultGroup.Get(a.service.DB, defaultGroup.ID, "Users", "Catalogues", "DataCatalogues", "ToolCatalogues"); err != nil {
		helpers.SendErrorResponse(c, helpers.NewInternalServerError("Failed to load group details"))
		return
	}

	// Serialize and return
	response := serializeGroup(defaultGroup)
	c.JSON(http.StatusOK, gin.H{"data": response})
}

// @Summary Update group
// @Description Update an existing group (Enterprise Edition only)
// @Tags groups
// @Accept json
// @Produce json
// @Param id path int true "Group ID"
// @Success 200 {object} map[string]interface{}
// @Failure 402 {object} ErrorResponse
// @Router /api/v1/groups/{id} [patch]
// @Security BearerAuth
func (a *API) updateGroup(c *gin.Context) {
	helpers.SendErrorResponse(c, helpers.NewPaymentRequiredError("Group management requires Enterprise Edition"))
}

// @Summary Delete group
// @Description Delete a specific group (Enterprise Edition only)
// @Tags groups
// @Accept json
// @Produce json
// @Param id path int true "Group ID"
// @Success 204
// @Failure 402 {object} ErrorResponse
// @Router /api/v1/groups/{id} [delete]
// @Security BearerAuth
func (a *API) deleteGroup(c *gin.Context) {
	helpers.SendErrorResponse(c, helpers.NewPaymentRequiredError("Group management requires Enterprise Edition"))
}

// @Summary List groups
// @Description Get all groups (CE: Returns Default group only)
// @Tags groups
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/groups [get]
// @Security BearerAuth
func (a *API) listGroups(c *gin.Context) {
	// CE: Return only the Default group
	defaultGroup, err := models.GetOrCreateDefaultGroup(a.service.DB)
	if err != nil {
		helpers.SendErrorResponse(c, helpers.NewInternalServerError("Failed to get default group"))
		return
	}

	// Preload associations for the default group
	if err := defaultGroup.Get(a.service.DB, defaultGroup.ID, "Users", "Catalogues", "DataCatalogues", "ToolCatalogues"); err != nil {
		helpers.SendErrorResponse(c, helpers.NewInternalServerError("Failed to load group details"))
		return
	}

	// Serialize response
	response := GroupListResponse{
		Type: "groups",
		ID:   strconv.FormatUint(uint64(defaultGroup.ID), 10),
	}
	response.Attributes.Name = defaultGroup.Name
	response.Attributes.UserCount = len(defaultGroup.Users)
	response.Attributes.CatalogueCount = len(defaultGroup.Catalogues)
	response.Attributes.DataCatalogueCount = len(defaultGroup.DataCatalogues)
	response.Attributes.ToolCatalogueCount = len(defaultGroup.ToolCatalogues)

	// Extract catalogue names
	catalogueNames := make([]string, len(defaultGroup.Catalogues))
	for i, cat := range defaultGroup.Catalogues {
		catalogueNames[i] = cat.Name
	}
	response.Attributes.CatalogueNames = catalogueNames

	dataCatalogueNames := make([]string, len(defaultGroup.DataCatalogues))
	for i, cat := range defaultGroup.DataCatalogues {
		dataCatalogueNames[i] = cat.Name
	}
	response.Attributes.DataCatalogueNames = dataCatalogueNames

	toolCatalogueNames := make([]string, len(defaultGroup.ToolCatalogues))
	for i, cat := range defaultGroup.ToolCatalogues {
		toolCatalogueNames[i] = cat.Name
	}
	response.Attributes.ToolCatalogueNames = toolCatalogueNames

	c.JSON(http.StatusOK, gin.H{
		"data": []GroupListResponse{response},
		"meta": gin.H{
			"total_count": 1,
			"page_size":   1,
			"page_number": 1,
			"total_pages": 1,
		},
	})
}

// @Summary Add user to group
// @Description Add a user to a specific group (Enterprise Edition only)
// @Tags groups
// @Accept json
// @Produce json
// @Param id path int true "Group ID"
// @Success 200 {object} map[string]interface{}
// @Failure 402 {object} ErrorResponse
// @Router /api/v1/groups/{id}/users [post]
// @Security BearerAuth
func (a *API) addUserToGroup(c *gin.Context) {
	helpers.SendErrorResponse(c, helpers.NewPaymentRequiredError("Group management requires Enterprise Edition"))
}

// @Summary Remove user from group
// @Description Remove a user from a specific group (Enterprise Edition only)
// @Tags groups
// @Accept json
// @Produce json
// @Param id path int true "Group ID"
// @Param userId path int true "User ID"
// @Success 204
// @Failure 402 {object} ErrorResponse
// @Router /api/v1/groups/{id}/users/{userId} [delete]
// @Security BearerAuth
func (a *API) removeUserFromGroup(c *gin.Context) {
	helpers.SendErrorResponse(c, helpers.NewPaymentRequiredError("Group management requires Enterprise Edition"))
}

// @Summary List group users
// @Description Get all users in a specific group (Enterprise Edition only)
// @Tags groups
// @Accept json
// @Produce json
// @Param id path int true "Group ID"
// @Success 200 {object} map[string]interface{}
// @Failure 402 {object} ErrorResponse
// @Router /api/v1/groups/{id}/users [get]
// @Security BearerAuth
func (a *API) listGroupUsers(c *gin.Context) {
	helpers.SendErrorResponse(c, helpers.NewPaymentRequiredError("Group management requires Enterprise Edition"))
}

// @Summary Update group users
// @Description Bulk update users in a specific group (Enterprise Edition only)
// @Tags groups
// @Accept json
// @Produce json
// @Param id path int true "Group ID"
// @Success 200 {object} map[string]interface{}
// @Failure 402 {object} ErrorResponse
// @Router /api/v1/groups/{id}/users [put]
// @Security BearerAuth
func (a *API) updateGroupUsers(c *gin.Context) {
	helpers.SendErrorResponse(c, helpers.NewPaymentRequiredError("Group management requires Enterprise Edition"))
}

// @Summary Add LLM catalogue to group
// @Description Add an LLM catalogue to a specific group (Enterprise Edition only)
// @Tags groups
// @Accept json
// @Produce json
// @Param id path int true "Group ID"
// @Success 200 {object} map[string]interface{}
// @Failure 402 {object} ErrorResponse
// @Router /api/v1/groups/{id}/catalogues [post]
// @Security BearerAuth
func (a *API) addCatalogueToGroup(c *gin.Context) {
	helpers.SendErrorResponse(c, helpers.NewPaymentRequiredError("Group management requires Enterprise Edition"))
}

// @Summary Remove LLM catalogue from group
// @Description Remove an LLM catalogue from a specific group (Enterprise Edition only)
// @Tags groups
// @Accept json
// @Produce json
// @Param id path int true "Group ID"
// @Param catalogueId path int true "Catalogue ID"
// @Success 204
// @Failure 402 {object} ErrorResponse
// @Router /api/v1/groups/{id}/catalogues/{catalogueId} [delete]
// @Security BearerAuth
func (a *API) removeCatalogueFromGroup(c *gin.Context) {
	helpers.SendErrorResponse(c, helpers.NewPaymentRequiredError("Group management requires Enterprise Edition"))
}

// @Summary List group LLM catalogues
// @Description Get all LLM catalogues for a specific group (Enterprise Edition only)
// @Tags groups
// @Accept json
// @Produce json
// @Param id path int true "Group ID"
// @Success 200 {object} map[string]interface{}
// @Failure 402 {object} ErrorResponse
// @Router /api/v1/groups/{id}/catalogues [get]
// @Security BearerAuth
func (a *API) listGroupCatalogues(c *gin.Context) {
	helpers.SendErrorResponse(c, helpers.NewPaymentRequiredError("Group management requires Enterprise Edition"))
}

// @Summary Get user's groups
// @Description Get all groups for a specific user (CE: Returns Default group only)
// @Tags groups
// @Accept json
// @Produce json
// @Param userId path int true "User ID"
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/users/{userId}/groups [get]
// @Security BearerAuth
func (a *API) getUserGroups(c *gin.Context) {
	// CE: All users are in Default group, return it
	defaultGroup, err := models.GetOrCreateDefaultGroup(a.service.DB)
	if err != nil {
		helpers.SendErrorResponse(c, helpers.NewInternalServerError("Failed to get default group"))
		return
	}

	// Simple response with just the Default group
	response := GroupResponse{
		Type: "groups",
		ID:   strconv.FormatUint(uint64(defaultGroup.ID), 10),
		Attributes: struct {
			Name           string                  `json:"name"`
			Users          []UserResponse          `json:"users,omitempty"`
			Catalogues     []CatalogueResponse     `json:"catalogues,omitempty"`
			DataCatalogues []DataCatalogueResponse `json:"data_catalogues,omitempty"`
			ToolCatalogues []ToolCatalogueResponse `json:"tool_catalogues,omitempty"`
		}{
			Name: defaultGroup.Name,
		},
	}

	c.JSON(http.StatusOK, gin.H{"data": []GroupResponse{response}})
}

// @Summary Add data catalogue to group
// @Description Add a data catalogue to a specific group (Enterprise Edition only)
// @Tags groups
// @Accept json
// @Produce json
// @Param id path int true "Group ID"
// @Success 200 {object} map[string]interface{}
// @Failure 402 {object} ErrorResponse
// @Router /api/v1/groups/{id}/data-catalogues [post]
// @Security BearerAuth
func (a *API) addDataCatalogueToGroup(c *gin.Context) {
	helpers.SendErrorResponse(c, helpers.NewPaymentRequiredError("Group management requires Enterprise Edition"))
}

// @Summary Remove data catalogue from group
// @Description Remove a data catalogue from a specific group (Enterprise Edition only)
// @Tags groups
// @Accept json
// @Produce json
// @Param id path int true "Group ID"
// @Param catalogueId path int true "Data Catalogue ID"
// @Success 204
// @Failure 402 {object} ErrorResponse
// @Router /api/v1/groups/{id}/data-catalogues/{catalogueId} [delete]
// @Security BearerAuth
func (a *API) removeDataCatalogueFromGroup(c *gin.Context) {
	helpers.SendErrorResponse(c, helpers.NewPaymentRequiredError("Group management requires Enterprise Edition"))
}

// @Summary List group data catalogues
// @Description Get all data catalogues for a specific group (Enterprise Edition only)
// @Tags groups
// @Accept json
// @Produce json
// @Param id path int true "Group ID"
// @Success 200 {object} map[string]interface{}
// @Failure 402 {object} ErrorResponse
// @Router /api/v1/groups/{id}/data-catalogues [get]
// @Security BearerAuth
func (a *API) listGroupDataCatalogues(c *gin.Context) {
	helpers.SendErrorResponse(c, helpers.NewPaymentRequiredError("Group management requires Enterprise Edition"))
}

// @Summary Add tool catalogue to group
// @Description Add a tool catalogue to a specific group (Enterprise Edition only)
// @Tags groups
// @Accept json
// @Produce json
// @Param id path int true "Group ID"
// @Success 200 {object} map[string]interface{}
// @Failure 402 {object} ErrorResponse
// @Router /api/v1/groups/{id}/tool-catalogues [post]
// @Security BearerAuth
func (a *API) addToolCatalogueToGroup(c *gin.Context) {
	helpers.SendErrorResponse(c, helpers.NewPaymentRequiredError("Group management requires Enterprise Edition"))
}

// @Summary Remove tool catalogue from group
// @Description Remove a tool catalogue from a specific group (Enterprise Edition only)
// @Tags groups
// @Accept json
// @Produce json
// @Param id path int true "Group ID"
// @Param catalogueId path int true "Tool Catalogue ID"
// @Success 204
// @Failure 402 {object} ErrorResponse
// @Router /api/v1/groups/{id}/tool-catalogues/{catalogueId} [delete]
// @Security BearerAuth
func (a *API) removeToolCatalogueFromGroup(c *gin.Context) {
	helpers.SendErrorResponse(c, helpers.NewPaymentRequiredError("Group management requires Enterprise Edition"))
}

// @Summary List group tool catalogues
// @Description Get all tool catalogues for a specific group (Enterprise Edition only)
// @Tags groups
// @Accept json
// @Produce json
// @Param id path int true "Group ID"
// @Success 200 {object} map[string]interface{}
// @Failure 402 {object} ErrorResponse
// @Router /api/v1/groups/{id}/tool-catalogues [get]
// @Security BearerAuth
func (a *API) listGroupToolCatalogues(c *gin.Context) {
	helpers.SendErrorResponse(c, helpers.NewPaymentRequiredError("Group management requires Enterprise Edition"))
}

// @Summary Update group catalogues
// @Description Bulk update all catalogues (LLM, data, tool) for a group (Enterprise Edition only)
// @Tags groups
// @Accept json
// @Produce json
// @Param id path int true "Group ID"
// @Success 200 {object} map[string]interface{}
// @Failure 402 {object} ErrorResponse
// @Router /api/v1/groups/{id}/catalogues [put]
// @Security BearerAuth
func (a *API) updateGroupCatalogues(c *gin.Context) {
	helpers.SendErrorResponse(c, helpers.NewPaymentRequiredError("Group management requires Enterprise Edition"))
}

// Serialization helper (needed by GET endpoints)
func serializeGroup(group *models.Group) GroupResponse {
	response := GroupResponse{
		Type: "groups",
		ID:   strconv.FormatUint(uint64(group.ID), 10),
		Attributes: struct {
			Name           string                  `json:"name"`
			Users          []UserResponse          `json:"users,omitempty"`
			Catalogues     []CatalogueResponse     `json:"catalogues,omitempty"`
			DataCatalogues []DataCatalogueResponse `json:"data_catalogues,omitempty"`
			ToolCatalogues []ToolCatalogueResponse `json:"tool_catalogues,omitempty"`
		}{
			Name: group.Name,
		},
	}

	if len(group.Users) > 0 {
		response.Attributes.Users = serializeUsers(group.Users)
	}

	if len(group.Catalogues) > 0 {
		response.Attributes.Catalogues = serializeCatalogues(group.Catalogues)
	}

	if len(group.DataCatalogues) > 0 {
		response.Attributes.DataCatalogues = serializeDataCatalogues(group.DataCatalogues)
	}

	if len(group.ToolCatalogues) > 0 {
		response.Attributes.ToolCatalogues = serializeToolCatalogues(group.ToolCatalogues, nil)
	}

	return response
}
