package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/auth"
	"github.com/TykTechnologies/midsommar/v2/config"
	"github.com/TykTechnologies/midsommar/v2/helpers"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	apitesting "github.com/TykTechnologies/midsommar/v2/api/testing"
)

// MockOAuthClientService (remains the same)
type MockOAuthClientService struct{ mock.Mock }

func (m *MockOAuthClientService) CreateClient(name string, redirectURIs []string, userID uint, scope string) (*models.OAuthClient, string, error) {
	args := m.Called(name, redirectURIs, userID, scope)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).(*models.OAuthClient), args.String(1), args.Error(2)
}
func (m *MockOAuthClientService) GetClient(clientID string) (*models.OAuthClient, error) {
	args := m.Called(clientID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.OAuthClient), args.Error(1)
}
func (m *MockOAuthClientService) ValidateClientSecret(client *models.OAuthClient, secret string) (bool, error) {
	args := m.Called(client, secret)
	return args.Bool(0), args.Error(1)
}
func (m *MockOAuthClientService) ValidateRedirectURI(client *models.OAuthClient, redirectURI string) (bool, error) {
	args := m.Called(client, redirectURI)
	return args.Bool(0), args.Error(1)
}

// MockAuthCodeService (remains the same)
type MockAuthCodeService struct{ mock.Mock }

func (m *MockAuthCodeService) CreateAuthCode(argsIn services.CreateAuthCodeArgs) (*models.AuthCode, string, error) {
	args := m.Called(argsIn)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).(*models.AuthCode), args.String(1), args.Error(2)
}
func (m *MockAuthCodeService) GetValidAuthCodeByCode(codeValue string) (*models.AuthCode, error) {
	args := m.Called(codeValue)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.AuthCode), args.Error(1)
}
func (m *MockAuthCodeService) MarkAuthCodeAsUsed(codeValue string) error {
	return m.Called(codeValue).Error(0)
}

// MockAccessTokenService (remains the same)
type MockAccessTokenService struct{ mock.Mock }

func (m *MockAccessTokenService) CreateAccessToken(argsIn services.CreateAccessTokenArgs) (*models.AccessToken, string, error) {
	args := m.Called(argsIn)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).(*models.AccessToken), args.String(1), args.Error(2)
}
func (m *MockAccessTokenService) GetValidAccessTokenByToken(tokenValue string) (*models.AccessToken, error) {
	args := m.Called(tokenValue)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.AccessToken), args.Error(1)
}

// MockPendingAuthRequestService (remains the same)
type MockPendingAuthRequestService struct{ mock.Mock }

func (m *MockPendingAuthRequestService) StorePendingAuthRequest(argsIn services.StorePendingAuthRequestArgs) (*models.PendingOAuthRequest, error) {
	args := m.Called(argsIn)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.PendingOAuthRequest), args.Error(1)
}
func (m *MockPendingAuthRequestService) GetPendingAuthRequest(id string, userID uint) (*models.PendingOAuthRequest, error) {
	args := m.Called(id, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.PendingOAuthRequest), args.Error(1)
}
func (m *MockPendingAuthRequestService) DeletePendingAuthRequest(id string) error {
	return m.Called(id).Error(0)
}

// AuthServiceMock with refined AuthMiddleware
type AuthServiceMock struct {
	mock.Mock
	SimulateAuthenticatedUser *models.User
	SimulateAuthError         error
}

func (m *AuthServiceMock) AuthMiddleware() gin.HandlerFunc {
	// This method itself is called when setting up the router.
	// The mock framework will expect an .On("AuthMiddleware").Return(...) for this.
	// The returned gin.HandlerFunc is what gets executed for each request.
	args := m.MethodCalled("AuthMiddleware")
	if handlerFunc, ok := args.Get(0).(gin.HandlerFunc); ok {
		return handlerFunc
	}
	// Fallback if .Return() was not a gin.HandlerFunc or not configured.
	return func(c *gin.Context) {
		// Default behavior for the actual middleware if not specifically configured by .Return(specificHandler)
		if m.SimulateAuthError != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error_middleware": m.SimulateAuthError.Error()})
			return
		}
		if m.SimulateAuthenticatedUser != nil {
			c.Set("user", m.SimulateAuthenticatedUser)
		}
		c.Next()
	}
}

