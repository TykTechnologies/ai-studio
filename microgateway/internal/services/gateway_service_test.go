// internal/services/gateway_service_test.go
package services

import (
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/auth"
	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupGatewayTestDB(t *testing.T) (*gorm.DB, *database.Repository, *auth.TokenCache) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Migrate
	err = database.Migrate(db)
	require.NoError(t, err)

	repo := database.NewRepository(db)
	cache := auth.NewTokenCache(100, 5*time.Minute)

	return db, repo, cache
}

func TestDatabaseGatewayService_GetActiveLLMs(t *testing.T) {
	db, repo, cache := setupGatewayTestDB(t)
	defer database.Close(db)
	defer cache.Close()

	service := NewDatabaseGatewayService(db, repo).(*DatabaseGatewayService)

	// Create test LLMs
	activeLLM := &database.LLM{
		Name:         "Active LLM",
		Slug:         "active-llm",
		Vendor:       "openai",
		DefaultModel: "gpt-4",
		IsActive:     true,
	}
	inactiveLLM := &database.LLM{
		Name:         "Inactive LLM",
		Slug:         "inactive-llm",
		Vendor:       "anthropic",
		DefaultModel: "claude-3",
		IsActive:     true, // Create as active first
	}

	repo.CreateLLM(activeLLM)
	repo.CreateLLM(inactiveLLM)
	
	// Now update to inactive
	inactiveLLM.IsActive = false
	repo.UpdateLLM(inactiveLLM)

	t.Run("GetActiveLLMs", func(t *testing.T) {
		llms, err := service.GetActiveLLMs()
		assert.NoError(t, err)
		assert.Len(t, llms, 1)

		// Verify it's the active one
		llm := llms[0].(*database.LLM)
		assert.Equal(t, "Active LLM", llm.Name)
		assert.True(t, llm.IsActive)
	})
}

func TestDatabaseGatewayService_GetLLMBySlug(t *testing.T) {
	db, repo, cache := setupGatewayTestDB(t)
	defer database.Close(db)
	defer cache.Close()

	service := NewDatabaseGatewayService(db, repo).(*DatabaseGatewayService)

	// Create test LLM
	llm := &database.LLM{
		Name:         "Test LLM",
		Slug:         "test-llm",
		Vendor:       "openai",
		DefaultModel: "gpt-4",
		IsActive:     true,
	}
	repo.CreateLLM(llm)

	t.Run("ValidSlug", func(t *testing.T) {
		result, err := service.GetLLMBySlug("test-llm")
		assert.NoError(t, err)
		assert.NotNil(t, result)

		llmResult := result.(*database.LLM)
		assert.Equal(t, "Test LLM", llmResult.Name)
		assert.Equal(t, "test-llm", llmResult.Slug)
	})

	t.Run("InvalidSlug", func(t *testing.T) {
		_, err := service.GetLLMBySlug("nonexistent-llm")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "LLM not found")
	})

	t.Run("InactiveLLM", func(t *testing.T) {
		// Create inactive LLM
		inactiveLLM := &database.LLM{
			Name:         "Inactive LLM",
			Slug:         "inactive-llm",
			Vendor:       "anthropic",
			DefaultModel: "claude-3",
			IsActive:     false,
		}
		repo.CreateLLM(inactiveLLM)

		_, err := service.GetLLMBySlug("inactive-llm")
		assert.Error(t, err) // Should not find inactive LLMs
	})
}

