package services

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
)

// Helper function to filter notifications by user
func filterNotificationsByUser(notifications []models.Notification, userID uint) []models.Notification {
	var filtered []models.Notification
	for _, n := range notifications {
		if n.UserID == userID {
			filtered = append(filtered, n)
		}
	}
	return filtered
}

// Helper function to filter notifications by title
func filterNotificationsByTitle(notifications []models.Notification, titleContains string) []models.Notification {
	var filtered []models.Notification
	for _, n := range notifications {
		if strings.Contains(n.Title, titleContains) {
			filtered = append(filtered, n)
		}
	}
	return filtered
}

func TestGetMonthlySpending(t *testing.T) {
	db := setupTestDB(t)
	notificationSvc := NewTestNotificationService(db)
	service := NewBudgetService(db, notificationSvc)

	// Create test app
	app := &models.App{
		Name:          "Test App",
		MonthlyBudget: ptr(100.0),
	}
	// Let GORM assign the primary key.
	db.Create(app)
	assert.NotZero(t, app.ID, "App should have an auto-assigned ID")

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
	notificationSvc := NewTestNotificationService(db)
	service := NewBudgetService(db, notificationSvc)

	// Create test LLM
	llm := &models.LLM{
		Name:          "Test LLM",
		MonthlyBudget: ptr(200.0),
	}
	db.Create(llm)
	assert.NotZero(t, llm.ID, "LLM should have an auto-assigned ID")

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
	notificationSvc := NewTestNotificationService(db)
	service := NewBudgetService(db, notificationSvc)

	// Create admin user
	admin := &models.User{
		Email:   "admin@example.com",
		IsAdmin: true,
	}
	db.Create(admin)
	assert.NotZero(t, admin.ID)

	// Create app owner
	owner := &models.User{
		Email: "owner@example.com",
	}
	db.Create(owner)
	assert.NotZero(t, owner.ID)

	// Create test app and LLM
	app := &models.App{
		Name:          "Test App",
		MonthlyBudget: ptr(100.0),
		UserID:        owner.ID,
	}
	db.Create(app)
	assert.NotZero(t, app.ID)

	llm := &models.LLM{
		Name:          "Test LLM",
		MonthlyBudget: ptr(200.0),
		DefaultModel:  "test-model",
		Vendor:        "mock",
	}
	db.Create(llm)
	assert.NotZero(t, llm.ID)

	// Create model price for currency
	modelPrice := &models.ModelPrice{
		ModelName: "test-model",
		Vendor:    "mock",
		Currency:  "USD",
	}
	db.Create(modelPrice)

	// Create app and LLM with no budget limits
	appNoBudget := &models.App{
		Name:          "App No Budget",
		MonthlyBudget: nil,
	}
	db.Create(appNoBudget)
	assert.NotZero(t, appNoBudget.ID)

	llmNoBudget := &models.LLM{
		Name:          "LLM No Budget",
		MonthlyBudget: nil,
	}
	db.Create(llmNoBudget)
	assert.NotZero(t, llmNoBudget.ID)

	now := time.Now()
	startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())

	monthOffset := int(startOfMonth.Sub(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)).Hours() / 24 / 30)

	// Create 100% threshold notification for app (owner)
	appNotificationOwner := &models.Notification{
		NotificationID: fmt.Sprintf("budget_app_%d_%d_%d_%d_owner",
			app.ID,
			monthOffset,
			int(*app.MonthlyBudget),
			100,
		),
		Type:    "budget_alert",
		Title:   "Budget Alert: App Test App at 100% Usage",
		Content: "Test content",
		UserID:  owner.ID,
		Read:    false,
		SentAt:  startOfMonth.Add(24 * time.Hour),
	}
	db.Create(appNotificationOwner)

	// Create 100% threshold notification for app (admin)
	appNotificationAdmin := &models.Notification{
		NotificationID: fmt.Sprintf("budget_app_%d_%d_%d_%d_admin_%d",
			app.ID,
			monthOffset,
			int(*app.MonthlyBudget),
			100,
			admin.ID,
		),
		Type:    "budget_alert",
		Title:   "Budget Alert: App Test App at 100% Usage",
		Content: "Test content",
		UserID:  admin.ID,
		Read:    false,
		SentAt:  startOfMonth.Add(24 * time.Hour),
	}
	db.Create(appNotificationAdmin)

	// Create 100% threshold notification for LLM (admin)
	llmNotification := &models.Notification{
		NotificationID: fmt.Sprintf("budget_llm_%d_%d_%d_%d_admin_%d",
			llm.ID,
			monthOffset,
			int(*llm.MonthlyBudget),
			100,
			admin.ID,
		),
		Type:    "budget_alert",
		Title:   "Budget Alert: LLM Test LLM at 100% Usage",
		Content: "Test content",
		UserID:  admin.ID,
		Read:    false,
		SentAt:  startOfMonth.Add(24 * time.Hour),
	}
	db.Create(llmNotification)

	tests := []struct {
		name           string
		app            *models.App
		llm            *models.LLM
		expectedAppPct float64
		expectedLLMPct float64
		shouldError    bool
	}{
		{
			name:           "App over budget (has 100% notification)",
			app:            app,
			llm:            llmNoBudget,
			expectedAppPct: 100,
			expectedLLMPct: 0,
			shouldError:    true,
		},
		{
			name:           "LLM over budget (has 100% notification)",
			app:            appNoBudget,
			llm:            llm,
			expectedAppPct: 0,
			expectedLLMPct: 100,
			shouldError:    true,
		},
		{
			name:           "No budget limits",
			app:            appNoBudget,
			llm:            llmNoBudget,
			expectedAppPct: 0,
			expectedLLMPct: 0,
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

func TestAnalyzeBudgetUsage(t *testing.T) {
	db := setupTestDB(t)
	notificationSvc := NewTestNotificationService(db)
	service := NewBudgetService(db, notificationSvc)

	now := time.Now()
	startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())

	// Initialize notification table
	err := models.InitModels(db)
	assert.NoError(t, err)

	// Create app owner
	owner := &models.User{
		Email:   "owner@example.com",
		Name:    "Owner",
		IsAdmin: false,
	}
	err = db.Create(owner).Error
	assert.NoError(t, err)
	assert.NotZero(t, owner.ID)

	// Create admin users
	admin1 := &models.User{
		Email:   "admin1@example.com",
		Name:    "Admin 1",
		IsAdmin: true,
	}
	admin2 := &models.User{
		Email:   "admin2@example.com",
		Name:    "Admin 2",
		IsAdmin: true,
	}
	err = db.Create(admin1).Error
	assert.NoError(t, err)
	err = db.Create(admin2).Error
	assert.NoError(t, err)
	assert.NotZero(t, admin1.ID)
	assert.NotZero(t, admin2.ID)

	// Create test app and LLM
	app := &models.App{
		Name:            "Test App",
		MonthlyBudget:   ptr(100.0),
		UserID:          owner.ID,
		BudgetStartDate: &startOfMonth,
	}
	llm := &models.LLM{
		Name:            "Test LLM",
		MonthlyBudget:   ptr(200.0),
		DefaultModel:    "test-model",
		Vendor:          "mock",
		BudgetStartDate: &startOfMonth,
	}
	db.Create(app)
	db.Create(llm)
	assert.NotZero(t, app.ID)
	assert.NotZero(t, llm.ID)

	// Create model price for currency
	modelPrice := &models.ModelPrice{
		ModelName: "test-model",
		Vendor:    "mock",
		Currency:  "USD",
	}
	db.Create(modelPrice)

	tests := []struct {
		name               string
		setupRecords       func()
		expectedAppNotifs  int
		expectedLLMNotifs  int
		checkNotifications func(t *testing.T, notifications []models.Notification)
	}{
		{
			name: "No notifications under 80%",
			setupRecords: func() {
				notificationSvc.ClearNotifications()
				db.Delete(&models.LLMChatRecord{}, "1=1") // Clean out prior records
				db.Create(&models.LLMChatRecord{
					AppID:     app.ID,
					LLMID:     llm.ID,
					Cost:      50.0, // 50% of app budget, 25% of LLM budget
					TimeStamp: startOfMonth.Add(24 * time.Hour),
				})
			},
			expectedAppNotifs: 0,
			expectedLLMNotifs: 0,
			checkNotifications: func(t *testing.T, notifications []models.Notification) {
				assert.Empty(t, notifications)
			},
		},
		{
			name: "80% threshold notifications",
			setupRecords: func() {
				notificationSvc.ClearNotifications()
				db.Delete(&models.LLMChatRecord{}, "1=1")
				// Create app record
				db.Create(&models.LLMChatRecord{
					AppID:     app.ID,
					Cost:      85.0, // 85% of app budget
					TimeStamp: startOfMonth.Add(24 * time.Hour),
				})

				// Create LLM record
				db.Create(&models.LLMChatRecord{
					LLMID:     llm.ID,
					Cost:      170.0, // 85% of LLM budget
					TimeStamp: startOfMonth.Add(24 * time.Hour),
				})
			},
			expectedAppNotifs: 1,
			expectedLLMNotifs: 1,
			checkNotifications: func(t *testing.T, notifications []models.Notification) {
				// For app, we expect notifications for owner and 2 admins
				appNotifs := filterNotificationsByTitle(notifications, "App Test App at 80% Usage")
				assert.Len(t, appNotifs, 3, "Should have 3 app notifications (1 owner + 2 admins)")

				// For LLM, we expect notifications for 2 admins only
				llmNotifs := filterNotificationsByTitle(notifications, "LLM Test LLM at 80% Usage")
				assert.Len(t, llmNotifs, 2, "Should have 2 LLM notifications (2 admins)")
			},
		},
		{
			name: "100% threshold notifications",
			setupRecords: func() {
				db.Delete(&models.LLMChatRecord{}, "1=1")
				// Create app record
				db.Create(&models.LLMChatRecord{
					AppID:     app.ID,
					Cost:      105.0, // 105% of app budget
					TimeStamp: startOfMonth.Add(24 * time.Hour),
				})
				// Create LLM record
				db.Create(&models.LLMChatRecord{
					LLMID:     llm.ID,
					Cost:      210.0, // 105% of LLM budget
					TimeStamp: startOfMonth.Add(24 * time.Hour),
				})
			},
			expectedAppNotifs: 2, // we expect 80% and 100%
			expectedLLMNotifs: 2, // we expect 80% and 100%
			checkNotifications: func(t *testing.T, notifications []models.Notification) {
				// For app, we expect notifications for owner and 2 admins at both thresholds
				app80Notifs := filterNotificationsByTitle(notifications, "App Test App at 80% Usage")
				app100Notifs := filterNotificationsByTitle(notifications, "App Test App at 100% Usage")
				assert.Len(t, app80Notifs, 3, "Should have 3 app 80% notifications (1 owner + 2 admins)")
				assert.Len(t, app100Notifs, 3, "Should have 3 app 100% notifications (1 owner + 2 admins)")

				// For LLM, we expect notifications for 2 admins only at both thresholds
				llm80Notifs := filterNotificationsByTitle(notifications, "LLM Test LLM at 80% Usage")
				llm100Notifs := filterNotificationsByTitle(notifications, "LLM Test LLM at 100% Usage")
				assert.Len(t, llm80Notifs, 2, "Should have 2 LLM 80% notifications (2 admins)")
				assert.Len(t, llm100Notifs, 2, "Should have 2 LLM 100% notifications (2 admins)")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupRecords()
			service.ClearCache() // Clear cache before analyzing budget usage

			// For 100% threshold test, we need to trigger both thresholds
			if tt.name == "100% threshold notifications" {
				// First trigger 80% notifications
				db.Delete(&models.LLMChatRecord{}, "1=1")
				db.Create(&models.LLMChatRecord{
					AppID:     app.ID,
					Cost:      85.0, // 85% of app budget
					TimeStamp: startOfMonth.Add(24 * time.Hour),
				})
				db.Create(&models.LLMChatRecord{
					LLMID:     llm.ID,
					Cost:      170.0, // 85% of LLM budget
					TimeStamp: startOfMonth.Add(24 * time.Hour),
				})
				service.AnalyzeBudgetUsage(app, llm)

				// Wait for 80% notifications
				for i := 0; i < 10; i++ {
					notifs := notificationSvc.GetNotifications()
					if len(notifs) >= 5 { // 3 app notifications + 2 LLM notifications
						break
					}
					time.Sleep(100 * time.Millisecond)
				}

				// Then trigger 100% notifications
				db.Delete(&models.LLMChatRecord{}, "1=1")
				db.Create(&models.LLMChatRecord{
					AppID:     app.ID,
					Cost:      105.0, // 105% of app budget
					TimeStamp: startOfMonth.Add(24 * time.Hour),
				})
				db.Create(&models.LLMChatRecord{
					LLMID:     llm.ID,
					Cost:      210.0, // 105% of LLM budget
					TimeStamp: startOfMonth.Add(24 * time.Hour),
				})
				service.ClearCache() // Clear cache to force recalculation of spending
				service.AnalyzeBudgetUsage(app, llm)
			} else {
				// For other tests, just run the setup and analysis
				tt.setupRecords()
				service.AnalyzeBudgetUsage(app, llm)
			}

			// Wait for all notifications
			var notifs []models.Notification
			for i := 0; i < 10; i++ {
				notifs = notificationSvc.GetNotifications()
				if len(notifs) >= tt.expectedAppNotifs*3+tt.expectedLLMNotifs*2 { // Each app notification goes to owner + 2 admins, each LLM notification goes to 2 admins
					break
				}
				time.Sleep(100 * time.Millisecond)
			}

			tt.checkNotifications(t, notifs)
		})
	}
}

func TestGetBudgetUsage(t *testing.T) {
	db := setupTestDB(t)
	notificationSvc := NewTestNotificationService(db)
	service := NewBudgetService(db, notificationSvc)

	// Create test apps and LLMs
	app1 := &models.App{
		Name:          "App 1",
		MonthlyBudget: ptr(100.0),
	}
	app2 := &models.App{
		Name:          "App 2",
		MonthlyBudget: nil, // No budget limit
	}
	llm1 := &models.LLM{
		Name:          "LLM 1",
		MonthlyBudget: ptr(200.0),
	}
	llm2 := &models.LLM{
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
				cpy := u
				app1Usage = &cpy
			} else if u.Name == app2.Name {
				cpy := u
				app2Usage = &cpy
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
				cpy := u
				llm1Usage = &cpy
			} else if u.Name == llm2.Name {
				cpy := u
				llm2Usage = &cpy
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

func TestCacheCleanup(t *testing.T) {
	db := setupTestDB(t)
	notificationSvc := NewTestNotificationService(db)
	service := NewBudgetService(db, notificationSvc)

	// Create test app and LLM
	app := &models.App{
		Name:          "Test App",
		MonthlyBudget: ptr(100.0),
	}
	llm := &models.LLM{
		Name:          "Test LLM",
		MonthlyBudget: ptr(200.0),
	}
	db.Create(app)
	db.Create(llm)

	// Create initial spending record
	now := time.Now()
	startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())

	record := models.LLMChatRecord{
		AppID:     app.ID,
		Cost:      50.0,
		TimeStamp: startOfMonth.Add(24 * time.Hour),
	}
	db.Create(&record)

	llmRecord := models.LLMChatRecord{
		LLMID:     llm.ID,
		Cost:      50.0,
		TimeStamp: startOfMonth.Add(24 * time.Hour),
	}
	db.Create(&llmRecord)

	// Test cache cleanup
	service.cacheDuration = 10 * time.Millisecond

	spent1, err := service.GetMonthlySpending(app.ID, startOfMonth, now)
	assert.NoError(t, err)
	assert.Equal(t, 50.0, spent1)

	key := usageKey{
		entityID:   app.ID,
		entityType: "App",
		startDate:  startOfMonth,
	}
	service.cacheMutex.RLock()
	_, exists := service.usageCache[key]
	service.cacheMutex.RUnlock()
	assert.True(t, exists, "Value should be in cache")

	time.Sleep(20 * time.Millisecond)

	service.cacheMutex.RLock()
	_, exists = service.usageCache[key]
	service.cacheMutex.RUnlock()
	assert.False(t, exists, "Cache entry should be removed after expiration")
}

func TestClearCache(t *testing.T) {
	db := setupTestDB(t)
	notificationSvc := NewTestNotificationService(db)
	service := NewBudgetService(db, notificationSvc)

	// Create test app
	app := &models.App{
		Name:          "Test App",
		MonthlyBudget: ptr(100.0),
	}
	db.Create(app)

	// Create initial spending record
	now := time.Now()
	startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	record := models.LLMChatRecord{
		AppID:     app.ID,
		Cost:      50.0,
		TimeStamp: startOfMonth.Add(24 * time.Hour),
	}
	db.Create(&record)

	// Get initial value into cache
	spent1, err := service.GetMonthlySpending(app.ID, startOfMonth, now)
	assert.NoError(t, err)
	assert.Equal(t, 50.0, spent1)

	// Add new record
	newRecord := models.LLMChatRecord{
		AppID:     app.ID,
		Cost:      25.0,
		TimeStamp: startOfMonth.Add(25 * time.Hour),
	}
	db.Create(&newRecord)

	// Confirm cached value
	spent2, err := service.GetMonthlySpending(app.ID, startOfMonth, now)
	assert.NoError(t, err)
	assert.Equal(t, 50.0, spent2, "Should return cached value")

	// Clear cache
	service.ClearCache()

	// Now it should recalc
	spent3, err := service.GetMonthlySpending(app.ID, startOfMonth, now)
	assert.NoError(t, err)
	assert.Equal(t, 75.0, spent3, "Should return new value after cache clear")
}

// Helper function to create pointer to float64
func ptr(f float64) *float64 {
	return &f
}
