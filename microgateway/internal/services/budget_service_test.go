//go:build enterprise
// +build enterprise

// Package services budget_service_test.go
//
// These tests verify budget enforcement behavior, particularly focusing on the
// edge case where budget is set to 0 (which should mean "no budget limit").
//
// BUG CAPTURED: TestGetBudgetStatus_ZeroBudget
// When an app's MonthlyBudget is 0 (meaning "no limit"), the GetBudgetStatus
// function incorrectly sets IsOverBudget = true whenever currentUsage > 0.
// This is because the logic uses `currentUsage > monthlyBudget` which evaluates
// to true when monthlyBudget is 0 and there's any usage.
//
// The bug can manifest when:
// 1. An app's budget is synced from control server as 0 (to remove budget limit)
// 2. The app has existing usage from before
// 3. The IsOverBudget flag incorrectly shows as true in status displays
//
// Note: The actual enforcement (CheckBudget) correctly handles this case by
// returning early when budget <= 0. The bug is only in the status reporting.
//
// Fix location: budget_service.go line 181 - IsOverBudget calculation should
// check if monthlyBudget > 0 before comparing usage.

package services

import (
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupBudgetServiceTestDB(t *testing.T) (*gorm.DB, *database.Repository) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = database.Migrate(db)
	require.NoError(t, err)

	repo := database.NewRepository(db)
	return db, repo
}

// TestCheckBudget_ZeroBudgetShouldNotEnforce tests that a budget of 0 is treated
// as "no budget limit" and does not block requests, even when there is existing usage.
//
// Bug scenario: When budget is synced from control server as 0 (which can happen
// during configuration sync), the microgateway should treat it as "unset" and allow
// all requests. Previously, a 0 budget with any existing usage would incorrectly
// trigger "budget exceeded" because currentUsage > 0 > monthlyBudget (0).
func TestCheckBudget_ZeroBudgetShouldNotEnforce(t *testing.T) {
	db, repo := setupBudgetServiceTestDB(t)
	budgetService := NewDatabaseBudgetService(db, repo, nil)

	// Create an app with MonthlyBudget = 0 (meaning "no budget limit")
	app := &database.App{
		Name:          "App with Zero Budget",
		IsActive:      true,
		MonthlyBudget: 0, // Zero budget = no limit
	}
	err := db.Create(app).Error
	require.NoError(t, err)

	// Create existing usage for the app (simulating previous requests)
	now := time.Now()
	periodStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	periodEnd := periodStart.AddDate(0, 1, 0).Add(-time.Second)

	existingUsage := &database.BudgetUsage{
		AppID:            app.ID,
		PeriodStart:      periodStart,
		PeriodEnd:        periodEnd,
		TotalCost:        500000.0, // $50.00 (in stored format: dollars * 10000)
		TokensUsed:       10000,
		RequestsCount:    100,
		PromptTokens:     8000,
		CompletionTokens: 2000,
	}
	err = db.Create(existingUsage).Error
	require.NoError(t, err)

	// CheckBudget should return nil (no error) because budget of 0 means "no limit"
	// BUG: If this test fails, it means the microgateway is incorrectly enforcing
	// a budget of 0 as "no money allowed" rather than "no budget limit set"
	err = budgetService.CheckBudget(app.ID, nil, 0.0)
	assert.NoError(t, err, "Budget of 0 should be treated as 'no limit', not 'no money allowed'")

	// Also test with an estimated cost - should still allow
	err = budgetService.CheckBudget(app.ID, nil, 10.0)
	assert.NoError(t, err, "Budget of 0 should allow requests regardless of estimated cost")
}

// TestCheckBudget_NegativeBudgetShouldNotEnforce tests that negative budgets
// (which could theoretically occur from calculation errors) are also treated as "no limit"
func TestCheckBudget_NegativeBudgetShouldNotEnforce(t *testing.T) {
	db, repo := setupBudgetServiceTestDB(t)
	budgetService := NewDatabaseBudgetService(db, repo, nil)

	// Create an app with a negative budget (edge case)
	app := &database.App{
		Name:          "App with Negative Budget",
		IsActive:      true,
		MonthlyBudget: -100.0, // Negative budget - should be treated as "no limit"
	}
	err := db.Create(app).Error
	require.NoError(t, err)

	// CheckBudget should return nil because budget <= 0 means "no limit"
	err = budgetService.CheckBudget(app.ID, nil, 0.0)
	assert.NoError(t, err, "Negative budget should be treated as 'no limit'")
}

// TestCheckBudget_PositiveBudgetEnforcesLimit tests that a positive budget correctly
// enforces the limit (this is the normal case that should work)
func TestCheckBudget_PositiveBudgetEnforcesLimit(t *testing.T) {
	db, repo := setupBudgetServiceTestDB(t)
	budgetService := NewDatabaseBudgetService(db, repo, nil)

	// Create an app with a $100 monthly budget
	app := &database.App{
		Name:          "App with Budget",
		IsActive:      true,
		MonthlyBudget: 100.0, // $100 budget
	}
	err := db.Create(app).Error
	require.NoError(t, err)

	// Create usage that is within budget ($50 used)
	now := time.Now()
	periodStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	periodEnd := periodStart.AddDate(0, 1, 0).Add(-time.Second)

	existingUsage := &database.BudgetUsage{
		AppID:       app.ID,
		PeriodStart: periodStart,
		PeriodEnd:   periodEnd,
		TotalCost:   500000.0, // $50.00 (in stored format)
	}
	err = db.Create(existingUsage).Error
	require.NoError(t, err)

	// CheckBudget with small estimated cost should succeed (within budget)
	err = budgetService.CheckBudget(app.ID, nil, 10.0)
	assert.NoError(t, err, "Request within budget should be allowed")

	// CheckBudget with large estimated cost should fail (would exceed budget)
	// $50 used + $60 estimated = $110 > $100 budget
	err = budgetService.CheckBudget(app.ID, nil, 60.0)
	assert.Error(t, err, "Request exceeding budget should be blocked")
	assert.Contains(t, err.Error(), "budget exceeded")
}