func TestDatabaseGatewayService_ValidateAppAccess(t *testing.T) {
	db, repo, cache := setupGatewayTestDB(t)
	defer database.Close(db)
	defer cache.Close()

	service := NewDatabaseGatewayService(db, repo).(*DatabaseGatewayService)

	// Create test app and LLM
	app := &database.App{
		Name:       "Test App",
		OwnerEmail: "test@example.com",
		IsActive:   true,
	}
	repo.CreateApp(app)

	llm := &database.LLM{
		Name:         "Test LLM",
		Slug:         "test-llm",
		Vendor:       "openai",
		DefaultModel: "gpt-4",
		IsActive:     true,
	}
	repo.CreateLLM(llm)

	t.Run("NoAccess", func(t *testing.T) {
		err := service.ValidateAppAccess(app.ID, "test-llm")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "does not have access")
	})

	t.Run("WithAccess", func(t *testing.T) {
		// Create app-LLM association
		appLLM := &database.AppLLM{
			AppID:     app.ID,
			LLMID:     llm.ID,
			IsActive:  true,
			CreatedAt: time.Now(),
		}
		db.Create(appLLM)

		err := service.ValidateAppAccess(app.ID, "test-llm")
		assert.NoError(t, err)
	})

	t.Run("InactiveLLM", func(t *testing.T) {
		// Make LLM inactive
		llm.IsActive = false
		repo.UpdateLLM(llm)

		err := service.ValidateAppAccess(app.ID, "test-llm")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "LLM is inactive")
	})
}

func TestDatabaseGatewayService_GetCredentialBySecret(t *testing.T) {
	db, repo, cache := setupGatewayTestDB(t)
	defer database.Close(db)
	defer cache.Close()

	service := NewDatabaseGatewayService(db, repo).(*DatabaseGatewayService)

	// Create test app and credential
	app := &database.App{
		Name:       "Test App",
		OwnerEmail: "test@example.com",
		IsActive:   true,
	}
	repo.CreateApp(app)

	secret := "test-secret-123"
	cred := &database.Credential{
		AppID:      app.ID,
		KeyID:      "test-key",
		SecretHash: service.hashSecret(secret),
		IsActive:   true,
	}
	repo.CreateCredential(cred)

	t.Run("ValidSecret", func(t *testing.T) {
		result, err := service.GetCredentialBySecret(secret)
		assert.NoError(t, err)
		assert.NotNil(t, result)
	})

	t.Run("InvalidSecret", func(t *testing.T) {
		_, err := service.GetCredentialBySecret("invalid-secret")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid credentials")
	})

	t.Run("CachedSecret", func(t *testing.T) {
		// First call to populate cache
		service.GetCredentialBySecret(secret)

		// Second call should hit cache
		result, err := service.GetCredentialBySecret(secret)
		assert.NoError(t, err)
		assert.NotNil(t, result)
	})
}

func TestDatabaseGatewayService_GetAppByCredentialID(t *testing.T) {
	db, repo, cache := setupGatewayTestDB(t)
	defer database.Close(db)
	defer cache.Close()

	service := NewDatabaseGatewayService(db, repo).(*DatabaseGatewayService)

	// Create test app and credential
	app := &database.App{
		Name:       "Test App",
		OwnerEmail: "test@example.com",
		IsActive:   true,
	}
	repo.CreateApp(app)

	cred := &database.Credential{
		AppID:      app.ID,
		KeyID:      "test-key",
		SecretHash: "hashed-secret",
		IsActive:   true,
	}
	repo.CreateCredential(cred)

	t.Run("ValidCredentialID", func(t *testing.T) {
		result, err := service.GetAppByCredentialID(cred.ID)
		assert.NoError(t, err)
		assert.NotNil(t, result)

		appResult := result.(*database.App)
		assert.Equal(t, app.Name, appResult.Name)
	})

	t.Run("InvalidCredentialID", func(t *testing.T) {
		_, err := service.GetAppByCredentialID(999)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "credential not found")
	})

	t.Run("InactiveApp", func(t *testing.T) {
		// Deactivate app
		app.IsActive = false
		repo.UpdateApp(app)

		_, err := service.GetAppByCredentialID(cred.ID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "app is inactive")
	})
}

func TestDatabaseGatewayService_Reload(t *testing.T) {
	db, repo, cache := setupGatewayTestDB(t)
	defer database.Close(db)
	defer cache.Close()

	service := NewDatabaseGatewayService(db, repo).(*DatabaseGatewayService)

	t.Run("ReloadClearsCache", func(t *testing.T) {
		// Add something to cache
		cache.SetCredential("test-secret", "key-123", 1, 5*time.Minute)
		assert.NotNil(t, cache.GetCredential("test-secret"))

		// Reload should clear cache
		err := service.Reload()
		assert.NoError(t, err)
		assert.Nil(t, cache.GetCredential("test-secret"))
	})
}
