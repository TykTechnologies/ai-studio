//go:build enterprise
// +build enterprise

package api

import (
	"net/http"

	"github.com/TykTechnologies/midsommar/v2/helpers"
	"github.com/TykTechnologies/midsommar/v2/services/sso"
	tykerrors "github.com/TykTechnologies/tyk-identity-broker/error"

	"github.com/gin-gonic/gin"
)

// @Summary Handle TIB authentication
// @Description Handle authentication through a TIB profile
// @Tags auth
// @Accept json
// @Produce json
// @Param id path string true "Identity provider ID"
// @Param provider path string true "Provider name"
// @Success 302 {string} string "Redirect to the provider's authentication page"
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /auth/{id}/{provider} [get]
func (a *API) handleTIBAuth(c *gin.Context) {
	user := a.auth.GetAuthenticatedUser(c)
	if user != nil {
		c.Redirect(http.StatusFound, "/")
		return
	}

	id := c.Param("id")
	if id == "" {
		helpers.SendErrorResponse(c, helpers.NewBadRequestError("Identity provider ID is required"))
		return
	}

	provider := c.Param("provider")
	if provider == "" {
		helpers.SendErrorResponse(c, helpers.NewBadRequestError("Provider name is required"))
		return
	}

	identityProvider, profile, err := a.ssoService.GetTapProfile(id)
	if err != nil {
		helpers.SendErrorResponse(c, err)
		return
	}

	params := map[string]string{
		"id":       id,
		"provider": provider,
	}

	identityProvider.Handle(c.Writer, c.Request, params, *profile)
}

// @Summary Handle the callback from an identity provider
// @Description Handle the callback from an identity provider after authentication
// @Tags auth
// @Accept json
// @Produce json
// @Param id path string true "Identity provider ID"
// @Param provider path string true "Provider name"
// @Success 302 {string} string "Redirect to the dashboard or portal"
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /auth/{id}/{provider}/callback [get]
func (a *API) handleTIBAuthCallback(c *gin.Context) {
	user := a.auth.GetAuthenticatedUser(c)
	if user != nil {
		c.Redirect(http.StatusFound, "/")
		return
	}

	id := c.Param("id")
	if id == "" {
		helpers.SendErrorResponse(c, helpers.NewBadRequestError("Identity provider ID is required"))
		return
	}

	identityProvider, profile, err := a.ssoService.GetTapProfile(id)
	if err != nil {
		helpers.SendErrorResponse(c, err)
		return
	}

	identityProvider.HandleCallback(c.Writer, c.Request, tykerrors.HandleError, *profile)
}

// @Summary Handle the SAML metadata request for an identity provider
// @Description Handle the SAML metadata request for an identity provider
// @Tags auth
// @Accept json
// @Produce json
// @Param id path string true "Identity provider ID"
// @Success 200 {object} string "Success"
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /auth/{id}/saml/metadata [get]
func (a *API) handleSAMLMetadata(c *gin.Context) {
	user := a.auth.GetAuthenticatedUser(c)
	if user != nil {
		c.Redirect(http.StatusFound, "/")
		return
	}

	id := c.Param("id")
	if id == "" {
		helpers.SendErrorResponse(c, helpers.NewBadRequestError("Identity provider ID is required"))
		return
	}

	identityProvider, _, err := a.ssoService.GetTapProfile(id)
	if err != nil {
		helpers.SendErrorResponse(c, err)
		return
	}

	identityProvider.HandleMetadata(c.Writer, c.Request)
}

// @Summary Handle SSO authentication with a nonce token
// @Description Process SSO authentication using a nonce token, validate user, and establish a session
// @Tags auth
// @Accept json
// @Produce json
// @Param nonce query string true "Nonce token for authentication"
// @Success 302 {string} string "Redirect to the dashboard or portal"
// @Failure 400 {object} ErrorResponse "Bad request or invalid token"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /sso [get]
func (a *API) handleSSO(c *gin.Context) {
	nonceToken := c.Request.URL.Query().Get("nonce")

	tokenMetadata, err := a.ssoService.ResolveNonce(nonceToken, true)
	if err != nil || tokenMetadata == nil {
		helpers.SendErrorResponse(c, helpers.NewBadRequestError("Invalid or missing nonce token"))
		return
	}

	if err = a.ssoService.ValidateNonceRequest(tokenMetadata); err != nil {
		helpers.SendErrorResponse(c, err)
		return
	}

	user, err := a.ssoService.HandleSSO(
		tokenMetadata.EmailAddress,
		tokenMetadata.DisplayName,
		tokenMetadata.GroupID,
		tokenMetadata.GroupsIDs,
		tokenMetadata.SSOOnlyForRegisteredUsers,
	)

	if err != nil {
		helpers.SendErrorResponse(c, err)
		return
	}

	if err := a.auth.SetUserSession(c, user); err != nil {
		helpers.SendErrorResponse(c, helpers.NewInternalServerError("Failed to set user session"))
		return
	}

	c.Redirect(http.StatusFound, "/")
}

func (a *API) SSOAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authorizationHeader := c.GetHeader("Authorization")

		if authorizationHeader != a.config.TIBAPISecret {
			helpers.SendErrorResponse(c, helpers.NewUnauthorizedError("Not authorized"))
			c.Abort()
			return
		}

		c.Next()
	}
}

// @Summary Create a nonce token for SSO authentication
// @Description Create a nonce token that can be used for SSO authentication
// @Tags auth
// @Accept json
// @Produce json
// @Param request body sso.NonceTokenRequest true "Nonce token request"
// @Success 200 {object} sso.NonceTokenResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /api/sso [post]
func (a *API) handleNonceRequest(c *gin.Context) {
	var nonceRequest sso.NonceTokenRequest
	if err := c.ShouldBindJSON(&nonceRequest); err != nil {
		helpers.SendErrorResponse(c, helpers.NewBadRequestError("Malformed request body"))
		return
	}

	if err := a.ssoService.ValidateNonceRequest(&nonceRequest); err != nil {
		helpers.SendErrorResponse(c, err)
		return
	}

	nonceToken, err := a.ssoService.GenerateNonce(nonceRequest)
	if err != nil {
		helpers.SendErrorResponse(c, err)
		return
	}

	c.JSON(http.StatusOK, sso.NonceTokenResponse{
		Status:  "ok",
		Message: "Nonce token created",
		Meta:    nonceToken,
	})
}
