package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv" // Added strconv
	"testing"
	"time"

	apitest "github.com/TykTechnologies/midsommar/v2/api/testing"
	"github.com/TykTechnologies/midsommar/v2/auth"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestAPIForCommonTests(t *testing.T) (*API, *gorm.DB, *services.Service) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	err = models.InitModels(db)
	assert.NoError(t, err)

	service := services.NewService(db)

	config := auth.Config{
		DB:                  db,
		Service:             service,
		CookieName:          "session",
		CookieSecure:        true,
		CookieHTTPOnly:      true,
		CookieSameSite:      http.SameSiteStrictMode,
		ResetTokenExpiry:    time.Hour,
		FrontendURL:         "http://example.com",
		RegistrationAllowed: true,
		AdminEmail:          "admin@example.com",
		TestMode:            true,
	}

	notificationService := services.NewTestNotificationService(db)
	authService := auth.NewAuthService(&config, nil, service, notificationService)
	licenser := apitest.SetupTestLicenser()
	api := NewAPI(service, true, authService, &config, nil, emptyFile, licenser)

	return api, db, service
}

func TestCommon_TestGetCatalogueLLMs(t *testing.T) {
	api, _, _ := setupTestAPIForCommonTests(t)

	// Create a test user and catalogue
	user := createTestUser(t, api.service)
	catalogue := createTestCatalogue(t, api.service)

	// Add the catalogue to the user's accessible catalogues
	addCatalogueToUserGroup(t, api.service, user.ID, catalogue.ID)

	// Create test LLMs and add them to the catalogue
	llm1 := createTestLLM(t, api.service, "LLM1")
	llm2 := createTestLLM(t, api.service, "LLM2")
	api.service.AddLLMToCatalogue(llm1.ID, catalogue.ID)
	api.service.AddLLMToCatalogue(llm2.ID, catalogue.ID)

	// Set up the request
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("user", user)
	c.Params = []gin.Param{{Key: "id", Value: fmt.Sprintf("%d", catalogue.ID)}}

	// Call the handler
	api.getCatalogueLLMs(c)

	// Assert the response
	assert.Equal(t, http.StatusOK, w.Code)

	var response []LLMResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Len(t, response, 2)
	assert.ElementsMatch(t, []string{"LLM1", "LLM2"}, []string{response[0].Attributes.Name, response[1].Attributes.Name})
}

func TestCommon_TestGetDataCatalogueDatasources(t *testing.T) {
	api, _, _ := setupTestAPIForCommonTests(t)

	// Create a test user and data catalogue
	user := createTestUser(t, api.service)
	dataCatalogue := createTestDataCatalogue(t, api.service)

	// Add the data catalogue to the user's accessible data catalogues
	addDataCatalogueToUserGroup(t, api.service, user.ID, dataCatalogue.ID)

	// Create test datasources and add them to the data catalogue
	ds1 := createTestDatasource(t, api.service, "Datasource1")
	ds2 := createTestDatasource(t, api.service, "Datasource2")
	api.service.AddDatasourceToDataCatalogue(dataCatalogue.ID, ds1.ID)
	api.service.AddDatasourceToDataCatalogue(dataCatalogue.ID, ds2.ID)

	// Set up the request
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("user", user)
	c.Params = []gin.Param{{Key: "id", Value: fmt.Sprintf("%d", dataCatalogue.ID)}}

	// Call the handler
	api.getDataCatalogueDatasources(c)

	// Assert the response
	assert.Equal(t, http.StatusOK, w.Code)
	if w.Code != http.StatusOK {
		t.Log(w.Body.String())
	}

	var response []DatasourceResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Len(t, response, 2)
	if len(response) == 2 {
		assert.ElementsMatch(t, []string{"Datasource1", "Datasource2"}, []string{response[0].Attributes.Name, response[1].Attributes.Name})
	}

}

