package services

import (
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/helpers"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// SSO provisioning stamps provenance, never issues a key, refreshes the
// login stamp on every login and refuses a disabled account untouched.

func TestCreateUserWithTx_Provenance(t *testing.T) {
	db := setupSSOTestDB(t)
	ssoService := &SSOService{db: db}

	t.Run("without a profile", func(t *testing.T) {
		user, err := ssoService.createUserWithTx(db, "sso-noprofile@example.com", "No Profile", nil)
		require.NoError(t, err)
		assert.Equal(t, models.AuthSourceSSO, user.AuthSource)
		assert.Empty(t, user.SSOProfileID)
		assert.Empty(t, user.APIKey, "SSO-provisioned users are issued no key")
		assert.Empty(t, user.Password)
		require.NotNil(t, user.LastLoginAt)
		assert.Equal(t, models.LoginMethodSSO, user.LastLoginMethod)
	})

	t.Run("with a profile", func(t *testing.T) {
		profile := models.NewProfile()
		profile.ProfileID = "onelogin"
		user, err := ssoService.createUserWithTx(db, "sso-profile@example.com", "With Profile", profile)
		require.NoError(t, err)
		assert.Equal(t, "onelogin", user.SSOProfileID)

		var stored models.User
		require.NoError(t, db.First(&stored, user.ID).Error)
		assert.Equal(t, models.AuthSourceSSO, stored.AuthSource)
		assert.Equal(t, "onelogin", stored.SSOProfileID)
	})
}

func TestHandleSSO_Lifecycle(t *testing.T) {
	db := setupSSOTestDB(t)
	ssoService := &SSOService{db: db}

	t.Run("existing user gets the login stamp and the profile once", func(t *testing.T) {
		old := time.Now().Add(-72 * time.Hour)
		existing := &models.User{Email: "returning@example.com", Name: "Returning", AuthSource: models.AuthSourceLocal, APIKey: "keep-me", LastLoginAt: &old, LastLoginMethod: models.LoginMethodPassword}
		require.NoError(t, db.Create(existing).Error)

		user, err := ssoService.HandleSSO(&NonceTokenRequest{EmailAddress: "returning@example.com", DisplayName: "Returning", GroupID: "1", ProfileID: "onelogin"})
		require.NoError(t, err)

		var stored models.User
		require.NoError(t, db.First(&stored, user.ID).Error)
		assert.Equal(t, models.AuthSourceLocal, stored.AuthSource, "origin never changes")
		assert.Equal(t, "onelogin", stored.SSOProfileID)
		assert.Equal(t, models.LoginMethodSSO, stored.LastLoginMethod)
		assert.True(t, stored.LastLoginAt.After(old))
		assert.Equal(t, "keep-me", stored.APIKey, "an existing key is left alone")

		// A later login through another profile does not overwrite the first.
		_, err = ssoService.HandleSSO(&NonceTokenRequest{EmailAddress: "returning@example.com", DisplayName: "Returning", GroupID: "1", ProfileID: "okta"})
		require.NoError(t, err)
		require.NoError(t, db.First(&stored, user.ID).Error)
		assert.Equal(t, "onelogin", stored.SSOProfileID)
	})

	t.Run("disabled user is refused before any mutation", func(t *testing.T) {
		disabled := &models.User{Email: "disabled-sso@example.com", Name: "Old Name", EmailVerified: false, Disabled: true}
		require.NoError(t, db.Create(disabled).Error)

		_, err := ssoService.HandleSSO(&NonceTokenRequest{EmailAddress: "disabled-sso@example.com", DisplayName: "New Name", GroupID: "1"})
		require.Error(t, err)
		var apiErr helpers.ErrorResponse
		require.ErrorAs(t, err, &apiErr)
		assert.Equal(t, 403, apiErr.StatusCode)

		var stored models.User
		require.NoError(t, db.First(&stored, disabled.ID).Error)
		assert.Equal(t, "Old Name", stored.Name)
		assert.False(t, stored.EmailVerified, "the login must not re-verify a disabled account")
		assert.Nil(t, stored.LastLoginAt)
		var groups int64
		require.NoError(t, db.Table("user_groups").Where("user_id = ?", disabled.ID).Count(&groups).Error)
		assert.Zero(t, groups, "no team memberships are written")
	})
}
