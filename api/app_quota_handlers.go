package api

import (
	"errors"
	"net/http"

	"github.com/TykTechnologies/midsommar/v2/helpers"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services/entitlements"
	"github.com/gin-gonic/gin"
)

// MyAppQuota is the signed-in user's self-service App quota.
type MyAppQuota struct {
	Used      int64 `json:"used"`
	Max       int64 `json:"max"`
	Unlimited bool  `json:"unlimited"`
}

// appQuotaErrorResponse maps entitlements errors onto HTTP statuses.
func appQuotaErrorResponse(c *gin.Context, err error) {
	switch {
	case errors.Is(err, entitlements.ErrEnterpriseFeature):
		helpers.SendErrorResponse(c, helpers.NewPaymentRequiredError(err.Error()))
	case errors.Is(err, entitlements.ErrTeamNotFound):
		helpers.SendErrorResponse(c, helpers.NewNotFoundError(err.Error()))
	case errors.Is(err, entitlements.ErrInvalidQuota):
		helpers.SendErrorResponse(c, helpers.NewBadRequestError(err.Error()))
	default:
		helpers.SendErrorResponse(c, helpers.NewInternalServerError(err.Error()))
	}
}

// @Summary Get a team's App quota
// @Description The team's own per-member quota of self-service Apps, and the global default its members get without one (Enterprise)
// @Tags groups
// @Produce json
// @Param id path int true "Team ID"
// @Success 200 {object} entitlements.TeamAppQuota
// @Failure 402 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /groups/{id}/app-quota [get]
// @Security BearerAuth
func (a *API) getTeamAppQuota(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	q, err := a.service.EntitlementsService().GetTeamAppQuota(a.service.DB, id)
	if err != nil {
		appQuotaErrorResponse(c, err)
		return
	}
	c.JSON(http.StatusOK, q)
}

// @Summary Set a team's App quota
// @Description Members may create this many Apps each through the portal; the most generous of a user's teams applies (Enterprise)
// @Tags groups
// @Accept json
// @Produce json
// @Param id path int true "Team ID"
// @Param body body entitlements.TeamAppQuotaInput true "Quota"
// @Success 200 {object} entitlements.TeamAppQuota
// @Failure 400 {object} ErrorResponse
// @Failure 402 {object} ErrorResponse
// @Router /groups/{id}/app-quota [put]
// @Security BearerAuth
func (a *API) setTeamAppQuota(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	var in entitlements.TeamAppQuotaInput
	if err := c.ShouldBindJSON(&in); err != nil {
		helpers.SendErrorResponse(c, helpers.NewBadRequestError("malformed request body: "+err.Error()))
		return
	}
	if err := a.service.EntitlementsService().SetTeamAppQuota(a.service.DB, id, in); err != nil {
		appQuotaErrorResponse(c, err)
		return
	}
	a.getTeamAppQuota(c)
}

// @Summary Remove a team's App quota
// @Description Members get the global default again (Enterprise)
// @Tags groups
// @Param id path int true "Team ID"
// @Success 204
// @Failure 402 {object} ErrorResponse
// @Router /groups/{id}/app-quota [delete]
// @Security BearerAuth
func (a *API) deleteTeamAppQuota(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	if err := a.service.EntitlementsService().DeleteTeamAppQuota(a.service.DB, id); err != nil {
		appQuotaErrorResponse(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// @Summary My App quota
// @Description How many Apps the signed-in user owns and how many they may create through the portal (Enterprise)
// @Tags portal
// @Produce json
// @Success 200 {object} MyAppQuota
// @Failure 402 {object} ErrorResponse
// @Router /common/me/app-quota [get]
// @Security BearerAuth
func (a *API) getMyAppQuota(c *gin.Context) {
	user, ok := c.Get("user")
	if !ok {
		helpers.SendErrorResponse(c, helpers.NewUnauthorizedError("user not found in context"))
		return
	}
	used, max, unlimited, err := a.service.EntitlementsService().UserAppQuota(a.service.DB, user.(*models.User).ID)
	if err != nil {
		appQuotaErrorResponse(c, err)
		return
	}
	c.JSON(http.StatusOK, MyAppQuota{Used: used, Max: max, Unlimited: unlimited})
}