func TestCommon_TestGetToolCatalogueTools(t *testing.T) {
	api, _, _ := setupTestAPIForCommonTests(t)

	// Create a test user and tool catalogue
	user := createTestUser(t, api.service)
	toolCatalogue := createTestToolCatalogue(t, api.service)

	// Add the tool catalogue to the user's accessible tool catalogues
	addToolCatalogueToUserGroup(t, api.service, user.ID, toolCatalogue.ID)

	// Create test tools and add them to the tool catalogue
	tool1 := createTestTool(t, api.service, "Tool1")
	tool2 := createTestTool(t, api.service, "Tool2")
	err := api.service.AddToolToToolCatalogue(tool1.ID, toolCatalogue.ID)
	assert.NoError(t, err)

	err = api.service.AddToolToToolCatalogue(tool2.ID, toolCatalogue.ID)
	assert.NoError(t, err)

	// Set up the request
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("user", user)
	c.Params = []gin.Param{{Key: "id", Value: fmt.Sprintf("%d", toolCatalogue.ID)}}

	// Call the handler
	api.getToolCatalogueTools(c)

	// Assert the response
	assert.Equal(t, http.StatusOK, w.Code)

	var response []ToolResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Len(t, response, 2)
	if len(response) == 2 {
		assert.ElementsMatch(t, []string{"Tool1", "Tool2"}, []string{response[0].Attributes.Name, response[1].Attributes.Name})
	}
}

func TestCommon_TestGetUserChatHistoryRecords(t *testing.T) {
	api, db, _ := setupTestAPIForCommonTests(t)

	// Create a test user
	user := createTestUser(t, api.service)

	// Create test chat history records for the user
	c1 := createTestChatHistoryRecord(t, api.service, user.ID, "Session1")
	for j := 1; j <= 5; j++ {
		message := &models.CMessage{
			Session:   c1.SessionID,
			Content:   []byte("Test Message"),
			ChatID:    c1.ChatID,
			CreatedAt: time.Now(),
		}
		err := db.Create(message).Error
		assert.NoError(t, err)
	}

	c2 := createTestChatHistoryRecord(t, api.service, user.ID, "Session2")
	for j := 1; j <= 5; j++ {
		message := &models.CMessage{
			Session:   c2.SessionID,
			Content:   []byte("Test Message"),
			ChatID:    c2.ChatID,
			CreatedAt: time.Now(),
		}
		err := db.Create(message).Error
		assert.NoError(t, err)
	}

	// Set up the request
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("user", user)
	c.Params = []gin.Param{{Key: "user_id", Value: fmt.Sprintf("%d", user.ID)}}

	// Call the handler
	api.getUserChatHistoryRecords(c)

	// Assert the response
	assert.Equal(t, http.StatusOK, w.Code)

	var response []ChatHistoryRecordResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Len(t, response, 2)
	if len(response) == 2 {
		assert.ElementsMatch(t, []string{"Session1", "Session2"}, []string{response[0].Attributes.SessionID, response[1].Attributes.SessionID})
	}
}

// Helper functions for creating test data

func createTestUser(t *testing.T, service *services.Service) *models.User {
	user, err := service.CreateUser("test@example.com", "Test User", "password", false, true, true, true, false, false)
	assert.NoError(t, err)
	return user
}

// Helper to create a test user with custom settings
func createTestUserWithSettings(t *testing.T, service *services.Service, email, name string, isAdmin, showPortal, showChat, emailVerified, notificationsEnabled bool) *models.User {
	user, err := service.CreateUser(email, name, "password", isAdmin, showPortal, showChat, emailVerified, notificationsEnabled, isAdmin && notificationsEnabled)
	assert.NoError(t, err)
	return user
}

func createTestCatalogue(t *testing.T, service *services.Service) *models.Catalogue {
	catalogue, err := service.CreateCatalogue("Test Catalogue")
	assert.NoError(t, err)
	return catalogue
}

func createTestDataCatalogue(t *testing.T, service *services.Service) *models.DataCatalogue {
	dataCatalogue, err := service.CreateDataCatalogue("Test Data Catalogue", "Short Desc", "Long Desc", "icon.png")
	assert.NoError(t, err)
	return dataCatalogue
}

func createTestToolCatalogue(t *testing.T, service *services.Service) *models.ToolCatalogue {
	toolCatalogue, err := service.CreateToolCatalogue("Test Tool Catalogue", "Short Desc", "Long Desc", "icon.png")
	assert.NoError(t, err)
	return toolCatalogue
}

func createTestLLM(t *testing.T, service *services.Service, name string) *models.LLM {
	llm, err := service.CreateLLM(name, "api_key", "https://api.example.com",
		80, "Short desc", "Long desc", "https://logo.example.com", models.OPENAI, true, nil, "", []string{}, nil, nil)
	assert.NoError(t, err)
	return llm
}

func createTestDatasource(t *testing.T, service *services.Service, name string) *models.Datasource {
	datasource, err := service.CreateDatasource(name, "Short desc", "Long desc", "icon.png", "https://example.com", 75, 1, []string{}, "conn_string", "source_type", "api_key", "dbname", "embed_vendor", "embed_url", "embed_api_key", "embed_model", true)
	assert.NoError(t, err)
	return datasource
}

