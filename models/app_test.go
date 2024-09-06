package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestApp_NewApp(t *testing.T) {
	app := NewApp()
	assert.NotNil(t, app)
}

func TestApp_Create(t *testing.T) {
	db := setupTestDB(t)

	// Test creating an app without a credential
	app := &App{
		Name:        "Test App",
		Description: "This is a test app",
		UserID:      1,
	}
	err := app.Create(db)
	assert.NoError(t, err)
	assert.NotZero(t, app.ID)
	assert.NotZero(t, app.CredentialID)

	// Test creating an app with an existing credential
	credential, _ := NewCredential()
	err = credential.Create(db)
	assert.NoError(t, err)

	appWithCredential := &App{
		Name:         "App with Credential",
		Description:  "This app has a pre-existing credential",
		UserID:       1,
		CredentialID: credential.ID,
	}
	err = appWithCredential.Create(db)
	assert.NoError(t, err)
	assert.NotZero(t, appWithCredential.ID)
	assert.Equal(t, credential.ID, appWithCredential.CredentialID)
}

func TestApp_Get(t *testing.T) {
	db := setupTestDB(t)

	app := &App{
		Name:        "Test App",
		Description: "This is a test app",
		UserID:      1,
	}
	err := app.Create(db)
	assert.NoError(t, err)

	fetchedApp := &App{}
	err = fetchedApp.Get(db, app.ID)
	assert.NoError(t, err)
	assert.Equal(t, app.ID, fetchedApp.ID)
	assert.Equal(t, app.Name, fetchedApp.Name)
	assert.Equal(t, app.Description, fetchedApp.Description)
	assert.Equal(t, app.UserID, fetchedApp.UserID)
	assert.NotZero(t, fetchedApp.CredentialID)
	assert.NotNil(t, fetchedApp.Credential)
}

func TestApp_Update(t *testing.T) {
	db := setupTestDB(t)

	app := &App{
		Name:        "Test App",
		Description: "This is a test app",
		UserID:      1,
	}
	err := app.Create(db)
	assert.NoError(t, err)

	app.Name = "Updated App Name"
	app.Description = "Updated description"
	err = app.Update(db)
	assert.NoError(t, err)

	fetchedApp := &App{}
	err = fetchedApp.Get(db, app.ID)
	assert.NoError(t, err)
	assert.Equal(t, "Updated App Name", fetchedApp.Name)
	assert.Equal(t, "Updated description", fetchedApp.Description)
}

func TestApp_Delete(t *testing.T) {
	db := setupTestDB(t)

	app := &App{
		Name:        "Test App",
		Description: "This is a test app",
		UserID:      1,
	}
	err := app.Create(db)
	assert.NoError(t, err)

	err = app.Delete(db)
	assert.NoError(t, err)

	fetchedApp := &App{}
	err = fetchedApp.Get(db, app.ID)
	assert.Error(t, err) // Should return an error as the app is deleted
}

func TestApp_GetByUserID(t *testing.T) {
	db := setupTestDB(t)

	// Create some test apps
	apps := []App{
		{Name: "App 1", Description: "Description 1", UserID: 1},
		{Name: "App 2", Description: "Description 2", UserID: 1},
		{Name: "App 3", Description: "Description 3", UserID: 2},
	}
	for i := range apps {
		err := apps[i].Create(db)
		assert.NoError(t, err)
	}

	fetchedApps, err := (&App{}).GetByUserID(db, 1)
	assert.NoError(t, err)
	assert.Len(t, fetchedApps, 2)
	assert.Equal(t, "App 1", fetchedApps[0].Name)
	assert.Equal(t, "App 2", fetchedApps[1].Name)
}

func TestApp_GetByName(t *testing.T) {
	db := setupTestDB(t)

	app := &App{
		Name:        "Unique App Name",
		Description: "This is a unique app",
		UserID:      1,
	}
	err := app.Create(db)
	assert.NoError(t, err)

	fetchedApp := &App{}
	err = fetchedApp.GetByName(db, "Unique App Name")
	assert.NoError(t, err)
	assert.Equal(t, app.ID, fetchedApp.ID)
	assert.Equal(t, app.Name, fetchedApp.Name)
	assert.Equal(t, app.Description, fetchedApp.Description)
}

func TestApp_ActivateCredential(t *testing.T) {
	db := setupTestDB(t)

	app := &App{
		Name:        "Test App",
		Description: "This is a test app",
		UserID:      1,
	}
	err := app.Create(db)
	assert.NoError(t, err)

	err = app.ActivateCredential(db)
	assert.NoError(t, err)

	fetchedApp := &App{}
	err = fetchedApp.Get(db, app.ID)
	assert.NoError(t, err)
	assert.True(t, fetchedApp.Credential.Active)
}

func TestApp_DeactivateCredential(t *testing.T) {
	db := setupTestDB(t)

	app := &App{
		Name:        "Test App",
		Description: "This is a test app",
		UserID:      1,
	}
	err := app.Create(db)
	assert.NoError(t, err)

	err = app.ActivateCredential(db)
	assert.NoError(t, err)

	err = app.DeactivateCredential(db)
	assert.NoError(t, err)

	fetchedApp := &App{}
	err = fetchedApp.Get(db, app.ID)
	assert.NoError(t, err)
	assert.False(t, fetchedApp.Credential.Active)
}
