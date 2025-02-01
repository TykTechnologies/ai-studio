package api

import (
	"fmt"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/gin-gonic/gin"
	"net/http"
	"strconv"
	"strings"
)

func (a *API) createSCIMUser(c *gin.Context) {
	var input models.SCIMUserRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, models.SCIMErrorResponse{
			Schemas: []string{"urn:ietf:params:scim:api:messages:2.0:Error"},
			Detail:  err.Error(),
			Status:  http.StatusBadRequest,
		})
		return
	}

	// Extract SCIM-compliant fields
	email := ""
	if len(input.Emails) > 0 {
		email = input.Emails[0].Value
	}

	fullName := strings.TrimSpace(input.Name.GivenName + " " + input.Name.FamilyName)

	user, err := a.service.CreateUser(
		email,
		fullName, // SCIM uses "userName" as a unique identifier
		"",       // SCIM does not provide passwords in requests
		false,    // Default: non-admin, unless role handling is added
		false,    // Default values for ShowChat
		false,    // Default values for ShowPortal
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.SCIMErrorResponse{
			Schemas: []string{"urn:ietf:params:scim:api:messages:2.0:Error"},
			Detail:  err.Error(),
			Status:  http.StatusInternalServerError,
		})
		return
	}

	// SCIM-compliant response with user resource
	response := models.SCIMUserResponse{
		Schemas:  []string{"urn:ietf:params:scim:schemas:core:2.0:User"},
		ID:       fmt.Sprintf("%d", user.ID),
		UserName: user.Email,
		Name:     user.ToSCIMName(),
		Emails: []models.SCIMEmail{
			{Value: user.Email, Type: "work"},
		},
		Meta: models.SCIMMeta{
			ResourceType: "User",
			Location:     fmt.Sprintf("%s/scim/v2/Users/%d", c.Request.Host, user.ID),
		},
	}

	c.Header("Location", response.Meta.Location)
	c.JSON(http.StatusCreated, response)
}

func (a *API) getSCIMUser(c *gin.Context) {
	// Extract user ID from the URL
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.SCIMErrorResponse{
			Schemas: []string{"urn:ietf:params:scim:api:messages:2.0:Error"},
			Detail:  "Invalid user ID",
			Status:  http.StatusBadRequest,
		})
		return
	}

	// Fetch user from service
	user, err := a.service.GetUserByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, models.SCIMErrorResponse{
			Schemas: []string{"urn:ietf:params:scim:api:messages:2.0:Error"},
			Detail:  "User not found",
			Status:  http.StatusNotFound,
		})
		return
	}

	// Convert DB name to SCIM-compliant format
	scimName := user.ToSCIMName()

	// Construct SCIM-compliant response
	response := models.SCIMUserResponse{
		Schemas:  []string{"urn:ietf:params:scim:schemas:core:2.0:User"},
		ID:       strconv.FormatUint(uint64(user.ID), 10),
		UserName: user.Email,
		Name:     scimName,
		Emails: []models.SCIMEmail{
			{Value: user.Email, Type: "work"},
		},
		Meta: models.SCIMMeta{
			ResourceType: "User",
			Location:     "https://chat.tyk.technology/scim/v2/Users/" + strconv.FormatUint(uint64(user.ID), 10),
		},
	}

	// Return SCIM-compliant response
	c.JSON(http.StatusOK, response)
}