func createTestTool(t *testing.T, service *services.Service, name string) *models.Tool {
	tool, err := service.CreateTool(name, "Description", models.ToolTypeREST, "OAS Spec", 8, "apiKey", "secret")
	assert.NoError(t, err)
	return tool
}

func createTestChatHistoryRecord(t *testing.T, service *services.Service, userID uint, sessionID string) *models.ChatHistoryRecord {
	record, err := service.CreateChatHistoryRecord(sessionID, 1, userID, "Test Chat")
	assert.NoError(t, err)
	return record
}

func addCatalogueToUserGroup(t *testing.T, service *services.Service, userID, catalogueID uint) {
	group, err := service.CreateGroup("Test Group", []uint{}, []uint{}, []uint{}, []uint{})
	assert.NoError(t, err)
	err = service.AddUserToGroup(userID, group.ID)
	assert.NoError(t, err)
	err = service.AddCatalogueToGroup(catalogueID, group.ID)
	assert.NoError(t, err)
}

func addDataCatalogueToUserGroup(t *testing.T, service *services.Service, userID, dataCatalogueID uint) {
	group, err := service.CreateGroup("Test Group", []uint{}, []uint{}, []uint{}, []uint{})
	assert.NoError(t, err)
	err = service.AddUserToGroup(userID, group.ID)
	assert.NoError(t, err)
	err = service.AddDataCatalogueToGroup(dataCatalogueID, group.ID)
	assert.NoError(t, err)
}

func addToolCatalogueToUserGroup(t *testing.T, service *services.Service, userID, toolCatalogueID uint) {
	group, err := service.CreateGroup("Test Group", []uint{}, []uint{}, []uint{}, []uint{})
	assert.NoError(t, err)
	err = service.AddUserToGroup(userID, group.ID)
	assert.NoError(t, err)
	err = service.AddToolCatalogueToGroup(toolCatalogueID, group.ID)
	assert.NoError(t, err)
}

func TestCommon_CreateUserAppWithTools(t *testing.T) {
	api, db, service := setupTestAPIForCommonTests(t)

	// Create a non-admin user
	user := createTestUserWithSettings(t, service, "testuser@example.com", "Test User", false, true, true, true, false)

	// Create tools
	toolA := createTestTool(t, service, "ToolA")
	toolB := createTestTool(t, service, "ToolB")

	// Create a tool catalogue
	toolCatalogue := createTestToolCatalogue(t, service)

	var err error // Define err for the test scope

	// Add tools to the catalogue
	err = service.AddToolToToolCatalogue(toolA.ID, toolCatalogue.ID)
	assert.NoError(t, err)
	err = service.AddToolToToolCatalogue(toolB.ID, toolCatalogue.ID)
	assert.NoError(t, err)

	// Create LLM and catalogue
	llm1 := createTestLLM(t, service, "LLMForApp")
	llmCatalogueForAppList := createTestCatalogue(t, service)
	err = service.AddLLMToCatalogue(llm1.ID, llmCatalogueForAppList.ID)
	assert.NoError(t, err)

	// Create Datasource and data catalogue
	ds1 := createTestDatasource(t, service, "DSForApp")
	dataCatalogueForAppList := createTestDataCatalogue(t, service) // Renamed dataCatalogue
	err = service.AddDatasourceToDataCatalogue(dataCatalogueForAppList.ID, ds1.ID)
	assert.NoError(t, err)

	// Create a group and add the user and catalogues to it
	_, err = service.CreateGroup("UserGroup",
		[]uint{user.ID},
		[]uint{llmCatalogueForAppList.ID},
		[]uint{dataCatalogueForAppList.ID},
		[]uint{toolCatalogue.ID})
	assert.NoError(t, err)

	// Prepare request payload
	createAppReq := CreateAppRequest{
		Name:          "AppWithTools",
		Description:   "An app created with tools",
		ToolIDs:       []uint{toolA.ID},
		LLMIDs:        []uint{llm1.ID},
		DataSourceIDs: []uint{ds1.ID},
	}
	payloadBytes, err := json.Marshal(createAppReq)
	assert.NoError(t, err)

	// Perform POST request
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/common/apps", bytes.NewBuffer(payloadBytes))
	req.Header.Set("Content-Type", "application/json")
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Set("user", user) // Set the authenticated user

	api.createUserApp(c) // Assuming createUserApp is the handler for POST /common/apps

	// Assert response
	assert.Equal(t, http.StatusCreated, w.Code, "Expected status 201 Created, got %d. Response: %s", w.Code, w.Body.String())

	var appResp AppResponse
	err = json.Unmarshal(w.Body.Bytes(), &appResp)
	assert.NoError(t, err)
	assert.Equal(t, "AppWithTools", appResp.Attributes.Name)

	// Log API response ToolIDs
	t.Logf("API Response App ID: %s, ToolIDs: %v", appResp.ID, appResp.Attributes.ToolIDs)

	// Verify in DB
	var fetchedApp models.App
	appIDUint, _ := strconv.ParseUint(appResp.ID, 10, 64)
	err = db.Preload("Tools").First(&fetchedApp, uint(appIDUint)).Error
	assert.NoError(t, err)

	// Log DB fetched Tools
	dbToolIDs := make([]uint, len(fetchedApp.Tools))
	for i, tool := range fetchedApp.Tools {
		dbToolIDs[i] = tool.ID
	}
	t.Logf("DB Fetched App ID: %d, Tools: %v (IDs: %v)", fetchedApp.ID, fetchedApp.Tools, dbToolIDs)

	// Assertions
	assert.Contains(t, appResp.Attributes.ToolIDs, toolA.ID, "ToolID from API response should contain toolA.ID")
	assert.Len(t, appResp.Attributes.ToolIDs, 1, "API response ToolIDs length should be 1")

	assert.True(t, len(fetchedApp.Tools) > 0, "Fetched app from DB should have tools")
	assert.Len(t, fetchedApp.Tools, 1, "Fetched app from DB should have 1 tool")
	assert.Contains(t, dbToolIDs, toolA.ID, "ToolID from DB fetch should contain toolA.ID")
	assert.Equal(t, toolA.ID, fetchedApp.Tools[0].ID, "First tool ID from DB fetch should be toolA.ID")

	assert.Equal(t, "AppWithTools", fetchedApp.Name)
}

