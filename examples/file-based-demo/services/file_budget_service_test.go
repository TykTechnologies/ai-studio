package services

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// setupTestBudgetConfig creates a temporary directory with test budget configuration
func setupTestBudgetConfig(t *testing.T) string {
	tempDir := t.TempDir()

	budgetConfig := `{
		"app_budgets": [
			{
				"app_id": 1,
				"monthly_limit": 100.0,
				"current_usage": 25.50,
				"currency": "USD",
				"reset_date": "2025-02-01T00:00:00Z",
				"notifications": {
					"50_percent": false,
					"80_percent": false,
					"90_percent": false,
					"100_percent": false
				}
			},
			{
				"app_id": 2,
				"monthly_limit": 10.0,
				"current_usage": 9.75,
				"currency": "USD",
				"reset_date": "2025-02-01T00:00:00Z",
				"notifications": {
					"50_percent": true,
					"80_percent": true,
					"90_percent": true,
					"100_percent": false
				}
			},
			{
				"app_id": 3,
				"monthly_limit": 50.0,
				"current_usage": 55.0,
				"currency": "USD",
				"reset_date": "2025-02-01T00:00:00Z",
				"notifications": {
					"50_percent": true,
					"80_percent": true,
					"90_percent": true,
					"100_percent": true
				}
			}
		],
		"llm_budgets": [
			{
				"llm_id": 1,
				"monthly_limit": 200.0,
				"current_usage": 45.25,
				"currency": "USD",
				"reset_date": "2025-02-01T00:00:00Z"
			},
			{
				"llm_id": 2,
				"monthly_limit": 150.0,
				"current_usage": 175.0,
				"currency": "USD",
				"reset_date": "2025-02-01T00:00:00Z"
			}
		]
	}`

	if err := os.WriteFile(filepath.Join(tempDir, "budgets.json"), []byte(budgetConfig), 0644); err != nil {
		t.Fatalf("Failed to write budgets.json: %v", err)
	}

	return tempDir
}

func TestNewFileBudgetService(t *testing.T) {
	configDir := setupTestBudgetConfig(t)

	service, err := NewFileBudgetService(configDir)
	if err != nil {
		t.Fatalf("Failed to create FileBudgetService: %v", err)
	}

	if service == nil {
		t.Fatal("Expected service to be non-nil")
	}

	if service.configDir != configDir {
		t.Errorf("Expected configDir %s, got %s", configDir, service.configDir)
	}

	// Verify budgets were loaded
	if len(service.appBudgets) != 3 {
		t.Errorf("Expected 3 app budgets, got %d", len(service.appBudgets))
	}

	if len(service.llmBudgets) != 2 {
		t.Errorf("Expected 2 LLM budgets, got %d", len(service.llmBudgets))
	}
}

func TestNewFileBudgetService_InvalidDir(t *testing.T) {
	_, err := NewFileBudgetService("/nonexistent/directory")
	if err == nil {
		t.Fatal("Expected error for non-existent directory")
	}
}

func TestCheckBudget_WithinLimits(t *testing.T) {
	configDir := setupTestBudgetConfig(t)
	service, err := NewFileBudgetService(configDir)
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}

	// Test app and LLM within budget limits
	app := &models.App{ID: 1} // 25.50/100.0 = 25.5%
	llm := &models.LLM{ID: 1} // 45.25/200.0 = 22.625%

	appUsage, llmUsage, err := service.CheckBudget(app, llm)
	if err != nil {
		t.Fatalf("Expected no error for within-budget check, got: %v", err)
	}

	expectedAppUsage := 25.5
	if appUsage != expectedAppUsage {
		t.Errorf("Expected app usage %.1f%%, got %.1f%%", expectedAppUsage, appUsage)
	}

	expectedLLMUsage := 22.625
	if llmUsage != expectedLLMUsage {
		t.Errorf("Expected LLM usage %.3f%%, got %.3f%%", expectedLLMUsage, llmUsage)
	}
}

