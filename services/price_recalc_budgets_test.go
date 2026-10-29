package services

import (
	"sync/atomic"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/replicas"
	"github.com/TykTechnologies/midsommar/v2/services/budget"
	"github.com/TykTechnologies/midsommar/v2/services/team_budget"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type clearCountingBudget struct {
	budget.Service
	cleared atomic.Int32
}

func (b *clearCountingBudget) ClearCache() { b.cleared.Add(1) }

type clearCountingTeamBudget struct {
	team_budget.Service
	cleared atomic.Int32
}

func (b *clearCountingTeamBudget) ClearCache() { b.cleared.Add(1) }

// A recalculation rewrites the costs budget spend was summed from: it clears
// the budget and team budget caches here and raises spend_recalculated, for
// this replica's budget sync and (through the cluster log) the others.
func TestUpdateModelPriceAndRecalculate_InvalidatesBudgetSpend(t *testing.T) {
	s := setupTestService(t)
	b, tb := &clearCountingBudget{}, &clearCountingTeamBudget{}
	s.Budget, s.TeamBudget = b, tb
	var signalled atomic.Int32
	defer replicas.OnSignal(replicas.SignalSpendRecalculated, func() { signalled.Add(1) })()

	mp := &models.ModelPrice{ModelName: "gemini-2.5-flash", Vendor: "google_ai", CPT: 0.000001, CPIT: 0.000001, Currency: "USD"}
	require.NoError(t, mp.Create(s.DB))

	_, err := s.UpdateModelPriceAndRecalculate(mp.ID, mp.ModelName, mp.Vendor, 0.000002, 0.000002, 0, 0, "USD")
	require.NoError(t, err)
	assert.EqualValues(t, 1, b.cleared.Load())
	assert.EqualValues(t, 1, tb.cleared.Load())
	assert.EqualValues(t, 1, signalled.Load())

	// A failed recalculation changed nothing and invalidates nothing.
	_, err = s.UpdateModelPriceAndRecalculate(mp.ID+100, mp.ModelName, mp.Vendor, 0.000003, 0.000003, 0, 0, "USD")
	require.Error(t, err)
	assert.EqualValues(t, 1, b.cleared.Load())
	assert.EqualValues(t, 1, signalled.Load())
}