// List SCIM users (paginated)
func (a *API) listSCIMUsers(c *gin.Context) {
	pageSize, pageNumber, _ := getPaginationParams(c)

	users, totalCount, _, err := a.service.GetAllUsers(pageSize, pageNumber, false)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.SCIMErrorResponse{
			Schemas: []string{"urn:ietf:params:scim:api:messages:2.0:Error"},
			Detail:  err.Error(),
			Status:  http.StatusInternalServerError,
		})
		return
	}

	// Serialize SCIM-compliant users
	scimUsers := make([]models.SCIMUserResponse, len(users))
	for i, user := range users {
		scimUsers[i] = models.SCIMUserResponse{
			Schemas:  []string{"urn:ietf:params:scim:schemas:core:2.0:User"},
			ID:       strconv.FormatUint(uint64(user.ID), 10),
			UserName: user.Email,
			Name:     user.ToSCIMName(),
			Emails: []models.SCIMEmail{
				{Value: user.Email, Type: "work"},
			},
			Meta: models.SCIMMeta{
				ResourceType: "User",
				Location:     fmt.Sprintf("https://chat.tyk.technology/scim/v2/Users/%d", user.ID),
			},
		}
	}

	// SCIM response with pagination headers
	c.Header("X-Total-Count", strconv.FormatInt(totalCount, 10))
	c.JSON(http.StatusOK, gin.H{
		"schemas":      []string{"urn:ietf:params:scim:schemas:core:2.0:ListResponse"},
		"totalResults": totalCount,
		"startIndex":   pageNumber,
		"itemsPerPage": pageSize,
		"Resources":    scimUsers,
	})
}

// Fully replace a SCIM user (PUT /Users/{id})
func (a *API) updateSCIMUser(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.SCIMErrorResponse{
			Schemas: []string{"urn:ietf:params:scim:api:messages:2.0:Error"},
			Detail:  "Invalid user ID",
			Status:  http.StatusBadRequest,
		})
		return
	}

	var input models.SCIMUserRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, models.SCIMErrorResponse{
			Schemas: []string{"urn:ietf:params:scim:api:messages:2.0:Error"},
			Detail:  err.Error(),
			Status:  http.StatusBadRequest,
		})
		return
	}

	fullName := strings.TrimSpace(input.Name.GivenName + " " + input.Name.FamilyName)
	email := ""
	if len(input.Emails) > 0 {
		email = input.Emails[0].Value
	}

	user, err := a.service.UpdateUser(uint(id), email, fullName, false, false, false)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.SCIMErrorResponse{
			Schemas: []string{"urn:ietf:params:scim:api:messages:2.0:Error"},
			Detail:  err.Error(),
			Status:  http.StatusInternalServerError,
		})
		return
	}

	response := models.SCIMUserResponse{
		Schemas:  []string{"urn:ietf:params:scim:schemas:core:2.0:User"},
		ID:       strconv.FormatUint(uint64(user.ID), 10),
		UserName: user.Email,
		Name:     user.ToSCIMName(),
		Emails: []models.SCIMEmail{
			{Value: user.Email, Type: "work"},
		},
		Meta: models.SCIMMeta{
			ResourceType: "User",
			Location:     fmt.Sprintf("https://chat.tyk.technology/scim/v2/Users/%d", user.ID),
		},
	}

	c.JSON(http.StatusOK, response)
}

// Partially update a SCIM user (PATCH /Users/{id})
func (a *API) patchSCIMUser(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.SCIMErrorResponse{
			Schemas: []string{"urn:ietf:params:scim:api:messages:2.0:Error"},
			Detail:  "Invalid user ID",
			Status:  http.StatusBadRequest,
		})
		return
	}

	var patchRequest models.SCIMPatchRequest
	if err := c.ShouldBindJSON(&patchRequest); err != nil {
		c.JSON(http.StatusBadRequest, models.SCIMErrorResponse{
			Schemas: []string{"urn:ietf:params:scim:api:messages:2.0:Error"},
			Detail:  err.Error(),
			Status:  http.StatusBadRequest,
		})
		return
	}

	user, err := a.service.GetUserByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, models.SCIMErrorResponse{
			Schemas: []string{"urn:ietf:params:scim:api:messages:2.0:Error"},
			Detail:  "User not found",
			Status:  http.StatusNotFound,
		})
		return
	}

	// Apply patch operations
	for _, op := range patchRequest.Operations {
		switch op.Op {
		case "replace":
			if op.Path == "emails" {
				user.Email = op.Value.(string)
			} else if op.Path == "name.givenName" {
				user.Name = strings.TrimSpace(op.Value.(string) + " " + user.ToSCIMName().FamilyName)
			} else if op.Path == "name.familyName" {
				user.Name = strings.TrimSpace(user.ToSCIMName().GivenName + " " + op.Value.(string))
			}
		}
	}

	// Save updated user
	updatedUser, err := a.service.UpdateUser(uint(id), user.Email, user.Name, false, false, false)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.SCIMErrorResponse{
			Schemas: []string{"urn:ietf:params:scim:api:messages:2.0:Error"},
			Detail:  err.Error(),
			Status:  http.StatusInternalServerError,
		})
		return
	}

	c.JSON(http.StatusOK, serializeSCIMUser(updatedUser))
}