// Implement other auth.AuthService methods as stubs
func (m *AuthServiceMock) Login(c *gin.Context, email, password string) error {
	return m.Called(c, email, password).Error(0)
}
func (m *AuthServiceMock) Logout(c *gin.Context) error { return m.Called(c).Error(0) }
func (m *AuthServiceMock) Register(email, name, password string, withPortal, withChat bool) error {
	return m.Called(email, name, password, withPortal, withChat).Error(0)
}
func (m *AuthServiceMock) ResetPassword(email string) error { return m.Called(email).Error(0) }
func (m *AuthServiceMock) ValidateResetToken(token string) (*models.User, error) {
	args := m.Called(token)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.User), args.Error(1)
}
func (m *AuthServiceMock) UpdatePassword(user *models.User, oldPassword, newPassword string) error {
	return m.Called(user, oldPassword, newPassword).Error(0)
}
func (m *AuthServiceMock) VerifyEmail(token string) error { return m.Called(token).Error(0) }
func (m *AuthServiceMock) ResendVerificationEmail(email string) error {
	return m.Called(email).Error(0)
}
func (m *AuthServiceMock) AdminOnly() gin.HandlerFunc {
	m.MethodCalled("AdminOnly")
	return func(c *gin.Context) { c.Next() }
}
func (m *AuthServiceMock) SSOOnly() gin.HandlerFunc {
	m.MethodCalled("SSOOnly")
	return func(c *gin.Context) { c.Next() }
}
func (m *AuthServiceMock) SSOAuthMiddleware() gin.HandlerFunc {
	m.MethodCalled("SSOAuthMiddleware")
	return func(c *gin.Context) { c.Next() }
}

func setupTestAPIWithMocks(t *testing.T) (*API, *gin.Engine, *MockOAuthClientService, *MockAuthCodeService, *MockAccessTokenService, *MockPendingAuthRequestService, *AuthServiceMock) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	mockOAuthClientSvc := new(MockOAuthClientService)
	mockAuthCodeSvc := new(MockAuthCodeService)
	mockAccessTokenSvc := new(MockAccessTokenService)
	mockPendingAuthReqSvc := new(MockPendingAuthRequestService)
	mockAuthSvc := new(AuthServiceMock)

	testDB := apitesting.SetupTestDB(t)
	authConfig := &auth.Config{DB: testDB, TestMode: true, RegistrationAllowed: true, FrontendURL: "http://localhost:3000"}

	realService := apitesting.SetupTestService(testDB)
	realAuthService := apitesting.SetupTestAuthService(testDB, realService)

	apiInstance := &API{
		router:   router,
		config:   authConfig,
		auth:     realAuthService,
		service:  realService,
		licenser: apitesting.SetupTestLicenser(),
	}

	// Configure the default behavior for AuthMiddleware() calls during router setup
	// This single returned handler will use the fields on mockAuthSvc instance (SimulateAuthenticatedUser, SimulateAuthError)
	// which can be manipulated by each test.
	defaultMiddlewareHandler := func(c *gin.Context) {
		if mockAuthSvc.SimulateAuthError != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error_middleware": mockAuthSvc.SimulateAuthError.Error()})
			return
		}
		if mockAuthSvc.SimulateAuthenticatedUser != nil {
			c.Set("user", mockAuthSvc.SimulateAuthenticatedUser)
		}
		c.Next()
	}
	// Tell the mock to return this handler whenever AuthMiddleware() is called.
	// Use Maybe() if not all paths in all tests will hit routes using this middleware.
	mockAuthSvc.On("AuthMiddleware").Return(defaultMiddlewareHandler)

	oauthGroup := router.Group("/oauth")
	oauthGroup.POST("/register_client", mockAuthSvc.AuthMiddleware(), apiInstance.handleRegisterOAuthClient)
	oauthGroup.GET("/authorize", mockAuthSvc.AuthMiddleware(), apiInstance.handleOAuthAuthorize)
	oauthGroup.POST("/token", apiInstance.handleOAuthToken)
	oauthGroup.GET("/consent_details", mockAuthSvc.AuthMiddleware(), apiInstance.handleGetConsentDetails)
	oauthGroup.POST("/submit_consent", mockAuthSvc.AuthMiddleware(), apiInstance.handleSubmitConsent)
	router.GET("/.well-known/oauth-authorization-server", apiInstance.handleOAuthMetadata)

	return apiInstance, router, mockOAuthClientSvc, mockAuthCodeSvc, mockAccessTokenSvc, mockPendingAuthReqSvc, mockAuthSvc
}

