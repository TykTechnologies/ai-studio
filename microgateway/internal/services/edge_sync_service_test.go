// internal/services/edge_sync_service_test.go
package services

import (
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupEdgeSyncTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Run migrations
	err = database.Migrate(db)
	require.NoError(t, err)

	return db
}

func TestEdgeSyncService_SyncConfiguration(t *testing.T) {
	db := setupEdgeSyncTestDB(t)
	namespace := "test-namespace"
	syncService := NewEdgeSyncService(db, namespace)

	now := time.Now()

	config := &pb.ConfigurationSnapshot{
		Version:       "1.0.0",
		EdgeNamespace: namespace,
		SnapshotTime:  timestamppb.Now(),
		Llms: []*pb.LLMConfig{
			{
				Id:           1,
				Name:         "Test LLM",
				Slug:         "test-llm",
				Vendor:       "openai",
				Endpoint:     "https://api.openai.com/v1",
				DefaultModel: "gpt-4",
				IsActive:     true,
				Namespace:    namespace,
				CreatedAt:    timestamppb.New(now),
				UpdatedAt:    timestamppb.New(now),
			},
		},
		Apps: []*pb.AppConfig{
			{
				Id:          1,
				Name:        "Test App",
				Description: "Test application",
				IsActive:    true,
				Namespace:   namespace,
				LlmIds:      []uint32{1},
				CreatedAt:   timestamppb.New(now),
				UpdatedAt:   timestamppb.New(now),
			},
		},
	}

	err := syncService.SyncConfiguration(config)
	require.NoError(t, err)

	// Verify LLM was synced
	var llm database.LLM
	err = db.First(&llm, 1).Error
	assert.NoError(t, err)
	assert.Equal(t, "Test LLM", llm.Name)
	assert.Equal(t, "openai", llm.Vendor)

	// Verify App was synced
	var app database.App
	err = db.First(&app, 1).Error
	assert.NoError(t, err)
	assert.Equal(t, "Test App", app.Name)

	// Verify app_llms join table was created
	var appLLM database.AppLLM
	err = db.Where("app_id = ? AND llm_id = ?", 1, 1).First(&appLLM).Error
	assert.NoError(t, err)
	assert.True(t, appLLM.IsActive)
}

func TestEdgeSyncService_SyncConfiguration_BudgetUsageInitialization(t *testing.T) {
	db := setupEdgeSyncTestDB(t)
	namespace := "test-budget"
	syncService := NewEdgeSyncService(db, namespace)

	now := time.Now()

	// Create a configuration with an app that has budget and current usage
	config := &pb.ConfigurationSnapshot{
		Version:       "1.0.0",
		EdgeNamespace: namespace,
		SnapshotTime:  timestamppb.Now(),
		Apps: []*pb.AppConfig{
			{
				Id:                 1,
				Name:               "Budget App",
				Description:        "App with budget tracking",
				IsActive:           true,
				Namespace:          namespace,
				MonthlyBudget:      100.0, // $100 budget
				CurrentPeriodUsage: 45.50, // $45.50 already spent (from control server)
				CreatedAt:          timestamppb.New(now),
				UpdatedAt:          timestamppb.New(now),
			},
		},
	}

	err := syncService.SyncConfiguration(config)
	require.NoError(t, err)

	// Verify App was synced with budget
	var app database.App
	err = db.First(&app, 1).Error
	assert.NoError(t, err)
	assert.Equal(t, "Budget App", app.Name)
	assert.Equal(t, 100.0, app.MonthlyBudget)

	// Verify BudgetUsage was initialized from CurrentPeriodUsage
	// Note: CurrentPeriodUsage is in dollars, but TotalCost is stored as dollars * 10000
	periodStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())

	var budgetUsage database.BudgetUsage
	err = db.Where("app_id = ? AND period_start = ?", 1, periodStart).First(&budgetUsage).Error
	assert.NoError(t, err, "BudgetUsage should be created for app with budget and usage")
	assert.Equal(t, uint(1), budgetUsage.AppID)
	// 45.50 dollars * 10000 = 455000
	assert.InDelta(t, 455000.0, budgetUsage.TotalCost, 1.0,
		"TotalCost should be CurrentPeriodUsage * 10000 (stored format)")
}

