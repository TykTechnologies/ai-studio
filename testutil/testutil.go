package testutil

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/api"
	apitest "github.com/TykTechnologies/midsommar/v2/api/testing"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	"gorm.io/gorm"
)

var emptyFile embed.FS

func SetupTestAPI(t *testing.T) (*api.API, *gorm.DB) {
	db := apitest.SetupTestDB(t)
	service := apitest.SetupTestService(db)
	config := apitest.SetupTestAuthConfig(db, service)
	authService := apitest.SetupTestAuthService(db, service)
	a := api.NewAPI(service, true, authService, config, nil, emptyFile, nil)

	// Initialize test data
	if err := SetupTestData(db, service); err != nil {
		t.Fatalf("Failed to setup test data: %v", err)
	}

	return a, db
}

// SetupTestData initializes test data in the database
func SetupTestData(db *gorm.DB, service *services.Service) error {
	// Create test users
	users := []models.User{
		{
			Email:         "test@test.com",
			Name:          "Test User",
			IsAdmin:       true,
			EmailVerified: true,
			ShowPortal:    true,
			ShowChat:      true,
		},
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
		if err := user.Create(db); err != nil {
			return fmt.Errorf("failed to create test user: %w", err)
		}
	}

	// Create default group
	defaultGroup := &models.Group{
		Name: "Default",
	}
	if err := db.Create(defaultGroup).Error; err != nil {
		return fmt.Errorf("failed to create default group: %w", err)
	}

	// Add first user to default group
	if err := service.AddUserToGroup(users[0].ID, defaultGroup.ID); err != nil {
		return fmt.Errorf("failed to add user to default group: %w", err)
	}

	// Create default LLM settings
	llmSettings := models.LLMSettings{
		ModelName:   "claude-3-sonnet-20240229",
		MaxTokens:   4000,
		Temperature: 0.7,
	}
	if err := db.Create(&llmSettings).Error; err != nil {
		return fmt.Errorf("failed to create LLM settings: %w", err)
	}

	// Create default LLM first
	defaultLLM := models.LLM{
		Name:         "Default Anthropic",
		Vendor:       "anthropic",
		Active:       true,
		APIEndpoint:  "https://api.anthropic.com",
		PrivacyScore: 75,
	}
	if err := db.Create(&defaultLLM).Error; err != nil {
		return fmt.Errorf("failed to create default LLM: %w", err)
	}

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
			Name:         "LLM4",
			APIKey:       "key4",
			APIEndpoint:  "https://api4.com",
			PrivacyScore: 90,
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
		if err := db.Create(&llm).Error; err != nil {
			return fmt.Errorf("failed to create LLM: %w", err)
		}
	}

	// Create default chat
	chat := &models.Chat{
		Name:          "Default Chat",
		Groups:        []models.Group{*defaultGroup},
		SupportsTools: true,
		SystemPrompt:  "You are a helpful assistant.",
		LLMSettingsID: llmSettings.ID,
		LLMID:         defaultLLM.ID,
	}
	if err := chat.Create(db); err != nil {
		return fmt.Errorf("failed to create default chat: %w", err)
	}

	return nil
}

func PerformRequest(r http.Handler, method, path string, body interface{}) *httptest.ResponseRecorder {
	var reqBody []byte
	if body != nil {
		reqBody, _ = json.Marshal(body)
	}
	req, _ := http.NewRequest(method, path, bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}
