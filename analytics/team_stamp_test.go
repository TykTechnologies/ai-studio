package analytics

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestTeamStamper(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, models.InitModels(db))

	eng := &models.Group{Name: "Engineering"}
	require.NoError(t, db.Create(eng).Error)
	user := &models.User{Email: "a@x.io"}
	require.NoError(t, db.Create(user).Error)
	require.NoError(t, db.Model(eng).Association("Users").Append(user))
	app := &models.App{Name: "a", UserID: user.ID, TeamID: &eng.ID}
	require.NoError(t, db.Create(app).Error)

	st := newTeamStamper(db)

	proxyRec := &models.LLMChatRecord{AppID: app.ID, UserID: user.ID}
	st.stamp(proxyRec)
	require.NotNil(t, proxyRec.TeamID, "proxy spend follows the App")
	assert.Equal(t, eng.ID, *proxyRec.TeamID)

	chatRec := &models.LLMChatRecord{UserID: user.ID}
	st.stamp(chatRec)
	require.NotNil(t, chatRec.TeamID, "chat spend follows the user")
	assert.Equal(t, eng.ID, *chatRec.TeamID)

	other := uint(99)
	preset := &models.LLMChatRecord{AppID: app.ID, TeamID: &other}
	st.stamp(preset)
	assert.Equal(t, other, *preset.TeamID, "an explicit team is kept")

	unknown := &models.LLMChatRecord{AppID: 12345}
	st.stamp(unknown)
	assert.Nil(t, unknown.TeamID)

	// A deleted App's late records still reach its team.
	require.NoError(t, db.Delete(app).Error)
	st2 := newTeamStamper(db)
	late := &models.LLMChatRecord{AppID: app.ID}
	st2.stamp(late)
	require.NotNil(t, late.TeamID)
	assert.Equal(t, eng.ID, *late.TeamID)
}

// countQueries counts the SELECTs run on db from now on.
func countQueries(t *testing.T, db *gorm.DB) *int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Callback().Query().After("gorm:query").Register("test:count_"+t.Name(), func(*gorm.DB) {
		atomic.AddInt64(&n, 1)
	}))
	require.NoError(t, db.Callback().Row().After("gorm:row").Register("test:count_row_"+t.Name(), func(*gorm.DB) {
		atomic.AddInt64(&n, 1)
	}))
	return &n
}

// A batch from an edge pulse can carry many Apps and users; resolving them
// one record at a time was an N+1 on a cold cache.
func TestTeamStamper_BatchIsNotNPlusOne(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, models.InitModels(db))

	def, err := models.GetOrCreateDefaultGroup(db)
	require.NoError(t, err)
	eng := &models.Group{Name: "Engineering"}
	ops := &models.Group{Name: "Ops"}
	require.NoError(t, db.Create(eng).Error)
	require.NoError(t, db.Create(ops).Error)

	var records []*models.LLMChatRecord
	want := map[*models.LLMChatRecord]*uint{}
	for i := 0; i < 20; i++ {
		u := &models.User{Email: fmt.Sprintf("u%d@x.io", i)}
		require.NoError(t, db.Create(u).Error)
		var team *models.Group
		switch i % 4 {
		case 0: // Default only
			require.NoError(t, db.Model(def).Association("Users").Append(u))
			team = def
		case 1: // first non-Default team
			require.NoError(t, db.Model(def).Association("Users").Append(u))
			require.NoError(t, db.Model(ops).Association("Users").Append(u))
			require.NoError(t, db.Model(eng).Association("Users").Append(u))
			team = eng
		case 2: // budget team wins
			require.NoError(t, db.Model(eng).Association("Users").Append(u))
			require.NoError(t, db.Model(ops).Association("Users").Append(u))
			require.NoError(t, db.Model(u).Update("budget_team_id", ops.ID).Error)
			team = ops
		case 3: // stale budget team is skipped
			require.NoError(t, db.Model(eng).Association("Users").Append(u))
			require.NoError(t, db.Model(u).Update("budget_team_id", ops.ID).Error)
			team = eng
		}
		app := &models.App{Name: fmt.Sprintf("a%d", i), UserID: u.ID, TeamID: &team.ID}
		require.NoError(t, db.Create(app).Error)

		proxyRec := &models.LLMChatRecord{AppID: app.ID, UserID: u.ID}
		chatRec := &models.LLMChatRecord{UserID: u.ID}
		records = append(records, proxyRec, chatRec)
		want[proxyRec] = &team.ID
		want[chatRec] = &team.ID
	}
	unknown := &models.LLMChatRecord{AppID: 99999}
	records = append(records, unknown)

	st := newTeamStamper(db)
	queries := countQueries(t, db)
	st.stampBatch(records)

	assert.Greater(t, atomic.LoadInt64(queries), int64(1), "the counter sees the lookups")
	assert.LessOrEqual(t, atomic.LoadInt64(queries), int64(4), "one query for Apps, a fixed few for users, whatever the batch size")
	for rec, teamID := range want {
		require.NotNil(t, rec.TeamID)
		assert.Equal(t, *teamID, *rec.TeamID)
	}
	assert.Nil(t, unknown.TeamID)

	// Warm cache: nothing more to look up, the unknown App included.
	before := atomic.LoadInt64(queries)
	again := []*models.LLMChatRecord{{AppID: records[0].AppID}, {UserID: records[1].UserID}, {AppID: 99999}}
	st.stampBatch(again)
	assert.Equal(t, before, atomic.LoadInt64(queries))
	assert.NotNil(t, again[0].TeamID)
}

// The caches keep only recent IDs: expired entries are swept.
func TestTeamStamper_SweepsExpiredEntries(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, models.InitModels(db))

	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	st := newTeamStamper(db)
	st.now = func() time.Time { return now }

	st.stampBatch([]*models.LLMChatRecord{{AppID: 1}, {AppID: 2}, {UserID: 3}})
	assert.Len(t, st.apps, 2)
	assert.Len(t, st.users, 1)

	now = now.Add(2 * teamStampTTL)
	st.stampBatch([]*models.LLMChatRecord{{AppID: 4}})
	assert.Len(t, st.apps, 1, "only the App looked up since")
	assert.Empty(t, st.users)
}
