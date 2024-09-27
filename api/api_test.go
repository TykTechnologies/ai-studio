package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
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

func setupTestAPI(t *testing.T) (*API, *gorm.DB) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	err = models.InitModels(db)
	assert.NoError(t, err)

	service := services.NewService(db)

	config := &auth.Config{
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

	api := NewAPI(service, true, auth.NewAuthService(config, newMockMailer()), config)

	return api, db
}

func performRequest(r http.Handler, method, path string, body interface{}) *httptest.ResponseRecorder {
	var reqBody []byte
	if body != nil {
		reqBody, _ = json.Marshal(body)
	}
	req, _ := http.NewRequest(method, path, bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-token")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestUserEndpoints(t *testing.T) {
	api, _ := setupTestAPI(t)

	// Test Create User
	createUserInput := UserInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				Email    string `json:"email"`
				Name     string `json:"name"`
				Password string `json:"password,omitempty"`
				IsAdmin  bool   `json:"is_admin"`
			} `json:"attributes"`
		}{
			Type: "users",
			Attributes: struct {
				Email    string `json:"email"`
				Name     string `json:"name"`
				Password string `json:"password,omitempty"`
				IsAdmin  bool   `json:"is_admin"`
			}{
				Email:    "test@example.com",
				Name:     "Test User",
				Password: "password123",
			},
		},
	}

	w := performRequest(api.router, "POST", "/api/v1/users", createUserInput)
	assert.Equal(t, http.StatusCreated, w.Code)

	var response map[string]UserResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "test@example.com", response["data"].Attributes.Email)

	userID := response["data"].ID

	// Test Get User
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/users/%s", userID), nil)
	assert.Equal(t, http.StatusOK, w.Code)

	// Test Update User
	updateUserInput := UserInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				Email    string `json:"email"`
				Name     string `json:"name"`
				Password string `json:"password,omitempty"`
				IsAdmin  bool   `json:"is_admin"`
			} `json:"attributes"`
		}{
			Type: "users",
			Attributes: struct {
				Email    string `json:"email"`
				Name     string `json:"name"`
				Password string `json:"password,omitempty"`
				IsAdmin  bool   `json:"is_admin"`
			}{
				Email: "updated@example.com",
				Name:  "Updated User",
			},
		},
	}

	w = performRequest(api.router, "PATCH", fmt.Sprintf("/api/v1/users/%s", userID), updateUserInput)
	assert.Equal(t, http.StatusOK, w.Code)

	// Test List Users
	w = performRequest(api.router, "GET", "/api/v1/users", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	// Test Delete User
	w = performRequest(api.router, "DELETE", fmt.Sprintf("/api/v1/users/%s", userID), nil)
	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestGroupEndpoints(t *testing.T) {
	api, _ := setupTestAPI(t)

	// Test Create Group
	createGroupInput := GroupInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				Name string `json:"name"`
			} `json:"attributes"`
		}{
			Type: "groups",
			Attributes: struct {
				Name string `json:"name"`
			}{
				Name: "Test Group",
			},
		},
	}

	w := performRequest(api.router, "POST", "/api/v1/groups", createGroupInput)
	assert.Equal(t, http.StatusCreated, w.Code)

	var response map[string]GroupResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "Test Group", response["data"].Attributes.Name)

	groupID := response["data"].ID

	// Test Get Group
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/groups/%s", groupID), nil)
	assert.Equal(t, http.StatusOK, w.Code)

	// Test Update Group
	updateGroupInput := GroupInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				Name string `json:"name"`
			} `json:"attributes"`
		}{
			Type: "groups",
			Attributes: struct {
				Name string `json:"name"`
			}{
				Name: "Updated Group",
			},
		},
	}

	w = performRequest(api.router, "PATCH", fmt.Sprintf("/api/v1/groups/%s", groupID), updateGroupInput)
	assert.Equal(t, http.StatusOK, w.Code)

	// Test List Groups
	w = performRequest(api.router, "GET", "/api/v1/groups", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	// Test Add User to Group
	createUserInput := UserInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				Email    string `json:"email"`
				Name     string `json:"name"`
				Password string `json:"password,omitempty"`
				IsAdmin  bool   `json:"is_admin"`
			} `json:"attributes"`
		}{
			Type: "users",
			Attributes: struct {
				Email    string `json:"email"`
				Name     string `json:"name"`
				Password string `json:"password,omitempty"`
				IsAdmin  bool   `json:"is_admin"`
			}{
				Email:    "groupuser@example.com",
				Name:     "Group User",
				Password: "password123",
				IsAdmin:  false,
			},
		},
	}

	w = performRequest(api.router, "POST", "/api/v1/users", createUserInput)
	assert.Equal(t, http.StatusCreated, w.Code)

	var userResponse map[string]UserResponse
	err = json.Unmarshal(w.Body.Bytes(), &userResponse)
	assert.NoError(t, err)
	userID := userResponse["data"].ID

	addUserToGroupInput := UserGroupInput{
		Data: struct {
			Type string `json:"type"`
			ID   string `json:"id"`
		}{
			Type: "users",
			ID:   userID,
		},
	}

	w = performRequest(api.router, "POST", fmt.Sprintf("/api/v1/groups/%s/users", groupID), addUserToGroupInput)
	assert.Equal(t, http.StatusNoContent, w.Code)

	// Test List Group Users
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/groups/%s/users", groupID), nil)
	assert.Equal(t, http.StatusOK, w.Code)

	// Test Remove User from Group
	w = performRequest(api.router, "DELETE", fmt.Sprintf("/api/v1/groups/%s/users/%s", groupID, userID), nil)
	assert.Equal(t, http.StatusNoContent, w.Code)

	// Test Add DataCatalogue to Group
	dataCatalogue, err := api.service.CreateDataCatalogue("Test Data Catalogue", "Short Desc", "Long Desc", "icon.png")
	assert.NoError(t, err)

	addDataCatalogueInput := GroupDataCatalogueInput{
		Data: struct {
			Type string `json:"type"`
			ID   string `json:"id"`
		}{
			Type: "data-catalogues",
			ID:   strconv.FormatUint(uint64(dataCatalogue.ID), 10),
		},
	}

	w = performRequest(api.router, "POST", fmt.Sprintf("/api/v1/groups/%s/data-catalogues", groupID), addDataCatalogueInput)
	assert.Equal(t, http.StatusNoContent, w.Code)

	// Test List Group DataCatalogues
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/groups/%s/data-catalogues", groupID), nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var dataCataloguesResponse map[string][]DataCatalogueResponse
	err = json.Unmarshal(w.Body.Bytes(), &dataCataloguesResponse)
	assert.NoError(t, err)
	assert.Len(t, dataCataloguesResponse["data"], 1)
	assert.Equal(t, dataCatalogue.Name, dataCataloguesResponse["data"][0].Attributes.Name)

	// Test Remove DataCatalogue from Group
	w = performRequest(api.router, "DELETE", fmt.Sprintf("/api/v1/groups/%s/data-catalogues/%d", groupID, dataCatalogue.ID), nil)
	assert.Equal(t, http.StatusNoContent, w.Code)

	// Verify DataCatalogue is removed
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/groups/%s/data-catalogues", groupID), nil)
	assert.Equal(t, http.StatusOK, w.Code)
	err = json.Unmarshal(w.Body.Bytes(), &dataCataloguesResponse)
	assert.NoError(t, err)
	assert.Len(t, dataCataloguesResponse["data"], 0)

	// Test Add ToolCatalogue to Group
	toolCatalogue, err := api.service.CreateToolCatalogue("Test Tool Catalogue", "Short Desc", "Long Desc", "icon.png")
	assert.NoError(t, err)

	addToolCatalogueInput := GroupToolCatalogueInput{
		Data: struct {
			Type string `json:"type"`
			ID   string `json:"id"`
		}{
			Type: "tool-catalogues",
			ID:   strconv.FormatUint(uint64(toolCatalogue.ID), 10),
		},
	}

	w = performRequest(api.router, "POST", fmt.Sprintf("/api/v1/groups/%s/tool-catalogues", groupID), addToolCatalogueInput)
	assert.Equal(t, http.StatusNoContent, w.Code)

	// Test List Group ToolCatalogues
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/groups/%s/tool-catalogues", groupID), nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var toolCataloguesResponse map[string][]ToolCatalogueResponse
	err = json.Unmarshal(w.Body.Bytes(), &toolCataloguesResponse)
	assert.NoError(t, err)
	assert.Len(t, toolCataloguesResponse["data"], 1)
	assert.Equal(t, toolCatalogue.Name, toolCataloguesResponse["data"][0].Attributes.Name)

	// Test Remove ToolCatalogue from Group
	w = performRequest(api.router, "DELETE", fmt.Sprintf("/api/v1/groups/%s/tool-catalogues/%d", groupID, toolCatalogue.ID), nil)
	assert.Equal(t, http.StatusNoContent, w.Code)

	// Verify ToolCatalogue is removed
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/groups/%s/tool-catalogues", groupID), nil)
	assert.Equal(t, http.StatusOK, w.Code)
	err = json.Unmarshal(w.Body.Bytes(), &toolCataloguesResponse)
	assert.NoError(t, err)
	assert.Len(t, toolCataloguesResponse["data"], 0)

	// Test Delete Group
	w = performRequest(api.router, "DELETE", fmt.Sprintf("/api/v1/groups/%s", groupID), nil)
	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestGroupEndpointsErrors(t *testing.T) {
	api, _ := setupTestAPI(t)

	// Test Add DataCatalogue to non-existent Group
	addDataCatalogueInput := GroupDataCatalogueInput{
		Data: struct {
			Type string `json:"type"`
			ID   string `json:"id"`
		}{
			Type: "data-catalogues",
			ID:   "1",
		},
	}
	w := performRequest(api.router, "POST", "/api/v1/groups/999/data-catalogues", addDataCatalogueInput)
	assert.Equal(t, http.StatusNotFound, w.Code)

	// Test Remove DataCatalogue from non-existent Group
	w = performRequest(api.router, "DELETE", "/api/v1/groups/999/data-catalogues/1", nil)
	assert.Equal(t, http.StatusNotFound, w.Code)

	// Test Add ToolCatalogue to non-existent Group
	addToolCatalogueInput := GroupToolCatalogueInput{
		Data: struct {
			Type string `json:"type"`
			ID   string `json:"id"`
		}{
			Type: "tool-catalogues",
			ID:   "1",
		},
	}
	w = performRequest(api.router, "POST", "/api/v1/groups/999/tool-catalogues", addToolCatalogueInput)
	assert.Equal(t, http.StatusNotFound, w.Code)

	// Test Remove ToolCatalogue from non-existent Group
	w = performRequest(api.router, "DELETE", "/api/v1/groups/999/tool-catalogues/1", nil)
	assert.Equal(t, http.StatusNotFound, w.Code)

	// Create a valid group for further testing
	group, err := api.service.CreateGroup("Test Group")
	assert.NoError(t, err)

	// Test Add non-existent DataCatalogue to Group
	addDataCatalogueInput = GroupDataCatalogueInput{
		Data: struct {
			Type string `json:"type"`
			ID   string `json:"id"`
		}{
			Type: "data-catalogues",
			ID:   "999",
		},
	}
	w = performRequest(api.router, "POST", fmt.Sprintf("/api/v1/groups/%d/data-catalogues", group.ID), addDataCatalogueInput)
	assert.Equal(t, http.StatusNotFound, w.Code)

	// Test Remove non-existent DataCatalogue from Group
	w = performRequest(api.router, "DELETE", fmt.Sprintf("/api/v1/groups/%d/data-catalogues/999", group.ID), nil)
	assert.Equal(t, http.StatusNotFound, w.Code)

	// Test Add non-existent ToolCatalogue to Group
	addToolCatalogueInput = GroupToolCatalogueInput{
		Data: struct {
			Type string `json:"type"`
			ID   string `json:"id"`
		}{
			Type: "tool-catalogues",
			ID:   "999",
		},
	}
	w = performRequest(api.router, "POST", fmt.Sprintf("/api/v1/groups/%d/tool-catalogues", group.ID), addToolCatalogueInput)
	assert.Equal(t, http.StatusNotFound, w.Code)

	// Test Remove non-existent ToolCatalogue from Group
	w = performRequest(api.router, "DELETE", fmt.Sprintf("/api/v1/groups/%d/tool-catalogues/999", group.ID), nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	m.Run()
}

func TestLLMEndpoints(t *testing.T) {
	api, _ := setupTestAPI(t)

	// Test Create LLM
	createLLMInput := LLMInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				Name             string `json:"name"`
				APIKey           string `json:"api_key"`
				APIEndpoint      string `json:"api_endpoint"`
				PrivacyScore     int    `json:"privacy_score"`
				ShortDescription string `json:"short_description"`
				LongDescription  string `json:"long_description"`
				LogoURL          string `json:"logo_url"`
				Vendor           string `json:"vendor"`
				Active           bool   `json:"active"`
			} `json:"attributes"`
		}{
			Type: "llms",
			Attributes: struct {
				Name             string `json:"name"`
				APIKey           string `json:"api_key"`
				APIEndpoint      string `json:"api_endpoint"`
				PrivacyScore     int    `json:"privacy_score"`
				ShortDescription string `json:"short_description"`
				LongDescription  string `json:"long_description"`
				LogoURL          string `json:"logo_url"`
				Vendor           string `json:"vendor"`
				Active           bool   `json:"active"`
			}{
				Name:             "Test LLM",
				APIKey:           "test-api-key",
				APIEndpoint:      "https://api.test.com",
				PrivacyScore:     75,
				ShortDescription: "A test LLM",
				LongDescription:  "This is a test LLM for API testing",
				LogoURL:          "https://testllm.com/logo.png",
				Vendor:           "Test Vendor",
				Active:           true,
			},
		},
	}

	w := performRequest(api.router, "POST", "/api/v1/llms", createLLMInput)
	assert.Equal(t, http.StatusCreated, w.Code)

	var response map[string]LLMResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "Test LLM", response["data"].Attributes.Name)

	llmID := response["data"].ID

	// Test Get LLM
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/llms/%s", llmID), nil)
	assert.Equal(t, http.StatusOK, w.Code)

	// Test Update LLM
	updateLLMInput := LLMInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				Name             string `json:"name"`
				APIKey           string `json:"api_key"`
				APIEndpoint      string `json:"api_endpoint"`
				PrivacyScore     int    `json:"privacy_score"`
				ShortDescription string `json:"short_description"`
				LongDescription  string `json:"long_description"`
				LogoURL          string `json:"logo_url"`
				Vendor           string `json:"vendor"`
				Active           bool   `json:"active"`
			} `json:"attributes"`
		}{
			Type: "llms",
			Attributes: struct {
				Name             string `json:"name"`
				APIKey           string `json:"api_key"`
				APIEndpoint      string `json:"api_endpoint"`
				PrivacyScore     int    `json:"privacy_score"`
				ShortDescription string `json:"short_description"`
				LongDescription  string `json:"long_description"`
				LogoURL          string `json:"logo_url"`
				Vendor           string `json:"vendor"`
				Active           bool   `json:"active"`
			}{
				Name:             "Updated Test LLM",
				APIKey:           "updated-api-key",
				APIEndpoint:      "https://updated-api.test.com",
				PrivacyScore:     80,
				ShortDescription: "An updated test LLM",
				LongDescription:  "This is an updated test LLM for API testing",
				LogoURL:          "https://updatedtestllm.com/logo.png",
				Vendor:           "Updated Test Vendor",
				Active:           true,
			},
		},
	}

	w = performRequest(api.router, "PATCH", fmt.Sprintf("/api/v1/llms/%s", llmID), updateLLMInput)
	assert.Equal(t, http.StatusOK, w.Code)

	// Test List LLMs
	w = performRequest(api.router, "GET", "/api/v1/llms", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	// Test Search LLMs
	w = performRequest(api.router, "GET", "/api/v1/llms/search?name=Updated", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	// Test Delete LLM
	w = performRequest(api.router, "DELETE", fmt.Sprintf("/api/v1/llms/%s", llmID), nil)
	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestLLMPrivacyScoreEndpoints(t *testing.T) {
	api, db := setupTestAPI(t)

	// Create some test LLMs with different privacy scores
	llms := []models.LLM{
		{Name: "LLM1", APIKey: "key1", APIEndpoint: "https://api1.com", PrivacyScore: 30},
		{Name: "LLM2", APIKey: "key2", APIEndpoint: "https://api2.com", PrivacyScore: 50},
		{Name: "LLM3", APIKey: "key3", APIEndpoint: "https://api3.com", PrivacyScore: 70},
		{Name: "LLM4", APIKey: "key4", APIEndpoint: "https://api4.com", PrivacyScore: 90},
	}

	for _, llm := range llms {
		err := db.Create(&llm).Error
		assert.NoError(t, err)
	}

	// Test GetLLMsByMaxPrivacyScore
	w := performRequest(api.router, "GET", "/api/v1/llms/max-privacy-score?max_score=60", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var maxScoreResponse map[string][]LLMResponse
	err := json.Unmarshal(w.Body.Bytes(), &maxScoreResponse)
	assert.NoError(t, err)
	assert.Len(t, maxScoreResponse["data"], 2)
	assert.ElementsMatch(t, []string{"LLM1", "LLM2"}, []string{maxScoreResponse["data"][0].Attributes.Name, maxScoreResponse["data"][1].Attributes.Name})

	// Test GetLLMsByMinPrivacyScore
	w = performRequest(api.router, "GET", "/api/v1/llms/min-privacy-score?min_score=70", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var minScoreResponse map[string][]LLMResponse
	err = json.Unmarshal(w.Body.Bytes(), &minScoreResponse)
	assert.NoError(t, err)
	assert.Len(t, minScoreResponse["data"], 2)
	assert.ElementsMatch(t, []string{"LLM3", "LLM4"}, []string{minScoreResponse["data"][0].Attributes.Name, minScoreResponse["data"][1].Attributes.Name})

	// Test GetLLMsByPrivacyScoreRange
	w = performRequest(api.router, "GET", "/api/v1/llms/privacy-score-range?min_score=40&max_score=80", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var rangeScoreResponse map[string][]LLMResponse
	err = json.Unmarshal(w.Body.Bytes(), &rangeScoreResponse)
	assert.NoError(t, err)
	assert.Len(t, rangeScoreResponse["data"], 2)
	assert.ElementsMatch(t, []string{"LLM2", "LLM3"}, []string{rangeScoreResponse["data"][0].Attributes.Name, rangeScoreResponse["data"][1].Attributes.Name})

	// Test invalid input for GetLLMsByMaxPrivacyScore
	w = performRequest(api.router, "GET", "/api/v1/llms/max-privacy-score?max_score=invalid", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// Test invalid input for GetLLMsByMinPrivacyScore
	w = performRequest(api.router, "GET", "/api/v1/llms/min-privacy-score?min_score=invalid", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// Test invalid input for GetLLMsByPrivacyScoreRange
	w = performRequest(api.router, "GET", "/api/v1/llms/privacy-score-range?min_score=80&max_score=70", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCatalogueEndpoints(t *testing.T) {
	api, _ := setupTestAPI(t)

	// Test Create Catalogue
	createCatalogueInput := CatalogueInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				Name string `json:"name"`
			} `json:"attributes"`
		}{
			Type: "catalogues",
			Attributes: struct {
				Name string `json:"name"`
			}{
				Name: "Test Catalogue",
			},
		},
	}

	w := performRequest(api.router, "POST", "/api/v1/catalogues", createCatalogueInput)
	assert.Equal(t, http.StatusCreated, w.Code)

	var response map[string]CatalogueResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "Test Catalogue", response["data"].Attributes.Name)

	catalogueID := response["data"].ID

	// Test Get Catalogue
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/catalogues/%s", catalogueID), nil)
	assert.Equal(t, http.StatusOK, w.Code)

	// Test Update Catalogue
	updateCatalogueInput := CatalogueInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				Name string `json:"name"`
			} `json:"attributes"`
		}{
			Type: "catalogues",
			Attributes: struct {
				Name string `json:"name"`
			}{
				Name: "Updated Catalogue",
			},
		},
	}

	w = performRequest(api.router, "PATCH", fmt.Sprintf("/api/v1/catalogues/%s", catalogueID), updateCatalogueInput)
	assert.Equal(t, http.StatusOK, w.Code)

	// Test List Catalogues
	w = performRequest(api.router, "GET", "/api/v1/catalogues", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	// Test Search Catalogues
	w = performRequest(api.router, "GET", "/api/v1/catalogues/search?name=Updated", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var searchResponse map[string][]CatalogueResponse
	err = json.Unmarshal(w.Body.Bytes(), &searchResponse)
	assert.NoError(t, err)
	assert.Len(t, searchResponse["data"], 1)
	assert.Equal(t, "Updated Catalogue", searchResponse["data"][0].Attributes.Name)

	// Test Add LLM to Catalogue
	createLLMInput := LLMInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				Name             string `json:"name"`
				APIKey           string `json:"api_key"`
				APIEndpoint      string `json:"api_endpoint"`
				PrivacyScore     int    `json:"privacy_score"`
				ShortDescription string `json:"short_description"`
				LongDescription  string `json:"long_description"`
				LogoURL          string `json:"logo_url"`
				Vendor           string `json:"vendor"`
				Active           bool   `json:"active"`
			} `json:"attributes"`
		}{
			Type: "llms",
			Attributes: struct {
				Name             string `json:"name"`
				APIKey           string `json:"api_key"`
				APIEndpoint      string `json:"api_endpoint"`
				PrivacyScore     int    `json:"privacy_score"`
				ShortDescription string `json:"short_description"`
				LongDescription  string `json:"long_description"`
				LogoURL          string `json:"logo_url"`
				Vendor           string `json:"vendor"`
				Active           bool   `json:"active"`
			}{
				Name:             "Test LLM",
				APIKey:           "test-api-key",
				APIEndpoint:      "https://api.test.com",
				PrivacyScore:     75,
				ShortDescription: "A test LLM",
				LongDescription:  "This is a test LLM for API testing",
				LogoURL:          "https://testllm.com/logo.png",
			},
		},
	}

	w = performRequest(api.router, "POST", "/api/v1/llms", createLLMInput)
	assert.Equal(t, http.StatusCreated, w.Code)

	var llmResponse map[string]LLMResponse
	err = json.Unmarshal(w.Body.Bytes(), &llmResponse)
	assert.NoError(t, err)
	llmID := llmResponse["data"].ID

	addLLMToCatalogueInput := CatalogueLLMInput{
		Data: struct {
			Type string `json:"type"`
			ID   string `json:"id"`
		}{
			Type: "llms",
			ID:   llmID,
		},
	}

	w = performRequest(api.router, "POST", fmt.Sprintf("/api/v1/catalogues/%s/llms", catalogueID), addLLMToCatalogueInput)
	assert.Equal(t, http.StatusNoContent, w.Code)

	// Test List Catalogue LLMs
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/catalogues/%s/llms", catalogueID), nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var llmsResponse map[string][]LLMResponse
	err = json.Unmarshal(w.Body.Bytes(), &llmsResponse)
	assert.NoError(t, err)
	assert.Len(t, llmsResponse["data"], 1)
	assert.Equal(t, "Test LLM", llmsResponse["data"][0].Attributes.Name)

	// Test Remove LLM from Catalogue
	w = performRequest(api.router, "DELETE", fmt.Sprintf("/api/v1/catalogues/%s/llms/%s", catalogueID, llmID), nil)
	assert.Equal(t, http.StatusNoContent, w.Code)

	// Verify LLM is removed
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/catalogues/%s/llms", catalogueID), nil)
	assert.Equal(t, http.StatusOK, w.Code)
	err = json.Unmarshal(w.Body.Bytes(), &llmsResponse)
	assert.NoError(t, err)
	assert.Len(t, llmsResponse["data"], 0)

	// Test Delete Catalogue
	w = performRequest(api.router, "DELETE", fmt.Sprintf("/api/v1/catalogues/%s", catalogueID), nil)
	assert.Equal(t, http.StatusNoContent, w.Code)

	// Verify catalogue is deleted
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/catalogues/%s", catalogueID), nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestGroupCatalogueAssociation(t *testing.T) {
	api, _ := setupTestAPI(t)

	// Create a group
	group, err := api.service.CreateGroup("Test Group")
	assert.NoError(t, err)

	// Create a catalogue
	catalogue, err := api.service.CreateCatalogue("Test Catalogue")
	assert.NoError(t, err)

	// Test Add Catalogue to Group
	addCatalogueInput := GroupCatalogueInput{
		Data: struct {
			Type string `json:"type"`
			ID   string `json:"id"`
		}{
			Type: "catalogues",
			ID:   strconv.FormatUint(uint64(catalogue.ID), 10),
		},
	}

	w := performRequest(api.router, "POST", fmt.Sprintf("/api/v1/groups/%d/catalogues", group.ID), addCatalogueInput)
	assert.Equal(t, http.StatusNoContent, w.Code)

	// Test List Group Catalogues
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/groups/%d/catalogues", group.ID), nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var cataloguesResponse map[string][]CatalogueResponse
	err = json.Unmarshal(w.Body.Bytes(), &cataloguesResponse)
	assert.NoError(t, err)
	assert.Len(t, cataloguesResponse["data"], 1)
	assert.Equal(t, catalogue.Name, cataloguesResponse["data"][0].Attributes.Name)

	// Test Remove Catalogue from Group
	w = performRequest(api.router, "DELETE", fmt.Sprintf("/api/v1/groups/%d/catalogues/%d", group.ID, catalogue.ID), nil)
	assert.Equal(t, http.StatusNoContent, w.Code)

	// Verify catalogue is removed
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/groups/%d/catalogues", group.ID), nil)
	assert.Equal(t, http.StatusOK, w.Code)
	err = json.Unmarshal(w.Body.Bytes(), &cataloguesResponse)
	assert.NoError(t, err)
	assert.Len(t, cataloguesResponse["data"], 0)
}

func TestUserAccessibleCatalogues(t *testing.T) {
	api, _ := setupTestAPI(t)

	// Create a user
	user, err := api.service.CreateUser("test@example.com", "Test User", "password123", false)
	assert.NoError(t, err)

	// Create a group
	group, err := api.service.CreateGroup("Test Group")
	assert.NoError(t, err)

	// Add user to group
	err = api.service.AddUserToGroup(user.ID, group.ID)
	assert.NoError(t, err)

	// Create a catalogue
	catalogue, err := api.service.CreateCatalogue("Test Catalogue")
	assert.NoError(t, err)

	// Add catalogue to group
	err = api.service.AddCatalogueToGroup(catalogue.ID, group.ID)
	assert.NoError(t, err)

	// Test Get User Accessible Catalogues
	w := performRequest(api.router, "GET", fmt.Sprintf("/api/v1/users/%d/catalogues", user.ID), nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]UserAccessibleCataloguesResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Len(t, response["data"].Attributes.Catalogues, 1)
	assert.Equal(t, catalogue.Name, response["data"].Attributes.Catalogues[0].Attributes.Name)
}

func TestTagEndpoints(t *testing.T) {
	api, _ := setupTestAPI(t)

	// Test Create Tag
	createTagInput := TagInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				Name string `json:"name"`
			} `json:"attributes"`
		}{
			Type: "tags",
			Attributes: struct {
				Name string `json:"name"`
			}{
				Name: "Test Tag",
			},
		},
	}

	w := performRequest(api.router, "POST", "/api/v1/tags", createTagInput)
	assert.Equal(t, http.StatusCreated, w.Code)

	var response map[string]TagResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "Test Tag", response["data"].Attributes.Name)

	tagID := response["data"].ID

	// Test Get Tag
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/tags/%s", tagID), nil)
	assert.Equal(t, http.StatusOK, w.Code)

	// Test Update Tag
	updateTagInput := TagInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				Name string `json:"name"`
			} `json:"attributes"`
		}{
			Type: "tags",
			Attributes: struct {
				Name string `json:"name"`
			}{
				Name: "Updated Tag",
			},
		},
	}

	w = performRequest(api.router, "PATCH", fmt.Sprintf("/api/v1/tags/%s", tagID), updateTagInput)
	assert.Equal(t, http.StatusOK, w.Code)

	// Test List Tags
	w = performRequest(api.router, "GET", "/api/v1/tags", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	// Test Search Tags
	w = performRequest(api.router, "GET", "/api/v1/tags/search?name=Updated", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var searchResponse map[string][]TagResponse
	err = json.Unmarshal(w.Body.Bytes(), &searchResponse)
	assert.NoError(t, err)
	assert.Len(t, searchResponse["data"], 1)
	assert.Equal(t, "Updated Tag", searchResponse["data"][0].Attributes.Name)

	// Test Delete Tag
	w = performRequest(api.router, "DELETE", fmt.Sprintf("/api/v1/tags/%s", tagID), nil)
	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestDatasourceEndpoints(t *testing.T) {
	api, _ := setupTestAPI(t)

	// Create a user for testing
	user, err := api.service.CreateUser("test@example.com", "Test User", "password123", true)
	assert.NoError(t, err)

	// Test Create Datasource
	createDatasourceInput := DatasourceInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				Name             string   `json:"name"`
				ShortDescription string   `json:"short_description"`
				LongDescription  string   `json:"long_description"`
				Icon             string   `json:"icon"`
				Url              string   `json:"url"`
				PrivacyScore     int      `json:"privacy_score"`
				UserID           uint     `json:"user_id"`
				Tags             []string `json:"tags"`
				DBConnString     string   `json:"db_conn_string"`
				DBSourceType     string   `json:"db_source_type"`
				DBConnAPIKey     string   `json:"db_conn_api_key"`
				DBName           string   `json:"db_name"`
				EmbedVendor      string   `json:"embed_vendor"`
				EmbedUrl         string   `json:"embed_url"`
				EmbedAPIKey      string   `json:"embed_api_key"`
				EmbedModel       string   `json:"embed_model"`
				Active           bool     `json:"active"`
			} `json:"attributes"`
		}{
			Type: "datasources",
			Attributes: struct {
				Name             string   `json:"name"`
				ShortDescription string   `json:"short_description"`
				LongDescription  string   `json:"long_description"`
				Icon             string   `json:"icon"`
				Url              string   `json:"url"`
				PrivacyScore     int      `json:"privacy_score"`
				UserID           uint     `json:"user_id"`
				Tags             []string `json:"tags"`
				DBConnString     string   `json:"db_conn_string"`
				DBSourceType     string   `json:"db_source_type"`
				DBConnAPIKey     string   `json:"db_conn_api_key"`
				DBName           string   `json:"db_name"`
				EmbedVendor      string   `json:"embed_vendor"`
				EmbedUrl         string   `json:"embed_url"`
				EmbedAPIKey      string   `json:"embed_api_key"`
				EmbedModel       string   `json:"embed_model"`
				Active           bool     `json:"active"`
			}{
				Name:             "Test Datasource",
				ShortDescription: "Short description",
				LongDescription:  "Long description",
				Icon:             "icon.png",
				Url:              "https://example.com",
				PrivacyScore:     75,
				UserID:           user.ID,
				Tags:             []string{"tag1", "tag2"},
				DBConnString:     "test_conn_string",
				DBSourceType:     "test_source_type",
				DBConnAPIKey:     "test_api_key",
				EmbedVendor:      "test_vendor",
				EmbedUrl:         "https://embed.example.com",
				EmbedAPIKey:      "test_embed_api_key",
				EmbedModel:       "test_model",
				Active:           true,
			},
		},
	}

	w := performRequest(api.router, "POST", "/api/v1/datasources", createDatasourceInput)
	assert.Equal(t, http.StatusCreated, w.Code)

	var response map[string]DatasourceResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "Test Datasource", response["data"].Attributes.Name)

	datasourceID := response["data"].ID

	// Test Get Datasource
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/datasources/%s", datasourceID), nil)
	assert.Equal(t, http.StatusOK, w.Code)

	// Test Update Datasource
	updateDatasourceInput := DatasourceInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				Name             string   `json:"name"`
				ShortDescription string   `json:"short_description"`
				LongDescription  string   `json:"long_description"`
				Icon             string   `json:"icon"`
				Url              string   `json:"url"`
				PrivacyScore     int      `json:"privacy_score"`
				UserID           uint     `json:"user_id"`
				Tags             []string `json:"tags"`
				DBConnString     string   `json:"db_conn_string"`
				DBSourceType     string   `json:"db_source_type"`
				DBConnAPIKey     string   `json:"db_conn_api_key"`
				DBName           string   `json:"db_name"`
				EmbedVendor      string   `json:"embed_vendor"`
				EmbedUrl         string   `json:"embed_url"`
				EmbedAPIKey      string   `json:"embed_api_key"`
				EmbedModel       string   `json:"embed_model"`
				Active           bool     `json:"active"`
			} `json:"attributes"`
		}{
			Type: "datasources",
			Attributes: struct {
				Name             string   `json:"name"`
				ShortDescription string   `json:"short_description"`
				LongDescription  string   `json:"long_description"`
				Icon             string   `json:"icon"`
				Url              string   `json:"url"`
				PrivacyScore     int      `json:"privacy_score"`
				UserID           uint     `json:"user_id"`
				Tags             []string `json:"tags"`
				DBConnString     string   `json:"db_conn_string"`
				DBSourceType     string   `json:"db_source_type"`
				DBConnAPIKey     string   `json:"db_conn_api_key"`
				DBName           string   `json:"db_name"`
				EmbedVendor      string   `json:"embed_vendor"`
				EmbedUrl         string   `json:"embed_url"`
				EmbedAPIKey      string   `json:"embed_api_key"`
				EmbedModel       string   `json:"embed_model"`
				Active           bool     `json:"active"`
			}{
				Name:             "Updated Datasource",
				ShortDescription: "Updated short description",
				LongDescription:  "Updated long description",
				Icon:             "updated-icon.png",
				Url:              "https://updated-example.com",
				PrivacyScore:     80,
				UserID:           user.ID,
				Tags:             []string{"tag1", "tag2", "tag3"},
				DBConnString:     "updated_conn_string",
				DBSourceType:     "updated_source_type",
				DBConnAPIKey:     "updated_api_key",
				EmbedVendor:      "updated_vendor",
				EmbedUrl:         "https://updated-embed.example.com",
				EmbedAPIKey:      "updated_embed_api_key",
				EmbedModel:       "updated_model",
				Active:           false,
			},
		},
	}

	w = performRequest(api.router, "PATCH", fmt.Sprintf("/api/v1/datasources/%s", datasourceID), updateDatasourceInput)
	assert.Equal(t, http.StatusOK, w.Code)

	// Test List Datasources
	w = performRequest(api.router, "GET", "/api/v1/datasources", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	// Test Search Datasources
	w = performRequest(api.router, "GET", "/api/v1/datasources/search?query=Updated", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var searchResponse map[string][]DatasourceResponse
	err = json.Unmarshal(w.Body.Bytes(), &searchResponse)
	assert.NoError(t, err)
	assert.Len(t, searchResponse["data"], 1)
	assert.Equal(t, "Updated Datasource", searchResponse["data"][0].Attributes.Name)

	// Test Get Datasources by Tag
	w = performRequest(api.router, "GET", "/api/v1/datasources/by-tag?tag=tag1", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var tagResponse map[string][]DatasourceResponse
	err = json.Unmarshal(w.Body.Bytes(), &tagResponse)
	assert.NoError(t, err)
	assert.Len(t, tagResponse["data"], 1)
	assert.Equal(t, "Updated Datasource", tagResponse["data"][0].Attributes.Name)

	// Test Delete Datasource
	w = performRequest(api.router, "DELETE", fmt.Sprintf("/api/v1/datasources/%s", datasourceID), nil)
	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestLLMSettingsEndpoints(t *testing.T) {
	api, _ := setupTestAPI(t)

	// Test Create LLMSettings
	createLLMSettingsInput := LLMSettingsInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				ModelName         string                 `json:"model_name"`
				MaxLength         int                    `json:"max_length"`
				MaxTokens         int                    `json:"max_tokens"`
				Metadata          map[string]interface{} `json:"metadata"`
				MinLength         int                    `json:"min_length"`
				RepetitionPenalty float64                `json:"repetition_penalty"`
				Seed              int                    `json:"seed"`
				StopWords         []string               `json:"stop_words"`
				Temperature       float64                `json:"temperature"`
				TopK              int                    `json:"top_k"`
				TopP              float64                `json:"top_p"`
				SystemPrompt      string                 `json:"system_prompt"`
			} `json:"attributes"`
		}{
			Type: "llm-settings",
			Attributes: struct {
				ModelName         string                 `json:"model_name"`
				MaxLength         int                    `json:"max_length"`
				MaxTokens         int                    `json:"max_tokens"`
				Metadata          map[string]interface{} `json:"metadata"`
				MinLength         int                    `json:"min_length"`
				RepetitionPenalty float64                `json:"repetition_penalty"`
				Seed              int                    `json:"seed"`
				StopWords         []string               `json:"stop_words"`
				Temperature       float64                `json:"temperature"`
				TopK              int                    `json:"top_k"`
				TopP              float64                `json:"top_p"`
				SystemPrompt      string                 `json:"system_prompt"`
			}{
				ModelName:         "TestModel",
				MaxLength:         100,
				MaxTokens:         50,
				Metadata:          map[string]interface{}{"key": "value"},
				MinLength:         10,
				RepetitionPenalty: 1.2,
				Seed:              42,
				StopWords:         []string{"stop1", "stop2"},
				Temperature:       0.7,
				TopK:              40,
				TopP:              0.9,
				SystemPrompt:      "Test prompt",
			},
		},
	}

	w := performRequest(api.router, "POST", "/api/v1/llm-settings", createLLMSettingsInput)
	assert.Equal(t, http.StatusCreated, w.Code)

	var response map[string]LLMSettingsResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "TestModel", response["data"].Attributes.ModelName)

	settingsID := response["data"].ID

	// Test Get LLMSettings
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/llm-settings/%s", settingsID), nil)
	assert.Equal(t, http.StatusOK, w.Code)

	// Test Update LLMSettings
	updateLLMSettingsInput := LLMSettingsInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				ModelName         string                 `json:"model_name"`
				MaxLength         int                    `json:"max_length"`
				MaxTokens         int                    `json:"max_tokens"`
				Metadata          map[string]interface{} `json:"metadata"`
				MinLength         int                    `json:"min_length"`
				RepetitionPenalty float64                `json:"repetition_penalty"`
				Seed              int                    `json:"seed"`
				StopWords         []string               `json:"stop_words"`
				Temperature       float64                `json:"temperature"`
				TopK              int                    `json:"top_k"`
				TopP              float64                `json:"top_p"`
				SystemPrompt      string                 `json:"system_prompt"`
			} `json:"attributes"`
		}{
			Type: "llm-settings",
			Attributes: struct {
				ModelName         string                 `json:"model_name"`
				MaxLength         int                    `json:"max_length"`
				MaxTokens         int                    `json:"max_tokens"`
				Metadata          map[string]interface{} `json:"metadata"`
				MinLength         int                    `json:"min_length"`
				RepetitionPenalty float64                `json:"repetition_penalty"`
				Seed              int                    `json:"seed"`
				StopWords         []string               `json:"stop_words"`
				Temperature       float64                `json:"temperature"`
				TopK              int                    `json:"top_k"`
				TopP              float64                `json:"top_p"`
				SystemPrompt      string                 `json:"system_prompt"`
			}{
				ModelName:         "UpdatedTestModel",
				MaxLength:         120,
				MaxTokens:         60,
				Metadata:          map[string]interface{}{"key": "updated_value"},
				MinLength:         15,
				RepetitionPenalty: 1.3,
				Seed:              43,
				StopWords:         []string{"stop1", "stop2", "stop3"},
				Temperature:       0.8,
				TopK:              50,
				TopP:              0.95,
			},
		},
	}

	w = performRequest(api.router, "PATCH", fmt.Sprintf("/api/v1/llm-settings/%s", settingsID), updateLLMSettingsInput)
	assert.Equal(t, http.StatusOK, w.Code)

	// Test List LLMSettings
	w = performRequest(api.router, "GET", "/api/v1/llm-settings", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var listResponse map[string][]LLMSettingsResponse
	err = json.Unmarshal(w.Body.Bytes(), &listResponse)
	assert.NoError(t, err)
	assert.Len(t, listResponse["data"], 1)
	assert.Equal(t, "UpdatedTestModel", listResponse["data"][0].Attributes.ModelName)

	// Test Search LLMSettings
	w = performRequest(api.router, "GET", "/api/v1/llm-settings/search?model_name=Updated", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var searchResponse map[string][]LLMSettingsResponse
	err = json.Unmarshal(w.Body.Bytes(), &searchResponse)
	assert.NoError(t, err)
	assert.Len(t, searchResponse["data"], 1)
	assert.Equal(t, "UpdatedTestModel", searchResponse["data"][0].Attributes.ModelName)

	// Test Delete LLMSettings
	w = performRequest(api.router, "DELETE", fmt.Sprintf("/api/v1/llm-settings/%s", settingsID), nil)
	assert.Equal(t, http.StatusNoContent, w.Code)

	// Verify LLMSettings is deleted
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/llm-settings/%s", settingsID), nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestChatEndpoints(t *testing.T) {
	api, _ := setupTestAPI(t)

	// Create test data
	group, err := api.service.CreateGroup("Test Group")
	assert.NoError(t, err)

	llmSettings, err := api.service.CreateLLMSettings(&models.LLMSettings{ModelName: "TestModel"})
	assert.NoError(t, err)

	llm, err := api.service.CreateLLM("TestLLM", "api-key", "http://api.test", 75, "Short desc", "Long desc", "http://logo.test", models.OPENAI, true)
	assert.NoError(t, err)

	// Test Create Chat
	createChatInput := ChatInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				Name          string `json:"name"`
				LLMSettingsID uint   `json:"llm_settings_id"`
				LLMID         uint   `json:"llm_id"`
				GroupIDs      []uint `json:"group_ids"`
			} `json:"attributes"`
		}{
			Type: "chats",
			Attributes: struct {
				Name          string `json:"name"`
				LLMSettingsID uint   `json:"llm_settings_id"`
				LLMID         uint   `json:"llm_id"`
				GroupIDs      []uint `json:"group_ids"`
			}{
				Name:          "Test Chat",
				LLMSettingsID: llmSettings.ID,
				LLMID:         llm.ID,
				GroupIDs:      []uint{group.ID},
			},
		},
	}

	w := performRequest(api.router, "POST", "/api/v1/chats", createChatInput)
	assert.Equal(t, http.StatusCreated, w.Code)

	var response map[string]ChatResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "Test Chat", response["data"].Attributes.Name)

	chatID := response["data"].ID

	// Test Get Chat
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/chats/%s", chatID), nil)
	assert.Equal(t, http.StatusOK, w.Code)

	// Test Update Chat
	updateChatInput := ChatInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				Name          string `json:"name"`
				LLMSettingsID uint   `json:"llm_settings_id"`
				LLMID         uint   `json:"llm_id"`
				GroupIDs      []uint `json:"group_ids"`
			} `json:"attributes"`
		}{
			Type: "chats",
			Attributes: struct {
				Name          string `json:"name"`
				LLMSettingsID uint   `json:"llm_settings_id"`
				LLMID         uint   `json:"llm_id"`
				GroupIDs      []uint `json:"group_ids"`
			}{
				Name:          "Updated Chat",
				LLMSettingsID: llmSettings.ID,
				LLMID:         llm.ID,
				GroupIDs:      []uint{group.ID},
			},
		},
	}

	w = performRequest(api.router, "PATCH", fmt.Sprintf("/api/v1/chats/%s", chatID), updateChatInput)
	assert.Equal(t, http.StatusOK, w.Code)

	// Test List Chats
	w = performRequest(api.router, "GET", "/api/v1/chats", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var listResponse map[string][]ChatResponse
	err = json.Unmarshal(w.Body.Bytes(), &listResponse)
	assert.NoError(t, err)
	assert.Len(t, listResponse["data"], 1)
	assert.Equal(t, "Updated Chat", listResponse["data"][0].Attributes.Name)

	// Test Get Chats by Group ID
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/chats/by-group?group_id=%d", group.ID), nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var groupChatsResponse map[string][]ChatResponse
	err = json.Unmarshal(w.Body.Bytes(), &groupChatsResponse)
	assert.NoError(t, err)
	assert.Len(t, groupChatsResponse["data"], 1)
	assert.Equal(t, "Updated Chat", groupChatsResponse["data"][0].Attributes.Name)

	// Test Delete Chat
	w = performRequest(api.router, "DELETE", fmt.Sprintf("/api/v1/chats/%s", chatID), nil)
	assert.Equal(t, http.StatusNoContent, w.Code)

	// Verify chat is deleted
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/chats/%s", chatID), nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestChatEndpointsErrors(t *testing.T) {
	api, _ := setupTestAPI(t)

	// Test Get non-existent chat
	w := performRequest(api.router, "GET", "/api/v1/chats/999", nil)
	assert.Equal(t, http.StatusNotFound, w.Code)

	// Test Update non-existent chat
	updateChatInput := ChatInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				Name          string `json:"name"`
				LLMSettingsID uint   `json:"llm_settings_id"`
				LLMID         uint   `json:"llm_id"`
				GroupIDs      []uint `json:"group_ids"`
			} `json:"attributes"`
		}{
			Type: "chats",
			Attributes: struct {
				Name          string `json:"name"`
				LLMSettingsID uint   `json:"llm_settings_id"`
				LLMID         uint   `json:"llm_id"`
				GroupIDs      []uint `json:"group_ids"`
			}{
				Name:          "Updated Chat",
				LLMSettingsID: 1,
				LLMID:         1,
				GroupIDs:      []uint{1},
			},
		},
	}
	w = performRequest(api.router, "PATCH", "/api/v1/chats/999", updateChatInput)
	assert.Equal(t, http.StatusNotFound, w.Code)

	// Test Delete non-existent chat
	w = performRequest(api.router, "DELETE", "/api/v1/chats/999", nil)
	assert.Equal(t, http.StatusInternalServerError, w.Code)

	// Test Create chat with invalid input
	invalidCreateChatInput := ChatInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				Name          string `json:"name"`
				LLMSettingsID uint   `json:"llm_settings_id"`
				LLMID         uint   `json:"llm_id"`
				GroupIDs      []uint `json:"group_ids"`
			} `json:"attributes"`
		}{
			Type: "chats",
			Attributes: struct {
				Name          string `json:"name"`
				LLMSettingsID uint   `json:"llm_settings_id"`
				LLMID         uint   `json:"llm_id"`
				GroupIDs      []uint `json:"group_ids"`
			}{
				Name:          "",
				LLMSettingsID: 0,
				LLMID:         0,
				GroupIDs:      []uint{},
			},
		},
	}
	w = performRequest(api.router, "POST", "/api/v1/chats", invalidCreateChatInput)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// Test Get chats by non-existent group
	w = performRequest(api.router, "GET", "/api/v1/chats/by-group?group_id=999", nil)
	assert.Equal(t, http.StatusOK, w.Code) // This should return an empty list, not an error

	var emptyResponse map[string][]ChatResponse
	err := json.Unmarshal(w.Body.Bytes(), &emptyResponse)
	assert.NoError(t, err)
	assert.Len(t, emptyResponse["data"], 0)
}

