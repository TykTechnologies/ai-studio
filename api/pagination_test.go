package api_test

import (
	"encoding/json"
	"net/http"
	gotest "testing"

	"github.com/TykTechnologies/midsommar/v2/api"
	apitest "github.com/TykTechnologies/midsommar/v2/api/testing"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
)

func TestPagination_LLMPagination(t *gotest.T) {
	db := apitest.SetupTestDB(t)
	service := apitest.SetupTestService(db)
	config := apitest.SetupTestAuthConfig(db, service)
	authService := apitest.SetupTestAuthService(db, service)
	licenser := apitest.SetupTestLicenser()
	a := api.NewAPI(service, true, authService, config, nil, apitest.EmptyFile, licenser)

	// Create test LLMs
	llms := []models.LLM{
		{
			Name:          "LLM1",
			APIKey:        "key1",
			APIEndpoint:   "https://api1.com",
			PrivacyScore:  30,
			AllowedModels: []string{"gpt-4"},
		},
		{
			Name:          "LLM2",
			APIKey:        "key2",
			APIEndpoint:   "https://api2.com",
			PrivacyScore:  50,
			AllowedModels: []string{"gpt-4.*", "gpt-3.5-turbo"},
		},
		{
			Name:          "LLM3",
			APIKey:        "key3",
			APIEndpoint:   "https://api3.com",
			PrivacyScore:  70,
			AllowedModels: []string{"claude-.*"},
		},
		{
			Name:          "LLM4",
			APIKey:        "key4",
			APIEndpoint:   "https://api4.com",
			PrivacyScore:  90,
			AllowedModels: nil,
		},
		{
			Name:          "LLM5",
			APIKey:        "key5",
			APIEndpoint:   "https://api5.com",
			PrivacyScore:  100,
			AllowedModels: []string{"gpt-4"},
		},
	}

	for _, llm := range llms {
		err := db.Create(&llm).Error
		assert.NoError(t, err)
	}

	// Test first page
	w := apitest.PerformRequest(a.Router(), "GET", "/api/v1/llms?page=1&page_size=2", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string][]api.LLMResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Len(t, response["data"], 2)
	assert.Equal(t, "3", w.Header().Get("X-Total-Pages"))
	assert.Equal(t, "5", w.Header().Get("X-Total-Count")) // 5 test LLMs

	// Test second page
	w = apitest.PerformRequest(a.Router(), "GET", "/api/v1/llms?page=2&page_size=2", nil)
	assert.Equal(t, http.StatusOK, w.Code)
	err = json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Len(t, response["data"], 2)

	// Test last page
	w = apitest.PerformRequest(a.Router(), "GET", "/api/v1/llms?page=3&page_size=2", nil)
	assert.Equal(t, http.StatusOK, w.Code)
	err = json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Len(t, response["data"], 1)

	// Test page out of range
	w = apitest.PerformRequest(a.Router(), "GET", "/api/v1/llms?page=4&page_size=2", nil)
	assert.Equal(t, http.StatusOK, w.Code)
	err = json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Len(t, response["data"], 0)

	// Test invalid page number
	w = apitest.PerformRequest(a.Router(), "GET", "/api/v1/llms?page=invalid&page_size=2", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestPagination_UserPagination(t *gotest.T) {
	db := apitest.SetupTestDB(t)
	service := apitest.SetupTestService(db)
	config := apitest.SetupTestAuthConfig(db, service)
	authService := apitest.SetupTestAuthService(db, service)
	licenser := apitest.SetupTestLicenser()
	a := api.NewAPI(service, true, authService, config, nil, apitest.EmptyFile, licenser)

	// Create test users
	users := []models.User{
		{
			Email:    "user1@test.com",
			Password: "password1",
		},
		{
			Email:    "user2@test.com",
			Password: "password2",
		},
		{
			Email:    "user3@test.com",
			Password: "password3",
		},
		{
			Email:    "user4@test.com",
			Password: "password4",
		},
		{
			Email:    "user5@test.com",
			Password: "password5",
		},
	}

	for _, user := range users {
		err := db.Create(&user).Error
		assert.NoError(t, err)
	}

	// Test first page
	w := apitest.PerformRequest(a.Router(), "GET", "/api/v1/users?page=1&page_size=2", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string][]api.UserResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Len(t, response["data"], 2)
	assert.Equal(t, "3", w.Header().Get("X-Total-Pages"))
	assert.Equal(t, "5", w.Header().Get("X-Total-Count")) // 5 test users

	// Test second page
	w = apitest.PerformRequest(a.Router(), "GET", "/api/v1/users?page=2&page_size=2", nil)
	assert.Equal(t, http.StatusOK, w.Code)
	err = json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Len(t, response["data"], 2)

	// Test last page
	w = apitest.PerformRequest(a.Router(), "GET", "/api/v1/users?page=3&page_size=2", nil)
	assert.Equal(t, http.StatusOK, w.Code)
	err = json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Len(t, response["data"], 1)

	// Test page out of range
	w = apitest.PerformRequest(a.Router(), "GET", "/api/v1/users?page=4&page_size=2", nil)
	assert.Equal(t, http.StatusOK, w.Code)
	err = json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Len(t, response["data"], 0)

	// Test invalid page number
	w = apitest.PerformRequest(a.Router(), "GET", "/api/v1/users?page=invalid&page_size=2", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}
