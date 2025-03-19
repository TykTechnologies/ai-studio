package api

import (
	"encoding/json"
	"net/http"

	"github.com/TykTechnologies/midsommar/v2/helpers"
	"github.com/TykTechnologies/midsommar/v2/services"
	tykerrors "github.com/TykTechnologies/tyk-identity-broker/error"

	"github.com/gin-gonic/gin"
)

// @Summary Handle an auth request to any of the registered profiles
// @Description Handle SSO authentication with the specified provider
// @Tags auth
// @Accept json
// @Produce json
// @Param id path string true "Identity provider ID"
// @Param provider path string true "Provider name"
// @Success 200 {object} string "Success"
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /auth/{id}/{provider} [get, post]
func (a *API) handleTIBAuth(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		sendErrorResponse(c, helpers.NewBadRequestError("Identity provider ID is required"))
		return
	}

	provider := c.Param("provider")
	if provider == "" {
		sendErrorResponse(c, helpers.NewBadRequestError("Provider name is required"))
		return
	}

	identityProvider, profile, err := a.ssoService.GetTapProfile(id)
	if err != nil {
		sendErrorResponse(c, err)
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
// @Success 200 {object} string "Success"
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /auth/{id}/{provider}/callback [get, post]
func (a *API) handleTIBAuthCallback(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		sendErrorResponse(c, helpers.NewBadRequestError("Identity provider ID is required"))
		return
	}

	identityProvider, profile, err := a.ssoService.GetTapProfile(id)
	if err != nil {
		sendErrorResponse(c, err)
		return
	}

	identityProvider.HandleCallback(c.Writer, c.Request, tykerrors.HandleError, *profile)
}

func sendErrorResponse(c *gin.Context, err error) {
	switch e := err.(type) {
	case helpers.ErrorResponse:
		c.JSON(e.StatusCode, gin.H{
			"error":   e.Title,
			"message": e.Message,
		})
	default:
		// Unexpected error type
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Internal Server Error",
			"message": err.Error(),
		})
	}
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
		sendErrorResponse(c, helpers.NewBadRequestError("Invalid or missing nonce token"))
		return
	}

	if err = a.ssoService.ValidateNonceRequest(tokenMetadata); err != nil {
		sendErrorResponse(c, err)
		return
	}

	user, err := a.ssoService.HandleSSO(
		tokenMetadata.EmailAddress,
		tokenMetadata.DisplayName,
		tokenMetadata.GroupID,
		tokenMetadata.SSOOnlyForRegisteredUsers,
		tokenMetadata.ForSection,
	)

	if err != nil {
		sendErrorResponse(c, err)
		return
	}

	if err := a.auth.SetUserSession(c, user); err != nil {
		sendErrorResponse(c, helpers.NewInternalServerError("Failed to set user session"))
		return
	}

	c.Redirect(http.StatusFound, "/")
}

func (a *API) SSOAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authorizationHeader := c.GetHeader("Authorization")

		if authorizationHeader != a.config.TIBAPISecret {
			sendErrorResponse(c, helpers.NewUnauthorizedError("Not authorized"))
			c.Abort()
			return
		}

		c.Next()
	}
}

// @Summary Generate a nonce token for SSO authentication
// @Description Generates a nonce token with 60 sec TTL for SSO authentication
// @Tags auth
// @Accept json
// @Produce json
// @Param request body services.NonceTokenRequest true "Nonce token request"
// @Success 200 {object} services.NonceTokenResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Router /api/sso [post]
func (a *API) handleNonceRequest(c *gin.Context) {
	var nonceRequest services.NonceTokenRequest
	if err := c.ShouldBindJSON(&nonceRequest); err != nil {
		sendErrorResponse(c, helpers.NewBadRequestError("Malformed request body"))
		return
	}

	if err := a.ssoService.ValidateNonceRequest(&nonceRequest); err != nil {
		sendErrorResponse(c, err)
		return
	}

	nonceToken, err := a.ssoService.GenerateNonce(nonceRequest)
	if err != nil {
		sendErrorResponse(c, err)
		return
	}

	c.JSON(http.StatusOK, services.NonceTokenResponse{
		Status:  "ok",
		Message: "Nonce token created",
		Meta:    nonceToken,
	})
}

// @Summary Create a new developer via SSO
// @Description Creates a new developer with the provided SSO key and adds them to the specified group
// @Tags auth
// @Accept json
// @Produce json
// @Param body body services.PortalDeveloper true "Developer information"
// @Success 201 "User created successfully"
// @Success 200 "User already exists"
// @Failure 400 {object} ErrorResponse "Bad request"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /portal/developers [post]
func (a *API) createSSOUser(c *gin.Context) {
	var developerRequest services.PortalDeveloper

	err := json.NewDecoder(c.Request.Body).Decode(&developerRequest)
	if err != nil {
		sendErrorResponse(c, helpers.NewBadRequestError("Malformed request body"))
		return
	}

	// Get nonce token metadata
	tokenMetadata, err := a.ssoService.ResolveNonce(developerRequest.Nonce, false)
	if err != nil || tokenMetadata == nil {
		sendErrorResponse(c, helpers.NewBadRequestError("Invalid or missing nonce token"))
		return
	}

	// Create user with transaction
	_, err = a.ssoService.CreateSSOUser(
		developerRequest.Email,
		tokenMetadata.DisplayName,
		developerRequest.Password,
		developerRequest.SSOKey,
		tokenMetadata.GroupID,
	)

	if err != nil {
		sendErrorResponse(c, err)
		return
	}

	c.Status(http.StatusCreated)
}

// @Summary Get a user by SSO key
// @Description Retrieves a user with the specified SSO key
// @Tags auth
// @Accept json
// @Produce json
// @Param id path string true "SSO key"
// @Success 200 {object} services.PortalDeveloper "User found"
// @Failure 404 "User not found"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /portal/developers/ssokey/{id} [get]
func (a *API) getSSOUserBySSOKey(c *gin.Context) {
	ssoKey := c.Param("id")

	developer, err := a.ssoService.GetUserBySSOKey(ssoKey)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}

	c.Header("Content-Type", "application/json")
	c.JSON(http.StatusOK, developer)
}

// @Summary Update an existing user
// @Description Updates an existing user with the provided information
// @Tags auth
// @Accept json
// @Produce json
// @Param id path string true "User ID"
// @Param body body services.PortalDeveloper true "Developer information"
// @Success 200 "User updated successfully"
// @Failure 400 {object} ErrorResponse "Bad request"
// @Failure 404 "User not found"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /portal/developers/{id} [put]
func (a *API) updateSSOUser(c *gin.Context) {
	var developerRequest services.PortalDeveloper

	err := json.NewDecoder(c.Request.Body).Decode(&developerRequest)
	if err != nil {
		sendErrorResponse(c, helpers.NewBadRequestError("Malformed request body"))
		return
	}

	// Get nonce token metadata
	tokenMetadata, err := a.ssoService.ResolveNonce(developerRequest.Nonce, false)
	if err != nil || tokenMetadata == nil {
		sendErrorResponse(c, helpers.NewBadRequestError("Invalid or missing nonce token"))
		return
	}

	// Update user with transaction
	_, err = a.ssoService.UpdateSSOUser(
		developerRequest.SSOKey,
		developerRequest.Email,
		developerRequest.Password,
		tokenMetadata.GroupID,
	)

	if err != nil {
		sendErrorResponse(c, err)
		return
	}

	c.Status(http.StatusOK)
}