func performOAuthRequest(r http.Handler, method, path string, body interface{}, headers map[string]string) *httptest.ResponseRecorder {
	var reqBodyBytes []byte
	if body != nil {
		if str, ok := body.(string); ok {
			reqBodyBytes = []byte(str)
		} else {
			reqBodyBytes, _ = json.Marshal(body)
		}
	}
	req, _ := http.NewRequest(method, path, bytes.NewBuffer(reqBodyBytes))
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	if req.Header.Get("Content-Type") == "" {
		if _, isStr := body.(string); !isStr && body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
	}

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

var testUserGlobal = &models.User{Model: gorm.Model{ID: 1}, Email: "testuser@example.com", Name: "Test User", IsAdmin: false}

// Helper to ensure user exists in DB for tests
func ensureUserInDB(t *testing.T, db *gorm.DB, user *models.User) *models.User {
	var dbUser models.User
	err := db.FirstOrCreate(&dbUser, models.User{Model: gorm.Model{ID: user.ID}, Email: user.Email, Name: user.Name, IsAdmin: user.IsAdmin}).Error
	require.NoError(t, err)
	// If password needs to be set and user is newly created:
	if dbUser.Password == "" && user.Password != "" { // Assuming user might have plain password for test setup
		require.NoError(t, dbUser.SetPassword(user.Password)) // Ensure user.Password is plain text if used here
		require.NoError(t, db.Save(&dbUser).Error)
	}
	return &dbUser
}

func TestHandleOAuthMetadata(t *testing.T) {
	_, router, _, _, _, _, _ := setupTestAPIWithMocks(t)
	originalAuthServerURL := config.Get().AuthServerURL
	config.Get().AuthServerURL = "http://auth.example.com"
	defer func() { config.Get().AuthServerURL = originalAuthServerURL }()

	w := performOAuthRequest(router, "GET", "/.well-known/oauth-authorization-server", nil, nil)
	require.Equal(t, http.StatusOK, w.Code)
	var metadata OAuthServerMetadata
	err := json.Unmarshal(w.Body.Bytes(), &metadata)
	require.NoError(t, err)
	require.Equal(t, "http://auth.example.com", metadata.Issuer)
	require.Equal(t, "http://auth.example.com/oauth/authorize", metadata.AuthorizationEndpoint)
}

func TestHandleRegisterOAuthClient_Success(t *testing.T) {
	_, router, _, _, _, _, mockAuthSvc := setupTestAPIWithMocks(t)

	mockAuthSvc.SimulateAuthenticatedUser = testUserGlobal // Configure middleware behavior
	mockAuthSvc.SimulateAuthError = nil
	// mockAuthSvc.On("AuthMiddleware").Return(...) // This is now handled globally in setupTestAPIWithMocks

	input := gin.H{
		"client_name": "Test MCP Client", "redirect_uris": []string{"http://localhost:8080/callback"}, "scope": "mcp",
	}
	w := performOAuthRequest(router, "POST", "/oauth/register_client", input, nil)
	require.Equal(t, http.StatusCreated, w.Code, "Response body: "+w.Body.String())
	var resp RegisterOAuthClientOutput
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	require.NotEmpty(t, resp.ClientID)
	require.NotEmpty(t, resp.ClientSecret)
	require.Equal(t, "Test MCP Client", resp.ClientName)
	// mockAuthSvc.AssertExpectations(t) // Asserting on method calls to AuthMiddleware can be tricky if it's called many times during setup
}

func TestHandleRegisterOAuthClient_InvalidBody(t *testing.T) {
	_, router, _, _, _, _, mockAuthSvc := setupTestAPIWithMocks(t)
	mockAuthSvc.SimulateAuthenticatedUser = testUserGlobal
	mockAuthSvc.SimulateAuthError = nil

	input := gin.H{"redirect_uris": []string{"http://localhost:8080/callback"}, "scope": "mcp"}
	w := performOAuthRequest(router, "POST", "/oauth/register_client", input, nil)
	require.Equal(t, http.StatusBadRequest, w.Code)
	var respBody map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &respBody)
	require.NoError(t, err)
	errorsField, _ := respBody["errors"].([]interface{})
	firstError, _ := errorsField[0].(map[string]interface{})
	require.Contains(t, strings.ToLower(firstError["detail"].(string)), "clientname")
}

