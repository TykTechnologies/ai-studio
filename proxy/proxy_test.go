package proxy

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gosimple/slug"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/config" // Added import
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
)

const (
	testToolName = "Test Mock Tool"
	testToolSlug = "test-mock-tool"
)

// newTestToolDefinition creates a sample tool definition for testing.
// The endpointURL parameter allows dynamic setting of the mock server's URL.
func newTestToolDefinition(endpointURL string) *models.Tool {
	// Define operation IDs for consistency
	opGetTestDataID := "getTestData"       // Was "get-test-data"
	opSubmitTestDataID := "submitTestData" // Was "submit-test-data"

	// Create OAS spec with updated operation IDs
	rawOASSpec := fmt.Sprintf(`{"openapi":"3.0.0","info":{"title":"Test Tool","version":"1.0.0"},"servers":[{"url":"%s"}],"paths":{"/test":{"get":{"operationId":"%s","summary":"Get Test Data","description":"Retrieves test data.","responses":{"200":{"description":"Success"}}}},"/submit":{"post":{"operationId":"%s","summary":"Submit Test Data","description":"Submits test data.","responses":{"200":{"description":"Success"}}}}}}`, endpointURL, opGetTestDataID, opSubmitTestDataID)

	// Base64 encode the OAS spec as required by the proxy
	base64OASSpec := base64.StdEncoding.EncodeToString([]byte(rawOASSpec))

	tool := &models.Tool{
		Name:         testToolName,
		Description:  "A mock tool for testing proxy functionality.",
		ToolType:     models.ToolTypeREST,
		OASSpec:      base64OASSpec,
		PrivacyScore: 5,
	}

	// Add operations directly with updated operation IDs
	tool.AddOperation(opGetTestDataID)
	tool.AddOperation(opSubmitTestDataID)

	return tool
}

// registerTestTool registers the test tool with the provided service.
// It returns the registered tool definition.
func registerTestTool(t *testing.T, service *services.Service, mockServerURL string) *models.Tool {
	t.Helper()
	toolDef := newTestToolDefinition(mockServerURL)
	tool, err := service.CreateTool(
		toolDef.Name,
		toolDef.Description,
		toolDef.ToolType,
		toolDef.OASSpec,
		toolDef.PrivacyScore,
		"",
		"",
	)
	assert.NoError(t, err, "Failed to register test tool")
	return tool
}

// unregisterTestTool removes the test tool from the service.
func unregisterTestTool(t *testing.T, service *services.Service, slug string) {
	t.Helper()
	// Find the tool by slug first
	tool, err := service.GetToolBySlug(slug)
	if err != nil {
		// If not found, just return
		return
	}
	// Delete the tool by ID
	err = service.DeleteTool(tool.ID)
	assert.NoError(t, err, "Failed to unregister test tool")
}

// recordedRequest stores information about a request received by the mock server.
type recordedRequest struct {
	Method  string
	Path    string
	Body    []byte
	Headers http.Header
}

// mockServerConfig holds the configuration for the mock HTTP server.
type mockServerConfig struct {
	ResponseStatus int
	ResponseBody   string
	// HeadersToReturn is a map of header keys to values to be returned by the mock server.
	HeadersToReturn map[string]string
	// ExpectedMethod is the HTTP method the mock server expects to receive.
	ExpectedMethod string
	// ExpectedPath is the path the mock server expects to receive requests on.
	ExpectedPath string

	// ReceivedRequests records all requests made to the mock server.
	ReceivedRequests []*recordedRequest
	mu               sync.Mutex // For thread-safe access to ReceivedRequests
}

// newMockServer sets up and starts a new mock HTTP server.
// It returns the server instance and a function to close the server.
func newMockServer(t *testing.T, config *mockServerConfig) (*httptest.Server, func()) {
	t.Helper()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		config.mu.Lock()
		defer config.mu.Unlock()

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read request body", http.StatusInternalServerError)
			return
		}
		defer r.Body.Close()

		req := &recordedRequest{
			Method:  r.Method,
			Path:    r.URL.Path,
			Body:    body,
			Headers: r.Header.Clone(),
		}
		config.ReceivedRequests = append(config.ReceivedRequests, req)

		// Check if the method and path match expectations, if provided
		if config.ExpectedMethod != "" && r.Method != config.ExpectedMethod {
			t.Errorf("Expected method %s, got %s", config.ExpectedMethod, r.Method)
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if config.ExpectedPath != "" && r.URL.Path != config.ExpectedPath {
			t.Errorf("Expected path %s, got %s", config.ExpectedPath, r.URL.Path)
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}

		for key, value := range config.HeadersToReturn {
			w.Header().Set(key, value)
		}

		w.WriteHeader(config.ResponseStatus)
		if _, err := w.Write([]byte(config.ResponseBody)); err != nil {
			t.Logf("Error writing response body: %v", err)
		}
	})

	server := httptest.NewServer(handler)
	tearDown := func() {
		server.Close()
	}

	return server, tearDown
}

