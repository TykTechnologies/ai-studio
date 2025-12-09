package services

import (
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupAppTest(t *testing.T) (*Service, *gorm.DB) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	err = models.InitModels(db)
	assert.NoError(t, err)

	service := NewService(db)
	return service, db
}

// Helper to create test user
func createTestAppUser(t *testing.T, service *Service, email, name string) *models.User {
	user := &models.User{
		Email:    email,
		Name:     name,
		Password: "password123",
		IsAdmin:  false,
	}
	err := user.Create(service.DB)
	assert.NoError(t, err)
	return user
}

// Helper to create test LLM
func createTestAppLLM(t *testing.T, service *Service, name string, privacyScore int) *models.LLM {
	llm := &models.LLM{
		Name:             name,
		Vendor:           models.OPENAI,
		ShortDescription: "Test LLM",
		DefaultModel:     "gpt-4",
		PrivacyScore:     privacyScore,
		Active:           true,
	}
	err := llm.Create(service.DB)
	assert.NoError(t, err)
	return llm
}

// Helper to create test Datasource
func createTestAppDatasource(t *testing.T, service *Service, name string, privacyScore int) *models.Datasource {
	ds := &models.Datasource{
		Name:             name,
		DBSourceType:     "chroma",
		ShortDescription: "Test Datasource",
		PrivacyScore:     privacyScore,
		Active:           true,
	}
	err := ds.Create(service.DB)
	assert.NoError(t, err)
	return ds
}

// Helper to create test Tool
func createTestAppTool(t *testing.T, service *Service, name string) *models.Tool {
	tool := &models.Tool{
		Name:        name,
		Description: "Test Tool",
		ToolType:    "REST",
	}
	err := tool.Create(service.DB)
	assert.NoError(t, err)
	return tool
}

func TestCreateApp(t *testing.T) {
	service, _ := setupAppTest(t)

	user := createTestAppUser(t, service, "app@test.com", "App User")
	llm := createTestAppLLM(t, service, "test-llm", 5)
	ds := createTestAppDatasource(t, service, "test-ds", 3)
	tool := createTestAppTool(t, service, "test-tool")

	t.Run("Create app successfully", func(t *testing.T) {
		budget := 100.0
		startDate := time.Now()
		metadata := map[string]interface{}{"key": "value"}

		app, err := service.CreateApp(
			"Test App",
			"Test Description",
			user.ID,
			[]uint{ds.ID},
			[]uint{llm.ID},
			[]uint{tool.ID},
			&budget,
			&startDate,
			metadata,
		)

		assert.NoError(t, err)
		assert.NotNil(t, app)
		assert.Equal(t, "Test App", app.Name)
		assert.Equal(t, user.ID, app.UserID)
		assert.Len(t, app.Datasources, 1)
		assert.Len(t, app.LLMs, 1)
		assert.Len(t, app.Tools, 1)
		assert.Equal(t, budget, *app.MonthlyBudget)
		assert.NotZero(t, app.CredentialID)
	})

	t.Run("Create app with privacy score violation", func(t *testing.T) {
		// Create datasource with higher privacy score than LLM
		highPrivacyDS := createTestAppDatasource(t, service, "high-privacy-ds", 10)
		lowPrivacyLLM := createTestAppLLM(t, service, "low-privacy-llm", 3)

		app, err := service.CreateApp(
			"Bad App",
			"Privacy mismatch",
			user.ID,
			[]uint{highPrivacyDS.ID},
			[]uint{lowPrivacyLLM.ID},
			[]uint{},
			nil,
			nil,
			nil,
		)

		assert.Error(t, err)
		assert.Nil(t, app)
		assert.Equal(t, ERRPrivacyScoreMismatch, err)
	})

	t.Run("Create app with non-existent datasource", func(t *testing.T) {
		app, err := service.CreateApp(
			"Bad App",
			"Non-existent DS",
			user.ID,
			[]uint{99999},
			[]uint{llm.ID},
			[]uint{},
			nil,
			nil,
			nil,
		)

		assert.Error(t, err)
		assert.Nil(t, app)
	})

	t.Run("Create app with non-existent LLM", func(t *testing.T) {
		app, err := service.CreateApp(
			"Bad App",
			"Non-existent LLM",
			user.ID,
			[]uint{ds.ID},
			[]uint{99999},
			[]uint{},
			nil,
			nil,
			nil,
		)

		assert.Error(t, err)
		assert.Nil(t, app)
	})

	t.Run("Create app with non-existent tool", func(t *testing.T) {
		app, err := service.CreateApp(
			"Bad App",
			"Non-existent Tool",
			user.ID,
			[]uint{ds.ID},
			[]uint{llm.ID},
			[]uint{99999},
			nil,
			nil,
			nil,
		)

		assert.Error(t, err)
		assert.Nil(t, app)
	})
}

