package api

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/TykTechnologies/midsommar/v2/config"
	"github.com/TykTechnologies/midsommar/v2/helpers"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// @Summary Get system feature set
// @Description Returns the current system feature set from licensing
// @Tags system
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /common/system [get]
func (a *API) handleFeatureSet(c *gin.Context) {
	featureSet := make(map[string]interface{})

	for k, v := range a.licenser.FeatureSet() {
		featureSet[k] = v
	}

	if cfg := config.Get(); cfg != nil {
		featureSet["docs_url"] = cfg.DocsURL
	}

	response := gin.H{
		"features": featureSet,
	}

	if license := a.licenser.License(); license != nil && !license.ExpiresAt.IsZero() {
		daysLeft := helpers.DaysLeft(license.ExpiresAt)
		if daysLeft > 0 && daysLeft < 30 {
			response["license_days_left"] = daysLeft
		}
	}

	c.JSON(http.StatusOK, response)
}

// @Summary Login user
// @Description Authenticate a user and return a session token
// @Tags auth
// @Accept json
// @Produce json
// @Param user body LoginInput true "Login credentials"
// @Success 200 {object} LoginResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Router /auth/login [post]
func (a *API) handleLogin(c *gin.Context) {
	var input LoginInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	err := a.auth.Login(c, input.Data.Attributes.Email, input.Data.Attributes.Password)
	if err != nil {
		return
	}

	u, err := a.service.GetUserByEmail(input.Data.Attributes.Email)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Login successful", "is_admin": u.IsAdmin})
}

// @Summary Register user
// @Description Register a new user
// @Tags auth
// @Accept json
// @Produce json
// @Param user body RegisterInput true "User registration details"
// @Success 201 {object} RegisterResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /auth/register [post]
func (a *API) handleRegister(c *gin.Context) {
	var input RegisterInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	err := a.auth.Register(input.Data.Attributes.Email, input.Data.Attributes.Name,
		input.Data.Attributes.Password, input.Data.Attributes.WithPortal, input.Data.Attributes.WithChat)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "User registered successfully"})
}

// @Summary Logout user
// @Description Log out the current user
// @Tags auth
// @Accept json
// @Produce json
// @Success 200 {object} LogoutResponse
// @Failure 500 {object} ErrorResponse
// @Router /auth/logout [post]
// @Security BearerAuth
func (a *API) handleLogout(c *gin.Context) {
	err := a.auth.Logout(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Logout successful"})
}

// @Summary Forgot password
// @Description Request a password reset
// @Tags auth
// @Accept json
// @Produce json
// @Param email body ForgotPasswordInput true "User's email"
// @Success 200 {object} ForgotPasswordResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /auth/forgot-password [post]
func (a *API) handleForgotPassword(c *gin.Context) {
	var input ForgotPasswordInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	err := a.auth.ResetPassword(input.Data.Attributes.Email)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Password reset email sent"})
}

// @Summary Validate reset token
// @Description Validate a password reset token without attempting to reset the password. Use this endpoint
// to check if a token is valid before showing the password reset form.
// @Tags auth
// @Accept json
// @Produce json
// @Param token query string true "Reset token"
// @Success 200 {object} TokenValidationResponse
// @Failure 400 {object} ErrorResponse
// @Router /auth/validate-reset-token [get]
func (a *API) handleValidateResetToken(c *gin.Context) {
	token := c.Query("token")
	if token == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Reset token is required"}},
		})
		return
	}

	_, err := a.auth.ValidateResetToken(token)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid or expired reset token"}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Token is valid"})
}

// @Summary Reset password
// @Description Reset user's password using a token
// @Tags auth
// @Accept json
// @Produce json
// @Param reset body ResetPasswordInput true "Password reset details"
// @Success 200 {object} ResetPasswordResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /auth/reset-password [post]
func (a *API) handleResetPassword(c *gin.Context) {
	var input ResetPasswordInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	user, err := a.auth.ValidateResetToken(input.Data.Attributes.Token)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid or expired reset token"}},
		})
		return
	}

	err = a.auth.UpdatePassword(user, "", input.Data.Attributes.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Password reset successful"})
}

// @Summary Verify email
// @Description Verify user's email using a token
// @Tags auth
// @Accept json
// @Produce json
// @Param token query string true "Verification token"
// @Success 200 {object} VerifyEmailResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /auth/verify-email [get]
func (a *API) handleVerifyEmail(c *gin.Context) {
	token := c.Query("token")
	if token == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Verification token is required"}},
		})
		return
	}

	err := a.auth.VerifyEmail(token)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	emailVerifiedHandler(c.Writer, c.Request)
	return
}

// @Summary Resend verification email
// @Description Resend the email verification link
// @Tags auth
// @Accept json
// @Produce json
// @Param email body ResendVerificationInput true "User's email"
// @Success 200 {object} ResendVerificationResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /auth/resend-verification [post]
func (a *API) handleResendVerification(c *gin.Context) {
	var input ResendVerificationInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	err := a.auth.ResendVerificationEmail(input.Data.Attributes.Email)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Verification email resent"})
}

