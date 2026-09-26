//go:build enterprise
// +build enterprise

package services

import (
	"testing"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The budget check runs before every request and the usage recording after
// it; both need the App's budget fields, which come from the cache until the
// App changes.
func TestBudgetAppReadCachedUntilAppChanges(t *testing.T) {
	db, selects := openCountingDB(t)
	repo := database.NewRepository(db)
	svc := NewDatabaseBudgetService(db, repo, nil).(*DatabaseBudgetService)

	app := &database.App{Name: "budgeted", IsActive: true, MonthlyBudget: 10}
	require.NoError(t, db.Create(app).Error)
	before := selects("apps")

	for i := 0; i < 5; i++ {
		_, limit, err := svc.CheckBudgetStatus(app.ID, nil, 0)
		require.NoError(t, err)
		assert.Equal(t, 10.0, limit)
		require.NoError(t, svc.RecordUsage(app.ID, nil, 10, 1000, 5, 5))
	}
	assert.EqualValues(t, 1, selects("apps")-before, "one App read for all checks and recordings")

	// Recorded spend is seen by the check.
	usage, limit, err := svc.CheckBudgetStatus(app.ID, nil, 0)
	require.NoError(t, err)
	assert.Equal(t, 10.0, limit)
	assert.InDelta(t, 0.5, usage, 1e-9) // 5 x 1000 stored units = $0.50

	require.NoError(t, db.Model(&database.App{}).Where("id = ?", app.ID).Update("monthly_budget", 0.25).Error)
	_, _, err = svc.CheckBudgetStatus(app.ID, nil, 0)
	assert.Error(t, err, "a lowered budget applies at once")

	require.NoError(t, db.Model(&database.App{}).Where("id = ?", app.ID).Update("is_active", false).Error)
	_, _, err = svc.CheckBudgetStatus(app.ID, nil, 0)
	assert.ErrorContains(t, err, "app not found or inactive")

	_, _, err = svc.CheckBudgetStatus(app.ID+100, nil, 0)
	assert.ErrorContains(t, err, "app not found or inactive")
}