func TestCreateAppWithNamespace(t *testing.T) {
	service, _ := setupAppTest(t)

	user := createTestAppUser(t, service, "ns@test.com", "NS User")
	llm := createTestAppLLM(t, service, "ns-llm", 5)
	ds := createTestAppDatasource(t, service, "ns-ds", 3)

	t.Run("Create app with namespace", func(t *testing.T) {
		app, err := service.CreateAppWithNamespace(
			"Namespace App",
			"App with namespace",
			user.ID,
			[]uint{ds.ID},
			[]uint{llm.ID},
			[]uint{},
			nil,
			nil,
			"production",
			nil,
		)

		assert.NoError(t, err)
		assert.NotNil(t, app)
		assert.Equal(t, "production", app.Namespace)
	})

	t.Run("Create app with global namespace", func(t *testing.T) {
		app, err := service.CreateAppWithNamespace(
			"Global App",
			"Global namespace app",
			user.ID,
			[]uint{ds.ID},
			[]uint{llm.ID},
			[]uint{},
			nil,
			nil,
			"",
			nil,
		)

		assert.NoError(t, err)
		assert.NotNil(t, app)
		assert.Equal(t, "", app.Namespace)
	})
}

func TestUpdateApp(t *testing.T) {
	service, _ := setupAppTest(t)

	user := createTestAppUser(t, service, "update@test.com", "Update User")
	llm1 := createTestAppLLM(t, service, "llm1", 5)
	llm2 := createTestAppLLM(t, service, "llm2", 5)
	ds1 := createTestAppDatasource(t, service, "ds1", 3)
	ds2 := createTestAppDatasource(t, service, "ds2", 3)
	tool1 := createTestAppTool(t, service, "tool1")
	tool2 := createTestAppTool(t, service, "tool2")

	// Create initial app
	app, err := service.CreateApp(
		"Original App",
		"Original Description",
		user.ID,
		[]uint{ds1.ID},
		[]uint{llm1.ID},
		[]uint{tool1.ID},
		nil,
		nil,
		nil,
	)
	assert.NoError(t, err)

	t.Run("Update app successfully", func(t *testing.T) {
		budget := 200.0
		updated, err := service.UpdateApp(
			app.ID,
			"Updated App",
			"Updated Description",
			user.ID,
			[]uint{ds2.ID},
			[]uint{llm2.ID},
			[]uint{tool2.ID},
			&budget,
			nil,
			map[string]interface{}{"updated": true},
		)

		assert.NoError(t, err)
		assert.Equal(t, "Updated App", updated.Name)
		assert.Equal(t, "Updated Description", updated.Description)
		assert.Len(t, updated.Datasources, 1)
		assert.Equal(t, ds2.ID, updated.Datasources[0].ID)
		assert.Len(t, updated.LLMs, 1)
		assert.Equal(t, llm2.ID, updated.LLMs[0].ID)
		assert.Len(t, updated.Tools, 1)
		assert.Equal(t, tool2.ID, updated.Tools[0].ID)
	})

	t.Run("Update non-existent app", func(t *testing.T) {
		updated, err := service.UpdateApp(
			99999,
			"Non-existent",
			"Description",
			user.ID,
			[]uint{},
			[]uint{},
			[]uint{},
			nil,
			nil,
			nil,
		)

		assert.Error(t, err)
		assert.Nil(t, updated)
	})

	t.Run("Update app with privacy violation", func(t *testing.T) {
		highPrivacyDS := createTestAppDatasource(t, service, "high-ds", 10)
		lowPrivacyLLM := createTestAppLLM(t, service, "low-llm", 2)

		updated, err := service.UpdateApp(
			app.ID,
			"Bad Update",
			"Privacy violation",
			user.ID,
			[]uint{highPrivacyDS.ID},
			[]uint{lowPrivacyLLM.ID},
			[]uint{},
			nil,
			nil,
			nil,
		)

		assert.Error(t, err)
		assert.Nil(t, updated)
		assert.Equal(t, ERRPrivacyScoreMismatch, err)
	})
}

