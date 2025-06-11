package services_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	apitesting "github.com/TykTechnologies/midsommar/v2/api/testing" // For SetupTestDB
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
)

func TestOAuthClientService(t *testing.T) {
	db := apitesting.SetupTestDB(t)
	service := services.NewOAuthClientService(db)

	// Create a dummy user for association
	testUser := models.User{Email: "testuser@example.com", Name: "Test User"}
	require.NoError(t, db.Create(&testUser).Error)

	var createdClient *models.OAuthClient
	var plainTextSecret string

	t.Run("TestCreateClient", func(t *testing.T) {
		clientName := "Test Client App"
		redirectURIs := []string{"http://localhost:8080/callback", "https://example.com/oauth"}
		scope := "mcp openid"

		client, secret, err := service.CreateClient(clientName, redirectURIs, &testUser.ID, scope)
		require.NoError(t, err)
		require.NotNil(t, client)
		require.NotEmpty(t, secret)

		createdClient = client
		plainTextSecret = secret

		require.NotEmpty(t, client.ClientID)
		require.NotEmpty(t, client.ClientSecret)
		require.NotEqual(t, secret, client.ClientSecret, "Stored secret should be hashed")

		err = bcrypt.CompareHashAndPassword([]byte(client.ClientSecret), []byte(secret))
		require.NoError(t, err, "Hashed secret should match plain text secret")

		require.Equal(t, clientName, client.ClientName)
		require.Equal(t, strings.Join(redirectURIs, ","), client.RedirectURIs)
		require.NotNil(t, client.UserID)
		require.Equal(t, testUser.ID, *client.UserID)
		require.Equal(t, scope, client.Scope)

		// Verify it's in DB
		var dbClient models.OAuthClient
		err = db.First(&dbClient, "client_id = ?", client.ClientID).Error
		require.NoError(t, err)
		require.Equal(t, client.ID, dbClient.ID)
	})

	t.Run("TestGetClient", func(t *testing.T) {
		require.NotNil(t, createdClient, "CreateClient must run first and succeed")

		// Test get existing client
		client, err := service.GetClient(createdClient.ClientID)
		require.NoError(t, err)
		require.NotNil(t, client)
		require.Equal(t, createdClient.ID, client.ID)
		require.Equal(t, createdClient.ClientName, client.ClientName)

		// Test get non-existent client
		_, err = service.GetClient("nonexistentclientid")
		require.Error(t, err) // Should be gorm.ErrRecordNotFound or similar
	})

	t.Run("TestValidateClientSecret", func(t *testing.T) {
		require.NotNil(t, createdClient, "CreateClient must run first and succeed")
		require.NotEmpty(t, plainTextSecret, "Plain text secret must be available")

		// Test with correct secret
		valid, err := service.ValidateClientSecret(createdClient, plainTextSecret)
		require.NoError(t, err)
		require.True(t, valid)

		// Test with incorrect secret
		invalid, err := service.ValidateClientSecret(createdClient, "wrongsecret")
		require.NoError(t, err) // bcrypt.CompareHashAndPassword returns nil for mismatch, not an error
		require.False(t, invalid)
	})

	t.Run("TestValidateRedirectURI", func(t *testing.T) {
		require.NotNil(t, createdClient, "CreateClient must run first and succeed")

		// Test with a URI present in the list
		valid, err := service.ValidateRedirectURI(createdClient, "http://localhost:8080/callback")
		require.NoError(t, err)
		require.True(t, valid)

		// Test with another URI present in the list
		valid, err = service.ValidateRedirectURI(createdClient, "https://example.com/oauth")
		require.NoError(t, err)
		require.True(t, valid)

		// Test with a URI not in the list
		valid, err = service.ValidateRedirectURI(createdClient, "http://unknown.com/callback")
		require.NoError(t, err)
		require.False(t, valid)

		// Test with a URI that is a substring of a valid one (should not match)
		valid, err = service.ValidateRedirectURI(createdClient, "http://localhost:8080")
		require.NoError(t, err)
		require.False(t, valid)

		// Test with nil client
		_, err = service.ValidateRedirectURI(nil, "http://localhost:8080/callback")
		require.Error(t, err)
		require.Equal(t, "client cannot be nil", err.Error())
	})
}
