// internal/database/repository_test.go
package database

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Run migrations
	err = Migrate(db)
	require.NoError(t, err)

	return db
}

func TestRepository_LLM_Operations(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)

	testLLM := &LLM{
		Name:         "Test GPT-4",
		Slug:         "test-gpt-4",
		Vendor:       "openai",
		Endpoint:     "https://api.openai.com/v1",
		DefaultModel: "gpt-4",
		MaxTokens:    4096,
		IsActive:     true,
	}

	t.Run("CreateLLM", func(t *testing.T) {
		err := repo.CreateLLM(testLLM)
		assert.NoError(t, err)
		assert.NotZero(t, testLLM.ID)
	})

	t.Run("GetLLM", func(t *testing.T) {
		retrieved, err := repo.GetLLM(testLLM.ID)
		assert.NoError(t, err)
		assert.Equal(t, testLLM.Name, retrieved.Name)
		assert.Equal(t, testLLM.Slug, retrieved.Slug)
		assert.Equal(t, testLLM.Vendor, retrieved.Vendor)
	})

	t.Run("GetLLMBySlug", func(t *testing.T) {
		retrieved, err := repo.GetLLMBySlug("test-gpt-4")
		assert.NoError(t, err)
		assert.Equal(t, testLLM.ID, retrieved.ID)
		assert.Equal(t, testLLM.Name, retrieved.Name)
	})

	t.Run("ListLLMs", func(t *testing.T) {
		// Create another LLM (create as active first)
		llm2 := &LLM{
			Name:     "Test Claude",
			Slug:     "test-claude",
			Vendor:   "anthropic",
			IsActive: true,
		}
		err := repo.CreateLLM(llm2)
		require.NoError(t, err)
		
		// Now update to inactive
		llm2.IsActive = false
		err = repo.UpdateLLM(llm2)
		require.NoError(t, err)

		// List active LLMs
		llms, total, err := repo.ListLLMs(1, 10, "", true)
		assert.NoError(t, err)
		assert.Equal(t, int64(1), total)
		assert.Len(t, llms, 1)
		assert.Equal(t, "Test GPT-4", llms[0].Name)

		// List inactive LLMs  
		llms, total, err = repo.ListLLMs(1, 10, "", false)
		assert.NoError(t, err)
		assert.Equal(t, int64(1), total) // Only inactive ones
		assert.Len(t, llms, 1)
		assert.Equal(t, "Test Claude", llms[0].Name)

		// Filter by vendor
		llms, total, err = repo.ListLLMs(1, 10, "openai", true)
		assert.NoError(t, err)
		assert.Equal(t, int64(1), total)
		assert.Len(t, llms, 1)
		assert.Equal(t, "openai", llms[0].Vendor)
	})

	t.Run("GetActiveLLMs", func(t *testing.T) {
		llms, err := repo.GetActiveLLMs()
		assert.NoError(t, err)
		assert.Len(t, llms, 1)
		assert.Equal(t, "Test GPT-4", llms[0].Name)
		assert.True(t, llms[0].IsActive)
	})

	t.Run("UpdateLLM", func(t *testing.T) {
		testLLM.MaxTokens = 8192
		testLLM.MonthlyBudget = 100.0

		err := repo.UpdateLLM(testLLM)
		assert.NoError(t, err)

		// Verify update
		retrieved, err := repo.GetLLM(testLLM.ID)
		assert.NoError(t, err)
		assert.Equal(t, 8192, retrieved.MaxTokens)
		assert.Equal(t, 100.0, retrieved.MonthlyBudget)
	})

	t.Run("DeleteLLM", func(t *testing.T) {
		err := repo.DeleteLLM(testLLM.ID)
		assert.NoError(t, err)

		// Verify soft delete
		_, err = repo.GetLLM(testLLM.ID)
		assert.Error(t, err)
		assert.Equal(t, gorm.ErrRecordNotFound, err)
	})
}

