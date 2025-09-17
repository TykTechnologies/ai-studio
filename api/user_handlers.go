package api

import (
	"net/http"
	"strconv"

	"github.com/TykTechnologies/midsommar/v2/helpers"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/gin-gonic/gin"
)

func (a *API) validateAdminPermissions(c *gin.Context) error {
	currentUser, exists := c.Get("user")
	if !exists {
		return helpers.NewUnauthorizedError("User not authenticated")
	}

	u, ok := currentUser.(*models.User)
	if !ok {
		return helpers.NewUnauthorizedError("User not authenticated")
	}

	if !u.IsAdmin {
		return helpers.NewForbiddenError("operation only allowed for admin users")
	}

	return nil
}

func (a *API) validateUserInput(userInput UserInput, userId uint) error {
	isUnique, err := models.IsEmailUnique(a.service.DB, userInput.Data.Attributes.Email, userId)
	if err != nil {
		return err
	}

	if !isUnique {
		return helpers.NewBadRequestError("Email is already in use")
	}

	return nil
}

// @Summary Create a new user
// @Description Create a new user with the provided information
// @Tags users
// @Accept json
// @Produce json
// @Param user body UserInput true "User information"
// @Success 201 {object} UserResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /users [post]
// @Security BearerAuth
func (a *API) createUser(c *gin.Context) {
	var input UserInput
	if err := c.ShouldBindJSON(&input); err != nil {
		helpers.SendErrorResponse(c, helpers.NewBadRequestError(err.Error()))
		return
	}

	if err := a.validateUserInput(input, 0); err != nil {
		helpers.SendErrorResponse(c, err)
		return
	}

	if input.Data.Attributes.IsAdmin {
		if err := a.validateAdminPermissions(c); err != nil {
			helpers.SendErrorResponse(c, err)
			return
		}
	}

	user, err := a.service.CreateUser(services.UserDTO{
		Email:                input.Data.Attributes.Email,
		Name:                 input.Data.Attributes.Name,
		Password:             input.Data.Attributes.Password,
		IsAdmin:              input.Data.Attributes.IsAdmin,
		ShowChat:             input.Data.Attributes.ShowChat,
		ShowPortal:           input.Data.Attributes.ShowPortal,
		EmailVerified:        input.Data.Attributes.EmailVerified,
		NotificationsEnabled: input.Data.Attributes.NotificationsEnabled,
		AccessToSSOConfig:    input.Data.Attributes.AccessToSSOConfig,
		Groups:               input.Data.Attributes.Groups,
	})
	if err != nil {
		helpers.SendErrorResponse(c, err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": serializeUser(user)})
}

// @Summary Get a user by ID
// @Description Get details of a user by their ID
// @Tags users
// @Accept json
// @Produce json
// @Param id path int true "User ID"
// @Success 200 {object} UserResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /users/{id} [get]
// @Security BearerAuth
func (a *API) getUser(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid user ID"}},
		})
		return
	}

	user, err := a.service.GetUserByID(uint(id), "Groups")
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Not Found", Detail: "User not found"}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": serializeUser(user)})
}

