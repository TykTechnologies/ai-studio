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
		// Create inactive LLM (create as active first due to GORM default)
		inactiveLLM := &database.LLM{
			Name:         "Inactive LLM",
			Slug:         "inactive-llm",
			Vendor:       "anthropic",
			DefaultModel: "claude-3",
			IsActive:     true,
		}
		repo.CreateLLM(inactiveLLM)
		
		// Now update to inactive
		inactiveLLM.IsActive = false
		repo.UpdateLLM(inactiveLLM)

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
		assert.Contains(t, err.Error(), "LLM not found") // Inactive LLMs treated as not found for security
	})
}

// TestDatabaseGatewayService_GetCredentialBySecret removed - credential authentication not supported anymore

// TestDatabaseGatewayService_GetAppByCredentialID removed - credential authentication not supported anymore

// TestDatabaseGatewayService_Reload removed - cache functionality was removed during simplification