func TestProxySetup(t *testing.T) {
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	service := services.NewService(db)
	notificationSvc := services.NewTestNotificationService(db)
	budgetService := services.NewBudgetService(db, notificationSvc)

	config := &Config{Port: 8080}
	p := NewProxy(service, config, budgetService)
	assert.NotNil(t, p)
}

func TestConcurrentAccess(t *testing.T) {
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	service := services.NewService(db)
	notificationSvc := services.NewTestNotificationService(db)
	budgetService := services.NewBudgetService(db, notificationSvc)
	proxy := NewProxy(service, &Config{Port: 9999}, budgetService)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = proxy.llms
			_ = proxy.datasources
		}()
	}
	wg.Wait()
}

func TestHandleToolRequest_ValidGET(t *testing.T) {
	// 1. Setup: DB, services, user, app, API key
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	service := services.NewService(db)
	notificationSvc := services.NewTestNotificationService(db)
	budgetService := services.NewBudgetService(db, notificationSvc) // Added budgetService

	user, err := service.CreateUser(services.UserDTO{
		Email:                "test@example.com",
		Name:                 "testuser",
		Password:             "password123",
		IsAdmin:              false,
		ShowChat:             true,
		ShowPortal:           false,
		EmailVerified:        false,
		NotificationsEnabled: false,
		AccessToSSOConfig:    false,
	})
	require.NoError(t, err)
	require.NotNil(t, user)

	// 2. Setup Mock Server
	mockServerConfig := &mockServerConfig{
		ResponseStatus: http.StatusOK,
		ResponseBody:   `{"message": "GET success"}`,
		ExpectedMethod: http.MethodGet,
		ExpectedPath:   "/test",
	}
	mockHttpServer, mockTeardown := newMockServer(t, mockServerConfig)
	defer mockTeardown()

	// 3. Register Test Tool and associate with App
	// IMPORTANT: Tool must be registered before creating the app with its ID.
	toolDefForApp := newTestToolDefinition(mockHttpServer.URL)
	// Create the tool to get an ID
	createdTool, err := service.CreateTool(
		toolDefForApp.Name,
		toolDefForApp.Description,
		toolDefForApp.ToolType,
		toolDefForApp.OASSpec,
		toolDefForApp.PrivacyScore,
		"", // Auth schema name
		"",
	)
	require.NoError(t, err)
	require.NotNil(t, createdTool)
	// Ensure the tool is found by slug for unregistration
	registeredToolDef, err := service.GetToolBySlug(testToolSlug)
	require.NoError(t, err)
	require.NotNil(t, registeredToolDef)

	app, err := service.CreateApp("Test App", "App for testing", user.ID, []uint{}, []uint{}, []uint{registeredToolDef.ID}, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, app)

	// Activate the app's credential
	err = service.ActivateAppCredential(app.ID)
	require.NoError(t, err)

	// Reload the app to get the credential
	app, err = service.GetAppByID(app.ID)
	require.NoError(t, err)
	require.NotNil(t, app.Credential)
	apiKey := app.Credential.Secret
	require.NotNil(t, apiKey)

	// 4. Setup Proxy
	proxyConfig := &Config{Port: 9998} // Use a different port for this test proxy
	p := NewProxy(service, proxyConfig, budgetService)
	err = p.loadResources() // Load tools, LLMs etc. into the proxy
	require.NoError(t, err)
	proxyRouter := p.createHandler()

	// 5. Prepare Proxy Request
	requestBody := map[string]interface{}{
		"operation_id": "getTestData", // Updated to match operationId in OASSpec
		"parameters":   map[string][]string{},
		"payload":      nil,
		"headers":      map[string][]string{},
	}
	jsonBody, err := json.Marshal(requestBody)
	require.NoError(t, err)

	reqPath := "/tools/" + testToolSlug
	proxyReq, err := http.NewRequest(http.MethodPost, reqPath, bytes.NewBuffer(jsonBody)) // handleToolRequest expects POST for operation dispatch
	require.NoError(t, err)
	proxyReq.Header.Set("Authorization", "Bearer "+apiKey)
	proxyReq.Header.Set("Content-Type", "application/json")

	// 6. Perform Request & Assert Response
	rr := httptest.NewRecorder()
	proxyRouter.ServeHTTP(rr, proxyReq)

	assert.Equal(t, http.StatusOK, rr.Code, "HTTP status code should be 200 OK")

	// Verify the response is correctly formatted as expected by tools output
	var response map[string]interface{}
	err = json.Unmarshal(rr.Body.Bytes(), &response)
	require.NoError(t, err)

	// Check if the response contains the data key from mock server
	// We've configured the mock server to return {"message": "GET success"}
	assert.Equal(t, "GET success", response["message"], "Response content should match the mock server response")

	// 7. Assert Mock Server Received Request
	require.Len(t, mockServerConfig.ReceivedRequests, 1, "Mock server should have received one request")
	receivedReq := mockServerConfig.ReceivedRequests[0]
	assert.Equal(t, http.MethodGet, receivedReq.Method)
	assert.Equal(t, "/test", receivedReq.Path)

	// 8. Cleanup tool (app, user, key will be cleaned by DB drop or further teardown if needed)
	// Unregister tool using the slug from the initially registered definition
	unregisterTestTool(t, service, slug.Make(registeredToolDef.Name))
}

