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
	assert.Empty(t, app.Datasources)
	assert.Empty(t, app.LLMs)
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
	assert.Empty(t, app.Datasources)
	assert.Empty(t, app.LLMs)
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

	// Add a datasource and LLM to the app
	datasource := &Datasource{Name: "Test Datasource"}
	err = datasource.Create(db)
	assert.NoError(t, err)
	err = app.AddDatasource(db, datasource)
	assert.NoError(t, err)

	llm := &LLM{Name: "Test LLM"}
	err = llm.Create(db)
	assert.NoError(t, err)
	err = app.AddLLM(db, llm)
	assert.NoError(t, err)

	// Fetch the updated app
	updatedApp := &App{}
	err = updatedApp.Get(db, app.ID)
	assert.NoError(t, err)

	// Check if the datasource and LLM were added correctly
	err = updatedApp.GetDatasources(db)
	assert.NoError(t, err)
	assert.Len(t, updatedApp.Datasources, 1)
	assert.Equal(t, datasource.ID, updatedApp.Datasources[0].ID)

	_, _, _, err = updatedApp.GetLLMs(db, 10, 1, true)
	assert.NoError(t, err)
	assert.Len(t, updatedApp.LLMs, 1)
	assert.Equal(t, llm.ID, updatedApp.LLMs[0].ID)

	// Update the app again
	updatedApp.Name = "Final App Name"
	err = updatedApp.Update(db)
	assert.NoError(t, err)

	// Fetch the app one last time to confirm all changes
	finalApp := &App{}
	err = finalApp.Get(db, app.ID)
	assert.NoError(t, err)
	assert.Equal(t, "Final App Name", finalApp.Name)
	assert.Equal(t, "Updated description", finalApp.Description)

	err = finalApp.GetDatasources(db)
	assert.NoError(t, err)
	assert.Len(t, finalApp.Datasources, 1)
	assert.Equal(t, datasource.ID, finalApp.Datasources[0].ID)

	_, _, _, err = finalApp.GetLLMs(db, 10, 1, true)
	assert.NoError(t, err)
	assert.Len(t, finalApp.LLMs, 1)
	assert.Equal(t, llm.ID, finalApp.LLMs[0].ID)
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

func TestApp_DatasourceAssociation(t *testing.T) {
	db := setupTestDB(t)

	app := &App{
		Name:        "Test App",
		Description: "This is a test app",
		UserID:      1,
	}
	err := app.Create(db)
	assert.NoError(t, err)

	datasource := &Datasource{Name: "Test Datasource"}
	err = datasource.Create(db)
	assert.NoError(t, err)

	// Add Datasource
	err = app.AddDatasource(db, datasource)
	assert.NoError(t, err)

	// Get Datasources
	err = app.GetDatasources(db)
	assert.NoError(t, err)
	assert.Len(t, app.Datasources, 1)
	assert.Equal(t, datasource.ID, app.Datasources[0].ID)

	// Remove Datasource
	err = app.RemoveDatasource(db, datasource)
	assert.NoError(t, err)

	err = app.GetDatasources(db)
	assert.NoError(t, err)
	assert.Len(t, app.Datasources, 0)
}

func TestApp_LLMAssociation(t *testing.T) {
	db := setupTestDB(t)

	app := &App{
		Name:        "Test App",
		Description: "This is a test app",
		UserID:      1,
	}
	err := app.Create(db)
	assert.NoError(t, err)

	llm := &LLM{Name: "Test LLM"}
	err = llm.Create(db)
	assert.NoError(t, err)

	// Add LLM
	err = app.AddLLM(db, llm)
	assert.NoError(t, err)

	// Get LLMs
	_, _, _, err = app.GetLLMs(db, 10, 1, true)
	assert.NoError(t, err)
	assert.Len(t, app.LLMs, 1)
	assert.Equal(t, llm.ID, app.LLMs[0].ID)

	// Remove LLM
	err = app.RemoveLLM(db, llm)
	assert.NoError(t, err)

	_, _, _, err = app.GetLLMs(db, 10, 1, true)
	assert.NoError(t, err)
	assert.Len(t, app.LLMs, 0)
}
