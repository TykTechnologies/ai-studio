package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/auth"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestPagination_LLMPagination(t *testing.T) {
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

	api := NewAPI(service, true, auth.NewAuthService(config, newMockMailer()), nil)

	// Create multiple LLMs
	for i := 1; i <= 15; i++ {
		_, err := service.CreateLLM(
			fmt.Sprintf("LLM %d", i),
			"api-key",
			"https://api.test.com",
			75,
			"Short desc",
			"Long desc",
			"https://logo.test",
			models.OPENAI,
			true,
		)
		assert.NoError(t, err)
	}

	// Test pagination
	w := performRequest(api.router, "GET", "/api/v1/llms?page=1&page_size=5", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string][]LLMResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Len(t, response["data"], 5)
	assert.Equal(t, "15", w.Header().Get("X-Total-Count"))
	assert.Equal(t, "3", w.Header().Get("X-Total-Pages"))

	// Test second page
	w = performRequest(api.router, "GET", "/api/v1/llms?page=2&page_size=5", nil)
	assert.Equal(t, http.StatusOK, w.Code)
	err = json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Len(t, response["data"], 5)

	// Test last page
	w = performRequest(api.router, "GET", "/api/v1/llms?page=3&page_size=5", nil)
	assert.Equal(t, http.StatusOK, w.Code)
	err = json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Len(t, response["data"], 5)

	// Test exceeding pages
	w = performRequest(api.router, "GET", "/api/v1/llms?page=4&page_size=5", nil)
	assert.Equal(t, http.StatusOK, w.Code)
	err = json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Len(t, response["data"], 0)

	// Test 'all' parameter
	w = performRequest(api.router, "GET", "/api/v1/llms?all=true", nil)
	assert.Equal(t, http.StatusOK, w.Code)
	err = json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Len(t, response["data"], 15)

	// Test without page_size parameter
	w = performRequest(api.router, "GET", "/api/v1/llms?page=1", nil)
	assert.Equal(t, http.StatusOK, w.Code)
	err = json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Len(t, response["data"], 15)
	assert.Equal(t, "15", w.Header().Get("X-Total-Count"))

	// Test without both page_size and page_number parameters
	w = performRequest(api.router, "GET", "/api/v1/llms", nil)
	assert.Equal(t, http.StatusOK, w.Code)
	err = json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Len(t, response["data"], 15)
	assert.Equal(t, "15", w.Header().Get("X-Total-Count"))
}

func TestPagination_UserPagination(t *testing.T) {
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

	api := NewAPI(service, true, auth.NewAuthService(config, newMockMailer()), nil)

	// Create multiple users
	for i := 1; i <= 15; i++ {
		_, err := service.CreateUser(
			fmt.Sprintf("user%d@example.com", i),
			fmt.Sprintf("User %d", i),
			"password123",
			false,
		)
		assert.NoError(t, err)
	}

	// Test pagination
	w := performRequest(api.router, "GET", "/api/v1/users?page=1&page_size=5", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string][]UserResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Len(t, response["data"], 5)
	assert.Equal(t, "15", w.Header().Get("X-Total-Count"))
	assert.Equal(t, "3", w.Header().Get("X-Total-Pages"))

	// Test second page
	w = performRequest(api.router, "GET", "/api/v1/users?page=2&page_size=5", nil)
	assert.Equal(t, http.StatusOK, w.Code)
	err = json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Len(t, response["data"], 5)

	// Test last page
	w = performRequest(api.router, "GET", "/api/v1/users?page=3&page_size=5", nil)
	assert.Equal(t, http.StatusOK, w.Code)
	err = json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Len(t, response["data"], 5)

	// Test exceeding pages
	w = performRequest(api.router, "GET", "/api/v1/users?page=4&page_size=5", nil)
	assert.Equal(t, http.StatusOK, w.Code)
	err = json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Len(t, response["data"], 0)

	// Test 'all' parameter
	w = performRequest(api.router, "GET", "/api/v1/users?all=true", nil)
	assert.Equal(t, http.StatusOK, w.Code)
	err = json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Len(t, response["data"], 15)
}