// @Summary Get current user with entitlements
// @Description Get the details of the currently logged-in user including their entitlements
// @Tags auth
// @Accept json
// @Produce json
// @Success 200 {object} UserWithEntitlementsResponse
// @Failure 401 {object} ErrorResponse
// @Router /api/v1/me [get]
// @Security BearerAuth
func (a *API) handleMe(c *gin.Context) {
	user, exists := c.Get("user")
	if !exists {
		c.Status(http.StatusUnauthorized)
		return
	}

	u, ok := user.(*models.User)
	if !ok {
		c.Status(http.StatusUnauthorized)
		return
	}

	entitlements, err := a.service.GetUserEntitlements(u.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	// Convert service-level entitlements to API response
	response := UserWithEntitlementsResponse{
		Type: "user",
		ID:   strconv.Itoa(int(entitlements.User.ID)),
		Attributes: struct {
			Email     string `json:"email"`
			Name      string `json:"name"`
			IsAdmin   bool   `json:"is_admin"`
			UIOptions struct {
				ShowChat       bool `json:"show_chat"`
				ShowPortal     bool `json:"show_portal"`
				ShowSSOConfig  bool `json:"show_sso_config"`
				SkipQuickStart bool `json:"skip_quick_start"`
			} `json:"ui_options"`
			Entitlements struct {
				Catalogues     []CatalogueResponse     `json:"catalogues"`
				DataCatalogues []DataCatalogueResponse `json:"data_catalogues"`
				ToolCatalogues []ToolCatalogueResponse `json:"tool_catalogues"`
				Chats          []ChatResponse          `json:"chats"`
			} `json:"entitlements"`
		}{
			Email:   entitlements.User.Email,
			Name:    entitlements.User.Name,
			IsAdmin: entitlements.User.IsAdmin,
			UIOptions: struct {
				ShowChat       bool `json:"show_chat"`
				ShowPortal     bool `json:"show_portal"`
				ShowSSOConfig  bool `json:"show_sso_config"`
				SkipQuickStart bool `json:"skip_quick_start"`
			}{
				ShowChat:       entitlements.User.ShowChat,
				ShowPortal:     entitlements.User.ShowPortal,
				ShowSSOConfig:  entitlements.User.IsAdmin && entitlements.User.AccessToSSOConfig,
				SkipQuickStart: entitlements.User.SkipQuickStart,
			},
			Entitlements: struct {
				Catalogues     []CatalogueResponse     `json:"catalogues"`
				DataCatalogues []DataCatalogueResponse `json:"data_catalogues"`
				ToolCatalogues []ToolCatalogueResponse `json:"tool_catalogues"`
				Chats          []ChatResponse          `json:"chats"`
			}{
				Catalogues:     serializeCatalogues(entitlements.Catalogues),
				DataCatalogues: serializeDataCatalogues(entitlements.DataCatalogues),
				ToolCatalogues: serializeToolCatalogues(entitlements.ToolCatalogues, a.config.DB),
				Chats:          serializeChats(entitlements.Chats, a.config.DB),
			},
		},
	}

	c.JSON(http.StatusOK, response)
}

// Helper function to convert map to slice
func mapToSlice[T any](m map[uint]T) []T {
	slice := make([]T, 0, len(m))
	for _, v := range m {
		slice = append(slice, v)
	}
	return slice
}

func emailVerifiedHandler(w http.ResponseWriter, r *http.Request) {
	// HTML content with auto-redirect
	html := `
<!DOCTYPE html>
<html>
<head>
    <title>Email Verification</title>
    <meta http-equiv="refresh" content="3;url=/">
    <style>
        body {
            font-family: Arial, sans-serif;
            display: flex;
            justify-content: center;
            align-items: center;
            height: 100vh;
            margin: 0;
            background-color: #f0f0f0;
        }
        .message {
            text-align: center;
            padding: 20px;
            background-color: #ffffff;
            border-radius: 5px;
            box-shadow: 0 2px 5px rgba(0,0,0,0.1);
        }
    </style>
</head>
<body>
    <div class="message">
        <h1>Email Verified</h1>
        <p>Redirecting to homepage...</p>
    </div>
</body>
</html>`

	// Set content type to HTML
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	// Write the HTML content
	fmt.Fprint(w, html)
}

// OAuth Client Registration
type RegisterOAuthClientInput struct {
	ClientName              string   `json:"client_name" binding:"required"`
	RedirectURIs            []string `json:"redirect_uris" binding:"required,dive,url"`
	Scope                   string   `json:"scope"` // Optional
	GrantTypes              []string `json:"grant_types"`
	ResponseTypes           []string `json:"response_types"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
}

type RegisterOAuthClientOutput struct {
	ClientID                string   `json:"client_id"`
	ClientSecret            string   `json:"client_secret,omitempty"` // Only shown once
	ClientName              string   `json:"client_name"`
	RedirectURIs            []string `json:"redirect_uris"`
	Scope                   string   `json:"scope"`
	GrantTypes              []string `json:"grant_types"`
	ResponseTypes           []string `json:"response_types"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
	ClientSecretExpiresAt   int      `json:"client_secret_expires_at"` // RFC7591: 0 means never expires
}

// @Summary Register OAuth Client
// @Description Register a new OAuth client application. Public endpoint (auth will be added later).
// @Tags oauth
// @Accept json
// @Produce json
// @Param client_details body RegisterOAuthClientInput true "OAuth Client Registration Details"
// @Success 201 {object} RegisterOAuthClientOutput
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /oauth/register_client [post]
func (a *API) handleRegisterOAuthClient(c *gin.Context) {
	// Set explicit CORS headers to allow * origins
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Methods", "POST, OPTIONS")
	c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization")
	c.Header("Access-Control-Max-Age", "43200") // 12 hours

	// Handle preflight requests
	if c.Request.Method == "OPTIONS" {
		c.Status(http.StatusOK)
		return
	}
	var input RegisterOAuthClientInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	// For now, create clients without user association (userID = 0)
	// Auth will be added later as mentioned
	var userID uint = 0

	// Default token endpoint auth method if not provided
	tokenAuthMethod := input.TokenEndpointAuthMethod
	if tokenAuthMethod == "" {
		tokenAuthMethod = "client_secret_post" // Default as per RFC7591
	}
	// Support client_secret_post, client_secret_basic, and none (for public clients)
	if tokenAuthMethod != "client_secret_post" && tokenAuthMethod != "client_secret_basic" && tokenAuthMethod != "none" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Unsupported token_endpoint_auth_method. Supported methods: client_secret_post, client_secret_basic, none"}},
		})
		return
	}

	oauthClientService := services.NewOAuthClientService(a.config.DB)
	client, plainSecret, err := oauthClientService.CreateClientWithAuthMethod(input.ClientName, input.RedirectURIs, userID, input.Scope, tokenAuthMethod)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: "Could not create OAuth client: " + err.Error()}},
		})
		return
	}

	grantTypes := input.GrantTypes
	if len(grantTypes) == 0 {
		grantTypes = []string{"authorization_code"}
	}
	responseTypes := input.ResponseTypes
	if len(responseTypes) == 0 {
		responseTypes = []string{"code"}
	}

	resp := RegisterOAuthClientOutput{
		ClientID:                client.ClientID,
		ClientName:              client.ClientName,
		RedirectURIs:            input.RedirectURIs,
		Scope:                   client.Scope,
		GrantTypes:              grantTypes,
		ResponseTypes:           responseTypes,
		TokenEndpointAuthMethod: tokenAuthMethod,
		ClientSecretExpiresAt:   0,
	}

	// Only include client secret for confidential clients (not for "none" auth method)
	if tokenAuthMethod != "none" {
		resp.ClientSecret = plainSecret
	}
	c.JSON(http.StatusCreated, resp)
}

