package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCredential_NewCredential(t *testing.T) {
	credential, err := NewCredential()
	assert.NoError(t, err)
	assert.NotNil(t, credential)
	assert.NotEmpty(t, credential.KeyID)
	assert.NotEmpty(t, credential.Secret)
	assert.False(t, credential.Active)
}

func TestCredential_CRUD(t *testing.T) {
	db := setupTestDB(t)

	// Create
	credential, err := NewCredential()
	assert.NoError(t, err)
	err = credential.Create(db)
	assert.NoError(t, err)
	assert.NotZero(t, credential.ID)

	// Get
	fetchedCredential := &Credential{}
	err = fetchedCredential.Get(db, credential.ID)
	assert.NoError(t, err)
	assert.Equal(t, credential.KeyID, fetchedCredential.KeyID)
	assert.Equal(t, credential.Secret, fetchedCredential.Secret)
	assert.Equal(t, credential.Active, fetchedCredential.Active)

	// GetByKeyID
	fetchedByKeyID := &Credential{}
	err = fetchedByKeyID.GetByKeyID(db, credential.KeyID)
	assert.NoError(t, err)
	assert.Equal(t, credential.ID, fetchedByKeyID.ID)

	// Update
	credential.Active = true
	err = credential.Update(db)
	assert.NoError(t, err)
	err = fetchedCredential.Get(db, credential.ID)
	assert.NoError(t, err)
	assert.True(t, fetchedCredential.Active)

	// Delete
	err = credential.Delete(db)
	assert.NoError(t, err)
	err = fetchedCredential.Get(db, credential.ID)
	assert.Error(t, err) // Should return an error as the credential is deleted
}

func TestCredential_Activate(t *testing.T) {
	db := setupTestDB(t)

	credential, _ := NewCredential()
	err := credential.Create(db)
	assert.NoError(t, err)

	err = credential.Activate(db)
	assert.NoError(t, err)

	fetchedCredential := &Credential{}
	err = fetchedCredential.Get(db, credential.ID)
	assert.NoError(t, err)
	assert.True(t, fetchedCredential.Active)
}

func TestCredential_Deactivate(t *testing.T) {
	db := setupTestDB(t)

	credential, _ := NewCredential()
	credential.Active = true
	err := credential.Create(db)
	assert.NoError(t, err)

	err = credential.Deactivate(db)
	assert.NoError(t, err)

	fetchedCredential := &Credential{}
	err = fetchedCredential.Get(db, credential.ID)
	assert.NoError(t, err)
	assert.False(t, fetchedCredential.Active)
}

func TestCredentials_GetAll(t *testing.T) {
	db := setupTestDB(t)

	// Create some test credentials
	for i := 0; i < 3; i++ {
		credential, _ := NewCredential()
		err := credential.Create(db)
		assert.NoError(t, err)
	}

	var credentials Credentials
	_, _, err := credentials.GetAll(db, 10, 1, true)
	assert.NoError(t, err)
	assert.Len(t, credentials, 3)
}

func TestCredentials_GetActive(t *testing.T) {
	db := setupTestDB(t)

	// Create some test credentials
	for i := 0; i < 5; i++ {
		credential, _ := NewCredential()
		credential.Active = i%2 == 0 // Make every other credential active
		err := credential.Create(db)
		assert.NoError(t, err)
	}

	var activeCredentials Credentials
	err := activeCredentials.GetActive(db)
	assert.NoError(t, err)
	assert.Len(t, activeCredentials, 3)
	for _, cred := range activeCredentials {
		assert.True(t, cred.Active)
	}
}