func TestGetAppByID(t *testing.T) {
	service, _ := setupAppTest(t)

	user := createTestAppUser(t, service, "get@test.com", "Get User")
	llm := createTestAppLLM(t, service, "get-llm", 5)

	app, _ := service.CreateApp("Get App", "Description", user.ID, []uint{}, []uint{llm.ID}, []uint{}, nil, nil, nil)

	t.Run("Get existing app", func(t *testing.T) {
		retrieved, err := service.GetAppByID(app.ID)
		assert.NoError(t, err)
		assert.Equal(t, app.ID, retrieved.ID)
		assert.Equal(t, "Get App", retrieved.Name)
	})

	t.Run("Get non-existent app", func(t *testing.T) {
		retrieved, err := service.GetAppByID(99999)
		assert.Error(t, err)
		assert.Nil(t, retrieved)
	})
}

func TestGetAppByCredentialID(t *testing.T) {
	service, _ := setupAppTest(t)

	user := createTestAppUser(t, service, "cred@test.com", "Cred User")
	llm := createTestAppLLM(t, service, "cred-llm", 5)

	app, _ := service.CreateApp("Cred App", "Description", user.ID, []uint{}, []uint{llm.ID}, []uint{}, nil, nil, nil)

	t.Run("Get app by credential ID", func(t *testing.T) {
		retrieved, err := service.GetAppByCredentialID(app.CredentialID)
		assert.NoError(t, err)
		assert.Equal(t, app.ID, retrieved.ID)
	})

	t.Run("Get app by non-existent credential ID", func(t *testing.T) {
		retrieved, err := service.GetAppByCredentialID(99999)
		assert.Error(t, err)
		assert.Nil(t, retrieved)
	})
}

func TestDeleteApp(t *testing.T) {
	service, _ := setupAppTest(t)

	user := createTestAppUser(t, service, "delete@test.com", "Delete User")
	llm := createTestAppLLM(t, service, "delete-llm", 5)

	app, _ := service.CreateApp("Delete App", "Description", user.ID, []uint{}, []uint{llm.ID}, []uint{}, nil, nil, nil)
	credentialID := app.CredentialID

	t.Run("Delete existing app", func(t *testing.T) {
		err := service.DeleteApp(app.ID)
		assert.NoError(t, err)

		// Verify app is deleted
		_, err = service.GetAppByID(app.ID)
		assert.Error(t, err)

		// Verify credential is deleted
		cred := &models.Credential{}
		err = cred.Get(service.DB, credentialID)
		assert.Error(t, err)
	})

	t.Run("Delete non-existent app", func(t *testing.T) {
		err := service.DeleteApp(99999)
		assert.Error(t, err)
	})
}

func TestGetAppsByUserID(t *testing.T) {
	service, _ := setupAppTest(t)

	user := createTestAppUser(t, service, "list@test.com", "List User")
	llm := createTestAppLLM(t, service, "list-llm", 5)

	// Create multiple apps for the user
	service.CreateApp("App 1", "Description 1", user.ID, []uint{}, []uint{llm.ID}, []uint{}, nil, nil, nil)
	service.CreateApp("App 2", "Description 2", user.ID, []uint{}, []uint{llm.ID}, []uint{}, nil, nil, nil)
	service.CreateApp("App 3", "Description 3", user.ID, []uint{}, []uint{llm.ID}, []uint{}, nil, nil, nil)

	t.Run("Get apps for user", func(t *testing.T) {
		apps, err := service.GetAppsByUserID(user.ID)
		assert.NoError(t, err)
		assert.Len(t, apps, 3)
	})

	t.Run("Get apps for user with no apps", func(t *testing.T) {
		apps, err := service.GetAppsByUserID(99999)
		assert.NoError(t, err)
		assert.Len(t, apps, 0)
	})
}

