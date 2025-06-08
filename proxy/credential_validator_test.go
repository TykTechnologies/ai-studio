package proxy

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	// "strings" // REMOVED as it seems unused in the final version of the test logic
	"testing"
	"time"

	apitesting "github.com/TykTechnologies/midsommar/v2/api/testing"
	"github.com/TykTechnologies/midsommar/v2/config"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/gosimple/slug"
	"github.com/stretchr/testify/require"
)

// Helper to create a new request and recorder for middleware tests
func newMiddlewareTest(method, target string, authHeaderValue string, apiKeyHeaderName string, apiKeyValue string) (*httptest.ResponseRecorder, *http.Request) {
	req := httptest.NewRequest(method, target, nil)
	if authHeaderValue != "" {
		req.Header.Set("Authorization", authHeaderValue)
	}
	if apiKeyHeaderName != "" && apiKeyValue != "" {
		req.Header.Set(apiKeyHeaderName, apiKeyValue)
	}
	rr := httptest.NewRecorder()
	return rr, req
}

// Dummy next handler for middleware tests
var nextHandler http.HandlerFunc = func(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	userCtx := r.Context().Value("user")
	appCtx := r.Context().Value("app")
	oauthClientCtx := r.Context().Value("oauthClient")
	toolCtx := r.Context().Value("tool")

	if userCtx != nil {
		user, _ := userCtx.(*models.User)
		fmt.Fprintf(w, "User:%d;", user.ID)
	}
	if appCtx != nil {
		app, _ := appCtx.(*models.App)
		fmt.Fprintf(w, "App:%d;", app.ID)
	}
	if oauthClientCtx != nil {
		client, _ := oauthClientCtx.(*models.OAuthClient)
		fmt.Fprintf(w, "OAuthClient:%s;", client.ClientID)
	}
	if toolCtx != nil {
		tool, _ := toolCtx.(*models.Tool)
		fmt.Fprintf(w, "Tool:%d;", tool.ID)
	}
}

