package services

import (
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupOAuthMethodsTest(t *testing.T) *Service {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	err = models.InitModels(db)
	assert.NoError(t, err)

	return NewService(db)
}

func TestGetValidAccessTokenByToken(t *testing.T) {
	service := setupOAuthMethodsTest(t)

	// Create user
	user := &models.User{
		Email:    "oauth@test.com",
		Name:     "OAuth User",
		Password: "password123",
	}
	err := service.DB.Create(user).Error
	assert.NoError(t, err)

	// Use AccessTokenService to create a valid token
	accessTokenService := NewAccessTokenService(service.DB)
	_, validTokenValue, err := accessTokenService.CreateAccessToken(CreateAccessTokenArgs{
		ClientID:  "test-client-id",
		UserID:    user.ID,
		Scope:     "read write",
		ExpiresIn: 1 * time.Hour,
	})
	assert.NoError(t, err)

	// Create expired token
	_, expiredTokenValue, err := accessTokenService.CreateAccessToken(CreateAccessTokenArgs{
		ClientID:  "test-client-id",
		UserID:    user.ID,
		Scope:     "read",
		ExpiresIn: -1 * time.Hour, // Already expired
	})
	assert.NoError(t, err)

	t.Run("Get valid access token", func(t *testing.T) {
		token, err := service.GetValidAccessTokenByToken(validTokenValue)
		assert.NoError(t, err)
		assert.NotNil(t, token)
		assert.Equal(t, validTokenValue, token.Token)
		assert.Equal(t, user.ID, token.UserID)
	})

	t.Run("Get expired access token returns error", func(t *testing.T) {
		token, err := service.GetValidAccessTokenByToken(expiredTokenValue)
		assert.Error(t, err)
		assert.Nil(t, token)
	})

	t.Run("Get non-existent access token", func(t *testing.T) {
		token, err := service.GetValidAccessTokenByToken("non-existent-token")
		assert.Error(t, err)
		assert.Nil(t, token)
	})
}

func TestGetOAuthClient(t *testing.T) {
	service := setupOAuthMethodsTest(t)

	// Create user
	user := &models.User{
		Email:    "client-user@test.com",
		Name:     "Client User",
		Password: "password123",
	}
	err := service.DB.Create(user).Error
	assert.NoError(t, err)

	// Use OAuthClientService to create a client
	oauthClientService := NewOAuthClientService(service.DB)
	client, _, err := oauthClientService.CreateClient(
		"Test OAuth Client",
		[]string{"http://localhost:9000/callback"},
		&user.ID,
		"read write",
	)
	assert.NoError(t, err)

	t.Run("Get existing OAuth client", func(t *testing.T) {
		retrieved, err := service.GetOAuthClient(client.ClientID)
		assert.NoError(t, err)
		assert.NotNil(t, retrieved)
		assert.Equal(t, client.ClientID, retrieved.ClientID)
		assert.Equal(t, "Test OAuth Client", retrieved.ClientName)
	})

	t.Run("Get non-existent OAuth client", func(t *testing.T) {
		retrieved, err := service.GetOAuthClient("non-existent-client")
		assert.Error(t, err)
		assert.Nil(t, retrieved)
	})
}