func TestGetAppByName(t *testing.T) {
	service, _ := setupAppTest(t)

	user := createTestAppUser(t, service, "name@test.com", "Name User")
	llm := createTestAppLLM(t, service, "name-llm", 5)

	service.CreateApp("Unique App Name", "Description", user.ID, []uint{}, []uint{llm.ID}, []uint{}, nil, nil, nil)

	t.Run("Get app by name", func(t *testing.T) {
		app, err := service.GetAppByName("Unique App Name")
		assert.NoError(t, err)
		assert.Equal(t, "Unique App Name", app.Name)
	})

	t.Run("Get app by non-existent name", func(t *testing.T) {
		app, err := service.GetAppByName("Non Existent App")
		assert.Error(t, err)
		assert.Nil(t, app)
	})
}

func TestActivateDeactivateAppCredential(t *testing.T) {
	service, _ := setupAppTest(t)

	user := createTestAppUser(t, service, "cred-test@test.com", "Cred Test User")
	llm := createTestAppLLM(t, service, "cred-test-llm", 5)

	app, _ := service.CreateApp("Cred Test App", "Description", user.ID, []uint{}, []uint{llm.ID}, []uint{}, nil, nil, nil)

	t.Run("Deactivate app credential", func(t *testing.T) {
		err := service.DeactivateAppCredential(app.ID)
		assert.NoError(t, err)

		// Verify credential is inactive
		cred := &models.Credential{}
		err = cred.Get(service.DB, app.CredentialID)
		assert.NoError(t, err)
		assert.False(t, cred.Active)
	})

	t.Run("Activate app credential", func(t *testing.T) {
		err := service.ActivateAppCredential(app.ID)
		assert.NoError(t, err)

		// Verify credential is active
		cred := &models.Credential{}
		err = cred.Get(service.DB, app.CredentialID)
		assert.NoError(t, err)
		assert.True(t, cred.Active)
	})

	t.Run("Activate credential for non-existent app", func(t *testing.T) {
		err := service.ActivateAppCredential(99999)
		assert.Error(t, err)
	})
}

func TestAddRemoveDatasourceFromApp(t *testing.T) {
	service, _ := setupAppTest(t)

	user := createTestAppUser(t, service, "ds-test@test.com", "DS Test User")
	llm := createTestAppLLM(t, service, "ds-test-llm", 5)
	ds := createTestAppDatasource(t, service, "ds-test-ds", 3)

	app, _ := service.CreateApp("DS Test App", "Description", user.ID, []uint{}, []uint{llm.ID}, []uint{}, nil, nil, nil)

	t.Run("Add datasource to app", func(t *testing.T) {
		err := service.AddDatasourceToApp(app.ID, ds.ID)
		assert.NoError(t, err)

		// Verify datasource is added
		retrieved, _ := service.GetAppByID(app.ID)
		assert.Len(t, retrieved.Datasources, 1)
	})

	t.Run("Remove datasource from app", func(t *testing.T) {
		err := service.RemoveDatasourceFromApp(app.ID, ds.ID)
		assert.NoError(t, err)

		// Verify datasource is removed
		retrieved, _ := service.GetAppByID(app.ID)
		assert.Len(t, retrieved.Datasources, 0)
	})

	t.Run("Add non-existent datasource", func(t *testing.T) {
		err := service.AddDatasourceToApp(app.ID, 99999)
		assert.Error(t, err)
	})
}

func TestGetAppDatasources(t *testing.T) {
	service, _ := setupAppTest(t)

	user := createTestAppUser(t, service, "ds-list@test.com", "DS List User")
	llm := createTestAppLLM(t, service, "ds-list-llm", 5)
	ds1 := createTestAppDatasource(t, service, "ds-list-ds1", 3)
	ds2 := createTestAppDatasource(t, service, "ds-list-ds2", 3)

	app, _ := service.CreateApp("DS List App", "Description", user.ID, []uint{ds1.ID, ds2.ID}, []uint{llm.ID}, []uint{}, nil, nil, nil)

	t.Run("Get app datasources", func(t *testing.T) {
		datasources, err := service.GetAppDatasources(app.ID)
		assert.NoError(t, err)
		assert.Len(t, datasources, 2)
	})
}

