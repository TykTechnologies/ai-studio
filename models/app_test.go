package models

import (
	"testing"
	// "time" // Removed as it's not directly used in this file's tests. app.go uses it.

	"github.com/stretchr/testify/assert"
	// "gorm.io/driver/sqlite" // Removed: setupTestDB from user_test.go handles this
	// "gorm.io/gorm" // Removed as it seems unused directly in this file
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
	assert.Empty(t, app.Tools)
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
	assert.Empty(t, app.Tools)
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

	// Add a datasource, LLM and Tool to the app
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

	tool := &Tool{Name: "Test Tool", ToolType: "REST"}
	err = tool.Create(db)
	assert.NoError(t, err)
	err = app.AddTool(db, tool)
	assert.NoError(t, err)

	// Fetch the updated app
	updatedApp := &App{}
	err = updatedApp.Get(db, app.ID)
	assert.NoError(t, err)

	// Check if the datasource, LLM and Tool were added correctly
	err = updatedApp.GetDatasources(db)
	assert.NoError(t, err)
	assert.Len(t, updatedApp.Datasources, 1)
	assert.Equal(t, datasource.ID, updatedApp.Datasources[0].ID)

	_, _, _, err = updatedApp.GetLLMs(db, 10, 1, true)
	assert.NoError(t, err)
	assert.Len(t, updatedApp.LLMs, 1)
	assert.Equal(t, llm.ID, updatedApp.LLMs[0].ID)

	_, err = updatedApp.GetTools(db)
	assert.NoError(t, err)
	assert.Len(t, updatedApp.Tools, 1)
	assert.Equal(t, tool.ID, updatedApp.Tools[0].ID)

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

	_, err = finalApp.GetTools(db)
	assert.NoError(t, err)
	assert.Len(t, finalApp.Tools, 1)
	assert.Equal(t, tool.ID, finalApp.Tools[0].ID)
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
	// defer cleanTestDB(db) // Removed

	user1 := User{Name: "User 1", Email: "user1@example.com", Password: "password"}
	user1.Create(db)
	user2 := User{Name: "User 2", Email: "user2@example.com", Password: "password"}
	user2.Create(db)

	// Create some test apps
	apps := []App{
		{Name: "App 1", Description: "Description 1", UserID: user1.ID},
		{Name: "App 2", Description: "Description 2", UserID: user1.ID},
		{Name: "App 3", Description: "Description 3", UserID: user2.ID},
	}
	for i := range apps {
		err := apps[i].Create(db)
		assert.NoError(t, err)
	}

	fetchedApps, err := (&App{}).GetByUserID(db, user1.ID)
	assert.NoError(t, err)
	assert.Len(t, fetchedApps, 2)

	// Check names, order might not be guaranteed
	appNames := []string{fetchedApps[0].Name, fetchedApps[1].Name}
	assert.Contains(t, appNames, "App 1")
	assert.Contains(t, appNames, "App 2")
}