// @Summary OAuth Authorization Endpoint
// @Description Handles user authorization requests for OAuth clients.
// @Tags oauth
// @Param response_type query string true "Must be 'code'"
// @Param client_id query string true "Client ID"
// @Param redirect_uri query string true "Client Redirect URI"
// @Param scope query string false "Requested scopes (space-separated)"
// @Param state query string false "Opaque value to be returned to client"
// @Param code_challenge query string true "PKCE Code Challenge (S256)"
// @Param code_challenge_method query string true "PKCE Code Challenge Method (must be 'S256')"
// @Success 302 "Redirects to client's redirect_uri with code and state or to consent page"
// @Failure 400 {object} ErrorResponse "Invalid request parameters"
// @Failure 401 "User not authenticated (redirects to login)"
// @Failure 404 {object} ErrorResponse "Client not found"
// @Router /oauth/authorize [get]
func (a *API) handleOAuthAuthorize(c *gin.Context) {
	// Set explicit CORS headers to allow * origins
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Methods", "GET, OPTIONS")
	c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization")
	c.Header("Access-Control-Max-Age", "43200") // 12 hours

	// Handle preflight requests
	if c.Request.Method == "OPTIONS" {
		c.Status(http.StatusOK)
		return
	}
	responseType := c.Query("response_type")
	clientID := c.Query("client_id")
	redirectURI := c.Query("redirect_uri")
	scope := c.Query("scope")
	state := c.Query("state")
	codeChallenge := c.Query("code_challenge")
	codeChallengeMethod := c.Query("code_challenge_method")

	if responseType != "code" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "error_description": "response_type must be 'code'"})
		return
	}
	if clientID == "" || redirectURI == "" || codeChallenge == "" || codeChallengeMethod == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "error_description": "client_id, redirect_uri, code_challenge, and code_challenge_method are required"})
		return
	}
	if codeChallengeMethod != "S256" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "error_description": "code_challenge_method must be 'S256'"})
		return
	}

	oauthClientService := services.NewOAuthClientService(a.config.DB)
	client, err := oauthClientService.GetClient(clientID)
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			log.Printf("Error fetching client %s: %v", clientID, err)
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "error_description": "Invalid client_id or server error."})
		return
	}

	validRedirect, err := oauthClientService.ValidateRedirectURI(client, redirectURI)
	if err != nil {
		log.Printf("Error validating redirect URI for client %s: %v", clientID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error", "error_description": "Error validating redirect URI"})
		return
	}
	if !validRedirect {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "error_description": "redirect_uri is not valid for this client"})
		return
	}

	userCtx, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "access_denied", "error_description": "User not authenticated"})
		return
	}
	currentUser, ok := userCtx.(*models.User)
	if !ok || currentUser == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "server_error", "error_description": "Invalid user session type"})
		return
	}

	pendingAuthService := services.NewPendingAuthRequestService(a.config.DB)
	pendingArgs := services.StorePendingAuthRequestArgs{
		ClientID:            client.ClientID,
		UserID:              currentUser.ID,
		RedirectURI:         redirectURI,
		Scope:               scope,
		State:               state,
		CodeChallenge:       codeChallenge,
		CodeChallengeMethod: codeChallengeMethod,
		ExpiresIn:           15 * time.Minute,
	}

	pendingRequest, err_store := pendingAuthService.StorePendingAuthRequest(pendingArgs)
	if err_store != nil {
		log.Printf("Error storing pending auth request for client %s: %v", clientID, err_store)
		errorRedirectURL, _ := url.Parse(redirectURI)
		qParams := errorRedirectURL.Query()
		qParams.Set("error", "server_error")
		qParams.Set("error_description", "Could not process authorization request.")
		if state != "" {
			qParams.Set("state", state)
		}
		errorRedirectURL.RawQuery = qParams.Encode()
		c.Redirect(http.StatusFound, errorRedirectURL.String())
		return
	}

	appConf := config.Get()
	consentPageBaseURL, err_parse_site_url := url.Parse(appConf.SiteURL)
	if err_parse_site_url != nil {
		log.Printf("Error parsing SiteURL '%s' for consent redirect: %v", appConf.SiteURL, err_parse_site_url)
		errorRedirectURL, _ := url.Parse(redirectURI)
		qParams := errorRedirectURL.Query()
		qParams.Set("error", "server_error")
		qParams.Set("error_description", "Server configuration error for consent redirection.")
		if state != "" {
			qParams.Set("state", state)
		}
		errorRedirectURL.RawQuery = qParams.Encode()
		c.Redirect(http.StatusFound, errorRedirectURL.String())
		return
	}

	consentPath, _ := url.Parse("/oauth/consent")
	consentPageQuery := consentPath.Query()
	consentPageQuery.Set("auth_req_id", pendingRequest.ID)
	consentPath.RawQuery = consentPageQuery.Encode()

	finalConsentURL := consentPageBaseURL.ResolveReference(consentPath)
	c.Redirect(http.StatusFound, finalConsentURL.String())
}