func TestHandleRegisterOAuthClient_AuthFailure(t *testing.T) {
	_, router, _, _, _, _, mockAuthSvc := setupTestAPIWithMocks(t)
	authErr := errors.New("simulated auth failure")
	mockAuthSvc.SimulateAuthenticatedUser = nil
	mockAuthSvc.SimulateAuthError = authErr

	input := gin.H{"client_name": "Test MCP Client", "redirect_uris": []string{"http://localhost:8080/callback"}, "scope": "mcp"}
	w := performOAuthRequest(router, "POST", "/oauth/register_client", input, nil)
	require.Equal(t, http.StatusUnauthorized, w.Code)
	var respBody map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &respBody)
	require.NoError(t, err)
	require.Contains(t, respBody["error_middleware"], "simulated auth failure")
}

func TestHandleOAuthAuthorize_SuccessRedirectsToConsent(t *testing.T) {
	apiInstance, router, _, _, _, _, mockAuthSvc := setupTestAPIWithMocks(t)
	testDB := apiInstance.config.DB

	currentUser := ensureUserInDB(t, testDB, testUserGlobal)
	mockAuthSvc.SimulateAuthenticatedUser = currentUser
	mockAuthSvc.SimulateAuthError = nil

	clientSvc := services.NewOAuthClientService(testDB)
	testClient, _, err := clientSvc.CreateClient("AuthzTestClient", []string{"http://client.example.com/callback"}, &currentUser.ID, "mcp")
	require.NoError(t, err)

	appConf := config.Get()
	originalSiteURL := appConf.SiteURL
	appConf.SiteURL = "http://dashboard.example.com"
	defer func() { appConf.SiteURL = originalSiteURL }()
	authURL := "/oauth/authorize?response_type=code&client_id=" + testClient.ClientID + "&redirect_uri=" + url.QueryEscape("http://client.example.com/callback") + "&code_challenge=challenge&code_challenge_method=S256&scope=mcp&state=123"
	w := performOAuthRequest(router, "GET", authURL, nil, nil)
	require.Equal(t, http.StatusFound, w.Code, "Body: "+w.Body.String())
	location := w.Header().Get("Location")
	require.True(t, strings.HasPrefix(location, "http://dashboard.example.com/oauth/consent?auth_req_id="), "Unexpected redirect URL: "+location)

	pendingReqService := services.NewPendingAuthRequestService(testDB)
	parsedLocation, _ := url.Parse(location)
	authReqID := parsedLocation.Query().Get("auth_req_id")
	_, err = pendingReqService.GetPendingAuthRequest(authReqID, currentUser.ID)
	require.NoError(t, err)
}

func TestHandleOAuthAuthorize_ValidationErrors(t *testing.T) {
	_, router, _, _, _, _, mockAuthSvc := setupTestAPIWithMocks(t)
	mockAuthSvc.SimulateAuthenticatedUser = testUserGlobal
	mockAuthSvc.SimulateAuthError = nil

	testCases := []struct {
		name                   string
		queryString            string
		expectedErrorSubstring string
	}{
		{"no_response_type", "?client_id=1&redirect_uri=x&code_challenge=y&code_challenge_method=S256", "response_type must be 'code'"},
		{"no_client_id", "?response_type=code&redirect_uri=x&code_challenge=y&code_challenge_method=S256", "client_id, redirect_uri, code_challenge, and code_challenge_method are required"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			w := performOAuthRequest(router, "GET", "/oauth/authorize"+tc.queryString, nil, nil)
			require.Equal(t, http.StatusBadRequest, w.Code)
			var resp gin.H
			err := json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Contains(t, resp["error_description"], tc.expectedErrorSubstring)
		})
	}
}