func TestHandleToolRequest_ValidPOST(t *testing.T) {
	// 1. Setup: DB, services, user, app, API key
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	service := services.NewService(db)
	notificationSvc := services.NewTestNotificationService(db)
	budgetService := services.NewBudgetService(db, notificationSvc)

	user, err := service.CreateUser(services.UserDTO{
		Email:                "testpost@example.com",
		Name:                 "testuser-post",
		Password:             "password123",
		IsAdmin:              false,
		ShowChat:             true,
		ShowPortal:           false,
		EmailVerified:        false,
		NotificationsEnabled: false,
		AccessToSSOConfig:    false,
	})
	require.NoError(t, err)

	// 2. Setup Mock Server
	expectedRequestBody := `{"data":"test_payload"}`
	mockResponseBody := `{"id": "123", "status": "created"}`
	mockServerConfig := &mockServerConfig{
		ResponseStatus: http.StatusCreated,
		ResponseBody:   mockResponseBody,
		ExpectedMethod: http.MethodPost,
		ExpectedPath:   "/submit",
	}
	mockHttpServer, mockTeardown := newMockServer(t, mockServerConfig)
	defer mockTeardown()

	// 3. Register Test Tool and associate with App
	toolDefForApp := newTestToolDefinition(mockHttpServer.URL)
	// Create the tool to get an ID
	createdTool, err := service.CreateTool(
		toolDefForApp.Name,
		toolDefForApp.Description,
		toolDefForApp.ToolType,
		toolDefForApp.OASSpec,
		toolDefForApp.PrivacyScore,
		"", // Auth schema name
		"",
	)
	require.NoError(t, err)
	require.NotNil(t, createdTool)
	registeredToolDef, err := service.GetToolBySlug(testToolSlug) // Fetch full def for ID and correct slug
	require.NoError(t, err)
	require.NotNil(t, registeredToolDef)

	app, err := service.CreateApp("Test App POST", "Test App POST", user.ID, []uint{}, []uint{}, []uint{registeredToolDef.ID}, nil, nil)
	require.NoError(t, err)
	// Activate the app's credential
	err = service.ActivateAppCredential(app.ID)
	require.NoError(t, err)

	// Reload the app to get the credential
	app, err = service.GetAppByID(app.ID)
	require.NoError(t, err)
	require.NotNil(t, app.Credential)
	apiKey := app.Credential.Secret

	// 4. Setup Proxy
	proxyConfig := &Config{Port: 9997}
	p := NewProxy(service, proxyConfig, budgetService)
	err = p.loadResources()
	require.NoError(t, err)
	proxyRouter := p.createHandler()

	// 5. Prepare Proxy Request
	requestPayload := map[string]interface{}{"data": "test_payload"}
	proxyRequestBody := map[string]interface{}{
		"operation_id": "submitTestData", // Updated to match operationId in OASSpec
		"parameters":   map[string][]string{},
		"payload":      requestPayload,
		"headers":      map[string][]string{},
	}
	jsonBody, err := json.Marshal(proxyRequestBody)
	require.NoError(t, err)

	reqPath := "/tools/" + testToolSlug
	proxyReq, err := http.NewRequest(http.MethodPost, reqPath, bytes.NewBuffer(jsonBody))
	require.NoError(t, err)
	proxyReq.Header.Set("Authorization", "Bearer "+apiKey)
	proxyReq.Header.Set("Content-Type", "application/json")

	// 6. Perform Request & Assert Response
	rr := httptest.NewRecorder()
	proxyRouter.ServeHTTP(rr, proxyReq)

	assert.Equal(t, http.StatusOK, rr.Code, "Proxy should return 200 OK")

	var proxyResponse map[string]interface{}
	err = json.Unmarshal(rr.Body.Bytes(), &proxyResponse)
	require.NoError(t, err)
	assert.Equal(t, "123", proxyResponse["id"])
	assert.Equal(t, "created", proxyResponse["status"])

	// 7. Assert Mock Server Received Request
	require.Len(t, mockServerConfig.ReceivedRequests, 1, "Mock server should have received one request")
	receivedReq := mockServerConfig.ReceivedRequests[0]
	assert.Equal(t, http.MethodPost, receivedReq.Method)
	assert.Equal(t, "/submit", receivedReq.Path)
	assert.JSONEq(t, expectedRequestBody, string(receivedReq.Body), "Mock server received incorrect request body")

	// 8. Cleanup
	unregisterTestTool(t, service, slug.Make(registeredToolDef.Name))
}

