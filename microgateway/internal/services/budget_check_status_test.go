//go:build enterprise
// +build enterprise

package services

import (
	"errors"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/v2/models"
	coresvc "github.com/TykTechnologies/midsommar/v2/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckBudgetStatus_ReturnsSpendAndLimit(t *testing.T) {
	db, repo := setupBudgetServiceTestDB(t)
	svc := NewDatabaseBudgetService(db, repo, nil).(*DatabaseBudgetService)

	app := &database.App{Name: "a", IsActive: true, MonthlyBudget: 100}
	require.NoError(t, db.Create(app).Error)
	now := time.Now()
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	require.NoError(t, db.Create(&database.BudgetUsage{AppID: app.ID, PeriodStart: start,
		PeriodEnd: start.AddDate(0, 1, 0).Add(-time.Second), TotalCost: 250000}).Error)

	usage, limit, err := svc.CheckBudgetStatus(app.ID, nil, 0)
	require.NoError(t, err)
	assert.InDelta(t, 25.0, usage, 0.001)
	assert.Equal(t, 100.0, limit)

	_, _, err = svc.CheckBudgetStatus(app.ID, nil, 80)
	require.Error(t, err)
	assert.False(t, errors.Is(err, coresvc.ErrBudgetCheckUnavailable), "an exceeded budget is a decision, not an outage")
}

// A lookup that fails for any reason other than "no such app" means the check
// could not run. The request is still refused, but as unavailable, not as a
// spent budget.
func TestCheckBudgetStatus_LookupFailureIsUnavailable(t *testing.T) {
	db, repo := setupBudgetServiceTestDB(t)
	svc := NewDatabaseBudgetService(db, repo, nil).(*DatabaseBudgetService)
	app := &database.App{Name: "a", IsActive: true, MonthlyBudget: 100}
	require.NoError(t, db.Create(app).Error)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	_, _, err = svc.CheckBudgetStatus(app.ID, nil, 0)
	require.Error(t, err)
	assert.True(t, errors.Is(err, coresvc.ErrBudgetCheckUnavailable), "got %v", err)
}

func TestCheckBudgetStatus_MissingAppIsARefusal(t *testing.T) {
	db, repo := setupBudgetServiceTestDB(t)
	svc := NewDatabaseBudgetService(db, repo, nil).(*DatabaseBudgetService)

	_, _, err := svc.CheckBudgetStatus(999, nil, 0)
	require.Error(t, err)
	assert.False(t, errors.Is(err, coresvc.ErrBudgetCheckUnavailable))
}

// The adapter answers from the single-pass check, and passes the
// unavailable marker through to the proxy.
func TestBudgetServiceAdapter_UsesSinglePassCheck(t *testing.T) {
	db, repo := setupBudgetServiceTestDB(t)
	svc := NewDatabaseBudgetService(db, repo, nil).(*DatabaseBudgetService)
	adapter := NewBudgetServiceAdapter(svc, nil)

	app := &database.App{Name: "a", IsActive: true, MonthlyBudget: 50}
	require.NoError(t, db.Create(app).Error)

	usage, limit, err := adapter.CheckBudget(&models.App{ID: app.ID}, nil)
	require.NoError(t, err)
	assert.Equal(t, 0.0, usage)
	assert.Equal(t, 50.0, limit)

	sqlDB, _ := db.DB()
	require.NoError(t, sqlDB.Close())
	_, _, err = adapter.CheckBudget(&models.App{ID: app.ID}, nil)
	assert.True(t, errors.Is(err, coresvc.ErrBudgetCheckUnavailable), "got %v", err)
}
