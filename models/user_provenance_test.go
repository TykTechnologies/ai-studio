package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestUser_GetByAPIKey_EmptyNeverMatches(t *testing.T) {
	db := setupTestDB(t)
	require.NoError(t, db.Create(&User{Email: "keyless@example.com", APIKey: ""}).Error)

	var u User
	assert.ErrorIs(t, u.GetByAPIKey(db, ""), gorm.ErrRecordNotFound)
}

func TestNewUser_StampsLocalOrigin(t *testing.T) {
	u := NewUser()
	assert.Equal(t, AuthSourceLocal, u.AuthSource)
	assert.NotEmpty(t, u.APIKey)
}

func TestBackfillAuthSource(t *testing.T) {
	db := setupTestDB(t)

	seed := []User{
		{Email: "sso@example.com", Password: "", APIKey: ""},
		{Email: "admin@example.com", Password: "hash", APIKey: ""},
		{Email: "local@example.com", Password: "hash", APIKey: "key"},
		{Email: "already@example.com", Password: "", APIKey: "", AuthSource: AuthSourceLocal},
	}
	for i := range seed {
		require.NoError(t, db.Create(&seed[i]).Error)
	}
	// Simulate rows that predate the column.
	require.NoError(t, db.Model(&User{}).Where("email <> ?", "already@example.com").Update("auth_source", "").Error)

	require.NoError(t, BackfillAuthSource(db))

	want := map[string]string{
		"sso@example.com":     AuthSourceSSO,
		"admin@example.com":   AuthSourceAdmin,
		"local@example.com":   AuthSourceLocal,
		"already@example.com": AuthSourceLocal, // untouched
	}
	for email, source := range want {
		var u User
		require.NoError(t, db.Where("email = ?", email).First(&u).Error)
		assert.Equal(t, source, u.AuthSource, email)
	}

	// Idempotent: a second run changes nothing, including a row an
	// administrator has since corrected.
	require.NoError(t, db.Model(&User{}).Where("email = ?", "local@example.com").Update("auth_source", AuthSourceAdmin).Error)
	require.NoError(t, BackfillAuthSource(db))
	var u User
	require.NoError(t, db.Where("email = ?", "local@example.com").First(&u).Error)
	assert.Equal(t, AuthSourceAdmin, u.AuthSource)
}

func TestSetDisabled_ClearsSessionAndResetToken(t *testing.T) {
	db := setupTestDB(t)
	u := &User{Email: "d@example.com", SessionToken: "sess", ResetToken: "reset"}
	require.NoError(t, db.Create(u).Error)

	require.NoError(t, SetDisabled(db, u.ID, true))
	var stored User
	require.NoError(t, db.First(&stored, u.ID).Error)
	assert.True(t, stored.Disabled)
	assert.NotNil(t, stored.DisabledAt)
	assert.Empty(t, stored.SessionToken)
	assert.Empty(t, stored.ResetToken)

	require.NoError(t, SetDisabled(db, u.ID, false))
	var enabled User // fresh struct: a NULL column leaves a reused pointer field untouched
	require.NoError(t, db.First(&enabled, u.ID).Error)
	assert.False(t, enabled.Disabled)
	assert.Nil(t, enabled.DisabledAt)
}

func TestUsers_QueryUsers_LifecycleFilters(t *testing.T) {
	db := setupTestDB(t)
	seed := []User{
		{Email: "a@example.com", AuthSource: AuthSourceSSO, APIKey: ""},
		{Email: "b@example.com", AuthSource: AuthSourceLocal, APIKey: "k1"},
		{Email: "c@example.com", AuthSource: AuthSourceAdmin, APIKey: "k2", Disabled: true},
	}
	for i := range seed {
		require.NoError(t, db.Create(&seed[i]).Error)
	}
	emails := func(p UserQueryParams) []string {
		var users Users
		p.PageSize, p.PageNumber = 10, 1
		_, _, err := users.QueryUsers(db, p)
		require.NoError(t, err)
		out := make([]string, 0, len(users))
		for _, u := range users {
			out = append(out, u.Email)
		}
		return out
	}
	yes, no := true, false

	assert.ElementsMatch(t, []string{"a@example.com"}, emails(UserQueryParams{AuthSource: AuthSourceSSO}))
	assert.ElementsMatch(t, []string{"b@example.com", "c@example.com"}, emails(UserQueryParams{HasAPIKey: &yes}))
	assert.ElementsMatch(t, []string{"a@example.com"}, emails(UserQueryParams{HasAPIKey: &no}))
	assert.ElementsMatch(t, []string{"c@example.com"}, emails(UserQueryParams{Disabled: &yes}))
	assert.ElementsMatch(t, []string{"a@example.com", "b@example.com"}, emails(UserQueryParams{Disabled: &no}))
	assert.ElementsMatch(t, []string{"c@example.com"}, emails(UserQueryParams{AuthSource: AuthSourceAdmin, HasAPIKey: &yes, Disabled: &yes}))
}
