package api

import (
	"errors"
	"net/http"

	"github.com/TykTechnologies/midsommar/v2/helpers"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services/entitlements"
	"github.com/gin-gonic/gin"
)

// NoticesResponse lists the near-limit notices for the signed-in user.
type NoticesResponse struct {
	Data []entitlements.Notice `json:"data"`
}

// @Summary My near-limit notices
// @Description Licence limits and App quota near their maximum that the signed-in user has not dismissed; always empty in Community Edition
// @Tags common
// @Produce json
// @Success 200 {object} NoticesResponse
// @Router /common/notices [get]
// @Security BearerAuth
func (a *API) getMyNotices(c *gin.Context) {
	user, ok := c.Get("user")
	if !ok {
		helpers.SendErrorResponse(c, helpers.NewUnauthorizedError("user not found in context"))
		return
	}
	notices, err := a.service.EntitlementsService().Notices(a.service.DB, user.(*models.User))
	switch {
	case errors.Is(err, entitlements.ErrEnterpriseFeature):
		notices = nil
	case err != nil:
		helpers.SendErrorResponse(c, helpers.NewInternalServerError(err.Error()))
		return
	}
	if notices == nil {
		notices = []entitlements.Notice{}
	}
	c.JSON(http.StatusOK, NoticesResponse{Data: notices})
}

// @Summary Dismiss a notice
// @Description Hides the notice for the signed-in user for good; a licence with a different maximum shows it again (Enterprise)
// @Tags common
// @Param key path string true "Notice key"
// @Success 204
// @Failure 400 {object} ErrorResponse
// @Failure 402 {object} ErrorResponse
// @Router /common/notices/{key}/dismiss [post]
// @Security BearerAuth
func (a *API) dismissMyNotice(c *gin.Context) {
	user, ok := c.Get("user")
	if !ok {
		helpers.SendErrorResponse(c, helpers.NewUnauthorizedError("user not found in context"))
		return
	}
	err := a.service.EntitlementsService().DismissNotice(a.service.DB, user.(*models.User), c.Param("key"))
	switch {
	case errors.Is(err, entitlements.ErrEnterpriseFeature):
		helpers.SendErrorResponse(c, helpers.NewPaymentRequiredError(err.Error()))
	case errors.Is(err, entitlements.ErrUnknownNotice):
		helpers.SendErrorResponse(c, helpers.NewBadRequestError(err.Error()))
	case err != nil:
		helpers.SendErrorResponse(c, helpers.NewInternalServerError(err.Error()))
	default:
		c.Status(http.StatusNoContent)
	}
}