func TestToolEndpoints(t *testing.T) {
	api, _ := setupTestAPI(t)

	// Test Create Tool
	createToolInput := ToolInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				Name         string `json:"name"`
				Description  string `json:"description"`
				ToolType     string `json:"tool_type"`
				OASSpec      []byte `json:"oas_spec"`
				PrivacyScore int    `json:"privacy_score"`

				AuthKey        string `json:"auth_key"`
				AuthSchemaName string `json:"auth_schema_name"`
			} `json:"attributes"`
		}{
			Type: "tools",
			Attributes: struct {
				Name         string `json:"name"`
				Description  string `json:"description"`
				ToolType     string `json:"tool_type"`
				OASSpec      []byte `json:"oas_spec"`
				PrivacyScore int    `json:"privacy_score"`

				AuthKey        string `json:"auth_key"`
				AuthSchemaName string `json:"auth_schema_name"`
			}{
				Name:         "Test Tool",
				Description:  "A test tool",
				ToolType:     models.ToolTypeREST,
				OASSpec:      []byte(`{"openapi": "3.0.0"}`),
				PrivacyScore: 8,
			},
		},
	}

	w := performRequest(api.router, "POST", "/api/v1/tools", createToolInput)
	assert.Equal(t, http.StatusCreated, w.Code)

	var response map[string]ToolResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "Test Tool", response["data"].Attributes.Name)

	toolID := response["data"].ID

	// Test Get Tool
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/tools/%s", toolID), nil)
	assert.Equal(t, http.StatusOK, w.Code)

	// Test Update Tool
	updateToolInput := ToolInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				Name         string `json:"name"`
				Description  string `json:"description"`
				ToolType     string `json:"tool_type"`
				OASSpec      []byte `json:"oas_spec"`
				PrivacyScore int    `json:"privacy_score"`

				AuthKey        string `json:"auth_key"`
				AuthSchemaName string `json:"auth_schema_name"`
			} `json:"attributes"`
		}{
			Type: "tools",
			Attributes: struct {
				Name         string `json:"name"`
				Description  string `json:"description"`
				ToolType     string `json:"tool_type"`
				OASSpec      []byte `json:"oas_spec"`
				PrivacyScore int    `json:"privacy_score"`

				AuthKey        string `json:"auth_key"`
				AuthSchemaName string `json:"auth_schema_name"`
			}{
				Name:         "Updated Tool",
				Description:  "An updated test tool",
				ToolType:     models.ToolTypeREST,
				OASSpec:      []byte(`{"openapi": "3.0.1"}`),
				PrivacyScore: 9,
			},
		},
	}

	w = performRequest(api.router, "PATCH", fmt.Sprintf("/api/v1/tools/%s", toolID), updateToolInput)
	assert.Equal(t, http.StatusOK, w.Code)

	// Test List Tools
	w = performRequest(api.router, "GET", "/api/v1/tools", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var listResponse map[string][]ToolResponse
	err = json.Unmarshal(w.Body.Bytes(), &listResponse)
	assert.NoError(t, err)
	assert.Len(t, listResponse["data"], 1)
	assert.Equal(t, "Updated Tool", listResponse["data"][0].Attributes.Name)

	// Test Get Tools by Type
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/tools/by-type?type=%s", models.ToolTypeREST), nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var typeResponse map[string][]ToolResponse
	err = json.Unmarshal(w.Body.Bytes(), &typeResponse)
	assert.NoError(t, err)
	assert.Len(t, typeResponse["data"], 1)
	assert.Equal(t, "Updated Tool", typeResponse["data"][0].Attributes.Name)

	// Test Search Tools
	w = performRequest(api.router, "GET", "/api/v1/tools/search?query=Updated", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var searchResponse map[string][]ToolResponse
	err = json.Unmarshal(w.Body.Bytes(), &searchResponse)
	assert.NoError(t, err)
	assert.Len(t, searchResponse["data"], 1)
	assert.Equal(t, "Updated Tool", searchResponse["data"][0].Attributes.Name)

	// Test Delete Tool
	w = performRequest(api.router, "DELETE", fmt.Sprintf("/api/v1/tools/%s", toolID), nil)
	assert.Equal(t, http.StatusNoContent, w.Code)

	// Verify tool is deleted
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/tools/%s", toolID), nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestToolEndpointsErrors(t *testing.T) {
	api, _ := setupTestAPI(t)

	// Test Get non-existent tool
	w := performRequest(api.router, "GET", "/api/v1/tools/999", nil)
	assert.Equal(t, http.StatusNotFound, w.Code)

	// Test Update non-existent tool
	updateToolInput := ToolInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				Name           string `json:"name"`
				Description    string `json:"description"`
				ToolType       string `json:"tool_type"`
				OASSpec        []byte `json:"oas_spec"`
				PrivacyScore   int    `json:"privacy_score"`
				AuthKey        string `json:"auth_key"`
				AuthSchemaName string `json:"auth_schema_name"`
			} `json:"attributes"`
		}{
			Type: "tools",
			Attributes: struct {
				Name           string `json:"name"`
				Description    string `json:"description"`
				ToolType       string `json:"tool_type"`
				OASSpec        []byte `json:"oas_spec"`
				PrivacyScore   int    `json:"privacy_score"`
				AuthKey        string `json:"auth_key"`
				AuthSchemaName string `json:"auth_schema_name"`
			}{
				Name:         "Updated Tool",
				Description:  "An updated test tool",
				ToolType:     models.ToolTypeREST,
				OASSpec:      []byte(`{"openapi": "3.0.1"}`),
				PrivacyScore: 9,
			},
		},
	}
	w = performRequest(api.router, "PATCH", "/api/v1/tools/999", updateToolInput)
	assert.Equal(t, http.StatusNotFound, w.Code)

	// Test Delete non-existent tool
	w = performRequest(api.router, "DELETE", "/api/v1/tools/999", nil)
	assert.Equal(t, http.StatusNotFound, w.Code)

	// Test Create tool with invalid input
	invalidCreateToolInput := ToolInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				Name         string `json:"name"`
				Description  string `json:"description"`
				ToolType     string `json:"tool_type"`
				OASSpec      []byte `json:"oas_spec"`
				PrivacyScore int    `json:"privacy_score"`

				AuthKey        string `json:"auth_key"`
				AuthSchemaName string `json:"auth_schema_name"`
			} `json:"attributes"`
		}{
			Type: "tools",
			Attributes: struct {
				Name         string `json:"name"`
				Description  string `json:"description"`
				ToolType     string `json:"tool_type"`
				OASSpec      []byte `json:"oas_spec"`
				PrivacyScore int    `json:"privacy_score"`

				AuthKey        string `json:"auth_key"`
				AuthSchemaName string `json:"auth_schema_name"`
			}{
				Name:         "",
				Description:  "",
				ToolType:     "",
				OASSpec:      nil,
				PrivacyScore: -1,
			},
		},
	}
	w = performRequest(api.router, "POST", "/api/v1/tools", invalidCreateToolInput)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// Test Get tools by invalid type
	w = performRequest(api.router, "GET", "/api/v1/tools/by-type?type=INVALID_TYPE", nil)
	assert.Equal(t, http.StatusOK, w.Code) // This should return an empty list, not an error

	var emptyResponse map[string][]ToolResponse
	err := json.Unmarshal(w.Body.Bytes(), &emptyResponse)
	assert.NoError(t, err)
	assert.Len(t, emptyResponse["data"], 0)
}

