package api

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/TykTechnologies/midsommar/v2/helpers"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/gin-gonic/gin"
)

const (
	provOpenID  = "openid-connect"
	provLDAP    = "ldap"
	provSAML    = "saml"
	userProfile = "profile for users"
)

func serializeProfile(profile *models.Profile) ProfileResponse {
	accessor := helpers.NewJSONMapAccessor(profile.ProviderConfig)
	callbackBaseURL := accessor.GetString("CallbackBaseURL", "")

	if profile.SelectedProviderType == provSAML {
		callbackBaseURL = accessor.GetString("SAMLBaseURL", "")
	}

	failureRedirect := accessor.GetString("FailureRedirect", "")

	resp := ProfileResponse{
		Type: "sso-profiles",
		ID:   profile.Model.ID,
	}

	resp.Attributes.Name = profile.Name
	resp.Attributes.OrgID = profile.OrgID
	resp.Attributes.ActionType = profile.ActionType
	resp.Attributes.MatchedPolicyID = profile.MatchedPolicyID
	resp.Attributes.Type = profile.Type
	resp.Attributes.ProviderName = profile.ProviderName
	resp.Attributes.CustomEmailField = profile.CustomEmailField
	resp.Attributes.CustomUserIDField = profile.CustomUserIDField
	resp.Attributes.ProviderConfig = profile.ProviderConfig
	resp.Attributes.IdentityHandlerConfig = profile.IdentityHandlerConfig
	resp.Attributes.ProviderConstraintsDomain = profile.ProviderConstraintsDomain
	resp.Attributes.ProviderConstraintsGroup = profile.ProviderConstraintsGroup
	resp.Attributes.ReturnURL = profile.ReturnURL
	resp.Attributes.DefaultUserGroupID = profile.DefaultUserGroupID
	resp.Attributes.CustomUserGroupField = profile.CustomUserGroupField
	resp.Attributes.UserGroupMapping = profile.UserGroupMapping
	resp.Attributes.UserGroupSeparator = profile.UserGroupSeparator
	resp.Attributes.SSOOnlyForRegisteredUsers = profile.SSOOnlyForRegisteredUsers
	resp.Attributes.ProfileID = profile.ProfileID
	resp.Attributes.SelectedProviderType = profile.SelectedProviderType

	urlFormat := "%sauth/%s/%s"
	callbackUrlFormat := "%sauth/%s/%s/callback"

	resp.Attributes.LoginURL = fmt.Sprintf(urlFormat, callbackBaseURL, profile.ProfileID, profile.SelectedProviderType)
	resp.Attributes.CallbackURL = fmt.Sprintf(callbackUrlFormat, callbackBaseURL, profile.ProfileID, profile.SelectedProviderType)
	resp.Attributes.FailureRedirectURL = failureRedirect
	resp.Attributes.UseInLoginPage = profile.UseInLoginPage

	return resp
}

func serializeProfileList(profile *models.Profile) ProfileListItem {
	userEmail := ""
	if profile.User.ID != 0 {
		userEmail = profile.User.Email
	}

	resp := ProfileListItem{
		Type: "sso-profiles",
		ID:   profile.Model.ID,
	}

	providerType := map[string]string{
		provOpenID: "Open ID Connect",
		provLDAP:   "LDAP",
		provSAML:   "SAML",
	}

	resp.Attributes.Name = profile.Name
	resp.Attributes.ProfileID = profile.ProfileID
	resp.Attributes.ProfileType = userProfile

	pt, ok := providerType[profile.SelectedProviderType]

	if ok {
		resp.Attributes.ProviderType = pt
	} else {
		resp.Attributes.ProviderType = "Social"
	}

	resp.Attributes.UpdatedBy = userEmail
	resp.Attributes.UpdatedAt = profile.UpdatedAt

	return resp
}

func serializeProfiles(profiles models.Profiles) []ProfileListItem {
	result := make([]ProfileListItem, len(profiles))
	for i, profile := range profiles {
		result[i] = serializeProfileList(&profile)
	}

	return result
}