func serializeSCIMUser(user *models.User) models.SCIMUserResponse {
	return models.SCIMUserResponse{
		Schemas:  []string{"urn:ietf:params:scim:schemas:core:2.0:User"},
		ID:       strconv.FormatUint(uint64(user.ID), 10),
		UserName: user.Email, // SCIM uses userName as the unique identifier
		Name:     user.ToSCIMName(),
		Emails: []models.SCIMEmail{
			{Value: user.Email, Type: "work"},
		},
		Meta: models.SCIMMeta{
			ResourceType: "User",
			Location:     fmt.Sprintf("https://chat.tyk.technology/scim/v2/Users/%d", user.ID),
		},
	}
}

// Soft delete a SCIM user (DELETE /Users/{id})
func (a *API) deleteSCIMUser(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.SCIMErrorResponse{
			Schemas: []string{"urn:ietf:params:scim:api:messages:2.0:Error"},
			Detail:  "Invalid user ID",
			Status:  http.StatusBadRequest,
		})
		return
	}

	// Soft delete user (set inactive)
	err = a.service.DeleteUser(uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.SCIMErrorResponse{
			Schemas: []string{"urn:ietf:params:scim:api:messages:2.0:Error"},
			Detail:  err.Error(),
			Status:  http.StatusInternalServerError,
		})
		return
	}

	c.Status(http.StatusNoContent)
}

func (a *API) listSCIMGroups(c *gin.Context) {
	pageSize, pageNumber, all := getPaginationParams(c)

	groups, totalCount, totalPages, err := a.service.GetAllGroups(pageSize, pageNumber, all)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.SCIMErrorResponse{
			Schemas: []string{"urn:ietf:params:scim:api:messages:2.0:Error"},
			Detail:  err.Error(),
			Status:  http.StatusInternalServerError,
		})
		return
	}

	scimGroups := make([]models.SCIMGroupResponse, len(groups))
	for i, group := range groups {
		scimGroups[i] = serializeSCIMGroup(&group)
	}

	c.Header("X-Total-Count", strconv.FormatInt(totalCount, 10))
	c.Header("X-Total-Pages", strconv.Itoa(totalPages))
	c.JSON(http.StatusOK, gin.H{
		"schemas":      []string{"urn:ietf:params:scim:schemas:core:2.0:ListResponse"},
		"totalResults": totalCount,
		"startIndex":   pageNumber,
		"itemsPerPage": pageSize,
		"Resources":    scimGroups,
	})
}

func (a *API) createSCIMGroup(c *gin.Context) {
	var input models.SCIMGroupRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, models.SCIMErrorResponse{
			Schemas: []string{"urn:ietf:params:scim:api:messages:2.0:Error"},
			Detail:  err.Error(),
			Status:  http.StatusBadRequest,
		})
		return
	}

	group, err := a.service.CreateGroup(input.DisplayName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.SCIMErrorResponse{
			Schemas: []string{"urn:ietf:params:scim:api:messages:2.0:Error"},
			Detail:  err.Error(),
			Status:  http.StatusInternalServerError,
		})
		return
	}

	c.Header("Location", fmt.Sprintf("https://chat.tyk.technology/scim/v2/Groups/%d", group.ID))
	c.JSON(http.StatusCreated, serializeSCIMGroup(group))
}

func (a *API) getSCIMGroup(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.SCIMErrorResponse{
			Schemas: []string{"urn:ietf:params:scim:api:messages:2.0:Error"},
			Detail:  "Invalid group ID",
			Status:  http.StatusBadRequest,
		})
		return
	}

	group, err := a.service.GetGroupByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, models.SCIMErrorResponse{
			Schemas: []string{"urn:ietf:params:scim:api:messages:2.0:Error"},
			Detail:  "Group not found",
			Status:  http.StatusNotFound,
		})
		return
	}

	c.JSON(http.StatusOK, serializeSCIMGroup(group))
}