func TestAddRemoveLLMFromApp(t *testing.T) {
	service, _ := setupAppTest(t)

	user := createTestAppUser(t, service, "llm-test@test.com", "LLM Test User")
	llm1 := createTestAppLLM(t, service, "llm-test-llm1", 5)
	llm2 := createTestAppLLM(t, service, "llm-test-llm2", 5)

	app, _ := service.CreateApp("LLM Test App", "Description", user.ID, []uint{}, []uint{llm1.ID}, []uint{}, nil, nil, nil)

	t.Run("Add LLM to app", func(t *testing.T) {
		err := service.AddLLMToApp(app.ID, llm2.ID)
		assert.NoError(t, err)

		// Verify LLM is added
		retrieved, _ := service.GetAppByID(app.ID)
		assert.Len(t, retrieved.LLMs, 2)
	})

	t.Run("Remove LLM from app", func(t *testing.T) {
		err := service.RemoveLLMFromApp(app.ID, llm2.ID)
		assert.NoError(t, err)

		// Verify LLM is removed
		retrieved, _ := service.GetAppByID(app.ID)
		assert.Len(t, retrieved.LLMs, 1)
	})
}

func TestGetAppLLMs(t *testing.T) {
	service, _ := setupAppTest(t)

	user := createTestAppUser(t, service, "llm-list@test.com", "LLM List User")
	llm1 := createTestAppLLM(t, service, "llm-list-llm1", 5)
	llm2 := createTestAppLLM(t, service, "llm-list-llm2", 5)

	app, _ := service.CreateApp("LLM List App", "Description", user.ID, []uint{}, []uint{llm1.ID, llm2.ID}, []uint{}, nil, nil, nil)

	t.Run("Get app LLMs with pagination", func(t *testing.T) {
		llms, totalCount, totalPages, err := service.GetAppLLMs(app.ID, 10, 1, false)
		assert.NoError(t, err)
		assert.Len(t, llms, 2)
		assert.Equal(t, int64(2), totalCount)
		assert.Equal(t, 1, totalPages)
	})
}

func TestAddRemoveToolFromApp(t *testing.T) {
	service, _ := setupAppTest(t)

	user := createTestAppUser(t, service, "tool-test@test.com", "Tool Test User")
	llm := createTestAppLLM(t, service, "tool-test-llm", 5)
	tool1 := createTestAppTool(t, service, "tool-test-tool1")
	tool2 := createTestAppTool(t, service, "tool-test-tool2")

	app, _ := service.CreateApp("Tool Test App", "Description", user.ID, []uint{}, []uint{llm.ID}, []uint{tool1.ID}, nil, nil, nil)

	t.Run("Add tool to app", func(t *testing.T) {
		updated, err := service.AddToolToApp(app.ID, tool2.ID)
		assert.NoError(t, err)
		assert.Len(t, updated.Tools, 2)
	})

	t.Run("Remove tool from app", func(t *testing.T) {
		err := service.RemoveToolFromApp(app.ID, tool2.ID)
		assert.NoError(t, err)

		// Verify tool is removed
		retrieved, _ := service.GetAppByID(app.ID)
		assert.Len(t, retrieved.Tools, 1)
	})
}

func TestGetAppTools(t *testing.T) {
	service, _ := setupAppTest(t)

	user := createTestAppUser(t, service, "tool-list@test.com", "Tool List User")
	llm := createTestAppLLM(t, service, "tool-list-llm", 5)
	tool1 := createTestAppTool(t, service, "tool-list-tool1")
	tool2 := createTestAppTool(t, service, "tool-list-tool2")

	app, _ := service.CreateApp("Tool List App", "Description", user.ID, []uint{}, []uint{llm.ID}, []uint{tool1.ID, tool2.ID}, nil, nil, nil)

	t.Run("Get app tools", func(t *testing.T) {
		tools, err := service.GetAppTools(app.ID)
		assert.NoError(t, err)
		assert.Len(t, tools, 2)
	})
}

