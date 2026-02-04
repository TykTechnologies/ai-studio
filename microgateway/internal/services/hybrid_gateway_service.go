// internal/services/hybrid_gateway_service.go
package services

import (
	"fmt"
	"sync"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// TokenCacheEntry represents a cached token validation result
type TokenCacheEntry struct {
	Result    *TokenValidationResult
	CachedAt  time.Time
	ExpiresAt time.Time
}

// HybridGatewayService wraps DatabaseGatewayService and overrides token validation
// for on-demand validation while keeping all other operations local (database-backed)
type HybridGatewayService struct {
	*DatabaseGatewayService                                  // Embed DatabaseGatewayService for all other methods
	edgeClient     interface{}                               // Edge client for on-demand token validation
	edgeNamespace  string                                    // Edge namespace for token validation
	
	// Token validation cache
	tokenCache     map[string]*TokenCacheEntry              // token -> validation result
	cacheMutex     sync.RWMutex                            // Protects token cache
	cacheConfig    config.HubSpokeConfig                    // Cache configuration
	stopCleanup    chan bool                               // Signal to stop cleanup goroutine
}

// NewHybridGatewayService creates a hybrid gateway service for edge instances
func NewHybridGatewayService(db *gorm.DB, repo *database.Repository, edgeNamespace string, cacheConfig config.HubSpokeConfig) *HybridGatewayService {
	dbService := NewDatabaseGatewayService(db, repo)
	
	service := &HybridGatewayService{
		DatabaseGatewayService: dbService.(*DatabaseGatewayService),
		edgeNamespace:         edgeNamespace,
		tokenCache:           make(map[string]*TokenCacheEntry),
		cacheConfig:          cacheConfig,
		stopCleanup:          make(chan bool),
	}
	
	// Start cache cleanup goroutine if caching is enabled
	if cacheConfig.TokenCacheEnabled {
		go service.cacheCleanupWorker()
		log.Debug().
			Dur("ttl", cacheConfig.TokenCacheTTL).
			Int("max_size", cacheConfig.TokenCacheMaxSize).
			Dur("cleanup_interval", cacheConfig.TokenCacheCleanupInt).
			Msg("Token validation cache enabled for hybrid gateway service")
	}
	
	return service
}

// SetEdgeClient sets the edge client reference for on-demand token validation
func (h *HybridGatewayService) SetEdgeClient(edgeClient interface{}) {
	h.edgeClient = edgeClient
	log.Debug().Msg("Edge client set for hybrid gateway service on-demand token validation")
}

// ValidateAPIToken overrides DatabaseGatewayService to use cached on-demand validation
func (h *HybridGatewayService) ValidateAPIToken(token string) (*TokenValidationResult, error) {
	tokenPrefix := token
	if len(token) > 8 {
		tokenPrefix = token[:8]
	}

	// Check cache first if enabled
	if h.cacheConfig.TokenCacheEnabled {
		if cachedResult := h.getFromCache(token); cachedResult != nil {
			log.Debug().Str("token_prefix", tokenPrefix).Msg("Token validation cache HIT - returning cached result")
			return cachedResult, nil
		}
		log.Debug().Str("token_prefix", tokenPrefix).Msg("Token validation cache MISS - calling control instance")
	}

	log.Debug().Str("token_prefix", tokenPrefix).Msg("HybridGatewayService: using on-demand token validation")

	if h.edgeClient == nil {
		return nil, fmt.Errorf("edge client not available for on-demand token validation")
	}

	// Cast to edge client and make validation call
	if edgeClient, ok := h.edgeClient.(interface{ ValidateTokenOnDemand(string) (*pb.TokenValidationResponse, error) }); ok {
		resp, err := edgeClient.ValidateTokenOnDemand(token)
		if err != nil {
			log.Debug().Err(err).Str("token_prefix", tokenPrefix).Msg("On-demand token validation failed")
			return nil, fmt.Errorf("token validation failed: %w", err)
		}

		if !resp.Valid {
			log.Debug().Str("token_prefix", tokenPrefix).Str("error", resp.ErrorMessage).Msg("Token validation rejected by control")
			return nil, fmt.Errorf("invalid token: %s", resp.ErrorMessage)
		}

		log.Debug().
			Str("token_prefix", tokenPrefix).
			Uint32("app_id", resp.AppId).
			Str("app_name", resp.AppName).
			Msg("On-demand token validation successful")

		var app *database.App

		// Pull-on-miss: Use AppConfig from response if available
		if resp.AppConfig != nil {
			log.Debug().
				Uint32("app_id", resp.AppId).
				Str("app_namespace", resp.AppConfig.Namespace).
				Str("edge_namespace", h.edgeNamespace).
				Msg("Pull-on-miss: received AppConfig from control server")

			// Validate namespace: app must be global (empty namespace) or match edge namespace
			if !h.validateAppNamespace(resp.AppConfig.Namespace) {
				log.Warn().
					Str("token_prefix", tokenPrefix).
					Str("app_namespace", resp.AppConfig.Namespace).
					Str("edge_namespace", h.edgeNamespace).
					Msg("Namespace mismatch: app not accessible from this edge")
				return nil, fmt.Errorf("app namespace '%s' not accessible from edge namespace '%s'", resp.AppConfig.Namespace, h.edgeNamespace)
			}

			// Cache the app locally in SQLite for future requests
			app, err = h.cacheAppFromConfig(resp.AppConfig)
			if err != nil {
				log.Warn().Err(err).Uint32("app_id", resp.AppId).Msg("Failed to cache app locally, falling back to minimal app")
				// Fall back to creating a minimal app from the response
				app = h.createMinimalAppFromConfig(resp.AppConfig)
			}
		} else {
			// Fallback: Get the app from local SQLite database (legacy behavior)
			app, err = h.DatabaseGatewayService.GetAppByTokenID(uint(resp.AppId))
			if err != nil {
				// App not found in SQLite, create a minimal app object with the validated app_id
				log.Debug().Uint32("app_id", resp.AppId).Msg("App not found in local SQLite, using minimal app object")

				// Create minimal app - the app_llm relationships should exist from sync
				var dbApp database.App
				if err := h.db.Where("id = ?", resp.AppId).Preload("LLMs").First(&dbApp).Error; err != nil {
					return nil, fmt.Errorf("app %d not found in synced SQLite: %w", resp.AppId, err)
				}

				app = &dbApp
			}
		}

		result := &TokenValidationResult{
			TokenID:   uint(resp.AppId), // Use app_id as pseudo token ID
			TokenName: "on-demand-validated",
			AppID:     uint(resp.AppId),
			App:       app,
		}

		// Cache the result if caching is enabled
		if h.cacheConfig.TokenCacheEnabled {
			h.storeInCache(token, result)
			log.Debug().Str("token_prefix", tokenPrefix).Dur("ttl", h.cacheConfig.TokenCacheTTL).Msg("Token validation result cached")
		}

		return result, nil
	}

	return nil, fmt.Errorf("edge client does not support token validation")
}

// validateAppNamespace checks if the app's namespace is accessible from this edge
func (h *HybridGatewayService) validateAppNamespace(appNamespace string) bool {
	// Global apps (empty namespace) are accessible from all edges
	if appNamespace == "" {
		return true
	}
	// Global edges (empty namespace) can only access global apps
	if h.edgeNamespace == "" {
		return false
	}
	// Specific namespace edges can access global + matching namespace apps
	return appNamespace == h.edgeNamespace
}

// cacheAppFromConfig caches an app from the AppConfig into local SQLite
func (h *HybridGatewayService) cacheAppFromConfig(appConfig *pb.AppConfig) (*database.App, error) {
	// Check if app already exists in local SQLite
	var existingApp database.App
	err := h.db.Where("id = ?", appConfig.Id).First(&existingApp).Error
	if err == nil {
		// App exists, update it
		existingApp.Name = appConfig.Name
		existingApp.Description = appConfig.Description
		existingApp.OwnerEmail = appConfig.OwnerEmail
		existingApp.IsActive = appConfig.IsActive
		existingApp.MonthlyBudget = appConfig.MonthlyBudget
		existingApp.BudgetResetDay = int(appConfig.BudgetResetDay)
		existingApp.RateLimitRPM = int(appConfig.RateLimitRpm)
		existingApp.Namespace = appConfig.Namespace
		existingApp.UserID = uint(appConfig.UserId)
		if appConfig.AllowedIps != "" {
			existingApp.AllowedIPs = []byte(appConfig.AllowedIps)
		}
		if appConfig.Metadata != "" {
			existingApp.Metadata = []byte(appConfig.Metadata)
		}

		if err := h.db.Save(&existingApp).Error; err != nil {
			return nil, fmt.Errorf("failed to update cached app: %w", err)
		}

		// Update LLM relationships
		if err := h.updateAppLLMRelationships(uint(appConfig.Id), appConfig.LlmIds); err != nil {
			log.Warn().Err(err).Uint32("app_id", appConfig.Id).Msg("Failed to update app-LLM relationships")
		}

		// Reload with relationships
		if err := h.db.Where("id = ?", appConfig.Id).Preload("LLMs").First(&existingApp).Error; err != nil {
			return nil, fmt.Errorf("failed to reload cached app: %w", err)
		}

		log.Debug().
			Uint32("app_id", appConfig.Id).
			Str("app_name", appConfig.Name).
			Int("llm_count", len(existingApp.LLMs)).
			Msg("Pull-on-miss: updated existing app in local cache")

		return &existingApp, nil
	}

	if err != gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("failed to check for existing app: %w", err)
	}

	// App doesn't exist, create it
	newApp := &database.App{
		Name:           appConfig.Name,
		Description:    appConfig.Description,
		OwnerEmail:     appConfig.OwnerEmail,
		IsActive:       appConfig.IsActive,
		MonthlyBudget:  appConfig.MonthlyBudget,
		BudgetResetDay: int(appConfig.BudgetResetDay),
		RateLimitRPM:   int(appConfig.RateLimitRpm),
		Namespace:      appConfig.Namespace,
		UserID:         uint(appConfig.UserId),
	}
	// Set the ID explicitly for edge SQLite (matches control server ID)
	newApp.ID = uint(appConfig.Id)

	if appConfig.AllowedIps != "" {
		newApp.AllowedIPs = []byte(appConfig.AllowedIps)
	}
	if appConfig.Metadata != "" {
		newApp.Metadata = []byte(appConfig.Metadata)
	}

	// Use raw SQL to insert with explicit ID (GORM normally auto-increments)
	if err := h.db.Exec(
		`INSERT OR REPLACE INTO apps (id, name, description, owner_email, is_active, monthly_budget, budget_reset_day, rate_limit_rpm, namespace, user_id, allowed_ips, metadata, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now'), datetime('now'))`,
		newApp.ID, newApp.Name, newApp.Description, newApp.OwnerEmail, newApp.IsActive, newApp.MonthlyBudget, newApp.BudgetResetDay, newApp.RateLimitRPM, newApp.Namespace, newApp.UserID, newApp.AllowedIPs, newApp.Metadata,
	).Error; err != nil {
		return nil, fmt.Errorf("failed to cache new app: %w", err)
	}

	// Create LLM relationships
	if err := h.updateAppLLMRelationships(uint(appConfig.Id), appConfig.LlmIds); err != nil {
		log.Warn().Err(err).Uint32("app_id", appConfig.Id).Msg("Failed to create app-LLM relationships")
	}

	// Reload with relationships
	var cachedApp database.App
	if err := h.db.Where("id = ?", appConfig.Id).Preload("LLMs").First(&cachedApp).Error; err != nil {
		return nil, fmt.Errorf("failed to reload newly cached app: %w", err)
	}

	log.Info().
		Uint32("app_id", appConfig.Id).
		Str("app_name", appConfig.Name).
		Int("llm_count", len(cachedApp.LLMs)).
		Msg("Pull-on-miss: cached new app in local SQLite")

	return &cachedApp, nil
}

