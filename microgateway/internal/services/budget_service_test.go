// internal/services/budget_service_test.go
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

func setupBudgetTestDB(t *testing.T) (*gorm.DB, *database.Repository) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Migrate
	err = database.Migrate(db)
	require.NoError(t, err)

	repo := database.NewRepository(db)
	return db, repo
}

func TestDatabaseBudgetService_CheckBudget(t *testing.T) {
	db, repo := setupBudgetTestDB(t)
	defer database.Close(db)

	service := NewDatabaseBudgetService(db, repo, nil).(*DatabaseBudgetService)

	// Create test app with budget
	app := &database.App{
		Name:          "Test App",
		OwnerEmail:    "test@example.com",
		IsActive:      true,
		MonthlyBudget: 100.0,
	}
	repo.CreateApp(app)

	t.Run("WithinBudget", func(t *testing.T) {
		err := service.CheckBudget(app.ID, nil, 10.0)
		assert.NoError(t, err)
	})

	t.Run("NoBudgetLimit", func(t *testing.T) {
		// Create app with no budget
		noBudgetApp := &database.App{
			Name:          "No Budget App",
			OwnerEmail:    "test@example.com",
			IsActive:      true,
			MonthlyBudget: 0, // No budget limit
		}
		repo.CreateApp(noBudgetApp)

		err := service.CheckBudget(noBudgetApp.ID, nil, 1000.0)
		assert.NoError(t, err) // Should allow any amount
	})

	t.Run("OverBudget", func(t *testing.T) {
		// Create usage that puts us over budget
		now := time.Now()
		periodStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		periodEnd := periodStart.AddDate(0, 1, 0).Add(-time.Second)
		
		usage, err := repo.GetOrCreateBudgetUsage(app.ID, nil, periodStart, periodEnd)
		require.NoError(t, err)
		
		// Set existing usage to 90.0, request 20.0 more (would be 110.0 > 100.0 budget)
		repo.UpdateBudgetUsage(usage.ID, 1000, 10, 90.0, 800, 200)
		
		err = service.CheckBudget(app.ID, nil, 20.0)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "budget exceeded")
	})

	t.Run("AppNotFound", func(t *testing.T) {
		err := service.CheckBudget(999, nil, 10.0)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "app not found")
	})
}

func TestDatabaseBudgetService_RecordUsage(t *testing.T) {
	db, repo := setupBudgetTestDB(t)
	defer database.Close(db)

	service := NewDatabaseBudgetService(db, repo, nil).(*DatabaseBudgetService)

	// Create test app
	app := &database.App{
		Name:       "Test App",
		OwnerEmail: "test@example.com",
		IsActive:   true,
	}
	repo.CreateApp(app)

	t.Run("RecordUsage", func(t *testing.T) {
		err := service.RecordUsage(app.ID, nil, 1000, 5.50, 800, 200)
		assert.NoError(t, err)

		// Verify usage was recorded
		now := time.Now()
		periodStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		periodEnd := periodStart.AddDate(0, 1, 0).Add(-time.Second)
		
		usage, err := repo.GetBudgetUsage(app.ID, nil, periodStart, periodEnd)
		assert.NoError(t, err)
		assert.Equal(t, int64(1000), usage.TokensUsed)
		assert.Equal(t, 1, usage.RequestsCount)
		assert.Equal(t, 5.50, usage.TotalCost)
	})

	t.Run("MultipleRecords", func(t *testing.T) {
		// Record multiple usage entries
		service.RecordUsage(app.ID, nil, 500, 2.75, 400, 100)
		service.RecordUsage(app.ID, nil, 300, 1.65, 250, 50)

		// Verify cumulative usage
		now := time.Now()
		periodStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		periodEnd := periodStart.AddDate(0, 1, 0).Add(-time.Second)
		
		usage, err := repo.GetBudgetUsage(app.ID, nil, periodStart, periodEnd)
		assert.NoError(t, err)
		assert.Equal(t, int64(1800), usage.TokensUsed) // 1000 + 500 + 300
		assert.Equal(t, 3, usage.RequestsCount) // 1 + 1 + 1  
		assert.Equal(t, 9.90, usage.TotalCost) // 5.50 + 2.75 + 1.65
	})
}

func TestDatabaseBudgetService_GetBudgetStatus(t *testing.T) {
	db, repo := setupBudgetTestDB(t)
	defer database.Close(db)

	service := NewDatabaseBudgetService(db, repo, nil).(*DatabaseBudgetService)

	// Create test app with budget
	app := &database.App{
		Name:          "Test App",
		OwnerEmail:    "test@example.com",
		IsActive:      true,
		MonthlyBudget: 200.0,
	}
	repo.CreateApp(app)

	// Record some usage
	service.RecordUsage(app.ID, nil, 2000, 50.0, 1600, 400)

	t.Run("GetStatus", func(t *testing.T) {
		status, err := service.GetBudgetStatus(app.ID, nil)
		assert.NoError(t, err)
		assert.Equal(t, app.ID, status.AppID)
		assert.Equal(t, 200.0, status.MonthlyBudget)
		assert.Equal(t, 50.0, status.CurrentUsage)
		assert.Equal(t, 150.0, status.RemainingBudget)
		assert.Equal(t, int64(2000), status.TokensUsed)
		assert.Equal(t, 1, status.RequestsCount)
		assert.False(t, status.IsOverBudget)
		assert.Equal(t, 25.0, status.PercentageUsed) // 50/200 * 100
	})
}

func TestDatabaseBudgetService_UpdateBudget(t *testing.T) {
	db, repo := setupBudgetTestDB(t)
	defer database.Close(db)

	service := NewDatabaseBudgetService(db, repo, nil).(*DatabaseBudgetService)

	// Create test app
	app := &database.App{
		Name:          "Test App",
		OwnerEmail:    "test@example.com",
		IsActive:      true,
		MonthlyBudget: 100.0,
	}
	repo.CreateApp(app)

	t.Run("UpdateBudget", func(t *testing.T) {
		err := service.UpdateBudget(app.ID, 300.0, 15)
		assert.NoError(t, err)

		// Verify update
		updatedApp, err := repo.GetApp(app.ID)
		assert.NoError(t, err)
		assert.Equal(t, 300.0, updatedApp.MonthlyBudget)
		assert.Equal(t, 15, updatedApp.BudgetResetDay)
	})

	t.Run("AppNotFound", func(t *testing.T) {
		err := service.UpdateBudget(999, 300.0, 15)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "app not found")
	})
}