func TestCommon_GetUserAccessibleTools(t *testing.T) {
	api, _, service := setupTestAPIForCommonTests(t)

	// Create users
	user1 := createTestUserWithSettings(t, service, "user1@example.com", "User One", false, true, true, true, false)
	user2 := createTestUserWithSettings(t, service, "user2@example.com", "User Two", false, true, true, true, false)
	user3 := createTestUserWithSettings(t, service, "user3@example.com", "User Three", false, true, true, true, false)

	// Create tools
	tool1 := createTestTool(t, service, "Tool1")
	tool2 := createTestTool(t, service, "Tool2")
	tool3 := createTestTool(t, service, "Tool3")

	// Create tool catalogues
	catalogueA := createTestToolCatalogue(t, service)
	catalogueB := createTestToolCatalogue(t, service)

	// Add tools to catalogues
	assert.NoError(t, service.AddToolToToolCatalogue(tool1.ID, catalogueA.ID))
	assert.NoError(t, service.AddToolToToolCatalogue(tool2.ID, catalogueA.ID))
	assert.NoError(t, service.AddToolToToolCatalogue(tool3.ID, catalogueB.ID))

	// Create groups
	var err error // Define err once for the scope
	_, err = service.CreateGroup("GroupA", []uint{user1.ID}, []uint{}, []uint{}, []uint{catalogueA.ID})
	assert.NoError(t, err)
	_, err = service.CreateGroup("GroupB", []uint{user2.ID}, []uint{}, []uint{}, []uint{catalogueB.ID})
	assert.NoError(t, err)

	// Associate catalogues with groups - already done with CreateGroup

	// Scenario 1: User1 access
	w1 := httptest.NewRecorder()
	req1, _ := http.NewRequest("GET", "/common/accessible-tools", nil)
	c1, _ := gin.CreateTestContext(w1)
	c1.Request = req1
	c1.Set("user", user1)
	api.getUserAccessibleTools(c1) // Assuming getAccessibleTools is the handler

	assert.Equal(t, http.StatusOK, w1.Code)
	var toolsUser1 []ToolResponse
	err = json.Unmarshal(w1.Body.Bytes(), &toolsUser1)
	assert.NoError(t, err)
	assert.Len(t, toolsUser1, 2)
	tool1Found := false
	tool2Found := false
	for _, tool := range toolsUser1 {
		if tool.ID == fmt.Sprintf("%d", tool1.ID) && tool.Attributes.Name == tool1.Name {
			tool1Found = true
		}
		if tool.ID == fmt.Sprintf("%d", tool2.ID) && tool.Attributes.Name == tool2.Name {
			tool2Found = true
		}
	}
	assert.True(t, tool1Found, "Tool1 not found for user1")
	assert.True(t, tool2Found, "Tool2 not found for user1")

	// Scenario 2: User2 access
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("GET", "/common/accessible-tools", nil)
	c2, _ := gin.CreateTestContext(w2)
	c2.Request = req2
	c2.Set("user", user2)
	api.getUserAccessibleTools(c2)

	assert.Equal(t, http.StatusOK, w2.Code)
	var toolsUser2 []ToolResponse
	err = json.Unmarshal(w2.Body.Bytes(), &toolsUser2)
	assert.NoError(t, err)
	assert.Len(t, toolsUser2, 1)
	assert.Equal(t, fmt.Sprintf("%d", tool3.ID), toolsUser2[0].ID)
	assert.Equal(t, tool3.Name, toolsUser2[0].Attributes.Name)

	// Scenario 3: User3 access (no tool catalogue)
	w3 := httptest.NewRecorder()
	req3, _ := http.NewRequest("GET", "/common/accessible-tools", nil)
	c3, _ := gin.CreateTestContext(w3)
	c3.Request = req3
	c3.Set("user", user3)
	api.getUserAccessibleTools(c3)

	assert.Equal(t, http.StatusOK, w3.Code)
	var toolsUser3 []ToolResponse
	err = json.Unmarshal(w3.Body.Bytes(), &toolsUser3)
	assert.NoError(t, err)
	assert.Len(t, toolsUser3, 0)
}

