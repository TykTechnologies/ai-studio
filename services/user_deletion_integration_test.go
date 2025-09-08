package services

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupUserDeletionTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	err = models.InitModels(db)
	assert.NoError(t, err)

	return db
}

func TestUserDeletionCredentialDeactivation(t *testing.T) {
	// Setup
	db := setupUserDeletionTestDB(t)
	service := NewService(db)

	// Create a user
	user := &models.User{
		Email:    "test@example.com",
		Name:     "Test User",
		IsAdmin:  false,
	}
	if err := user.Create(db); err != nil {
		t.Fatal(err)
	}

	// Create an app for the user (which will create a credential)
	app := &models.App{
		Name:        "Test App",
		Description: "Test application",
		UserID:      user.ID,
	}
	if err := app.Create(db); err != nil {
		t.Fatal(err)
	}

	// Activate the credential (they are created inactive by default)
	var credential models.Credential
	if err := db.First(&credential, app.CredentialID).Error; err != nil {
		t.Fatal(err)
	}
	
	if err := credential.Activate(db); err != nil {
		t.Fatal(err)
	}
	
	// Verify the credential is now active
	if err := db.First(&credential, app.CredentialID).Error; err != nil {
		t.Fatal(err)
	}
	
	if !credential.Active {
		t.Fatal("Expected credential to be active after activation")
	}

	// Delete the user
	if err := service.DeleteUser(user); err != nil {
		t.Fatal(err)
	}

	// Verify the credential was deactivated
	if err := db.First(&credential, app.CredentialID).Error; err != nil {
		t.Fatal(err)
	}
	
	if credential.Active {
		t.Errorf("Expected credential to be deactivated after user deletion, but it's still active")
	}

	// Verify the app is marked as orphaned
	var updatedApp models.App
	if err := db.First(&updatedApp, app.ID).Error; err != nil {
		t.Fatal(err)
	}
	
	if !updatedApp.IsOrphaned {
		t.Errorf("Expected app to be marked as orphaned after user deletion")
	}

	// Verify the user is soft deleted
	var deletedUser models.User
	if err := db.First(&deletedUser, user.ID).Error; err != nil {
		if err != gorm.ErrRecordNotFound {
			t.Errorf("Expected user to be soft deleted, but got error: %v", err)
		}
	} else {
		t.Error("Expected user to be soft deleted but it's still accessible")
	}

	// Verify the app still exists (not deleted)
	if err := db.First(&updatedApp, app.ID).Error; err != nil {
		t.Errorf("Expected app to still exist after user deletion, but got error: %v", err)
	}
}

func TestUserDeletionWithMultipleApps(t *testing.T) {
	// Setup
	db := setupUserDeletionTestDB(t)
	service := NewService(db)

	// Create a user
	user := &models.User{
		Email:    "test2@example.com",
		Name:     "Test User 2",
		IsAdmin:  false,
	}
	if err := user.Create(db); err != nil {
		t.Fatal(err)
	}

	// Create multiple apps for the user
	apps := []*models.App{
		{
			Name:        "Test App 1",
			Description: "First test application",
			UserID:      user.ID,
		},
		{
			Name:        "Test App 2", 
			Description: "Second test application",
			UserID:      user.ID,
		},
	}

	credentialIDs := make([]uint, len(apps))
	for i, app := range apps {
		if err := app.Create(db); err != nil {
			t.Fatal(err)
		}
		credentialIDs[i] = app.CredentialID
	}

	// Activate all credentials (they are created inactive by default)
	for _, credID := range credentialIDs {
		var cred models.Credential
		if err := db.First(&cred, credID).Error; err != nil {
			t.Fatal(err)
		}
		if err := cred.Activate(db); err != nil {
			t.Fatal(err)
		}
	}
	
	// Verify all credentials are now active
	for _, credID := range credentialIDs {
		var cred models.Credential
		if err := db.First(&cred, credID).Error; err != nil {
			t.Fatal(err)
		}
		if !cred.Active {
			t.Fatal("Expected all credentials to be active after activation")
		}
	}

	// Delete the user
	if err := service.DeleteUser(user); err != nil {
		t.Fatal(err)
	}

	// Verify all credentials were deactivated
	for i, credID := range credentialIDs {
		var cred models.Credential
		if err := db.First(&cred, credID).Error; err != nil {
			t.Fatal(err)
		}
		if cred.Active {
			t.Errorf("Expected credential %d to be deactivated after user deletion", i+1)
		}
	}

	// Verify all apps are marked as orphaned
	for i, app := range apps {
		var updatedApp models.App
		if err := db.First(&updatedApp, app.ID).Error; err != nil {
			t.Fatal(err)
		}
		if !updatedApp.IsOrphaned {
			t.Errorf("Expected app %d to be marked as orphaned after user deletion", i+1)
		}
	}
}

func TestUserDeletionRollbackOnError(t *testing.T) {
	// Setup
	db := setupUserDeletionTestDB(t)
	service := NewService(db)

	// Create a user
	user := &models.User{
		Email:    "test3@example.com",
		Name:     "Test User 3",
		IsAdmin:  false,
	}
	if err := user.Create(db); err != nil {
		t.Fatal(err)
	}

	// Create an app
	app := &models.App{
		Name:        "Test App 3",
		Description: "Third test application",
		UserID:      user.ID,
	}
	if err := app.Create(db); err != nil {
		t.Fatal(err)
	}

	// Activate the credential and store its state
	var originalCredential models.Credential
	if err := db.First(&originalCredential, app.CredentialID).Error; err != nil {
		t.Fatal(err)
	}
	
	if err := originalCredential.Activate(db); err != nil {
		t.Fatal(err)
	}
	
	// Refresh to get updated state
	if err := db.First(&originalCredential, app.CredentialID).Error; err != nil {
		t.Fatal(err)
	}

	// Force an error by trying to delete a super admin user
	user.IsAdmin = true
	user.ID = models.SuperAdminID // Make it the super admin
	if err := user.Update(db); err != nil {
		t.Fatal(err)
	}

	// Attempt to delete the super admin user (should fail)
	err := service.DeleteUser(user)
	if err == nil {
		t.Fatal("Expected error when trying to delete super admin user")
	}

	// Verify the credential is still in its original state (not deactivated)
	var finalCredential models.Credential
	if err := db.First(&finalCredential, app.CredentialID).Error; err != nil {
		t.Fatal(err)
	}
	
	if finalCredential.Active != originalCredential.Active {
		t.Error("Expected credential state to remain unchanged after failed user deletion")
	}

	// Verify the app is not marked as orphaned
	var finalApp models.App
	if err := db.First(&finalApp, app.ID).Error; err != nil {
		t.Fatal(err)
	}
	
	if finalApp.IsOrphaned {
		t.Error("Expected app to not be marked as orphaned after failed user deletion")
	}
}