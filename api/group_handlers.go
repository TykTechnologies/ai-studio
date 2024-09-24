package api

import (
	"net/http"
	"strconv"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// @Summary Create a new group
// @Description Create a new group with the provided information
// @Tags groups
// @Accept json
// @Produce json
// @Param group body GroupInput true "Group information"
// @Success 201 {object} GroupResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /groups [post]
// @Security BearerAuth
func (a *API) createGroup(c *gin.Context) {
	var input GroupInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	group, err := a.service.CreateGroup(input.Data.Attributes.Name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": serializeGroup(group)})
}

// @Summary Get a group by ID
// @Description Get details of a group by its ID
// @Tags groups
// @Accept json
// @Produce json
// @Param id path int true "Group ID"
// @Success 200 {object} GroupResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /groups/{id} [get]
// @Security BearerAuth
func (a *API) getGroup(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid group ID"}},
		})
		return
	}

	group, err := a.service.GetGroupByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Not Found", Detail: "Group not found"}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": serializeGroup(group)})
}

// @Summary Update a group
// @Description Update an existing group's information
// @Tags groups
// @Accept json
// @Produce json
// @Param id path int true "Group ID"
// @Param group body GroupInput true "Updated group information"
// @Success 200 {object} GroupResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /groups/{id} [patch]
// @Security BearerAuth
func (a *API) updateGroup(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid group ID"}},
		})
		return
	}

	var input GroupInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	group, err := a.service.UpdateGroup(uint(id), input.Data.Attributes.Name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": serializeGroup(group)})
}

// @Summary Delete a group
// @Description Delete a group by its ID
// @Tags groups
// @Accept json
// @Produce json
// @Param id path int true "Group ID"
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /groups/{id} [delete]
// @Security BearerAuth
func (a *API) deleteGroup(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid group ID"}},
		})
		return
	}

	err = a.service.DeleteGroup(uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.Status(http.StatusNoContent)
}

// @Summary List all groups
// @Description Get a list of all groups
// @Tags groups
// @Accept json
// @Produce json
// @Success 200 {array} GroupResponse
// @Failure 500 {object} ErrorResponse
// @Router /groups [get]
// @Security BearerAuth
func (a *API) listGroups(c *gin.Context) {
	pageSize, pageNumber, all := getPaginationParams(c)

	groups, totalCount, totalPages, err := a.service.GetAllGroups(pageSize, pageNumber, all)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.Header("X-Total-Count", strconv.FormatInt(totalCount, 10))
	c.Header("X-Total-Pages", strconv.Itoa(totalPages))
	c.JSON(http.StatusOK, gin.H{"data": serializeGroups(groups)})
}

// @Summary Search groups by name
// @Description Search for groups using a name stub
// @Tags groups
// @Accept json
// @Produce json
// @Param name query string true "Name stub to search for"
// @Success 200 {array} GroupResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /groups/search [get]
// @Security BearerAuth
func (a *API) searchGroups(c *gin.Context) {
	nameStub := c.Query("name")
	if nameStub == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Name stub is required"}},
		})
		return
	}

	groups, err := a.service.SearchGroupsByNameStub(nameStub)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": serializeGroups(groups)})
}

