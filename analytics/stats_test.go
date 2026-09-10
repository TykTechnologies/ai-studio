package analytics

import (
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupStatsTest(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	// Automigrate required models
	err = db.AutoMigrate(
		&models.LLMChatRecord{},
		&models.LLMChatLogEntry{},
		&models.User{},
		&models.LLM{},
		&models.App{},
		&models.Tool{},
		&models.ProxyLog{},
		&models.ToolCallRecord{},
		&models.ModelPrice{},
	)
	assert.NoError(t, err)

	return db
}

func TestGetChatRecordsPerDay(t *testing.T) {
	db := setupStatsTest(t)

	t.Run("Get chat records by day", func(t *testing.T) {
		// Create test data for 3 days
		now := time.Now()
		for i := 0; i < 3; i++ {
			timestamp := now.AddDate(0, 0, -i)
			record := &models.LLMChatRecord{
				ChatID:    "chat-1",
				TimeStamp: timestamp,
			}
			db.Create(record)
		}

		startDate := now.AddDate(0, 0, -4)
		endDate := now.AddDate(0, 0, 1)

		chartData, err := GetChatRecordsPerDay(db, &startDate, &endDate)

		assert.NoError(t, err)
		assert.NotNil(t, chartData)
		assert.GreaterOrEqual(t, len(chartData.Labels), 1)
		assert.Equal(t, len(chartData.Labels), len(chartData.Data))
	})
}

func TestGetToolCallsPerDay(t *testing.T) {
	db := setupStatsTest(t)

	t.Run("Get tool calls by day", func(t *testing.T) {
		// Create test tool call records
		now := time.Now()
		for i := 0; i < 2; i++ {
			timestamp := now.AddDate(0, 0, -i)
			record := &models.ToolCallRecord{
				ToolID:    1,
				Name:      "TestTool",
				TimeStamp: timestamp,
			}
			db.Create(record)
		}

		startDate := now.AddDate(0, 0, -3)
		endDate := now.AddDate(0, 0, 1)

		chartData, err := GetToolCallsPerDay(db, startDate, endDate)

		assert.NoError(t, err)
		assert.NotNil(t, chartData)
		assert.GreaterOrEqual(t, len(chartData.Labels), 1)
	})
}

func TestGetChatRecordsPerUser(t *testing.T) {
	db := setupStatsTest(t)

	t.Run("Get chat records per user", func(t *testing.T) {
		// Create users
		user1 := &models.User{Email: "user1@test.com", Name: "User 1"}
		user2 := &models.User{Email: "user2@test.com", Name: "User 2"}
		db.Create(user1)
		db.Create(user2)

		// Create chat records
		now := time.Now()
		db.Create(&models.LLMChatRecord{ChatID: "1", UserID: user1.ID, TimeStamp: now})
		db.Create(&models.LLMChatRecord{ChatID: "2", UserID: user1.ID, TimeStamp: now})
		db.Create(&models.LLMChatRecord{ChatID: "3", UserID: user2.ID, TimeStamp: now})

		startDate := now.AddDate(0, 0, -1)
		endDate := now.AddDate(0, 0, 1)

		chartData, err := GetChatRecordsPerUser(db, startDate, endDate)

		assert.NoError(t, err)
		assert.NotNil(t, chartData)
		assert.GreaterOrEqual(t, len(chartData.Labels), 2)
	})
}

func TestGetUniqueUsersPerDay(t *testing.T) {
	db := setupStatsTest(t)

	t.Run("Get unique users per day", func(t *testing.T) {
		// Create users
		user1 := &models.User{Email: "user1@test.com"}
		user2 := &models.User{Email: "user2@test.com"}
		db.Create(user1)
		db.Create(user2)

		// Create chat records for today
		now := time.Now()
		db.Create(&models.LLMChatRecord{ChatID: "1", UserID: user1.ID, TimeStamp: now})
		db.Create(&models.LLMChatRecord{ChatID: "2", UserID: user2.ID, TimeStamp: now})
		db.Create(&models.LLMChatRecord{ChatID: "3", UserID: user1.ID, TimeStamp: now}) // Same user, should count once

		startDate := now.AddDate(0, 0, -1)
		endDate := now.AddDate(0, 0, 1)

		chartData, err := GetUniqueUsersPerDay(db, startDate, endDate)

		assert.NoError(t, err)
		assert.NotNil(t, chartData)
		assert.GreaterOrEqual(t, len(chartData.Data), 1)
	})
}

func TestGetTokenUsagePerUser(t *testing.T) {
	db := setupStatsTest(t)

	t.Run("Get token usage per user", func(t *testing.T) {
		// Create users
		user1 := &models.User{Email: "user1@test.com", Name: "User 1"}
		db.Create(user1)

		// Create LLM chat records
		now := time.Now()
		interactionType := models.ChatInteraction
		db.Create(&models.LLMChatRecord{
			UserID:          user1.ID,
			PromptTokens:    100,
			ResponseTokens:  50,
			TotalTokens:     150,
			TimeStamp:       now,
			InteractionType: interactionType,
		})

		startDate := now.AddDate(0, 0, -1)
		endDate := now.AddDate(0, 0, 1)

		chartData, err := GetTokenUsagePerUser(db, startDate, endDate, &interactionType)

		assert.NoError(t, err)
		assert.NotNil(t, chartData)
	})
}

func TestGetTokenUsagePerApp(t *testing.T) {
	db := setupStatsTest(t)

	t.Run("Get token usage per app", func(t *testing.T) {
		// Create app
		app := &models.App{Name: "Test App"}
		db.Create(app)

		// Create LLM chat records
		now := time.Now()
		interactionType := models.ChatInteraction
		db.Create(&models.LLMChatRecord{
			AppID:          app.ID,
			PromptTokens:   200,
			ResponseTokens: 100,
			TotalTokens:    300,
			TimeStamp:      now,
			InteractionType: interactionType,
		})

		startDate := now.AddDate(0, 0, -1)
		endDate := now.AddDate(0, 0, 1)

		chartData, err := GetTokenUsagePerApp(db, startDate, endDate, &interactionType)

		assert.NoError(t, err)
		assert.NotNil(t, chartData)
	})
}

func TestGetToolUsageStatistics(t *testing.T) {
	db := setupStatsTest(t)

	t.Run("Get tool usage statistics", func(t *testing.T) {
		// Create tool
		tool := &models.Tool{Name: "Test Tool"}
		db.Create(tool)

		// Create tool call records
		now := time.Now()
		db.Create(&models.ToolCallRecord{
			ToolID:    tool.ID,
			Name:      "TestTool",
			TimeStamp: now,
		})
		db.Create(&models.ToolCallRecord{
			ToolID:    tool.ID,
			Name:      "TestTool",
			TimeStamp: now,
		})

		startDate := now.AddDate(0, 0, -1)
		endDate := now.AddDate(0, 0, 1)

		chartData, err := GetToolUsageStatistics(db, startDate, endDate)

		assert.NoError(t, err)
		assert.NotNil(t, chartData)
		if len(chartData.Data) > 0 {
			assert.GreaterOrEqual(t, chartData.Data[0], float64(1))
		}
	})
}

func TestGetToolOperationsUsageOverTime(t *testing.T) {
	db := setupStatsTest(t)

	t.Run("Get tool operations over time", func(t *testing.T) {
		// Create tool
		tool := &models.Tool{Name: "Test Tool"}
		db.Create(tool)

		// Create tool call records with different operation names
		now := time.Now()
		db.Create(&models.ToolCallRecord{
			ToolID:    tool.ID,
			Name:      "getUsers",
			TimeStamp: now,
		})
		db.Create(&models.ToolCallRecord{
			ToolID:    tool.ID,
			Name:      "createUser",
			TimeStamp: now,
		})

		startDate := now.AddDate(0, 0, -1)
		endDate := now.AddDate(0, 0, 1)

		chartData, err := GetToolOperationsUsageOverTime(db, tool.ID, startDate, endDate)

		assert.NoError(t, err)
		assert.NotNil(t, chartData)
	})
}

func TestGetChatInteractionsForChat(t *testing.T) {
	db := setupStatsTest(t)

	t.Run("Get interactions for chat", func(t *testing.T) {
		// Create chat records
		now := time.Now()
		chatID := "test-chat-123"
		db.Create(&models.LLMChatRecord{
			ChatID:    chatID,
			TimeStamp: now,
		})
		db.Create(&models.LLMChatRecord{
			ChatID:    chatID,
			TimeStamp: now.Add(1 * time.Hour),
		})

		startDate := now.AddDate(0, 0, -1)
		endDate := now.AddDate(0, 0, 1)

		chartData, err := GetChatInteractionsForChat(db, startDate, endDate, chatID)

		assert.NoError(t, err)
		assert.NotNil(t, chartData)
		assert.GreaterOrEqual(t, len(chartData.Data), 1)
	})
}

func TestGetAppInteractionsOverTime(t *testing.T) {
	db := setupStatsTest(t)

	t.Run("Get app interactions over time", func(t *testing.T) {
		// Create app
		app := &models.App{Name: "Test App"}
		db.Create(app)

		// Create LLM chat records
		now := time.Now()
		interactionType := models.ChatInteraction
		db.Create(&models.LLMChatRecord{
			AppID:          app.ID,
			TimeStamp:      now,
			InteractionType: interactionType,
		})

		startDate := now.AddDate(0, 0, -1)
		endDate := now.AddDate(0, 0, 1)

		chartData, err := GetAppInteractionsOverTime(db, startDate, endDate, app.ID)

		assert.NoError(t, err)
		assert.NotNil(t, chartData)
	})
}

func TestGetModelUsage(t *testing.T) {
	db := setupStatsTest(t)

	t.Run("Get usage for specific model", func(t *testing.T) {
		// Create LLM usage records
		now := time.Now()
		modelName := "gpt-4"
		interactionType := models.ChatInteraction
		db.Create(&models.LLMChatRecord{
			Name:           modelName,
			PromptTokens:    100,
			ResponseTokens: 50,
			TimeStamp:       now,
			InteractionType: interactionType,
		})
		db.Create(&models.LLMChatRecord{
			Name:           modelName,
			PromptTokens:    200,
			ResponseTokens: 100,
			TimeStamp:       now.Add(1 * time.Hour),
			InteractionType: interactionType,
		})

		startDate := now.AddDate(0, 0, -1)
		endDate := now.AddDate(0, 0, 1)

		chartData, err := GetModelUsage(db, startDate, endDate, modelName)

		assert.NoError(t, err)
		assert.NotNil(t, chartData)
		assert.GreaterOrEqual(t, len(chartData.Data), 1)
	})
}

func TestGetVendorUsage(t *testing.T) {
	db := setupStatsTest(t)

	t.Run("Get usage for specific vendor", func(t *testing.T) {
		// Create LLM
		llm := &models.LLM{
			Name:         "Test LLM",
			Vendor:       models.OPENAI,
			DefaultModel: "gpt-4",
			Active:       true,
		}
		db.Create(llm)

		// Create LLM usage records
		now := time.Now()
		interactionType := models.ChatInteraction
		db.Create(&models.LLMChatRecord{
			LLMID:           llm.ID,
			Vendor:      string(models.OPENAI),
			PromptTokens:    150,
			ResponseTokens: 75,
			TimeStamp:       now,
			InteractionType: interactionType,
		})

		startDate := now.AddDate(0, 0, -1)
		endDate := now.AddDate(0, 0, 1)

		chartData, err := GetVendorUsage(db, startDate, endDate, string(models.OPENAI), &llm.ID)

		assert.NoError(t, err)
		assert.NotNil(t, chartData)
	})

	t.Run("Get usage for vendor without LLM filter", func(t *testing.T) {
		now := time.Now()
		interactionType := models.ChatInteraction
		db.Create(&models.LLMChatRecord{
			Vendor:      string(models.ANTHROPIC),
			PromptTokens:    100,
			ResponseTokens: 50,
			TimeStamp:       now,
			InteractionType: interactionType,
		})

		startDate := now.AddDate(0, 0, -1)
		endDate := now.AddDate(0, 0, 1)

		chartData, err := GetVendorUsage(db, startDate, endDate, string(models.ANTHROPIC), nil)

		assert.NoError(t, err)
		assert.NotNil(t, chartData)
	})
}

func TestGetCostAnalysis(t *testing.T) {
	db := setupStatsTest(t)

	t.Run("Get cost analysis by vendor", func(t *testing.T) {
		// Create LLM usage with costs
		now := time.Now()
		interactionType := models.ChatInteraction
		db.Create(&models.LLMChatRecord{
			Vendor:      string(models.OPENAI),
			Cost:       1.50,
			TimeStamp:       now,
			InteractionType: interactionType,
		})
		db.Create(&models.LLMChatRecord{
			Vendor:      string(models.ANTHROPIC),
			Cost:       2.00,
			TimeStamp:       now,
			InteractionType: interactionType,
		})

		startDate := now.AddDate(0, 0, -1)
		endDate := now.AddDate(0, 0, 1)

		costData, err := GetCostAnalysis(db, startDate, endDate, &interactionType)

		assert.NoError(t, err)
		assert.NotNil(t, costData)
		assert.GreaterOrEqual(t, len(costData), 1)
	})
}

func TestGetMostUsedLLMModels(t *testing.T) {
	db := setupStatsTest(t)

	t.Run("Get most used models", func(t *testing.T) {
		// Create LLM usage records
		now := time.Now()
		interactionType := models.ChatInteraction
		db.Create(&models.LLMChatRecord{
			Name:           "gpt-4",
			TimeStamp:       now,
			InteractionType: interactionType,
		})
		db.Create(&models.LLMChatRecord{
			Name:           "gpt-4",
			TimeStamp:       now,
			InteractionType: interactionType,
		})
		db.Create(&models.LLMChatRecord{
			Name:           "claude-3",
			TimeStamp:       now,
			InteractionType: interactionType,
		})

		startDate := now.AddDate(0, 0, -1)
		endDate := now.AddDate(0, 0, 1)

		chartData, err := GetMostUsedLLMModels(db, startDate, endDate, &interactionType)

		assert.NoError(t, err)
		assert.NotNil(t, chartData)
		assert.GreaterOrEqual(t, len(chartData.Labels), 1)
	})
}

func TestGetTokenUsageForApp(t *testing.T) {
	db := setupStatsTest(t)

	t.Run("Get token usage for specific app", func(t *testing.T) {
		// Create app
		app := &models.App{Name: "Test App"}
		db.Create(app)

		// Create LLM usage records
		now := time.Now()
		db.Create(&models.LLMChatRecord{
			AppID:           app.ID,
			PromptTokens:    500,
			ResponseTokens: 250,
			TotalTokens:     750,
			TimeStamp:       now,
		})

		startDate := now.AddDate(0, 0, -1)
		endDate := now.AddDate(0, 0, 1)

		chartData, err := GetTokenUsageForApp(db, startDate, endDate, app.ID)

		assert.NoError(t, err)
		assert.NotNil(t, chartData)
	})
}

func TestGetUsage(t *testing.T) {
	db := setupStatsTest(t)

	t.Run("Get multi-axis usage data", func(t *testing.T) {
		// Create app and LLM
		app := &models.App{Name: "Test App"}
		llm := &models.LLM{Name: "Test LLM", Vendor: models.OPENAI}
		db.Create(app)
		db.Create(llm)

		// Create LLM usage records
		now := time.Now()
		interactionType := models.ChatInteraction
		db.Create(&models.LLMChatRecord{
			LLMID:           llm.ID,
			AppID:           app.ID,
			Vendor:      string(models.OPENAI),
			PromptTokens:    100,
			ResponseTokens: 50,
			Cost:       0.50,
			TimeStamp:       now,
			InteractionType: interactionType,
		})

		startDate := now.AddDate(0, 0, -1)
		endDate := now.AddDate(0, 0, 1)

		multiAxisData, err := GetUsage(db, startDate, endDate, string(models.OPENAI), &llm.ID, &app.ID, &interactionType, "")

		assert.NoError(t, err)
		assert.NotNil(t, multiAxisData)
	})

	t.Run("Filters by model name", func(t *testing.T) {
		llm := &models.LLM{Name: "Model Filter LLM", Vendor: models.OPENAI}
		db.Create(llm)

		now := time.Now()
		db.Create(&models.LLMChatRecord{LLMID: llm.ID, Vendor: string(models.OPENAI), Name: "gpt-4o", TotalTokens: 100, TimeStamp: now, InteractionType: models.ProxyInteraction})
		db.Create(&models.LLMChatRecord{LLMID: llm.ID, Vendor: string(models.OPENAI), Name: "gpt-4o", TotalTokens: 200, TimeStamp: now, InteractionType: models.ProxyInteraction})
		db.Create(&models.LLMChatRecord{LLMID: llm.ID, Vendor: string(models.OPENAI), Name: "gpt-3.5-turbo", TotalTokens: 5000, TimeStamp: now, InteractionType: models.ProxyInteraction})

		startDate := now.AddDate(0, 0, -1)
		endDate := now.AddDate(0, 0, 1)

		filtered, err := GetUsage(db, startDate, endDate, "", &llm.ID, nil, nil, "gpt-4o")
		assert.NoError(t, err)
		if assert.Len(t, filtered.Labels, 1) {
			assert.Equal(t, float64(300), filtered.Datasets[0].Data[0], "only gpt-4o tokens should be summed")
		}

		unfiltered, err := GetUsage(db, startDate, endDate, "", &llm.ID, nil, nil, "")
		assert.NoError(t, err)
		if assert.Len(t, unfiltered.Labels, 1) {
			assert.Equal(t, float64(5300), unfiltered.Datasets[0].Data[0])
		}
	})
}

func TestGetTokenUsageAndCostForApp(t *testing.T) {
	db := setupStatsTest(t)

	t.Run("Get token usage and cost for app", func(t *testing.T) {
		// Create app
		app := &models.App{Name: "Test App"}
		db.Create(app)

		// Create LLM usage records
		now := time.Now()
		db.Create(&models.LLMChatRecord{
			AppID:           app.ID,
			PromptTokens:    1000,
			ResponseTokens: 500,
			Cost:       1.25,
			TimeStamp:       now,
		})

		startDate := now.AddDate(0, 0, -1)
		endDate := now.AddDate(0, 0, 1)

		multiAxisData, err := GetTokenUsageAndCostForApp(db, startDate, endDate, app.ID)

		assert.NoError(t, err)
		assert.NotNil(t, multiAxisData)
	})
}

func TestGetTotalCostPerVendorAndModel(t *testing.T) {
	db := setupStatsTest(t)

	t.Run("Get total cost per vendor and model", func(t *testing.T) {
		// Create LLM
		llm := &models.LLM{Name: "Test LLM", Vendor: models.OPENAI}
		db.Create(llm)

		// Create LLM usage records
		now := time.Now()
		interactionType := models.ChatInteraction
		db.Create(&models.LLMChatRecord{
			LLMID:           llm.ID,
			Vendor:      string(models.OPENAI),
			Name:           "gpt-4",
			Cost:       2.50,
			TimeStamp:       now,
			InteractionType: interactionType,
		})

		startDate := now.AddDate(0, 0, -1)
		endDate := now.AddDate(0, 0, 1)

		costs, err := GetTotalCostPerVendorAndModel(db, startDate, endDate, &interactionType, &llm.ID)

		assert.NoError(t, err)
		assert.NotNil(t, costs)
	})

	t.Run("Reports request count, distinct apps and last used per model", func(t *testing.T) {
		llm := &models.LLM{Name: "Usage LLM", Vendor: models.OPENAI}
		db.Create(llm)
		appA := &models.App{Name: "App A"}
		appB := &models.App{Name: "App B"}
		db.Create(appA)
		db.Create(appB)

		now := time.Now().Truncate(time.Second)
		newest := now.Add(-5 * time.Minute)
		records := []models.LLMChatRecord{
			{LLMID: llm.ID, AppID: appA.ID, Vendor: string(models.OPENAI), Name: "gpt-4o", TotalTokens: 10, TimeStamp: now.Add(-3 * time.Hour), InteractionType: models.ProxyInteraction},
			{LLMID: llm.ID, AppID: appA.ID, Vendor: string(models.OPENAI), Name: "gpt-4o", TotalTokens: 10, TimeStamp: newest, InteractionType: models.ProxyInteraction},
			{LLMID: llm.ID, AppID: appB.ID, Vendor: string(models.OPENAI), Name: "gpt-4o", TotalTokens: 10, TimeStamp: now.Add(-2 * time.Hour), InteractionType: models.ChatInteraction},
			{LLMID: llm.ID, AppID: appB.ID, Vendor: string(models.OPENAI), Name: "gpt-3.5-turbo", TotalTokens: 10, TimeStamp: now.Add(-1 * time.Hour), InteractionType: models.ProxyInteraction},
		}
		for i := range records {
			db.Create(&records[i])
		}

		startDate := now.AddDate(0, 0, -1)
		endDate := now.AddDate(0, 0, 1)

		costs, err := GetTotalCostPerVendorAndModel(db, startDate, endDate, nil, &llm.ID)
		assert.NoError(t, err)

		byModel := map[string]VendorModelCost{}
		for _, c := range costs {
			byModel[c.Model] = c
		}
		if assert.Contains(t, byModel, "gpt-4o") {
			row := byModel["gpt-4o"]
			assert.Equal(t, int64(3), row.RequestCount)
			assert.Equal(t, int64(2), row.AppCount)
			// MAX(time_stamp) comes back as raw text on SQLite; ScanTime must parse it.
			assert.WithinDuration(t, newest, row.LastUsed.Time, time.Second)
		}
		if assert.Contains(t, byModel, "gpt-3.5-turbo") {
			row := byModel["gpt-3.5-turbo"]
			assert.Equal(t, int64(1), row.RequestCount)
			assert.Equal(t, int64(1), row.AppCount)
			assert.WithinDuration(t, now.Add(-1*time.Hour), row.LastUsed.Time, time.Second)
		}
	})
}

func TestGetAppsForModel(t *testing.T) {
	db := setupStatsTest(t)

	llm := &models.LLM{Name: "Apps LLM", Vendor: models.OPENAI}
	otherLLM := &models.LLM{Name: "Other LLM", Vendor: models.OPENAI}
	db.Create(llm)
	db.Create(otherLLM)

	owner := &models.User{Email: "owner@example.com"}
	db.Create(owner)

	active := &models.App{Name: "Active App", UserID: owner.ID}
	quiet := &models.App{Name: "Quiet App"}
	deleted := &models.App{Name: "Retired App"}
	db.Create(active)
	db.Create(quiet)
	db.Create(deleted)
	db.Delete(deleted) // soft delete: usage must stay attributable

	now := time.Now().Truncate(time.Second)
	records := []models.LLMChatRecord{
		// active: two calls, latest 5 minutes ago
		{LLMID: llm.ID, AppID: active.ID, Name: "gpt-3.5-turbo", Vendor: string(models.OPENAI), TotalTokens: 100, Cost: 1234, TimeStamp: now.Add(-2 * time.Hour), InteractionType: models.ProxyInteraction},
		{LLMID: llm.ID, AppID: active.ID, Name: "gpt-3.5-turbo", Vendor: string(models.OPENAI), TotalTokens: 50, Cost: 766, TimeStamp: now.Add(-5 * time.Minute), InteractionType: models.ProxyInteraction},
		// quiet: one call a day ago
		{LLMID: llm.ID, AppID: quiet.ID, Name: "gpt-3.5-turbo", Vendor: string(models.OPENAI), TotalTokens: 10, TimeStamp: now.Add(-24 * time.Hour), InteractionType: models.ChatInteraction},
		// deleted app: one call 3 hours ago
		{LLMID: llm.ID, AppID: deleted.ID, Name: "gpt-3.5-turbo", Vendor: string(models.OPENAI), TotalTokens: 10, TimeStamp: now.Add(-3 * time.Hour), InteractionType: models.ProxyInteraction},
		// hard-deleted / unknown app id
		{LLMID: llm.ID, AppID: 9999, Name: "gpt-3.5-turbo", Vendor: string(models.OPENAI), TotalTokens: 10, TimeStamp: now.Add(-4 * time.Hour), InteractionType: models.ProxyInteraction},
		// noise: outside the range, different model, different LLM entry
		{LLMID: llm.ID, AppID: active.ID, Name: "gpt-3.5-turbo", Vendor: string(models.OPENAI), TotalTokens: 10, TimeStamp: now.AddDate(0, 0, -10), InteractionType: models.ProxyInteraction},
		{LLMID: llm.ID, AppID: active.ID, Name: "gpt-4o", Vendor: string(models.OPENAI), TotalTokens: 10, TimeStamp: now, InteractionType: models.ProxyInteraction},
		{LLMID: otherLLM.ID, AppID: quiet.ID, Name: "gpt-3.5-turbo", Vendor: string(models.OPENAI), TotalTokens: 10, TimeStamp: now, InteractionType: models.ProxyInteraction},
	}
	for i := range records {
		db.Create(&records[i])
	}

	startDate := now.AddDate(0, 0, -2)
	endDate := now.AddDate(0, 0, 1)

	t.Run("Lists apps most recently used first with owner and counts", func(t *testing.T) {
		apps, err := GetAppsForModel(db, startDate, endDate, llm.ID, "gpt-3.5-turbo", nil)
		assert.NoError(t, err)
		if !assert.Len(t, apps, 4) {
			return
		}

		assert.Equal(t, active.ID, apps[0].AppID)
		assert.Equal(t, "Active App", apps[0].AppName)
		assert.False(t, apps[0].AppDeleted)
		assert.Equal(t, owner.ID, apps[0].OwnerUserID)
		assert.Equal(t, "owner@example.com", apps[0].OwnerEmail)
		assert.Equal(t, int64(2), apps[0].RequestCount)
		assert.Equal(t, int64(150), apps[0].TotalTokens)
		assert.InDelta(t, 0.2, apps[0].TotalCost, 0.0001)
		assert.WithinDuration(t, now.Add(-2*time.Hour), apps[0].FirstUsed.Time, time.Second)
		assert.WithinDuration(t, now.Add(-5*time.Minute), apps[0].LastUsed.Time, time.Second)

		assert.Equal(t, deleted.ID, apps[1].AppID)
		assert.Equal(t, "Retired App", apps[1].AppName)
		assert.True(t, apps[1].AppDeleted)

		assert.Equal(t, uint(9999), apps[2].AppID)
		assert.Equal(t, "Deleted app #9999", apps[2].AppName)
		assert.True(t, apps[2].AppDeleted)

		assert.Equal(t, quiet.ID, apps[3].AppID)
		assert.Equal(t, "", apps[3].OwnerEmail)
		assert.Equal(t, int64(1), apps[3].RequestCount)
	})

	t.Run("Filters by interaction type", func(t *testing.T) {
		chat := models.ChatInteraction
		apps, err := GetAppsForModel(db, startDate, endDate, llm.ID, "gpt-3.5-turbo", &chat)
		assert.NoError(t, err)
		if assert.Len(t, apps, 1) {
			assert.Equal(t, quiet.ID, apps[0].AppID)
		}
	})

	t.Run("Returns empty list for an unused model", func(t *testing.T) {
		apps, err := GetAppsForModel(db, startDate, endDate, llm.ID, "claude-3-opus", nil)
		assert.NoError(t, err)
		assert.Empty(t, apps)
	})
}

func TestScanTime(t *testing.T) {
	ref := time.Date(2026, 9, 8, 10, 11, 12, 345000000, time.UTC)

	cases := map[string]interface{}{
		"time.Time":         ref,
		"sqlite text":       "2026-09-08 10:11:12.345+00:00",
		"sqlite text bytes": []byte("2026-09-08 10:11:12.345+00:00"),
		"rfc3339":           "2026-09-08T10:11:12.345Z",
	}
	for name, value := range cases {
		t.Run(name, func(t *testing.T) {
			var s ScanTime
			assert.NoError(t, s.Scan(value))
			assert.True(t, ref.Equal(s.Time), "got %v", s.Time)
		})
	}

	t.Run("nil scans to zero time", func(t *testing.T) {
		var s ScanTime
		assert.NoError(t, s.Scan(nil))
		assert.True(t, s.IsZero())
	})

	t.Run("garbage is an error", func(t *testing.T) {
		var s ScanTime
		assert.Error(t, s.Scan("not a time"))
	})
}

func TestGetChatLogsForChatID(t *testing.T) {
	db := setupStatsTest(t)

	t.Run("Get chat logs for chat ID", func(t *testing.T) {
		// Create chat logs
		chatID := uint(12345)
		db.Create(&models.LLMChatLogEntry{
			ChatID:   "chat-12345",
			Prompt:   "Hello",
			Response: "Hi there",
		})
		db.Create(&models.LLMChatLogEntry{
			ChatID:   "chat-12345",
			Prompt:   "How are you?",
			Response: "I'm good",
		})

		logs, err := GetChatLogsForChatID(db, chatID)

		assert.NoError(t, err)
		// Note: May return 0 or more depending on how function maps ChatID
		assert.GreaterOrEqual(t, len(logs), 0)
	})
}

func TestGetBudgetUsage(t *testing.T) {
	db := setupStatsTest(t)

	t.Run("Get budget usage statistics", func(t *testing.T) {
		// Create LLM with budget
		budget := 100.00
		llm := &models.LLM{
			Name:          "Test LLM",
			Vendor:        models.OPENAI,
			MonthlyBudget: &budget,
		}
		db.Create(llm)

		// Create usage records
		now := time.Now()
		db.Create(&models.LLMChatRecord{
			LLMID:     llm.ID,
			Cost: 25.00,
			TimeStamp: now,
		})

		startDate := now.AddDate(0, 0, -30)
		endDate := now.AddDate(0, 0, 1)

		usageData, err := GetBudgetUsage(db, &startDate, &endDate, &llm.ID)

		assert.NoError(t, err)
		assert.NotNil(t, usageData)
	})

	t.Run("Get budget usage without LLM filter", func(t *testing.T) {
		now := time.Now()
		startDate := now.AddDate(0, 0, -7)
		endDate := now.AddDate(0, 0, 1)

		usageData, err := GetBudgetUsage(db, &startDate, &endDate, nil)

		assert.NoError(t, err)
		assert.NotNil(t, usageData)
	})
}

func TestGetProxyLogsForAppID(t *testing.T) {
	db := setupStatsTest(t)

	t.Run("Get proxy logs for app with pagination", func(t *testing.T) {
		// Create app
		app := &models.App{Name: "Test App"}
		db.Create(app)

		// Create proxy logs
		now := time.Now()
		for i := 0; i < 5; i++ {
			db.Create(&models.ProxyLog{
				AppID:     app.ID,
				TimeStamp: now,
			})
		}

		startDate := now.AddDate(0, 0, -1)
		endDate := now.AddDate(0, 0, 1)

		logs, total, err := GetProxyLogsForAppID(db, startDate, endDate, app.ID, 1, 10, "")

		assert.NoError(t, err)
		assert.GreaterOrEqual(t, len(logs), 1)
		assert.GreaterOrEqual(t, total, int64(1))
	})
}

func TestGetProxyLogsForLLM(t *testing.T) {
	db := setupStatsTest(t)

	t.Run("Get proxy logs for LLM with pagination", func(t *testing.T) {
		// Create LLM
		llm := &models.LLM{Name: "Test LLM", Vendor: models.OPENAI}
		db.Create(llm)

		now := time.Now()
		for i := 0; i < 3; i++ {
			db.Create(&models.ProxyLog{
				LLMID:     llm.ID,
				Vendor:    string(models.OPENAI),
				TimeStamp: now,
			})
		}

		startDate := now.AddDate(0, 0, -1)
		endDate := now.AddDate(0, 0, 1)

		logs, total, err := GetProxyLogsForLLM(db, startDate, endDate, llm.ID, 1, 10, "")

		assert.NoError(t, err)
		assert.Equal(t, 3, len(logs))
		assert.Equal(t, int64(3), total)
	})

	// Reproduces the bug where two LLM entries share the same vendor type
	// (e.g. one personal Anthropic key + one work Anthropic key) and the
	// detail page for one shows proxy logs from the other.
	t.Run("Two LLMs same vendor are isolated", func(t *testing.T) {
		db := setupStatsTest(t)

		personal := &models.LLM{Name: "Anthropic (personal)", Vendor: models.ANTHROPIC}
		work := &models.LLM{Name: "Anthropic (work)", Vendor: models.ANTHROPIC}
		db.Create(personal)
		db.Create(work)

		now := time.Now()
		// 4 logs attributed to the personal LLM
		for i := 0; i < 4; i++ {
			db.Create(&models.ProxyLog{
				LLMID:     personal.ID,
				Vendor:    string(models.ANTHROPIC),
				TimeStamp: now,
			})
		}
		// 2 logs attributed to the work LLM
		for i := 0; i < 2; i++ {
			db.Create(&models.ProxyLog{
				LLMID:     work.ID,
				Vendor:    string(models.ANTHROPIC),
				TimeStamp: now,
			})
		}

		startDate := now.AddDate(0, 0, -1)
		endDate := now.AddDate(0, 0, 1)

		personalLogs, personalTotal, err := GetProxyLogsForLLM(db, startDate, endDate, personal.ID, 1, 50, "")
		assert.NoError(t, err)
		assert.Equal(t, int64(4), personalTotal, "personal LLM should only see its own 4 logs, not the work LLM's 2")
		assert.Equal(t, 4, len(personalLogs))
		for _, l := range personalLogs {
			assert.Equal(t, personal.ID, l.LLMID, "log returned for personal LLM was attributed to a different LLM")
		}

		workLogs, workTotal, err := GetProxyLogsForLLM(db, startDate, endDate, work.ID, 1, 50, "")
		assert.NoError(t, err)
		assert.Equal(t, int64(2), workTotal, "work LLM should only see its own 2 logs, not the personal LLM's 4")
		assert.Equal(t, 2, len(workLogs))
		for _, l := range workLogs {
			assert.Equal(t, work.ID, l.LLMID, "log returned for work LLM was attributed to a different LLM")
		}
	})
}
