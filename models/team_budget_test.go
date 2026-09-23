package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func teamTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, InitModels(db))
	return db
}

func TestResolveBudgetTeam(t *testing.T) {
	db := teamTestDB(t)
	def, err := GetOrCreateDefaultGroup(db)
	require.NoError(t, err)
	ops := &Group{Name: "Ops"}
	eng := &Group{Name: "Engineering"}
	require.NoError(t, db.Create(ops).Error)
	require.NoError(t, db.Create(eng).Error)

	newUser := func(email string, groups ...*Group) *User {
		u := &User{Email: email}
		require.NoError(t, db.Create(u).Error)
		for _, g := range groups {
			require.NoError(t, db.Model(g).Association("Users").Append(u))
		}
		return u
	}

	// Lowest non-Default team ID.
	u := newUser("a@x.io", def, eng, ops)
	got, err := ResolveBudgetTeam(db, u.ID)
	require.NoError(t, err)
	assert.Equal(t, ops.ID, *got)

	// The budget team wins while the user is a member.
	require.NoError(t, db.Model(u).Update("budget_team_id", eng.ID).Error)
	got, err = ResolveBudgetTeam(db, u.ID)
	require.NoError(t, err)
	assert.Equal(t, eng.ID, *got)

	// A deleted budget team is skipped.
	require.NoError(t, db.Delete(eng).Error)
	got, err = ResolveBudgetTeam(db, u.ID)
	require.NoError(t, err)
	assert.Equal(t, ops.ID, *got)

	// Only Default: Default. In no team at all: still Default.
	only := newUser("b@x.io", def)
	got, err = ResolveBudgetTeam(db, only.ID)
	require.NoError(t, err)
	assert.Equal(t, def.ID, *got)
	none := newUser("c@x.io")
	got, err = ResolveBudgetTeam(db, none.ID)
	require.NoError(t, err)
	assert.Equal(t, def.ID, *got)

	// Unknown user: no team.
	got, err = ResolveBudgetTeam(db, 999)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestBackfillTeamAttribution(t *testing.T) {
	db := teamTestDB(t)
	eng := &Group{Name: "Engineering"}
	require.NoError(t, db.Create(eng).Error)
	u := &User{Email: "a@x.io"}
	require.NoError(t, db.Create(u).Error)
	require.NoError(t, db.Model(eng).Association("Users").Append(u))

	// InitModels already ran the (empty) backfill; simulate an upgrade.
	require.NoError(t, db.Model(&TeamBudgetSettings{}).Where("id = 1").Update("attribution_backfilled", false).Error)
	budget := 5.0
	app := &App{Name: "legacy", UserID: u.ID, MonthlyBudget: &budget}
	require.NoError(t, db.Create(app).Error)
	deleted := &App{Name: "gone", UserID: u.ID}
	require.NoError(t, db.Create(deleted).Error)
	require.NoError(t, db.Delete(deleted).Error)
	require.NoError(t, db.Create(&LLMChatRecord{AppID: app.ID, Cost: 1}).Error)
	require.NoError(t, db.Create(&LLMChatRecord{AppID: deleted.ID, Cost: 1}).Error)
	require.NoError(t, db.Create(&LLMChatRecord{UserID: u.ID, Cost: 1}).Error)

	require.NoError(t, BackfillTeamAttribution(db))

	var got App
	require.NoError(t, db.First(&got, app.ID).Error)
	require.NotNil(t, got.TeamID)
	assert.Equal(t, eng.ID, *got.TeamID)
	assert.Equal(t, 5.0, *got.MonthlyBudget)

	var unstamped int64
	require.NoError(t, db.Model(&LLMChatRecord{}).Where("team_id IS NULL").Count(&unstamped).Error)
	assert.Zero(t, unstamped, "proxy, deleted-App and chat spend are all attributed")

	s, err := GetTeamBudgetSettings(db)
	require.NoError(t, err)
	assert.True(t, s.AttributionBackfilled)
}

func TestClearLegacyZeroBudgets(t *testing.T) {
	db := teamTestDB(t)
	// InitModels already ran it on the empty database; simulate an upgrade.
	require.NoError(t, db.Model(&TeamBudgetSettings{}).Where("id = 1").Update("zero_budgets_cleared", false).Error)

	zero, five := 0.0, 5.0
	legacyZero := &App{Name: "legacy-zero", MonthlyBudget: &zero}
	capped := &App{Name: "capped", MonthlyBudget: &five}
	unlimited := &App{Name: "unlimited"}
	for _, a := range []*App{legacyZero, capped, unlimited} {
		require.NoError(t, db.Create(a).Error)
	}
	llm := &LLM{Name: "llm", MonthlyBudget: &zero}
	require.NoError(t, db.Create(llm).Error)

	require.NoError(t, ClearLegacyZeroBudgets(db))

	var gotZero, gotCapped App
	require.NoError(t, db.First(&gotZero, legacyZero.ID).Error)
	assert.Nil(t, gotZero.MonthlyBudget, "0 meant no limit before the upgrade")
	require.NoError(t, db.First(&gotCapped, capped.ID).Error)
	assert.Equal(t, 5.0, *gotCapped.MonthlyBudget)
	var gotLLM LLM
	require.NoError(t, db.First(&gotLLM, llm.ID).Error)
	assert.Nil(t, gotLLM.MonthlyBudget)

	// After the upgrade a 0 is a deliberate zero budget: it must survive
	// the next boot.
	require.NoError(t, db.Model(&App{}).Where("id = ?", capped.ID).Update("monthly_budget", 0).Error)
	require.NoError(t, ClearLegacyZeroBudgets(db))
	var again App
	require.NoError(t, db.First(&again, capped.ID).Error)
	require.NotNil(t, again.MonthlyBudget)
	assert.Equal(t, 0.0, *again.MonthlyBudget)
}