func TestHandleOAuthToken_ValidCode(t *testing.T) {
	apiInstance, router, _, _, _, _, _ := setupTestAPIWithMocks(t)
	testDB := apiInstance.config.DB

	dbUser := models.User{Email: "tokenuser@example.com", Name: "Token User", EmailVerified: true}
	require.NoError(t, dbUser.SetPassword("password123"))
	require.NoError(t, testDB.Create(&dbUser).Error)

	// Create a tool and app for the user
	dummyTool := &models.Tool{
		Name:         "Test Token Tool",
		Description:  "Test tool for token test",
		ToolType:     models.ToolTypeREST,
		PrivacyScore: 5,
	}
	require.NoError(t, testDB.Create(dummyTool).Error)

	testService := services.NewService(testDB)
	testApp, err := testService.CreateApp(
		"Test Token App",
		"Test app for token",
		dbUser.ID,
		[]uint{},             // no datasources
		[]uint{},             // no LLMs
		[]uint{dummyTool.ID}, // one tool
		nil,                  // no budget
		nil,                  // no budget start date
	)
	require.NoError(t, err)
	require.NotNil(t, testApp)

	// Activate the app's credential
	require.NoError(t, testDB.Model(&models.Credential{}).Where("id = ?", testApp.CredentialID).Update("active", true).Error)

	oauthClientService := services.NewOAuthClientService(testDB)
	client, plainSecret, err := oauthClientService.CreateClient("TokenTestClient", []string{"http://client.example.com/cb"}, &dbUser.ID, "mcp")
	require.NoError(t, err)

	authCodeService := services.NewAuthCodeService(testDB)
	codeVerifier := "testverifier"
	realChallenge := helpers.CalculatePKCEChallengeS256(codeVerifier)
	codeArgs := services.CreateAuthCodeArgs{
		ClientID: client.ClientID, UserID: dbUser.ID, RedirectURI: "http://client.example.com/cb",
		Scope: "mcp", ExpiresIn: 10 * time.Minute, CodeChallenge: realChallenge, CodeChallengeMethod: "S256",
		AppID: &testApp.ID, // Associate with the created app
	}
	authCodeInstance, codeValue, err := authCodeService.CreateAuthCode(codeArgs)
	require.NoError(t, err)
	require.NotNil(t, authCodeInstance)

	form := url.Values{
		"grant_type": {"authorization_code"}, "code": {codeValue}, "redirect_uri": {"http://client.example.com/cb"},
		"client_id": {client.ClientID}, "client_secret": {plainSecret}, "code_verifier": {codeVerifier},
	}
	w := performOAuthRequest(router, "POST", "/oauth/token", form.Encode(), map[string]string{"Content-Type": "application/x-www-form-urlencoded"})
	require.Equal(t, http.StatusOK, w.Code, "Response body: "+w.Body.String())
	var resp AccessTokenResponse
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	require.NotEmpty(t, resp.AccessToken)
}

func TestHandleOAuthToken_ErrorCases(t *testing.T) {
	_, router, _, _, _, _, _ := setupTestAPIWithMocks(t)
	testCases := []struct {
		name                     string
		formData                 url.Values
		expectedCode             int
		expectedErrDescSubstring string
	}{
		{"invalid_grant_type", url.Values{"grant_type": {"wrong_type"}, "code": {"c"}, "redirect_uri": {"r"}, "client_id": {"id"}, "code_verifier": {"v"}}, http.StatusBadRequest, "grant_type must be 'authorization_code'"},
		{"missing_params", url.Values{"grant_type": {"authorization_code"}}, http.StatusBadRequest, "Missing required parameters"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			w := performOAuthRequest(router, "POST", "/oauth/token", tc.formData.Encode(), map[string]string{"Content-Type": "application/x-www-form-urlencoded"})
			require.Equal(t, tc.expectedCode, w.Code, "Response: %s", w.Body.String())
			var resp OAuthErrorResponse
			err := json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Contains(t, resp.ErrorDescription, tc.expectedErrDescSubstring)
		})
	}
}

func TestHandleGetConsentDetails_Success(t *testing.T) {
	apiInstance, router, _, _, _, _, mockAuthSvc := setupTestAPIWithMocks(t)
	testDB := apiInstance.config.DB
	currentUser := ensureUserInDB(t, testDB, testUserGlobal)

	mockAuthSvc.SimulateAuthenticatedUser = currentUser
	mockAuthSvc.SimulateAuthError = nil

	clientSvc := services.NewOAuthClientService(testDB)
	client, _, _ := clientSvc.CreateClient("ConsentTestClient", []string{"http://cb"}, &currentUser.ID, "mcp profile")
	pendingSvc := services.NewPendingAuthRequestService(testDB)
	pendingReqArgs := services.StorePendingAuthRequestArgs{
		ClientID: client.ClientID, UserID: currentUser.ID, RedirectURI: "http://cb", Scope: "mcp profile",
		ExpiresIn: 5 * time.Minute, CodeChallenge: "ch", CodeChallengeMethod: "S256",
	}
	pendingReq, _ := pendingSvc.StorePendingAuthRequest(pendingReqArgs)
	w := performOAuthRequest(router, "GET", "/oauth/consent_details?auth_req_id="+pendingReq.ID, nil, nil)
	require.Equal(t, http.StatusOK, w.Code)
	var resp ConsentDetailsResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	require.Equal(t, pendingReq.ID, resp.AuthRequestID)
	require.Equal(t, "ConsentTestClient", resp.ClientName)
}