func TestListApps(t *testing.T) {
	service, _ := setupAppTest(t)

	user := createTestAppUser(t, service, "list-all@test.com", "List All User")
	llm := createTestAppLLM(t, service, "list-all-llm", 5)

	// Create multiple apps
	for i := 1; i <= 5; i++ {
		service.CreateApp("List App "+string(rune('0'+i)), "Description", user.ID, []uint{}, []uint{llm.ID}, []uint{}, nil, nil, nil)
	}

	t.Run("List all apps", func(t *testing.T) {
		apps, err := service.ListApps()
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, len(apps), 5)
	})
}

func TestListAppsWithPagination(t *testing.T) {
	service, _ := setupAppTest(t)

	user := createTestAppUser(t, service, "page-test@test.com", "Page Test User")
	llm := createTestAppLLM(t, service, "page-test-llm", 5)

	// Create apps
	for i := 1; i <= 7; i++ {
		service.CreateApp("Page App "+string(rune('0'+i)), "Description", user.ID, []uint{}, []uint{llm.ID}, []uint{}, nil, nil, nil)
	}

	t.Run("List apps with pagination", func(t *testing.T) {
		apps, totalCount, totalPages, err := service.ListAppsWithPagination(3, 1, false, "name")
		assert.NoError(t, err)
		assert.Len(t, apps, 3)
		assert.Equal(t, int64(7), totalCount)
		assert.Equal(t, 3, totalPages)
	})
}

func TestListAppsByUserID(t *testing.T) {
	service, _ := setupAppTest(t)

	user1 := createTestAppUser(t, service, "user1@test.com", "User 1")
	user2 := createTestAppUser(t, service, "user2@test.com", "User 2")
	llm := createTestAppLLM(t, service, "user-list-llm", 5)

	// Create apps for user1
	for i := 1; i <= 3; i++ {
		service.CreateApp("User1 App "+string(rune('0'+i)), "Description", user1.ID, []uint{}, []uint{llm.ID}, []uint{}, nil, nil, nil)
	}

	// Create apps for user2
	for i := 1; i <= 2; i++ {
		service.CreateApp("User2 App "+string(rune('0'+i)), "Description", user2.ID, []uint{}, []uint{llm.ID}, []uint{}, nil, nil, nil)
	}

	t.Run("List apps for specific user", func(t *testing.T) {
		apps, totalCount, _, err := service.ListAppsByUserID(user1.ID, 10, 1, false, "name")
		assert.NoError(t, err)
		assert.Len(t, apps, 3)
		assert.Equal(t, int64(3), totalCount)
	})
}

func TestSearchApps(t *testing.T) {
	service, _ := setupAppTest(t)

	user := createTestAppUser(t, service, "search@test.com", "Search User")
	llm := createTestAppLLM(t, service, "search-llm", 5)

	service.CreateApp("Production App", "Description", user.ID, []uint{}, []uint{llm.ID}, []uint{}, nil, nil, nil)
	service.CreateApp("Development App", "Description", user.ID, []uint{}, []uint{llm.ID}, []uint{}, nil, nil, nil)
	service.CreateApp("Testing Tool", "Description", user.ID, []uint{}, []uint{llm.ID}, []uint{}, nil, nil, nil)

	t.Run("Search apps", func(t *testing.T) {
		apps, totalCount, _, err := service.SearchApps("App", 10, 1, false, "name")
		assert.NoError(t, err)
		assert.Len(t, apps, 2) // Should find "Production App" and "Development App"
		assert.Equal(t, int64(2), totalCount)
	})
}

func TestListAppsWithFilters(t *testing.T) {
	service, _ := setupAppTest(t)

	user := createTestAppUser(t, service, "filter@test.com", "Filter User")
	llm := createTestAppLLM(t, service, "filter-llm", 5)

	// Create apps with different namespaces
	service.CreateAppWithNamespace("Global App", "Description", user.ID, []uint{}, []uint{llm.ID}, []uint{}, nil, nil, "", nil)
	service.CreateAppWithNamespace("Prod App", "Description", user.ID, []uint{}, []uint{llm.ID}, []uint{}, nil, nil, "production", nil)
	service.CreateAppWithNamespace("Dev App", "Description", user.ID, []uint{}, []uint{llm.ID}, []uint{}, nil, nil, "development", nil)

	t.Run("Filter apps by namespace", func(t *testing.T) {
		apps, totalCount, _, err := service.ListAppsWithFilters(10, 1, false, "name", "production", nil)
		assert.NoError(t, err)
		assert.Len(t, apps, 1)
		if len(apps) > 0 {
			assert.Equal(t, "production", apps[0].Namespace)
		}
		assert.Equal(t, int64(1), totalCount)
	})
}