func TestCheckBudget_AppExceeded(t *testing.T) {
	configDir := setupTestBudgetConfig(t)
	service, err := NewFileBudgetService(configDir)
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}

	// Test app that has exceeded budget
	app := &models.App{ID: 3, Name: "Over Budget App"} // 55.0/50.0 = 110%
	llm := &models.LLM{ID: 1}

	appUsage, llmUsage, err := service.CheckBudget(app, llm)
	if err == nil {
		t.Fatal("Expected error for exceeded app budget")
	}

	if appUsage != 100.0 {
		t.Errorf("Expected app usage 100%% for exceeded budget, got %.1f%%", appUsage)
	}

	// Should still calculate LLM usage
	expectedLLMUsage := 0.0 // Since app budget was exceeded, LLM usage not calculated in error case
	if llmUsage != expectedLLMUsage {
		t.Errorf("Expected LLM usage %.1f%% when app exceeded, got %.1f%%", expectedLLMUsage, llmUsage)
	}
}

func TestCheckBudget_LLMExceeded(t *testing.T) {
	configDir := setupTestBudgetConfig(t)
	service, err := NewFileBudgetService(configDir)
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}

	// Test LLM that has exceeded budget
	app := &models.App{ID: 1}                          // 25.5% - within limits
	llm := &models.LLM{ID: 2, Name: "Over Budget LLM"} // 175.0/150.0 = 116.67%

	appUsage, llmUsage, err := service.CheckBudget(app, llm)
	if err == nil {
		t.Fatal("Expected error for exceeded LLM budget")
	}

	expectedAppUsage := 25.5
	if appUsage != expectedAppUsage {
		t.Errorf("Expected app usage %.1f%% when LLM exceeded, got %.1f%%", expectedAppUsage, appUsage)
	}

	if llmUsage != 100.0 {
		t.Errorf("Expected LLM usage 100%% for exceeded budget, got %.1f%%", llmUsage)
	}
}

func TestCheckBudget_NonExistentBudgets(t *testing.T) {
	configDir := setupTestBudgetConfig(t)
	service, err := NewFileBudgetService(configDir)
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}

	// Test with non-existent app and LLM IDs
	app := &models.App{ID: 999}
	llm := &models.LLM{ID: 999}

	appUsage, llmUsage, err := service.CheckBudget(app, llm)
	if err != nil {
		t.Fatalf("Expected no error for non-existent budgets, got: %v", err)
	}

	// Should return 0 usage for non-existent budgets
	if appUsage != 0.0 {
		t.Errorf("Expected 0%% app usage for non-existent budget, got %.1f%%", appUsage)
	}

	if llmUsage != 0.0 {
		t.Errorf("Expected 0%% LLM usage for non-existent budget, got %.1f%%", llmUsage)
	}
}

func TestAnalyzeBudgetUsage(t *testing.T) {
	configDir := setupTestBudgetConfig(t)
	service, err := NewFileBudgetService(configDir)
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}

	// Test analyze budget usage for different scenarios
	testCases := []struct {
		appID       uint
		llmID       uint
		appName     string
		llmName     string
		description string
	}{
		{1, 1, "Normal App", "Normal LLM", "Within budget limits"},
		{2, 1, "High Usage App", "Normal LLM", "App near budget limit"},
		{3, 2, "Over Budget App", "Over Budget LLM", "Both over budget"},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			app := &models.App{ID: tc.appID, Name: tc.appName}
			llm := &models.LLM{ID: tc.llmID, Name: tc.llmName}

			// This should not panic and should handle different budget states
			service.AnalyzeBudgetUsage(app, llm)
		})
	}
}

func TestAddUsage(t *testing.T) {
	configDir := setupTestBudgetConfig(t)
	service, err := NewFileBudgetService(configDir)
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}

	initialAppUsage := service.appBudgets[1].CurrentUsage // App ID 1
	initialLLMUsage := service.llmBudgets[1].CurrentUsage // LLM ID 1

	cost := 5.25
	service.AddUsage(1, 1, cost)

	// Verify usage was added
	newAppUsage := service.appBudgets[1].CurrentUsage
	newLLMUsage := service.llmBudgets[1].CurrentUsage

	expectedAppUsage := initialAppUsage + cost
	if newAppUsage != expectedAppUsage {
		t.Errorf("Expected app usage %f, got %f", expectedAppUsage, newAppUsage)
	}

	expectedLLMUsage := initialLLMUsage + cost
	if newLLMUsage != expectedLLMUsage {
		t.Errorf("Expected LLM usage %f, got %f", expectedLLMUsage, newLLMUsage)
	}
}

