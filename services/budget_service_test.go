package services

import (
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/notifications"
	"github.com/stretchr/testify/assert"
)

func TestGetMonthlySpending(t *testing.T) {
	db := setupTestDB(t)
	mailService := notifications.NewTestMailService()
	service := NewBudgetService(db, mailService)

	// Create test app
	app := &models.App{
		ID:            1,
		Name:          "Test App",
		MonthlyBudget: ptr(100.0),
	}
	db.Create(app)

	// Create test records
	now := time.Now()
	startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())

	records := []models.LLMChatRecord{
		{
			AppID:     app.ID,
			Cost:      10.0,
			TimeStamp: startOfMonth.Add(24 * time.Hour), // Day 2
		},
		{
			AppID:     app.ID,
			Cost:      15.0,
			TimeStamp: startOfMonth.Add(48 * time.Hour), // Day 3
		},
		{
			AppID:     app.ID,
			Cost:      20.0,
			TimeStamp: startOfMonth.Add(-24 * time.Hour), // Previous month
		},
	}

	for _, record := range records {
		db.Create(&record)
	}

	// Test spending calculation
	spent, err := service.GetMonthlySpending(app.ID, startOfMonth, startOfMonth.AddDate(0, 1, 0).Add(-time.Second))
	assert.NoError(t, err)
	assert.Equal(t, 25.0, spent) // 10.0 + 15.0 = 25.0 (only current month)
}

func TestGetLLMMonthlySpending(t *testing.T) {
	db := setupTestDB(t)
	mailService := notifications.NewTestMailService()
	service := NewBudgetService(db, mailService)

	// Create test LLM
	llm := &models.LLM{
		ID:            1,
		Name:          "Test LLM",
		MonthlyBudget: ptr(200.0),
	}
	db.Create(llm)

	// Create test records
	now := time.Now()
	startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())

	records := []models.LLMChatRecord{
		{
			LLMID:     llm.ID,
			Cost:      30.0,
			TimeStamp: startOfMonth.Add(24 * time.Hour),
		},
		{
			LLMID:     llm.ID,
			Cost:      40.0,
			TimeStamp: startOfMonth.Add(48 * time.Hour),
		},
		{
			LLMID:     llm.ID,
			Cost:      50.0,
			TimeStamp: startOfMonth.Add(-24 * time.Hour), // Previous month
		},
	}

	for _, record := range records {
		db.Create(&record)
	}

	// Test spending calculation
	spent, err := service.GetLLMMonthlySpending(llm.ID, startOfMonth, startOfMonth.AddDate(0, 1, 0).Add(-time.Second))
	assert.NoError(t, err)
	assert.Equal(t, 70.0, spent) // 30.0 + 40.0 = 70.0 (only current month)
}