func TestModelPriceEndpoints(t *testing.T) {
	api, _ := setupTestAPI(t)

	// Test Create ModelPrice
	createModelPriceInput := ModelPriceInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				ModelName string  `json:"model_name"`
				Vendor    string  `json:"vendor"`
				CPT       float64 `json:"cpt"`
				Currency  string  `json:"currency"`
			} `json:"attributes"`
		}{
			Type: "model-prices",
			Attributes: struct {
				ModelName string  `json:"model_name"`
				Vendor    string  `json:"vendor"`
				CPT       float64 `json:"cpt"`
				Currency  string  `json:"currency"`
			}{
				ModelName: "GPT-3",
				Vendor:    "OpenAI",
				CPT:       0.002,
				Currency:  "USD",
			},
		},
	}

	w := performRequest(api.router, "POST", "/api/v1/model-prices", createModelPriceInput)
	assert.Equal(t, http.StatusCreated, w.Code)

	var response map[string]ModelPriceResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "GPT-3", response["data"].Attributes.ModelName)
	assert.Equal(t, "OpenAI", response["data"].Attributes.Vendor)
	assert.Equal(t, 0.002, response["data"].Attributes.CPT)

	modelPriceID := response["data"].ID

	// Test Get ModelPrice
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/model-prices/%s", modelPriceID), nil)
	assert.Equal(t, http.StatusOK, w.Code)

	// Test Update ModelPrice
	updateModelPriceInput := ModelPriceInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				ModelName string  `json:"model_name"`
				Vendor    string  `json:"vendor"`
				CPT       float64 `json:"cpt"`
				Currency  string  `json:"currency"`
			} `json:"attributes"`
		}{
			Type: "model-prices",
			Attributes: struct {
				ModelName string  `json:"model_name"`
				Vendor    string  `json:"vendor"`
				CPT       float64 `json:"cpt"`
				Currency  string  `json:"currency"`
			}{
				ModelName: "GPT-3",
				Vendor:    "OpenAI",
				CPT:       0.003,
				Currency:  "USD",
			},
		},
	}

	w = performRequest(api.router, "PATCH", fmt.Sprintf("/api/v1/model-prices/%s", modelPriceID), updateModelPriceInput)
	assert.Equal(t, http.StatusOK, w.Code)

	var updateResponse map[string]ModelPriceResponse
	err = json.Unmarshal(w.Body.Bytes(), &updateResponse)
	assert.NoError(t, err)
	assert.Equal(t, 0.003, updateResponse["data"].Attributes.CPT)

	// Test List ModelPrices
	w = performRequest(api.router, "GET", "/api/v1/model-prices", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var listResponse map[string][]ModelPriceResponse
	err = json.Unmarshal(w.Body.Bytes(), &listResponse)
	assert.NoError(t, err)
	assert.Len(t, listResponse["data"], 1)
	assert.Equal(t, "GPT-3", listResponse["data"][0].Attributes.ModelName)

	// Test Get ModelPrices by Vendor
	w = performRequest(api.router, "GET", "/api/v1/model-prices/by-vendor?vendor=OpenAI", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var vendorResponse map[string][]ModelPriceResponse
	err = json.Unmarshal(w.Body.Bytes(), &vendorResponse)
	assert.NoError(t, err)
	assert.Len(t, vendorResponse["data"], 1)
	assert.Equal(t, "OpenAI", vendorResponse["data"][0].Attributes.Vendor)

	// Test Delete ModelPrice
	w = performRequest(api.router, "DELETE", fmt.Sprintf("/api/v1/model-prices/%s", modelPriceID), nil)
	assert.Equal(t, http.StatusNoContent, w.Code)

	// Verify ModelPrice is deleted
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/model-prices/%s", modelPriceID), nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestModelPriceEndpointsErrors(t *testing.T) {
	api, _ := setupTestAPI(t)

	// Test Get non-existent ModelPrice
	w := performRequest(api.router, "GET", "/api/v1/model-prices/999", nil)
	assert.Equal(t, http.StatusNotFound, w.Code)

	// Test Update non-existent ModelPrice
	updateModelPriceInput := ModelPriceInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				ModelName string  `json:"model_name"`
				Vendor    string  `json:"vendor"`
				CPT       float64 `json:"cpt"`
				Currency  string  `json:"currency"`
			} `json:"attributes"`
		}{
			Type: "model-prices",
			Attributes: struct {
				ModelName string  `json:"model_name"`
				Vendor    string  `json:"vendor"`
				CPT       float64 `json:"cpt"`
				Currency  string  `json:"currency"`
			}{
				ModelName: "GPT-3",
				Vendor:    "OpenAI",
				CPT:       0.003,
				Currency:  "USD",
			},
		},
	}
	w = performRequest(api.router, "PATCH", "/api/v1/model-prices/999", updateModelPriceInput)
	assert.Equal(t, http.StatusNotFound, w.Code)

	// Test Delete non-existent ModelPrice
	w = performRequest(api.router, "DELETE", "/api/v1/model-prices/999", nil)
	assert.Equal(t, http.StatusInternalServerError, w.Code)

	// Test Create ModelPrice with invalid input
	invalidCreateModelPriceInput := ModelPriceInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				ModelName string  `json:"model_name"`
				Vendor    string  `json:"vendor"`
				CPT       float64 `json:"cpt"`
				Currency  string  `json:"currency"`
			} `json:"attributes"`
		}{
			Type: "model-prices",
			Attributes: struct {
				ModelName string  `json:"model_name"`
				Vendor    string  `json:"vendor"`
				CPT       float64 `json:"cpt"`
				Currency  string  `json:"currency"`
			}{
				ModelName: "",
				Vendor:    "",
				CPT:       -1,
				Currency:  "USD",
			},
		},
	}
	w = performRequest(api.router, "POST", "/api/v1/model-prices", invalidCreateModelPriceInput)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// Test Get ModelPrices by non-existent vendor
	w = performRequest(api.router, "GET", "/api/v1/model-prices/by-vendor?vendor=NonExistentVendor", nil)
	assert.Equal(t, http.StatusOK, w.Code) // This should return an empty list, not an error

	var emptyResponse map[string][]ModelPriceResponse
	err := json.Unmarshal(w.Body.Bytes(), &emptyResponse)
	assert.NoError(t, err)
	assert.Len(t, emptyResponse["data"], 0)
}
