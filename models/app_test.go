package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
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
	defer cleanTestDB(db)

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
	defer cleanTestDB(db)

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
	defer cleanTestDB(db)
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
	defer cleanTestDB(db)
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
	defer cleanTestDB(db)
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
	defer cleanTestDB(db)
	user := User{Name: "Test User", Email: "test@example.com", Password: "password"}
	user.Create(db)

	app := &App{
		Name:        "Test App",
		Description: "This is a test app",
		UserID:      user.ID,
	}
	err := app.Create(db)
	assert.NoError(t, err)

	llm := &LLM{Name: "Test LLM", Vendor: "OpenAI", ModelID: "gpt-4"}
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
	defer cleanTestDB(db) // Ensure database is cleaned up

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
	nonExistentTool := &Tool{Name: "Ghost Tool"} // No ID, not created
	err = app.AddTool(db, nonExistentTool)
	// GORM might allow adding a non-persisted object to an association if cascade save is on.
	// For many2many, it usually expects existing records.
	// Let's check the count, it should not increase if the tool isn't valid for association.
	currentToolCount := len(retrievedTools)
	retrievedToolsAfterGhost, _ := app.GetTools(db)
	assert.Len(t, retrievedToolsAfterGhost, currentToolCount, "Adding a non-persisted tool should not change association count without cascade")

}


func TestApps_GetAppCount(t *testing.T) {
	db := setupTestDB(t)
	defer cleanTestDB(db)

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
// and migrates the schema.
func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to connect to database: %v", err)
	}

	// Auto-migrate all necessary models
	err = db.AutoMigrate(
		&User{}, &Group{}, &LLM{}, &Catalogue{}, &Tags{},
		&Datasource{}, &DataCatalogue{}, &Credential{}, &App{},
		&LLMSettings{}, &Chat{}, &CMessage{}, &Tool{}, &ModelPrice{},
		&Filter{}, &ChatHistoryRecord{}, &ToolCatalogue{}, &AppTool{}, // Added AppTool
	)
	if err != nil {
		t.Fatalf("Failed to migrate database: %v", err)
	}
	return db
}

// cleanTestDB drops all tables from the test database.
func cleanTestDB(db *gorm.DB) {
	// Order matters due to foreign key constraints
	tables := []string{
		"app_tools", "app_llms", "app_datasources", // Join tables first
		"tool_dependencies", "tool_filters", "tool_filestores",
		"group_tool_catalogues", "group_data_catalogues", "group_catalogues", "user_groups",
		"catalogue_llms", "catalogue_tags", "data_catalogue_datasources", "data_catalogue_tags",
		"tool_catalogue_tools", "tool_catalogue_tags",
		"tools", "filters", "filestores", "datasources", "llms", "secrets",
		"credentials", "apps", "users", "groups", "catalogues", "tags",
		"data_catalogues", "tool_catalogues", "llm_settings",
		"model_prices", "chat_history_records", "c_messages", "chats", "notifications", "prompt_templates",
		"llm_chat_records",
	}
	for _, table := range tables {
		if err := db.Migrator().DropTable(table); err != nil {
			// t.Logf("Failed to drop table %s: %v", table, err) // Log instead of fail for cleanup
		}
	}
}
