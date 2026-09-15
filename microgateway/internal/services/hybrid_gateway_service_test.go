// internal/services/hybrid_gateway_service_test.go
package services

import (
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupHybridTestDB(t *testing.T) (*gorm.DB, *database.Repository) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Migrate
	err = database.Migrate(db)
	require.NoError(t, err)

	repo := database.NewRepository(db)
	return db, repo
}

func createTestHybridService(t *testing.T, db *gorm.DB, repo *database.Repository) *HybridGatewayService {
	cacheConfig := config.HubSpokeConfig{
		TokenCacheEnabled:    false, // Disable caching for tests
		TokenCacheTTL:        5 * time.Minute,
		TokenCacheMaxSize:    100,
		TokenCacheCleanupInt: time.Minute,
	}

	return NewHybridGatewayService(db, repo, "test-namespace", cacheConfig)
}

func TestHybridGatewayService_storeAppFromPullOnMiss(t *testing.T) {
	t.Run("creates new app when not exists", func(t *testing.T) {
		db, repo := setupHybridTestDB(t)
		defer database.Close(db)

		service := createTestHybridService(t, db, repo)

		// Create test LLMs first
		llm1 := &database.LLM{Name: "Test LLM 1", Slug: "llm-1", Vendor: "openai", IsActive: true}
		llm2 := &database.LLM{Name: "Test LLM 2", Slug: "llm-2", Vendor: "anthropic", IsActive: true}
		db.Create(llm1)
		db.Create(llm2)

		now := time.Now()
		pbApp := &pb.AppConfig{
			Id:            100,
			Name:          "Test Pull-on-Miss App",
			Description:   "App created via pull-on-miss",
			IsActive:      true,
			MonthlyBudget: 500.0,
			Namespace:     "test-namespace",
			UserId:        42,
			LlmIds:        []uint32{uint32(llm1.ID), uint32(llm2.ID)},
			CreatedAt:     timestamppb.New(now),
			UpdatedAt:     timestamppb.New(now),
		}

		err := service.storeAppFromPullOnMiss(pbApp)
		require.NoError(t, err)

		// Verify app was created
		var app database.App
		err = db.Preload("LLMs").Where("id = ?", 100).First(&app).Error
		require.NoError(t, err)

		assert.Equal(t, "Test Pull-on-Miss App", app.Name)
		assert.Equal(t, "test-namespace", app.Namespace)
		assert.True(t, app.IsActive)
		assert.Equal(t, uint(42), app.UserID)
		assert.Len(t, app.LLMs, 2, "App should have 2 LLM associations")
	})

	t.Run("updates existing app when already exists", func(t *testing.T) {
		db, repo := setupHybridTestDB(t)
		defer database.Close(db)

		service := createTestHybridService(t, db, repo)

		// Create existing app
		existingApp := &database.App{
			Model:       gorm.Model{ID: 200},
			Name:        "Original App Name",
			Description: "Original description",
			IsActive:    true,
			Namespace:   "old-namespace",
		}
		db.Create(existingApp)

		// Create LLMs
		llm := &database.LLM{Name: "New LLM", Slug: "new-llm", Vendor: "openai", IsActive: true}
		db.Create(llm)

		now := time.Now()
		pbApp := &pb.AppConfig{
			Id:          200,
			Name:        "Updated App Name",
			Description: "Updated description",
			IsActive:    true,
			Namespace:   "new-namespace",
			LlmIds:      []uint32{uint32(llm.ID)},
			CreatedAt:   timestamppb.New(now),
			UpdatedAt:   timestamppb.New(now),
		}

		err := service.storeAppFromPullOnMiss(pbApp)
		require.NoError(t, err)

		// Verify app was updated
		var app database.App
		err = db.Preload("LLMs").Where("id = ?", 200).First(&app).Error
		require.NoError(t, err)

		assert.Equal(t, "Updated App Name", app.Name)
		assert.Equal(t, "new-namespace", app.Namespace)
		assert.Len(t, app.LLMs, 1, "App should have 1 LLM association after update")
	})

	t.Run("clears and recreates llm associations", func(t *testing.T) {
		db, repo := setupHybridTestDB(t)
		defer database.Close(db)

		service := createTestHybridService(t, db, repo)

		// Create LLMs
		llm1 := &database.LLM{Name: "LLM 1", Slug: "llm-1", Vendor: "openai", IsActive: true}
		llm2 := &database.LLM{Name: "LLM 2", Slug: "llm-2", Vendor: "anthropic", IsActive: true}
		llm3 := &database.LLM{Name: "LLM 3", Slug: "llm-3", Vendor: "vertex", IsActive: true}
		db.Create(llm1)
		db.Create(llm2)
		db.Create(llm3)

		// Create app with initial LLM associations
		existingApp := &database.App{
			Model:    gorm.Model{ID: 300},
			Name:     "Multi-LLM App",
			IsActive: true,
		}
		db.Create(existingApp)

		// Create initial app_llms associations
		db.Create(&database.AppLLM{AppID: 300, LLMID: llm1.ID, IsActive: true})
		db.Create(&database.AppLLM{AppID: 300, LLMID: llm2.ID, IsActive: true})

		// Update via pull-on-miss with different LLMs
		now := time.Now()
		pbApp := &pb.AppConfig{
			Id:        300,
			Name:      "Multi-LLM App",
			IsActive:  true,
			LlmIds:    []uint32{uint32(llm2.ID), uint32(llm3.ID)}, // Changed: removed llm1, added llm3
			CreatedAt: timestamppb.New(now),
			UpdatedAt: timestamppb.New(now),
		}

		err := service.storeAppFromPullOnMiss(pbApp)
		require.NoError(t, err)

		// Verify LLM associations were updated
		var appLLMs []database.AppLLM
		db.Where("app_id = ?", 300).Find(&appLLMs)

		assert.Len(t, appLLMs, 2, "Should have exactly 2 LLM associations")

		// Check the correct LLMs are associated
		llmIDMap := make(map[uint]bool)
		for _, appLLM := range appLLMs {
			llmIDMap[appLLM.LLMID] = true
		}
		assert.False(t, llmIDMap[llm1.ID], "LLM 1 should no longer be associated")
		assert.True(t, llmIDMap[llm2.ID], "LLM 2 should still be associated")
		assert.True(t, llmIDMap[llm3.ID], "LLM 3 should now be associated")
	})

	t.Run("preserves existing budget usage - does not overwrite", func(t *testing.T) {
		db, repo := setupHybridTestDB(t)
		defer database.Close(db)

		service := createTestHybridService(t, db, repo)

		// Create existing app with budget usage already tracked locally
		existingApp := &database.App{
			Model:         gorm.Model{ID: 400},
			Name:          "Budget App",
			IsActive:      true,
			MonthlyBudget: 1000.0,
		}
		db.Create(existingApp)

		// Simulate existing local budget usage (e.g., from previous requests)
		now := time.Now()
		periodStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		periodEnd := periodStart.AddDate(0, 1, 0)
		existingBudgetUsage := &database.BudgetUsage{
			AppID:       400,
			PeriodStart: periodStart,
			PeriodEnd:   periodEnd,
			TotalCost:   5000000, // $500 already used locally (stored as dollars * 10000)
		}
		db.Create(existingBudgetUsage)

		// Pull-on-miss with CurrentPeriodUsage=0 (control server doesn't calculate it)
		pbApp := &pb.AppConfig{
			Id:                 400,
			Name:               "Budget App Updated",
			IsActive:           true,
			MonthlyBudget:      1000.0,
			CurrentPeriodUsage: 0, // Intentionally 0 from control server
			CreatedAt:          timestamppb.New(now),
			UpdatedAt:          timestamppb.New(now),
		}

		err := service.storeAppFromPullOnMiss(pbApp)
		require.NoError(t, err)

		// Verify existing budget usage was NOT overwritten
		var budgetUsage database.BudgetUsage
		err = db.Where("app_id = ?", 400).First(&budgetUsage).Error
		require.NoError(t, err)

		// Should still be the original $500 (5000000), not reset to 0
		assert.Equal(t, 5000000.0, budgetUsage.TotalCost, "Existing budget usage should be preserved")
	})

	t.Run("handles nil timestamps gracefully", func(t *testing.T) {
		db, repo := setupHybridTestDB(t)
		defer database.Close(db)

		service := createTestHybridService(t, db, repo)

		pbApp := &pb.AppConfig{
			Id:        500,
			Name:      "No Timestamps App",
			IsActive:  true,
			CreatedAt: nil, // No timestamps
			UpdatedAt: nil,
		}

		err := service.storeAppFromPullOnMiss(pbApp)
		require.NoError(t, err)

		// Verify app was created
		var app database.App
		err = db.Where("id = ?", 500).First(&app).Error
		require.NoError(t, err)
		assert.Equal(t, "No Timestamps App", app.Name)
	})
}