// @Summary Add a user to a group
// @Description Add a user to a specific group
// @Tags groups
// @Accept json
// @Produce json
// @Param id path int true "Group ID"
// @Param user body UserGroupInput true "User to add"
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /groups/{id}/users [post]
// @Security BearerAuth
func (a *API) addUserToGroup(c *gin.Context) {
	groupID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid group ID"}},
		})
		return
	}

	var input UserGroupInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	userID, err := strconv.ParseUint(input.Data.ID, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid user ID"}},
		})
		return
	}

	err = a.service.AddUserToGroup(uint(userID), uint(groupID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.Status(http.StatusNoContent)
}

// @Summary Remove a user from a group
// @Description Remove a user from a specific group
// @Tags groups
// @Accept json
// @Produce json
// @Param id path int true "Group ID"
// @Param userId path int true "User ID"
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /groups/{id}/users/{userId} [delete]
// @Security BearerAuth
func (a *API) removeUserFromGroup(c *gin.Context) {
	groupID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid group ID"}},
		})
		return
	}

	userID, err := strconv.ParseUint(c.Param("userId"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid user ID"}},
		})
		return
	}

	err = a.service.RemoveUserFromGroup(uint(userID), uint(groupID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.Status(http.StatusNoContent)
}

// @Summary List users in a group
// @Description Get a list of all users in a specific group
// @Tags groups
// @Accept json
// @Produce json
// @Param id path int true "Group ID"
// @Success 200 {array} UserResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /groups/{id}/users [get]
// @Security BearerAuth
func (a *API) listGroupUsers(c *gin.Context) {
	groupID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid group ID"}},
		})
		return
	}

	users, err := a.service.GetGroupUsers(uint(groupID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": serializeUsers(users)})
}

func serializeGroup(group *models.Group) GroupResponse {
	return GroupResponse{
		Type: "groups",
		ID:   strconv.FormatUint(uint64(group.ID), 10),
		Attributes: struct {
			Name string `json:"name"`
		}{
			Name: group.Name,
		},
	}
}

func serializeGroups(groups models.Groups) []GroupResponse {
	result := make([]GroupResponse, len(groups))
	for i, group := range groups {
		result[i] = serializeGroup(&group)
	}
	return result
}

// Add these new handler functions to the existing file

// @Summary Add a catalogue to a group
// @Description Add a catalogue to a specific group
// @Tags groups
// @Accept json
// @Produce json
// @Param id path int true "Group ID"
// @Param catalogue body GroupCatalogueInput true "Catalogue to add"
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /groups/{id}/catalogues [post]
// @Security BearerAuth
func (a *API) addCatalogueToGroup(c *gin.Context) {
	groupID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid group ID"}},
		})
		return
	}

	var input GroupCatalogueInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	catalogueID, err := strconv.ParseUint(input.Data.ID, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid catalogue ID"}},
		})
		return
	}

	err = a.service.AddCatalogueToGroup(uint(catalogueID), uint(groupID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.Status(http.StatusNoContent)
}

// @Summary Remove a catalogue from a group
// @Description Remove a catalogue from a specific group
// @Tags groups
// @Accept json
// @Produce json
// @Param id path int true "Group ID"
// @Param catalogueId path int true "Catalogue ID"
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /groups/{id}/catalogues/{catalogueId} [delete]
// @Security BearerAuth
func (a *API) removeCatalogueFromGroup(c *gin.Context) {
	groupID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid group ID"}},
		})
		return
	}

	catalogueID, err := strconv.ParseUint(c.Param("catalogueId"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid catalogue ID"}},
		})
		return
	}

	err = a.service.RemoveCatalogueFromGroup(uint(catalogueID), uint(groupID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.Status(http.StatusNoContent)
}

// @Summary List catalogues in a group
// @Description Get a list of all catalogues in a specific group
// @Tags groups
// @Accept json
// @Produce json
// @Param id path int true "Group ID"
// @Success 200 {array} CatalogueResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /groups/{id}/catalogues [get]
// @Security BearerAuth
func (a *API) listGroupCatalogues(c *gin.Context) {
	groupID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid group ID"}},
		})
		return
	}

	catalogues, err := a.service.GetGroupCatalogues(uint(groupID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": serializeCatalogues(catalogues)})
}

// Add this function to group_handlers.go

// @Summary Get groups for a user
// @Description Get a list of all groups a specific user belongs to
// @Tags groups
// @Accept json
// @Produce json
// @Param userId path int true "User ID"
// @Success 200 {array} GroupResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /users/{userId}/groups [get]
// @Security BearerAuth
func (a *API) getUserGroups(c *gin.Context) {
	userID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid user ID"}},
		})
		return
	}

	groups, err := a.service.GetGroupsByUserID(uint(userID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": serializeGroups(groups)})
}

// @Summary Add a data catalogue to a group
// @Description Add a data catalogue to a specific group
// @Tags groups
// @Accept json
// @Produce json
// @Param id path int true "Group ID"
// @Param dataCatalogue body DataCatalogueInput true "Data Catalogue to add"
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /groups/{id}/data-catalogues [post]
// @Security BearerAuth
func (a *API) addDataCatalogueToGroup(c *gin.Context) {
	groupID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid group ID"}},
		})
		return
	}

	var input GroupDataCatalogueInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	dataCatalogueID, err := strconv.ParseUint(input.Data.ID, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid data catalogue ID"}},
		})
		return
	}

	err = a.service.AddDataCatalogueToGroup(uint(dataCatalogueID), uint(groupID))
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Bad Request", Detail: "Data catalogue or group not found"}},
			})
			return
		}

		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.Status(http.StatusNoContent)
}

// @Summary Remove a data catalogue from a group
// @Description Remove a data catalogue from a specific group
// @Tags groups
// @Accept json
// @Produce json
// @Param id path int true "Group ID"
// @Param dataCatalogueId path int true "Data Catalogue ID"
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /groups/{id}/data-catalogues/{dataCatalogueId} [delete]
// @Security BearerAuth
func (a *API) removeDataCatalogueFromGroup(c *gin.Context) {
	groupID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid group ID"}},
		})
		return
	}

	dataCatalogueID, err := strconv.ParseUint(c.Param("dataCatalogueId"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid data catalogue ID"}},
		})
		return
	}

	err = a.service.RemoveDataCatalogueFromGroup(uint(dataCatalogueID), uint(groupID))
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Bad Request", Detail: "Data catalogue or group not found"}},
			})
			return
		}

		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.Status(http.StatusNoContent)
}

