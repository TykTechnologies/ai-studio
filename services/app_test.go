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

	// Create test datasources and LLMs
	ds1, _ := service.CreateDatasource("DS1", "Short1", "Long1", "icon1.png", "https://ds1.com", 60, user.ID, []string{})
	ds2, _ := service.CreateDatasource("DS2", "Short2", "Long2", "icon2.png", "https://ds2.com", 70, user.ID, []string{})
	llm1, _ := service.CreateLLM("LLM1", "key1", "https://api1.com", "https://stream1.com", 80, "Short1", "Long1", "https://llm1.com", "https://logo1.com", models.OPENAI)
	llm2, _ := service.CreateLLM("LLM2", "key2", "https://api2.com", "https://stream2.com", 90, "Short2", "Long2", "https://llm2.com", "https://logo2.com", models.OPENAI)

	// Test CreateApp with valid privacy scores
	app, err := service.CreateApp("Test App", "Description", user.ID, []uint{ds1.ID, ds2.ID}, []uint{llm1.ID, llm2.ID})
	assert.NoError(t, err)
	assert.NotNil(t, app)
	assert.NotZero(t, app.ID)
	assert.NotZero(t, app.CredentialID)
	assert.Len(t, app.Datasources, 2)
	assert.Len(t, app.LLMs, 2)

	// Test CreateApp with invalid privacy scores
	invalidDS, _ := service.CreateDatasource("InvalidDS", "Short", "Long", "icon.png", "https://invalid.com", 95, user.ID, []string{})
	_, err = service.CreateApp("Invalid App", "Description", user.ID, []uint{invalidDS.ID}, []uint{llm1.ID, llm2.ID})
	assert.Error(t, err)

	// Test GetAppByID
	fetchedApp, err := service.GetAppByID(app.ID)
	assert.NoError(t, err)
	assert.Equal(t, app.ID, fetchedApp.ID)
	assert.Equal(t, app.Name, fetchedApp.Name)
	assert.Equal(t, app.Description, fetchedApp.Description)
	assert.Equal(t, app.UserID, fetchedApp.UserID)

	// Test UpdateApp with valid privacy scores
	updatedApp, err := service.UpdateApp(app.ID, "Updated App", "Updated Description", []uint{ds1.ID}, []uint{llm2.ID})
	assert.NoError(t, err)
	assert.Equal(t, app.ID, updatedApp.ID)
	assert.Equal(t, "Updated App", updatedApp.Name)
	assert.Equal(t, "Updated Description", updatedApp.Description)
	assert.Len(t, updatedApp.Datasources, 1)
	assert.Len(t, updatedApp.LLMs, 1)

	// Test UpdateApp with invalid privacy scores
	_, err = service.UpdateApp(app.ID, "Invalid Update", "Description", []uint{invalidDS.ID}, []uint{llm1.ID, llm2.ID})
	assert.Error(t, err)

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

	// Test AddDatasourceToApp and GetAppDatasources
	newDS, _ := service.CreateDatasource("NewDS", "Short", "Long", "icon.png", "https://newds.com", 65, user.ID, []string{})
	err = service.AddDatasourceToApp(app.ID, newDS.ID)
	assert.NoError(t, err)

	appDatasources, err := service.GetAppDatasources(app.ID)
	assert.NoError(t, err)
	assert.Len(t, appDatasources, 3)
	assert.Contains(t, []uint{appDatasources[0].ID, appDatasources[1].ID, appDatasources[2].ID}, newDS.ID)

	// Test RemoveDatasourceFromApp
	err = service.RemoveDatasourceFromApp(app.ID, newDS.ID)
	assert.NoError(t, err)

	appDatasources, err = service.GetAppDatasources(app.ID)
	assert.NoError(t, err)
	assert.Len(t, appDatasources, 2)

	// Test AddLLMToApp and GetAppLLMs
	newLLM, _ := service.CreateLLM("NewLLM", "newkey", "https://newapi.com", "https://newstream.com", 85, "NewShort", "NewLong", "https://newllm.com", "https://newlogo.com", models.OPENAI)
	err = service.AddLLMToApp(app.ID, newLLM.ID)
	assert.NoError(t, err)

	appLLMs, err := service.GetAppLLMs(app.ID)
	assert.NoError(t, err)
	assert.Len(t, appLLMs, 3)
	assert.Contains(t, []uint{appLLMs[0].ID, appLLMs[1].ID, appLLMs[2].ID}, newLLM.ID)

	// Test RemoveLLMFromApp
	err = service.RemoveLLMFromApp(app.ID, newLLM.ID)
	assert.NoError(t, err)

	appLLMs, err = service.GetAppLLMs(app.ID)
	assert.NoError(t, err)
	assert.Len(t, appLLMs, 2)

	// Test DeleteApp
	err = service.DeleteApp(app.ID)
	assert.NoError(t, err)

	// Verify app is deleted
	_, err = service.GetAppByID(app.ID)
	assert.Error(t, err)
}