func TestApp_GetByName(t *testing.T) {
	db := setupTestDB(t)
	// defer cleanTestDB(db) // Removed

	user := User{Name: "Test User", Email: "testuser@example.com", Password: "password"}
	user.Create(db)

	app := &App{
		Name:        "Unique App Name",
		Description: "This is a unique app",
		UserID:      user.ID,
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
	// defer cleanTestDB(db) // Removed
	user := User{Name: "Test User", Email: "test@example.com", Password: "password"}
	user.Create(db)

	app := &App{
		Name:        "Test App",
		Description: "This is a test app",
		UserID:      user.ID,
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
	// defer cleanTestDB(db) // Removed
	user := User{Name: "Test User", Email: "test@example.com", Password: "password"}
	user.Create(db)

	app := &App{
		Name:        "Test App",
		Description: "This is a test app",
		UserID:      user.ID,
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
	// defer cleanTestDB(db) // Removed
	user := User{Name: "Test User", Email: "test@example.com", Password: "password"}
	user.Create(db)

	app := &App{
		Name:        "Test App",
		Description: "This is a test app",
		UserID:      user.ID,
	}
	err := app.Create(db)
	assert.NoError(t, err)

	datasource := &Datasource{Name: "Test Datasource", UserID: user.ID}
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
	// defer cleanTestDB(db) // Removed
	user := User{Name: "Test User", Email: "test@example.com", Password: "password"}
	user.Create(db)

	app := &App{
		Name:        "Test App",
		Description: "This is a test app",
		UserID:      user.ID,
	}
	err := app.Create(db)
	assert.NoError(t, err)

	llm := &LLM{Name: "Test LLM", Vendor: "OpenAI", DefaultModel: "gpt-4"} // Changed ModelID to DefaultModel
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

	// Reload app associations
	app.LLMs = []LLM{} // Clear existing loaded LLMs
	_, _, _, err = app.GetLLMs(db, 10, 1, true)
	assert.NoError(t, err)
	assert.Len(t, app.LLMs, 0)
}

func TestApp_ToolAssociation(t *testing.T) {
	db := setupTestDB(t)
	// defer cleanTestDB(db) // Ensure database is cleaned up

	// Create a user for the app
	user := &User{Name: "Test User for App-Tool", Email: "apptool@example.com", Password: "password"}
	err := user.Create(db)
	assert.NoError(t, err)

	// Create an app
	app := &App{Name: "Tool Test App", UserID: user.ID}
	err = app.Create(db)
	assert.NoError(t, err)

	// Create a tool
	tool1 := &Tool{Name: "Test Tool 1", ToolType: "REST"}
	err = tool1.Create(db)
	assert.NoError(t, err)

	// 1. Test AddTool
	err = app.AddTool(db, tool1)
	assert.NoError(t, err)

	// Verify association by fetching the app again and checking its Tools field
	fetchedApp := &App{}
	err = db.Preload("Tools").First(fetchedApp, app.ID).Error
	assert.NoError(t, err)
	assert.Len(t, fetchedApp.Tools, 1, "App should have 1 tool associated")
	assert.Equal(t, tool1.ID, fetchedApp.Tools[0].ID, "Associated tool ID should match")

	// 2. Test GetTools
	retrievedTools, err := app.GetTools(db)
	assert.NoError(t, err)
	assert.Len(t, retrievedTools, 1, "GetTools should return 1 tool")
	assert.Equal(t, tool1.ID, retrievedTools[0].ID, "Retrieved tool ID should match")

	// Add another tool
	tool2 := &Tool{Name: "Test Tool 2", ToolType: "REST"}
	err = tool2.Create(db)
	assert.NoError(t, err)
	err = app.AddTool(db, tool2)
	assert.NoError(t, err)

	retrievedTools, err = app.GetTools(db)
	assert.NoError(t, err)
	assert.Len(t, retrievedTools, 2, "GetTools should return 2 tools")

	// 3. Test RemoveTool
	err = app.RemoveTool(db, tool1)
	assert.NoError(t, err)

	retrievedTools, err = app.GetTools(db)
	assert.NoError(t, err)
	assert.Len(t, retrievedTools, 1, "GetTools should return 1 tool after removal")
	assert.Equal(t, tool2.ID, retrievedTools[0].ID, "Remaining tool ID should be tool2's ID")

	// Test removing a tool not associated (should not error, GORM handles this gracefully)
	nonAssociatedTool := &Tool{Name: "Non Associated Tool"}
	nonAssociatedTool.ID = 999 // Non-existent or not associated
	err = app.RemoveTool(db, nonAssociatedTool)
	assert.NoError(t, err) // GORM's Delete for associations doesn't error if the target isn't found

	// Test GetTools on an app with no tools
	appWithNoTools := &App{Name: "No Tools App", UserID: user.ID}
	err = appWithNoTools.Create(db)
	assert.NoError(t, err)
	retrievedNoTools, err := appWithNoTools.GetTools(db)
	assert.NoError(t, err)
	assert.Len(t, retrievedNoTools, 0, "GetTools should return 0 tools for an app with no associations")

	// Test adding a tool that doesn't exist in DB (should fail at tool.Create or app.AddTool if tool is not persisted)
	// This is more of a service layer concern, model layer expects valid *Tool object.
	// For AddTool, GORM might try to create the association if the tool object has an ID,
	// but if the tool itself is not in the 'tools' table, foreign key constraints would typically fail.
	// However, GORM's behavior can vary. Let's assume tool must exist.
	nonExistentTool := &Tool{} // No ID, no Name, no ToolType. Should fail to save if Name/ToolType are NOT NULL.
	err = app.AddTool(db, nonExistentTool)
	// If GORM attempts to save nonExistentTool and it fails due to constraints (e.g., Name being required),
	// then err might not be nil here, or nonExistentTool.ID would remain 0.
	// The goal is that it's not successfully associated and persisted.

	// GORM saves the nonExistentTool (if ID is 0) and associates it.
	// So, err should be nil, and nonExistentTool.ID should now be non-zero.
	assert.NoError(t, err, "Adding a new tool (even empty) should not error with default GORM behavior")
	assert.NotZero(t, nonExistentTool.ID, "nonExistentTool ID should be non-zero as GORM should have saved it")

	// Let's check the count, it should increase by 1.
	currentToolCount := len(retrievedTools) // This was 1 (only tool2 was left)
	retrievedToolsAfterGhost, errGet := app.GetTools(db)
	assert.NoError(t, errGet) // Getting tools should still work
	assert.Len(t, retrievedToolsAfterGhost, currentToolCount+1, "Adding a new tool should increase the association count by 1")

	// Verify the newly added tool is indeed the ghostTool by its new ID
	var foundGhostTool bool
	for _, rt := range retrievedToolsAfterGhost {
		if rt.ID == nonExistentTool.ID {
			foundGhostTool = true
			break
		}
	}
	assert.True(t, foundGhostTool, "The ghost tool should be present in the retrieved tools")
}


func TestApps_GetAppCount(t *testing.T) {
	db := setupTestDB(t)
	// defer cleanTestDB(db) // Removed

	user := User{Name: "Test User", Email: "test@example.com", Password: "password"}
	user.Create(db)

	testApps := []App{
		{Name: "App 1", Description: "Description 1", UserID: user.ID},
		{Name: "App 2", Description: "Description 2", UserID: user.ID},
		{Name: "App 3", Description: "Description 3", UserID: user.ID},
	}

	for i := range testApps {
		err := testApps[i].Create(db)
		assert.NoError(t, err)
	}

	var apps Apps
	count, err := apps.GetAppCount(db)

	assert.NoError(t, err)
	assert.Equal(t, int64(len(testApps)), count)
}

// setupTestDB initializes an in-memory SQLite database for testing
// setupTestDB is now expected to be used from user_test.go to avoid redeclaration.
// cleanTestDB is also removed; tests should manage their own cleanup or rely on in-memory DB behavior.
// If InitModels in user_test.go's setupTestDB is not comprehensive, this might need adjustment.
