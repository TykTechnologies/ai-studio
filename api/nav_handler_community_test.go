//go:build !enterprise

package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apitest "github.com/TykTechnologies/midsommar/v2/api/testing"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services/group_access"
)

func TestNavManifestHandler(t *testing.T) {
	db := apitest.SetupTestDB(t)
	service := apitest.SetupTestService(db)
	service.GroupAccessService = group_access.NewService(db) // the chat menu reads entitlements
	cfg := apitest.SetupTestAuthConfig(db, service)
	authService := apitest.SetupTestAuthService(db, service)
	r := NewAPI(service, true, authService, cfg, nil, emptyFile, nil).router

	mkUser := func(email string, admin, portal, chat bool) string {
		u := models.NewUser()
		u.Email = email
		u.Name = email
		u.Password = "hash"
		u.IsAdmin = admin
		u.EmailVerified = true
		u.ShowPortal = portal
		u.ShowChat = chat
		require.NoError(t, u.Create(db))
		return u.APIKey
	}
	get := func(key string) NavManifest {
		w := apitest.PerformAuthRequest(r, "GET", "/common/nav", nil, key)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var m NavManifest
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &m))
		return m
	}

	admin := get(mkUser("admin@nav.test", true, true, true))
	assert.Equal(t, []string{"Admin", "AI Portal", "Chat"}, labels(admin.Surfaces))
	assert.Equal(t, "Overview", admin.Admin[0].Text)
	assert.Equal(t, "Plugins", admin.Admin[len(admin.Admin)-1].Text)
	assert.NotNil(t, find(admin.Admin, "llms"))
	assert.Nil(t, find(admin.Admin, "governance"), "Community Edition has no Governance")

	user := get(mkUser("dev@nav.test", false, true, false))
	assert.Equal(t, []string{"AI Portal"}, labels(user.Surfaces))
	assert.Empty(t, user.Admin, "no admin permissions, no admin menu")
	assert.Equal(t, "Overview", user.Portal[0].Text)
	assert.NotNil(t, find(user.Portal, "browse-llms"))
	assert.Empty(t, user.Chat, "chat is off for this user")

	// Chat: the user's rooms, recent conversations and the active agents
	// they may use (public, or shared with one of their teams).
	chatterKey := mkUser("chatter@nav.test", false, false, true)
	var chatter models.User
	require.NoError(t, db.Where("email = ?", "chatter@nav.test").First(&chatter).Error)
	team := models.Group{Name: "nav-team"}
	other := models.Group{Name: "nav-other"}
	require.NoError(t, db.Create(&team).Error)
	require.NoError(t, db.Create(&other).Error)
	require.NoError(t, db.Model(&chatter).Association("Groups").Append(&team))
	require.NoError(t, db.Create(&models.AgentConfig{Name: "Public", Slug: "nav-public", IsActive: true}).Error)
	require.NoError(t, db.Create(&models.AgentConfig{Name: "Team", Slug: "nav-team", IsActive: true, Groups: []models.Group{team}}).Error)
	require.NoError(t, db.Create(&models.AgentConfig{Name: "Elsewhere", Slug: "nav-elsewhere", IsActive: true, Groups: []models.Group{other}}).Error)
	off := models.AgentConfig{Name: "Off", Slug: "nav-off", IsActive: true}
	require.NoError(t, db.Create(&off).Error)
	require.NoError(t, db.Model(&off).Update("is_active", false).Error)
	// History lists conversations with more than one message.
	require.NoError(t, db.Create(&models.ChatHistoryRecord{UserID: chatter.ID, ChatID: 1, SessionID: "s1", Name: "Yesterday"}).Error)
	require.NoError(t, db.Create(&models.CMessage{Session: "s1", ChatID: 1}).Error)
	require.NoError(t, db.Create(&models.CMessage{Session: "s1", ChatID: 1}).Error)

	chat := get(chatterKey)
	assert.Equal(t, []string{"Chat"}, labels(chat.Surfaces))
	assert.Empty(t, chat.Portal, "the portal is off for this user")
	require.NotNil(t, find(chat.Chat, "agents"))
	assert.Equal(t, []string{"Public", "Team"}, labels(find(chat.Chat, "agents").Items), "inactive agents and other teams' agents are left out")
	require.NotNil(t, find(chat.Chat, "past-conversations"))
	assert.Equal(t, []string{"Yesterday", "View all conversations"}, labels(find(chat.Chat, "past-conversations").Items))

	w := apitest.PerformRequest(r, "GET", "/common/nav", nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}