func TestHandleToolRequest_InvalidRequestBody(t *testing.T) {
	// 1. Setup: DB, services, user, app, API key
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	service := services.NewService(db)
	notificationSvc := services.NewTestNotificationService(db)
	budgetService := services.NewBudgetService(db, notificationSvc)

	user, err := service.CreateUser(services.UserDTO{
		Email:                "testinvalidbody@example.com",
		Name:                 "testuser-invalidbody",
		Password:             "password123",
		IsAdmin:              false,
		ShowChat:             true,
		ShowPortal:           false,
		EmailVerified:        false,
		NotificationsEnabled: false,
		AccessToSSOConfig:    false,
	})
	require.NoError(t, err)

	// 2. Mock server isn't strictly needed as the request should fail before hitting the tool,
	// but we need to register a tool for the routing to pass credValidator.
	mockHttpServer, mockTeardown := newMockServer(t, &mockServerConfig{})
	defer mockTeardown()

	toolDefForApp := newTestToolDefinition(mockHttpServer.URL)
	registeredToolDef, err := service.CreateTool(
		toolDefForApp.Name,
		toolDefForApp.Description,
		toolDefForApp.ToolType,
		toolDefForApp.OASSpec,
		toolDefForApp.PrivacyScore,
		"", // Auth schema name
		"", // API key
	)
	require.NoError(t, err)
	require.NotNil(t, registeredToolDef)

	app, err := service.CreateApp("Test App InvalidBody", "App for InvalidBody testing", user.ID, []uint{}, []uint{}, []uint{registeredToolDef.ID}, nil, nil)
	require.NoError(t, err)
	// Activate the app's credential
	err = service.ActivateAppCredential(app.ID)
	require.NoError(t, err)

	// Reload the app to get the credential
	app, err = service.GetAppByID(app.ID)
	require.NoError(t, err)
	require.NotNil(t, app.Credential)
	apiKey := app.Credential.Secret

	// 3. Setup Proxy
	proxyConfig := &Config{Port: 9993}
	p := NewProxy(service, proxyConfig, budgetService)
	err = p.loadResources()
	require.NoError(t, err)
	proxyRouter := p.createHandler()

	// 4. Test Scenarios for invalid bodies
	testCases := []struct {
		name         string
		body         string
		expectedCode int
		expectedMsg  string
	}{
		{
			name:         "Malformed JSON",
			body:         `{"operation_id": "getTestData", "payload": {"key": "value"}`, // Missing closing brace
			expectedCode: http.StatusBadRequest,
			expectedMsg:  "invalid request body",
		},
		{
			name:         "Missing operation_id",
			body:         `{"payload": {"key": "value"}}`,
			expectedCode: http.StatusInternalServerError,  // From the error logs, missing operation_id causes a 500 error
			expectedMsg:  "failed to call tool operation", // Based on error logs
		},
		{
			name:         "Wrong type for operation_id",
			body:         `{"operation_id": 123, "payload": {"key": "value"}}`, // operation_id should be string
			expectedCode: http.StatusBadRequest,
			expectedMsg:  "invalid request body", // due to json unmarshal type error
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			reqPath := "/tools/" + testToolSlug
			proxyReq, err := http.NewRequest(http.MethodPost, reqPath, bytes.NewBufferString(tc.body))
			require.NoError(t, err)
			proxyReq.Header.Set("Authorization", "Bearer "+apiKey)
			proxyReq.Header.Set("Content-Type", "application/json")

			rr := httptest.NewRecorder()
			proxyRouter.ServeHTTP(rr, proxyReq)

			assert.Equal(t, tc.expectedCode, rr.Code, "Proxy returned wrong status code for: %s", tc.name)

			var errorResponse ErrorResponse
			err = json.Unmarshal(rr.Body.Bytes(), &errorResponse)
			require.NoError(t, err, "Failed to unmarshal error response for: %s", tc.name)
			assert.Contains(t, errorResponse.Message, tc.expectedMsg, "Error message mismatch for: %s", tc.name)
		})
	}

	// Test case for missing operation_id specifically, which should lead to 500 as CallToolOperation fails
	t.Run("Missing operation_id field", func(t *testing.T) {
		body := `{"payload": {"key": "value"}}` // operation_id field is completely missing
		reqPath := "/tools/" + testToolSlug
		proxyReq, err := http.NewRequest(http.MethodPost, reqPath, bytes.NewBufferString(body))
		require.NoError(t, err)
		proxyReq.Header.Set("Authorization", "Bearer "+apiKey) // apiKey is from the outer scope of TestHandleToolRequest_InvalidRequestBody
		proxyReq.Header.Set("Content-Type", "application/json")

		rr := httptest.NewRecorder()
		proxyRouter.ServeHTTP(rr, proxyReq) // proxyRouter is from outer scope

		assert.Equal(t, http.StatusInternalServerError, rr.Code, "Proxy should return 500 if operation_id field is missing")
		var errorResponse ErrorResponse
		err = json.Unmarshal(rr.Body.Bytes(), &errorResponse)
		require.NoError(t, err)
		assert.Contains(t, errorResponse.Message, "failed to call tool operation")
		// This assertion depends on the exact error message from the service layer when operation_id is missing from input to CallToolOperation.
		// Assuming it might error out earlier due to unmarshalling or a direct check.
		// If CallToolOperation is called with an empty operationID, it should return an error like "operation not found".
		assert.Contains(t, errorResponse.Error, "operation not found")
	})

	// 5. Cleanup
	unregisterTestTool(t, service, slug.Make(registeredToolDef.Name)) // service from outer scope
}