func TestRepository_App_Operations(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)

	testApp := &App{
		Name:          "Test App",
		Description:   "A test application",
		OwnerEmail:    "test@example.com",
		IsActive:      true,
		MonthlyBudget: 500.0,
		RateLimitRPM:  1000,
	}

	t.Run("CreateApp", func(t *testing.T) {
		err := repo.CreateApp(testApp)
		assert.NoError(t, err)
		assert.NotZero(t, testApp.ID)
	})

	t.Run("GetApp", func(t *testing.T) {
		retrieved, err := repo.GetApp(testApp.ID)
		assert.NoError(t, err)
		assert.Equal(t, testApp.Name, retrieved.Name)
		assert.Equal(t, testApp.OwnerEmail, retrieved.OwnerEmail)
		assert.Equal(t, testApp.MonthlyBudget, retrieved.MonthlyBudget)
	})

	t.Run("ListApps", func(t *testing.T) {
		// Create another app (create as active first due to GORM default)
		app2 := &App{
			Name:     "Test App 2",
			IsActive: true,
		}
		err := repo.CreateApp(app2)
		require.NoError(t, err)
		
		// Now update to inactive
		app2.IsActive = false
		err = repo.UpdateApp(app2)
		require.NoError(t, err)

		// List active apps
		apps, total, err := repo.ListApps(1, 10, true)
		assert.NoError(t, err)
		assert.Equal(t, int64(1), total)
		assert.Len(t, apps, 1)
		assert.Equal(t, "Test App", apps[0].Name)

		// List inactive apps
		apps, total, err = repo.ListApps(1, 10, false)
		assert.NoError(t, err)
		assert.Equal(t, int64(1), total)
		assert.Len(t, apps, 1)
		assert.Equal(t, "Test App 2", apps[0].Name)
	})

	t.Run("UpdateApp", func(t *testing.T) {
		testApp.MonthlyBudget = 750.0
		testApp.RateLimitRPM = 2000

		err := repo.UpdateApp(testApp)
		assert.NoError(t, err)

		// Verify update
		retrieved, err := repo.GetApp(testApp.ID)
		assert.NoError(t, err)
		assert.Equal(t, 750.0, retrieved.MonthlyBudget)
		assert.Equal(t, 2000, retrieved.RateLimitRPM)
	})

	t.Run("DeleteApp", func(t *testing.T) {
		err := repo.DeleteApp(testApp.ID)
		assert.NoError(t, err)

		// Verify soft delete
		_, err = repo.GetApp(testApp.ID)
		assert.Error(t, err)
		assert.Equal(t, gorm.ErrRecordNotFound, err)
	})
}

func TestRepository_Credential_Operations(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)

	// Create test app first
	app := &App{
		Name:     "Test App",
		IsActive: true,
	}
	err := repo.CreateApp(app)
	require.NoError(t, err)

	testCred := &Credential{
		AppID:      app.ID,
		KeyID:      "test-key-123",
		SecretHash: "hashed-secret",
		Name:       "Test Credential",
		IsActive:   true,
	}

	t.Run("CreateCredential", func(t *testing.T) {
		err := repo.CreateCredential(testCred)
		assert.NoError(t, err)
		assert.NotZero(t, testCred.ID)
	})

	t.Run("GetCredentialBySecret", func(t *testing.T) {
		retrieved, err := repo.GetCredentialBySecret("hashed-secret")
		assert.NoError(t, err)
		assert.Equal(t, testCred.KeyID, retrieved.KeyID)
		assert.Equal(t, testCred.AppID, retrieved.AppID)
	})

	t.Run("GetCredentialByKeyID", func(t *testing.T) {
		retrieved, err := repo.GetCredentialByKeyID("test-key-123")
		assert.NoError(t, err)
		assert.Equal(t, testCred.ID, retrieved.ID)
		assert.Equal(t, testCred.SecretHash, retrieved.SecretHash)
	})

	t.Run("ListCredentials", func(t *testing.T) {
		// Create another credential
		cred2 := &Credential{
			AppID:      app.ID,
			KeyID:      "test-key-456",
			SecretHash: "another-hashed-secret",
			IsActive:   true,
		}
		err := repo.CreateCredential(cred2)
		require.NoError(t, err)

		// List credentials for app
		creds, err := repo.ListCredentials(app.ID)
		assert.NoError(t, err)
		assert.Len(t, creds, 2)
	})

	t.Run("UpdateCredentialLastUsed", func(t *testing.T) {
		err := repo.UpdateCredentialLastUsed(testCred.ID)
		assert.NoError(t, err)

		// Verify last used was updated
		retrieved, err := repo.GetCredentialByKeyID("test-key-123")
		assert.NoError(t, err)
		assert.NotNil(t, retrieved.LastUsedAt)
		assert.True(t, retrieved.LastUsedAt.After(time.Now().Add(-1*time.Minute)))
	})

	t.Run("DeleteCredential", func(t *testing.T) {
		err := repo.DeleteCredential(testCred.ID)
		assert.NoError(t, err)

		// Verify soft delete
		_, err = repo.GetCredentialByKeyID("test-key-123")
		assert.Error(t, err)
		assert.Equal(t, gorm.ErrRecordNotFound, err)
	})
}