func TestAddUsage_NonExistentBudgets(t *testing.T) {
	configDir := setupTestBudgetConfig(t)
	service, err := NewFileBudgetService(configDir)
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}

	// Adding usage to non-existent budgets should not panic
	service.AddUsage(999, 999, 10.0)

	// Should not create new budgets
	if len(service.appBudgets) != 3 {
		t.Errorf("Expected 3 app budgets, got %d", len(service.appBudgets))
	}
	if len(service.llmBudgets) != 2 {
		t.Errorf("Expected 2 LLM budgets, got %d", len(service.llmBudgets))
	}
}

func TestGetAppBudget(t *testing.T) {
	configDir := setupTestBudgetConfig(t)
	service, err := NewFileBudgetService(configDir)
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}

	// Test existing budget
	budget, err := service.GetAppBudget(1)
	if err != nil {
		t.Fatalf("Failed to get app budget: %v", err)
	}

	if budget == nil {
		t.Fatal("Expected budget to be non-nil")
	}

	if budget.AppID != 1 {
		t.Errorf("Expected app ID 1, got %d", budget.AppID)
	}

	if budget.MonthlyLimit != 100.0 {
		t.Errorf("Expected monthly limit 100.0, got %f", budget.MonthlyLimit)
	}

	// Test non-existent budget
	_, err = service.GetAppBudget(999)
	if err == nil {
		t.Fatal("Expected error for non-existent app budget")
	}
}

func TestGetLLMBudget(t *testing.T) {
	configDir := setupTestBudgetConfig(t)
	service, err := NewFileBudgetService(configDir)
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}

	// Test existing budget
	budget, err := service.GetLLMBudget(1)
	if err != nil {
		t.Fatalf("Failed to get LLM budget: %v", err)
	}

	if budget == nil {
		t.Fatal("Expected budget to be non-nil")
	}

	if budget.LLMID != 1 {
		t.Errorf("Expected LLM ID 1, got %d", budget.LLMID)
	}

	if budget.MonthlyLimit != 200.0 {
		t.Errorf("Expected monthly limit 200.0, got %f", budget.MonthlyLimit)
	}

	// Test non-existent budget
	_, err = service.GetLLMBudget(999)
	if err == nil {
		t.Fatal("Expected error for non-existent LLM budget")
	}
}

func TestSaveBudgets(t *testing.T) {
	configDir := setupTestBudgetConfig(t)
	service, err := NewFileBudgetService(configDir)
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}

	// Modify some budget data
	service.AddUsage(1, 1, 10.0)

	// Save budgets
	err = service.SaveBudgets()
	if err != nil {
		t.Fatalf("Failed to save budgets: %v", err)
	}

	// Verify file was written
	budgetFile := filepath.Join(configDir, "budgets.json")
	if _, err := os.Stat(budgetFile); os.IsNotExist(err) {
		t.Fatal("Expected budgets.json to be written")
	}

	// Create new service and verify data persisted
	newService, err := NewFileBudgetService(configDir)
	if err != nil {
		t.Fatalf("Failed to create new service: %v", err)
	}

	// Check that usage was persisted
	budget, err := newService.GetAppBudget(1)
	if err != nil {
		t.Fatalf("Failed to get app budget from new service: %v", err)
	}

	expectedUsage := 35.5 // Original 25.5 + added 10.0
	if budget.CurrentUsage != expectedUsage {
		t.Errorf("Expected current usage %f, got %f", expectedUsage, budget.CurrentUsage)
	}
}