// handleGetConsentDetails provides details needed for the consent screen.
// @Summary Get OAuth Consent Details
// @Description Retrieves details for an OAuth consent request. Requires user authentication.
// @Tags oauth
// @Param auth_req_id query string true "Authorization Request ID"
// @Success 200 {object} ConsentDetailsResponse
// @Failure 400 {object} ErrorResponse "Invalid or missing auth_req_id"
// @Failure 401 {object} ErrorResponse "User not authenticated or mismatch"
// @Failure 404 {object} ErrorResponse "Request not found or expired"
// @Failure 500 {object} ErrorResponse
// @Router /oauth/consent_details [get]
// @Security BearerAuth
func (a *API) handleGetConsentDetails(c *gin.Context) {
	// Set explicit CORS headers to allow * origins
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Methods", "GET, OPTIONS")
	c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization")
	c.Header("Access-Control-Max-Age", "43200") // 12 hours

	// Handle preflight requests
	if c.Request.Method == "OPTIONS" {
		c.Status(http.StatusOK)
		return
	}
	authRequestID := c.Query("auth_req_id")
	if authRequestID == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Bad Request", Detail: "auth_req_id is required"}}})
		return
	}

	userCtx, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Unauthorized", Detail: "User not authenticated"}}})
		return
	}
	currentUser, ok := userCtx.(*models.User)
	if !ok || currentUser == nil {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Unauthorized", Detail: "Invalid user session"}}})
		return
	}

	pendingAuthService := services.NewPendingAuthRequestService(a.config.DB)
	pendingRequest, err := pendingAuthService.GetPendingAuthRequest(authRequestID, currentUser.ID)
	if err != nil {
		statusCode := http.StatusInternalServerError
		if err.Error() == "pending authorization request not found" || err.Error() == "pending authorization request has expired" {
			statusCode = http.StatusNotFound
		} else if err.Error() == "user mismatch for pending authorization request" {
			statusCode = http.StatusUnauthorized
		}
		c.JSON(statusCode, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Error", Detail: err.Error()}}})
		return
	}

	oauthClientService := services.NewOAuthClientService(a.config.DB)
	clientDetails, err_client := oauthClientService.GetClient(pendingRequest.ClientID)
	if err_client != nil {
		log.Printf("Error fetching client %s for consent: %v", pendingRequest.ClientID, err_client)
		c.JSON(http.StatusInternalServerError, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Server Error", Detail: "Could not retrieve client details."}}})
		return
	}

	scopesList := strings.Split(pendingRequest.Scope, " ")
	if pendingRequest.Scope == "" {
		scopesList = []string{}
	}

	// Get user's approved apps with tools
	var availableApps []AppWithTools
	var noAppsMessage string

	// Always require app selection for OAuth flows
	appModel := &models.App{}
	apps, err := appModel.GetByUserID(a.config.DB, currentUser.ID)
	if err != nil {
		log.Printf("Error fetching user apps for consent: %v", err)
	} else {
		// Filter apps that have active credentials AND at least one tool
		for _, app := range apps {
			if app.Credential.Active && len(app.Tools) > 0 {
				toolNames := make([]string, len(app.Tools))
				for i, tool := range app.Tools {
					toolNames[i] = tool.Name
				}
				availableApps = append(availableApps, AppWithTools{
					ID:          app.ID,
					Name:        app.Name,
					Description: app.Description,
					Tools:       toolNames,
				})
			}
		}
	}

	if len(availableApps) == 0 {
		noAppsMessage = "No approved apps with tools found. Please create an app in the developer portal and add tools to it before using OAuth access."
	}

	resp := ConsentDetailsResponse{
		AuthRequestID: authRequestID,
		ClientName:    clientDetails.ClientName,
		Scopes:        scopesList,
		AvailableApps: availableApps,
		NoAppsMessage: noAppsMessage,
	}
	c.JSON(http.StatusOK, resp)
}

type ConsentDetailsResponse struct {
	AuthRequestID string         `json:"auth_req_id"`
	ClientName    string         `json:"client_name"`
	Scopes        []string       `json:"scopes"`
	AvailableApps []AppWithTools `json:"available_apps"`
	NoAppsMessage string         `json:"no_apps_message,omitempty"`
}

type AppWithTools struct {
	ID          uint     `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Tools       []string `json:"tools"`
}

type SubmitConsentInput struct {
	AuthRequestID string `json:"auth_req_id" form:"auth_req_id" binding:"required"`
	Decision      string `json:"decision" form:"decision" binding:"required"`
	SelectedAppID uint   `json:"selected_app_id" form:"selected_app_id"`
}

// handleSubmitConsent handles the user's consent decision.
// @Summary Submit OAuth Consent
// @Description Submits user's consent decision (approve/deny) for an OAuth request.
// @Tags oauth
// @Accept json
// @Produce json
// @Param consent_submission body SubmitConsentInput true "Consent Submission Details"
// @Success 302 "Redirects to client's redirect_uri with code/state or error/state"
// @Failure 400 {object} ErrorResponse "Invalid input"
// @Failure 401 {object} ErrorResponse "User not authenticated or mismatch"
// @Failure 404 {object} ErrorResponse "Request not found or expired"
// @Failure 500 {object} ErrorResponse
// @Router /oauth/submit_consent [post]
// @Security BearerAuth
func (a *API) handleSubmitConsent(c *gin.Context) {
	// Set explicit CORS headers to allow * origins
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Methods", "POST, OPTIONS")
	c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization")
	c.Header("Access-Control-Max-Age", "43200") // 12 hours

	// Handle preflight requests
	if c.Request.Method == "OPTIONS" {
		c.Status(http.StatusOK)
		return
	}
	var input SubmitConsentInput

	// Try to bind as JSON first, then as form data
	contentType := c.GetHeader("Content-Type")
	var err_bind error

	if strings.Contains(contentType, "application/json") {
		err_bind = c.ShouldBindJSON(&input)
	} else {
		// Try form binding for regular form submissions
		err_bind = c.ShouldBind(&input)
	}

	if err_bind != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Bad Request", Detail: err_bind.Error()}}})
		return
	}

	userCtx, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Unauthorized", Detail: "User not authenticated"}}})
		return
	}
	currentUser, ok := userCtx.(*models.User)
	if !ok || currentUser == nil {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Unauthorized", Detail: "Invalid user session"}}})
		return
	}

	pendingAuthService := services.NewPendingAuthRequestService(a.config.DB)
	pendingRequest, err_get_pending := pendingAuthService.GetPendingAuthRequest(input.AuthRequestID, currentUser.ID)
	if err_get_pending != nil {
		statusCode := http.StatusInternalServerError
		if err_get_pending.Error() == "pending authorization request not found" || err_get_pending.Error() == "pending authorization request has expired" {
			statusCode = http.StatusNotFound
		} else if err_get_pending.Error() == "user mismatch for pending authorization request" {
			statusCode = http.StatusUnauthorized
		}
		c.JSON(statusCode, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Error", Detail: err_get_pending.Error()}}})
		return
	}

	defer pendingAuthService.DeletePendingAuthRequest(pendingRequest.ID)

	finalRedirectURL, _ := url.Parse(pendingRequest.RedirectURI)
	qParams := finalRedirectURL.Query()

	if input.Decision == "approved" {
		// Always validate selected app for OAuth flows
		if input.SelectedAppID == 0 {
			qParams.Set("error", "invalid_request")
			qParams.Set("error_description", "App selection is required for OAuth access.")
			finalRedirectURL.RawQuery = qParams.Encode()
			c.Redirect(http.StatusFound, finalRedirectURL.String())
			return
		}

		// Verify user owns the selected app
		appModel := &models.App{}
		if err := appModel.Get(a.config.DB, input.SelectedAppID); err != nil {
			log.Printf("Error fetching selected app %d for consent: %v", input.SelectedAppID, err)
			qParams.Set("error", "invalid_request")
			qParams.Set("error_description", "Selected app not found.")
			finalRedirectURL.RawQuery = qParams.Encode()
			c.Redirect(http.StatusFound, finalRedirectURL.String())
			return
		}

		if appModel.UserID != currentUser.ID {
			log.Printf("User %d trying to use app %d owned by user %d", currentUser.ID, input.SelectedAppID, appModel.UserID)
			qParams.Set("error", "access_denied")
			qParams.Set("error_description", "You don't have permission to use this app.")
			finalRedirectURL.RawQuery = qParams.Encode()
			c.Redirect(http.StatusFound, finalRedirectURL.String())
			return
		}

		if !appModel.Credential.Active {
			qParams.Set("error", "invalid_request")
			qParams.Set("error_description", "Selected app has inactive credentials.")
			finalRedirectURL.RawQuery = qParams.Encode()
			c.Redirect(http.StatusFound, finalRedirectURL.String())
			return
		}

		if len(appModel.Tools) == 0 {
			qParams.Set("error", "invalid_request")
			qParams.Set("error_description", "Selected app has no tools.")
			finalRedirectURL.RawQuery = qParams.Encode()
			c.Redirect(http.StatusFound, finalRedirectURL.String())
			return
		}

		appID := &input.SelectedAppID

		authCodeService := services.NewAuthCodeService(a.config.DB)
		createArgs := services.CreateAuthCodeArgs{
			ClientID:            pendingRequest.ClientID,
			UserID:              pendingRequest.UserID,
			RedirectURI:         pendingRequest.RedirectURI,
			Scope:               pendingRequest.Scope,
			ExpiresIn:           10 * time.Minute,
			CodeChallenge:       pendingRequest.CodeChallenge,
			CodeChallengeMethod: pendingRequest.CodeChallengeMethod,
			AppID:               appID,
		}
		_, codeValue, codeErr := authCodeService.CreateAuthCode(createArgs)
		if codeErr != nil {
			log.Printf("Error creating auth code after consent for req %s: %v", input.AuthRequestID, codeErr)
			qParams.Set("error", "server_error")
			qParams.Set("error_description", "Could not generate authorization code after consent.")
		} else {
			qParams.Set("code", codeValue)
		}
	} else {
		qParams.Set("error", "access_denied")
		qParams.Set("error_description", "The resource owner or authorization server denied the request.")
	}

	if pendingRequest.State != "" {
		qParams.Set("state", pendingRequest.State)
	}
	finalRedirectURL.RawQuery = qParams.Encode()
	c.Redirect(http.StatusFound, finalRedirectURL.String())
}

// @Summary OAuth Token Endpoint
// @Description Exchanges an authorization code for an access token.
// @Tags oauth
// @Accept x-www-form-urlencoded
// @Produce json
// @Param grant_type formData string true "Must be 'authorization_code'"
// @Param code formData string true "Authorization code"
// @Param redirect_uri formData string true "Redirect URI used in authorization request"
// @Param client_id formData string true "Client ID"
// @Param client_secret formData string false "Client Secret (for confidential clients using client_secret_post)"
// @Param code_verifier formData string true "PKCE Code Verifier"
// @Success 200 {object} AccessTokenResponse
// @Failure 400 {object} OAuthErrorResponse "e.g., invalid_request, invalid_grant"
// @Failure 401 {object} OAuthErrorResponse "e.g., invalid_client"
// @Router /oauth/token [post]
func (a *API) handleOAuthToken(c *gin.Context) {
	// Set explicit CORS headers to allow * origins
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Methods", "POST, OPTIONS")
	c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization")
	c.Header("Access-Control-Max-Age", "43200") // 12 hours

	// Handle preflight requests
	if c.Request.Method == "OPTIONS" {
		c.Status(http.StatusOK)
		return
	}
	grantType := c.PostForm("grant_type")
	code := c.PostForm("code")
	redirectURI := c.PostForm("redirect_uri")
	clientID := c.PostForm("client_id")
	clientSecret := c.PostForm("client_secret")
	codeVerifier := c.PostForm("code_verifier")

	if grantType != "authorization_code" {
		c.JSON(http.StatusBadRequest, OAuthErrorResponse{Error: "unsupported_grant_type", ErrorDescription: "grant_type must be 'authorization_code'"})
		return
	}
	if code == "" || redirectURI == "" || clientID == "" || codeVerifier == "" {
		c.JSON(http.StatusBadRequest, OAuthErrorResponse{Error: "invalid_request", ErrorDescription: "Missing required parameters: code, redirect_uri, client_id, code_verifier"})
		return
	}

	oauthClientService := services.NewOAuthClientService(a.config.DB)
	client, err := oauthClientService.GetClient(clientID)
	if err != nil {
		log.Printf("Token endpoint: Client not found %s: %v", clientID, err)
		c.JSON(http.StatusUnauthorized, OAuthErrorResponse{Error: "invalid_client", ErrorDescription: "Client authentication failed."})
		return
	}

	// Check if this is a public client (no client secret required)
	if oauthClientService.IsPublicClient(client) {
		// Public client - no client secret required, but we still rely on PKCE
		if clientSecret != "" {
			log.Printf("Token endpoint: Public client %s provided unexpected client secret", clientID)
			c.JSON(http.StatusBadRequest, OAuthErrorResponse{Error: "invalid_request", ErrorDescription: "Public clients should not provide client_secret."})
			return
		}
	} else {
		// Confidential client - client secret is required
		if clientSecret == "" {
			log.Printf("Token endpoint: Client secret not provided by client %s", clientID)
			c.JSON(http.StatusUnauthorized, OAuthErrorResponse{Error: "invalid_client", ErrorDescription: "Client authentication failed (missing secret)."})
			return
		}
		validSecret, err_validate := oauthClientService.ValidateClientSecret(client, clientSecret)
		if err_validate != nil {
			log.Printf("Token endpoint: Error validating client secret for %s: %v", clientID, err_validate)
			c.JSON(http.StatusUnauthorized, OAuthErrorResponse{Error: "invalid_client", ErrorDescription: "Client authentication failed."})
			return
		}
		if !validSecret {
			log.Printf("Token endpoint: Invalid client secret for %s", clientID)
			c.JSON(http.StatusUnauthorized, OAuthErrorResponse{Error: "invalid_client", ErrorDescription: "Client authentication failed (invalid secret)."})
			return
		}
	}

	authCodeService := services.NewAuthCodeService(a.config.DB)
	storedAuthCode, err_auth_code := authCodeService.GetValidAuthCodeByCode(code)
	if err_auth_code != nil {
		log.Printf("Token endpoint: GetValidAuthCodeByCode for code %s failed: %v", code, err_auth_code)
		c.JSON(http.StatusBadRequest, OAuthErrorResponse{Error: "invalid_grant", ErrorDescription: "Invalid or expired authorization code."})
		return
	}

	if storedAuthCode.ClientID != clientID {
		log.Printf("Token endpoint: Auth code clientID mismatch. Expected %s, got %s", storedAuthCode.ClientID, clientID)
		c.JSON(http.StatusBadRequest, OAuthErrorResponse{Error: "invalid_grant", ErrorDescription: "Authorization code client_id mismatch."})
		return
	}
	if storedAuthCode.RedirectURI != redirectURI {
		log.Printf("Token endpoint: Auth code redirect_uri mismatch. Expected %s, got %s", storedAuthCode.RedirectURI, redirectURI)
		c.JSON(http.StatusBadRequest, OAuthErrorResponse{Error: "invalid_grant", ErrorDescription: "Authorization code redirect_uri mismatch."})
		return
	}

	if storedAuthCode.CodeChallengeMethod == "S256" {
		calculatedChallenge := helpers.CalculatePKCEChallengeS256(codeVerifier)
		if calculatedChallenge != storedAuthCode.CodeChallenge {
			log.Printf("Token endpoint: PKCE challenge failed for client %s. Expected %s, got %s (from verifier %s)", clientID, storedAuthCode.CodeChallenge, calculatedChallenge, codeVerifier)
			c.JSON(http.StatusBadRequest, OAuthErrorResponse{Error: "invalid_grant", ErrorDescription: "PKCE code_verifier challenge failed."})
			return
		}
	} else {
		log.Printf("Token endpoint: Unsupported code_challenge_method %s for client %s", storedAuthCode.CodeChallengeMethod, clientID)
		c.JSON(http.StatusBadRequest, OAuthErrorResponse{Error: "invalid_grant", ErrorDescription: "Unsupported code_challenge_method."})
		return
	}

	err_mark_used := authCodeService.MarkAuthCodeAsUsed(code)
	if err_mark_used != nil {
		log.Printf("Token endpoint: Failed to mark auth code %s as used: %v", code, err_mark_used)
		c.JSON(http.StatusInternalServerError, OAuthErrorResponse{Error: "server_error", ErrorDescription: "Failed to process authorization code."})
		return
	}

	// Always return app secret instead of generating temporary tokens
	if storedAuthCode.AppID == nil {
		log.Printf("Token endpoint: No app selected for client %s", clientID)
		c.JSON(http.StatusBadRequest, OAuthErrorResponse{Error: "invalid_grant", ErrorDescription: "No app selected for OAuth access."})
		return
	}

	// Get the selected app and return its secret
	appModel := &models.App{}
	if err := appModel.Get(a.config.DB, *storedAuthCode.AppID); err != nil {
		log.Printf("Token endpoint: Failed to fetch app %d: %v", *storedAuthCode.AppID, err)
		c.JSON(http.StatusInternalServerError, OAuthErrorResponse{Error: "server_error", ErrorDescription: "Failed to retrieve app credentials."})
		return
	}

	if appModel.UserID != storedAuthCode.UserID {
		log.Printf("Token endpoint: App %d ownership mismatch for user %d", *storedAuthCode.AppID, storedAuthCode.UserID)
		c.JSON(http.StatusBadRequest, OAuthErrorResponse{Error: "invalid_grant", ErrorDescription: "App ownership mismatch."})
		return
	}

	if !appModel.Credential.Active {
		log.Printf("Token endpoint: App %d has inactive credentials", *storedAuthCode.AppID)
		c.JSON(http.StatusBadRequest, OAuthErrorResponse{Error: "invalid_grant", ErrorDescription: "App credentials are inactive."})
		return
	}

	c.JSON(http.StatusOK, AccessTokenResponse{
		AccessToken: appModel.Credential.Secret,
		TokenType:   "Bearer",
		ExpiresIn:   0, // App secrets don't expire
		Scope:       storedAuthCode.Scope,
	})
}

// OAuthErrorResponse defines the structure for OAuth 2.0 error responses.
type OAuthErrorResponse struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description,omitempty"`
	ErrorURI         string `json:"error_uri,omitempty"`
}

// AccessTokenResponse defines the structure for successful token responses.
type AccessTokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"` // In seconds
	RefreshToken string `json:"refresh_token,omitempty"`
	Scope        string `json:"scope,omitempty"`
}