func TestCommon_GetUserAppsWithTools(t *testing.T) {
	api, _, service := setupTestAPIForCommonTests(t)

	user := createTestUserWithSettings(t, service, "appuser@example.com", "App User", false, true, true, true, false)
	toolA := createTestTool(t, service, "ToolForAppList")

	// LLM and Datasource setup for app creation
	llmForAppList := createTestLLM(t, service, "LLMForAppList")
	dsForAppList := createTestDatasource(t, service, "DSForAppList")
	llmCatalogueForAppList_apps := createTestCatalogue(t, service)
	dataCatalogueForAppList_apps := createTestDataCatalogue(t, service)
	var err error // Define err for this test scope
	err = service.AddLLMToCatalogue(llmForAppList.ID, llmCatalogueForAppList_apps.ID)
	assert.NoError(t, err)
	err = service.AddDatasourceToDataCatalogue(dataCatalogueForAppList_apps.ID, dsForAppList.ID)
	assert.NoError(t, err)

	toolCatalogue_apps := createTestToolCatalogue(t, service)
	err = service.AddToolToToolCatalogue(toolA.ID, toolCatalogue_apps.ID)
	assert.NoError(t, err)

	_, err = service.CreateGroup("AppUserGroup",
		[]uint{user.ID},
		[]uint{llmCatalogueForAppList_apps.ID},  // Corrected variable
		[]uint{dataCatalogueForAppList_apps.ID}, // Corrected variable
		[]uint{toolCatalogue_apps.ID})           // Corrected variable
	assert.NoError(t, err)

	// Create an app with ToolA for the user
	appPayload := CreateAppRequest{
		Name:          "AppInList",
		Description:   "Test app for list",
		ToolIDs:       []uint{toolA.ID},
		LLMIDs:        []uint{llmForAppList.ID},
		DataSourceIDs: []uint{dsForAppList.ID},
	}
	appPayloadBytes, _ := json.Marshal(appPayload)
	wCreate := httptest.NewRecorder()
	reqCreate, _ := http.NewRequest("POST", "/common/apps", bytes.NewBuffer(appPayloadBytes))
	reqCreate.Header.Set("Content-Type", "application/json")
	cCreate, _ := gin.CreateTestContext(wCreate)
	cCreate.Request = reqCreate
	cCreate.Set("user", user)
	api.createUserApp(cCreate)
	assert.Equal(t, http.StatusCreated, wCreate.Code)
	var createdAppResp AppResponse
	err = json.Unmarshal(wCreate.Body.Bytes(), &createdAppResp)
	assert.NoError(t, err)

	// Perform GET request for /common/apps
	wList := httptest.NewRecorder()
	reqList, _ := http.NewRequest("GET", "/common/apps", nil)
	cList, _ := gin.CreateTestContext(wList)
	cList.Request = reqList
	cList.Set("user", user)
	api.getUserApps(cList) // Assuming getUserApps is the handler

	assert.Equal(t, http.StatusOK, wList.Code)
	var appListResp AppListResponse
	err = json.Unmarshal(wList.Body.Bytes(), &appListResp)
	assert.NoError(t, err)
	assert.True(t, len(appListResp.Data) > 0, "App list should not be empty")

	foundApp := false
	for _, app := range appListResp.Data {
		if app.ID == createdAppResp.ID {
			foundApp = true
			assert.Contains(t, app.Attributes.ToolIDs, toolA.ID)
			assert.Len(t, app.Attributes.ToolIDs, 1)
			break
		}
	}
	assert.True(t, foundApp, "Created app not found in list")
}

