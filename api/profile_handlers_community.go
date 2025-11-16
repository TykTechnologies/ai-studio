//go:build !enterprise
// +build !enterprise

package api

import (
	"github.com/TykTechnologies/midsommar/v2/helpers"
	"github.com/gin-gonic/gin"
)

// createProfile - CE: Returns 402 Payment Required
func (a *API) createProfile(c *gin.Context) {
	helpers.SendErrorResponse(c, helpers.NewPaymentRequiredError("SSO profile management is an Enterprise feature"))
}

// getProfile - CE: Returns 402 Payment Required
func (a *API) getProfile(c *gin.Context) {
	helpers.SendErrorResponse(c, helpers.NewPaymentRequiredError("SSO profile management is an Enterprise feature"))
}

// updateProfile - CE: Returns 402 Payment Required
func (a *API) updateProfile(c *gin.Context) {
	helpers.SendErrorResponse(c, helpers.NewPaymentRequiredError("SSO profile management is an Enterprise feature"))
}

// deleteProfile - CE: Returns 402 Payment Required
func (a *API) deleteProfile(c *gin.Context) {
	helpers.SendErrorResponse(c, helpers.NewPaymentRequiredError("SSO profile management is an Enterprise feature"))
}

// listProfiles - CE: Returns 402 Payment Required
func (a *API) listProfiles(c *gin.Context) {
	helpers.SendErrorResponse(c, helpers.NewPaymentRequiredError("SSO profile management is an Enterprise feature"))
}

// setProfileUseInLoginPage - CE: Returns 402 Payment Required
func (a *API) setProfileUseInLoginPage(c *gin.Context) {
	helpers.SendErrorResponse(c, helpers.NewPaymentRequiredError("SSO profile management is an Enterprise feature"))
}

// getLoginPageProfile - CE: Returns nil (no profile available)
func (a *API) getLoginPageProfile(c *gin.Context) {
	// Return empty response - SSO not available in CE
	c.JSON(200, gin.H{
		"data": nil,
	})
}
