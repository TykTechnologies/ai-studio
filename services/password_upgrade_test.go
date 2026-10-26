package services

import (
	"crypto/fips140"
	"strings"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/pwhash"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// bcryptCorrectHorse is bcrypt("correct horse"), as standard builds store it.
const bcryptCorrectHorse = "$2a$10$Q28z2xyQnAaw3y7XgwQEF.DQtiAfhYrYUDv4tJBI8E3l.tNo6WncG"

func seedBcryptUser(t *testing.T, svc *Service) *models.User {
	t.Helper()
	user := models.NewUser()
	user.Email = "legacy@example.com"
	user.EmailVerified = true
	user.Password = bcryptCorrectHorse
	require.NoError(t, user.Create(svc.DB))
	return user
}

func storedPassword(t *testing.T, svc *Service, id uint) string {
	t.Helper()
	var u models.User
	require.NoError(t, svc.DB.First(&u, id).Error)
	return u.Password
}

// A bcrypt user logs in on a FIPS build: the stored hash becomes PBKDF2, and
// the next login verifies against it.
func TestAuthenticateUser_FIPSModeUpgradesBcryptHash(t *testing.T) {
	svc := NewService(setupUserTestDB(t))
	user := seedBcryptUser(t, svc)
	pwhash.ForceFIPSModeForTest(t)

	_, err := svc.AuthenticateUser("legacy@example.com", "wrong horse")
	assert.ErrorIs(t, err, LoginFailedError)
	assert.Equal(t, bcryptCorrectHorse, storedPassword(t, svc, user.ID), "a failed login rewrites nothing")

	got, err := svc.AuthenticateUser("legacy@example.com", "correct horse")
	require.NoError(t, err)
	assert.Equal(t, user.ID, got.ID)
	stored := storedPassword(t, svc, user.ID)
	assert.True(t, strings.HasPrefix(stored, pwhash.PBKDF2Prefix), "upgraded on login, got %q", stored)

	_, err = svc.AuthenticateUser("legacy@example.com", "correct horse")
	require.NoError(t, err)
	assert.Equal(t, stored, storedPassword(t, svc, user.ID), "a PBKDF2 hash is not rewritten")
}

// A standard build leaves a bcrypt hash alone, and reads a PBKDF2 hash a
// FIPS build wrote.
func TestAuthenticateUser_StandardModeKeepsBcryptAndReadsPBKDF2(t *testing.T) {
	if fips140.Enabled() {
		t.Skip("standard-mode behaviour; this process runs in FIPS mode")
	}
	svc := NewService(setupUserTestDB(t))
	user := seedBcryptUser(t, svc)

	_, err := svc.AuthenticateUser("legacy@example.com", "correct horse")
	require.NoError(t, err)
	assert.Equal(t, bcryptCorrectHorse, storedPassword(t, svc, user.ID))

	pbkdf2Hash, err := pwhash.HashPBKDF2("correct horse")
	require.NoError(t, err)
	require.NoError(t, svc.DB.Model(&models.User{}).Where("id = ?", user.ID).Update("password", pbkdf2Hash).Error)
	_, err = svc.AuthenticateUser("legacy@example.com", "correct horse")
	require.NoError(t, err)
	assert.Equal(t, pbkdf2Hash, storedPassword(t, svc, user.ID))
}

func TestSetPassword_FIPSModeWritesPBKDF2(t *testing.T) {
	pwhash.ForceFIPSModeForTest(t)
	user := models.NewUser()
	require.NoError(t, user.SetPassword("correct horse"))
	assert.True(t, strings.HasPrefix(user.Password, pwhash.PBKDF2Prefix))
	assert.True(t, user.DoesPasswordMatch("correct horse"))
}

// An OAuth client secret stored as bcrypt is upgraded on its first
// successful use in FIPS mode.
func TestValidateClientSecret_FIPSModeUpgradesBcrypt(t *testing.T) {
	db := setupUserTestDB(t)
	svc := NewOAuthClientService(db)
	client := &models.OAuthClient{ClientID: "legacy-client", ClientSecret: bcryptCorrectHorse, RedirectURIs: "http://localhost/cb"}
	require.NoError(t, db.Create(client).Error)
	pwhash.ForceFIPSModeForTest(t)

	ok, err := svc.ValidateClientSecret(client, "wrong horse")
	require.NoError(t, err)
	assert.False(t, ok)

	ok, err = svc.ValidateClientSecret(client, "correct horse")
	require.NoError(t, err)
	assert.True(t, ok)

	var stored models.OAuthClient
	require.NoError(t, db.First(&stored, client.ID).Error)
	assert.True(t, strings.HasPrefix(stored.ClientSecret, pwhash.PBKDF2Prefix), "got %q", stored.ClientSecret)

	ok, err = svc.ValidateClientSecret(&stored, "correct horse")
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestCreateClient_FIPSModeWritesPBKDF2(t *testing.T) {
	db := setupUserTestDB(t)
	svc := NewOAuthClientService(db)
	pwhash.ForceFIPSModeForTest(t)

	client, secret, err := svc.CreateClient("fips", []string{"http://localhost/cb"}, nil, "mcp")
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(client.ClientSecret, pwhash.PBKDF2Prefix))
	ok, err := svc.ValidateClientSecret(client, secret)
	require.NoError(t, err)
	assert.True(t, ok)
}