func TestCheckBudget(t *testing.T) {
	db := setupTestDB(t)
	mailService := notifications.NewTestMailService()
	service := NewBudgetService(db, mailService)

	// Create test app and LLM
	app := &models.App{
		ID:            1,
		Name:          "Test App",
		MonthlyBudget: ptr(100.0),
		UserID:        1, // Add UserID for notification testing
	}
	llm := &models.LLM{
		ID:            1,
		Name:          "Test LLM",
		MonthlyBudget: ptr(200.0),
	}
	db.Create(app)
	db.Create(llm)

	// Create app owner
	owner := &models.User{
		ID:    1,
		Email: "owner@example.com",
	}
	db.Create(owner)

	// Create admin user
	admin := &models.User{
		ID:      2,
		Email:   "admin@example.com",
		IsAdmin: true,
	}
	db.Create(admin)

	// Create additional test app and LLM for under budget test
	app2 := &models.App{
		ID:            2,
		Name:          "Test App 2",
		MonthlyBudget: ptr(1000.0),
	}
	llm2 := &models.LLM{
		ID:            2,
		Name:          "Test LLM 2",
		MonthlyBudget: ptr(1000.0),
	}
	db.Create(app2)
	db.Create(llm2)

	// Create test records
	now := time.Now()
	startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())

	// Create spending records for app1 over budget scenario
	db.Create(&models.LLMChatRecord{
		AppID:     app.ID,
		Cost:      90.0,
		TimeStamp: startOfMonth.Add(24 * time.Hour),
	})
	db.Create(&models.LLMChatRecord{
		AppID:     app.ID,
		Cost:      150.0,
		TimeStamp: startOfMonth.Add(24 * time.Hour),
	})

	// Create spending records for llm1 over budget scenario
	db.Create(&models.LLMChatRecord{
		LLMID:     llm.ID,
		Cost:      165.0, // This will push LLM1 over budget
		TimeStamp: startOfMonth.Add(24 * time.Hour),
	})
	db.Create(&models.LLMChatRecord{
		LLMID:     llm.ID,
		Cost:      90.0,
		TimeStamp: startOfMonth.Add(24 * time.Hour),
	})

	// Create app and LLM with no budget limits
	appNoBudget := &models.App{
		ID:            3,
		Name:          "App No Budget",
		MonthlyBudget: nil,
	}
	llmNoBudget := &models.LLM{
		ID:            3,
		Name:          "LLM No Budget",
		MonthlyBudget: nil,
	}
	db.Create(appNoBudget)
	db.Create(llmNoBudget)

	// Test budget check
	tests := []struct {
		name           string
		app            *models.App
		llm            *models.LLM
		expectedAppPct float64
		expectedLLMPct float64
		shouldError    bool
	}{
		{
			name:           "Both under budget",
			app:            app2,
			llm:            llm2,
			expectedAppPct: 0, // No spending for app2
			expectedLLMPct: 0, // No spending for llm2
			shouldError:    false,
		},
		{
			name:           "App over budget",
			app:            app,
			llm:            llm2,
			expectedAppPct: 240, // (90.0 + 150.0)/100.0 * 100
			expectedLLMPct: 0,   // No spending for llm2
			shouldError:    true,
		},
		{
			name:           "LLM over budget",
			app:            app2,
			llm:            llm,
			expectedAppPct: 0,     // No spending for app2
			expectedLLMPct: 127.5, // (165.0 + 90.0)/200.0 * 100
			shouldError:    true,
		},
		{
			name:           "No budget limits",
			app:            appNoBudget,
			llm:            llmNoBudget,
			expectedAppPct: 0, // No budget set
			expectedLLMPct: 0, // No budget set
			shouldError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			appUsage, llmUsage, err := service.CheckBudget(tt.app, tt.llm)
			if tt.shouldError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.InDelta(t, tt.expectedAppPct, appUsage, 0.1)
			assert.InDelta(t, tt.expectedLLMPct, llmUsage, 0.1)
		})
	}
}

func TestGetBudgetUsage(t *testing.T) {
	db := setupTestDB(t)
	mailService := notifications.NewTestMailService()
	service := NewBudgetService(db, mailService)

	// Create test apps and LLMs
	app1 := &models.App{
		ID:            1,
		Name:          "App 1",
		MonthlyBudget: ptr(100.0),
	}
	app2 := &models.App{
		ID:            2,
		Name:          "App 2",
		MonthlyBudget: nil, // No budget limit
	}
	llm1 := &models.LLM{
		ID:            1,
		Name:          "LLM 1",
		MonthlyBudget: ptr(200.0),
	}
	llm2 := &models.LLM{
		ID:            2,
		Name:          "LLM 2",
		MonthlyBudget: ptr(300.0),
	}

	db.Create(app1)
	db.Create(app2)
	db.Create(llm1)
	db.Create(llm2)

	// Create test records
	now := time.Now()
	startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())

	records := []models.LLMChatRecord{
		{
			AppID:     app1.ID,
			LLMID:     llm1.ID,
			Cost:      50.0,
			TimeStamp: startOfMonth.Add(24 * time.Hour),
		},
		{
			AppID:     app2.ID,
			LLMID:     llm2.ID,
			Cost:      100.0,
			TimeStamp: startOfMonth.Add(24 * time.Hour),
		},
	}

	for _, record := range records {
		db.Create(&record)
	}

	// Test budget usage
	usage, err := service.GetBudgetUsage()
	assert.NoError(t, err)
	assert.NotNil(t, usage)

	// Verify app budgets
	var app1Usage, app2Usage *models.BudgetUsage
	for _, u := range usage {
		if u.EntityType == "App" {
			if u.Name == app1.Name {
				app1Usage = &u
			} else if u.Name == app2.Name {
				app2Usage = &u
			}
		}
	}

	assert.NotNil(t, app1Usage)
	assert.Equal(t, 50.0, app1Usage.Spent)
	assert.Equal(t, *app1.MonthlyBudget, *app1Usage.Budget)

	assert.NotNil(t, app2Usage)
	assert.Equal(t, 100.0, app2Usage.Spent)
	assert.Nil(t, app2Usage.Budget)

	// Verify LLM budgets
	var llm1Usage, llm2Usage *models.BudgetUsage
	for _, u := range usage {
		if u.EntityType == "LLM" {
			if u.Name == llm1.Name {
				llm1Usage = &u
			} else if u.Name == llm2.Name {
				llm2Usage = &u
			}
		}
	}

	assert.NotNil(t, llm1Usage)
	assert.Equal(t, 50.0, llm1Usage.Spent)
	assert.Equal(t, *llm1.MonthlyBudget, *llm1Usage.Budget)

	assert.NotNil(t, llm2Usage)
	assert.Equal(t, 100.0, llm2Usage.Spent)
	assert.Equal(t, *llm2.MonthlyBudget, *llm2Usage.Budget)
}

