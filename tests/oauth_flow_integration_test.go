package tests

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/api"
	apitesting "github.com/TykTechnologies/midsommar/v2/api/testing"
	"github.com/TykTechnologies/midsommar/v2/auth"
	"github.com/TykTechnologies/midsommar/v2/config"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/proxy"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

var testDB *gorm.DB
var dashboardServer *httptest.Server
var proxyServer *httptest.Server
var testUser models.User
var testAPI *api.API // To access services if needed for setup not covered by API calls

// generatePKCEChallenge creates a code verifier and challenge for PKCE.
func generatePKCEChallenge() (verifier string, challenge string, err error) {
	randomBytes := make([]byte, 32)
	_, err = rand.Read(randomBytes)
	if err != nil {
		return "", "", err
	}
	verifier = base64.RawURLEncoding.EncodeToString(randomBytes)
	hash := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(hash[:])
	return verifier, challenge, nil
}

// setupIntegrationTestEnv initializes the test DB, services, and starts test servers.
func setupIntegrationTestEnv(t *testing.T) (*http.Client, string, string) {
	gin.SetMode(gin.TestMode)
	testDB = apitesting.SetupTestDB(t)

	// Create main service bundle
	serviceInstance := apitesting.SetupTestService(testDB)
	authConfig := apitesting.SetupTestAuthConfig(testDB, serviceInstance)
	authService := apitesting.SetupTestAuthService(testDB, serviceInstance) // Uses real services
	licenser := apitesting.SetupTestLicenser()

	// Setup Dashboard API server
	testAPI = api.NewAPI(serviceInstance, true, authService, authConfig, nil,apitesting.GetEmptyFSForTest(), licenser)
	dashboardServer = httptest.NewServer(testAPI.Router())

	// Setup Proxy server
	// The proxy needs access to the same service instance for token validation etc.
	proxyConfig := &proxy.Config{Port: 0} // Port 0 for httptest
	proxyInstance := proxy.NewProxy(serviceInstance, proxyConfig, serviceInstance.Budget)
	proxyServer = httptest.NewServer(proxyInstance.CreateHandler()) // Assuming CreateHandler returns http.Handler

	// Create a test user
	testUser = models.User{
		Email:         "testuser@example.com",
		Name:          "Test User OAuth",
		Password:      "password123",
		EmailVerified: true,
		IsAdmin:       false,
	}
	err := testUser.CreateWithPassword(testDB)
	require.NoError(t, err)

	// HTTP client with cookie jar to simulate browser sessions
	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	httpClient := &http.Client{
		Jar: jar,
		// Prevent auto-redirects to inspect 302 responses
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	// Override config URLs to point to test servers
	config.Get().SiteURL = dashboardServer.URL          // For consent redirects
	config.Get().AuthServerURL = dashboardServer.URL    // For metadata
	config.Get().ProxyURL = proxyServer.URL             // For metadata resource field
	config.Get().ProxyOAuthMetadataURL = proxyServer.URL + "/.well-known/oauth-protected-resource"


	return httpClient, dashboardServer.URL, proxyServer.URL
}

func teardownIntegrationTestEnv() {
	if dashboardServer != nil {
		dashboardServer.Close()
	}
	if proxyServer != nil {
		proxyServer.Close()
	}
	// DB cleanup can be done here if needed, or rely on in-memory nature
}

// Helper to perform login and get session cookie
func loginUserForSession(t *testing.T, client *http.Client, dashboardURL string) {
	loginData := url.Values{}
	loginData.Set("data[attributes][email]", testUser.Email)
	loginData.Set("data[attributes][password]", "password123")

	req, err := http.NewRequest("POST", dashboardURL+"/auth/login", strings.NewReader(loginData.Encode()))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded") // Or application/json if API expects that

	// If login expects JSON:
	// loginPayload := map[string]interface{}{"data": map[string]interface{}{"attributes": map[string]string{"email": testUser.Email, "password": "password123"}}}
	// jsonBody, _ := json.Marshal(loginPayload)
	// req, err = http.NewRequest("POST", dashboardURL+"/auth/login", bytes.NewBuffer(jsonBody))
	// req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode, "Login failed")
	// Cookies are now in client.Jar
}


func TestDCRFlow(t *testing.T) {
	client, dashboardURL, _ := setupIntegrationTestEnv(t)
	defer teardownIntegrationTestEnv()

	// 1. Simulate user login to establish session
	loginUserForSession(t, client, dashboardURL)

	// 2. Make DCR request
	dcrPayload := map[string]interface{}{
		"client_name":    "Test DCR Client",
		"redirect_uris":  []string{"http://localhost:7070/callback"},
		"scope":          "mcp offline",
		"grant_types":    []string{"authorization_code"},
		"response_types": []string{"code"},
		"token_endpoint_auth_method": "client_secret_post",
	}
	jsonBody, _ := json.Marshal(dcrPayload)

	dcrReq, err := http.NewRequest("POST", dashboardURL+"/oauth/register_client", bytes.NewBuffer(jsonBody))
	require.NoError(t, err)
	dcrReq.Header.Set("Content-Type", "application/json")
	// Cookies from loginUserForSession will be automatically included by client.Jar

	dcrResp, err := client.Do(dcrReq)
	require.NoError(t, err)
	defer dcrResp.Body.Close()

	require.Equal(t, http.StatusCreated, dcrResp.StatusCode, "DCR request failed")

	var dcrResult map[string]interface{}
	err = json.NewDecoder(dcrResp.Body).Decode(&dcrResult)
	require.NoError(t, err)

	require.NotEmpty(t, dcrResult["client_id"])
	require.NotEmpty(t, dcrResult["client_secret"])
	require.Equal(t, "Test DCR Client", dcrResult["client_name"])

	// Verify in DB
	clientID := dcrResult["client_id"].(string)
	oauthClientSvc := services.NewOAuthClientService(testDB)
	dbClient, err := oauthClientSvc.GetClient(clientID)
	require.NoError(t, err)
	require.NotNil(t, dbClient)
	require.Equal(t, "Test DCR Client", dbClient.ClientName)
	require.Equal(t, testUser.ID, dbClient.UserID) // Check association with logged-in user

	// Check if secret is hashed in DB (cannot directly compare plain to hash here without bcrypt)
	require.NotEmpty(t, dbClient.ClientSecret)
	require.NotEqual(t, dcrResult["client_secret"].(string), dbClient.ClientSecret)
}

// Placeholder for TestAuthorizationCodeFlow
// Placeholder for TestProxyTokenValidation

// Note: actual tests for Auth Code Flow and Proxy Token Validation will be complex
// and require careful step-by-step execution and state management (codes, tokens etc.)
// The above provides the foundational setup.
// The full TestAuthorizationCodeFlow and TestProxyTokenValidation will be added in the next step.

func TestFullAuthorizationCodeFlowAndProxyAccess(t *testing.T) {
	httpClient, dashboardURL, proxyURL := setupIntegrationTestEnv(t)
	defer teardownIntegrationTestEnv()

	// --- User Login ---
	loginUserForSession(t, httpClient, dashboardURL)

	// --- DCR (Simplified: directly create client for test reliability) ---
	oauthClientSvc := services.NewOAuthClientService(testDB)
	redirectURI := proxyURL + "/test/callback" // Dummy callback for proxy based test
	registeredClient, plainClientSecret, err := oauthClientSvc.CreateClient(
		"TestFlowClient", []string{redirectURI}, testUser.ID, "mcp profile",
	)
	require.NoError(t, err)
	clientID := registeredClient.ClientID

	// --- Initiate Authorization (/oauth/authorize) ---
	codeVerifier, codeChallenge, err := generatePKCEChallenge()
	require.NoError(t, err)
	state := "testState123"

	authURLValues := url.Values{}
	authURLValues.Set("response_type", "code")
	authURLValues.Set("client_id", clientID)
	authURLValues.Set("redirect_uri", redirectURI)
	authURLValues.Set("scope", "mcp profile")
	authURLValues.Set("state", state)
	authURLValues.Set("code_challenge", codeChallenge)
	authURLValues.Set("code_challenge_method", "S256")

	authReq, err := http.NewRequest("GET", dashboardURL+"/oauth/authorize?"+authURLValues.Encode(), nil)
	require.NoError(t, err)
	// Cookies from login are in httpClient.Jar

	authResp, err := httpClient.Do(authReq)
	require.NoError(t, err)
	defer authResp.Body.Close()

	require.Equal(t, http.StatusFound, authResp.StatusCode, "Expected redirect to consent page")
	consentPageURL, err := authResp.Location()
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(consentPageURL.String(), config.Get().SiteURL+"/oauth/consent"), "Should redirect to consent page on configured SiteURL")
	authRequestID := consentPageURL.Query().Get("auth_req_id")
	require.NotEmpty(t, authRequestID)

	// --- Simulate Consent ---
	// GET /oauth/consent_details (cookies are passed by httpClient)
	consentDetailsReq, err := http.NewRequest("GET", dashboardURL+"/oauth/consent_details?auth_req_id="+authRequestID, nil)
	require.NoError(t, err)
	consentDetailsResp, err := httpClient.Do(consentDetailsReq)
	require.NoError(t, err)
	defer consentDetailsResp.Body.Close()
	require.Equal(t, http.StatusOK, consentDetailsResp.StatusCode)
	// Can further verify details if needed

	// POST /oauth/submit_consent
	submitConsentPayload := map[string]string{"auth_req_id": authRequestID, "decision": "approved"}
	jsonConsentBody, _ := json.Marshal(submitConsentPayload)
	submitConsentReq, err := http.NewRequest("POST", dashboardURL+"/oauth/submit_consent", bytes.NewBuffer(jsonConsentBody))
	require.NoError(t, err)
	submitConsentReq.Header.Set("Content-Type", "application/json")

	submitConsentResp, err := httpClient.Do(submitConsentReq)
	require.NoError(t, err)
	defer submitConsentResp.Body.Close()

	require.Equal(t, http.StatusFound, submitConsentResp.StatusCode, "Expected redirect to client's redirect_uri")
	callbackURL, err := submitConsentResp.Location()
	require.NoError(t, err)
	require.Equal(t, redirectURI, callbackURL.Scheme+"://"+callbackURL.Host+callbackURL.Path)

	authCode := callbackURL.Query().Get("code")
	require.NotEmpty(t, authCode)
	require.Equal(t, state, callbackURL.Query().Get("state"))

	// --- Exchange Code for Token (/oauth/token) ---
	tokenReqPayload := url.Values{}
	tokenReqPayload.Set("grant_type", "authorization_code")
	tokenReqPayload.Set("code", authCode)
	tokenReqPayload.Set("redirect_uri", redirectURI)
	tokenReqPayload.Set("client_id", clientID)
	tokenReqPayload.Set("client_secret", plainClientSecret)
	tokenReqPayload.Set("code_verifier", codeVerifier)

	tokenReq, err := http.NewRequest("POST", dashboardURL+"/oauth/token", strings.NewReader(tokenReqPayload.Encode()))
	require.NoError(t, err)
	tokenReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	tokenResp, err := httpClient.Do(tokenReq) // Use a new client or clear cookies if client auth is basic/post not session based
	require.NoError(t, err)
	defer tokenResp.Body.Close()

	tokenRespBodyBytes, _ := io.ReadAll(tokenResp.Body)
	require.Equal(t, http.StatusOK, tokenResp.StatusCode, "Token exchange failed. Body: %s", string(tokenRespBodyBytes))

	var tokenResult map[string]interface{}
	err = json.Unmarshal(tokenRespBodyBytes, &tokenResult)
	require.NoError(t, err)
	accessToken, ok := tokenResult["access_token"].(string)
	require.True(t, ok)
	require.NotEmpty(t, accessToken)
	require.Equal(t, "Bearer", tokenResult["token_type"])

	// --- Verify Database State ---
	authCodeSvc := services.NewAuthCodeService(testDB)
	dbAuthCode, err := authCodeSvc.GetValidAuthCodeByCode(authCode) // Should fail as it's used
	require.Error(t, err, "Auth code should be marked as used or deleted effectively")
	if err != nil {
		require.Contains(t, err.Error(), "authorization code has already been used")
	}


	// --- Access Protected Resource with Token (Proxy Token Validation) ---
	// This part assumes a protected route on the proxy, e.g., /llm/test-model/foo
	// The proxy's CredentialValidator will be hit.
	// For this test, we need a dummy handler on the proxy that would be protected.
	// The existing proxy setup has /llm/{slug}/{rest...}, so we can try to hit that.
	// We need an LLM model setup that this token's user/client would have access to,
	// or the proxy's CheckAPICredential/new OAuth check should allow if scope matches etc.
	// For now, let's assume any authenticated request to a path matching a pattern is fine.

	// Setup a dummy LLM for the proxy to route to (if not using a generic auth path)
	llmForProxy := models.LLM{Name: "TestProtectedLLM", Slug: "test-protected-llm", Vendor: "dummy", Active: true, APIEndpoint: "http://dummy-upstream"}
	require.NoError(t, testDB.Create(&llmForProxy).Error)
	proxyInstance.LoadResources() // Reload proxy resources to pick up new LLM

	protectedReq, err := http.NewRequest("POST", proxyURL+"/llm/stream/"+llmForProxy.Slug+"/somecall", nil)
	require.NoError(t, err)
	protectedReq.Header.Set("Authorization", "Bearer "+accessToken)

	protectedResp, err := httpClient.Do(protectedReq)
	require.NoError(t, err)
	defer protectedResp.Body.Close()

	// If the dummy upstream isn't running, we might get 502 or similar from proxy.
	// The key is that it's NOT 401.
	require.NotEqual(t, http.StatusUnauthorized, protectedResp.StatusCode, "Access with valid token failed")

	// Test Invalid Token
	invalidTokenReq, err := http.NewRequest("POST", proxyURL+"/llm/stream/"+llmForProxy.Slug+"/somecall", nil)
	require.NoError(t, err)
	invalidTokenReq.Header.Set("Authorization", "Bearer invalidtoken123")
	invalidTokenResp, err := httpClient.Do(invalidTokenReq)
	require.NoError(t, err)
	defer invalidTokenResp.Body.Close()
	require.Equal(t, http.StatusUnauthorized, invalidTokenResp.StatusCode)
	require.Contains(t, invalidTokenResp.Header.Get("WWW-Authenticate"), "Bearer realm=\"MCPResources\"")

	// Test No Token
	noTokenReq, err := http.NewRequest("POST", proxyURL+"/llm/stream/"+llmForProxy.Slug+"/somecall", nil)
	require.NoError(t, err)
	noTokenResp, err := httpClient.Do(noTokenReq)
	require.NoError(t, err)
	defer noTokenResp.Body.Close()
	require.Equal(t, http.StatusUnauthorized, noTokenResp.StatusCode)
	require.Contains(t, noTokenResp.Header.Get("WWW-Authenticate"), "Bearer realm=\"MCPResources\"")
}
