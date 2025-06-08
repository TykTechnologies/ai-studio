package services_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	apitesting "github.com/TykTechnologies/midsommar/v2/api/testing" // For SetupTestDB
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
)

func TestAuthCodeService(t *testing.T) {
	db := apitesting.SetupTestDB(t)
	service := services.NewAuthCodeService(db)

	// Create a dummy user for association
	testUser := models.User{Email: "authcodeuser@example.com", Name: "AuthCode User"}
	require.NoError(t, db.Create(&testUser).Error)

	var createdAuthCodeModel *models.AuthCode
	var createdAuthCodeValue string

	createArgs := services.CreateAuthCodeArgs{
		ClientID:            "test-client-id",
		UserID:              testUser.ID,
		RedirectURI:         "http://localhost/callback",
		Scope:               "mcp",
		ExpiresIn:           10 * time.Minute,
		CodeChallenge:       "test_challenge",
		CodeChallengeMethod: "S256",
	}

	t.Run("TestCreateAuthCode", func(t *testing.T) {
		authCodeModel, authCodeValue, err := service.CreateAuthCode(createArgs)
		require.NoError(t, err)
		require.NotNil(t, authCodeModel)
		require.NotEmpty(t, authCodeValue)

		createdAuthCodeModel = authCodeModel
		createdAuthCodeValue = authCodeValue

		require.Equal(t, createArgs.ClientID, authCodeModel.ClientID)
		require.Equal(t, createArgs.UserID, authCodeModel.UserID)
		require.Equal(t, createArgs.RedirectURI, authCodeModel.RedirectURI)
		require.Equal(t, createArgs.Scope, authCodeModel.Scope)
		require.Equal(t, createArgs.CodeChallenge, authCodeModel.CodeChallenge)
		require.Equal(t, createArgs.CodeChallengeMethod, authCodeModel.CodeChallengeMethod)
		require.False(t, authCodeModel.Used)
		require.WithinDuration(t, time.Now().Add(createArgs.ExpiresIn), authCodeModel.ExpiresAt, time.Second)

		// Verify it's in DB
		var dbAuthCode models.AuthCode
		err = db.First(&dbAuthCode, "code = ?", authCodeValue).Error
		require.NoError(t, err)
		require.Equal(t, authCodeModel.ID, dbAuthCode.ID)
	})

	t.Run("TestGetValidAuthCodeByCode_Valid", func(t *testing.T) {
		require.NotEmpty(t, createdAuthCodeValue, "CreateAuthCode must run first")

		retrievedCode, err := service.GetValidAuthCodeByCode(createdAuthCodeValue)
		require.NoError(t, err)
		require.NotNil(t, retrievedCode)
		require.Equal(t, createdAuthCodeModel.ID, retrievedCode.ID)
		require.False(t, retrievedCode.Used)
		require.True(t, retrievedCode.ExpiresAt.After(time.Now()))
	})

	t.Run("TestGetValidAuthCodeByCode_NonExistent", func(t *testing.T) {
		_, err := service.GetValidAuthCodeByCode("nonexistentcode")
		require.Error(t, err)
		require.Equal(t, "authorization code not found", err.Error())
	})

	t.Run("TestMarkAuthCodeAsUsed", func(t *testing.T) {
		require.NotEmpty(t, createdAuthCodeValue, "CreateAuthCode must run first")

		err := service.MarkAuthCodeAsUsed(createdAuthCodeValue)
		require.NoError(t, err)

		// Verify it's marked as used
		var dbAuthCode models.AuthCode
		db.First(&dbAuthCode, "code = ?", createdAuthCodeValue)
		require.True(t, dbAuthCode.Used)

		// Try to get it again, should fail as it's used
		_, err = service.GetValidAuthCodeByCode(createdAuthCodeValue)
		require.Error(t, err)
		require.Equal(t, "authorization code has already been used", err.Error())

		// Test idempotency (calling MarkAuthCodeAsUsed on an already used code)
		// MarkAuthCodeAsUsed itself calls GetValidAuthCodeByCode, so it will return an error
		err = service.MarkAuthCodeAsUsed(createdAuthCodeValue)
		require.Error(t, err)
		require.Equal(t, "authorization code has already been used", err.Error())
	})

	t.Run("TestGetValidAuthCodeByCode_Expired", func(t *testing.T) {
		expiredArgs := services.CreateAuthCodeArgs{
			ClientID: "expired-client", UserID: testUser.ID, RedirectURI: "http://localhost/callback",
			Scope: "test", ExpiresIn: -1 * time.Minute, // Already expired
			CodeChallenge: "expired_challenge", CodeChallengeMethod: "S256",
		}
		_, expiredCodeValue, err := service.CreateAuthCode(expiredArgs)
		require.NoError(t, err)

		_, err = service.GetValidAuthCodeByCode(expiredCodeValue)
		require.Error(t, err)
		require.Equal(t, "authorization code has expired", err.Error())
	})
}