// @Summary Create a new SSO profile
// @Description Create a new SSO profile with the provided information
// @Tags sso-profiles
// @Accept json
// @Produce json
// @Param profile body ProfileInput true "Profile information"
// @Success 201 {object} ProfileResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/sso-profiles [post]
// @Security BearerAuth
func (a *API) createProfile(c *gin.Context) {
	var input ProfileInput
	if err := c.ShouldBindJSON(&input); err != nil {
		helpers.SendErrorResponse(c, helpers.NewBadRequestError(err.Error()))
		return
	}

	// Get user from context
	userObj, exists := c.Get("user")
	if !exists {
		helpers.SendErrorResponse(c, helpers.NewUnauthorizedError("User not found in context"))
		return
	}

	currentUser, ok := userObj.(*models.User)
	if !ok {
		helpers.SendErrorResponse(c, helpers.NewUnauthorizedError("User not found in context"))
		return
	}

	uid := currentUser.ID
	profile := &models.Profile{
		Name:                      input.Data.Attributes.Name,
		OrgID:                     input.Data.Attributes.OrgID,
		ActionType:                input.Data.Attributes.ActionType,
		MatchedPolicyID:           input.Data.Attributes.MatchedPolicyID,
		Type:                      input.Data.Attributes.Type,
		ProviderName:              input.Data.Attributes.ProviderName,
		CustomEmailField:          input.Data.Attributes.CustomEmailField,
		CustomUserIDField:         input.Data.Attributes.CustomUserIDField,
		ProviderConfig:            input.Data.Attributes.ProviderConfig,
		IdentityHandlerConfig:     input.Data.Attributes.IdentityHandlerConfig,
		ProviderConstraintsDomain: input.Data.Attributes.ProviderConstraintsDomain,
		ProviderConstraintsGroup:  input.Data.Attributes.ProviderConstraintsGroup,
		ReturnURL:                 input.Data.Attributes.ReturnURL,
		DefaultUserGroupID:        input.Data.Attributes.DefaultUserGroupID,
		CustomUserGroupField:      input.Data.Attributes.CustomUserGroupField,
		UserGroupMapping:          input.Data.Attributes.UserGroupMapping,
		UserGroupSeparator:        input.Data.Attributes.UserGroupSeparator,
		SSOOnlyForRegisteredUsers: input.Data.Attributes.SSOOnlyForRegisteredUsers,
		ProfileID:                 input.Data.Attributes.ProfileID,
	}

	if err := a.service.CreateProfile(profile, uid); err != nil {
		helpers.SendErrorResponse(c, err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": serializeProfile(profile)})
}

// @Summary Get an SSO profile by ID
// @Description Get details of an SSO profile by its ID
// @Tags sso-profiles
// @Accept json
// @Produce json
// @Param id path string true "Profile ID"
// @Success 200 {object} ProfileResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /api/v1/sso-profiles/{id} [get]
// @Security BearerAuth
func (a *API) getProfile(c *gin.Context) {
	id := c.Param("profile_id")
	if id == "" {
		helpers.SendErrorResponse(c, helpers.NewBadRequestError("Invalid profile ID"))
		return
	}

	profile, err := a.service.GetProfileByID(id)
	if err != nil {
		helpers.SendErrorResponse(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": serializeProfile(profile)})
}

// @Summary Update an SSO profile
// @Description Update an existing SSO profile's information
// @Tags sso-profiles
// @Accept json
// @Produce json
// @Param id path string true "Profile ID"
// @Param profile body ProfileInput true "Updated profile information"
// @Success 200 {object} ProfileResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/sso-profiles/{id} [put]
// @Security BearerAuth
func (a *API) updateProfile(c *gin.Context) {
	id := c.Param("profile_id")
	if id == "" {
		helpers.SendErrorResponse(c, helpers.NewBadRequestError("Invalid profile ID"))
		return
	}

	var input ProfileInput
	if err := c.ShouldBindJSON(&input); err != nil {
		helpers.SendErrorResponse(c, helpers.NewBadRequestError(err.Error()))
		return
	}

	// Get user from context
	userObj, exists := c.Get("user")
	if !exists {
		helpers.SendErrorResponse(c, helpers.NewUnauthorizedError("User not found in context"))
		return
	}
	currentUser := userObj.(*models.User)
	uid := currentUser.ID

	updatedProfile := &models.Profile{
		Name:                      input.Data.Attributes.Name,
		OrgID:                     input.Data.Attributes.OrgID,
		ActionType:                input.Data.Attributes.ActionType,
		MatchedPolicyID:           input.Data.Attributes.MatchedPolicyID,
		Type:                      input.Data.Attributes.Type,
		ProviderName:              input.Data.Attributes.ProviderName,
		CustomEmailField:          input.Data.Attributes.CustomEmailField,
		CustomUserIDField:         input.Data.Attributes.CustomUserIDField,
		ProviderConfig:            input.Data.Attributes.ProviderConfig,
		IdentityHandlerConfig:     input.Data.Attributes.IdentityHandlerConfig,
		ProviderConstraintsDomain: input.Data.Attributes.ProviderConstraintsDomain,
		ProviderConstraintsGroup:  input.Data.Attributes.ProviderConstraintsGroup,
		ReturnURL:                 input.Data.Attributes.ReturnURL,
		DefaultUserGroupID:        input.Data.Attributes.DefaultUserGroupID,
		CustomUserGroupField:      input.Data.Attributes.CustomUserGroupField,
		UserGroupMapping:          input.Data.Attributes.UserGroupMapping,
		UserGroupSeparator:        input.Data.Attributes.UserGroupSeparator,
		SSOOnlyForRegisteredUsers: input.Data.Attributes.SSOOnlyForRegisteredUsers,
		ProfileID:                 input.Data.Attributes.ProfileID,
	}
	profile, err := a.service.UpdateProfile(id, updatedProfile, uid)
	if err != nil {
		helpers.SendErrorResponse(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": serializeProfile(profile)})
}

// @Summary Delete an SSO profile
// @Description Delete an SSO profile by its ID
// @Tags sso-profiles
// @Accept json
// @Produce json
// @Param id path string true "Profile ID"
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/sso-profiles/{id} [delete]
// @Security BearerAuth
func (a *API) deleteProfile(c *gin.Context) {
	id := c.Param("profile_id")
	if id == "" {
		helpers.SendErrorResponse(c, helpers.NewBadRequestError("Invalid profile ID"))
		return
	}

	if err := a.service.DeleteProfile(id); err != nil {
		helpers.SendErrorResponse(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

// @Summary List all SSO profiles
// @Description Get a list of all SSO profiles with pagination
// @Tags sso-profiles
// @Accept json
// @Produce json
// @Param page_size query int false "Page size"
// @Param page query int false "Page number"
// @Param all query bool false "Return all profiles"
// @Success 200 {object} ProfilesResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/sso-profiles [get]
// @Security BearerAuth
func (a *API) listProfiles(c *gin.Context) {
	pageSize, pageNumber, all := getPaginationParams(c)
	sort := c.Query("sort")

	profiles, totalCount, totalPages, err := a.service.ListProfiles(pageSize, pageNumber, all, sort)
	if err != nil {
		helpers.SendErrorResponse(c, err)
		return
	}

	c.Header("X-Total-Count", strconv.FormatInt(totalCount, 10))
	c.Header("X-Total-Pages", strconv.Itoa(totalPages))

	response := ProfilesResponse{
		Data: serializeProfiles(profiles),
		Meta: struct {
			TotalCount int64 `json:"total_count"`
			TotalPages int   `json:"total_pages"`
			PageSize   int   `json:"page_size"`
			PageNumber int   `json:"page_number"`
		}{
			TotalCount: totalCount,
			TotalPages: totalPages,
			PageSize:   pageSize,
			PageNumber: pageNumber,
		},
	}

	c.JSON(http.StatusOK, response)
}

// @Summary Set profile use in login page
// @Description Set a profile to be used in the login page
// @Tags sso-profiles
// @Accept json
// @Produce json
// @Param profile_id path string true "Profile ID"
// @Success 200 "OK"
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/sso-profiles/{profile_id}/use-in-login-page [post]
// @Security BearerAuth
func (a *API) setProfileUseInLoginPage(c *gin.Context) {
	id := c.Param("profile_id")
	if id == "" {
		helpers.SendErrorResponse(c, helpers.NewBadRequestError("Invalid profile ID"))
		return
	}

	if err := a.service.SetProfileUseInLoginPage(id); err != nil {
		helpers.SendErrorResponse(c, err)
		return
	}

	c.Status(http.StatusOK)
}

// @Summary Get the profile used in the login page
// @Description Get the profile that has UseInLoginPage set to true
// @Tags sso-profiles
// @Accept json
// @Produce json
// @Success 200 {object} ProfileResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/sso-profiles/login-page [get]
// @Security BearerAuth
func (a *API) getLoginPageProfile(c *gin.Context) {
	profile, err := a.service.GetLoginPageProfile()
	if err != nil {
		helpers.SendErrorResponse(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": serializeProfile(profile)})
}