func TestCountApps(t *testing.T) {
	service, _ := setupAppTest(t)

	user := createTestAppUser(t, service, "count@test.com", "Count User")
	llm := createTestAppLLM(t, service, "count-llm", 5)

	// Create apps
	for i := 1; i <= 5; i++ {
		service.CreateApp("Count App "+string(rune('0'+i)), "Description", user.ID, []uint{}, []uint{llm.ID}, []uint{}, nil, nil, nil)
	}

	t.Run("Count all apps", func(t *testing.T) {
		count, err := service.CountApps()
		assert.NoError(t, err)
		assert.Equal(t, int64(5), count)
	})
}

func TestCountAppsByUserID(t *testing.T) {
	service, _ := setupAppTest(t)

	user1 := createTestAppUser(t, service, "count-user1@test.com", "Count User 1")
	user2 := createTestAppUser(t, service, "count-user2@test.com", "Count User 2")
	llm := createTestAppLLM(t, service, "count-user-llm", 5)

	// Create apps for user1
	for i := 1; i <= 3; i++ {
		service.CreateApp("User1 Count "+string(rune('0'+i)), "Description", user1.ID, []uint{}, []uint{llm.ID}, []uint{}, nil, nil, nil)
	}

	// Create apps for user2
	service.CreateApp("User2 Count", "Description", user2.ID, []uint{}, []uint{llm.ID}, []uint{}, nil, nil, nil)

	t.Run("Count apps by user ID", func(t *testing.T) {
		count, err := service.CountAppsByUserID(user1.ID)
		assert.NoError(t, err)
		assert.Equal(t, int64(3), count)

		count, err = service.CountAppsByUserID(user2.ID)
		assert.NoError(t, err)
		assert.Equal(t, int64(1), count)
	})
}

func TestGetAppsInNamespace(t *testing.T) {
	service, _ := setupAppTest(t)

	user := createTestAppUser(t, service, "ns-get@test.com", "NS Get User")
	llm := createTestAppLLM(t, service, "ns-get-llm", 5)

	// Create apps in different namespaces
	service.CreateAppWithNamespace("Global App 1", "Description", user.ID, []uint{}, []uint{llm.ID}, []uint{}, nil, nil, "", nil)
	service.CreateAppWithNamespace("Prod App 1", "Description", user.ID, []uint{}, []uint{llm.ID}, []uint{}, nil, nil, "production", nil)
	service.CreateAppWithNamespace("Prod App 2", "Description", user.ID, []uint{}, []uint{llm.ID}, []uint{}, nil, nil, "production", nil)
	service.CreateAppWithNamespace("Dev App 1", "Description", user.ID, []uint{}, []uint{llm.ID}, []uint{}, nil, nil, "development", nil)

	t.Run("Get apps in production namespace", func(t *testing.T) {
		apps, err := service.GetAppsInNamespace("production")
		assert.NoError(t, err)
		// Should include global + production apps
		assert.GreaterOrEqual(t, len(apps), 2) // At least 2 production apps
	})

	t.Run("Get apps in global namespace", func(t *testing.T) {
		apps, err := service.GetAppsInNamespace("global")
		assert.NoError(t, err)
		// Should only include global apps
		for _, app := range apps {
			assert.Equal(t, "", app.Namespace)
		}
	})

	t.Run("Get all apps", func(t *testing.T) {
		apps, err := service.GetAppsInNamespace("all")
		assert.NoError(t, err)
		// Should return all apps regardless of namespace
		assert.Greater(t, len(apps), 0)
	})
}

