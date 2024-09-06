package services

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDBForApps(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	err = models.InitModels(db)
	assert.NoError(t, err)

	return db
}

func TestAppService(t *testing.T) {
	db := setupTestDBForApps(t)
	service := NewService(db)

	// Create a test user
	user, err := service.CreateUser("test@example.com", "Test User", "password123")
	assert.NoError(t, err)

	// Test CreateApp
	app, err := service.CreateApp("Test App", "This is a test app", user.ID)
	assert.NoError(t, err)
	assert.NotNil(t, app)
	assert.NotZero(t, app.ID)
	assert.NotZero(t, app.CredentialID)

	// Test GetAppByID
	fetchedApp, err := service.GetAppByID(app.ID)
	assert.NoError(t, err)
	assert.Equal(t, app.ID, fetchedApp.ID)
	assert.Equal(t, app.Name, fetchedApp.Name)
	assert.Equal(t, app.Description, fetchedApp.Description)
	assert.Equal(t, app.UserID, fetchedApp.UserID)

	// Test UpdateApp
	updatedApp, err := service.UpdateApp(app.ID, "Updated App", "This is an updated app")
	assert.NoError(t, err)
	assert.Equal(t, app.ID, updatedApp.ID)
	assert.Equal(t, "Updated App", updatedApp.Name)
	assert.Equal(t, "This is an updated app", updatedApp.Description)

	// Test GetAppsByUserID
	userApps, err := service.GetAppsByUserID(user.ID)
	assert.NoError(t, err)
	assert.Len(t, userApps, 1)
	assert.Equal(t, app.ID, userApps[0].ID)

	// Test GetAppByName
	namedApp, err := service.GetAppByName("Updated App")
	assert.NoError(t, err)
	assert.Equal(t, app.ID, namedApp.ID)

	// Test ActivateAppCredential
	err = service.ActivateAppCredential(app.ID)
	assert.NoError(t, err)
	activatedApp, _ := service.GetAppByID(app.ID)
	assert.True(t, activatedApp.Credential.Active)

	// Test DeactivateAppCredential
	err = service.DeactivateAppCredential(app.ID)
	assert.NoError(t, err)
	deactivatedApp, _ := service.GetAppByID(app.ID)
	assert.False(t, deactivatedApp.Credential.Active)

	// Test DeleteApp
	err = service.DeleteApp(app.ID)
	assert.NoError(t, err)
	_, err = service.GetAppByID(app.ID)
	assert.Error(t, err)
}

func TestAppService_MultipleApps(t *testing.T) {
	db := setupTestDBForApps(t)
	service := NewService(db)

	// Create test users
	user1, _ := service.CreateUser("user1@example.com", "User 1", "password123")
	user2, _ := service.CreateUser("user2@example.com", "User 2", "password456")

	// Create multiple apps
	app1, _ := service.CreateApp("App 1", "Description 1", user1.ID)
	app2, _ := service.CreateApp("App 2", "Description 2", user1.ID)
	app3, _ := service.CreateApp("App 3", "Description 3", user2.ID)

	// Test GetAppsByUserID for user1
	user1Apps, err := service.GetAppsByUserID(user1.ID)
	assert.NoError(t, err)
	assert.Len(t, user1Apps, 2)
	assert.ElementsMatch(t, []uint{app1.ID, app2.ID}, []uint{user1Apps[0].ID, user1Apps[1].ID})

	// Test GetAppsByUserID for user2
	user2Apps, err := service.GetAppsByUserID(user2.ID)
	assert.NoError(t, err)
	assert.Len(t, user2Apps, 1)
	assert.Equal(t, app3.ID, user2Apps[0].ID)

	// Test activating credentials for all apps
	for _, app := range []uint{app1.ID, app2.ID, app3.ID} {
		err := service.ActivateAppCredential(app)
		assert.NoError(t, err)
	}

	// Verify all credentials are active
	for _, app := range []uint{app1.ID, app2.ID, app3.ID} {
		fetchedApp, _ := service.GetAppByID(app)
		assert.True(t, fetchedApp.Credential.Active)
	}

	// Test deactivating credentials for user1's apps
	for _, app := range user1Apps {
		err := service.DeactivateAppCredential(app.ID)
		assert.NoError(t, err)
	}

	// Verify user1's app credentials are inactive and user2's app credential is still active
	for _, app := range user1Apps {
		fetchedApp, _ := service.GetAppByID(app.ID)
		assert.False(t, fetchedApp.Credential.Active)
	}
	fetchedApp3, _ := service.GetAppByID(app3.ID)
	assert.True(t, fetchedApp3.Credential.Active)
}