// OAuthServerMetadata defines the structure for the AS metadata response.
type OAuthServerMetadata struct {
	Issuer                                string   `json:"issuer"`
	AuthorizationEndpoint                 string   `json:"authorization_endpoint"`
	TokenEndpoint                         string   `json:"token_endpoint"`
	RegistrationEndpoint                  string   `json:"registration_endpoint,omitempty"`
	ScopesSupported                       []string `json:"scopes_supported,omitempty"`
	ResponseTypesSupported                []string `json:"response_types_supported"`
	GrantTypesSupported                   []string `json:"grant_types_supported,omitempty"`
	TokenEndpointAuthMethodsSupported     []string `json:"token_endpoint_auth_methods_supported,omitempty"`
	CodeChallengeMethodsSupported         []string `json:"code_challenge_methods_supported,omitempty"`
	IntrospectionEndpoint                 string   `json:"introspection_endpoint,omitempty"`
	RevocationEndpoint                    string   `json:"revocation_endpoint,omitempty"`
	DeviceAuthorizationEndpoint           string   `json:"device_authorization_endpoint,omitempty"`
	PushedAuthorizationRequestEndpoint    string   `json:"pushed_authorization_request_endpoint,omitempty"`
	RequirePushedAuthorizationRequests    bool     `json:"require_pushed_authorization_requests,omitempty"`
	TlsClientCertificateBoundAccessTokens bool     `json:"tls_client_certificate_bound_access_tokens,omitempty"`
	RequestURIParameterSupported          bool     `json:"request_uri_parameter_supported,omitempty"`
	RequestParameterSupported             bool     `json:"request_parameter_supported,omitempty"`
	ServiceDocumentation                  string   `json:"service_documentation,omitempty"`
	UILocalesSupported                    []string `json:"ui_locales_supported,omitempty"`
	OpPolicyURI                           string   `json:"op_policy_uri,omitempty"`
	OpTosURI                              string   `json:"op_tos_uri,omitempty"`
	JwksURI                               string   `json:"jwks_uri,omitempty"`
}