func TestRepository_APIToken_Operations(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)

	// Create test app first
	app := &App{
		Name:     "Test App",
		IsActive: true,
	}
	err := repo.CreateApp(app)
	require.NoError(t, err)

	testToken := &APIToken{
		Token:    "test-api-token-123",
		Name:     "Test API Token",
		AppID:    app.ID,
		IsActive: true,
	}

	t.Run("CreateAPIToken", func(t *testing.T) {
		err := repo.CreateAPIToken(testToken)
		assert.NoError(t, err)
		assert.NotZero(t, testToken.ID)
	})

	t.Run("GetAPIToken", func(t *testing.T) {
		retrieved, err := repo.GetAPIToken("test-api-token-123")
		assert.NoError(t, err)
		assert.Equal(t, testToken.Name, retrieved.Name)
		assert.Equal(t, testToken.AppID, retrieved.AppID)
		assert.NotNil(t, retrieved.App)
		assert.Equal(t, app.Name, retrieved.App.Name)
	})

	t.Run("ListAPITokens", func(t *testing.T) {
		// Create another token
		token2 := &APIToken{
			Token:    "test-api-token-456",
			Name:     "Test API Token 2",
			AppID:    app.ID,
			IsActive: true,
		}
		err := repo.CreateAPIToken(token2)
		require.NoError(t, err)

		// List tokens for app
		tokens, err := repo.ListAPITokens(app.ID)
		assert.NoError(t, err)
		assert.Len(t, tokens, 2)
	})

	t.Run("UpdateAPITokenLastUsed", func(t *testing.T) {
		err := repo.UpdateAPITokenLastUsed(testToken.ID)
		assert.NoError(t, err)

		// Verify last used was updated
		retrieved, err := repo.GetAPIToken("test-api-token-123")
		assert.NoError(t, err)
		assert.NotNil(t, retrieved.LastUsedAt)
	})

	t.Run("RevokeAPIToken", func(t *testing.T) {
		err := repo.RevokeAPIToken("test-api-token-123")
		assert.NoError(t, err)

		// Verify token is inactive
		_, err = repo.GetAPIToken("test-api-token-123")
		assert.Error(t, err) // Should not find active token
		assert.Equal(t, gorm.ErrRecordNotFound, err)
	})
}

func TestRepository_BudgetUsage_Operations(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)

	// Create test app
	app := &App{
		Name:     "Test App",
		IsActive: true,
	}
	err := repo.CreateApp(app)
	require.NoError(t, err)

	// Create test LLM
	llm := &LLM{
		Name:     "Test LLM",
		Slug:     "test-llm",
		Vendor:   "openai",
		IsActive: true,
	}
	err = repo.CreateLLM(llm)
	require.NoError(t, err)

	periodStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	periodEnd := time.Date(2024, 1, 31, 23, 59, 59, 0, time.UTC)

	t.Run("GetOrCreateBudgetUsage", func(t *testing.T) {
		usage, err := repo.GetOrCreateBudgetUsage(app.ID, &llm.ID, periodStart, periodEnd)
		assert.NoError(t, err)
		assert.NotNil(t, usage)
		assert.Equal(t, app.ID, usage.AppID)
		assert.Equal(t, llm.ID, *usage.LLMID)
		assert.Equal(t, periodStart, usage.PeriodStart)
		assert.Equal(t, periodEnd, usage.PeriodEnd)
	})

	t.Run("UpdateBudgetUsage", func(t *testing.T) {
		// First get/create the usage record
		usage, err := repo.GetOrCreateBudgetUsage(app.ID, &llm.ID, periodStart, periodEnd)
		require.NoError(t, err)

		// Update usage
		err = repo.UpdateBudgetUsage(usage.ID, 1000, 5, 10.50, 800, 200)
		assert.NoError(t, err)

		// Verify updates
		retrieved, err := repo.GetBudgetUsage(app.ID, &llm.ID, periodStart, periodEnd)
		assert.NoError(t, err)
		assert.Equal(t, int64(1000), retrieved.TokensUsed)
		assert.Equal(t, 5, retrieved.RequestsCount)
		assert.Equal(t, 10.50, retrieved.TotalCost)
		assert.Equal(t, int64(800), retrieved.PromptTokens)
		assert.Equal(t, int64(200), retrieved.CompletionTokens)
	})

	t.Run("GetBudgetUsage", func(t *testing.T) {
		usage, err := repo.GetBudgetUsage(app.ID, &llm.ID, periodStart, periodEnd)
		assert.NoError(t, err)
		assert.NotNil(t, usage)
		assert.Equal(t, app.ID, usage.AppID)
	})

	t.Run("GetBudgetUsageNotFound", func(t *testing.T) {
		futureStart := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		futureEnd := time.Date(2025, 1, 31, 23, 59, 59, 0, time.UTC)
		
		_, err := repo.GetBudgetUsage(app.ID, &llm.ID, futureStart, futureEnd)
		assert.Error(t, err)
		assert.Equal(t, gorm.ErrRecordNotFound, err)
	})
}