// fakeEdgeClient scripts what the hub answers to on-demand validation.
type fakeEdgeClient struct {
	resp  *pb.TokenValidationResponse
	err   error
	calls int
}

func (f *fakeEdgeClient) ValidateTokenOnDemand(string) (*pb.TokenValidationResponse, error) {
	f.calls++
	return f.resp, f.err
}

// seedExpiredEntry plants a validation result whose TTL ran out expiredFor ago.
func seedExpiredEntry(h *HybridGatewayService, token string, expiredFor time.Duration) *TokenValidationResult {
	result := &TokenValidationResult{TokenID: 42, TokenName: "seeded", AppID: 42}
	h.cacheMutex.Lock()
	defer h.cacheMutex.Unlock()
	h.tokenCache[token] = &TokenCacheEntry{
		Result:    result,
		CachedAt:  time.Now().Add(-expiredFor - time.Minute),
		ExpiresAt: time.Now().Add(-expiredFor),
	}
	return result
}

func newStaleGraceService(t *testing.T, grace time.Duration) *HybridGatewayService {
	db, repo := setupHybridTestDB(t)
	t.Cleanup(func() { database.Close(db) })
	dbService := NewDatabaseGatewayService(db, repo)
	// Built by hand so no cleanup goroutine runs during the test.
	return &HybridGatewayService{
		DatabaseGatewayService: dbService.(*DatabaseGatewayService),
		tokenCache:             make(map[string]*TokenCacheEntry),
		cacheConfig: config.HubSpokeConfig{
			TokenCacheEnabled:    true,
			TokenCacheTTL:        time.Minute,
			TokenCacheMaxSize:    100,
			TokenCacheStaleGrace: grace,
		},
		stopCleanup: make(chan bool),
	}
}

