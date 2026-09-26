package grpc

import (
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func naiveSpend(t *testing.T, db *gorm.DB, app models.App, now time.Time) float64 {
	t.Helper()
	start, end := calculateBudgetPeriod(app.BudgetStartDate, now)
	var total float64
	require.NoError(t, db.Model(&models.LLMChatRecord{}).Select("COALESCE(SUM(cost), 0)").
		Where("app_id = ? AND time_stamp >= ? AND time_stamp <= ?", app.ID, start, end).Scan(&total).Error)
	return total
}

func addSpend(t *testing.T, db *gorm.DB, appID uint, cost float64, at time.Time) {
	t.Helper()
	require.NoError(t, db.Create(&models.LLMChatRecord{AppID: appID, LLMID: 1, Cost: cost, TimeStamp: at}).Error)
}

// The tracker's figures equal a full sum of each App's period on every cycle,
// while rows keep arriving, for calendar-month and custom-start Apps.
func TestBudgetUsageTracker_MatchesFullSum(t *testing.T) {
	db := setupBudgetSyncTestDB(t)
	now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	custom := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC) // periods start on the 10th
	apps := []models.App{{Name: "calendar"}, {Name: "custom", BudgetStartDate: &custom}}
	for i := range apps {
		require.NoError(t, db.Create(&apps[i]).Error)
	}

	// History: inside and outside each period.
	for _, a := range apps {
		addSpend(t, db, a.ID, 100, time.Date(2026, 9, 1, 1, 0, 0, 0, time.UTC))  // in calendar period, before the custom one
		addSpend(t, db, a.ID, 200, time.Date(2026, 9, 15, 1, 0, 0, 0, time.UTC)) // in both
		addSpend(t, db, a.ID, 400, time.Date(2026, 8, 31, 1, 0, 0, 0, time.UTC)) // in neither
	}

	tr := newBudgetUsageTracker()
	for cycle := 0; cycle < 8; cycle++ {
		for _, a := range apps {
			addSpend(t, db, a.ID, float64(10*(cycle+1)), now.Add(-time.Duration(cycle)*time.Minute))
		}
		got, err := tr.usage(db, apps, now)
		require.NoError(t, err)
		for _, a := range apps {
			assert.InDelta(t, naiveSpend(t, db, a, now), got[a.ID].cost, 1e-9, "cycle %d app %s", cycle, a.Name)
		}
	}
}

// After the first cycle the full period is not summed again: later cycles only
// read rows by id. That is the point: a full sum per App per cycle kept a busy
// hub's database scanning continuously.
func TestBudgetUsageTracker_ReadsOnlyNewRowsAfterStart(t *testing.T) {
	db := setupBudgetSyncTestDB(t)
	now := time.Now()
	app := models.App{Name: "busy"}
	require.NoError(t, db.Create(&app).Error)
	for i := 0; i < 20; i++ {
		addSpend(t, db, app.ID, 1, now.Add(-time.Hour))
	}

	var unboundedSums atomic.Int32
	count := func(tx *gorm.DB) {
		sql := tx.Statement.SQL.String()
		if strings.Contains(sql, "llm_chat_records") && strings.Contains(sql, "SUM(cost)") && !strings.Contains(sql, "id >") {
			unboundedSums.Add(1)
		}
	}
	// Scan runs through the Row callbacks; Find/First through Query.
	require.NoError(t, db.Callback().Row().After("gorm:row").Register("test:count_unbounded_row", count))
	require.NoError(t, db.Callback().Query().After("gorm:query").Register("test:count_unbounded_query", count))

	tr := newBudgetUsageTracker()
	for cycle := 0; cycle < 6; cycle++ {
		addSpend(t, db, app.ID, 1, now)
		got, err := tr.usage(db, []models.App{app}, now)
		require.NoError(t, err)
		assert.InDelta(t, float64(21+cycle), got[app.ID].cost, 1e-9)
	}
	assert.EqualValues(t, 1, unboundedSums.Load(), "the period is summed in full once, at the first cycle")
}

// A row committed after rows with higher ids (another hub replica's slower
// transaction) is still counted if it arrives within the settle window.
func TestBudgetUsageTracker_LateCommitWithinWindowCounted(t *testing.T) {
	db := setupBudgetSyncTestDB(t)
	now := time.Now()
	app := models.App{Name: "replicas"}
	require.NoError(t, db.Create(&app).Error)

	tr := newBudgetUsageTracker()
	_, err := tr.usage(db, []models.App{app}, now) // start: nothing yet
	require.NoError(t, err)

	// Replica B commits ids 10..12 first; replica A's id 5 commits a cycle later.
	for id := uint(10); id <= 12; id++ {
		require.NoError(t, db.Create(&models.LLMChatRecord{ID: id, AppID: app.ID, LLMID: 1, Cost: 1, TimeStamp: now}).Error)
	}
	got, err := tr.usage(db, []models.App{app}, now)
	require.NoError(t, err)
	assert.InDelta(t, 3.0, got[app.ID].cost, 1e-9)

	require.NoError(t, db.Create(&models.LLMChatRecord{ID: 5, AppID: app.ID, LLMID: 1, Cost: 100, TimeStamp: now}).Error)
	for cycle := 0; cycle < budgetSettleCycles+2; cycle++ {
		got, err = tr.usage(db, []models.App{app}, now)
		require.NoError(t, err)
	}
	assert.InDelta(t, 103.0, got[app.ID].cost, 1e-9, "the late row is counted and stays counted once settled")
}

// A budget reset (new budget_start_date) or a new month starts a new period,
// which is summed afresh; a deleted App is forgotten.
func TestBudgetUsageTracker_PeriodChangesAndDeletedApps(t *testing.T) {
	db := setupBudgetSyncTestDB(t)
	now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	app := models.App{Name: "resettable"}
	other := models.App{Name: "deleted-later"}
	require.NoError(t, db.Create(&app).Error)
	require.NoError(t, db.Create(&other).Error)
	addSpend(t, db, app.ID, 500, time.Date(2026, 9, 5, 0, 0, 0, 0, time.UTC))
	addSpend(t, db, app.ID, 50, time.Date(2026, 9, 19, 0, 0, 0, 0, time.UTC))
	addSpend(t, db, other.ID, 7, now)

	tr := newBudgetUsageTracker()
	got, err := tr.usage(db, []models.App{app, other}, now)
	require.NoError(t, err)
	assert.InDelta(t, 550.0, got[app.ID].cost, 1e-9)

	// Budget reset on the 18th: only spend since then counts.
	reset := time.Date(2026, 9, 18, 0, 0, 0, 0, time.UTC)
	app.BudgetStartDate = &reset
	got, err = tr.usage(db, []models.App{app}, now)
	require.NoError(t, err)
	assert.InDelta(t, 50.0, got[app.ID].cost, 1e-9)
	assert.NotContains(t, tr.totals, other.ID, "a deleted App is dropped")

	// Next month.
	app.BudgetStartDate = nil
	next := time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)
	addSpend(t, db, app.ID, 9, time.Date(2026, 10, 1, 1, 0, 0, 0, time.UTC))
	got, err = tr.usage(db, []models.App{app}, next)
	require.NoError(t, err)
	assert.InDelta(t, 9.0, got[app.ID].cost, 1e-9)
}
