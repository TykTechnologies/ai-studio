package analytics

import (
	"testing"

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
