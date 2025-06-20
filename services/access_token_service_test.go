package services_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	apitesting "github.com/TykTechnologies/midsommar/v2/api/testing" // For SetupTestDB
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
)

func TestAccessTokenService(t *testing.T) {
	db := apitesting.SetupTestDB(t)
	service := services.NewAccessTokenService(db)

	// Create a dummy user for association
	testUser := models.User{Email: "accesstokenuser@example.com", Name: "AccessToken User"}
	require.NoError(t, db.Create(&testUser).Error)

	var createdTokenModel *models.AccessToken
	var createdTokenValue string

	createArgs := services.CreateAccessTokenArgs{
		ClientID:  "test-client-for-token",
		UserID:    testUser.ID,
		Scope:     "mcp read write",
		ExpiresIn: 1 * time.Hour,
	}

	t.Run("TestCreateAccessToken", func(t *testing.T) {
		tokenModel, tokenValue, err := service.CreateAccessToken(createArgs)
		require.NoError(t, err)
		require.NotNil(t, tokenModel)
		require.NotEmpty(t, tokenValue)

		createdTokenModel = tokenModel
		createdTokenValue = tokenValue

		require.Equal(t, createArgs.ClientID, tokenModel.ClientID)
		require.Equal(t, createArgs.UserID, tokenModel.UserID)
		require.Equal(t, createArgs.Scope, tokenModel.Scope)
		require.WithinDuration(t, time.Now().Add(createArgs.ExpiresIn), tokenModel.ExpiresAt, time.Second)

		// Verify it's in DB
		var dbToken models.AccessToken
		err = db.First(&dbToken, "token = ?", tokenValue).Error
		require.NoError(t, err)
		require.Equal(t, tokenModel.ID, dbToken.ID)
	})

	t.Run("TestGetValidAccessTokenByToken_Valid", func(t *testing.T) {
		require.NotEmpty(t, createdTokenValue, "CreateAccessToken must run first")

		retrievedToken, err := service.GetValidAccessTokenByToken(createdTokenValue)
		require.NoError(t, err)
		require.NotNil(t, retrievedToken)
		require.Equal(t, createdTokenModel.ID, retrievedToken.ID)
		require.True(t, retrievedToken.ExpiresAt.After(time.Now()))
	})

	t.Run("TestGetValidAccessTokenByToken_NonExistent", func(t *testing.T) {
		_, err := service.GetValidAccessTokenByToken("nonexistenttoken")
		require.Error(t, err)
		require.Equal(t, "access token not found", err.Error())
	})

	t.Run("TestGetValidAccessTokenByToken_Expired", func(t *testing.T) {
		expiredArgs := services.CreateAccessTokenArgs{
			ClientID: "expired-client-token", UserID: testUser.ID,
			Scope: "test", ExpiresIn: -1 * time.Hour, // Already expired
		}
		_, expiredTokenValue, err := service.CreateAccessToken(expiredArgs)
		require.NoError(t, err)

		_, err = service.GetValidAccessTokenByToken(expiredTokenValue)
		require.Error(t, err)
		require.Equal(t, "access token has expired", err.Error())
	})
}