func TestBudgetCaching(t *testing.T) {
	db := setupTestDB(t)
	mailService := notifications.NewTestMailService()
	service := NewBudgetService(db, mailService)

	// Create test app and LLM
	app := &models.App{
		ID:            1,
		Name:          "Test App",
		MonthlyBudget: ptr(100.0),
	}
	llm := &models.LLM{
		ID:            1,
		Name:          "Test LLM",
		MonthlyBudget: ptr(200.0),
	}
	db.Create(app)
	db.Create(llm)

	// Create initial spending record
	now := time.Now()
	startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())

	// Create initial app spending record
	record := models.LLMChatRecord{
		AppID:     app.ID,
		Cost:      50.0,
		TimeStamp: startOfMonth.Add(24 * time.Hour),
	}
	db.Create(&record)

	// Create initial LLM spending record
	llmRecord := models.LLMChatRecord{
		LLMID:     llm.ID,
		Cost:      50.0,
		TimeStamp: startOfMonth.Add(24 * time.Hour),
	}
	db.Create(&llmRecord)

	// First call for app spending - should hit database
	spent1, err := service.GetMonthlySpending(app.ID, startOfMonth, now)
	assert.NoError(t, err)
	assert.Equal(t, 50.0, spent1)

	// Add new app spending record
	newRecord := models.LLMChatRecord{
		AppID:     app.ID,
		Cost:      25.0,
		TimeStamp: startOfMonth.Add(25 * time.Hour),
	}
	db.Create(&newRecord)

	// Second call within cache window - should return cached value
	spent2, err := service.GetMonthlySpending(app.ID, startOfMonth, now)
	assert.NoError(t, err)
	assert.Equal(t, 50.0, spent2, "Should return cached value")

	// First call for LLM spending - should hit database
	llmSpent1, err := service.GetLLMMonthlySpending(llm.ID, startOfMonth, now)
	assert.NoError(t, err)
	assert.Equal(t, 50.0, llmSpent1)

	// Add new LLM spending record
	newLLMRecord := models.LLMChatRecord{
		LLMID:     llm.ID,
		Cost:      25.0,
		TimeStamp: startOfMonth.Add(26 * time.Hour),
	}
	db.Create(&newLLMRecord)

	// Second call within cache window - should return cached value
	llmSpent2, err := service.GetLLMMonthlySpending(llm.ID, startOfMonth, now)
	assert.NoError(t, err)
	assert.Equal(t, 50.0, llmSpent2, "Should return cached value")

	// Verify budget check uses cached values
	appUsage, llmUsage, err := service.CheckBudget(app, llm)
	assert.NoError(t, err)
	assert.InDelta(t, 50.0, appUsage, 0.1, "Should use cached app spending")
	assert.InDelta(t, 25.0, llmUsage, 0.1, "Should use cached llm spending")
}

// Helper function to create pointer to float64
func ptr(f float64) *float64 {
	return &f
}
