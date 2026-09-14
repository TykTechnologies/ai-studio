package api

import (
	"net/http"

	"github.com/TykTechnologies/midsommar/v2/helpers"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/gin-gonic/gin"
)

// Self-service profile routes under /common/me. They act on the signed-in
// user only and reuse the same service functions and policy as the admin
// user routes, so an administrator and a user rolling the same key get the
// same result and the same refusals.

// PreferencesResponse is the body of GET/PATCH /common/me/preferences.
// @Description Notification preferences of the signed-in user
type PreferencesResponse struct {
	// NotificationsEnabled is the in-app admin fan-out consent (new users,
	// app requests). It can only be on for administrators.
	NotificationsEnabled bool `json:"notifications_enabled"`
	// EmailNotificationsEnabled controls whether notifications are also
	// emailed; the bell receives them either way.
	EmailNotificationsEnabled bool `json:"email_notifications_enabled"`
}

// PreferencesUpdateRequest is the body of PATCH /common/me/preferences;
// omitted fields are left unchanged.
// @Description Notification preferences to change
type PreferencesUpdateRequest struct {
	NotificationsEnabled      *bool `json:"notifications_enabled"`
	EmailNotificationsEnabled *bool `json:"email_notifications_enabled"`
}

func currentUser(c *gin.Context) (*models.User, bool) {
	user, exists := c.Get("user")
	if !exists {
		return nil, false
	}
	u, ok := user.(*models.User)
	return u, ok
}

func preferencesOf(u *models.User) gin.H {
	return gin.H{"data": PreferencesResponse{
		NotificationsEnabled:      u.NotificationsEnabled,
		EmailNotificationsEnabled: u.EmailNotificationsEnabled,
	}}
}

// @Summary Get my notification preferences
// @Description Notification preferences of the signed-in user.
// @Tags me
// @Produce json
// @Success 200 {object} map[string]PreferencesResponse
// @Failure 401 {object} ErrorResponse
// @Router /common/me/preferences [get]
// @Security BearerAuth
func (a *API) getMyPreferences(c *gin.Context) {
	u, ok := currentUser(c)
	if !ok {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	c.JSON(http.StatusOK, preferencesOf(u))
}

// @Summary Update my notification preferences
// @Description Change one or both notification preferences of the signed-in user. notifications_enabled can only be switched on by administrators (the same rule as the admin user form).
// @Tags me
// @Accept json
// @Produce json
// @Param body body PreferencesUpdateRequest true "Preferences to change"
// @Success 200 {object} map[string]PreferencesResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Router /common/me/preferences [patch]
// @Security BearerAuth
func (a *API) updateMyPreferences(c *gin.Context) {
	u, ok := currentUser(c)
	if !ok {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var req PreferencesUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		helpers.SendErrorResponse(c, helpers.NewBadRequestError("Invalid request body: "+err.Error()))
		return
	}
	if req.NotificationsEnabled == nil && req.EmailNotificationsEnabled == nil {
		helpers.SendErrorResponse(c, helpers.NewBadRequestError("nothing to update: pass notifications_enabled and/or email_notifications_enabled"))
		return
	}

	// Column updates: a whole-struct Save would race the login and key-use
	// stamps, and an insert-style write would trip the default:true column.
	if req.NotificationsEnabled != nil {
		if *req.NotificationsEnabled && !u.IsAdmin {
			helpers.SendErrorResponse(c, helpers.NewBadRequestError("notifications can only be enabled for admin users"))
			return
		}
		if err := models.SetNotificationsEnabled(a.config.DB, u.ID, *req.NotificationsEnabled); err != nil {
			helpers.SendErrorResponse(c, helpers.NewInternalServerError(err.Error()))
			return
		}
		u.NotificationsEnabled = *req.NotificationsEnabled
	}
	if req.EmailNotificationsEnabled != nil {
		if err := models.SetEmailNotificationsEnabled(a.config.DB, u.ID, *req.EmailNotificationsEnabled); err != nil {
			helpers.SendErrorResponse(c, helpers.NewInternalServerError(err.Error()))
			return
		}
		u.EmailNotificationsEnabled = *req.EmailNotificationsEnabled
	}

	c.JSON(http.StatusOK, preferencesOf(u))
}

// @Summary Roll my API key
// @Description Issue a new API key for the signed-in user, replacing any existing one. The key is returned once and never shown again. Refused for SSO-provisioned users unless ALLOW_SSO_USER_API_KEYS is set. The browser session is unaffected.
// @Tags me
// @Produce json
// @Success 200 {object} map[string]map[string]string
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /common/me/api-key/roll [post]
// @Security BearerAuth
func (a *API) rollMyAPIKey(c *gin.Context) {
	u, ok := currentUser(c)
	if !ok {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	if !a.userMayHoldAPIKey(u) {
		helpers.SendErrorResponse(c, errSSOUserAPIKeysForbidden())
		return
	}

	// GenerateAPIKeyForUser writes the api_key column only, so the session
	// token behind the caller's cookie is untouched and they stay signed in.
	if err := a.service.GenerateAPIKeyForUser(u.ID); err != nil {
		helpers.SendErrorResponse(c, err)
		return
	}

	fresh, err := a.service.GetUserByID(u.ID)
	if err != nil {
		helpers.SendErrorResponse(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": gin.H{"api_key": fresh.APIKey}})
}

// @Summary Revoke my API key
// @Description Clear the signed-in user's API key. The key stops working immediately; the browser session is unaffected.
// @Tags me
// @Success 204
// @Failure 401 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /common/me/api-key [delete]
// @Security BearerAuth
func (a *API) revokeMyAPIKey(c *gin.Context) {
	u, ok := currentUser(c)
	if !ok {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	if err := a.service.RevokeAPIKeyForUser(u.ID); err != nil {
		helpers.SendErrorResponse(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}