func TestHandleSubmitConsent_Approved(t *testing.T) {
	apiInstance, router, _, _, _, _, mockAuthSvc := setupTestAPIWithMocks(t)
	testDB := apiInstance.config.DB
	currentUser := ensureUserInDB(t, testDB, testUserGlobal)

	mockAuthSvc.SimulateAuthenticatedUser = currentUser
	mockAuthSvc.SimulateAuthError = nil

	// Create a tool and app for the user
	dummyTool := &models.Tool{
		Name:         "Test Consent Tool",
		Description:  "Test tool for consent test",
		ToolType:     models.ToolTypeREST,
		PrivacyScore: 5,
	}
	require.NoError(t, testDB.Create(dummyTool).Error)

	testService := services.NewService(testDB)
	testApp, err := testService.CreateApp(
		"Test Consent App",
		"Test app for consent",
		currentUser.ID,
		[]uint{},             // no datasources
		[]uint{},             // no LLMs
		[]uint{dummyTool.ID}, // one tool
		nil,                  // no budget
		nil,                  // no budget start date
	)
	require.NoError(t, err)
	require.NotNil(t, testApp)

	// Activate the app's credential
	require.NoError(t, testDB.Model(&models.Credential{}).Where("id = ?", testApp.CredentialID).Update("active", true).Error)

	clientSvc := services.NewOAuthClientService(testDB)
	client, _, _ := clientSvc.CreateClient("ConsentSubmitClient", []string{"http://client.com/cb"}, &currentUser.ID, "mcp")
	pendingSvc := services.NewPendingAuthRequestService(testDB)
	pendingReqArgs := services.StorePendingAuthRequestArgs{
		ClientID: client.ClientID, UserID: currentUser.ID, RedirectURI: "http://client.com/cb", State: "s123",
		Scope: "mcp", ExpiresIn: 5 * time.Minute, CodeChallenge: "ch", CodeChallengeMethod: "S256",
	}
	pendingReq, _ := pendingSvc.StorePendingAuthRequest(pendingReqArgs)
	input := SubmitConsentInput{AuthRequestID: pendingReq.ID, Decision: "approved", SelectedAppID: testApp.ID}
	w := performOAuthRequest(router, "POST", "/oauth/submit_consent", input, nil)
	require.Equal(t, http.StatusFound, w.Code)
	location := w.Header().Get("Location")
	require.Contains(t, location, "http://client.com/cb?")
	require.Contains(t, location, "code=")
}

func TestHandleSubmitConsent_Denied(t *testing.T) {
	apiInstance, router, _, _, _, _, mockAuthSvc := setupTestAPIWithMocks(t)
	testDB := apiInstance.config.DB
	currentUser := ensureUserInDB(t, testDB, testUserGlobal)

	mockAuthSvc.SimulateAuthenticatedUser = currentUser
	mockAuthSvc.SimulateAuthError = nil

	clientSvc := services.NewOAuthClientService(testDB)
	client, _, _ := clientSvc.CreateClient("ConsentDenyClient", []string{"http://client.deny.com/cb"}, &currentUser.ID, "mcp")
	pendingSvc := services.NewPendingAuthRequestService(testDB)
	pendingReqArgs := services.StorePendingAuthRequestArgs{
		ClientID: client.ClientID, UserID: currentUser.ID, RedirectURI: "http://client.deny.com/cb", State: "denystate",
		Scope: "mcp", ExpiresIn: 5 * time.Minute, CodeChallenge: "ch", CodeChallengeMethod: "S256",
	}
	pendingReq, _ := pendingSvc.StorePendingAuthRequest(pendingReqArgs)
	input := SubmitConsentInput{AuthRequestID: pendingReq.ID, Decision: "denied"}
	w := performOAuthRequest(router, "POST", "/oauth/submit_consent", input, nil)
	require.Equal(t, http.StatusFound, w.Code)
	location := w.Header().Get("Location")
	require.Contains(t, location, "http://client.deny.com/cb?")
	require.Contains(t, location, "error=access_denied")
}
