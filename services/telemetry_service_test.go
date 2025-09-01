package services

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
)

func TestNewTelemetryService(t *testing.T) {
	db := setupTestDB(t)

	service := NewTelemetryService(db)

	if service == nil {
		t.Fatal("Expected NewTelemetryService to return a non-nil service")
	}

	if service.DB != db {
		t.Fatal("Expected TelemetryService.DB to be the provided database")
	}
}

func TestTelemetryService_GetLLMStats(t *testing.T) {
	db := setupTestDB(t)
	service := NewTelemetryService(db)

	// Test with empty database
	stats, err := service.GetLLMStats()
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if stats == nil {
		t.Fatal("Expected stats to be non-nil")
	}

	llmCount, exists := stats["llms_count"]
	if !exists {
		t.Fatal("Expected llms_count to exist in stats")
	}

	if llmCount != int64(0) {
		t.Fatalf("Expected llms_count to be 0, got: %v", llmCount)
	}

	totalTokens, exists := stats["total_tokens"]
	if !exists {
		t.Fatal("Expected total_tokens to exist in stats")
	}

	if totalTokens != int64(0) {
		t.Fatalf("Expected total_tokens to be 0, got: %v", totalTokens)
	}
}

func TestTelemetryService_GetAppStats(t *testing.T) {
	db := setupTestDB(t)
	service := NewTelemetryService(db)

	// Test with empty database
	stats, err := service.GetAppStats()
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if stats == nil {
		t.Fatal("Expected stats to be non-nil")
	}

	appCount, exists := stats["apps_count"]
	if !exists {
		t.Fatal("Expected apps_count to exist in stats")
	}

	if appCount != int64(0) {
		t.Fatalf("Expected apps_count to be 0, got: %v", appCount)
	}

	totalTokens, exists := stats["total_tokens"]
	if !exists {
		t.Fatal("Expected total_tokens to exist in stats")
	}

	if totalTokens != int64(0) {
		t.Fatalf("Expected total_tokens to be 0, got: %v", totalTokens)
	}
}

func TestTelemetryService_GetUserStats(t *testing.T) {
	db := setupTestDB(t)
	service := NewTelemetryService(db)

	// Test with database
	stats, err := service.GetUserStats()
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if stats == nil {
		t.Fatal("Expected stats to be non-nil")
	}

	expectedKeys := []string{"users_count", "admin_users", "developers", "chat_users", "user_groups"}
	for _, key := range expectedKeys {
		if _, exists := stats[key]; !exists {
			t.Fatalf("Expected %s to exist in stats", key)
		}
	}

	// Verify we get numeric values (counts can vary due to existing data)
	usersCount := stats["users_count"]
	if _, ok := usersCount.(int64); !ok {
		t.Fatalf("Expected users_count to be int64, got: %T", usersCount)
	}
}

func TestTelemetryService_GetChatStats(t *testing.T) {
	db := setupTestDB(t)
	service := NewTelemetryService(db)

	// Test with empty database
	stats, err := service.GetChatStats()
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if stats == nil {
		t.Fatal("Expected stats to be non-nil")
	}

	chatsCount, exists := stats["chats_count"]
	if !exists {
		t.Fatal("Expected chats_count to exist in stats")
	}

	if chatsCount != int64(0) {
		t.Fatalf("Expected chats_count to be 0, got: %v", chatsCount)
	}

	totalTokens, exists := stats["total_tokens"]
	if !exists {
		t.Fatal("Expected total_tokens to exist in stats")
	}

	if totalTokens != int64(0) {
		t.Fatalf("Expected total_tokens to be 0, got: %v", totalTokens)
	}
}

func TestTelemetryService_GetUserStatsWithData(t *testing.T) {
	db := setupTestDB(t)
	service := NewTelemetryService(db)

	// Get initial counts to account for existing data
	initialStats, err := service.GetUserStats()
	if err != nil {
		t.Fatalf("Expected no error getting initial stats, got: %v", err)
	}
	initialUserCount := initialStats["users_count"].(int64)
	initialGroupCount := initialStats["user_groups"].(int64)

	// Create some test users
	users := []models.User{
		{Email: "admin@test.com", IsAdmin: true},
		{Email: "dev@test.com", ShowPortal: true},
		{Email: "user@test.com", ShowChat: true},
	}

	for _, user := range users {
		if err := db.Create(&user).Error; err != nil {
			t.Fatalf("Failed to create test user: %v", err)
		}
	}

	// Create a test group
	group := models.Group{Name: "test-group"}
	if err := db.Create(&group).Error; err != nil {
		t.Fatalf("Failed to create test group: %v", err)
	}

	stats, err := service.GetUserStats()
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	usersCount := stats["users_count"]
	expectedUserCount := initialUserCount + 3
	if usersCount != expectedUserCount {
		t.Fatalf("Expected users_count to be %d, got: %v", expectedUserCount, usersCount)
	}

	userGroups := stats["user_groups"]
	expectedGroupCount := initialGroupCount + 1
	if userGroups != expectedGroupCount {
		t.Fatalf("Expected user_groups to be %d, got: %v", expectedGroupCount, userGroups)
	}
}