func TestBudgetReload(t *testing.T) {
	configDir := setupTestBudgetConfig(t)
	service, err := NewFileBudgetService(configDir)
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}

	// Modify in-memory data
	service.AddUsage(1, 1, 50.0)

	// Reload should reset to file data
	err = service.Reload()
	if err != nil {
		t.Fatalf("Failed to reload: %v", err)
	}

	// Check that data was reset to original values
	budget, err := service.GetAppBudget(1)
	if err != nil {
		t.Fatalf("Failed to get app budget after reload: %v", err)
	}

	originalUsage := 25.5
	if budget.CurrentUsage != originalUsage {
		t.Errorf("Expected original usage %f after reload, got %f", originalUsage, budget.CurrentUsage)
	}
}

func TestBudgetDateParsing(t *testing.T) {
	tempDir := t.TempDir()

	testDate := "2025-07-15T14:30:00Z"
	budgetConfig := `{
		"app_budgets": [
			{
				"app_id": 1,
				"monthly_limit": 100.0,
				"current_usage": 25.0,
				"currency": "USD",
				"reset_date": "` + testDate + `",
				"notifications": {}
			}
		],
		"llm_budgets": [
			{
				"llm_id": 1,
				"monthly_limit": 200.0,
				"current_usage": 50.0,
				"currency": "USD",
				"reset_date": "` + testDate + `"
			}
		]
	}`

	if err := os.WriteFile(filepath.Join(tempDir, "budgets.json"), []byte(budgetConfig), 0644); err != nil {
		t.Fatalf("Failed to write budgets.json: %v", err)
	}

	service, err := NewFileBudgetService(tempDir)
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}

	expectedTime, _ := time.Parse(time.RFC3339, testDate)

	// Check app budget date
	appBudget, err := service.GetAppBudget(1)
	if err != nil {
		t.Fatalf("Failed to get app budget: %v", err)
	}

	if !appBudget.ResetDate.Equal(expectedTime) {
		t.Errorf("Expected app reset date %v, got %v", expectedTime, appBudget.ResetDate)
	}

	// Check LLM budget date
	llmBudget, err := service.GetLLMBudget(1)
	if err != nil {
		t.Fatalf("Failed to get LLM budget: %v", err)
	}

	if !llmBudget.ResetDate.Equal(expectedTime) {
		t.Errorf("Expected LLM reset date %v, got %v", expectedTime, llmBudget.ResetDate)
	}
}

func TestNotificationsHandling(t *testing.T) {
	configDir := setupTestBudgetConfig(t)
	service, err := NewFileBudgetService(configDir)
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}

	// Check that notifications are properly loaded
	budget, err := service.GetAppBudget(2)
	if err != nil {
		t.Fatalf("Failed to get app budget: %v", err)
	}

	// App 2 should have notifications for 50%, 80%, 90% thresholds
	if !budget.Notifications["50_percent"] {
		t.Error("Expected 50_percent notification to be true")
	}
	if !budget.Notifications["80_percent"] {
		t.Error("Expected 80_percent notification to be true")
	}
	if !budget.Notifications["90_percent"] {
		t.Error("Expected 90_percent notification to be true")
	}
	if budget.Notifications["100_percent"] {
		t.Error("Expected 100_percent notification to be false")
	}
}

func TestConcurrentAccess(t *testing.T) {
	configDir := setupTestBudgetConfig(t)
	service, err := NewFileBudgetService(configDir)
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}

	// Test concurrent access to ensure thread safety
	done := make(chan bool, 10)

	// Launch multiple goroutines that read and write budget data
	for i := 0; i < 10; i++ {
		go func(id int) {
			defer func() { done <- true }()

			// Perform various operations
			service.AddUsage(1, 1, 1.0)
			_, _ = service.GetAppBudget(1)
			_, _ = service.GetLLMBudget(1)

			app := &models.App{ID: 1}
			llm := &models.LLM{ID: 1}
			_, _, _ = service.CheckBudget(app, llm)
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify final state is consistent
	budget, err := service.GetAppBudget(1)
	if err != nil {
		t.Fatalf("Failed to get final app budget: %v", err)
	}

	// Should have added 10.0 (10 goroutines * 1.0 each) to original 25.5
	expectedFinalUsage := 35.5
	if budget.CurrentUsage != expectedFinalUsage {
		t.Errorf("Expected final usage %f, got %f", expectedFinalUsage, budget.CurrentUsage)
	}
}
