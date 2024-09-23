package services

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDBForCredentials(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	err = models.InitModels(db)
	assert.NoError(t, err)

	return db
}

func TestCredentialService(t *testing.T) {
	db := setupTestDBForCredentials(t)
	service := NewService(db)

	// Test CreateCredential
	credential, err := service.CreateCredential()
	assert.NoError(t, err)
	assert.NotNil(t, credential)
	assert.NotEmpty(t, credential.KeyID)
	assert.NotEmpty(t, credential.Secret)
	assert.False(t, credential.Active)

	// Test GetCredentialByID
	fetchedCredential, err := service.GetCredentialByID(credential.ID)
	assert.NoError(t, err)
	assert.Equal(t, credential.ID, fetchedCredential.ID)
	assert.Equal(t, credential.KeyID, fetchedCredential.KeyID)
	assert.Equal(t, credential.Secret, fetchedCredential.Secret)

	// Test GetCredentialByKeyID
	fetchedByKeyID, err := service.GetCredentialByKeyID(credential.KeyID)
	assert.NoError(t, err)
	assert.Equal(t, credential.ID, fetchedByKeyID.ID)

	// Test UpdateCredential
	credential.Active = true
	err = service.UpdateCredential(credential)
	assert.NoError(t, err)
	updatedCredential, _ := service.GetCredentialByID(credential.ID)
	assert.True(t, updatedCredential.Active)

	// Test ActivateCredential
	inactiveCredential, _ := service.CreateCredential()
	err = service.ActivateCredential(inactiveCredential.ID)
	assert.NoError(t, err)
	activatedCredential, _ := service.GetCredentialByID(inactiveCredential.ID)
	assert.True(t, activatedCredential.Active)

	// Test DeactivateCredential
	err = service.DeactivateCredential(activatedCredential.ID)
	assert.NoError(t, err)
	deactivatedCredential, _ := service.GetCredentialByID(activatedCredential.ID)
	assert.False(t, deactivatedCredential.Active)

	// Test GetAllCredentials
	allCredentials, _, _, err := service.GetAllCredentials(10, 1, true)
	assert.NoError(t, err)
	assert.Len(t, allCredentials, 2)

	// Test GetActiveCredentials
	activeCredentials, err := service.GetActiveCredentials()
	assert.NoError(t, err)
	assert.Len(t, activeCredentials, 1)
	assert.Equal(t, credential.ID, activeCredentials[0].ID)

	// Test DeleteCredential
	err = service.DeleteCredential(credential.ID)
	assert.NoError(t, err)
	_, err = service.GetCredentialByID(credential.ID)
	assert.Error(t, err)
}

func TestCredentialService_MultipleCredentials(t *testing.T) {
	db := setupTestDBForCredentials(t)
	service := NewService(db)

	// Create multiple credentials
	credential1, _ := service.CreateCredential()
	service.CreateCredential()
	credential3, _ := service.CreateCredential()

	// Activate some credentials
	service.ActivateCredential(credential1.ID)
	service.ActivateCredential(credential3.ID)

	// Test GetAllCredentials
	allCredentials, _, _, err := service.GetAllCredentials(10, 1, true)
	assert.NoError(t, err)
	assert.Len(t, allCredentials, 3)

	// Test GetActiveCredentials
	activeCredentials, err := service.GetActiveCredentials()
	assert.NoError(t, err)
	assert.Len(t, activeCredentials, 2)

	// Verify active credentials
	activeIDs := []uint{activeCredentials[0].ID, activeCredentials[1].ID}
	assert.Contains(t, activeIDs, credential1.ID)
	assert.Contains(t, activeIDs, credential3.ID)

	// Test deactivating all credentials
	for _, cred := range allCredentials {
		err := service.DeactivateCredential(cred.ID)
		assert.NoError(t, err)
	}

	// Verify no active credentials
	activeCredentials, err = service.GetActiveCredentials()
	assert.NoError(t, err)
	assert.Len(t, activeCredentials, 0)
}
