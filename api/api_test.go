package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

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
	api := NewAPI(service)

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
			Type       string "json:\"type\""
			Attributes struct {
				Email    string "json:\"email\""
				Name     string "json:\"name\""
				Password string "json:\"password,omitempty\""
			} "json:\"attributes\""
		}{
			Type: "users",
			Attributes: struct {
				Email    string "json:\"email\""
				Name     string "json:\"name\""
				Password string "json:\"password,omitempty\""
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
			Type       string "json:\"type\""
			Attributes struct {
				Email    string "json:\"email\""
				Name     string "json:\"name\""
				Password string "json:\"password,omitempty\""
			} "json:\"attributes\""
		}{
			Type: "users",
			Attributes: struct {
				Email    string "json:\"email\""
				Name     string "json:\"name\""
				Password string "json:\"password,omitempty\""
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
			Type       string "json:\"type\""
			Attributes struct {
				Name string "json:\"name\""
			} "json:\"attributes\""
		}{
			Type: "groups",
			Attributes: struct {
				Name string "json:\"name\""
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
			Type       string "json:\"type\""
			Attributes struct {
				Name string "json:\"name\""
			} "json:\"attributes\""
		}{
			Type: "groups",
			Attributes: struct {
				Name string "json:\"name\""
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
			Type       string "json:\"type\""
			Attributes struct {
				Email    string "json:\"email\""
				Name     string "json:\"name\""
				Password string "json:\"password,omitempty\""
			} "json:\"attributes\""
		}{
			Type: "users",
			Attributes: struct {
				Email    string "json:\"email\""
				Name     string "json:\"name\""
				Password string "json:\"password,omitempty\""
			}{
				Email:    "groupuser@example.com",
				Name:     "Group User",
				Password: "password123",
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
			Type string "json:\"type\""
			ID   string "json:\"id\""
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

	// Test Delete Group
	w = performRequest(api.router, "DELETE", fmt.Sprintf("/api/v1/groups/%s", groupID), nil)
	assert.Equal(t, http.StatusNoContent, w.Code)
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
				Name              string `json:"name"`
				APIKey            string `json:"api_key"`
				APIEndpoint       string `json:"api_endpoint"`
				StreamingEndpoint string `json:"streaming_endpoint"`
				PrivacyScore      int    `json:"privacy_score"`
				ShortDescription  string `json:"short_description"`
				LongDescription   string `json:"long_description"`
				ExternalURL       string `json:"external_url"`
				LogoURL           string `json:"logo_url"`
			} `json:"attributes"`
		}{
			Type: "llms",
			Attributes: struct {
				Name              string `json:"name"`
				APIKey            string `json:"api_key"`
				APIEndpoint       string `json:"api_endpoint"`
				StreamingEndpoint string `json:"streaming_endpoint"`
				PrivacyScore      int    `json:"privacy_score"`
				ShortDescription  string `json:"short_description"`
				LongDescription   string `json:"long_description"`
				ExternalURL       string `json:"external_url"`
				LogoURL           string `json:"logo_url"`
			}{
				Name:              "Test LLM",
				APIKey:            "test-api-key",
				APIEndpoint:       "https://api.test.com",
				StreamingEndpoint: "https://streaming.test.com",
				PrivacyScore:      75,
				ShortDescription:  "A test LLM",
				LongDescription:   "This is a test LLM for API testing",
				ExternalURL:       "https://testllm.com",
				LogoURL:           "https://testllm.com/logo.png",
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
				Name              string `json:"name"`
				APIKey            string `json:"api_key"`
				APIEndpoint       string `json:"api_endpoint"`
				StreamingEndpoint string `json:"streaming_endpoint"`
				PrivacyScore      int    `json:"privacy_score"`
				ShortDescription  string `json:"short_description"`
				LongDescription   string `json:"long_description"`
				ExternalURL       string `json:"external_url"`
				LogoURL           string `json:"logo_url"`
			} `json:"attributes"`
		}{
			Type: "llms",
			Attributes: struct {
				Name              string `json:"name"`
				APIKey            string `json:"api_key"`
				APIEndpoint       string `json:"api_endpoint"`
				StreamingEndpoint string `json:"streaming_endpoint"`
				PrivacyScore      int    `json:"privacy_score"`
				ShortDescription  string `json:"short_description"`
				LongDescription   string `json:"long_description"`
				ExternalURL       string `json:"external_url"`
				LogoURL           string `json:"logo_url"`
			}{
				Name:              "Updated Test LLM",
				APIKey:            "updated-api-key",
				APIEndpoint:       "https://updated-api.test.com",
				StreamingEndpoint: "https://updated-streaming.test.com",
				PrivacyScore:      80,
				ShortDescription:  "An updated test LLM",
				LongDescription:   "This is an updated test LLM for API testing",
				ExternalURL:       "https://updatedtestllm.com",
				LogoURL:           "https://updatedtestllm.com/logo.png",
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
		{Name: "LLM1", APIKey: "key1", APIEndpoint: "https://api1.com", StreamingEndpoint: "https://streaming1.com", PrivacyScore: 30},
		{Name: "LLM2", APIKey: "key2", APIEndpoint: "https://api2.com", StreamingEndpoint: "https://streaming2.com", PrivacyScore: 50},
		{Name: "LLM3", APIKey: "key3", APIEndpoint: "https://api3.com", StreamingEndpoint: "https://streaming3.com", PrivacyScore: 70},
		{Name: "LLM4", APIKey: "key4", APIEndpoint: "https://api4.com", StreamingEndpoint: "https://streaming4.com", PrivacyScore: 90},
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
				Name              string `json:"name"`
				APIKey            string `json:"api_key"`
				APIEndpoint       string `json:"api_endpoint"`
				StreamingEndpoint string `json:"streaming_endpoint"`
				PrivacyScore      int    `json:"privacy_score"`
				ShortDescription  string `json:"short_description"`
				LongDescription   string `json:"long_description"`
				ExternalURL       string `json:"external_url"`
				LogoURL           string `json:"logo_url"`
			} `json:"attributes"`
		}{
			Type: "llms",
			Attributes: struct {
				Name              string `json:"name"`
				APIKey            string `json:"api_key"`
				APIEndpoint       string `json:"api_endpoint"`
				StreamingEndpoint string `json:"streaming_endpoint"`
				PrivacyScore      int    `json:"privacy_score"`
				ShortDescription  string `json:"short_description"`
				LongDescription   string `json:"long_description"`
				ExternalURL       string `json:"external_url"`
				LogoURL           string `json:"logo_url"`
			}{
				Name:              "Test LLM",
				APIKey:            "test-api-key",
				APIEndpoint:       "https://api.test.com",
				StreamingEndpoint: "https://streaming.test.com",
				PrivacyScore:      75,
				ShortDescription:  "A test LLM",
				LongDescription:   "This is a test LLM for API testing",
				ExternalURL:       "https://testllm.com",
				LogoURL:           "https://testllm.com/logo.png",
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
	user, err := api.service.CreateUser("test@example.com", "Test User", "password123")
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
	user, err := api.service.CreateUser("test@example.com", "Test User", "password123")
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
			}{
				Name:             "Test Datasource",
				ShortDescription: "Short description",
				LongDescription:  "Long description",
				Icon:             "icon.png",
				Url:              "https://example.com",
				PrivacyScore:     75,
				UserID:           user.ID,
				Tags:             []string{"tag1", "tag2"},
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
			}{
				Name:             "Updated Datasource",
				ShortDescription: "Updated short description",
				LongDescription:  "Updated long description",
				Icon:             "updated-icon.png",
				Url:              "https://updated-example.com",
				PrivacyScore:     80,
				UserID:           user.ID,
				Tags:             []string{"tag1", "tag2", "tag3"},
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