func TestRepository_AnalyticsEvent_Operations(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)

	// Create test app
	app := &App{
		Name:     "Test App",
		IsActive: true,
	}
	err := repo.CreateApp(app)
	require.NoError(t, err)

	testEvent := &AnalyticsEvent{
		RequestID:      "req-123",
		AppID:          app.ID,
		Endpoint:       "/chat/completions",
		Method:         "POST",
		StatusCode:     200,
		RequestTokens:  100,
		ResponseTokens: 150,
		TotalTokens:    250,
		Cost:           0.005,
		LatencyMs:      250,
	}

	t.Run("CreateAnalyticsEvent", func(t *testing.T) {
		err := repo.CreateAnalyticsEvent(testEvent)
		assert.NoError(t, err)
		assert.NotZero(t, testEvent.ID)
	})

	t.Run("CreateAnalyticsEventsBatch", func(t *testing.T) {
		events := []AnalyticsEvent{
			{
				RequestID:  "req-456",
				AppID:      app.ID,
				Endpoint:   "/completions",
				Method:     "POST",
				StatusCode: 200,
				TotalTokens: 300,
			},
			{
				RequestID:  "req-789",
				AppID:      app.ID,
				Endpoint:   "/embeddings",
				Method:     "POST",
				StatusCode: 200,
				TotalTokens: 50,
			},
		}

		err := repo.CreateAnalyticsEventsBatch(events)
		assert.NoError(t, err)
	})

	t.Run("GetAnalyticsEvents", func(t *testing.T) {
		events, total, err := repo.GetAnalyticsEvents(app.ID, 1, 10)
		assert.NoError(t, err)
		assert.Equal(t, int64(3), total) // Should have 3 events total
		assert.Len(t, events, 3)

		// Verify ordering (should be DESC by created_at)
		assert.Equal(t, "req-789", events[0].RequestID) // Most recent first
	})

	t.Run("GetAnalyticsEventsPagination", func(t *testing.T) {
		events, total, err := repo.GetAnalyticsEvents(app.ID, 1, 2)
		assert.NoError(t, err)
		assert.Equal(t, int64(3), total)
		assert.Len(t, events, 2) // Limited to 2

		// Get next page
		events, total, err = repo.GetAnalyticsEvents(app.ID, 2, 2)
		assert.NoError(t, err)
		assert.Equal(t, int64(3), total)
		assert.Len(t, events, 1) // Only 1 remaining
	})
}

func TestRepository_WithTransaction(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRepository(db)

	t.Run("SuccessfulTransaction", func(t *testing.T) {
		var appID uint
		
		err := repo.WithTransaction(func(txRepo *Repository) error {
			app := &App{
				Name:     "Transactional App",
				IsActive: true,
			}
			if err := txRepo.CreateApp(app); err != nil {
				return err
			}
			appID = app.ID

			cred := &Credential{
				AppID:      app.ID,
				KeyID:      "tx-key",
				SecretHash: "tx-secret",
				IsActive:   true,
			}
			return txRepo.CreateCredential(cred)
		})

		assert.NoError(t, err)

		// Verify both app and credential were created
		app, err := repo.GetApp(appID)
		assert.NoError(t, err)
		assert.Equal(t, "Transactional App", app.Name)

		creds, err := repo.ListCredentials(appID)
		assert.NoError(t, err)
		assert.Len(t, creds, 1)
		assert.Equal(t, "tx-key", creds[0].KeyID)
	})

	t.Run("FailedTransaction", func(t *testing.T) {
		err := repo.WithTransaction(func(txRepo *Repository) error {
			app := &App{
				Name:     "Failed App",
				IsActive: true,
			}
			if err := txRepo.CreateApp(app); err != nil {
				return err
			}

			// Force an error by trying to create invalid credential
			cred := &Credential{
				AppID:      999999, // Non-existent app ID
				KeyID:      "invalid-key",
				SecretHash: "invalid-secret",
				IsActive:   true,
			}
			return txRepo.CreateCredential(cred)
		})

		assert.Error(t, err) // Transaction should fail

		// Verify no app was created due to transaction rollback
		apps, _, err := repo.ListApps(1, 100, true)
		assert.NoError(t, err)
		
		// Should not contain "Failed App"
		for _, app := range apps {
			assert.NotEqual(t, "Failed App", app.Name)
		}
	})
}