// TestCheckBudget_ZeroBudgetAfterSync tests the specific scenario where:
// 1. An app originally had a budget set
// 2. The budget was reset/synced to 0 from the control server
// 3. Existing usage from before the reset should not cause enforcement
func TestCheckBudget_ZeroBudgetAfterSync(t *testing.T) {
	db, repo := setupBudgetServiceTestDB(t)
	budgetService := NewDatabaseBudgetService(db, repo, nil)

	// Simulate the sync scenario:
	// App originally had budget of $100 and used $90
	// Now budget is synced to 0 (meaning "remove budget limit")
	app := &database.App{
		Name:          "App Synced from Control",
		IsActive:      true,
		MonthlyBudget: 0, // Budget reset to 0 via sync
	}
	err := db.Create(app).Error
	require.NoError(t, err)

	// Existing usage from before the budget was reset
	now := time.Now()
	periodStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	periodEnd := periodStart.AddDate(0, 1, 0).Add(-time.Second)

	existingUsage := &database.BudgetUsage{
		AppID:       app.ID,
		PeriodStart: periodStart,
		PeriodEnd:   periodEnd,
		TotalCost:   9000000.0, // $900.00 - large existing usage
	}
	err = db.Create(existingUsage).Error
	require.NoError(t, err)

	// Even with large existing usage, budget of 0 should allow requests
	err = budgetService.CheckBudget(app.ID, nil, 100.0)
	assert.NoError(t, err, "Budget of 0 should bypass enforcement regardless of existing usage")
}

// TestGetBudgetStatus_ZeroBudget tests that GetBudgetStatus correctly handles
// a zero budget (for status display, not enforcement)
func TestGetBudgetStatus_ZeroBudget(t *testing.T) {
	db, repo := setupBudgetServiceTestDB(t)
	budgetService := NewDatabaseBudgetService(db, repo, nil)

	// Create an app with zero budget
	app := &database.App{
		Name:          "App with Zero Budget",
		IsActive:      true,
		MonthlyBudget: 0,
	}
	err := db.Create(app).Error
	require.NoError(t, err)

	// Create some usage
	now := time.Now()
	periodStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	periodEnd := periodStart.AddDate(0, 1, 0).Add(-time.Second)

	existingUsage := &database.BudgetUsage{
		AppID:       app.ID,
		PeriodStart: periodStart,
		PeriodEnd:   periodEnd,
		TotalCost:   500000.0, // $50.00
	}
	err = db.Create(existingUsage).Error
	require.NoError(t, err)

	// GetBudgetStatus should work and show the current usage
	status, err := budgetService.GetBudgetStatus(app.ID, nil)
	require.NoError(t, err)
	require.NotNil(t, status)

	// The status should show:
	// - MonthlyBudget = 0
	// - CurrentUsage = $50.00
	// - IsOverBudget should be false when budget is 0 (no budget = can't be over)
	assert.Equal(t, 0.0, status.MonthlyBudget)
	assert.InDelta(t, 50.0, status.CurrentUsage, 0.01)

	// BUG: IsOverBudget calculation uses currentUsage > monthlyBudget
	// When budget is 0, this incorrectly returns true (50 > 0)
	// Expected: IsOverBudget should be false when budget is 0 (no limit means can't be over)
	// This test documents the expected behavior - if it fails, IsOverBudget logic needs fixing
	assert.False(t, status.IsOverBudget, "With budget of 0 (no limit), IsOverBudget should be false")
}

// TestCheckBudget_InactiveApp tests that budget check fails for inactive apps
func TestCheckBudget_InactiveApp(t *testing.T) {
	db, repo := setupBudgetServiceTestDB(t)
	budgetService := NewDatabaseBudgetService(db, repo, nil)

	// Create an active app first, then deactivate it
	// This avoids GORM zero-value issues with booleans
	app := &database.App{
		Name:          "Inactive App",
		IsActive:      true,
		MonthlyBudget: 100.0,
	}
	err := db.Create(app).Error
	require.NoError(t, err)

	// Deactivate the app
	err = db.Model(app).Update("is_active", false).Error
	require.NoError(t, err)

	// CheckBudget should fail for inactive app
	err = budgetService.CheckBudget(app.ID, nil, 0.0)
	require.Error(t, err, "CheckBudget should fail for inactive apps")
	assert.Contains(t, err.Error(), "app not found or inactive")
}

// TestCheckBudget_NonExistentApp tests that budget check fails for non-existent apps
func TestCheckBudget_NonExistentApp(t *testing.T) {
	db, repo := setupBudgetServiceTestDB(t)
	budgetService := NewDatabaseBudgetService(db, repo, nil)

	// CheckBudget for non-existent app should fail
	err := budgetService.CheckBudget(99999, nil, 0.0)
	require.Error(t, err, "CheckBudget should fail for non-existent apps")
	assert.Contains(t, err.Error(), "app not found or inactive")
}