func (a *API) updateSCIMGroup(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.SCIMErrorResponse{
			Schemas: []string{"urn:ietf:params:scim:api:messages:2.0:Error"},
			Detail:  "Invalid group ID",
			Status:  http.StatusBadRequest,
		})
		return
	}

	var input models.SCIMGroupRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, models.SCIMErrorResponse{
			Schemas: []string{"urn:ietf:params:scim:api:messages:2.0:Error"},
			Detail:  err.Error(),
			Status:  http.StatusBadRequest,
		})
		return
	}

	group, err := a.service.UpdateGroup(uint(id), input.DisplayName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.SCIMErrorResponse{
			Schemas: []string{"urn:ietf:params:scim:api:messages:2.0:Error"},
			Detail:  err.Error(),
			Status:  http.StatusInternalServerError,
		})
		return
	}

	c.JSON(http.StatusOK, serializeSCIMGroup(group))
}

func (a *API) patchSCIMGroup(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.SCIMErrorResponse{
			Schemas: []string{"urn:ietf:params:scim:api:messages:2.0:Error"},
			Detail:  "Invalid group ID",
			Status:  http.StatusBadRequest,
		})
		return
	}

	var patchRequest models.SCIMGroupPatchRequest
	if err := c.ShouldBindJSON(&patchRequest); err != nil {
		c.JSON(http.StatusBadRequest, models.SCIMErrorResponse{
			Schemas: []string{"urn:ietf:params:scim:api:messages:2.0:Error"},
			Detail:  err.Error(),
			Status:  http.StatusBadRequest,
		})
		return
	}

	group, err := a.service.GetGroupByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, models.SCIMErrorResponse{
			Schemas: []string{"urn:ietf:params:scim:api:messages:2.0:Error"},
			Detail:  "Group not found",
			Status:  http.StatusNotFound,
		})
		return
	}

	// Apply PATCH operations
	for _, op := range patchRequest.Operations {
		switch op.Op {
		case "add":
			for _, member := range op.Value.([]models.SCIMMember) {
				userID, _ := strconv.ParseUint(member.Value, 10, 32)
				a.service.AddUserToGroup(uint(id), uint(userID))
			}
		case "remove":
			for _, member := range op.Value.([]models.SCIMMember) {
				userID, _ := strconv.ParseUint(member.Value, 10, 32)
				a.service.RemoveUserFromGroup(uint(id), uint(userID))
			}
		}
	}

	c.JSON(http.StatusOK, serializeSCIMGroup(group))
}

func (a *API) deleteSCIMGroup(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.SCIMErrorResponse{
			Schemas: []string{"urn:ietf:params:scim:api:messages:2.0:Error"},
			Detail:  "Invalid group ID",
			Status:  http.StatusBadRequest,
		})
		return
	}

	err = a.service.DeleteGroup(uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.SCIMErrorResponse{
			Schemas: []string{"urn:ietf:params:scim:api:messages:2.0:Error"},
			Detail:  err.Error(),
			Status:  http.StatusInternalServerError,
		})
		return
	}

	c.Status(http.StatusNoContent)
}

func serializeSCIMGroup(group *models.Group) models.SCIMGroupResponse {
	members := make([]models.SCIMMember, len(group.Users))
	for i, user := range group.Users {
		members[i] = models.SCIMMember{
			Value: strconv.FormatUint(uint64(user.ID), 10),
			Ref:   fmt.Sprintf("https://chat.tyk.technology/scim/v2/Users/%d", user.ID),
		}
	}

	return models.SCIMGroupResponse{
		Schemas:     []string{"urn:ietf:params:scim:schemas:core:2.0:Group"},
		ID:          strconv.FormatUint(uint64(group.ID), 10),
		DisplayName: group.Name,
		Members:     members,
		Meta: models.SCIMMeta{
			ResourceType: "Group",
			Location:     fmt.Sprintf("https://chat.tyk.technology/scim/v2/Groups/%d", group.ID),
		},
	}
}
