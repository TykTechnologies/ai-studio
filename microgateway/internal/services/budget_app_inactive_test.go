//go:build enterprise
// +build enterprise

package services

import (
	"errors"
	"testing"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// A config push drops an inactive App from the edge while its token can still
// be cached, so the budget check can be the first to notice the App is gone.
// It must say so with services.ErrAppInactive; otherwise the proxy reports the
// refusal as "Budget limit exceeded".
func TestCheckBudgetStatus_InactiveOrMissingAppIsErrAppInactive(t *testing.T) {
	db, _ := openCountingDB(t)
	svc := NewDatabaseBudgetService(db, database.NewRepository(db), nil).(*DatabaseBudgetService)

	app := &database.App{Name: "gone", IsActive: true, MonthlyBudget: 10}
	require.NoError(t, db.Create(app).Error)
	require.NoError(t, db.Model(&database.App{}).Where("id = ?", app.ID).Update("is_active", false).Error)

	for _, id := range []uint{app.ID, app.ID + 100} {
		_, _, err := svc.CheckBudgetStatus(id, nil, 0)
		require.Error(t, err)
		require.True(t, errors.Is(err, services.ErrAppInactive), "app %d: %v", id, err)
		require.True(t, errors.Is(err, gorm.ErrRecordNotFound), "callers that test for a missing record must still see one: %v", err)
		require.False(t, errors.Is(err, services.ErrBudgetCheckUnavailable), "an inactive App is a decision, not a failed check")
	}
}