// updateAppLLMRelationships updates the app_llms join table for an app
func (h *HybridGatewayService) updateAppLLMRelationships(appID uint, llmIDs []uint32) error {
	// Delete existing relationships for this app
	if err := h.db.Exec("DELETE FROM app_llms WHERE app_id = ?", appID).Error; err != nil {
		return fmt.Errorf("failed to clear existing app-LLM relationships: %w", err)
	}

	// Insert new relationships
	for _, llmID := range llmIDs {
		if err := h.db.Exec(
			"INSERT INTO app_llms (app_id, llm_id, is_active, created_at, updated_at) VALUES (?, ?, 1, datetime('now'), datetime('now'))",
			appID, llmID,
		).Error; err != nil {
			log.Warn().Err(err).Uint("app_id", appID).Uint32("llm_id", llmID).Msg("Failed to insert app-LLM relationship")
		}
	}

	return nil
}

// createMinimalAppFromConfig creates a minimal in-memory App from AppConfig
// Used as a fallback when SQLite caching fails
func (h *HybridGatewayService) createMinimalAppFromConfig(appConfig *pb.AppConfig) *database.App {
	app := &database.App{
		Name:           appConfig.Name,
		Description:    appConfig.Description,
		OwnerEmail:     appConfig.OwnerEmail,
		IsActive:       appConfig.IsActive,
		MonthlyBudget:  appConfig.MonthlyBudget,
		BudgetResetDay: int(appConfig.BudgetResetDay),
		RateLimitRPM:   int(appConfig.RateLimitRpm),
		Namespace:      appConfig.Namespace,
		UserID:         uint(appConfig.UserId),
	}
	app.ID = uint(appConfig.Id)

	// Create LLM stubs for the relationship (allows LLM access validation)
	app.LLMs = make([]database.LLM, len(appConfig.LlmIds))
	for i, llmID := range appConfig.LlmIds {
		app.LLMs[i] = database.LLM{}
		app.LLMs[i].ID = uint(llmID)
	}

	log.Debug().
		Uint32("app_id", appConfig.Id).
		Int("llm_count", len(app.LLMs)).
		Msg("Created minimal in-memory app from AppConfig")

	return app
}