func TestEdgeSyncService_SyncConfiguration_BudgetUsageUpdate(t *testing.T) {
	db := setupEdgeSyncTestDB(t)
	namespace := "test-budget-update"
	syncService := NewEdgeSyncService(db, namespace)

	now := time.Now()
	periodStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	periodEnd := periodStart.AddDate(0, 1, 0).Add(-time.Second)

	// Pre-create an existing BudgetUsage record (simulating edge has been running)
	// Note: TotalCost is stored as dollars * 10000, so $20.00 = 200000
	existingUsage := &database.BudgetUsage{
		AppID:       1,
		PeriodStart: periodStart,
		PeriodEnd:   periodEnd,
		TotalCost:   200000.0, // Edge tracked $20.00 before sync (stored as dollars * 10000)
	}
	err := db.Create(existingUsage).Error
	require.NoError(t, err)

	// Now sync with new configuration from control server showing higher usage
	config := &pb.ConfigurationSnapshot{
		Version:       "1.0.0",
		EdgeNamespace: namespace,
		SnapshotTime:  timestamppb.Now(),
		Apps: []*pb.AppConfig{
			{
				Id:                 1,
				Name:               "Budget App",
				IsActive:           true,
				Namespace:          namespace,
				MonthlyBudget:      100.0,
				CurrentPeriodUsage: 75.00, // Control server says $75.00 spent (in dollars)
				CreatedAt:          timestamppb.New(now),
				UpdatedAt:          timestamppb.New(now),
			},
		},
	}

	err = syncService.SyncConfiguration(config)
	require.NoError(t, err)

	// Verify BudgetUsage was updated to control server's value
	// Note: 75.00 dollars * 10000 = 750000
	var budgetUsage database.BudgetUsage
	err = db.Where("app_id = ? AND period_start = ?", 1, periodStart).First(&budgetUsage).Error
	assert.NoError(t, err)
	assert.InDelta(t, 750000.0, budgetUsage.TotalCost, 1.0,
		"TotalCost should be updated to control server's CurrentPeriodUsage * 10000")
}

func TestEdgeSyncService_SyncConfiguration_NoBudget(t *testing.T) {
	db := setupEdgeSyncTestDB(t)
	namespace := "test-no-budget"
	syncService := NewEdgeSyncService(db, namespace)

	now := time.Now()

	// Create a configuration with an app WITHOUT budget
	config := &pb.ConfigurationSnapshot{
		Version:       "1.0.0",
		EdgeNamespace: namespace,
		SnapshotTime:  timestamppb.Now(),
		Apps: []*pb.AppConfig{
			{
				Id:                 1,
				Name:               "No Budget App",
				IsActive:           true,
				Namespace:          namespace,
				MonthlyBudget:      0, // No budget
				CurrentPeriodUsage: 0, // No usage
				CreatedAt:          timestamppb.New(now),
				UpdatedAt:          timestamppb.New(now),
			},
		},
	}

	err := syncService.SyncConfiguration(config)
	require.NoError(t, err)

	// Verify App was synced
	var app database.App
	err = db.First(&app, 1).Error
	assert.NoError(t, err)
	assert.Equal(t, "No Budget App", app.Name)

	// Verify NO BudgetUsage was created (app has no budget)
	periodStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	var budgetUsage database.BudgetUsage
	err = db.Where("app_id = ? AND period_start = ?", 1, periodStart).First(&budgetUsage).Error
	assert.Error(t, err, "BudgetUsage should NOT be created for app without budget")
	assert.Equal(t, gorm.ErrRecordNotFound, err)
}

