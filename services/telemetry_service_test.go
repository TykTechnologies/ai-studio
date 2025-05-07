package services

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupTestDBForTelemetry creates an in-memory SQLite database for telemetry tests
func setupTestDBForTelemetry(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	err = models.InitModels(db)
	assert.NoError(t, err)

	return db
}

func TestGetLLMStats(t *testing.T) {
	db := setupTestDBForTelemetry(t)
	telemetryService := NewTelemetryService(db)

	// Create test LLMs directly in the database
	llm1 := &models.LLM{
		Name:             "LLM1",
		ShortDescription: "Short1",
		LongDescription:  "Long1",
		APIEndpoint:      "https://api1.com",
		LogoURL:          "https://logo1.com",
		PrivacyScore:     80,
		Vendor:           models.OPENAI,
		Active:           true,
	}
	err := db.Create(llm1).Error
	assert.NoError(t, err)

	llm2 := &models.LLM{
		Name:             "LLM2",
		ShortDescription: "Short2",
		LongDescription:  "Long2",
		APIEndpoint:      "https://api2.com",
		LogoURL:          "https://logo2.com",
		PrivacyScore:     90,
		Vendor:           models.OPENAI,
		Active:           true,
	}
	err = db.Create(llm2).Error
	assert.NoError(t, err)

	// Test getting stats
	stats, err := telemetryService.GetLLMStats()
	assert.NoError(t, err)

	// Verify LLM count is 2
	assert.Equal(t, int64(2), stats["llms_count"])

	// Verify total tokens key exists
	assert.Contains(t, stats, "total_tokens")
}

func TestGetAppStats(t *testing.T) {
	db := setupTestDBForTelemetry(t)
	telemetryService := NewTelemetryService(db)

	// Create test apps directly in the database
	app1 := &models.App{
		Name:        "App 1",
		Description: "Description 1",
	}
	err := db.Create(app1).Error
	assert.NoError(t, err)

	app2 := &models.App{
		Name:        "App 2",
		Description: "Description 2",
	}
	err = db.Create(app2).Error
	assert.NoError(t, err)

	// Test getting stats
	stats, err := telemetryService.GetAppStats()
	assert.NoError(t, err)

	// Verify app count is 2
	assert.Equal(t, int64(2), stats["apps_count"])

	// Verify total tokens key exists
	assert.Contains(t, stats, "total_tokens")
}

func TestGetUserStats(t *testing.T) {
	db := setupTestDBForTelemetry(t)
	telemetryService := NewTelemetryService(db)

	// Create users of different types directly in the database
	regularUser := &models.User{
		Email:         "regular@example.com",
		Name:          "Regular User",
		IsAdmin:       false,
		ShowPortal:    false,
		ShowChat:      true,
		EmailVerified: true,
	}
	err := db.Create(regularUser).Error
	assert.NoError(t, err)

	adminUser := &models.User{
		Email:         "admin@example.com",
		Name:          "Admin User",
		IsAdmin:       true,
		ShowPortal:    true,
		ShowChat:      true,
		EmailVerified: true,
	}
	err = db.Create(adminUser).Error
	assert.NoError(t, err)

	devUser := &models.User{
		Email:         "dev@example.com",
		Name:          "Developer User",
		IsAdmin:       false,
		ShowPortal:    true,
		ShowChat:      true,
		EmailVerified: true,
	}
	err = db.Create(devUser).Error
	assert.NoError(t, err)

	chatUser := &models.User{
		Email:         "chat@example.com",
		Name:          "Chat User",
		IsAdmin:       false,
		ShowPortal:    false,
		ShowChat:      true,
		EmailVerified: true,
	}
	err = db.Create(chatUser).Error
	assert.NoError(t, err)

	// Create a user group
	group := &models.Group{
		Name: "Test Group",
	}
	err = db.Create(group).Error
	assert.NoError(t, err)

	// Test getting stats
	stats, err := telemetryService.GetUserStats()
	assert.NoError(t, err)

	// Verify user counts
	assert.Equal(t, int64(4), stats["users_count"])
	assert.Equal(t, int64(1), stats["admin_users"])
	assert.Equal(t, int64(1), stats["developers"])
	assert.Equal(t, int64(2), stats["chat_users"])
	assert.Equal(t, int64(1), stats["user_groups"])
}

func TestGetChatStats(t *testing.T) {
	db := setupTestDBForTelemetry(t)
	telemetryService := NewTelemetryService(db)

	// Create test chats
	chat1 := &models.Chat{
		Name:        "Chat 1",
		Description: "Chat Description 1",
	}
	err := db.Create(chat1).Error
	assert.NoError(t, err)

	chat2 := &models.Chat{
		Name:        "Chat 2",
		Description: "Chat Description 2",
	}
	err = db.Create(chat2).Error
	assert.NoError(t, err)

	// Test getting stats
	stats, err := telemetryService.GetChatStats()
	assert.NoError(t, err)

	// Verify chat count is 2
	assert.Equal(t, int64(2), stats["chats_count"])

	// Verify total tokens key exists
	assert.Contains(t, stats, "total_tokens")
}

func TestTelemetryServiceErrorCases(t *testing.T) {
	// Create a DB but don't initialize models to cause errors in queries
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	telemetryService := NewTelemetryService(db)

	// Test each method for error handling
	_, err = telemetryService.GetLLMStats()
	assert.Error(t, err)

	_, err = telemetryService.GetAppStats()
	assert.Error(t, err)

	_, err = telemetryService.GetUserStats()
	assert.Error(t, err)

	_, err = telemetryService.GetChatStats()
	assert.Error(t, err)
}