func TestGetActiveAppsInNamespace(t *testing.T) {
	service, _ := setupAppTest(t)

	user := createTestAppUser(t, service, "active-ns@test.com", "Active NS User")
	llm := createTestAppLLM(t, service, "active-ns-llm", 5)

	// Create active apps
	service.CreateAppWithNamespace("Active App 1", "Description", user.ID, []uint{}, []uint{llm.ID}, []uint{}, nil, nil, "production", nil)

	// Create inactive app
	app, _ := service.CreateAppWithNamespace("Inactive App", "Description", user.ID, []uint{}, []uint{llm.ID}, []uint{}, nil, nil, "production", nil)
	app.IsActive = false
	service.DB.Save(app)

	t.Run("Get active apps in namespace", func(t *testing.T) {
		apps, err := service.GetActiveAppsInNamespace("production")
		assert.NoError(t, err)
		// Should only include active apps
		for _, app := range apps {
			assert.True(t, app.IsActive)
		}
	})
}

func TestConvertIDs(t *testing.T) {
	service, _ := setupAppTest(t)

	t.Run("Convert valid ID strings", func(t *testing.T) {
		ids, err := service.convertIDs([]string{"1", "2", "3"})
		assert.NoError(t, err)
		assert.Equal(t, []uint{1, 2, 3}, ids)
	})

	t.Run("Convert empty slice", func(t *testing.T) {
		ids, err := service.convertIDs([]string{})
		assert.NoError(t, err)
		assert.Equal(t, []uint{}, ids)
	})

	t.Run("Convert nil slice", func(t *testing.T) {
		ids, err := service.convertIDs(nil)
		assert.NoError(t, err)
		assert.Equal(t, []uint{}, ids)
	})

	t.Run("Convert invalid ID string", func(t *testing.T) {
		ids, err := service.convertIDs([]string{"invalid"})
		assert.Error(t, err)
		assert.Nil(t, ids)
	})
}

func TestValidatePrivacyScores(t *testing.T) {
	service, _ := setupAppTest(t)

	llm1 := createTestAppLLM(t, service, "privacy-llm1", 5)
	llm2 := createTestAppLLM(t, service, "privacy-llm2", 8)
	ds1 := createTestAppDatasource(t, service, "privacy-ds1", 3)
	ds2 := createTestAppDatasource(t, service, "privacy-ds2", 6)
	ds3 := createTestAppDatasource(t, service, "privacy-ds3", 10)

	t.Run("Valid privacy scores", func(t *testing.T) {
		err := service.validatePrivacyScores([]uint{ds1.ID}, []uint{llm1.ID})
		assert.NoError(t, err)
	})

	t.Run("Datasource privacy score equals LLM privacy score", func(t *testing.T) {
		err := service.validatePrivacyScores([]uint{ds1.ID}, []uint{llm1.ID})
		assert.NoError(t, err)
	})

	t.Run("Datasource privacy score higher than LLM", func(t *testing.T) {
		err := service.validatePrivacyScores([]uint{ds3.ID}, []uint{llm1.ID})
		assert.Error(t, err)
		assert.Equal(t, ERRPrivacyScoreMismatch, err)
	})

	t.Run("Multiple datasources and LLMs - valid", func(t *testing.T) {
		// ds2(6) < llm2(8)
		err := service.validatePrivacyScores([]uint{ds1.ID, ds2.ID}, []uint{llm1.ID, llm2.ID})
		assert.NoError(t, err)
	})

	t.Run("Multiple datasources and LLMs - invalid", func(t *testing.T) {
		// ds3(10) > llm2(8)
		err := service.validatePrivacyScores([]uint{ds1.ID, ds3.ID}, []uint{llm1.ID, llm2.ID})
		assert.Error(t, err)
		assert.Equal(t, ERRPrivacyScoreMismatch, err)
	})

	t.Run("Empty datasources", func(t *testing.T) {
		err := service.validatePrivacyScores([]uint{}, []uint{llm1.ID})
		assert.NoError(t, err)
	})

	t.Run("Empty LLMs", func(t *testing.T) {
		err := service.validatePrivacyScores([]uint{ds1.ID}, []uint{})
		assert.Error(t, err) // Datasource privacy score > -1 (no LLMs)
	})

	t.Run("Non-existent LLM", func(t *testing.T) {
		err := service.validatePrivacyScores([]uint{ds1.ID}, []uint{99999})
		assert.Error(t, err)
	})

	t.Run("Non-existent datasource", func(t *testing.T) {
		err := service.validatePrivacyScores([]uint{99999}, []uint{llm1.ID})
		assert.Error(t, err)
	})
}
