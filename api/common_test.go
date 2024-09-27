package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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

	authService := auth.NewAuthService(&config, newMockMailer())
	api := NewAPI(service, true, authService, nil)

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
	api, _, _ := setupTestAPIForCommonTests(t)

	// Create a test user
	user := createTestUser(t, api.service)

	// Create test chat history records for the user
	createTestChatHistoryRecord(t, api.service, user.ID, "Session1")
	createTestChatHistoryRecord(t, api.service, user.ID, "Session2")

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
	assert.ElementsMatch(t, []string{"Session1", "Session2"}, []string{response[0].Attributes.SessionID, response[1].Attributes.SessionID})
}

// Helper functions for creating test data

func createTestUser(t *testing.T, service *services.Service) *models.User {
	user, err := service.CreateUser("test@example.com", "Test User", "password", false)
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
	llm, err := service.CreateLLM(name, "api_key", "https://api.example.com", 80, "Short desc", "Long desc", "https://logo.example.com", models.OPENAI, true)
	assert.NoError(t, err)
	return llm
}

func createTestDatasource(t *testing.T, service *services.Service, name string) *models.Datasource {
	datasource, err := service.CreateDatasource(name, "Short desc", "Long desc", "icon.png", "https://example.com", 75, 1, []string{}, "conn_string", "source_type", "api_key", "dbname", "embed_vendor", "embed_url", "embed_api_key", "embed_model", true)
	assert.NoError(t, err)
	return datasource
}

func createTestTool(t *testing.T, service *services.Service, name string) *models.Tool {
	tool, err := service.CreateTool(name, "Description", models.ToolTypeREST, []byte("OAS Spec"), 8, "apiKey", "secret")
	assert.NoError(t, err)
	return tool
}

func createTestChatHistoryRecord(t *testing.T, service *services.Service, userID uint, sessionID string) *models.ChatHistoryRecord {
	record, err := service.CreateChatHistoryRecord(sessionID, 1, userID, "Test Chat")
	assert.NoError(t, err)
	return record
}

func addCatalogueToUserGroup(t *testing.T, service *services.Service, userID, catalogueID uint) {
	group, err := service.CreateGroup("Test Group")
	assert.NoError(t, err)
	err = service.AddUserToGroup(userID, group.ID)
	assert.NoError(t, err)
	err = service.AddCatalogueToGroup(catalogueID, group.ID)
	assert.NoError(t, err)
}

func addDataCatalogueToUserGroup(t *testing.T, service *services.Service, userID, dataCatalogueID uint) {
	group, err := service.CreateGroup("Test Group")
	assert.NoError(t, err)
	err = service.AddUserToGroup(userID, group.ID)
	assert.NoError(t, err)
	err = service.AddDataCatalogueToGroup(dataCatalogueID, group.ID)
	assert.NoError(t, err)
}

func addToolCatalogueToUserGroup(t *testing.T, service *services.Service, userID, toolCatalogueID uint) {
	group, err := service.CreateGroup("Test Group")
	assert.NoError(t, err)
	err = service.AddUserToGroup(userID, group.ID)
	assert.NoError(t, err)
	err = service.AddToolCatalogueToGroup(toolCatalogueID, group.ID)
	assert.NoError(t, err)
}
