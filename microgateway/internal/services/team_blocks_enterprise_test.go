//go:build enterprise
// +build enterprise

package services

import (
	"testing"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckBudget_TeamBlockRefusesUncappedApp(t *testing.T) {
	db, repo := setupBudgetServiceTestDB(t)
	svc := NewDatabaseBudgetService(db, repo, nil).(*DatabaseBudgetService)
	svc.teamBlocks = &TeamBlocks{blocks: map[uint]string{}}

	app := &database.App{Name: "team app", IsActive: true}
	require.NoError(t, db.Create(app).Error)
	require.NoError(t, svc.CheckBudget(app.ID, nil, 0))

	svc.teamBlocks.Replace(map[uint]string{app.ID: "team allocation exhausted"})
	err := svc.CheckBudget(app.ID, nil, 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "team allocation exhausted")

	svc.teamBlocks.Replace(nil)
	assert.NoError(t, svc.CheckBudget(app.ID, nil, 0))
}