func TestHandleOAuthProtectedResourceMetadata(t *testing.T) {
	// Minimal setup for Proxy, as handler mostly uses global config
	p := &Proxy{}
	handler := http.HandlerFunc(p.handleOAuthProtectedResourceMetadata)

	// Backup and defer restore of original config values
	originalAuthServerURL := config.Get().AuthServerURL
	originalProxyOAuthMetaURL := config.Get().ProxyOAuthMetadataURL
	originalProxyURL := config.Get().ProxyURL

	defer func() {
		config.Get().AuthServerURL = originalAuthServerURL
		config.Get().ProxyOAuthMetadataURL = originalProxyOAuthMetaURL
		config.Get().ProxyURL = originalProxyURL
	}()

	// Set test config values
	config.Get().AuthServerURL = "http://auth.example.com"
	config.Get().ProxyOAuthMetadataURL = "http://proxy.example.com/.well-known/oauth-protected-resource"
	config.Get().ProxyURL = "http://proxy.example.com"

	req := httptest.NewRequest("GET", "/.well-known/oauth-protected-resource", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	require.Equal(t, "application/json; charset=utf-8", rr.Header().Get("Content-Type"))

	var metadata map[string]interface{}
	err := json.Unmarshal(rr.Body.Bytes(), &metadata)
	require.NoError(t, err)

	require.Equal(t, "http://proxy.example.com", metadata["resource"])

	authServers, ok := metadata["authorization_servers"].([]interface{})
	require.True(t, ok)
	require.Len(t, authServers, 1)
	require.Equal(t, "http://auth.example.com/.well-known/oauth-authorization-server", authServers[0])

	scopesSupported, ok := metadata["scopes_supported"].([]interface{})
	require.True(t, ok)
	require.Contains(t, scopesSupported, "mcp")

	bearerMethods, ok := metadata["bearer_methods_supported"].([]interface{})
	require.True(t, ok)
	require.Contains(t, bearerMethods, "auth_header")

	require.Equal(t, "1.0", metadata["mcp_protocol_version"])
}

func TestRespondWithError_WWWAuthenticate(t *testing.T) {
	rr := httptest.NewRecorder()

	// Backup and defer restore
	originalProxyOAuthMetaURL := config.Get().ProxyOAuthMetadataURL
	defer func() { config.Get().ProxyOAuthMetadataURL = originalProxyOAuthMetaURL }()

	config.Get().ProxyOAuthMetadataURL = "http://proxy.example.com/.well-known/oauth-protected-resource"

	respondWithError(rr, http.StatusUnauthorized, "test auth error", nil, true)

	require.Equal(t, http.StatusUnauthorized, rr.Code)
	expectedHeader := `Bearer realm="MCPResources", resource_metadata_uri="http://proxy.example.com/.well-known/oauth-protected-resource"`
	require.Equal(t, expectedHeader, rr.Header().Get("WWW-Authenticate"))

	// Test without WWW-Authenticate
	rrNoAuth := httptest.NewRecorder()
	respondWithError(rrNoAuth, http.StatusUnauthorized, "test auth error no header", nil, false)
	require.Equal(t, http.StatusUnauthorized, rrNoAuth.Code)
	require.Empty(t, rrNoAuth.Header().Get("WWW-Authenticate"))

	// Test with different status code
	rrOtherStatus := httptest.NewRecorder()
	respondWithError(rrOtherStatus, http.StatusForbidden, "test forbidden", nil, true)
	require.Equal(t, http.StatusForbidden, rrOtherStatus.Code)
	require.Empty(t, rrOtherStatus.Header().Get("WWW-Authenticate"))
}

func TestRespondWithOAIError_WWWAuthenticate(t *testing.T) {
	rr := httptest.NewRecorder()

	originalProxyOAuthMetaURL := config.Get().ProxyOAuthMetadataURL
	defer func() { config.Get().ProxyOAuthMetadataURL = originalProxyOAuthMetaURL }()

	config.Get().ProxyOAuthMetadataURL = "http://proxy.example.com/oai/.well-known/oauth-protected-resource"

	respondWithOAIError(rr, http.StatusUnauthorized, "test oai auth error", nil, true)

	require.Equal(t, http.StatusUnauthorized, rr.Code)
	expectedHeader := `Bearer realm="MCPResources", resource_metadata_uri="http://proxy.example.com/oai/.well-known/oauth-protected-resource"`
	require.Equal(t, expectedHeader, rr.Header().Get("WWW-Authenticate"))

	// Test without WWW-Authenticate
	rrNoAuth := httptest.NewRecorder()
	respondWithOAIError(rrNoAuth, http.StatusUnauthorized, "test oai auth error no header", nil, false)
	require.Equal(t, http.StatusUnauthorized, rrNoAuth.Code)
	require.Empty(t, rrNoAuth.Header().Get("WWW-Authenticate"))
}

// Note: The t.Run("Missing operation_id field", ...) block was moved back into TestHandleToolRequest_InvalidRequestBody.
// The unregisterTestTool call and the end of TestHandleToolRequest_InvalidRequestBody were also part of that move.

func TestHandleToolRequest_ToolNotFound(t *testing.T) {
	// 1. Setup: DB, services, user, app, API key
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	service := services.NewService(db)
	notificationSvc := services.NewTestNotificationService(db)
	budgetService := services.NewBudgetService(db, notificationSvc)

	// First create a tool to create a valid credential
	mockHttpServer, mockTeardown := newMockServer(t, &mockServerConfig{})
	defer mockTeardown()
	toolDef := newTestToolDefinition(mockHttpServer.URL)
	registeredTool, err := service.CreateTool(
		toolDef.Name,
		toolDef.Description,
		toolDef.ToolType,
		toolDef.OASSpec,
		toolDef.PrivacyScore,
		"", // Auth schema name
		"", // API key
	)
	require.NoError(t, err)

	user, err := service.CreateUser(services.UserDTO{
		Email:                "testnotfound@example.com",
		Name:                 "testuser-notfound",
		Password:             "password123",
		IsAdmin:              false,
		ShowChat:             true,
		ShowPortal:           false,
		EmailVerified:        false,
		NotificationsEnabled: false,
		AccessToSSOConfig:    false,
	})
	require.NoError(t, err)
	app, err := service.CreateApp("Test App NotFound", "App for NotFound testing", user.ID, []uint{}, []uint{}, []uint{registeredTool.ID}, nil, nil) // Associate with valid tool
	require.NoError(t, err)
	// Activate the app's credential
	err = service.ActivateAppCredential(app.ID)
	require.NoError(t, err)

	// Reload the app to get the credential
	app, err = service.GetAppByID(app.ID)
	require.NoError(t, err)
	require.NotNil(t, app.Credential)
	apiKey := app.Credential.Secret

	// Delete the tool to set up the not found scenario but keep the app credential valid
	err = service.DeleteTool(registeredTool.ID)
	require.NoError(t, err)

	// 2. Setup Proxy (no mock server or tool registration needed for this test)
	proxyConfig := &Config{Port: 9996}
	p := NewProxy(service, proxyConfig, budgetService)
	err = p.loadResources() // Load resources (which will be empty for tools)
	require.NoError(t, err)
	proxyRouter := p.createHandler()

	// 3. Prepare Proxy Request to a non-existent tool slug
	requestBody := map[string]interface{}{
		"operation_id": "any-operation",
	}
	jsonBody, err := json.Marshal(requestBody)
	require.NoError(t, err)

	reqPath := "/tools/non-existent-tool"
	proxyReq, err := http.NewRequest(http.MethodPost, reqPath, bytes.NewBuffer(jsonBody))
	require.NoError(t, err)
	proxyReq.Header.Set("Authorization", "Bearer "+apiKey) // Valid API key is needed to pass auth
	proxyReq.Header.Set("Content-Type", "application/json")

	// 4. Perform Request & Assert Response
	rr := httptest.NewRecorder()
	proxyRouter.ServeHTTP(rr, proxyReq)

	// With a valid credential but non-existent tool slug, we expect 401
	// This is because the credential is valid but the tool is not found, which returns 401
	assert.Equal(t, http.StatusUnauthorized, rr.Code, "Proxy should return 401 for a non-existent tool slug with valid credential")

	var errorResponse ErrorResponse
	err = json.Unmarshal(rr.Body.Bytes(), &errorResponse)
	require.NoError(t, err, "Failed to unmarshal error response")
	assert.Contains(t, errorResponse.Message, "invalid credential", "Error message should indicate invalid credential for non-existent tool")
}

func TestHandleToolRequest_OperationNotFound(t *testing.T) {
	// 1. Setup: DB, services, user, app, API key
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	service := services.NewService(db)
	notificationSvc := services.NewTestNotificationService(db)
	budgetService := services.NewBudgetService(db, notificationSvc)

	user, err := service.CreateUser(services.UserDTO{
		Email:                "testopnotfound@example.com",
		Name:                 "testuser-opnotfound",
		Password:             "password123",
		IsAdmin:              false,
		ShowChat:             true,
		ShowPortal:           false,
		EmailVerified:        false,
		NotificationsEnabled: false,
		AccessToSSOConfig:    false,
	})
	require.NoError(t, err)

	// 2. Setup Mock Server (although it won't be called in this case)
	mockServerConfig := &mockServerConfig{
		ResponseStatus: http.StatusOK,
		ResponseBody:   `{"result": "This won't be returned"}`,
	}
	mockHttpServer, mockTeardown := newMockServer(t, mockServerConfig)
	defer mockTeardown()

	// 3. Register Test Tool and associate with App
	toolDefForApp := newTestToolDefinition(mockHttpServer.URL)
	registeredToolDef, err := service.CreateTool(
		toolDefForApp.Name,
		toolDefForApp.Description,
		toolDefForApp.ToolType,
		toolDefForApp.OASSpec,
		toolDefForApp.PrivacyScore,
		"", // Auth schema name
		"", // API key
	)
	require.NoError(t, err)
	require.NotNil(t, registeredToolDef)

	app, err := service.CreateApp("Test App OpNotFound", "App for OpNotFound testing", user.ID, []uint{}, []uint{}, []uint{registeredToolDef.ID}, nil, nil)
	require.NoError(t, err)
	// Activate the app's credential
	err = service.ActivateAppCredential(app.ID)
	require.NoError(t, err)

	// Reload the app to get the credential
	app, err = service.GetAppByID(app.ID)
	require.NoError(t, err)
	require.NotNil(t, app.Credential)
	apiKey := app.Credential.Secret

	// 4. Setup Proxy
	proxyConfig := &Config{Port: 9995}
	p := NewProxy(service, proxyConfig, budgetService)
	err = p.loadResources()
	require.NoError(t, err)
	proxyRouter := p.createHandler()

	// 5. Prepare Proxy Request with a non-existent operation_id
	requestBody := map[string]interface{}{
		"operation_id": "non-existent-operation",
		"parameters":   map[string][]string{},
		"payload":      nil,
		"headers":      map[string][]string{},
	}
	jsonBody, err := json.Marshal(requestBody)
	require.NoError(t, err)

	reqPath := "/tools/" + testToolSlug
	proxyReq, err := http.NewRequest(http.MethodPost, reqPath, bytes.NewBuffer(jsonBody))
	require.NoError(t, err)
	proxyReq.Header.Set("Authorization", "Bearer "+apiKey)
	proxyReq.Header.Set("Content-Type", "application/json")

	// 6. Perform Request & Assert Response
	rr := httptest.NewRecorder()
	proxyRouter.ServeHTTP(rr, proxyReq)

	assert.Equal(t, http.StatusInternalServerError, rr.Code, "HTTP status code should be 500 Internal Server Error")

	var errorResponse ErrorResponse
	err = json.Unmarshal(rr.Body.Bytes(), &errorResponse)
	require.NoError(t, err, "Should be able to unmarshal error response")
	// Check key error fields
	assert.Contains(t, errorResponse.Message, "failed to call tool operation", "Error message mismatch")
	require.NotEmpty(t, errorResponse.Error, "Detailed error should be present")
	assert.Contains(t, errorResponse.Error, "operation not found", "Detailed error should mention operation not found")

	// 7. Cleanup
	unregisterTestTool(t, service, slug.Make(registeredToolDef.Name))
}

func TestHandleToolRequest_BackendServerError(t *testing.T) {
	// 1. Setup: DB, services, user, app, API key
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	service := services.NewService(db)
	notificationSvc := services.NewTestNotificationService(db)
	budgetService := services.NewBudgetService(db, notificationSvc)

	user, err := service.CreateUser(services.UserDTO{
		Email:                "testuser-servererror",
		Name:                 "testservererror@example.com",
		Password:             "password123",
		IsAdmin:              false,
		ShowChat:             false,
		ShowPortal:           false,
		EmailVerified:        false,
		NotificationsEnabled: false,
		AccessToSSOConfig:    false,
	})
	require.NoError(t, err)

	// 2. Setup Mock Server to return 500
	mockServerConfig := &mockServerConfig{
		ResponseStatus: http.StatusInternalServerError,
		ResponseBody:   `{"error": "internal mock server error"}`,
		ExpectedMethod: http.MethodGet, // Could be any method defined in the tool
		ExpectedPath:   "/test",
	}
	mockHttpServer, mockTeardown := newMockServer(t, mockServerConfig)
	defer mockTeardown()

	// 3. Register Test Tool and associate with App
	toolDefForApp := newTestToolDefinition(mockHttpServer.URL)
	registeredToolDef, err := service.CreateTool(
		toolDefForApp.Name,
		toolDefForApp.Description,
		toolDefForApp.ToolType,
		toolDefForApp.OASSpec,
		toolDefForApp.PrivacyScore,
		"", // Auth schema name
		"", // API key
	)
	require.NoError(t, err)
	require.NotNil(t, registeredToolDef)

	app, err := service.CreateApp("Test App ServerError", "App for ServerError testing", user.ID, []uint{}, []uint{}, []uint{registeredToolDef.ID}, nil, nil)
	require.NoError(t, err)
	// Activate the app's credential
	err = service.ActivateAppCredential(app.ID)
	require.NoError(t, err)

	// Reload the app to get the credential
	app, err = service.GetAppByID(app.ID)
	require.NoError(t, err)
	require.NotNil(t, app.Credential)
	apiKey := app.Credential.Secret

	// 4. Setup Proxy
	proxyConfig := &Config{Port: 9994}
	p := NewProxy(service, proxyConfig, budgetService)
	err = p.loadResources()
	require.NoError(t, err)
	proxyRouter := p.createHandler()

	// 5. Prepare Proxy Request
	requestBody := map[string]interface{}{
		"operation_id": "getTestData", // Target the GET /test operation
		"parameters":   map[string][]string{},
		"payload":      nil,
		"headers":      map[string][]string{},
	}
	jsonBody, err := json.Marshal(requestBody)
	require.NoError(t, err)

	reqPath := "/tools/" + testToolSlug
	proxyReq, err := http.NewRequest(http.MethodPost, reqPath, bytes.NewBuffer(jsonBody))
	require.NoError(t, err)
	proxyReq.Header.Set("Authorization", "Bearer "+apiKey)
	proxyReq.Header.Set("Content-Type", "application/json")

	// 6. Perform Request & Assert Response
	rr := httptest.NewRecorder()
	proxyRouter.ServeHTTP(rr, proxyReq)

	// When the backend tool (mock server) returns a 500,
	// service.CallToolOperation propagates this error.
	// The handleToolRequest then wraps it in its own 500 error response.
	assert.Equal(t, http.StatusInternalServerError, rr.Code, "Proxy should return 500 if backend server fails")

	var errorResponse ErrorResponse
	err = json.Unmarshal(rr.Body.Bytes(), &errorResponse)
	require.NoError(t, err, "Failed to unmarshal error response")
	assert.Contains(t, errorResponse.Message, "failed to call tool operation", "Error message should indicate tool operation call failed")
	assert.Contains(t, errorResponse.Error, "request failed with status code", "Error should mention status code")
	// The universalclient.Client.CallOperation method returns an error that includes the status code from the server
	assert.Contains(t, errorResponse.Error, "error: 500", "Error message should contain the 500 status code")

	// 7. Assert Mock Server Received Request (even though it failed)
	require.Len(t, mockServerConfig.ReceivedRequests, 1, "Mock server should have received one request")
	receivedReq := mockServerConfig.ReceivedRequests[0]
	assert.Equal(t, http.MethodGet, receivedReq.Method)
	assert.Equal(t, "/test", receivedReq.Path)

	// 8. Cleanup
	unregisterTestTool(t, service, slug.Make(registeredToolDef.Name))
}
