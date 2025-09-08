package services

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestUserDeletionSecurityIntegration(t *testing.T) {
	// Setup database and service
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	err = models.InitModels(db)
	assert.NoError(t, err)

	service := NewService(db)

	// Create a user
	user := &models.User{
		Email:   "security-test@example.com",
		Name:    "Security Test User",
		IsAdmin: false,
	}
	assert.NoError(t, user.Create(db))

	// Create an app for the user
	app := &models.App{
		Name:        "Security Test App",
		Description: "Test app for security validation",
		UserID:      user.ID,
	}
	assert.NoError(t, app.Create(db))

	// Activate the credential
	var credential models.Credential
	assert.NoError(t, db.First(&credential, app.CredentialID).Error)
	assert.NoError(t, credential.Activate(db))

	// Reload to get updated state
	assert.NoError(t, db.First(&credential, app.CredentialID).Error)
	assert.True(t, credential.Active, "Credential should be active before user deletion")

	// Test 1: Before user deletion, GetCredentialBySecret should work
	retrievedCred, err := service.GetCredentialBySecret(credential.Secret)
	assert.NoError(t, err, "GetCredentialBySecret should work before user deletion")
	assert.True(t, retrievedCred.Active, "Retrieved credential should be active")

	// Delete the user
	assert.NoError(t, service.DeleteUser(user))

	// Test 2: After user deletion, GetCredentialBySecret should still work but credential should be deactivated
	retrievedCred, err = service.GetCredentialBySecret(credential.Secret)
	assert.NoError(t, err, "GetCredentialBySecret should still work (it retrieves regardless of active status)")
	assert.False(t, retrievedCred.Active, "Retrieved credential should be deactivated after user deletion")

	// Verify the credential was actually deactivated
	assert.NoError(t, db.First(&credential, app.CredentialID).Error)
	assert.False(t, credential.Active, "Credential should be deactivated after user deletion")

	// Verify the app is marked as orphaned but still exists
	var updatedApp models.App
	assert.NoError(t, db.First(&updatedApp, app.ID).Error)
	assert.True(t, updatedApp.IsOrphaned, "App should be marked as orphaned")

	// Verify the user is soft-deleted (should not be found in regular queries)
	var deletedUser models.User
	err = db.First(&deletedUser, user.ID).Error
	assert.Equal(t, gorm.ErrRecordNotFound, err, "User should be soft-deleted and not found in regular queries")
}

func TestGetCredentialBySecretRetrievesCredentialRegardlessOfStatus(t *testing.T) {
	// This test verifies that GetCredentialBySecret retrieves credentials regardless of active status.
	// The proxy layer is responsible for checking the Active field for authorization.
	
	// Setup
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	err = models.InitModels(db)
	assert.NoError(t, err)

	service := NewService(db)

	// Create and save a credential
	credential, err := models.NewCredential()
	assert.NoError(t, err)
	assert.NoError(t, credential.Create(db))

	// Test 1: Inactive credential should be returned (but will be inactive)
	retrievedCred, err := service.GetCredentialBySecret(credential.Secret)
	assert.NoError(t, err, "GetCredentialBySecret should work for inactive credentials")
	assert.Equal(t, credential.ID, retrievedCred.ID)
	assert.False(t, retrievedCred.Active, "Retrieved credential should be inactive")

	// Test 2: Active credential should be returned as active
	assert.NoError(t, credential.Activate(db))
	retrievedCred, err = service.GetCredentialBySecret(credential.Secret)
	assert.NoError(t, err)
	assert.Equal(t, credential.ID, retrievedCred.ID)
	assert.True(t, retrievedCred.Active, "Retrieved credential should be active")

	// Test 3: Deactivated credential should be returned as deactivated
	assert.NoError(t, credential.Deactivate(db))
	retrievedCred, err = service.GetCredentialBySecret(credential.Secret)
	assert.NoError(t, err, "GetCredentialBySecret should work for deactivated credentials")
	assert.False(t, retrievedCred.Active, "Retrieved credential should be deactivated")
}