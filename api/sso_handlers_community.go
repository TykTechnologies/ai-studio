//go:build !enterprise
// +build !enterprise

package api

import (
	"net/http"

	"github.com/TykTechnologies/midsommar/v2/helpers"
	"github.com/gin-gonic/gin"
)

// handleTIBAuth - CE: Returns 402 Payment Required
func (a *API) handleTIBAuth(c *gin.Context) {
	helpers.SendErrorResponse(c, helpers.NewPaymentRequiredError("SSO is an Enterprise feature"))
}

// handleTIBAuthCallback - CE: Returns 402 Payment Required
func (a *API) handleTIBAuthCallback(c *gin.Context) {
	helpers.SendErrorResponse(c, helpers.NewPaymentRequiredError("SSO is an Enterprise feature"))
}

// handleSAMLMetadata - CE: Returns 402 Payment Required
func (a *API) handleSAMLMetadata(c *gin.Context) {
	helpers.SendErrorResponse(c, helpers.NewPaymentRequiredError("SSO is an Enterprise feature"))
}

// handleSSO - CE: Returns 402 Payment Required
func (a *API) handleSSO(c *gin.Context) {
	helpers.SendErrorResponse(c, helpers.NewPaymentRequiredError("SSO is an Enterprise feature"))
}

// SSOAuthMiddleware - CE: Always returns unauthorized
func (a *API) SSOAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		helpers.SendErrorResponse(c, helpers.NewUnauthorizedError("SSO is an Enterprise feature"))
		c.Abort()
	}
}

// handleNonceRequest - CE: Returns 402 Payment Required
func (a *API) handleNonceRequest(c *gin.Context) {
	c.JSON(http.StatusPaymentRequired, gin.H{
		"Status":  "error",
		"Message": "SSO is an Enterprise feature",
	})
}
