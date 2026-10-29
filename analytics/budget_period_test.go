package analytics

import (
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services/budget"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The budget dashboard's monthly figure is the spend in each App's and LLM's
// own budget period (budget.PeriodStart of its budget_start_date), the figure
// enforcement blocks on, not the UTC calendar month. TotalCost still covers
// the requested range.
func TestGetBudgetUsage_MonthlyUsageIsTheBudgetPeriod(t *testing.T) {
	db := setupStatsTest(t)
	now := time.Now()
	anchor := now.AddDate(0, 0, -5)
	start := budget.PeriodStart(&anchor, now)
	require.True(t, start.After(now.AddDate(0, 0, -6)), "the period starts on the anchor day, 5 days ago")

	limit := 100.0
	app := models.App{Name: "anchored", MonthlyBudget: &limit, BudgetStartDate: &anchor}
	require.NoError(t, db.Create(&app).Error)
	llm := models.LLM{Name: "anchored-llm", Vendor: models.OPENAI, MonthlyBudget: &limit, BudgetStartDate: &anchor}
	require.NoError(t, db.Create(&llm).Error)
	for _, r := range []struct {
		at   time.Time
		cost float64
	}{
		{start.Add(-time.Hour), 70000}, // $7, the previous period
		{start.Add(time.Hour), 30000},  // $3, this period
	} {
		require.NoError(t, db.Create(&models.LLMChatRecord{AppID: app.ID, LLMID: llm.ID, Cost: r.cost, TimeStamp: r.at}).Error)
	}

	from, to := now.AddDate(0, 0, -60), now
	usages, err := GetBudgetUsage(db, &from, &to, nil)
	require.NoError(t, err)
	seen := 0
	for _, u := range usages {
		if (u.EntityType == "App" && u.EntityID == app.ID) || (u.EntityType == "LLM" && u.EntityID == llm.ID) {
			seen++
			assert.InDelta(t, 3, u.Spent, 1e-9, "%s spent", u.EntityType)
			assert.InDelta(t, 3, u.Usage, 1e-9, "%s usage %%", u.EntityType)
			assert.InDelta(t, 10, u.TotalCost, 1e-9, "%s total cost", u.EntityType)
		}
	}
	assert.Equal(t, 2, seen)
}