func TestAppServiceErrorCases(t *testing.T) {
	db := setupTestDBForApps(t)
	service := NewService(db)

	// Create a test user and app
	user, err := service.CreateUser("test@example.com", "Test User", "password123")
	assert.NoError(t, err)

	app, err := service.CreateApp("Test App", "Description", user.ID, nil, nil)
	assert.NoError(t, err)

	// Test AddDatasourceToApp with non-existent datasource
	err = service.AddDatasourceToApp(app.ID, 9999)
	assert.Error(t, err)

	// Test RemoveDatasourceFromApp with non-existent datasource
	err = service.RemoveDatasourceFromApp(app.ID, 9999)
	assert.Error(t, err)

	// Test GetAppDatasources with non-existent app
	_, err = service.GetAppDatasources(9999)
	assert.Error(t, err)

	// Test AddLLMToApp with non-existent LLM
	err = service.AddLLMToApp(app.ID, 9999)
	assert.Error(t, err)

	// Test RemoveLLMFromApp with non-existent LLM
	err = service.RemoveLLMFromApp(app.ID, 9999)
	assert.Error(t, err)

	// Test GetAppLLMs with non-existent app
	_, err = service.GetAppLLMs(9999)
	assert.Error(t, err)

	// Test CreateApp with non-existent datasource
	_, err = service.CreateApp("Invalid App", "Description", user.ID, []uint{9999}, []uint{})
	assert.Error(t, err)

	// Test CreateApp with non-existent LLM
	_, err = service.CreateApp("Invalid App", "Description", user.ID, []uint{}, []uint{9999})
	assert.Error(t, err)

	// Test UpdateApp with non-existent datasource
	_, err = service.UpdateApp(app.ID, "Invalid Update", "Description", []uint{9999}, []uint{})
	assert.Error(t, err)

	// Test UpdateApp with non-existent LLM
	_, err = service.UpdateApp(app.ID, "Invalid Update", "Description", []uint{}, []uint{9999})
	assert.Error(t, err)
}

func TestAppService_MultipleApps(t *testing.T) {
	db := setupTestDBForApps(t)
	service := NewService(db)

	// Create test users
	user1, _ := service.CreateUser("user1@example.com", "User 1", "password123")
	user2, _ := service.CreateUser("user2@example.com", "User 2", "password456")

	// Create datasources and LLMs
	ds1, _ := service.CreateDatasource("DS1", "Short1", "Long1", "icon1.png", "https://ds1.com", 60, user1.ID, []string{})
	ds2, _ := service.CreateDatasource("DS2", "Short2", "Long2", "icon2.png", "https://ds2.com", 70, user2.ID, []string{})
	llm1, _ := service.CreateLLM("LLM1", "key1", "https://api1.com", "https://stream1.com", 80, "Short1", "Long1", "https://llm1.com", "https://logo1.com", models.OPENAI)
	llm2, _ := service.CreateLLM("LLM2", "key2", "https://api2.com", "https://stream2.com", 90, "Short2", "Long2", "https://llm2.com", "https://logo2.com", models.OPENAI)

	// Create multiple apps
	app1, _ := service.CreateApp("App 1", "Description 1", user1.ID, []uint{ds1.ID}, []uint{llm1.ID})
	app2, _ := service.CreateApp("App 2", "Description 2", user1.ID, []uint{ds1.ID}, []uint{llm2.ID})
	app3, _ := service.CreateApp("App 3", "Description 3", user2.ID, []uint{ds2.ID}, []uint{llm2.ID})

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