// @Summary Update a user
// @Description Update an existing user's information
// @Tags users
// @Accept json
// @Produce json
// @Param id path int true "User ID"
// @Param user body UserInput true "Updated user information"
// @Success 200 {object} UserResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /users/{id} [patch]
// @Security BearerAuth
func (a *API) updateUser(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		helpers.SendErrorResponse(c, helpers.NewBadRequestError("Invalid user ID"))
		return
	}

	var input UserInput
	if err := c.ShouldBindJSON(&input); err != nil {
		helpers.SendErrorResponse(c, helpers.NewBadRequestError(err.Error()))
		return
	}

	if err := a.validateUserInput(input, uint(id)); err != nil {
		helpers.SendErrorResponse(c, err)
		return
	}

	user, err := a.service.GetUserByID(uint(id))
	if err != nil {
		helpers.SendErrorResponse(c, err)
		return
	}

	if user.IsAdmin {
		if err := a.validateAdminPermissions(c); err != nil {
			helpers.SendErrorResponse(c, err)
			return
		}
	}

	updatedUser, err := a.service.UpdateUser(
		user,
		services.UserDTO{
			Email:                input.Data.Attributes.Email,
			Name:                 input.Data.Attributes.Name,
			IsAdmin:              input.Data.Attributes.IsAdmin,
			ShowChat:             input.Data.Attributes.ShowChat,
			ShowPortal:           input.Data.Attributes.ShowPortal,
			EmailVerified:        input.Data.Attributes.EmailVerified,
			NotificationsEnabled: input.Data.Attributes.NotificationsEnabled,
			AccessToSSOConfig:    input.Data.Attributes.AccessToSSOConfig,
			Groups:               input.Data.Attributes.Groups,
		},
	)
	if err != nil {
		helpers.SendErrorResponse(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": serializeUser(updatedUser)})
}

// @Summary Delete a user
// @Description Delete a user by their ID
// @Tags users
// @Accept json
// @Produce json
// @Param id path int true "User ID"
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /users/{id} [delete]
// @Security BearerAuth
func (a *API) deleteUser(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		helpers.SendErrorResponse(c, helpers.NewBadRequestError("Invalid user ID"))
		return
	}

	user, err := a.service.GetUserByID(uint(id))
	if err != nil {
		helpers.SendErrorResponse(c, err)
		return
	}

	if user.IsAdmin {
		if err := a.validateAdminPermissions(c); err != nil {
			helpers.SendErrorResponse(c, err)
			return
		}
	}

	err = a.service.DeleteUser(user)
	if err != nil {
		helpers.SendErrorResponse(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

// @Summary List all users with optional search filter
// @Description Get a list of all users, optionally filtered by search term
// @Tags users
// @Accept json
// @Produce json
// @Param search query string false "Search term for filtering users by email or name"
// @Success 200 {array} UserResponse
// @Failure 500 {object} ErrorResponse
// @Router /users [get]
// @Security BearerAuth
func (a *API) listUsers(c *gin.Context) {
	pageSize, pageNumber, all := getPaginationParams(c)
	sort := c.Query("sort")
	searchTerm := c.Query("search")
	excludeGroupID := c.Query("exclude_group_id")

	var excludeGroupIDInt int
	if excludeGroupID != "" {
		id, err := strconv.Atoi(excludeGroupID)
		if err == nil {
			excludeGroupIDInt = id
		}
	}

	params := services.ListUsersParams{
		Search:         searchTerm,
		ExcludeGroupID: excludeGroupIDInt,
		PageSize:       pageSize,
		PageNumber:     pageNumber,
		All:            all,
		Sort:           sort,
	}

	users, totalCount, totalPages, err := a.service.ListUsers(params)

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
	c.JSON(http.StatusOK, gin.H{"data": serializeUsers(users)})
}

func serializeUser(user *models.User) UserResponse {
	response := UserResponse{
		Type: "users",
		ID:   strconv.FormatUint(uint64(user.ID), 10),
		Attributes: struct {
			Email                string          `json:"email"`
			Name                 string          `json:"name"`
			IsAdmin              bool            `json:"is_admin"`
			ShowChat             bool            `json:"show_chat"`
			ShowPortal           bool            `json:"show_portal"`
			EmailVerified        bool            `json:"email_verified"`
			APIKey               string          `json:"api_key"`
			NotificationsEnabled bool            `json:"notifications_enabled"`
			AccessToSSOConfig    bool            `json:"access_to_sso_config"`
			Role                 string          `json:"role"`
			Groups               []GroupResponse `json:"groups,omitempty"`
		}{
			Email:                user.Email,
			Name:                 user.Name,
			IsAdmin:              user.IsAdmin,
			ShowChat:             user.ShowChat,
			ShowPortal:           user.ShowPortal,
			EmailVerified:        user.EmailVerified,
			APIKey:               user.APIKey,
			NotificationsEnabled: user.NotificationsEnabled,
			AccessToSSOConfig:    user.AccessToSSOConfig,
			Role:                 user.GetRole(),
		},
	}

	if len(user.Groups) > 0 {
		response.Attributes.Groups = serializeGroups(user.Groups)
	}

	return response
}

func serializeUsers(users models.Users) []UserResponse {
	result := make([]UserResponse, len(users))
	for i, user := range users {
		response := UserResponse{
			Type: "users",
			ID:   strconv.FormatUint(uint64(user.ID), 10),
			Attributes: struct {
				Email                string          `json:"email"`
				Name                 string          `json:"name"`
				IsAdmin              bool            `json:"is_admin"`
				ShowChat             bool            `json:"show_chat"`
				ShowPortal           bool            `json:"show_portal"`
				EmailVerified        bool            `json:"email_verified"`
				APIKey               string          `json:"api_key"`
				NotificationsEnabled bool            `json:"notifications_enabled"`
				AccessToSSOConfig    bool            `json:"access_to_sso_config"`
				Role                 string          `json:"role"`
				Groups               []GroupResponse `json:"groups,omitempty"`
			}{
				Email:                user.Email,
				Name:                 user.Name,
				IsAdmin:              user.IsAdmin,
				ShowChat:             user.ShowChat,
				ShowPortal:           user.ShowPortal,
				EmailVerified:        user.EmailVerified,
				APIKey:               user.APIKey,
				NotificationsEnabled: user.NotificationsEnabled,
				AccessToSSOConfig:    user.AccessToSSOConfig,
				Role:                 user.GetRole(),
			},
		}

		if len(user.Groups) > 0 {
			response.Attributes.Groups = serializeGroups(user.Groups)
		}

		result[i] = response
	}
	return result
}

// @Summary Get user accessible catalogues
// @Description Get a list of all catalogues accessible to a user
// @Tags users
// @Accept json
// @Produce json
// @Param id path int true "User ID"
// @Success 200 {object} UserAccessibleCataloguesResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /users/{id}/catalogues [get]
// @Security BearerAuth
func (a *API) getUserAccessibleCatalogues(c *gin.Context) {
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

	catalogues, err := a.service.GetUserAccessibleCatalogues(uint(userID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	response := UserAccessibleCataloguesResponse{
		Type: "user_accessible_catalogues",
		ID:   strconv.FormatUint(userID, 10),
		Attributes: struct {
			Catalogues []CatalogueResponse `json:"catalogues"`
		}{
			Catalogues: serializeCatalogues(catalogues),
		},
	}

	c.JSON(http.StatusOK, gin.H{"data": response})
}

// @Summary Roll API Key
// @Description Generate a new API key for a user
// @Tags users
// @Accept json
// @Produce json
// @Param id path int true "User ID"
// @Success 200 {object} UserResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /users/{id}/roll-api-key [post]
// @Security BearerAuth
func (a *API) rollUserAPIKey(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid user ID"}},
		})
		return
	}

	err = a.service.GenerateAPIKeyForUser(uint(id))
	if err != nil {
		status := http.StatusInternalServerError
		title := "Internal Server Error"
		detail := err.Error()

		c.JSON(status, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: title, Detail: detail}},
		})
		return
	}

	user, err := a.service.GetUserByID(uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": serializeUser(user)})
}

// @Summary Skip user quick start wizard
// @Description Set a user's skip_quick_start flag to true
// @Tags users
// @Accept json
// @Produce json
// @Param id path int true "User ID"
// @Success 200 {object} string
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /users/{id}/skip-quick-start [post]
// @Security BearerAuth
func (a *API) skipUserQuickStart(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid user ID"}},
		})
		return
	}

	err = a.service.SkipQuickStartForUser(uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success"})
}