// @Summary List data catalogues in a group
// @Description Get a list of all data catalogues in a specific group
// @Tags groups
// @Accept json
// @Produce json
// @Param id path int true "Group ID"
// @Success 200 {array} DataCatalogueResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /groups/{id}/data-catalogues [get]
// @Security BearerAuth
func (a *API) listGroupDataCatalogues(c *gin.Context) {
	groupID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid group ID"}},
		})
		return
	}

	dataCatalogues, err := a.service.GetGroupDataCatalogues(uint(groupID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": serializeDataCatalogues(dataCatalogues)})
}

// @Summary Add a tool catalogue to a group
// @Description Add a tool catalogue to a specific group
// @Tags groups
// @Accept json
// @Produce json
// @Param id path int true "Group ID"
// @Param toolCatalogue body GroupToolCatalogueInput true "Tool Catalogue to add"
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /groups/{id}/tool-catalogues [post]
// @Security BearerAuth
func (a *API) addToolCatalogueToGroup(c *gin.Context) {
	groupID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid group ID"}},
		})
		return
	}

	var input GroupToolCatalogueInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	toolCatalogueID, err := strconv.ParseUint(input.Data.ID, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid tool catalogue ID"}},
		})
		return
	}

	err = a.service.AddToolCatalogueToGroup(uint(toolCatalogueID), uint(groupID))
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Not Found", Detail: "Tool catalogue or group not found"}},
			})
			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.Status(http.StatusNoContent)
}

// @Summary Remove a tool catalogue from a group
// @Description Remove a tool catalogue from a specific group
// @Tags groups
// @Accept json
// @Produce json
// @Param id path int true "Group ID"
// @Param toolCatalogueId path int true "Tool Catalogue ID"
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /groups/{id}/tool-catalogues/{toolCatalogueId} [delete]
// @Security BearerAuth
func (a *API) removeToolCatalogueFromGroup(c *gin.Context) {
	groupID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid group ID"}},
		})
		return
	}

	toolCatalogueID, err := strconv.ParseUint(c.Param("toolCatalogueId"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid tool catalogue ID"}},
		})
		return
	}

	err = a.service.RemoveToolCatalogueFromGroup(uint(toolCatalogueID), uint(groupID))
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Not Found", Detail: "Tool catalogue or group not found"}},
			})
			return
		}

		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.Status(http.StatusNoContent)
}

// @Summary List tool catalogues in a group
// @Description Get a list of all tool catalogues in a specific group
// @Tags groups
// @Accept json
// @Produce json
// @Param id path int true "Group ID"
// @Success 200 {array} ToolCatalogueResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /groups/{id}/tool-catalogues [get]
// @Security BearerAuth
func (a *API) listGroupToolCatalogues(c *gin.Context) {
	groupID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid group ID"}},
		})
		return
	}

	pageSize, pageNumber, all := getPaginationParams(c)

	toolCatalogues, totalCount, totalPages, err := a.service.GetGroupToolCatalogues(uint(groupID), pageSize, pageNumber, all)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.Header("X-Total-Count", strconv.FormatInt(totalCount, 10))
	c.Header("X-Total-Pages", strconv.Itoa(totalPages))
	c.JSON(http.StatusOK, gin.H{"data": serializeToolCatalogues(toolCatalogues)})
}

// Helper function to serialize ToolCatalogues
func serializeToolCatalogues(toolCatalogues models.ToolCatalogues) []ToolCatalogueResponse {
	result := make([]ToolCatalogueResponse, len(toolCatalogues))
	for i, tc := range toolCatalogues {
		result[i] = serializeToolCatalogue(&tc)
	}
	return result
}

// Helper function to serialize a single ToolCatalogue
func serializeToolCatalogue(tc *models.ToolCatalogue) ToolCatalogueResponse {
	return ToolCatalogueResponse{
		Type: "tool-catalogues",
		ID:   strconv.FormatUint(uint64(tc.ID), 10),
		Attributes: struct {
			Name             string         `json:"name"`
			ShortDescription string         `json:"short_description"`
			LongDescription  string         `json:"long_description"`
			Icon             string         `json:"icon"`
			Tools            []ToolResponse `json:"tools"`
			Tags             []TagResponse  `json:"tags"`
		}{
			Name:             tc.Name,
			ShortDescription: tc.ShortDescription,
			LongDescription:  tc.LongDescription,
			Icon:             tc.Icon,
			Tools:            serializeTools(tc.Tools),
			Tags:             serializeTags(tc.Tags),
		},
	}
}