// GetAppByTokenID overrides to handle pseudo token IDs from on-demand validation
func (h *HybridGatewayService) GetAppByTokenID(tokenID uint) (*database.App, error) {
	log.Debug().Uint("token_id", tokenID).Msg("HybridGatewayService.GetAppByTokenID called")

	// For on-demand validation, token_id equals app_id
	// Get the app directly from local SQLite (now has full relationships!)
	var app database.App
	if err := h.db.Where("id = ?", tokenID).Preload("LLMs").First(&app).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			log.Debug().Uint("app_id", tokenID).Msg("App not found in local SQLite")
			return nil, fmt.Errorf("app not found: %d", tokenID)
		}
		return nil, fmt.Errorf("failed to get app from SQLite: %w", err)
	}

	log.Debug().
		Uint("token_id", tokenID).
		Uint("app_id", app.ID).
		Str("app_name", app.Name).
		Int("llm_count", len(app.LLMs)).
		Msg("Successfully found app with LLM relationships from synced SQLite")

	return &app, nil
}

// getFromCache retrieves a token validation result from cache if valid
func (h *HybridGatewayService) getFromCache(token string) *TokenValidationResult {
	if !h.cacheConfig.TokenCacheEnabled {
		return nil
	}

	h.cacheMutex.RLock()
	defer h.cacheMutex.RUnlock()

	entry, exists := h.tokenCache[token]
	if !exists {
		return nil
	}

	// Check if entry has expired
	if time.Now().After(entry.ExpiresAt) {
		return nil
	}

	return entry.Result
}

