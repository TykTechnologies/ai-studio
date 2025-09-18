// internal/services/management_service_test.go
package services

import (
	"testing"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// Helper functions
func stringPtr(s string) *string { return &s }
func intPtr(i int) *int { return &i }
func boolPtr(b bool) *bool { return &b }

func setupManagementTestDB(t *testing.T) (*gorm.DB, *database.Repository, *ManagementService) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Migrate
	err = database.Migrate(db)
	require.NoError(t, err)

	repo := database.NewRepository(db)
	crypto := NewCryptoService("12345678901234567890123456789012")
	service := NewManagementService(db, repo, crypto).(*ManagementService)

	return db, repo, service
}

func TestManagementService_CreateLLM(t *testing.T) {
	db, _, service := setupManagementTestDB(t)
	defer database.Close(db)

	t.Run("ValidCreate", func(t *testing.T) {
		req := &CreateLLMRequest{
			Name:         "Test OpenAI",
			Vendor:       "openai",
			DefaultModel: "gpt-4",
			APIKey:       "sk-test123",
			MaxTokens:    8192,
			IsActive:     true,
		}

		llm, err := service.CreateLLM(req)
		assert.NoError(t, err)
		assert.NotNil(t, llm)
		assert.Equal(t, "Test OpenAI", llm.Name)
		assert.Equal(t, "test-openai", llm.Slug)
		assert.Equal(t, "openai", llm.Vendor)
		assert.Equal(t, 8192, llm.MaxTokens)
		assert.NotEmpty(t, llm.APIKeyEncrypted) // Should be encrypted
	})

	t.Run("MissingAPIKey", func(t *testing.T) {
		req := &CreateLLMRequest{
			Name:         "Test OpenAI No Key",
			Vendor:       "openai",
			DefaultModel: "gpt-4",
			// Missing APIKey
		}

		_, err := service.CreateLLM(req)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "API key is required")
	})

	t.Run("DuplicateName", func(t *testing.T) {
		req1 := &CreateLLMRequest{
			Name:         "Duplicate LLM",
			Vendor:       "openai",
			DefaultModel: "gpt-4",
			APIKey:       "sk-test123",
		}
		req2 := &CreateLLMRequest{
			Name:         "Duplicate LLM", // Same name
			Vendor:       "anthropic",
			DefaultModel: "claude-3",
			APIKey:       "sk-ant-123",
		}

		_, err := service.CreateLLM(req1)
		assert.NoError(t, err)

		_, err = service.CreateLLM(req2)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already exists")
	})

	t.Run("OllamaEndpointRequired", func(t *testing.T) {
		req := &CreateLLMRequest{
			Name:         "Test Ollama",
			Vendor:       "ollama",
			DefaultModel: "llama2",
			// Missing Endpoint
		}

		_, err := service.CreateLLM(req)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "endpoint is required")
	})

	t.Run("DefaultValues", func(t *testing.T) {
		req := &CreateLLMRequest{
			Name:         "Test Defaults",
			Vendor:       "anthropic",
			DefaultModel: "claude-3",
			APIKey:       "sk-ant-123",
			// No MaxTokens, TimeoutSeconds, RetryCount specified
		}

		llm, err := service.CreateLLM(req)
		assert.NoError(t, err)
		assert.Equal(t, 4096, llm.MaxTokens)        // Default
		assert.Equal(t, 30, llm.TimeoutSeconds)     // Default
		assert.Equal(t, 3, llm.RetryCount)          // Default
	})
}

func TestManagementService_GetLLM(t *testing.T) {
	db, repo, service := setupManagementTestDB(t)
	defer database.Close(db)

	// Create test LLM
	llm := &database.LLM{
		Name:         "Test LLM",
		Slug:         "test-llm",
		Vendor:       "openai",
		DefaultModel: "gpt-4",
		IsActive:     true,
	}
	repo.CreateLLM(llm)

	t.Run("ValidGet", func(t *testing.T) {
		retrieved, err := service.GetLLM(llm.ID)
		assert.NoError(t, err)
		assert.Equal(t, llm.Name, retrieved.Name)
		assert.Equal(t, llm.Slug, retrieved.Slug)
	})

	t.Run("NotFound", func(t *testing.T) {
		_, err := service.GetLLM(999)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "LLM not found")
	})
}

func TestManagementService_UpdateLLM(t *testing.T) {
	db, repo, service := setupManagementTestDB(t)
	defer database.Close(db)

	// Create test LLM
	llm := &database.LLM{
		Name:         "Test LLM",
		Slug:         "test-llm",
		Vendor:       "openai",
		DefaultModel: "gpt-4",
		MaxTokens:    4096,
		IsActive:     true,
	}
	repo.CreateLLM(llm)

	t.Run("ValidUpdate", func(t *testing.T) {
		updateReq := &UpdateLLMRequest{
			Name:      stringPtr("Updated LLM"),
			MaxTokens: intPtr(8192),
			IsActive:  boolPtr(false),
		}

		updated, err := service.UpdateLLM(llm.ID, updateReq)
		assert.NoError(t, err)
		assert.Equal(t, "Updated LLM", updated.Name)
		assert.Equal(t, "updated-llm", updated.Slug) // Slug should update with name
		assert.Equal(t, 8192, updated.MaxTokens)
		assert.False(t, updated.IsActive)
	})

	t.Run("UpdateNotFound", func(t *testing.T) {
		updateReq := &UpdateLLMRequest{
			Name: stringPtr("Nonexistent"),
		}

		_, err := service.UpdateLLM(999, updateReq)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "LLM not found")
	})

	t.Run("UpdateAPIKey", func(t *testing.T) {
		updateReq := &UpdateLLMRequest{
			APIKey: stringPtr("new-api-key-123"),
		}

		updated, err := service.UpdateLLM(llm.ID, updateReq)
		assert.NoError(t, err)
		assert.NotEmpty(t, updated.APIKeyEncrypted)
		
		// Verify the key can be decrypted (test the encryption worked)
		crypto := NewCryptoService("12345678901234567890123456789012")
		decrypted, err := crypto.Decrypt(updated.APIKeyEncrypted)
		assert.NoError(t, err)
		assert.Equal(t, "new-api-key-123", decrypted)
	})
}