func TestTelemetryService_GetLLMStatsWithData(t *testing.T) {
	db := setupTestDB(t)
	service := NewTelemetryService(db)

	// Create some test LLMs
	llms := []models.LLM{
		{Name: "gpt-4", Vendor: "openai"},
		{Name: "claude-3", Vendor: "anthropic"},
	}

	for _, llm := range llms {
		if err := db.Create(&llm).Error; err != nil {
			t.Fatalf("Failed to create test LLM: %v", err)
		}
	}

	// Create some test chat records to generate token usage
	chatRecords := []models.LLMChatRecord{
		{TotalTokens: 100, LLMID: llms[0].ID},
		{TotalTokens: 200, LLMID: llms[1].ID},
	}

	for _, record := range chatRecords {
		if err := db.Create(&record).Error; err != nil {
			t.Fatalf("Failed to create test chat record: %v", err)
		}
	}

	stats, err := service.GetLLMStats()
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	llmCount := stats["llms_count"]
	if llmCount != int64(2) {
		t.Fatalf("Expected llms_count to be 2, got: %v", llmCount)
	}

	totalTokens := stats["total_tokens"]
	if totalTokens != int64(300) {
		t.Fatalf("Expected total_tokens to be 300, got: %v", totalTokens)
	}
}

func TestTelemetryService_GetAppStatsWithData(t *testing.T) {
	db := setupTestDB(t)
	service := NewTelemetryService(db)

	// Create some test apps
	apps := []models.App{
		{Name: "test-app-1"},
		{Name: "test-app-2"},
	}

	for _, app := range apps {
		if err := db.Create(&app).Error; err != nil {
			t.Fatalf("Failed to create test app: %v", err)
		}
	}

	// Create some test proxy interactions
	proxyRecords := []models.LLMChatRecord{
		{TotalTokens: 150, InteractionType: models.ProxyInteraction, AppID: apps[0].ID},
		{TotalTokens: 250, InteractionType: models.ProxyInteraction, AppID: apps[1].ID},
	}

	for _, record := range proxyRecords {
		if err := db.Create(&record).Error; err != nil {
			t.Fatalf("Failed to create test proxy record: %v", err)
		}
	}

	stats, err := service.GetAppStats()
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	appCount := stats["apps_count"]
	if appCount != int64(2) {
		t.Fatalf("Expected apps_count to be 2, got: %v", appCount)
	}

	totalTokens := stats["total_tokens"]
	if totalTokens != int64(400) {
		t.Fatalf("Expected total_tokens to be 400, got: %v", totalTokens)
	}
}

func TestTelemetryService_GetChatStatsWithData(t *testing.T) {
	db := setupTestDB(t)
	service := NewTelemetryService(db)

	// Create some test chats
	chats := []models.Chat{
		{Name: "test-chat-1"},
		{Name: "test-chat-2"},
	}

	for _, chat := range chats {
		if err := db.Create(&chat).Error; err != nil {
			t.Fatalf("Failed to create test chat: %v", err)
		}
	}

	// Create some test chat interactions
	chatRecords := []models.LLMChatRecord{
		{TotalTokens: 300, InteractionType: models.ChatInteraction},
		{TotalTokens: 400, InteractionType: models.ChatInteraction},
	}

	for _, record := range chatRecords {
		if err := db.Create(&record).Error; err != nil {
			t.Fatalf("Failed to create test chat record: %v", err)
		}
	}

	stats, err := service.GetChatStats()
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	chatCount := stats["chats_count"]
	if chatCount != int64(2) {
		t.Fatalf("Expected chats_count to be 2, got: %v", chatCount)
	}

	totalTokens := stats["total_tokens"]
	if totalTokens != int64(700) {
		t.Fatalf("Expected total_tokens to be 700, got: %v", totalTokens)
	}
}