// storeInCache stores a token validation result in cache
func (h *HybridGatewayService) storeInCache(token string, result *TokenValidationResult) {
	if !h.cacheConfig.TokenCacheEnabled {
		return
	}

	h.cacheMutex.Lock()
	defer h.cacheMutex.Unlock()

	// Check cache size limit and evict oldest entries if needed
	if len(h.tokenCache) >= h.cacheConfig.TokenCacheMaxSize {
		h.evictOldestEntry()
	}

	// Store new entry
	now := time.Now()
	h.tokenCache[token] = &TokenCacheEntry{
		Result:    result,
		CachedAt:  now,
		ExpiresAt: now.Add(h.cacheConfig.TokenCacheTTL),
	}
}

// evictOldestEntry removes the oldest cache entry (simple LRU)
func (h *HybridGatewayService) evictOldestEntry() {
	var oldestToken string
	var oldestTime time.Time

	for token, entry := range h.tokenCache {
		if oldestToken == "" || entry.CachedAt.Before(oldestTime) {
			oldestToken = token
			oldestTime = entry.CachedAt
		}
	}

	if oldestToken != "" {
		delete(h.tokenCache, oldestToken)
	}
}

// cacheCleanupWorker periodically removes expired entries
func (h *HybridGatewayService) cacheCleanupWorker() {
	ticker := time.NewTicker(h.cacheConfig.TokenCacheCleanupInt)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			h.cleanupExpiredEntries()
		case <-h.stopCleanup:
			return
		}
	}
}

// cleanupExpiredEntries removes expired entries from cache
func (h *HybridGatewayService) cleanupExpiredEntries() {
	h.cacheMutex.Lock()
	defer h.cacheMutex.Unlock()

	now := time.Now()
	expiredCount := 0

	for token, entry := range h.tokenCache {
		if now.After(entry.ExpiresAt) {
			delete(h.tokenCache, token)
			expiredCount++
		}
	}

	if expiredCount > 0 {
		log.Debug().
			Int("expired_count", expiredCount).
			Int("cache_size", len(h.tokenCache)).
			Msg("Cleaned up expired token validation cache entries")
	}
}

// Stop stops the cache cleanup worker
func (h *HybridGatewayService) Stop() {
	if h.cacheConfig.TokenCacheEnabled {
		close(h.stopCleanup)
		log.Debug().Int("cache_size", len(h.tokenCache)).Msg("Stopped token validation cache cleanup worker")
	}
}