func TestCommon_GetUserAppDetailsWithTools(t *testing.T) {
	api, _, service := setupTestAPIForCommonTests(t)

	user := createTestUserWithSettings(t, service, "detailuser@example.com", "Detail User", false, true, true, true, false)
	toolB := createTestTool(t, service, "ToolForAppDetail")

	// LLM and Datasource setup for app creation
	llmForAppDetail := createTestLLM(t, service, "LLMForAppDetail")
	dsForAppDetail := createTestDatasource(t, service, "DSForAppDetail")
	llmCatalogueForAppDetail_details := createTestCatalogue(t, service)
	dataCatalogueForAppDetail_details := createTestDataCatalogue(t, service)
	var err error // Define err for this test scope
	err = service.AddLLMToCatalogue(llmForAppDetail.ID, llmCatalogueForAppDetail_details.ID)
	assert.NoError(t, err)
	err = service.AddDatasourceToDataCatalogue(dataCatalogueForAppDetail_details.ID, dsForAppDetail.ID)
	assert.NoError(t, err)

	toolCatalogue_details := createTestToolCatalogue(t, service)
	err = service.AddToolToToolCatalogue(toolB.ID, toolCatalogue_details.ID)
	assert.NoError(t, err)

	_, err = service.CreateGroup("DetailUserGroup",
		[]uint{user.ID},
		[]uint{llmCatalogueForAppDetail_details.ID},  // Corrected variable
		[]uint{dataCatalogueForAppDetail_details.ID}, // Corrected variable
		[]uint{toolCatalogue_details.ID})             // Corrected variable
	assert.NoError(t, err)

	// Create an app with ToolB for the user
	appPayload := CreateAppRequest{
		Name:          "AppForDetail",
		Description:   "Test app for detail view",
		ToolIDs:       []uint{toolB.ID},
		LLMIDs:        []uint{llmForAppDetail.ID},
		DataSourceIDs: []uint{dsForAppDetail.ID},
	}
	appPayloadBytes, _ := json.Marshal(appPayload)
	wCreate := httptest.NewRecorder()
	reqCreate, _ := http.NewRequest("POST", "/common/apps", bytes.NewBuffer(appPayloadBytes))
	reqCreate.Header.Set("Content-Type", "application/json")
	cCreate, _ := gin.CreateTestContext(wCreate)
	cCreate.Request = reqCreate
	cCreate.Set("user", user)
	api.createUserApp(cCreate)
	assert.Equal(t, http.StatusCreated, wCreate.Code)
	var createdAppResp AppResponse
	err = json.Unmarshal(wCreate.Body.Bytes(), &createdAppResp)
	assert.NoError(t, err)

	// Perform GET request for /common/apps/{id}
	wDetail := httptest.NewRecorder()
	reqDetail, _ := http.NewRequest("GET", fmt.Sprintf("/common/apps/%s", createdAppResp.ID), nil)
	cDetail, _ := gin.CreateTestContext(wDetail)
	cDetail.Request = reqDetail
	cDetail.Set("user", user)
	cDetail.Params = gin.Params{gin.Param{Key: "id", Value: createdAppResp.ID}}
	api.getUserAppDetails(cDetail) // Assuming getUserAppDetails is the handler

	assert.Equal(t, http.StatusOK, wDetail.Code, "Expected status 200 OK, got %d. Response: %s", wDetail.Code, wDetail.Body.String())
	var appDetailResp AppDetailResponse
	err = json.Unmarshal(wDetail.Body.Bytes(), &appDetailResp)
	assert.NoError(t, err)
	assert.Equal(t, createdAppResp.ID, appDetailResp.ID)
	assert.Contains(t, appDetailResp.Attributes.ToolIDs, toolB.ID)
	assert.Len(t, appDetailResp.Attributes.ToolIDs, 1)
}
