package services

import (
	"fmt"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupTestDBForApps is the same as the old version, minus any mailer setup.
func setupTestDBForApps(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	err = models.InitModels(db)
	assert.NoError(t, err)

	return db
}

func TestCreateAppWithNotifications(t *testing.T) {
	t.Run("with notifications enabled", func(t *testing.T) {
		db := setupTestDBForApps(t)
		notificationService := NewTestNotificationService(db)
		service := &Service{
			DB:                  db,
			NotificationService: notificationService,
		}

		// Create admin user with notifications enabled
		admin := &models.User{
			Email:                "admin@test.com",
			Name:                 "Admin",
			IsAdmin:              true,
			NotificationsEnabled: true,
			EmailVerified:        true,
		}
		err := admin.Create(db)
		assert.NoError(t, err)

		// Create app
		app, err := service.CreateApp("Test App", "Description", admin.ID, nil, nil)
		assert.NoError(t, err)
		assert.NotNil(t, app)

		// Verify notification was sent
		notifs := service.NotificationService.GetNotifications()
		assert.Len(t, notifs, 1)
		if len(notifs) > 0 {
			assert.Equal(t, admin.ID, notifs[0].UserID)
			assert.Contains(t, notifs[0].Title, "New App Created")
		}
	})

	t.Run("with notifications disabled", func(t *testing.T) {
		db := setupTestDBForApps(t)
		notificationService := NewTestNotificationService(db)
		service := &Service{
			DB:                  db,
			NotificationService: notificationService,
		}

		// Create admin user with notifications disabled
		admin := &models.User{
			Email:                "admin@test.com",
			Name:                 "Admin",
			IsAdmin:              true,
			NotificationsEnabled: false,
			EmailVerified:        true,
		}
		err := admin.Create(db)
		assert.NoError(t, err)

		// Create app - should succeed even with notifications disabled
		app, err := service.CreateApp("Test App", "Description", admin.ID, nil, nil)
		assert.NoError(t, err)
		assert.NotNil(t, app)

		// Verify no notification was created
		notifs := service.NotificationService.GetNotifications()
		assert.Len(t, notifs, 0)
	})
}

func TestCreateApp(t *testing.T) {
	db := setupTestDBForApps(t)
	notificationService := NewTestNotificationService(db)
	service := &Service{
		DB:                  db,
		NotificationService: notificationService,
	}

	user, _ := service.CreateUser("test@example.com", "Test User", "password123", true, true, true, true, true, true)
	ds1, _ := service.CreateDatasource("DS1", "Short1", "Long1", "icon1.png", "https://ds1.com", 60, user.ID, []string{}, "conn_string1", "source_type1", "api_key1", "db1", "embed_vendor1", "embed_url1", "embed_api_key1", "embed_model1", true)
	ds2, _ := service.CreateDatasource("DS2", "Short2", "Long2", "icon2.png", "https://ds2.com", 70, user.ID, []string{}, "conn_string2", "source_type2", "api_key2", "db2", "embed_vendor2", "embed_url2", "embed_api_key2", "embed_model2", true)
	llm1, _ := service.CreateLLM("LLM1", "key1", "https://api1.com", 80, "Short1", "Long1", "https://logo1.com", models.OPENAI, true, nil, "", []string{}, nil, nil)
	llm2, _ := service.CreateLLM("LLM2", "key2", "https://api2.com", 90, "Short2", "Long2", "https://logo2.com", models.OPENAI, true, nil, "", []string{}, nil, nil)

	app, err := service.CreateApp("Test App", "Description", user.ID, []uint{ds1.ID, ds2.ID}, []uint{llm1.ID, llm2.ID})
	assert.NoError(t, err)
	assert.NotNil(t, app)
	assert.NotZero(t, app.ID)
	assert.NotZero(t, app.CredentialID)
	assert.Len(t, app.Datasources, 2)
	assert.Len(t, app.LLMs, 2)

	invalidDS, _ := service.CreateDatasource("InvalidDS", "Short", "Long", "icon.png", "https://invalid.com", 95, user.ID, []string{}, "conn_string_invalid", "source_type_invalid", "api_key_invalid", "db1", "embed_vendor_invalid", "embed_url_invalid", "embed_api_key_invalid", "embed_model_invalid", true)
	_, err = service.CreateApp("Invalid App", "Description", user.ID, []uint{invalidDS.ID}, []uint{llm1.ID, llm2.ID})
	assert.Error(t, err)
}

func TestGetApp(t *testing.T) {
	db := setupTestDBForApps(t)
	notificationService := NewTestNotificationService(db)
	service := &Service{
		DB:                  db,
		NotificationService: notificationService,
	}

	user, _ := service.CreateUser("test@example.com", "Test User", "password123", true, true, true, true, true, true)
	ds1, _ := service.CreateDatasource("DS1", "Short1", "Long1", "icon1.png", "https://ds1.com", 60, user.ID, []string{}, "conn_string1", "source_type1", "api_key1", "db1", "embed_vendor1", "embed_url1", "embed_api_key1", "embed_model1", true)
	llm1, _ := service.CreateLLM("LLM1", "key1", "https://api1.com", 80, "Short1", "Long1", "https://logo1.com", models.OPENAI, true, nil, "", []string{}, nil, nil)

	app, _ := service.CreateApp("Test App", "Description", user.ID, []uint{ds1.ID}, []uint{llm1.ID})

	fetchedApp, err := service.GetAppByID(app.ID)
	assert.NoError(t, err)
	assert.Equal(t, app.ID, fetchedApp.ID)
	assert.Equal(t, app.Name, fetchedApp.Name)
	assert.Equal(t, app.Description, fetchedApp.Description)
	assert.Equal(t, app.UserID, fetchedApp.UserID)

	namedApp, err := service.GetAppByName("Test App")
	assert.NoError(t, err)
	assert.Equal(t, app.ID, namedApp.ID)

	userApps, err := service.GetAppsByUserID(user.ID)
	assert.NoError(t, err)
	assert.Len(t, userApps, 1)
	assert.Equal(t, app.ID, userApps[0].ID)
}

func TestUpdateApp(t *testing.T) {
	db := setupTestDBForApps(t)
	notificationService := NewTestNotificationService(db)
	service := &Service{
		DB:                  db,
		NotificationService: notificationService,
	}

	user, _ := service.CreateUser("test@example.com", "Test User", "password123", true, true, true, true, true, true)
	ds1, _ := service.CreateDatasource("DS1", "Short1", "Long1", "icon1.png", "https://ds1.com", 60, user.ID, []string{}, "conn_string1", "source_type1", "api_key1", "db1", "embed_vendor1", "embed_url1", "embed_api_key1", "embed_model1", true)
	llm1, _ := service.CreateLLM("LLM1", "key1", "https://api1.com", 80, "Short1", "Long1", "https://logo1.com", models.OPENAI, true, nil, "", []string{}, nil, nil)
	llm2, _ := service.CreateLLM("LLM2", "key2", "https://api2.com", 90, "Short2", "Long2", "https://logo2.com", models.OPENAI, true, nil, "", []string{}, nil, nil)

	app, _ := service.CreateApp("Test App", "Description", user.ID, []uint{ds1.ID}, []uint{llm1.ID})

	updatedApp, err := service.UpdateApp(app.ID, "Updated App", "Updated Description", []uint{ds1.ID}, []uint{llm2.ID}, nil, nil)
	assert.NoError(t, err)
	assert.Equal(t, app.ID, updatedApp.ID)
	assert.Equal(t, "Updated App", updatedApp.Name)
	assert.Equal(t, "Updated Description", updatedApp.Description)
	assert.Len(t, updatedApp.Datasources, 1)
	assert.Len(t, updatedApp.LLMs, 1)

	invalidDS, _ := service.CreateDatasource("InvalidDS", "Short", "Long", "icon.png", "https://invalid.com", 95, user.ID, []string{}, "conn_string_invalid", "source_type_invalid", "api_key_invalid", "db1", "embed_vendor_invalid", "embed_url_invalid", "embed_api_key_invalid", "embed_model_invalid", true)
	_, err = service.UpdateApp(app.ID, "Invalid Update", "Description", []uint{invalidDS.ID}, []uint{llm1.ID, llm2.ID}, nil, nil)
	assert.Error(t, err)
}

func TestAppCredentialActivation(t *testing.T) {
	db := setupTestDBForApps(t)
	notificationService := NewTestNotificationService(db)
	service := &Service{
		DB:                  db,
		NotificationService: notificationService,
	}

	user, _ := service.CreateUser("test@example.com", "Test User", "password123", true, true, true, true, true, true)
	ds1, _ := service.CreateDatasource("DS1", "Short1", "Long1", "icon1.png", "https://ds1.com", 60, user.ID, []string{}, "conn_string1", "source_type1", "api_key1", "db1", "embed_vendor1", "embed_url1", "embed_api_key1", "embed_model1", true)
	llm1, _ := service.CreateLLM("LLM1", "key1", "https://api1.com", 80, "Short1", "Long1", "https://logo1.com", models.OPENAI, true, nil, "", []string{}, nil, nil)

	app, _ := service.CreateApp("Test App", "Description", user.ID, []uint{ds1.ID}, []uint{llm1.ID})

	err := service.ActivateAppCredential(app.ID)
	assert.NoError(t, err)
	activatedApp, _ := service.GetAppByID(app.ID)
	assert.True(t, activatedApp.Credential.Active)

	err = service.DeactivateAppCredential(app.ID)
	assert.NoError(t, err)
	deactivatedApp, _ := service.GetAppByID(app.ID)
	assert.False(t, deactivatedApp.Credential.Active)
}

func TestAppDatasourceOperations(t *testing.T) {
	db := setupTestDBForApps(t)
	notificationService := NewTestNotificationService(db)
	service := &Service{
		DB:                  db,
		NotificationService: notificationService,
	}

	user, _ := service.CreateUser("test@example.com", "Test User", "password123", true, true, true, true, true, true)
	ds1, _ := service.CreateDatasource("DS1", "Short1", "Long1", "icon1.png", "https://ds1.com", 60, user.ID, []string{}, "conn_string1", "source_type1", "api_key1", "db1", "embed_vendor1", "embed_url1", "embed_api_key1", "embed_model1", true)
	llm1, _ := service.CreateLLM("LLM1", "key1", "https://api1.com", 80, "Short1", "Long1", "https://logo1.com", models.OPENAI, true, nil, "", []string{}, nil, nil)

	app, _ := service.CreateApp("Test App", "Description", user.ID, []uint{ds1.ID}, []uint{llm1.ID})

	newDS, _ := service.CreateDatasource("NewDS", "Short", "Long", "icon.png", "https://newds.com", 65, user.ID, []string{}, "conn_string_new", "source_type_new", "api_key_new", "db1", "embed_vendor_new", "embed_url_new", "embed_api_key_new", "embed_model_new", true)
	err := service.AddDatasourceToApp(app.ID, newDS.ID)
	assert.NoError(t, err)

	appDatasources, err := service.GetAppDatasources(app.ID)
	assert.NoError(t, err)
	assert.Len(t, appDatasources, 2)
	assert.Contains(t, []uint{appDatasources[0].ID, appDatasources[1].ID}, newDS.ID)

	err = service.RemoveDatasourceFromApp(app.ID, newDS.ID)
	assert.NoError(t, err)

	appDatasources, err = service.GetAppDatasources(app.ID)
	assert.NoError(t, err)
	assert.Len(t, appDatasources, 1)
}

func TestAppLLMOperations(t *testing.T) {
	db := setupTestDBForApps(t)
	notificationService := NewTestNotificationService(db)
	service := &Service{
		DB:                  db,
		NotificationService: notificationService,
	}

	user, _ := service.CreateUser("test@example.com", "Test User", "password123", true, true, true, true, true, true)
	ds1, _ := service.CreateDatasource("DS1", "Short1", "Long1", "icon1.png", "https://ds1.com", 60, user.ID, []string{}, "conn_string1", "source_type1", "api_key1", "db1", "embed_vendor1", "embed_url1", "embed_api_key1", "embed_model1", true)
	llm1, _ := service.CreateLLM("LLM1", "key1", "https://api1.com", 80, "Short1", "Long1", "https://logo1.com", models.OPENAI, true, nil, "", []string{}, nil, nil)

	app, _ := service.CreateApp("Test App", "Description", user.ID, []uint{}, []uint{})
	err := service.AddLLMToApp(app.ID, llm1.ID)
	assert.NoError(t, err)
	err = service.AddDatasourceToApp(app.ID, ds1.ID)
	assert.NoError(t, err)

	newLLM, err := service.CreateLLM("NewLLM", "newkey", "https://newapi.com", 85, "NewShort", "NewLong", "https://newlogo.com", models.OPENAI, true, nil, "", []string{}, nil, nil)
	assert.NoError(t, err)

	err = service.AddLLMToApp(app.ID, newLLM.ID)
	assert.NoError(t, err)

	appLLMs, totalCount, totalPages, err := service.GetAppLLMs(app.ID, 10, 1, true)
	assert.NoError(t, err)
	assert.Len(t, appLLMs, 2)
	if len(appLLMs) == 2 {
		assert.Equal(t, int64(2), totalCount)
		assert.Equal(t, 1, totalPages)
		assert.Contains(t, []uint{appLLMs[0].ID, appLLMs[1].ID}, newLLM.ID)
		assert.Contains(t, []uint{appLLMs[0].ID, appLLMs[1].ID}, llm1.ID)
	}

	err = service.RemoveLLMFromApp(app.ID, newLLM.ID)
	assert.NoError(t, err)

	appLLMs, totalCount, totalPages, err = service.GetAppLLMs(app.ID, 10, 1, true)
	assert.NoError(t, err)
	assert.Len(t, appLLMs, 1)
	assert.Equal(t, int64(1), totalCount)
	assert.Equal(t, 1, totalPages)
	assert.Equal(t, llm1.ID, appLLMs[0].ID)
}

func TestDeleteApp(t *testing.T) {
	db := setupTestDBForApps(t)
	notificationService := NewTestNotificationService(db)
	service := &Service{
		DB:                  db,
		NotificationService: notificationService,
	}

	user, _ := service.CreateUser("test@example.com", "Test User", "password123", true, true, true, true, true, true)
	ds1, _ := service.CreateDatasource("DS1", "Short1", "Long1", "icon1.png", "https://ds1.com", 60, user.ID, []string{}, "conn_string1", "source_type1", "api_key1", "db1", "embed_vendor1", "embed_url1", "embed_api_key1", "embed_model1", true)
	llm1, _ := service.CreateLLM("LLM1", "key1", "https://api1.com", 80, "Short1", "Long1", "https://logo1.com", models.OPENAI, true, nil, "", []string{}, nil, nil)

	app, _ := service.CreateApp("Test App", "Description", user.ID, []uint{ds1.ID}, []uint{llm1.ID})

	err := service.DeleteApp(app.ID)
	assert.NoError(t, err)

	_, err = service.GetAppByID(app.ID)
	assert.Error(t, err)
}

func TestAppServiceErrorCases(t *testing.T) {
	db := setupTestDBForApps(t)
	notificationService := NewTestNotificationService(db)
	service := &Service{
		DB:                  db,
		NotificationService: notificationService,
	}

	// Create a test user and app
	user, err := service.CreateUser("test@example.com", "Test User", "password123", true, true, true, true, true, true)
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
	_, _, _, err = service.GetAppLLMs(9999, 10, 1, true)
	assert.Error(t, err)

	// Test CreateApp with non-existent datasource
	_, err = service.CreateApp("Invalid App", "Description", user.ID, []uint{9999}, []uint{})
	assert.Error(t, err)

	// Test CreateApp with non-existent LLM
	_, err = service.CreateApp("Invalid App", "Description", user.ID, []uint{}, []uint{9999})
	assert.Error(t, err)

	// Test UpdateApp with non-existent datasource
	_, err = service.UpdateApp(app.ID, "Invalid Update", "Description", []uint{9999}, []uint{}, nil, nil)
	assert.Error(t, err)

	// Test UpdateApp with non-existent LLM
	_, err = service.UpdateApp(app.ID, "Invalid Update", "Description", []uint{}, []uint{9999}, nil, nil)
	assert.Error(t, err)
}

func TestAppService_MultipleApps(t *testing.T) {
	db := setupTestDBForApps(t)
	notificationService := NewTestNotificationService(db)
	service := &Service{
		DB:                  db,
		NotificationService: notificationService,
	}

	// Create test users
	user1, _ := service.CreateUser("user1@example.com", "User 1", "password123", true, true, true, true, true, true)
	user2, _ := service.CreateUser("user2@example.com", "User 2", "password456", true, true, true, true, true, true)

	// Create datasources and LLMs
	ds1, _ := service.CreateDatasource("DS1", "Short1", "Long1", "icon1.png", "https://ds1.com", 60, user1.ID, []string{}, "conn_string1", "source_type1", "api_key1", "db1", "embed_vendor1", "embed_url1", "embed_api_key1", "embed_model1", true)
	ds2, _ := service.CreateDatasource("DS2", "Short2", "Long2", "icon2.png", "https://ds2.com", 70, user2.ID, []string{}, "conn_string2", "source_type2", "api_key2", "db2", "embed_vendor2", "embed_url2", "embed_api_key2", "embed_model2", true)
	llm1, _ := service.CreateLLM("LLM1", "key1", "https://api1.com", 80, "Short1", "Long1", "https://logo1.com", models.OPENAI, true, nil, "", []string{}, nil, nil)
	llm2, _ := service.CreateLLM("LLM2", "key2", "https://api2.com", 90, "Short2", "Long2", "https://logo2.com", models.OPENAI, true, nil, "", []string{}, nil, nil)

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

func TestListApps(t *testing.T) {
	db := setupTestDBForApps(t)
	notificationService := NewTestNotificationService(db)
	service := &Service{
		DB:                  db,
		NotificationService: notificationService,
	}

	// Create test users
	user1, _ := service.CreateUser("user1@example.com", "User 1", "password123", true, true, true, true, true, true)
	user2, _ := service.CreateUser("user2@example.com", "User 2", "password456", true, true, true, true, true, true)

	// Create multiple apps
	app1, _ := service.CreateApp("App 1", "Description 1", user1.ID, nil, nil)
	app2, _ := service.CreateApp("App 2", "Description 2", user1.ID, nil, nil)
	app3, _ := service.CreateApp("App 3", "Description 3", user2.ID, nil, nil)

	// Test ListApps
	apps, err := service.ListApps()
	assert.NoError(t, err)
	assert.Len(t, apps, 3)
	assert.ElementsMatch(t, []uint{app1.ID, app2.ID, app3.ID}, []uint{apps[0].ID, apps[1].ID, apps[2].ID})
}

func TestListAppsWithPagination(t *testing.T) {
	db := setupTestDBForApps(t)
	notificationService := NewTestNotificationService(db)
	service := &Service{
		DB:                  db,
		NotificationService: notificationService,
	}

	user, _ := service.CreateUser("user@example.com", "User", "password123", true, true, true, true, true, true)

	// Create 5 apps
	for i := 1; i <= 5; i++ {
		_, _ = service.CreateApp(fmt.Sprintf("App %d", i), fmt.Sprintf("Description %d", i), user.ID, nil, nil)
	}

	// Test ListAppsWithPagination
	apps, _, _, err := service.ListAppsWithPagination(1, 3, false, "id")
	assert.NoError(t, err)
	assert.Len(t, apps, 1)

	apps, _, _, err = service.ListAppsWithPagination(2, 3, false, "id")
	assert.NoError(t, err)
	assert.Len(t, apps, 1)

	// Test with different sort orders
	appsAsc, _, _, err := service.ListAppsWithPagination(10, 1, true, "name")
	assert.NoError(t, err)
	assert.Len(t, appsAsc, 5)
	// Check if sorted in ascending order by name
	for i := 0; i < len(appsAsc)-1; i++ {
		assert.LessOrEqual(t, appsAsc[i].Name, appsAsc[i+1].Name)
	}

	appsDesc, _, _, err := service.ListAppsWithPagination(10, 1, true, "-name")
	assert.NoError(t, err)
	assert.Len(t, appsDesc, 5)
	// Check if sorted in descending order by name
	for i := 0; i < len(appsDesc)-1; i++ {
		assert.GreaterOrEqual(t, appsDesc[i].Name, appsDesc[i+1].Name)
	}
}

func TestListAppsByUserID(t *testing.T) {
	db := setupTestDBForApps(t)
	notificationService := NewTestNotificationService(db)
	service := &Service{
		DB:                  db,
		NotificationService: notificationService,
	}

	user1, _ := service.CreateUser("user1@example.com", "User 1", "password123", true, true, true, true, true, true)
	user2, _ := service.CreateUser("user2@example.com", "User 2", "password456", true, true, true, true, true, true)

	// Create 3 apps for user1 and 2 apps for user2
	for i := 1; i <= 3; i++ {
		_, _ = service.CreateApp(fmt.Sprintf("User1 App %d", i), "Description", user1.ID, nil, nil)
	}
	for i := 1; i <= 2; i++ {
		_, _ = service.CreateApp(fmt.Sprintf("User2 App %d", i), "Description", user2.ID, nil, nil)
	}

	// Test ListAppsByUserID
	user1Apps, _, _, err := service.ListAppsByUserID(user1.ID, 1, 10, true, "id")
	assert.NoError(t, err)
	assert.Len(t, user1Apps, 3)

	user2Apps, _, _, err := service.ListAppsByUserID(user2.ID, 1, 10, true, "id")
	assert.NoError(t, err)
	assert.Len(t, user2Apps, 2)

	// Test with different sort orders
	user1AppsAsc, _, _, err := service.ListAppsByUserID(user1.ID, 10, 1, true, "name")
	assert.NoError(t, err)
	assert.Len(t, user1AppsAsc, 3)
	// Check if sorted in ascending order by name
	for i := 0; i < len(user1AppsAsc)-1; i++ {
		assert.LessOrEqual(t, user1AppsAsc[i].Name, user1AppsAsc[i+1].Name)
	}

	user1AppsDesc, _, _, err := service.ListAppsByUserID(user1.ID, 10, 1, true, "-name")
	assert.NoError(t, err)
	assert.Len(t, user1AppsDesc, 3)
	// Check if sorted in descending order by name
	for i := 0; i < len(user1AppsDesc)-1; i++ {
		assert.GreaterOrEqual(t, user1AppsDesc[i].Name, user1AppsDesc[i+1].Name)
	}
}

func TestSearchApps(t *testing.T) {
	db := setupTestDBForApps(t)
	notificationService := NewTestNotificationService(db)
	service := &Service{
		DB:                  db,
		NotificationService: notificationService,
	}

	user, _ := service.CreateUser("user@example.com", "User", "password123", true, true, true, true, true, true)

	// Create apps with different names and descriptions
	_, _ = service.CreateApp("Test App", "This is a test app", user.ID, nil, nil)
	_, _ = service.CreateApp("Production App", "This is a production app", user.ID, nil, nil)
	_, _ = service.CreateApp("Development App", "This is a development app", user.ID, nil, nil)

	// Test SearchApps
	testApps, totalCount, totalPages, err := service.SearchApps("test", 1, 10, true, "id")
	t.Logf("Search for 'test': found %d apps, totalCount=%d, totalPages=%d", len(testApps), totalCount, totalPages)
	if len(testApps) > 0 {
		t.Logf("First app name: %s", testApps[0].Name)
	}
	assert.NoError(t, err)
	assert.Len(t, testApps, 1)
	assert.Equal(t, "Test App", testApps[0].Name)
	assert.Equal(t, int64(1), totalCount)
	assert.Equal(t, 1, totalPages)

	productionApps, totalCount, totalPages, err := service.SearchApps("production", 1, 10, true, "id")
	assert.NoError(t, err)
	assert.Len(t, productionApps, 1)
	assert.Equal(t, "Production App", productionApps[0].Name)
	assert.Equal(t, int64(1), totalCount)
	assert.Equal(t, 1, totalPages)

	allApps, totalCount, totalPages, err := service.SearchApps("app", 1, 10, true, "id")
	assert.NoError(t, err)
	assert.Len(t, allApps, 3)
	assert.Equal(t, int64(3), totalCount)
	assert.Equal(t, 1, totalPages)

	// Test search with different sort orders
	allAppsAsc, _, _, err := service.SearchApps("app", 1, 10, true, "name")
	assert.NoError(t, err)
	assert.Len(t, allAppsAsc, 3)
	// Check if sorted in ascending order by name
	for i := 0; i < len(allAppsAsc)-1; i++ {
		assert.LessOrEqual(t, allAppsAsc[i].Name, allAppsAsc[i+1].Name)
	}

	allAppsDesc, _, _, err := service.SearchApps("app", 1, 10, true, "-name")
	assert.NoError(t, err)
	assert.Len(t, allAppsDesc, 3)
	// Check if sorted in descending order by name
	for i := 0; i < len(allAppsDesc)-1; i++ {
		assert.GreaterOrEqual(t, allAppsDesc[i].Name, allAppsDesc[i+1].Name)
	}

	// Test search with no results
	noApps, totalCount, totalPages, err := service.SearchApps("nonexistent", 1, 10, true, "id")
	assert.NoError(t, err)
	assert.Len(t, noApps, 0)
	assert.Equal(t, int64(0), totalCount)
	assert.Equal(t, 1, totalPages)
}

func TestCountApps(t *testing.T) {
	db := setupTestDBForApps(t)
	notificationService := NewTestNotificationService(db)
	service := &Service{
		DB:                  db,
		NotificationService: notificationService,
	}

	user, _ := service.CreateUser("user@example.com", "User", "password123", true, true, true, true, true, true)

	// Create 5 apps
	for i := 1; i <= 5; i++ {
		_, _ = service.CreateApp(fmt.Sprintf("App %d", i), fmt.Sprintf("Description %d", i), user.ID, nil, nil)
	}

	// Test CountApps
	count, err := service.CountApps()
	assert.NoError(t, err)
	assert.Equal(t, int64(5), count)
}

func TestCountAppsByUserID(t *testing.T) {
	db := setupTestDBForApps(t)
	notificationService := NewTestNotificationService(db)
	service := &Service{
		DB:                  db,
		NotificationService: notificationService,
	}

	user1, _ := service.CreateUser("user1@example.com", "User 1", "password123", true, true, true, true, true, true)
	user2, _ := service.CreateUser("user2@example.com", "User 2", "password456", true, true, true, true, true, true)

	// Create 3 apps for user1 and 2 apps for user2
	for i := 1; i <= 3; i++ {
		_, _ = service.CreateApp(fmt.Sprintf("User1 App %d", i), "Description", user1.ID, nil, nil)
	}
	for i := 1; i <= 2; i++ {
		_, _ = service.CreateApp(fmt.Sprintf("User2 App %d", i), "Description", user2.ID, nil, nil)
	}

	// Test CountAppsByUserID
	user1Count, err := service.CountAppsByUserID(user1.ID)
	assert.NoError(t, err)
	assert.Equal(t, int64(3), user1Count)

	user2Count, err := service.CountAppsByUserID(user2.ID)
	assert.NoError(t, err)
	assert.Equal(t, int64(2), user2Count)
}