// When the hub cannot be reached, a validation result that only just expired
// keeps the edge serving. Before this the edge failed closed on the first
// cache miss of an outage.
func TestHybridGatewayService_ValidateAPIToken_StaleGrace(t *testing.T) {
	t.Run("transport error within grace serves the stale entry", func(t *testing.T) {
		h := newStaleGraceService(t, time.Hour)
		seeded := seedExpiredEntry(h, "tok-stale", 10*time.Minute)
		h.SetEdgeClient(&fakeEdgeClient{err: assert.AnError})

		got, err := h.ValidateAPIToken("tok-stale")
		require.NoError(t, err)
		assert.Same(t, seeded, got)
	})

	t.Run("no edge client within grace serves the stale entry", func(t *testing.T) {
		h := newStaleGraceService(t, time.Hour)
		seeded := seedExpiredEntry(h, "tok-noclient", 10*time.Minute)

		got, err := h.ValidateAPIToken("tok-noclient")
		require.NoError(t, err)
		assert.Same(t, seeded, got)
	})

	t.Run("transport error past grace fails closed", func(t *testing.T) {
		h := newStaleGraceService(t, time.Hour)
		seedExpiredEntry(h, "tok-old", 2*time.Hour)
		h.SetEdgeClient(&fakeEdgeClient{err: assert.AnError})

		got, err := h.ValidateAPIToken("tok-old")
		require.Error(t, err)
		assert.Nil(t, got)
	})

	t.Run("explicit rejection ignores the stale entry and evicts it", func(t *testing.T) {
		h := newStaleGraceService(t, time.Hour)
		seedExpiredEntry(h, "tok-revoked", 10*time.Minute)
		h.SetEdgeClient(&fakeEdgeClient{resp: &pb.TokenValidationResponse{Valid: false, ErrorMessage: "revoked"}})

		got, err := h.ValidateAPIToken("tok-revoked")
		require.Error(t, err)
		assert.Nil(t, got)
		assert.Contains(t, err.Error(), "revoked")

		h.cacheMutex.RLock()
		_, still := h.tokenCache["tok-revoked"]
		h.cacheMutex.RUnlock()
		assert.False(t, still, "a rejected token must not linger in the cache")

		// Even a later outage cannot resurrect it.
		h.SetEdgeClient(&fakeEdgeClient{err: assert.AnError})
		_, err = h.ValidateAPIToken("tok-revoked")
		require.Error(t, err)
	})

	t.Run("grace of zero is fail-closed", func(t *testing.T) {
		h := newStaleGraceService(t, 0)
		seedExpiredEntry(h, "tok-nograce", time.Second)
		h.SetEdgeClient(&fakeEdgeClient{err: assert.AnError})

		got, err := h.ValidateAPIToken("tok-nograce")
		require.Error(t, err)
		assert.Nil(t, got)
	})

	t.Run("fresh entries never reach the hub", func(t *testing.T) {
		h := newStaleGraceService(t, time.Hour)
		client := &fakeEdgeClient{err: assert.AnError}
		h.SetEdgeClient(client)
		fresh := &TokenValidationResult{TokenID: 7, AppID: 7}
		h.storeInCache("tok-fresh", fresh)

		got, err := h.ValidateAPIToken("tok-fresh")
		require.NoError(t, err)
		assert.Same(t, fresh, got)
		assert.Equal(t, 0, client.calls)
	})

	t.Run("cleanup keeps entries inside the grace and drops those past it", func(t *testing.T) {
		h := newStaleGraceService(t, time.Hour)
		seedExpiredEntry(h, "tok-keep", 10*time.Minute)
		seedExpiredEntry(h, "tok-drop", 2*time.Hour)

		h.cleanupExpiredEntries()

		h.cacheMutex.RLock()
		defer h.cacheMutex.RUnlock()
		_, keep := h.tokenCache["tok-keep"]
		_, drop := h.tokenCache["tok-drop"]
		assert.True(t, keep)
		assert.False(t, drop)
	})
}