func TestEdgeSyncService_SyncConfiguration_ZeroUsage(t *testing.T) {
	db := setupEdgeSyncTestDB(t)
	namespace := "test-zero-usage"
	syncService := NewEdgeSyncService(db, namespace)

	now := time.Now()

	// Create a configuration with an app that has budget but zero usage
	config := &pb.ConfigurationSnapshot{
		Version:       "1.0.0",
		EdgeNamespace: namespace,
		SnapshotTime:  timestamppb.Now(),
		Apps: []*pb.AppConfig{
			{
				Id:                 1,
				Name:               "New Budget App",
				IsActive:           true,
				Namespace:          namespace,
				MonthlyBudget:      50.0,  // Has budget
				CurrentPeriodUsage: 0,     // But no usage yet
				CreatedAt:          timestamppb.New(now),
				UpdatedAt:          timestamppb.New(now),
			},
		},
	}

	err := syncService.SyncConfiguration(config)
	require.NoError(t, err)

	// Verify App was synced
	var app database.App
	err = db.First(&app, 1).Error
	assert.NoError(t, err)
	assert.Equal(t, 50.0, app.MonthlyBudget)

	// Verify BudgetUsage WAS created with TotalCost = 0
	// This is intentional - we now always create BudgetUsage for apps with budget
	// so that budget enforcement is initialized even with zero usage
	periodStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	var budgetUsage database.BudgetUsage
	err = db.Where("app_id = ? AND period_start = ?", 1, periodStart).First(&budgetUsage).Error
	assert.NoError(t, err, "BudgetUsage should be created for app with budget (even with zero usage)")
	assert.Equal(t, 0.0, budgetUsage.TotalCost, "TotalCost should be 0 when CurrentPeriodUsage is 0")
}

func TestEdgeSyncService_SyncConfiguration_MultipleApps(t *testing.T) {
	db := setupEdgeSyncTestDB(t)
	namespace := "test-multi-apps"
	syncService := NewEdgeSyncService(db, namespace)

	now := time.Now()

	// Create configuration with multiple apps with different budget states
	config := &pb.ConfigurationSnapshot{
		Version:       "1.0.0",
		EdgeNamespace: namespace,
		SnapshotTime:  timestamppb.Now(),
		Apps: []*pb.AppConfig{
			{
				Id:                 1,
				Name:               "App With Budget And Usage",
				IsActive:           true,
				Namespace:          namespace,
				MonthlyBudget:      100.0,
				CurrentPeriodUsage: 30.0,
				CreatedAt:          timestamppb.New(now),
				UpdatedAt:          timestamppb.New(now),
			},
			{
				Id:                 2,
				Name:               "App With Budget No Usage",
				IsActive:           true,
				Namespace:          namespace,
				MonthlyBudget:      50.0,
				CurrentPeriodUsage: 0,
				CreatedAt:          timestamppb.New(now),
				UpdatedAt:          timestamppb.New(now),
			},
			{
				Id:                 3,
				Name:               "App No Budget",
				IsActive:           true,
				Namespace:          namespace,
				MonthlyBudget:      0,
				CurrentPeriodUsage: 0,
				CreatedAt:          timestamppb.New(now),
				UpdatedAt:          timestamppb.New(now),
			},
		},
	}

	err := syncService.SyncConfiguration(config)
	require.NoError(t, err)

	periodStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())

	// App 1: Should have BudgetUsage with $30.00 * 10000 = 300000
	var usage1 database.BudgetUsage
	err = db.Where("app_id = ? AND period_start = ?", 1, periodStart).First(&usage1).Error
	assert.NoError(t, err, "App 1 should have BudgetUsage")
	assert.InDelta(t, 300000.0, usage1.TotalCost, 1.0)

	// App 2: Should have BudgetUsage with TotalCost = 0 (has budget, zero usage)
	var usage2 database.BudgetUsage
	err = db.Where("app_id = ? AND period_start = ?", 2, periodStart).First(&usage2).Error
	assert.NoError(t, err, "App 2 should have BudgetUsage (has budget, even with zero usage)")
	assert.Equal(t, 0.0, usage2.TotalCost, "App 2 TotalCost should be 0")

	// App 3: Should NOT have BudgetUsage (no budget)
	var usage3 database.BudgetUsage
	err = db.Where("app_id = ? AND period_start = ?", 3, periodStart).First(&usage3).Error
	assert.Error(t, err, "App 3 should NOT have BudgetUsage (no budget)")
}