func TestManagementService_CreateApp(t *testing.T) {
	db, _, service := setupManagementTestDB(t)
	defer database.Close(db)

	t.Run("ValidCreate", func(t *testing.T) {
		req := &CreateAppRequest{
			Name:          "Test Application",
			Description:   "A test app",
			OwnerEmail:    "owner@example.com",
			MonthlyBudget: 500.0,
			RateLimitRPM:  2000,
			AllowedIPs:    []string{"192.168.1.1", "10.0.0.0/8"},
		}

		app, err := service.CreateApp(req)
		assert.NoError(t, err)
		assert.Equal(t, "Test Application", app.Name)
		assert.Equal(t, "owner@example.com", app.OwnerEmail)
		assert.Equal(t, 500.0, app.MonthlyBudget)
		assert.Equal(t, 2000, app.RateLimitRPM)
		assert.True(t, app.IsActive)
		assert.Equal(t, 1, app.BudgetResetDay) // Default
	})

	t.Run("MissingName", func(t *testing.T) {
		req := &CreateAppRequest{
			OwnerEmail: "owner@example.com",
			// Missing Name
		}

		_, err := service.CreateApp(req)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "app name is required")
	})

	t.Run("MissingEmail", func(t *testing.T) {
		req := &CreateAppRequest{
			Name: "Test App",
			// Missing OwnerEmail
		}

		_, err := service.CreateApp(req)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "owner email is required")
	})
}

func TestManagementService_CreateCredential(t *testing.T) {
	db, repo, service := setupManagementTestDB(t)
	defer database.Close(db)

	// Create test app
	app := &database.App{
		Name:       "Test App",
		OwnerEmail: "test@example.com",
		IsActive:   true,
	}
	repo.CreateApp(app)

	t.Run("ValidCreate", func(t *testing.T) {
		req := &CreateCredentialRequest{
			Name: "Test Credential",
		}

		cred, err := service.CreateCredential(app.ID, req)
		assert.NoError(t, err)
		assert.Equal(t, "Test Credential", cred.Name)
		assert.Equal(t, app.ID, cred.AppID)
		assert.NotEmpty(t, cred.KeyID)
		assert.NotEmpty(t, cred.SecretHash) // Contains the plain secret for response
	})

	t.Run("AppNotFound", func(t *testing.T) {
		req := &CreateCredentialRequest{
			Name: "Test Credential",
		}

		_, err := service.CreateCredential(999, req)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "app not found")
	})
}

func TestManagementService_AppLLMAssociation(t *testing.T) {
	db, repo, service := setupManagementTestDB(t)
	defer database.Close(db)

	// Create test app
	app := &database.App{
		Name:       "Test App",
		OwnerEmail: "test@example.com", 
		IsActive:   true,
	}
	repo.CreateApp(app)

	// Create test LLMs
	llm1 := &database.LLM{
		Name:         "LLM 1",
		Slug:         "llm-1",
		Vendor:       "openai",
		DefaultModel: "gpt-4",
		IsActive:     true,
	}
	llm2 := &database.LLM{
		Name:         "LLM 2",
		Slug:         "llm-2",
		Vendor:       "anthropic",
		DefaultModel: "claude-3",
		IsActive:     true,
	}
	repo.CreateLLM(llm1)
	repo.CreateLLM(llm2)

	t.Run("UpdateAppLLMs", func(t *testing.T) {
		err := service.UpdateAppLLMs(app.ID, []uint{llm1.ID, llm2.ID})
		assert.NoError(t, err)

		// Verify associations
		llms, err := service.GetAppLLMs(app.ID)
		assert.NoError(t, err)
		assert.Len(t, llms, 2)

		// Verify LLM names
		names := make(map[string]bool)
		for _, llm := range llms {
			names[llm.Name] = true
		}
		assert.True(t, names["LLM 1"])
		assert.True(t, names["LLM 2"])
	})

	t.Run("RemoveAssociations", func(t *testing.T) {
		// Update to only one LLM
		err := service.UpdateAppLLMs(app.ID, []uint{llm1.ID})
		assert.NoError(t, err)

		// Verify only one association remains
		llms, err := service.GetAppLLMs(app.ID)
		assert.NoError(t, err)
		assert.Len(t, llms, 1)
		assert.Equal(t, "LLM 1", llms[0].Name)
	})

	t.Run("GetAppLLMsNotFound", func(t *testing.T) {
		_, err := service.GetAppLLMs(999)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "app not found")
	})
}