func TestCredentialValidatorMiddleware(t *testing.T) {
	_ = context.Background() // Explicitly "use" context

	db := apitesting.SetupTestDB(t)
	serviceInstance := apitesting.SetupTestService(db)

	proxyInstance := NewProxy(serviceInstance, &Config{Port: 9090}, serviceInstance.Budget)
	err := proxyInstance.loadResources()
	require.NoError(t, err)

	validator := NewCredentialValidator(serviceInstance, proxyInstance)

	testUser := models.User{Email: "beareruser@example.com", Name: "Bearer User"}
	require.NoError(t, db.Create(&testUser).Error)

	oauthClientSvc := services.NewOAuthClientService(db)
	oauthClient, _, err := oauthClientSvc.CreateClient("BearerTestClient", []string{"http://dummy/cb"}, testUser.ID, "mcp")
	require.NoError(t, err)

	apiKeyUser := models.User{Email: "apikeyuser@example.com", Name: "APIKey User"}
	require.NoError(t, db.Create(&apiKeyUser).Error)

	var toolIDs []uint
	appInstance, err := serviceInstance.CreateApp("TestAppForApiKey", "Test App Description", apiKeyUser.ID, []uint{}, []uint{}, toolIDs, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, appInstance.CredentialID)

	appInstance, err = serviceInstance.GetAppByID(appInstance.ID)
	require.NoError(t, err)
	require.NotNil(t, appInstance.Credential)
	require.NotEmpty(t, appInstance.Credential.Secret)

	require.NoError(t, serviceInstance.ActivateCredential(appInstance.CredentialID))
	apiCred := appInstance.Credential

	authTestToolName := "AuthTestTool"
	authTestToolSlug := slug.Make(authTestToolName)
	authTestTool, err := serviceInstance.CreateTool(authTestToolName, "Tool for testing auth paths", models.ToolTypeREST, "", 0, "", "")
	require.NoError(t, err)
	_, err = serviceInstance.AddToolToApp(appInstance.ID, authTestTool.ID)
	require.NoError(t, err)
	err = proxyInstance.loadResources() // Reload resources in proxy
	require.NoError(t, err)


	originalProxyOAuthMetaURL := config.Get().ProxyOAuthMetadataURL
	config.Get().ProxyOAuthMetadataURL = "http://proxy.test/.well-known/oauth-protected-resource"
	defer func() { config.Get().ProxyOAuthMetadataURL = originalProxyOAuthMetaURL }()

	t.Run("ValidBearerToken", func(t *testing.T) {
		accessTokenSvc := services.NewAccessTokenService(db)
		tokenArgs := services.CreateAccessTokenArgs{
			ClientID: oauthClient.ClientID, UserID: testUser.ID, Scope: "mcp", ExpiresIn: 1 * time.Hour,
		}
		_, tokenValue, err := accessTokenSvc.CreateAccessToken(tokenArgs)
		require.NoError(t, err)

		rr, req := newMiddlewareTest("GET", "/tools/"+authTestToolSlug, "Bearer "+tokenValue, "", "")
		validator.Middleware(nextHandler).ServeHTTP(rr, req)

		require.Equal(t, http.StatusOK, rr.Code, "Body: %s", rr.Body.String())
		responseBody := rr.Body.String()
		require.Contains(t, responseBody, fmt.Sprintf("User:%d;", testUser.ID))
		require.Contains(t, responseBody, fmt.Sprintf("OAuthClient:%s;", oauthClient.ClientID))
	})

	t.Run("InvalidBearerToken_NotFound", func(t *testing.T) {
		rr, req := newMiddlewareTest("GET", "/tools/"+authTestToolSlug, "Bearer invalid-token-value", "", "")
		validator.Middleware(nextHandler).ServeHTTP(rr, req)

		require.Equal(t, http.StatusUnauthorized, rr.Code)
		expectedHeader := `Bearer realm="MCPResources", resource_metadata_uri="http://proxy.test/.well-known/oauth-protected-resource"`
		require.Equal(t, expectedHeader, rr.Header().Get("WWW-Authenticate"))
	})

	t.Run("InvalidBearerToken_Expired", func(t *testing.T) {
		accessTokenSvc := services.NewAccessTokenService(db)
		tokenArgs := services.CreateAccessTokenArgs{
			ClientID: oauthClient.ClientID, UserID: testUser.ID, Scope: "mcp", ExpiresIn: -1 * time.Hour,
		}
		_, tokenValue, err := accessTokenSvc.CreateAccessToken(tokenArgs)
		require.NoError(t, err)

		rr, req := newMiddlewareTest("GET", "/tools/"+authTestToolSlug, "Bearer "+tokenValue, "", "")
		validator.Middleware(nextHandler).ServeHTTP(rr, req)

		require.Equal(t, http.StatusUnauthorized, rr.Code)
		expectedHeader := `Bearer realm="MCPResources", resource_metadata_uri="http://proxy.test/.well-known/oauth-protected-resource"`
		require.Equal(t, expectedHeader, rr.Header().Get("WWW-Authenticate"))
	})

	t.Run("MalformedBearerToken_NoSpace", func(t *testing.T) {
		rr, req := newMiddlewareTest("GET", "/tools/"+authTestToolSlug, "Bearertoken", "", "")
		validator.Middleware(nextHandler).ServeHTTP(rr, req)
		require.Equal(t, http.StatusUnauthorized, rr.Code)
		expectedHeader := `Bearer realm="MCPResources", resource_metadata_uri="http://proxy.test/.well-known/oauth-protected-resource"`
		require.Equal(t, expectedHeader, rr.Header().Get("WWW-Authenticate"))
	})

	t.Run("MalformedBearerToken_NoToken", func(t *testing.T) {
		rr, req := newMiddlewareTest("GET", "/tools/"+authTestToolSlug, "Bearer ", "", "")
		validator.Middleware(nextHandler).ServeHTTP(rr, req)
		require.Equal(t, http.StatusUnauthorized, rr.Code)
		expectedHeader := `Bearer realm="MCPResources", resource_metadata_uri="http://proxy.test/.well-known/oauth-protected-resource"`
		require.Equal(t, expectedHeader, rr.Header().Get("WWW-Authenticate"))
	})

	t.Run("ValidAPIKey_NoBearerToken", func(t *testing.T) {
		rr, req := newMiddlewareTest("GET", "/tools/"+authTestToolSlug, apiCred.Secret, "", "")
		validator.Middleware(nextHandler).ServeHTTP(rr, req)

		require.Equal(t, http.StatusOK, rr.Code, "Body: %s", rr.Body.String())
		responseBody := rr.Body.String()
		require.Contains(t, responseBody, fmt.Sprintf("App:%d;", appInstance.ID))
		require.Contains(t, responseBody, fmt.Sprintf("Tool:%d;", authTestTool.ID))
	})

	t.Run("InvalidAPIKey_NoBearerToken", func(t *testing.T) {
		rr, req := newMiddlewareTest("GET", "/tools/"+authTestToolSlug, "invalid-api-key-value", "", "")
		validator.Middleware(nextHandler).ServeHTTP(rr, req)

		require.Equal(t, http.StatusUnauthorized, rr.Code)
		expectedHeader := `Bearer realm="MCPResources", resource_metadata_uri="http://proxy.test/.well-known/oauth-protected-resource"`
		require.Equal(t, expectedHeader, rr.Header().Get("WWW-Authenticate"))
	})

	t.Run("NoAuthenticationProvided", func(t *testing.T) {
		rr, req := newMiddlewareTest("GET", "/tools/"+authTestToolSlug, "", "", "")
		validator.Middleware(nextHandler).ServeHTTP(rr, req)

		require.Equal(t, http.StatusUnauthorized, rr.Code)
		expectedHeader := `Bearer realm="MCPResources", resource_metadata_uri="http://proxy.test/.well-known/oauth-protected-resource"`
		require.Equal(t, expectedHeader, rr.Header().Get("WWW-Authenticate"))
	})

	t.Run("WellKnownPath_NoAuthNeeded", func(t *testing.T) {
		rr, req := newMiddlewareTest("GET", "/.well-known/oauth-protected-resource", "", "", "")
		validator.Middleware(nextHandler).ServeHTTP(rr, req)
		require.Equal(t, http.StatusOK, rr.Code)
		require.Empty(t, rr.Header().Get("WWW-Authenticate"))
	})
}
