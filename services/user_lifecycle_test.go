package services

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Provenance stamping, the disabled switch and API key revocation.

func TestCreateUser_StampsAdminOrigin(t *testing.T) {
	db := setupUserTestDB(t)
	service := NewService(db)

	user, err := service.CreateUser(UserDTO{
		Email: "made@example.com", Name: "Made", Password: "password123", EmailVerified: true,
	})
	require.NoError(t, err)
	assert.Equal(t, models.AuthSourceAdmin, user.AuthSource)
	assert.Empty(t, user.APIKey, "admin-created users are issued no key")
	assert.False(t, user.Disabled)
}

func TestAuthenticateUser_Disabled(t *testing.T) {
	db := setupUserTestDB(t)
	service := NewService(db)

	user := models.NewUser()
	user.Email = "off@example.com"
	user.EmailVerified = true
	user.Disabled = true
	require.NoError(t, user.SetPassword("Password1!"))
	require.NoError(t, user.Create(db))

	_, err := service.AuthenticateUser("off@example.com", "Password1!")
	assert.ErrorIs(t, err, UserDisabledError)
}

func TestGetUserByAPIKey_EmptyIsNotFound(t *testing.T) {
	db := setupUserTestDB(t)
	service := NewService(db)
	require.NoError(t, db.Create(&models.User{Email: "keyless@example.com"}).Error)

	_, err := service.GetUserByAPIKey("")
	assert.Error(t, err)
}

func TestRevokeAPIKeyForUser(t *testing.T) {
	db := setupUserTestDB(t)
	service := NewService(db)

	user := models.NewUser()
	user.Email = "keyed@example.com"
	require.NoError(t, user.Create(db))
	require.NotEmpty(t, user.APIKey)

	require.NoError(t, service.RevokeAPIKeyForUser(user.ID))
	stored, err := service.GetUserByID(user.ID)
	require.NoError(t, err)
	assert.Empty(t, stored.APIKey)

	assert.Error(t, service.RevokeAPIKeyForUser(9999), "unknown user")
}

func TestGenerateAPIKeyForUser_DoesNotClobberStamps(t *testing.T) {
	db := setupUserTestDB(t)
	service := NewService(db)

	user := &models.User{Email: "stamped@example.com", Disabled: true}
	require.NoError(t, db.Create(user).Error)

	key, err := service.GenerateAPIKeyForUser(user.ID)
	require.NoError(t, err)
	stored, err := service.GetUserByID(user.ID)
	require.NoError(t, err)
	assert.NotEmpty(t, stored.APIKey)
	assert.Equal(t, key, stored.APIKey, "the returned key is the stored one")
	assert.True(t, stored.Disabled, "a column update must not touch other fields")
}

func TestSetUserDisabled(t *testing.T) {
	db := setupUserTestDB(t)
	service := NewService(db)

	// ID 1 is the super admin.
	superAdmin := &models.User{Email: "root@example.com", IsAdmin: true}
	require.NoError(t, db.Create(superAdmin).Error)
	require.Equal(t, models.SuperAdminID, superAdmin.ID)

	actor := &models.User{Email: "actor@example.com", IsAdmin: true}
	require.NoError(t, db.Create(actor).Error)

	target := &models.User{Email: "target@example.com", SessionToken: "live", ResetToken: "pending"}
	require.NoError(t, db.Create(target).Error)

	cred, err := models.NewCredential()
	require.NoError(t, err)
	cred.Active = true
	require.NoError(t, cred.Create(db))
	app := &models.App{Name: "target app", UserID: target.ID, CredentialID: cred.ID}
	require.NoError(t, db.Create(app).Error)

	t.Run("refuses self and the super admin", func(t *testing.T) {
		_, err := service.SetUserDisabled(actor.ID, actor.ID, true)
		assert.Error(t, err)
		_, err = service.SetUserDisabled(actor.ID, superAdmin.ID, true)
		assert.Error(t, err)
	})

	t.Run("disable drops the session, the reset token and the app credentials", func(t *testing.T) {
		updated, err := service.SetUserDisabled(actor.ID, target.ID, true)
		require.NoError(t, err)
		assert.True(t, updated.Disabled)
		assert.NotNil(t, updated.DisabledAt)
		assert.Empty(t, updated.SessionToken)
		assert.Empty(t, updated.ResetToken)

		var storedCred models.Credential
		require.NoError(t, db.First(&storedCred, cred.ID).Error)
		assert.False(t, storedCred.Active, "owned app credentials are deactivated")
	})

	t.Run("enable restores the account but not the app credentials", func(t *testing.T) {
		updated, err := service.SetUserDisabled(actor.ID, target.ID, false)
		require.NoError(t, err)
		assert.False(t, updated.Disabled)
		assert.Nil(t, updated.DisabledAt)

		var storedCred models.Credential
		require.NoError(t, db.First(&storedCred, cred.ID).Error)
		assert.False(t, storedCred.Active, "re-activation is an explicit per-app action")
	})

	t.Run("unknown user", func(t *testing.T) {
		_, err := service.SetUserDisabled(actor.ID, 9999, true)
		assert.Error(t, err)
	})
}

func TestListUsers_LifecycleFilters(t *testing.T) {
	db := setupUserTestDB(t)
	service := NewService(db)

	seed := []models.User{
		{Email: "sso@example.com", AuthSource: models.AuthSourceSSO},
		{Email: "local@example.com", AuthSource: models.AuthSourceLocal, APIKey: "k"},
		{Email: "off@example.com", AuthSource: models.AuthSourceAdmin, Disabled: true},
	}
	for i := range seed {
		require.NoError(t, db.Create(&seed[i]).Error)
	}
	yes := true

	users, total, _, err := service.ListUsers(ListUsersParams{AuthSource: models.AuthSourceSSO, PageSize: 10, PageNumber: 1})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Equal(t, "sso@example.com", users[0].Email)

	users, total, _, err = service.ListUsers(ListUsersParams{HasAPIKey: &yes, PageSize: 10, PageNumber: 1})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Equal(t, "local@example.com", users[0].Email)

	users, total, _, err = service.ListUsers(ListUsersParams{Disabled: &yes, PageSize: 10, PageNumber: 1})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Equal(t, "off@example.com", users[0].Email)
}
