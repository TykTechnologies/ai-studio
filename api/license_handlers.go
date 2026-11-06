package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/TykTechnologies/midsommar/v2/helpers"
	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/services/entitlements"
)

// sendInternalError answers 500 with a generic message and logs the cause,
// so database or internal detail never reaches the client.
func sendInternalError(c *gin.Context, what string, err error) {
	logger.Errorf("%s %s: %s: %v", c.Request.Method, c.Request.URL.Path, what, err)
	helpers.SendErrorResponse(c, helpers.NewInternalServerError(what))
}

// requireOwner answers 403 unless the signed-in user is an Owner (the Super
// Admin without roles). The License page is theirs alone.
func (a *API) requireOwner(c *gin.Context) bool {
	u, ok := currentUser(c)
	if !ok {
		helpers.SendErrorResponse(c, helpers.NewUnauthorizedError("not signed in"))
		return false
	}
	owner, err := a.service.Authz().IsOwner(c.Request.Context(), u.ID)
	if err != nil {
		sendInternalError(c, "failed to check the Owner role", err)
		return false
	}
	if !owner {
		helpers.SendErrorResponse(c, helpers.NewForbiddenError("only an Owner may view or manage the licence"))
		return false
	}
	return true
}

// licenseErrorResponse maps entitlements errors onto HTTP statuses.
func licenseErrorResponse(c *gin.Context, err error) {
	if errors.Is(err, entitlements.ErrEnterpriseFeature) {
		helpers.SendErrorResponse(c, helpers.NewPaymentRequiredError(err.Error()))
		return
	}
	if he, ok := asHelperError(err); ok {
		helpers.SendErrorResponse(c, he)
		return
	}
	sendInternalError(c, "failed to read the licence", err)
}

// getLicense godoc
// @Summary Licence report
// @Description The licence's SKUs, expiry, features, and each limit with its usage, plus the namespaces holding a licensed slot. Owner only; Community Edition answers 402.
// @Tags license
// @Produce json
// @Success 200 {object} entitlements.Report
// @Failure 402 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Router /api/v1/license [get]
// @Security BearerAuth
func (a *API) getLicense(c *gin.Context) {
	if !a.requireOwner(c) {
		return
	}
	report, err := a.service.EntitlementsService().Report(a.service.DB)
	if err != nil {
		licenseErrorResponse(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": report})
}

// releaseLicensedNamespace godoc
// @Summary Release a namespace's licensed slot
// @Description Frees the namespace's licensed slot so another namespace can claim it. Refused (409) while edges in the namespace are connected. Owner only; Community Edition answers 402.
// @Tags license
// @Produce json
// @Param namespace path string true "Namespace"
// @Success 204
// @Failure 402 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Router /api/v1/license/namespaces/{namespace} [delete]
// @Security BearerAuth
func (a *API) releaseLicensedNamespace(c *gin.Context) {
	if !a.requireOwner(c) {
		return
	}
	ns := c.Param("namespace")
	if err := validateNamespace(ns); err != nil {
		helpers.SendErrorResponse(c, helpers.NewBadRequestError(err.Error()))
		return
	}
	if err := a.service.EntitlementsService().ReleaseNamespace(a.service.DB, ns); err != nil {
		licenseErrorResponse(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