// @Summary OAuth Authorization Server Metadata
// @Description Provides metadata about the OAuth authorization server.
// @Tags oauth
// @Produce json
// @Success 200 {object} OAuthServerMetadata
// @Router /.well-known/oauth-authorization-server [get]
func (a *API) handleOAuthMetadata(c *gin.Context) {
	// Set explicit CORS headers to allow * origins
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Methods", "GET, OPTIONS")
	c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization")
	c.Header("Access-Control-Max-Age", "43200") // 12 hours

	// Handle preflight requests
	if c.Request.Method == "OPTIONS" {
		c.Status(http.StatusOK)
		return
	}

	appConf := config.Get()
	baseURL, err := url.Parse(appConf.AuthServerURL)
	if err != nil {
		log.Printf("Error parsing AuthServerURL '%s': %v", appConf.AuthServerURL, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server_configuration_error", "error_description": "Invalid authorization server URL configured."})
		return
	}

	resolve := func(p string) string {
		rel, _ := url.Parse(p)
		return baseURL.ResolveReference(rel).String()
	}

	metadata := OAuthServerMetadata{
		Issuer:                            appConf.AuthServerURL,
		AuthorizationEndpoint:             resolve("/oauth/authorize"),
		TokenEndpoint:                     resolve("/oauth/token"),
		RegistrationEndpoint:              resolve("/oauth/register_client"),
		ScopesSupported:                   []string{"openid", "profile", "email", "mcp"},
		ResponseTypesSupported:            []string{"code"},
		GrantTypesSupported:               []string{"authorization_code"},
		TokenEndpointAuthMethodsSupported: []string{"client_secret_post", "client_secret_basic", "none"},
		CodeChallengeMethodsSupported:     []string{"S256"},
	}
	c.JSON(http.StatusOK, metadata)
}

// Need to import "errors", "log", "net/url", "time", "gorm.io/gorm"
// and "strings"
// and "github.com/TykTechnologies/midsommar/v2/services"
